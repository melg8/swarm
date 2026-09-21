// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kiting layer of the archer fight (issue #13): a bow user that
// fights a mob from the weapon range never wants the mob in its face.
// The Mobius server AI stands the archer still while the auto attack
// shoots, and a melee mob that closes the distance simply swings away
// - the standing archer trades blow for blow with a mob it could
// outrange. The kite step answers exactly that: while the fight runs,
// a target that closed inside the retreat radius (the mob is a
// second or two from melee) makes the character step straight away
// from it, the movement window pauses the forced attack re-requests
// (a forced attack request interrupts a running walk server-side,
// the same rule the impending-add step of loop_avoid.go follows),
// and once the step finished the engage re-requests the attack - the
// bow shoots again from the opened distance. The cycle - stand,
// shoot, step clear, shoot - is the kiting behavior of the issue:
// the damage slows by the step overhead, the melee uptime drops to
// zero on a ground the mob cannot cross faster than the character.
//
// The honest limits of the first round, by the edge cases the issue
// names:
//   - cornered: a step that would leave the hunting square is
//     skipped - the leash outranks the kite, the archer stands and
//     shoots (the panic machinery of the loop still owns a fight that
//     turns unwinnable);
//   - multiple mobs chasing: the step direction reads the current
//     target alone; the pile up answer (the attacker count gate, the
//     panic run) already owns the too-many-attackers case;
//   - pathfinding while retreating: the step goes through WalkTo,
//     so the walk follower drives it over the mesh routes - the
//     retreat never runs into a wall the geodata knows about;
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
)

// The kiting constants of the archer fight.
const (
    // kiteRetreatRadius is the trigger distance of the kite step: a
    // bow target closer than this to the fighting character is about
    // to reach melee (the swing range sits around 150 units and the
    // chasers run 120+ units per second), so the step fires before
    // the blows land. Above it the archer stands and shoots - the
    // standing fight inside the weapon range is the good case.
    kiteRetreatRadius = 250.0
    // kiteStep is the retreat length away from the closed target:
    // sized so the distance after the step (the mob follows at its
    // run speed through the ~2s window) lands back inside the bow
    // engage radius (450) - the forced attack re-request then shoots
    // from range instead of starting a server side chase that would
    // walk the distance right back in.
    kiteStep = 400.0
    // kiteStepWindow is the movement window the step owns: the walk
    // runs ~400 units (about two seconds at the run speed), and a
    // forced attack request would interrupt it server-side, so the
    // engage holds its re-requests until the window closes - the
    // same contract the impending-add step uses
    // (combatAvoidStepPeriod).
    kiteStepWindow = 2 * time.Second
    // kiteStepPeriod paces the steps: the kite cycle spends the
    // window walking and the rest standing and shooting; the period
    // bounds the walking share from above (2s walk, 1s shoot at the
    // fastest).
    kiteStepPeriod = 3 * time.Second
    // kiteStreakLimit bounds the consecutive kite steps of one
    // target: a chaser at least as fast as the character never falls
    // behind (the step opens no net distance), and an endless kite
    // shuffle would starve the fight of every swing. Past the limit
    // the archer stops kiting THIS target and fights it out - the
    // losing fight machinery (losingFight, the panic run) still owns
    // the death risk.
    kiteStreakLimit = 8
)

// kiteFromTarget steps a bow fighting character away from its own
// target when the mob closed inside the retreat radius: the archer
// buys back the weapon range instead of tanking the melee. The step
// respects the zone leash (a cornered archer stands and shoots), the
// movement window contract of the fight steps (the forced attack
// re-requests wait out the walk), and the streak limit (an
// unwinnable distance race falls back to the ordinary fight). A
// fresh target resets the streak. Reports whether the tick issued
// the step (the caller skips the rest of the fighting branch then -
// the walk owns the movement).
func (l *Loop) kiteFromTarget(now time.Time) bool {
    if !l.bowEquipped() || l.target == 0 {
        return false
    }
    // A step window that is still running (this step or an
    // impending-add step) owns the movement: no second walk may
    // interrupt it.
    if now.Before(l.combatAvoidUntil) {
        return false
    }
    if !l.kiteAt.IsZero() && now.Sub(l.kiteAt) < kiteStepPeriod {
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
    x, y, _, ok := l.tracker.ObjectPosition(l.target)
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !ok || !selfOK {
        return false
    }
    dx := float64(selfX) - float64(x)
    dy := float64(selfY) - float64(y)
    dist := math.Hypot(dx, dy)
    if dist >= kiteRetreatRadius || dist < 1 {
        return false
    }
    stepX := int32(float64(selfX) + dx/dist*kiteStep)
    stepY := int32(float64(selfY) + dy/dist*kiteStep)
    if zone := l.zone(); zone != nil && !zone.Contains(stepX, stepY) {
        // Cornered against the leash: the ground outranks the kite,
        // the archer stands and shoots (the losing fight and the
        // panic machinery own the risk from here).
        return false
    }
    l.kiteAt = now
    l.kiteStreak++
    // The shared movement window of the fighting steps: the engage
    // holds its forced attack re-requests until the walk finished
    // (see the combatAvoidUntil gate of the engage branch).
    l.combatAvoidUntil = now.Add(kiteStepWindow)
    l.logger.Printf("Hunt: target %d closed to %d units, "+
        "kiting clear (step %d of %d)",
        l.target, int(math.Round(dist)), l.kiteStreak, kiteStreakLimit)
    if err := l.game.WalkTo(stepX, stepY, selfZ); err != nil {
        l.logger.Printf("Hunt: kite walk failed: %v", err)
    }

    return true
}
