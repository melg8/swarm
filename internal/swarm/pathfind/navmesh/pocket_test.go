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

// TestPocketEscapeServesTheWideTerrace pins the terrace class of the
// stranded components (the 2026-09-20 stuck point reports): a wide
// sealed yard whose link component fits the pocket box (640 units -
// the measured terraces run 592..752) with connected ground next door
// answers the walk out of it. The pre terrace answer kept the plain
// not found for every component past the old 320 side - the walk
// what you can partial does NOT serve the wide strands (the astar
// partial walks the character to the component's inner boundary and
// strands it there, the next cycle replans the same partial), so the
// escape is the only answer that walks it out.
func TestPocketEscapeServesTheWideTerrace(t *testing.T) {
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
    require.False(t, route.Found,
        "the destination itself stays unreachable from the yard")
    require.True(t, route.Partial,
        "the escape must answer the walk out of the wide strand")
    require.True(t, route.PocketEscape)
    require.Len(t, route.Waypoints, 1,
        "the plan is the single exit aim, never the standing point")
    exit := route.Waypoints[0]
    require.False(t, exit.X >= 32768 && exit.X < 32768+640 &&
        exit.Y >= 32768 && exit.Y < 32768+640,
        "the exit must leave the yard box, got %v", exit)
    require.GreaterOrEqual(t, math.Hypot(exit.X-start.X, exit.Y-start.Y),
        48.0, "the exit aim must stand far enough for the walk clicks")
}

// TestPocketEscapeSparesTheWideComponent pins the flood bound: a
// start whose link component outgrows the pocket box (a 2240 unit
// sealed area - the honest ground scale, the mainland and the big
// islands live past the bound) keeps the plain answers, the escape
// never fabricates an exit route for it.
func TestPocketEscapeSparesTheWideComponent(t *testing.T) {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 140, y1: 140, h: 0, area: AreaGround}, // huge yard
        {x0: 141, y0: 0, x1: 142, y1: 1, h: 0, area: AreaGround}, // neighbor
        {x0: 142, y0: 0, x1: 143, y1: 1, h: 0, area: AreaGround}, // pair
    }
    links := []linkSpec{
        {poly: 1, side: SideMaxX, to: 2, t0: 0, t1: 0},
        {poly: 2, side: SideMinX, to: 1, t0: 0, t1: 0},
    }
    mesh := NewMesh(writeTiles(t, assembleTile(21, rects, links, nil)))
    start := worldPos(70, 70, 0)
    dest := worldPos(141.5, 0.5, 0)

    route, err := mesh.Route(start, dest, DefaultFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.False(t, route.Found)
    require.False(t, route.Partial,
        "the wide component keeps the plain not found")
    require.Empty(t, route.Waypoints)
}

// TestPocketEscapeSparesTheVerticalStack pins the horizontal
// displacement floor of the exit scan (the 2026-09-20 stuck point
// report): a stranded spot stacked OVER the connected deck (the
// terrace cell 32 units above the surrounding ground) must not exit
// through the ground directly under the standing point - the deck
// under the start and the diagonal neighbors sharing its corner
// answer the closest 3D points of the whole scan, but the character
// cannot walk straight down onto them and the boundary they form
// carries no horizontal displacement (the aim march degenerates into
// the standing cell - the follower would click its own position and
// the bot would stand frozen with a plan in hand). The exit must land
// on the horizontally displaced deck the walk can reach.
func TestPocketEscapeSparesTheVerticalStack(t *testing.T) {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 30, y1: 30, h: 0, area: AreaGround},     // deck
        {x0: 30, y0: 30, x1: 31, y1: 31, h: -32, area: AreaGround}, // spot
        {x0: 40, y0: 0, x1: 50, y1: 10, h: 0, area: AreaGround},    // mainland
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxX, to: 2, t0: 0, t1: 159},
        {poly: 2, side: SideMinX, to: 0, t0: 0, t1: 159},
    }
    mesh := NewMesh(writeTiles(t, assembleTile(21, rects, links, nil)))
    start := worldPos(30.5, 30.5, -32)
    dest := worldPos(5, 5, 0)

    route, err := mesh.Route(start, dest, DefaultFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.False(t, route.Found)
    require.True(t, route.Partial,
        "the spot is a stranded one poly component, the escape answers")
    require.Len(t, route.Waypoints, 1)
    exit := route.Waypoints[0]
    horizontal := math.Hypot(exit.X-start.X, exit.Y-start.Y)
    require.GreaterOrEqual(t, horizontal, 48.0,
        "the exit must stand horizontally displaced from the standing "+
            "point, got %v", exit)
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

// stuckTerraces are the 2026-09-20 stuck point report: the bot
// positions the owner found the fleet frozen on. The two spots split
// after the column-first FindNearestPoly round (the guard stairs
// round): the first stands on a cell whose honest surface is the
// -2992 ground under the terrace edge - the old pure 3D nearest
// snapped one cell north onto the sealed terrace deck (the reported z
// sits 32 units over the ground) and stranded it - while the column
// of the second is genuinely sealed terrace (29 polygons, the
// 592x432 box) and keeps the pocket escape contract. The first spot
// routes found again, the second walks out through the escape.
func TestReproStuckTerraces43632And41920(t *testing.T) {
    dir := navmeshDataDir()
    if dir == "" {
        t.Skip("no local navmesh tiles, the dump reproduction needs them")
    }
    mesh := NewMesh(dir)
    dests := []Pos{
        {X: 43032, Y: 50408, Z: -2992}, // the village plaza
        {X: 25500, Y: 51095, Z: -3408}, // the hunting zone center
        {X: 45478, Y: 49730, Z: -3056}, // the village center
    }

    // Terrace1 (43632 50560 -2960): the reported z drifts 32 units
    // over the honest cell surface; the column containment binds the
    // connected ground and every route answers found - the spot is
    // cured at the binding level, no escape needed.
    cured := Pos{X: 43632, Y: 50560, Z: -2960}
    ref, bound, ok := mesh.FindNearestPoly(cured)
    require.True(t, ok)
    require.InDelta(t, -2992.0, bound.Z, 0.5,
        "the honest binding: the ground under the terrace edge")
    component := mesh.floodPocketComponent(ref)
    require.Nil(t, component,
        "the cured spot: the bound poly is not a pocket")
    for _, dest := range dests {
        route, err := mesh.Route(cured, dest, DefaultFilter())
        require.NoError(t, err)
        require.NotNil(t, route)
        require.True(t, route.Found,
            "the cured spot walks the route to %v", dest)
        require.False(t, route.Partial)
        require.False(t, route.PocketEscape)
    }

    // Terrace2 (41920 52128 -3000): the standing cell is genuinely
    // sealed terrace - every link direction walled, the diagonal
    // squeezes the server movement allows carry the only way out the
    // mesh links do not carry. The escape answers the walk out.
    sealed := Pos{X: 41920, Y: 52128, Z: -3000}
    ref, _, ok = mesh.FindNearestPoly(sealed)
    require.True(t, ok)
    _, poly := mesh.polyOfRef(ref)
    require.GreaterOrEqual(t, poly.FirstLink, int32(0),
        "the terrace precondition: the standing polygon carries "+
            "links (the component is the terrace, not a one cell "+
            "island)")
    component = mesh.floodPocketComponent(ref)
    require.NotNil(t, component,
        "the terrace precondition: the component fits the pocket "+
            "box (the strand the escape serves)")
    side := math.Max(component.maxX-component.minX,
        component.maxY-component.minY)
    require.Greater(t, side, 320.0,
        "the terrace precondition: the component outgrows the old "+
            "pocket side (the class the old bound refused)")
    require.LessOrEqual(t, side, pocketMaxSide,
        "the terrace precondition: the component fits the bound")

    x0, y0, x1, y1 := meshTileOf(t, mesh, ref).WorldRect(poly)
    for _, dest := range dests {
        route, err := mesh.Route(sealed, dest, DefaultFilter())
        require.NoError(t, err)
        require.NotNil(t, route)
        require.False(t, route.Found,
            "the destination itself stays unreachable from the "+
                "terrace")
        require.True(t, route.Partial,
            "the escape must answer the walk out toward %v", dest)
        require.True(t, route.PocketEscape)
        require.Len(t, route.Waypoints, 1)
        exit := route.Waypoints[0]
        inStart := exit.X >= x0 && exit.X < x1 && exit.Y >= y0 &&
            exit.Y < y1
        require.False(t, inStart,
            "the exit must leave the standing cell, got %v", exit)
        horizontal := math.Hypot(exit.X-sealed.X, exit.Y-sealed.Y)
        require.GreaterOrEqual(t, horizontal, 48.0,
            "the exit must stand horizontally displaced far enough "+
                "for the walk clicks, got %v", exit)
    }
}
