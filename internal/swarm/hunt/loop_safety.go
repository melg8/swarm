// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The combat safety layer of the hunt loop: the flee decision, the paced escape
// walks and the emergency logout that ends a lost session.

import (
    "fmt"
    "math"
    "time"
)

// dropAttackTarget drops the held fight and the server side selection with it:
// a selection the loop left behind would resurrect as the next engage pick (the
// server never unselects on its own), so the drop sends the clear self click.
// The mob itself stays untouched, with no skip entry.
func (l *Loop) dropAttackTarget(reason string) {
    if l.target == 0 && l.tracker.SelfTargetID() == 0 {
        return
    }
    l.logf("Hunt: dropping the target %d: %s", l.target, reason)
    l.target = 0
    l.engageAt = time.Time{}
    l.resetFightClock()
    l.clearBlindRecovery()
    l.noTargetSince = time.Time{}
    l.noPickLogAt = time.Time{}
    l.clearServerSelection()
}

// fleeFromTarget drops a fight the character is losing and opens distance: the
// target lands on the long skip list (the walk back must not re-select it), the
// engage bookkeeping clears and the shared threat walk runs.
func (l *Loop) fleeFromTarget(targetID int32, now time.Time) {
    l.logf("Hunt: HP %.0f%%, fleeing the fight with %d",
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
    // The self click replaces the server side target with the own object, so a
    // stale selection never hands the abandoned mob to the next engage.
    l.clearServerSelection()
    l.fleeFromThreat(now)
}

// fleeFromThreat walks the character away from the nearest living threat (the
// chasing mob of a fled fight, an aggressive pull): one paced segment per call
// along the ordered escape lane fan (escapeWalkDestination), never toward the
// threat and never through the zone center the mob may hold. Two guards end a
// dead escape: no open lane at all and a window that moved less than
// escapeNoProgressDistance, both through the emergency logout. The bow
// character answers with kiteFleeThreat first.
func (l *Loop) fleeFromThreat(now time.Time) {
    if !l.fleeAt.IsZero() && now.Sub(l.fleeAt) < selectPeriod {
        return
    }
    l.fleeAt = now
    if l.kiteFleeThreat(now) {
        // The archer flight owns the defensive flee: the straight away ray, the
        // full escape segment, its own window and the re-click ladder. The fan
        // below only serves a walled lane, clamped to the hunting zone box.
        return
    }
    if l.kiteLayerArmed() {
        // The bow character never falls back to the zone-clamped melee fan: a
        // refused flight tick answers with the standing trade, while the fan
        // would log the session out at full health.
        return
    }
    if l.fleeSince.IsZero() {
        l.fleeSince = now
        l.noteFleeGap(now)
    }
    if now.Sub(l.fleeSince) >= fleeLogoutAfter {
        // The escape never shook the chase: the session ends and the login
        // cooldown resets the aggro while the character regenerates.
        l.logf("Hunt: fleeing for %.0fs without shaking "+
            "the chase, resetting the aggro via logout",
            now.Sub(l.fleeSince).Seconds())
        l.emergencyLogout()

        return
    }
    if l.escapeStalled(now) {
        // The clicks went out but the character did not travel (the H-006 dead
        // escape or a terrain pin): standing in the blows outlasts nothing.
        l.logf("Hunt: the escape moved less than %.0f units in "+
            "%.0fs, resetting the aggro via logout",
            escapeNoProgressDistance, escapeNoProgressWindow.Seconds())
        l.emergencyLogout()

        return
    }
    if gap, held := l.escapeNotOpening(now); held {
        // The character travels, but the chaser travels with it: a level 10
        // Kaboo Orc Fighter held ~19 units through 21 s of fleeing.
        l.logf("Hunt: the chaser keeps pace at %.0f units after "+
            "%.0fs of fleeing, resetting the aggro via logout",
            gap, fleeGapWindow.Seconds())
        l.emergencyLogout()

        return
    }
    if !l.standUpGuarded(now) {
        return
    }
    moveX, moveY, moveZ, ok := l.escapeWalkDestination()
    if !ok {
        if _, _, hasThreat := l.threatPosition(); hasThreat {
            // Every ordered lane is clamped to nothing or folds toward the
            // threat: a cornered character with the chase still on it.
            l.logf("Hunt: no escape lane opens from the chase, " +
                "resetting the aggro via logout")
            l.emergencyLogout()

            return
        }
        // The blows landed but no mob stands around anymore:
        // the under attack window closes on its own in seconds.
        return
    }
    if err := l.game.WalkTo(moveX, moveY, moveZ); err != nil {
        l.logf("Hunt: escape walk failed: %v", err)
    }
    l.noteEscapeWalk(now)
}

// escapeWalkDestination plans one escape segment away from the nearest threat
// as an ordered lane fan: the straight away ray first, then the 45 and 90
// degree lanes each side. Every candidate is clamped along its ray into the
// hunting square, so an escape that would leave the ground stops on the inner
// margin; the first lane with a usable length wins, which keeps the direction
// stable between ticks.
func (l *Loop) escapeWalkDestination() (int32, int32, int32, bool) {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return 0, 0, 0, false
    }
    threatX, threatY, hasThreat := l.threatPosition()
    if !hasThreat {
        return 0, 0, 0, false
    }
    awayX := float64(selfX - threatX)
    awayY := float64(selfY - threatY)
    dist := math.Hypot(awayX, awayY)
    if dist <= 1 {
        return 0, 0, 0, false
    }
    awayX /= dist
    awayY /= dist
    var rays [1 + 2*escapeFanSteps][2]float64
    rays[0] = [2]float64{awayX, awayY}
    count := 1
    for step := 1; step <= escapeFanSteps; step++ {
        angle := escapeFanStep * float64(step)
        for _, sign := range [2]float64{1, -1} {
            rays[count][0], rays[count][1] =
                rotatePlanar(awayX, awayY, sign*angle)
            count++
        }
    }
    for _, ray := range rays[:count] {
        reach := l.escapeLaneReach(selfX, selfY, ray[0], ray[1])
        if reach < escapeMinLane {
            continue
        }
        moveX := selfX + int32(math.Round(ray[0]*reach))
        moveY := selfY + int32(math.Round(ray[1]*reach))

        return moveX, moveY, selfZ, true
    }

    return 0, 0, 0, false
}

// escapeLaneReach returns the length of one escape lane along its unit ray: the
// full escapeWalkDistance, clamped at the inner margin of the hunting square. A
// nil zone never clamps, and a lane with no room past the margin reports zero.
func (l *Loop) escapeLaneReach(
    selfX, selfY int32, dirX, dirY float64,
) float64 {
    zone := l.zone()
    if zone == nil {
        return escapeWalkDistance
    }
    reach := math.Min(escapeWalkDistance, escapeSlabExit(
        float64(selfX), dirX,
        float64(zone.CX-zone.Half)+escapeZoneMargin,
        float64(zone.CX+zone.Half)-escapeZoneMargin))
    reach = math.Min(reach, escapeSlabExit(
        float64(selfY), dirY,
        float64(zone.CY-zone.Half)+escapeZoneMargin,
        float64(zone.CY+zone.Half)-escapeZoneMargin))
    if reach < 0 {
        return 0
    }

    return reach
}

// escapeSlabExit returns the distance from the origin along one axis direction
// to the inner boundary of the slab [lo, hi]. A parallel direction never
// clamps, and an origin past the boundary reports a negative reach.
func escapeSlabExit(origin, dir, lo, hi float64) float64 {
    switch {
    case dir > 1e-9:
        return (hi - origin) / dir
    case dir < -1e-9:
        return (lo - origin) / dir
    default:
        return math.Inf(1)
    }
}

// escapeStalled reports whether the escape stopped moving: less than
// escapeNoProgressDistance covered since the anchor inside
// escapeNoProgressWindow. It only measures episodes that issued a walk, and it
// re-anchors every accepted window, so a long run keeps measuring rolling
// progress.
func (l *Loop) escapeStalled(now time.Time) bool {
    if l.escapeWalkAt.IsZero() || l.escapeAnchorAt.IsZero() {
        return false
    }
    x, y, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    if now.Sub(l.escapeAnchorAt) < escapeNoProgressWindow {
        return false
    }
    moved := math.Hypot(
        float64(x-l.escapeAnchorX), float64(y-l.escapeAnchorY))
    if moved < escapeNoProgressDistance {
        return true
    }
    l.escapeAnchorAt = now
    l.escapeAnchorX, l.escapeAnchorY = x, y

    return false
}

// noteFleeGap anchors the closure-rate watchdog of the escape: the episode
// start and the distance to its threat then. A chase with no threat to measure
// arms nothing.
func (l *Loop) noteFleeGap(now time.Time) {
    l.fleeGapAt = now
    l.fleeGap = 0
    x, y, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    threatX, threatY, ok := l.threatPosition()
    if !ok {
        return
    }
    l.fleeGap = math.Hypot(float64(x-threatX), float64(y-threatY))
}

// escapeNotOpening reports whether the chase held its distance through the
// probe window: the window lapsed and the threat is still no farther than
// fleeGapOpen past the anchor gap, so the chaser runs at the character's speed.
// A widened gap re-anchors the probe, and no live threat arms nothing.
func (l *Loop) escapeNotOpening(now time.Time) (float64, bool) {
    if l.fleeGapAt.IsZero() {
        return 0, false
    }
    if now.Sub(l.fleeGapAt) < fleeGapWindow {
        return 0, false
    }
    x, y, _, ok := l.tracker.SelfPosition()
    if !ok {
        return 0, false
    }
    threatX, threatY, ok := l.threatPosition()
    if !ok {
        return 0, false
    }
    gap := math.Hypot(float64(x-threatX), float64(y-threatY))
    if gap >= l.fleeGap+fleeGapOpen {
        l.fleeGapAt = now
        l.fleeGap = gap

        return gap, false
    }

    return gap, true
}

// noteEscapeWalk starts the no-progress window: the first walk of the episode
// anchors it, later walks leave the running window alone.
func (l *Loop) noteEscapeWalk(now time.Time) {
    l.escapeWalkAt = now
    if !l.escapeAnchorAt.IsZero() {
        return
    }
    x, y, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    l.escapeAnchorAt = now
    l.escapeAnchorX, l.escapeAnchorY = x, y
}

// clearEscapeAnchor ends the no-progress window and the closure-rate probe with
// the episode; the next escape starts fresh windows.
func (l *Loop) clearEscapeAnchor() {
    l.escapeAnchorAt = time.Time{}
    l.escapeAnchorX, l.escapeAnchorY = 0, 0
    l.escapeWalkAt = time.Time{}
    l.fleeGapAt = time.Time{}
    l.fleeGap = 0
}

// threatPosition returns the position of the mob the escape runs from: the
// living target while one is engaged, the nearest attacker otherwise. A mob
// that merely stands nearby is not a threat, or a finished fight would run the
// character off the kill spot.
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

// panicPileUpRun answers the social pile up (two or more mobs hold the
// character as their target) with a run instead of the instant logout. The
// first call anchors the aggro point and drops the fight, every later call
// walks one paced escape segment. The logout fires once the run opened
// panicRunDistance units, so the chasing pack stays behind and the relogin
// lands outside its aggro range; a run that cannot open the distance in the
// flee budget logs out wherever it got to.
func (l *Loop) panicPileUpRun(now time.Time) {
    if l.panicAt.IsZero() {
        x, y, _, ok := l.tracker.SelfPosition()
        if !ok {
            // No position to measure the run from: log out at once.
            l.emergencyLogout()

            return
        }
        l.panicAt = now
        l.panicX, l.panicY = x, y
        l.logger.Printf("Hunt: %d mobs piled on us, running %.0f units "+
            "from the aggro point before the logout",
            l.tracker.SelfAttackerCount(), panicRunDistance)
        l.dropPileUpFight(now)
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
        if _, _, hasThreat := l.threatPosition(); hasThreat {
            // A chaser still holds the character while every lane is walled:
            // the cornered pile up.
            l.logger.Printf("Hunt: no escape lane opens from the "+
                "pile up at %.0f units, logging out",
                l.panicAnchorDistance())
            l.emergencyLogout()

            return
        }
        // The pack dissolved: log out now instead of idling out the budget.
        l.logger.Printf("Hunt: the pile up run shook the chase "+
            "at %.0f units, logging out", l.panicAnchorDistance())
        l.emergencyLogout()

        return
    }
    if err := l.game.WalkTo(moveX, moveY, moveZ); err != nil {
        l.logger.Printf("Hunt: pile up escape walk failed: %v", err)
    }
}

// dropPileUpFight drops the fight the pack joined: the target lands on the long
// skip list and the engage bookkeeping clears, the handoff fleeFromTarget
// makes.
func (l *Loop) dropPileUpFight(now time.Time) {
    if l.target == 0 {
        return
    }
    if l.targetSkip == nil {
        l.targetSkip = make(map[int32]time.Time)
    }
    l.targetSkip[l.target] = now.Add(fleeSkipDelay)
    l.target = 0
    l.engageAt = time.Time{}
    l.clearBlindRecovery()
    l.noTargetSince = time.Time{}
}

// panicAnchorDistance measures the planar distance from the anchored aggro
// point: the terrain height does not make a mob pack closer.
func (l *Loop) panicAnchorDistance() float64 {
    x, y, _, ok := l.tracker.SelfPosition()
    if !ok {
        return 0
    }

    return math.Hypot(float64(x-l.panicX), float64(y-l.panicY))
}

// emergencyLogout saves a character with no way out: the health is critical,
// the blows keep landing and the escape could not shake the chase, or the pile
// up run opened its distance. One last escape segment keeps the character
// moving through the fifteen second combat window the server holds an offline
// character in the world, then the session logs out and the login cooldown
// resets the aggro. The logout also counts against the zone it fires in (85
// logouts on one ground in an observed run).
func (l *Loop) emergencyLogout() {
    reason := fmt.Sprintf("HP %.0f%% under attack",
        l.tracker.SelfHealthPercent())
    if count := l.tracker.SelfAttackerCount(); count >= panicLogoutAttackers {
        reason = fmt.Sprintf("%d mobs piled on us", count)
    }
    l.noteZoneDanger()
    l.emergencyLogoutWithReason(reason)
}

// emergencyLogoutWithReason is the shared body of the emergency logout: the
// reason names what drove the session out, the escape segment keeps the
// character moving through the logout combat window and the login cooldown
// paces the reconnect. The stagnation recovery calls it with its own reason
// (stagnation.go).
func (l *Loop) emergencyLogoutWithReason(reason string) {
    l.logoutDone = true
    l.logf("Hunt: %s, emergency logout for %s",
        reason, panicLogoutPause)
    // The structured logout event and the tracker reason replace the "use of
    // closed network connection" line the supervisor would book for the socket
    // close.
    l.tracker.SetEmergencyLogout(reason)
    if l.journal != nil {
        l.journal.Logout(l.tracker.ID(), reason, panicLogoutPause)
    }
    if moveX, moveY, moveZ, ok := l.escapeWalkDestination(); ok {
        if err := l.game.WalkTo(moveX, moveY, moveZ); err != nil {
            l.logf("Hunt: escape walk failed: %v", err)
        }
    }
    l.tracker.SetLoginCooldown(panicLogoutPause)
    l.panicAt = time.Time{}
    l.clearEscapeAnchor()
    if err := l.game.RequestLogout(); err != nil {
        l.logf("Hunt: logout request failed: %v", err)
    }
}

// targetSkipped reports whether the object id is held out of the engage search:
// a stuck pick keeps its short delay, a fled target its long one, both in one
// expiry map.
func (l *Loop) targetSkipped(objectID int32, now time.Time) bool {
    until, ok := l.targetSkip[objectID]

    return ok && now.Before(until)
}

// attackerEngageable reports whether the character may answer the attacker by
// fighting: the mob level sits inside the engage ceiling and the character is
// healthy enough. The ceiling is the general pick ceiling (the character level
// plus the slack) for every profile - a measured race of a level 7 archer
// against a level 10 Kaboo Orc Fighter took it from 100 to 59 percent of its
// health in sixteen seconds. An unresolved template passes like every level
// filter.
func (l *Loop) attackerEngageable(objectID int32) bool {
    if l.tracker.SelfHealthPercent() < reengageHealthPercent {
        return false
    }
    level, ok := l.tracker.ObjectLevel(objectID)
    if !ok || level <= 0 {
        return true
    }
    maxLevel := l.maxTargetLevel()

    return maxLevel <= 0 || level <= maxLevel
}

// pileUpWinnable reports whether a social pile up answers with a fight instead
// of the panic run: the character is healthy enough, every attacker sits inside
// the engage level ceiling and the pack is small enough to tank (the two mob
// softlock, where the panic run restarts the same aggro cycle on both mobs).
// The health gate keeps the verdict dynamic: a fight that goes bad re-arms the
// panic run, a kill that thins the pack hands the answer to the ordinary aggro
// flow, and a target inside the kill window is finished first.
func (l *Loop) pileUpWinnable() bool {
    if l.tracker.SelfHealthPercent() < reengageHealthPercent {
        return false
    }
    attackers := l.tracker.SelfAttackers()
    if len(attackers) > pileUpFightMaxAttackers {
        return false
    }
    maxLevel := l.maxTargetLevel()
    for i := range attackers {
        level, ok := l.tracker.ObjectLevel(attackers[i].ObjectID)
        if !ok || level <= 0 {
            continue
        }
        if maxLevel > 0 && level > maxLevel {
            return false
        }
    }

    return true
}

// activeSkips collects the object ids whose skip has not expired into the
// reused dense scratch list (see skipScratch).
func (l *Loop) activeSkips(now time.Time) []int32 {
    l.skipScratch = l.skipScratch[:0]
    for objectID, until := range l.targetSkip {
        if now.Before(until) {
            l.skipScratch = append(l.skipScratch, objectID)
        }
    }

    return l.skipScratch
}
