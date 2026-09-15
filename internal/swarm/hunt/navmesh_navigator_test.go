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
    "encoding/binary"
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

// huntFlatRegion writes a synthetic flat block geodata region 21_19:
// the land block square (bx, by below landBlocks) stands at landH,
// the rest of the region at bedH - the same world the hunt tiles
// model, in the engine's own file format (the flat block: one byte
// kind zero plus the raw little endian height, all walls open).
func huntFlatRegion(
    t *testing.T, dir string, landBlocks int, landH, bedH int16,
) {
    t.Helper()
    data := make([]byte, 0, 3*256*256)
    word := make([]byte, 2)
    for bx := range 256 {
        for by := range 256 {
            height := bedH
            if bx < landBlocks && by < landBlocks {
                height = landH
            }
            data = append(data, 0)
            binary.LittleEndian.PutUint16(word, uint16(height))
            data = append(data, word...)
        }
    }
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "21_19.l2j"), data, 0o600))
}

// huntWorldOf converts a region local cell into the world position of
// its lower corner (the same anchor the hunt tile rects use).
func huntWorldOf(localCell int) float64 {
    return 32768 + float64(localCell)*16
}

// shoreWorldTile builds the synthetic shore world tile: dry mainland
// A, dry shore band B east of it, water C east of the band - the mesh
// counterpart of the flat block geodata the partial tests write for
// the engine.
func shoreWorldTile(t *testing.T) *navmesh.Tile {
    t.Helper()

    return huntTile(t, 21, 19, []nmRect{
        {x0: 0, y0: 0, x1: 160, y1: 320, h: -3770, area: navmesh.AreaGround},
        {x0: 160, y0: 0, x1: 320, y1: 320, h: -3770, area: navmesh.AreaGround},
        {x0: 320, y0: 0, x1: 480, y1: 320, h: -3800, area: navmesh.AreaWater},
    }, []nmLink{
        {poly: 0, side: navmesh.SideMaxX, to: 1, t0: 0, t1: 319},
        {poly: 1, side: navmesh.SideMinX, to: 0, t0: 0, t1: 319},
        {poly: 1, side: navmesh.SideMaxX, to: 2, t0: 0, t1: 319},
        {poly: 2, side: navmesh.SideMinX, to: 1, t0: 0, t1: 319},
    })
}

// shoreWorldMesh writes the shore world tile into its own directory
// and answers the mesh over it.
func shoreWorldMesh(t *testing.T) *navmesh.Mesh {
    t.Helper()
    dir := t.TempDir()
    data, err := navmesh.EncodeTile(shoreWorldTile(t))
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "21_19.nm"), data, 0o600))

    return navmesh.NewMesh(dir)
}

// TestNavmeshNavigatorPartialServesClosestReachable pins the partial
// round of the dry avoiding form: the destination sits on the water
// polygon the dry filter walls, so the mesh answers the
// closest-reachable corridor and the engine - over the same synthetic
// world - confirms the destination unreachable with its own clean not
// found; the hybrid then serves the partial waypoints through
// Result.Partial instead of the bare abort (the walk toward the shore
// the town legs can make).
func TestNavmeshNavigatorPartialServesClosestReachable(t *testing.T) {
    mesh := shoreWorldMesh(t)
    geodataDir := t.TempDir()
    huntFlatRegion(t, geodataDir, 40, -3770, -3800)

    engine := pathfind.NewEngine(geodataDir)
    navigator := NewNavmeshNavigator(engine, mesh)

    start := pathfind.Vec3{
        X: huntWorldOf(80), Y: huntWorldOf(160), Z: -3770,
    }
    end := pathfind.Vec3{
        X: huntWorldOf(400), Y: huntWorldOf(160), Z: -3800,
    }

    // The premise: the engine over the same world answers the clean
    // not found for the swim-only destination.
    engineResult, err := engine.FindPathApproachDryAvoiding(
        start, end, 150, engine.MaxPassableHeight(), nil)
    require.NoError(t, err)
    require.NotNil(t, engineResult)
    require.False(t, engineResult.Found)

    // The hybrid serves the mesh partial corridor after that
    // confirmation: the walk ends at the closest reachable dry point.
    result, err := navigator.FindPathApproachDryAvoiding(
        start, end, 150, nil)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.False(t, result.Found)
    require.True(t, result.Partial)
    require.NotEmpty(t, result.Waypoints)
    require.GreaterOrEqual(t, len(result.Waypoints), 2)
    // The corridor stays dry: every waypoint stands above the water
    // level, the last one is the shore border of the band B.
    for i, wp := range result.Waypoints {
        require.GreaterOrEqual(t, wp.Z, -3780.0,
            "partial waypoint %d must stay dry", i)
    }
    last := result.Waypoints[len(result.Waypoints)-1]
    require.InDelta(t, huntWorldOf(320), last.X, 1.0)
    require.InDelta(t, huntWorldOf(160), last.Y, 64.0)
    require.Greater(t, result.Length, 3000.0)
}

// TestNavmeshNavigatorPartialDefersToEngineRoute pins the
// can-only-add rule of the partial round: a mesh that models less
// ground than the engine (the C-D link missing - the corridor ends at
// C) answers a partial, but the engine holds the full route, and the
// engine route WINS - the hybrid never trades a found route for a
// partial walk.
func TestNavmeshNavigatorPartialDefersToEngineRoute(t *testing.T) {
    // The broken-chain world: A-B-C linked, D isolated (no C-D link).
    tile := huntTile(t, 21, 19, []nmRect{
        {x0: 0, y0: 0, x1: 160, y1: 320, h: -3770, area: navmesh.AreaGround},
        {x0: 160, y0: 0, x1: 320, y1: 320, h: -3770, area: navmesh.AreaGround},
        {x0: 320, y0: 0, x1: 480, y1: 320, h: -3770, area: navmesh.AreaGround},
        {x0: 480, y0: 0, x1: 640, y1: 320, h: -3770, area: navmesh.AreaGround},
    }, []nmLink{
        {poly: 0, side: navmesh.SideMaxX, to: 1, t0: 0, t1: 319},
        {poly: 1, side: navmesh.SideMinX, to: 0, t0: 0, t1: 319},
        {poly: 1, side: navmesh.SideMaxX, to: 2, t0: 0, t1: 319},
        {poly: 2, side: navmesh.SideMinX, to: 1, t0: 0, t1: 319},
    })
    meshDir := t.TempDir()
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(
        filepath.Join(meshDir, "21_19.nm"), data, 0o600))
    // The engine geodata is all land: the full route exists there.
    geodataDir := t.TempDir()
    huntFlatRegion(t, geodataDir, 256, -3770, -3770)

    mesh := navmesh.NewMesh(meshDir)
    engine := pathfind.NewEngine(geodataDir)
    navigator := NewNavmeshNavigator(engine, mesh)

    start := pathfind.Vec3{
        X: huntWorldOf(80), Y: huntWorldOf(160), Z: -3770,
    }
    end := pathfind.Vec3{
        X: huntWorldOf(560), Y: huntWorldOf(160), Z: -3770,
    }
    result, err := navigator.FindPathApproachDryAvoiding(
        start, end, 150, nil)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.True(t, result.Found)
    require.False(t, result.Partial)
    require.NotEmpty(t, result.Waypoints)
    // The engine route crosses into D - the mesh chain would have
    // ended at the C border (world 32768 + 480*16).
    last := result.Waypoints[len(result.Waypoints)-1]
    require.Greater(t, last.X, 32768+480*16.0)
}

// TestNavmeshNavigatorPartialNeedsEngineVerdict pins the confirmation
// rule: the mesh partial alone never serves - an engine that cannot
// even answer (no geodata under the endpoints) surfaces its error,
// the hybrid never invents a walk out of a mesh corridor without the
// engine verdict.
func TestNavmeshNavigatorPartialNeedsEngineVerdict(t *testing.T) {
    mesh := shoreWorldMesh(t)
    engine := pathfind.NewEngine(t.TempDir())
    navigator := NewNavmeshNavigator(engine, mesh)

    start := pathfind.Vec3{
        X: huntWorldOf(80), Y: huntWorldOf(160), Z: -3770,
    }
    end := pathfind.Vec3{
        X: huntWorldOf(400), Y: huntWorldOf(160), Z: -3800,
    }
    _, err := navigator.FindPathApproachDryAvoiding(start, end, 150, nil)
    require.Error(t, err,
        "the engine without geodata answers the honest error, the "+
            "mesh partial never serves without the engine verdict")
}

// TestNavmeshNavigatorHardPairPartial pins the partial round on the
// REAL elven village pair: the village deck to the water under the
// bridge - the motivating stacked-layer walk of the whole port -
// planned DRY answers the mesh partial corridor to the closest
// reachable dry point once the engine confirms the water destination
// unreachable without a swim. The walk the town legs then make ends
// on the shore instead of aborting on the deck.
func TestNavmeshNavigatorHardPairPartial(t *testing.T) {
    navigator, _ := realNavmeshNavigator(t)

    village := pathfind.Vec3{X: 45768, Y: 49848, Z: -3056}
    water := pathfind.Vec3{X: 44920, Y: 50792, Z: -3928}
    result, err := navigator.FindPathApproachDryAvoiding(
        village, water, 150, nil)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.False(t, result.Found)
    require.True(t, result.Partial)
    require.NotEmpty(t, result.Waypoints)
    // Every waypoint of the partial walk stays dry: the corridor
    // walls the water polygons, the funnel never dips below the
    // C1 water level.
    for i, wp := range result.Waypoints {
        require.GreaterOrEqual(t, wp.Z, -3780.0,
            "partial waypoint %d must stay dry", i)
    }
    // The walk leaves the deck toward the water - the closest
    // reachable point of the dry corridor.
    require.Greater(t, result.Length, 300.0)
    // The engine confirmation is already proven BY the partial
    // serving: Result.Partial only surfaces after the engine answered
    // its own not found for the same query (the hybrid runs the
    // engine internally before serving the mesh corridor) - the
    // production profile of an unreachable dry destination stays the
    // mesh milliseconds plus the one engine flood the round one
    // fallback already paid.
}
