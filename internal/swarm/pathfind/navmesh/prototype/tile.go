// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package prototype is the research prototype of the Detour runtime
// query engine in pure Go: it parses the navigation mesh tile the
// research/recast C++ converter exports from the l2j geodata and
// answers nearest polygon and path queries over it. It is the Go
// feasibility half of the feature/new-pathfind research round
// (docs/recast_pathfinding.md): the build pipeline stays offline,
// the runtime is this package.
package prototype

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Tile format constants of Detour (DetourNavMesh.h): the magic, the
// version and the polygon vertex capacity the C++ exporter wrote.
const (
	navMeshMagic     = 0x444E4156 // 'DNAV' as the C++ shifts build it
	navMeshVersion   = 7
	vertsPerPolygon  = 6
	headerWordCount  = 25
	polyStructSize   = 32
	linkStructSize   = 12
	bvNodeStructSize = 16
)

// ErrBadTile reports a tile that is not a Detour navmesh tile of the
// version this parser reads.
var ErrBadTile = errors.New("not a Detour navmesh tile")

// TileHeader mirrors dtMeshHeader field by field: every field is one
// 4 byte little endian word in the wire order below.
type TileHeader struct {
	Magic, Version, X, Y, Layer     int32
	UserID                          uint32
	PolyCount, VertCount            int32
	MaxLinkCount                    int32
	DetailMeshCount                 int32
	DetailVertCount, DetailTriCount int32
	BVNodeCount, OffMeshConCount    int32
	OffMeshBase                     int32
	WalkableHeight, WalkableRadius  float32
	WalkableClimb                   float32
	BMin, BMax                      [3]float32
	BVQuantFactor                   float32
}

// Poly is one navigation polygon: the vertex indices into the tile
// vertex array, the neighbor polygon index per edge (the Detour nei
// encoding: neighbor index plus one, zero marks a border edge) and
// the flags and area the converter assigned.
type Poly struct {
	Verts     [vertsPerPolygon]uint16
	Neis      [vertsPerPolygon]uint16
	Flags     uint16
	VertCount int
	Area      int
	Type      int
}

// BVNode is one node of the tile bounding volume tree: quantized
// bounds with the leaf escape encoded as a negative index.
type BVNode struct {
	BMin [3]uint16
	BMax [3]uint16
	I    int32
}

// Tile is a parsed single tile navigation mesh. The vertices keep the
// Recast axis order of the export (x, height, world y).
type Tile struct {
	Header TileHeader
	Verts  []float32
	Polys  []Poly
	BVTree []BVNode
	// Adjacency mirrors the link lists Detour builds at load time:
	// Adjacency[poly][edge] is the neighbor polygon index of the edge
	// (or -1 for a border edge).
	Adjacency [][]int16
}

// ParseTile decodes the raw bytes of one Detour navmesh tile as
// written by dtCreateNavMeshData (the section order: header, verts,
// polys, link space, detail meshes, detail verts, detail triangles,
// bvtree, off mesh connections). The link section is left empty by
// the exporter on purpose - the links are rebuilt from the polygon
// neighbor fields, exactly like dtNavMesh::connectIntLinks does.
func ParseTile(data []byte) (*Tile, error) {
	if len(data) < headerWordCount*4 {
		return nil, fmt.Errorf("%w: %d bytes is too short", ErrBadTile,
			len(data))
	}
	header := parseHeader(data)
	if header.Magic != navMeshMagic {
		return nil, fmt.Errorf("%w: magic 0x%x", ErrBadTile, header.Magic)
	}
	if header.Version != navMeshVersion {
		return nil, fmt.Errorf("%w: version %d", ErrBadTile, header.Version)
	}
	if header.PolyCount <= 0 || header.VertCount <= 0 {
		return nil, fmt.Errorf("%w: %d polys %d verts", ErrBadTile,
			header.PolyCount, header.VertCount)
	}

	offset := headerWordCount * 4
	verts, err := parseVerts(data, &offset, int(header.VertCount))
	if err != nil {
		return nil, err
	}
	polys, err := parsePolys(data, &offset, int(header.PolyCount))
	if err != nil {
		return nil, err
	}
	// The link section carries allocated space only: skip it, the
	// links are rebuilt from the neighbor fields.
	offset += align4(int(header.MaxLinkCount) * linkStructSize)
	// The detail meshes, detail verts and detail triangles are not
	// needed by the query prototype: skip them.
	offset += align4(int(header.DetailMeshCount) * 12)
	offset += align4(int(header.DetailVertCount) * 12)
	offset += align4(int(header.DetailTriCount) * 4)
	tree, err := parseBVTree(data, &offset, int(header.BVNodeCount))
	if err != nil {
		return nil, err
	}

	tile := &Tile{
		Header:    header,
		Verts:     verts,
		Polys:     polys,
		BVTree:    tree,
		Adjacency: buildAdjacency(polys),
	}

	return tile, nil
}

// parseHeader decodes the 25 word tile header.
func parseHeader(data []byte) TileHeader {
	words := make([]uint32, headerWordCount)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	bmin := [3]float32{}
	bmax := [3]float32{}
	for a := range 3 {
		bmin[a] = math.Float32frombits(words[18+a])
		bmax[a] = math.Float32frombits(words[21+a])
	}

	return TileHeader{
		Magic:           int32(words[0]),
		Version:         int32(words[1]),
		X:               int32(words[2]),
		Y:               int32(words[3]),
		Layer:           int32(words[4]),
		UserID:          words[5],
		PolyCount:       int32(words[6]),
		VertCount:       int32(words[7]),
		MaxLinkCount:    int32(words[8]),
		DetailMeshCount: int32(words[9]),
		DetailVertCount: int32(words[10]),
		DetailTriCount:  int32(words[11]),
		BVNodeCount:     int32(words[12]),
		OffMeshConCount: int32(words[13]),
		OffMeshBase:     int32(words[14]),
		WalkableHeight:  math.Float32frombits(words[15]),
		WalkableRadius:  math.Float32frombits(words[16]),
		WalkableClimb:   math.Float32frombits(words[17]),
		BMin:            bmin,
		BMax:            bmax,
		BVQuantFactor:   math.Float32frombits(words[24]),
	}
}

// parseVerts reads the vertex block (three float32 words per vertex).
func parseVerts(data []byte, offset *int, count int) ([]float32, error) {
	size := align4(count * 12)
	if *offset+size > len(data) {
		return nil, fmt.Errorf("%w: verts block truncated", ErrBadTile)
	}
	verts := make([]float32, count*3)
	for i := range verts {
		verts[i] = math.Float32frombits(
			binary.LittleEndian.Uint32(data[*offset+i*4:]))
	}
	*offset += size

	return verts, nil
}

// parsePolys reads the polygon block of the fixed size dtPoly structs.
func parsePolys(data []byte, offset *int, count int) ([]Poly, error) {
	size := align4(count * polyStructSize)
	if *offset+size > len(data) {
		return nil, fmt.Errorf("%w: polys block truncated", ErrBadTile)
	}
	polys := make([]Poly, count)
	for i := range polys {
		base := *offset + i*polyStructSize
		poly := &polys[i]
		for v := range vertsPerPolygon {
			poly.Verts[v] = binary.LittleEndian.Uint16(
				data[base+4+v*2:])
			poly.Neis[v] = binary.LittleEndian.Uint16(
				data[base+16+v*2:])
		}
		poly.Flags = binary.LittleEndian.Uint16(data[base+28:])
		poly.VertCount = int(data[base+30])
		areaAndType := data[base+31]
		poly.Area = int(areaAndType & 0x3F)
		poly.Type = int(areaAndType >> 6)
	}
	*offset += size

	return polys, nil
}

// parseBVTree reads the bounding volume tree block.
func parseBVTree(data []byte, offset *int, count int) ([]BVNode, error) {
	if count == 0 {
		return nil, nil
	}
	size := align4(count * bvNodeStructSize)
	if *offset+size > len(data) {
		return nil, fmt.Errorf("%w: bvtree block truncated", ErrBadTile)
	}
	tree := make([]BVNode, count)
	for i := range tree {
		base := *offset + i*bvNodeStructSize
		node := &tree[i]
		for a := range 3 {
			node.BMin[a] = binary.LittleEndian.Uint16(data[base+a*2:])
			node.BMax[a] = binary.LittleEndian.Uint16(data[base+6+a*2:])
		}
		node.I = int32(binary.LittleEndian.Uint32(data[base+12:]))
	}
	*offset += size

	return tree, nil
}

// buildAdjacency converts the Detour nei fields into neighbor polygon
// indices per edge: a nei value of zero is a border edge, anything
// else is the neighbor index plus one. External link flags are absent
// in a single tile mesh.
func buildAdjacency(polys []Poly) [][]int16 {
	adjacency := make([][]int16, len(polys))
	for i := range polys {
		poly := &polys[i]
		neighbors := make([]int16, vertsPerPolygon)
		for e := range vertsPerPolygon {
			switch {
			case e >= poly.VertCount:
				neighbors[e] = -1
			case poly.Neis[e] == 0:
				neighbors[e] = -1
			default:
				neighbors[e] = int16(poly.Neis[e]) - 1
			}
		}
		adjacency[i] = neighbors
	}

	return adjacency
}

// align4 rounds the size up to the next multiple of four, the Detour
// block alignment of every tile section.
func align4(size int) int {
	return (size + 3) &^ 3
}
