// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// walkToward sends a ground click walk to the point at most once per
// walk request period, sharing the move pacing of the trip machinery.
func (l *Loop) walkToward(x, y, z int32, now time.Time) {
    if !l.moveAt.IsZero() && now.Sub(l.moveAt) < walkRequestPeriod {
        return
    }
    l.moveAt = now
    if err := l.game.WalkTo(x, y, z); err != nil {
        l.logf("Hunt: walk request failed: %v", err)
    }
}

// exactApproachWanted reports whether a completed ring walk needs the exact
// final approach: the stop is a merchant stop whose segment destination is the
// merchant spawn, the walk was the ring approach (the exact segments already
// arrive at the customer cell), and the merchant sits beyond the customer ring
// but inside the exact distance gate - near enough for the bounded search, far
// enough that the talk would fire from the ring edge instead of the counter.
func (l *Loop) exactApproachWanted() bool {
    if len(l.tripStops) == 0 || l.tripStops[0].teach {
        return false
    }
    merchant := l.tripStops[0].merchant
    if l.segmentDest != merchantStandPoint(merchant) {
        return false
    }
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        return false
    }
    if !l.merchantWithinExactRange(merchant) {
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dx := float64(merchant.X - selfX)
    dy := float64(merchant.Y - selfY)
    dz := float64(merchant.Z - selfZ)

    return math.Sqrt(dx*dx+dy*dy+dz*dz) > npcApproachOffset
}

// stopNpcArrivalMet reports whether the running segment brought the character
// close enough for the merchant stop's talk machinery: within the wide arrive
// radius of the segment destination AND within the interaction distance of the
// merchant npc. The tight final-cell verification can grind forever against the
// vintage geodata disagreement (the live server resolves the click onto its own
// surface, the pack holds cells 64 units off the live floor), and the stuck
// ladder would abort a trip whose character stands ready to trade
// (docs/hunting.md, the npc arrival handoff).
func (l *Loop) stopNpcArrivalMet() bool {
    if len(l.tripStops) == 0 || l.tripStops[0].teach {
        return false
    }
    dest := l.segmentDest
    if dest != merchantStandPoint(l.tripStops[0].merchant) {
        // The segment walks something else (the far haul leg, the
        // return): the npc handoff owns the merchant stop walks only.
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dx := dest.X - float64(selfX)
    dy := dest.Y - float64(selfY)
    dz := dest.Z - float64(selfZ)
    if math.Sqrt(dx*dx+dy*dy+dz*dz) > waypointArriveDist {
        return false
    }
    npc, ok := l.tracker.NearestNpcByTemplates(
        l.stopMerchantTemplates(), merchantFindRadius)
    if !ok {
        return false
    }
    dx = float64(npc.X - selfX)
    dy = float64(npc.Y - selfY)
    dz = float64(npc.Z - selfZ)

    return math.Sqrt(dx*dx+dy*dy+dz*dz) <= npcInteractionDist
}

// startWalkSegment plans the walk to the destination through the priced
// approach search and arms the waypoint follower: the search goal is the
// merchant interaction distance, so a destination behind a counter or on a
// floor layer the geodata does not model is still reached on the surrounding
// deck. A destination no route reaches reports false (the callers abort the
// trip), and the planning position publishes as the segment origin of the walk
// plan view.
func (l *Loop) startWalkSegment(dest pathfind.Vec3) bool {
    return l.startWalkSegmentSearch(dest)
}

// startZoneReturnSegment plans the zone return walk through the priced approach
// search: the partial corridor answer gives the bot the walkable route toward
// home whenever the corridor exists (docs/navmesh.md, the partial round). It
// reports whether the segment was planned.
func (l *Loop) startZoneReturnSegment(dest pathfind.Vec3) bool {
    return l.startWalkSegmentSearch(dest)
}

// segmentSearchView freezes one mesh search contract into the walk plan
// view: the approach radius the search ran with.
func segmentSearchView(approach float64) *state.WalkSearch {
    return &state.WalkSearch{Approach: approach, Avoid: nil}
}

// startWalkSegmentSearch plans the walk through the priced approach search and
// arms the waypoint follower: the water crossings pay the swim rate, so the
// plan prefers the land detours when they are the faster walk. It reports
// whether the segment was planned.
func (l *Loop) startWalkSegmentSearch(dest pathfind.Vec3) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    radius := l.segmentRadius
    if radius <= 0 {
        radius = tripApproachRadius
    }
    result, err := l.navigator.FindPathApproach(from, dest, radius)
    if err != nil {
        l.logf("Hunt: town trip path search failed: %v", err)

        return false
    }
    if result == nil || len(result.Waypoints) == 0 {
        l.logf("Hunt: no path to %d %d at all",
            int(dest.X), int(dest.Y))

        return false
    }
    if !result.Found {
        // The partial round (docs/navmesh.md): both engines agree the
        // destination is unreachable under the filter, and the mesh funnel
        // still holds the walkable corridor toward it - the segment walks the
        // closest reachable point instead of aborting at the start position, so
        // the trip continues from wherever the ground ends (the shore of a
        // swim-only destination, the border of the sealed corridor).
        l.logf("Hunt: no route to %d %d, walking the closest "+
            "reachable point", int(dest.X), int(dest.Y))
    }

    return l.armTownWalkSegment(selfX, selfY, selfZ, from, dest, result,
        radius)
}

// startWalkNpcSegment plans the npc destination walk through the two rung
// ladder: the npc rung searches with npcApproachRadius (the plan must end at
// the npc's own point) and refuses a found plan that resolved onto a foreign
// deck (a connected roof over the shop answers the closest layer hundreds of
// units above the npc floor); the wide rung searches with the trip ring and
// arms whatever walkable deck it catches, and the offset click window of the
// talk machinery owns the last stretch there. It reports whether the segment
// was planned (docs/hunting.md, the path layer selection).
func (l *Loop) startWalkNpcSegment(dest pathfind.Vec3) bool {
    if l.planNpcSegment(dest) {
        return true
    }
    l.segmentRadius = tripApproachRadius

    return l.startWalkSegment(dest)
}

// planNpcSegment is the npc rung: the npc radius plan, armed only when it ends
// on the npc's deck (the partial answers always arm - the closest reachable
// point is the walk-what-you-can contract).
func (l *Loop) planNpcSegment(dest pathfind.Vec3) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    l.segmentRadius = npcApproachRadius
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    result, err := l.navigator.FindPathApproach(
        from, dest, npcApproachRadius)
    if err != nil {
        l.logf("Hunt: town trip npc path search failed: %v", err)

        return false
    }
    if result == nil || len(result.Waypoints) == 0 {
        l.logf("Hunt: no npc path to %d %d at all",
            int(dest.X), int(dest.Y))

        return false
    }
    if result.Found {
        last := result.Waypoints[len(result.Waypoints)-1]
        if dz := math.Abs(last.Z - dest.Z); dz > exactSegmentDeckTolerance {
            // The plan resolved onto a foreign deck: the roof over the shop.
            // Walking it would click the roof the server resolves onto the
            // character - the npc rung refuses, the wide rung stops on the
            // surrounding deck instead.
            l.logf("Hunt: npc route to %d %d lands %.0f units off "+
                "the npc deck, refusing the plan",
                int(dest.X), int(dest.Y), dz)

            return false
        }
    } else {
        // The partial round (docs/navmesh.md): the destination is
        // unreachable under the filter and the funnel still holds the
        // walkable corridor toward it - the segment walks the closest
        // reachable point (the customer cell across the counter).
        l.logf("Hunt: no npc route to %d %d, walking the closest "+
            "reachable point", int(dest.X), int(dest.Y))
    }

    return l.armTownWalkSegment(selfX, selfY, selfZ, from, dest, result,
        npcApproachRadius)
}

// replanTownWalkSegment re-plans the current town segment from the standing
// cell preserving its search contract: the exact segments re-plan the exact
// search (not the ring that ends the plan outside the shop again), the npc
// segments the npc search with its foreign deck refusal, the return segments
// their wide radius. It reports whether the segment was planned.
func (l *Loop) replanTownWalkSegment(dest pathfind.Vec3) bool {
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        planned, ringFallback := l.startWalkExactSegment(dest)
        if planned || !ringFallback {
            return planned
        }

        return l.startWalkNpcSegment(dest)
    }
    if l.segmentSearch != nil &&
        l.segmentSearch.Approach == npcApproachRadius {
        return l.startWalkNpcSegment(dest)
    }

    return l.startWalkSegmentSearch(dest)
}

// startWalkExactSegment plans the walk to the exact destination through the
// mesh corridor search (approach zero - the plan must arrive at the destination
// cell, whatever walkable cell the mesh resolves it onto): the merchant stops
// navigate with it, because the approach ring catches the first deck polygon
// inside its radius and ends the plan short, on the outer railing side of the
// stall, while the exact search answers the customer cell across the counter.
// It also reports whether the ring fallback is worth running: the foreign
// deck resolution only (docs/hunting.md, the exact merchant segment).
func (l *Loop) startWalkExactSegment(dest pathfind.Vec3) (bool, bool) {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false, false
    }
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    result, err := l.navigator.FindPath(from, dest)
    if err != nil {
        l.logf("Hunt: town trip exact path search failed: %v", err)

        return false, false
    }
    if result == nil || len(result.Waypoints) == 0 {
        l.logf("Hunt: no exact path to %d %d at all",
            int(dest.X), int(dest.Y))

        return false, false
    }
    if !result.Found && !result.Partial {
        // The manual walk contract (planUserWalk): a bare not found with no
        // corridor means the destination ground the mesh does not reach at all
        // - the ring goal of the same mesh answers the same nothing.
        l.logf("Hunt: no exact route to %d %d, no corridor",
            int(dest.X), int(dest.Y))

        return false, false
    }
    last := result.Waypoints[len(result.Waypoints)-1]
    if dz := math.Abs(last.Z - dest.Z); dz > exactSegmentDeckTolerance {
        // The plan resolved onto a foreign deck: the roof over the
        // shop (the connected roof layer answers the destination
        // cell's closest-layer resolution) or a floor the merchant
        // does not stand on. The ring stop on the surrounding deck is
        // the safe answer, the talk machinery owns the rest.
        l.logf("Hunt: exact route to %d %d lands %.0f units off "+
            "the merchant deck, falling back to the ring",
            int(dest.X), int(dest.Y), dz)

        return false, true
    }
    if !result.Found {
        // The partial corridor of the exact search IS the answer the merchant
        // stop wants: the destination sits on the counter row the mesh never
        // walks onto, the funnel ends at the customer cell in front of it.
        l.logf("Hunt: exact route to %d %d ends at the closest "+
            "walkable cell (the counter row)", int(dest.X), int(dest.Y))
    }

    return l.armTownWalkSegment(selfX, selfY, selfZ, from, dest, result, 0),
        false
}

// armTownWalkSegment arms the waypoint follower with a planned town walk:
// the plan view (the search contract of the answer), the frame
// measurement and the fresh segment state. Shared by the ring planner
// (startWalkSegmentSearch) and the exact planner (startWalkExactSegment).
func (l *Loop) armTownWalkSegment(
    selfX, selfY, selfZ int32,
    from, dest pathfind.Vec3,
    result *pathfind.Result,
    radius float64,
) bool {
    l.waypoints = result.Waypoints
    l.wpIndex = 0
    l.segmentDest = dest
    l.segmentStart = from
    // The plan view carries the search contract of this very search, so a
    // viewer replay rebuilds the walk the bot follows instead of a lookalike.
    l.segmentSearch = segmentSearchView(radius)
    // The fresh plan opens with a fresh frame measurement: the first waypoint
    // IS the character's own cell resolved on the pack, so the z difference is
    // the vintage shift of the standing surface (see click_frame.go). A
    // character the pack answers underwater measures no shift: the swim z and
    // the mesh floor z are geometry, not a pack disagreement.
    l.segmentFrameOffset = 0
    if !l.navigator.OverWater(
        float64(selfX), float64(selfY), int16(selfZ)) {
        l.segmentFrameOffset = measureFrameOffset(
            selfZ, result.Waypoints[0].Z)
    }
    l.moveAt = time.Time{}
    l.stuckAt = time.Time{}
    l.stuckFast = false
    // A fresh plan re-arms the varied aim budget; the segmentRefused latch
    // itself stays - it carries the trip level evidence the escalation gate
    // reads.
    l.refusalVariants = 0
    l.moveStartAt = time.Time{}
    l.forceStuck = false
    // A planned geodata segment owns the movement now: the direct zone
    // segment stall watcher stands down (its window would otherwise read
    // a trip's frozen standstill as its own and fire early on the
    // segments the budget gate resumes after the trip ends).
    l.zoneSegmentAt = time.Time{}

    return true
}
