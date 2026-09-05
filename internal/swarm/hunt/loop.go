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
	// PickupItem clicks a ground item to walk to it and pick it up.
	PickupItem(item state.LootItem) error
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
	// selectPeriod is the minimum pause between two nearest target
	// selections. The server accepts one player action per second
	// (PlayerActionFloodProtector), so selecting faster is pointless.
	selectPeriod = 1 * time.Second
	// reengageHealthPercent is the HP level above which the next target
	// is engaged immediately after a kill instead of resting. Below it
	// the loop waits for the natural regeneration to recover.
	reengageHealthPercent = 50.0
	// engageRetryPeriod is the pause between repeated forced attack
	// requests for the selected target. The server accepts one player
	// action per second (PlayerActionFloodProtector), so one second is
	// the fastest useful cadence.
	engageRetryPeriod = 1 * time.Second
	// pickupTimeout is how long one pickup attempt may take before the
	// item is skipped.
	pickupTimeout = 12 * time.Second
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
	game    GameAPI
	tracker *state.Bot
	logger  *log.Logger
	phase   phase
	target  int32
	lastHit time.Time
	lootID  int32
	lootAt  time.Time
	skipped map[int32]time.Time
}

// NewLoop creates the hunt loop for a connected game client.
func NewLoop(game GameAPI, tracker *state.Bot) *Loop {
	return &Loop{
		game:    game,
		tracker: tracker,
		logger:  log.Default(),
		phase:   phaseEngage,
		target:  0,
		lastHit: time.Time{},
		lootID:  0,
		lootAt:  time.Time{},
		skipped: make(map[int32]time.Time),
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
	l.cleanupInventory()
	if l.phase == phaseEngage {
		l.engage()
	}
	if l.phase == phaseLoot {
		l.loot()
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
		// death. Healthy characters chain the next target
		// immediately (the selectPeriod rate limit keeps the
		// server flood protector happy).
		if hp := l.tracker.SelfHealthPercent(); hp < reengageHealthPercent {
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

// rest logs the regeneration wait once per rest phase so the idle time
// stays explainable in the bot log instead of silently standing still.
// It borrows the lastHit timestamp as the log rate limiter because no
// player action is sent while resting.
func (l *Loop) rest() {
	now := time.Now()
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	l.logger.Printf("Hunt: resting, HP %.0f%% below %.0f%%",
		l.tracker.SelfHealthPercent(), reengageHealthPercent)
}

// loot picks up the ground items around the character until none is left
// within the loot radius, then hunts the next target.
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
		if err := l.game.PickupItem(item); err != nil {
			l.logger.Printf("Hunt: pickup failed: %v", err)
		}

		return
	}
	if now.Sub(l.lootAt) > pickupTimeout {
		// The pickup did not finish: the item is protected or the
		// inventory is full. Skip it for a while and try the next one.
		l.skipped[item.ObjectID] = now.Add(pickupRetryDelay)
		l.lootID = 0
		l.logger.Printf("Hunt: pickup of %d timed out, skipping", item.ObjectID)
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
