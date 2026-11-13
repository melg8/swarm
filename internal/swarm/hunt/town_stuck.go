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
// still: the correlation is the send timestamp plus the absence of a
// later non-walk request, which could own the answer.
func (l *Loop) refusalEvidence() bool {
    if l.moveAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    return failedAt.After(l.moveAt) &&
        failedAt.Sub(l.moveAt) <= refusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.moveAt, failedAt)
}

// sendVariedAim answers a refused click by varying the aim at the same
// waypoint: the refusal is target specific, so a shorter prefix or a
// sideways offset often validates. Every variant passes the same click
// port as a planned click.
func (l *Loop) sendVariedAim(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) ||
        l.refusalVariants >= refusalVariantsMax {
        return false
    }
    wp := l.waypoints[l.wpIndex]
    // The varied aims ride the same server frame transport as the
    // plain clicks (click_frame.go).
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

// refusalVariantTarget computes the varied click target of one variant on the
// capped click geometry: variant 0 halves it, 1 quarters it, 2 and 3 bend the
// half click sideways. It returns nil when the click is too short to vary.
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

// noteMoveStart is the move start watchdog: the deadline arms with the
// click, clears on a position change or a movement broadcast, and a
// lapsed deadline on the baseline cell names the click dead. The
// window covers the once-per-second broadcast gate plus the lag.
func (l *Loop) noteMoveStart(
    now time.Time, selfX int32, selfY int32,
) bool {
    if l.moveStartAt.IsZero() {
        if l.moveAt.IsZero() || l.tracker.SelfWalking() {
            return false
        }
        l.moveStartAt = l.moveAt.Add(moveStartWindow)
        l.moveStartX, l.moveStartY = selfX, selfY
        // Fall through: the arming tick may already sit past the deadline (the
        // click went out a tick before the watchdog first saw it) - the fire
        // below must not wait another window for a verdict it can already name.
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

// walkStuck tracks the movement progress and re-paths once the character stands
// still or wobbles without net progress toward the waypoint (a pure movement
// reset misses the tangent flip-flop). The first stuck of a trip waits
// stuckTimeout (15 s), the later ones the shorter stuckFastTimeout (4 s).
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

// stuckProgressed reports whether the walk made net progress: the
// cursor advanced AND the character moved, the current waypoint came
// closer by the margin, or the segment destination did while the
// character stays on the route corridor.
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

// stuckDestinationProgressed reports whether the segment destination came
// measurably closer while the character stays inside the route corridor: an
// off-route hop that merely points destination-ward is no progress.
func (l *Loop) stuckDestinationProgressed(selfX int32, selfY int32) bool {
    if !l.stuckOnRouteCorridor(selfX, selfY) {
        return false
    }

    return l.stuckDestinationDistance(selfX, selfY) <
        l.stuckDestinationDistance(l.stuckX, l.stuckY)-stuckProgressUnits
}

// stuckOnRouteCorridor reports whether the position sits within the
// lateral corridor of the remaining route.
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

// pocketRefused reports whether the current refusal verdict stands on the very
// cell where the segment's first refusal latched: the ground the character
// stands on refuses every click, not one specific click line.
func (l *Loop) pocketRefused(selfX int32, selfY int32) bool {
    return l.segmentRefused &&
        selfX == l.segmentRefusedX && selfY == l.segmentRefusedY
}

func (l *Loop) currentWaypoint() (pathfind.Vec3, bool) {
    if l.wpIndex < 0 || l.wpIndex >= len(l.waypoints) {
        return pathfind.Vec3{X: 0, Y: 0, Z: 0}, false
    }

    return l.waypoints[l.wpIndex], true
}

// stuckTownWalk drives the town segment stuck recovery: skip the
// current waypoint when a later one has a walkable line from the
// standing cell, else re-plan the whole segment. A skip does not
// consume the re-path budget and arms the fast timeout.
//
//nolint:funlen // the town walk repeats the stuck checks per segment
func (l *Loop) stuckTownWalk(now time.Time, selfX int32, selfY int32) bool {
    // The online refusal answer separates a server side refusal from a corridor
    // freeze, and the refusal is target specific: the varied aim runs before
    // the waypoint skip, and the latch arms the pocket gate below.
    if l.refusalEvidence() {
        if !l.segmentRefused {
            l.segmentRefused = true
            l.segmentRefusedX, l.segmentRefusedY = selfX, selfY
            l.logf("Hunt: the server refused the walk click " +
                "(ActionFailed), varying the aim")
        }
        // The varied aims run first: the refusal is target specific far more
        // often than it is a pocket, one variant per stuck verdict that walks
        // the character where the plain aim bounced.
        if l.sendVariedAim(selfX, selfY, l.stuckSelfZ(), now) {
            l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
            l.stuckWP = l.wpIndex
            l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
            l.stuckFast = true

            return false
        }
        // The refusing pocket: the varied aims are spent and the
        // character still stands on the refusal cell, so the cursor key
        // escape owns the recovery.
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
        // The clear successor exists but the last skip moved nothing:
        // the re-path ladder owns the segment.
        l.logf("Hunt: town walk stuck, the last skip moved the "+
            "character nowhere (cursor %d of %d)", l.wpIndex,
            len(l.waypoints))
    }
    if l.noteRepathCell(selfX, selfY) {
        // The previous re-path started from this very cell and moved
        // nothing: the next one would plan the identical route.
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
        // The fresh plan found no route: the escalation ladder owns the
        // freeze evidence instead of aborting past it.
        l.abortFrozenTrip("re-path failed")

        return true
    }
    // The re-path proved the plain clicks of this segment do not move
    // the character: arm the 4 s fast window and re-baseline it from
    // the re-path tick.
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.stuckWP = l.wpIndex
    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
    l.stuckFast = true

    return false
}

// skipMoveFresh reports whether a waypoint skip may run: no skip yet,
// or the character moved since the last one beyond the stood-on-cell
// tolerance. A repeat skip on the same cell proved the transport dead
// and would starve the recovery ladder.
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

// noteRepathCell records the cell a re-path plans from and reports whether the
// previous one started from the same cell without movement: the deterministic
// planner would re-plan the identical route.
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

// abortFrozenTrip ends a trip whose re-path produced no movement: the
// cursor key escape takes the frozen segment over when it can, else the
// trip aborts with its cooldown.
func (l *Loop) abortFrozenTrip(reason string) {
    if l.escalateFrozenSegment() {
        return
    }
    l.abortTownTrip(reason)
}

// escalateFrozenSegment arms the cursor key escape of a frozen town walk
// segment: the claimed ValidatePosition stream walks the character ALONG THE
// PLANNED ROUTE (a click-refusing ground answers no mouse click). It reports
// whether the escape took over the recovery.
func (l *Loop) escalateFrozenSegment() bool {
    if l.navigator == nil {
        return false
    }
    // The ladder runs for both the town walk and the zone return
    // segments: both follow a planned route the transport may refuse.
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
// standing cell can click directly (the segmentAdvanceClear gate); it
// returns the current cursor when no successor is reachable.
func (l *Loop) nextClearWaypoint(selfX, selfY, selfZ int32) int {
    for next := l.wpIndex + 1; next < len(l.waypoints); next++ {
        if l.segmentAdvanceClear(selfX, selfY, selfZ, next) {
            return next
        }
    }

    return l.wpIndex
}

// stuckSelfZ returns the current character z on demand: the stuck tracker
// carries only x and y, and 0 lets the planner resolve the standing layer.
func (l *Loop) stuckSelfZ() int32 {
    _, _, z, ok := l.tracker.SelfPosition()
    if !ok {
        return 0
    }

    return z
}
