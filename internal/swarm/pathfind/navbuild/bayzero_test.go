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

// TestBayZeroHistogram counts the full resolution cell classes of the
// bay regions: the water cells, the zero height dry cells (the l2j
// uninitialized default), the other dry cells and the void cells -
// with the bounding box of the zero class. The zero extent over the
// bay decides the fake land repair scope.
func TestBayZeroHistogram(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    regions := "16_21,17_21,18_21,19_21,16_22,17_22,18_22,19_22," +
        "17_20,18_20,19_20,20_21,20_22,17_23,18_23"
    for _, spec := range strings.Split(regions, ",") {
        var col, row int16
        if _, err := fmt.Sscanf(spec, "%d_%d", &col, &row); err != nil {
            continue
        }
        path := fmt.Sprintf("%s/%d_%d.l2j", geodataDir, col, row)
        data, err := os.ReadFile(path)
        if err != nil {
            t.Logf("%d_%d: unreadable", col, row)

            continue
        }
        rl, err := extractRegion(data, col, row,
            DefaultOptions().DedupDelta, 0)
        if err != nil {
            t.Logf("%d_%d: parse error: %v", col, row, err)

            continue
        }
        water, zero, dry, voids := 0, 0, 0, 0
        zMinX, zMinY, zMaxX, zMaxY := 1<<30, 1<<30, -1, -1
        for cx := range regionCellsSide {
            for cy := range regionCellsSide {
                idx := cx*regionCellsSide + cy
                off, cnt := int(rl.cellOff[idx]), int(rl.cellCnt[idx])
                if cnt == 0 {
                    voids++

                    continue
                }
                layer := rl.layers[off]
                switch {
                case layer.h <= waterLevel:
                    water++
                case layer.h == 0:
                    zero++
                    if cx < zMinX {
                        zMinX = cx
                    }
                    if cy < zMinY {
                        zMinY = cy
                    }
                    if cx > zMaxX {
                        zMaxX = cx
                    }
                    if cy > zMaxY {
                        zMaxY = cy
                    }
                default:
                    dry++
                }
            }
        }
        zBox := "none"
        if zero > 0 {
            zBox = fmt.Sprintf("bbox %d_%d..%d_%d (%d cells wide)",
                zMinX, zMinY, zMaxX, zMaxY, zMaxX-zMinX+1)
        }
        t.Logf("%d_%d: water %d, zero-dry %d, dry %d, void %d; zero %s",
            col, row, water, zero, dry, voids, zBox)
    }
}
