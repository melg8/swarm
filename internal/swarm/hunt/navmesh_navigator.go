// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The live integration of the navmesh runtime (docs/navmesh.md): the route
// planning serves from the prebuilt tile mesh through the Detour-style corridor
// search (the stacked deck disambiguation, the C1 water zone pricing, the
// funnel string pulling, the recovery ban walls), and the mesh is the sole
// route planner - no route query of this navigator ever asks the grid engine
// for a path. The grid engine stays the click validation layer (ValidateClick,
// the sight lines, the water rasters, the deck heights), its capsule radius
// arms the funnel clearance, and a partial corridor serves through Partial.

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// navmeshNavigator is the mesh navigator: the route planning on the mesh, the
// validation on the grid engine. The engine capsule clearance governs both
// surfaces - the funnel pivots pull inward from the portal span ends and the
// shortcut pass answers to the grid capsule - so the grid oracle can only keep
// more funnel waypoints, never fold the route across a wall the mesh detours.
type navmeshNavigator struct {
    engine    *pathfind.Engine
    mesh      *navmesh.Mesh
    capsule   *pathfind.Capsule
    clearance float64
}

// NewNavmeshNavigator wraps a geodata engine and a navigation mesh into the
// hunt navigator (the Navigator seam of the hunt package, see NewNavigator).
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

// clearedFilter arms the funnel pivot clearance from the engine capsule, the
// shortcut pass with the grid capsule as the extra wall oracle, and the C1
// water zone pricing (the river beds the zone data omits walk at land rate).
func (n navmeshNavigator) clearedFilter(filter navmesh.Filter) navmesh.Filter {
    filter.WaypointClearance = n.clearance
    filter.Smooth = n.clearance > 0
    filter.WaterZones = navmesh.C1WaterZones()
    if n.capsule != nil {
        filter.Guard = n.capsule
    }

    return filter
}

// FindPathApproach plans the walk through the mesh corridor search only (the
// approach radius goal, the swim pricing).
func (n navmeshNavigator) FindPathApproach(
    start, end pathfind.Vec3, approachRadius float64,
) (*pathfind.Result, error) {
    return n.meshQuery(start, end, approachRadius,
        n.clearedFilter(navmesh.DefaultFilter()))
}

// FindPathApproachAvoiding plans the walk around the avoid areas through the
// mesh corridor search (the ban walls with the escape ring of the own ban). The
// partial corridors surface through Result.Partial.
func (n navmeshNavigator) FindPathApproachAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    filter := n.clearedFilter(navmesh.DefaultFilter())
    filter.Avoid = avoidCircles(avoid)

    return n.meshQuery(start, end, approachRadius, filter)
}

// FindPath plans the exact destination walk through the mesh corridor search.
func (n navmeshNavigator) FindPath(
    start, end pathfind.Vec3,
) (*pathfind.Result, error) {
    return n.meshQuery(start, end, 0,
        n.clearedFilter(navmesh.DefaultFilter()))
}

// ClosestHeight resolves the deck height with the grid engine (the deck the
// server itself picks); the mesh has no equivalent contract.
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

// ValidateClick mirrors the server move validation with the engine: the click
// guard stays on the raster the server walks, never on the mesh surface.
func (n navmeshNavigator) ValidateClick(
    from, to pathfind.Vec3,
) (pathfind.Vec3, bool) {
    return n.engine.ValidateClick(from, to)
}

// meshQuery runs one mesh RouteApproach under the filter and maps the answer
// onto the Navigator contract: the full corridor, the closest-reachable
// partial, the bare not found (a dropped destination, so the caller walks the
// direct server route) and the honest error. No answer consults the grid
// engine.
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
        if route.PocketEscape {
            // The stranded start answer: the mesh named the exit ground,
            // the click validation port names the deliverable direction
            // (the mesh aim stands when nothing validates).
            waypoints[0] = n.refinePocketExit(start, waypoints[0])
        }

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

// meshResult maps a full mesh route answer onto the pathfind result, nil when
// the answer is not a full found route.
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

// meshWaypoints converts the funnel waypoints into the pathfind vectors and the
// walked length; the search already owns the wall avoidance, no post pass folds
// the route across the corridor.
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

// pocketExitSweepDeg is the direction sweep of the pocket exit refinement: the
// mesh aim first, then the circle in 30 degree steps ordered by angular
// distance from it, because the strict link graph cannot know which direction
// the permissive click transport walks out through.
var pocketExitSweepDeg = [12]float64{
    0, 30, -30, 60, -60, 90, -90, 120, -120, 150, -150, 180,
}

// pocketExitAimDist is the transport distance of the exit aim: past the wide
// arrival slack (waypointArriveDist) and the server rescue click floor.
const pocketExitAimDist = 224.0

// refinePocketExit turns the mesh exit aim into an aim the clicks can deliver:
// the mesh ground seeds the direction, the sweep rotates it around the standing
// point and the click validation port picks the first direction the server
// transport walks. The aim height rides ClosestHeight, so a hole, a drop or a
// wall refuses and the sweep moves on; nothing validating keeps the mesh aim.
func (n navmeshNavigator) refinePocketExit(
    start, aim pathfind.Vec3,
) pathfind.Vec3 {
    if n.engine == nil {
        return aim
    }
    dist := math.Hypot(aim.X-start.X, aim.Y-start.Y)
    if dist < pocketExitAimDist {
        dist = pocketExitAimDist
    }
    base := math.Atan2(aim.Y-start.Y, aim.X-start.X)
    for _, delta := range pocketExitSweepDeg {
        rad := base + delta*math.Pi/180
        cx := start.X + math.Cos(rad)*dist
        cy := start.Y + math.Sin(rad)*dist
        z, err := n.engine.ClosestHeight(cx, cy, int16(start.Z))
        if err != nil {
            continue
        }
        candidate := pathfind.Vec3{X: cx, Y: cy, Z: float64(z)}
        if _, ok := n.engine.ValidateClick(start, candidate); ok {
            return candidate
        }
    }

    return aim
}

// avoidCircles converts the avoid areas into the mesh ban disks.
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
