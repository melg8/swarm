// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The blind engage recovery, split out of the loop.go god file: when an
// obstacle (a column, a wall) stands between the character and its
// selected target, the Mobius server answers every swing attempt with
// "Cannot see target." while keeping the attack intention armed - the
// bot would stand at melee distance re-requesting the refused attack
// forever. The recovery walks around the obstacle over the geodata
// first and switches the target only when the walk cannot clear the
// block.

import (
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
)

// Timing and geometry constants of the blind engage recovery.
const (
	// cannotSeeFreshWindow bounds how long a "Cannot see target."
	// answer still counts as the current refusal: the server answers
	// at the attack request cadence (once per second from the loop,
	// once per AI think from the server itself), so a window covering
	// a few missed answers keeps the detection stable.
	cannotSeeFreshWindow = 8 * time.Second
	// blindEngageDelay is the minimum age of an engage attempt before
	// the recovery may take over: the selection and the first forced
	// attack need a second or two, and a healthy engage must not be
	// interrupted mid start.
	blindEngageDelay = 2 * time.Second
	// blindWalkBudget bounds one reposition walk: the detour around a
	// small obstacle is a few hundred units (a couple of seconds at
	// run speed), the budget also covers a longer routing around a
	// wall past the stuck timeout hold.
	blindWalkBudget = 15 * time.Second
	// blindMaxAttempts bounds the reposition attempts per target: the
	// first walk follows the planned geodata route, a second one
	// re-plans from wherever the first ended (a moving target, a walk
	// interrupted by a blow). Past the budget the target is switched.
	blindMaxAttempts = 2
	// blindMeleeRadius is the ring radius around the target where the
	// repositioned character stands: the refused swing proves the
	// melee range held at a similar distance, so the vantage point
	// keeps the fight startable the moment the sight line clears.
	blindMeleeRadius = 90.0
	// blindSkipDelay keeps an unreachable target out of the search:
	// shorter than the flee skip (the mob is not dangerous, only
	// obstructed), longer than the plain stuck skip (the obstacle
	// stays, an immediate re-pick would rebuild the same blind spot).
	blindSkipDelay = 1 * time.Minute
)

// blindEngageBlocked reports whether the current engage attempt is
// being refused because the target is not visible: the server answered
// an attack with "Cannot see target." during this attempt, the answer
// is fresh, and no swing or chase step of this fight landed in between
// (a fresh fight means the sight line cleared on its own - a moving
// character or mob walked past the obstacle edge).
func (l *Loop) blindEngageBlocked(now time.Time) bool {
	if l.target == 0 || l.tracker.SelfFighting(l.target) {
		return false
	}
	if l.engageAt.IsZero() || now.Sub(l.engageAt) < blindEngageDelay {
		return false
	}
	cannotSeeAt := l.tracker.SelfCannotSeeTargetAt()
	if cannotSeeAt.Before(l.engageAt) {
		// The answer belongs to an earlier attempt: the current
		// engage started after the obstruction was last reported.
		return false
	}

	return now.Sub(cannotSeeAt) <= cannotSeeFreshWindow
}

// blindRecoveryArmed reports whether the blind engage recovery owns
// the current engage attempt: a fresh refusal was seen or the
// reposition walk is running. The plain engage stuck timeout stays
// held while it is true - the recovery manages its own budgets.
func (l *Loop) blindRecoveryArmed(now time.Time) bool {
	return !l.losAt.IsZero() || l.blindEngageBlocked(now)
}

// recoverBlindEngage drives the two levels of the recovery: level A
// plans and walks the geodata route to a standing point that sees the
// obstructed target, level B drops the target for another one when no
// route works. The walk stops the attack re-requests entirely: an
// attack request would set the ATTACK intention again and cancel the
// walk the recovery just started.
func (l *Loop) recoverBlindEngage(now time.Time) {
	if l.tracker.SelfFighting(l.target) {
		// The fight is running: the walk cleared the sight line (or
		// the mob walked past the obstacle edge on its own). Stand
		// down and let the engage own the fight again.
		l.clearBlindRecovery()

		return
	}
	if l.losAt.IsZero() {
		l.startBlindReposition(now)

		return
	}
	if now.Sub(l.losAt) > blindWalkBudget {
		// The reposition walk missed its budget: the route was
		// blocked, the walk stalled or the vantage point did not
		// clear the sight line. Switch the target instead of
		// re-arming the same blind engage.
		l.switchBlindTarget(now)

		return
	}
	l.walkBlindWaypoints(now)
}

// startBlindReposition plans the route to a standing point that sees
// the obstructed target (level A). The candidates form a ring around
// the target at the melee radius; the nearest one with a bot side
// geodata sight line to the target wins, and the planned path to it
// becomes the reposition waypoints. Without a navigator, without a
// vantage point or without a path the recovery falls through to the
// target switch (level B).
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
		// No geodata route to the vantage point: the fallback walk
		// would run the character straight into the same obstacle,
		// so the target is switched instead.
		l.switchBlindTarget(now)

		return
	}
	l.losWaypoints = result.Waypoints
	l.losWpIndex = 0
	l.losMoveAt = time.Time{}
	l.logger.Printf("Hunt: target %d is not visible from here, "+
		"walking around the obstacle", l.target)
	// The first leg leaves on the planning tick: the detour around a
	// small obstacle is short, a planning tick of pure standing would
	// double the recovery latency.
	l.walkBlindWaypoints(now)
}

// blindVantagePoint searches the ring of melee range standing points
// around the target for the nearest one with a clear geodata sight
// line to it. The ring is sampled in sixteen directions; the closest
// candidate to the character wins (the shortest detour), a sight line
// error counts as blocked (the recovery must stay conservative: a
// phantom vantage point re-creates the blind engage on arrival).
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

// walkBlindWaypoints follows the planned reposition route with paced
// ground click walks: one leg per walk request period, the long legs
// split at the server move limit like every hunt walk. Reaching the
// final waypoint clears the attempt bookkeeping - the engage takes
// over from the new standing point and re-requests the attack, and a
// persisting block re-arms the recovery with the remaining attempt
// budget.
func (l *Loop) walkBlindWaypoints(now time.Time) {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	for l.losWpIndex < len(l.losWaypoints) {
		wp := l.losWaypoints[l.losWpIndex]
		dist := waypointDistance(wp, selfX, selfY, selfZ)
		if dist > waypointArriveDist {
			if l.losWpIndex+1 < len(l.losWaypoints) {
				next := l.losWaypoints[l.losWpIndex+1]
				nextDist := waypointDistance(next, selfX, selfY, selfZ)
				if nextDist < dist {
					l.losWpIndex++
					l.losMoveAt = time.Time{}

					continue
				}
			}

			break
		}
		l.losWpIndex++
		l.losMoveAt = time.Time{}
	}
	if l.losWpIndex >= len(l.losWaypoints) {
		// Arrived: hand the engage back with a fresh attempt clock,
		// the block detection re-arms the recovery if the sight line
		// is still closed.
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
	moveX, moveY := int32(wp.X), int32(wp.Y)
	dx := wp.X - float64(selfX)
	dy := wp.Y - float64(selfY)
	if leg := math.Hypot(dx, dy); leg > maxMoveLeg {
		frac := maxMoveLeg / leg
		moveX = int32(float64(selfX) + dx*frac)
		moveY = int32(float64(selfY) + dy*frac)
	}
	if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
		l.logger.Printf("Hunt: blind reposition walk failed: %v", err)
	}
}

// switchBlindTarget drops the obstructed target (level B): the mob is
// alive and valid, only unreachable behind the terrain, so it lands on
// the skip list for a minute and the next search picks a different
// one. The selection of the replacement target clears the stale server
// side attack intention of the obstructed one.
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

// clearBlindRecovery resets the reposition bookkeeping.
func (l *Loop) clearBlindRecovery() {
	l.losAt = time.Time{}
	l.losWaypoints = nil
	l.losWpIndex = 0
	l.losTried = 0
	l.losMoveAt = time.Time{}
}
