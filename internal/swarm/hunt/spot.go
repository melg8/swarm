// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// Spot-anchored hunting: the spawn ground of a region as visibility
// bounded anchors with respawn awareness and efficiency scoring (the
// redesign of the square zone system, see
// docs/hunting_system_redesign.md). A Spot is one farm anchor: the
// density centroid of a spawn mass cluster with a radius that never
// grows past the guaranteed knownlist circle of a standing character
// (the Mobius world grid broadcasts the objects of the own region
// plus the 8 adjacent ones, ~2048 units), the mob composition of the
// ground it covers and the respawn window of every species. The hunt
// loop leashes itself to the inscribed square of the spot circle, so
// every mob the leash allows is inside the knownlist while the
// character holds the anchor - the knownlist equals the spot
// population by construction, the "cleared square is not cleared"
// failure of the old geometry cannot happen.

// SpotMob describes one mob species of a hunting spot: the expected
// population share of the ground (the spawn mass cluster the spot
// covers) and the respawn window of the species from the spawn data
// (the live measured elven window as the default).
type SpotMob struct {
	// TemplateID is the npc template id of the spawn data (the
	// Mobius CT0 xml id, see npcdata.NPCWireTemplateID).
	TemplateID int32
	// Name is the display name of the mob.
	Name string
	// Level is the mob level of the npc stats.
	Level int32
	// Count is the expected spawned population of the species
	// inside the spot ground.
	Count int32
	// RespawnMin and RespawnMax bound the respawn delay of the
	// species in seconds (the server schedules death + rnd of the
	// window).
	RespawnMin int32
	RespawnMax int32
}

// Spot is one hunting ground of the spot registry: an anchor on the
// spawn mass with a visibility bounded radius.
type Spot struct {
	// ID is the stable identifier of the spot (region prefixed).
	ID string
	// Name is the display name of the map view.
	Name string
	// Region groups the spots of one territory (elven, orc, ...).
	Region string
	// MinLevel and MaxLevel are the mob level band of the spot
	// ground (the picker scores the window mass, not the band -
	// the band only feeds the map label and the diagnostics).
	MinLevel int32
	MaxLevel int32
	// AnchorX and AnchorY are the density centroid of the spawn
	// mass cluster: the leash square centers here.
	AnchorX int32
	AnchorY int32
	// Radius is the spot circle: the spawn mass of the cluster
	// stays within it, clamped to the guaranteed visible circle
	// (2048).
	Radius int32
	// RespawnMin and RespawnMax are the dominant respawn window
	// of the ground in seconds (the count weighted midpoint of
	// the species windows).
	RespawnMin int32
	RespawnMax int32
	// Mass is the expected total population of the ground (the
	// count sum of the mob list).
	Mass float64
	// Mobs lists every species of the ground the spot covers.
	Mobs []SpotMob
}

// leashHalf returns the engage square half of the spot: the square
// INSCRIBED in the visibility circle (radius / sqrt(2)), so every
// point the leash covers stays inside the guaranteed knownlist circle
// of the anchor. The farm ground of the spot is this square; the
// spawn mass beyond it belongs to the neighboring spots and the
// wait-or-move economy handles the walk between the anchors.
func (s Spot) leashHalf() int32 {
	half := int32(math.Round(float64(s.Radius) / math.Sqrt2))
	if half < 1 {
		half = 1
	}

	return half
}

// zoneSquare returns the leash square of the spot in the tracker zone
// form (the engage leash, the far target search bounds and the
// emptiness reading all use it).
func (s Spot) zoneSquare() *state.Zone {
	return &state.Zone{CX: s.AnchorX, CY: s.AnchorY, Half: s.leashHalf()}
}

// spotDistance measures the anchor distance between a spot and a
// world position.
func spotDistance(spot Spot, x int32, y int32) float64 {
	return math.Hypot(float64(spot.AnchorX-x), float64(spot.AnchorY-y))
}

// spotWindowMass returns the expected population of the spot inside
// the white-green window [max(1, level-5), level]: the mobs a few
// levels below the character drop full loot without any exp or adena
// penalty and die in a few swings - the gold optimum the picker
// scores.
func spotWindowMass(spot Spot, level int32) float64 {
	low := level - 5
	if low < 1 {
		low = 1
	}
	mass := 0.0
	for index := range spot.Mobs {
		mob := &spot.Mobs[index]
		if mob.Level >= low && mob.Level <= level {
			mass += float64(mob.Count)
		}
	}

	return mass
}

// spotEligible reports whether the character can productively hunt
// the spot at the given level: the ground holds at least one species
// inside the wide window [max(1, level-8), level+2] (mob level + 8 is
// the full adena edge, level + 2 the hard engage ceiling). A spot
// outside the window wastes the character's time - too low pays no
// adena, too high kills it.
func spotEligible(spot Spot, level int32) bool {
	low := level - 8
	if low < 1 {
		low = 1
	}
	high := level + spotMaxLevelSlack
	for index := range spot.Mobs {
		mob := &spot.Mobs[index]
		if mob.Level >= low && mob.Level <= high {
			return true
		}
	}

	return false
}

// spotLevelDistance returns the smallest mob level distance between
// the spot ground and the character level: the fallback ranking of
// the picker when no spot passes the window (a character above or
// below every window still hunts the closest ground instead of
// standing idle).
func spotLevelDistance(spot Spot, level int32) int32 {
	best := int32(1 << 30)
	for index := range spot.Mobs {
		gap := spot.Mobs[index].Level - level
		if gap < 0 {
			gap = -gap
		}
		if gap < best {
			best = gap
		}
	}

	return best
}

// spotAggroMass returns the expected aggressive population of the
// spot ground: the static danger input of the safety score (an
// aggressive-heavy ground attacks on sight, the measured death rate
// dominates it once the experience accumulates).
func spotAggroMass(spot Spot) float64 {
	mass := 0.0
	for index := range spot.Mobs {
		mob := &spot.Mobs[index]
		if npcdata.NPCIsAggressive(mob.TemplateID) {
			mass += float64(mob.Count)
		}
	}

	return mass
}

// spotMobPriorities builds the window biased engage priority map of a
// spot: the white-green preference subrange [level-4, level-1] reads
// as the strongest bias (those mobs die in a few swings and pay full
// loot), the rest of the window [level-5, level] a weaker one, the
// wide window tail [level-8, level+2] the weakest. The priorities
// translate the template ids onto their wire keys (the NpcInfo
// packets identify the same npc by the display id plus offset), so
// the bias actually matches the scan template ids.
func spotMobPriorities(spot Spot, level int32) map[int32]int32 {
	var priorities map[int32]int32
	for index := range spot.Mobs {
		mob := &spot.Mobs[index]
		var priority int32
		switch {
		case mob.Level >= level-4 && mob.Level <= level-1:
			priority = 3
		case mob.Level >= level-5 && mob.Level <= level:
			priority = 2
		case mob.Level >= level-8 && mob.Level <= level+spotMaxLevelSlack:
			priority = 1
		}
		if priority <= 0 {
			continue
		}
		if priorities == nil {
			priorities = make(map[int32]int32, len(spot.Mobs))
		}
		priorities[npcdata.NPCWireTemplateID(mob.TemplateID)] = priority
	}

	return priorities
}

// spotMobRespawn resolves the respawn window of the spot species a
// kill record belongs to (the wire template id of the NpcInfo packet
// keyed back onto the spawn data id of the mob list).
func spotMobRespawn(spots []Spot, wireTemplateID int32) (int32, int32, bool) {
	for index := range spots {
		for mobIndex := range spots[index].Mobs {
			mob := &spots[index].Mobs[mobIndex]
			if npcdata.NPCWireTemplateID(mob.TemplateID) == wireTemplateID {
				return mob.RespawnMin, mob.RespawnMax, true
			}
		}
	}

	return 0, 0, false
}
