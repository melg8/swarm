// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "os"
    "testing"
)

// TestEastSeamDump dumps the raw geodata cells across the 19_22
// 20_22 border around the Giran strand cell (the world x 0, the
// local y 1024): the seam the mesh build stitches and the climb rule
// answers.
func TestEastSeamDump(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    west := loadRegionForDump(t, geodataDir, 19, 22)
    east := loadRegionForDump(t, geodataDir, 20, 22)
    if west == nil || east == nil {
        t.Fatal("the seam regions fail to load")
    }
    t.Logf("the 19_22 east edge (x 2032..2047) and the 20_22 west" +
        " edge (x 0..15) around y 1024:")
    for y := 1016; y <= 1032; y++ {
        westText, eastText := "", ""
        for x := 2040; x < 2048; x++ {
            westText += " " + layerText(west, x, y)
        }
        for x := 0; x < 8; x++ {
            eastText += " " + layerText(east, x, y)
        }
        t.Logf("y %4d | west:%s | east:%s", y, westText, eastText)
    }
}

// loadRegionForDump parses one region file for the dump probes.
func loadRegionForDump(t *testing.T, geodataDir string, col, row int16,
) *regionLayers {
    t.Helper()
    path := fmt.Sprintf("%s/%d_%d.l2j", geodataDir, col, row)
    data, err := os.ReadFile(path) //nolint:gosec // the fixed dir
    if err != nil {
        t.Fatalf("region %d_%d: %v", col, row, err)
    }
    rl, err := extractRegion(data, col, row, DefaultOptions().DedupDelta)
    if err != nil {
        t.Fatalf("region %d_%d: %v", col, row, err)
    }

    return rl
}

// layerText renders one cell stack compactly.
func layerText(rl *regionLayers, x, y int) string {
    idx := x*regionCellsSide + y
    off, cnt := int(rl.cellOff[idx]), int(rl.cellCnt[idx])
    if cnt == 0 {
        return "void"
    }
    text := ""
    for li := off; li < off+cnt; li++ {
        layer := rl.layers[li]
        text += fmt.Sprintf("%d/%02x", layer.h, layer.nswe)
    }

    return text
}
