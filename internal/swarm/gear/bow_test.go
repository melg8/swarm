// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// bowTestCatalog mirrors the elven village deployment with the weapon
// trader (the bows) and the grocery trader (the arrows).
func bowTestCatalog() Catalog {
    return Catalog{Shops: []Shop{
        {MerchantTemplateID: 7147, TaxRate: 0.15, Lists: []int32{3014700}},
        {MerchantTemplateID: 7150, TaxRate: 0.15, Lists: []int32{3015000}},
    }}
}

// bowEquipment builds the working set of the bow tests: a worn short
// sword plus the optional owned pieces.
func bowEquipment(owned []state.InventoryItem) Equipment {
    return bowEquipmentWithWorn(shortSwordItemIDGear, owned)
}

// bowEquipmentWithWorn builds the working set with the given worn
// right hand weapon plus the owned pieces.
func bowEquipmentWithWorn(worn int32, owned []state.InventoryItem) Equipment {
    items := []state.InventoryItem{
        {ObjectID: 501, ItemID: worn, Count: 1, Equipped: true},
    }
    items = append(items, owned...)
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 501

    return NewEquipment(items, paperdoll)
}

// shortSwordItemIDGear mirrors the short sword of the item stats.
const shortSwordItemIDGear = int32(1)

func TestBowPhaseBuysTheRangedToolBehindTheWeapon(t *testing.T) {
    profile := MeleeFighter{}
    // A wallet that cannot reach the weapon milestone rung still buys
    // the luring tool: the sword is worn, the bow phase owns the plan.
    equipment := bowEquipment(nil)
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 2500)
    var bow *Purchase
    for index := range purchases {
        if purchases[index].ItemID == 13 {
            bow = &purchases[index]
        }
    }
    require.NotNil(t, bow, "the short bow must be planned")
    require.Equal(t, buyPrice(13), bow.Price)
    require.Equal(t, int32(7147), bow.MerchantTemplateID)
    require.Equal(t, int32(3014700), bow.ListID)
    require.Equal(t, int32(1), bow.Count)
    // The quiver rides along: the arrows from the grocery list.
    var arrows *Purchase
    for index := range purchases {
        if purchases[index].ItemID == 17 {
            arrows = &purchases[index]
        }
    }
    require.NotNil(t, arrows, "the arrow restock must be planned")
    require.Equal(t, int32(7150), arrows.MerchantTemplateID)
    require.Equal(t, int32(3015000), arrows.ListID)
    require.Equal(t, int32(arrowRestockTarget), arrows.Count)
}

func TestBowPhaseUpgradesGradually(t *testing.T) {
    profile := MeleeFighter{}
    // The wallet covers the weapon milestone rung AND the Bow (14):
    // the next bow after the owned Short Bow, never a rung the wallet
    // overshoots.
    equipment := bowEquipmentWithWorn(3, []state.InventoryItem{
        {ObjectID: 502, ItemID: 13, Count: 1},
    })
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 50000)
    var bow *Purchase
    for index := range purchases {
        if purchases[index].ItemID == 14 {
            bow = &purchases[index]
        }
    }
    require.NotNil(t, bow, "the bow upgrade must be planned")
    // The owned bow answers no second copy.
    for index := range purchases {
        require.NotEqual(t, int32(13), purchases[index].ItemID,
            "the owned short bow must not be re-bought")
    }
    // The quiver is full: no restock with a healthy arrow stack.
    equipment = bowEquipmentWithWorn(3, []state.InventoryItem{
        {ObjectID: 502, ItemID: 13, Count: 1},
        {ObjectID: 503, ItemID: 17, Count: arrowRestockFloor},
    })
    purchases = PlanPurchases(profile, equipment, bowTestCatalog(), 50000)
    for index := range purchases {
        require.NotEqual(t, int32(17), purchases[index].ItemID,
            "a full quiver must not restock")
    }
}

func TestBowPhaseRestocksTheEmptyQuiver(t *testing.T) {
    profile := MeleeFighter{}
    equipment := bowEquipment([]state.InventoryItem{
        {ObjectID: 502, ItemID: 13, Count: 1},
        {ObjectID: 503, ItemID: 17, Count: arrowRestockFloor - 1},
    })
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 3000)
    var arrows *Purchase
    for index := range purchases {
        if purchases[index].ItemID == 17 {
            arrows = &purchases[index]
        }
    }
    require.NotNil(t, arrows)
    require.Equal(t, int32(arrowRestockTarget-arrowRestockFloor+1),
        arrows.Count, "the restock tops the quiver up to the target")
    // No bow upgrade: the owned short bow is the best the wallet
    // reaches, the quiver is the point of the trip.
    for index := range purchases {
        require.NotEqual(t, int32(14), purchases[index].ItemID)
    }
}

func TestBowPhaseStandsBehindTheWeaponMilestone(t *testing.T) {
    profile := MeleeFighter{}
    // A bare-handed character with an affordable weapon in the
    // catalog: the weapon milestone owns the first purchase, the bow
    // tool follows behind it, never ahead.
    equipment := NewEquipment(nil, [state.PaperdollSlots]int32{})
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 2000)
    require.NotEmpty(t, purchases)
    firstStats, ok := npcdata.ItemGearStats(purchases[0].ItemID)
    require.True(t, ok)
    require.NotEmpty(t, firstStats.WeaponType,
        "the first purchase of a bare-handed hunter is the weapon")
    bowAt := -1
    for index := range purchases {
        if purchases[index].ItemID == 13 {
            bowAt = index
        }
    }
    if bowAt >= 0 {
        require.Positive(t, bowAt,
            "the bow tool never runs ahead of the weapon milestone")
    }
}

func TestMysticNeverLures(t *testing.T) {
    profile := MysticFighter{}
    equipment := bowEquipment(nil)
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 5000)
    for index := range purchases {
        require.NotEqual(t, int32(13), purchases[index].ItemID,
            "the mystic casts from range, no luring tool")
        require.NotEqual(t, int32(17), purchases[index].ItemID)
    }
}

func TestArrowItemAndBowsOfTheCatalog(t *testing.T) {
    catalog := bowTestCatalog()
    require.Equal(t, int32(17), arrowItemID(catalog))
    listID, merchant, price, ok := arrowOffer(catalog, 17)
    require.True(t, ok)
    require.Equal(t, int32(3015000), listID)
    require.Equal(t, int32(7150), merchant)
    require.Equal(t, buyPrice(17), price)
    bows := bowOffers(catalog)
    require.NotEmpty(t, bows)
    // The strongest bow of the elven weapon list: the Forest Bow.
    require.Equal(t, int32(272), bows[0].itemID)
    forestStats, _ := npcdata.ItemGearStats(272)
    require.InDelta(t, float64(forestStats.PAtk), bows[0].score, 0.0001)
    // The offers carry the shop pricing.
    for _, bow := range bows {
        require.Equal(t, buyPrice(bow.itemID), bow.price)
    }
}
