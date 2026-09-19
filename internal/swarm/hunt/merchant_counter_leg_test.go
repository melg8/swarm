// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/stretchr/testify/require"
)

// The user rule of the shop quarter round (2026-09-20): the merchant
// stop of a town trip must walk the character to the customer cell
// across the counter - inside the shop, face to face with the
// merchant - not stop on the first deck polygon the approach ring
// catches outside the building. The wide ring plans ended 147-232
// units from the elven village merchants (Unoren, Ariel) on the outer
// railing side, the talk fired (or bounced) from there and the buys
// never ran; the character bumped into the outer railing and slid
// along it instead of walking through the stall front. The exact
// mesh search lands the plan on the closest walkable floor cell to
// the npc (the counter row itself the mesh never walks onto) - the
// standing spot of a real customer.
const (
    // arielX/Y/Z is the elven village weapon merchant spawn
    // (townMerchants).
    arielX = int32(44683)
    arielY = int32(46952)
    arielZ = int32(-2981)
    // customerApproachX/Y is the farm side cell north of the shop
    // quarter the trip walks from.
    customerApproachX = int32(44683)
    customerApproachY = int32(47300)
    customerApproachZ = int32(-2984)
)

// armMerchantStop arms the sell stop of the weapon merchant the way
// the trip machinery advances into it: two stops queued, the
// advanceTripStop drops the first and plans the second - the real
// planning branch of the merchant legs.
func armMerchantStop(t *testing.T, loop *Loop, npc townNpc) {
    t.Helper()
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{
        {merchant: townNpc{TemplateID: 7149, Name: "Creamees",
            X: 42700, Y: 50057, Z: -2984}, sell: true},
        {merchant: npc, sell: true},
    }
    loop.advanceTripStop()
}

// lastWaypointDistanceToAriel measures the plan end against the
// merchant spawn point.
func lastWaypointDistanceToAriel(
    t *testing.T, loop *Loop,
) (d2D float64, dz float64, d3D float64) {
    t.Helper()
    require.NotEmpty(t, loop.waypoints,
        "the merchant leg must plan")
    last := loop.waypoints[len(loop.waypoints)-1]
    d2D = math.Hypot(last.X-float64(arielX), last.Y-float64(arielY))
    dz = math.Abs(last.Z - float64(arielZ))
    d3D = math.Sqrt(d2D*d2D + dz*dz)

    return d2D, dz, d3D
}

// TestMerchantStopWalksToTheCustomerCell pins the user rule on the
// real pack and the real mesh tiles: the sell stop leg of the weapon
// merchant plans the exact search (approach zero in the leg contract)
// and its plan end sits at the customer cell - floor level, inside
// the interaction distance, far inside the old wide ring stop.
func TestMerchantStopWalksToTheCustomerCell(t *testing.T) {
    disablePace(t)
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, customerApproachX, customerApproachY,
        customerApproachZ)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(nav)

    armMerchantStop(t, loop, townNpc{TemplateID: 7148, Name: "Ariel",
        X: arielX, Y: arielY, Z: arielZ})
    require.Equal(t, phaseTownWalk, loop.phase,
        "the merchant leg must plan (no trip abort)")
    d2D, dz, d3D := lastWaypointDistanceToAriel(t, loop)
    require.LessOrEqual(t, d2D, npcApproachOffset,
        "the plan must end at the customer cell across the counter, "+
            "not on the outer deck the ring catches")
    require.LessOrEqual(t, dz, 64.0,
        "the plan must end on the merchant floor, never on the roof "+
            "layer above the shop")
    require.LessOrEqual(t, d3D, npcInteractionDist,
        "the talk gate must fire from the plan end")
    require.NotNil(t, loop.legSearch)
    require.Equal(t, 0.0, loop.legSearch.Approach,
        "the leg contract must carry the exact search")
    require.Equal(t, waypointPassDist, loop.finalArriveRadius(),
        "the exact leg must walk to its final cell, not stop a wide "+
            "arrive radius short of it")
}

// TestMerchantExactContractSurvivesTheRepath pins the re-path
// preservation: a stuck exact leg re-plans the exact search (the
// customer cell plan re-arms), it must not degrade into the approach
// ring that ends the plan outside the shop again.
func TestMerchantExactContractSurvivesTheRepath(t *testing.T) {
    disablePace(t)
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, customerApproachX, customerApproachY,
        customerApproachZ)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(nav)

    armMerchantStop(t, loop, townNpc{TemplateID: 7148, Name: "Ariel",
        X: arielX, Y: arielY, Z: arielZ})
    require.NotNil(t, loop.legSearch)
    require.Equal(t, 0.0, loop.legSearch.Approach)

    require.True(t, loop.replanTownWalkLeg(loop.legDest),
        "the exact leg must re-plan")
    d2D, dz, _ := lastWaypointDistanceToAriel(t, loop)
    require.LessOrEqual(t, d2D, npcApproachOffset,
        "the re-planned leg must still end at the customer cell")
    require.LessOrEqual(t, dz, 64.0,
        "the re-planned leg must still end on the merchant floor")
}

// TestMerchantStopFallsBackToTheRing pins the fallback of the one
// failure class the exact search cannot answer: the plan that
// resolved onto a foreign deck (a connected roof layer over the shop
// answers the destination cell's closest-layer resolution hundreds
// of units above the merchant's floor). The ring stop on the
// surrounding deck is the safe answer there, the talk machinery owns
// the rest. A corridor-less exact answer takes no fallback at all -
// the reachable mesh component is the same for both goals.
func TestMerchantStopFallsBackToTheRing(t *testing.T) {
    disablePace(t)
    // The roof plan: the exact search "succeeds" onto the roof deck
    // 349 units above the merchant floor (the 2026-09-11 roof
    // teleport report geometry).
    nav := &fakeNavigator{found: true, route: []pathfind.Vec3{
        {X: 44683, Y: 47300, Z: -2984},
        {X: 44683, Y: 46952, Z: -2632},
    }}
    bot := newTestBot()
    moveSelfTo(bot, customerApproachX, customerApproachY,
        customerApproachZ)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(nav)

    armMerchantStop(t, loop, townNpc{TemplateID: 7148, Name: "Ariel",
        X: arielX, Y: arielY, Z: arielZ})
    require.Equal(t, phaseTownWalk, loop.phase,
        "the ring fallback must keep the trip walking")
    require.NotEmpty(t, loop.waypoints,
        "the ring fallback must plan the leg")
    require.NotNil(t, loop.legSearch)
    require.Equal(t, tripApproachRadius, loop.legSearch.Approach,
        "the fallback leg carries the ring contract")
}
