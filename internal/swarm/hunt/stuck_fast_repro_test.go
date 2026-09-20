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

// The reproduction of the 2026-09-14 08:13 state dump (build c7a0855,
// bot unittest3, phase townReturn, uptime 19s): the level 15 character
// stood at x 45768 y 49848 z -3056 (the Elven Village south terrace
// deck) in the townReturn phase, the pathfound zone return held the
// 6 waypoint route to the Kaboo Orc Fighter SW-7 cell (dest 36000
// 46765 -3712, on the field deck), the first waypoint sat at 43512
// 50504 -2992 (the village plaza corner). The hunt log carried
// "outside the hunting zone, pathfinding back" then a single
// "town walk stuck, re-pathing (1 of 3)" line at 16s of uptime -
// the click to wp1 was validated against the offline click port
// (so it passed clickServerValidated), but the server silently
// canceled the move (the geodata correction collapsed it or the
// server-side pathfinder refused the route), the character never
// moved a cell, the stuck timeout fired at 15s, the re-path planned
// the identical route from the same cell, and the next detection
// waited the FULL 15s stuckTimeout again - the fast stuck window
// (stuckFastTimeout = 4s) never armed on the re-path branch. The
// bot sat at the same cell through 30+ seconds of recovery for a
// freeze the fast window would have caught in 4s.
//
// Root cause: stuckTownWalk arms stuckFast = true on the WAYPOINT
// SKIP branch (nextClearWaypoint finds a clear successor) but NOT
// on the RE-PATH branch (no clear successor, the segment re-plans).
// The re-path proved the plain clicks of this segment do not move the
// character - the same evidence the waypoint skip carries - so the
// fast window belongs there too. The fix arms stuckFast and
// re-baselines the stuck window from the re-path tick on the
// re-path branch, the same way the skip branch does.

const (
    // reproFastStuckX/Y/Z is the reported stuck position (the dump
    // of 2026-09-14 08:13:08, the character unittest3 on the Elven
    // Village south terrace deck).
    reproFastStuckX = int32(45768)
    reproFastStuckY = int32(49848)
    reproFastStuckZ = int32(-3056)
    // reproFastStuckZoneX/Y/Z is the hunting zone center of the
    // report (the Kaboo Orc Fighter SW-7 cell of the dump).
    reproFastStuckZoneX = int32(36000)
    reproFastStuckZoneY = int32(46765)
    reproFastStuckZoneZ = int32(-3712)
)

// railingPortAimed models the village railing of the 2026-09-14 08:13
// dump through the click transport port: the line toward the AIMED
// waypoint validates (the capped click target and its on-line
// prefixes - the dump's click went out and the server silently
// swallowed the move), every line off the aimed line refuses offline
// (the skip and the forward jump targets over the railing) - the
// stuck fires the re-path branch, never the waypoint skip. The hook
// reads the live cursor, so it arms after the loop and its plan
// exist.
func railingPortAimed(
    loop *Loop,
) func(from, to pathfind.Vec3) (pathfind.Vec3, bool) {
    return func(from, to pathfind.Vec3) (pathfind.Vec3, bool) {
        if loop.wpIndex >= len(loop.waypoints) {
            return to, true
        }
        aimed := loop.waypoints[loop.wpIndex]
        ax, ay := aimed.X-from.X, aimed.Y-from.Y
        alen := math.Hypot(ax, ay)
        if alen < 1 {
            return to, true
        }
        tx, ty := to.X-from.X, to.Y-from.Y
        cross := math.Abs(tx*ay-ty*ax) / alen
        along := (tx*ax + ty*ay) / alen
        if cross <= 1 && along >= -1 && along <= maxMoveDistance+1 {
            return to, true
        }

        return from, false
    }
}

// TestStuckRepathArmsFastWindow pins the fix: after a stuck re-path
// that did not move the character, the fast stuck window arms, so
// the next stuck detection fires on stuckFastTimeout (4s) instead of
// the full stuckTimeout (15s). The dump of 2026-09-14 08:13 showed
// the bot sitting through the full 15s window after every re-path -
// the fast window never armed on the re-path branch.
func TestStuckRepathArmsFastWindow(t *testing.T) {
    bot := newTestBot()
    moveSelfTo(bot, reproFastStuckX, reproFastStuckY, reproFastStuckZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    // The click validation passes (the dump's click was accepted by
    // the offline click port - the server silently canceled the move
    // after the validation). The route is the dump's 6 waypoint plan.
    nav := &fakeNavigator{
        found: true,
        route: []pathfind.Vec3{
            {X: 43512, Y: 50504, Z: -2992},
            {X: 42664, Y: 51336, Z: -2992},
            {X: 40648, Y: 52680, Z: -3224},
            {X: 36168, Y: 47368, Z: -3664},
            {X: 36008, Y: 46952, Z: -3704},
            {X: 36000, Y: 46765, Z: -3712},
        },
    }
    loop.SetNavigator(nav)
    // The click transport refuses every line off the aimed waypoint's
    // line: the village railing walls the skip and the jump targets,
    // so nextClearWaypoint returns the pinned cursor and the stuck
    // fires the re-path branch (not the waypoint skip).
    nav.validateHook = railingPortAimed(loop)
    dest := pathfind.Vec3{
        X: float64(reproFastStuckZoneX),
        Y: float64(reproFastStuckZoneY),
        Z: float64(reproFastStuckZoneZ),
    }
    require.True(t, loop.startZoneReturnSegment(dest),
        "the zone return must plan the 6 waypoint route of the dump")
    require.Len(t, loop.waypoints, 6,
        "the plan matches the dump's 6 waypoint route")
    loop.phase = phaseTownReturn

    // The first followWaypoints tick: the click to wp1 passes the
    // offline validation (the click is sent), but the server
    // silently cancels the move - the character never moves. The
    // stuck window opens from this tick.
    now := time.Now()
    _, _, selfZ, _ := bot.SelfPosition()
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.False(t, loop.stuckFast,
        "the first click did not arm the fast stuck window")
    require.False(t, loop.stuckAt.IsZero(),
        "the stuck window opened from the first click")

    // Advance past the full stuckTimeout: the first stuck detection
    // fires the re-path branch of stuckTownWalk (no clear successor
    // from the same cell - nextClearWaypoint returns the pinned
    // cursor because every forward waypoint's line stays blocked by
    // the same village railing). The re-path re-plans the identical
    // route.
    now = now.Add(stuckTimeout + time.Second)
    loop.moveAt = time.Time{}
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.Equal(t, 1, loop.rePaths,
        "the first stuck fired the re-path (1 of 3)")
    // The fix: the re-path arm sets stuckFast, the same way the
    // waypoint skip arm does.
    require.True(t, loop.stuckFast,
        "the re-path arm sets the fast stuck window - the dump's "+
            "recovery burned 15s of the trip budget on the next "+
            "detection the fast window would have caught in 4s")

    // The next stuck detection now fires on stuckFastTimeout (4s)
    // instead of the full stuckTimeout (15s). The dump of
    // 2026-09-14 08:13 waited the full 15s after every re-path.
    fastAt := now
    now = now.Add(stuckFastTimeout + time.Second)
    loop.moveAt = time.Time{}
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    // The detection fired on the fast window, 4s after the first
    // re-path, NOT 15s.
    detectionGap := now.Sub(fastAt)
    require.LessOrEqual(t, detectionGap, stuckFastTimeout+2*time.Second,
        "the second stuck detection fired on the fast window (4s), "+
            "not the full stuckTimeout (15s)")
}

// TestStuckSkipWaypointStillArmsFastWindow is the regression guard:
// the waypoint skip branch of stuckTownWalk already armed stuckFast
// before the fix - the fix must not break it.
func TestStuckSkipWaypointStillArmsFastWindow(t *testing.T) {
    bot := newTestBot()
    moveSelfTo(bot, reproFastStuckX, reproFastStuckY, reproFastStuckZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    // The route holds two waypoints: wp1 sits behind a wall (the
    // click refuses), wp2 sits on a clear line (the skip jumps the
    // cursor onto it).
    loop.SetNavigator(&fakeNavigator{
        found: true,
        route: []pathfind.Vec3{
            {X: 43512, Y: 50504, Z: -2992},
            {X: 42664, Y: 51336, Z: -2992},
            {X: 36000, Y: 46765, Z: -3712},
        },
        validateHook: func(_, to pathfind.Vec3) (pathfind.Vec3, bool) {
            // The line to wp1 is blocked (the wall), the line to
            // wp2 is clear (the skip target).
            if int32(to.X) == 43512 && int32(to.Y) == 50504 {
                return to, false
            }

            return to, true
        },
    })
    dest := pathfind.Vec3{
        X: float64(reproFastStuckZoneX),
        Y: float64(reproFastStuckZoneY),
        Z: float64(reproFastStuckZoneZ),
    }
    require.True(t, loop.startZoneReturnSegment(dest))
    require.Len(t, loop.waypoints, 3)

    // The first followWaypoints tick: the click to wp1 is refused
    // (the click transport blocks it), the clickServerValidated re-path fires,
    // the re-path plans the identical 3 waypoint route.
    now := time.Now()
    _, _, selfZ, _ := bot.SelfPosition()
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)

    // Advance past the full stuckTimeout: the first stuck detection
    // finds wp2 as a clear successor (nextClearWaypoint jumps the
    // cursor) - the WAYPOINT SKIP branch fires.
    now = now.Add(stuckTimeout + time.Second)
    loop.moveAt = time.Time{}
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.Equal(t, 1, loop.wpIndex,
        "the skip jumped the cursor onto wp2")
    require.True(t, loop.stuckFast,
        "the waypoint skip arm sets the fast stuck window (unchanged)")

    // The fast window fires the next detection in 4s, not 15s.
    now = now.Add(stuckFastTimeout + time.Second)
    loop.moveAt = time.Time{}
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.Equal(t, 2, loop.wpIndex,
        "the fast window fired the next detection")
}

// TestStuckRepathRebaselinesStuckWindow pins the second half of the
// fix: the re-path arm re-baselines the stuck window (stuckAt, stuckX,
// stuckY, stuckWP, stuckBest) from the re-path tick, the same way the
// skip arm does. Without this the stuck window carries the stale
// baseline from the first detection and the next detection's progress
// check measures against the wrong reference.
func TestStuckRepathRebaselinesStuckWindow(t *testing.T) {
    bot := newTestBot()
    moveSelfTo(bot, reproFastStuckX, reproFastStuckY, reproFastStuckZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    nav := &fakeNavigator{
        found: true,
        route: []pathfind.Vec3{
            {X: 43512, Y: 50504, Z: -2992},
            {X: 42664, Y: 51336, Z: -2992},
            {X: 36000, Y: 46765, Z: -3712},
        },
    }
    loop.SetNavigator(nav)
    // The click transport refuses every line off the aimed waypoint's
    // line: the village railing walls the skip targets, so
    // nextClearWaypoint returns the pinned cursor and the stuck fires
    // the re-path branch.
    nav.validateHook = railingPortAimed(loop)
    dest := pathfind.Vec3{
        X: float64(reproFastStuckZoneX),
        Y: float64(reproFastStuckZoneY),
        Z: float64(reproFastStuckZoneZ),
    }
    require.True(t, loop.startZoneReturnSegment(dest))
    loop.phase = phaseTownReturn

    // Open the stuck window at the dump position, then advance past
    // the stuckTimeout. The first stuck fires the re-path branch.
    baseline := time.Now().Add(-stuckTimeout - time.Second)
    loop.stuckAt = baseline
    loop.stuckX = reproFastStuckX
    loop.stuckY = reproFastStuckY
    loop.stuckWP = 0
    loop.stuckBest = 0
    _, _, selfZ, _ := bot.SelfPosition()
    rePathAt := time.Now()
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, rePathAt)
    require.Equal(t, 1, loop.rePaths,
        "the first stuck fired the re-path")

    // The re-path arm re-baselined the stuck window from the re-path
    // tick: stuckAt moved past the original baseline.
    require.True(t, loop.stuckAt.After(baseline),
        "the re-path arm re-baselined stuckAt from the re-path tick")
    require.True(t, loop.stuckAt.Equal(rePathAt) ||
        loop.stuckAt.After(rePathAt.Add(-time.Second)),
        "the re-path arm set stuckAt to the re-path tick")
    require.Equal(t, reproFastStuckX, loop.stuckX,
        "the re-path arm re-baselined stuckX")
    require.Equal(t, reproFastStuckY, loop.stuckY,
        "the re-path arm re-baselined stuckY")
    require.Equal(t, loop.wpIndex, loop.stuckWP,
        "the re-path arm re-baselined stuckWP to the current cursor")
}
