// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// corner probe: the delevel guard walk route (the dryad ground to the
// eastern guard Kendell) planned the way the hunt navigator plans it,
// then every intermediate turn validated the way the server movement
// validates the follower clicks (the grid ValidateClick port - the
// Bresenham raster with the anti corner cut). A turn whose click the
// server refuses from the arrival neighborhood of the previous
// waypoint is a corner stick the follower walks into.
package main

import (
    "fmt"
    "math"
    "os"
    "strconv"
    "strings"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navbuild"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

const (
    // dryadFocus is the Green Dryad S-16 focus cell of the delevel
    // acceptance scenario.
    dryadX, dryadY, dryadZ = 43500.0, 54560.0, -3664.0
    // kendell and starden are the archer guard posts.
    kendellX, kendellY, kendellZ = 47595.0, 51569.0, -2992.0
    stardenX, stardenY, stardenZ = 42971.0, 51372.0, -2992.0
    // villageRestart is the elven village respawn point of a death
    // (the second delevel walk starts here).
    villageX, villageY, villageZ = 44962.0, 51105.0, -3024.0
)

// main plans the guard walks and audits every turn of the plans.
func main() {
    geodata := "../../../data/geodata"
    if len(os.Args) > 1 {
        geodata = os.Args[1]
    }
    raw, err := os.ReadFile(geodata + "/21_19.l2j")
    if err != nil {
        fmt.Println("geodata pack not present:", err)

        return
    }
    build, err := navbuild.BuildRegion(raw, 21, 19,
        navbuild.DefaultOptions())
    if err != nil {
        fmt.Println("build region:", err)

        return
    }
    dir, err := os.MkdirTemp("", "cornerprobe")
    if err != nil {
        fmt.Println("tempdir:", err)

        return
    }
    data, err := navmesh.EncodeTile(build.Tile)
    if err != nil {
        fmt.Println("encode:", err)

        return
    }
    if err := os.WriteFile(dir+"/21_19.nm", data, 0o600); err != nil {
        fmt.Println("write:", err)

        return
    }
    mesh := navmesh.NewMesh(dir)
    engine := pathfind.NewEngine(geodata)
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)

    type walk struct {
        name string
        from pathfind.Vec3
        to   pathfind.Vec3
    }
    walks := []walk{
        {"dryad -> Starden", vec(dryadX, dryadY, dryadZ),
            vec(stardenX, stardenY, stardenZ)},
        {"dryad -> Kendell", vec(dryadX, dryadY, dryadZ),
            vec(kendellX, kendellY, kendellZ)},
        {"village -> Kendell", vec(villageX, villageY, villageZ),
            vec(kendellX, kendellY, kendellZ)},
        {"village -> Starden", vec(villageX, villageY, villageZ),
            vec(stardenX, stardenY, stardenZ)},
    }
    for _, w := range walks {
        route, err := mesh.RouteApproach(meshPos(w.from), meshPos(w.to),
            10, clearedFilter())
        if err != nil {
            fmt.Printf("\n=== %s: ERR %v\n", w.name, err)

            continue
        }
        fmt.Printf("\n=== %s: found=%v partial=%v wps=%d\n",
            w.name, route.Found, route.Partial, len(route.Waypoints))
        auditLegs(engine, route)
        auditTurns(engine, route)
    }
}

// auditLegs validates every planned leg of the route the way the
// server validates the follower clicks: a refused leg means the plan
// itself holds a chord the server movement never delivers - the
// follower sticks on it whatever the recovery does.
func auditLegs(engine *pathfind.Engine, route *navmesh.Route) {
    wps := route.Waypoints
    for i := 0; i+1 < len(wps); i++ {
        from := wps[i]
        to := wps[i+1]
        if _, ok := engine.ValidateClick(vec(from.X, from.Y, from.Z),
            vec(to.X, to.Y, to.Z)); !ok {
            fmt.Printf("  LEG wp[%d] (%.0f %.0f %.0f) -> wp[%d] "+
                "(%.0f %.0f %.0f): REFUSED\n", i, from.X, from.Y,
                from.Z, i+1, to.X, to.Y, to.Z)
        }
    }
}

// auditTurns validates every intermediate turn of the plan the way
// the follower's click guard does: from arrival samples around the
// turn waypoint (the pass radius ring, biased to the incoming leg)
// toward the next waypoint. The sample z rides the leg interpolation
// (the surface the character walks), not a layer resolution. Any
// refused sample is a stick the walker hits when the server stops it
// short of the turn.
func auditTurns(engine *pathfind.Engine, route *navmesh.Route) {
    wps := route.Waypoints
    for i := 1; i+1 < len(wps); i++ {
        wp, next := wps[i], wps[i+1]
        prev := wps[i-1]
        // The incoming direction: the arrival samples sit on the
        // incoming leg 0..50 units short of the turn, plus the
        // lateral offset the server stop distance leaves.
        dx, dy := wp.X-prev.X, wp.Y-prev.Y
        leg := math.Hypot(dx, dy)
        if leg < 1 {
            continue
        }
        ux, uy := dx/leg, dy/leg
        refused := 0
        total := 0
        firstBad := pathfind.Vec3{X: 0, Y: 0, Z: 0}
        badAt := 0.0
        for _, back := range []float64{0, 2, 4, 8, 12, 15, 16, 24, 32, 40, 48} {
            for _, side := range []float64{0, -8, 8} {
                // The lateral vector of the incoming leg.
                px := wp.X - ux*back - uy*side
                py := wp.Y - uy*back + ux*side
                pz := legZ(wp, prev, back)
                from := pathfind.Vec3{X: px, Y: py, Z: pz}
                to := pathfind.Vec3{X: next.X, Y: next.Y, Z: next.Z}
                total++
                if _, ok := engine.ValidateClick(from, to); !ok {
                    refused++
                    if refused == 1 {
                        firstBad = from
                        badAt = back
                    }
                }
            }
        }
        if refused > 0 {
            fmt.Printf("  TURN wp[%d] (%.0f %.0f) -> wp[%d] "+
                "(%.0f %.0f): %d/%d arrival clicks refused "+
                "(first at %.0f units back, from %.0f %.0f)\n",
                i, wp.X, wp.Y, i+1, next.X, next.Y, refused, total,
                badAt, firstBad.X, firstBad.Y)
        }
    }
    // The near pivot detail of every refused turn: which arrival
    // distances refuse and does the pivot itself click through; and
    // does the FORWARD overshoot click (past the turn along the
    // outgoing leg) validate from the refused positions - the corner
    // recovery candidate.
    for i := 1; i+1 < len(wps); i++ {
        wp, next := wps[i], wps[i+1]
        prev := wps[i-1]
        dx, dy := wp.X-prev.X, wp.Y-prev.Y
        leg := math.Hypot(dx, dy)
        if leg < 1 {
            continue
        }
        ux, uy := dx/leg, dy/leg
        line := ""
        outDx, outDy := next.X-wp.X, next.Y-wp.Y
        outLeg := math.Hypot(outDx, outDy)
        if outLeg < 1 {
            continue
        }
        oux, ouy := outDx/outLeg, outDy/outLeg
        bad := false
        for _, back := range []float64{0, 2, 4, 8, 12, 15, 16, 24, 32, 40, 48} {
            px := wp.X - ux*back
            py := wp.Y - uy*back
            from := pathfind.Vec3{
                X: px, Y: py, Z: legZ(wp, prev, back),
            }
            to := pathfind.Vec3{X: next.X, Y: next.Y, Z: next.Z}
            _, ok := engine.ValidateClick(from, to)
            mark := "."
            if !ok {
                mark = "X"
                bad = true
                // The overshoot probes from the refused position.
                var over strings.Builder
                over.WriteString(" over:")
                for _, s := range []float64{50, 100, 150, 200} {
                    ox := next.X + oux*s
                    oy := next.Y + ouy*s
                    oz, oerr := engine.ClosestHeight(ox, oy,
                        int16(next.Z))
                    if oerr != nil {
                        over.WriteString(" ?:")
                        over.WriteString(strconv.Itoa(int(s)))

                        continue
                    }
                    if _, ok := engine.ValidateClick(from,
                        pathfind.Vec3{X: ox, Y: oy, Z: float64(oz)}); ok {
                        over.WriteString(" Y:")
                    } else {
                        over.WriteString(" N:")
                    }
                    over.WriteString(strconv.Itoa(int(s)))
                }
                // The pivot hop candidate: the click to the turn
                // pivot itself from the refused position (the
                // incoming leg - the ground just walked).
                if _, ok := engine.ValidateClick(from, pathfind.Vec3{
                    X: wp.X, Y: wp.Y, Z: wp.Z,
                }); ok {
                    over.WriteString(" hop:Y")
                } else {
                    over.WriteString(" hop:N")
                }
                line += mark + over.String()
            }
            if !strings.HasSuffix(line, mark) || mark == "." {
                line += mark
            }
        }
        if bad {
            fmt.Printf("  DETAIL wp[%d]->wp[%d] (back:mark+overshoot)%s\n",
                i, i+1, line)
            // The neighborhood dump of the refused band: the plan
            // waypoints around the turn.
            fmt.Printf("    plan around: ")
            for j := i - 2; j <= i+2 && j >= 0 && j < len(wps); j++ {
                fmt.Printf("wp[%d](%.0f %.0f %.0f) ", j, wps[j].X,
                    wps[j].Y, wps[j].Z)
            }
            fmt.Println()
        }
    }
    // The recovery probe at the FIRST refused band position: which
    // farther waypoints, local ring samples and back bends validate
    // from the stuck spot - the candidate recovery clicks.
    for i := 1; i+1 < len(wps); i++ {
        wp, next := wps[i], wps[i+1]
        prev := wps[i-1]
        dx, dy := wp.X-prev.X, wp.Y-prev.Y
        leg := math.Hypot(dx, dy)
        if leg < 1 {
            continue
        }
        ux, uy := dx/leg, dy/leg
        stuck := pathfind.Vec3{X: 0, Y: 0, Z: 0}
        found := false
        for _, back := range []float64{8, 15, 16} {
            from := pathfind.Vec3{
                X: wp.X - ux*back, Y: wp.Y - uy*back,
                Z: legZ(wp, prev, back),
            }
            if _, ok := engine.ValidateClick(from, pathfind.Vec3{
                X: next.X, Y: next.Y, Z: next.Z,
            }); !ok {
                stuck = from
                found = true

                break
            }
        }
        if !found {
            continue
        }
        fmt.Printf("  RECOVER from (%.0f %.0f %.0f) at turn wp[%d]:\n",
            stuck.X, stuck.Y, stuck.Z, i)
        // The farther plan waypoints.
        for j := i + 1; j < len(wps) && j <= i+4; j++ {
            _, ok := engine.ValidateClick(stuck, pathfind.Vec3{
                X: wps[j].X, Y: wps[j].Y, Z: wps[j].Z,
            })
            fmt.Printf("    wp[%d] (%.0f %.0f): %v\n", j, wps[j].X,
                wps[j].Y, ok)
        }
        // The local ring around the turn pivot.
        hits := 0
        for _, ring := range []float64{16, 32, 48} {
            for a := 0; a < 12; a++ {
                ang := float64(a) * 30 * math.Pi / 180
                px := wp.X + math.Cos(ang)*ring
                py := wp.Y + math.Sin(ang)*ring
                pz, err := engine.ClosestHeight(px, py, int16(wp.Z))
                if err != nil {
                    continue
                }
                if _, ok := engine.ValidateClick(stuck,
                    pathfind.Vec3{X: px, Y: py, Z: float64(pz)}); ok {
                    hits++
                    if hits <= 3 {
                        fmt.Printf("    ring %.0f ang %d (%.0f %.0f): "+
                            "Y\n", ring, a*30, px, py)
                    }
                }
            }
        }
        fmt.Printf("    ring hits: %d of 36\n", hits)
        // The back bend (the way it came).
        _, ok := engine.ValidateClick(stuck, pathfind.Vec3{
            X: prev.X, Y: prev.Y, Z: prev.Z,
        })
        fmt.Printf("    back wp[%d] (%.0f %.0f): %v\n", i-1, prev.X,
            prev.Y, ok)
    }
}

// legZ interpolates the surface height along the incoming leg at the
// given distance short of the turn waypoint.
func legZ(wp, prev navmesh.Pos, back float64) float64 {
    dx, dy := wp.X-prev.X, wp.Y-prev.Y
    leg := math.Hypot(dx, dy)
    if leg < 1 {
        return wp.Z
    }

    return wp.Z + (prev.Z-wp.Z)*(back/leg)
}

func vec(x, y, z float64) pathfind.Vec3 {
    return pathfind.Vec3{X: x, Y: y, Z: z}
}

func meshPos(v pathfind.Vec3) navmesh.Pos {
    return navmesh.Pos{X: v.X, Y: v.Y, Z: v.Z}
}

// clearedFilter mirrors the hunt navigator filter: the funnel pivot
// clearance of the fighter capsule, the shortcut pass, the C1 water
// pricing.
func clearedFilter() navmesh.Filter {
    filter := navmesh.DefaultFilter()
    filter.WaypointClearance = pathfind.DefaultCollisionRadius
    filter.Smooth = true
    filter.WaterZones = navmesh.C1WaterZones()

    return filter
}
