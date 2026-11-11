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

// The move start watchdog of the quest walk: a click whose movement
// never started names its waypoint dead instead of burning the stuck
// window, and the plan skips that waypoint - a re-plan from the same
// position answers the identical deterministic waypoint, which is the
// loop the frozen client pawn of the city shopping run produced.

// shortenMoveStartSeams squeezes the quest walk watchdog seams: the
// move start verdict lands in milliseconds and the polls cost no wall
// clock. The production values are restored on cleanup.
func shortenMoveStartSeams(t *testing.T) {
    t.Helper()
    wait, originalPace := questMoveStartWait, pace
    questMoveStartWait = 20 * time.Millisecond
    pace = func(time.Duration) {}
    t.Cleanup(func() {
        questMoveStartWait = wait
        pace = originalPace
    })
}

// blockedRoute is a three waypoint plan the character never walks:
// every click is accepted by the fake game and moves nothing.
var blockedRoute = []pathfind.Vec3{
    {X: 46000, Y: 50000, Z: -3500},
    {X: 47000, Y: 50000, Z: -3500},
    {X: 48000, Y: 50000, Z: -3500},
}

// TestFollowPlannedSegmentSkipsBlockedWaypoints pins the skip: every
// waypoint of the plan is clicked once, named dead by the watchdog and
// skipped in the same plan - the follower never re-clicks the first
// waypoint while the deterministic plan keeps answering it.
func TestFollowPlannedSegmentSkipsBlockedWaypoints(t *testing.T) {
    shortenMoveStartSeams(t)

    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{found: true, route: blockedRoute})

    selfX, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    followed := loop.followPlannedSegment(selfX, selfY, selfZ,
        48000, 50000, time.Now().Add(10*time.Second))
    require.False(t, followed,
        "a plan whose every click started nothing reports failure")

    expected := make([][3]int32, 0, len(blockedRoute))
    for _, wp := range blockedRoute {
        expected = append(expected, [3]int32{
            int32(wp.X), int32(wp.Y), int32(wp.Z),
        })
    }
    require.Equal(t, expected, game.walks,
        "the follower clicks each blocked waypoint once and skips it, "+
            "never repeating the first one")
}

// TestFollowPlannedSegmentKeepsFollowingStartedWalks verifies the
// watchdog does not fire on a click whose movement started: the game
// moves the character on every click and the whole plan completes.
func TestFollowPlannedSegmentKeepsFollowingStartedWalks(t *testing.T) {
    shortenMoveStartSeams(t)

    bot := newTestBot()
    game := &movingGame{fakeGame: &fakeGame{}, bot: bot}
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{found: true, route: blockedRoute})

    selfX, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    followed := loop.followPlannedSegment(selfX, selfY, selfZ,
        48000, 50000, time.Now().Add(10*time.Second))
    require.True(t, followed, "the walked plan completes")
    require.Len(t, game.walks, len(blockedRoute),
        "one click per waypoint, no watchdog interruption")
}

// TestAwaitSegmentProgressNamesTheDeadClickFast pins the direct click
// fast path: the route gives up inside the move start window instead
// of standing out the whole walk budget on a click that started
// nothing.
func TestAwaitSegmentProgressNamesTheDeadClickFast(t *testing.T) {
    shortenMoveStartSeams(t)

    bot := newTestBot()
    loop := NewLoop(&fakeGame{}, bot)
    started := time.Now()
    moved := loop.awaitSegmentProgress(65000, 50000,
        time.Now().Add(time.Minute))
    require.False(t, moved, "a click that started nothing never arrives")
    require.Less(t, time.Since(started), time.Second,
        "the dead click is named inside the move start window, not at "+
            "the route deadline")
}

// TestAwaitSegmentProgressFollowsTheRunningWalk verifies the direct
// click path keeps its contract: a walk that started reports progress
// at once, before the move start window lapses.
func TestAwaitSegmentProgressFollowsTheRunningWalk(t *testing.T) {
    shortenMoveStartSeams(t)

    bot := newTestBot()
    loop := NewLoop(&fakeGame{}, bot)
    moved := false
    pace = func(time.Duration) {
        if moved {
            return
        }
        moved = true
        moveSelfTo(bot, 45000, 50100, -3500)
    }
    require.True(t, loop.awaitSegmentProgress(65000, 50000,
        time.Now().Add(5*time.Second)),
        "the running walk reports progress")
}
