// Scratch analysis of navmesh tile geometry and the raw geodata: hunts
// the polygon shapes behind the viewer defects (steep cascade quads,
// stacked duplicate layers, riverbed water class).
package main

import (
    "fmt"
    "math"
    "os"
    "sort"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navbuild"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// countZFight counts the stacked polygon pairs whose mid heights sit
// within 8 units over a shared cell.
func countZFight(tile *navmesh.Tile) int {
    type cover struct {
        lo, hi int32
    }
    covers := make(map[int32][]cover)
    for i := range tile.Polys {
        p := &tile.Polys[i]
        lo, hi := int32(p.H00), int32(p.H00)
        for _, h := range []int16{p.H10, p.H01, p.H11} {
            if int32(h) < lo {
                lo = int32(h)
            }
            if int32(h) > hi {
                hi = int32(h)
            }
        }
        for x := p.X0; x < p.X1; x++ {
            for y := p.Y0; y < p.Y1; y++ {
                covers[x*2048+y] = append(covers[x*2048+y],
                    cover{lo, hi})
            }
        }
    }
    zfight := 0
    for _, cs := range covers {
        if len(cs) < 2 {
            continue
        }
        heights := make([]float64, len(cs))
        for k, c := range cs {
            heights[k] = float64(c.lo+c.hi) / 2
        }
        for a := range heights {
            for b := a + 1; b < len(heights); b++ {
                d := heights[a] - heights[b]
                if d < 0 {
                    d = -d
                }
                if d <= 8 {
                    zfight++
                }
            }
        }
    }

    return zfight
}

func main() {
    if len(os.Args) < 2 {
        fmt.Println("usage: navanalyze <tile.nm> [geodata region file]")
        os.Exit(1)
    }
    // A/B: build the region from the geodata with both dedup deltas
    // and count the z-fighting pairs of each.
    if len(os.Args) >= 4 {
        geo, err := os.ReadFile(os.Args[3]) //nolint:gosec // operator named
        if err != nil {
            panic(err)
        }
        for _, delta := range []int32{16, 32} {
            opts := navbuild.DefaultOptions()
            opts.DedupDelta = delta
            build, err := navbuild.BuildRegion(geo, 22, 16, opts)
            if err != nil {
                panic(err)
            }
            fmt.Printf("dedup %d: %d polys, %d sheets, zfight %d\n",
                delta, len(build.Tile.Polys), build.Stats.Sheets,
                countZFight(build.Tile))
        }
    }

    data, err := os.ReadFile(os.Args[1]) //nolint:gosec // operator named
    if err != nil {
        panic(err)
    }
    tile, err := navmesh.DecodeTile(data)
    if err != nil {
        panic(err)
    }
    fmt.Printf("tile %d_%d: %d polys, %d links\n",
        tile.Col, tile.Row, len(tile.Polys), len(tile.Links))

    steep45, steep60, isolatedSteep := 0, 0, 0
    for i := range tile.Polys {
        p := &tile.Polys[i]
        w := float64(p.X1-p.X0) * 16
        h := float64(p.Y1-p.Y0) * 16
        gx := math.Max(
            math.Abs(float64(p.H10-p.H00))/w,
            math.Abs(float64(p.H11-p.H01))/w)
        gy := math.Max(
            math.Abs(float64(p.H01-p.H00))/h,
            math.Abs(float64(p.H11-p.H10))/h)
        slopeDeg := math.Atan(math.Max(gx, gy)) * 180 / math.Pi
        if slopeDeg >= 45 {
            steep45++
            if slopeDeg >= 60 {
                steep60++
            }
            if p.FirstLink < 0 {
                isolatedSteep++
            }
        }
    }
    fmt.Printf("slope>=45: %d (%.1f%%)  >=60: %d  isolated steep: %d\n",
        steep45, 100*float64(steep45)/float64(len(tile.Polys)),
        steep60, isolatedSteep)

    // water depth distribution: how deep below the water level do the
    // water polys actually sit (the riverbed class).
    below := 0
    depthBuckets := map[int]int{}
    for i := range tile.Polys {
        p := &tile.Polys[i]
        if p.Area != navmesh.AreaWater {
            continue
        }
        mid := (int32(p.H00) + int32(p.H10) + int32(p.H01) + int32(p.H11)) / 4
        if mid < -3780 {
            below++
            depthBuckets[int(-3780-mid)/256]++
        }
    }
    fmt.Printf("water polys strictly below the water level: %d\n", below)
    keys := make([]int, 0, len(depthBuckets))
    for k := range depthBuckets {
        keys = append(keys, k)
    }
    sort.Ints(keys)
    for _, k := range keys {
        fmt.Printf("  depth %4d..%4d: %d polys\n", k*256, (k+1)*256,
            depthBuckets[k])
    }

    // near-coincident stacked polygon pairs (the z-fighting measure).
    type cover struct {
        lo, hi int32
    }
    covers := make(map[int32][]cover)
    for i := range tile.Polys {
        p := &tile.Polys[i]
        lo, hi := int32(p.H00), int32(p.H00)
        for _, h := range []int16{p.H10, p.H01, p.H11} {
            if int32(h) < lo {
                lo = int32(h)
            }
            if int32(h) > hi {
                hi = int32(h)
            }
        }
        for x := p.X0; x < p.X1; x++ {
            for y := p.Y0; y < p.Y1; y++ {
                covers[x*2048+y] = append(covers[x*2048+y],
                    cover{lo, hi})
            }
        }
    }
    // the precise z-fighting measure: bilinear heights at the shared
    // cell centers of stacked polygons
    zfight := 0
    for _, cs := range covers {
        if len(cs) < 2 {
            continue
        }
        hAt := func(c cover) float64 {
            return float64(c.lo+c.hi) / 2
        }
        heights := make([]float64, len(cs))
        for k, c := range cs {
            heights[k] = hAt(c)
        }
        for a := range heights {
            for b := a + 1; b < len(heights); b++ {
                d := heights[a] - heights[b]
                if d < 0 {
                    d = -d
                }
                if d <= 8 {
                    zfight++
                }
            }
        }
    }

    fmt.Printf("z-fighting pairs (mid heights within 8): %d\n", zfight)

    if len(os.Args) < 3 {
        return
    }
    geo, err := os.ReadFile(os.Args[2]) //nolint:gosec // operator named
    if err != nil {
        panic(err)
    }
    region, err := pathfind.ParseRegionData(geo,
        pathfind.RegionKey{Col: tile.Col, Row: tile.Row})
    if err != nil {
        panic(err)
    }

    // duplicate layer noise: stacks whose consecutive layers sit within
    // 0..32 units, split by the open/blocked combination (only the
    // both-open pairs become stacked sheets).
    exact := map[int32]int{}
    exactOpen := map[int32]int{}
    dups := 0
    stack := make([]pathfind.Layer, 0, 8)
    for cx := range 2048 {
        for cy := range 2048 {
            stack = region.LayerStack(pathfind.Point{X: int32(cx),
                Y: int32(cy)}, stack[:0])
            for i := 1; i < len(stack); i++ {
                d := int32(stack[i].Height) - int32(stack[i-1].Height)
                if d < 0 {
                    d = -d
                }
                if d <= 32 {
                    dups++
                    exact[d]++
                    if stack[i].NSWE != 0 && stack[i-1].NSWE != 0 {
                        exactOpen[d]++
                    }
                }
            }
        }
    }
    fmt.Printf("geodata consecutive layer pairs within 32: %d\n", dups)
    dk := make([]int32, 0, len(exact))
    for d := range exact {
        dk = append(dk, d)
    }
    sort.Slice(dk, func(i, j int) bool { return dk[i] < dk[j] })
    for _, d := range dk {
        fmt.Printf("  delta %d: %d pairs (%d both-open)\n", d, exact[d],
            exactOpen[d])
    }

    // dump example cells carrying both-open near-duplicate pairs.
    fmt.Println("both-open delta-32/24/16 example stacks:")
    shown := 0
    for cx := 0; cx < 2048 && shown < 14; cx++ {
        for cy := 0; cy < 2048 && shown < 14; cy++ {
            stack = region.LayerStack(pathfind.Point{X: int32(cx),
                Y: int32(cy)}, stack[:0])
            for i := 1; i < len(stack); i++ {
                d := int32(stack[i].Height) - int32(stack[i-1].Height)
                if d < 0 {
                    d = -d
                }
                if d >= 16 && d <= 32 && stack[i].NSWE != 0 &&
                    stack[i-1].NSWE != 0 {
                    fmt.Printf("  (%d,%d):", cx, cy)
                    for _, l := range stack {
                        fmt.Printf(" h=%d nswe=%b", l.Height, l.NSWE)
                    }
                    fmt.Println()
                    shown++

                    break
                }
            }
        }
    }

    // dump one ramp: the cells around the max-span polygon of 22_16.
    fmt.Println("ramp dump (x 1568..1603, y 106..111):")
    for cx := 1568; cx <= 1603; cx++ {
        for cy := 106; cy <= 111; cy++ {
            stack = region.LayerStack(pathfind.Point{X: int32(cx),
                Y: int32(cy)}, stack[:0])
            if len(stack) == 0 {
                continue
            }
            fmt.Printf("  (%d,%d):", cx, cy)
            for _, l := range stack {
                fmt.Printf(" h=%d nswe=%b", l.Height, l.NSWE)
            }
            fmt.Println()
        }
    }
}
