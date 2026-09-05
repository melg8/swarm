// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package hunt drives the automatic hunting behavior of a bot: attack the
// nearest attackable npc, pick up the loot it drops around the corpse and
// keep the inventory of the long living session working by destroying
// junk items when the slots or the weight run out.
package hunt

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// GameAPI abstracts the game actions the hunt loop needs. The
// connection.GameClient implements it.
type GameAPI interface {
	// AttackNearest selects the closest attackable npc and returns its
	// object id, zero when nothing was attackable. The Mobius server
	// answers the first request for a new target with MyTargetSelected.
	AttackNearest() (int32, error)
	// AttackTarget repeats the attack request for an already selected
	// target: the server resolves it to a forced attack and starts the
	// combat.
	AttackTarget(objectID int32) error
	// WalkTo makes the character walk to a world point, like a ground
	// click of the official client.
	WalkTo(x int32, y int32, z int32) error
	// PickupItem clicks a ground item to walk to it and pick it up.
	PickupItem(item state.LootItem) error
	// ActionSitStand toggles between sitting and standing.
	ActionSitStand() error
	// RestartAtVillage revives a dead character at the nearest village.
	RestartAtVillage() error
	// DestroyItem destroys inventory items.
	DestroyItem(objectID int32, count int32) error
}

// Timing and threshold constants of the hunt loop.
const (
	// tickPeriod is the decision cadence of the loop. The Mobius
	// PlayerActionFloodProtector accepts one player action per second,
	// so the actual requests stay rate limited by the engage periods
	// below; the short cadence only makes the state transitions (target
	// died, loot finished, health recovered) act within a quarter
	// second instead of a full one.
	tickPeriod = 250 * time.Millisecond
	// lootRadius is the distance around the character within drops are
	// picked up after a kill.
	lootRadius = 900.0
	// lootApproachRadius is the distance within which a ground item is
	// clicked instead of walked to: the server AI covers the last
	// stretch and executes the pickup.
	lootApproachRadius = 60.0
	// selectPeriod is the minimum pause between two nearest target
	// selections. The server accepts one player action per second
	// (PlayerActionFloodProtector), so selecting faster is pointless.
	selectPeriod = 1 * time.Second
	// reengageHealthPercent is the HP level above which the next target
	// is engaged immediately after a kill instead of resting. Below it
	// the loop waits for the natural regeneration to recover.
	reengageHealthPercent = 50.0
	// sitDownHealthPercent is the HP level below which the resting
	// character sits down: the sitting regeneration is much faster.
	sitDownHealthPercent = 30.0
	// standUpHealthPercent is the HP level at which a sitting character
	// stands up again and resumes the hunt.
	standUpHealthPercent = 90.0
	// restRetryPeriod guards the sit/stand toggle against confirmation
	// lag: the ChangeWaitType broadcast confirms each transition and a
	// repeat is only sent when the flip never happened (lost packet),
	// so a slow confirmation can never toggle the character back.
	restRetryPeriod = 3 * time.Second
	// deathRestartPeriod is the pause between village restart requests
	// of a dead character: the server keeps a short death delay and
	// refuses early revives, so the request retries until it lands.
	deathRestartPeriod = 5 * time.Second
	// huntReturnDistance is how close to the remembered death spot the
	// character must walk after a village revive before giving up on
	// the return (nothing attackable lives around the corpse).
	huntReturnDistance = 200.0
	// engageRetryPeriod is the pause between repeated forced attack
	// requests for the selected target. The server accepts one player
	// action per second (PlayerActionFloodProtector), so one second is
	// the fastest useful cadence.
	engageRetryPeriod = 1 * time.Second
	// pickupTimeout is how long one pickup attempt may take before the
	// item is skipped. It must cover the walk to the farthest item of
	// the loot radius plus the server side pickup.
	pickupTimeout = 20 * time.Second
	// pickupRetryDelay is how long a failed item stays skipped.
	pickupRetryDelay = 30 * time.Second
	// cleanupSlotPercent destroys junk at this inventory fill level.
	cleanupSlotPercent = 70.0
	// cleanupWeightPercent destroys junk at this weight level.
	cleanupWeightPercent = 75.0
	// destroyBatch is the number of junk items destroyed per cleanup.
	destroyBatch = 4
)

// phase is the coarse activity of the hunt loop.
type phase string

const (
	// phaseEngage walks to and attacks the current or nearest target.
	phaseEngage phase = "engage"
	// phaseLoot picks up the drops around the last kill.
	phaseLoot phase = "loot"
)

// Loop is the hunt state machine of one bot session.
type Loop struct {
	game          GameAPI
	tracker       *state.Bot
	logger        *log.Logger
	phase         phase
	target        int32
	lastHit       time.Time
	lootID        int32
	lootAt        time.Time
	lootMoveAt    time.Time
	skipped       map[int32]time.Time
	restActionAt  time.Time
	restActionSit bool
	restartAt     time.Time
	deathX        int32
	deathY        int32
	deathZ        int32
	returning     bool
}

// NewLoop creates the hunt loop for a connected game client.
func NewLoop(game GameAPI, tracker *state.Bot) *Loop {
	return &Loop{
		game:          game,
		tracker:       tracker,
		logger:        log.Default(),
		phase:         phaseEngage,
		target:        0,
		lastHit:       time.Time{},
		lootID:        0,
		lootAt:        time.Time{},
		lootMoveAt:    time.Time{},
		skipped:       make(map[int32]time.Time),
		restActionAt:  time.Time{},
		restActionSit: false,
		restartAt:     time.Time{},
		deathX:        0,
		deathY:        0,
		deathZ:        0,
		returning:     false,
	}
}

// Run drives the hunt loop until the context is done.
func (l *Loop) Run(ctx context.Context) {
	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.tick()
		}
	}
}

// tick advances the hunt state machine by one decision. The phase
// transitions fall through, so a kill switches into looting and the
// first pickup happens on the same tick.
func (l *Loop) tick() {
	if l.tracker.SelfDead() {
		l.recoverFromDeath()

		return
	}
	l.cleanupInventory()
	if l.phase == phaseEngage {
		l.engage()
	}
	if l.phase == phaseLoot {
		l.loot()
	}
}

// recoverFromDeath returns a dead character to the hunt: the village
// restart request is the death dialog choice of the official client and
// revives the character with restored vitals. The request retries until
// the revival lands (the server refuses it during the death delay), the
// stale target and loot references are dropped and the death spot is
// remembered so the loop can walk back to the hunting grounds.
func (l *Loop) recoverFromDeath() {
	now := time.Now()
	if l.restartAt.IsZero() {
		// The position of the first death observation is the corpse
		// spot; later retries must not overwrite it with the village
		// coordinates the character was revived at.
		if x, y, z, ok := l.tracker.SelfPosition(); ok {
			l.deathX, l.deathY, l.deathZ = x, y, z
			l.returning = true
		}
	}
	if !l.restartAt.IsZero() && now.Sub(l.restartAt) < deathRestartPeriod {
		return
	}
	l.restartAt = now
	l.target = 0
	l.lootID = 0
	l.phase = phaseEngage
	l.logger.Printf("Hunt: character died, restarting at the nearest village")
	if err := l.game.RestartAtVillage(); err != nil {
		l.logger.Printf("Hunt: village restart failed: %v", err)
	}
}

// returnToHunt walks the character back to its death spot after a
// village revive: the village itself has nothing attackable, so without
// the return walk a revived bot would idle there forever. The walk
// stops once attackable npcs show up again or the spot is reached.
func (l *Loop) returnToHunt(now time.Time) {
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	x, y, _, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	if math.Hypot(float64(x-l.deathX), float64(y-l.deathY)) < huntReturnDistance {
		l.returning = false
		l.logger.Printf("Hunt: returned to the hunting grounds")

		return
	}
	l.logger.Printf("Hunt: no prey in the village, walking back to the death spot")
	if err := l.game.WalkTo(l.deathX, l.deathY, l.deathZ); err != nil {
		l.logger.Printf("Hunt: walk back failed: %v", err)
	}
}

// engage attacks the nearest attackable npc while the current target
// lives, then switches to the loot phase. The Mobius AttackRequest has
// double click semantics: the first request selects the target (the
// MyTargetSelected answer), the repeated request for the already
// selected target triggers the forced attack. The loop therefore keeps
// re-requesting the target until the character is actually engaged in
// the fight, which the MoveToPawn/Attack/AutoAttackStart broadcasts
// confirm.
func (l *Loop) engage() {
	// Prefer the server view of the target while it lives: the
	// MyTargetSelected answer of the last attack request arrives
	// asynchronously, so the fresh value is read every tick. A stale
	// id of a dead or removed target must not be re-adopted: the
	// server never clears the selection of a corpse (only the next
	// selection replaces it), so blindly trusting it locked the
	// loop into an engage/loot ping-pong where the next target was
	// never selected.
	serverTarget := l.tracker.SelfTargetID()
	if serverTarget != 0 && l.tracker.ObjectAlive(serverTarget) {
		l.target = serverTarget
	}
	if l.target != 0 && !l.tracker.ObjectAlive(l.target) {
		l.logger.Printf("Hunt: target %d died, looting", l.target)
		l.target = 0
		l.phase = phaseLoot
		l.lootID = 0

		return
	}
	now := time.Now()
	if l.target == 0 {
		// Rest while the character is hurt: the regeneration is
		// faster out of combat and engaging with low HP risks
		// death. A sitting character stands up through the rest
		// logic once recovered. A character that is being hit right
		// now keeps fighting instead of sitting into the blows.
		hurt := l.tracker.SelfHealthPercent() < reengageHealthPercent
		if l.tracker.SelfSitting() || (hurt && !l.tracker.SelfUnderAttack()) {
			l.rest()

			return
		}
		if now.Sub(l.lastHit) < selectPeriod {
			return
		}
		target, err := l.game.AttackNearest()
		if err != nil {
			l.logger.Printf("Hunt: attack failed: %v", err)

			return
		}
		if target != 0 {
			l.target = target
			l.lastHit = now
			l.returning = false

			return
		}
		if l.returning {
			// Nothing attackable around (the village after a revive):
			// walk back to the death spot instead of idling.
			l.returnToHunt(now)
		}

		return
	}
	if l.tracker.SelfEngaged(l.target) {
		return
	}
	if now.Sub(l.lastHit) < engageRetryPeriod {
		return
	}
	if err := l.game.AttackTarget(l.target); err != nil {
		l.logger.Printf("Hunt: attack failed: %v", err)

		return
	}
	l.lastHit = now
}

// rest brings the resting character back to full health. Sitting
// accelerates the regeneration, so the character sits down below the
// sit threshold and stands up again once recovered. The sit/stand
// action is a server side toggle, so every transition is confirmed by
// the ChangeWaitType broadcast before the opposite one is ever sent.
func (l *Loop) rest() {
	now := time.Now()
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	hp := l.tracker.SelfHealthPercent()
	wantSit := false
	switch {
	case l.tracker.SelfSitting() && hp < standUpHealthPercent:
		// The sit is confirmed and the regeneration is running.
		return
	case l.tracker.SelfSitting():
		wantSit = false
	case hp < sitDownHealthPercent:
		wantSit = true
	default:
		l.logger.Printf("Hunt: resting, HP %.0f%% below %.0f%%",
			hp, reengageHealthPercent)

		return
	}
	if l.tracker.SelfSitting() == wantSit {
		return
	}
	if !l.restActionAt.IsZero() {
		if l.restActionSit == l.tracker.SelfSitting() {
			// The previous transition is confirmed, consume it.
			l.restActionAt = time.Time{}
		} else if now.Sub(l.restActionAt) < restRetryPeriod {
			// Confirmation still pending, never double toggle.
			return
		}
	}
	if wantSit {
		l.logger.Printf("Hunt: HP %.0f%% below %.0f%%, sitting down to regenerate",
			hp, sitDownHealthPercent)
	} else {
		l.logger.Printf("Hunt: HP %.0f%% recovered, standing up", hp)
	}
	if err := l.game.ActionSitStand(); err != nil {
		l.logger.Printf("Hunt: sit/stand action failed: %v", err)

		return
	}
	l.restActionAt = now
	l.restActionSit = wantSit
}

// loot picks up the ground items around the character until none is left
// within the loot radius, then hunts the next target. Farther items are
// approached with an explicit walk first so the character visibly runs
// toward the loot instead of trusting the click to start the whole
// approach.
func (l *Loop) loot() {
	item, ok := l.tracker.NearestGroundItemExcluding(lootRadius, l.skipped)
	if !ok {
		l.phase = phaseEngage
		l.target = 0

		return
	}
	now := time.Now()
	if item.ObjectID != l.lootID {
		l.lootID = item.ObjectID
		l.lootAt = now
		l.lootMoveAt = time.Time{}
	}
	if now.Sub(l.lootAt) > pickupTimeout {
		// The pickup did not finish: the item is protected or
		// unreachable. Skip it for a while and try the next one.
		l.skipped[item.ObjectID] = now.Add(pickupRetryDelay)
		l.lootID = 0
		l.logger.Printf("Hunt: pickup of %d timed out, skipping", item.ObjectID)

		return
	}
	if now.Sub(l.lootMoveAt) < selectPeriod {
		return
	}
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	dist := math.Hypot(float64(item.X-selfX), float64(item.Y-selfY))
	l.lootMoveAt = now
	if dist > lootApproachRadius {
		if err := l.game.WalkTo(item.X, item.Y, item.Z); err != nil {
			l.logger.Printf("Hunt: walk to loot failed: %v", err)
		}

		return
	}
	if err := l.game.PickupItem(item); err != nil {
		l.logger.Printf("Hunt: pickup failed: %v", err)
	}
}

// cleanupInventory destroys junk items when the slots or the weight of
// the character approach the server limits.
func (l *Loop) cleanupInventory() {
	stats := l.tracker.InventoryStats()
	if stats.SlotPercent < cleanupSlotPercent &&
		stats.WeightPercent < cleanupWeightPercent {
		return
	}
	junk := l.tracker.DestroyableItems(destroyBatch)
	if len(junk) == 0 {
		return
	}
	l.logger.Printf("Hunt: inventory at %d slots and %.0f%% weight, "+
		"destroying %d items", stats.Slots, stats.WeightPercent, len(junk))
	for _, item := range junk {
		err := l.game.DestroyItem(item.ObjectID, item.Count)
		if err != nil {
			l.logger.Printf("Hunt: destroy failed: %v", err)

			return
		}
	}
}
