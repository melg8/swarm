// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The water regression of 2026-09-10 (see pathfind/water_escape_test.go
// for the engine side): the town trip to the trader swam below the
// elven village plateau and stood paralyzed under its cliff - the
// server walks characters into water without any hesitation (no water
// cost in its own routing, and swimming move requests skip the
// geodata validation entirely) while every click toward the village
// deck above returns the character's own position. The follower now
// refuses wet click lines and recovers through the shore escape.

// TestTripWaterEscapePlansShoreWalk pins the recovery entry: a
// character standing over a lake bed during a town trip replaces the
// leg with the shore escape walk, clicks along the escape waypoints
// without the water guard and never towards the original village
// waypoint while it stands in the water.
func TestTripWaterEscapePlansShoreWalk(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    fillInventory(bot)
    nav.overWater = true
    nav.escapeRoute = []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3850},
        {X: 45600, Y: 50400, Z: -3770},
    }
    // The character floats over the lake bed (the first escape
    // waypoint is its own standing cell, like the real BFS plans).
    moveSelfTo(bot, 45000, 50000, -3800)

    loop.tick()
    require.Equal(t, phaseTownWalk, loop.phase,
        "the trip keeps walking while the escape runs")
    require.True(t, loop.waterEscape,
        "the water escape must be armed")
    require.Equal(t, nav.escapeRoute, loop.waypoints,
        "the escape waypoints must replace the trip leg")
    require.Zero(t, loop.wpIndex)
    require.Empty(t, game.walks,
        "the planning tick sends no walk yet")
    require.Equal(t, 1, nav.escapeCalls)

    // The next tick walks the escape: the click aims at the shore
    // waypoint even though the original village waypoint is closer -
    // the unclimbable cliff waypoint must not win the skip logic.
    loop.tick()
    require.Equal(t, [][3]int32{{45600, 50400, -3770}}, game.walks,
        "the escape walk must aim at the shore waypoint")
}

// TestTripWaterEscapeReplansLegOnShore pins the recovery exit: once
// the character walks onto dry ground, the escape drops and the
// interrupted trip leg re-plans from the shore with a fresh re-path
// budget.
func TestTripWaterEscapeReplansLegOnShore(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    fillInventory(bot)
    nav.overWater = true
    nav.escapeRoute = []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3850},
        {X: 45600, Y: 50400, Z: -3770},
    }

    loop.tick()
    require.True(t, loop.waterEscape)
    // The character walks out of the water.
    nav.overWater = false
    moveSelfTo(bot, 45600, 50400, -3770)
    loop.tick()
    require.False(t, loop.waterEscape,
        "the escape must drop on the dry shore")
    require.Zero(t, loop.rePaths,
        "the recovery leaves a fresh re-path budget")
    require.NotNil(t, loop.waypoints)
    require.NotEqual(t, nav.escapeRoute, loop.waypoints,
        "the trip leg must re-plan to the trader")
    legDest := state.WalkPoint{
        X: herbielPos[0], Y: herbielPos[1], Z: herbielPos[2],
    }
    snap := bot.Snapshot()
    require.NotNil(t, snap.WalkDest)
    require.Equal(t, legDest, *snap.WalkDest,
        "the re-planned leg still aims at the trader")
    require.Empty(t, game.walks,
        "the shore tick plans the new leg without walking yet")
}

// TestTripWaterEscapeWithoutShoreAborts pins the failure path: a
// character standing in water without any walkable shore aborts the
// trip instead of clicking into the cliff forever.
func TestTripWaterEscapeWithoutShoreAborts(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    fillInventory(bot)
    nav.overWater = true

    loop.tick()
    require.NotEqual(t, phaseTownWalk, loop.phase,
        "the trip must abort without a shore path")
    require.Equal(t, phaseEngage, loop.phase)
    require.False(t, loop.tripCooldownOver(),
        "the abort arms the trip cooldown")
}

// TestTripWaterEscapeStuckReplans pins the stuck handling of the
// escape: a swimming character that stands still re-plans the escape
// itself, not the town leg.
func TestTripWaterEscapeStuckReplans(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    fillInventory(bot)
    nav.overWater = true
    nav.escapeRoute = []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3850},
        {X: 45600, Y: 50400, Z: -3770},
    }

    loop.tick()
    require.True(t, loop.waterEscape)
    legSearches := len(nav.approachEnds)
    // The character stands still in the water past the stuck window.
    for range 20 {
        loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
        loop.tick()
    }
    require.GreaterOrEqual(t, nav.escapeCalls, 2,
        "the stuck escape must re-plan the escape itself")
    require.Len(t, nav.approachEnds, legSearches,
        "the stuck escape must never re-plan the town leg")
    require.Equal(t, phaseEngage, loop.phase,
        "the escape exhausts its budget and aborts the trip")
}

// TestTripWetClickWalksThePlan pins the priced water round: the plan
// prices every crossing at the swim rate, so the follower walks the
// wet legs it planned - a click line that crosses water goes out
// unchanged (the guard that refused wet clicks and re-planned around
// the shore was the outdated way this round retired), and the escape
// machinery owns the off-plan swims instead.
func TestTripWetClickWalksThePlan(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    fillInventory(bot)
    nav.wetLine = true

    loop.tick()
    require.NotEmpty(t, game.walks,
        "the planned wet click is sent to the server")
    require.Zero(t, loop.rePaths,
        "a wet click is not a refusal, no re-path burns")
    require.Equal(t, phaseTownWalk, loop.phase,
        "the trip keeps walking its priced plan")
}

// TestTripPlannedSwimKeepsFollowingThePlan pins the escape gate of
// the priced water round: a character floating over a lake bed whose
// aimed waypoint stands on the bed ahead (the crossing the search
// priced) keeps following the plan - the water escape does not arm,
// the click walks the wet waypoint.
func TestTripPlannedSwimKeepsFollowingThePlan(t *testing.T) {
    loop, game, _, nav := newTripLoop()
    nav.overWater = true
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.legDest = pathfind.Vec3{X: 45200, Y: 50200, Z: -3539}
    loop.legStart = pathfind.Vec3{X: 45000, Y: 50000, Z: -3800}
    // The plan crosses the lake: the aimed waypoint stands on the bed
    // ahead of the character.
    loop.waypoints = []pathfind.Vec3{
        {X: 44900, Y: 49900, Z: -3539},
        {X: 45050, Y: 50050, Z: -3850},
        {X: 45200, Y: 50200, Z: -3539},
    }
    loop.wpIndex = 1

    require.False(t, loop.walkTownWaypoints(),
        "the planned swim keeps walking")
    require.False(t, loop.waterEscape,
        "the escape must not arm for a priced crossing")
    require.Zero(t, nav.escapeCalls,
        "no shore search runs for a priced crossing")
    require.Len(t, game.walks, 1,
        "the walk click goes to the wet waypoint")
    require.Equal(t, [3]int32{45050, 50050, -3850}, game.walks[0],
        "the click aims the bed waypoint the plan prices")
}
