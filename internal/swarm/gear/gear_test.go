// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// short sword (item 1): rhand SWORD pAtk 8 pAtkSpd 379.
const shortSwordID = int32(1)

// broadsword (item 3): rhand SWORD pAtk 11 pAtkSpd 379.
const broadswordID = int32(3)

// short bow (item 13): lrhand BOW pAtk 16 pAtkSpd 293.
const shortBowID = int32(13)

// short spear (item 15): lrhand POLE pAtk 24 pAtkSpd 325.
const shortSpearID = int32(15)

// shirt (item 21): chest pDef 36.
const shirtID = int32(21)

// leather shirt (item 22): chest pDef 43.
const leatherShirtID = int32(22)

// pants (item 28): legs pDef 22.
const pantsID = int32(28)

// small shield (item 19): lhand sDef 56 rShld 20.
const smallShieldID = int32(19)

// apprentice's earring (item 112): rear;lear mDef 11.
const apprenticeEarringID = int32(112)

// mystic's earring (item 113): rear;lear mDef 13.
const mysticEarringID = int32(113)

// necklace of magic (item 118): neck mDef 15.
const necklaceOfMagicID = int32(118)

// cloth cap (item 41): head pDef 13.
const clothCapID = int32(41)

// one-piece armor (item 60): onepiece pDef 224.
const onePieceID = int32(60)

// item builds an inventory item entry.
func item(objectID int32, itemID int32) state.InventoryItem {
	return state.InventoryItem{
		ObjectID: objectID,
		ItemID:   itemID,
		Count:    1,
	}
}

// equipmentWith builds the equipment from the items and the equipped
// object ids of the slots, routing them through the paperdoll block
// the same way the hunt loop does.
func equipmentWith(
	items []state.InventoryItem, equipped map[Slot]int32,
) Equipment {
	var paperdoll [state.PaperdollSlots]int32
	indexOfSlot := map[Slot]int{
		SlotUnderwear: state.PaperdollUnderwear,
		SlotREar:      state.PaperdollREar,
		SlotLEar:      state.PaperdollLEar,
		SlotNeck:      state.PaperdollNeck,
		SlotRFinger:   state.PaperdollRFinger,
		SlotLFinger:   state.PaperdollLFinger,
		SlotHead:      state.PaperdollHead,
		SlotRHand:     state.PaperdollRHand,
		SlotLHand:     state.PaperdollLHand,
		SlotGloves:    state.PaperdollGloves,
		SlotChest:     state.PaperdollChest,
		SlotLegs:      state.PaperdollLegs,
		SlotFeet:      state.PaperdollFeet,
		SlotBack:      state.PaperdollBack,
	}
	for slot, objectID := range equipped {
		paperdoll[indexOfSlot[slot]] = objectID
	}

	return NewEquipment(items, paperdoll)
}

func TestSlotOfPaperdollIndex(t *testing.T) {
	slot, ok := SlotOfPaperdollIndex(state.PaperdollRHand)
	require.True(t, ok)
	require.Equal(t, SlotRHand, slot)

	slot, ok = SlotOfPaperdollIndex(state.PaperdollDuplicateRHand)
	require.False(t, ok)
	require.Equal(t, slotInvalid, slot)

	_, ok = SlotOfPaperdollIndex(-1)
	require.False(t, ok)

	_, ok = SlotOfPaperdollIndex(state.PaperdollSlots)
	require.False(t, ok)
}

func TestSlotsForBodyPart(t *testing.T) {
	require.Equal(t, []Slot{SlotRHand}, SlotsForBodyPart("rhand"))
	require.Equal(t, []Slot{SlotChest}, SlotsForBodyPart("onepiece"))
	require.Equal(t, []Slot{SlotRHand}, SlotsForBodyPart("lrhand"))
	require.Equal(
		t, []Slot{SlotREar, SlotLEar}, SlotsForBodyPart("rear;lear"))
	require.Empty(t, SlotsForBodyPart("hair"))
	require.Empty(t, SlotsForBodyPart("unknown"))
}

func TestMeleeFighterScoring(t *testing.T) {
	profile := MeleeFighter{}

	shortSword, ok := npcdata.ItemGearStats(shortSwordID)
	require.True(t, ok)
	broadsword, ok := npcdata.ItemGearStats(broadswordID)
	require.True(t, ok)
	require.Greater(
		t, profile.WeaponScore(broadsword),
		profile.WeaponScore(shortSword))

	bow, ok := npcdata.ItemGearStats(shortBowID)
	require.True(t, ok)
	require.Zero(t, profile.WeaponScore(bow))
	require.Zero(t, profile.WeaponPoints(bow))

	shield, ok := npcdata.ItemGearStats(smallShieldID)
	require.True(t, ok)
	require.InDelta(t, 11.2, profile.ShieldScore(shield), 0.0001)

	require.Equal(t, int32(11), profile.WeaponPoints(broadsword))
}

func TestNextUpgradeFillsEmptySlots(t *testing.T) {
	profile := MeleeFighter{}
	items := []state.InventoryItem{
		item(100, shirtID),
		item(101, clothCapID),
		item(102, necklaceOfMagicID),
	}
	equipment := equipmentWith(items, nil)

	// The shirt (36 pDef) beats the necklace (15 mDef) and the cap
	// (13 pDef) in the empty slot race.
	action, ok := NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.True(t, action.Equip)
	require.Equal(t, int32(100), action.ObjectID)
	require.Equal(t, SlotChest, action.Slot)
	require.InDelta(t, 36.0, action.Gain, 0.0001)
}

func TestNextUpgradeSwapsWeapons(t *testing.T) {
	profile := MeleeFighter{}
	items := []state.InventoryItem{
		item(100, shortSwordID),
		item(101, broadswordID),
	}
	equipment := equipmentWith(
		items, map[Slot]int32{SlotRHand: 100})

	action, ok := NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.True(t, action.Equip)
	require.Equal(t, int32(101), action.ObjectID)
	require.Equal(t, SlotRHand, action.Slot)
	require.Greater(t, action.Gain, float64(0))
	require.Contains(t, action.Reason, "swapping")
}

func TestNextUpgradeOptimalPaperdoll(t *testing.T) {
	profile := MeleeFighter{}
	items := []state.InventoryItem{
		item(100, broadswordID),
		item(101, shortSwordID),
		item(102, shortBowID),
	}
	equipment := equipmentWith(
		items, map[Slot]int32{SlotRHand: 100})

	// The weaker sword and the bow (score zero for a melee fighter)
	// bring no gain: the paperdoll is already optimal.
	_, ok := NextUpgrade(profile, equipment)
	require.False(t, ok)
}

func TestNextUpgradePairUnequipsTheWorseJewel(t *testing.T) {
	profile := MeleeFighter{}
	items := []state.InventoryItem{
		item(100, apprenticeEarringID), // mDef 11, right ear
		item(101, mysticEarringID),     // mDef 13, left ear
		item(102, necklaceOfMagicID),   // mDef 15, unequipped neck
	}
	// The necklace fills the empty neck slot first.
	equipment := equipmentWith(items, map[Slot]int32{
		SlotREar: 100,
		SlotLEar: 101,
	})
	action, ok := NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.True(t, action.Equip)
	require.Equal(t, SlotNeck, action.Slot)
	require.Equal(t, int32(102), action.ObjectID)

	// A better earring arrives (mystic earring 13 > the right ear
	// 11): the weaker right earring is freed first so the next
	// planner call equips the better one into it.
	items = append(items, item(103, mysticEarringID))
	equipment = equipmentWith(items, map[Slot]int32{
		SlotREar: 100,
		SlotLEar: 101,
		SlotNeck: 102,
	})
	action, ok = NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.False(t, action.Equip)
	require.Equal(t, int32(100), action.ObjectID)
	require.Equal(t, SlotREar, action.Slot)
	require.InDelta(t, 2.0, action.Gain, 0.0001)
}

func TestNextUpgradePairFillsTheEmptyHalf(t *testing.T) {
	profile := MeleeFighter{}
	items := []state.InventoryItem{
		item(100, apprenticeEarringID), // mDef 11, right ear
		item(101, mysticEarringID),     // mDef 13, unequipped
	}
	equipment := equipmentWith(items, map[Slot]int32{
		SlotREar: 100,
	})
	action, ok := NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.True(t, action.Equip)
	require.Equal(t, int32(101), action.ObjectID)
	require.Equal(t, SlotLEar, action.Slot)
	require.InDelta(t, 13.0, action.Gain, 0.0001)
}

func TestNextUpgradeTwoHandWeaponGuardsTheShield(t *testing.T) {
	profile := MeleeFighter{}
	// A short spear (pAtk 24, pole) with a small shield equipped:
	// equipping the spear drops the shield, so the swap must keep the
	// net gain positive (24*325 - 8*379 - 11.2 > 0).
	items := []state.InventoryItem{
		item(100, shortSwordID),
		item(101, smallShieldID),
		item(102, shortSpearID),
	}
	equipment := equipmentWith(items, map[Slot]int32{
		SlotRHand: 100,
		SlotLHand: 101,
	})
	action, ok := NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.True(t, action.Equip)
	require.Equal(t, int32(102), action.ObjectID)
	require.Equal(t, SlotRHand, action.Slot)

	// The reverse: equipping the shield while the two hand spear is
	// equipped loses the weapon - no positive swap exists.
	items = []state.InventoryItem{
		item(100, shortSpearID),
		item(101, smallShieldID),
	}
	equipment = equipmentWith(items, map[Slot]int32{
		SlotRHand: 100,
	})
	_, ok = NextUpgrade(profile, equipment)
	require.False(t, ok)
}

func TestNextUpgradeOnePieceAndFamily(t *testing.T) {
	profile := MeleeFighter{}
	// A one-piece must beat the chest plus legs family total
	// (224 > 36 + 22): it equips and unequips both pieces.
	items := []state.InventoryItem{
		item(100, shirtID),
		item(101, pantsID),
		item(102, onePieceID),
	}
	equipment := equipmentWith(items, map[Slot]int32{
		SlotChest: 100,
		SlotLegs:  101,
	})
	action, ok := NextUpgrade(profile, equipment)
	require.True(t, ok)
	require.True(t, action.Equip)
	require.Equal(t, int32(102), action.ObjectID)
	require.Equal(t, SlotChest, action.Slot)
	require.InDelta(t, 166.0, action.Gain, 0.0001)

	// The reverse family: legs + chest refill against a weak
	// one-piece. A leather shirt (43) refills the chest after the
	// pants (22) unequip the one-piece: 43 + 22 > 224 is false, so
	// no swap runs against a strong one-piece.
	items = []state.InventoryItem{
		item(100, onePieceID),
		item(101, pantsID),
		item(102, leatherShirtID),
	}
	equipment = equipmentWith(items, map[Slot]int32{
		SlotChest: 100,
	})
	_, ok = NextUpgrade(profile, equipment)
	require.False(t, ok)
}

func TestNextUpgradeLegsAgainstOnePiece(t *testing.T) {
	profile := MeleeFighter{}
	// Legs armor against an equipped one-piece: the one-piece leaves
	// and the best chest candidate refills, so the family total must
	// improve (leather shirt 43 + pants 22 > 224 is false: no swap).
	items := []state.InventoryItem{
		item(100, onePieceID),
		item(101, pantsID),
		item(102, leatherShirtID),
	}
	equipment := equipmentWith(items, map[Slot]int32{
		SlotChest: 100,
	})
	_, ok := NextUpgrade(profile, equipment)
	require.False(t, ok)
}

func TestTotalGearPoints(t *testing.T) {
	profile := MeleeFighter{}
	// Short sword (8 points) + shirt (36) + pants (22) = 66.
	items := []state.InventoryItem{
		item(100, shortSwordID),
		item(101, shirtID),
		item(102, pantsID),
	}
	equipment := equipmentWith(items, map[Slot]int32{
		SlotRHand: 100,
		SlotChest: 101,
		SlotLegs:  102,
	})
	require.Equal(t, int32(66), TotalGearPoints(profile, equipment))
}

func TestScoreSkipsNonGearItems(t *testing.T) {
	profile := MeleeFighter{}
	// Adena (57) carries no gear stats: zero score, zero points.
	require.Zero(t, Score(profile, item(1, 57)))
	require.Zero(t, GearPoints(profile, item(1, 57)))
}
