// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestBuildRegionInteriorWall pins the wall rule of the rectangle
// growth: a wall segment that starts and ends INSIDE what would be the
// maximal rectangle (not on its border) splits the decomposition, so
// no polygon swallows a walled cell pair. The wall-blind growth of the
// first rounds covered such pairs with one polygon and the link walk's
// same-polygon skip then hid the wall from the search - the corridor
// tunnelled straight through (the out-of-town walks of the Dion
// merchant quarter).
func TestBuildRegionInteriorWall(t *testing.T) {
    // A 6x6 open field with one east-west wall pair inside it: the
    // step between (2, 3) and (3, 3) is closed from both sides while
    // every vertical step of the field stays open.
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
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)
    tile := build.Tile

    // Every polygon avoids the wall: none of them spans both x 2 and
    // x 3 at y 3 (the walled pair must sit in two different polygons).
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        spansWall := poly.X0 <= 2 && poly.X1 > 3 &&
            poly.Y0 <= 3 && poly.Y1 > 3
        require.False(t, spansWall,
            "poly %d (%d..%d x %d..%d) swallows the interior wall", i,
            poly.X0, poly.X1, poly.Y0, poly.Y1)
    }

    // The wall shows up as a blocked pair between two polygons.
    require.Positive(t, build.Stats.NSWEBlockedPairs)
}

// TestBuildRegionInteriorWalls audits the real regions end to end:
// every adjacent kept cell pair that lands inside ONE polygon must be
// an open step (the paired NSWE walls plus the climb height rule) -
// the interior-walkable contract the link walk's same-polygon skip and
// the funnel legs rely on. The wall-blind growth of the first rounds
// failed this audit on the town regions by the millions.
func TestBuildRegionInteriorWalls(t *testing.T) {
    for _, region := range [][2]int16{{21, 19}, {21, 22}} {
        auditRegionInteriorWalls(t, region[0], region[1])
    }
}

// auditRegionInteriorWalls runs the same-polygon wall audit over one
// real region of the shipped geodata pack.
func auditRegionInteriorWalls(t *testing.T, col, row int16) {
    t.Helper()
    data := realRegionData(t, col, row)
    opts := DefaultOptions()
    rl, err := extractRegion(data, col, row, opts.DedupDelta)
    require.NoError(t, err)
    sh := assignSheets(rl, opts.Climb, opts.MinSheetLayers)
    _, polyAt := buildRects(rl, sh, opts.Climb)

    // Every kept layer instance must own a polygon: the decomposition
    // covers the sheet completely.
    uncovered := 0
    for j, sheet := range sh.sheetOf {
        if sheet < 0 || sh.dropped[sheet] {
            continue
        }
        if polyAt[j] < 0 {
            uncovered++
        }
    }
    require.Zero(t, uncovered,
        "every kept layer of %d_%d must map to a polygon", col, row)

    // The wall audit: the east and the south neighbour pairs.
    swallowed := 0
    interior := 0
    for cx := range regionCellsSide - 1 {
        for cy := range regionCellsSide - 1 {
            auditCellPair(t, rl, polyAt, opts, cx, cy, 1, 0,
                &swallowed, &interior)
            auditCellPair(t, rl, polyAt, opts, cx, cy, 0, 1,
                &swallowed, &interior)
        }
    }
    require.Zero(t, swallowed,
        "region %d_%d: %d walled cell pairs live inside one polygon",
        col, row, swallowed)
    t.Logf("region %d_%d: %d interior pairs audited, 0 walls swallowed",
        col, row, interior)
}

// auditCellPair checks every kept layer pair of two adjacent cells
// that shares one polygon: the pair must admit the step.
func auditCellPair(t *testing.T, rl *regionLayers, polyAt []int32,
    opts Options, cx, cy int, dx, dy int, swallowed, interior *int,
) {
    t.Helper()
    idx := cx*regionCellsSide + cy
    nIdx := (cx+dx)*regionCellsSide + cy + dy
    off, cnt := int(rl.cellOff[idx]), int(rl.cellCnt[idx])
    nOff, nCnt := int(rl.cellOff[nIdx]), int(rl.cellCnt[nIdx])
    for k := off; k < off+cnt; k++ {
        if polyAt[k] < 0 {
            continue
        }
        for m := nOff; m < nOff+nCnt; m++ {
            if polyAt[m] < 0 || polyAt[m] != polyAt[k] {
                continue
            }
            *interior++
            if !stepOpen(rl.layers[k], rl.layers[m],
                int32(dx), int32(dy), opts.Climb) {
                *swallowed++
            }
        }
    }
}
