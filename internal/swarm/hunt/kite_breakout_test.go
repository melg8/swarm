// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The cornered-pocket breakout of the kite (issue #70, the
// walled-pocket round): when the whole retreat hemisphere stands
// walled, the escape may still live in the cone the sweep never
// covered - the anti-gap rays threading BETWEEN the chasers. The
// live fleet round measured the standing answer eating whole windows
// on the hex-cell pocket (an 8.5 s median shot-to-retreat lag over
// four retreats); these tests pin the breakout contract: the
// threading step through the walled pocket, the flank clearance that
// skips the crowded rays, the sealed-pocket hold, and the rotation
// path that breaks out after the hemisphere exhausted its lanes.

import (
    "bytes"
    "log"
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// pocketChaser adds a chaser under its own object id to the pocket
// scene: the mob stands at the given point and already targets the
// character (the same SelfAttackers membership trainMember builds for
// mob 8 - the crowd scenes need more than one extra chaser).
func pocketChaser(bot *state.Bot, objectID, x, y int32) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: objectID, TemplateID: 1000001, Attackable: true,
        X: x, Y: y, Z: -3500, Name: "Keltir",
    })
    bot.ApplyAttack(state.Attack{
        AttackerID: objectID, X: x, Y: y, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
}

// TestKiteBreakoutThreadsTheWalledPocket pins the core breakout rule:
// an east-west chaser line whose gap hemisphere stands walled no
// longer holds the archer - the breakout threads the anti-gap cone,
// the first 135 degree weave off the gap ray, and the step OPENS the
// distance to both chasers while it escapes (the measured pocket of
// the hex cells: the lane south of the chaser line stayed open while
// the north answered walled).
func TestKiteBreakoutThreadsTheWalledPocket(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 7, 45200)
    nav := &fakeNavigator{}
    // The gap hemisphere (north of the chaser line) answers walled;
    // the anti-gap cone (south) stays open.
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.Y < 50000, nil
    }
    loop.SetNavigator(nav)
    var logBuf bytes.Buffer
    loop.SetLogger(log.New(&logBuf, "", 0))

    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "a walled gap hemisphere must not hold the archer - the "+
            "breakout threads the anti-gap cone")
    step := game.walks[0]
    // The gap bisector of the east-west line runs north; the first
    // breakout candidate rotates it by +135 degrees - the southwest
    // weave, 45 degrees off the western chaser's own ray.
    require.Equal(t, [3]int32{44717, 49717, -3500}, step,
        "the breakout takes the first clearance weave off the gap ray")
    // The weave opens the distance to BOTH chasers while it escapes.
    toTarget := math.Hypot(
        float64(step[0]-45200), float64(step[1]-50000))
    toMember := math.Hypot(
        float64(step[0]-44800), float64(step[1]-50000))
    require.Greater(t, toTarget, 200.0,
        "the breakout must open the distance to the target")
    require.Greater(t, toMember, 200.0,
        "the breakout must open the distance to the train member")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the breakout is a step, not a hold")
    require.Contains(t, logBuf.String(), "breaking out",
        "the breakout names itself in the event feed")
}

// TestKiteBreakoutSkipsTheCrowdedRays pins the flank clearance: a
// crowd sitting ON the weave rays pushes the breakout down the
// ladder - the straight anti-gap ray - instead of running onto a
// mob's own bearing. The clearance is the stricter contract that
// replaces the away half-plane guard inside the breakout cone: it
// weaves between the train's seams, it never steps into the melee.
func TestKiteBreakoutSkipsTheCrowdedRays(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 44800)
    // The crowd: the target 200 units west plus a member 200 units
    // southeast - both 135 degree weave rays of the gap (northeast,
    // the bisector of the 225 degree open span) land within the
    // flank clearance of one of the two bearings.
    pocketChaser(bot, 9, 45141, 49859)
    mobHitsCharacterAt(bot, 7, 44800)
    nav := &fakeNavigator{}
    // The whole gap hemisphere and the shallow south answer walled;
    // the deep south ray of the ladder stays open.
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.Y < 49800, nil
    }
    loop.SetNavigator(nav)

    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the crowded weaves fall through to the anti-gap ray")
    step := game.walks[0]
    // Both weave candidates sit 22.5 degrees off a chaser bearing
    // (inside the ~36 degree clearance refusal); the straight
    // anti-gap ray clears both bearings by 67.5 degrees - it serves
    // the breakout.
    require.Equal(t, [3]int32{44847, 49630, -3500}, step,
        "the breakout skips the crowded rays onto the anti-gap ray")
    toTarget := math.Hypot(
        float64(step[0]-44800), float64(step[1]-50000))
    require.Greater(t, toTarget, 200.0,
        "the anti-gap step must still open the distance to the target")
}

// TestKiteRotationExhaustedBreaksOut pins the breakout on the
// rotation path: a retreat lane the server refuses after the walk
// armed rotates onto the next hemisphere lane as always - but when
// the whole hemisphere is dead or walled, the rotation falls through
// to the breakout cone instead of standing the ladder down. The
// refused cells land in the dead memory either way: the next probe
// starts on what the battery has left.
func TestKiteRotationExhaustedBreaksOut(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 7, 45200)
    nav := &fakeNavigator{}
    // Only two lanes answer open: the straight west lane of the gap
    // fan (the initial retreat) and the straight south ray of the
    // breakout ladder - everything else is walled.
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.X == 44600 ||
            (to.X == 45000 && to.Y == 49600), nil
    }
    loop.SetNavigator(nav)

    // The opening retreat takes the open west lane of the fan.
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the open fan lane carried the opening retreat")
    require.Equal(t, [3]int32{44600, 50000, -3500}, game.walks[0])

    // The west cell got refused and the walk aged past the probe:
    // the hemisphere sweep (the dead west cell skipped, every other
    // candidate walled) finds nothing - the rotation breaks out
    // through the south ray instead of standing down.
    loop.tracker.ApplyActionFailed(time.Now())
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the exhausted rotation breaks out through the cone")
    require.Equal(t, [3]int32{45000, 49600, -3500}, game.walks[1],
        "the rotation-path breakout lands on the south ray")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the breakout keeps the ladder alive")
}
