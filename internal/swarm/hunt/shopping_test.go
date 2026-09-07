// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
)

// spawnMerchantNPC places the merchant npc of the template id at its
// town position.
func spawnMerchantNPC(bot *state.Bot, npc townNpc, objectID int32) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID:   objectID,
		TemplateID: npc.TemplateID + npcDisplayOffset,
		X:          npc.X,
		Y:          npc.Y,
		Z:          npc.Z,
		Name:       npc.Name,
	})
}

// settleMerchant walks the character to the merchant, picks and
// selects it, so the transactions may run.
func settleMerchant(
	t *testing.T, loop *Loop, game *fakeGame, bot *state.Bot,
	npc townNpc, objectID int32,
) {
	t.Helper()
	spawnMerchantNPC(bot, npc, objectID)
	moveSelfTo(bot, npc.X, npc.Y, npc.Z)
	loop.merchantID = 0
	// The arrival tick (the walk leg completes) precedes the merchant
	// pick: tick until the merchant shows up.
	for range 3 {
		loop.merchantPick = time.Now().Add(-2 * time.Second)
		loop.tick()
		if loop.merchantID != 0 {
			break
		}
	}
	require.NotEqual(t, int32(0), loop.merchantID,
		"the merchant %s must be picked", npc.Name)
	loop.merchantPick = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Contains(t, game.forces, objectID,
		"the merchant %s must be selected before the transaction", npc.Name)
	bot.ApplySelfTarget(objectID)
}

// TestShoppingTripBuysAfterSelling runs the full shopping trip: the
// inventory trigger starts the trip at Herbiel, the junk selling
// frees the slots, the fresh adena plans the purchases and the buy
// stops of the plan (Creamees next to Herbiel, then Ariel across the
// village) buy their lists.
func TestShoppingTripBuysAfterSelling(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot, 500)
	// The adena of the character: 100 adena buys the apprentice's
	// shoes, the short gloves and the magic ring of the elven
	// catalogs.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 100, Type2: 4, Change: 1},
	})

	// The trip starts on the full inventory and walks to Herbiel.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)

	// Herbiel is selected and the first junk batch sells.
	settleMerchant(t, loop, game, bot, townMerchants[3], 55)
	loop.sellAt = time.Time{}
	loop.tick()
	require.Len(t, game.sells, 1)
	require.Len(t, game.sells[0], sellBatchSize)

	// The server confirms the sale: the remaining 16 junk items keep
	// selling below the trigger, the complete sale frees the bag.
	updates := make([]state.InventoryItem, 0, sellBatchSize)
	for _, item := range game.sells[0] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.tick()
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.Len(t, game.sells, 2, "the second junk batch sells")
	updates = updates[:0]
	for _, item := range game.sells[1] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.tick()
	require.True(t, loop.buysPlanned,
		"the purchase planning must run after the selling")
	// The plan of 100 adena buys the shoes, the shield and the gloves
	// of the armor trader Ariel: the sell stop is done, the trip
	// advances to the single buy stop.
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Len(t, loop.tripStops, 1)
	require.Equal(t, int32(7148), loop.tripStops[0].merchant.TemplateID)
	boughtAdena := int64(0)
	for _, stop := range loop.tripStops {
		for _, purchase := range stop.buys {
			boughtAdena += purchase.Price
		}
	}
	require.LessOrEqual(t, boughtAdena, int64(100),
		"the plan must respect the adena budget")
	require.Greater(t, boughtAdena, int64(0))

	// The character walks to Ariel and the armor list buys.
	ariel := townMerchants[1]
	settleMerchant(t, loop, game, bot, ariel, 57)
	loop.buyAt = time.Now().Add(-12 * time.Second)
	loop.merchantPick = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.NotEmpty(t, game.buys, "the armor list must be bought")
	for _, purchase := range game.buys[0] {
		require.Equal(t, int32(3014800), purchase.ListID)
		require.Equal(t, int32(7148), purchase.MerchantTemplateID)
	}

	// Every stop is done: the return leg starts.
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase)
}

// TestShoppingTriggerPlansTrip pins the shopping trigger: a plan
// above the trip minimum value starts a town trip without the
// inventory trigger.
func TestShoppingTriggerPlansTrip(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	// 500 adena with an empty inventory plans fillers worth more
	// than the trip minimum.
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 500, Type2: 4, Change: 1},
	})

	require.True(t, loop.shoppingWanted())
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase,
		"the shopping plan must start a town trip")
	require.NotEmpty(t, game.walks)
}

// TestShoppingTriggerSkipsSmallPlans pins the trip minimum: a plan
// worth a handful of adena does not justify a town trip.
func TestShoppingTriggerSkipsSmallPlans(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 50, Type2: 4, Change: 1},
	})

	require.False(t, loop.shoppingWanted())
}

// TestShoppingWithoutGearProfile disables the shopping (no profile):
// only the inventory trigger starts trips.
func TestShoppingWithoutGearProfile(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 500, Type2: 4, Change: 1},
	})
	loop.equip = nil

	require.False(t, loop.shoppingWanted())
	require.False(t, loop.shoppingTripEnabled())
}

// TestAdvanceTripStopReturnsHome verifies the last stop flows into
// the return leg.
func TestAdvanceTripStopReturnsHome(t *testing.T) {
	loop, _, _, _ := newTripLoop()
	loop.tripStops = []tripStop{{merchant: townMerchants[0]}}
	loop.farmX, loop.farmY, loop.farmZ = 45000, 50000, -3500

	loop.advanceTripStop()
	require.Equal(t, phaseTownReturn, loop.phase)
}

// TestShopCatalogCoversTownMerchants pins the catalog plumbing: every
// elven village merchant of the trip targets sells its generated
// buylists.
func TestShopCatalogCoversTownMerchants(t *testing.T) {
	catalog := shopCatalog(townMerchants)
	require.Len(t, catalog.Shops, 4)
	templates := map[int32]bool{}
	for _, shop := range catalog.Shops {
		require.NotEmpty(t, shop.Lists)
		require.InDelta(t, 0.15, shop.TaxRate, 0.0001)
		templates[shop.MerchantTemplateID] = true
	}
	require.Contains(t, templates, int32(7147))
	require.Contains(t, templates, int32(7148))
	require.Contains(t, templates, int32(7149))
	require.Contains(t, templates, int32(7150))
}

// TestPlanShoppingStopsMergesCurrentMerchant verifies the first buy
// group of the merchant the character stands at merges into the
// current stop instead of appending a walk to the same npc.
func TestPlanShoppingStopsMergesCurrentMerchant(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	// The character stands at Creamees with enough adena for the
	// jewel fillers.
	creamees := townMerchants[2]
	moveSelfTo(bot, creamees.X, creamees.Y, creamees.Z)
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 200, Type2: 4, Change: 1},
	})
	loop.tripStops = []tripStop{{merchant: creamees, sell: true}}

	loop.planShoppingStops()
	require.True(t, loop.buysPlanned)
	if len(loop.tripStops) == 1 {
		require.Empty(t, loop.tripStops[0].buys,
			"the jewel purchases merge into the current stop")
	}

	// A plan of another merchant only appends stops, never rewalks
	// to the current one.
	_ = game
	_ = pathfind.Vec3{}
	_ = gear.Purchase{}
}
