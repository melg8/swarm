// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestWholeMapHierarchyStats walks the whole repository tile pack (the
// full map build under data/navmesh) and answers the hierarchy
// arithmetic on the real data: the level 0 polygon count, the level 1
// cluster node and edge counts, and one whole map route from the
// owner diagonal start into the farthest tile that carries mesh. The
// test skips when the pack has not been built.
func TestWholeMapHierarchyStats(t *testing.T) {
    dir := "../../../../data/navmesh"
    probe := NewMesh(dir)
    files := probe.TileFiles()
    if len(files) < 100 {
        t.Skipf("the whole map pack is not present (%d tiles)", len(files))
    }

    mesh := NewMesh(dir)
    // The LRU stays small (the decoded dense tiles are tens of MB
    // each): the abstracts retain the stats, the tiles evict.
    mesh.SetCacheCapacity(2)

    start := Pos{X: 59003, Y: 91907, Z: -3696}

    // The level 0/1 arithmetic: decode every tile through the
    // abstract (the abstractOf build is one pass over the links) and
    // accumulate the real counts.
    var tiles, polys, nodes, edges int
    var maxPolys int
    maxTile := RegionKey{}
    for _, key := range files {
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
    }
    t.Logf("whole map: %d tiles, level 0 %d polys (max %d at %d_%d),"+
        " level 1 %d cluster nodes, %d deduped portal edges",
        tiles, polys, maxPolys, maxTile.Col, maxTile.Row, nodes, edges)

    // The farthest tile center that binds to real mesh: the whole map
    // route target.
    _, startPos, ok := mesh.FindNearestPoly(start)
    require.True(t, ok, "the owner start must bind to the mesh")
    bestDist := -1.0
    var target Pos
    targetKey := RegionKey{}
    for _, key := range files {
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
