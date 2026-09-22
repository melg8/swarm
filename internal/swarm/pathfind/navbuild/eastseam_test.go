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

// TestEastSeamDump dumps the raw geodata cells across the 19_22
// 20_22 border around the Giran strand cell (the world x 0, the
// local y 1024): the seam the mesh build stitches and the climb rule
// answers.
func TestEastSeamDump(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    west := loadRegionForDump(t, 19)
    east := loadRegionForDump(t, 20)
    if west == nil || east == nil {
        t.Fatal("the seam regions fail to load")
    }
    t.Logf("the 19_22 east edge (x 2032..2047) and the 20_22 west" +
        " edge (x 0..15) around y 1024:")
    for y := 1016; y <= 1032; y++ {
        westParts := make([]string, 0, 8)
        for x := 2040; x < 2048; x++ {
            westParts = append(westParts, " "+layerText(west, x, y))
        }
        eastParts := make([]string, 0, 8)
        for x := range 8 {
            eastParts = append(eastParts, " "+layerText(east, x, y))
        }
        t.Logf("y %4d | west:%s | east:%s", y,
            strings.Join(westParts, ""), strings.Join(eastParts, ""))
    }
}

// loadRegionForDump parses one region file for the dump probes.
func loadRegionForDump(t *testing.T, col int16,
) *regionLayers {
    t.Helper()
    path := fmt.Sprintf("../../../../data/geodata/%d_%d.l2j", col, 22)
    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("region %d_22: %v", col, err)
    }
    rl, err := extractRegion(data, col, 22, DefaultOptions().DedupDelta, 0)
    if err != nil {
        t.Fatalf("region %d_22: %v", col, err)
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
    var text strings.Builder
    for li := off; li < off+cnt; li++ {
        layer := rl.layers[li]
        fmt.Fprintf(&text, "%d/%02x", layer.h, layer.nswe)
    }

    return text.String()
}
