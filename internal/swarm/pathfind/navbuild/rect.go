// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

// rectPoly is one rectangle polygon of the built tile: the region
// local cell bounds (half open) and the exact geodata height of its
// cells. A rectangle merges only the cells of one exact height, so
// every polygon is the flat square quad the raw l2j geodata holds -
// the detour mesh represents the original squares as they are, the
// visual and the actual surface alike, quantization staircase
// included (the owner's porting directive; the bilinear vertex field
// of the previous rounds smoothed that staircase away and the owner
// pinned the faithful representation instead).
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

// buildRects decomposes every kept sheet into maximal rectangles of
// one exact cell height. The answer is the polygon list and the
// layer-instance-to-polygon map the link walk consumes.
//
// The growth respects the NSWE walls: a rectangle only spans cells
// whose mutual steps are open (the paired walls of both sides plus
// the climb height rule), so the interior of every polygon is
// walkable by construction - the contract the link walk's
// same-polygon skip and the funnel legs rely on. The wall-blind
// growth of the first rounds let a single polygon swallow walled
// cell pairs (the Dion merchant quarter measured 1.24M such pairs in
// 21_22, 1.43M in 22_22): the corridor search then tunnelled
// straight through the buildings and the bots walked out of town.
//
// The growth also respects the exact height: two cells of a different
// height never share a rectangle, so a polygon's surface is exactly
// the height its cells carry in the geodata - the port of the l2j
// squares changes nothing about where the ground sits.
func buildRects(rl *regionLayers, sh *sheets,
    climb int32,
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
        builder.decomposeSheet(rl, sh, members, sheet, climb)
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

// decomposeSheet runs the maximal same-height rectangle decomposition
// of one sheet.
func (b *rectBuilder) decomposeSheet(rl *regionLayers, sh *sheets,
    members [][]uint32, sheet int, climb int32,
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
        x1 := b.extendRight(cx, cy, climb)
        y1 := b.extendDown(cx, cy, x1, climb)
        b.markCovered(cx, cy, x1, y1)
        b.emitRect(cx, cy, x1, y1, sh.class[sheet])
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
// sheet, uncovered, carries the seed cell's exact height and the
// horizontal step into the new cell crosses no wall.
func (b *rectBuilder) extendRight(cx, cy int, climb int32) int32 {
    height := b.cellHeight(cx, cy)
    x1 := int32(cx + 1)
    for x1 < regionCellsSide &&
        b.free(int(x1), cy) &&
        b.cellHeight(int(x1), cy) == height &&
        b.hStepOpen(int(x1)-1, cy, climb) {
        x1++
    }

    return x1
}

// extendDown grows the rectangle height while every cell of the next
// row stays in the sheet, uncovered, carries the seed cell's exact
// height, the vertical step into it crosses no wall and the row
// itself holds no interior wall (the exact cover invariant: the
// emitted rectangles never overlap; the wall invariant: every
// adjacent cell pair inside one polygon is an open step, so the
// interior stays walkable by construction).
func (b *rectBuilder) extendDown(cx, cy int, x1 int32, climb int32) int32 {
    height := b.cellHeight(cx, cy)
    y1 := int32(cy + 1)
rows:
    for y1 < regionCellsSide {
        for x := int32(cx); x < x1; x++ {
            if !b.free(int(x), int(y1)) ||
                b.cellHeight(int(x), int(y1)) != height ||
                !b.vStepOpen(int(x), int(y1)-1, climb) {
                break rows
            }
            if x > int32(cx) &&
                !b.hStepOpen(int(x)-1, int(y1), climb) {
                break rows
            }
        }
        y1++
    }

    return y1
}

// hStepOpen reports whether the horizontal step between the cells
// (x, y) and (x+1, y) of the current sheet is an open passage: the
// east wall of the source, the west wall of the target (the
// wallsOpen rule of the grid search) and the climb height range.
func (b *rectBuilder) hStepOpen(x, y int, climb int32) bool {
    a := b.grid[x*regionCellsSide+y]
    c := b.grid[(x+1)*regionCellsSide+y]
    if a < 0 || c < 0 {
        return false
    }

    return stepOpen(b.layers[a], b.layers[c], 1, 0, climb)
}

// vStepOpen reports whether the vertical step between the cells
// (x, y) and (x, y+1) of the current sheet is an open passage.
func (b *rectBuilder) vStepOpen(x, y int, climb int32) bool {
    a := b.grid[x*regionCellsSide+y]
    c := b.grid[x*regionCellsSide+y+1]
    if a < 0 || c < 0 {
        return false
    }

    return stepOpen(b.layers[a], b.layers[c], 0, 1, climb)
}

// stepOpen reports whether the cell layer pair admits the step in
// (dx, dy): the paired NSWE walls of both sides plus the climb
// height rule - the same canStep the grid search walks on.
func stepOpen(a, b cellLayer, dx, dy int32, climb int32) bool {
    if abs16(a.h-b.h) > climb {
        return false
    }

    return nsweOpen(a, b, dx, dy)
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

// emitRect appends one rectangle polygon flat at the exact geodata
// height of its cells: the growth above only ever spans cells of one
// height, so all four corners carry that height and the polygon
// surface is the square the raw l2j geometry holds - no vertex
// field, no bilinear approximation, nothing between the mesh and the
// geodata numbers.
func (b *rectBuilder) emitRect(cx, cy int, x1, y1 int32, area uint8) {
    height := b.cellHeight(cx, cy)
    index := int32(len(b.polys))
    b.polys = append(b.polys, rectPoly{
        x0: int32(cx), y0: int32(cy), x1: x1, y1: y1,
        h00: height, h10: height, h01: height, h11: height,
        area: area,
    })
    b.bindLayers(cx, cy, x1, y1, index)
}

// cellHeight returns the exact geodata height of the cell's layer in
// the current sheet (the callers only ask for sheet members).
func (b *rectBuilder) cellHeight(cx, cy int) int16 {
    return b.layers[b.grid[cx*regionCellsSide+cy]].h
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
