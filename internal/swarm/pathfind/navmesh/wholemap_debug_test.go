// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWholeMapChainDebug traces where the whole map chain breaks: the
// coarse chain, the hop attempts and the partial corridor end tile.
// The test is the diagnostic round of the whole map route work - it
// skips when the pack is absent.
func TestWholeMapChainDebug(t *testing.T) {
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

	sc, sr := TileOf(startRef)
	ec, er := TileOf(endRef)
	t.Logf("start poly tile %d_%d, end poly tile %d_%d", sc, sr, ec, er)

	// The coarse chain for the pair.
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
	chain := mesh.coarseChain(&query, endRef, endPos, true)
	t.Logf("coarse chain: edges=%d clusters=%d explored=%d reached=%t",
		len(chain.edges), len(chain.clusters), chain.explored,
		chain.reached)
	if len(chain.clusters) > 0 {
		first := chain.clusters[0]
		last := chain.clusters[len(chain.clusters)-1]
		t.Logf("chain walk %d_%d/%d .. %d_%d/%d",
			first.Col, first.Row, first.ID, last.Col, last.Row, last.ID)
	}

	route := &Route{Hierarchical: true}
	ok = mesh.refineChain(&query, chain, route)
	t.Logf("refine chain: ok=%t found=%t corridor=%d", ok, route.Found,
		len(route.Corridor))
	if !ok {
		for ref := range query.bans {
			t.Logf("banned edge: region %d_%d index %d", ref.Region.Col,
				ref.Region.Row, ref.Index)
		}
	}
}
