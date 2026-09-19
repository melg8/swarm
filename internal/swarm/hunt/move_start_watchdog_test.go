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

// The move start watchdog tests pin the owner rule of the
// 2026-09-19 round: a movement command that never started the
// movement switches the recovery mode AT ONCE - the walker never
// stands out a full stuck window (15 s) on a click it can already
// name dead. The three dead click families each get their fast
// path: the silent click (the server accepted the packet and did
// nothing), the refusing pocket (ActionFailed from the ground the
// character stands on) and the silent routed hop of the direct leg.

// watchdogRoute is the 6 waypoint plan of the 2026-09-14 08:13 dump
// (the same route the stuck fast repro walks): the follower leg
// whose first click the server swallows.
var watchdogRoute = []pathfind.Vec3{
    {X: 43512, Y: 50504, Z: -2992},
    {X: 42664, Y: 51336, Z: -2992},
    {X: 40648, Y: 52680, Z: -3224},
    {X: 36168, Y: 47368, Z: -3664},
    {X: 36008, Y: 46952, Z: -3704},
    {X: 36000, Y: 46765, Z: -3712},
}

// TestMoveStartWatchdogForcesTheRecovery pins the silent click fast
// path: the click passes the offline validation and goes out, the
// server never answers and never moves the character, and the
// recovery (the re-path) fires inside the move start window - long
// before the full stuckTimeout the pre-watchdog walker waited.
func TestMoveStartWatchdogForcesTheRecovery(t *testing.T) {
    bot := newTestBot()
    moveSelfTo(bot, reproFastStuckX, reproFastStuckY, reproFastStuckZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{
        found: true,
        route: watchdogRoute,
        // Every forward line blocked: the re-path branch owns the
        // recovery (no waypoint skip).
        blind: true,
    })
    dest := pathfind.Vec3{
        X: float64(reproFastStuckZoneX),
        Y: float64(reproFastStuckZoneY),
        Z: float64(reproFastStuckZoneZ),
    }
    require.True(t, loop.startZoneReturnLeg(dest))
    loop.phase = phaseTownReturn

    now := time.Now()
    _, _, selfZ, _ := bot.SelfPosition()
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.False(t, loop.moveAt.IsZero(),
        "the first click went out")

    // The next tick past the move start window: the watchdog names
    // the click dead and forces the stuck verdict - the re-path
    // fires without waiting the full stuckTimeout.
    now = now.Add(moveStartWindow + time.Second)
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.Equal(t, 1, loop.rePaths,
        "the watchdog forced the re-path on the first dead click")
    require.Less(t, moveStartWindow+time.Second, stuckTimeout,
        "the whole recovery ran inside the move start window, "+
            "the full stuck window never burned")
}

// TestPocketRefusalArmsTheCursorEscapeAtOnce pins the refusing pocket
// fast path: the server answers ActionFailed for every click from the
// cell (the 2026-09-14 15:10 report - even the official client's
// mouse clicks died there). The watchdog forces a stuck verdict per
// dead click, the varied aims spend their budget first (the target
// specific cure keeps its chance), and the moment they are spent on
// the same refusing cell the cursor key escape arms - the claim
// ladder walks the planned route out of the pocket.
func TestPocketRefusalArmsTheCursorEscapeAtOnce(t *testing.T) {
    bot := newTestBot()
    moveSelfTo(bot, reproFastStuckX, reproFastStuckY, reproFastStuckZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{
        found: true,
        route: watchdogRoute[:3],
        blind: true,
    })
    dest := pathfind.Vec3{
        X: float64(reproFastStuckZoneX),
        Y: float64(reproFastStuckZoneY),
        Z: float64(reproFastStuckZoneZ),
    }
    require.True(t, loop.startZoneReturnLeg(dest))
    loop.phase = phaseTownReturn

    now := time.Now()
    _, _, selfZ, _ := bot.SelfPosition()
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    bot.ApplyActionFailed(now.Add(time.Millisecond))
    require.NotEmpty(t, game.walks, "the click went out")

    // The watchdog ticks: every dead click forces the next verdict,
    // the varied aims go out one per verdict and the pocket escape
    // arms the moment the variant budget is spent on the same cell.
    now = now.Add(moveStartWindow + time.Second)
    for i := range refusalVariantsMax {
        _ = loop.followWaypoints(
            reproFastStuckX, reproFastStuckY, selfZ, now)
        bot.ApplyActionFailed(now.Add(time.Millisecond))
        now = now.Add(moveStartWindow + time.Second)
        require.Len(t, game.walks, i+2,
            "verdict %d varied the aim (one variant per verdict)",
            i+1)
    }
    _ = loop.followWaypoints(
        reproFastStuckX, reproFastStuckY, selfZ, now)
    require.Equal(t, 1, loop.cursorEscapes,
        "the refusing pocket armed the cursor key escape once the "+
            "varied aims proved useless")
    require.NotEmpty(t, game.cursorWalks,
        "the movement mode 0 arm went out")

    // The armed escape owns the leg: the phase handler (the walk
    // town waypoints entry the real loop ticks through) drives the
    // claims, the mouse clicks stay down.
    claimCount := len(game.claims)
    walkCount := len(game.walks)
    _ = loop.walkTownWaypoints()
    require.Greater(t, len(game.claims), claimCount,
        "the claims advance while the escape owns the leg")
    require.Len(t, game.walks, walkCount,
        "no further mouse click goes out while the escape runs")
}

// TestDirectLegSilentHopsArmTheEscape was the direct leg fast path
// pin; the direct server routed walk is eliminated (the owner rule of
// the 2026-09-19 16:02 round: НИКОГДА не идти напрямую - see
// direct_walk_elimination_repro_test.go). The never-burn-the-window
// contract lives on in the follower path: the move start watchdog
// forces the stuck verdict per dead click (the first test of this
// file) and the frozen ladder arms the cursor escape along the plan
// (the new repro file).
