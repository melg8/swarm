// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The continuous waypoint flow: the scan picks the farthest route waypoint
// the server click transport delivers inside the aim span, so the character
// cuts the corners the validation allows and never stops on a dense route
// point of a smoothed plan (docs/hunting.md).

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// flowDefaultRunSpeed is the run speed assumed before the server
// sends the UserInfo character data.
const flowDefaultRunSpeed = 120.0

// flowAim returns the click span: the ground the request periods of the
// measured run speed cover, floored before the UserInfo and capped at the
// server move limit.
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

// flowOnRoute reports whether the click chord stays on the drawn route:
// every skipped waypoint projects inside the chord within waypointCorridor
// and its anchored height stays within flowLayerTolerance of the chord. The
// click validation alone would allow a switchback cut or a priced water dip;
// this gate keeps the flow a shortcut of the planned route.
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
            // The cursor itself may sit behind the character (a pinned
            // waypoint walked past); a later waypoint behind the chord is a
            // switchback the flow must not cut.
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

// flowAdvance moves the waypoint cursor onto the farthest route waypoint
// the click transport delivers from the standing cell: the scan bounds the
// arc by flowAim and probes the farthest candidates first. A jump lands only
// on a validated line, and the skip bookkeeping denies a repeat jump from a
// cell the character never left (the dead click transport, see
// skipMoveFresh). It returns the cursor to use, the input index when no
// farther target validates.
func (l *Loop) flowAdvance(
    waypoints []pathfind.Vec3, index int, frameOffset float64,
    selfX, selfY, selfZ int32,
) int {
    if l.navigator == nil || index < 0 || index >= len(waypoints) {
        return index
    }
    if !l.skipMoveFresh(selfX, selfY) {
        // The previous jump left the character on this cell: the click
        // transport is dead here and the recovery ladder owns the walk.
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

// flowPreClick re-aims a running walk within flowCornerLead of the current
// click target, so the server replaces the destination mid-move instead of
// stopping the character on the arrival. It stays off while the character is
// not walking and paces the click and the scan at flowPreClickGap; it
// reports whether a click went out.
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
