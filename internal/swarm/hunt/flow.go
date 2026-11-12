// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The continuous waypoint flow: the click target of both ground walk
// followers spans several route waypoints, so the character never
// reaches the aimed point before the next request re-aims farther.
// The server replaces the destination of a running move without
// stopping the character, but every reached click target stops it
// (the zero distance arrival broadcast), and a follower that aims
// only the current waypoint stops on every dense route point of a
// smoothed plan. The flow scan picks the farthest route waypoint the
// server click transport delivers inside the aim span, so the
// character cuts the corners the validation allows, keeps the bends
// it walls, and runs through the rest without an arrival stop.

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// flowDefaultRunSpeed is the run speed assumed before the server
// sends the UserInfo character data.
const flowDefaultRunSpeed = 120.0

// flowAim returns the click span of the continuous follower: the
// ground two request periods of the measured run speed cover, floored
// for the pre UserInfo clicks and capped at the server move limit so
// the click is never clipped short of the aim.
func (l *Loop) flowAim() float64 {
    speed := l.tracker.SelfRunSpeed()
    if speed <= 0 {
        speed = flowDefaultRunSpeed
    }
    aim := speed * (flowAimPeriods * walkRequestPeriod.Seconds())
    if aim < flowAimFloor {
        aim = flowAimFloor
    }
    if aim > maxMoveDistance {
        aim = maxMoveDistance
    }

    return aim
}

// flowOnRoute reports whether the click chord to the candidate stays
// on the drawn route: every skipped waypoint must project inside the
// chord within the route corridor and its anchored height must stay
// within flowLayerTolerance of the chord height. The server click
// validation alone allows a chord across a switchback or a priced
// water dip the plan walks around; the corridor keeps the flow a
// shortcut of the planned route, never a different route.
func flowOnRoute(
    waypoints []pathfind.Vec3, index, next int, frameOffset float64,
    selfX, selfY, selfZ int32,
) bool {
    fromX, fromY := float64(selfX), float64(selfY)
    dx := waypoints[next].X - fromX
    dy := waypoints[next].Y - fromY
    length := math.Hypot(dx, dy)
    if length < 1 {
        return false
    }
    toZ := anchorZToServerFrame(waypoints[next].Z, frameOffset)
    for j := index; j < next; j++ {
        wp := waypoints[j]
        relX, relY := wp.X-fromX, wp.Y-fromY
        along := (relX*dx + relY*dy) / length
        if along < 0 {
            // The cursor itself may sit behind the character (a
            // pinned waypoint the route samples walked past): the
            // flow serves the forward march, the cursor is no
            // intermediate route point then. A later waypoint
            // behind the chord is a switchback the flow must not
            // cut.
            if j == index {
                continue
            }

            return false
        }
        if along > length {
            return false
        }
        lateral := math.Abs(relX*dy-relY*dx) / length
        if lateral > waypointCorridor {
            return false
        }
        chordZ := float64(selfZ) + (toZ-float64(selfZ))*(along/length)
        if math.Abs(anchorZToServerFrame(wp.Z, frameOffset)-chordZ) >
            flowLayerTolerance {
            return false
        }
    }

    return true
}

// flowAdvance moves the waypoint cursor onto the farthest route
// waypoint the click transport delivers from the standing cell. The
// scan bounds the route arc by flowAim and probes the farthest
// candidates first, so the character cuts the corners the server's
// own click validation allows and keeps the planned bends it walls.
// A cursor jump only lands on a validated line and the skip
// bookkeeping denies a repeat jump from a cell the character never
// left (the dead click transport, see skipMoveFresh). It reports the
// cursor to use: the input index when no farther target validates.
func (l *Loop) flowAdvance(
    waypoints []pathfind.Vec3, index int, frameOffset float64,
    selfX, selfY, selfZ int32,
) int {
    if l.navigator == nil || index < 0 || index >= len(waypoints) {
        return index
    }
    if !l.skipMoveFresh(selfX, selfY) {
        // The previous cursor jump left the character standing on this
        // cell: the click transport is dead here and the recovery
        // ladder owns the walk.
        return index
    }
    aim := l.flowAim()
    wp := waypoints[index]
    base := math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY))
    if base > aim {
        // The current target already lies past the click span: the
        // character cannot reach it before the next request re-aims.
        return index
    }
    count := 0
    arc := base
    for i := index; i+1 < len(waypoints); i++ {
        arc += math.Hypot(
            waypoints[i+1].X-waypoints[i].X,
            waypoints[i+1].Y-waypoints[i].Y)
        if arc > aim {
            break
        }
        count++
    }
    if count == 0 {
        return index
    }
    stride := count / flowProbeMax
    if stride < 1 {
        stride = 1
    }
    probes := 0
    for k := count - 1; k >= 0 && probes < flowProbeMax; k -= stride {
        probes++
        next := index + 1 + k
        if !flowOnRoute(waypoints, index, next, frameOffset,
            selfX, selfY, selfZ) {
            continue
        }
        if !l.routeAdvanceClear(waypoints, next, frameOffset,
            selfX, selfY, selfZ) {
            continue
        }
        l.noteSkipStand(selfX, selfY)

        return next
    }

    return index
}

// flowPreClick re-aims a running walk before the character reaches
// the current click target: within flowCornerLead of it the follower
// scans for a farther validated target and sends the click at once,
// so the server replaces the destination mid-move instead of
// stopping the character on the arrival. The pre-click stays off
// when the character is not walking (a dead click owns the stuck
// ladder) and paces both the click and the scan at flowPreClickGap.
// It reports whether a click went out.
func (l *Loop) flowPreClick(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) ||
        !l.tracker.SelfWalking() {
        return false
    }
    if !l.moveAt.IsZero() && now.Sub(l.moveAt) < flowPreClickGap {
        return false
    }
    if !l.flowScanAt.IsZero() && now.Sub(l.flowScanAt) < flowPreClickGap {
        return false
    }
    wp := l.waypoints[l.wpIndex]
    if math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY)) >
        flowCornerLead {
        return false
    }
    l.flowScanAt = now
    next := l.flowAdvance(l.waypoints, l.wpIndex,
        l.segmentFrameOffset, selfX, selfY, selfZ)
    if next == l.wpIndex {
        return false
    }
    l.wpIndex = next
    l.moveAt = time.Time{}
    l.clickWaypoint(selfX, selfY, selfZ, now)

    return true
}
