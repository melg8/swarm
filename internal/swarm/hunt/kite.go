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
// The behavior arms itself on the weapon in hand (bowEquipped): no
// config, no class check - whatever bot ends up holding a bow (the
// planned archer type of the config launch, a lured guard round)
// kites its fights.

import (
    "math"
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
    // on every side), the train is too wide to outrun and the archer
    // holds ground and shoots through it. A single chaser always sums
    // to a full unit (share 1.0); two chasers on opposite sides sum
    // to ~0.
    kiteEncircleShare = 0.3
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
    // kiteHoldLogPeriod paces the cornered hold diagnostic: the hold
    // itself re-probes at the kite pacing, the log line lands once
    // per period - a cornered fight is a standing fight, the line is
    // its visibility, not a complaint.
    kiteHoldLogPeriod = 15 * time.Second
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
// Returns the threat position, its identity for the logs and its
// ground distance to the character.
func (l *Loop) kiteThreat(
    selfX, selfY, selfZ int32,
) (x, y float64, id int32, dist float64, ok bool) {
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        dx := float64(selfX) - float64(tx)
        dy := float64(selfY) - float64(ty)
        x, y, id, dist, ok =
            float64(tx), float64(ty), l.target, math.Hypot(dx, dy), true
    }
    if attacker, aok := l.tracker.NearestAttacker(); aok &&
        attacker.ObjectID != l.target {
        if attacker.Z == 0 ||
            math.Abs(float64(attacker.Z-selfZ)) <= deckReachableZ {
            dx := float64(selfX) - float64(attacker.X)
            dy := float64(selfY) - float64(attacker.Y)
            adist := math.Hypot(dx, dy)
            if !ok || adist < dist {
                x, y, id, dist, ok =
                    float64(attacker.X), float64(attacker.Y),
                    attacker.ObjectID, adist, true
            }
        }
    }

    return x, y, id, dist, ok
}

// kiteStepAdmitted guards the kite step of one tick: the bow and the
// fight target must be there, the movement window must be free (a
// running step or an impending-add walk owns it), the kite pacing
// must have aged out (the step and the cornered hold re-probe
// alike), the streak limit must not be spent, and a fresh target
// resets the streak of the previous one (the distance race of one
// mob is not the race of the next). Reports whether the step may run
// now.
func (l *Loop) kiteStepAdmitted(now time.Time) bool {
    // The params gate comes first: a kite-disabled profile (the
    // standing-archer baseline of issue #29) never arms the layer,
    // whatever the bow and the fight report.
    if !l.kite.Enabled || !l.bowEquipped() || l.target == 0 {
        return false
    }
    // A step window that is still running (this step or an
    // impending-add step) owns the movement: no second walk may
    // interrupt it.
    if now.Before(l.combatAvoidUntil) {
        return false
    }
    if !l.kiteAt.IsZero() && now.Sub(l.kiteAt) < l.kite.StepPeriod {
        return false
    }
    // The cornered hold owns its own pacing: a retreat with no
    // walkable lane re-probes at the kite period, not every tick -
    // the failed probe attempt itself must never become the idle
    // stutter the issue forbids. A fresh target re-probes at once.
    if !l.kiteHeldAt.IsZero() && l.kiteHeldFor == l.target &&
        now.Sub(l.kiteHeldAt) < l.kite.StepPeriod {
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
// fresh target resets the streak. Reports whether the tick issued
// the step (the caller skips the rest of the fighting branch then -
// the walk owns the movement).
func (l *Loop) kiteFromTarget(now time.Time) bool {
    if !l.kiteStepAdmitted(now) {
        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    _, _, threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= l.kite.RetreatRadius || dist < 1 {
        return false
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled {
        // The train surrounds the character - the away vectors
        // cancel, every step direction walks INTO a chaser. Too wide
        // to outrun: hold ground and shoot through the train (the
        // losing fight and the pile up machinery still own a train
        // that outdamages the standing fight).
        l.kiteHoldGround(now, true)

        return false
    }
    stepX, stepY, found := l.kiteRetreatLane(
        selfX, selfY, selfZ, dirX, dirY)
    if !found {
        // Cornered: no walkable lane in the whole away hemisphere.
        // The hold ground answer is the archetype rule - the bow is
        // the always-weapon, the cornered archer never switches to a
        // melee trade, it stands and shoots the way out.
        l.kiteHoldGround(now, false)

        return false
    }
    l.kiteHeldAt = time.Time{}
    l.kiteAt = now
    l.kiteStreak++
    // The shared movement window of the fighting steps: the engage
    // holds its forced attack re-requests until the walk finished
    // (see the combatAvoidUntil gate of the engage branch). The
    // re-engage delay rides the same timestamp - the knob of the
    // tuning round.
    l.combatAvoidUntil = now.Add(kiteStepWindow + l.kite.ReengageDelay)
    l.logger.Printf("Hunt: hostile %d closed to %d units of the "+
        "fight on %d, kiting clear (step %d of %d)",
        threatID, int(math.Round(dist)), l.target,
        l.kiteStreak, kiteStreakLimit)
    if err := l.game.WalkTo(stepX, stepY, selfZ); err != nil {
        l.logger.Printf("Hunt: kite walk failed: %v", err)
    }

    return true
}

// kiteTrainDirection resolves the retreat DIRECTION of one kite
// step: the centroid away-vector of the chaser train - the fight
// target's own away unit vector plus every SelfAttackers member
// within the scan range and on a reachable deck. Each chaser weighs
// one unit - the centroid semantics the issue names; the nearest
// mob drags the direction hardest only through the trigger radius it
// already crossed (the armed threat of kiteThreat). A train whose
// away vectors cancel under the encirclement share has no direction
// at all - the caller holds ground and shoots through it. The
// positions are the raw last-known packet ones (the projected scan
// only serves the nearest chaser); the trigger margin absorbs the
// broadcast lag. Reports the unit direction and whether the train
// encircles the character.
func (l *Loop) kiteTrainDirection(
    selfX, selfY, selfZ int32,
) (dirX, dirY float64, encircled bool) {
    var sumX, sumY float64
    count := 0.0
    // The fight target weighs first: it chases the retreat step
    // wherever it lands, its away vector always counts.
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        dx := float64(selfX) - float64(tx)
        dy := float64(selfY) - float64(ty)
        if dist := math.Hypot(dx, dy); dist >= 1 {
            sumX += dx / dist
            sumY += dy / dist
            count++
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
        sumX += dx / dist
        sumY += dy / dist
        count++
    }
    sumLen := math.Hypot(sumX, sumY)
    if sumLen < kiteEncircleShare*count || sumLen < 1 {
        // The encircled sum (or the degenerate zero): no retreat
        // direction exists.
        return 0, 0, true
    }

    return sumX / sumLen, sumY / sumLen, false
}

// kiteRetreatLane resolves the retreat endpoint of one kite step:
// the straight away-ray first, then the fan candidates at kiteFanStep
// increments each side of it - the open backward lanes over the
// blocked corridors, the hemisphere edge as the last resort. Every
// candidate runs the full lane gate battery (the camp deflection,
// the leash, the away half-plane, the wall and the water). Reports
// the endpoint and whether a walkable lane exists.
func (l *Loop) kiteRetreatLane(
    selfX, selfY, selfZ int32, dirX, dirY float64,
) (int32, int32, bool) {
    // The straight away-ray is the lane of record.
    endX := selfX + int32(math.Round(dirX*l.kite.Step))
    endY := selfY + int32(math.Round(dirY*l.kite.Step))
    if laneX, laneY, ok := l.kiteLaneResolve(
        selfX, selfY, selfZ, endX, endY, dirX, dirY); ok {
        return laneX, laneY, true
    }
    // Then the fan candidates, widening symmetrically around it -
    // the 45 degree lanes before the 90 degree ones.
    for step := 1; step <= kiteFanSteps; step++ {
        angle := kiteFanStep * float64(step)
        for _, sign := range [2]float64{1, -1} {
            candX, candY := rotatePlanar(dirX, dirY, sign*angle)
            endX = selfX + int32(math.Round(candX*l.kite.Step))
            endY = selfY + int32(math.Round(candY*l.kite.Step))
            if laneX, laneY, ok := l.kiteLaneResolve(
                selfX, selfY, selfZ, endX, endY, dirX, dirY); ok {
                return laneX, laneY, true
            }
        }
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
// (the encircled train against the walled corner).
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
