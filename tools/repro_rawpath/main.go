// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// repro_rawpath runs one mesh route query with the viewer filter and
// prints the raw funnel answer with the per segment length and turn
// angle stats, so the zigzag pivots of the raw answer count against
// the straight line distance (the owner zigzag report of the
// path=raw viewer toggle).
//
// Usage:
//
//    go run ./tools/repro_rawpath -mesh data/navmesh-world \
//        -from 45257,49353,-3059 -to 25500,51095,-3408
package main

import (
    "flag"
    "fmt"
    "math"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// vec parses one "x,y,z" triple.
func vec(name, value string) navmesh.Pos {
    parts := strings.Split(value, ",")
    if len(parts) != 3 {
        fmt.Printf("%s must be x,y,z: %q\n", name, value)
        os.Exit(1)
    }
    x, _ := strconv.ParseFloat(parts[0], 64)
    y, _ := strconv.ParseFloat(parts[1], 64)
    z, _ := strconv.ParseFloat(parts[2], 64)

    return navmesh.Pos{X: x, Y: y, Z: z}
}

func main() {
    meshDir := flag.String("mesh", "data/navmesh-world", "tile pack")
    from := flag.String("from", "45257,49353,-3059", "start x,y,z")
    to := flag.String("to", "25500,51095,-3408", "end x,y,z")
    clearance := flag.Float64("clearance", 7.5, "waypoint clearance")
    limit := flag.Int("limit", 20, "waypoints printed from each end")
    dump := flag.String("dump", "", "write all raw waypoints to the file")
    flag.Parse()

    mesh := navmesh.NewMesh(*meshDir)
    start, end := vec("-from", *from), vec("-to", *to)
    filter := navmesh.DefaultFilter()
    filter.WaterZones = navmesh.C1WaterZones()
    filter.WaypointClearance = *clearance
    filter.Smooth = *clearance > 0

    began := time.Now()
    route, err := mesh.Route(start, end, filter)
    if err != nil {
        fmt.Printf("route failed: %v\n", err)
        os.Exit(1)
    }
    ms := float64(time.Since(began).Nanoseconds()) / 1e6
    if route == nil {
        fmt.Printf("no route in %.1fms\n", ms)
        os.Exit(1)
    }
    status := "no path"
    switch {
    case route.Found:
        status = "found"
    case route.Partial:
        status = "partial"
    }
    raw := route.RawWaypoints
    if raw == nil {
        raw = route.Waypoints
    }
    fmt.Printf("route %s in %.1fms: corridor=%d raw=%d smooth=%d\n",
        status, ms, len(route.Corridor), len(raw),
        len(route.Waypoints))

    straight := math.Hypot(end.X-start.X, end.Y-start.Y)
    fmt.Printf("straight line %.0f, raw path %.0f (%.2fx)\n",
        straight, pathLength(raw), pathLength(raw)/straight)
    fmt.Printf("raw turns: %d over %.0f deg (sum |turn|)\n",
        countTurns(raw), totalTurnDeg(raw))

    printHeadTail("raw", raw, *limit)
    printHeadTail("smooth", route.Waypoints, *limit)

    if *dump != "" {
        f, err := os.Create(*dump)
        if err != nil {
            fmt.Printf("dump failed: %v\n", err)
            os.Exit(1)
        }
        defer f.Close()
        for i, wp := range raw {
            fmt.Fprintf(f, "wp %d: %d %d %d\n", i, int32(wp.X),
                int32(wp.Y), int32(wp.Z))
        }
        fmt.Printf("dumped %d raw waypoints to %s\n", len(raw), *dump)
    }
}

// pathLength sums the 2D leg lengths.
func pathLength(wps []navmesh.Pos) float64 {
    total := 0.0
    for i := 1; i < len(wps); i++ {
        total += math.Hypot(wps[i].X-wps[i-1].X, wps[i].Y-wps[i-1].Y)
    }

    return total
}

// countTurns counts the vertices that change the heading by more
// than 15 degrees either way.
func countTurns(wps []navmesh.Pos) int {
    turns := 0
    for i := 2; i < len(wps); i++ {
        if math.Abs(turnDeg(wps[i-2], wps[i-1], wps[i])) > 15 {
            turns++
        }
    }

    return turns
}

// totalTurnDeg sums the absolute heading change over the path.
func totalTurnDeg(wps []navmesh.Pos) float64 {
    total := 0.0
    for i := 2; i < len(wps); i++ {
        total += math.Abs(turnDeg(wps[i-2], wps[i-1], wps[i]))
    }

    return total
}

// turnDeg is the signed heading change at b along a-b-c.
func turnDeg(a, b, c navmesh.Pos) float64 {
    a1 := math.Atan2(b.Y-a.Y, b.X-a.X)
    a2 := math.Atan2(c.Y-b.Y, c.X-b.X)
    deg := (a2 - a1) * 180 / math.Pi
    if deg > 180 {
        deg -= 360
    }
    if deg < -180 {
        deg += 360
    }

    return deg
}

// printHeadTail prints the first and last limit waypoints with the
// leg length and the turn at the waypoint.
func printHeadTail(name string, wps []navmesh.Pos, limit int) {
    if len(wps) == 0 {
        fmt.Printf("%s: none\n", name)

        return
    }
    show := func(i int) {
        leg := 0.0
        turn := ""
        if i > 0 {
            leg = math.Hypot(wps[i].X-wps[i-1].X, wps[i].Y-wps[i-1].Y)
        }
        if i > 1 {
            turn = fmt.Sprintf(" turn=%+.0f", turnDeg(wps[i-2],
                wps[i-1], wps[i]))
        }
        fmt.Printf("  %s wp %d: %d %d %d leg=%.0f%s\n", name, i,
            int32(wps[i].X), int32(wps[i].Y), int32(wps[i].Z), leg,
            turn)
    }
    head := limit
    if head > len(wps) {
        head = len(wps)
    }
    for i := 0; i < head; i++ {
        show(i)
    }
    if len(wps) > 2*head {
        fmt.Printf("  ... %d skipped ...\n", len(wps)-2*head)
        for i := len(wps) - head; i < len(wps); i++ {
            show(i)
        }
    } else {
        for i := head; i < len(wps); i++ {
            show(i)
        }
    }
}
