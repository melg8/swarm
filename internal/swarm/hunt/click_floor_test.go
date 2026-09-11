// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/stretchr/testify/require"
)

// The short click extension of the waypoint follower: the 2026-09-11
// 11:34 village return dump froze the character on the 22 unit first
// waypoint of an otherwise valid plan - the server's own move
// validation collapsed the click and Creature.moveToLocation only
// hands a collapsed click to the server side pathfinder when the
// original line was longer than 30 units, so the short click was
// silently canceled. After the first stuck the follower arms the
// extension and its short or backward clicks re-aim at the forward
// route samples past the rescue floor.

// TestExtendShortClickCandidatesMarchForward pins the march itself:
// the candidates walk the forward route from the aimed waypoint, skip
// the samples under the floor or behind the character and stop at the
// candidate cap.
func TestExtendShortClickCandidatesMarchForward(t *testing.T) {
	waypoints := []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1020, Y: 1000, Z: 0},
		{X: 1020, Y: 1200, Z: 0},
	}
	// The character stands between the first bend and the far leg.
	candidates, count := extendShortClickCandidates(1010, 1100, waypoints, 1)
	require.Positive(t, count, "the march must collect the far samples")
	for c := 0; c < count; c++ {
		sample := candidates[c]
		// The march walks the segment from wp1 (1020 1000) toward
		// wp2 (1020 1200): every collected sample must sit past the
		// character toward wp2 and clear the rescue floor.
		require.Greater(t, sample.Y, float64(1100),
			"no sample behind the character")
		chord := math.Hypot(sample.X-1010, sample.Y-1100)
		require.GreaterOrEqual(t, chord, minWalkClick,
			"every collected sample clears the rescue floor")
	}
	require.LessOrEqual(t, count, extendCandidateMax,
		"the march stops at the candidate cap")
	// The last waypoint aim collects nothing (no forward segment).
	_, count = extendShortClickCandidates(1010, 1100, waypoints, 2)
	require.Zero(t, count,
		"the arrival aim keeps its plain waypoint click")
}

// TestWaypointBehindRoute pins the geometry: a waypoint counts as
// behind when the character already moved past it along the segment
// leaving it - the pinned cursor of a passed waypoint.
func TestWaypointBehindRoute(t *testing.T) {
	waypoints := []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1100, Y: 1000, Z: 0},
		{X: 1200, Y: 1000, Z: 0},
	}
	require.False(t, waypointBehindRoute(waypoints, 1, 1050, 1000),
		"a waypoint ahead of the character is not behind")
	require.True(t, waypointBehindRoute(waypoints, 1, 1150, 1000),
		"a waypoint the character passed is behind")
	require.False(t, waypointBehindRoute(waypoints, 2, 1150, 1000),
		"the last waypoint has no forward segment")
}

// TestFollowerExtendsShortClickAfterStuck pins the armed behavior:
// after a stuck re-path (extendArmed), the click to a short waypoint
// re-aims at the forward route sample past the rescue floor - the
// server cannot silently cancel it.
func TestFollowerExtendsShortClickAfterStuck(t *testing.T) {
	loop, game, nav := newClickGuardLoop(
		func(from, to pathfind.Vec3) (pathfind.Vec3, bool) {
			// Only the route samples past the floor validate.
			if math.Hypot(to.X-from.X, to.Y-from.Y) < minWalkClick {
				return from, false
			}

			return to, true
		})
	// The plan aims a SHORT second waypoint with the far goal behind
	// it: the cursor pins on the short one (the far line refuses).
	loop.waypoints = []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1020, Y: 1000, Z: 0},
		{X: 1600, Y: 1000, Z: 0},
	}
	loop.wpIndex = 1
	loop.legDest = pathfind.Vec3{X: 1600, Y: 1000, Z: 0}
	loop.phase = phaseTownWalk
	loop.extendArmed = true

	follow(loop)

	require.NotEmpty(t, game.walks,
		"the armed follower must send the extended click")
	sent := game.walks[len(game.walks)-1]
	length := math.Hypot(float64(sent[0]-1000), float64(sent[1]-1000))
	require.GreaterOrEqual(t, length, minWalkClick,
		"the armed click rides over the server rescue floor")
	require.Zero(t, nav.calls, "no re-plan runs for the extended click")
	require.Zero(t, loop.rePaths, "no re-path budget burns")
}

// TestFollowerKeepsShortClickBeforeStuck pins the unarmed behavior:
// without a stuck (the normal tight ramp climb of the teacher legs),
// the short waypoint click stands as is - the extension never fires
// on a walking plan.
func TestFollowerKeepsShortClickBeforeStuck(t *testing.T) {
	loop, game, nav := newClickGuardLoop(nil)
	// The successor line stays blocked so the cursor pins on the short
	// waypoint (the teacher ramp shape).
	nav.sightFunc = func(_, _ pathfind.Vec3) (bool, error) {
		return false, nil
	}
	loop.waypoints = []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1020, Y: 1000, Z: 0},
		{X: 1600, Y: 1000, Z: 0},
	}
	loop.wpIndex = 1
	loop.legDest = pathfind.Vec3{X: 1600, Y: 1000, Z: 0}
	loop.phase = phaseTownWalk
	require.False(t, loop.extendArmed)

	follow(loop)

	require.NotEmpty(t, game.walks,
		"the unarmed follower keeps clicking the waypoint")
	require.Equal(t, int32(1020), game.walks[len(game.walks)-1][0],
		"the short waypoint click stands as is before any stuck")
}

// TestFollowerHoldsBackwardClickWhenArmed pins the hold: an armed
// follower whose waypoint sits behind the character (the pinned cursor
// of a passed waypoint) and whose route offers no validating forward
// sample sends nothing - the backward click would walk the character
// off the ground the extension just walked.
func TestFollowerHoldsBackwardClickWhenArmed(t *testing.T) {
	loop, game, _ := newClickGuardLoop(
		func(from, _ pathfind.Vec3) (pathfind.Vec3, bool) {
			return from, false
		})
	// The character already passed wp1: it sits west while the route
	// continues east.
	loop.waypoints = []pathfind.Vec3{
		{X: 1000, Y: 1000, Z: 0},
		{X: 1020, Y: 1000, Z: 0},
		{X: 1600, Y: 1000, Z: 0},
	}
	loop.wpIndex = 1
	loop.legDest = pathfind.Vec3{X: 1600, Y: 1000, Z: 0}
	loop.phase = phaseTownWalk
	loop.extendArmed = true
	// The character stands well past wp1 on the route.
	bot := newTestBot()
	moveSelfTo(bot, 1200, 1000, 0)
	loop.tracker = bot

	loop.followWaypoints(1200, 1000, 0, time.Now(), false)

	require.Empty(t, game.walks,
		"the armed follower holds the backward click")
}
