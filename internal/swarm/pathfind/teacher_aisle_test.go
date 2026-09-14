// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// The round 56 acceptance: the bot must reliably reach the teacher
// Ellenia (45725 52105 -2792) from every walking position of the
// elven village - the trainer hall aisle the dump froze in, the
// pocket the failed recovery crept into, the terraces around it and
// the shop quarter. The trainer hall building carries three geodata
// layers (the sloped roof -2600..-2448, the walkable floor -2792 with
// the walls in the NSWE flags, the water deck -3928) and its interior
// east of the aisle has no floor layer at all, so a route may only
// enter through the west aisle column or around the building - the
// tests pin that every plan the search builds for Ellenia consists of
// legs the server click validation accepts in full (the round 52
// port), from every approach direction.
var elleniaSpawn = Vec3{X: 45725, Y: 52105, Z: -2792}

// elleniaApproaches are the village positions the walk to Ellenia
// starts from: the dump aisle entrance and the trap pocket east of it,
// the north terrace and the south approach around the trainer hall,
// the east plaza the current planner routes through and the two shop
// quarter spots.
var elleniaApproaches = []struct {
    name string
    pos  Vec3
}{
    {"the dump aisle entrance", Vec3{X: 44728, Y: 51992, Z: -2792}},
    {"the trap pocket", Vec3{X: 44776, Y: 51992, Z: -2808}},
    {"the north terrace bend", Vec3{X: 44568, Y: 51528, Z: -2808}},
    {"the south approach", Vec3{X: 44568, Y: 52536, Z: -2832}},
    {"the east plaza", Vec3{X: 46104, Y: 51528, Z: -2808}},
    {"the shop deck", Vec3{X: 44872, Y: 47160, Z: -2992}},
    {"the southwest shore path", Vec3{X: 43752, Y: 48024, Z: -2992}},
}

// TestElleniaReachableFromEveryVillageApproach walks the dry approach
// search from every village position to Ellenia and verifies the two
// invariants the follower needs: the route ends within the trip
// approach radius of the teacher, and EVERY leg of the plan is a
// click the server would accept in full (the validated destination is
// the clicked waypoint itself) - a plan of fully validated legs can
// never arm the collapsed partial click that crept the dump character
// into the dead-end pocket.
func TestElleniaReachableFromEveryVillageApproach(t *testing.T) {
    engine := townTestEngine(t)

    for _, approach := range elleniaApproaches {
        t.Run(approach.name, func(t *testing.T) {
            result, err := engine.FindPathApproachDry(
                approach.pos, elleniaSpawn, 200, DefaultMaxPassableHeight)
            require.NoError(t, err)
            require.True(t, result.Found,
                "the dry route to Ellenia must exist from %s", approach.name)

            last := result.Waypoints[len(result.Waypoints)-1]
            require.LessOrEqual(t, dist3D(last, elleniaSpawn), 200.0,
                "the route must end inside the Ellenia approach ring")

            for i := 1; i < len(result.Waypoints); i++ {
                from := result.Waypoints[i-1]
                to := result.Waypoints[i]
                validated, ok := engine.ValidateClick(from, to)
                require.True(t, ok,
                    "the leg %d -> %d must survive the server validation",
                    i-1, i)
                require.InDelta(t, to.X, validated.X, 1.0,
                    "the leg %d must validate in full (no partial collapse)", i)
                require.InDelta(t, to.Y, validated.Y, 1.0,
                    "the leg %d must validate in full (no partial collapse)", i)
            }
        })
    }
}

// TestElleniaAisleRouteMatchesTheDumpPlan pins the exact dump walk
// from the aisle entrance: the route must enter the building through
// the aisle column south (the 48 unit leg the dump carried as wp 2)
// and cross the south hall east - the same plan the 06:19 state dump
// shows, now verified as fully server-valid.
func TestElleniaAisleRouteMatchesTheDumpPlan(t *testing.T) {
    engine := townTestEngine(t)

    result, err := engine.FindPathApproachDry(
        elleniaApproaches[0].pos, elleniaSpawn, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, result.Found)

    // The dump plan waypoints (44728 51992 -> 44728 52040 ->
    // 45160 52120 -> 45528 52104): every one must appear in the route.
    for _, want := range []Vec3{
        {X: 44728, Y: 52040, Z: -2792},
        {X: 45160, Y: 52120, Z: -2792},
        {X: 45528, Y: 52104, Z: -2792},
    } {
        found := false
        for _, wp := range result.Waypoints {
            if dist3D(wp, want) < 64 {
                found = true

                break
            }
        }
        require.True(t, found,
            "the route must pass the dump waypoint %.0f %.0f", want.X, want.Y)
    }
}

// TestElleniaPocketLinesRefuseTheHallClick pins the trap itself: the
// straight click from the pocket cell (44776 51992 -2808) toward the
// east hall waypoint (45160 52120) is refused by the ported server
// rules (the pocket's east wall is closed) while the click from the
// aisle entrance (44728 51992) runs only PARTIALLY - it collapses
// onto the first step east. These are the two answers the round 56
// skip gate protects the follower from arming.
func TestElleniaPocketLinesRefuseTheHallClick(t *testing.T) {
    engine := townTestEngine(t)

    hall := Vec3{X: 45160, Y: 52120, Z: -2792}
    pocket := Vec3{X: 44776, Y: 51992, Z: -2808}
    aisle := Vec3{X: 44728, Y: 51992, Z: -2792}

    // From the pocket: the first Bresenham step east hits the closed
    // east wall, the click collapses onto the walker - a refusal.
    _, ok := engine.ValidateClick(pocket, hall)
    require.False(t, ok,
        "the hall click from the pocket must be refused (the closed east wall)")

    // From the aisle entrance: the click runs but stops at the first
    // step - the partial that crept the dump character east.
    validated, ok := engine.ValidateClick(aisle, hall)
    require.True(t, ok, "the hall click from the aisle runs partially")
    require.Less(t, math.Hypot(validated.X-aisle.X, validated.Y-aisle.Y),
        64.0,
        "the hall click from the aisle must collapse onto the first step east")
}

// The frozen corridor ban of the hunt loop recovery (the 2026-09-12
// trainer hall aisle freeze): the session records the aimed waypoint
// of a leg whose re-path produced no movement at all, and every later
// dry search routes around the banned patch - the deterministic
// planner must never reproduce the identical frozen corridor.

// TestAvoidingSearchDetoursAroundTheFrozenAisle pins the ban itself:
// the dry approach search from the aisle entrance to Ellenia normally
// walks the west aisle column (the dump plan); with the aisle column
// banned, the same search detours around the building - north over
// the terrace, east past the hall, into the approach ring from the
// east. The detour stays dry, ends inside the approach ring and every
// one of its legs survives the server click validation in full (the
// follower only clicks verified lines).
func TestAvoidingSearchDetoursAroundTheFrozenAisle(t *testing.T) {
    engine := townTestEngine(t)

    ban := []AvoidArea{{
        Center: Vec3{X: 44728, Y: 52040, Z: -2792},
        Radius: 48.0,
    }}
    result, err := engine.FindPathApproachDryAvoiding(
        elleniaApproaches[0].pos, elleniaSpawn, 200, DefaultMaxPassableHeight, ban)
    require.NoError(t, err)
    require.True(t, result.Found,
        "the banned aisle must leave the around-the-building route")

    // The detour never enters the banned corridor: every waypoint
    // stays on or outside the ban circle (the start cell of the search
    // may sit exactly on the boundary - the walker standing inside its
    // own patch must be able to plan the way out).
    for i, wp := range result.Waypoints {
        require.GreaterOrEqual(t, math.Hypot(wp.X-ban[0].Center.X, wp.Y-ban[0].Center.Y),
            ban[0].Radius,
            "waypoint %d of the detour must stay outside the banned aisle", i)
    }
    // The detour ends inside the approach ring of the teacher.
    last := result.Waypoints[len(result.Waypoints)-1]
    require.LessOrEqual(t, dist3D(last, elleniaSpawn), 200.0,
        "the detour must end inside the Ellenia approach ring")
    // Every leg of the detour validates in full: the follower clicks
    // only lines the ported server rules accept completely (the
    // degenerate zero length legs of the duplicated plan start are
    // skipped by the follower cursor, they never become clicks).
    for i := 1; i < len(result.Waypoints); i++ {
        from := result.Waypoints[i-1]
        to := result.Waypoints[i]
        if math.Hypot(to.X-from.X, to.Y-from.Y) < 1 {
            continue
        }
        validated, ok := engine.ValidateClick(from, to)
        require.True(t, ok,
            "the detour leg %d must survive the server validation", i)
        require.InDelta(t, to.X, validated.X, 1.0,
            "the detour leg %d must validate in full (no partial collapse)", i)
        require.InDelta(t, to.Y, validated.Y, 1.0,
            "the detour leg %d must validate in full (no partial collapse)", i)
    }
}

// TestAvoidingSearchStillAnswersTheUnbannedRoute pins the empty ban:
// no avoid areas keep the ordinary dry search untouched (the aisle
// route of the dump plan stays the answer).
func TestAvoidingSearchStillAnswersTheUnbannedRoute(t *testing.T) {
    engine := townTestEngine(t)

    plain, err := engine.FindPathApproachDry(
        elleniaApproaches[0].pos, elleniaSpawn, 200, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, plain.Found)

    avoiding, err := engine.FindPathApproachDryAvoiding(
        elleniaApproaches[0].pos, elleniaSpawn, 200, DefaultMaxPassableHeight, nil)
    require.NoError(t, err)
    require.True(t, avoiding.Found)
    require.Len(t, avoiding.Waypoints, len(plain.Waypoints),
        "the empty ban must not change the planned route")
    for i := range plain.Waypoints {
        require.InDelta(t, plain.Waypoints[i].X, avoiding.Waypoints[i].X, 1.0)
        require.InDelta(t, plain.Waypoints[i].Y, avoiding.Waypoints[i].Y, 1.0)
    }
}

// TestAvoidingSearchRefusesGoalsInsideTheBan pins the boundary: a
// goal that only exists inside the banned ground answers not found -
// the caller falls back to its next recovery rung instead of walking
// a route through the corridor the server refused.
func TestAvoidingSearchRefusesGoalsInsideTheBan(t *testing.T) {
    engine := townTestEngine(t)

    ban := []AvoidArea{{
        Center: Vec3{X: 44728, Y: 52040, Z: -2792},
        Radius: 400.0,
    }}
    result, err := engine.FindPathApproachDryAvoiding(
        elleniaApproaches[0].pos, elleniaSpawn, 150, DefaultMaxPassableHeight, ban)
    require.NoError(t, err)
    require.False(t, result.Found,
        "a goal sealed inside the ban must answer not found")
}

// The escape ring of the own ban (the 2026-09-14 08:42 dump): the
// frozen corridor recovery widens its bans by doubling the radius, and
// a character that crept toward the frozen corridor can end up
// standing DEEP inside its own ban - 129 units from the center of a
// 192 radius patch. The strict ban sealed such a start (the search
// expanded one cell and answered no route - every neighbor sat inside
// the ban too), the zone return then fell back to the ban-less search
// and re-planned the very corridor the ban exists to detour. The
// escape ring keeps the neighborhood of the standing cell plannable
// so the search always finds the way OUT of its own ban, while the
// ban beyond the ring and every foreign ban keep their walls.
var (
    // escapeRingStart is the crept standing cell of the dump: 129
    // units from the center of the widened corridor ban below.
    escapeRingStart = Vec3{X: 45352, Y: 49720, Z: -3056}
    // escapeRingBanCenter is the corridor ban the dump's ladder armed
    // at the aimed waypoint of the crept plan.
    escapeRingBanCenter = Vec3{X: 45288, Y: 49832, Z: -2992}
    // escapeRingZone is the held cell center of the dump (the walk
    // goal of the zone return).
    escapeRingZone = Vec3{X: 36000, Y: 46765, Z: -3712}
)

// TestAvoidingSearchEscapesTheOwnBanFromDeepInside pins the unsealing:
// the dry avoiding search from the crept cell (deep inside the widened
// corridor ban, a foreign corridor ban between it and the goal) must
// find the route out - through the escape ring of the own ban, around
// the foreign ban, dry all the way to the zone. The sealed start (the
// pre-fix behavior) answered not found and trapped the trips on the
// ban-less corridor fallback forever.
func TestAvoidingSearchEscapesTheOwnBanFromDeepInside(t *testing.T) {
    engine := townTestEngine(t)
    bans := []AvoidArea{
        {Center: escapeRingBanCenter, Radius: 192.0},
        {Center: Vec3{X: 43512, Y: 50504, Z: -2992}, Radius: 192.0},
    }
    result, err := engine.FindPathApproachDryAvoiding(
        escapeRingStart, escapeRingZone, 200, DefaultMaxPassableHeight, bans)
    require.NoError(t, err)
    require.True(t, result.Found,
        "the deep-interior start of the own ban must stay escapable")
    require.Greater(t, result.Explored, 1,
        "the search must expand past the start cell (the pre-fix seal)")

    // The route leaves the own ban through the ring and never returns:
    // after the first waypoint outside the ban circle, no later
    // waypoint may sit inside it again.
    left := false
    for i, wp := range result.Waypoints {
        inside := math.Hypot(
            wp.X-escapeRingBanCenter.X, wp.Y-escapeRingBanCenter.Y) <= 192
        if !inside {
            left = true

            continue
        }
        require.False(t, left && i > 0,
            "waypoint %d re-enters the own ban after leaving it", i)
        // The in-ban waypoints of the escape stay within the ring.
        require.LessOrEqual(t,
            math.Hypot(wp.X-escapeRingStart.X, wp.Y-escapeRingStart.Y),
            avoidEscapeRadius+16,
            "the escape waypoint %d stays inside the start ring", i)
    }
    // The route never enters the foreign corridor ban.
    for i, wp := range result.Waypoints {
        require.Greater(t, math.Hypot(wp.X-43512, wp.Y-50504), 192.0,
            "waypoint %d must stay outside the foreign corridor ban", i)
    }
}
