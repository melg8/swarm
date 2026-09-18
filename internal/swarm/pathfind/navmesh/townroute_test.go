// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "os"
    "strconv"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// townLegs are the classic trip legs the owner named: the Elven
// Village to Gludio, Gludio to Gludin, Gludin to Giran - the whole
// map scale routes the bot fleet has to plan fast. The coordinates
// are the teleporter arrival points of the Mobius C1 data
// (dist/game/data/teleporters/town).
type townLeg struct {
    name  string
    start Pos
    end   Pos
}

var townLegs = []townLeg{
    {"ElvenToGludio", Pos{X: 46890, Y: 51531, Z: -2976},
        Pos{X: -12787, Y: 122779, Z: -3114}},
    {"GludioToGludin", Pos{X: -12787, Y: 122779, Z: -3114},
        Pos{X: -80826, Y: 149775, Z: -3043}},
    {"GludinToGiran", Pos{X: -80826, Y: 149775, Z: -3043},
        Pos{X: 83336, Y: 147972, Z: -3404}},
}

// TestNavmeshTownRoutes runs every leg as a subtest (each leg is one
// -run filter away: the cold whole map searches outlast a single
// sandbox process, the subtests separate them). The mesh capacity
// rides SWARM_TOWN_CAPACITY (the production default is 4, the
// thrash free comparison passes a bigger one).
func TestNavmeshTownRoutes(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    // The corridor pack serves the town legs: the three legs ride
    // the regions 16..22 x 19..22, the whole map pack answers the
    // same queries with the coarse cache pressure on top.
    if len(mesh.TileFiles()) < 20 {
        t.Skip("the town corridor pack is not present")
    }
    mesh.SetCacheCapacity(townCapacity())

    for _, l := range townLegs {
        t.Run(l.name, func(t *testing.T) {
            runTownLeg(t, mesh, l.name, l.start, l.end)
        })
    }
}

// runTownLeg measures one leg: the coarse chain phase, the hop
// refinement phase and the production Route answer (cold and warm).
func runTownLeg(t *testing.T, mesh *Mesh, name string, start, end Pos) {
    startRef, startPos, ok := mesh.FindNearestPoly(start)
    require.True(t, ok, "%s: the start must bind", name)
    endRef, endPos, ok := mesh.FindNearestPoly(end)
    require.True(t, ok, "%s: the end must bind", name)
    sc, sr := TileOf(startRef)
    ec, er := TileOf(endRef)
    t.Logf("%s: tiles %d_%d -> %d_%d, straight %.0f units", name,
        sc, sr, ec, er, dist3(startPos, endPos))

    // The production answer first: the cold search (the hop cache is
    // as empty as this test left it) and the warm repeat.
    began := time.Now()
    route, err := mesh.Route(startPos, endPos, DefaultFilter())
    require.NoError(t, err)
    cold := time.Since(began)
    t.Logf("  cold: found=%t partial=%t hier=%t explored=%d"+
        " corridor=%d wps=%d length=%.0f %s", route.Found,
        route.Partial, route.Hierarchical, route.Explored,
        len(route.Corridor), len(route.Waypoints),
        routeLength(route), cold)
    if !route.Found {
        // The one shot answer is a measurement, not a gate: the
        // geodata seams strand some one shot queries (the report
        // names them) - the segmented walk is the bot's answer.
        t.Logf("  (the one shot route misses the town: the segmented" +
            " walk measures the practical answer)")
    }

    began = time.Now()
    warm, err := mesh.Route(startPos, endPos, DefaultFilter())
    require.NoError(t, err)
    t.Logf("  warm: found=%t explored=%d wps=%d length=%.0f %s",
        warm.Found, warm.Explored, len(warm.Waypoints),
        routeLength(warm), time.Since(began))

    // The segmented walk: the way the bot actually covers a trip the
    // one shot query strands - walk the partial answer, re-plan from
    // its end (the hunt loop re-paths constantly; the geodata holes
    // over the bays make the straight line aims unbindable, the
    // partial corridors follow the honest shoreline instead).
    began = time.Now()
    current := startPos
    iterations, walked := 0, 0.0
    reached := false
    for iterations < 12 {
        iterations++
        leg, err := mesh.Route(current, endPos, DefaultFilter())
        require.NoError(t, err)
        if leg.Found {
            wps := leg.Waypoints
            if len(wps) > 1 {
                walked += dist3(current, wps[len(wps)-1])
            }
            reached = true

            break
        }
        // The partial answer: advance to the waypoint pair past the
        // last one (the walk consumes it, the next plan starts from
        // the arrival).
        wps := leg.Waypoints
        if len(wps) < 2 {
            break
        }
        next := wps[len(wps)-1]
        if dist3(current, next) < 1000 {
            break
        }
        walked += dist3(current, next)
        current = next
    }
    total := time.Since(began)
    t.Logf("  segmented: %d replans, reached=%t, walked %.0f of"+
        " %.0f, %s", iterations, reached, walked,
        dist3(startPos, endPos), total)
}

// townCapacity reads the mesh capacity override of the test run
// (SWARM_TOWN_CAPACITY, the production default 4).
func townCapacity() int {
    capacity := 4
    if text := os.Getenv("SWARM_TOWN_CAPACITY"); text != "" {
        if n, err := strconv.Atoi(text); err == nil && n > 0 {
            capacity = n
        }
    }

    return capacity
}
