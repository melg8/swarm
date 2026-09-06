// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWorldToCellRoundTrip checks that the cell of a world position
// contains the position and the center conversion keeps it inside.
func TestWorldToCellRoundTrip(t *testing.T) {
	positions := []Vec3{
		{X: 0, Y: 0},
		{X: 80364, Y: 147100},
		{X: -44000, Y: 120000},
		{X: 65536, Y: 131072},
		{X: 98303, Y: 163839},
		{X: -131072, Y: -262144},
		{X: -7.5, Y: 7.5},
	}
	for _, position := range positions {
		cell := WorldToCell(position.X, position.Y)
		corner := CellToWorldMin(cell)
		require.LessOrEqual(t, corner.X, position.X)
		require.Less(t, position.X, corner.X+cellSize)
		require.LessOrEqual(t, corner.Y, position.Y)
		require.Less(t, position.Y, corner.Y+cellSize)
		center := CellToWorldCenter(cell)
		require.InDelta(t, corner.X+cellSize/2, center.X, 0.001)
		require.InDelta(t, corner.Y+cellSize/2, center.Y, 0.001)
	}
}

// TestWorldToCellGiran checks the cell anchors against the Giran region
// of the original pathfinder example (region file 22_22.l2j): the world
// position (80364, 147100) lives in region 22_22, local cell (926,
// 1001).
func TestWorldToCellGiran(t *testing.T) {
	cell := WorldToCell(80364, 147100)
	require.Equal(t, RegionKey{Col: 22, Row: 22}, CellToRegion(cell))
	require.Equal(t, Point{X: 22*cellsPerRegionSide + 926,
		Y: 22*cellsPerRegionSide + 1001}, cell)
}

// TestCellToRegionNegative checks the region split for cells left or
// above the zero tile.
func TestCellToRegionNegative(t *testing.T) {
	cell := WorldToCell(-44000, 120000)
	// -44000 / 32768 floors to -2, so the region column is 18.
	require.Equal(t, RegionKey{Col: 18, Row: 21}, CellToRegion(cell))
	local := LocalCell(cell)
	require.GreaterOrEqual(t, local.X, int32(0))
	require.Less(t, local.X, int32(cellsPerRegionSide))
	require.GreaterOrEqual(t, local.Y, int32(0))
	require.Less(t, local.Y, int32(cellsPerRegionSide))
}

// TestFloorDivMod checks the floored division used by the region split.
func TestFloorDivMod(t *testing.T) {
	require.Equal(t, int32(2), floorDiv(5, 2))
	require.Equal(t, int32(-3), floorDiv(-5, 2))
	require.Equal(t, int32(-1), floorDiv(-2048, 2048))
	require.Equal(t, int32(-2), floorDiv(-2049, 2048))
	require.Equal(t, int32(0), floorMod(-2048, 2048))
	require.Equal(t, int32(2047), floorMod(-1, 2048))
}

// TestParseRegionFileName checks the region file name parsing.
func TestParseRegionFileName(t *testing.T) {
	col, row, ok := parseRegionFileName("22_22.l2j")
	require.True(t, ok)
	require.Equal(t, 22, col)
	require.Equal(t, 22, row)
	_, _, ok = parseRegionFileName("geo_index.txt")
	require.False(t, ok)
	_, _, ok = parseRegionFileName("22_22")
	require.False(t, ok)
	_, _, ok = parseRegionFileName("x_y.l2j")
	require.False(t, ok)
}
