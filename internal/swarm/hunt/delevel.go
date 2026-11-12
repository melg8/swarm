// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Deleveling of the hunt loop: when the hunting ground holds only mobs
// far below the character level, the drop chance collapses (the Mobius
// level gap rules), so the bot walks to the town guards, provokes them
// and dies until the level is back in the rewarding range.
//
// Only archer guards are provoked, and always in melee: an archer guard
// in the attack intention shoots at anything inside its bow range and
// always retaliates, while a melee guard only follows the provoker and
// the chase dies in the checkTarget karma gate. The guard death penalty
// applies from level 10 up (the Lucky newbie skill absorbs it below), so
// the deleveling never starts under level 10 and never aims under 9; a
// character at the Life Cubic band (level 16) never aims under 16. The
// free death counter aborts a server that stops penalizing the deaths
// instead of looping forever. The server facts and the measured numbers:
// docs/hunting.md (the deleveling section).
package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// Timing and threshold constants of the deleveling.
const (
    // delevelTriggerDiff starts the deleveling when the character level
    // exceeds the median zone mob level by this much: the elven gremlins
    // cycle between 8 and 6.
    delevelTriggerDiff = int32(7)
    // delevelTargetGap is the delevel target above the median zone mob
    // level: mob level + 5 is the last level with the full item drop
    // chance of the level gap rules.
    delevelTargetGap = int32(5)
    // luckyProtectLevel is the level under which the Lucky newbie skill
    // (id 194, Player.isLucky) absorbs the death experience penalty:
    // verified live, a guard death at 9 removes nothing, at 10 it pays.
    luckyProtectLevel = int32(9)
    // delevelMinLevel is the lowest level a deleveling may start at: only
    // deaths at level 10 and up remove experience.
    delevelMinLevel = luckyProtectLevel + 1
    // delevelGuideFloorLevel is the lowest level the deleveling may aim
    // at once the character stands above it: the Life Cubic (skill 67)
    // rides the 16..19 window only, and the owner rule keeps the guide
    // effect over the drop chance below it. A character under 16 still
    // delevels to the Lucky floor, it never held the cubic.
    delevelGuideFloorLevel = guideCubicLevelMin
    // delevelMeleeRange is the approach distance of the provocation: the
    // archer guard answers in melee whatever its line of sight, and the
    // retreat kiting of the thinkAttack ticks cannot outpace it.
    delevelMeleeRange = 60.0
    // delevelFreeDeaths is the number of consecutive deaths that removed
    // no experience before the deleveling gives up: a server with the
    // penalty disabled would otherwise loop the deaths forever.
    delevelFreeDeaths = 3
    // delevelFreeCooldown is the long pause after a penalty-free
    // deleveling gave up: retrying the guard deaths of a server that never
    // penalizes them only burns the walks to the village.
    delevelFreeCooldown = 30 * time.Minute
    // delevelFightTimeout bounds a fight stage without the guard hitting
    // back (the usual cause is the village peace zone): the stage walks a
    // step out toward the farm spot and provokes again.
    delevelFightTimeout = 20 * time.Second
    // delevelCooldown pauses new deleveling after one ended, guarding
    // against a flickering median of the zone mob levels.
    delevelCooldown = time.Minute
    // delevelAbortBackoffBase is the pause the first consecutive aborted
    // deleveling arms: an abort leaves the level above the trigger, so a
    // flat cooldown would send the bot straight back to the guards. The
    // backoff buys real farming time and lets the server state change.
    delevelAbortBackoffBase = 5 * time.Minute
    // delevelAbortBackoffFactor is the growth of the abort backoff: every
    // further consecutive abort multiplies the wait.
    delevelAbortBackoffFactor = 5
    // delevelAbortBackoffMax caps the abort backoff at the free death
    // cooldown: the pause never grows beyond a half hour.
    delevelAbortBackoffMax = delevelFreeCooldown
    // delevelTimeout bounds a whole deleveling: 11 to 6 costs tens of
    // deaths at ~30 s each.
    delevelTimeout = 60 * time.Minute
)

// delevelGuards are the archer town guards the bot provokes, with their
// spawn coordinates from the Mobius C1 spawn data
// (ElvenTerritory/ElvenVillageNPCs.xml). TemplateID is the client display
// id the NpcInfo packet carries; archer guards only - the melee sentinels
// Veltress and Rayen may never retaliate (docs/hunting.md).
var delevelGuards = []townNpc{
    {TemplateID: 7218, Name: "Kendell", X: 47595, Y: 51569, Z: -2992},
    {TemplateID: 7220, Name: "Starden", X: 42971, Y: 51372, Z: -2992},
}

// delevelGuardTemplates lists the packet template ids of the guards.
func delevelGuardTemplates() []int32 {
    templates := make([]int32, 0, len(delevelGuards))
    for _, guard := range delevelGuards {
        templates = append(templates, guard.TemplateID+npcdata.NPCDisplayOffset)
    }

    return templates
}

// delevelCooldownOver reports whether a new deleveling may start: the
// short cooldown after a finished or aborted run and the long cooldown
// armed by the free death counter both have to be over.
func (l *Loop) delevelCooldownOver() bool {
    if !l.delevelWait.IsZero() && l.now().Before(l.delevelWait) {
        return false
    }

    return l.delevelEnd.IsZero() ||
        l.now().Sub(l.delevelEnd) >= delevelCooldown
}

// delevelTriggerMedian returns the zone mob level the delevel trigger
// measures against: the live median of the leash when it reads anything,
// the static median of the anchored cell otherwise. The target
// computation reads it too, so a trigger that fired on the static median
// never clamps its target to an empty live read.
func (l *Loop) delevelTriggerMedian() int32 {
    median := l.tracker.MedianZoneMobLevel(l.targetZone())
    if median > 0 {
        return median
    }
    if !l.onHeldGround() {
        return 0
    }

    return l.anchoredCellMedian()
}

// delevelWanted reports whether the character outleveled the hunting
// ground: the live median below the trigger difference, with the static
// median of the anchored cell as the stable fallback and the agreeing
// hysteresis when both read (the live median flickers with the respawn
// windows, the static one is the designed mix). The static answer only
// stands while the character holds the ground it belongs to. The guide
// floor gates the target: a target that cannot sit below the current
// level never starts the guard walk. The rules: docs/hunting.md.
func (l *Loop) delevelWanted() bool {
    if l.navigator == nil || !l.delevelCooldownOver() {
        return false
    }
    level := l.tracker.SelfLevel()
    if level < delevelMinLevel {
        return false
    }
    median := l.tracker.MedianZoneMobLevel(l.targetZone())
    if median <= 0 {
        if !l.onHeldGround() {
            return false
        }
        median = l.anchoredCellMedian()
        if median <= 0 {
            return false
        }
    } else if static := l.anchoredCellMedian(); static > 0 &&
        level-static < delevelTriggerDiff {
        return false
    }
    if level-median < delevelTriggerDiff {
        return false
    }

    return delevelTargetFor(level, median) < level
}

// delevelTargetFor computes the target level from the character level
// over the median ground: the last level of the full item drop chance
// (median + 5), raised to the Lucky floor and, above it, to the guide
// floor that keeps the level-keyed support magic.
func delevelTargetFor(level, median int32) int32 {
    target := median + delevelTargetGap
    if target < luckyProtectLevel {
        target = luckyProtectLevel
    }
    if level >= delevelGuideFloorLevel && target < delevelGuideFloorLevel {
        target = delevelGuideFloorLevel
    }

    return target
}

// anchoredCellMedian returns the count weighted median mob level of
// the currently held cell (0 when none is picked): the designed mob
// mix of the ground, the stable counterpart of the live median.
func (l *Loop) anchoredCellMedian() int32 {
    if l.cell == nil || l.cell.picked < 0 ||
        l.cell.picked >= len(l.cell.cells) {
        return 0
    }

    return cellMedianLevel(l.cell.cells[l.cell.picked])
}

// startDelevel begins the deleveling: the target level is computed from
// the median zone mob level and the walk to the nearest guard starts.
func (l *Loop) startDelevel() {
    median := l.delevelTriggerMedian()
    if median <= 0 {
        // The trigger and the median read race a knownlist that can
        // still empty between them: the target is undefined without
        // a median, the next tick re-triggers with a fresh read.
        return
    }
    level := l.tracker.SelfLevel()
    target := delevelTargetFor(level, median)
    l.rememberFarmSpot()
    l.tripStart = l.now()
    l.rePaths = 0
    l.delevelTarget = target
    l.delevelMedian = median
    l.delevelGuard = 0
    l.delevelFight = time.Time{}
    l.delevelTried = nil
    l.delevelExp = l.tracker.SelfExp()
    l.delevelLevel = level
    l.delevelFree = 0
    l.delevelCounted = false
    // The engage leftovers die with the phase switch: a selected mob or a
    // pending pickup never leaks into the guard walk.
    l.target = 0
    l.lootID = 0
    l.clearBlindRecovery()
    l.waypoints = nil
    l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.phase = phaseDelevel
    l.logf("Hunt: level %d is too high for level %d mobs (the level "+
        "gap collapsed the drops), deleveling to %d at the town "+
        "guards", level, median, target)
}

// delevelAbortWait returns the pause the next aborted deleveling arms:
// the base scaled by the consecutive abort streak, capped at the maximum.
func (l *Loop) delevelAbortWait() time.Duration {
    wait := delevelAbortBackoffBase
    for i := 1; i < l.delevelAborts && wait < delevelAbortBackoffMax; i++ {
        wait *= delevelAbortBackoffFactor
    }
    if wait > delevelAbortBackoffMax {
        wait = delevelAbortBackoffMax
    }

    return wait
}

// tickDelevel advances the deleveling by one decision: walk to the
// guard, provoke it, die, and start over until the target level is
// reached.
func (l *Loop) tickDelevel() {
    // The character is alive again: the next death counts fresh.
    l.delevelCounted = false
    if l.now().Sub(l.tripStart) > delevelTimeout {
        l.abortDelevel("delevel timed out")

        return
    }
    level := l.tracker.SelfLevel()
    if level > 0 && level <= l.delevelTarget {
        l.finishDelevel()

        return
    }
    if l.delevelFree >= delevelFreeDeaths {
        // The guard deaths of this server remove no experience: the level
        // never drops, give up and keep the long cooldown.
        l.abortDelevel("guard deaths remove no experience on this " +
            "server")
        l.delevelWait = l.now().Add(delevelFreeCooldown)

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
    l.fightDelevelGuard(l.now())
}

// planDelevelWalk plans the walk to the nearest guard spawn point.
func (l *Loop) planDelevelWalk() {
    guard, ok := l.nearestDelevelGuard()
    if !ok {
        l.abortDelevel("no known guard")

        return
    }
    l.delevelGuard = 0
    // The guard is an npc destination: the npc search radius plans the
    // walk to the spawn point itself, and the fight machinery owns the
    // last stretch onto the live guard.
    if !l.startWalkNpcSegment(townNpcPosition(guard)) {
        l.abortDelevel("no walkable path to the guard")

        return
    }
    l.logf("Hunt: walking to the guard %s", guard.Name)
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
// the attack once per second until the guard kills the character. A guard
// beating the character counts as progress; a guard that never fights
// back is marked as tried and the walk replans to the next one.
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
            // The NpcInfo of the guard did not arrive yet (or the nearest
            // one is a tried guard): keep walking toward the next spawn
            // point while waiting.
            if target, found := l.nearestDelevelGuard(); found {
                l.walkToward(target.X, target.Y, target.Z, now)
            }

            return
        }
        l.delevelGuard = guard.ObjectID
        l.delevelFight = now
        l.logf("Hunt: provoking the guard %s", guard.Name)
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
            // Not a single blow of this stage landed: the town guards sit
            // around level 70, the vanilla calcHitMiss floors the hit
            // chance at 20 percent there, and a whole timeout window
            // without a landed blow is a normal miss streak, not a stuck
            // guard. The guard answers the first hit, so the stage
            // extends.
            l.delevelFight = now

            return
        }
        l.delevelFight = now
        l.rePaths++
        if l.rePaths > maxRePaths {
            l.abortDelevel("the guard does not fight back")

            return
        }
        // The guard ignored landed damage: a stale AI state (an attack
        // intention from before the last death, a stuck follow task) keeps
        // ignoring the same character, and a damage event on another guard
        // resets hate and intention. The peace zone is no blocker - the
        // Mobius isInsidePeaceZone only guards playable versus playable
        // combat.
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
        l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}

        return
    }
    if now.Sub(l.lastActionAt) < engageRetryPeriod {
        return
    }
    l.lastActionAt = now
    if err := l.game.AttackTarget(l.delevelGuard); err != nil {
        l.logf("Hunt: guard attack failed: %v", err)
    }
}

// noteDelevelDeath counts one deleveling death against the experience the
// server actually removed: the server refreshes the exp in the UserInfo
// packet after every change (PlayerStat removeExpAndSp calls
// updateUserInfo), so the counting compares the exp before and after the
// death. The count happens once per death.
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

// finishDelevel returns the character to the hunt through the town trip
// return segment and resets the abort streak, so the next legitimate
// cycle starts from the short pause again.
func (l *Loop) finishDelevel() {
    l.delevelEnd = l.now()
    l.delevelTarget = 0
    l.delevelGuard = 0
    l.delevelAborts = 0
    l.logf("Hunt: delevel finished at level %d, walking back",
        l.tracker.SelfLevel())
    l.startDelevelReturnSegment()
}

// abortDelevel gives the deleveling up and returns to the hunt through the
// town trip return segment: a raw engage walk to the zone center is
// refused from the far guard posts (the 9900 unit move limit). The
// consecutive abort streak arms the escalating pause through the
// delevelWait gate, and only a finished deleveling resets it.
func (l *Loop) abortDelevel(reason string) {
    l.delevelEnd = l.now()
    l.delevelTarget = 0
    l.delevelGuard = 0
    l.delevelAborts++
    wait := l.delevelAbortWait()
    l.delevelWait = l.now().Add(wait)
    l.logf("Hunt: delevel aborted: %s (the next attempt waits %s)",
        reason, wait)
    l.startDelevelReturnSegment()
}

// startDelevelReturnSegment starts the walk back to the farm spot with a
// fresh trip budget: the return segment must not inherit the elapsed time
// of the deleveling, or the trip timeout would end it before it started.
func (l *Loop) startDelevelReturnSegment() {
    l.tripStart = l.now()
    l.rePaths = 0
    l.startReturnSegment()
}
