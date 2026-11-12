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

// The continuous flow pins (see flow.go): the follower clicks the
// farthest route waypoint the server transport delivers inside the
// aim span, keeps the bends and the final point of the plan, and
// never probes the route from a cell a previous jump left frozen.

// flowStraightRoute is a dense straight plan ahead of the test farm
// spot: the route points sit 120 units apart at the standing height,
// so the default 480 unit aim (120 run speed, two request periods)
// reaches the fourth waypoint.
func flowStraightRoute(count int) []pathfind.Vec3 {
    route := make([]pathfind.Vec3, 0, count)
    for i := 1; i <= count; i++ {
        route = append(route, pathfind.Vec3{
            X: 45000 + float64(120*i),
            Y: 50000,
            Z: -3500,
        })
    }

    return route
}

// TestFlowAimSelectsTheFarthestRouteWaypoint pins the span of one
// click: the cursor jumps to the farthest plan point inside the aim,
// not to the dense point right ahead, so the character keeps running
// instead of stopping at every reached waypoint.
func TestFlowAimSelectsTheFarthestRouteWaypoint(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.waypoints = flowStraightRoute(5)

    loop.wpIndex = loop.flowAdvance(loop.waypoints, 0, 0,
        45000, 50000, -3500)

    require.Equal(t, 3, loop.wpIndex,
        "the 480 unit aim reaches the fourth route point")
    require.NotEmpty(t, nav.validatedClicks,
        "the far target answered through the server click port")
    require.InDelta(t, 45480.0, nav.validatedClicks[0].X, 0.001,
        "the farthest candidate is the one the transport delivered")

    loop.clickWaypoint(45000, 50000, -3500, time.Now())
    require.Equal(t, [3]int32{45480, 50000, -3500}, game.walks[0],
        "the click aims the far waypoint inside the aim span")
}

// TestFlowAimFallsBackToTheFarthestValidatedWaypoint pins the
// validation order: a refused farthest candidate must not collapse
// the flow to the nearest point - the next farthest valid target
// takes the click.
func TestFlowAimFallsBackToTheFarthestValidatedWaypoint(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.waypoints = flowStraightRoute(5)
    nav.validateHook = func(_, to pathfind.Vec3) (pathfind.Vec3, bool) {
        if to.X > 45400 {
            return pathfind.Vec3{}, false
        }

        return to, true
    }

    loop.wpIndex = loop.flowAdvance(loop.waypoints, 0, 0,
        45000, 50000, -3500)
    require.Equal(t, 2, loop.wpIndex,
        "the farthest target the server delivers wins")

    loop.clickWaypoint(45000, 50000, -3500, time.Now())
    require.Equal(t, [3]int32{45360, 50000, -3500}, game.walks[0],
        "the click lands on the validated target")
}

// TestFlowKeepsTheSwitchback pins the route fidelity guard: the
// straight line from the character to the last route point runs far
// outside the drawn corridor across the bend, so the flow advances
// only to the first leg and the plan's turn survives.
func TestFlowKeepsTheSwitchback(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.waypoints = []pathfind.Vec3{
        {X: 45120, Y: 50000, Z: -3500},
        {X: 45240, Y: 50000, Z: -3500},
        {X: 45240, Y: 50120, Z: -3500},
        {X: 45120, Y: 50120, Z: -3500},
    }

    loop.wpIndex = loop.flowAdvance(loop.waypoints, 0, 0,
        45000, 50000, -3500)

    require.Equal(t, 1, loop.wpIndex,
        "the switchback point stays the cursor")
    loop.clickWaypoint(45000, 50000, -3500, time.Now())
    require.Equal(t, [3]int32{45240, 50000, -3500}, game.walks[0],
        "the click stays on the first leg of the turn")
}

// TestFlowKeepsTheFinalWaypointAsTheTarget pins the end of the plan:
// the scan never advances past the last waypoint, and a plan point
// that is the only remaining target is clicked itself.
func TestFlowKeepsTheFinalWaypointAsTheTarget(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.waypoints = flowStraightRoute(2)

    loop.wpIndex = loop.flowAdvance(loop.waypoints, 0, 0,
        45000, 50000, -3500)
    require.Equal(t, 1, loop.wpIndex,
        "the final waypoint is the farthest legal cursor")
    loop.clickWaypoint(45000, 50000, -3500, time.Now())
    require.Equal(t, [3]int32{45240, 50000, -3500}, game.walks[0])

    require.Equal(t, 0, loop.flowAdvance(flowStraightRoute(1), 0, 0,
        45000, 50000, -3500), "a single point plan stays put")
}

// TestFlowDoesNotProbeFromAFrozenSkipCell pins the dead transport
// guard: the previous cursor jump left the character on this cell, so
// the scan returns before the validation probes instead of spending
// them on a transport that cannot move the character.
func TestFlowDoesNotProbeFromAFrozenSkipCell(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.waypoints = flowStraightRoute(5)
    loop.skipArmed = true
    loop.skipX, loop.skipY = 45000, 50000

    require.Equal(t, 0, loop.flowAdvance(loop.waypoints, 0, 0,
        45000, 50000, -3500))
    require.Empty(t, nav.validatedClicks,
        "the frozen cell short-circuits before the probes")
}

// TestFlowSkipsTheScanWhenTheCursorIsBeyondTheAim pins the cheap
// steady state: a current target already past the aim needs no scan,
// the next request period re-aims the same point.
func TestFlowSkipsTheScanWhenTheCursorIsBeyondTheAim(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.waypoints = []pathfind.Vec3{
        {X: 45600, Y: 50000, Z: -3500},
        {X: 45720, Y: 50000, Z: -3500},
    }

    require.Equal(t, 0, loop.flowAdvance(loop.waypoints, 0, 0,
        45000, 50000, -3500))
    require.Empty(t, nav.validatedClicks,
        "the scan runs only when the cursor sits inside the aim")
}

// TestManualWalkReAimsWhileWalking pins the manual follower: the
// running move does not stand the click down any more - every request
// period re-aims the farthest delivered waypoint, so the manually
// ordered walk crosses its dense plan points without an arrival stop.
func TestManualWalkReAimsWhileWalking(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45200, DestY: 50000, DestZ: -3500,
    })
    require.True(t, bot.SelfWalking())
    now := time.Now()
    loop.userWaypoints = []pathfind.Vec3{
        {X: 45200, Y: 50000, Z: -3500},
        {X: 45350, Y: 50000, Z: -3500},
    }
    loop.userWpIndex = 0
    loop.userStart = now
    loop.userMoveAt = now.Add(-walkRequestPeriod)

    loop.followUserWaypoints(now, 45000, 50000, -3500)

    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{45350, 50000, -3500}, game.walks[0],
        "the running walk re-aims past the near waypoint")
}

// TestFlowPreClickReplacesTheDestinationMidWalk pins the corner
// pre-click: within the lead of the current target the follower sends
// a farther validated click at once, so the server replaces the
// destination before the arrival stops the character.
func TestFlowPreClickReplacesTheDestinationMidWalk(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45050, DestY: 50000, DestZ: -3500,
    })
    require.True(t, bot.SelfWalking())
    loop.waypoints = []pathfind.Vec3{
        {X: 45050, Y: 50000, Z: -3500},
        {X: 45350, Y: 50000, Z: -3500},
    }
    now := time.Now()
    loop.moveAt = now.Add(-flowPreClickGap / 2)

    require.False(t, loop.flowPreClick(45000, 50000, -3500, now),
        "the pre-click keeps its own pace")
    require.Empty(t, game.walks)

    loop.moveAt = now.Add(-flowPreClickGap)
    require.True(t, loop.flowPreClick(45000, 50000, -3500, now))
    require.Equal(t, [3]int32{45350, 50000, -3500}, game.walks[0],
        "the pre-click re-aims the running move to the far waypoint")
}

// TestFlowPreClickStaysOffWhenStanding pins the dead click handover: a
// standing character means the last click never moved it, and the
// stuck ladder owns that cell - the pre-click must not paper over it
// with another request.
func TestFlowPreClickStaysOffWhenStanding(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    require.False(t, bot.SelfWalking())
    loop.waypoints = []pathfind.Vec3{
        {X: 45050, Y: 50000, Z: -3500},
        {X: 45350, Y: 50000, Z: -3500},
    }
    now := time.Now()
    loop.moveAt = now.Add(-walkRequestPeriod)

    require.False(t, loop.flowPreClick(45000, 50000, -3500, now))
    require.Empty(t, game.walks)
}

// TestFlowPreClickPacesTheFailedScan pins the probe budget: a scan
// that found no far target still counts, so the ring is not probed
// again on the next tick of the same second.
func TestFlowPreClickPacesTheFailedScan(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 45000, 50000, -3500)
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45050, DestY: 50000, DestZ: -3500,
    })
    loop.waypoints = []pathfind.Vec3{
        {X: 45050, Y: 50000, Z: -3500},
        {X: 45600, Y: 50000, Z: -3500},
    }
    now := time.Now()
    loop.moveAt = now.Add(-walkRequestPeriod)

    require.False(t, loop.flowPreClick(45000, 50000, -3500, now),
        "the far point sits past the aim")
    require.False(t, loop.flowScanAt.IsZero(),
        "the failed scan is paced like a sent pre-click")
    require.False(t, loop.flowPreClick(45000, 50000, -3500,
        now.Add(flowPreClickGap/2)),
        "the next tick of the same second does not probe")
    require.Empty(t, game.walks)
}
