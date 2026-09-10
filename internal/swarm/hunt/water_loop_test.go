// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The delevel water loop regression of 2026-09-10: the state dump
// showed the bot cycling "level 11 is too high ... deleveling to 9 at
// the town guards" -> "walking to the guard Starden" -> "the walk
// would enter water at 40648 43432, re-pathing (1..3 of 3)" -> "town
// trip ended: aborted, the walk would cross water" every 1.3 seconds
// for 25 minutes straight. Two defects chained: the planner planned
// water crossing routes the click guard refused (fixed by the dry
// search of startWalkLeg), and the abort of a delevel walk ended only
// the town trip - the delevel state stayed armed without a cooldown,
// so the very next tick restarted the walk into the same blocker.

// TestDelevelWaterAbortArmsCooldown pins the loop break: a deleveling
// whose walk machinery aborts - here a character standing over water
// without any shore path - ends the DELEVELING (the cooldown arms, the
// walk home starts) instead of only the town trip, and the following
// ticks never restart the deleveling while the cooldown holds.
func TestDelevelWaterAbortArmsCooldown(t *testing.T) {
	loop, _, _, nav := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)
	nav.overWater = true // no escapeRoute: the shore search fails

	loop.tick()
	require.NotEqual(t, phaseDelevel, loop.phase,
		"the water abort must end the deleveling, not only the trip")
	require.False(t, loop.delevelEnd.IsZero(),
		"the abort arms the delevel cooldown")
	require.False(t, loop.delevelCooldownOver(),
		"the cooldown blocks the immediate restart")

	// The cooldown holds: no tick restarts the deleveling while it
	// runs (the old loop re-entered the delevel phase every 1.3 s).
	for range 8 {
		loop.tick()
		require.NotEqual(t, phaseDelevel, loop.phase,
			"the deleveling must stay down while the cooldown runs")
	}
}

// TestDelevelWetPlanTrustsAfterBudget pins the composition with the
// plaza raster artifacts (the geodata cells without a modeled floor
// resolving to the lake layer below): wet click lines on a planned
// delevel walk exhaust the budget, the plan is trusted for the leg
// and the deleveling keeps walking over the server routing - the old
// refuse-and-restart cycle is gone. A genuine swim mid-route still
// ends the trip: the standing water check runs the shore escape, and
// the next budget exhaustion aborts (into the deleveling itself, see
// TestDelevelWaterAbortArmsCooldown).
func TestDelevelWetPlanTrustsAfterBudget(t *testing.T) {
	loop, game, _, nav := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)
	nav.wetLine = true

	// The trigger tick burns the first re-path, three more exhaust
	// the budget of 3 and trust the planned leg.
	for range 4 {
		loop.tick()
	}
	require.True(t, loop.wetPlanTrusted,
		"the budget exhaustion trusts the planned leg")
	require.Equal(t, phaseDelevel, loop.phase,
		"the trusted leg keeps the deleveling walking")
	require.NotEmpty(t, game.walks,
		"the trusted legs send their click walks")
}

// TestDelevelWetClicksNeverWalk pins the walk side of the same scene:
// while the delevel walk fights the wet lines, not a single move
// request goes to the server - the refused clicks re-path instead.
func TestDelevelWetClicksNeverWalk(t *testing.T) {
	loop, game, _, nav := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)
	nav.wetLine = true

	for range 3 {
		loop.tick()
	}
	require.Empty(t, game.walks,
		"the wet click lines must never reach the server")
	require.Equal(t, 3, loop.rePaths,
		"every refused click counts against the re-path budget")
}

// TestDelevelDryMissAborts pins the planner side of the same scene: a
// guard the dry search cannot reach aborts the deleveling at the
// planning tick - the cooldown arms and the walk home starts, the
// loop never enters the refuse-and-restart cycle.
func TestDelevelDryMissAborts(t *testing.T) {
	loop, game, _, nav := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)
	nav.dryMiss = true

	loop.tick()
	require.NotEqual(t, phaseDelevel, loop.phase,
		"the deleveling aborts without a dry path to the guard")
	require.False(t, loop.delevelEnd.IsZero(),
		"the abort arms the delevel cooldown")
	require.False(t, loop.delevelCooldownOver(),
		"the cooldown blocks the immediate restart")
	require.Empty(t, game.walks,
		"no walk goes out without a dry plan")
}
