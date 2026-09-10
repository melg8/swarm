// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The dry search regression of 2026-09-10 (the delevel water loop):
// the town walker refuses wet click lines, but the planner kept
// planning water crossing routes - the water step only cost three
// land steps, so the cheapest route to the far shore of the elven
// village bay swam across it. The walker re-planned the identical wet
// route three times, aborted, and the deleveling restarted the whole
// cycle every 1.3 seconds forever. FindPathApproachDry walls the
// water off: the routes it plans the walker can actually walk.

// TestDryApproachDetoursAroundAChannel pins the positive case: a
// shallow channel splits two dry points, the dry search still finds
// the land detour and every waypoint of the route stands above the
// water level.
func TestDryApproachDetoursAroundAChannel(t *testing.T) {
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
	result, err := engine.FindPathApproachDry(
		start, end, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found, "the land detour around the channel exists")
	for i, wp := range result.Waypoints {
		require.GreaterOrEqual(t, wp.Z, float64(waterLevel),
			"waypoint %d must stay dry", i)
	}
	// Every leg is a clean dry walk: the click guard passes them.
	for i := 1; i < len(result.Waypoints); i++ {
		dry, err := engine.DryLine(
			result.Waypoints[i-1], result.Waypoints[i])
		require.NoError(t, err)
		require.True(t, dry, "the leg %d must stay dry", i-1)
	}
}

// TestDryApproachRefusesASwimOnlyTarget pins the wall semantics: a
// strait without any land crossing is found by the ordinary search
// (it swims) and answered not found by the dry one - the caller aborts
// the leg instead of planning a route the walker refuses.
func TestDryApproachRefusesASwimOnlyTarget(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(shoreLand)
	// The strait spans the whole region side as whole flat water
	// blocks (whole blocks keep the neighbors of every water cell at
	// the bed height): no dry crossing exists.
	waterBlock := blockSpec{
		kind:  blockFlat,
		cells: [cellsPerBlock]Layer{{Height: shoreBed, NSWE: nsweAll}},
	}
	for bx := 37; bx <= 45; bx++ {
		for by := range blocksPerRegionSide {
			spec.blocks[bx][by] = waterBlock
		}
	}
	engine := newTestEngine(t, spec)

	start := worldOf(200, 700, shoreLand)
	end := worldOf(420, 700, shoreLand)
	wet, err := engine.FindPathApproach(
		start, end, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, wet.Found,
		"the ordinary search still plans the swim across the strait")
	dry, err := engine.FindPathApproachDry(
		start, end, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, dry.Found,
		"the dry search refuses the swim only target")
}

// TestDelevelShoreRouteStaysDry replays the reported loop scene
// against the real geodata pack: the character stood at the elven
// village shore (40648 43432, the state dump of the hang) and the
// deleveling planned its walk to the guard Starden straight through
// the bay - the wet first waypoint is the route the click guard
// refused three times per cycle. The dry search must not produce such
// a route: found means every leg is a clean dry walk, and the
// ordinary search of the same pair must carry the wet waypoint that
// proves the regression existed.
func TestDelevelShoreRouteStaysDry(t *testing.T) {
	engine := townTestEngine(t)

	shore := Vec3{X: 40648, Y: 43432, Z: -3624}
	starden := Vec3{X: 42971, Y: 51372, Z: -2992}

	wet, err := engine.FindPathApproach(
		shore, starden, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, wet.Found, "the ordinary search plans the bay crossing")
	wetLegs := 0
	for i := 1; i < len(wet.Waypoints); i++ {
		dry, err := engine.DryLine(
			wet.Waypoints[i-1], wet.Waypoints[i])
		require.NoError(t, err)
		if !dry {
			wetLegs++
		}
	}
	require.Positive(t, wetLegs,
		"the ordinary route crosses the water - the plan the click "+
			"guard refused in the reported loop")

	dryRoute, err := engine.FindPathApproachDry(
		shore, starden, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	if !dryRoute.Found {
		// No dry route exists on the pack: the honest miss is the
		// correct answer (the caller aborts and arms its cooldown),
		// the loop is broken either way.
		return
	}
	for i := 1; i < len(dryRoute.Waypoints); i++ {
		dry, err := engine.DryLine(
			dryRoute.Waypoints[i-1], dryRoute.Waypoints[i])
		require.NoError(t, err)
		require.True(t, dry, "the dry route leg %d from %.0f %.0f must "+
			"stay dry", i-1,
			dryRoute.Waypoints[i-1].X, dryRoute.Waypoints[i-1].Y)
	}
}
