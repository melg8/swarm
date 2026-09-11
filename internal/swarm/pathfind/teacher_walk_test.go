// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// The teacher walk regression of 2026-09-11: the state dump showed a
// level 11 elven fighter walking the learning trip to the teacher
// Ellenia (45725 52105 -2792) from the village shop (Ariel at 44683
// 46952), the follower passing waypoints 0..10 of the planned route
// and then standing still at 46152 51656 -2808 until the re-path
// budget aborted the whole trip - "town trip ended: aborted, walk
// stuck" - so the queued lessons never reached the teacher. The
// planner route is smooth, but the last ramp steps before the trainer
// plaza are 16..25 units apart and the 50 unit pass radius of the
// follower skipped them while the character stood one cell beside
// the line: the straight click at the far waypoint crossed the closed
// north wall of the standing cell and the server (the deployment
// runs PathFinding = 0, every click is a straight line validation)
// canceled the move at the character's own position. These tests pin
// the exact geometry of that corner against the real geodata pack.
// teacherShopStart is the leg origin the dump walk plan carried.
var teacherShopStart = Vec3{X: 44872, Y: 46936, Z: -2992}

// teacherSpawn is the elven fighter teacher Ellenia.
var teacherSpawn = Vec3{X: 45725, Y: 52105, Z: -2792}

// teacherStuck is the reported standing position of the dump.
var teacherStuck = Vec3{X: 46152, Y: 51656, Z: -2808}

// teacherRampTop is wp 10 of the dump route, the ramp top.
var teacherRampTop = Vec3{X: 46136, Y: 51656, Z: -2776}

// teacherRampFoot is wp 9 of the dump route, the ramp foot south of
// the pocket cell.
var teacherRampFoot = Vec3{X: 46152, Y: 51640, Z: -2808}

// teacherPlaza is wp 11 of the dump route, across the railing.
var teacherPlaza = Vec3{X: 45992, Y: 52040, Z: -2792}

// TestTeacherRouteMatchesTheDumpPlan pins the planned route of the
// teacher leg: the dry approach search from the shop area to Ellenia
// finds the walk the dump carried, including the tight ramp steps
// before the plaza (wp 8..10 sit 16..48 units apart).
func TestTeacherRouteMatchesTheDumpPlan(t *testing.T) {
	engine := townTestEngine(t)

	result, err := engine.FindPathApproachDry(
		teacherShopStart, teacherSpawn, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found, "the dry route to the teacher exists")
	require.GreaterOrEqual(t, len(result.Waypoints), 11,
		"the route crosses the village, the ramp steps stay")
	// The dump route: ..., 46200 51640, 46152 51640, 46136 51656,
	// 45992 52040, 45912 52056 (approach ring of Ellenia).
	last := result.Waypoints[len(result.Waypoints)-1]
	require.LessOrEqual(t, dist3D(last, teacherSpawn), 200.0,
		"the route ends inside the teacher approach ring")
	// The dump route crossed the trainer ramp (wp 9..10, the tight 16
	// unit steps at 46152 51640 and 46136 51656); the Round 52 server
	// click validation found the ramp diagonal flank walled (the
	// vertical flank cell of the SW step carries nswe 0x9) and the
	// server PathFinding = 2 deployment refuses that click - the
	// route now climbs the east approach instead and crosses the
	// plaza level further west. The pinned points move with it: the
	// east ascent bend and the plaza corner before the approach ring.
	for _, want := range []Vec3{
		{X: 46184, Y: 51528, Z: -2824},
		{X: 46104, Y: 51528, Z: -2808},
		teacherPlaza,
	} {
		found := false
		for _, wp := range result.Waypoints {
			if dist3D(wp, want) < 64 {
				found = true

				break
			}
		}
		require.True(t, found, "the route must pass near %.0f %.0f",
			want.X, want.Y)
	}
}

// TestTeacherCornerWallBlocksTheStraightClick pins the mechanism of
// the stuck: the straight line from the reported standing position to
// the next planned waypoint is NOT walkable (the closed north wall of
// the standing cell - the server getValidLocation raster walks the
// same cells), and neither is the line to the ramp top itself (the
// west wall of the pocket cell). The only clear line out of the
// pocket goes south to the ramp foot, and the ramp climb opens from
// the foot - exactly the recovery the follower gate keeps. The plaza
// leg of the dump route no longer collapses into one straight click
// either: the reverse wall rule of the concurrent Round 49 refines
// the route with intermediate waypoints there instead, so the
// follower clicks a chain of verified short legs.
func TestTeacherCornerWallBlocksTheStraightClick(t *testing.T) {
	engine := townTestEngine(t)

	skip, err := engine.LineOfSight(
		teacherStuck, teacherPlaza, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, skip,
		"the skipped-forward click crosses the plaza railing wall")

	ramp, err := engine.LineOfSight(
		teacherStuck, teacherRampTop, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, ramp,
		"the pocket cell cannot click the ramp top either: its west "+
			"wall is closed too")

	foot, err := engine.LineOfSight(
		teacherStuck, teacherRampFoot, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, foot,
		"the south line to the ramp foot is the pocket's way out")

	climb, err := engine.LineOfSight(
		teacherRampFoot, teacherRampTop, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, climb,
		"the ramp climb diagonal is flank walled: the Round 52 anti "+
			"corner cut mirrors the server click validation, which "+
			"refuses the foot->top click on the PathFinding = 2 "+
			"deployment (the vertical flank cell carries nswe 0x9, its "+
			"west wall closed) - the planned routes climb the east "+
			"approach instead")

	planned, err := engine.LineOfSight(
		teacherRampTop, teacherPlaza, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, planned,
		"the plaza leg stays split into verified steps: the reverse "+
			"wall rule of the 2026-09-11 Round 49 keeps the straight "+
			"collapse honest")
}

// TestTeacherReplanFromTheStuckCorner pins the recovery: the dry
// approach search from the reported stuck position still finds a
// route to the teacher (the search walks around the railing), so the
// stuck re-path machinery has a way out when a walk ever lands off
// the planned line.
func TestTeacherReplanFromTheStuckCorner(t *testing.T) {
	engine := townTestEngine(t)

	result, err := engine.FindPathApproachDry(
		teacherStuck, teacherSpawn, 200, DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.True(t, result.Found,
		"the re-plan from the stuck corner must find the teacher")
	last := result.Waypoints[len(result.Waypoints)-1]
	require.LessOrEqual(t, dist3D(last, teacherSpawn), 200.0,
		"the re-planned route ends inside the approach ring")
}

// dist3D is the full 3D distance of two world points.
func dist3D(a, b Vec3) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
