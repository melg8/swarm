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
//
// The counter stand round (2026-09-20, the follow up user report):
// the closest walkable cell to the spawn is still the wrong side for
// the stall merchants - for Unoren the spawn route answered the
// OUTER side of the stall front 42 units north-west (the spawn cell
// sits inside the roofed stall the pack models as roof-only cells,
// nobody can stand on it directly), while the customer side is the
// counter front along the merchant facing heading. The merchant
// stops now target the curated stand table (merchantStands): the
// cell just beyond the counter front, verified against the mesh by
// cmd/counterprobe, so the plan ends face to face with the merchant
// across the counter.
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
// planning branch of the merchant segments.
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
        "the merchant segment must plan")
    last := loop.waypoints[len(loop.waypoints)-1]
    d2D = math.Hypot(last.X-float64(arielX), last.Y-float64(arielY))
    dz = math.Abs(last.Z - float64(arielZ))
    d3D = math.Sqrt(d2D*d2D + dz*dz)

    return d2D, dz, d3D
}

// TestMerchantStopWalksToTheCustomerCell pins the user rule on the
// real pack and the real mesh tiles: the sell stop segment of the weapon
// merchant plans the exact search (approach zero in the segment contract)
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
        "the merchant segment must plan (no trip abort)")
    d2D, dz, d3D := lastWaypointDistanceToAriel(t, loop)
    require.LessOrEqual(t, d2D, npcApproachOffset,
        "the plan must end at the customer cell across the counter, "+
            "not on the outer deck the ring catches")
    require.LessOrEqual(t, dz, 64.0,
        "the plan must end on the merchant floor, never on the roof "+
            "layer above the shop")
    require.LessOrEqual(t, d3D, npcInteractionDist,
        "the talk gate must fire from the plan end")
    require.NotNil(t, loop.segmentSearch)
    require.Zero(t, loop.segmentSearch.Approach,
        "the segment contract must carry the exact search")
    require.InDelta(t, waypointPassDist, loop.finalArriveRadius(), 0.001,
        "the exact segment must walk to its final cell, not stop a wide "+
            "arrive radius short of it")
}

// TestMerchantExactContractSurvivesTheRepath pins the re-path
// preservation: a stuck exact segment re-plans the exact search (the
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
    require.NotNil(t, loop.segmentSearch)
    require.Zero(t, loop.segmentSearch.Approach)

    require.True(t, loop.replanTownWalkSegment(loop.segmentDest),
        "the exact segment must re-plan")
    d2D, dz, _ := lastWaypointDistanceToAriel(t, loop)
    require.LessOrEqual(t, d2D, npcApproachOffset,
        "the re-planned segment must still end at the customer cell")
    require.LessOrEqual(t, dz, 64.0,
        "the re-planned segment must still end on the merchant floor")
}

// TestMerchantRoofFallbackStopsOnTheWideDeck pins the ladder of the
// one failure class the exact and npc searches cannot answer: the plan
// that resolved onto a foreign deck (a connected roof layer over the
// shop answers the destination cell's closest-layer resolution hundreds
// of units above the merchant's floor - the 2026-09-11 roof teleport
// geometry). The npc rung plans the same roof answer at the npc search
// radius and refuses it the same way the exact planner does; the wide
// rung then serves the conservative deck stop the talk machinery
// finishes through the offset click window. A corridor-less exact
// answer takes no fallback at all - the reachable mesh component is
// the same for both goals.
func TestMerchantRoofFallbackStopsOnTheWideDeck(t *testing.T) {
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
        "the wide deck stop must keep the trip walking")
    require.NotEmpty(t, loop.waypoints,
        "the wide rung must plan the segment")
    require.Len(t, nav.approachRadii, 2,
        "the ladder runs the npc rung, then the wide rung")
    require.InDelta(t, npcApproachRadius, nav.approachRadii[0], 0.001,
        "the npc rung searches with the npc radius first")
    require.InDelta(t, tripApproachRadius, nav.approachRadii[1], 0.001,
        "the wide rung serves the conservative deck stop")
    require.InDelta(t, tripApproachRadius, loop.segmentSearch.Approach,
        0.001, "the armed segment carries the wide rung contract")
}

// TestMerchantStandTableCoversTheCounterTraders pins the stand table
// integrity: every known shop merchant carries a customer stand
// entry, every stand sits on the customer side of the counter (a
// distinct cell from the spawn) and stays well inside the interaction
// distance of the spawn the talk gates measure.
func TestMerchantStandTableCoversTheCounterTraders(t *testing.T) {
    for _, list := range [][]townNpc{townMerchants, dionMerchants} {
        for _, npc := range list {
            stand, ok := merchantStands[npc.TemplateID]
            require.True(t, ok,
                "the counter trader %s must carry a stand entry", npc.Name)
            require.NotEqual(t, townNpcPosition(npc), stand,
                "the stand of %s must not be the blocked spawn cell",
                npc.Name)
            d3D := math.Sqrt(
                math.Pow(stand.X-float64(npc.X), 2) +
                    math.Pow(stand.Y-float64(npc.Y), 2) +
                    math.Pow(stand.Z-float64(npc.Z), 2))
            require.LessOrEqual(t, d3D, npcInteractionDist,
                "the stand of %s must sit inside the interaction "+
                    "distance of the spawn the talk gates measure",
                npc.Name)
        }
    }
}

// TestUnorenStopTargetsTheCounterStand pins the user report of the
// counter stand round on the real pack and the real mesh tiles: the
// sell stop of the weapon merchant Unoren plans the exact search to
// the STAND TABLE cell (the counter front west of the stall, the
// customer side) and its plan end sits on that cell - not on the old
// outer side of the stall front 42 units north-west of the spawn the
// raw spawn route answered.
func TestUnorenStopTargetsTheCounterStand(t *testing.T) {
    disablePace(t)
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, customerApproachX, customerApproachY,
        customerApproachZ)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(nav)

    unoren := townNpc{TemplateID: 7147, Name: "Unoren",
        X: 44667, Y: 46896, Z: -2982}
    stand := merchantStandPoint(unoren)
    require.Equal(t, merchantStands[7147], stand,
        "the stand table must know the Unoren customer cell")

    armMerchantStop(t, loop, unoren)
    require.Equal(t, phaseTownWalk, loop.phase,
        "the merchant segment must plan (no trip abort)")
    require.Equal(t, stand, loop.segmentDest,
        "the segment must target the stand cell, not the spawn")
    require.NotEmpty(t, loop.waypoints,
        "the merchant segment must plan")
    last := loop.waypoints[len(loop.waypoints)-1]
    miss := math.Hypot(last.X-stand.X, last.Y-stand.Y)
    require.LessOrEqual(t, miss, 16.0,
        "the plan must end on the stand cell at the counter front")
    d2D := math.Hypot(last.X-float64(unoren.X), last.Y-float64(unoren.Y))
    require.LessOrEqual(t, d2D, npcInteractionDist,
        "the talk gate must fire from the stand cell")
}
