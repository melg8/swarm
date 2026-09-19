// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "fmt"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

// rectSpec is one rectangle polygon of a synthetic test tile: uniform
// height, the area class and the link specs leaving it.
type rectSpec struct {
    x0, y0, x1, y1 int32
    h              int16
    area           uint8
}

// linkSpec is one link of a synthetic tile: the source polygon, the
// side, the internal target (or the negative external form) and the
// open portal span.
type linkSpec struct {
    poly   int32
    side   uint8
    to     int32
    t0, t1 int32
}

// assembleTile builds an in-memory tile from rectangle and link specs:
// the polygon heights are uniform per rectangle and the link chains
// follow the spec order per polygon.
func assembleTile(col int16, rects []rectSpec, links []linkSpec,
    ext []ExtLink,
) *Tile {
    tile := &Tile{
        Col:      col,
        Row:      19,
        Climb:    40,
        Polys:    make([]Poly, len(rects)),
        Links:    make([]Link, len(links)),
        ExtLinks: ext,
    }
    for i, rect := range rects {
        tile.Polys[i] = Poly{
            X0: rect.x0, Y0: rect.y0, X1: rect.x1, Y1: rect.y1,
            H00: rect.h, H10: rect.h, H01: rect.h, H11: rect.h,
            FirstLink: -1, Area: rect.area,
        }
    }
    for i, spec := range links {
        tile.Links[i] = Link{
            Side: spec.side, To: spec.to, Next: -1,
            T0: spec.t0, T1: spec.t1,
        }
        poly := &tile.Polys[spec.poly]
        if poly.FirstLink < 0 {
            poly.FirstLink = int32(i)
        } else {
            chain := poly.FirstLink
            for tile.Links[chain].Next >= 0 {
                chain = tile.Links[chain].Next
            }
            tile.Links[chain].Next = int32(i)
        }
    }
    data, err := EncodeTile(tile)
    if err != nil {
        panic(fmt.Sprintf("encode synthetic tile: %v", err))
    }
    decoded, err := DecodeTile(data)
    if err != nil {
        panic(fmt.Sprintf("decode synthetic tile: %v", err))
    }

    return decoded
}

// corridorWorld is the synthetic walk of the query tests: a mainland
// rectangle A, a deck B east of it, a shore ramp C north of B and the
// water D north of the ramp, plus a floating deck E stacked over the
// water with no connection (the isolated-surface case).
//
//    y
//    ^
//    320 +---E---+
//    208 +-C-+-D-+   C: shore ramp at -40, D: water at -80
//    160 +--B----+   B: deck at 0
//      0 +--A----+   A: mainland at 0
//        0       160 320 >x
func corridorWorld() *Tile {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 160, y1: 160, h: 0, area: AreaGround},       // 0 A
        {x0: 160, y0: 0, x1: 320, y1: 160, h: 0, area: AreaGround},     // 1 B
        {x0: 160, y0: 160, x1: 320, y1: 208, h: -40, area: AreaGround}, // 2 C
        {x0: 160, y0: 208, x1: 320, y1: 320, h: -80, area: AreaWater},  // 3 D
        {x0: 160, y0: 208, x1: 320, y1: 320, h: 100, area: AreaGround}, // 4 E
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxX, to: 1, t0: 0, t1: 159},
        {poly: 1, side: SideMinX, to: 0, t0: 0, t1: 159},
        {poly: 1, side: SideMaxY, to: 2, t0: 160, t1: 319},
        {poly: 2, side: SideMinY, to: 1, t0: 160, t1: 319},
        {poly: 2, side: SideMaxY, to: 3, t0: 160, t1: 319},
        {poly: 3, side: SideMinY, to: 2, t0: 160, t1: 319},
    }

    return assembleTile(21, rects, links, nil)
}

// writeTiles writes tiles as X_Y.nm files into a fresh directory.
func writeTiles(t *testing.T, tiles ...*Tile) string {
    t.Helper()
    dir := t.TempDir()
    for _, tile := range tiles {
        data, err := EncodeTile(tile)
        require.NoError(t, err)
        name := fmt.Sprintf("%d_%d%s", tile.Col, tile.Row, tileFileExt)
        require.NoError(t, os.WriteFile(filepath.Join(dir, name), data,
            0o600))
    }

    return dir
}

// worldPos converts region local cell coordinates of region 21_19 into
// a world position.
func worldPos(cx, cy float64, z float64) Pos {
    return Pos{X: 32768 + cx*16, Y: 32768 + cy*16, Z: z}
}

// TestFindNearestPolyStacked pins the stacked-layer disambiguation of
// the nearest polygon resolution: the water and the floating deck
// share the footprint, the 3D distance picks the right surface.
func TestFindNearestPolyStacked(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    ref, pos, ok := mesh.FindNearestPoly(worldPos(240, 264, -80))
    require.True(t, ok)
    tile, poly := mesh.polyOfRef(ref)
    require.Equal(t, AreaWater, poly.Area)
    require.InDelta(t, -80, pos.Z, 1e-9)
    // The same footprint at deck height binds to the deck polygon.
    ref, pos, ok = mesh.FindNearestPoly(worldPos(240, 264, 100))
    require.True(t, ok)
    _, poly = mesh.polyOfRef(ref)
    require.Equal(t, AreaGround, poly.Area)
    require.InDelta(t, 100, pos.Z, 1e-9)
    require.NotNil(t, tile)
}

// TestRouteCorridorAndFunnel walks the mainland to the water through
// the L-shaped corridor: the funnel bends at the ramp corner and the
// waypoints never leave the corridor rectangles.
func TestRouteCorridorAndFunnel(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    route, err := mesh.Route(worldPos(88, 88, 0), worldPos(240, 264, -80),
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.False(t, route.Partial)
    require.Len(t, route.Corridor, 4)

    // The funnel turns exactly once: at the ramp corner shared by the
    // deck, the ramp and the mainland side (local 160 160 = world
    // 33024... x: 32768+160*16? no - the cell boundary 160 maps to
    // 32768 + 160*16 = 35328? the cells span 160 units = 160 cells,
    // so the boundary of cell x 160 sits at 32768 + 160*16 = 35328).
    require.Len(t, route.Waypoints, 3)
    require.InDelta(t, 32768+88*16, route.Waypoints[0].X, 1e-6)
    require.InDelta(t, 32768+88*16, route.Waypoints[0].Y, 1e-6)
    corner := route.Waypoints[1]
    require.InDelta(t, 32768+160*16, corner.X, 1e-6)
    require.InDelta(t, 32768+160*16, corner.Y, 1e-6)
    require.InDelta(t, 0, corner.Z, 1e-6)
    last := route.Waypoints[2]
    require.InDelta(t, 32768+240*16, last.X, 1e-6)
    require.InDelta(t, 32768+264*16, last.Y, 1e-6)
    require.InDelta(t, -80, last.Z, 1e-6)
}

// TestRoutePricedCrossesTheWater pins the priced contract: a
// swim-only target answers the full found route through the water
// polygons (the swim rate prices the crossing, no walled form exists
// anymore) - the corridor walks the same ramp the escape later
// climbs back.
func TestRoutePricedCrossesTheWater(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    route, err := mesh.Route(worldPos(88, 88, 0), worldPos(240, 264,
        -80), DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.False(t, route.Partial)
    require.Len(t, route.Corridor, 4)

    last := route.Waypoints[len(route.Waypoints)-1]
    require.InDelta(t, -80, last.Z, 1e-6)
    require.InDelta(t, 32768+240*16, last.X, 1e-6)
    require.InDelta(t, 32768+264*16, last.Y, 1e-6)
}

// TestWaterEscape pins the escape contract: the way out of the water
// is the ordinary priced search to the first dry polygon, ending at
// the shore crossing; a dry start needs no escape.
func TestWaterEscape(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    route, err := mesh.WaterEscape(worldPos(240, 264, -80))
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Corridor, 2)
    last := route.Waypoints[len(route.Waypoints)-1]
    require.InDelta(t, 32768+208*16, last.Y, 1e-6)

    // A dry start answers Found=false without an error.
    route, err = mesh.WaterEscape(worldPos(88, 88, 0))
    require.NoError(t, err)
    require.False(t, route.Found)
}

// TestRouteIsolatedSurface pins the unreachable destination: the
// floating deck has no links, the route answers the closest-reachable
// partial corridor (never a found route).
func TestRouteIsolatedSurface(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    route, err := mesh.Route(worldPos(88, 88, 0), worldPos(240, 264, 100),
        DefaultFilter())
    require.NoError(t, err)
    require.False(t, route.Found)
    require.True(t, route.Partial)
    require.NotEmpty(t, route.Waypoints)
}

// TestRoutePortalSpanRestriction narrows the ramp portal: the funnel
// crossing of the narrowed edge stays inside the open span although
// the straight line would cross outside it.
func TestRoutePortalSpanRestriction(t *testing.T) {
    tile := corridorWorld()
    // The B->C and C->B links keep only cells x 200..210 open: the
    // ramp edge narrows to a 11 cell gate.
    for _, spec := range []struct {
        poly int32
        side uint8
    }{{poly: 1, side: SideMaxY}, {poly: 2, side: SideMinY}} {
        poly := &tile.Polys[spec.poly]
        for li := poly.FirstLink; li >= 0; li = tile.Links[li].Next {
            if tile.Links[li].Side == spec.side {
                tile.Links[li].T0 = 200
                tile.Links[li].T1 = 210
            }
        }
    }
    mesh := NewMesh(writeTiles(t, tile))
    route, err := mesh.Route(worldPos(88, 88, 0), worldPos(240, 264, -80),
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)

    // Every waypoint segment crossing the ramp edge (the y=160 cell
    // line) crosses inside the open span x 200..211 (the world span
    // 32768+200*16 .. 32768+211*16).
    edgeY := 32768.0 + 160*16
    spanMinX := 32768.0 + 200*16
    spanMaxX := 32768.0 + 211*16
    crossed := false
    for i := 0; i+1 < len(route.Waypoints); i++ {
        a, b := route.Waypoints[i], route.Waypoints[i+1]
        if (a.Y < edgeY) == (b.Y < edgeY) {
            continue
        }
        crossed = true
        pos := (edgeY - a.Y) / (b.Y - a.Y)
        crossX := a.X + (b.X-a.X)*pos
        require.GreaterOrEqual(t, crossX, spanMinX-1e-6,
            "the crossing left the open span")
        require.LessOrEqual(t, crossX, spanMaxX+1e-6,
            "the crossing left the open span")
    }
    require.True(t, crossed, "the route must cross the ramp edge")
}

// TestRouteMissingMesh pins the missing mesh errors: a position
// without a tile or without a polygon under it answers
// NoNavmeshError, a missing neighbor tile walls the links.
func TestRouteMissingMesh(t *testing.T) {
    dir := t.TempDir()
    mesh := NewMesh(dir)
    _, err := mesh.Route(worldPos(88, 88, 0), worldPos(240, 264, -80),
        DefaultFilter())
    require.Error(t, err)
    var missing *NoNavmeshError
    require.ErrorAs(t, err, &missing)

    // A mesh whose only tile is far away: the position has no polygon.
    mesh = NewMesh(writeTiles(t, corridorWorld()))
    _, err = mesh.Route(Pos{X: 200000, Y: 200000, Z: 0},
        worldPos(240, 264, -80), DefaultFilter())
    require.ErrorAs(t, err, &missing)
}

// TestRouteSamePolygon pins the trivial corridor: the start and the
// end on one polygon answer the two waypoint walk without a search.
func TestRouteSamePolygon(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    route, err := mesh.Route(worldPos(88, 88, 0), worldPos(120, 120, 0),
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Corridor, 1)
    require.Len(t, route.Waypoints, 2)
}

// TestMeshLazyLoadAndLRU pins the tile cache: the lazy loads, the
// failed lookups and the capacity bound.
func TestMeshLazyLoadAndLRU(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    stats := mesh.Stats()
    require.Equal(t, 1, stats.TileFiles)
    require.Equal(t, 0, stats.Loaded)

    tile, err := mesh.Tile(RegionKey{Col: 21, Row: 19})
    require.NoError(t, err)
    require.NotNil(t, tile)
    require.Equal(t, 1, mesh.Stats().Loaded)

    // A missing region answers the absent sentinel (a wall to the
    // link resolution), a failed decode would answer its error.
    tile, err = mesh.Tile(RegionKey{Col: 40, Row: 40})
    require.Nil(t, tile)
    require.ErrorIs(t, err, ErrTileAbsent)

    mesh.SetCacheCapacity(1)
    _, err = mesh.Tile(RegionKey{Col: 21, Row: 19})
    require.NoError(t, err)
    tile, err = mesh.Tile(RegionKey{Col: 40, Row: 40})
    require.Nil(t, tile)
    require.ErrorIs(t, err, ErrTileAbsent)
    require.LessOrEqual(t, mesh.Stats().Loaded, 1)
}

// borderTiles builds two one-polygon tiles whose rectangles meet at
// the region border: 21_19 ends at cell x 2048 (the world line 65536)
// where 22_19 begins, linked through external links.
func borderTiles() (*Tile, *Tile) {
    west := assembleTile(21,
        []rectSpec{{x0: 1900, y0: 0, x1: 2048, y1: 100, h: 0,
            area: AreaGround}},
        []linkSpec{{poly: 0, side: SideMaxX, to: -1, t0: 0, t1: 99}},
        []ExtLink{{Col: 22, Row: 19, Poly: 0}})
    east := assembleTile(22,
        []rectSpec{{x0: 0, y0: 0, x1: 100, y1: 100, h: 0,
            area: AreaGround}},
        []linkSpec{{poly: 0, side: SideMinX, to: -1, t0: 0, t1: 99}},
        []ExtLink{{Col: 21, Row: 19, Poly: 0}})

    return west, east
}

// TestRouteCrossesRegionBorder pins the multi tile runtime: a route
// crosses the region border through the external links, loading the
// neighbor tile on demand.
func TestRouteCrossesRegionBorder(t *testing.T) {
    west, east := borderTiles()
    mesh := NewMesh(writeTiles(t, west, east))
    route, err := mesh.Route(
        Pos{X: 32768 + 1974*16, Y: 32768 + 50*16, Z: 0},
        Pos{X: 65536 + 50*16, Y: 32768 + 50*16, Z: 0},
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Corridor, 2)
    col, row := TileOf(route.Corridor[0])
    require.EqualValues(t, 21, col)
    require.EqualValues(t, 19, row)
    col, row = TileOf(route.Corridor[1])
    require.EqualValues(t, 22, col)
    require.EqualValues(t, 19, row)
    require.Len(t, route.Waypoints, 2)
    require.Equal(t, 2, mesh.Stats().Loaded)

    // The reverse route crosses back.
    route, err = mesh.Route(
        Pos{X: 65536 + 50*16, Y: 32768 + 50*16, Z: 0},
        Pos{X: 32768 + 1974*16, Y: 32768 + 50*16, Z: 0},
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Corridor, 2)
}

// TestResolveLinkMissingTileWalls pins the missing tile semantics at
// the link level: an external link into an absent region resolves to
// the null reference (a wall), so the corridor searches never follow
// it.
func TestResolveLinkMissingTileWalls(t *testing.T) {
    west, _ := borderTiles()
    mesh := NewMesh(writeTiles(t, west))
    tile, err := mesh.Tile(RegionKey{Col: 21, Row: 19})
    require.NoError(t, err)
    require.NotNil(t, tile)
    require.Len(t, tile.Links, 1)
    require.Equal(t, PolyRef(0), mesh.resolveLink(tile, &tile.Links[0]))
}
