// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "sync"
)

// NSWE wall flags of a cell layer, following the Mobius Cell.java bit
// layout: east bit 0, west bit 1, south bit 2, north bit 3. A set bit
// means the direction is open (walkable).
const (
    nsweEast  uint8 = 1 << 0
    nsweWest  uint8 = 1 << 1
    nsweSouth uint8 = 1 << 2
    nsweNorth uint8 = 1 << 3
    nsweAll   uint8 = nsweEast | nsweWest | nsweSouth | nsweNorth
)

// Layer is one walkable level of a geodata cell: the surface height in
// world units and the open wall directions.
type Layer struct {
    Height int16
    NSWE   uint8
}

// IsNorthOpen reports whether the cell can be left to the north.
func (l Layer) IsNorthOpen() bool { return l.NSWE&nsweNorth != 0 }

// IsSouthOpen reports whether the cell can be left to the south.
func (l Layer) IsSouthOpen() bool { return l.NSWE&nsweSouth != 0 }

// IsWestOpen reports whether the cell can be left to the west.
func (l Layer) IsWestOpen() bool { return l.NSWE&nsweWest != 0 }

// IsEastOpen reports whether the cell can be left to the east.
func (l Layer) IsEastOpen() bool { return l.NSWE&nsweEast != 0 }

// IsCompletelyOpen reports whether every wall of the cell is open.
func (l Layer) IsCompletelyOpen() bool { return l.NSWE == nsweAll }

// IsCompletelyBlocked reports whether every wall of the cell is closed.
// The Mobius MoveToLocation handler rejects any move whose target cell
// is completely blocked (isCompletelyBlocked of GeoEngine), so the
// pathfinder must never plan a step onto such a cell.
func (l Layer) IsCompletelyBlocked() bool { return l.NSWE == 0 }

// layerPool interns layers so identical (height, walls) pairs are stored
// once per engine and cells reference them by a small id. Real regions
// contain only a few thousand distinct layers, so this cuts the memory
// of the parsed data several fold (the original uses the same trick in
// its LayerFactory). The pool is shared by every region of the engine:
// parsing a region interns under the engine lock while searches of the
// already loaded regions read through get without it, so the pool
// guards itself.
type layerPool struct {
    mu     sync.RWMutex
    ids    map[layerKey]uint16
    layers []Layer
    // table is the fixed size lookup array of get: the intern writes
    // the slot once under the lock, the readers index it lock free
    // (the ids are stable, the entries are immutable after the
    // publish, see get).
    table [poolLayerCapacity]Layer
}

// layerKey is the deduplication key of a layer.
type layerKey struct {
    height int16
    nswe   uint8
}

// newLayerPool creates an empty layer pool.
func newLayerPool() *layerPool {
    return &layerPool{
        mu:     sync.RWMutex{},
        ids:    make(map[layerKey]uint16),
        layers: make([]Layer, 0, 4096),
    }
}

// intern returns the pool id of the layer, adding it on first use.
func (p *layerPool) intern(layer Layer) uint16 {
    key := layerKey{height: layer.Height, nswe: layer.NSWE}
    p.mu.Lock()
    defer p.mu.Unlock()
    if id, ok := p.ids[key]; ok {
        return id
    }
    if len(p.layers) >= poolLayerCapacity {
        panic("layer pool overflow: more than 65536 distinct layers " +
            "in one engine")
    }
    id := uint16(len(p.layers))
    p.layers = append(p.layers, layer)
    p.table[id] = layer
    p.ids[key] = id

    return id
}

// poolLayerCapacity is the fixed table size of the pool: the ids ride
// the uint16 references of the region cells, so the table never grows
// past 65536 entries (the same bound the uint16 cast of the append
// form carried implicitly).
const poolLayerCapacity = 1 << 16

// get returns the layer of a pool id. The table is a fixed array the
// intern fills once: every id the callers hold was interned before
// its region published (the parse happens under the engine cache
// lock), the entry never changes afterwards and the concurrent
// interns write other slots - the plain index answers without any
// synchronization (the old RWMutex read was the profile cost of the
// guard probes: two atomic RMW per layer lookup, millions per route).
func (p *layerPool) get(id uint16) Layer {
    return p.table[id]
}
