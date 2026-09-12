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

// TestWaypointPassedGeometry pins the projection pass test of the
// waypoint followers: only a character that moved past the waypoint ON
// the route towards the next one may skip it.
func TestWaypointPassedGeometry(t *testing.T) {
	entry := pathfind.Vec3{X: 0, Y: 0, Z: 0}
	across := pathfind.Vec3{X: 0, Y: 1000, Z: 0}

	tests := []struct {
		name   string
		selfX  int32
		selfY  int32
		passed bool
	}{
		// On the route, past the entry: the server correction
		// case - the skip must work.
		{"on the route past the waypoint", 30, 400, true},
		// Beside the route (the bridge railing side): the raw
		// distance to the far waypoint is smaller (412 against
		// 838), the projection must refuse the skip anyway.
		{"beside the route closer to the far end", 250, 800, false},
		// Before the waypoint: nothing passed yet.
		{"before the waypoint", 30, -400, false},
		// Past the waypoint but far off the corridor.
		{"past the waypoint off the corridor", 250, 400, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.passed,
				waypointPassed(entry, across, tt.selfX, tt.selfY))
		})
	}

	// A vertical drop segment degenerates in the plane: no lateral
	// pass logic, the arrival distance alone decides.
	drop := pathfind.Vec3{X: 0, Y: 0, Z: -900}
	require.False(t, waypointPassed(entry, drop, 0, 0),
		"a vertical segment never passes the character laterally")
}

// TestWaypointArrivedRadii pins the two arrival radii: the intermediate
// waypoints need the tight pass radius (a bridge ramp entry or a detour
// turn must be walked through), the final waypoint keeps the wide trip
// arrival radius.
func TestWaypointArrivedRadii(t *testing.T) {
	waypoints := []pathfind.Vec3{
		{X: 0, Y: 0, Z: 0},
		{X: 0, Y: 1000, Z: 0},
	}
	// 60 units from the intermediate waypoint: inside the legacy
	// 150 radius, outside the tight 50 - not arrived anymore.
	require.False(t, waypointArrived(waypoints, 0, 60, 0, 0, waypointArriveDist),
		"an intermediate waypoint must not count as reached at 60 units")
	require.True(t, waypointArrived(waypoints, 0, 40, 0, 0, waypointArriveDist),
		"an intermediate waypoint counts as reached at 40 units")
	// The final waypoint keeps the wide radius.
	require.True(t, waypointArrived(waypoints, 1, 0, 860, 0, waypointArriveDist),
		"the final waypoint counts as reached within 150 units")
	require.False(t, waypointArrived(waypoints, 1, 0, 840, 0, waypointArriveDist),
		"the final waypoint is not reached beyond 150 units")
}

// TestTripWalkKeepsBridgeEntryWaypoint reproduces the reported bridge
// stall: the plan holds the entry waypoint of the bridge and the far
// waypoint across the deck. A character standing at the RAILING SIDE of
// the bridge - closer to the far waypoint in raw distance - must keep
// targeting the entry it never walked through instead of cutting the
// corner into the railing (the legacy "the next waypoint is closer"
// skip walked it straight into the bridge side).
func TestTripWalkKeepsBridgeEntryWaypoint(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	entry := pathfind.Vec3{X: 0, Y: 0, Z: 0}
	across := pathfind.Vec3{X: 0, Y: 1000, Z: 0}
	loop.waypoints = []pathfind.Vec3{entry, across}
	loop.wpIndex = 0
	loop.phase = phaseTownWalk

	// The railing side: 250 units east of the bridge axis, past the
	// entry latitude. The raw distance to the far waypoint (412) is
	// smaller than to the entry (838).
	moveSelfTo(bot, 250, 800, 0)
	loop.moveAt = time.Time{}
	require.False(t, loop.walkTownWaypoints())
	require.Equal(t, 0, loop.wpIndex,
		"the entry waypoint must not be skipped from beside the route")
	require.Equal(t, [][3]int32{{0, 0, 0}}, game.walks,
		"the walk aims back at the bridge entry, not across the deck")

	// A character past the entry ON the route skips it: the walk
	// target moves to the far waypoint.
	moveSelfTo(bot, 30, 400, 0)
	loop.moveAt = time.Time{}
	game.walks = nil
	require.False(t, loop.walkTownWaypoints())
	require.Equal(t, 1, loop.wpIndex,
		"a character past the entry on the route skips it")
	require.Equal(t, [][3]int32{{0, 1000, 0}}, game.walks,
		"the walk aims at the far waypoint across the bridge")
}

// TestTripWalkRunsThroughTheEntryWaypoint pins the tight intermediate
// arrival: a character 60 units short of the bridge entry (inside the
// legacy 150 radius) still walks the remaining steps through the entry
// instead of accepting the detour turn from the side.
func TestTripWalkRunsThroughTheEntryWaypoint(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	entry := pathfind.Vec3{X: 0, Y: 0, Z: 0}
	across := pathfind.Vec3{X: 0, Y: 1000, Z: 0}
	loop.waypoints = []pathfind.Vec3{entry, across}
	loop.wpIndex = 0
	loop.phase = phaseTownWalk

	// 60 units short of the entry, on the approach line: the
	// follower keeps the entry targeted.
	moveSelfTo(bot, 60, 0, 0)
	loop.moveAt = time.Time{}
	require.False(t, loop.walkTownWaypoints())
	require.Equal(t, 0, loop.wpIndex,
		"60 units short of the entry is not an arrival anymore")
	require.Equal(t, [][3]int32{{0, 0, 0}}, game.walks,
		"the walk still runs through the entry waypoint")
}
