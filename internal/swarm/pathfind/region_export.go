// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

// ParseRegionData parses the raw bytes of one geodata region file with
// a private layer pool. The offline navmesh builder
// (internal/swarm/pathfind/navbuild) walks every cell of the pack
// through it without going through the engine region cache: a builder
// pass holds one region at a time and never shares the pool.
func ParseRegionData(data []byte, key RegionKey) (*Region, error) {
    return parseRegion(data, key, newLayerPool())
}

// LayerStack fills the layer stack of the region local cell (0..
// cellsPerRegionSide-1 per axis) into buf and returns the filled slice
// - buf itself when it has room, a grown copy otherwise. The values
// are copies of the pooled layers, so the caller owns the slice. The
// stack keeps the file order of the layers (ascending height). The
// builder iterates whole regions this way without allocating a stack
// per cell; the hot search path keeps using ClosestLayer.
func (r *Region) LayerStack(local Point, buf []Layer) []Layer {
    span := r.spans[cellSpanIndex(blockIndex(local), cellIndexInBlock(local))]
    offset, count := unpackSpan(span)
    if cap(buf) < count {
        buf = make([]Layer, count)
    }
    stack := buf[:count]
    for i := range stack {
        stack[i] = r.pool.get(r.refs[offset+i])
    }

    return stack
}
