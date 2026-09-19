// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The systemic anti ping-pong guard of the deleveling: the owner
// report named bots that never complete the deleveling - every
// attempt aborts (a silent guard, a failed walk plan, the timeout),
// the bot returns to the farm spot and the flat one minute cooldown
// sends it straight back to the guards. The fix arms the escalating
// wait on every consecutive abort (delevelAbortWait), the streak
// resets only on a finished deleveling. These tests pin the ladder,
// the reset and the silence of the trigger while the wait holds.

// restartDelevelAfterAbort ends the return leg of a just aborted
// deleveling (the character walks home to the zone center) and waits
// for the trigger to start the next attempt: the wait is cleared and
// the end cooldown warped, the way the hours of farming would pass.
// The first tick ends the return trip (the arrival consumes it), the
// second tick runs the delevel check ahead of the engage.
func restartDelevelAfterAbort(t *testing.T, loop *Loop, bot *state.Bot) {
    t.Helper()
    loop.delevelWait = time.Time{}
    loop.delevelEnd = time.Now().Add(-2 * delevelCooldown)
    moveSelfTo(bot, 46112, 41500, -3500)
    loop.tick()
    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase,
        "the expired wait lets the trigger start again")
}

// abortRunningDelevel ages the running deleveling past its timeout:
// the deterministic abort path of the harness (no navigator games).
func abortRunningDelevel(t *testing.T, loop *Loop) {
    t.Helper()
    loop.tripStart = time.Now().Add(-delevelTimeout - time.Minute)
    loop.tick()
}

// TestDelevelAbortArmsEscalatingWait verifies the ladder: the first
// abort waits the base, every further consecutive abort multiplies
// the wait and the cap holds the maximum. While the wait holds, a
// trigger that is still true (the level stayed above the gap) must
// not start a new deleveling - the bot farms instead of commuting.
func TestDelevelAbortArmsEscalatingWait(t *testing.T) {
    loop, _, _, _ := newDelevelLoop(11)
    spawnZoneMobs(loop.tracker)

    // The deleveling starts and the timeout aborts it.
    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase)
    abortRunningDelevel(t, loop)
    require.Equal(t, 1, loop.delevelAborts)
    firstWait := loop.delevelAbortWait()
    require.Equal(t, delevelAbortBackoffBase, firstWait,
        "the first abort arms the base wait")
    require.False(t, loop.delevelCooldownOver(),
        "the abort wait holds the trigger")

    // The wait holds: the level is still above the trigger, the
    // gremlins are alive, yet no new deleveling starts while the
    // wait runs.
    for range 3 {
        loop.tick()
        require.NotEqual(t, phaseDelevel, loop.phase,
            "the abort wait keeps the bot off the guards")
    }

    // The second consecutive abort escalates: the wait multiplies.
    restartDelevelAfterAbort(t, loop, loop.tracker)
    abortRunningDelevel(t, loop)
    require.Equal(t, 2, loop.delevelAborts)
    secondWait := loop.delevelAbortWait()
    require.Equal(t,
        delevelAbortBackoffBase*delevelAbortBackoffFactor, secondWait,
        "the second consecutive abort multiplies the wait")
    require.Greater(t, secondWait, firstWait)

    // The third abort hits the cap: the wait cannot grow forever.
    restartDelevelAfterAbort(t, loop, loop.tracker)
    abortRunningDelevel(t, loop)
    require.Equal(t, 3, loop.delevelAborts)
    require.Equal(t, delevelAbortBackoffMax, loop.delevelAbortWait(),
        "the ladder caps at the maximum wait")

    // The armed wait of the last abort holds the trigger again.
    require.False(t, loop.delevelCooldownOver())
    loop.tick()
    require.NotEqual(t, phaseDelevel, loop.phase)
}

// TestDelevelFinishResetsAbortStreak verifies the reset: only a
// COMPLETED deleveling (the target reached) clears the abort streak -
// the mechanism proved itself, the next legitimate cycle (the level
// climbed back over the trigger) starts from the short pause again.
// A new start never resets the streak.
func TestDelevelFinishResetsAbortStreak(t *testing.T) {
    loop, _, bot, _ := newDelevelLoop(11)
    spawnZoneMobs(bot)

    // The first abort: the streak arms the base wait.
    loop.tick()
    abortRunningDelevel(t, loop)
    require.Equal(t, 1, loop.delevelAborts)
    require.False(t, loop.delevelCooldownOver())

    // The deleveling runs again and FINISHES at the target level.
    restartDelevelAfterAbort(t, loop, bot)
    require.Equal(t, 1, loop.delevelAborts,
        "a new start never resets the streak")
    bot.ApplyUserInfo(stateUserInfoLevel(9))
    loop.tick()
    require.Equal(t, phaseTownReturn, loop.phase,
        "the target level finishes the deleveling")
    require.Equal(t, 0, loop.delevelAborts,
        "the finished deleveling resets the streak")

    // The next abort starts from the first rung again.
    bot.ApplyUserInfo(stateUserInfoLevel(11))
    restartDelevelAfterAbort(t, loop, bot)
    abortRunningDelevel(t, loop)
    require.Equal(t, 1, loop.delevelAborts,
        "the streak restarted from zero")
    require.Equal(t, delevelAbortBackoffBase, loop.delevelAbortWait())
}

// TestDelevelDiagnosticsCarryTargetAndReason verifies the webui
// message fields: while the deleveling runs the published diagnostics
// carry the active flag, the target level, the start level and the
// median mob level of the held ground - the phase banner renders the
// "up to which level and why" message from them. The finish clears
// the message on the same tick the phase leaves.
func TestDelevelDiagnosticsCarryTargetAndReason(t *testing.T) {
    loop, _, bot, _ := newDelevelLoop(11)
    spawnZoneMobs(bot)

    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase)
    hunt := bot.Snapshot().Diagnostics.Hunt
    require.True(t, hunt.DelevelActive)
    require.Equal(t, int32(9), hunt.DelevelTarget,
        "gremlins are level 1, the target clamps at the Lucky level")
    require.Equal(t, int32(11), hunt.DelevelFromLevel,
        "the start level is the trigger evidence")
    require.Equal(t, int32(1), hunt.DelevelZoneMedian,
        "the median mob level of the held ground is the why")

    // The finish clears the message: the banner falls back to the
    // return walk text the same tick the phase leaves.
    bot.ApplyUserInfo(stateUserInfoLevel(9))
    loop.tick()
    require.Equal(t, phaseTownReturn, loop.phase)
    hunt = bot.Snapshot().Diagnostics.Hunt
    require.False(t, hunt.DelevelActive)
    require.Zero(t, hunt.DelevelTarget)
    require.Zero(t, hunt.DelevelFromLevel)
    require.Zero(t, hunt.DelevelZoneMedian)
}

// stateUserInfoLevel builds the UserInfo refresh of the test
// character at the given level (the exp of the level bottom): the
// same shape the delevel tests apply for the death penalties.
func stateUserInfoLevel(level int32) state.UserInfo {
    return state.UserInfo{
        Name: "unittest1", Level: level, Race: 1, ClassID: 18,
        Exp: 50000,
        X:   45000, Y: 50000, Z: -3500,
        MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
        MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
        MoveSpeedMult: 1,
    }
}
