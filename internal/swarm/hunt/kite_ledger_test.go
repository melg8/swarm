// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kite motion ledger tests (issue #70, the round-14 feedback
// ask): the per-fight standing/moving accounting that answers "does
// the archer run all the time" directly - the bucket classification
// of one standing moment (the server-forced windup, the hold verdict,
// the unattributed idle) and the fight-end summary line with the
// moving share.

import (
    "bytes"
    "log"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestKiteLedgerKindNamesTheStandingReason pins the bucket
// classification of one standing moment: a character inside the
// windup of its own fresh shot stands on the SERVER'S order (the
// one standing the kite spec allows), a standing character past it
// with no hold verdict is the unattributed IDLE the redesign exists
// to shrink, and a fresh hold verdict names the pocket answer.
func TestKiteLedgerKindNamesTheStandingReason(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45200)
    // The own shot is fresh (the scene's swing broadcast), the
    // character never moved: the windup bucket.
    require.Equal(t, kiteMotionWindup, loop.kiteLedgerKind(time.Now()),
        "a fresh own shot with no movement books the windup")

    // The windup lapses with no hold verdict: the idle bucket.
    time.Sleep(1800 * time.Millisecond)
    require.Equal(t, kiteMotionIdle, loop.kiteLedgerKind(time.Now()),
        "a standstill past the windup with no verdict books the idle")

    // A fresh hold verdict: the hold bucket.
    loop.kiteHeldFor = 7
    loop.kiteHeldAt = time.Now()
    require.Equal(t, kiteMotionHold, loop.kiteLedgerKind(time.Now()),
        "a fresh hold verdict books the hold")

    // A walking character is MOVING whatever walk it runs (the
    // oracle cannot split the walker, the fight's walks own the
    // window by contract).
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45100, DestY: 50000, DestZ: -3500,
    })
    require.Equal(t, kiteMotionMoving,
        loop.kiteLedgerKind(time.Now()),
        "a walking character books the moving bucket")
}

// TestKiteLedgerClosePrintsTheMovingShare pins the fight-end summary
// line: the moving share of the fight and the standing split land in
// the log, and the close resets the ledger whole for the next fight.
func TestKiteLedgerClosePrintsTheMovingShare(t *testing.T) {
    _, _, loop := kiteBowBot(t, 45200)
    var buf bytes.Buffer
    loop.SetLogger(log.New(&buf, "", 0))
    loop.kiteMotionFor = 7
    loop.kiteMotionMoving = 6 * time.Second
    loop.kiteMotionWindup = 3 * time.Second
    loop.kiteMotionHold = 1 * time.Second
    loop.kiteLedgerClose()
    require.Contains(t, buf.String(), "kite motion ledger on 7",
        "the summary names the fight")
    require.Contains(t, buf.String(), "60%",
        "the summary carries the moving share (6 s of 10 s)")
    require.Contains(t, buf.String(), "windup 3.0s",
        "the summary splits the standing by its reason")
    require.Zero(t, loop.kiteMotionFor,
        "the close resets the ledger for the next fight")

    // A fight shorter than the evidence floor books no summary.
    buf.Reset()
    loop.kiteMotionFor = 7
    loop.kiteMotionMoving = 200 * time.Millisecond
    loop.kiteLedgerClose()
    require.Empty(t, buf.String(),
        "a two-tick fight carries no evidence, no summary line")
}

// TestKiteLedgerTickSwitchesTheFight pins the ledger lifecycle: the
// tick of a fresh target closes the previous fight's ledger and
// opens its own, and a cleared target (the kill, the drop) closes
// the open one.
func TestKiteLedgerTickSwitchesTheFight(t *testing.T) {
    _, _, loop := kiteBowBot(t, 45200)
    var buf bytes.Buffer
    loop.SetLogger(log.New(&buf, "", 0))
    loop.kiteLedgerTick(time.Now())
    require.Equal(t, int32(7), loop.kiteMotionFor,
        "the fight tick opens the ledger of the target")
    require.False(t, loop.kiteMotionAt.IsZero(),
        "the open ledger stamps its clock")

    // The target clears: the ledger closes with the summary.
    loop.target = 0
    loop.kiteMotionMoving = 3 * time.Second
    loop.kiteLedgerTick(time.Now())
    require.Zero(t, loop.kiteMotionFor,
        "the cleared target closes the ledger")
    require.Contains(t, buf.String(), "kite motion ledger on 7",
        "the close printed the summary of the finished fight")

    // A fresh target opens its own ledger whole.
    loop.target = 9
    loop.kiteLedgerTick(time.Now())
    require.Equal(t, int32(9), loop.kiteMotionFor)
    require.Zero(t, loop.kiteMotionMoving,
        "the fresh ledger starts empty")
}
