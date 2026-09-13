// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// The registry invariants pin the committed Voronoi partition against
// the generator drift: the convexity and the orientation of every
// cell polygon, the symmetric adjacency, the patrol square and the
// focus inside the polygon, the mass arithmetic and the ordering.
// A regeneration that breaks one of these is a generator bug, not a
// data change.

// cross2 computes the z cross product of the edge AB with AC.
func cross2(ax, ay, bx, by, cx, cy int32) int64 {
    return int64(bx-ax)*int64(cy-ay) - int64(by-ay)*int64(cx-ax)
}

func TestElvenCellRegistryInvariants(t *testing.T) {
    cells := ElvenHuntingCells()
    require.NotEmpty(t, cells)

    ids := make(map[string]bool, len(cells))
    for index := range cells {
        cell := &cells[index]

        t.Run(cell.ID, func(t *testing.T) {
            require.NotEmpty(t, cell.ID)
            require.False(t, ids[cell.ID], "the id repeats")
            ids[cell.ID] = true
            require.NotEmpty(t, cell.Name)
            require.Equal(t, regionElven, cell.Region)
            require.Positive(t, cell.PatrolHalf)
            require.Len(t, cell.Vertices, len(cell.Vertices))
            require.GreaterOrEqual(t, len(cell.Vertices), 3,
                "a cell polygon needs at least 3 corners")

            // The mass arithmetic: the count sum of the mob list.
            var mass int32
            for mobIndex := range cell.Mobs {
                mob := &cell.Mobs[mobIndex]
                require.NotEmpty(t, mob.Name)
                require.Positive(t, mob.Level)
                require.Positive(t, mob.Count)
                require.GreaterOrEqual(t, mob.RespawnMin, int32(1))
                require.GreaterOrEqual(t, mob.RespawnMax,
                    mob.RespawnMin)
                mass += mob.Count
            }
            require.InDelta(t, float64(mass), cell.Mass, 0.01)

            // The convex counter-clockwise polygon: every turn of the
            // vertex ring stays left (the cross product is
            // non-negative; a negative turn means a concave or a
            // clockwise polygon and the containment test of the
            // leash would fence the wrong half).
            n := len(cell.Vertices)
            for i := range cell.Vertices {
                a := cell.Vertices[i]
                b := cell.Vertices[(i+1)%n]
                c := cell.Vertices[(i+2)%n]
                require.GreaterOrEqual(t,
                    cross2(a.X, a.Y, b.X, b.Y, c.X, c.Y), int64(0),
                    "the polygon turns right at vertex %d", i)
            }

            // The leash semantics: the polygon containment answers
            // true inside, false outside (a sample in and out of the
            // bounding box of the polygon).
            zone := cell.targetZone()
            minX, minY := cell.Vertices[0].X, cell.Vertices[0].Y
            maxX, maxY := minX, minY
            for _, v := range cell.Vertices {
                minX = min(minX, v.X)
                minY = min(minY, v.Y)
                maxX = max(maxX, v.X)
                maxY = max(maxY, v.Y)
            }
            midX, midY := (minX+maxX)/2, (minY+maxY)/2
            require.True(t, zone.Contains(midX, midY),
                "the polygon center belongs to the cell")
            require.True(t, zone.Contains(cell.FocusX, cell.FocusY),
                "the focus belongs to its cell")
            require.False(t, zone.Contains(minX-100000, midY))
            require.False(t, zone.Contains(midX, minY-100000))

            // The patrol square stays inside the polygon: every
            // corner of the square passes the containment (the
            // movement leash never leaves the cell).
            half := cell.PatrolHalf
            for _, corner := range [][2]int32{
                {cell.FocusX - half, cell.FocusY - half},
                {cell.FocusX + half, cell.FocusY - half},
                {cell.FocusX + half, cell.FocusY + half},
                {cell.FocusX - half, cell.FocusY + half},
            } {
                require.True(t, zone.Contains(corner[0], corner[1]),
                    "the patrol corner (%d, %d) leaves the cell",
                    corner[0], corner[1])
            }

            // The neighbors: in range, unique, and symmetric (the
            // rotation graph is undirected by construction).
            seen := make(map[int32]bool, len(cell.Neighbors))
            for _, neighbor := range cell.Neighbors {
                require.GreaterOrEqual(t, neighbor, int32(0))
                require.Less(t, neighbor, int32(len(cells)))
                require.NotEqual(t, int32(index), neighbor,
                    "a cell never neighbors itself")
                require.False(t, seen[neighbor], "the neighbor repeats")
                seen[neighbor] = true
                back := false
                for _, reverse := range cells[neighbor].Neighbors {
                    if reverse == int32(index) {
                        back = true

                        break
                    }
                }
                require.True(t, back,
                    "the neighbor %d does not list %d back",
                    neighbor, index)
            }
        })
    }
}

func TestElvenCellRegistryOrderAndMass(t *testing.T) {
    cells := ElvenHuntingCells()
    // The total spawn mass of the elven ground the partition covers
    // (the Mobius ElvenStarting.xml npc count sum; the generator
    // preserves it exactly through the largest-remainder share).
    var total int32
    for index := range cells {
        for mobIndex := range cells[index].Mobs {
            total += cells[index].Mobs[mobIndex].Count
        }
    }
    require.Equal(t, int32(812), total,
        "the partition must carry the whole elven spawn mass")

    // The registry order: the lowest band first, the village nearest
    // ground of the band leads (the starter fallback of the picker).
    const villageX, villageY = 46112, 41500
    // The band sort key of the registry: (low, high, village
    // distance) - the starter fallback of the picker is the head of
    // the lowest band, the village nearest ground of that exact
    // (low, high) pair.
    first := cells[0]
    village := math.Hypot(
        float64(first.FocusX-villageX), float64(first.FocusY-villageY))
    for _, cell := range cells {
        if cell.MinLevel != first.MinLevel ||
            cell.MaxLevel != first.MaxLevel {
            break
        }
        dist := math.Hypot(
            float64(cell.FocusX-villageX), float64(cell.FocusY-villageY))
        require.GreaterOrEqual(t, dist, village,
            "%s sits nearer the village than the registry head",
            cell.ID)
    }
}
