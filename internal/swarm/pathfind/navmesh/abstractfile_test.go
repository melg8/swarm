// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// clusterSplitWorld builds the two polygon tile whose link crosses
// the x 128 cluster boundary: the roundtrip exercises the edge wire.
func clusterSplitWorld() *Tile {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 128, y1: 160, h: 0, area: AreaGround},
        {x0: 128, y0: 0, x1: 256, y1: 160, h: 0, area: AreaGround},
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxX, to: 1, t0: 0, t1: 159},
        {poly: 1, side: SideMinX, to: 0, t0: 0, t1: 159},
    }

    return assembleTile(21, 19, rects, links, nil)
}

// TestAbstractSidecarRoundtrip builds the abstract of the cluster
// split world tile, serializes it, decodes it back and compares every
// structure - the persistent coarse layer must answer the same graph
// the tile scan builds.
func TestAbstractSidecarRoundtrip(t *testing.T) {
    tile := clusterSplitWorld()
    original := BuildAbstract(tile)
    require.NotNil(t, original)
    require.NotEmpty(t, original.nodes)
    require.NotEmpty(t, original.edges, "the world must cross the"+
        " cluster boundary")

    data, err := EncodeAbstract(original)
    require.NoError(t, err)
    decoded, err := DecodeAbstract(data)
    require.NoError(t, err)

    require.Equal(t, original.key, decoded.key)
    require.Equal(t, original.polys, decoded.polys)
    require.Equal(t, original.comps, decoded.comps)
    require.Equal(t, original.edges, decoded.edges)
    require.Len(t, decoded.nodes, len(original.nodes))
    for i := range original.nodes {
        require.Equal(t, original.nodes[i].id, decoded.nodes[i].id)
        require.Equal(t, original.nodes[i].edges,
            decoded.nodes[i].edges)
        require.Len(t, decoded.nodes[i].classes,
            len(original.nodes[i].classes))
        for class, idx := range original.nodes[i].classes {
            got, ok := decoded.nodes[i].classes[class]
            require.True(t, ok, "the class %v survives", class)
            require.Equal(t, idx, got)
        }
    }
    require.Equal(t, original.index, decoded.index)
}

// TestAbstractSidecarServesCoarse pins the sidecar path end to end:
// the pack dir with the tile and the sidecar serves the coarse graph
// from the sidecar (the stat guard passes), and a tile rewritten
// under a stale sidecar falls back to the rebuild. The discriminator
// is the TileSize stat: only the sidecar path fills it.
func TestAbstractSidecarServesCoarse(t *testing.T) {
    tile := clusterSplitWorld()
    dir := writeTiles(t, tile)
    key := RegionKey{Col: tile.Col, Row: tile.Row}

    tilePath := filepath.Join(dir,
        fmt.Sprintf("%d_%d%s", tile.Col, tile.Row, tileFileExt))
    info, err := os.Stat(tilePath)
    require.NoError(t, err)
    abstract := BuildAbstract(tile)
    abstract.TileSize = info.Size()
    abstract.TileModTime = info.ModTime()
    data, err := EncodeAbstract(abstract)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(filepath.Join(dir,
        fmt.Sprintf("%d_%d%s", tile.Col, tile.Row, abstractFileExt)),
        data, 0o600))

    // The fresh mesh over the same dir: the sidecar answers (the
    // TileSize stat proves the path) and the graph matches the tile
    // scan.
    fresh := NewMesh(dir)
    served := fresh.abstractOf(key)
    require.NotNil(t, served)
    require.Equal(t, info.Size(), served.TileSize)
    rebuilt := BuildAbstract(tile)
    require.Equal(t, rebuilt.edges, served.edges)
    require.Equal(t, rebuilt.comps, served.comps)

    // The stale guard: rewrite the tile, force a distinct mtime (the
    // file system timestamp granularity hides a fast rewrite) and a
    // fresh mesh (the restart) rejects the sidecar - the rebuild
    // answers the same graph with the stat left zero.
    encoded, err := EncodeTile(tile)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(tilePath, encoded, 0o600))
    bumped := time.Now().Add(time.Hour)
    require.NoError(t, os.Chtimes(tilePath, bumped, bumped))
    restarted := NewMesh(dir)
    stale := restarted.abstractOf(key)
    require.NotNil(t, stale)
    require.Zero(t, stale.TileSize)
    require.Equal(t, rebuilt.edges, stale.edges)
}
