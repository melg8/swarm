// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The repro contract of the 3D pathfind link (the 2026-09-19 route
// mismatch round): every published walk plan carries the mesh search
// contract it answered - the filter, the approach radius and the ban
// circles - so the viewer rebuilds the very search the bot walks
// instead of a lookalike (the dry zone return rebuilt with the swim
// filter and folded into a straight water blind chord drew a route
// the bot never walked).

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestZoneReturnPlanCarriesDrySearchContract pins the zone return
// stamp: the planned return leg publishes the dry filter with the
// trip approach radius - the viewer replay of the link rebuilds the
// dry approach search, not the swim exact lookalike.
func TestZoneReturnPlanCarriesDrySearchContract(t *testing.T) {
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
    require.True(t, plan.Search.Dry,
        "the zone return plans the dry search first")
    require.InDelta(t, tripApproachRadius, plan.Search.Approach, 0.01)
    require.Empty(t, plan.Search.Avoid,
        "the clean session carries no ban circles")
}

// TestDirectLegPlanCarriesNoSearchContract pins the direct leg stamp:
// the server routed fallback answers no mesh search, the plan view
// carries no contract and the pathfind link keeps the viewer defaults.
func TestDirectLegPlanCarriesNoSearchContract(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    game.noTargets = true
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{found: true, height: -3539})
    loop.phase = phaseTownReturn
    loop.legDest = pathfind.Vec3{X: 46112, Y: 41500, Z: -3539}
    loop.legStart = pathfind.Vec3{X: 49308, Y: 44213, Z: -3539}
    // A stale mesh contract of a previous leg must not leak into the
    // direct leg's plan view.
    loop.legSearch = &state.WalkSearch{Dry: true,
        Approach: tripApproachRadius}

    loop.armDirectLeg("the waypoint plan died")

    plan := loop.activeWalkPlan()
    require.NotNil(t, plan)
    require.Nil(t, plan.Search,
        "the direct leg answers no mesh search")
}

// TestManualMeshPlanCarriesSwimSearchContract pins the manual move
// stamp: the mesh planned user walk publishes the swim filter with
// the user approach radius, so the link rebuilds the manual walk the
// follower walks.
func TestManualMeshPlanCarriesSwimSearchContract(t *testing.T) {
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
    plan := loop.activeWalkPlan()
    require.NotNil(t, plan)
    require.NotNil(t, plan.Search)
    require.False(t, plan.Search.Dry,
        "the manual walk plans the swim search")
    require.InDelta(t, userApproachRadius, plan.Search.Approach, 0.01)
}
