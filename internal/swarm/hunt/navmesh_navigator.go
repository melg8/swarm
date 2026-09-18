// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The live integration of the navmesh runtime (docs/navmesh.md): the
// route planning serves from the prebuilt tile mesh through the
// Detour-style corridor search - the stacked deck disambiguation, the
// C1 water zone pricing, the funnel string pulling and the recovery
// ban walls - and the mesh is the SOLE route planner (the owner
// directive: the served walk plan must be the mesh answer, never a
// grid plan the funnel pass folds or a fallback route the grid engine
// answers over its own world view). The grid engine stays the click
// validation and local walk layer it already is (ValidateClick, the
// sight lines, the water rasters, the deck heights) and its capsule
// radius arms the mesh funnel clearance, but no route query of this
// navigator ever asks the grid engine for a path.
//
// The partial round: when the mesh answers a closest-reachable
// corridor (the destination unreachable under the filter) the hybrid
// serves the mesh partial waypoints through Result.Partial - the walk
// toward the closest reachable point instead of the bare abort at the
// start position. A destination on ground the sheet decomposition
// dropped answers the honest not found (the manual walk falls back to
// the direct server routed walk, the town legs keep their recovery).

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// navmeshNavigator is the mesh navigator of the live integration: the
// route planning on the mesh, the validation on the grid engine. The
// capsule clearance of the engine (SetCapsuleClearance) governs the
// mesh answers too: the funnel pivots pull inward from the portal
// span ends and the shortcut pass answers to the grid capsule - one
// radius, both surfaces. The merged chords never leave the corridor
// geometry (the portal spans and the wall edges of the polygons the
// corridor walks), so the grid oracle can only keep more funnel
// waypoints, never fold the route across a wall the mesh detours.
type navmeshNavigator struct {
    engine    *pathfind.Engine
    mesh      *navmesh.Mesh
    capsule   *pathfind.Capsule
    clearance float64
}

// NewNavmeshNavigator wraps a geodata engine and a navigation mesh
// into the hunt navigator. Returning the Navigator interface is the
// deliberate seam of the hunt package (see NewNavigator).
func NewNavmeshNavigator( //nolint:ireturn
    engine *pathfind.Engine, mesh *navmesh.Mesh,
) Navigator {
    clearance := engine.CapsuleRadius()
    var capsule *pathfind.Capsule
    if clearance > 0 {
        capsule = pathfind.NewCapsule(engine)
    }

    return navmeshNavigator{
        engine:    engine,
        mesh:      mesh,
        capsule:   capsule,
        clearance: clearance,
    }
}

// clearedFilter arms the funnel pivot clearance of the mesh search
// from the engine's capsule radius, the shortcut pass over the funnel
// answer with the grid capsule as the extra wall oracle, and the C1
// water zone pricing (the same swim pricing the viewer's swim filter
// serves: the water polygons a C1 WaterZone cuboid covers swim at the
// run/swim speed ratio of the player templates, the river beds the
// zone data omits walk at the plain land rate).
func (n navmeshNavigator) clearedFilter(filter navmesh.Filter) navmesh.Filter {
    filter.WaypointClearance = n.clearance
    filter.Smooth = n.clearance > 0
    filter.WaterZones = navmesh.C1WaterZones()
    if n.capsule != nil {
        filter.Guard = n.capsule
    }

    return filter
}

// FindPathApproach plans the walk through the mesh corridor search
// only (the approach radius goal, the swim pricing): the mesh answer
// is the walk plan.
func (n navmeshNavigator) FindPathApproach(
    start, end pathfind.Vec3, approachRadius float64,
) (*pathfind.Result, error) {
    return n.meshQuery(start, end, approachRadius,
        n.clearedFilter(navmesh.DefaultFilter()))
}

// FindPathApproachAvoiding plans the water permitting walk around the
// avoid areas through the mesh corridor search (the ban walls with
// the escape ring of the own ban). The mesh partial corridors surface
// through Result.Partial: the waypoints end at the closest reachable
// point around the bans (the walk-what-you-can contract of the zone
// return fallback).
func (n navmeshNavigator) FindPathApproachAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    filter := n.clearedFilter(navmesh.DefaultFilter())
    filter.Avoid = avoidCircles(avoid)

    return n.meshQuery(start, end, approachRadius, filter)
}

// FindPathApproachDryAvoiding plans the water walled walk around the
// avoid areas through the mesh corridor search: a dry target the mesh
// cannot reach answers the closest-reachable dry partial (the
// waypoints end at the closest reachable dry point, the walk the town
// legs make) or the bare not found.
func (n navmeshNavigator) FindPathApproachDryAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    filter := n.clearedFilter(navmesh.DryFilter())
    filter.Avoid = avoidCircles(avoid)

    return n.meshQuery(start, end, approachRadius, filter)
}

// FindPath plans the exact destination walk through the mesh corridor
// search.
func (n navmeshNavigator) FindPath(
    start, end pathfind.Vec3,
) (*pathfind.Result, error) {
    return n.meshQuery(start, end, 0,
        n.clearedFilter(navmesh.DefaultFilter()))
}

// FindWaterEscape plans the way out of the water through the mesh
// escape search (the 8x priced flood to the first dry polygon).
func (n navmeshNavigator) FindWaterEscape(
    start pathfind.Vec3,
) (*pathfind.Result, error) {
    began := time.Now()
    route, err := n.mesh.WaterEscape(meshPos(start))
    if err != nil {
        return nil, err
    }
    if result := n.meshResult(route, began); result != nil {
        return result, nil
    }

    return &pathfind.Result{Found: false,
        Duration: time.Since(began)}, nil
}

// ClosestHeight resolves the destination deck height with the grid
// engine: the layer resolution is the engine's own semantics (the
// deck the server itself picks), the mesh has no equivalent contract.
func (n navmeshNavigator) ClosestHeight(
    x, y float64, refZ int16,
) (int16, error) {
    return n.engine.ClosestHeight(x, y, refZ)
}

// LineOfSight answers the geodata sight line with the engine.
func (n navmeshNavigator) LineOfSight(
    start, end pathfind.Vec3,
) (bool, error) {
    return n.engine.LineOfSight(start, end, n.engine.MaxPassableHeight())
}

// OverWater answers the geodata water surface check with the engine.
func (n navmeshNavigator) OverWater(x, y float64, refZ int16) bool {
    return n.engine.OverWater(x, y, refZ)
}

// WaterCrossed answers the geodata water raster with the engine.
func (n navmeshNavigator) WaterCrossed(
    start, end pathfind.Vec3,
) (bool, error) {
    return n.engine.WaterCrossed(start, end)
}

// ValidateClick mirrors the server move validation with the engine:
// the click guard of the town walk stays on the raster the server
// itself walks, never on the mesh surface.
func (n navmeshNavigator) ValidateClick(
    from, to pathfind.Vec3,
) (pathfind.Vec3, bool) {
    return n.engine.ValidateClick(from, to)
}

// meshQuery runs one mesh RouteApproach under the filter and maps
// every answer onto the Navigator contract: the full corridor found,
// the closest-reachable partial (Found=false, Partial=true, the
// waypoints end where the filter lets the corridor continue), the
// bare not found, the honest error (no tile under an endpoint). No
// answer ever consults the grid engine.
func (n navmeshNavigator) meshQuery(
    start, end pathfind.Vec3, approachRadius float64,
    filter navmesh.Filter,
) (*pathfind.Result, error) {
    began := time.Now()
    route, err := n.mesh.RouteApproach(meshPos(start), meshPos(end),
        approachRadius, filter)
    if err != nil {
        return nil, err
    }
    if result := n.meshResult(route, began); result != nil {
        return result, nil
    }
    if route != nil && route.Partial && len(route.Waypoints) > 0 {
        waypoints, length := n.meshWaypoints(route)

        return &pathfind.Result{
            Found:     false,
            Partial:   true,
            Waypoints: waypoints,
            Duration:  time.Since(began),
            Explored:  route.Explored,
            Length:    length,
        }, nil
    }

    return &pathfind.Result{Found: false,
        Duration: time.Since(began)}, nil
}

// meshResult maps one full mesh route answer onto the pathfind result
// of the Navigator contract, nil when the answer is not a full found
// route.
func (n navmeshNavigator) meshResult(
    route *navmesh.Route, began time.Time,
) *pathfind.Result {
    if route == nil || !route.Found || len(route.Waypoints) == 0 {
        return nil
    }
    waypoints, length := n.meshWaypoints(route)

    return &pathfind.Result{
        Found:     true,
        Partial:   false,
        Aborted:   false,
        Waypoints: waypoints,
        RawPath:   nil,
        Duration:  time.Since(began),
        Explored:  route.Explored,
        OpenLeft:  0,
        Length:    length,
    }
}

// meshWaypoints converts the funnel waypoints of a mesh route into
// the pathfind vectors together with the walked length. The funnel
// answer serves as the mesh search produced it: the pivot clearance
// and the shortcut pass inside the search already own the wall
// avoidance, no post pass folds the route across the corridor.
func (n navmeshNavigator) meshWaypoints(
    route *navmesh.Route,
) ([]pathfind.Vec3, float64) {
    waypoints := make([]pathfind.Vec3, len(route.Waypoints))
    length := 0.0
    for i, wp := range route.Waypoints {
        waypoints[i] = pathfind.Vec3{X: wp.X, Y: wp.Y, Z: wp.Z}
        if i > 0 {
            dx := wp.X - route.Waypoints[i-1].X
            dy := wp.Y - route.Waypoints[i-1].Y
            dz := wp.Z - route.Waypoints[i-1].Z
            length += math.Sqrt(dx*dx + dy*dy + dz*dz)
        }
    }

    return waypoints, length
}

// meshPos converts a pathfind vector into the mesh position.
func meshPos(v pathfind.Vec3) navmesh.Pos {
    return navmesh.Pos{X: v.X, Y: v.Y, Z: v.Z}
}

// avoidCircles converts the avoid areas of the Navigator contract
// into the mesh ban disks.
func avoidCircles(avoid []pathfind.AvoidArea) []navmesh.AvoidCircle {
    if len(avoid) == 0 {
        return nil
    }
    circles := make([]navmesh.AvoidCircle, len(avoid))
    for i, area := range avoid {
        circles[i] = navmesh.AvoidCircle{
            CenterX: area.Center.X,
            CenterY: area.Center.Y,
            Radius:  area.Radius,
        }
    }

    return circles
}
