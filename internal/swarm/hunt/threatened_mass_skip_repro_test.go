// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The reproduction of the 2026-09-20 town walk report (build 4805d9a,
// bot test3, phase townWalk): the state dump showed the waypoints 8
// through 23 of the 42 waypoint plan passed with legs of 0.0 to 0.7
// seconds while the character stood still - the events name the writer:
// "the waypoint sits inside an aggro circle, skipping ahead" fired
// fifteen times in three seconds (cursors 9 through 23) and the stuck
// handler then jumped onto wp 24, 8800 units away, walking the rest of
// the trip cross-country off the planned route.
//
// The scene numbers are the dump's own: the idle aggressive Kaboo Orc
// Fighter 268439358 camped at 37920 44989 (the trigger circle runs at
// the MaxAggroRange clamp 450, the steering margin lifts the verdict
// radius to 600), the character standing at 37540 45992 (the dump
// position 39086 45220 walked back along the wp 24 bearing by the
// 12 seconds of its last segment), the plan waypoints 8 through 13 of
// the dump walk to the trader Unoren (the segment dest 44667 46896).
//
// The bug: the threatened verdict of the skip read the ISSUED CLICK
// TARGET - a far waypoint's click rides the maxMoveDistance (1000)
// clip next to the character - so the camp between the character and
// the horizon condemned every far waypoint along the direction: the
// clipped targets of the dump waypoints 11 through 22 all land 319 to
// 528 units from the camp (inside the 600 margin) while the waypoints
// themselves sit 1164 to 4729 units out (far outside it). The skip
// must read the waypoint it would abandon.

// TestThreatenedVerdictReadsTheWaypointNotTheClick pins the geometry
// split of the report: the clipped click target of the far waypoint
// lands inside the camp circle while the waypoint itself stays far
// outside it - exactly one of the two may drive the skip.
func TestThreatenedVerdictReadsTheWaypointNotTheClick(t *testing.T) {
    bot := avoidSceneBot()
    // The dump camp: the idle aggressive fighter at 37920 44989.
    avoidCampMob(bot, 37920, 44989)
    // The threat scan runs around the character: park it where the
    // report's character stood when the skip chain ran.
    moveSelfTo(bot, 37540, 45992, -3500)
    loop := NewLoop(&fakeGame{}, bot)

    // The dump wp 11 (39040 44672) sits 1164 units from the camp -
    // far outside the 600 unit margin circle: no skip may ever fire
    // for it from this scene.
    require.False(t, loop.segmentTargetThreatened(39040, 44672, 44667, 46896),
        "the far waypoint itself is clean - the skip must read this")

    // The click the follower issues for the same waypoint rides the
    // maxMoveDistance clip: it parks 1000 units from the standing
    // cell, 503 units from the camp - inside the margin circle. The
    // old code fed THIS point to the threatened verdict and skipped
    // the waypoint it never examined.
    click := segmentWalkTarget([3]int32{37540, 45992, -3500},
        pathfind.Vec3{X: 39040, Y: 44672, Z: -3632})
    clearance := math.Hypot(float64(click[0]-37920),
        float64(click[1]-44989))
    require.Less(t, clearance, 600.0,
        "the scene holds: the clipped click target sits inside the circle")
    require.True(t, loop.segmentTargetThreatened(click[0], click[1],
        44667, 46896),
        "the clipped click target is threatened - the very point the "+
            "old verdict read instead of the waypoint")

    // The dump waypoints 8 through 10 sit genuinely inside the circle
    // (587, 393 and 358 units from the camp): the skip is correct for
    // exactly these.
    require.True(t, loop.segmentTargetThreatened(38016, 45568, 44667, 46896),
        "wp 8 of the dump is 587 units from the camp")
    require.True(t, loop.segmentTargetThreatened(38144, 45312, 44667, 46896),
        "wp 9 of the dump is 393 units from the camp")
    require.True(t, loop.segmentTargetThreatened(38272, 45056, 44667, 46896),
        "wp 10 of the dump is 358 units from the camp")
}

// TestThreatenedSkipStopsAtTheFirstClearWaypoint pins the follower
// behavior end to end on the dump scene: the cursor skips the three
// waypoints the camp circle genuinely holds and STOPS on the first
// clear one - the walk continues along the plan (the steering arcs the
// clicks around the camp), the far waypoints stay ahead of the cursor
// and the published plan never marks them passed while the character
// stands still.
func TestThreatenedSkipStopsAtTheFirstClearWaypoint(t *testing.T) {
    bot := avoidSceneBot()
    // The dump camp beside the route the plan shares with the report.
    avoidCampMob(bot, 37920, 44989)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.phase = phaseTownWalk
    // The dump segment destination: the trader Unoren in town.
    loop.segmentDest = pathfind.Vec3{X: 44667, Y: 46896, Z: -2982}
    // The dump plan waypoints 8 through 13; the character stands
    // where the report's character stood when the skip chain ran.
    loop.waypoints = []pathfind.Vec3{
        {X: 38016, Y: 45568, Z: -3624},
        {X: 38144, Y: 45312, Z: -3624},
        {X: 38272, Y: 45056, Z: -3616},
        {X: 39040, Y: 44672, Z: -3632},
        {X: 39296, Y: 44416, Z: -3688},
        {X: 39424, Y: 44048, Z: -3760},
    }
    loop.wpIndex = 0
    moveSelfTo(bot, 37540, 45992, -3500)

    // Drive the follower past every skip decision - far more calls
    // than the chain needs, so the old code runs out of plan here.
    for range 8 {
        loop.walkTownWaypoints()
    }

    // The cursor rests on wp 11 (the index 3): the first waypoint
    // outside the camp circle. The old code chained the skips onto
    // the far waypoints (the cursor ended on the LAST waypoint of
    // the plan, the whole middle marked passed) and walked the rest
    // cross-country.
    require.Equal(t, 3, loop.wpIndex,
        "the skip chain stops on the first waypoint outside the circle")
    require.Len(t, game.walks, 1,
        "exactly the clear waypoint's click goes out")
    walk := game.walks[0]
    clearance := math.Hypot(float64(walk[0]-37920), float64(walk[1]-44989))
    require.GreaterOrEqual(t, clearance, 600.0,
        "the issued click steers clear of the camp circle")

    // The published plan reads honestly: the cursor names wp 11, the
    // far waypoints 12 and 13 stay ahead of it - nothing marked
    // passed that the character never reached.
    loop.publishWalkPlan()
    snap := bot.Snapshot()
    require.Equal(t, 3, snap.WalkIndex,
        "the published plan keeps the far waypoints ahead of the cursor")
    require.Len(t, snap.WalkPath, 6)
}
