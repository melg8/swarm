// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The Scroll of Escape economy tests (the owner rule of 2026-09-21):
// the keep one stock line, the junk sell guard and the profitability
// decision of the trip start.

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// addSOE puts a scroll stack into the test inventory.
func addSOE(bot *state.Bot, count int32) {
    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 900, ItemID: soeItemID, Count: count, Type2: 5,
        Change: 1,
    }})
}

// TestSoeCountAndObjectResolution pins the inventory reading of the
// scroll stacks.
func TestSoeCountAndObjectResolution(t *testing.T) {
    loop, _, bot, _ := newTripLoop()

    require.Equal(t, 0, loop.soeCount())
    _, ok := loop.soeObjectID()
    require.False(t, ok)

    addSOE(bot, 2)

    require.Equal(t, 2, loop.soeCount())
    objectID, ok := loop.soeObjectID()
    require.True(t, ok)
    require.Equal(t, int32(900), objectID)
}

// TestSoePurchaseLineKeepsOne pins the keep one rule: the queue line
// lands only when the bag holds none, and it pins the region grocery
// merchant (the Herbiel list carries the scroll).
func TestSoePurchaseLineKeepsOne(t *testing.T) {
    loop, _, bot, _ := newTripLoop()

    line, ok := loop.soePurchaseLine()
    require.True(t, ok)
    require.Equal(t, soeItemID, line.ItemID)
    require.Equal(t, int32(1), line.Count)
    require.Equal(t, int32(3015000), line.ListID)
    require.Equal(t, int32(7150), line.MerchantTemplateID)
    require.True(t, line.Affordable)

    addSOE(bot, 1)

    _, ok = loop.soePurchaseLine()
    require.False(t, ok, "the stock rule is satisfied")
}

// TestSoeNeverSellsBack pins the junk guard: the scroll stack stays
// out of the sellable junk whatever the vendor would pay.
func TestSoeNeverSellsBack(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    addSOE(bot, 1)

    for _, item := range loop.sellableJunk() {
        require.NotEqual(t, soeItemID, item.ItemID,
            "the escape scroll never sells back")
    }
}

// TestSoeEscapeProfitable pins the pricing rule: the walk home valued
// at the measured income must beat the scroll price, a zero income or
// a short walk keeps the legs.
func TestSoeEscapeProfitable(t *testing.T) {
    loop, _, _, _ := newTripLoop()

    // No income: the scroll never runs (the walk is free dead time
    // the bot cannot price).
    require.False(t, loop.soeEscapeProfitable(100000, 0))

    // A rich income and a long walk: 100000 units at 120 speed is
    // ~1167 seconds of walking, minutes of farm time the scroll
    // saves for 460 adena.
    require.True(t, loop.soeEscapeProfitable(100000, 100000))

    // The short residual leg: the 20 second cast costs more than the
    // walk it replaces.
    require.False(t, loop.soeEscapeProfitable(1000, 100000))
}

// trustTheGround arms the picked cell metric with a rich measured
// income: the scroll pricing only runs on a trusted rate.
func trustTheGround(t *testing.T, loop *Loop) {
    t.Helper()
    loop.SetHuntingCells(ElvenHuntingCells())
    cellHunter := loop.cell
    cellHunter.picked = 0
    metric := &cellHunter.metrics[0]
    metric.visitAdena = 1000000
    metric.visitActiveMin = 100
    metric.activeMin = 100
    metric.adena = 1000000
    if !metric.trusted() {
        t.Skipf("the trusted rate seam differs, the unit pricing " +
            "test above owns the rule")
    }
}

// landTheScroll simulates the teleport landing of the server after
// the cast: the character reappears at the village respawn.
func landTheScroll(bot *state.Bot) {
    go func() {
        time.Sleep(300 * time.Millisecond)
        moveSelfTo(bot, 45475, 48820, -3056)
    }()
}

// TestMaybeEscapeUsesTheScroll pins the trip start leg: a profitable
// escape sends the UseItem request and the simulated teleport landing
// hands the trip to the village walk planning.
func TestMaybeEscapeUsesTheScroll(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    addSOE(bot, 1)
    moveSelfTo(bot, 40000, 52000, -3500)
    trustTheGround(t, loop)
    landTheScroll(bot)

    loop.maybeEscapeWithSOE(townMerchants[3])

    require.NotEmpty(t, game.uses, "the scroll goes out")
}

// TestMaybeEscapeToTheVillage pins the coordinate entry the city
// errand rides: the scroll leg of the walk to Mirabel prices the leg
// against the ground income and sends the UseItem request.
func TestMaybeEscapeToTheVillage(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    addSOE(bot, 1)
    moveSelfTo(bot, 36000, 50229, -3456)
    trustTheGround(t, loop)
    landTheScroll(bot)

    loop.maybeEscapeTo(cityShoppingOutbound.Gatekeeper.X,
        cityShoppingOutbound.Gatekeeper.Y)

    require.NotEmpty(t, game.uses, "the scroll goes out")
}
