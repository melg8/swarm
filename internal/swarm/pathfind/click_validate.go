// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The server-side click validation port: answers whether the game
// server would run or cancel a mouse-mode MoveToLocation click between
// two world points, using the same rules the Mobius C1 server applies
// (Creature.moveToLocation + GeoEngine.getValidLocation).
package pathfind

import "math"

// Click validation constants, mirroring the Mobius C1 server.
const (
    // worldMinX/worldMinY anchor the cell grid on the world map (the
    // negative of the tile zero coordinates, in world units).
    worldMinX = -tileZeroCol * tileSize
    worldMinY = -tileZeroRow * tileSize
    // clickHeightIncreaseLimit mirrors GeoEngine.HEIGHT_INCREASE_LIMIT:
    // a Bresenham step that climbs more than this over the running
    // height is a terrace wall - unless a neighbour layer continues
    // the surface (the server steps over single-cell layer gaps).
    clickHeightIncreaseLimit = int16(40)
    // clickLayerTolerance mirrors GeoEngine.PATH_CONTINUITY_TOLERANCE:
    // the neighbour layer search window of the gap step-over.
    clickLayerTolerance = int16(16)
    // clickFarDistance mirrors the server rule that skips the geodata
    // correction for far player clicks ("Should be able to click far
    // away and move"): a click beyond this distance moves as clicked.
    clickFarDistance = 3000.0
    // clickMinimumDistance mirrors the distance < 1 cancellation of
    // Creature.moveToLocation: a click the geodata correction
    // collapses onto the walker never runs.
    clickMinimumDistance = 1.0
)

// ValidateClick answers what the server would do with a mouse-mode
// MoveToLocation click: the destination it actually walks to and whether the
// click runs at all. A click the geodata correction collapses onto the walker
// is canceled, and the bot re-clicking the same waypoint freezes until the
// re-path budget aborts the trip. The port follows Creature.moveToLocation with
// a failing server pathfinder: the far click rule, the Bresenham line with the
// running height resolution, the climb limit and step-over, the blocked cell
// and anti corner cut checks, the final layer match (docs/pathfinding.md).
func (e *Engine) ValidateClick(from, to Vec3) (Vec3, bool) {
    if e == nil {
        return to, true
    }
    dx := to.X - from.X
    dy := to.Y - from.Y
    if math.Hypot(dx, dy) >= clickFarDistance {
        // The far click rule: the server skips the geodata correction
        // and moves as clicked.
        return to, true
    }
    x, y, z := int32(math.Round(from.X)), int32(math.Round(from.Y)),
        int32(math.Round(from.Z))
    tx, ty, tz := int32(math.Round(to.X)), int32(math.Round(to.Y)),
        int32(math.Round(to.Z))
    vx, vy, vz := e.validLocation(x, y, z, tx, ty, tz)
    if math.Hypot(float64(vx-x), float64(vy-y)) < clickMinimumDistance {
        return Vec3{X: from.X, Y: from.Y, Z: from.Z}, false
    }

    return Vec3{X: float64(vx), Y: float64(vy), Z: float64(vz)}, true
}

// validLocation is the GeoEngine.getValidLocation port: it walks the
// Bresenham cell line from (x, y) to (tx, ty) the way the server does
// and returns the last position the line reaches legally.
func (e *Engine) validLocation(
    x, y, z, tx, ty, tz int32,
) (int32, int32, int32) {
    from := WorldToCell(float64(x), float64(y))
    to := WorldToCell(float64(tx), float64(ty))
    nearestFrom := e.nearestCellZ(from, int16(z))
    nearestTo := e.nearestCellZ(to, int16(tz))
    iter := newClickLine(from.X, from.Y, to.X, to.Y)
    iter.next()
    prevX, prevY := iter.x(), iter.y()
    prevZ := nearestFrom
    for iter.next() {
        curX, curY := iter.x(), iter.y()
        curZ := e.nearestCellZ(Point{X: curX, Y: curY}, prevZ)
        if curZ-prevZ > clickHeightIncreaseLimit {
            if !e.neighbourLayerNear(curX, curY, prevZ,
                clickLayerTolerance) {
                return clickWorldOf(prevX, prevY, prevZ)
            }
            curZ = prevZ
        }
        dir := clickDirection(prevX, prevY, curX, curY)
        if e.cellBlocked(Point{X: curX, Y: curY}, curZ) ||
            !e.nsweAllows(Point{X: prevX, Y: prevY}, prevZ, dir) ||
            !e.antiCornerCut(prevX, prevY, prevZ, dir) {
            return clickWorldOf(prevX, prevY, prevZ)
        }
        prevX, prevY, prevZ = curX, curY, curZ
    }
    if prevZ != nearestTo {
        // The final layer rule: the line arrived on a different layer than the
        // target z names - the server collapses the click back onto the walker.
        return clickWorldOf(from.X, from.Y, nearestFrom)
    }

    return clickWorldOf(to.X, to.Y, nearestTo)
}

// clickLine is a verbatim port of the server GridLineIterator2D: the Bresenham
// raster whose diagonal double steps the anti corner cut rule applies to (the
// search raster splits the same line into cardinal steps).
type clickLine struct {
    curX, curY     int32
    tgtX, tgtY     int32
    stepX, stepY   int32
    deltaX, deltaY int32
    steep          bool
    err            int32
    started        bool
}

func newClickLine(sx, sy, ex, ey int32) *clickLine {
    dx := ex - sx
    if dx < 0 {
        dx = -dx
    }
    dy := ey - sy
    if dy < 0 {
        dy = -dy
    }
    steep := dy > dx
    err := dx
    if steep {
        err = dy
    }

    return &clickLine{
        curX: sx, curY: sy, tgtX: ex, tgtY: ey,
        stepX: clickSign(ex - sx), stepY: clickSign(ey - sy),
        deltaX: dx, deltaY: dy, steep: steep, err: err / 2,
        started: false,
    }
}

func (it *clickLine) next() bool {
    if !it.started {
        it.started = true

        return true
    }
    if it.curX == it.tgtX && it.curY == it.tgtY {
        return false
    }
    if it.steep {
        it.curY += it.stepY
        it.err -= it.deltaX
        if it.err < 0 {
            it.curX += it.stepX
            it.err += it.deltaY
        }
    } else {
        it.curX += it.stepX
        it.err -= it.deltaY
        if it.err < 0 {
            it.curY += it.stepY
            it.err += it.deltaX
        }
    }

    return true
}

func (it *clickLine) x() int32 {
    return it.curX
}

func (it *clickLine) y() int32 {
    return it.curY
}

func clickSign(v int32) int32 {
    switch {
    case v > 0:
        return 1
    case v < 0:
        return -1
    default:
        return 0
    }
}

// clickDirection computes the NSWE direction mask of a step, with the
// bit values of the Mobius Cell: east 0x01, west 0x02, south 0x04,
// north 0x08 (world y grows south).
func clickDirection(fx, fy, tx, ty int32) uint8 {
    var dir uint8
    if tx > fx {
        dir |= 1 << 0
    }
    if tx < fx {
        dir |= 1 << 1
    }
    if ty > fy {
        dir |= 1 << 2
    }
    if ty < fy {
        dir |= 1 << 3
    }

    return dir
}

// clickWorldOf converts a cell to the world position of its center,
// the coordinate the server getValidLocation returns for it.
func clickWorldOf(cx, cy int32, z int16) (int32, int32, int32) {
    return cx*cellSize + worldMinX + cellSize/2,
        cy*cellSize + worldMinY + cellSize/2, int32(z)
}

// nearestCellZ resolves the layer height of the cell closest to z (a missing
// region keeps z: the server reads no geodata from it and corrects nothing).
func (e *Engine) nearestCellZ(cell Point, z int16) int16 {
    entry, err := e.entry(CellToRegion(cell))
    if err != nil {
        return z
    }
    layer, ok := entry.region.ClosestLayer(LocalCell(cell), z)
    if !ok {
        return z
    }

    return layer.Height
}

// cellNswe resolves the NSWE mask of the cell layer closest to z.
func (e *Engine) cellNswe(cell Point, z int16) uint8 {
    entry, err := e.entry(CellToRegion(cell))
    if err != nil {
        return nsweAll
    }
    layer, ok := entry.region.ClosestLayer(LocalCell(cell), z)
    if !ok {
        return nsweAll
    }

    return layer.NSWE
}

// nsweAllows reports whether the cell allows moving into the
// direction mask (a set bit is an open direction).
func (e *Engine) nsweAllows(cell Point, z int16, dir uint8) bool {
    return e.cellNswe(cell, z)&dir == dir
}

// cellBlocked reports whether every direction of the cell is walled
// at the layer closest to z (the isCompletelyBlocked check).
func (e *Engine) cellBlocked(cell Point, z int16) bool {
    return e.cellNswe(cell, z)&nsweAll == 0
}

// antiCornerCut is the checkNearestNsweAntiCornerCut port: a diagonal step
// needs both flanking cells to allow the crossing, a cardinal step only checks
// the source wall.
func (e *Engine) antiCornerCut(geoX, geoY int32, z int16, nswe uint8) bool {
    const (
        nsweEast  uint8 = 1 << 0
        nsweWest  uint8 = 1 << 1
        nsweSouth uint8 = 1 << 2
        nsweNorth uint8 = 1 << 3
    )
    canMove := true
    if nswe&nsweNorth|nswe&nsweEast == nsweNorth|nsweEast {
        canMove = e.nsweAllows(Point{X: geoX, Y: geoY - 1}, z, nsweEast) &&
            e.nsweAllows(Point{X: geoX + 1, Y: geoY}, z, nsweNorth)
    }
    if canMove && nswe&nsweNorth|nswe&nsweWest == nsweNorth|nsweWest {
        canMove = e.nsweAllows(Point{X: geoX, Y: geoY - 1}, z, nsweWest) &&
            e.nsweAllows(Point{X: geoX - 1, Y: geoY}, z, nsweNorth)
    }
    if canMove && nswe&nsweSouth|nswe&nsweEast == nsweSouth|nsweEast {
        canMove = e.nsweAllows(Point{X: geoX, Y: geoY + 1}, z, nsweEast) &&
            e.nsweAllows(Point{X: geoX + 1, Y: geoY}, z, nsweSouth)
    }
    if canMove && nswe&nsweSouth|nswe&nsweWest == nsweSouth|nsweWest {
        canMove = e.nsweAllows(Point{X: geoX, Y: geoY + 1}, z, nsweWest) &&
            e.nsweAllows(Point{X: geoX - 1, Y: geoY}, z, nsweSouth)
    }

    return canMove && e.nsweAllows(Point{X: geoX, Y: geoY}, z, nswe)
}

// neighbourLayerNear is the hasNeighbourLayerNear port: the step-over escape of
// the height limit - a cell whose own nearest layer sits far above still
// continues the surface when a neighbour has a layer at the running height.
func (e *Engine) neighbourLayerNear(
    geoX, geoY int32, z int16, tolerance int16,
) bool {
    deltas := [...][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
    for _, d := range deltas {
        cell := Point{X: geoX + d[0], Y: geoY + d[1]}
        entry, err := e.entry(CellToRegion(cell))
        if err != nil {
            continue
        }
        layer, ok := entry.region.ClosestLayer(LocalCell(cell), z)
        if ok && clickAbs(layer.Height-z) <= tolerance {
            return true
        }
    }

    return false
}

func clickAbs(v int16) int16 {
    if v < 0 {
        return -v
    }

    return v
}
