// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "testing"

// benchPackMesh builds a mesh over the corridor pack and skips when
// the requested tile file is absent (the pack stays out of git).
func benchPackMesh(b *testing.B, key RegionKey) *Mesh {
    b.Helper()
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    if _, err := mesh.Tile(key); err != nil {
        b.Skipf("the tile pack is not present: %v", err)
    }

    return mesh
}

// BenchmarkFlatFound21x19 times the flat search of a found interior
// pair of 21_19 (the warm steady state of the production Route on
// the single dense tile, docs/fastpath_research.md section 7.2).
func BenchmarkFlatFound21x19(b *testing.B) {
    mesh := benchPackMesh(b, RegionKey{Col: 21, Row: 19})
    start := Pos{X: 36000, Y: 36000, Z: -3696}
    end := Pos{X: 50000, Y: 50000, Z: -3696}
    b.ResetTimer()
    for range b.N {
        route, err := mesh.Route(start, end, DefaultFilter())
        if err != nil || !route.Found {
            b.Fatal("the found bench pair must answer found")
        }
    }
}

// BenchmarkFlatExhaustive21x19 times the unreachable interior pair:
// the search walks the whole connected component up to the node
// budget before the closest reachable answer - the shape the
// confined Giran searches repeat while the strand stands.
func BenchmarkFlatExhaustive21x19(b *testing.B) {
    mesh := benchPackMesh(b, RegionKey{Col: 21, Row: 19})
    start := Pos{X: 34000, Y: 34000, Z: -3696}
    end := Pos{X: 64000, Y: 64000, Z: -3696}
    b.ResetTimer()
    for range b.N {
        route, err := mesh.Route(start, end, DefaultFilter())
        if err != nil || route.Found || !route.Partial {
            b.Fatal("the exhaustive bench pair must answer the partial")
        }
    }
}
