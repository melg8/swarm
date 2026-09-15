// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package prototype

import (
    "os"
    "testing"
)

// BenchmarkFindPathBridge measures the hard pair of the research
// round through the Go query engine: the C++ Detour answers it in
// 169 us, the grid engine in 5.2 s with 500k node expansions.
func BenchmarkFindPathBridge(b *testing.B) {
    data, err := os.ReadFile(researchTile)
    if err != nil {
        b.Skip("the research navmesh tile is not present")
    }
    tile, err := ParseTile(data)
    if err != nil {
        b.Fatalf("parse tile: %v", err)
    }
    query := NewQuery(tile)
    for area := 31; area < 47; area++ {
        query.SetAreaCost(area, 3)
    }
    village := Point{45768, -3056, 49848}
    under := Point{44920, -3928, 50792}
    startPoly, startPos, ok := query.FindNearestPoly(village)
    if !ok {
        b.Fatal("the village point has no polygon")
    }
    endPoly, endPos, ok := query.FindNearestPoly(under)
    if !ok {
        b.Fatal("the under bridge point has no polygon")
    }
    b.ResetTimer()
    for range b.N {
        path := query.FindPath(startPoly, endPoly, startPos, endPos)
        if path == nil {
            b.Fatal("the bridge route vanished")
        }
    }
}

// BenchmarkParseTile measures the tile load cost: the runtime budget
// a bot startup or a tile cache miss pays per region.
func BenchmarkParseTile(b *testing.B) {
    data, err := os.ReadFile(researchTile)
    if err != nil {
        b.Skip("the research navmesh tile is not present")
    }
    b.ResetTimer()
    for range b.N {
        tile, err := ParseTile(data)
        if err != nil {
            b.Fatalf("parse tile: %v", err)
        }
        if tile.Header.PolyCount != 14062 {
            b.Fatalf("unexpected poly count %d", tile.Header.PolyCount)
        }
    }
}
