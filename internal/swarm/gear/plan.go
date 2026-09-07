// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"sort"
	"strconv"

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
	found  bool
}

// better replaces the accumulator when the gain strictly improves
// (the candidate iteration order breaks ties deterministically).
func (c *nextPlanCandidate) better(action EquipAction) {
	if !c.found || action.Gain > c.action.Gain {
		c.action = action
		c.found = true
	}
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
	candidates := scoreUnequipped(profile, equipment)
	var best nextPlanCandidate
	for _, candidate := range candidates {
		slots := SlotsForBodyPart(candidate.Stats.BodyPart)
		if len(slots) == 0 || candidate.Item.Count < 1 {
			continue
		}
		switch candidate.Stats.BodyPart {
		case "lrhand":
			planTwoHandWeapon(&best, paperdoll, candidate)
		case "onepiece":
			planOnePiece(&best, paperdoll, candidate)
		case "lhand":
			planShield(&best, paperdoll, candidate)
		case "legs":
			planLegs(&best, paperdoll, candidate, equipment, profile)
		default:
			planSimple(&best, paperdoll, candidate, slots)
		}
	}

	return best.action, best.found
}

// scoreUnequipped lists the unequipped inventory items the profile
// can use with their scores, the highest score first (the object id
// breaks ties so the plan is deterministic).
func scoreUnequipped(
	profile Profile, equipment Equipment,
) []ScoredItem {
	candidates := make([]ScoredItem, 0, len(equipment.Items))
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
		})
	}
	sort.Slice(candidates, func(i int, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}

		return candidates[i].Item.ObjectID < candidates[j].Item.ObjectID
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
		planPair(best, paperdoll, candidate, slots[0], slots[1])

		return
	}
	slot := slots[0]
	current := paperdoll[slot]
	if current.Item.ObjectID == 0 {
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     slot,
			Gain:     candidate.Score,
			Reason: "equipping " + describeItem(candidate) +
				" into the empty " + slot.String() + " slot",
		})

		return
	}
	if candidate.Score > current.Score {
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     slot,
			Gain:     candidate.Score - current.Score,
			Reason: "swapping the " + slot.String() + " " +
				describeItem(current) + " for the better " +
				describeItem(candidate),
		})
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
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     empty,
			Gain:     candidate.Score,
			Reason: "equipping " + describeItem(candidate) +
				" into the empty " + empty.String() + " slot",
		})

		return
	}
	worse, worseSlot := firstEntry, first
	if secondEntry.Score < firstEntry.Score {
		worse, worseSlot = secondEntry, second
	}
	if candidate.Score > worse.Score {
		best.better(EquipAction{
			ObjectID: worse.Item.ObjectID,
			Equip:    false,
			Slot:     worseSlot,
			Gain:     candidate.Score - worse.Score,
			Reason: "removing the weaker " + describeItem(worse) +
				" from the " + worseSlot.String() +
				" slot for the better " + describeItem(candidate),
		})
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
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     SlotRHand,
			Gain:     gain,
			Reason:   "swapping to the two hand " + describeItem(candidate),
		})
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
	if weapon.Stats.BodyPart == "lrhand" {
		gain -= weapon.Score
	}
	if gain > 0 {
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     SlotLHand,
			Gain:     gain,
			Reason:   "equipping the shield " + describeItem(candidate),
		})
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
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     SlotChest,
			Gain:     candidate.Score - family,
			Reason: "equipping the one-piece " + describeItem(candidate) +
				" over the chest and legs family",
		})
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
	if chest.Stats.BodyPart != "onepiece" {
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
		if !ok || stats.BodyPart != "chest" {
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
		best.better(EquipAction{
			ObjectID: candidate.Item.ObjectID,
			Equip:    true,
			Slot:     SlotLegs,
			Gain:     gain,
			Reason: "equipping legs " + describeItem(candidate) +
				" over the one-piece " + describeItem(chest),
		})
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
