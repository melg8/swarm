// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The movement phases of the hunt loop: closing on far packs, the idle patrol
// toward the zone center, the geodata walk home and the targetless diagnostic
// that explains a standing hunter in the log.

import (
    "fmt"
    "math"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// walkToFarTarget walks a targetless hunter toward the nearest valid mob of
// the zone when the pack sits outside the engage radius. The walk starts on
// the first targetless tick, one paced segment at a time - the per second
// target search picks up any mob the segment comes past, so a finished loot
// hands the next move over without a standstill. The cell mode drops the fence
// (pickZone): the segment walks toward the nearest VISIBLE enemy wherever it
// stands. Reports whether the tick was handled; without a far target the caller
// falls back to the center patrol.
func (l *Loop) walkToFarTarget(now time.Time) bool {
    if l.noTargetSince.IsZero() {
        l.noTargetSince = now
    }
    if now.Sub(l.lastActionAt) < selectPeriod {
        return true
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    pick, found := l.tracker.NearestAttackablePreferredWindowed(
        farTargetRange, l.pickZone(), l.activeSkips(now),
        l.minTargetLevel(), l.maxTargetLevel(), true,
        l.zoneMobPriority, l.avoidBowTargets())
    if !found {
        // The far search scanned everything the character sees: no pickable
        // mob at any distance. Explain the standing hunter in the log instead
        // of letting a fenced social pack look like a broken bot.
        l.logNoPickableTargets(now)

        return false
    }
    dist := math.Hypot(float64(pick.X-selfX), float64(pick.Y-selfY))
    if dist <= attackNearestRange {
        // Inside the engage radius already: the per second pick takes
        // it from here.
        return false
    }
    l.lastActionAt = now
    moveX, moveY, _ := walkClickSegment(selfX, selfY, selfZ,
        float64(pick.X), float64(pick.Y), float64(selfZ),
        returnWalkSegment)
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logf("Hunt: far target walk failed: %v", err)
    }

    return true
}

// approachPick starts the walk toward a fresh pick at once: the server chase
// only starts with the repeated forced attack, a flood window after the
// selection, so the handover from a corpse would stand the character for that
// second. MoveToLocation is outside the flood window; the click stops at the
// approach stop of the weapon so a bow keeps its range.
func (l *Loop) approachPick(now time.Time, pick state.AttackTarget) {
    if l.lureArmed() || now.Before(l.combatAvoidUntil) {
        return
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    dist := math.Hypot(float64(pick.X-selfX), float64(pick.Y-selfY))
    stop := l.pickApproachStopFor()
    if dist <= stop {
        return
    }
    moveX, moveY, _ := walkClickSegment(selfX, selfY, selfZ,
        float64(pick.X), float64(pick.Y), float64(selfZ),
        math.Min(returnWalkSegment, dist-stop))
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logf("Hunt: approach walk failed: %v", err)
    }
}

// logNoPickableTargets logs the targetless diagnostic: the nearest mobs around
// the character, their positions and the reasons the target search rejects
// them (a social clan pack, the level ceiling, the zone square, the skip list).
// The pacing keeps it to one line per noPickLogPeriod.
func (l *Loop) logNoPickableTargets(now time.Time) {
    if !l.noPickLogAt.IsZero() && now.Sub(l.noPickLogAt) < noPickLogPeriod {
        return
    }
    l.noPickLogAt = now
    blocked := l.tracker.NearestBlockedTargetsWindowed(
        l.pickZone(), l.minTargetLevel(), l.maxTargetLevel(),
        l.activeSkips(now), noPickLogLimit, l.avoidBowTargets())
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

// logWeaponWait logs the bare-handed hold of the engage gate: the character
// owns no weapon while its plan offers an affordable one, so the fresh picks
// hold until the weapon run buys it. One line per noPickLogPeriod.
func (l *Loop) logWeaponWait(now time.Time) {
    if !l.weaponWaitLogAt.IsZero() &&
        now.Sub(l.weaponWaitLogAt) < noPickLogPeriod {
        return
    }
    l.weaponWaitLogAt = now
    l.logger.Printf("Hunt: no weapon in hand, holding the target " +
        "picks until the weapon run buys one")
}

// patrolToCenter walks a targetless hunter toward the zone center: the pack
// moved on or the social fence keeps the camps out of reach. The walk starts on
// the first targetless tick, one paced segment at a time, so the per second
// target search picks up any mob the segment comes past instead of marching to
// the center first.
func (l *Loop) patrolToCenter(now time.Time) {
    zone := l.zone()
    if zone == nil {
        return
    }
    if l.noTargetSince.IsZero() {
        l.noTargetSince = now
    }
    if now.Sub(l.lastActionAt) < selectPeriod {
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
    l.lastActionAt = now
    l.walkZoneSegment(zone, selfX, selfY, selfZ, now)
}

// adoptOutZoneFight keeps the fight that crossed the hunting zone line running:
// the mob that dragged the character out of the square (a chase, an aggressive
// pull) is finished where it stands before the walk home. The adoption covers
// the own living target, the fresh server side selection and the mob that holds
// the character as its target; a target the flee flow held out stays out.
// Reports whether a fight is live; without one the leash walks home.
func (l *Loop) adoptOutZoneFight(now time.Time) bool {
    if l.target != 0 && l.tracker.ObjectAlive(l.target) {
        return true
    }
    // The road budget: the aggressive road feeds a fresh attacker every respawn
    // window, so adopting each one holds the character on the road forever.
    // Past the budget no new fight starts (roadFightBudget).
    if l.roadFights >= roadFightBudget {
        return false
    }
    serverTarget := l.tracker.SelfTargetID()
    if serverTarget != 0 && l.tracker.ObjectAlive(serverTarget) &&
        !l.targetSkipped(serverTarget, now) {
        l.target = serverTarget
        l.engageAt = now
        l.clearBlindRecovery()
        l.roadFights++

        return true
    }
    if l.tracker.SelfUnderAttack() {
        if l.kiteFlightHolds(now) {
            // The chase that dragged the character over the zone line is one
            // the flight already ran from: the away ray keeps the answer until
            // the server clears the aggro (kiteFlightHolds).
            l.fleeFromThreat(now)

            return true
        }
        if pick, ok := l.tracker.NearestAttacker(); ok &&
            !l.targetSkipped(pick.ObjectID, now) {
            if !l.attackerEngageable(pick.ObjectID) {
                // The chase is too strong to answer with a fight (the attacker
                // sits above the level ceiling, the character is hurt): the
                // defensive escape beats walking home through the blows.
                l.fleeFromThreat(now)

                return true
            }
            l.logger.Printf("Hunt: %s (%d) keeps attacking outside "+
                "the zone, finishing it", pick.Name, pick.ObjectID)
            l.target = pick.ObjectID
            l.engageAt = now
            l.clearBlindRecovery()
            l.roadFights++

            return true
        }
    }

    return false
}

// holdZoneReturn parks the budget-burned return: the walk holds instead of
// marching the direct segments toward the zone (only the planned routes are
// walked). One fresh planning cycle arms per backoff window.
func (l *Loop) holdZoneReturn(now time.Time) {
    if now.Sub(l.zoneSegmentLogAt) >= noPickLogPeriod {
        l.zoneSegmentLogAt = now
        l.logf("Hunt: %d zone returns found no walkable route, "+
            "holding the return instead of the direct segments",
            l.zoneFails)
        l.zoneFails = 0
    }
}

// returnToZone walks the character back into the hunting square over the
// geodata waypoints: a village respawn after death or a deleveling guard post
// sits behind the village walls, so the return is planned with the pathfinder
// and followed by the town trip waypoint machinery (phaseTownReturn). The
// remembered farm spot is the destination when one exists, the zone center
// otherwise. A return whose budget burned or whose planning failed holds the
// walk; only deployments without a route planner keep the direct segments.
func (l *Loop) returnToZone() {
    now := l.now()
    if !l.standUpGuarded(now) {
        return
    }
    if now.Sub(l.lastActionAt) < selectPeriod {
        return
    }
    l.lastActionAt = now
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
        // The previous pathfound return segment ended without reaching
        // the zone (a stuck walk aborts the segment): count the failure
        // and stop planning past the budget.
        l.zoneFails++
    }
    if !l.zoneReturn {
        l.zoneReturn = true
        l.logf("Hunt: outside the hunting zone, pathfinding back")
    }
    if (l.navigator == nil) || l.zoneFails >= zoneReturnFailBudget {
        l.phase = phaseEngage
        if l.navigator == nil {
            // No geodata: the direct short segments are the whole return
            // machinery there is.
            l.walkZoneSegment(zone, selfX, selfY, selfZ, now)

            return
        }
        l.holdZoneReturn(now)

        return
    }
    dest := l.zoneReturnGoal(zone, selfZ)
    l.tripStart = l.now()
    l.rePaths = 0
    l.phase = phaseTownReturn
    l.segmentRadius = tripApproachRadius
    if !l.startZoneReturnSegment(dest) {
        // The planner owns no route to the zone at all: the return holds
        // instead of marching the direct segments. The paced line explains the
        // standing hunter, the next returnToZone re-plans.
        l.phase = phaseEngage
        l.zoneFails++
        if now.Sub(l.zoneSegmentLogAt) >= noPickLogPeriod {
            l.zoneSegmentLogAt = now
            l.logf("Hunt: no route to %d %d, holding the zone return "+
                "instead of the direct segments", int(dest.X), int(dest.Y))
        }

        return
    }
}

// resolveDestinationDeck resolves the deck the destination x/y sits on through
// the navigator (the closest layer to the reference z). A destination z
// remembered on another deck misses the mesh search's nearest window, so the
// deck under the x/y is the honest search goal; a lookup failure keeps the z.
func (l *Loop) resolveDestinationDeck(x, y, z int32) int32 {
    if l.navigator == nil {
        return z
    }
    if height, err := l.navigator.ClosestHeight(
        float64(x), float64(y), int16(z)); err == nil {
        return int32(height)
    }

    return z
}

// zoneReturnGoal builds the search goal of the zone return: the remembered
// farm spot when it lies inside the zone (the walk home returns to the ground
// the hunt left), the square center's resolved deck position otherwise. The
// spot z resolves onto its own deck first (resolveDestinationDeck).
func (l *Loop) zoneReturnGoal(
    zone *state.Zone, selfZ int32,
) pathfind.Vec3 {
    if l.farmX != 0 || l.farmY != 0 {
        if zone.Contains(l.farmX, l.farmY) {
            return pathfind.Vec3{
                X: float64(l.farmX),
                Y: float64(l.farmY),
                Z: float64(l.resolveDestinationDeck(
                    l.farmX, l.farmY, l.farmZ)),
            }
        }
    }

    return l.zoneReturnDestination(zone, selfZ)
}

// zoneReturnDestination builds the search goal for the square center: the x
// and y of the center with the real deck height under it, resolved the way the
// server resolves a destination. A fabricated self height at the target put the
// 3D approach goal mid air whenever the zone sat on another deck (the floating
// elven city at z -2992 against a ground zone at z -3664), and the search
// burned the whole expansion cap. A lookup failure keeps the self height.
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

// walkZoneSegment walks one direct short segment toward the zone center: the
// pacing segment of the in-zone targetless patrol and the whole return
// machinery of the deployments without a route planner. The segment length
// respects the server move request limit (9900 units) and the walk rate limits
// itself through the select pacing. The aggro-aware steering bends it around
// the idle camps on its line (loop_avoid.go); the click guard and the stall
// watch run first, so a refused or frozen segment re-arms the pathfound return.
func (l *Loop) walkZoneSegment(
    zone *state.Zone, selfX int32, selfY int32, selfZ int32, now time.Time,
) {
    // The move start fast path of the direct zone segments: a click whose
    // movement never started backdates the stall window, so the dead click
    // re-discovers itself in seconds instead of the full stuck timeout.
    if !l.moveAt.IsZero() && l.moveAt.After(l.zoneSegmentAt) &&
        selfX == l.zoneSegmentX && selfY == l.zoneSegmentY &&
        !l.tracker.SelfWalking() &&
        now.Sub(l.moveAt) >= moveStartWindow {
        l.zoneSegmentAt = now.Add(-stuckTimeout)
    }
    if l.noteZoneSegmentStall(now, selfX, selfY) {
        return
    }
    clickX, clickY, _ := walkClickSegment(selfX, selfY, selfZ,
        float64(zone.CX), float64(zone.CY), float64(selfZ),
        returnWalkSegment)
    moveX, moveY := int32(clickX), int32(clickY)
    if ax, ay, dodged := l.steerClearOfAggro(
        selfX, selfY, selfZ, moveX, moveY, selfZ, zone.CX, zone.CY,
        now); dodged {
        moveX, moveY = ax, ay
    }
    if !l.guardZoneSegmentClick(selfX, selfY, selfZ, moveX, moveY) {
        return
    }
    // The click timestamp lands in the shared moveAt slot so the refusal
    // evidence correlates the zone segments with the ActionFailed answers the
    // same way it correlates the town walk clicks.
    l.moveAt = now
    if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
        l.logf("Hunt: walk back failed: %v", err)
    }
}

// noteZoneSegmentStall watches the direct zone segments for the freeze the
// offline click guard cannot see: a wall the geodata pack does not model
// passes ValidateClick, the server silently cancels the click and the
// character never moves a cell. The window baselines on the first segment,
// re-baselines on every cell change and fires when the position holds past the
// stuck timeout; the stall re-arms the pathfound zone return, whose frozen
// corridor ban widens.
func (l *Loop) noteZoneSegmentStall(
    now time.Time, selfX int32, selfY int32,
) bool {
    if l.navigator == nil {
        // No geodata: the direct segments are the whole return machinery,
        // the pathfound re-arm the stall arms has nothing to plan.
        return false
    }
    if l.zoneSegmentAt.IsZero() || selfX != l.zoneSegmentX ||
        selfY != l.zoneSegmentY {
        l.zoneSegmentAt, l.zoneSegmentX, l.zoneSegmentY = now, selfX, selfY

        return false
    }
    held := now.Sub(l.zoneSegmentAt)
    if held < stuckTimeout {
        return false
    }
    // The refusal answer separates the two stall families: without it the
    // segments froze mid corridor (the fresh geodata route widens the bans);
    // with it the server refused the click, and a character that progressed
    // re-arms the pathfound return while one on the same cell keeps the
    // backoff.
    if l.refusalEvidence() {
        l.zoneSegmentAt = time.Time{}
        if selfX == l.zoneRefusalX && selfY == l.zoneRefusalY {
            l.zoneFails = zoneReturnFailBudget
            l.logf("Hunt: the direct zone segments moved nothing for %s "+
                "at %d %d and the server refused the clicks, "+
                "holding the return backoff",
                held.Round(time.Second), selfX, selfY)

            return true
        }
        l.zoneRefusalX, l.zoneRefusalY = selfX, selfY
        l.zoneFails = 0
        l.logf("Hunt: the direct zone segments moved nothing for %s at "+
            "%d %d and the server refused the clicks, re-arming "+
            "the pathfound return",
            held.Round(time.Second), selfX, selfY)

        return true
    }
    l.zoneSegmentAt = time.Time{}
    l.logf("Hunt: the direct zone segments moved nothing for %s at %d %d, "+
        "re-arming the pathfound return",
        held.Round(time.Second), selfX, selfY)
    l.zoneFails = 0

    return true
}

// guardZoneSegmentClick validates one direct zone segment through the server
// click port (Navigator.ValidateClick) and reports whether the request may be
// sent: a click the server would cancel never moves the character (the geodata
// correction collapses its target onto the walker cell), so the refusal
// re-arms the pathfound zone return and the paced log line explains the
// standing hunter. Without a navigator the click goes out unvalidated.
func (l *Loop) guardZoneSegmentClick(
    selfX int32, selfY int32, selfZ int32, moveX int32, moveY int32,
) bool {
    if l.navigator == nil {
        return true
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    to := pathfind.Vec3{
        X: float64(moveX), Y: float64(moveY), Z: float64(selfZ),
    }
    if _, ok := l.navigator.ValidateClick(from, to); ok {
        return true
    }
    if l.zoneFails >= zoneReturnFailBudget {
        l.zoneFails = 0
    }
    now := l.now()
    if now.Sub(l.zoneSegmentLogAt) >= noPickLogPeriod {
        l.zoneSegmentLogAt = now
        l.logf("Hunt: the direct zone segment to %d %d is walled, "+
            "re-arming the pathfound return", moveX, moveY)
    }

    return false
}
