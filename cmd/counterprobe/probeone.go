// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// probeOne is the debugging helper of the counterprobe world pass
// (-mode one -scan <template id>): it dumps the spawn cell
// classification, the close cell map and one verification route of
// the named merchant.
func probeOne(geodata string, id int32) {
    worldGeodata = geodata
    spawn, ok := npcdata.MerchantSpawnOf(id)
    if !ok {
        fmt.Println("unknown merchant id")

        return
    }
    m := merchant{
        id:      id,
        name:    spawn.Name,
        x:       float64(spawn.X),
        y:       float64(spawn.Y),
        z:       float64(spawn.Z),
        heading: float64(spawn.Heading),
    }
    fmt.Printf("%s (%d) at %.0f %.0f %.0f heading %.0f\n",
        m.name, m.id, m.x, m.y, m.z, m.heading/65536*360)

    region := worldRegionOf(m.x, m.y)
    if region == nil {
        fmt.Println("region missing or unparsable")

        return
    }
    engine := pathfind.NewEngine(geodata)
    z, err := engine.ClosestHeight(m.x, m.y, int16(m.z))
    fmt.Printf("ClosestHeight: %d %v, hasFloor %v\n", z, err,
        hasFloor(region, m.x, m.y, int(m.z)))

    center := pathfind.WorldToCell(m.x, m.y)
    buf := make([]pathfind.Layer, 0, 8)
    for row := -4; row <= 4; row++ {
        fmt.Print("  ")
        for col := -4; col <= 4; col++ {
            cell := pathfind.Point{
                X: center.X + int32(col), Y: center.Y + int32(row),
            }
            stack := region.LayerStack(pathfind.LocalCell(cell), buf)
            fmt.Print(classify(stack, int(m.z)))
        }
        fmt.Println()
    }

    ways := verifyWorldStand(engine, m, m.x, m.y, float64(z))
    fmt.Printf("spawn verification ways: %d\n", ways)

    // One route dump toward the spawn from the east.
    start := pathfind.Vec3{X: m.x + 400, Y: m.y, Z: float64(z)}
    sz, serr := engine.ClosestHeight(start.X, start.Y, int16(m.z))
    fmt.Printf("east start ClosestHeight: %d %v\n", sz, serr)
    if serr == nil {
        start.Z = float64(sz)
        result, rerr := engine.FindPathApproach(start,
            pathfind.Vec3{X: m.x, Y: m.y, Z: float64(z)},
            16, pathfind.DefaultMaxPassableHeight)
        found := result != nil && result.Found
        count := 0
        if result != nil {
            count = len(result.Waypoints)
        }
        fmt.Printf("route err %v, found %v, waypoints %d\n",
            rerr, found, count)
    }
}

// parseID parses the -scan value of the one mode (the template id).
func parseID(value string) int64 {
    var id int64
    if _, err := fmt.Sscanf(value, "%d", &id); err != nil {
        return 0
    }

    return id
}
