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
// with the waypoint height anchored into the server frame (see
// click_frame.go): the plan's own start waypoint resolved onto the
// standing cell and its pack height carries the vintage shift the
// frame offset measured, so the raw 3D distance to a waypoint the
// character stands ON is the shift alone. The jewelry shop porch of
// the farm readiness round (2026-09-20) measured 64 units on a plan
// whose every waypoint rode the shifted deck - 14 units past the
// intermediate pass radius - the cursor never advanced off the plan's
// own start and the self click of the pinned cursor burned the whole
// recovery ladder without a single walk attempt. The anchoring keeps
// the deck edge protection the 3D distance exists for: a waypoint a
// whole deck below the character still measures its true gap, the
// shift (tens to a few hundred units) explains only the vintage
// disagreement.
func waypointDistanceAnchored(
    wp pathfind.Vec3, selfX, selfY, selfZ int32, frameOffset float64,
) float64 {
    dx := wp.X - float64(selfX)
    dy := wp.Y - float64(selfY)
    dz := anchorZToServerFrame(wp.Z, frameOffset) - float64(selfZ)

    return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// waypointArrived reports whether the follower counts the waypoint at
// the index as reached from the character position: the intermediate
// waypoints need the tight pass radius (a detour turn or a bridge ramp
// entry must be walked through - the wide radius let the follower cut
// the corner into the railing), the final waypoint keeps the trip
// arrival radius of the segment (the search goal of the segment, the server
// may stop the character slightly short of the click). The tight ring
// segments (the teach stop close approach) pass their own tight final
// radius: the wide slack would end the walk a whole ring short of the
// teacher. The frame offset anchors the waypoint height into the
// server frame first (see waypointDistanceAnchored): the arrival test
// of the plan's own start must not measure the vintage shift.
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

// finalArriveRadius answers the arrival radius of the final waypoint
// of the current town segment: the wide trip slack for the ordinary segments,
// the tight pass radius for the close ring segments - a segment that searches
// its route within npcApproachOffset of the npc must actually reach
// the route end, the wide slack accepts a stop a full ring short of
// the teacher and dumps the last stretch onto the straight offset
// clicks whose lines cross the roof-only interior bands (the trainer
// hall rows carry the floor, the spaces between them do not). The
// exact segments (the merchant stops' approach zero search) answer the
// tight radius the same way: their final cell is the customer spot
// the segment exists to reach.
func (l *Loop) finalArriveRadius() float64 {
    // The exact segments (the merchant stops' startWalkExactSegment) must walk
    // the plan all the way to its final cell: the wide arrive radius
    // would end the walk a whole arrive radius short of the customer
    // cell the exact search answered - the standing spot opposite the
    // merchant behind the counter.
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        return waypointPassDist
    }
    if l.segmentRadius > 0 && l.segmentRadius < tripApproachRadius {
        return waypointPassDist
    }

    return waypointArriveDist
}

// waypointPassed reports whether the character already moved past the
// waypoint along the route towards the next one: the projection of the
// character onto the wp -> next segment is beyond the waypoint and the
// character stays within the corridor of the segment. A raw "the next
// waypoint is closer" test once let the follower skip the bridge entry
// waypoints while the character stood at the RAILING SIDE of the deck -
// the waypoint across the bridge was closer through the railing than
// the entry around the ramp - and the bot ground into the railing
// instead of walking the planned detour. The projection test keeps the
// skip working for its real purpose (a server correction or a restart
// jump placing the character ahead ON the route) while a character off
// to the side of the segment keeps targeting the waypoint it missed.
// The test is planar on purpose: the z axis belongs to the arrival
// distance, and a segment that degenerates in the plane (a vertical
// drop) never passes the character by the lateral logic.
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
// the route at or beyond the aimed waypoint: the projection onto the
// aimed waypoint's own segment past its start (the plain passed test
// of the cursor advance), or the projection onto ANY later segment of
// the remaining route (the march past a bend the pinned cursor never
// advanced onto - the 2026-09-20 acceptance round's armed extension
// walked the character along the route samples across the wp 1 bend
// while the cursor stayed pinned on wp 0, and the plain single segment
// test measured the lateral to the OLD segment - a full corridor
// width past it - and answered not passed, so the raw aim of the
// passed start waypoint walked the character the whole bend back).
// A character inside the arrival radius of the aimed waypoint never
// counts (the caller gates on it): the pull back onto the waypoint is
// the re-approach click the ramp corner needs, not a backward walk.
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

// walkTownWaypoints follows the planned waypoints with ground click
// walks and returns true when the final waypoint is reached. The plan
// prices the water (the mesh swim rate of the C1 zone data), so the
// follower walks the wet segments it planned - the swim is a priced
// slowdown, not a failure.
// Segments longer than the server move request limit are split into
// straight intermediate points (the smoothing guarantees the line of
// sight of every segment, so the intermediate points stay on the verified
// segment). A waypoint the character already passed ON THE ROUTE is
// skipped: a server position correction or a restart jump can place
// the character ahead of the follower, and walking back to a passed
// waypoint would loop. A walk that stands still re-paths from the
// current position to the segment destination, bounded by the re-path
// budget of the trip.
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

// advanceWaypoints walks the waypoint cursor forward as far as the
// character's position allows: a waypoint counts as passed when it is
// reached within its radius or already bypassed along the route AND
// the straight line from the actual standing cell to the successor
// waypoint is walkable. The arrival radius is wide enough to cover
// the tight waypoints of a ramp climb, but the line from the actual
// standing cell to the next waypoint may still cross a closed wall -
// skipping ahead would click through it and the server cancels the
// move at the character's own position (the 2026-09-11 teacher walk
// stuck: the follower skipped the 16 unit ramp steps and clicked the
// plaza waypoint through the railing). The gated waypoint stays the
// target: walking onto it re-opens the line.
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

// followWaypoints is the shared waypoint follower core of the town
// segments: the waypoint arrival (tight for the
// intermediate turns, wide for the final goal), the passed waypoint
// skipping, the stuck tracking and the click pace. The planned water
// segments walk like the dry ones: the plan prices the crossings (the
// swim rate), the follower follows it - the re-plan ladder owns
// the off-plan swims instead of a click guard (see walkTownWaypoints).
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
    // The stuck handling may have re-planned the segment: the fresh plan
    // starts at the standing cell, so its wp 0 IS the character's own
    // position and a click at it is the self-click the server always
    // collapses (distance below the cancellation limit) - the round 56
    // reproduction caught the recovery burning a second re-path on
    // exactly that refusal. Re-run the cursor advance so the click
    // below aims the fresh plan's first real waypoint instead. The
    // stuck recovery may also have armed the cursor key escape (the
    // refusing pocket branch): the claims own the segment then, the click
    // below stays down until the escape settles.
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
    // The goal span of the click: the farthest route waypoint the
    // server delivers inside the aim window (see flowAdvance). The
    // click never targets a point the character reaches before the
    // next request re-aims farther, so the walk crosses the dense
    // plan points without an arrival stop.
    l.wpIndex = l.flowAdvance(l.waypoints, l.wpIndex,
        l.segmentFrameOffset, selfX, selfY, selfZ)
    l.clickWaypoint(selfX, selfY, selfZ, now)

    return false
}

// waypointPassedBehind reports whether the aimed waypoint sits behind
// the character along the route: the character marched past it and a
// click onto it walks backward (see waypointPassedAlongRoute). A
// character inside the arrival radius of the aimed waypoint never
// counts - the pull back onto the waypoint is the re-approach click
// the ramp corner needs, not a backward walk (the round 56 gated
// waypoint design).
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
// idle aggressive camps, guards the line against the server refusal
// and sends it. The coordinate half (the z anchoring and the server
// move request limit clip) lives in anchorWaypointClick, the short
// click extension and the backward holds live in resolveClickExtension
// (see minWalkClick); the remaining ladder reads as the linear gate
// chain: the hold verdicts, the aggro camp skip, the aggro aware
// steering and the server click validation - the validation runs after
// the steering so the line it verifies is the one actually being
// sent. Without a navigator the guard stays off (the walk was planned
// elsewhere, the follower only walks it).
func (l *Loop) clickWaypoint(
    selfX, selfY, selfZ int32, now time.Time,
) {
    wp := l.waypoints[l.wpIndex]
    moveX, moveY, moveZ, dist := l.anchorWaypointClick(
        selfX, selfY, selfZ, wp)
    // The backward walk guard: an aimed waypoint the character already
    // moved PAST along the route itself (see waypointPassedAlongRoute)
    // would walk it BACK off the ground the route samples just covered
    // - the one step away and return ping pong of the 2026-09-20
    // acceptance round. The armed extension keeps its own behind
    // verdict too (the far V-detour waypoint whose leaving segment
    // already points back, see waypointBehindRoute).
    passedBehind := l.waypointPassedBehind(wp, selfX, selfY, selfZ)
    moveX, moveY, moveZ, send := l.resolveClickExtension(
        selfX, selfY, selfZ, dist, passedBehind, moveX, moveY, moveZ)
    if !send {
        return
    }
    // A waypoint inside an idle mob's trigger circle cannot be reached
    // by any tangent arc (the tangent side flips at every re-issue, see
    // segmentTargetThreatened): skip it ahead instead of ping-ponging
    // around the camp for hours. The verdict reads the WAYPOINT the
    // skip would abandon, never the issued click target: a far
    // waypoint's click rides the maxMoveDistance clip next to the
    // character, so a camp sitting between the character and the
    // horizon flags every far waypoint along that direction while the
    // waypoints themselves stay hundreds to thousands of units outside
    // the circle - the 2026-09-20 town walk (build 4805d9a, bot test3)
    // skipped cursors 9 through 23 of 42 in three seconds that way,
    // marked the whole middle of the plan passed while the character
    // stood still and walked the rest cross-country. The skip reuses
    // the stuck machinery's clear-successor gate, so the cursor only
    // jumps onto a waypoint the standing cell can click directly.
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
        // No clear successor, or the previous skip moved the character
        // nothing: the denied skip must not march the cursor away from
        // a frozen character, the click of the threatened waypoint
        // rides the steered arc, and the stuck window escalates the
        // segment when the click moves nothing too.
    }
    // The aggro-aware steering: the camps of idle aggressive mobs
    // sitting on the segment bend it sideways (see loop_avoid.go). Every
    // transit walk passes through it - the town runs both ways, the
    // returns to the farm spot, the inter-ground walks of the spot
    // economy - while the mobs at the destination stay exempt.
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

// anchorWaypointClick resolves the click target coordinates of the
// aimed waypoint: the click z rides the server frame transport (see
// click_frame.go) - the mesh frame waypoint height plus the measured
// vintage shift of the standing surface - the server resolves the
// click's destination layer by the nearest height to this z, and the
// raw mesh z names the wrong layer wherever the packs disagree (the
// village sandwich refusals). A far waypoint's click clips to the
// server move request limit along the line to the character, the z
// interpolated along the same line. The distance to the waypoint
// comes back too: the extension ladder of resolveClickExtension keys
// its rescue floor on it.
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

// resolveClickExtension runs the extension ladder of the aimed
// waypoint click - the minWalkClick interplay and the backward holds.
//
// The rescue floor discipline runs IMMEDIATELY, no stuck verdict
// needed: the server's findPath branch only takes a collapsed click
// over the rescue threshold, a shorter one is silently canceled with
// ActionFailed and never moves a cell (the 2026-09-11 11:34 dump
// froze two whole trip cycles on the 22 unit first waypoint click) -
// the sub-floor aim re-aims at the forward route samples before any
// click leaves the bot. No sample validating keeps the plain waypoint
// click: the refusal machinery of clickServerValidated answers it
// exactly like today.
//
// The recovery of a stuck segment (extendArmed): the stuck proved the
// plain clicks of this segment do not move the character (a server
// side refusal the offline click validation cannot see), so the
// primary target behind the character on the route gives way to the
// forward route samples. No forward sample validating and the
// waypoint behind: clicking it walks the character backward into the
// pocket the route samples just escaped. Hold the click - the stuck
// window re-plans from the standing cell, and the planner knows the
// wall the server-side routing has to route around.
//
// The same hold without the armed extension: a backward click is
// ground loss no matter the recovery state, and the stuck window owns
// the answer (the skip ladder walks the first clear successor, the
// re-path plans around the wall).
//
// Returns the click target to send and whether the click may leave
// the bot at all - the hold verdicts answer false and the caller
// drops the click.
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

// waypointBehindRoute reports whether the waypoint at the index sits
// behind the character relative to the route segment leaving it: the
// cursor can stay pinned on a waypoint the character already moved
// past (the successor line is not clear yet), and clicking it walks
// the character BACK off the ground the forward route samples just
// walked - the extension takes over such clicks. Only a waypoint the
// character stands NEAR counts: the V-shaped detour routes (the
// 2026-09-12 trainer hall recovery - the route climbs far north
// before it doubles back south east) carry waypoints hundreds of
// units ahead whose position projects beyond the doubling segment,
// and clicking those far waypoints is the whole point of the climb -
// the projection test alone re-aimed their clicks at the far route
// samples whose straight lines cross the terrace walls (the freeze
// the walled aisle reproduction exposed). A waypoint ahead or beside
// the character (the normal pull back onto the route, the round 56
// gated waypoint design) never triggers it.
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

// extendShortClick re-aims a click whose primary target sits under the
// server rescue floor (minWalkClick) at the forward route samples past
// the floor: the click line still starts at the standing cell, but its
// target walks along the planned route far enough that a server side
// collapse hands the click to the server pathfinder instead of
// silently canceling it (see the minWalkClick contract). The candidates
// try in forward order and the first the local rules bless wins - the
// server click validation port runs on the chord, and a route
// cell that walls one chord (the 2026-09-11 11:34 reproduction: the
// terrace hillside step refused every chord crossing it) only skips
// that sample, the next one further along the route still carries the
// click. No candidate passing keeps the plain waypoint click - the
// refusal machinery of clickServerValidated answers it exactly like
// today.
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

// extendShortClickCandidates marches the forward route of the plan
// starting at the aimed waypoint and collects the first route samples
// whose straight line distance from the standing position reaches the
// floor: the click chords to them clear the server rescue threshold
// while their targets stay on the planned route. The march only looks
// FORWARD - the aimed waypoint itself may sit behind the character (a
// pinned cursor beside the route), and the backward polyline samples
// produced chords the server refused from the cells past the first
// step (the 2026-09-11 11:34 reproduction: the second extension
// clicked a 24 unit chord into the terrace wall and the escape hop
// walked the character back, a ping pong through the whole stuck
// cycle). A route whose whole forward stretch stays under the floor
// collects nothing - the arrival case keeps its plain waypoint click.
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
                // route: a sample behind the character pulls the
                // walk back off the ground the extension just
                // walked (the 2026-09-11 11:34 reproduction
                // ping ponged on exactly the samples near the
                // pinned waypoint the character already passed).
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

// clickServerValidated gates a walk click through the server
// validation port (Navigator.ValidateClick): a click the server would
// cancel never moves the character, so sending it just grinds
// ActionFailed answers until the stuck timeout fires - the town walk
// stuck of 2026-09-10 (the bot clicked the second waypoint 58 units
// over the village plaza corner, the geodata correction collapsed the
// target onto the walker and the character froze through all three
// re-paths). A refused click shortens the segment first (the Bresenham
// prefix of a split segment is not a prefix of the full raster, a shorter
// line often validates), then hops to the nearest swallowed plan bend
// (the escape step out of a trap cell), and finally falls back to the
// re-path of the stuck path. It reports whether the click target in
// the move pointers may be sent.
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
    // No local escape works: re-path from the current position like
    // the stuck path does, bounded by the same budget. A re-path
    // from the cell the previous one already planned from (and
    // moved nothing on) proves the fresh plan cannot move the
    // character either: the trip aborts for its callers' recovery.
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

// clickForwardJump answers a refused click with the first later plan
// waypoint whose straight line the server transport validates from
// the standing cell: the corner turn band. The destination
// correction of the previous leg stops the character 8..50 units
// short of the turn pivot (the click transport cannot reach the
// funnel pivot over the corner approach cells), the pass radius
// counts the turn reached, and the turn chord from the stopped
// position cuts the corner - the server collapses it, the shorten
// ladder halves into the same corner and the back hop walks the
// character away from the corner it wants to round (the re-approach
// lands in the same band - the ping pong). A FARTHER waypoint's
// chord clears the corner at a wider angle (the real pack probe of
// the 2026-09-20 delevel round: from the stuck band at 40968 53400
// the turn wp and every forward sample along the outgoing leg refuse
// while wp+2 validates and the pivot neighborhood ring answers 33 of
// 36). The cursor jumps onto the validated waypoint and the move
// pointers carry its corrected destination - the same jump the stuck
// handler runs after its window, moved to click time: the 15 s stuck
// window never opens for a corner the scan answers, and the re-path
// ladder (whose deterministic re-plan reproduces the identical route)
// never burns. It reports whether the pointers carry a validated jump
// target.
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
        // The jump must move the character a real step: a validated
        // waypoint whose correction lands on the standing cell (the
        // click transport refuses the whole chord but the collapse
        // check passes on a rounding edge) would click the walker's
        // own position - the server cancels it and the stick stays.
        // One geodata cell is the honest step floor.
        if math.Hypot(corrected.X-from.X, corrected.Y-from.Y) <
            extendMarchStep {
            continue
        }
        // The jump rides the same discipline as the waypoint skips
        // (the 2026-09-21 storm class): a jump that left the character
        // on this very cell proved the chord bet dead on this
        // transport, and a farther bet would march the cursor while
        // the character stands still. The stuck ladder owns the
        // segment instead.
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

// shortenClickSegment halves a refused click segment toward its target until a
// prefix validates: the split segments of a long waypoint line rasterize
// differently than the planned segment (the Bresenham accumulator starts
// from the endpoint deltas), so the far half of a line can fail where
// a shorter prefix passes. It reports whether the move pointers carry
// a validated shorter target.
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
// swallowed: the geodata holds trap cells (entered legally through an
// open wall, their own walls box the walker in - the village terrace
// rows), and the re-path plans the escape step over them; but the bend
// sits inside the waypoint arrival radius, the cursor skips it and the
// far clicks keep leaving the boxed cell. The hop clicks the bend
// directly so the character stands on it and the following clicks
// validate from there. It reports whether the move pointers carry a
// validated hop target.
func (l *Loop) clickEscapeHop(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64,
) bool {
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for j := l.wpIndex - 1; j >= 0; j-- {
        wp := l.waypoints[j]
        // The anchored distance: on a shifted deck the raw 3D
        // distance to a bend the character stands on measures the
        // shift alone, past the pass radius - the hop scan would
        // break on the first candidate and never hop (the same
        // frame duality the arrival test answered, see
        // waypointDistanceAnchored).
        dist := waypointDistanceAnchored(wp, selfX, selfY, selfZ,
            l.segmentFrameOffset)
        if dist > waypointPassDist {
            // Deeper waypoints stand farther back along the
            // route: walking to them retraces the route
            // instead of escaping the spot.
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

// segmentAdvanceClear reports whether the follower may advance past the
// waypoint whose successor sits at the index: the straight line from
// the CURRENT character position to that next waypoint must be
// walkable by the SERVER CLICK TRANSPORT - the same oracle the click
// the follower would send for that waypoint passes (Navigator.
// ValidateClick, the GeoEngine.getValidLocation port: the climb limit
// with its layer step-over, the free drops, the source wall plus the
// anti corner cut). The server validates every ground click as a
// straight line (the deployment runs PathFinding = 0 so no server side
// routing exists): a click whose first step hits a closed wall
// resolves to the character's own position, the move is canceled at
// once and the character never moves. The old follower skipped any
// waypoint inside the 50 unit pass radius - tighter than the 16 unit
// ramp steps of the trainer plaza approach - and clicked the far
// waypoint straight through the plaza railing: the click canceled, the
// 15 s stuck detector re-planned the identical deterministic route,
// the follower skipped the same tight waypoints again and the third
// budget burned into "town trip ended: aborted, walk stuck" (the
// 2026-09-11 teacher walk, the lessons never reached the teacher). The
// gate keeps the skipped-from waypoint as the target until walking
// onto it re-opens the line.
//
// The oracle choice is the load bearing contract: the gate and the
// click MUST answer through one semantics. The old gate asked the grid
// engine's symmetric line of sight (the A* node rule: a step must
// climb no more than the passable height up AND down, both cells'
// walls open in both directions) while the clicks obeyed the
// asymmetric server rule - on ordinary terrain (a terrace drop, a one
// sided wall) the two disagree, the cursor pinned on a waypoint whose
// click the server walked fine, the short click extension escaped
// ~60 units out and the pinned cursor clicked the character right
// back - the plaza ping-pong of the 2026-09-20 farm readiness report
// (nine alternating MoveToLocation clicks between two points 63 units
// apart while the server accepted every one of them, the frozen trip
// verdict then banning the walkable plaza for the session). The
// round 57 family pinned the same disagreement from the other side:
// the pinned cursor kept re-clicking the 22 unit first waypoint the
// server's rescue threshold silently canceled while the far segments
// of the very plan validated fine.
//
// The gate mirrors the click exactly the follower would send: the
// waypoint height anchored into the server frame (see
// click_frame.go), the distance capped at the move request limit,
// and the port's own verdict - a click the transport validates walks
// the character SOMEWHERE along the line (a running click may stop
// a hundred units short of the asked cell at a terrace lip; the
// character still made that progress, the arrival test and the next
// click converge from there), a click the transport collapses onto
// the walker (the correction under the cancellation limit) never
// moves a cell and the cursor keeps the skipped-from waypoint
// targeted until walking onto it re-opens the line.
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
    // The delivery test: the server's correction of the line must
    // land within the waypoint pass radius of the asked target - a
    // click that stops a hundred units short (the terrace lip of the
    // round 57 route, the hall wall of the round 56 aisle approach)
    // has not delivered the waypoint, the cursor keeps the skipped
    // from waypoint targeted and the character walks the plan's bends
    // in the order the planner drew them.
    return math.Hypot(corrected.X-targetX, corrected.Y-targetY) <=
        waypointPassDist
}
