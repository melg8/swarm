package navmesh

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/stretchr/testify/require"
)

// TestGuardStairsApproachFromStairs reproduces the owner route of the
// 2026-09-20 guard stairs report: the walk plan from the stairs point
// (46880, 50752, -2889) to the guard (47595, 51569, -2992) with the
// trip approach radius 200. The reported from z sits 103 units above
// the quantized geodata surface (-2992) - the server z drift of a
// character walking the visual staircase. The pure 3D nearest polygon
// of the old FindNearestPoly bound the sealed decorative platform two
// cells aside (its slanted distance beat the honest ground's vertical
// one), the corridor search stranded inside its link component and
// the answer was the one waypoint pocket partial where the grid
// engine walks the route. The column-first binding pins the honest
// ground under the clicked x/y and the route answers found.
func TestGuardStairsApproachFromStairs(t *testing.T) {
    dir := navmeshDataDir()
    if dir == "" {
        t.Skip("no local navmesh tiles, the dump reproduction needs them")
    }
    mesh := NewMesh(dir)
    start := Pos{X: 46880, Y: 50752, Z: -2889}
    end := Pos{X: 47595, Y: 51569, Z: -2992}

    // The binding: the polygon containing the query x/y wins - the
    // -2992 ground strip under the stairs point, not the sealed
    // -2960 platform two cells north.
    startRef, startPos, ok := mesh.FindNearestPoly(start)
    require.True(t, ok)
    require.InDelta(t, start.X, startPos.X, 0.0001,
        "the bound surface point keeps the query x/y")
    require.InDelta(t, start.Y, startPos.Y, 0.0001)
    require.InDelta(t, -2992.0, startPos.Z, 0.5,
        "the honest geodata surface under the stairs point")
    require.Nil(t, mesh.floodPocketComponent(startRef),
        "the bound poly is connected ground, not the sealed platform")

    route, err := mesh.RouteApproach(start, end, 200, DefaultFilter())
    require.NoError(t, err)
    require.NotNil(t, route)
    require.True(t, route.Found,
        "the guard is reachable from the stairs ground")
    require.False(t, route.Partial)
    require.False(t, route.PocketEscape)
    require.NotEmpty(t, route.Waypoints)
    last := route.Waypoints[len(route.Waypoints)-1]
    horizontal := (last.X-end.X)*(last.X-end.X) +
        (last.Y-end.Y)*(last.Y-end.Y)
    require.LessOrEqual(t, horizontal, 200.0*200.0,
        "the plan ends within the approach radius of the guard")

    // The grid engine parity: the mesh answer matches the movement
    // authority on the same pair.
    engine := pathfind.NewEngine("../../../../data/geodata")
    if !engine.Stats().HasData {
        t.Log("no geodata next to the tiles, the parity check skips")

        return
    }
    result, err := engine.FindPathApproach(
        pathfind.Vec3{X: start.X, Y: start.Y, Z: start.Z},
        pathfind.Vec3{X: end.X, Y: end.Y, Z: end.Z}, 200,
        engine.MaxPassableHeight())
    require.NoError(t, err)
    require.True(t, result.Found,
        "the grid engine walks the route (the parity the mesh must keep)")
}
