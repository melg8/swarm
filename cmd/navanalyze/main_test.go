// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// The smoke pass of issue #9: the z fight counter of the tile
// analyzer - two stacked surfaces whose mid heights sit within 8
// units over a shared cell are the defect it hunts.

func tileOf(polys ...navmesh.Poly) *navmesh.Tile {
    return &navmesh.Tile{Col: 24, Row: 19, Polys: polys}
}

func flatPoly(x0, y0, x1, y1 int32, h int16) navmesh.Poly {
    return navmesh.Poly{
        X0: x0, Y0: y0, X1: x1, Y1: y1,
        H00: h, H10: h, H01: h, H11: h,
    }
}

func TestCountZFight(t *testing.T) {
    t.Run("an empty tile counts nothing", func(t *testing.T) {
        require.Zero(t, countZFight(tileOf()))
    })

    t.Run("a single surface never fights", func(t *testing.T) {
        tile := tileOf(flatPoly(0, 0, 8, 8, -3200))
        require.Zero(t, countZFight(tile))
    })

    t.Run("two surfaces 4 units apart fight over the shared cell", func(t *testing.T) {
        tile := tileOf(
            flatPoly(0, 0, 1, 1, -3200),
            flatPoly(0, 0, 1, 1, -3196),
        )
        // The mid heights sit 4 units apart over one shared cell:
        // one fighting pair.
        require.Equal(t, 1, countZFight(tile))
    })

    t.Run("two surfaces 100 units apart stay quiet", func(t *testing.T) {
        tile := tileOf(
            flatPoly(0, 0, 1, 1, -3200),
            flatPoly(0, 0, 1, 1, -3100),
        )
        require.Zero(t, countZFight(tile))
    })

    t.Run("disjoint cells never meet", func(t *testing.T) {
        tile := tileOf(
            flatPoly(0, 0, 8, 8, -3200),
            flatPoly(8, 8, 16, 16, -3200),
        )
        require.Zero(t, countZFight(tile))
    })

    t.Run("a wide overlap multiplies the pairs", func(t *testing.T) {
        // Two surfaces over four shared cells: four fighting pairs.
        tile := tileOf(
            flatPoly(0, 0, 2, 2, -3200),
            flatPoly(0, 0, 2, 2, -3198),
        )
        require.Equal(t, 4, countZFight(tile))
    })

    t.Run("the corner heights feed the mid height", func(t *testing.T) {
        // A tilted first surface (mids at -3200) against a flat
        // second at -3196 over one shared cell: still within 8,
        // still one fight.
        tilted := navmesh.Poly{
            X0: 0, Y0: 0, X1: 1, Y1: 1,
            H00: -3204, H10: -3204, H01: -3196, H11: -3196,
        }
        tile := tileOf(tilted, flatPoly(0, 0, 1, 1, -3196))
        require.Equal(t, 1, countZFight(tile))
    })
}
