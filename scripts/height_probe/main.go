// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The height distribution probe of the height quantization evaluation
// (issue #56): decodes one tile and histograms the polygon corner
// heights by their residue mod the candidate quantization steps - the
// share of heights each step actually moves decides the candidates.
package main

import (
    "fmt"
    "os"
    "sort"

    "github.com/klauspost/compress/zstd"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// zstdReader returns the single threaded decoder the probe uses.
func zstdReader() (*zstd.Decoder, error) {
    return zstd.NewReader(nil,
        zstd.WithDecoderConcurrency(1),
        zstd.WithDecoderMaxMemory(1<<30))
}

func main() {
    path := "data/navmesh/19_23.nm"
    if len(os.Args) > 1 {
        path = os.Args[1]
    }
    raw, err := os.ReadFile(path)
    if err != nil {
        panic(err)
    }
    // The pack tiles are zstd framed; DecodeTile wants raw bytes.
    var data []byte
    if len(raw) >= 4 && raw[0] == 0x28 && raw[1] == 0xB5 &&
        raw[2] == 0x2F && raw[3] == 0xFD {
        dec, err := zstdReader()
        if err != nil {
            panic(err)
        }
        data, err = dec.DecodeAll(raw, nil)
        if err != nil {
            panic(err)
        }
    } else {
        data = raw
    }
    tile, err := navmesh.DecodeTile(data)
    if err != nil {
        panic(err)
    }
    steps := []int{2, 4, 8, 12, 16, 20, 24, 32, 40}
    hist := make(map[int]map[int]int)
    for _, s := range steps {
        hist[s] = make(map[int]int)
    }
    var heights []int
    for i := range tile.Polys {
        p := &tile.Polys[i]
        for _, h := range [4]int16{p.H00, p.H10, p.H01, p.H11} {
            hv := int(h)
            heights = append(heights, hv)
            for _, s := range steps {
                r := ((hv % s) + s) % s
                if r > s/2 {
                    r -= s // distance to the nearest multiple
                }
                hist[s][absInt(r)]++
            }
        }
    }
    sort.Ints(heights)
    fmt.Printf("tile %s: %d polys, %d corners\n",
        path, len(tile.Polys), len(heights))
    fmt.Printf("height range: %d .. %d\n",
        heights[0], heights[len(heights)-1])
    fmt.Println()
    fmt.Println("step | corners already on step | corners moved by snap")
    for _, s := range steps {
        on := hist[s][0]
        moved := len(heights) - on
        fmt.Printf("%4d | %7d (%5.2f%%)         | %7d (%5.2f%%)\n",
            s, on, 100*float64(on)/float64(len(heights)),
            moved, 100*float64(moved)/float64(len(heights)))
    }
    fmt.Println()
    // The snapping distance distribution per step (the error the
    // snapped surface carries against the exact geodata).
    fmt.Println("step | max snap distance | share moved <= s/4")
    for _, s := range steps {
        quarter := 0
        for d, n := range hist[s] {
            if d > 0 && d <= s/4 {
                quarter += n
            }
        }
        fmt.Printf("%4d | %3d               | %5.1f%% of all corners\n",
            s, s/2, 100*float64(quarter)/float64(len(heights)))
    }
}

func absInt(a int) int {
    if a < 0 {
        return -a
    }

    return a
}
