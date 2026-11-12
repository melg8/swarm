// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "time"
)

// walkPlanTTL bounds how long a published walk plan survives
// without a refresh: the hunt loop re-publishes the plan every
// tick while the walk runs, so an expired plan means the
// loop moved on (or died) and the map must stop drawing the line
// and the marker.
const walkPlanTTL = 2 * time.Second

// WalkPoint is one waypoint of the published walk plan: a planned
// geodata waypoint, the planning origin or the final destination of
// the walk.
type WalkPoint struct {
    X int32 `json:"x"`
    Y int32 `json:"y"`
    Z int32 `json:"z"`
}

// WalkPlan is the published walk plan of a running segment: where the
// segment was planned from (Origin), the full waypoint list of the plan
// (Points), the waypoint the follower currently heads to (Index) and
// the final destination of the walk (Dest). The state dump prints
// the whole thing - a stuck trip reads at a glance - and the map
// draws the planned line against the live character position.
type WalkPlan struct {
    // Origin is the position the segment was planned from, nil when the
    // publisher does not know it (the manual direct walks).
    Origin *WalkPoint `json:"origin"`
    // Points lists every planned waypoint of the segment in walk order,
    // including the passed ones.
    Points []WalkPoint `json:"points"`
    // Index is the position in Points the follower currently aims
    // at: the waypoints before it are passed, the rest lies ahead.
    Index int `json:"index"`
    // Dest is the final destination of the walk (the merchant
    // spawn, the farm spot, the clicked point), nil when the plan
    // itself carries it.
    Dest *WalkPoint `json:"dest"`
    // Search carries the mesh search contract the plan answers (the
    // repro contract of the 3D pathfind link): the approach radius
    // and the ban circles the search ran with. Nil for the plans no
    // mesh search produced (the direct server routed segments) - the
    // link then keeps the viewer defaults.
    Search *WalkSearch `json:"search,omitempty"`
}

// WalkSearch is the mesh search contract behind a walk plan: the
// exact parameters a replay of the search needs (the 2026-09-19
// route mismatch round: the web viewer rebuilt the bot's walk with
// the exact destination and no bans, and folded the answer with a
// water blind grid pass - the drawn route had nothing in common with
// the walk the bot follows). The water pricing needs no contract
// word: every search of this round prices the water at the swim
// rate, there is no walled form to name.
type WalkSearch struct {
    // Approach is the approach radius of the search: the search
    // succeeds on the first polygon whose surface sits within the
    // radius of the destination (the trip searches plan with 200),
    // zero keeps the exact destination contract (the plan then ends
    // on the destination polygon itself).
    Approach float64 `json:"approach"`
    // Avoid lists the ban circles the search walled off (the frozen
    // areas of the session). Empty for the clean searches.
    Avoid []WalkAvoidCircle `json:"avoid,omitempty"`
}

// WalkAvoidCircle is one ban circle of a walk search: the center on
// the world plane and the radius the search seals around it.
type WalkAvoidCircle struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    R float64 `json:"r"`
}

// SetWalkPlan publishes the walk plan of a running segment: the planning
// origin, the full waypoint list, the current target index and the
// final destination. A plan without points clears it. Republishing
// an equal plan only refreshes its lifetime, so the steady per tick
// refresh of the hunt loop never churns the event stream.
func (b *Bot) SetWalkPlan(plan WalkPlan) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if len(plan.Points) == 0 {
        b.clearWalkPlanLocked()

        return
    }
    now := time.Now()
    if b.walkPlan != nil && walkPlansEqual(*b.walkPlan, plan) {
        b.walkPlanAt = now

        return
    }
    b.publishWalkPlanLocked(&plan, now)
    b.rememberWalkPlanLocked(&plan)
    b.touch()
}

// publishWalkPlanLocked installs a changed plan with its timing
// view, the caller must hold the state write lock. The same route
// with the follower cursor ahead (the every tick republish after a
// waypoint was passed) keeps the walk zero point and records the
// observed arrival time of every newly passed waypoint - the dump
// prints how long each segment took. A different route (a fresh walk,
// a re-plan) or a cursor that moved back starts a new timing view:
// the zero point moves to now and the waypoints the plan already
// aims past pre-fill with now (a mid walk publish never loses the
// passed prefix).
func (b *Bot) publishWalkPlanLocked(plan *WalkPlan, now time.Time) {
    if b.walkPlan != nil && plan.Index >= b.walkPlan.Index &&
        walkPlansSameRoute(*b.walkPlan, *plan) {
        for i := b.walkPlan.Index; i < plan.Index && i < len(b.walkWpAt); i++ {
            b.walkWpAt[i] = now
        }
        b.walkPlan = plan
        b.walkPlanAt = now

        return
    }
    b.walkPlanStart = now
    arrivals := make([]time.Time, len(plan.Points))
    for i := 0; i < plan.Index && i < len(arrivals); i++ {
        arrivals[i] = now
    }
    b.walkWpAt = arrivals
    b.walkPlan = plan
    b.walkPlanAt = now
}

// rememberWalkPlanLocked copies the plan into the last walk record
// (the caller must hold the state write lock): a deep copy, so a
// later mutation of the published slice never rewrites the record
// the dump already reads. The timing view (the zero point and the
// arrivals) copies with the points, so the dump of a finished walk
// still reads the segment durations.
func (b *Bot) rememberWalkPlanLocked(plan *WalkPlan) {
    record := WalkPlan{
        Index: plan.Index,
    }
    if plan.Origin != nil {
        origin := *plan.Origin
        record.Origin = &origin
    }
    if plan.Dest != nil {
        dest := *plan.Dest
        record.Dest = &dest
    }
    if plan.Search != nil {
        search := *plan.Search
        search.Avoid = make([]WalkAvoidCircle, len(plan.Search.Avoid))
        copy(search.Avoid, plan.Search.Avoid)
        record.Search = &search
    }
    record.Points = make([]WalkPoint, len(plan.Points))
    copy(record.Points, plan.Points)
    b.lastWalkPlan = &record
    b.lastWalkPlanAt = time.Now()
    b.lastWalkStart = b.walkPlanStart
    b.lastWalkWpAt = make([]time.Time, len(b.walkWpAt))
    copy(b.lastWalkWpAt, b.walkWpAt)
}

// ClearWalkPlan drops the published walk plan (a no-op when none
// is published).
func (b *Bot) ClearWalkPlan() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.clearWalkPlanLocked()
}

// clearWalkPlanLocked drops the walk plan, the caller must hold
// the state write lock. The timing view drops with the plan (the
// last walk record kept its own copy, see rememberWalkPlanLocked).
func (b *Bot) clearWalkPlanLocked() {
    if b.walkPlan == nil {
        return
    }
    b.walkPlan = nil
    b.walkPlanAt = time.Time{}
    b.walkPlanStart = time.Time{}
    b.walkWpAt = nil
    b.touch()
}

// walkPlansSameRoute compares two walk plans ignoring the follower
// cursor: the planning origin, the destination and the point list
// decide. The every tick republish of one walk rides this (the
// timing view survives, see publishWalkPlanLocked), a re-planned
// walk over the same waypoints with a restarted cursor does not.
func walkPlansSameRoute(a, b WalkPlan) bool {
    a.Index, b.Index = 0, 0

    return walkPlansEqual(a, b)
}

// walkPlansEqual compares two walk plans field by field, the points
// element wise.
func walkPlansEqual(a, b WalkPlan) bool {
    if (a.Origin == nil) != (b.Origin == nil) ||
        (a.Dest == nil) != (b.Dest == nil) ||
        (a.Search == nil) != (b.Search == nil) ||
        a.Index != b.Index || len(a.Points) != len(b.Points) {
        return false
    }
    if a.Search != nil && !walkSearchesEqual(*a.Search, *b.Search) {
        return false
    }
    if a.Origin != nil && *a.Origin != *b.Origin {
        return false
    }
    if a.Dest != nil && *a.Dest != *b.Dest {
        return false
    }
    for i := range a.Points {
        if a.Points[i] != b.Points[i] {
            return false
        }
    }

    return true
}

// walkSearchesEqual compares two search contracts field by field,
// the ban circles element wise.
func walkSearchesEqual(a, b WalkSearch) bool {
    if a.Approach != b.Approach || len(a.Avoid) != len(b.Avoid) {
        return false
    }
    for i := range a.Avoid {
        if a.Avoid[i] != b.Avoid[i] {
            return false
        }
    }

    return true
}
