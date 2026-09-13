// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

// ZoneVertex is one vertex of a convex cell polygon in world
// coordinates. The vertices are ordered counter-clockwise (the
// generator normalizes the orientation), so the containment test
// walks the left side of every edge.
type ZoneVertex struct {
    X int32
    Y int32
}

// CellZone is the convex polygon leash of the Voronoi cell hunting:
// the hunt ground is the whole cell, not a square approximation of
// it, so the target searches, the loot scans and the emptiness
// readings fence on the exact cell shape (the half-plane
// intersection the generator clipped). A point belongs to the cell
// when it stays on the inner side of every polygon edge - the
// cross product of the edge and the difference to the vertex stays
// non-negative for a CCW polygon. A nil CellZone or a degenerate
// vertex list means no limit, mirroring the nil *Zone semantics.
type CellZone struct {
    Vertices []ZoneVertex
}

// NewCellZone builds the convex polygon leash from the
// counter-clockwise vertex list (the hunt registry order; the
// generator guarantees the CCW orientation and the convexity, the
// constructor only wraps).
func NewCellZone(vertices []ZoneVertex) *CellZone {
    return &CellZone{Vertices: vertices}
}

// Contains reports whether the world point lies inside the convex
// polygon. The cross products run on int64: the vertex coordinates
// and the tested points span tens of thousands of world units, an
// int32 product of two such differences would overflow on the far
// map regions.
func (z *CellZone) Contains(x int32, y int32) bool {
    if z == nil {
        return true
    }
    n := len(z.Vertices)
    if n < 3 {
        return true
    }
    for i := range z.Vertices {
        a := z.Vertices[i]
        b := z.Vertices[(i+1)%n]
        cross := int64(b.X-a.X)*int64(y-a.Y) -
            int64(b.Y-a.Y)*int64(x-a.X)
        if cross < 0 {
            return false
        }
    }

    return true
}

// CellZoneFromPairs builds the convex polygon leash from a flat
// x,y coordinate sequence (the wire form of the mesh payload: the
// pairs arrive counter-clockwise). An odd or short sequence leaves
// no leash at all (no limit), the malformed input never fences the
// hunt.
func CellZoneFromPairs(pairs []int32) *CellZone {
    if len(pairs) < 6 || len(pairs)%2 != 0 {
        return nil
    }
    vertices := make([]ZoneVertex, 0, len(pairs)/2)
    for i := 0; i+1 < len(pairs); i += 2 {
        vertices = append(vertices, ZoneVertex{X: pairs[i], Y: pairs[i+1]})
    }

    return NewCellZone(vertices)
}
