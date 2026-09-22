// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

// The route regression corpus of the navmesh pack (owner issue #27):
// the harness that grows the five query pairwise comparison of the
// transport round (#11) into a recorded corpus. Three modes:
//
//    navpack-verify generate -mesh DIR -out FILE [-seed N]
//        [-samples N] [-anchors FILE]
//    navpack-verify replay -mesh DIR -corpus FILE
//    navpack-verify compare -old DIR -new DIR [-corpus FILE]
//
// generate walks a deterministic query set over one pack - the town
// trip and hunt cell anchors, the cross region corridors and the
// seeded random pairs - runs every query and records the answer (the
// verdict, the waypoint count, the waypoint stream hash, the first
// and last waypoints) into the corpus JSON. replay runs the same
// queries over one pack and refuses any answer drift, so a structurally
// reduced pack (the polygon count round) proves the route answers
// stay identical before it lands. compare keeps the legacy two pack
// pairwise form of the transport round (the compressed transport must
// not move a single waypoint), optionally binding both sides to a
// recorded corpus. Every mode exits nonzero on any mismatch: the
// corpus is the gate, not a report.
package main

import (
    "bytes"
    "crypto/sha256"
    "encoding/binary"
    "encoding/hex"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "math"
    "math/rand"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The corpus wire version: bump on any answer semantics change, the
// replay refuses a corpus of a foreign version instead of comparing
// apples to oranges.
const corpusVersion = 1

// The verdict classes of a recorded answer: the found and partial
// routes and the refusals (a missing mesh under an endpoint or any
// other query error) - the structural round must not flip any of the
// three.
const (
    verdictFound   = "found"
    verdictPartial = "partial"
    verdictRefused = "refused"
)

// The query kinds of the generator: the hand-picked anchors (the town
// trips and the hunt cells the traffic walks), the region to region
// corridors and the seeded random pairs.
const (
    kindAnchor   = "anchor"
    kindCorridor = "corridor"
    kindSample   = "sample"
    kindHuntCell = "hunt-cell"
)

// corpusPos is one world position of the corpus JSON (the navmesh.Pos
// fields as they are).
type corpusPos struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    Z float64 `json:"z"`
}

// corpusAnswer is the recorded answer of one query: everything the
// replay compares - the verdict class, the error class of a refusal,
// the waypoint count and the hash of the waypoint stream.
type corpusAnswer struct {
    Verdict   string    `json:"verdict"`
    Error     string    `json:"error,omitempty"`
    Waypoints int       `json:"waypoints"`
    SHA       string    `json:"waypoint_sha256"`
    First     corpusPos `json:"first"`
    Last      corpusPos `json:"last"`
}

// corpusQuery is one recorded query: the endpoints, the kind and the
// answer the pack under test must reproduce.
type corpusQuery struct {
    ID     string       `json:"id"`
    Kind   string       `json:"kind"`
    Start  corpusPos    `json:"start"`
    Goal   corpusPos    `json:"goal"`
    Answer corpusAnswer `json:"answer"`
}

// corpusFile is the corpus JSON: the query set with the recorded
// answers, plus the metadata the replay reads back (the version gate,
// the seed and the region list the generator ran over).
type corpusFile struct {
    Version    int           `json:"version"`
    Generator  string        `json:"generator"`
    RecordedAt string        `json:"recorded_at"`
    Seed       int64         `json:"seed"`
    Regions    []string      `json:"regions"`
    Queries    []corpusQuery `json:"queries"`
}

// posOf converts a navmesh position into the corpus shape.
func posOf(p navmesh.Pos) corpusPos {
    return corpusPos{X: p.X, Y: p.Y, Z: p.Z}
}

// zeroPos is the exhaustive zero position (the unset answer ends).
var zeroPos = corpusPos{X: 0, Y: 0, Z: 0}

// zeroAnswer is the exhaustive zero answer (the pre-record state of
// a generated query - runQuery overwrites it).
func zeroAnswer() corpusAnswer {
    return corpusAnswer{
        Verdict: "", Error: "", Waypoints: 0, SHA: "",
        First: zeroPos, Last: zeroPos,
    }
}

// newQuery is the constructor of the pre-record queries: the one
// place the full query literal lives, the generators name the
// endpoints and move on.
func newQuery(
    id, kind string, start, goal navmesh.Pos,
) corpusQuery {
    return corpusQuery{
        ID: id, Kind: kind,
        Start: posOf(start), Goal: posOf(goal),
        Answer: zeroAnswer(),
    }
}

// waypointSHA hashes the waypoint stream: the little endian float64
// bits of every waypoint in order - any moved waypoint flips the
// hash, the count alone would miss the substitutions.
func waypointSHA(route []navmesh.Pos) string {
    h := sha256.New()
    var buf [24]byte
    for _, p := range route {
        binary.LittleEndian.PutUint64(buf[0:8], math.Float64bits(p.X))
        binary.LittleEndian.PutUint64(buf[8:16], math.Float64bits(p.Y))
        binary.LittleEndian.PutUint64(buf[16:24], math.Float64bits(p.Z))
        h.Write(buf[:])
    }

    return hex.EncodeToString(h.Sum(nil))
}

// runQuery answers one query over the mesh: the recorded answer
// shape. A refusal records the error CLASS, not the error text - the
// text drifts with the versions, the class is the regression signal.
func runQuery(mesh *navmesh.Mesh, q corpusQuery) corpusAnswer {
    route, err := mesh.Route(
        navmesh.Pos(q.Start), navmesh.Pos(q.Goal),
        navmesh.DefaultFilter())
    if err != nil {
        var noNav *navmesh.NoNavmeshError
        class := "error"
        if errors.As(err, &noNav) {
            class = "no-navmesh"
        }

        return corpusAnswer{
            Verdict: verdictRefused, Error: class,
            Waypoints: 0, SHA: waypointSHA(nil),
            First: corpusPos{X: 0, Y: 0, Z: 0},
            Last:  corpusPos{X: 0, Y: 0, Z: 0},
        }
    }
    verdict := verdictFound
    if route.Partial {
        verdict = verdictPartial
    }
    first := corpusPos{X: 0, Y: 0, Z: 0}
    last := corpusPos{X: 0, Y: 0, Z: 0}
    if len(route.Waypoints) > 0 {
        first = posOf(route.Waypoints[0])
        last = posOf(route.Waypoints[len(route.Waypoints)-1])
    }

    return corpusAnswer{
        Verdict: verdict, Error: "",
        Waypoints: len(route.Waypoints), SHA: waypointSHA(route.Waypoints),
        First: first, Last: last,
    }
}

// answersDiffer compares two answers the replay way: every recorded
// field must match.
func answersDiffer(a, b corpusAnswer) bool {
    return a.Verdict != b.Verdict || a.Error != b.Error ||
        a.Waypoints != b.Waypoints || a.SHA != b.SHA ||
        a.First != b.First || a.Last != b.Last
}

// regionKey is one pack region: the col/row key and its name form.
type regionKey struct {
    col, row int16
    name     string
}

// listRegions enumerates the pack regions: every X_Y.nm file of the
// directory, sorted by name - the deterministic base of the generator.
func listRegions(dir string) ([]regionKey, error) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, fmt.Errorf("read the pack directory: %w", err)
    }
    var keys []regionKey
    for _, e := range entries {
        if filepath.Ext(e.Name()) != ".nm" {
            continue
        }
        base := strings.TrimSuffix(e.Name(), ".nm")
        parts := strings.SplitN(base, "_", 2)
        if len(parts) != 2 {
            continue
        }
        var col, row int16
        if _, err := fmt.Sscanf(
            parts[0]+" "+parts[1], "%d %d", &col, &row,
        ); err != nil {
            continue
        }
        keys = append(keys, regionKey{col: col, row: row, name: base})
    }
    if len(keys) == 0 {
        return nil, fmt.Errorf("no tiles under %s", dir)
    }
    sort.Slice(keys, func(i, j int) bool {
        return keys[i].name < keys[j].name
    })

    return keys, nil
}

// regionRect reports the world rectangle of one region: the origin
// anchor the region name derives from and the world size of one tile.
func regionRect(k regionKey) (minX, minY, size float64) {
    size = navmesh.TileWorldSize()
    minX = (float64(k.col) - float64(navmesh.TileZeroCol())) * size
    minY = (float64(k.row) - float64(navmesh.TileZeroRow())) * size

    return minX, minY, size
}

// regionCenter is the world center of one region.
func regionCenter(k regionKey) navmesh.Pos {
    minX, minY, size := regionRect(k)

    return navmesh.Pos{X: minX + size/2, Y: minY + size/2, Z: 0}
}

// The built-in anchor set of the transport round (the elven village
// block endpoints of the town trips and the hunt cells) - the real
// traffic queries, kept verbatim so the corpus of every round carries
// them.
func anchorQueries() []corpusQuery {
    pairs := [][2]navmesh.Pos{
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
    queries := make([]corpusQuery, 0, len(pairs))
    for i, p := range pairs {
        queries = append(queries, newQuery(
            fmt.Sprintf("anchor-%d", i+1), kindAnchor, p[0], p[1]))
    }

    return queries
}

// huntCell is the anchor file entry the generator reads: the focus
// point of one hunt cell (the cells_elven.json shape).
type huntCell struct {
    ID     string  `json:"id"`
    Name   string  `json:"name"`
    FocusX float64 `json:"focus_x"`
    FocusY float64 `json:"focus_y"`
}

// anchorPairsFromHuntCells reads the hunt cells file and pairs the
// cell foci into cross ground queries: cell i walks toward cell
// (i*7+11) mod len - a deterministic spread over the hunting grounds.
// The count caps at 64 to bound the generate run.
func anchorPairsFromHuntCells(path string) ([]corpusQuery, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read the anchors file: %w", err)
    }
    var cells []huntCell
    if err := json.Unmarshal(data, &cells); err != nil {
        return nil, fmt.Errorf("parse the anchors file: %w", err)
    }
    queries := make([]corpusQuery, 0, 64)
    for i := range cells {
        if len(queries) >= 64 {
            break
        }
        j := (i*7 + 11) % len(cells)
        if j == i {
            continue
        }
        queries = append(queries, newQuery(
            "hunt-"+cells[i].ID, kindHuntCell,
            navmesh.Pos{X: cells[i].FocusX, Y: cells[i].FocusY, Z: 0},
            navmesh.Pos{X: cells[j].FocusX, Y: cells[j].FocusY, Z: 0}))
    }

    return queries, nil
}

// corridorQueries pairs the region centers: every consecutive region
// pair in the sorted key order walks center to center, the first and
// the last region close the ring - the long cross region corridors
// the town trips ride.
func corridorQueries(regions []regionKey) []corpusQuery {
    queries := make([]corpusQuery, 0, len(regions))
    for i := 0; i < len(regions); i++ {
        j := (i + 1) % len(regions)
        queries = append(queries, newQuery(
            fmt.Sprintf("corridor-%s-%s",
                regions[i].name, regions[j].name),
            kindCorridor,
            regionCenter(regions[i]), regionCenter(regions[j])))
    }

    return queries
}

// sampleQueries draws the seeded random pairs over the pack polygons:
// one walkable surface point per endpoint, drawn straight from a
// random polygon of a random region (the rect center at its exact
// surface height - the corpus endpoints sit ON the world the pack
// carries, every sample answers a real route question). Every 4th
// sample pairs one region with itself - the intra region density the
// hunt walks produce.
func sampleQueries(
    mesh *navmesh.Mesh, regions []regionKey, seed int64, count int,
) []corpusQuery {
    rng := rand.New(rand.NewSource(seed))
    queries := make([]corpusQuery, 0, count)
    for i := 0; i < count; i++ {
        a := regions[rng.Intn(len(regions))]
        b := a
        if i%4 != 0 {
            b = regions[rng.Intn(len(regions))]
        }
        queries = append(queries, newQuery(
            fmt.Sprintf("sample-%04d", i+1), kindSample,
            surfacePoint(mesh, rng, a), surfacePoint(mesh, rng, b)))
    }

    return queries
}

// surfacePoint draws one walkable world point of one region: a
// random polygon of the region tile, its rect center, the surface
// height under it. A region whose tile fails to load falls back to
// the region center (the refusal records itself - the lost tile is
// a regression signal too).
func surfacePoint(
    mesh *navmesh.Mesh, rng *rand.Rand, k regionKey,
) navmesh.Pos {
    tile, err := mesh.Tile(
        navmesh.RegionKey{Col: k.col, Row: k.row})
    if err != nil || len(tile.Polys) == 0 {
        return regionCenter(k)
    }
    poly := &tile.Polys[rng.Intn(len(tile.Polys))]
    x0, y0, x1, y1 := tile.WorldRect(poly)
    x := (x0 + x1) / 2
    y := (y0 + y1) / 2

    return navmesh.Pos{X: x, Y: y, Z: tile.HeightAt(poly, x, y)}
}

// runGenerate builds the corpus over one pack: the query set first
// (deterministic: the sorted regions, the fixed anchors, the seeded
// samples), then the answers - every query runs and records.
func runGenerate(args []string) error {
    fs := flag.NewFlagSet("generate", flag.ExitOnError)
    meshDir := fs.String("mesh", "data/navmesh",
        "the pack directory the corpus records from")
    out := fs.String("out", "navmesh_corpus.json",
        "the corpus file to write")
    seed := fs.Int64("seed", 1, "the sample query seed")
    samples := fs.Int("samples", 256,
        "the seeded random pair count")
    anchors := fs.String("anchors", "",
        "optional hunt cells JSON (the focus point anchors)")
    if err := fs.Parse(args); err != nil {
        return err
    }

    regions, err := listRegions(*meshDir)
    if err != nil {
        return err
    }
    names := make([]string, len(regions))
    for i, k := range regions {
        names[i] = k.name
    }

    queries := anchorQueries()
    if *anchors != "" {
        hunt, err := anchorPairsFromHuntCells(*anchors)
        if err != nil {
            return err
        }
        queries = append(queries, hunt...)
    }
    mesh := navmesh.NewMesh(*meshDir)
    queries = append(queries, corridorQueries(regions)...)
    queries = append(queries,
        sampleQueries(mesh, regions, *seed, *samples)...)
    for i := range queries {
        queries[i].Answer = runQuery(mesh, queries[i])
    }

    corpus := corpusFile{
        Version:    corpusVersion,
        Generator:  "navpack-verify/1",
        RecordedAt: time.Now().UTC().Format(time.RFC3339),
        Seed:       *seed,
        Regions:    names,
        Queries:    queries,
    }
    data, err := json.MarshalIndent(corpus, "", "  ")
    if err != nil {
        return fmt.Errorf("encode the corpus: %w", err)
    }
    if err := os.WriteFile(*out, data, 0o600); err != nil {
        return fmt.Errorf("write the corpus: %w", err)
    }

    found := 0
    for _, q := range queries {
        if q.Answer.Verdict == verdictFound {
            found++
        }
    }
    fmt.Printf("corpus %s: %d queries over %d regions "+
        "(%d found, %d partial, %d refused)\n",
        *out, len(queries), len(regions), found,
        countVerdict(queries, verdictPartial),
        countVerdict(queries, verdictRefused))

    return nil
}

// countVerdict counts the queries of one verdict class (the summary
// line).
func countVerdict(queries []corpusQuery, verdict string) int {
    n := 0
    for _, q := range queries {
        if q.Answer.Verdict == verdict {
            n++
        }
    }

    return n
}

// loadCorpus reads and validates the corpus file: the version gate
// first, a foreign corpus refuses instead of comparing.
func loadCorpus(path string) (*corpusFile, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read the corpus: %w", err)
    }
    dec := json.NewDecoder(bytes.NewReader(data))
    dec.DisallowUnknownFields()
    var corpus corpusFile
    if err := dec.Decode(&corpus); err != nil {
        return nil, fmt.Errorf("parse the corpus: %w", err)
    }
    if corpus.Version != corpusVersion {
        return nil, fmt.Errorf(
            "corpus version %d, the tool speaks %d - regenerate",
            corpus.Version, corpusVersion)
    }

    return &corpus, nil
}

// runReplay replays the corpus over one pack: every query must
// answer exactly the recorded answer, any drift names the query and
// the differing fields and fails the run.
func runReplay(args []string) error {
    fs := flag.NewFlagSet("replay", flag.ExitOnError)
    meshDir := fs.String("mesh", "data/navmesh",
        "the pack directory the corpus verifies")
    corpusPath := fs.String("corpus", "",
        "the corpus file to replay")
    if err := fs.Parse(args); err != nil {
        return err
    }
    if *corpusPath == "" {
        return errors.New("replay needs -corpus")
    }

    corpus, err := loadCorpus(*corpusPath)
    if err != nil {
        return err
    }
    mesh := navmesh.NewMesh(*meshDir)
    mismatch := 0
    for _, q := range corpus.Queries {
        got := runQuery(mesh, q)
        if !answersDiffer(got, q.Answer) {
            continue
        }
        mismatch++
        fmt.Printf("%s (%s): ANSWER DRIFT\n  recorded: %s\n"+
            "  actual:   %s\n", q.ID, q.Kind,
            answerText(q.Answer), answerText(got))
    }
    if mismatch > 0 {
        return fmt.Errorf("%d of %d corpus queries drifted",
            mismatch, len(corpus.Queries))
    }
    fmt.Printf("CORPUS IDENTICAL: %d queries over %s\n",
        len(corpus.Queries), *meshDir)

    return nil
}

// answerText renders one answer for the report lines.
func answerText(a corpusAnswer) string {
    if a.Verdict == verdictRefused {
        return fmt.Sprintf("%s (%s)", a.Verdict, a.Error)
    }

    return fmt.Sprintf("%s, %d waypoints, sha %s",
        a.Verdict, a.Waypoints, a.SHA[:12])
}

// runCompare keeps the two pack pairwise contract of the transport
// round: the same queries run over both packs, the answers must
// match side for side. With -corpus the query set is the corpus,
// without it the built-in anchors - the form the transport round
// shipped.
func runCompare(args []string) error {
    fs := flag.NewFlagSet("compare", flag.ExitOnError)
    oldDir := fs.String("old", "",
        "the reference pack directory")
    newDir := fs.String("new", "",
        "the pack under test")
    corpusPath := fs.String("corpus", "",
        "optional corpus file (the query set to compare over)")
    if err := fs.Parse(args); err != nil {
        return err
    }
    refDir, testDir := *oldDir, *newDir
    if fs.NArg() == 2 {
        // The legacy positional form: navpack-verify OLD NEW.
        refDir, testDir = fs.Arg(0), fs.Arg(1)
    }
    if refDir == "" || testDir == "" {
        return errors.New(
            "compare needs -old and -new (or the OLD NEW arguments)")
    }

    var queries []corpusQuery
    if *corpusPath != "" {
        corpus, err := loadCorpus(*corpusPath)
        if err != nil {
            return err
        }
        queries = corpus.Queries
    } else {
        queries = anchorQueries()
    }

    oldMesh := navmesh.NewMesh(refDir)
    newMesh := navmesh.NewMesh(testDir)
    mismatch := 0
    for _, q := range queries {
        oldAnswer := runQuery(oldMesh, q)
        newAnswer := runQuery(newMesh, q)
        if !answersDiffer(oldAnswer, newAnswer) {
            fmt.Printf("%s (%s): identical (%s)\n",
                q.ID, q.Kind, answerText(oldAnswer))
            continue
        }
        mismatch++
        fmt.Printf("%s (%s): MISMATCH\n  old: %s\n  new: %s\n",
            q.ID, q.Kind, answerText(oldAnswer), answerText(newAnswer))
    }
    if mismatch > 0 {
        return fmt.Errorf("%d of %d queries mismatch",
            mismatch, len(queries))
    }
    fmt.Println("ALL QUERIES IDENTICAL")

    return nil
}

func main() {
    args := os.Args[1:]
    if len(args) == 0 {
        fmt.Println(usage)
        os.Exit(2)
    }
    var err error
    switch args[0] {
    case "generate":
        err = runGenerate(args[1:])
    case "replay":
        err = runReplay(args[1:])
    case "compare":
        err = runCompare(args[1:])
    default:
        // The legacy positional form of the transport round.
        err = runCompare(args)
    }
    if err != nil {
        fmt.Println("FAILED:", err)
        os.Exit(1)
    }
}

// usage is the one screen command surface.
const usage = `navpack-verify: the route regression corpus of the navmesh pack

  navpack-verify generate -mesh DIR -out FILE [-seed N] [-samples N]
      [-anchors FILE]
      record the route answers of one pack into the corpus file

  navpack-verify replay -mesh DIR -corpus FILE
      verify one pack against the recorded answers (any drift fails)

  navpack-verify compare -old DIR -new DIR [-corpus FILE]
      the two pack pairwise comparison (the transport round form)`
