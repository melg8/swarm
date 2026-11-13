// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Manual command handling of the hunt loop: the web UI queues commands on the
// tracker and the loop turns them into world actions. The one shot inventory
// commands execute at once, the movement commands enter the manual phase.

// Timing and threshold constants of the manual mode.
const (
    // userArriveRadius is the distance at which a manual move counts as
    // arrived (the server stops a collision radius short anyway).
    userArriveRadius = 60.0
    // userMoveTimeout bounds one manual walk without a planned route; a
    // planned mesh route carries its own priced budget (userMoveBudget).
    userMoveTimeout = 90 * time.Second
    // userMoveTimeoutMax caps the budget of a long planned route: a cross-map
    // click is valid, an endless walk is not.
    userMoveTimeoutMax = 30 * time.Minute
    // userPickupTimeout bounds one manual pickup (the autonomous looting span).
    userPickupTimeout = 30 * time.Second
    // userAttackTimeout bounds a manual attack that never starts the fight.
    userAttackTimeout = 30 * time.Second
    // userApproachRadius is the search goal of a long manual move: the walk
    // ends within this 3D distance of the clicked point.
    userApproachRadius = 150.0
    // inventoryConfirmTimeout bounds how long one inventory action blocks a
    // different item: the gate releases as soon as the tracker sees the effect,
    // the timeout only rescues a refused request (the UseItem flood protector
    // is disabled: FloodProtectorUseItemInterval = 0).
    inventoryConfirmTimeout = 600 * time.Millisecond
    // inventoryResendTimeout bounds how long the same item stays gated against
    // a re-send: the Mobius UseItem handler defers an equip inside the attack
    // window to the attack end (~1.5 s bow windup, ~3 s cycle).
    inventoryResendTimeout = 4 * time.Second
)

// consumeUserCommands drains the command queue: the inventory commands execute
// at once (gated on the previous confirmation), the movement commands
// reprogram the manual phase, the newest wins.
func (l *Loop) consumeUserCommands() {
    l.flushDeferredCommands()
    for {
        select {
        case cmd := <-l.tracker.Commands():
            l.applyUserCommand(cmd)
        default:
            return
        }
    }
}

// flushDeferredCommands retries the inventory commands the spacing gate
// deferred, oldest first; the gate re-checks every entry.
func (l *Loop) flushDeferredCommands() {
    if len(l.userDeferred) == 0 {
        return
    }
    kept := l.userDeferred[:0]
    for _, cmd := range l.userDeferred {
        if l.gateInventoryCommand(cmd) {
            kept = append(kept, cmd)

            continue
        }
        l.applyUserCommand(cmd)
    }
    l.userDeferred = kept
}

// gateInventoryCommand defers a command that would race an in-flight inventory
// action: the Mobius executor runs every client packet on its own task, so two
// writes on the same item or slots cancel each other. The gate holds the
// newcomer until the tracker sees the effect.
func (l *Loop) gateInventoryCommand(cmd state.Command) bool {
    switch cmd.Kind {
    case state.CommandUseItem, state.CommandDrop, state.CommandDestroy:
    default:
        return false
    }
    if l.inventoryItemAllowed(cmd.ObjectID,
        gear.ServerWriteSlots(l.equipment(), cmd.ObjectID)) {
        return false
    }
    l.userDeferred = append(l.userDeferred, cmd)
    l.logf("Hunt: user command %q waits for the item pace",
        cmd.Kind)

    return true
}

// pendingInventory is one in-flight action: the item state at the request and
// the paperdoll slots its write set touches.
type pendingInventory struct {
    equip bool
    count int32
    at    time.Time
    slots []gear.Slot
}

// inventoryItemAllowed reports whether a new action on the item may go out: no
// in-flight action on the same item and none writing a touched slot.
func (l *Loop) inventoryItemAllowed(objectID int32, slots []gear.Slot) bool {
    now := l.now()
    for pendingID, pending := range l.pendingActions {
        if l.pendingInventoryConfirmed(pendingID, pending) ||
            now.Sub(pending.at) >= inventoryResendTimeout {
            delete(l.pendingActions, pendingID)

            continue
        }
        if pendingID == objectID {
            // The same item: the re-send guard outlives the attack-window
            // equip deferral (inventoryResendTimeout).
            return false
        }
        if now.Sub(pending.at) >= inventoryConfirmTimeout {
            // A stale pending of another item no longer gates the slots it
            // writes, but the same-item guard above still holds its re-send.
            continue
        }
        for _, pendingSlot := range pending.slots {
            for _, slot := range slots {
                if pendingSlot == slot {
                    return false
                }
            }
        }
    }

    return true
}

// prunePendingActions drops the in-flight actions the tracker confirmed or
// whose re-send window expired (inventoryResendTimeout).
func (l *Loop) prunePendingActions(now time.Time) {
    for pendingID, pending := range l.pendingActions {
        if l.pendingInventoryConfirmed(pendingID, pending) ||
            now.Sub(pending.at) >= inventoryResendTimeout {
            delete(l.pendingActions, pendingID)
        }
    }
}

// pendingInventoryConfirmed checks the tracked inventory for the effect of one
// action: a changed equipped flag, stack count or the vanished item means the
// server processed the request.
func (l *Loop) pendingInventoryConfirmed(
    objectID int32, pending pendingInventory,
) bool {
    item, ok := l.tracker.InventoryItemState(objectID)
    if !ok {
        return true
    }

    return item.Equipped != pending.equip ||
        item.Count != pending.count
}

func (l *Loop) applyUserCommand(cmd state.Command) {
    if l.gateInventoryCommand(cmd) {
        return
    }
    switch cmd.Kind {
    case state.CommandUseItem:
        l.userUseItem(cmd)
    case state.CommandDrop:
        l.userDrop(cmd)
    case state.CommandDestroy:
        l.userDestroy(cmd)
    case state.CommandSay:
        l.userSay(cmd)
    case state.CommandMove, state.CommandAttack, state.CommandPickup:
        l.userMovement(cmd)
    default:
        l.logf("Hunt: unknown user command %q", cmd.Kind)
    }
}

// userSay sends one chat message: the web UI validated it against the Say2
// refusals, and the packet validates itself again (an invalid chat would
// disconnect the session).
func (l *Loop) userSay(cmd state.Command) {
    if cmd.Text == "" {
        return
    }
    l.logf("Hunt: user command: say %q on channel %d",
        cmd.Text, cmd.Channel)
    if err := l.game.Say(cmd.Text, cmd.Channel, cmd.Target); err != nil {
        l.logf("Hunt: say failed: %v", err)
    }
}

// markInventoryAction records one in-flight action: the item state at the
// request plus the paperdoll slots its write set touches.
func (l *Loop) markInventoryAction(objectID int32) {
    item, ok := l.tracker.InventoryItemState(objectID)
    pending := pendingInventory{
        equip: false,
        count: 0,
        at:    l.now(),
        slots: gear.ServerWriteSlots(l.equipment(), objectID),
    }
    if ok {
        pending.equip = item.Equipped
        pending.count = item.Count
    }
    l.pendingActions[objectID] = pending
}

// userUseItem executes the equip/unequip toggle at once: the packet is a one
// shot request, the server applies or refuses it, the gate paces the pairs.
func (l *Loop) userUseItem(cmd state.Command) {
    if cmd.ObjectID == 0 {
        return
    }
    l.logf("Hunt: user command: use item %d", cmd.ObjectID)
    l.markInventoryAction(cmd.ObjectID)
    if err := l.game.UseItem(cmd.ObjectID); err != nil {
        l.logf("Hunt: use item failed: %v", err)
    }
}

// userDrop drops inventory items at the feet: the server accepts drops only
// within 150 units of the character, so the own position is the drop point.
func (l *Loop) userDrop(cmd state.Command) {
    if cmd.ObjectID == 0 || cmd.Count < 1 {
        return
    }
    x, y, z, ok := l.tracker.SelfPosition()
    if !ok {
        l.logf("Hunt: user command: drop failed: no self position")

        return
    }
    l.logf("Hunt: user command: drop %d of item %d",
        cmd.Count, cmd.ObjectID)
    l.markInventoryAction(cmd.ObjectID)
    if err := l.game.DropItem(cmd.ObjectID, cmd.Count, x, y, z); err != nil {
        l.logf("Hunt: drop item failed: %v", err)
    }
}

// userDestroy destroys inventory items in place: the server splits the stack
// and unequips an equipped item itself.
func (l *Loop) userDestroy(cmd state.Command) {
    if cmd.ObjectID == 0 || cmd.Count < 1 {
        return
    }
    l.logf("Hunt: user command: destroy %d of item %d",
        cmd.Count, cmd.ObjectID)
    l.markInventoryAction(cmd.ObjectID)
    if err := l.game.DestroyItem(cmd.ObjectID, cmd.Count); err != nil {
        l.logf("Hunt: destroy item failed: %v", err)
    }
}

// userMovement switches the loop into the manual phase for a move, attack or
// pickup click: a town trip is cancelled, a deleveling refuses the command.
func (l *Loop) userMovement(cmd state.Command) {
    if l.phase == phaseDelevel {
        l.logf("Hunt: user command %q ignored while deleveling",
            cmd.Kind)

        return
    }
    if l.phase == phaseUser {
        l.logf("Hunt: user command: %s replaces the manual %s",
            cmd.Kind, l.userKind)
    } else if l.tripActive() {
        l.logf("Hunt: user command: %s cancels the town trip",
            cmd.Kind)
        l.resetTownTrip()
    }
    l.phase = phaseUser
    l.userKind = cmd.Kind
    l.userX = cmd.X
    l.userY = cmd.Y
    l.userZ = cmd.Z
    l.userTarget = cmd.ObjectID
    l.userStart = l.now()
    l.userDeadline = time.Time{}
    l.userMoveAt = time.Time{}
    // The new command redirects a running walk: the next tick re-issues the
    // request at once (the server replaces the destination of a running walk).
    l.userRedirect = true
    l.userWaypoints = nil
    l.userSearch = nil
    l.userWpIndex = 0
    l.userPathTried = false
    l.userFrameOffset = 0
    // The walk plan origin: where the character stood when the click arrived
    // (the dump prints the whole walk from it); unknown positions keep the zero
    // sentinel.
    if selfX, selfY, selfZ, ok := l.tracker.SelfPosition(); ok {
        l.userPlanX, l.userPlanY, l.userPlanZ = selfX, selfY, selfZ
    } else {
        l.userPlanX, l.userPlanY, l.userPlanZ = 0, 0, 0
    }
    l.resetChaseSamples()
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.engageAt = time.Time{}
    switch cmd.Kind {
    case state.CommandMove:
        l.logf("Hunt: user command: walking to %d %d %d",
            cmd.X, cmd.Y, cmd.Z)
    case state.CommandAttack:
        l.logf("Hunt: user command: attacking object %d",
            cmd.ObjectID)
    case state.CommandPickup:
        l.logf("Hunt: user command: picking up item %d",
            cmd.ObjectID)
    }
}

// tickUser advances the manual phase at the player action cadence (one action
// per second) until it completes. A sitting character stands up first: the
// server refuses every action of a seated session.
func (l *Loop) tickUser() {
    now := l.now()
    if l.tracker.SelfSitting() && l.userKind != "" {
        // The stand shares the pending transition gate with the rest logic;
        // the walk starts once ChangeWaitType confirms the standing.
        if !l.standUpGuarded(now) {
            return
        }
    }
    switch l.userKind {
    case state.CommandMove:
        l.tickUserMove(now)
    case state.CommandAttack:
        l.tickUserAttack(now)
    case state.CommandPickup:
        l.tickUserPickup(now)
    default:
        l.resumeAuto()
    }
}

// tickUserMove walks to the clicked point until it arrives or the walk times
// out; the request repeats only when the walk stalled.
func (l *Loop) tickUserMove(now time.Time) {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        l.resumeAuto()

        return
    }
    if len(l.userWaypoints) > 0 {
        l.followUserWaypoints(now, selfX, selfY, selfZ)

        return
    }
    dist := math.Hypot(float64(l.userX-selfX), float64(l.userY-selfY))
    if dist <= userArriveRadius {
        l.logf("Hunt: manual walk arrived (%d units left)",
            int(dist))
        l.resumeAuto()

        return
    }
    if now.Sub(l.userStart) > userMoveTimeout {
        l.logf("Hunt: manual walk timed out, resuming the hunt")
        l.resumeAuto()

        return
    }
    if l.navigator != nil && !l.userPathTried {
        // Every manual move plans through the navigator, so a stuck segment
        // reads in the walk plan view like a bot walk; a failed search falls
        // through to the direct walk.
        l.userPathTried = true
        l.planUserWalk(selfX, selfY, selfZ)

        return
    }
    if l.tracker.SelfWalking() && !l.userRedirect {
        // The walk runs toward the manual target: the server path stays.
        return
    }
    if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < selectPeriod {
        return
    }
    l.userRedirect = false
    l.userMoveAt = now
    if err := l.game.WalkTo(l.userX, l.userY, l.userZ); err != nil {
        l.logf("Hunt: manual walk failed: %v", err)
    }
}

// planUserWalk computes the mesh path of one long manual move: the exact search
// first (the owner clicked the point), the approach corridor as the fallback,
// the direct server walk without a plan. A partial corridor ends at the closest
// reachable point.
func (l *Loop) planUserWalk(selfX int32, selfY int32, selfZ int32) {
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    end := pathfind.Vec3{
        X: float64(l.userX), Y: float64(l.userY), Z: float64(l.userZ),
    }
    approach := 0.0
    result, err := l.navigator.FindPath(from, end)
    if err != nil || result == nil || len(result.Waypoints) == 0 ||
        (!result.Found && !result.Partial) {
        approach = userApproachRadius
        result, err = l.navigator.FindPathApproach(from, end,
            userApproachRadius)
    }
    if err != nil {
        l.logf("Hunt: manual walk path search failed: %v", err)

        return
    }
    if result == nil || len(result.Waypoints) == 0 ||
        (!result.Found && !result.Partial) {
        l.logf("Hunt: no mesh path to %d %d, "+
            "walking by server routing", l.userX, l.userY)

        return
    }
    if !result.Found {
        l.logf("Hunt: mesh route to %d %d is partial, "+
            "walking the closest reachable corridor", l.userX, l.userY)
    }
    l.userWaypoints = result.Waypoints
    l.userWpIndex = 0
    // The route budget: the plan length over the measured run speed estimates
    // the walk time, so a long route (the owner repro walks 20k units) is not
    // cut off at the base window.
    speed := l.tracker.SelfRunSpeed()
    if speed <= 0 {
        speed = flowDefaultRunSpeed
    }
    l.userDeadline = l.now().Add(
        userMoveBudget(manualTravel(result.Length, speed)))
    // The plan publishes its search contract (the approach radius): the 3D
    // pathfind link rebuilds the very search.
    l.userSearch = &state.WalkSearch{Approach: approach,
        Avoid: nil}
    // The plan opens with the frame measurement of every fresh plan
    // (click_frame.go); a swimming character measures no shift.
    l.userFrameOffset = 0
    if !l.navigator.OverWater(
        float64(selfX), float64(selfY), int16(selfZ)) {
        l.userFrameOffset = measureFrameOffset(
            selfZ, result.Waypoints[0].Z)
    }
    l.userMoveAt = time.Time{}
    // The flow scan shares the skip bookkeeping with the town walk: a manual
    // plan must not inherit the dead-cell verdict of the previous walk.
    l.skipArmed = false
    l.skipX, l.skipY = 0, 0
    l.userRefusalVariants = 0
    l.cursorEscapes = 0
    l.cursorEscape = cursorEscapeState{} //nolint:exhaustruct_v5 // zero reset
    l.logf("Hunt: manual walk path planned: %d waypoints, "+
        "%.0f units (%.2fs search)", len(result.Waypoints),
        result.Length, result.Duration.Seconds())
}

// userSegmentRefused reports whether the server answered the last manual click
// with ActionFailed while the character stood still (the refusalEvidence
// attribution).
func (l *Loop) userSegmentRefused() bool {
    if l.userMoveAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    return failedAt.After(l.userMoveAt) &&
        failedAt.Sub(l.userMoveAt) <= refusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.userMoveAt, failedAt)
}

// sendUserVariedAim answers a refused manual click by varying the aim at the
// waypoint: the refusal is target specific, and a shorter prefix or a sideways
// offset often validates. The variants ride the same frame transport.
func (l *Loop) sendUserVariedAim(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    if l.navigator == nil || l.userWpIndex >= len(l.userWaypoints) ||
        l.userRefusalVariants >= refusalVariantsMax {
        return false
    }
    wp := l.userWaypoints[l.userWpIndex]
    wpZ := anchorZToServerFrame(wp.Z, l.userFrameOffset)
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for ; l.userRefusalVariants < refusalVariantsMax; l.userRefusalVariants++ {
        variant := refusalVariantTarget(
            pathfind.Vec3{X: wp.X, Y: wp.Y, Z: wpZ}, from,
            l.userRefusalVariants)
        if variant == nil {
            continue
        }
        to := pathfind.Vec3{
            X: variant[0], Y: variant[1], Z: variant[2],
        }
        if _, ok := l.navigator.ValidateClick(from, to); !ok {
            continue
        }
        l.userRefusalVariants++
        l.logf("Hunt: the server refused the manual walk click, "+
            "varying the aim to %.0f %.0f (%d of %d)",
            to.X, to.Y, l.userRefusalVariants, refusalVariantsMax)
        l.userMoveAt = now
        if err := l.game.WalkTo(
            int32(to.X), int32(to.Y), int32(to.Z)); err != nil {
            l.logf("Hunt: manual walk request failed: %v", err)
        }

        return true
    }

    return false
}

// beginUserCursorKeyEscape hands a refused manual segment to the cursor key
// escape: the keyboard mode 0 arm plus the claimed ValidatePosition stream,
// which the server follows without click validation.
func (l *Loop) beginUserCursorKeyEscape(
    selfX, selfY, selfZ, aimX, aimY, aimZ int32,
) bool {
    if l.cursorEscapes >= cursorEscapeAttemptsMax {
        return false
    }
    steps := l.cursorEscapeSteps(selfX, selfY, selfZ,
        aimX, aimY, aimZ)
    if len(steps) == 0 {
        return false
    }
    arm := steps[len(steps)-1]
    l.cursorEscapes++
    l.cursorEscape = cursorEscapeState{
        armed:           true,
        originX:         selfX,
        originY:         selfY,
        steps:           steps,
        next:            0,
        lastClaimAt:     time.Time{},
        claimsSinceMove: 0,
        wpMap:           nil,
    }
    if err := l.game.CursorKeyWalkTo(arm[0], arm[1], arm[2]); err != nil {
        l.logf("Hunt: the cursor key arm failed: %v", err)
    }
    l.logf("Hunt: the refused manual clicks hand the walk to the "+
        "cursor key escape toward %d %d (%d claimed steps)",
        arm[0], arm[1], len(steps))

    return true
}

// followUserWaypoints walks the planned segments of a long manual move: the
// click spans the farthest waypoints inside the aim window, longer segments
// split into straight points, a passed waypoint skips ahead.
func (l *Loop) followUserWaypoints(
    now time.Time, selfX int32, selfY int32, selfZ int32,
) {
    next, moved := advanceWalkCursor(l.userWaypoints, l.userWpIndex,
        selfX, selfY, selfZ, l.userFrameOffset, waypointArriveDist, nil)
    l.userWpIndex = next
    if moved {
        l.userMoveAt = time.Time{}
        l.userRefusalVariants = 0
    }
    if l.userWpIndex >= len(l.userWaypoints) {
        l.logf("Hunt: manual walk arrived (path done)")
        l.resumeAuto()

        return
    }
    if l.userMoveExpired(now) {
        l.logf("Hunt: manual walk timed out, resuming the hunt")
        l.resumeAuto()

        return
    }
    // The cursor key escape owns the walk while it runs: the claim ladder
    // drives past the refusing ground, the click machinery stays down.
    if l.cursorEscape.armed {
        l.driveCursorKeyEscape(now, selfX, selfY)

        return
    }
    if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < walkRequestPeriod {
        return
    }
    // The refusal ladder: it varies the aim (the refusal is target specific)
    // and hands the segment to the cursor key escape once the variants spent.
    if l.userSegmentRefused() {
        if l.sendUserVariedAim(selfX, selfY, selfZ, now) {
            return
        }
        wp := l.userWaypoints[l.userWpIndex]
        wpZ := anchorZToServerFrame(wp.Z, l.userFrameOffset)
        if l.beginUserCursorKeyEscape(selfX, selfY, selfZ,
            int32(wp.X), int32(wp.Y), int32(wpZ)) {
            return
        }
    }
    // The goal span: the farthest route waypoint the server delivers inside
    // the aim window (flowAdvance), so the walk crosses the dense plan points
    // without an arrival stop.
    l.userWpIndex = l.flowAdvance(l.userWaypoints, l.userWpIndex,
        l.userFrameOffset, selfX, selfY, selfZ)
    l.clickUserWaypoint(now, selfX, selfY, selfZ)
}

// userMoveBudget resolves the time budget of a manual walk: the base window
// plus twice the estimated travel, capped at userMoveTimeoutMax.
func userMoveBudget(travel time.Duration) time.Duration {
    budget := userMoveTimeout + 2*travel
    if budget > userMoveTimeoutMax {
        return userMoveTimeoutMax
    }

    return budget
}

// manualTravel estimates the walk time of a plan: the route length over the
// measured run speed; a missing length or speed answers zero.
func manualTravel(length, speed float64) time.Duration {
    if length <= 0 || speed <= 0 {
        return 0
    }

    return time.Duration(length / speed * float64(time.Second))
}

// userMoveExpired reports whether the manual walk spent its budget: a planned
// route counts from its deadline (planUserWalk), the direct walk keeps the base
// window.
func (l *Loop) userMoveExpired(now time.Time) bool {
    if !l.userDeadline.IsZero() {
        return now.After(l.userDeadline)
    }

    return now.Sub(l.userStart) > userMoveTimeout
}

// clickUserWaypoint clicks toward the current manual waypoint: the z rides the
// plan's frame transport and a long segment splits into a straight point.
func (l *Loop) clickUserWaypoint(
    now time.Time, selfX int32, selfY int32, selfZ int32,
) {
    wp := l.userWaypoints[l.userWpIndex]
    wpZ := anchorZToServerFrame(wp.Z, l.userFrameOffset)
    moveX, moveY, moveZ := walkClickSegment(
        selfX, selfY, selfZ, wp.X, wp.Y, wpZ, maxMoveDistance)
    l.userRedirect = false
    l.userMoveAt = now
    if err := l.game.WalkTo(
        int32(moveX), int32(moveY), int32(moveZ)); err != nil {
        l.logf("Hunt: manual walk failed: %v", err)
    }
}

// publishWalkPlan refreshes the walk plan view: a running walk publishes the
// full plan of the current segment, every other state clears it.
func (l *Loop) publishWalkPlan() {
    plan := l.activeWalkPlan()
    if plan == nil {
        l.tracker.ClearWalkPlan()

        return
    }
    l.tracker.SetWalkPlan(*plan)
}

// activeWalkPlan returns the walk plan of the followed segment, nil when the
// loop walks no planned path.
func (l *Loop) activeWalkPlan() *state.WalkPlan {
    switch l.phase {
    case phaseUser:
        if l.userKind != state.CommandMove {
            return nil
        }

        return l.userWalkPlan()
    case phaseTownWalk, phaseTownReturn, phaseDelevel:
        return l.geodataWalkPlan()
    default:
        return nil
    }
}

// userWalkPlan builds the walk plan of a manual move: the origin, the
// waypoints, the follower cursor and the clicked destination.
func (l *Loop) userWalkPlan() *state.WalkPlan {
    dest := state.WalkPoint{X: l.userX, Y: l.userY, Z: l.userZ}
    pts := []state.WalkPoint{}
    for _, wp := range l.userWaypoints {
        pts = append(pts, state.WalkPoint{
            X: int32(wp.X), Y: int32(wp.Y), Z: int32(wp.Z),
        })
    }
    if len(pts) == 0 {
        pts = append(pts, dest)
    }
    var origin *state.WalkPoint
    if l.userPlanX != 0 || l.userPlanY != 0 {
        origin = &state.WalkPoint{
            X: l.userPlanX, Y: l.userPlanY, Z: l.userPlanZ,
        }
    }

    return &state.WalkPlan{
        Origin: origin,
        Points: pts,
        Index:  min(l.userWpIndex, len(pts)-1),
        Dest:   &dest,
        Search: l.userSearch,
    }
}

// geodataWalkPlan builds the walk plan of a town trip or guard walk: the
// planning origin, the waypoint list with the cursor and the segment
// destination startWalkSegment armed.
func (l *Loop) geodataWalkPlan() *state.WalkPlan {
    if len(l.waypoints) == 0 {
        return nil
    }
    pts := make([]state.WalkPoint, 0, len(l.waypoints))
    for _, wp := range l.waypoints {
        pts = append(pts, state.WalkPoint{
            X: int32(wp.X), Y: int32(wp.Y), Z: int32(wp.Z),
        })
    }
    var origin *state.WalkPoint
    if l.segmentStart.X != 0 || l.segmentStart.Y != 0 {
        origin = &state.WalkPoint{
            X: int32(l.segmentStart.X),
            Y: int32(l.segmentStart.Y),
            Z: int32(l.segmentStart.Z),
        }
    }
    var dest *state.WalkPoint
    if l.segmentDest.X != 0 || l.segmentDest.Y != 0 {
        dest = &state.WalkPoint{
            X: int32(l.segmentDest.X),
            Y: int32(l.segmentDest.Y),
            Z: int32(l.segmentDest.Z),
        }
    }

    return &state.WalkPlan{
        Origin: origin,
        Points: pts,
        Index:  min(l.wpIndex, len(pts)-1),
        Dest:   dest,
        // The mesh search contract rides the plan (the repro contract of the
        // 3D pathfind link); every plan of the loop is a mesh answer.
        Search: l.publishedSegmentSearch(),
    }
}

// publishedSegmentSearch returns the search contract of the current segment:
// the mesh answer of startWalkSegmentSearch carries it.
func (l *Loop) publishedSegmentSearch() *state.WalkSearch {
    return l.segmentSearch
}

// engageRadiusFor picks the approach distance of an attack by the weapon in
// hand: a bow shoots from the weapon range (~500 units), melee closes.
func (l *Loop) engageRadiusFor() float64 {
    if l.bowEquipped() {
        return userBowEngageRadius
    }

    return userEngageRadius
}

// pickApproachStopFor is where the fresh-pick approach click ends: a bow
// keeps its weapon range, melee walks up to the swing contact.
func (l *Loop) pickApproachStopFor() float64 {
    if l.bowEquipped() {
        return userBowEngageRadius
    }

    return pickApproachStopRadius
}

// stallRadiusFor picks the chase stall boundary by the weapon: a bow fight
// shooting inside the weapon range is progress, not a stalled chase.
func (l *Loop) stallRadiusFor() float64 {
    if l.bowEquipped() {
        return userBowStallRadius
    }

    return userEngageRadius
}

// bowEquipped reports whether the paperdoll weapon is a bow: the attack order
// and the chase stall watchdog then use the ranged radii.
func (l *Loop) bowEquipped() bool {
    for _, item := range l.tracker.InventoryItems() {
        if !item.Equipped {
            continue
        }
        if stats, ok := npcdata.ItemGearStats(item.ItemID); ok &&
            stats.WeaponType == "BOW" {
            return true
        }
    }

    return false
}

// tickUserAttack forces the attack on the clicked object until the fight
// starts, then the server AI plays it out; a stalled chase is walked by the
// loop, a dead target hands control to the loot phase.
//
//nolint:cyclop,funlen,gocognit // the user attack decision tree
func (l *Loop) tickUserAttack(now time.Time) {
    if l.userTarget == 0 {
        l.resumeAuto()

        return
    }
    if !l.tracker.ObjectAlive(l.userTarget) {
        if l.autonomous {
            // The kill drops loot around the corpse: the looting phase picks
            // it up.
            l.phase = phaseLoot
        } else {
            l.phase = phaseIdle
        }
        l.lootID = 0
        l.userKind = ""
        l.target = 0
        l.clearBlindRecovery()
        l.logf("Hunt: manual target %d died or vanished",
            l.userTarget)

        return
    }
    x, y, z, ok := l.tracker.ObjectPosition(l.userTarget)
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !ok || !selfOK {
        if now.Sub(l.userStart) > userAttackTimeout {
            l.resumeAuto()
        }

        return
    }
    dist := math.Hypot(float64(x-selfX), float64(y-selfY))
    // A bow shoots from the weapon range: the forced request starts the shot
    // immediately, a melee distance walk would only delay it.
    engageRadius := l.engageRadiusFor()
    stallRadius := l.stallRadiusFor()
    fighting := l.tracker.SelfFighting(l.userTarget)
    if fighting || l.tracker.SelfWalking() {
        // A running fight or walk refreshes the deadline: a long manual fight
        // never hands control back mid swing.
        l.userStart = now
    }
    if fighting && dist > stallRadius &&
        !l.chaseProgress(&l.userLastDist, &l.userDistAt, dist, now) {
        // The server chase stalled with the target far away: walk toward the
        // target instead of trusting it.
        l.logf("Hunt: manual attack chase stalled at %d "+
            "units, walking to the target", int(dist))
        fighting = false
    }
    if fighting {
        // The fight runs: the swings and chase steps keep the engagement
        // fresh, the death branch takes over on the next tick.
        return
    }
    if now.Sub(l.userStart) > userAttackTimeout {
        l.logf("Hunt: manual attack never engaged, " +
            "resuming the hunt")
        l.resumeAuto()

        return
    }
    if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < selectPeriod {
        return
    }
    l.userMoveAt = now
    if l.tracker.SelfTargetID() != l.userTarget {
        // The first request selects the target (and starts the server chase).
        if err := l.game.AttackTarget(l.userTarget); err != nil {
            l.logf("Hunt: manual attack failed: %v", err)
        }

        return
    }
    if dist > engageRadius {
        // Selected but out of engage range: the walk request works where the
        // AI chase stalls.
        if !l.tracker.SelfWalking() {
            if err := l.game.WalkTo(x, y, z); err != nil {
                l.logf("Hunt: manual attack walk failed: %v", err)
            }
        }

        return
    }
    if err := l.game.AttackTarget(l.userTarget); err != nil {
        l.logf("Hunt: manual attack failed: %v", err)
    }
}

// tickUserPickup walks to the clicked ground item and picks it up: a vanished
// item ends the manual phase; the walk and click follow the autonomous looting
// approach radius.
func (l *Loop) tickUserPickup(now time.Time) {
    item, ok := l.tracker.GroundItemByID(l.userTarget)
    if l.userTarget == 0 || !ok {
        l.logf("Hunt: manual pickup of %d finished, "+
            "resuming the hunt", l.userTarget)
        l.resumeAuto()

        return
    }
    if now.Sub(l.userStart) > userPickupTimeout {
        l.logf("Hunt: manual pickup timed out, " +
            "resuming the hunt")
        l.resumeAuto()

        return
    }
    if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < selectPeriod {
        return
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        l.resumeAuto()

        return
    }
    l.userMoveAt = now
    dist := math.Hypot(float64(item.X-selfX), float64(item.Y-selfY))
    if dist > lootApproachRadius {
        if err := l.game.WalkTo(item.X, item.Y, item.Z); err != nil {
            l.logf("Hunt: manual pickup walk failed: %v", err)
        }

        return
    }
    if err := l.game.PickupItem(item); err != nil {
        l.logf("Hunt: manual pickup failed: %v", err)
    }
}

// chaseProgress samples the distance to the chase target once per progress
// window and reports whether the chase still closes in.
func (l *Loop) chaseProgress(
    sampler *float64, at *time.Time, dist float64, now time.Time,
) bool {
    if at.IsZero() || now.Sub(*at) >= chaseProgressWindow {
        progressed := dist <= *sampler-chaseProgressStep
        *sampler = dist
        *at = now

        return progressed
    }

    return true
}

// resetChaseSamples drops the chase progress state: a new command or target
// restarts the sampling.
func (l *Loop) resetChaseSamples() {
    l.userLastDist = 0
    l.userDistAt = time.Time{}
    l.engLastDist = 0
    l.engDistAt = time.Time{}
}

// resumeAuto returns the loop to the autonomous hunting: the engage phase
// re-selects a target (or walks back into the zone after a manual action
// led outside of it).
func (l *Loop) resumeAuto() {
    if l.autonomous {
        l.phase = phaseEngage
    } else {
        // A manual only session has no hunt to resume: it waits in the idle
        // phase for the next web command.
        l.phase = phaseIdle
    }
    l.userKind = ""
    l.userTarget = 0
    l.userRedirect = false
    l.userDeadline = time.Time{}
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.engageAt = time.Time{}
    l.tracker.ClearWalkPlan()
}
