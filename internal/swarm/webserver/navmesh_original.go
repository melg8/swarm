// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The original geometry encoder of the navmesh viewer (the comparison
// variant the owner asked for): the NMV2 payload rendered straight
// from the raw l2j geodata cells, no sheet decomposition, no rectangle
// smoothing, no vertex field - the terrain exactly as the server sees
// it. Every cell layer with at least one open wall direction renders
// as its own flat quad at the exact layer height; adjacent cells of a
// slope disagree at the shared edge and the same height step walls of
// the mesh payload close the crack - the honest per cell staircase
// that started the "roof sheets" report. The link portal block stays
// empty: passability lives in the cell walls themselves, the viewer
// draws it through the step geometry, not through the overlay.
//
// The only compression is the exact height merge: neighboring cells
// of the SAME layer height merge into maximal rectangles, which
// renders pixel identical to the per cell quads while keeping the
// payload at the mesh tile scale (21_19: 390k rectangles over 4.4M
// walkable layers). Anything else - heights, steps, layers, water -
// stays untouched.

// origWaterLevel is the C1 water surface (navbuild waterLevel): the
// layers below it are the swim areas, rendered with the water ramp.
const origWaterLevel = int16(-3780)

// origCell is one walkable (height, cell) pair of the region walk.
type origCell struct {
	height int16
	cell   int32
}

// origPoly is one exact height rectangle of the original geometry.
// The four corner heights are all H (a flat quad).
type origPoly struct {
	X0, Y0, X1, Y1 int32
	H              int16
	Area           uint8
}

// origRegionSide is the cell side of one region (2048x2048 cells).
const origRegionSide = 2048

// origLayerStackBufSize is the layer stack buffer a cell walk reuses.
const origLayerStackBufSize = 8

// buildOriginalPolys walks every cell of the region and merges the
// walkable layers into the exact height rectangles. The result is
// deterministic: heights ascend, seeds scan row major, ties keep the
// first rectangle.
func buildOriginalPolys(region *pathfind.Region) []origPoly {
	pairs := origWalkCells(region)
	polys := make([]origPoly, 0, len(pairs)/8+64)
	bitset := make([]uint64, origRegionSide*origRegionSide/64)

	for start := 0; start < len(pairs); {
		end := start
		for end < len(pairs) && pairs[end].height == pairs[start].height {
			end++
		}
		polys = append(polys, origRectsOfHeight(
			pairs[start:end], bitset)...)
		start = end
	}

	return polys
}

// origWalkCells collects the (height, cell) pairs of every layer with
// at least one open direction, deduplicated per cell and sorted by
// height then cell (the merge and the wall passes rely on the order).
func origWalkCells(region *pathfind.Region) []origCell {
	pairs := make([]origCell, 0, 1<<20)
	buf := make([]pathfind.Layer, 0, origLayerStackBufSize)
	for x := range origRegionSide {
		for y := range origRegionSide {
			stack := region.LayerStack(pathfind.Point{
				X: int32(x), Y: int32(y)}, buf)
			buf = stack[:0]
			for _, layer := range stack {
				if layer.NSWE == 0 {
					continue
				}
				pairs = append(pairs, origCell{
					height: layer.Height,
					cell:   int32(x*origRegionSide + y),
				})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].height != pairs[j].height {
			return pairs[i].height < pairs[j].height
		}

		return pairs[i].cell < pairs[j].cell
	})

	return pairs
}

// origSet marks one cell in the merge bitset.
func origSet(bitset []uint64, cell int32) {
	bitset[cell>>6] |= 1 << uint(cell&63)
}

// origClear drops one cell from the merge bitset.
func origClear(bitset []uint64, cell int32) {
	bitset[cell>>6] &^= 1 << uint(cell&63)
}

// origHas reports whether one cell sits in the merge bitset.
func origHas(bitset []uint64, cell int32) bool {
	return bitset[cell>>6]&(1<<uint(cell&63)) != 0
}

// origRectsOfHeight greedily merges one height's cell run into
// maximal rectangles: the run's cells seed the bitset, every still
// set cell starts a rectangle that extends along y while the cells
// hold and then along x while whole rows hold, and clears its cells.
// The bitset scratch stays dirty only with the current run's cells
// (every emitted rect clears itself), so the caller reuses it across
// runs.
func origRectsOfHeight(run []origCell, bitset []uint64) []origPoly {
	height := run[0].height
	area := navmesh.AreaGround
	if height < origWaterLevel {
		area = navmesh.AreaWater
	}
	for _, pair := range run {
		origSet(bitset, pair.cell)
	}
	polys := make([]origPoly, 0, 16)
	for _, pair := range run {
		if !origHas(bitset, pair.cell) {
			continue
		}
		x, y := pair.cell/origRegionSide, pair.cell%origRegionSide
		width := int32(0)
		for yy := y; yy < origRegionSide; yy++ {
			if !origHas(bitset, x*origRegionSide+yy) {
				break
			}
			width++
		}
		span := int32(1)
	extend:
		for xx := x + 1; xx < origRegionSide; xx++ {
			for yy := y; yy < y+width; yy++ {
				if !origHas(bitset, xx*origRegionSide+yy) {
					break extend
				}
			}
			span++
		}
		for xx := x; xx < x+span; xx++ {
			for yy := y; yy < y+width; yy++ {
				origClear(bitset, xx*origRegionSide+yy)
			}
		}
		polys = append(polys, origPoly{
			X0:   x,
			Y0:   y,
			X1:   x + span,
			Y1:   y + width,
			H:    height,
			Area: area,
		})
	}

	return polys
}

// encodeOriginalGeometry renders one region into the binary NMV2
// payload the viewer draws for the original variant: the exact height
// surface quads, no link portals and the height step walls that close
// the cracks between the neighboring cells. The east and north region
// borders resolve their targets through the neighbor regions (the
// loader answers nil for a region that does not exist - the seam
// stays open and merges with the void of the unloaded tile).
func encodeOriginalGeometry(key navmesh.RegionKey,
	region *pathfind.Region,
	neighbor func(navmesh.RegionKey) *pathfind.Region,
) ([]byte, error) {
	polys := buildOriginalPolys(region)
	if len(polys) == 0 {
		return nil, fmt.Errorf("region %d_%d holds no walkable cells",
			key.Col, key.Row)
	}
	if int64(len(polys))*navmeshGeoRecordSize > 1<<31 {
		return nil, fmt.Errorf("region %d_%d too large: %d rects",
			key.Col, key.Row, len(polys))
	}
	walls := encodeOriginalWalls(key, polys, neighbor)

	size := navmeshGeoHeaderSize +
		len(polys)*navmeshGeoCornerStride*4 +
		geoPad4(len(polys)) + len(walls)
	payload := make([]byte, size)
	minH, maxH := origHeightRange(polys)
	binary.LittleEndian.PutUint32(payload[0:], navmeshGeoMagic)
	binary.LittleEndian.PutUint16(payload[4:], uint16(key.Col))
	binary.LittleEndian.PutUint16(payload[6:], uint16(key.Row))
	binary.LittleEndian.PutUint32(payload[8:], uint32(origWorldMinX(key)))
	binary.LittleEndian.PutUint32(payload[12:], uint32(origWorldMinY(key)))
	binary.LittleEndian.PutUint16(payload[16:], uint16(minH))
	binary.LittleEndian.PutUint16(payload[18:], uint16(maxH))
	binary.LittleEndian.PutUint32(payload[20:], uint32(len(polys)))
	binary.LittleEndian.PutUint32(payload[24:], 0)
	binary.LittleEndian.PutUint32(payload[28:],
		uint32(len(walls)/navmeshGeoWallStride))

	offset := navmeshGeoHeaderSize
	for i := range polys {
		poly := &polys[i]
		writeGeoCorner(payload, offset,
			poly.X0, poly.Y0, int32(poly.H))
		writeGeoCorner(payload, offset+6,
			poly.X1, poly.Y0, int32(poly.H))
		writeGeoCorner(payload, offset+12,
			poly.X0, poly.Y1, int32(poly.H))
		writeGeoCorner(payload, offset+18,
			poly.X1, poly.Y1, int32(poly.H))
		offset += navmeshGeoCornerStride * 4
	}
	for i := range polys {
		payload[offset+i] = polys[i].Area
	}
	offset += geoPad4(len(polys))
	offset += copy(payload[offset:], walls)
	if offset != len(payload) {
		return nil, fmt.Errorf("region %d_%d original size mismatch: "+
			"%d of %d", key.Col, key.Row, offset, len(payload))
	}

	return payload, nil
}

// origWorldMinX answers the world x anchor of a region key (the same
// derivation the tile decode uses: the region file name anchor is
// world x floor division plus the zero column).
func origWorldMinX(key navmesh.RegionKey) float64 {
	return (float64(key.Col) - float64(navmesh.TileZeroCol())) *
		navmesh.TileWorldSize()
}

// origWorldMinY answers the world y anchor of a region key.
func origWorldMinY(key navmesh.RegionKey) float64 {
	return (float64(key.Row) - float64(navmesh.TileZeroRow())) *
		navmesh.TileWorldSize()
}

// origHeightRange scans the rectangle heights of the payload.
func origHeightRange(polys []origPoly) (int16, int16) {
	minH, maxH := int16(32767), int16(-32768)
	for i := range polys {
		if polys[i].H < minH {
			minH = polys[i].H
		}
		if polys[i].H > maxH {
			maxH = polys[i].H
		}
	}

	return minH, maxH
}

// encodeOriginalWalls renders the height step wall block of the
// original geometry. The emission follows the mesh payload rules: the
// walls leave through the MaxX and MaxY sides of the emitting
// rectangle, every shared edge is walled exactly once, the subpixel
// steps stay open and the over cap steps (the open air between two
// separate worlds - the deck over the lake) stay void. The region
// borders resolve through the east and north neighbor regions.
func encodeOriginalWalls(key navmesh.RegionKey, polys []origPoly,
	neighbor func(navmesh.RegionKey) *pathfind.Region,
) []byte {
	side := int32(origRegionSide)
	targetsX := make(map[int32][]*origPoly, 64)
	targetsY := make(map[int32][]*origPoly, 64)
	for i := range polys {
		p := &polys[i]
		targetsX[p.X0] = append(targetsX[p.X0], p)
		targetsY[p.Y0] = append(targetsY[p.Y0], p)
	}
	var east, north []*origPoly
	if neighbor != nil {
		if eastRegion := neighbor(navmesh.RegionKey{
			Col: key.Col + 1, Row: key.Row}); eastRegion != nil {
			east = origBorderStrip(eastRegion, false)
		}
		if northRegion := neighbor(navmesh.RegionKey{
			Col: key.Col, Row: key.Row + 1}); northRegion != nil {
			north = origBorderStrip(northRegion, true)
		}
	}

	writer := make([]byte, 0, len(polys)*navmeshGeoWallStride)
	for i := range polys {
		poly := &polys[i]
		if poly.X1 < side {
			for _, target := range targetsX[poly.X1] {
				origWallVertical(&writer, poly, target)
			}
		} else {
			for _, target := range east {
				origWallVertical(&writer, poly, target)
			}
		}
		if poly.Y1 < side {
			for _, target := range targetsY[poly.Y1] {
				origWallHorizontal(&writer, poly, target)
			}
		} else {
			for _, target := range north {
				origWallHorizontal(&writer, poly, target)
			}
		}
	}

	return writer
}

// origBorderStrip renders the border facing cell strip of one region
// as one cell wide rectangles: the exact surface profile the region
// border walls close onto (the east neighbor faces through its x 0
// column, the north one through its y 0 row). The strip walk touches
// 2048 cells - a fraction of the full region pass the emitter side
// already paid.
func origBorderStrip(region *pathfind.Region, north bool) []*origPoly {
	buf := make([]pathfind.Layer, 0, origLayerStackBufSize)
	heights := map[int16][]int32{}
	for v := range origRegionSide {
		var point pathfind.Point
		if north {
			point = pathfind.Point{X: int32(v), Y: 0}
		} else {
			point = pathfind.Point{X: 0, Y: int32(v)}
		}
		layers := region.LayerStack(point, buf)
		buf = layers[:0]
		for _, layer := range layers {
			if layer.NSWE == 0 {
				continue
			}
			heights[layer.Height] = append(heights[layer.Height],
				int32(v))
		}
	}
	keys := make([]int, 0, len(heights))
	for h := range heights {
		keys = append(keys, int(h))
	}
	sort.Ints(keys)

	polys := make([]*origPoly, 0, len(keys))
	for _, h := range keys {
		for _, run := range origRuns(heights[int16(h)]) {
			polys = append(polys, origStripPoly(int16(h), run, north))
		}
	}

	return polys
}

// origRuns merges the ascending border cell indices of one height
// into the consecutive half open runs [start, end).
func origRuns(cells []int32) [][2]int32 {
	runs := make([][2]int32, 0, 2)
	start, prev := cells[0], cells[0]
	for _, v := range cells[1:] {
		if v == prev+1 {
			prev = v

			continue
		}
		runs = append(runs, [2]int32{start, prev + 1})
		start, prev = v, v
	}

	return append(runs, [2]int32{start, prev + 1})
}

// origStripPoly builds the one cell wide border rectangle of one
// consecutive run: along the border for the east neighbor (its x 0
// column), across it for the north one (its y 0 row).
func origStripPoly(height int16, run [2]int32, north bool) *origPoly {
	area := navmesh.AreaGround
	if height < origWaterLevel {
		area = navmesh.AreaWater
	}
	poly := &origPoly{
		H:    height,
		Area: area,
		X0:   run[0],
		X1:   run[1],
		Y0:   0,
		Y1:   1,
	}
	if !north {
		poly.X0, poly.X1 = 0, 1
		poly.Y0, poly.Y1 = run[0], run[1]
	}

	return poly
}

// origWallVertical walls the MaxX edge of the emitter against the
// MinX edge of the target: the shared x line, the overlapping y
// range, the two flat surface heights.
func origWallVertical(writer *[]byte, a, b *origPoly) {
	lo := max(a.Y0, b.Y0)
	hi := min(a.Y1, b.Y1)
	if lo >= hi {
		return
	}
	geoWallSpans(writer, float64(a.X1)*16, float64(lo)*16,
		float64(hi)*16,
		func(int32) float64 { return float64(a.H) },
		func(int32) float64 { return float64(b.H) },
		lo, hi, a.Area, navmeshWallVertical)
}

// origWallHorizontal walls the MaxY edge of the emitter against the
// MinY edge of the target: the shared y line, the overlapping x
// range.
func origWallHorizontal(writer *[]byte, a, b *origPoly) {
	lo := max(a.X0, b.X0)
	hi := min(a.X1, b.X1)
	if lo >= hi {
		return
	}
	geoWallSpans(writer, float64(a.Y1)*16, float64(lo)*16,
		float64(hi)*16,
		func(int32) float64 { return float64(a.H) },
		func(int32) float64 { return float64(b.H) },
		lo, hi, a.Area, navmeshWallHorizontal)
}

// loadGeodataRegion reads and parses one raw geodata region file of
// the engine directory (the original geometry source of the viewer).
func loadGeodataRegion(dir string, key navmesh.RegionKey,
) (*pathfind.Region, error) {
	path := filepath.Join(dir,
		fmt.Sprintf("%d_%d.l2j", key.Col, key.Row))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read geodata region %d_%d: %w",
			key.Col, key.Row, err)
	}

	return pathfind.ParseRegionData(data, pathfind.RegionKey{
		Col: key.Col, Row: key.Row})
}

// navmeshOriginalGeometry returns the cached binary original payload
// of one region together with its ETag. The first request pays the
// region parse and the exact height merge (seconds for a real
// region); every later request serves the cached bytes.
func (s *Server) navmeshOriginalGeometry(key navmesh.RegionKey,
) ([]byte, string, error) {
	s.navmeshGeoMu.Lock()
	defer s.navmeshGeoMu.Unlock()
	if s.navmeshOriginal == nil {
		s.navmeshOriginal = make(map[navmesh.RegionKey][]byte)
	}
	if payload, ok := s.navmeshOriginal[key]; ok {
		return payload, origGeoETag(key, payload), nil
	}
	if s.navmeshRegions == nil {
		return nil, "", errOriginalUnavailable
	}
	region, err := s.navmeshRegions(key)
	if err != nil {
		return nil, "", err
	}
	payload, err := encodeOriginalGeometry(key, region,
		func(neighborKey navmesh.RegionKey) *pathfind.Region {
			neighbor, err := s.navmeshRegions(neighborKey)
			if err != nil {
				return nil
			}

			return neighbor
		})
	if err != nil {
		return nil, "", err
	}
	s.navmeshOriginal[key] = payload

	return payload, origGeoETag(key, payload), nil
}

// errOriginalUnavailable answers the original geometry requests of a
// viewer without the geodata engine.
var errOriginalUnavailable = errors.New(
	"original geometry needs the geodata engine (the -geodata " +
		"directory with the X_Y.l2j files)")

// origGeoETag derives the immutable original geometry tag of a
// region: the key with the block counts of the NMV2 payload.
func origGeoETag(key navmesh.RegionKey, payload []byte) string {
	return fmt.Sprintf(`"nmv2o-%d_%d-%d-%d-%d"`, key.Col, key.Row,
		binary.LittleEndian.Uint32(payload[20:24]),
		binary.LittleEndian.Uint32(payload[24:28]),
		binary.LittleEndian.Uint32(payload[28:32]))
}
