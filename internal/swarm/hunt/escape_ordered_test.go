// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The ordered escape regressions of the reported chaotic flee: the
// lane fan runs away from the threat along the square edge instead
// of turning through the zone center, and a cornered or dead escape
// resets the aggro through the logout.

import (
    "bytes"
    "log"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// spawnMobAtXY publishes an attackable npc at an explicit world
// point (the corner scenes need a y off the test character's line).
func spawnMobAtXY(bot *state.Bot, objectID int32, x int32, y int32) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: objectID, TemplateID: 1000001, Attackable: true,
        X: x, Y: y, Z: -3500, Name: "Gremlin",
    })
}

// relocateSelf relocates the test character (object id 100 of the
// newTestBot scene) without starting a walk.
func relocateSelf(bot *state.Bot, x int32, y int32) {
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: x, Y: y, Z: -3500,
        DestX: x, DestY: y, DestZ: -3500,
    })
}

// TestEscapeRunsAlongTheEdgeInsteadOfTheCenterFallback pins the
// reported 180 degree flip: a character in the north-east corner
// with the mob to the west used to run toward the zone center -
// straight through the mob. The ordered fan clamps the outward lanes
// and runs south along the square edge instead.
func TestEscapeRunsAlongTheEdgeInsteadOfTheCenterFallback(t *testing.T) {
    bot := newTestBot()
    relocateSelf(bot, 45500, 50500)
    spawnMobAtXY(bot, 7, 45000, 50500)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetHuntingZone(45000, 50000, 633)
    loop.target = 7

    moveX, moveY, _, ok := loop.escapeWalkDestination()
    require.True(t, ok)
    require.Equal(t, int32(45500), moveX,
        "the east lanes are clamped, the escape never folds back west")
    require.Equal(t, int32(49800), moveY,
        "the south lane covers one full escape segment along the edge")
    require.Greater(t, moveY, int32(49427),
        "the endpoint stays inside the patrol square")
}

// TestEscapeNeverRunsTowardTheThreat pins the lane invariant: with
// the mob due east every fan candidate points west or sideways, so
// no escape segment can close on the chaser.
func TestEscapeNeverRunsTowardTheThreat(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    moveX, moveY, _, ok := loop.escapeWalkDestination()
    require.True(t, ok)
    require.Less(t, moveX, int32(45000),
        "the escape opens eastward distance")
    require.Equal(t, int32(50000), moveY)
}

// TestFleeLogsOutWhenNoEscapeLaneOpens pins the cornered answer: a
// square too small to carry any ordered lane (every candidate is
// clamped under escapeMinLane) resets the aggro through the logout
// instead of walking nowhere.
func TestFleeLogsOutWhenNoEscapeLaneOpens(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    attackerHits(bot, 7, 45600)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingZone(45000, 50000, 100)

    loop.fleeFromThreat(time.Now())
    require.Equal(t, 1, game.logouts,
        "the cornered flee resets the aggro instead of standing")
}

// TestFleeLogsOutWhenTheEscapeDoesNotMove pins the silent-drop
// guard (H-006): the first segment goes out, the server accepts the
// clicks but the character never travels - the next observation
// window logs the session out instead of standing in the blows.
func TestFleeLogsOutWhenTheEscapeDoesNotMove(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    attackerHits(bot, 7, 45600)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    base := time.Now().Add(-time.Minute)

    loop.fleeFromThreat(base)
    require.Len(t, game.walks, 1, "the escape issues its first segment")
    require.Zero(t, game.logouts)

    loop.fleeFromThreat(base.Add(escapeNoProgressWindow +
        100*time.Millisecond))
    require.Equal(t, 1, game.logouts,
        "a dead escape resets the aggro instead of standing in the blows")
}

// TestFleeLogsOutWhenTheChaserKeepsPace pins the closure-rate exit:
// the character walks the whole segment but the chaser holds the gap
// (a mob of the character's own run speed), so the probe window ends
// the session instead of burning the full flee budget. The 14:03:09
// dump ran 21 s with a level 10 Kaboo Orc Fighter at ~19 units.
func TestFleeLogsOutWhenTheChaserKeepsPace(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45019)
    attackerHits(bot, 7, 45019)
    game := &fakeGame{}
    logBuf := &bytes.Buffer{}
    loop := NewLoop(game, bot)
    loop.SetLogger(log.New(logBuf, "", 0))
    base := time.Now().Add(-time.Minute)

    loop.fleeFromThreat(base)
    require.Len(t, game.walks, 1, "the escape issues its first segment")
    require.Zero(t, game.logouts)

    // The character covered the segment and the chaser matched it:
    // the gap stays at ~19 units when the probe window lapses.
    relocateSelf(bot, 44400, 50000)
    bot.ApplyMovement(state.Movement{
        ObjectID: 7, X: 44419, Y: 50000, Z: -3500,
        DestX: 44419, DestY: 50000, DestZ: -3500,
    })
    loop.fleeFromThreat(base.Add(fleeGapWindow + time.Second))

    require.Equal(t, 1, game.logouts,
        "a chaser that keeps pace ends the session early")
    require.Contains(t, logBuf.String(), "the chaser keeps pace at")
}

// TestEscapeClosureWatchdogReanchorsOnAnOpeningGap pins the healthy
// half: a gap that widened past the open margin keeps the escape
// running and re-anchors the probe.
func TestEscapeClosureWatchdogReanchorsOnAnOpeningGap(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45019)
    attackerHits(bot, 7, 45019)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    base := time.Now().Add(-time.Minute)

    loop.fleeFromThreat(base)
    require.Len(t, game.walks, 1)

    // The chaser turned home / fell behind: the gap opened well past
    // the margin at the probe.
    relocateSelf(bot, 44400, 50000)
    bot.ApplyMovement(state.Movement{
        ObjectID: 7, X: 45400, Y: 50000, Z: -3500,
        DestX: 45400, DestY: 50000, DestZ: -3500,
    })
    gap, held := loop.escapeNotOpening(base.Add(fleeGapWindow + time.Second))

    require.False(t, held, "an opening gap never ends the escape")
    require.GreaterOrEqual(t, gap, float64(1000))
    require.Equal(t, gap, loop.fleeGap, "the probe re-anchors")
}

// TestEscapeStallWatchdogReanchorsOnProgress pins the rolling
// window: a run that covered the escape distance re-anchors, so a
// long healthy chase is never mistaken for a dead escape.
func TestEscapeStallWatchdogReanchorsOnProgress(t *testing.T) {
    bot := newTestBot()
    loop := NewLoop(&fakeGame{}, bot)
    base := time.Now().Add(-time.Minute)

    loop.noteEscapeWalk(base)
    require.False(t, loop.escapeStalled(base),
        "the issued walk opens the observation window")
    relocateSelf(bot, 45200, 50000)
    require.False(t,
        loop.escapeStalled(base.Add(escapeNoProgressWindow)),
        "the covered window re-anchors")

    // The fresh window measures the character standing at its new
    // anchor: no further movement is a stall.
    require.True(t,
        loop.escapeStalled(base.Add(2*escapeNoProgressWindow)),
        "a stationary second window names the dead escape")
}

// TestSittingEscapeDoesNotTripTheStallWatchdog pins the stand-up
// grace: the flee of a sitting character spends its first seconds
// standing up, and a window without an issued walk must not read as
// a dead escape.
func TestSittingEscapeDoesNotTripTheStallWatchdog(t *testing.T) {
    bot := newTestBot()
    attackerHits(bot, 7, 45600)
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    base := time.Now().Add(-time.Minute)

    loop.fleeFromThreat(base)
    require.Empty(t, game.walks, "the stand request goes first")
    require.False(t,
        loop.escapeStalled(base.Add(escapeNoProgressWindow)),
        "the stand settle is not a stalled escape")
}
