// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

// sheets is the sheet decomposition of one region: the walkable
// layers partitioned into 2D manifold surfaces. A sheet never stacks
// over itself (the flood fill occupies every column at most once), so
// the rectangle decomposition of each sheet produces polygons whose
// footprint never self overlaps - the property the naive span import
// loses and the reason the Recast region build crashed on the real
// data (docs/recast_pathfinding.md).
type sheets struct {
    sheetOf []int32
    count   int
    // class marks the water sheets (any layer below the water level -
    // the flood never crosses the wetness boundary, so the class is
    // uniform per sheet by construction).
    class []uint8
    // dropped marks the sheets the build rejects: the islands smaller
    // than the minimum and the floating components no walk can reach
    // (dropIslandComponents below).
    dropped []bool
    // island marks the sheets dropped as unreachable floating
    // components (the stats separate them from the size drops).
    island []bool
    sizes  []int32
}

// sheet constants: 2048 buckets cover the geodata height range (the
// heights are multiples of 2, the buckets are 8 units wide).
const sheetBuckets = 2048

// assignSheets floods the sheets top down: the highest unassigned
// walkable layer seeds a new sheet, the fill moves to the neighbour
// column layers within the climb range of the same wetness class
// whenever the sheet does not already occupy the neighbour column.
// The NSWE walls are deliberately ignored here: the sheets are
// geometric surfaces, the walls become the portal spans of the
// polygon links (the link walk filters them afterwards).
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

// dropIslandComponents marks the sheets of the components no walk can
// ever reach. The sheet graph joins two sheets when a layer of one
// admits an engine step into an XY adjacent layer of the other: the
// paired NSWE walls of both sides plus the climb height rule, the
// exact canStep the grid search walks (the wetness split of the flood
// is ignored, because the engine steps between the shore and the
// water freely; the NSWE walls the flood ignores are honored here,
// because a wall a character cannot cross is no connection at all -
// the trunk helixes of the elven forest giant trees step within the
// climb range from the ground but wall every step of the way up). A
// component survives only when one of its sheets touches the region
// border, the seam where the neighbour region may continue the walk
// (the phase B stitching decides there); everything else is a
// floating island of the geodata structural encoding - the tree
// canopies over the elven forest, the trunk rings stacked hundreds of
// units apart, the deck skirts under the bridge - real geometry a
// character can never stand on. Keeping them renders the mother tree
// as a walkable ramp fused into the ground and curtains the floating
// village down to the lake.
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

// islandDeltas are the east and south cell offsets of the island
// adjacency walk (every cell pair is visited exactly once).
var islandDeltas = [2][2]int{{1, 0}, {0, 1}}

// floodNeighbours relaxes the neighbour columns of one flooded layer.
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

// sheetBucket is the height bucket of a layer (the buckets are 8
// units wide and cover [-8192, 8192)).
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
