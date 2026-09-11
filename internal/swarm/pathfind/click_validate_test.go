// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The 2026-09-10 town walk stuck regression: the bot clicked a
// waypoint 58 units over the village plaza corner, the server click
// validation (GeoEngine.getValidLocation) refused the corner cut
// diagonal and collapsed the destination onto the walker - the
// character froze re-clicking the same waypoint through all three
// re-paths. These tests pin the ported rules on synthetic terrain.

// plazaCornerRegion builds the plaza corner of the stuck report: a
// walkable plane at z 0 whose south row cells (y 101) wall their west
// edge, the diagonal SW step over the corner must be refused while
// the cardinal steps pass.
func plazaCornerRegion(t *testing.T) *Engine {
	t.Helper()
	spec := &regionSpec{}
	spec.setFlat(0)
	// The block of the corner converts to complex cells (the flat
	// blocks carry no per cell walls): every cell stays open except
	// the corner itself, whose west edge the terrace walls off.
	for x := 96; x < 104; x++ {
		for y := 96; y < 104; y++ {
			spec.setCell(x, y, Layer{Height: 0, NSWE: nsweAll})
		}
	}
	// The south neighbour of the start cell walls its west edge: the
	// SW diagonal cut crosses it.
	spec.setCell(100, 101, Layer{Height: 0, NSWE: nsweNorth | nsweSouth | nsweEast})
	engine := engineOverSpec(t, spec)
	require.True(t, engine.Stats().HasData, "the synthetic region must load")

	return engine
}

// TestValidateClickRefusesCornerCutDiagonal pins the anti corner cut
// rule of the port: a click whose Bresenham line cuts the walled
// corner collapses onto the walker (the server cancels the move,
// distance zero), while the cardinal clicks of the same corner walk.
func TestValidateClickRefusesCornerCutDiagonal(t *testing.T) {
	engine := plazaCornerRegion(t)
	// From the cell (100, 100) center: the diagonal SW click to the
	// cell (99, 101) crosses the walled corner.
	from := worldOf(100, 100, 0)
	diag := worldOf(99, 101, 0)
	_, ok := engine.ValidateClick(from, diag)
	require.False(t, ok, "the corner cutting click must be refused")
	// The cardinal clicks of the same geometry walk: south then west
	// decomposes the diagonal into legal steps.
	south := worldOf(100, 101, 0)
	_, ok = engine.ValidateClick(from, south)
	require.True(t, ok, "the south cardinal click must run")
	west := worldOf(99, 100, 0)
	_, ok = engine.ValidateClick(from, west)
	require.True(t, ok, "the west cardinal click must run")
}

// TestSearchWallsOpenRefusesCornerCutDiagonal pins the search side of
// the fix: the A* step rule applies the same anti corner cut to its
// diagonal expansions, so no planned route may rely on a corner the
// server click validation refuses.
func TestSearchWallsOpenRefusesCornerCutDiagonal(t *testing.T) {
	engine := plazaCornerRegion(t)
	search := newSearch(engine, DefaultMaxPassableHeight)
	from := search.node(WorldToCell(worldOf(100, 100, 0).X, worldOf(100, 100, 0).Y), 0)
	diag := search.node(WorldToCell(worldOf(99, 101, 0).X, worldOf(99, 101, 0).Y), 0)
	require.NotNil(t, from)
	require.NotNil(t, diag)
	require.False(t, search.wallsOpen(from, diag),
		"the diagonal step over the walled corner must be refused")
	south := search.node(WorldToCell(worldOf(100, 101, 0).X, worldOf(100, 101, 0).Y), 0)
	west := search.node(WorldToCell(worldOf(99, 100, 0).X, worldOf(99, 100, 0).Y), 0)
	require.True(t, search.wallsOpen(from, south), "the cardinal south step passes")
	require.True(t, search.wallsOpen(from, west), "the cardinal west step passes")
}

// TestValidateClickFarTargetSkipsValidation pins the far click rule of
// the server: a mouse click beyond 3000 units skips the geodata
// correction entirely (the far click moves as clicked), so the port
// answers it as accepted without walking the line.
func TestValidateClickFarTargetSkipsValidation(t *testing.T) {
	engine := plazaCornerRegion(t)
	from := worldOf(100, 100, 0)
	// A far click over the walled corner: the distance rule protects
	// it from the correction.
	far := worldOf(400, 500, 0)
	target, ok := engine.ValidateClick(from, far)
	require.True(t, ok, "the far click skips the geodata validation")
	require.InDelta(t, far.X, target.X, 1, "the far click moves as clicked")
	require.InDelta(t, far.Y, target.Y, 1, "the far click moves as clicked")
}

// TestValidateClickFinalLayerCollapse pins the final layer rule of the
// server port: a click whose line arrives at the target cell on a
// different layer than the one the click z names collapses back onto
// the walker - the second collapse mechanism of the stuck report
// (after the corner cut refused the first step).
func TestValidateClickFinalLayerCollapse(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	// The target cell holds two decks: the ground at 0 and a bridge
	// deck far above. A click at the deck height arrives on the ground
	// layer the line walked: the layer mismatch collapses the click.
	spec.setMultilayer(120, 120, []Layer{
		{Height: 0, NSWE: nsweAll},
		{Height: 600, NSWE: nsweAll},
	})
	engine := engineOverSpec(t, spec)
	from := worldOf(100, 100, 0)
	// Click the deck: the z 600 names the upper layer, the line from
	// the flat plane arrives on the layer closest to its running z 0.
	deck := worldOf(120, 120, 600)
	_, ok := engine.ValidateClick(from, deck)
	require.False(t, ok, "the layer mismatch must collapse the click")
	// Clicking the ground under the deck runs: the line arrives on the
	// layer the click z names.
	ground := worldOf(120, 120, 0)
	_, ok = engine.ValidateClick(from, ground)
	require.True(t, ok, "the matching layer click must run")
}

// engineOverSpec writes the spec into a temp directory and builds the
// engine over it.
func engineOverSpec(t *testing.T, spec *regionSpec) *Engine {
	t.Helper()
	dir := t.TempDir()
	spec.writeRegion(t, dir)

	return NewEngine(dir)
}
