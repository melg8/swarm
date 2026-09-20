// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestPinnedCursorMarchesForwardWithoutWalkingBack pins the ping pong
// fix of the 2026-09-20 acceptance round (the owner report: the bot
// bought at the jewelry shop and froze on the walk to the teacher -
// stepping one step away from the plan origin, returning, repeating).
// The shape: the advance gate pins the cursor on the plan start (the
// sight refuses every forward line of the corridor), the armed
// extension marches the character out along the route samples - and
// the pinned aim (a waypoint the character already walked past) must
// NEVER walk it back. The pre-fix follower alternated the forward
// extension sample with the raw backward aim of the passed waypoint,
// one 63 unit step per walk request period, until the stuck ladder
// burned the trip. The fix holds the backward click and lets the
// extension own the march; the stuck detector counts the destination
// ground the march covers, so the recovery budget never burns on a
// walk that is honestly progressing.
func TestPinnedCursorMarchesForwardWithoutWalkingBack(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    nav.found = true
    nav.route = []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3500},
        {X: 45020, Y: 50060, Z: -3500},
        {X: 45040, Y: 50120, Z: -3500},
        {X: 45600, Y: 50600, Z: -3500},
    }
    // The sight refuses every forward line: the advance gate pins the
    // cursor on whichever waypoint it sits on (the elven corridor
    // shape - the whole corridor walls the sight oracle).
    nav.sightFunc = func(_, _ pathfind.Vec3) (bool, error) {
        return false, nil
    }
    fillInventory(bot)
    loop.tick()
    require.Equal(t, phaseTownWalk, loop.phase)
    // The recovery state of the acceptance round: the first stuck
    // already armed the short click extension (the extension arms in
    // the stuck handler when no clear successor exists).
    loop.extendArmed = true

    dest := pathfind.Vec3{X: 45600, Y: 50600, Z: -3500}
    destDist := func(x, y int32) float64 {
        return math.Hypot(dest.X-float64(x), dest.Y-float64(y))
    }

    // The march: one tick per walk request period, the sim stands the
    // character on the sent click target (the honest server answer
    // for a validated click). Every tick must leave the character no
    // farther from the destination than the tick before - the walk
    // back to the passed waypoint would violate it.
    now := time.Now()
    prevDist := destDist(45000, 50000)
    marched := 0
    for i := range 40 {
        now = now.Add(walkRequestPeriod)
        x, y, z, ok := bot.SelfPosition()
        require.True(t, ok)
        if loop.followWaypoints(x, y, z, now) {
            break
        }
        if len(game.walks) > marched {
            sent := game.walks[len(game.walks)-1]
            moveSelfTo(bot, sent[0], sent[1], sent[2])
            cur := destDist(sent[0], sent[1])
            require.LessOrEqual(t, cur, prevDist+1,
                "tick %d: the sent click walked the character back "+
                    "toward the passed waypoint (dest dist %.0f -> %.0f)"+
                    " - the ping pong the acceptance round reported",
                i, prevDist, cur)
            prevDist = cur
            marched = len(game.walks)
        }
    }

    require.Greater(t, marched, 2,
        "the march must cover real ground on the pinned corridor")
    finalX, finalY, _, ok := bot.SelfPosition()
    require.True(t, ok)
    require.Less(t, destDist(finalX, finalY), destDist(45000, 50000),
        "the march must close on the destination")
}

// TestStuckWindowCountsMarchedGround pins the progress term of the
// pinned march: the character marching along the route samples with
// the cursor pinned behind it shrinks the destination distance every
// window - the stuck detector must not burn the re-path budget on
// that progress (the acceptance round's escape ladder answered the
// walk that was honestly covering ground).
func TestStuckWindowCountsMarchedGround(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    nav := &fakeNavigator{found: true}
    nav.route = []pathfind.Vec3{
        {X: 45000, Y: 50000, Z: -3500},
        {X: 45020, Y: 50060, Z: -3500},
        {X: 45600, Y: 50600, Z: -3500},
    }
    loop.SetNavigator(nav)
    loop.lastHit = time.Now().Add(-time.Minute)

    // The march state: the cursor pinned on wp 0, the character 126
    // units along the route (past the aimed waypoint, inside the
    // corridor), the stuck window opened at the plan start.
    loop.waypoints = nav.route
    loop.wpIndex = 0
    loop.segmentDest = nav.route[2]
    loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
    loop.stuckX, loop.stuckY = 45000, 50000
    loop.stuckWP = 0
    loop.stuckBest = loop.stuckWaypointDistance(45000, 50000)

    var _ *state.Bot
    moveSelfTo(bot, 45039, 50117, -3500)
    x, y, _, _ := bot.SelfPosition()
    require.False(t, loop.walkStuck(time.Now(), x, y),
        "the marched ground toward the destination is progress - "+
            "the stuck window must reset on it")
}
