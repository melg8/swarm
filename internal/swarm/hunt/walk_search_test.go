// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The repro contract of the 3D pathfind link (the 2026-09-19 route
// mismatch round): every published walk plan carries the mesh search
// contract it answered - the approach radius and the ban circles - so
// the viewer rebuilds the very search the bot walks instead of a
// lookalike. The water pricing needs no contract word: every search
// prices the water at the swim rate, there is no walled form to name.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestZoneReturnPlanCarriesSearchContract pins the zone return stamp:
// the planned return leg publishes the search contract with the trip
// approach radius - the viewer replay of the link rebuilds the priced
// approach search, not the exact destination lookalike.
func TestZoneReturnPlanCarriesSearchContract(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    game.noTargets = true
    nav := &fakeNavigator{found: true, height: -3539}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(46112, 41500, 450)
    loop.lastHit = time.Now().Add(-time.Minute)

    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
        DestX: 49308, DestY: 44213, DestZ: -3539,
    })
    loop.lastHit = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, phaseTownReturn, loop.phase)

    plan := loop.activeWalkPlan()
    require.NotNil(t, plan)
    require.NotNil(t, plan.Search,
        "the mesh plan publishes its search contract")
    require.InDelta(t, tripApproachRadius, plan.Search.Approach, 0.01)
    require.Empty(t, plan.Search.Avoid,
        "the clean session carries no ban circles")
}

// TestManualMeshPlanCarriesSearchContract pins the manual move stamp:
// the mesh planned user walk publishes the search contract of the
// exact destination search (approach zero, no bans), so the link
// rebuilds the manual walk the follower walks - the walk the owner
// clicked must arrive at the clicked point, the approach ring is the
// fallback only.
func TestManualMeshPlanCarriesSearchContract(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    game.noTargets = true
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{found: true, height: -3500})

    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45000, DestY: 50000, DestZ: -3500,
    })
    loop.phase = phaseUser
    loop.userKind = state.CommandMove
    loop.userX, loop.userY, loop.userZ = 46200, 51100, -3500
    loop.planUserWalk(45000, 50000, -3500)

    require.NotEmpty(t, loop.userWaypoints,
        "the fake navigator answers a route")
    require.InDelta(t, 46200.0,
        loop.userWaypoints[len(loop.userWaypoints)-1].X, 0.01,
        "the exact plan ends at the clicked point")
    require.InDelta(t, 51100.0,
        loop.userWaypoints[len(loop.userWaypoints)-1].Y, 0.01,
        "the exact plan ends at the clicked point")
    plan := loop.activeWalkPlan()
    require.NotNil(t, plan)
    require.NotNil(t, plan.Search)
    require.InDelta(t, 0.0, plan.Search.Approach, 0.01)
}

// TestManualWalkPlansTheExactClickedPoint pins the temple entrance
// round of the 2026-09-19 owner report: the manual walk from the
// plaza cell (44694 51921 -2808) to the temple interior cell
// (44718 52291 -2792) plans the exact route - the approach ring
// search would end the plan at the doorway polygon (44718 52144,
// 147 units short of the clicked cell, inside the 150 ring) and hold
// the character at the entrance forever.
func TestManualWalkPlansTheExactClickedPoint(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    game.noTargets = true
    nav := &fakeNavigator{found: true, height: -2792}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)

    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 44694, Y: 51921, Z: -2808,
        DestX: 44694, DestY: 51921, DestZ: -2808,
    })
    loop.phase = phaseUser
    loop.userKind = state.CommandMove
    loop.userX, loop.userY, loop.userZ = 44718, 52291, -2792
    loop.planUserWalk(44694, 51921, -2808)

    require.NotEmpty(t, loop.userWaypoints)
    last := loop.userWaypoints[len(loop.userWaypoints)-1]
    require.InDelta(t, 44718.0, last.X, 0.01)
    require.InDelta(t, 52291.0, last.Y, 0.01)
    require.Empty(t, nav.approachEnds,
        "the reachable click never asks the approach search")
    plan := loop.activeWalkPlan()
    require.NotNil(t, plan)
    require.NotNil(t, plan.Search)
    require.InDelta(t, 0.0, plan.Search.Approach, 0.01)
}

// TestManualMeshPlanFallsBackToApproach pins the fallback: the exact
// search without an answer (the clicked point on ground the mesh
// does not reach) plans the approach corridor instead and the
// published contract names the user approach radius.
func TestManualMeshPlanFallsBackToApproach(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    game.noTargets = true
    nav := &fakeNavigator{exactMiss: true, found: true, height: -3500}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)

    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45000, DestY: 50000, DestZ: -3500,
    })
    loop.phase = phaseUser
    loop.userKind = state.CommandMove
    loop.userX, loop.userY, loop.userZ = 46200, 51100, -3500
    loop.planUserWalk(45000, 50000, -3500)

    require.NotEmpty(t, loop.userWaypoints,
        "the approach fallback answers the corridor")
    require.Len(t, nav.approachEnds, 1,
        "the fallback asked the approach search once")
    plan := loop.activeWalkPlan()
    require.NotNil(t, plan)
    require.NotNil(t, plan.Search)
    require.InDelta(t, userApproachRadius, plan.Search.Approach, 0.01)
}
