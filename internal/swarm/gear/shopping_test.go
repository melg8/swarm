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
	// A bare character with 500 adena fills the empty slots with the
	// best value per adena first - and every filled slot takes its top
	// affordable tier: the cloth shoes (9 pDef for 42 adena) beat the
	// cheaper apprentice's shoes (8 pDef for 9 adena) on the same slot,
	// so the weaker rung never slides in behind the value ranking.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 500)
	require.NotEmpty(t, purchases)
	require.LessOrEqual(t, AdenaSpent(purchases), int64(500))
	// One purchase per paperdoll slot, and the feet slot takes the
	// cloth shoes tier, never the apprentice's intermediate.
	boughtSlots := make(map[string]bool)
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok, "every purchase must carry gear stats")
		require.False(t, boughtSlots[stats.BodyPart],
			"one purchase per slot: %s bought twice", stats.BodyPart)
		boughtSlots[stats.BodyPart] = true
		require.NotEqual(t, int32(1121), purchase.ItemID,
			"the apprentice's shoes are the intermediate of the feet slot")
	}
	require.True(t, boughtSlots["lhand"] || boughtSlots["rhand"],
		"the plan fills the cheap defense slots first")

	// The plan never buys the same item twice.
	seen := make(map[int32]bool)
	for _, purchase := range purchases {
		require.False(t, seen[purchase.ItemID])
		seen[purchase.ItemID] = true
	}
}

func TestPlanPurchasesOneWeaponPerTrip(t *testing.T) {
	profile := MeleeFighter{}
	// A rich character buys ONE weapon per trip: the top-tier guard
	// keeps the slot ladder at its best affordable step, so the Long
	// Sword (24 pAtk, the top shop weapon) is bought directly - never
	// the cheap ladder rungs the value per adena ranking would pick
	// first (the knife, the short sword, the sickle).
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(
		profile, equipment, elvenCatalog(), 10_000_000)
	require.NotEmpty(t, purchases)
	weapons := 0
	foundLongSword := false
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok, "every purchase must carry gear stats")
		if stats.BodyPart == "rhand" || stats.BodyPart == "lrhand" {
			weapons++
			foundLongSword = purchase.ItemID == 2
		}
	}
	require.Equal(t, 1, weapons,
		"one weapon purchase per trip, the chain is cut")
	require.True(t, foundLongSword,
		"the Long Sword is the top tier of the affordable weapon ladder")
	for _, purchase := range purchases {
		require.NotEqual(t, int32(1), purchase.ItemID,
			"the short sword intermediate must not be bought")
		require.NotEqual(t, int32(12), purchase.ItemID,
			"the knife intermediate must not be bought")
		require.NotEqual(t, int32(153), purchase.ItemID,
			"the sickle intermediate must not be bought")
		require.NotEqual(t, int32(3), purchase.ItemID,
			"the broadsword intermediate must not be bought")
		require.NotEqual(t, int32(66), purchase.ItemID,
			"the gladius intermediate must not be bought")
		require.NotEqual(t, int32(216), purchase.ItemID,
			"the dirk intermediate must not be bought")
	}
	require.LessOrEqual(t, AdenaSpent(purchases), int64(10_000_000))
}

// TestPlanPurchasesNoDuplicateNecklace reproduces the live regression
// of the first rich session: the plan bought the Necklace of Magic
// AND the Necklace of Knowledge in one walk and only the better one
// was ever worn. One purchase per slot keeps the adena. The top-tier
// guard narrows the neck ladder further: when a necklace is bought at
// all, it is the top affordable tier (the Necklace of Wisdom) - the
// cheaper Magic and Knowledge rungs stay out of the plan, which either
// buys the Wisdom or leaves the neck to the next trip.
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
			require.Equal(t, int32(908), purchase.ItemID,
				"the Wisdom, the top neck tier, or no necklace at all")
		}
	}
	require.LessOrEqual(t, necks, 1,
		"one necklace per trip: never both the Magic and the Knowledge")
	require.LessOrEqual(t, AdenaSpent(purchases), int64(40_000))
}

func TestPlanPurchasesSkipsInventoryItems(t *testing.T) {
	profile := MeleeFighter{}
	// The inventory carries an unequipped broadsword (3): the free
	// upgrade is simulated first, so the shop broadsword is never
	// planned and the single weapon purchase of the trip is the top
	// affordable tier over it (the Long Sword) - the cheaper rungs
	// (the dirk among them) are intermediate steps the top-tier guard
	// drops.
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
			require.Equal(t, int32(2), purchase.ItemID,
				"the Long Sword is the top tier over the carried broadsword")
		}
		require.NotEqual(t, int32(216), purchase.ItemID,
			"the dirk intermediate must not be bought")
	}
	require.Equal(t, 1, weapons, "one weapon purchase per trip")
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

// TestPlanPurchasesCreditsDisplacedGear pins the sell credit of the
// replacement planning: a purchase that displaces an equipped piece
// carries its SellFirst object ids and its sell value, and the
// affordability counts the credit - 56000 adena alone cannot pay the
// dirk (62214), the 9250 credit of the equipped sickle closes the
// gap, so the plan spends past the carried adena.
func TestPlanPurchasesCreditsDisplacedGear(t *testing.T) {
	profile := MeleeFighter{}
	// The character wears the sickle (18500 reference price).
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 153)},
		map[Slot]int32{SlotRHand: 100})
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 56000)
	var weapon *Purchase
	for index := range purchases {
		stats, ok := npcdata.ItemGearStats(purchases[index].ItemID)
		require.True(t, ok)
		if stats.BodyPart == "rhand" || stats.BodyPart == "lrhand" {
			weapon = &purchases[index]
		}
	}
	require.NotNil(t, weapon,
		"the dirk upgrade appears only through the sell credit")
	require.Equal(t, []int32{100}, weapon.SellFirst,
		"the equipped sickle is sold before the buy")
	require.Equal(t, npcdata.ItemPrice(153)/2, weapon.SellCredit)
	require.Greater(t, AdenaSpent(purchases), int64(56000),
		"the plan spends past the carried adena through the credit")
}

// TestPlanPurchasesNoCreditWithoutEquippedGear pins the empty slot
// case: an empty weapon slot displaces nothing, the fillers carry no
// sell credit and the spend stays inside the carried adena.
func TestPlanPurchasesNoCreditWithoutEquippedGear(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 56000)
	require.LessOrEqual(t, AdenaSpent(purchases), int64(56000))
	require.Zero(t, SellCreditOf(purchases))
	for _, purchase := range purchases {
		require.Empty(t, purchase.SellFirst)
	}
}

// weaponCatalog is the weapon shop slice of the elven catalog (the
// Unoren buylist only): the weapon ladder tests plan against it, so
// the armor and jewel fillers of the other merchants stay out of the
// picture.
func weaponCatalog() Catalog {
	return Catalog{Shops: []Shop{
		{MerchantTemplateID: 7147, TaxRate: 0.15, Lists: []int32{3014700}},
	}}
}

// weaponPurchases returns the weapon purchases of the plan.
func weaponPurchases(purchases []Purchase) []Purchase {
	var weapons []Purchase
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		if ok && (stats.BodyPart == "rhand" || stats.BodyPart == "lrhand") {
			weapons = append(weapons, purchase)
		}
	}

	return weapons
}

// TestPlanPurchasesTopTierAfterSaleReplan reproduces the live
// regression of the shopping queue: a bot sold its replaced weapon and
// re-plans at the shop with the fresh adena and the EMPTY weapon slot.
// The carried adena covers the ~62k top tier (the two hand Brandish),
// but the value per adena ranking of the old planner bought the 883
// adena short sword first and the one purchase per slot guard blocked
// the top tier for the trip. The top-tier guard plans the top
// affordable weapon directly - no intermediate rung in between.
func TestPlanPurchasesTopTierAfterSaleReplan(t *testing.T) {
	profile := MeleeFighter{}
	// The post sale wallet: the carried adena plus the sale proceeds
	// reaches the Brandish (54100 reference, 62215 with the tax, the
	// 21x325 = 6825 score tops the same price dirk's 6495) with 35
	// adena to spare.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(
		profile, equipment, weaponCatalog(), 62250)
	weapons := weaponPurchases(purchases)
	require.Len(t, weapons, 1, "one weapon purchase per trip")
	require.Equal(t, int32(1333), weapons[0].ItemID,
		"the Brandish, the top affordable tier, is bought directly")
	require.NotEqual(t, int32(1), weapons[0].ItemID,
		"the 1k short sword intermediate must never be bought")
	require.Empty(t, weapons[0].SellFirst,
		"the empty slot displaces nothing")
	require.LessOrEqual(t, AdenaSpent(purchases), int64(62250))
}

// TestPlanPurchasesReplacedWeaponTargetsTopTier pins the hunt side of
// the same story: the bot wears the broadsword (12500 reference, the
// 14k class weapon) and the carried adena plus its sale credit reaches
// the ~60k Brandish. The plan must carry the top tier with the
// SellFirst sale of the worn weapon - the trip then sells the
// broadsword and the re-planned buys buy the Brandish, never an
// intermediate sword.
func TestPlanPurchasesReplacedWeaponTargetsTopTier(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 3)},
		map[Slot]int32{SlotRHand: 100})
	purchases := PlanPurchases(
		profile, equipment, weaponCatalog(), 56000)
	weapons := weaponPurchases(purchases)
	require.Len(t, weapons, 1, "one weapon purchase per trip")
	require.Equal(t, int32(1333), weapons[0].ItemID,
		"the Brandish is the top tier the adena plus the credit reaches")
	require.Equal(t, []int32{100}, weapons[0].SellFirst,
		"the worn broadsword is sold before the buy")
	require.Equal(t, npcdata.ItemPrice(3)/2, weapons[0].SellCredit,
		"the credit is the Mobius sell value of the broadsword")
	require.Greater(t, AdenaSpent(purchases), int64(56000),
		"the plan spends past the carried adena through the credit")
}

// TestPlanPurchasesIntermediateNeverFitsUnderTopTier pins the cross
// round guard: the plan buys armor fillers of the other slots first
// (the documented value per adena priority) and the eroded budget no
// longer reaches the top weapon - the weapon slot stays unpurchased
// and waits for the next trip instead of buying the intermediate rung
// the eroded budget suddenly fits.
func TestPlanPurchasesIntermediateNeverFitsUnderTopTier(t *testing.T) {
	profile := MeleeFighter{}
	// The bare character with the post sale wallet: the armor and
	// jewel fillers go out first and push the budget under the dirk
	// price; the short sword must not slide in behind them.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 62250)
	weapons := weaponPurchases(purchases)
	for _, weapon := range weapons {
		require.NotEqual(t, int32(1), weapon.ItemID,
			"the 1k short sword intermediate must never be bought")
	}
	if len(weapons) > 0 {
		require.Equal(t, int32(1333), weapons[0].ItemID,
			"when a weapon fits the plan, it is the top tier")
	}
	require.LessOrEqual(t, AdenaSpent(purchases), int64(62250))
}
