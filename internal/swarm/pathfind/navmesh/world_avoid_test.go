// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
        "math"
        "os"
        "testing"
        "time"
)

// worldPackDir resolves the shipped world pack of the sandbox checkout
// (the tests skip when the pack is absent).
func worldPackDir(t *testing.T) string {
        t.Helper()
        dir := "../../../../data/navmesh-world"
        if _, err := os.Stat(dir + "/20_19.nm"); err != nil {
                t.Skipf("the world pack is not present: %v", err)
        }

        return dir
}

// worldRouteProfile measures the walk answer of one route: the total
// length, the water share (the polygon area class under every raw
// waypoint leg midpoint) and the search cost.
type worldRouteProfile struct {
        length   float64
        waterLen float64
        explored int
        duration time.Duration
        found    bool
        lastX    float64
        lastY    float64
}

// profileWorldRoute runs one route over the world mesh and walks the
// corridor polys for the water share.
func profileWorldRoute(t *testing.T, mesh *Mesh, start, end Pos,
        filter Filter) worldRouteProfile {
        t.Helper()
        began := time.Now()
        route, err := mesh.Route(start, end, filter)
        if err != nil {
                t.Fatalf("route: %v", err)
        }
        profile := worldRouteProfile{
                explored: route.Explored,
                duration: time.Since(began),
                found:    route.Found,
        }
        if len(route.Waypoints) > 0 {
                last := route.Waypoints[len(route.Waypoints)-1]
                profile.lastX, profile.lastY = last.X, last.Y
        }
        waypoints := route.Waypoints
        if len(route.RawWaypoints) > 0 {
                waypoints = route.RawWaypoints
        }
        water := map[PolyRef]bool{}
        for _, ref := range route.Corridor {
                col, row := TileOf(ref)
                tile, err := mesh.Tile(RegionKey{Col: col, Row: row})
                if err != nil || int(PolyOf(ref)) >= len(tile.Polys) {
                        continue
                }
                water[ref] = tile.Polys[PolyOf(ref)].Area == AreaWater
        }
        for i := 0; i+1 < len(waypoints); i++ {
                a, b := waypoints[i], waypoints[i+1]
                leg := math.Hypot(b.X-a.X, b.Y-a.Y)
                profile.length += leg
                for _, ref := range route.Corridor {
                        col, row := TileOf(ref)
                        tile, _ := mesh.Tile(RegionKey{Col: col, Row: row})
                        if tile == nil {
                                continue
                        }
                        poly := &tile.Polys[PolyOf(ref)]
                        x0, y0, x1, y1 := tile.WorldRect(poly)
                        mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
                        if mx >= x0 && mx <= x1 && my >= y0 && my <= y1 {
                                if water[ref] {
                                        profile.waterLen += leg
                                }

                                break
                        }
                }
        }

        return profile
}

// TestWorldTownAvoidRoute pins the town bypass of the world pack: the
// diagonal across the walled island town answers the honest through
// town walk (the flat contest of the hierarchical query brought the
// once broken 90.5k answer to the 65.9k optimum), the avoid circle
// over the island bans the town and the answer takes the moat detour
// - 66.6k units, found under the raised budget the capped banned
// query rerun owns. The regression keeps the avoid mechanism honest
// on the real pack (the synthetic avoid tests stay toy sized): the
// banned answer stays within the detour margin of the free optimum
// (the ban costs the walk the moat, not a search failure).
func TestWorldTownAvoidRoute(t *testing.T) {
        mesh := NewMesh(worldPackDir(t))
        start := Pos{X: 55040, Y: 40146, Z: -3722}
        end := Pos{X: 17269, Y: 90553, Z: -3656}
        town := AvoidCircle{CenterX: 45020, CenterY: 49220, Radius: 3000}

        straight := profileWorldRoute(t, mesh, start, end, DefaultFilter())
        t.Logf("through town: len %.0f, water %.0f (%.1f%%), explored %d,"+
                " %s, found %v", straight.length, straight.waterLen,
                100*straight.waterLen/straight.length, straight.explored,
                straight.duration, straight.found)

        avoidFilter := DefaultFilter()
        avoidFilter.Avoid = []AvoidCircle{town}
        around := profileWorldRoute(t, mesh, start, end, avoidFilter)
        t.Logf("around town: len %.0f, water %.0f (%.1f%%), explored %d,"+
                " %s, found %v", around.length, around.waterLen,
                100*around.waterLen/around.length, around.explored,
                around.duration, around.found)
        route, err := mesh.Route(start, end, avoidFilter)
        if err != nil {
                t.Fatalf("avoid route: %v", err)
        }
        if len(route.Waypoints) > 0 {
                last := route.Waypoints[len(route.Waypoints)-1]
                t.Logf("partial endpoint: (%.0f, %.0f, %.0f), partial %v",
                        last.X, last.Y, last.Z, route.Partial)
        }

        if !around.found {
                t.Fatalf("the avoid route lost the destination"+
                        " (partial endpoint %.0f, %.0f)", around.lastX, around.lastY)
        }
        if around.length >= straight.length*1.02 {
                t.Fatalf("the ban must not price the moat detour past the"+
                        " honest margin: %.0f over %.0f",
                        around.length, straight.length)
        }
}
