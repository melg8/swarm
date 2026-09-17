// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWholeMapHopTrace walks the coarse chain hop by hop and reports
// the first failing hop with its region pair and positions - the
// diagnostic round for the whole map partial answer.
func TestWholeMapHopTrace(t *testing.T) {
	if os.Getenv("SWARM_WHOLEMAP") != "1" {
		t.Skip("the whole map walk is gated (SWARM_WHOLEMAP=1)")
	}
	dir := "../../../../data/navmesh"
	probe := NewMesh(dir)
	if len(probe.TileFiles()) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh := NewMesh(dir)
	mesh.SetCacheCapacity(1)

	start := Pos{X: 59003, Y: 91907, Z: -3696}
	target := Pos{X: 180192, Y: -213024, Z: -3744}
	startRef, startPos, ok := mesh.FindNearestPoly(start)
	require.True(t, ok)
	endRef, endPos, ok := mesh.FindNearestPoly(target)
	require.True(t, ok)

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
	chain := mesh.coarseChain(&query, endRef, endPos)
	t.Logf("chain: edges=%d clusters=%d explored=%d reached=%t",
		len(chain.edges), len(chain.clusters), chain.explored,
		chain.reached)
	if !chain.reached {
		for i, ref := range chain.edges {
			edge, ok := mesh.hierEdge(ref)
			if !ok {
				t.Logf("  edge %d: %d_%d #%d (missing)", i, ref.Region.Col,
					ref.Region.Row, ref.Index)

				continue
			}
			dst := edge.dstComp
			if dst == 0 {
				dst = mesh.edgeDstComp(ref, edge)
			}
			t.Logf("  edge %d: %d_%d #%d -> %d_%d/%d srcComp=%d"+
				" dstComp=%d", i, ref.Region.Col, ref.Region.Row,
				ref.Index, edge.to.Col, edge.to.Row, edge.to.ID,
				edge.srcComp, dst)
		}
		last := chain.clusters[len(chain.clusters)-1]
		t.Logf("partial chain ends at %d_%d/%d", last.Col, last.Row,
			last.ID)
		t.SkipNow()
	}
	require.True(t, chain.reached)

	currentRef := startRef
	currentPos := startPos
	var prevEdge abstractEdgeRef
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
			t.Logf("hop %d (%d_%d #%d): no target poly (region %d_%d)",
				i, exitRef.Region.Col, exitRef.Region.Row, exitRef.Index,
				exitEdge.to.Col, exitEdge.to.Row)
			break
		}
		segment := mesh.runHop(&query, currentRef, currentPos, goalRef,
			exitEdge.mid, false)
		if segment == nil || !segment.reached && i <= 4 {
			// The diagnostic round: the same tile component check
			// between the hop start poly and the target poly (the
			// link graph flood, no budget, no heap).
			fc, fr := TileOf(currentRef)
			tc, tr := TileOf(goalRef)
			if fc == tc && fr == tr {
				a, b := mesh.sameTileComponent(currentRef, goalRef)
				t.Logf("hop %d component check: connected=%t"+
					" (floodA=%d, floodB=%d)", i, a, b[0], b[1])
				mesh.logComponentShape(t, goalRef)
			}
		}
		if segment == nil || !segment.reached {
			explored := -1
			partial := false
			if segment != nil {
				explored = segment.explored
				partial = segment.partial
			}
			fc, fr := TileOf(currentRef)
			tc, tr := TileOf(goalRef)
			t.Logf("hop %d FAILED: edge %d_%d #%d, from poly tile"+
				" %d_%d to tile %d_%d, mid %.0f %.0f (cluster"+
				" %d_%d/%d), explored=%d partial=%t",
				i, exitRef.Region.Col, exitRef.Region.Row, exitRef.Index,
				fc, fr, tc, tr,
				exitEdge.mid.X, exitEdge.mid.Y,
				exitEdge.to.Col, exitEdge.to.Row, exitEdge.to.ID,
				explored, partial)
			break
		}
		tc, tr := TileOf(goalRef)
		if i <= 4 || i == len(chain.edges)-1 {
			t.Logf("hop %d ok (tile %d_%d, explored=%d)", i, tc, tr,
				segment.explored)
		}
		currentRef = goalRef
		currentPos = exitEdge.mid
		prevEdge = exitRef
		_ = prevEdge
	}
}

// runHopBudget is the runHop variant with an explicit budget: the
// diagnostic rounds need the region scale searches the hop budget
// caps.
func (m *Mesh) runHopBudget(q *hierQuery, fromRef PolyRef, fromPos Pos,
	toRef PolyRef, toPos Pos, last bool, budget int,
) *hopSegment {
	goal := astarGoal{target: toRef, escape: false, approach: 0}
	if last {
		goal.approach = q.approach
	}
	result := m.astar(q.state, goal, fromRef, fromPos, toPos, q.filter,
		noAvoid(), budget, nil)
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

// sameTileComponent floods the link graph of one tile from two
// polygons and reports whether the floods meet (the polygon level
// connectivity answer the budget capped searches cannot give).
func (m *Mesh) sameTileComponent(a, b PolyRef) (bool, [2]int) {
	ac, ar := TileOf(a)
	tile, err := m.Tile(RegionKey{Col: ac, Row: ar})
	if err != nil || tile == nil {
		return false, [2]int{}
	}
	visitedA := make([]bool, len(tile.Polys))
	visitedB := make([]bool, len(tile.Polys))
	flood := func(start uint32, visited []bool) int {
		stack := []uint32{start}
		visited[start] = true
		count := 0
		for len(stack) > 0 {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			count++
			poly := &tile.Polys[top]
			for li := poly.FirstLink; li >= 0 &&
				int(li) < len(tile.Links); {
				link := &tile.Links[li]
				li = link.Next
				if link.To < 0 || int(link.To) >= len(tile.Polys) {
					continue
				}
				if !visited[link.To] {
					visited[link.To] = true
					stack = append(stack, uint32(link.To))
				}
			}
		}

		return count
	}
	ai := PolyOf(a)
	bi := PolyOf(b)
	if ai < 0 || ai >= int32(len(tile.Polys)) ||
		bi < 0 || bi >= int32(len(tile.Polys)) {
		return false, [2]int{}
	}
	countA := flood(uint32(ai), visitedA)
	countB := flood(uint32(bi), visitedB)

	return visitedB[ai], [2]int{countA, countB}
}

// logComponentShape prints the area histogram and the world bounding
// box of the link component holding a polygon - the shape answer that
// names the disconnect (an island, a pocket sea, a void cut).
func (m *Mesh) logComponentShape(t *testing.T, ref PolyRef) {
	c, r := TileOf(ref)
	tile, err := m.Tile(RegionKey{Col: c, Row: r})
	if err != nil || tile == nil {
		return
	}
	start := PolyOf(ref)
	visited := make([]bool, len(tile.Polys))
	stack := []uint32{uint32(start)}
	visited[start] = true
	var water, ground, other int
	x0, y0 := math.MaxFloat64, math.MaxFloat64
	x1, y1 := -math.MaxFloat64, -math.MaxFloat64
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		poly := &tile.Polys[top]
		switch poly.Area {
		case AreaWater:
			water++
		case AreaGround:
			ground++
		default:
			other++
		}
		px0, py0, px1, py1 := tile.WorldRect(poly)
		if px0 < x0 {
			x0 = px0
		}
		if py0 < y0 {
			y0 = py0
		}
		if px1 > x1 {
			x1 = px1
		}
		if py1 > y1 {
			y1 = py1
		}
		for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
			link := &tile.Links[li]
			li = link.Next
			if link.To < 0 || int(link.To) >= len(tile.Polys) {
				continue
			}
			if !visited[link.To] {
				visited[link.To] = true
				stack = append(stack, uint32(link.To))
			}
		}
	}
	t.Logf("  target component: %d water, %d ground, %d other,"+
		" bbox [%.0f %.0f .. %.0f %.0f]", water, ground, other, x0, y0,
		x1, y1)
}
