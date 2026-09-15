// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// maxQueryNodes bounds the A* node count of one search. The corridors
// of real routes hold hundreds of polygons; the bound exists so a
// pathological filter (or a mesh bug) answers a partial route instead
// of walking every polygon of every loaded region.
const maxQueryNodes = 65536

// astarNode is one node of the corridor search: the polygon, its
// entry position (the portal midpoint), the parent, the costs and the
// open heap bookkeeping.
type astarNode struct {
    ref     PolyRef
    parent  uint32
    pos     Pos
    g       float64
    f       float64
    closed  bool
    heapIdx int32
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
    index  map[PolyRef]uint32
    open   []uint32
    best   uint32
    bestH  float64
    escape bool
}

// reset empties the state for reuse.
func (s *queryState) reset(escape bool) {
    s.nodes = s.nodes[:0]
    clear(s.index)
    s.open = s.open[:0]
    s.best = 0
    s.bestH = math.MaxFloat64
    s.escape = escape
}

// create appends a fresh node for a reference (the caller guarantees
// the reference is new).
func (s *queryState) create(ref PolyRef, parent uint32, pos Pos,
    g, h float64,
) uint32 {
    s.nodes = append(s.nodes, astarNode{
        ref:     ref,
        parent:  parent,
        pos:     pos,
        g:       g,
        f:       g + h,
        closed:  false,
        heapIdx: notInHeap,
    })
    idx := uint32(len(s.nodes) - 1)
    s.index[ref] = idx
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

// astarGoal selects the stop condition of the corridor search: the
// ordinary search stops at one target polygon, the water escape stops
// at the first ground polygon.
type astarGoal struct {
    target PolyRef
    escape bool
}

// reached reports whether a settled polygon satisfies the goal.
func (g astarGoal) reached(poly *Poly, ref PolyRef) bool {
    if g.target != 0 {
        return ref == g.target
    }

    return g.escape && poly != nil && poly.Area == AreaGround
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
    endPos Pos, filter Filter,
) astarResult {
    state.reset(goal.escape)
    startH := dist3(startPos, endPos)
    if goal.escape {
        startH = 0
    }
    state.create(startRef, noParent, startPos, 0, startH)
    state.bestH = startH
    state.push(0)

    result := astarResult{
        corridor: nil,
        end:      Pos{},
        reached:  false,
        partial:  false,
        explored: 0,
        capped:   false,
    }
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
        _, poly := m.polyOfRef(node.ref)
        if goal.reached(poly, node.ref) {
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
        if len(state.nodes) >= maxQueryNodes {
            result.capped = true

            break
        }
        m.expand(state, node, idx, endPos, filter)
    }

    if state.best != 0 {
        result.corridor = state.corridorOf(state.best)
        result.partial = true
        result.end = state.nodes[state.best].pos
    }

    return result
}

// expand relaxes every link of one settled node.
func (m *Mesh) expand(state *queryState, node *astarNode, idx uint32,
    endPos Pos, filter Filter,
) {
    tile, poly := m.polyOfRef(node.ref)
    if tile == nil {
        return
    }
    areaCost := [2]float64{1, filter.WaterCost}
    for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
        link := &tile.Links[li]
        li = link.Next
        targetRef := m.resolveLink(tile, link)
        if targetRef == 0 {
            continue
        }
        targetTile, targetPoly := m.polyOfRef(targetRef)
        if targetTile == nil {
            continue
        }
        if !filter.AllowWater && targetPoly.Area == AreaWater {
            continue
        }
        ax, ay, bx, by := tile.Portal(poly, link)
        midX, midY := (ax+bx)*0.5, (ay+by)*0.5
        midZ := tile.HeightAt(poly, midX, midY)
        mid := Pos{X: midX, Y: midY, Z: midZ}
        g := node.g + dist3(node.pos, mid)*
            (areaCost[poly.Area]+areaCost[targetPoly.Area])*0.5
        h := dist3(mid, endPos)
        if state.escape {
            h = 0
        }
        existing, ok := state.index[targetRef]
        if !ok {
            created := state.create(targetRef, idx, mid, g, h)
            state.push(created)
        } else if !state.nodes[existing].closed {
            other := &state.nodes[existing]
            if g < other.g {
                other.g = g
                other.f = g + h
                other.parent = idx
                other.pos = mid
                state.fix(existing)
            }
        }
    }
}

// dist3 is the 3D distance between two positions.
func dist3(a, b Pos) float64 {
    dx := a.X - b.X
    dy := a.Y - b.Y
    dz := a.Z - b.Z

    return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
