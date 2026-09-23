// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import "time"

// The kite motion ledger (issue #70, the round-14 feedback ask): the
// owner complaint "the archer does not run all the time, it eats
// damage" needs a measurement that answers it DIRECTLY, in the live
// log, at the moment the fight ends - not only in the offline fleet
// audit. The ledger samples the motion state of the current fight
// once per loop tick and books every second into exactly one bucket:
//
//   - MOVING: the character walks (the retreat, the pursuit chain,
//     the shove - any walk of the fight).
//   - WINDUP: the character stands inside the bow windup of its own
//     shot - the ~1.5 s standstill the SERVER forces (a
//     MoveToLocation inside it is saved and replayed at the cycle
//     end, see docs/kite_timing_findings.md); the only standing the
//     kite spec allows.
//   - HOLD: the character stands on a cornered/encircled hold
//     verdict (kiteHoldGround) - the pocket tiers found no lane.
//   - IDLE: the character stands in the fight with no verdict - the
//     unattributed standing the redesign exists to shrink (an
//     unanswered engage, a lost tick, a recovery pause).
//
// The fight-end summary line carries the shares and the health cost,
// so a bad fight names its own problem in one readable line.

// kiteMotionMinFight bounds the ledger noise: a fight shorter than
// this books no summary (a one-shot kill, a target switch inside a
// volley - the shares of a two-tick fight carry no evidence).
const kiteMotionMinFight = 2 * time.Second

// kiteLedgerTick samples the motion state of the current fight and
// closes the ledger of a finished one: the kite layer gates the
// bookkeeping (a non-kite profile never arms it), a fresh target
// closes the previous ledger and opens its own, and a cleared target
// (the kill, the drop, the death) closes the ledger with the
// summary. The tick is cheap (no position scan beyond the oracles
// the kite itself already reads) and runs on every loop tick of the
// fight path - the samples are the tick cadence the loop already
// paces.
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

// kiteLedgerKind names the motion bucket of the current moment: a
// walking character is MOVING whatever walks it (the ladder, the
// pursuit chain - the oracle cannot split the walker, the fight's
// walks own the kite window by contract), a standing one inside the
// windup of its own fresh shot is WINDUP (the server-forced
// standstill), a standing one on a fresh hold verdict is HOLD, and
// everything else is IDLE - the unattributed standing the summary
// names.
func (l *Loop) kiteLedgerKind(now time.Time) kiteMotionKind {
    if l.tracker.SelfWalking() {
        return kiteMotionMoving
    }
    if shotAt := l.tracker.SelfLastShotAt(); !shotAt.IsZero() {
        if clickAt := shotAt.Add(
            l.kiteWindupWindow() + kiteWindupLead); now.Before(clickAt) {
            return kiteMotionWindup
        }
    }
    if l.kiteHeldFor == l.target && !l.kiteHeldAt.IsZero() &&
        now.Sub(l.kiteHeldAt) < 2*kiteStepPeriod {
        return kiteMotionHold
    }

    return kiteMotionIdle
}

// kiteLedgerClose closes the ledger of the finished fight: the
// summary line lands when the fight ran long enough to carry
// evidence, and the ledger resets whole for the next one.
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

// kiteLedgerReset clears the ledger state.
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
