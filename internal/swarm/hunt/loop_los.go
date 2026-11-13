// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The blind engage recovery: when an obstacle stands between the character and
// its target, the Mobius server answers every swing with "Cannot see target."
// while keeping the attack intention armed, so the bot would stand at melee
// distance re-requesting forever. The recovery walks around the obstacle over
// the geodata first and switches the target only when the walk cannot clear
// the block.

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// Timing and geometry constants of the blind engage recovery.
const (
    // cannotSeeFreshWindow bounds how long a "Cannot see target." answer still
    // counts as the current refusal: the server answers at the attack request
    // cadence, so a few missed answers keep the detection stable.
    cannotSeeFreshWindow = 8 * time.Second
    // blindEngageDelay is the minimum age of an engage attempt before the
    // recovery may take over, so a healthy engage is not interrupted mid start.
    blindEngageDelay = 2 * time.Second
    // blindWalkBudget bounds one reposition walk: the detour around a small
    // obstacle is a few hundred units, the budget also covers a longer routing
    // around a wall.
    blindWalkBudget = 15 * time.Second
    // blindMaxAttempts bounds the reposition attempts per target: the first
    // walk follows the planned route, a second one re-plans from wherever it
    // ended (a moving target, an interrupted walk); past the budget the target
    // is switched.
    blindMaxAttempts = 2
    // blindMeleeRadius is the ring radius around the target where the
    // repositioned character stands: the refused swing proves the melee range
    // held, so the fight starts the moment the sight line clears.
    blindMeleeRadius = 90.0
    // blindSkipDelay keeps an unreachable target out of the search: shorter
    // than the flee skip (the mob is only obstructed), longer than the plain
    // stuck skip (an immediate re-pick would rebuild the same blind spot).
    blindSkipDelay = 1 * time.Minute
)

// blindEngageBlocked reports whether the current engage attempt is refused
// because the target is not visible: the server answered an attack with "Cannot
// see target." during this attempt, the answer is fresh, and nothing of this
// fight landed in between - no swing and no chase step newer than the refusal.
// A fresh activity view alone does not clear the block: the server of a refused
// attack keeps broadcasting the chase steps of its own re-try, so the phantom
// chase reads as a live fight.
func (l *Loop) blindEngageBlocked(now time.Time) bool {
    if l.target == 0 {
        return false
    }
    if l.engageAt.IsZero() || now.Sub(l.engageAt) < blindEngageDelay {
        return false
    }
    cannotSeeAt := l.tracker.SelfCannotSeeTargetAt()
    if !cannotSeeAt.After(l.engageAt) {
        // The answer belongs to an earlier attempt: the current engage started
        // at or after the obstruction was last reported. The equality counts as
        // earlier, because the walk completion re-anchors the clock after the
        // refusal and the Windows clock granularity can stamp both inside one
        // millisecond.
        return false
    }
    if now.Sub(cannotSeeAt) > cannotSeeFreshWindow {
        return false
    }

    return !l.tracker.SelfCombatActiveAt().After(cannotSeeAt)
}

// blindRecoveryArmed reports whether the blind engage recovery owns the current
// attempt: a fresh refusal was seen or the reposition walk is running. The
// plain engage stuck timeout stays held while it is true.
func (l *Loop) blindRecoveryArmed(now time.Time) bool {
    return !l.losAt.IsZero() || l.blindEngageBlocked(now)
}

// recoverBlindEngage drives the two levels of the recovery: level A plans and
// walks the geodata route to a standing point that sees the obstructed target,
// level B drops the target for another one when no route works. The walk stops
// the attack re-requests, which would set the ATTACK intention again and cancel
// the walk.
func (l *Loop) recoverBlindEngage(now time.Time) {
    if l.fightClearedRefusal() {
        // The fight progressed past the refusal: a swing or a chase step landed
        // after the last "cannot see" (the walk cleared the sight line, or the
        // mob walked past the obstacle edge). The phantom chase alone is not
        // enough, so the recovery stands down only on newer activity.
        l.clearBlindRecovery()

        return
    }
    if l.losAt.IsZero() {
        l.startBlindReposition(now)

        return
    }
    if now.Sub(l.losAt) > blindWalkBudget {
        // The reposition walk missed its budget: the route was blocked, the
        // walk stalled or the vantage point did not clear the sight line.
        // Switch the target instead of re-arming the same blind engage.
        l.switchBlindTarget(now)

        return
    }
    l.walkBlindWaypoints(now)
}

// fightClearedRefusal reports whether the running fight progressed past the
// last "Cannot see target." refusal: the engagement is fresh and its last
// activity landed after the refusal. The refusal proves the sight line was
// closed, so only newer activity proves it opened again.
func (l *Loop) fightClearedRefusal() bool {
    if !l.tracker.SelfFighting(l.target) {
        return false
    }

    // The equality counts as progressed: the Windows timer granularity cannot
    // order a chase step and the refusal of the same tick, and the next refusal
    // re-arms.
    return !l.tracker.SelfCombatActiveAt().Before(
        l.tracker.SelfCannotSeeTargetAt())
}

// startBlindReposition plans the route to a standing point that sees the
// obstructed target (level A). The candidates form a ring around the target at
// the melee radius; the nearest one with a bot side geodata sight line wins.
// Without a navigator, a vantage point or a path the recovery falls through to
// the target switch (level B).
func (l *Loop) startBlindReposition(now time.Time) {
    l.losAt = now
    if l.navigator == nil || l.losTried >= blindMaxAttempts {
        l.switchBlindTarget(now)

        return
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        l.switchBlindTarget(now)

        return
    }
    targetX, targetY, targetZ, ok := l.tracker.ObjectPosition(l.target)
    if !ok {
        l.switchBlindTarget(now)

        return
    }
    l.losTried++
    goal, ok := l.blindVantagePoint(
        float64(selfX), float64(selfY),
        float64(targetX), float64(targetY), float64(targetZ),
    )
    if !ok {
        l.switchBlindTarget(now)

        return
    }
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    result, err := l.navigator.FindPathApproach(from, goal, waypointArriveDist)
    if err != nil || result == nil || !result.Found ||
        len(result.Waypoints) == 0 {
        // No geodata route to the vantage point: the fallback walk would run
        // into the same obstacle, so the target is switched instead.
        l.switchBlindTarget(now)

        return
    }
    l.losWaypoints = result.Waypoints
    l.losWpIndex = 0
    l.losMoveAt = time.Time{}
    l.logger.Printf("Hunt: target %d is not visible from here, "+
        "walking around the obstacle", l.target)
    // The first segment leaves on the planning tick: a detour around a small
    // obstacle is short, and a planning tick of pure standing would double the
    // latency.
    l.walkBlindWaypoints(now)
}

// blindVantagePoint searches the ring of melee range standing points around the
// target for the nearest one with a clear geodata sight line. The ring is
// sampled in sixteen directions, the closest candidate wins, and a sight line
// error counts as blocked (a phantom vantage point re-creates the blind engage
// on arrival).
func (l *Loop) blindVantagePoint(
    selfX, selfY, targetX, targetY, targetZ float64,
) (pathfind.Vec3, bool) {
    best := pathfind.Vec3{X: 0, Y: 0, Z: 0}
    found := false
    bestDist := math.MaxFloat64
    for i := range 16 {
        angle := float64(i) * 2 * math.Pi / 16
        x := targetX + blindMeleeRadius*math.Cos(angle)
        y := targetY + blindMeleeRadius*math.Sin(angle)
        z := targetZ
        if height, err := l.navigator.ClosestHeight(
            x, y, int16(targetZ)); err == nil {
            z = float64(height)
        }
        point := pathfind.Vec3{X: x, Y: y, Z: z}
        target := pathfind.Vec3{
            X: targetX,
            Y: targetY,
            Z: targetZ,
        }
        visible, err := l.navigator.LineOfSight(point, target)
        if err != nil || !visible {
            continue
        }
        dist := math.Hypot(x-selfX, y-selfY)
        if dist < bestDist {
            bestDist = dist
            best = point
            found = true
        }
    }

    return best, found
}

// walkBlindWaypoints follows the planned reposition route with paced ground
// click walks: one segment per walk request period, the long ones split at the
// server move limit. A waypoint counts as reached within its arrival radius and
// only one passed ON THE ROUTE skips ahead (the projection test of the town
// walker). Reaching the final waypoint clears the attempt bookkeeping, so the
// engage takes over from the new standing point while a persisting block
// re-arms the recovery.
func (l *Loop) walkBlindWaypoints(now time.Time) {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    for l.losWpIndex < len(l.losWaypoints) {
        if waypointArrived(l.losWaypoints, l.losWpIndex,
            selfX, selfY, selfZ, 0, waypointArriveDist) {
            l.losWpIndex++
            l.losMoveAt = time.Time{}

            continue
        }
        // Not reached: skip it only when the character already
        // passed it on the route towards the next waypoint.
        if l.losWpIndex+1 < len(l.losWaypoints) &&
            waypointPassed(l.losWaypoints[l.losWpIndex],
                l.losWaypoints[l.losWpIndex+1], selfX, selfY) {
            l.losWpIndex++
            l.losMoveAt = time.Time{}

            continue
        }

        break
    }
    if l.losWpIndex >= len(l.losWaypoints) {
        // Arrived: hand the engage back with a fresh attempt clock; the block
        // detection re-arms the recovery if the sight line is still closed.
        l.losAt = time.Time{}
        l.losWaypoints = nil
        l.engageAt = now

        return
    }
    if !l.losMoveAt.IsZero() && now.Sub(l.losMoveAt) < walkRequestPeriod {
        return
    }
    l.losMoveAt = now
    wp := l.losWaypoints[l.losWpIndex]
    moveX, moveY, _ := walkClickSegment(selfX, selfY, selfZ, wp.X, wp.Y,
        float64(selfZ), maxMoveDistance)
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logger.Printf("Hunt: blind reposition walk failed: %v", err)
    }
}

// switchBlindTarget drops the obstructed target (level B): the mob is alive and
// valid, only unreachable behind the terrain, so it lands on the skip list for
// a minute and the next search picks another one.
func (l *Loop) switchBlindTarget(now time.Time) {
    l.logger.Printf("Hunt: target %d stays invisible, switching to "+
        "another", l.target)
    if l.targetSkip == nil {
        l.targetSkip = make(map[int32]time.Time)
    }
    l.targetSkip[l.target] = now.Add(blindSkipDelay)
    l.target = 0
    l.engageAt = time.Time{}
    l.clearBlindRecovery()
    l.noTargetSince = time.Time{}
}

func (l *Loop) clearBlindRecovery() {
    l.losAt = time.Time{}
    l.losWaypoints = nil
    l.losWpIndex = 0
    l.losTried = 0
    l.losMoveAt = time.Time{}
}
