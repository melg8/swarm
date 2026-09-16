// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// TestRealRegionFloatingIslands pins the floating component drop on
// the real elven region: the geodata encodes the giant trees of the
// elven forest as stacked canopy layers a character can never reach
// (no within-climb step leads up there from anywhere), and the sheet
// component filter must reject them - while the walkable world (the
// lake shore, the village decks, the bridge over the lake, the water
// itself) survives untouched. The defect round of the viewer showed
// what keeping them renders: the mother tree fused into the ground
// through its canopy polygons and the floating village curtained
// down to the lake through the height step walls.
func TestRealRegionFloatingIslands(t *testing.T) {
    build := buildRealRegion(t, 21, 19)
    tile := build.Tile

    // The floating drop is substantial: the elven forest canopy
    // population (the measured round: 1377 sheets, 60118 layers).
    require.Greater(t, build.Stats.IslandSheets, 1000,
        "the canopy population of 21_19 must register as floating")
    require.Greater(t, build.Stats.IslandLayers, 50000,
        "the canopy layers of 21_19 must register as floating")

    // The mother tree block around local cells (731..869, 470..603)
    // holds its branches at -2544..-1344 (the sheet 83 of the
    // measured round): no polygon may remain up there.
    canopy := 0
    for i := range tile.Polys {
        p := &tile.Polys[i]
        if p.X0 >= 731 && p.X1 <= 870 && p.Y0 >= 470 && p.Y1 <= 604 &&
            p.H00 > -2700 {
            canopy++
        }
    }
    require.Zero(t, canopy,
        "the mother tree canopy must not produce polygons")

    // The walkable world survives: the bridge deck cell (565, 1214)
    // keeps its -3040 polygon, the village deck cell (858, 983) keeps
    // its -3056 polygon.
    require.True(t, cellHoldsPolyNear(tile, 565, 1214, -3040, 40),
        "the bridge deck polygon must survive the island filter")
    require.True(t, cellHoldsPolyNear(tile, 858, 983, -3056, 40),
        "the village deck polygon must survive the island filter")
}

// cellHoldsPolyNear reports whether one cell is covered by a polygon
// whose corners sit within the delta of the height.
func cellHoldsPolyNear(tile *navmesh.Tile, cx, cy int32, h int16,
    delta int32,
) bool {
    for i := range tile.Polys {
        p := &tile.Polys[i]
        if cx < p.X0 || cx >= p.X1 || cy < p.Y0 || cy >= p.Y1 {
            continue
        }
        for _, corner := range []int16{p.H00, p.H10, p.H01, p.H11} {
            if abs16(corner-h) <= delta {
                return true
            }
        }
    }

    return false
}
