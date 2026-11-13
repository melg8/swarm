// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import "math"

// The capsule clearance post-pass: the planned waypoints of both engines may
// sit closer to a wall than the character's collision cylinder allows, so this
// file pushes the interior waypoints clear and bends the grazing segments
// around the walls. The server never checks the capsule (its movement
// validation is cell level), so the clearance is the planner's own job
// (docs/pathfinding.md, the capsule clearance section).

// DefaultCollisionRadius is the collision radius of the C1 player capsule: the
// elven fighter template carries 7.5 units
// (dist/game/data/stats/players/templates/StartingClass/ElvenFighter.xml,
// collisionMale radius). The 16 unit geodata cell fits the capsule with half a
// unit of slack.
const DefaultCollisionRadius = 7.5

// capsuleMaxDistance caps the reported clearance: every wall edge outside the
// 3x3 cell neighborhood is at least a full 16 unit cell away, so the cap only
// marks "no wall nearby" and any radius below 16 is answered exactly.
const capsuleMaxDistance = 32.0

// The post pass tunables: the 4 unit sample cannot step over a tight spot at
// the 16 unit cell granularity, the push headroom over the radius, the anchor
// spacing and cap of a bend chain, and the push steps, damping and bend
// recursion depth.
const (
    capsuleSampleStep    = 4.0
    capsulePushHeadroom  = 0.5
    capsuleAnchorSpacing = 16.0
    capsuleMaxAnchors    = 64
    capsuleMaxPushSteps  = 12
    capsulePushDamping   = 0.75
    capsuleMaxBendDepth  = 4
)

// Capsule answers wall clearance questions for one engine's geodata and is safe
// for concurrent use (every method only reads the engine's parsed regions under
// the engine cache lock).
type Capsule struct {
    engine *Engine
}

func NewCapsule(engine *Engine) *Capsule {
    return &Capsule{engine: engine}
}

// SetCapsuleClearance arms the engine side post-pass: every search of this
// engine runs its smoothed waypoints through the capsule clearance pass with
// the given radius. A radius of zero (the default) disables the pass and the
// engine answers the raw smoothed waypoints.
func (e *Engine) SetCapsuleClearance(radius float64) {
    e.capsuleRadius = radius
}

// CapsuleRadius returns the armed clearance radius of the engine (zero when the
// post-pass is disabled).
func (e *Engine) CapsuleRadius() float64 {
    return e.capsuleRadius
}

// capsuleObstacle is the nearest wall edge of a probe: the distance, the
// closest point on the edge and the world center of the cell that declared the
// wall closed (the push direction when the probe sits exactly on the edge).
type capsuleObstacle struct {
    dist   float64
    px, py float64
    cx, cy float64
}

// Clearance returns the distance from the point to the nearest closed wall edge
// of the walkable layer nearest refZ among the cells of the 3x3 neighborhood; a
// point without geodata under it or without any wall within one cell answers
// capsuleMaxDistance.
func (c *Capsule) Clearance(x, y float64, refZ int16) float64 {
    return c.nearestWall(x, y, refZ).dist
}

func (c *Capsule) nearestWall(x, y float64, refZ int16) capsuleObstacle {
    var best capsuleObstacle
    best.dist = capsuleMaxDistance
    cell := WorldToCell(x, y)
    key := CellToRegion(cell)
    entry, err := c.engine.entry(key)
    if err != nil || entry.region == nil {
        return best
    }
    local := LocalCell(cell)
    stack := make([]Layer, 0, 8)
    for dx := -1; dx <= 1; dx++ {
        for dy := -1; dy <= 1; dy++ {
            nx := int(local.X) + dx
            ny := int(local.Y) + dy
            if nx < 0 || ny < 0 ||
                nx >= cellsPerRegionSide || ny >= cellsPerRegionSide {
                continue
            }
            stack = entry.region.LayerStack(
                Point{X: int32(nx), Y: int32(ny)}, stack[:0])
            if len(stack) == 0 {
                continue
            }
            layer := closestLayerOf(stack, refZ)
            best = mergeCellWalls(best, x, y, key, nx, ny, layer)
        }
    }

    return best
}

// closestLayerOf picks the layer of a cell stack whose height is closest to z
// (the Region.ClosestLayer semantics without the span unpacking).
func closestLayerOf(stack []Layer, z int16) Layer {
    best := stack[0]
    bestDelta := heightDelta(best.Height, z)
    for _, layer := range stack[1:] {
        if delta := heightDelta(layer.Height, z); delta < bestDelta {
            best, bestDelta = layer, delta
        }
    }

    return best
}

// mergeCellWalls folds the closed wall edges of one cell into the best obstacle
// tracking; the wall directions follow the search semantics (north is -y, south
// +y, east +x, west -x).
func mergeCellWalls(best capsuleObstacle, x, y float64,
    key RegionKey, nx, ny int, layer Layer,
) capsuleObstacle {
    global := Point{
        X: int32(key.Col)*cellsPerRegionSide + int32(nx),
        Y: int32(key.Row)*cellsPerRegionSide + int32(ny),
    }
    minCorner := CellToWorldMin(global)
    x0, y0 := minCorner.X, minCorner.Y
    x1, y1 := x0+cellSize, y0+cellSize
    centerX, centerY := x0+cellSize/2, y0+cellSize/2
    // wallSegment folds one closed edge into the tracker.
    wall := func(ax, ay, bx, by float64) {
        d, px, py := segmentDistance(x, y, ax, ay, bx, by)
        if d < best.dist {
            best = capsuleObstacle{
                dist: d, px: px, py: py, cx: centerX, cy: centerY,
            }
        }
    }
    if !layer.IsWestOpen() {
        wall(x0, y0, x0, y1)
    }
    if !layer.IsEastOpen() {
        wall(x1, y0, x1, y1)
    }
    if !layer.IsNorthOpen() {
        wall(x0, y0, x1, y0)
    }
    if !layer.IsSouthOpen() {
        wall(x0, y1, x1, y1)
    }

    return best
}

// segmentDistance is the distance of the point to the segment with the closest
// point on it.
func segmentDistance(x, y, ax, ay, bx, by float64,
) (float64, float64, float64) {
    dx, dy := bx-ax, by-ay
    if dx == 0 && dy == 0 {
        ex, ey := x-ax, y-ay

        return math.Hypot(ex, ey), ax, ay
    }
    t := ((x-ax)*dx + (y-ay)*dy) / (dx*dx + dy*dy)
    t = math.Max(0, math.Min(1, t))
    px, py := ax+t*dx, ay+t*dy

    return math.Hypot(x-px, y-py), px, py
}

// SegmentClear answers whether the straight segment walks the server accurate
// grid safely: the line of sight the movement channel applies passes end to end
// and every sampled point keeps the radius from the nearest closed wall edge.
// The mesh wall spans of the shortcut pass are the side level approximation;
// this oracle is the authority the grid movement validation enforces.
func (c *Capsule) SegmentClear(ax, ay, az, bx, by, bz, radius float64) bool {
    if c == nil || c.engine == nil {
        return true
    }

    return c.engine.segmentClearCells(Vec3{X: ax, Y: ay, Z: az},
        Vec3{X: bx, Y: by, Z: bz}, radius,
        c.engine.MaxPassableHeight())
}

// ShortenPath folds the waypoint path into the longest segments the grid wall
// oracle allows (greedy farthest visible over the ordered points): every
// surviving segment answers SegmentClear, the first and the last waypoints
// never move, and the scan window of 32 caps the merge horizon per anchor. The
// server move clamps split here too: the water moves at the 700 unit sphere,
// the land moves through the server's own getValidLocation truncation.
func (c *Capsule) ShortenPath(waypoints []Vec3, radius float64,
) []Vec3 {
    if c == nil || c.engine == nil || len(waypoints) < 3 {
        return waypoints
    }
    const window = 32
    out := make([]Vec3, 0, len(waypoints))
    out = append(out, waypoints[0])
    anchor := waypoints[0]
    index := 0
    for index < len(waypoints)-1 {
        far := index + 1
        if last := len(waypoints) - 1; far+window < last {
            far += window
        } else {
            far = last
        }
        chosen := index + 1
        for k := far; k > index+1; k-- {
            if c.SegmentClear(anchor.X, anchor.Y, anchor.Z,
                waypoints[k].X, waypoints[k].Y, waypoints[k].Z,
                radius) {
                chosen = k

                break
            }
        }
        target := waypoints[chosen]
        if limit, capped := c.segmentLimit(anchor); capped {
            segment := math.Hypot(target.X-anchor.X, target.Y-anchor.Y)
            if segment > limit {
                // The server would truncate this move: the fold cuts it at
                // the same distance, so the aim is the point the character
                // actually reaches.
                split := c.snapZ(
                    segmentPoint(anchor, target, limit/segment), anchor.Z)
                out = append(out, split)
                anchor = split
                // The index holds: the fold resumes from the split point
                // over the same horizon.
                continue
            }
        }
        out = append(out, target)
        anchor = target
        index = chosen
    }

    return out
}

// waterMoveSegment mirrors the server clamp of the water moves: the
// moveToLocation of the game server scales the destination of every swimming
// move request onto the 700 unit sphere around the current position
// (Creature.moveToLocation, the isInWater divider), so the fold splits the
// water anchored segments at the clamp instead of aiming past it.
const waterMoveSegment = 700.0

// segmentLimit answers the server move clamp of a walk that leaves the point: a
// water anchored segment truncates at the water move limit, a dry segment
// answers no cap (the land moves run the server's own truncation).
func (c *Capsule) segmentLimit(from Vec3) (float64, bool) {
    if c.engine.OverWater(from.X, from.Y, int16(from.Z)) {
        return waterMoveSegment, true
    }

    return 0, false
}

// ApplyPath returns the waypoint path with the capsule clearance enforced:
// every interior waypoint clears the walls by the radius and every segment
// passes no closer than the radius to a wall edge. The first and the last
// waypoints never move, and every adjustment is validated against the engine's
// own walk rules and falls back to the original geometry when the validation
// refuses it, so the answer can only add clearance.
func (c *Capsule) ApplyPath(waypoints []Vec3, radius float64) []Vec3 {
    if c == nil || c.engine == nil || len(waypoints) < 2 || radius <= 0 {
        return waypoints
    }
    out := make([]Vec3, len(waypoints))
    copy(out, waypoints)
    // Phase A: the interior waypoints off the walls.
    for i := 1; i < len(out)-1; i++ {
        waypoint := out[i]
        if c.Clearance(waypoint.X, waypoint.Y, int16(waypoint.Z)) >= radius {
            continue
        }
        pushed, ok := c.pushClear(waypoint, radius, 16)
        if !ok {
            continue
        }
        if c.segmentWalkable(out[i-1], pushed) &&
            c.segmentWalkable(pushed, out[i+1]) {
            out[i] = c.snapZ(pushed, waypoint.Z)
        }
    }
    // Phase B: the segments around the walls.
    result := make([]Vec3, 0, len(out)+4)
    result = append(result, out[0])
    for i := 1; i < len(out); i++ {
        result = append(result, c.fixSegment(result[len(result)-1], out[i],
            radius, 0)...)
    }

    return result
}

// pushClear moves the point away from the nearest wall until its clearance
// reaches the radius: the projection follows the current nearest obstacle (the
// closest point on the wall edge, or the declaring cell center when the point
// sits on the edge), the damped step converges onto the clearance ridge between
// two walls instead of oscillating, and a point that never clears within
// maxTravel answers false.
func (c *Capsule) pushClear(point Vec3, radius float64,
    maxTravel float64,
) (Vec3, bool) {
    current := point
    travel := 0.0
    damp := 1.0
    for range capsuleMaxPushSteps {
        obstacle := c.nearestWall(current.X, current.Y, int16(point.Z))
        if obstacle.dist >= radius {
            return current, true
        }
        nx := current.X - obstacle.px
        ny := current.Y - obstacle.py
        if math.Hypot(nx, ny) < 1e-6 {
            nx = current.X - obstacle.cx
            ny = current.Y - obstacle.cy
        }
        length := math.Hypot(nx, ny)
        if length < 1e-6 {
            break
        }
        move := (radius - obstacle.dist + 0.5) * damp
        nx, ny = nx/length*move, ny/length*move
        if travel+math.Hypot(nx, ny) > maxTravel {
            break
        }
        current = Vec3{X: current.X + nx, Y: current.Y + ny, Z: current.Z}
        travel += math.Hypot(nx, ny)
        damp *= capsulePushDamping
    }

    return point, false
}

// fixSegment walks the segment from a to b and bends it around the walls: every
// tight sample becomes a pushed-in anchor, the anchor chain replaces the
// straight segment and the chain segments recurse. A segment that clears the
// radius passes through unchanged, and a chain the walk rules refuse falls back
// to the original straight segment.
func (c *Capsule) fixSegment(a, b Vec3, radius float64, depth int) []Vec3 {
    anchors := c.bendAnchors(a, b, radius)
    if len(anchors) == 0 {
        return []Vec3{b}
    }
    chain := make([]Vec3, 0, len(anchors)+2)
    chain = append(chain, a)
    chain = append(chain, anchors...)
    chain = append(chain, b)
    result := make([]Vec3, 0, len(chain))
    current := chain[0]
    for _, next := range chain[1:] {
        if !c.segmentWalkable(current, next) {
            return []Vec3{b}
        }
        if depth < capsuleMaxBendDepth {
            result = append(result, c.fixSegment(current, next, radius,
                depth+1)...)
        } else {
            result = append(result, next)
        }
        current = next
    }

    return result
}

// bendAnchors samples the segment at capsuleSampleStep and lifts every tight
// sample out of the wall danger zone: the push follows the segment
// perpendicular first (away from a wall the segment runs along, around the end
// of a face it runs into), falls back to the nearest wall projection and snaps
// back onto the geodata surface. The anchors keep a minimum spacing and a per
// segment cap, so a long wall-parallel segment bends at a handful of ridge
// points.
func (c *Capsule) bendAnchors(a, b Vec3, radius float64) []Vec3 {
    length := math.Hypot(b.X-a.X, b.Y-a.Y)
    samples := int(length/capsuleSampleStep) + 1
    if samples < 2 {
        samples = 2
    }
    nx := -(b.Y - a.Y) / length
    ny := (b.X - a.X) / length
    target := radius + capsulePushHeadroom
    anchors := make([]Vec3, 0, 4)
    var last Vec3
    for i := 1; i < samples; i++ {
        t := float64(i) / float64(samples)
        sample := segmentPoint(a, b, t)
        if c.Clearance(sample.X, sample.Y, int16(sample.Z)) >= radius {
            continue
        }
        if len(anchors) >= capsuleMaxAnchors {
            break
        }
        pushed, ok := c.pushDirected(sample, nx, ny, target)
        if !ok || !c.segmentWalkable(sample, pushed) {
            pushed, ok = c.pushDirected(sample, -nx, -ny, target)
        }
        if !ok || !c.segmentWalkable(sample, pushed) {
            pushed, ok = c.pushClear(sample, target, 32)
        }
        if !ok || !c.segmentWalkable(sample, pushed) {
            continue
        }
        pushed = c.snapZ(pushed, sample.Z)
        if len(anchors) > 0 &&
            math.Hypot(pushed.X-last.X, pushed.Y-last.Y) <
                capsuleAnchorSpacing {
            continue
        }
        anchors = append(anchors, pushed)
        last = pushed
    }

    return anchors
}

// pushDirected marches the point along the unit direction until the clearance
// reaches the target (bounded travel) and answers the first clearing point.
func (c *Capsule) pushDirected(point Vec3, nx, ny, target float64,
) (Vec3, bool) {
    length := math.Hypot(nx, ny)
    if length < 1e-9 {
        return point, false
    }
    nx, ny = nx/length, ny/length
    for travel := 2.0; travel <= 32; travel += 2.0 {
        probe := Vec3{
            X: point.X + nx*travel, Y: point.Y + ny*travel, Z: point.Z,
        }
        if c.Clearance(probe.X, probe.Y, int16(point.Z)) >= target {
            return probe, true
        }
    }

    return point, false
}

// segmentPoint interpolates the segment at t, the height included.
func segmentPoint(a, b Vec3, t float64) Vec3 {
    return Vec3{
        X: a.X + (b.X-a.X)*t,
        Y: a.Y + (b.Y-a.Y)*t,
        Z: a.Z + (b.Z-a.Z)*t,
    }
}

// segmentWalkable answers the engine line of sight for one adjusted segment;
// an error (no geodata under an endpoint) reads as not walkable, so the caller
// keeps the original geometry.
func (c *Capsule) segmentWalkable(a, b Vec3) bool {
    ok, err := c.engine.LineOfSight(a, b, c.engine.MaxPassableHeight())

    return err == nil && ok
}

// snapZ puts a moved waypoint back onto the geodata surface: the height of the
// layer closest to the reference z at the new cell.
func (c *Capsule) snapZ(point Vec3, refZ float64) Vec3 {
    height, err := c.engine.ClosestHeight(point.X, point.Y, int16(refZ))
    if err != nil {
        return point
    }
    point.Z = float64(height)

    return point
}
