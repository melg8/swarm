// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

// The pack compression verification of issue #11: runs the same
// route queries over the old pack (the default zstd level, the plain
// sidecars) and the new pack (the best level, the zstd framed
// sidecars) and compares the answers - the compressed transport must
// not move a single waypoint. The queries stay inside the 20_18..21_20
// block both packs carry.
package main

import (
    "fmt"
    "os"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

func main() {
    oldDir := os.Args[1]
    newDir := os.Args[2]

    // The query corpus: the elven village block endpoints of the
    // town trips and the hunt cells (world coordinates).
    queries := [][2]navmesh.Pos{
        {
            {X: 46880, Y: 50752, Z: -2889},
            {X: 47595, Y: 51569, Z: -2992},
        },
        {
            {X: 45000, Y: 50000, Z: -3500},
            {X: 46880, Y: 50752, Z: -2889},
        },
        {
            {X: 43632, Y: 50560, Z: -2960},
            {X: 47595, Y: 51569, Z: -2992},
        },
        {
            {X: 45150, Y: 50150, Z: -3500},
            {X: 41920, Y: 52128, Z: -3000},
        },
        {
            {X: 46880, Y: 50752, Z: -2889},
            {X: 43632, Y: 50560, Z: -2960},
        },
    }

    oldMesh := navmesh.NewMesh(oldDir)
    newMesh := navmesh.NewMesh(newDir)
    mismatch := 0
    for i, query := range queries {
        fmt.Printf("query %d running...\n", i)
        oldRoute, oldErr := oldMesh.Route(query[0], query[1],
            navmesh.DefaultFilter())
        newRoute, newErr := newMesh.Route(query[0], query[1],
            navmesh.DefaultFilter())
        if (oldErr == nil) != (newErr == nil) {
            fmt.Printf("query %d: verdict mismatch: old %v new %v\n",
                i, oldErr, newErr)
            mismatch++
            continue
        }
        if oldErr != nil {
            fmt.Printf("query %d: both refuse (%v)\n", i, oldErr)
            continue
        }
        if oldRoute.Found != newRoute.Found ||
            oldRoute.Partial != newRoute.Partial {
            fmt.Printf("query %d: verdict mismatch (found/partial)\n", i)
            mismatch++
            continue
        }
        if len(oldRoute.Waypoints) != len(newRoute.Waypoints) {
            fmt.Printf("query %d: %d vs %d waypoints\n", i,
                len(oldRoute.Waypoints), len(newRoute.Waypoints))
            mismatch++
            continue
        }
        same := true
        for w := range oldRoute.Waypoints {
            if oldRoute.Waypoints[w] != newRoute.Waypoints[w] {
                fmt.Printf("query %d waypoint %d: %v vs %v\n",
                    i, w, oldRoute.Waypoints[w], newRoute.Waypoints[w])
                same = false
                break
            }
        }
        if !same {
            mismatch++
            continue
        }
        fmt.Printf("query %d: identical (%d waypoints, found=%v)\n", i,
            len(oldRoute.Waypoints), oldRoute.Found)
    }
    if mismatch > 0 {
        fmt.Printf("FAILED: %d mismatches\n", mismatch)
        os.Exit(1)
    }
    fmt.Println("ALL QUERIES IDENTICAL")
}
