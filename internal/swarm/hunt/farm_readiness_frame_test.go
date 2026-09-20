// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The frame tests of the farm readiness round (2026-09-20): the plan
// waypoints carry the pack heights, the character position carries
// the server frame, and where the two frames disagree (the vintage
// shift of the elven village deck, 64 units at the jewelry shop
// porch) every raw z comparison inflates by the shift. The three
// answers: the arrival test anchors the waypoint z (the pinned
// cursor of the plan's own start), the cursor key claims anchor the
// step z (the underground claims the server corrects away) and the
// trip return resolves the destination deck (the search that missed
// the farm zone by hundreds of units forever).

// TestWaypointArrivedAnchorsTheFrameOffset pins the arrival fix: a
// plan whose start waypoint resolved onto the standing cell with the
// pack 64 units under the live ground counts as arrived WITH the
// measured offset and never without it. Without the anchoring the
// cursor pins at the plan's own start, the self click bounces and
// the recovery ladder burns without a walk attempt (the porch round).
func TestWaypointArrivedAnchorsTheFrameOffset(t *testing.T) {
    waypoints := []pathfind.Vec3{
        {X: 44584, Y: 46944, Z: -2984},
        {X: 44136, Y: 47168, Z: -2984},
    }
    // The character stands ON the start waypoint planar, the server
    // frame 64 units above the pack surface (the porch shift).
    const shift = 64.0
    selfX, selfY, selfZ := int32(44584), int32(46944),
        int32(-2984)+int32(shift)

    require.False(t, waypointArrived(waypoints, 0, selfX, selfY, selfZ,
        0, waypointArriveDist),
        "without the offset the raw distance measures the shift alone")
    require.True(t, waypointArrived(waypoints, 0, selfX, selfY, selfZ,
        shift, waypointArriveDist),
        "the measured offset anchors the start onto the standing cell")

    // The deck edge protection stays: a waypoint a whole deck below
    // the character is not arrived even with the offset - the shift
    // explains the vintage disagreement, not a 920 unit drop.
    deck := []pathfind.Vec3{
        {X: 44584, Y: 46944, Z: -2984},
        {X: 44584, Y: 46944, Z: -3904},
    }
    require.False(t, waypointArrived(deck, 1, selfX, selfY, selfZ,
        shift, waypointArriveDist),
        "the deck drop stays unarrived through the offset")
}

// TestCursorEscapeClaimsRideServerFrame pins the claim anchoring:
// every claimed step of a route following escape carries the pack z
// plus the segment's frame offset, the arm request included (it aims
// the ladder's far end). The raw pack z would claim the character 64
// units under its own ground on every shifted cell - the server
// corrects the placement back and the escape walks the character
// nowhere.
func TestCursorEscapeClaimsRideServerFrame(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.SetNavigator(&fakeNavigator{})
    loop.waypoints = []pathfind.Vec3{
        {X: 1000, Y: 0, Z: -2984},
        {X: 2000, Y: 0, Z: -2984},
    }
    loop.wpIndex = 0
    const shift = 64.0
    loop.segmentFrameOffset = shift

    steps, _ := loop.cursorEscapeRouteSteps(0, 0, -2920)
    require.NotEmpty(t, steps)
    for i, step := range steps {
        require.InDelta(t, -2920.0, float64(step[2]), 1,
            "step %d claims the pack z without the frame offset: %d",
            i, step[2])
    }

    // The planless fallback: the aim z anchors the same way, so the
    // straight ladder interpolates inside one frame - every claimed
    // step rides the server frame z of the standing ground.
    loop.waypoints = nil
    loop.wpIndex = 0
    loop.segmentFrameOffset = shift
    require.True(t, loop.beginCursorKeyEscape(0, 0, -2920, 300, 0,
        -2984),
        "the planless fallback arms its straight ladder")
    require.NotEmpty(t, loop.cursorEscape.steps)
    for i, step := range loop.cursorEscape.steps {
        require.InDelta(t, -2920.0, float64(step[2]), 1,
            "fallback step %d claims the raw aim z without the "+
                "frame offset: %d", i, step[2])
    }
}

// TestTripReturnResolvesDestinationDeck pins the return fix: the trip
// return leg resolves the deck under the destination through the
// navigator before the search. The remembered spot z is the
// character's own standing z of another area; the farm readiness
// round rode the village porch z (-2920) into a zone center whose
// deck sits at -3712 and the mesh search answered "no navmesh under
// the position" on every retry.
func TestTripReturnResolvesDestinationDeck(t *testing.T) {
    loop, _, _, nav := newTripLoop()
    // The remembered farm spot: the zone center x/y with the stale
    // standing z of the village deck. The navigator answers the real
    // deck of the destination.
    loop.farmX, loop.farmY, loop.farmZ = 36000, 46765, -2920
    nav.height = -3712

    loop.startReturnSegment()

    require.Equal(t, phaseTownReturn, loop.phase,
        "the return segment arms")
    require.InDelta(t, -3712.0, loop.segmentDest.Z, 0.1,
        "the return search walks to the resolved deck, not the stale z")
    require.Equal(t, 1, nav.closestCalls,
        "the deck resolution asks the navigator once")
    require.InDelta(t, 36000.0, nav.closestX, 0.1,
        "the lookup names the destination x")
    require.InDelta(t, 46765.0, nav.closestY, 0.1,
        "the lookup names the destination y")
    require.InDelta(t, -2920.0, nav.closestRefZ, 0.1,
        "the lookup rides the remembered z as the reference")

    // A lookup failure keeps the remembered z: the same-deck case it
    // answers correctly (the fake's heightErr flips the answer).
    loop, _, _, nav = newTripLoop()
    loop.farmX, loop.farmY, loop.farmZ = 36000, 46765, -2920
    nav.heightErr = true

    loop.startReturnSegment()

    require.InDelta(t, -2920.0, loop.segmentDest.Z, 0.1,
        "the failed lookup keeps the remembered z")
}
