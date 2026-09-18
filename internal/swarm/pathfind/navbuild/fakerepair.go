// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

// The fake cell repair: the C1 geodata pack carries whole areas with
// no recorded data - the l2j uninitialized filler of one layer at
// height 0 with the walls fully open (the east halves of the column
// 17 regions, the whole 17_23). The filler builds into a floating
// plateau 3.8k units above the sea floor with cliffs against every
// real neighbour: the town legs strand on it (the Gludio Gludin bay
// crossing, the Gludin town cells inside the filler area).
//
// The repair rebuilds the filler from the nearest real cells: the
// multi source BFS walks the fake area from the real land and the
// real water borders, every fake cell blends the two nearest source
// heights by the BFS distances (the fake bay turns into the sea floor
// water the swim route needs, the gaps between the real land patches
// turn into the connecting ground), and the polish pass pulls the
// residual over climb steps together until the filled surface walks.

// fakeLayer reports the l2j uninitialized filler cell: exactly one
// layer, height 0, every wall open. The pattern holds 99.97 percent
// of the zero height cells of the pack (the rest are the rare real
// zero height terrain the repair leaves alone).
func fakeLayer(rl *regionLayers, idx int) bool {
    off, cnt := int(rl.cellOff[idx]), int(rl.cellCnt[idx])
    if cnt != 1 {
        return false
    }
    layer := rl.layers[off]

    return layer.h == 0 && layer.nswe == 0x0F
}

// repairFakeCells replaces the filler cells with the synthesized
// surface of the nearest real neighbours (the class doc carries the
// rule). The pass mutates the layer pool in place and answers the
// filled cell count.
func repairFakeCells(rl *regionLayers, climb int32) int {
    const side = regionCellsSide
    cells := side * side
    fake := make([]bool, cells)
    fakes := 0
    for idx := range fake {
        if fakeLayer(rl, idx) {
            fake[idx] = true
            fakes++
        }
    }
    if fakes == 0 {
        return 0
    }

    // The two multi source BFS passes: the land sources (the real
    // cells above the water level) and the water sources (the real
    // cells at or below it). Every fake cell records the distance and
    // the seed height of the closest source of each class; the seed
    // order is the cell index order, the queue is FIFO - the fill is
    // deterministic.
    type reach struct {
        dist int32
        h    int16
        set  bool
    }
    fill := func(isSource func(idx int) bool) []reach {
        dist := make([]reach, cells)
        queue := make([]int32, 0, cells)
        for idx := range fake {
            if fake[idx] || !isSource(idx) {
                continue
            }
            dist[idx] = reach{dist: 0, h: rl.layers[rl.cellOff[idx]].h,
                set: true}
            queue = append(queue, int32(idx))
        }
        for head := 0; head < len(queue); head++ {
            idx := queue[head]
            cur := dist[idx]
            cx, cy := int(idx)/side, int(idx)%side
            for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1},
                {0, 1}} {
                nx, ny := cx+d[0], cy+d[1]
                if nx < 0 || ny < 0 || nx >= side || ny >= side {
                    continue
                }
                nIdx := nx*side + ny
                if !fake[nIdx] || dist[nIdx].set {
                    continue
                }
                dist[nIdx] = reach{dist: cur.dist + 1, h: cur.h,
                    set: true}
                queue = append(queue, int32(nIdx))
            }
        }

        return dist
    }
    land := fill(func(idx int) bool {
        return rl.layers[rl.cellOff[idx]].h > waterLevel
    })
    water := fill(func(idx int) bool {
        return rl.layers[rl.cellOff[idx]].h <= waterLevel
    })

    // The blend: the fake cell takes the inverse distance weighted
    // mix of the two source heights (or the single reachable source).
    filledCount := 0
    for idx := range fake {
        if !fake[idx] {
            continue
        }
        var h int16
        switch {
        case land[idx].set && water[idx].set:
            dl, dw := float64(land[idx].dist), float64(water[idx].dist)
            blend := dw / (dl + dw)
            raw := float64(water[idx].h) +
                blend*float64(int32(land[idx].h)-int32(water[idx].h))
            h = int16(raw)
        case land[idx].set:
            h = land[idx].h
        case water[idx].set:
            h = water[idx].h
        default:
            continue // no real source in the region: the cell stays
        }
        rl.layers[rl.cellOff[idx]] = cellLayer{h: h, nswe: 0x0F}
        filledCount++
    }

    // The polish: the pair pull over the filled cells away from the
    // real anchors - when two filled cells disagree by more than the
    // climb increase, both move halfway together unless either is an
    // anchor (a filled cell with a real neighbour keeps the boundary
    // height the blend gave it). The walk needs the increase rule
    // only; the pass repeats while the over climb pairs remain (the
    // fixed cap keeps the pass bounded; the residual cliffs report
    // through the build stats).
    filled := make([]bool, cells)
    anchored := make([]bool, cells)
    for idx := range fake {
        filled[idx] = fake[idx] && (land[idx].set || water[idx].set)
    }
    for cx := range side {
        for cy := range side {
            idx := cx*side + cy
            if !filled[idx] {
                continue
            }
            for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1},
                {0, 1}} {
                nx, ny := cx+d[0], cy+d[1]
                if nx < 0 || ny < 0 || nx >= side || ny >= side {
                    continue
                }
                if !fake[nx*side+ny] {
                    anchored[idx] = true

                    break
                }
            }
        }
    }
    heightOf := func(idx int) int32 {
        return int32(rl.layers[rl.cellOff[idx]].h)
    }
    setHeight := func(idx int, h int32) {
        rl.layers[rl.cellOff[idx]].h = int16(h)
    }
    for range 64 {
        moves := 0
        for cx := range side {
            for cy := range side {
                idx := cx*side + cy
                if !filled[idx] || anchored[idx] {
                    continue
                }
                for _, d := range [2][2]int{{1, 0}, {0, 1}} {
                    nx, ny := cx+d[0], cy+d[1]
                    if nx >= side || ny >= side {
                        continue
                    }
                    nIdx := nx*side + ny
                    if !filled[nIdx] || anchored[nIdx] {
                        continue
                    }
                    a, b := heightOf(idx), heightOf(nIdx)
                    if a-b > climb {
                        mid := (a + b) / 2
                        setHeight(idx, mid)
                        setHeight(nIdx, mid)
                        moves++
                    }
                }
            }
        }
        if moves == 0 {
            break
        }
    }

    return filledCount
}
