// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// maxQueryNodes bounds the A* node count of one flat search. The
// corridors of real routes hold hundreds of polygons; the bound exists
// so a pathological filter (or a mesh bug) answers a partial route
// instead of walking every polygon of every loaded region. The
// hierarchical route passes its own budgets (the coarse level and the
// refinement hops, see hierarchy.go).
const maxQueryNodes = 65536

// astarNode is one node of the corridor search: the polygon with its
// tile (resolved once at relaxation - the mesh lookup per settled
// node is the hot path cost this cache removes), the entry position
// (the portal midpoint), the parent, the costs and the open heap
// bookkeeping.
type astarNode struct {
        ref     PolyRef
        parent  uint32
        pos     Pos
        g       float64
        f       float64
        closed  bool
        heapIdx int32
        tile    *Tile
        poly    *Poly
}

// noParent marks the root of the parent chain.
const (
        noParent  = math.MaxUint32
        notInHeap = -1
)

// queryState is the pooled per-query search state: the node array,
// the reference-to-node index, the open heap and the best partial
// node tracking. One Route call takes a state from the mesh pool,
// resets and returns it.
type queryState struct {
        nodes  []astarNode
        index  refIndex
        open   []uint32
        best   uint32
        bestH  float64
        escape bool
}

// refIndex is the open addressing reference-to-node table the flat
// search relaxes through. It replaces the map[PolyRef]uint32 whose
// hash lookup per link relaxation owned the flat search constant
// (the external benchmark round, docs/fastpath_research.md section
// 8: the identical exhaustive query answers 494 ms with the map and
// 35 ms with the typed arena the raasta engine uses). The epoch
// stamp makes the reset O(1): the slots of the older epochs read as
// empty, no rehash and no clear per query.
type refIndex struct {
        slots []refSlot
        mask  uint64
        count int
        epoch uint32
}

// refSlot is one open addressing cell: the reference key, the node
// index it created and the query generation it belongs to.
type refSlot struct {
        ref   PolyRef
        node  uint32
        epoch uint32
}

// refIndexMinSlots bounds the smallest table (the single tile
// searches create thousands of nodes, the whole map hops hundreds
// of thousands).
const refIndexMinSlots = 1 << 13

// init sizes the table to the power of two that keeps the default
// load under 0.7 at the requested node count.
func (ix *refIndex) init(nodes int) {
        size := refIndexMinSlots
        for size < (nodes+1)*2 {
                size <<= 1
        }
        ix.slots = make([]refSlot, size)
        ix.mask = uint64(size - 1)
        ix.count = 0
        ix.epoch = 1
}

// reset starts a new query generation: one integer bump, the table
// contents stay (the older epochs never match).
func (ix *refIndex) reset() {
        ix.count = 0
        ix.epoch++
        // The epoch wrap recycles the table: the slots all read stale at
        // the epoch 0 boundary, the insert fills them afresh.
        if ix.epoch == 0 {
                for i := range ix.slots {
                        ix.slots[i].epoch = 0
                }
                ix.epoch = 1
        }
}

// find returns the node index of a reference (false when the search
// has not created it in this generation).
func (ix *refIndex) find(ref PolyRef) (uint32, bool) {
        h := hashRef(uint64(ref))
        for {
                slot := &ix.slots[h&ix.mask]
                if slot.epoch != ix.epoch {
                        return 0, false
                }
                if slot.ref == ref {
                        return slot.node, true
                }
                h++
        }
}

// insert records a freshly created node (the caller guarantees the
// reference is new for this generation). The table doubles at the
// 0.7 load and rehashes the live generation only.
func (ix *refIndex) insert(ref PolyRef, node uint32) {
        if ix.count*10 >= len(ix.slots)*7 {
                ix.grow()
        }
        h := hashRef(uint64(ref))
        for {
                slot := &ix.slots[h&ix.mask]
                if slot.epoch != ix.epoch {
                        slot.ref = ref
                        slot.node = node
                        slot.epoch = ix.epoch
                        ix.count++

                        return
                }
                h++
        }
}

// grow doubles the table and carries the live generation's entries.
func (ix *refIndex) grow() {
        old := ix.slots
        ix.slots = make([]refSlot, len(old)*2)
        ix.mask = uint64(len(ix.slots) - 1)
        ix.count = 0
        for i := range old {
                if old[i].epoch == ix.epoch {
                        ix.insert(old[i].ref, old[i].node)
                }
        }
}

// hashRef is the splitmix64 finalizer: the packed references differ
// mostly in the low bits (the polygon index) and the tile bits (the
// region key), the avalanche spreads both into the table index.
func hashRef(v uint64) uint64 {
        v ^= v >> 33
        v *= 0xff51afd7ed558ccd
        v ^= v >> 33
        v *= 0xc4ceb9fe1a85ec53
        v ^= v >> 33

        return v
}

// reset empties the state for reuse.
func (s *queryState) reset(escape bool) {
        s.nodes = s.nodes[:0]
        s.index.reset()
        s.open = s.open[:0]
        s.best = 0
        s.bestH = math.MaxFloat64
        s.escape = escape
}

// create appends a fresh node for a reference (the caller guarantees
// the reference is new).
func (s *queryState) create(ref PolyRef, parent uint32, pos Pos,
        g, h float64, tile *Tile, poly *Poly,
) uint32 {
        s.nodes = append(s.nodes, astarNode{
                ref:     ref,
                parent:  parent,
                pos:     pos,
                g:       g,
                f:       g + h,
                closed:  false,
                heapIdx: notInHeap,
                tile:    tile,
                poly:    poly,
        })
        idx := uint32(len(s.nodes) - 1)
        s.index.insert(ref, idx)
        if h < s.bestH {
                s.bestH = h
                s.best = idx
        }

        return idx
}

// push inserts a node index into the open heap.
func (s *queryState) push(idx uint32) {
        if s.nodes[idx].heapIdx >= 0 {
                return
        }
        s.open = append(s.open, idx)
        i := int32(len(s.open) - 1)
        s.nodes[idx].heapIdx = i
        for i > 0 {
                parent := (i - 1) / 2
                if s.less(s.open[i], s.open[parent]) {
                        s.swap(i, parent)
                        i = parent
                } else {
                        break
                }
        }
}

// fix restores the heap property of a node whose score improved.
func (s *queryState) fix(idx uint32) {
        i := s.nodes[idx].heapIdx
        if i < 0 {
                s.push(idx)

                return
        }
        for i > 0 {
                parent := (i - 1) / 2
                if s.less(s.open[i], s.open[parent]) {
                        s.swap(i, parent)
                        i = parent
                } else {
                        break
                }
        }
}

// pop removes and returns the best open node index.
func (s *queryState) pop() (uint32, bool) {
        if len(s.open) == 0 {
                return 0, false
        }
        top := s.open[0]
        s.nodes[top].heapIdx = notInHeap
        last := s.open[len(s.open)-1]
        s.open = s.open[:len(s.open)-1]
        if len(s.open) > 0 {
                s.open[0] = last
                s.nodes[last].heapIdx = 0
                i := int32(0)
                for {
                        left := 2*i + 1
                        right := 2*i + 2
                        best := i
                        if int(left) < len(s.open) && s.less(s.open[left], s.open[best]) {
                                best = left
                        }
                        if int(right) < len(s.open) && s.less(s.open[right],
                                s.open[best]) {
                                best = right
                        }
                        if best == i {
                                break
                        }
                        s.swap(i, best)
                        i = best
                }
        }

        return top, true
}

// less orders two heap entries by their f score.
func (s *queryState) less(a, b uint32) bool {
        return s.nodes[a].f < s.nodes[b].f
}

// swap exchanges two heap entries and fixes their indices.
func (s *queryState) swap(i, j int32) {
        s.open[i], s.open[j] = s.open[j], s.open[i]
        s.nodes[s.open[i]].heapIdx = i
        s.nodes[s.open[j]].heapIdx = j
}

// corridorOf walks the parent chain of a node back to the root and
// returns the start-to-node polygon corridor.
func (s *queryState) corridorOf(idx uint32) []PolyRef {
        depth := 0
        for node := idx; node != noParent; node = s.nodes[node].parent {
                depth++
        }
        corridor := make([]PolyRef, depth)
        for node := idx; node != noParent; node = s.nodes[node].parent {
                depth--
                corridor[depth] = s.nodes[node].ref
        }

        return corridor
}

// astarResult is the outcome of the corridor search.
type astarResult struct {
        corridor []PolyRef
        // end is the effective end position of the corridor: the search
        // end for a reached goal, the entry position of the best partial
        // node for the closest-reachable answer.
        end      Pos
        reached  bool
        partial  bool
        explored int
        capped   bool
}

// emptyResult is the no-answer of the corridor search (a missing
// start polygon).
func emptyResult() astarResult {
        return astarResult{
                corridor: nil,
                end:      Pos{X: 0, Y: 0, Z: 0},
                reached:  false,
                partial:  false,
                explored: 0,
                capped:   false,
        }
}

// astarGoal selects the stop condition of the corridor search: the
// ordinary search stops at one target polygon, the water escape stops
// at the first ground polygon. The approach radius widens the
// ordinary goal: the search also stops on the first polygon whose
// closest surface point lies within the radius of the end position.
type astarGoal struct {
        target PolyRef
        escape bool
        // approach is the 3D radius of the FindPathApproach goal (the
        // merchant interaction distance of the town trips); zero keeps
        // the exact target contract.
        approach float64
}

// reached reports whether a settled polygon satisfies the goal.
func (g astarGoal) reached(poly *Poly, ref PolyRef) bool {
        if g.target != 0 {
                return ref == g.target
        }

        return g.escape && poly != nil && poly.Area == AreaGround
}

// approachReached reports whether a settled polygon already
// satisfies an approach radius goal: the closest surface point of
// the polygon lies within the 3D radius of the search end. This is
// the polygon-granularity form of the grid nodeReached rule (the
// first node within the approach radius of the target point): the
// walker may stand anywhere on the rectangle, so the closest point
// of the whole surface decides, not a cell center. The stacked-layer
// disambiguation rides the z axis of the test unchanged - a deck
// floating over the end position answers with its own height, the
// vertical gap keeps it outside the radius.
func (g astarGoal) approachReached(tile *Tile, poly *Poly, end Pos) bool {
        if g.approach <= 0 || tile == nil || poly == nil {
                return false
        }
        cx, cy, cz := tile.ClosestPoint(poly, end.X, end.Y, end.Z)
        dx := end.X - cx
        dy := end.Y - cy
        dz := end.Z - cz

        return math.Sqrt(dx*dx+dy*dy+dz*dz) <= g.approach
}

// astar runs the corridor search over the mesh. The step cost is the
// 3D distance between entry positions times the averaged area costs
// of the two polygons (the water pricing of the filter); the
// heuristic of the ordinary search is the 3D distance to the end
// position, the escape search runs with a zero heuristic (a priced
// flood whose first dry polygon is the cheapest way out). When the
// goal is not reached the answer carries the corridor to the node
// closest to the end position - the Detour partial result the dry
// searches turn into the closest reachable dry point.
func (m *Mesh) astar(
        state *queryState, goal astarGoal, startRef PolyRef, startPos Pos,
        endPos Pos, filter Filter, avoid avoidCtx, budget int,
        allow *confinedSet,
) astarResult {
        state.reset(goal.escape)
        startH := dist3(startPos, endPos)
        if goal.escape {
                startH = 0
        }
        startTile, startPoly := m.polyOfRef(startRef)
        if startTile == nil {
                return emptyResult()
        }
        state.create(startRef, noParent, startPos, 0, startH, startTile,
                startPoly)
        state.bestH = startH
        state.push(0)

        result := emptyResult()
        for {
                idx, ok := state.pop()
                if !ok {
                        break
                }
                node := &state.nodes[idx]
                if node.closed {
                        continue
                }
                node.closed = true
                result.explored++
                if goal.reached(node.poly, node.ref) ||
                        goal.approachReached(node.tile, node.poly, endPos) {
                        result.corridor = state.corridorOf(idx)
                        result.reached = true
                        // The ordinary search ends at the requested position; the
                        // escape ends at the crossing into the first dry polygon
                        // (the entry position of the reached node).
                        if goal.escape {
                                result.end = node.pos
                        } else {
                                result.end = endPos
                        }

                        return result
                }
                if len(state.nodes) >= budget {
                        result.capped = true

                        break
                }
                m.expand(state, node, idx, endPos, filter, avoid, allow)
        }

        if state.best != 0 {
                result.corridor = state.corridorOf(state.best)
                result.partial = true
                result.end = state.nodes[state.best].pos
        }

        return result
}

// expand relaxes every link of one settled node. The internal links
// resolve inside the node tile without any mesh lookup (the map and
// the lock only serve the rare cross region links). The avoid
// context walls the links onto banned polygons and prices the escape
// polygons of the ban holding the start (the recovery ban rules of
// the grid costTo - the rectangle granularity form, see avoid.go).
func (m *Mesh) expand(state *queryState, node *astarNode, idx uint32,
        endPos Pos, filter Filter, avoid avoidCtx, allow *confinedSet,
) {
        tile := node.tile
        poly := node.poly
        if tile == nil || poly == nil {
                return
        }
        areaCost := [2]float64{1, filter.WaterCost}
        for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
                link := &tile.Links[li]
                li = link.Next
                targetRef, targetTile, targetPoly := m.linkTarget(tile, link)
                if targetTile == nil {
                        continue
                }
                if !filter.AllowWater && targetPoly.Area == AreaWater {
                        continue
                }
                if allow != nil && !allow.allows(
                        RegionKey{Col: targetTile.Col, Row: targetTile.Row},
                        targetPoly) {
                        continue
                }
                ban := avoid.state(targetTile, targetPoly)
                if ban == avoidWall {
                        // The recovery ban: the live server proved this ground
                        // unwalkable for this session, the detour around it is
                        // the only plan worth planning.
                        continue
                }
                ax, ay, bx, by := tile.Portal(poly, link)
                midX, midY := (ax+bx)*0.5, (ay+by)*0.5
                midZ := tile.HeightAt(poly, midX, midY)
                mid := Pos{X: midX, Y: midY, Z: midZ}
                g := node.g + dist3(node.pos, mid)*
                        (areaCost[poly.Area]+areaCost[targetPoly.Area])*0.5
                if ban == avoidEscape {
                        // The ban that holds the start: the only honest route out
                        // of it crosses its own ground - expensive, never sealed.
                        g *= avoidEscapeMultiplier
                }
                h := dist3(mid, endPos)
                if state.escape {
                        h = 0
                }
                relax(state, idx, targetRef, targetTile, targetPoly, mid, g, h)
        }
}

// linkTarget resolves one link of a node polygon to its target
// reference, tile and polygon: the internal link stays inside the
// node tile (no mesh lookup), the external one resolves through the
// mesh. A link the mesh cannot resolve answers nil tiles - the caller
// treats it as a wall.
func (m *Mesh) linkTarget(
        tile *Tile, link *Link,
) (PolyRef, *Tile, *Poly) {
        if link.To >= 0 {
                if int(link.To) >= len(tile.Polys) {
                        return 0, nil, nil
                }

                return RefOf(tile.Col, tile.Row, uint32(link.To)), tile,
                        &tile.Polys[link.To]
        }
        targetRef := m.resolveLink(tile, link)
        if targetRef == 0 {
                return 0, nil, nil
        }
        targetTile, targetPoly := m.polyOfRef(targetRef)
        if targetTile == nil {
                return 0, nil, nil
        }

        return targetRef, targetTile, targetPoly
}

// relax relaxes one link target of the corridor search: a reference
// the search has not seen yet becomes a fresh node, an open one
// improves through the cheaper parent.
func relax(
        state *queryState, idx uint32, targetRef PolyRef,
        targetTile *Tile, targetPoly *Poly, mid Pos, g, h float64,
) {
        existing, ok := state.index.find(targetRef)
        if !ok {
                created := state.create(targetRef, idx, mid, g, h,
                        targetTile, targetPoly)
                state.push(created)

                return
        }
        if state.nodes[existing].closed {
                return
        }
        other := &state.nodes[existing]
        if g < other.g {
                other.g = g
                other.f = g + h
                other.parent = idx
                other.pos = mid
                state.fix(existing)
        }
}

// dist3 is the 3D distance between two positions.
func dist3(a, b Pos) float64 {
        dx := a.X - b.X
        dy := a.Y - b.Y
        dz := a.Z - b.Z

        return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
