// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package prototype

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// nowNanos is the monotonic clock of the test timings.
func nowNanos() int64 {
	return time.Now().UnixNano()
}

// The tile of the research round: the C++ converter export of the
// 21_19 elven village region (14062 polygons, 525 water). The file
// lives in the research results and travels with the repository as
// the reproducible evidence of the report.
const researchTile = "../../../../../research/recast/results/navmesh_21_19.bin"

// loadResearchTile parses the exported tile or skips the test when
// the research artifact is absent.
func loadResearchTile(t *testing.T) *Tile {
	t.Helper()
	data, err := os.ReadFile(researchTile)
	if err != nil {
		t.Skip("the research navmesh tile is not present")
	}
	tile, err := ParseTile(data)
	require.NoError(t, err)

	return tile
}

// TestParseTile checks the header decode of the exported tile against
// the numbers the C++ experiment printed.
func TestParseTile(t *testing.T) {
	tile := loadResearchTile(t)
	require.Equal(t, int32(14062), tile.Header.PolyCount)
	require.Equal(t, int32(23149), tile.Header.VertCount)
	require.Equal(t, int32(32768), int32(tile.Header.BMin[0]))
	require.Equal(t, int32(65536), int32(tile.Header.BMax[0]))
	// Every polygon must carry a sane area id and vertex count.
	water := 0
	for i := range tile.Polys {
		poly := &tile.Polys[i]
		require.GreaterOrEqual(t, poly.VertCount, 3)
		require.LessOrEqual(t, poly.VertCount, vertsPerPolygon)
		if poly.Area >= 31 && poly.Area < 47 {
			water++
		}
	}
	require.Equal(t, 525, water)
}

// TestFindNearestPolyLayers is the Go repeat of the stacked layer
// disambiguation on the real bridge column: the under the bridge
// point binds to the water polygon, the same x/y at deck height to
// the ground deck polygon.
func TestFindNearestPolyLayers(t *testing.T) {
	tile := loadResearchTile(t)
	query := NewQuery(tile)
	for area := 31; area < 47; area++ {
		query.SetAreaCost(area, 3) // swimming costs three land steps
	}

	under := Point{44920, -3928, 50792}
	deck := Point{44920, -3048, 50792}
	poly, closest, ok := query.FindNearestPoly(under)
	require.True(t, ok)
	require.GreaterOrEqual(t, tile.Polys[poly].Area, 31)
	require.Less(t, tile.Polys[poly].Area, 47)
	require.InDelta(t, -3920, closest[1], 64)

	poly, closest, ok = query.FindNearestPoly(deck)
	require.True(t, ok)
	require.GreaterOrEqual(t, tile.Polys[poly].Area, 47)
	require.InDelta(t, -3036, closest[1], 64)
}

// TestFindPathBridge replays the hard pair of the research round:
// the village dump cell to the water under the bridge, the route the
// grid engine answers in 5.2 seconds and 500k node expansions.
func TestFindPathBridge(t *testing.T) {
	tile := loadResearchTile(t)
	query := NewQuery(tile)
	for area := 31; area < 47; area++ {
		query.SetAreaCost(area, 3)
	}

	village := Point{45768, -3056, 49848}
	under := Point{44920, -3928, 50792}
	startPoly, startPos, ok := query.FindNearestPoly(village)
	require.True(t, ok)
	endPoly, endPos, ok := query.FindNearestPoly(under)
	require.True(t, ok)

	path := query.FindPath(startPoly, endPoly, startPos, endPos)
	require.NotNil(t, path)
	require.Greater(t, len(path), 8)
	require.Equal(t, startPoly, path[0])
	require.Equal(t, endPoly, path[len(path)-1])

	corners := query.Corners(path, startPos, endPos)
	require.NotEmpty(t, corners)
	// The route must cross the water: some corner sits below the C1
	// water level.
	wet := false
	for _, corner := range corners {
		if corner[1] < -3780 {
			wet = true
		}
	}
	require.True(t, wet)
}

// TestPairsReplay replays the 200 random research pairs through the
// Go query engine and reports the per pair cost next to the C++
// Detour numbers (339 us average) and the grid engine numbers (2.9 s
// average, 39 pairs aborted at the expansion cap).
func TestPairsReplay(t *testing.T) {
	tile := loadResearchTile(t)
	query := NewQuery(tile)
	for area := 31; area < 47; area++ {
		query.SetAreaCost(area, 3)
	}
	pairs := loadResearchPairs(t)

	found := 0
	var totalNs int64
	for _, pair := range pairs {
		startPoly, startPos, ok := query.FindNearestPoly(pair[0])
		if !ok {
			continue
		}
		endPoly, endPos, ok := query.FindNearestPoly(pair[1])
		if !ok {
			continue
		}
		began := nowNanos()
		path := query.FindPath(startPoly, endPoly, startPos, endPos)
		totalNs += nowNanos() - began
		if path != nil {
			found++
		}
	}
	average := float64(totalNs) / float64(len(pairs)) / 1000.0
	t.Logf("go navmesh on the research pairs: %d/%d routable,"+
		" average %.1f us per FindPath (%d polys in the tile)",
		found, len(pairs), average, len(tile.Polys))
	// The prototype resolves the nearest polygon with a height
	// approximation instead of the detail mesh, so a minority of the
	// pairs binds to a stacked layer the C++ search did not pick.
	require.Greater(t, found, len(pairs)*4/5)
}

// loadResearchPairs reads the pairs file the experiment captured.
func loadResearchPairs(t *testing.T) [][2]Point {
	t.Helper()
	file, err := os.Open(
		"../testdata/navmesh_pairs_21_19.txt")
	if err != nil {
		t.Skip("the research pairs file is not present")
	}
	defer func() { _ = file.Close() }()

	pairs := make([][2]Point, 0, 256)
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
		for i, field := range fields {
			value, err := strconv.ParseFloat(field, 64)
			if err != nil {
				continue
			}
			numbers[i] = value
		}
		pairs = append(pairs, [2]Point{
			{numbers[0], numbers[1], numbers[2]},
			{numbers[3], numbers[4], numbers[5]},
		})
	}

	return pairs
}
