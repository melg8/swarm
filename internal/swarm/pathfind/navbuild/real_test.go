// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "bufio"
    "fmt"
    "os"
    "strconv"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// realRegionData reads one region file of the shipped geodata pack.
func realRegionData(t *testing.T, col, row int16) []byte {
    t.Helper()
    data, err := os.ReadFile(fmt.Sprintf(
        "../../../../data/geodata/%d_%d.l2j", col, row))
    if err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }

    return data
}

// buildRealRegion builds one real region and returns the build.
func buildRealRegion(t *testing.T) *RegionBuild {
    t.Helper()
    col, row := int16(21), int16(19)
    started := time.Now()
    build, err := BuildRegion(realRegionData(t, col, row), col, row,
        DefaultOptions())
    require.NoError(t, err)
    t.Logf("region %d_%d built in %s: %d layers, %d sheets"+
        " (%d islands dropped), %d polys (%d water), %d links,"+
        " %d blocked pairs",
        col, row, time.Since(started), build.Stats.Layers,
        build.Stats.Sheets, build.Stats.DroppedSheets,
        build.Stats.Polys, build.Stats.WaterPolys, build.Stats.Links,
        build.Stats.NSWEBlockedPairs)

    return build
}

// TestRealRegionBuildHealth builds the real elven village region and
// audits the mesh health: the polygon bounds stay inside the region,
// the link chains are consistent, every internal link has its
// symmetric counterpart and the wire encode derives the index.
func TestRealRegionBuildHealth(t *testing.T) {
    build := buildRealRegion(t)
    tile := build.Tile

    for i := range tile.Polys {
        poly := &tile.Polys[i]
        require.Greater(t, poly.X1, poly.X0, "poly %d", i)
        require.Greater(t, poly.Y1, poly.Y0, "poly %d", i)
        require.GreaterOrEqual(t, poly.X0, int32(0), "poly %d", i)
        require.GreaterOrEqual(t, poly.Y0, int32(0), "poly %d", i)
        require.LessOrEqual(t, poly.X1, int32(2048), "poly %d", i)
        require.LessOrEqual(t, poly.Y1, int32(2048), "poly %d", i)
    }
    // The link graph audit: every internal link of A points at a
    // polygon whose own chain points back (the corridor searches are
    // symmetric by construction).
    symmetric := 0
    oneWay := 0
    for p := range tile.Polys {
        for li := tile.Polys[p].FirstLink; li >= 0; li = tile.Links[li].Next {
            link := &tile.Links[li]
            if link.To < 0 {
                continue
            }
            back := false
            for lj := tile.Polys[link.To].FirstLink; lj >= 0; lj = tile.Links[lj].Next {
                if tile.Links[lj].To == int32(p) &&
                    tile.Links[lj].Side != link.Side {
                    back = true

                    break
                }
            }
            if back {
                symmetric++
            } else {
                oneWay++
            }
        }
    }
    require.Zero(t, oneWay, "the mesh must hold zero one-way links")
    require.Positive(t, symmetric)
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    encoded, err := navmesh.DecodeTile(data)
    require.NoError(t, err)
    require.NotNil(t, encoded.Grid, "the v3 encode derives the grid")
}

// TestRealRegionBridgePair runs the hard case of the research round
// on the real mesh: the route from the elven village dump cell to the
// water under the bridge - the same x/y over a walkable deck column
// but the z hundreds of units below it.
func TestRealRegionBridgePair(t *testing.T) {
    build := buildRealRegion(t)
    mesh := writeAndLoad(t, build.Tile)
    village := navmesh.Pos{X: 45768, Y: 49848, Z: -3056}
    under := navmesh.Pos{X: 44920, Y: 50792, Z: -3928}

    // The disambiguation: the village point binds onto the village
    // ground, the under-bridge point onto the water.
    ref, pos, ok := mesh.FindNearestPoly(under)
    require.True(t, ok)
    _, poly := mesh.PolyOf(ref)
    require.NotNil(t, poly)
    require.Equal(t, navmesh.AreaWater, poly.Area)
    require.InDelta(t, -3928, pos.Z, 200,
        "the under-bridge point must bind onto the water surface")
    ref, _, ok = mesh.FindNearestPoly(village)
    require.True(t, ok)
    _, poly = mesh.PolyOf(ref)
    require.NotNil(t, poly)
    require.Equal(t, navmesh.AreaGround, poly.Area)

    // The full swim route.
    started := time.Now()
    route, err := mesh.Route(village, under, navmesh.DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found, "the swim route must exist")
    wet := false
    for _, wp := range route.Waypoints[1:] {
        if wp.Z < -3780 {
            wet = true
        }
    }
    require.True(t, wet, "the route must swim")
    t.Logf("the hard pair: %d polys, %d waypoints, %d us,"+
        " %d explored",
        len(route.Corridor), len(route.Waypoints),
        time.Since(started).Microseconds(), route.Explored)

    // The reverse escape: from the water back onto the dry ground.
    escape, err := mesh.WaterEscape(under)
    require.NoError(t, err)
    require.True(t, escape.Found, "the water escape must exist")
}

// TestRealRegionPairReplay replays the 200 random region wide pairs
// the research round captured. The C++ Detour reported "200/200
// routable at 339 us" - but dtStatusSucceed counts DT_PARTIAL_RESULT
// as success, so the C++ number mixed full corridors with
// closest-reachable partials. This replay classifies honestly: a pair
// counts as answered when the mesh returns either the full route or
// the partial closest-reachable corridor (the Detour behavior), the
// split between them is the report number.
func TestRealRegionPairReplay(t *testing.T) {
    build := buildRealRegion(t)
    mesh := writeAndLoad(t, build.Tile)
    pairs := loadRealPairs(t)
    require.NotEmpty(t, pairs)

    found, partial, isolated := 0, 0, 0
    var total time.Duration
    for _, pair := range pairs {
        started := time.Now()
        route, err := mesh.Route(pair[0], pair[1], navmesh.DefaultFilter())
        total += time.Since(started)
        require.NoError(t, err)
        switch {
        case route.Found:
            found++
        case route.Partial:
            partial++
        default:
            isolated++
        }
    }
    answered := found + partial
    average := total / time.Duration(len(pairs))
    t.Logf("the production mesh on the research pairs: %d full +"+
        " %d partial + %d isolated = %d/%d answered, %d us average",
        found, partial, isolated, answered, len(pairs),
        average.Microseconds())
    // The 11 isolated pairs stand on surfaces no legal walk leaves:
    // the 8 islands the C++ audit reported as separate components
    // plus 3 pairs the wall-honest rectangle decomposition unmasked
    // (their corridors used to tunnel through the walls a single
    // polygon swallowed). The grid engine - the reachability
    // authority - answers every one of the 11 with its own clean not
    // found, so the honest answer keeps them separate.
    require.GreaterOrEqual(t, answered, len(pairs)-15,
        "the mesh must answer the research pairs the way Detour did")
    require.Greater(t, found, 100,
        "the majority must be full corridors")
}

// loadRealPairs reads the research pairs file (x, height, world-y
// triples per endpoint, the Recast axis order of the capture).
func loadRealPairs(t *testing.T) [][2]navmesh.Pos {
    t.Helper()
    file, err := os.Open(
        "../testdata/navmesh_pairs_21_19.txt")
    if err != nil {
        t.Skipf("the pairs file is not present: %v", err)
    }
    defer func() { _ = file.Close() }()

    pairs := make([][2]navmesh.Pos, 0, 256)
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        fields := strings.Fields(line)
        if len(fields) != 6 {
            continue
        }
        numbers := make([]float64, 6)
        valid := true
        for i, field := range fields {
            value, err := strconv.ParseFloat(field, 64)
            if err != nil {
                valid = false

                break
            }
            numbers[i] = value
        }
        if !valid {
            continue
        }
        pairs = append(pairs, [2]navmesh.Pos{
            {X: numbers[0], Y: numbers[2], Z: numbers[1]},
            {X: numbers[3], Y: numbers[5], Z: numbers[4]},
        })
    }
    require.NoError(t, scanner.Err())

    return pairs
}
