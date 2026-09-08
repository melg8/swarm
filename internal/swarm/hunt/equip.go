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
	// starterRetryAt maps a starter item object id to the time its
	// failed destroy request may retry.
	starterRetryAt map[int32]time.Time
	// equipScanVersion holds the tracker inventory version the last
	// upgrade scan ran against, equipScanNone its empty result: the
	// tick path skips the gear scoring while the bag is unchanged
	// (the scan cost the full inventory walk of the planner).
	equipScanVersion uint64
	equipScanNone    bool
	// starterScanVersion and starterScanNone gate the starter item
	// scan the same way.
	starterScanVersion uint64
	starterScanNone    bool
}

// equipActionPeriod paces the auto equipment requests: the Mobius
// PlayerActionFloodProtector accepts one player action per second and
// the equipment shares the budget with the attack and select
// requests.
const equipActionPeriod = 2 * time.Second

// starterRetryDelay spaces the destroy retries of one starter item:
// a refused or lost request must not re-send and re-log every pacing
// period.
const starterRetryDelay = 10 * time.Second

// newEquipManager creates the manager for the gear profile.
func newEquipManager(profile gear.Profile) *equipManager {
	return &equipManager{
		profile:            profile,
		lastActionAt:       time.Time{},
		starterRetryAt:     make(map[int32]time.Time),
		equipScanVersion:   0,
		equipScanNone:      false,
		starterScanVersion: 0,
		starterScanNone:    false,
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
	if l.replacementSellingActive() {
		// The sell first step of the replacement sales owns the
		// affected slots right now: an auto equip here would pull the
		// just unequipped pieces right back on before their sale.
		return
	}
	if now.Sub(manager.lastActionAt) < equipActionPeriod {
		return
	}
	// The scan cache: an unchanged bag since the last empty scan
	// cannot hold a new upgrade, skip the inventory walk.
	version := l.tracker.InventoryVersion()
	if manager.equipScanNone && version == manager.equipScanVersion {
		return
	}
	action, ok := gear.NextUpgrade(manager.profile, l.equipment())
	manager.equipScanVersion = version
	manager.equipScanNone = !ok
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

// maybeDestroyReplacedStarters destroys the starter kit items a
// replacement has already displaced on the paperdoll: the Squire's
// set is neither sellable to a shop nor droppable on the ground
// (is_sellable=false, is_dropable=false in the Mobius item xml), so
// the destroy request is the only way the dead weight ever leaves
// the character. The request runs behind the same confirmation gate
// as the equips (the vanishing of the item confirms it) and shares
// the equip action budget, so a replace swap always lands its equip
// first and the starter item is destroyed only once the tracker
// shows the replacement worn. Called on every tick of the
// autonomous hunting phases right after the auto equipment.
func (l *Loop) maybeDestroyReplacedStarters() {
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
	// The scan cache: an unchanged bag since the last empty scan
	// holds no replaced starters.
	version := l.tracker.InventoryVersion()
	if manager.starterScanNone && version == manager.starterScanVersion {
		return
	}
	replaced := gear.ReplacedStarterItems(manager.profile, l.equipment())
	manager.starterScanVersion = version
	manager.starterScanNone = len(replaced) == 0
	for _, drop := range replaced {
		if until, ok := manager.starterRetryAt[drop.Item.ObjectID]; ok &&
			now.Before(until) {
			continue
		}
		l.markInventoryAction(drop.Item.ObjectID)
		if err := l.game.DestroyItem(drop.Item.ObjectID,
			drop.Item.Count); err != nil {
			l.logger.Printf("Hunt: starter destroy failed: %v", err)
			manager.starterRetryAt[drop.Item.ObjectID] = now.Add(starterRetryDelay)

			return
		}
		manager.lastActionAt = now
		l.logger.Printf("Hunt: gear: %s", drop.Reason)

		return
	}
}

// gearPoints reports the zone gating points of the equipped gear.
func (l *Loop) gearPoints() int32 {
	if l.equip == nil {
		return 0
	}

	return gear.TotalGearPoints(l.equip.profile, l.equipment())
}
