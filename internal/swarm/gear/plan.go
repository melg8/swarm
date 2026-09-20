// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "slices"
    "strconv"
    "sync"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// EquipAction is one use item request of the equip planner: using an
// equippable inventory item toggles it (see UseItem), so equipping
// into a slot the server replaces directly (weapons, chest, neck,
// ...) and swapping one jewel of a pair (ears, fingers) needs the
// unequip of the worse piece first - the next planner call equips the
// better item into the freed slot.
type EquipAction struct {
    // ObjectID is the inventory item the use item request targets.
    ObjectID int32
    // Equip is true for an equip request and false for the freeing
    // unequip of a pair swap.
    Equip bool
    // Slot is the paperdoll slot the action improves.
    Slot Slot
    // Gain is the score the swap gains on the slot (the net gain of
    // the pair swap: the better item minus the worse equipped one).
    Gain float64
    // Reason is the human readable log line.
    Reason string
}

// nextPlanCandidate is the internal best action accumulator.
type nextPlanCandidate struct {
    action EquipAction
    // The reason inputs of the winning offer: the text shape and the
    // paperdoll entries the text names beside the candidate. The
    // reason string builds once for the winner in describeUpgrade -
    // building it per offered candidate allocated two item
    // descriptions and a concatenation for every offer that lost the
    // better() race.
    kind      planKind
    candidate ScoredItem
    current   ScoredItem
    slot      Slot
    found     bool
}

// planKind names the text shape of the winning action's reason.
type planKind uint8

const (
    planKindEmpty planKind = iota
    planKindSwap
    planKindPairFree
    planKindTwoHand
    planKindShield
    planKindOnePiece
    planKindLegsOverOnePiece
)

// better replaces the accumulator when the gain strictly improves
// (the candidate iteration order breaks ties deterministically).
func (c *nextPlanCandidate) better(
    action EquipAction, kind planKind,
    candidate ScoredItem, current ScoredItem, slot Slot,
) {
    if !c.found || action.Gain > c.action.Gain {
        c.action = action
        c.kind = kind
        c.candidate = candidate
        c.current = current
        c.slot = slot
        c.found = true
    }
}

// describeUpgrade renders the reason of the winning action: the same
// text shapes the eager per candidate reasons used to carry (the
// test pins read them), built once per planner call.
func describeUpgrade(best nextPlanCandidate) string {
    switch best.kind {
    case planKindEmpty:
        return "equipping " + describeItem(best.candidate) +
            " into the empty " + best.slot.String() + " slot"
    case planKindSwap:
        return "swapping the " + best.slot.String() + " " +
            describeItem(best.current) + " for the better " +
            describeItem(best.candidate)
    case planKindPairFree:
        return "removing the weaker " + describeItem(best.current) +
            " from the " + best.slot.String() + " slot for the better " +
            describeItem(best.candidate)
    case planKindTwoHand:
        return "swapping to the two hand " + describeItem(best.candidate)
    case planKindShield:
        return "equipping the shield " + describeItem(best.candidate)
    case planKindOnePiece:
        return "equipping the one-piece " + describeItem(best.candidate) +
            " over the chest and legs family"
    case planKindLegsOverOnePiece:
        return "equipping legs " + describeItem(best.candidate) +
            " over the one-piece " + describeItem(best.current)
    default:
        return ""
    }
}

// HasWeapon reports whether the character wears or carries any weapon
// the profile can fight with: every inventory item (equipped or
// bagged) whose gear stats place it in the weapon family with a
// positive profile score counts - a bow is no weapon for the melee
// fighter, a sword is none for the mystic, the profile decides. The
// weapon run trigger of the hunt loop reads it: a character that
// answers false while its plan offers an affordable weapon shops for
// the weapon before anything else (see hunt/shopping.go).
func HasWeapon(profile Profile, equipment Equipment) bool {
    for _, item := range equipment.Items {
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if !ok {
            continue
        }
        if CategoryOf(stats) == CategoryWeapon &&
            scoreStats(profile, stats) > 0 {
            return true
        }
    }

    return false
}

// NextUpgrade plans the next equip action that strictly improves the
// paperdoll under the profile: an empty slot filled with the best
// scoring fitting item, a slot swapped for a strictly better item or
// the worse jewel of a pair freed for the better inventory item. It
// reports ok=false when the paperdoll is already optimal for the
// inventory.
func NextUpgrade(
    profile Profile, equipment Equipment,
) (EquipAction, bool) {
    paperdoll := equipment.Paperdoll(profile)
    buffer := scoredItemPool.Get().(*[]ScoredItem)
    candidates := appendScoredUnequipped(
        profile, equipment, (*buffer)[:0])
    defer scoredItemPool.Put(buffer)
    var best nextPlanCandidate
    for _, candidate := range candidates {
        slots := SlotsForBodyPart(candidate.Stats.BodyPart)
        if len(slots) == 0 || candidate.Item.Count < 1 {
            continue
        }
        switch candidate.Stats.BodyPart {
        case partLrhand:
            planTwoHandWeapon(&best, paperdoll, candidate)
        case partOnepiece:
            planOnePiece(&best, paperdoll, candidate)
        case partLhand:
            planShield(&best, paperdoll, candidate)
        case partLegs:
            planLegs(&best, paperdoll, candidate, equipment, profile)
        default:
            planSimple(&best, paperdoll, candidate, slots)
        }
    }
    if best.found {
        // The reason text builds once for the winning action: the
        // eager per candidate strings were pure waste - every offer
        // that lost the better() race allocated two item descriptions
        // and a concatenation the planner threw away (the 40 bot
        // fleet profile held them at 3.6 percent of all allocated
        // bytes).
        best.action.Reason = describeUpgrade(best)
    }

    return best.action, best.found
}

// emptyScoredItem is the no entry sentinel of the reason inputs (the
// text shapes that name no paperdoll entry beside the candidate).
var emptyScoredItem ScoredItem

// scoredItemPool recycles the candidate slices of the planner scans:
// the equip planner re-scores the whole inventory once per planner
// call and the burst loop calls the planner once per collected
// action, so a fresh slice per call was the dominant allocation
// source of the fleet (the 40 bot run held it at over half of all
// allocated bytes).
var scoredItemPool = sync.Pool{
    New: func() any {
        buf := make([]ScoredItem, 0, 64)

        return &buf
    },
}

// scoreUnequipped lists the unequipped inventory items the profile
// can use with their scores, the highest score first (the object id
// breaks ties so the plan is deterministic). The one-shot form of
// appendScoredUnequipped: the planner's own hot path uses the pooled
// buffer (see NextUpgrade).
func scoreUnequipped(
    profile Profile, equipment Equipment,
) []ScoredItem {
    return appendScoredUnequipped(
        profile, equipment, make([]ScoredItem, 0, len(equipment.Items)))
}

// appendScoredUnequipped appends the scored unequipped inventory
// items of the profile to the given buffer and sorts it, the highest
// score first (the object id breaks ties so the plan is
// deterministic). The buffer form exists so the burst loop can
// recycle the backing array across its planner calls.
func appendScoredUnequipped(
    profile Profile, equipment Equipment, candidates []ScoredItem,
) []ScoredItem {
    for _, item := range equipment.Items {
        if item.Equipped {
            continue
        }
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if !ok {
            continue
        }
        score := scoreStats(profile, stats)
        if score <= 0 {
            continue
        }
        candidates = append(candidates, ScoredItem{
            Item:  item,
            Stats: stats,
            Score: score,
            Slot:  slotInvalid,
        })
    }
    // slices.SortFunc over sort.Slice: the generic sort needs no
    // reflection swapper, the planner scan ran the reflectlite
    // allocation on every call of the fleet profile.
    slices.SortFunc(candidates, func(i, j ScoredItem) int {
        if i.Score != j.Score {
            if i.Score > j.Score {
                return -1
            }

            return 1
        }
        switch {
        case i.Item.ObjectID < j.Item.ObjectID:
            return -1
        case i.Item.ObjectID > j.Item.ObjectID:
            return 1
        default:
            return 0
        }
    })

    return candidates
}

// planSimple plans the equip of a single slot item (weapon, chest,
// neck, gloves, feet, head, back, underwear) and the pair families
// (ears, fingers): an empty slot equips directly, an occupied slot
// swaps when the candidate is strictly better. A full pair swaps
// through the unequip of the worse piece: the server would replace
// the left slot blindly, which can hit the better item.
func planSimple(
    best *nextPlanCandidate, paperdoll [slotCount]ScoredItem,
    candidate ScoredItem, slots []Slot,
) {
    if len(slots) == 2 {
        // G602: the len(slots) == 2 branch proves both indexes exist.
        planPair(best, paperdoll, candidate, slots[0], slots[1]) //nolint:gosec

        return
    }
    if len(slots) == 0 {
        return
    }
    slot := slots[0]
    current := paperdoll[slot]
    if current.Item.ObjectID == 0 {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     slot,
            Gain:     candidate.Score,
        }, planKindEmpty, candidate, emptyScoredItem, slot)

        return
    }
    if candidate.Score > current.Score {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     slot,
            Gain:     candidate.Score - current.Score,
        }, planKindSwap, candidate, current, slot)
    }
}

// planPair plans the equip of an ear or finger jewel: an empty slot
// of the pair equips directly; a full pair frees its worse piece
// first so the next planner call equips the candidate into it.
func planPair(
    best *nextPlanCandidate, paperdoll [slotCount]ScoredItem,
    candidate ScoredItem, first Slot, second Slot,
) {
    firstEntry := paperdoll[first]
    secondEntry := paperdoll[second]
    if firstEntry.Item.ObjectID == 0 || secondEntry.Item.ObjectID == 0 {
        empty := first
        if firstEntry.Item.ObjectID != 0 {
            empty = second
        }
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     empty,
            Gain:     candidate.Score,
        }, planKindEmpty, candidate, emptyScoredItem, empty)

        return
    }
    worse, worseSlot := firstEntry, first
    if secondEntry.Score < firstEntry.Score {
        worse, worseSlot = secondEntry, second
    }
    if candidate.Score > worse.Score {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: worse.Item.ObjectID,
            Equip:    false,
            Slot:     worseSlot,
            Gain:     candidate.Score - worse.Score,
        }, planKindPairFree, candidate, worse, worseSlot)
    }
}

// planTwoHandWeapon plans lrhand weapons (poles, bows): equipping one
// unequips a shield the server drops from the left hand, so the gain
// must cover the loss (bows never reach here for a melee profile:
// they score zero).
func planTwoHandWeapon(
    best *nextPlanCandidate, paperdoll [slotCount]ScoredItem,
    candidate ScoredItem,
) {
    weapon := paperdoll[SlotRHand]
    shield := paperdoll[SlotLHand]
    gain := candidate.Score
    if weapon.Item.ObjectID != 0 {
        gain -= weapon.Score
    }
    if shield.Item.ObjectID != 0 {
        gain -= shield.Score
    }
    if gain > 0 {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     SlotRHand,
            Gain:     gain,
        }, planKindTwoHand, candidate, emptyScoredItem, SlotRHand)
    }
}

// planShield plans left hand shields: equipping a shield while a two
// hand weapon occupies the right hand unequips the weapon (the
// server drops it from the right hand slot), so the gain must cover
// the weapon loss.
func planShield(
    best *nextPlanCandidate, paperdoll [slotCount]ScoredItem,
    candidate ScoredItem,
) {
    current := paperdoll[SlotLHand]
    weapon := paperdoll[SlotRHand]
    gain := candidate.Score
    if current.Item.ObjectID != 0 {
        gain -= current.Score
    }
    if weapon.Stats.BodyPart == partLrhand {
        gain -= weapon.Score
    }
    if gain > 0 {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     SlotLHand,
            Gain:     gain,
        }, planKindShield, candidate, emptyScoredItem, SlotLHand)
    }
}

// planOnePiece plans one-piece armors: they occupy the chest slot and
// block the legs slot, so their gain replaces the family (chest plus
// legs) and only a strictly better family total equips.
func planOnePiece(
    best *nextPlanCandidate, paperdoll [slotCount]ScoredItem,
    candidate ScoredItem,
) {
    chest := paperdoll[SlotChest]
    legs := paperdoll[SlotLegs]
    family := chest.Score + legs.Score
    if candidate.Score > family {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     SlotChest,
            Gain:     candidate.Score - family,
        }, planKindOnePiece, candidate, emptyScoredItem, SlotChest)
    }
}

// planLegs plans legs armor: equipping legs while a one-piece armor
// occupies the chest unequips it (the server drops it from the chest
// slot), so the gain compares against the one-piece score minus the
// best chest armor the inventory can refill the freed chest with.
func planLegs(
    best *nextPlanCandidate, paperdoll [slotCount]ScoredItem,
    candidate ScoredItem, equipment Equipment, profile Profile,
) {
    chest := paperdoll[SlotChest]
    current := paperdoll[SlotLegs]
    if chest.Stats.BodyPart != partOnepiece {
        planSimple(best, paperdoll, candidate, []Slot{SlotLegs})

        return
    }
    // The one-piece leaves the chest empty: the best unequipped chest
    // candidate refills it on the next planner call.
    refill := float64(0)
    for _, item := range equipment.Items {
        if item.Equipped {
            continue
        }
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if !ok || stats.BodyPart != partChest {
            continue
        }
        score := scoreStats(profile, stats)
        if score > refill {
            refill = score
        }
    }
    gain := candidate.Score + refill - chest.Score
    if current.Item.ObjectID != 0 {
        gain += current.Score
    }
    if gain > 0 {
        //nolint:exhaustruct_v5 // the reason builds on the winner
        best.better(EquipAction{
            ObjectID: candidate.Item.ObjectID,
            Equip:    true,
            Slot:     SlotLegs,
            Gain:     gain,
        }, planKindLegsOverOnePiece, candidate, chest, SlotLegs)
    }
}

// describeItem renders the item for action logs.
func describeItem(item ScoredItem) string {
    name := npcdata.ItemName(item.Item.ItemID)
    if name == "" {
        name = "item #" + strconv.Itoa(int(item.Item.ItemID))
    }
    score := strconv.FormatFloat(item.Score, 'f', -1, 64)

    return name + " (" + score + ")"
}
