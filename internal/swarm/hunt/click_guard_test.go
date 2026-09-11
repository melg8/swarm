// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/stretchr/testify/require"
)

// The server click validation reactions of the waypoint follower: the
// 2026-09-10 town walk stuck report (the click to the second waypoint
// collapsed onto the walker through the village plaza corner) - the
// follower now gates every click through the validation port and
// reacts to a refusal with the leg shortening, the plan bend hop and
// the re-path instead of grinding ActionFailed answers into the stuck
// timeout.

// newClickGuardLoop builds the follower test bed: a loop with the
// fake navigator (a hook overrides its validation answers), the
// character on the route start and a three waypoint plan whose first
// bend sits close (the arrival radius swallows it) and the goal far.
func newClickGuardLoop(
	validate func(from, to pathfind.Vec3) (pathfind.Vec3, bool),
) (*Loop, *fakeGame, *fakeNavigator) {
	bot := newTestBot()
	// The character stands on the route start.
	moveSelfTo(bot, 1000, 1000, 0)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	nav := &fakeNavigator{found: true}
	nav.validateHook = validate
	loop.SetNavigator(nav)
	// A plan the follower walks: the start, a close bend the arrival
	// radius swallows and the far goal.
	loop.waypoints = []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1020, Y: 1000, Z: 0},
		{X: 1600, Y: 1000, Z: 0},
	}
	loop.wpIndex = 0
	loop.legDest = pathfind.Vec3{X: 1600, Y: 1000, Z: 0}
	loop.lastHit = time.Now().Add(-time.Minute)

	return loop, game, nav
}

// follow drives one follower pass with the character position.
func follow(loop *Loop) {
	loop.followWaypoints(1000, 1000, 0, time.Now(), false)
}

// TestFollowerSendsValidatedClick pins the default path: the
// validation accepts the click and the follower sends it unchanged.
func TestFollowerSendsValidatedClick(t *testing.T) {
	loop, game, nav := newClickGuardLoop(nil)
	follow(loop)
	require.GreaterOrEqual(t, len(game.walks), 1,
		"the validated click must be sent")
	require.Equal(t, int32(1600), game.walks[0][0],
		"the click targets the aimed waypoint")
	require.Equal(t, 2, loop.wpIndex,
		"the start and the close bend are swallowed by the arrival "+
			"radius, the aim stays the far goal")
	require.Zero(t, loop.rePaths, "no re-path fires")
	require.Zero(t, nav.calls, "no re-plan search runs")
}

// TestFollowerRefusedClickShortensLeg pins the shortening reaction:
// the far click refuses, the half leg validates and the follower
// sends the shortened target instead of the refused one.
func TestFollowerRefusedClickShortensLeg(t *testing.T) {
	loop, game, nav := newClickGuardLoop(
		func(_, to pathfind.Vec3) (pathfind.Vec3, bool) {
			// Refuse the full leg to the waypoint, accept the
			// shortened prefixes.
			if to.X >= 1500 {
				return to, false
			}

			return to, true
		})
	follow(loop)
	require.GreaterOrEqual(t, len(game.walks), 1,
		"the shortened click must be sent")
	sent := float64(game.walks[0][0])
	require.InDelta(t, 1300, sent, 1,
		"the first shortening halves the 600 unit leg to the far goal")
	require.Zero(t, loop.rePaths, "a shortened click needs no re-path")
	require.Zero(t, nav.calls, "no re-plan search runs")
}

// TestFollowerRefusedClickHopsToTheSwallowedBend pins the hop
// reaction: every prefix of the far aim refuses, the close bend the
// arrival radius swallowed validates and the follower hops onto it -
// the escape step out of a trap cell the re-path would plan but the
// cursor would skip.
func TestFollowerRefusedClickHopsToTheSwallowedBend(t *testing.T) {
	loop, game, nav := newClickGuardLoop(
		func(_, to pathfind.Vec3) (pathfind.Vec3, bool) {
			// Only the bend validates (the escape direction), every
			// prefix toward the far goal refuses.
			if to.X > 1021 {
				return to, false
			}

			return to, true
		})
	// The bend is 20 units away: inside the arrival radius but beyond
	// the coincide distance - the hop target.
	follow(loop)
	require.GreaterOrEqual(t, len(game.walks), 1,
		"the hop click must be sent")
	require.Equal(t, int32(1020), game.walks[0][0],
		"the hop targets the swallowed plan bend")
	require.Zero(t, loop.rePaths, "the hop replaces the re-path")
	require.Zero(t, nav.calls, "no re-plan search runs")
}

// TestFollowerRefusedClickRepathsAndAborts pins the terminal reaction:
// nothing validates (the trap has no local escape), the follower
// re-paths once - and the second refusal from the same cell aborts the
// trip without re-planning the identical route (the frozen re-path
// rule: a re-path that produced no movement proves the fresh plan
// cannot move the character either).
func TestFollowerRefusedClickRepathsAndAborts(t *testing.T) {
	loop, game, nav := newClickGuardLoop(
		func(from, _ pathfind.Vec3) (pathfind.Vec3, bool) {
			return from, false
		})
	loop.phase = phaseTownWalk
	// The first refusal re-paths within the budget.
	follow(loop)
	require.Equal(t, 1, nav.calls,
		"the first refusal re-paths once")
	require.Equal(t, 1, loop.rePaths)
	// The re-path refreshed the plan from the fake navigator:
	// restore the test route so the next pass aims the same refused
	// goal from the same cell.
	loop.waypoints = []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1020, Y: 1000, Z: 0},
		{X: 1600, Y: 1000, Z: 0},
	}
	loop.wpIndex = 0
	loop.moveAt = time.Time{}
	// The second refusal from the same cell aborts the trip instead of
	// re-planning the identical route.
	follow(loop)
	require.Equal(t, 1, nav.calls,
		"the frozen re-path aborts instead of re-planning")
	require.Equal(t, phaseEngage, loop.phase,
		"the frozen abort ends the trip back into the hunt")
	require.Empty(t, game.walks,
		"a refused click is never sent")
}
