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

// gatedCoarsePops bounds the component gated guide attempt: the gate
// keeps the chain honest where the sampling serves it, and the cap
// hands the pathological pairs (the sparse cluster crossings the
// component chain strands) to the gate free retry fast.
const gatedCoarsePops = 4096

// The coarse guide prices the cluster edges uniformly (the plain 3D
// distance, no area multipliers): the refinement hops run the real
// priced searches, the guide only has to point the direction - and a
// uniform guide keeps the plain dist3 heuristic consistent, which the
// swim pricing (3x on the real edges) breaks (the honest water route
// costs 3x the straight line and the plain A* answers by flooding
// every land cluster the underpriced heuristic cannot prune - the
// half map wave the town route measurements drowned in).
const coarseGuideUniform = true

// maxConfinedNodes bounds the corridor confined fallback search: the
// allowed set keeps the exploration inside the coarse chain clusters,
// so the bound only guards the pathological mazes. The whole map town
// legs thread half a million real polygons through their chain - the
// bound rides at a million expansions (the node memory is the flat
// search's, the pooled state carries it).
const maxConfinedNodes = 1 << 20

// hopCacheCapacity bounds the cached refinement corridors (the
// portal pair answers that serve every later route through them).
const hopCacheCapacity = 4096

// coarseNode is one node of the cluster level search: the (cluster,
// entering crossing) pair. The entry component (the link component of
// the crossing target polygon) resolves when the node pops - the
// eager neighbour abstract build the relaxation would force is the
// whole frontier cost the lazy resolution avoids (the search builds
// the abstracts of the clusters it actually settles, not of every
// cluster the frontier touches).
type coarseNode struct {
    key       clusterKey
    entryPoly uint32
    entryComp uint32
    parent    int32
    edgeRef   abstractEdgeRef
    pos       Pos
    g, f      float64
    closed    bool
    heapIdx   int32
}

// coarseNodeKey is the search state identity: the cluster and the
// crossing the search entered it through (the entry component merges
// the duplicate crossings at pop time).
type coarseNodeKey struct {
    key  clusterKey
    edge abstractEdgeRef
}

// coarseCompKey is the settled (cluster, entry component) identity of
// the pop time dedupe: several crossings into one cluster sharing the
// component expand it once.
type coarseCompKey struct {
    key  clusterKey
    comp uint32
}

// coarseState is the pooled search state of the cluster level.
type coarseState struct {
    nodes   []coarseNode
    index   map[coarseNodeKey]int32
    settled map[coarseCompKey]bool
    open    []int32
    best    int32
    bestH   float64
}

// reset empties the coarse state for reuse.
func (s *coarseState) reset() {
    s.nodes = s.nodes[:0]
    clear(s.index)
    clear(s.settled)
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
    // gaveUp reports the gated guide budget stop (the gate free
    // retry takes over).
    gaveUp bool
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
// graph cannot see, so those queries stay flat. The cross tile
// queries go hierarchical first (a flat search over a multi tile
// corridor only caps into the partial answer); the same tile queries
// stay flat and the budget cap escalates them (RouteApproach) - the
// within tile measurements answer the flat search faster and with
// the shorter corridors when the target fits the budget.
func hierWorthy(startRef, endRef PolyRef, startPos, endPos Pos,
    filter Filter,
) bool {
    if len(filter.Avoid) > 0 {
        return false
    }
    sc, sr := TileOf(startRef)
    ec, er := TileOf(endRef)

    return sc != ec || sr != er
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

    // The line corridor prefetch: the coarse search loads the
    // region cluster graphs lazily per settled node; the regions
    // the straight segment crosses are the common frontier, the
    // parallel sidecar load serves them from the cache (the cap
    // bounds the half world diagonals; the RAM bound stays the
    // abstract LRU).
    m.PrefetchAbstracts(lineRegionKeys(startPos, endPos))

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
    // The gated attempts first: the component verified chains (the
    // honest connectivity the sampling serves) with the ban and
    // replan cycles of a failed hop.
    var chain coarseResult
    gaveUp := false
    for range maxCoarseAttempts {
        chain = m.coarseChain(&query, endRef, endPos, true)
        route.Explored += chain.explored
        if chain.gaveUp {
            gaveUp = true

            break
        }
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
    if gaveUp {
        // The gated guide stranded (the sparse cluster crossings the
        // component chain needs): the gate free chain samples the
        // crossings plain and the refinement verifies them on the
        // real mesh - the hop failures ban the fake crossings and the
        // confined fallback threads the chain clusters.
        clear(query.bans)
        for range maxCoarseAttempts {
            chain = m.coarseChain(&query, endRef, endPos, false)
            route.Explored += chain.explored
            if len(chain.edges) == 0 {
                return nil
            }
            if m.refineChain(&query, chain, route) {
                return route
            }
        }
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
// partial chain when the goal is unreachable). The gated pass caps
// the exploration (the component sampling of the sparse cluster
// pairs strands the honest search into a whole map flood - the gate
// free retry chains the crossings and the confined refinement
// restores the connectivity on the real mesh).
func (m *Mesh) coarseChain(q *hierQuery, endRef PolyRef,
    endPos Pos, gated bool,
) coarseResult {
    result := coarseResult{edges: nil, clusters: nil, explored: 0,
        reached: false}
    straight := dist3(q.startPos, q.endPos)
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
    // the start stands on the start polygon's component (resolved
    // here, the start abstract is built), the crossing entries
    // resolve at pop time through the node tile abstract.
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
        entryPoly: uint32(PolyOf(q.startRef)),
        entryComp: startComp,
        parent:    -1,
        edgeRef:   abstractEdgeRef{},
        pos:       q.startPos,
        g:         0,
        f:         dist3(q.startPos, endPos),
        closed:    false,
        heapIdx:   -1,
    })
    q.coarse.index[coarseNodeKey{key: startKey, edge: abstractEdgeRef{}}] = 0
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

        if gated {
            if node.entryComp == 0 {
                // The lazy entry resolution: the crossing target
                // poly's link component lives in the node tile
                // abstract - the same build the expansion needs, so
                // the settle order pays the decode once per searched
                // tile.
                abstract := m.abstractOf(RegionKey{Col: node.key.Col,
                    Row: node.key.Row})
                if abstract != nil && int(node.entryPoly) <
                    len(abstract.comps) {
                    node.entryComp = abstract.comps[node.entryPoly]
                }
            }
            // The settled component merge: another crossing into this
            // cluster already expanded the same component - the chain
            // through this node would duplicate that work.
            compKey := coarseCompKey{key: node.key, comp: node.entryComp}
            if q.coarse.settled[compKey] {
                continue
            }
            q.coarse.settled[compKey] = true
            if node.key == goalKey && node.entryComp == goalComp {
                result.reached = true
                result.edges, result.clusters = coarseChainWalk(q.coarse, idx)

                return result
            }
        } else if node.key == goalKey {
            // The gate free pass settles clusters plain (the classic
            // HPA* chain): the refinement hops verify the crossings
            // on the real mesh.
            result.reached = true
            result.edges, result.clusters = coarseChainWalk(q.coarse, idx)

            return result
        }
        if len(q.coarse.nodes) >= maxCoarseNodes {
            break
        }
        if gated && (result.explored >= gatedCoarsePops ||
            node.f > 1.5*straight+8192) {
            // The gated guide gave up: the gate free retry
            // takes over (routeHierarchical drives it).
            result.gaveUp = true

            return result
        }
        m.expandCoarse(q, idx, endPos, gated)
    }
    if q.coarse.best >= 0 {
        result.edges, result.clusters = coarseChainWalk(q.coarse,
            q.coarse.best)
    }

    return result
}

// expandCoarse relaxes the coarse edges of one settled cluster node.
// The gated pass verifies the cluster connectivity through the link
// components; the gate free pass chains the sampled crossings and
// lets the refinement hops verify them on the real mesh.
func (m *Mesh) expandCoarse(q *hierQuery, idx int32, endPos Pos,
    gated bool,
) {
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
        // The node identity rides the crossing (the component gate
        // resolves at pop time, the neighbour abstract stays unbuilt
        // until the search actually settles the cluster).
        nodeKey := coarseNodeKey{key: edge.to, edge: ref}
        existing, seen := q.coarse.index[nodeKey]
        if seen && q.coarse.nodes[existing].closed {
            continue
        }
        cost := dist3(node.pos, edge.mid)
        g := node.g + cost
        h := dist3(edge.mid, endPos)
        if !seen {
            created := int32(len(q.coarse.nodes))
            q.coarse.nodes = append(q.coarse.nodes, coarseNode{
                key:       edge.to,
                entryPoly: edgeEntryPoly(edge),
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

// edgeEntryPoly names the polygon index (within the target cluster's
// tile) whose link component the crossing enters through: the
// external edges (they carry the neighbour region key) name the
// neighbour polygon of the source tile's link record, the internal
// ones carry the full target reference.
func edgeEntryPoly(edge *abstractEdge) uint32 {
    if edge.extCol != 0 || edge.extRow != 0 {
        return edge.extPoly
    }
    if edge.toRef != 0 {
        if poly := PolyOf(edge.toRef); poly >= 0 {
            return uint32(poly)
        }
    }

    return 0
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
    // The one cluster dilation: the coarse chain samples the cluster
    // crossings - the honest corridor between them leaves the chain's
    // own cluster blocks where the sampled crossings sit a block off
    // the real path (the bay detours the geodata holes force). The
    // dilation keeps the confinement meaningful (the goal direction)
    // while the neighbourhood absorbs the sampling slack.
    set := &confinedSet{regions: make(map[RegionKey]*clusterBitmap)}
    add := func(col, row int16, cx, cy int32) {
        if cx < 0 || cx >= clustersPerSide || cy < 0 ||
            cy >= clustersPerSide {
            return
        }
        regionKey := RegionKey{Col: col, Row: row}
        bm := set.regions[regionKey]
        if bm == nil {
            bm = &clusterBitmap{}
            set.regions[regionKey] = bm
        }
        bm[cx][cy] = true
    }
    for _, key := range clusters {
        cx, cy := int32(key.ID>>4), int32(key.ID&0x0F)
        for dx := int32(-1); dx <= 1; dx++ {
            for dy := int32(-1); dy <= 1; dy++ {
                nx, ny := cx+dx, cy+dy
                col, row := key.Col, key.Row
                if nx < 0 {
                    col--
                    nx += clustersPerSide
                }
                if nx >= clustersPerSide {
                    col++
                    nx -= clustersPerSide
                }
                if ny < 0 {
                    row--
                    ny += clustersPerSide
                }
                if ny >= clustersPerSide {
                    row++
                    ny -= clustersPerSide
                }
                add(col, row, nx, ny)
            }
        }
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

// lineRegionKeys lists the regions the straight segment crosses (the
// coarse frontier visits them first; the sample cap bounds the
// pathological half world diagonals).
func lineRegionKeys(start, end Pos) []RegionKey {
    sc, sr := RegionOfWorld(start.X, start.Y)
    ec, er := RegionOfWorld(end.X, end.Y)
    span := abs16(ec-sc) + abs16(er-sr)
    if span == 0 {
        return nil
    }
    if span > maxLinePrefetchRegions {
        span = maxLinePrefetchRegions
    }
    seen := make(map[RegionKey]struct{}, int(span)+2)
    keys := make([]RegionKey, 0, int(span)+2)
    add := func(col, row int16) {
        key := RegionKey{Col: col, Row: row}
        if _, ok := seen[key]; ok {
            return
        }
        seen[key] = struct{}{}
        keys = append(keys, key)
    }
    for i := 0; i <= int(span)+1; i++ {
        t := float64(i) / float64(span+1)
        col, row := RegionOfWorld(
            start.X+(end.X-start.X)*t,
            start.Y+(end.Y-start.Y)*t)
        add(col, row)
    }

    return keys
}

// abs16 is the |a| of an int16 pair delta.
func abs16(a int16) int16 {
    if a < 0 {
        return -a
    }

    return a
}

// maxLinePrefetchRegions caps the line corridor abstract prefetch
// (the abstract LRU stays the RAM bound, the cap keeps the half
// world queries from loading every region graph on the line).
const maxLinePrefetchRegions = 24

// chainRegionKeys names the distinct regions of the coarse chain
// walk (the first bound count feeds the synchronous prefetch, the
// rest ride the rolling look ahead of the refinement loop).
func chainRegionKeys(chain coarseResult, bound int) []RegionKey {
    keys := make([]RegionKey, 0, len(chain.clusters))
    seen := make(map[RegionKey]struct{}, len(chain.clusters))
    for _, cluster := range chain.clusters {
        key := RegionKey{Col: cluster.Col, Row: cluster.Row}
        if _, ok := seen[key]; ok {
            continue
        }
        seen[key] = struct{}{}
        keys = append(keys, key)
        if len(keys) >= bound {
            break
        }
    }

    return keys
}

// refineChain walks the coarse edge chain and fills the route with
// the stitched refinement corridors. The answer reports whether the
// route is finished (found or the final partial); a false answer
// bans the failed hop exit and asks for the replan.
func (m *Mesh) refineChain(q *hierQuery, chain coarseResult,
    route *Route,
) bool {
    // The chain region prefetch: the first regions of the walk
    // decode in parallel before the hops start (the LRU bound is
    // the window the walk keeps live; the further regions ride
    // the rolling look ahead below).
    m.PrefetchTiles(chainRegionKeys(chain, maxPrefetchWorkers))
    corridor := make([]PolyRef, 0, 64+len(chain.edges)*8)
    currentRef := q.startRef
    currentPos := q.startPos
    var prevEdge abstractEdgeRef
    for i := range chain.edges {
        // The rolling look ahead: the tile two clusters ahead
        // decodes in the background while this hop searches
        // (the singleflight merges with the on demand loads).
        if i+2 < len(chain.clusters) {
            m.prefetchAhead(RegionKey{
                Col: chain.clusters[i+2].Col,
                Row: chain.clusters[i+2].Row,
            })
        }
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
// only - the portal goals are exact). A search that hit the node
// budget retries with the quadrupled budget before the caller bans
// the edge: the component gate already verified the polygon path
// exists, the capped search just never saw the goal side (the intra
// tile hops of the dense regions wander past the flat bound), so
// the retry converges where the replan would repeat the same cap.
func (m *Mesh) runHop(q *hierQuery, fromRef PolyRef, fromPos Pos,
    toRef PolyRef, toPos Pos, last bool,
) *hopSegment {
    goal := astarGoal{target: toRef, escape: false, approach: 0}
    if last {
        goal.approach = q.approach
    }
    budget := maxHopperNodes
    explored := 0
    retries := 0
    for {
        result := m.astar(q.state, goal, fromRef, fromPos, toPos,
            q.filter, noAvoid(), budget, nil)
        if result.corridor == nil {
            return nil
        }
        explored += result.explored
        if result.reached || !result.capped || retries >= 2 ||
            budget >= maxConfinedNodes {
            return &hopSegment{
                corridor: result.corridor,
                explored: explored,
                reached:  result.reached,
                partial:  result.partial,
            }
        }
        // The progress guard: a capped search that never left the
        // start side names a crossing the mesh does not serve - the
        // bigger budget would repeat the same flood (the hop fails,
        // the caller bans the edge and replans instead).
        if dist3(fromPos, result.end) < 0.25*dist3(fromPos, toPos) {
            return &hopSegment{
                corridor: result.corridor,
                explored: explored,
                reached:  result.reached,
                partial:  result.partial,
            }
        }
        budget *= 4
        retries++
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
