// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// blockSpec describes one geodata block of a synthetic region.
type blockSpec struct {
	kind  blockKind
	cells [cellsPerBlock]Layer
	// stacks holds the multilayer cell stacks (used when kind is
	// blockMultilayer, one entry per cell).
	stacks [cellsPerBlock][]Layer
}

// regionSpec is a 256x256 grid of block specs, encoded by writeRegion.
type regionSpec struct {
	blocks [blocksPerRegionSide][blocksPerRegionSide]blockSpec
}

// testRegionCol/Row is the region the synthetic worlds live in.
const (
	testRegionCol = 22
	testRegionRow = 22
)

// encodeLayer converts a layer to the wire format cell word: the height
// shifted left by one plus the wall flags (Mobius setNearestNswe).
func encodeLayer(layer Layer) uint16 {
	return uint16(layer.Height)<<1 | uint16(layer.NSWE&0x0F)
}

// encode writes the region spec in the l2j little endian format: blocks
// in file order, a flat block is type 0 + height, a complex block type 1
// + 64 cell words, a multilayer block type 2 + per cell count + words.
func (s *regionSpec) encode() []byte {
	data := make([]byte, 0, blocksPerRegion*(1+2*cellsPerBlock))
	for bx := 0; bx < blocksPerRegionSide; bx++ {
		for by := 0; by < blocksPerRegionSide; by++ {
			spec := &s.blocks[bx][by]
			switch spec.kind {
			case blockFlat:
				// A flat block stores the raw height, no encoding.
				data = append(data, byte(blockFlat))
				data = appendRawWord(data, uint16(spec.cells[0].Height))
			case blockComplex:
				data = append(data, byte(blockComplex))
				for _, layer := range spec.cells {
					data = appendCellWord(data, layer)
				}
			case blockMultilayer:
				data = append(data, byte(blockMultilayer))
				for cell := range spec.cells {
					stack := spec.stacks[cell]
					if len(stack) == 0 {
						stack = []Layer{spec.cells[cell]}
					}
					data = append(data, byte(len(stack)))
					for _, layer := range stack {
						data = appendCellWord(data, layer)
					}
				}
			}
		}
	}

	return data
}

// appendCellWord appends one little endian encoded cell word.
func appendCellWord(data []byte, layer Layer) []byte {
	return appendRawWord(data, encodeLayer(layer))
}

// appendRawWord appends one little endian word without encoding.
func appendRawWord(data []byte, word uint16) []byte {
	raw := make([]byte, 2)
	binary.LittleEndian.PutUint16(raw, word)

	return append(data, raw...)
}

// setFlat makes the whole region a flat plane at the height.
func (s *regionSpec) setFlat(height int16) {
	for bx := range s.blocks {
		for by := range s.blocks[bx] {
			s.blocks[bx][by] = blockSpec{
				kind:  blockFlat,
				cells: [cellsPerBlock]Layer{{Height: height, NSWE: nsweAll}},
			}
		}
	}
}

// setCell converts the block of a local cell to complex and stores a
// layer in it.
func (s *regionSpec) setCell(localX, localY int, layer Layer) {
	spec := s.blockAt(localX, localY)
	if spec.kind != blockComplex {
		spec = blockSpec{
			kind:  blockComplex,
			cells: [cellsPerBlock]Layer{{Height: 0, NSWE: nsweAll}},
		}
	}
	cx, cy := localX%cellsPerBlockSide, localY%cellsPerBlockSide
	spec.cells[cx*cellsPerBlockSide+cy] = layer
	s.writeBlock(localX, localY, spec)
}

// setMultilayer converts the block of a local cell to multilayer and
// stores the layer stack in it.
func (s *regionSpec) setMultilayer(localX, localY int, layers []Layer) {
	spec := s.blockAt(localX, localY)
	if spec.kind != blockMultilayer {
		spec = blockSpec{
			kind:  blockMultilayer,
			cells: [cellsPerBlock]Layer{{Height: 0, NSWE: nsweAll}},
		}
	}
	cx, cy := localX%cellsPerBlockSide, localY%cellsPerBlockSide
	spec.stacks[cx*cellsPerBlockSide+cy] = layers
	s.writeBlock(localX, localY, spec)
}

// blockAt returns a copy of the block spec of a local cell.
func (s *regionSpec) blockAt(localX, localY int) blockSpec {
	bx, by := localX/cellsPerBlockSide, localY/cellsPerBlockSide

	return s.blocks[bx][by]
}

// writeBlock stores a modified block spec back.
func (s *regionSpec) writeBlock(localX, localY int, spec blockSpec) {
	bx, by := localX/cellsPerBlockSide, localY/cellsPerBlockSide
	s.blocks[bx][by] = spec
}

// writeRegion encodes the spec and stores it as the test region file.
func (s *regionSpec) writeRegion(t *testing.T, dir string) {
	t.Helper()
	name := filepath.Join(dir, regionFileName(testRegionCol, testRegionRow))
	require.NoError(t, os.WriteFile(name, s.encode(), 0o644))
}

// worldOf converts region local cell coordinates to the world center of
// the cell, with z on top of the layer height.
func worldOf(localX, localY int, height int16) Vec3 {
	baseX := (testRegionCol - tileZeroCol) * tileSize
	baseY := (testRegionRow - tileZeroRow) * tileSize

	return Vec3{
		X: float64(baseX + localX*cellSize + cellSize/2),
		Y: float64(baseY + localY*cellSize + cellSize/2),
		Z: float64(height),
	}
}

// regionFileName builds the X_Y.l2j name of a region.
func regionFileName(col, row int) string {
	return strconv.Itoa(col) + "_" + strconv.Itoa(row) + ".l2j"
}

// decodeCellRoundTrip checks the cell word encoding against the decoder.
// The wire format stores the height shifted left by one with the wall
// flags in the low nibble, so heights are quantized to multiples of 8 -
// exactly like the real geodata (flat heights of the Giran sample:
// -3584, -3576, -3560...).
func TestDecodeCellRoundTrip(t *testing.T) {
	layers := []Layer{
		{Height: 0, NSWE: nsweAll},
		{Height: -3536, NSWE: nsweNorth | nsweSouth},
		{Height: 152, NSWE: 0},
		{Height: -8, NSWE: nsweEast},
		{Height: 16384 - 8, NSWE: nsweAll},
		{Height: -16384, NSWE: nsweAll},
	}
	for _, layer := range layers {
		decoded := decodeCell(encodeLayer(layer))
		require.Equal(t, layer, decoded)
	}
}

// TestParseRegionSynthetic checks the block indexing and the layer
// decoding of a hand built region: flat, complex and multilayer blocks
// are addressed through the Mobius formulas.
func TestParseRegionSynthetic(t *testing.T) {
	pool := newLayerPool()
	spec := &regionSpec{}
	spec.setFlat(-104)
	// A complex cell with walls and a height.
	spec.setCell(5, 3, Layer{Height: -3536, NSWE: nsweNorth | nsweEast})
	// A complex cell in another block.
	spec.setCell(600, 1300, Layer{Height: 40, NSWE: nsweWest})
	// A multilayer cell with two floors.
	spec.setMultilayer(100, 200, []Layer{
		{Height: -48, NSWE: nsweAll},
		{Height: 496, NSWE: nsweAll},
	})

	region, err := parseRegion(spec.encode(), RegionKey{Col: 22, Row: 22}, pool)
	require.NoError(t, err)

	// The untouched flat cells keep their height and open walls.
	flat, ok := region.ClosestLayer(Point{X: 1000, Y: 1000}, 0)
	require.True(t, ok)
	require.Equal(t, int16(-104), flat.Height)
	require.True(t, flat.IsCompletelyOpen())

	// The complex cell round trips height and walls.
	walled, ok := region.ClosestLayer(Point{X: 5, Y: 3}, -3536)
	require.True(t, ok)
	require.Equal(t, int16(-3536), walled.Height)
	require.True(t, walled.IsNorthOpen())
	require.True(t, walled.IsEastOpen())
	require.False(t, walled.IsSouthOpen())
	require.False(t, walled.IsWestOpen())

	far, ok := region.ClosestLayer(Point{X: 600, Y: 1300}, 40)
	require.True(t, ok)
	require.Equal(t, int16(40), far.Height)
	require.True(t, far.IsWestOpen())
	require.False(t, far.IsEastOpen())

	// The multilayer cell picks the layer closest to the query height.
	ground, ok := region.ClosestLayer(Point{X: 100, Y: 200}, -3536)
	require.True(t, ok)
	require.Equal(t, int16(-48), ground.Height)
	upper, ok := region.ClosestLayer(Point{X: 100, Y: 200}, 496)
	require.True(t, ok)
	require.Equal(t, int16(496), upper.Height)

	// The layer stack is complete.
	layers := region.Layers(Point{X: 100, Y: 200})
	require.Len(t, layers, 2)
	require.Equal(t, int16(-48), layers[0].Height)
	require.Equal(t, int16(496), layers[1].Height)
}

// TestParseRegionRejectsCorrupt checks the strictness of the parser:
// truncated files, unknown block types, impossible layer counts and
// trailing bytes are all rejected.
func TestParseRegionRejectsCorrupt(t *testing.T) {
	pool := newLayerPool()
	key := RegionKey{Col: 1, Row: 1}

	build := func(mangle func(data []byte) []byte) []byte {
		spec := &regionSpec{}
		spec.setFlat(0)
		data := spec.encode()
		if mangle != nil {
			data = mangle(data)
		}

		return data
	}

	_, err := parseRegion(build(nil), key, pool)
	require.NoError(t, err)

	_, err = parseRegion(build(nil)[:100], key, pool)
	require.Error(t, err)

	_, err = parseRegion(append(build(nil), 0), key, pool)
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing")

	mangled := build(nil)
	mangled[0] = 7
	_, err = parseRegion(mangled, key, pool)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid block type")

	// A multilayer block with a zero layer count byte: block 0 of the
	// file holds one flat block (3 bytes), so the next block header
	// starts at offset 3.
	mangled = build(nil)
	mangled[3] = byte(blockMultilayer)
	mangled[4] = 0
	_, err = parseRegion(mangled, key, pool)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid layer count")
}

// TestParseRegionGiranSample parses the real Giran region shipped with
// the original pathfinder example: the file must be consumed exactly,
// the cells must decode and the terrain must not be an empty ocean
// plane.
func TestParseRegionGiranSample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "22_22.l2j"))
	require.NoError(t, err)

	pool := newLayerPool()
	region, err := parseRegion(data, RegionKey{Col: 22, Row: 22}, pool)
	require.NoError(t, err)

	// The Giran region contains flat, complex and multilayer blocks.
	kinds := map[blockKind]int{}
	for _, kind := range region.kinds {
		kinds[kind]++
	}
	require.Greater(t, kinds[blockFlat], 0)
	require.Greater(t, kinds[blockComplex], 0)
	require.Greater(t, kinds[blockMultilayer], 0)

	// The example start position near the Giran weapon shop decodes to
	// a sane street height.
	height, ok := region.ClosestLayer(Point{X: 926, Y: 1001}, -3533)
	require.True(t, ok)
	require.Greater(t, height.Height, int16(-12000))
	require.Less(t, height.Height, int16(16000))
}
