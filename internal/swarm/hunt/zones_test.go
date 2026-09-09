// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The elven registry tests of the generated zone list: the assertions
// look the zones up by band and position instead of hardcoding the
// generated ids, so a regeneration of zones_elven.go (a Mobius spawn
// data refresh) keeps the suite meaningful.

// zoneIndexAt returns the registry index of the zone whose square
// contains the point (the first match wins, like deathZone).
func zoneIndexAt(zones []HuntingZone, x, y int32) int {
	for index := range zones {
		if zones[index].containsPoint(x, y) {
			return index
		}
	}

	return -1
}

// nearestZoneIndexOfBand finds the nearest zone of one band to the
// point, skipping the excluded ids - the plain contest of the picker
// and the rotation target choice without the cooldown bookkeeping.
func nearestZoneIndexOfBand(
	zones []HuntingZone, minLevel, maxLevel int32, x, y int32,
	exclude map[string]bool,
) int {
	best := -1
	bestDist := math.MaxFloat64
	for index := range zones {
		zone := zones[index]
		if zone.MinLevel != minLevel || zone.MaxLevel != maxLevel {
			continue
		}
		if exclude[zone.ID] {
			continue
		}
		if dist := zoneDistance(zone, x, y); dist < bestDist {
			best = index
			bestDist = dist
		}
	}

	return best
}

// requireZoneOfBand asserts that the picked zone belongs to the band.
func requireZoneOfBand(
	t *testing.T, zone HuntingZone, minLevel, maxLevel, minGear int32,
) {
	t.Helper()
	require.Equal(t, minLevel, zone.MinLevel)
	require.Equal(t, maxLevel, zone.MaxLevel)
	require.Equal(t, minGear, zone.MinGear)
}

func TestGeneratedElvenZoneRegistry(t *testing.T) {
	zones := ElvenHuntingZones()
	// The generated registry is an order of magnitude denser than the
	// hand placed thirty squares it replaced, every square sits on a
	// real spawn polygon and carries the mob list of its territory.
	require.Greater(t, len(zones), 150)

	seen := make(map[string]bool, len(zones))
	for index := range zones {
		zone := zones[index]
		require.False(t, seen[zone.ID], "duplicate zone id %s", zone.ID)
		seen[zone.ID] = true
		require.NotEmpty(t, zone.Name)
		require.NotEmpty(t, zone.Region)
		require.Positive(t, zone.MinLevel)
		require.GreaterOrEqual(t, zone.MaxLevel, zone.MinLevel)
		// The squares stay compact (the rotation works on walkable
		// distances) and never degenerate.
		require.Greater(t, zone.Half, int32(500))
		require.LessOrEqual(t, zone.Half, int32(1900))
		// Every generated zone farms the full mob list of its spawn
		// territory.
		require.NotEmpty(t, zone.Mobs)
		for mobIndex := range zone.Mobs {
			mob := zone.Mobs[mobIndex]
			require.NotEmpty(t, mob.Name)
			require.Positive(t, mob.TemplateID)
			require.GreaterOrEqual(t, mob.Priority, int32(0))
		}
	}
	// The registry ladders the bands in order and the first entry is
	// the starter fallback (a band 1-3 square near the village).
	requireZoneOfBand(t, zones[0], 1, 3, 0)
	for index := 1; index < len(zones); index++ {
		require.GreaterOrEqual(t,
			zoneBandRank(zones[index], zones[index-1]), 0,
			"the registry must ladder the bands in order")
	}
}

func TestPickHuntingZoneGatesOnLevelAndGear(t *testing.T) {
	zones := ElvenHuntingZones()

	// A fresh level 1 character without gear hunts the starter keltir
	// band (the nearest ground to the village).
	zone, ok := PickHuntingZone(zones, 1, 0, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, zones[0].ID, zone.ID)

	// Level 5 with the starter kit (44 points) hunts the wolf band -
	// the mobs sit 1-2 levels below the character; level 5 with bare
	// fists stays in the starter keltir band (the wolf gear gate).
	zone, ok = PickHuntingZone(zones, 5, 44, "", 46112, 41500, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 3, 4, 20)
	zone, ok = PickHuntingZone(zones, 5, 10, "", 46112, 41500, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 1, 3, 0)

	// Level 10 with the wooden set (130 points) hunts the grunt band;
	// without it (60 points) the gear gate holds the character in the
	// raider band.
	zone, ok = PickHuntingZone(zones, 10, 130, "", 46112, 41500, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 7, 8, 100)
	zone, ok = PickHuntingZone(zones, 10, 60, "", 46112, 41500, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 4, 6, 50)

	// Level 17 with the deep set (280 points) hunts the elder woods;
	// the full drop dress (320 points) opens the spider band on top.
	zone, ok = PickHuntingZone(zones, 17, 280, "", 46112, 41500, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 12, 14, 260)
	zone, ok = PickHuntingZone(zones, 17, 320, "", 46112, 41500, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 13, 16, 300)
}

func TestPickHuntingZoneNearestOfTheBand(t *testing.T) {
	zones := ElvenHuntingZones()
	// No current zone: the nearest ground of the winning band wins.
	zone, ok := PickHuntingZone(zones, 1, 0, "", 49000, 42100, -1)
	require.True(t, ok)
	expected := nearestZoneIndexOfBand(zones, 1, 3, 49000, 42100, nil)
	require.GreaterOrEqual(t, expected, 0)
	require.Equal(t, zones[expected].ID, zone.ID)
	// The same from the northwest.
	zone, ok = PickHuntingZone(zones, 1, 0, "", 41500, 39500, -1)
	require.True(t, ok)
	expected = nearestZoneIndexOfBand(zones, 1, 3, 41500, 39500, nil)
	require.GreaterOrEqual(t, expected, 0)
	require.Equal(t, zones[expected].ID, zone.ID)
}

func TestPickHuntingZoneKeepsCurrentZoneOfTheBand(t *testing.T) {
	zones := ElvenHuntingZones()
	// The current zone of the winning band keeps its post even when a
	// sibling square is nearer: the periodic re-pick never bounces the
	// character between the grounds of one band. The level 5 character
	// with the wolf gear holds the 3-4 band, the current ground is the
	// western wolf downs (not the village nearest northern one).
	current := zones[nearestZoneIndexOfBand(zones, 3, 4, 38272, 39103, nil)]
	requireZoneOfBand(t, current, 3, 4, 20)
	nearer := nearestZoneIndexOfBand(zones, 3, 4, 46112, 41500, nil)
	require.NotEqual(t, current.ID, zones[nearer].ID)
	zone, ok := PickHuntingZone(zones, 5, 44, current.ID, 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, current.ID, zone.ID)
}

func TestPickHuntingZoneDeathCap(t *testing.T) {
	zones := ElvenHuntingZones()
	// The death regression capped the ladder below the goblin band
	// (MinLevel 5): a level 8 character with the gear for the goblins
	// regresses to the raider band instead.
	zone, ok := PickHuntingZone(zones, 8, 120, "", 51707, 50504, 4)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 4, 6, 50)
	// A negative cap (the default) disables the regression: the level 8
	// character hunts the goblin band (mobs 1-2 levels below it).
	zone, ok = PickHuntingZone(zones, 8, 120, "", 51707, 50504, -1)
	require.True(t, ok)
	requireZoneOfBand(t, zone, 5, 7, 70)
}

func TestPickHuntingZoneFallbacks(t *testing.T) {
	zones := ElvenHuntingZones()
	// A level below every band falls back to the starter zone.
	zone, ok := PickHuntingZone(zones, 0, 0, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, zones[0].ID, zone.ID)
	// A cap that closes the whole ladder falls back to the starter
	// zone as well (nothing above the keltir band to hunt).
	zone, ok = PickHuntingZone(zones, 10, 300, "", 46112, 41500, 0)
	require.True(t, ok)
	require.Equal(t, zones[0].ID, zone.ID)
	// An empty registry reports no zone.
	_, ok = PickHuntingZone(nil, 10, 100, "", 0, 0, -1)
	require.False(t, ok)
}

func TestLoopSwitchesZoneOnLevel(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())

	// The first tick picks the nearest keltir square for the level 1
	// character.
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, ElvenHuntingZones()[0].ID, loop.zonePickedID)

	// The snapshot view carries the registry with the starter square
	// active.
	views := bot.Snapshot().HuntingZones
	require.Greater(t, len(views), 150)
	activeCount := 0
	for index := range views {
		if views[index].Active {
			activeCount++
		}
	}
	require.Equal(t, 1, activeCount)

	// The character grows to level 5 with the starter kit: the next
	// evaluation switches to the wolf band (mobs 1-2 levels below).
	setZoneTestLevel(bot, 5)
	equipZoneWithGear(bot, 44)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	picked, ok := loop.zoneByID(loop.zonePickedID)
	require.True(t, ok)
	requireZoneOfBand(t, picked, 3, 4, 20)
}

func TestLoopKeepsZoneDuringCooldown(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	first := loop.zonePickedID

	// The level grows but the evaluation cooldown holds the zone.
	setZoneTestLevel(bot, 6)
	equipZoneWithGear(bot, 44)
	loop.tick()
	require.Equal(t, first, loop.zonePickedID)
}

func TestUserZoneSelectOverridesPicker(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()

	// The user selects a far fighter ground: the override applies at
	// once.
	fighter := nearestZoneIndexOfBand(zones, 8, 10, 30000, 52000, nil)
	require.GreaterOrEqual(t, fighter, 0)
	loop.userZoneSelect(int32(fighter))
	require.Equal(t, zones[fighter].ID, loop.zonePickedID)
	require.Equal(t, fighter, loop.zoneOverride)

	// The automatic picker respects the override while the level
	// stays inside the band slack.
	setZoneTestLevel(bot, 10)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, zones[fighter].ID, loop.zonePickedID)

	// Outgrowing the band (level beyond max + slack) resumes the
	// automatic picker. The full shop dress (212 points) opens the
	// lieutenant band, the elder band waits for its 260 gear gate.
	setZoneTestLevel(bot, 16)
	equipZoneWithGear(bot, 212)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, -1, loop.zoneOverride)
	picked, ok := loop.zoneByID(loop.zonePickedID)
	require.True(t, ok)
	requireZoneOfBand(t, picked, 9, 12, 180)
}

func TestUserZoneSelectBounds(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	first := loop.zonePickedID

	loop.userZoneSelect(-1)
	require.Equal(t, first, loop.zonePickedID)
	loop.userZoneSelect(int32(len(zones)))
	require.Equal(t, first, loop.zonePickedID)
}

func TestZoneDeathsDemoteTheBand(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 8)
	equipZoneWithGear(bot, 212)
	loop.tick()
	// The level 8 character hunts the goblin band (5-7).
	picked, ok := loop.zoneByID(loop.zonePickedID)
	require.True(t, ok)
	requireZoneOfBand(t, picked, 5, 7, 70)

	// The hunt walks into the goblin ground: deaths count against
	// the ground they happen in (the position attribution). The
	// goblin camps of 2119_18 sit around (51604, 49429).
	goblin := zoneIndexAt(zones, 51707, 50504)
	require.GreaterOrEqual(t, goblin, 0)
	requireZoneOfBand(t, zones[goblin], 5, 7, 70)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	zoneDeathWaitRevival(bot)
	// Two deaths count, the third demotes the band.
	for range 2 {
		zoneDeathKill(bot, loop)
		require.Equal(t, zones[goblin].ID, loop.zonePickedID)
	}
	zoneDeathKill(bot, loop)
	require.Equal(t, int32(4), loop.zoneDeathCap,
		"the ladder caps below the goblin band (MinLevel 5)")
	require.True(t, loop.zoneCheckAt.IsZero(),
		"the demotion forces the ladder re-pick")

	// The revived character re-picks on the next tick: the goblin
	// band is capped out, the picker regresses to the raider band.
	zoneDeathWaitRevival(bot)
	loop.tick()
	picked, ok = loop.zoneByID(loop.zonePickedID)
	require.True(t, ok)
	requireZoneOfBand(t, picked, 4, 6, 50)
	require.Equal(t, int32(3), loop.zoneDeaths[zones[goblin].ID])

	// The view carries the deaths and the demoted marker of the
	// goblin band zones.
	views := bot.Snapshot().HuntingZones
	var goblinView *state.ZoneView
	for index := range views {
		if views[index].ID == zones[goblin].ID {
			goblinView = &views[index]
		}
	}
	require.NotNil(t, goblinView)
	require.Equal(t, int32(3), goblinView.Deaths)
	require.True(t, goblinView.Demoted)
}

func TestZoneDeathsResetOnLevelChange(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 8)
	equipZoneWithGear(bot, 212)
	loop.tick()
	goblin := zoneIndexAt(zones, 51707, 50504)
	require.GreaterOrEqual(t, goblin, 0)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	zoneDeathWaitRevival(bot)
	require.Equal(t, int32(-1), loop.zoneDeathCap)

	// The character grows a level: the bookkeeping resets and the
	// picker may retry the demotable band later.
	setZoneTestLevel(bot, 6)
	loop.tick()
	require.Equal(t, int32(-1), loop.zoneDeathCap)
	require.Empty(t, loop.zoneDeaths)
}

// TestZoneDeathOnFreshSessionCountsByPosition pins the emergency
// logout scenario: the character died while the session was offline
// (the server keeps it in the world through the combat window), so
// the reconnect lands on a fresh loop before any zone pick - the
// death spot is the only truthful attribution and the position
// lookup counts it there.
func TestZoneDeathOnFreshSessionCountsByPosition(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	// The character reconnects dead at the goblin camp, no zone picked.
	setZoneTestLevel(bot, 5)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	goblin := zoneIndexAt(zones, 51707, 50504)
	require.GreaterOrEqual(t, goblin, 0)
	loop.tick()
	require.Empty(t, loop.zonePickedID,
		"the death tick runs before the first zone pick")
	require.Equal(t, int32(1), loop.zoneDeaths[zones[goblin].ID],
		"the death counts against the square it happened in")
}

// TestZoneDemotionBreaksTheManualOverride pins the safety semantics of
// the regression: a manually selected ground the character keeps
// dying in releases its override with the demotion, so the automatic
// picker walks the character onto an easier band instead of marching
// the corpse back into the same blows.
func TestZoneDemotionBreaksTheManualOverride(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 8)
	equipZoneWithGear(bot, 212)
	loop.tick()

	// The operator forces a far fighter ground of the 8-10 band (way
	// above what the level 8 character pulls - the band opens at 11).
	fighter := zoneIndexAt(zones, 34769, 51063)
	require.GreaterOrEqual(t, fighter, 0)
	requireZoneOfBand(t, zones[fighter], 8, 10, 140)
	loop.userZoneSelect(int32(fighter))
	require.Equal(t, zones[fighter].ID, loop.zonePickedID)
	require.Equal(t, fighter, loop.zoneOverride)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 34769, Y: 51063, Z: -3400,
	})
	zoneDeathWaitRevival(bot)
	for range 3 {
		zoneDeathKill(bot, loop)
	}
	require.Equal(t, -1, loop.zoneOverride,
		"the demotion releases the manual zone selection")

	// The next living tick re-picks with the cap: the fighter band is
	// below the cap line, the goblin band is the highest one left.
	zoneDeathWaitRevival(bot)
	loop.tick()
	picked, ok := loop.zoneByID(loop.zonePickedID)
	require.True(t, ok)
	requireZoneOfBand(t, picked, 5, 7, 70)
	require.True(t, bot.Snapshot().HuntingZones[fighter].Demoted)
}

func TestZoneDeathOfDelevelingNeverCounts(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 8)
	equipZoneWithGear(bot, 212)
	loop.tick()
	goblin := zoneIndexAt(zones, 51707, 50504)
	require.GreaterOrEqual(t, goblin, 0)

	// A death of the delevel phase is the point of the phase, not a
	// regression signal (the character stands inside the goblin
	// square: only the phase check keeps it from counting).
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	loop.phase = phaseDelevel
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	require.Empty(t, loop.zoneDeaths)
	require.Equal(t, int32(-1), loop.zoneDeathCap)
}

// TestZoneRotationOnEmptySquare pins the forward sweep of the
// rotation: the emptied square takes its cooldown, so the next
// rotation moves past it onto the following sibling of the band.
func TestZoneRotationOnEmptySquare(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, zones[0].ID, loop.zonePickedID)

	// The character stands in the middle of the empty square, the
	// emptiness window is up: the rotation moves it to the nearest
	// sibling ground of the band.
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: zones[0].CX, Y: zones[0].CY, Z: -3500,
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	next := nearestZoneIndexOfBand(zones, 1, 3,
		zones[0].CX, zones[0].CY, map[string]bool{zones[0].ID: true})
	require.GreaterOrEqual(t, next, 0)
	require.Equal(t, zones[next].ID, loop.zonePickedID,
		"the empty square rotates to the nearest sibling")
	require.True(t, loop.zoneEmptySince.IsZero(),
		"the new square starts with a fresh emptiness window")
	require.False(t, loop.zoneCheckAt.IsZero(),
		"the rotation holds the ladder re-eval for a period")
	require.True(t, loop.zoneCoolingDown(zones[0].ID, time.Now()),
		"the rotated-away square keeps its empty cooldown")

	// The walk into the new square lands, it clears out as well: the
	// next rotation continues onto the following sibling (the first
	// square still cools down).
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: zones[next].CX, Y: zones[next].CY, Z: -3500,
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	third := nearestZoneIndexOfBand(zones, 1, 3,
		zones[next].CX, zones[next].CY,
		map[string]bool{zones[0].ID: true, zones[next].ID: true})
	require.GreaterOrEqual(t, third, 0)
	require.Equal(t, zones[third].ID, loop.zonePickedID,
		"the rotation sweeps forward past the cooling squares")
	require.True(t, loop.zoneCoolingDown(zones[next].ID, time.Now()))

	// The cooldown of the first squares expires: they rejoin the
	// rotation contest (the respawn refilled them by then).
	expired := time.Now().Add(-zoneEmptyCooldown - time.Second)
	loop.zoneEmptyUntil[zones[next].ID] = expired
	loop.zoneEmptyUntil[zones[0].ID] = expired
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: zones[third].CX, Y: zones[third].CY, Z: -3500,
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.NotEqual(t, zones[third].ID, loop.zonePickedID,
		"the emptied square rotates away again")
}

// TestZoneRotationCooldownBlocksTheReturn pins the reported ping pong:
// zone A empties, the hunter rotates to B; B empties too, and the
// nearest sibling of B is A again - the cooldown must hold A out of
// the contest so the sweep continues to C instead of walking back
// into the still empty A.
func TestZoneRotationCooldownBlocksTheReturn(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	a := loop.zonePickedID
	aZone, ok := loop.zoneByID(a)
	require.True(t, ok)

	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: aZone.CX, Y: aZone.CY, Z: -3500,
	})
	// First rotation: to the nearest sibling B.
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	b := loop.zonePickedID
	require.NotEqual(t, a, b)
	// The nearest sibling of B ignoring the cooldown would be A (the
	// two squares are the closest pair of the band).
	nearest := nearestZoneIndexOfBand(zones, 1, 3,
		aZone.CX, aZone.CY, map[string]bool{b: true})
	require.Equal(t, a, zones[nearest].ID,
		"the test setup needs B nearest to A")

	// Second rotation from B: A is cooling down, the sweep takes the
	// next sibling C instead.
	bZone, ok := loop.zoneByID(b)
	require.True(t, ok)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: bZone.CX, Y: bZone.CY, Z: -3500,
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	c := loop.zonePickedID
	require.NotEqual(t, b, c)
	require.NotEqual(t, a, c, "the cooldown must block the return to A")
}

func TestZoneRotationWaitsWhileMobsRemain(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	first := loop.zonePickedID
	firstZone, ok := loop.zoneByID(first)
	require.True(t, ok)

	// A living attackable mob inside the square (a young red keltir
	// of level 1, inside the level 1 character's target ceiling): no
	// rotation however long the window runs.
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: firstZone.CX, Y: firstZone.CY, Z: -3500,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 77, TemplateID: 1000530, Attackable: true,
		X: firstZone.CX - 300, Y: firstZone.CY, Name: "Young Red Keltir",
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	for range 3 {
		loop.tick()
	}
	require.Equal(t, first, loop.zonePickedID)
	require.True(t, loop.zoneEmptySince.IsZero())
}

// TestZoneRotationTreatsTooStrongMobsAsEmpty pins the level aware
// emptiness reading: a square whose only survivors sit above the
// engage ceiling is as good as empty for this hunter - standing in it
// forever waiting for a fight that never starts wastes the session.
func TestZoneRotationTreatsTooStrongMobsAsEmpty(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	first := loop.zonePickedID
	firstZone, ok := loop.zoneByID(first)
	require.True(t, ok)

	// An orc archer (level 8) wanders inside the keltir square of a
	// level 1 character: the mob never passes the engage ceiling, the
	// square counts as empty.
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: firstZone.CX, Y: firstZone.CY, Z: -3500,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 78, TemplateID: 1000006, Attackable: true,
		X: firstZone.CX - 300, Y: firstZone.CY, Name: "Orc Archer",
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.NotEqual(t, first, loop.zonePickedID,
		"a square of unfightable mobs rotates away")
}

func TestZoneRotationSkipsMidFight(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	first := loop.zonePickedID

	// A running fight (a live target) resets the emptiness window:
	// the rotation never abandons a fight.
	loop.target = 55
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, first, loop.zonePickedID)
	require.True(t, loop.zoneEmptySince.IsZero())
}

func TestZoneRotationWithoutSiblingStays(t *testing.T) {
	zones := []HuntingZone{
		{
			ID: "one", Name: "One", Region: "test",
			MinLevel: 1, MaxLevel: 3, MinGear: 0,
			CX: 46112, CY: 41500, Half: 1000,
		},
	}
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "one", loop.zonePickedID)

	// No sibling of the band: the empty window re-arms and the hunter
	// waits out the respawn of its only square.
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, "one", loop.zonePickedID)
	require.False(t, loop.zoneEmptySince.IsZero(),
		"the window re-arms instead of spinning the check")
}

// TestZoneRotationAllSiblingsCoolingWaits pins the exhausted sweep:
// every sibling of the band sits in its empty cooldown, so the hunter
// waits out the respawn in place instead of walking in circles.
func TestZoneRotationAllSiblingsCoolingWaits(t *testing.T) {
	zones := []HuntingZone{
		{
			ID: "one", Name: "One", Region: "test",
			MinLevel: 1, MaxLevel: 3, MinGear: 0,
			CX: 46112, CY: 41500, Half: 1000,
		},
		{
			ID: "two", Name: "Two", Region: "test",
			MinLevel: 1, MaxLevel: 3, MinGear: 0,
			CX: 48112, CY: 41500, Half: 1000,
		},
	}
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "one", loop.zonePickedID)

	// Both squares cooling: no rotation target exists, the window
	// re-arms.
	loop.markZoneEmpty("one", time.Now())
	loop.markZoneEmpty("two", time.Now())
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, "one", loop.zonePickedID)
	require.False(t, loop.zoneEmptySince.IsZero(),
		"the window re-arms while every sibling cools down")
}

func TestZoneSwitchStopsTheRunningWalks(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, zones[0].ID, loop.zonePickedID)

	// The bot walks a manual move toward the east when the user
	// switches the zone: the manual phase ends and the server walk is
	// stopped (one walk request to the current spot replaces the
	// running destination).
	loop.phase = phaseUser
	loop.userKind = "move"
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
		DestX: 47000, DestY: 50000, DestZ: -3500,
	})
	loop.userZoneSelect(1)
	require.Equal(t, zones[1].ID, loop.zonePickedID)
	require.NotEqual(t, phaseUser, loop.phase,
		"the manual move phase ends with the zone switch")
	require.NotEmpty(t, game.walks, "the running walk is stopped")
	stop := game.walks[len(game.walks)-1]
	require.Equal(t, [3]int32{45000, 50000, -3500}, stop,
		"the stop walks to the current spot")

	// A town trip walk to the trader is cancelled the same way.
	game.walks = nil
	loop.phase = phaseTownReturn
	loop.zoneReturn = true
	loop.zoneFails = 2
	loop.zonePickedID = zones[0].ID
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
		DestX: 47000, DestY: 50000, DestZ: -3500,
	})
	loop.userZoneSelect(2)
	require.Equal(t, phaseEngage, loop.phase,
		"the trip walk phase ends with the zone switch")
	require.False(t, loop.zoneReturn)
	require.Zero(t, loop.zoneFails)

	// The deleveling refuses the stop: its guard walk must finish.
	game.walks = nil
	loop.phase = phaseDelevel
	loop.userZoneSelect(3)
	require.Equal(t, phaseDelevel, loop.phase)
	require.Empty(t, game.walks)
}

func TestZoneSwitchDropsTheStaleFarmSpot(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := ElvenHuntingZones()
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, zones[0].ID, loop.zonePickedID)

	// The farm spot of the starter square (outside the sibling
	// ground): a zone switch must drop it, a return leg aims at the
	// new zone center instead of walking to the old square.
	loop.farmX, loop.farmY, loop.farmZ = 46200, 41600, -3455
	loop.userZoneSelect(1)
	require.Equal(t, zones[1].ID, loop.zonePickedID)
	require.Zero(t, loop.farmX)
	require.Zero(t, loop.farmY)
	zone := loop.zone()
	require.NotNil(t, zone)
	require.True(t, zone.Contains(
		zone.CX, zone.CY), "the fallback destination is the new center")
}

// TestZoneMobPriorityGuidesTheEngage pins the mob priorities of the
// zone data: the hunt farms every species inside the square, and the
// exp richer mob wins the pick among comparably near candidates (a
// far preferred mob still loses to a doorstep one).
func TestZoneMobPriorityGuidesTheEngage(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := []HuntingZone{
		{
			ID: "priority", Name: "Priority Ground", Region: "test",
			MinLevel: 1, MaxLevel: 3, MinGear: 0,
			CX: 46112, CY: 41500, Half: 1400,
			Mobs: []ZoneMob{
				{
					TemplateID: 1000001, Name: "Gremlin",
					Level: 1, Count: 3, Priority: 0,
				},
				{
					TemplateID: 1000002, Name: "Rabbit",
					Level: 1, Count: 3, Priority: 2,
				},
			},
		},
	}
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "priority", loop.zonePickedID)
	require.NotNil(t, loop.zoneMobPriority)
	require.Equal(t, int32(2), loop.zoneMobPriority[1000002])

	// A gremlin at 300 units and a rabbit at 600: the priority 2
	// rabbit (600 - 2x200 bias = 200) wins the pick over the 300
	// unit gremlin - the bias tilts comparable ranges.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 71, TemplateID: 1000001, Attackable: true,
		X: 46412, Y: 41500, Name: "Gremlin",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 72, TemplateID: 1000002, Attackable: true,
		X: 46712, Y: 41500, Name: "Rabbit",
	})
	loop.tick()
	require.Equal(t, int32(72), loop.target)

	// Both near the doorstep: the gremlin with a plain zero priority
	// at 200 units keeps losing to the rabbit at 400 (400 - 400 = 0
	// against 200) - still the rabbit. A far rabbit loses: at 1200
	// units its biased score (800) is far above the gremlin at 200,
	// the gremlin wins the pick.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 73, TemplateID: 1000002, Attackable: true,
		X: 47312, Y: 41500, Name: "Rabbit",
	})
	bot.ApplyStatusUpdate(72, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	bot.ApplyStatusUpdate(71, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 74, TemplateID: 1000001, Attackable: true,
		X: 46312, Y: 41500, Name: "Gremlin",
	})
	loop.target = 0
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, int32(74), loop.target,
		"the doorstep mob beats the far preferred one")
}

// TestZoneMobPriorityTranslatesTheRegistryIDs pins the id space of
// the generated zone registries: the zone mob lists carry the Mobius
// CT0 xml template ids of the spawn data (20471 for the Kaboo Orc
// Fighter) while the NpcInfo packets identify the same npcs by the
// C4 display ids plus the 1000000 offset, so the priority map keys
// translate onto the wire ids - the bias of the generated registries
// never matched a scan template id before and stayed dead.
func TestZoneMobPriorityTranslatesTheRegistryIDs(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zones := []HuntingZone{
		{
			ID: "kaboo", Name: "Kaboo Ground", Region: "test",
			MinLevel: 9, MaxLevel: 12, MinGear: 0,
			CX: 46112, CY: 41500, Half: 1400,
			Mobs: []ZoneMob{
				{
					TemplateID: 20471, Name: "Kaboo Orc Fighter",
					Level: 10, Count: 4, Priority: 1,
				},
				{
					TemplateID: 20473, Name: "Kaboo Orc Fighter Lieutenant",
					Level: 11, Count: 5, Priority: 2,
				},
			},
		},
	}
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 13)
	loop.tick()
	require.Equal(t, "kaboo", loop.zonePickedID)
	// The map keys are the wire template ids of the packets.
	require.NotNil(t, loop.zoneMobPriority)
	require.Equal(t, int32(1), loop.zoneMobPriority[1000471])
	require.Equal(t, int32(2), loop.zoneMobPriority[1000473])
	require.NotContains(t, loop.zoneMobPriority, int32(20471))

	// The translated bias guides the pick against wire-id scans: the
	// fighter 600 units east (600 - 1x200 = 400) loses to the
	// lieutenant 700 units west (700 - 2x200 = 300) - the priority 2
	// mob wins from farther out. The two stand 1300 units apart, past
	// the ORC clan fence of the social filter.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 81, TemplateID: 1000471, Attackable: true,
		X: 46712, Y: 41500, Name: "Kaboo Orc Fighter",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 82, TemplateID: 1000473, Attackable: true,
		X: 45412, Y: 41500, Name: "Kaboo Orc Fighter Lieutenant",
	})
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, int32(82), loop.target,
		"the translated priority bias guides the pick on the wire ids")
}

// setZoneTestLevel sets the character level through the userinfo
// path (the paperdoll resets with it, so equip the gear after) and
// places it at the elven village.
func setZoneTestLevel(bot *state.Bot, level int32) {
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: level,
		X: 46112, Y: 41500, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
	})
}

// equipZoneWithGear dresses the bot through the tracked inventory and
// paperdoll so the gear points reach the wanted level: the starter
// kit (short sword and shirt, 44), the wooden set step (with pants,
// 66) or the full shop dress (212 points: the long sword, the bone
// set, the helmet, the boots and the top jewels).
func equipZoneWithGear(bot *state.Bot, points int32) {
	items := []state.InventoryItem{
		{ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
		{ObjectID: 502, ItemID: 21, Count: 1, Equipped: true},
	}
	var paperdoll [state.PaperdollSlots]int32
	paperdoll[state.PaperdollRHand] = 501
	paperdoll[state.PaperdollChest] = 502
	switch {
	case points >= 200:
		items = []state.InventoryItem{
			{ObjectID: 501, ItemID: 2, Count: 1, Equipped: true},
			{ObjectID: 502, ItemID: 24, Count: 1, Equipped: true},
			{ObjectID: 503, ItemID: 31, Count: 1, Equipped: true},
			{ObjectID: 504, ItemID: 44, Count: 1, Equipped: true},
			{ObjectID: 505, ItemID: 38, Count: 1, Equipped: true},
			{ObjectID: 506, ItemID: 908, Count: 1, Equipped: true},
			{ObjectID: 507, ItemID: 845, Count: 1, Equipped: true},
			{ObjectID: 508, ItemID: 877, Count: 1, Equipped: true},
			{ObjectID: 509, ItemID: 877, Count: 1, Equipped: true},
		}
		paperdoll[state.PaperdollRHand] = 501
		paperdoll[state.PaperdollChest] = 502
		paperdoll[state.PaperdollLegs] = 503
		paperdoll[state.PaperdollHead] = 504
		paperdoll[state.PaperdollFeet] = 505
		paperdoll[state.PaperdollNeck] = 506
		paperdoll[state.PaperdollREar] = 507
		paperdoll[state.PaperdollRFinger] = 508
		paperdoll[state.PaperdollLFinger] = 509
	case points >= 66:
		items = append(items, state.InventoryItem{
			ObjectID: 503, ItemID: 28, Count: 1, Equipped: true,
		})
		paperdoll[state.PaperdollLegs] = 503
	case points >= 44:
	default:
		items = []state.InventoryItem{
			{ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
		}
		paperdoll = [state.PaperdollSlots]int32{}
		paperdoll[state.PaperdollRHand] = 501
	}
	bot.ApplyItemList(items)
	bot.ApplyPaperdoll(paperdoll)
}

// zoneDeathKill kills the character through the status update of the
// self object and runs one tick: recoverFromDeath counts the death
// against the active zone. The restart pacing clears first, so every
// kill of the sequence goes through the full recovery path.
func zoneDeathKill(bot *state.Bot, loop *Loop) {
	loop.restartAt = time.Time{}
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
}

// zoneDeathWaitRevival revives the character and clears the restart
// pacing so the next tick runs the living phases again.
func zoneDeathWaitRevival(bot *state.Bot) {
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
	})
}
