// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
    "testing"
    "time"
)

// TestGiranStrandProbe walks the Gludin Giran leg replan by replan
// and names the stall point of every partial answer (the region, the
// local cell and the world position) - the anatomy of the strand the
// town route measurement reports.
func TestGiranStrandProbe(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    if len(mesh.TileFiles()) < 20 {
        t.Skip("the town corridor pack is not present")
    }
    mesh.SetCacheCapacity(8)

    start := Pos{X: -80826, Y: 149775, Z: -3043}
    end := Pos{X: 83336, Y: 147972, Z: -3404}
    _, startPos, ok := mesh.FindNearestPoly(start)
    if !ok {
        t.Fatal("the start does not bind")
    }
    _, endPos, ok := mesh.FindNearestPoly(end)
    if !ok {
        t.Fatal("the end does not bind")
    }
    current := startPos
    for iteration := 1; iteration <= 12; iteration++ {
        began := time.Now()
        leg, err := mesh.Route(current, endPos, DefaultFilter())
        if err != nil {
            t.Fatal(err)
        }
        tag := func(p Pos) string {
            col, row := RegionOfWorld(p.X, p.Y)
            lx := int(math.Mod(p.X-(float64(col)-20)*32768, 32768)) / 16
            ly := int(math.Mod(p.Y-(float64(row)-18)*32768, 32768)) / 16

            return formatRegionCell(col, row, lx, ly)
        }
        t.Logf("iter %d: found=%t partial=%t explored=%d wps=%d"+
            " length=%.0f %s from %s", iteration, leg.Found,
            leg.Partial, leg.Explored, len(leg.Waypoints),
            routeLength(leg), time.Since(began).Round(time.Millisecond),
            tag(current))
        if leg.Found {
            t.Logf("  REACHED after %d replans", iteration)

            return
        }
        if len(leg.Waypoints) < 2 {
            t.Logf("  the partial answer carries no advance pair")

            return
        }
        next := leg.Waypoints[len(leg.Waypoints)-1]
        t.Logf("  advances to %s (%.0f, %.0f, %.0f), step %.0f", tag(next),
            next.X, next.Y, next.Z, dist3(current, next))
        if dist3(current, next) < 1000 {
            t.Logf("  STALLED: the advance is under the step floor")

            return
        }
        current = next
    }
    t.Logf("the walk exhausted the replan budget without the arrival")
}

// formatRegionCell renders the region and cell of a stall point.
func formatRegionCell(col, row int16, lx, ly int) string {
    return strconvFormat(int32(col)) + "_" + strconvFormat(int32(row)) +
        " cell " + strconvFormat(int32(lx)) + "_" +
        strconvFormat(int32(ly))
}

// strconvFormat is a small int formatter keeping the probe imports
// light.
func strconvFormat(v int32) string {
    if v == 0 {
        return "0"
    }
    digits := ""
    neg := v < 0
    if neg {
        v = -v
    }
    for v > 0 {
        digits = string('0'+v%10) + digits
        v /= 10
    }
    if neg {
        digits = "-" + digits
    }

    return digits
}
