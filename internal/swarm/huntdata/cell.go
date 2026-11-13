// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package huntdata

import (
    "github.com/melg8/swarm/internal/swarm/state"
)

// Voronoi cell hunting: the spawn ground partition of a region into
// convex cells (see docs/hunting_cells.md). Every cell is the set of
// ground points closer to its seed than to any other - the half-plane
// intersection - so every spawn point of the region belongs to
// EXACTLY ONE cell and no respawn area is ever half-covered (the
// depletion/oversaturation failure of the circle geometry: a circle
// that farms half a respawn ground starves its half while the other
// accumulates). The cell extent from the focus stays inside the
// guaranteed knownlist circle of the Mobius world grid (~2048
// units), so a bot standing anywhere in its patrol square sees the
// whole cell: the emptiness reading of the wait-or-switch economy is
// the truth, never a load boundary artifact. The target leash is the
// exact convex polygon; the movement machinery keeps the patrol
// square inscribed in it. The hunt rotates through the adjacency
// graph of the partition paced by the respawn ripeness - see the
// hunt package cell_policy.go.

// CellVertex is one vertex of the convex cell polygon (world
// coordinates, counter-clockwise; the generator normalizes the
// orientation).
type CellVertex struct {
    X int32
    Y int32
}

// CellMob describes one mob species of a hunting cell: the expected
// population share of the ground (the territory sample points the
// nearest-seed assignment gave the cell) and the respawn window of
// the species from the spawn data.
type CellMob struct {
    // TemplateID is the npc template id of the spawn data (the
    // Mobius CT0 xml id, see npcdata.NPCWireTemplateID).
    TemplateID int32
    // Name is the display name of the mob.
    Name string
    // Level is the mob level of the npc stats.
    Level int32
    // Count is the expected spawned population of the species
    // inside the cell ground.
    Count int32
    // RespawnMin and RespawnMax bound the respawn delay of the
    // species in seconds (the server schedules death + rnd of the
    // window).
    RespawnMin int32
    RespawnMax int32
}

// Cell is one hunting ground of the Voronoi partition registry.
type Cell struct {
    // ID is the stable identifier of the cell (region prefixed).
    ID string
    // Name is the display name of the map view.
    Name string
    // Region groups the cells of one ground (elven, orc, ...).
    Region string
    // MinLevel and MaxLevel are the mob level band of the cell
    // ground (the picker scores the window mass, not the band - the
    // band only feeds the map label and the diagnostics).
    MinLevel int32
    MaxLevel int32
    // FocusX and FocusY are the mass centroid of the cell ground:
    // the patrol destination and the ripeness clock anchor.
    FocusX int32
    FocusY int32
    // PatrolHalf is the movement leash half: the axis-aligned
    // square inscribed in the cell polygon at the focus, capped so
    // the whole cell stays visible from any point of the square.
    PatrolHalf int32
    // RespawnMin and RespawnMax are the dominant respawn window of
    // the ground in seconds (the count weighted midpoint of the
    // species windows).
    RespawnMin int32
    RespawnMax int32
    // Mass is the expected total population of the ground (the
    // count sum of the mob list).
    Mass float64
    // Mobs lists every species of the ground the cell covers.
    Mobs []CellMob
    // Vertices are the convex polygon corners (counter-clockwise):
    // the exact target leash of the cell.
    Vertices []CellVertex
    // Neighbors are the registry indices of the adjacent cells (the
    // bisector-bounded Voronoi neighbors, symmetric by construction):
    // the rotation graph of the hunt policy.
    Neighbors []int32
}

// TargetZone builds the target leash of the cell: the exact convex
// polygon. The engage, the far target search and the emptiness
// reading fence on the whole cell (not the patrol square), so the
// farm ground is the complete cell - the half-covered respawn
// failure of the circle geometry cannot reappear through the leash.
func (c Cell) TargetZone() *state.CellZone {
    vertices := make([]state.ZoneVertex, len(c.Vertices))
    for index := range c.Vertices {
        vertices[index] = state.ZoneVertex{
            X: c.Vertices[index].X,
            Y: c.Vertices[index].Y,
        }
    }

    return state.NewCellZone(vertices)
}
