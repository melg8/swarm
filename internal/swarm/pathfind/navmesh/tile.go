// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package navmesh is the production Detour-style navigation mesh
// runtime of the feature/new-pathfind port (docs/navmesh.md): tiles
// of rectangle polygons built offline from the l2j geodata by
// internal/swarm/pathfind/navbuild, loaded lazily by the Mesh and
// answered by the Query (the nearest polygon resolution, the A*
// corridor search, the funnel string pulling and the water escape).
//
// The tile carries the walkable surfaces as rectangle polygons: the
// region local cell bounds, the four exact corner heights of the
// geodata cells at the corners and the Detour-style link chains whose
// portal spans keep every crossing inside the open cell pairs of the
// shared edge (the NSWE wall fidelity of docs/recast_pathfinding.md).
package navmesh

import (
    "encoding/binary"
    "errors"
    "fmt"
    "math"
)

// World geometry constants, mirroring internal/swarm/pathfind
// (geometry.go: World.TILE_SIZE and the region anchors of the l2j
// grid). They are duplicated here so the runtime stays a self
// contained leaf package.
const (
    // regionCellsSide is the cell count of one region side (2048).
    regionCellsSide = 2048
    // cellSizeWorld is the world size of one geodata cell (16).
    cellSizeWorld = 16
    // tileWorldSize is the world size of one region (32768).
    tileWorldSize = regionCellsSide * cellSizeWorld
    // tileZeroCol/Row are the region file name anchors: the region of
    // world x is floor(x/tileWorldSize) + tileZeroCol.
    tileZeroCol = 20
    tileZeroRow = 18
    // heightFloor is the world height the BVTree quantization anchors
    // on (the geodata height range is well inside +-8192).
    heightFloor = -8192
)

// Polygon area ids. The area carries the water semantics of the grid
// engine: the water polys multiply the step cost by the swim area
// cost (the dry filters exclude them, the escape prices them high).
const (
    AreaGround uint8 = 0
    AreaWater  uint8 = 1
)

// Poly sides: the side of the rectangle a link leaves through. The
// naming follows the cell axes (SideMinX = the x==X0 edge); the NSWE
// compass of the geodata maps west/east/north/south onto
// minx/maxx/miny/maxy respectively.
const (
    SideMinX uint8 = 0
    SideMaxX uint8 = 1
    SideMinY uint8 = 2
    SideMaxY uint8 = 3
)

// Tile format constants: the magic word, the version and the sizes of
// the wire structures (every section is 4 byte aligned, little
// endian). Version 2 quantizes the polygon bounds and the link spans
// to uint16 cell units (the region fits 2048 cells per side, the
// heights stay int16), the external link region keys to int16 and
// keeps the link chain offsets uint32 (the dense regions carry over
// a million links). Version 3 is the columnar layout: the polygon
// planes, the poly ordered link chains (the implicit CSR, no Next
// wire), the packed side and span word and the uniform bucket grid
// instead of the bounding volume tree - the section split probe
// named the tree and the link records the compressed mass
// (docs/fastpath_research.md section 11). The decode reads every
// version.
const (
    tileMagic = 0x31574E53 // 'SWN1' little endian

    tileVersion = 3

    tileHeaderSize = 40

    // The version 1 wire sizes (the int32 bounds and spans).
    polyWireSizeV1    = 32
    linkWireSizeV1    = 20
    extLinkWireSizeV1 = 12

    // The version 2 wire sizes (the uint16 quantization).
    polyWireSize    = 24
    linkWireSize    = 16
    extLinkWireSize = 8

    bvNodeWireSize = 16

    // The version 3 grid index: the bucket count per axis and the
    // cell span of one bucket (the region is 2048 cells per side).
    gridSide        = 64
    gridBucketCells = regionCellsSide / gridSide
    gridBuckets     = gridSide * gridSide
)

// ErrBadTile reports a tile file that is not a navigation mesh tile
// of the version this package reads.
var ErrBadTile = errors.New("not a navmesh tile")

// Tile is one parsed navigation mesh tile: the rectangle polygons of
// one geodata region, their link chains and the bounding volume tree.
// A tile is immutable after decoding and safe for concurrent reads.
type Tile struct {
    Col, Row int16
    // Climb is the height step the links respect (the 40 unit
    // HEIGHT_INCREASE_LIMIT the server movement validation uses).
    Climb int32
    // Polys are the rectangle polygons in region local cell bounds
    // (X0 <= x < X1, Y0 <= y < Y1).
    Polys []Poly
    // Links is the flat link store every polygon chains into through
    // FirstLink/Link.Next.
    Links []Link
    // ExtLinks holds the cross region link targets the Links with a
    // negative To resolve into.
    ExtLinks []ExtLink
    // BVTree is the quantized bounding volume tree over the polygon
    // bounds (the Detour layout: axis 0 = world x, axis 1 = height,
    // axis 2 = world y). The v3 tiles answer nil: the Grid replaces
    // it.
    BVTree []BVNode
    // Grid is the uniform bucket index of the v3 tiles (nil for the
    // v1/v2 tiles: the BVTree serves those).
    Grid *TileGrid

    // worldMinX/worldMinY are derived at decode: the world anchor of
    // the region (the min corner of cell 0 0).
    worldMinX, worldMinY float64
}

// TileGrid is the uniform spatial index of the v3 tiles: the 64x64
// bucket grid of polygon ids over the region footprint. A polygon
// lists in every bucket its cell rectangle touches, the entries of
// one bucket run in increasing polygon order (the conservative 2D
// candidate set; the query post filters the height window and the
// caller tests the exact 3D geometry).
type TileGrid struct {
    Offsets []uint32
    Entries []uint32
}

// Poly is one rectangle navigation polygon.
type Poly struct {
    // X0, Y0, X1, Y1 are the region local cell bounds (half open:
    // the polygon covers the cells X0 <= cx < X1, Y0 <= cy < Y1; the
    // world rectangle spans the grid vertices X0*cell .. X1*cell).
    X0, Y0, X1, Y1 int32
    // H00/H10/H01/H11 are the corner heights, taken from the geodata
    // cells at the inside corners: H00 = cell (X0, Y0), H10 = cell
    // (X1-1, Y0), H01 = cell (X0, Y1-1), H11 = cell (X1-1, Y1-1).
    // The interior height is the bilinear interpolation of the four.
    H00, H10, H01, H11 int16
    // FirstLink indexes the first link of the chain (-1: the polygon
    // has no links - an isolated surface).
    FirstLink int32
    // Area is AreaGround or AreaWater.
    Area uint8
}

// Link is one polygon-to-polygon connection leaving through a side.
type Link struct {
    // Side is the rectangle side the link crosses (SideMinX..).
    Side uint8
    // To names the neighbor: >= 0 the polygon index inside the same
    // tile, < 0 the external target -(index into ExtLinks)-1.
    To int32
    // Next chains the next link of the same polygon (-1: the end).
    Next int32
    // T0/T1 bound the portal: the inclusive cell range along the
    // crossing axis where the geodata walls are open (side minx/maxx:
    // the y cell range; side miny/maxy: the x cell range). The
    // funnel may only cross the shared edge inside this span.
    T0, T1 int32
}

// ExtLink is the cross region target of a link.
type ExtLink struct {
    Col, Row int32
    Poly     uint32
}

// BVNode is one node of the tile bounding volume tree: quantized
// bounds with the leaf escape encoded as a negative index (the Detour
// dtBVNode layout).
type BVNode struct {
    BMin [3]uint16
    BMax [3]uint16
    I    int32
}

// PolyRef identifies one polygon of one tile: the packed region key
// and the polygon index (0 is the null reference).
type PolyRef uint64

// RefOf packs a region key and a polygon index into a reference.
func RefOf(col, row int16, poly uint32) PolyRef {
    return PolyRef(uint64(uint16(col)))<<48 | PolyRef(uint64(uint16(row)))<<32 |
        PolyRef(poly+1)
}

// TileOf returns the region key of a reference.
func TileOf(ref PolyRef) (col, row int16) {
    return int16(uint16(ref >> 48)), int16(uint16(ref >> 32))
}

// PolyOf returns the polygon index of a reference (-1 for the null
// reference).
func PolyOf(ref PolyRef) int32 {
    if ref == 0 {
        return -1
    }

    return int32(uint32(ref&0xFFFFFFFF)) - 1
}

// RegionOfWorld maps a world position to its region key.
func RegionOfWorld(x, y float64) (int16, int16) {
    return int16(math.Floor(x/tileWorldSize)) + tileZeroCol,
        int16(math.Floor(y/tileWorldSize)) + tileZeroRow
}

// TileWorldSize returns the world size of one region tile (32768).
func TileWorldSize() float64 { return tileWorldSize }

// TileZeroCol returns the region file name anchor of the world x
// axis (the region of world x is floor(x/tileWorldSize) + 20).
func TileZeroCol() int16 { return tileZeroCol }

// TileZeroRow returns the region file name anchor of the world y
// axis (the anchor the region row derives from, 18).
func TileZeroRow() int16 { return tileZeroRow }

// WorldMinX returns the world x anchor of the region (the min corner
// of the local cell column 0).
func (t *Tile) WorldMinX() float64 { return t.worldMinX }

// WorldMinY returns the world y anchor of the region.
func (t *Tile) WorldMinY() float64 { return t.worldMinY }

// WorldRect returns the world bounds of one polygon.
func (t *Tile) WorldRect(p *Poly) (x0, y0, x1, y1 float64) {
    return t.worldMinX + float64(p.X0)*cellSizeWorld,
        t.worldMinY + float64(p.Y0)*cellSizeWorld,
        t.worldMinX + float64(p.X1)*cellSizeWorld,
        t.worldMinY + float64(p.Y1)*cellSizeWorld
}

// HeightAt returns the bilinear surface height of the polygon at a
// world position (the position may lie outside the rectangle: the
// interpolation clamps u and v into [0, 1]).
func (t *Tile) HeightAt(p *Poly, worldX, worldY float64) float64 {
    x0, y0, x1, y1 := t.WorldRect(p)
    u := (worldX - x0) / (x1 - x0)
    v := (worldY - y0) / (y1 - y0)
    u = math.Max(0, math.Min(1, u))
    v = math.Max(0, math.Min(1, v))
    h00 := float64(p.H00)
    h10 := float64(p.H10)
    h01 := float64(p.H01)
    h11 := float64(p.H11)

    return (1-u)*(1-v)*h00 + u*(1-v)*h10 + (1-u)*v*h01 + u*v*h11
}

// ClosestPoint returns the point of the polygon surface closest to
// the world position in 3D: the position itself with the surface
// height when the footprint contains it, otherwise the clamped
// boundary point with the interpolated edge height. This is the
// closestPointOnPoly of the Detour query model - the heights are the
// exact bilinear surface of the four geodata corner cells.
func (t *Tile) ClosestPoint(p *Poly, x, y, z float64) (cx, cy, cz float64) {
    x0, y0, x1, y1 := t.WorldRect(p)
    px := math.Max(x0, math.Min(x1, x))
    py := math.Max(y0, math.Min(y1, y))

    return px, py, t.HeightAt(p, px, py)
}

// Portal returns the world segment of the link portal: the open span
// of the shared edge the funnel may cross. The segment runs along the
// polygon side, from the T0 cell boundary to the T1+1 cell boundary.
func (t *Tile) Portal(p *Poly, link *Link) (ax, ay, bx, by float64) {
    switch link.Side {
    case SideMinX:
        ax, ay = t.worldMinX+float64(p.X0)*cellSizeWorld,
            t.worldMinY+float64(link.T0)*cellSizeWorld
        bx, by = ax, t.worldMinY+float64(link.T1+1)*cellSizeWorld
    case SideMaxX:
        ax, ay = t.worldMinX+float64(p.X1)*cellSizeWorld,
            t.worldMinY+float64(link.T0)*cellSizeWorld
        bx, by = ax, t.worldMinY+float64(link.T1+1)*cellSizeWorld
    case SideMinY:
        ax, ay = t.worldMinX+float64(link.T0)*cellSizeWorld,
            t.worldMinY+float64(p.Y0)*cellSizeWorld
        bx, by = t.worldMinX+float64(link.T1+1)*cellSizeWorld, ay
    default: // SideMaxY
        ax, ay = t.worldMinX+float64(link.T0)*cellSizeWorld,
            t.worldMinY+float64(p.Y1)*cellSizeWorld
        bx, by = t.worldMinX+float64(link.T1+1)*cellSizeWorld, ay
    }

    return ax, ay, bx, by
}

// DecodeTile parses the raw bytes of one tile file. The wire
// versions decode: version 1 (the int32 bounds and spans), version 2
// (the uint16 quantization) and version 3 (the columnar layout with
// the bucket grid index).
func DecodeTile(data []byte) (*Tile, error) {
    if len(data) < tileHeaderSize {
        return nil, fmt.Errorf("%w: %d bytes is too short", ErrBadTile,
            len(data))
    }
    if magic := binary.LittleEndian.Uint32(data[0:]); magic != tileMagic {
        return nil, fmt.Errorf("%w: magic 0x%x", ErrBadTile, magic)
    }
    version := binary.LittleEndian.Uint32(data[4:])
    if version < 1 || version > tileVersion {
        return nil, fmt.Errorf("%w: version %d", ErrBadTile, version)
    }

    col := int32(binary.LittleEndian.Uint32(data[8:]))
    row := int32(binary.LittleEndian.Uint32(data[12:]))
    climb := int32(binary.LittleEndian.Uint32(data[16:]))
    polyCount := int(binary.LittleEndian.Uint32(data[20:]))
    linkCount := int(binary.LittleEndian.Uint32(data[24:]))
    extCount := int(binary.LittleEndian.Uint32(data[28:]))
    indexCount := int(binary.LittleEndian.Uint32(data[32:]))

    tile := &Tile{
        Col:      int16(col),
        Row:      int16(row),
        Climb:    climb,
        Polys:    make([]Poly, polyCount),
        Links:    make([]Link, linkCount),
        ExtLinks: make([]ExtLink, extCount),
    }
    if version < 3 {
        tile.BVTree = make([]BVNode, indexCount)
    }
    tile.worldMinX = (float64(col) - tileZeroCol) * tileWorldSize
    tile.worldMinY = (float64(row) - tileZeroRow) * tileWorldSize

    offset := tileHeaderSize
    if version == 1 {
        if err := decodePolysV1(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeLinksV1(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeExtLinksV1(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeBVTree(data, &offset, tile); err != nil {
            return nil, err
        }
    } else if version == 2 {
        if err := decodePolys(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeLinks(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeExtLinks(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeBVTree(data, &offset, tile); err != nil {
            return nil, err
        }
    } else {
        if err := decodeTileV3(data, &offset, tile); err != nil {
            return nil, err
        }
    }
    if offset != len(data) {
        return nil, fmt.Errorf("%w: %d trailing bytes", ErrBadTile,
            len(data)-offset)
    }

    return tile, nil
}

// decodePolysV1 reads the version 1 polygon section (the int32
// bounds).
func decodePolysV1(data []byte, offset *int, tile *Tile) error {
    size := len(tile.Polys) * polyWireSizeV1
    if *offset+size > len(data) {
        return fmt.Errorf("%w: polys block truncated", ErrBadTile)
    }
    for i := range tile.Polys {
        base := *offset + i*polyWireSizeV1
        poly := &tile.Polys[i]
        poly.X0 = int32(binary.LittleEndian.Uint32(data[base:]))
        poly.Y0 = int32(binary.LittleEndian.Uint32(data[base+4:]))
        poly.X1 = int32(binary.LittleEndian.Uint32(data[base+8:]))
        poly.Y1 = int32(binary.LittleEndian.Uint32(data[base+12:]))
        poly.H00 = int16(binary.LittleEndian.Uint16(data[base+16:]))
        poly.H10 = int16(binary.LittleEndian.Uint16(data[base+18:]))
        poly.H01 = int16(binary.LittleEndian.Uint16(data[base+20:]))
        poly.H11 = int16(binary.LittleEndian.Uint16(data[base+22:]))
        poly.FirstLink = int32(binary.LittleEndian.Uint32(data[base+24:]))
        poly.Area = data[base+28]
    }
    *offset += size

    return validatePolys(tile)
}

// decodePolys reads the version 2 polygon section (the uint16
// quantized bounds).
func decodePolys(data []byte, offset *int, tile *Tile) error {
    size := len(tile.Polys) * polyWireSize
    if *offset+size > len(data) {
        return fmt.Errorf("%w: polys block truncated", ErrBadTile)
    }
    for i := range tile.Polys {
        base := *offset + i*polyWireSize
        poly := &tile.Polys[i]
        poly.X0 = int32(binary.LittleEndian.Uint16(data[base:]))
        poly.Y0 = int32(binary.LittleEndian.Uint16(data[base+2:]))
        poly.X1 = int32(binary.LittleEndian.Uint16(data[base+4:]))
        poly.Y1 = int32(binary.LittleEndian.Uint16(data[base+6:]))
        poly.H00 = int16(binary.LittleEndian.Uint16(data[base+8:]))
        poly.H10 = int16(binary.LittleEndian.Uint16(data[base+10:]))
        poly.H01 = int16(binary.LittleEndian.Uint16(data[base+12:]))
        poly.H11 = int16(binary.LittleEndian.Uint16(data[base+14:]))
        poly.FirstLink = int32(binary.LittleEndian.Uint32(data[base+16:]))
        poly.Area = data[base+20]
    }
    *offset += size

    return validatePolys(tile)
}

// validatePolys checks the decoded polygon bounds (the shared
// invariant of both wire versions).
func validatePolys(tile *Tile) error {
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        if poly.X0 < 0 || poly.Y0 < 0 || poly.X1 <= poly.X0 ||
            poly.Y1 <= poly.Y0 || poly.X1 > regionCellsSide ||
            poly.Y1 > regionCellsSide {
            return fmt.Errorf("%w: poly %d bounds %d %d %d %d",
                ErrBadTile, i, poly.X0, poly.Y0, poly.X1, poly.Y1)
        }
    }

    return nil
}

// decodeLinksV1 reads the version 1 link section (the int32 spans)
// and validates the chains.
func decodeLinksV1(data []byte, offset *int, tile *Tile) error {
    size := len(tile.Links) * linkWireSizeV1
    if *offset+size > len(data) {
        return fmt.Errorf("%w: links block truncated", ErrBadTile)
    }
    for i := range tile.Links {
        base := *offset + i*linkWireSizeV1
        link := &tile.Links[i]
        link.Side = data[base]
        link.To = int32(binary.LittleEndian.Uint32(data[base+4:]))
        link.Next = int32(binary.LittleEndian.Uint32(data[base+8:]))
        link.T0 = int32(binary.LittleEndian.Uint32(data[base+12:]))
        link.T1 = int32(binary.LittleEndian.Uint32(data[base+16:]))
    }
    *offset += size

    return validateLinks(tile)
}

// decodeLinks reads the version 2 link section (the uint16 spans)
// and validates the chains.
func decodeLinks(data []byte, offset *int, tile *Tile) error {
    size := len(tile.Links) * linkWireSize
    if *offset+size > len(data) {
        return fmt.Errorf("%w: links block truncated", ErrBadTile)
    }
    for i := range tile.Links {
        base := *offset + i*linkWireSize
        link := &tile.Links[i]
        link.Side = data[base]
        link.To = int32(binary.LittleEndian.Uint32(data[base+4:]))
        link.Next = int32(binary.LittleEndian.Uint32(data[base+8:]))
        link.T0 = int32(binary.LittleEndian.Uint16(data[base+12:]))
        link.T1 = int32(binary.LittleEndian.Uint16(data[base+14:]))
    }
    *offset += size

    return validateLinks(tile)
}

// validateLinks checks the decoded link chains (the shared invariant
// of both wire versions).
func validateLinks(tile *Tile) error {
    for i := range tile.Links {
        link := &tile.Links[i]
        if link.Side > SideMaxY {
            return fmt.Errorf("%w: link %d side %d", ErrBadTile, i,
                link.Side)
        }
        if link.To >= int32(len(tile.Polys)) ||
            link.To < -int32(len(tile.ExtLinks)) {
            return fmt.Errorf("%w: link %d target %d", ErrBadTile, i,
                link.To)
        }
        if link.Next >= int32(len(tile.Links)) {
            return fmt.Errorf("%w: link %d next %d", ErrBadTile, i,
                link.Next)
        }
        if link.T0 > link.T1 {
            return fmt.Errorf("%w: link %d span %d..%d", ErrBadTile, i,
                link.T0, link.T1)
        }
    }

    return nil
}

// decodeExtLinksV1 reads the version 1 external link section (the
// int32 region keys).
func decodeExtLinksV1(data []byte, offset *int, tile *Tile) error {
    size := len(tile.ExtLinks) * extLinkWireSizeV1
    if *offset+size > len(data) {
        return fmt.Errorf("%w: ext links block truncated", ErrBadTile)
    }
    for i := range tile.ExtLinks {
        base := *offset + i*extLinkWireSizeV1
        ext := &tile.ExtLinks[i]
        ext.Col = int32(binary.LittleEndian.Uint32(data[base:]))
        ext.Row = int32(binary.LittleEndian.Uint32(data[base+4:]))
        ext.Poly = binary.LittleEndian.Uint32(data[base+8:])
    }
    *offset += size

    return nil
}

// decodeExtLinks reads the version 2 external link section (the
// int16 region keys).
func decodeExtLinks(data []byte, offset *int, tile *Tile) error {
    size := len(tile.ExtLinks) * extLinkWireSize
    if *offset+size > len(data) {
        return fmt.Errorf("%w: ext links block truncated", ErrBadTile)
    }
    for i := range tile.ExtLinks {
        base := *offset + i*extLinkWireSize
        ext := &tile.ExtLinks[i]
        ext.Col = int32(int16(binary.LittleEndian.Uint16(data[base:])))
        ext.Row = int32(int16(binary.LittleEndian.Uint16(data[base+2:])))
        ext.Poly = binary.LittleEndian.Uint32(data[base+4:])
    }
    *offset += size

    return nil
}

// decodeBVTree reads the bounding volume tree section.
func decodeBVTree(data []byte, offset *int, tile *Tile) error {
    size := len(tile.BVTree) * bvNodeWireSize
    if *offset+size > len(data) {
        return fmt.Errorf("%w: bvtree block truncated", ErrBadTile)
    }
    for i := range tile.BVTree {
        base := *offset + i*bvNodeWireSize
        node := &tile.BVTree[i]
        for a := range 3 {
            node.BMin[a] = binary.LittleEndian.Uint16(data[base+a*2:])
            node.BMax[a] = binary.LittleEndian.Uint16(data[base+6+a*2:])
        }
        node.I = int32(binary.LittleEndian.Uint32(data[base+12:]))
    }
    *offset += size

    return nil
}

// packLinkSpan folds the side and the crossing span into one word
// (the side 2 bits, the t0 and the t1 12 bits each: the spans fit the
// 2048 cell region side).
func packLinkSpan(side uint8, t0, t1 int32) uint32 {
    return uint32(side) | uint32(t0)<<2 | uint32(t1)<<14
}

// unpackLinkSpan splits the packed span word.
func unpackLinkSpan(word uint32) (side uint8, t0, t1 int32) {
    return uint8(word & 3), int32(word >> 2 & 0xFFF), int32(word >> 14 & 0xFFF)
}

// zigzag folds a signed delta into the uvarint range.
func zigzag(v int64) uint64 { return uint64(v<<1) ^ uint64(v>>63) }

// unzigzag unfolds the signed delta.
func unzigzag(v uint64) int64 { return int64(v>>1) ^ -int64(v&1) }

// buildGridIndex buckets the polygon rectangles into the uniform grid
// (the encode side of the v3 spatial index). The entries of one
// bucket run in increasing polygon order and the stream stores the
// per bucket zigzag uvarint deltas.
func buildGridIndex(polys []Poly) (offsets []uint32, stream []byte,
    entries int,
) {
    counts := make([]uint32, gridBuckets)
    for i := range polys {
        poly := &polys[i]
        bx0 := int(poly.X0) / gridBucketCells
        bx1 := (int(poly.X1) - 1) / gridBucketCells
        by0 := int(poly.Y0) / gridBucketCells
        by1 := (int(poly.Y1) - 1) / gridBucketCells
        for by := by0; by <= by1; by++ {
            base := by * gridSide
            for bx := bx0; bx <= bx1; bx++ {
                counts[base+bx]++
            }
        }
    }
    offsets = make([]uint32, gridBuckets+1)
    total := 0
    for b := 0; b < gridBuckets; b++ {
        offsets[b] = uint32(total)
        total += int(counts[b])
    }
    offsets[gridBuckets] = uint32(total)
    fill := make([]uint32, gridBuckets)
    copy(fill, offsets[:gridBuckets])
    ids := make([]uint32, total)
    for i := range polys {
        poly := &polys[i]
        bx0 := int(poly.X0) / gridBucketCells
        bx1 := (int(poly.X1) - 1) / gridBucketCells
        by0 := int(poly.Y0) / gridBucketCells
        by1 := (int(poly.Y1) - 1) / gridBucketCells
        for by := by0; by <= by1; by++ {
            base := by * gridSide
            for bx := bx0; bx <= bx1; bx++ {
                ids[fill[base+bx]] = uint32(i)
                fill[base+bx]++
            }
        }
    }
    stream = make([]byte, 0, total*2)
    for b := 0; b < gridBuckets; b++ {
        prev := uint64(0)
        for e := offsets[b]; e < offsets[b+1]; e++ {
            stream = binary.AppendUvarint(stream, uint64(ids[e])-prev)
            prev = uint64(ids[e])
        }
    }

    return offsets, stream, total
}

// EncodeTile serializes a tile into the version 3 wire format: the
// columnar polygon planes, the poly ordered link chains (the CSR
// offsets, no Next on the wire), the packed span words, the zigzag
// uvarint target deltas and the bucket grid index. The derived world
// anchor is NOT part of the wire format - it recomputes from the
// region key.
func EncodeTile(tile *Tile) ([]byte, error) {
    if len(tile.Polys) == 0 {
        return nil, fmt.Errorf("%w: encode of an empty tile", ErrBadTile)
    }
    if int64(len(tile.Polys)) >= 1<<31 {
        return nil, fmt.Errorf("%w: poly count %d", ErrBadTile,
            len(tile.Polys))
    }
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        if poly.X0 < 0 || poly.Y0 < 0 || poly.X1 <= poly.X0 ||
            poly.Y1 <= poly.Y0 || poly.X1 > regionCellsSide ||
            poly.Y1 > regionCellsSide {
            return nil, fmt.Errorf(
                "%w: poly %d bounds %d %d %d %d overflow "+
                    "the uint16 cell units", ErrBadTile, i,
                poly.X0, poly.Y0, poly.X1, poly.Y1)
        }
    }
    for i := range tile.Links {
        link := &tile.Links[i]
        if link.T0 < 0 || link.T1 > regionCellsSide {
            return nil, fmt.Errorf("%w: link %d span %d..%d "+
                "overflows the uint16 cell units", ErrBadTile,
                i, link.T0, link.T1)
        }
    }

    // The link chains: the CSR offsets, the span plane and the
    // zigzag uvarint target deltas (the delta against the source
    // polygon keeps the neighbour targets in the one byte range).
    offsets := make([]uint32, len(tile.Polys)+1)
    spans := make([]uint32, 0, len(tile.Links))
    toStream := make([]byte, 0, len(tile.Links)*2)
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        chain := 0
        for li := poly.FirstLink; li >= 0; li = tile.Links[li].Next {
            link := &tile.Links[li]
            spans = append(spans,
                packLinkSpan(link.Side, link.T0, link.T1))
            toStream = binary.AppendUvarint(toStream,
                zigzag(int64(link.To)-int64(i)))
            chain++
        }
        offsets[i+1] = offsets[i] + uint32(chain)
    }
    if len(spans) != len(tile.Links) {
        return nil, fmt.Errorf("%w: the link chains cover %d of %d "+
            "links", ErrBadTile, len(spans), len(tile.Links))
    }

    gridOffsets, gridStream, gridEntries := buildGridIndex(tile.Polys)

    size := tileHeaderSize +
        8*2*len(tile.Polys) + // the bounds and the height planes
        len(tile.Polys) + // the area plane
        4*(len(tile.Polys)+1) + // the CSR
        4*len(spans) +
        len(toStream) +
        len(tile.ExtLinks)*extLinkWireSize +
        4*(gridBuckets+1) +
        len(gridStream)
    data := make([]byte, size)

    binary.LittleEndian.PutUint32(data[0:], tileMagic)
    binary.LittleEndian.PutUint32(data[4:], tileVersion)
    binary.LittleEndian.PutUint32(data[8:], uint32(int32(tile.Col)))
    binary.LittleEndian.PutUint32(data[12:], uint32(int32(tile.Row)))
    binary.LittleEndian.PutUint32(data[16:], uint32(tile.Climb))
    binary.LittleEndian.PutUint32(data[20:], uint32(len(tile.Polys)))
    binary.LittleEndian.PutUint32(data[24:], uint32(len(tile.Links)))
    binary.LittleEndian.PutUint32(data[28:], uint32(len(tile.ExtLinks)))
    binary.LittleEndian.PutUint32(data[32:], uint32(gridEntries))
    binary.LittleEndian.PutUint32(data[36:], uint32(len(toStream)))

    offset := tileHeaderSize
    put16 := func(off int, v uint16) {
        binary.LittleEndian.PutUint16(data[off:], v)
    }
    put32 := func(off int, v uint32) {
        binary.LittleEndian.PutUint32(data[off:], v)
    }
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        put16(offset+i*2, uint16(poly.X0))
        put16(offset+(len(tile.Polys)+i)*2, uint16(poly.Y0))
        put16(offset+(2*len(tile.Polys)+i)*2, uint16(poly.X1))
        put16(offset+(3*len(tile.Polys)+i)*2, uint16(poly.Y1))
        put16(offset+(4*len(tile.Polys)+i)*2, uint16(poly.H00))
        put16(offset+(5*len(tile.Polys)+i)*2, uint16(poly.H10))
        put16(offset+(6*len(tile.Polys)+i)*2, uint16(poly.H01))
        put16(offset+(7*len(tile.Polys)+i)*2, uint16(poly.H11))
    }
    offset += 8 * 2 * len(tile.Polys)
    for i := range tile.Polys {
        data[offset+i] = tile.Polys[i].Area
    }
    offset += len(tile.Polys)
    for i, v := range offsets {
        put32(offset+i*4, v)
    }
    offset += 4 * len(offsets)
    for i, span := range spans {
        put32(offset+i*4, span)
    }
    offset += 4 * len(spans)
    copy(data[offset:], toStream)
    offset += len(toStream)

    for i := range tile.ExtLinks {
        base := offset + i*extLinkWireSize
        ext := &tile.ExtLinks[i]
        put16(base, uint16(ext.Col))
        put16(base+2, uint16(ext.Row))
        put32(base+4, ext.Poly)
    }
    offset += len(tile.ExtLinks) * extLinkWireSize

    for i, v := range gridOffsets {
        put32(offset+i*4, v)
    }
    offset += 4 * len(gridOffsets)
    copy(data[offset:], gridStream)
    offset += len(gridStream)

    if offset != len(data) {
        return nil, fmt.Errorf("%w: the v3 encode wrote %d of %d bytes",
            ErrBadTile, offset, len(data))
    }

    return data, nil
}

// decodeTileV3 reads the columnar version 3 sections.
func decodeTileV3(data []byte, offset *int, tile *Tile) error {
    polyN := len(tile.Polys)
    linkN := len(tile.Links)
    gridEntries := int(binary.LittleEndian.Uint32(data[32:]))
    toStreamSize := int(binary.LittleEndian.Uint32(data[36:]))

    bounds := func(plane int) func(i int) uint16 {
        base := *offset + plane*2*polyN

        return func(i int) uint16 {
            return binary.LittleEndian.Uint16(data[base+i*2:])
        }
    }
    if *offset+8*2*polyN > len(data) {
        return fmt.Errorf("%w: the v3 poly planes truncate", ErrBadTile)
    }
    x0Plane, y0Plane := bounds(0), bounds(1)
    x1Plane, y1Plane := bounds(2), bounds(3)
    h00Plane, h10Plane := bounds(4), bounds(5)
    h01Plane, h11Plane := bounds(6), bounds(7)
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        poly.X0 = int32(x0Plane(i))
        poly.Y0 = int32(y0Plane(i))
        poly.X1 = int32(x1Plane(i))
        poly.Y1 = int32(y1Plane(i))
        poly.H00 = int16(h00Plane(i))
        poly.H10 = int16(h10Plane(i))
        poly.H01 = int16(h01Plane(i))
        poly.H11 = int16(h11Plane(i))
    }
    *offset += 8 * 2 * polyN

    if *offset+polyN > len(data) {
        return fmt.Errorf("%w: the v3 area plane truncates", ErrBadTile)
    }
    for i := range tile.Polys {
        tile.Polys[i].Area = data[*offset+i]
    }
    *offset += polyN

    size := 4 * (polyN + 1)
    if *offset+size > len(data) {
        return fmt.Errorf("%w: the v3 link CSR truncates", ErrBadTile)
    }
    chainOffsets := make([]uint32, polyN+1)
    for i := 0; i <= polyN; i++ {
        chainOffsets[i] = binary.LittleEndian.Uint32(data[*offset+i*4:])
    }
    if polyN > 0 && chainOffsets[0] != 0 {
        return fmt.Errorf("%w: the v3 link CSR head %d", ErrBadTile,
            chainOffsets[0])
    }
    for i := 0; i < polyN; i++ {
        if chainOffsets[i] > chainOffsets[i+1] ||
            chainOffsets[i+1] > uint32(linkN) {
            return fmt.Errorf("%w: the v3 link CSR step %d", ErrBadTile, i)
        }
    }
    if int(chainOffsets[polyN]) != linkN {
        return fmt.Errorf("%w: the v3 link CSR tail %d of %d",
            ErrBadTile, chainOffsets[polyN], linkN)
    }
    *offset += size

    size = 4 * linkN
    if *offset+size > len(data) {
        return fmt.Errorf("%w: the v3 span plane truncates", ErrBadTile)
    }
    for i := range tile.Links {
        side, t0, t1 := unpackLinkSpan(
            binary.LittleEndian.Uint32(data[*offset+i*4:]))
        tile.Links[i].Side = side
        tile.Links[i].T0 = t0
        tile.Links[i].T1 = t1
    }
    *offset += size

    if *offset+toStreamSize > len(data) {
        return fmt.Errorf("%w: the v3 target stream truncates", ErrBadTile)
    }
    cursor := *offset
    for i := 0; i < polyN; i++ {
        poly := &tile.Polys[i]
        end := chainOffsets[i+1]
        if end > chainOffsets[i] {
            poly.FirstLink = int32(chainOffsets[i])
        } else {
            // The isolated surface: the empty chain answers -1 (the
            // wire CSR only carries the non empty chains).
            poly.FirstLink = -1
        }
        for e := chainOffsets[i]; e < end; e++ {
            zz, read := binary.Uvarint(data[cursor:])
            if read <= 0 {
                return fmt.Errorf("%w: the v3 target stream breaks",
                    ErrBadTile)
            }
            cursor += read
            to := unzigzag(zz) + int64(i)
            tile.Links[e].To = int32(to)
            if e+1 < end {
                tile.Links[e].Next = int32(e + 1)
            } else {
                tile.Links[e].Next = -1
            }
        }
    }
    if cursor != *offset+toStreamSize {
        return fmt.Errorf("%w: the v3 target stream overruns", ErrBadTile)
    }
    *offset += toStreamSize

    if err := decodeExtLinks(data, offset, tile); err != nil {
        return err
    }

    size = 4 * (gridBuckets + 1)
    if *offset+size > len(data) {
        return fmt.Errorf("%w: the v3 grid offsets truncate", ErrBadTile)
    }
    grid := &TileGrid{
        Offsets: make([]uint32, gridBuckets+1),
    }
    for i := range grid.Offsets {
        grid.Offsets[i] = binary.LittleEndian.Uint32(data[*offset+i*4:])
    }
    if int(grid.Offsets[gridBuckets]) != gridEntries {
        return fmt.Errorf("%w: the v3 grid tail %d of %d", ErrBadTile,
            grid.Offsets[gridBuckets], gridEntries)
    }
    *offset += size

    grid.Entries = make([]uint32, gridEntries)
    position := *offset
    for b := 0; b < gridBuckets; b++ {
        prev := uint64(0)
        for e := grid.Offsets[b]; e < grid.Offsets[b+1]; e++ {
            delta, read := binary.Uvarint(data[position:])
            if read <= 0 {
                return fmt.Errorf("%w: the v3 grid stream breaks",
                    ErrBadTile)
            }
            position += read
            prev += delta
            grid.Entries[e] = uint32(prev)
        }
    }
    if position != len(data) {
        return fmt.Errorf("%w: the v3 grid stream overruns", ErrBadTile)
    }
    *offset = len(data)
    tile.Grid = grid

    if err := validatePolys(tile); err != nil {
        return err
    }

    return validateLinks(tile)
}
