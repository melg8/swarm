// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"os"
	"path/filepath"
	"testing"
)

// benchOpenField builds a flat region engine for the benchmarks.
func benchOpenField() *Engine {
	spec := &regionSpec{}
	spec.setFlat(0)

	return NewEngine(benchDirWithSpec(spec))
}

// benchMaze builds a comb maze: vertical wall teeth with alternating
// gaps force a long zigzag path.
func benchMaze() *Engine {
	spec := &regionSpec{}
	spec.setFlat(0)
	for wall := 8; wall < 800; wall += 8 {
		gapStart := (wall / 8) % 2 * 1000
		for ly := 0; ly < cellsPerRegionSide; ly++ {
			if ly >= gapStart && ly < gapStart+1000 {
				continue
			}
			spec.setCell(wall, ly, wallOnlyLayer(0))
		}
	}

	return NewEngine(benchDirWithSpec(spec))
}

// giranEngine is the engine over the real Giran region testdata.
func giranEngine() *Engine {
	return NewEngine("testdata")
}

// BenchmarkFindPathOpenField measures a 400 cell diagonal over a flat
// plane (the line of sight fast path).
func BenchmarkFindPathOpenField(b *testing.B) {
	engine := benchOpenField()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := engine.FindPath(
			worldOf(100, 100, 0), worldOf(500, 400, 0),
			DefaultMaxPassableHeight)
		if err != nil || !result.Found {
			b.Fatal(err, result)
		}
	}
}

// BenchmarkFindPathWallMaze measures a long zigzag path around nine wall
// teeth with the smoothing overhead.
func BenchmarkFindPathWallMaze(b *testing.B) {
	engine := benchMaze()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := engine.FindPath(
			worldOf(100, 500, 0), worldOf(900, 1500, 0),
			DefaultMaxPassableHeight)
		if err != nil || !result.Found {
			b.Fatal(err, result)
		}
	}
}

// BenchmarkFindPathGiran measures the original example path across
// Giran (near the weapon shop to the north bridge) over the real
// geodata region 22_22 with the example max passable height 20.
func BenchmarkFindPathGiran(b *testing.B) {
	engine := giranEngine()
	start := Vec3{X: 80364, Y: 147100, Z: -3533}
	end := Vec3{X: 83864, Y: 143100}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := engine.FindPath(start, end, 20)
		if err != nil || !result.Found {
			b.Fatal(err, result)
		}
	}
}

// BenchmarkLineOfSightGiran measures a line of sight check over the real
// region.
func BenchmarkLineOfSightGiran(b *testing.B) {
	engine := giranEngine()
	start := Vec3{X: 80364, Y: 147100, Z: -3533}
	end := Vec3{X: 83864, Y: 143100}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clear, err := engine.LineOfSight(start, end, 20)
		if err != nil {
			b.Fatal(err)
		}
		if clear {
			b.Fatal("expected the city blocks to block the sight")
		}
	}
}

// benchDirWithSpec writes the spec into a shared benchmark temp dir.
func benchDirWithSpec(spec *regionSpec) string {
	dir, err := os.MkdirTemp("", "swarm-pathfind-bench")
	if err != nil {
		panic(err)
	}
	name := filepath.Join(dir, regionFileName(testRegionCol, testRegionRow))
	if err := os.WriteFile(name, spec.encode(), 0o644); err != nil {
		panic(err)
	}

	return dir
}

// BenchmarkParseRegionGiran measures the one time cost of parsing the
// real Giran region into the compact in memory layout.
func BenchmarkParseRegionGiran(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("testdata", "22_22.l2j"))
	if err != nil {
		b.Skip("no testdata")
	}
	key := RegionKey{Col: 22, Row: 22}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool := newLayerPool()
		_, err := parseRegion(data, key, pool)
		if err != nil {
			b.Fatal(err)
		}
	}
}
