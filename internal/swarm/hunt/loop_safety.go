// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The combat safety layer of the hunt loop, split out of the
// loop.go god file: the flee decision, the paced escape walks and
// the emergency logout that ends a lost session.

import (
	"fmt"
	"math"
	"time"
)

// fleeFromTarget drops a fight the character is losing and opens
// distance: the target lands on the long skip list (the walk back
// must not re-select it), the pending engage bookkeeping clears and
// the shared threat walk runs.
func (l *Loop) fleeFromTarget(targetID int32, now time.Time) {
	l.logger.Printf("Hunt: HP %.0f%%, fleeing the fight with %d",
		l.tracker.SelfHealthPercent(), targetID)
	l.target = 0
	l.engageAt = time.Time{}
	l.noTargetSince = time.Time{}
	if l.targetSkip == nil {
		l.targetSkip = make(map[int32]time.Time)
	}
	l.targetSkip[targetID] = now.Add(fleeSkipDelay)
	l.fleeFromThreat(now)
}

// fleeFromThreat walks the character away from the nearest living
// threat (the chasing mob of a fled fight, an aggressive pull it
// never selected): one paced leg per call, straight away from the
// threat when the escape point stays inside the zone and toward
// the zone center when it does not (the center direction leashes
// the chasers near their spawns). A sitting character stands up
// first - the server refuses move requests while it sits.
func (l *Loop) fleeFromThreat(now time.Time) {
	if !l.fleeAt.IsZero() && now.Sub(l.fleeAt) < selectPeriod {
		return
	}
	l.fleeAt = now
	if l.fleeSince.IsZero() {
		l.fleeSince = now
	}
	if now.Sub(l.fleeSince) >= fleeLogoutAfter {
		// The escape never shook the chase: the mobs keep the
		// character running forever, the session ends and the
		// login cooldown resets the aggro while the character
		// regenerates sitting.
		l.logger.Printf("Hunt: fleeing for %.0fs without shaking "+
			"the chase, resetting the aggro via logout",
			now.Sub(l.fleeSince).Seconds())
		l.emergencyLogout()

		return
	}
	if !l.standUpGuarded(now) {
		return
	}
	moveX, moveY, moveZ, ok := l.escapeWalkDestination()
	if !ok {
		// The blows landed but no mob stands around anymore:
		// the under attack window closes on its own in seconds.
		return
	}
	if err := l.game.WalkTo(moveX, moveY, moveZ); err != nil {
		l.logger.Printf("Hunt: escape walk failed: %v", err)
	}
}

// escapeWalkDestination plans one escape leg away from the nearest
// threat: straight away from it while the point stays inside the
// zone, toward the zone center when it does not (the center
// direction leashes the chasers near their spawns).
func (l *Loop) escapeWalkDestination() (int32, int32, int32, bool) {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return 0, 0, 0, false
	}
	threatX, threatY, hasThreat := l.threatPosition()
	if !hasThreat {
		return 0, 0, 0, false
	}
	dx := float64(selfX - threatX)
	dy := float64(selfY - threatY)
	dist := math.Hypot(dx, dy)
	if dist <= 1 {
		return 0, 0, 0, false
	}
	moveX := int32(float64(selfX) + dx/dist*escapeWalkDistance)
	moveY := int32(float64(selfY) + dy/dist*escapeWalkDistance)
	zone := l.zone()
	if zone != nil && !zone.Contains(moveX, moveY) {
		// The straight escape leaves the hunting square: run
		// toward the center instead.
		dx = float64(zone.CX - selfX)
		dy = float64(zone.CY - selfY)
		if centerDist := math.Hypot(dx, dy); centerDist > 1 {
			frac := math.Min(1, escapeWalkDistance/centerDist)
			moveX = int32(float64(selfX) + dx*frac)
			moveY = int32(float64(selfY) + dy*frac)
		}
	}

	return moveX, moveY, selfZ, true
}

// threatPosition returns the position of the mob the escape runs
// from: the current target while one is engaged, the nearest
// attacker around otherwise (the mob whose blows land carries the
// character as its target), the nearest living attackable npc as
// the last resort (a hit from a mob that already switched away).
func (l *Loop) threatPosition() (int32, int32, bool) {
	if l.target != 0 {
		if x, y, _, ok := l.tracker.ObjectPosition(l.target); ok {
			return x, y, true
		}
	}
	if pick, ok := l.tracker.NearestAttacker(); ok {
		return pick.X, pick.Y, true
	}
	pick, ok := l.tracker.NearestAttackable(escapeThreatRange, nil)
	if !ok {
		return 0, 0, false
	}

	return pick.X, pick.Y, true
}

// emergencyLogout saves a character with no way out: the health
// is critical, the blows keep landing and the escape could not
// shake the chase. One last escape leg keeps the character moving
// through the combat window the server holds an offline character
// in the world (fifteen seconds), then the session logs out and
// the supervisor reconnects after the armed login cooldown - by
// then the mobs reset and the character regenerates sitting.
func (l *Loop) emergencyLogout() {
	l.logoutDone = true
	reason := fmt.Sprintf("HP %.0f%% under attack",
		l.tracker.SelfHealthPercent())
	if count := l.tracker.SelfAttackerCount(); count >= panicLogoutAttackers {
		reason = fmt.Sprintf("%d mobs piled on us", count)
	}
	l.logger.Printf("Hunt: %s, emergency logout for %s",
		reason, panicLogoutPause)
	if moveX, moveY, moveZ, ok := l.escapeWalkDestination(); ok {
		if err := l.game.WalkTo(moveX, moveY, moveZ); err != nil {
			l.logger.Printf("Hunt: escape walk failed: %v", err)
		}
	}
	l.tracker.SetLoginCooldown(panicLogoutPause)
	if err := l.game.RequestLogout(); err != nil {
		l.logger.Printf("Hunt: logout request failed: %v", err)
	}
}

// targetSkipped reports whether the object id is currently held out of
// the engage target search: a stuck pick keeps its short delay, a
// fled target its long one (both live in the same expiry map).
func (l *Loop) targetSkipped(objectID int32, now time.Time) bool {
	until, ok := l.targetSkip[objectID]

	return ok && now.Before(until)
}

// activeSkips collects the object ids whose skip expiry has not
// passed yet into the reused dense scratch list (see skipScratch).
func (l *Loop) activeSkips(now time.Time) []int32 {
	l.skipScratch = l.skipScratch[:0]
	for objectID, until := range l.targetSkip {
		if now.Before(until) {
			l.skipScratch = append(l.skipScratch, objectID)
		}
	}

	return l.skipScratch
}
