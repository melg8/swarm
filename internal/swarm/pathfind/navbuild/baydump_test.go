// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "math"
    "os"
    "strings"
    "testing"
)

// TestBayCellDump dumps the raw geodata cell stacks along the Gludio
// Gludin bay crossing: the diagnosis of what the water actually
// carries (the layer heights, the NSWE walls and the shore steps the
// mesh build has to swim through). The probe skips without the
// geodata pack.
func TestBayCellDump(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }

    // The bay points of the north shore probe plus a shore to shore
    // transect: Gludio harbor (-12787, 122779) toward Gludin
    // (-80826, 149775).
    points := [][2]float64{
        {-80000, 135000}, {-75000, 138000}, {-85000, 140000},
        {-70000, 140000}, {-65000, 135000}, {-60000, 130000},
    }
    for _, p := range points {
        col := int16(math.Floor(p[0]/32768)) + 20
        row := int16(math.Floor(p[1]/32768)) + 18
        localX := int((p[0] - (float64(col)-20)*32768) / 16)
        localY := int((p[1] - (float64(row)-18)*32768) / 16)
        path := fmt.Sprintf("%s/%d_%d.l2j", geodataDir, col, row)
        data, err := os.ReadFile(path)
        if err != nil {
            t.Logf("(%0.f, %0.f) region %d_%d: unreadable %v", p[0],
                p[1], col, row, err)

            continue
        }
        rl, err := extractRegion(data, col, row,
            DefaultOptions().DedupDelta, 0)
        if err != nil {
            t.Logf("(%0.f, %0.f) region %d_%d cell %d %d: parse %v",
                p[0], p[1], col, row, localX, localY, err)

            continue
        }
        dumpCell(t, rl, localX, localY, fmt.Sprintf(
            "(%.0f, %.0f) in %d_%d", p[0], p[1], col, row))
        // The transect toward the Gludio shore: every 32 cells along
        // the diagonal to the harbor.
        dx, dy := localX-(-12787-((int(col)-20)*2048))/1, localY-0
        _ = dx
        _ = dy
    }

    // The shore transect: from the Gludio harbor east into the bay,
    // every 24 cells: the step profile the climb rule answers.
    harbor := [2]int{0, 0}
    harbor[0] = (-12787 - (-32768)) / 16 // region 19_21 local x
    harbor[1] = (122779 - 98304) / 16    // region 19_21 local y
    col, row := int16(19), int16(21)
    data, err := os.ReadFile(fmt.Sprintf("%s/%d_%d.l2j", geodataDir,
        col, row))
    if err == nil {
        rl, err := extractRegion(data, col, row,
            DefaultOptions().DedupDelta, 0)
        if err == nil {
            t.Logf("the Gludio harbor transect (region 19_21, from the" +
                " harbor cell east/north into the water):")
            for step := range 16 {
                cx := harbor[0] + step*24
                cy := harbor[1] + step*16
                if cx >= regionCellsSide || cy >= regionCellsSide {
                    break
                }
                dumpCell(t, rl, cx, cy, fmt.Sprintf("step %d", step))
            }
        }
    }
}

// dumpCell prints one cell stack with its 4 neighbours.
func dumpCell(t *testing.T, rl *regionLayers, cx, cy int, tag string) {
    t.Helper()
    describe := func(x, y int) string {
        if x < 0 || y < 0 || x >= regionCellsSide ||
            y >= regionCellsSide {
            return "out"
        }
        idx := x*regionCellsSide + y
        off, cnt := int(rl.cellOff[idx]), int(rl.cellCnt[idx])
        if cnt == 0 {
            return "void"
        }
        var text strings.Builder
        for li := off; li < off+cnt; li++ {
            layer := rl.layers[li]
            area := "dry"
            if layer.h <= waterLevel {
                area = "WATER"
            }
            fmt.Fprintf(&text, "[h %d nswe %02x %s]", layer.h, layer.nswe,
                area)
        }

        return text.String()
    }
    t.Logf("%s cell (%d %d): %s | west: %s | east: %s | south: %s"+
        " | north: %s", tag, cx, cy, describe(cx, cy),
        describe(cx-1, cy), describe(cx+1, cy), describe(cx, cy-1),
        describe(cx, cy+1))
}
