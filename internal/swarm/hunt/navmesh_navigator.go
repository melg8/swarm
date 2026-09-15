// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The live integration of the navmesh runtime (docs/navmesh.md): the
// long route queries of the Navigator seam serve from the prebuilt
// tile mesh through the Detour-style corridor search - the stacked
// deck disambiguation, the water pricing, the funnel string pulling
// and the recovery ban walls - while the grid engine stays the click
// validation and local walk layer it already is (ValidateClick, the
// sight lines, the water rasters, the deck heights). The fallback
// rule keeps the grid engine the authority on every answer the mesh
// cannot serve: a missing tile, a destination on ground the sheet
// decomposition dropped, a sealed goal - the hybrid can only ADD
// routes (the milliseconds of the mesh corridor instead of the
// seconds of the grid flood), never lose them.

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// navmeshNavigator is the hybrid navigator of the live integration
// round: the route planning on the mesh, the validation on the grid
// engine.
type navmeshNavigator struct {
    engine *pathfind.Engine
    mesh   *navmesh.Mesh
}

// NewNavmeshNavigator wraps a geodata engine and a navigation mesh
// into the hunt navigator. Returning the Navigator interface is the
// deliberate seam of the hunt package (see NewNavigator).
func NewNavmeshNavigator( //nolint:ireturn
    engine *pathfind.Engine, mesh *navmesh.Mesh,
) Navigator {
    return navmeshNavigator{engine: engine, mesh: mesh}
}

// FindPathApproach plans the walk through the mesh corridor search
// (the approach radius goal, the swim pricing), falling back to the
// grid engine when the mesh holds no full corridor for the query.
func (n navmeshNavigator) FindPathApproach(
    start, end pathfind.Vec3, approachRadius float64,
) (*pathfind.Result, error) {
    if result := n.meshRoute(start, end, approachRadius,
        navmesh.DefaultFilter()); result != nil {
        return result, nil
    }

    return n.engine.FindPathApproach(
        start, end, approachRadius, n.engine.MaxPassableHeight())
}

// FindPathApproachAvoiding plans the water permitting walk around the
// avoid areas through the mesh corridor search (the ban walls with
// the escape ring of the own ban), falling back to the grid engine.
func (n navmeshNavigator) FindPathApproachAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    filter := navmesh.DefaultFilter()
    filter.Avoid = avoidCircles(avoid)
    if result := n.meshRoute(start, end, approachRadius,
        filter); result != nil {
        return result, nil
    }

    return n.engine.FindPathApproachAvoiding(
        start, end, approachRadius, n.engine.MaxPassableHeight(), avoid)
}

// FindPathApproachDryAvoiding plans the water walled walk around the
// avoid areas through the mesh corridor search, falling back to the
// grid engine: a dry target the mesh cannot reach answers the engine
// verdict (the mesh partial corridors surface as Found=false today -
// surfacing the closest-reachable waypoints is the follow-up round).
func (n navmeshNavigator) FindPathApproachDryAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    filter := navmesh.DryFilter()
    filter.Avoid = avoidCircles(avoid)
    if result := n.meshRoute(start, end, approachRadius,
        filter); result != nil {
        return result, nil
    }

    return n.engine.FindPathApproachDryAvoiding(
        start, end, approachRadius, n.engine.MaxPassableHeight(), avoid)
}

// FindPath plans the exact destination walk through the mesh corridor
// search, falling back to the grid engine.
func (n navmeshNavigator) FindPath(
    start, end pathfind.Vec3,
) (*pathfind.Result, error) {
    if result := n.meshRoute(start, end, 0,
        navmesh.DefaultFilter()); result != nil {
        return result, nil
    }

    return n.engine.FindPath(start, end, n.engine.MaxPassableHeight())
}

// FindWaterEscape plans the way out of the water through the mesh
// escape search (the 8x priced flood to the first dry polygon),
// falling back to the grid engine.
func (n navmeshNavigator) FindWaterEscape(
    start pathfind.Vec3,
) (*pathfind.Result, error) {
    began := time.Now()
    route, err := n.mesh.WaterEscape(meshPos(start))
    if result := meshResult(route, err, began); result != nil {
        return result, nil
    }

    return n.engine.FindWaterEscape(start)
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

// meshRoute answers the route query through the mesh when it holds a
// FULL corridor, nil otherwise: no mesh under an endpoint, no
// corridor under the filter, or a partial (closest-reachable) answer
// - all of them hand the question back to the grid engine, which
// stays the authority on reachability for the round one contract.
func (n navmeshNavigator) meshRoute(
    start, end pathfind.Vec3, approachRadius float64,
    filter navmesh.Filter,
) *pathfind.Result {
    began := time.Now()
    route, err := n.mesh.RouteApproach(meshPos(start), meshPos(end),
        approachRadius, filter)

    return meshResult(route, err, began)
}

// meshResult maps one mesh route answer onto the pathfind result of
// the Navigator contract, nil when the answer is not a full found
// route (the fallback marker).
func meshResult(
    route *navmesh.Route, err error, began time.Time,
) *pathfind.Result {
    if err != nil || route == nil || !route.Found ||
        len(route.Waypoints) == 0 {
        return nil
    }
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

    return &pathfind.Result{
        Found:     true,
        Aborted:   false,
        Waypoints: waypoints,
        RawPath:   nil,
        Duration:  time.Since(began),
        Explored:  route.Explored,
        OpenLeft:  0,
        Length:    length,
    }
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
