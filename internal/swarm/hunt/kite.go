// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kiting layer of the archer fight: while the fight runs, a hostile
// inside the retreat radius makes the character step away, the movement
// window pauses the forced attack re-requests, and the engage re-requests
// the attack once the step finished - the bow shoots again from the
// opened distance it just bought.
//
// Three rules shape every layer below. The retreat runs one straight
// away-ray and never turns (a turn spends the distance the step bought);
// a blocked lane holds the ground and keeps shooting the bow; and the
// layer never arms against a bow-wielding enemy, which cannot be outrun.
// The shot-paced layer reads the character's own Attack broadcast as the
// server commit of the shot, so the retreat starts inside the bow
// cooldown instead of after the mob closed the range.
//
// The design, the round history and the live measurements live in
// docs/hunting.md (the archer kite section and its subsections).

import (
    "math"
    "time"
)

// The kiting constants of the archer fight.
const (
    // kiteRetreatRadius arms the step: a hostile closer than this is
    // about to reach melee (the swing range sits near 150, a chaser
    // runs 120+ units per second).
    kiteRetreatRadius = 250.0
    // kiteStep is the retreat length of one walk, sized so a completed
    // walk lands back inside the optimal band.
    kiteStep = 400.0
    // kiteStepWindow is the movement window one step owns: a forced
    // attack request interrupts a running walk server-side, so the
    // engage holds its re-requests until the window closes.
    kiteStepWindow = 2 * time.Second
    // kiteReengageDelay holds the first forced attack re-request after
    // the walk. Zero today - the window end IS the re-engage.
    kiteReengageDelay time.Duration = 0
    // kiteStepPeriod paces the steps and the cornered hold re-probes.
    kiteStepPeriod = 3 * time.Second
    // kiteStreakLimit is the diagnostic threshold of an unwinnable
    // race: a chaser at least as fast never falls behind, the counter
    // names it, and the retreat keeps running.
    kiteStreakLimit = 8
    // kiteTrainScanRange bounds the chaser scan of the retreat
    // direction: a farther member cannot reach the character inside a
    // step window.
    kiteTrainScanRange = 800.0
    // kiteEncircleShare is the encirclement threshold: when the summed
    // away vectors shrink under this share of the chaser count (they
    // stand on every side), the widest-gap bisector takes the step.
    kiteEncircleShare = 0.3
    // kiteMaxTrainMembers caps the bearing scratch; the array stays
    // fixed to keep the resolution allocation free.
    kiteMaxTrainMembers = 16
    // kiteHalfPlaneSlack lets a lane run perpendicular to the away-ray
    // but never fold back toward the train.
    kiteHalfPlaneSlack = -0.05
    // kiteRoamRadius fences the retreat around the fight anchor (the
    // target position at the first retreat resolution): three bow
    // engage radii, location-general, no zone table.
    kiteRoamRadius = 3 * userBowEngageRadius
    // kiteMinStep is the shortest step the ring budget may leave; the
    // lane gate refuses even that when it is walled.
    kiteMinStep = 250.0
    // kiteHoldLogPeriod paces the cornered hold diagnostic line.
    kiteHoldLogPeriod = 15 * time.Second
    // kiteShotWindow is the freshness window of the character's own
    // Attack broadcast, the server commit of the shot. It keeps the
    // broadcast catchable for at least two ticks at the quarter-second
    // loop cadence while bounding the schedule to its own shot.
    kiteShotWindow = 700 * time.Millisecond
    // kiteReclickPeriod paces the re-click ladder of the retreat walk
    // while the character still stands on the issue cell. It sits
    // under the server's once-per-second movement broadcast gate, so a
    // re-click re-aims the same endpoint and is never wasted.
    kiteReclickPeriod = 250 * time.Millisecond
    // kiteReclickLimit bounds the re-clicks of one walk (the initial
    // click plus this many re-issues): the packet budget of a dead
    // transport.
    kiteReclickLimit = 8
    // kiteProbeElapsed is the silent-verdict age: the initial click
    // and every re-click stayed unanswered while the character stands
    // on the issue cell (no ActionFailed, no movement broadcast), so
    // the endpoint is dead. It sits past the once-per-second
    // broadcast gate plus the pacing headroom.
    kiteProbeElapsed = 1200 * time.Millisecond
    // kiteRefusalAnswerWindow bounds the ActionFailed attribution of a
    // kite click: a genuine refusal answers within the round trip, and
    // the server AI's own attack retries bounce at a one-second
    // cadence through the whole bow disable - a wide window would
    // latch healthy endpoints dead.
    kiteRefusalAnswerWindow = 250 * time.Millisecond
    // kiteBowRetryLead is the look-back band of the bow retry: an
    // attack refused during the disable schedules its re-try a second
    // ahead (Creature.canUseRangeWeapon) and its ActionFailed lands
    // with no client request inside the tight answer window.
    kiteBowRetryLead = 1350 * time.Millisecond
    // kiteBowReuseDelay is the reuse delay of the bow family the fleet
    // shoots (the C1 item data: the Short Bow and the D-grade bows
    // carry 1500); it feeds the bow disable formula of kiteWalkWindow.
    kiteBowReuseDelay = 1500.0
    // kiteWalkWindowFloor and kiteWalkWindowMax clamp the bow-aware
    // walk window: the formula reads the live pAtkSpd, and a broken
    // broadcast must never collapse or explode the window. Outside the
    // band the shipped kiteStepWindow applies.
    kiteWalkWindowFloor = 1 * time.Second
    kiteWalkWindowMax   = 5 * time.Second
    // kiteGaugeMaxAge bounds how long a RED SetupGauge still counts as
    // the bow cycle of the shot in flight: the server sends one on
    // every commit, so an older gauge belongs to a fight that ended.
    kiteGaugeMaxAge = 10 * time.Second
    // kiteWindupLead is the safety lead the deferred retreat click
    // adds past the windup end: the accepted move window opens at the
    // (timeAtk+reuse)/2 boundary, and the lead must clear it despite
    // the quarter-second loop cadence and the broadcast lag.
    kiteWindupLead = 150 * time.Millisecond
    // kiteRingLeashShare caps the straight step as a share of the roam
    // radius: the ring keeps the whole fight band inside the 1500 unit
    // audit drift leash from the anchor.
    kiteRingLeashShare = 0.72
    // kiteArrivalEpsilon is the arrival radius of the re-click ladder:
    // a stand inside it completes the step, a stand beyond it re-arms
    // the episode (the server cell granularity and a partial route
    // never leave the character exactly on the clicked cell).
    kiteArrivalEpsilon = 100.0
)

// KiteParams is the tunable block of the kite fight: the four knobs
// the launch config carries per bot spec (launch_config.go), plus the
// enable gate. Every other kite constant stays a package constant -
// only these four are tuned between measurement runs.
type KiteParams struct {
    // Enabled gates the kite layer whole: false turns every bow fight
    // into the standing archer the baseline measures (the mob closes,
    // the archer stands and shoots - the melee fallback rate of the
    // baseline comes from exactly this).
    Enabled bool
    // RetreatRadius is the trigger distance of the kite step and the
    // inner edge of the optimal band (kiteRetreatRadius is the
    // shipped value).
    RetreatRadius float64
    // Step is the retreat length away from the closed threat
    // (kiteStep is the shipped value).
    Step float64
    // StepPeriod paces the steps and the cornered hold re-probes
    // (kiteStepPeriod is the shipped value).
    StepPeriod time.Duration
    // ReengageDelay is the hold between the step walk ending and the
    // first forced attack re-request (kiteReengageDelay is the
    // shipped value).
    ReengageDelay time.Duration
}

// DefaultKiteParams returns the shipped tuning: the constants block
// above read into the params shape.
func DefaultKiteParams() KiteParams {
    return KiteParams{
        Enabled:       true,
        RetreatRadius: kiteRetreatRadius,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: kiteReengageDelay,
    }
}

// SetKiteParams overrides the tunable block of the loop. A
// non-positive value falls back to the shipped one (a broken config
// line degrades to the shipped tuning); the re-engage delay keeps a
// zero, which is legitimate. The pacing and the streak survive the
// override.
func (l *Loop) SetKiteParams(p KiteParams) {
    if p.RetreatRadius <= 0 {
        p.RetreatRadius = kiteRetreatRadius
    }
    if p.Step <= 0 {
        p.Step = kiteStep
    }
    if p.StepPeriod <= 0 {
        p.StepPeriod = kiteStepPeriod
    }
    if p.ReengageDelay < 0 {
        p.ReengageDelay = kiteReengageDelay
    }
    l.kite = p
}

// closedDistance bounds how close a chaser may stand NOW from the
// distance of its last movement broadcast: the server sends a
// chasing mob's MoveToPawn at a one to two second cadence, so the
// raw gap can lag a whole close-in behind the live mob. The bound
// assumes the chaser ran for the whole age at its fastest speed, so
// the result is never above the true gap - the floor decisions of the
// kite may only ever UNDER-estimate the distance.
func closedDistance(
    dist float64, age time.Duration, speed float64,
) float64 {
    if age <= 0 || speed <= 0 {
        return dist
    }
    closed := dist - age.Seconds()*speed
    if closed < 0 {
        return 0
    }

    return closed
}

// kiteClosedDistance applies the closure bound to one object: the raw
// distance of its last movement broadcast, aged by the movement start
// and shortened by the speed it can run. A standing object, a missing
// stamp and a scene without the track answer the raw distance.
func (l *Loop) kiteClosedDistance(
    now time.Time, objectID int32, dist float64,
) float64 {
    track, ok := l.tracker.ObjectTrackOf(objectID)
    if !ok || !track.Moving || track.MovedAt.IsZero() {
        return dist
    }

    return closedDistance(dist, now.Sub(track.MovedAt), track.Speed)
}

// kiteThreat locates the hostile that arms the kite step: the fight
// target or the nearest chasing attacker, whichever sits closer. A mob
// on a deck the walk cannot reach (deckReachableZ) is no melee threat.
// The distance is the conservative closed distance (kiteClosedDistance)
// because every floor decision reads it; the direction weighs the
// whole train separately (kiteTrainDirection).
func (l *Loop) kiteThreat(
    now time.Time, selfX, selfY, selfZ int32,
) (id int32, dist float64, ok bool) {
    consider := func(candidate int32, x, y int32) {
        raw := math.Hypot(
            float64(selfX-x), float64(selfY-y))
        closed := l.kiteClosedDistance(now, candidate, raw)
        if !ok || closed < dist {
            id, dist, ok = candidate, closed, true
        }
    }
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        consider(l.target, tx, ty)
    }
    if attacker, aok := l.tracker.NearestAttacker(); aok &&
        attacker.ObjectID != l.target {
        if track, tok := l.tracker.ObjectTrackOf(
            attacker.ObjectID); tok {
            if track.Z == 0 ||
                math.Abs(float64(track.Z-selfZ)) <= deckReachableZ {
                consider(attacker.ObjectID, track.X, track.Y)
            }
        }
    }

    return id, dist, ok
}

// kiteLayerGates covers the gates every kite layer shares: the
// params triple (the enabled profile, the bow in hand, the fight
// target), the movement window (a running step or an impending-add
// walk owns it - no second walk may interrupt it) and the cornered
// hold pacing (a retreat with no walkable lane re-probes at the
// kite period, not every tick). Reports whether a kite step of any
// layer may run now.

func (l *Loop) kiteLayerGates(now time.Time) bool {
    // The params gate comes first: a kite-disabled profile (the
    // standing-archer baseline of issue #29) never arms the layer,
    // whatever the bow and the fight report.
    if !l.kite.Enabled || !l.bowEquipped() || l.target == 0 {
        return false
    }
    // The enemy weapon gate (docs/hunting.md): against an archer the
    // kite never engages - a ranged enemy cannot be outrun, so the
    // standing trade is the answer. Only the retreat is off.
    if l.tracker.ObjectUsesBow(l.target) {
        return false
    }
    if now.Before(l.combatAvoidUntil) {
        return false
    }
    if !l.kiteHeldAt.IsZero() && l.kiteHeldFor == l.target &&
        now.Sub(l.kiteHeldAt) < l.kite.StepPeriod {
        return false
    }

    return true
}

// kiteOwnsRetreat reports whether the archer kite owns the aggro
// answer of the current fight: enabled, bow in hand, a fight target
// that is not itself an archer. While it holds, the melee escape
// stands down - its fan is clamped to the hunting zone, which a
// drifted kite fight leaves, and the kite lane battery validates the
// real world instead (docs/hunting.md).
func (l *Loop) kiteOwnsRetreat() bool {
    return l.kite.Enabled && l.target != 0 && l.bowEquipped() &&
        !l.tracker.ObjectUsesBow(l.target)
}

// kiteLayerArmed reports whether the archer kite owns the defensive
// answer of this character: the profile enables the layer and the
// bow is in hand. The melee escape fan is never a fallback of it
// (see fleeFromThreat).
func (l *Loop) kiteLayerArmed() bool {
    return l.kite.Enabled && l.bowEquipped()
}

// kiteFlightWanted reports whether the straight flight could own the
// aggro answer right now: the layer is armed and the nearest attacker
// is a melee chaser. Unlike kiteFlightOwnsEscape the fight target may
// still be held - the pile up gate reads it to convert the unwinnable
// kite into the flight.
func (l *Loop) kiteFlightWanted() bool {
    if !l.kiteLayerArmed() {
        return false
    }
    attacker, ok := l.tracker.NearestAttacker()

    return ok && !l.tracker.ObjectUsesBow(attacker.ObjectID)
}

// kiteKillWindow reports whether the held target is one or two arrows
// from death (pileUpKillWindowPercent): a pile up never throws such a
// kill away.
func (l *Loop) kiteKillWindow() bool {
    if l.target == 0 {
        return false
    }
    hp := l.tracker.ObjectHealthPercent(l.target)

    return hp > 0 && hp <= pileUpKillWindowPercent
}

// kiteFlightOwnsEscape reports whether the targetless straight flight
// owns the aggro answer: the flight is wanted and the losing verdict
// already dropped the target. The pile up gate stands down then - the
// flight is the survival answer the zoned melee fan cannot give.
func (l *Loop) kiteFlightOwnsEscape() bool {
    return l.target == 0 && l.kiteFlightWanted()
}

// dropFightForFlight hands the aggro answer to the straight bow
// flight: the held target is dropped and skipped (the fleeFromTarget
// contract) and the targetless flight walks the away-ray. The pile up
// gate calls it when the standing kite turned unwinnable - the shot
// cycle stands half of every reload inside the melee range, so a pack
// that outnumbers the arrows grinds the health down.
func (l *Loop) dropFightForFlight(now time.Time) {
    if l.target != 0 {
        l.fleeFromTarget(l.target, now)

        return
    }
    l.fleeFromThreat(now)
}

// avoidBowTargets reports whether the hunt target search fences bow
// wielding mobs out: the archer profile never initiates a fight with
// an archer, because the standing trade is a break-even race it loses
// the moment a melee add joins (docs/hunting.md). An archer that
// attacks the character is still answered through attackerEngageable,
// never through this pick.
func (l *Loop) avoidBowTargets() bool {
    return l.kite.Enabled && l.bowEquipped()
}

// kiteFlightHolds reports whether a mob the loop just fled still
// holds the character as its target: the flight owns the aggro answer
// until the server clears that chase. AttackableAI MAX_ATTACK_TIMEOUT
// (1200 game ticks of 100 ms) drops the aggro of a mob that cannot
// reach its target two minutes after the last swing, so the flight is
// bounded. A fresh attacker that joins rides the same flight - the
// shot cycle spends half of every reload inside the melee range.
func (l *Loop) kiteFlightHolds(now time.Time) bool {
    if !l.kiteLayerArmed() {
        return false
    }
    for _, attacker := range l.tracker.SelfAttackers() {
        if l.targetSkipped(attacker.ObjectID, now) {
            return true
        }
    }

    return false
}

// kiteFleeBusy reports whether the flight's own movement state owns
// the tick: a live walk window (the re-click ladder keeps it) or a
// movement hold another step armed. A lapsed window stands down and
// reports free, so the next step issues on this very tick.
func (l *Loop) kiteFleeBusy(now time.Time) bool {
    if !l.kiteWalkUntil.IsZero() {
        if now.After(l.kiteWalkUntil) {
            l.kiteWalkClear()
        } else {
            l.kiteReclickWalk(now)

            return true
        }
    }

    return now.Before(l.combatAvoidUntil)
}

// kiteFleeHeld reports whether the last flight step found no lane:
// the hold paces the re-probe at the kite period and keeps the ticks
// in between. Only the targetless hold (kiteHeldFor == 0) belongs to
// the flight; a hold of a live target names its own fight.
func (l *Loop) kiteFleeHeld(now time.Time) bool {
    return l.kiteHeldFor == 0 && !l.kiteHeldAt.IsZero() &&
        now.Sub(l.kiteHeldAt) < l.kite.StepPeriod
}

// kiteFlightMovable reports whether the bow may start a movement right
// now: the server SAVES a MoveToLocation that lands inside the bow
// windup and replays it only at the disable end, so a click inside the
// windup travels seconds later, and the escape stall watchdog reads
// those saved seconds as a dead click and logs the session out. A
// gauge that is absent, malformed or stale owns no live cycle.
func (l *Loop) kiteFlightMovable(now time.Time) bool {
    at, cycle := l.tracker.SelfBowCycle()
    if at.IsZero() || cycle <= 0 || now.Before(at) ||
        now.Sub(at) > kiteGaugeMaxAge {
        return true
    }
    if cycle < kiteWalkWindowFloor || cycle > kiteWalkWindowMax {
        return true
    }

    return !now.Before(at.Add(cycle/2 + kiteWindupLead))
}

// kiteFleeWait reports whether a movement owner keeps the flight on
// the ground this tick: a deferred click of a finished fight (the loot
// interrupt drops the target a second after the shot, and its disable
// hold would keep the whole flight standing), the busy window, the
// walled-lane hold, the bow windup (kiteFlightMovable) or the stand
// request - the first walk must land on a movable character.
func (l *Loop) kiteFleeWait(now time.Time) bool {
    if !l.kiteClickAt.IsZero() && l.kiteClickFor != l.target {
        l.kiteClickDrop()
    }

    return l.kiteFleeBusy(now) || l.kiteFleeHeld(now) ||
        !l.kiteFlightMovable(now) || !l.standUpGuarded(now)
}

// kiteFleeOtherDeck reports whether the attacker sits on another
// deck: its swings cannot land and the straight lane crosses the
// geodata wall - the hunt owns that chase.
func kiteFleeOtherDeck(attackerZ, selfZ int32) bool {
    return attackerZ != 0 &&
        math.Abs(float64(attackerZ-selfZ)) > deckReachableZ
}

// kiteFleeThreat answers a defensive flee with the straight bow
// flight instead of the melee fan: the fan is clamped to the current
// hunting zone, and a kite fight drifting across the cell boundaries
// collapsed it until the closure watchdog logged the session out. The
// flight runs the away-ray every kite layer computes, at the full
// escape segment, and keeps re-clicking through the ladder. A walled
// lane stamps the hold for one kite period and hands the ticks to the
// fan; a bow-wielding attacker closes the flight (it cannot be
// outrun).
//
// The flight bypasses the fan's budgets on purpose: a chaser that
// keeps pace is cleared by the server's own attack timeout
// (AttackableAI MAX_ATTACK_TIMEOUT, two minutes without a landed
// hit), and the critical-HP logout at the tick top stays armed above
// both. Reports whether the flight spent the tick.
func (l *Loop) kiteFleeThreat(now time.Time) bool {
    if !l.kiteLayerArmed() {
        return false
    }
    attacker, ok := l.tracker.NearestAttacker()
    if !ok || l.tracker.ObjectUsesBow(attacker.ObjectID) {
        return false
    }
    if l.kiteFleeWait(now) {
        return true
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    if kiteFleeOtherDeck(attacker.Z, selfZ) {
        return false
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if dirX == 0 && dirY == 0 {
        // No away bearing at all: the chaser stands ON the character
        // (kiteContactAway) or its position has not broadcast yet.
        // Standing in contact is death - the ring rays answer instead
        // of the silent tick the run-29 dump spent 21 seconds on.
        var contact bool
        dirX, dirY, contact = l.kiteContactAway(selfX, selfY, selfZ)
        if !contact {
            // Even the ring is walled: hold the ground and re-probe at
            // the kite period (the critical-HP logout stays armed).
            l.kiteHoldGround(now, encircled)

            return true
        }
    }
    until := now.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
    stepX, stepY, found := l.kiteRetreatLane(
        selfX, selfY, selfZ, dirX, dirY, dirX, dirY,
        escapeWalkDistance)
    breakout := false
    if !found {
        // The away side is a dead end (a corner pocket, a wall
        // behind the flight): break out through the back half-plane
        // - the open ground the character came from, THROUGH the
        // chaser's bearing - instead of handing the tick to the
        // zone-clamped fan and its logout (the 2026-11-10 run-24
        // dump: 30 s of healthy flight ended in a north-west pocket
        // and the fan reset the aggro at 99 percent health).
        stepX, stepY, found = l.kiteRetreatLane(
            selfX, selfY, selfZ, -dirX, -dirY, -dirX, -dirY,
            escapeWalkDistance)
        breakout = found
    }
    if !found {
        // A full pocket: hold the ground and re-probe at the kite
        // period. The flight keeps the tick - the critical-HP logout
        // of the tick top stays armed above it.
        l.kiteHoldGround(now, encircled)

        return true
    }
    l.kiteHeldAt = time.Time{}
    l.kiteIssueWalk(now, until, selfX, selfY, selfZ, stepX, stepY)
    l.noteEscapeWalk(now)
    l.fleeSince = time.Time{}
    if breakout {
        l.logger.Printf("Hunt: the away lane from %s is walled, "+
            "breaking out through the open back lane (step %d)",
            attacker.Name,
            int(math.Round(math.Hypot(
                float64(stepX-selfX), float64(stepY-selfY)))))

        return true
    }
    l.logger.Printf("Hunt: %s holds the bow flee at %d units, "+
        "kiting clear (step %d)", attacker.Name,
        int(math.Round(math.Hypot(
            float64(selfX-attacker.X),
            float64(selfY-attacker.Y)))),
        int(math.Round(math.Hypot(
            float64(stepX-selfX), float64(stepY-selfY)))))

    return true
}

// kiteFightLive reports whether a fight is actually live around the
// character: the selected target is engaged by either side (the own
// fresh swing or chase step, or the target holding the character as
// its victim - SelfEngagedBy), or another mob already holds the
// character as its target (a train member grinding the retreat).
// The kite triggers read this instead of the strict SelfFighting
// view: a long retreat leg spends seconds walking without a swing,
// and the strict view went stale exactly then, standing the whole
// kite layer down mid-cycle (the 2026-11-09 live run). An ordinary
// approach to a peaceful pick is not a live fight and never fires
// the retreat.

func (l *Loop) kiteFightLive() bool {
    if l.target == 0 {
        return false
    }
    if l.tracker.SelfEngagedBy(l.target) {
        return true
    }

    return l.tracker.SelfAttackerCount() > 0
}

// kiteStepAdmitted guards the kite step of one tick: the shared
// layer gates must pass (see kiteLayerGates), the kite pacing must
// have aged out (the step re-probe), and a fresh target resets the
// streak of the previous one (the distance race of one mob is not
// the race of the next). The round-14 always-run redesign removed
// the old streak-limit hard stop ("stop the shuffle, fight it
// out"): the standing trade is the melee damage the kite exists to
// avoid, so the proximity step keeps answering a closed hostile
// for as long as the fight runs - the streak counter crosses
// kiteStreakLimit only as the diagnostic that names the unwinnable
// race. Reports whether the step may run now.

func (l *Loop) kiteStepAdmitted(now time.Time) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    if !l.kiteAt.IsZero() && now.Sub(l.kiteAt) < l.kite.StepPeriod {
        return false
    }
    if l.kiteFor != l.target {
        // A fresh target: the streak of the previous one is history.
        l.kiteFor = l.target
        l.kiteStreak = 0
    }
    if l.kiteStreak == kiteStreakLimit {
        // The diagnostic crossing (exactly once per crossing): the
        // race is unwinnable, the retreat keeps running anyway - a
        // same-speed chase outwaits the mob's home leash while a
        // standing one eats the health bar.
        l.logf("Hunt: the distance race on %d is unwinnable, "+
            "running it out straight (step %d)",
            l.target, l.kiteStreak)
    }

    return true
}

// kiteFromTarget steps a bow fighting character away from the hostile
// that armed the step (kiteThreat), buying the weapon range back
// instead of tanking the melee. The direction is the straight away-ray
// of the whole chaser train (kiteTrainDirection) and it is the only
// lane: a turn would spend the distance the step just bought. When the
// train surrounds the character or the lane is blocked, the cornered
// hold keeps the bow shooting at melee range - never a weapon switch.
// The timing follows the live shot cycle (kiteShotPhase): the click
// defers past the windup, goes out at once inside the accepted move
// window, and uses the full window when no cycle is live. Reports
// whether the tick armed the step - the walk owns the movement then.

func (l *Loop) kiteFromTarget(now time.Time) bool {
    if !l.kiteStepAdmitted(now) {
        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(now, selfX, selfY, selfZ)
    if !ok || dist >= l.kite.RetreatRadius {
        return false
    }
    clickAt, until, live := l.kiteShotPhase(now)
    if live && now.Before(clickAt) {
        // The shot cycle holds its windup: an immediate click would
        // defer to the disable end and die at the re-shot (the
        // measured deferral of the issue #70 findings). The pressure
        // trigger rides the same deferred schedule the shot-paced
        // rhythm arms - one click at the boundary, the whole reload
        // tail of walking.
        if l.kiteArmClick(clickAt, until) {
            l.logger.Printf("Hunt: hostile %d closed to %d units "+
                "of the fight on %d inside the windup, the retreat "+
                "waits the windup end", threatID,
                int(math.Round(dist)), l.target)

            return true
        }

        return false
    }
    if !live {
        // No shot cycle owns the moment (the re-engage wait, a
        // chase without a broadcast): the ordinary full window.
        until = now.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered (see
        // kiteResolveAndClick): the hold ground rule owns the cycle.
        return false
    }
    l.kiteStreak++
    l.logger.Printf("Hunt: hostile %d closed to %d units of the "+
        "fight on %d, kiting clear (step %d, the unwinnable-race "+
        "diagnostic bound %d)",
        threatID, int(math.Round(dist)), l.target,
        l.kiteStreak, kiteStreakLimit)

    return true
}

// kiteThreatBeyondThePursue reports whether the threat left the
// pursue band with no live attack behind it (the round-14 fleet
// duel finding): a hostile beyond the bow engage radius that is NOT
// an attacker (a fleeing melee mob, a target standing off) is the
// stall watchdog's re-approach case, while a LIVE ATTACKER beyond
// the band - the measured ranged duel, a Kaboo shooter holding 474
// units and trading arrows - keeps the shot-paced rhythm moving:
// the standing answer ate 21 s of idle trading in one measured
// window while the melee adds closed on the stationary archer. The
// strafe rides the same straight retreat the rhythm always runs
// (the distance holds near the bow's own max), capped at the train
// scan range - beyond it neither side reaches the other and the
// rhythm has nothing to pace.

func (l *Loop) kiteThreatBeyondThePursue(
    threatID int32, dist float64,
) bool {
    if dist < userBowEngageRadius || dist >= kiteTrainScanRange {
        return dist >= kiteTrainScanRange
    }
    for _, attacker := range l.tracker.SelfAttackers() {
        if attacker.ObjectID == threatID {
            // A live attacker beyond the band (the ranged duel)
            // still rides the rhythm.
            return false
        }
    }

    return true
}

// kiteFromShot arms the shot-paced retreat of the bow fighting
// character (issue #60, reworked on the issue #70 findings): the
// Attack broadcast of the character is the server commit of the bow
// shot - the hit roll, the arrow consumption and the HitTask
// schedule all happened before the packet, the damage task carries
// no attacker movement check - but the measured server cycle FORBIDS
// the movement through the first half of the bow disable window (the
// windup): a MoveToLocation inside it is saved by the server AI and
// replayed only at the disable end, where the re-shot cancels it
// (PlayerAI.setIntentionMoveTo saves the intention, the replay rides
// notifyActionReadyToAct - see docs/kite_timing_findings.md). The
// broadcast therefore arms the DEFERRED retreat (kiteArmClick): the
// click waits the windup end plus the safety lead, then fires
// through kiteClickWalk into the accepted move window and the walk
// banks the reload tail - the shoot, run the reuse tail, shoot again
// the moment the bow is ready cycle the owner asks for. The retreat
// fires once per shot while a hostile holds the pursue band (inside
// the bow engage radius - a mob that keeps chasing the fight), the
// straight lane machinery (the train direction, the leash, the wall
// and the water gates, the cornered hold) resolves at the click
// moment - the chasers move while the character stands the windup
// out - and the rhythm does NOT count toward the streak limit: the
// shots never starve. A hostile beyond the band leaves the standing
// fight (the stall watchdog owns the re-approach). Reports whether
// the tick armed the deferred retreat.

func (l *Loop) kiteFromShot(now time.Time) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    // The trigger itself: the character's own shot, fresh inside the
    // broadcast window.
    shotAt := l.tracker.SelfLastShotAt()
    if shotAt.IsZero() || now.Sub(shotAt) > kiteShotWindow {
        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(now, selfX, selfY, selfZ)
    if !ok || l.kiteThreatBeyondThePursue(threatID, dist) {
        return false
    }
    clickAt, until, live := l.kiteShotPhase(now)
    if !live {
        // The disable window cannot lapse while the broadcast is
        // fresh (the window outlasts the freshness gate on every
        // clamped speed); the guard keeps a degenerate broadcast
        // (a zero pAtkSpd tearing the formula) from arming a
        // schedule in the past.
        return false
    }
    if now.Before(clickAt) {
        // The windup holds the movement: the retreat defers to the
        // accepted window (the measured deferral of an immediate
        // click costs the whole reload of standing time).
        if l.kiteArmClick(clickAt, until) {
            l.logger.Printf("Hunt: shot released on %d, hostile %d "+
                "holds %d units, the retreat clicks at the windup "+
                "end", l.target, threatID, int(math.Round(dist)))

            return true
        }

        return false
    }
    // The broadcast tick landed inside the accepted window already
    // (the late catch of a fast bow): the retreat clicks at once,
    // the walk owns the remainder of the cycle.
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered.
        return false
    }
    l.logger.Printf("Hunt: shot released on %d, hostile %d holds "+
        "%d units, kiting the reload tail", l.target, threatID,
        int(math.Round(dist)))

    return true
}

// kiteStraightStep budgets a STRAIGHT leg against the opening ring:
// the endpoint of the away-ray must stay inside the fight's ring -
// the leash share of the roam radius around the fight anchor (the
// target's stand, see kiteResolveAndClick) - so the retreat stays on
// the ground the fight keeps. A stand already at the ring keeps at
// least the short hop of kiteMinStep; the lane gate refuses the hop
// when even that is walled, and the hold answers. The margin
// (kiteArrivalEpsilon's half) keeps the int32 cell rounding of the
// endpoint from crossing the ring the battery enforces - a
// ring-budget step rounded onto the ring plus one reads past the
// leash and the walk is refused. Reports the step unchanged before
// the fight anchored.

func (l *Loop) kiteStraightStep(step float64, selfX, selfY int32) float64 {
    if l.kiteFightFor != l.target {
        return step
    }
    anchorDist := math.Hypot(
        float64(selfX-l.kiteFightX), float64(selfY-l.kiteFightY))
    ring := kiteRoamRadius * kiteRingLeashShare
    if budget := ring - anchorDist -
        kiteArrivalEpsilon/2; budget < step {
        step = math.Max(kiteMinStep, budget)
    }

    return step
}

// kiteResolveAndClick resolves the straight retreat lane against the
// live train and terrain and issues the retreat click through the
// ladder seam: the encircled train (the away vectors cancel - chasers
// stand on every side) and a blocked straight lane answer with the
// hold ground rule of the archetype (never a weapon switch - the bow
// is the always-weapon, and the owner rule of the 2026-11-09 round
// removed every turning answer), a walkable lane clicks at once with
// the movement window the caller computed. The shared seam serves the
// deferred click of the shot cycle (kiteClickWalk), the accepted
// window catch of the broadcast tick and the ordinary proximity step
// - one resolution, one click path, one ladder. The leg length is
// the plain kiteStep, fenced by the fight ring (kiteStraightStep): the
// walk is interrupted by the forced attack at the disable end, so the
// endpoint names the line of the retreat, not a distance to wait for.
// Reports whether the click went out.

func (l *Loop) kiteResolveAndClick(
    now, until time.Time, selfX, selfY, selfZ int32,
) bool {
    threatID, _, threatOK := l.kiteThreat(now, selfX, selfY, selfZ)
    if l.kiteFightFor != l.target {
        // A fresh fight: the anchor falls back to the self position
        // until the threat position resolves below.
        l.kiteFightFor = l.target
        l.kiteFightX, l.kiteFightY = selfX, selfY
    }
    if threatOK {
        if tx, ty, _, tok := l.tracker.ObjectPosition(threatID); tok {
            // The live-threat re-anchor (the 2026-11-09 owner round):
            // the threat MOVES while the fight runs, and a frozen
            // anchor turns the roam ring into a fence behind the
            // retreat. The live 17:43 dump decoded it: the shot stood
            // 111 units from the mob, the clamped 250-unit endpoint
            // read ~1404 from the fight's first stand, past the 1350
            // roam radius, and the "cornered" hold answered a runway
            // the geodata probe found open at every distance - the
            // second failure at 17:44 repeated it. Re-reading the
            // threat on EVERY resolution centers the ring on the
            // fight it belongs to; a chaser that follows keeps the
            // ring around itself, so the fence only holds back a
            // retreat from a threat that stands its ground. The
            // anchor follows the NEAREST threat (kiteThreat: the
            // fight target or the nearest attacker), not just the
            // selected target: a train member grinding the character
            // at point blank with the target standing 873 units out
            // (the 18:32 live run) clamped the step to the 250 floor
            // around the far target - the ring must center on the
            // danger that actually arms the step. A vanished threat
            // keeps the last anchor (the fight is over or the
            // broadcast lags).
            l.kiteFightX, l.kiteFightY = tx, ty
        }
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        // The degenerate no-chaser scene (a tracker gap inside the
        // broadcast window) or a threat standing ON the character:
        // the train has no ray to guard. Contact is the one scene the
        // hold never owns - the retreat takes the first open ring ray
        // (kiteContactAway), a tracker gap still holds the pacing
        // until the next probe.
        var contact bool
        dirX, dirY, contact = l.kiteContactAway(selfX, selfY, selfZ)
        if !contact {
            l.kiteHoldGround(now, true)

            return false
        }
    }
    // The straight retreat (the owner rule of the 2026-11-09 round):
    // the step runs exactly the away-ray of the train - the centroid
    // of a normal train, the widest-gap bisector of an encircled one.
    // There is no fan, no deflection and no chord: a turn spends the
    // distance the retreat just bought, and the fights whose speeds
    // sit too close cannot afford it. The raw ray stays both the
    // preferred lane and the half-plane reference of the battery.
    // The leg is the plain step, ring-budgeted so the fight stays on
    // its own ground.
    laneLen := l.kiteStraightStep(l.kite.Step, selfX, selfY)
    stepX, stepY, found := l.kiteRetreatLane(
        selfX, selfY, selfZ, dirX, dirY, dirX, dirY, laneLen)
    if !found {
        // The straight lane is blocked: the hold ground answer is
        // the archetype rule - the bow is the always-weapon, the
        // cornered archer never switches to a melee trade and never
        // turns the retreat, it stands and shoots the way out.
        l.kiteHoldGround(now, encircled)

        return false
    }
    l.kiteHeldAt = time.Time{}
    l.kiteAt = now
    l.kiteIssueWalk(now, until, selfX, selfY, selfZ, stepX, stepY)

    return true
}

// kiteArmClick arms the deferred retreat click of the live shot
// cycle: the click time, the walk window end and the owning fight
// land in the scheduler state (see kiteClickWalk - the issue #70
// findings: the server saves a MoveToLocation inside the bow windup
// and replays it only at the disable end, where the re-shot cancels
// it, so the click waits the windup out) and the movement window the
// forced attack re-requests respect stretches to the same end - a
// re-request inside the cycle would interrupt the planned walk, and
// inside the windup it only burns the server's bow disable answer
// anyway. The arming is idempotent per schedule: the very same click
// time (the very same shot) re-arms silently - the caller yields the
// tick only on a fresh schedule. Reports whether the arming spent
// the tick.

func (l *Loop) kiteArmClick(clickAt, until time.Time) bool {
    if l.kiteClickFor == l.target && l.kiteClickAt.Equal(clickAt) {
        // The schedule of this very shot is already armed (a second
        // trigger of the same cycle): the tick stays with the fight
        // machinery (the potions, the casts).
        return false
    }
    l.kiteClickAt = clickAt
    l.kiteClickFor = l.target
    l.kiteClickUntil = until
    l.combatAvoidUntil = until

    return true
}

// kiteClickWalk fires the deferred retreat click at the scheduled
// boundary (see kiteArmClick): the windup of the arming shot held
// the movement until now, so the click rides the accepted move
// window - the server adopts the intention at once (the movement
// broadcast follows) instead of saving it for the disable end. The
// fire re-checks everything the windup may have changed: the fight
// the schedule belongs to (a target switch - the old target died, a
// fresh pick - disarms it), the threat band (a hostile that left the
// pursue band is the stall watchdog's re-approach, not a retreat)
// and the straight lane itself (the chasers moved while the character
// stood - the train direction resolves fresh at the click moment).
// The encircled and the cornered answers hold the ground as always.
// A threat standing ON the character is no reason to stand either:
// the train geometry has no bearing there, so the retreat takes the
// first open ring ray (kiteContactAway) instead of spending the shot
// cycle in melee. Reports whether the tick spent the click.

func (l *Loop) kiteClickWalk(now time.Time) bool {
    if l.kiteClickAt.IsZero() {
        return false
    }
    if l.kiteClickFor != l.target || l.target == 0 {
        // The fight moved on: the schedule served the shot that
        // armed it, a fresh fight re-arms its own. The movement
        // hold dies with the ownership - but only the hold the
        // arming itself set (the Equal guard): a walk already in
        // flight keeps its own window, and the fire-time threat
        // re-check below keeps the shot's real disable hold (a
        // re-request the server would only refuse through the
        // disable).
        l.kiteClickDrop()

        return false
    }
    if now.Before(l.kiteClickAt) {
        // The windup still holds the movement: the tick stays with
        // the fight machinery (the potions, the casts) - the
        // movement window and the re-request gate are already armed.
        return false
    }
    if now.After(l.kiteClickUntil) {
        // The disable lapsed between the ticks (a stalled tick, a
        // blocking write): a click now would race the re-shot and
        // lose a whole cycle (the 3000 ms probe row of the findings)
        // - its walk window is already past, the ladder would die
        // on arrival and the forced re-request would cancel the
        // just-issued walk. The schedule stands down; the next shot
        // arms its own.
        l.kiteClickClear()

        return false
    }
    until := l.kiteClickUntil
    l.kiteClickClear()
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(now, selfX, selfY, selfZ)
    if !ok || l.kiteThreatBeyondThePursue(threatID, dist) {
        // The hostile left the pursue band (or the chase dissolved):
        // the retreat has nothing to retreat from - the stall
        // watchdog owns the re-approach, the next shot cycle re-arms
        // the rhythm on its own broadcast.
        return false
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered (the hold paces
        // its own re-probe): the next cycle re-arms fresh.
        return false
    }
    l.logger.Printf("Hunt: the windup on %d ended (hostile %d at "+
        "%d units), kiting the reload tail", l.target, threatID,
        int(math.Round(dist)))

    return true
}

// kiteClickClear stands the deferred click schedule down: the next
// shot cycle (or the proximity trigger of the next fight) arms its
// own schedule whole. The re-click ladder keeps its own lifecycle
// (the walk it belongs to).

func (l *Loop) kiteClickClear() {
    l.kiteClickAt = time.Time{}
    l.kiteClickFor = 0
    l.kiteClickUntil = time.Time{}
}

// kiteClickDrop stands a deferred click schedule down together with
// the movement hold its arming set: the Equal guard keeps a walk
// already in flight on its own window (only the hold the arming
// itself armed dies with the schedule). The fight side runs it from
// kiteClickWalk (a target switch), the defensive flight from
// kiteFleeThreat (the loot interrupt drops the target a second after
// the shot, and the orphaned hold kept the whole flight standing).
func (l *Loop) kiteClickDrop() {
    if l.combatAvoidUntil.Equal(l.kiteClickUntil) {
        l.combatAvoidUntil = time.Time{}
    }
    l.kiteClickClear()
}

// kiteShotPhase resolves the server bow cycle a retreat must
// respect: the shot broadcast anchors the windup (the server forbids
// the movement before its end - the saved intention of
// PlayerAI.setIntentionMoveTo) and the disable end (no re-shot
// before it lapses), and the retreat click owns the reuse tail
// between the two (the accepted move window of the issue #70
// findings: the earliest immediate retreat sat at the windup end,
// 1500 ms after the shot at pAtkSpd 337, while a click inside the
// windup stood to the full cycle end). Reports the earliest accepted
// click time (the windup end plus the safety lead), the walk window
// end (the disable end plus the re-engage delay) and whether the
// cycle is still live - a cycle that ended leaves the retreat to
// the ordinary full window.

func (l *Loop) kiteShotPhase(
    now time.Time,
) (clickAt, until time.Time, live bool) {
    shotAt := l.tracker.SelfLastShotAt()
    if shotAt.IsZero() {
        return time.Time{}, time.Time{}, false
    }
    until = shotAt.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
    if now.After(until) {
        // The disable lapsed: the re-engage owns the moment, and a
        // retreat riding the dead cycle would hold its window for
        // nothing.
        return time.Time{}, time.Time{}, false
    }
    clickAt = shotAt.Add(l.kiteWindupWindow(now) + kiteWindupLead)

    return clickAt, until, true
}

// kiteIssueWalk sends the retreat click of one kite step and arms
// the re-click ladder on it (issue #60, the second round): the walk
// endpoint, the issue cell and the window land in the ladder state
// (see kiteReclickWalk) so the dead-click recovery - the repeated
// click the owner's manual evidence names - rides the SAME retreat
// instead of the character standing through the reload a swallowed
// click leaves it. Every kite path (the deferred click of the shot
// cycle, the accepted-window catch, the proximity step) issues its
// walks through this one seam; a fresh step resets the ladder whole
// (a walk already in flight owns the movement, the layer gates hold
// the double step). The window end is the caller's phase-aware
// moment: the deferred click of a live cycle ends at the shot's
// disable end (the walk owns the reuse tail, the forced attack
// re-request fires the moment it lapses), a cycle-less step keeps
// the full window from now.

func (l *Loop) kiteIssueWalk(
    now, until time.Time, selfX, selfY, selfZ, stepX, stepY int32,
) {
    l.kiteWalkX, l.kiteWalkY, l.kiteWalkZ = stepX, stepY, selfZ
    l.kiteWalkBaseX, l.kiteWalkBaseY = selfX, selfY
    l.kiteWalkIssuedAt = now
    // The window is the caller's phase-aware moment: the deferred
    // click of a live cycle ends at the shot's disable end (the walk
    // owns the reuse tail, the forced attack re-request fires the
    // moment it lapses - that request IS the next shot), a
    // cycle-less step keeps the full bow window from now. The walk
    // is deliberately NOT stretched past that moment: waiting for a
    // regained distance is the treadmill the 5-minute acceptance run
    // measured.
    l.kiteWalkUntil = until
    l.combatAvoidUntil = until
    l.kiteReclickAt = now
    l.kiteReclicks = 0
    l.kiteWalkDead = false
    if err := l.game.WalkTo(stepX, stepY, selfZ); err != nil {
        l.logger.Printf("Hunt: kite walk failed: %v", err)
    }
}

// kiteWindupWindow resolves the bow windup the live server forbids
// the movement in: the Attack launch arms isAttackingNow for the
// first half of the bow disable window (Creature.doAttack, the BOW
// branch: _attackEndTime = now + timeToHit + reuse/2 = now +
// (timeAtk+reuse)/2, with timeToHit = timeAtk/2 - the Mobius C1
// source read of the issue #70 round), and PlayerAI.setIntentionMoveTo
// answers a move inside it by SAVING the intention and replaying it
// at notifyActionReadyToAct - the disable end, where the re-shot
// cancels the replay. The live probe confirmed the boundary: the
// 1500 ms click after the shot moved at once while the 1200 ms one
// stood to the cycle end (docs/kite_timing_findings.md). The window
// rides the same pAtkSpd formula, clamps and fallback as
// kiteWalkWindow - exactly its first half.

func (l *Loop) kiteWindupWindow(now time.Time) time.Duration {
    return l.kiteWalkWindow(now) / 2
}

// kiteGaugeCycle returns the bow disable window the server announced
// with its own RED SetupGauge, while the gauge is fresh enough to
// belong to the shot cycle in flight: the exact timing the formula
// only approximates. The ATK_SPD attribute is not rebroadcast after a
// relogin until the gear changes, so the formula fell back to its 2 s
// default while the server ran the real 1483 ms windup - the 18:33:13
// retreat click landed 233 ms inside the windup and the server
// answered ActionFailed (docs/kite_timing_findings.md). The gauge
// carries no such gap: it arrives with every shot.

func (l *Loop) kiteGaugeCycle(now time.Time) (time.Duration, bool) {
    at, cycle := l.tracker.SelfBowCycle()
    if at.IsZero() || cycle <= 0 || now.Before(at) ||
        now.Sub(at) > kiteGaugeMaxAge {
        return 0, false
    }
    if cycle < kiteWalkWindowFloor || cycle > kiteWalkWindowMax {
        return 0, false
    }

    return cycle, true
}

// kiteWalkWindow resolves the movement window the kite walk owns:
// the bow disable window the C1 server enforces between the shots -
// timeAtk + reuse = 500000/pAtkSpd + reuseDelay*333/pAtkSpd
// (Creature.calculateTimeBetweenAttacks and calculateReuseTime, the
// reuseDelay of the bow family in the item data). The server's own
// RED SetupGauge (SetupGauge(RED, sAtk + reuse) at the shot commit,
// see Creature.doAttackHitByBow) outranks the formula whenever it is
// fresh: it is the exact window the server runs, whatever the
// client's pAtkSpd view says. The formula stays the fallback for a
// cycle without a parsed gauge, read from the live pAtkSpd the
// StatusUpdate broadcasts (attr 0x12, the value the acceptance dump
// shows as 337 for the Short Bow kit, giving roughly (500000 +
// 1500*333)/337 = 2.97s - the "3 seconds to draw shot" the owner
// measured by hand). The walk then spends the WHOLE cooldown
// running (the owner ask of the third round: the proper archering
// delays from the C1 Mobius formulas - if it is not shooting it
// should be moving away from the target) instead of the shipped
// fixed 2s that stood the character idle for the last second of
// every reload. A missing or absurd pAtkSpd broadcast falls back to
// the shipped kiteStepWindow (the walk contract never depends on
// the packet being parsed).

func (l *Loop) kiteWalkWindow(now time.Time) time.Duration {
    if cycle, ok := l.kiteGaugeCycle(now); ok {
        return cycle
    }
    spd := l.tracker.SelfPAtkSpd()
    if spd <= 0 {
        return kiteStepWindow
    }
    ms := (500000.0 + kiteBowReuseDelay*333.0) * float64(time.Millisecond) /
        float64(spd)
    if ms < float64(kiteWalkWindowFloor) || ms > float64(kiteWalkWindowMax) {
        return kiteStepWindow
    }

    return time.Duration(ms)
}

// kiteClickRefused reports whether the server answered the last
// kite click with ActionFailed: the one byte refusal answer carries
// no request identity, so the correlation is the send time (the
// same contract the town walker's refusalEvidence runs) - a failure
// that arrived after the click, inside the round-trip window, with
// no other request between, belongs to the click. The acceptance
// dump of the third round shows the kite endpoint refusals landing
// the same millisecond the click went out: the server (the Mobius
// MoveToLocation handler - the isCompletelyBlocked destination
// check among others) refuses specific destination cells outright,
// and the bot's own geodata does not always agree (the dump cells
// read open in the shipped pack), so the ONLINE evidence is the
// only refusal channel that never lies.

func (l *Loop) kiteClickRefused() bool {
    if l.kiteReclickAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    // The failure may carry the SAME coarse clock stamp as the click
    // (the Windows timer granularity - the live server answers inside
    // the same millisecond), so the correlation starts at the click
    // moment inclusive instead of strictly after it.
    if failedAt.Sub(l.kiteReclickAt) > kiteRefusalAnswerWindow {
        return false
    }
    if failedAt.Before(l.kiteReclickAt) {
        return false
    }
    if l.tracker.OtherRequestBetween(l.kiteReclickAt, failedAt) {
        // An action request rides the answer window itself.
        return false
    }

    // The bow retry chain: an attack request the server bounced
    // through the bow disable answers ActionFailed about a second
    // later (Creature.canUseRangeWeapon schedules the retry), far
    // outside the tight window of the click. Attributing that chain
    // to the retreat click latched a dead endpoint on a healthy
    // click (the 14:06:50 dump).
    return !l.tracker.OtherRequestBetween(
        failedAt.Add(-kiteBowRetryLead), l.kiteReclickAt)
}

// kiteReclickWalk runs the re-click ladder of the retreat walk: while
// the movement window runs and the character still stands, the click
// goes out again at kiteReclickPeriod toward the same endpoint - a
// re-click inside the server's once-per-second broadcast gate re-aims
// it and never wastes it. A stand inside kiteArrivalEpsilon completes
// the step; a stand short of it re-bases the ladder onto the current
// cell. The retreat has one lane, so a refused or silent endpoint
// latches the dead verdict and the layer holds the ground for the bow
// instead of turning. Reports whether this tick spent a re-click.

func (l *Loop) kiteReclickWalk(now time.Time) bool {
    if l.kiteWalkUntil.IsZero() {
        return false
    }
    if now.After(l.kiteWalkUntil) {
        // The disable end: the walk stands down and the forced
        // attack re-request owns the tick. That request IS the next
        // shot - the owner contract of the 2026-11-09 round - even
        // when the chaser stands in melee after the reload.
        l.kiteWalkClear()

        return false
    }
    if l.tracker.SelfWalking() {
        // The walk runs: the click landed. The episode budget
        // refreshes (a mid-window drop later gets its own re-clicks),
        // the leg state stays armed for the whole window.
        l.kiteReclicks = 0

        return false
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    if math.Hypot(
        float64(selfX-l.kiteWalkX),
        float64(selfY-l.kiteWalkY)) <= kiteArrivalEpsilon {
        // The leg completed: the character stands on (or within a
        // cell's throw of) the endpoint. The ladder stops re-clicking
        // (the endpoint is reached), but the walk stays armed: the
        // movement hold must survive the early arrival so the forced
        // attack fires at the disable end, never at the collapsed
        // range of the arrival cell.
        return false
    }
    if selfX != l.kiteWalkBaseX || selfY != l.kiteWalkBaseY {
        // A fresh standing episode: the character left the last
        // issue cell (a mid-leg silent drop, a short server path)
        // and stands short of the endpoint - the ladder re-bases
        // onto the current cell and keeps pushing the SAME endpoint,
        // the episode budget and the probe window re-arm whole.
        l.kiteWalkBaseX, l.kiteWalkBaseY = selfX, selfY
        l.kiteReclicks = 0
        l.kiteWalkIssuedAt = now
        l.kiteWalkDead = false
    }
    if l.kiteReclicks >= kiteReclickLimit {
        return false
    }
    // The refusal evidence outranks the pacing: a click the server
    // answered ActionFailed never starts a walk, waiting only burns
    // the movement window.
    refused := l.kiteClickRefused()
    if !refused && now.Sub(l.kiteReclickAt) < kiteReclickPeriod {
        return false
    }
    l.kiteReclicks++
    l.kiteReclickAt = now
    if !l.kiteReclickVerdict(now, selfX, selfY, refused) {
        return false
    }
    if err := l.game.WalkTo(
        l.kiteWalkX, l.kiteWalkY, l.kiteWalkZ); err != nil {
        l.logger.Printf("Hunt: kite re-click failed: %v", err)

        return false
    }

    return true
}

// kiteReclickVerdict latches the dead-endpoint verdict of the ladder
// and answers it with the hold. The evidence verdict comes first (the
// server answered the last click with ActionFailed - the probe line
// lands once per walk so the dump names the mechanism), the silent
// probe second (the walk stayed unanswered past the probe age with
// the character still on the issue cell - the H-006 silent drop
// signature). Both keep the spam going while the walk is younger
// than the broadcast gate (a healthy click's movement broadcast may
// still be in flight), and past the gate the straight design has no
// second lane: the ladder clears, the hold owns the cycle and the bow
// keeps shooting (the owner rule of the 2026-11-09 round - a turn
// spends the distance the retreat just bought). Reports whether the
// re-click may still go out.

func (l *Loop) kiteReclickVerdict(
    now time.Time, selfX, selfY int32, refused bool,
) bool {
    if refused && !l.kiteWalkDead {
        // The evidence verdict: the server answered the click with
        // ActionFailed inside the round trip. The probe line lands
        // once per walk so the dump names the mechanism; the spam
        // keeps going while the walk is younger than the broadcast
        // gate below - a refusal answer of the server AI's own attack
        // retries (the bow disable path schedules them a second
        // apart, each retry on a still-disabled bow bounces) rides
        // the same packet and only the age gate separates it from
        // the movement broadcast that may still be in flight for a
        // healthy click.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "was refused by the server (standing on %d %d), "+
            "holding ground at the probe age",
            l.target, selfX, selfY)
    } else if !refused && !l.kiteWalkDead &&
        now.Sub(l.kiteWalkIssuedAt) >= kiteProbeElapsed {
        // The silent probe: the initial click, the plain re-clicks
        // and the character still stands on the issue cell past the
        // broadcast gate plus the pacing headroom - the movement
        // never started and no refusal arrived. One line per walk,
        // the hold follows.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "never started the movement (%d clicks out, standing "+
            "on %d %d), holding ground",
            l.target, l.kiteReclicks+1, selfX, selfY)
    }
    if !l.kiteWalkDead ||
        now.Sub(l.kiteWalkIssuedAt) < kiteProbeElapsed {
        // No verdict yet, or the walk is still younger than the
        // broadcast gate: the re-click keeps hitting the endpoint -
        // the spam the owner asked for, and the movement broadcast
        // of a healthy click still gets its chance to clear the
        // ladder before the hold spends the cycle.
        return true
    }
    // The straight design has no second lane to rotate onto (the
    // owner rule of the 2026-11-09 round): the dead endpoint stands
    // the ladder down and the hold ground answer keeps the bow
    // shooting. The next shot cycle re-arms the retreat fresh.
    l.kiteWalkClear()
    l.kiteHoldGround(now, false)

    return false
}

// kiteWalkClear stands the re-click ladder down: the endpoint fields
// stay (they name the walk of record for the diagnostics), the gate
// (kiteWalkUntil), the issue stamp, the click count and the probe
// latch reset - the next kite step arms the ladder whole through
// kiteIssueWalk. The movement hold the walk itself armed
// (combatAvoidUntil equal to the walk window) ends with it: the hold
// exists to keep the walking archer from standing through the reload,
// and the window end is the bow disable end where the forced attack
// re-request (the next shot) owns the tick. A hold of any other owner
// - the deferred click of a live shot cycle, an impending-add step -
// keeps its own window.

func (l *Loop) kiteWalkClear() {
    if l.combatAvoidUntil.Equal(l.kiteWalkUntil) {
        l.combatAvoidUntil = time.Time{}
    }
    l.kiteWalkUntil = time.Time{}
    l.kiteWalkIssuedAt = time.Time{}
    l.kiteReclicks = 0
    l.kiteWalkDead = false
}

// kiteHoldGround arms the hold of the kite: the archer stops
// retreating and keeps shooting the bow at melee range. The hold
// answers a blocked straight lane (the owner rule of the 2026-11-09
// round: no turning, no side step - the straight line is the whole
// retreat contract), a refused or silent retreat click, and an
// encircled train with no walkable gap. It paces its own re-probe
// (the kite period) so a failed lane never stutters the tick loop,
// and logs its reason once per hold episode - a standing fight that
// names itself stays diagnosable in the event feed. The surrounded
// flag picks the diagnostic wording.

func (l *Loop) kiteHoldGround(now time.Time, surrounded bool) {
    fresh := l.kiteHeldFor != l.target ||
        l.kiteHeldAt.IsZero() ||
        now.Sub(l.kiteHeldAt) >= kiteHoldLogPeriod
    if fresh {
        switch {
        case l.target == 0:
            // The targetless hold belongs to the defensive flight
            // (kiteFleeThreat): the full pocket keeps the ticks and
            // re-probes at the kite period - the zone-clamped fan
            // never runs behind it.
            l.logger.Printf("Hunt: no straight bow flee lane opens " +
                "from the chase, holding the ground through the " +
                "chase")
        case surrounded:
            l.logger.Printf("Hunt: the chasers surround the bow "+
                "fight on target %d, holding ground and shooting "+
                "through the train", l.target)
        default:
            l.logger.Printf("Hunt: no walkable retreat lane on "+
                "target %d (cornered), holding ground and shooting "+
                "the bow", l.target)
        }
    }
    l.kiteHeldFor = l.target
    l.kiteHeldAt = now
}

// rotatePlanar rotates a planar unit direction by the given angle
// (counterclockwise, the mathematical sign convention).

func rotatePlanar(x, y, angle float64) (float64, float64) {
    cos, sin := math.Cos(angle), math.Sin(angle)

    return x*cos - y*sin, x*sin + y*cos
}
