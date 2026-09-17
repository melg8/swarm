// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// The vertex corner height tests: the rectangle corners read the
// sheet's own vertex field (the average of the sheet's cells around
// the grid vertex), so the adjacent rectangles of one sheet read the
// same height at their shared edge and the surfaces join seamlessly -
// the roof sheet seams of the inside-cell corners are gone - while
// different sheets keep their own levels and a genuine cliff or deck
// edge stays sharp.

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

// edgeProfile returns the surface height of the polygon edge at the
// given crossing coordinate: the linear interpolation of the two
// corner heights of that edge (the same profile the geometry
// encoder's wall filler compares).
func edgeProfile(h0, h1 int16, t0, t1, v int32) float64 {
    if t1 <= t0 {
        return float64(h0)
    }

    return float64(h0) + float64(h1-h0)*float64(v-t0)/float64(t1-t0)
}

// TestVertexCornersJoinSameSheetSurfaces pins the seam free join: on
// the linear slope every vertical and horizontal adjacency of the
// tile agrees about the shared edge height at the overlap ends
// within the subpixel skip of the wall filler - the inside corner
// scheme disagreed by the full 8 unit quantization step on every
// slope adjacency.
func TestVertexCornersJoinSameSheetSurfaces(t *testing.T) {
    tile := slopeWorld(t).Tile
    worst := 0.0
    for i := range tile.Polys {
        a := &tile.Polys[i]
        for j := range tile.Polys {
            b := &tile.Polys[j]
            if a.X1 == b.X0 {
                // Vertical shared edge: the overlap in y.
                lo := max(a.Y0, b.Y0)
                hi := min(a.Y1, b.Y1)
                if lo >= hi {
                    continue
                }
                da := edgeProfile(a.H10, a.H11, a.Y0, a.Y1, lo) -
                    edgeProfile(b.H00, b.H01, b.Y0, b.Y1, lo)
                db := edgeProfile(a.H10, a.H11, a.Y0, a.Y1, hi) -
                    edgeProfile(b.H00, b.H01, b.Y0, b.Y1, hi)
                worst = math.Max(worst, math.Max(math.Abs(da),
                    math.Abs(db)))
            }
            if a.Y1 == b.Y0 {
                lo := max(a.X0, b.X0)
                hi := min(a.X1, b.X1)
                if lo >= hi {
                    continue
                }
                da := edgeProfile(a.H01, a.H11, a.X0, a.X1, lo) -
                    edgeProfile(b.H00, b.H10, b.X0, b.X1, lo)
                db := edgeProfile(a.H01, a.H11, a.X0, a.X1, hi) -
                    edgeProfile(b.H00, b.H10, b.X0, b.X1, hi)
                worst = math.Max(worst, math.Max(math.Abs(da),
                    math.Abs(db)))
            }
        }
    }
    require.LessOrEqual(t, worst, 0.5,
        "the same sheet surfaces must join without a visible seam")
}

// cliffWorld builds a shore region: a dry plateau at -3504 west of
// x 10, the water bed at -3784 east of it - two sheets with a sharp
// 280 unit step between them (both touch the region border, so both
// survive the island filter).
func cliffWorld(t *testing.T) *RegionBuild {
    t.Helper()
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 10 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }

        return []layerSpec{{h: -3784, nswe: 0x0F}}
    })
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)

    return build
}

// TestVertexCornersKeepCliffsSharp pins the sheet separation: the
// vertex field of a sheet averages only its own cells, so the plateau
// surface stays exactly at its level up to the shore edge and the
// water bed at its own - the step between the two sheets keeps its
// full 280 units instead of melting into a slope.
func TestVertexCornersKeepCliffsSharp(t *testing.T) {
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
