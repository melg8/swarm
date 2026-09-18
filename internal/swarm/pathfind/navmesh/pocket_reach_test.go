// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "testing"
)

// TestPocketReach maps the reachability of the Giran strand pocket:
// from the stall cell (20_22 west edge, y 1024) the probe routes to
// the eastward targets along the Giran line, the northward targets
// toward the row 21 pass and the westward back to the seam - the
// reachable set names the pocket boundary the search dies against.
func TestPocketReach(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    if len(mesh.TileFiles()) < 20 {
        t.Skip("the town corridor pack is not present")
    }
    mesh.SetCacheCapacity(8)
    stall := Pos{X: 0, Y: 147456, Z: -3720}
    targets := []struct {
        name string
        pos  Pos
    }{
        {"east 8k", Pos{X: 8000, Y: 147972, Z: -3500}},
        {"east 16k", Pos{X: 16000, Y: 147972, Z: -3500}},
        {"east 24k", Pos{X: 24000, Y: 147972, Z: -3500}},
        {"north 8k", Pos{X: 0, Y: 155456, Z: -3500}},
        {"north 24k", Pos{X: 0, Y: 171456, Z: -3500}},
        {"south 8k", Pos{X: 0, Y: 139456, Z: -3500}},
        {"seam back west", Pos{X: -400, Y: 147456, Z: -3720}},
        {"giran", Pos{X: 83336, Y: 147972, Z: -3404}},
    }
    for _, tg := range targets {
        endRef, _, ok := mesh.FindNearestPoly(tg.pos)
        if !ok {
            t.Logf("%s (%.0f %.0f): the target does not bind", tg.name,
                tg.pos.X, tg.pos.Y)

            continue
        }
        route, err := mesh.Route(stall, tg.pos, DefaultFilter())
        if err != nil {
            t.Fatal(err)
        }
        col, row := TileOf(endRef)
        t.Logf("%s (%.0f %.0f -> tile %d_%d): found=%t partial=%t"+
            " explored=%d length=%.0f", tg.name, tg.pos.X, tg.pos.Y, col,
            row, route.Found, route.Partial, route.Explored,
            routeLength(route))
    }
}
