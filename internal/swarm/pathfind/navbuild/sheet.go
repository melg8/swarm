// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

// sheets is the sheet decomposition of one region: the walkable layers
// partitioned into 2D manifold surfaces. A sheet never stacks over itself (the
// flood occupies every column once), so its rectangle footprints never self
// overlap - the property the naive span import loses (docs/navmesh.md).
type sheets struct {
    sheetOf []int32
    count   int
    // class marks the water sheets (the flood never crosses the wetness
    // boundary, so the class is uniform per sheet).
    class []uint8
    // dropped marks the sheets the build rejects: the islands smaller than the
    // minimum and the floating components no walk can reach.
    dropped []bool
    // island marks the sheets dropped as unreachable floating components (the
    // stats separate them from the size drops).
    island []bool
    sizes  []int32
}

// sheetBuckets covers the geodata height range with 8 unit wide buckets.
const sheetBuckets = 2048

// assignSheets floods the sheets top down: the highest unassigned walkable
// layer seeds a new sheet, the fill moves to the neighbour column layers within
// the climb range of the same wetness class, never occupying a column twice.
// The NSWE walls are ignored here - they become the portal spans of the links.
func assignSheets(rl *regionLayers, climb int32, minLayers int32,
) *sheets {
    layerCount := len(rl.layers)
    out := &sheets{
        sheetOf: make([]int32, layerCount),
        count:   0,
        class:   nil,
        dropped: nil,
        sizes:   nil,
    }
    for j := range out.sheetOf {
        out.sheetOf[j] = -1
    }

    buckets := make([][]uint32, sheetBuckets)
    for j, layer := range rl.layers {
        if layer.nswe == 0 {
            continue
        }
        bucket := sheetBucket(layer.h)
        buckets[bucket] = append(buckets[bucket], uint32(j))
    }

    stack := make([]uint32, 0, 1024)
    for bucket := sheetBuckets - 1; bucket >= 0; bucket-- {
        for _, seed := range buckets[bucket] {
            if out.sheetOf[seed] >= 0 {
                continue
            }
            sheet := int32(out.count)
            out.count++
            out.sheetOf[seed] = sheet
            stack = append(stack, seed)
            for len(stack) > 0 {
                cur := stack[len(stack)-1]
                stack = stack[:len(stack)-1]
                out.floodNeighbours(rl, cur, sheet, climb, &stack)
            }
        }
    }

    out.class = make([]uint8, out.count)
    out.sizes = make([]int32, out.count)
    out.dropped = make([]bool, out.count)
    out.island = make([]bool, out.count)
    for j, layer := range rl.layers {
        sheet := out.sheetOf[j]
        if sheet < 0 {
            continue
        }
        if layer.h < waterLevel {
            out.class[sheet] = 1
        }
        out.sizes[sheet]++
    }
    for s := range out.count {
        if out.sizes[s] < minLayers {
            out.dropped[s] = true
        }
    }
    out.dropIslandComponents(rl, climb)

    return out
}

// dropIslandComponents marks the sheets of the components no walk can ever
// reach: the sheet graph joins two sheets when a layer of one admits an
// engine step into an adjacent layer of the other (the paired NSWE walls
// plus the climb rule), and a component survives only when a sheet touches
// the region border, where the neighbour region may continue the walk. The
// rest are the floating canopies, trunks and deck skirts (docs/navmesh.md).
//
//nolint:cyclop,gocognit,funlen // the union find pass reads side by side
func (s *sheets) dropIslandComponents(rl *regionLayers, climb int32) {
    parent := make([]int32, s.count)
    for i := range parent {
        parent[i] = int32(i)
    }
    find := func(x int32) int32 {
        for parent[x] != x {
            parent[x] = parent[parent[x]]
            x = parent[x]
        }

        return x
    }
    union := func(a, b int32) {
        ra, rb := find(a), find(b)
        if ra != rb {
            parent[rb] = ra
        }
    }
    border := make([]bool, s.count)
    for cx := range regionCellsSide {
        for cy := range regionCellsSide {
            idx := cx*regionCellsSide + cy
            off := int(rl.cellOff[idx])
            cnt := int(rl.cellCnt[idx])
            for k := off; k < off+cnt; k++ {
                sa := s.sheetOf[k]
                if sa < 0 {
                    continue
                }
                if cx == 0 || cy == 0 ||
                    cx == regionCellsSide-1 || cy == regionCellsSide-1 {
                    border[sa] = true
                }
                for _, delta := range islandDeltas {
                    nx, ny := cx+delta[0], cy+delta[1]
                    if nx < 0 || ny < 0 || nx >= regionCellsSide ||
                        ny >= regionCellsSide {
                        continue
                    }
                    nIdx := nx*regionCellsSide + ny
                    nOff := int(rl.cellOff[nIdx])
                    nCnt := int(rl.cellCnt[nIdx])
                    for m := nOff; m < nOff+nCnt; m++ {
                        sb := s.sheetOf[m]
                        if sb < 0 || sb == sa {
                            continue
                        }
                        if !stepOpen(rl.layers[k], rl.layers[m],
                            int32(delta[0]), int32(delta[1]), climb) {
                            continue
                        }
                        union(sa, sb)
                    }
                }
            }
        }
    }
    componentBorder := make([]bool, s.count)
    for sheet := range s.count {
        if border[sheet] {
            componentBorder[find(int32(sheet))] = true
        }
    }
    for sheet := range s.count {
        if s.dropped[sheet] || componentBorder[find(int32(sheet))] {
            continue
        }
        s.dropped[sheet] = true
        s.island[sheet] = true
    }
}

// islandDeltas are the east and south offsets of the island adjacency walk, so
// every cell pair is visited once.
var islandDeltas = [2][2]int{{1, 0}, {0, 1}}

func (s *sheets) floodNeighbours(rl *regionLayers, cur uint32,
    sheet int32, climb int32, stack *[]uint32,
) {
    cellIdx := rl.cellIndexOf[cur]
    cx := int(cellIdx / regionCellsSide)
    cy := int(cellIdx % regionCellsSide)
    layer := rl.layers[cur]
    wet := layer.h < waterLevel
    for _, delta := range neighborDeltas {
        nx := cx + delta[0]
        ny := cy + delta[1]
        if nx < 0 || ny < 0 || nx >= regionCellsSide ||
            ny >= regionCellsSide {
            continue
        }
        nIdx := nx*regionCellsSide + ny
        off := int(rl.cellOff[nIdx])
        cnt := int(rl.cellCnt[nIdx])
        for k := off; k < off+cnt; k++ {
            if s.sheetOf[k] >= 0 || rl.layers[k].nswe == 0 {
                continue
            }
            if (rl.layers[k].h < waterLevel) != wet {
                continue
            }
            if abs16(rl.layers[k].h-layer.h) > climb {
                continue
            }
            // The sheet may occupy the neighbour column only once.
            if s.columnHolds(off, cnt, sheet) {
                continue
            }
            s.sheetOf[k] = sheet
            *stack = append(*stack, uint32(k))
        }
    }
}

// columnHolds reports whether the neighbour column already carries a
// layer of the sheet.
func (s *sheets) columnHolds(off, cnt int, sheet int32) bool {
    for m := off; m < off+cnt; m++ {
        if s.sheetOf[m] == sheet {
            return true
        }
    }

    return false
}

// neighborDeltas are the four cell axis neighbour offsets.
var neighborDeltas = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// sheetBucket is the height bucket of a layer (8 unit wide, [-8192, 8192)).
func sheetBucket(h int16) int {
    bucket := (int32(h) + 8192) / 8
    if bucket < 0 {
        return 0
    }
    if bucket >= sheetBuckets {
        return sheetBuckets - 1
    }

    return int(bucket)
}
