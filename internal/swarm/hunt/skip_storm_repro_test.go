// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The reproduction of the 2026-09-21 town walk report (build a483578,
// bot test3, phase townWalk): the character froze at 31040 54016 -3415
// on the 87 waypoint walk to the trader Unoren and the stuck skip
// ladder marched the cursor from 15 to 63 - "town walk stuck,
// skipping waypoint" fired 49 times, one per fast stuck window, the
// aim ended 18,000 units out while the character never moved a unit,
// no ActionFailed answered and no re-path, refusal branch or escape
// ever ran (the dump: last action failed 11:59:40, the walk start;
// the position identical through the whole storm).
//
// The mechanism: the skip branch of stuckTownWalk preempts every
// other recovery rung as long as nextClearWaypoint finds a clear
// successor - and over open ground it always does (every far
// waypoint's clipped line validates). A skip that leaves the
// character frozen is not a recovery: repeating it only inflates the
// aim and starves the ladder that owns the dead click transport (the
// re-path budget, the frozen trip abort, the cursor key escape).
//
// The contract under test: a waypoint skip may run only while the
// previous skip moved the character; a repeat skip on the same cell
// falls through to the re-path ladder, whose frozen detector hands
// the walk to the cursor key escape - the claims transport that
// moves a character the clicks cannot.

// skipStormRoute is the synthetic open ground plan of the report:
// waypoints marching away from the frozen cell in ~700 unit legs
// (the dump plan marched 87 waypoints to 44584 46944, the aim ending
// 18,000 units from the standing cell).
var skipStormRoute = []pathfind.Vec3{
    {X: 31040, Y: 54016, Z: -3415},
    {X: 31740, Y: 53640, Z: -3490},
    {X: 32440, Y: 53260, Z: -3520},
    {X: 33140, Y: 52880, Z: -3560},
    {X: 33840, Y: 52500, Z: -3580},
    {X: 34540, Y: 52120, Z: -3600},
    {X: 35240, Y: 51740, Z: -3610},
    {X: 35940, Y: 51360, Z: -3620},
    {X: 36640, Y: 50980, Z: -3630},
    {X: 37340, Y: 50600, Z: -3640},
    {X: 38040, Y: 50220, Z: -3650},
    {X: 38740, Y: 49840, Z: -3660},
}

// The frozen scene constants: the dump position the character never
// left while the skip ladder ran.
const (
    skipStormX = int32(31040)
    skipStormY = int32(54016)
    skipStormZ = int32(-3415)
)

// TestFrozenSkipsYieldToTheRecoveryLadder replays the storm: every
// click validates (the open ground oracle of the report), the server
// accepts it and moves nothing (the deployed silence - no
// ActionFailed, no movement broadcast), and the move start watchdog
// forces a stuck verdict per dead click exactly like the live run.
// The first skip of the frozen episode may fire; the second one must
// be denied and hand the recovery to the re-path ladder, whose
// frozen detector arms the cursor key escape instead of skipping on.
func TestFrozenSkipsYieldToTheRecoveryLadder(t *testing.T) {
    bot := newTestBot()
    moveSelfTo(bot, skipStormX, skipStormY, skipStormZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    nav := &fakeNavigator{
        found: true,
        route: skipStormRoute,
    }
    // The open ground oracle: every line validates, the way
    // nextClearWaypoint found a clear successor 49 times in a row.
    nav.validateHook = func(
        _ pathfind.Vec3, to pathfind.Vec3,
    ) (pathfind.Vec3, bool) {
        return to, true
    }
    loop.SetNavigator(nav)
    dest := pathfind.Vec3{
        X: skipStormRoute[len(skipStormRoute)-1].X,
        Y: skipStormRoute[len(skipStormRoute)-1].Y,
        Z: skipStormRoute[len(skipStormRoute)-1].Z,
    }
    require.True(t, loop.startZoneReturnSegment(dest),
        "the storm plan must start")
    loop.phase = phaseTownReturn

    // Tick 1: the first click goes out.
    now := time.Now()
    _ = loop.followWaypoints(skipStormX, skipStormY, skipStormZ, now)
    require.False(t, loop.moveAt.IsZero(), "the first click went out")

    // Tick 2, past the move start window: the watchdog names the
    // click dead, the stuck verdict runs and the first skip fires
    // (the walled waypoint case keeps its cheap recovery).
    now = now.Add(moveStartWindow + time.Second)
    _ = loop.followWaypoints(skipStormX, skipStormY, skipStormZ, now)
    cursorAfterFirstSkip := loop.wpIndex
    require.Greater(t, cursorAfterFirstSkip, 1,
        "the first frozen skip advanced the cursor")

    // Tick 3, another dead click later: the character still stands on
    // the very cell the first skip left it on - the second skip must
    // be DENIED and the re-path ladder must own the segment now (the
    // replan resets the cursor, but it may not march past the frozen
    // skip's aim the way the old storm did).
    now = now.Add(moveStartWindow + time.Second)
    _ = loop.followWaypoints(skipStormX, skipStormY, skipStormZ, now)
    require.LessOrEqual(t, loop.wpIndex, cursorAfterFirstSkip,
        "a skip that moved the character nothing must not repeat - "+
            "the cursor may not march away while the character stands "+
            "still")
    require.Equal(t, 1, loop.rePaths,
        "the denied skip falls through to the re-path ladder")

    // Tick 4: the second identical re-path from the same frozen cell
    // proves the deterministic planner cannot move the character
    // either - the frozen trip abort escalates and the cursor key
    // escape (the claims transport) takes over the walk.
    now = now.Add(moveStartWindow + time.Second)
    _ = loop.followWaypoints(skipStormX, skipStormY, skipStormZ, now)
    require.True(t, loop.cursorEscape.armed,
        "the frozen segment must escalate to the cursor key escape, "+
            "the only transport that moves a character the clicks "+
            "cannot")
    require.Less(t, loop.wpIndex, len(skipStormRoute)-1,
        "the cursor stayed near the frozen cell instead of running "+
            "to the plan end")
}
