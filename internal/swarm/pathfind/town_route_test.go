// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// townGeodataCandidates mirrors the bot mode geodata detection of
// cmd/swarm: the in-repository pack first (go test runs with the
// package directory as CWD, so the search walks up to the repository
// root where data/geodata lives), then the reference Windows
// deployment of this project.
func townGeodataCandidates() []string {
	candidates := make([]string, 0, 8)
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
	candidates = append(candidates, filepath.Join("E:\\", "work",
		"lineage_workspace_fresh", "L2J_Mobius_C1_HarbingersOfWar",
		"game", "data", "geodata"))

	return candidates
}

// townTestEngine builds an engine over the real geodata pack when it is
// available on the machine and skips the test otherwise: the town route
// below spans several regions and only exists in the deployed pack.
func townTestEngine(t *testing.T) *Engine {
	t.Helper()
	for _, candidate := range townGeodataCandidates() {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			engine := NewEngine(candidate)
			require.True(t, engine.Stats().HasData,
				"the geodata directory %s must contain region files",
				candidate)

			return engine
		}
	}
	t.Skip("no local geodata pack, the town route test needs it")
	t.Helper()

	return nil
}

// The elven village navigation fixtures of the town trips: the trader
// Unoren of the floating island, the lake shore hunting grounds and
// the stuck position the user reported (the village deck edge at the
// pond west of the shop quarter).
var (
	unorenSpawn = Vec3{X: 44667, Y: 46896, Z: -2982}
	shoreFarm   = Vec3{X: 47320, Y: 42216, Z: -3488}
	stuckSpot   = Vec3{X: 45544, Y: 45880, Z: -2992}
)

// TestFindPathToShopDeck verifies the approach search of the town
// trips against the real elven village: the route to the trader
// Unoren must end within the interaction radius of the merchant,
// standing on the village deck (z near -2992), never on the water
// deck below the shop (z -3928) the plain search used to arrive on.
// The shop cells hold no floor layer at the real merchant z (the C1
// geodata models only a raised surface and the water below), so the
// interaction distance is the only correct goal.
func TestFindPathToShopDeck(t *testing.T) {
	engine := townTestEngine(t)

	for _, start := range []Vec3{shoreFarm, stuckSpot} {
		toShop, err := engine.FindPathApproach(
			start, unorenSpawn, 200, DefaultMaxPassableHeight)
		require.NoError(t, err)
		require.True(t, toShop.Found,
			"the shop approach from %.0f %.0f must be reachable", start.X, start.Y)

		end := toShop.Waypoints[len(toShop.Waypoints)-1]
		dist := math.Sqrt(
			(end.X-unorenSpawn.X)*(end.X-unorenSpawn.X) +
				(end.Y-unorenSpawn.Y)*(end.Y-unorenSpawn.Y) +
				(end.Z-unorenSpawn.Z)*(end.Z-unorenSpawn.Z))
		t.Logf("from %.0f %.0f: %d waypoints, len %.0f, end %.0f %.0f %.0f "+
			"(dist to merchant %.0f, explored %d)",
			start.X, start.Y, len(toShop.Waypoints), toShop.Length,
			end.X, end.Y, end.Z, dist, toShop.Explored)
		require.LessOrEqual(t, dist, 200.0,
			"the approach must end within the interaction distance")
		require.Greater(t, end.Z, -3400.0,
			"the approach must stay on the village deck, not the water")
		// The bridge route wins over swimming: the whole route stays
		// above the water surface except the shore start.
		deep := 0
		for _, wp := range toShop.Waypoints {
			if wp.Z < -3780 {
				deep++
			}
		}
		require.Zero(t, deep, "the route must not swim under the village")
	}
}

// TestFindPathToShopPlainWaterArrival documents the plain search
// behavior the trips no longer rely on: with any-layer arrival the
// cheapest route to the multilayer shop cell ends on the water deck
// below the village (the swim), which is exactly why the approach
// search replaced it.
func TestFindPathToShopPlainWaterArrival(t *testing.T) {
	engine := townTestEngine(t)

	plain, err := engine.FindPath(shoreFarm, unorenSpawn, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, plain.Found, "the shop cell must be reachable on some deck")
	end := plain.Waypoints[len(plain.Waypoints)-1]
	require.Less(t, end.Z, -3800.0,
		"the plain search ends on the water deck below the shop")
}

// TestFindPathGludioToDionCorridor pins the T-018 finding: the
// Gludio -> Dion walking corridor of the 20-25 band survey exists
// on the real geodata pack within the shipped expansion cap (the
// measured route: 42 831 units, 25 waypoints, 0.94M nodes, ~9 s).
// A geodata refresh or a search regression that breaks the mainland
// connectivity of the band trips fails here instead of surfacing as
// a walk that never arrives. The elven -> Gludio leg (1.25M nodes,
// 25 percent above the shipped cap) stays the documentation fact of
// docs/navigation_analysis.md - the cap-as-a-parameter roadmap item
// owns it.
func TestFindPathGludioToDionCorridor(t *testing.T) {
	engine := townTestEngine(t)

	gludio := Vec3{X: -12694, Y: 122776, Z: -3114}
	dion := Vec3{X: 15671, Y: 142994, Z: -2704}
	result, err := engine.FindPathApproach(
		gludio, dion, 150, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found,
		"the Gludio -> Dion corridor must stay connected")
	require.False(t, result.Aborted,
		"the corridor sits within the shipped expansion cap")
	require.Greater(t, result.Length, 35000.0,
		"the corridor route spans the two towns")
	require.NotEmpty(t, result.Waypoints)
	end := result.Waypoints[len(result.Waypoints)-1]
	require.LessOrEqual(t,
		math.Hypot(float64(end.X-dion.X), float64(end.Y-dion.Y)),
		300.0, "the route ends at the Dion town square")
}
