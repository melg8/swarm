// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

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

// layerPool interns layers so identical (height, walls) pairs are stored
// once per engine and cells reference them by a small id. Real regions
// contain only a few thousand distinct layers, so this cuts the memory
// of the parsed data several fold (the original uses the same trick in
// its LayerFactory).
type layerPool struct {
	ids    map[layerKey]uint16
	layers []Layer
}

// layerKey is the deduplication key of a layer.
type layerKey struct {
	height int16
	nswe   uint8
}

// newLayerPool creates an empty layer pool.
func newLayerPool() *layerPool {
	return &layerPool{
		ids:    make(map[layerKey]uint16),
		layers: make([]Layer, 0, 4096),
	}
}

// intern returns the pool id of the layer, adding it on first use.
func (p *layerPool) intern(layer Layer) uint16 {
	key := layerKey{height: layer.Height, nswe: layer.NSWE}
	if id, ok := p.ids[key]; ok {
		return id
	}
	id := uint16(len(p.layers))
	p.layers = append(p.layers, layer)
	p.ids[key] = id

	return id
}

// get returns the layer of a pool id.
func (p *layerPool) get(id uint16) Layer {
	return p.layers[id]
}
