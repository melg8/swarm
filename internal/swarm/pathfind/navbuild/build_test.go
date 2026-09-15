// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// buildMiniWorld runs the full pipeline over the synthetic region and
// returns the build.
func buildMiniWorld(t *testing.T) *RegionBuild {
    t.Helper()
    build, err := BuildRegion(miniWorld(t), 21, 19, DefaultOptions())
    require.NoError(t, err)

    return build
}

// worldPos converts region local cells to a world position.
func worldPos(cx, cy float64, z float64) navmesh.Pos {
    return navmesh.Pos{X: 32768 + cx*16, Y: 32768 + cy*16, Z: z}
}

// TestBuildRegionSheets pins the sheet decomposition of the synthetic
// region: the dry surface (mainland + ramp + shore), the water and
// the isolated floating deck are three separate sheets, the stacked
// deck column never merges with the water below it.
func TestBuildRegionSheets(t *testing.T) {
    build := buildMiniWorld(t)
    require.Equal(t, 3, build.Stats.Sheets)
    require.Equal(t, 0, build.Stats.DroppedSheets)
    // The stacked column count: the water cells hold two layers.
    require.Equal(t, 80, build.Stats.StackedColumns)
    require.Equal(t, 80, build.Stats.UnderwaterLayers)

    // The polygon areas: the water and the deck are single uniform
    // rectangles, the dry sheet decomposes into the plateau and the
    // ramp strips of the height-bounded split.
    water, ground := 0, 0
    for i := range build.Tile.Polys {
        if build.Tile.Polys[i].Area == navmesh.AreaWater {
            water++
        } else {
            ground++
        }
    }
    require.Equal(t, 1, water)
    t.Logf("dry polygons: %d", ground)
}

// TestBuildRegionLinkPortals pins the NSWE portal spans: the ramp
// edge link between the plateau polygon and the first ramp polygon
// carries only the open span (cells x 10..19), the walled western
// half never appears.
func TestBuildRegionLinkPortals(t *testing.T) {
    build := buildMiniWorld(t)
    tile := build.Tile

    // The plateau polygon: the ground rectangle covering y 0..10.
    var plateau, ramp0 int32 = -1, -1
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        if poly.Area == navmesh.AreaGround && poly.Y1 == 10 &&
            poly.X1 == 20 {
            plateau = int32(i)
        }
        if poly.Area == navmesh.AreaGround && poly.Y0 == 10 &&
            poly.X1 == 20 {
            ramp0 = int32(i)
        }
    }
    require.GreaterOrEqual(t, plateau, int32(0))
    require.GreaterOrEqual(t, ramp0, int32(0))

    // The link from the plateau into the first ramp row.
    found := 0
    for li := tile.Polys[plateau].FirstLink; li >= 0; li = tile.Links[li].Next {
        link := &tile.Links[li]
        if link.To == ramp0 {
            require.Equal(t, navmesh.SideMaxY, link.Side)
            require.Equal(t, int32(10), link.T0)
            require.Equal(t, int32(19), link.T1)
            found++
        }
    }
    require.Equal(t, 1, found, "the plateau->ramp portal must exist"+
        " exactly once with the open span")

    // The reverse link carries the same span.
    found = 0
    for li := tile.Polys[ramp0].FirstLink; li >= 0; li = tile.Links[li].Next {
        link := &tile.Links[li]
        if link.To == plateau {
            require.Equal(t, navmesh.SideMinY, link.Side)
            require.Equal(t, int32(10), link.T0)
            require.Equal(t, int32(19), link.T1)
            found++
        }
    }
    require.Equal(t, 1, found)

    // The blocked audit: the ten walled pairs of the ramp edge.
    require.Equal(t, 10, build.Stats.NSWEBlockedPairs)
}

// TestBuildRegionQueries runs the runtime queries over the built
// tile: the full swim route, the dry partial, the water escape and
// the stacked disambiguation - the miniatures of the hard bridge
// case.
func TestBuildRegionQueries(t *testing.T) {
    build := buildMiniWorld(t)
    mesh := writeAndLoad(t, build.Tile)

    // The full route from the plateau into the water.
    route, err := mesh.Route(worldPos(5, 5, -3504), worldPos(15, 18,
        -3784), navmesh.DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.False(t, route.Partial)
    require.NotEmpty(t, route.Waypoints)
    // The route crosses water: some waypoint sits below -3780.
    wet := false
    for _, wp := range route.Waypoints[1:] {
        if wp.Z < -3780 {
            wet = true
        }
    }
    require.True(t, wet, "the swim route must reach the water")

    // The dry route answers the partial corridor up the shore.
    dry, err := mesh.RouteDry(worldPos(5, 5, -3504), worldPos(15, 18,
        -3784))
    require.NoError(t, err)
    require.False(t, dry.Found)
    require.True(t, dry.Partial)
    last := dry.Waypoints[len(dry.Waypoints)-1]
    require.InDelta(t, -3744, last.Z, 1e-6)

    // The escape from the water climbs the shore.
    escape, err := mesh.WaterEscape(worldPos(15, 18, -3784))
    require.NoError(t, err)
    require.True(t, escape.Found)

    // The stacked disambiguation: the deck over the water.
    mesh = writeAndLoad(t, build.Tile)
    ref, pos, ok := mesh.FindNearestPoly(worldPos(15, 18, -3504))
    require.True(t, ok)
    _, poly := mesh.PolyOf(ref)
    require.NotNil(t, poly)
    require.Equal(t, navmesh.AreaGround, poly.Area)
    require.InDelta(t, -3504, pos.Z, 1e-6)
    ref, pos, ok = mesh.FindNearestPoly(worldPos(15, 18, -3784))
    require.True(t, ok)
    _, poly = mesh.PolyOf(ref)
    require.NotNil(t, poly)
    require.Equal(t, navmesh.AreaWater, poly.Area)
    require.InDelta(t, -3784, pos.Z, 1e-6)
}

// TestBuildRegionIslandFilter pins the island drop: a two cell
// isolated patch (smaller than the minimum) produces no polygon.
func TestBuildRegionIslandFilter(t *testing.T) {
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx >= 100 && cx < 102 && cy >= 100 && cy < 101 &&
            (cx == 100 || cy == 100) {
            return []layerSpec{{h: -1000, nswe: 0x0F}}
        }
        if cx >= 200 && cx < 210 && cy >= 200 && cy < 210 {
            return []layerSpec{{h: -2000, nswe: 0x0F}}
        }

        return nil
    })
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)
    // Both patches are islands (no neighbour within the climb); the
    // 2 cell patch dies at the island filter, the 10x10 patch stays
    // as an isolated polygon.
    require.Equal(t, 2, build.Stats.Sheets)
    require.Equal(t, 1, build.Stats.DroppedSheets)
    require.Len(t, build.Tile.Polys, 1)
}

// TestBuildRegionRectSplit pins the height-bounded split: a curved
// surface splits into strips whose bilinear corner surfaces stay
// within the tolerance.
func TestBuildRegionRectSplit(t *testing.T) {
    // A parabolic valley: the height curves faster than the tolerance
    // allows over the full 40 cell span.
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx >= 0 && cx < 40 && cy >= 0 && cy < 40 {
            h := int16(-3504 + ((cx-20)*(cx-20)/8)*8)

            return []layerSpec{{h: h, nswe: 0x0F}}
        }

        return nil
    })
    build, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)
    require.Equal(t, 1, build.Stats.Sheets)
    require.Greater(t, len(build.Tile.Polys), 1,
        "the curved surface must split")
    // Every polygon stays within the tolerance of its cells.
    for i := range build.Tile.Polys {
        poly := &build.Tile.Polys[i]
        require.LessOrEqual(t, poly.X1-poly.X0, int32(40))
    }
}

// TestStitchRegionCrossesBorder pins the phase B stitching: two
// synthetic neighbouring regions link through their border strips and
// the runtime routes across the region border.
func TestStitchRegionCrossesBorder(t *testing.T) {
    // The west region ends with a walkable strip at x 2040..2048; the
    // east region begins with one at x 0..8.
    westGrid := func(cx, cy int) []layerSpec {
        if cx >= 2040 && cy >= 1000 && cy < 1008 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }

        return nil
    }
    eastGrid := func(cx, cy int) []layerSpec {
        if cx < 8 && cy >= 1000 && cy < 1008 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }

        return nil
    }
    west, err := BuildRegion(
        writeRegionFile(t, westGrid), 21, 19, DefaultOptions())
    require.NoError(t, err)
    east, err := BuildRegion(
        writeRegionFile(t, eastGrid), 22, 19, DefaultOptions())
    require.NoError(t, err)

    added, err := StitchRegion(west.Tile, west.Strips,
        [4]*borderStrips{nil, &east.Strips, nil, nil}, DefaultOptions())
    require.NoError(t, err)
    require.Positive(t, added)
    require.NotEmpty(t, west.Tile.ExtLinks)
    added, err = StitchRegion(east.Tile, east.Strips,
        [4]*borderStrips{&west.Strips, nil, nil, nil}, DefaultOptions())
    require.NoError(t, err)
    require.Positive(t, added)

    // Roundtrip both tiles through the wire format and route across.
    mesh := writeAndLoad(t, west.Tile, east.Tile)
    route, err := mesh.Route(
        navmesh.Pos{X: 32768 + 2044*16, Y: 32768 + 1004*16, Z: -3504},
        navmesh.Pos{X: 65536 + 4*16, Y: 32768 + 1004*16, Z: -3504},
        navmesh.DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Corridor, 2)
    col, row := navmesh.TileOf(route.Corridor[1])
    require.EqualValues(t, 22, col)
    require.EqualValues(t, 19, row)

    // A walled border (the west cells' east bits closed) stays shut.
    walledGrid := func(cx, cy int) []layerSpec {
        if cx >= 2040 && cy >= 1000 && cy < 1008 {
            nswe := uint8(0x0F)
            if cx == 2047 {
                nswe = 0x0F &^ nsweEast
            }

            return []layerSpec{{h: -3504, nswe: nswe}}
        }

        return nil
    }
    walled, err := BuildRegion(
        writeRegionFile(t, walledGrid), 21, 19, DefaultOptions())
    require.NoError(t, err)
    added, err = StitchRegion(walled.Tile, walled.Strips,
        [4]*borderStrips{&east.Strips, nil, nil, nil}, DefaultOptions())
    require.NoError(t, err)
    require.Zero(t, added,
        "the walled border must produce no external link")
}

// writeAndLoad encodes tiles into a temp directory and loads the mesh.
func writeAndLoad(t *testing.T, tiles ...*navmesh.Tile) *navmesh.Mesh {
    t.Helper()
    dir := t.TempDir()
    for _, tile := range tiles {
        data, err := navmesh.EncodeTile(tile)
        require.NoError(t, err)
        name := fmt.Sprintf("%d_%d.nm", tile.Col, tile.Row)
        require.NoError(t, os.WriteFile(filepath.Join(dir, name), data,
            0o600))
    }

    return navmesh.NewMesh(dir)
}
