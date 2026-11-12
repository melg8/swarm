// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"
)

// cursorEscapeState carries the armed cursor key escape: the origin
// the refusal verdict stood at, the claimed dry steps toward the
// validated hop aim, the claim cursor and the follow bookkeeping
// (see beginCursorKeyEscape).
type cursorEscapeState struct {
    armed   bool
    originX int32
    originY int32
    steps   [][3]int32
    // wpMap names the route waypoint every claimed step completes
    // (the index into the segment's waypoints, -1 for the mid segment
    // strides): the WASD walk IS the ground progress, the waypoint
    // the claims walk onto marks passed (the owner report of the
    // 2026-09-19 17:18 dump - the wasd walked points must never be
    // walked again by the resumed clicks). Nil for the planless
    // fallback whose straight ladder owns no route waypoints.
    wpMap []int
    next  int
    // lastClaimAt paces the claims at the official client cadence.
    lastClaimAt time.Time
    // claimsSinceMove counts the claims since the last observed
    // server position change: the follow probe of the escape.
    claimsSinceMove int
}

// zeroCursorEscape returns the cleared escape state of the segment and
// trip boundaries: every field explicit (the exhaustruct convention
// of the repository).
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

// beginCursorKeyEscape arms the cursor key escape of a click
// refusing cell: the routed walk clicks bounced with ActionFailed
// while the character stood still, and the 2026-09-14 15:10 report
// proved the user's server answers NO mouse click from such a cell
// at all - even the official client stood frozen on the village
// plaza cell until the player walked it out with the ARROW KEYS.
// The escape emulates exactly that movement: the keyboard-mode move
// request (MoveToLocation movement mode 0) arms the server's cursor
// key handling, and the claimed ValidatePosition stream the official
// client drives from its own movement simulation moves the character
// server-side without any click validation (ValidatePosition.runImpl
// syncs the claimed placement into the world and broadcasts it). The
// claimed steps follow the planned route when the segment holds one (the
// route ladder bends where the plan bends - see
// cursorEscapeRouteSteps) and fall back to the straight line toward
// the aim without a plan, one run-speed step per second - the aim
// clamped into the pocket radius, so a far target can never pull a
// straight march out of the planless escape (the owner rule of the
// 2026-09-19 round: NEVER walk the direct line). It reports whether
// the escape armed; a server that ignores the claims burns the
// attempts (see driveCursorKeyEscape) and the caller keeps its
// honest abort.
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
        // The planless fallback: the straight ladder toward the aim,
        // pocket sized. A far aim is clamped to the pocket radius
        // along its line - the escape walks the first stretch toward
        // the target and the re-plans of the settle own the rest. The
        // aim z rides the frame offset first: the interpolation holds
        // only between two z of one frame, and the raw waypoint z is
        // the pack frame (see cursorEscapeRouteSteps).
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
        // No stride fits the budget toward the aim: the honest abort
        // of the caller stands.
        return false
    }
    // The ground re-anchor: the cursor sits on the first waypoint the
    // character's ground owes - the claims own the segment now and the
    // bookkeeping bet of the click ladder (the forward jump, the skip
    // chain) is void the moment the escape arms. The marks of the
    // claimed strides advance the cursor from here as the ground is
    // walked, the settle's own advance reconciles it with the arrival
    // test, and the plan view the web UI serves during the escape
    // shows the walked prefix instead of the jumped cursor.
    l.wpIndex = marchStart
    // The arm request aims the ladder's far end: the mode 0 move
    // starts the server side walk the claims then own, and a target
    // on the character's own cell would answer the stopMove refusal
    // before the flag ever latches (the fresh plan's first waypoint
    // is the standing cell itself).
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

// cursorEscapeRouteSteps builds the claimed steps of a route
// following escape: the planned waypoints from the character's ground
// position (the pathfind route the segment already holds)
// interpolated into run-speed strides - the claims follow the plan
// the mesh priced, wet strides included. The strides march
// every segment of the route in order, so the ladder bends where the
// plan bends: an obstacle the straight chord would push the
// character through (the tree on the plaza, the railing corner) is
// walked around the way the planner drew it. Every step z rides the
// segment's frame offset into the server frame (see click_frame.go):
// the claims name the placement the server owns, and a raw pack z
// would place the character below its own ground on every shifted
// cell - the server correction snaps it back and the escape walks
// the character nowhere (the farm readiness round of 2026-09-20:
// the porch claims rode the pack z 64 units under the live ground).
// The route length caps
// at cursorEscapeRouteMax so one escape stays a pocket recovery -
// the claims never walk the character across the whole map. It
// returns the steps with their waypoint map (the route waypoint
// every step completes, -1 for the mid segment strides - the WASD
// ground progress of the drive, see markEscapeClaimedWaypoint) and
// nil maps without a plan (the straight fallback owns the segment).
//
// The march starts at the first waypoint the CHARACTER's ground still
// owes (see escapeMarchStart), never blindly at the plan cursor: the
// cursor may sit AHEAD of the walked ground - the click ladder bets
// far waypoints (the forward jump of a refused click validates a
// longer chord and jumps the cursor onto it, the stuck skip jumps
// onto the first clear successor) and a server that refuses the bet
// leaves the cursor ahead while the character never moved. Marching
// from the jumped cursor would claim the straight chord from the
// standing cell to the far waypoint - off the route the planner drew
// and through whatever the chord cuts. The claims walk the ROUTE
// from the character's own ground: the stride sequence re-enters the
// plan at the first un-walked bend and every segment from there is
// the planner's own geometry.
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
        // The waypoint z anchors into the server frame BEFORE the
        // interpolation: the march interpolates between the character
        // z (the server frame) and the waypoint z, and a raw pack
        // waypoint z would blend the two frames mid segment - the
        // claimed stride then drifts below the live ground by half
        // the shift before the end of the first stretch.
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
            // A stride that lands within the coincide radius of the
            // segment's waypoint completes it (the closing step
            // below is then skipped).
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
        // Close the segment onto the waypoint itself when the last
        // stride ended short of it: the bend points stay on the
        // route, the next segment leaves from the route bend and
        // not from a corner the stride cut.
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

// escapeMarchStart answers the index of the first plan waypoint the
// character's ground still owes: the first waypoint that is neither
// arrived at (the intermediate pass radius - the walked ground
// contract of the arrival test) nor passed along the route (the
// projection test of advanceWaypoints). The plan cursor itself can
// sit ahead of this index - the forward jump and the stuck skip bet
// far waypoints whose clicks the offline port validates, and a server
// that refuses them leaves the bookkeeping ahead of the ground. The
// escape march trusts the ground: the claims re-enter the plan at the
// first un-walked bend and the marks (markEscapeClaimedWaypoint) pull
// the cursor back onto the walked prefix as the strides land.
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

// cursorEscapeSteps builds the claimed steps of the cursor key
// escape: the straight line from the standing cell toward the
// validated hop aim, interpolated into run-speed steps - the claims
// follow the straight fallback exactly as planned. The steps stop at
// the aim.
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
    // The aim itself closes the ladder when it is not already the
    // last interpolated step.
    if len(steps) == 0 ||
        steps[len(steps)-1][0] != aimX || steps[len(steps)-1][1] != aimY {
        steps = append(steps, [3]int32{aimX, aimY, aimZ})
    }

    return steps
}

// driveCursorKeyEscape advances the armed cursor key escape by one
// decision: the claims pace at the official client cadence (one
// run-speed step per second), the follow probe watches the server
// position - the server that follows the claims moves the character
// and the escape runs its ladder to the aim, the server that ignores
// them (the keyboard movement disabled, a build without the cursor
// key branch of ValidatePosition) burns the follow patience and the
// escape ends with the honest log line for the caller's abort
// ladder. The settle window after the last claim lets the position
// broadcasts land before the segment resumes its clicks.
func (l *Loop) driveCursorKeyEscape(
    now time.Time, selfX, selfY int32,
) {
    // The follow probe: any drift past the margin from the escape
    // origin re-arms the patience (the character moved - the claims
    // own it).
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
    // The claim pacing: one run-speed step per second, the official
    // client cadence of the ValidatePosition stream.
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
        // The official client renders the arrow walk facing from its
        // own movement simulation - the claim facing IS the client
        // side truth while the keyboard movement owns the stream
        // (the server echo carries the arm heading instead, see
        // ApplySelfFacing).
        l.tracker.ApplySelfFacing(heading)
        l.cursorEscape.next++
        l.cursorEscape.lastClaimAt = now
        l.cursorEscape.claimsSinceMove++
        // The WASD ground progress: the claims of a FOLLOWED escape
        // mark the route waypoints passed (the server position moved
        // - the claims own the character, the walked points stay
        // passed for the resumed clicks). The claims of an ignored
        // escape mark nothing - the plan cursor never fakes the
        // ground the character never walked, the abort ladder of the
        // ignoring server stays honest.
        if l.escapeFollows(selfX, selfY) {
            for k := range l.cursorEscape.next {
                l.markEscapeClaimedWaypoint(k)
            }
        }

        return
    }
    // The settle message names the walk that resumes: the follower
    // mode resumes the planned waypoint clicks on the same plan the
    // claims just walked (the owner contract: the normal mode at the
    // point).
    if now.Sub(l.cursorEscape.lastClaimAt) >= cursorEscapeSettle {
        aim := l.cursorEscape.steps[len(l.cursorEscape.steps)-1]
        l.cursorEscape.armed = false
        // The WASD ground becomes the plan progress: the position the
        // character ACTUALLY reached advances the cursor (the arrival
        // radius and the route projection of the real ground) - the
        // resumed clicks aim the first waypoint still ahead, never
        // back to the walked ones (the owner report of the 2026-09-19
        // 17:18 dump: the walk returned to the wasd walked points and
        // burned two minutes skipping them). The ladder's aim itself
        // is never trusted here: a server that ignored the claims
        // left the character on the origin ground, and progress the
        // server never delivered must never complete the plan.
        if sx, sy, sz, ok := l.tracker.SelfPosition(); ok {
            l.advanceWaypoints(sx, sy, sz)
        }
        // The escape moved the character server side: the stuck
        // window re-baselines from the settle ground - the frozen
        // verdict of the pre escape ground must not fire on the
        // resumed clicks (the dump: the skip fired one second after
        // the resume).
        l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
        l.logf("Hunt: the cursor key escape walked to %d %d, "+
            "resuming the planned walk clicks", aim[0], aim[1])
    }
}

// escapeFollows reports whether the server follows the claimed stream
// of the running escape: the character position drifted past the
// follow margin from the escape origin (the same verdict the follow
// probe of the drive applies). The ground progress marking rides this
// verdict - a server that ignores the claims never moves the
// character, and progress it never delivered must never mark the
// plan.
func (l *Loop) escapeFollows(selfX, selfY int32) bool {
    return math.Hypot(
        float64(selfX-l.cursorEscape.originX),
        float64(selfY-l.cursorEscape.originY),
    ) > cursorEscapeFollowStep
}

// markEscapeClaimedWaypoint advances the plan cursor past the route
// waypoint the claimed step completed: the WASD walk IS the ground
// progress, the waypoint the claims walked onto counts as passed
// (the owner report: the points the wasd walked must show passed and
// the resumed clicks must never walk back to them). No line gate
// stands here on purpose: the claim ladder follows the planner's own
// bends, the ground the claim landed on IS the route ground - the
// conservative advance gates would re-introduce the exact backtrack
// the report pinned (the walked prefix marked unpassed because the
// line test ran from the walked ground to a waypoint behind it).
// The mark is UNCONDITIONAL about the cursor it finds: the cursor may
// sit AHEAD of the walked ground (the forward jump bet a far
// waypoint's chord and the server refused the click, the skip chain
// ran ahead while the character stood) - the claim's ground progress
// is the honest authority and the cursor follows it, the bookkeeping
// bet never keeps a waypoint the claims have not walked marked
// passed (the resumed clicks would aim the jumped far waypoint
// again and re-lose the ground the escape just gained).
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

// cursorEscapeHeading renders the L2 heading of a step direction in
// the convention of the reference Mobius master
// (LocationUtil.calculateHeadingFrom: the degrees of atan2(deltaY,
// deltaX) scaled by 65536 over 360, so east is 0, south 16384, west
// 32768 and north 49152) - the same convention the state tracker
// applies to the movement broadcasts (state.HeadingFromDelta). The
// swapped atan2 arguments this function carried before the
// 2026-09-19 17:18 report mirrored the facing (the character walked
// west while the web UI drew the south line) and the mirror is not
// only cosmetic: the mobius cursor key movement probes its obstacle
// front along the heading (Creature.updatePosition), the mirrored
// claims probe behind the character's back.
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
