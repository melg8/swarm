// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Deleveling of the hunt loop: when the hunting ground holds only mobs
// far below the character level, the drop chance collapses (the Mobius
// level gap rules: item drops slide from 100% at mob level + 5 down to
// 10% at + 10 and beyond, experience and SP stop at + 11), so the bot
// walks to the town guards, provokes them and dies until the level is
// back in the rewarding range. The Mobius C1 death penalty removes a
// per level percentage of the current level span on every death
// (data/stats/players/experienceLoss.xml, ~9-10% at low levels, capped
// at 10% of the span, Delevel config enabled) - there is no low level
// immunity in this data pack, the deleveling stops at the computed
// target level instead.
package hunt

import (
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
)

// Timing and threshold constants of the deleveling.
const (
	// delevelTriggerDiff starts the deleveling when the character
	// level exceeds the median zone mob level by this much: with the
	// level 1 gremlins of the elven fields the cycle is delevel at 8
	// (the item drop chance slid to 82 percent) and hunt from 6 (full
	// drop chance) back to 8.
	delevelTriggerDiff = int32(7)
	// delevelTargetGap is the delevel target above the median zone mob
	// level: mob level + 5 is the last level with the full item drop
	// chance of the level gap rules.
	delevelTargetGap = int32(5)
	// delevelTargetFloor is the lowest level the deleveling ever aims
	// at, a safety net for degenerate medians.
	delevelTargetFloor = int32(5)
	// delevelAttackRange is the distance to approach the guard before
	// attacking it: the guard covers the last stretch itself once
	// provoked.
	delevelAttackRange = 300.0
	// delevelFightTimeout bounds a fight stage without the guard
	// hitting back: the usual cause is the village peace zone, the
	// guard never swings at a player standing inside it. The stage
	// walks a step out of the village toward the farm spot and
	// provokes again; the re-path budget of the trip aborts the
	// delevel when the walks do not help.
	delevelFightTimeout = 20 * time.Second
	// delevelCooldown pauses new deleveling after one ended, guarding
	// against a flickering median of the zone mob levels.
	delevelCooldown = time.Minute
	// delevelTimeout bounds a whole deleveling: 11 to 6 costs tens of
	// deaths at ~30 s each.
	delevelTimeout = 60 * time.Minute
)

// delevelGuards are the town guards the bot provokes for the death
// penalty, with their spawn coordinates from the Mobius C1 spawn data
// (ElvenTerritory/ElvenVillageNPCs.xml). TemplateID is the client
// display id the NpcInfo packet carries (the C1 spawn ids 30218..30221
// map to the CT0 display ids through CT0_to_C4_ids.txt). Any guard of
// the list kills a low level character in a few hits.
var delevelGuards = []townNpc{
	{TemplateID: 7218, Name: "Kendell", X: 47595, Y: 51569, Z: -2992},
	{TemplateID: 7219, Name: "Veltress", X: 47401, Y: 51764, Z: -2992},
	{TemplateID: 7220, Name: "Starden", X: 42971, Y: 51372, Z: -2992},
	{TemplateID: 7221, Name: "Rayen", X: 42812, Y: 51138, Z: -2992},
}

// delevelGuardTemplates lists the packet template ids of the guards.
func delevelGuardTemplates() []int32 {
	templates := make([]int32, 0, len(delevelGuards))
	for _, guard := range delevelGuards {
		templates = append(templates, guard.TemplateID+npcDisplayOffset)
	}

	return templates
}

// delevelCooldownOver reports whether a new deleveling may start.
func (l *Loop) delevelCooldownOver() bool {
	return l.delevelEnd.IsZero() ||
		time.Since(l.delevelEnd) >= delevelCooldown
}

// delevelWanted reports whether the character outleveled the hunting
// ground: the median level of the living attackable npcs inside the
// zone trails the character level by the trigger difference.
func (l *Loop) delevelWanted() bool {
	if l.navigator == nil || !l.delevelCooldownOver() {
		return false
	}
	level := l.tracker.SelfLevel()
	if level <= 0 {
		return false
	}
	median := l.tracker.MedianZoneMobLevel(l.zone())
	if median <= 0 {
		return false
	}

	return level-median >= delevelTriggerDiff
}

// startDelevel begins the deleveling: the target level is computed from
// the median zone mob level and the walk to the nearest guard starts.
func (l *Loop) startDelevel() {
	median := l.tracker.MedianZoneMobLevel(l.zone())
	target := median + delevelTargetGap
	if target < delevelTargetFloor {
		target = delevelTargetFloor
	}
	l.rememberFarmSpot()
	l.tripStart = time.Now()
	l.rePaths = 0
	l.delevelTarget = target
	l.delevelGuard = 0
	l.delevelFight = time.Time{}
	l.waypoints = nil
	l.phase = phaseDelevel
	l.logger.Printf("Hunt: level %d is too high for level %d mobs, "+
		"deleveling to %d at the town guards", l.tracker.SelfLevel(),
		median, target)
}

// tickDelevel advances the deleveling by one decision: walk to the
// guard, provoke it, die, and start over until the target level is
// reached.
func (l *Loop) tickDelevel() {
	if time.Since(l.tripStart) > delevelTimeout {
		l.abortDelevel("delevel timed out")

		return
	}
	level := l.tracker.SelfLevel()
	if level > 0 && level <= l.delevelTarget {
		l.finishDelevel()

		return
	}
	if l.waypoints == nil {
		l.planDelevelWalk()
		if l.waypoints == nil {
			// The planning aborted the deleveling.
			return
		}
	}
	if !l.walkTownWaypoints() {
		return
	}
	l.fightDelevelGuard(time.Now())
}

// planDelevelWalk plans the walk to the nearest guard spawn point.
func (l *Loop) planDelevelWalk() {
	guard, ok := l.nearestDelevelGuard()
	if !ok {
		l.abortDelevel("no known guard")

		return
	}
	l.delevelGuard = 0
	if !l.startWalkLeg(townNpcPosition(guard)) {
		l.abortDelevel("no walkable path to the guard")

		return
	}
	l.logger.Printf("Hunt: walking to the guard " + guard.Name)
}

// nearestDelevelGuard returns the guard closest to the character.
func (l *Loop) nearestDelevelGuard() (townNpc, bool) {
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		return townNpc{}, false
	}
	best := townNpc{}
	bestDist := math.MaxFloat64
	found := false
	for _, guard := range delevelGuards {
		dist := math.Hypot(
			float64(guard.X-selfX), float64(guard.Y-selfY))
		if dist < bestDist {
			bestDist = dist
			best = guard
			found = true
		}
	}

	return best, found
}

// fightDelevelGuard provokes the guard once close enough and re-requests
// the attack once per second until the guard kills the character. The
// guard beating the character counts as fight progress; a guard that
// never fights back (an ignored request, another geodata deck) re-paths
// the walk and aborts the delevel when the budget runs out.
func (l *Loop) fightDelevelGuard(now time.Time) {
	if l.delevelGuard == 0 || !l.tracker.ObjectAlive(l.delevelGuard) {
		guard, ok := l.tracker.NearestNpcByTemplates(
			delevelGuardTemplates(), merchantFindRadius)
		if !ok {
			// The NpcInfo of the guard did not arrive yet: keep
			// walking toward its spawn point while waiting.
			if target, found := l.nearestDelevelGuard(); found {
				l.walkToward(target.X, target.Y, target.Z, now)
			}

			return
		}
		l.delevelGuard = guard.ObjectID
		l.delevelFight = now
		l.logger.Printf("Hunt: provoking the guard " + guard.Name)
	}
	x, y, z, ok := l.tracker.ObjectPosition(l.delevelGuard)
	if !ok {
		l.delevelGuard = 0

		return
	}
	selfX, selfY, _, _ := l.tracker.SelfPosition()
	dist := math.Hypot(float64(x-selfX), float64(y-selfY))
	if dist > delevelAttackRange {
		l.walkToward(x, y, z, now)

		return
	}
	if l.tracker.SelfUnderAttack() {
		l.delevelFight = now
	} else if now.Sub(l.delevelFight) > delevelFightTimeout {
		l.delevelFight = now
		l.rePaths++
		if l.rePaths > maxRePaths {
			l.abortDelevel("the guard does not fight back")

			return
		}
		// The village peace zone blocks the guard retaliation: step
		// out of the village toward the farm spot and provoke again -
		// the guard follows and kills outside the zone.
		l.logger.Printf("Hunt: the guard does not attack, stepping out "+
			"of the peace zone (%d of %d)", l.rePaths, maxRePaths)
		l.walkToward(l.farmX, l.farmY, l.farmZ, now)

		return
	}
	if now.Sub(l.lastHit) < engageRetryPeriod {
		return
	}
	l.lastHit = now
	if err := l.game.AttackTarget(l.delevelGuard); err != nil {
		l.logger.Printf("Hunt: guard attack failed: %v", err)
	}
}

// finishDelevel returns the character to the hunt: the walk back to the
// farm spot reuses the town trip return leg, the engage routine resumes
// when it arrives.
func (l *Loop) finishDelevel() {
	l.delevelEnd = time.Now()
	l.delevelTarget = 0
	l.delevelGuard = 0
	l.logger.Printf("Hunt: delevel finished at level %d, walking back",
		l.tracker.SelfLevel())
	l.startReturnLeg()
}

// abortDelevel gives the deleveling up and returns to the hunt.
func (l *Loop) abortDelevel(reason string) {
	l.delevelEnd = time.Now()
	l.delevelTarget = 0
	l.delevelGuard = 0
	l.waypoints = nil
	l.legDest = pathfind.Vec3{}
	l.phase = phaseEngage
	l.target = 0
	l.lootID = 0
	l.logger.Printf("Hunt: delevel aborted: " + reason)
}
