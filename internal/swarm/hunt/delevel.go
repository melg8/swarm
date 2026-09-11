// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Deleveling of the hunt loop: when the hunting ground holds only mobs
// far below the character level, the drop chance collapses (the Mobius
// level gap rules: item drops slide from 100% at mob level + 5 down to
// 10% at + 10 and beyond, experience and SP stop at + 11), so the bot
// walks to the town guards, provokes them and dies until the level is
// back in the rewarding range.
//
// Only archer guards are provoked, and always in melee (Elven Village:
// the sentinels Kendell and Starden carry the Elven Bow with the ARCHER
// ai type). An archer guard in the attack intention shoots at anything
// inside its 850+ unit bow range - the thinkAttack doAttack branch
// applies no karma gate - so it always retaliates on a provocation in
// its direct line of sight. A melee guard only chases the provoker
// (Guard.addDamage startFollow) and the chase dies in the checkTarget
// gate (Player.isAutoAttackable returns karma > 0 for guards), so melee
// guards may follow the character around without ever swinging.
//
// The experience penalty of a guard death only applies from level 10
// up: verified live on the vanilla server (the DEATHLOG diagnostics of
// the local checkout), a guard death at level 9 removes no experience
// at all (the Lucky newbie skill absorbs it, Player.isLucky gates at
// level <= 9) while a guard death at level 10 pays the penalty - the
// doDie penalty branch runs for every killer, guards and mobs pay it
// just like players - removing a per level percentage of the level span
// (data/stats/players/experienceLoss.xml, ~9% at low levels, capped at
// 10%). The deleveling therefore never triggers below level 10 and its
// target never aims below 9. The free death counter stays as the safety
// net: if the deaths ever stop removing experience (a penalty disabled
// by configuration, a rebalanced Lucky), a few penalty-free deaths
// abort the deleveling instead of an endless death loop.
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
	// luckyProtectLevel is the level under which the Lucky newbie skill
	// (id 194, granted with the character creation) absorbs the death
	// experience penalty on the server (Player.isLucky: level <= 9).
	// Verified in the world: a guard death at level 9 removes nothing,
	// a guard death at level 10 pays the penalty.
	luckyProtectLevel = int32(9)
	// delevelMinLevel is the lowest level a deleveling may ever start
	// at: the deaths have to remove experience and only deaths at
	// level 10 and up do. An attempt at level 9 or below can only
	// burn walks and deaths without dropping a single level.
	delevelMinLevel = luckyProtectLevel + 1
	// delevelMeleeRange is the approach distance of the provocation:
	// the deleveling strikes the archer guards in melee, where the
	// bow answer of the guard always lands (a far provocation may sit
	// outside the line of sight or the bow range) and the archer
	// retreat kiting of the thinkAttack ticks cannot outpace it.
	delevelMeleeRange = 60.0
	// delevelFreeDeaths is the number of consecutive deleveling deaths
	// that removed no experience before the deleveling gives up: the
	// vanilla guard deaths pay the penalty from level 10 up (verified
	// live), but a server with the penalty disabled or a rebalanced
	// Lucky would make every death free - the level then never drops
	// and the counter turns the endless death loop into an abort.
	delevelFreeDeaths = 3
	// delevelFreeCooldown is the long pause after a penalty-free
	// deleveling gave up: retrying the guard deaths of a server that
	// never penalizes them would only burn the walks to the village.
	delevelFreeCooldown = 30 * time.Minute
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
	// returnWalkLeg caps the direct walk requests of the zone return:
	// the server refuses move requests with a target farther than 9900
	// units (MoveToLocation runImpl), and a village respawn, a guard
	// post of the deleveling or a long chase easily starts farther than
	// that from the zone center, which locked the loop into an endless
	// stream of refused requests.
	returnWalkLeg = 1000.0
	// delevelTimeout bounds a whole deleveling: 11 to 6 costs tens of
	// deaths at ~30 s each.
	delevelTimeout = 60 * time.Minute
)

// delevelGuards are the archer town guards the bot provokes for the
// death penalty, with their spawn coordinates from the Mobius C1 spawn
// data (ElvenTerritory/ElvenVillageNPCs.xml). TemplateID is the client
// display id the NpcInfo packet carries (the C1 spawn ids 30218 and
// 30220 map to the CT0 display ids through CT0_to_C4_ids.txt).
// Archer guards only: Kendell and Starden carry the Elven Bow (npc
// equipment rhand 276, attack type BOW, range 1100, ai type ARCHER)
// while the sentinels Veltress (30219) and Rayen (30221) carry the
// Elven Sword with a 40 unit attack range - melee guards whose
// retaliation may never start (see the package comment). Any guard of
// the list kills a low level character in a few hits.
var delevelGuards = []townNpc{
	{TemplateID: 7218, Name: "Kendell", X: 47595, Y: 51569, Z: -2992},
	{TemplateID: 7220, Name: "Starden", X: 42971, Y: 51372, Z: -2992},
}

// delevelGuardTemplates lists the packet template ids of the guards.
func delevelGuardTemplates() []int32 {
	templates := make([]int32, 0, len(delevelGuards))
	for _, guard := range delevelGuards {
		templates = append(templates, guard.TemplateID+npcDisplayOffset)
	}

	return templates
}

// delevelCooldownOver reports whether a new deleveling may start: the
// short cooldown after a finished or aborted run and the long cooldown
// armed by the free death counter both have to be over.
func (l *Loop) delevelCooldownOver() bool {
	if !l.delevelWait.IsZero() && time.Now().Before(l.delevelWait) {
		return false
	}

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

	return (level-median >= delevelTriggerDiff) &&
		(level >= delevelMinLevel)
}

// startDelevel begins the deleveling: the target level is computed from
// the median zone mob level and the walk to the nearest guard starts.
func (l *Loop) startDelevel() {
	median := l.tracker.MedianZoneMobLevel(l.zone())
	target := median + delevelTargetGap
	if target < luckyProtectLevel {
		// The Lucky newbie protection absorbs the death exp penalty
		// below this level: aim at the lowest reachable level instead.
		// The last productive death happens at level 10 and drops
		// the character to 9, where the deleveling finishes.
		target = luckyProtectLevel
	}
	l.rememberFarmSpot()
	l.tripStart = time.Now()
	l.rePaths = 0
	l.delevelTarget = target
	l.delevelGuard = 0
	l.delevelFight = time.Time{}
	l.delevelTried = nil
	l.delevelExp = l.tracker.SelfExp()
	l.delevelLevel = l.tracker.SelfLevel()
	l.delevelFree = 0
	l.delevelCounted = false
	l.waypoints = nil
	l.legStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
	l.waterEscape = false
	l.phase = phaseDelevel
	l.logf("Hunt: level %d is too high for level %d mobs, "+
		"deleveling to %d at the town guards", l.tracker.SelfLevel(),
		median, target)
}

// tickDelevel advances the deleveling by one decision: walk to the
// guard, provoke it, die, and start over until the target level is
// reached.
func (l *Loop) tickDelevel() {
	// The character is alive again: the next death counts fresh.
	l.delevelCounted = false
	if time.Since(l.tripStart) > delevelTimeout {
		l.abortDelevel("delevel timed out")

		return
	}
	level := l.tracker.SelfLevel()
	if level > 0 && level <= l.delevelTarget {
		l.finishDelevel()

		return
	}
	if l.delevelFree >= delevelFreeDeaths {
		// The guard deaths of this server remove no experience (a
		// penalty disabled by configuration, a rebalanced Lucky):
		// the level never drops, give up and keep the long
		// cooldown from retrying the walks.
		l.abortDelevel("guard deaths remove no experience on this " +
			"server")
		l.delevelWait = time.Now().Add(delevelFreeCooldown)

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
	l.logf("Hunt: walking to the guard " + guard.Name)
}

// nearestDelevelGuard returns the guard closest to the character,
// skipping the guards that already ignored a provocation during this
// deleveling (a guard with a stale AI state keeps ignoring the same
// character, a fresh guard resets the fight).
func (l *Loop) nearestDelevelGuard() (townNpc, bool) {
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		return zeroTownNpc, false
	}
	best := zeroTownNpc
	bestDist := math.MaxFloat64
	found := false
	for _, guard := range delevelGuards {
		if l.delevelTried[guard.Name] {
			continue
		}
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
// never fights back is marked as tried and the walk replans to the
// next guard, aborting the delevel when every guard ignored the
// provocation.
// Pre-consolidation phase debt; the hunt loop cleanup is planned
// (docs/quality_review_and_agent_prompts.md P07).
//
//nolint:cyclop,funlen,gocognit // the delevel guard decision tree
func (l *Loop) fightDelevelGuard(now time.Time) {
	if l.delevelGuard == 0 || !l.tracker.ObjectAlive(l.delevelGuard) {
		guard, ok := l.tracker.NearestNpcByTemplates(
			delevelGuardTemplates(), merchantFindRadius)
		if ok && l.delevelTried[guard.Name] {
			// The guard already ignored this deleveling: do not
			// re-provoke it, walk to a fresh one instead.
			ok = false
		}
		if !ok {
			// The NpcInfo of the guard did not arrive yet (or the
			// nearest one is a tried guard): keep walking toward
			// the next spawn point while waiting. The spawn to
			// spawn walks stay far below the 9900 unit server
			// move limit.
			if target, found := l.nearestDelevelGuard(); found {
				l.walkToward(target.X, target.Y, target.Z, now)
			}

			return
		}
		l.delevelGuard = guard.ObjectID
		l.delevelFight = now
		l.logf("Hunt: provoking the guard " + guard.Name)
	}
	x, y, z, ok := l.tracker.ObjectPosition(l.delevelGuard)
	if !ok {
		l.delevelGuard = 0

		return
	}
	selfX, selfY, _, _ := l.tracker.SelfPosition()
	dist := math.Hypot(float64(x-selfX), float64(y-selfY))
	if dist > delevelMeleeRange {
		l.walkToward(x, y, z, now)

		return
	}
	if l.tracker.SelfUnderAttack() {
		l.delevelFight = now
	} else if now.Sub(l.delevelFight) > delevelFightTimeout {
		hitTarget, hitAt := l.tracker.SelfLandedHit()
		if hitTarget != l.delevelGuard || !hitAt.After(l.delevelFight) {
			// Not a single blow of this stage landed: the town guards
			// sit around level 70 while the deleveling character
			// climbs down from the low tens, and the vanilla
			// calcHitMiss floors the hit chance at 20 percent there -
			// a whole timeout window without a landed blow is the
			// normal miss streak of that gap, not a stuck guard (the
			// guard has no damage to retaliate against yet). Extending
			// the stage keeps the provocation going: the guard that
			// gets hit always answers, the streak always ends.
			l.delevelFight = now

			return
		}
		l.delevelFight = now
		l.rePaths++
		if l.rePaths > maxRePaths {
			l.abortDelevel("the guard does not fight back")

			return
		}
		// The guard ignored landed damage. Marking it as tried and
		// replanning the walk switches the deleveling to a fresh
		// guard: a guard with a stale AI state (an attack intention
		// from before the last death, a stuck follow task) keeps
		// ignoring the same character, while a new damage event on
		// another guard resets both the hate and the intention
		// cleanly. The peace zone is not the blocker here - the
		// Mobius isInsidePeaceZone only guards playable versus
		// playable combat, guards swing at players anywhere.
		guardName := l.tracker.ObjectName(l.delevelGuard)
		l.logf("Hunt: the guard %s does not attack, trying "+
			"the next one (%d of %d)", guardName, l.rePaths,
			maxRePaths)
		l.delevelGuard = 0
		if guardName != "" {
			if l.delevelTried == nil {
				l.delevelTried = make(map[string]bool)
			}
			l.delevelTried[guardName] = true
		}
		l.waypoints = nil
		l.legStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
		l.waterEscape = false

		return
	}
	if now.Sub(l.lastHit) < engageRetryPeriod {
		return
	}
	l.lastHit = now
	if err := l.game.AttackTarget(l.delevelGuard); err != nil {
		l.logf("Hunt: guard attack failed: %v", err)
	}
}

// noteDelevelDeath counts one deleveling death against the experience
// the server actually removed with it. The vanilla guard deaths pay the
// death penalty from level 10 up (verified live), and the server
// refreshes the exp in the UserInfo packet after every change
// (PlayerStat removeExpAndSp calls updateUserInfo), so the counting is
// a straight comparison of the exp before and after the death; a stale
// value after the death means nothing was removed. The count happens
// once per death: the character stays dead across many ticks while the
// village restart request retries.
func (l *Loop) noteDelevelDeath() {
	if l.delevelCounted {
		return
	}
	l.delevelCounted = true
	level := l.tracker.SelfLevel()
	exp := l.tracker.SelfExp()
	progressed := exp < l.delevelExp || level < l.delevelLevel
	l.delevelExp, l.delevelLevel = exp, level
	if progressed {
		l.delevelFree = 0

		return
	}
	l.delevelFree++
	l.logf("Hunt: delevel death %d removed no experience",
		l.delevelFree)
}

// finishDelevel returns the character to the hunt: the walk back to the
// farm spot reuses the town trip return leg, the engage routine resumes
// when it arrives.
func (l *Loop) finishDelevel() {
	l.delevelEnd = time.Now()
	l.delevelTarget = 0
	l.delevelGuard = 0
	l.logf("Hunt: delevel finished at level %d, walking back",
		l.tracker.SelfLevel())
	l.startDelevelReturnLeg()
}

// abortDelevel gives the deleveling up and returns to the hunt through
// the town trip return leg as well: the raw engage phase would issue one
// direct walk to the zone center, which the server refuses from the far
// guard posts (the 9900 unit move limit) and the loop would hang on the
// refused requests.
func (l *Loop) abortDelevel(reason string) {
	l.delevelEnd = time.Now()
	l.delevelTarget = 0
	l.delevelGuard = 0
	l.logf("Hunt: delevel aborted: " + reason)
	l.startDelevelReturnLeg()
}

// startDelevelReturnLeg starts the walk back to the farm spot with a
// fresh trip budget: the deleveling ran under its own timeout and the
// return leg must not inherit its elapsed time, or the trip timeout
// would end the return before it started.
func (l *Loop) startDelevelReturnLeg() {
	l.tripStart = time.Now()
	l.rePaths = 0
	l.startReturnLeg()
}
