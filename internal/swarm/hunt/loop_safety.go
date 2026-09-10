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
	l.clearBlindRecovery()
	l.noTargetSince = time.Time{}
	l.noPickLogAt = time.Time{}
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
// from: the living target while one is engaged, the nearest attacker
// around otherwise (the mob whose blows land or whose chase steps
// toward the character carry it as the target). A mob that merely
// stands nearby is not a threat: the old last resort to the nearest
// living attackable npc armed the escape against a passive bystander
// the moment a finished fight left its fresh SelfUnderAttack window -
// the hurt character ran several hundred units away from the kill
// spot (sometimes out of the zone) before it sat down to rest. With
// no mob holding the character as its target the escape has nothing
// to run from and the rest happens where the fight ended.
func (l *Loop) threatPosition() (int32, int32, bool) {
	if l.target != 0 && l.tracker.ObjectAlive(l.target) {
		if x, y, _, ok := l.tracker.ObjectPosition(l.target); ok {
			return x, y, true
		}
	}
	if pick, ok := l.tracker.NearestAttacker(); ok {
		return pick.X, pick.Y, true
	}

	return 0, 0, false
}

// panicPileUpRun answers the social pile up (two or more mobs
// hold the character as their target) with a run instead of the
// instant logout. The first call anchors the aggro point - the
// spot the pack piled up on - and drops the current fight (the
// walk away must not re-engage it), every later call walks one
// paced escape leg away from the threats. The logout fires only
// once the run opened panicRunDistance units between the character
// and the anchor: the chasing pack stays behind, the mobs drop the
// target and walk home while the character is offline, and the
// short relogin lands outside their aggro range instead of on top
// of the same pack. A run that cannot open the distance within the
// shared flee budget (a cornered or blocked escape) still logs out
// wherever it got to - the session must not run forever. So does a
// run whose pack dissolved on the way (no mob holds the target
// anymore, nothing attackable stands near): nothing is left to
// run from, the spot is as safe as the run gets.
func (l *Loop) panicPileUpRun(now time.Time) {
	if l.panicAt.IsZero() {
		x, y, _, ok := l.tracker.SelfPosition()
		if !ok {
			// No known position to measure the run from:
			// the instant logout is the only answer left.
			l.emergencyLogout()

			return
		}
		l.panicAt = now
		l.panicX, l.panicY = x, y
		l.logger.Printf("Hunt: %d mobs piled on us, running %.0f units "+
			"from the aggro point before the logout",
			l.tracker.SelfAttackerCount(), panicRunDistance)
		// Drop the fight the pack joined: the dropped target
		// lands on the long skip list (the run must not
		// re-engage it), the pending engage bookkeeping
		// clears - the same handoff fleeFromTarget makes.
		if l.target != 0 {
			if l.targetSkip == nil {
				l.targetSkip = make(map[int32]time.Time)
			}
			l.targetSkip[l.target] = now.Add(fleeSkipDelay)
			l.target = 0
			l.engageAt = time.Time{}
			l.clearBlindRecovery()
			l.noTargetSince = time.Time{}
		}
	}
	if dist := l.panicAnchorDistance(); dist >= panicRunDistance {
		l.logger.Printf("Hunt: %.0f units from the aggro point, "+
			"logging out", dist)
		l.emergencyLogout()

		return
	}
	if now.Sub(l.panicAt) >= fleeLogoutAfter {
		l.logger.Printf("Hunt: the pile up run could not open %.0f units "+
			"within %.0fs, logging out anyway",
			panicRunDistance, fleeLogoutAfter.Seconds())
		l.emergencyLogout()

		return
	}
	if !l.fleeAt.IsZero() && now.Sub(l.fleeAt) < selectPeriod {
		return
	}
	l.fleeAt = now
	if l.fleeSince.IsZero() {
		l.fleeSince = now
	}
	if !l.standUpGuarded(now) {
		return
	}
	moveX, moveY, moveZ, ok := l.escapeWalkDestination()
	if !ok {
		// No mob holds the target anymore and nothing
		// attackable stands within the escape range: the
		// pack dissolved, the chase is over wherever the
		// run got to. Log out now instead of idling out the
		// budget - the relogin spot is already clear.
		l.logger.Printf("Hunt: the pile up run shook the chase "+
			"at %.0f units, logging out", l.panicAnchorDistance())
		l.emergencyLogout()

		return
	}
	if err := l.game.WalkTo(moveX, moveY, moveZ); err != nil {
		l.logger.Printf("Hunt: pile up escape walk failed: %v", err)
	}
}

// panicAnchorDistance measures how far the character stands from
// the anchored aggro point of the pile up run (the planar distance:
// the height of the terrain does not make a mob pack closer).
func (l *Loop) panicAnchorDistance() float64 {
	x, y, _, ok := l.tracker.SelfPosition()
	if !ok {
		return 0
	}

	return math.Hypot(float64(x-l.panicX), float64(y-l.panicY))
}

// emergencyLogout saves a character with no way out: the health
// is critical, the blows keep landing and the escape could not
// shake the chase, or the pile up run opened its escape distance
// (or lapsed its budget) away from the pack. One last escape leg
// keeps the character moving through the combat window the server
// holds an offline character in the world (fifteen seconds), then
// the session logs out and the supervisor reconnects after the
// armed login cooldown - the aggro resets on the disappearance,
// the mobs left behind walk home, and the character regenerates
// sitting.
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
	l.panicAt = time.Time{}
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
