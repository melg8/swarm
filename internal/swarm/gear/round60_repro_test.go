// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// leather pants (item 29): legs pDef 27 - the affordable legs filler
// of the 2026-09-12 dump wallet.
const leatherPantsID = int32(29)

// The reproduction of the 2026-09-12 04:58 state dump report: the
// level 14 elven fighter test2 returned from a town trip without its
// legs armor - every other slot was filled (Sickle, Buckler, Leather
// Shirt, Wooden Helmet, Gloves, Leather Shoes, the basic jewels) and
// the bag carried nothing but the 13162 adena. This file asks the
// planner what it plans against that exact state.

// dumpEquipment builds the paperdoll of the dump: every item id and
// slot of the report, the legs slot empty.
func dumpEquipment() Equipment {
	return equipmentWith(
		[]state.InventoryItem{
			item(1, 20),   // Buckler
			item(2, 22),   // Leather Shirt
			item(3, 37),   // Leather Shoes
			item(4, 43),   // Wooden Helmet
			item(5, 49),   // Gloves
			item(6, 112),  // Apprentice's Earring
			item(7, 112),  // Apprentice's Earring
			item(8, 116),  // Magic Ring
			item(9, 116),  // Magic Ring
			item(10, 118), // Necklace of Magic
			item(11, 153), // Sickle
		},
		map[Slot]int32{
			SlotLHand:   1,
			SlotChest:   2,
			SlotFeet:    3,
			SlotHead:    4,
			SlotGloves:  5,
			SlotREar:    6,
			SlotLEar:    7,
			SlotRFinger: 8,
			SlotLFinger: 9,
			SlotNeck:    10,
			SlotRHand:   11,
		},
	)
}

// TestRound60DumpPlansLegs asks the phased planner for the plan
// against the dump state: the empty legs slot must produce a legs
// purchase the 13162 adena wallet can pay for.
func TestRound60DumpPlansLegs(t *testing.T) {
	profile := MeleeFighter{}
	equipment := dumpEquipment()
	queue := PlanPurchaseQueue(profile, equipment, elvenCatalog(), 13162)
	var legsPurchase *Purchase
	for index := range queue {
		if !queue[index].Affordable {
			break
		}
		stats, ok := npcdata.ItemGearStats(queue[index].ItemID)
		if !ok || stats.BodyPart != partLegs {
			continue
		}
		legsPurchase = &queue[index]

		break
	}
	require.NotNil(t, legsPurchase,
		"the empty legs slot must be planned in the affordable prefix")
	require.Equal(t, leatherPantsID, legsPurchase.ItemID,
		"the affordable legs filler of the dump wallet is the Leather Pants")
	require.True(t, legsPurchase.Affordable,
		"the legs filler must be affordable for the 13162 adena wallet")
}

// TestRound60DumpSlotModel pins the dump reconstruction itself: every
// armor slot of the report is filled except the legs - the state the
// gear debt machinery of the hunt package arms its refill on.
func TestRound60DumpSlotModel(t *testing.T) {
	profile := MeleeFighter{}
	equipment := dumpEquipment()
	paperdoll := equipment.Paperdoll(profile)
	require.True(t, paperdollEmpty(paperdoll[SlotLegs]),
		"the legs slot of the dump state is empty")
	for _, slot := range []Slot{
		SlotChest, SlotHead, SlotGloves, SlotFeet, SlotRHand, SlotLHand,
	} {
		require.Positive(t, paperdoll[slot].Score,
			"the %s slot of the dump state is filled", slot)
	}
}
