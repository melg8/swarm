// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "log"
    "testing"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The unit gates of the 2026-09-21 owner report fix: the trip
// execution used the inventory ENTRY count for every purchase filter,
// so the partial 101 arrow stack (one entry) blocked its own count
// aware restock order and the arrival gate would have confirmed a
// stack order instantly (the server merges the delivery into the
// carried stack, the id presence never changes). The wearable orders
// keep the family copy semantics - the pair families fill both slots,
// a single slot family blocks its second copy.

// newGateLoop builds a loop with the tracker carrying the given
// inventory and returns it with its log buffer.
func newGateLoop(
    t *testing.T, items []state.InventoryItem,
) (*Loop, *bytes.Buffer) {
    t.Helper()
    loop, _, bot, _ := newTripLoop()
    bot.ApplyItemList(items)
    logBuf := &bytes.Buffer{}
    loop.SetLogger(log.New(logBuf, "", 0))

    return loop, logBuf
}

// arrowReproAdenaStack is the adena entry of the gate fixtures: the
// 6000 wallet of the reproduction.
func arrowReproAdenaStack() state.InventoryItem {
    return state.InventoryItem{
        ObjectID: round60AdenaObjectID, ItemID: 57, Count: 6000,
        Type2: 4,
    }
}

// TestDropOwnedPurchasesKeepsTheStackTopUp pins the count aware drop
// rule of the stackable orders: the partial stack under the restock
// floor keeps its top up, the covered order drops, the wearable
// family rule stays intact.
func TestDropOwnedPurchasesKeepsTheStackTopUp(t *testing.T) {
    loop, logBuf := newGateLoop(t, []state.InventoryItem{
        {
            ObjectID: arrowReproArrowsObjectID,
            ItemID:   arrowReproWoodenArrowID,
            Count:    101,
        },
        {
            ObjectID: 200, ItemID: 29, Count: 1, Equipped: true,
            BodyPart: 0x800,
        },
        arrowReproAdenaStack(),
    })

    kept := loop.dropOwnedPurchases([]gear.Purchase{
        {
            ItemID: arrowReproWoodenArrowID, ListID: 3015000,
            MerchantTemplateID: 7150, Count: 499, Price: 998,
            Affordable: true,
        },
    })
    require.Len(t, kept, 1,
        "the 101 arrow stack must not block its 499 arrow top up")
    require.Empty(t, logBuf.String(), "the kept order logs nothing")

    kept = loop.dropOwnedPurchases([]gear.Purchase{
        {
            ItemID: arrowReproWoodenArrowID, ListID: 3015000,
            MerchantTemplateID: 7150, Count: 499, Price: 998,
            Affordable: true,
        },
        {
            ItemID: arrowReproWoodenArrowID, ListID: 3015000,
            MerchantTemplateID: 7150, Count: 100, Price: 200,
            Affordable: true,
        },
    })
    require.Len(t, kept, 1,
        "the stale order the carried stack covers drops, the top up stays")
    require.Equal(t, int32(499), kept[0].Count,
        "the surviving order is the uncovered top up")
    require.Contains(t, logBuf.String(),
        "the Wooden Arrow stack already covers the order, "+
            "skipping the purchase")
}

// TestDropOwnedPurchasesKeepsTheFamilyRule pins the wearable side of
// the gate: the worn piece blocks its second copy, the pair family
// still fills both slots, the unowned scroll order stays.
func TestDropOwnedPurchasesKeepsTheFamilyRule(t *testing.T) {
    loop, logBuf := newGateLoop(t, []state.InventoryItem{
        {
            ObjectID: 200, ItemID: 29, Count: 1, Equipped: true,
            BodyPart: 0x800,
        },
        {ObjectID: 105, ItemID: 112, Count: 1, Equipped: true},
        arrowReproAdenaStack(),
    })

    kept := loop.dropOwnedPurchases([]gear.Purchase{
        {ItemID: 29, ListID: 3014800, MerchantTemplateID: 7148,
            Count: 1, Affordable: true},
        {ItemID: 112, ListID: 3014900, MerchantTemplateID: 7149,
            Count: 1, Affordable: true},
        {ItemID: 1063, ListID: 3015000, MerchantTemplateID: 7150,
            Count: 1, Affordable: true},
    })
    require.Len(t, kept, 2,
        "the worn pants order drops, the pair family fills both slots")
    require.Contains(t, logBuf.String(),
        "Leather Pants already in the inventory, skipping the purchase")
}

// TestBuysArrivedWaitsForTheStackGrowth pins the count aware arrival
// rule: the stack order confirms only when the carried count grew
// past the baseline the batch left behind plus the order, the wearable
// orders keep the id presence signal.
func TestBuysArrivedWaitsForTheStackGrowth(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    bot.ApplyItemList([]state.InventoryItem{
        {
            ObjectID: arrowReproArrowsObjectID,
            ItemID:   arrowReproWoodenArrowID,
            Count:    101,
        },
        arrowReproAdenaStack(),
    })
    batch := []gear.Purchase{{
        ItemID: arrowReproWoodenArrowID, ListID: 3015000,
        MerchantTemplateID: 7150, Count: 499, Price: 998,
        Affordable: true,
    }}
    loop.buyRequested = batch
    loop.buyBaseline = map[int32]int32{
        arrowReproWoodenArrowID: 101,
    }

    require.False(t, loop.buysArrived(batch),
        "the carried stack without the delivery holds the gate")

    // A lost baseline (a fresh process after a restart mid trip)
    // degrades to the order coverage check, never to the instant
    // confirmation the id presence check delivered: the partial stack
    // below the order still holds the gate.
    loop.buyBaseline = nil
    require.False(t, loop.buysArrived(batch),
        "the missing baseline asks for the order coverage")

    // The delivery merges into the stack entry: only the count grows.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: arrowReproArrowsObjectID,
            ItemID:   arrowReproWoodenArrowID,
            Count:    600, Change: 2,
        },
    })
    require.True(t, loop.buysArrived(batch),
        "the grown stack confirms the delivery")
}

// TestBuysArrivedKeepsTheIdPresenceRule pins the wearable arrival
// signal: the appearance of the id is the arrival, the absence holds
// the gate for the retry budget.
func TestBuysArrivedKeepsTheIdPresenceRule(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    bot.ApplyItemList([]state.InventoryItem{
        arrowReproAdenaStack(),
    })
    batch := []gear.Purchase{{
        ItemID: 29, ListID: 3014800, MerchantTemplateID: 7148,
        Count: 1, Price: 5715, SellFirst: []int32{200},
        Affordable: true,
    }}

    require.False(t, loop.buysArrived(batch),
        "the absent wearable holds the gate")
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 210, ItemID: 29, Count: 1, Change: 1},
    })
    require.True(t, loop.buysArrived(batch),
        "the appeared wearable confirms the delivery")
}
