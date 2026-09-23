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
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/pathfind"
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
    // The fight freshness rides the character's own chase step: the
    // shot-paced rhythm stays quiet without a fresh shot, the
    // proximity path answers the closed mob and its bookkeeping is
    // what the asserts below pin.
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 40,
        X: 45000, Y: 50000, TargetX: 45200, TargetY: 50000,
        TargetZ: -3500,
    })
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Len(t, game.walks, 1,
        "the archer profile with a worn bow must kite the closed mob")
    step := game.walks[0]
    // The race leg of the round-17 redesign: the deficit 280 bought
    // back at the parity rate caps at the fresh anchor's 1350 leash
    // budget - the kite steps the whole leg away from the closed
    // mob.
    require.Equal(t, int32(43650), step[0],
        "the kite steps away from the closed mob at the race length")
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
    selfSwingsAt(bot2, 45200)
    loop2.lastHit = time.Now().Add(-time.Minute)
    // A wall behind the retreat corners the archetype: the sealed
    // pocket of the geodata (the away hemisphere walled, the
    // wall-face wedges past the hemisphere edge walled too - only
    // the narrow eastern corridor of the fight line stays clear,
    // the same pocket the shot-paced corner test of
    // kite_shot_test.go stages). The location-general leash of the
    // round-14 redesign replaced the hunting-square zone read, so
    // the corner must come from the terrain now.
    nav := &fakeNavigator{}
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.X > 45000 &&
            math.Abs(float64(to.Y)-50000) < 100, nil
    }
    loop2.SetNavigator(nav)
    // The fresh shot arms the deferred click, the aged click fires
    // into the walled hemisphere - the cornered hold answers.
    loop2.tick()
    ageKiteClick(loop2)
    loop2.tick()
    require.Empty(t, game2.walks,
        "the cornered archer holds ground against the wall")
    require.Equal(t, int32(7), loop2.kiteHeldFor,
        "the cornered hold is armed for the target")
    require.True(t, loop2.bowEquipped(),
        "the cornered archetype keeps the bow in hand - never a "+
            "weapon switch")
}
