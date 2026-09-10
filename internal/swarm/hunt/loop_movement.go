// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The movement phases of the hunt loop, split out of the loop.go
// god file: closing on far packs, the idle patrol toward the zone
// center, the geodata walk home and the targetless diagnostic that
// explains a standing hunter in the log.

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
)

// walkToFarTarget walks a targetless hunter toward the nearest
// valid mob of the zone when the pack sits outside the engage
// radius: a big square holds its mobs far from wherever the
// character stands, and standing still until luck walks a mob
// into the radius stalls the hunt (the zone rotation only fires
// on a fully empty square, a far pack keeps it armed). One paced
// leg at a time - the per second target search of the engage
// picks up any mob the leg comes past, so the character engages
// the moment something valid enters the radius. Reports whether
// the tick was handled (a far target exists); without one the
// caller falls back to the center patrol.
func (l *Loop) walkToFarTarget(now time.Time) bool {
	if l.noTargetSince.IsZero() {
		l.noTargetSince = now

		return false
	}
	if now.Sub(l.noTargetSince) < noTargetPatience {
		return false
	}
	if now.Sub(l.lastHit) < selectPeriod {
		return true
	}
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return false
	}
	pick, found := l.tracker.NearestAttackablePreferred(
		farTargetRange, l.zone(), l.activeSkips(now),
		l.maxTargetLevel(), true, l.zoneMobPriority)
	if !found {
		// The far search scans the whole square: nothing in the zone
		// is pickable at any distance. Explain the standing hunter in
		// the log - the mobs the character sees, their positions and
		// why the target search rejects them - instead of letting a
		// fenced social pack look like a broken bot.
		l.logNoPickableTargets(now)

		return false
	}
	dist := math.Hypot(float64(pick.X-selfX), float64(pick.Y-selfY))
	if dist <= attackNearestRange {
		// Inside the engage radius already: the per second pick takes
		// it from here.
		return false
	}
	l.lastHit = now
	moveX, moveY := pick.X, pick.Y
	dx := float64(pick.X - selfX)
	dy := float64(pick.Y - selfY)
	if dist > returnWalkLeg {
		frac := returnWalkLeg / dist
		moveX = int32(float64(selfX) + dx*frac)
		moveY = int32(float64(selfY) + dy*frac)
	}
	if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
		l.logger.Printf("Hunt: far target walk failed: %v", err)
	}

	return true
}

// logNoPickableTargets logs the targetless diagnostic: the nearest
// mobs the character sees around itself, their positions and the
// reasons the engage target search rejects them (a social clan pack,
// the level ceiling, the zone square, the skip list). The caller
// reaches here only after the far search confirmed that the whole
// zone holds nothing pickable, so the line answers the "why is the
// bot standing there" question directly in the log. The pacing
// keeps it to one line per noPickLogPeriod while the state lasts; a
// successful pick or a flee re-arms it.
func (l *Loop) logNoPickableTargets(now time.Time) {
	if !l.noPickLogAt.IsZero() && now.Sub(l.noPickLogAt) < noPickLogPeriod {
		return
	}
	l.noPickLogAt = now
	blocked := l.tracker.NearestBlockedTargets(
		l.zone(), l.maxTargetLevel(), l.activeSkips(now), noPickLogLimit)
	if len(blocked) == 0 {
		l.logger.Printf("Hunt: no pickable target in the zone, " +
			"no attackable npc in sight")

		return
	}
	selfX, selfY, _, selfOK := l.tracker.SelfPosition()
	var line strings.Builder
	line.WriteString("Hunt: no pickable target in the zone:")
	for i := range blocked {
		entry := &blocked[i]
		fmt.Fprintf(&line, " %s (%d) at %d %d %d",
			entry.Name, entry.ObjectID, entry.X, entry.Y, entry.Z)
		if selfOK {
			dist := math.Hypot(
				float64(entry.X-selfX), float64(entry.Y-selfY))
			fmt.Fprintf(&line, ", %.0f units", dist)
		}
		fmt.Fprintf(&line, " - %s;", entry.Reason)
	}
	l.logger.Printf("%s", strings.TrimSuffix(line.String(), ";"))
}

// patrolToCenter walks a targetless hunter toward the zone center:
// the pack moved on or the social fence keeps the camps out of
// reach, and standing still waits for luck. One paced leg at a
// time, so the per second target search of the engage picks up
// any mob the leg comes past - the character engages the moment
// something valid enters the radius instead of marching to the
// center first.
func (l *Loop) patrolToCenter(now time.Time) {
	zone := l.zone()
	if zone == nil {
		return
	}
	if l.noTargetSince.IsZero() {
		l.noTargetSince = now

		return
	}
	if now.Sub(l.noTargetSince) < noTargetPatience {
		return
	}
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	dist := math.Hypot(float64(zone.CX-selfX), float64(zone.CY-selfY))
	if dist < patrolCenterMinDist {
		return
	}
	l.lastHit = now
	l.walkZoneLeg(zone, selfX, selfY, selfZ)
}

// adoptOutZoneFight keeps the fight that crossed the hunting zone
// line running: the mob that dragged the character out of the square
// (a chase, an aggressive pull) is finished where it stands before
// the walk home. The adoption covers the own living target, the
// fresh server side selection and the mob that currently holds the
// character as its target; a target the flee flow held out (the skip
// list) stays out - the escape decision of the flee keeps its
// authority, the walk home is safer than a fight the character just
// ran from. Reports whether a fight is live (the caller continues
// the engage logic outside the zone); without one the leash walks
// the character home.
func (l *Loop) adoptOutZoneFight(now time.Time) bool {
	if l.target != 0 && l.tracker.ObjectAlive(l.target) {
		return true
	}
	serverTarget := l.tracker.SelfTargetID()
	if serverTarget != 0 && l.tracker.ObjectAlive(serverTarget) &&
		!l.targetSkipped(serverTarget, now) {
		l.target = serverTarget
		l.engageAt = now
		l.clearBlindRecovery()

		return true
	}
	if l.tracker.SelfUnderAttack() {
		if pick, ok := l.tracker.NearestAttacker(); ok &&
			!l.targetSkipped(pick.ObjectID, now) {
			l.logger.Printf("Hunt: %s (%d) keeps attacking outside "+
				"the zone, finishing it", pick.Name, pick.ObjectID)
			l.target = pick.ObjectID
			l.engageAt = now
			l.clearBlindRecovery()

			return true
		}
	}

	return false
}

// returnToZone walks the character back into the hunting square over the
// geodata waypoints: a village respawn after death or a deleveling guard
// post sits behind the village walls, and a direct walk bumps into them,
// so the return is planned with the pathfinder and followed by the town
// trip waypoint machinery (leg splitting, passed waypoint skipping, stuck
// re-pathing) through phaseTownReturn. The remembered farm spot is the
// destination when one exists, the zone center otherwise. The failures of
// the pathfound legs (a missing geodata region, an unreachable deck) fall
// back to the legacy direct legs, so a character without a walkable path
// still moves home. The return needs a standing character: the server
// refuses every move request while it sits (the AI stays on the REST
// intention and answers ActionFailed), so a zone switch that lands on a
// resting character first waits out the sit transition, stands up and
// only then plans the walk.
func (l *Loop) returnToZone() {
	now := time.Now()
	if !l.standUpGuarded(now) {
		return
	}
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	zone := l.zone()
	if zone == nil {
		return
	}
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	l.target = 0
	l.clearBlindRecovery()
	l.lootID = 0
	if l.zoneReturn && l.phase == phaseEngage {
		// The previous pathfound return leg ended without reaching
		// the zone (a stuck walk aborts the leg): count the failure
		// and stop planning past the budget.
		l.zoneFails++
	}
	if !l.zoneReturn {
		l.zoneReturn = true
		l.logger.Printf("Hunt: outside the hunting zone, pathfinding back")
	}
	if (l.navigator == nil) || l.zoneFails >= zoneReturnFailBudget {
		l.phase = phaseEngage
		l.walkZoneLeg(zone, selfX, selfY, selfZ)

		return
	}
	dest := l.zoneReturnDestination(zone, selfZ)
	farmKnown := l.farmX != 0 || l.farmY != 0
	if farmKnown && zone.Contains(l.farmX, l.farmY) {
		dest = pathfind.Vec3{
			X: float64(l.farmX),
			Y: float64(l.farmY),
			Z: float64(l.farmZ),
		}
	}
	l.tripStart = time.Now()
	l.rePaths = 0
	l.phase = phaseTownReturn
	if !l.startWalkLeg(dest) {
		// No geodata path: direct legs toward the zone, the server
		// stops them at obstacles and the next second plans again.
		l.phase = phaseEngage
		l.walkZoneLeg(zone, selfX, selfY, selfZ)

		return
	}
}

// zoneReturnDestination builds the search goal of the zone return for
// the square center: the x and y of the center with the REAL deck
// height under it, resolved the way the server itself resolves a
// destination - the layer closest to the walker height. A fabricated
// self height at the target (the old code) put the 3D approach goal
// mid air whenever the zone sits on another deck than the character
// (the live case: the floating elven city deck at z -2992 against a
// ground zone at z -3664): no cell ever matched the approach radius,
// the search burned the whole expansion cap (a thirteen second
// frozen tick per attempt) and the return fell back to the direct
// walk that ran the character into the city railing. A height lookup
// failure keeps the self height - the same-deck case it answers
// correctly.
func (l *Loop) zoneReturnDestination(
	zone *state.Zone, selfZ int32,
) pathfind.Vec3 {
	dest := pathfind.Vec3{
		X: float64(zone.CX),
		Y: float64(zone.CY),
		Z: float64(selfZ),
	}
	height, err := l.navigator.ClosestHeight(
		float64(zone.CX), float64(zone.CY), int16(selfZ))
	if err != nil {
		l.logger.Printf("Hunt: zone deck height lookup failed: %v", err)

		return dest
	}
	dest.Z = float64(height)

	return dest
}

// walkZoneLeg walks one direct short leg toward the zone center: the
// emergency fallback of the pathfinding zone return. The leg length
// respects the server move request limit (9900 units) and the walk rate
// limits itself through the select pacing of the return.
func (l *Loop) walkZoneLeg(
	zone *state.Zone, selfX int32, selfY int32, selfZ int32,
) {
	moveX, moveY := zone.CX, zone.CY
	dx := float64(zone.CX - selfX)
	dy := float64(zone.CY - selfY)
	if dist := math.Hypot(dx, dy); dist > returnWalkLeg {
		frac := returnWalkLeg / dist
		moveX = int32(float64(selfX) + dx*frac)
		moveY = int32(float64(selfY) + dy*frac)
	}
	if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
		l.logger.Printf("Hunt: walk back failed: %v", err)
	}
}
