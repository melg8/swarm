// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "strconv"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// emptyItem and emptyStats are the zero values the cleared virtual
// paperdoll entries carry (plain var declarations: the value types are
// cleared and rebuilt wholesale, never partially constructed).
var (
    emptyItem  state.InventoryItem
    emptyStats npcdata.GearStats
)

// clearedScoredItem is the empty entry a simulated equip writes into a
// paperdoll slot whose item the equip removes.
var clearedScoredItem = ScoredItem{
    Item:  emptyItem,
    Stats: emptyStats,
    Score: 0,
    Slot:  slotInvalid,
}

// slotBlocked reports whether an item of the bodypart would write
// into a slot the plan already bought for on this trip.
func slotBlocked(
    virtual [slotCount]ScoredItem, bodyPart string, boughtSlots map[Slot]bool,
) bool {
    buf := affectedSlots(virtual, bodyPart)
    for i := range buf.n {
        if boughtSlots[buf.data[i]] {
            return true
        }
    }

    return false
}

// applyToVirtual equips the entry on the virtual paperdoll with the same family
// effects the server applies: a one-piece empties the legs, a two hand weapon
// the left hand, a shield a two hand weapon and the legs a one-piece chest.
func applyToVirtual(virtual *[slotCount]ScoredItem, entry ScoredItem) {
    stats := entry.Stats
    slots := SlotsForBodyPart(stats.BodyPart)
    switch {
    case stats.BodyPart == partLrhand:
        entry.Slot = SlotRHand
        virtual[SlotRHand] = entry
        virtual[SlotLHand] = clearedScoredItem
    case stats.BodyPart == partOnepiece:
        entry.Slot = SlotChest
        virtual[SlotChest] = entry
        virtual[SlotLegs] = clearedScoredItem
    case stats.BodyPart == partLegs &&
        virtual[SlotChest].Stats.BodyPart == partOnepiece:
        entry.Slot = SlotLegs
        virtual[SlotLegs] = entry
        virtual[SlotChest] = clearedScoredItem
    case stats.BodyPart == partLhand &&
        virtual[SlotRHand].Stats.BodyPart == partLrhand:
        entry.Slot = SlotLHand
        virtual[SlotLHand] = entry
        virtual[SlotRHand] = clearedScoredItem
    case stats.BodyPart == partEars || stats.BodyPart == partFingers:
        entry.Slot = pairSlot(*virtual, slots)
        if entry.Slot != slotInvalid {
            virtual[entry.Slot] = entry
        }
    case len(slots) > 0:
        entry.Slot = slots[0]
        virtual[slots[0]] = entry
    }
}

// pairSlot picks the pair slot an equip fills: the empty one or the
// weaker one (mirroring the equip planner pair swap).
func pairSlot(virtual [slotCount]ScoredItem, slots []Slot) Slot {
    if len(slots) != 2 {
        return slotInvalid
    }
    first, second := slots[0], slots[1]
    if virtual[first].Item.ObjectID == 0 &&
        virtual[first].Stats.BodyPart == "" {
        return first
    }
    if virtual[second].Item.ObjectID == 0 &&
        virtual[second].Stats.BodyPart == "" {
        return second
    }
    if slotScore(virtual[second]) < slotScore(virtual[first]) {
        return second
    }

    return first
}

// SimulateInventory applies every free inventory upgrade to a copy of the
// paperdoll: the purchases plan against the paperdoll the auto equipment will
// reach anyway, so nothing gets bought that the inventory already carries.
func SimulateInventory(
    profile Profile, equipment Equipment,
) [slotCount]ScoredItem {
    virtual := equipment.Paperdoll(profile)
    candidates := scoreUnequipped(profile, equipment)
    for _, candidate := range candidates {
        _, ok := purchaseGain(virtual, candidate.Stats, candidate.Score)
        if ok {
            applyToVirtual(&virtual, candidate)
        }
    }

    return virtual
}

// PlannedEquips lists the object ids the auto equipment will wear: the
// unequipped pieces SimulateInventory places on the virtual paperdoll,
// the free upgrades the shop plan treats as already owned. The junk
// flows consult the set before selling or destroying - an item
// scheduled for wearing is never sold for instant adena or destroyed
// for bag space. Pieces the simulation leaves off (duplicates,
// downgrades, the displaced halves of pair swaps) stay plain junk.
func PlannedEquips(
    profile Profile, equipment Equipment,
) map[int32]bool {
    equipped := make(map[int32]bool, slotCount)
    for _, objectID := range equipment.Slots {
        if objectID != 0 {
            equipped[objectID] = true
        }
    }
    keeps := make(map[int32]bool, len(equipment.Items))
    virtual := SimulateInventory(profile, equipment)
    for slot := Slot(0); slot < slotCount; slot++ {
        entry := virtual[slot]
        if paperdollEmpty(entry) || equipped[entry.Item.ObjectID] {
            continue
        }
        keeps[entry.Item.ObjectID] = true
    }

    return keeps
}

func (c *purchaseCandidate) describe(gain float64) string {
    name := npcdata.ItemName(c.itemID)
    if name == "" {
        name = "item #" + strconv.Itoa(int(c.itemID))
    }
    gainText := strconv.FormatFloat(gain, 'f', -1, 64)
    priceText := strconv.FormatInt(c.price, 10)

    return name + " (+" + gainText + " for " + priceText + " adena)"
}

func AdenaSpent(purchases []Purchase) int64 {
    total := int64(0)
    for _, purchase := range purchases {
        total += purchase.Price
    }

    return total
}

// SellCreditOf sums the sell credits of the purchases: the adena the
// trip banks from selling the displaced pieces before the buys.
func SellCreditOf(purchases []Purchase) int64 {
    total := int64(0)
    for _, purchase := range purchases {
        total += purchase.SellCredit
    }

    return total
}

// displacedValue prices the equipped pieces the purchase displaces: the real
// paperdoll occupants of the affected slots (the virtual paperdoll decides
// WHICH slots a purchase writes to, the real one decides WHAT is sold), each at
// its sell value of referencePrice/2. Empty slots and the newbie kit items
// contribute nothing: the server silently skips every unsellable sell entry
// (is_sellable=false in the Mobius item xml, gear.IsStarterItem), so counting
// their credit armed a phantom budget. The returned sellFirst slice starts with
// room for two affected slots, so 0-2 displaced items pay one allocation.
func displacedValue(equipment Equipment, slots []Slot) (int64, []int32) {
    var credit int64
    ids := make([]int32, 0, len(slots))
    for _, slot := range slots {
        objectID := equipment.Slots[slot]
        if objectID == 0 {
            continue
        }
        item, ok := equipment.itemByID(objectID)
        if !ok {
            continue
        }
        if starterSet[item.ItemID] {
            // The shops refuse the newbie kit: no credit, no sell-first step -
            // the destroy flow of the replaced starters owns these items.
            continue
        }
        credit += npcdata.ItemPrice(item.ItemID) / 2
        ids = append(ids, objectID)
    }

    return credit, ids
}

// DisplacedByItem prices the equipped pieces a purchase of the item replaces:
// the paperdoll occupants of the item's bodypart slots at their sell value (see
// displacedValue). A fixed purchase plan (the city shopping errand) derives the
// same SellFirst credit the planner reads from its virtual paperdoll.
func DisplacedByItem(equipment Equipment, itemID int32) (int64, []int32) {
    stats, ok := npcdata.ItemGearStats(itemID)
    if !ok {
        return 0, nil
    }
    slots := SlotsForBodyPart(stats.BodyPart)
    if len(slots) == 0 {
        return 0, nil
    }

    return displacedValue(equipment, slots)
}
