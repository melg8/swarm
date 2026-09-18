// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "fmt"
    "sort"
)

// VoidHole is one connected void cluster of a region: the cells the
// geodata carries no layer for. The hole class tells whether the
// neighbours sit over the water (the bay holes the town legs strand
// on) or over the land (the encoding holes a water fill must never
// touch).
type VoidHole struct {
    Cells int
    MinX  int32
    MinY  int32
    MaxX  int32
    MaxY  int32
    // MinFloor/MaxFloor are the height range of the neighbouring
    // non void layers within the sample radius (the fill height
    // evidence: water holes sample against the sea floor, land holes
    // against the cliffs around them).
    MinFloor  int16
    MaxFloor  int16
    WaterNbrs int
    LandNbrs  int
    // Water answers the classification: the water neighbours dominate
    // (>= 80 percent) and every sampled floor sits below the water
    // level.
    Water bool
}

// VoidReport summarizes the void cells of one region file.
type VoidReport struct {
    Col, Row int16
    VoidNbrs int // the void cell count before the clustering
    Holes    []VoidHole
}

// ScanWaterHoles parses one region file and reports the void cell
// clusters: the connected components of the cells with no layer,
// classified by their neighbour layers within the sample radius. The
// water holes are the fill candidates of the bay repair; the land
// holes stay untouched.
func ScanWaterHoles(data []byte, col, row int16,
) (*VoidReport, error) {
    rl, err := extractRegion(data, col, row,
        DefaultOptions().DedupDelta)
    if err != nil {
        return nil, err
    }

    // The void mask and the BFS label pass over the 4 connected cell
    // grid.
    const side = regionCellsSide
    voidCell := make([]bool, side*side)
    voids := 0
    for idx := range voidCell {
        if rl.cellCnt[idx] == 0 {
            voidCell[idx] = true
            voids++
        }
    }
    report := &VoidReport{Col: col, Row: row, VoidNbrs: voids}
    label := make([]int32, side*side)
    holeAt := make([]int32, 0, 64)
    for start := range voidCell {
        if !voidCell[start] || label[start] != 0 {
            continue
        }
        hole := VoidHole{
            MinFloor: 32767,
            MaxFloor: -32768,
            MinX:     1 << 30, MinY: 1 << 30,
        }
        holeAt = append(holeAt[:0], int32(start))
        label[start] = int32(len(report.Holes) + 1)
        for head := 0; head < len(holeAt); head++ {
            idx := holeAt[head]
            cx, cy := int(idx/side), int(idx%side)
            hole.Cells++
            if cx < int(hole.MinX) {
                hole.MinX = int32(cx)
            }
            if cy < int(hole.MinY) {
                hole.MinY = int32(cy)
            }
            if cx > int(hole.MaxX) {
                hole.MaxX = int32(cx)
            }
            if cy > int(hole.MaxY) {
                hole.MaxY = int32(cy)
            }
            for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1},
                {0, 1}} {
                nx, ny := cx+d[0], cy+d[1]
                if nx < 0 || ny < 0 || nx >= side || ny >= side {
                    continue
                }
                nIdx := nx*side + ny
                if voidCell[nIdx] {
                    if label[nIdx] == 0 {
                        label[nIdx] = label[start]
                        holeAt = append(holeAt, int32(nIdx))
                    }

                    continue
                }
                // A non void neighbour: sample its layers for the
                // floor evidence.
                off, cnt := int(rl.cellOff[nIdx]), int(rl.cellCnt[nIdx])
                for li := off; li < off+cnt; li++ {
                    layer := rl.layers[li]
                    if layer.h < hole.MinFloor {
                        hole.MinFloor = layer.h
                    }
                    if layer.h > hole.MaxFloor {
                        hole.MaxFloor = layer.h
                    }
                    if layer.h <= waterLevel {
                        hole.WaterNbrs++
                    } else {
                        hole.LandNbrs++
                    }
                }
            }
        }
        sampled := hole.WaterNbrs + hole.LandNbrs
        hole.Water = sampled > 0 &&
            hole.WaterNbrs*5 >= sampled*4 && // >= 80 percent
            hole.MaxFloor <= waterLevel
        report.Holes = append(report.Holes, hole)
    }
    sort.Slice(report.Holes, func(i, j int) bool {
        return report.Holes[i].Cells > report.Holes[j].Cells
    })

    return report, nil
}

// String renders one hole line of the report log.
func (h *VoidHole) String() string {
    kind := "land"
    if h.Water {
        kind = "WATER"
    }

    return fmt.Sprintf("%d cells bbox %d_%d..%d_%d floors %d..%d"+
        " nbrs(w%d/l%d) %s", h.Cells, h.MinX, h.MinY, h.MaxX, h.MaxY,
        h.MinFloor, h.MaxFloor, h.WaterNbrs, h.LandNbrs, kind)
}
