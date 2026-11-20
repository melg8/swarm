// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The healing potion economy tests: the restock line, the wallet cap, the junk
// sell guard and the drink preference.

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// addPotions puts a potion stack into the test inventory.
func addPotions(bot *state.Bot, itemID int32, count int32) {
    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 700 + itemID, ItemID: itemID, Count: count, Type2: 5,
        Change: 1,
    }})
}

// TestPotionRestockLinePlansTheTarget pins the line: below the floor the queue
// orders the stronger potion the region sells, sized to the target count and
// priced with the town tax.
func TestPotionRestockLinePlansTheTarget(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    loop.zoneRegion = regionDion

    line, ok := loop.potionPurchaseLine(1_000_000)
    require.True(t, ok)
    require.Equal(t, healingPotionID, line.ItemID)
    require.Equal(t, int32(3006300), line.ListID)
    require.Equal(t, int32(7063), line.MerchantTemplateID)
    require.Equal(t, potionRestockTarget, line.Count)
    require.Equal(t, int64(330)+int64(float64(330)*dionTownTaxRate),
        line.Price/int64(line.Count))
    require.True(t, line.Affordable)

    addPotions(bot, healingPotionID, potionRestockFloor)

    _, ok = loop.potionPurchaseLine(1_000_000)
    require.False(t, ok, "a stock at the floor plans no restock")
}

// TestPotionRestockCapsByWallet pins the wallet cap: a thin purse buys what it
// covers instead of planning an unaffordable order, and a purse under one
// potion plans nothing.
func TestPotionRestockCapsByWallet(t *testing.T) {
    loop, _, _, _ := newTripLoop()
    loop.zoneRegion = regionDion
    unit := potionUnitPrice(regionDion, healingPotionID)

    line, ok := loop.potionPurchaseLine(unit * 3)
    require.True(t, ok)
    require.Equal(t, int32(3), line.Count)
    require.Equal(t, unit*3, line.Price)

    _, ok = loop.potionPurchaseLine(unit - 1)
    require.False(t, ok)
}

// TestPotionRestockFollowsTheRegionMerchant pins the elven fallback: the
// village Herbiel sells only the 1060 potion.
func TestPotionRestockFollowsTheRegionMerchant(t *testing.T) {
    loop, _, _, _ := newTripLoop()

    line, ok := loop.potionPurchaseLine(1_000_000)
    require.True(t, ok)
    require.Equal(t, lesserHealingPotionID, line.ItemID)
    require.Equal(t, int32(3015000), line.ListID)
    require.Equal(t, int32(7150), line.MerchantTemplateID)
}

// TestPotionStockNeverSells pins the junk guard: the sell list keeps every
// healing stack out of the vendor batch.
func TestPotionStockNeverSells(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    addPotions(bot, lesserHealingPotionID, 10)
    addPotions(bot, healingPotionID, 5)
    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 999, ItemID: 1864, Count: 3, Type2: 5, Change: 1,
    }})

    junk := loop.sellableJunk()
    require.Len(t, junk, 1)
    require.Equal(t, int32(1864), junk[0].ItemID)
}

// TestHealingPotionObjectPrefersTheStronger pins the drink choice: the 1061
// stack wins over the 1060 one.
func TestHealingPotionObjectPrefersTheStronger(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    addPotions(bot, lesserHealingPotionID, 4)
    addPotions(bot, healingPotionID, 2)

    objectID, ok := loop.healingPotionObject()
    require.True(t, ok)
    require.Equal(t, 700+healingPotionID, objectID)
}

// TestDrinkFiresBeforeTheFleeGate pins the order: a hurt fighting character
// spends a potion instead of opening the flee budget.
func TestDrinkFiresBeforeTheFleeGate(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    addPotions(bot, healingPotionID, 5)
    bot.ApplyUserInfo(state.UserInfo{
        Name: "potion", Level: 20, Race: 1, ClassID: 18,
        X: 1000, Y: 1000, Z: -3500,
        MaxHP: 500, CurHP: 300, MaxMP: 100, CurMP: 100,
    })
    loop.nowFn = func() time.Time { return time.Unix(1_700_000_000, 0) }

    loop.maybeDrinkFightPotion(loop.now())

    require.Equal(t, []int32{700 + healingPotionID}, game.uses)
    require.False(t, loop.questPotionAt.IsZero())
}
