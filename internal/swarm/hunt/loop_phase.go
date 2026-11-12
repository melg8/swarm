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
            started := time.Now()
            l.tickContext(ctx)
            // The tick duration feeds the statistics view
            // of the web UI: the loop cadence health of a
            // loaded process (NoteHuntTick averages and
            // bounds the window max, see state metrics).
            l.tracker.NoteHuntTick(time.Since(started))
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

// tickContext advances the hunt state machine by one decision. The phase
// transitions fall through, so a kill switches into looting and the
// first pickup happens on the same tick.
// Pre-consolidation phase debt; the hunt loop cleanup is planned
// (docs/quality_review_and_agent_prompts.md P07).
//
//nolint:cyclop,funlen,gocognit // the phase debt waits for the planned split
func (l *Loop) tickContext(ctx context.Context) {
    // Publish the hunt loop phase to the bot tracker so the web UI
    // can show the human readable activity banner. SetPhase is a
    // no-op when the phase has not changed, so the per tick call
    // never churns the event stream. Runs on every return path
    // through the defer. The closure captures l.phase by reference
    // so the value at return time is published (a plain defer call
    // evaluates its arguments at registration time). The shopping
    // view of the web UI widget rides the same defer: every tick
    // republishes the queue (SetShoppingPlan no-ops on an identical
    // view) so the widget never shows a stale plan whatever phase
    // the tick ended in. The same defer publishes the loop internals
    // for the diagnostics section of the state dump
    // (SetHuntDiagnostics stamps the heartbeat the encoders age
    // without a version bump). The stagnation watch rides the same
    // defer so it observes every tick whatever path the state
    // machine took (see stagnation.go).
    defer func() {
        now := time.Now()
        l.tracker.SetPhase(string(l.phase))
        l.publishShoppingView()
        l.tracker.SetHuntDiagnostics(l.diagnostics(now))
        l.observeStagnation(now)
    }()
    if l.tracker.SelfDead() {
        l.recoverFromDeath()

        return
    }
    // The emergency logout: critical health with the blows still
    // landing ends the session at once (one hit from death, the
    // run has nothing left to protect), while a social pile up -
    // several mobs already hold the character as their target -
    // starts the pile up run instead: the logout waits until the
    // character opened the escape distance from the aggro point.
    // The deleveling wants the deaths, a manual only session
    // never decides on its own, and a request already sent stays
    // one shot while the session unwinds. The armed run keeps
    // driving the logout even after the pack thins out on the
    // way: the escape distance, not the live mob count, ends it.
    // The winnable pile up is the exception: a healthy character
    // whose attackers sit inside the engage ceiling tanks the pack
    // down instead of throwing the fight away - the old unconditional
    // run, relogin and immediate walk home restarted the same aggro
    // cycle on the same spot (see pileUpWinnable).
    if !l.logoutDone && l.autonomous && l.phase != phaseDelevel {
        if l.tracker.SelfHealthPercent() < panicLogoutHealthPercent &&
            l.tracker.SelfUnderAttack() {
            l.emergencyLogout()

            return
        }
        if !l.panicAt.IsZero() {
            l.panicPileUpRun(time.Now())

            return
        }
        if l.tracker.SelfAttackerCount() >= panicLogoutAttackers &&
            !l.pileUpWinnable() && !l.settleHolding() {
            if l.kiteOwnsRetreat() && l.kiteFlightWanted() &&
                !l.kiteKillWindow() {
                // The standing kite turned unwinnable: the shot
                // cycle spends half of every reload inside the
                // melee range, so the pack grinds the health down
                // while the character waits for the bow. The
                // straight flight is the survival answer - the
                // fight is dropped and the away-ray opens the
                // ground (see dropFightForFlight). A target inside
                // the kill window is finished first: the flight
                // would throw the kill away and the pack trade of
                // two arrows is its price.
                l.dropFightForFlight(time.Now())

                return
            }
            if !l.kiteOwnsRetreat() && !l.kiteFlightOwnsEscape() {
                l.panicPileUpRun(time.Now())

                return
            }
        }
    }
    if l.logoutDone {
        return
    }
    // The manual commands of the web UI arrive asynchronously on the
    // bot tracker: drain them before the phase dispatch so a command
    // can interrupt the current phase.
    l.consumeUserCommands()
    if l.phase == phaseDelevel {
        l.tickDelevel()
        l.publishWalkPlan()

        return
    }
    if l.phase == phaseUser {
        l.tickUser()
        // The walk plan view follows the manual phase: the plan
        // publishes while a manual move runs, anything else clears
        // it (ClearWalkPlan is a no-op without a plan).
        l.publishWalkPlan()

        return
    }
    if !l.autonomous {
        // A manual only session never hunts on its own: the loop
        // waits in the idle phase for the next web command.
        l.tracker.ClearWalkPlan()
        l.phase = phaseIdle

        return
    }
    // The auto equipment runs in every phase of the hunt: the paperdoll
    // stays current while a town trip buys its gear and while the loot
    // drops arrive, so the combat stats never lag behind the inventory.
    // The class profile pick runs before it: a mystic class scores the
    // caster gear from its very first equip decision.
    l.maybePickGearProfile()
    l.maybeEquipGear()
    // The archer quiver follows the auto equipment: the bow the
    // planner placed shoots only with the arrows worn, and the ammo
    // is invisible to the planner (it scores zero - a consumable, not
    // gear), so the arming is the loop's own step right behind it.
    l.maybeArmQuiver()
    // The replaced starter kit follows the equips: the unsellable,
    // undroppable Squire's pieces leave the bag through the destroy
    // request as soon as their replacement is worn.
    l.maybeDestroyReplacedStarters()
    // The first cell pick serves every gate below it: the delevel
    // trigger reads the held ground's median (a nil leash reads an
    // empty live zone), the trip zone gate reads the patrol square.
    // A fresh session picks the cell before the delevel check runs,
    // so the outleveled spawn spot starts the guard walk on the very
    // first tick instead of losing the race to the engage (the 2026-
    // 09-20 delevel acceptance report: the mobs of the spot pulled
    // the farming first while the cell pick lagged a tick). The
    // rotation economy keeps its own place between the fights.
    if l.cell != nil && l.cell.picked < 0 {
        l.cellEvaluate(time.Now())
    }
    // The deleveling outranks the town trips and the hunt: the guard
    // deaths are the point of the phase - a shopping detour of a
    // character that plans to die buys nothing, and the mobs of the
    // outleveled spot pay no drops worth the walk delay (the level
    // gap collapsed them). The check runs ahead of the trip start,
    // so a full bag or a shopping plan never walks the character to
    // the shops instead of the guards (the 2026-09-20 report: the
    // fresh level 15 wallet shopped while the ground waited for the
    // deleveling).
    if l.delevelWanted() {
        l.startDelevel()
        l.tickDelevel()
        l.publishWalkPlan()

        return
    }
    // The city shopping errand blocks the whole tick: it owns every
    // move and transaction from the village walk to the return home
    // (see city_shopping.go).
    if l.maybeCityShopping(ctx) {
        return
    }
    if l.handleTownTrip() {
        return
    }
    // The non walking hunt phases clear the walk plan view: the
    // engage and loot phases have no planned path to draw.
    l.tracker.ClearWalkPlan()
    // The destroy cleanup runs outside the trips: everything the
    // merchant refuses is still better sold at the next shop than
    // destroyed on the way.
    l.cleanupInventory()
    l.maybeSwitchZone()
    if l.phase == phaseEngage {
        l.engage()
    }
    if l.phase == phaseLoot {
        l.loot()
    }
}

// handleTownTrip drives the town trip start and the running trip tick.
// It reports true when the tick was consumed by a town trip (already
// running or just started), so the caller skips the engage/loot logic.
// The town walk plan view publishes here too: while the trip walks to
// or from the merchant, the remaining geodata waypoints publish so the
// map draws the planned path. A real fight still holds the start (the
// loot of the kill is the point of the fight, the blows landing now
// need the answer), but a merely held engage target drops: the trip
// owns the target register the moment it starts (see dropAttackTarget).
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
