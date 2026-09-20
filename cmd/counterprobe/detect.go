// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "fmt"
    "math"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The counter direction detector. The question the owner asked (the
// shop quarter round): how to detect the counter direction of a
// merchant and tell it apart from the wall behind the merchant's
// back. The answer this probe implements works on the raw geodata
// pack and needs no client:
//
//   - The merchant spawn of a counter trader sits inside the roofed
//     stall (the pack models the stall interior as roof-only cells -
//     the merchant floor the pack never walks). The stall edge in a
//     direction shows as the first cell whose layer stack holds a
//     floor at the spawn z; the wall shows as roof cells continuing
//     for the whole scan window.
//   - The counter sits on the FACING side of the merchant (the
//     merchant looks across the counter at the customers): among the
//     directions that hold a stall edge within the window, the one
//     agreeing with the spawn heading wins. A wall behind the back
//     has no edge at all - it cannot win the vote; an edge on the
//     wrong side loses the heading agreement.
//
// The customer cell lands one cell beyond the detected edge, which
// the mesh verification (mode stands / scan) then pins exactly.

// detectCounter prints the per direction edge profile of every
// merchant, the detected counter direction and the derived stand
// cell, plus the agreement verdict against the spawn heading.
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
