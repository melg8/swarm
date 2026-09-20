// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// regionFileReal is the fixed relative path of the real elven
// village region file of the shipped pack.
const regionFileReal = "../../../../data/geodata/21_19.l2j"

// realBenchMesh builds the real region once and loads the mesh.
func realBenchMesh(b *testing.B) (*RegionBuild, *navmesh.Mesh) {
    b.Helper()
    data, err := os.ReadFile(regionFileReal)
    if err != nil {
        b.Skip("the geodata pack is not present")
    }
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    if err != nil {
        b.Fatal(err)
    }
    dir := b.TempDir()
    encoded, err := navmesh.EncodeTile(build.Tile)
    if err != nil {
        b.Fatal(err)
    }
    name := fmt.Sprintf("%d_%d.nm", build.Tile.Col, build.Tile.Row)
    //nolint:gosec // the tile region key owns the file name
    if err := os.WriteFile(filepath.Join(dir, name), encoded, 0o600); err != nil {
        b.Fatal(err)
    }

    return build, navmesh.NewMesh(dir)
}

// BenchmarkRealBuildRegion measures the offline build of the real
// region (the C++ experiment needed 5.1 s for the same region).
func BenchmarkRealBuildRegion(b *testing.B) {
    data, err := os.ReadFile(regionFileReal)
    if err != nil {
        b.Skip("the geodata pack is not present")
    }
    b.ResetTimer()
    for range b.N {
        build, err := BuildRegion(data, 21, 19, DefaultOptions())
        if err != nil {
            b.Fatal(err)
        }
        if len(build.Tile.Polys) < 1000 {
            b.Fatal("the build lost polygons")
        }
    }
}

// BenchmarkRealBridgePair measures the hard pair query (the C++
// Detour answered 169 us, the grid engine 5.2 s).
func BenchmarkRealBridgePair(b *testing.B) {
    _, mesh := realBenchMesh(b)
    village := navmesh.Pos{X: 45768, Y: 49848, Z: -3056}
    under := navmesh.Pos{X: 44920, Y: 50792, Z: -3928}
    b.ResetTimer()
    for range b.N {
        route, err := mesh.Route(village, under, navmesh.DefaultFilter())
        if err != nil || !route.Found {
            b.Fatalf("route failed: %v", err)
        }
    }
}

// BenchmarkRealNearestPoly measures the stacked disambiguation
// resolution.
func BenchmarkRealNearestPoly(b *testing.B) {
    _, mesh := realBenchMesh(b)
    under := navmesh.Pos{X: 44920, Y: 50792, Z: -3928}
    b.ResetTimer()
    for range b.N {
        _, _, ok := mesh.FindNearestPoly(under)
        if !ok {
            b.Fatal("the nearest poly must resolve")
        }
    }
}

// BenchmarkRealTileDecode measures the tile parse cost (the research
// prototype measured 1.45 ms on the Detour binary format).
func BenchmarkRealTileDecode(b *testing.B) {
    build, _ := realBenchMesh(b)
    encoded, err := navmesh.EncodeTile(build.Tile)
    if err != nil {
        b.Fatal(err)
    }
    b.ResetTimer()
    for range b.N {
        tile, err := navmesh.DecodeTile(encoded)
        if err != nil || len(tile.Polys) == 0 {
            b.Fatal("the decode must succeed")
        }
    }
}

// The build timing sanity: the full-region build must stay in the
// seconds range (the pack-wide build budget of the offline cmd).
func TestRealBuildTimeBudget(t *testing.T) {
    data := realRegionData(t, 21, 19)
    started := time.Now()
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)
    elapsed := time.Since(started)
    t.Logf("the real region build: %s (%d polys)", elapsed,
        build.Stats.Polys)
    require.Less(t, elapsed.Seconds(), 30.0,
        "the per-region build must stay well under the deploy budget")
}
