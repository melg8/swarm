// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The Squire's starter kit of the generated item stats: 2369 the
// sword (rhand, pAtk 6 x 379), 1146 the shirt (chest, pDef 33), 1147
// the pants (legs, pDef 20).
const (
	squireSwordID = int32(2369)
	squireShirtID = int32(1146)
	squirePantsID = int32(1147)
)

// dagger (item 10): rhand, pAtk 5 x 433 - scores below the Squire's
// Sword.
const daggerID = int32(10)

// apprentice's rod (item 7): rhand blunt, pAtk 6 x 379 - scores
// exactly like the Squire's Sword.
const apprenticeRodID = int32(7)

// equippedItem builds an equipped inventory item entry.
func equippedItem(objectID int32, itemID int32) state.InventoryItem {
	return state.InventoryItem{
		ObjectID: objectID,
		ItemID:   itemID,
		Count:    1,
		Equipped: true,
	}
}

// TestReplacedStarterItemsListTheWholeReplacedKit verifies the dead
// weight detection: every starter piece whose slot holds an equal or
// better replacement is listed for destruction, in the deterministic
// object id order, with the weight it frees.
func TestReplacedStarterItemsListTheWholeReplacedKit(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith([]state.InventoryItem{
		item(900, squireSwordID),
		item(901, squireShirtID),
		item(902, squirePantsID),
		equippedItem(555, shortSwordID),
		equippedItem(556, shirtID),
		equippedItem(557, pantsID),
	}, map[Slot]int32{
		SlotChest: 556, SlotLegs: 557, SlotRHand: 555,
	})

	replaced := ReplacedStarterItems(profile, equipment)

	require.Len(t, replaced, 3, "the fully replaced kit must be listed")
	require.Equal(t, int32(900), replaced[0].Item.ObjectID)
	require.Equal(t, int32(901), replaced[1].Item.ObjectID)
	require.Equal(t, int32(902), replaced[2].Item.ObjectID)
	require.Equal(t, int32(1600), replaced[0].Weight,
		"the sword drop must report the freed weight")
	require.Contains(t, replaced[0].Reason, "Short Sword",
		"the reason must name the replacement")
	require.Contains(t, replaced[1].Reason, "Shirt (36)")
}

// TestReplacedStarterItemsSurviveWithoutReplacement pins the guard:
// an empty slot keeps its starter piece (the auto equipment still
// wears it) and no plain inventory item is ever destroyed.
func TestReplacedStarterItemsSurviveWithoutReplacement(t *testing.T) {
	profile := MeleeFighter{}
	// Nothing equipped at all: the starter pieces are the best the
	// character owns, plus a duplicate short sword in the bag.
	equipment := equipmentWith([]state.InventoryItem{
		item(900, squireSwordID),
		item(901, squireShirtID),
		item(902, squirePantsID),
		item(555, shortSwordID),
	}, nil)

	require.Empty(t, ReplacedStarterItems(profile, equipment),
		"no replacement equipped means no destruction")
}

// TestReplacedStarterItemsSurviveWhileStillBest pins the other guard:
// a worse equipped item keeps the starter piece (the planner would
// re-equip it) and an equipped starter piece is never its own
// replacement.
func TestReplacedStarterItemsSurviveWhileStillBest(t *testing.T) {
	profile := MeleeFighter{}
	// A dagger (5 x 433 = 2165) scores below the Squire's Sword
	// (6 x 379 = 2274): the planner swaps back, the sword survives.
	equipment := equipmentWith([]state.InventoryItem{
		item(900, squireSwordID),
		equippedItem(555, daggerID),
	}, map[Slot]int32{SlotRHand: 555})

	require.Empty(t, ReplacedStarterItems(profile, equipment),
		"a starter piece better than the equipped item survives")

	// The squire sword itself is worn: no destruction of the equipped
	// gear.
	equipment = equipmentWith([]state.InventoryItem{
		equippedItem(900, squireSwordID),
	}, map[Slot]int32{SlotRHand: 900})

	require.Empty(t, ReplacedStarterItems(profile, equipment),
		"an equipped starter piece is never destroyed")
}

// TestReplacedStarterItemsEqualReplacementDestroys pins the equality
// semantics: the equip planner only swaps on a strict gain, so an
// equal scoring replacement leaves the starter piece dead weight.
func TestReplacedStarterItemsEqualReplacementDestroys(t *testing.T) {
	profile := MeleeFighter{}
	// The rod (6 x 379 = 2274) scores exactly like the Squire's
	// Sword: the planner keeps the equipped one, the starter piece is
	// dead weight.
	equipment := equipmentWith([]state.InventoryItem{
		item(900, squireSwordID),
		equippedItem(555, apprenticeRodID),
	}, map[Slot]int32{SlotRHand: 555})

	replaced := ReplacedStarterItems(profile, equipment)
	require.Len(t, replaced, 1,
		"an equal replacement leaves the starter piece dead weight")
}

// TestReplacedStarterItemsOnePieceDisplacesThePants covers the family
// interplay: a one-piece armor blocks the legs slot, so the starter
// pants are dead weight even with the legs slot empty.
func TestReplacedStarterItemsOnePieceDisplacesThePants(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith([]state.InventoryItem{
		item(902, squirePantsID),
		equippedItem(558, onePieceID),
	}, map[Slot]int32{SlotChest: 558})

	replaced := ReplacedStarterItems(profile, equipment)
	require.Len(t, replaced, 1,
		"the one-piece displaces the starter pants")
	require.Equal(t, int32(902), replaced[0].Item.ObjectID)
}

// TestReplacedStarterItemsIgnorePlainDrops pins the scope: only the
// starter kit ids are ever destroyed - a plain duplicate drop (two
// short swords, one worn) stays in the bag for the next shop sale.
func TestReplacedStarterItemsIgnorePlainDrops(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith([]state.InventoryItem{
		equippedItem(555, shortSwordID),
		item(556, shortSwordID),
		item(557, pantsID),
	}, map[Slot]int32{SlotRHand: 555})

	require.Empty(t, ReplacedStarterItems(profile, equipment),
		"plain inventory items are never destroyed")
}
