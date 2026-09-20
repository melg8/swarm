// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// Auto equipment of the hunt loop: the burst planner of the gear
// package (gear.BurstUpgrade) computes every strictly improving use
// item action the paperdoll still needs and that cannot race another
// request of the same burst on the server, and the per item
// confirmation gate (markInventoryAction, inventoryItemAllowed)
// paces only the requests that actually share state. The gate is
// shared with the manual inventory commands: a request never goes
// out while an in flight one targets the same item (a second request
// would toggle the first one's effect back) or writes a paperdoll
// slot the in flight one writes too (the Mobius packet executor runs
// every client packet as its own thread pool task, so same slot
// requests can interleave on the server and cancel each other).
// The deployed build disables the UseItem flood protector
// (FloodProtectorUseItemInterval = 0, the retail matching config) and
// gives no same client ordering guarantee, so the limit of the
// sequential equip acceleration is exactly this write set model: the
// independent equips - the whole starting bag on an empty paperdoll -
// go out in one tick, the dependent steps (the pair swap refill, the
// one-piece drop follow up) ride the next tick after their
// prerequisite confirmed.
type equipManager struct {
    // profile scores the gear for the combat class of the character.
    profile gear.Profile
    // lastActionAt records the last inventory action of the manager:
    // the starter destroy flow keeps its fixed spacing from it (the
    // equips themselves pace on the confirmation gate alone, see
    // equipActionPeriod).
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
    // keepsCache holds the planned equip object ids of the last keeps
    // scan and keepsVersion the tracker inventory version it ran
    // against: the junk flows (the shop selling, the overflow destroy)
    // consult the set on every tick, so the scan is cached per
    // inventory mutation like the upgrade scan.
    keepsCache   map[int32]bool
    keepsVersion uint64
    keepsScanned bool
}

// equipActionPeriod spaces the item flows that keep a fixed window:
// the starter kit destroys (the server refuses fast destroys with
// "You are destroying items too fast.") and the shop unequip request
// retries of the replacement sales. The auto equips do not use it -
// the deployed build disables the UseItem flood protector
// (FloodProtectorUseItemInterval = 0) and the UseItem request never
// touches the one second PlayerActionFloodProtector of the attack and
// select packets, so the write set gate of the burst plan alone paces
// them at the speed the server actually applies the flips (see
// gear.BurstUpgrade).
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
        keepsCache:         nil,
        keepsVersion:       0,
        keepsScanned:       false,
    }
}

// plannedEquipKeeps resolves the object ids the junk flows must keep:
// the unequipped inventory items the gear simulation places on the
// paperdoll (see gear.PlannedEquips) plus the ranged luring tool of
// the melee profiles (see addLureToolKeeps). The junk flows of the
// town trips and the overflow cleanup exclude them - a looted or
// bought upgrade waiting for its paced use item request (a pair swap
// in flight, the confirmation window) is never sold for its instant
// adena and never destroyed for bag space. The set is cached per
// inventory mutation; sessions without a gear profile keep nothing.
func (l *Loop) plannedEquipKeeps() map[int32]bool {
    manager := l.equip
    if manager == nil || l.game == nil {
        return nil
    }
    version := l.tracker.InventoryVersion()
    if manager.keepsScanned && version == manager.keepsVersion {
        return manager.keepsCache
    }
    keeps := gear.PlannedEquips(manager.profile, l.equipment())
    l.addLureToolKeeps(manager.profile, keeps)
    manager.keepsCache = keeps
    manager.keepsVersion = version
    manager.keepsScanned = true

    return manager.keepsCache
}

// addLureToolKeeps adds the ranged luring tool of the melee profiles
// to the keep set: the bow scores zero under the profile (the
// profile's weapon ladder ranks the close combat damage), so the gear
// simulation never places it on the paperdoll and the plain keep set
// never holds it - the junk flows would sell the bought luring bow at
// the very next vendor visit (the owner report: the bot reaches the
// vendor and sells the bow it owns although it plans no more
// expensive bow) and the overflow cleanup would destroy it for bag
// space. The lure flow equips the bow from the bag on demand (see
// lure.go), so the strongest owned bow and the arrow stacks must
// survive every junk decision. A second, weaker bow stays plain junk
// (the duplicate gear rank of the sell order owns it), the mystic
// profiles never lure - their inventory bows are junk.
func (l *Loop) addLureToolKeeps(
    profile gear.Profile, keeps map[int32]bool,
) {
    if !gear.BowLurer(profile) {
        return
    }
    bestBowPower := int32(0)
    var bestBowObjectID int32
    for _, item := range l.tracker.InventoryItems() {
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if !ok {
            continue
        }
        if stats.WeaponType == weaponTypeBow {
            if stats.PAtk > bestBowPower {
                bestBowPower = stats.PAtk
                bestBowObjectID = item.ObjectID
            }

            continue
        }
        if stats.Type == "EtcItem" && stats.BodyPart == "lhand" &&
            stats.WeaponType == "" {
            // The ammo discriminator of gear/bow.go: an etc item that
            // rides the left hand is a quiver item (the Mobius arrow
            // xml: etcitem_type ARROW, bodypart lhand).
            keeps[item.ObjectID] = true
        }
    }
    if bestBowObjectID != 0 {
        keeps[bestBowObjectID] = true
    }
}

// equipment builds the planner working set from the tracker.
func (l *Loop) equipment() gear.Equipment {
    return gear.NewEquipment(
        l.tracker.InventoryItems(),
        l.tracker.PaperdollSlotObjectIDs())
}

// maybeEquipGear executes the auto equipment burst: every
// independent upgrade the burst planner offers goes out in this tick
// (the whole starting bag on an empty paperdoll dresses in one
// burst), the steps that would race an in flight request wait for its
// confirmation behind the per item gate. It defers to the manual
// inventory command queue (in flight or deferred commands own the
// item action budget) and arms the confirmation gate for every
// request it sends. The deployed build disables the UseItem flood
// protector, so no fixed pause limits the chain and a full starting
// outfit lands within one tick of the world entry. Called on every
// tick of the autonomous hunting phases; after every inventory
// changing event (loot, buy, sell) the next call re-plans and keeps
// the paperdoll up to date while the bot works.
func (l *Loop) maybeEquipGear() {
    manager := l.equip
    if manager == nil || l.game == nil {
        return
    }
    now := time.Now()
    l.prunePendingActions(now)
    if len(l.userDeferred) > 0 {
        return
    }
    if l.replacementSellingActive() {
        // The sell first step of the replacement sales owns the
        // affected slots right now: an auto equip here would pull the
        // just unequipped pieces right back on before their sale.
        return
    }
    if l.lureArmed() {
        // The lure owns the weapon slots while it runs: the auto
        // equipment would pull the melee weapon back over the bow in
        // the middle of the pull (the bow scores zero for the melee
        // profile, the planner cannot know the lure wants it worn).
        return
    }
    // The scan cache: an unchanged bag since the last empty scan
    // cannot hold a new upgrade, skip the inventory walk.
    version := l.tracker.InventoryVersion()
    if manager.equipScanNone && version == manager.equipScanVersion {
        return
    }
    planned := gear.BurstUpgrade(manager.profile, l.equipment())
    manager.equipScanVersion = version
    manager.equipScanNone = len(planned) == 0
    for _, action := range planned {
        if !l.inventoryItemAllowed(action.ObjectID, action.Slots) {
            // The step races an in flight request (its own item or
            // write set): it rides the next tick after the
            // confirmation. The scan stays armed (equipScanNone is
            // false for a non empty plan) so the tick re-plans.
            continue
        }
        l.markInventoryAction(action.ObjectID)
        if err := l.game.UseItem(action.ObjectID); err != nil {
            l.logf("Hunt: gear equip failed: %v", err)

            return
        }
        manager.lastActionAt = now
        l.logf("Hunt: gear: %s", action.Reason)
    }
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
    if len(l.userDeferred) > 0 {
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
        if !l.inventoryItemAllowed(drop.Item.ObjectID,
            gear.ServerWriteSlots(l.equipment(), drop.Item.ObjectID)) {
            continue
        }
        l.markInventoryAction(drop.Item.ObjectID)
        if err := l.game.DestroyItem(drop.Item.ObjectID,
            drop.Item.Count); err != nil {
            l.logf("Hunt: starter destroy failed: %v", err)
            manager.starterRetryAt[drop.Item.ObjectID] =
                now.Add(starterRetryDelay)

            return
        }
        manager.lastActionAt = now
        l.logf("Hunt: gear: %s", drop.Reason)

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
