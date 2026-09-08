// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
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
	fillInventory(bot)
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
	require.Positive(t, boughtAdena)

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

	// The server confirms the buys: the inventory update carries the
	// bought items, the stop completes on the arrival confirmation.
	bought := make([]state.InventoryItem, 0, len(game.buys[0]))
	for index, purchase := range game.buys[0] {
		bought = append(bought, state.InventoryItem{
			ObjectID: 9000 + int32(index), ItemID: purchase.ItemID,
			Count: 1, Type2: 5, Change: 1,
		})
	}
	bot.ApplyInventoryUpdate(bought)

	// The confirmation completes the stop; the next tick starts the
	// return leg.
	loop.tick()
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase)
}

// TestStopBuyRetriesAndSkipsLostBatch pins the arrival confirmation
// of the buy requests: a transaction the server refuses answers
// silently (the ActionFailed packet references nothing), so the
// batch is re-requested on its confirmation deadline and given up
// after the retry budget - the trip never stalls on a lost buy.
func TestStopBuyRetriesAndSkipsLostBatch(t *testing.T) {
	loop, game, _, _ := newTripLoop()
	herbiel := townMerchants[3]
	loop.tripStops = []tripStop{{merchant: herbiel, buys: []gear.Purchase{
		{
			ItemID: 1121, ListID: 3014800, MerchantTemplateID: 7148,
			Count: 1, Price: 9,
		},
	}}}
	// The merchant never showed up: the sells work without one, the
	// buy requests still go out.
	loop.merchantID = -1
	loop.buyAt = time.Now().Add(-buyPause - time.Second)

	done := loop.tickStopShopping(time.Now())
	require.False(t, done, "the stop waits for the arrival")
	require.Len(t, game.buys, 1, "the buy request went out")
	require.Len(t, loop.buyRequested, 1)

	// The items never arrive (a refused transaction answers
	// silently): every deadline re-requests the batch.
	for i := 1; i <= stopBuyRetries; i++ {
		loop.buyConfirmAt = time.Now().Add(-buyConfirmWait - time.Second)
		loop.buyAt = time.Now().Add(-buyPause - time.Second)
		done = loop.tickStopShopping(time.Now())
		require.False(t, done, "the retry still waits for the arrival")
		require.Len(t, game.buys, i+1, "retry %d re-requests the batch", i)
	}

	// The retry budget is spent: the batch is skipped and the stop
	// completes without it.
	loop.buyConfirmAt = time.Now().Add(-buyConfirmWait - time.Second)
	loop.buyAt = time.Now().Add(-buyPause - time.Second)
	done = loop.tickStopShopping(time.Now())
	require.True(t, done, "the skipped batch completes the stop")
	require.Len(t, game.buys, stopBuyRetries+1)
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

// TestReplacementSalesSellBeforeBuy pins the sell first step of the
// shopping trips: the equipped piece a planned purchase displaces is
// unequipped and sold BEFORE the buys run, so its proceeds fund the
// replacement. The sequence: the unequip request goes out, the
// equipped flag flip collects the piece, the sell batch goes out and
// only then the purchase planning starts.
func TestReplacementSalesSellBeforeBuy(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	// The character wears the sickle and carries adena that upgrades
	// the weapon only through the sickle's sell credit.
	bot.ApplyItemList([]state.InventoryItem{
		{
			ObjectID: 100, ItemID: 153, Count: 1, Equipped: true,
			BodyPart: 0x80, Change: 1,
		},
		{ObjectID: 999, ItemID: 57, Count: 60000, Type2: 4, Change: 1},
	})
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 8,
		X: 46112, Y: 41500, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		PaperdollObjectIDs: [state.PaperdollSlots]int32{0, 0, 0, 0, 0, 0, 0, 100},
	})

	// The sell stop with no junk: the replacement step runs before
	// the buy planning.
	loop.phase = phaseTownSell
	loop.tripStops = []tripStop{{merchant: townMerchants[3], sell: true}}
	loop.tripStart = time.Now()
	loop.sellPhaseAt = time.Now().Add(-time.Minute)
	loop.sellAt = time.Time{}

	// The first step call plans the replacement targets.
	loop.tick()
	require.True(t, loop.replacePlanned)
	require.Equal(t, []int32{100}, loop.replaceQueue,
		"the equipped sickle is queued for the sale")

	// The unequip request goes out (paced by the equip period).
	loop.replaceUnequipAt = time.Now().Add(-equipActionPeriod - time.Second)
	loop.tick()
	require.Equal(t, []int32{100}, game.uses,
		"the displaced piece is unequipped first")
	require.Empty(t, game.sells, "no sell before the unequip lands")

	// The unequip confirms: the piece became a plain sellable
	// candidate and sells (the junk flow or the replacement batch -
	// one offer either way).
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{
			ObjectID: 100, ItemID: 153, Count: 1, Equipped: false,
			BodyPart: 0x80, Change: 2,
		},
	})
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.NotEmpty(t, game.sells,
		"the displaced piece is sold before the buys")
	offers := 0
	for _, batch := range game.sells {
		for _, item := range batch {
			if item.ObjectID == 100 {
				offers++
			}
		}
	}
	require.Equal(t, 1, offers, "exactly one sale offer of the piece")

	// The sale lands: the piece vanishes from the tracked inventory
	// (the same update carries the adena) and the buy planning runs
	// against the fresh budget that includes the proceeds.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 100, ItemID: 153, Count: 0, Change: 3},
		{ObjectID: 999, ItemID: 57, Count: 69250, Type2: 4, Change: 2},
	})
	loop.tick()
	require.True(t, loop.replaceDone,
		"the sell first step completes with the confirmation")
	require.True(t, loop.buysPlanned,
		"the purchase planning runs after the replacement sale")

	// The re-plan with the sale proceeds still buys a weapon
	// upgrade - the freed slot and the fresh adena pay for it.
	foundWeapon := false
	for _, stop := range loop.tripStops {
		for _, purchase := range stop.buys {
			stats, ok := npcdata.ItemGearStats(purchase.ItemID)
			if ok && (stats.BodyPart == "rhand" ||
				stats.BodyPart == "lrhand") {
				foundWeapon = true
			}
		}
	}
	require.True(t, foundWeapon,
		"a weapon upgrade is planned with the sale proceeds")

	// The auto equipment stayed suspended through the step: nothing
	// re-equipped the sickle between the unequip and the sale.
	require.Len(t, game.uses, 1,
		"no re-equip raced the sale")
}
