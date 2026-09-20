// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "sort"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// hierBenchPair is one measured query of the within tile benchmark.
type hierBenchPair struct {
    name  string
    start Pos
    end   Pos
}

// bindPos resolves a world position onto the mesh: the vertical probe
// walks the height window in overlapping bands (the nearest poly
// window is +/-600, the step stays under the 1200 band width) and an
// xy jitter loop covers the positions inside a wall footprint.
func bindPos(t *testing.T, mesh *Mesh, pos Pos) (PolyRef, Pos) {
    t.Helper()
    for z := -12000.0; z <= 12000.0; z += 800.0 {
        probe := Pos{X: pos.X, Y: pos.Y, Z: z}
        ref, snapped, ok := mesh.FindNearestPoly(probe)
        if ok {
            return ref, snapped
        }
    }
    for radius := 512.0; radius <= 4096.0; radius *= 2 {
        for _, d := range [][2]float64{{1, 0}, {-1, 0}, {0, 1},
            {0, -1}, {0.7, 0.7}, {-0.7, 0.7}, {0.7, -0.7},
            {-0.7, -0.7}} {
            probe := Pos{X: pos.X + radius*d[0],
                Y: pos.Y + radius*d[1], Z: pos.Z}
            ref, snapped, ok := mesh.FindNearestPoly(probe)
            if ok {
                return ref, snapped
            }
        }
    }
    t.Fatalf("the position %.0f %.0f does not bind to the mesh",
        pos.X, pos.Y)

    return 0, Pos{}
}

// routeLength sums the waypoint walk length of a route.
func routeLength(route *Route) float64 {
    length := 0.0
    for i := 1; i < len(route.Waypoints); i++ {
        length += dist3(route.Waypoints[i-1], route.Waypoints[i])
    }

    return length
}

// reportRoute prints one answer line of the benchmark.
func reportRoute(t *testing.T, label string, route *Route,
    elapsed time.Duration,
) {
    t.Helper()
    t.Logf("%s: found=%t partial=%t hier=%t explored=%d corridor=%d"+
        " wps=%d length=%.0f %s", label, route.Found, route.Partial,
        route.Hierarchical, route.Explored, len(route.Corridor),
        len(route.Waypoints), routeLength(route), elapsed)
}

// portalTarget is one candidate end position of the benchmark: the
// midpoint of a portal crossing whose source polygon shares the link
// component of the start (the reachability the component gate
// guarantees for the whole tile walk).
type portalTarget struct {
    pos  Pos
    dist float64
}

// portalTargets lists the portal midpoints of one tile that share the
// start polygon's link component, sorted by the distance to the start
// (the reachable within tile ends the benchmark measures).
func portalTargets(mesh *Mesh, col, row int16, startRef PolyRef,
    startPos Pos,
) []portalTarget {
    abstract := mesh.abstractOf(RegionKey{Col: col, Row: row})
    if abstract == nil {
        return nil
    }
    idx := PolyOf(startRef)
    if idx < 0 || int(idx) >= len(abstract.comps) {
        return nil
    }
    startComp := abstract.comps[idx]
    targets := make([]portalTarget, 0, len(abstract.edges))
    for ei := range abstract.edges {
        edge := &abstract.edges[ei]
        if edge.srcComp != startComp {
            continue
        }
        // The same tile ends only (the within tile benchmark).
        if edge.to.Col != col || edge.to.Row != row {
            continue
        }
        targets = append(targets, portalTarget{
            pos:  edge.mid,
            dist: dist2D(startPos, edge.mid),
        })
    }
    sort.Slice(targets, func(i, j int) bool {
        return targets[i].dist < targets[j].dist
    })

    return targets
}

// TestHierarchyVersusFlat21x19 measures the flat corridor search
// against the hierarchical query on the distances of one region tile
// (21_19, the Elven forest): a short town scale hop, a medium cross
// and the tile diagonal. The ends ride the portal midpoints of the
// start polygon's link component - the reachability the component
// gate guarantees. The flat search runs twice - the production budget
// (maxQueryNodes, the bound the long flat queries used to cap into
// the partial answer) and an uncapped budget (what the flat search
// pays to actually finish). The hierarchical query runs cold and warm
// (the hop cache of the second run serves the portal pair corridors).
// The test skips when the pack is absent.
func TestHierarchyVersusFlat21x19(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    keys := mesh.TileFiles()
    if len(keys) < 100 {
        t.Skipf("the whole map pack is not present (%d tiles)",
            len(keys))
    }
    mesh.SetCacheCapacity(4)

    // The tile facts: the level 0 size and the abstract size of 21_19.
    abstract := mesh.abstractOf(RegionKey{Col: 21, Row: 19})
    require.NotNil(t, abstract, "the 21_19 abstract must build")
    t.Logf("21_19 facts: %d level 0 polys, %d clusters,"+
        " %d portal edges", abstract.polys, len(abstract.nodes),
        len(abstract.edges))

    elven := Pos{X: 46890, Y: 51531, Z: -2976}
    startRef, startPos := bindPos(t, mesh, elven)
    t.Logf("the Elven village start binds at %.0f %.0f %.0f",
        startPos.X, startPos.Y, startPos.Z)

    // The portal derived ends: the closest to 16k and the farthest.
    targets := portalTargets(mesh, 21, 19, startRef, startPos)
    require.NotEmpty(t, targets, "the start component must hold portals")
    medium := targets[0]
    for _, candidate := range targets {
        if candidate.dist >= 16000 {
            medium = candidate

            break
        }
    }
    farthest := targets[len(targets)-1]
    t.Logf("portal derived ends: medium %.0f units, farthest %.0f units",
        medium.dist, farthest.dist)

    pairs := []hierBenchPair{
        {
            name:  "short ~1.5k (village to the east gate)",
            start: startPos,
            end:   Pos{X: 47872, Y: 52736, Z: -3800},
        },
        {
            name:  "medium (village to a ~16k portal)",
            start: startPos,
            end:   medium.pos,
        },
        {
            name:  "diagonal (village to the farthest portal)",
            start: startPos,
            end:   farthest.pos,
        },
    }

    state := mesh.acquireState()
    defer mesh.releaseState(state)

    for _, pair := range pairs {
        startRef, startPos := bindPos(t, mesh, pair.start)
        endRef, endPos := bindPos(t, mesh, pair.end)
        straight := dist2D(startPos, endPos)
        t.Logf("%s: straight line %.0f units (start z %.0f, end z"+
            " %.0f)", pair.name, straight, startPos.Z, endPos.Z)

        // The hierarchical query first: cold and warm (the Route entry
        // selects the hierarchy for the distances above the threshold,
        // the direct call covers the short hop). It is the cheap one
        // and its reachability answer guards the uncapped flat run.
        began := time.Now()
        cold := mesh.routeHierarchical(startRef, startPos, endRef,
            endPos, 0, DefaultFilter(), state)
        coldTime := time.Since(began)
        if cold != nil {
            reportRoute(t, "  hierarchical cold", cold, coldTime)
        } else {
            t.Logf("  hierarchical cold: declined (%s)", coldTime)
        }

        began = time.Now()
        warm := mesh.routeHierarchical(startRef, startPos, endRef,
            endPos, 0, DefaultFilter(), state)
        warmTime := time.Since(began)
        if warm != nil {
            reportRoute(t, "  hierarchical warm", warm, warmTime)
        } else {
            t.Logf("  hierarchical warm: declined (%s)", warmTime)
        }

        // The flat search under the production budget.
        began = time.Now()
        flat := mesh.astar(state,
            astarGoal{target: endRef, approach: 0},
            startRef, startPos, endPos, DefaultFilter(), noAvoid(),
            maxQueryNodes, nil)
        flatTime := time.Since(began)
        t.Logf("  flat budget %d: reached=%t partial=%t explored=%d"+
            " corridor=%d capped=%t %s", maxQueryNodes,
            flat.reached, flat.partial, flat.explored,
            len(flat.corridor), flat.capped, flatTime)

        // The flat search without the cap: the honest flat cost. Only
        // for a reachable pair - an unreachable one floods the whole
        // component (a million expansions, the OOM territory).
        if cold != nil && cold.Found {
            began = time.Now()
            uncapped := mesh.astar(state,
                astarGoal{target: endRef, approach: 0},
                startRef, startPos, endPos, DefaultFilter(),
                noAvoid(), 1<<20, nil)
            uncappedTime := time.Since(began)
            t.Logf("  flat uncapped: reached=%t partial=%t"+
                " explored=%d corridor=%d %s", uncapped.reached,
                uncapped.partial, uncapped.explored,
                len(uncapped.corridor), uncappedTime)
        } else {
            t.Logf("  flat uncapped: skipped (the hierarchy answers" +
                " the pair unreachable, the flat flood would blow" +
                " the memory)")
        }

        // The Route entry for the pair (what the callers see): the
        // bound positions feed both ends.
        began = time.Now()
        route, err := mesh.Route(startPos, endPos,
            DefaultFilter())
        require.NoError(t, err)
        reportRoute(t, "  Route entry", route, time.Since(began))
    }
}
