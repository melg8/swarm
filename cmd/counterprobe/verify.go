// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "fmt"
    "math"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// verifyGridStand routes the walk from the trip start to a candidate
// stand cell over the mesh and then verifies EVERY route leg with the
// grid line of sight (the NSWE faithful walk the simulated server and
// the real geodata enabled server apply). A leg the grid refuses is
// the frozen corridor class - the mesh links can carry a route the
// grid walls off (the counter front faces are exactly such walls), so
// the stand entry is table ready only when every leg verifies.
func verifyGridStand(
    engine *pathfind.Engine, mesh *navmesh.Mesh, m merchant,
    x, y float64,
) string {
    z, err := engine.ClosestHeight(x, y, int16(m.z))
    if err != nil {
        return fmt.Sprintf("no floor at the cell (%v)", err)
    }
    approach := approachOf(m)
    result, rerr := mesh.Route(
        navmesh.Pos{X: approach.X, Y: approach.Y, Z: approach.Z},
        navmesh.Pos{X: x, Y: y, Z: float64(z)},
        navmesh.DefaultFilter())
    if rerr != nil || result == nil || len(result.Waypoints) == 0 {
        return "no mesh route"
    }
    last := result.Waypoints[len(result.Waypoints)-1]
    miss := math.Hypot(last.X-x, last.Y-y)
    if miss > 16 {
        return fmt.Sprintf("route ends %.0f units short", miss)
    }
    bad := 0
    for i := 0; i+1 < len(result.Waypoints); i++ {
        a := result.Waypoints[i]
        b := result.Waypoints[i+1]
        ok, lerr := engine.LineOfSight(
            pathfind.Vec3{X: a.X, Y: a.Y, Z: a.Z},
            pathfind.Vec3{X: b.X, Y: b.Y, Z: b.Z},
            pathfind.DefaultMaxPassableHeight)
        if lerr != nil || !ok {
            bad++
            fmt.Printf("    leg %d (%.0f %.0f -> %.0f %.0f) "+
                "grid refuses\n", i, a.X, a.Y, b.X, b.Y)
        }
    }
    if bad > 0 {
        return fmt.Sprintf("%d of %d legs grid refused", bad,
            len(result.Waypoints)-1)
    }

    return fmt.Sprintf("OK: %d legs, all grid verified, "+
        "end %.0f %.0f, spawn d2d %.0f", len(result.Waypoints)-1,
        last.X, last.Y, math.Hypot(x-m.x, y-m.y))
}

// verifyStandsGrid runs the grid verified check over the candidate
// stand cells given on the command line style list per merchant name.
func verifyStandsGrid(
    engine *pathfind.Engine, mesh *navmesh.Mesh,
    candidates map[string][2]float64,
) {
    fmt.Println("\n=== grid verified stand check")
    for _, m := range merchants {
        c, ok := candidates[m.name]
        if !ok {
            continue
        }
        verdict := verifyGridStand(engine, mesh, m, c[0], c[1])
        fmt.Printf("%-9s stand %.0f %.0f: %s\n", m.name, c[0], c[1],
            verdict)
    }
}

// standVerifyList carries the candidate stand cells to verify (the
// table entries or the scan candidates under test).
var standVerifyList = map[string][2]float64{
    unorenName:   {44584, 46944},
    arielName:    {44584, 46952},
    creameesName: {42727, 50115},
    herbielName:  {42798, 50101},
    sabrinName:   {18072, 144488},
    caseyName:    {18044, 144560},
    soniaName:    {19320, 146168},
    laraName:     {19224, 146168},
}
