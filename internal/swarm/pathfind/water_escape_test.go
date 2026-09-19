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
// the C1 server covers everything below -3780). The defenses of this
// round: the smoothing keeps the legs the search priced (a chord over
// water between dry points never folds), the escape search finds the
// nearest shore for a position standing in a lake, and the plans
// price every crossing at the swim rate - the water is walkable, the
// slowdown honest.

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
// - every leg of the smoothed route stays dry and walkable.
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
    // Every smoothed leg is a clean dry walk: the follower may click
    // straight along each leg without entering the water.
    for i := 1; i < len(result.Waypoints); i++ {
        crossed, err := engine.WaterCrossed(
            result.Waypoints[i-1], result.Waypoints[i])
        require.NoError(t, err)
        require.False(t, crossed, "the leg %d must stay dry", i-1)
    }
}

// TestFindWaterEscape verifies the escape search on a lake with one
// gradual ramp: the flood finds the nearest dry cell through the
// ramp and the smoothed escape path ends above the water level.
func TestFindWaterEscape(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    // The lake bed, walled off from the land by the 80 unit shore
    // cliff everywhere except the east ramp.
    for x := 200; x <= 600; x++ {
        for y := 200; y <= 600; y++ {
            spec.setCell(x, y, Layer{Height: -3850, NSWE: nsweAll})
        }
    }
    // The east ramp: three cells climbing 32 units each onto the land.
    for i, height := range []int16{-3818, -3786, -3754} {
        spec.setCell(601+i, 400, Layer{Height: height, NSWE: nsweAll})
    }
    engine := newTestEngine(t, spec)

    result, err := engine.FindWaterEscape(
        worldOf(400, 400, -3850))
    require.NoError(t, err)
    require.True(t, result.Found, "the ramp connects the lake to the land")
    require.NotEmpty(t, result.Waypoints)
    last := result.Waypoints[len(result.Waypoints)-1]
    require.GreaterOrEqual(t, last.Z, float64(WaterLevel),
        "the escape must end on dry ground")
    // The raw path walks the ramp gradually: every step stays within
    // the passable height (the escape shares the canStep rules).
    for i := 1; i < len(result.RawPath); i++ {
        delta := result.RawPath[i].Z - result.RawPath[i-1].Z
        require.LessOrEqual(t, delta, float64(DefaultMaxPassableHeight),
            "the escape step %d must stay climbable", i)
        require.GreaterOrEqual(t, delta, -float64(DefaultMaxPassableHeight),
            "the escape step %d must stay droppable", i)
    }
}

// TestFindWaterEscapeSealedLake verifies the escape on a lake whose
// whole shore is a cliff: no walkable connection exists, the flood
// exhausts the bed and the search reports no escape.
func TestFindWaterEscapeSealedLake(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    for x := 200; x <= 600; x++ {
        for y := 200; y <= 600; y++ {
            spec.setCell(x, y, Layer{Height: -3850, NSWE: nsweAll})
        }
    }
    engine := newTestEngine(t, spec)

    result, err := engine.FindWaterEscape(worldOf(400, 400, -3850))
    require.NoError(t, err)
    require.False(t, result.Found,
        "the cliff shore leaves no walkable escape")
}

// TestFindWaterEscapeDryStart documents the dry start contract: a
// position standing above the water level needs no escape and the
// search answers found=false without an error.
func TestFindWaterEscapeDryStart(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    engine := newTestEngine(t, spec)

    result, err := engine.FindWaterEscape(worldOf(400, 400, shoreLand))
    require.NoError(t, err)
    require.False(t, result.Found)
}

// TestWaterCrossedSplitsWaterFromHeightSteps pins the water-only
// raster of the engine: a line across a real channel trips it, a line
// that merely climbs a tall dry step (the village deck ramps) does
// not - terrain is not water, whatever the walkability of the line.
func TestWaterCrossedSplitsWaterFromHeightSteps(t *testing.T) {
    // The channel engine of the smoothing test: water between the
    // dry shores.
    channel := &regionSpec{}
    channel.setFlat(shoreLand)
    for x := 300; x <= 500; x++ {
        for y := 500; y <= 900; y++ {
            channel.setCell(x, y, Layer{Height: shoreBed, NSWE: nsweAll})
        }
    }
    channelEngine := newTestEngine(t, channel)

    across := worldOf(200, 700, shoreLand)
    beyond := worldOf(600, 700, shoreLand)
    crossed, err := channelEngine.WaterCrossed(across, beyond)
    require.NoError(t, err)
    require.True(t, crossed,
        "the line across the channel crosses water")

    // The ramp engine: a dry step too tall for the line of sight
    // (the deck ramps of the elven village, ~190 units against the
    // passable 30), no water anywhere.
    ramp := &regionSpec{}
    ramp.setFlat(shoreLand)
    for x := 400; x <= 600; x++ {
        for y := 500; y <= 900; y++ {
            ramp.setCell(x, y, Layer{Height: shoreLand + 190, NSWE: nsweAll})
        }
    }
    rampEngine := newTestEngine(t, ramp)

    below := worldOf(200, 700, shoreLand)
    above := worldOf(700, 700, shoreLand+190)
    crossed, err = rampEngine.WaterCrossed(below, above)
    require.NoError(t, err)
    require.False(t, crossed,
        "the tall dry step is terrain, not water")
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

// TestElvenLakeStuckEscape replays the reported stuck case against
// the real geodata pack: the character swam into the elven village
// lake and stood below the plateau cliff at 47136 46564 -3738. The
// escape search must find the nearest shore from there, the town
// route from the hunting grounds must stay dry leg by leg, and the
// over water query must separate the lake position from the village
// deck position.
func TestElvenLakeStuckEscape(t *testing.T) {
    engine := townTestEngine(t)

    stuck := Vec3{X: 47136, Y: 46564, Z: -3738}

    // The stuck position stands over the lake bed.
    require.True(t, engine.OverWater(stuck.X, stuck.Y, int16(stuck.Z)),
        "the reported stuck position is over the elven lake")
    // The village plateau does not.
    require.False(t, engine.OverWater(45480, 46680, -2992),
        "the village deck is dry land")

    // The escape finds a shore.
    escape, err := engine.FindWaterEscape(stuck)
    require.NoError(t, err)
    require.True(t, escape.Found, "the elven lake has walkable shores")
    require.NotEmpty(t, escape.Waypoints)
    last := escape.Waypoints[len(escape.Waypoints)-1]
    require.GreaterOrEqual(t, last.Z, float64(WaterLevel),
        "the escape ends on dry ground, not in the lake")
    require.False(t, engine.OverWater(last.X, last.Y, int16(last.Z)),
        "the escape target itself is ashore")

    // The town trip from the hunting spot to the trader Ariel prices
    // every crossing at the swim rate: the lake legs the plan carries
    // (if any) are the faster walk, the rest stays ashore.
    spot := Vec3{X: 53504, Y: 45249, Z: -3520}
    ariel := Vec3{X: 44683, Y: 46952, Z: -2981}
    route, err := engine.FindPathApproach(
        spot, ariel, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, route.Found)
}
