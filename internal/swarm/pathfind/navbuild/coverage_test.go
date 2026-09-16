// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestRealRegionWalkableCoverage audits the mesh coverage of the real
// 21_19 region at the cell level: every walkable layer instance of a
// kept sheet must be covered by at least one rectangle polygon (the
// polyAt map answers). The layers of the dropped island sheets and
// the fully blocked cells are the honest voids of the render - the
// viewer shows them as the clear color - so the audit counts them
// separately from a genuine coverage hole (a kept-sheet layer with no
// polygon), which would be a build bug: a walkable cell the mesh
// silently lost.
func TestRealRegionWalkableCoverage(t *testing.T) {
    data := realRegionData(t, 21, 19)
    opts := DefaultOptions()

    rl, err := extractRegion(data, 21, 19, opts.DedupDelta)
    require.NoError(t, err)
    sh := assignSheets(rl, opts.Climb, opts.MinSheetLayers)
    rects, polyAt := buildRects(rl, sh, opts.HeightTolerance,
        opts.Climb)

    var walkable, blocked, dropped, covered, holes int
    holeCells := make(map[int32]int)
    for j, layer := range rl.layers {
        cellIdx := rl.cellIndexOf[j]
        if layer.nswe == 0 {
            blocked++
            continue
        }
        walkable++
        sheet := sh.sheetOf[j]
        if sheet < 0 || sh.dropped[sheet] {
            dropped++
            continue
        }
        if polyAt[j] >= 0 && polyAt[j] < int32(len(rects)) {
            covered++
        } else {
            holes++
            holeCells[cellIdx]++
        }
    }
    t.Logf("coverage audit of 21_19: %d layers, %d walkable, %d"+
        " blocked, %d dropped-island layers, %d covered, %d HOLES,"+
        " %d polys",
        len(rl.layers), walkable, blocked, dropped, covered, holes,
        len(rects))
    // The honest void census of one hole cell, for the follow-up.
    if holes > 0 {
        for cell, n := range holeCells {
            cx, cy := int(cell/regionCellsSide), int(cell%regionCellsSide)
            off := int(rl.cellOff[cell])
            t.Logf("hole at cell (%d, %d): %d layer(s), nswe=%02b,"+
                " sheet=%d",
                cx, cy, n, rl.layers[off].nswe, sh.sheetOf[off])
            break
        }
    }

    // A kept-sheet walkable layer without a covering polygon is a
    // lost walkable cell: the mesh lies about where a bot may stand.
    require.Zero(t, holes,
        "the kept sheets must cover every walkable layer instance")
}
