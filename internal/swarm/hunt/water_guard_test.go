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

	// The second and third wet clicks re-path again, the fourth
	// exhausts the budget: the re-paths keep reproducing the same wet
	// line, which means the geodata pack itself routes through the
	// water (the disconnected village decks under the plaza). The
	// plan is trusted - the click goes out and the server routing
	// carries the walk over the real plaza (the teacher legs of the
	// learning trips died on this abort before).
	loop.tick()
	loop.tick()
	require.Empty(t, game.walks)
	require.False(t, loop.wetPlanTrusted,
		"the re-path budget is not exhausted yet")
	loop.tick()
	require.True(t, loop.wetPlanTrusted,
		"the exhausted budget trusts the plan of the leg")
	require.Len(t, game.walks, 1,
		"the trusted click goes out over the server routing")

	// The trust holds for the whole leg: the next clicks skip the
	// guard (the follower clicks the remaining waypoints directly,
	// paced by the walk request period).
	loop.moveAt = time.Now().Add(-2 * walkRequestPeriod)
	loop.tick()
	require.Len(t, game.walks, 2)
}

// TestTripWetBudgetAbortsAfterATrustedSwim pins the trust bounds: the
// released leg plan is a gamble on the server routing - when the
// character genuinely ends up in the water (the standing check trips
// and the shore escape runs), a SECOND exhausted budget aborts the
// trip instead of looping the trust into the same lake forever.
func TestTripWetBudgetAbortsAfterATrustedSwim(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	fillInventory(bot)
	nav.wetLine = true
	nav.overWater = true
	nav.escapeRoute = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3850},
		{X: 45600, Y: 50400, Z: -3770},
	}
	moveSelfTo(bot, 45000, 50000, -3800)

	// The leg exhausts its budget and trusts the plan (no escape ran
	// yet), but the character actually stands over the lake bed: the
	// standing water check wins over the follower and the shore
	// escape takes over the tick.
	for range 4 {
		loop.tick()
	}
	require.True(t, loop.waterEscape,
		"the standing water check must own the tick over the trusted plan")
	require.Equal(t, 1, loop.waterEscapes,
		"the escape counts against the trust of the running trip")
	require.False(t, loop.wetPlanTrusted,
		"the escape re-arms the guard for the re-planned leg")

	// The character walks onto the shore, the leg re-plans, every
	// click of the new leg stays wet and the budget exhausts again:
	// this time an escape already ran, the trip aborts.
	nav.overWater = false
	moveSelfTo(bot, 45600, 50400, -3770)
	loop.tick()
	require.False(t, loop.waterEscape, "back on the shore")
	for range 6 {
		loop.tick()
	}
	require.Equal(t, phaseEngage, loop.phase,
		"the second exhaustion after a trusted swim aborts the trip")
	require.True(t, loop.tripEndedAt.After(loop.tripStart))
	_ = game
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
