// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package pathfind finds walkable paths over L2j geodata files for long
// distance movement. The algorithm is a Go port of L2jGeodataPathFinder
// (https://github.com/k0t9i/L2jGeodataPathFinder), the pathfinder used by
// L2Bot2.0: an A* search over the geodata cell grid with wall (NSWE) and
// height checks, a supercover line rasterization for line of sight and a
// string pulling post smoothing that collapses the raw cell path into
// turning points. The geodata file format follows the Mobius C1 game
// server (GeoEngine.java): headerless little endian region files named
// X_Y.l2j.
//
// The package is self contained: it only reads .l2j files from disk and
// never talks to the game server, so it works without a running bot and
// can later serve as a movement service or a hunt helper library.
package pathfind

import "math"

// Geodata geometry constants. They mirror both the Mobius C1 sources
// (World.TILE_SIZE, World.TILE_ZERO_COORD_X/Y, IRegion.REGION_CELLS_X)
// and the Constants.h of the original L2jGeodataPathFinder.
const (
	// cellsPerBlockSide is the cell count of one geodata block side.
	cellsPerBlockSide = 8
	// cellsPerBlock is the cell count of one geodata block (64).
	cellsPerBlock = cellsPerBlockSide * cellsPerBlockSide
	// blocksPerRegionSide is the block count of one region side.
	blocksPerRegionSide = 256
	// cellsPerRegionSide is the cell count of one region side (2048).
	cellsPerRegionSide = cellsPerBlockSide * blocksPerRegionSide
	// blocksPerRegion is the block count of a whole region (65536).
	blocksPerRegion = blocksPerRegionSide * blocksPerRegionSide
	// cellsPerRegion is the cell count of a whole region (4194304).
	cellsPerRegion = cellsPerRegionSide * cellsPerRegionSide
	// tileSize is the world size of one region (World.TILE_SIZE).
	tileSize = 32768
	// tileZeroCol/Row are the World.TILE_ZERO_COORD anchors: the region
	// file name X_Y.l2j uses X = floor(x / tileSize) + tileZeroCol.
	tileZeroCol = 20
	tileZeroRow = 18
	// cellSize is the world size of one geodata cell (16 units).
	cellSize = tileSize / cellsPerRegionSide
)

// Point is a global geodata cell coordinate. The mapping to world space
// is fixed by the region anchors: cell X = floor(x / tileSize) *
// cellsPerRegionSide + tileZeroCol * cellsPerRegionSide ... in short
// floor((x / tileSize + tileZeroCol) * cellsPerRegionSide).
type Point struct {
	X, Y int32
}

// Vec3 is a world position in game units.
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// RegionKey identifies a geodata region file by its tile coordinates
// (the X and Y of the X_Y.l2j file name).
type RegionKey struct {
	Col, Row int16
}

// WorldToCell converts a world position to the global geodata cell
// coordinate that contains it.
func WorldToCell(x, y float64) Point {
	return Point{
		X: int32(math.Floor((x/tileSize + tileZeroCol) * cellsPerRegionSide)),
		Y: int32(math.Floor((y/tileSize + tileZeroRow) * cellsPerRegionSide)),
	}
}

// CellToWorldMin returns the world position of the lower corner of the
// cell (the original pathfinder exposes this corner as the node box).
func CellToWorldMin(p Point) Vec3 {
	return Vec3{
		X: float64((p.X - tileZeroCol*cellsPerRegionSide) * cellSize),
		Y: float64((p.Y - tileZeroRow*cellsPerRegionSide) * cellSize),
	}
}

// CellToWorldCenter returns the world center of the cell. Path waypoints
// use the center: the server accepts any point inside the cell, and the
// center is the deterministic choice the walker can follow.
func CellToWorldCenter(p Point) Vec3 {
	corner := CellToWorldMin(p)

	return Vec3{X: corner.X + cellSize/2, Y: corner.Y + cellSize/2}
}

// CellToRegion returns the region file coordinates of a global cell.
func CellToRegion(p Point) RegionKey {
	return RegionKey{
		Col: int16(floorDiv(p.X, cellsPerRegionSide)),
		Row: int16(floorDiv(p.Y, cellsPerRegionSide)),
	}
}

// LocalCell converts a global cell to the cell index inside its region
// (0..cellsPerRegionSide-1 per axis).
func LocalCell(p Point) Point {
	return Point{
		X: floorMod(p.X, cellsPerRegionSide),
		Y: floorMod(p.Y, cellsPerRegionSide),
	}
}

// floorDiv is the floored integer division (Go "/" truncates toward
// zero, negative cells would round the wrong way).
func floorDiv(v, d int32) int32 {
	q := v / d
	if (v%d != 0) && ((v < 0) != (d < 0)) {
		q--
	}

	return q
}

// floorMod is the floored integer remainder, always in [0, d).
func floorMod(v, d int32) int32 {
	return v - floorDiv(v, d)*d
}
