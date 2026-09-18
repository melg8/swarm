// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "sort"
    "testing"
)

// TestEastSeamScan walks the full 19_22 20_22 border and reports the
// crossable y ranges: the cell pairs whose height step stays inside
// the climb rule with the walls open - the places the mesh stitch can
// bridge. The answer tells whether the Giran route has a legal border
// crossing at all.
func TestEastSeamScan(t *testing.T) {
    west := loadRegionForDump(t, "../../../../data/geodata", 19, 22)
    east := loadRegionForDump(t, "../../../../data/geodata", 20, 22)
    climb := DefaultOptions().Climb

    firstLayerHeight := func(rl *regionLayers, x, y int) (int16, uint8,
        bool) {
        idx := x*regionCellsSide + y
        if rl.cellCnt[idx] == 0 {
            return 0, 0, false
        }
        layer := rl.layers[rl.cellOff[idx]]

        return layer.h, layer.nswe, true
    }
    type span struct {
        from, to int
    }
    var okSpans []span
    cur := span{-1, -1}
    for y := range regionCellsSide {
        crossable := false
        // Any west column x pair against the east edge columns: the
        // build pairs the border cells of both regions; the simplest
        // legal check: some of the last west cells against some of
        // the first east cells with an open nswe and the step inside
        // the climb.
        for wx := 2044; wx < 2048 && !crossable; wx++ {
            wh, wnswe, wok := firstLayerHeight(west, wx, y)
            if !wok {
                continue
            }
            for ex := 0; ex < 4; ex++ {
                eh, enswe, eok := firstLayerHeight(east, ex, y)
                if !eok {
                    continue
                }
                if eh-eh == 0 && wh <= waterLevel && eh <= waterLevel {
                    crossable = true // water to water

                    break
                }
                step := int32(eh) - int32(wh)
                if step < 0 {
                    step = -step
                }
                if step <= climb && wnswe != 0 && enswe != 0 {
                    crossable = true

                    break
                }
            }
        }
        if crossable {
            if cur.from < 0 {
                cur = span{y, y}
            } else {
                cur.to = y
            }

            continue
        }
        if cur.from >= 0 {
            okSpans = append(okSpans, cur)
            cur = span{-1, -1}
        }
    }
    if cur.from >= 0 {
        okSpans = append(okSpans, cur)
    }
    sort.Slice(okSpans, func(i, j int) bool {
        return (okSpans[j].to - okSpans[j].from) <
            (okSpans[i].to - okSpans[i].from)
    })
    total := 0
    for i, s := range okSpans {
        total += s.to - s.from + 1
        if i >= 12 {
            continue
        }
        t.Logf("crossable y span %d..%d (%d cells)", s.from, s.to,
            s.to-s.from+1)
    }
    t.Logf("total crossable border cells: %d of %d", total,
        regionCellsSide)
}
