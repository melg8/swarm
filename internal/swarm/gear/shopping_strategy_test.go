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

// This file pins the shop strategy against the level journey of an
// elven fighter: the character leaves the creation screen with the
// Squire's kit (Squire's Sword, Squire's Shirt, Squire's Pants, a
// free Dagger in the bag) and zero adena, hunts the elven lands mob
// ladder and visits the town shops once per level. The journey runs
// twice - once through the legacy greedy planner (the value per
// adena walk the strategy replaced) and once through the phased
// planner - so the test prints the was/is comparison of every trip
// and asserts the ordering rules of the new strategy: the cheap armor
// floor opens the journey, the weapon milestone follows, the jewelry
// waits for the filled armor slots and the worn weapon.

// journeyIncome models the adena the mob ladder of the elven lands
// pays per level: the exp table of the Mobius C1 experience.xml
// divided by the exp of the mob the level hunts (the keltirs 1-4,
// the black wolves, the goblins 5, the kaboo orcs 6-12, the kasha
// spiders 13+), each kill dropping adena at 70 percent chance of the
// min-max average of the npc drop lists. Gear drops sold to the
// shops are not counted (pure luck, the planner handles them as
// free inventory upgrades anyway), so the model is the steady
// income floor.
var journeyIncome = map[int]int64{
	1: 20, 2: 40, 3: 130, 4: 340, 5: 860, 6: 1_840, 7: 3_330,
	8: 6_100, 9: 10_700, 10: 15_500, 11: 22_100, 12: 27_100,
	13: 36_800, 14: 48_900, 15: 61_100, 16: 78_600, 17: 97_500,
	18: 122_000, 19: 151_000, 20: 185_000,
}

// journeyTopLevel bounds the simulated journey through the top of
// the elven lands income table.
const journeyTopLevel = 20

// legacyBestPurchase picks the affordable candidate with the highest
// score gain per adena: the greedy rule the shop strategy used
// before the phase rework (the cheap empty slot fillers outranked
// the weapon upgrades, the jewel ladder mixed into the armor picks).
// The walk loop it serves is the plain planPurchases of this file
// with tail 0; the copy lives here so the comparison test can run
// the exact old selection against the same catalogs, candidates and
// family logic the new strategy uses.
func legacyBestPurchase(
	virtual [slotCount]ScoredItem, candidates []purchaseCandidate,
	budget int64, planned map[int32]bool, boughtSlots map[Slot]bool,
	equipment Equipment,
) (*purchaseCandidate, float64, int64, []int32) {
	var best *purchaseCandidate
	bestValue := float64(0)
	bestGain := float64(0)
	var bestCredit int64
	var bestSellFirst []int32
	for index := range candidates {
		candidate := &candidates[index]
		if planned[candidate.itemID] {
			continue
		}
		credit, sellFirst := displacedValue(equipment, affectedSlots(
			virtual, candidate.stats.BodyPart).slice())
		if candidate.price > budget+credit {
			continue
		}
		gain, ok := purchaseGain(virtual, candidate.stats, candidate.score)
		if !ok {
			continue
		}
		if slotBlocked(virtual, candidate.stats.BodyPart, boughtSlots) {
			continue
		}
		value := gain
		if candidate.price > 0 {
			value = gain / float64(candidate.price)
		}
		if value > bestValue || (value == bestValue && gain > bestGain) {
			best = candidate
			bestValue = value
			bestGain = gain
			bestCredit = credit
			bestSellFirst = sellFirst
		}
	}

	return best, bestGain, bestCredit, bestSellFirst
}

// legacyPlanPurchases is the pre-rework planner: the greedy value
// per adena walk (the same trip loop, the legacy best pick). It
// takes no character level - the old strategy never looked at it.
func legacyPlanPurchases(
	profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase {
	virtual := SimulateInventory(profile, equipment)
	candidates := catalogCandidates(profile, catalog)
	purchases := make([]Purchase, 0, len(candidates))
	budget := adena
	planned := make(map[int32]bool)
	boughtSlots := make(map[Slot]bool)
	spent := int64(0)
	credited := int64(0)
	for budget > 0 {
		best, gain, credit, sellFirst := legacyBestPurchase(
			virtual, candidates, budget, planned, boughtSlots, equipment)
		if best == nil || gain <= 0 {
			break
		}
		planned[best.itemID] = true
		budget += credit
		budget -= best.price
		spent += best.price
		credited += credit
		purchases = append(purchases, walkedPurchase(
			best, gain, credit, sellFirst, adena, spent, credited, true))
		for _, slot := range affectedSlots(virtual, best.stats.BodyPart).slice() {
			boughtSlots[slot] = true
		}
		applyToVirtual(&virtual, boughtEntry(best))
	}

	return purchases
}

// journeyRecord carries one purchase trip of the journey: the
// level it happened at, the wallet the planner split and the
// purchases of the strategy under test.
type journeyRecord struct {
	level  int
	wallet int64
	is     []Purchase
}

// journeyPlanner abstracts the strategy under test: the legacy copy
// and the phased PlanPurchases share the signature.
type journeyPlanner func(
	profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase

// runJourney walks the levels 1..journeyTopLevel: every level adds
// its income to the wallet, the planner plans the trip against the
// current gear, the affordable purchases are bought (the displaced
// pieces sell at reference/2, the displaced starter kit pieces are
// destroyed - the shops refuse them), and the auto equipment equips
// everything the bag now holds. The trips with purchases are
// returned for the comparison.
func runJourney(
	t *testing.T, planner journeyPlanner,
) []journeyRecord {
	t.Helper()
	profile := MeleeFighter{}
	equipment := journeyStart()
	wallet := int64(0)
	nextObjectID := int32(1000)
	var trips []journeyRecord
	for level := 1; level <= journeyTopLevel; level++ {
		wallet += journeyIncome[level]
		var legacy []Purchase
		if planner == nil {
			legacy = legacyPlanPurchases(
				profile, equipment, elvenCatalog(), wallet)
		} else {
			legacy = planner(profile, equipment, elvenCatalog(), wallet)
		}
		if len(legacy) > 0 {
			trips = append(trips, journeyRecord{
				level: level, wallet: wallet, is: legacy,
			})
		}
		wallet, nextObjectID, equipment = applyTrip(
			profile, equipment, legacy, wallet, nextObjectID)
	}

	return trips
}

// journeyStart builds the creation screen state: the Squire's kit
// worn, the free Dagger in the bag.
func journeyStart() Equipment {
	return equipmentWith(
		[]state.InventoryItem{
			item(1, 2369), item(2, 1146), item(3, 1147), item(4, 10),
		},
		map[Slot]int32{
			SlotRHand: 1, SlotChest: 2, SlotLegs: 3,
		})
}

// applyTrip executes the purchases of one trip in order. Every buy
// first pays with its displaced pieces (sold at reference/2; the
// starter kit pieces are destroyed instead - the shops refuse them,
// so they pay nothing), and a buy the wallet plus the real proceeds
// cannot cover is skipped - the server rejects the request, the trip
// moves on and the next trip re-plans the piece. The bought pieces
// join the bag and the auto equipment walks the paperdoll to its
// optimum. The real-credit re-check matters because the planner
// prices the displaced starter kit at reference/2 as well: its plan
// may overshoot the true wallet by those few adena, and the skip
// models exactly what the server then does.
func applyTrip(
	profile Profile, equipment Equipment, purchases []Purchase,
	wallet int64, nextObjectID int32,
) (int64, int32, Equipment) {
	for index := range purchases {
		purchase := purchases[index]
		credit := int64(0)
		for _, objectID := range purchase.SellFirst {
			if !starterSet[objectItemID(equipment, objectID)] {
				credit += npcdata.ItemPrice(
					objectItemID(equipment, objectID)) / 2
			}
		}
		if wallet+credit < purchase.Price {
			continue
		}
		for _, objectID := range purchase.SellFirst {
			itemID := objectItemID(equipment, objectID)
			if !starterSet[itemID] {
				wallet += npcdata.ItemPrice(itemID) / 2
			}
			equipment = removeInventoryItem(equipment, objectID)
		}
		wallet -= purchase.Price
		equipment.Items = append(equipment.Items,
			state.InventoryItem{
				ObjectID: nextObjectID,
				ItemID:   purchase.ItemID,
				Count:    1,
			})
		nextObjectID++
	}
	equipment = autoEquip(profile, equipment)

	return wallet, nextObjectID, equipment
}

// removeInventoryItem drops an inventory entry (sold or destroyed)
// and clears its paperdoll slot so no ghost object id survives.
func removeInventoryItem(equipment Equipment, objectID int32) Equipment {
	items := make([]state.InventoryItem, 0, len(equipment.Items))
	for _, entry := range equipment.Items {
		if entry.ObjectID == objectID {
			continue
		}
		items = append(items, entry)
	}
	equipment.Items = items
	for slot := Slot(0); slot < slotCount; slot++ {
		if equipment.Slots[slot] == objectID {
			equipment.Slots[slot] = 0
		}
	}

	return equipment
}

// objectItemID resolves the item id of an inventory object id.
func objectItemID(equipment Equipment, objectID int32) int32 {
	item, ok := equipment.itemByID(objectID)
	if !ok {
		return 0
	}

	return item.ItemID
}

// autoEquip applies NextUpgrade until the paperdoll is optimal for
// the bag: the same walk the hunt loop runs after every loot, buy
// and sell.
func autoEquip(profile Profile, equipment Equipment) Equipment {
	for {
		action, ok := NextUpgrade(profile, equipment)
		if !ok {
			return equipment
		}
		if !action.Equip {
			equipment.Slots[action.Slot] = 0
			equipment.setItemEquipped(action.ObjectID, false)

			continue
		}
		equipment.Slots[action.Slot] = action.ObjectID
		equipment.setItemEquipped(action.ObjectID, true)
	}
}

// setItemEquipped flips the equipped flag of an inventory entry.
func (e *Equipment) setItemEquipped(objectID int32, equipped bool) {
	for index := range e.Items {
		if e.Items[index].ObjectID == objectID {
			e.Items[index].Equipped = equipped

			return
		}
	}
}

// TestShoppingStrategyJourneyComparison walks the level journey
// through both planners and pins the ordering rules of the phased
// strategy: the weapon milestone leads every trip that affords one,
// the pdef maximizing armor set follows, no jewel runs before the
// first weapon and the jewels stay on the basic floor set through the
// whole journey (the starting locations barely attack with magic).
func TestShoppingStrategyJourneyComparison(t *testing.T) {
	wasTrips := runLegacyJourney(t)
	isTrips := runJourney(t, PlanPurchases)

	printJourney(t, "WAS (greedy value per adena)", wasTrips)
	printJourney(t, "IS (weapon milestone, pdef-maximizing armor set, "+
		"basic jewels)", isTrips)

	// Rule 1: the weapon milestone leads every trip that affords one -
	// the first purchase of such a trip is the weapon, the armor set
	// and the jewels follow behind it. The trips below every weapon
	// tier spend the wallet on the pdef maximizing armor set alone.
	for _, trip := range isTrips {
		weapons := weaponPurchases(trip.is)
		if len(weapons) == 0 {
			continue
		}
		first := trip.is[0]
		require.Equal(t, CategoryWeapon,
			CategoryOf(candidateStats(t, first.ItemID)),
			"level %d: the weapon milestone leads the trip", trip.level)
	}

	// Rule 2: the first weapon purchase comes before any jewel
	// purchase of the whole journey - the jewelry waits for the worn
	// weapon (the user rule of the opening game).
	weaponFirst := firstWeaponIndex(t, isTrips)
	require.Positive(t, weaponFirst,
		"the journey must buy a weapon")
	flattened := flattenTrips(isTrips)
	for _, purchase := range flattened[:weaponFirst] {
		require.NotEqual(t, CategoryJewel,
			CategoryOf(candidateStats(t, purchase.ItemID)),
			"no jewel runs before the first weapon")
	}

	// Rule 3: the jewels stay basic through the whole journey - the
	// starting locations barely attack with magic, the floor items are
	// the only jewel purchases any level plans.
	floorIDs := cheapestJewelIDs(catalogCandidates(
		MeleeFighter{}, elvenCatalog()))
	for _, trip := range isTrips {
		for _, purchase := range trip.is {
			if CategoryOf(candidateStats(t, purchase.ItemID)) !=
				CategoryJewel {
				continue
			}
			require.True(t, floorIDs[purchase.ItemID],
				"level %d buys the jewel %d outside the floor",
				trip.level, purchase.ItemID)
		}
	}

	// Rule 4: the acceptance wallet of the farm readiness round - a
	// bare level 15 character with 100,000 adena buys the weapon
	// milestone (the brandish), the pdef maximizing armor set (the
	// summed armor pdef reaches past the cheap floor by a wide margin -
	// the aggressive adena utilization) and the basic jewel set (both
	// halves of the pairs included) in ONE plan.
	profile := MeleeFighter{}
	equipment := equipmentWith(nil, nil)
	plan := PlanPurchases(profile, equipment, elvenCatalog(), 100_000)
	require.NotEmpty(t, plan)
	weapons := weaponPurchases(plan)
	require.Len(t, weapons, 1, "one weapon milestone")
	require.Equal(t, int32(1333), weapons[0].ItemID,
		"the brandish is the top affordable tier of 100k adena")
	var armorPdef int32
	jewels := 0
	for _, purchase := range plan {
		stats := candidateStats(t, purchase.ItemID)
		switch CategoryOf(stats) {
		case CategoryArmor:
			armorPdef += stats.PDef
		case CategoryJewel:
			jewels++
		case CategoryShield, CategoryWeapon, CategoryUnusable:
		}
	}
	require.GreaterOrEqual(t, armorPdef, int32(120),
		"the armor set maximizes the pdef of the leftover budget")
	require.Equal(t, 5, jewels,
		"the basic jewel set fills every slot, the pair halves included")
	require.LessOrEqual(t, AdenaSpent(plan), int64(100_000))

	// Rule 5: the legacy strategy bought the cheap fillers in a greedy
	// value per adena soup with no phases (the apprentice's shoes at
	// 8 adena open the journey) - the phased strategy now opens with
	// the same cheap armor deliberately, but the order behind it is
	// the phase walk, not the value soup.
	wasFlattened := flattenTrips(wasTrips)
	wasWeapon := firstWeaponIndex(t, wasTrips)
	require.Positive(t, wasWeapon,
		"the greedy planner buys armor fillers before the weapon")
	require.Equal(t, int32(1121), wasFlattened[0].ItemID,
		"the greedy journey opens with the apprentice's shoes")
}

// runLegacyJourney walks the journey through the legacy planner.
func runLegacyJourney(t *testing.T) []journeyRecord {
	t.Helper()

	return runJourney(t, legacyPlanPurchases)
}

// printJourney prints one journey leg for the comparison log.
func printJourney(t *testing.T, label string, trips []journeyRecord) {
	t.Helper()
	t.Logf("=== %s ===", label)
	for _, trip := range trips {
		t.Logf("level %2d (wallet %7d):", trip.level, trip.wallet)
		for _, purchase := range trip.is {
			t.Logf("    %-28s %6d adena",
				npcdata.ItemName(purchase.ItemID), purchase.Price)
		}
	}
}

// firstWeaponIndex finds the flattened index of the first weapon
// purchase of the journey.
func firstWeaponIndex(t *testing.T, trips []journeyRecord) int {
	t.Helper()
	for index, purchase := range flattenTrips(trips) {
		if CategoryOf(candidateStats(t, purchase.ItemID)) ==
			CategoryWeapon {
			return index
		}
	}

	return -1
}

// flattenTrips joins the purchases of every trip in order.
func flattenTrips(trips []journeyRecord) []Purchase {
	var purchases []Purchase
	for _, trip := range trips {
		purchases = append(purchases, trip.is...)
	}

	return purchases
}

// candidateStats resolves the gear stats of an item id for the
// category assertions.
func candidateStats(t *testing.T, itemID int32) npcdata.GearStats {
	t.Helper()
	stats, ok := npcdata.ItemGearStats(itemID)
	require.True(t, ok, "item %d carries no gear stats", itemID)

	return stats
}
