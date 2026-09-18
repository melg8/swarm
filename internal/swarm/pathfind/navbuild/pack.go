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
    "sort"
    "strconv"
    "strings"
    "sync"
    "sync/atomic"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// PackStats summarizes a pack build.
type PackStats struct {
    Built     int
    Skipped   int
    Failed    int
    Polys     int
    Links     int
    Stitched  int
    TileBytes int64
}

// BuildPack builds every requested region of the geodata directory
// into tiles of the output directory and stitches the region borders.
// The two phases keep the memory flat: phase A builds one region,
// writes its tile and retains only the border strips (a few dozen
// kilobytes per region), phase B re-decodes the tiles whose
// neighbours exist and appends the external links. A region whose
// tile file is newer than its region file is skipped unless force is
// set. The log callback reports one line per region (nil mutes).
//
// The phases run in a worker pool (opts.Workers goroutines at most,
// one region in flight per worker): the region builds and the tile
// stitches are file disjoint, so the pack scales over the cores
// without locking (the strips map freezes after phase A and phase B
// reads it concurrently). Workers <= 0 answers runtime.NumCPU().
func BuildPack(geodataDir, outDir string, keys []navmesh.RegionKey,
    opts Options, force bool, log func(format string, args ...any),
) (PackStats, error) {
    stats := PackStats{
        Built:     0,
        Skipped:   0,
        Failed:    0,
        Polys:     0,
        Links:     0,
        Stitched:  0,
        TileBytes: 0,
    }
    if log == nil {
        log = func(string, ...any) {}
    }
    counters := packCounters{}
    strips, err := buildPackPhaseA(keys, geodataDir, outDir, opts,
        force, &counters, log)
    if err != nil {
        return stats, err
    }

    // The phase B stitches every requested region whose tile file
    // exists: the strips come from the in-pass builds or, for the
    // tiles of earlier passes, from the sidecar files (the chunked
    // pack builds stitch their cross chunk borders on the final full
    // pass). The stitch dedupes against the links the tile already
    // carries, so the repeated passes add nothing.
    order := make([]navmesh.RegionKey, 0, len(keys))
    for _, key := range keys {
        if _, ok := strips[key]; ok {
            order = append(order, key)

            continue
        }
        if _, err := os.Stat(tilePathOf(outDir, key)); err == nil {
            order = append(order, key)
        }
    }
    sort.Slice(order, func(i, j int) bool {
        if order[i].Col != order[j].Col {
            return order[i].Col < order[j].Col
        }

        return order[i].Row < order[j].Row
    })
    if err := stitchPackPhaseB(order, outDir, strips, opts, &counters); err != nil {
        return stats, err
    }

    stats.Built = int(counters.built.Load())
    stats.Skipped = int(counters.skipped.Load())
    stats.Failed = int(counters.failed.Load())
    stats.Polys = int(counters.polys.Load())
    stats.Links = int(counters.links.Load())
    stats.Stitched = int(counters.stitched.Load())
    stats.TileBytes = counters.tileBytes.Load()

    return stats, nil
}

// packCounters is the atomic stats accumulator of the parallel pack
// phases (the workers never touch the exported PackStats directly).
type packCounters struct {
    built     atomic.Int64
    skipped   atomic.Int64
    failed    atomic.Int64
    polys     atomic.Int64
    links     atomic.Int64
    stitched  atomic.Int64
    tileBytes atomic.Int64
}

// packWorkers resolves the worker count of the parallel phases.
func packWorkers(opts Options, jobs int) int {
    workers := opts.Workers
    if workers <= 0 {
        workers = runtime.NumCPU()
    }
    if workers > jobs {
        workers = jobs
    }

    return workers
}

// runPool feeds the keys through the worker pool: one work unit per
// key, the first hard error cancels the feed (the in flight units
// drain). The work callback answers one error at most.
func runPool(ctx context.Context, keys []navmesh.RegionKey,
    workers int, work func(context.Context, navmesh.RegionKey) error,
) error {
    jobs := make(chan navmesh.RegionKey)
    feedCtx, cancelFeed := context.WithCancel(ctx)
    defer cancelFeed()
    go func() {
        defer close(jobs)
        for _, key := range keys {
            select {
            case jobs <- key:
            case <-feedCtx.Done():
                return
            }
        }
    }()
    var (
        wg       sync.WaitGroup
        firstErr error
        errMu    sync.Mutex
    )
    for range workers {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for {
                select {
                case <-ctx.Done():
                    return
                case key, ok := <-jobs:
                    if !ok {
                        return
                    }
                    if err := work(ctx, key); err != nil {
                        errMu.Lock()
                        if firstErr == nil {
                            firstErr = err
                        }
                        errMu.Unlock()
                        cancelFeed()

                        return
                    }
                }
            }
        }()
    }
    wg.Wait()

    return firstErr
}

// buildPackPhaseA builds every requested region, writes the tiles
// and retains the border strips for the phase B stitching. The region
// builds run through the worker pool (one region in flight per
// worker); the strips map mutates under the log mutex and freezes
// once the phase answers. The counters accumulate atomically; the
// answer is the strips map.
func buildPackPhaseA(keys []navmesh.RegionKey, geodataDir, outDir string,
    opts Options, force bool, counters *packCounters,
    log func(format string, args ...any),
) (map[navmesh.RegionKey]*borderStrips, error) {
    var (
        mu          sync.Mutex // guards the strips map and the log lines
        strips      = make(map[navmesh.RegionKey]*borderStrips, len(keys))
        ctx, cancel = context.WithCancel(context.Background())
    )
    defer cancel()
    err := runPool(ctx, keys, packWorkers(opts, len(keys)),
        func(_ context.Context, key navmesh.RegionKey) error {
            build, err := buildPackRegion(geodataDir, outDir, key,
                opts, force)
            if err != nil {
                mu.Lock()
                log("region %d_%d FAILED: %v", key.Col, key.Row, err)
                mu.Unlock()
                counters.failed.Add(1)

                return nil
            }
            if build == nil {
                counters.skipped.Add(1)

                return nil
            }
            // The strips copy breaks the reference to the RegionBuild
            // - addressing the field of the build would pin the whole
            // tile (hundreds of megabytes per dense region) for the
            // phase B.
            own := build.Strips
            if err := writeStripsSidecar(outDir, key, own); err != nil {
                cancel()

                return fmt.Errorf("sidecar %d_%d: %w", key.Col,
                    key.Row, err)
            }
            mu.Lock()
            strips[key] = &own
            log("region %d_%d: %d layers, %d sheets (%d islands,"+
                " %d floating), %d polys (%d water), %d links,"+
                " %d blocked pairs, %s",
                key.Col, key.Row, build.Stats.Layers, build.Stats.Sheets,
                build.Stats.DroppedSheets, build.Stats.IslandSheets,
                build.Stats.Polys,
                build.Stats.WaterPolys, build.Stats.Links,
                build.Stats.NSWEBlockedPairs,
                build.Stats.BuildTime.String())
            mu.Unlock()
            counters.built.Add(1)
            counters.polys.Add(int64(build.Stats.Polys))
            counters.links.Add(int64(build.Stats.Links))
            if info, err := os.Stat(tilePathOf(outDir, key)); err == nil {
                counters.tileBytes.Add(info.Size())
            }

            return nil
        })
    if err != nil {
        return nil, err
    }

    return strips, nil
}

// stitchPackPhaseB stitches the ordered region keys through the
// worker pool: the strips map is frozen (concurrent reads are safe)
// and the tile rewrites are file disjoint per key.
func stitchPackPhaseB(order []navmesh.RegionKey, outDir string,
    strips map[navmesh.RegionKey]*borderStrips, opts Options,
    counters *packCounters,
) error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    return runPool(ctx, order, packWorkers(opts, len(order)),
        func(_ context.Context, key navmesh.RegionKey) error {
            own, ok := strips[key]
            if !ok {
                loaded, loadedOK := readStripsSidecar(outDir, key)
                if !loadedOK {
                    // No in-pass strip and no sidecar: the tile of an
                    // older build cannot pair against anything new.
                    return nil
                }
                own = &loaded
            }
            var neighbors [4]*borderStrips
            for side, delta := range [4][2]int16{{-1, 0}, {1, 0},
                {0, -1}, {0, 1}} {
                neighbor := navmesh.RegionKey{
                    Col: key.Col + delta[0], Row: key.Row + delta[1],
                }
                if strip, ok := strips[neighbor]; ok {
                    neighbors[side] = strip

                    continue
                }
                if _, err := os.Stat(tilePathOf(outDir, neighbor)); err != nil {
                    continue
                }
                loaded, loadedOK := readStripsSidecar(outDir, neighbor)
                if loadedOK {
                    neighbors[side] = &loaded
                }
            }
            if neighbors[0] == nil && neighbors[1] == nil &&
                neighbors[2] == nil && neighbors[3] == nil {
                return nil
            }
            added, err := stitchPackTile(outDir, key, own, neighbors,
                opts)
            if err != nil {
                cancel()

                return fmt.Errorf("stitch %d_%d: %w", key.Col,
                    key.Row, err)
            }
            counters.stitched.Add(int64(added))

            return nil
        })
}

// buildPackRegion builds one region and writes its tile unless the
// tile is already fresh (a nil build answers the skip).
func buildPackRegion(geodataDir, outDir string, key navmesh.RegionKey,
    opts Options, force bool,
) (*RegionBuild, error) {
    regionPath := filepath.Join(geodataDir,
        fmt.Sprintf("%d_%d.l2j", key.Col, key.Row))
    tilePath := tilePathOf(outDir, key)
    regionInfo, err := os.Stat(regionPath)
    if err != nil {
        return nil, fmt.Errorf("stat the region: %w", err)
    }
    if !force {
        if tileInfo, statErr := os.Stat(tilePath); statErr == nil &&
            tileInfo.ModTime().After(regionInfo.ModTime()) {
            return nil, nil
        }
    }
    data, err := os.ReadFile(regionPath) //nolint:gosec // a fixed arg
    if err != nil {
        return nil, fmt.Errorf("read the region: %w", err)
    }
    build, err := BuildRegion(data, key.Col, key.Row, opts)
    if err != nil {
        return nil, fmt.Errorf("build: %w", err)
    }
    encoded, err := navmesh.EncodeTile(build.Tile)
    if err != nil {
        return nil, fmt.Errorf("encode: %w", err)
    }
    if err := os.WriteFile(tilePath, encoded, 0o600); err != nil {
        return nil, fmt.Errorf("write the tile: %w", err)
    }

    return build, nil
}

// stitchPackTile decodes one written tile, appends the external links
// against the neighbour strips and rewrites it.
func stitchPackTile(outDir string, key navmesh.RegionKey,
    own *borderStrips, neighbors [4]*borderStrips, opts Options,
) (int, error) {
    tilePath := tilePathOf(outDir, key)
    data, err := os.ReadFile(tilePath) //nolint:gosec // a fixed arg
    if err != nil {
        return 0, fmt.Errorf("read the tile: %w", err)
    }
    raw, compressed, err := maybeDecompressTile(data)
    if err != nil {
        return 0, fmt.Errorf("decompress %d_%d: %w", key.Col, key.Row,
            err)
    }
    tile, err := navmesh.DecodeTile(raw)
    if err != nil {
        return 0, fmt.Errorf("decode the tile: %w", err)
    }
    added, err := StitchRegion(tile, *own, neighbors, opts)
    if err != nil {
        return 0, err
    }
    if added == 0 {
        return 0, nil
    }
    encoded, err := navmesh.EncodeTile(tile)
    if err != nil {
        return 0, fmt.Errorf("re-encode: %w", err)
    }
    encoded, err = maybeCompressTile(encoded, compressed)
    if err != nil {
        return 0, fmt.Errorf("re-compress %d_%d: %w", key.Col, key.Row,
            err)
    }
    // The atomic rewrite: a killed build must never leave a half
    // written tile behind.
    tmp := tilePath + ".tmp"
    //nolint:gosec // the tmp path is the tile path plus .tmp, the
    // tile path is the validated out dir plus the region key pair.
    if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
        return 0, fmt.Errorf("write the tile: %w", err)
    }
    if err := os.Rename(tmp, tilePath); err != nil {
        return 0, fmt.Errorf("replace the tile: %w", err)
    }

    return added, nil
}

// tilePathOf returns the tile file path of a region key.
func tilePathOf(outDir string, key navmesh.RegionKey) string {
    return filepath.Join(outDir, fmt.Sprintf("%d_%d.nm", key.Col,
        key.Row))
}

// RegionKeysOfDir lists the region keys of the geodata directory
// (sorted, col then row).
func RegionKeysOfDir(geodataDir string,
) ([]navmesh.RegionKey, error) {
    entries, err := os.ReadDir(geodataDir)
    if err != nil {
        return nil, fmt.Errorf("read the geodata directory: %w", err)
    }
    keys := make([]navmesh.RegionKey, 0, len(entries))
    for _, entry := range entries {
        base, ok := strings.CutSuffix(entry.Name(), ".l2j")
        if !ok {
            continue
        }
        key, ok := parseRegionKey(base)
        if !ok {
            continue
        }
        keys = append(keys, key)
    }
    sort.Slice(keys, func(i, j int) bool {
        if keys[i].Col != keys[j].Col {
            return keys[i].Col < keys[j].Col
        }

        return keys[i].Row < keys[j].Row
    })

    return keys, nil
}

// parseRegionKey parses "col_row" into a region key.
func parseRegionKey(base string) (navmesh.RegionKey, bool) {
    colText, rowText, found := strings.Cut(base, "_")
    if !found {
        return navmesh.RegionKey{Col: 0, Row: 0}, false
    }
    col, err := strconv.Atoi(colText)
    if err != nil || col < -32768 || col > 32767 {
        return navmesh.RegionKey{Col: 0, Row: 0}, false
    }
    row, err := strconv.Atoi(rowText)
    if err != nil || row < -32768 || row > 32767 {
        return navmesh.RegionKey{Col: 0, Row: 0}, false
    }

    return navmesh.RegionKey{ //nolint:gosec // guarded above
        Col: int16(col), Row: int16(row)}, true
}
