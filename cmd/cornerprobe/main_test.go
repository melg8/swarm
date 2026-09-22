// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "math"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// The smoke pass of issue #9: the pure helpers of the corner audit
// probe - the back extrapolation of a leg height, the coordinate
// adapters and the hunt navigator filter it mirrors.

func TestLegZ(t *testing.T) {
    t.Run("a sub unit leg answers the waypoint height", func(t *testing.T) {
        wp := navmesh.Pos{X: 100, Y: 100, Z: -3000}
        prev := navmesh.Pos{X: 100.5, Y: 100.5, Z: -2900}
        require.InDelta(t, -3000.0, legZ(wp, prev, 40), 1e-9)
    })

    t.Run("a long leg interpolates the back distance", func(t *testing.T) {
        // A 100 unit leg from -3000 to -2900: 25 units back from
        // the waypoint sits 25% down toward the previous height.
        wp := navmesh.Pos{X: 100, Y: 100, Z: -3000}
        prev := navmesh.Pos{X: 200, Y: 100, Z: -2900}
        require.InDelta(t, -2975.0, legZ(wp, prev, 25), 1e-9)
    })

    t.Run("the diagonal leg measures the hypotenuse", func(t *testing.T) {
        // A 3-4-5 triangle leg: 5 units back of a 50 unit leg drops
        // 10% of the height difference.
        wp := navmesh.Pos{X: 0, Y: 0, Z: -3000}
        prev := navmesh.Pos{X: 30, Y: 40, Z: -2900}
        require.InDelta(t, -2990.0, legZ(wp, prev, 5), 1e-9)
    })
}

func TestCoordinateAdapters(t *testing.T) {
    t.Run("vec builds the pathfind triple", func(t *testing.T) {
        require.Equal(t, pathfind.Vec3{X: 1, Y: 2, Z: 3}, vec(1, 2, 3))
    })

    t.Run("meshPos carries the triple into the mesh space", func(t *testing.T) {
        require.Equal(t, navmesh.Pos{X: 1, Y: 2, Z: 3},
            meshPos(pathfind.Vec3{X: 1, Y: 2, Z: 3}))
    })
}

func TestClearedFilter(t *testing.T) {
    t.Run("the filter mirrors the hunt navigator", func(t *testing.T) {
        filter := clearedFilter()
        require.InDelta(t, pathfind.DefaultCollisionRadius,
            filter.WaypointClearance, 1e-9)
        require.True(t, filter.Smooth)
        require.NotEmpty(t, filter.WaterZones,
            "the C1 water pricing rides along")
        require.False(t, math.IsNaN(float64(filter.WaypointClearance)))
    })
}
