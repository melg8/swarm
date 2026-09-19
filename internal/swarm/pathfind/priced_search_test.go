package pathfind

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The priced water round of 2026-09-19 (the owner directive): the
// walled form of the water was the outdated way - the bot plans
// through the water objects too, with the correct slowdowns priced
// (swimming is slower than running, the run/swim speed ratio 2.3).
// The pins here hold the two sides of the pricing: a wide band is
// cheaper to swim than to walk around, a narrow one is cheaper to
// detour - the search answers the genuinely faster walk of each
// world. The water bodies are whole flat blocks (the per cell setCell
// would raise the untouched cells of every touched block to height 0
// - the pillars would wall the water off and the pricing would never
// decide).

// waterBand writes a rectangle of whole flat water blocks over the
// flat land: block columns bx0..bx1, block rows by0..by1.
func waterBand(spec *regionSpec, bx0, bx1, by0, by1 int) {
    waterBlock := blockSpec{
        kind:  blockFlat,
        cells: [cellsPerBlock]Layer{{Height: shoreBed, NSWE: nsweAll}},
    }
    for bx := bx0; bx <= bx1; bx++ {
        for by := by0; by <= by1; by++ {
            spec.blocks[bx][by] = waterBlock
        }
    }
}

// TestPricedApproachSwimsAWideBand pins the swim side: a 48 cell wide
// water band 448 cells tall splits two dry points, the land detour
// around its far edge costs more than the swim at the 2.3x rate, and
// the priced search crosses the water - the plan may swim, the
// slowdown priced.
func TestPricedApproachSwimsAWideBand(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    // The band: block columns 38..43 (cells 304..351), block rows
    // 60..115 (cells 480..927).
    waterBand(spec, 38, 43, 60, 115)
    engine := newTestEngine(t, spec)

    start := worldOf(200, 700, shoreLand)
    end := worldOf(600, 700, shoreLand)
    result, err := engine.FindPathApproach(
        start, end, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, result.Found, "the priced search answers the band")
    // The route crosses the water: the straight chord through the
    // band beats the land detour at the 2.3x rate (the smoothing
    // folds the swim into one leg whose endpoints stand dry, so the
    // crossing shows on the leg raster, not the waypoint heights).
    wetLegs := 0
    for i := 1; i < len(result.Waypoints); i++ {
        crossed, err := engine.WaterCrossed(
            result.Waypoints[i-1], result.Waypoints[i])
        require.NoError(t, err)
        if crossed {
            wetLegs++
        }
    }
    require.Positive(t, wetLegs,
        "the swim across the band beats the land detour at 2.3x")
}

// TestPricedApproachDetoursANarrowBand pins the land side: a band
// only 8 cells tall is cheaper to walk around than to swim across
// (the 2.3x rate more than doubles the 48 cell crossing), so the
// priced search detours it and every waypoint and every leg of the
// answer stays dry - the slowdown keeps the bot ashore whenever
// running is the faster walk.
func TestPricedApproachDetoursANarrowBand(t *testing.T) {
    spec := &regionSpec{}
    spec.setFlat(shoreLand)
    // The narrow band: block columns 38..43 (cells 304..351), block
    // row 88 (cells 704..711). The endpoints sit inside the band's
    // y range, so the straight chord between them crosses it.
    waterBand(spec, 38, 43, 88, 88)
    engine := newTestEngine(t, spec)

    start := worldOf(200, 707, shoreLand)
    end := worldOf(600, 707, shoreLand)
    result, err := engine.FindPathApproach(
        start, end, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, result.Found, "the detour around the band exists")
    require.GreaterOrEqual(t, len(result.Waypoints), 3,
        "the route must keep the shore pivots the search paid for")
    for i, wp := range result.Waypoints {
        require.GreaterOrEqual(t, wp.Z, float64(WaterLevel),
            "waypoint %d must stay dry", i)
    }
    // Every leg is a clean dry walk: the follower clicks straight
    // along each leg without entering the water.
    for i := 1; i < len(result.Waypoints); i++ {
        crossed, err := engine.WaterCrossed(
            result.Waypoints[i-1], result.Waypoints[i])
        require.NoError(t, err)
        require.False(t, crossed, "the leg %d must stay dry", i-1)
    }
}

// TestDelevelShoreRoutePricesTheBay replays the reported scene
// against the real geodata pack: the character stood at the elven
// village shore (40648 43432, the state dump of the hang) and the
// deleveling planned its walk to the guard Starden. The walled form
// of the water once burned its re-path budget on routes the click
// guard refused; the priced search answers the honest walk instead -
// the bay crossing priced at the swim rate competes with the shore
// walk, and whatever the answer is, the follower walks it (the escape
// owns the off-plan swims).
func TestDelevelShoreRoutePricesTheBay(t *testing.T) {
    engine := townTestEngine(t)

    shore := Vec3{X: 40648, Y: 43432, Z: -3624}
    starden := Vec3{X: 42971, Y: 51372, Z: -2992}

    route, err := engine.FindPathApproach(
        shore, starden, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, route.Found,
        "the priced search answers the honest walk: the bay crossing "+
            "priced at the swim rate competes with the shore walk")
    require.NotEmpty(t, route.Waypoints)
}
