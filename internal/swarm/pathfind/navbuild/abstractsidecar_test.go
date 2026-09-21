// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "bytes"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// TestAbstractSidecarWritesCompressed pins the sidecar compression
// round of issue #11: the pack sidecar pass wraps the abstract bytes
// into the zstd frame (the plain 64 byte edge records are mostly zero
// and repeated fields - the 164 region pack measured 405.9 MB plain
// against 72.6 MB framed) and the frame decodes back to the exact
// abstract the tile scan builds.
func TestAbstractSidecarWritesCompressed(t *testing.T) {
    dir := t.TempDir()
    stats, err := buildTestPack(t, dir, 2)
    require.NoError(t, err)
    require.Equal(t, 4, stats.Built)

    key := navmesh.RegionKey{Col: 21, Row: 19}
    absStats, err := WriteAbstractSidecars(dir,
        []navmesh.RegionKey{key}, 1, nil)
    require.NoError(t, err)
    require.Equal(t, 1, absStats.Written)

    // The written sidecar carries the zstd frame.
    raw, err := os.ReadFile(filepath.Join(dir, "21_19.ab"))
    require.NoError(t, err)
    require.True(t, isZstd(raw),
        "the sidecar must start with the zstd frame magic")

    // The frame unwraps to the EXACT bytes the plain sidecar would
    // carry: the zstd wrapping is a transparent transport (the decode
    // path of the loader answers the same graph), so a byte equal
    // re-encode of the same abstract proves the roundtrip whole.
    unwrapped, wasCompressed, err := maybeDecompressTile(raw)
    require.NoError(t, err)
    require.True(t, wasCompressed)

    tileBytes, err := os.ReadFile(filepath.Join(dir, "21_19.nm"))
    require.NoError(t, err)
    tileRaw, _, err := maybeDecompressTile(tileBytes)
    require.NoError(t, err)
    tile, err := navmesh.DecodeTile(tileRaw)
    require.NoError(t, err)
    expected := navmesh.BuildAbstract(tile)
    require.NotNil(t, expected)
    info, err := os.Stat(filepath.Join(dir, "21_19.nm"))
    require.NoError(t, err)
    expected.TileSize = info.Size()
    expected.TileModTime = info.ModTime()
    expected.TileChecksums = true
    expected.TileHeadCRC, expected.TileTailCRC =
        navmesh.TileChecksumsOf(tileBytes)
    expectedEncoded, err := navmesh.EncodeAbstract(expected)
    require.NoError(t, err)
    require.True(t, bytes.Equal(unwrapped, expectedEncoded),
        "the unwrapped sidecar bytes must equal the plain encode")
}
