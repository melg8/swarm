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

// elvenCatalog mirrors the elven village shop deployment: the weapon
// trader Unoren (7147), the armor trader Ariel (7148) and the jewel
// trader Creamees (7149) at the 15 percent elven village buy tax.
func elvenCatalog() Catalog {
	return Catalog{Shops: []Shop{
		{MerchantTemplateID: 7147, TaxRate: 0.15, Lists: []int32{3014700}},
		{MerchantTemplateID: 7148, TaxRate: 0.15, Lists: []int32{3014800}},
		{MerchantTemplateID: 7149, TaxRate: 0.15, Lists: []int32{3014900}},
	}}
}

// buyPrice resolves the expected elven buy price of an item.
func buyPrice(itemID int32) int64 {
	return int64(float64(npcdata.ItemPrice(itemID)) * 1.15)
}

func TestCatalogCandidatesCheapestOffer(t *testing.T) {
	profile := MeleeFighter{}
	candidates := catalogCandidates(profile, elvenCatalog())
	require.NotEmpty(t, candidates)
	var longSword *purchaseCandidate
	for index := range candidates {
		if candidates[index].itemID == 2 {
			longSword = &candidates[index]
		}
	}
	require.NotNil(t, longSword, "the long sword must be offered")
	require.Equal(t, buyPrice(2), longSword.price)
	require.Equal(t, int32(3014700), longSword.listID)
	require.Equal(t, int32(7147), longSword.merchant)

	// The bow (13) never enters the melee catalog.
	for _, candidate := range candidates {
		if candidate.itemID == 13 {
			t.Fatal("the melee profile must not buy bows")
		}
	}
}

func TestPlanPurchasesEmptyWithoutAdena(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 0)
	require.Empty(t, purchases)
}

func TestPlanPurchasesCheapFillersFirst(t *testing.T) {
	profile := MeleeFighter{}
	// A bare character with 500 adena: the greedy strategy fills the
	// empty slots with the best value per adena first. The
	// apprentice's shoes (8 pDef for 9 adena with tax) are the best
	// opening pick of the elven catalogs.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 500)
	require.NotEmpty(t, purchases)
	require.LessOrEqual(t, AdenaSpent(purchases), int64(500))
	first := purchases[0]
	require.Equal(t, int32(1121), first.ItemID,
		"the apprentice's shoes (8 pDef for 9 adena) are the best pick")
	require.Equal(t, buyPrice(1121), first.Price)
	require.Equal(t, int32(3014800), first.ListID)
	require.Equal(t, int32(7148), first.MerchantTemplateID)

	// The plan never buys the same item twice.
	seen := make(map[int32]bool)
	for _, purchase := range purchases {
		require.False(t, seen[purchase.ItemID])
		seen[purchase.ItemID] = true
	}
}

func TestPlanPurchasesOneWeaponPerTrip(t *testing.T) {
	profile := MeleeFighter{}
	// A rich character buys ONE weapon per trip: the best value per
	// adena pick (the short sword) takes the weapon slot and the upgrade
	// chain (the knife, the broadsword, the sickle, the long sword) is
	// cut - the next trip re-plans against the sword this one reached.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(
		profile, equipment, elvenCatalog(), 10_000_000)
	require.NotEmpty(t, purchases)
	weapons := 0
	foundShortSword := false
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok, "every purchase must carry gear stats")
		if stats.BodyPart == "rhand" || stats.BodyPart == "lrhand" {
			weapons++
		}
		if purchase.ItemID == 1 {
			foundShortSword = true
		}
	}
	require.Equal(t, 1, weapons,
		"one weapon purchase per trip, the chain is cut")
	require.True(t, foundShortSword,
		"the short sword is the value pick of the weapon slot")
	for _, purchase := range purchases {
		require.NotEqual(t, int32(2), purchase.ItemID,
			"the long sword upgrade waits for the next trip")
		require.NotEqual(t, int32(12), purchase.ItemID,
			"the knife upgrade waits for the next trip")
		require.NotEqual(t, int32(153), purchase.ItemID,
			"the sickle upgrade waits for the next trip")
	}
	require.LessOrEqual(t, AdenaSpent(purchases), int64(10_000_000))
}

// TestPlanPurchasesNoDuplicateNecklace reproduces the live regression
// of the first rich session: the plan bought the Necklace of Magic
// AND the Necklace of Knowledge in one walk and only the better one
// was ever worn. One purchase per slot keeps the adena.
func TestPlanPurchasesNoDuplicateNecklace(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 40_000)
	require.NotEmpty(t, purchases)
	necks := 0
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok)
		if stats.BodyPart == "neck" {
			necks++
		}
	}
	require.Equal(t, 1, necks,
		"one necklace per trip: never both the Magic and the Knowledge")
	require.LessOrEqual(t, AdenaSpent(purchases), int64(40_000))
}

func TestPlanPurchasesSkipsInventoryItems(t *testing.T) {
	profile := MeleeFighter{}
	// The inventory carries an unequipped broadsword (3): the free
	// upgrade is simulated first, so the shop broadsword is never
	// planned and the single weapon purchase of the trip is the best
	// value upgrade over it (the dirk).
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 3)}, nil)
	purchases := PlanPurchases(
		profile, equipment, elvenCatalog(), 10_000_000)
	weapons := 0
	for _, purchase := range purchases {
		require.NotEqual(t, int32(3), purchase.ItemID,
			"the broadsword the inventory already carries must not be bought")
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok)
		if stats.BodyPart == "rhand" || stats.BodyPart == "lrhand" {
			weapons++
		}
	}
	require.Equal(t, 1, weapons, "one weapon purchase per trip")
	found := false
	for _, purchase := range purchases {
		if purchase.ItemID == 216 {
			found = true
		}
	}
	require.True(t, found,
		"the dirk, the best value upgrade over the broadsword, is planned")
}

func TestPlanPurchasesSkipsEquippedGear(t *testing.T) {
	profile := MeleeFighter{}
	// The character wears a broadsword and a shirt: the empty slots
	// get their fillers, but the weapon purchase must be the long
	// sword, never a broadsword repeat.
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 3), item(101, 21)},
		map[Slot]int32{SlotRHand: 100, SlotChest: 101})
	purchases := PlanPurchases(
		profile, equipment, elvenCatalog(), 10_000_000)
	for _, purchase := range purchases {
		require.NotEqual(t, int32(3), purchase.ItemID)
	}
}

func TestSimulateInventoryAppliesFreeUpgrades(t *testing.T) {
	profile := MeleeFighter{}
	// The short sword is equipped and the broadsword sits in the
	// inventory: the simulation carries the broadsword on the weapon
	// slot.
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 1), item(101, 3)},
		map[Slot]int32{SlotRHand: 100})
	virtual := SimulateInventory(profile, equipment)
	require.Equal(t, int32(3), virtual[SlotRHand].Item.ItemID)
}
