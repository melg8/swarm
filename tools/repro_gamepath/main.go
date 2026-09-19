// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// repro_gamepath runs one walk query through every planner
// configuration the live bot and the viewer use, so the plan a bot
// served (the state dump walk plan) compares against the mesh answer
// and the grid engine answer side by side.
//
// Usage:
//
//    go run ./tools/repro_gamepath -mesh data/navmesh-world \
//        -geodata data/geodata -from 45956,49341,-3051 -to 47595,51569,-2992
package main

import (
    "flag"
    "fmt"
    "math"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// vec parses one "x,y,z" triple.
func vec(name, value string) pathfind.Vec3 {
    parts := strings.Split(value, ",")
    if len(parts) != 3 {
        fmt.Printf("%s must be x,y,z: %q\n", name, value)
        os.Exit(1)
    }
    x, _ := strconv.ParseFloat(parts[0], 64)
    y, _ := strconv.ParseFloat(parts[1], 64)
    z, _ := strconv.ParseFloat(parts[2], 64)

    return pathfind.Vec3{X: x, Y: y, Z: z}
}

// printResult prints the waypoints in the dump walk plan format (the
// int32 truncation the tracker publishes).
func printResult(name string, result *pathfind.Result) {
    if result == nil {
        fmt.Printf("%s: nil\n", name)

        return
    }
    fmt.Printf("%s: found=%v partial=%v waypoints=%d length=%.0f "+
        "explored=%d %.0fms\n", name, result.Found, result.Partial,
        len(result.Waypoints), result.Length, result.Explored,
        float64(result.Duration.Nanoseconds())/1e6)
    for i, wp := range result.Waypoints {
        marker := ""
        if i == 1 {
            marker = "  <-- TARGET"
        }
        fmt.Printf("  wp %d: %d %d %d%s\n", i, int32(wp.X),
            int32(wp.Y), int32(wp.Z), marker)
    }
}

// botFilter arms the live bot configuration: the default filter with
// the capsule clearance, the smooth pass and the grid capsule guard,
// NO water zones (the hunt navigator never arms them).
func botFilter(capsule *pathfind.Capsule,
    radius float64) navmesh.Filter {
    filter := navmesh.DefaultFilter()
    filter.WaypointClearance = radius
    filter.Smooth = radius > 0
    if capsule != nil {
        filter.Guard = capsule
    }

    return filter
}

// viewerFilter arms the pure viewer configuration: the default filter
// with the C1 water zone pricing, no clearance, no smooth, no guard
// (the -show-navmesh mode runs without the geodata engine).
func viewerFilter() navmesh.Filter {
    filter := navmesh.DefaultFilter()
    filter.WaterZones = navmesh.C1WaterZones()

    return filter
}

func main() {
    meshDir := flag.String("mesh", "data/navmesh-world",
        "navmesh tile directory")
    geodataDir := flag.String("geodata", "data/geodata",
        "geodata directory (the grid engine and the capsule)")
    fromFlag := flag.String("from", "45956,49341,-3051", "start x,y,z")
    toFlag := flag.String("to", "47595,51569,-2992", "end x,y,z")
    radiusFlag := flag.Float64("radius", 150, "approach radius")
    flag.Parse()

    from := vec("from", *fromFlag)
    to := vec("to", *toFlag)

    engine := pathfind.NewEngine(*geodataDir)
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)
    radius := engine.CapsuleRadius()
    var capsule *pathfind.Capsule
    if radius > 0 {
        capsule = pathfind.NewCapsule(engine)
    }
    fmt.Printf("engine capsule radius: %.1f\n", radius)

    mesh := navmesh.NewMesh(*meshDir)
    stats := mesh.Stats()
    fmt.Printf("mesh: %d tiles in %s\n\n", stats.TileFiles, stats.Dir)

    // 1. The live bot configuration: RouteApproach, no water zones,
    // the grid capsule guard.
    began := time.Now()
    route, err := mesh.RouteApproach(navmesh.Pos{X: from.X, Y: from.Y,
        Z: from.Z}, navmesh.Pos{X: to.X, Y: to.Y, Z: to.Z}, *radiusFlag,
        botFilter(capsule, radius))
    botResult := mapResult(route, err, began)
    printResult("BOT (approach, no zones, guard)", botResult)

    // 2. The pure viewer configuration: exact Route, water zones,
    // no guard.
    began = time.Now()
    route, err = mesh.Route(navmesh.Pos{X: from.X, Y: from.Y, Z: from.Z},
        navmesh.Pos{X: to.X, Y: to.Y, Z: to.Z}, viewerFilter())
    viewerResult := mapResult(route, err, began)
    printResult("VIEWER (exact, zones, no guard)", viewerResult)

    // 3. The candidate fix: the bot configuration plus the zones.
    began = time.Now()
    zones := botFilter(capsule, radius)
    zones.WaterZones = navmesh.C1WaterZones()
    route, err = mesh.RouteApproach(navmesh.Pos{X: from.X, Y: from.Y,
        Z: from.Z}, navmesh.Pos{X: to.X, Y: to.Y, Z: to.Z}, *radiusFlag,
        zones)
    fixedResult := mapResult(route, err, began)
    printResult("BOT+ZONES (approach, zones, guard)", fixedResult)

    // 4. The grid engine fallback (the plan the bot served when the
    // mesh failed).
    began = time.Now()
    engineResult, engineErr := engine.FindPathApproach(from, to,
        *radiusFlag, engine.MaxPassableHeight())
    if engineErr != nil {
        fmt.Printf("GRID: error: %v\n", engineErr)
    } else {
        if engineResult.Duration == 0 {
            engineResult.Duration = time.Since(began)
        }
        printResult("GRID (FindPathApproach)", engineResult)
    }
}

// mapResult converts the mesh route answer into the pathfind result
// (the same mapping the hunt navigator uses, the post pass included
// when the capsule exists).
func mapResult(route *navmesh.Route, err error,
    began time.Time) *pathfind.Result {
    if err != nil || route == nil || !route.Found ||
        len(route.Waypoints) == 0 {
        if err != nil {
            fmt.Printf("  (mesh error: %v)\n", err)
        }

        return &pathfind.Result{Found: false,
            Duration: time.Since(began)}
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
        Waypoints: waypoints,
        Duration:  time.Since(began),
        Explored:  route.Explored,
        Length:    length,
    }
}
