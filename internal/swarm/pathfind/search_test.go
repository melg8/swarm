// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
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
func closedWalls() Layer {
	return Layer{Height: 0, NSWE: 0}
}

// wallOnlyLayer keeps only the north and south walls open, closing the
// east and west sides: the vertical wall columns of the scenarios.
func wallOnlyLayer() Layer {
	return Layer{Height: 0, NSWE: nsweNorth | nsweSouth}
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
	require.Positive(t, int64(result.Duration))
}

// TestFindPathAroundWall builds a full height wall with a single gap and
// checks that the path crosses the wall inside the gap and that every
// smoothed leg has line of sight.
func TestFindPathAroundWall(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	wallX := 300
	gapY := 1000
	for ly := range cellsPerRegionSide {
		if ly >= gapY && ly < gapY+8 {
			continue
		}
		spec.setCell(wallX, ly, wallOnlyLayer())
		spec.setCell(wallX+1, ly, wallOnlyLayer())
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
		cleared, err := engine.LineOfSight(
			result.Waypoints[i], result.Waypoints[i+1],
			DefaultMaxPassableHeight)
		require.NoError(t, err)
		require.True(t, cleared, "leg %d of the path is blocked", i)
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
	for ly := range cellsPerRegionSide {
		spec.setCell(300, ly, wallOnlyLayer())
		spec.setCell(301, ly, wallOnlyLayer())
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(100, 1000, 0), worldOf(500, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, result.Found)
	require.Empty(t, result.Waypoints)
	require.Positive(t, result.Explored)
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
			spec.setCell(targetX+dx, targetY+dy, closedWalls())
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
	for bx := range blocksPerRegionSide {
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
	spec.setCell(300, 1000, wallOnlyLayer())
	spec.setCell(301, 1000, wallOnlyLayer())
	engine := newTestEngine(t, spec)

	cleared, err := engine.LineOfSight(
		worldOf(200, 1000, 0), worldOf(400, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, cleared)

	cleared, err = engine.LineOfSight(
		worldOf(200, 1000, 0), worldOf(280, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, cleared)

	cleared, err = engine.LineOfSight(
		worldOf(300, 1000, 0), worldOf(300, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, cleared)
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
		spec.setCell(x, 1000, Layer{
			Height: int16((i + 1) * 16),
			NSWE:   nsweAll,
		})
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
	spec.setCell(300, 700, wallOnlyLayer())
	spec.setCell(300, 701, wallOnlyLayer())
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
	require.ErrorIs(t, err, ErrMissingCell, err)

	// With no files at all even the start fails.
	empty := NewEngine(t.TempDir())
	_, err = empty.FindPath(
		worldOf(100, 100, 0), worldOf(500, 500, 0),
		DefaultMaxPassableHeight)
	require.ErrorIs(t, err, ErrMissingCell, err)
}

// TestFindPathApproachRadius checks the approach goal of the town
// trips: a target whose cell cannot be entered (the merchant behind
// the counter boards) is still reached by the walk - the search ends
// on the first cell within the approach radius, at the counter front.
func TestFindPathApproachRadius(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	// The counter: the target cell and its whole ring are sealed.
	targetX, targetY := 1000, 1000
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			spec.setCell(targetX+dx, targetY+dy, closedWalls())
		}
	}
	engine := newTestEngine(t, spec)
	target := worldOf(targetX, targetY, 0)

	// The plain search cannot reach the sealed cell at all.
	plain, err := engine.FindPath(
		worldOf(200, 200, 0), target, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, plain.Found)

	// The approach search stops at the counter front: within the
	// radius, on an open cell.
	result, err := engine.FindPathApproach(
		worldOf(200, 200, 0), target, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)
	end := result.Waypoints[len(result.Waypoints)-1]
	dist := math.Sqrt(
		(end.X-target.X)*(end.X-target.X) +
			(end.Y-target.Y)*(end.Y-target.Y) +
			(end.Z-target.Z)*(end.Z-target.Z))
	require.LessOrEqual(t, dist, 200.0)
	require.Greater(t, dist, 40.0, "the walk stops in front, not inside")
}

// TestFindPathApproachPrefersExactTarget checks that the approach
// radius never shortens a walk that can reach the exact target: over
// open ground the arrival is the target cell, not a radius shortcut.
func TestFindPathApproachPrefersExactTarget(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	engine := newTestEngine(t, spec)
	target := worldOf(900, 900, 0)

	result, err := engine.FindPathApproach(
		worldOf(100, 100, 0), target, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)
	end := result.Waypoints[len(result.Waypoints)-1]
	require.InDelta(t, target.X, end.X, cellSize)
	require.InDelta(t, target.Y, end.Y, cellSize)
}

// waterChannelSpec builds the elven lake shape: the land at -3000 on
// both sides of a water channel whose bed sits at -4000 (below the
// -3780 C1 water surface), a climbable 40 unit step slope on the south
// shore and an optional bridge deck column crossing the channel at
// land height. The start and the goal sit on opposite shores. The
// blocks of the channel band are built whole because setCell zeroes
// the untouched cells of a block.
func waterChannelSpec(bridge bool) (*regionSpec, Vec3, Vec3) {
	const land = int16(-3000)
	const bed = int16(-4000)
	spec := &regionSpec{}
	spec.setFlat(land)
	// The channel band covers the local cells y 400..703 (the block
	// rows 50..87): the sharp north edge, the bed, the south slope.
	for by := 50; by <= 87; by++ {
		for bx := range blocksPerRegionSide {
			if bridge && bx >= 50 && bx <= 52 {
				// The bridge column blocks: multilayer with the deck.
				block := blockSpec{
					kind:  blockMultilayer,
					cells: [cellsPerBlock]Layer{},
				}
				for cy := range cellsPerBlockSide {
					ly := by*cellsPerBlockSide + cy
					for cx := range cellsPerBlockSide {
						lx := bx*cellsPerBlockSide + cx
						base := cellHeight(ly, land, bed)
						block.cells[cx*cellsPerBlockSide+cy] = Layer{
							Height: base, NSWE: nsweAll,
						}
						if lx >= 400 && lx <= 420 {
							block.stacks[cx*cellsPerBlockSide+cy] = []Layer{
								{Height: land, NSWE: nsweAll},
								{Height: base, NSWE: nsweAll},
							}
						}
					}
				}
				spec.blocks[bx][by] = block

				continue
			}
			block := blockSpec{kind: blockComplex, cells: [cellsPerBlock]Layer{}}
			for cy := range cellsPerBlockSide {
				ly := by*cellsPerBlockSide + cy
				for cx := range cellsPerBlockSide {
					block.cells[cx*cellsPerBlockSide+cy] = Layer{
						Height: cellHeight(ly, land, bed), NSWE: nsweAll,
					}
				}
			}
			spec.blocks[bx][by] = block
		}
	}
	// The start and end sit 100 cells west of the bridge column: the
	// bridge detour stays longer than the straight swim line (the
	// assertion below) while the water cost of the 260 cell bed
	// crossing beats the detour cost decisively - the margins of the
	// old lx 100 fixture were inside the diagonal shortcut noise and
	// the route flipped between the swim and the bridge.
	start := worldOf(300, 300, land)
	end := worldOf(300, 800, land)

	return spec, start, end
}

// cellHeight returns the surface height of a channel band cell: the
// bed in the middle, 40 unit slopes on BOTH shores (the real geodata
// lake shores descend gradually into the water - the measured elven
// lake entries step 8..24 units per cell) and the land outside. The
// north shore must slope too: a cliff there would seal the channel
// for a surface-staying search, and only the drop planning of the
// old asymmetric rule could enter the water over it.
func cellHeight(ly int, land, bed int16) int16 {
	switch {
	case ly <= 424:
		return int16(-3000 + (ly-400)*(-40))
	case ly <= 675:
		return bed
	case ly <= 700:
		return int16(-4000 + (ly-675)*40)
	default:
		return land
	}
}

// TestFindPathWaterCost checks the water cost dimension: a water
// channel (the geodata floor below the C1 water surface) is walkable
// but costs more per step than land, so the route prefers the longer
// bridge over the shorter swim - the elven village town trip shape.
func TestFindPathWaterCost(t *testing.T) {
	spec, start, end := waterChannelSpec(true)
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(start, end, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)
	// The route never swims: every waypoint stays above the C1 water
	// surface (the slope cells above the bed are fair land).
	for _, wp := range result.Waypoints {
		require.Greater(t, wp.Z, -3780.0,
			"the route must cross the water above its surface")
	}
	// The bridge route is longer than the direct swim line.
	direct := math.Hypot(end.X-start.X, end.Y-start.Y)
	require.Greater(t, result.Length, direct,
		"the bridge detour is longer than the straight swim")
	// The route actually uses the bridge column.
	lowX, highX := worldOf(399, 0, 0).X, worldOf(421, 0, 0).X
	onBridge := false
	for _, wp := range result.Waypoints {
		if wp.X >= lowX && wp.X <= highX {
			onBridge = true
		}
	}
	require.True(t, onBridge, "the route must cross the bridge column")
}

// TestFindPathWaterOnlyRouteStillSwims checks the water cost is a
// preference, not a wall: with no bridge at all the route still
// crosses the water bed (the server accepts swimming).
func TestFindPathWaterOnlyRouteStillSwims(t *testing.T) {
	spec, start, end := waterChannelSpec(false)
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(start, end, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found, "water remains walkable at a cost")
}

// TestFindPathTerraceBoundaryNeedsARamp pins the surface rule of the
// search: a height jump of hundreds of units between neighbouring
// cells is a terrace boundary, not a walkable connection. The walk
// from the upper terrace to the lower one must use a gradual ramp -
// the elven city bridges between the floating deck and the ground
// step 8..16 units per cell - and with no ramp at all the terraces
// stay sealed for the search (the character cannot leave the deck
// over the railing the geodata does not model).
func TestFindPathTerraceBoundaryNeedsARamp(t *testing.T) {
	// A high strip without a ramp: the drop at its end seals it.
	spec := &regionSpec{}
	spec.setFlat(0)
	for lx := range 400 {
		spec.setCell(lx, 1000, Layer{Height: 500, NSWE: nsweAll})
	}
	engine := newTestEngine(t, spec)

	result, err := engine.FindPath(
		worldOf(100, 1000, 500), worldOf(900, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, result.Found,
		"a 500 unit drop is a terrace boundary, not a walkable step")

	// The same strip with a gradual ramp at its east end connects
	// the terraces: the descent plans and every raw step stays
	// within the passable height.
	spec = &regionSpec{}
	spec.setFlat(0)
	for lx := range 400 {
		spec.setCell(lx, 1000, Layer{Height: 500, NSWE: nsweAll})
	}
	// The ramp: 30 cells descending 16 units each, then a 36 unit
	// step onto the ground - every step within the 40 limit.
	for lx := 370; lx < 400; lx++ {
		spec.setCell(lx, 1000, Layer{
			Height: int16(500 - (lx-370)*16), NSWE: nsweAll,
		})
	}
	engine = newTestEngine(t, spec)

	result, err = engine.FindPath(
		worldOf(100, 1000, 500), worldOf(900, 1000, 0),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found, "the ramp connects the terraces")
	require.NotEmpty(t, result.RawPath)
	for i := 1; i < len(result.RawPath); i++ {
		step := math.Abs(result.RawPath[i].Z - result.RawPath[i-1].Z)
		require.LessOrEqual(t, step, float64(DefaultMaxPassableHeight),
			"raw path step %d drops %.0f units - a terrace boundary",
			i, step)
	}

	// The reverse walk climbs the ramp back onto the strip.
	back, err := engine.FindPath(
		worldOf(900, 1000, 0), worldOf(100, 1000, 500),
		DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, back.Found, "the ramp climbs back up")
}

// TestFindPathStepUpForty checks the climb gate matches the Mobius
// HEIGHT_INCREASE_LIMIT of 40: a 40 unit step is walkable, a 48 unit
// step is not.
func TestFindPathStepUpForty(t *testing.T) {
	for _, tc := range []struct {
		step   int16
		expect bool
	}{
		{step: 40, expect: true},
		{step: 48, expect: false},
	} {
		spec := &regionSpec{}
		spec.setFlat(0)
		for lx := 400; lx < cellsPerRegionSide; lx++ {
			spec.setCell(lx, 1000, Layer{Height: tc.step, NSWE: nsweAll})
		}
		engine := newTestEngine(t, spec)
		result, err := engine.FindPath(
			worldOf(100, 1000, 0), worldOf(900, 1000, tc.step),
			DefaultMaxPassableHeight)
		require.NoError(t, err)
		require.Equal(t, tc.expect, result.Found,
			"a %d unit climb must be found=%t", tc.step, tc.expect)
	}
}
