// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "slices"

    "github.com/melg8/swarm/internal/swarm/npcdata"
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
// graph of the partition paced by the respawn ripeness - see
// cell_policy.go.

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

// targetZone builds the target leash of the cell: the exact convex
// polygon. The engage, the far target search and the emptiness
// reading fence on the whole cell (not the patrol square), so the
// farm ground is the complete cell - the half-covered respawn
// failure of the circle geometry cannot reappear through the leash.
func (c Cell) targetZone() *state.CellZone {
    vertices := make([]state.ZoneVertex, len(c.Vertices))
    for index := range c.Vertices {
        vertices[index] = state.ZoneVertex{
            X: c.Vertices[index].X,
            Y: c.Vertices[index].Y,
        }
    }

    return state.NewCellZone(vertices)
}

// CellLeash returns the convex polygon leash of the cell (the exact
// Voronoi ground): the audit tool and the tests reuse the same
// containment the hunt scans run.
func CellLeash(cell Cell) *state.CellZone {
    return cell.targetZone()
}

// cellDistance measures the focus distance between a cell and a
// world position.
func cellDistance(cell Cell, x int32, y int32) float64 {
    return math.Hypot(float64(cell.FocusX-x), float64(cell.FocusY-y))
}

// cellWindowMass returns the expected population of the cell inside
// the white-green window [max(1, level-5), level]: the mobs a few
// levels below the character drop full loot without any exp or
// adena penalty and die in a few swings - the gold optimum the
// picker scores.
func cellWindowMass(cell Cell, level int32) float64 {
    low := level - 5
    if low < 1 {
        low = 1
    }
    mass := 0.0
    for index := range cell.Mobs {
        mob := &cell.Mobs[index]
        if mob.Level >= low && mob.Level <= level {
            mass += float64(mob.Count)
        }
    }

    return mass
}

// cellEligible reports whether the character can productively hunt
// the cell at the given level: the ground holds at least one species
// inside the wide window [max(1, level-8), level+2] (mob level + 8
// is the full adena edge, level + 2 the hard engage ceiling). A cell
// outside the window wastes the character's time - too low pays no
// adena, too high kills it.
func cellEligible(cell Cell, level int32) bool {
    low := level - 8
    if low < 1 {
        low = 1
    }
    high := level + cellMaxLevelSlack
    for index := range cell.Mobs {
        mob := &cell.Mobs[index]
        if mob.Level >= low && mob.Level <= high {
            return true
        }
    }

    return false
}

// cellMedianLevel returns the count weighted median mob level of the
// cell: the delevel trigger measures the character level against
// exactly this gap (MedianZoneMobLevel observes the live spawns of
// the polygon ground, the static median is what the picker can
// judge BEFORE committing the character to the ground).
func cellMedianLevel(cell Cell) int32 {
    type census struct {
        level int32
        mass  int32
    }
    censusRows := make([]census, 0, len(cell.Mobs))
    total := int32(0)
    for index := range cell.Mobs {
        if cell.Mobs[index].Count <= 0 {
            continue
        }
        censusRows = append(censusRows, census{
            level: cell.Mobs[index].Level,
            mass:  cell.Mobs[index].Count,
        })
        total += cell.Mobs[index].Count
    }
    if total <= 0 {
        return 0
    }
    slices.SortFunc(censusRows, func(a, b census) int {
        return int(a.level - b.level)
    })
    seen := int32(0)
    for _, row := range censusRows {
        seen += row.mass
        if seen*2 > total {
            return row.level
        }
    }

    return censusRows[len(censusRows)-1].level
}

// cellDelevelSafe reports whether holding the cell at the given
// level keeps the character under the delevel trigger: the cell
// window (level-8) admits grounds whose median sits 7+ levels below
// the character, and the deleveling answers such ground with the
// guard deaths - the picker must not commit the character to ground
// it immediately delevels it for. A cell without a computable median
// keeps the plain window judgement.
func cellDelevelSafe(cell Cell, level int32) bool {
    median := cellMedianLevel(cell)
    if median <= 0 {
        return true
    }

    return level-median < delevelTriggerDiff
}

// cellLevelDistance returns the smallest mob level distance between
// the cell ground and the character level: the fallback ranking of
// the picker when no cell passes the window (a character above or
// below every window still hunts the closest ground instead of
// standing idle).
func cellLevelDistance(cell Cell, level int32) int32 {
    best := int32(1 << 30)
    for index := range cell.Mobs {
        gap := cell.Mobs[index].Level - level
        if gap < 0 {
            gap = -gap
        }
        if gap < best {
            best = gap
        }
    }

    return best
}

// cellAggroMass returns the expected aggressive population of the
// cell ground: the static danger input of the safety score (an
// aggressive-heavy ground attacks on sight, the measured death rate
// dominates it once the experience accumulates).
func cellAggroMass(cell Cell) float64 {
    mass := 0.0
    for index := range cell.Mobs {
        mob := &cell.Mobs[index]
        if npcdata.NPCIsAggressive(mob.TemplateID) {
            mass += float64(mob.Count)
        }
    }

    return mass
}

// cellMobPriorities builds the window biased engage priority map of
// a cell: the white-green preference subrange [level-4, level-1]
// reads as the strongest bias (those mobs die in a few swings and
// pay full loot), the rest of the window [level-5, level] a weaker
// one, the wide window tail [level-8, level+2] the weakest. The
// priorities translate the template ids onto their wire keys (the
// NpcInfo packets identify the same npc by the display id plus
// offset), so the bias actually matches the scan template ids.
func cellMobPriorities(cell Cell, level int32) map[int32]int32 {
    var priorities map[int32]int32
    for index := range cell.Mobs {
        mob := &cell.Mobs[index]
        var priority int32
        switch {
        case mob.Level >= level-4 && mob.Level <= level-1:
            priority = 3
        case mob.Level >= level-5 && mob.Level <= level:
            priority = 2
        case mob.Level >= level-8 && mob.Level <= level+cellMaxLevelSlack:
            priority = 1
        }
        if priority <= 0 {
            continue
        }
        if priorities == nil {
            priorities = make(map[int32]int32, len(cell.Mobs))
        }
        priorities[npcdata.NPCWireTemplateID(mob.TemplateID)] = priority
    }

    return priorities
}

// cellMobRespawn resolves the respawn window of the cell species a
// kill record belongs to (the wire template id of the NpcInfo packet
// keyed back onto the spawn data id of the mob list). The first
// match wins: the species windows of one ground are near identical
// in practice and the lookup only feeds the respawn prediction.
func cellMobRespawn(cells []Cell, wireTemplateID int32) (int32, int32, bool) {
    for index := range cells {
        for mobIndex := range cells[index].Mobs {
            mob := &cells[index].Mobs[mobIndex]
            if npcdata.NPCWireTemplateID(mob.TemplateID) == wireTemplateID {
                return mob.RespawnMin, mob.RespawnMax, true
            }
        }
    }

    return 0, 0, false
}
