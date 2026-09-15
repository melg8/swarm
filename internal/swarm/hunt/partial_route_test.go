// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The consumer pins of the partial round (docs/navmesh.md): a dry
// approach search answering the closest-reachable corridor (Result
// with Found=false and Partial set - what the navmesh hybrid serves
// once the engine confirms the destination unreachable) plans the
// town legs and the quest segments instead of aborting them - the
// walk goes toward the closest reachable point.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestTownLegPartialWalksClosestReachable pins the town leg planner:
// a partial dry answer arms the waypoint follower with the
// closest-reachable corridor - the leg plans (no abort), the
// destination stays the requested one (the arrival checks of the
// trip machinery answer the not-reached case through their own
// flows), and the first follower click aims the partial corridor.
func TestTownLegPartialWalksClosestReachable(t *testing.T) {
    loop, game, _, nav := newDelevelLoop(11)
    partial := []pathfind.Vec3{
        {X: 45000, Y: 50600, Z: -3500},
        {X: 45000, Y: 51000, Z: -3500},
    }
    nav.partialRoute = partial
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()

    dest := pathfind.Vec3{X: 45000, Y: 52000, Z: -3500}
    require.True(t, loop.startWalkLeg(dest),
        "the partial corridor plans the leg")
    require.Equal(t, partial, loop.waypoints,
        "the follower arms on the closest-reachable waypoints")
    require.Equal(t, dest, loop.legDest,
        "the leg destination stays the requested one")
    require.Equal(t, phaseTownWalk, loop.phase,
        "the trip walks instead of aborting")

    // One follower step: the click aims the first partial waypoint.
    loop.moveAt = time.Time{}
    walked := loop.walkTownWaypoints()
    require.False(t, walked, "the partial walk still has ground")
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{45000, 50600, -3500}, game.walks[0])
}

// TestQuestSegmentPartialWalksClosestReachable pins the quest segment
// planner: a partial dry answer walks its waypoints - the plan exists
// (the caller keeps the planned walk instead of falling through to
// the direct click) and the waypoint clicks walk the corridor toward
// the closest reachable point.
func TestQuestSegmentPartialWalksClosestReachable(t *testing.T) {
    bot := state.NewBot("acc1")
    bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplyUserInfo(state.UserInfo{
        Name: "test1", Level: 11, Race: 1, ClassID: 18,
        Exp: 50000,
        X:   45000, Y: 50000, Z: -3500,
        MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
        MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
        MoveSpeedMult: 1,
    })
    game := &movingQuestGame{bot: bot}
    nav := &fakeNavigator{found: true}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.lastHit = time.Now().Add(-time.Minute)
    partial := []pathfind.Vec3{
        {X: 45000, Y: 50600, Z: -3500},
        {X: 45000, Y: 51000, Z: -3500},
    }
    nav.partialRoute = partial

    planned := loop.followPlannedSegment(
        45000, 50000, -3500, 45000, 51800,
        time.Now().Add(80*time.Millisecond))
    require.True(t, planned,
        "the partial segment plans and walks its corridor")
    require.NotEmpty(t, game.walks,
        "the waypoint clicks go out")
    require.Equal(t, [3]int32{45000, 50600, -3500}, game.walks[0])
    // The character advanced along the partial corridor: the segment
    // moved it off the start position toward the closest reachable
    // point (the degenerate no-progress guard stays satisfied).
    afterX, afterY, _, ok := loop.tracker.SelfPosition()
    require.True(t, ok)
    require.Greater(t, afterY, int32(50000))
    require.Equal(t, int32(45000), afterX)
}

// movingQuestGame snaps the character onto every accepted walk click
// (the server's move broadcast simplified): the follower waypoint
// then counts as reached and the segment completes.
type movingQuestGame struct {
    fakeGame
    bot *state.Bot
}

// WalkTo records the click and moves the character onto its target.
func (g *movingQuestGame) WalkTo(x int32, y int32, z int32) error {
    if err := g.fakeGame.WalkTo(x, y, z); err != nil {
        return err
    }
    moveSelfTo(g.bot, x, y, z)

    return nil
}

// TestTownLegDryMissStillAborts pins the boundary: a bare not found
// (no waypoints, the verdict the engine answers for a swim-only
// destination) still refuses to plan the leg - the partial round
// changes the closest-reachable walk, never the reachability
// verdict itself.
func TestTownLegDryMissStillAborts(t *testing.T) {
    loop, _, _, nav := newDelevelLoop(11)
    nav.dryMiss = true
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()

    require.False(t, loop.startWalkLeg(
        pathfind.Vec3{X: 45000, Y: 52000, Z: -3500}),
        "the bare not found still aborts the leg")
}
