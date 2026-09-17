// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "math"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// The funnel clearance tests: the exact Detour pivot is the portal
// span end - the point where the open span meets the wall - and a
// character capsule turning exactly there clips the wall corner (the
// owner's sticking report). The WaypointClearance filter pulls the
// pivots inward; an L corridor with known walls pins the geometry.

// lCorridor builds a region whose walkable surface is an L shaped
// corridor: the east arm (cells x 0..15, y 0..1) turns south into the
// vertical arm (cells x 14..15, y 2..7). Everything else is void, so
// the corridor boundary walls are exactly known.
func lCorridor(t *testing.T) *RegionBuild {
    t.Helper()
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        inEast := cx < 16 && cy < 2
        inSouth := cx >= 14 && cx < 16 && cy >= 2 && cy < 8
        if inEast || inSouth {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }

        return nil
    })
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)

    return build
}

// boundaryDistance returns the distance from the world point to the
// corridor boundary segments of the L corridor (the walls the capsule
// must not clip).
func boundaryDistance(x, y float64) float64 {
    base := 32768.0
    px, py := x-base, y-base
    segments := [6][4]float64{
        // north edge of the east arm, its west edge, the south edge
        // of the east arm up to the junction, the west edge of the
        // south arm below the junction, the south edge of the south
        // arm, its east edge.
        {0, 0, 256, 0},
        {0, 0, 0, 32},
        {0, 32, 224, 32},
        {224, 32, 224, 128},
        {224, 128, 256, 128},
        {256, 0, 256, 128},
    }
    best := math.MaxFloat64
    for _, s := range segments {
        dx, dy := s[2]-s[0], s[3]-s[1]
        t := (px-s[0])*dx + (py-s[1])*dy
        if dx*dx+dy*dy > 0 {
            t /= dx*dx + dy*dy
        }
        t = math.Max(0, math.Min(1, t))
        ex, ey := s[0]+t*dx-px, s[1]+t*dy-py
        best = math.Min(best, math.Hypot(ex, ey))
    }

    return best
}

// TestFunnelClearancePivot pins the pivot behavior: without the
// clearance the turn waypoint sits on the inner corner (the wall
// boundary itself), with the radius it keeps the capsule away from
// both walls, and every waypoint of the cleared route stays outside
// the capsule radius of the boundary.
func TestFunnelClearancePivot(t *testing.T) {
    tile := lCorridor(t).Tile
    mesh := writeAndLoad(t, tile)

    start := worldPos(2.5, 0.5, -3504)
    end := worldPos(14.5, 6.5, -3504)

    raw, err := mesh.Route(start, end, navmesh.DefaultFilter())
    require.NoError(t, err)
    require.True(t, raw.Found)
    closest := math.MaxFloat64
    for _, wp := range raw.Waypoints {
        if wp.X == start.X && wp.Y == start.Y {
            continue
        }
        if wp.X == end.X && wp.Y == end.Y {
            continue
        }
        closest = math.Min(closest, boundaryDistance(wp.X, wp.Y))
    }
    require.Less(t, closest, 1.0,
        "the raw funnel pivots on the wall boundary")

    cleared := navmesh.DefaultFilter()
    cleared.WaypointClearance = 7.5
    safe, err := mesh.Route(start, end, cleared)
    require.NoError(t, err)
    require.True(t, safe.Found)
    closest = math.MaxFloat64
    for _, wp := range safe.Waypoints {
        closest = math.Min(closest, boundaryDistance(wp.X, wp.Y))
    }
    require.GreaterOrEqual(t, closest, 7.4,
        "every waypoint of the cleared route keeps the capsule radius")
}
