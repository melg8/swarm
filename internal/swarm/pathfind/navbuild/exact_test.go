// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The exact rectangle height tests: the rectangle polygons merge only
// the cells of one exact geodata height, so the detour mesh represents
// the raw l2j squares - flat quads at the exact cell heights, the
// quantization staircase included. The bilinear vertex field of the
// previous rounds smoothed that staircase into interpolated surfaces;
// the owner's porting directive pinned the faithful representation
// instead (nothing between the mesh and the geodata numbers, the
// visual and the actual surface alike).

// slopeWorld builds a region whose ground descends 8 units per cell
// along x (the linear ramp the l2j quantization produces on smooth
// hills): every cell of the 40x40 patch sits at -1000 - 8*x.
func slopeWorld(t *testing.T) *RegionBuild {
    t.Helper()
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 40 && cy < 40 {
            return []layerSpec{{h: int16(-1000 - 8*cx), nswe: 0x0F}}
        }

        return nil
    })
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)

    return build
}

// TestRectPolyExactFlatSquares pins the faithful square port on the
// linear slope: every polygon is flat (all four corners carry one
// height), the height is the exact geodata value of every covered
// column and the slope decomposes into the per-height strips - one
// maximal rectangle per height run - whose neighbors differ by exactly
// the 8 unit quantization step of the raw squares.
func TestRectPolyExactFlatSquares(t *testing.T) {
    tile := slopeWorld(t).Tile
    require.NotEmpty(t, tile.Polys)
    for i := range tile.Polys {
        p := &tile.Polys[i]
        require.Equal(t, p.H00, p.H10, "poly %d corners disagree", i)
        require.Equal(t, p.H00, p.H01, "poly %d corners disagree", i)
        require.Equal(t, p.H00, p.H11, "poly %d corners disagree", i)
        for x := p.X0; x < p.X1; x++ {
            require.Equal(t, int16(-1000-8*x), p.H00,
                "poly %d column %d", i, x)
        }
        for y := p.Y0; y < p.Y1; y++ {
            require.Equal(t, int16(-1000-8*p.X0), p.H00,
                "poly %d row %d", i, y)
        }
    }
    // One maximal rectangle per column height: 40 distinct heights,
    // none of them adjacent, so the decomposition cannot merge strips.
    require.Len(t, tile.Polys, 40)
}

// cliffWorld builds a shore region: a dry plateau at -3504 west of
// x 10, the water bed at -3784 east of it - two sheets with a sharp
// 280 unit step between them (both touch the region border, so both
// survive the island filter).
func cliffWorld(t *testing.T) *RegionBuild {
    t.Helper()
    data := writeRegionFile(t, func(cx, _ int) []layerSpec {
        if cx < 10 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }

        return []layerSpec{{h: -3784, nswe: 0x0F}}
    })
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)

    return build
}

// TestRectPolyKeepCliffsSharp pins the sheet separation: the plateau
// surface stays exactly at its level up to the shore edge and the
// water bed at its own - the step between the two sheets keeps its
// full 280 units instead of melting into a slope.
func TestRectPolyKeepCliffsSharp(t *testing.T) {
    tile := cliffWorld(t).Tile
    require.NotEmpty(t, tile.Polys)
    for i := range tile.Polys {
        p := &tile.Polys[i]
        if p.X0 < 10 {
            // The plateau: flat at -3504 everywhere, up to the edge.
            require.EqualValues(t, -3504, p.H00, "poly %d", i)
            require.EqualValues(t, -3504, p.H10, "poly %d", i)
            require.EqualValues(t, -3504, p.H01, "poly %d", i)
            require.EqualValues(t, -3504, p.H11, "poly %d", i)
        } else {
            // The water bed: flat at -3784.
            require.EqualValues(t, -3784, p.H00, "poly %d", i)
            require.EqualValues(t, -3784, p.H10, "poly %d", i)
            require.EqualValues(t, -3784, p.H01, "poly %d", i)
            require.EqualValues(t, -3784, p.H11, "poly %d", i)
        }
    }
}
