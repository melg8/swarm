// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The water regression of 2026-09-10: the town trip to the trader
// Ariel entered the elven village lake, the character swam below the
// plateau and stood paralyzed under the village cliff (the water zone
// of the C1 server covers everything below -3780; a swimming
// character whose z floats above that bound loses the swim move
// semantics while the geodata still resolves it onto the lake bed,
// and every click toward the village deck returns the character's own
// position). The tests here pin the three defenses: the smoothed
// routes never ford water between dry points, the escape search
// finds the nearest shore for a position standing in a lake, and the
// click lines of the walker verify dry before they are sent.

// shoreLand is the dry test land height (above the C1 water surface).
const shoreLand = int16(-3770)

// shoreBed is the underwater test bed height: 30 below the land, so
// the cell steps of a shore ramp stay within the passable height and
// the line of sight across the bed passes - the exact shape of the
// elven lake the string pulling used to cut through.
const shoreBed = int16(-3800)

// TestSmoothedLegsStayDry pins the water awareness of the smoothing:
// a shallow channel (the bed within the passable step of its shores)
// splits two dry points, the cost aware search detours around it and
// the smoothing must not collapse the detour back into the straight
// water crossing - every leg of the smoothed route has to stay dry
// and walkable, the invariant the town walker's click guard relies on.
func TestSmoothedLegsStayDry(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	// The channel strip: underwater, the shores of every cell open.
	for x := 300; x <= 500; x++ {
		for y := 500; y <= 900; y++ {
			spec.setCell(x, y, Layer{Height: shoreBed, NSWE: nsweAll})
		}
	}
	engine := newTestEngine(t, spec)

	start := worldOf(200, 700, shoreLand)
	end := worldOf(600, 700, shoreLand)
	result, err := engine.FindPath(
		start, end, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found, "the detour around the channel exists")
	require.GreaterOrEqual(t, len(result.Waypoints), 3,
		"the route must keep the shore corners the search paid for")
	for i, wp := range result.Waypoints {
		require.GreaterOrEqual(t, wp.Z, float64(waterLevel),
			"waypoint %d must stay dry", i)
	}
	// Every smoothed leg is a clean dry walk: the follower may click
	// straight along each leg without entering the water.
	for i := 1; i < len(result.Waypoints); i++ {
		dry, err := engine.DryLine(
			result.Waypoints[i-1], result.Waypoints[i])
		require.NoError(t, err)
		require.True(t, dry, "the leg %d must stay dry", i-1)
	}
}

// TestFindWaterEscape verifies the escape search on a lake with one
// gradual ramp: the flood finds the nearest dry cell through the
// ramp and the smoothed escape path ends above the water level.
func TestFindWaterEscape(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	// The lake bed, walled off from the land by the 80 unit shore
	// cliff everywhere except the east ramp.
	for x := 200; x <= 600; x++ {
		for y := 200; y <= 600; y++ {
			spec.setCell(x, y, Layer{Height: -3850, NSWE: nsweAll})
		}
	}
	// The east ramp: three cells climbing 32 units each onto the land.
	for i, height := range []int16{-3818, -3786, -3754} {
		spec.setCell(601+i, 400, Layer{Height: height, NSWE: nsweAll})
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindWaterEscape(
		worldOf(400, 400, -3850))
	require.NoError(t, err)
	require.True(t, result.Found, "the ramp connects the lake to the land")
	require.NotEmpty(t, result.Waypoints)
	last := result.Waypoints[len(result.Waypoints)-1]
	require.GreaterOrEqual(t, last.Z, float64(waterLevel),
		"the escape must end on dry ground")
	// The raw path walks the ramp gradually: every step stays within
	// the passable height (the escape shares the canStep rules).
	for i := 1; i < len(result.RawPath); i++ {
		delta := result.RawPath[i].Z - result.RawPath[i-1].Z
		require.LessOrEqual(t, delta, float64(DefaultMaxPassableHeight),
			"the escape step %d must stay climbable", i)
		require.GreaterOrEqual(t, delta, -float64(DefaultMaxPassableHeight),
			"the escape step %d must stay droppable", i)
	}
}

// TestFindWaterEscapeSealedLake verifies the escape on a lake whose
// whole shore is a cliff: no walkable connection exists, the flood
// exhausts the bed and the search reports no escape.
func TestFindWaterEscapeSealedLake(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	for x := 200; x <= 600; x++ {
		for y := 200; y <= 600; y++ {
			spec.setCell(x, y, Layer{Height: -3850, NSWE: nsweAll})
		}
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindWaterEscape(worldOf(400, 400, -3850))
	require.NoError(t, err)
	require.False(t, result.Found,
		"the cliff shore leaves no walkable escape")
}

// TestFindWaterEscapeDryStart documents the dry start contract: a
// position standing above the water level needs no escape and the
// search answers found=false without an error.
func TestFindWaterEscapeDryStart(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	engine := newTestEngine(t, spec)

	result, err := engine.FindWaterEscape(worldOf(400, 400, shoreLand))
	require.NoError(t, err)
	require.False(t, result.Found)
}

// TestDryLine verifies the click guard query: a line crossing the
// water is wet whatever it endpoints stand on, a dry line over open
// land passes, and a walled line fails the walkability half.
func TestDryLine(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	for x := 300; x <= 500; x++ {
		for y := 500; y <= 900; y++ {
			spec.setCell(x, y, Layer{Height: shoreBed, NSWE: nsweAll})
		}
	}
	// A wall strip on the open land south of the channel.
	for y := 700; y <= 900; y++ {
		spec.setCell(550, y, Layer{Height: shoreLand, NSWE: 0})
	}
	engine := newTestEngine(t, spec)

	// A dry open line passes.
	dry, err := engine.DryLine(
		worldOf(100, 700, shoreLand), worldOf(290, 700, shoreLand))
	require.NoError(t, err)
	require.True(t, dry)

	// The line across the channel enters the water.
	wet, err := engine.DryLine(
		worldOf(200, 700, shoreLand), worldOf(600, 700, shoreLand))
	require.NoError(t, err)
	require.False(t, wet, "the line crossing the channel is wet")

	// The walled line is not a clean walk either.
	blocked, err := engine.DryLine(
		worldOf(540, 700, shoreLand), worldOf(560, 700, shoreLand))
	require.NoError(t, err)
	require.False(t, blocked, "the walled line is not walkable")
}

// TestOverWater verifies the over water query: the layer closest to
// the reference z decides, so a swimmer above the bed of a multilayer
// cell reports over water while a character on the deck above the
// same cell does not.
func TestOverWater(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	spec.setMultilayer(500, 500, []Layer{
		{Height: -3850, NSWE: nsweAll},
		{Height: -2992, NSWE: nsweAll},
	})
	engine := newTestEngine(t, spec)

	bed := worldOf(500, 500, -3738)
	deck := worldOf(500, 500, -2992)
	land := worldOf(100, 100, shoreLand)
	require.True(t, engine.OverWater(bed.X, bed.Y, -3738),
		"a character floating above the bed stands over water")
	require.False(t, engine.OverWater(deck.X, deck.Y, -2992),
		"a character on the deck is ashore")
	require.False(t, engine.OverWater(land.X, land.Y, shoreLand),
		"the flat dry land is never water")
}

// TestElvenLakeStuckEscape replays the reported stuck case against
// the real geodata pack: the character swam into the elven village
// lake and stood below the plateau cliff at 47136 46564 -3738. The
// escape search must find the nearest shore from there, the town
// route from the hunting grounds must stay dry leg by leg, and the
// over water query must separate the lake position from the village
// deck position.
func TestElvenLakeStuckEscape(t *testing.T) {
	engine := townTestEngine(t)

	stuck := Vec3{X: 47136, Y: 46564, Z: -3738}

	// The stuck position stands over the lake bed.
	require.True(t, engine.OverWater(stuck.X, stuck.Y, int16(stuck.Z)),
		"the reported stuck position is over the elven lake")
	// The village plateau does not.
	require.False(t, engine.OverWater(45480, 46680, -2992),
		"the village deck is dry land")

	// The escape finds a shore.
	escape, err := engine.FindWaterEscape(stuck)
	require.NoError(t, err)
	require.True(t, escape.Found, "the elven lake has walkable shores")
	require.NotEmpty(t, escape.Waypoints)
	last := escape.Waypoints[len(escape.Waypoints)-1]
	require.GreaterOrEqual(t, last.Z, float64(waterLevel),
		"the escape ends on dry ground, not in the lake")
	require.False(t, engine.OverWater(last.X, last.Y, int16(last.Z)),
		"the escape target itself is ashore")

	// The town trip from the hunting spot to the trader Ariel plans
	// dry legs only (the follower clicks along them unchecked by the
	// server's own water blind routing).
	spot := Vec3{X: 53504, Y: 45249, Z: -3520}
	ariel := Vec3{X: 44683, Y: 46952, Z: -2981}
	route, err := engine.FindPathApproach(
		spot, ariel, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, route.Found)
	for i := 1; i < len(route.Waypoints); i++ {
		dry, err := engine.DryLine(
			route.Waypoints[i-1], route.Waypoints[i])
		require.NoError(t, err)
		require.True(t, dry,
			"the town route leg %d from %.0f %.0f must stay dry",
			i-1, route.Waypoints[i-1].X, route.Waypoints[i-1].Y)
	}
}
