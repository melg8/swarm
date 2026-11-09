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

// dropAttackTarget drops the held fight and the server side target
// selection with it: the travel tasks of the loop (the walk to town,
// the route change of a zone or ground rotation) own the target
// register - a loop target the selection keeps would resurrect on the
// next engage tick (the re-adopt reads the server view of the
// selection, and the server never unselects on its own), so the drop
// sends the clear self click whenever a selection exists. The mob
// itself stays untouched - no skip entry, the next pick may fight it
// again when the travel is over.
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
    l.clearTalkedTarget()
}

// fleeFromTarget drops a fight the character is losing and opens
// distance: the target lands on the long skip list (the walk back
// must not re-select it), the pending engage bookkeeping clears and
// the shared threat walk runs.
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
    l.fleeFromThreat(now)
}

// fleeFromThreat walks the character away from the nearest living
// threat (the chasing mob of a fled fight, an aggressive pull it
// never selected): one paced segment per call along the ordered
// escape lane fan (see escapeWalkDestination), never toward the
// threat and never through the zone center the mob may hold. A
// sitting character stands up first - the server refuses move
// requests while it sits. Two guards end a dead escape instead of
// running it forever: no open lane at all (the cornered or walled
// pocket) and a window that moved less than escapeNoProgressDistance
// (the silent H-006 click drop, a walk pinned against the terrain).
// Both reset the aggro through the emergency logout - the user rule
// "relog when the fight cannot be won".
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
        l.logf("Hunt: fleeing for %.0fs without shaking "+
            "the chase, resetting the aggro via logout",
            now.Sub(l.fleeSince).Seconds())
        l.emergencyLogout()

        return
    }
    if l.escapeStalled(now) {
        // The clicks went out but the character did not travel:
        // the dead escape of H-006 or a terrain pin. Standing in
        // the blows outlasts nothing - the session resets.
        l.logf("Hunt: the escape moved less than %.0f units in "+
            "%.0fs, resetting the aggro via logout",
            escapeNoProgressDistance, escapeNoProgressWindow.Seconds())
        l.emergencyLogout()

        return
    }
    if !l.standUpGuarded(now) {
        return
    }
    moveX, moveY, moveZ, ok := l.escapeWalkDestination()
    if !ok {
        if _, _, hasThreat := l.threatPosition(); hasThreat {
            // Every ordered lane is clamped to nothing or folds
            // toward the threat: a cornered character with the
            // chase still on it. The walk would not open distance,
            // the relog resets the aggro instead.
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

// escapeWalkDestination plans one escape segment away from the nearest
// threat as an ORDERED lane fan: the straight away ray first, then the
// 45 and 90 degree lanes each side of it. Every candidate is clamped
// along its own ray into the hunting square, so an escape that would
// leave the ground stops on the safe inner margin instead of turning
// toward the zone center - the old center fallback ran the character
// back through the chasing mob whenever the straight away point left
// the square, the reported 180 degree flips. No candidate points into
// the far side of the away hemisphere, so the run always opens
// distance; the first lane with a usable length wins, which keeps the
// direction stable between ticks while the geometry does.
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

// escapeLaneReach returns the length of one escape lane along its
// unit ray: the full escapeWalkDistance, clamped at the inner margin
// of the hunting square. A nil zone (the legacy no-leash mode) never
// clamps, and a lane with no room past the margin reports zero.
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

// escapeSlabExit returns the distance from the origin along one axis
// direction to the inner boundary of the slab [lo, hi]. A direction
// parallel to the slab never clamps; an origin already past the
// boundary has no room at all (a negative reach).
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
// escapeNoProgressWindow. It only measures episodes that issued a
// walk (escapeWalkAt), so the stand-up settle of a sitting character
// never reads as a stall; it re-anchors every accepted window, so a
// long run keeps measuring rolling progress, while a walk transport
// that silently drops the clicks (H-006) or a run pinned against the
// terrain names itself inside one window. The caller logs the session
// out; clearEscapeAnchor ends the observation with the flee episode.
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

// noteEscapeWalk starts the no-progress window of an issued escape
// segment: the first walk of the episode anchors the window at the
// spot it was issued from, later walks leave the running window
// alone (its own anchor measures the progress).
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

// clearEscapeAnchor ends the no-progress observation with the flee
// episode: the next escape starts a fresh window.
func (l *Loop) clearEscapeAnchor() {
    l.escapeAnchorAt = time.Time{}
    l.escapeAnchorX, l.escapeAnchorY = 0, 0
    l.escapeWalkAt = time.Time{}
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
// paced escape segment away from the threats. The logout fires only
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
// (or lapsed its budget) away from the pack. One last escape segment
// keeps the character moving through the combat window the server
// holds an offline character in the world (fifteen seconds), then
// the session logs out and the supervisor reconnects after the
// armed login cooldown - the aggro resets on the disappearance,
// the mobs walk home, and the character regenerates sitting. The
// logout also counts against the zone it fires in: a ground that
// keeps piling mobs on the character is as hostile as one that keeps
// killing it, and without the count the relogin loop can burn hours
// (the observed run: 85 emergency logouts on the same spider ground,
// every session a fresh loop instance with a zeroed regression).
func (l *Loop) emergencyLogout() {
    reason := fmt.Sprintf("HP %.0f%% under attack",
        l.tracker.SelfHealthPercent())
    if count := l.tracker.SelfAttackerCount(); count >= panicLogoutAttackers {
        reason = fmt.Sprintf("%d mobs piled on us", count)
    }
    l.noteZoneDanger()
    l.emergencyLogoutWithReason(reason)
}

// emergencyLogoutWithReason is the shared body of the emergency
// logout: the reason line names what drove the session out, the
// escape segment keeps the character moving through the logout combat
// window and the login cooldown paces the supervisor reconnect.
// The stagnation recovery calls it with its own honest reason (a
// livelocked loop is as good a reason to rebuild the session as a
// death risk - see stagnation.go).
func (l *Loop) emergencyLogoutWithReason(reason string) {
    l.logoutDone = true
    l.logf("Hunt: %s, emergency logout for %s",
        reason, panicLogoutPause)
    // The honest trail: the structured logout event and the tracker
    // reason replace the "use of closed network connection" line the
    // supervisor would book for the socket close (see runBotForever).
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

// targetSkipped reports whether the object id is currently held out of
// the engage target search: a stuck pick keeps its short delay, a
// fled target its long one (both live in the same expiry map).
func (l *Loop) targetSkipped(objectID int32, now time.Time) bool {
    until, ok := l.targetSkip[objectID]

    return ok && now.Before(until)
}

// attackerEngageable reports whether the character may answer the
// attacker by fighting it: the mob level sits inside the engage
// ceiling (the character level plus the slack) and the character is
// healthy enough to press a fight - the winnable half of the aggro
// answer. An unresolved template (level 0 or unknown) passes like
// every level filter of the target search; an attacker above the
// ceiling never becomes winnable and the defense flow answers
// instead (the escape walk and, when the chase holds, the logout).
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

// pileUpWinnable reports whether a social pile up answers with a
// fight instead of the panic run: the character is healthy enough to
// press the fight, every attacker sits inside the engage level
// ceiling and the pack is small enough to tank. The tank answers the
// reported two mob softlock - the old unconditional panic run,
// relogin and immediate walk home restarted the same aggro cycle
// with full aggro on both mobs, while a healthy character that tanks
// the pair down (one mob dies, the loot runs, the aggro answer picks
// the survivor) keeps the experience and the drops. The health gate
// makes the verdict dynamic: the fight that goes bad re-arms the
// panic run on the next tick, the kill that thins the pack to one
// attacker hands the answer to the ordinary aggro flow.
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
