// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package huntdata

// RegionElven is the region key of the elven lands zone registry.
const RegionElven = "elven"

// RegionDion is the region key of the Dion 20-25 band (the survey grounds of
// docs/band_20_25_survey.md and the zones_dion.go registry).
const RegionDion = "dion"

// regionElven and regionDion are the unqualified keys the generated
// registries emit.
const (
    regionElven = RegionElven
    regionDion  = RegionDion
)

// ZoneMob describes one mob species of a hunting zone: the whole mob
// list of the spawn territory the square covers. Every species is
// farmed; Priority adds the soft engage preference of targetPriorityBonus.
type ZoneMob struct {
    // TemplateID is the npc template id of the spawn data.
    TemplateID int32
    // Name is the display name of the mob.
    Name string
    // Level is the mob level of the npc stats.
    Level int32
    // Count is the spawned count of the parent territory.
    Count int32
    // Priority biases the engage toward this species: every point
    // makes the mob read as closer than it stands (soft tie break,
    // distance still dominates far picks).
    Priority int32
}

// HuntingZone describes one hunting ground: the level band of its
// mobs, the gear gate in zone gating points and the square the bot
// leashes itself to (the engage zone of the hunt loop).
type HuntingZone struct {
    // ID is the stable identifier of the zone (region prefixed).
    ID string
    // Name is the display name of the map view.
    Name string
    // Region groups the zones of one territory (elven, orc, ...).
    Region string
    // MinLevel and MaxLevel are the mob level band of the zone.
    MinLevel int32
    MaxLevel int32
    // MinGear is the zone gating points the equipped gear must reach
    // before the bot hunts here (see gear.TotalGearPoints).
    MinGear int32
    // CX, CY and Half describe the hunting square.
    CX   int32
    CY   int32
    Half int32
    // Mobs lists every mob species of the spawn territory the square
    // covers (all of them are farmed inside the square, the
    // priorities bias the engage; empty for hand placed zones).
    Mobs []ZoneMob
}

// ContainsPoint reports whether the world point lies inside the
// hunting square of the zone.
func (z HuntingZone) ContainsPoint(x int32, y int32) bool {
    return z.Half > 0 && x >= z.CX-z.Half && x <= z.CX+z.Half &&
        y >= z.CY-z.Half && y <= z.CY+z.Half
}
