// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"encoding/binary"
	"fmt"
)

// Block kinds of the l2j geodata format (Mobius IBlock.TYPE_*).
type blockKind uint8

const (
	blockFlat       blockKind = 0
	blockComplex    blockKind = 1
	blockMultilayer blockKind = 2
	// maxLayersPerCell bounds the layer count of a multilayer cell, the
	// same corruption guard the Mobius MultilayerBlock uses (1..125).
	maxLayersPerCell = 125
)

// Region is one parsed X_Y.l2j geodata region: 2048x2048 cells, each
// holding one or more walkable layers.
//
// The cells are stored in file order: span index = block*64 + cell where
// block = (localX/8)*256 + localY/8 and cell = (localX%8)*8 + localY%8,
// exactly the indexing of the Mobius Region.getBlock/ComplexBlock. Every
// span packs a refs offset (low 24 bits) and a layer count (high 8
// bits). The refs slice is deduplicated for single layer cells, so it
// holds only the distinct layers of the region plus the multilayer
// stacks.
type Region struct {
	col, row int16
	kinds    [blocksPerRegion]blockKind
	spans    [cellsPerRegion]uint32
	refs     []uint16
	refIndex map[uint16]uint32
	pool     *layerPool
}

// parseRegion parses the raw bytes of one region file. The format has no
// header (the Mobius C1 GeoEngine reads the blocks straight away, little
// endian) and must be consumed exactly: 65536 blocks, a flat block is
// 1+2 bytes, a complex block 1+128 bytes and a multilayer block
// 1 + 64*(1 + 2*layers) bytes.
func parseRegion(data []byte, key RegionKey, pool *layerPool) (*Region, error) {
	region := &Region{
		col:      key.Col,
		row:      key.Row,
		refs:     make([]uint16, 0, 4096),
		refIndex: make(map[uint16]uint32),
		pool:     pool,
	}

	position := 0
	for block := 0; block < blocksPerRegion; block++ {
		if position >= len(data) {
			return nil, fmt.Errorf("region %d_%d truncated at block %d",
				key.Col, key.Row, block)
		}
		kind := blockKind(data[position])
		position++

		var err error
		switch kind {
		case blockFlat:
			err = region.parseFlat(data, &position, block)
		case blockComplex:
			err = region.parseComplex(data, &position, block)
		case blockMultilayer:
			err = region.parseMultilayer(data, &position, block)
		default:
			err = fmt.Errorf("invalid block type %d at offset %d",
				kind, position-1)
		}
		if err != nil {
			return nil, fmt.Errorf("region %d_%d: %w", key.Col, key.Row, err)
		}
	}
	if position != len(data) {
		return nil, fmt.Errorf("region %d_%d: %d trailing bytes",
			key.Col, key.Row, len(data)-position)
	}

	return region, nil
}

// parseFlat reads the 2 byte little endian height of a flat block: one
// layer shared by all 64 cells, every wall open.
func (r *Region) parseFlat(data []byte, position *int, block int) error {
	if *position+2 > len(data) {
		return fmt.Errorf("truncated flat block at offset %d", *position-1)
	}
	height := int16(binary.LittleEndian.Uint16(data[*position:]))
	*position += 2
	r.addUniformBlock(block, blockFlat, height, nsweAll)

	return nil
}

// parseComplex reads the 64 per cell layer words of a complex block.
func (r *Region) parseComplex(data []byte, position *int, block int) error {
	if *position+2*cellsPerBlock > len(data) {
		return fmt.Errorf("truncated complex block at offset %d", *position-1)
	}
	r.kinds[block] = blockComplex
	for cell := 0; cell < cellsPerBlock; cell++ {
		info := binary.LittleEndian.Uint16(data[*position:])
		*position += 2
		r.setSingleLayer(block, cell, decodeCell(info))
	}

	return nil
}

// parseMultilayer reads the 64 cells of a multilayer block, each with a
// leading layer count byte followed by count layer words.
func (r *Region) parseMultilayer(data []byte, position *int, block int) error {
	r.kinds[block] = blockMultilayer
	for cell := 0; cell < cellsPerBlock; cell++ {
		if *position >= len(data) {
			return fmt.Errorf(
				"truncated multilayer block at offset %d", *position-1)
		}
		count := int(data[*position])
		*position++
		if count == 0 || count > maxLayersPerCell {
			return fmt.Errorf("invalid layer count %d at offset %d",
				count, *position-1)
		}
		if *position+count*2 > len(data) {
			return fmt.Errorf("truncated multilayer cell at offset %d",
				*position-1)
		}
		offset := len(r.refs)
		if offset >= 1<<spanOffsetBits {
			return fmt.Errorf("region exceeds the span offset capacity")
		}
		for j := 0; j < count; j++ {
			info := binary.LittleEndian.Uint16(data[*position:])
			*position += 2
			r.refs = append(r.refs, r.pool.intern(decodeCell(info)))
		}
		r.spans[cellSpanIndex(block, cell)] = packSpan(offset, count)
	}

	return nil
}

// addUniformBlock stores one layer for all 64 cells of a block.
func (r *Region) addUniformBlock(block int, kind blockKind, height int16, nswe uint8) {
	r.kinds[block] = kind
	layer := Layer{Height: height, NSWE: nswe}
	for cell := 0; cell < cellsPerBlock; cell++ {
		r.setSingleLayer(block, cell, layer)
	}
}

// setSingleLayer stores the single layer of one cell, reusing the refs
// slot when the identical layer id was already appended.
func (r *Region) setSingleLayer(block, cell int, layer Layer) {
	id := r.pool.intern(layer)
	offset, ok := r.refIndex[id]
	if !ok {
		offset = uint32(len(r.refs))
		r.refs = append(r.refs, id)
		r.refIndex[id] = offset
	}
	r.spans[cellSpanIndex(block, cell)] = packSpan(int(offset), 1)
}

// Span packing: the low 24 bits hold the refs offset, the high 8 bits
// the layer count.
const spanOffsetBits = 24

// packSpan merges a refs offset and a layer count into one span word.
func packSpan(offset, count int) uint32 {
	return uint32(offset)<<8 | uint32(count)
}

// unpackSpan splits a span word into refs offset and layer count.
func unpackSpan(span uint32) (offset, count int) {
	return int(span >> 8), int(span & 0xFF)
}

// cellSpanIndex converts a block and cell position to the span index.
func cellSpanIndex(block, cell int) int {
	return block*cellsPerBlock + cell
}

// blockIndex converts region local cell coordinates to the block index
// (Mobius Region.getBlock: (localX/8 % 256)*256 + localY/8 % 256).
func blockIndex(local Point) int {
	return (int(local.X)/cellsPerBlockSide)*blocksPerRegionSide +
		int(local.Y)/cellsPerBlockSide
}

// cellIndexInBlock converts region local cell coordinates to the cell
// position inside its block (Mobius ComplexBlock: (localX%8)*8+localY%8).
func cellIndexInBlock(local Point) int {
	return (int(local.X)%cellsPerBlockSide)*cellsPerBlockSide +
		int(local.Y)%cellsPerBlockSide
}

// Layers returns a fresh slice with the layer stack of a cell in region
// local coordinates (0..cellsPerRegionSide-1 per axis). The hot search
// path uses ClosestLayer instead, this is for tests and debugging.
func (r *Region) Layers(local Point) []Layer {
	span := r.spans[cellSpanIndex(blockIndex(local), cellIndexInBlock(local))]
	offset, count := unpackSpan(span)
	layers := make([]Layer, count)
	for i := range layers {
		layers[i] = r.pool.get(r.refs[offset+i])
	}

	return layers
}

// ClosestLayer returns the layer of the cell whose height is the closest
// to z (ties keep the first, like the original GetClosestLayer and the
// Mobius MultilayerBlock.getNearestLayer). The second return is false
// for cells without layers, which valid files never contain.
func (r *Region) ClosestLayer(local Point, z int16) (Layer, bool) {
	span := r.spans[cellSpanIndex(blockIndex(local), cellIndexInBlock(local))]
	offset, count := unpackSpan(span)
	best := r.pool.get(r.refs[offset])
	bestDelta := heightDelta(best.Height, z)
	for i := 1; i < count; i++ {
		layer := r.pool.get(r.refs[offset+i])
		if delta := heightDelta(layer.Height, z); delta < bestDelta {
			best, bestDelta = layer, delta
		}
	}

	return best, true
}

// heightDelta is the absolute height distance of two layers.
func heightDelta(a, b int16) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}

	return d
}

// decodeCell splits one encoded cell word into height and walls. The
// height is the upper 15 bits arithmetic shifted right by one, keeping
// the sign (the Mobius (short)(data & 0xFFF0) >> 1).
func decodeCell(info uint16) Layer {
	return Layer{
		Height: int16(info&0xFFF0) >> 1,
		NSWE:   uint8(info) & 0x0F,
	}
}
