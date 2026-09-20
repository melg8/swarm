// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// The npc search radius round (the 2026-09-20 user report): the wide
// trip ring (approach 200) ended the merchant plans at the first
// walkable surface inside its ball - the shop edge - while the
// requested point sat inside the shop. The real pack replay held the
// Unoren plan 226 units short (44640 46672 against the spawn 44667
// 46896), the Ariel plan 154 short; the npc search radius walks the
// same corridors into the shop and ends at the customer cell across
// the counter (42 and 40 units). The user rule: every path search to
// an npc ends at the npc's own point - the approach radius at most
// npcApproachRadius, never the wide trip ring.

// farmSpotX/Y/Z is the plan origin of the user report (the walk plan
// origin of the repro link: from 46045 41251 -3440).
const (
    farmSpotX = int32(46045)
    farmSpotY = int32(41251)
    farmSpotZ = int32(-3440)
)

// TestFarMerchantTripWalksIntoTheShop pins the user rule on the real
// pack and the real mesh tiles: the sell stop of the trader Unoren
// planned from the report's farm spot (far beyond the exact search
// gate) plans the npc search and its plan end sits at the customer
// cell inside the shop - not on the shop edge the wide ring caught.
func TestFarMerchantTripWalksIntoTheShop(t *testing.T) {
    disablePace(t)
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, farmSpotX, farmSpotY, farmSpotZ)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(nav)

    unoren := townNpc{TemplateID: 7147, Name: "Unoren",
        X: reproUnorenX, Y: reproUnorenY, Z: reproUnorenZ}
    armMerchantStop(t, loop, unoren)
    require.Equal(t, phaseTownWalk, loop.phase,
        "the far merchant segment must plan (no trip abort)")
    require.NotEmpty(t, loop.waypoints)
    require.NotNil(t, loop.segmentSearch)
    require.InDelta(t, npcApproachRadius, loop.segmentSearch.Approach,
        0.001, "the far merchant segment carries the npc search contract")

    last := loop.waypoints[len(loop.waypoints)-1]
    // The requested point of the stop: the customer stand point the
    // stand table names (the counter front cell inside the shop).
    stand := merchantStandPoint(unoren)
    d2D := math.Hypot(last.X-stand.X, last.Y-stand.Y)
    dz := math.Abs(last.Z - stand.Z)
    d3D := math.Sqrt(d2D*d2D + dz*dz)
    t.Logf("the far merchant plan ends at %.0f %.0f %.0f "+
        "(dist to the stand point %.0f)", last.X, last.Y, last.Z, d3D)
    require.LessOrEqual(t, d3D, npcApproachRadius+64.0,
        "the plan must end at the requested stand point - the customer "+
            "cell inside the shop, not on the shop edge the wide ring "+
            "caught (the replay held 226 units there)")
    require.LessOrEqual(t, dz, 64.0,
        "the plan must end on the merchant floor, never on the roof "+
            "layer above the shop")
    require.InDelta(t, waypointPassDist, loop.finalArriveRadius(), 0.001,
        "the npc segment must walk to its final cell, not stop a wide "+
            "arrive radius short of it")
}

// TestNpcStopsSearchWithTheNpcApproachRadius pins the search contract
// of every npc destination walk on the fake navigator: the merchant
// stop and the teacher stop search with the npc radius, the farm spot
// return keeps the wide trip ring.
func TestNpcStopsSearchWithTheNpcApproachRadius(t *testing.T) {
    disablePace(t)
    nav := &fakeNavigator{found: true}
    bot := newTestBot()
    moveSelfTo(bot, customerApproachX, customerApproachY,
        customerApproachZ)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetNavigator(nav)

    // The merchant stop beyond the exact search gate: the npc segment.
    // Creamees sits ~3400 units from the standing cell - the exact
    // search gate (exactSegmentMaxDistance) does not reach.
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{
        {merchant: townNpc{TemplateID: 7147, Name: "Unoren",
            X: reproUnorenX, Y: reproUnorenY, Z: reproUnorenZ},
            sell: true},
        {merchant: townNpc{TemplateID: 7149, Name: "Creamees",
            X: 42700, Y: 50057, Z: -2984}, sell: true},
    }
    loop.advanceTripStop()
    require.Equal(t, phaseTownWalk, loop.phase,
        "the far merchant segment must plan")
    require.NotEmpty(t, nav.approachRadii)
    require.InDelta(t, npcApproachRadius,
        nav.approachRadii[len(nav.approachRadii)-1], 0.001,
        "the merchant stop searches with the npc radius")

    // The teacher stop: the npc segment too (Ellenia, the elven
    // village master of the skill teachers table).
    nav.approachRadii = nav.approachRadii[:0]
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{
        {merchant: townNpc{TemplateID: 7149, Name: "Creamees",
            X: 42700, Y: 50057, Z: -2984}, sell: true},
        {merchant: townNpc{TemplateID: 7155, Name: "Ellenia",
            X: 45725, Y: 52105, Z: -2792}, teach: true},
    }
    loop.advanceTripStop()
    require.Equal(t, phaseTownWalk, loop.phase,
        "the teacher segment must plan")
    require.NotEmpty(t, nav.approachRadii)
    require.InDelta(t, npcApproachRadius,
        nav.approachRadii[len(nav.approachRadii)-1], 0.001,
        "the teacher stop searches with the npc radius")

    // The farm spot return: the wide trip ring stays.
    nav.approachRadii = nav.approachRadii[:0]
    loop.startReturnSegment()
    require.NotEmpty(t, nav.approachRadii)
    require.InDelta(t, tripApproachRadius,
        nav.approachRadii[len(nav.approachRadii)-1], 0.001,
        "the return segment keeps the wide trip ring")
}

// TestDelevelGuardWalkSearchesWithTheNpcRadius pins the delevel guard
// walk on the npc search contract: the walk to the guard spawn is an
// npc destination walk (the fight machinery owns the last stretch onto
// the live guard), the plan ends at the spawn point itself.
func TestDelevelGuardWalkSearchesWithTheNpcRadius(t *testing.T) {
    disablePace(t)
    loop, _, _, nav := newDelevelLoop(11)
    spawnZoneMobs(loop.tracker)

    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase,
        "the deleveling must arm")
    require.NotEmpty(t, nav.approachRadii,
        "the guard walk must have searched")
    require.InDelta(t, npcApproachRadius,
        nav.approachRadii[len(nav.approachRadii)-1], 0.001,
        "the guard walk searches with the npc radius")
}
