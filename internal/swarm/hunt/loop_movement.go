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
// segment at a time - the per second target search of the engage
// picks up any mob the segment comes past, so the character engages
// the moment something valid enters the radius. The cell mode
// drops the fence entirely (see pickZone): the segment walks toward
// the nearest VISIBLE enemy wherever it stands - toward the zone
// that holds the enemies, never into an enemy-less one. Reports
// whether the tick was handled (a far target exists); without one
// the caller falls back to the center patrol.
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
    pick, found := l.tracker.NearestAttackablePreferredWindowed(
        farTargetRange, l.pickZone(), l.activeSkips(now),
        l.minTargetLevel(), l.maxTargetLevel(), true,
        l.zoneMobPriority)
    if !found {
        // The far search scanned everything the character sees: no
        // pickable mob at any distance. Explain the standing hunter
        // in the log - the mobs the character sees, their positions
        // and why the target search rejects them - instead of letting
        // a fenced social pack look like a broken bot.
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
    if dist > returnWalkSegment {
        frac := returnWalkSegment / dist
        moveX = int32(float64(selfX) + dx*frac)
        moveY = int32(float64(selfY) + dy*frac)
    }
    if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
        l.logf("Hunt: far target walk failed: %v", err)
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
    blocked := l.tracker.NearestBlockedTargetsWindowed(
        l.pickZone(), l.minTargetLevel(), l.maxTargetLevel(),
        l.activeSkips(now), noPickLogLimit)
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

// logWeaponWait logs the bare-handed hold of the engage gate: the
// character owns no weapon while its plan offers an affordable one,
// so the fresh picks hold until the weapon run trip buys it. One line
// per noPickLogPeriod while the hold lasts; a landed weapon ends the
// hold and the log with it.
func (l *Loop) logWeaponWait(now time.Time) {
    if !l.weaponWaitLogAt.IsZero() &&
        now.Sub(l.weaponWaitLogAt) < noPickLogPeriod {
        return
    }
    l.weaponWaitLogAt = now
    l.logger.Printf("Hunt: no weapon in hand, holding the target " +
        "picks until the weapon run buys one")
}

// patrolToCenter walks a targetless hunter toward the zone center:
// the pack moved on or the social fence keeps the camps out of
// reach, and standing still waits for luck. One paced segment at a
// time, so the per second target search of the engage picks up
// any mob the segment comes past - the character engages the moment
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
    l.walkZoneSegment(zone, selfX, selfY, selfZ, now)
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
    // The road budget: the aggressive territory on the walk home
    // feeds a fresh attacker every respawn window - adopting each
    // one holds the character on the road forever (the 2026-09-11
    // 08:04 parallel round: the farm segment timed out on the road
    // fights, the walk home never resumed). Past the budget no new
    // fight starts: the walk home continues through the blows, the
    // flee flow above owns the hurt case and the mobs leash back
    // once the character leaves the aggro radius. The budget resets
    // on the zone entry.
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
        if pick, ok := l.tracker.NearestAttacker(); ok &&
            !l.targetSkipped(pick.ObjectID, now) {
            if !l.attackerEngageable(pick.ObjectID) {
                // The chase is too strong to answer with a fight (the
                // attacker sits above the level ceiling, the character
                // is hurt): the defensive escape - the standard run
                // that logs out when the chase never shakes - beats
                // both walking home through the blows and pressing a
                // losing fight.
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

// holdZoneReturn parks the budget-burned return: the walk holds
// instead of marching the direct segments toward the zone (the owner rule
// of the 2026-09-19 round: ONLY walk the planned routes and NEVER
// walk the direct line). One fresh planning cycle arms per backoff
// window - the
// counter resets with the paced log line, so the next returnToZone
// retries the planner while the session's frozen corridor bans
// (widened by every failed trip) keep reshaping the routes it may
// answer.
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
// geodata waypoints: a village respawn after death or a deleveling guard
// post sits behind the village walls, and a direct walk bumps into them,
// so the return is planned with the pathfinder and followed by the town
// trip waypoint machinery (segment splitting, passed waypoint skipping, stuck
// re-pathing) through phaseTownReturn. The remembered farm spot is the
// destination when one exists, the zone center otherwise. A return whose
// budget burned or whose planning failed HOLDS the walk (the paced log
// names it): the direct segments toward the zone are the march the owner
// forbade - only the deployments without a route planner keep them (the
// whole return machinery there is). The return needs a standing
// character: the server refuses every move request while it sits (the
// AI stays on the REST intention and answers ActionFailed), so a zone
// switch that lands on a resting character first waits out the sit
// transition, stands up and only then plans the walk.
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
            // machinery there is (the legacy deployment without a
            // route planner - nothing exists to follow instead).
            l.walkZoneSegment(zone, selfX, selfY, selfZ, now)

            return
        }
        l.holdZoneReturn(now)

        return
    }
    dest := l.zoneReturnGoal(zone, selfZ)
    l.tripStart = time.Now()
    l.rePaths = 0
    l.phase = phaseTownReturn
    l.segmentRadius = tripApproachRadius
    if !l.startZoneReturnSegment(dest) {
        // The planner owns no route to the zone at all (both searches
        // failed): the return holds instead of marching the direct
        // segments toward the zone (the owner rule of the 2026-09-19
        // round: NEVER walk the direct line). The paced line explains
        // the standing hunter in the state dump; the next returnToZone
        // re-plans (a failed trip's bans may have reshaped the answer
        // by then).
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

// resolveDestinationDeck resolves the deck the destination x/y sits
// on through the navigator (the closest layer to the reference z -
// the same semantics the zone return center applies). A destination
// z remembered on another deck (the standing z of the village riding
// a farm zone x/y) misses the mesh search's nearest window by
// hundreds of units and the search answers the honest "no navmesh
// under the position" forever - the deck under the x/y is the honest
// search goal. A lookup failure keeps the reference z: the same-deck
// case it answers correctly.
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

// zoneReturnGoal builds the search goal of the zone return: the
// remembered farm spot when it lies inside the zone (the walk home
// returns to the ground the hunt left), the square center's resolved
// deck position otherwise (see zoneReturnDestination). The remembered
// spot z resolves onto the spot's own deck first (see
// resolveDestinationDeck): a spot remembered as the zone center x/y
// with the standing z of another deck would strand the return search
// on the mesh's nearest window.
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

// walkZoneSegment walks one direct short segment toward the zone center: the
// pacing segment of the in-zone targetless patrol and the whole return
// machinery of the deployments without a route planner (the owner
// rule of the 2026-09-19 round keeps it out of the pathfound return:
// a session with a navigator walks ROUTES only, the direct segments are
// gone from its ladder). The segment length respects the server move
// request limit (9900 units) and the walk rate limits itself through
// the select pacing. The aggro-aware steering bends the segment around
// the idle aggressive camps sitting on its line (see loop_avoid.go) -
// the mobs at the zone center itself stay exempt: the ground the
// patrol deliberately enters carries its own prey. The click guard
// runs before the request: a segment the server would cancel never moves
// the character, so a refused segment re-arms the pathfound return
// instead of grinding refused clicks forever (see guardZoneSegmentClick).
// The no-movement stall of noteZoneSegmentStall runs first: segments the
// offline guard blessed but the server silently refuses (a wall the
// geodata pack does not model) grind nothing forever without it - the
// terminal freeze of the 2026-09-14 08:42 dump, whose bot stood at
// the village terrace clicking the collapsed southwest segment once a
// second with no stuck window, no abort and no log line left to see.
func (l *Loop) walkZoneSegment(
    zone *state.Zone, selfX int32, selfY int32, selfZ int32, now time.Time,
) {
    // The move start fast path of the direct zone segments: a segment click
    // sent after the stall baseline whose movement never started (no
    // broadcast, no position change) backdates the stall window so
    // noteZoneSegmentStall fires this tick - the dead click re-discovers
    // itself in seconds instead of standing out the full stuck
    // timeout (the owner rule of the 2026-09-19 round).
    if !l.moveAt.IsZero() && l.moveAt.After(l.zoneSegmentAt) &&
        selfX == l.zoneSegmentX && selfY == l.zoneSegmentY &&
        !l.tracker.SelfWalking() &&
        now.Sub(l.moveAt) >= moveStartWindow {
        l.zoneSegmentAt = now.Add(-stuckTimeout)
    }
    if l.noteZoneSegmentStall(now, selfX, selfY) {
        return
    }
    moveX, moveY := zone.CX, zone.CY
    dx := float64(zone.CX - selfX)
    dy := float64(zone.CY - selfY)
    if dist := math.Hypot(dx, dy); dist > returnWalkSegment {
        frac := returnWalkSegment / dist
        moveX = int32(float64(selfX) + dx*frac)
        moveY = int32(float64(selfY) + dy*frac)
    }
    if ax, ay, dodged := l.steerClearOfAggro(
        selfX, selfY, selfZ, moveX, moveY, selfZ, zone.CX, zone.CY,
        now); dodged {
        moveX, moveY = ax, ay
    }
    if !l.guardZoneSegmentClick(selfX, selfY, selfZ, moveX, moveY) {
        return
    }
    // The click timestamp lands in the shared moveAt slot so the
    // online refusal evidence correlates the zone segments with the
    // ActionFailed answers the same way it correlates the town walk
    // clicks (see refusalEvidence): without it the segments bypass the
    // channel and a server that refuses them reads as a plain
    // freeze.
    l.moveAt = now
    if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
        l.logf("Hunt: walk back failed: %v", err)
    }
}

// noteZoneSegmentStall watches the direct zone segments for the freeze the
// offline click guard cannot see: a server wall the geodata pack does
// not model passes the ValidateClick port, the click goes out and the
// server silently cancels it - the character never moves a cell while
// the segments keep going out once a second (the terminal state of the
// 2026-09-14 08:42 dump: after three aborted zone return trips the
// budget-gated direct segments ground against the village railing forever
// with no detector left). The window baselines on the first segment send,
// re-baselines on every cell change and fires when the position holds
// past the stuck timeout: the stall re-arms the pathfound zone return
// (the zone return fail budget clears, the same recovery the offline
// refusal of guardZoneSegmentClick arms) so the next tick plans a fresh
// geodata route - whose frozen corridor ban the ladder keeps widening
// (see banFrozenCorridor), so every stall window buys a different
// route instead of reproducing the identical frozen one. It reports
// whether the stall fired (the caller holds the click of this tick).
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
    // The refusal answer separates the two stall families: without
    // it the segments froze mid corridor (the re-arm below answers -
    // the fresh geodata route widens the bans). With it the server
    // refused the click itself, and the answer splits once more by
    // the ground covered since the last refusal stall: a character
    // that PROGRESSED (the server accepts some clicks - the
    // partially refusing deployment) re-arms the pathfound return
    // whose follower varies its aims through the refusals, while a
    // character that stood on the very same cell holds the return
    // backoff - the re-arm would only restart the cycle the server
    // keeps refusing (the 2026-09-14 10:18 dump: the stall re-armed
    // the pathfound return every 15 s for seven minutes on a server
    // that refused every click).
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
// click port (Navigator.ValidateClick) and reports whether the request
// may be sent. A click the server would cancel never moves the
// character - the geodata correction collapses its target onto the
// walker cell (the village walls, the deck edges), the answer is a
// silent ActionFailed and the character freezes - so the escalation
// to the direct segments must not send it: the refusal re-arms the
// pathfound zone return instead (zoneFails back under the escalation
// budget, the next returnToZone plans a fresh geodata route whose
// first click the offline probe validates before it leaves) and the
// paced log line explains the standing hunter in the state dump (the
// 2026-09-12 01:50 report: the post-abort escalation clicked the same
// collapsed southwest segment of the elven village street for over an
// hour without a single event or a single cell of movement). Without
// a navigator there is nothing to validate with: the legacy behavior
// stands and the click goes out as it always did.
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
    now := time.Now()
    if now.Sub(l.zoneSegmentLogAt) >= noPickLogPeriod {
        l.zoneSegmentLogAt = now
        l.logf("Hunt: the direct zone segment to %d %d is walled, "+
            "re-arming the pathfound return", moveX, moveY)
    }

    return false
}
