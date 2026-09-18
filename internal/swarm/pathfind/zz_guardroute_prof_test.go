// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "os"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The guard route probe: the web viewer route flow (the mesh search
// with the capsule guard oracle) over a full world pack. Skips
// without the GUARD_ROUTE_PROFILE=1 environment (the data harness of
// the 2026-09-18 round, not a CI contract): point
// GUARD_ROUTE_PROFILE_MESH and GUARD_ROUTE_PROFILE_GEODATA at the
// built pack and the geodata directory (the defaults name the
// repository layout data/navmesh and data/geodata).
func TestGuardRouteProfile(t *testing.T) {
    if os.Getenv("GUARD_ROUTE_PROFILE") == "" {
        t.Skip("set GUARD_ROUTE_PROFILE=1 to run the world probe")
    }
    meshDir := getenvOrDefault("GUARD_ROUTE_PROFILE_MESH",
        "../../../data/navmesh")
    geodataDir := getenvOrDefault("GUARD_ROUTE_PROFILE_GEODATA",
        "../../../data/geodata")
    mesh := navmesh.NewMesh(meshDir)
    engine := NewEngine(geodataDir)
    capsule := NewCapsule(engine)

    filter := navmesh.DefaultFilter()
    filter.WaypointClearance = engine.CapsuleRadius()
    if filter.WaypointClearance == 0 {
        engine.SetCapsuleClearance(DefaultCollisionRadius)
        filter.WaypointClearance = DefaultCollisionRadius
    }
    filter.Smooth = true
    filter.Guard = capsule

    start := navmesh.Pos{X: 11413, Y: 41533, Z: -3680}
    end := navmesh.Pos{X: 58083, Y: 89830, Z: -3608}

    // The first call pays the tile decode (cold); the second call
    // matches the warm mesh of the viewer repeat.
    route, err := mesh.Route(start, end, filter)
    if err != nil {
        t.Fatalf("cold route: %v", err)
    }
    if route == nil || !route.Found {
        t.Fatalf("cold route not found")
    }
    t.Logf("cold: %d waypoints, %d raw, explored %d",
        len(route.Waypoints), len(route.RawWaypoints), route.Explored)

    route, err = mesh.Route(start, end, filter)
    if err != nil {
        t.Fatalf("warm route: %v", err)
    }
    if route == nil || !route.Found {
        t.Fatalf("warm route not found")
    }
    t.Logf("warm: %d waypoints, %d raw, explored %d",
        len(route.Waypoints), len(route.RawWaypoints), route.Explored)
}

// getenvOrDefault answers the environment variable or the fallback.
func getenvOrDefault(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }

    return fallback
}
