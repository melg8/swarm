// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"fmt"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWholeMapHierarchyStats walks the whole repository tile pack (the
// full map build under data/navmesh) and answers the hierarchy
// arithmetic on the real data: the level 0 polygon count, the level 1
// cluster node and edge counts, and one whole map route from the
// owner diagonal start into the farthest tile that carries mesh. The
// test skips when the pack has not been built. The whole pack walk
// carries the dense tile decodes and the abstract builds over the
// sandbox memory and CPU budget, so the run is gated behind
// SWARM_WHOLEMAP=1 and chunked: SWARM_WHOLEMAP_CHUNK="i/N" processes
// the i-th slice of N slices of the file list (the per chunk log
// lines sum into the whole map arithmetic; SWARM_WHOLEMAP=1 without
// the chunk runs the stats and the route whole).
func TestWholeMapHierarchyStats(t *testing.T) {
	if os.Getenv("SWARM_WHOLEMAP") != "1" {
		t.Skip("the whole map walk is gated (SWARM_WHOLEMAP=1)")
	}
	dir := "../../../../data/navmesh"
	probe := NewMesh(dir)
	all := probe.TileFiles()
	if len(all) < 100 {
		t.Skipf("the whole map pack is not present (%d tiles)", len(all))
	}
	files := all
	if chunk := os.Getenv("SWARM_WHOLEMAP_CHUNK"); chunk != "" {
		i, n := 0, 0
		if _, err := fmt.Sscanf(chunk, "%d/%d", &i, &n); err != nil ||
			n <= 0 || i < 0 || i >= n {
			t.Fatalf("bad chunk %q", chunk)
		}
		from := len(all) * i / n
		to := len(all) * (i + 1) / n
		files = all[from:to]
		t.Logf("chunk %d/%d: tiles %d..%d of %d", i, n, from, to-1,
			len(all))
	}

	mesh := NewMesh(dir)
	// The caches stay minimal (the decoded dense tiles hold hundreds
	// of MB and the abstracts add their comps arrays on top - the
	// whole pack resident blows the sandbox memory): the abstracts
	// evict and rebuild, the stats accumulate, the tiles evict.
	mesh.SetCacheCapacity(1)
	mesh.SetAbstractCapacity(2)

	start := Pos{X: 59003, Y: 91907, Z: -3696}

	// The level 0/1 arithmetic: decode every tile through the
	// abstract (the abstractOf build is one pass over the links) and
	// accumulate the real counts.
	var tiles, polys, nodes, edges int
	var maxPolys int
	maxTile := RegionKey{}
	for i, key := range files {
		abstract := mesh.abstractOf(key)
		if abstract == nil {
			continue
		}
		tiles++
		polys += abstract.polys
		nodes += len(abstract.nodes)
		edges += len(abstract.edges)
		if abstract.polys > maxPolys {
			maxPolys = abstract.polys
			maxTile = key
		}
		// The dense tile decodes ratchet the RSS hundreds of MB at
		// a step - the release keeps the whole pack loop alive.
		if i%4 == 3 {
			debug.FreeOSMemory()
		}
	}
	t.Logf("whole map: %d tiles, level 0 %d polys (max %d at %d_%d),"+
		" level 1 %d cluster nodes, %d deduped portal edges",
		tiles, polys, maxPolys, maxTile.Col, maxTile.Row, nodes, edges)

	if os.Getenv("SWARM_WHOLEMAP_CHUNK") != "" {
		// The chunked runs answer the arithmetic only: the route and
		// the farthest target walk stay with the unchunked run.
		return
	}

	// The farthest tile center that binds to real mesh: the whole map
	// route target.
	_, startPos, ok := mesh.FindNearestPoly(start)
	require.True(t, ok, "the owner start must bind to the mesh")
	bestDist := -1.0
	var target Pos
	targetKey := RegionKey{}
	for i, key := range all {
		abstract := mesh.abstractOf(key)
		if abstract == nil || len(abstract.nodes) == 0 {
			continue
		}
		cx := (float64(key.Col) - tileZeroCol + 0.5) * tileWorldSize
		cy := (float64(key.Row) - tileZeroRow + 0.5) * tileWorldSize
		_, pos, found := mesh.FindNearestPoly(Pos{X: cx, Y: cy,
			Z: startPos.Z})
		if !found {
			continue
		}
		d := dist3(start, pos)
		if d > bestDist {
			bestDist = d
			target = pos
			targetKey = key
		}
		if i%8 == 7 {
			debug.FreeOSMemory()
		}
	}
	require.True(t, bestDist > 0, "the pack must hold a routable target")
	t.Logf("whole map target: %d_%d at %.0f %.0f %.0f (%.0f units out)",
		targetKey.Col, targetKey.Row, target.X, target.Y, target.Z,
		bestDist)

	began := time.Now()
	route, err := mesh.Route(start, target, DefaultFilter())
	require.NoError(t, err)
	elapsed := time.Since(began)

	t.Logf("whole map route: found=%t partial=%t hierarchical=%t"+
		" explored=%d corridor=%d waypoints=%d %s",
		route.Found, route.Partial, route.Hierarchical, route.Explored,
		len(route.Corridor), len(route.Waypoints), elapsed)

	require.True(t, route.Found,
		"the whole map route must be found, not partial")
	require.False(t, route.Partial)
}
