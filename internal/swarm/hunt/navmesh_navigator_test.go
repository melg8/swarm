// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The tests of the hybrid navigator (docs/navmesh.md): the long
// routes serve from the mesh, every answer the mesh cannot serve
// falls back to the grid engine, and the validation layer (the
// clicks, the sight lines, the water rasters, the deck heights)
// never leaves the engine.

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navbuild"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// nmRect is one rectangle polygon of the synthetic hunt tile.
type nmRect struct {
    x0, y0, x1, y1 int32
    h              int16
    area           uint8
}

// nmLink is one link of the synthetic hunt tile.
type nmLink struct {
    poly   int32
    side   uint8
    to     int32
    t0, t1 int32
}

// huntTile assembles an in-memory navigation tile from rectangle and
// link specs and roundtrips it through the wire format - the same
// discipline the navmesh package's own tests use, built here from the
// exported tile API (the hunt package cannot reach the package
// private fixtures).
func huntTile(
    t *testing.T, col, row int16, rects []nmRect, links []nmLink,
) *navmesh.Tile {
    t.Helper()
    tile := &navmesh.Tile{
        Col:      col,
        Row:      row,
        Climb:    40,
        Polys:    make([]navmesh.Poly, len(rects)),
        Links:    make([]navmesh.Link, len(links)),
        ExtLinks: nil,
        BVTree:   nil,
    }
    for i, rect := range rects {
        tile.Polys[i] = navmesh.Poly{
            X0: rect.x0, Y0: rect.y0, X1: rect.x1, Y1: rect.y1,
            H00: rect.h, H10: rect.h, H01: rect.h, H11: rect.h,
            FirstLink: -1, Area: rect.area,
        }
    }
    for i, spec := range links {
        tile.Links[i] = navmesh.Link{
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
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    decoded, err := navmesh.DecodeTile(data)
    require.NoError(t, err)

    return decoded
}

// TestNavmeshNavigatorServesMeshRoutes pins the happy path of the
// hybrid: the mesh holds the corridor, the answer carries the funnel
// waypoints of the mesh surface (the ramp height, the swim pricing)
// and the grid engine - here an engine without any geodata - is never
// asked.
func TestNavmeshNavigatorServesMeshRoutes(t *testing.T) {
    // The corridor world: mainland A, deck B east, ramp C north of B,
    // water D north of the ramp.
    tile := huntTile(t, 21, 19, []nmRect{
        {x0: 0, y0: 0, x1: 160, y1: 160, h: 0, area: navmesh.AreaGround},
        {x0: 160, y0: 0, x1: 320, y1: 160, h: 0, area: navmesh.AreaGround},
        {x0: 160, y0: 160, x1: 320, y1: 208, h: -40, area: navmesh.AreaGround},
        {x0: 160, y0: 208, x1: 320, y1: 320, h: -80, area: navmesh.AreaWater},
    }, []nmLink{
        {poly: 0, side: navmesh.SideMaxX, to: 1, t0: 0, t1: 159},
        {poly: 1, side: navmesh.SideMinX, to: 0, t0: 0, t1: 159},
        {poly: 1, side: navmesh.SideMaxY, to: 2, t0: 160, t1: 319},
        {poly: 2, side: navmesh.SideMinY, to: 1, t0: 160, t1: 319},
        {poly: 2, side: navmesh.SideMaxY, to: 3, t0: 160, t1: 319},
        {poly: 3, side: navmesh.SideMinY, to: 2, t0: 160, t1: 319},
    })
    dir := t.TempDir()
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "21_19.nm"), data, 0o600))

    mesh := navmesh.NewMesh(dir)
    engine := pathfind.NewEngine(t.TempDir())
    navigator := NewNavmeshNavigator(engine, mesh)

    start := pathfind.Vec3{X: 32768 + 8*16, Y: 32768 + 8*16, Z: 0}
    end := pathfind.Vec3{X: 32768 + 240*16, Y: 32768 + 264*16, Z: -80}
    result, err := navigator.FindPathApproach(start, end, 150)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.True(t, result.Found)
    require.NotEmpty(t, result.Waypoints)
    require.Greater(t, len(result.Waypoints), 2)
    // The route starts on the mainland surface and ends on the water
    // surface of the mesh.
    require.InDelta(t, 0, result.Waypoints[0].Z, 1e-9)
    require.InDelta(t, -80, result.Waypoints[len(result.Waypoints)-1].Z,
        1e-9)
    require.Greater(t, result.Length, 3000.0)

    // The water escape of a start standing on the water polygon also
    // serves from the mesh.
    waterStart := pathfind.Vec3{
        X: 32768 + 240*16, Y: 32768 + 264*16, Z: -80,
    }
    escape, err := navigator.FindWaterEscape(waterStart)
    require.NoError(t, err)
    require.NotNil(t, escape)
    require.True(t, escape.Found)
    require.NotEmpty(t, escape.Waypoints)
    // The escape ends ashore: the final waypoint stands above the
    // water level of the synthetic world (-80 is the water bed).
    require.Greater(t, escape.Waypoints[len(escape.Waypoints)-1].Z, -80.0)
}

// TestNavmeshNavigatorFallsBackToEngine pins the fallback rule: a
// query the mesh cannot serve (no tile under the endpoints at all)
// hands the question to the grid engine - here one without geodata,
// so the honest answer is the engine error, never a silent miss.
func TestNavmeshNavigatorFallsBackToEngine(t *testing.T) {
    mesh := navmesh.NewMesh(t.TempDir())
    engine := pathfind.NewEngine(t.TempDir())
    navigator := NewNavmeshNavigator(engine, mesh)

    start := pathfind.Vec3{X: 45000, Y: 50000, Z: -3000}
    end := pathfind.Vec3{X: 46000, Y: 51000, Z: -3000}
    _, err := navigator.FindPathApproach(start, end, 150)
    require.Error(t, err)
    _, err = navigator.FindPath(start, end)
    require.Error(t, err)
    _, err = navigator.FindWaterEscape(start)
    require.Error(t, err)
}

// TestNavmeshNavigatorBansReachMesh pins the recovery ban flow: the
// frozen corridor bans of the hunt loop wall the mesh corridor search
// (the only lane banned -> no mesh route -> the engine fallback
// surfaces), so the deterministic mesh planner detours instead of
// reproducing the frozen corridor.
func TestNavmeshNavigatorBansReachMesh(t *testing.T) {
    // The two-lane world: mainland M, north lane N, south lane S, far
    // mainland E.
    tile := huntTile(t, 21, 19, []nmRect{
        {x0: 0, y0: 0, x1: 160, y1: 320, h: 0, area: navmesh.AreaGround},
        {x0: 160, y0: 0, x1: 320, y1: 160, h: 0, area: navmesh.AreaGround},
        {x0: 160, y0: 160, x1: 320, y1: 320, h: 0, area: navmesh.AreaGround},
        {x0: 320, y0: 0, x1: 480, y1: 320, h: 0, area: navmesh.AreaGround},
    }, []nmLink{
        {poly: 0, side: navmesh.SideMaxX, to: 1, t0: 0, t1: 159},
        {poly: 1, side: navmesh.SideMinX, to: 0, t0: 0, t1: 159},
        {poly: 0, side: navmesh.SideMaxX, to: 2, t0: 160, t1: 319},
        {poly: 2, side: navmesh.SideMinX, to: 0, t0: 160, t1: 319},
        {poly: 1, side: navmesh.SideMaxX, to: 3, t0: 0, t1: 159},
        {poly: 3, side: navmesh.SideMinX, to: 1, t0: 0, t1: 159},
        {poly: 2, side: navmesh.SideMaxX, to: 3, t0: 160, t1: 319},
        {poly: 3, side: navmesh.SideMinX, to: 2, t0: 160, t1: 319},
    })
    dir := t.TempDir()
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "21_19.nm"), data, 0o600))

    mesh := navmesh.NewMesh(dir)
    engine := pathfind.NewEngine(t.TempDir())
    navigator := NewNavmeshNavigator(engine, mesh)

    start := pathfind.Vec3{X: 32768 + 80*16, Y: 32768 + 160*16, Z: 0}
    end := pathfind.Vec3{X: 32768 + 400*16, Y: 32768 + 160*16, Z: 0}

    // Without the ban the mesh serves the east walk.
    result, err := navigator.FindPathApproachDryAvoiding(start, end, 150,
        nil)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.True(t, result.Found)

    // A ban covering BOTH lanes (the whole east border of the
    // mainland) seals the mesh: the fallback engine has no geodata
    // and answers the honest error - without the ban reaching the
    // mesh search the mesh would have served the route and no error
    // could surface.
    ban := pathfind.AvoidArea{
        Center: pathfind.Vec3{
            X: 32768 + 160*16, Y: 32768 + 160*16, Z: 0,
        },
        Radius: 600,
    }
    _, err = navigator.FindPathApproachDryAvoiding(start, end, 150,
        []pathfind.AvoidArea{ban})
    require.Error(t, err)
}

// realNavmeshNavigator builds the hybrid over the real elven village
// region: the tile straight from navbuild.BuildRegion of the shipped
// geodata, the grid engine over the same directory. The test skips
// itself without the geodata pack.
func realNavmeshNavigator(
    t *testing.T,
) (navmeshNavigator, *pathfind.Engine) {
    t.Helper()
    geodata, err := os.ReadFile(realGeodataRegionFile)
    if err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    build, err := navbuild.BuildRegion(geodata, 21, 19,
        navbuild.DefaultOptions())
    require.NoError(t, err)
    dir := t.TempDir()
    data, err := navmesh.EncodeTile(build.Tile)
    require.NoError(t, err)
    //nolint:gosec // the tile bytes come from the freshly built region
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "21_19.nm"), data, 0o600))
    mesh := navmesh.NewMesh(dir)
    engine := pathfind.NewEngine(realGeodataDir)

    return navmeshNavigator{engine: engine, mesh: mesh}, engine
}

// realGeodataDir is the shipped geodata pack of the repository
// (see the navbuild real tests for the sibling pattern).
const realGeodataDir = "../../../data/geodata"

// realGeodataRegionFile is the elven village region file of the pack.
const realGeodataRegionFile = realGeodataDir + "/21_19.l2j"

// TestNavmeshNavigatorHardPair pins the live integration acceptance
// pair on the real elven village mesh: the village deck to the water
// under the bridge - the stacked-deck walk the grid engine needs 5.17
// seconds and 500k nodes for - answers from the mesh in one
// FindPathApproach call, with the waypoints descending from the deck
// height onto the water surface.
func TestNavmeshNavigatorHardPair(t *testing.T) {
    navigator, _ := realNavmeshNavigator(t)

    village := pathfind.Vec3{X: 45768, Y: 49848, Z: -3056}
    water := pathfind.Vec3{X: 44920, Y: 50792, Z: -3928}
    result, err := navigator.FindPathApproach(village, water, 150)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.True(t, result.Found)
    require.NotEmpty(t, result.Waypoints)
    // The route starts on the village deck and ends under the
    // bridge: the stacked layers disambiguate through the mesh.
    require.InDelta(t, -3056, result.Waypoints[0].Z, 300)
    require.InDelta(t, -3928, result.Waypoints[len(result.Waypoints)-1].Z,
        300)
    require.Greater(t, result.Length, 1000.0)
}

// TestNavmeshNavigatorValidationStaysOnEngine pins the validation
// layer contract on the real geodata: with the mesh present, every
// validation answer of the hybrid is the engine answer - the click
// guard, the water rasters and the deck heights never consult the
// mesh.
func TestNavmeshNavigatorValidationStaysOnEngine(t *testing.T) {
    hybrid, engine := realNavmeshNavigator(t)

    village := pathfind.Vec3{X: 45768, Y: 49848, Z: -3056}
    water := pathfind.Vec3{X: 44920, Y: 50792, Z: -3928}

    validated, ok := hybrid.ValidateClick(village, water)
    engineValidated, engineOK := engine.ValidateClick(village, water)
    require.Equal(t, engineOK, ok)
    require.Equal(t, engineValidated, validated)

    crossed, err := hybrid.WaterCrossed(village, water)
    engineCrossed, engineErr := engine.WaterCrossed(village, water)
    require.Equal(t, engineErr, err)
    require.Equal(t, engineCrossed, crossed)

    height, err := hybrid.ClosestHeight(45768, 49848, -3056)
    engineHeight, engineErr := engine.ClosestHeight(45768, 49848, -3056)
    require.Equal(t, engineErr, err)
    require.Equal(t, engineHeight, height)
    require.NoError(t, err)
}
