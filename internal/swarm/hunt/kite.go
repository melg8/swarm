// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kiting layer of the archer fight: a hostile inside the retreat radius
// makes the character step away, the movement window pauses the forced attack
// re-requests and the engage re-requests it once the step finished. One
// straight away-ray and no turn, a blocked lane holds the ground and keeps
// shooting, and the layer never arms against a bow-wielding enemy. The
// design: docs/hunting.md, the timing: docs/kite_timing_findings.md.

import (
    "math"
    "time"
)

// The kiting constants of the archer fight.
const (
    // kiteRetreatRadius arms the step: a closer hostile reaches melee
    // (the swing range sits near 150).
    kiteRetreatRadius = 250.0
    // kiteStep is the retreat length: a walk lands back inside the band.
    kiteStep = 400.0
    // kiteStepWindow is the window one step owns: an attack cancels a walk.
    kiteStepWindow = 2 * time.Second
    // kiteReengageDelay holds the first attack re-request after the walk.
    kiteReengageDelay time.Duration = 0
    // kiteStepPeriod paces the steps and the cornered hold re-probes.
    kiteStepPeriod = 3 * time.Second
    // kiteStreakLimit is the diagnostic threshold of an unwinnable race.
    kiteStreakLimit = 8
    // kiteTrainScanRange bounds the chaser scan of the retreat direction.
    kiteTrainScanRange = 800.0
    // kiteEncircleShare is the encirclement threshold of the away vectors.
    kiteEncircleShare = 0.3
    // kiteMaxTrainMembers caps the bearing scratch, a fixed array.
    kiteMaxTrainMembers = 16
    // kiteHalfPlaneSlack lets a lane run perpendicular, never back toward
    // the train.
    kiteHalfPlaneSlack = -0.05
    // kiteRoamRadius fences the retreat around the fight anchor: three bow
    // engage radii, no zone table.
    kiteRoamRadius = 3 * userBowEngageRadius
    // kiteMinStep is the shortest step the ring budget may leave.
    kiteMinStep = 250.0
    // kiteHoldLogPeriod paces the cornered hold diagnostic line.
    kiteHoldLogPeriod = 15 * time.Second
    // kiteShotWindow is the freshness window of the own Attack broadcast,
    // the server commit of the shot: two ticks at the loop cadence.
    kiteShotWindow = 700 * time.Millisecond
    // kiteReclickPeriod paces the re-click ladder under the broadcast gate.
    kiteReclickPeriod = 250 * time.Millisecond
    // kiteReclickLimit bounds the re-clicks of one walk.
    kiteReclickLimit = 8
    // kiteProbeElapsed is the silent-verdict age of a dead endpoint.
    kiteProbeElapsed = 1200 * time.Millisecond
    // kiteRefusalAnswerWindow bounds the ActionFailed attribution of a
    // click; a wider window would latch healthy endpoints dead.
    kiteRefusalAnswerWindow = 250 * time.Millisecond
    // kiteBowRetryLead is the look-back band of the delayed bow refusal
    // (Creature.canUseRangeWeapon).
    kiteBowRetryLead = 1350 * time.Millisecond
    // kiteBowReuseDelay is the reuse delay of the bow family (1500).
    kiteBowReuseDelay = 1500.0
    // kiteWalkWindowFloor/Max clamp the bow-aware walk window.
    kiteWalkWindowFloor = 1 * time.Second
    kiteWalkWindowMax   = 5 * time.Second
    // kiteGaugeMaxAge bounds how long a RED SetupGauge is the bow cycle.
    kiteGaugeMaxAge = 10 * time.Second
    // kiteWindupLead clears the (timeAtk+reuse)/2 boundary despite the lag.
    kiteWindupLead = 150 * time.Millisecond
    // kiteRingLeashShare caps the step inside the 1500 unit audit leash.
    kiteRingLeashShare = 0.72
    // kiteArrivalEpsilon is the arrival radius of the re-click ladder.
    kiteArrivalEpsilon = 100.0
)

// KiteParams is the tunable block of the kite fight: the four knobs the
// launch config carries per bot spec (launch_config.go) plus the enable gate.
type KiteParams struct {
    // Enabled gates the kite layer: false keeps the standing archer.
    Enabled bool
    // RetreatRadius is the trigger distance and the inner band edge.
    RetreatRadius float64
    // Step is the retreat length away from the closed threat.
    Step float64
    // StepPeriod paces the steps and the cornered hold re-probes.
    StepPeriod time.Duration
    // ReengageDelay is the hold between the walk end and the re-request.
    ReengageDelay time.Duration
}

// DefaultKiteParams returns the shipped tuning.
func DefaultKiteParams() KiteParams {
    return KiteParams{
        Enabled:       true,
        RetreatRadius: kiteRetreatRadius,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: kiteReengageDelay,
    }
}

// SetKiteParams overrides the tunable block: a non-positive value
// falls back to the shipped tuning, the re-engage delay keeps a zero.
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

// closedDistance bounds how close a chaser may stand since its last
// broadcast: the result never exceeds the true gap.
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

// kiteClosedDistance applies the closure bound to one tracked object; a
// standing object, a missing stamp and no track answer the raw distance.
func (l *Loop) kiteClosedDistance(
    now time.Time, objectID int32, dist float64,
) float64 {
    track, ok := l.tracker.ObjectTrackOf(objectID)
    if !ok || !track.Moving || track.MovedAt.IsZero() {
        return dist
    }

    return closedDistance(dist, now.Sub(track.MovedAt), track.Speed)
}

// kiteThreat locates the hostile that arms the step: the fight target or
// the nearest chaser, whichever is closer; an unreachable deck is none.
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

// kiteLayerGates covers the gates every kite layer shares.
func (l *Loop) kiteLayerGates(now time.Time) bool {
    // The params gate comes first: a kite-disabled profile never arms.
    if !l.kite.Enabled || !l.bowEquipped() || l.target == 0 {
        return false
    }
    // Against an archer the retreat is off: a ranged enemy cannot be
    // outrun (docs/hunting.md).
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

// kiteOwnsRetreat reports whether the kite owns the aggro answer.
func (l *Loop) kiteOwnsRetreat() bool {
    return l.kite.Enabled && l.target != 0 && l.bowEquipped() &&
        !l.tracker.ObjectUsesBow(l.target)
}

// kiteLayerArmed reports whether the layer is enabled with a bow in hand.
func (l *Loop) kiteLayerArmed() bool {
    return l.kite.Enabled && l.bowEquipped()
}

// kiteFlightWanted reports whether the straight flight owns the answer.
func (l *Loop) kiteFlightWanted() bool {
    if !l.kiteLayerArmed() {
        return false
    }
    attacker, ok := l.tracker.NearestAttacker()

    return ok && !l.tracker.ObjectUsesBow(attacker.ObjectID)
}

// kiteKillWindow reports whether the target is one or two arrows from
// death (pileUpKillWindowPercent): a pile up never throws such a kill away.
func (l *Loop) kiteKillWindow() bool {
    if l.target == 0 {
        return false
    }
    hp := l.tracker.ObjectHealthPercent(l.target)

    return hp > 0 && hp <= pileUpKillWindowPercent
}

// kiteFlightOwnsEscape reports whether the targetless straight flight owns
// the answer: the flight is wanted and the losing verdict dropped the target.
func (l *Loop) kiteFlightOwnsEscape() bool {
    return l.target == 0 && l.kiteFlightWanted()
}

// dropFightForFlight hands the aggro answer to the straight bow flight.
func (l *Loop) dropFightForFlight(now time.Time) {
    if l.target != 0 {
        l.fleeFromTarget(l.target, now)

        return
    }
    l.fleeFromThreat(now)
}

// avoidBowTargets reports whether the target search fences bow wielders out;
// an archer that attacks is still answered via attackerEngageable.
func (l *Loop) avoidBowTargets() bool {
    return l.kite.Enabled && l.bowEquipped()
}

// kiteFlightHolds reports whether a fled mob still holds the character:
// MAX_ATTACK_TIMEOUT (1200 ticks of 100 ms) drops the aggro in 2 min.
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

// kiteFleeBusy reports whether a movement owner holds the tick: the walk
// window (the re-click ladder keeps it) or another step's hold.
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

// kiteFleeHeld reports whether the last flight step found no lane; only a
// targetless hold qualifies.
func (l *Loop) kiteFleeHeld(now time.Time) bool {
    return l.kiteHeldFor == 0 && !l.kiteHeldAt.IsZero() &&
        now.Sub(l.kiteHeldAt) < l.kite.StepPeriod
}

// kiteFlightMovable reports whether the bow may start a movement: the server
// saves a MoveToLocation inside the windup and replays it at the disable end,
// which the escape watchdog reads as a dead click.
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

// kiteFleeWait reports whether a movement owner keeps the flight grounded:
// a deferred click, the busy window, the hold, the windup, the stand.
func (l *Loop) kiteFleeWait(now time.Time) bool {
    if !l.kiteClickAt.IsZero() && l.kiteClickFor != l.target {
        l.kiteClickDrop()
    }

    return l.kiteFleeBusy(now) || l.kiteFleeHeld(now) ||
        !l.kiteFlightMovable(now) || !l.standUpGuarded(now)
}

// kiteFleeOtherDeck reports whether the attacker sits on another deck: its
// swings cannot land and the lane crosses the geodata wall.
func kiteFleeOtherDeck(attackerZ, selfZ int32) bool {
    return attackerZ != 0 &&
        math.Abs(float64(attackerZ-selfZ)) > deckReachableZ
}

// kiteFleeThreat answers a defensive flee with the straight bow flight
// instead of the melee fan: the away-ray and the re-click ladder, bypassing
// the fan budgets. A walled lane stamps the hold for one kite period.
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
        // No away bearing: the chaser stands ON the character or has not
        // broadcast yet; the ring rays answer contact.
        var contact bool
        dirX, dirY, contact = l.kiteContactAway(selfX, selfY, selfZ)
        if !contact {
            // Even the ring is walled: hold and re-probe at the kite
            // period (the critical-HP logout stays armed).
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
        // The away side is a dead end: break out through the other
        // half-plane, the open ground the character came from.
        stepX, stepY, found = l.kiteRetreatLane(
            selfX, selfY, selfZ, -dirX, -dirY, -dirX, -dirY,
            escapeWalkDistance)
        breakout = found
    }
    if !found {
        // A full pocket: hold and re-probe; the flight keeps the tick, so
        // the critical-HP logout stays armed.
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

// kiteFightLive reports whether a fight is live around the character.
func (l *Loop) kiteFightLive() bool {
    if l.target == 0 {
        return false
    }
    if l.tracker.SelfEngagedBy(l.target) {
        return true
    }

    return l.tracker.SelfAttackerCount() > 0
}

// kiteStepAdmitted guards the kite step of one tick: the layer gates, the
// pacing and a fresh-target streak reset. The streak is a diagnostic only.
func (l *Loop) kiteStepAdmitted(now time.Time) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    if !l.kiteAt.IsZero() && now.Sub(l.kiteAt) < l.kite.StepPeriod {
        return false
    }
    if l.kiteFor != l.target {
        l.kiteFor = l.target
        l.kiteStreak = 0
    }
    if l.kiteStreak == kiteStreakLimit {
        // The crossing fires once per target: a same-speed chase outwaits
        // the mob's home leash while a standing one eats the health bar.
        l.logf("Hunt: the distance race on %d is unwinnable, "+
            "running it out straight (step %d)",
            l.target, l.kiteStreak)
    }

    return true
}

// kiteFromTarget steps away from the hostile that armed the step to buy the
// weapon range back: the straight away-ray of the whole train. An encircled
// or walled lane holds the ground and the timing follows the shot.
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
        // An immediate click inside the windup would die at the re-shot:
        // arm the deferred schedule instead.
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
        // No shot cycle owns the moment: the ordinary full window.
        until = now.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered: it owns the cycle.
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

// kiteThreatBeyondThePursue reports whether the threat left the pursue band
// with no live attack behind it; a live attacker there keeps the rhythm.
func (l *Loop) kiteThreatBeyondThePursue(
    threatID int32, dist float64,
) bool {
    if dist < userBowEngageRadius || dist >= kiteTrainScanRange {
        return dist >= kiteTrainScanRange
    }
    for _, attacker := range l.tracker.SelfAttackers() {
        if attacker.ObjectID == threatID {
            // A live attacker beyond the band (the duel) rides the rhythm.
            return false
        }
    }

    return true
}

// kiteFromShot arms the shot-paced retreat: the own Attack broadcast is the
// server commit of the shot, and the server forbids movement through the
// windup (it replays the saved click at the disable end, where the re-shot
// cancels it). The click waits the windup end plus the safety lead.
func (l *Loop) kiteFromShot(now time.Time) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    // The trigger: the character's own shot, fresh inside the broadcast window.
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
        // The window cannot lapse while the broadcast is fresh; the guard keeps
        // a degenerate broadcast (a zero pAtkSpd) from arming a past schedule.
        return false
    }
    if now.Before(clickAt) {
        // The windup holds the movement: an immediate click would be
        // replayed at the disable end and cost the whole reload.
        if l.kiteArmClick(clickAt, until) {
            l.logger.Printf("Hunt: shot released on %d, hostile %d "+
                "holds %d units, the retreat clicks at the windup "+
                "end", l.target, threatID, int(math.Round(dist)))

            return true
        }

        return false
    }
    // The broadcast tick landed inside the accepted window already: the
    // retreat clicks at once and the walk owns the rest of the cycle.
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered.
        return false
    }
    l.logger.Printf("Hunt: shot released on %d, hostile %d holds "+
        "%d units, kiting the reload tail", l.target, threatID,
        int(math.Round(dist)))

    return true
}

// kiteStraightStep budgets a straight step inside the leash share of the
// roam radius around the fight anchor, with at least kiteMinStep.
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

// kiteResolveAndClick resolves the retreat lane against the live train and
// terrain and issues the click: an encircled train or a blocked lane holds the
// ground, a walkable lane clicks with the caller's window. One resolution
// serves every trigger.
func (l *Loop) kiteResolveAndClick(
    now, until time.Time, selfX, selfY, selfZ int32,
) bool {
    threatID, _, threatOK := l.kiteThreat(now, selfX, selfY, selfZ)
    if l.kiteFightFor != l.target {
        // A fresh fight: the anchor falls back to the self position.
        l.kiteFightFor = l.target
        l.kiteFightX, l.kiteFightY = selfX, selfY
    }
    if threatOK {
        if tx, ty, _, tok := l.tracker.ObjectPosition(threatID); tok {
            // Re-read the threat on every resolution: a frozen anchor turns
            // the roam ring into a fence behind the retreat.
            l.kiteFightX, l.kiteFightY = tx, ty
        }
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        // No chaser bearing (a tracker gap) or a threat standing ON the
        // character: the train has no ray to guard, and the retreat takes
        // the first open ring ray.
        var contact bool
        dirX, dirY, contact = l.kiteContactAway(selfX, selfY, selfZ)
        if !contact {
            l.kiteHoldGround(now, true)

            return false
        }
    }
    // The step runs the away-ray of the train (the widest-gap bisector
    // when encircled): no fan, no turn.
    laneLen := l.kiteStraightStep(l.kite.Step, selfX, selfY)
    stepX, stepY, found := l.kiteRetreatLane(
        selfX, selfY, selfZ, dirX, dirY, dirX, dirY, laneLen)
    if !found {
        // The lane is blocked: the cornered archer stands and shoots the
        // way out, never trading melee and never turning the retreat.
        l.kiteHoldGround(now, encircled)

        return false
    }
    l.kiteHeldAt = time.Time{}
    l.kiteAt = now
    l.kiteIssueWalk(now, until, selfX, selfY, selfZ, stepX, stepY)

    return true
}

// kiteArmClick arms the deferred retreat click: the click time, the walk
// window end and the owning fight land in the scheduler, and the movement
// window stretches to the same end.
func (l *Loop) kiteArmClick(clickAt, until time.Time) bool {
    if l.kiteClickFor == l.target && l.kiteClickAt.Equal(clickAt) {
        // This shot's schedule is already armed (a second trigger of the
        // cycle): the tick stays with the fight machinery.
        return false
    }
    l.kiteClickAt = clickAt
    l.kiteClickFor = l.target
    l.kiteClickUntil = until
    l.combatAvoidUntil = until

    return true
}

// kiteClickWalk fires the deferred retreat click at the boundary and
// re-checks the fight, the threat band and the lane.
func (l *Loop) kiteClickWalk(now time.Time) bool {
    if l.kiteClickAt.IsZero() {
        return false
    }
    if l.kiteClickFor != l.target || l.target == 0 {
        // The fight moved on: the schedule served the shot that armed it. Only
        // the hold the arming set dies here; a walk in flight keeps its window.
        l.kiteClickDrop()

        return false
    }
    if now.Before(l.kiteClickAt) {
        // The windup still holds the movement: the window and the
        // re-request gate are already armed.
        return false
    }
    if now.After(l.kiteClickUntil) {
        // The disable lapsed between the ticks: a click now would race the
        // re-shot and lose a whole cycle.
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
        // The hostile left the pursue band: the stall watchdog owns the
        // re-approach, the next shot arms its own rhythm.
        return false
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered, pacing its own re-probe.
        return false
    }
    l.logger.Printf("Hunt: the windup on %d ended (hostile %d at "+
        "%d units), kiting the reload tail", l.target, threatID,
        int(math.Round(dist)))

    return true
}

// kiteClickClear stands the deferred click schedule down.
func (l *Loop) kiteClickClear() {
    l.kiteClickAt = time.Time{}
    l.kiteClickFor = 0
    l.kiteClickUntil = time.Time{}
}

// kiteClickDrop stands a deferred click down with the movement hold its
// arming set; a walk in flight keeps its window.
func (l *Loop) kiteClickDrop() {
    if l.combatAvoidUntil.Equal(l.kiteClickUntil) {
        l.combatAvoidUntil = time.Time{}
    }
    l.kiteClickClear()
}

// kiteShotPhase resolves the bow cycle a retreat respects: the shot anchors
// the windup and the disable end, the retreat owns the reuse tail between
// them (docs/kite_timing_findings.md).
func (l *Loop) kiteShotPhase(
    now time.Time,
) (clickAt, until time.Time, live bool) {
    shotAt := l.tracker.SelfLastShotAt()
    if shotAt.IsZero() {
        return time.Time{}, time.Time{}, false
    }
    until = shotAt.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
    if now.After(until) {
        // The disable lapsed: the re-engage owns the moment.
        return time.Time{}, time.Time{}, false
    }
    clickAt = shotAt.Add(l.kiteWindupWindow(now) + kiteWindupLead)

    return clickAt, until, true
}

// kiteIssueWalk sends the retreat click of one step and arms the re-click
// ladder on it.
func (l *Loop) kiteIssueWalk(
    now, until time.Time, selfX, selfY, selfZ, stepX, stepY int32,
) {
    l.kiteWalkX, l.kiteWalkY, l.kiteWalkZ = stepX, stepY, selfZ
    l.kiteWalkBaseX, l.kiteWalkBaseY = selfX, selfY
    l.kiteWalkIssuedAt = now
    // The window is the caller's phase-aware moment; the walk is never
    // stretched past it.
    l.kiteWalkUntil = until
    l.combatAvoidUntil = until
    l.kiteReclickAt = now
    l.kiteReclicks = 0
    l.kiteWalkDead = false
    if err := l.game.WalkTo(stepX, stepY, selfZ); err != nil {
        l.logger.Printf("Hunt: kite walk failed: %v", err)
    }
}

// kiteWindupWindow resolves the windup the server forbids the movement in:
// the first half of the bow disable window (Creature.doAttack).
func (l *Loop) kiteWindupWindow(now time.Time) time.Duration {
    return l.kiteWalkWindow(now) / 2
}

// kiteGaugeCycle returns the bow disable window the server announced with
// its own RED SetupGauge while it is fresh.
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

// kiteWalkWindow resolves the movement window the kite walk owns: the bow
// disable window, timeAtk + reuse = 500000/pAtkSpd + reuseDelay*333/pAtkSpd
// (Creature.calculateTimeBetweenAttacks). The fresh gauge outranks the
// formula; a missing pAtkSpd falls back to kiteStepWindow.
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

// kiteClickRefused reports whether the server answered the last kite click
// with ActionFailed: the byte carries no request identity, so the correlation
// is the send time inside the round-trip window with no other request between.
func (l *Loop) kiteClickRefused() bool {
    if l.kiteReclickAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    // The failure may carry the same coarse clock stamp as the click, so
    // the correlation starts at the click.
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

    // The bow retry chain: an attack bounced through the bow disable answers
    // ActionFailed about a second later (Creature.canUseRangeWeapon).
    return !l.tracker.OtherRequestBetween(
        failedAt.Add(-kiteBowRetryLead), l.kiteReclickAt)
}

// kiteReclickWalk runs the re-click ladder: while the window runs and the
// character stands, the click goes out again toward the same endpoint. An
// arrival completes the step, a dead verdict holds ground.
func (l *Loop) kiteReclickWalk(now time.Time) bool {
    if l.kiteWalkUntil.IsZero() {
        return false
    }
    if now.After(l.kiteWalkUntil) {
        // The disable end: the walk stands down and the forced attack
        // re-request owns the tick, and that request IS the next shot.
        l.kiteWalkClear()

        return false
    }
    if l.tracker.SelfWalking() {
        // The walk runs: the click landed. The episode budget refreshes.
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
        // The leg completed: the ladder stops re-clicking, the walk stays
        // armed so the forced attack fires at the disable end.
        return false
    }
    if selfX != l.kiteWalkBaseX || selfY != l.kiteWalkBaseY {
        // A fresh standing episode: the ladder re-bases onto the current
        // cell and keeps pushing the same endpoint.
        l.kiteWalkBaseX, l.kiteWalkBaseY = selfX, selfY
        l.kiteReclicks = 0
        l.kiteWalkIssuedAt = now
        l.kiteWalkDead = false
    }
    if l.kiteReclicks >= kiteReclickLimit {
        return false
    }
    // The refusal evidence outranks the pacing: a refused click never
    // starts a walk, so waiting only burns the window.
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

// kiteReclickVerdict latches the dead-endpoint verdict and answers it with
// the hold: the evidence verdict first (ActionFailed), the silent probe
// second. The straight design has no second lane.
func (l *Loop) kiteReclickVerdict(
    now time.Time, selfX, selfY int32, refused bool,
) bool {
    if refused && !l.kiteWalkDead {
        // The evidence verdict: the server answered the click with
        // ActionFailed inside the round trip.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "was refused by the server (standing on %d %d), "+
            "holding ground at the probe age",
            l.target, selfX, selfY)
    } else if !refused && !l.kiteWalkDead &&
        now.Sub(l.kiteWalkIssuedAt) >= kiteProbeElapsed {
        // The silent probe: the click stayed unanswered past the broadcast
        // gate and the character still stands on the issue cell.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "never started the movement (%d clicks out, standing "+
            "on %d %d), holding ground",
            l.target, l.kiteReclicks+1, selfX, selfY)
    }
    if !l.kiteWalkDead ||
        now.Sub(l.kiteWalkIssuedAt) < kiteProbeElapsed {
        // No verdict yet, or the walk is younger than the broadcast gate:
        // the re-click keeps hitting the endpoint.
        return true
    }
    // The dead endpoint stands the ladder down and the hold keeps the bow
    // shooting.
    l.kiteWalkClear()
    l.kiteHoldGround(now, false)

    return false
}

// kiteWalkClear clears the re-click ladder and its movement hold; another
// owner's hold keeps its window. The endpoint fields stay for the diagnostics.
func (l *Loop) kiteWalkClear() {
    if l.combatAvoidUntil.Equal(l.kiteWalkUntil) {
        l.combatAvoidUntil = time.Time{}
    }
    l.kiteWalkUntil = time.Time{}
    l.kiteWalkIssuedAt = time.Time{}
    l.kiteReclicks = 0
    l.kiteWalkDead = false
}

// kiteHoldGround arms the hold: the archer stops retreating and keeps
// shooting at melee range. The hold answers a blocked lane, a refused click or
// an encircled train, re-probes at the kite period and logs once per episode.
func (l *Loop) kiteHoldGround(now time.Time, surrounded bool) {
    fresh := l.kiteHeldFor != l.target ||
        l.kiteHeldAt.IsZero() ||
        now.Sub(l.kiteHeldAt) >= kiteHoldLogPeriod
    if fresh {
        switch {
        case l.target == 0:
            // The targetless hold belongs to the defensive flight
            // (kiteFleeThreat): the full pocket keeps the ticks and re-probes
            // at the kite period.
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

// rotatePlanar rotates a planar unit direction counterclockwise.
func rotatePlanar(x, y, angle float64) (float64, float64) {
    cos, sin := math.Cos(angle), math.Sin(angle)

    return x*cos - y*sin, x*sin + y*cos
}
