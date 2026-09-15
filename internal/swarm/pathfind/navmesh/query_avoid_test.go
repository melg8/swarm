// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// avoidWorld is the synthetic two-lane world of the avoid tests: the
// mainland M west, the north lane N and the south lane S east of it
// and the far mainland E - two parallel corridors between the same
// endpoints, so a ban over one lane forces the detour through the
// other.
//
//    y
//    ^
//    320  +---N---+
//         |       +----E----+
//         M   +---S---+     |
//    0     +---+       +----+
//          0   160   320   480  >x
func avoidWorld() *Tile {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 160, y1: 320, h: 0, area: AreaGround},     // 0 M
        {x0: 160, y0: 0, x1: 320, y1: 160, h: 0, area: AreaGround},   // 1 N
        {x0: 160, y0: 160, x1: 320, y1: 320, h: 0, area: AreaGround}, // 2 S
        {x0: 320, y0: 0, x1: 480, y1: 320, h: 0, area: AreaGround},   // 3 E
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxX, to: 1, t0: 0, t1: 159},
        {poly: 1, side: SideMinX, to: 0, t0: 0, t1: 159},
        {poly: 0, side: SideMaxX, to: 2, t0: 160, t1: 319},
        {poly: 2, side: SideMinX, to: 0, t0: 160, t1: 319},
        {poly: 1, side: SideMaxX, to: 3, t0: 0, t1: 159},
        {poly: 3, side: SideMinX, to: 1, t0: 0, t1: 159},
        {poly: 2, side: SideMaxX, to: 3, t0: 160, t1: 319},
        {poly: 3, side: SideMinX, to: 2, t0: 160, t1: 319},
    }

    return assembleTile(21, 19, rects, links, nil)
}

// TestRouteApproachRadiusEndsEarly pins the approach goal of the
// corridor search: the water target of the corridor world answers a
// full four polygon corridor under the exact contract, and a wide
// enough approach radius ends the same walk on the shore ramp before
// any water polygon enters the corridor.
func TestRouteApproachRadiusEndsEarly(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    start := worldPos(8, 8, 0)
    end := worldPos(240, 264, -80)

    exact, err := mesh.Route(start, end, DefaultFilter())
    require.NoError(t, err)
    require.True(t, exact.Found)
    require.Len(t, exact.Corridor, 4)

    // The shore ramp C (poly 2) holds the closest dry surface point
    // of the water walk: its footprint clamps the water end 896
    // units north and 40 units up - a radius of 1000 accepts it.
    approach, err := mesh.RouteApproach(start, end, 1000, DefaultFilter())
    require.NoError(t, err)
    require.True(t, approach.Found)
    require.False(t, approach.Partial)
    require.Len(t, approach.Corridor, 3)
    require.Equal(t, int32(2), PolyOf(approach.Corridor[2]))
    tile, poly := mesh.polyOfRef(approach.Corridor[len(approach.Corridor)-1])
    require.NotNil(t, tile)
    require.Equal(t, AreaGround, poly.Area)
    require.InDelta(t, -40.0,
        approach.Waypoints[len(approach.Waypoints)-1].Z, 1e-9)
}

// TestRouteApproachStartWithinRadius pins the approach goal firing on
// the start polygon itself: the end sits one lane east of the start,
// the radius covers the gap, and the answer is the one polygon
// corridor whose final waypoint projects the end back onto the start
// surface.
func TestRouteApproachStartWithinRadius(t *testing.T) {
    mesh := NewMesh(writeTiles(t, avoidWorld()))
    start := worldPos(150, 80, 0)
    end := worldPos(170, 80, 0)

    route, err := mesh.RouteApproach(start, end, 300, DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Corridor, 1)
    require.GreaterOrEqual(t, len(route.Waypoints), 2)
    // The final waypoint is the projection of the end onto the start
    // polygon: clamped onto its east edge, surface height 0.
    final := route.Waypoints[len(route.Waypoints)-1]
    require.InDelta(t, end.X-160, final.X, 1e-6)
    require.InDelta(t, end.Y, final.Y, 1e-6)
    require.InDelta(t, 0, final.Z, 1e-9)

    // The exact contract needs the crossing into the east lane.
    exact, err := mesh.Route(start, end, DefaultFilter())
    require.NoError(t, err)
    require.True(t, exact.Found)
    require.Len(t, exact.Corridor, 2)
}

// TestRouteAvoidDetour pins the recovery ban of the corridor search:
// the north lane banned, the same walk detours through the south lane
// and no waypoint of the funnelled route enters the banned disk.
func TestRouteAvoidDetour(t *testing.T) {
    mesh := NewMesh(writeTiles(t, avoidWorld()))
    start := worldPos(80, 160, 0)
    end := worldPos(400, 160, 0)

    plain, err := mesh.Route(start, end, DefaultFilter())
    require.NoError(t, err)
    require.True(t, plain.Found)

    filter := DefaultFilter()
    filter.Avoid = []AvoidCircle{{
        // The north lane center: radius 600 covers the lane middle
        // without touching M, S or E (the lane spans 2560 world
        // units; the ban walls it whole).
        CenterX: 32768 + 240*16,
        CenterY: 32768 + 80*16,
        Radius:  600,
    }}
    banned, err := mesh.Route(start, end, filter)
    require.NoError(t, err)
    require.True(t, banned.Found)

    // The corridor runs through the south lane, never the north one.
    north := false
    south := false
    for _, ref := range banned.Corridor {
        if PolyOf(ref) == 1 {
            north = true
        }
        if PolyOf(ref) == 2 {
            south = true
        }
    }
    require.False(t, north)
    require.True(t, south)

    // No funnelled waypoint stands inside the banned disk.
    for _, wp := range banned.Waypoints {
        dist := math.Hypot(wp.X-filter.Avoid[0].CenterX,
            wp.Y-filter.Avoid[0].CenterY)
        require.Greater(t, dist, filter.Avoid[0].Radius,
            "waypoint %v inside the ban", wp)
    }
}

// TestRouteAvoidWallsWholeRoute pins the sealed goal: a ban over the
// only corridor of the corridor world leaves no route at all - no
// partial corridor, no waypoints (the ban exists to make the
// deterministic planner give up on that ground).
func TestRouteAvoidWallsWholeRoute(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    start := worldPos(8, 8, 0)
    end := worldPos(240, 264, -80)

    filter := DefaultFilter()
    filter.Avoid = []AvoidCircle{{
        // The deck B center: every A -> water route crosses B.
        CenterX: 32768 + 240*16,
        CenterY: 32768 + 80*16,
        Radius:  600,
    }}
    route, err := mesh.Route(start, end, filter)
    require.NoError(t, err)
    require.False(t, route.Found)
    require.False(t, route.Partial)
    require.Empty(t, route.Corridor)
    require.Empty(t, route.Waypoints)
}

// TestRouteAvoidEscapeWayOut pins the way-out rule: the ban holding
// the start is passable near the start (the escape ring, priced
// heavily) and sealed beyond it. A start inside the border ban - one
// that touches both lanes - plans out through an escape lane; a start
// deeper inside the same ban, with both lanes beyond the 256 unit
// escape ring, has no route at all.
func TestRouteAvoidEscapeWayOut(t *testing.T) {
    mesh := NewMesh(writeTiles(t, avoidWorld()))
    end := worldPos(400, 160, 0)
    // Centered on the middle of M's east border: the disk touches M
    // and the near corners of BOTH lanes.
    ban := AvoidCircle{
        CenterX: 32768 + 160*16,
        CenterY: 32768 + 160*16,
        Radius:  600,
    }
    filter := DefaultFilter()
    filter.Avoid = []AvoidCircle{ban}

    // The start 160 units west of the border sits inside the ban;
    // both lane footprints reach within the 256 unit escape ring of
    // it, so an escape lane opens at the heavy multiplier.
    nearStart := worldPos(150, 160, 0)
    require.LessOrEqual(t,
        math.Hypot(nearStart.X-ban.CenterX, nearStart.Y-ban.CenterY),
        ban.Radius)
    route, err := mesh.Route(nearStart, end, filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.NotEmpty(t, route.Waypoints)

    // The start 576 units west of the border still sits inside the
    // 600 unit ban, but both lanes lie beyond the 256 unit escape
    // ring: the own ban ground stays sealed and no lane opens.
    farStart := worldPos(124, 160, 0)
    require.LessOrEqual(t,
        math.Hypot(farStart.X-ban.CenterX, farStart.Y-ban.CenterY),
        ban.Radius)
    route, err = mesh.Route(farStart, end, filter)
    require.NoError(t, err)
    require.False(t, route.Found)
}

// TestRouteAvoidForeignBanWins pins the foreign ban precedence: the
// escape ring of the own ban never opens ground a foreign ban
// touches - the route out of the own ban detours through the free
// lane instead of threading the foreign banned one.
func TestRouteAvoidForeignBanWins(t *testing.T) {
    mesh := NewMesh(writeTiles(t, avoidWorld()))
    end := worldPos(400, 160, 0)
    own := AvoidCircle{
        // Holding the start: touches M and the near corners of both
        // lanes.
        CenterX: 32768 + 160*16,
        CenterY: 32768 + 160*16,
        Radius:  600,
    }
    foreign := AvoidCircle{
        // Over the north lane only: well inside N, clear of M, S and
        // E.
        CenterX: 32768 + 260*16,
        CenterY: 32768 + 80*16,
        Radius:  300,
    }
    filter := DefaultFilter()
    filter.Avoid = []AvoidCircle{own, foreign}

    start := worldPos(150, 160, 0)
    route, err := mesh.Route(start, end, filter)
    require.NoError(t, err)
    require.True(t, route.Found)

    // The north lane would be escape ground of the own ban (its
    // footprint reaches within the escape ring of the start), but the
    // foreign ban keeps it walled: the corridor detours south.
    north := false
    south := false
    for _, ref := range route.Corridor {
        if PolyOf(ref) == 1 {
            north = true
        }
        if PolyOf(ref) == 2 {
            south = true
        }
    }
    require.False(t, north)
    require.True(t, south)
}
