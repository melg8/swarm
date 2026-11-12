// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// refusalEvidence reports whether the server answered the last walk
// click of this segment with ActionFailed while the character stood
// still: the one byte refusal answer is the only online channel that
// names a server side move refusal (the offline click validation
// port mirrors the reference Mobius master, but a deployment running
// a different build or geodata validates the same click differently
// - the 2026-09-14 10:18 dump: every click of every plan bounced
// while the local reference stack walked the identical scenario
// cleanly, and the dump's unknown packet fingerprints 0x57/53 bytes
// and 0xe7/21 bytes do not exist on the reference master). The
// correlation is the sent click timestamp: the answer arrives after
// the request, and a click the server accepted moves the character
// instead (the stuck verdict calling this already proved it did
// not).
//
// The attribution is honest about the answer's owner: the ActionFailed
// packet carries no request identity, and every other request of the
// session (the equip and skill requests of the gear machinery, the
// transactions) answers ActionFailed the same way. An arrival with a
// non walk request sent after the click belongs to that request at
// least as likely as to the click - the gear swap refusals ride the
// walk clicks exactly this way (the 2026-09-14 12:31 dump rerun: the
// equip ActionFaileds landed one to three seconds after the walk
// clicks of the same tick window) - so the correlation skips them
// instead of latching a walk refusal that never happened.
func (l *Loop) refusalEvidence() bool {
    if l.moveAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    return failedAt.After(l.moveAt) &&
        failedAt.Sub(l.moveAt) <= refusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.moveAt, failedAt)
}

// sendVariedAim answers a stuck verdict with refusal evidence by
// varying the aim at the current waypoint instead of assuming the
// corridor froze: the server refused THIS click, and the refusal is
// target specific - the Bresenham raster of a shorter prefix or a
// sideways offset of the same waypoint often validates where the
// plain aim bounced. The variant list walks from the safest
// variation (half of the segment) to the sideways probes (the
// perpendicular offsets that relocate the character off the refused
// flank). Each variant passes the same offline gates as a planned
// click (the server click port on its line); the first gate-passing
// variant is clicked. It reports whether a
// variant was sent (the caller resets the stuck window for it).
func (l *Loop) sendVariedAim(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) ||
        l.refusalVariants >= refusalVariantsMax {
        return false
    }
    wp := l.waypoints[l.wpIndex]
    // The varied aims ride the same server frame transport as the
    // plain clicks (see click_frame.go): the interpolated z of every
    // variant starts from the anchored waypoint height, so the
    // variant that finally validates names the standing surface's
    // layer in the server frame, not the mesh frame the packs
    // disagree about.
    wp.Z = anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for ; l.refusalVariants < refusalVariantsMax; l.refusalVariants++ {
        variant := refusalVariantTarget(
            wp, from, l.refusalVariants)
        if variant == nil {
            continue
        }
        to := pathfind.Vec3{
            X: variant[0], Y: variant[1], Z: variant[2],
        }
        if _, ok := l.navigator.ValidateClick(from, to); !ok {
            continue
        }
        l.refusalVariants++
        l.logf("Hunt: the server refused the walk click, "+
            "varying the aim to %.0f %.0f (%d of %d)",
            to.X, to.Y, l.refusalVariants, refusalVariantsMax)
        l.moveAt = now
        if err := l.game.WalkTo(
            int32(to.X), int32(to.Y), int32(to.Z)); err != nil {
            l.logf("Hunt: town walk request failed: %v", err)
        }

        return true
    }

    return false
}

// refusalVariantTarget computes the varied click target of one
// refusal variant index. The variation works on the CLICK geometry,
// not the raw waypoint distance: the plain click caps at maxMoveDistance
// (the follower's segment splitting), so the variants must sit INSIDE
// that cap or they aim farther than the click that just bounced. On
// the capped segment, variant 0 halves the click (the Bresenham prefix
// of a split segment rasterizes differently than the full line), 1
// quarters it, 2 and 3 bend the half click sideways (left and right
// of the route direction - the probe that relocates the character
// off the refused flank while keeping the forward progress). It
// returns nil when the click is too short to vary.
func refusalVariantTarget(
    wp pathfind.Vec3, from pathfind.Vec3, index int,
) *[3]float64 {
    dx := wp.X - from.X
    dy := wp.Y - from.Y
    full := math.Hypot(dx, dy)
    if full < minWalkClick {
        return nil
    }
    // The click geometry: the plain click caps at maxMoveDistance, the
    // variants vary the click - never the raw waypoint distance.
    click := full
    if click > maxMoveDistance {
        click = maxMoveDistance
    }
    frac := 0.5
    side := 0.0
    switch index {
    case 1:
        frac = 0.25
    case 2:
        side = refusalVariantStep
    case 3:
        side = -refusalVariantStep
    }
    ux, uy := dx/full, dy/full
    x := from.X + ux*click*frac - uy*side
    y := from.Y + uy*click*frac + ux*side
    z := from.Z + (wp.Z-from.Z)*(click*frac/full)

    return &[3]float64{x, y, z}
}

// noteMoveStart is the move start watchdog of the walker: the
// deadline arms when a walk click goes out (the moveAt send slot)
// while the character stands still, a position change or the server's
// own movement broadcast clears it (the click obviously started the
// movement) and a deadline that passes with the character still on
// the baseline cell names the click dead. It reports the fire: the
// caller runs its next recovery mode at once instead of standing out
// the full stuck window - the owner rule of the 2026-09-19 round: a
// movement command that did not start the movement must not leave
// the character standing in one spot while the window burns. The
// window covers the server's once per second movement broadcast gate
// plus the network lag with head room, so an accepted click always
// clears the deadline before it can fire.
func (l *Loop) noteMoveStart(
    now time.Time, selfX int32, selfY int32,
) bool {
    if l.moveStartAt.IsZero() {
        if l.moveAt.IsZero() || l.tracker.SelfWalking() {
            return false
        }
        l.moveStartAt = l.moveAt.Add(moveStartWindow)
        l.moveStartX, l.moveStartY = selfX, selfY
        // Fall through: the arming tick may already sit past the
        // deadline (the click went out a tick before the watchdog
        // first saw it) - the fire below must not wait another
        // window for a verdict it can already name.
    }
    if selfX != l.moveStartX || selfY != l.moveStartY ||
        l.tracker.SelfWalking() {
        l.moveStartAt = time.Time{}

        return false
    }
    if now.Before(l.moveStartAt) {
        return false
    }
    // The click never started the movement: one fire per arming, the
    // deadline re-arms on the next recovery click.
    l.moveStartAt = time.Time{}

    return true
}

// walkStuck tracks the movement progress of the walker and re-paths
// around the obstacle once the character stands still for too long OR
// wobbles without net progress toward the current waypoint. The same
// cell check alone misses the oscillation loops: the aggro steering
// flips its tangent side whenever the segment target sits inside a threat
// circle, and the character ping-pongs between the two tangent
// endpoints - every hop moves, so a plain movement reset never fires,
// while the walk makes no progress at all (the observed delevel walk
// burned an hour between two points 300 units apart). The net progress
// gate resets the window only when the waypoint cursor advanced or the
// current waypoint came measurably closer; anything else that outlives
// the timeout is a stuck and runs the recovery below. The first stuck
// of a trip waits the full stuckTimeout (a slow server position
// broadcast must not trip a false stuck); subsequent stucks within the
// same trip wait the shorter stuckFastTimeout - once the walker knows
// the server refuses its clicks on this segment, waiting the full window
// for every waypoint just burns the trip's time budget. The skip of a
// waypoint does NOT consume the re-path budget: only the full segment
// re-plan (startWalkSegment) does. The move start watchdog force (see
// noteMoveStart, wired by followWaypoints) skips the window check for
// one verdict: a click that never started the movement runs the
// recovery at once. It reports whether the trip had to abort.
func (l *Loop) walkStuck(now time.Time, selfX int32, selfY int32) bool {
    forced := l.forceStuck
    l.forceStuck = false
    if l.stuckAt.IsZero() {
        l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
        l.stuckWP = l.wpIndex
        l.stuckBest = l.stuckWaypointDistance(selfX, selfY)

        return false
    }
    if l.stuckProgressed(selfX, selfY) {
        l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
        l.stuckWP = l.wpIndex
        l.stuckBest = l.stuckWaypointDistance(selfX, selfY)

        return false
    }
    timeout := stuckTimeout
    if l.stuckFast {
        timeout = stuckFastTimeout
    }
    if !forced && now.Sub(l.stuckAt) < timeout {
        return false
    }
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.stuckWP = l.wpIndex
    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)

    return l.stuckTownWalk(now, selfX, selfY)
}

// stuckWaypointDistance measures the planar distance from the position
// to the current waypoint (0 without a plan: the follower only runs
// over a planned segment).
func (l *Loop) stuckWaypointDistance(selfX int32, selfY int32) float64 {
    if l.wpIndex < 0 || l.wpIndex >= len(l.waypoints) {
        return 0
    }
    wp := l.waypoints[l.wpIndex]

    return math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY))
}

// stuckProgressed reports whether the walk made net progress since the
// stuck window opened: the waypoint cursor advanced AND the character
// moved (a cursor bump alone - a passed waypoint of a freshly planned
// route - is plan bookkeeping, not ground covered), the character
// moved and the current waypoint came closer by the progress margin,
// or the character moved and the SEGMENT DESTINATION came closer by
// the margin. The destination term is the pinned cursor's honest
// progress signal: the advance gate pins the cursor on a
// waypoint whose sight line ahead is walled (the 2026-09-20
// acceptance round: every line of the Ellenia corridor ahead of the
// plan start was walled for the sight oracle), the armed extension
// then marches the character forward along the route samples while
// the cursor waits - the aimed waypoint sits BEHIND the marching
// character and its distance GROWS, so the waypoint term alone
// judged the healthy march a stuck and burned the re-path budget on
// a walk that was covering ground every tick. The destination
// distance shrinks monotonically on that march and grows on a real
// oscillation (the one step away and return loop lands no closer),
// so the term separates the two exactly. A standstill and an
// oscillation both still fail the gate; a detour climb passes it
// (every waypoint of the route approaches in turn).
func (l *Loop) stuckProgressed(selfX int32, selfY int32) bool {
    moved := selfX != l.stuckX || selfY != l.stuckY
    if !moved {
        return false
    }
    if l.wpIndex != l.stuckWP {
        return true
    }

    waypointCloser := l.stuckWaypointDistance(selfX, selfY) <
        l.stuckBest-stuckProgressUnits

    return waypointCloser || l.stuckDestinationProgressed(selfX, selfY)
}

// stuckDestinationProgressed reports whether the segment destination
// came measurably closer since the stuck window opened WHILE the
// character stays inside the route corridor: the ground covered along
// the planned route counts as progress even when the cursor is pinned
// on a waypoint the character already marched past (see
// stuckProgressed for the pinned march round), while an off-route hop
// that merely happens to point destination-ward (the tangent
// flip-flop's sideways bounce) is no progress at all - the corridor
// test keeps the term honest exactly there.
func (l *Loop) stuckDestinationProgressed(selfX int32, selfY int32) bool {
    if !l.stuckOnRouteCorridor(selfX, selfY) {
        return false
    }

    return l.stuckDestinationDistance(selfX, selfY) <
        l.stuckDestinationDistance(l.stuckX, l.stuckY)-stuckProgressUnits
}

// stuckOnRouteCorridor reports whether the position sits within the
// lateral corridor of the remaining route (the aimed waypoint's
// segment and every segment after it): the route the follower plans
// is the ground the march may cover, the sideways bounce off it is
// not.
func (l *Loop) stuckOnRouteCorridor(selfX int32, selfY int32) bool {
    for i := l.wpIndex; i+1 < len(l.waypoints); i++ {
        wp := l.waypoints[i]
        next := l.waypoints[i+1]
        segX := next.X - wp.X
        segY := next.Y - wp.Y
        segLen := math.Hypot(segX, segY)
        if segLen < 1 {
            continue
        }
        selfDX := float64(selfX) - wp.X
        selfDY := float64(selfY) - wp.Y
        along := (selfDX*segX + selfDY*segY) / (segLen * segLen)
        lateral := math.Abs(selfDX*segY-selfDY*segX) / segLen
        if along >= 0 && along <= 1 && lateral <= waypointCorridor {
            return true
        }
    }

    return false
}

// stuckDestinationDistance measures the planar distance from a
// position to the segment destination (0 without a segment).
func (l *Loop) stuckDestinationDistance(selfX int32, selfY int32) float64 {
    if l.segmentDest.X == 0 && l.segmentDest.Y == 0 &&
        l.segmentDest.Z == 0 {
        return 0
    }

    return math.Hypot(l.segmentDest.X-float64(selfX),
        l.segmentDest.Y-float64(selfY))
}

// pocketRefused reports whether the current refusal verdict stands on
// the very cell where the segment's first refusal latched: the ground the
// character stands on refuses every click (the refusing pocket of
// the 2026-09-14 15:10 report), not one specific click line.
func (l *Loop) pocketRefused(selfX int32, selfY int32) bool {
    return l.segmentRefused &&
        selfX == l.segmentRefusedX && selfY == l.segmentRefusedY
}

// currentWaypoint returns the waypoint the follower cursor aims at.
func (l *Loop) currentWaypoint() (pathfind.Vec3, bool) {
    if l.wpIndex < 0 || l.wpIndex >= len(l.waypoints) {
        return pathfind.Vec3{X: 0, Y: 0, Z: 0}, false
    }

    return l.waypoints[l.wpIndex], true
}

// stuckTownWalk drives the town segment stuck recovery: first try to SKIP
// the current waypoint (a further one may be reachable through a cell
// the server accepts), and when no waypoint ahead has a walkable line
// from the standing cell, re-plan the whole segment from the current
// position. The skip does NOT consume the re-path budget - it advances
// the cursor without re-planning, so the walker can skip several
// waypoints in a row while looking for one the server accepts. The
// skip only jumps onto a waypoint whose straight line from the
// standing cell is walkable (the same gate the cursor advance
// applies): a blind skip arms the follower with a target whose click
// the server collapses partway - the Bresenham line stops at the
// first walled flank and the partial click creeps the character cell
// by cell toward the nearest trap pocket instead of walking the
// route (the 2026-09-11 06:19 aisle dump: the skip jumped from the
// aisle entrance onto the east hall waypoint, every click crept the
// character 16 units east into the dead-end pocket cell at 44776
// 51992 and the pocket's closed east wall then refused the click
// wholesale - "the server would refuse the walk click" - burning the
// re-path budget on the recovery). The fast timeout flag arms after
// the first skip so subsequent stuck detections fire on the shorter
// window.
//
//nolint:funlen // the town walk repeats the stuck checks per segment
func (l *Loop) stuckTownWalk(now time.Time, selfX int32, selfY int32) bool {
    // The online refusal answer separates the server side refusal
    // from the corridor freeze: an ActionFailed that answered the
    // sent click while the character stood still names a refusal
    // the offline validation cannot see (a different server build
    // or geodata), and the refusal is target specific - varying the
    // aim at the same waypoint (a shorter prefix, a sideways
    // offset) often walks where the plain click bounced. The
    // refusal branch runs BEFORE the waypoint skip: the skip aims a
    // FARTHER waypoint (a worse target for a length sensitive
    // refusal), the variation keeps the near aim the plan already
    // holds. Latching the refusal also arms the pocket gate below:
    // a segment whose every aim the server refused on the very
    // standing cell hands the walk to the cursor key escape at once.
    if l.refusalEvidence() {
        if !l.segmentRefused {
            l.segmentRefused = true
            l.segmentRefusedX, l.segmentRefusedY = selfX, selfY
            l.logf("Hunt: the server refused the walk click " +
                "(ActionFailed), varying the aim")
        }
        // The varied aims run first: the refusal is target specific
        // far more often than it is a pocket (the round 82 evidence),
        // one variant per stuck verdict - a variant the server
        // accepts walks the character where the plain aim bounced.
        if l.sendVariedAim(selfX, selfY, l.stuckSelfZ(), now) {
            l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
            l.stuckWP = l.wpIndex
            l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
            l.stuckFast = true

            return false
        }
        // The refusing pocket: the varied aims of this segment are spent
        // (or nothing exists to vary) and the character still stands
        // on the very cell where the first refusal latched - the
        // server answers NO click from this ground (the 2026-09-14
        // 15:10 report: even the official client's mouse clicks died
        // on the plaza cell). The cursor key escape owns the recovery
        // now, walking the claims along the planned route (see
        // beginCursorKeyEscape) - more refused clicks from the same
        // ground would only burn the trip budget.
        if l.pocketRefused(selfX, selfY) {
            if wp, ok := l.currentWaypoint(); ok {
                if l.beginCursorKeyEscape(selfX, selfY,
                    l.stuckSelfZ(), int32(wp.X), int32(wp.Y),
                    int32(wp.Z)) {
                    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
                    l.stuckWP = l.wpIndex
                    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
                    l.stuckFast = true

                    return false
                }
            }
        }
    }
    next := l.nextClearWaypoint(selfX, selfY, l.stuckSelfZ())
    if next > l.wpIndex && l.skipMoveFresh(selfX, selfY) {
        l.wpIndex = next
        l.moveAt = time.Time{}
        l.noteSkipStand(selfX, selfY)
        l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
        l.stuckWP = l.wpIndex
        l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
        l.stuckFast = true
        l.logger.Printf("Hunt: town walk stuck, skipping waypoint "+
            "(cursor %d of %d)", l.wpIndex, len(l.waypoints))

        return false
    }
    if next > l.wpIndex {
        // The clear successor exists but the previous skip left the
        // character on this very cell: the skips proved the click
        // transport dead on this ground, and every farther skip only
        // inflates the aim while the character stands still (the
        // 2026-09-21 storm). The re-path ladder owns the segment now.
        l.logf("Hunt: town walk stuck, the last skip moved the "+
            "character nowhere (cursor %d of %d)", l.wpIndex,
            len(l.waypoints))
    }
    if l.noteRepathCell(selfX, selfY) {
        // The previous re-path started from this very cell and
        // its fresh plan moved the character nowhere: the next
        // re-path would plan the identical route into the same
        // refusal (see frozenRepathLimit). The trip ends and
        // the recovery hands over to its callers.
        l.abortFrozenTrip("walk stuck, no movement since the re-path")

        return true
    }
    // The stuck found no clear successor (the pinned cursor): the
    // plain waypoint clicks of this segment do not move the character
    // - arm the short click extension for the recovery clicks.
    l.extendArmed = true
    l.rePaths++
    if l.rePaths > maxRePaths {
        l.abortTownTrip("walk stuck")

        return true
    }
    l.logf("Hunt: town walk stuck, re-pathing (%d of %d)",
        l.rePaths, maxRePaths)
    if l.journal != nil {
        l.journal.Repath(l.tracker.ID(), l.rePaths)
    }
    if !l.replanTownWalkSegment(l.segmentDest) {
        // The fresh plan found no route (the water may wall every dry
        // one): the freeze evidence still belongs to the escalation
        // ladder - the escape arms over the current plan instead of
        // aborting the trip back into the identical cycle (the
        // 2026-09-14 08:42 dump: the re-path failure aborted past the
        // ladder, the corridor route returned through the fallback
        // and the bot looped on it forever).
        l.abortFrozenTrip("re-path failed")

        return true
    }
    // The re-path proved the plain clicks of this segment do not move
    // the character: arm the fast stuck window for the next detection
    // (4 s instead of 15 s) and re-baseline the stuck window from
    // the re-path tick, so the next stuck fires on the fast timeout
    // the same way the waypoint skip arm does. Without this the
    // first re-path waits the full 15 s before the next detection
    // - the dump of 2026-09-14 08:13 (build c7a0855, bot test3)
    // showed the bot sitting at 45768 49848 -3056 through the whole
    // stuckTimeout after a re-path that did not move it a cell, the
    // recovery burned 30 s of the trip budget on a freeze the fast
    // window would have caught in 4 s.
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.stuckWP = l.wpIndex
    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
    l.stuckFast = true

    return false
}

// skipMoveFresh reports whether a waypoint skip may run for this
// verdict: no skip ran yet, or the character moved since the previous
// skip beyond the stood-on-cell tolerance (the server may correct a
// standing character's position by a unit or two; only real ground
// counts as a fresh episode). A skip that left the character on the
// very cell it already held proved the click transport dead on this
// ground - the successor the skip armed was clicked and the character
// never walked to it.
// Repeating the skip cannot help (the farther waypoint rides the same
// dead transport), it only marches the cursor and the eventual chord
// walk away from the plan while the standing still character watches
// the waypoints get farther (the 2026-09-21 town walk report: 49
// stuck skips advanced the cursor 15 -> 63 of 87 through 3.5 frozen
// minutes), and it starves the recovery ladder that owns the dead
// transport - the re-path budget, the frozen trip abort and the
// cursor key escape all sit behind the skip branch. The first skip of
// a frozen episode stays free: the walled waypoint case walks the
// character on with the successor's click, and only the repeat on an
// unchanged position names the transport dead.
func (l *Loop) skipMoveFresh(selfX int32, selfY int32) bool {
    if !l.skipArmed {
        return true
    }

    return math.Hypot(float64(selfX-l.skipX),
        float64(selfY-l.skipY)) >= hopCoincideDist
}

// noteSkipStand records the cell a waypoint skip leaves the character
// on: the next skipMoveFresh denies the skip that finds the character
// still standing there (see skipMoveFresh for the dead transport
// verdict and the recovery ladder it hands the walk to).
func (l *Loop) noteSkipStand(selfX int32, selfY int32) {
    l.skipArmed = true
    l.skipX, l.skipY = selfX, selfY
}

// noteRepathCell records the cell a stuck re-path plans from and
// reports whether the previous re-path of this trip started from the
// same cell without a single cell of movement in between: the
// deterministic planner re-plans the identical route from the same
// standing position, so the freeze the previous plan could not move
// through is exactly the freeze the next one re-clicks into (the
// 2026-09-11 11:34 dump: three identical re-paths per trip, two trip
// cycles, the character never moved a cell). Any movement between the
// re-paths clears the counter - a different plan shape then has a
// different first segment to try.
func (l *Loop) noteRepathCell(selfX int32, selfY int32) bool {
    frozen := selfX == l.repathX && selfY == l.repathY
    if frozen {
        l.frozenRepaths++
    } else {
        l.frozenRepaths = 0
    }
    l.repathX, l.repathY = selfX, selfY

    return l.frozenRepaths >= frozenRepathLimit
}

// abortFrozenTrip ends a trip whose re-path produced no movement and
// escalates the recovery of the frozen segment before giving up: the town
// walk segments and the zone return segments both hand the walk to the
// cursor key escape along the plan (see escalateFrozenSegment). Without
// the escape the zone return cycles between the
// pathfound-return-stuck-abort and the frozen
// re-plan, never moving: the deterministic planner re-plans the
// identical route the ground keeps refusing (the 2026-09-14 08:25
// dump, build d2ea298: the bot stood at the village terrace for 34s
// after the abort, no walk sent, no Hunt log). The shopping trips keep
// their cooldown recovery when the ladder is exhausted: the hunt
// continues and the next trip retries from a fresh state.
func (l *Loop) abortFrozenTrip(reason string) {
    if l.escalateFrozenSegment() {
        return
    }
    l.abortTownTrip(reason)
}

// escalateFrozenSegment arms the cursor key escape of a frozen town
// walk segment - a segment whose full re-path cycle produced no
// movement at all, the signature of a ground the click transport
// cannot cross (the 2026-09-12 trainer hall aisle dump: the plan
// entered the building through the west aisle column, the server
// walled it, and the character stood frozen through every re-path of
// two whole trips).
//
// The claims transport takes the walk over: the keyboard mode 0 arm
// plus the claimed ValidatePosition stream the server follows without
// any click validation (the 2026-09-14 15:10 report: the official
// client's mouse clicks died on the refusing cell while the ARROW
// KEYS walked it out - the claims are the only movement a click
// refusing ground answers) and walks the character ALONG THE PLANNED
// ROUTE - the plan stays the segment's own route, the claims bend
// where it bends, and the settle returns the walk to the normal
// routed clicks on the same plan (the owner contract of the
// 2026-09-19 round: wasd along the route, the normal mode at the
// point, NEVER walk the direct line - the rung never replaces the
// plan with a straight line to the far target). The re-arm repeats
// while the trip's escape attempts last (cursorEscapeAttemptsMax); a
// spent budget falls back to the plain trip abort with its cooldown.
//
// The historical rung this ladder replaced - the frozen corridor ban
// that sealed the walled waypoint's cells into every later search -
// is gone deliberately: its rectangle granularity walled the whole
// merged mesh sheet any ban touched (the plaza sheet of the
// 2026-09-20 farm readiness report spans 368x32 units around the ban
// center; the reachable world collapsed from 1.3M polygons to 2 and
// every later route search of the session answered "no path at
// all"), its trigger read the follower's own cursor pinning as a
// freeze (the advance gate disagreed with the click transport - see
// segmentAdvanceClear) and its widening (48 -> 1536 units) compounded
// every misfire into a sealed village quarter. The escape owns the
// freeze recovery alone: it walks the character through, it never
// poisons a later search, and its budget bounds itself.
//
// The ladder reports whether the escape took over the recovery (the
// caller skips its abort).
func (l *Loop) escalateFrozenSegment() bool {
    if l.navigator == nil {
        return false
    }
    // The ladder runs for both the town walk segments (phaseTownWalk) and
    // the zone return segments (phaseTownReturn): both follow a planned
    // geodata route whose clicks the click transport may refuse, and
    // both need the claims walk off the refusing ground (the
    // 2026-09-14 08:25 dump: the zone return at the village terrace
    // cycled between the pathfound-return-stuck-abort and the frozen
    // segments because the ladder never ran for it).
    if l.phase != phaseTownWalk && l.phase != phaseTownReturn {
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    if wp, hasWp := l.currentWaypoint(); hasWp {
        if l.beginCursorKeyEscape(selfX, selfY, selfZ,
            int32(wp.X), int32(wp.Y), int32(wp.Z)) {
            return true
        }
    }

    return false
}

// nextClearWaypoint scans the plan ahead for the first waypoint the
// standing cell can click directly: the stuck skip must only arm
// targets the server walks, never a line it would collapse partway
// into a trap cell (the segmentAdvanceClear gate of the cursor advance).
// It returns the index of the first clear successor, or the current
// cursor when no successor ahead is reachable.
func (l *Loop) nextClearWaypoint(selfX, selfY, selfZ int32) int {
    for next := l.wpIndex + 1; next < len(l.waypoints); next++ {
        if l.segmentAdvanceClear(selfX, selfY, selfZ, next) {
            return next
        }
    }

    return l.wpIndex
}

// stuckSelfZ returns the current character z for the stuck recovery:
// the recovery needs the full position and the stuck tracker only
// carries x and y, so the z comes from the tracker on demand (0 when
// the position is not known yet - the planner resolves the layer of
// the standing cell anyway).
func (l *Loop) stuckSelfZ() int32 {
    _, _, z, ok := l.tracker.SelfPosition()
    if !ok {
        return 0
    }

    return z
}
