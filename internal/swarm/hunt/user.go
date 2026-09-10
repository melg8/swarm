// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"time"

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
	// userPathfindDistance is the straight line distance above which a
	// manual move switches to the geodata planner: the server side
	// pathfinder silently refuses far targets (observed stuck walks on
	// requests past a few thousand units), so long clicks follow the
	// bot planned waypoints instead, one server accepted leg at a time.
	userPathfindDistance = 2000.0
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
// action that would race the previous one on the server: the Mobius
// packet executor runs every client packet as its own thread pool
// task, so the unequip and the equip of one swap cancel each other
// when they land in the same burst. The gate holds the newcomer
// until the tracker observed the effect of the previous action, so
// the pair continues as fast as the server actually processes it -
// not on a fixed pause. The deferred list preserves the pair order
// of a swap (the older command was already sent, this one waits for
// its server game tick).
func (l *Loop) gateInventoryCommand(cmd state.Command) bool {
	switch cmd.Kind {
	case state.CommandUseItem, state.CommandDrop, state.CommandDestroy:
	default:
		return false
	}
	if l.inventoryGateOpen() {
		return false
	}
	l.userDeferred = append(l.userDeferred, cmd)
	l.logger.Printf("Hunt: user command %q waits for the item pace",
		cmd.Kind)

	return true
}

// inventoryGateOpen reports whether the previous inventory action is
// confirmed: its effect showed up in the tracked inventory, or the
// action is old enough that a refused request must not block the
// queue forever.
func (l *Loop) inventoryGateOpen() bool {
	if l.userPendingAt.IsZero() {
		return true
	}
	if time.Since(l.userPendingAt) >= inventoryConfirmTimeout {
		return true
	}

	return l.pendingInventoryConfirmed()
}

// pendingInventoryConfirmed checks the tracked inventory for the
// effect of the previous action: any change of the equipped flag,
// the stack count or the existence of the item means the server
// processed the request.
func (l *Loop) pendingInventoryConfirmed() bool {
	item, ok := l.tracker.InventoryItemState(l.userPendingItem)
	if !ok {
		// Vanished: consumed by the request.
		return true
	}

	return item.Equipped != l.userPendingEquip ||
		item.Count != l.userPendingCount
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
	case state.CommandZone:
		l.userZoneSelect(cmd.Count)
	case state.CommandMove, state.CommandAttack, state.CommandPickup:
		l.userMovement(cmd)
	default:
		l.logger.Printf("Hunt: unknown user command %q", cmd.Kind)
	}
}

// markInventoryAction records the pending confirmation of one
// inventory action: the item state as it was when the request left.
// The gate watches the tracker for the actual server effect (see
// pendingInventoryConfirmed).
func (l *Loop) markInventoryAction(objectID int32) {
	item, ok := l.tracker.InventoryItemState(objectID)
	l.userPendingItem = objectID
	l.userPendingEquip = item.Equipped
	l.userPendingCount = item.Count
	l.userPendingAt = time.Now()
	if !ok {
		// Unknown item (the request may remove it entirely): the
		// vanishing itself is the confirmation.
		l.userPendingEquip = false
		l.userPendingCount = 0
	}
}

// userUseItem executes the equip/unequip toggle of one item right
// away: the packet is a one shot request, the server applies or
// refuses it by itself (the UseItem flood protector of this build
// is disabled, the confirmation gate paces the pairs).
func (l *Loop) userUseItem(cmd state.Command) {
	if cmd.ObjectID == 0 {
		return
	}
	l.logger.Printf("Hunt: user command: use item %d", cmd.ObjectID)
	l.markInventoryAction(cmd.ObjectID)
	if err := l.game.UseItem(cmd.ObjectID); err != nil {
		l.logger.Printf("Hunt: use item failed: %v", err)
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
		l.logger.Printf("Hunt: user command: drop failed: no self position")

		return
	}
	l.logger.Printf("Hunt: user command: drop %d of item %d",
		cmd.Count, cmd.ObjectID)
	l.markInventoryAction(cmd.ObjectID)
	if err := l.game.DropItem(cmd.ObjectID, cmd.Count, x, y, z); err != nil {
		l.logger.Printf("Hunt: drop item failed: %v", err)
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
	l.logger.Printf("Hunt: user command: destroy %d of item %d",
		cmd.Count, cmd.ObjectID)
	l.markInventoryAction(cmd.ObjectID)
	if err := l.game.DestroyItem(cmd.ObjectID, cmd.Count); err != nil {
		l.logger.Printf("Hunt: destroy item failed: %v", err)
	}
}

// userMovement switches the loop into the manual phase for a move,
// attack or pickup click of the map. The manual phase overrides the
// autonomous hunting: an active town trip is cancelled, the deleveling
// keeps running (its guard walk must finish for the level to drop) and
// refuses the command instead.
func (l *Loop) userMovement(cmd state.Command) {
	if l.phase == phaseDelevel {
		l.logger.Printf("Hunt: user command %q ignored while deleveling",
			cmd.Kind)

		return
	}
	if l.phase == phaseUser {
		l.logger.Printf("Hunt: user command: %s replaces the manual %s",
			cmd.Kind, l.userKind)
	} else if l.tripActive() {
		l.logger.Printf("Hunt: user command: %s cancels the town trip",
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
	l.userWpIndex = 0
	l.userPathTried = false
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
		l.logger.Printf("Hunt: user command: walking to %d %d %d",
			cmd.X, cmd.Y, cmd.Z)
	case state.CommandAttack:
		l.logger.Printf("Hunt: user command: attacking object %d",
			cmd.ObjectID)
	case state.CommandPickup:
		l.logger.Printf("Hunt: user command: picking up item %d",
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
		l.logger.Printf("Hunt: manual walk arrived (%d units left)",
			int(dist))
		l.resumeAuto()

		return
	}
	if now.Sub(l.userStart) > userMoveTimeout {
		l.logger.Printf("Hunt: manual walk timed out, resuming the hunt")
		l.resumeAuto()

		return
	}
	if dist >= userPathfindDistance && l.navigator != nil &&
		!l.userPathTried {
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
		l.logger.Printf("Hunt: manual walk failed: %v", err)
	}
}

// planUserWalk computes the geodata path of one long manual move. The
// search runs once per move (a failed or missing path leaves the
// direct server routed walk). The result becomes the leg plan the
// follower walks one server accepted leg at a time.
func (l *Loop) planUserWalk(selfX int32, selfY int32, selfZ int32) {
	from := pathfind.Vec3{
		X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
	}
	end := pathfind.Vec3{
		X: float64(l.userX), Y: float64(l.userY), Z: float64(l.userZ),
	}
	result, err := l.navigator.FindPathApproach(from, end, userApproachRadius)
	if err != nil {
		l.logger.Printf("Hunt: manual walk path search failed: %v", err)

		return
	}
	if result == nil || !result.Found || len(result.Waypoints) == 0 {
		l.logger.Printf("Hunt: no geodata path to %d %d, "+
			"walking by server routing", l.userX, l.userY)

		return
	}
	l.userWaypoints = result.Waypoints
	l.userWpIndex = 0
	l.userMoveAt = time.Time{}
	l.logger.Printf("Hunt: manual walk path planned: %d waypoints, "+
		"%.0f units (%.2fs search)", len(result.Waypoints),
		result.Length, result.Duration.Seconds())
}

// followUserWaypoints walks the planned legs of a long manual move:
// every waypoint gets a ground click walk, legs longer than the server
// move limit split into straight intermediate points (the smoothing
// keeps the line of sight of every leg), and waypoints the character
// already passed skip ahead. The walk ends when the last waypoint is
// reached or the manual deadline passes; a leg that stalls (the server
// stopped the character short) re-issues at the walk request period.
func (l *Loop) followUserWaypoints(
	now time.Time, selfX int32, selfY int32, selfZ int32,
) {
	for l.userWpIndex < len(l.userWaypoints) {
		wp := l.userWaypoints[l.userWpIndex]
		dist := waypointDistance(wp, selfX, selfY, selfZ)
		if dist > waypointArriveDist {
			if l.userWpIndex+1 < len(l.userWaypoints) {
				next := l.userWaypoints[l.userWpIndex+1]
				nextDist := waypointDistance(next, selfX, selfY, selfZ)
				if nextDist < dist {
					l.userWpIndex++
					l.userMoveAt = time.Time{}

					continue
				}
			}

			break
		}
		l.userWpIndex++
		l.userMoveAt = time.Time{}
	}
	if l.userWpIndex >= len(l.userWaypoints) {
		l.logger.Printf("Hunt: manual walk arrived (path done)")
		l.resumeAuto()

		return
	}
	if now.Sub(l.userStart) > userMoveTimeout {
		l.logger.Printf("Hunt: manual walk timed out, resuming the hunt")
		l.resumeAuto()

		return
	}
	if l.tracker.SelfWalking() && !l.userRedirect {
		// The current leg is running: do not restart the server path.
		return
	}
	if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < walkRequestPeriod {
		return
	}
	wp := l.userWaypoints[l.userWpIndex]
	dx := wp.X - float64(selfX)
	dy := wp.Y - float64(selfY)
	dist := math.Hypot(dx, dy)
	moveX, moveY, moveZ := wp.X, wp.Y, wp.Z
	if dist > maxMoveLeg {
		// Split the leg into a straight intermediate point: the
		// smoothing verified the whole segment, the server only
		// gets the short piece it accepts.
		scale := maxMoveLeg / dist
		moveX = float64(selfX) + dx*scale
		moveY = float64(selfY) + dy*scale
		moveZ = float64(selfZ)
	}
	l.userRedirect = false
	l.userMoveAt = now
	if err := l.game.WalkTo(
		int32(moveX), int32(moveY), int32(moveZ)); err != nil {
		l.logger.Printf("Hunt: manual walk failed: %v", err)
	}
}

// publishWalkPlan refreshes the walk plan view of the web UI: while a
// walk runs (a manual move, a town trip leg or a deleveling guard
// walk), the full plan of the current leg publishes - the planning
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

// activeWalkPlan returns the walk plan of the leg the loop is
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
	}
}

// geodataWalkPlan builds the walk plan of a town trip or a deleveling
// guard walk: the planning origin (the position startWalkLeg planned
// from, the point where we wanted to go), the full waypoint list
// (shared l.waypoints slice, l.wpIndex cursor) and the leg destination
// last. startWalkLeg arms l.legDest with the destination it planned
// the walk to (the merchant spawn, the farm spot, the guard spawn), so
// the map can draw the final target even when the smoothing collapsed
// it into the last waypoint. Returns nil when the loop is between
// legs (no waypoints, no destination).
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
	if l.legStart.X != 0 || l.legStart.Y != 0 {
		origin = &state.WalkPoint{
			X: int32(l.legStart.X),
			Y: int32(l.legStart.Y),
			Z: int32(l.legStart.Z),
		}
	}
	var dest *state.WalkPoint
	if l.legDest.X != 0 || l.legDest.Y != 0 {
		dest = &state.WalkPoint{
			X: int32(l.legDest.X),
			Y: int32(l.legDest.Y),
			Z: int32(l.legDest.Z),
		}
	}

	return &state.WalkPlan{
		Origin: origin,
		Points: pts,
		Index:  min(l.wpIndex, len(pts)-1),
		Dest:   dest,
	}
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
func (l *Loop) tickUserAttack(now time.Time) { //nolint:cyclop,funlen
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
		l.logger.Printf("Hunt: manual target %d died or vanished",
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
	fighting := l.tracker.SelfFighting(l.userTarget)
	if fighting || l.tracker.SelfWalking() {
		// A running fight or walk refreshes the manual deadline: a
		// long fight under manual control never hands control back
		// mid swing.
		l.userStart = now
	}
	if fighting && dist > userEngageRadius &&
		!l.chaseProgress(&l.userLastDist, &l.userDistAt, dist, now) {
		// The server chase stalled with the target far away: walk
		// toward the target instead of trusting the stuck chase.
		l.logger.Printf("Hunt: manual attack chase stalled at %d "+
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
		l.logger.Printf("Hunt: manual attack never engaged, " +
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
			l.logger.Printf("Hunt: manual attack failed: %v", err)
		}

		return
	}
	if dist > userEngageRadius {
		// Selected but out of melee range: approach the target - the
		// walk request works where the AI chase stalls.
		if !l.tracker.SelfWalking() {
			if err := l.game.WalkTo(x, y, z); err != nil {
				l.logger.Printf("Hunt: manual attack walk failed: %v", err)
			}
		}

		return
	}
	if err := l.game.AttackTarget(l.userTarget); err != nil {
		l.logger.Printf("Hunt: manual attack failed: %v", err)
	}
}

// tickUserPickup walks to the clicked ground item and picks it up. A
// vanished item (picked up, despawned) ends the manual phase; the
// walk plus click follow the same approach radius as the autonomous
// looting.
func (l *Loop) tickUserPickup(now time.Time) {
	item, ok := l.tracker.GroundItemByID(l.userTarget)
	if l.userTarget == 0 || !ok {
		l.logger.Printf("Hunt: manual pickup of %d finished, "+
			"resuming the hunt", l.userTarget)
		l.resumeAuto()

		return
	}
	if now.Sub(l.userStart) > userPickupTimeout {
		l.logger.Printf("Hunt: manual pickup timed out, " +
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
			l.logger.Printf("Hunt: manual pickup walk failed: %v", err)
		}

		return
	}
	if err := l.game.PickupItem(item); err != nil {
		l.logger.Printf("Hunt: manual pickup failed: %v", err)
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
