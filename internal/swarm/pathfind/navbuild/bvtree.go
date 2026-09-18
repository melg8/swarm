// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "sort"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// heightQuantum is the BVTree height quantization: the quantized
// height axis counts 16 unit steps above the -8192 floor (the cell
// size quantization of the horizontal axes, the Detour convention of
// using cs for every tree dimension).
const heightQuantum = 16

// heightQuantFloor is the quantization anchor of the height axis.
const heightQuantFloor = -8192

// bvItem is one polygon of the tree build: the quantized bounds and
// the polygon index.
type bvItem struct {
    bmin [3]uint16
    bmax [3]uint16
    poly int32
}

// buildBVTree builds the bounding volume tree over the polygon
// bounds - the port of the Detour dtCreateBVTree: the median split
// along the longest axis, the leaf escape encoded as a negative node
// index. The tree holds exactly 2N-1 nodes for N polygons.
func buildBVTree(tile *navmesh.Tile, polys []rectPoly,
) []navmesh.BVNode {
    if len(polys) == 0 {
        return nil
    }
    items := make([]bvItem, len(polys))
    for i, poly := range polys {
        items[i] = bvItemOf(tile, i, &poly)
    }
    nodes := make([]navmesh.BVNode, 2*len(polys))
    cursor := 0
    subdivide(items, 0, len(items), &cursor, nodes)

    return nodes[:cursor]
}

// bvItemOf quantizes the bounds of one polygon: the horizontal axes
// in cell units relative to the region anchor, the height axis in 16
// unit steps above the floor.
func bvItemOf(tile *navmesh.Tile, i int, poly *rectPoly) bvItem {
    hMin, hMax := poly.h00, poly.h00
    for _, h := range []int16{poly.h10, poly.h01, poly.h11} {
        if h < hMin {
            hMin = h
        }
        if h > hMax {
            hMax = h
        }
    }
    item := bvItem{
        bmin: [3]uint16{
            uint16(poly.x0),
            quantHeight(hMin),
            uint16(poly.y0),
        },
        bmax: [3]uint16{
            uint16(poly.x1),
            quantHeight(hMax) + 1,
            uint16(poly.y1),
        },
        poly: int32(i),
    }
    _ = tile

    return item
}

// quantHeight converts a world height into the quantized tree units.
func quantHeight(h int16) uint16 {
    steps := (int32(h) - heightQuantFloor) / heightQuantum
    if steps < 0 {
        return 0
    }
    if steps > 65534 {
        return 65534
    }

    return uint16(steps)
}

// subdivide recursively splits the item range into the tree nodes.
func subdivide(items []bvItem, imin, imax int, cursor *int,
    nodes []navmesh.BVNode,
) {
    inum := imax - imin
    if inum <= 0 {
        return
    }
    node := &nodes[*cursor]
    *cursor++
    if inum == 1 {
        node.BMin = items[imin].bmin
        node.BMax = items[imin].bmax
        node.I = items[imin].poly

        return
    }
    calcExtends(items, imin, imax, node)
    axis := longestAxis(node)
    sort.Slice(items[imin:imax], func(i, j int) bool {
        return items[imin+i].bmin[axis] < items[imin+j].bmin[axis]
    })
    isplit := imin + inum/2
    before := *cursor
    subdivide(items, imin, isplit, cursor, nodes)
    subdivide(items, isplit, imax, cursor, nodes)
    node.I = -int32(*cursor - before)
}

// calcExtends fills the node bounds with the union of the item range.
func calcExtends(items []bvItem, imin, imax int, node *navmesh.BVNode) {
    node.BMin = items[imin].bmin
    node.BMax = items[imin].bmax
    for i := imin + 1; i < imax; i++ {
        for a := range 3 {
            if items[i].bmin[a] < node.BMin[a] {
                node.BMin[a] = items[i].bmin[a]
            }
            if items[i].bmax[a] > node.BMax[a] {
                node.BMax[a] = items[i].bmax[a]
            }
        }
    }
}

// longestAxis picks the widest axis of the node bounds.
func longestAxis(node *navmesh.BVNode) int {
    axis := 0
    maxSpan := spanOf(node, 0)
    for a := 1; a < 3; a++ {
        if span := spanOf(node, a); span > maxSpan {
            axis = a
            maxSpan = span
        }
    }

    return axis
}

// spanOf is the bound span of one axis.
func spanOf(node *navmesh.BVNode, axis int) uint16 {
    return node.BMax[axis] - node.BMin[axis]
}
