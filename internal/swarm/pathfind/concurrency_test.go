// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEngineConcurrentSearchesRaceFree pins the concurrent use of one
// Engine by several bots: region parsing interns into the shared layer
// pool while the searches of the already loaded regions read through it,
// which raced before the pool guarded itself (see
// docs/development_log.md, 2026-09-08). The cache is deliberately
// smaller than the region count, so every search evicts and re-parses
// regions while the others search - under `-race` (task test:race, the
// detector needs cgo) this exercises the intern/get pair hard.
func TestEngineConcurrentSearchesRaceFree(t *testing.T) {
	dir := t.TempDir()

	// Four flat open regions in a 2x2 grid; the paths cross the region
	// borders, so one search touches two regions.
	regionCoords := [][2]int{{22, 22}, {23, 22}, {22, 23}, {23, 23}}
	for _, coords := range regionCoords {
		spec := &regionSpec{}
		spec.setFlat(0)
		data := spec.encode()
		name := filepath.Join(dir, regionFileName(coords[0], coords[1]))
		require.NoError(t, os.WriteFile(name, data, 0o600))
	}

	engine := NewEngine(dir)
	engine.capacity = 2

	// pointIn maps a region and local cell offsets to a world point on
	// its flat surface. The endpoints sit near the shared border of the
	// 2x2 grid, so one search is short while still touching two regions
	// (and thrashing the two-slot cache, which re-parses constantly).
	pointIn := func(coords [2]int, dx, dy int) Vec3 {
		return Vec3{
			X: float64((coords[0]-tileZeroCol)*tileSize + dx*cellSize),
			Y: float64((coords[1]-tileZeroRow)*tileSize + dy*cellSize),
			Z: 0,
		}
	}
	border := cellsPerRegionSide - 4

	var wg sync.WaitGroup
	// Four searches per goroutine keep the test fast: every search
	// re-parses two regions through the two-slot cache (a parse
	// allocates the ~17 MB span array, so hundreds of them would turn
	// the test into a minute long allocation benchmark), while the
	// parallel goroutines keep an intern/get window open on every one.
	for i := range 4 {
		from, to := regionCoords[i%2], regionCoords[i%2+1]
		wg.Add(1)
		go func(from, to [2]int, shift int) {
			defer wg.Done()
			fromWorld := pointIn(from, border, shift)
			toWorld := pointIn(to, 2, shift)
			for range 4 {
				result, err := engine.FindPath(
					fromWorld, toWorld, DefaultMaxPassableHeight)
				if err != nil {
					t.Errorf("concurrent search failed: %v", err)

					return
				}
				if result == nil || !result.Found {
					t.Errorf("path %v -> %v not found on flat ground",
						from, to)

					return
				}
			}
		}(from, to, 8+i*4)
	}
	wg.Wait()
}
