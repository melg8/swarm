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

// Manual command handling of the hunt loop: the web UI queues commands
// on the bot tracker (state.Bot.PushCommand) and the loop turns them
// into world actions. The one shot inventory commands (useItem, drop)
// execute immediately, the movement commands (move, attack, pickup)
// switch the loop into the phaseUser manual mode that overrides the
// autonomous hunting until the action completes or times out.

// Timing and threshold constants of the manual mode.
const (
    // userArriveRadius is the distance at which a manual move counts
    // as arrived (the server stops creatures a collision radius short
    // of the destination anyway).
    userArriveRadius = 60.0
    // userMoveTimeout bounds one manual walk: a click that never
    // arrives (blocked, too far, interrupted) returns control to the
    // autonomous hunt.
    userMoveTimeout = 90 * time.Second
    // userPickupTimeout bounds one manual pickup, the same span the
    // autonomous looting allows for the farthest items.
    userPickupTimeout = 30 * time.Second
    // userAttackTimeout bounds a manual attack that never starts the
    // fight (an unreachable or protected target).
    userAttackTimeout = 30 * time.Second
    // userApproachRadius is the geodata search goal of a long manual
    // move: the walk ends within this 3D distance of the clicked point,
    // so a click onto a shop interior cell or a walled structure still
    // lands on the reachable deck around it instead of routing through
    // the water below.
    userApproachRadius = 150.0
    // inventoryConfirmTimeout bounds how long one inventory action
    // waits for its server confirmation before the next command fires
    // anyway. The gate normally releases as soon as the tracker
    // observed the effect of the previous action (the equipped flag
    // flipped, the count changed, the item vanished), so a swap pair
    // continues at the speed the server actually processes it; the
    // timeout only rescues a refused request from blocking the queue.
    // The UseItem flood protector of this server build is disabled
    // (FloodProtectorUseItemInterval = 0, retail matching), so once
    // the packet race is serialized nothing on the server side rate
    // limits the pair.
    inventoryConfirmTimeout = 600 * time.Millisecond
)

// consumeUserCommands drains the command queue of the bot and applies
// every entry. One shot inventory commands execute right away (gated
// on the server confirmation of the previous one, see
// gateInventoryCommand), the movement commands (move, attack,
// pickup) reprogram the manual phase (the newest command wins).
// Deferred commands retry first so the gate never reorders a swap
// pair.
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

// flushDeferredCommands retries the inventory commands that were
// deferred by the spacing gate, oldest first. The gate re-checks every
// entry, so a still gapped command stays deferred.
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

// gateInventoryCommand defers one command when it is an inventory
// action that would race an in flight one on the server: the Mobius
// packet executor runs every client packet as its own thread pool
// task, so two requests racing on the same item or the same paperdoll
// slots can interleave their reads and writes and cancel each other.
// The gate holds the newcomer until the tracker observed the effect
// of the racing action (or its confirmation window expired), so the
// pair continues as fast as the server actually processes it - not
// on a fixed pause. The deferred list preserves the pair order of a
// swap (the older command was already sent, this one waits for its
// server game tick).
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

// pendingInventory is one in flight inventory action waiting for its
// server confirmation: the item state as it was when the request left
// and the paperdoll slots the server write set of the request touches
// (the equip landing plus the pieces it displaces, see
// gear.ServerWriteSlots).
type pendingInventory struct {
    equip bool
    count int32
    at    time.Time
    slots []gear.Slot
}

// inventoryItemAllowed reports whether a new inventory action on the
// item may go out: no in flight action exists for the same item (a
// second request would toggle the first one's effect back) and no in
// flight action writes a paperdoll slot the new request touches (the
// concurrent packet tasks of the Mobius executor would race on the
// slot). The confirmed and expired pendings prune on the check: the
// confirmation releases the item at the speed the server actually
// applies the flips, the timeout only rescues a refused request from
// blocking the queue forever.
func (l *Loop) inventoryItemAllowed(objectID int32, slots []gear.Slot) bool {
    now := time.Now()
    for pendingID, pending := range l.pendingActions {
        if l.pendingInventoryConfirmed(pendingID, pending) ||
            now.Sub(pending.at) >= inventoryConfirmTimeout {
            delete(l.pendingActions, pendingID)

            continue
        }
        if pendingID == objectID {
            return false
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

// prunePendingActions drops the in flight actions the tracker already
// confirmed or whose confirmation window expired: the map holds only
// the requests that still gate the new ones.
func (l *Loop) prunePendingActions(now time.Time) {
    for pendingID, pending := range l.pendingActions {
        if l.pendingInventoryConfirmed(pendingID, pending) ||
            now.Sub(pending.at) >= inventoryConfirmTimeout {
            delete(l.pendingActions, pendingID)
        }
    }
}

// pendingInventoryConfirmed checks the tracked inventory for the
// effect of one in flight action: any change of the equipped flag,
// the stack count or the existence of the item means the server
// processed the request.
func (l *Loop) pendingInventoryConfirmed(
    objectID int32, pending pendingInventory,
) bool {
    item, ok := l.tracker.InventoryItemState(objectID)
    if !ok {
        // Vanished: consumed by the request.
        return true
    }

    return item.Equipped != pending.equip ||
        item.Count != pending.count
}

// applyUserCommand turns one queued web command into world action.
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

// userSay sends one chat message through the character. The command is
// a one shot request like the inventory actions: the web UI validated
// the message against the Say2 refusals already, the packet validates
// itself again (an invalid chat would disconnect the session).
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

// markInventoryAction records one in flight inventory action: the
// item state as it was when the request left plus the paperdoll slots
// the server write set of the request touches (see
// gear.ServerWriteSlots). The gate watches the tracker for the actual
// server effect per item (see pendingInventoryConfirmed) and holds
// back only the requests that would race the write set.
func (l *Loop) markInventoryAction(objectID int32) {
    item, ok := l.tracker.InventoryItemState(objectID)
    pending := pendingInventory{
        equip: false,
        count: 0,
        at:    time.Now(),
        slots: gear.ServerWriteSlots(l.equipment(), objectID),
    }
    if ok {
        pending.equip = item.Equipped
        pending.count = item.Count
    }
    l.pendingActions[objectID] = pending
}

// userUseItem executes the equip/unequip toggle of one item right
// away: the packet is a one shot request, the server applies or
// refuses it by itself (the UseItem flood protector of this build
// is disabled, the confirmation gate paces the pairs).
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

// userDrop drops inventory items at the feet of the character: the
// server only accepts drops within 150 units of the player, so the
// current character position is the drop point.
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

// userDestroy destroys inventory items without dropping them: the
// trash target of the equipment widget sends it. The server (see
// RequestDestroyItem.runImpl) splits the stack by the count and even
// unequips an equipped item before destroying it, so no extra request
// is needed.
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

// userMovement switches the loop into the manual phase for a move,
// attack or pickup click of the map. The manual phase overrides the
// autonomous hunting: an active town trip is cancelled, the deleveling
// keeps running (its guard walk must finish for the level to drop) and
// refuses the command instead.
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
    l.userStart = time.Now()
    l.userMoveAt = time.Time{}
    // The new command redirects a walk that is already running: the
    // next tick re-issues the walk request at once instead of
    // waiting for the old server walk to finish (the server replaces
    // the destination of a running walk with the next move request).
    l.userRedirect = true
    l.userWaypoints = nil
    l.userSearch = nil
    l.userWpIndex = 0
    l.userPathTried = false
    l.userFrameOffset = 0
    // The walk plan origin: where the character stood when the click
    // arrived (the dump prints the whole walk from it). Unknown
    // positions keep the zero sentinel and publish no origin.
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

// tickUser advances the manual phase: it re-issues the action at the
// player action cadence (the Mobius flood protector allows one action
// per second) until it completes, then the autonomous hunting resumes.
// A sitting character stands up first: the server refuses every move,
// attack and pickup request of a sitting session with a bare
// ActionFailed, so walking straight into the request loop leaves the
// bot sitting through an endless stream of refusals (the softlock of
// a rest interrupted by a manual click).
func (l *Loop) tickUser() {
    now := time.Now()
    if l.tracker.SelfSitting() && l.userKind != "" {
        // The stand request shares the pending transition gate with
        // the rest logic (never a double toggle); the walk starts on
        // a later tick once the ChangeWaitType broadcast confirms the
        // standing.
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

// tickUserMove walks to the clicked point until the character arrives
// or the walk times out. The request repeats only when the walk
// actually stalled: the server pathfinds and runs the walk on its own,
// and re-clicking every second would restart its path search and slow
// the walk down. A stall shows up as a stopped character (the server
// broadcasts the stop) or a silent moving flag (a lost stop packet).
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
        // Every manual move plans through the navigator (the owner
        // request: the map double click IS the pathfind call - the
        // planned waypoints publish into the walk plan view and the
        // state dump exactly like the bot's own planned walks, so a
        // stuck segment reads at a glance). A failed or missing path
        // falls through to the direct server routed walk.
        l.userPathTried = true
        l.planUserWalk(selfX, selfY, selfZ)

        return
    }
    if l.tracker.SelfWalking() && !l.userRedirect {
        // The walk is running toward the manual target: do not
        // restart the server side path.
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

// planUserWalk computes the mesh path of one long manual move. The
// exact search runs first (the owner clicked the point - the walk
// must arrive at it): the approach ring of the NPC segments is the
// fallback only, because the ring can catch an early corridor
// polygon and end the plan short - the temple entrance round of the
// owner report (the doorway polygon 147 units short of the clicked
// interior cell inside the 150 ring) held the walk at the door
// forever. A failed exact search (the clicked point on ground the
// mesh does not reach) falls back to the approach corridor, a
// missing tile or a bare not found leaves the direct server routed
// walk. The result becomes the segment plan the follower walks one
// server accepted segment at a time; a partial corridor (the
// destination unreachable under the filter) walks the closest
// reachable point instead.
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
    // The manual plan publishes its search contract (the approach
    // radius of the answer - zero for the exact destination search,
    // the user approach radius of the fallback corridor - and no
    // bans): the 3D pathfind link rebuilds the very search instead
    // of a lookalike.
    l.userSearch = &state.WalkSearch{Approach: approach,
        Avoid: nil}
    // The manual walk plan opens with the same frame measurement as
    // every fresh plan (see click_frame.go): the route's first
    // waypoint is the character's own cell resolved on the pack, its
    // z against the server vouched standing z is the vintage shift
    // the follower's clicks ride. A swimming character measures no
    // shift: the swim z against the mesh floor is geometry, not a
    // pack disagreement.
    l.userFrameOffset = 0
    if !l.navigator.OverWater(
        float64(selfX), float64(selfY), int16(selfZ)) {
        l.userFrameOffset = measureFrameOffset(
            selfZ, result.Waypoints[0].Z)
    }
    l.userMoveAt = time.Time{}
    l.userRefusalVariants = 0
    l.cursorEscapes = 0
    l.cursorEscape = cursorEscapeState{} //nolint:exhaustruct_v5 // zero reset
    l.logf("Hunt: manual walk path planned: %d waypoints, "+
        "%.0f units (%.2fs search)", len(result.Waypoints),
        result.Length, result.Duration.Seconds())
}

// userSegmentRefused reports whether the server answered the last manual
// walk click with ActionFailed while the character stood still: the
// same sent click attribution the town walk follower applies (see
// refusalEvidence - the one byte refusal answer is the only online
// channel that names a server side move refusal, and the answer
// carries no request identity, so the correlation skips the answers
// that belong to a non walk request sent between the click and the
// refusal).
func (l *Loop) userSegmentRefused() bool {
    if l.userMoveAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    return failedAt.After(l.userMoveAt) &&
        failedAt.Sub(l.userMoveAt) <= refusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.userMoveAt, failedAt)
}

// sendUserVariedAim answers a refused manual walk click by varying
// the aim at the current waypoint: the refusal of the server is
// target specific - the Bresenham raster of a shorter prefix or a
// sideways offset of the same waypoint often validates where the
// plain aim bounced (the temple entrance porch: the straight line to
// the interior clipped the door frame, the east sideways aim crossed
// the opening). The variants ride the same server frame transport
// and the same offline gates as the planned click (the click
// validation port on the line, the geometry inside the move limit)
// and share the refusalVariantTarget ladder of the town walk. It
// reports whether a variant was sent.
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

// beginUserCursorKeyEscape hands a refused manual segment to the cursor
// key escape: the keyboard mode 0 arm plus the claimed
// ValidatePosition stream the server follows without any click
// validation (the 2026-09-14 15:10 report proved a server that
// answers NO mouse click from a cell still walks the arrow key
// claims - the same recovery the town walk follower owns). The
// claimed ladder marches the planned waypoint line in run speed
// strides - the claims follow the plan's own ground, never a
// straight cut the planner did not draw. It reports
// whether the escape armed.
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

// followUserWaypoints walks the planned segments of a long manual move:
// every waypoint gets a ground click walk, segments longer than the server
// move limit split into straight intermediate points (the smoothing
// keeps the line of sight of every segment), and a waypoint the character
// already passed ON THE ROUTE skips ahead (the projection pass test of
// the town walker - a character beside the route keeps targeting the
// waypoint it missed). The walk ends when the last waypoint is
// reached or the manual deadline passes; a segment that stalls (the server
// stopped the character short) re-issues at the walk request period.
//
//nolint:cyclop,funlen // the follower ladder is one linear decision train
func (l *Loop) followUserWaypoints(
    now time.Time, selfX int32, selfY int32, selfZ int32,
) {
    for l.userWpIndex < len(l.userWaypoints) {
        if waypointArrived(l.userWaypoints, l.userWpIndex,
            selfX, selfY, selfZ, l.userFrameOffset, waypointArriveDist) {
            l.userWpIndex++
            l.userMoveAt = time.Time{}
            l.userRefusalVariants = 0

            continue
        }
        // Not reached: skip it only when the character already
        // passed it on the route towards the next waypoint.
        if l.userWpIndex+1 < len(l.userWaypoints) &&
            waypointPassed(l.userWaypoints[l.userWpIndex],
                l.userWaypoints[l.userWpIndex+1], selfX, selfY) {
            l.userWpIndex++
            l.userMoveAt = time.Time{}
            l.userRefusalVariants = 0

            continue
        }

        break
    }
    if l.userWpIndex >= len(l.userWaypoints) {
        l.logf("Hunt: manual walk arrived (path done)")
        l.resumeAuto()

        return
    }
    if now.Sub(l.userStart) > userMoveTimeout {
        l.logf("Hunt: manual walk timed out, resuming the hunt")
        l.resumeAuto()

        return
    }
    // The cursor key escape owns the walk while it runs: the claim
    // ladder drives the character past the refusing ground (the
    // server follows the claimed ValidatePosition stream without any
    // click validation), the click machinery stays down until the
    // escape ends - the same ownership the town walk follower
    // applies (the 2026-09-14 15:10 report: a server that answers NO
    // click from a cell still walks the arrow key claims).
    if l.cursorEscape.armed {
        l.driveCursorKeyEscape(now, selfX, selfY)

        return
    }
    if l.tracker.SelfWalking() && !l.userRedirect {
        // The current segment is running: do not restart the server path.
        return
    }
    if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < walkRequestPeriod {
        return
    }
    // The refusal ladder of the manual walk: the server answered the
    // last click of this segment with ActionFailed while the character
    // stood still. The plain follower re-clicked the same aim every
    // period and the same refusal bounced forever (the temple
    // entrance round: the server stopped its own walk at the door
    // frame and refused every straight re-click from the porch while
    // the character stood there for the whole walk window). The
    // ladder first varies the aim (the refusal is target specific -
    // a shorter prefix or a sideways offset validates where the
    // plain aim bounced) and hands the segment to the cursor key escape
    // once the variants spent: the claims walk without any click
    // validation, so the ground no click leaves is still walkable.
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
    wp := l.userWaypoints[l.userWpIndex]
    dx := wp.X - float64(selfX)
    dy := wp.Y - float64(selfY)
    dist := math.Hypot(dx, dy)
    // The click z rides the server frame transport of the manual
    // plan (see click_frame.go): the mesh height plus the measured
    // vintage shift - the raw mesh z names the wrong layer wherever
    // the packs disagree about a surface's absolute height.
    wpZ := anchorZToServerFrame(wp.Z, l.userFrameOffset)
    moveX, moveY, moveZ := wp.X, wp.Y, wpZ
    if dist > maxMoveDistance {
        // Split the segment into a straight intermediate point: the
        // smoothing verified the whole segment, the server only
        // gets the short piece it accepts.
        scale := maxMoveDistance / dist
        moveX = float64(selfX) + dx*scale
        moveY = float64(selfY) + dy*scale
        moveZ = float64(selfZ) + (wpZ-float64(selfZ))*scale
    }
    l.userRedirect = false
    l.userMoveAt = now
    if err := l.game.WalkTo(
        int32(moveX), int32(moveY), int32(moveZ)); err != nil {
        l.logf("Hunt: manual walk failed: %v", err)
    }
}

// publishWalkPlan refreshes the walk plan view of the web UI: while a
// walk runs (a manual move, a town trip segment or a deleveling guard
// walk), the full plan of the current segment publishes - the planning
// origin, every planned waypoint with the follower cursor and the
// final destination - so the map draws the planned line against the
// live character position and the state dump reads the whole walk at
// a glance. Every other state of the loop clears the plan; the tracker
// also expires it on its own, so an abrupt exit never leaves a stale
// line.
func (l *Loop) publishWalkPlan() {
    plan := l.activeWalkPlan()
    if plan == nil {
        l.tracker.ClearWalkPlan()

        return
    }
    l.tracker.SetWalkPlan(*plan)
}

// activeWalkPlan returns the walk plan of the segment the loop is
// currently following: the planning origin, the full waypoint list,
// the waypoint the follower currently heads to and the final
// destination. Returns nil when the loop is not walking a planned
// path right now. The manual move plan carries the clicked
// destination; the town trip and deleveling plans carry the geodata
// waypoints to their target.
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

// userWalkPlan builds the walk plan of a manual move: the position of
// the character at the click (the origin), the full geodata waypoints
// of the planned walk (a direct walk carries the clicked destination
// alone), the follower cursor and the clicked destination. The
// destination stays its own field even when the waypoints carry it -
// the map destination marker pins the click itself.
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

// geodataWalkPlan builds the walk plan of a town trip or a deleveling
// guard walk: the planning origin (the position startWalkSegment planned
// from, the point where we wanted to go), the full waypoint list
// (shared l.waypoints slice, l.wpIndex cursor) and the segment destination
// last. startWalkSegment arms l.segmentDest with the destination it planned
// the walk to (the merchant spawn, the farm spot, the guard spawn), so
// the map can draw the final target even when the smoothing collapsed
// it into the last waypoint. Returns nil when the loop is between
// segments (no waypoints, no destination).
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
        // The mesh search contract rides the plan (the repro contract
        // of the 3D pathfind link); every plan of the loop is a mesh
        // answer - the walk always follows routes.
        Search: l.publishedSegmentSearch(),
    }
}

// publishedSegmentSearch returns the search contract of the current segment:
// the mesh answer of startWalkSegmentSearch carries it (every walk plan
// of the loop is a mesh answer - the walk always follows routes).
func (l *Loop) publishedSegmentSearch() *state.WalkSearch {
    return l.segmentSearch
}

// engageRadiusFor picks the approach distance of an attack request
// by the weapon in hand: a bow user shoots from the weapon range
// (the Mobius bow attack range is ~500 units), a melee fighter
// closes to the swing distance.
func (l *Loop) engageRadiusFor() float64 {
    if l.bowEquipped() {
        return userBowEngageRadius
    }

    return userEngageRadius
}

// stallRadiusFor picks the chase stall boundary by the weapon in
// hand: a bow fight stands and shoots inside the weapon range and
// closes no chase distance while doing it, which is progress, not a
// stalled chase.
func (l *Loop) stallRadiusFor() float64 {
    if l.bowEquipped() {
        return userBowStallRadius
    }

    return userEngageRadius
}

// bowEquipped reports whether the paperdoll weapon is a bow: a bow
// user engages and fights at the weapon range, not at the melee
// distance, so the attack order and the chase stall watchdog use the
// ranged radii for it.
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

// tickUserAttack forces the attack on the clicked object until the
// fight starts, then the fight plays out under the server AI. The
// first request selects the target, the repeated request forces the
// attack; a target beyond melee range is chased by the server AI, and
// when that chase stalls (the Mobius path search gives up on some
// routes while the engagement keeps looking fresh) the loop walks the
// stretch itself. A dead or vanished target hands control to the
// looting phase so the drops of a killed mob are picked up; a fight
// that never starts times out and the autonomous hunting resumes.
// Pre-consolidation phase debt; the hunt loop cleanup is planned
// (docs/quality_review_and_agent_prompts.md P07).
//
//nolint:cyclop,funlen,gocognit // the user attack decision tree
func (l *Loop) tickUserAttack(now time.Time) {
    if l.userTarget == 0 {
        l.resumeAuto()

        return
    }
    if !l.tracker.ObjectAlive(l.userTarget) {
        if l.autonomous {
            // The kill drops loot around the corpse: hand control to
            // the looting phase so the drops are picked up.
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
    // A bow in hand shoots from the weapon range: the forced request
    // starts the shot immediately, the melee distance walk would only
    // delay it (see the userBow radii).
    engageRadius := l.engageRadiusFor()
    stallRadius := l.stallRadiusFor()
    fighting := l.tracker.SelfFighting(l.userTarget)
    if fighting || l.tracker.SelfWalking() {
        // A running fight or walk refreshes the manual deadline: a
        // long fight under manual control never hands control back
        // mid swing.
        l.userStart = now
    }
    if fighting && dist > stallRadius &&
        !l.chaseProgress(&l.userLastDist, &l.userDistAt, dist, now) {
        // The server chase stalled with the target far away: walk
        // toward the target instead of trusting the stuck chase.
        l.logf("Hunt: manual attack chase stalled at %d "+
            "units, walking to the target", int(dist))
        fighting = false
    }
    if fighting {
        // The fight is running right now: the swings and the chase
        // steps keep the engagement fresh, the death branch takes
        // over on the next tick.
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
        // The first request selects the target (and starts the
        // server chase).
        if err := l.game.AttackTarget(l.userTarget); err != nil {
            l.logf("Hunt: manual attack failed: %v", err)
        }

        return
    }
    if dist > engageRadius {
        // Selected but out of engage range: approach the target - the
        // walk request works where the AI chase stalls.
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

// tickUserPickup walks to the clicked ground item and picks it up. A
// vanished item (picked up, despawned) ends the manual phase; the
// walk plus click follow the same approach radius as the autonomous
// looting.
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

// chaseProgress samples the distance to the chase target once per
// progress window and reports whether the chase still closes in: a
// healthy chase covers well past the step distance per window, a stall
// (the Mobius path search gave up while the engagement stays fresh
// through stuck chase packets) needs the fallback walk.
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

// resetChaseSamples drops the chase progress state: a new command or
// a new target restarts the sampling.
func (l *Loop) resetChaseSamples() {
    l.userLastDist = 0
    l.userDistAt = time.Time{}
    l.engLastDist = 0
    l.engDistAt = time.Time{}
}

// resumeAuto returns the loop to the autonomous hunting: the engage
// phase re-selects a target (or walks back into the zone when the
// manual action led outside of it).
func (l *Loop) resumeAuto() {
    if l.autonomous {
        l.phase = phaseEngage
    } else {
        // A manual only session has no hunt to resume: the loop
        // waits in the idle phase for the next web command.
        l.phase = phaseIdle
    }
    l.userKind = ""
    l.userTarget = 0
    l.userRedirect = false
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.engageAt = time.Time{}
    l.tracker.ClearWalkPlan()
}
