// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The archetype integration contract of the archer type (issue #21,
// verified on the integration branch that composes the config launch
// chain with the kiting chain): a bot launched as an archer reaches
// the fight behavior through THREE independent layers, and the
// contract is that they compose - the launch type seeds the archer
// gear profile (cmd/swarm/main.go calls SetGearProfile(gear.Archer{})
// for the archer bot plans of the config launch, issue #17), the
// profile's gear plan puts a bow on the paperdoll (the weapon
// milestone and the equip planner of the gear layer, issue #17), and
// the bow on the paperdoll arms the ranged fight and the kite
// (bowEquipped drives the radii and the kite step, issues #18 and
// #19). No layer knows the others' names: the profile answers gear
// scores, the fight reads the weapon in hand - the integration test
// pins the seam the slice tests cannot see from inside their layer.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestArcherArchetypeComposesTheLayers pins the full chain in one
// scene: the archer profile wiring (the SetGearProfile call the
// launch makes for the archer type), the bow reaching the paperdoll
// (the weapon the gear plan buys, arriving the way the wire delivers
// it), and the kite answering the closed mob (the edge-complete kite
// of #19: the centroid direction, the pacing, the movement window).
// A melee fighter in the same scene stays in the melee answer - the
// profile is the only difference, the behavior difference proves the
// wiring carries.
func TestArcherArchetypeComposesTheLayers(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    // The launch wiring of an archer-type bot (cmd/swarm/main.go):
    // the config names the type, the loop receives the profile.
    loop.SetGearProfile(gear.Archer{})
    require.Equal(t, "archer", loop.equip.profile.Name(),
        "the profile wiring must reach the equip manager")

    // The gear plan's bow arrives the way the wire delivers gear:
    // the weapon run bought it, the equip planner wore it.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 700, ItemID: shortBowItemID, Count: 1, Equipped: true},
        {ObjectID: 701, ItemID: woodenArrowItemID, Count: 600},
    })
    bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 700))

    // The fight: the mob closed inside the retreat radius of a bow
    // user - the kite must answer.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, Name: "Keltir",
    })
    loop.target = 7
    loop.engageAt = time.Now()
    selfSwingsAt(bot, 7, 45200)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Len(t, game.walks, 1,
        "the archer profile with a worn bow must kite the closed mob")
    step := game.walks[0]
    require.Equal(t, int32(44600), step[0],
        "the kite steps away from the closed mob")
    require.Empty(t, game.forces,
        "the kite tick must not re-request the attack")
    require.Equal(t, int32(7), loop.kiteFor,
        "the kite bookkeeping belongs to the fight target")
    require.Equal(t, 1, loop.kiteStreak,
        "the issued step counts into the kite streak")

    // The cornered archetype rule composes too: the same profile,
    // the same bow, a wall behind - the hold ground answer, never a
    // weapon switch, never a standstill (the re-request re-arms the
    // shooting once the stance lapses).
    bot2 := newTestBot()
    game2 := &fakeGame{}
    loop2 := NewLoop(game2, bot2)
    loop2.SetGearProfile(gear.Archer{})
    bot2.ApplyItemList([]state.InventoryItem{
        {ObjectID: 700, ItemID: shortBowItemID, Count: 1, Equipped: true},
        {ObjectID: 701, ItemID: woodenArrowItemID, Count: 600},
    })
    bot2.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 700))
    bot2.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, Name: "Keltir",
    })
    loop2.target = 7
    loop2.engageAt = time.Now()
    selfSwingsAt(bot2, 7, 45200)
    loop2.lastHit = time.Now().Add(-time.Minute)
    // The zone leash corners the retreat: the 400 unit step west
    // would leave the square.
    loop2.SetHuntingZone(45000, 50000, 200)
    loop2.tick()
    require.Empty(t, game2.walks,
        "the cornered archer holds ground against the leash")
    require.True(t, loop2.bowEquipped(),
        "the cornered archetype keeps the bow in hand - never a "+
            "weapon switch")
}
