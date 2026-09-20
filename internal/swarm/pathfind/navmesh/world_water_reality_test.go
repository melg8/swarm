// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/stretchr/testify/require"
)

// The mesh water reality tests (the owner directive round): the hunt
// water guard subsystems (the shore escape machinery, the wet click
// refusals, the wet claim guards of the cursor key escape) were built
// for the grid era whose search walled the water. The mesh prices the
// water instead (the swim rate of the C1 zone data), and the owner
// belief this round puts under tests before the removal: the mesh
// search answers correct routes from ANY standing cell - a wet one
// included - so the escape machinery has no job left.
//
// The scene is the real one: the 2026-09-20 town trip dump (build
// 6b61f5b, bot test3) walked the sell trip to the trader Unoren,
// drifted over the elven village lake bed and the water escape burned
// the trip into "aborted, the water escape could not leave the water"
// after four shore attempts from the wet cells below. The tests run
// against the real elven region pack (the 21_19 tile and its
// neighbours) and the real geodata the grid raster serves.

// The world positions of the dump scene.
var (
    // dumpFarmSpot is the walk plan origin of the dump trip.
    dumpFarmSpot = Pos{X: 37520, Y: 47877, Z: -3599}
    // dumpTrader is the sell trip destination (the trader Unoren).
    dumpTrader = Pos{X: 44667, Y: 46896, Z: -2982}
    // dumpWetCells are the standing cells the character floated over
    // when the escape armed and re-armed (the lake bed south west of
    // the village deck).
    dumpWetCells = []Pos{
        {X: 39962, Y: 43851, Z: -3800},
        {X: 39936, Y: 43904, Z: -3735},
        {X: 39962, Y: 43904, Z: -3800},
        {X: 39936, Y: 43851, Z: -3735},
    }
)

// waterRealityMesh loads the elven pack with the cache sized for the
// sandbox memory, "" when the pack is not built.
func waterRealityMesh(t *testing.T) *Mesh {
    t.Helper()
    dir := navmeshDataDir()
    if dir == "" {
        t.Skip("no local navmesh tiles, the dump reproduction needs them")
    }
    mesh := NewMesh(dir)
    mesh.SetCacheCapacity(2)
    mesh.SetAbstractCapacity(2)

    return mesh
}

// waterRealityEngine loads the grid engine over the geodata the tiles
// were built from (the water raster authority), nil without data.
func waterRealityEngine(t *testing.T) *pathfind.Engine {
    t.Helper()
    engine := pathfind.NewEngine("../../../../data/geodata")
    if !engine.Stats().HasData {
        t.Log("no geodata next to the tiles, the raster checks skip")

        return nil
    }

    return engine
}

// overWaterAt answers the grid water raster for the position, false
// without an engine (the callers skip the dry assertions then).
func overWaterAt(engine *pathfind.Engine, pos Pos) bool {
    if engine == nil {
        return false
    }

    return engine.OverWater(pos.X, pos.Y, int16(pos.Z))
}

// pricedFilter is the production water pricing of the live navigator
// (the C1 zone cuboids arm the swim rate; the clearance shaping is a
// pivot concern this round does not test).
func pricedFilter() Filter {
    filter := DefaultFilter()
    filter.WaterZones = C1WaterZones()

    return filter
}

// TestMeshPlansFromWetStandingCells pins the core of the owner belief:
// from every wet cell the dump recorded, the mesh RouteApproach plans
// a full route to the trader - the search needs no shore escape
// pre-pass, the cheapest corridor swims out of the lake at the priced
// rate whenever swimming wins and walks the shore whenever it does
// not. The route must end on dry ground within the trip approach
// radius of the trader (the destination stands on land).
func TestMeshPlansFromWetStandingCells(t *testing.T) {
    mesh := waterRealityMesh(t)
    engine := waterRealityEngine(t)

    for i, start := range dumpWetCells {
        require.True(t, overWaterAt(engine, start),
            "wet cell %d must sit over a lake bed in the raster, "+
                "the scene drifted", i)
        route, err := mesh.RouteApproach(start, dumpTrader, 200,
            pricedFilter())
        require.NoError(t, err)
        require.NotNil(t, route)
        require.True(t, route.Found,
            "wet cell %d: the mesh plans the trader route from the "+
                "lake bed", i)
        require.False(t, route.Partial,
            "wet cell %d: the trader is reachable, the answer is not "+
                "a closest corridor", i)
        require.False(t, route.PocketEscape,
            "wet cell %d: the lake bed is connected mesh, not a "+
                "sealed pocket", i)
        require.NotEmpty(t, route.Waypoints)
        last := route.Waypoints[len(route.Waypoints)-1]
        dx, dy := last.X-dumpTrader.X, last.Y-dumpTrader.Y
        // The merchant stands behind the C1 counter row the mesh
        // never walks onto: the plan ends on the customer ground in
        // front of it, the server interaction distance (250, the
        // npcInteractionDist of the hunt loop) owns the rest.
        require.LessOrEqual(t, dx*dx+dy*dy, 250.0*250.0,
            "wet cell %d: the plan ends within the interaction "+
                "distance of the trader", i)
        require.False(t, overWaterAt(engine, last),
            "wet cell %d: the plan ends on dry ground", i)
        wet := 0
        for _, wp := range route.Waypoints {
            if overWaterAt(engine, wp) {
                wet++
            }
        }
        t.Logf("wet cell %d (%.0f %.0f): %d waypoints, %d wet waypoints",
            i, start.X, start.Y, len(route.Waypoints), wet)
    }
}

// TestMeshPlansTheDumpTrip pins the whole trip answer: the plan the
// bot followed in the dump (the farm spot to the trader Unoren) comes
// out of the mesh as one found route - the walk the follower serves
// without any water pre-pass. The wet waypoints the plan carries are
// the priced crossings, logged for the round evidence.
func TestMeshPlansTheDumpTrip(t *testing.T) {
    mesh := waterRealityMesh(t)
    engine := waterRealityEngine(t)
    require.False(t, overWaterAt(engine, dumpFarmSpot),
        "the farm spot stands dry, the scene drifts")

    route, err := mesh.RouteApproach(dumpFarmSpot, dumpTrader, 200,
        pricedFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.True(t, route.Found, "the mesh plans the whole dump trip")
    require.False(t, route.Partial)
    require.False(t, route.PocketEscape)
    last := route.Waypoints[len(route.Waypoints)-1]
    dx, dy := last.X-dumpTrader.X, last.Y-dumpTrader.Y
    t.Logf("the dump trip endpoint: %.0f %.0f %.0f (%.1f units from "+
        "the trader), %d waypoints", last.X, last.Y, last.Z,
        math.Sqrt(dx*dx+dy*dy), len(route.Waypoints))
    // The counter row: the plan ends on the customer ground within
    // the server interaction distance (250), not on the NPC itself.
    require.LessOrEqual(t, dx*dx+dy*dy, 250.0*250.0,
        "the plan ends within the interaction distance")
    wet := 0
    for _, wp := range route.Waypoints {
        if overWaterAt(engine, wp) {
            wet++
        }
    }
    t.Logf("the dump trip: %d waypoints, %d wet waypoints",
        len(route.Waypoints), wet)
}
