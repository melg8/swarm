// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestElvenForestToGludioProbe measures the corridor route of the
// route abuse acceptance scenarios on the local corridor pack.
func TestElvenForestToGludioProbe(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    if len(mesh.TileFiles()) < 9 {
        t.Skip("the corridor pack is not present")
    }
    mesh.SetCacheCapacity(9)

    start := Pos{X: 46045, Y: 41251, Z: -3440}
    end := Pos{X: -12787, Y: 122779, Z: -3114}
    startRef, startPos, ok := mesh.FindNearestPoly(start)
    require.True(t, ok, "the elven forest spawn must bind")
    endRef, endPos, ok := mesh.FindNearestPoly(end)
    require.True(t, ok, "the Gludio arrival point must bind")
    t.Logf("bound start %v -> %v (ref %d), end %v -> %v (ref %d)",
        start, startPos, startRef, end, endPos, endRef)

    began := time.Now()
    route, err := mesh.Route(startPos, endPos, DefaultFilter())
    require.NoError(t, err)
    t.Logf("one shot: found=%t partial=%t hier=%t wps=%d length=%.0f %s",
        route.Found, route.Partial, route.Hierarchical,
        len(route.Waypoints), routeLength(route), time.Since(began))
    if route.Found {
        return
    }

    // The segmented round: walk the partial corridors.
    current := startPos
    total := 0.0
    for i := range 12 {
        began = time.Now()
        segment, err := mesh.Route(current, endPos, DefaultFilter())
        require.NoError(t, err)
        t.Logf("  segment %d: found=%t partial=%t wps=%d length=%.0f %s",
            i+1, segment.Found, segment.Partial,
            len(segment.Waypoints), routeLength(segment),
            time.Since(began))
        if segment.Found {
            total += routeLength(segment)

            break
        }
        if len(segment.Waypoints) < 2 {
            break
        }
        next := segment.Waypoints[len(segment.Waypoints)-1]
        step := dist3(current, next)
        total += step
        if step < 1000 {
            break
        }
        current = next
    }
    t.Logf("segmented total: %.0f units", total)
}
