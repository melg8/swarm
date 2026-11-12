// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// vendorVillageSword is the level 15 village weapon the errand
// upgrades: the armed gate reads a bagged or worn weapon.
const vendorVillageSword = int32(2)

// cityShopAllStops returns the kit stops plus the bow stop: the
// static stock facts of the errand, independent of a bot's plan.
func cityShopAllStops() []cityShopStop {
    stops := make([]cityShopStop, 0, len(cityShoppingStops)+1)
    stops = append(stops, cityShoppingStops...)
    stops = append(stops, cityShopBowStop)

    return stops
}

// TestCityShopStockMatchesTheNamedLists pins the live stock facts of
// the errand: every purchase rides the buylist of its merchant, so
// the buy request cannot drift from the generated shop data.
func TestCityShopStockMatchesTheNamedLists(t *testing.T) {
    for _, stop := range cityShopAllStops() {
        lists := npcdata.BuyListsOfNPC(stop.merchant.TemplateID)
        require.Contains(t, lists, stop.listID,
            "%s must sell list %d", stop.merchant.Name, stop.listID)
        sold := npcdata.ItemsOfBuyList(stop.listID)
        for _, itemID := range stop.itemIDs {
            require.Contains(t, sold, itemID,
                "list %d of %s must carry %s", stop.listID,
                stop.merchant.Name, npcdata.ItemName(itemID))
        }
    }
}

// TestCityShoppingKitOrder pins the purchase order the acceptance
// scenario renders: the weapon first, then the armor set.
func TestCityShoppingKitOrder(t *testing.T) {
    require.Equal(t, []int32{
        cityShopWeaponID,
        cityShopChestID,
        cityShopLegsID,
        cityShopHeadID,
        cityShopGlovesID,
        cityShopFeetID,
        cityShopShieldID,
    }, CityShoppingKit())
}

// TestCityShopPrice pins the Gludio counter prices of the plan: the
// reference price plus the 20 percent castle tax. The full kit costs
// 455 760 adena and the bow another 292 800, the 748 560 adena budget
// of the whole errand plan.
func TestCityShopPrice(t *testing.T) {
    cases := []struct {
        itemID int32
        want   int64
    }{
        {cityShopWeaponID, 292800},
        {cityShopChestID, 59040},
        {cityShopLegsID, 36840},
        {cityShopHeadID, 22080},
        {cityShopGlovesID, 14760},
        {cityShopFeetID, 14760},
        {cityShopShieldID, 15480},
        {cityShopBowID, 292800},
    }
    total := int64(0)
    for _, testCase := range cases {
        require.Equal(t, testCase.want, cityShopPrice(testCase.itemID),
            "the price of %s", npcdata.ItemName(testCase.itemID))
        total += testCase.want
    }
    require.Equal(t, int64(748560), total,
        "the kit plus the bow must stay under the 800 000 adena wallet")
}

// TestCityShopBuysShape pins the frozen purchase lines of one stop:
// one line per item, the whole stop riding its own buylist, one
// unit per line at the Gludio price.
func TestCityShopBuysShape(t *testing.T) {
    loop, _, _, _ := newTripLoop()
    for _, stop := range cityShopAllStops() {
        buys := loop.cityShopBuys(stop)
        require.Len(t, buys, len(stop.itemIDs))
        for index, purchase := range buys {
            require.Equal(t, stop.itemIDs[index], purchase.ItemID)
            require.Equal(t, stop.listID, purchase.ListID)
            require.Equal(t, stop.merchant.TemplateID,
                purchase.MerchantTemplateID)
            require.Equal(t, int32(1), purchase.Count)
            require.Equal(t, cityShopPrice(purchase.ItemID),
                purchase.Price)
            require.True(t, purchase.Affordable)
        }
    }
}

// TestCityShopBuysCarryTheDisplacedKit pins the sell first wiring of
// the errand: every kit purchase names the worn village piece of its
// slot and its sell credit, so the stop sells the old dress before
// the replacement lands.
func TestCityShopBuysCarryTheDisplacedKit(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 100, ItemID: 2, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 101, ItemID: 24, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 102, ItemID: 31, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 103, ItemID: 44, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 104, ItemID: 50, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 105, ItemID: 38, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 106, ItemID: 102, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 107, ItemID: 272, Count: 1},
    })
    paperdoll := [state.PaperdollSlots]int32{}
    for _, piece := range []struct {
        objectID int32
        slot     int
    }{
        {100, state.PaperdollRHand},
        {101, state.PaperdollChest},
        {102, state.PaperdollLegs},
        {103, state.PaperdollHead},
        {104, state.PaperdollGloves},
        {105, state.PaperdollFeet},
        {106, state.PaperdollLHand},
    } {
        paperdoll[piece.slot] = piece.objectID
    }
    bot.ApplyPaperdoll(paperdoll)

    weaponBuys := loop.cityShopBuys(cityShoppingStops[0])
    require.Len(t, weaponBuys, 1)
    require.Equal(t, []int32{100}, weaponBuys[0].SellFirst,
        "the worn village sword is displaced by the Falchion")
    require.Equal(t, npcdata.ItemPrice(2)/2, weaponBuys[0].SellCredit)

    armorBuys := loop.cityShopBuys(cityShoppingStops[1])
    require.Len(t, armorBuys, len(cityShoppingStops[1].itemIDs))
    displaced := make([][]int32, 0, len(armorBuys))
    for _, purchase := range armorBuys {
        require.Len(t, purchase.SellFirst, 1,
            "the purchase of %s displaces one village piece",
            npcdata.ItemName(purchase.ItemID))
        require.Positive(t, purchase.SellCredit)
        displaced = append(displaced, purchase.SellFirst)
    }
    require.Equal(t,
        [][]int32{{101}, {102}, {103}, {104}, {105}, {106}},
        displaced, "every armor slot sells its own village piece")

    bowBuys := loop.cityShopBuys(cityShopBowStop)
    require.Len(t, bowBuys, 1)
    require.Equal(t, []int32{107}, bowBuys[0].SellFirst,
        "the weaker Forest Bow is replaced by the Composition Bow")
    require.Equal(t, npcdata.ItemPrice(272)/2, bowBuys[0].SellCredit,
        "the old bow feeds its sell value into the stop")
}

// TestCityShopTradeSellsBeforeTheBuy pins the stop wiring: the worn
// weapon is unequipped and sold, and only the confirmed sale lets the
// replacement buy request out.
func TestCityShopTradeSellsBeforeTheBuy(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 100, ItemID: 2, Count: 1, Equipped: true, Change: 1},
        {ObjectID: 999, ItemID: 57, Count: cityShoppingWallet, Type2: 4},
    })
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 100
    bot.ApplyPaperdoll(paperdoll)
    stop := cityShoppingStops[0]
    arriveAtMerchantStand(bot, stop.merchant)
    spawnMerchantNPC(bot, stop.merchant, 61)

    buys := loop.cityShopBuys(stop)
    require.Equal(t, []int32{100}, buys[0].SellFirst)
    loop.tripPlan = buys
    loop.sold = make(map[int32]bool)
    loop.resetReplacementSales()
    loop.tripStops = []tripStop{{
        merchant: stop.merchant,
        buys:     buys,
        sell:     false,
        teach:    false,
        guide:    false,
    }}
    loop.sellPhaseAt = time.Now()

    now := time.Now()
    require.False(t, loop.cityShopTrade(now))
    require.Equal(t, []int32{100}, loop.replaceQueue,
        "the worn village sword is queued for the sale")
    require.Empty(t, game.buys, "no buy while the piece sells")

    require.False(t, loop.cityShopTrade(now))
    require.Equal(t, []int32{100}, game.uses,
        "the displaced sword is unequipped first")
    require.Empty(t, game.sells)
    require.Empty(t, game.buys)

    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 100, ItemID: 2, Count: 1, Equipped: false, Change: 2,
    }})
    loop.sellAt = time.Time{}
    require.False(t, loop.cityShopTrade(now))
    require.NotEmpty(t, game.sells, "the displaced sword sells first")
    require.Empty(t, game.buys, "the buy waits for the confirmed sale")

    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 100, ItemID: 2, Count: 0, Change: 3,
    }})
    loop.sellAt = now.Add(-transactionPause - time.Second)
    loop.merchantID = 0
    loop.merchantPick = now.Add(-2 * time.Second)
    require.False(t, loop.cityShopTrade(now))
    require.Equal(t, int32(61), loop.merchantID,
        "the buy picks the weapon merchant")

    loop.merchantPick = now.Add(-2 * time.Second)
    require.False(t, loop.cityShopTrade(now))
    require.Contains(t, game.forces, int32(61),
        "the buy selects the weapon merchant")
    bot.ApplySelfTarget(61)
    loop.merchantPick = now.Add(-2 * time.Second)
    require.False(t, loop.cityShopTrade(now))
    require.NotEmpty(t, game.buys, "the replacement buy follows the sale")
    require.Equal(t, cityShopWeaponID, game.buys[0][0].ItemID)
}

// TestCityShopBowStopSellsTheOldBow pins the owner report: the bagged
// Forest Bow is the lurer tool the Composition Bow replaces, so the
// bow stop sells it before the buy request goes out.
func TestCityShopBowStopSellsTheOldBow(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 200, ItemID: 272, Count: 1, Change: 1},
        {ObjectID: 999, ItemID: 57, Count: cityShoppingWallet, Type2: 4},
    })
    stop := cityShopBowStop
    arriveAtMerchantStand(bot, stop.merchant)
    spawnMerchantNPC(bot, stop.merchant, 61)

    buys := loop.cityShopBuys(stop)
    require.Len(t, buys, 1)
    require.Equal(t, []int32{200}, buys[0].SellFirst,
        "the only owned bow is the replaced tool")
    loop.tripPlan = buys
    loop.sold = make(map[int32]bool)
    loop.resetReplacementSales()
    loop.tripStops = []tripStop{{
        merchant: stop.merchant,
        buys:     buys,
        sell:     false,
        teach:    false,
        guide:    false,
    }}
    loop.sellPhaseAt = time.Now()

    now := time.Now()
    require.False(t, loop.cityShopTrade(now))
    require.Equal(t, []int32{200}, loop.replaceQueue,
        "the bagged bow is queued for the sale")
    require.Empty(t, game.uses,
        "the bagged bow comes off no paperdoll slot")
    require.Empty(t, game.sells)
    require.Empty(t, game.buys)

    require.False(t, loop.cityShopTrade(now))
    require.Len(t, game.sells, 1, "the old bow sells first")
    require.Equal(t, int32(272), game.sells[0][0].ItemID)
    require.Empty(t, game.buys, "the buy waits for the confirmed sale")

    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 200, ItemID: 272, Count: 0, Change: 3,
    }})
    loop.sellAt = now.Add(-transactionPause - time.Second)
    loop.merchantID = 0
    loop.merchantPick = now.Add(-2 * time.Second)
    require.False(t, loop.cityShopTrade(now))
    require.Equal(t, int32(61), loop.merchantID,
        "the buy picks the weapon merchant")

    loop.merchantPick = now.Add(-2 * time.Second)
    require.False(t, loop.cityShopTrade(now))
    require.Contains(t, game.forces, int32(61),
        "the buy selects the weapon merchant")
    bot.ApplySelfTarget(61)
    loop.merchantPick = now.Add(-2 * time.Second)
    require.False(t, loop.cityShopTrade(now))
    require.NotEmpty(t, game.buys, "the bow buy follows the sale")
    require.Equal(t, cityShopBowID, game.buys[0][0].ItemID)
}

// cityShoppingTestLoop builds a loop on the default elven ground with
// a navigator, the melee profile and the given wallet and bag.
func cityShoppingTestLoop(
    adena int32, bag []state.InventoryItem,
) *Loop {
    bot := newTestBot()
    items := make([]state.InventoryItem, 0, len(bag)+1)
    items = append(items, state.InventoryItem{
        ObjectID: 1, ItemID: 57, Count: adena, Type2: 4,
    })
    items = append(items, bag...)
    bot.ApplyItemList(items)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(&fakeNavigator{})

    return loop
}

// TestCityShoppingWantedGate pins the start decision matrix of the
// errand: the navigator, the region, the gear profile, the armed
// weapon, the missing kit, the wallet, the cooldown and the running
// trip all gate the start.
func TestCityShoppingWantedGate(t *testing.T) {
    villageSword := state.InventoryItem{
        ObjectID: 2, ItemID: vendorVillageSword, Count: 1,
    }
    ownedKit := make([]state.InventoryItem, 0, 7)
    for index, itemID := range CityShoppingKit() {
        ownedKit = append(ownedKit, state.InventoryItem{
            ObjectID: int32(100 + index), ItemID: itemID, Count: 1,
        })
    }
    kitCost := int64(0)
    for _, itemID := range CityShoppingKit() {
        kitCost += cityShopPrice(itemID)
    }
    bowCost := cityShopPrice(cityShopBowID)
    withKit := func(extra ...state.InventoryItem) []state.InventoryItem {
        bag := make([]state.InventoryItem, 0, 1+len(ownedKit)+len(extra))
        bag = append(bag, villageSword)
        bag = append(bag, ownedKit...)
        bag = append(bag, extra...)

        return bag
    }
    compositionBow := state.InventoryItem{
        ObjectID: 200, ItemID: cityShopBowID, Count: 1,
    }
    strengthenedBow := state.InventoryItem{
        ObjectID: 201, ItemID: 274, Count: 1,
    }
    cases := []struct {
        name  string
        adena int32
        bag   []state.InventoryItem
        setup func(*Loop)
        want  bool
    }{
        {
            name:  "the dressed wallet runs the errand",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            want: true,
        },
        {
            name:  "the elven ground runs the errand",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) { loop.zoneRegion = regionElven },
            want:  true,
        },
        {
            name:  "the dion ground keeps the errand off",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) { loop.zoneRegion = regionDion },
            want:  false,
        },
        {
            name:  "no navigator holds the start",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) { loop.SetNavigator(nil) },
            want:  false,
        },
        {
            name:  "a bare handed character keeps the weapon run",
            adena: cityShoppingWallet, bag: nil, want: false,
        },
        {
            name:  "the wallet one adena short holds the start",
            adena: int32(kitCost - 1),
            bag:   []state.InventoryItem{villageSword}, want: false,
        },
        {
            name:  "the exact kit price starts the errand",
            adena: int32(kitCost),
            bag:   []state.InventoryItem{villageSword}, want: true,
        },
        {
            name:  "one adena short of the bow still runs the kit",
            adena: int32(kitCost + bowCost - 1),
            bag:   []state.InventoryItem{villageSword}, want: true,
        },
        {
            name:  "the kit plus bow wallet runs the whole plan",
            adena: int32(kitCost + bowCost),
            bag:   []state.InventoryItem{villageSword}, want: true,
        },
        {
            name:  "the owned kit ends the errand",
            adena: 0, bag: withKit(), want: false,
        },
        {
            name:  "the owned kit with the bow wallet runs the bow errand",
            adena: int32(bowCost), bag: withKit(), want: true,
        },
        {
            name:  "the owned kit one adena short of the bow holds",
            adena: int32(bowCost - 1), bag: withKit(), want: false,
        },
        {
            name:  "the owned kit and bow ends the errand",
            adena: cityShoppingWallet, bag: withKit(compositionBow),
            want: false,
        },
        {
            name:  "a stronger owned bow keeps the errand off",
            adena: cityShoppingWallet, bag: withKit(strengthenedBow),
            want: false,
        },
        {
            name:  "the retry cooldown holds the start",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) {
                loop.cityShoppingTriedAt = time.Now()
            },
            want: false,
        },
        {
            name:  "the expired cooldown releases the start",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) {
                loop.cityShoppingTriedAt = time.Now().Add(
                    -cityShopCooldown - time.Second)
            },
            want: true,
        },
        {
            name:  "a running town trip owns the session",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) { loop.phase = phaseTownWalk },
            want:  false,
        },
    }
    for _, testCase := range cases {
        t.Run(testCase.name, func(t *testing.T) {
            loop := cityShoppingTestLoop(testCase.adena, testCase.bag)
            if testCase.setup != nil {
                testCase.setup(loop)
            }
            require.Equal(t, testCase.want,
                loop.cityShoppingWanted())
        })
    }
}

// TestCityShoppingPlanCost pins the plan resolution: the kit cost
// always drives the errand, and the bow joins the cost only when the
// wallet covers the whole plan beside the owned tool.
func TestCityShoppingPlanCost(t *testing.T) {
    kitCost := int64(0)
    for _, itemID := range CityShoppingKit() {
        kitCost += cityShopPrice(itemID)
    }
    bowCost := cityShopPrice(cityShopBowID)
    villageSword := state.InventoryItem{
        ObjectID: 2, ItemID: vendorVillageSword, Count: 1,
    }
    ownedKit := make([]state.InventoryItem, 0, 8)
    ownedKit = append(ownedKit, villageSword)
    for index, itemID := range CityShoppingKit() {
        ownedKit = append(ownedKit, state.InventoryItem{
            ObjectID: int32(100 + index), ItemID: itemID, Count: 1,
        })
    }

    cost, withBow := cityShoppingTestLoop(cityShoppingWallet,
        []state.InventoryItem{villageSword}).cityShoppingPlan()
    require.Equal(t, kitCost+bowCost, cost)
    require.True(t, withBow, "the wallet covers the kit and the bow")

    cost, withBow = cityShoppingTestLoop(int32(kitCost),
        []state.InventoryItem{villageSword}).cityShoppingPlan()
    require.Equal(t, kitCost, cost)
    require.False(t, withBow, "the kit only wallet leaves the bow")

    cost, withBow = cityShoppingTestLoop(int32(bowCost),
        ownedKit).cityShoppingPlan()
    require.Equal(t, bowCost, cost)
    require.True(t, withBow, "the owned kit plans the bow alone")

    withOldBow := make([]state.InventoryItem, 0, len(ownedKit)+1)
    withOldBow = append(withOldBow, ownedKit...)
    withOldBow = append(withOldBow, state.InventoryItem{
        ObjectID: 300, ItemID: 272, Count: 1,
    })
    cost, withBow = cityShoppingTestLoop(int32(bowCost),
        withOldBow).cityShoppingPlan()
    require.Equal(t, bowCost, cost)
    require.True(t, withBow,
        "the weaker Forest Bow still plans the upgrade at the full price")
}

// TestCityShoppingPlanStops pins the frozen route: the kit stops in
// purchase order, the bow stop last, the owned lines dropped and no
// walk to a merchant left with an empty list.
func TestCityShoppingPlanStops(t *testing.T) {
    loop := cityShoppingTestLoop(cityShoppingWallet, nil)
    stops := loop.cityShoppingPlanStops(false)
    require.Len(t, stops, 2)
    require.Equal(t, []int32{cityShopWeaponID}, stops[0].itemIDs)
    require.Equal(t, cityShoppingArmorStop.itemIDs, stops[1].itemIDs)

    stops = loop.cityShoppingPlanStops(true)
    require.Len(t, stops, 3)
    require.Equal(t, []int32{cityShopBowID}, stops[2].itemIDs,
        "the bow rides the last stop of the route")
    require.Equal(t, cityShoppingWeaponStop.merchant, stops[2].merchant)
    require.Equal(t, cityShoppingWeaponStop.listID, stops[2].listID)

    owned := make([]state.InventoryItem, 0, 7)
    for index, itemID := range CityShoppingKit() {
        owned = append(owned, state.InventoryItem{
            ObjectID: int32(100 + index), ItemID: itemID, Count: 1,
        })
    }
    stops = cityShoppingTestLoop(cityShoppingWallet,
        owned).cityShoppingPlanStops(true)
    require.Len(t, stops, 1, "an owned kit leaves the bow walk alone")
    require.Equal(t, []int32{cityShopBowID}, stops[0].itemIDs)
}

// TestOwnedBowAttackFollowsTheBag pins the version cache of the errand
// bow scan: a repeated ask answers from the cache, an inventory
// mutation refreshes it.
func TestOwnedBowAttackFollowsTheBag(t *testing.T) {
    loop := cityShoppingTestLoop(cityShoppingWallet,
        []state.InventoryItem{{
            ObjectID: 9, ItemID: 272, Count: 1,
        }})
    require.Equal(t, int32(49), loop.ownedBowAttack())
    require.Equal(t, int32(49), loop.ownedBowAttack(),
        "the cached answer survives the second ask")

    loop.tracker.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 10, ItemID: cityShopBowID, Count: 1, Change: 1,
    }})
    require.Equal(t, int32(64), loop.ownedBowAttack(),
        "an inventory mutation refreshes the scan")
}

// TestCityShoppingWornWeaponArms pins the paperdoll half of the
// armed gate: a worn village weapon qualifies without a bagged copy.
func TestCityShoppingWornWeaponArms(t *testing.T) {
    bot := newTestBot()
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 1, ItemID: 57, Count: cityShoppingWallet, Type2: 4},
        {
            ObjectID: 2, ItemID: vendorVillageSword, Count: 1,
            Equipped: true,
        },
    })
    bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 2))
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(&fakeNavigator{})
    require.True(t, loop.cityShoppingWanted())
}

// TestCityShoppingWantedNeedsTheSelfPosition pins the world entry
// gate: the errand cannot plan a walk home before the first UserInfo
// of the session placed the character.
func TestCityShoppingWantedNeedsTheSelfPosition(t *testing.T) {
    bot := state.NewBot("fresh")
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 1, ItemID: 57, Count: cityShoppingWallet, Type2: 4},
        {
            ObjectID: 2, ItemID: vendorVillageSword, Count: 1,
            Equipped: true,
        },
    })
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(&fakeNavigator{})
    require.False(t, loop.cityShoppingWanted())
}

// TestCityShoppingProfileGate pins the melee fence of the fixed plan:
// the archer and the caster keep their own gear runs, the melee
// fighter arms the errand.
func TestCityShoppingProfileGate(t *testing.T) {
    cases := []struct {
        name    string
        profile gear.Profile
        want    bool
    }{
        {
            name:    "the melee fighter arms the errand",
            profile: gear.MeleeFighter{}, want: true,
        },
        {
            name:    "the archer keeps its bow run",
            profile: gear.Archer{}, want: false,
        },
        {
            name:    "the caster keeps its staff run",
            profile: gear.MysticFighter{}, want: false,
        },
    }
    for _, testCase := range cases {
        t.Run(testCase.name, func(t *testing.T) {
            loop := cityShoppingTestLoop(cityShoppingWallet,
                []state.InventoryItem{{
                    ObjectID: 2, ItemID: vendorVillageSword, Count: 1,
                }})
            loop.equip.profile = testCase.profile
            require.Equal(t, testCase.want,
                loop.cityShoppingProfileAccepts())
            require.Equal(t, testCase.want,
                loop.cityShoppingWanted())
        })
    }
}

// TestMaybeCityShoppingHoldsTheFight pins the combat gate of the
// start: a pending loot and an incoming hit hold the errand off the
// tick, a merely held target drops before the walk.
func TestMaybeCityShoppingHoldsTheFight(t *testing.T) {
    cases := []struct {
        name        string
        phase       phase
        target      int32
        underAttack bool
        want        bool
        wantTarget  int32
    }{
        {
            name:  "a pending loot holds the start",
            phase: phaseLoot, target: 7, want: false, wantTarget: 7,
        },
        {
            name:  "an incoming hit holds the start",
            phase: phaseEngage, target: 7, underAttack: true,
            want: false, wantTarget: 7,
        },
        {
            name:  "a merely held target drops",
            phase: phaseEngage, target: 7, want: true, wantTarget: 0,
        },
    }
    for _, testCase := range cases {
        t.Run(testCase.name, func(t *testing.T) {
            bot := newTestBot()
            bot.ApplyItemList([]state.InventoryItem{
                {
                    ObjectID: 1, ItemID: 57, Count: cityShoppingWallet,
                    Type2: 4,
                },
                {ObjectID: 2, ItemID: vendorVillageSword, Count: 1},
            })
            if testCase.underAttack {
                spawnMob(bot)
                mobHitsCharacter(bot)
            }
            loop := NewLoop(&fakeGame{}, bot)
            loop.SetNavigator(&fakeNavigator{})
            loop.phase = testCase.phase
            loop.target = testCase.target
            // The closed context fails the started errand at its first
            // stage instead of walking a live world.
            ctx, cancel := context.WithCancel(context.Background())
            cancel()

            ran := loop.maybeCityShopping(ctx)

            require.Equal(t, testCase.want, ran)
            require.Equal(t, testCase.wantTarget, loop.target)
        })
    }
}

// cityShoppingWallet is the wallet of the scenario: the full plan of
// the kit and the bow (748 560 adena) leaves 51 440 adena behind.
const cityShoppingWallet = int32(800000)

// TestStartCityShoppingPublishesTheErrandStatus pins the fail fast
// hand-off of the errand: the tracker status carries the start and
// the terminal error, so the acceptance watch never idles out its
// own budget on an errand that already stopped.
func TestStartCityShoppingPublishesTheErrandStatus(t *testing.T) {
    // The bare tracker has no self position: the errand fails at its
    // first check instead of walking a live world.
    bot := state.NewBot("unittest1")
    loop := NewLoop(&fakeGame{}, bot)

    loop.startCityShopping(context.Background())

    errand := bot.HuntErrand()
    require.Equal(t, CityShoppingErrand, errand.Name)
    require.False(t, errand.StartedAt.IsZero(),
        "the errand must stamp its start")
    require.False(t, errand.EndedAt.IsZero(),
        "the terminal status must stamp its end")
    require.Contains(t, errand.Err, "no self position",
        "the terminal error must reach the watch")
    require.Equal(t, phaseEngage, loop.phase,
        "the loop returns to engage after the errand")
}
