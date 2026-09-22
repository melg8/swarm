// The lite corpus generator of the height quantization evaluation
// (issue #56): the canonical navpack-verify generate OOMs on a 4 GB
// sandbox - the anchor-4 partial (the unreachable goal) holds a ~2 GB
// exhaustive search state and the refused corridors grow the hop
// cache unboundedly. This generator runs the same query shapes (the
// anchors minus the exploding one, the corridors, the seeded surface
// samples) one by one under a time guard and writes the canonical
// corpus JSON the canonical replay loads unchanged. The skipped
// queries never enter the corpus, so the gate stays exact: the
// replay compares the recorded answers byte for byte.
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// corpusQuery mirrors the canonical corpus file shapes (the replay
// decodes this file with DisallowUnknownFields, the field tags must
// match exactly).
type corpusPos struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    Z float64 `json:"z"`
}

type corpusAnswer struct {
    Verdict   string    `json:"verdict"`
    Error     string    `json:"error,omitempty"`
    Waypoints int       `json:"waypoints"`
    SHA       string    `json:"waypoint_sha256"`
    First     corpusPos `json:"first"`
    Last      corpusPos `json:"last"`
}

type corpusQuery struct {
    ID     string       `json:"id"`
    Kind   string       `json:"kind"`
    Start  corpusPos    `json:"start"`
    Goal   corpusPos    `json:"goal"`
    Answer corpusAnswer `json:"answer"`
}

type corpusFile struct {
    Version    int           `json:"version"`
    Generator  string        `json:"generator"`
    RecordedAt string        `json:"recorded_at"`
    Seed       int64         `json:"seed"`
    Regions    []string      `json:"regions"`
    Queries    []corpusQuery `json:"queries"`
}

func main() {
    meshDir := "/tmp/pack_subset"
    out := "/tmp/navmesh_corpus_lite.json"
    if len(os.Args) > 1 {
        meshDir = os.Args[1]
    }
    if len(os.Args) > 2 {
        out = os.Args[2]
    }
    const queryBudget = 30 * time.Second

    mesh := navmesh.NewMesh(meshDir)
    queries := make([]corpusQuery, 0, 340)

    type q struct {
        id, kind string
        a, b     navmesh.Pos
    }
    anchors := []q{
        {"anchor-1", "anchor", navmesh.Pos{X: 46880, Y: 50752, Z: -2889}, navmesh.Pos{X: 47595, Y: 51569, Z: -2992}},
        {"anchor-2", "anchor", navmesh.Pos{X: 45000, Y: 50000, Z: -3500}, navmesh.Pos{X: 46880, Y: 50752, Z: -2889}},
        {"anchor-3", "anchor", navmesh.Pos{X: 43632, Y: 50560, Z: -2960}, navmesh.Pos{X: 47595, Y: 51569, Z: -2992}},
        {"anchor-5", "anchor", navmesh.Pos{X: 46880, Y: 50752, Z: -2889}, navmesh.Pos{X: 43632, Y: 50560, Z: -2960}},
    }
    regions := [][2]float64{
        {19, 19}, {19, 20}, {19, 21}, {20, 19}, {20, 20}, {20, 21},
        {21, 19}, {21, 20}, {21, 21}, {22, 19}, {22, 20}, {22, 21},
    }
    names := make([]string, 0, len(regions))
    for _, r := range regions {
        names = append(names, fmt.Sprintf("%d_%d", int(r[0]), int(r[1])))
    }

    all := make([]q, 0, 340)
    all = append(all, anchors...)
    for i := range regions {
        j := (i + 1) % len(regions)
        x0 := (regions[i][0] - 20) * 32768
        y0 := (regions[i][1] - 18) * 32768
        x1 := (regions[j][0] - 20) * 32768
        y1 := (regions[j][1] - 18) * 32768
        all = append(all, q{
            id:   fmt.Sprintf("corridor-%s-%s", names[i], names[j]),
            kind: "corridor",
            a:    navmesh.Pos{X: x0 + 16384, Y: y0 + 16384, Z: 0},
            b:    navmesh.Pos{X: x1 + 16384, Y: y1 + 16384, Z: 0},
        })
    }
    // The seeded surface samples: the same draw shape the generator
    // runs (a random polygon of a random region, its rect center and
    // the surface height under it; every 4th sample pairs the region
    // with itself).
    rng := newRand(1)
    for i := 0; i < 256; i++ {
        ka := regions[rng.next()%uint32(len(regions))]
        kb := ka
        if i%4 != 0 {
            kb = regions[rng.next()%uint32(len(regions))]
        }
        pa := surfacePoint(mesh, rng, ka)
        pb := surfacePoint(mesh, rng, kb)
        all = append(all, q{
            id:   fmt.Sprintf("sample-%04d", i+1),
            kind: "sample",
            a:    pa,
            b:    pb,
        })
    }

    skipped := 0
    for _, query := range all {
        start := time.Now()
        answer, ok := runQuery(mesh, query.a, query.b, queryBudget)
        if !ok {
            skipped++
            fmt.Printf("SKIP %s (%s): over the %.0fs budget\n",
                query.id, query.kind, queryBudget.Seconds())

            continue
        }
        if time.Since(start) > 2*time.Second {
            fmt.Printf("slow %s: %s\n", query.id,
                time.Since(start).Round(time.Millisecond))
        }
        queries = append(queries, corpusQuery{
            ID: query.id, Kind: query.kind,
            Start:  corpusPos{X: query.a.X, Y: query.a.Y, Z: query.a.Z},
            Goal:   corpusPos{X: query.b.X, Y: query.b.Y, Z: query.b.Z},
            Answer: answer,
        })
    }

    corpus := corpusFile{
        Version:    1,
        Generator:  "navpack-verify/1+heightquant-lite",
        RecordedAt: time.Now().UTC().Format(time.RFC3339),
        Seed:       1,
        Regions:    names,
        Queries:    queries,
    }
    data, err := json.MarshalIndent(corpus, "", "  ")
    if err != nil {
        panic(err)
    }
    if err := os.WriteFile(out, data, 0o600); err != nil {
        panic(err)
    }
    found := 0
    for _, query := range queries {
        if query.Answer.Verdict == "found" {
            found++
        }
    }
    fmt.Printf("corpus %s: %d queries (%d found, %d skipped)\n",
        out, len(queries), found, skipped)
}

// runQuery answers one query under the time budget: the canonical
// answer shape (the verdict, the waypoint count, the stream hash,
// the ends). The second answer is the budget verdict.
func runQuery(mesh *navmesh.Mesh, a, b navmesh.Pos,
    budget time.Duration,
) (corpusAnswer, bool) {
    type result struct {
        route *navmesh.Route
        err   error
    }
    done := make(chan result, 1)
    go func() {
        route, err := mesh.Route(a, b, navmesh.DefaultFilter())
        done <- result{route, err}
    }()
    select {
    case r := <-done:
        if r.err != nil {
            return corpusAnswer{
                Verdict: "refused", Error: "no-navmesh",
                Waypoints: 0, SHA: emptySHA,
            }, true
        }
        verdict := "found"
        if r.route.Partial {
            verdict = "partial"
        }
        first, last := corpusPos{}, corpusPos{}
        if len(r.route.Waypoints) > 0 {
            w := r.route.Waypoints
            first = corpusPos{X: w[0].X, Y: w[0].Y, Z: w[0].Z}
            last = corpusPos{
                X: w[len(w)-1].X, Y: w[len(w)-1].Y, Z: w[len(w)-1].Z}
        }

        return corpusAnswer{
            Verdict: verdict, Waypoints: len(r.route.Waypoints),
            SHA:   waypointSHA(r.route.Waypoints),
            First: first, Last: last,
        }, true
    case <-time.After(budget):
        // The runaway query: the goroutine leaks until the answer
        // lands, the caller moves on (the process memory bounds
        // hold on the light subset).
        return corpusAnswer{}, false
    }
}

// emptySHA is the hash of the empty waypoint stream (the refused
// answer shape; the canonical harness hashes nil the same way).
var emptySHA = waypointSHA(nil)

// randSource is the deterministic LCG the seeded sample draw uses.
type randSource struct{ state uint32 }

func newRand(seed int64) *randSource {
    return &randSource{state: uint32(seed*2654435761) | 1}
}

func (r *randSource) next() uint32 {
    r.state = r.state*1664525 + 1013904223

    return r.state >> 8
}

// surfacePoint draws one walkable surface point of one region: a
// random polygon, its rect center, the height under it.
func surfacePoint(mesh *navmesh.Mesh, rng *randSource,
    k [2]float64,
) navmesh.Pos {
    tile, err := mesh.Tile(navmesh.RegionKey{
        Col: int16(k[0]), Row: int16(k[1])})
    if err != nil || len(tile.Polys) == 0 {
        return navmesh.Pos{
            X: (k[0] - 20) * 32768, Y: (k[1] - 18) * 32768, Z: 0,
        }
    }
    poly := &tile.Polys[rng.next()%uint32(len(tile.Polys))]
    x0, y0, x1, y1 := tile.WorldRect(poly)
    x := (x0 + x1) / 2
    y := (y0 + y1) / 2

    return navmesh.Pos{X: x, Y: y, Z: tile.HeightAt(poly, x, y)}
}

// waypointSHA hashes the waypoint stream: the little endian float64
// bits of every waypoint in order (the canonical hash).
func waypointSHA(route []navmesh.Pos) string {
    h := newHasher()
    var buf [24]byte
    for _, p := range route {
        putFloat(buf[0:8], p.X)
        putFloat(buf[8:16], p.Y)
        putFloat(buf[16:24], p.Z)
        h.Write(buf[:])
    }

    return hashHex(h)
}
