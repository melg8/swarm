// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "log"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestMerchantWaresClassifyTheTownMerchants pins the merchant wares
// knowledge against the generated catalogs: the weapon trader sells
// weapons, the armor trader the armor with the shields, the jeweler
// the jewels with the spellbooks and the grocer the consumables - the
// same split in both towns (the elven village and the Dion quarter).
func TestMerchantWaresClassifyTheTownMerchants(t *testing.T) {
    require.Equal(t, WaresWeapon, merchantWares(7147),
        "Unoren trades weapons")
    require.Equal(t, WaresArmor, merchantWares(7148),
        "Ariel trades armor with the shields")
    require.Equal(t, WaresJewel|WaresMagic, merchantWares(7149),
        "Creamees trades jewels and spellbooks")
    require.Equal(t, WaresConsumable, merchantWares(7150),
        "Herbiel trades consumables")
    require.Equal(t, WaresWeapon, merchantWares(7060),
        "Sabrin trades weapons")
    require.Equal(t, WaresArmor, merchantWares(7061),
        "Casey trades armor with the shields")
    require.Equal(t, WaresJewel|WaresMagic, merchantWares(7062),
        "Sonia trades jewels and spellbooks")
    require.Equal(t, WaresConsumable, merchantWares(7063),
        "Lara trades consumables")
}

// TestMerchantWaresStringRendersTheFamilies pins the log rendering of
// the wares set.
func TestMerchantWaresStringRendersTheFamilies(t *testing.T) {
    require.Equal(t, "nothing", WaresNone.String())
    require.Equal(t, "weapons", WaresWeapon.String())
    require.Equal(t, "jewels, spellbooks", (WaresJewel | WaresMagic).String())
}

// TestMerchantSellsItemJoinsTheGeneratedLists pins the exact item
// check the buy enforcement rides: the item must sit in one of the
// merchant's own buylists (a weapon never answers true at the armor
// trader and vice versa).
func TestMerchantSellsItemJoinsTheGeneratedLists(t *testing.T) {
    require.True(t, merchantSellsItem(7147, 1333),
        "Unoren sells the Brandish")
    require.False(t, merchantSellsItem(7148, 1333),
        "Ariel does not sell the Brandish")
    require.True(t, merchantSellsItem(7148, 21),
        "Ariel sells the Shirt")
    require.False(t, merchantSellsItem(7147, 21),
        "Unoren does not sell the Shirt")
    require.True(t, merchantSellsItem(7149, 1048),
        "Creamees sells the spellbooks")
    require.True(t, merchantSellsItem(7150, 17),
        "Herbiel sells the wooden arrows")
    require.False(t, merchantSellsItem(7063, 1333),
        "Lara does not sell weapons")
}

// TestShoppingPlanDropsForeignMerchantLines pins the freeze filter: a
// plan line pinned to a merchant that does not trade the item (a data
// bug) never rides the trip - the server would silently refuse the
// buy and the stop would burn its whole retry budget for nothing.
func TestShoppingPlanDropsForeignMerchantLines(t *testing.T) {
    loop, _, _, _ := newTripLoop()
    plan := loop.dropForeignMerchantPurchases([]gear.Purchase{
        {
            ItemID: 1333, ListID: 3014700, MerchantTemplateID: 7147,
            Count: 1, Price: 980, Reason: "the weapon milestone",
            SellFirst: nil, SellCredit: 0, Gain: 0,
            Affordable: true, Missing: 0,
        },
        {
            ItemID: 1333, ListID: 3014800, MerchantTemplateID: 7148,
            Count: 1, Price: 980, Reason: "the weapon milestone",
            SellFirst: nil, SellCredit: 0, Gain: 0,
            Affordable: true, Missing: 0,
        },
    })
    require.Len(t, plan, 1, "the armor trader line of the weapon drops")
    require.Equal(t, int32(7147), plan[0].MerchantTemplateID)
    require.Equal(t, int32(1333), plan[0].ItemID)
}

// TestBuyStopReSelectsTheStopMerchant pins the seller distinction at
// the transaction level: the sell phase picks the nearest vendor of
// the town (every merchant accepts the sale of any sellable item) and
// the buy stop of the weapon purchase must re-select the WEAPON
// merchant - the server resolves the buy through the targeted folk
// npc and silently refuses the list the targeted trader does not
// carry, so a weapon buy aimed at the armor trader burns the retry
// budget and the trip leaves without the weapon.
func TestBuyStopReSelectsTheStopMerchant(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    logBuf := &bytes.Buffer{}
    loop.SetLogger(log.New(logBuf, "", 0))
    unoren := townMerchants[0]
    ariel := townMerchants[1]
    weapon := gear.Purchase{
        ItemID: 1333, ListID: 3014700, MerchantTemplateID: 7147,
        Count: 1, Price: 980, Reason: "the weapon milestone",
        SellFirst: nil, SellCredit: 0, Gain: 0,
        Affordable: true, Missing: 0,
    }
    loop.phase = phaseTownSell
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{{merchant: unoren, sell: true}}
    loop.tripPlan = []gear.Purchase{weapon}
    loop.sellPhaseAt = time.Now().Add(-time.Minute)
    // The character stands at the armor trader's counter: the junk
    // sale picks the nearest vendor (Ariel), the weapon buy must not.
    moveSelfTo(bot, ariel.X, ariel.Y, ariel.Z)
    junk := make([]state.InventoryItem, 0, 20)
    for i := range 20 {
        junk = append(junk, state.InventoryItem{
            ObjectID: 500 + int32(i), ItemID: 1060, Count: 1,
            Type2: 5, Change: 1,
        })
    }
    bot.ApplyItemList(junk)
    spawnMerchantNPC(bot, ariel, 61)
    spawnMerchantNPC(bot, unoren, 60)

    // The sell phase picks the nearest vendor and sells the junk.
    loop.merchantPick = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, int32(61), loop.merchantID,
        "the junk sells at the nearest vendor Ariel")
    bot.ApplySelfTarget(61)
    loop.sellAt = time.Time{}
    loop.tick()
    require.Len(t, game.sells, 1, "the junk batch sells to Ariel")
    require.Len(t, game.sells[0], 20)

    // The sale lands: the junk leaves the inventory, the frozen plan
    // distributes (the weapon group merges into the current stop) and
    // the buy phase re-picks the stop merchant (the stale selection
    // of the sell phase does not aim the weapon buy at the armor
    // trader).
    updates := make([]state.InventoryItem, 0, len(game.sells[0]))
    for _, item := range game.sells[0] {
        updates = append(updates, state.InventoryItem{
            ObjectID: item.ObjectID, ItemID: item.ItemID,
            Count: item.Count, Type2: 5, Change: 3,
        })
    }
    bot.ApplyInventoryUpdate(updates)
    loop.merchantPick = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, int32(60), loop.merchantID,
        "the buy stop re-selects the weapon merchant Unoren")
    require.Contains(t, logBuf.String(),
        "does not sell this stop's goods",
        "the stale selection names itself in the log")

    // The re-selection clicks Unoren (the target still names Ariel).
    loop.merchantPick = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Contains(t, game.forces, int32(60),
        "the re-selection clicks the weapon merchant")
    bot.ApplySelfTarget(60)
    loop.sellAt = time.Now().Add(-buyPause - time.Second)
    loop.buyAt = time.Now().Add(-buyPause - time.Second)
    loop.merchantPick = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.NotEmpty(t, game.buys, "the weapon list buys at Unoren")
    require.Equal(t, int32(3014700), game.buys[0][0].ListID)
    require.Equal(t, int32(7147), game.buys[0][0].MerchantTemplateID)
}

// TestMerchantSetsFollowTheRegion pins the region aware merchant
// sets: the elven default targets the elven traders and the Dion
// region the Dion traders (the same set the region shop catalog
// builds from), so the trip start and the sell pick never walk a
// foreign town's merchants.
func TestMerchantSetsFollowTheRegion(t *testing.T) {
    loop, _, _, _ := newTripLoop()
    merchant, ok := loop.nearestMerchant(43000, 50000)
    require.True(t, ok)
    require.Equal(t, int32(7150), merchant.TemplateID,
        "the elven default picks the nearest elven trader")
    templates := loop.merchantTemplates()
    require.Contains(t, templates, int32(7147)+npcDisplayOffset)
    require.NotContains(t, templates, int32(7060)+npcDisplayOffset)

    loop.zoneRegion = regionDion
    merchant, ok = loop.nearestMerchant(18000, 144500)
    require.True(t, ok)
    require.Equal(t, int32(7060), merchant.TemplateID,
        "the Dion region picks the nearest Dion trader")
    templates = loop.merchantTemplates()
    require.Contains(t, templates, int32(7060)+npcDisplayOffset)
    require.NotContains(t, templates, int32(7147)+npcDisplayOffset)
}
