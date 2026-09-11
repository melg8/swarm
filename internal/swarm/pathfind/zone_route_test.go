// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The live stuck case of the manual hunting zone switch: the
// character stood on the floating elven city deck (z -2992) heading
// to a Spore Fungus hunting zone whose center sits on the ground
// hundreds of units below. The zone return used to plan the search
// goal with the WALKER height at the zone center - the 3D approach
// goal sat mid air, the search burned the whole expansion cap (a
// thirteen second frozen hunt tick per attempt) and the fallback
// direct walk ran the character into the city railing. The fixed
// goal height found a route - but the asymmetric step rule (any drop
// walkable) planned it as a jump off the city deck into the lake and
// a swim west: the geodata does not model the deck railing, so the
// A* happily dropped 920 units at the deck edge while the character
// ground into the railing and re-pathed the same route forever. The
// terrace rule (a step over the passable height is a terrace
// boundary, only gradual ramps connect the surfaces) now seals the
// drop and routes the descent through the city bridges.
var (
	cityWestEdge = Vec3{X: 42536, Y: 48648, Z: -2992}
	sporeZone    = Vec3{X: 36090, Y: 47434, Z: -2992}
	// The live dump position heading to the Spore Fungus SW-e1 zone:
	// the follower consumed the deck edge drop waypoint (80 units
	// away in 2D, 920 below) and ground the long west leg into the
	// city walls at exactly this spot.
	liveDeckStuck = Vec3{X: 42440, Y: 49032, Z: -2992}
	sporeZoneEast = Vec3{X: 32206, Y: 49064, Z: -2992}
)

// assertRampSteps pins the terrace invariant of a found route: every
// step of the raw cell path stays within the passable height, so the
// route never leaves the walkable surface (no deck edge jumps, no
// cliffs).
func assertRampSteps(t *testing.T, result *Result) {
	t.Helper()
	require.NotEmpty(t, result.RawPath)
	for i := 1; i < len(result.RawPath); i++ {
		step := math.Abs(result.RawPath[i].Z - result.RawPath[i-1].Z)
		require.LessOrEqual(t, step, float64(DefaultMaxPassableHeight),
			"raw path step %d drops %.0f units - a terrace boundary, "+
				"the route must use a ramp", i, step)
	}
}

// TestFindPathFromCityDeckToGroundZone pins the fixed zone return
// against the real geodata: the goal height is resolved to the real
// deck under the zone center, and the route from the floating city
// deck to the ground zone exists, plans fast and ends within the
// approach radius of the resolved goal.
func TestFindPathFromCityDeckToGroundZone(t *testing.T) {
	engine := townTestEngine(t)

	// The zone return resolves the deck under the zone center the way
	// the server resolves a destination: the layer closest to the
	// walker height - the ground, not the mid air city height.
	height, err := engine.ClosestHeight(
		sporeZone.X, sporeZone.Y, int16(cityWestEdge.Z))
	require.NoError(t, err)
	require.EqualValues(t, -3664, height,
		"the spore zone center sits on the ground deck")

	goal := Vec3{X: sporeZone.X, Y: sporeZone.Y, Z: float64(height)}
	result, err := engine.FindPathApproach(
		cityWestEdge, goal, 200, engine.MaxPassableHeight())
	require.NoError(t, err)
	require.True(t, result.Found,
		"the route from the city deck to the ground zone must exist")
	require.False(t, result.Aborted)
	require.Greater(t, result.Length, 5000.0,
		"the route spans the west of the elven lands")
	require.Less(t, result.Duration, 5*time.Second,
		"the planned route must not freeze the hunt tick")

	// The route descends from the city deck onto the ground west of
	// the island through the city bridge ramps (every raw step within
	// the passable height) and ends within the approach radius of the
	// resolved goal.
	assertRampSteps(t, result)
	require.LessOrEqual(t, result.Waypoints[0].Z, -2990.0)
	end := result.Waypoints[len(result.Waypoints)-1]
	dist := math.Sqrt(
		(end.X-goal.X)*(end.X-goal.X) +
			(end.Y-goal.Y)*(end.Y-goal.Y) +
			(end.Z-goal.Z)*(end.Z-goal.Z))
	require.LessOrEqual(t, dist, 200.0,
		"the route ends within the approach radius of the zone goal")
	require.GreaterOrEqual(t, end.Z, -3780.0,
		"the route ends on dry ground, not under the water surface")
}

// TestFindPathFailsOnAFabricatedGoalHeight pins the mechanism behind
// the live case on a cheap synthetic world: a search goal that floats
// far above the reachable ground can never satisfy the 3D approach
// radius, so the search fails - while the resolved goal on the real
// deck under the same target is reached. This is why the zone return
// must resolve the real deck height before planning.
func TestFindPathFailsOnAFabricatedGoalHeight(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	// The counter: the target cell and its whole ring are sealed (the
	// merchant behind the boards of the town trip tests).
	targetX, targetY := 1000, 1000
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			spec.setCell(targetX+dx, targetY+dy, closedWalls())
		}
	}
	// A wall ring seals the playground: the reachable world stays
	// small so the starved search exhausts fast instead of burning
	// the expansion cap.
	for x := 950; x <= 1050; x++ {
		spec.setCell(x, 950, closedWalls())
		spec.setCell(x, 1050, closedWalls())
		spec.setCell(950, x, closedWalls())
		spec.setCell(1050, x, closedWalls())
	}
	engine := newTestEngine(t, spec)
	start := worldOf(960, 960, 0)
	target := worldOf(targetX, targetY, 0)

	// The fabricated goal: 250 units above the ground at the target.
	// No reachable cell ever comes within the 200 radius of it - the
	// walk fails.
	fabricated, err := engine.FindPathApproach(
		start, Vec3{X: target.X, Y: target.Y, Z: 250},
		200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, fabricated.Found,
		"the mid air goal starves the search")

	// The resolved goal on the deck under the same target: the walk
	// stops at the counter front, within the radius.
	height, err := engine.ClosestHeight(target.X, target.Y, 250)
	require.NoError(t, err)
	require.EqualValues(t, 0, height)
	ground, err := engine.FindPathApproach(
		start, Vec3{X: target.X, Y: target.Y, Z: float64(height)},
		200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, ground.Found, "the resolved goal is reachable")
}

// TestFindPathFromDeckToFarWestZoneStaysOnRamps pins the live dump
// case: the character stood at the west quarter of the floating city
// deck (grinding into the building walls of the shop row after the
// follower consumed the deck edge drop waypoint) heading to the
// Spore Fungus SW-e1 zone center on the far west ground. The route
// must exist, stay on the ramps (the terrace invariant) and reach
// the approach radius of the resolved zone goal - the bridge detour
// east or south and the walk west on the ground, never a jump off
// the deck and never a swim under the water surface.
func TestFindPathFromDeckToFarWestZoneStaysOnRamps(t *testing.T) {
	engine := townTestEngine(t)

	height, err := engine.ClosestHeight(
		sporeZoneEast.X, sporeZoneEast.Y, int16(liveDeckStuck.Z))
	require.NoError(t, err)
	require.Less(t, height, int16(-3600),
		"the SW-e1 zone center sits on the ground deck")

	goal := Vec3{X: sporeZoneEast.X, Y: sporeZoneEast.Y, Z: float64(height)}
	result, err := engine.FindPathApproach(
		liveDeckStuck, goal, 200, engine.MaxPassableHeight())
	require.NoError(t, err)
	require.True(t, result.Found,
		"the far west zone route must exist from the deck")
	require.False(t, result.Aborted)
	require.Less(t, result.Duration, 5*time.Second,
		"the planned route must not freeze the hunt tick")
	assertRampSteps(t, result)

	// The bridge detour beats the swim: the route stays above the
	// water surface except the shore crossing onto dry ground.
	deep := 0
	for _, wp := range result.Waypoints {
		if wp.Z < -3780 {
			deep++
		}
	}
	require.Zero(t, deep, "the route must not swim to the zone")
	require.Greater(t, result.Length, 10000.0,
		"the bridge detour spans the west of the elven lands")
}
