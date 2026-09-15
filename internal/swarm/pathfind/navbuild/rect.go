// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import "math"

// spanMax returns the larger of two int32 height spans.
func spanMax(a, b int32) int32 {
    if a > b {
        return a
    }

    return b
}

// rectPoly is one rectangle polygon of the built tile: the region
// local cell bounds (half open) and the exact corner heights taken
// from the geodata cells at the inside corners. The interior surface
// is the bilinear interpolation of the four corners, kept within the
// height tolerance of every covered cell by construction (the split
// rule below) - the exact per vertex heights the Detour detail mesh
// would carry.
type rectPoly struct {
    x0, y0, x1, y1     int32
    h00, h10, h01, h11 int16
    area               uint8
}

// rectBuilder decomposes the kept sheets into rectangle polygons. The
// two working grids are reused across the sheets (a cell belongs to
// at most one layer per sheet, so the grid entry is the layer index).
type rectBuilder struct {
    grid    []int32 // per cell: the layer index of the current sheet
    covered []bool  // per cell: the maximal rectangles took it
    polyAt  []int32 // per layer instance: the covering polygon index
    polys   []rectPoly
    layers  []cellLayer // the region layer store the heights read
}

// cellsTotal is the cell count of one region.
const cellsTotal = regionCellsSide * regionCellsSide

// buildRects decomposes every kept sheet into maximal rectangles and
// splits them until the bilinear corner-height surface stays within
// tolerance of every covered cell height. The answer is the polygon
// list and the layer-instance-to-polygon map the link walk consumes.
func buildRects(rl *regionLayers, sh *sheets,
    heightTolerance float64,
) ([]rectPoly, []int32) {
    builder := &rectBuilder{
        grid:    make([]int32, cellsTotal),
        covered: make([]bool, cellsTotal),
        polyAt:  make([]int32, len(rl.layers)),
        polys:   make([]rectPoly, 0, 16384),
        layers:  rl.layers,
    }
    for j := range builder.polyAt {
        builder.polyAt[j] = -1
    }
    for cell := range builder.grid {
        builder.grid[cell] = -1
    }

    members := sheetMembers(sh)
    for sheet := range sh.count {
        if sh.dropped[sheet] {
            continue
        }
        builder.decomposeSheet(rl, sh, members, sheet, heightTolerance)
    }

    return builder.polys, builder.polyAt
}

// sheetMembers buckets the layer instances by their sheet.
func sheetMembers(sh *sheets) [][]uint32 {
    members := make([][]uint32, sh.count)
    for sheet := range members {
        members[sheet] = make([]uint32, 0, sh.sizes[sheet])
    }
    for j, sheet := range sh.sheetOf {
        if sheet < 0 || sh.dropped[sheet] {
            continue
        }
        members[sheet] = append(members[sheet], uint32(j))
    }

    return members
}

// decomposeSheet runs the maximal rectangle decomposition of one sheet
// with the height-bounded splits.
func (b *rectBuilder) decomposeSheet(rl *regionLayers, sh *sheets,
    members [][]uint32, sheet int, heightTolerance float64,
) {
    for _, j := range members[sheet] {
        b.grid[rl.cellIndexOf[j]] = int32(j)
    }
    for _, j := range members[sheet] {
        cell := rl.cellIndexOf[j]
        if b.covered[cell] {
            continue
        }
        cx := int(cell / regionCellsSide)
        cy := int(cell % regionCellsSide)
        x1 := b.extendRight(cx, cy)
        y1 := b.extendDown(cx, cy, x1)
        b.markCovered(cx, cy, x1, y1)
        b.emitRect(cx, cy, x1, y1, sh.class[sheet], heightTolerance)
    }
    // The working grids reset per sheet: a stacked column holds one
    // layer per sheet, the covered flag is sheet local.
    for _, j := range members[sheet] {
        cell := rl.cellIndexOf[j]
        b.grid[cell] = -1
        b.covered[cell] = false
    }
}

// extendRight grows the rectangle width while the row stays in the
// sheet and uncovered.
func (b *rectBuilder) extendRight(cx, cy int) int32 {
    x1 := int32(cx + 1)
    for x1 < regionCellsSide &&
        b.free(int(x1), cy) {
        x1++
    }

    return x1
}

// extendDown grows the rectangle height while every cell of the next
// row stays in the sheet and uncovered (the exact cover invariant:
// the emitted rectangles never overlap).
func (b *rectBuilder) extendDown(cx, cy int, x1 int32) int32 {
    y1 := int32(cy + 1)
rows:
    for y1 < regionCellsSide {
        for x := int32(cx); x < x1; x++ {
            if !b.free(int(x), int(y1)) {
                break rows
            }
        }
        y1++
    }

    return y1
}

// free reports whether a cell belongs to the current sheet and no
// emitted rectangle took it yet.
func (b *rectBuilder) free(cx, cy int) bool {
    cell := cx*regionCellsSide + cy

    return b.grid[cell] >= 0 && !b.covered[cell]
}

// markCovered flags the cells of one emitted rectangle.
func (b *rectBuilder) markCovered(cx, cy int, x1, y1 int32) {
    for x := int32(cx); x < x1; x++ {
        for y := int32(cy); y < y1; y++ {
            b.covered[int(x)*regionCellsSide+int(y)] = true
        }
    }
}

// emitRect appends one rectangle polygon, splitting it recursively
// when the bilinear surface of the four corner heights leaves the
// tolerance at any covered cell.
func (b *rectBuilder) emitRect(cx, cy int, x1, y1 int32, area uint8,
    heightTolerance float64,
) {
    h00 := b.cellHeight(cx, cy)
    h10 := b.cellHeight(int(x1)-1, cy)
    h01 := b.cellHeight(cx, int(y1)-1)
    h11 := b.cellHeight(int(x1)-1, int(y1)-1)
    if b.rectWithinTolerance(cx, cy, x1, y1, h00, h10, h01, h11,
        heightTolerance) {
        index := int32(len(b.polys))
        b.polys = append(b.polys, rectPoly{
            x0: int32(cx), y0: int32(cy), x1: x1, y1: y1,
            h00: h00, h10: h10, h01: h01, h11: h11,
            area: area,
        })
        b.bindLayers(cx, cy, x1, y1, index)

        return
    }
    // Split along the axis that carries the height variation (the
    // interpolation error shrinks with the interpolation distance);
    // ties and flat rectangles fall back to the longer axis. A 1x1
    // rectangle is always exact.
    spanX := spanMax(abs16(h10-h00), abs16(h11-h01))
    spanY := spanMax(abs16(h01-h00), abs16(h11-h10))
    splitX := spanX > spanY ||
        (spanX == spanY && x1-int32(cx) >= y1-int32(cy))
    if splitX {
        mid := (int32(cx) + x1) / 2
        if mid == int32(cx) || mid == x1 {
            b.emitExact(cx, cy, x1, y1, area)

            return
        }
        b.emitRect(cx, cy, mid, y1, area, heightTolerance)
        b.emitRect(int(mid), cy, x1, y1, area, heightTolerance)
    } else {
        mid := (int32(cy) + y1) / 2
        if mid == int32(cy) || mid == y1 {
            b.emitExact(cx, cy, x1, y1, area)

            return
        }
        b.emitRect(cx, cy, x1, mid, area, heightTolerance)
        b.emitRect(cx, int(mid), x1, y1, area, heightTolerance)
    }
}

// emitExact appends a rectangle without the tolerance check (the
// degenerate split fallback of a rectangle that cannot split further
// but still leaves the tolerance - the honest surface of a cliff
// staircase cell pair).
func (b *rectBuilder) emitExact(cx, cy int, x1, y1 int32, area uint8) {
    index := int32(len(b.polys))
    b.polys = append(b.polys, rectPoly{
        x0: int32(cx), y0: int32(cy), x1: x1, y1: y1,
        h00:  b.cellHeight(cx, cy),
        h10:  b.cellHeight(int(x1)-1, cy),
        h01:  b.cellHeight(cx, int(y1)-1),
        h11:  b.cellHeight(int(x1)-1, int(y1)-1),
        area: area,
    })
    b.bindLayers(cx, cy, x1, y1, index)
}

// cellHeight returns the layer height of the current sheet's cell.
func (b *rectBuilder) cellHeight(cx, cy int) int16 {
    j := b.grid[cx*regionCellsSide+cy]
    if j < 0 {
        return 0
    }

    return b.layers[j].h
}

// bindLayers maps every covered layer instance to the polygon index.
func (b *rectBuilder) bindLayers(cx, cy int, x1, y1 int32, index int32) {
    for x := int32(cx); x < x1; x++ {
        for y := int32(cy); y < y1; y++ {
            j := b.grid[int(x)*regionCellsSide+int(y)]
            if j >= 0 {
                b.polyAt[j] = index
            }
        }
    }
}

// rectWithinTolerance checks the bilinear corner surface against every
// covered cell height.
func (b *rectBuilder) rectWithinTolerance(cx, cy int, x1, y1 int32,
    h00, h10, h01, h11 int16, heightTolerance float64,
) bool {
    width := float64(x1-int32(cx)) * 16
    height := float64(y1-int32(cy)) * 16
    for x := int32(cx); x < x1; x++ {
        for y := int32(cy); y < y1; y++ {
            j := b.grid[int(x)*regionCellsSide+int(y)]
            if j < 0 {
                continue
            }
            u := (float64(x-int32(cx))*16 + 8) / width
            v := (float64(y-int32(cy))*16 + 8) / height
            surface := (1-u)*(1-v)*float64(h00) +
                u*(1-v)*float64(h10) + (1-u)*v*float64(h01) +
                u*v*float64(h11)
            if math.Abs(float64(b.layers[j].h)-surface) >
                heightTolerance {
                return false
            }
        }
    }

    return true
}
