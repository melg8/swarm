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

// TestBayAreaMap renders the coarse area map of the bay regions: per
// 32x32 cell block the dominant class - W the water, 0 the zero
// height dry (the l2j uninitialized default), d the other dry, x the
// mixed block. The diagnosis map of the fake land over the Gludio
// Gludin bay.
func TestBayAreaMap(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    regions := "16_21,17_21,18_21,19_21,16_22,17_22,18_22,19_22"
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
            DefaultOptions().DedupDelta)
        if err != nil {
            t.Logf("%d_%d: parse error: %v", col, row, err)

            continue
        }
        var b strings.Builder
        for bx := 0; bx < regionCellsSide; bx += 32 {
            for by := 0; by < regionCellsSide; by += 32 {
                water, zero, dry, other := 0, 0, 0, 0
                for cx := bx; cx < bx+32; cx += 4 {
                    for cy := by; cy < by+32; cy += 4 {
                        idx := cx*regionCellsSide + cy
                        off, cnt := int(rl.cellOff[idx]),
                            int(rl.cellCnt[idx])
                        if cnt == 0 {
                            other++

                            continue
                        }
                        layer := rl.layers[off]
                        switch {
                        case layer.h <= waterLevel:
                            water++
                        case layer.h == 0:
                            zero++
                        default:
                            dry++
                        }
                    }
                }
                switch {
                case water >= zero+dry+other && water > 0:
                    b.WriteByte('W')
                case zero >= dry+other && zero > 0:
                    b.WriteByte('0')
                case dry >= other && dry > 0:
                    b.WriteByte('d')
                case other > 0:
                    b.WriteByte('.')
                default:
                    b.WriteByte('?')
                }
            }
            b.WriteByte('\n')
        }
        t.Logf("region %d_%d (64 blocks of 32 cells, x down, y right):\n%s",
            col, row, b.String())
    }
}
