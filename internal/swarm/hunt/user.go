// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"time"

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
)

// consumeUserCommands drains the command queue of the bot and applies
// every entry. One shot inventory commands execute right away, the
// movement commands reprogram the manual phase (the newest command
// wins).
func (l *Loop) consumeUserCommands() {
	for {
		select {
		case cmd := <-l.tracker.Commands():
			l.applyUserCommand(cmd)
		default:
			return
		}
	}
}

// applyUserCommand turns one queued web command into world action.
func (l *Loop) applyUserCommand(cmd state.Command) {
	switch cmd.Kind {
	case state.CommandUseItem:
		l.userUseItem(cmd)
	case state.CommandDrop:
		l.userDrop(cmd)
	case state.CommandMove, state.CommandAttack, state.CommandPickup:
		l.userMovement(cmd)
	default:
		l.logger.Printf("Hunt: unknown user command %q", cmd.Kind)
	}
}

// userUseItem executes the equip/unequip toggle of one item right
// away: the packet is a one shot request, the server applies or
// refuses it by itself (the flood protector allows one use per
// second).
func (l *Loop) userUseItem(cmd state.Command) {
	if cmd.ObjectID == 0 {
		return
	}
	l.logger.Printf("Hunt: user command: use item %d", cmd.ObjectID)
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
	if err := l.game.DropItem(cmd.ObjectID, cmd.Count, x, y, z); err != nil {
		l.logger.Printf("Hunt: drop item failed: %v", err)
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
	l.target = 0
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
func (l *Loop) tickUser() {
	now := time.Now()
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
// or the walk times out. The repeated MoveToLocation requests are the
// ground clicks of the official client: they keep the walk going when
// the server interrupted it (an obstacle, a hit).
func (l *Loop) tickUserMove(now time.Time) {
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		l.resumeAuto()

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
	if !l.userMoveAt.IsZero() && now.Sub(l.userMoveAt) < selectPeriod {
		return
	}
	l.userMoveAt = now
	if err := l.game.WalkTo(l.userX, l.userY, l.userZ); err != nil {
		l.logger.Printf("Hunt: manual walk failed: %v", err)
	}
}

// tickUserAttack forces the attack on the clicked object until the
// fight starts, then the fight plays out under the server AI. A dead
// or vanished target hands control to the looting phase so the drops
// of a killed mob are picked up (the loot phase falls straight back
// to the hunt when there is nothing around); a fight that never
// starts times out and the autonomous hunting resumes.
func (l *Loop) tickUserAttack(now time.Time) {
	if l.userTarget == 0 {
		l.resumeAuto()

		return
	}
	if !l.tracker.ObjectAlive(l.userTarget) {
		l.logger.Printf("Hunt: manual target %d died or vanished, looting",
			l.userTarget)
		l.phase = phaseLoot
		l.lootID = 0
		l.userKind = ""
		l.target = 0

		return
	}
	if l.tracker.SelfEngaged(l.userTarget) {
		// The fight is running: the death branch takes over on the
		// next tick.
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

// resumeAuto returns the loop to the autonomous hunting: the engage
// phase re-selects a target (or walks back into the zone when the
// manual action led outside of it).
func (l *Loop) resumeAuto() {
	l.phase = phaseEngage
	l.userKind = ""
	l.userTarget = 0
	l.target = 0
	l.lootID = 0
	l.engageAt = time.Time{}
}
