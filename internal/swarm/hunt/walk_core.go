// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The shared click and cursor core of the walkers: every walk of the
// hunt package - the town trips, the manual walks, the quest trips,
// the zone returns, the delevel walks and the fight approaches -
// clips its ground click onto one server move request and advances
// its plan cursor by the same geometry.
package hunt

import (
    "math"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// walkClickSegment clips a ground click onto one server move request:
// a segment farther than the limit is cut to the straight piece the
// server accepts, the z interpolated along the same line. The point
// comes back in world coordinates, the caller converts it to the
// packet integers.
func walkClickSegment(
    fromX, fromY, fromZ int32, toX, toY, toZ, limit float64,
) (clickX, clickY, clickZ float64) {
    dx := toX - float64(fromX)
    dy := toY - float64(fromY)
    dist := math.Hypot(dx, dy)
    if dist <= limit {
        return toX, toY, toZ
    }
    frac := limit / dist

    return float64(fromX) + dx*frac, float64(fromY) + dy*frac,
        float64(fromZ) + (toZ-float64(fromZ))*frac
}

// advanceWalkCursor walks the waypoint cursor forward as far as the
// character position allows and reports the new cursor plus whether
// it moved: a waypoint counts as passed when it is reached within its
// radius or already bypassed along the route AND the straight line
// from the standing cell to the successor waypoint is clear. The
// gate callback is the walker's own line test (nil when it has
// none); a gated waypoint stays the target, walking onto it re-opens
// the line.
func advanceWalkCursor(
    waypoints []pathfind.Vec3, index int, selfX, selfY, selfZ int32,
    frameOffset, finalArrive float64, gate func(next int) bool,
) (int, bool) {
    moved := false
    for index < len(waypoints) {
        arrived := waypointArrived(waypoints, index, selfX, selfY,
            selfZ, frameOffset, finalArrive)
        passed := !arrived && index+1 < len(waypoints) &&
            waypointPassed(waypoints[index], waypoints[index+1],
                selfX, selfY)
        if !arrived && !passed {
            break
        }
        if gate != nil && !gate(index+1) {
            break
        }
        index++
        moved = true
    }

    return index, moved
}
