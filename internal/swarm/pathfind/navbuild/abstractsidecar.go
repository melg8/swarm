// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "runtime"
    "sync"
    "sync/atomic"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// AbstractStats summarizes the sidecar pass.
type AbstractStats struct {
    Written int
    Skipped int
    Failed  int
    Bytes   int64
}

// WriteAbstractSidecars serializes the cluster graph of every tile in
// the output directory into the X_Y.ab sidecars (the persistent
// coarse layer, docs/fastpath_research.md section 2). The pass runs
// after the tile build and the compression: the sidecar records the
// tile file stat it was built against and the runtime rejects a
// sidecar whose tile changed under it. One worker per in flight
// region (Workers caps the pool, 0 answers every core). The log
// callback reports one line per region (nil mutes). A region whose
// tile fails to decode counts failed and lets the pack finish.
func WriteAbstractSidecars(outDir string, keys []navmesh.RegionKey,
    workers int, log func(format string, args ...any),
) (AbstractStats, error) {
    if workers <= 0 {
        workers = runtime.NumCPU()
    }
    if workers > len(keys) {
        workers = len(keys)
    }
    if log == nil {
        log = func(string, ...any) {}
    }
    var (
        counters struct {
            written atomic.Int64
            skipped atomic.Int64
            failed  atomic.Int64
            bytes   atomic.Int64
        }
        mu sync.Mutex // keeps the log lines whole
    )
    ctx := context.Background()
    err := runPool(ctx, keys, workers,
        func(_ context.Context, key navmesh.RegionKey) error {
            size, err := writeAbstractSidecar(outDir, key)
            if err != nil {
                mu.Lock()
                log("abstract %d_%d FAILED: %v", key.Col, key.Row, err)
                mu.Unlock()
                counters.failed.Add(1)

                return nil
            }
            if size == 0 {
                counters.skipped.Add(1)

                return nil
            }
            counters.written.Add(1)
            counters.bytes.Add(size)
            mu.Lock()
            log("abstract %d_%d: %.1f KB", key.Col, key.Row,
                float64(size)/1024)
            mu.Unlock()

            return nil
        })
    if err != nil {
        return AbstractStats{}, err
    }

    return AbstractStats{
        Written: int(counters.written.Load()),
        Skipped: int(counters.skipped.Load()),
        Failed:  int(counters.failed.Load()),
        Bytes:   counters.bytes.Load(),
    }, nil
}

// writeAbstractSidecar builds the cluster graph of one tile and
// writes its sidecar; the answer is the sidecar byte count (zero for
// a skip). The tile decodes through the same gzip aware read the
// stitch uses, so the pass runs before or after the compression.
func writeAbstractSidecar(outDir string, key navmesh.RegionKey,
) (int64, error) {
    tilePath := tilePathOf(outDir, key)
    info, err := os.Stat(tilePath)
    if err != nil {
        return 0, nil // no tile: no sidecar (the absent region)
    }
    data, err := os.ReadFile(tilePath) //nolint:gosec // the fixed dir
    if err != nil {
        return 0, fmt.Errorf("read the tile: %w", err)
    }
    raw, _, err := maybeDecompressTile(data)
    if err != nil {
        return 0, err
    }
    tile, err := navmesh.DecodeTile(raw)
    if err != nil {
        return 0, fmt.Errorf("decode the tile: %w", err)
    }
    abstract := navmesh.BuildAbstract(tile)
    abstract.TileSize = info.Size()
    abstract.TileModTime = info.ModTime()
    encoded, err := navmesh.EncodeAbstract(abstract)
    if err != nil {
        return 0, fmt.Errorf("encode the abstract: %w", err)
    }
    sidecarPath := filepath.Join(outDir,
        fmt.Sprintf("%d_%d.ab", key.Col, key.Row))
    tmp := sidecarPath + ".tmp"
    if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
        return 0, fmt.Errorf("write the sidecar: %w", err)
    }
    if err := os.Rename(tmp, sidecarPath); err != nil {
        return 0, fmt.Errorf("replace the sidecar: %w", err)
    }

    return int64(len(encoded)), nil
}
