// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package navbuild builds navigation mesh tiles from the l2j geodata
// region files (docs/navmesh.md): the walkable cell layers are
// partitioned into 2D manifold sheets, every sheet decomposes into
// rectangle polygons with exact corner heights, the polygon links
// carry the open NSWE portal spans of the shared edges and the tiles
// serialize into the format of internal/swarm/pathfind/navmesh.
//
// The pipeline replaces the Recast build of the research round
// (docs/recast_pathfinding.md): the geodata is already an exact voxel
// surface, so the rasterization half of Recast has nothing to
// contribute, while the region/contour half fights the stacked
// surfaces the sheet decomposition here resolves directly.
package navbuild

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// regionCellsSide is the cell count of one region side (2048), the
// same grid as internal/swarm/pathfind (geometry.go).
const regionCellsSide = 2048

// waterLevel is the C1 water surface: the layers below it are the
// swim areas (pathfind waterLevel).
const waterLevel = int16(-3780)

// cellLayer is one l2j cell layer: the surface height and the NSWE
// wall flags (a set bit is an open direction, 0 is fully blocked).
type cellLayer struct {
    h    int16
    nswe uint8
}

// regionLayers is the flat layer store of one region: per cell a
// contiguous slice of the layer pool, and per layer instance the
// owning cell index (the flood fill of the sheet decomposition walks
// both directions). The cell index is x-major
// (cx*regionCellsSide + cy).
type regionLayers struct {
    col, row    int16
    cellOff     []uint32
    cellCnt     []uint16
    cellIndexOf []int32
    layers      []cellLayer
}

// extractRegion parses the region file through the pathfind parser
// and flattens the cell stacks into the builder layout: the dedup of
// the within-delta duplicate layers runs during the copy (the same
// rule the research converter used - the l2j multilayer cells carry
// the same surface twice with a small jitter, and no real walkable
// geometry stacks two surfaces 32 units apart in one cell).
func extractRegion(
    data []byte, col, row int16, dedupDelta int32,
) (*regionLayers, error) {
    region, err := pathfind.ParseRegionData(data,
        pathfind.RegionKey{Col: col, Row: row})
    if err != nil {
        return nil, fmt.Errorf("parse region %d_%d: %w", col, row, err)
    }

    cells := regionCellsSide * regionCellsSide
    flat := &regionLayers{
        col:         col,
        row:         row,
        cellOff:     make([]uint32, cells),
        cellCnt:     make([]uint16, cells),
        cellIndexOf: make([]int32, 0, cells*2),
        layers:      make([]cellLayer, 0, cells*2),
    }
    stack := make([]pathfind.Layer, 0, 8)
    for cx := range regionCellsSide {
        for cy := range regionCellsSide {
            stack = region.LayerStack(pathfind.Point{
                X: int32(cx), Y: int32(cy)}, stack[:0])
            idx := cx*regionCellsSide + cy
            flat.cellOff[idx] = uint32(len(flat.layers))
            kept := int16(0)
            count := 0
            for _, layer := range stack {
                if count > 0 &&
                    abs16(layer.Height-kept) <= dedupDelta {
                    // The within-delta duplicate: keep the higher
                    // surface (the one the character stands on).
                    if layer.Height > kept {
                        kept = layer.Height
                        flat.layers[len(flat.layers)-1] = cellLayer{
                            h: kept, nswe: layer.NSWE}
                    }

                    continue
                }
                flat.layers = append(flat.layers, cellLayer{
                    h: layer.Height, nswe: layer.NSWE})
                flat.cellIndexOf = append(flat.cellIndexOf, int32(idx))
                kept = layer.Height
                count++
            }
            flat.cellCnt[idx] = uint16(count)
        }
    }

    return flat, nil
}

// abs16 is the absolute value of an int16 as int32.
func abs16(a int16) int32 {
    d := int32(a)
    if d < 0 {
        return -d
    }

    return d
}
