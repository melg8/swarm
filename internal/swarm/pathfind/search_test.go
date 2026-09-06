// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestEngine writes the region spec into a temp directory and builds
// an engine over it.
func newTestEngine(t *testing.T, spec *regionSpec) *Engine {
	t.Helper()
	dir := t.TempDir()
	spec.writeRegion(t, dir)

	return NewEngine(dir)
}

// closedWalls is the layer of an impassable cell (every wall closed).
func closedWalls(height int16) Layer {
	return Layer{Height: height, NSWE: 0}
}

// wallOnlyLayer keeps only the north and south walls open, closing the
// east and west sides: the vertical wall columns of the scenarios.
func wallOnlyLayer(height int16) Layer {
	return Layer{Height: height, NSWE: nsweNorth | nsweSouth}
}

// TestFindPathOpenField checks the direct line of sight path over a flat
// plane: two waypoints, the geometric distance of the ends.
func TestFindPathOpenField(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(100, 100, 0), worldOf(500, 400, 0), DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)
	require.Len(t, result.Waypoints, 2)
	require.InDelta(t, 8000, result.Length, cellSize*2)
	require.NotEmpty(t, result.RawPath)
	require.Greater(t, int64(result.Duration), int64(0))
}

// TestFindPathAroundWall builds a full height wall with a single gap and
// checks that the path crosses the wall inside the gap and that every
// smoothed leg has line of sight.
func TestFindPathAroundWall(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	wallX := 300
	gapY := 1000
	for ly := 0; ly < cellsPerRegionSide; ly++ {
		if ly >= gapY && ly < gapY+8 {
			continue
		}
		spec.setCell(wallX, ly, wallOnlyLayer(0))
		spec.setCell(wallX+1, ly, wallOnlyLayer(0))
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(100, 500, 0), worldOf(500, 500, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)
	require.Greater(t, len(result.Waypoints), 2)

	// Every smoothed leg must be walkable by construction.
	for i := 0; i+1 < len(result.Waypoints); i++ {
		clear, err := engine.LineOfSight(
			result.Waypoints[i], result.Waypoints[i+1],
			DefaultMaxPassableHeight)
		require.NoError(t, err)
		require.True(t, clear, "leg %d of the path is blocked", i)
	}

	// The raw cell path crosses the wall line inside the gap rows.
	wallWorldX := float64((testRegionCol-tileZeroCol)*tileSize + wallX*cellSize)
	gapWorldY := float64((testRegionRow-tileZeroRow)*tileSize + gapY*cellSize)
	crossed := false
	for _, point := range result.RawPath {
		if point.X >= wallWorldX && point.X <= wallWorldX+2*cellSize &&
			point.Y >= gapWorldY && point.Y <= gapWorldY+8*cellSize {
			crossed = true
		}
	}
	require.True(t, crossed, "the path never crosses the gap")

	// The detour costs more than the straight line but stays sane.
	straight := math.Hypot(400*cellSize, 0)
	require.Greater(t, result.Length, straight)
	require.Less(t, result.Length, straight*3)
}

// TestFindPathWallWithoutGap checks that a solid wall makes the target
// unreachable without an error.
func TestFindPathWallWithoutGap(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	for ly := 0; ly < cellsPerRegionSide; ly++ {
		spec.setCell(300, ly, wallOnlyLayer(0))
		spec.setCell(301, ly, wallOnlyLayer(0))
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(100, 1000, 0), worldOf(500, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, result.Found)
	require.Empty(t, result.Waypoints)
	require.Greater(t, result.Explored, 0)
}

// TestFindPathEnclosedTarget checks the fully enclosed target case: the
// search exhausts the reachable area and reports not found.
func TestFindPathEnclosedTarget(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	targetX, targetY := 1000, 1000
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			spec.setCell(targetX+dx, targetY+dy, closedWalls(0))
		}
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(200, 200, 0), worldOf(targetX, targetY, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, result.Found)
}

// TestFindPathHeightCliff checks the max passable height gate: a step
// higher than the limit blocks the path, a larger limit restores it.
func TestFindPathHeightCliff(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	// Blocks from block row 62 on (cell 496) sit 200 units higher.
	for bx := 0; bx < blocksPerRegionSide; bx++ {
		for by := 62; by < blocksPerRegionSide; by++ {
			spec.blocks[bx][by] = blockSpec{
				kind:  blockFlat,
				cells: [cellsPerBlock]Layer{{Height: 200, NSWE: nsweAll}},
			}
		}
	}
	engine := newTestEngine(t, spec)

	blocked, err := engine.FindPath(
		worldOf(300, 400, 0), worldOf(300, 600, 0), 30)
	require.NoError(t, err)
	require.False(t, blocked.Found)

	result, err := engine.FindPath(
		worldOf(300, 400, 0), worldOf(300, 600, 0), 256)
	require.NoError(t, err)
	require.True(t, result.Found)
}

// TestFindPathUpperDeck checks the multilayer floor separation: a deck
// 500 units above the ground is only walkable when the search height
// matches the deck.
func TestFindPathUpperDeck(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	// A small upper deck: every deck cell has a ground and a roof layer.
	for lx := 250; lx <= 260; lx++ {
		for ly := 250; ly <= 260; ly++ {
			spec.setMultilayer(lx, ly, []Layer{
				{Height: 0, NSWE: nsweAll},
				{Height: 504, NSWE: nsweAll},
			})
		}
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(252, 252, 504), worldOf(258, 258, 504),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)
	for _, waypoint := range result.Waypoints {
		require.InDelta(t, 504, waypoint.Z, 0.001)
	}

	// From the ground the same target resolves to the ground layer.
	ground, err := engine.FindPath(
		worldOf(252, 252, 0), worldOf(258, 258, 0), DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, ground.Found)
	for _, waypoint := range ground.Waypoints {
		require.InDelta(t, 0, waypoint.Z, 0.001)
	}
}

// TestLineOfSight checks the wall and height gates of the line of sight.
func TestLineOfSight(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	spec.setCell(300, 1000, wallOnlyLayer(0))
	spec.setCell(301, 1000, wallOnlyLayer(0))
	engine := newTestEngine(t, spec)

	clear, err := engine.LineOfSight(
		worldOf(200, 1000, 0), worldOf(400, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, clear)

	clear, err = engine.LineOfSight(
		worldOf(200, 1000, 0), worldOf(280, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, clear)

	clear, err = engine.LineOfSight(
		worldOf(300, 1000, 0), worldOf(300, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, clear)
}

// TestFindPathBridgeOverWater is the regression test of the layer
// poisoning: a bridge deck rides over walkable water as a second layer.
// While the search floods the water along the bridge, every deck cell it
// touches from the water must not lose its deck layer - the walker
// climbing the ramp afterwards needs it. The old single node per cell
// cache resolved the layer on first touch and cut the bridge off.
func TestFindPathBridgeOverWater(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	// The ramp climbs from the water shore to the deck height in steps
	// the default max passable height allows. It stays inside one block
	// so the builder does not overwrite it with the deck block.
	for i, x := range []int{100, 101, 102, 103} {
		spec.setCell(x, 1000, Layer{Height: int16((i + 1) * 16),
			NSWE: nsweAll})
	}
	// The deck and the island ride one layer above the water.
	for x := 104; x <= 440; x++ {
		for y := 999; y <= 1001; y++ {
			spec.setMultilayer(x, y, []Layer{
				{Height: 0, NSWE: nsweAll},
				{Height: 64, NSWE: nsweAll},
			})
		}
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(95, 1000, 0), worldOf(430, 1000, 64), 30)
	require.NoError(t, err)
	require.True(t, result.Found)
	require.Greater(t, len(result.Waypoints), 2)
	// The arrival stands on the deck, the start on the water shore.
	last := result.Waypoints[len(result.Waypoints)-1]
	require.InDelta(t, 64, last.Z, 0.001)
}

// TestFindPathDeterministic checks that identical searches return
// identical paths (the UI redraws while the user drags).
func TestFindPathDeterministic(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	spec.setCell(300, 700, wallOnlyLayer(0))
	spec.setCell(300, 701, wallOnlyLayer(0))
	engine := newTestEngine(t, spec)

	first, err := engine.FindPath(
		worldOf(100, 500, 0), worldOf(500, 900, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	second, err := engine.FindPath(
		worldOf(100, 500, 0), worldOf(500, 900, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.Equal(t, first.Waypoints, second.Waypoints)
}

// TestFindPathMissingGeodata checks the hard error when the start or the
// target has no geodata underneath.
func TestFindPathMissingGeodata(t *testing.T) {
	dir := t.TempDir()
	spec := &regionSpec{}
	spec.setFlat(0)
	// Only the 22_22 file exists: the 23_22 area east of it is void.
	spec.writeRegion(t, dir)
	engine := NewEngine(dir)

	// The target sits in the 23_22 region which has no file.
	_, err := engine.FindPath(
		worldOf(100, 100, 0), Vec3{X: 100000, Y: 140000},
		DefaultMaxPassableHeight)
	require.True(t, errors.Is(err, ErrMissingCell), err)

	// With no files at all even the start fails.
	empty := NewEngine(t.TempDir())
	_, err = empty.FindPath(
		worldOf(100, 100, 0), worldOf(500, 500, 0),
		DefaultMaxPassableHeight)
	require.True(t, errors.Is(err, ErrMissingCell), err)
}
