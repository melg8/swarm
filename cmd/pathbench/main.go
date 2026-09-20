// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// pathbench is the Go half of the Go-vs-Rust pathfind comparison
// (benchmarks/pathfind-go-vs-rust): it binds a fixed set of world
// position pairs onto the mesh, runs the Route query over them and
// writes the timed answers as JSON.
//
// Modes:
//
//	go run ./cmd/pathbench -mode generate -mesh data/navmesh -out pairs.json
//	go run ./cmd/pathbench -mode warm -mesh data/navmesh -pairs pairs.json -rounds 50 -out go_results.json
//	go run ./cmd/pathbench -mode cold -mesh data/navmesh -pairs pairs.json -out go_cold.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// pairSpec is one benchmark query of the shared pair file.
type pairSpec struct {
	Name      string     `json:"name"`
	Start     [3]float64 `json:"start"`
	End       [3]float64 `json:"end"`
	Smooth    bool       `json:"smooth"`
	Clearance float64    `json:"clearance"`

	// Filled by the generator: the bound positions and the expected
	// answer both engines must reproduce.
	BoundStart [3]float64 `json:"boundStart"`
	BoundEnd   [3]float64 `json:"boundEnd"`
	Expected   struct {
		Found       bool    `json:"found"`
		Partial     bool    `json:"partial"`
		Hier        bool    `json:"hierarchical"`
		Corridor    int     `json:"corridor"`
		Waypoints   int     `json:"waypoints"`
		Length      float64 `json:"length"`
		Explored    int     `json:"explored"`
		Pocket      bool    `json:"pocketEscape"`
	} `json:"expected"`
}

// seedPair is one hardcoded candidate of the generator.
type seedPair struct {
	name      string
	start     [3]float64
	end       [3]float64
	smooth    bool
	clearance float64
}

// seedPairs carries the real world coordinates of the repo tests
// (the Elven village, the guard stairs round, the town scale walks)
// plus the region spread points of the built pack (21_18..22_20).
func seedPairs() []seedPair {
	elven := [3]float64{46890, 51531, -2976}
	eastGate := [3]float64{47872, 52736, -3800}
	guardStart := [3]float64{46880, 50752, -2889}
	guardEnd := [3]float64{47595, 51569, -2992}

	return []seedPair{
		{"short_village_eastgate", elven, eastGate, false, 0},
		{"medium_guard_stairs", guardStart, guardEnd, false, 0},
		{"within_21_19_a", [3]float64{45000, 48000, -3500}, [3]float64{52000, 56000, -3500}, false, 0},
		{"within_21_19_b", [3]float64{44000, 60000, -3000}, [3]float64{55000, 44000, -3000}, false, 0},
		{"within_21_19_smooth", elven, eastGate, true, 7.5},
		{"within_21_19_smooth_long", [3]float64{45000, 48000, -3500}, [3]float64{52000, 56000, -3500}, true, 7.5},
		{"cross_to_21_20", elven, [3]float64{50000, 70000, -4000}, false, 0},
		{"cross_to_22_19", elven, [3]float64{75000, 55000, -3500}, false, 0},
		{"cross_long_21_18_to_22_20", [3]float64{40000, 20000, -3000}, [3]float64{75000, 70000, -3500}, false, 0},
		{"cross_22_19_to_21_20", [3]float64{70000, 40000, -3500}, [3]float64{45000, 72000, -4000}, false, 0},
		{"cross_smooth_22_19", [3]float64{70000, 40000, -3500}, [3]float64{48000, 50000, -3500}, true, 7.5},
		{"within_22_19", [3]float64{68000, 38000, -3500}, [3]float64{76000, 50000, -3500}, false, 0},
		{"within_21_20", [3]float64{40000, 70000, -4000}, [3]float64{52000, 80000, -4000}, false, 0},
		{"far_diagonal_20_19_missing", [3]float64{10000, 45000, -3500}, [3]float64{55000, 50000, -3500}, false, 0},
	}
}

// bindPos resolves a world position onto the mesh: the vertical probe
// walks the height window in overlapping bands and an xy jitter loop
// covers the positions inside a wall footprint (the same ladder the
// hier_bench_test uses).
func bindPos(mesh *navmesh.Mesh, pos [3]float64) (navmesh.Pos, bool) {
	for z := -12000.0; z <= 12000.0; z += 800.0 {
		probe := navmesh.Pos{X: pos[0], Y: pos[1], Z: z}
		if ref, snapped, ok := mesh.FindNearestPoly(probe); ok && ref != 0 {
			return snapped, true
		}
	}
	for radius := 512.0; radius <= 4096.0; radius *= 2 {
		for _, d := range [][2]float64{{1, 0}, {-1, 0}, {0, 1}, {0, -1},
			{0.7, 0.7}, {-0.7, 0.7}, {0.7, -0.7}, {-0.7, -0.7}} {
			probe := navmesh.Pos{
				X: pos[0] + radius*d[0],
				Y: pos[1] + radius*d[1],
				Z: pos[2],
			}
			if ref, snapped, ok := mesh.FindNearestPoly(probe); ok && ref != 0 {
				return snapped, true
			}
		}
	}

	return navmesh.Pos{}, false
}

func filterOf(spec pairSpec) navmesh.Filter {
	filter := navmesh.DefaultFilter()
	filter.Smooth = spec.Smooth
	filter.WaypointClearance = spec.Clearance

	return filter
}

func runGenerate(meshDir, outPath string) error {
	mesh := navmesh.NewMesh(meshDir)
	mesh.SetCacheCapacity(8)
	specs := make([]pairSpec, 0)
	for _, seed := range seedPairs() {
		start, ok := bindPos(mesh, seed.start)
		if !ok {
			fmt.Printf("skip (start unbindable): %s\n", seed.name)

			continue
		}
		end, ok := bindPos(mesh, seed.end)
		if !ok {
			fmt.Printf("skip (end unbindable): %s\n", seed.name)

			continue
		}
		spec := pairSpec{
			Name:       seed.name,
			Start:      seed.start,
			End:        seed.end,
			Smooth:     seed.smooth,
			Clearance:  seed.clearance,
			BoundStart: [3]float64{start.X, start.Y, start.Z},
			BoundEnd:   [3]float64{end.X, end.Y, end.Z},
		}
		route, err := mesh.Route(start, end, filterOf(spec))
		if err != nil {
			fmt.Printf("skip (route error): %s: %v\n", seed.name, err)

			continue
		}
		spec.Expected.Found = route.Found
		spec.Expected.Partial = route.Partial
		spec.Expected.Hier = route.Hierarchical
		spec.Expected.Corridor = len(route.Corridor)
		spec.Expected.Waypoints = len(route.Waypoints)
		spec.Expected.Length = routeLength(route.Waypoints)
		spec.Expected.Explored = route.Explored
		spec.Expected.Pocket = route.PocketEscape
		specs = append(specs, spec)
		fmt.Printf("%s: found=%t partial=%t hier=%t corridor=%d wps=%d length=%.0f explored=%d\n",
			spec.Name, route.Found, route.Partial, route.Hierarchical,
			len(route.Corridor), len(route.Waypoints), spec.Expected.Length,
			route.Explored)
	}
	data, err := json.MarshalIndent(specs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outPath, data, 0o644)
}

// routeLength sums the waypoint walk length.
func routeLength(wps []navmesh.Pos) float64 {
	sum := 0.0
	for i := 1; i < len(wps); i++ {
		sum += dist3(wps[i-1], wps[i])
	}

	return sum
}

func dist3(a, b navmesh.Pos) float64 {
	dx, dy, dz := a.X-b.X, a.Y-b.Y, a.Z-b.Z

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// pairResult is one timed pair of the benchmark answer.
type pairResult struct {
	Name      string  `json:"name"`
	Found     bool    `json:"found"`
	Partial   bool    `json:"partial"`
	Hier      bool    `json:"hierarchical"`
	Corridor  int     `json:"corridor"`
	Waypoints int     `json:"waypoints"`
	Length    float64 `json:"length"`
	Explored  int     `json:"explored"`
	RoundsN   int     `json:"rounds"`
	TotalNs   int64   `json:"totalNs"`
	MinNs     int64   `json:"minNs"`
	MedianNs  int64   `json:"medianNs"`
	MaxNs     int64   `json:"maxNs"`
	MismatchN int     `json:"mismatches"`
	FirstNote string  `json:"firstNote"`

	Medians []int64 `json:"-"`
}

// benchOutput is the JSON file the comparison reads.
type benchOutput struct {
	Engine       string       `json:"engine"`
	Mode         string       `json:"mode"`
	Rounds       int          `json:"rounds"`
	Pairs        []pairResult `json:"pairs"`
	TotalTimedNs int64        `json:"totalTimedNs"`
	PeakRSSKB    int64        `json:"peakRssKb"`
	TotalAllocB  uint64       `json:"totalAllocBytes"`
	Mallocs      uint64       `json:"mallocs"`
	VerifyErrors int          `json:"verifyErrors"`
	WarmupNs     int64        `json:"warmupNs"`
}

func runWarm(meshDir, pairsPath string, rounds int, outPath string) error {
	specs := loadPairs(pairsPath)
	mesh := navmesh.NewMesh(meshDir)
	mesh.SetCacheCapacity(8)
	output := benchOutput{Engine: "go", Mode: "warm", Rounds: rounds}

	// The warmup pass: tiles, abstracts and hop caches fill here and
	// the verify baseline pins.
	warmupBegan := time.Now()
	for i := range specs {
		spec := &specs[i]
		route, err := mesh.Route(posOf(spec.BoundStart), posOf(spec.BoundEnd), filterOf(*spec))
		if err != nil {
			spec.Expected.Corridor = -1 // the pair answers an error: mark
			output.Pairs = append(output.Pairs, pairResult{
				Name: spec.Name, FirstNote: fmt.Sprintf("error: %v", err),
			})

			continue
		}
		verify(spec, route, &output)
	}
	output.WarmupNs = time.Since(warmupBegan).Nanoseconds()

	// The timed rounds.
	results := make(map[string]*pairResult, len(specs))
	for i := range specs {
		results[specs[i].Name] = &pairResult{Name: specs[i].Name, MinNs: math.MaxInt64}
	}
	for round := 0; round < rounds; round++ {
		for i := range specs {
			spec := &specs[i]
			if spec.Expected.Corridor < 0 {
				continue // the error pair
			}
			began := time.Now()
			route, err := mesh.Route(posOf(spec.BoundStart), posOf(spec.BoundEnd), filterOf(*spec))
			elapsed := time.Since(began)
			res := results[spec.Name]
			if err != nil {
				res.MismatchN++

				continue
			}
			ns := elapsed.Nanoseconds()
			res.RoundsN++
			res.TotalNs += ns
			if ns < res.MinNs {
				res.MinNs = ns
			}
			if ns > res.MaxNs {
				res.MaxNs = ns
			}
			res.Medians = append(res.Medians, ns)
			if route.Found != spec.Expected.Found || route.Partial != spec.Expected.Partial ||
				len(route.Corridor) != spec.Expected.Corridor ||
				len(route.Waypoints) != spec.Expected.Waypoints {
				res.MismatchN++
				output.VerifyErrors++
			}
		}
	}
	var totalTimed int64
	for i := range specs {
		spec := &specs[i]
		res := results[spec.Name]
		if res.RoundsN == 0 {
			continue
		}
		sort.Slice(res.Medians, func(a, b int) bool { return res.Medians[a] < res.Medians[b] })
		res.MedianNs = res.Medians[len(res.Medians)/2]
		res.Found = spec.Expected.Found
		res.Partial = spec.Expected.Partial
		res.Hier = spec.Expected.Hier
		res.Corridor = spec.Expected.Corridor
		res.Waypoints = spec.Expected.Waypoints
		res.Length = spec.Expected.Length
		res.Explored = spec.Expected.Explored
		totalTimed += res.TotalNs
		output.Pairs = append(output.Pairs, *res)
	}
	output.TotalTimedNs = totalTimed
	output.PeakRSSKB = peakRSSKB()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	output.TotalAllocB = ms.TotalAlloc
	output.Mallocs = ms.Mallocs

	return writeJSON(outPath, output)
}

func runCold(meshDir, pairsPath, outPath string) error {
	specs := loadPairs(pairsPath)
	output := benchOutput{Engine: "go", Mode: "cold", Rounds: 1}
	var total int64
	for i := range specs {
		spec := &specs[i]
		// A fresh mesh per pair: the cold path pays every decode and
		// every abstract build.
		mesh := navmesh.NewMesh(meshDir)
		mesh.SetCacheCapacity(8)
		began := time.Now()
		_, err := mesh.Route(posOf(spec.BoundStart), posOf(spec.BoundEnd), filterOf(*spec))
		elapsed := time.Since(began)
		note := ""
		if err != nil {
			note = fmt.Sprintf("error: %v", err)
		}
		output.Pairs = append(output.Pairs, pairResult{
			Name: spec.Name, TotalNs: elapsed.Nanoseconds(), MinNs: elapsed.Nanoseconds(),
			MedianNs: elapsed.Nanoseconds(), MaxNs: elapsed.Nanoseconds(), RoundsN: 1,
			FirstNote: note,
		})
		total += elapsed.Nanoseconds()
	}
	output.TotalTimedNs = total
	output.PeakRSSKB = peakRSSKB()

	return writeJSON(outPath, output)
}

func loadPairs(path string) []pairSpec {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read pairs:", err)
		os.Exit(1)
	}
	var specs []pairSpec
	if err := json.Unmarshal(data, &specs); err != nil {
		fmt.Fprintln(os.Stderr, "parse pairs:", err)
		os.Exit(1)
	}

	return specs
}

func verify(spec *pairSpec, route *navmesh.Route, output *benchOutput) {
	if route.Found != spec.Expected.Found || route.Partial != spec.Expected.Partial ||
		len(route.Corridor) != spec.Expected.Corridor ||
		len(route.Waypoints) != spec.Expected.Waypoints {
		output.VerifyErrors++
		fmt.Printf("VERIFY MISMATCH %s: got found=%t partial=%t corridor=%d wps=%d, want found=%t partial=%t corridor=%d wps=%d\n",
			spec.Name, route.Found, route.Partial, len(route.Corridor),
			len(route.Waypoints), spec.Expected.Found, spec.Expected.Partial,
			spec.Expected.Corridor, spec.Expected.Waypoints)
	}
}

func posOf(v [3]float64) navmesh.Pos {
	return navmesh.Pos{X: v[0], Y: v[1], Z: v[2]}
}

func peakRSSKB() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	lines := string(data)
	idx := indexString(lines, "VmHWM:")
	if idx < 0 {
		return 0
	}
	rest := lines[idx+6:]
	end := 0
	for end < len(rest) && rest[end] != '\n' {
		end++
	}
	var kb int64
	_, _ = fmt.Sscanf(rest[:end], "%d", &kb)

	return kb
}

func indexString(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func main() {
	meshDir := flag.String("mesh", "data/navmesh", "the navmesh tile directory")
	mode := flag.String("mode", "warm", "generate | warm | cold")
	pairsPath := flag.String("pairs", "benchmarks/pathfind-go-vs-rust/pairs.json", "the shared pair file")
	rounds := flag.Int("rounds", 50, "the timed rounds over the pair set")
	outPath := flag.String("out", "go_results.json", "the output JSON path")
	flag.Parse()

	var err error
	switch *mode {
	case "generate":
		err = runGenerate(*meshDir, *outPath)
	case "warm":
		err = runWarm(*meshDir, *pairsPath, *rounds, *outPath)
	case "cold":
		err = runCold(*meshDir, *pairsPath, *outPath)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
