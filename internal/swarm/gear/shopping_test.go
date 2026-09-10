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
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 0, 1)
	require.Empty(t, purchases)
}

// TestPlanPurchasesArmorFloorFirst pins the opening picks of a bare
// character: the armor floor (the cheapest armor pieces filling the
// empty armor slots) comes before everything - the cheap armor runs
// ahead of the weapon and the jewels, and the jewelry never appears
// while no real weapon is worn (the jewel floor waits for the
// milestone).
func TestPlanPurchasesArmorFloorFirst(t *testing.T) {
	profile := MeleeFighter{}
	// A bare character with 500 adena: the floor buys the cheapest
	// armor set of the empty armor slots (the chest, legs, head,
	// gloves and feet pieces, cheapest first), nothing else - the
	// weapon at 883 stays out of reach and the jewel floor stays
	// closed behind the missing real weapon.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 500, 1)
	require.NotEmpty(t, purchases)
	require.LessOrEqual(t, AdenaSpent(purchases), int64(500))
	first := purchases[0]
	require.Equal(t, int32(1121), first.ItemID,
		"the apprentice's shoes are the cheapest armor floor fill")
	require.Equal(t, buyPrice(1121), first.Price)
	require.Equal(t, int32(3014800), first.ListID)
	require.Equal(t, int32(7148), first.MerchantTemplateID)
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok)
		require.Equal(t, CategoryArmor, CategoryOf(stats),
			"the opening plan buys the cheap armor floor only")
	}

	// The plan never buys the same item twice.
	seen := make(map[int32]bool)
	for _, purchase := range purchases {
		require.False(t, seen[purchase.ItemID])
		seen[purchase.ItemID] = true
	}

	// A worn real weapon opens the jewel floor behind the armor: the
	// same bare character now wields the short sword, and the plan
	// fills the empty armor slots first, then the cheapest jewel set
	// (one ring, one earring, one necklace this trip - the pair
	// halves wait for the next) and the defense upgrades that fit
	// the short sword budget (the leather shield). The next weapon
	// milestone (the knife at 14374) stays out of reach of the 1000
	// adena wallet.
	equipped := equipmentWith(
		[]state.InventoryItem{item(100, 1)},
		map[Slot]int32{SlotRHand: 100})
	plan := PlanPurchases(profile, equipped, elvenCatalog(), 1000, 1)
	require.NotEmpty(t, plan)
	require.LessOrEqual(t, AdenaSpent(plan), int64(1000))
	armorSeen, jewelSeen := 0, 0
	for _, purchase := range plan {
		category := CategoryOf(candidateStats(t, purchase.ItemID))
		switch category {
		case CategoryArmor, CategoryShield:
			armorSeen++
		case CategoryJewel:
			jewelSeen++
		default:
			t.Fatalf("unexpected purchase category %d of item %d",
				category, purchase.ItemID)
		}
	}
	require.Positive(t, armorSeen, "the armor floor still fills the slots")
	require.Positive(t, jewelSeen,
		"the jewel floor opens behind the worn real weapon")
	jewelStart := -1
	for index, purchase := range plan {
		if CategoryOf(candidateStats(t, purchase.ItemID)) == CategoryJewel {
			jewelStart = index

			break
		}
	}
	for _, purchase := range plan[:jewelStart] {
		require.Equal(t, CategoryArmor,
			CategoryOf(candidateStats(t, purchase.ItemID)),
			"the armor fills run ahead of the jewels")
	}
}

func TestPlanPurchasesOneWeaponPerTrip(t *testing.T) {
	profile := MeleeFighter{}
	// A rich character buys ONE weapon per trip: the best value
	// weapon milestone (the short sword from bare fists) takes the
	// weapon slot and the upgrade chain (the knife, the broadsword,
	// the sickle, the long sword) is cut - the next trip re-plans
	// against the sword this one reached.
	equipment := equipmentWith(nil, nil)
	purchases := PlanPurchases(
		profile, equipment, elvenCatalog(), 10_000_000, 1)
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
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 40_000, 1)
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
	// value upgrade over it (the brandish: the two hand sword gains
	// more score per adena than the dirk and nothing blocked its left
	// hand slot ahead of the weapon phase).
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 3)}, nil)
	purchases := PlanPurchases(
		profile, equipment, elvenCatalog(), 10_000_000, 1)
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
		if purchase.ItemID == 1333 {
			found = true
		}
	}
	require.True(t, found,
		"the brandish, the best value upgrade over the broadsword, is planned")
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
		profile, equipment, elvenCatalog(), 10_000_000, 1)
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
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 56000, 1)
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
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 56000, 1)
	require.LessOrEqual(t, AdenaSpent(purchases), int64(56000))
	require.Zero(t, SellCreditOf(purchases))
	for _, purchase := range purchases {
		require.Empty(t, purchase.SellFirst)
	}
}

// TestPlanPurchaseQueueMatchesPlainPlan pins the queue walker against
// the plain planner: the affordable prefix of a purchase queue holds
// exactly the PlanPurchases picks (same items, same order), the
// affordability and the missing amounts of the prefix stay zero.
func TestPlanPurchaseQueueMatchesPlainPlan(t *testing.T) {
	profile := MeleeFighter{}
	for _, adena := range []int64{0, 500, 56000, 10_000_000} {
		equipment := equipmentWith(nil, nil)
		plan := PlanPurchases(profile, equipment, elvenCatalog(), adena, 1)
		queue := PlanPurchaseQueue(profile, equipment, elvenCatalog(),
			adena, 1)
		affordable := affordablePurchases(queue)
		require.Equal(t, plan, affordable,
			"the queue prefix must match the plain plan")
		for index := range affordable {
			require.True(t, affordable[index].Affordable)
			require.Zero(t, affordable[index].Missing,
				"an affordable entry misses no adena")
		}
		if len(queue) > len(plan) {
			require.False(t, queue[len(plan)].Affordable,
				"the first entry past the plan is a wanted one")
		}
	}
}

// TestPlanPurchaseQueueWantedTail pins the wanted tail of a queue: a
// poor wallet plans no affordable buys, every queue entry is wanted
// and the missing amounts grow with the cumulative prices minus the
// planning adena and the sell credits.
func TestPlanPurchaseQueueWantedTail(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith(nil, nil)
	queue := PlanPurchaseQueue(profile, equipment, elvenCatalog(), 0, 1)
	require.NotEmpty(t, queue, "a bare character still wants gear")
	adena := int64(0)
	cumulative := int64(0)
	credit := int64(0)
	for _, purchase := range queue {
		require.False(t, purchase.Affordable,
			"a zero wallet affords nothing")
		cumulative += purchase.Price
		credit += purchase.SellCredit
		missing := cumulative - adena - credit
		if missing < 0 {
			missing = 0
		}
		require.Equal(t, missing, purchase.Missing,
			"the missing amount tracks the cumulative shortfall")
	}
	require.LessOrEqual(t, len(queue), shoppingQueueTail,
		"the tail stays bounded")
}

// TestPlanPurchaseQueueRichNeedsNoTail pins the rich wallet: an
// unbounded adena plans the same full queue with no wanted entries.
func TestPlanPurchaseQueueRichNeedsNoTail(t *testing.T) {
	profile := MeleeFighter{}
	equipment := equipmentWith(nil, nil)
	queue := PlanPurchaseQueue(
		profile, equipment, elvenCatalog(), 10_000_000, 1)
	require.NotEmpty(t, queue)
	for _, purchase := range queue {
		require.True(t, purchase.Affordable)
		require.Zero(t, purchase.Missing)
	}
}

// TestPlanPurchaseQueueCreditsDisplacedGear pins the sell credit
// accounting of the tail: a wanted replacement carries the SellFirst
// object ids and the sell value of the piece it displaces (the same
// credit semantics the affordable planner applies, see
// TestPlanPurchasesCreditsDisplacedGear), and its missing amount
// counts that credit - the shortfall stays below the raw price.
func TestPlanPurchaseQueueCreditsDisplacedGear(t *testing.T) {
	profile := MeleeFighter{}
	// The character wears the sickle (18500 reference price) and
	// carries nothing: the dirk (62214 with tax) is the wanted
	// weapon upgrade, the sickle's sale pays 9250 of it.
	equipment := equipmentWith(
		[]state.InventoryItem{item(100, 153)},
		map[Slot]int32{SlotRHand: 100})
	// A long tail: the weapon upgrade ranks low on the value per
	// adena scale, the shoppingQueueTail bound of the widget queue
	// would cut it before the dirk's turn.
	queue := planPurchases(profile, equipment, elvenCatalog(), 0, 1, 40)
	adena := int64(0)
	cumulative := int64(0)
	credit := int64(0)
	var weapon *Purchase
	for index := range queue {
		purchase := &queue[index]
		cumulative += purchase.Price
		credit += purchase.SellCredit
		missing := cumulative - adena - credit
		if missing < 0 {
			missing = 0
		}
		require.Equal(t, missing, purchase.Missing,
			"the missing tracks the cumulative shortfall")
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok)
		if stats.BodyPart == "rhand" || stats.BodyPart == "lrhand" {
			weapon = purchase
		}
	}
	require.NotNil(t, weapon, "the dirk upgrade is the wanted weapon")
	require.False(t, weapon.Affordable)
	require.Equal(t, []int32{100}, weapon.SellFirst,
		"the equipped sickle is sold before the buy")
	require.Equal(t, npcdata.ItemPrice(153)/2, weapon.SellCredit)
}

// affordablePurchases filters the affordable prefix of a queue.
func affordablePurchases(queue []Purchase) []Purchase {
	affordable := make([]Purchase, 0, len(queue))
	for _, purchase := range queue {
		if !purchase.Affordable {
			break
		}
		affordable = append(affordable, purchase)
	}

	return affordable
}

// TestPlannedEquipsListsThePendingWearables pins the keep set of the
// junk flows: the looted upgrades the simulation places on the virtual
// paperdoll are pending equips (never sold, never destroyed), while
// the duplicates, the downgrades and the displaced halves of pair
// swaps stay plain junk.
func TestPlannedEquipsListsThePendingWearables(t *testing.T) {
	profile := MeleeFighter{}
	// The short sword is worn, the looted broadsword upgrades it and
	// the second broadsword is a surplus duplicate.
	equipment := equipmentWith(
		[]state.InventoryItem{
			item(100, shortSwordID), item(101, broadswordID),
			item(102, broadswordID),
		},
		map[Slot]int32{SlotRHand: 100})
	keeps := PlannedEquips(profile, equipment)
	require.Equal(t, map[int32]bool{101: true}, keeps,
		"the looted broadsword upgrade is the pending equip, its "+
			"duplicate and the worn sword are not")

	// A looted downgrade never becomes a pending equip: the worn
	// broadsword beats it.
	equipment = equipmentWith(
		[]state.InventoryItem{item(100, broadswordID), item(101, shortSwordID)},
		map[Slot]int32{SlotRHand: 100})
	require.Empty(t, PlannedEquips(profile, equipment),
		"the looted downgrade stays junk")

	// The pair swap mid flight: one apprentice earring is worn, the
	// weaker one already came off, the looted mystic earring waits for
	// its equip behind the pacing window. The mystic is the pending
	// equip, the displaced apprentice stays junk (it is about to be
	// sold anyway).
	equipment = equipmentWith(
		[]state.InventoryItem{
			item(100, apprenticeEarringID), item(101, apprenticeEarringID),
			item(102, mysticEarringID),
		},
		map[Slot]int32{SlotREar: 100})
	require.Equal(t, map[int32]bool{102: true},
		PlannedEquips(profile, equipment),
		"the looted mystic earring is the pending equip, the displaced "+
			"apprentice earring stays junk")
}

// TestPlanPurchasesStarterWeaponCarriesNoCredit pins the phantom
// credit fix of the reported loop: a character wearing the starter
// dagger plans the short sword milestone, but no shop ever buys the
// dagger (is_sellable=false), so the plan carries neither its
// SellFirst id nor its sell value - the affordability needs the full
// carried adena. The legacy credit of 69 adena once let an 850 adena
// wallet plan the 883 adena sword, walk to town and walk back empty.
func TestPlanPurchasesStarterWeaponCarriesNoCredit(t *testing.T) {
	profile := MeleeFighter{}
	// The character wears the starter dagger (10) and nothing else
	// of the weapon family.
	equipment := equipmentWith(
		[]state.InventoryItem{item(910, 10)},
		map[Slot]int32{SlotRHand: 910})

	// 850 adena + the phantom 69 credit covered the 883 sword
	// before: the plan must stay empty without the credit.
	purchases := PlanPurchases(profile, equipment, elvenCatalog(), 850, 1)
	for _, purchase := range purchases {
		stats, ok := npcdata.ItemGearStats(purchase.ItemID)
		require.True(t, ok)
		require.NotEqual(t, "rhand", stats.BodyPart,
			"the 883 sword stays out of reach of an 850 wallet")
	}

	// With the armor floor (387 adena) plus the full sword price on
	// hand the milestone is planned - without any sell-first step
	// or credit for the unsellable dagger.
	purchases = PlanPurchases(profile, equipment, elvenCatalog(), 1300, 1)
	var weapon *Purchase
	for index := range purchases {
		stats, ok := npcdata.ItemGearStats(purchases[index].ItemID)
		require.True(t, ok)
		if stats.BodyPart == "rhand" || stats.BodyPart == "lrhand" {
			weapon = &purchases[index]
		}
	}
	require.NotNil(t, weapon, "the affordable sword milestone is planned")
	require.Empty(t, weapon.SellFirst,
		"the unsellable dagger never feeds a sell-first step")
	require.Zero(t, weapon.SellCredit,
		"the unsellable dagger never credits the budget")
	require.Equal(t, buyPrice(1), weapon.Price)
}
