// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "encoding/binary"
    "fmt"
    "math"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The NMV2 geometry encoder of the navmesh viewer. The payload of one
// tile carries three blocks beyond the header:
//
//   - the polygon corners: four quantized corner triples per
//     rectangle (the surface quads the viewer tessellates),
//   - the link portals: one record per polygon-to-polygon connection
//     with the world segment of the open span and the area class of
//     the pair (the edge connections overlay: green fields, blue
//     water, teal shore),
//   - the height step walls: the vertical filler quads between the
//     bilinear surfaces of adjacent rectangles. The corner heights of
//     a polygon come from its own inside cells, so two rectangles
//     sharing an edge generally disagree about the height of that
//     edge - the honest staircase of the geodata seen through the
//     bilinear approximation. Without the filler the disagreement
//     reads as see-through black wedges (the defect of the previous
//     round: the clear color behind a cracked surface).
//
// The walls are emitted only through the MaxX and MaxY sides of the
// emitting polygon, so every shared edge is walled exactly once; the
// region borders resolve their targets in the east and north neighbor
// tiles through the mesh (a missing neighbor tile simply leaves the
// seam open - it merges with the void of the unloaded region).

// navmeshGeoMagic is the magic word of the NMV2 geometry payload
// ('NMV2' little endian). The fixed header of navmeshGeoHeaderSize
// bytes follows: magic u32, col i16, row i16, worldMinX i32,
// worldMinY i32, minH i16, maxH i16, polyCount u32, linkCount u32,
// wallCount u32; then the corner block, the padded area tail, the
// link records and the wall records described on the stride
// constants.
const navmeshGeoMagic = 0x32564D4E

// navmeshGeoHeaderSize is the fixed prefix of the geometry payload.
const navmeshGeoHeaderSize = 32

// navmeshGeoCornerStride is the wire size of one polygon corner
// triple (cellX, cellY, height as int16).
const navmeshGeoCornerStride = 6

// navmeshGeoRecordSize is the wire size of one polygon: four corner
// triples plus the area byte. The layout keeps the corner block
// contiguous (every polygon contributes twelve int16 values in a row,
// the corner order X0Y0 X1Y0 X0Y1 X1Y1 - the world position of a
// corner is worldMin + cell*16) with the area bytes as one padded
// tail block, so the viewer reads the corners as a single Int16Array
// view. The triangles themselves never ride the wire: every polygon
// is its own quad of four consecutive corners, the viewer
// tessellates (0, 2, 1) (0, 2, 3).
const navmeshGeoRecordSize = navmeshGeoCornerStride*4 + 1

// navmeshGeoLinkStride is the wire size of one link portal record:
// ax u16, ay u16, ah i16, bx u16, by u16, bh i16, class u8, three
// pad bytes. The coordinates are tile local world offsets (multiples
// of sixteen, 0..32768), the heights are the polygon surface at the
// portal ends rounded to whole units.
const navmeshGeoLinkStride = 16

// navmeshGeoWallStride is the wire size of one height step wall
// record: fixed u16, lo u16, hi u16, hA0 i16, hA1 i16, hB0 i16,
// hB1 i16, areaA u8, orient u8, two pad bytes. The fixed coordinate
// names the shared edge (a multiple of sixteen); lo and hi bound the
// wall along the crossing axis in tile local world offsets (whole
// units - a crossing split may land inside a cell); hA0/hA1 are the
// emitter surface heights at lo/hi, hB0/hB1 the target surface
// heights; orient 0 is a vertical wall at a fixed x, 1 a horizontal
// wall at a fixed y.
const navmeshGeoWallStride = 16

// The area classes of the link portal records.
const (
    // navmeshLinkGround connects two ground polygons.
    navmeshLinkGround uint8 = 0
    // navmeshLinkWater connects two water polygons.
    navmeshLinkWater uint8 = 1
    // navmeshLinkShore connects a ground and a water polygon.
    navmeshLinkShore uint8 = 2
    // navmeshLinkUnknown connects into a tile that did not resolve
    // (absent neighbor region or a stale external target).
    navmeshLinkUnknown uint8 = 3
)

// The wall orientations: the axis the shared edge runs along.
const (
    // navmeshWallVertical is a wall at a fixed x spanning lo..hi in y.
    navmeshWallVertical uint8 = 0
    // navmeshWallHorizontal is a wall at a fixed y spanning lo..hi in x.
    navmeshWallHorizontal uint8 = 1
)

// navmeshWallMinStep is the smallest height difference that still
// emits a wall: below half a unit the crack is subpixel at every
// viewing distance of the stitched world.
const navmeshWallMinStep = 0.5

// geoTarget is one rectangle interval a wall may close onto: the
// target polygon with its cell range along the crossing axis.
type geoTarget struct {
    poly *navmesh.Poly
    lo   int32
    hi   int32
}

// encodeNavmeshGeometry renders one tile into the binary NMV2 payload
// the viewer draws: the surface quads, the link portals of the
// connection overlay and the height step walls that close the cracks
// between the bilinear surfaces of adjacent rectangles. The mesh
// resolves the neighbor tiles of the region border walls and the
// external link areas.
func encodeNavmeshGeometry(mesh *navmesh.Mesh, tile *navmesh.Tile,
) ([]byte, error) {
    if len(tile.Polys) == 0 {
        return nil, fmt.Errorf("tile %d_%d holds no polygons", tile.Col,
            tile.Row)
    }
    if int64(len(tile.Polys))*navmeshGeoRecordSize > 1<<31 {
        return nil, fmt.Errorf("tile %d_%d too large: %d polys", tile.Col,
            tile.Row, len(tile.Polys))
    }

    links := encodeNavmeshLinks(mesh, tile)
    walls := encodeNavmeshWalls(mesh, tile)

    // The payload layout: the header, the contiguous corner block, the
    // area tail padded to the four byte alignment, the link portal
    // block and the wall block.
    size := navmeshGeoHeaderSize +
        len(tile.Polys)*navmeshGeoCornerStride*4 +
        geoPad4(len(tile.Polys)) + len(links) + len(walls)
    payload := make([]byte, size)
    minH, maxH := geoHeightRange(tile)
    binary.LittleEndian.PutUint32(payload[0:], navmeshGeoMagic)
    binary.LittleEndian.PutUint16(payload[4:], uint16(tile.Col))
    binary.LittleEndian.PutUint16(payload[6:], uint16(tile.Row))
    binary.LittleEndian.PutUint32(payload[8:],
        uint32(int32(tile.WorldMinX())))
    binary.LittleEndian.PutUint32(payload[12:],
        uint32(int32(tile.WorldMinY())))
    binary.LittleEndian.PutUint16(payload[16:], uint16(minH))
    binary.LittleEndian.PutUint16(payload[18:], uint16(maxH))
    binary.LittleEndian.PutUint32(payload[20:], uint32(len(tile.Polys)))
    binary.LittleEndian.PutUint32(payload[24:],
        uint32(len(links)/navmeshGeoLinkStride))
    binary.LittleEndian.PutUint32(payload[28:],
        uint32(len(walls)/navmeshGeoWallStride))

    offset := navmeshGeoHeaderSize
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        writeGeoCorner(payload, offset, poly.X0, poly.Y0, int32(poly.H00))
        writeGeoCorner(payload, offset+6, poly.X1, poly.Y0, int32(poly.H10))
        writeGeoCorner(payload, offset+12, poly.X0, poly.Y1, int32(poly.H01))
        writeGeoCorner(payload, offset+18, poly.X1, poly.Y1, int32(poly.H11))
        offset += navmeshGeoCornerStride * 4
    }
    for i := range tile.Polys {
        payload[offset+i] = tile.Polys[i].Area
    }
    offset += geoPad4(len(tile.Polys))
    offset += copy(payload[offset:], links)
    offset += copy(payload[offset:], walls)
    if offset != len(payload) {
        return nil, fmt.Errorf("tile %d_%d geometry size mismatch: %d of %d",
            tile.Col, tile.Row, offset, len(payload))
    }

    return payload, nil
}

// geoHeightRange scans the corner heights of the tile polygons.
func geoHeightRange(tile *navmesh.Tile) (int16, int16) {
    minH, maxH := int16(32767), int16(-32768)
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        for _, h := range []int16{poly.H00, poly.H10, poly.H01, poly.H11} {
            if h < minH {
                minH = h
            }
            if h > maxH {
                maxH = h
            }
        }
    }

    return minH, maxH
}

// geoPad4 rounds a block size up to the four byte alignment of the
// payload sections that follow it.
func geoPad4(size int) int {
    return (size + 3) &^ 3
}

// encodeNavmeshLinks renders the link portal block: every connection
// of every polygon as its open world span with the area class of the
// pair.
func encodeNavmeshLinks(mesh *navmesh.Mesh, tile *navmesh.Tile) []byte {
    links := make([]byte, 0, len(tile.Links)*navmeshGeoLinkStride)
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        for idx := poly.FirstLink; idx >= 0 &&
            int(idx) < len(tile.Links); idx = tile.Links[idx].Next {
            link := &tile.Links[idx]
            ax, ay, bx, by := tile.Portal(poly, link)
            class := navmeshLinkClass(mesh, tile, poly, link)
            base := len(links)
            links = append(links, make([]byte, navmeshGeoLinkStride)...)
            binary.LittleEndian.PutUint16(links[base:],
                geoOffset(ax-tile.WorldMinX()))
            binary.LittleEndian.PutUint16(links[base+2:],
                geoOffset(ay-tile.WorldMinY()))
            binary.LittleEndian.PutUint16(links[base+4:],
                geoHeight(tile.HeightAt(poly, ax, ay)))
            binary.LittleEndian.PutUint16(links[base+6:],
                geoOffset(bx-tile.WorldMinX()))
            binary.LittleEndian.PutUint16(links[base+8:],
                geoOffset(by-tile.WorldMinY()))
            binary.LittleEndian.PutUint16(links[base+10:],
                geoHeight(tile.HeightAt(poly, bx, by)))
            links[base+12] = class
        }
    }

    return links
}

// navmeshLinkClass names the area pair of one link: the internal
// target reads the neighbor polygon of the same tile, the external
// target loads the neighbor region through the mesh. A target that
// does not resolve keeps the unknown class - the overlay draws it
// gray instead of guessing.
func navmeshLinkClass(mesh *navmesh.Mesh, tile *navmesh.Tile,
    poly *navmesh.Poly, link *navmesh.Link,
) uint8 {
    var area uint8
    if link.To >= 0 {
        if int(link.To) >= len(tile.Polys) {
            return navmeshLinkUnknown
        }
        area = tile.Polys[link.To].Area
    } else {
        extIdx := -link.To - 1
        if mesh == nil || int(extIdx) >= len(tile.ExtLinks) {
            return navmeshLinkUnknown
        }
        ext := &tile.ExtLinks[extIdx]
        neighbor, err := mesh.Tile(navmesh.RegionKey{
            Col: int16(ext.Col), Row: int16(ext.Row)})
        if err != nil || neighbor == nil ||
            int(ext.Poly) >= len(neighbor.Polys) {
            return navmeshLinkUnknown
        }
        area = neighbor.Polys[ext.Poly].Area
    }
    if area != poly.Area {
        return navmeshLinkShore
    }
    if area == navmesh.AreaWater {
        return navmeshLinkWater
    }

    return navmeshLinkGround
}

// encodeNavmeshWalls renders the height step wall block. The walls
// leave through the MaxX and MaxY sides of the emitting polygon, so
// every shared edge is walled exactly once across the whole mesh; the
// region border emitters resolve their targets in the east and north
// neighbor tiles.
func encodeNavmeshWalls(mesh *navmesh.Mesh, tile *navmesh.Tile) []byte {
    side := int32(2048)
    targetsX := make(map[int32][]geoTarget, 64)
    targetsY := make(map[int32][]geoTarget, 64)
    for i := range tile.Polys {
        p := &tile.Polys[i]
        targetsX[p.X0] = append(targetsX[p.X0],
            geoTarget{poly: p, lo: p.Y0, hi: p.Y1})
        targetsY[p.Y0] = append(targetsY[p.Y0],
            geoTarget{poly: p, lo: p.X0, hi: p.X1})
    }
    // The region border emitters close onto the border facing spans
    // of the neighbor tiles: the MinX == 0 spans of the east neighbor
    // share the y axis, the MinY == 0 spans of the north neighbor
    // share the x axis.
    east := navmeshBorderTargets(mesh, navmesh.RegionKey{
        Col: tile.Col + 1, Row: tile.Row}, false)
    north := navmeshBorderTargets(mesh, navmesh.RegionKey{
        Col: tile.Col, Row: tile.Row + 1}, true)

    writer := make([]byte, 0, len(tile.Polys)*navmeshGeoWallStride)
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        if poly.X1 < side {
            for _, target := range targetsX[poly.X1] {
                geoWallVertical(&writer, poly, target)
            }
        } else {
            for _, target := range east {
                geoWallVertical(&writer, poly, target)
            }
        }
        if poly.Y1 < side {
            for _, target := range targetsY[poly.Y1] {
                geoWallHorizontal(&writer, poly, target)
            }
        } else {
            for _, target := range north {
                geoWallHorizontal(&writer, poly, target)
            }
        }
    }

    return writer
}

// navmeshBorderTargets lists the border facing intervals of one
// neighbor tile: the MinX == 0 spans of the east neighbor for the
// vertical walls at the region border, the MinY == 0 spans of the
// north neighbor for the horizontal ones. A tile that does not
// resolve (absent region, failed decode) answers nil - the border
// seam stays open and merges with the void of the missing tile.
func navmeshBorderTargets(mesh *navmesh.Mesh,
    key navmesh.RegionKey, north bool) []geoTarget {
    if mesh == nil {
        return nil
    }
    neighbor, err := mesh.Tile(key)
    if err != nil || neighbor == nil {
        return nil
    }
    var targets []geoTarget
    for i := range neighbor.Polys {
        p := &neighbor.Polys[i]
        if north {
            if p.Y0 != 0 {
                continue
            }
            targets = append(targets,
                geoTarget{poly: p, lo: p.X0, hi: p.X1})
        } else {
            if p.X0 != 0 {
                continue
            }
            targets = append(targets,
                geoTarget{poly: p, lo: p.Y0, hi: p.Y1})
        }
    }

    return targets
}

// geoWallVertical walls the MaxX edge of the emitter against the MinX
// edge of the target: the shared x line, the overlapping y range, the
// two bilinear surface heights along it.
func geoWallVertical(writer *[]byte, a *navmesh.Poly, target geoTarget) {
    lo := max(a.Y0, target.lo)
    hi := min(a.Y1, target.hi)
    if lo >= hi {
        return
    }
    b := target.poly
    // The emitter edge runs from (X1, Y0, H10) to (X1, Y1, H11); the
    // target edge from (X0, Y0, H00) to (X0, Y1, H01).
    hA := func(v int32) float64 { return geoLerp(a.H10, a.H11, a.Y0, a.Y1, v) }
    hB := func(v int32) float64 { return geoLerp(b.H00, b.H01, b.Y0, b.Y1, v) }
    geoWallSpans(writer, float64(a.X1)*16, float64(lo)*16, float64(hi)*16,
        hA, hB, lo, hi, a.Area, navmeshWallVertical)
}

// geoWallHorizontal walls the MaxY edge of the emitter against the
// MinY edge of the target: the shared y line, the overlapping x
// range.
func geoWallHorizontal(writer *[]byte, a *navmesh.Poly, target geoTarget) {
    lo := max(a.X0, target.lo)
    hi := min(a.X1, target.hi)
    if lo >= hi {
        return
    }
    b := target.poly
    // The emitter edge runs from (X0, Y1, H01) to (X1, Y1, H11); the
    // target edge from (X0, Y0, H00) to (X1, Y0, H10).
    hA := func(v int32) float64 { return geoLerp(a.H01, a.H11, a.X0, a.X1, v) }
    hB := func(v int32) float64 { return geoLerp(b.H00, b.H10, b.X0, b.X1, v) }
    geoWallSpans(writer, float64(a.Y1)*16, float64(lo)*16, float64(hi)*16,
        hA, hB, lo, hi, a.Area, navmeshWallHorizontal)
}

// geoWallSpans emits the wall quads of one shared edge overlap: one
// quad when the height difference keeps its sign, two when the
// surfaces cross inside the span (the split keeps both quads simple).
// A span where both surfaces agree within half a unit is skipped -
// the crack is subpixel at every viewing distance.
func geoWallSpans(writer *[]byte, fixed, lo, hi float64,
    hA, hB func(int32) float64, loCell, hiCell int32, area uint8,
    orient uint8,
) {
    dLo := hA(loCell) - hB(loCell)
    dHi := hA(hiCell) - hB(hiCell)
    if math.Abs(dLo) < navmeshWallMinStep &&
        math.Abs(dHi) < navmeshWallMinStep {
        return
    }
    if dLo*dHi < 0 {
        // The surfaces cross between the overlap ends: split at the
        // crossing, quantized to whole world units.
        t := dLo / (dLo - dHi)
        mid := lo + (hi-lo)*t
        midCell := loCell + int32((mid-lo)/16)
        if midCell > loCell && midCell < hiCell {
            geoWall(writer, fixed, lo, mid, hA(loCell), hA(midCell),
                hB(loCell), hB(midCell), area, orient)
            geoWall(writer, fixed, mid, hi, hA(midCell), hA(hiCell),
                hB(midCell), hB(hiCell), area, orient)

            return
        }
    }
    geoWall(writer, fixed, lo, hi, hA(loCell), hA(hiCell), hB(loCell),
        hB(hiCell), area, orient)
}

// geoWall appends one wall quad record.
func geoWall(writer *[]byte, fixed, lo, hi, hA0, hA1, hB0, hB1 float64,
    area uint8, orient uint8,
) {
    if hi-lo < 1 {
        return
    }
    base := len(*writer)
    *writer = append(*writer, make([]byte, navmeshGeoWallStride)...)
    binary.LittleEndian.PutUint16((*writer)[base:], geoOffset(fixed))
    binary.LittleEndian.PutUint16((*writer)[base+2:], geoOffset(lo))
    binary.LittleEndian.PutUint16((*writer)[base+4:], geoOffset(hi))
    binary.LittleEndian.PutUint16((*writer)[base+6:], geoHeight(hA0))
    binary.LittleEndian.PutUint16((*writer)[base+8:], geoHeight(hA1))
    binary.LittleEndian.PutUint16((*writer)[base+10:], geoHeight(hB0))
    binary.LittleEndian.PutUint16((*writer)[base+12:], geoHeight(hB1))
    (*writer)[base+14] = area
    (*writer)[base+15] = orient
}

// geoLerp interpolates one edge height profile: the corner heights of
// the edge over the cell range it spans.
func geoLerp(h0, h1 int16, t0, t1, v int32) float64 {
    if t1 <= t0 {
        return float64(h0)
    }

    return float64(h0) + float64(h1-h0)*float64(v-t0)/float64(t1-t0)
}

// geoOffset quantizes one tile local world offset (0..32768) into the
// wire u16.
func geoOffset(v float64) uint16 {
    return uint16(math.Round(v))
}

// geoHeight quantizes one surface height into the wire i16 (the
// two's complement roundtrip of the negative depths).
func geoHeight(v float64) uint16 {
    return uint16(int16(math.Round(v)))
}

// writeGeoCorner stores one quantized polygon corner (the cell
// coordinates and the exact geodata height).
func writeGeoCorner(payload []byte, offset int, cellX, cellY, height int32) {
    binary.LittleEndian.PutUint16(payload[offset:], uint16(cellX))
    binary.LittleEndian.PutUint16(payload[offset+2:], uint16(cellY))
    binary.LittleEndian.PutUint16(payload[offset+4:], uint16(height))
}
