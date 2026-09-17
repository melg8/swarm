// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTownLegPhaseTrace walks one town leg through the hierarchical
// phases with the per phase and per hop timing - the diagnostic round
// for the long leg cost. The leg picks via the SWARM_TOWN_LEG env
// (0,1,2); the test skips when the pack is absent.
func TestTownLegPhaseTrace(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(4)

	idx := 0
	if text := os.Getenv("SWARM_TOWN_LEG"); text != "" {
		n, err := strconv.Atoi(text)
		if err == nil && n >= 0 && n < len(townLegs) {
			idx = n
		}
	}
	leg := townLegs[idx]

	startRef, startPos, ok := mesh.FindNearestPoly(leg.start)
	require.True(t, ok)
	endRef, endPos, ok := mesh.FindNearestPoly(leg.end)
	require.True(t, ok)
	sc, sr := TileOf(startRef)
	ec, er := TileOf(endRef)
	t.Logf("%s: tiles %d_%d -> %d_%d, straight %.0f", leg.name,
		sc, sr, ec, er, dist3(startPos, endPos))

	state := mesh.acquireState()
	defer mesh.releaseState(state)
	coarse := mesh.acquireCoarse()
	defer mesh.releaseCoarse(coarse)
	query := hierQuery{
		startRef: startRef, startPos: startPos,
		endRef: endRef, endPos: endPos,
		approach: 0, filter: DefaultFilter(),
		state: state, coarse: coarse,
		bans: make(map[abstractEdgeRef]bool),
	}

	began := time.Now()
	chain := mesh.coarseChain(&query, endRef, endPos, true)
	chainTime := time.Since(began)
	t.Logf("coarse chain: edges=%d clusters=%d explored=%d reached=%t"+
		" %s", len(chain.edges), len(chain.clusters), chain.explored,
		chain.reached, chainTime)
	require.True(t, chain.reached, "the coarse chain must reach")

	began = time.Now()
	currentRef := startRef
	currentPos := startPos
	totalExplored := 0
	maxHop := 0
	for i := range chain.edges {
		exitRef := chain.edges[i]
		exitEdge, ok := mesh.hierEdge(exitRef)
		if !ok {
			t.Fatalf("hop %d: exit edge missing %v", i, exitRef)
		}
		goalRef := exitEdge.toRef
		if goalRef == 0 {
			goalRef = mesh.resolveEdgeTarget(exitRef, exitEdge)
		}
		if goalRef == 0 {
			t.Logf("hop %d: no target poly", i)

			continue
		}
		began = time.Now()
		segment := mesh.runHop(&query, currentRef, currentPos, goalRef,
			exitEdge.mid, false)
		hopTime := time.Since(began)
		totalExplored += segment.explored
		if segment.explored > maxHop {
			maxHop = segment.explored
		}
		if !segment.reached || hopTime > 2*time.Second || i < 3 ||
			i > len(chain.edges)-4 {
			fc, fr := TileOf(currentRef)
			tc, tr := TileOf(goalRef)
			t.Logf("hop %d: %d_%d -> %d_%d explored=%d reached=%t"+
				" partial=%t %s", i, fc, fr, tc, tr,
				segment.explored, segment.reached,
				segment.partial, hopTime)
		}
		currentRef = goalRef
		currentPos = exitEdge.mid
	}
	t.Logf("hops: %d edges, %d explored total, max hop %d, %s",
		len(chain.edges), totalExplored, maxHop, time.Since(began))
}

// TestTownLegFlatProbe answers the flat search reachability for one
// leg (the diagnostic round for the coarse flood).
func TestTownLegFlatProbe(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(4)
	idx := 0
	if text := os.Getenv("SWARM_TOWN_LEG"); text != "" {
		if n, err := strconv.Atoi(text); err == nil &&
			n >= 0 && n < len(townLegs) {
			idx = n
		}
	}
	leg := townLegs[idx]
	startRef, startPos, ok := mesh.FindNearestPoly(leg.start)
	require.True(t, ok)
	endRef, endPos, ok := mesh.FindNearestPoly(leg.end)
	require.True(t, ok)
	state := mesh.acquireState()
	defer mesh.releaseState(state)
	began := time.Now()
	result := mesh.astar(state,
		astarGoal{target: endRef, escape: false, approach: 0},
		startRef, startPos, endPos, DefaultFilter(), noAvoid(), 1<<21,
		nil)
	t.Logf("flat: reached=%t partial=%t capped=%t explored=%d %s",
		result.reached, result.partial, result.capped, result.explored,
		time.Since(began))
	if !result.reached && len(result.corridor) > 0 {
		tile, poly := mesh.polyOfRef(
			result.corridor[len(result.corridor)-1])
		if tile != nil {
			x0, y0, x1, y1 := tile.WorldRect(poly)
			t.Logf("partial ends at %d_%d [%.0f %.0f .. %.0f %.0f]",
				tile.Col, tile.Row, x0, y0, x1, y1)
		}
	}
}

// TestTownGoalComponentProbe answers the goal cluster component
// arithmetic: the end poly's component, the crossings into the goal
// cluster and their entry components (the diagnostic round for the
// coarse flood of the town legs).
func TestTownGoalComponentProbe(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(8)
	leg := townLegs[1]
	endRef, endPos, ok := mesh.FindNearestPoly(leg.end)
	require.True(t, ok)
	gc, gr := TileOf(endRef)
	goalAbstract := mesh.abstractOf(RegionKey{Col: gc, Row: gr})
	require.NotNil(t, goalAbstract)
	endIdx := PolyOf(endRef)
	goalComp := goalAbstract.comps[endIdx]
	count := 0
	for _, c := range goalAbstract.comps {
		if c == goalComp {
			count++
		}
	}
	t.Logf("goal tile %d_%d, end poly %d comp %d holds %d of %d polys",
		gc, gr, endIdx, goalComp, count, len(goalAbstract.comps))

	// The start poly component too (the Gludio side).
	startRef, _, ok := mesh.FindNearestPoly(leg.start)
	require.True(t, ok)
	sc, sr := TileOf(startRef)
	startAbstract := mesh.abstractOf(RegionKey{Col: sc, Row: sr})
	sComp := startAbstract.comps[PolyOf(startRef)]
	sCount := 0
	for _, c := range startAbstract.comps {
		if c == sComp {
			sCount++
		}
	}
	t.Logf("start tile %d_%d, start poly comp %d holds %d polys",
		sc, sr, sComp, sCount)

	// Every crossing INTO the goal cluster: the edges of the goal
	// tile and its neighbours pointing there, with the entry comps.
	goalKey := clusterOfPos(gc, gr, endPos.X, endPos.Y)
	t.Logf("goal cluster: %d_%d/%d", goalKey.Col, goalKey.Row,
		goalKey.ID)
	neighbours := [][2]int16{{gc, gr}, {gc - 1, gr}, {gc + 1, gr},
		{gc, gr - 1}, {gc, gr + 1}, {gc - 1, gr - 1},
		{gc + 1, gr + 1}, {gc - 1, gr + 1}, {gc + 1, gr - 1}}
	entries := map[uint32]int{}
	for _, nb := range neighbours {
		abstract := mesh.abstractOf(RegionKey{Col: nb[0], Row: nb[1]})
		if abstract == nil {
			continue
		}
		for ei := range abstract.edges {
			edge := &abstract.edges[ei]
			if edge.to.Col != goalKey.Col ||
				edge.to.Row != goalKey.Row ||
				edge.to.ID != goalKey.ID {
				continue
			}
			entries[edgeEntryPoly(edge)]++
		}
	}
	compCounts := map[uint32]int{}
	for polyIdx := range entries {
		if int(polyIdx) < len(goalAbstract.comps) {
			compCounts[goalAbstract.comps[polyIdx]]++
		}
	}
	t.Logf("crossings into the goal cluster: %d, entry comps: %v",
		len(entries), compCounts)
	t.Logf("the goal comp is served: %t",
		compCounts[goalComp] > 0)
}

// TestTileBorderComps lists the cross tile crossings into one tile
// with the entry component of each (the SWARM_PROBE_TILE env, the
// diagnostic round for the comp gate reachability).
func TestTileBorderComps(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(16)
	col, row := int16(17), int16(22)
	abstract := mesh.abstractOf(RegionKey{Col: col, Row: row})
	require.NotNil(t, abstract)
	main := map[uint32]int{}
	for _, c := range abstract.comps {
		main[c]++
	}
	t.Logf("tile 17_22 comps: %d distinct, top:", len(main))
	type pair struct {
		c uint32
		n int
	}
	var top []pair
	for c, n := range main {
		top = append(top, pair{c, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	for i := 0; i < len(top) && i < 5; i++ {
		t.Logf("  comp %d: %d polys", top[i].c, top[i].n)
	}

	neighbours := [][2]int16{{col - 1, row}, {col + 1, row},
		{col, row - 1}, {col, row + 1}, {col - 1, row - 1},
		{col + 1, row + 1}, {col - 1, row + 1}, {col + 1, row - 1}}
	for _, nb := range neighbours {
		na := mesh.abstractOf(RegionKey{Col: nb[0], Row: nb[1]})
		if na == nil {
			continue
		}
		entries := map[uint32]int{}
		for ei := range na.edges {
			edge := &na.edges[ei]
			if edge.extCol != col || edge.extRow != row {
				continue
			}
			if int(edge.extPoly) < len(abstract.comps) {
				entries[abstract.comps[edge.extPoly]]++
			}
		}
		if len(entries) > 0 {
			t.Logf("crossings %d_%d -> 17_22 entry comps: %v",
				nb[0], nb[1], entries)
		}
	}
}

// TestBayCrossingProbe lists the abstract edge census of the bay
// tiles between Gludio and Gludin (the swim route the town leg
// expects): every edge leaving each tile grouped by the target tile
// and the target area (the water wall question of the stitch).
func TestBayCrossingProbe(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(24)
	tiles := [][2]int16{{19, 21}, {18, 21}, {18, 22}, {17, 21},
		{17, 22}}
	for _, tl := range tiles {
		abstract := mesh.abstractOf(RegionKey{Col: tl[0], Row: tl[1]})
		if abstract == nil {
			t.Logf("%d_%d: no abstract", tl[0], tl[1])

			continue
		}
		byTarget := map[string]int{}
		waterEdges := 0
		for ei := range abstract.edges {
			edge := &abstract.edges[ei]
			key := fmt.Sprintf("%d_%d", edge.to.Col, edge.to.Row)
			byTarget[key]++
			if edge.srcArea == AreaWater {
				waterEdges++
			}
		}
		t.Logf("%d_%d: %d edges (%d water sourced) -> %v",
			tl[0], tl[1], len(abstract.edges), waterEdges, byTarget)
	}
}

// TestCoarseReverseReach walks the abstract graph backward from the
// goal with the exact component gate semantics of the coarse search:
// the (cluster, entry comp) states the search needs and whether the
// start reaches them.
func TestCoarseReverseReach(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(64)
	leg := townLegs[1]
	startRef, startPos, ok := mesh.FindNearestPoly(leg.start)
	require.True(t, ok)
	endRef, endPos, ok := mesh.FindNearestPoly(leg.end)
	require.True(t, ok)
	sc, sr := TileOf(startRef)
	ec, er := TileOf(endRef)
	startAbstract := mesh.abstractOf(RegionKey{Col: sc, Row: sr})
	goalAbstract := mesh.abstractOf(RegionKey{Col: ec, Row: er})
	startKey := clusterOfPos(sc, sr, startPos.X, startPos.Y)
	startComp := startAbstract.comps[PolyOf(startRef)]
	goalKey := clusterOfPos(ec, er, endPos.X, endPos.Y)
	goalComp := goalAbstract.comps[PolyOf(endRef)]
	t.Logf("start %d_%d/%d comp %d -> goal %d_%d/%d comp %d",
		startKey.Col, startKey.Row, startKey.ID, startComp,
		goalKey.Col, goalKey.Row, goalKey.ID, goalComp)

	// The exact crossings 18_22 -> 17_22 with both sides' comps.
	na := mesh.abstractOf(RegionKey{Col: ec - 1, Row: er})
	if na != nil {
		inverse := make(map[int32]clusterID, len(na.index))
		for cid, idx := range na.index {
			inverse[idx] = cid
		}
		for nodeID := range na.nodes {
			node := &na.nodes[nodeID]
			for _, ei := range node.edges {
				edge := &na.edges[ei]
				if edge.extCol != ec || edge.extRow != er {
					continue
				}
				t.Logf("crossing 18_22/%d (srcComp %d) -> 17_22 poly"+
					" %d (comp %d)", inverse[int32(nodeID)],
					edge.srcComp, edge.extPoly,
					goalAbstract.comps[edge.extPoly])
			}
		}
	}

	type state struct {
		key  clusterKey
		comp uint32
	}
	goalState := state{goalKey, goalComp}
	startState := state{startKey, startComp}
	frontier := []state{goalState}
	seen := map[state]bool{goalState: true}
	abstractOfKey := func(key clusterKey) *regionAbstract {
		return mesh.abstractOf(RegionKey{Col: key.Col, Row: key.Row})
	}
	// The 3x3 abstracts around a cluster name every edge into it.
	steps := 0
	for len(frontier) > 0 && steps < 300000 {
		steps++
		current := frontier[0]
		frontier = frontier[1:]
		if current == startState {
			t.Logf("REACHABLE: the start state served in %d steps,"+
				" %d states", steps, len(seen))

			return
		}
		a := abstractOfKey(current.key)
		if a == nil {
			continue
		}
		for dc := int16(-1); dc <= 1; dc++ {
			for dr := int16(-1); dr <= 1; dr++ {
				col := current.key.Col + dc
				row := current.key.Row + dr
				na := abstractOfKey(clusterKey{Col: col, Row: row})
				if na == nil {
					continue
				}
				// Map the node index -> cluster id once per
				// abstract (the node slice order is the build
				// order, the cluster id lives in the index).
				inverse := make(map[int32]clusterID,
					len(na.index))
				for cid, idx := range na.index {
					inverse[idx] = cid
				}
				for nodeID := range na.nodes {
					node := &na.nodes[nodeID]
					cid, ok := inverse[int32(nodeID)]
					if !ok {
						continue
					}
					for _, ei := range node.edges {
						edge := &na.edges[ei]
						if edge.to.Col != current.key.Col ||
							edge.to.Row != current.key.Row ||
							edge.to.ID != current.key.ID {
							continue
						}
						entryPoly := edgeEntryPoly(edge)
						if int(entryPoly) >= len(a.comps) ||
							a.comps[entryPoly] != current.comp {
							continue
						}
						nextState := state{
							key: clusterKey{Col: col, Row: row,
								ID: cid},
							comp: edge.srcComp,
						}
						if !seen[nextState] {
							seen[nextState] = true
							frontier = append(frontier, nextState)
						}
					}
				}
			}
		}
	}
	t.Logf("NOT reachable: %d states explored, the frontier died."+
		" States by tile:", len(seen))
	byTile := map[string]int{}
	for s := range seen {
		byTile[fmt.Sprintf("%d_%d", s.key.Col, s.key.Row)]++
	}
	for tile, n := range byTile {
		t.Logf("  %s: %d states", tile, n)
	}
	// The 17_22 states: the clusters the goal comp chain touches.
	comps137 := map[string][]string{}
	for s := range seen {
		if s.key.Col == ec && s.key.Row == er {
			cx := int(s.key.ID) / 16
			cy := int(s.key.ID) % 16
			comps137[fmt.Sprintf("CX%d", cx)] = append(
				comps137[fmt.Sprintf("CX%d", cx)],
				fmt.Sprintf("cy%d:c%d", cy, s.comp))
		}
	}
	for cx, list := range comps137 {
		t.Logf("  %s: %v", cx, list)
	}
}

// TestWaterBorderStitch probes the raw external links across the
// water borders south of the Gludin Gludio line (the swim route the
// town legs need): whether the water polygons stitch across the tile
// borders at all.
func TestWaterBorderStitch(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(16)
	pairs := [][2][2]int16{
		{{18, 21}, {17, 21}},
		{{18, 21}, {18, 22}},
		{{19, 21}, {18, 21}},
		{{18, 22}, {17, 22}},
		{{17, 23}, {17, 24}},
		{{17, 24}, {17, 25}},
		{{17, 24}, {18, 24}},
		{{16, 11}, {16, 12}},
	}
	for _, pr := range pairs {
		a := pr[0]
		b := pr[1]
		tileA, err := mesh.Tile(RegionKey{Col: a[0], Row: a[1]})
		if err != nil || tileA == nil {
			t.Logf("%d_%d -> %d_%d: tile fails %v", a[0], a[1],
				b[0], b[1], err)

			continue
		}
		total, waterSrc, toB, waterToB, dead := 0, 0, 0, 0, 0
		for i := range tileA.ExtLinks {
			ext := &tileA.ExtLinks[i]
			if ext.Col != int32(b[0]) || ext.Row != int32(b[1]) {
				continue
			}
			total++
			if ext.Poly == 0xFFFFFFFF {
				dead++

				continue
			}
			toB++
			// The source poly area: walk the polys to find the
			// source (the ext link does not carry it - count via
			// the links of the source polys instead).
			_ = waterSrc
		}
		// The water sourced count: scan the links of the water polys.
		for pi := range tileA.Polys {
			poly := &tileA.Polys[pi]
			if poly.Area != AreaWater {
				continue
			}
			for li := poly.FirstLink; li >= 0 &&
				int(li) < len(tileA.Links); {
				link := &tileA.Links[li]
				li = link.Next
				if link.To >= 0 {
					continue
				}
				extIdx := -link.To - 1
				if int(extIdx) >= len(tileA.ExtLinks) {
					continue
				}
				ext := &tileA.ExtLinks[extIdx]
				if ext.Col == int32(b[0]) && ext.Row == int32(b[1]) {
					waterToB++
				}
			}
		}
		t.Logf("%d_%d -> %d_%d: ext records %d (dead %d), water"+
			" sourced links %d", a[0], a[1], b[0], b[1], total,
			dead, waterToB)
	}
}

// TestBayNorthShoreProbe probes candidate waypoints along the north
// shore of the Gludio bay for the mesh binding (the intermediate
// waypoint the segmented Gludio Gludin walk needs while the pack's
// geodata leaves the bay water unmapped).
func TestBayNorthShoreProbe(t *testing.T) {
	dir := "../../../../data/navmesh"
	mesh := NewMesh(dir)
	if len(mesh.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh.SetCacheCapacity(8)
	candidates := [][2]float64{
		{-70000, 160000}, {-75000, 158000}, {-65000, 162000},
		{-60000, 155000}, {-55000, 150000}, {-70000, 155000},
		{-80000, 160000}, {-90000, 150000}, {-95000, 145000},
		{-45000, 148000},
	}
	for _, c := range candidates {
		_, pos, ok := mesh.FindNearestPoly(Pos{X: c[0], Y: c[1],
			Z: -4800})
		if ok {
			t.Logf("(%.0f, %.0f) binds at %.0f %.0f %.0f", c[0], c[1],
				pos.X, pos.Y, pos.Z)
		} else {
			t.Logf("(%.0f, %.0f) does not bind", c[0], c[1])
		}
	}
	// The bay itself: does the water hold mesh at all?
	bay := [][2]float64{{-80000, 135000}, {-75000, 138000},
		{-85000, 140000}, {-70000, 140000}}
	for _, c := range bay {
		_, pos, ok := mesh.FindNearestPoly(Pos{X: c[0], Y: c[1],
			Z: -4800})
		if ok {
			t.Logf("bay (%.0f, %.0f) binds at z %.0f", c[0], c[1],
				pos.Z)
		} else {
			t.Logf("bay (%.0f, %.0f) does NOT bind - the water hole"+
				" is real", c[0], c[1])
		}
	}
}
