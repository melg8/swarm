// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// The round 56 acceptance: the bot must reliably reach the teacher
// Ellenia (45725 52105 -2792) from every walking position of the
// elven village - the trainer hall aisle the dump froze in, the
// pocket the failed recovery crept into, the terraces around it and
// the shop quarter. The trainer hall building carries three geodata
// layers (the sloped roof -2600..-2448, the walkable floor -2792 with
// the walls in the NSWE flags, the water deck -3928) and its interior
// east of the aisle has no floor layer at all, so a route may only
// enter through the west aisle column or around the building - the
// tests pin that every plan the search builds for Ellenia consists of
// legs the server click validation accepts in full (the round 52
// port), from every approach direction.
var elleniaSpawn = Vec3{X: 45725, Y: 52105, Z: -2792}

// elleniaApproaches are the village positions the walk to Ellenia
// starts from: the dump aisle entrance and the trap pocket east of it,
// the north terrace and the south approach around the trainer hall,
// the east plaza the current planner routes through and the two shop
// quarter spots.
var elleniaApproaches = []struct {
	name string
	pos  Vec3
}{
	{"the dump aisle entrance", Vec3{X: 44728, Y: 51992, Z: -2792}},
	{"the trap pocket", Vec3{X: 44776, Y: 51992, Z: -2808}},
	{"the north terrace bend", Vec3{X: 44568, Y: 51528, Z: -2808}},
	{"the south approach", Vec3{X: 44568, Y: 52536, Z: -2832}},
	{"the east plaza", Vec3{X: 46104, Y: 51528, Z: -2808}},
	{"the shop deck", Vec3{X: 44872, Y: 47160, Z: -2992}},
	{"the southwest shore path", Vec3{X: 43752, Y: 48024, Z: -2992}},
}

// TestElleniaReachableFromEveryVillageApproach walks the dry approach
// search from every village position to Ellenia and verifies the two
// invariants the follower needs: the route ends within the trip
// approach radius of the teacher, and EVERY leg of the plan is a
// click the server would accept in full (the validated destination is
// the clicked waypoint itself) - a plan of fully validated legs can
// never arm the collapsed partial click that crept the dump character
// into the dead-end pocket.
func TestElleniaReachableFromEveryVillageApproach(t *testing.T) {
	engine := townTestEngine(t)

	for _, approach := range elleniaApproaches {
		t.Run(approach.name, func(t *testing.T) {
			result, err := engine.FindPathApproachDry(
				approach.pos, elleniaSpawn, 200, DefaultMaxPassableHeight)
			require.NoError(t, err)
			require.True(t, result.Found,
				"the dry route to Ellenia must exist from %s", approach.name)

			last := result.Waypoints[len(result.Waypoints)-1]
			require.LessOrEqual(t, dist3D(last, elleniaSpawn), 200.0,
				"the route must end inside the Ellenia approach ring")

			for i := 1; i < len(result.Waypoints); i++ {
				from := result.Waypoints[i-1]
				to := result.Waypoints[i]
				validated, ok := engine.ValidateClick(from, to)
				require.True(t, ok,
					"the leg %d -> %d must survive the server validation",
					i-1, i)
				require.InDelta(t, to.X, validated.X, 1.0,
					"the leg %d must validate in full (no partial collapse)", i)
				require.InDelta(t, to.Y, validated.Y, 1.0,
					"the leg %d must validate in full (no partial collapse)", i)
			}
		})
	}
}

// TestElleniaAisleRouteMatchesTheDumpPlan pins the exact dump walk
// from the aisle entrance: the route must enter the building through
// the aisle column south (the 48 unit leg the dump carried as wp 2)
// and cross the south hall east - the same plan the 06:19 state dump
// shows, now verified as fully server-valid.
func TestElleniaAisleRouteMatchesTheDumpPlan(t *testing.T) {
	engine := townTestEngine(t)

	result, err := engine.FindPathApproachDry(
		elleniaApproaches[0].pos, elleniaSpawn, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found)

	// The dump plan waypoints (44728 51992 -> 44728 52040 ->
	// 45160 52120 -> 45528 52104): every one must appear in the route.
	for _, want := range []Vec3{
		{X: 44728, Y: 52040, Z: -2792},
		{X: 45160, Y: 52120, Z: -2792},
		{X: 45528, Y: 52104, Z: -2792},
	} {
		found := false
		for _, wp := range result.Waypoints {
			if dist3D(wp, want) < 64 {
				found = true

				break
			}
		}
		require.True(t, found,
			"the route must pass the dump waypoint %.0f %.0f", want.X, want.Y)
	}
}

// TestElleniaPocketLinesRefuseTheHallClick pins the trap itself: the
// straight click from the pocket cell (44776 51992 -2808) toward the
// east hall waypoint (45160 52120) is refused by the ported server
// rules (the pocket's east wall is closed) while the click from the
// aisle entrance (44728 51992) runs only PARTIALLY - it collapses
// onto the first step east. These are the two answers the round 56
// skip gate protects the follower from arming.
func TestElleniaPocketLinesRefuseTheHallClick(t *testing.T) {
	engine := townTestEngine(t)

	hall := Vec3{X: 45160, Y: 52120, Z: -2792}
	pocket := Vec3{X: 44776, Y: 51992, Z: -2808}
	aisle := Vec3{X: 44728, Y: 51992, Z: -2792}

	// From the pocket: the first Bresenham step east hits the closed
	// east wall, the click collapses onto the walker - a refusal.
	_, ok := engine.ValidateClick(pocket, hall)
	require.False(t, ok,
		"the hall click from the pocket must be refused (the closed east wall)")

	// From the aisle entrance: the click runs but stops at the first
	// step - the partial that crept the dump character east.
	validated, ok := engine.ValidateClick(aisle, hall)
	require.True(t, ok, "the hall click from the aisle runs partially")
	require.Less(t, math.Hypot(validated.X-aisle.X, validated.Y-aisle.Y),
		64.0,
		"the hall click from the aisle must collapse onto the first step east")
}
