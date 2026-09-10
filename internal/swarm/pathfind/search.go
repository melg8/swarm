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

// Water plane constants of the C1 world. The water zones of the
// Mobius data sit with their surface at -3780 (the maxZ of the
// water.xml cuboids); the geodata layers below it are the lake and
// sea beds. The server lets characters walk (swim) over them, so
// the search must too - but at a cost, so land routes win whenever
// they exist (the elven village town trips crossed the whole lake
// under the floating island before this cost existed).
const (
	// waterLevel is the water surface height; layers below it count
	// as underwater.
	waterLevel = int16(-3780)
	// waterCostMultiplier scales the step cost of every move landing
	// on an underwater cell: swimming is several times slower than
	// running and burns the breath meter.
	waterCostMultiplier = float32(3)
)

// nodeKey identifies one search node: the cell plus the height of the
// layer it was reached with. Multilayer cells produce one node per
// walkable floor, so a cell first touched from the water does not seal
// its bridge deck layer away from the search (a single layer per cell
// resolved on first touch did exactly that on the Elven village bridge).
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
	approachRadius  float64
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
// the server's own pathfinder does (PathFinding.findPath resolves
// getHeight(tx, ty, tz)): a coordinate with several floors picks the
// floor the destination names, not the floor the start stands on.
// The search answers the straight line of sight raster directly only
// when the line is a dry minimum-rate walk (directOrAstar below);
// every other route comes from the cost aware A*. With a positive
// approachRadius the search succeeds on the first node within that
// 3D distance of the end point (the merchant interaction distance
// of the town trips); with zero it succeeds on the first arrival on
// the target cell at any layer (the original behavior).
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

	result := &Result{
		Found:     false,
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
				// A wall hit: the cell is not walkable from here.
				// The original library inserted it into the open set
				// with an astronomic cost instead, which let a sealed
				// target produce a wall crossing path; unreachable
				// targets must report not found.
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

// directOrAstar answers the straight raster line between the start
// and the target when it is a clean dry walk, and runs the full cost
// aware A* otherwise. It returns the raw node path together with its
// smoothed form. The old line of sight short circuit answered ANY
// walkable straight line - including one that fords a lake bed - and
// so bypassed the water cost entirely: the synthetic channel route
// swam straight across while the bridge, cheaper by the water
// penalty, sat unplanned. A dry line of sight is the safe case to
// keep answering directly: it walks the minimum step rate the grid
// allows, so no detour beats it by more than the wall proximity
// noise, and skipping the search keeps the flat field walks (the
// region crossing concurrent routes) instant instead of flooding
// the A* ellipse and the quadratic smoothing cascade over them (the
// accepted direct line is its own smoothing: the two endpoints). A
// line that steps into water, hits a wall or a terrace boundary, or
// ends short of the target (missing geodata truncated the raster)
// defers to the A*, which weighs the swim against the bridge detour
// and the walls against the detour itself.
func (s *search) directOrAstar(from, to *node) ([]*node, []*node) {
	direct := s.straightPath(from, to)
	if len(direct) > 0 && direct[len(direct)-1].coords == to.coords &&
		s.directLineDry(direct) {
		return direct, []*node{direct[0], direct[len(direct)-1]}
	}

	raw := s.astar(from)
	if raw == nil {
		return nil, nil
	}

	return raw, s.smoothPath(raw)
}

// directLineDry reports whether every step of a raster line is
// walkable at the plain step rate and lands on dry ground: the water
// penalty is the one cost dimension that can make a walkable
// straight line clearly worse than a detour.
func (s *search) directLineDry(direct []*node) bool {
	for i := 0; i+1 < len(direct); i++ {
		if !s.canStep(direct[i], direct[i+1]) {
			return false
		}
		if direct[i+1].layer.Height < waterLevel {
			return false
		}
	}

	return true
}

// dryLine reports whether the whole raster line between two nodes
// stays above the water level - the strict form the click guard uses:
// a line that touches a lake or sea bed anywhere is wet, whatever
// its endpoints stand on.
func (s *search) dryLine(from, to *node) bool {
	for _, step := range s.straightPath(from, to) {
		if step.layer.Height < waterLevel {
			return false
		}
	}

	return true
}

// legDry reports whether the smoothing may replace the walk between
// two nodes with a straight leg: a leg between two dry points must
// stay above the water surface. The string pulling itself is water
// blind - it only asks the line of sight - and the line of sight
// happily crosses a lake bed whose shores step within the passable
// height, so without this rule the smoothed path fords bays the cost
// aware search routed around and hands the walker water crossing
// legs (the elven village lake sent the town trips swimming). A leg
// that starts or ends in the water is exempt: it belongs to the swim
// escape of a character that already stands in a lake, and the water
// is the only surface such a walk can use.
func (s *search) legDry(from, to *node) bool {
	if from.layer.Height < waterLevel || to.layer.Height < waterLevel {
		return true
	}

	return s.dryLine(from, to)
}

// nodeReached reports whether a popped node satisfies the goal of
// the search: an approach run accepts the first node within the 3D
// approach radius of the target point (the exact target node sits
// inside that ball whenever it is reachable), a plain run accepts the
// first arrival on the target cell whatever layer the walk came on.
func (s *search) nodeReached(current *node) bool {
	if s.approachRadius > 0 {
		world := nodeWorld(current)
		dx := world.X - s.targetWorld.X
		dy := world.Y - s.targetWorld.Y
		dz := world.Z - s.targetWorld.Z

		return math.Sqrt(dx*dx+dy*dy+dz*dz) <= s.approachRadius
	}

	return current.coords == s.target
}

// runEscape plans the way out of the water for a position whose
// geodata surface lies below the water level: a breadth first flood
// over the walkable surface (the same canStep rules as the A*) that
// stops on the first node standing above the water surface - the
// nearest shore. A character floating over a lake bed cannot trust
// the ordinary searches: the server refuses move requests whose
// target resolves onto a deck the bed has no walkable connection to
// (the terrace under the elven village), so the only sensible walk
// is the one back to the shore the terrain itself offers. The start
// must be resolved by the caller; a start already above the water
// level needs no escape and answers Found=false.
func (s *search) runEscape(start Vec3) (*Result, error) {
	began := time.Now()
	from, err := s.nodeAtWorld(start)
	if err != nil {
		return nil, err
	}
	result := &Result{
		Found:     false,
		Aborted:   false,
		Waypoints: nil,
		RawPath:   nil,
		Duration:  0,
		Explored:  0,
		OpenLeft:  0,
		Length:    0,
	}
	if from.layer.Height >= waterLevel {
		return result, nil
	}
	raw := s.escape(from)
	result.Duration = time.Since(began)
	result.Explored = s.explored
	result.Aborted = s.aborted
	if raw == nil {
		return result, nil
	}
	result.Found = true
	result.RawPath = nodesToWorld(raw)
	result.Waypoints = nodesToWorld(s.smoothPath(raw))
	result.Length = pathLength(result.Waypoints)

	return result, nil
}

// escape floods the walkable surface from the start and returns the
// raw node path to the first node above the water level. The flood
// breathes outward step by step (a plain BFS - the escape wants the
// nearest shore, not the cheapest one), never re-enters a node it
// already touched and stops at the expansion cap so a lake without a
// walkable shore cannot loop forever.
func (s *search) escape(from *node) []*node {
	queue := []*node{from}
	seen := map[nodeKey]bool{from.key: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.layer.Height >= waterLevel {
			return s.reconstruct(current)
		}
		if s.explored >= MaxSearchExpansions {
			s.aborted = true

			return nil
		}
		s.explored++
		for _, next := range s.neighbors(current, 1) {
			if seen[next.key] {
				continue
			}
			if !s.canStep(current, next) {
				continue
			}
			seen[next.key] = true
			next.parent = current
			queue = append(queue, next)
		}
	}

	return nil
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

// costTo returns the movement cost between neighbouring cells, with the
// wall proximity multiplier of the original (the more walled cells in the
// 7x7 ring, the more the step costs, pulling the path away from)
// and the water penalty of the destination.
func (s *search) costTo(current, next *node, ring []*node) float32 {
	if !s.canStep(current, next) {
		return impassableScore
	}
	cost := commonScore
	if current.coords.X != next.coords.X &&
		current.coords.Y != next.coords.Y {
		cost = diagonalScore
	}
	if next.layer.Height < waterLevel {
		// The step lands underwater: swimming costs several land
		// steps, so bridges and shores beat water crossings.
		cost *= waterCostMultiplier
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

// canStep mirrors the walkable surface rule of one cell step: the
// walls of the source cell must be open for the step direction and the
// height difference must stay within the passable limit in BOTH
// directions (the Mobius HEIGHT_INCREASE_LIMIT of 40 gates the climb;
// the drop uses the same bound). The Mobius movement validation
// accepts any downward step, but a planned walk must stay on the
// walkable surfaces: the world stacks terraces over each other (the
// lake bed under the floating elven city deck) and the only walkable
// connections between them are the gradual ramps - the bridges and
// the shores, every ramp cell measured at 8..24 units of the real
// geodata. A step that drops hundreds of units is a terrace boundary
// (a deck edge behind a railing the geodata does not model, a cliff):
// planning it sends the walker over the railing the server never
// lets it cross or into a fall. The line of sight raster and the
// smoothing share the same strict symmetric form so no leg of the
// smoothed path ever leaves the surface either.
func (s *search) canStep(from, to *node) bool {
	if !s.wallsOpen(from, to) {
		return false
	}

	return heightDelta(from.layer.Height, to.layer.Height) <=
		s.maxPassableHeight
}

// wallsOpen reports whether the walls of the source node allow the
// step to the adjacent node.
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

	return true
}

// canMoveTo reports whether the straight walk from one cell to an
// adjacent one stays on the walkable surface: the walls of the source
// cell open for the direction and the symmetric height difference
// within the passable limit - the same terrace rule canStep applies
// to the search expansions. The line of sight raster and the
// smoothing use this strict form: a leg they verify must stay on one
// walkable surface, never drop off it.
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

// neighbors returns the existing cell nodes around a node: radius 1 is
// the 8 step neighbourhood, radius 3 the 7x7 ring for the wall
// proximity. Cells without geodata are skipped. The scratch buffer is
// reused between expansions to keep the hot loop allocation free.
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

	return true
}

// straightPath walks the supercover line between two nodes in cell
// coordinates: the t/k test advances x and y so corners are not cut and
// every shared edge crossing lands on a real cell. It is a verbatim port
// of the original GetStraightPath, including the start height driving
// the layer selection of the rastered cells.
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
// still sees the candidate ahead and commits a turning point whenever
// the line of sight breaks. The anchor stays on the committed point, so
// every leg between two waypoints was verified with a line of sight -
// the original jumped the anchor one node past the commit, which left
// the leg between two waypoints unchecked and let the smoothed path cut
// wall corners near gaps. Every leg between two dry points must also
// stay dry (legDry): the cost aware search may have walked around a
// lake while the sight lines across its bed stay open, and collapsing
// them back would reintroduce the swim the search paid to avoid.
func (s *search) smoothPath(path []*node) []*node {
	if len(path) == 0 {
		return nil
	}
	result := []*node{path[0]}
	current := path[0]
	for i := 1; i < len(path); i++ {
		if s.lineOfSight(current, path[i]) && s.legDry(current, path[i]) {
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
