// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The between-fights phases of the hunt loop, split out of the
// loop.go god file: resting to full health, looting the corpses
// and the inventory overflow cleanup.

import (
	"math"
	"time"
)

// restSittingHeld reports whether the resting logic still holds a
// sitting character down: the health below the stand threshold, or a
// caster whose mana has not recovered to the stand one yet.
func (l *Loop) restSittingHeld(hp float64) bool {
	if !l.tracker.SelfSitting() {
		return false
	}

	return hp < standUpHealthPercent || l.mageManaLow()
}

// rest brings the resting character back to full health (and a
// caster back to full mana). Sitting accelerates the regeneration,
// so the character sits down below the sit threshold and stands up
// again once recovered. The mana of a mystic rides the same toggles:
// the dry caster sits down under the mana sit threshold and keeps
// sitting until the mana recovered to the stand one (see
// combat_skills.go), a fighter ignores the mana gates entirely. The
// sit/stand action is a server side toggle, so every transition is
// confirmed by the ChangeWaitType broadcast before the opposite one
// is ever sent.
func (l *Loop) rest() {
	now := time.Now()
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	hp := l.tracker.SelfHealthPercent()
	wantSit := false
	switch {
	case l.restSittingHeld(hp):
		// The sit is confirmed and the regeneration is running -
		// the health or the mana of the caster still holds it.
		return
	case l.tracker.SelfSitting():
		// Sitting and recovered: stand up (wantSit stays false).
	case hp < sitDownHealthPercent:
		wantSit = true
	case l.mageManaDry():
		// The caster ran its mana dry: sitting regenerates several
		// times faster than standing, the fights resume at the stand
		// threshold.
		wantSit = true
	default:
		l.logf("Hunt: resting, HP %.0f%% below %.0f%%",
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
		if hp < sitDownHealthPercent {
			l.logf("Hunt: HP %.0f%% below %.0f%%, sitting down to regenerate",
				hp, sitDownHealthPercent)
		} else {
			l.logf("Hunt: mana %.0f%% below %.0f%%, sitting down to regenerate",
				l.tracker.SelfManaPercent(), manaSitPercent)
		}
	} else {
		l.logf("Hunt: HP %.0f%% recovered, standing up", hp)
	}
	if err := l.game.ActionSitStand(); err != nil {
		l.logf("Hunt: sit/stand action failed: %v", err)

		return
	}
	l.restActionAt = now
	l.restActionSit = wantSit
}

// loot picks up the ground items around the character until none is left
// within the loot radius, then hunts the next target. Farther items are
// approached with an explicit walk first so the character visibly runs
// toward the loot instead of trusting the click to start the whole
// approach. The search carries no zone filter: the kill that produced
// the drop often happens past the square line (the chase, the scatter
// of the drop), and a drop left on the ground because a line on the
// map crossed it is a pure loss - anything within the loot radius of
// the character is picked up, wherever it lies.
func (l *Loop) loot() {
	item, ok := l.tracker.NearestGroundItemExcluding(lootRadius, l.skipped, nil)
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
		l.logf("Hunt: pickup of %d timed out, skipping", item.ObjectID)

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
			l.logf("Hunt: walk to loot failed: %v", err)
		}

		return
	}
	if err := l.game.PickupItem(item); err != nil {
		l.logf("Hunt: pickup failed: %v", err)
	}
}

// cleanupInventory destroys junk items when the slots or the weight of
// the character approach the server limits. The planned equips of the
// auto equipment stay out of the destroy candidates: a looted upgrade
// waiting for its paced use item request is never destroyed for bag
// space - the bot wears it instead.
func (l *Loop) cleanupInventory() {
	stats := l.tracker.InventoryStats()
	if stats.SlotPercent < cleanupSlotPercent &&
		stats.WeightPercent < cleanupWeightPercent {
		return
	}
	junk := l.tracker.DestroyableItemsExcluding(
		l.plannedEquipKeeps(), destroyBatch)
	if len(junk) == 0 {
		return
	}
	l.logf("Hunt: inventory at %d slots and %.0f%% weight, "+
		"destroying %d items", stats.Slots, stats.WeightPercent, len(junk))
	for _, item := range junk {
		err := l.game.DestroyItem(item.ObjectID, item.Count)
		if err != nil {
			l.logf("Hunt: destroy failed: %v", err)

			return
		}
	}
}
