// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// The hierarchical route query (docs/navmesh.md, the hierarchy
// section): the HNA* query model over the cluster graph. The coarse
// search runs the A* over the abstract nodes (the cluster blocks) and
// answers with the chain of portal edges it crossed; the refinement
// then searches the real mesh hop by hop through the chain - every
// hop is one ordinary corridor search from the previous portal
// midpoint to the next portal polygon, budgeted and cached (the
// HNA* intra-edge cache; the cached corridor of a portal pair serves
// every future route through it). A failed hop bans its exit edge
// and the coarse search replans, at most a few attempts; a route the
// hierarchy cannot finish answers the partial corridor up to the
// failure - the flat search semantics stay the contract.

// maxCoarseNodes bounds the cluster level A* of one query. The whole
// 165 region map holds 42240 cluster nodes at the 128 cell cluster
// granularity, so the bound never binds before the open set drains.
const maxCoarseNodes = 65536

// maxHopperNodes bounds one refinement hop: the portal to portal
// searches stay local (a cluster diagonal of distance), the bound
// only guards the pathological mazes.
const maxHopperNodes = 32768

// maxCoarseAttempts bounds the ban and replan cycles of one query.
const maxCoarseAttempts = 3


// maxConfinedNodes bounds the corridor confined fallback search: the
// allowed set keeps the exploration inside the coarse chain clusters,
// so the bound only guards the pathological mazes.
const maxConfinedNodes = 262144

// hopCacheCapacity bounds the cached refinement corridors (the
// portal pair answers that serve every later route through them).
const hopCacheCapacity = 4096

// coarseNode is one node of the cluster level search: the (cluster,
// entry component) pair - the component naming the polygon the search
// stands on when entering the cluster (the coarse chain through a
// cluster is verified polygon level connectivity, see abstract.go).
type coarseNode struct {
    key       clusterKey
    entryComp uint32
    parent    int32
    edgeRef   abstractEdgeRef
    pos       Pos
    g, f      float64
    closed    bool
    heapIdx   int32
}

// coarseNodeKey is the search state identity: the cluster and the
// component the search entered it through.
type coarseNodeKey struct {
    key  clusterKey
    comp uint32
}

// coarseState is the pooled search state of the cluster level.
type coarseState struct {
    nodes []coarseNode
    index map[coarseNodeKey]int32
    open  []int32
    best  int32
    bestH float64
}

// reset empties the coarse state for reuse.
func (s *coarseState) reset() {
    s.nodes = s.nodes[:0]
    clear(s.index)
    s.open = s.open[:0]
    s.best = -1
    s.bestH = math.MaxFloat64
}

// push inserts a node index into the open heap.
func (s *coarseState) push(idx int32) {
    if s.nodes[idx].heapIdx >= 0 {
        return
    }
    s.open = append(s.open, idx)
    i := int32(len(s.open) - 1)
    s.nodes[idx].heapIdx = i
    for i > 0 {
        parent := (i - 1) / 2
        if s.nodes[s.open[i]].f < s.nodes[s.open[parent]].f {
            s.swap(i, parent)
            i = parent
        } else {
            break
        }
    }
}

// fix restores the heap property of a node whose score improved.
func (s *coarseState) fix(idx int32) {
    i := s.nodes[idx].heapIdx
    if i < 0 {
        s.push(idx)

        return
    }
    for i > 0 {
        parent := (i - 1) / 2
        if s.nodes[s.open[i]].f < s.nodes[s.open[parent]].f {
            s.swap(i, parent)
            i = parent
        } else {
            break
        }
    }
}

// pop removes and returns the best open node index.
func (s *coarseState) pop() (int32, bool) {
    if len(s.open) == 0 {
        return 0, false
    }
    top := s.open[0]
    s.nodes[top].heapIdx = -1
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
            if int(left) < len(s.open) &&
                s.nodes[s.open[left]].f < s.nodes[s.open[best]].f {
                best = left
            }
            if int(right) < len(s.open) &&
                s.nodes[s.open[right]].f < s.nodes[s.open[best]].f {
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

// swap exchanges two heap entries and fixes their indices.
func (s *coarseState) swap(i, j int32) {
    s.open[i], s.open[j] = s.open[j], s.open[i]
    s.nodes[s.open[i]].heapIdx = i
    s.nodes[s.open[j]].heapIdx = j
}

// coarseResult is the outcome of the cluster level search.
type coarseResult struct {
    // edges is the crossed portal chain, start to goal (the best
    // partial chain when the goal cluster is unreachable).
    edges []abstractEdgeRef
    // clusters is the cluster key walk of the same chain (one more
    // entry than the edges: the start cluster first) - the confined
    // fallback search allows exactly these clusters.
    clusters []clusterKey
    explored int
    reached  bool
}

// hierQuery carries one hierarchical route query.
type hierQuery struct {
    startRef PolyRef
    startPos Pos
    endRef   PolyRef
    endPos   Pos
    approach float64
    filter   Filter
    state    *queryState
    coarse   *coarseState
    bans     map[abstractEdgeRef]bool
}

// hierWorthy reports whether the route goes through the hierarchy:
// the avoid bans wall polygons at the mesh granularity the coarse
// graph cannot see, so those queries stay flat.
func hierWorthy(startRef, endRef PolyRef, startPos, endPos Pos,
    filter Filter,
) bool {
    if len(filter.Avoid) > 0 {
        return false
    }
    sc, sr := TileOf(startRef)
    ec, er := TileOf(endRef)
    if sc != ec || sr != er {
        return true
    }

    return dist2D(startPos, endPos) > hierRouteThreshold
}

// routeHierarchical runs the hierarchical query: the coarse chain and
// the hop refinement, stitched into the ordinary route answer. The
// answer is nil when the hierarchy declines (an abstract graph could
// not build, or not even the first hop produced a corridor) - the
// caller falls back to the flat search.
func (m *Mesh) routeHierarchical(
    startRef PolyRef, startPos Pos, endRef PolyRef, endPos Pos,
    approach float64, filter Filter, state *queryState,
) *Route {
    col, row := TileOf(startRef)
    startAbstract := m.abstractOf(RegionKey{Col: col, Row: row})
    if startAbstract == nil {
        return nil
    }

    coarse := m.acquireCoarse()
    defer m.releaseCoarse(coarse)
    query := hierQuery{
        startRef: startRef,
        startPos: startPos,
        endRef:   endRef,
        endPos:   endPos,
        approach: approach,
        filter:   filter,
        state:    state,
        coarse:   coarse,
        bans:     make(map[abstractEdgeRef]bool),
    }

    route := &Route{
        Found:        false,
        Partial:      false,
        Waypoints:    nil,
        RawWaypoints: nil,
        Corridor:     nil,
        Explored:     0,
        Hierarchical: true,
    }
    var chain coarseResult
    for range maxCoarseAttempts {
        chain = m.coarseChain(&query, endRef, endPos)
        route.Explored += chain.explored
        if len(chain.edges) == 0 {
            // The cluster level found nothing at all: the flat search
            // holds the honest answer for this start (an isolated
            // surface the abstract graph also cannot leave).
            return nil
        }
        if m.refineChain(&query, chain, route) {
            return route
        }
        // A hop failed: its exit edge is banned, the replan walks a
        // different portal chain.
    }

    // The attempts are exhausted: the representative crossings of a
    // cluster pair need not be co-reachable at the polygon level (the
    // dedupe keeps one crossing per class). The confined search
    // threads the real mesh through the chain clusters instead,
    // choosing the crossings that actually connect.
    if route.Corridor == nil && m.confinedRoute(&query, chain.clusters,
        route) {
        return route
    }
    if route.Corridor == nil {
        return nil
    }
    route.Partial = true
    m.answerWaypoints(route, route.Corridor, query.startPos,
        query.endPos, query.filter)

    return route
}

// coarseChain runs the cluster level A* and returns the portal edge
// chain from the start cluster toward the goal cluster (the best
// partial chain when the goal is unreachable).
func (m *Mesh) coarseChain(q *hierQuery, endRef PolyRef,
    endPos Pos,
) coarseResult {
    result := coarseResult{edges: nil, clusters: nil, explored: 0,
        reached: false}
    col, row := TileOf(q.startRef)
    startKey := clusterOfPos(col, row, q.startPos.X, q.startPos.Y)
    goalCol, goalRow := TileOf(endRef)
    goalKey := clusterOfPos(goalCol, goalRow, endPos.X, endPos.Y)

    q.coarse.reset()
    startAbstract := m.abstractOf(RegionKey{Col: col, Row: row})
    if startAbstract == nil {
        return result
    }
    if _, ok := startAbstract.index[startKey.ID]; !ok {
        return result
    }
    goalAbstract := m.abstractOf(RegionKey{Col: goalCol, Row: goalRow})
    if goalAbstract == nil {
        return result
    }
    // The search states carry the component the crossing stood on:
    // the start stands on the start polygon's component, the goal
    // terminal answers only the entry into the end polygon's one.
    startComp := uint32(0)
    if startIdx := PolyOf(q.startRef); startIdx >= 0 &&
        int(startIdx) < len(startAbstract.comps) {
        startComp = startAbstract.comps[startIdx]
    }
    goalComp := uint32(0)
    if endIdx := PolyOf(endRef); endIdx >= 0 &&
        int(endIdx) < len(goalAbstract.comps) {
        goalComp = goalAbstract.comps[endIdx]
    }
    q.coarse.nodes = append(q.coarse.nodes, coarseNode{
        key:       startKey,
        entryComp: startComp,
        parent:    -1,
        edgeRef: abstractEdgeRef{Region: RegionKey{Col: 0, Row: 0},
            Index: 0},
        pos:     q.startPos,
        g:       0,
        f:       dist3(q.startPos, endPos),
        closed:  false,
        heapIdx: -1,
    })
    q.coarse.index[coarseNodeKey{key: startKey, comp: startComp}] = 0
    q.coarse.push(0)

    for {
        idx, ok := q.coarse.pop()
        if !ok {
            break
        }
        node := &q.coarse.nodes[idx]
        if node.closed {
            continue
        }
        node.closed = true
        result.explored++
        if node.key == goalKey && node.entryComp == goalComp {
            result.reached = true
            result.edges, result.clusters = coarseChainWalk(q.coarse, idx)

            return result
        }
        if len(q.coarse.nodes) >= maxCoarseNodes {
            break
        }
        m.expandCoarse(q, idx, endPos)
    }
    if q.coarse.best >= 0 {
        result.edges, result.clusters = coarseChainWalk(q.coarse,
            q.coarse.best)
    }

    return result
}

// expandCoarse relaxes the coarse edges of one settled cluster node.
func (m *Mesh) expandCoarse(q *hierQuery, idx int32, endPos Pos) {
    node := &q.coarse.nodes[idx]
    abstract := m.abstractOf(RegionKey{Col: node.key.Col,
        Row: node.key.Row})
    if abstract == nil {
        return
    }
    local, ok := abstract.index[node.key.ID]
    if !ok {
        return
    }
    areaCost := [2]float64{1, q.filter.WaterCost}
    for _, ei := range abstract.nodes[local].edges {
        edge := &abstract.edges[ei]
        ref := abstractEdgeRef{Region: abstract.key, Index: ei}
        if q.bans[ref] {
            continue
        }
        if edge.srcComp != node.entryComp {
            // The component gate: the crossing the search stands on
            // and this crossing live in different link components of
            // the cluster - no polygon path connects them, chaining
            // the edge would fake the connectivity.
            continue
        }
        if !q.filter.AllowWater && (edge.srcArea == AreaWater ||
            edge.dstArea == AreaWater) {
            // The dry search never enters the water polygons (the
            // unknown external target area is ground priced, the
            // refinement hop decides it for real).
            continue
        }
        dstComp := edge.dstComp
        if dstComp == 0 {
            // The external target: its component lives in the
            // neighbour abstract (resolved once, cached).
            dstComp = m.edgeDstComp(ref, edge)
            if dstComp == 0 {
                continue
            }
        }
        dstArea := edge.dstArea
        if dstArea == unknownArea {
            dstArea = AreaGround
        }
        nodeKey := coarseNodeKey{key: edge.to, comp: dstComp}
        existing, seen := q.coarse.index[nodeKey]
        if seen && q.coarse.nodes[existing].closed {
            continue
        }
        cost := dist3(node.pos, edge.mid) *
            (areaCost[edge.srcArea] + areaCost[dstArea]) * 0.5
        g := node.g + cost
        h := dist3(edge.mid, endPos)
        if !seen {
            created := int32(len(q.coarse.nodes))
            q.coarse.nodes = append(q.coarse.nodes, coarseNode{
                key:       edge.to,
                entryComp: dstComp,
                parent:    idx,
                edgeRef:   ref,
                pos:       edge.mid,
                g:         g,
                f:         g + h,
                closed:    false,
                heapIdx:   -1,
            })
            q.coarse.index[nodeKey] = created
            q.coarse.push(created)
            if h < q.coarse.bestH {
                q.coarse.bestH = h
                q.coarse.best = created
            }

            continue
        }
        known := &q.coarse.nodes[existing]
        if g < known.g {
            known.g = g
            known.f = g + h
            known.parent = idx
            known.edgeRef = ref
            known.pos = edge.mid
            q.coarse.fix(existing)
        }
    }
}

// edgeDstComp resolves and caches the link component of an external
// edge target: the target polygon index rides the source tile's
// external link record, the component labels live in the neighbour
// abstract (built once, cached forever). Zero answers when the
// neighbour is absent (the coarse search treats the edge as a wall,
// the refinement semantics for a missing neighbour).
func (m *Mesh) edgeDstComp(ref abstractEdgeRef, edge *abstractEdge) uint32 {
    m.hierMu.Lock()
    cached, ok := m.dstComps[ref]
    m.hierMu.Unlock()
    if ok {
        return cached
    }
    comp := uint32(0)
    if edge.extPoly != 0 && edge.extPoly != 0xFFFFFFFF {
        neighbour := m.abstractOf(RegionKey{Col: edge.extCol,
            Row: edge.extRow})
        if neighbour != nil && int(edge.extPoly) < len(neighbour.comps) {
            comp = neighbour.comps[edge.extPoly]
        }
    }
    m.hierMu.Lock()
    m.dstComps[ref] = comp
    m.hierMu.Unlock()

    return comp
}

// coarseChainWalk walks the parent chain and returns the crossed edge
// refs and the cluster keys from the start cluster to the node (the
// edges goal first, the clusters start first after the flip).
func coarseChainWalk(state *coarseState,
    idx int32,
) ([]abstractEdgeRef, []clusterKey) {
    depth := 0
    for n := idx; n >= 0; n = state.nodes[n].parent {
        if state.nodes[n].parent >= 0 {
            depth++
        }
    }
    edges := make([]abstractEdgeRef, 0, depth)
    clusters := make([]clusterKey, 0, depth+1)
    for n := idx; n >= 0; n = state.nodes[n].parent {
        clusters = append(clusters, state.nodes[n].key)
        if state.nodes[n].parent >= 0 {
            edges = append(edges, state.nodes[n].edgeRef)
        }
    }
    // The chains walked goal first; reverse into the start order.
    for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
        edges[i], edges[j] = edges[j], edges[i]
    }
    for i, j := 0, len(clusters)-1; i < j; i, j = i+1, j-1 {
        clusters[i], clusters[j] = clusters[j], clusters[i]
    }

    return edges, clusters
}

// confinedSet is the allowed cluster set of the fallback search: one
// bitmap per region, the coarse chain clusters marked.
type confinedSet struct {
    regions map[RegionKey]*clusterBitmap
}

// clusterBitmap marks the allowed clusters of one region.
type clusterBitmap = [clustersPerSide][clustersPerSide]bool

// newConfinedSet marks the chain clusters.
func newConfinedSet(clusters []clusterKey) *confinedSet {
    set := &confinedSet{regions: make(map[RegionKey]*clusterBitmap)}
    for _, key := range clusters {
        regionKey := RegionKey{Col: key.Col, Row: key.Row}
        bm := set.regions[regionKey]
        if bm == nil {
            bm = &clusterBitmap{}
            set.regions[regionKey] = bm
        }
        bm[key.ID>>4][key.ID&0x0F] = true
    }

    return set
}

// allows reports whether the polygon footprint touches any allowed
// cluster of its region (a polygon spanning the whole region always
// does - the giant sea and field rectangles).
func (cs *confinedSet) allows(key RegionKey, p *Poly) bool {
    bm := cs.regions[key]
    if bm == nil {
        return false
    }
    x0, x1 := p.X0/clusterCellsSide, (p.X1-1)/clusterCellsSide
    y0, y1 := p.Y0/clusterCellsSide, (p.Y1-1)/clusterCellsSide
    if x1-x0 >= clustersPerSide-1 && y1-y0 >= clustersPerSide-1 {
        return true
    }
    for cx := x0; cx <= x1; cx++ {
        for cy := y0; cy <= y1; cy++ {
            if bm[cx][cy] {
                return true
            }
        }
    }

    return false
}

// confinedRoute runs the fallback search: one fine A* over the mesh
// with the expansion confined to the coarse chain clusters. The
// coarse level already answered the connectivity question; the
// confined search threads the real polygons through the chain,
// choosing the crossings the representative hops failed to pair.
func (m *Mesh) confinedRoute(q *hierQuery, clusters []clusterKey,
    route *Route,
) bool {
    if len(clusters) == 0 {
        return false
    }
    allow := newConfinedSet(clusters)
    result := m.astar(q.state,
        astarGoal{target: q.endRef, escape: false, approach: q.approach},
        q.startRef, q.startPos, q.endPos, q.filter, noAvoid(),
        maxConfinedNodes, allow)
    if result.corridor == nil {
        return false
    }
    if !result.reached && !result.partial {
        return false
    }
    route.Corridor = result.corridor
    route.Explored += result.explored
    route.Found = result.reached
    route.Partial = !result.reached
    m.answerWaypoints(route, route.Corridor, q.startPos, q.endPos,
        q.filter)

    return true
}

// refineChain walks the coarse edge chain and fills the route with
// the stitched refinement corridors. The answer reports whether the
// route is finished (found or the final partial); a false answer
// bans the failed hop exit and asks for the replan.
func (m *Mesh) refineChain(q *hierQuery, chain coarseResult,
    route *Route,
) bool {
    corridor := make([]PolyRef, 0, 64+len(chain.edges)*8)
    currentRef := q.startRef
    currentPos := q.startPos
    var prevEdge abstractEdgeRef
    for i := range chain.edges {
        exitRef := chain.edges[i]
        exitEdge, ok := m.hierEdge(exitRef)
        if !ok {
            q.bans[exitRef] = true

            return false
        }
        last := i == len(chain.edges)-1
        goalRef := q.endRef
        goalPos := q.endPos
        if !last {
            goalRef = exitEdge.toRef
            if goalRef == 0 {
                goalRef = m.resolveEdgeTarget(exitRef, exitEdge)
                if goalRef == 0 {
                    q.bans[exitRef] = true

                    return false
                }
            }
            goalPos = exitEdge.mid
        }
        segment := m.hopCorridor(q, currentRef, currentPos,
            prevEdge, exitRef, goalRef, goalPos, i > 0, last)
        if segment == nil || (!segment.reached &&
            (!last || !segment.partial)) {
            // The hop into the next portal failed: ban the exit edge
            // and replan (the last hop keeps its partial answer).
            q.bans[exitRef] = true

            return false
        }
        route.Explored += segment.explored
        if last && segment.reached {
            route.Found = true
        }
        start := 0
        if len(corridor) > 0 && len(segment.corridor) > 0 &&
            corridor[len(corridor)-1] == segment.corridor[0] {
            start = 1
        }
        corridor = append(corridor, segment.corridor[start:]...)
        currentRef = goalRef
        currentPos = goalPos
        prevEdge = exitRef
    }

    route.Corridor = corridor
    route.Partial = !route.Found
    m.answerWaypoints(route, route.Corridor, q.startPos, q.endPos,
        q.filter)

    return true
}

// hopSegment is one refinement answer: the polygon corridor plus the
// search bookkeeping (the coarse level consumed the reachability
// question, the hop only needs the corridor).
type hopSegment struct {
    corridor []PolyRef
    explored int
    reached  bool
    partial  bool
}

// hopCorridor answers one portal to portal corridor: the cached
// intra-edge answer when the pair ran before (the HNA* intra-edge
// cache), a fresh budgeted search otherwise. Only the interior hops
// cache - the pair of their entry and exit edges pins the exact
// start and goal polygons and midpoints of the segment; the first
// and the last segments search from the real positions.
func (m *Mesh) hopCorridor(q *hierQuery, fromRef PolyRef, fromPos Pos,
    entry, exit abstractEdgeRef, toRef PolyRef, toPos Pos,
    cacheable, last bool,
) *hopSegment {
    dry := !q.filter.AllowWater
    var key hopKey
    if cacheable {
        key = hopKey{from: entry, to: exit, dry: dry}
        if cached, ok := m.cachedHop(key); ok {
            return &hopSegment{
                corridor: cached,
                explored: 0,
                reached:  true,
                partial:  false,
            }
        }
    }

    segment := m.runHop(q, fromRef, fromPos, toRef, toPos, last)
    if segment == nil {
        return nil
    }
    if cacheable && segment.reached && len(segment.corridor) > 0 {
        m.storeHop(key, segment.corridor)
    }

    return segment
}

// runHop searches one hop on the mesh: the ordinary corridor search
// with the hop budget (the approach radius rides on the last hop
// only - the portal goals are exact).
func (m *Mesh) runHop(q *hierQuery, fromRef PolyRef, fromPos Pos,
    toRef PolyRef, toPos Pos, last bool,
) *hopSegment {
    goal := astarGoal{target: toRef, escape: false, approach: 0}
    if last {
        goal.approach = q.approach
    }
    result := m.astar(q.state, goal, fromRef, fromPos, toPos, q.filter,
        noAvoid(), maxHopperNodes, nil)
    if result.corridor == nil {
        return nil
    }

    return &hopSegment{
        corridor: result.corridor,
        explored: result.explored,
        reached:  result.reached,
        partial:  result.partial,
    }
}

// hierEdge loads one coarse edge by reference (its region abstract
// must exist - the chain just walked it).
func (m *Mesh) hierEdge(ref abstractEdgeRef) (*abstractEdge, bool) {
    abstract := m.abstractOf(ref.Region)
    if abstract == nil || ref.Index < 0 ||
        int(ref.Index) >= len(abstract.edges) {
        return nil, false
    }

    return &abstract.edges[ref.Index], true
}

// resolveEdgeTarget resolves the crossing target polygon of an
// external edge: the source tile decodes on demand, the missing
// neighbour answers the null reference (the hop then fails and the
// edge bans).
func (m *Mesh) resolveEdgeTarget(ref abstractEdgeRef,
    edge *abstractEdge,
) PolyRef {
    tile, err := m.Tile(ref.Region)
    if err != nil || tile == nil {
        return 0
    }
    if int(edge.link) >= len(tile.Links) {
        return 0
    }
    target, _, _ := m.linkTarget(tile, &tile.Links[edge.link])

    return target
}

// hopKey names one cached refinement corridor: the entry edge (its
// target polygon and midpoint start the hop), the exit edge (its
// target polygon and midpoint end it) and the water class.
type hopKey struct {
    from abstractEdgeRef
    to   abstractEdgeRef
    dry  bool
}

// cachedHop answers the cached corridor of a hop key.
func (m *Mesh) cachedHop(key hopKey) ([]PolyRef, bool) {
    m.hierMu.Lock()
    defer m.hierMu.Unlock()
    corridor, ok := m.hops[key]
    if !ok {
        return nil, false
    }
    // The copy keeps the cache immutable against the callers.
    out := make([]PolyRef, len(corridor))
    copy(out, corridor)

    return out, true
}

// storeHop caches the corridor of a portal pair.
func (m *Mesh) storeHop(key hopKey, corridor []PolyRef) {
    m.hierMu.Lock()
    defer m.hierMu.Unlock()
    if _, ok := m.hops[key]; ok {
        return
    }
    if len(m.hopOrder) >= hopCacheCapacity {
        oldest := m.hopOrder[0]
        m.hopOrder = m.hopOrder[1:]
        delete(m.hops, oldest)
    }
    stored := make([]PolyRef, len(corridor))
    copy(stored, corridor)
    m.hops[key] = stored
    m.hopOrder = append(m.hopOrder, key)
}

// dist2D is the horizontal distance between two positions.
func dist2D(a, b Pos) float64 {
    dx := a.X - b.X
    dy := a.Y - b.Y

    return math.Sqrt(dx*dx + dy*dy)
}
