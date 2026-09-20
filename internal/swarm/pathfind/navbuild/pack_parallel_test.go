// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "bytes"
    "fmt"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// packStripWorld synthesizes a 2x2 region block with a walkable cross
// through the middle that reaches the region borders: the internal
// borders of the block must stitch (the strips pair through the pack
// phase B) whichever worker count builds the pack. The cross segments
// touch all four region sides, so the components survive the island
// filter of the build.
func packStripWorld(cx, cy int) []layerSpec {
    const half = regionCellsSide / 2
    const segmentHalf = 8
    if (cx >= half-segmentHalf && cx < half+segmentHalf) ||
        (cy >= half-segmentHalf && cy < half+segmentHalf) {
        return []layerSpec{{h: -3504, nswe: 0x0F}}
    }

    return nil
}

// TestBuildPackParallelDeterminism pins the worker pool: the pack of
// the same synthetic block answers byte identical tiles whether one
// worker or four build it (the parallel phases change nothing but the
// wall clock).
func TestBuildPackParallelDeterminism(t *testing.T) {
    serialDir := t.TempDir()
    serial, err := buildTestPack(t, serialDir, 1)
    require.NoError(t, err)
    parallelDir := t.TempDir()
    parallel, err := buildTestPack(t, parallelDir, 4)
    require.NoError(t, err)

    require.Equal(t, serial.Built, parallel.Built)
    require.Equal(t, serial.Skipped, parallel.Skipped)
    require.Equal(t, serial.Failed, parallel.Failed)
    require.Equal(t, serial.Polys, parallel.Polys)
    require.Equal(t, serial.Links, parallel.Links)
    require.Equal(t, serial.Stitched, parallel.Stitched)

    names, err := filepath.Glob(filepath.Join(serialDir, "*.nm"))
    require.NoError(t, err)
    require.Len(t, names, 4)
    for _, name := range names {
        want, err := os.ReadFile(name)
        require.NoError(t, err)
        got, err := os.ReadFile(
            filepath.Join(parallelDir, filepath.Base(name)))
        require.NoError(t, err)
        require.True(t, bytes.Equal(want, got),
            "tile %s differs between the worker counts",
            filepath.Base(name))
    }
}

// buildTestPack writes the synthetic 2x2 block geodata into a temp
// directory and builds the pack with the given worker count.
func buildTestPack(t *testing.T, outDir string, workers int) (PackStats,
    error,
) {
    t.Helper()
    geodataDir := t.TempDir()
    keys := []navmesh.RegionKey{
        {Col: 21, Row: 19}, {Col: 22, Row: 19},
        {Col: 21, Row: 20}, {Col: 22, Row: 20},
    }
    for _, key := range keys {
        region := writeRegionFile(t, packStripWorld)
        path := filepath.Join(geodataDir,
            fmt.Sprintf("%d_%d.l2j", key.Col, key.Row))
        require.NoError(t, os.WriteFile(path, region, 0o600))
    }
    opts := DefaultOptions()
    opts.Workers = workers

    return BuildPack(geodataDir, outDir, keys, opts, true, nil)
}
