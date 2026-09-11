// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package gear scores equipment and plans equips and purchases. The
// slot model mirrors the paperdoll of the Mobius server, the scoring
// profiles rank items per combat class (a melee fighter now, mages
// later) and the planners turn the inventory and the shop catalogs
// into concrete use item and buy item actions.
package gear

import (
	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// Slot is one equip position of the paperdoll. The ear and finger
// slots come in pairs; the UserInfo paperdoll block is the only source
// that tells which of the two an equipped jewel occupies.
type Slot int

const (
	// SlotUnderwear is the underwear slot.
	SlotUnderwear Slot = iota
	// SlotREar is the right ear slot.
	SlotREar
	// SlotLEar is the left ear slot.
	SlotLEar
	// SlotNeck is the necklace slot.
	SlotNeck
	// SlotRFinger is the right finger slot.
	SlotRFinger
	// SlotLFinger is the left finger slot.
	SlotLFinger
	// SlotHead is the head slot.
	SlotHead
	// SlotRHand is the right hand (weapon) slot.
	SlotRHand
	// SlotLHand is the left hand (shield) slot.
	SlotLHand
	// SlotGloves is the gloves slot.
	SlotGloves
	// SlotChest is the chest slot; a one-piece armor occupies it and
	// blocks the legs slot.
	SlotChest
	// SlotLegs is the legs slot.
	SlotLegs
	// SlotFeet is the feet slot.
	SlotFeet
	// SlotBack is the back (cloak) slot.
	SlotBack
	// slotCount bounds the slot list.
	slotCount
)

// slotNames carries the log names of the slots.
var slotNames = [slotCount]string{
	"underwear", "right ear", "left ear", partNeck, "right finger",
	"left finger", partHead, "right hand", "left hand", partGloves,
	partChest, partLegs, partFeet, partBack,
}

// String renders the slot for logs.
func (s Slot) String() string {
	if s < 0 || s >= slotCount {
		return "unknown slot"
	}

	return slotNames[s]
}

// slotInvalid marks paperdoll block entries that map to no managed
// slot (the C1 duplicate right hand at the end of the block).
const slotInvalid Slot = -1

// Body part mask names of the C1 item stats: the values the generated
// GearStats.BodyPart field spells for the hand and family slots (the
// pair families are "either-or" masks the server data writes as one
// string).
const (
	partLhand    = "lhand"
	partLrhand   = "lrhand"
	partChest    = "chest"
	partLegs     = "legs"
	partNeck     = "neck"
	partOnepiece = "onepiece"
	partEars     = "rear;lear"
	partFingers  = "rfinger;lfinger"
	partHead     = "head"
	partGloves   = "gloves"
	partFeet     = "feet"
	partBack     = "back"
)

// paperdollIndexSlots maps the UserInfo paperdoll block index (the
// state paperdoll constants) to the gear slot.
var paperdollIndexSlots = [state.PaperdollSlots]Slot{
	state.PaperdollUnderwear:      SlotUnderwear,
	state.PaperdollREar:           SlotREar,
	state.PaperdollLEar:           SlotLEar,
	state.PaperdollNeck:           SlotNeck,
	state.PaperdollRFinger:        SlotRFinger,
	state.PaperdollLFinger:        SlotLFinger,
	state.PaperdollHead:           SlotHead,
	state.PaperdollRHand:          SlotRHand,
	state.PaperdollLHand:          SlotLHand,
	state.PaperdollGloves:         SlotGloves,
	state.PaperdollChest:          SlotChest,
	state.PaperdollLegs:           SlotLegs,
	state.PaperdollFeet:           SlotFeet,
	state.PaperdollBack:           SlotBack,
	state.PaperdollDuplicateRHand: slotInvalid,
}

// SlotOfPaperdollIndex maps a UserInfo paperdoll block index to the
// gear slot; ok is false for the C1 duplicate right hand entry.
func SlotOfPaperdollIndex(index int) (Slot, bool) {
	if index < 0 || index >= state.PaperdollSlots {
		return slotInvalid, false
	}
	slot := paperdollIndexSlots[index]
	if slot == slotInvalid {
		return slot, false
	}

	return slot, true
}

// bodyPartSlots maps the item template bodypart to the paperdoll
// slots an item of it can occupy. Hair has no slot inside the 15 slot
// block of the C1 UserInfo paperdoll, so hair items are unequippable
// for the planner (an empty slot list).
var bodyPartSlots = map[string][]Slot{
	"rhand":      {SlotRHand},
	partLrhand:   {SlotRHand},
	partLhand:    {SlotLHand},
	partChest:    {SlotChest},
	partLegs:     {SlotLegs},
	partOnepiece: {SlotChest},
	partHead:     {SlotHead},
	partGloves:   {SlotGloves},
	partFeet:     {SlotFeet},
	partBack:     {SlotBack},
	"underwear":  {SlotUnderwear},
	partNeck:     {SlotNeck},
	partEars:     {SlotREar, SlotLEar},
	partFingers:  {SlotRFinger, SlotLFinger},
}

// SlotsForBodyPart lists the paperdoll slots an item with the template
// bodypart can occupy.
func SlotsForBodyPart(bodyPart string) []Slot {
	return bodyPartSlots[bodyPart]
}

// jewelBodyParts reports whether the bodypart belongs to the jewel
// family (the mDef carrying accessories).
func jewelBodyPart(bodyPart string) bool {
	switch bodyPart {
	case partEars, partFingers, partNeck:
		return true
	default:
		return false
	}
}

// Category is the gear family of an item for the scoring dispatch.
type Category int

const (
	// CategoryUnusable covers items without gear stats or of families
	// the profile cannot use.
	CategoryUnusable Category = iota
	// CategoryWeapon covers right hand and two hand weapons.
	CategoryWeapon
	// CategoryShield covers left hand shields.
	CategoryShield
	// CategoryArmor covers the cloth and leather slots.
	CategoryArmor
	// CategoryJewel covers ears, fingers and necks.
	CategoryJewel
)

// CategoryOf resolves the gear family of the item from its stats:
// weapons carry a weapon type, shields sit in the left hand, jewels
// on the ear, finger and neck bodyparts and everything else
// equippable is armor.
func CategoryOf(stats npcdata.GearStats) Category {
	switch {
	case stats.WeaponType != "" &&
		(stats.BodyPart == "rhand" || stats.BodyPart == partLrhand):
		return CategoryWeapon
	case stats.BodyPart == partLhand && stats.WeaponType == "":
		return CategoryShield
	case jewelBodyPart(stats.BodyPart):
		return CategoryJewel
	case stats.BodyPart != "":
		return CategoryArmor
	default:
		return CategoryUnusable
	}
}

// ScoredItem is an inventory item joined with its stats and profile
// score. The Slot is the paperdoll slot of an equipped entry (or the
// slot a simulated equip placed it in).
type ScoredItem struct {
	Item  state.InventoryItem
	Stats npcdata.GearStats
	Score float64
	Slot  Slot
}

// Equipment is the working set of the planners: every inventory item
// of the character and the equipped object id of every paperdoll slot
// (0 when the slot is empty).
type Equipment struct {
	Items []state.InventoryItem
	Slots [slotCount]int32
}

// NewEquipment joins the inventory items with the paperdoll object
// ids in the UserInfo block order.
func NewEquipment(
	items []state.InventoryItem, paperdoll [state.PaperdollSlots]int32,
) Equipment {
	//nolint:exhaustruct_v5 // zero slots fill below
	equipment := Equipment{Items: items}
	for index, objectID := range paperdoll {
		slot, ok := SlotOfPaperdollIndex(index)
		if ok {
			equipment.Slots[slot] = objectID
		}
	}

	return equipment
}

// itemByID finds the inventory item of the object id.
func (e Equipment) itemByID(objectID int32) (state.InventoryItem, bool) {
	for _, item := range e.Items {
		if item.ObjectID == objectID {
			return item, true
		}
	}

	return state.InventoryItem{
		ObjectID: 0,
		ItemID:   0,
		Count:    0,
		Type1:    0,
		Type2:    0,
		Equipped: false,
		BodyPart: 0,
		Enchant:  0,
		Change:   0,
	}, false
}

// Paperdoll returns the equipped item and its score of every slot
// (an empty ScoredItem when the slot is empty or its item carries no
// gear stats).
func (e Equipment) Paperdoll(profile Profile) [slotCount]ScoredItem {
	var paperdoll [slotCount]ScoredItem
	for slot, objectID := range e.Slots {
		if objectID == 0 {
			continue
		}
		item, ok := e.itemByID(objectID)
		if !ok {
			continue
		}
		stats, ok := npcdata.ItemGearStats(item.ItemID)
		if !ok {
			continue
		}
		paperdoll[slot] = ScoredItem{
			Item:  item,
			Stats: stats,
			Score: scoreStats(profile, stats),
			Slot:  Slot(slot),
		}
	}

	return paperdoll
}
