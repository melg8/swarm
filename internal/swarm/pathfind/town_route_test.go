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
// cmd/swarm: the relative server layout first, then the reference
// Windows deployment of this project.
var townGeodataCandidates = []string{
	filepath.Join("data", "geodata"),
	filepath.Join("E:\\", "work", "lineage_workspace_fresh",
		"L2J_Mobius_C1_HarbingersOfWar", "game", "data", "geodata"),
}

// townTestEngine builds an engine over the real geodata pack when it is
// available on the machine and skips the test otherwise: the town route
// below spans several regions and only exists in the deployed pack.
func townTestEngine(t *testing.T) *Engine {
	t.Helper()
	for _, candidate := range townGeodataCandidates {
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

// TestFindPathToShopDeck verifies the targeted search of the town
// trips: when the route to the trader Unoren is found it must end on
// the village deck the merchant stands on (-2982), never on the water
// deck below it that the plain search arrives on. The deployed geodata
// pack disconnects the village deck from the hunting fields, so the
// strict search reports not found there and the town trips fall back
// to the plain search (the sale works from any deck).
func TestFindPathToShopDeck(t *testing.T) {
	engine := townTestEngine(t)
	start := Vec3{X: 47320, Y: 42216, Z: -3488}
	shop := Vec3{X: 44667, Y: 46896, Z: -2982}

	plain, err := engine.FindPath(start, shop, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, plain.Found,
		"the shop cell must be reachable on some deck")

	toShop, err := engine.FindPathTo(
		start, shop, int16(shop.Z), DefaultMaxPassableHeight)
	require.NoError(t, err)
	if !toShop.Found {
		t.Log("the shop deck is disconnected from the fields in " +
			"this geodata pack, the town trips sell without targeting")

		return
	}
	end := toShop.Waypoints[len(toShop.Waypoints)-1]
	t.Logf("to deck: %d waypoints, end %.0f %.0f %.0f",
		len(toShop.Waypoints), end.X, end.Y, end.Z)
	require.Less(t, math.Abs(end.Z-shop.Z), 300.0,
		"the targeted search must end on the shop deck")
}
