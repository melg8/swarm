// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestBuildRegionMergeToleranceExactDefault pins the default
// decomposition: at the zero merge tolerance the exact same-height
// rule stays in force, so the 32x32 checkerboard of two heights 8
// apart builds one 1x1 polygon per cell - the faithful square port
// the owner pinned (the merge tolerance of issue #57 must not move
// the shipped default).
func TestBuildRegionMergeToleranceExactDefault(t *testing.T) {
    data := checkerboardRegion(t, 32)
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)
    require.Len(t, build.Tile.Polys, 32*32,
        "the exact rule keeps one polygon per checkerboard cell")
    for i := range build.Tile.Polys {
        poly := &build.Tile.Polys[i]
        require.Equal(t, poly.H00, poly.H10)
        require.Equal(t, poly.H00, poly.H01)
        require.Equal(t, poly.H00, poly.H11)
    }
}

// TestBuildRegionMergeToleranceMergesNoise pins the bounded merge:
// at the tolerance 8 the 8 unit checkerboard noise collapses into one
// rectangle whose four corners carry the geodata heights of their
// corner cells (the bilinear surface blends the staircase - the
// drift the corpus replay of navpack-verify measures).
func TestBuildRegionMergeToleranceMergesNoise(t *testing.T) {
    data := checkerboardRegion(t, 32)
    opts := DefaultOptions()
    opts.MergeTolerance = 8
    build, err := BuildRegion(data, 21, 19, opts)
    require.NoError(t, err)
    require.Len(t, build.Tile.Polys, 1,
        "the 8 unit pair deltas all fit the tolerance 8")
    poly := &build.Tile.Polys[0]
    require.Equal(t, int32(0), poly.X0)
    require.Equal(t, int32(32), poly.X1)
    require.Equal(t, int32(0), poly.Y0)
    require.Equal(t, int32(32), poly.Y1)
    // The checkerboard parity: (x+y) even carries -3504, odd -3512.
    // The corners: (0,0) even, (31,0) odd, (0,31) odd, (31,31) even.
    require.Equal(t, int16(-3504), poly.H00)
    require.Equal(t, int16(-3512), poly.H10)
    require.Equal(t, int16(-3512), poly.H01)
    require.Equal(t, int16(-3504), poly.H11)
}

// TestBuildRegionMergeToleranceClimbCap pins the climb ceiling of the
// merge: the heights beyond the climb never share a sheet (the step
// between the cells is closed), so no tolerance - however large -
// merges them into one polygon.
func TestBuildRegionMergeToleranceClimbCap(t *testing.T) {
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 8 && cy < 8 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }
        if cx >= 8 && cx < 16 && cy < 8 {
            return []layerSpec{{h: -3552, nswe: 0x0F}}
        }

        return nil
    })
    opts := DefaultOptions()
    opts.MergeTolerance = 1000 // beyond the climb: the step rule caps
    build, err := BuildRegion(data, 21, 19, opts)
    require.NoError(t, err)
    require.Len(t, build.Tile.Polys, 2,
        "the 48 unit delta exceeds the climb: two polygons stay")
}

// TestBuildRegionMergeToleranceWallRespected pins the wall rule under
// the merge: the tolerance never overrides the NSWE walls - the
// walled pair of the interior wall field stays in two polygons even
// at the full climb tolerance.
func TestBuildRegionMergeToleranceWallRespected(t *testing.T) {
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 6 && cy < 6 {
            nswe := uint8(0x0F)
            if cx == 2 && cy == 3 {
                nswe &^= nsweEast
            }
            if cx == 3 && cy == 3 {
                nswe &^= nsweWest
            }

            return []layerSpec{{h: -3504, nswe: nswe}}
        }

        return nil
    })
    opts := DefaultOptions()
    opts.MergeTolerance = 40
    build, err := BuildRegion(data, 21, 19, opts)
    require.NoError(t, err)
    for i := range build.Tile.Polys {
        poly := &build.Tile.Polys[i]
        spansWall := poly.X0 <= 2 && poly.X1 > 3 &&
            poly.Y0 <= 3 && poly.Y1 > 3
        require.False(t, spansWall,
            "poly %d (%d..%d x %d..%d) swallows the interior wall", i,
            poly.X0, poly.X1, poly.Y0, poly.Y1)
    }
}

// TestBuildRegionMergeToleranceCornersExact pins the corner fidelity:
// every polygon of a tolerance build carries the geodata heights of
// its own four corner cells - the surfaces the bilinear runtime
// blends stay anchored to the real cells.
func TestBuildRegionMergeToleranceCornersExact(t *testing.T) {
    data := checkerboardRegion(t, 16)
    opts := DefaultOptions()
    opts.MergeTolerance = 8
    build, err := BuildRegion(data, 21, 19, opts)
    require.NoError(t, err)
    require.NotEmpty(t, build.Tile.Polys)
    for i := range build.Tile.Polys {
        poly := &build.Tile.Polys[i]
        require.Equal(t, checkerHeight(poly.X0, poly.Y0),
            poly.H00, "poly %d: h00 must match its corner cell", i)
        require.Equal(t, checkerHeight(poly.X1-1, poly.Y0),
            poly.H10, "poly %d: h10 must match its corner cell", i)
        require.Equal(t, checkerHeight(poly.X0, poly.Y1-1),
            poly.H01, "poly %d: h01 must match its corner cell", i)
        require.Equal(t, checkerHeight(poly.X1-1, poly.Y1-1),
            poly.H11, "poly %d: h11 must match its corner cell", i)
    }
}

// checkerboardRegion serializes the square checkerboard field of the
// side cells: (x+y) even carries checkerEven, odd checkerOdd.
func checkerboardRegion(t *testing.T, side int) []byte {
    t.Helper()

    return writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < side && cy < side {
            h := checkerEven
            if (cx+cy)%2 != 0 {
                h = checkerOdd
            }

            return []layerSpec{{h: h, nswe: 0x0F}}
        }

        return nil
    })
}

// checkerEven and checkerOdd are the two checkerboard heights of the
// merge tolerance tests (multiples of 8, one noise step apart).
const (
    checkerEven = int16(-3504)
    checkerOdd  = int16(-3512)
)

// checkerHeight answers the checkerboard height of one cell: (x+y)
// even carries checkerEven, odd checkerOdd.
func checkerHeight(cx, cy int32) int16 {
    if (cx+cy)%2 != 0 {
        return checkerOdd
    }

    return checkerEven
}
