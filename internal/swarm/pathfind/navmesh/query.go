// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
)

// Pos is a world position: X and Y are the horizontal world axes, Z
// is the height (the same field semantics as pathfind.Vec3, kept as
// a leaf-package local type).
type Pos struct {
    X, Y, Z float64
}

// Filter prices the areas of a search.
type Filter struct {
    // WaterCost multiplies the step cost of the water polygons (the
    // swim pricing - the grid engine waterCostMultiplier of 3).
    WaterCost float64
    // AllowWater keeps the water polygons walkable; a false value
    // walls them (the dry searches of the hunt loop).
    AllowWater bool
    // Avoid carries the recovery bans of the hunt loop (see
    // AvoidCircle): every polygon whose footprint a ban touches
    // walls the search - except the escape polygons of the ban that
    // holds the start (the way-out rule of the grid engine, priced
    // at the avoidEscapeMultiplier). A nil slice bans nothing.
    Avoid []AvoidCircle
    // WaypointClearance pulls the funnel pivots inward from the
    // portal span ends by this radius before the string pulling: the
    // span ends sit on the wall boundary - the exact Detour pivot is
    // where a character capsule clips the corner - and the turn
    // happens a capsule radius away from it instead. A span narrower
    // than twice the radius pivots at its middle (the deepest point
    // of a narrow doorway). Zero keeps the exact pivots.
    WaypointClearance float64
}

// DefaultFilter is the swim allowing search with the 3x water cost.
func DefaultFilter() Filter {
    return Filter{WaterCost: 3, AllowWater: true, Avoid: nil}
}

// DryFilter walls the water polygons: a route only exists over dry
// ground, an unreachable dry target answers the partial corridor to
// the closest reachable dry point.
func DryFilter() Filter {
    return Filter{WaterCost: 3, AllowWater: false, Avoid: nil}
}

// escapeWaterCost prices the water polygons of the escape search (the
// research round measured the 8x flood as the honest way out).
const escapeWaterCost = 8

// nearestHalfXZ/nearestHalfZ are the query extents of the nearest
// polygon resolution: two cells of horizontal slack around the query
// point and the stacked-layer disambiguation window vertically.
const (
    nearestHalfXZ = 64
    nearestHalfZ  = 600
)

// Route is one navigation answer: the funnel waypoints the walker
// follows, the polygon corridor they never leave and the search
// statistics.
type Route struct {
    // Found reports whether the requested destination was reached.
    Found bool
    // Partial reports the closest-reachable answer of a search whose
    // destination is unreachable under the filter (the dry search of
    // a swim-only target): the waypoints then end at the closest
    // reachable point, the corridor is the walkable part of it.
    Partial   bool
    Waypoints []Pos
    Corridor  []PolyRef
    Explored  int
}

// Route searches the walkable route from start to end under the
// filter: the exact destination contract (Found only when the
// destination polygon itself is reached, the partial answer carries
// the closest reachable corridor otherwise).
func (m *Mesh) Route(start, end Pos, filter Filter) (*Route, error) {
    return m.RouteApproach(start, end, 0, filter)
}

// RouteApproach searches the walkable route from start to end
// succeeding on the first polygon whose closest surface point lies
// within the approach radius (3D) of the end position - the
// FindPathApproach contract of the grid engine: the merchant stops
// hold their interaction distance, a destination behind a counter or
// on a floor layer the mesh does not model is still reached on the
// surrounding deck. A radius of zero degenerates to Route. The start
// and end resolve onto the mesh through the 3D nearest polygon (the
// stacked-layer disambiguation: a point under a bridge binds to the
// water polygon, the same x/y at deck height to the bridge polygon).
// A missing mesh under either endpoint answers NoNavmeshError; an
// unreachable destination answers Found=false with Partial set when
// a closest-reachable corridor exists.
func (m *Mesh) RouteApproach(
    start, end Pos, approachRadius float64, filter Filter,
) (*Route, error) {
    startRef, startPos, ok := m.FindNearestPoly(start)
    if !ok {
        return nil, wrapNoNavmesh(start)
    }
    endRef, endPos, ok := m.FindNearestPoly(end)
    if !ok {
        return nil, wrapNoNavmesh(end)
    }

    route := &Route{
        Found:     false,
        Partial:   false,
        Waypoints: nil,
        Corridor:  nil,
        Explored:  0,
    }
    state := m.acquireState()
    defer m.releaseState(state)

    if startRef == endRef {
        route.Corridor = []PolyRef{startRef}
        route.Waypoints = m.straightPath(route.Corridor, startPos,
            endPos, filter.WaypointClearance)
        route.Found = true

        return route, nil
    }

    avoid := newAvoidCtx(filter.Avoid, start)
    result := m.astar(state,
        astarGoal{target: endRef, escape: false, approach: approachRadius},
        startRef, startPos, endPos, filter, avoid)
    route.Explored = result.explored
    route.Corridor = result.corridor
    switch {
    case result.reached:
        route.Found = true
        route.Waypoints = m.straightPath(result.corridor, startPos,
            endPos, filter.WaypointClearance)
    case result.partial && len(result.corridor) > 1:
        route.Partial = true
        // The partial answer funnels toward the original end: the
        // projection onto the last corridor polygon is the closest
        // reachable point of it (the dry search contract).
        route.Waypoints = m.straightPath(result.corridor, startPos,
            endPos, filter.WaypointClearance)
    default:
        route.Corridor = nil
    }

    return route, nil
}

// RouteDry searches the dry route from start to end: the water
// polygons are walls, so a target only swimming reaches answers the
// partial corridor to the closest dry point (the FindPathApproachDry
// contract of the grid engine).
func (m *Mesh) RouteDry(start, end Pos) (*Route, error) {
    return m.Route(start, end, DryFilter())
}

// WaterEscape plans the way out of the water for a position standing
// on a water polygon: the cheapest corridor to the first dry polygon
// with the water priced 8x (the FindWaterEscape contract of the grid
// engine - an ordinary priced search, no dedicated breadth first
// flood). A start already on dry ground answers Found=false.
func (m *Mesh) WaterEscape(start Pos) (*Route, error) {
    startRef, startPos, ok := m.FindNearestPoly(start)
    if !ok {
        return nil, wrapNoNavmesh(start)
    }
    _, poly := m.polyOfRef(startRef)
    if poly == nil {
        return nil, wrapNoNavmesh(start)
    }
    if poly.Area == AreaGround {
        return &Route{
            Found:     false,
            Partial:   false,
            Waypoints: nil,
            Corridor:  nil,
            Explored:  0,
        }, nil
    }

    state := m.acquireState()
    defer m.releaseState(state)
    filter := Filter{
        WaterCost:  escapeWaterCost,
        AllowWater: true,
        Avoid:      nil,
    }
    result := m.astar(state, astarGoal{
        target:   0,
        escape:   true,
        approach: 0,
    }, startRef, startPos, start, filter, noAvoid())

    route := &Route{
        Found:     false,
        Partial:   false,
        Waypoints: nil,
        Corridor:  nil,
        Explored:  result.explored,
    }
    if !result.reached {
        return route, nil
    }
    route.Found = true
    route.Corridor = result.corridor
    // The walk ends at the crossing into the first dry polygon.
    route.Waypoints = m.straightPath(result.corridor, startPos,
        result.end, 0)

    return route, nil
}

// FindNearestPoly returns the polygon whose surface is closest to the
// position in 3D together with the closest surface point - the
// dtNavMeshQuery::findNearestPoly semantics that resolves stacked
// layers by the 3D distance. The query extents give the horizontal
// slack and the vertical window; a position without any polygon under
// it answers false.
func (m *Mesh) FindNearestPoly(pos Pos) (PolyRef, Pos, bool) {
    minX, maxX := pos.X-nearestHalfXZ, pos.X+nearestHalfXZ
    minY, maxY := pos.Y-nearestHalfXZ, pos.Y+nearestHalfXZ
    minZ, maxZ := pos.Z-nearestHalfZ, pos.Z+nearestHalfZ

    bestRef := PolyRef(0)
    bestPos := Pos{}
    bestDist := math.MaxFloat64
    candidates := make([]int32, 0, 32)
    for _, key := range m.regionKeysOfBox(minX, minY, maxX, maxY) {
        tile, err := m.Tile(key)
        if err != nil || tile == nil {
            continue
        }
        candidates = tileQueryPolys(tile, minX, minY, maxX, maxY, minZ,
            maxZ, candidates[:0])
        for _, pi := range candidates {
            poly := &tile.Polys[pi]
            cx, cy, cz := tile.ClosestPoint(poly, pos.X, pos.Y, pos.Z)
            dx := pos.X - cx
            dy := pos.Y - cy
            dz := pos.Z - cz
            dist := dx*dx + dy*dy + dz*dz
            if dist < bestDist {
                bestDist = dist
                bestRef = RefOf(key.Col, key.Row, uint32(pi))
                bestPos = Pos{X: cx, Y: cy, Z: cz}
            }
        }
    }

    return bestRef, bestPos, bestRef != 0
}

// regionKeysOfBox lists the region keys a world box may touch (the
// box spans at most four regions when it sits on a region corner).
func (m *Mesh) regionKeysOfBox(minX, minY, maxX, maxY float64,
) []RegionKey {
    cols := [2]int16{}
    rows := [2]int16{}
    cols[0], rows[0] = RegionOfWorld(minX, minY)
    cols[1], rows[1] = RegionOfWorld(maxX, maxY)
    keys := make([]RegionKey, 0, 4)
    for _, col := range cols {
        for _, row := range rows {
            key := RegionKey{Col: col, Row: row}
            duplicate := false
            for _, existing := range keys {
                if existing == key {
                    duplicate = true

                    break
                }
            }
            if !duplicate {
                keys = append(keys, key)
            }
        }
    }

    return keys
}

// tileQueryPolys walks the bounding volume tree of a tile and
// appends the polygon indices whose quantized bounds overlap the
// world query box (the dtNavMeshQuery::queryPolygonsInTile
// traversal). An empty tree degrades to every polygon.
func tileQueryPolys(tile *Tile, minX, minY, maxX, maxY, minZ, maxZ float64,
    out []int32,
) []int32 {
    if len(tile.BVTree) == 0 {
        for i := range tile.Polys {
            out = append(out, int32(i))
        }

        return out
    }

    var qmin [3]uint16
    var qmax [3]uint16
    lo := [3]float64{minX - tile.worldMinX, minZ - heightFloor,
        minY - tile.worldMinY}
    hi := [3]float64{maxX - tile.worldMinX, maxZ - heightFloor,
        maxY - tile.worldMinY}
    for a := range 3 {
        l := quantizeBV(lo[a])
        h := quantizeBV(hi[a])
        // The even/odd lowest bit keeps overlapping quantized bounds
        // strictly separated (the Detour dtOverlapQuantBounds trick).
        qmin[a] = l & 0xFFFE
        qmax[a] = h | 1
    }

    i := 0
    for i < len(tile.BVTree) {
        node := &tile.BVTree[i]
        overlap := quantOverlap(qmin, qmax, node.BMin, node.BMax)
        leaf := node.I >= 0
        if leaf && overlap {
            out = append(out, node.I)
        }
        if overlap || leaf {
            i++
        } else {
            i += int(-node.I)
        }
    }

    return out
}

// quantizeBV converts a world offset into the quantized BVTree units
// (world units divided by the cell size).
func quantizeBV(world float64) uint16 {
    cells := world / cellSizeWorld
    if cells < 0 {
        return 0
    }
    if cells > 65535 {
        return 65535
    }

    return uint16(cells)
}

// quantOverlap reports whether two quantized bounds overlap.
func quantOverlap(aMin, aMax, bMin, bMax [3]uint16) bool {
    for a := range 3 {
        if aMin[a] > bMax[a] || aMax[a] < bMin[a] {
            return false
        }
    }

    return true
}

// wrapNoNavmesh wraps the missing mesh error with the position.
func wrapNoNavmesh(pos Pos) error {
    return &NoNavmeshError{Pos: pos}
}

// NoNavmeshError reports a position without a usable mesh polygon.
type NoNavmeshError struct {
    Pos Pos
}

// Error implements the error interface.
func (e *NoNavmeshError) Error() string {
    return "no navmesh under the position"
}
