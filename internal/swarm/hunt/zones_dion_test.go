// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Dion registry tests pin the generated 20-25 band zones against
// the survey tables of docs/band_20_25_survey.md - the acceptance
// reference of T-009: the mob species and levels of the grounds, the
// five band ladder with the gear gates of the D-grade dress stages,
// and the walking distances of the survey teleport anchors. Like the
// elven registry tests, the assertions look the zones up by band,
// mobs and position instead of hardcoding the generated ids, so a
// regeneration of zones_dion.go (a Mobius spawn data refresh) keeps
// the suite meaningful.

// dionSurveyAnchors are the teleport arrival points the survey
// measures the walking distances against: the Trisha "Execution
// Ground" arrival (the entry ground of the band), the Trisha cruma
// center, the Trisha "Plains of Dion" arrival and the Bella "Town of
// Dion" square.
var dionSurveyAnchors = [][2]int32{
	{46165, 150008},
	{5941, 125455},
	{630, 179184},
	{15671, 142994},
}

// dionNearestAnchorDistance returns the distance of the point to the
// nearest survey anchor.
func dionNearestAnchorDistance(x int32, y int32) float64 {
	best := math.MaxFloat64
	for _, anchor := range dionSurveyAnchors {
		dist := math.Hypot(
			float64(x-anchor[0]), float64(y-anchor[1]))
		if dist < best {
			best = dist
		}
	}

	return best
}

// dionSurveyMobLevels pins the level of every mob species the survey
// tables name (the grounds of the three Dion spawn sources).
var dionSurveyMobLevels = map[int32]int32{
	20154: 21, // Mandragora Sprout, the passive entry mob
	20223: 20, // Mandragora Sprout variant
	20155: 23, // Mandragora Sapling
	20156: 25, // Mandragora Blossom
	20171: 26, // Specter, the pull-over-25 neighbor
	20225: 25, // Giant Mist Leech
	20226: 26, // Gray Ant
	20227: 27, // Horror Mist Ripper
	20205: 24, // Dire Wolf
	20266: 25, // Monster Eye Gazer
	20206: 25, // Kadif Werewolf
	20068: 26, // Monster Eye Destroyer
	20250: 27, // Glass Jaguar
}

// zoneCarriesMob reports whether the zone farms the mob species.
func zoneCarriesMob(zone HuntingZone, templateID int32) bool {
	for mobIndex := range zone.Mobs {
		if zone.Mobs[mobIndex].TemplateID == templateID {
			return true
		}
	}

	return false
}

// crumaSpecies reports whether the mob id belongs to the cruma edge
// table (the leech, the gray ant, the ripper).
func crumaSpecies(templateID int32) bool {
	return templateID == 20225 || templateID == 20226 ||
		templateID == 20227
}

// mobLevelsOfZones collects the level of every mob species the zone
// list carries.
func mobLevelsOfZones(zones []HuntingZone) map[int32]int32 {
	levels := make(map[int32]int32)
	for index := range zones {
		for mobIndex := range zones[index].Mobs {
			mob := zones[index].Mobs[mobIndex]
			levels[mob.TemplateID] = mob.Level
		}
	}

	return levels
}

func TestGeneratedDionZoneRegistry(t *testing.T) {
	zones := DionHuntingZones()
	require.Greater(t, len(zones), 60)

	// The five band windows of the survey ladder with the gear gates
	// of the buyable dress stages (the generator calibration).
	bands := map[[3]int32]bool{
		{20, 21, 280}: true,
		{23, 24, 290}: true,
		{24, 25, 310}: true,
		{25, 26, 340}: true,
		{26, 27, 380}: true,
	}
	seen := make(map[string]bool, len(zones))
	for index := range zones {
		zone := zones[index]
		require.False(t, seen[zone.ID], "duplicate zone id %s", zone.ID)
		seen[zone.ID] = true
		require.Equal(t, regionDion, zone.Region, "zone %s", zone.ID)
		require.True(t,
			bands[[3]int32{zone.MinLevel, zone.MaxLevel, zone.MinGear}],
			"zone %s sits in a band outside the survey ladder", zone.ID)
		require.NotEmpty(t, zone.Name)
		require.NotEmpty(t, zone.Mobs)
		// The survey grounds carry the mobs of the 20-27 level window
		// (the band mobs plus the over-25 neighbors the tables name).
		require.GreaterOrEqual(t, zone.MinLevel, int32(20))
		for mobIndex := range zone.Mobs {
			mob := zone.Mobs[mobIndex]
			require.NotEmpty(t, mob.Name)
			require.Positive(t, mob.TemplateID)
			require.GreaterOrEqual(t, mob.Level, int32(20))
			require.LessOrEqual(t, mob.Level, int32(27))
			require.GreaterOrEqual(t, mob.Priority, int32(0))
		}
		// Every square stays compact (the rotation walks between
		// grounds, the engage leashes inside the square).
		require.Greater(t, zone.Half, int32(500))
		require.LessOrEqual(t, zone.Half, int32(1900))
	}
	// The registry ladders the bands in order and the first entry is
	// the starter fallback: the survey's entry ground, the passive
	// sprout square nearest the execution ground arrival.
	requireZoneOfBand(t, zones[0], 20, 21, 280)
	for index := 1; index < len(zones); index++ {
		require.GreaterOrEqual(t,
			zoneBandRank(zones[index], zones[index-1]), 0,
			"the registry must ladder the bands in order")
	}
}

func TestGeneratedDionSurveyMobTables(t *testing.T) {
	zones := DionHuntingZones()

	// The registry carries exactly the species the survey tables name,
	// every one at its table level.
	levels := mobLevelsOfZones(zones)
	require.Len(t, levels, len(dionSurveyMobLevels),
		"the registry must carry exactly the survey species")
	for templateID, level := range dionSurveyMobLevels {
		require.Equal(t, level, levels[templateID],
			"mob %d level", templateID)
	}

	// The entry ground table: the passive 20-21 sprout grounds carry
	// only the two sprout species (the safe approach of the fresh
	// level 20 character).
	for index := range zones {
		zone := zones[index]
		if zone.MaxLevel != 21 {
			continue
		}
		require.Equal(t, int32(20), zone.MinLevel)
		for mobIndex := range zone.Mobs {
			templateID := zone.Mobs[mobIndex].TemplateID
			require.True(t, templateID == 20154 || templateID == 20223,
				"zone %s carries a non-sprout mob %d",
				zone.ID, templateID)
		}
	}

	// The cruma edge table: the leech/ant/ripper grounds mix exactly
	// the three marsh species, and every square that carries the
	// aggressive ripper also carries the passive leech band mass.
	for index := range zones {
		zone := zones[index]
		if !zoneCarriesMob(zone, 20225) && !zoneCarriesMob(zone, 20226) &&
			!zoneCarriesMob(zone, 20227) {
			continue
		}
		for mobIndex := range zone.Mobs {
			templateID := zone.Mobs[mobIndex].TemplateID
			require.True(t, crumaSpecies(templateID),
				"zone %s carries a non-cruma mob %d",
				zone.ID, templateID)
		}
		if zoneCarriesMob(zone, 20227) {
			require.True(t, zoneCarriesMob(zone, 20225),
				"zone %s carries the ripper without the leech", zone.ID)
		}
	}
}

func TestGeneratedDionSurveyWalkingDistances(t *testing.T) {
	zones := DionHuntingZones()
	for index := range zones {
		zone := zones[index]
		// The survey reach: every ground of the band sits within the
		// walking distance of a teleport arrival (the furthest cells
		// are the NE plains squares, 18.1k from the execution
		// arrival).
		require.Less(t, dionNearestAnchorDistance(zone.CX, zone.CY),
			18500.0, "zone %s", zone.ID)
		if zone.MaxLevel == 21 {
			// The sprout grounds sit 6.6-7.9k from the execution
			// arrival (the partition cells keep that reach).
			dist := math.Hypot(
				float64(zone.CX-46165), float64(zone.CY-150008))
			require.Less(t, dist, 10000.0, "zone %s", zone.ID)
		}
		if zoneCarriesMob(zone, 20225) || zoneCarriesMob(zone, 20226) {
			// The cruma edge grounds sit 7.7-8.8k from the cruma
			// center arrival.
			dist := math.Hypot(
				float64(zone.CX-5941), float64(zone.CY-125455))
			require.Less(t, dist, 12000.0, "zone %s", zone.ID)
		}
		if zone.MaxLevel == 24 {
			// The northern wolf grounds are the walkable reach of the
			// Dion town square (13-17k out, no further teleport).
			dist := math.Hypot(
				float64(zone.CX-15671), float64(zone.CY-142994))
			require.Less(t, dist, 18000.0, "zone %s", zone.ID)
		}
	}
}

func TestPickHuntingZoneDionLadder(t *testing.T) {
	zones := DionHuntingZones()

	// A fresh level 20 character sits below every band of the registry
	// (the ladder gates need the level lead): the starter fallback
	// contest serves the survey's entry ground, the passive sprouts
	// nearest the Trisha execution arrival.
	zone, ok := PickHuntingZone(zones, 20, 0, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 20, 21, 280)

	// Level 22 in the elven NG dress (284 points) opens the sprout
	// band through the ladder gate; under the gear gate the character
	// stays on the sprouts through the starter fallback.
	zone, ok = PickHuntingZone(zones, 22, 284, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 20, 21, 280)
	zone, ok = PickHuntingZone(zones, 22, 100, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 20, 21, 280)

	// Level 25 in the Falchion dress (292) opens the aggressive dire
	// wolf band of the northern plains.
	zone, ok = PickHuntingZone(zones, 25, 292, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 23, 24, 290)

	// Level 26 with the Bastard Sword dress (312) opens the
	// sapling+blossom, gazer and werewolf grounds.
	zone, ok = PickHuntingZone(zones, 26, 312, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 24, 25, 310)

	// Level 27 in the partial mithril dress (341) opens the specter,
	// gray ant and monster eye destroyer grounds.
	zone, ok = PickHuntingZone(zones, 27, 341, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 25, 26, 340)

	// Level 28 in the full D dress (391) opens the cruma and jaguar
	// grounds - the top of the survey ladder.
	zone, ok = PickHuntingZone(zones, 28, 391, "", 46165, 150008, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 26, 27, 380)

	// A death regression capping the ladder below the wolf band keeps
	// a level 25 character on the sprout grounds.
	zone, ok = PickHuntingZone(zones, 25, 292, "", 46165, 150008, 22)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 20, 21, 280)
}

func TestSetHuntingZoneRegionInstallsDionRegistry(t *testing.T) {
	// A deployment that selects the Dion region gets the generated
	// 20-25 band registry installed (the default elven flow is
	// unchanged) and the region recorded for the gear catalog
	// selection of the shopping trips (shopCatalogForRegion).
	loop := NewLoop(&fakeGame{}, spotTestBot(t))
	loop.SetHuntingZoneRegion(regionDion)

	zones := DionHuntingZones()
	require.Len(t, loop.zones, len(zones))
	require.Equal(t, zones[0].ID, loop.zones[0].ID)
	require.Equal(t, regionDion, loop.zoneRegion)
	require.Nil(t, loop.spot, "the zone registry stands the spot mode down")
}
