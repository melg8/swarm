// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

// rectPoly is one rectangle polygon of the built tile: the region
// local cell bounds (half open) and the geodata heights of its four
// corner cells. While the merge tolerance keeps the exact height
// rule (the default), every polygon is the flat square quad the raw
// l2j geodata holds - the detour mesh represents the original
// squares as they are, the visual and the actual surface alike,
// quantization staircase included (the owner's porting directive;
// the bilinear vertex field of the previous rounds smoothed that
// staircase away and the owner pinned the faithful representation
// instead). The bounded merge across height deltas of the structural
// round (issue #57) relaxes the rule: the corners then carry the
// geodata heights of their corner cells and the surface blends the
// merged staircase - the drift the corpus replay measures.
type rectPoly struct {
    x0, y0, x1, y1     int32
    h00, h10, h01, h11 int16
    area               uint8
}

// rectBuilder decomposes the kept sheets into rectangle polygons. The
// two working grids are reused across the sheets (a cell belongs to
// at most one layer per sheet, so the grid entry is the layer index).
type rectBuilder struct {
    grid      []int32 // per cell: the layer index of the current sheet
    covered   []bool  // per cell: the maximal rectangles took it
    polyAt    []int32 // per layer instance: the covering polygon index
    polys     []rectPoly
    layers    []cellLayer // the region layer store the heights read
    tolerance int32       // the merge tolerance of the growth (0: exact)
}

// cellsTotal is the cell count of one region.
const cellsTotal = regionCellsSide * regionCellsSide

// buildRects decomposes every kept sheet into maximal rectangles
// bounded by the merge tolerance (0: one exact cell height). The
// answer is the polygon list and the layer-instance-to-polygon map
// the link walk consumes.
//
// The growth respects the NSWE walls: a rectangle only spans cells
// whose mutual steps are open (the paired walls of both sides plus
// the climb height rule), so the interior of every polygon is
// walkable by construction - the contract the link walk's
// same-polygon skip and the funnel segments rely on. The wall-blind
// growth of the first rounds let a single polygon swallow walled
// cell pairs (the Dion merchant quarter measured 1.24M such pairs in
// 21_22, 1.43M in 22_22): the corridor search then tunnelled
// straight through the buildings and the bots walked out of town.
//
// The growth respects the merge tolerance the same way: two adjacent
// cells whose height difference exceeds the tolerance never share a
// rectangle, and the tolerance never overrides the walls or the
// climb - a pair beyond the climb cannot share an open step, let
// alone a rectangle, so the climb limit caps the merge from above.
// At 0 the rule degenerates to the exact decomposition the faithful
// square port pinned: two cells of a different height never share a
// rectangle, so a polygon's surface is exactly the height its cells
// carry in the geodata (the structural round, issue #57).
func buildRects(rl *regionLayers, sh *sheets,
    climb int32, mergeTolerance int32,
) ([]rectPoly, []int32) {
    builder := &rectBuilder{
        grid:      make([]int32, cellsTotal),
        covered:   make([]bool, cellsTotal),
        polyAt:    make([]int32, len(rl.layers)),
        polys:     make([]rectPoly, 0, 16384),
        layers:    rl.layers,
        tolerance: mergeTolerance,
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
// sheet, uncovered, the horizontal step into the new cell crosses no
// wall and the height difference across the step stays within the
// merge tolerance (0: the exact same-height rule).
func (b *rectBuilder) extendRight(cx, cy int, climb int32) int32 {
    x1 := int32(cx + 1)
    for x1 < regionCellsSide &&
        b.free(int(x1), cy) &&
        b.pairWithinTolerance(
            b.cellHeight(int(x1)-1, cy), b.cellHeight(int(x1), cy)) &&
        b.hStepOpen(int(x1)-1, cy, climb) {
        x1++
    }

    return x1
}

// extendDown grows the rectangle height while every cell of the next
// row stays in the sheet, uncovered, the vertical step into it
// crosses no wall with the height difference within the merge
// tolerance, and the row itself holds no interior wall (the exact
// cover invariant: the emitted rectangles never overlap; the wall
// invariant: every adjacent cell pair inside one polygon is an open
// step, so the interior stays walkable by construction).
func (b *rectBuilder) extendDown(cx, cy int, x1 int32, climb int32) int32 {
    y1 := int32(cy + 1)
rows:
    for y1 < regionCellsSide {
        for x := int32(cx); x < x1; x++ {
            if !b.free(int(x), int(y1)) ||
                !b.pairWithinTolerance(
                    b.cellHeight(int(x), int(y1)-1),
                    b.cellHeight(int(x), int(y1))) ||
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

// pairWithinTolerance reports whether the height difference of two
// adjacent cells fits the merge tolerance (0: only equal heights
// merge - the exact decomposition of the faithful port).
func (b *rectBuilder) pairWithinTolerance(a, c int16) bool {
    return abs16(a-c) <= b.tolerance
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

// emitRect appends one rectangle polygon whose four corners carry
// the geodata heights of their corner cells: the growth keeps every
// interior step within the walls, the climb and the tolerance, and
// at the zero tolerance all four cells share the one exact height,
// so the polygon is the flat square the raw l2j geometry holds - no
// vertex field, no bilinear approximation, nothing between the mesh
// and the geodata numbers. With the tolerance the corners blend the
// merged staircase (the drift the corpus replay measures per
// candidate).
func (b *rectBuilder) emitRect(cx, cy int, x1, y1 int32, area uint8) {
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
