// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"
)

// cursorEscapeState carries the armed cursor key escape: the origin the refusal
// verdict stood at, the claimed dry steps, the claim cursor and the follow
// bookkeeping (see beginCursorKeyEscape).
type cursorEscapeState struct {
    armed   bool
    originX int32
    originY int32
    steps   [][3]int32
    // wpMap names the route waypoint every claimed step completes (the segment
    // waypoint index, -1 for a mid segment stride): the waypoint the claims
    // walk onto marks passed, so the resumed clicks never walk it again. Nil
    // for the planless fallback whose straight ladder owns no waypoints.
    wpMap []int
    next  int
    // lastClaimAt paces the claims at the official client cadence.
    lastClaimAt time.Time
    // claimsSinceMove counts the claims since the last observed move.
    claimsSinceMove int
}

// zeroCursorEscape returns the cleared escape state of the segment and trip
// boundaries; every field is explicit (the exhaustruct convention).
func zeroCursorEscape() cursorEscapeState {
    return cursorEscapeState{
        armed:           false,
        originX:         0,
        originY:         0,
        steps:           nil,
        wpMap:           nil,
        next:            0,
        lastClaimAt:     time.Time{},
        claimsSinceMove: 0,
    }
}

// beginCursorKeyEscape arms the cursor key escape of a click refusing cell: the
// routed clicks bounced with ActionFailed while the character stood still, and
// the server answers no mouse click from such a cell - only the ARROW KEYS walk
// out (docs/hunting.md). The keyboard-mode move request (movement mode 0) arms
// the cursor key handling and the claimed ValidatePosition stream moves the
// character without click validation, along the planned route or the clamped
// straight line, one run-speed step per second.
func (l *Loop) beginCursorKeyEscape(
    selfX, selfY, selfZ, aimX, aimY, aimZ int32,
) bool {
    if l.cursorEscapes >= cursorEscapeAttemptsMax {
        return false
    }
    marchStart := l.escapeMarchStart(selfX, selfY, selfZ)
    steps, wpMap := l.cursorEscapeRouteSteps(
        selfX, selfY, selfZ, marchStart)
    form := "along the planned route"
    if len(steps) == 0 {
        // The planless fallback: the straight ladder toward the aim, pocket
        // sized. A far aim is clamped to the pocket radius along its line, and
        // the aim z rides the frame offset first (the interpolation holds only
        // between two z of one frame, see cursorEscapeRouteSteps).
        aimZ = int32(math.Round(
            anchorZToServerFrame(float64(aimZ), l.segmentFrameOffset)))
        dx := float64(aimX - selfX)
        dy := float64(aimY - selfY)
        if dist := math.Hypot(dx, dy); dist > cursorEscapeRouteMax {
            frac := cursorEscapeRouteMax / dist
            aimX = selfX + int32(dx*frac)
            aimY = selfY + int32(dy*frac)
        }
        steps = l.cursorEscapeSteps(selfX, selfY, selfZ,
            aimX, aimY, aimZ)
        wpMap = nil
        form = "toward the aim"
    }
    if len(steps) == 0 {
        // No stride fits the budget toward the aim: the honest abort stands.
        return false
    }
    // The ground re-anchor: the cursor sits on the first waypoint the character
    // ground owes - the claims own the segment now and the click ladder bet
    // (the forward jump, the skip chain) is void. The claimed stride marks
    // advance the cursor as the ground is walked and the view shows the prefix.
    l.wpIndex = marchStart
    // The arm request aims the ladder's far end: the mode 0 move starts the
    // server side walk the claims then own - a target on the own cell would
    // answer the stopMove refusal before the flag ever latches.
    arm := steps[len(steps)-1]
    l.cursorEscapes++
    l.cursorEscape = cursorEscapeState{
        armed:           true,
        originX:         selfX,
        originY:         selfY,
        steps:           steps,
        wpMap:           wpMap,
        next:            0,
        lastClaimAt:     time.Time{},
        claimsSinceMove: 0,
    }
    if err := l.game.CursorKeyWalkTo(arm[0], arm[1], arm[2]); err != nil {
        l.logf("Hunt: the cursor key arm failed: %v", err)
    }
    l.logf("Hunt: the refused clicks hand the walk to the cursor key "+
        "escape, walking %s toward %d %d (%d claimed steps)",
        form, arm[0], arm[1], len(steps))

    return true
}

// cursorEscapeRouteSteps builds the claimed steps of a route following escape:
// the planned waypoints interpolated into run-speed strides, so the ladder
// bends where the plan bends. Every step z rides the segment frame offset into
// the server frame (a raw pack z would place the character below its ground).
// The route length caps at cursorEscapeRouteMax, so one escape stays a pocket
// recovery. It returns the steps and their waypoint map, and the march starts
// at the first waypoint the ground still owes (escapeMarchStart).
func (l *Loop) cursorEscapeRouteSteps(
    selfX, selfY, selfZ int32, start int,
) ([][3]int32, []int) {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) ||
        start >= len(l.waypoints) {
        return nil, nil
    }
    steps := make([][3]int32, 0, 16)
    wpMap := make([]int, 0, 16)
    px, py, pz := float64(selfX), float64(selfY), float64(selfZ)
    budget := cursorEscapeRouteMax
    for i := start; i < len(l.waypoints); i++ {
        wp := l.waypoints[i]
        // The waypoint z anchors into the server frame before the
        // interpolation: a raw pack z would blend the two frames mid segment
        // and drift the claim below the live ground.
        wpZ := anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
        ox, oy, oz := px, py, pz
        dx := wp.X - ox
        dy := wp.Y - oy
        dz := wpZ - oz
        dist := math.Hypot(dx, dy)
        stride := cursorEscapeStep
        for stride < dist && budget > 0 {
            frac := stride / dist
            sx := ox + dx*frac
            sy := oy + dy*frac
            sz := oz + dz*frac
            // A stride landing within the coincide radius completes the
            // waypoint (the closing step below is then skipped).
            done := -1
            if math.Hypot(wp.X-sx, wp.Y-sy) <= hopCoincideDist {
                done = i
            }
            steps = append(steps, [3]int32{
                int32(math.Round(sx)),
                int32(math.Round(sy)),
                int32(math.Round(sz)),
            })
            wpMap = append(wpMap, done)
            px, py, pz = sx, sy, sz
            budget -= cursorEscapeStep
            stride += cursorEscapeStep
        }
        if budget <= 0 {
            return steps, wpMap
        }
        // Close the segment onto the waypoint when the last stride ended short
        // of it: the next segment leaves from the route bend, not a cut corner.
        if math.Hypot(wp.X-px, wp.Y-py) > hopCoincideDist {
            steps = append(steps, [3]int32{
                int32(math.Round(wp.X)),
                int32(math.Round(wp.Y)),
                int32(math.Round(wpZ)),
            })
            wpMap = append(wpMap, i)
            px, py, pz = wp.X, wp.Y, wpZ
            budget -= cursorEscapeStep
        }
    }

    return steps, wpMap
}

// escapeMarchStart answers the index of the first plan waypoint the character's
// ground still owes: neither arrived at (the intermediate pass radius) nor
// passed along the route (the projection test). The plan cursor itself can sit
// ahead - the forward jump and the stuck skip bet far waypoints - and the marks
// pull it back onto the walked prefix as the strides land.
func (l *Loop) escapeMarchStart(selfX, selfY, selfZ int32) int {
    for i := range l.waypoints {
        if waypointArrived(l.waypoints, i, selfX, selfY, selfZ,
            l.segmentFrameOffset, waypointPassDist) {
            continue
        }
        if i+1 < len(l.waypoints) &&
            waypointPassed(l.waypoints[i], l.waypoints[i+1], selfX, selfY) {
            continue
        }

        return i
    }

    return len(l.waypoints)
}

// cursorEscapeSteps builds the claimed steps of the straight fallback: the line
// from the standing cell toward the aim interpolated into run-speed steps.
func (l *Loop) cursorEscapeSteps(
    selfX, selfY, selfZ, aimX, aimY, aimZ int32,
) [][3]int32 {
    total := math.Hypot(float64(aimX-selfX), float64(aimY-selfY))
    if total < 1 {
        return nil
    }
    var steps [][3]int32
    for walked := cursorEscapeStep; walked < total; walked += cursorEscapeStep {
        frac := walked / total
        stepX := int32(float64(selfX) +
            float64(aimX-selfX)*frac)
        stepY := int32(float64(selfY) +
            float64(aimY-selfY)*frac)
        stepZ := int32(float64(selfZ) +
            float64(aimZ-selfZ)*frac)
        steps = append(steps, [3]int32{stepX, stepY, stepZ})
    }
    // The aim itself closes the ladder when not already the last step.
    if len(steps) == 0 ||
        steps[len(steps)-1][0] != aimX || steps[len(steps)-1][1] != aimY {
        steps = append(steps, [3]int32{aimX, aimY, aimZ})
    }

    return steps
}

// driveCursorKeyEscape advances the armed escape by one decision: the claims
// pace at the official client cadence (one run-speed step per second) and the
// follow probe watches the server position. A server that follows moves the
// character and the ladder runs to the aim; one that ignores the claims burns
// the follow patience and the escape ends for the caller's abort ladder. The
// settle window after the last claim lets the broadcasts land.
func (l *Loop) driveCursorKeyEscape(
    now time.Time, selfX, selfY int32,
) {
    // The follow probe: a drift past the margin from the escape origin
    // re-arms the patience (the character moved - the claims own it).
    moved := math.Hypot(
        float64(selfX-l.cursorEscape.originX),
        float64(selfY-l.cursorEscape.originY))
    if moved > cursorEscapeFollowStep {
        l.cursorEscape.claimsSinceMove = 0
    } else if l.cursorEscape.claimsSinceMove >= cursorEscapeFollowClaims {
        l.cursorEscape.armed = false
        l.logf("Hunt: the cursor key escape made no progress - " +
            "the server ignores the claimed positions")

        return
    }
    // The claim pacing: one run-speed step per second, the client cadence.
    if !l.cursorEscape.lastClaimAt.IsZero() &&
        now.Sub(l.cursorEscape.lastClaimAt) < cursorEscapePeriod {
        return
    }
    if l.cursorEscape.next < len(l.cursorEscape.steps) {
        step := l.cursorEscape.steps[l.cursorEscape.next]
        heading := cursorEscapeHeading(selfX, selfY, step[0], step[1])
        if err := l.game.ClaimValidatePosition(
            step[0], step[1], step[2], heading); err != nil {
            l.logf("Hunt: the claimed position failed: %v", err)
        }
        // The claim facing IS the client side truth while the keyboard
        // movement owns the stream (the server echo carries the arm heading
        // instead, see ApplySelfFacing).
        l.tracker.ApplySelfFacing(heading)
        l.cursorEscape.next++
        l.cursorEscape.lastClaimAt = now
        l.cursorEscape.claimsSinceMove++
        // The WASD ground progress: a followed escape marks the route
        // waypoints passed, so the resumed clicks never walk them again. An
        // ignored escape marks nothing - progress never walked must not pass.
        if l.escapeFollows(selfX, selfY) {
            for k := range l.cursorEscape.next {
                l.markEscapeClaimedWaypoint(k)
            }
        }

        return
    }
    // The settle message names the walk that resumes: the follower mode resumes
    // the planned waypoint clicks on the same plan the claims just walked.
    if now.Sub(l.cursorEscape.lastClaimAt) >= cursorEscapeSettle {
        aim := l.cursorEscape.steps[len(l.cursorEscape.steps)-1]
        l.cursorEscape.armed = false
        // The WASD ground becomes the plan progress: the position the character
        // ACTUALLY reached advances the cursor, never the ladder's aim - a
        // server that ignored the claims left the character on the origin
        // ground, and progress it never delivered must never complete the plan.
        if sx, sy, sz, ok := l.tracker.SelfPosition(); ok {
            l.advanceWaypoints(sx, sy, sz)
        }
        // The escape moved the character server side: the stuck window
        // re-baselines from the settle ground, so the frozen verdict of the pre
        // escape ground cannot fire on the resumed clicks.
        l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
        l.logf("Hunt: the cursor key escape walked to %d %d, "+
            "resuming the planned walk clicks", aim[0], aim[1])
    }
}

// escapeFollows reports whether the server follows the claimed stream: the
// character position drifted past the follow margin from the escape origin (the
// same verdict the follow probe applies). A server that ignores the claims
// never moves the character, and that progress must never mark the plan.
func (l *Loop) escapeFollows(selfX, selfY int32) bool {
    return math.Hypot(
        float64(selfX-l.cursorEscape.originX),
        float64(selfY-l.cursorEscape.originY),
    ) > cursorEscapeFollowStep
}

// markEscapeClaimedWaypoint advances the plan cursor past the route waypoint
// the claimed step completed: the WASD walk IS the ground progress, so the
// waypoint the claims walked onto counts as passed and the resumed clicks never
// walk back to it. No line gate stands here on purpose - the claim ladder
// follows the planner's bends, the ground it landed on IS the route ground.
// The mark is unconditional about the cursor it finds: the claim ground
// progress is the honest authority, even when the cursor sits ahead of it.
func (l *Loop) markEscapeClaimedWaypoint(stepIndex int) {
    if stepIndex >= len(l.cursorEscape.wpMap) {
        return
    }
    wp := l.cursorEscape.wpMap[stepIndex]
    if wp < 0 {
        return
    }
    l.wpIndex = wp + 1
    l.moveAt = time.Time{}
}

// cursorEscapeHeading renders the L2 heading of a step direction in the Mobius
// convention (LocationUtil.calculateHeadingFrom: atan2(deltaY, deltaX) degrees
// scaled by 65536 over 360, east 0, south 16384, west 32768, north 49152), the
// one state.HeadingFromDelta applies. The heading matters beyond the drawing:
// the cursor key movement probes its obstacle front along it
// (Creature.updatePosition).
func cursorEscapeHeading(
    fromX, fromY, toX, toY int32,
) int32 {
    dx := float64(toX - fromX)
    dy := float64(toY - fromY)
    angle := math.Atan2(dy, dx)
    if angle < 0 {
        angle += 2 * math.Pi
    }

    return int32(angle * 65536 / (2 * math.Pi))
}
