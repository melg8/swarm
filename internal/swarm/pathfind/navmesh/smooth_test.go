// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// smoothCornerWorld is the L corridor of two cell wide arms: the west
// arm A (cells 0..4 x 0..2), the north arm B (cells 0..2 x 2..4), the
// inner wall corner at the cell boundary (32, 32) world. The shared
// edge A.maxy/B.miny spans the full arm width (cells 0..2).
//
//      ^ y
//    64 +---+
//       | B |
//    32 +---+   <- the inner corner (32, 32) world
//       | A |
//     0 +---+
//       0  32  64 > x
func smoothCornerWorld() *Tile {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 4, y1: 2, h: 0, area: AreaGround}, // 0 A
        {x0: 0, y0: 2, x1: 2, y1: 4, h: 0, area: AreaGround}, // 1 B
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxY, to: 1, t0: 0, t1: 1},
        {poly: 1, side: SideMinY, to: 0, t0: 0, t1: 1},
    }

    return assembleTile(21, rects, links, nil)
}

// TestSmoothKeepsCornerUnderCapsule pins the wall rule of the
// shortcut pass: the direct chord of the L corridor passes 5.66 units
// from the inner wall corner - inside the 7.5 capsule radius - so the
// pass refuses it and the funnel corner survives the smoothing.
func TestSmoothKeepsCornerUnderCapsule(t *testing.T) {
    mesh := NewMesh(writeTiles(t, smoothCornerWorld()))
    filter := DefaultFilter()
    filter.WaypointClearance = 7.5
    filter.Smooth = true
    route, err := mesh.Route(worldPos(3.5, 0.5, 0), worldPos(0.5, 3.5, 0),
        filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.RawWaypoints, 3)
    require.Len(t, route.Waypoints, 3,
        "the chord grazing the inner corner must be refused")
    // The raw answer is the funnel: the same three waypoints.
    require.Equal(t, route.RawWaypoints, route.Waypoints)
}

// TestSmoothWithoutArmingKeepsRawFunnel pins the contract of the
// untouched filter: no clearance, no smoothing, no raw variant. The
// raw funnel of the unarmed query even cuts the corner (the crossing
// sits 8 units off the inner wall - legal only without a capsule),
// which is exactly what the armed clearance exists to prevent.
func TestSmoothWithoutArmingKeepsRawFunnel(t *testing.T) {
    mesh := NewMesh(writeTiles(t, smoothCornerWorld()))
    route, err := mesh.Route(worldPos(3.5, 0.5, 0), worldPos(0.5, 3.5, 0),
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Waypoints, 2)
    require.Nil(t, route.RawWaypoints)
}

// smoothTWorld is the T junction world: the field A (cells 0..16
// squared) opens east into the stacked pair B (south) and C (north)
// through the full shared edge, and B and C are linked along their
// shared edge (the walkable boundary the pivot offset treats as a
// wall). The corridor A to B crosses the portal whose north span end
// adjoins the OPEN A-C span - the open continuation keeps the span
// end at its full extent and the raw funnel crosses it straight.
func smoothTWorld() *Tile {
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 16, y1: 16, h: 0, area: AreaGround},  // 0 A
        {x0: 16, y0: 0, x1: 32, y1: 8, h: 0, area: AreaGround},  // 1 B
        {x0: 16, y0: 8, x1: 32, y1: 16, h: 0, area: AreaGround}, // 2 C
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxX, to: 1, t0: 0, t1: 7},
        {poly: 1, side: SideMinX, to: 0, t0: 0, t1: 7},
        {poly: 0, side: SideMaxX, to: 2, t0: 8, t1: 15},
        {poly: 2, side: SideMinX, to: 0, t0: 8, t1: 15},
        {poly: 1, side: SideMaxY, to: 2, t0: 16, t1: 31},
        {poly: 2, side: SideMinY, to: 1, t0: 16, t1: 31},
    }

    return assembleTile(21, rects, links, nil)
}

// TestFunnelWalksOpenSpanEndStraight pins the open span end contract
// of the clearance (the owner zigzag fix): the span end adjoining
// another open span stands on no wall, the clearance keeps its
// extent and the raw funnel walks the straight chord through the
// open boundary - the pull of the shrunk end answered a pivot there
// and the walk micro steered the junction.
func TestFunnelWalksOpenSpanEndStraight(t *testing.T) {
    mesh := NewMesh(writeTiles(t, smoothTWorld()))
    filter := DefaultFilter()
    filter.WaypointClearance = 7.5
    filter.Smooth = true
    // The straight chord start-end crosses the A-B portal at world y
    // (216+32)/2 = 124: inside the open span (0..128), at the span
    // end (128) the open A-C span adjoins.
    route, err := mesh.Route(worldPos(2, 13.5, 0), worldPos(30, 2, 0),
        filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.RawWaypoints, 2,
        "the open span end keeps its extent, the funnel walks straight")
    require.Len(t, route.Waypoints, 2)
    // The merged answer keeps the endpoints.
    require.Equal(t, route.RawWaypoints[0], route.Waypoints[0])
    require.Equal(t, route.RawWaypoints[len(route.RawWaypoints)-1],
        route.Waypoints[len(route.Waypoints)-1])
}

// TestSmoothMergesGranularStairPivots walks a long flat corridor of
// one-cell wide strips (the granular pivot farm of the exact square
// mesh) and pins the pass at scale: every merged leg keeps the
// capsule clearance from the walls, the answer never grows.
func TestSmoothMergesGranularStairPivots(t *testing.T) {
    // One long hall of four stacked strips with staggered side walls:
    // the strip joints pinch the funnel into pivots every strip, the
    // wide hall lets the chords thread the joints.
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 16, y1: 4, h: 0, area: AreaGround},
        {x0: 0, y0: 4, x1: 16, y1: 8, h: 0, area: AreaGround},
        {x0: 0, y0: 8, x1: 16, y1: 12, h: 0, area: AreaGround},
        {x0: 0, y0: 12, x1: 16, y1: 16, h: 0, area: AreaGround},
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxY, to: 1, t0: 0, t1: 15},
        {poly: 1, side: SideMinY, to: 0, t0: 0, t1: 15},
        {poly: 1, side: SideMaxY, to: 2, t0: 0, t1: 15},
        {poly: 2, side: SideMinY, to: 1, t0: 0, t1: 15},
        {poly: 2, side: SideMaxY, to: 3, t0: 0, t1: 15},
        {poly: 3, side: SideMinY, to: 2, t0: 0, t1: 15},
    }
    mesh := NewMesh(writeTiles(t, assembleTile(21, rects, links,
        nil)))
    filter := DefaultFilter()
    filter.WaypointClearance = 7.5
    filter.Smooth = true
    route, err := mesh.Route(worldPos(1, 2, 0), worldPos(15, 14, 0),
        filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.NotNil(t, route.RawWaypoints)
    require.LessOrEqual(t, len(route.Waypoints), len(route.RawWaypoints))
    // Every merged leg stays inside the hall: the wall spans of the
    // hall border (x 0 and x 256 world) keep the 7.5 clearance along
    // every leg of the smoothed answer.
    for i := 0; i+1 < len(route.Waypoints); i++ {
        a, b := route.Waypoints[i], route.Waypoints[i+1]
        require.GreaterOrEqual(t, a.X, 32768+7.5-1e-6,
            "leg %d start hugs the west wall", i)
        require.GreaterOrEqual(t, b.X, 32768+7.5-1e-6,
            "leg %d end hugs the west wall", i)
        require.LessOrEqual(t, a.X, 32768+16*16-7.5+1e-6,
            "leg %d start hugs the east wall", i)
        require.LessOrEqual(t, b.X, 32768+16*16-7.5+1e-6,
            "leg %d end hugs the east wall", i)
    }
}

// TestSmoothRespectsPortalSpans pins the span rule on a hand built
// corridor: the corridor A-B-D detours around the void block between
// the arms; the straight chord start-end crosses the west edge
// outside the bridge portal (through the block) and the pass refuses
// it - the detour pivots survive.
func TestSmoothRespectsPortalSpans(t *testing.T) {
    // The U around the void: west column A (cells 0..2 x 0..16), the
    // north bridge B (cells 2..4 x 12..16), the east column D (cells
    // 4..6 x 0..16); the void sits at cells 2..4 x 0..12.
    rects := []rectSpec{
        {x0: 0, y0: 0, x1: 2, y1: 16, h: 0, area: AreaGround},  // 0 A
        {x0: 2, y0: 12, x1: 4, y1: 16, h: 0, area: AreaGround}, // 1 B
        {x0: 4, y0: 0, x1: 6, y1: 16, h: 0, area: AreaGround},  // 2 D
    }
    links := []linkSpec{
        {poly: 0, side: SideMaxX, to: 1, t0: 12, t1: 15},
        {poly: 1, side: SideMinX, to: 0, t0: 12, t1: 15},
        {poly: 1, side: SideMaxX, to: 2, t0: 12, t1: 15},
        {poly: 2, side: SideMinX, to: 1, t0: 12, t1: 15},
    }
    mesh := NewMesh(writeTiles(t, assembleTile(21, rects, links,
        nil)))
    filter := DefaultFilter()
    filter.WaypointClearance = 7.5
    filter.Smooth = true
    // The straight chord between the column middles runs through the
    // void block: the funnel detours over the north bridge and the
    // pass keeps every pivot of the detour.
    route, err := mesh.Route(worldPos(1, 8, 0), worldPos(5, 8, 0), filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.GreaterOrEqual(t, len(route.RawWaypoints), 4,
        "the detour around the block pivots at least twice")
    require.Equal(t, route.RawWaypoints, route.Waypoints,
        "no chord may cut through the void")
}

// TestSmoothPortalIndexChain pins the walk address contract of the
// funnel waypoints: the portal indices strictly grow along the
// answer, the start sits before the first portal and the end on the
// last corridor polygon.
func TestSmoothPortalIndexChain(t *testing.T) {
    mesh := NewMesh(writeTiles(t, corridorWorld()))
    wps := mesh.straightPathWps([]PolyRef{}, Pos{}, Pos{}, 0)
    require.Nil(t, wps)

    route, err := mesh.Route(worldPos(88, 88, 0), worldPos(240, 264, -80),
        DefaultFilter())
    require.NoError(t, err)
    require.True(t, route.Found)
    wps = mesh.straightPathWps(route.Corridor, worldPos(88, 88, 0),
        worldPos(240, 264, -80), 7.5)
    require.GreaterOrEqual(t, len(wps), 3)
    require.Equal(t, int32(-1), wps[0].portal)
    for i := 1; i < len(wps); i++ {
        require.Greater(t, wps[i].portal, wps[i-1].portal,
            "waypoint %d of the chain", i)
    }
    require.Equal(t, int32(len(route.Corridor)-1),
        wps[len(wps)-1].portal)
}

// guardStub is the scripted wall oracle: every chord whose id sits in
// the refused set answers false, the rest true.
type guardStub struct {
    refused map[[2]int]bool
    asked   int
}

func (g *guardStub) LegClear(ax, ay, _ float64,
    _, _, _, _ float64,
) bool {
    g.asked++

    return !g.refused[[2]int{int(ax), int(ay)}]
}

// TestSmoothGuardRefusesChord pins the wall oracle of the shortcut
// pass: the guard the filter arms answers every chord before the
// mesh wall spans, a refused chord keeps the funnel pivots and an
// allowing guard merges them - the corridor portal crossings stay
// the hard rule under both. The L corridor arms the case: the funnel
// pivots at the inner wall corner and the direct chord grazes it.
func TestSmoothGuardRefusesChord(t *testing.T) {
    mesh := NewMesh(writeTiles(t, smoothCornerWorld()))
    start := worldPos(3.5, 0.5, 0)
    end := worldPos(0.5, 3.5, 0)

    // The allowing guard: the merge of the corner chord goes through
    // (the guard the filter arms overrules the mesh wall spans).
    allowing := &guardStub{}
    filter := DefaultFilter()
    filter.WaypointClearance = 7.5
    filter.Smooth = true
    filter.Guard = allowing
    route, err := mesh.Route(start, end, filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Waypoints, 2)
    require.Positive(t, allowing.asked, "the guard answers chords")

    // The refusing guard: the chord from the start is refused, the
    // funnel pivot survives.
    refusing := &guardStub{refused: map[[2]int]bool{
        {int(start.X), int(start.Y)}: true,
    }}
    filter.Guard = refusing
    route, err = mesh.Route(start, end, filter)
    require.NoError(t, err)
    require.True(t, route.Found)
    require.Len(t, route.Waypoints, 3,
        "the refused chord keeps the funnel pivots")
}
