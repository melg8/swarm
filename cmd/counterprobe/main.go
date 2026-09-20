// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Scratch probe of the merchant counter geometry. The map mode dumps
// the raw geodata of a square around every known shop merchant spawn
// (each cell classified from its layer stack: floor, raised deck,
// counter row, wall). The route mode prints the mesh route answer a
// trip segment would serve today. The render mode paints the same
// classification as a zoomed PNG per merchant. The custom mode renders
// a custom square; the text mode prints the precise marker grid of it.
package main

import (
    "flag"
    "fmt"
    "math"
    "os"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// merchant is one shop npc of the probe (the townMerchants rows plus
// the spawn heading of the Mobius data).
type merchant struct {
    name    string
    x, y, z float64
    heading float64 // the L2 heading (65536 = 360 deg, 0 = east)
}

// The names the modes and the text markers share (the goconst
// occurrences of the literal).
const (
    customName   = "custom"
    unorenName   = "Unoren"
    arielName    = "Ariel"
    creameesName = "Creamees"
    herbielName  = "Herbiel"
    sabrinName   = "Sabrin"
    caseyName    = "Casey"
    soniaName    = "Sonia"
    laraName     = "Lara"
)

// merchants are the elven village and Dion shop merchants.
var merchants = []merchant{
    {unorenName, 44667, 46896, -2982, 31000},
    {arielName, 44683, 46952, -2981, 28672},
    {creameesName, 42700, 50057, -2984, 14500},
    {herbielName, 42766, 50037, -2984, 11500},
    {sabrinName, 17999, 144484, -3046, 6000},
    {caseyName, 17948, 144560, -3046, 6000},
    {soniaName, 19313, 146229, -3069, 49152},
    {laraName, 19223, 146228, -3069, 49152},
}

// dirs are the eight walk directions of the profiles; north is -y in
// the L2 world.
var dirs = []struct {
    name   string
    dx, dy float64
}{
    {"E", 1, 0}, {"NE", 0.7071, -0.7071}, {"N", 0, -1},
    {"NW", -0.7071, -0.7071}, {"W", -1, 0}, {"SW", -0.7071, 0.7071},
    {"S", 0, 1}, {"SE", 0.7071, 0.7071},
}

// approachOf returns the point the verification routes walk from: a
// plausible trip start side of each shop (the farm side north of the
// elven shop quarter, the town side of the Dion shops).
func approachOf(m merchant) pathfind.Vec3 {
    approaches := map[string]pathfind.Vec3{
        unorenName:   {X: 44683, Y: 47300, Z: -2984},
        arielName:    {X: 44683, Y: 47300, Z: -2984},
        creameesName: {X: 42700, Y: 50400, Z: -2984},
        herbielName:  {X: 42766, Y: 50400, Z: -2984},
        sabrinName:   {X: 18200, Y: 144500, Z: -3040},
        caseyName:    {X: 18200, Y: 144500, Z: -3040},
        soniaName:    {X: 19313, Y: 145900, Z: -3064},
        laraName:     {X: 19313, Y: 145900, Z: -3064},
    }

    return approaches[m.name]
}

func main() {
    geodata := flag.String("geodata", "data/geodata", "geodata dir")
    meshDir := flag.String("mesh", "data/navmesh", "navmesh tile dir")
    mode := flag.String("mode", "map",
        "map, route, render, custom, text, stands, scan, detect")
    out := flag.String("out", ".", "render output dir")
    scale := flag.Int("scale", 8, "render pixel size per cell")
    cx := flag.Float64("cx", 44660, "custom center x")
    cy := flag.Float64("cy", 47000, "custom center y")
    cz := flag.Float64("cz", -2984, "custom center z")
    radius := flag.Int("radius", 24, "custom render radius in cells")
    scanName := flag.String("scan", "", "scan one merchant stand area by name")
    flag.Parse()

    regions := map[string]*pathfind.Region{}
    for _, m := range merchants {
        path := regionPath(*geodata, m.x, m.y)
        if regions[path] != nil {
            continue
        }
        data, err := os.ReadFile(path)
        if err != nil {
            fmt.Printf("read %s: %v\n", path, err)
            os.Exit(1)
        }
        region, err := pathfind.ParseRegionData(data, regionKeyOf(path))
        if err != nil {
            fmt.Printf("parse %s: %v\n", path, err)
            os.Exit(1)
        }
        regions[path] = region
    }

    switch *mode {
    case "map":
        for _, m := range merchants {
            dumpMap(regions[regionPath(*geodata, m.x, m.y)], m)
        }
    case "route":
        mesh := navmesh.NewMesh(*meshDir)
        for _, m := range merchants {
            dumpRoute(mesh, m)
        }
    case "render":
        mesh := navmesh.NewMesh(*meshDir)
        planEnds := collectPlanEnds(mesh)
        for _, m := range merchants {
            dumpRoute(mesh, m)
        }
        outDir = *out
        renderZoom(regions, *geodata, *scale, planEnds)
    case customName:
        mesh := navmesh.NewMesh(*meshDir)
        planEnds := collectPlanEnds(mesh)
        probe := merchant{
            name: customName, x: *cx, y: *cy, z: *cz, heading: 0,
        }
        outDir = *out
        renderCustom(regions, *geodata, *scale, *radius, probe,
            merchants, planEnds)
    case "text":
        mesh := navmesh.NewMesh(*meshDir)
        planEnds := collectPlanEnds(mesh)
        probe := merchant{
            name: customName, x: *cx, y: *cy, z: *cz, heading: 0,
        }
        dumpText(regions[regionPath(*geodata, *cx, *cy)], probe,
            merchants, planEnds)
    case "stands":
        engine := pathfind.NewEngine(*geodata)
        mesh := navmesh.NewMesh(*meshDir)
        verifyStands(engine, mesh)
    case "verify":
        engine := pathfind.NewEngine(*geodata)
        mesh := navmesh.NewMesh(*meshDir)
        verifyStandsGrid(engine, mesh, standVerifyList)
    case "scan":
        engine := pathfind.NewEngine(*geodata)
        mesh := navmesh.NewMesh(*meshDir)
        scanStandArea(engine, mesh, *scanName)
    case "detect":
        detectCounter(func(x, y float64) *pathfind.Region {
            return regions[regionPath(*geodata, x, y)]
        })
    }
}

// collectPlanEnds returns the mesh route end per merchant name.
func collectPlanEnds(mesh *navmesh.Mesh) map[string]pathfind.Vec3 {
    planEnds := map[string]pathfind.Vec3{}
    for _, m := range merchants {
        if end, ok := routeEnd(mesh, m); ok {
            planEnds[m.name] = end
        }
    }

    return planEnds
}

// dumpRoute prints the mesh route answer toward the merchant spawn.
func dumpRoute(mesh *navmesh.Mesh, m merchant) {
    end, ok := routeEnd(mesh, m)
    fmt.Printf("%-9s:", m.name)
    if !ok {
        fmt.Println(" no route")

        return
    }
    d2 := math.Hypot(end.X-m.x, end.Y-m.y)
    fmt.Printf(" end %.0f %.0f %.0f (d2d %.0f dz %.0f) bearing %s\n",
        end.X, end.Y, end.Z, d2, end.Z-m.z,
        bearingOf(m, end.X, end.Y))
}

// routeEnd returns the last waypoint of the mesh route toward the
// merchant spawn (false when the route answers nothing).
func routeEnd(mesh *navmesh.Mesh, m merchant) (pathfind.Vec3, bool) {
    approach := approachOf(m)
    spawn := pathfind.Vec3{X: m.x, Y: m.y, Z: m.z}
    result, err := mesh.Route(
        navmesh.Pos{X: approach.X, Y: approach.Y, Z: approach.Z},
        navmesh.Pos{X: spawn.X, Y: spawn.Y, Z: spawn.Z},
        navmesh.DefaultFilter())
    if err != nil || result == nil || len(result.Waypoints) == 0 {
        return pathfind.Vec3{X: 0, Y: 0, Z: 0}, false
    }
    last := result.Waypoints[len(result.Waypoints)-1]

    return pathfind.Vec3{X: last.X, Y: last.Y, Z: last.Z}, true
}

// bearingOf names the compass direction of the plan end relative to
// the merchant spawn (north is -y).
func bearingOf(m merchant, x, y float64) string {
    dx, dy := x-m.x, y-m.y
    if math.Hypot(dx, dy) < 8 {
        return "at"
    }
    switch {
    case dx >= 0 && dy < 0:
        return "NE"
    case dx > 0 && dy <= 0:
        return "E"
    case dx >= 0 && dy > 0:
        return "SE"
    case dx > 0:
        return "S"
    case dx <= 0 && dy > 0:
        return "SW"
    case dx < 0 && dy >= 0:
        return "W"
    default:
        return "NW"
    }
}

// dumpMap prints the cell classification square around the spawn.
// North is up (-y), east is right (+x).
func dumpMap(region *pathfind.Region, m merchant) {
    fmt.Printf("\n=== %s at %.0f %.0f %.0f (heading %.1f deg)\n",
        m.name, m.x, m.y, m.z, m.heading/65536*360)
    buf := make([]pathfind.Layer, 0, 8)
    center := pathfind.WorldToCell(m.x, m.y)
    const radius = 8
    for row := -radius; row <= radius; row++ {
        fmt.Print("  ")
        for col := -radius; col <= radius; col++ {
            cell := pathfind.Point{
                X: center.X + int32(col),
                Y: center.Y + int32(row),
            }
            stack := region.LayerStack(pathfind.LocalCell(cell), buf)
            fmt.Print(classify(stack, int(m.z)))
        }
        if row == -radius {
            fmt.Print("  north (-y) up")
        }
        if row == radius {
            fmt.Print("  south (+y) down")
        }
        fmt.Println()
    }
    fmt.Println("  legend: . floor  , floor below  o low step  " +
        "O high step  T deck above  # no floor  @ spawn")
}

// classify reduces one layer stack to the map symbol (the reference
// is the merchant spawn z: floor cells answer ., raised decks answer
// o/O/T and cells without a usable layer answer #).
func classify(stack []pathfind.Layer, refZ int) string {
    best := "#"
    bestScore := math.MaxInt
    for _, l := range stack {
        d := int(l.Height) - refZ
        score := abs(d)
        var symbol string
        switch {
        case d >= -96 && d <= 24:
            symbol = "."
        case d < -96 && d >= -200:
            symbol = ","
        case d > 24 && d <= 96:
            symbol = "o"
        case d > 96 && d <= 200:
            symbol = "O"
        case d > 200 && d <= 400:
            symbol = "T"
        default:
            symbol = "#"
        }
        if symbol == "." {
            return "."
        }
        if score < bestScore {
            best, bestScore = symbol, score
        }
    }

    return best
}

// abs is the integer absolute value.
func abs(v int) int {
    if v < 0 {
        return -v
    }

    return v
}

// regionPath builds the region file path of the world position.
func regionPath(geodata string, x, y float64) string {
    key := pathfind.CellToRegion(pathfind.WorldToCell(x, y))

    return fmt.Sprintf("%s/%d_%d.l2j", geodata, key.Col, key.Row)
}

// regionKeyOf parses the region key back out of the file path.
func regionKeyOf(path string) pathfind.RegionKey {
    var col, row int16
    tail := path[len(path)-len("00_00.l2j"):]
    if _, err := fmt.Sscanf(tail, "%d_%d.l2j", &col, &row); err != nil {
        return pathfind.RegionKey{Col: 0, Row: 0}
    }

    return pathfind.RegionKey{Col: col, Row: row}
}
