// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The server frame click transport pins (see click_frame.go): a plan
// carries the mesh frame z of the bot's own geodata pack, the server
// resolves a click's destination layer by the nearest height to the z
// the request names, and wherever the pack vintages disagree about a
// surface's absolute height the raw mesh z names the wrong layer - the
// village sandwich refusals. The transport measures the shift at the
// one pair both frames vouch for (the character's standing cell) and
// rides every plan derived click z into the server frame.

// frameRoute is the mesh frame plan of the pins: the character's cell
// first (the plan start IS the standing cell), one bend ahead. The
// mesh names the surface -3000; the deployed server's pack names the
// same physical surface -2600 (a 400 unit vintage shift, inside the
// measured flip margin band of the village sandwich).
var frameRoute = []pathfind.Vec3{
    {X: 46000, Y: 51000, Z: -3000},
    {X: 46100, Y: 51100, Z: -3050},
    {X: 46200, Y: 51200, Z: -3100},
}

// frameTripLoop builds the town walk state the transport pins walk
// through: the trip mid leg with the cursor on the second waypoint of
// frameRoute and the character standing on the shifted surface (the
// server z -2600 against the mesh -3000).
func frameTripLoop() (*Loop, *fakeGame, *state.Bot, *fakeNavigator) {
    loop, game, bot, nav := newTripLoop()
    route := make([]pathfind.Vec3, len(frameRoute))
    copy(route, frameRoute)
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{{
        merchant: townNpc{
            TemplateID: 7155, Name: "Ellenia",
            X: 45725, Y: 52105, Z: -2792,
        },
    }}
    loop.waypoints = route
    loop.wpIndex = 1
    loop.legDest = pathfind.Vec3{X: 46300, Y: 51300, Z: -3100}
    moveSelfTo(bot, 46000, 51000, -2600)

    return loop, game, bot, nav
}

// TestMeasureFrameOffsetDiscardsLayerSnaps pins the measurement
// itself: a difference inside the limit is the vintage shift the
// transport rides, a difference beyond it is a layer snap (the pack
// resolved the standing cell onto another layer of the sandwich) and
// measures zero - anchoring by a layer gap would corrupt every click
// of the plan.
func TestMeasureFrameOffsetDiscardsLayerSnaps(t *testing.T) {
    require.InDelta(t, 400.0, measureFrameOffset(-2600, -3000), 0.001,
        "the vintage shift measures signed")
    require.InDelta(t, -400.0, measureFrameOffset(-3000, -2600), 0.001,
        "the shift measures in the server frame direction")
    require.InDelta(t, -frameOffsetLimit,
        measureFrameOffset(-3000-int32(frameOffsetLimit), -3000), 0.001,
        "the limit itself still measures")
    require.Zero(t, measureFrameOffset(-3000-872, -3000),
        "the deck over water layer gap measures no shift")
    require.Zero(t, measureFrameOffset(-3000, 0),
        "a plan start on another layer than the standing surface "+
            "measures no shift")
}

// TestFreshLegCalibratesFrameOffsetFromTheStandingPair pins the
// calibration of a fresh leg plan: the plan's first waypoint is the
// character's own cell resolved on the pack, so its mesh z against
// the server vouched standing z measures the shift every click of
// this leg rides.
func TestFreshLegCalibratesFrameOffsetFromTheStandingPair(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    nav.route = []pathfind.Vec3{
        {X: 46000, Y: 51000, Z: -3000},
        {X: 46200, Y: 51200, Z: -3100},
    }
    moveSelfTo(bot, 46000, 51000, -2600)

    require.True(t, loop.startWalkLeg(
        pathfind.Vec3{X: 46200, Y: 51200, Z: -3100}))
    require.InDelta(t, 400.0, loop.legFrameOffset,
        0.001, "the standing pair measures the vintage shift")

    // The swimming character measures no shift: the swim z rides the
    // water surface, the mesh z names the floor, the pair is
    // geometry - not a pack disagreement.
    nav2 := &fakeNavigator{found: true, overWater: true,
        route: []pathfind.Vec3{
            {X: 46000, Y: 51000, Z: -3900},
            {X: 46200, Y: 51200, Z: -3100},
        }}
    loop2, _, bot2, _ := newTripLoop()
    loop2.SetNavigator(nav2)
    moveSelfTo(bot2, 46000, 51000, -3720)
    require.True(t, loop2.startWalkLeg(
        pathfind.Vec3{X: 46200, Y: 51200, Z: -3100}))
    require.Zero(t, loop2.legFrameOffset,
        "the swim floor pair is not a vintage pair")
}

// TestWalkClickRidesTheServerFrameTransport pins the systemic fix of
// the refused click round: the walk click carries the anchored z (the
// mesh height plus the measured shift), which names the standing
// surface's layer in the server frame - the raw mesh z of the old
// clicks named the water floor under the deck wherever the vintages
// disagreed and every such click answered ActionFailed.
func TestWalkClickRidesTheServerFrameTransport(t *testing.T) {
    loop, game, _, _ := frameTripLoop()
    loop.legFrameOffset = 400

    loop.tick()

    require.Equal(t, [][3]int32{{46100, 51100, -2650}}, game.walks,
        "the click z rides the measured shift into the server frame "+
            "(-3050 mesh + 400 shift), never the raw mesh z")
}

// TestWalkClickLongLegSplitsInTheServerFrame pins the far split of
// the transport: a waypoint beyond the move leg cap splits into a
// straight intermediate point whose z interpolates from the server
// vouched standing z to the anchored waypoint z - the split point
// names the same layer the full leg would in the server frame.
func TestWalkClickLongLegSplitsInTheServerFrame(t *testing.T) {
    loop, game, _, _ := frameTripLoop()
    loop.legFrameOffset = 400
    loop.waypoints[2] = pathfind.Vec3{X: 47400, Y: 52400, Z: -3100}
    loop.wpIndex = 2

    loop.tick()

    // The split rides the 1000 unit chord toward the anchored
    // waypoint z (-3100 + 400 = -2700): self -2600 plus half of the
    // -100 drop.
    require.Equal(t, [][3]int32{{46707, 51707, -2650}}, game.walks,
        "the split point interpolates standing z to anchored z")
}

// TestWalkClickRidesRawMeshZWhenThePairIsALayerSnap pins the
// discard: an offset beyond the frame limit is a layer snap, the
// transport measures zero and the click rides the raw mesh z - the
// refusal ladder owns the plans whose start the pack cannot resolve
// onto the standing layer.
func TestWalkClickRidesRawMeshZWhenThePairIsALayerSnap(t *testing.T) {
    loop, game, _, _ := frameTripLoop()
    loop.legFrameOffset = 0

    loop.tick()

    require.Equal(t, [][3]int32{{46100, 51100, -3050}}, game.walks,
        "a zero offset rides the raw mesh z")
}

// TestUserWalkClickRidesTheServerFrameTransport pins the manual walk
// follower of the transport: the mesh route of a manual move anchors
// its waypoint clicks with the offset measured at the plan start -
// the manual walks into the village sandwich name the standing
// surface's layer in the server frame exactly like the autonomous
// legs do.
func TestUserWalkClickRidesTheServerFrameTransport(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    nav.route = []pathfind.Vec3{
        {X: 46000, Y: 51000, Z: -3000},
        {X: 46100, Y: 51100, Z: -3050},
    }
    moveSelfTo(bot, 46000, 51000, -2600)

    loop.planUserWalk(46000, 51000, -2600)
    require.InDelta(t, 400.0, loop.userFrameOffset,
        0.001, "the manual plan measures the same standing pair")

    // The cursor rides the second waypoint: the first one sits under
    // the character (its z gap above the server z keeps the 3D
    // arrival quiet) and the click of the pin aims the bend ahead.
    loop.userWpIndex = 1
    loop.userStart = time.Now()
    loop.followUserWaypoints(time.Now(), 46000, 51000, -2600)

    require.Equal(t, [][3]int32{{46100, 51100, -2650}}, game.walks,
        "the manual click rides the measured shift")
}

// TestQuestWaypointClickRidesTheServerFrameTransport pins the quest
// segment follower of the transport: the segment plan measures the
// standing pair at its start and the waypoint clicks ride the shift -
// the Gludio lesson (the raw self z never rides a rise) stays, the
// mesh height just arrives in the server frame.
func TestQuestWaypointClickRidesTheServerFrameTransport(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, 46000, 51000, -2600)

    waypoints := []pathfind.Vec3{
        {X: 46000, Y: 51000, Z: -3000},
        {X: 46100, Y: 51100, Z: -3050},
    }
    done := make(chan struct{})
    go func() {
        defer close(done)
        // The follower blocks until the waypoint arrival or the
        // deadline: the walk request of the pin goes out on the
        // first iteration, the deadline closes the loop.
        loop.followWaypoint(waypoints, 1, 400.0,
            time.Now().Add(50*time.Millisecond))
    }()
    // The join orders the follower's walk record before the
    // assertion read (the race detector clean way to pin a blocking
    // follower's first click).
    <-done

    require.Equal(t, [][3]int32{{46100, 51100, -2650}}, game.walks,
        "the quest segment click rides the measured shift")
}
