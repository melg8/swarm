// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The water regression of 2026-09-10 (see pathfind/water_escape_test.go
// for the engine side): the town trip to the trader swam below the
// elven village plateau and stood paralyzed under its cliff - the
// server walks characters into water without any hesitation (no water
// cost in its own routing, and swimming move requests skip the
// geodata validation entirely) while every click toward the village
// deck above returns the character's own position. The follower now
// refuses wet click lines and recovers through the shore escape.

// TestTripWaterEscapePlansShoreWalk pins the recovery entry: a
// character standing over a lake bed during a town trip replaces the
// leg with the shore escape walk, clicks along the escape waypoints
// without the water guard and never towards the original village
// waypoint while it stands in the water.
func TestTripWaterEscapePlansShoreWalk(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.overWater = true
	nav.escapeRoute = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3850},
		{X: 45600, Y: 50400, Z: -3770},
	}
	// The character floats over the lake bed (the first escape
	// waypoint is its own standing cell, like the real BFS plans).
	moveSelfTo(bot, 45000, 50000, -3800)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase,
		"the trip keeps walking while the escape runs")
	require.True(t, loop.waterEscape,
		"the water escape must be armed")
	require.Equal(t, nav.escapeRoute, loop.waypoints,
		"the escape waypoints must replace the trip leg")
	require.Zero(t, loop.wpIndex)
	require.Empty(t, game.walks,
		"the planning tick sends no walk yet")
	require.Equal(t, 1, nav.escapeCalls)

	// The next tick walks the escape: the click aims at the shore
	// waypoint even though the original village waypoint is closer -
	// the unclimbable cliff waypoint must not win the skip logic.
	loop.tick()
	require.Equal(t, [][3]int32{{45600, 50400, -3770}}, game.walks,
		"the escape walk must aim at the shore waypoint")
}

// TestTripWaterEscapeReplansLegOnShore pins the recovery exit: once
// the character walks onto dry ground, the escape drops and the
// interrupted trip leg re-plans from the shore with a fresh re-path
// budget.
func TestTripWaterEscapeReplansLegOnShore(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.overWater = true
	nav.escapeRoute = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3850},
		{X: 45600, Y: 50400, Z: -3770},
	}

	loop.tick()
	require.True(t, loop.waterEscape)
	// The character walks out of the water.
	nav.overWater = false
	moveSelfTo(bot, 45600, 50400, -3770)
	loop.tick()
	require.False(t, loop.waterEscape,
		"the escape must drop on the dry shore")
	require.Zero(t, loop.rePaths,
		"the recovery leaves a fresh re-path budget")
	require.NotNil(t, loop.waypoints)
	require.NotEqual(t, nav.escapeRoute, loop.waypoints,
		"the trip leg must re-plan to the trader")
	legDest := state.WalkPoint{
		X: herbielPos[0], Y: herbielPos[1], Z: herbielPos[2],
	}
	snap := bot.Snapshot()
	require.NotNil(t, snap.WalkDest)
	require.Equal(t, legDest, *snap.WalkDest,
		"the re-planned leg still aims at the trader")
	require.Empty(t, game.walks,
		"the shore tick plans the new leg without walking yet")
}

// TestTripWaterEscapeWithoutShoreAborts pins the failure path: a
// character standing in water without any walkable shore aborts the
// trip instead of clicking into the cliff forever.
func TestTripWaterEscapeWithoutShoreAborts(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.overWater = true

	loop.tick()
	require.NotEqual(t, phaseTownWalk, loop.phase,
		"the trip must abort without a shore path")
	require.Equal(t, phaseEngage, loop.phase)
	require.False(t, loop.tripCooldownOver(),
		"the abort arms the trip cooldown")
}

// TestTripWaterEscapeStuckReplans pins the stuck handling of the
// escape: a swimming character that stands still re-plans the escape
// itself, not the town leg.
func TestTripWaterEscapeStuckReplans(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.overWater = true
	nav.escapeRoute = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3850},
		{X: 45600, Y: 50400, Z: -3770},
	}

	loop.tick()
	require.True(t, loop.waterEscape)
	legSearches := len(nav.approachEnds)
	// The character stands still in the water past the stuck window.
	for range 20 {
		loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
		loop.tick()
	}
	require.GreaterOrEqual(t, nav.escapeCalls, 2,
		"the stuck escape must re-plan the escape itself")
	require.Len(t, nav.approachEnds, legSearches,
		"the stuck escape must never re-plan the town leg")
	require.Equal(t, phaseEngage, loop.phase,
		"the escape exhausts its budget and aborts the trip")
}

// TestTripWetClickRepatsAroundShore pins the click water guard: a dry
// character whose straight click line would enter the water never
// sends the click - the walk re-paths around the shore instead.
func TestTripWetClickRepatsAroundShore(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.wetLine = true

	loop.tick()
	require.Empty(t, game.walks,
		"the wet click must never be sent to the server")
	require.Equal(t, 1, loop.rePaths,
		"the refused click counts as a re-path")
	require.Len(t, nav.approachEnds, 2,
		"the trip planned its leg and re-planned around the shore")

	// The second wet click re-paths again, the third and fourth
	// exhaust the budget and abort the trip (the walk cannot cross
	// the water and no shore route exists).
	loop.tick()
	loop.tick()
	loop.tick()
	require.Empty(t, game.walks)
	require.Equal(t, phaseEngage, loop.phase,
		"the trip must abort when every dry re-path crosses water")
}

// TestTripDryClickStillWalks pins the guard neutral path: a dry click
// line passes through unchanged (the ordinary town walk behavior).
func TestTripDryClickStillWalks(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)

	loop.tick()
	require.Len(t, game.walks, 1,
		"the dry click must be sent as before")
}

// TestTripWalkPlanCarriesFullLeg pins the full walk plan of a town
// trip: the origin where the leg was planned, every planned waypoint
// with the follower cursor and the trader destination - the whole
// walk from where we wanted to go to where we want to arrive.
func TestTripWalkPlanCarriesFullLeg(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.route = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3500},
		{X: 44000, Y: 50500, Z: -3520},
		{X: 43000, Y: 50500, Z: -3540},
	}

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Len(t, game.walks, 1)
	snap := bot.Snapshot()
	require.Len(t, snap.WalkPath, 3,
		"the full plan publishes, passed waypoints included")
	require.Equal(t, &state.WalkPoint{
		X: 45000, Y: 50000, Z: -3500,
	}, snap.WalkOrigin, "the origin is the planning position")
	require.Equal(t, &state.WalkPoint{
		X: herbielPos[0], Y: herbielPos[1], Z: herbielPos[2],
	}, snap.WalkDest, "the destination is the trader spawn")

	// The character passes the second waypoint: the plan keeps them
	// all and the cursor marks the current target.
	moveSelfTo(bot, 44000, 50500, -3520)
	loop.tick()
	cursored := bot.Snapshot()
	require.Len(t, cursored.WalkPath, 3,
		"the passed waypoints stay in the published plan")
	require.Equal(t, 2, cursored.WalkIndex,
		"the cursor marks the waypoint ahead")
}
