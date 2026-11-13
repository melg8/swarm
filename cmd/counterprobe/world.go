// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "fmt"
    "math"
    "os"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// The world stand derivation: the counter round pinned the eight counter
// trader stands by hand, the world pass derives an approach point for every
// one of the 93 merchant spawns on the grid engine (the world wide geodata
// authority). The spawn cell serves when the pack holds a floor at the spawn
// height and a verified route onto it; otherwise the heading push and the
// ring scan derive the customer cell and the pass pins it only when the grid
// route ends exactly on the cell with the line of sight of every leg
// (docs/hunting.md). The emit mode prints the rows as the hunt map source.

// curatedStands are the hand verified counter round stands (town.go): the
// world pass cross-checks them and the emit leaves them to the curated table.
var curatedStands = map[int32]pathfind.Vec3{
    7147: {X: 44584, Y: 46944, Z: -2984},
    7148: {X: 44584, Y: 46952, Z: -2984},
    7149: {X: 42727, Y: 50115, Z: -2984},
    7150: {X: 42798, Y: 50101, Z: -2984},
    7060: {X: 18072, Y: 144488, Z: -3040},
    7061: {X: 18044, Y: 144560, Z: -3040},
    7062: {X: 19320, Y: 146168, Z: -3064},
    7063: {X: 19224, Y: 146168, Z: -3064},
}

// worldScanRadii is the ring scan order: the heading push (64 units) rides
// first, then one cell band per step up to the interaction distance bound.
var worldScanRadii = []float64{48, 64, 80, 96, 112, 128, 144, 160,
    176, 192, 208, 224, 240}

// worldRegionCache lazily parses the touched geodata regions, one file per
// 32768 x 32768 square, shared across queries.
var worldRegionCache = map[string]*pathfind.Region{}

// worldFlatRegions records the flat placeholder regions of the pack (the
// 196608 byte files, every cell the flat zero layer): no geometry to derive
// from, the five Talking Island traders take the spawn fallback.
var worldFlatRegions = map[string]bool{}

// flatRegionBytes is the size of a flat placeholder region file.
const flatRegionBytes = 196608

// worldGeodata is the geodata directory of the running world pass.
var worldGeodata string

// worldRegionOf resolves (and caches) the region of a world position.
func worldRegionOf(x, y float64) *pathfind.Region {
    path := regionPath(worldGeodata, x, y)
    if region, ok := worldRegionCache[path]; ok {
        return region
    }
    worldFlatRegions[path] = regionFileSize(path) == flatRegionBytes
    region := loadRegion(path)
    worldRegionCache[path] = region

    return region
}

// regionFileSize returns the size of the geodata file (zero on error).
func regionFileSize(path string) int64 {
    info, err := os.Stat(path)
    if err != nil {
        return 0
    }

    return info.Size()
}

// loadRegion parses one geodata region file (nil on any failure).
func loadRegion(path string) *pathfind.Region {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil
    }
    region, err := pathfind.ParseRegionData(data, regionKeyOf(path))
    if err != nil {
        return nil
    }

    return region
}

// worldStand is the derived approach point of one merchant spawn.
type worldStand struct {
    id      int32
    name    string
    x, y, z float64
    // from names the derivation source ("spawn", "heading push",
    // "ring 96 SW"); ways counts the verified neighborhood directions.
    from string
    ways int
}

// verifyWorldStand routes the walk from the probed neighborhood starts to
// the candidate stand and checks every leg with the grid line of sight,
// answering the number of compass directions that verified, bounded at two -
// one proves the walk, the second names the redundant side. The cheap line of
// sight pre-gates every start: a failed exact search exhausts its whole
// connected component (the Herbiel freeze class the trip
// exactSegmentMaxDistance gate bounds for the same reason).
func verifyWorldStand(
    engine *pathfind.Engine, m merchant, x, y, z float64,
) int {
    if math.Hypot(x-m.x, y-m.y) > 250 {
        return 0
    }
    // The stand must sit on the merchant's own floor scale: a deck hundreds
    // of units away (a roof, the flat zero layer) is a foreign deck the
    // interaction distance can never serve.
    if math.Abs(z-m.z) > 96 {
        return 0
    }
    refZ := int16(m.z)
    stand := pathfind.Vec3{X: x, Y: y, Z: z}
    ways := 0
    for _, d := range dirs {
        for _, radius := range []float64{200, 400} {
            sx := m.x + d.dx*radius
            sy := m.y + d.dy*radius
            sz, err := engine.ClosestHeight(sx, sy, refZ)
            if err != nil || math.Abs(float64(sz)-m.z) > 96 {
                continue
            }
            start := pathfind.Vec3{X: sx, Y: sy, Z: float64(sz)}
            seen, serr := engine.LineOfSight(start, stand,
                pathfind.DefaultMaxPassableHeight)
            if serr != nil || !seen {
                continue
            }
            result, rerr := engine.FindPathApproach(start, stand,
                16, pathfind.DefaultMaxPassableHeight,
            )
            if rerr != nil || result == nil ||
                len(result.Waypoints) == 0 {
                continue
            }
            last := result.Waypoints[len(result.Waypoints)-1]
            if math.Hypot(last.X-x, last.Y-y) > 16 {
                continue
            }
            if !worldSegmentsVerified(engine, result) {
                continue
            }
            ways++
            if ways >= 2 {
                return ways
            }

            break
        }
    }

    return ways
}

// worldSegmentsVerified checks the grid line of sight of every route leg: the
// stand entry serves only a walk the grid walks too (the mesh link truth the
// grid walls off is the frozen corridor class).
func worldSegmentsVerified(
    engine *pathfind.Engine, result *pathfind.Result,
) bool {
    for i := 0; i+1 < len(result.Waypoints); i++ {
        a := result.Waypoints[i]
        b := result.Waypoints[i+1]
        ok, err := engine.LineOfSight(
            pathfind.Vec3{X: a.X, Y: a.Y, Z: a.Z},
            pathfind.Vec3{X: b.X, Y: b.Y, Z: b.Z},
            pathfind.DefaultMaxPassableHeight,
        )
        if err != nil || !ok {
            return false
        }
    }

    return true
}

// deriveWorldStand picks the customer cell of one merchant spawn: the spawn
// cell when the pack serves it, otherwise the verified push or ring candidate.
func deriveWorldStand(
    engine *pathfind.Engine, m merchant,
) (worldStand, bool) {
    refZ := int16(m.z)
    // A flat plane carries no customer cell: the spawn fallback serves the
    // trader and the pass reports it.
    region := worldRegionOf(m.x, m.y)
    if region != nil &&
        worldFlatRegions[regionPath(worldGeodata, m.x, m.y)] {
        return worldStand{
            id: m.id, name: m.name,
            x: m.x, y: m.y, z: m.z,
            from: "flat region", ways: 0,
        }, true
    }
    // The spawn cell itself: the pack may serve it directly.
    if region != nil && hasFloor(region, m.x, m.y, int(m.z)) {
        if z, err := engine.ClosestHeight(m.x, m.y, refZ); err == nil {
            if ways := verifyWorldStand(engine, m, m.x, m.y,
                float64(z)); ways > 0 {
                return worldStand{
                    id:   m.id,
                    name: m.name,
                    x:    m.x, y: m.y, z: float64(z),
                    from: "spawn", ways: ways,
                }, true
            }
        }
    }
    // The heading push: the counter sits on the facing side of the merchant,
    // the customer cell one band beyond it.
    angle := m.heading / 65536 * 2 * math.Pi
    px := m.x + 64*math.Cos(angle)
    py := m.y + 64*math.Sin(angle)
    if stand, ok := verifyWorldCandidate(engine, m, px, py,
        "heading push"); ok {
        return stand, true
    }
    // The ring scan, two stages: the facing sector first (the counter
    // front), then every direction - the interior shops whose heading sector
    // holds only roof decks still find the floor by the counter side.
    for _, sectorOnly := range []bool{true, false} {
        for _, radius := range worldScanRadii {
            best := worldStand{
                id: 0, name: "", x: 0, y: 0, z: 0, from: "", ways: 0,
            }
            found := false
            for _, d := range dirs {
                agree := math.Abs(math.Remainder(
                    math.Atan2(d.dy, d.dx)-angle, 2*math.Pi))
                if sectorOnly && agree > math.Pi/3 {
                    continue
                }
                cx := m.x + d.dx*radius
                cy := m.y + d.dy*radius
                stand, ok := verifyWorldCandidate(engine, m, cx, cy,
                    fmt.Sprintf("ring %.0f %s", radius, d.name))
                if ok && (!found || stand.ways > best.ways) {
                    best, found = stand, true
                }
            }
            if found {
                return best, true
            }
        }
    }

    return worldStand{
        id: 0, name: "", x: 0, y: 0, z: 0, from: "", ways: 0,
    }, false
}

// verifyWorldCandidate resolves the pack floor of one candidate cell and runs
// the route verification on it.
func verifyWorldCandidate(
    engine *pathfind.Engine, m merchant, x, y float64, from string,
) (worldStand, bool) {
    region := worldRegionOf(x, y)
    if region == nil || !hasFloor(region, x, y, int(m.z)) {
        return worldStand{
            id: 0, name: "", x: 0, y: 0, z: 0, from: "", ways: 0,
        }, false
    }
    z, err := engine.ClosestHeight(x, y, int16(m.z))
    if err != nil {
        return worldStand{
            id: 0, name: "", x: 0, y: 0, z: 0, from: "", ways: 0,
        }, false
    }
    ways := verifyWorldStand(engine, m, x, y, float64(z))
    if ways == 0 {
        return worldStand{
            id: 0, name: "", x: 0, y: 0, z: 0, from: "", ways: 0,
        }, false
    }

    return worldStand{
        id: m.id, name: m.name,
        x: x, y: y, z: float64(z),
        from: from, ways: ways,
    }, true
}

// runWorld derives and verifies the approach point of every world merchant
// spawn (npcdata.MerchantTemplateIDs order), cross-checks the curated rows and
// prints the verdict table; with emit the verified non curated rows print as
// the Go map source of the hunt package world table.
func runWorld(geodata string, emit bool) {
    worldGeodata = geodata
    engine := pathfind.NewEngine(geodata)
    spawnServed, derived, curatedOK, failed := 0, 0, 0, 0

    if emit {
        fmt.Println("// The world merchant stand rows the " +
            "counterprobe world pass verified. Code generated by " +
            "cmd/counterprobe -mode world-emit; regenerate with " +
            "go run ./cmd/counterprobe -geodata data/geodata " +
            "-mode world-emit.")
        fmt.Println("package hunt")
        fmt.Println()
        fmt.Println("import " +
            "\"github.com/melg8/swarm/internal/swarm/pathfind\"")
        fmt.Println()
        fmt.Println("var merchantStandsWorld = map[int32]" +
            "pathfind.Vec3{")
    }

    for _, id := range npcdata.MerchantTemplateIDs() {
        m, ok := worldMerchantRow(id)
        if !ok {
            continue
        }
        if _, ok := curatedStands[id]; ok {
            stand := curatedStands[id]
            ways := verifyWorldStand(engine, m,
                stand.X, stand.Y, float64(stand.Z))
            if ways > 0 {
                curatedOK++
            }
            worldReportf(emit,
                "%-18s (%d) curated stand %.0f %.0f: %s\n",
                m.name, id, stand.X, stand.Y,
                worldVerdict(ways))

            continue
        }
        stand, ok := deriveWorldStand(engine, m)
        switch {
        case !ok:
            failed++
            worldReportf(emit, "%-18s (%d) NO STAND DERIVED\n",
                m.name, id)
        case stand.from == "spawn", stand.from == "flat region":
            spawnServed++
            worldReportf(emit, "%-18s (%d) spawn serves (%s)\n",
                m.name, id, stand.from)
        default:
            derived++
            worldReportf(emit,
                "%-18s (%d) stand %.0f %.0f z %.0f "+
                    "(%s, %d ways, d2d %.0f)\n",
                m.name, id, stand.x, stand.y, stand.z,
                stand.from, stand.ways,
                math.Hypot(stand.x-m.x, stand.y-m.y))
            if emit {
                fmt.Printf("    %d: {X: %.0f, Y: %.0f, Z: %.0f}, "+
                    "// %s, %s, %d ways\n",
                    stand.id, stand.x, stand.y, stand.z,
                    stand.name, stand.from, stand.ways)
            }
        }
    }

    if emit {
        fmt.Println("}")
    }
    worldReportf(emit,
        "\nworld verdict: %d spawn served, %d stands derived, "+
            "%d curated ok, %d failed (of %d spawns)\n",
        spawnServed, derived, curatedOK, failed,
        npcdata.MerchantSpawnCount())
}

// worldReportf prints the per merchant verdict line to stdout; the emit mode
// routes it to stderr so the redirect captures the clean Go source only.
func worldReportf(emit bool, format string, args ...any) {
    if emit {
        fmt.Fprintf(os.Stderr, format, args...)

        return
    }
    fmt.Printf(format, args...)
}

// worldVerdict names the curated cross-check answer.
func worldVerdict(ways int) string {
    if ways > 0 {
        return fmt.Sprintf("verified (%d ways)", ways)
    }

    return "NOT VERIFIED"
}

// worldMerchantRow builds the probe merchant of one npcdata spawn row.
func worldMerchantRow(id int32) (merchant, bool) {
    spawn, ok := npcdata.MerchantSpawnOf(id)
    if !ok {
        return merchant{id: 0, name: "", x: 0, y: 0, z: 0, heading: 0},
            false
    }

    return merchant{
        id:      id,
        name:    spawn.Name,
        x:       float64(spawn.X),
        y:       float64(spawn.Y),
        z:       float64(spawn.Z),
        heading: float64(spawn.Heading),
    }, true
}
