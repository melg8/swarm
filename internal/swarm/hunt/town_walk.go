// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// waypointDistanceAnchored measures the character to waypoint distance
// with the waypoint height anchored into the server frame
// (click_frame.go): the vintage shift explains only the pack
// disagreement, a whole deck below still measures its true gap.
func waypointDistanceAnchored(
    wp pathfind.Vec3, selfX, selfY, selfZ int32, frameOffset float64,
) float64 {
    dx := wp.X - float64(selfX)
    dy := wp.Y - float64(selfY)
    dz := anchorZToServerFrame(wp.Z, frameOffset) - float64(selfZ)

    return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// waypointArrived reports whether the follower counts the waypoint as
// reached: the 50 unit pass radius for the intermediate turns (a wide
// radius cuts corners into railings), the final arrival radius of the
// segment for the last one, both anchored into the server frame.
func waypointArrived(
    waypoints []pathfind.Vec3, index int, selfX, selfY, selfZ int32,
    frameOffset, finalArrive float64,
) bool {
    radius := waypointPassDist
    if index == len(waypoints)-1 {
        radius = finalArrive
    }

    return waypointDistanceAnchored(waypoints[index], selfX, selfY,
        selfZ, frameOffset) <= radius
}

// finalArriveRadius answers the arrival radius of the final waypoint:
// the tight pass radius for the exact segments (the merchant customer
// spot) and the close npc ring segments (the teacher hall rows), the
// wide trip slack otherwise.
func (l *Loop) finalArriveRadius() float64 {
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        return waypointPassDist
    }
    if l.segmentRadius > 0 && l.segmentRadius < tripApproachRadius {
        return waypointPassDist
    }

    return waypointArriveDist
}

// waypointPassed reports whether the character moved past the waypoint
// along the route: the projection beyond it and inside the 100 unit
// corridor. A raw "next waypoint is closer" test skipped bridge entries
// through the railing; the planar test never passes a vertical drop.
func waypointPassed(
    wp, next pathfind.Vec3, selfX, selfY int32,
) bool {
    segX := next.X - wp.X
    segY := next.Y - wp.Y
    segLen := math.Hypot(segX, segY)
    if segLen < 1 {
        // A vertical drop segment: no planar pass geometry.
        return false
    }
    selfDX := float64(selfX) - wp.X
    selfDY := float64(selfY) - wp.Y
    along := (selfDX*segX + selfDY*segY) / segLen
    if along <= 0 {
        // Still before the waypoint: nothing passed yet.
        return false
    }
    lateral := math.Abs(selfDX*segY-selfDY*segX) / segLen

    return lateral <= waypointCorridor
}

// waypointPassedAlongRoute reports whether the character already rides
// the route at or beyond the aimed waypoint: past it on its own segment
// or inside the corridor of ANY later segment, so a pinned cursor never
// walks a covered bend back. A character inside the arrival radius
// never counts - the caller gates on it.
func waypointPassedAlongRoute(
    waypoints []pathfind.Vec3, index int, selfX, selfY int32,
) bool {
    for j := index; j+1 < len(waypoints); j++ {
        wp := waypoints[j]
        next := waypoints[j+1]
        segX := next.X - wp.X
        segY := next.Y - wp.Y
        segLen := math.Hypot(segX, segY)
        if segLen < 1 {
            continue
        }
        selfDX := float64(selfX) - wp.X
        selfDY := float64(selfY) - wp.Y
        along := (selfDX*segX + selfDY*segY) / segLen
        if along <= 0 {
            continue
        }
        lateral := math.Abs(selfDX*segY-selfDY*segX) / segLen
        if lateral > waypointCorridor {
            continue
        }
        if j > index || along <= segLen {
            // On a later segment (any point of it lies past the
            // aimed waypoint along the walk) or past the aimed
            // waypoint on its own segment.
            return true
        }
    }

    return false
}

// walkTownWaypoints follows the planned waypoints with ground clicks
// and reports the final arrival. Long segments split at the move limit,
// a waypoint passed ON THE ROUTE skips (walking back would loop), and a
// standing walk re-paths within the trip budget. The plan prices the
// water, so the wet segments walk like the dry ones.
func (l *Loop) walkTownWaypoints() bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    // The cursor key escape owns the segment while it runs: the claim
    // ladder drives the character (the escape arms from the follower
    // recovery too - the refusing pocket branch of stuckTownWalk),
    // the click machinery stays down until the escape ends.
    if l.cursorEscape.armed {
        l.driveCursorKeyEscape(l.now(), selfX, selfY)

        return false
    }

    return l.followWaypoints(selfX, selfY, selfZ, l.now())
}

// advanceWaypoints walks the cursor forward: a waypoint counts as
// passed when reached or bypassed along the route AND the line from the
// standing cell to the successor is walkable. A gated waypoint stays
// the target until walking onto it re-opens the line.
func (l *Loop) advanceWaypoints(selfX, selfY, selfZ int32) {
    next, moved := advanceWalkCursor(l.waypoints, l.wpIndex,
        selfX, selfY, selfZ, l.segmentFrameOffset, l.finalArriveRadius(),
        func(next int) bool {
            return l.segmentAdvanceClear(selfX, selfY, selfZ, next)
        })
    l.wpIndex = next
    if moved {
        l.moveAt = time.Time{}
    }
}

// followWaypoints is the shared follower core: the arrival radii, the
// passed-waypoint skip, the stuck tracking and the click pace. The
// re-plan ladder owns the off-plan swims instead of a click guard.
func (l *Loop) followWaypoints(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    l.advanceWaypoints(selfX, selfY, selfZ)
    if l.wpIndex >= len(l.waypoints) {
        return true
    }
    // The move start watchdog: a click whose movement never started
    // forces the stuck verdict below - the recovery ladder runs at
    // once instead of standing out the full window on a dead click.
    if l.noteMoveStart(now, selfX, selfY) {
        l.forceStuck = true
    }
    if l.walkStuck(now, selfX, selfY) {
        return false
    }
    // The stuck handling may have re-planned the segment: re-run the
    // cursor advance so the click aims the fresh plan's first real
    // waypoint, never the self-click of its wp 0.
    if l.cursorEscape.armed {
        return false
    }
    l.advanceWaypoints(selfX, selfY, selfZ)
    if l.wpIndex >= len(l.waypoints) {
        return true
    }
    // The corner pre-click replaces the destination just before the
    // character reaches the current one (see flowPreClick): the
    // server keeps the move running instead of stopping the character
    // on the arrival of a dense route point.
    if l.flowPreClick(selfX, selfY, selfZ, now) {
        return false
    }
    if !l.moveAt.IsZero() && now.Sub(l.moveAt) < walkRequestPeriod {
        return false
    }
    // The goal span of the click: the farthest waypoint the server
    // delivers inside the aim window (flowAdvance).
    l.wpIndex = l.flowAdvance(l.waypoints, l.wpIndex,
        l.segmentFrameOffset, selfX, selfY, selfZ)
    l.clickWaypoint(selfX, selfY, selfZ, now)

    return false
}

// waypointPassedBehind reports whether the aimed waypoint sits behind
// the character along the route, so a click onto it walks backward. A
// character inside the arrival radius never counts - the pull back is
// the re-approach the ramp corner needs.
func (l *Loop) waypointPassedBehind(
    wp pathfind.Vec3, selfX, selfY, selfZ int32,
) bool {
    if l.wpIndex+1 >= len(l.waypoints) {
        return false
    }
    arrived := waypointDistanceAnchored(wp, selfX, selfY, selfZ,
        l.segmentFrameOffset) <= waypointPassDist

    return !arrived && waypointPassedAlongRoute(l.waypoints, l.wpIndex,
        selfX, selfY)
}

// clickWaypoint aims the current waypoint, bends the click around the
// idle aggressive camps, guards the line against the server refusal and
// sends it: the hold verdicts, the camp skip, the aggro steering and
// the validation, in that order.
func (l *Loop) clickWaypoint(
    selfX, selfY, selfZ int32, now time.Time,
) {
    wp := l.waypoints[l.wpIndex]
    moveX, moveY, moveZ, dist := l.anchorWaypointClick(
        selfX, selfY, selfZ, wp)
    // The backward walk guard: an aimed waypoint the character already
    // moved past along the route would walk it back off the ground the
    // route samples just covered.
    passedBehind := l.waypointPassedBehind(wp, selfX, selfY, selfZ)
    moveX, moveY, moveZ, send := l.resolveClickExtension(
        selfX, selfY, selfZ, dist, passedBehind, moveX, moveY, moveZ)
    if !send {
        return
    }
    // A waypoint inside an idle mob's trigger circle cannot be reached
    // by any tangent arc (the side flips at every re-issue), so it skips
    // onto a clear successor. The verdict reads the WAYPOINT, never the
    // clipped click target, and the successor must be clickable.
    moveXI, moveYI := int32(math.Round(moveX)), int32(math.Round(moveY))
    moveZI := int32(math.Round(moveZ))
    wpXI, wpYI := int32(math.Round(wp.X)), int32(math.Round(wp.Y))
    if l.segmentTargetThreatened(wpXI, wpYI,
        int32(l.segmentDest.X), int32(l.segmentDest.Y)) {
        if next := l.nextClearWaypoint(selfX, selfY, selfZ); next > l.wpIndex &&
            l.skipMoveFresh(selfX, selfY) {
            l.wpIndex = next
            l.moveAt = time.Time{}
            l.noteSkipStand(selfX, selfY)
            l.logger.Printf("Hunt: the waypoint sits inside an aggro "+
                "circle, skipping ahead (cursor %d of %d)",
                l.wpIndex, len(l.waypoints))

            return
        }
        // No clear successor or a repeat skip: the steered arc answers,
        // the stuck window escalates the segment.
    }
    // The aggro-aware steering: the camps of idle aggressive mobs on
    // the segment bend it sideways (loop_avoid.go); the mobs at the
    // destination stay exempt.
    if ax, ay, dodged := l.steerClearOfAggro(
        selfX, selfY, selfZ, moveXI, moveYI, moveZI,
        int32(l.segmentDest.X), int32(l.segmentDest.Y), now); dodged {
        moveX, moveY = float64(ax), float64(ay)
    }
    // The server click validation runs last: the server refuses
    // whole lines its Bresenham raster walks into walled corners -
    // a refused click never moves the character.
    if l.navigator != nil && !l.clickServerValidated(
        selfX, selfY, selfZ, &moveX, &moveY, &moveZ, now) {
        return
    }
    l.moveAt = now
    if err := l.game.WalkTo(int32(moveX), int32(moveY),
        int32(moveZ)); err != nil {
        l.logf("Hunt: town walk request failed: %v", err)
    }
}

// anchorWaypointClick resolves the click target of the aimed waypoint:
// the z rides the server frame transport (click_frame.go), a far
// waypoint clips to the move limit along the line, and the distance
// comes back for the rescue floor of resolveClickExtension.
func (l *Loop) anchorWaypointClick(
    selfX, selfY, selfZ int32, wp pathfind.Vec3,
) (moveX, moveY, moveZ, dist float64) {
    dx := wp.X - float64(selfX)
    dy := wp.Y - float64(selfY)
    dist = math.Hypot(dx, dy)
    wpZ := anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
    moveX, moveY, moveZ = walkClickSegment(
        selfX, selfY, selfZ, wp.X, wp.Y, wpZ, maxMoveDistance)

    return moveX, moveY, moveZ, dist
}

// resolveClickExtension runs the extension ladder of the aimed waypoint
// click: an under-floor target re-aims at the forward route samples
// (no stuck verdict needed), a backward target of an armed stuck
// segment re-aims the same way, and every other backward click holds -
// the stuck window owns the answer. It returns the click target and
// whether the click may leave the bot; the rescue floor arithmetic and
// the sample march: docs/hunting.md (the short click extension).
func (l *Loop) resolveClickExtension(
    selfX, selfY, selfZ int32,
    dist float64, passedBehind bool,
    moveX, moveY, moveZ float64,
) (float64, float64, float64, bool) {
    switch {
    case dist < minWalkClick:
        extX, extY, extZ, ok := l.extendShortClick(
            selfX, selfY, selfZ, moveX, moveY, moveZ)
        if ok {
            return extX, extY, extZ, true
        }
    case l.extendArmed && (passedBehind || waypointBehindRoute(
        l.waypoints, l.wpIndex, selfX, selfY)):
        extX, extY, extZ, ok := l.extendShortClick(
            selfX, selfY, selfZ,
            moveX, moveY, moveZ)
        if ok {
            return extX, extY, extZ, true
        }

        return moveX, moveY, moveZ, false
    case passedBehind:
        return moveX, moveY, moveZ, false
    }

    return moveX, moveY, moveZ, true
}

// waypointBehindRoute reports whether the waypoint sits behind the
// character relative to the segment leaving it; the extension takes
// such clicks over. Only a waypoint the character stands NEAR counts,
// so a V-shaped detour keeps its far climb targets.
func waypointBehindRoute(
    waypoints []pathfind.Vec3, index int, selfX, selfY int32,
) bool {
    if index+1 >= len(waypoints) {
        return false
    }
    wp := waypoints[index]
    next := waypoints[index+1]
    fdx := next.X - wp.X
    fdy := next.Y - wp.Y
    if math.Hypot(fdx, fdy) < 1 {
        return false
    }
    if math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY)) >
        waypointPassDist {
        // The waypoint sits far away: the character has not passed
        // it, whatever the segment direction says.
        return false
    }

    return (wp.X-float64(selfX))*fdx+(wp.Y-float64(selfY))*fdy < 0
}

// extendShortClick re-aims an under-floor click (minWalkClick) at the
// forward route samples: the first sample the click transport
// validates wins, so a server collapse hands the click to its
// pathfinder. No candidate keeps the plain waypoint click.
func (l *Loop) extendShortClick(
    selfX, selfY, selfZ int32,
    primX, primY, primZ float64,
) (float64, float64, float64, bool) {
    if l.navigator == nil {
        return primX, primY, primZ, true
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    candidates, count := extendShortClickCandidates(
        selfX, selfY, l.waypoints, l.wpIndex, l.segmentFrameOffset)
    for c := range count {
        sample := candidates[c]
        if _, ok := l.navigator.ValidateClick(from, sample); !ok {
            continue
        }

        return sample.X, sample.Y, sample.Z, true
    }

    return primX, primY, primZ, false
}

// extendShortClickCandidates marches the forward route from the aimed
// waypoint and collects the first samples past the rescue floor: only
// FORWARD, because backward samples pull the walk back off the ground
// the extension just covered.
func extendShortClickCandidates(
    selfX, selfY int32,
    waypoints []pathfind.Vec3, index int, frameOffset float64,
) ([extendCandidateMax]pathfind.Vec3, int) {
    var out [extendCandidateMax]pathfind.Vec3
    count := 0
    px, py := float64(selfX), float64(selfY)
    for i := index; i+1 < len(waypoints) && count < extendCandidateMax; i++ {
        from, to := waypoints[i], waypoints[i+1]
        fdx, fdy := to.X-from.X, to.Y-from.Y
        seg := math.Hypot(fdx, fdy)
        if seg < 1 {
            continue
        }
        steps := int(seg/extendMarchStep) + 1
        for s := 1; s <= steps && count < extendCandidateMax; s++ {
            frac := float64(s) / float64(steps)
            qx := from.X + fdx*frac
            qy := from.Y + fdy*frac
            relX, relY := qx-px, qy-py
            if math.Hypot(relX, relY) < minWalkClick ||
                relX*fdx+relY*fdy <= 0 {
                // Under the rescue floor or backward along the
                // route: such a sample pulls the walk back.
                continue
            }
            out[count] = pathfind.Vec3{
                X: qx, Y: qy,
                Z: anchorZToServerFrame(
                    from.Z+(to.Z-from.Z)*frac, frameOffset),
            }
            count++
        }
    }

    return out, count
}

// clickServerValidated gates a walk click through Navigator.
// ValidateClick: a refused click shortens the line, then hops to the
// nearest swallowed plan bend, then falls back to the re-path of the
// stuck path.
func (l *Loop) clickServerValidated(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64, now time.Time,
) bool {
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    if _, ok := l.navigator.ValidateClick(from,
        pathfind.Vec3{X: *moveX, Y: *moveY, Z: *moveZ}); ok {
        return true
    }
    if l.shortenClickSegment(selfX, selfY, selfZ, moveX, moveY, moveZ, from) {
        return true
    }
    if l.clickForwardJump(selfX, selfY, selfZ, moveX, moveY, moveZ, now) {
        return true
    }
    if l.clickEscapeHop(selfX, selfY, selfZ, moveX, moveY, moveZ) {
        return true
    }
    // No local escape works: re-path from the current position like the
    // stuck path does, bounded by the same budget; a second re-path from
    // the same cell aborts the frozen trip.
    if l.noteRepathCell(selfX, selfY) {
        l.abortFrozenTrip(
            "the server refuses the walk click from this cell")

        return false
    }
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.rePaths++
    if l.rePaths > maxRePaths {
        l.abortTownTrip("the server refuses every walk click")

        return false
    }
    l.logger.Printf("Hunt: the server would refuse the walk click to "+
        "%d %d, re-pathing (%d of %d)",
        int32(*moveX), int32(*moveY), l.rePaths, maxRePaths)
    if !l.replanTownWalkSegment(l.segmentDest) {
        // The re-path found no route: the same freeze evidence the
        // stuck path carries - the ladder owns it (see
        // stuckTownWalk).
        l.abortFrozenTrip("re-path failed")
    }

    return false
}

// clickForwardJump answers a refused corner click with the first later
// plan waypoint the transport validates from the standing cell: the
// corner band stops 8 to 50 units short of the turn pivot, and a
// farther chord clears the corner at a wider angle.
func (l *Loop) clickForwardJump(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64, now time.Time,
) bool {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) {
        return false
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for next := l.wpIndex + 1; next < len(l.waypoints); next++ {
        wp := l.waypoints[next]
        to := pathfind.Vec3{
            X: wp.X, Y: wp.Y,
            Z: anchorZToServerFrame(wp.Z, l.segmentFrameOffset),
        }
        corrected, ok := l.navigator.ValidateClick(from, to)
        if !ok {
            continue
        }
        // The jump must move a real step (one geodata cell): a
        // correction landing on the standing cell would click the
        // walker's own position.
        if math.Hypot(corrected.X-from.X, corrected.Y-from.Y) <
            extendMarchStep {
            continue
        }
        // The jump rides the skip discipline: a jump that left the
        // character on this cell proved the chord dead.
        if !l.skipMoveFresh(selfX, selfY) {
            return false
        }
        l.wpIndex = next
        l.moveAt = now
        l.noteSkipStand(selfX, selfY)
        l.logger.Printf("Hunt: the turn click is walled, jumping the "+
            "cursor to the waypoint %d of %d whose line validates",
            next, len(l.waypoints))
        *moveX, *moveY, *moveZ = corrected.X, corrected.Y, corrected.Z

        return true
    }

    return false
}

// shortenClickSegment halves a refused click toward its target until a
// prefix validates: a split segment rasterizes differently, so the far
// half can fail where a shorter prefix passes.
func (l *Loop) shortenClickSegment(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64, from pathfind.Vec3,
) bool {
    dx := *moveX - float64(selfX)
    dy := *moveY - float64(selfY)
    dz := *moveZ - float64(selfZ)
    full := math.Hypot(dx, dy)
    for segment := full / 2; segment >= clickShortenFloor; segment /= 2 {
        frac := segment / full
        shortX := float64(selfX) + dx*frac
        shortY := float64(selfY) + dy*frac
        shortZ := float64(selfZ) + dz*frac
        if _, ok := l.navigator.ValidateClick(from, pathfind.Vec3{
            X: shortX, Y: shortY, Z: shortZ,
        }); ok {
            *moveX, *moveY, *moveZ = shortX, shortY, shortZ

            return true
        }
    }

    return false
}

// clickEscapeHop walks the nearest plan bend the arrival slack
// swallowed: the geodata trap cells box the walker in, and the far
// clicks keep leaving the boxed cell until the character stands on the
// bend itself.
func (l *Loop) clickEscapeHop(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64,
) bool {
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for j := l.wpIndex - 1; j >= 0; j-- {
        wp := l.waypoints[j]
        // The anchored distance: on a shifted deck the raw 3D distance
        // to a bend the character stands on measures the shift alone.
        dist := waypointDistanceAnchored(wp, selfX, selfY, selfZ,
            l.segmentFrameOffset)
        if dist > waypointPassDist {
            // Deeper waypoints walk backward: the escape is over.
            break
        }
        if dist <= hopCoincideDist {
            // Stood on it: no hop needed.
            continue
        }
        if _, ok := l.navigator.ValidateClick(from, pathfind.Vec3{
            X: wp.X, Y: wp.Y,
            Z: anchorZToServerFrame(wp.Z, l.segmentFrameOffset),
        }); ok {
            *moveX, *moveY, *moveZ = wp.X, wp.Y,
                anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
            l.logger.Printf("Hunt: walk click refused, hopping "+
                "back to the plan bend at %d %d",
                int32(wp.X), int32(wp.Y))

            return true
        }
    }

    return false
}

// segmentAdvanceClear gates the advance past the waypoint whose
// successor sits at the index: the line from the standing cell to that
// successor must pass the click transport, the one oracle the click
// itself faces. A refused line keeps the skipped-from waypoint targeted.
func (l *Loop) segmentAdvanceClear(
    selfX, selfY, selfZ int32, next int,
) bool {
    return l.routeAdvanceClear(l.waypoints, next,
        l.segmentFrameOffset, selfX, selfY, selfZ)
}

// routeAdvanceClear is the segmentAdvanceClear gate over a caller
// supplied route and frame offset: the manual walk plan carries its
// own waypoints and z transport, so the continuous flow scan of both
// followers answers through one click transport semantics.
func (l *Loop) routeAdvanceClear(
    waypoints []pathfind.Vec3, next int, frameOffset float64,
    selfX, selfY, selfZ int32,
) bool {
    if next < 0 || next >= len(waypoints) || l.navigator == nil {
        return true
    }
    wp := waypoints[next]
    clickZ := anchorZToServerFrame(wp.Z, frameOffset)
    targetX, targetY, targetZ := walkClickSegment(
        selfX, selfY, selfZ, wp.X, wp.Y, clickZ, maxMoveDistance)
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    corrected, ok := l.navigator.ValidateClick(from, pathfind.Vec3{
        X: targetX, Y: targetY, Z: targetZ,
    })
    if !ok {
        return false
    }
    // The delivery test: the server's correction must land within the
    // waypoint pass radius of the asked target, else the cursor keeps
    // the skipped-from waypoint.
    return math.Hypot(corrected.X-targetX, corrected.Y-targetY) <=
        waypointPassDist
}
