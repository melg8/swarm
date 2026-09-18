// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "os"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The geometry encode probe: the first open of a dense world tile
// pays the NMV2 polygon soup build (the process cache answers the
// repeats). Skips without the NAVMESH_GEO_PROFILE=1 environment; the
// NAVMESH_GEO_PROFILE_MESH names the built pack (the default is the
// repository data/navmesh layout).
func TestNavmeshGeometryProfile(t *testing.T) {
    if os.Getenv("NAVMESH_GEO_PROFILE") == "" {
        t.Skip("set NAVMESH_GEO_PROFILE=1 to run the geometry probe")
    }
    mesh := navmesh.NewMesh(getenvOrDefault(
        "NAVMESH_GEO_PROFILE_MESH", "../../../data/navmesh"))
    key := navmesh.RegionKey{Col: 21, Row: 20}
    tile, err := mesh.Tile(key)
    if err != nil {
        t.Fatalf("load the tile: %v", err)
    }
    payload, err := encodeNavmeshGeometry(mesh, tile)
    if err != nil {
        t.Fatalf("encode: %v", err)
    }
    t.Logf("tile 21_20: %d polys, %d links, payload %.1f MB",
        len(tile.Polys), len(tile.Links),
        float64(len(payload))/(1024*1024))
}

// getenvOrDefault answers the environment variable or the fallback.
func getenvOrDefault(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }

    return fallback
}
