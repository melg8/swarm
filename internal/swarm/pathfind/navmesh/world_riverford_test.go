// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
    "os"
    "path/filepath"
    "testing"
)

// TestWorldRiverFord pins the zone aware swim pricing on the owner
// routes across the Giran river (the 20_19..21_20 pack): the probe
// logs the answers of the shipped 3x uniform model, the measured 2.3
// one and the zone armed one - the river beds the server water zone
// data omits walk at the plain land rate (the zone data, not the
// depth, prices the swim). The regression contract: the zone armed
// answer of the owner route pair must not price above the measured
// flat answer under the zone aware model.
func TestWorldRiverFord(t *testing.T) {
    dir := "../../../../data/navmesh"
    if _, err := os.Stat(filepath.Join(dir, "20_19.nm")); err != nil {
        t.Skipf("the tile pack is not present: %v", err)
    }
    mesh := NewMesh(dir)
    pairs := [][2]Pos{
        {{X: 5496, Y: 89455, Z: -3440}, {X: 54587, Y: 39535, Z: -3728}},
        {{X: 55040, Y: 40146, Z: -3722}, {X: 17269, Y: 90553, Z: -3656}},
    }
    models := []struct {
        name   string
        filter func() Filter
    }{
        {"shipped3x", func() Filter {
            f := DefaultFilter()
            f.WaterCost = 3

            return f
        }},
        {"measured23", func() Filter {
            f := DefaultFilter()

            return f
        }},
        {"zones23", func() Filter {
            f := DefaultFilter()
            f.WaterZones = C1WaterZones()

            return f
        }},
    }

    for i, pair := range pairs {
        start, end := pair[0], pair[1]
        for _, model := range models {
            route, err := mesh.Route(start, end, model.filter())
            if err != nil {
                t.Fatalf("pair %d %s: %v", i, model.name, err)
            }
            t.Logf("pair %d %-10s: found=%t waypoints=%d "+
                "explored=%d length=%.0f water=%d zoneCost=%.0f",
                i, model.name, route.Found, len(route.Waypoints),
                route.Explored, fordPolylineLength(route.Waypoints),
                fordWaterWaypoints(route.Waypoints),
                fordZoneAwareCost(route.Waypoints))
            if !route.Found {
                t.Fatalf("pair %d %s: the route must be found",
                    i, model.name)
            }
        }
    }

    // The regression contract on the owner route pair: the zone
    // armed search must not answer a corridor the zone aware model
    // prices above the measured flat answer.
    zoned := DefaultFilter()
    zoned.WaterZones = C1WaterZones()
    flat := DefaultFilter()
    zonedRoute, err := mesh.Route(pairs[0][0], pairs[0][1], zoned)
    if err != nil {
        t.Fatalf("zoned route: %v", err)
    }
    flatRoute, err := mesh.Route(pairs[0][0], pairs[0][1], flat)
    if err != nil {
        t.Fatalf("flat route: %v", err)
    }
    // The tolerance carries the search approximation noise (the
    // funnel and the corridor center walk of two independent
    // searches of nearly equal cost): the contract pins the
    // dishonesty class (the zoned search overpaying the water the
    // zone data walks at the land rate), not the sub per mille
    // corridor ties.
    if fordZoneAwareCost(zonedRoute.Waypoints) >
        fordZoneAwareCost(flatRoute.Waypoints)*1.001 {
        t.Fatalf("the zone armed search answered the costlier corridor: "+
            "zoned %.0f over flat %.0f",
            fordZoneAwareCost(zonedRoute.Waypoints),
            fordZoneAwareCost(flatRoute.Waypoints))
    }
}

// fordPolylineLength sums the 3D polyline of the waypoints.
func fordPolylineLength(points []Pos) float64 {
    total := 0.0
    for i := 1; i < len(points); i++ {
        total += dist3(points[i-1], points[i])
    }

    return total
}

// fordWaterWaypoints counts the waypoints standing below the C1
// water surface.
func fordWaterWaypoints(points []Pos) int {
    count := 0
    for _, p := range points {
        if p.Z < -3780 {
            count++
        }
    }

    return count
}

// fordZoneAwareCost prices the polyline under the zone aware model:
// the segment pays the run/swim ratio on the zoned swim ground, the
// plain rate elsewhere - the character time the server actually
// charges.
func fordZoneAwareCost(points []Pos) float64 {
    if len(points) < 2 {
        return 0
    }
    zones := C1WaterZones()
    total := 0.0
    for i := 1; i < len(points); i++ {
        a, b := points[i-1], points[i]
        segment := dist3(a, b)
        mx, my := (a.X+b.X)*0.5, (a.Y+b.Y)*0.5
        mz := math.Min(a.Z, b.Z)
        swim := false
        for _, box := range zones {
            if mx >= box.MinX && mx <= box.MaxX && my >= box.MinY &&
                my <= box.MaxY && mz <= box.MaxZ {
                swim = true

                break
            }
        }
        if swim {
            segment *= 2.3
        }
        total += segment
    }

    return total
}
