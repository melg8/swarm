// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

// pocketWorld is the synthetic railing pocket: a mainland of two
// linked halves and a single walled cell whose only contact with the
// mainland is the shared corner - the diagonal squeeze the server's
// anti corner cut rule lets a walker through and the edge-only link
// builder cannot represent. The pocket polygon holds no links: the
// corridor search can never leave it.
//
//    y
//    ^    P (1 cell, no links)
//    160 +---+---------+
//        |   | north B |
//     80 | A +---------+
//        |   | south B |
//      0 +---+---------+
//        0  160        320 > x
func pocketWorld() *Tile {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 160, y1: 80, h: 0, area: AreaGround},      // 0 A
        {x0: 0, y0: 80, x1: 160, y1: 160, h: 0, area: AreaGround},    // 1 B
        {x0: 160, y0: 160, x1: 161, y1: 161, h: 0, area: AreaGround}, // 2 P
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxY, to: 1, t0: 0, t1: 159},
        {poly: 1, side: SideMinY, to: 0, t0: 0, t1: 159},
    }

    return assembleTile(21, rects, links, nil)
}

// TestRouteEscapesTheAxisWalledPocket pins the pocket escape: a start
// standing on a linkless one cell polygon (the railing pocket the
// edge-only link builder cannot connect - every axis neighbor walled,
// the diagonal squeeze the server movement allows carries the only
// way out) must still produce a walkable answer: the closest
// reachable route OUT of the pocket toward the destination (the
// partial contract). The pre escape answer was the bare not found -
// the bot standing in such a cell could not build a route anywhere.
func TestRouteEscapesTheAxisWalledPocket(t *testing.T) {
    mesh := NewMesh(writeTiles(t, pocketWorld()))
    start := worldPos(160.5, 160.5, 0)
    dest := worldPos(80, 40, 0)

    ref, _, ok := mesh.FindNearestPoly(start)
    require.True(t, ok)
    _, poly := mesh.polyOfRef(ref)
    require.Negative(t, poly.FirstLink,
        "the pocket precondition: the standing polygon holds no links")
    require.Equal(t, int32(1), poly.X1-poly.X0,
        "the pocket precondition: a single cell polygon")
    require.Equal(t, int32(1), poly.Y1-poly.Y0)

    route, err := mesh.Route(start, dest, DefaultFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.False(t, route.Found,
        "the destination itself stays unreachable from the pocket")
    require.True(t, route.Partial,
        "the pocket escape must answer the closest reachable route out")
    require.Len(t, route.Waypoints, 1,
        "the plan is the single exit aim, never the standing point")
    exit := route.Waypoints[0]
    // The exit lands deep on the mainland past the shared corner
    // (region 21_19 anchored at 32768): outside the pocket cell, far
    // enough for the walk machinery to click and arrive, on connected
    // ground the later plans route from.
    require.Less(t, exit.X, 35280.0)
    require.Less(t, exit.Y, 35280.0)
    require.Greater(t, exit.X, 35072.0)
    require.InDelta(t, 0.0, exit.Z, 1e-9)
}

// TestPocketEscapePrefersTheDryGround pins the water pricing of the
// exit search: a water polygon stacked under the pocket (the deck
// sandwich shape - the bed 80 units below the deck cell) is a
// candidate the 3D distance would happily pick against nothing, the
// escape pricing keeps the exit on the dry connected deck instead of
// aiming the walk into the water under it.
func TestPocketEscapePrefersTheDryGround(t *testing.T) {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 160, y1: 80, h: 0, area: AreaGround},       // 0 A
        {x0: 0, y0: 80, x1: 160, y1: 160, h: 0, area: AreaGround},     // 1 B
        {x0: 160, y0: 160, x1: 161, y1: 161, h: -80, area: AreaWater}, // 2 bed
        {x0: 160, y0: 160, x1: 161, y1: 161, h: 0, area: AreaGround},  // 3 P
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxY, to: 1, t0: 0, t1: 159},
        {poly: 1, side: SideMinY, to: 0, t0: 0, t1: 159},
    }
    mesh := NewMesh(writeTiles(t, assembleTile(21, rects, links, nil)))
    start := worldPos(160.5, 160.5, 0)
    dest := worldPos(80, 40, 0)

    route, err := mesh.Route(start, dest, DefaultFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.True(t, route.Partial)
    require.Len(t, route.Waypoints, 1)
    require.InDelta(t, 0.0, route.Waypoints[0].Z, 1e-9,
        "the exit stays on the dry deck, never the bed below")
}

// TestPocketEscapeSparesTheWideComponent pins the flood bound: a
// start whose link component is honest ground (a wide isolated area
// - the whole sealed yard case) keeps the plain answers, the escape
// never fabricates an exit route for it even when other ground lies
// next door (the escape exists for the stranded spot, not for the
// sealed area the walk-what-you-can partial already serves).
func TestPocketEscapeSparesTheWideComponent(t *testing.T) {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 40, y1: 40, h: 0, area: AreaGround}, // wide yard
        {x0: 41, y0: 0, x1: 42, y1: 1, h: 0, area: AreaGround}, // neighbor
        {x0: 42, y0: 0, x1: 43, y1: 1, h: 0, area: AreaGround}, // pair
    }
    links := []linkSpec{
        {poly: 1, side: SideMaxX, to: 2, t0: 0, t1: 0},
        {poly: 2, side: SideMinX, to: 1, t0: 0, t1: 0},
    }
    mesh := NewMesh(writeTiles(t, assembleTile(21, rects, links, nil)))
    start := worldPos(20, 20, 0)
    dest := worldPos(41.5, 0.5, 0)

    route, err := mesh.Route(start, dest, DefaultFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.False(t, route.Found)
    require.False(t, route.Partial,
        "the wide component keeps the plain not found")
    require.Empty(t, route.Waypoints)
}

// navmeshDataDir finds the navmesh tile directory the way the bot
// mode detects it: data/navmesh above the package directory. The
// tiles are a local runtime artifact (cmd/navmesh-build, gitignored)
// the same way the geodata pack is.
func navmeshDataDir() string {
    dir, err := os.Getwd()
    if err != nil {
        return ""
    }
    for range 6 {
        candidate := filepath.Join(dir, "data", "navmesh")
        if info, err := os.Stat(candidate); err == nil && info.IsDir() {
            return candidate
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            break
        }
        dir = parent
    }

    return ""
}

// TestReproRailingPocket43736 is the live reproduction of the
// reported cell: from 43736 47048 -2992 (the elven village deck cell
// just off the railings) the mesh could not build a route anywhere -
// the standing polygon is a linkless one cell island (every axis
// neighbor walled by the railing segments, the diagonal squeeze the
// server movement allows is the only way out), the corridor search
// answered the bare not found for every destination while the grid
// engine found routes fine. The pocket escape answers the walk out.
func TestReproRailingPocket43736(t *testing.T) {
    dir := navmeshDataDir()
    if dir == "" {
        t.Skip("no local navmesh tiles, the dump reproduction needs them")
    }
    mesh := NewMesh(dir)
    start := Pos{X: 43736, Y: 47048, Z: -2992}

    ref, _, ok := mesh.FindNearestPoly(start)
    require.True(t, ok)
    _, poly := mesh.polyOfRef(ref)
    require.Negative(t, poly.FirstLink,
        "the dump precondition: the standing polygon is a linkless island")
    x0, y0, x1, y1 := meshTileOf(t, mesh, ref).WorldRect(poly)

    dests := []Pos{
        {X: 43836, Y: 47148, Z: -2992}, // a hundred units southeast
        {X: 25500, Y: 51095, Z: -3408}, // the hunting zone center
        {X: 43032, Y: 50408, Z: -2992}, // the village plaza
    }
    for _, dest := range dests {
        route, err := mesh.Route(start, dest, DefaultFilter())
        require.NoError(t, err)
        require.NotNil(t, route)
        require.False(t, route.Found,
            "the destination itself stays unreachable from the pocket")
        require.True(t, route.Partial,
            "the pocket escape must answer the walk out of %v", dest)
        require.Len(t, route.Waypoints, 1)
        exit := route.Waypoints[0]
        inPocket := exit.X >= x0 && exit.X < x1 && exit.Y >= y0 &&
            exit.Y < y1
        require.False(t, inPocket,
            "the exit must leave the pocket cell, got %v", exit)
        require.GreaterOrEqual(t, math.Hypot(exit.X-start.X,
            exit.Y-start.Y), 48.0,
            "the exit aim must stand far enough for the walk clicks")
    }
}

// meshTileOf resolves the tile of a reference for the assertions.
func meshTileOf(t *testing.T, mesh *Mesh, ref PolyRef) *Tile {
    t.Helper()
    col, row := TileOf(ref)
    tile, err := mesh.Tile(RegionKey{Col: col, Row: row})
    require.NoError(t, err)

    return tile
}
