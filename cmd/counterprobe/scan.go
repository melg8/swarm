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

// scanStandArea prints the route verdict for the cell grid around the
// derived stand cell (the distance and angle ladder), so the table
// entry can pick the cleanest reachable cell near the counter front.
func scanStandArea(engine *pathfind.Engine, mesh *navmesh.Mesh, name string) {
    var target *merchant
    for i := range merchants {
        if merchants[i].name == name {
            target = &merchants[i]
        }
    }
    if target == nil {
        return
    }
    m := *target
    angle := m.heading / 65536 * 2 * math.Pi
    fmt.Printf("\n=== scan %s (heading %.1f deg)\n", name,
        m.heading/65536*360)
    for _, dist := range []float64{56, 64, 72, 80, 88} {
        for _, da := range []float64{-45, -30, -15, 0, 15, 30, 45} {
            a := angle + da*math.Pi/180
            x := m.x + dist*math.Cos(a)
            y := m.y + dist*math.Sin(a)
            z, err := engine.ClosestHeight(x, y, int16(m.z))
            if err != nil {
                fmt.Printf("  d %.0f da %+.0f: no floor\n", dist, da)

                continue
            }
            line := fmt.Sprintf("  d %.0f da %+.0f cell %.0f %.0f z %d",
                dist, da, x, y, z)
            result, rerr := mesh.Route(
                navmesh.Pos{
                    X: approachOf(m).X, Y: approachOf(m).Y,
                    Z: approachOf(m).Z,
                },
                navmesh.Pos{X: x, Y: y, Z: float64(z)},
                navmesh.DefaultFilter())
            if rerr != nil || result == nil ||
                len(result.Waypoints) == 0 {
                fmt.Println(line, "no route")

                continue
            }
            last := result.Waypoints[len(result.Waypoints)-1]
            miss := math.Hypot(last.X-x, last.Y-y)
            fmt.Println(line, fmt.Sprintf(
                "miss %.0f found %v partial %v", miss,
                result.Found, result.Partial))
        }
    }
}
