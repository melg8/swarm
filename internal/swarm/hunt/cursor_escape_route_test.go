// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// routeStepsCorridor reports the distance from a claimed step to the
// synthetic route polyline (the minimum over the segment distances):
// the route following ladder must stay on the planned line, a step
// far off it would cut the bend the planner drew around the obstacle.
func routeStepsCorridor(
    waypoints []pathfind.Vec3, self pathfind.Vec3, step [3]int32,
) float64 {
    best := math.MaxFloat64
    px, py := self.X, self.Y
    for i := range waypoints {
        wp := waypoints[i]
        best = math.Min(best,
            pointSegmentDist(float64(step[0]), float64(step[1]),
                px, py, wp.X, wp.Y))
        px, py = wp.X, wp.Y
    }

    return best
}

func pointSegmentDist(
    x, y, ax, ay, bx, by float64,
) float64 {
    dx := bx - ax
    dy := by - ay
    length := math.Hypot(dx, dy)
    if length < 1 {
        return math.Hypot(x-ax, y-ay)
    }
    t := ((x-ax)*dx + (y-ay)*dy) / (length * length)
    t = math.Max(0, math.Min(1, t))

    return math.Hypot(x-(ax+t*dx), y-(ay+t*dy))
}

// TestCursorEscapeRouteStepsFollowThePlanBend pins the route
// following claim ladder: the escape of a refusing cell walks ALONG
// the planned route (the user rule of the 2026-09-19 round - the
// claims bend where the plan bends, never a straight line across the
// map), the stride spacing matches the run-speed step and the route
// length cap keeps the escape a pocket recovery.
func TestCursorEscapeRouteStepsFollowThePlanBend(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.SetNavigator(&fakeNavigator{})
    loop.waypoints = []pathfind.Vec3{
        {X: 1000, Y: 0, Z: -3000},
        {X: 1000, Y: 1000, Z: -3000},
        {X: 0, Y: 2000, Z: -3000},
    }
    loop.wpIndex = 0

    steps, wpMap := loop.cursorEscapeRouteSteps(0, 0, -3000, 0)
    require.NotEmpty(t, steps,
        "a planned route arms a route following ladder")
    require.Len(t, wpMap, len(steps),
        "every claimed step carries its route waypoint map")
    for i, done := range wpMap {
        require.True(t, done == -1 || done >= 0 && done < len(loop.waypoints),
            "step %d maps outside the plan: %d", i, done)
    }

    self := pathfind.Vec3{X: 0, Y: 0, Z: -3000}
    walked := 0.0
    prevX, prevY := 0.0, 0.0
    for i, step := range steps {
        dist := routeStepsCorridor(loop.waypoints, self, step)
        require.LessOrEqual(t, dist, waypointCorridor,
            "step %d sits %.0f off the planned route - the ladder "+
                "must follow the plan bends", i, dist)
        hop := math.Hypot(float64(step[0])-prevX,
            float64(step[1])-prevY)
        require.LessOrEqual(t, hop, cursorEscapeStep+1,
            "step %d strides past the run-speed step", i)
        walked += hop
        prevX, prevY = float64(step[0]), float64(step[1])
    }
    require.LessOrEqual(t, walked, cursorEscapeRouteMax+cursorEscapeStep,
        "the ladder walks past the route cap")
    // The bend closure: the route bend point lands among the steps
    // (the segment leaves from the bend, not from a cut corner) and
    // the step that walks onto the bend COMPLETES the bend waypoint
    // in the map (the WASD ground progress of the drive).
    bend := loop.waypoints[0]
    found := false
    for i, step := range steps {
        if math.Hypot(float64(step[0])-bend.X,
            float64(step[1])-bend.Y) <= hopCoincideDist {
            require.Equal(t, 0, wpMap[i],
                "the step on the bend completes the bend waypoint")
            found = true

            break
        }
    }
    require.True(t, found,
        "the route bend point must appear among the claimed steps")
}

// TestCursorEscapeRouteStepsCapKeepsThePocketRecovery pins the route
// length cap: a long planned route produces a bounded ladder - the
// escape never walks the character across the whole map on claims
// alone, the clicks resume from the escaped ground.
func TestCursorEscapeRouteStepsCapKeepsThePocketRecovery(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.SetNavigator(&fakeNavigator{})
    var waypoints []pathfind.Vec3
    for i := range 12 {
        waypoints = append(waypoints, pathfind.Vec3{
            X: float64((i + 1) * 1000), Y: 0, Z: -3000,
        })
    }
    loop.waypoints = waypoints
    loop.wpIndex = 0

    steps, wpMap := loop.cursorEscapeRouteSteps(0, 0, -3000, 0)
    require.NotEmpty(t, steps)
    require.Len(t, wpMap, len(steps))
    last := steps[len(steps)-1]
    total := math.Hypot(float64(last[0]), float64(last[1]))
    require.LessOrEqual(t, total, cursorEscapeRouteMax+cursorEscapeStep,
        "the capped ladder ends at the route cap, walked %.0f", total)
}

// TestCursorEscapeRouteStepsNilWithoutPlan pins the fallback
// contract: without a plan (no navigator, no waypoints, an exhausted
// cursor) the route ladder answers nil and the straight line ladder
// toward the validated aim owns the escape.
func TestCursorEscapeRouteStepsNilWithoutPlan(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    _, wpMap := loop.cursorEscapeRouteSteps(0, 0, -3000, 0)
    require.Nil(t, wpMap,
        "no navigator: no route ladder")

    loop.SetNavigator(&fakeNavigator{})
    _, wpMap = loop.cursorEscapeRouteSteps(0, 0, -3000, 0)
    require.Nil(t, wpMap,
        "no waypoints: no route ladder")

    loop.waypoints = []pathfind.Vec3{{X: 100, Y: 0, Z: -3000}}
    loop.wpIndex = 1
    _, wpMap = loop.cursorEscapeRouteSteps(0, 0, -3000, 0)
    require.Nil(t, wpMap,
        "an exhausted cursor: no route ladder")
}
