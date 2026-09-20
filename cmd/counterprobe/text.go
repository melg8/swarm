// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// dumpText prints the precise cell classification grid of the shop
// quarter with the merchant and plan-end markers.
func dumpText(
    region *pathfind.Region, center merchant, list []merchant,
    planEnds map[string]pathfind.Vec3,
) {
    fmt.Printf("\n=== text map at %.0f %.0f ref z %.0f\n",
        center.x, center.y, center.z)
    buf := make([]pathfind.Layer, 0, 8)
    c := pathfind.WorldToCell(center.x, center.y)
    const radius = 19
    for row := -radius; row <= radius; row++ {
        fmt.Printf("  y=%6.0f ", center.y+float64(row)*16)
        for col := -radius; col <= radius; col++ {
            cell := pathfind.Point{
                X: c.X + int32(col),
                Y: c.Y + int32(row),
            }
            stack := region.LayerStack(pathfind.LocalCell(cell), buf)
            symbol := classify(stack, int(center.z))
            wx := center.x + float64(col)*16
            wy := center.y + float64(row)*16
            for _, m := range list {
                if near(m.x, wx) && near(m.y, wy) {
                    symbol = "U"
                    if m.name != unorenName {
                        symbol = "A"
                    }
                }
                if end, ok := planEnds[m.name]; ok &&
                    near(end.X, wx) && near(end.Y, wy) {
                    symbol = "p"
                }
            }
            fmt.Print(symbol)
        }
        fmt.Println()
    }
    fmt.Println("  (x from", center.x-radius*16, "to",
        center.x+radius*16, "step 16; U/A merchant, p plan end)")
}

// near reports whether the world coordinate sits inside the cell the
// grid point covers (the grid points are the cell centers).
func near(a, b float64) bool {
    d := a - b
    if d < 0 {
        d = -d
    }

    return d <= 8
}
