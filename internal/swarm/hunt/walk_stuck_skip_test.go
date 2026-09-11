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

// The town walk stuck loop of the 2026-09-11 state dump: the bot
// stood at the elven village teacher plaza (44440 52552 -2832),
// planned a dry route to the trader Herbiel and sent walk clicks at
// the first waypoint, but the character never moved - the server
// refused the click (the target cell sat on a blocked surface the
// bot's geodata resolved differently). The deterministic A* re-planned
// the identical route from the identical start, the walker burned its
// whole re-path budget on the same refused click and the trip aborted.
// The fix: on a stuck, SKIP the current waypoint and try the next one
// before re-planning the whole leg. The next waypoint may be reachable
// through a different cell the server accepts.

// armStuck simulates the character standing still past the stuck
// timeout: the stuck timer is set far enough in the past that the
// next walkStuck call fires, and the stuck position matches the
// character's current position so the position-change check does not
// reset the timer.
func armStuck(loop *Loop, bot *state.Bot) {
	selfX, selfY, _, ok := bot.SelfPosition()
	if !ok {
		return
	}
	loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
	loop.stuckX = selfX
	loop.stuckY = selfY
}

// TestWalkStuckSkipsCurrentWaypoint verifies the skip: a planned route
// of three waypoints (start, mid, end) where the character stands still
// at the mid waypoint. The first stuck fires the skip, the follower
// cursor advances to the end waypoint and a fresh walk click targets
// the end waypoint (not the mid one the server refused). The skip does
// NOT consume the re-path budget (rePaths stays zero): the budget bounds
// the expensive full leg re-plan, not the cursor advance.
func TestWalkStuckSkipsCurrentWaypoint(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	nav.found = true
	nav.route = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3500},
		{X: 44800, Y: 50200, Z: -3500},
		{X: 44600, Y: 50400, Z: -3500},
	}
	fillInventory(bot)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Len(t, loop.waypoints, 3)
	// The first waypoint (the start) is already reached: the follower
	// cursor sits on the mid waypoint.
	require.Equal(t, 1, loop.wpIndex)
	require.Len(t, game.walks, 1)

	// Arm the stuck timer past the timeout and tick: the skip fires.
	armStuck(loop, bot)
	loop.tick()
	require.Zero(t, loop.rePaths,
		"the skip must NOT increment the re-path counter (the budget bounds full re-plans)")
	require.Equal(t, 2, loop.wpIndex,
		"the first stuck must skip to the next waypoint")
	require.True(t, loop.stuckFast,
		"the first skip must arm the fast stuck timeout for subsequent stucks")
	// The skip sends a fresh walk click at the new waypoint target.
	require.Len(t, game.walks, 2,
		"the skip must send a fresh walk at the new waypoint")
	lastWalk := game.walks[len(game.walks)-1]
	require.Equal(t, int32(44600), lastWalk[0],
		"the fresh walk must target the skipped-to waypoint")
}

// TestWalkStuckRepathsAfterAllWaypointsSkipped verifies the re-path
// fallback: when all intermediate waypoints have been skipped and the
// character is still stuck at the final waypoint, the next stuck
// re-plans the whole leg from the current position to the destination.
// The re-plan (not the skip) consumes the re-path budget.
func TestWalkStuckRepathsAfterAllWaypointsSkipped(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.found = true
	nav.route = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3500},
		{X: 44800, Y: 50200, Z: -3500},
		{X: 44600, Y: 50400, Z: -3500},
	}
	fillInventory(bot)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, 1, loop.wpIndex)

	// First stuck: skip from mid (wp1) to end (wp2). No re-path budget
	// consumed (the skip is a cursor advance).
	armStuck(loop, bot)
	loop.tick()
	require.Zero(t, loop.rePaths,
		"the skip must NOT consume the re-path budget")
	require.Equal(t, 2, loop.wpIndex)

	// Second stuck: the last waypoint is the final one (no more to
	// skip), so the leg re-plans from the current position. The re-plan
	// consumes the re-path budget.
	armStuck(loop, bot)
	nav.calls = 0
	loop.tick()
	require.Equal(t, 1, loop.rePaths,
		"the re-plan must consume the re-path budget")
	require.Equal(t, 0, loop.wpIndex,
		"the re-plan must reset the follower cursor")
	require.False(t, loop.stuckFast,
		"the re-plan must clear the fast stuck flag")
	require.Greater(t, nav.calls, 0,
		"the re-plan must call the navigator")
}

// TestWalkStuckAbortsAfterMaxRePaths verifies the abort: after
// maxRePaths+1 stuck re-paths the trip aborts with the walk stuck
// reason. The skip does NOT consume the budget, so with a two-waypoint
// route (no intermediate waypoint to skip), each stuck re-plans the
// leg, and after maxRePaths re-paths the trip aborts.
func TestWalkStuckAbortsAfterMaxRePaths(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.found = true
	nav.route = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3500},
		{X: 44800, Y: 50200, Z: -3500},
	}
	fillInventory(bot)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)

	// Burn through the re-path budget: each stuck re-plans the leg
	// (with only 2 waypoints, the skip never fires), and after
	// maxRePaths attempts the trip aborts.
	for range maxRePaths {
		require.Equal(t, phaseTownWalk, loop.phase,
			"the trip must stay in the walk phase until the budget is exhausted")
		armStuck(loop, bot)
		loop.tick()
	}
	// After maxRePaths, one more stuck aborts the trip.
	armStuck(loop, bot)
	loop.tick()
	require.NotEqual(t, phaseTownWalk, loop.phase,
		"the trip must abort after the re-path budget is exhausted")
}

// TestWalkStuckFastTimeoutArmsAfterSkip pins the fast timeout: after the
// first skip, the stuck timer uses the shorter stuckFastTimeout so the
// walker cycles through the remaining waypoints quickly. The round 53
// dump showed the bot waiting 15 s per waypoint while the server refused
// every click - the fast timeout cuts that to 4 s after the first stuck.
func TestWalkStuckFastTimeoutArmsAfterSkip(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.found = true
	nav.route = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3500},
		{X: 44800, Y: 50200, Z: -3500},
		{X: 44600, Y: 50400, Z: -3500},
		{X: 44400, Y: 50600, Z: -3500},
	}
	fillInventory(bot)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, 1, loop.wpIndex)

	// First stuck: skip from wp1 to wp2. Arms the fast timeout.
	armStuck(loop, bot)
	loop.tick()
	require.True(t, loop.stuckFast, "the fast timeout must arm after the first skip")

	// A stuck that is past the fast timeout but NOT past the full
	// timeout must still fire the skip.
	selfX, selfY, _, _ := bot.SelfPosition()
	loop.stuckAt = time.Now().Add(-stuckFastTimeout - time.Second)
	loop.stuckX = selfX
	loop.stuckY = selfY
	loop.tick()
	require.Equal(t, 3, loop.wpIndex,
		"the fast timeout must fire the second skip before the full timeout")
}

// TestWalkStuckDoesNotSkipWaterEscape verifies the water escape branch
// is unchanged: a stuck water escape re-plans the escape itself, not
// the town leg. The skip logic only applies to the normal town walk.
func TestWalkStuckDoesNotSkipWaterEscape(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.found = true
	nav.overWater = true
	nav.route = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3700},
		{X: 44800, Y: 50200, Z: -3700},
	}
	nav.escapeRoute = []pathfind.Vec3{
		{X: 45000, Y: 50000, Z: -3700},
		{X: 45200, Y: 49800, Z: -3500},
	}
	fillInventory(bot)
	moveSelfTo(bot, 45000, 50000, -3700)
	loop.tick()
	require.True(t, loop.waterEscape,
		"the character over water must enter the water escape")

	// Stuck water escape: re-plan the escape, not skip a waypoint.
	armStuck(loop, bot)
	loop.tick()
	require.True(t, loop.waterEscape,
		"the water escape must stay active through the re-plan")
	require.Equal(t, 1, loop.rePaths)
}
