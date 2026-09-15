// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"

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
    strips := buildPackPhaseA(keys, geodataDir, outDir, opts, force,
        &stats, log)

    // The phase B: only the regions whose neighbours were built in
    // this pass (or skipped with a fresh tile - their strips are
    // absent, the stitch then legitimately adds nothing: the fresh
    // tile of a previous run already carries its links).
    order := make([]navmesh.RegionKey, 0, len(strips))
    for key := range strips {
        order = append(order, key)
    }
    sort.Slice(order, func(i, j int) bool {
        if order[i].Col != order[j].Col {
            return order[i].Col < order[j].Col
        }

        return order[i].Row < order[j].Row
    })
    for _, key := range order {
        var neighbors [4]*borderStrips
        for side, delta := range [4][2]int16{{-1, 0}, {1, 0}, {0, -1},
            {0, 1}} {
            neighbor := navmesh.RegionKey{
                Col: key.Col + delta[0], Row: key.Row + delta[1],
            }
            if strip, ok := strips[neighbor]; ok {
                neighbors[side] = strip
            }
        }
        if neighbors[0] == nil && neighbors[1] == nil &&
            neighbors[2] == nil && neighbors[3] == nil {
            continue
        }
        added, err := stitchPackTile(outDir, key, strips[key], neighbors,
            opts)
        if err != nil {
            return stats, fmt.Errorf("stitch %d_%d: %w", key.Col, key.Row,
                err)
        }
        stats.Stitched += added
    }

    return stats, nil
}

// buildPackPhaseA builds every requested region, writes the tiles
// and retains the border strips for the phase B stitching. The stats
// mutate in place; the answer is the strips map.
func buildPackPhaseA(keys []navmesh.RegionKey, geodataDir, outDir string,
    opts Options, force bool, stats *PackStats,
    log func(format string, args ...any),
) map[navmesh.RegionKey]*borderStrips {
    strips := make(map[navmesh.RegionKey]*borderStrips, len(keys))
    for _, key := range keys {
        build, err := buildPackRegion(geodataDir, outDir, key, opts,
            force)
        if err != nil {
            log("region %d_%d FAILED: %v", key.Col, key.Row, err)
            stats.Failed++

            continue
        }
        if build == nil {
            stats.Skipped++

            continue
        }
        // The strips copy breaks the reference to the RegionBuild -
        // addressing the field of the build would pin the whole tile
        // (hundreds of megabytes per dense region) for the phase B.
        own := build.Strips
        strips[key] = &own
        stats.Built++
        stats.Polys += build.Stats.Polys
        stats.Links += build.Stats.Links
        if info, err := os.Stat(tilePathOf(outDir, key)); err == nil {
            stats.TileBytes += info.Size()
        }
        log("region %d_%d: %d layers, %d sheets (%d islands),"+
            " %d polys (%d water), %d links, %d blocked pairs, %s",
            key.Col, key.Row, build.Stats.Layers, build.Stats.Sheets,
            build.Stats.DroppedSheets, build.Stats.Polys,
            build.Stats.WaterPolys, build.Stats.Links,
            build.Stats.NSWEBlockedPairs,
            build.Stats.BuildTime.String())
    }

    return strips
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
    tile, err := navmesh.DecodeTile(data)
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
    if err := os.WriteFile(tilePath, encoded, 0o600); err != nil {
        return 0, fmt.Errorf("rewrite the tile: %w", err)
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
