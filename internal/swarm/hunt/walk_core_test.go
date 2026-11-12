// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/stretchr/testify/require"
)

// TestWalkClickSegmentClipsOntoTheMoveLimit pins the shared click
// clip: a target inside the limit goes out as is, a longer segment is
// cut to the limit along the line with the z interpolated.
func TestWalkClickSegmentClipsOntoTheMoveLimit(t *testing.T) {
    tests := []struct {
        name    string
        toX     float64
        toY     float64
        toZ     float64
        limit   float64
        wantX   float64
        wantY   float64
        wantZ   float64
        wantLen float64
    }{
        {
            name: "target inside the limit", toX: 300, toY: 400,
            toZ: 100, limit: 1000, wantX: 300, wantY: 400, wantZ: 100,
            wantLen: 500,
        },
        {
            name: "target on the limit", toX: 300, toY: 400, toZ: 100,
            limit: 500, wantX: 300, wantY: 400, wantZ: 100,
            wantLen: 500,
        },
        {
            name: "target beyond the limit", toX: 3000, toY: 4000,
            toZ: 500, limit: 1000, wantX: 600, wantY: 800, wantZ: 100,
            wantLen: 1000,
        },
        {
            name: "flat target keeps the height", toX: 300, toY: 400,
            toZ: 0, limit: 250, wantX: 150, wantY: 200, wantZ: 0,
            wantLen: 250,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            x, y, z := walkClickSegment(
                0, 0, 0, tt.toX, tt.toY, tt.toZ, tt.limit)
            require.InDelta(t, tt.wantX, x, 1e-9)
            require.InDelta(t, tt.wantY, y, 1e-9)
            require.InDelta(t, tt.wantZ, z, 1e-9)
            require.InDelta(t, tt.wantLen, math.Hypot(x, y), 1e-9,
                "the clipped click ends on the limit ring")
        })
    }
}

// TestWalkClickSegmentKeepsTheLine pins the direction of the clipped
// click: the cap keeps the line towards the target, only its length
// changes.
func TestWalkClickSegmentKeepsTheLine(t *testing.T) {
    fromX, fromY := -500.0, 250.0
    x, y, _ := walkClickSegment(-500, 250, 0, 1500, -750, 0, 1000)
    // The direction (-500, 250) -> (1500, -750) is (2, -1) over 2236
    // units; the cap leaves 1000 of them from the standing point.
    require.InDelta(t, 1000, math.Hypot(x-fromX, y-fromY), 1e-9)
    cross := (x-fromX)*(-1) - (y-fromY)*2
    require.InDelta(t, 0, cross, 1e-9, "the click stays on its line")
}

// TestAdvanceWalkCursor pins the shared cursor advance: the reached
// and the passed-along-route waypoints move the cursor, a clear
// callback gates the step, and a waypoint ahead holds the cursor.
func TestAdvanceWalkCursor(t *testing.T) {
    route := []pathfind.Vec3{
        {X: 0, Y: 0, Z: 0},
        {X: 0, Y: 1000, Z: 0},
    }
    tests := []struct {
        name      string
        waypoints []pathfind.Vec3
        index     int
        selfX     int32
        selfY     int32
        final     float64
        gate      func(next int) bool
        want      int
        wantMoved bool
    }{
        {
            name: "empty plan", waypoints: nil, index: 0, want: 0,
        },
        {
            name:      "reached intermediate waypoint advances",
            waypoints: route, index: 0, selfX: 0, selfY: 40,
            final: waypointArriveDist, want: 1, wantMoved: true,
        },
        {
            name:      "waypoint passed on the route advances",
            waypoints: route, index: 0, selfX: 0, selfY: 80,
            final: waypointArriveDist, want: 1, wantMoved: true,
        },
        {
            name:      "waypoint ahead holds the cursor",
            waypoints: route, index: 0, selfX: 0, selfY: -60,
            final: waypointArriveDist, want: 0,
        },
        {
            name:      "the clear gate blocks the step",
            waypoints: route, index: 0, selfX: 0, selfY: 40,
            final: waypointArriveDist,
            gate:  func(int) bool { return false },
            want:  0,
        },
        {
            name:      "the final waypoint closes the plan",
            waypoints: route, index: 1, selfX: 0, selfY: 860,
            final: waypointArriveDist, want: 2, wantMoved: true,
        },
        {
            name:      "a tight final radius keeps the last walk",
            waypoints: route, index: 1, selfX: 0, selfY: 860,
            final: waypointPassDist, want: 1,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, moved := advanceWalkCursor(tt.waypoints, tt.index,
                tt.selfX, tt.selfY, 0, 0, tt.final, tt.gate)
            require.Equal(t, tt.want, got)
            require.Equal(t, tt.wantMoved, moved)
        })
    }
}
