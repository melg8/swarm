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

// LegGuard is the wall oracle of the shortcut pass: the server
// accurate wall question for one straight leg. The mesh wall spans
// are the side level approximation - the grid movement validation
// sees the per cell walls (the paired NSWE walls and the diagonal
// anti corner cut) the rectangle sides lump together. A guard armed
// on the filter (the grid capsule of the caller) answers every
// shortcut chord against the authoritative raster; without one the
// pass falls back to the mesh wall spans.
type LegGuard interface {
    LegClear(ax, ay, az, bx, by, bz, radius float64) bool
}

// WaterZone is one server water zone cuboid (the ZoneCuboid of the
// C1 water.xml data): the x/y box and the water surface the server
// tests the swim state against. The server switches the movement to
// the swim speeds inside these cuboids only (the CreatureStat
// getMoveSpeed water branch reads the ZoneId.WATER flag the
// WaterZone onEnter sets, the ZoneCuboid.isInsideZone tests the
// position against the box) - the swim state of a position is the
// authored zone data, not the water depth.
type WaterZone struct {
    MinX, MaxX float64
    MinY, MaxY float64
    // MinZ is the box bottom of the zone data (kept for the honest
    // table; the coverage test prices by the surface).
    MinZ float64
    // MaxZ is the water surface of the zone (the zone data maxZ); a
    // bed at or below it lies under the zone water body.
    MaxZ float64
}

// Filter prices the areas of a search.
type Filter struct {
    // WaterCost multiplies the step cost of the water polygons (the
    // swim pricing - the measured run/swim speed ratio: the C1
    // templates carry the run speeds 115..125 against the swim 50 of
    // every class, the HumanFighter the bots walk prices the swim at
    // 115/50 = 2.3).
    WaterCost float64
    // WaterZones carries the server water zone cuboids of the world
    // (the C1WaterZones table the webserver route arms): a water
    // polygon whose center a cuboid covers keeps the WaterCost swim
    // price, the water polygons no cuboid covers price at the land
    // rate - the server walks such beds at the plain run speed, the
    // river segments the zone data omits cross for free (the zone
    // data, not the depth, prices the swim). A nil slice prices
    // every water polygon at the WaterCost (the toy worlds and the
    // searches without the zone table).
    WaterZones []WaterZone
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
    // Smooth runs the shortcut pass over the funnel answer (the
    // smoothing): the greedy farthest visible merge walks the corridor
    // and folds the waypoints into the longest chords that cross every
    // intermediate portal inside its open span and keep the
    // WaypointClearance radius from the wall edges of the crossed
    // polygons. It needs the pivot clearance armed - a zero
    // WaypointClearance keeps the raw funnel answer.
    Smooth bool
    // Guard is the optional wall oracle of the shortcut pass (see
    // LegGuard): when armed, every merged chord answers to it instead
    // of the mesh wall spans - the grid raster is the authority the
    // side level spans approximate.
    Guard LegGuard
    // AvoidGrazed prices the foreign banned ground whose own portal
    // segment stays clear of the circle at avoidGrazedMultiplier
    // instead of sealing it (the route shaping contract: the ban
    // bends the route toward the detour, the town moat ring beats the
    // through town walk). The zero value keeps the sealed goal of the
    // recovery bans - the deterministic planner gives up on the
    // banned ground (the hunt loop contract).
    AvoidGrazed bool
}

// DefaultFilter is the swim allowing search with the measured swim
// pricing (the run/swim speed ratio 2.3 of the HumanFighter
// templates, the swim 50 of every class against the run 115..125).
func DefaultFilter() Filter {
    return Filter{
        WaterCost:         2.3,
        AllowWater:        true,
        Avoid:             nil,
        WaypointClearance: 0,
        Smooth:            false,
    }
}

// DryFilter walls the water polygons: a route only exists over dry
// ground, an unreachable dry target answers the partial corridor to
// the closest reachable dry point.
func DryFilter() Filter {
    return Filter{
        WaterCost:         2.3,
        AllowWater:        false,
        Avoid:             nil,
        WaypointClearance: 0,
        Smooth:            false,
    }
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

// Route is one navigation answer: the waypoints the walker follows,
// the polygon corridor they never leave and the search statistics.
type Route struct {
    // Found reports whether the requested destination was reached.
    Found bool
    // Partial reports the closest-reachable answer of a search whose
    // destination is unreachable under the filter (the dry search of
    // a swim-only target): the waypoints then end at the closest
    // reachable point, the corridor is the walkable part of it.
    Partial bool
    // Waypoints is the walk answer: the smoothed funnel when the
    // filter arms the shortcut pass, the raw funnel otherwise.
    Waypoints []Pos
    // RawWaypoints is the unsmoothed funnel answer, populated when
    // the shortcut pass ran (the comparison variant the viewer
    // toggles against); nil keeps "the waypoints are the raw funnel".
    RawWaypoints []Pos
    Corridor     []PolyRef
    Explored     int
    // Hierarchical reports the answer of the cluster level route
    // (docs/navmesh.md, the hierarchy section): the coarse chain
    // search plus the refinement hops instead of one flat search.
    Hierarchical bool
}

// answerWaypoints fills the route waypoints from the corridor: the
// raw funnel answer, smoothed through the shortcut pass when the
// filter arms it (the raw answer rides along for the comparison).
func (m *Mesh) answerWaypoints(route *Route, corridor []PolyRef,
    startPos, endPos Pos, filter Filter,
) {
    wps := m.straightPathWps(corridor, startPos, endPos,
        filter.WaypointClearance)
    if filter.Smooth && filter.WaypointClearance > 0 {
        route.RawWaypoints = funnelPositions(wps)
        route.Waypoints = m.smoothPath(corridor, wps, filter)

        return
    }
    route.Waypoints = funnelPositions(wps)
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
    // The cold path prefetch: the endpoint tiles decode in
    // parallel (the sequential nearest poly loads were the first
    // cold stall; the warm route answers a pair of cache hits).
    startCol, startRow := RegionOfWorld(start.X, start.Y)
    endCol, endRow := RegionOfWorld(end.X, end.Y)
    m.PrefetchTiles([]RegionKey{
        {Col: startCol, Row: startRow},
        {Col: endCol, Row: endRow},
    })

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
        m.answerWaypoints(route, route.Corridor, startPos, endPos,
            filter)
        route.Found = true

        return route, nil
    }

    // The long routes go through the hierarchy: the coarse cluster
    // chain plus the budgeted refinement hops (the flat search over a
    // half world corridor only answers the capped partial). The
    // answer falls back to the flat search when the hierarchy
    // declines. The avoid banned queries skip the hierarchy - the
    // coarse graph cannot see the bans, its chain happily walks the
    // banned ground and the refinement hops strand on the ban edge
    // answering a partial the flat search beats (the world town ban
    // turns the 90.5k through town answer into the 72.9k moat
    // detour).
    if len(filter.Avoid) == 0 && hierWorthy(startRef, endRef, startPos,
        endPos, filter) {
        hierarchical := m.routeHierarchical(startRef, startPos, endRef,
            endPos, approachRadius, filter, state)
        if hierarchical != nil {
            return hierarchical, nil
        }
    }

    avoid := newAvoidCtx(filter.Avoid, start)
    result := m.astar(state,
        astarGoal{target: endRef, escape: false, approach: approachRadius},
        startRef, startPos, endPos, filter, avoid, maxQueryNodes, nil)
    // The capped escalation: a flat search that hit the node budget
    // never saw the target side of the corridor - the hierarchy
    // answers those (the same tile far routes and the same tile
    // unreachable ones keep their flat partial answer, the hierarchy
    // declines or misses there too). The avoid banned queries stay
    // flat (the coarse graph cannot see the bans) - the capped banned
    // query reruns with the raised budget instead: the world town ban
    // diagonal needs ~85k nodes, the 64k budget capped at the river
    // ford and answered a partial the rerun turns into the found
    // detour.
    if result.capped && len(filter.Avoid) == 0 {
        hierarchical := m.routeHierarchical(startRef, startPos, endRef,
            endPos, approachRadius, filter, state)
        if hierarchical != nil && hierarchical.Found {
            return hierarchical, nil
        }
    } else if result.capped {
        result = m.astar(state,
            astarGoal{target: endRef, escape: false,
                approach: approachRadius},
            startRef, startPos, endPos, filter, avoid, maxQueryNodes*8,
            nil)
    }
    route.Explored = result.explored
    route.Corridor = result.corridor
    switch {
    case result.reached:
        route.Found = true
        m.answerWaypoints(route, result.corridor, startPos, endPos,
            filter)
    case result.partial && len(result.corridor) > 1:
        route.Partial = true
        // The partial answer funnels toward the original end: the
        // projection onto the last corridor polygon is the closest
        // reachable point of it (the dry search contract).
        m.answerWaypoints(route, result.corridor, startPos, endPos,
            filter)
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
        WaterCost:         escapeWaterCost,
        AllowWater:        true,
        Avoid:             nil,
        WaypointClearance: 0,
    }
    result := m.astar(state, astarGoal{
        target:   0,
        escape:   true,
        approach: 0,
    }, startRef, startPos, start, filter, noAvoid(), maxQueryNodes,
        nil)

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

// tileQueryPolys walks the spatial index of a tile and appends the
// polygon indices whose bounds overlap the world query box (the
// dtNavMeshQuery::queryPolygonsInTile traversal). The v3 tiles walk
// the bucket grid, the v1/v2 tiles walk the bounding volume tree. An
// empty index degrades to every polygon.
func tileQueryPolys(tile *Tile, minX, minY, maxX, maxY, minZ, maxZ float64,
    out []int32,
) []int32 {
    if tile.Grid != nil {
        return tileQueryPolysGrid(tile, minX, minY, maxX, maxY, minZ,
            maxZ, out)
    }
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

// tileQueryPolysGrid walks the bucket grid of a v3 tile: the buckets
// the query footprint touches list their polygons, the height window
// prunes the stacked surfaces the 2D grid cannot separate (the
// caller tests the exact 3D geometry on the survivors).
func tileQueryPolysGrid(tile *Tile, minX, minY, maxX, maxY, minZ, maxZ float64,
    out []int32,
) []int32 {
    grid := tile.Grid
    bucket := func(world, anchor float64) int {
        cells := int(math.Floor((world - anchor) / cellSizeWorld))
        b := cells / gridBucketCells
        if b < 0 {
            return 0
        }
        if b >= gridSide {
            return gridSide - 1
        }

        return b
    }
    bx0 := bucket(minX, tile.worldMinX)
    bx1 := bucket(maxX, tile.worldMinX)
    by0 := bucket(minY, tile.worldMinY)
    by1 := bucket(maxY, tile.worldMinY)
    for by := by0; by <= by1; by++ {
        base := by * gridSide
        for bx := bx0; bx <= bx1; bx++ {
            b := base + bx
            for e := grid.Offsets[b]; e < grid.Offsets[b+1]; e++ {
                pi := int32(grid.Entries[e])
                poly := &tile.Polys[pi]
                hMin, hMax := polyHeightRange(poly)
                if float64(hMax) < minZ || float64(hMin) > maxZ {
                    continue
                }
                out = append(out, pi)
            }
        }
    }

    return out
}

// polyHeightRange returns the min and the max corner height of a
// polygon (the bilinear surface stays inside the corner range).
func polyHeightRange(poly *Poly) (int16, int16) {
    hMin := poly.H00
    hMax := poly.H00
    for _, h := range [4]int16{poly.H00, poly.H10, poly.H01, poly.H11} {
        if h < hMin {
            hMin = h
        }
        if h > hMax {
            hMax = h
        }
    }

    return hMin, hMax
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
