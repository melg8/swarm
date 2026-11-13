// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import "time"

// The kite motion ledger answers "the archer does not run all the time, it eats
// damage" directly in the live log: every loop tick samples the motion state of
// the current fight and books the time into one bucket - MOVING, WINDUP (the
// server-forced bow standstill), HOLD (the cornered hold verdict) or IDLE (the
// unattributed standing the design exists to shrink). The fight-end summary
// carries the shares and the health cost, so a bad fight names its own problem
// (docs/hunting.md).

// kiteMotionMinFight bounds the ledger noise: a shorter fight books no summary,
// because the shares of a two-tick fight carry no evidence.
const kiteMotionMinFight = 2 * time.Second

// kiteLedgerTick samples the current fight and closes the ledger of a finished
// one: the kite layer gates the bookkeeping (a non-kite profile never arms it),
// a fresh target closes the previous ledger and opens its own, and a cleared
// target (the kill, the drop, the death) closes with the summary. The tick
// reads no position beyond the oracles the kite already reads.
func (l *Loop) kiteLedgerTick(now time.Time) {
    if l.target == 0 || !l.kite.Enabled || !l.bowEquipped() {
        l.kiteLedgerClose()

        return
    }
    if l.kiteMotionFor != l.target {
        l.kiteLedgerClose()
        l.kiteMotionFor = l.target
        l.kiteMotionAt = now
        l.kiteMotionMoving = 0
        l.kiteMotionWindup = 0
        l.kiteMotionHold = 0
        l.kiteMotionIdle = 0
        l.kiteMotionHPStart = l.tracker.SelfHealthPercent()
        l.kiteMotionHPMin = l.kiteMotionHPStart

        return
    }
    if l.kiteMotionAt.IsZero() {
        l.kiteMotionAt = now

        return
    }
    delta := now.Sub(l.kiteMotionAt)
    if delta <= 0 {
        return
    }
    l.kiteMotionAt = now
    if hp := l.tracker.SelfHealthPercent(); hp < l.kiteMotionHPMin {
        l.kiteMotionHPMin = hp
    }
    switch l.kiteLedgerKind(now) {
    case kiteMotionMoving:
        l.kiteMotionMoving += delta
    case kiteMotionWindup:
        l.kiteMotionWindup += delta
    case kiteMotionHold:
        l.kiteMotionHold += delta
    default:
        l.kiteMotionIdle += delta
    }
}

// kiteLedgerKind names the motion bucket of the current moment: a walking
// character is MOVING whatever walks it (the oracle cannot split the walker),
// a standing one inside the windup of its own fresh shot is WINDUP, a standing
// one on a fresh hold verdict is HOLD, and everything else is IDLE.
func (l *Loop) kiteLedgerKind(now time.Time) kiteMotionKind {
    if l.tracker.SelfWalking() {
        return kiteMotionMoving
    }
    if shotAt := l.tracker.SelfLastShotAt(); !shotAt.IsZero() {
        if clickAt := shotAt.Add(
            l.kiteWindupWindow(now) + kiteWindupLead); now.Before(clickAt) {
            return kiteMotionWindup
        }
    }
    if l.kiteHeldFor == l.target && !l.kiteHeldAt.IsZero() &&
        now.Sub(l.kiteHeldAt) < 2*kiteStepPeriod {
        return kiteMotionHold
    }

    return kiteMotionIdle
}

// kiteLedgerClose closes the ledger of the finished fight: the summary lands
// when the fight carried evidence, and the ledger resets whole.
func (l *Loop) kiteLedgerClose() {
    if l.kiteMotionFor == 0 {
        l.kiteLedgerReset()

        return
    }
    total := l.kiteMotionMoving + l.kiteMotionWindup +
        l.kiteMotionHold + l.kiteMotionIdle
    if total >= kiteMotionMinFight {
        movingShare := 100 * float64(l.kiteMotionMoving) /
            float64(total)
        l.logf("Hunt: kite motion ledger on %d: %.1fs moving of "+
            "%.1fs (%.0f%%), standing %.1fs (windup %.1fs, hold "+
            "%.1fs, idle %.1fs), health %.0f%% (worst %.0f%%)",
            l.kiteMotionFor,
            l.kiteMotionMoving.Seconds(), total.Seconds(),
            movingShare,
            (l.kiteMotionWindup + l.kiteMotionHold +
                l.kiteMotionIdle).Seconds(),
            l.kiteMotionWindup.Seconds(),
            l.kiteMotionHold.Seconds(),
            l.kiteMotionIdle.Seconds(),
            l.kiteMotionHPStart,
            l.kiteMotionHPMin)
    }
    l.kiteLedgerReset()
}

func (l *Loop) kiteLedgerReset() {
    l.kiteMotionFor = 0
    l.kiteMotionAt = time.Time{}
    l.kiteMotionMoving = 0
    l.kiteMotionWindup = 0
    l.kiteMotionHold = 0
    l.kiteMotionIdle = 0
    l.kiteMotionHPStart = 0
    l.kiteMotionHPMin = 0
}

// kiteMotionKind names the motion buckets of the ledger.
type kiteMotionKind int

const (
    kiteMotionMoving kiteMotionKind = iota
    kiteMotionWindup
    kiteMotionHold
    kiteMotionIdle
)
