// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The movement phases of the hunt loop, split out of the loop.go
// god file: closing on far packs, the idle patrol toward the zone
// center and the geodata walk home.

import (
	"math"
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
	pick, found := l.tracker.NearestAttackableConstrained(
		farTargetRange, l.zone(), l.activeSkips(now),
		l.maxTargetLevel(), true)
	if !found {
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

// returnToZone walks the character back into the hunting square over the
// geodata waypoints: a village respawn after death or a deleveling guard
// post sits behind the village walls, and a direct walk bumps into them,
// so the return is planned with the pathfinder and followed by the town
// trip waypoint machinery (leg splitting, passed waypoint skipping, stuck
// re-pathing) through phaseTownReturn. The remembered farm spot is the
// destination when one exists, the zone center otherwise. The failures of
// the pathfound legs (a missing geodata region, an unreachable deck) fall
// back to the legacy direct legs, so a character without a walkable path
// still moves home.
func (l *Loop) returnToZone() {
	now := time.Now()
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
	dest := pathfind.Vec3{
		X: float64(zone.CX),
		Y: float64(zone.CY),
		Z: float64(selfZ),
	}
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
