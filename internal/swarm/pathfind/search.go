// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "container/heap"
    "fmt"
    "math"
    "time"
)

// Movement scores of the search, matching the original constants: an
// orthogonal cell step costs 10, a diagonal one 10*sqrt(2), and the
// heuristic is the Manhattan distance of the cells times 10.
const (
    commonScore   = float32(10)
    diagonalScore = commonScore * 1.41421356
    // impassableScore marks a wall hit; half of the float32 maximum
    // keeps the additions of the original overflow free.
    impassableScore = float32(math.MaxFloat32 / 2)
)

// WaterLevel is the water surface of the C1 world (the maxZ of the
// Mobius water.xml cuboids); the layers below it are the lake and sea
// beds the server lets characters swim over.
const (
    WaterLevel = int16(-3780)
    // waterCostMultiplier prices an underwater step at the run/swim
    // speed ratio of the C1 server (115/50 for the HumanFighter the bots
    // walk; the swim of every class is 50).
    waterCostMultiplier = float32(2.3)
    // avoidEscapeMultiplier prices an escape cell of the ban that holds
    // the search start: expensive enough to leave by the nearest edge,
    // never impassable (a sealed start answers no route at all).
    avoidEscapeMultiplier = float32(6)
    // avoidEscapeRadius bounds that escape ring: a route threading
    // deeper into the own ban toward a goal sealed behind it still
    // answers not found (the sealed goal falls back to the next rung).
    avoidEscapeRadius = 256.0
)

// nodeKey identifies one search node: the cell plus the height of the
// layer it was reached with. A multilayer cell carries one node per
// walkable floor, so a cell first touched from the water never seals
// its bridge deck layer (docs/pathfinding.md).
type nodeKey struct {
    p Point
    h int16
}

// node is one search node: a geodata cell resolved to one of its
// layers, with the A* bookkeeping attached.
type node struct {
    key    nodeKey
    coords Point
    layer  Layer
    parent *node
    g, h   float32
    seq    uint64
    index  int
}

// search is one path finding run: the node cache, the open and closed
// sets and the target cell of the search. The caches are per run, the
// parsed regions are shared through the engine.
type search struct {
    engine            *Engine
    maxPassableHeight int
    nodes             map[nodeKey]*node
    missing           map[Point]bool
    openSet           map[nodeKey]*node
    closed            map[nodeKey]*node
    queue             nodeQueue
    target            Point
    targetKey         nodeKey
    // targetWorld is the world point of the goal; the approach
    // searches measure the remaining 3D distance against it.
    targetWorld Vec3
    // approachRadius terminates the search on the first node within
    // this 3D distance of targetWorld. Zero keeps the plain cell
    // arrival semantics (any layer of the target cell).
    approachRadius float64
    // avoid holds the world patches this search must route around (the
    // hunt recovery's ban): a step onto a banned cell is impassable, and
    // neither the direct line nor the smoothing may cross it. The ban is
    // the planner's memory of ground the live server refused to walk
    // though the pack models it open.
    avoid []AvoidArea
    // escapeIdx is the index of the avoid area holding the search start
    // (-1 when the start sits on no ban): its cells within
    // avoidEscapeRadius stay plannable at avoidEscapeMultiplier.
    escapeIdx int
    // startWorld is the world position of the search start: the
    // escape ring of the own ban measures against it.
    startWorld Vec3
    // neighborScratch and ringScratch are the reusable neighbor
    // buffers of the expansion loop.
    neighborScratch []*node
    ringScratch     []*node
    region          *Region
    regionKey       RegionKey
    explored        int
    aborted         bool
    seq             uint64
}

// newSearch prepares a fresh search over an engine.
func newSearch(engine *Engine, maxPassableHeight uint16) *search {
    return &search{
        engine:            engine,
        maxPassableHeight: int(maxPassableHeight),
        nodes:             make(map[nodeKey]*node),
        missing:           make(map[Point]bool),
        openSet:           make(map[nodeKey]*node),
        closed:            make(map[nodeKey]*node),
        queue:             make(nodeQueue, 0, 256),
        target:            Point{X: 0, Y: 0},
        targetKey:         nodeKey{p: Point{X: 0, Y: 0}, h: 0},
        targetWorld:       Vec3{X: 0, Y: 0, Z: 0},
        approachRadius:    0,
        avoid:             nil,
        escapeIdx:         -1,
        startWorld:        Vec3{X: 0, Y: 0, Z: 0},
        neighborScratch:   nil,
        ringScratch:       nil,
        region:            nil,
        regionKey:         RegionKey{Col: 0, Row: 0},
        explored:          0,
        aborted:           false,
        seq:               0,
    }
}

// closestLayer returns the layer of a cell closest to z, going through
// the cached region when possible: the walk is highly local, so one
// pointer check replaces the engine cache lock on almost every access.
func (s *search) closestLayer(p Point, z int16) (Layer, bool) {
    key := CellToRegion(p)
    if s.region == nil || s.regionKey != key {
        entry, err := s.engine.entry(key)
        if err != nil || entry.region == nil {
            s.region, s.regionKey = nil, key

            return Layer{Height: 0, NSWE: 0}, false
        }
        s.region, s.regionKey = entry.region, key
    }

    return s.region.ClosestLayer(LocalCell(p), z)
}

// nodeAtWorld resolves a world position to its cell node. A position
// without geodata is a hard error: the search has no meaningful start or
// target without it.
func (s *search) nodeAtWorld(position Vec3) (*node, error) {
    coords := WorldToCell(position.X, position.Y)
    node := s.node(coords, int16(position.Z))
    if node == nil {
        return nil, fmt.Errorf("%w at %.0f %.0f", ErrMissingCell,
            position.X, position.Y)
    }

    return node, nil
}

// node returns the search node of a cell for the layer closest to z,
// creating it on first use. Cells without geodata return nil and stay
// cached as missing so the neighbour loops do not re-query them.
func (s *search) node(coords Point, z int16) *node {
    if s.missing[coords] {
        return nil
    }
    layer, ok := s.closestLayer(coords, z)
    if !ok {
        s.missing[coords] = true

        return nil
    }
    key := nodeKey{p: coords, h: layer.Height}
    if existing, ok := s.nodes[key]; ok {
        return existing
    }
    resolved := &node{
        key:    key,
        coords: coords,
        layer:  layer,
        parent: nil,
        g:      0,
        h:      0,
        seq:    s.nextSeq(),
        index:  0,
    }
    s.nodes[key] = resolved

    return resolved
}

// nextSeq returns the insertion sequence for stable heap tie breaking.
func (s *search) nextSeq() uint64 {
    s.seq++

    return s.seq
}

// run executes the whole search and fills the result statistics. The
// target cell resolves its intended layer against the target z, like
// the server pathfinder does (getHeight(tx, ty, tz)): a coordinate with
// several floors picks the floor the destination names. A positive
// approachRadius ends the search on the first node within that 3D
// distance, zero on the first arrival on the target cell.
func (s *search) run(start, end Vec3, approachRadius float64) (*Result, error) {
    began := time.Now()
    from, err := s.nodeAtWorld(start)
    if err != nil {
        return nil, err
    }
    to, err := s.nodeAtWorld(end)
    if err != nil {
        return nil, err
    }
    s.target = to.coords
    s.targetKey = to.key
    s.targetWorld = end
    s.approachRadius = approachRadius
    s.startWorld = start
    s.escapeIdx = s.startAvoidIndex(from)

    result := &Result{
        Found:     false,
        Partial:   false,
        Aborted:   false,
        Waypoints: nil,
        RawPath:   nil,
        Duration:  0,
        Explored:  0,
        OpenLeft:  0,
        Length:    0,
    }
    raw, smooth := s.directOrAstar(from, to)
    result.Duration = time.Since(began)
    result.Explored = s.explored
    result.OpenLeft = len(s.openSet)
    result.Aborted = s.aborted
    if raw == nil {
        return result, nil
    }
    result.Found = true
    result.RawPath = nodesToWorld(raw)
    result.Waypoints = nodesToWorld(smooth)
    if s.engine.capsuleRadius > 0 {
        result.Waypoints = NewCapsule(s.engine).ApplyPath(result.Waypoints,
            s.engine.capsuleRadius)
    }
    result.Length = pathLength(result.Waypoints)

    return result, nil
}

// astar searches the cell grid and returns the raw node path from the
// start to the target, or nil when the target is unreachable.
func (s *search) astar(from *node) []*node {
    from.g = 0
    from.h = s.heuristic(from, s.target)
    s.push(from)
    for s.queue.Len() > 0 {
        current := heap.Pop(&s.queue).(*node)
        delete(s.openSet, current.key)
        if s.nodeReached(current) {
            return s.reconstruct(current)
        }
        if s.explored >= MaxSearchExpansions {
            s.aborted = true

            return nil
        }
        s.closed[current.key] = current
        s.explored++
        // The wall proximity multiplier needs the 7x7 ring around the
        // expanded cell; the original recomputes it per neighbour, the
        // ring is identical for all of them.
        ring := s.neighbors(current, 3)
        for _, next := range s.neighbors(current, 1) {
            if _, done := s.closed[next.key]; done {
                continue
            }
            step := current.g + s.costTo(current, next, ring)
            if step >= impassableScore {
                // A wall hit: unreachable targets report not found, never
                // a wall crossing path (the original opened it at an
                // astronomical cost).
                continue
            }
            if existing, inOpen := s.openSet[next.key]; inOpen {
                if step >= existing.g {
                    continue
                }
                next.parent = current
                next.g = step
                next.h = s.heuristic(next, s.target)
                heap.Fix(&s.queue, next.index)

                continue
            }
            next.parent = current
            next.g = step
            next.h = s.heuristic(next, s.target)
            s.push(next)
        }
    }

    return nil
}

// directOrAstar answers the straight raster line between the start and
// the target when it is a clean dry walk, and runs the full cost aware
// A* otherwise (it returns the raw node path and its smoothed form). A
// dry line walks the minimum step rate the grid allows, so no detour
// beats it: the direct answer is its own smoothing (the two endpoints).
// A line that steps into water, hits a wall or a terrace boundary, or
// ends short of the target defers to the A*.
func (s *search) directOrAstar(from, to *node) ([]*node, []*node) {
    direct := s.straightPath(from, to)
    if len(direct) > 0 && direct[len(direct)-1].coords == to.coords &&
        s.directLineDry(direct) && s.serverSegmentVerified(from, to) &&
        s.segmentAllowed(from, to) {
        return direct, []*node{direct[0], direct[len(direct)-1]}
    }

    raw := s.astar(from)
    if raw == nil {
        return nil, nil
    }

    return raw, s.smoothPath(raw)
}

// directLineDry reports whether every step of a raster line is walkable
// at the plain step rate and lands on dry ground: water is the one cost
// a detour can beat.
func (s *search) directLineDry(direct []*node) bool {
    for i := 0; i+1 < len(direct); i++ {
        if !s.canStep(direct[i], direct[i+1]) {
            return false
        }
        if direct[i+1].layer.Height < WaterLevel {
            return false
        }
    }

    return true
}

// dryLine reports whether the whole raster line between two nodes
// stays above the water level: a line that touches a lake or sea bed
// anywhere is wet, whatever its endpoints stand on.
func (s *search) dryLine(from, to *node) bool {
    for _, step := range s.straightPath(from, to) {
        if step.layer.Height < WaterLevel {
            return false
        }
    }

    return true
}

// segmentDry reports whether the smoothing may replace the walk between
// two nodes with a straight segment: a segment between two dry points
// must stay above the water surface, since the sight line happily
// crosses a lake bed the cost aware search paid to route around. A
// segment that starts or ends in the water is exempt - it belongs to a
// crossing the search itself priced.
func (s *search) segmentDry(from, to *node) bool {
    if from.layer.Height < WaterLevel || to.layer.Height < WaterLevel {
        return true
    }

    return s.dryLine(from, to)
}

// nodeReached reports whether a popped node satisfies the goal: an
// approach run accepts the first node within the 3D approach radius or
// the target cell itself (a cell center sits up to half a diagonal away
// from any point inside it, so a tighter radius would flood the whole
// component). A plain run accepts the target cell on any layer; the
// caller validates the landed deck against the destination.
func (s *search) nodeReached(current *node) bool {
    if s.approachRadius > 0 {
        world := nodeWorld(current)
        dx := world.X - s.targetWorld.X
        dy := world.Y - s.targetWorld.Y
        dz := world.Z - s.targetWorld.Z
        if math.Sqrt(dx*dx+dy*dy+dz*dz) <= s.approachRadius {
            return true
        }
    }

    return current.coords == s.target
}

// push inserts a node into the open set.
func (s *search) push(node *node) {
    s.openSet[node.key] = node
    heap.Push(&s.queue, node)
}

// reconstruct walks the parent chain of the target back to the start.
func (s *search) reconstruct(target *node) []*node {
    path := []*node{}
    for current := target; current != nil; current = current.parent {
        path = append(path, current)
    }
    for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
        path[i], path[j] = path[j], path[i]
    }

    return path
}

// costTo returns the movement cost between neighbouring cells: the step
// score, the water penalty of the destination and the wall proximity
// multiplier of the 7x7 ring (the more walled cells, the dearer the
// step).
func (s *search) costTo(current, next *node, ring []*node) float32 {
    if !s.canStep(current, next) {
        return impassableScore
    }
    escape, avoided := s.cellAvoidedEscape(next.coords)
    if avoided && !escape {
        // The recovery ban: the live server proved this ground
        // unwalkable for this session, the detour around it is the
        // only plan worth planning.
        return impassableScore
    }
    cost := commonScore
    if current.coords.X != next.coords.X &&
        current.coords.Y != next.coords.Y {
        cost = diagonalScore
    }
    if next.layer.Height < WaterLevel {
        // The step lands underwater: the swim price keeps bridges and
        // shores ahead unless the water cut is the genuinely faster walk.
        cost *= waterCostMultiplier
    }
    if escape {
        // The ban that holds the start: the only honest route out
        // of it crosses its own ground - expensive, never sealed
        // (see avoidEscapeMultiplier).
        cost *= avoidEscapeMultiplier
    }

    return cost * s.obstacleMultiplier(ring)
}

// obstacleMultiplier counts the walled cells of the ring: an open
// neighbourhood multiplies by 1, a corridor multiplies by ring/obstacles.
func (s *search) obstacleMultiplier(ring []*node) float32 {
    if len(ring) == 0 {
        return 1
    }
    obstacles := 0
    for _, candidate := range ring {
        if !candidate.layer.IsCompletelyOpen() {
            obstacles++
        }
    }
    if obstacles == 0 {
        return 1
    }

    return float32(len(ring)) / float32(obstacles)
}

// canStep mirrors the walkable surface rule of one step: the walls of
// the source cell open for the direction, the walls of the target cell
// open in reverse, and the height difference within the Mobius
// HEIGHT_INCREASE_LIMIT of 40 in both directions. The server accepts
// any downward step, but a planned walk must stay on the walkable
// surfaces: the world stacks terraces (the lake bed under the floating
// city deck) whose only connections are the 8..24 unit ramps. A drop
// of hundreds of units is a terrace edge or a cliff, and the sight
// raster and the smoothing share the strict symmetric form.
func (s *search) canStep(from, to *node) bool {
    if !s.wallsOpen(from, to) {
        return false
    }

    return heightDelta(from.layer.Height, to.layer.Height) <=
        s.maxPassableHeight
}

// wallsOpen reports whether the walls of the source node allow the step
// to the adjacent node: the cardinal directions check the source
// layer's own wall, the reverse direction checks the target cell (the
// MoveToLocation isCompletelyBlocked rule), and a diagonal step needs
// BOTH flanking cells open - the anti corner cut rule every server
// movement channel applies (docs/pathfinding.md). A route that cuts a
// walled corner plans a walk the server refuses wholesale: the click
// collapses onto the walker cell and freezes the character in place.
//
//nolint:cyclop
func (s *search) wallsOpen(from, to *node) bool {
    if from.coords.Y > to.coords.Y && !from.layer.IsNorthOpen() {
        return false
    }
    if from.coords.Y < to.coords.Y && !from.layer.IsSouthOpen() {
        return false
    }
    if from.coords.X < to.coords.X && !from.layer.IsEastOpen() {
        return false
    }
    if from.coords.X > to.coords.X && !from.layer.IsWestOpen() {
        return false
    }
    // The target cell's reverse wall must be open too: MoveToLocation
    // rejects a cell whose walls are all closed from the approach side.
    if from.coords.Y > to.coords.Y && !to.layer.IsSouthOpen() {
        return false
    }
    if from.coords.Y < to.coords.Y && !to.layer.IsNorthOpen() {
        return false
    }
    if from.coords.X < to.coords.X && !to.layer.IsWestOpen() {
        return false
    }
    if from.coords.X > to.coords.X && !to.layer.IsEastOpen() {
        return false
    }
    if from.coords.X != to.coords.X && from.coords.Y != to.coords.Y {
        return s.diagonalFlanksOpen(from, to)
    }

    return true
}

// diagonalFlanksOpen mirrors the anti corner cut of a diagonal step: the
// two orthogonal cells the step cuts across must allow the crossing,
// their layers resolved against the source height (the running z of the
// server line validation). The SW step needs (x, y+1) open to the west
// and (x-1, y) open to the south; a flank without geodata counts open.
func (s *search) diagonalFlanksOpen(from, to *node) bool {
    south := to.coords.Y > from.coords.Y
    east := to.coords.X > from.coords.X
    vertical := s.node(
        Point{X: from.coords.X, Y: to.coords.Y}, from.layer.Height)
    if vertical != nil {
        if east && !vertical.layer.IsEastOpen() {
            return false
        }
        if !east && !vertical.layer.IsWestOpen() {
            return false
        }
    }
    horizontal := s.node(
        Point{X: to.coords.X, Y: from.coords.Y}, from.layer.Height)
    if horizontal != nil {
        if south && !horizontal.layer.IsSouthOpen() {
            return false
        }
        if !south && !horizontal.layer.IsNorthOpen() {
            return false
        }
    }

    return true
}

// canMoveTo reports whether the straight walk between two adjacent cells
// stays on the walkable surface: the strict symmetric canStep rule the
// sight raster and the smoothing verify every segment with.
func (s *search) canMoveTo(from, to *node) bool {
    return s.canStep(from, to)
}

// heuristic is the Manhattan cell distance scaled like the step costs.
func (s *search) heuristic(current *node, target Point) float32 {
    dx := target.X - current.coords.X
    dy := target.Y - current.coords.Y
    if dx < 0 {
        dx = -dx
    }
    if dy < 0 {
        dy = -dy
    }

    return commonScore * float32(dx+dy)
}

// neighbors returns the existing nodes around a center: radius 1 is the
// 8 step neighbourhood, radius 3 the 7x7 ring of the wall proximity; a
// cell without geodata is skipped. The scratch buffers keep the hot loop
// allocation free.
func (s *search) neighbors(center *node, radius int) []*node {
    scratch := s.neighborScratch[:0]
    if radius > 1 {
        scratch = s.ringScratch[:0]
    }
    for dx := -radius; dx <= radius; dx++ {
        for dy := -radius; dy <= radius; dy++ {
            if dx == 0 && dy == 0 {
                continue
            }
            coords := Point{
                X: center.coords.X + int32(dx),
                Y: center.coords.Y + int32(dy),
            }
            if next := s.node(coords, center.layer.Height); next != nil {
                scratch = append(scratch, next)
            }
        }
    }
    if radius > 1 {
        s.ringScratch = scratch
    } else {
        s.neighborScratch = scratch
    }

    return scratch
}

// lineOfSight rasterizes the straight cell line between two nodes and
// requires every step to be walkable.
func (s *search) lineOfSight(from, to *node) bool {
    path := s.straightPath(from, to)
    for i := 0; i+1 < len(path); i++ {
        if !s.canMoveTo(path[i], path[i+1]) {
            return false
        }
    }

    return s.serverSegmentVerified(from, to)
}

// serverSegmentVerified answers whether the server click validation
// walks the straight line between two nodes: its Bresenham raster cuts
// the diagonals the t/k supercover of straightPath decomposes into
// cardinal steps, so a supercover-legal segment can still be refused.
// The smoothing may only collapse turns into segments a click survives.
func (s *search) serverSegmentVerified(from, to *node) bool {
    fromX, fromY, fromZ := nodeClickWorld(from)
    toX, toY, toZ := nodeClickWorld(to)
    vx, vy, vz := s.engine.validLocation(fromX, fromY, fromZ, toX, toY, toZ)

    return vx == toX && vy == toY && vz == toZ
}

// nodeClickWorld returns the world position of a node the way the
// server names a click target: the cell center with the layer height.
func nodeClickWorld(node *node) (int32, int32, int32) {
    return node.coords.X*cellSize + worldMinX + cellSize/2,
        node.coords.Y*cellSize + worldMinY + cellSize/2,
        int32(node.layer.Height)
}

// straightPath walks the supercover line between two nodes in cell
// coordinates: the t/k test advances x and y so every shared edge
// crossing lands on a real cell (the GetStraightPath port), the start
// height driving the layer selection.
func (s *search) straightPath(from, to *node) []*node {
    xS, yS := from.coords.X, from.coords.Y
    xE, yE := to.coords.X, to.coords.Y
    signX := sign(xE - xS)
    signY := sign(yE - yS)
    x0, y0 := int64(0), int64(0)
    x1, y1 := int64(xE-xS), int64(yE-yS)
    k := math.Abs(float64(y1-y0) / float64(x1-x0))

    path := []*node{s.node(Point{X: xS, Y: yS}, from.layer.Height)}
    x, y := x0, y0
    for x != x1 || y != y1 {
        t := float64(2*y*int64(signY)+1) / float64(2*x*int64(signX)+1)
        if t >= k {
            x += int64(signX)
        }
        if t <= k {
            y += int64(signY)
        }
        next := s.node(
            Point{X: xS + int32(x), Y: yS + int32(y)}, from.layer.Height)
        if next == nil {
            break
        }
        path = append(path, next)
    }

    return path
}

// smoothPath pulls the raw path straight: it keeps the last node that
// still sees the candidate ahead and commits a turning point where the
// sight, the dry or the ban test breaks. The anchor stays on the
// committed point, so every kept segment was verified - the original
// jumped it one node past the commit and cut wall corners near gaps.
func (s *search) smoothPath(path []*node) []*node {
    if len(path) == 0 {
        return nil
    }
    result := []*node{path[0]}
    current := path[0]
    for i := 1; i < len(path); i++ {
        if s.lineOfSight(current, path[i]) && s.segmentDry(current, path[i]) &&
            s.segmentAllowed(current, path[i]) {
            continue
        }
        current = path[i-1]
        result = append(result, current)
    }
    if current != path[len(path)-1] {
        result = append(result, path[len(path)-1])
    }

    return result
}

// cellAvoided reports whether the cell falls inside one of the avoid
// areas of the search: the recovery ban of the hunt loop, expressed in
// world coordinates the caller derives from its own freeze reports.
func (s *search) cellAvoided(p Point) bool {
    if len(s.avoid) == 0 {
        return false
    }
    center := CellToWorldCenter(p)
    for _, area := range s.avoid {
        if math.Hypot(center.X-area.Center.X, center.Y-area.Center.Y) <=
            area.Radius {
            return true
        }
    }

    return false
}

// cellAvoidedEscape splits the avoid answer of cellAvoided: a cell of
// the area holding the search start within avoidEscapeRadius is an
// escape (passable at the escape price), while a cell of any other area
// and the own ban beyond the start neighborhood stay walls.
func (s *search) cellAvoidedEscape(p Point) (escape bool, avoided bool) {
    if len(s.avoid) == 0 {
        return false, false
    }
    center := CellToWorldCenter(p)
    escapeOnly := false
    for i, area := range s.avoid {
        if math.Hypot(center.X-area.Center.X, center.Y-area.Center.Y) >
            area.Radius {
            continue
        }
        if i != s.escapeIdx {
            return false, true
        }
        if math.Hypot(center.X-s.startWorld.X, center.Y-s.startWorld.Y) >
            avoidEscapeRadius {
            return false, true
        }
        escapeOnly = true
    }

    return escapeOnly, escapeOnly
}

// startAvoidIndex answers the index of the first avoid area that
// contains the world cell of the search start (-1 when the start sits
// on no ban): the escape exemption of cellAvoidedEscape keys on it.
func (s *search) startAvoidIndex(from *node) int {
    center := CellToWorldCenter(from.coords)
    for i, area := range s.avoid {
        if math.Hypot(center.X-area.Center.X, center.Y-area.Center.Y) <=
            area.Radius {
            return i
        }
    }

    return -1
}

// segmentAllowed reports whether the straight segment between two nodes
// stays outside every avoid area: the direct line and the smoothing may
// only collapse segments the ban does not cross.
func (s *search) segmentAllowed(from, to *node) bool {
    if len(s.avoid) == 0 {
        return true
    }
    for _, step := range s.straightPath(from, to) {
        if s.cellAvoided(step.coords) {
            return false
        }
    }

    return true
}

// sign returns the sign of an integer difference.
func sign(v int32) int32 {
    if v > 0 {
        return 1
    }
    if v < 0 {
        return -1
    }

    return 0
}

// nodesToWorld converts a node path to world cell centers with the layer
// height as z.
func nodesToWorld(path []*node) []Vec3 {
    world := make([]Vec3, len(path))
    for i, node := range path {
        world[i] = nodeWorld(node)
    }

    return world
}

// nodeWorld is the world position of a node (cell center, layer height).
func nodeWorld(node *node) Vec3 {
    center := CellToWorldCenter(node.coords)

    return Vec3{X: center.X, Y: center.Y, Z: float64(node.layer.Height)}
}

// pathLength sums the 3D segment lengths of a waypoint path.
func pathLength(waypoints []Vec3) float64 {
    length := 0.0
    for i := 1; i < len(waypoints); i++ {
        dx := waypoints[i].X - waypoints[i-1].X
        dy := waypoints[i].Y - waypoints[i-1].Y
        dz := waypoints[i].Z - waypoints[i-1].Z
        length += math.Sqrt(dx*dx + dy*dy + dz*dz)
    }

    return length
}

// nodeQueue is the A* open set: a binary heap ordered by the f score,
// ties broken by the remaining distance and then by insertion order so
// runs are deterministic.
type nodeQueue []*node

// Len returns the heap length.
func (q nodeQueue) Len() int { return len(q) }

// Less orders the nodes by f score, then h, then insertion sequence.
func (q nodeQueue) Less(i, j int) bool {
    fi, fj := q[i].g+q[i].h, q[j].g+q[j].h
    if fi != fj {
        return fi < fj
    }
    if q[i].h != q[j].h {
        return q[i].h < q[j].h
    }

    return q[i].seq < q[j].seq
}

// Swap exchanges two heap entries and maintains the node indices for
// heap.Fix.
func (q nodeQueue) Swap(i, j int) {
    q[i], q[j] = q[j], q[i]
    q[i].index = i
    q[j].index = j
}

// Push appends a node to the heap.
func (q *nodeQueue) Push(value any) {
    node := value.(*node)
    node.index = len(*q)
    *q = append(*q, node)
}

// Pop removes the best node from the heap.
func (q *nodeQueue) Pop() any {
    old := *q
    item := old[len(old)-1]
    old[len(old)-1] = nil
    *q = old[:len(old)-1]
    item.index = 0

    return item
}
