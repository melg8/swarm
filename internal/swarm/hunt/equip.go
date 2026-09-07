// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
)

// Auto equipment of the hunt loop: the gear planner of the gear
// package computes the next strictly improving use item action from
// the tracked inventory and paperdoll and the manager paces the
// requests. The confirmation gate is shared with the manual inventory
// commands (markInventoryAction): the next request - manual or
// automatic - only goes out after the inventory update confirmed the
// previous one flipped the equipped flag, so a lost answer can never
// toggle an item back off and the two sources never race on the same
// item.
type equipManager struct {
	// profile scores the gear for the combat class of the character.
	profile gear.Profile
	// lastActionAt paces the use item requests between the player
	// action flood protector windows.
	lastActionAt time.Time
}

// equipActionPeriod paces the auto equipment requests: the Mobius
// PlayerActionFloodProtector accepts one player action per second and
// the equipment shares the budget with the attack and select
// requests.
const equipActionPeriod = 2 * time.Second

// newEquipManager creates the manager for the gear profile.
func newEquipManager(profile gear.Profile) *equipManager {
	return &equipManager{
		profile:      profile,
		lastActionAt: time.Time{},
	}
}

// equipment builds the planner working set from the tracker.
func (l *Loop) equipment() gear.Equipment {

	return gear.NewEquipment(
		l.tracker.InventoryItems(),
		l.tracker.PaperdollSlotObjectIDs())
}

// maybeEquipGear executes the next auto equipment action. It defers
// to the manual inventory command queue (in flight or deferred
// commands own the item action budget), paces its own requests and
// arms the shared confirmation gate for the flip of the equipped
// flag. Called on every tick of the autonomous hunting phases; after
// every inventory changing event (loot, buy, sell) the next call
// re-plans and keeps the paperdoll up to date while the bot works.
func (l *Loop) maybeEquipGear() {
	manager := l.equip
	if manager == nil || l.game == nil {
		return
	}
	now := time.Now()
	if !l.inventoryGateOpen() || len(l.userDeferred) > 0 {
		return
	}
	if now.Sub(manager.lastActionAt) < equipActionPeriod {
		return
	}
	action, ok := gear.NextUpgrade(manager.profile, l.equipment())
	if !ok {
		return
	}
	l.markInventoryAction(action.ObjectID)
	if err := l.game.UseItem(action.ObjectID); err != nil {
		l.logger.Printf("Hunt: gear equip failed: %v", err)

		return
	}
	manager.lastActionAt = now
	l.logger.Printf("Hunt: gear: %s", action.Reason)
}

// gearPoints reports the zone gating points of the equipped gear.
func (l *Loop) gearPoints() int32 {
	if l.equip == nil {
		return 0
	}

	return gear.TotalGearPoints(l.equip.profile, l.equipment())
}
