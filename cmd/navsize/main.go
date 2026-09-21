// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

// Scratch analysis of the navmesh tile pack sizes (issue #11): walks
// a tile directory, decodes every tile and breaks the wire bytes down
// by section (the polygon planes, the area plane, the link CSR, the
// span words, the target deltas, the external links, the grid index),
// then re-encodes the raw tile at several zstd levels to measure the
// compression headroom the current SpeedDefault wrapping leaves.
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "sync"

    "github.com/klauspost/compress/zstd"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

func main() {
    dir := "data/navmesh"
    if len(os.Args) > 1 {
        dir = os.Args[1]
    }
    // The zstd level sweep of the raw (decompressed) tile bytes.
    levels := []struct {
        name  string
        level zstd.EncoderLevel
    }{
        {"default", zstd.SpeedDefault},
        {"better", zstd.SpeedBetterCompression},
        {"best", zstd.SpeedBestCompression},
    }
    encoders := make([]*zstd.Encoder, len(levels))
    for i, lv := range levels {
        enc, err := zstd.NewWriter(nil,
            zstd.WithEncoderConcurrency(1),
            zstd.WithEncoderLevel(lv.level))
        if err != nil {
            panic(err)
        }
        encoders[i] = enc
    }
    decoder, err := zstd.NewReader(nil,
        zstd.WithDecoderConcurrency(1),
        zstd.WithDecoderMaxMemory(1<<30))
    if err != nil {
        panic(err)
    }

    entries, err := os.ReadDir(dir)
    if err != nil {
        panic(err)
    }
    var files []string
    for _, e := range entries {
        if filepath.Ext(e.Name()) == ".nm" {
            files = append(files, filepath.Join(dir, e.Name()))
        }
    }
    sort.Strings(files)
    fmt.Printf("tiles: %d\n", len(files))

    type totals struct {
        fileBytes   int64
        rawBytes    int64
        polys       int64
        links       int64
        extLinks    int64
        polyPlanes  int64
        areaPlane   int64
        csr         int64
        spanWords   int64
        toDeltas    int64
        extSection  int64
        gridOffsets int64
        gridEntries int64
        headers     int64
        levelBytes  []int64
    }
    //nolint:exhaustruct_v5 // the accumulator starts zeroed
    newTotals := func() totals {
        return totals{levelBytes: make([]int64, len(levels))}
    }
    grand := newTotals()
    var mu sync.Mutex
    var wg sync.WaitGroup
    sem := make(chan struct{}, 4)
    for _, path := range files {
        wg.Add(1)
        go func(path string) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()
            compressed, err := os.ReadFile(path)
            if err != nil {
                panic(err)
            }
            fileLen := int64(len(compressed))
            var raw []byte
            if len(compressed) >= 4 && compressed[0] == 0x28 &&
                compressed[1] == 0xB5 && compressed[2] == 0x2F &&
                compressed[3] == 0xFD {
                raw, err = decoder.DecodeAll(compressed, nil)
                if err != nil {
                    fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)

                    return
                }
            } else {
                raw = compressed
            }
            tile, err := navmesh.DecodeTile(raw)
            if err != nil {
                fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)

                return
            }
            t := newTotals()
            t.fileBytes = fileLen
            t.polys = int64(len(tile.Polys))
            t.links = int64(len(tile.Links))
            t.extLinks = int64(len(tile.ExtLinks))
            t.polyPlanes = 16 * t.polys
            t.areaPlane = t.polys
            t.csr = 4 * (t.polys + 1)
            t.spanWords = 4 * t.links
            t.extSection = 8 * t.extLinks
            if tile.Grid != nil {
                t.gridOffsets = 4 * int64(len(tile.Grid.Offsets))
                t.gridEntries = 4 * int64(len(tile.Grid.Entries))
            }
            t.rawBytes = int64(len(raw))
            t.headers = 40
            t.toDeltas = t.rawBytes - t.polyPlanes - t.areaPlane -
                t.csr - t.spanWords - t.extSection -
                t.gridOffsets - t.gridEntries - t.headers
            for i, enc := range encoders {
                t.levelBytes[i] = int64(len(enc.EncodeAll(raw, nil)))
            }
            mu.Lock()
            grand.fileBytes += t.fileBytes
            grand.rawBytes += t.rawBytes
            grand.polys += t.polys
            grand.links += t.links
            grand.extLinks += t.extLinks
            grand.polyPlanes += t.polyPlanes
            grand.areaPlane += t.areaPlane
            grand.csr += t.csr
            grand.spanWords += t.spanWords
            grand.toDeltas += t.toDeltas
            grand.extSection += t.extSection
            grand.gridOffsets += t.gridOffsets
            grand.gridEntries += t.gridEntries
            grand.headers += t.headers
            for i := range t.levelBytes {
                grand.levelBytes[i] += t.levelBytes[i]
            }
            mu.Unlock()
        }(path)
    }
    wg.Wait()

    mb := func(v int64) float64 { return float64(v) / 1024 / 1024 }
    fmt.Printf("file bytes:      %8.1f MB\n", mb(grand.fileBytes))
    fmt.Printf("raw wire bytes:  %8.1f MB\n", mb(grand.rawBytes))
    fmt.Printf("polys:           %d\n", grand.polys)
    fmt.Printf("links:           %d\n", grand.links)
    fmt.Printf("ext links:       %d\n", grand.extLinks)
    fmt.Printf("-- raw sections --\n")
    fmt.Printf("poly planes:     %8.1f MB (16 B/poly)\n", mb(grand.polyPlanes))
    fmt.Printf("area plane:      %8.1f MB (1 B/poly)\n", mb(grand.areaPlane))
    fmt.Printf("link CSR:        %8.1f MB (4 B/poly)\n", mb(grand.csr))
    fmt.Printf("span words:      %8.1f MB (4 B/link)\n", mb(grand.spanWords))
    fmt.Printf("target deltas:   %8.1f MB (uvarint)\n", mb(grand.toDeltas))
    fmt.Printf("ext link table:  %8.1f MB (8 B/ext)\n", mb(grand.extSection))
    fmt.Printf("grid offsets:    %8.1f MB\n", mb(grand.gridOffsets))
    fmt.Printf("grid entries:    %8.1f MB\n", mb(grand.gridEntries))
    fmt.Printf("headers:         %8.1f MB\n", mb(grand.headers))
    fmt.Printf("-- zstd level sweep of the raw bytes --\n")
    for i, lv := range levels {
        fmt.Printf("level %-12s %8.1f MB  (%.1f%% of the shipped default)\n",
            lv.name, mb(grand.levelBytes[i]),
            100*float64(grand.levelBytes[i])/float64(grand.fileBytes))
    }

    // The abstract sidecar sweep: the plain .ab files and their zstd
    // headroom (the compression round of the pack measured the plain
    // sidecars at 405.9 MB against 72.6 MB framed).
    var sidecarFiles []string
    for _, e := range entries {
        if filepath.Ext(e.Name()) == ".ab" {
            sidecarFiles = append(sidecarFiles,
                filepath.Join(dir, e.Name()))
        }
    }
    var plain, framed int64
    for _, path := range sidecarFiles {
        data, err := os.ReadFile(path)
        if err != nil {
            panic(err)
        }
        plain += int64(len(data))
        framed += int64(len(encoders[len(levels)-1].EncodeAll(data, nil)))
    }
    fmt.Printf("-- abstract sidecars: %d files --\n", len(sidecarFiles))
    fmt.Printf("on disk:          %8.1f MB\n", mb(plain))
    fmt.Printf("zstd best:        %8.1f MB (%.1f%%)\n", mb(framed),
        100*float64(framed)/float64(plain))
}
