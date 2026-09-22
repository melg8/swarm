// The per query memory probe of the corpus generate (issue #56
// diagnostic): runs the corpus query shapes over one pack one by
// one, prints the verdict and the RSS after every query - the query
// that explodes the memory names itself.
package main

import (
    "fmt"
    "os"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

func rssKB() int64 {
    data, err := os.ReadFile("/proc/self/status")
    if err != nil {
        return -1
    }
    kb := int64(-1)
    for start := 0; start < len(data); {
        end := start
        for end < len(data) && data[end] != '\n' {
            end++
        }
        line := string(data[start:end])
        if len(line) > 6 && line[:6] == "VmRSS:" {
            fmt.Sscanf(line, "VmRSS: %d", &kb)
        }
        start = end + 1
    }

    return kb
}

func main() {
    dir := "/tmp/pack_subset"
    if len(os.Args) > 1 {
        dir = os.Args[1]
    }
    limit := 40
    if len(os.Args) > 2 {
        limit, _ = strconv.Atoi(os.Args[2])
    }
    mesh := navmesh.NewMesh(dir)

    type q struct {
        name string
        a, b [3]float64
    }
    anchors := []q{
        {"anchor-1", [3]float64{46880, 50752, -2889}, [3]float64{47595, 51569, -2992}},
        {"anchor-2", [3]float64{45000, 50000, -3500}, [3]float64{46880, 50752, -2889}},
        {"anchor-3", [3]float64{43632, 50560, -2960}, [3]float64{47595, 51569, -2992}},
        {"anchor-4", [3]float64{45150, 50150, -3500}, [3]float64{41920, 52128, -3000}},
        {"anchor-5", [3]float64{46880, 50752, -2889}, [3]float64{43632, 50560, -2960}},
    }
    run := func(name string, a, b [3]float64) {
        start := time.Now()
        route, err := mesh.Route(
            navmesh.Pos{X: a[0], Y: a[1], Z: a[2]},
            navmesh.Pos{X: b[0], Y: b[1], Z: b[2]},
            navmesh.DefaultFilter())
        elapsed := time.Since(start)
        verdict := "found"
        if err != nil {
            verdict = "refused"
        } else if route.Partial {
            verdict = "partial"
        }
        waypoints := 0
        if err == nil {
            waypoints = len(route.Waypoints)
        }
        fmt.Printf("%-14s %-10s wp=%4d %10s RSS=%dMB\n",
            name, verdict, waypoints,
            elapsed.Round(time.Millisecond), rssKB()/(1<<10))
    }
    for _, c := range anchors {
        run(c.name, c.a, c.b)
    }
    regions := [][2]float64{
        {19, 19}, {19, 20}, {19, 21}, {20, 19}, {20, 20}, {20, 21},
        {21, 19}, {21, 20}, {21, 21}, {22, 19}, {22, 20}, {22, 21},
    }
    for i := range regions {
        j := (i + 1) % len(regions)
        x0 := (regions[i][0] - 20) * 32768
        y0 := (regions[i][1] - 18) * 32768
        x1 := (regions[j][0] - 20) * 32768
        y1 := (regions[j][1] - 18) * 32768
        run(fmt.Sprintf("corridor-%02d", i),
            [3]float64{x0 + 16384, y0 + 16384, 0},
            [3]float64{x1 + 16384, y1 + 16384, 0})
    }
    rng := uint32(1)
    next := func() uint32 {
        rng = rng*1664525 + 1013904223

        return rng
    }
    for i := 0; i < limit; i++ {
        col := int16(19 + next()%4)
        row := int16(19 + next()%4)
        tile, err := mesh.Tile(navmesh.RegionKey{Col: col, Row: row})
        if err != nil || len(tile.Polys) == 0 {
            fmt.Printf("sample-%03d      no tile %d_%d\n", i, col, row)

            continue
        }
        pa := &tile.Polys[next()%uint32(len(tile.Polys))]
        x0w, y0w, x1w, y1w := tile.WorldRect(pa)
        run(fmt.Sprintf("sample-%03d", i),
            [3]float64{(x0w + x1w) / 2, (y0w + y1w) / 2,
                tile.HeightAt(pa, (x0w+x1w)/2, (y0w+y1w)/2)},
            [3]float64{x0w, y0w, tile.HeightAt(pa, x0w, y0w)})
    }
    fmt.Println("PROBE COMPLETE")
}
