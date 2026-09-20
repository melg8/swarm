// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The reproduction of the 2026-09-19 17:18 state dump report (build
// 94ec5e3, bot unittest1, phase townReturn): the cursor key escape of the
// refused spawn cell finally walked ALONG the planned route (the
// previous round's contract held - "the refused clicks hand the walk
// to the cursor key escape, walking along the planned route toward
// 42748 51232 (18 claimed steps)"), but the owner met two new
// defects on the way:
//
//   - "прошел несколько точек по wasd - при этом они не отметились
//     как пройденные и потом перейдя в обычный режим вернулся назад
//     к предыдущим точкам": the claims walked the character across
//     the route's first waypoints, the plan cursor stayed behind
//     (the escape never touched it), and the resumed clicks walked
//     the character BACK - the dump's own fingerprint: the settle at
//     17:16:22, then "town walk stuck, skipping waypoint (cursor 1
//     of 49)" one second later, then the plan view showing wp 0
//     passed at t+22.5s (the resume time, not the walk time) and
//     segments of 8.3s, 5.0s, 10.7s spent walking BACK to the waypoints
//     the escape had already covered. Two minutes of the trip burned
//     on the walked prefix.
//   - "webui при ходьбе wasd - не корректно отображается направление
//     персонажа (не по ходу движения)": the claimed steps carried a
//     mirrored heading (cursorEscapeHeading fed atan2 the swapped
//     arguments), and the server echo of every claim carried the arm
//     heading instead of the claim facing - the web UI showed a
//     direction the character never walked.
//
// The contract pinned below: the WASD walk IS the ground progress -
// every route waypoint the claims walk onto marks passed the moment
// the claim lands, the settle advances the cursor to the first
// waypoint still ahead, the resumed clicks never walk back to the
// walked prefix, and the facing the session reports during the
// escape follows the claims (the mobius convention: atan2(deltaY,
// deltaX), LocationUtil.calculateHeadingFrom).

// syncDriveTick mirrors the production tick of walkTownWaypoints plus
// the plan view publish of the loop tick: the armed escape drives the
// claims, the routed segments follow their waypoints, every tick
// republishes the walk plan the way the loop does (the plan view
// stamps the passed waypoints as the cursor moves).
func syncDriveTick(
    t *testing.T, loop *Loop, sim *cursorKeyServer, game *fakeGame,
    bot *state.Bot, now time.Time,
) {
    t.Helper()
    driveSpawnWalk(t, loop, sim, game, bot, now)
    loop.publishWalkPlan()
}

// nearestPlanWaypoint returns the plan index of the waypoint nearest
// to the point (the plan index of a claim or a click target that sits
// on the route).
func nearestPlanWaypoint(
    waypoints []pathfind.Vec3, x, y int32,
) int {
    best, bestDist := 0, math.MaxFloat64
    for i := range waypoints {
        wp := waypoints[i]
        dist := math.Hypot(wp.X-float64(x), wp.Y-float64(y))
        if dist < bestDist {
            best, bestDist = i, dist
        }
    }

    return best
}

// TestReproEscapeClaimsMarkWalkedWaypointsPassed pins the ground
// progress contract of the 17:18 report end to end: the escape arms
// over the planned route, the claims walk the character along it, the
// walked waypoints mark passed DURING the escape (the plan view
// stamps them the moment the claims walk onto them), the settle
// leaves the cursor on the first waypoint still ahead, and the
// resumed clicks aim forward - never back to the walked prefix (the
// dump: two minutes of backtracking segments after the resume).
func TestReproEscapeClaimsMarkWalkedWaypointsPassed(t *testing.T) {
    loop, game, bot, sim, sink := spawnDumpLoop(t)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase)
    require.Greater(t, len(loop.waypoints), 1,
        "the zone return plans a real multi waypoint route")

    armWp := -1
    armed := false
    settled := false
    settleLog := ""
    firstClickAfterSettle := [3]int32{}
    now := time.Now()
    for i := 0; i < 400 && !settled; i++ {
        now = now.Add(2 * time.Second)
        if loop.cursorEscape.armed && !armed {
            armed = true
            armWp = loop.wpIndex
        }
        beforeWalks := len(game.walks)
        syncDriveTick(t, loop, sim, game, bot, now)
        if armed && !settled {
            // The cursor of the armed escape never regresses: the
            // claims only mark the walked ground passed.
            require.GreaterOrEqual(t, loop.wpIndex, armWp,
                "the plan cursor regressed under the armed escape")
            if len(game.claims) > 0 {
                // Every completing claim marks its route waypoint
                // passed the moment it lands (the owner contract: the
                // wasd walked points show passed while the wasd walk
                // streams).
                for k := range game.claims {
                    if k >= len(loop.cursorEscape.wpMap) {
                        break
                    }
                    if done := loop.cursorEscape.wpMap[k]; done >= 0 {
                        require.GreaterOrEqual(t, loop.wpIndex, done+1,
                            "claim %d completed route waypoint %d - "+
                                "the plan cursor must mark it passed", k, done)
                    }
                }
            }
            if !loop.cursorEscape.armed && armWp >= 0 {
                settled = true
                settleLog = sink.String()
                if len(game.walks) > beforeWalks {
                    firstClickAfterSettle = game.walks[beforeWalks]
                }
            }
        }
    }

    require.True(t, armed, "the escape armed over the refused plan")
    require.True(t, settled, "the escape settled back to the clicks")
    require.NotEmpty(t, game.claims,
        "the claimed steps walked the route")

    // The cursor moved past the walked prefix: the escape walked
    // cursorEscapeRouteMax of route ground - the plan must show it
    // (the dump kept the cursor at wp 1 through the whole escape).
    require.Greater(t, loop.wpIndex, armWp,
        "the wasd walked waypoints must mark passed - the plan "+
            "cursor must advance past the walked prefix")

    // The plan view stamps the walked prefix as passed: every
    // waypoint before the cursor carries its arrival time (the
    // dump's wp 0 showed passed at the resume time only because the
    // skip machinery walked back through it).
    snap := bot.Snapshot()
    require.Equal(t, loop.wpIndex, snap.WalkIndex,
        "the published plan view follows the follower cursor")
    for i := 0; i < snap.WalkIndex && i < len(snap.WalkWpAt); i++ {
        require.False(t, snap.WalkWpAt[i].IsZero(),
            "the plan view must stamp walked waypoint %d as passed", i)
    }

    // The resumed clicks aim forward: the first click target sits
    // closer to the escaped ground than to the escape origin (the
    // dump's resume clicked wp 1 - 1100 units BACK toward the spawn).
    if firstClickAfterSettle != ([3]int32{}) {
        settle := game.claims[len(game.claims)-1]
        toClick := nearestPlanWaypoint(loop.waypoints,
            firstClickAfterSettle[0], firstClickAfterSettle[1])
        click := loop.waypoints[toClick]
        fromSettle := math.Hypot(
            click.X-float64(settle[0]), click.Y-float64(settle[1]))
        fromOrigin := math.Hypot(
            click.X-float64(loop.cursorEscape.originX),
            click.Y-float64(loop.cursorEscape.originY))
        require.Less(t, fromSettle, fromOrigin,
            "the resumed click must aim ground ahead of the escaped "+
                "character, never back toward the escape origin")
    }

    // The dump's fingerprint must not reappear: no stuck skip runs
    // between the settle and the first resumed click (the dump's
    // skip fired one second after the resume and the walk went back).
    settleAt := strings.Index(settleLog, "the cursor key escape walked to")
    require.GreaterOrEqual(t, settleAt, 0, "the settle line logged")
    tail := settleLog[settleAt:]
    if firstClickAfterSettle != ([3]int32{}) {
        require.NotContains(t, tail, "town walk stuck, skipping waypoint",
            "the resumed walk must click forward, not burn the window "+
                "on the skipped walked prefix")
    }

    // The walk completes along the routes (the end to end of the
    // report scenario keeps holding).
    for range 400 {
        now = now.Add(2 * time.Second)
        syncDriveTick(t, loop, sim, game, bot, now)
        if x, y, _, ok := bot.SelfPosition(); ok &&
            inZoneSquare(x, y, zoneDumpX, zoneDumpY, 633) &&
            !loop.tripActive() {
            break
        }
    }
    x, y, _, ok := bot.SelfPosition()
    require.True(t, ok)
    require.True(t, inZoneSquare(x, y, zoneDumpX, zoneDumpY, 633),
        "the walk must reach the hunting zone along the routes")
}

// TestReproEscapeClaimsCarryTheServerHeading pins the facing of the
// claimed stream: every claim's heading must equal the mobius
// convention of its own step direction (state.HeadingFromDelta is
// the same convention the movement broadcasts apply) - the mirrored
// atan2 arguments the claims carried before the 17:18 report faced
// the character sideways to its WASD walk, and the mirror is not
// only cosmetic: the mobius cursor key movement probes its obstacle
// front along the heading (Creature.updatePosition).
func TestReproEscapeClaimsCarryTheServerHeading(t *testing.T) {
    loop, game, bot, sim, _ := spawnDumpLoop(t)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase)

    now := time.Now()
    for i := 0; i < 200 && len(game.claims) < 8; i++ {
        now = now.Add(2 * time.Second)
        syncDriveTick(t, loop, sim, game, bot, now)
    }

    require.GreaterOrEqual(t, len(game.claims), 3,
        "the escape streamed its claimed steps")

    // The tracker facing follows the claims (the official client
    // renders the arrow walk facing from its own movement
    // simulation): after every drive tick the facing equals the last
    // claim's heading.
    last := game.claims[len(game.claims)-1]
    require.Equal(t, last[3], bot.SelfHeading(),
        "the session facing must follow the claimed steps - the web "+
            "UI direction of the wasd walk")

    // Every claim names the heading of its own step direction (the
    // first claim leaves the escape origin, the rest leave the
    // previous claim's landing - the sim syncs the claimed
    // placement exactly).
    prev := [2]int32{loop.cursorEscape.originX,
        loop.cursorEscape.originY}
    for i, claim := range game.claims {
        want := state.HeadingFromDelta(
            claim[0]-prev[0], claim[1]-prev[1])
        require.Equal(t, want, claim[3],
            "claim %d carries the mirrored heading", i)
        prev = [2]int32{claim[0], claim[1]}
    }
}

// TestCursorEscapeHeadingMatchesTheServerConvention pins the heading
// formula against the mobius master (LocationUtil.calculateHeadingFrom:
// degrees of atan2(deltaY, deltaX) scaled by 65536 over 360): east is
// 0, south 16384, west 32768, north 49152. The dump sample of the
// 17:18 report (the walk from 29549 51756 toward 28928 51714 - west
// with a slight north tilt) names 33472 on the server; the swapped
// atan2 the claims carried produced 48408 for the same step.
func TestCursorEscapeHeadingMatchesTheServerConvention(t *testing.T) {
    // The compass anchors of the mobius convention.
    require.Equal(t, int32(0), cursorEscapeHeading(0, 0, 100, 0),
        "east is heading 0")
    require.Equal(t, int32(16384), cursorEscapeHeading(0, 0, 0, 100),
        "south (+y) is heading 16384")
    require.Equal(t, int32(32768), cursorEscapeHeading(0, 0, -100, 0),
        "west is heading 32768")
    require.Equal(t, int32(49152), cursorEscapeHeading(0, 0, 0, -100),
        "north (-y) is heading 49152")

    // The convention the state tracker applies to the movement
    // broadcasts: the claims must agree with it for every direction.
    for _, dir := range [][2]int32{
        {100, 0}, {0, 100}, {-100, 0}, {0, -100},
        {100, 100}, {-100, 100}, {-100, -100}, {100, -100},
        {621, -42}, {-621, -42},
    } {
        want := state.HeadingFromDelta(dir[0], dir[1])
        require.Equal(t, want,
            cursorEscapeHeading(0, 0, dir[0], dir[1]),
            "the claim heading must match the movement broadcast "+
                "convention for the direction %v", dir)
    }

    // The dump sample: west with a slight north tilt is 33472 on the
    // server (the walked direction of the 17:18 dump), never the
    // mirrored south line the old formula produced.
    walked := cursorEscapeHeading(29549, 51756, 28928, 51714)
    require.Equal(t, int32(33472), walked,
        "the dump's own walk names heading 33472 on the server")
    require.Equal(t, int32(33472),
        state.HeadingFromDelta(-621, -42),
        "the state convention agrees with the server answer")
}

// TestEscapeSettleRebaselinesTheStuckWindow pins the stuck window of
// the settle: the escape moved the character server side, so the
// frozen verdict of the pre escape ground must not fire on the
// resumed clicks (the dump: the skip fired one second after the
// resume because the stuck window carried through the escape).
func TestEscapeSettleRebaselinesTheStuckWindow(t *testing.T) {
    loop, game, bot, sim, sink := spawnDumpLoop(t)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase)

    armed := false
    settled := false
    settleLogLen := 0
    now := time.Now()
    for i := 0; i < 200 && !settled; i++ {
        now = now.Add(2 * time.Second)
        if loop.cursorEscape.armed {
            armed = true
        }
        if armed && !loop.cursorEscape.armed {
            settled = true
            settleLogLen = sink.Len()
        }
        if !settled {
            syncDriveTick(t, loop, sim, game, bot, now)
        }
    }

    require.True(t, armed, "the escape armed")
    require.True(t, settled, "the escape settled")
    // The ticks right after the settle drive the follower: the fresh
    // stuck window must hold (no skip, no re-path) while the resumed
    // click walks the character forward.
    for range 3 {
        now = now.Add(2 * time.Second)
        syncDriveTick(t, loop, sim, game, bot, now)
    }
    after := sink.String()[settleLogLen:]
    require.NotContains(t, after, "town walk stuck, skipping waypoint",
        "the settled walk must not trip the carried stuck verdict")
    require.NotContains(t, after, "town walk stuck, re-pathing",
        "the settled walk must not re-path on the carried verdict")

    // The clicks resumed from the escaped ground (the normal mode
    // returned at the point).
    require.NotEmpty(t, game.walks,
        "the routed clicks resumed after the settle")
}
