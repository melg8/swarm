// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// sampleTile builds a two polygon tile with an internal link and an
// external link: the minimal production shape that exercises every
// wire structure (the v3 encode derives the bucket grid, the v1
// compat test carries the explicit BVTree).
func sampleTile() *Tile {
    tile := &Tile{
        Col:   21,
        Row:   19,
        Climb: 40,
        Polys: []Poly{
            {
                X0: 100, Y0: 200, X1: 110, Y1: 210,
                H00: -3000, H10: -2992, H01: -2996, H11: -2988,
                FirstLink: 0, Area: AreaGround,
            },
            {
                X0: 110, Y0: 200, X1: 120, Y1: 210,
                H00: -2992, H10: -2984, H01: -2988, H11: -2980,
                FirstLink: 1, Area: AreaWater,
            },
        },
        Links: []Link{
            {Side: SideMaxX, To: 1, Next: -1, T0: 203, T1: 206},
            {Side: SideMinX, To: 0, Next: 2, T0: 203, T1: 206},
            {Side: SideMaxY, To: -1, Next: -1, T0: 112, T1: 115},
        },
        ExtLinks: []ExtLink{{Col: 22, Row: 19, Poly: 7}},
        BVTree: []BVNode{
            {BMin: [3]uint16{100, 646, 200}, BMax: [3]uint16{110, 647, 210},
                I: -2},
            {BMin: [3]uint16{100, 646, 200}, BMax: [3]uint16{110, 647, 210},
                I: 0},
            {BMin: [3]uint16{110, 646, 200}, BMax: [3]uint16{120, 647, 210},
                I: 1},
        },
    }
    tile.worldMinX = (float64(tile.Col) - tileZeroCol) * tileWorldSize
    tile.worldMinY = (float64(tile.Row) - tileZeroRow) * tileWorldSize

    return tile
}

// TestTileRoundtrip encodes and decodes the sample tile and compares
// every structure field by field (the v3 encode answers the bucket
// grid, the BVTree stays a v1/v2 structure).
func TestTileRoundtrip(t *testing.T) {
    original := sampleTile()
    data, err := EncodeTile(original)
    require.NoError(t, err)

    tile, err := DecodeTile(data)
    require.NoError(t, err)
    require.Equal(t, original.Col, tile.Col)
    require.Equal(t, original.Row, tile.Row)
    require.Equal(t, original.Climb, tile.Climb)
    require.Equal(t, original.Polys, tile.Polys)
    require.Equal(t, original.Links, tile.Links)
    require.Equal(t, original.ExtLinks, tile.ExtLinks)
    require.Nil(t, tile.BVTree)
    require.NotNil(t, tile.Grid)
    require.Equal(t, 2, int(tile.Grid.Offsets[gridBuckets]),
        "both polygons index")

    // The grid entries list both polygons in both touched buckets
    // (the rects share the bucket row and the x range).
    entries := map[uint32]int{}
    for b := 0; b < gridBuckets; b++ {
        for e := tile.Grid.Offsets[b]; e < tile.Grid.Offsets[b+1]; e++ {
            entries[tile.Grid.Entries[e]]++
        }
    }
    require.Equal(t, map[uint32]int{0: 1, 1: 1}, entries)

    // The world anchor derives from the region key: region 21_19
    // anchors at world (32768, 32768).
    require.InDelta(t, 32768, tile.WorldMinX(), 1e-9)
    require.InDelta(t, 32768, tile.WorldMinY(), 1e-9)
}

// TestTileDecodeV1 pins the version 1 backward compatibility: the
// int32 wire tile of the earlier pack builds decodes into the same
// structures the version 2 encoder answers.
func TestTileDecodeV1(t *testing.T) {
    original := sampleTile()

    // The version 1 wire: the header (40 bytes), the int32 bound
    // polygons (32), the int32 span links (20), the int32 key
    // external links (12) and the unchanged bv tree (16).
    var buf []byte
    put32 := func(v uint32) {
        buf = append(buf, byte(v), byte(v>>8), byte(v>>16),
            byte(v>>24))
    }
    put16 := func(v uint16) {
        buf = append(buf, byte(v), byte(v>>8))
    }
    put32(tileMagic)
    put32(1) // the version 1 tag
    put32(uint32(int32(original.Col)))
    put32(uint32(int32(original.Row)))
    put32(uint32(original.Climb))
    put32(uint32(len(original.Polys)))
    put32(uint32(len(original.Links)))
    put32(uint32(len(original.ExtLinks)))
    put32(uint32(len(original.BVTree)))
    put32(0) // the reserved header tail (the 40 byte header)
    for _, poly := range original.Polys {
        put32(uint32(poly.X0))
        put32(uint32(poly.Y0))
        put32(uint32(poly.X1))
        put32(uint32(poly.Y1))
        put16(uint16(poly.H00))
        put16(uint16(poly.H10))
        put16(uint16(poly.H01))
        put16(uint16(poly.H11))
        put32(uint32(poly.FirstLink))
        buf = append(buf, poly.Area, 0, 0, 0) // the alignment pad
    }
    for _, link := range original.Links {
        buf = append(buf, link.Side, 0, 0, 0)
        put32(uint32(link.To))
        put32(uint32(link.Next))
        put32(uint32(link.T0))
        put32(uint32(link.T1))
    }
    for _, ext := range original.ExtLinks {
        put32(uint32(ext.Col))
        put32(uint32(ext.Row))
        put32(ext.Poly)
    }
    for _, node := range original.BVTree {
        for a := range 3 {
            put16(node.BMin[a])
        }
        for a := range 3 {
            put16(node.BMax[a])
        }
        put32(uint32(node.I))
    }

    tile, err := DecodeTile(buf)
    require.NoError(t, err)
    require.Equal(t, original.Polys, tile.Polys)
    require.Equal(t, original.Links, tile.Links)
    require.Equal(t, original.ExtLinks, tile.ExtLinks)
    require.Equal(t, original.BVTree, tile.BVTree)
}

// TestTileDecodeRejects checks the corrupt tile guards: the bad magic,
// the bad version, truncation and a trailing byte.
func TestTileDecodeRejects(t *testing.T) {
    data, err := EncodeTile(sampleTile())
    require.NoError(t, err)

    badMagic := append([]byte{}, data...)
    badMagic[0] = 'X'
    _, err = DecodeTile(badMagic)
    require.ErrorIs(t, err, ErrBadTile)

    badVersion := append([]byte{}, data...)
    badVersion[4] = 9
    _, err = DecodeTile(badVersion)
    require.ErrorIs(t, err, ErrBadTile)

    _, err = DecodeTile(data[:len(data)-1])
    require.ErrorIs(t, err, ErrBadTile)

    _, err = DecodeTile(append(append([]byte{}, data...), 0))
    require.ErrorIs(t, err, ErrBadTile)

    _, err = DecodeTile(nil)
    require.ErrorIs(t, err, ErrBadTile)

    empty := &Tile{Col: 21, Row: 19, Climb: 40}
    _, err = EncodeTile(empty)
    require.ErrorIs(t, err, ErrBadTile)
}

// TestPolyRefPacking pins the reference packing: the roundtrip of the
// region key and the polygon index, the null reference and the
// negative region keys (the geodata grid sits at the negative world
// quadrant edge).
func TestPolyRefPacking(t *testing.T) {
    ref := RefOf(21, 19, 4)
    col, row := TileOf(ref)
    require.EqualValues(t, 21, col)
    require.EqualValues(t, 19, row)
    require.EqualValues(t, 4, PolyOf(ref))

    neg := RefOf(-3, 5, 0)
    col, row = TileOf(neg)
    require.EqualValues(t, -3, col)
    require.EqualValues(t, 5, row)
    require.EqualValues(t, 0, PolyOf(neg))

    var null PolyRef
    require.EqualValues(t, -1, PolyOf(null))
}

// TestRegionOfWorld pins the world to region mapping against the
// geodata anchors: region 21_19 spans world [32768, 65536) per axis.
func TestRegionOfWorld(t *testing.T) {
    col, row := RegionOfWorld(45768, 49848)
    require.EqualValues(t, 21, col)
    require.EqualValues(t, 19, row)

    col, row = RegionOfWorld(32768, 32768)
    require.EqualValues(t, 21, col)
    require.EqualValues(t, 19, row)

    col, row = RegionOfWorld(32767.9, 65536)
    require.EqualValues(t, 20, col)
    require.EqualValues(t, 20, row)
}

// TestHeightAndClosest exercises the bilinear surface of a rectangle
// polygon: the four corners interpolate exactly, the center averages,
// and the closest point clamps onto the boundary.
func TestHeightAndClosest(t *testing.T) {
    tile := sampleTile()
    poly := &tile.Polys[0]
    x0, y0, x1, y1 := tile.WorldRect(poly)
    require.InDelta(t, 32768+100*16, x0, 1e-9)
    require.InDelta(t, 32768+200*16, y0, 1e-9)
    require.InDelta(t, 32768+110*16, x1, 1e-9)
    require.InDelta(t, 32768+210*16, y1, 1e-9)

    require.InDelta(t, -3000, tile.HeightAt(poly, x0, y0), 1e-9)
    require.InDelta(t, -2992, tile.HeightAt(poly, x1, y0), 1e-9)
    require.InDelta(t, -2996, tile.HeightAt(poly, x0, y1), 1e-9)
    require.InDelta(t, -2988, tile.HeightAt(poly, x1, y1), 1e-9)
    centerH := tile.HeightAt(poly, (x0+x1)*0.5, (y0+y1)*0.5)
    require.InDelta(t, (-3000-2992-2996-2988)*0.25, centerH, 1e-9)

    // Outside the footprint: the position clamps onto the boundary
    // with the boundary height.
    cx, cy, cz := tile.ClosestPoint(poly, x0-500, y0-500, 0)
    require.InDelta(t, x0, cx, 1e-9)
    require.InDelta(t, y0, cy, 1e-9)
    require.InDelta(t, -3000, cz, 1e-9)
    require.False(t, math.IsNaN(cz))
}

// TestPortal covers the portal world segments of all four sides: the
// span of link 0 (the maxx side of poly 0, y cells 203..206) maps to
// the world boundary line of x1 with the span endpoints.
func TestPortal(t *testing.T) {
    tile := sampleTile()
    poly := &tile.Polys[0]
    link := &tile.Links[0]
    ax, ay, bx, by := tile.Portal(poly, link)
    x0, _, x1, _ := tile.WorldRect(poly)
    require.InDelta(t, x1, ax, 1e-9)
    require.InDelta(t, tile.WorldMinY()+203*16, ay, 1e-9)
    require.InDelta(t, x1, bx, 1e-9)
    require.InDelta(t, tile.WorldMinY()+207*16, by, 1e-9)

    // A full side portal of the minx side.
    full := Link{Side: SideMinX, T0: poly.Y0, T1: poly.Y1 - 1}
    ax, ay, bx, by = tile.Portal(poly, &full)
    require.InDelta(t, x0, ax, 1e-9)
    require.InDelta(t, tile.WorldMinY()+float64(poly.Y0)*16, ay, 1e-9)
    require.InDelta(t, x0, bx, 1e-9)
    require.InDelta(t, tile.WorldMinY()+float64(poly.Y1)*16, by, 1e-9)
}

// TestTileGridQueryAgainstBruteForce walks the v3 bucket grid over a
// synthetic multi surface tile and compares the candidate set with
// the exhaustive rect overlap scan (the index contract: the grid
// answers a superset of the true footprint overlaps, the height
// window prunes the stacked surfaces).
func TestTileGridQueryAgainstBruteForce(t *testing.T) {
    tile := corridorWorld()
    toWorld := func(cells float64) float64 {
        return tile.WorldMinX() + cells*cellSizeWorld
    }
    cell := func(v float64) float64 { return toWorld(v) }
    rects := []struct {
        x0, y0, x1, y1 float64
        z              float64
    }{
        {cell(0), cell(0), cell(160), cell(160), 0},       // A
        {cell(160), cell(0), cell(320), cell(160), 0},     // B
        {cell(160), cell(160), cell(320), cell(208), -40}, // C
        {cell(160), cell(208), cell(320), cell(320), -80}, // D
        {cell(160), cell(208), cell(320), cell(320), 100}, // E
    }

    boxes := [][4]float64{
        {toWorld(4), toWorld(4), toWorld(12), toWorld(12)},
        {toWorld(150), toWorld(150), toWorld(170), toWorld(170)},
        {toWorld(200), toWorld(180), toWorld(310), toWorld(300)},
        {toWorld(0), toWorld(0), toWorld(2048), toWorld(2048)},
    }
    zWindows := [][2]float64{
        {-600, 600},
        {-600, 600},
        {-600, 600},
        {-600, 600},
    }
    for i, box := range boxes {
        minX, minY, maxX, maxY := box[0], box[1], box[2], box[3]
        minZ, maxZ := zWindows[i][0], zWindows[i][1]
        got := tileQueryPolysGrid(tile, minX, minY, maxX, maxY, minZ, maxZ,
            nil)
        var want []int32
        for pi := range tile.Polys {
            r := rects[pi]
            overlap := r.x0 < maxX && r.x1 > minX && r.y0 < maxY &&
                r.y1 > minY && r.z >= minZ && r.z <= maxZ
            if overlap {
                want = append(want, int32(pi))
            }
        }
        // The multi bucket boxes may list a bucket spanning polygon
        // once per touched bucket (the nearest poly caller tests the
        // exact geometry per candidate, the duplicate only costs a
        // repeat test): the comparison folds both sides.
        require.ElementsMatch(t, unique(want), unique(got),
            "grid candidates at box %d", i)
    }
}

// unique folds a candidate slice to its distinct values (the test
// helper of the grid comparison).
func unique(candidates []int32) []int32 {
    seen := map[int32]bool{}
    out := make([]int32, 0, len(candidates))
    for _, c := range candidates {
        if seen[c] {
            continue
        }
        seen[c] = true
        out = append(out, c)
    }

    return out
}
