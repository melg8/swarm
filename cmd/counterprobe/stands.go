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

// standCandidate is the derived customer cell of one merchant: the
// spawn pushed 64 units along the facing heading (the counter front
// direction) with the pack floor z resolved at that cell.
type standCandidate struct {
    name     string
    merchant merchant
    x, y     float64 // the derived cell
}

// standCandidates derives the customer cell of every merchant from
// the spawn heading (the counter sits on the facing side of the
// merchant, the customer cell one cell band beyond it).
func standCandidates() []standCandidate {
    out := make([]standCandidate, 0, len(merchants))
    for _, m := range merchants {
        angle := m.heading / 65536 * 2 * math.Pi
        x := m.x + 64*math.Cos(angle)
        y := m.y + 64*math.Sin(angle)
        out = append(out, standCandidate{
            name: m.name, merchant: m, x: x, y: y,
        })
    }

    return out
}

// verifyStands prints the pack floor height and the mesh route answer
// for every derived customer cell: the entry is table ready when the
// route ends at the cell (the walk machinery serves it as is).
func verifyStands(engine *pathfind.Engine, mesh *navmesh.Mesh) {
    fmt.Println("\n=== stand cell verification")
    for _, s := range standCandidates() {
        z, err := engine.ClosestHeight(s.x, s.y, int16(s.merchant.z))
        if err != nil {
            fmt.Printf("%-9s cell %.0f %.0f: no floor (%v)\n",
                s.name, s.x, s.y, err)

            continue
        }
        spawnDist := math.Hypot(s.x-s.merchant.x, s.y-s.merchant.y)
        result, rerr := mesh.Route(
            navmesh.Pos{
                X: approachOf(s.merchant).X,
                Y: approachOf(s.merchant).Y,
                Z: approachOf(s.merchant).Z,
            },
            navmesh.Pos{X: s.x, Y: s.y, Z: float64(z)},
            navmesh.DefaultFilter())
        verdict := "no route"
        if rerr == nil && result != nil && len(result.Waypoints) > 0 {
            last := result.Waypoints[len(result.Waypoints)-1]
            miss := math.Hypot(last.X-s.x, last.Y-s.y)
            verdict = fmt.Sprintf(
                "route end %.0f %.0f %.0f miss %.0f found %v",
                last.X, last.Y, last.Z, miss, result.Found)
        }
        fmt.Printf("%-9s cell %.0f %.0f z %d (spawn d2d %.0f): %s\n",
            s.name, s.x, s.y, z, spawnDist, verdict)
    }
}
