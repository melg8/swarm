// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "bufio"
    "os"
    "strconv"
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestNavmeshPairReplay replays the 200 random query pairs the
// research/recast geodata experiment captured on the real 21_19
// region (the C++ Detour navmesh answered all 200 with an average of
// 339 us per findPath+findStraightPath, the hard bridge pair 169 us)
// through the current grid A* engine, so the research report compares
// the two engines on identical endpoints. The pairs file is a copy of
// research/recast/results/query_pairs_21_19.txt; the test skips when
// the geodata directory is absent.
func TestNavmeshPairReplay(t *testing.T) {
    pairs := loadNavmeshPairs(t, "testdata/navmesh_pairs_21_19.txt")
    engine := NewEngine("../../../data/geodata")
    if !engine.Stats().HasData {
        t.Skip("the geodata pack is not present")
    }

    found := 0
    aborted := 0
    var totalDuration int64
    explored := 0
    for _, pair := range pairs {
        result, err := engine.FindPath(
            pair[0], pair[1], DefaultMaxPassableHeight)
        require.NoError(t, err)
        if result.Found {
            found++
        }
        if result.Aborted {
            aborted++
        }
        totalDuration += result.Duration.Nanoseconds()
        explored += result.Explored
    }

    average := int64(0)
    if len(pairs) > 0 {
        average = totalDuration / int64(len(pairs))
    }
    t.Logf("grid engine on the navmesh research pairs: %d/%d found,"+
        " %d aborted, average %d ns (%.1f us) per pair, average %d"+
        " explored nodes",
        found, len(pairs), aborted, average,
        float64(average)/1000.0, explored/len(pairs))

    // The hard pair of the research round: the village dump cell to
    // the water under the bridge column.
    hard := [2]Vec3{
        {X: 45768, Y: 49848, Z: -3056},
        {X: 44920, Y: 50792, Z: -3928},
    }
    result, err := engine.FindPath(
        hard[0], hard[1], DefaultMaxPassableHeight)
    require.NoError(t, err)
    t.Logf("grid engine on the hard bridge pair: found=%t aborted=%t"+
        " %s %d nodes explored %d us %d waypoints",
        result.Found, result.Aborted, result.Duration,
        result.Explored, result.Duration.Microseconds(),
        len(result.Waypoints))
}

// BenchmarkNavmeshPairReplay measures the grid engine per pair cost
// on the research pairs for the report comparison table.
func BenchmarkNavmeshPairReplay(b *testing.B) {
    pairs := loadNavmeshPairsB(b, "testdata/navmesh_pairs_21_19.txt")
    engine := NewEngine("../../../data/geodata")
    if !engine.Stats().HasData {
        b.Skip("the geodata pack is not present")
    }
    b.ResetTimer()
    for i := range b.N {
        pair := pairs[i%len(pairs)]
        result, err := engine.FindPath(
            pair[0], pair[1], DefaultMaxPassableHeight)
        if err != nil || !result.Found {
            b.Fatalf("pair %d not found: %v %t", i, err, result.Found)
        }
    }
}

// loadNavmeshPairs reads the pairs file of the research experiment:
// one "x1 y1 z1 x2 y2 z2" line per pair.
func loadNavmeshPairs(t *testing.T, path string) [][2]Vec3 {
    t.Helper()
    file, err := os.Open(path)
    require.NoError(t, err)
    defer func() { _ = file.Close() }()

    return scanNavmeshPairs(file)
}

// loadNavmeshPairsB is the benchmark variant of loadNavmeshPairs.
func loadNavmeshPairsB(b *testing.B, path string) [][2]Vec3 {
    b.Helper()
    file, err := os.Open(path)
    if err != nil {
        b.Fatalf("open pairs: %v", err)
    }
    defer func() { _ = file.Close() }()

    return scanNavmeshPairs(file)
}

// scanNavmeshPairs parses the whitespace separated pair lines and
// skips the comment header. The file stores the points in the Recast
// axis order of the research experiment (x, height, world-y), so the
// middle field of every triple is the game z.
func scanNavmeshPairs(file *os.File) [][2]Vec3 {
    pairs := make([][2]Vec3, 0, 256)
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
        pairs = append(pairs, [2]Vec3{
            {X: numbers[0], Y: numbers[2], Z: numbers[1]},
            {X: numbers[3], Y: numbers[5], Z: numbers[4]},
        })
    }

    return pairs
}
