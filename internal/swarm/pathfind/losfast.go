// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "fmt"
    "math"
)

// The map free line of sight raster. The capsule guard probes call
// the engine line of sight millions of times per route (every merged
// chord candidate of the smooth pass walks the full supercover line),
// and the profile named the A* node machinery as the cost: every
// rastered cell allocated a node and paid two map operations (the
// nodes insert and the missing lookup) although the walkability
// contract needs no graph at all - only the cell sequence, the layer
// choices and the step rules. This file re-walks the identical t/k
// supercover with plain values: the same cell sequence, the same
// closest layer resolution, the same wall and height rules, the same
// server click verification - one region cache slot instead of the
// maps, one scratch buffer per leg instead of per cell allocations.

// cellState is one rastered cell with its resolved layer: the plain
// value pair the fast raster carries instead of the node objects.
type cellState struct {
    p     Point
    layer Layer
}

// regionSlot is the single region cache of the fast raster: the walk
// is highly local, so one pointer check replaces the engine cache
// lock on almost every cell (the search.closestLayer pattern without
// the search allocations).
type regionSlot struct {
    key    RegionKey
    region *Region
}

// cellLayerAt resolves the layer of a cell closest to z through the
// slot cache (false when the cell has no geodata under it).
func (e *Engine) cellLayerAt(slot *regionSlot, p Point,
    z int16,
) (Layer, bool) {
    key := CellToRegion(p)
    if slot.region == nil || slot.key != key {
        entry, err := e.entry(key)
        if err != nil || entry.region == nil {
            slot.region, slot.key = nil, key

            return Layer{Height: 0, NSWE: 0}, false
        }
        slot.region, slot.key = entry.region, key
    }

    return slot.region.ClosestLayer(LocalCell(p), z)
}

// wallsOpenCells mirrors search.wallsOpen for a cell state pair: the
// source wall open in the step direction, the target wall open in the
// reverse direction and the anti corner cut flanks of a diagonal step
// (a flank cell without geodata counts as open, the server reads no
// wall from a region it has no data for).
func (e *Engine) wallsOpenCells(slot *regionSlot, from,
    to cellState,
) bool {
    if from.p.Y > to.p.Y && !from.layer.IsNorthOpen() {
        return false
    }
    if from.p.Y < to.p.Y && !from.layer.IsSouthOpen() {
        return false
    }
    if from.p.X < to.p.X && !from.layer.IsEastOpen() {
        return false
    }
    if from.p.X > to.p.X && !from.layer.IsWestOpen() {
        return false
    }
    if from.p.Y > to.p.Y && !to.layer.IsSouthOpen() {
        return false
    }
    if from.p.Y < to.p.Y && !to.layer.IsNorthOpen() {
        return false
    }
    if from.p.X < to.p.X && !to.layer.IsWestOpen() {
        return false
    }
    if from.p.X > to.p.X && !to.layer.IsEastOpen() {
        return false
    }
    if from.p.X != to.p.X && from.p.Y != to.p.Y {
        return e.diagonalFlanksOpenCells(slot, from, to)
    }

    return true
}

// diagonalFlanksOpenCells mirrors search.diagonalFlanksOpen: the two
// orthogonal cells the diagonal step cuts across must each allow the
// crossing direction, their layers resolved against the source
// height.
func (e *Engine) diagonalFlanksOpenCells(slot *regionSlot, from,
    to cellState,
) bool {
    south := to.p.Y > from.p.Y
    east := to.p.X > from.p.X
    vertical, ok := e.cellLayerAt(slot,
        Point{X: from.p.X, Y: to.p.Y}, from.layer.Height)
    if ok {
        if east && !vertical.IsEastOpen() {
            return false
        }
        if !east && !vertical.IsWestOpen() {
            return false
        }
    }
    horizontal, ok := e.cellLayerAt(slot,
        Point{X: to.p.X, Y: from.p.Y}, from.layer.Height)
    if ok {
        if south && !horizontal.IsSouthOpen() {
            return false
        }
        if !south && !horizontal.IsNorthOpen() {
            return false
        }
    }

    return true
}

// canStepCells mirrors search.canStep: the walls of both cells allow
// the step and the symmetric height difference stays within the
// passable limit.
func (e *Engine) canStepCells(slot *regionSlot, from, to cellState,
    maxPassableHeight int,
) bool {
    if !e.wallsOpenCells(slot, from, to) {
        return false
    }

    return int(heightDelta(from.layer.Height, to.layer.Height)) <=
        maxPassableHeight
}

// cellClickWorld names a cell state the way the server names a click
// target: the cell center with the layer height (the nodeClickWorld
// contract without the node).
func cellClickWorld(c cellState) (int32, int32, int32) {
    return c.p.X*cellSize + worldMinX + cellSize/2,
        c.p.Y*cellSize + worldMinY + cellSize/2,
        int32(c.layer.Height)
}

// serverLegVerifiedBetween resolves the endpoint cells and runs the
// server click validation between them (the serverLegVerified
// contract without the nodes).
func (e *Engine) serverLegVerifiedBetween(a, b Vec3) bool {
    var slot regionSlot
    fromLayer, ok := e.cellLayerAt(&slot,
        WorldToCell(a.X, a.Y), int16(a.Z))
    if !ok {
        return false
    }
    toLayer, ok := e.cellLayerAt(&slot,
        WorldToCell(b.X, b.Y), int16(b.Z))
    if !ok {
        return false
    }
    fromX, fromY, fromZ := cellClickWorld(
        cellState{p: WorldToCell(a.X, a.Y), layer: fromLayer})
    toX, toY, toZ := cellClickWorld(
        cellState{p: WorldToCell(b.X, b.Y), layer: toLayer})
    vx, vy, vz := e.validLocation(fromX, fromY, fromZ, toX, toY, toZ)

    return vx == toX && vy == toY && vz == toZ
}

// legClearCells answers the capsule guard contract for one leg: the
// supercover raster stays walkable end to end, the wall clearance of
// the open interior holds everywhere (every crossed cell checks the
// true minimum distance of its leg portion against the walls of its
// 3x3 neighborhood - strictly tighter than a 4 unit point sample
// chain, the walls are axis aligned segments) and the server click
// validation accepts the leg wholesale. The endpoints stay unprobed
// (the first and the last four units ride on the callers: the start
// is where the character stands, the end is where it asked to go -
// the waypoint passes own their clearance). A missing endpoint cell
// reads as not clear (the legWalkable error contract).
func (e *Engine) legClearCells(a, b Vec3, radius float64,
    maxPassableHeight uint16,
) bool {
    walked, err := e.walkSupercover(a, b, int(maxPassableHeight), radius)

    return err == nil && walked && e.serverLegVerifiedBetween(a, b)
}

// lineOfSightCells answers whether the straight walk between two
// world positions stays on one walkable surface the whole way (the
// server movement rules, the click validation included). A missing
// endpoint cell is the ErrMissingCell error (the node oracle
// contract).
func (e *Engine) lineOfSightCells(start, end Vec3,
    maxPassableHeight uint16,
) (bool, error) {
    walked, err := e.walkSupercover(start, end,
        int(maxPassableHeight), 0)
    if err != nil {
        return false, err
    }
    if !walked {
        return false, nil
    }

    return e.serverLegVerifiedBetween(start, end), nil
}

// lineOfSightNodes is the node graph oracle of the fast raster (the
// differential test pins the two together; the grid smoothing keeps
// the node machinery for its own context).
func (e *Engine) lineOfSightNodes(start, end Vec3,
    maxPassableHeight uint16,
) (bool, error) {
    search := newSearch(e, maxPassableHeight)
    from, err := search.nodeAtWorld(start)
    if err != nil {
        return false, err
    }
    to, err := search.nodeAtWorld(end)
    if err != nil {
        return false, err
    }

    return search.lineOfSight(from, to), nil
}

// walkSupercover resolves the endpoints and walks the supercover
// raster with the clearance contract of the radius (zero skips the
// wall checks - the pure walkability form). A missing endpoint cell
// is the ErrMissingCell error.
func (e *Engine) walkSupercover(a, b Vec3, maxPassableHeight int,
    radius float64,
) (bool, error) {
    var slot regionSlot
    fromP := WorldToCell(a.X, a.Y)
    toP := WorldToCell(b.X, b.Y)
    fromLayer, ok := e.cellLayerAt(&slot, fromP, int16(a.Z))
    if !ok {
        return false, fmt.Errorf("%w at %.0f %.0f", ErrMissingCell,
            a.X, a.Y)
    }
    toLayer, ok := e.cellLayerAt(&slot, toP, int16(b.Z))
    if !ok {
        return false, fmt.Errorf("%w at %.0f %.0f", ErrMissingCell,
            b.X, b.Y)
    }
    scratch := make([]Layer, 0, 8)
    walked := e.walkSupercoverCells(slot,
        cellState{p: fromP, layer: fromLayer},
        cellState{p: toP, layer: toLayer}, a, b,
        maxPassableHeight, radius, scratch)

    return walked, nil
}

// walkSupercoverCells walks the t/k supercover line between the two
// cells (the straightPath stepping, verbatim): every step must pass
// the walk rules and, with a positive radius, every crossed cell must
// hold the capsule clearance over its portion of the line. A cell
// without geodata truncates the walk (the straightPath break - the
// server click validation answers the gap).
func (e *Engine) walkSupercoverCells(slot regionSlot, from,
    to cellState, a, b Vec3, maxPassableHeight int,
    radius float64, scratch []Layer,
) bool {
    xS, yS := from.p.X, from.p.Y
    xE, yE := to.p.X, to.p.Y
    signX := sign(xE - xS)
    signY := sign(yE - yS)
    x, y := int64(0), int64(0)
    x1, y1 := int64(xE-xS), int64(yE-yS)
    k := math.Abs(float64(y1) / float64(x1))

    // The line parameters of the clearance portions: the interior
    // contract skips the capsuleClearanceTrim arc units at both ends
    // (the endpoints are the callers' own responsibility).
    length := math.Hypot(b.X-a.X, b.Y-a.Y)
    trimStart, trimEnd := 0.0, 1.0
    if radius > 0 && length > 2*capsuleClearanceTrim {
        trimStart = capsuleClearanceTrim / length
        trimEnd = 1 - trimStart
    }

    current := from
    tEnter := 0.0
    truncated := false
    for x != x1 || y != y1 {
        t := float64(2*y*int64(signY)+1) / float64(2*x*int64(signX)+1)
        xStepped, yStepped := false, false
        if t >= k {
            x += int64(signX)
            xStepped = signX != 0
        }
        if t <= k {
            y += int64(signY)
            yStepped = signY != 0
        }
        nextP := Point{X: xS + int32(x), Y: yS + int32(y)}
        nextLayer, ok := e.cellLayerAt(&slot, nextP, from.layer.Height)
        if !ok {
            truncated = true
            break
        }
        next := cellState{p: nextP, layer: nextLayer}
        if !e.canStepCells(&slot, current, next, maxPassableHeight) {
            return false
        }
        if radius > 0 && length > 0 {
            tExit := 1.0
            if xStepped {
                tExit = math.Min(tExit, crossingT(a.X, b.X,
                    float64(nextP.X)*cellSize+worldMinX))
            }
            if yStepped {
                tExit = math.Min(tExit, crossingT(a.Y, b.Y,
                    float64(nextP.Y)*cellSize+worldMinY))
            }
            if !portionClear(e, &slot, current, a, b, tEnter, tExit,
                trimStart, trimEnd, radius, scratch) {
                return false
            }
            tEnter = tExit
        }
        current = next
    }
    if truncated {
        return true // the walk broke on a missing cell: the click
        // validation answers the gap (the straightPath break)
    }

    // The tail portion of the last cell (the destination cell never
    // exits, its clearance portion ends the leg).
    if radius > 0 && length > 0 {
        return portionClear(e, &slot, current, a, b, tEnter, 1.0,
            trimStart, trimEnd, radius, scratch)
    }

    return true
}

// crossingT answers the line parameter where the leg crosses the cell
// boundary at the given world coordinate (the boundary always sits
// between the endpoints on that axis).
func crossingT(a, b, boundary float64) float64 {
    if b == a {
        return 1
    }

    return (boundary - a) / (b - a)
}

// capsuleClearanceTrim is the unprobed arc length at both leg ends of
// the clearance raster: the four unit first sample of the point chain
// it replaces (the start is where the character stands, the end is
// the asked destination - their wall distance belongs to the waypoint
// passes).
const capsuleClearanceTrim = 4.0

// portionClear answers whether the leg portion between the line
// parameters lo and hi keeps the radius from every closed wall edge
// of the containing cell's 3x3 neighborhood (the layers resolved
// against the portion's middle height - the nearestWall layer choice
// per neighbor cell). The portion is clipped to the trim window
// first; a portion fully inside the trim window answers clear.
func portionClear(e *Engine, slot *regionSlot, cell cellState,
    a, b Vec3, tLo, tHi, trimStart, trimEnd, radius float64,
    scratch []Layer,
) bool {
    lo, hi := tLo, tHi
    if lo < trimStart {
        lo = trimStart
    }
    if hi > trimEnd {
        hi = trimEnd
    }
    if hi <= lo {
        return true
    }
    x0 := a.X + (b.X-a.X)*lo
    y0 := a.Y + (b.Y-a.Y)*lo
    x1 := a.X + (b.X-a.X)*hi
    y1 := a.Y + (b.Y-a.Y)*hi
    zMid := a.Z + (b.Z-a.Z)*(lo+hi)/2

    key := CellToRegion(cell.p)
    region := slot.region
    if region == nil || slot.key != key {
        entry, err := e.entry(key)
        if err != nil || entry.region == nil {
            return true // no geodata: no walls to hold
        }
        region = entry.region
    }
    local := LocalCell(cell.p)
    refZ := int16(zMid)
    stack := scratch[:0]
    for dx := -1; dx <= 1; dx++ {
        for dy := -1; dy <= 1; dy++ {
            nx := int(local.X) + dx
            ny := int(local.Y) + dy
            if nx < 0 || ny < 0 ||
                nx >= cellsPerRegionSide || ny >= cellsPerRegionSide {
                continue
            }
            stack = region.LayerStack(
                Point{X: int32(nx), Y: int32(ny)}, stack)
            if len(stack) == 0 {
                continue
            }
            layer := closestLayerOf(stack, refZ)
            if !portionWallsClear(x0, y0, x1, y1, key, nx, ny,
                layer, radius) {
                return false
            }
        }
    }

    return true
}

// portionWallsClear folds the closed wall edges of one neighbor cell
// into the distance check of the leg portion (the mergeCellWalls
// wall set with the segment form of the distance).
func portionWallsClear(x0, y0, x1, y1 float64, key RegionKey,
    nx, ny int, layer Layer, radius float64,
) bool {
    global := Point{
        X: int32(key.Col)*cellsPerRegionSide + int32(nx),
        Y: int32(key.Row)*cellsPerRegionSide + int32(ny),
    }
    minCorner := CellToWorldMin(global)
    wx0, wy0 := minCorner.X, minCorner.Y
    wx1, wy1 := wx0+cellSize, wy0+cellSize
    wall := func(ax, ay, bx, by float64) bool {
        return segmentSegmentDistance(x0, y0, x1, y1, ax, ay, bx, by) <
            radius
    }
    if !layer.IsWestOpen() && wall(wx0, wy0, wx0, wy1) {
        return false
    }
    if !layer.IsEastOpen() && wall(wx1, wy0, wx1, wy1) {
        return false
    }
    if !layer.IsNorthOpen() && wall(wx0, wy0, wx1, wy0) {
        return false
    }
    if !layer.IsSouthOpen() && wall(wx0, wy1, wx1, wy1) {
        return false
    }

    return true
}

// segmentSegmentDistance answers the smallest distance between two
// segments: zero when they touch, the minimum of the endpoint to
// segment distances otherwise (the walls are axis aligned edges, the
// leg portions are short slivers, the closed form stays simple).
func segmentSegmentDistance(ax, ay, bx, by, cx, cy, dx,
    dy float64,
) float64 {
    // The intersection test through the orientation signs.
    d1 := orientSign(cx, cy, dx, dy, ax, ay)
    d2 := orientSign(cx, cy, dx, dy, bx, by)
    d3 := orientSign(ax, ay, bx, by, cx, cy)
    d4 := orientSign(ax, ay, bx, by, dx, dy)
    if d1*d2 < 0 && d3*d4 < 0 {
        return 0
    }
    if d1 == 0 && onSegment(ax, ay, cx, cy, dx, dy) {
        return 0
    }
    if d2 == 0 && onSegment(bx, by, cx, cy, dx, dy) {
        return 0
    }
    if d3 == 0 && onSegment(cx, cy, ax, ay, bx, by) {
        return 0
    }
    if d4 == 0 && onSegment(dx, dy, ax, ay, bx, by) {
        return 0
    }
    best, _, _ := segmentDistance(ax, ay, cx, cy, dx, dy)
    if d, _, _ := segmentDistance(bx, by, cx, cy, dx, dy); d < best {
        best = d
    }
    if d, _, _ := segmentDistance(cx, cy, ax, ay, bx, by); d < best {
        best = d
    }
    if d, _, _ := segmentDistance(dx, dy, ax, ay, bx, by); d < best {
        best = d
    }

    return best
}

// orientSign answers the orientation of the triplet (the cross
// product sign of the two edge vectors).
func orientSign(ax, ay, bx, by, px, py float64) float64 {
    return (bx-ax)*(py-ay) - (by-ay)*(px-ax)
}

// onSegment answers whether the point sits on the segment's bounding
// box (the collinear case of the intersection test).
func onSegment(px, py, ax, ay, bx, by float64) bool {
    return px >= math.Min(ax, bx)-1e-9 && px <= math.Max(ax, bx)+1e-9 &&
        py >= math.Min(ay, by)-1e-9 && py <= math.Max(ay, by)+1e-9
}
