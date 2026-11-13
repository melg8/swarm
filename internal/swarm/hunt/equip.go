// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// equipManager runs the auto equipment of the hunt loop: gear.BurstUpgrade
// computes every strictly improving use item action the paperdoll needs, and
// the per item confirmation gate (markInventoryAction, inventoryItemAllowed)
// paces the requests that share state. A request waits while an in flight one
// targets the same item (a second request toggles the first back) or writes a
// paperdoll slot the other writes too, or the server threads could cancel each
// other. The independent equips go out in one tick; the dependent steps ride
// the next tick after their prerequisite confirmed (docs/shopping_strategy.md).
type equipManager struct {
    // profile scores the gear for the combat class of the character.
    profile gear.Profile
    // lastActionAt records the last inventory action (the starter destroy
    // spacing reads it); the equips pace on the confirmation gate alone.
    lastActionAt time.Time
    // starterRetryAt maps a starter item object id to the time its
    // failed destroy request may retry.
    starterRetryAt map[int32]time.Time
    // equipScanVersion holds the inventory version of the last upgrade scan and
    // equipScanNone its empty result: an unchanged bag skips the gear scoring.
    equipScanVersion uint64
    equipScanNone    bool
    // starterScanVersion and starterScanNone gate the starter scan the same
    // way.
    starterScanVersion uint64
    starterScanNone    bool
    // keepsCache holds the planned keep object ids of the last scan at
    // keepsVersion: the junk flows (the shop selling, the overflow destroy)
    // consult it every tick, so the scan caches per inventory mutation.
    keepsCache   map[int32]bool
    keepsVersion uint64
    keepsScanned bool
    // bowAttack caches the best owned bow attack at bowAttackVersion: the city
    // shopping gate asks every tick, while the answer only moves with an
    // inventory mutation.
    bowAttack        int32
    bowAttackVersion uint64
    bowAttackScanned bool
}

// equipActionPeriod spaces the fixed-window flows: the starter destroys (the
// server refuses fast destroys) and the shop unequip retries. The auto equips
// pace on the write set gate alone: the deployed build disables the UseItem
// flood protector (see gear.BurstUpgrade).
const equipActionPeriod = 2 * time.Second

// starterRetryDelay spaces the destroy retries of one starter item, so a lost
// request does not re-log every pacing period.
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
        bowAttack:          0,
        bowAttackVersion:   0,
        bowAttackScanned:   false,
    }
}

// plannedEquipKeeps resolves the object ids the junk flows must keep: the
// unequipped items the gear simulation places on the paperdoll plus the ranged
// luring tool of the melee profiles. A looted or bought upgrade waiting for its
// paced request is never sold or destroyed. Cached per inventory mutation.
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

// addLureToolKeeps adds the ranged tooling of the quiver carriers to the keep
// set: the melee lurer's bow scores zero under its profile, so the gear
// simulation never places it and the junk flows would sell the bought luring
// bow at the next vendor visit or destroy it for bag space. Every arrow stack
// stays too - ammo scores zero under every profile.
func (l *Loop) addLureToolKeeps(
    profile gear.Profile, keeps map[int32]bool,
) {
    if !gear.QuiverCarrier(profile) {
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
            // The ammo discriminator of gear/bow.go: an etc item riding the
            // left hand is a quiver item (the Mobius arrow xml).
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

// ownedBowAttack resolves the best owned bow attack through the version cache:
// the city shopping gate asks every tick, while the answer only moves with an
// inventory mutation.
func (l *Loop) ownedBowAttack() int32 {
    manager := l.equip
    if manager == nil {
        return gear.OwnedBow(l.equipment())
    }
    version := l.tracker.InventoryVersion()
    if manager.bowAttackScanned && version == manager.bowAttackVersion {
        return manager.bowAttack
    }
    manager.bowAttack = gear.OwnedBow(l.equipment())
    manager.bowAttackVersion = version
    manager.bowAttackScanned = true

    return manager.bowAttack
}

// maybeArmQuiver equips the archer's arrow stack: the bow shoots only with a
// quiver in the left hand, but ammo scores zero under every profile, so the
// equip planner never places it. One use item request per arming on the same
// write set gate; a dry stack empties the hand and the next tick re-arms from
// the biggest bag stack.
func (l *Loop) maybeArmQuiver() {
    manager := l.equip
    if manager == nil || l.game == nil {
        return
    }
    if !gear.IsArcher(manager.profile) {
        return
    }
    paperdoll := l.tracker.PaperdollSlotObjectIDs()
    if paperdoll[state.PaperdollLHand] != 0 {
        // The quiver (the only left hand item the archer profile keeps) is
        // worn already.
        return
    }
    if paperdoll[state.PaperdollRHand] == 0 {
        return
    }
    kind, ok := l.tracker.SelfWeaponKind()
    if !ok || kind != weaponTypeBow {
        return
    }
    _, arrowID, hasTool := l.bowAndArrow()
    if !hasTool {
        return
    }
    if len(l.userDeferred) > 0 || l.replacementSellingActive() {
        // The manual mode or the replacement sale owns the inventory
        // flows right now; the arming rides the next tick.
        return
    }
    if !l.inventoryItemAllowed(arrowID, []gear.Slot{gear.SlotLHand}) {
        return
    }
    l.markInventoryAction(arrowID)
    if err := l.game.UseItem(arrowID); err != nil {
        l.logf("Hunt: quiver equip failed: %v", err)

        return
    }
    l.logf("Hunt: gear: arming the quiver, the bow shoots with " +
        "the arrows worn")
}

// maybeEquipGear executes the auto equipment burst: one independent upgrade per
// tick (the Mobius PacketExecutor has no same-client ordering, so a racing pair
// can bounce a member, which re-rides the next tick), while the dependent steps
// wait for their confirmation behind the per item gate. A full starting bag
// dresses in plan order across a handful of ticks. The manual command queue and
// the lure own the item budget.
func (l *Loop) maybeEquipGear() {
    manager := l.equip
    if manager == nil || l.game == nil {
        return
    }
    now := l.now()
    l.prunePendingActions(now)
    if len(l.userDeferred) > 0 {
        return
    }
    if l.replacementSellingActive() {
        // The sell first step of the replacement sales owns the affected slots:
        // an auto equip would pull the pieces back on before their sale.
        return
    }
    if l.lureArmed() {
        // The lure owns the weapon slots: the auto equipment would pull the
        // melee weapon back over the bow mid pull (the planner cannot know the
        // lure wants it worn).
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
            // The step races an in flight request: it rides the next tick
            // after the confirmation.
            continue
        }
        l.markInventoryAction(action.ObjectID)
        if err := l.game.UseItem(action.ObjectID); err != nil {
            l.logf("Hunt: gear equip failed: %v", err)

            return
        }
        manager.lastActionAt = now
        l.logf("Hunt: gear: %s", action.Reason)

        // ONE equip per tick: the next planned action rides the next tick
        // behind the confirmation gate.
        return
    }
}

// maybeDestroyReplacedStarters destroys the starter kit pieces a replacement
// displaced: the Squire's set is neither sellable nor droppable, so the destroy
// is the only way it leaves the character. The request runs behind the same
// gate and budget as the equips, so the replacement always shows worn first,
// and a failed destroy retries after starterRetryDelay.
func (l *Loop) maybeDestroyReplacedStarters() {
    manager := l.equip
    if manager == nil || l.game == nil {
        return
    }
    now := l.now()
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
