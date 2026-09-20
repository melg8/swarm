// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The water round of 2026-09-10 and the priced round of 2026-09-19:
// the town trip to the trader Ariel once entered the elven village
// lake and stood paralyzed under the plateau cliff (the water zone of
// the C1 server covers everything below -3780). The priced form that
// survived every later round: the plans price every crossing at the
// swim rate - the water is walkable, the slowdown honest. The escape
// machinery and the wet click raster of the grid era retired with the
// mesh round (the mesh plans out of every standing cell, the swim
// priced - see the navmesh world water reality tests).

// shoreLand is the dry test land height (above the C1 water surface).
const shoreLand = int16(-3770)

// shoreBed is the underwater test bed height: 30 below the land, so
// the cell steps of a shore ramp stay within the passable height and
// the line of sight across the bed passes - the exact shape of the
// elven lake the string pulling used to cut through.
const shoreBed = int16(-3800)

// TestPricedRouteKeepsTheShorePivots pins the water awareness of the
// smoothing under the priced search: a narrow underwater band (whole
// flat blocks, so the water is actually swimmable) splits two dry
// points inside its y range, the priced search detours it (the 2.3x
// swim rate makes the crossing the slower walk) and the smoothing
// must not collapse the detour back into the straight water crossing
// - every segment of the smoothed route stays dry and walkable.
func TestPricedRouteKeepsTheShorePivots(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    // The narrow band: block columns 38..43 (cells 304..351), block
    // row 88 (cells 704..711).
    waterBlock := blockSpec{
        kind:  blockFlat,
        cells: [cellsPerBlock]Layer{{Height: shoreBed, NSWE: nsweAll}},
    }
    for bx := 38; bx <= 43; bx++ {
        spec.blocks[bx][88] = waterBlock
    }
    engine := newTestEngine(t, spec)

    start := worldOf(200, 707, shoreLand)
    end := worldOf(600, 707, shoreLand)
    result, err := engine.FindPath(
        start, end, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, result.Found, "the detour around the band exists")
    require.GreaterOrEqual(t, len(result.Waypoints), 3,
        "the route must keep the shore pivots the search paid for")
    for i, wp := range result.Waypoints {
        require.GreaterOrEqual(t, wp.Z, float64(WaterLevel),
            "waypoint %d must stay dry", i)
    }
    // Every smoothed segment is a clean dry walk: the follower may click
    // straight along each segment without entering the water.
    for i := 1; i < len(result.Waypoints); i++ {
        require.False(t, lineWet(engine, result.Waypoints[i-1],
            result.Waypoints[i]),
            "the segment %d must stay dry", i-1)
    }
}

// TestOverWater verifies the over water query: the layer closest to
// the reference z decides, so a swimmer above the bed of a multilayer
// cell reports over water while a character on the deck above the
// same cell does not.
func TestOverWater(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    spec.setMultilayer(500, 500, []Layer{
        {Height: -3850, NSWE: nsweAll},
        {Height: -2992, NSWE: nsweAll},
    })
    engine := newTestEngine(t, spec)

    bed := worldOf(500, 500, -3738)
    deck := worldOf(500, 500, -2992)
    land := worldOf(100, 100, shoreLand)
    require.True(t, engine.OverWater(bed.X, bed.Y, -3738),
        "a character floating above the bed stands over water")
    require.False(t, engine.OverWater(deck.X, deck.Y, -2992),
        "a character on the deck is ashore")
    require.False(t, engine.OverWater(land.X, land.Y, shoreLand),
        "the flat dry land is never water")
}

// TestElvenLakeStandsOverWater replays the reported stuck case
// against the real geodata pack: the character swam into the elven
// village lake and stood below the plateau cliff at 47136 46564
// -3738. The over water query separates the lake position from the
// village deck position, and the town route from the hunting grounds
// to the trader Ariel answers found - the priced swim crosses cost
// what they cost, the walk exists.
func TestElvenLakeStandsOverWater(t *testing.T) {
    engine := townTestEngine(t)

    stuck := Vec3{X: 47136, Y: 46564, Z: -3738}

    // The stuck position stands over the lake bed.
    require.True(t, engine.OverWater(stuck.X, stuck.Y, int16(stuck.Z)),
        "the reported stuck position is over the elven lake")
    // The village plateau does not.
    require.False(t, engine.OverWater(45480, 46680, -2992),
        "the village deck is dry land")

    // The town trip from the hunting spot to the trader Ariel prices
    // every crossing at the swim rate: the lake segments the plan carries
    // (if any) are the faster walk, the rest stays ashore.
    spot := Vec3{X: 53504, Y: 45249, Z: -3520}
    ariel := Vec3{X: 44683, Y: 46952, Z: -2981}
    route, err := engine.FindPathApproach(
        spot, ariel, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, route.Found)
}
