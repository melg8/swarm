// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

package gear

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestArcherScoresOnlyBows pins the archer profile contract: the bows
// rank by their ranged output, the melee families score zero (the
// archetype never swaps to a melee weapon), the shields score zero
// (the left hand carries the quiver) and the armor and jewels rank
// like the fighter's.
func TestArcherScoresOnlyBows(t *testing.T) {
    profile := Archer{}
    require.Equal(t, "archer", profile.Name())

    bowStats, ok := npcdata.ItemGearStats(13)
    require.True(t, ok, "the short bow must exist in the item stats")
    require.Equal(t, "BOW", bowStats.WeaponType)
    require.Positive(t, profile.WeaponScore(bowStats),
        "the bow is the weapon of the archer")
    require.Equal(t, bowStats.PAtk, profile.WeaponPoints(bowStats))

    swordStats, ok := npcdata.ItemGearStats(1)
    require.True(t, ok, "the short sword must exist in the item stats")
    require.Zero(t, profile.WeaponScore(swordStats),
        "the melee families score zero: no melee weapon in the archer's hand")
    require.Zero(t, profile.WeaponPoints(swordStats))

    // The shield scores zero: the quiver owns the left hand.
    shieldStats, ok := npcdata.ItemGearStats(19)
    require.True(t, ok, "the small shield must exist in the item stats")
    require.Zero(t, profile.ShieldScore(shieldStats),
        "the planner must never place a shield on the archer")

    // The armor and the jewels rank by their defenses like the
    // fighter's.
    melee := MeleeFighter{}
    shirtStats, ok := npcdata.ItemGearStats(21)
    require.True(t, ok)
    require.InDelta(t, melee.ArmorScore(shirtStats),
        profile.ArmorScore(shirtStats), 0.0001)
    earringStats, ok := npcdata.ItemGearStats(112)
    require.True(t, ok)
    require.InDelta(t, melee.JewelScore(earringStats),
        profile.JewelScore(earringStats), 0.0001)
}

// TestArcherQuiverPredicates pins the profile predicates of the bow
// tooling: the melee fighter lures with the bow, the archer shoots it
// as the primary weapon, the mystic never touches one - both bow
// shooters carry the quiver the junk keeps and the shop restock
// serve.
func TestArcherQuiverPredicates(t *testing.T) {
    require.True(t, BowLurer(MeleeFighter{}))
    require.False(t, BowLurer(Archer{}),
        "the archer's bow is the weapon itself, not a luring tool")
    require.False(t, BowLurer(MysticFighter{}))

    require.True(t, QuiverCarrier(MeleeFighter{}))
    require.True(t, QuiverCarrier(Archer{}))
    require.False(t, QuiverCarrier(MysticFighter{}))

    require.True(t, IsArcher(Archer{}))
    require.False(t, IsArcher(MeleeFighter{}))
    require.False(t, IsArcher(MysticFighter{}))
}

// TestArcherWeaponMilestoneBuysTheBow pins the archer gear plan: the
// weapon milestone itself buys the bow (the profile ranks the bows,
// so the generic weapon candidates carry them) - never a melee
// weapon, and never a second lure-tool copy of a bow the milestone
// already owns.
func TestArcherWeaponMilestoneBuysTheBow(t *testing.T) {
    profile := Archer{}
    // A bare-handed archer with a wallet that reaches the mid tier:
    // the first purchase must be a bow.
    equipment := NewEquipment(nil, [state.PaperdollSlots]int32{})
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 50000)
    require.NotEmpty(t, purchases)
    firstStats, ok := npcdata.ItemGearStats(purchases[0].ItemID)
    require.True(t, ok)
    require.Equal(t, "BOW", firstStats.WeaponType,
        "the first purchase of a bare-handed archer is the bow")

    bowBuys := 0
    for index := range purchases {
        stats, statsOK := npcdata.ItemGearStats(purchases[index].ItemID)
        if statsOK && stats.WeaponType == "BOW" {
            bowBuys++
        }
        require.NotEqual(t, int32(1), purchases[index].ItemID,
            "the short sword never enters the archer's plan")
    }
    require.Equal(t, 1, bowBuys,
        "one bow per trip: the milestone owns it, no lure-tool duplicate")

    // The starter sword worn scores zero: the bow is still a strict
    // upgrade over it, the milestone buys the bow directly.
    equipment = bowEquipment(nil)
    purchases = PlanPurchases(profile, equipment, bowTestCatalog(), 50000)
    require.NotEmpty(t, purchases)
    firstStats, ok = npcdata.ItemGearStats(purchases[0].ItemID)
    require.True(t, ok)
    require.Equal(t, "BOW", firstStats.WeaponType,
        "the bow upgrades the zero scoring melee sword straight away")
}

// TestArcherArrowRestock pins the quiver economy of the archer: a low
// arrow stack under the worn bow restocks to the target, a healthy
// quiver buys nothing, and the arrows ride behind the weapon
// milestone of the trip.
func TestArcherArrowRestock(t *testing.T) {
    profile := Archer{}
    // The worn short bow plus a nearly empty quiver.
    equipment := bowEquipmentWithWorn(13, []state.InventoryItem{
        {ObjectID: 503, ItemID: 17, Count: arrowRestockFloor - 1},
    })
    purchases := PlanPurchases(profile, equipment, bowTestCatalog(), 3000)
    var arrows *Purchase
    for index := range purchases {
        if purchases[index].ItemID == 17 {
            arrows = &purchases[index]
        }
    }
    require.NotNil(t, arrows, "the archer restocks the quiver it shoots")
    require.Equal(t, int32(arrowRestockTarget-arrowRestockFloor+1),
        arrows.Count)

    // A full quiver rests nothing.
    equipment = bowEquipmentWithWorn(13, []state.InventoryItem{
        {ObjectID: 503, ItemID: 17, Count: arrowRestockFloor},
    })
    purchases = PlanPurchases(profile, equipment, bowTestCatalog(), 3000)
    for index := range purchases {
        require.NotEqual(t, int32(17), purchases[index].ItemID,
            "a healthy quiver must not restock")
    }
}

// TestArcherPlannerPlacesTheBowAndKeepsTheLeftHandFree pins the
// paperdoll side: the equip planner places an owned bow over a worn
// melee weapon for the archer (the strict upgrade under the profile),
// and never places a shield - the left hand stays free for the quiver
// the hunt loop arms.
func TestArcherPlannerPlacesTheBowAndKeepsTheLeftHandFree(t *testing.T) {
    profile := Archer{}
    // A worn short sword plus a better bow in the bag.
    equipment := bowEquipment([]state.InventoryItem{
        {ObjectID: 502, ItemID: 13, Count: 1},
    })
    actions := BurstUpgrade(profile, equipment)
    found := false
    for _, action := range actions {
        if action.Slot == SlotRHand {
            equipped, ok := equipment.itemByID(action.ObjectID)
            require.True(t, ok)
            stats, statsOK := npcdata.ItemGearStats(equipped.ItemID)
            require.True(t, statsOK)
            require.Equal(t, "BOW", stats.WeaponType,
                "the right hand upgrade of the archer is the bow")
            found = true
        }
        require.NotEqual(t, SlotLHand, action.Slot,
            "the planner never writes the left hand of the archer")
    }
    require.True(t, found, "the owned bow must replace the melee sword")
}
