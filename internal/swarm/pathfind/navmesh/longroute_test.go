// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tilePackMesh builds a mesh over the repository tile pack (the
// cmd/navmesh-build output under data/navmesh). The test skips when
// the pack has not been built - the real tile rounds run where the
// pack exists.
func tilePackMesh(t *testing.T, keys ...RegionKey) *Mesh {
	t.Helper()
	dir := "../../../../data/navmesh"
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("the navmesh tile pack is not present: %v", err)
	}
	for _, key := range keys {
		file := fmt.Sprintf("%s/%d_%d.nm", dir, key.Col, key.Row)
		if _, err := os.Stat(file); err != nil {
			t.Skipf("the tile pack lacks %d_%d: %v", key.Col, key.Row,
				err)
		}
	}

	return NewMesh(dir)
}

// TestLongRouteDiagonalSwim replays the owner long route the viewer
// answered with a partial corridor: the diagonal across the loaded
// pack from the north east corner (21_20) to the south west one
// (20_19), the swim filter. The route exercises the whole machine:
// the far endpoints, the water pricing over the open sea and the
// search budget of a corridor that threads two dense land regions.
func TestLongRouteDiagonalSwim(t *testing.T) {
	mesh := tilePackMesh(t,
		RegionKey{Col: 20, Row: 19}, RegionKey{Col: 20, Row: 20},
		RegionKey{Col: 21, Row: 19}, RegionKey{Col: 21, Row: 20})

	start := Pos{X: 59003, Y: 91907, Z: -3696}
	end := Pos{X: 11738, Y: 42586, Z: -3664}
	began := time.Now()
	route, err := mesh.Route(start, end, DefaultFilter())
	require.NoError(t, err)
	elapsed := time.Since(began)

	t.Logf("diagonal swim route: found=%t partial=%t hierarchical=%t"+
		" explored=%d corridor=%d waypoints=%d %s",
		route.Found, route.Partial, route.Hierarchical, route.Explored,
		len(route.Corridor), len(route.Waypoints), elapsed)
	if len(route.Waypoints) > 0 {
		first := route.Waypoints[0]
		last := route.Waypoints[len(route.Waypoints)-1]
		t.Logf("  first waypoint %.0f %.0f %.0f, last %.0f %.0f %.0f",
			first.X, first.Y, first.Z, last.X, last.Y, last.Z)
	}

	require.True(t, route.Found,
		"the diagonal swim route across the pack must be found")
	require.False(t, route.Partial,
		"the diagonal swim route must not answer the partial corridor")
}
