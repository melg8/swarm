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

// The priced water reality of the mesh round: the plan prices every
// crossing at the swim rate (the C1 zone data arms the pricing), so
// the follower walks the wet segments it planned - the swim is a
// slowdown, not a failure. The water escape machinery of the grid era
// retired with it (the mesh plans out of every standing cell, wet
// included - the navmesh world water reality tests pin the planning
// side on the real pack): an off-plan drift over a lake bed recovers
// through the plain re-plan from the wet standing cell, the same
// ladder every stuck walk uses.

// TestTripPlannedSwimKeepsFollowingThePlan pins the priced crossing:
// a character floating over a lake bed whose aimed waypoint stands on
// the bed ahead (the crossing the search priced) keeps following the
// plan - no escape arms, the click walks the wet waypoint.
func TestTripPlannedSwimKeepsFollowingThePlan(t *testing.T) {
    loop, game, _, nav := newTripLoop()
    nav.overWater = true
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.segmentDest = pathfind.Vec3{X: 45200, Y: 50200, Z: -3539}
    loop.segmentStart = pathfind.Vec3{X: 45000, Y: 50000, Z: -3800}
    // The plan crosses the lake: the aimed waypoint stands on the bed
    // ahead of the character.
    loop.waypoints = []pathfind.Vec3{
        {X: 44900, Y: 49900, Z: -3539},
        {X: 45050, Y: 50050, Z: -3850},
        {X: 45200, Y: 50200, Z: -3539},
    }
    loop.wpIndex = 1

    require.False(t, loop.walkTownWaypoints(),
        "the planned swim keeps walking")
    require.Zero(t, nav.calls,
        "no shore search runs for a priced crossing")
    require.Len(t, game.walks, 1,
        "the walk click goes to the wet waypoint")
    require.Equal(t, [3]int32{45050, 50050, -3850}, game.walks[0],
        "the click aims the bed waypoint the plan prices")
}

// TestTripWetStandingReplansTheWalk pins the off-plan drift recovery:
// a character the water drifted off its plan (the server push, the
// click drift) recovers through the plain stuck ladder - the walk
// re-plans from the wet standing cell and the fresh plan owns the
// segment. No shore escape exists to burn the trip into an abort.
func TestTripWetStandingReplansTheWalk(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    nav.found = true
    nav.overWater = true
    nav.blind = true
    nav.route = []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3800},
        {X: 45200, Y: 50200, Z: -3700},
    }
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.segmentDest = pathfind.Vec3{X: 45200, Y: 50200, Z: -3700}
    loop.segmentStart = pathfind.Vec3{X: 45000, Y: 50000, Z: -3800}
    loop.waypoints = []pathfind.Vec3{
        {X: 44900, Y: 49900, Z: -3800},
        {X: 45200, Y: 50200, Z: -3700},
    }
    loop.wpIndex = 0
    fillInventory(bot)
    moveSelfTo(bot, 45000, 50000, -3800)

    // The character stands still past the stuck window: the recovery
    // re-plans the segment from the wet standing cell.
    armStuck(loop, bot)
    loop.tick()
    require.Equal(t, phaseTownWalk, loop.phase,
        "the wet standing never aborts the trip by itself")
    require.Equal(t, 1, loop.rePaths,
        "the re-plan consumed one budget step")
    require.Equal(t, []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3800},
        {X: 45200, Y: 50200, Z: -3700},
    }, loop.waypoints,
        "the fresh plan replaces the segment from the wet cell")
}
