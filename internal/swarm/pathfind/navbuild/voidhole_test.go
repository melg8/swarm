// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "os"
    "strconv"
    "strings"
    "testing"
)

// TestVoidHoleScanSynthetic pins the hole classification on the
// synthetic world: a sea strip with a void hole over it classifies
// water, a void hole over the land stays land.
func TestVoidHoleScanSynthetic(t *testing.T) {
    // The west 300 cell columns ride at the sea floor (below the C1
    // water level), the rest is the land plateau; two void holes sit
    // inside each.
    grid := func(cx, cy int) []layerSpec {
        voidCol, voidRow := 100, 1024
        landVoidCol, landVoidRow := 1500, 1024
        if cx == voidCol && cy >= voidRow-4 && cy <= voidRow+4 {
            return []layerSpec{} // the true void over the sea
        }
        if cx == landVoidCol && cy >= landVoidRow-4 &&
            cy <= landVoidRow+4 {
            return []layerSpec{} // the true void over the land
        }
        if cx < 300 {
            return []layerSpec{{h: -4736, nswe: 0x0F}}
        }

        return []layerSpec{{h: -3504, nswe: 0x0F}}
    }
    report, err := ScanWaterHoles(writeRegionFile(t, grid), 18, 21)
    if err != nil {
        t.Fatal(err)
    }
    if report.VoidNbrs != 2*9 {
        t.Fatalf("void cells %d, want 18", report.VoidNbrs)
    }
    if len(report.Holes) != 2 {
        t.Fatalf("holes %d, want 2: %v", len(report.Holes), report.Holes)
    }
    water, land := report.Holes[0], report.Holes[1]
    if !water.Water || water.MaxFloor > waterLevel {
        t.Fatalf("the sea hole classifies land: %s", water.String())
    }
    if land.Water {
        t.Fatalf("the land hole classifies water: %s", land.String())
    }
}

// TestVoidHoleScanPack walks the real regions of the bay corridor
// (default) or the SWARM_VOID_REGIONS list and logs the hole map -
// the diagnosis round for the town leg strands. The probe skips when
// the geodata pack is absent.
func TestVoidHoleScanPack(t *testing.T) {
    geodataDir := "../../../../data/geodata"
    if _, err := os.Stat(geodataDir); err != nil {
        t.Skipf("the geodata pack is not present: %v", err)
    }
    regions := "16_21,17_21,18_21,19_21,16_22,17_22,18_22,19_22"
    if spec := os.Getenv("SWARM_VOID_REGIONS"); spec != "" {
        regions = spec
    }
    for _, spec := range strings.Split(regions, ",") {
        colText, rowText, found := strings.Cut(strings.TrimSpace(spec),
            "_")
        if !found {
            continue
        }
        col, err := strconv.Atoi(colText)
        if err != nil {
            continue
        }
        row, err := strconv.Atoi(rowText)
        if err != nil {
            continue
        }
        path := fmt.Sprintf("%s/%d_%d.l2j", geodataDir, col, row)
        data, err := os.ReadFile(path) //nolint:gosec // the fixed dir
        if err != nil {
            t.Logf("%d_%d: unreadable (%v)", col, row, err)

            continue
        }
        report, err := ScanWaterHoles(data, int16(col), int16(row))
        if err != nil {
            t.Logf("%d_%d: scan failed: %v", col, row, err)

            continue
        }
        waterHoles, waterCells := 0, 0
        for i, hole := range report.Holes {
            if i >= 6 {
                break // the log keeps the six biggest holes per region
            }
            t.Logf("%d_%d hole: %s", col, row, hole.String())
            if hole.Water {
                waterHoles++
                waterCells += hole.Cells
            }
        }
        t.Logf("%d_%d: %d void cells, %d holes, %d water holes"+
            " (%d cells)", col, row, report.VoidNbrs, len(report.Holes),
            waterHoles, waterCells)
    }
}
