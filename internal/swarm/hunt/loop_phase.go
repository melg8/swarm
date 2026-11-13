// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "context"
    "time"
)

// phase is the coarse activity of the hunt loop.
type phase string

const (
    // phaseEngage walks to and attacks the current or nearest target.
    phaseEngage phase = "engage"
    // phaseLoot picks up the drops around the last kill.
    phaseLoot phase = "loot"
    // phaseTownWalk follows the geodata waypoints to the town shop.
    phaseTownWalk phase = "townWalk"
    // phaseTownSell sells the inventory junk at the shop merchant.
    phaseTownSell phase = "townSell"
    // phaseTownReturn follows the geodata waypoints back to the farm
    // spot.
    phaseTownReturn phase = "townReturn"
    // phaseDelevel dies at the town guards to lose the excess levels.
    phaseDelevel phase = "delevel"
    // phaseCityShop runs the blocking city shopping errand: the ride
    // to the city, the top kit purchase and the ride home.
    phaseCityShop phase = "cityShop"
    // phaseUser executes a manual command of the web UI (a map click
    // for a walk, an attack or a pickup) until it completes, then the
    // autonomous hunting resumes.
    phaseUser phase = "user"
    // phaseIdle waits for the next manual command of the web UI: the
    // manual only mode of a session started without -hunt never leaves
    // this phase, the loop exists purely to turn the queued commands
    // into world actions.
    phaseIdle phase = "idle"
)

// Run drives the hunt loop until the context is done.
func (l *Loop) Run(ctx context.Context) {
    ticker := time.NewTicker(tickPeriod)
    defer ticker.Stop()
    // The fleet claim of the hunted spot dies with this goroutine: a
    // session end (the emergency logout of the safety layer, a server
    // restart, a lost connection) unwinds the whole loop together
    // with its spotHunter, and the claim must not outlive it (see
    // spotHunter.releaseClaim).
    defer l.releaseCellClaim()
    // The emergency logout spots of the previous sessions count against
    // their zones from the first tick: the regression counters of this
    // loop instance start at zero, but the tracker carried the danger
    // over the relogin (see seedZoneDanger).
    l.seedZoneDanger()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            started := l.now()
            l.tickContext(ctx)
            // The tick duration feeds the statistics view
            // of the web UI: the loop cadence health of a
            // loaded process (NoteHuntTick averages and
            // bounds the window max, see state metrics).
            l.tracker.NoteHuntTick(l.now().Sub(started))
        }
    }
}

// releaseCellClaim hands the fleet occupancy claim of the hunted cell
// back to the hub when the loop stops. Idempotent: a second call (a
// stop after a cell release) is a no-op.
func (l *Loop) releaseCellClaim() {
    if l.cell != nil {
        l.cell.releaseClaim()
    }
}

// tick advances the hunt state machine by one decision outside a
// session context (the tests and the manual probes drive the loop
// one decision at a time; the blocking errands see no cancellation).
func (l *Loop) tick() {
    l.tickContext(context.Background())
}

// tickContext advances the hunt state machine by one decision. The
// phase transitions fall through, so a kill switches into looting and
// the first pickup happens on the same tick.
func (l *Loop) tickContext(ctx context.Context) {
    defer l.publishTickState()
    if l.tickDeathOrSafety() {
        return
    }
    if l.logoutDone {
        return
    }
    if l.tickCommandPhase() {
        return
    }
    if l.tickErrands(ctx) {
        return
    }
    l.tickHunt()
}

// publishTickState mirrors the tick result into the tracker: the phase
// banner, the shopping widget queue, the diagnostics heartbeat and the
// stagnation watch. The defer reads l.phase at return time, so every
// path of the tick publishes the phase it ended in.
func (l *Loop) publishTickState() {
    now := l.now()
    l.tracker.SetPhase(string(l.phase))
    l.publishShoppingView()
    l.tracker.SetHuntDiagnostics(l.diagnostics(now))
    l.observeStagnation(now)
}

// tickDeathOrSafety answers the death and the defensive layer: the
// village restart of a dead character, the critical health logout, the
// armed pile up run and the fresh social pile up. It reports true when
// the tick was consumed.
func (l *Loop) tickDeathOrSafety() bool {
    if l.tracker.SelfDead() {
        l.recoverFromDeath()

        return true
    }
    if l.logoutDone || !l.autonomous || l.phase == phaseDelevel {
        return false
    }
    if l.tracker.SelfHealthPercent() < panicLogoutHealthPercent &&
        l.tracker.SelfUnderAttack() {
        l.emergencyLogout()

        return true
    }
    if !l.panicAt.IsZero() {
        l.panicPileUpRun(l.now())

        return true
    }

    return l.tickPileUp(l.now())
}

// tickPileUp answers a social pile up: several mobs already hold the
// character as their target. A healthy character whose attackers sit
// inside the engage ceiling tanks the pack down, everything else starts
// the pile up run; a standing kite that turned unwinnable drops the
// fight for the straight flight. It reports true when the tick was
// consumed.
func (l *Loop) tickPileUp(now time.Time) bool {
    if l.tracker.SelfAttackerCount() < panicLogoutAttackers ||
        l.pileUpWinnable() || l.settleHolding() {
        return false
    }
    if l.kiteOwnsRetreat() && l.kiteFlightWanted() &&
        !l.kiteKillWindow() {
        l.dropFightForFlight(now)

        return true
    }
    if !l.kiteOwnsRetreat() && !l.kiteFlightOwnsEscape() {
        l.panicPileUpRun(now)

        return true
    }

    return false
}

// tickCommandPhase drains the queued web UI commands and runs the
// phases they own: the deleveling, the manual command and the idle hold
// of a manual only session. It reports true when the tick was consumed.
func (l *Loop) tickCommandPhase() bool {
    l.consumeUserCommands()
    if l.phase == phaseDelevel {
        l.tickDelevel()
        l.publishWalkPlan()

        return true
    }
    if l.phase == phaseUser {
        l.tickUser()
        l.publishWalkPlan()

        return true
    }
    if !l.autonomous {
        l.tracker.ClearWalkPlan()
        l.phase = phaseIdle

        return true
    }

    return false
}

// tickErrands runs the autonomous errands ahead of the hunt: the auto
// equipment, the first cell pick, the deleveling and the shopping
// trips. The deleveling outranks the trips (its deaths are the point of
// the phase), and a running trip owns every move until it returns. It
// reports true when the tick was consumed.
func (l *Loop) tickErrands(ctx context.Context) bool {
    l.maybePickGearProfile()
    l.maybeEquipGear()
    l.maybeArmQuiver()
    l.maybeDestroyReplacedStarters()
    if l.cell != nil && l.cell.picked < 0 {
        l.cellEvaluate(l.now())
    }
    if l.delevelWanted() {
        l.startDelevel()
        l.tickDelevel()
        l.publishWalkPlan()

        return true
    }
    if l.maybeCityShopping(ctx) {
        return true
    }

    return l.handleTownTrip()
}

// tickHunt runs the hunt itself: the inventory cleanup, the zone switch
// and the engage or loot step of the current phase.
func (l *Loop) tickHunt() {
    l.tracker.ClearWalkPlan()
    l.cleanupInventory()
    l.maybeSwitchZone()
    if l.phase == phaseEngage {
        l.engage()
    }
    if l.phase == phaseLoot {
        l.loot()
    }
}

// handleTownTrip drives the town trip start and the running trip
// tick, and reports true when the tick was consumed by a town trip so
// the caller skips the engage and loot logic. The walk plan publishes
// here too: while the trip walks to or from the merchant the remaining
// geodata waypoints publish for the map. A real fight still holds the
// start (the loot needs the answer), but a merely held engage target
// drops: the trip owns the target register from the start (see
// dropAttackTarget).
func (l *Loop) handleTownTrip() bool {
    if l.tripActive() {
        l.tickTownTrip()
        l.publishWalkPlan()

        return true
    }
    if l.fightBusy() {
        if l.phase == phaseLoot || l.tracker.SelfUnderAttack() {
            return false
        }
        // A merely held target falls through to the start attempt: the
        // drop decision belongs to maybeStartTownTrip (its trigger
        // gates decide whether the way changes hands at all).
    }
    l.maybeStartTownTrip()
    if !l.tripActive() {
        return false
    }
    l.tickTownTrip()
    l.publishWalkPlan()

    return true
}

// fightBusy reports whether the character is still bound to the fight
// it started: a selected living target, a pending loot pickup or a hit
// landing right now. The town trip start waits out a real fight (the
// loot, the blows) and drops a merely held target (see dropAttackTarget).
func (l *Loop) fightBusy() bool {
    if l.phase == phaseLoot {
        return true
    }
    if l.phase != phaseEngage {
        return false
    }
    if l.target != 0 {
        return true
    }

    return l.tracker.SelfUnderAttack()
}
