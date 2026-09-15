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
    // dropped marks the island sheets smaller than the minimum: no
    // walk can reach them (their fill found no within-climb neighbour
    // anywhere), so the grid engine cannot reach them either.
    dropped []bool
    sizes   []int32
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

    return out
}

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
