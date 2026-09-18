// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "os"
    "strings"
    "testing"
)

// TestFakePattern counts the exact layer pattern of the zero height
// cells of the bay regions: how many are the single layer open
// (h == 0, nswe == 0x0F) filler the fake repair targets, how many
// carry other shapes (the real zero height terrain candidates).
func TestFakePattern(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    regions := "17_20,17_21,17_22,17_23,18_21,18_22"
    for _, spec := range strings.Split(regions, ",") {
        var col, row int16
        if _, err := fmt.Sscanf(spec, "%d_%d", &col, &row); err != nil {
            continue
        }
        path := fmt.Sprintf("%s/%d_%d.l2j", geodataDir, col, row)
        data, err := os.ReadFile(path) //nolint:gosec // the fixed dir
        if err != nil {
            continue
        }
        rl, err := extractRegion(data, col, row,
            DefaultOptions().DedupDelta)
        if err != nil {
            continue
        }
        singleOpen, singleWalls, multiZero, other := 0, 0, 0, 0
        for idx := range rl.cellCnt {
            off, cnt := int(rl.cellOff[idx]), int(rl.cellCnt[idx])
            if cnt == 0 {
                continue
            }
            first := rl.layers[off]
            if first.h != 0 {
                continue
            }
            switch {
            case cnt == 1 && first.nswe == 0x0F:
                singleOpen++
            case cnt == 1:
                singleWalls++
            default:
                multiZero++
            }
        }
        _ = other
        t.Logf("%d_%d: fake (1 layer h0 open) %d, 1 layer h0 walled %d,"+
            " multi layer with h0 %d", col, row, singleOpen, singleWalls,
            multiZero)
    }
}
