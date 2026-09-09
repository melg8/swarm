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
// to the Spore Fungus SW-d1 hunting zone whose center sits on the
// ground 600+ below (z -3664). The zone return used to plan the
// search goal with the WALKER height at the zone center - the 3D
// approach goal sat mid air, the search burned the whole expansion
// cap (a thirteen second frozen hunt tick per attempt) and the
// fallback direct walk ran the character into the city railing.
var (
	cityWestEdge = Vec3{X: 42536, Y: 48648, Z: -2992}
	sporeZone    = Vec3{X: 36090, Y: 47434, Z: -2992}
)

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
	// the island (through the drop the geodata models at the city
	// edge) and ends within the approach radius of the resolved goal.
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
			spec.setCell(targetX+dx, targetY+dy, closedWalls(0))
		}
	}
	// A wall ring seals the playground: the reachable world stays
	// small so the starved search exhausts fast instead of burning
	// the expansion cap.
	for x := 950; x <= 1050; x++ {
		spec.setCell(x, 950, closedWalls(0))
		spec.setCell(x, 1050, closedWalls(0))
		spec.setCell(950, x, closedWalls(0))
		spec.setCell(1050, x, closedWalls(0))
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
