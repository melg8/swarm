// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// Multi zone hunting: the zones of a region ladder up by the mob
// level bands, every band gates on the equipped gear (the zone gating
// points of the gear package) and the loop picks the highest band the
// character level and its gear allow, switching the hunting square
// when the character grows into the next band. The zone registry is
// data driven: the elven lands are configured here, the other regions
// (orcs, dwarves, dark elves, humans) join as their own lists, and a
// mage profile changes only the gear scoring, never the zone model.

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
}

// elvenHuntingZones ladders the elven lands: the keltir field of the
// village surroundings (levels 1-4, the default square of this
// deployment), the goblin and kaboo orc camps east of the village
// (levels 5-7), the kaboo fighter woods in the west (levels 8-12,
// entered once the gear pays for it) and the dryad and spider forest
// of the southwest (levels 13-18, the deep hunting grounds). Centers
// and halves mirror the Mobius C1 spawn data of ElvenStarting.xml.
var elvenHuntingZones = []HuntingZone{
	{
		ID: "elven-keltirs", Name: "Elven Village Keltir Field",
		Region: "elven", MinLevel: 1, MaxLevel: 4, MinGear: 0,
		CX: 46112, CY: 41500, Half: 1650,
	},
	{
		ID: "elven-goblins", Name: "East Forest Goblin Camp",
		Region: "elven", MinLevel: 5, MaxLevel: 7, MinGear: 40,
		CX: 51300, CY: 48700, Half: 2900,
	},
	{
		ID: "elven-kaboo", Name: "West Kaboo Woods",
		Region: "elven", MinLevel: 8, MaxLevel: 12, MinGear: 110,
		CX: 35500, CY: 48700, Half: 3200,
	},
	{
		ID: "elven-dryads", Name: "Southwest Dryad Forest",
		Region: "elven", MinLevel: 13, MaxLevel: 18, MinGear: 200,
		CX: 6000, CY: 53000, Half: 3500,
	},
}

// ElvenHuntingZones returns the hunting grounds of the elven lands.
func ElvenHuntingZones() []HuntingZone {
	zones := make([]HuntingZone, len(elvenHuntingZones))
	copy(zones, elvenHuntingZones)

	return zones
}

// zoneSwitchPeriod bounds the automatic zone re-evaluation: the level
// changes and the gear upgrades (the town trips and the auto
// equipment) move the character up the ladder within this period.
const zoneSwitchPeriod = 30 * time.Second

// zoneOverrideSlack is the level slack a manual zone selection keeps
// its override: the automatic picker resumes once the character
// outgrows the band.
const zoneOverrideSlack = 3

// PickHuntingZone returns the best zone of the list for the character
// level and the gear points: the highest level band both gates allow,
// so the gear gate holds the character back until its equipment pays
// for the stronger mobs. The starter zone (the lowest band) is the
// fallback for levels below every band.
func PickHuntingZone(
	zones []HuntingZone, level int32, gearPoints int32,
) (HuntingZone, bool) {
	best := -1
	for index := range zones {
		if level < zones[index].MinLevel ||
			gearPoints < zones[index].MinGear {
			continue
		}
		if best < 0 ||
			zones[index].MaxLevel > zones[best].MaxLevel ||
			(zones[index].MaxLevel == zones[best].MaxLevel &&
				zones[index].MinLevel > zones[best].MinLevel) {
			best = index
		}
	}
	if best < 0 {
		// Below every band: the first zone of the region (the starter
		// ground).
		if len(zones) > 0 {
			return zones[0], true
		}

		return HuntingZone{}, false
	}

	return zones[best], true
}

// SetHuntingZones installs the zone registry of the deployment: the
// loop picks the zone for the character state on the next tick and
// re-evaluates it as the level and the gear grow. Without a registry
// the loop hunts with the plain SetHuntingZone square (or without a
// zone at all).
func (l *Loop) SetHuntingZones(zones []HuntingZone) {
	l.zones = zones
	l.zoneOverride = -1
	l.zonePickedID = ""
	l.zoneCheckAt = time.Time{}
	l.publishZoneView()
}

// SetHuntingZoneRegion installs the registry of one region by name
// ("elven"): the map of the future deployments.
func (l *Loop) SetHuntingZoneRegion(region string) {
	switch region {
	case "elven", "":
		l.SetHuntingZones(ElvenHuntingZones())
	default:
		l.logger.Printf("Hunt: no zone registry for region %q, hunting "+
			"without zones", region)
	}
}

// userZoneSelect applies the manual zone selection of the web UI: the
// index refers to the zone registry order. The selection overrides
// the automatic picker until the character outgrows the band.
func (l *Loop) userZoneSelect(index int32) {
	if len(l.zones) == 0 {
		return
	}
	if index < 0 || int(index) >= len(l.zones) {
		l.logger.Printf("Hunt: zone index %d out of range", index)

		return
	}
	zone := l.zones[index]
	l.zoneOverride = int(index)
	l.logger.Printf("Hunt: user selected the hunting zone %s", zone.Name)
	l.applyHuntingZone(zone)
}

// maybeSwitchZone re-evaluates the automatic zone pick: a level gain
// or a gear upgrade moves the character up the ladder. The switch
// only happens between the fights (no target, nobody attacks), so a
// running fight always finishes in the old square.
func (l *Loop) maybeSwitchZone() {
	if len(l.zones) == 0 {
		return
	}
	now := time.Now()
	if l.zonePickedID != "" && now.Sub(l.zoneCheckAt) < zoneSwitchPeriod {
		return
	}
	l.zoneCheckAt = now
	level := l.tracker.SelfLevel()
	if l.zoneOverride >= 0 {
		zone := l.zones[l.zoneOverride]
		if level <= zone.MaxLevel+zoneOverrideSlack {
			return
		}
		l.zoneOverride = -1
		l.logger.Printf("Hunt: outgrew the manual zone %s, resuming the "+
			"automatic picker", zone.Name)
	}
	zone, ok := PickHuntingZone(l.zones, level, l.gearPoints())
	if !ok || zone.ID == l.zonePickedID {
		return
	}
	l.applyHuntingZone(zone)
	l.logger.Printf("Hunt: level %d with gear %d: hunting %s (levels "+
		"%d-%d)", level, l.gearPoints(), zone.Name, zone.MinLevel,
		zone.MaxLevel)
}

// applyHuntingZone switches the hunting square of the loop and the
// tracker and publishes the zone view of the map.
func (l *Loop) applyHuntingZone(zone HuntingZone) {
	l.zonePickedID = zone.ID
	l.zoneCX, l.zoneCY, l.zoneHalf = zone.CX, zone.CY, zone.Half
	l.tracker.SetHuntingZone(zone.CX, zone.CY, zone.Half)
	l.publishZoneView()
}

// publishZoneView pushes the zone registry with the active marker to
// the tracker snapshot: the map draws every zone and highlights the
// one the bot hunts in.
func (l *Loop) publishZoneView() {
	if l.tracker == nil {
		return
	}
	views := make([]state.ZoneView, 0, len(l.zones))
	for _, zone := range l.zones {
		views = append(views, state.ZoneView{
			ID:       zone.ID,
			Name:     zone.Name,
			Region:   zone.Region,
			MinLevel: zone.MinLevel,
			MaxLevel: zone.MaxLevel,
			MinGear:  zone.MinGear,
			CX:       zone.CX,
			CY:       zone.CY,
			Half:     zone.Half,
			Active:   zone.ID == l.zonePickedID,
		})
	}
	l.tracker.SetHuntingZones(views)
}
