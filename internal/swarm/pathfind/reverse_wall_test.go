// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The reverse wall check of wallsOpen prevents the search from
// planning steps onto cells whose reverse wall is closed. The Mobius
// MoveToLocation handler rejects any move whose target cell is
// completely blocked (isCompletelyBlocked of GeoEngine), and the
// movement validation checks the target cell's wall in the approach
// direction. Without the reverse check the search planned steps onto
// cells the server refused to enter - the character stood still, the
// stuck timer fired and the deterministic re-path planned the
// identical route (the 2026-09-11 town walk stuck loop).

func TestWallsOpenRejectsTargetWithClosedReverseWall(t *testing.T) {
	// Source cell: north wall open. Target cell (to the north): south
	// wall CLOSED. The reverse check must reject the step.
	from := &node{
		key:    nodeKey{p: Point{X: 0, Y: 0}, h: -1000},
		coords: Point{X: 0, Y: 0},
		layer:  Layer{Height: -1000, NSWE: nsweNorth},
	}
	to := &node{
		key:    nodeKey{p: Point{X: 0, Y: -1}, h: -1000},
		coords: Point{X: 0, Y: -1},
		layer:  Layer{Height: -1000, NSWE: nsweEast}, // south wall closed
	}
	s := newSearch(nil, DefaultMaxPassableHeight)
	require.False(t, s.wallsOpen(from, to),
		"the step to a cell whose reverse wall is closed must be rejected")
}

func TestWallsOpenAcceptsTargetWithOpenReverseWall(t *testing.T) {
	// Source cell: north wall open. Target cell (to the north): south
	// wall OPEN. The reverse check must accept the step.
	from := &node{
		key:    nodeKey{p: Point{X: 0, Y: 0}, h: -1000},
		coords: Point{X: 0, Y: 0},
		layer:  Layer{Height: -1000, NSWE: nsweNorth},
	}
	to := &node{
		key:    nodeKey{p: Point{X: 0, Y: -1}, h: -1000},
		coords: Point{X: 0, Y: -1},
		layer:  Layer{Height: -1000, NSWE: nsweSouth},
	}
	s := newSearch(nil, DefaultMaxPassableHeight)
	require.True(t, s.wallsOpen(from, to),
		"the step to a cell whose reverse wall is open must be accepted")
}

func TestWallsOpenRejectsCompletelyBlockedTarget(t *testing.T) {
	// Source cell: all walls open. Target cell: completely blocked
	// (NSWE = 0). The reverse check must reject the step regardless
	// of direction.
	from := &node{
		key:    nodeKey{p: Point{X: 0, Y: 0}, h: -1000},
		coords: Point{X: 0, Y: 0},
		layer:  Layer{Height: -1000, NSWE: nsweAll},
	}
	to := &node{
		key:    nodeKey{p: Point{X: 1, Y: 0}, h: -1000},
		coords: Point{X: 1, Y: 0},
		layer:  Layer{Height: -1000, NSWE: 0}, // completely blocked
	}
	s := newSearch(nil, DefaultMaxPassableHeight)
	require.False(t, s.wallsOpen(from, to),
		"the step to a completely blocked cell must be rejected")
}

func TestLayerIsCompletelyBlocked(t *testing.T) {
	require.True(t, Layer{NSWE: 0}.IsCompletelyBlocked())
	require.False(t, Layer{NSWE: nsweNorth}.IsCompletelyBlocked())
	require.False(t, Layer{NSWE: nsweAll}.IsCompletelyBlocked())
}

// The dry path from the dump stuck position (the elven village teacher
// plaza at 44440 52552 -2832) to the trader Herbiel (42766 50037
// -2984) must be found and must not cross water. The reverse wall fix
// changed the planned route: the old route detoured through the plaza
// deck at z -2792 (wp2 44424 51704 -2792) before dropping back, the
// new route stays on the z -2832 deck and descends directly. Both
// routes are dry; the new route is shorter and avoids the plaza detour
// that triggered the stuck loop.
func TestDryPathFromStuckSpotToHerbielFound(t *testing.T) {
	engine := stuckSpotEngine(t)
	from := Vec3{X: 44440, Y: 52552, Z: -2832}
	herbiel := Vec3{X: 42766, Y: 50037, Z: -2984}
	result, err := engine.FindPathApproachDry(
		from, herbiel, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Found, "the dry path to Herbiel must be found")
	require.Greater(t, len(result.Waypoints), 1,
		"the plan must be a real geodata route")
	for _, wp := range result.Waypoints {
		require.Greater(t, wp.Z, -3780.0,
			"every waypoint must stand above the water surface")
	}
}

// The dry path from the stuck spot to the teacher Cobendell (44823
// 52414 -2792) must be found: the teacher stop of the learning trip
// walks there. The path is short (the character stands near the
// teacher plaza) and dry.
func TestDryPathFromStuckSpotToCobendellFound(t *testing.T) {
	engine := stuckSpotEngine(t)
	from := Vec3{X: 44440, Y: 52552, Z: -2832}
	cobendell := Vec3{X: 44823, Y: 52414, Z: -2792}
	result, err := engine.FindPathApproachDry(
		from, cobendell, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Found, "the dry path to Cobendell must be found")
	for _, wp := range result.Waypoints {
		require.Greater(t, wp.Z, -3780.0,
			"every waypoint must stand above the water surface")
	}
}

// stuckSpotEngine loads the real geodata pack for the stuck spot
// reproduction tests, skipping the test when the pack is not available.
func stuckSpotEngine(t *testing.T) *Engine {
	t.Helper()
	candidates := []string{
		"data/geodata",
	}
	if dir, err := os.Getwd(); err == nil {
		for range 6 {
			candidates = append(candidates, filepath.Join(dir, "data", "geodata"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			engine := NewEngine(c)
			if engine.Stats().HasData {
				return engine
			}
		}
	}
	t.Skip("no local geodata pack, the stuck spot reproduction needs it")

	return nil
}
