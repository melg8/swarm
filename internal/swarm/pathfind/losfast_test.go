// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "math/rand"
    "testing"

    "github.com/stretchr/testify/require"
)

// The differential pin of the map free raster: the fast line of sight
// and the node graph oracle must answer every segment identically (the
// boolean and the error presence both) - the guard probes changed the
// carrier, never the contract.

// losWorld builds a region with the shapes the raster meets in the
// field: open flats, walled corridors, a terrace of height steps and
// a two layer deck.
func losWorld(t *testing.T) (*Engine, func(float64, float64) Vec3) {
    t.Helper()
    spec := &regionSpec{}
    spec.setFlat(0)
    // The corridor walls (the capsule world pattern).
    for x := 4; x < 40; x++ {
        spec.setCell(x, 12,
            Layer{Height: 0, NSWE: nsweAll &^ nsweSouth})
        spec.setCell(x, 13,
            Layer{Height: 0, NSWE: nsweAll &^ nsweNorth})
        spec.setCell(x, 16,
            Layer{Height: 0, NSWE: nsweAll &^ nsweSouth})
        spec.setCell(x, 17,
            Layer{Height: 0, NSWE: nsweAll &^ nsweNorth})
    }
    // The terrace: a height step cliff across the middle.
    for y := 20; y < 60; y++ {
        spec.setCell(32, y, Layer{Height: 0, NSWE: nsweAll &^ nsweEast})
        for x := 33; x < 64; x++ {
            spec.setCell(x, y,
                Layer{Height: 96, NSWE: nsweAll &^ nsweWest})
        }
    }
    // The two layer deck over the south east corner.
    for x := 80; x < 112; x++ {
        for y := 80; y < 112; y++ {
            spec.setCell(x, y,
                Layer{Height: 0, NSWE: nsweAll})
            spec.setCell(x, y,
                Layer{Height: 480, NSWE: nsweAll})
        }
    }
    dir := t.TempDir()
    spec.writeRegion(t, dir)
    engine := NewEngine(dir)

    return engine, func(cellX, cellY float64) Vec3 {
        baseX := float64((testRegionCol - tileZeroCol) * tileSize)
        baseY := float64((testRegionRow - tileZeroRow) * tileSize)

        return Vec3{
            X: baseX + cellX*cellSize,
            Y: baseY + cellY*cellSize,
            Z: 0,
        }
    }
}

// TestLineOfSightFastMatchesNodes walks deterministic pseudo random
// segments over the synthetic world and requires the fast raster and the
// node oracle to agree everywhere.
func TestLineOfSightFastMatchesNodes(t *testing.T) {
    engine, world := losWorld(t)
    //nolint:gosec // the fixed seed makes the segment walk deterministic:
    // the weak generator is the point, no secret is involved.
    rng := rand.New(rand.NewSource(20260918))
    checked := 0
    for range 3000 {
        ax := rng.Float64() * 2048
        ay := rng.Float64() * 2048
        bx := rng.Float64() * 2048
        by := rng.Float64() * 2048
        a := world(ax, ay)
        b := world(bx, by)

        fast, fastErr := engine.LineOfSight(a, b,
            DefaultMaxPassableHeight)
        oracle, oracleErr := engine.lineOfSightNodes(a, b,
            DefaultMaxPassableHeight)

        require.Equal(t, oracleErr != nil, fastErr != nil,
            "segment %.1f %.1f -> %.1f %.1f: the error presence differs"+
                " (fast %v, oracle %v)", ax, ay, bx, by, fastErr,
            oracleErr)
        require.Equal(t, oracle, fast,
            "segment %.1f %.1f -> %.1f %.1f: the answers differ",
            ax, ay, bx, by)
        checked++
    }
    require.Positive(t, checked)
}

// TestLineOfSightFastAxisSegments pins the degenerate steppings the
// random walk rarely produces: the vertical, the horizontal, the
// exact diagonal and the zero length segment.
func TestLineOfSightFastAxisSegments(t *testing.T) {
    engine, world := losWorld(t)
    segments := [][2][2]float64{
        {{100, 100}, {100, 190}},   // the vertical
        {{100, 100}, {190, 100}},   // the horizontal
        {{100, 100}, {190, 190}},   // the exact diagonal
        {{100, 100}, {100, 100}},   // the zero length
        {{10.5, 10.5}, {10.5, 14}}, // the short vertical
        {{10.5, 10.5}, {14, 10.5}}, // the short horizontal
    }
    for _, segment := range segments {
        a := world(segment[0][0], segment[0][1])
        b := world(segment[1][0], segment[1][1])
        fast, fastErr := engine.LineOfSight(a, b,
            DefaultMaxPassableHeight)
        oracle, oracleErr := engine.lineOfSightNodes(a, b,
            DefaultMaxPassableHeight)

        require.Equal(t, oracleErr != nil, fastErr != nil)
        require.Equal(t, oracle, fast)
    }
}
