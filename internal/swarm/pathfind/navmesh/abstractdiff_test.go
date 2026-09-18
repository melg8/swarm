// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "testing"
)

// TestAbstractDiff diffs the sidecar served abstract against the tile
// scan build for the bay regions - the regression probe of the
// persistent coarse layer.
func TestAbstractDiff(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    if len(mesh.TileFiles()) < 20 {
        t.Skip("the town corridor pack is not present")
    }
    for _, key := range []RegionKey{{19, 21}, {17, 22}, {18, 22}} {
        served := mesh.abstractOf(key)
        if served == nil {
            t.Fatalf("%d_%d: no served abstract", key.Col, key.Row)
        }
        tile, err := mesh.Tile(key)
        if err != nil {
            t.Fatal(err)
        }
        built := BuildAbstract(tile)
        if len(served.edges) != len(built.edges) {
            t.Logf("%d_%d: edge count served %d vs built %d", key.Col,
                key.Row, len(served.edges), len(built.edges))

            continue
        }
        diffs := 0
        for i := range served.edges {
            a, b := &served.edges[i], &built.edges[i]
            if *a != *b {
                diffs++
                if diffs <= 3 {
                    t.Logf("%d_%d edge %d differs: served %+v vs built %+v",
                        key.Col, key.Row, i, *a, *b)
                }
            }
        }
        compDiffs := 0
        for i := range served.comps {
            if served.comps[i] != built.comps[i] {
                compDiffs++
            }
        }
        t.Logf("%d_%d: edges %d (%d differing), comps %d (%d differing),"+
            " nodes %d vs %d", key.Col, key.Row, len(served.edges), diffs,
            len(served.comps), compDiffs, len(served.nodes),
            len(built.nodes))
    }
}
