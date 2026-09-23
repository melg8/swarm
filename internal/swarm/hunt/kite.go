// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kiting layer of the archer fight (issue #13, the edge case
// slice of issue #19): a bow user that fights a mob from the weapon
// range never wants the mob in its face. The Mobius server AI stands
// the archer still while the auto attack shoots, and a melee mob that
// closes the distance simply swings away - the standing archer trades
// blow for blow with a mob it could outrange. The kite step answers
// exactly that: while the fight runs, a hostile that closed inside
// the retreat radius (the mob is a second or two from melee) makes
// the character step away, the movement window pauses the forced
// attack re-requests (a forced attack request interrupts a running
// walk server-side, the same rule the impending-add step of
// loop_avoid.go follows), and once the step finished the engage
// re-requests the attack - the bow shoots again from the opened
// distance. The cycle - stand, shoot, step clear, shoot - is the
// kiting behavior of the issue: the damage slows by the step
// overhead, the melee uptime drops to zero on a ground the mob cannot
// cross faster than the character.
//
// The edge case layer of this slice (issue #19) rides the same step
// the core round armed:
//
//   - the train direction: the retreat direction weighs EVERY chaser
//     on the character (the centroid away-vector of the fight target
//     plus the SelfAttackers train), not the armed threat alone - a
//     train member dragging west bends the retreat west before it
//     becomes the pile up. A train that surrounds the character (the
//     summed away vectors cancel - chasers stand on every side) is
//     too wide to outrun: the archer holds ground and shoots through
//     it. The neighboring camps never join the train: a retreat lane
//     that would wake an idle aggressive mob (the straight step
//     enters its on-sight trigger circle) deflects onto the tangent
//     ray of that circle - the same aggro-aware steering the transit
//     walks use (tangentClearDirection of loop_avoid.go).
//   - the corner: when no walkable retreat lane exists - the zone
//     leash, a wall behind (the geodata line of sight), water behind
//     (the OverWater oracle), or a lane every fan candidate of the
//     away hemisphere fails on - the archer stops retreating and
//     KEEPS SHOOTING THE BOW at melee range: no weapon switch, no
//     standing idle (the archetype rule of issue #21 - the bow is the
//     always-weapon, a cornered kite never degrades to a melee
//     trade). The cornered hold re-probes at the kite pacing, not
//     every tick - no idle stutter of failed probes - and the hunt
//     loop's watchdogs stay quiet through it: the stuck timeout only
//     owns a fight that never started (the cornered fight runs - the
//     swings keep it fresh), the chase stall watchdog only owns the
//     stretches beyond the bow stall radius (the cornered fight sits
//     at melee range, well inside it).
//   - the lane quality: the step prefers the open backward lanes
//     over the blocked corridors - the straight away-ray first, then
//     the fan candidates at 45 degree steps around the away
//     hemisphere; the wall, water, leash and camp gates run per
//     candidate, so a dead-end lane is re-planned at the very next
//     probe (one hop) instead of walking the character into it, and
//     no deflected lane ever folds back into the train (the away
//     half-plane gate).
//
// The honest limits of the layer, by the edge cases the issue names:
//   - the chaser positions of the DIRECTION scan are the raw
//     last-known packet positions (SelfAttackers), not the projected
//     ones - the centroid needs every chaser, the projected scan only
//     serves the nearest one (the armed threat of kiteThreat); a
//     chaser mid-chase lags its broadcast by up to a second, which
//     the 250 unit trigger margin absorbs.
//   - a retreat the gates clear but the server refuses (the click
//     validation collapses it, a mob body blocks the click point)
//     surfaces through the ordinary move-start watchdog machinery,
//     not through the kite - the kite re-probes on its pacing and
//     picks the next fan candidate then.
//   - the streak limit of the core stays: a chaser at least as fast
//     as the character never falls behind, and past the limit the
//     archer stops shuffling and fights it out.
//   - attack animation/travel time: the bow arrow flies ~500ms; the
//     step window pauses the re-requests but the server keeps the
//     already flying shots - a shot fired a moment before the step
//     still lands. Not modeled beyond that.
//
// The shot-paced layer of issue #60 rides the same machinery and
// flips the trigger: the character's own Attack broadcast is the
// server commit of the shot (the Mobius doAttack rolls the hit,
// consumes the arrow and schedules the HitTask BEFORE the packet
// goes out, and the damage task carries no attacker movement check),
// so the retreat starts the moment the shot released - inside the
// bow cooldown (timeAtk + reuse, roughly three seconds for the short
// bow) instead of after the mob crossed the retreat radius. The
// cycle becomes shoot, run the reload, gain distance, shoot again:
// the melee uptime drops to the stand moments, a chaser slower than
// the character never reaches the swings. The shot-paced step paces
// itself on the shot cycle, shares the lane battery and the holds
// with the proximity path and skips the streak counting - the
// shuffle bound exists for the swing-starving race, the rhythm keeps
// shooting once per cooldown.
//
// The re-click ladder of the issue #60 second round owns the WALK
// of the retreat: the owner's follow-up dump showed the rhythm
// trigger working (a "shot released" line every cycle) with the
// character still standing through every reload ("moving: no",
// the mob closing ~380 units per cycle) - the single retreat click
// barely executed. The manual clicking the owner demonstrated
// ("clicking behind the character runs 2 seconds no problem") is
// the recovery the ladder replicates: while the movement window
// runs and the character stands still on the issue cell, the click
// goes out again at the 600ms pace - the same endpoint first, then
// (once the probe names the endpoint click dead at the second
// re-click) the next fan candidate, then the rotated endpoint once
// more. The three candidate mechanisms the ladder answers: the
// stance transition swallow (the click lands while the server side
// ATTACK intention of the just-broadcast shot is still tearing
// down), the destination-cell refusal (the server refuses the cell
// the bot side LineOfSight blessed) and the H-006 silent drop (the
// deployment that swallows accepted move requests). The probe line
// and the rotation line of the ladder are the dump markers that
// name which mechanism ate the clicks of a given fight.
//
// The behavior arms itself on the weapon in hand (bowEquipped): no
// config, no class check - whatever bot ends up holding a bow (the
// planned archer type of the config launch, a lured guard round)
// kites its fights.

import (
    "math"
    "sort"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The kiting constants of the archer fight.
const (
    // kiteRetreatRadius is the trigger distance of the kite step and
    // the INNER edge of the optimal band: any bow fight hostile (the
    // target or a train member) closer than this to the fighting
    // character is about to reach melee (the swing range sits around
    // 150 units and the chasers run 120+ units per second), so the
    // step fires before the blows land. Above it the archer stands
    // and shoots - the standing fight inside the optimal band is the
    // good case.
    kiteRetreatRadius = 250.0
    // kiteStep is the retreat length away from the closed threat:
    // sized so the distance after the step (the mob follows at its
    // run speed through the ~2s window) lands back inside the
    // optimal band - the forced attack re-request then shoots from
    // range instead of starting a server side chase that would walk
    // the distance right back in.
    kiteStep = 400.0
    // kiteStepWindow is the movement window the step owns: the walk
    // runs ~400 units (about two seconds at the run speed), and a
    // forced attack request would interrupt it server-side, so the
    // engage holds its re-requests until the window closes - the
    // same contract the impending-add step uses
    // (combatAvoidStepPeriod).
    kiteStepWindow = 2 * time.Second
    // kiteReengageDelay is the hold between the step walk ending and
    // the first forced attack re-request of the re-engage: zero
    // today - the window end IS the re-engage (the opened distance
    // sits inside the optimal band, the re-request shoots from
    // range), the knob is pinned here so the live tuning round (#20)
    // can hold the aim without touching the walk contract.
    kiteReengageDelay time.Duration = 0
    // kiteStepPeriod paces the steps: the kite cycle spends the
    // window walking and the rest standing and shooting; the period
    // bounds the walking share from above (2s walk, 1s shoot at the
    // fastest). The cornered hold re-probes on the same pacing.
    kiteStepPeriod = 3 * time.Second
    // kiteStreakLimit bounds the consecutive kite steps of one
    // target: a chaser at least as fast as the character never falls
    // behind (the step opens no net distance), and an endless kite
    // shuffle would starve the fight of every swing. Past the limit
    // the archer stops kiting THIS target and fights it out - the
    // losing fight machinery (losingFight, the panic run) still owns
    // the death risk.
    kiteStreakLimit = 8
    // kiteTrainScanRange bounds the chaser scan of the retreat
    // DIRECTION: a train member farther than this from the character
    // cannot reach it inside a step window, so it cannot drag the
    // centroid - the flee machinery owns chasers that distant. The
    // value matches the combat avoid scan band (the impending-add
    // step reads the same neighborhood).
    kiteTrainScanRange = 800.0
    // kiteEncircleShare is the encirclement threshold of the train:
    // when the summed away vectors of the chasers shrink under this
    // share of their count (the unit vectors cancel - chasers stand
    // on every side), the centroid names no retreat direction and
    // the widest-gap bisector takes the step over (kiteGapDirection
    // - the live fleet round of issue #70 measured the standing
    // encircled fight as a death trap on the mass cells). A single
    // chaser always sums to a full unit (share 1.0); two chasers on
    // opposite sides sum to ~0.
    kiteEncircleShare = 0.3
    // kiteMaxTrainMembers caps the bearing scratch of the retreat
    // direction (kiteBearings): the gap geometry of a wider train
    // reads its nearest members alone - a train that wide outranks
    // the kite (the flee machinery's logout case), and the scratch
    // must stay a fixed array to keep the resolution allocation
    // free.
    kiteMaxTrainMembers = 16
    // kiteFanStep is the rotation increment of the retreat lane fan:
    // the straight away-ray first, then the candidates at its
    // multiples - a wall or a camp blocking the straight lane leaves
    // the 45 degree lanes, the leash corner leaves the 90 degree
    // ones.
    kiteFanStep = math.Pi / 4
    // kiteFanSteps bounds the fan on each side of the away-ray (2:
    // the 45 and 90 degree candidates each side). The fan spans the
    // away hemisphere only - a wider bend would step toward the
    // chasing train, and the half-plane gate rejects any deep camp
    // deflection that folds a lane back the same way.
    kiteFanSteps = 2
    // kiteHalfPlaneSlack is the tolerance of the away half-plane
    // gate: a lane may run perpendicular to the away-ray (the camp
    // side-step exits the trigger circle sideways), but never fold
    // back toward the train - the guard rejects the endpoints whose
    // retreat component turned negative beyond the float noise.
    kiteHalfPlaneSlack = -0.05
    // kiteCurveStep is the tangential bearing of the curving retreat
    // (issue #70, the fifth behavior): after the opening straight
    // retreat of a fight every later retreat leans this far off the
    // away-ray, on a turn side the fight keeps for its whole life -
    // the character circles the fight instead of marching one ray,
    // and the drift away from the farm point stays bounded by the
    // circle's radius instead of the leash. The bearing rides the
    // away half-plane (a mob that circled behind or a cornered
    // hemisphere mirrors the turn side rather than fold a lane
    // back into the train).
    kiteCurveStep = 70 * math.Pi / 180
    // kiteReshotFloor is the re-shot gate of the pursuit hold: the
    // distance under which the next bow windup would drag the
    // hostile down to melee - the windup debt (the ~1.45 s movement
    // standstill the live probe measured times the 110 run speed of
    // the elven-ground chasers the Mobius tables carry, ~160 units)
    // plus the retreat radius (the melee danger line the proximity
    // step holds). The fleet rounds of issue #70 measured the cycle
    // without the gate: the re-shot fires at the walk-window end,
    // restarts the windup, and the fight distance spirals to the
    // 13-68 unit medians of the max-range verdicts - the walk banks
    // ~190 units a window only to hand ~160 back through the
    // standstill. Above the floor the shot is affordable wherever
    // the fight stands; under it the pursuit continues the walk
    // (the server keeps the auto-attack displaced while the
    // character moves, and a 110 chaser cannot hit what it cannot
    // catch).
    kiteReshotFloor = 410.0
    // kiteHoldLogPeriod paces the cornered hold diagnostic: the hold
    // itself re-probes at the kite pacing, the log line lands once
    // per period - a cornered fight is a standing fight, the line is
    // its visibility, not a complaint.
    kiteHoldLogPeriod = 15 * time.Second
    // kiteShotWindow is the freshness window of the character's own
    // Attack broadcast that arms the shot-paced retreat (issue #60):
    // the packet is the server commit of the shot (the hit roll, the
    // arrow consumption and the HitTask schedule all happened before
    // the broadcast, and the damage task carries no attacker movement
    // check). The broadcast arms the DEFERRED retreat schedule (see
    // kiteArmClick - the issue #70 findings: the server forbids the
    // movement through the windup, so the click waits the windup end
    // instead of going out at the broadcast). The loop ticks four
    // times a second - the window keeps the broadcast catchable for
    // at least two ticks while still bounding the schedule to the
    // shot it belongs to.
    kiteShotWindow = 700 * time.Millisecond
    // kiteReclickPeriod paces the re-click ladder of the kite walk
    // (issue #60, the second round - the owner feedback: the bot
    // click "doesnt cover enough distance, dont start running",
    // while the manual clicking behind the character "runs 2
    // seconds no problem, covering large distance"): while the kite
    // movement window runs and the character still stands on the
    // issue cell, the retreat click goes out again at this pace -
    // the manual behavior the owner demonstrated, replicated by
    // the bot one click at a time. The period sits under the
    // server's once per second movement broadcast gate on purpose:
    // an accepted click may take up to a second to answer with the
    // broadcast, and a re-click that lands inside that gate just
    // re-aims the same endpoint server-side (the walk continues,
    // the click is not wasted) - exactly what the repeated manual
    // clicks do.
    kiteReclickPeriod = 250 * time.Millisecond
    // kiteReclickLimit bounds the re-clicks of one kite walk: the
    // initial click plus this many re-issues - the owner ask of the
    // third round is the FASTER spam ("it should spam clicks much
    // faster"), so the bound pins the packet budget of a dead
    // transport while the 250ms pace fills the whole window with
    // retries.
    kiteReclickLimit = 8
    // kiteProbeElapsed is the silent-verdict age of the walk: the
    // initial click plus the re-clicks stayed unanswered for this
    // long while the character still stands on the issue cell (no
    // ActionFailed, no movement broadcast - the H-006 silent drop
    // signature) names the endpoint click dead the way the probe
    // does. The value sits past the server's once-per-second
    // movement broadcast gate plus the pacing headroom - a healthy
    // accepted click answers with the broadcast inside it.
    kiteProbeElapsed = 1200 * time.Millisecond
    // kiteRefusalAnswerWindow bounds the ActionFailed attribution of
    // the kite clicks: the genuine refusal answer of a refused
    // endpoint arrives within the round trip (the acceptance dump
    // shows it landing the same millisecond), so a failure older
    // than one tick past the click belongs to some other request,
    // not the click. The window is deliberately TIGHT: the server
    // AI's own attack retries answer ActionFailed at a one-second
    // cadence through the whole bow disable (Creature.doAttack
    // schedules notifyActionReadyToAct a second ahead, each retry
    // on a still-disabled bow bounces) - a wide window would
    // misattribute that cadence to the kite clicks and rotate
    // healthy endpoints away (the local acceptance round caught
    // exactly that cascade before the tightening).
    kiteRefusalAnswerWindow = 250 * time.Millisecond
    // kiteBowReuseDelay is the reuse delay of the bow family the
    // fleet shoots (the C1 item data: the Short Bow and the D-grade
    // bows carry 1500, dist/game/data/stats/items - the value feeds
    // the bow disable window formula of kiteWalkWindow).
    kiteBowReuseDelay = 1500.0
    // kiteWalkWindowMin/Max clamp the bow-aware walk window: the
    // formula reads the live pAtkSpd, and a broken broadcast (a
    // zero, a partial) must never collapse or explode the window -
    // the clamp keeps the walk inside the sane attack-speed band
    // and falls back to the shipped kiteStepWindow outside it.
    kiteWalkWindowMin = 1 * time.Second
    kiteWalkWindowMax = 5 * time.Second
    // kiteWindupLead is the safety lead the deferred retreat click
    // adds past the windup end before it goes out (issue #70, the
    // kite timing findings): the live ladder measured the accepted
    // move window OPENING at the windup end - a click 1500 ms after
    // the shot moved at once (1.52 s) while a 1200 ms one stood to
    // the cycle end (2.96 s), the boundary sitting at the
    // theoretical (timeAtk+reuse)/2 = 1483 ms of pAtkSpd 337 - so
    // the click must land safely PAST that boundary despite the
    // quarter-second loop cadence and the broadcast lag. The lead
    // trades 150 ms of the walk tail for the certainty the click
    // rides the accepted window instead of the deferred one (the
    // deferred click is replayed only at the disable end where the
    // re-shot cancels it - the whole-reload stall the findings
    // measured as the fleet retreat lag).
    kiteWindupLead = 150 * time.Millisecond
)

// KiteParams is the tunable block of the kite fight (owner issue
// #29): the four named knobs the live tuning round adjusts from the
// measured numbers, plus the enable gate. The launch config carries
// one per bot spec (the "kite" section of launch_config.go), so the
// standing-archer baseline (a bow bot with the kite disabled) and the
// tuned profile switch by editing the file instead of a code edit and
// a rebuild between the measurement runs. Every remaining kite
// constant (the streak limit, the train scan, the lane fan) stays a
// package constant on purpose: the tuning round names these four,
// the rest ride the shipped values until the numbers ask otherwise.
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
// above read into the params shape. A loop starts with these - the
// config file overrides per bot spec, everything else keeps the
// behavior the acceptance scenario (#28) pins.
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
// non-positive number or period falls back to the shipped value (a
// broken config line degrades to the shipped tuning, never to a
// degenerate fight), the re-engage delay keeps zero - it is a
// legitimate value (the window end IS the re-engage) - so only the
// negative side normalizes. The kite state carries over: the override
// between fights never resets the pacing or the streak.
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

// The optimal band of the kite fight is the distance range where the
// standing archer is the good case: the inner edge is
// kiteRetreatRadius (a hostile below it arms the next step), the
// outer edge is the bow engage radius of the weapon-aware radii
// (userBowEngageRadius, user.go - 450), where the forced attack
// re-request shoots from range. The band is pinned as these two
// edge references on purpose - a copy of the engage radius here
// would drift from the radii the fight actually fights with - and
// the kiteStep length is sized against both edges.

// kiteThreat locates the hostile that arms the kite step: the fight
// target or the nearest train member - a living mob that already
// chases the character (its server target is the character, the
// projected NearestAttacker scan reads it, see scans.go) - whichever
// sits closer. A mob on a deck the walk cannot reach (the z gap past
// deckReachableZ, the same test the pick uses) is no melee threat.
// Returns the threat identity for the logs and its ground distance
// to the character (the retreat DIRECTION weighs the whole train
// through kiteTrainDirection, so the position itself has no reader).
func (l *Loop) kiteThreat(
    selfX, selfY, selfZ int32,
) (id int32, dist float64, ok bool) {
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        dx := float64(selfX) - float64(tx)
        dy := float64(selfY) - float64(ty)
        id, dist, ok = l.target, math.Hypot(dx, dy), true
    }
    if attacker, aok := l.tracker.NearestAttacker(); aok &&
        attacker.ObjectID != l.target {
        if attacker.Z == 0 ||
            math.Abs(float64(attacker.Z-selfZ)) <= deckReachableZ {
            dx := float64(selfX) - float64(attacker.X)
            dy := float64(selfY) - float64(attacker.Y)
            adist := math.Hypot(dx, dy)
            if !ok || adist < dist {
                id, dist, ok = attacker.ObjectID, adist, true
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
    if now.Before(l.combatAvoidUntil) {
        return false
    }
    if !l.kiteHeldAt.IsZero() && l.kiteHeldFor == l.target &&
        now.Sub(l.kiteHeldAt) < l.kite.StepPeriod {
        return false
    }

    return true
}

// kiteStepAdmitted guards the kite step of one tick: the shared
// layer gates must pass (see kiteLayerGates), the kite pacing must
// have aged out (the step re-probe), the streak limit must not be
// spent, and a fresh target resets the streak of the previous one
// (the distance race of one mob is not the race of the next).
// Reports whether the step may run now.
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
    if l.kiteStreak >= kiteStreakLimit {
        // The distance race is unwinnable: stop the shuffle, fight.
        return false
    }

    return true
}

// kiteFromTarget steps a bow fighting character away from the
// hostile that armed the step (the target or a train member, see
// kiteThreat): the archer buys back the weapon range instead of
// tanking the melee. The step DIRECTION is the centroid away-vector
// of the whole chaser train (the target plus every attacker in
// range, see kiteTrainDirection), the lane prefers the open backward
// lanes over the blocked corridors (the 45 degree fan candidates
// behind the straight ray), and a neighborhood camp the lane would
// wake deflects it onto the tangent ray of the camp's trigger
// circle. A train that surrounds the character or a retreat with no
// walkable lane resolves to the cornered hold: the archer stops
// retreating and keeps shooting the bow at melee range - never a
// weapon switch (the archetype rule of issue #21). The step
// respects the movement window contract of the fight steps (the
// forced attack re-requests wait out the walk), and the streak limit
// (an unwinnable distance race falls back to the ordinary fight). A
// fresh target resets the streak. The step timing honors the live
// shot cycle (kiteShotPhase): inside the bow windup the click
// defers to the windup end (the server would save an immediate one
// and replay it at the disable end where the re-shot cancels it),
// inside the accepted move window the click goes out at once with
// the cycle end as its window, and with no live cycle the ordinary
// full window applies. Reports whether the tick issued or armed the
// step (the caller skips the rest of the fighting branch then - the
// walk owns the movement).
func (l *Loop) kiteFromTarget(now time.Time) bool {
    if !l.kiteStepAdmitted(now) {
        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= l.kite.RetreatRadius || dist < 1 {
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
        until = now.Add(l.kiteWalkWindow() + l.kite.ReengageDelay)
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered (see
        // kiteResolveAndClick): the hold ground rule owns the cycle.
        return false
    }
    l.kiteStreak++
    l.logger.Printf("Hunt: hostile %d closed to %d units of the "+
        "fight on %d, kiting clear (step %d of %d)",
        threatID, int(math.Round(dist)), l.target,
        l.kiteStreak, kiteStreakLimit)

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
// banks the reload tail - the shoot, run the reuse tail, re-shoot
// cycle the issue asks for. The retreat fires once per shot while a
// hostile holds the pursue band (inside the bow engage radius - a
// mob that keeps chasing the fight), the lane machinery (the train
// direction, the camp deflection, the leash, the wall and the water
// gates, the cornered hold) resolves at the click moment - the
// chasers move while the character stands the windup out - and the
// rhythm does NOT count toward the streak limit: the shots never
// starve. A hostile beyond the band leaves the standing fight (the
// stall watchdog owns the re-approach). Reports whether the tick
// armed the deferred retreat.
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
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= userBowEngageRadius || dist < 1 {
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

// kiteResolveAndClick resolves the retreat lane against the live
// train and terrain and issues the retreat click through the ladder
// seam: the encircled train (the away vectors cancel - chasers stand
// on every side) and the cornered battery (no walkable lane in the
// whole away hemisphere) answer with the hold ground rule of the
// archetype (never a weapon switch - the bow is the always-weapon),
// a walkable lane clicks at once with the movement window the caller
// computed. The shared seam serves the deferred click of the shot
// cycle (kiteClickWalk), the accepted-window catch of the broadcast
// tick and the ordinary proximity step - one resolution, one click
// path, one ladder. Reports whether the click went out.
func (l *Loop) kiteResolveAndClick(
    now, until time.Time, selfX, selfY, selfZ int32,
) bool {
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        // The degenerate no-chaser scene (a tracker gap inside the
        // broadcast window): no ray to guard, the hold keeps the
        // pacing until the next probe.
        l.kiteHoldGround(now, true)

        return false
    }
    // The curving retreat (issue #70, the fifth behavior): the
    // opening retreat of a fight runs the straight away-ray, every
    // later one leans the fixed tangential bearing on the fight's
    // turn side - the circle. The RAW away vector stays the
    // half-plane reference of the lane battery (the curve may bend
    // the preferred ray, never the guard). An encircled train
    // KEEPS its raw ray as the preference: the gap bisector already
    // encodes the escape geometry, and a tangential lean on top of
    // it would fold the step back onto a flanker.
    prefX, prefY := dirX, dirY
    if !encircled {
        prefX, prefY = l.kiteCurveDirection(dirX, dirY, selfX, selfY)
    }
    stepX, stepY, found := l.kiteRetreatLaneSkipping(
        selfX, selfY, selfZ, prefX, prefY, dirX, dirY,
        l.kiteDeadCellsFor())
    if !found {
        // Cornered: no walkable lane in the whole guarded
        // hemisphere (the raw away-ray of a normal train, the gap
        // ray of an encircled one). The hold ground answer is the
        // archetype rule - the bow is the always-weapon, the
        // cornered archer never switches to a melee trade, it
        // stands and shoots the way out.
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
// and the lane itself (the chasers moved while the character stood -
// the train direction, the camp deflection and the dead-cell memory
// all resolve fresh at the click moment). The encircled and the
// cornered answers hold the ground as always. Reports whether the
// tick spent the click.
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
        if l.combatAvoidUntil.Equal(l.kiteClickUntil) {
            l.combatAvoidUntil = time.Time{}
        }
        l.kiteClickClear()

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
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= userBowEngageRadius || dist < 1 {
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
// own schedule whole. The dead-cell memory and the ladder state
// belong to their own lifecycles (the target and the walk).
func (l *Loop) kiteClickClear() {
    l.kiteClickAt = time.Time{}
    l.kiteClickFor = 0
    l.kiteClickUntil = time.Time{}
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
    until = shotAt.Add(l.kiteWalkWindow() + l.kite.ReengageDelay)
    if now.After(until) {
        // The disable lapsed: the re-engage owns the moment, and a
        // retreat riding the dead cycle would hold its window for
        // nothing.
        return time.Time{}, time.Time{}, false
    }
    clickAt = shotAt.Add(l.kiteWindupWindow() + kiteWindupLead)

    return clickAt, until, true
}

// kiteCurveDirection bends the retreat ray into the curving circle
// of the fight (issue #70, the fifth behavior): the OPENING retreat
// of a fight runs the straight away-ray (the panic retreat opens
// the distance at once), every later one leans the fixed tangential
// bearing kiteCurveStep on the fight's turn side - the character
// circles the fight instead of marching one ray, so the drift away
// from the farm point stays bounded by the circle. The turn side
// is the fight's own constant (kiteCurveSide picks it once: toward
// the hunting zone center when the leash knows one, a fixed side
// otherwise). The step itself stays under the half-plane bound
// (cos(kiteCurveStep) >= kiteHalfPlaneSlack, pinned by a unit
// test), so the bent ray never folds back into the train whatever
// the turn side. The advance to the curved steps belongs to the
// issued walk alone (kiteIssueWalk): a cornered or refused retreat
// spends nothing, the opening straight step stays armed until a
// walk really goes out.
func (l *Loop) kiteCurveDirection(
    dirX, dirY float64, selfX, selfY int32,
) (float64, float64) {
    if l.kiteCurveFor != l.target {
        l.kiteCurveFor = l.target
        l.kiteCurved = false
        l.kiteCurveSign = l.kiteCurveSide(dirX, dirY, selfX, selfY)
    }
    if !l.kiteCurved {
        return dirX, dirY
    }

    // The constant contract (pinned by TestKiteCurveStepStaysInTheAwayHalfPlane):
    // kiteCurveStep stays under the half-plane bound
    // (cos(step) >= kiteHalfPlaneSlack), so the bent ray can never
    // fold back into the train whatever the turn side - the guard
    // would mirror the side otherwise, but a step that needs the
    // mirror has no business being a circle bearing.
    bentX, bentY := rotatePlanar(dirX, dirY, l.kiteCurveSign*kiteCurveStep)

    return bentX, bentY
}

// kiteCurveSide picks the turn side of a fight's circle once: the
// side whose tangential ray leans back toward the hunting zone
// center when the leash knows one (the circle bends the drift back
// toward the farm point), a fixed counterclockwise side otherwise.
func (l *Loop) kiteCurveSide(
    dirX, dirY float64, selfX, selfY int32,
) float64 {
    if l.zoneHalf == 0 {
        return 1
    }
    toCenterX := float64(l.zoneCX - selfX)
    toCenterY := float64(l.zoneCY - selfY)
    if len := math.Hypot(toCenterX, toCenterY); len >= 1 {
        ccwX, ccwY := rotatePlanar(dirX, dirY, kiteCurveStep)
        if ccwX*toCenterX/len+ccwY*toCenterY/len >=
            dirX*toCenterX/len+dirY*toCenterY/len {
            return 1
        }

        return -1
    }

    return 1
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
    l.kiteWalkUntil = until
    l.combatAvoidUntil = until
    // The curving circle advances on the issued walk alone: the
    // opening straight retreat of the fight is spent, every later
    // retreat leans the tangential bearing (kiteCurveDirection).
    l.kiteCurved = true
    l.kiteReclickAt = now
    l.kiteReclicks = 0
    l.kiteWalkDead = false
    if l.kiteDeadFor != l.target {
        // A fresh fight: the dead-cell memory of the previous
        // target is history - the terrain refusal verdicts belong
        // to the ground the OLD fight stood on. The memory of THIS
        // target persists across the walk cycles (the cells the
        // server refused stay refused - the terrain does not
        // change between the shots, and the later cycles start on
        // a lane that already proved walkable instead of paying
        // the probe tax on the same dead straight cell every
        // cycle).
        l.kiteDeadFor = l.target
        l.kiteWalkDeadCount = 0
    }
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
func (l *Loop) kiteWindupWindow() time.Duration {
    return l.kiteWalkWindow() / 2
}

// kiteWalkWindow resolves the movement window the kite walk owns:
// the bow disable window the C1 server enforces between the shots -
// timeAtk + reuse = 500000/pAtkSpd + reuseDelay*333/pAtkSpd
// (Creature.calculateTimeBetweenAttacks and calculateReuseTime, the
// reuseDelay of the bow family in the item data) - read from the
// live pAtkSpd the StatusUpdate broadcasts (attr 0x12, the value
// the acceptance dump shows as 337 for the Short Bow kit, giving
// roughly (500000 + 1500*333)/337 = 2.97s - the "3 seconds to draw
// shot" the owner measured by hand). The walk then spends the WHOLE
// cooldown running (the owner ask of the third round: the proper
// archering delays from the C1 Mobius formulas - if it is not
// shooting it should be moving away from the target) instead of the
// shipped fixed 2s that stood the character idle for the last
// second of every reload. A missing or absurd pAtkSpd broadcast
// falls back to the shipped kiteStepWindow (the walk contract
// never depends on the packet being parsed).
func (l *Loop) kiteWalkWindow() time.Duration {
    spd := l.tracker.SelfPAtkSpd()
    if spd <= 0 {
        return kiteStepWindow
    }
    ms := (500000.0 + kiteBowReuseDelay*333.0) * float64(time.Millisecond) /
        float64(spd)
    if ms < float64(kiteWalkWindowMin) || ms > float64(kiteWalkWindowMax) {
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

    return failedAt.After(l.kiteReclickAt) &&
        failedAt.Sub(l.kiteReclickAt) <= kiteRefusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.kiteReclickAt, failedAt)
}

// kiteRotateDeadEndpoint rotates the dead endpoint of the ladder
// onto the next fan candidate: the dead cell joins the refused set
// (the rotation never re-clicks a cell the server already refused -
// a refused cell never starts a walk) and the lane battery resolves
// the next walkable lane skipping the whole set, the fresh train
// direction included (the chasers moved while the character stood).
// Reports whether a fresh endpoint serves the retreat: an
// encircled train or a battery that finds no lane leaves the
// rotation empty and the caller stands the ladder down.
func (l *Loop) kiteRotateDeadEndpoint(
    selfX, selfY, selfZ int32,
) bool {
    if l.kiteWalkDeadCount < len(l.kiteWalkDeadCells) {
        l.kiteWalkDeadCells[l.kiteWalkDeadCount] =
            [2]int32{l.kiteWalkX, l.kiteWalkY}
        l.kiteWalkDeadCount++
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        return false
    }
    // The rotation stays DIRECTION-CENTERED (the raw ray the
    // half-plane guards - the centroid of a normal train, the gap
    // bisector of an encircled one): the dead-endpoint recovery is
    // a coverage sweep of the whole hemisphere, the circle owns
    // the fresh retreat preference alone - a rotation that clung
    // to the curved ray would sweep only the three candidates left
    // of its fan.
    stepX, stepY, found := l.kiteRetreatLaneSkipping(
        selfX, selfY, selfZ, dirX, dirY, dirX, dirY,
        l.kiteWalkDeadCells[:l.kiteWalkDeadCount])
    if !found {
        return false
    }
    l.logger.Printf("Hunt: rotating the kite retreat of target %d "+
        "onto the lane %d %d (the endpoint %d %d refused or silent)",
        l.target, stepX, stepY, l.kiteWalkX, l.kiteWalkY)
    l.kiteWalkX, l.kiteWalkY = stepX, stepY

    return true
}

// kitePursuitHold answers whether a lapsed kite walk re-arms as the
// pursuit continuation of the same fight (issue #70, the max-range
// behavior): the fleet rounds measured the walk-window cycle losing
// the distance race - the re-shot at the window end restarts the
// windup standstill, the 110-speed chaser collects ~160 units of it
// back, and the fight spirals to the melee medians of the max-range
// verdicts. The issue's contract inverts the priority: the retreat
// strives for the maximum distance FIRST, the re-shot waits for it.
// While the nearest hostile holds inside the re-shot floor (the
// windup debt plus the retreat radius - the distance the next
// windup would eat down to melee) the lapsed window re-arms one
// more walk through the ordinary lane resolution, the streak counts
// it, and the server keeps the auto-attack displaced while the
// character moves (a 110 chaser cannot hit what it cannot catch).
// The first moment the hostile clears the floor - or the streak
// budget, the leash or the lane battery stops the chase - the walk
// stands down and the re-request fires the affordable shot from the
// opened distance. Reports whether the tick spent the continuation.
func (l *Loop) kitePursuitHold(
    now time.Time, selfX, selfY, selfZ int32,
) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    if l.kiteStreak >= kiteStreakLimit {
        // The distance race is unwinnable (the leash or the terrain
        // owns the ground): the standing fight keeps the damage on.
        return false
    }
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= kiteReshotFloor || dist < 1 {
        // The shot is affordable (or the scene is degenerate): the
        // re-request owns the tick.
        return false
    }
    until := now.Add(l.kiteWalkWindow() + l.kite.ReengageDelay)
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered the continuation
        // the same way it answers the opening step.
        return false
    }
    l.kiteStreak++
    l.logger.Printf("Hunt: hostile %d holds %d units at the walk "+
        "end, the pursuit continues the retreat (step %d of %d)",
        threatID, int(math.Round(dist)), l.kiteStreak, kiteStreakLimit)

    return true
}

// kiteReclickWalk is the re-click ladder of the kite retreat (issue
// #60, the rounds two and three): the single retreat click of the
// first round proved fragile three ways, and the owner's dumps show
// the character standing through the whole bow reload while the mob
// closed. The third-round acceptance dump named the dominant
// mechanism exactly: the server answers SOME destination cells with
// an instant bare ActionFailed (the MoveToLocation handler refusal
// family - the isCompletelyBlocked destination check among others)
// while the rotated fan lane a few cells aside walks fine, and the
// bot's own geodata reads the refused cells open, so only the ONLINE
// evidence settles it. The ladder answers with the manual behavior
// the owner demonstrated ("clicking behind the character runs 2
// seconds no problem"), faster now per the third-round ask:
//
//   - while the movement window runs and the character stands on
//     the issue cell, the click goes out again every
//     kiteReclickPeriod (250ms - the tick cadence, the owner ask:
//     "it should spam clicks much faster"); a re-click inside the
//     broadcast gate just re-aims the same endpoint server-side.
//   - the moment the refusal evidence lands (kiteClickRefused - the
//     ActionFailed answer the server gave the last click), the dead
//     endpoint rotates onto the next fan candidate AT ONCE: the
//     rotation no longer waits for the silent probe, the refused
//     cell joins the refused set and the battery skips the whole
//     set (a refused cell never starts a walk, re-clicking it
//     changes nothing).
//   - the silent probe stays as the fallback for the H-006 silent
//     drop (no ActionFailed at all): a walk unanswered past
//     kiteProbeElapsed with the character still on the issue cell
//     names the endpoint dead and rotates the same way.
//   - an encircled train or a lane battery with nothing left stands
//     the ladder down (the cornered hold owns the answer), the walk
//     starting clears it (the SelfWalking oracle), the character
//     leaving the issue cell clears it, the window ending clears it
//     (the forced attack re-request owns the tick).
//
// Reports whether this tick spent a re-click so the caller yields
// the tick to the walk.
func (l *Loop) kiteReclickWalk(now time.Time) bool {
    if l.kiteWalkUntil.IsZero() {
        return false
    }
    if now.After(l.kiteWalkUntil) {
        // The window ended: the pursuit hold asks first whether the
        // fight's distance still collapses under the re-shot floor
        // (the 400 unit step outlasts the window, so this is the
        // moment the walk really ends) - the continuation re-arms
        // the retreat, the ordinary stand-down hands the tick to
        // the forced attack re-request (the affordable shot).
        if selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition(); selfOK &&
            l.kitePursuitHold(now, selfX, selfY, selfZ) {
            return true
        }
        // The re-engage machinery owns the next
        // ticks (the forced attack re-request restarts the shooting
        // from the opened distance), and a re-click past it would
        // only fight the re-engage for the movement.
        l.kiteWalkClear()

        return false
    }
    if l.tracker.SelfWalking() {
        // The walk runs: the click landed, the manual-clicking
        // replication stops. The walk owns the movement until the
        // window ends server-side interrupts it.
        l.kiteWalkClear()

        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    if selfX != l.kiteWalkBaseX || selfY != l.kiteWalkBaseY {
        // The character left the issue cell: the click started
        // something (the ground truth moved before the broadcast -
        // or the walk already ran and finished short). The ladder
        // stands down, the next shot cycle re-arms it fresh.
        l.kiteWalkClear()

        return false
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
    if !l.kiteReclickVerdict(now, selfX, selfY, selfZ, refused) {
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
// and runs the rotation it demands. The evidence verdict comes first
// (the server answered the last click with ActionFailed - the probe
// line lands once per walk so the dump names the mechanism), the
// silent probe second (the walk stayed unanswered past the probe age
// with the character still on the issue cell - the H-006 silent drop
// signature). Either verdict arms the rotation: the refused set skips
// every cell already named dead, the fresh train direction included -
// the chasers moved while the character stood. Reports whether the
// re-click may still go out: a rotation with nothing left (encircled,
// every lane refused or blocked) stands the whole ladder down - the
// cornered hold owns the answer, the next shot cycle re-arms the
// retreat fresh.
func (l *Loop) kiteReclickVerdict(
    now time.Time, selfX, selfY, selfZ int32, refused bool,
) bool {
    if refused && !l.kiteWalkDead {
        // The evidence verdict: the server answered the click with
        // ActionFailed inside the round trip. The probe line lands
        // once per walk so the dump names the mechanism; the
        // ROTATION itself waits for the probe age below - a refusal
        // answer of the server AI's own attack retries (the bow
        // disable path schedules them a second apart, each retry on
        // a still-disabled bow bounces) rides the same packet and
        // only the age gate separates it from the movement
        // broadcast that may still be in flight for a healthy
        // click.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "was refused by the server (standing on %d %d), "+
            "rotating the retreat lane at the probe age",
            l.target, selfX, selfY)
    } else if !refused && !l.kiteWalkDead &&
        now.Sub(l.kiteWalkIssuedAt) >= kiteProbeElapsed {
        // The silent probe: the initial click, the plain re-clicks
        // and the character still stands on the issue cell past the
        // broadcast gate plus the pacing headroom - the movement
        // never started and no refusal arrived. One line per walk,
        // the rotation follows.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "never started the movement (%d clicks out, standing "+
            "on %d %d), rotating the retreat lane",
            l.target, l.kiteReclicks+1, selfX, selfY)
    }
    if !l.kiteWalkDead ||
        now.Sub(l.kiteWalkIssuedAt) < kiteProbeElapsed {
        // No verdict yet, or the walk is still younger than the
        // broadcast gate: the re-click keeps hitting the endpoint -
        // the spam the owner asked for, and the movement broadcast
        // of a healthy click still gets its chance to clear the
        // ladder before any rotation spends the fan battery.
        return true
    }
    if !l.kiteRotateDeadEndpoint(selfX, selfY, selfZ) {
        // The rotation found nothing: every candidate of the away
        // hemisphere is refused or blocked - the cornered answer
        // owns the cycle, the next shot re-arms the retreat fresh
        // (the dead-cell memory persists per target, the next
        // cycle starts on what the battery has left).
        l.logger.Printf("Hunt: no walkable retreat lane left on "+
            "target %d (every candidate refused or silent), "+
            "holding ground", l.target)
        l.kiteWalkClear()

        return false
    }

    return true
}

// kiteDeadCellsFor returns the dead-cell memory of the CURRENT
// fight: the cells the server refused (or that stayed silent
// through the probe) stay skipped by the initial lane resolution of
// every later cycle of the same target - the terrain does not
// change between the shots, and a cycle that starts on a lane that
// already proved walkable never pays the probe tax on the same dead
// straight cell again. A memory of another target (or an empty one)
// answers nil - the plain resolution.
func (l *Loop) kiteDeadCellsFor() [][2]int32 {
    if l.kiteDeadFor != l.target || l.kiteWalkDeadCount == 0 {
        return nil
    }

    return l.kiteWalkDeadCells[:l.kiteWalkDeadCount]
}

// kiteWalkClear stands the re-click ladder down: the endpoint fields
// stay (they name the walk of record for the diagnostics), the gate
// (kiteWalkUntil), the issue stamp, the click count and the probe
// latch reset - the next kite step arms the ladder whole through
// kiteIssueWalk. The dead-cell memory STAYS: it belongs to the
// target, not the walk, and the next cycle of the same fight skips
// the cells the server already refused.
func (l *Loop) kiteWalkClear() {
    l.kiteWalkUntil = time.Time{}
    l.kiteWalkIssuedAt = time.Time{}
    l.kiteReclicks = 0
    l.kiteWalkDead = false
}

// kiteChaserBearings appends the planar bearing (self to chaser,
// the atan2 convention) of every chaser the retreat direction
// weighs to out and returns the filled slice: the fight target
// first, then every SelfAttackers member within the scan range on a
// reachable deck. The enumeration is the single source of the train
// geometry - the centroid sum of kiteTrainDirection and the gap
// search of kiteGapDirection read the same members, so the two
// answers can never disagree about the train's shape. The output
// caps at the capacity of out (the kiteBearings scratch): a wider
// train is the flee machinery's case, not a kite shape.
func (l *Loop) kiteChaserBearings(
    selfX, selfY, selfZ int32, out []float64,
) []float64 {
    // The fight target weighs first: it chases the retreat step
    // wherever it lands, its away vector always counts.
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        dx := float64(selfX) - float64(tx)
        dy := float64(selfY) - float64(ty)
        if math.Hypot(dx, dy) >= 1 {
            out = append(out, math.Atan2(
                float64(ty-selfY), float64(tx-selfX)))
        }
    }
    for _, chaser := range l.tracker.SelfAttackers() {
        if chaser.ObjectID == l.target {
            // The target already weighed in through its own vector.
            continue
        }
        if chaser.Z != 0 &&
            math.Abs(float64(chaser.Z-selfZ)) > deckReachableZ {
            // Another deck: its swings cannot land, its lane crosses
            // the geodata wall.
            continue
        }
        dx := float64(selfX) - float64(chaser.X)
        dy := float64(selfY) - float64(chaser.Y)
        dist := math.Hypot(dx, dy)
        if dist >= kiteTrainScanRange || dist < 1 {
            // Too far to reach the character inside a step window -
            // the flee machinery owns a chase that distant - or
            // standing on the character itself (no away direction).
            continue
        }
        out = append(out, math.Atan2(
            float64(chaser.Y-selfY), float64(chaser.X-selfX)))
        if len(out) == cap(out) {
            // The scratch is full: the gap geometry of a wider
            // train reads its nearest members alone (see
            // kiteMaxTrainMembers).
            break
        }
    }

    return out
}

// kiteGapDirection resolves the escape ray of an encircled train:
// the bisector of the WIDEST angular gap between the chaser
// bearings - the direction that stays farthest from every flanker
// at once. The live fleet round of issue #70 measured what the
// standing encircled answer costs on the mass cells (a 26 second
// stand with the train growing to three chasers, the potions and
// the emergency logout of the wounded slots): the archer that keeps
// stepping through the widest gap breaks the pile-up before it
// forms, the one that stands invites it. The perpendicular break of
// a two-mob line is the classic case - the bisector of the two 180
// degree gaps opens the distance to BOTH chasers at once. Reports
// the unit direction and whether the bearings leave a gap at all (a
// single bearing or none pins no escape geometry).
func kiteGapDirection(bearings []float64) (float64, float64, bool) {
    if len(bearings) < 2 {
        return 0, 0, false
    }
    sorted := make([]float64, len(bearings))
    copy(sorted, bearings)
    sort.Float64s(sorted)
    // The widest gap between consecutive bearings, the wraparound
    // included: the first widest gap wins the ties (a deterministic
    // answer for the symmetric trains - two opposite chasers name
    // the same perpendicular ray every call).
    bestSpan := 0.0
    bestAt := sorted[0]
    for i := 1; i < len(sorted); i++ {
        if span := sorted[i] - sorted[i-1]; span > bestSpan {
            bestSpan, bestAt = span, sorted[i-1]
        }
    }
    if span := sorted[0] + 2*math.Pi - sorted[len(sorted)-1]; span > bestSpan {
        bestSpan, bestAt = span, sorted[len(sorted)-1]
    }
    bisector := bestAt + bestSpan/2

    return math.Cos(bisector), math.Sin(bisector), true
}

// kiteTrainDirection resolves the retreat DIRECTION of one kite
// step: the centroid away-vector of the chaser train - the fight
// target's own away unit vector plus every SelfAttackers member
// within the scan range and on a reachable deck. Each chaser weighs
// one unit - the centroid semantics the issue names; the nearest
// mob drags the direction hardest only through the trigger radius it
// already crossed (the armed threat of kiteThreat). A train whose
// away vectors cancel under the encirclement share names no
// centroid - the WIDEST-GAP bisector takes the step over (the live
// fleet round of issue #70 measured the standing encircled answer
// as a death trap on the mass cells: the train only grows while the
// archer stands - the gap ray keeps the movement contract of the
// kite, the lane battery still owns the terrain, and the hold
// ground answer survives for the gap that is walled or dead). The
// positions are the raw last-known packet ones (the projected scan
// only serves the nearest chaser); the trigger margin absorbs the
// broadcast lag. Reports the unit direction (the centroid ray of a
// normal train, the gap bisector of an encircled one, the zero of
// the degenerate no-chaser scene) and whether the train encircles
// the character.
func (l *Loop) kiteTrainDirection(
    selfX, selfY, selfZ int32,
) (dirX, dirY float64, encircled bool) {
    bearings := l.kiteChaserBearings(
        selfX, selfY, selfZ, l.kiteBearings[:0])
    var sumX, sumY float64
    for _, bearing := range bearings {
        sumX += -math.Cos(bearing)
        sumY += -math.Sin(bearing)
    }
    count := float64(len(bearings))
    sumLen := math.Hypot(sumX, sumY)
    if sumLen < kiteEncircleShare*count || sumLen < 1 {
        // The encircled sum (or the degenerate zero): the centroid
        // names no direction - the widest-gap bisector serves the
        // step when the bearings leave one.
        if gapX, gapY, gap := kiteGapDirection(bearings); gap {
            return gapX, gapY, true
        }

        return 0, 0, true
    }

    return sumX / sumLen, sumY / sumLen, false
}

// kiteRetreatLaneSkipping resolves the retreat lane of one kite step:
// the straight away-ray first, then the fan candidates at kiteFanStep
// increments each side of it - the open backward lanes over the
// blocked corridors, the hemisphere edge as the last resort - minus
// the candidates whose RESOLVED endpoints sit
// in the dead set: the re-click ladder rotates a dead endpoint onto
// the next fan candidate (the destination-cell refusal answer of
// issue #60 - a cell the server refuses never starts a walk, so
// clicking it again changes nothing) and the rotation needs the
// lane battery to answer "which lane comes after these". An empty
// dead set keeps every candidate (the plain resolution of the
// first round). Every candidate runs the full lane gate battery
// (the camp deflection, the leash, the away half-plane, the wall
// and the water). Reports the endpoint and whether a walkable lane
// exists.
func (l *Loop) kiteRetreatLaneSkipping(
    selfX, selfY, selfZ int32, prefX, prefY, awayX, awayY float64,
    dead [][2]int32,
) (int32, int32, bool) {
    // The candidate rays of the away hemisphere around the
    // PREFERRED direction (the straight away-ray of the opening
    // retreat, the curved bearing of the circling ones): the
    // preferred ray first (the lane of record), then the fan
    // candidates widening symmetrically around it - the 45 degree
    // lanes before the 90 degree ones. The half-plane reference
    // stays the RAW away vector whatever the preference leans to.
    var rays [1 + 2*kiteFanSteps][2]float64
    rays[0] = [2]float64{prefX, prefY}
    count := 1
    for step := 1; step <= kiteFanSteps; step++ {
        angle := kiteFanStep * float64(step)
        for _, sign := range [2]float64{1, -1} {
            rays[count][0], rays[count][1] =
                rotatePlanar(prefX, prefY, sign*angle)
            count++
        }
    }
    for _, ray := range rays[:count] {
        endX := selfX + int32(math.Round(ray[0]*l.kite.Step))
        endY := selfY + int32(math.Round(ray[1]*l.kite.Step))
        laneX, laneY, ok := l.kiteLaneResolve(
            selfX, selfY, selfZ, endX, endY, awayX, awayY)
        if !ok {
            continue
        }
        refused := false
        for _, cell := range dead {
            if laneX == cell[0] && laneY == cell[1] {
                refused = true

                break
            }
        }
        if refused {
            // A cell the rotation already named dead (the server
            // refused it or it stayed silent through the probe) -
            // the next candidate serves the retreat instead.
            continue
        }

        return laneX, laneY, true
    }

    return 0, 0, false
}

// kiteLaneResolve resolves one retreat lane candidate to its final
// endpoint and reports whether the lane carries a step: the camp
// deflection first (the tangent endpoint replaces the straight one
// when the lane would wake a neighborhood camp - the deflected
// endpoint runs the remaining gates like any candidate), then the
// leash, the away half-plane, the geodata wall and the water. The
// half-plane gate guards the deflections hardest: a lane may bend
// sideways of the away-ray (the camp side-step exits the trigger
// circle perpendicular), but never fold back toward the chasing
// train. The terrain gates need the navigator - without one (the
// no-geodata runtime, the plain unit scenes) the leash alone fences
// the step, exactly the contract of the first kite round.
func (l *Loop) kiteLaneResolve(
    selfX, selfY, selfZ, endX, endY int32,
    dirX, dirY float64,
) (int32, int32, bool) {
    if defX, defY, dodged := l.kiteDeflectFromCamps(
        selfX, selfY, selfZ, endX, endY); dodged {
        endX, endY = defX, defY
    }
    if zone := l.zone(); zone != nil && !zone.Contains(endX, endY) {
        // The lane leaves the hunting square: the leash outranks the
        // kite - the next fan candidate may still fit inside the
        // ground.
        return 0, 0, false
    }
    laneX, laneY := float64(endX-selfX), float64(endY-selfY)
    laneLen := math.Hypot(laneX, laneY)
    if laneLen < 1 {
        return 0, 0, false
    }
    if laneX/laneLen*dirX+laneY/laneLen*dirY < kiteHalfPlaneSlack {
        // The lane (a deep camp deflection, most likely) folded back
        // toward the train: stepping it would walk INTO the melee the
        // kite exists to avoid.
        return 0, 0, false
    }
    if l.navigator == nil {
        // No geodata runtime: the leash alone fences the step (the
        // contract of the first kite round).
        return endX, endY, true
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ)}
    to := pathfind.Vec3{X: float64(endX), Y: float64(endY),
        Z: float64(selfZ)}
    if sight, err := l.navigator.LineOfSight(from, to); err != nil ||
        !sight {
        // A wall (or an unreadable geodata) behind: the lane is a
        // corridor the walk cannot take.
        return 0, 0, false
    }
    if l.navigator.OverWater(float64(endX), float64(endY),
        int16(selfZ)) {
        // Water behind: swimming trades the bow for the paddle.
        return 0, 0, false
    }

    return endX, endY, true
}

// kiteDeflectFromCamps bends one retreat candidate around the idle
// aggressive camps its line would wake: the first threat circle
// (the effective aggro range plus the steering clearance) the
// straight self-to-end segment enters deflects the endpoint onto the
// tangent ray of that circle, on the side the segment leaned to -
// the same geometry the transit walk steering uses
// (steerClearOfAggro), minus its destination exemption: a retreat
// has no destination-mob contract, a camp standing anywhere on the
// lane (its endpoint included) must not be met. The chasers never
// deflect the lane: they already hold the character as their target
// (the scan skips them by the held target). Reports the deflected
// endpoint and whether a deflection happened.
func (l *Loop) kiteDeflectFromCamps(
    selfX, selfY, selfZ, endX, endY int32,
) (int32, int32, bool) {
    segmentX := float64(endX - selfX)
    segmentY := float64(endY - selfY)
    segmentLen := math.Hypot(segmentX, segmentY)
    if segmentLen < avoidMinSegment {
        return endX, endY, false
    }
    l.avoidScratch = l.tracker.AppendAggroThreats(
        l.avoidScratch[:0], l.target, avoidScanRange)
    if len(l.avoidScratch) == 0 {
        return endX, endY, false
    }
    // The first camp the straight lane would wake: the smallest
    // along-lane parameter wins (the earliest wake), the deepest
    // penetration breaks the tie - the same selection rule as the
    // transit steering.
    first := -1
    firstT := 1.0
    firstPen := 0.0
    for i := range l.avoidScratch {
        threat := &l.avoidScratch[i]
        relX := threat.X - float64(selfX)
        relY := threat.Y - float64(selfY)
        t := (relX*segmentX + relY*segmentY) /
            (segmentLen * segmentLen)
        if t < 0 {
            t = 0
        } else if t > 1 {
            t = 1
        }
        cx := float64(selfX) + segmentX*t
        cy := float64(selfY) + segmentY*t
        // The planar clearance mirrors the server's own 3D trigger:
        // the retreat lane runs on the character's deck, a camp on
        // another deck never blocks it.
        dz := float64(threat.Z) - float64(selfZ)
        clearance := math.Hypot(
            math.Hypot(threat.X-cx, threat.Y-cy), dz)
        needed := threat.AggroRange + avoidClearance
        if pen := needed - clearance; pen > 0 && (t < firstT ||
            (t == firstT && pen > firstPen)) {
            first, firstT, firstPen = i, t, pen
        }
    }
    if first < 0 {
        return endX, endY, false
    }
    threat := &l.avoidScratch[first]
    defX, defY := tangentClearDirection(
        float64(selfX), float64(selfY), threat.X, threat.Y,
        segmentX/segmentLen, segmentY/segmentLen,
        threat.AggroRange+avoidClearance)

    return selfX + int32(math.Round(defX*l.kite.Step)),
        selfY + int32(math.Round(defY*l.kite.Step)), true
}

// kiteHoldGround arms the cornered hold of the kite: the archer
// stops retreating and keeps shooting the bow at melee range. The
// hold paces its own re-probe (the kite period) so a failed lane
// never stutters the tick loop, and logs its reason once per hold
// episode - a standing fight that names itself stays diagnosable in
// the event feed. The surrounded flag picks the diagnostic wording
// (the encircled train whose gap ray found no lane against the
// walled corner).
func (l *Loop) kiteHoldGround(now time.Time, surrounded bool) {
    fresh := l.kiteHeldFor != l.target ||
        l.kiteHeldAt.IsZero() ||
        now.Sub(l.kiteHeldAt) >= kiteHoldLogPeriod
    if fresh {
        if surrounded {
            l.logger.Printf("Hunt: the chasers surround the bow "+
                "fight on target %d, holding ground and shooting "+
                "through the train", l.target)
        } else {
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
