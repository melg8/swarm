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
// a million links). The decode reads both versions.
const (
    tileMagic = 0x31574E53 // 'SWN1' little endian

    tileVersion = 2

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
    // axis 2 = world y).
    BVTree []BVNode

    // worldMinX/worldMinY are derived at decode: the world anchor of
    // the region (the min corner of cell 0 0).
    worldMinX, worldMinY float64
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

// DecodeTile parses the raw bytes of one tile file. Both wire
// versions decode: version 1 (the int32 bounds and spans) and
// version 2 (the uint16 quantization of the current encoder).
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
    bvCount := int(binary.LittleEndian.Uint32(data[32:]))

    tile := &Tile{
        Col:      int16(col),
        Row:      int16(row),
        Climb:    climb,
        Polys:    make([]Poly, polyCount),
        Links:    make([]Link, linkCount),
        ExtLinks: make([]ExtLink, extCount),
        BVTree:   make([]BVNode, bvCount),
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
    } else {
        if err := decodePolys(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeLinks(data, &offset, tile); err != nil {
            return nil, err
        }
        if err := decodeExtLinks(data, &offset, tile); err != nil {
            return nil, err
        }
    }
    if err := decodeBVTree(data, &offset, tile); err != nil {
        return nil, err
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

// EncodeTile serializes a tile into the version 2 wire format (the
// uint16 quantized bounds, spans and external region keys). The
// derived world anchor is NOT part of the wire format - it recomputes
// from the region key.
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
    size := tileHeaderSize + len(tile.Polys)*polyWireSize +
        len(tile.Links)*linkWireSize + len(tile.ExtLinks)*extLinkWireSize +
        len(tile.BVTree)*bvNodeWireSize
    data := make([]byte, size)

    binary.LittleEndian.PutUint32(data[0:], tileMagic)
    binary.LittleEndian.PutUint32(data[4:], tileVersion)
    binary.LittleEndian.PutUint32(data[8:], uint32(int32(tile.Col)))
    binary.LittleEndian.PutUint32(data[12:], uint32(int32(tile.Row)))
    binary.LittleEndian.PutUint32(data[16:], uint32(tile.Climb))
    binary.LittleEndian.PutUint32(data[20:], uint32(len(tile.Polys)))
    binary.LittleEndian.PutUint32(data[24:], uint32(len(tile.Links)))
    binary.LittleEndian.PutUint32(data[28:], uint32(len(tile.ExtLinks)))
    binary.LittleEndian.PutUint32(data[32:], uint32(len(tile.BVTree)))

    offset := tileHeaderSize
    for i := range tile.Polys {
        base := offset + i*polyWireSize
        poly := &tile.Polys[i]
        binary.LittleEndian.PutUint16(data[base:], uint16(poly.X0))
        binary.LittleEndian.PutUint16(data[base+2:], uint16(poly.Y0))
        binary.LittleEndian.PutUint16(data[base+4:], uint16(poly.X1))
        binary.LittleEndian.PutUint16(data[base+6:], uint16(poly.Y1))
        binary.LittleEndian.PutUint16(data[base+8:], uint16(poly.H00))
        binary.LittleEndian.PutUint16(data[base+10:], uint16(poly.H10))
        binary.LittleEndian.PutUint16(data[base+12:], uint16(poly.H01))
        binary.LittleEndian.PutUint16(data[base+14:], uint16(poly.H11))
        binary.LittleEndian.PutUint32(data[base+16:], uint32(poly.FirstLink))
        data[base+20] = poly.Area
    }
    offset += len(tile.Polys) * polyWireSize

    for i := range tile.Links {
        base := offset + i*linkWireSize
        link := &tile.Links[i]
        data[base] = link.Side
        binary.LittleEndian.PutUint32(data[base+4:], uint32(link.To))
        binary.LittleEndian.PutUint32(data[base+8:], uint32(link.Next))
        binary.LittleEndian.PutUint16(data[base+12:], uint16(link.T0))
        binary.LittleEndian.PutUint16(data[base+14:], uint16(link.T1))
    }
    offset += len(tile.Links) * linkWireSize

    for i := range tile.ExtLinks {
        base := offset + i*extLinkWireSize
        ext := &tile.ExtLinks[i]
        binary.LittleEndian.PutUint16(data[base:], uint16(ext.Col))
        binary.LittleEndian.PutUint16(data[base+2:], uint16(ext.Row))
        binary.LittleEndian.PutUint32(data[base+4:], ext.Poly)
    }
    offset += len(tile.ExtLinks) * extLinkWireSize

    for i := range tile.BVTree {
        base := offset + i*bvNodeWireSize
        node := &tile.BVTree[i]
        for a := range 3 {
            binary.LittleEndian.PutUint16(data[base+a*2:], node.BMin[a])
            binary.LittleEndian.PutUint16(data[base+6+a*2:], node.BMax[a])
        }
        binary.LittleEndian.PutUint32(data[base+12:], uint32(node.I))
    }

    return data, nil
}
