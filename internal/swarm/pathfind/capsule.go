// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import "math"

// The capsule clearance post-pass. The planned waypoints of both
// engines (the grid A* smoothing and the mesh funnel) may sit closer
// to a wall than the character's collision cylinder allows: the funnel
// pivots at the portal span ends - exactly on the wall boundary - and
// a smoothed leg can graze a wall corner while staying inside open
// cells. A character that walks such a plan clips every wall edge and
// corner with its capsule and sticks (the owner report: the path
// points "too close to the wall edges and corners").
//
// The server itself never checks the capsule against the geodata (the
// Mobius movement validation is cell level: the paired NSWE walls and
// the anti corner cut), so keeping the clearance is the planner's own
// job. This file implements it as a geometric post-pass over the
// planned waypoints:
//
//  1. the clearance of a point is the distance to the nearest closed
//     wall edge of the walkable layer nearest its reference z, exact
//     within one cell (the 3x3 neighborhood holds every wall edge
//     closer than 16 units; cell edges further away cannot matter for
//     a radius below 16),
//  2. every interior waypoint whose clearance falls below the radius
//     is pushed away from the nearest wall until it clears it (the
//     move is validated against the engine's own walk rules and
//     dropped when the validation refuses it),
//  3. every leg is sampled; a leg that grazes a wall is bent around
//     the tightest spot at a pushed-in anchor point (bounded depth,
//     the original leg stays when no valid bend exists).
//
// The first and the last waypoint never move: the start is where the
// character actually stands and the end is the destination the caller
// asked for (the approach contracts measure their distance to it).

// DefaultCollisionRadius is the collision radius of the C1 player
// capsule: the elven fighter templates carry 7.5 units
// (dist/game/data/stats/players/templates/StartingClass/ElvenFighter.xml,
// collisionMale radius - the character class the bot creates). The
// geodata cell is 16 units wide, so the capsule fits one cell with
// half a unit of slack: a cell center waypoint always clears the
// walls of its own cell, a funnel span end never does.
const DefaultCollisionRadius = 7.5

// capsuleMaxDistance caps the reported clearance. Every wall edge
// outside the 3x3 cell neighborhood of the probe is at least one full
// cell (16 units) away, so the cap only marks "no wall nearby"; any
// clearance question for a radius below 16 is answered exactly.
const capsuleMaxDistance = 32.0

// The post-pass tunables: the leg sampling step (the clearance field
// changes at the 16 unit cell granularity, a 4 unit sample cannot
// step over a tight spot), the push headroom over the radius, the
// minimum anchor spacing of a bend chain and the bend recursion
// depth.
const (
    capsuleSampleStep    = 4.0
    capsulePushHeadroom  = 0.5
    capsuleAnchorSpacing = 16.0
    capsuleMaxAnchors    = 64
    capsuleMaxPushSteps  = 12
    capsulePushDamping   = 0.75
    capsuleMaxBendDepth  = 4
)

// Capsule answers wall clearance questions for one engine's geodata.
// It is safe for concurrent use (every method only reads the engine's
// parsed regions under the engine cache lock).
type Capsule struct {
    engine *Engine
}

// NewCapsule creates the clearance probe over an engine.
func NewCapsule(engine *Engine) *Capsule {
    return &Capsule{engine: engine}
}

// SetCapsuleClearance arms the engine side post-pass: every search of
// this engine runs its smoothed waypoints through the capsule
// clearance pass with the given radius. A radius of zero (the
// default) disables the pass - the engine answers the raw smoothed
// waypoints, the contract the existing tests and the acceptance
// scenarios pin.
func (e *Engine) SetCapsuleClearance(radius float64) {
    e.capsuleRadius = radius
}

// CapsuleRadius returns the armed clearance radius of the engine
// (zero when the post-pass is disabled).
func (e *Engine) CapsuleRadius() float64 {
    return e.capsuleRadius
}

// capsuleObstacle is the nearest wall edge of a probe: the distance to
// it, the closest point on the edge and the world center of the cell
// that declared the wall closed (the open side the character stands
// on - the push direction when the probe sits exactly on the edge).
type capsuleObstacle struct {
    dist   float64
    px, py float64
    cx, cy float64
}

// Clearance returns the distance from the point to the nearest closed
// wall edge of the walkable layer nearest refZ, among the cells of
// the 3x3 neighborhood. A point without geodata under it or without
// any wall within one cell answers capsuleMaxDistance.
func (c *Capsule) Clearance(x, y float64, refZ int16) float64 {
    return c.nearestWall(x, y, refZ).dist
}

// nearestWall resolves the closest wall obstacle of the point.
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

// closestLayerOf picks the layer of a cell stack whose height is the
// closest to z (the Region.ClosestLayer semantics without the span
// unpacking, the stack is already resolved).
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

// mergeCellWalls folds the closed wall edges of one cell into the
// best obstacle tracking. The wall directions follow the search
// semantics: north is -y, south +y, east +x, west -x.
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

// segmentDistance is the distance of the point to the segment with
// the closest point on it.
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

// LegClear answers whether the straight leg walks the server accurate
// grid safely: the line of sight the movement channel applies (the
// supercover raster with the strict symmetric height rule) passes end
// to end and every sampled point of the leg keeps the radius from the
// nearest closed wall edge (the 4 unit sample of the bend pass - the
// wall field changes at the 16 unit cell granularity, a 4 unit sample
// cannot step over a tight spot). The mesh wall spans of the shortcut
// pass are the side level approximation; this oracle is the authority
// the grid movement validation enforces.
func (c *Capsule) LegClear(ax, ay, az, bx, by, bz, radius float64) bool {
    if c == nil || c.engine == nil {
        return true
    }
    a := Vec3{X: ax, Y: ay, Z: az}
    b := Vec3{X: bx, Y: by, Z: bz}
    if !c.legWalkable(a, b) {
        return false
    }
    if radius <= 0 {
        return true
    }
    length := math.Hypot(b.X-a.X, b.Y-a.Y)
    samples := int(length/capsuleSampleStep) + 1
    if samples < 2 {
        samples = 2
    }
    for i := 1; i < samples; i++ {
        sample := legPoint(a, b, float64(i)/float64(samples))
        if c.Clearance(sample.X, sample.Y, int16(sample.Z)) < radius {
            return false
        }
    }

    return true
}

// ShortenPath folds the waypoint path into the longest legs the grid
// wall oracle allows (greedy farthest visible over the ordered
// points): every surviving leg answers LegClear - the server walk
// rules end to end and the capsule radius off every sampled wall -
// and the server move clamps (the 9900 unit packet refusal is the
// walker's own maxMoveLeg concern, the water clamp of the swimming
// moves splits here). The first and the last waypoints never move.
// The scan window caps the merge horizon per anchor: the wall bends
// chain every handful of points, a longer chord beyond the window is
// rare enough to leave unexplored.
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
            if c.LegClear(anchor.X, anchor.Y, anchor.Z,
                waypoints[k].X, waypoints[k].Y, waypoints[k].Z,
                radius) {
                chosen = k

                break
            }
        }
        target := waypoints[chosen]
        if limit, capped := c.legLimit(anchor); capped {
            leg := math.Hypot(target.X-anchor.X, target.Y-anchor.Y)
            if leg > limit {
                // The server would truncate this move on its own:
                // the fold cuts it at the same distance first, so
                // the waypoint the walker aims at is the waypoint
                // the character actually reaches.
                split := c.snapZ(
                    legPoint(anchor, target, limit/leg), anchor.Z)
                out = append(out, split)
                anchor = split
                // The index holds: the fold resumes from the split
                // point over the same horizon and re-answers the
                // remainder of the capped chord.
                continue
            }
        }
        out = append(out, target)
        anchor = target
        index = chosen
    }

    return out
}

// waterMoveLeg mirrors the server clamp of the water moves: the
// moveToLocation of the game server scales the destination of every
// swimming move request onto the 700 unit sphere around the current
// position (Creature.moveToLocation - the isInWater divider), and a
// target beyond it never answers. The smoothing over open water
// merges the funnel pinholes into legs several times the clamp - the
// server would stop the character short of every such waypoint and
// the follower would never see the arrival (the path "is not
// passed", the owner report). The fold splits the water anchored
// legs at the clamp instead.
const waterMoveLeg = 700.0

// legLimit answers the server move clamp of a walk that leaves the
// point. A water anchored leg truncates at the water move limit: the
// character issues the click from the water, the server clamps the
// destination. A dry leg answers no cap - the land moves run the
// server's own getValidLocation truncation and pathfinding instead,
// nothing the planner has to pre-split.
func (c *Capsule) legLimit(from Vec3) (float64, bool) {
    if c.engine.OverWater(from.X, from.Y, int16(from.Z)) {
        return waterMoveLeg, true
    }

    return 0, false
}

// ApplyPath returns the waypoint path with the capsule clearance
// enforced: every interior waypoint clears the walls by the radius
// and every leg passes no closer than the radius to a wall edge. The
// first and the last waypoints never move. Every adjustment is
// validated against the engine's own walk rules (the line of sight
// the server movement channel applies) and falls back to the
// original geometry when the validation refuses it, so the answer is
// never worse than the input plan - it can only add clearance.
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
        if c.legWalkable(out[i-1], pushed) &&
            c.legWalkable(pushed, out[i+1]) {
            out[i] = c.snapZ(pushed, waypoint.Z)
        }
    }
    // Phase B: the legs around the walls.
    result := make([]Vec3, 0, len(out)+4)
    result = append(result, out[0])
    for i := 1; i < len(out); i++ {
        result = append(result, c.fixLeg(result[len(result)-1], out[i],
            radius, 0)...)
    }

    return result
}

// pushClear moves the point away from the nearest wall until its
// clearance reaches the radius. The projection direction follows the
// current nearest obstacle (the closest point on the wall edge, or
// the declaring cell center when the point sits on the edge); the
// step length damps every iteration, so a point squeezed between two
// walls (a corridor corner) converges onto the clearance ridge
// instead of oscillating between the two constraint normals. The
// travel is bounded by maxTravel; a point that never clears the
// radius answers false.
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

// fixLeg walks the leg from a to b and bends it around the walls:
// every sampled point whose clearance falls below the radius becomes
// a pushed-in anchor (the clearance projection), the anchor chain
// replaces the straight leg and the chain legs recurse. A leg that
// clears the radius passes through unchanged; a chain the walk rules
// refuse falls back to the original straight leg - the original plan
// is the honest fallback, never a broken one.
func (c *Capsule) fixLeg(a, b Vec3, radius float64, depth int) []Vec3 {
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
        if !c.legWalkable(current, next) {
            return []Vec3{b}
        }
        if depth < capsuleMaxBendDepth {
            result = append(result, c.fixLeg(current, next, radius,
                depth+1)...)
        } else {
            result = append(result, next)
        }
        current = next
    }

    return result
}

// bendAnchors samples the leg at capsuleSampleStep and lifts every
// tight sample out of the wall danger zone. The push follows the leg
// perpendicular first (the detour direction - away from a wall the
// leg runs along, around the end of a face it runs into), falls back
// to the nearest wall projection, and every candidate is snapped back
// onto the geodata surface. The anchors keep a minimum spacing so a
// long wall-parallel leg bends at a handful of ridge points instead
// of one waypoint per sample, and the count is capped per leg.
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
        sample := legPoint(a, b, t)
        if c.Clearance(sample.X, sample.Y, int16(sample.Z)) >= radius {
            continue
        }
        if len(anchors) >= capsuleMaxAnchors {
            break
        }
        pushed, ok := c.pushDirected(sample, nx, ny, target)
        if !ok || !c.legWalkable(sample, pushed) {
            pushed, ok = c.pushDirected(sample, -nx, -ny, target)
        }
        if !ok || !c.legWalkable(sample, pushed) {
            pushed, ok = c.pushClear(sample, target, 32)
        }
        if !ok || !c.legWalkable(sample, pushed) {
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

// pushDirected marches the point along the unit direction until the
// clearance reaches the target (bounded travel). It answers the
// first point that clears; false when none does within the bound.
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

// legPoint interpolates the leg at t, the height included.
func legPoint(a, b Vec3, t float64) Vec3 {
    return Vec3{
        X: a.X + (b.X-a.X)*t,
        Y: a.Y + (b.Y-a.Y)*t,
        Z: a.Z + (b.Z-a.Z)*t,
    }
}

// legWalkable answers the engine line of sight for one adjusted leg:
// the walk rules the server movement channel applies (the supercover
// raster with the strict symmetric height rule). An error (no geodata
// under an endpoint) reads as not walkable - the caller keeps the
// original geometry then.
func (c *Capsule) legWalkable(a, b Vec3) bool {
    ok, err := c.engine.LineOfSight(a, b, c.engine.MaxPassableHeight())

    return err == nil && ok
}

// snapZ puts a moved waypoint back onto the geodata surface: the
// height of the layer closest to the reference z at the new cell.
func (c *Capsule) snapZ(point Vec3, refZ float64) Vec3 {
    height, err := c.engine.ClosestHeight(point.X, point.Y, int16(refZ))
    if err != nil {
        return point
    }
    point.Z = float64(height)

    return point
}
