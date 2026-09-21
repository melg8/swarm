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

// The reproduction of the 2026-09-21 owner report: the bot carried a
// pair of pants for sale (the worn Leather Pants of a planned Hard
// Leather Pants replacement, worse than the upgrade) and 101 Wooden
// Arrows, reached the trader, sold the pants - and never bought the
// arrows its shop queue kept displaying. The arrow restock of the
// planner is count aware (101 arrows sit under the 150 restock floor,
// the plan tops the quiver up to 600 with a 499 arrow order), but the
// trip execution filtered the order out before any request went out:
// dropOwnedPurchases counted the inventory ENTRIES of the item id
// (the 101 arrow stack is one entry) against the family copy count
// and dropped the order as a "surplus copy" - while the queue went on
// advertising the arrows trip after trip.

// arrowReproAdena is the wallet of the reproduction: the replacement
// (through the worn pants' sell credit) plus the 998 adena arrow
// order both fit, every other rung of the ladder stays out of reach.
const arrowReproAdena = int64(6000)

// arrowReproArrowsObjectID is the object id of the partial arrow
// stack in the bag.
const arrowReproArrowsObjectID = int32(302)

// arrowReproWoodenArrowID is the Wooden Arrow display id (the lhand
// etc item the quiver restock buys).
const arrowReproWoodenArrowID = int32(17)

// applyArrowReproBag fills the bag of the reproduction: the junk
// stems the sell stop always carries, the luring bow (the planner
// buys no bow while the owned tool stands), the 101 arrow stack and
// nothing else.
func applyArrowReproBag(bot *state.Bot) {
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 300, ItemID: 1864, Count: 5, Type2: 5, Change: 1},
        {ObjectID: 301, ItemID: 13, Count: 1, Change: 1},
        {
            ObjectID: arrowReproArrowsObjectID,
            ItemID:   arrowReproWoodenArrowID,
            Count:    101, Change: 1,
        },
    })
}

// arrowPurchaseOf returns the arrow restock purchase of a plan
// (nil when the plan carries none).
func arrowPurchaseOf(plan []gear.Purchase) *gear.Purchase {
    for i := range plan {
        if plan[i].ItemID == arrowReproWoodenArrowID {
            return &plan[i]
        }
    }

    return nil
}

// arrowBatchOf returns the first buy batch that carries the arrow
// order (nil when no buy request held it).
func arrowBatchOf(buys [][]gear.Purchase) []gear.Purchase {
    for _, batch := range buys {
        for _, purchase := range batch {
            if purchase.ItemID == arrowReproWoodenArrowID {
                return batch
            }
        }
    }

    return nil
}

// TestReproArrowRestockBuysPastTheOwnedStack walks the reported trip:
// the sell stop at the nearest trader (Herbiel - the arrows merchant,
// the first stop group merges into the sell stop) sells the junk and
// the replaced pants, then the arrow restock order buys at the same
// trader despite the 101 arrow stack in the bag. The delivery merges
// into the carried stack (101 becomes 600), the confirmation
// completes the stop and the rest of the trip (the armor stop with
// the replacement) runs untouched.
func TestReproArrowRestockBuysPastTheOwnedStack(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    logBuf := &bytes.Buffer{}
    loop.SetLogger(log.New(logBuf, "", 0))
    // The reported character one trip earlier: every slot filled
    // including the Leather Pants (the replacement plans the Hard
    // Leather Pants and sells the worn piece first).
    applyRound60Gear(bot, arrowReproAdena, true)
    applyArrowReproBag(bot)
    // The keep one scroll stock is owned and the full newbie buff set
    // is active: the trip plans no scroll buy and no guide stop (the
    // SOE economy and the guide have their own tests).
    addSOE(bot, 1)
    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 1204, Level: 2, Time: 1200},
        {SkillID: 1040, Level: 3, Time: 1200},
        {SkillID: 1045, Level: 1, Time: 1200},
        {SkillID: 1068, Level: 1, Time: 1200},
        {SkillID: 1044, Level: 1, Time: 1200},
    })
    require.True(t, loop.shoppingWanted(),
        "the replacement plus the arrow restock plan a trip")

    // The trip starts and walks to the nearest merchant: Herbiel, the
    // grocery trader whose list carries the arrows.
    loop.tick()
    require.Equal(t, phaseTownWalk, loop.phase)
    arrows := arrowPurchaseOf(loop.tripPlan)
    require.NotNil(t, arrows,
        "the plan carries the arrow restock line under the floor")
    require.Equal(t, int32(499), arrows.Count,
        "the restock tops the 101 arrow stack up to the 600 target")
    require.Equal(t, int32(7150), arrows.MerchantTemplateID,
        "the arrow order pins Herbiel, the arrows merchant")

    // The sell stop: the junk sells first, then the replacement
    // unequips and sells the worn pants.
    arriveAtStop(t, loop, bot)
    settleMerchant(t, loop, game, bot, loop.tripStops[0].merchant, 55)
    loop.sellAt = time.Time{}
    loop.tick()
    require.NotEmpty(t, game.sells, "the junk batch sells")
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 300, ItemID: 1864, Count: 0, Type2: 5, Change: 3},
    })
    loop.sellAt = time.Now().Add(-sellPause - time.Second)
    loop.tick()
    require.Equal(t, []int32{round60PantsObjectID}, loop.replaceQueue)
    loop.replaceUnequipAt = time.Now().Add(-equipActionPeriod - time.Second)
    loop.tick()
    require.Contains(t, game.uses, round60PantsObjectID,
        "the replaced pants come off first")
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: round60PantsObjectID, ItemID: 29, Count: 1,
            Equipped: false, BodyPart: 0x800, Change: 2,
        },
    })
    bot.ApplyPaperdoll(round60Paperdoll(false))
    loop.sellAt = time.Now().Add(-sellPause - time.Second)
    loop.tick()
    pantsOffered := false
    for _, batch := range game.sells {
        for _, item := range batch {
            if item.ObjectID == round60PantsObjectID {
                pantsOffered = true
            }
        }
    }
    require.True(t, pantsOffered, "the pants are sold before the buys")
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: round60PantsObjectID, ItemID: 29, Count: 0, Change: 3},
    })

    // The arrows: the same stop carries the Herbiel buys of the plan
    // (the first merchant group merges into the sell stop - the bot
    // sells the pants to the trader and buys the arrows from him),
    // the partial stack in the bag must not block the restock.
    loop.buyAt = time.Now().Add(-buyPause - time.Second)
    loop.sellAt = time.Now().Add(-buyPause - time.Second)
    loop.tick()
    batch := arrowBatchOf(game.buys)
    require.NotNil(t, batch,
        "the arrow restock buys past the owned 101 arrow stack: %s",
        logBuf.String())
    require.Len(t, batch, 1, "the arrow order flies alone (list 3015000)")
    require.Equal(t, int32(3015000), batch[0].ListID)
    require.Equal(t, int32(499), batch[0].Count)
    require.NotEmpty(t, loop.buyRequested,
        "the arrow batch is in flight for the confirmation gate")

    // The server delivers the arrows into the carried stack (the
    // Mobius inventory merges the delivery, the stack entry only
    // grows) and the count aware confirmation completes the stop.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: arrowReproArrowsObjectID,
            ItemID:   arrowReproWoodenArrowID,
            Count:    600, Change: 2,
        },
    })
    loop.tick()
    require.Empty(t, loop.buyRequested,
        "the arrow delivery confirms the batch")

    // The confirmed batch completes the stop on the next tick (the
    // stop tick returns after the shopping settled): the trip advances
    // to the armor stop (the replacement buy at Ariel) and the rest of
    // the trip runs untouched - the replacement request goes out, the
    // silent refusal skips it through the retry budget, the trip walks
    // home.
    loop.tick()
    require.NotEqual(t, phaseTownSell, loop.phase,
        "the completed stop advances the trip")
    ariel := loop.tripStops[0].merchant
    arriveAtMerchantStand(bot, ariel)
    loop.tick()
    require.Equal(t, phaseTownSell, loop.phase,
        "the arrival at the armor stop enters the sell phase")
    settleMerchant(t, loop, game, bot, ariel, 57)
    loop.buyAt = time.Now().Add(-buyPause - time.Second)
    loop.sellAt = time.Now().Add(-buyPause - time.Second)
    loop.tick()
    legsBatch := false
    for _, stopBatch := range game.buys {
        for _, purchase := range stopBatch {
            if purchase.ItemID == 30 {
                legsBatch = true
            }
        }
    }
    require.True(t, legsBatch, "the replacement buy still goes out")
    for i := 1; i <= stopBuyRetries; i++ {
        loop.buyConfirmAt = time.Now().Add(-buyConfirmWait - time.Second)
        loop.buyAt = time.Now().Add(-buyPause - time.Second)
        loop.sellAt = time.Now().Add(-buyPause - time.Second)
        loop.tick()
    }
    loop.buyConfirmAt = time.Now().Add(-buyConfirmWait - time.Second)
    loop.buyAt = time.Now().Add(-buyPause - time.Second)
    loop.sellAt = time.Now().Add(-buyPause - time.Second)
    loop.tick()
    // The skipped batch completes the stop on the next tick; the
    // return segment arrives at the farm spot and the trip ends.
    loop.tick()
    for loop.phase == phaseTownReturn {
        moveSelfTo(bot, int32(loop.segmentDest.X),
            int32(loop.segmentDest.Y), int32(loop.segmentDest.Z))
        loop.tick()
    }
    require.False(t, loop.tripActive(),
        "the trip ends as a success after the confirmed arrows")
    require.NotContains(t, logBuf.String(),
        "already in the inventory, skipping the purchase",
        "the owned stack must not drop the count aware restock")
}
