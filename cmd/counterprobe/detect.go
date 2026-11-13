// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "fmt"
    "math"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The counter direction detector: along each direction the first cell holding a
// floor at the spawn height is the stall edge, while the wall behind the back
// stays roof cells over the whole scan window. The edge agreeing with the spawn
// heading wins - a wall has no edge and a wrong side loses the agreement. The
// customer cell lands one cell beyond the edge (docs/hunting.md).

// detectCounter prints the per direction edge profile of every merchant, the
// detected counter direction and the stand cell.
func detectCounter(regionOf func(float64, float64) *pathfind.Region) {
    fmt.Println("\n=== counter direction detection")
    for _, m := range merchants {
        region := regionOf(m.x, m.y)
        angle := m.heading / 65536 * 2 * math.Pi
        fmt.Printf("%-9s heading %.1f deg:\n", m.name,
            m.heading/65536*360)
        best := ""
        bestAgree := 361.0
        for _, d := range dirs {
            edge := 0
            for n := 1; n <= 8; n++ {
                x := m.x + d.dx*float64(n)*16
                y := m.y + d.dy*float64(n)*16
                if hasFloor(region, x, y, int(m.z)) {
                    edge = n

                    break
                }
            }
            dirAngle := math.Atan2(d.dy, d.dx)
            agree := math.Abs(
                math.Remainder(dirAngle-angle, 2*math.Pi))
            verdict := "wall (no edge in the window)"
            if edge > 0 {
                verdict = fmt.Sprintf("edge at %d cells (%.0f units)",
                    edge, float64(edge)*16)
                if agree <= math.Pi/3 && agree < bestAgree {
                    bestAgree = agree
                    best = d.name
                }
            }
            fmt.Printf("  %-2s: %s\n", d.name, verdict)
        }
        if best == "" {
            fmt.Printf("  no counter edge detected\n")

            continue
        }
        fmt.Printf("  counter direction %s (heading agreement "+
            "%.0f deg), stand one cell beyond the edge\n",
            best, bestAgree/math.Pi*180)
    }
}

// hasFloor reports whether the cell under the world position holds a
// floor layer at the reference height (within +-96).
func hasFloor(
    region *pathfind.Region, x, y float64, refZ int,
) bool {
    buf := make([]pathfind.Layer, 0, 8)
    stack := region.LayerStack(localCellOf(x, y), buf)
    for _, l := range stack {
        d := int(l.Height) - refZ
        if d >= -96 && d <= 96 {
            return true
        }
    }

    return false
}

// localCellOf resolves the region local cell of a world position.
func localCellOf(x, y float64) pathfind.Point {
    return pathfind.LocalCell(pathfind.WorldToCell(x, y))
}
