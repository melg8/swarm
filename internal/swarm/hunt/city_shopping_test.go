// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// vendorVillageSword is the level 15 village weapon the errand
// upgrades: the armed gate reads a bagged or worn weapon.
const vendorVillageSword = int32(2)

// TestCityShopStockMatchesTheNamedLists pins the live stock facts of
// the errand: every purchase rides the buylist of its merchant, so
// the buy request cannot drift from the generated shop data.
func TestCityShopStockMatchesTheNamedLists(t *testing.T) {
    for _, stop := range cityShoppingStops {
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

// TestCityShopPrice pins the Gludio counter prices of the kit: the
// reference price plus the 20 percent castle tax. The full kit costs
// 455 760 adena, the budget the errand gate checks.
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
    }
    total := int64(0)
    for _, testCase := range cases {
        require.Equal(t, testCase.want, cityShopPrice(testCase.itemID),
            "the price of %s", npcdata.ItemName(testCase.itemID))
        total += testCase.want
    }
    require.Equal(t, int64(455760), total,
        "the full kit must stay under the 500 000 adena budget")
}

// TestCityShopBuysShape pins the frozen purchase lines of one stop:
// one line per item, the whole stop riding its own buylist, one
// unit per line at the Gludio price.
func TestCityShopBuysShape(t *testing.T) {
    for _, stop := range cityShoppingStops {
        buys := cityShopBuys(stop)
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

// cityShoppingTestLoop builds a loop with the armed seam, a navigator
// and the given wallet and bag.
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
    loop.SetCityShoppingEnabled(true)

    return loop
}

// TestCityShoppingWantedGate pins the start decision matrix of the
// errand: the seam, the navigator, the armed weapon, the missing kit,
// the wallet, the cooldown and the running trip all gate the start.
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
            name:  "the seam off keeps the errand off",
            adena: cityShoppingWallet, bag: []state.InventoryItem{
                villageSword,
            },
            setup: func(loop *Loop) { loop.SetCityShoppingEnabled(false) },
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
            name:  "the owned kit ends the errand",
            adena: 0, bag: append([]state.InventoryItem{
                villageSword,
            }, ownedKit...), want: false,
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
    loop.SetCityShoppingEnabled(true)
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
    loop.SetCityShoppingEnabled(true)
    require.False(t, loop.cityShoppingWanted())
}

// cityShoppingWallet is the wallet of the scenario: the full Gludio
// kit price leaves 44 240 adena behind.
const cityShoppingWallet = int32(500000)

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
