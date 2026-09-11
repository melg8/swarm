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
// stop of the plan (Ariel across the village, the armor floor pieces
// of the strategy opening) buys its list.
func TestShoppingTripBuysAfterSelling(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)
	// The adena of the character: 100 adena buys the armor floor of
	// the cheapest pieces (the apprentice's shoes and the short
	// gloves of the armor trader Ariel) - the jewels wait for a
	// worn real weapon, so the jewel floor stays closed.
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
	// The plan of 100 adena buys the armor floor of the armor
	// trader Ariel: the sell stop is done, the trip advances to the
	// single buy stop.
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
			Reason: "", SellFirst: nil, SellCredit: 0, Gain: 0,
			Affordable: true, Missing: 0,
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
	// The character stands at Ariel with enough adena for the
	// pdef maximizing armor set of the strategy opening (the shoes
	// plus the shirt: 44 pdef for 177 adena).
	ariel := townMerchants[1]
	moveSelfTo(bot, ariel.X, ariel.Y, ariel.Z)
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 200, Type2: 4, Change: 1},
	})
	loop.tripStops = []tripStop{{merchant: ariel, sell: true}}
	// The trip plan freezes at the trip start (maybeStartTownTrip);
	// the stop planning only distributes it.
	loop.tripPlan = loop.shoppingPlan()

	loop.planShoppingStops()
	require.True(t, loop.buysPlanned)
	require.Len(t, loop.tripStops, 1,
		"the armor set needs no second merchant")
	require.Len(t, loop.tripStops[0].buys, 2,
		"the armor purchases merge into the current stop")
	for _, purchase := range loop.tripStops[0].buys {
		require.Equal(t, int32(7148), purchase.MerchantTemplateID)
		require.Equal(t, int32(3014800), purchase.ListID)
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
	// The trip plan freezes at the trip start (maybeStartTownTrip):
	// the weapon purchase of the frozen plan carries the sickle as
	// its SellFirst piece.
	loop.tripPlan = loop.shoppingPlan()

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

	// The frozen plan's purchases still fill the stops after the
	// sale proceeds landed: the freed slot and the counted credit
	// pay for the weapon upgrade they were planned against.
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

// TestPublishShoppingViewQueue pins the widget view of the hunting
// loop: the tick publishes the purchase queue with the trip flag off,
// the affordable entries carry no missing amount, the wanted tail
// entries do, and the entries resolve their names and merchants.
func TestPublishShoppingViewQueue(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	// A bare character with 50 adena: the affordable plan buys the
	// cheap fillers (under the 100 adena trip minimum, so no town trip
	// starts) and the wanted tail shows the saves beyond the wallet.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 50, Type2: 4, Change: 1},
	})

	loop.tick()

	plan := bot.Snapshot().Shopping
	require.NotNil(t, plan, "the queue view must publish on the tick")
	require.False(t, plan.Trip)
	require.Equal(t, int64(50), plan.Adena,
		"the view carries the planning wallet")
	require.NotEmpty(t, plan.Entries)
	affordable := 0
	wanted := 0
	for _, entry := range plan.Entries {
		require.NotEmpty(t, entry.Name, "the entry name resolves")
		require.NotEmpty(t, entry.Merchant, "the merchant name resolves")
		require.NotEmpty(t, entry.Icon, "the entry icon resolves")
		require.Positive(t, entry.Price)
		if entry.Affordable {
			affordable++
			require.Zero(t, entry.Missing,
				"an affordable entry misses no adena")
		} else {
			wanted++
			require.Positive(t, entry.Missing,
				"a wanted entry carries its missing adena")
		}
	}
	require.Positive(t, affordable, "the wallet affords the fillers")
	require.Positive(t, wanted, "the wanted tail follows the plan")
	require.Equal(t, affordableTotal(plan.Entries), plan.Total,
		"the total sums the affordable entries")
}

// TestPublishShoppingViewManualSessionClears pins the manual only
// sessions: no autonomous trips are planned, so the widget view stays
// unpublished.
func TestPublishShoppingViewManualSessionClears(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 50, Type2: 4, Change: 1},
	})
	loop.SetAutonomy(false)

	loop.tick()

	require.Nil(t, bot.Snapshot().Shopping,
		"a manual session never plans a trip")
}

// TestPublishShoppingViewTripView pins the trip view: while a town
// trip runs, the widget shows its remaining buys - the in-flight
// batch marked buying first, then the pending purchases of the stops.
func TestPublishShoppingViewTripView(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 5000, Type2: 4, Change: 1},
	})
	loop.tripStart = time.Now()
	loop.phase = phaseTownSell
	loop.tripStops = []tripStop{
		{merchant: townMerchants[1]},
		{merchant: townMerchants[0]},
	}
	loop.buyRequested = []gear.Purchase{{
		ItemID: 1121, ListID: 3014800, MerchantTemplateID: 7148,
		Count: 1, Price: 9, Reason: "buying Apprentice's Shoes",
		Affordable: true,
	}}
	loop.tripStops[0].buys = []gear.Purchase{{
		ItemID: 1146, ListID: 3014800, MerchantTemplateID: 7148,
		Count: 1, Price: 20, Reason: "buying Cloth Cap",
		Affordable: true,
	}}
	loop.tripStops[1].buys = []gear.Purchase{{
		ItemID: 1, ListID: 3014700, MerchantTemplateID: 7147,
		Count: 1, Price: 883, Reason: "buying Short Sword",
		Affordable: true,
	}}

	publishShoppingViewForTest(loop)

	plan := bot.Snapshot().Shopping
	require.NotNil(t, plan)
	require.True(t, plan.Trip, "the running trip marks the view")
	require.Len(t, plan.Entries, 3)
	require.True(t, plan.Entries[0].Buying,
		"the in-flight batch entry is marked buying")
	require.False(t, plan.Entries[1].Buying)
	require.False(t, plan.Entries[2].Buying)
	require.Equal(t, int32(1121), plan.Entries[0].ItemID)
	require.Equal(t, int32(1146), plan.Entries[1].ItemID)
	require.Equal(t, int32(1), plan.Entries[2].ItemID)
	require.Equal(t, int64(912), plan.Total,
		"the total sums the remaining trip buys")
	require.Equal(t, int64(5000), plan.Adena)
}

// publishShoppingViewForTest drives the publisher with the tick defer
// semantics: the trip branch needs the phase bookkeeping of a tick
// without running the trip state machine itself.
func publishShoppingViewForTest(loop *Loop) {
	loop.publishShoppingView()
}

// affordableTotal sums the prices of the affordable entries.
func affordableTotal(entries []state.ShoppingEntryView) int64 {
	var total int64
	for _, entry := range entries {
		if entry.Affordable {
			total += entry.Price
		}
	}

	return total
}

// TestTripShoppingViewHoldsThePlanDuringWalk pins the widget view of
// the town walk: the trip stops are planned only at the shop (after
// the junk selling), so the walk to town carried an EMPTY trip view
// and the widget went blank for the whole leg - the reported "the
// shopping list is missing after the bot switch". The plan that
// triggered the trip (the cached hunt queue) stays on the widget
// until the stop planning replaces it.
func TestTripShoppingViewHoldsThePlanDuringWalk(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	fillInventory(bot)
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 100, Type2: 4, Change: 1},
	})

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase,
		"the full inventory starts the town trip")
	require.Empty(t, loop.tripStops[0].buys,
		"the trip stops are planned only at the shop")

	snap := bot.Snapshot()
	require.NotNil(t, snap.Shopping,
		"the triggering plan stays on the widget during the walk")
	require.NotEmpty(t, snap.Shopping.Entries,
		"the queue view carries the planned purchases")
	require.False(t, snap.Shopping.Trip,
		"the walk view is the hunt queue, not a trip batch")
}

// test1DumpPaperdoll is the UserInfo paperdoll block of the
// 2026-09-11 10:30 test1 dump at its trip start: the Short Sword, the
// Gloves and the Apprentice's Shoes worn among the rest of the outfit
// (the same object ids the dump carries).
var test1DumpPaperdoll = [state.PaperdollSlots]int32{
	0,         // underwear
	268457163, // right ear: Earring of Strength
	268457164, // left ear: Earring of Wisdom
	268450945, // neck: Necklace of Anguish
	268457165, // right finger: Ring of Anguish
	268457188, // left finger: Ring of Anguish
	268451114, // head: Leather Helmet
	268451661, // right hand: Short Sword
	0,         // left hand
	268457274, // gloves: Gloves
	268451006, // chest: Wooden Breastplate
	268451057, // legs: Hard Leather Pants
	268451634, // feet: Apprentice's Shoes
	0,         // back
	268451661, // the C1 duplicate of the right hand
}

// test1DumpInventory is the inventory behind the paperdoll plus the
// 71420 adena the reconstruction of the dump derives (71420 + 387 of
// the sale proceeds - 70007 of the buys = the 1800 the dump ends with).
func test1DumpInventory() []state.InventoryItem {
	return []state.InventoryItem{
		{ObjectID: 268451661, ItemID: 1, Count: 1, Type2: 0, Equipped: true, Change: 1},
		{ObjectID: 268457274, ItemID: 49, Count: 1, Type2: 1, Equipped: true, Change: 1},
		{ObjectID: 268451634, ItemID: 1121, Count: 1, Type2: 1, Equipped: true, Change: 1},
		{ObjectID: 268451006, ItemID: 23, Count: 1, Type2: 1, Equipped: true, Change: 1},
		{ObjectID: 268451057, ItemID: 30, Count: 1, Type2: 1, Equipped: true, Change: 1},
		{ObjectID: 268451114, ItemID: 44, Count: 1, Type2: 1, Equipped: true, Change: 1},
		{ObjectID: 268457163, ItemID: 114, Count: 1, Type2: 2, Equipped: true, Change: 1},
		{ObjectID: 268457164, ItemID: 115, Count: 1, Type2: 2, Equipped: true, Change: 1},
		{ObjectID: 268457165, ItemID: 876, Count: 1, Type2: 2, Equipped: true, Change: 1},
		{ObjectID: 268457188, ItemID: 876, Count: 1, Type2: 2, Equipped: true, Change: 1},
		{ObjectID: 268450945, ItemID: 907, Count: 1, Type2: 2, Equipped: true, Change: 1},
		{ObjectID: 999, ItemID: 57, Count: 71420, Type2: 4, Change: 1},
	}
}

// TestTripPlanFreezesPurchasesAgainstResale replays the 2026-09-11
// 10:30 test1 trip on the frozen plan machinery: the trip plans
// [Brandish + Low Boots] once at its start (worth 69999 adena, the
// dump's own number), sells the displaced Short Sword and Apprentice's
// Shoes - and the stop planning distributes EXACTLY the frozen plan.
// The shop re-plan this regression replaces produced a different
// list against the freed slots and the fresh adena: it re-bought the
// Apprentice's Shoes the trip had just sold (the armor floor pulled
// the cheapest piece into the emptied slot) and planned the Leather
// Gloves whose displaced Gloves were never queued for the sale - the
// bot walked home holding two pairs of gloves.
func TestTripPlanFreezesPurchasesAgainstResale(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	bot.ApplyItemList(test1DumpInventory())
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 14,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		PaperdollObjectIDs: test1DumpPaperdoll,
	})

	// The trip arms on the shopping trigger and freezes its plan.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Len(t, loop.tripPlan, 2, "the frozen plan holds both purchases")
	require.Equal(t, int64(69999), gear.AdenaSpent(loop.tripPlan),
		"the frozen plan matches the dump's worth")
	require.Equal(t, int32(1333), loop.tripPlan[0].ItemID,
		"the weapon milestone of the frozen plan is the Brandish")
	require.Equal(t, []int32{268451661}, loop.tripPlan[0].SellFirst)
	require.Equal(t, int32(38), loop.tripPlan[1].ItemID,
		"the feet upgrade of the frozen plan is the Low Boots")
	require.Equal(t, []int32{268451634}, loop.tripPlan[1].SellFirst)

	// The sell stop routes to the weapon merchant of the frozen plan.
	unoren := townMerchants[0]
	moveSelfTo(bot, unoren.X, unoren.Y, unoren.Z)
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)

	// The replacement queue of the frozen plan: the sword and the
	// shoes - the gloves stay worn (their upgrade is not in the plan).
	for range 3 {
		loop.tick()
		if loop.replacePlanned {
			break
		}
	}
	require.Equal(t, []int32{268451661, 268451634}, loop.replaceQueue)

	// Both replaced pieces come off (paced) and their unequips confirm:
	// the server answers with the UserInfo paperdoll block (the slot
	// empties) plus the inventory update (the equipped flag flips).
	paperdoll := test1DumpPaperdoll
	for _, piece := range []struct {
		objectID int32
		itemID   int32
		type2    int16
		slot     int
	}{
		{268451661, 1, 0, state.PaperdollRHand},
		{268451634, 1121, 1, state.PaperdollFeet},
	} {
		loop.replaceUnequipAt = time.Now().Add(-equipActionPeriod - time.Second)
		loop.tick()
		paperdoll[piece.slot] = 0
		bot.ApplyPaperdoll(paperdoll)
		bot.ApplyInventoryUpdate([]state.InventoryItem{
			{
				ObjectID: piece.objectID, ItemID: piece.itemID,
				Count: 1, Type2: piece.type2, Equipped: false, Change: 2,
			},
		})
	}
	require.Equal(t, []int32{268451661, 268451634}, game.uses,
		"both displaced pieces were unequipped in the plan order")

	// The offer batch sells both pieces; the removals settle the step.
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.Len(t, game.sells, 1, "both replaced pieces go out as one batch")
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 268451661, ItemID: 1, Count: 0, Change: 3},
		{ObjectID: 268451634, ItemID: 1121, Count: 0, Change: 3},
		{ObjectID: 999, ItemID: 57, Count: 71807, Type2: 4, Change: 2},
	})
	loop.tick()
	require.True(t, loop.buysPlanned,
		"the frozen plan distributes after the sales settled")

	// THE REGRESSION: the stops carry exactly the frozen plan - the
	// Brandish at its merchant and the Low Boots at its own - never the
	// re-bought Apprentice's Shoes and never a purchase whose displaced
	// piece was not queued for the sale.
	require.Len(t, loop.tripStops, 2)
	require.Equal(t, int32(7147), loop.tripStops[0].merchant.TemplateID)
	require.Len(t, loop.tripStops[0].buys, 1)
	require.Equal(t, int32(1333), loop.tripStops[0].buys[0].ItemID)
	require.Equal(t, int32(7148), loop.tripStops[1].merchant.TemplateID)
	require.Len(t, loop.tripStops[1].buys, 1)
	require.Equal(t, int32(38), loop.tripStops[1].buys[0].ItemID)
	for _, stop := range loop.tripStops {
		for _, purchase := range stop.buys {
			require.NotEqual(t, int32(1121), purchase.ItemID,
				"the sold Apprentice's Shoes are never bought back")
			require.NotEqual(t, int32(50), purchase.ItemID,
				"no purchase appears without its queued sale")
		}
	}

	// The buy of the merged stop executes against the frozen plan: the
	// manager knows what it buys and waits for the confirmation.
	settleMerchant(t, loop, game, bot, unoren, 55)
	loop.buyAt = time.Now().Add(-buyPause - time.Second)
	loop.sellAt = time.Now().Add(-buyPause - time.Second)
	loop.merchantPick = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Len(t, game.buys, 1, "the frozen plan's weapon buys")
	require.Len(t, game.buys[0], 1)
	require.Equal(t, int32(1333), game.buys[0][0].ItemID)
	require.Equal(t, int32(3014700), game.buys[0][0].ListID)
}

// TestStopShoppingSkipsOwnedItems pins the last responsible moment of
// the frozen plan: an item the inventory already carries (a loot drop
// the auto equipment wore mid trip, a manual user purchase) drops out
// of the stop before any request goes out - the buy would deliver a
// duplicate the plan never wanted (the second pair of gloves).
func TestStopShoppingSkipsOwnedItems(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	// The bot already holds the Leather Gloves the stop is about to buy.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 800, ItemID: 50, Count: 1, Type2: 1, Change: 1},
	})
	loop.tripStops = []tripStop{{
		merchant: townMerchants[1],
		buys: []gear.Purchase{{
			ItemID: 50, ListID: 3014800, MerchantTemplateID: 7148,
			Count: 1, Price: 7785, Reason: "buying Leather Gloves",
			Affordable: true,
		}},
	}}
	loop.merchantID = -1
	loop.buyAt = time.Now().Add(-buyPause - time.Second)

	done := loop.tickStopShopping(time.Now())
	require.True(t, done, "the owned line drops out and the stop completes")
	require.Empty(t, game.buys, "the owned item is never bought again")
	require.Empty(t, loop.tripStops[0].buys, "the owned line left the stop")
}
