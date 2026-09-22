// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// navmesh-build converts the l2j geodata region files into navigation
// mesh tiles (docs/navmesh.md): the offline half of the
// feature/new-pathfind port. Every X_Y.l2j region of the geodata
// directory becomes an X_Y.nm tile in the output directory, the
// neighbouring regions stitch their border strips, and the audit
// lines report the sheet decomposition and the link health per
// region.
//
// Usage:
//
//    go run ./cmd/navmesh-build -geodata data/geodata -out data/navmesh
//
// The build is idempotent: a region whose tile file already carries a
// newer mtime than the region file is skipped unless -force is set.
// The pack phases run in parallel (one region in flight per worker,
// -workers caps the pool, default: every core).
package main

import (
    "flag"
    "fmt"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind/navbuild"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

func main() {
    geodataDir := flag.String("geodata", "data/geodata",
        "the geodata directory with the X_Y.l2j region files")
    outDir := flag.String("out", "data/navmesh",
        "the output directory for the X_Y.nm tiles")
    regions := flag.String("regions", "",
        "the comma separated region keys to build (col_row pairs),"+
            " default: every region of the geodata directory")
    force := flag.Bool("force", false,
        "rebuild the regions whose tile file is already fresh")
    compress := flag.Bool("compress", true,
        "zstd the tile files in place (the runtime detects the format "+
            "by the magic word, the legacy gzip packs stay readable; "+
            "roughly half the disk)")
    workers := flag.Int("workers", 0,
        "parallel build workers (0: every core)")
    repairFake := flag.Bool("repair-fake", false,
        "replace the l2j uninitialized filler cells (one layer, "+
            "height 0, fully open) with the nearest real surface blend "+
            "(the bay filler turns into the sea floor water, the land "+
            "gaps into the connecting ground)")
    heightStep := flag.Int("height-step", 0,
        "snap every cell height to the nearest multiple of the step "+
            "before the decomposition (0 keeps the exact geodata "+
            "heights; the height quantization evaluation of issue #56 "+
            "measures the candidate steps 16, 24, 32, 40 - the 40 "+
            "unit climb limit bounds the walkable verdict noise)")
    flag.Parse()

    opts := navbuild.DefaultOptions()
    opts.Workers = *workers
    opts.RepairFake = *repairFake
    opts.HeightStep = int32(*heightStep)
    if err := run(*geodataDir, *outDir, *regions, *force, *compress,
        opts); err != nil {
        fmt.Println("Error:", err)
        os.Exit(1)
    }
}

// run executes the build over the requested regions.
func run(geodataDir, outDir, regionsSpec string, force, compress bool,
    opts navbuild.Options,
) error {
    keys, err := regionKeys(geodataDir, regionsSpec)
    if err != nil {
        return err
    }
    if len(keys) == 0 {
        return fmt.Errorf("no geodata regions under %s", geodataDir)
    }
    if err := os.MkdirAll(outDir, 0o755); err != nil {
        return fmt.Errorf("create the output directory: %w", err)
    }

    started := time.Now()
    stats, err := navbuild.BuildPack(geodataDir, outDir, keys, opts,
        force,
        func(format string, args ...any) {
            fmt.Printf(format+"\n", args...)
        })
    if err != nil {
        return err
    }
    if compress {
        // The sidecars stay plain (kilobytes), the tiles compress.
        saved, err := navbuild.CompressTileDir(outDir)
        if err != nil {
            return err
        }
        fmt.Printf("compressed %d tiles, %.1f MB saved\n",
            saved.Count, float64(saved.Bytes)/(1024*1024))
    }

    // The persistent coarse layer pass (after the compression: the
    // sidecar records the final tile stat the runtime checks). Every
    // geodata region with a built tile gets its cluster graph
    // sidecar; the runtime falls back to the tile scan without one.
    allKeys, err := navbuild.RegionKeysOfDir(geodataDir)
    if err != nil {
        return err
    }
    abstracts, err := navbuild.WriteAbstractSidecars(outDir, allKeys,
        opts.Workers,
        func(format string, args ...any) {
            fmt.Printf(format+"\n", args...)
        })
    if err != nil {
        return err
    }
    fmt.Printf("wrote %d abstract sidecars (%d skipped, %d failed),"+
        " %.1f MB\n", abstracts.Written, abstracts.Skipped,
        abstracts.Failed, float64(abstracts.Bytes)/(1024*1024))
    fmt.Printf("built %d regions (%d skipped, %d upgraded, %d failed),"+
        " %d polys, %d links, %d external links, %.1f MB of tiles in"+
        " %s\n",
        stats.Built, stats.Skipped, stats.Upgraded, stats.Failed,
        stats.Polys, stats.Links, stats.Stitched,
        float64(stats.TileBytes)/(1024*1024),
        time.Since(started).Round(time.Millisecond))

    return nil
}

// regionKeys resolves the region keys of the run: the explicit
// specification or the directory listing.
func regionKeys(geodataDir, regionsSpec string,
) ([]navmesh.RegionKey, error) {
    if regionsSpec == "" {
        return navbuild.RegionKeysOfDir(geodataDir)
    }
    specs := strings.Split(regionsSpec, ",")
    keys := make([]navmesh.RegionKey, 0, len(specs))
    for _, spec := range specs {
        colText, rowText, found := strings.Cut(strings.TrimSpace(spec),
            "_")
        if !found {
            return nil, fmt.Errorf("bad region spec %q", spec)
        }
        col, err := strconv.Atoi(colText)
        if err != nil || col < -32768 || col > 32767 {
            return nil, fmt.Errorf("bad region col %q: %w", colText, err)
        }
        row, err := strconv.Atoi(rowText)
        if err != nil || row < -32768 || row > 32767 {
            return nil, fmt.Errorf("bad region row %q: %w", rowText, err)
        }
        keys = append(keys, navmesh.RegionKey{
            //nolint:gosec // the bounds above pin col and row into
            // the int16 range before the conversion.
            Col: int16(col), Row: int16(row)})
    }

    return keys, nil
}
