// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/stretchr/testify/require"
)

// The teacher walk regression of 2026-09-11 (see
// pathfind/teacher_walk_test.go for the engine side pins): the state
// dump of the learning trip showed the follower passing waypoints
// 0..10 of the shop -> Ellenia route, then standing frozen at
// 46152 51656 -2808 aiming at wp 11 (45992 52040 -2792) while every
// ground click canceled - the server (PathFinding = 0) validates each
// click as a straight geodata line, and the line from the standing
// pocket cell crosses the closed north wall of the plaza railing.
// The follower had skipped the tight ramp waypoints (16..35 units
// apart, all inside the 50 unit pass radius) without checking the
// line ahead, clicked the far waypoint through the wall and burned
// the whole re-path budget into "town trip ended: aborted, walk
// stuck" - the lessons never reached the teacher and the bot looped
// hunt -> near death -> emergency logout -> relogin -> trip forever.
// The follower now gates every waypoint skip on the walkable line to
// the next waypoint: the pocket cell keeps targeting the waypoint it
// can still reach, walking onto it re-opens the line.

// The teacher leg fixtures of the dump: the last waypoints of the
// planned route (the ramp climb into the trainer plaza) and the
// reported standing position one cell east of the ramp top.
var (
	teacherWalkRoute = []pathfind.Vec3{
		{X: 44872, Y: 46936, Z: -2992},
		{X: 45592, Y: 47768, Z: -3048},
		{X: 46904, Y: 50664, Z: -3000},
		{X: 46936, Y: 50920, Z: -2992},
		{X: 46632, Y: 51304, Z: -2992},
		{X: 46616, Y: 51320, Z: -2952},
		{X: 46488, Y: 51336, Z: -2904},
		{X: 46472, Y: 51352, Z: -2888},
		{X: 46200, Y: 51640, Z: -2824},
		{X: 46152, Y: 51640, Z: -2808},
		{X: 46136, Y: 51656, Z: -2776},
		{X: 45992, Y: 52040, Z: -2792},
		{X: 45912, Y: 52056, Z: -2792},
	}
	teacherWalkDest = pathfind.Vec3{X: 45725, Y: 52105, Z: -2792}
)

// teacherSight mirrors the real geodata answers of the ramp corner
// (probed against the deployed pack, see the engine test): the pocket
// cell 46152 51656 -2808 sees neither the ramp top (the west wall
// closed) nor the plaza waypoint (the north wall closed), the ramp
// foot cell sees the ramp top but not the plaza, and the ramp top
// sees the plaza.
func teacherSight(from, to pathfind.Vec3) (bool, error) {
	pocket := pathfind.Vec3{X: 46152, Y: 51656, Z: -2808}
	foot := pathfind.Vec3{X: 46152, Y: 51640, Z: -2808}
	if from == pocket {
		return to == foot, nil
	}
	if from == foot {
		return to.Y == 51656 && to.X == 46136, nil
	}

	return true, nil
}

// TestTeacherWalkKeepsTheWaypointWhenTheLineAheadIsWalled pins the
// gate itself: the follower standing one cell east of the ramp top
// with wp 9 and wp 10 inside the pass radius must NOT skip them - the
// click goes to the reachable waypoint (south onto the ramp foot),
// never to the walled plaza waypoint. The dump's follower clicked
// 45992 52040 from exactly this position and froze.
func TestTeacherWalkKeepsTheWaypointWhenTheLineAheadIsWalled(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	nav.sightFunc = teacherSight
	// The dump state: the trip walks the teacher leg, the follower
	// aims at wp 9 while the character already stands beside it.
	loop.phase = phaseTownWalk
	loop.tripStart = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7155, Name: "Ellenia",
			X: 45725, Y: 52105, Z: -2792,
		},
	}}
	loop.waypoints = teacherWalkRoute
	loop.wpIndex = 9
	loop.legDest = teacherWalkDest
	moveSelfTo(bot, 46152, 51656, -2808)

	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase,
		"the walk continues, no abort")
	require.Equal(t, 9, loop.wpIndex,
		"the follower must not skip the walled-ahead waypoints")
	require.Equal(t, [][3]int32{{46152, 51640, -2808}}, game.walks,
		"the click must aim at the reachable ramp foot waypoint, "+
			"not the plaza waypoint through the wall")
	require.Zero(t, loop.rePaths,
		"the gate answer is not a stuck, no re-path budget burns")

	// The character walks onto the ramp foot: the line to the ramp
	// top opens, the follower advances through the tight waypoints
	// and clicks the ramp top.
	moveSelfTo(bot, 46152, 51640, -2808)
	loop.tick()
	require.Equal(t, [][3]int32{
		{46152, 51640, -2808},
		{46136, 51656, -2776},
	}, game.walks,
		"the ramp top becomes the target from the ramp foot")
	require.Equal(t, 10, loop.wpIndex)

	// The character climbs onto the ramp top: the line to the plaza
	// waypoint opens and the walk continues north.
	moveSelfTo(bot, 46136, 51656, -2776)
	loop.tick()
	require.Equal(t, [][3]int32{
		{46152, 51640, -2808},
		{46136, 51656, -2776},
		{45992, 52040, -2792},
	}, game.walks,
		"the plaza waypoint is the target once the line is clear")
	require.Equal(t, 11, loop.wpIndex)
	require.Zero(t, loop.rePaths,
		"the whole corner passes without a single re-path")
}

// TestTeacherWalkRecoversFromTheDumpStuckSpot runs the exact reported
// state against the real geodata pack under the simulated server: the
// character stands at the dump position (the pocket cell east of the
// ramp top) with the teacher leg running. The follower must click the
// reachable ramp foot waypoint first (never the walled plaza waypoint
// the dump clicked), walk the corner and complete the leg into the
// stop phase - the dump burned all three re-paths and aborted without
// ever moving. The simulated server models the straight line
// validation conservatively (its raster refuses steps the real
// Mobius accepts - free downward drops, the layer gap fallback), so
// one recovery re-path at the plaza edge stays within the budget.
func TestTeacherWalkRecoversFromTheDumpStuckSpot(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, 46152, 51656, -2808)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	sim := &reproServer{nav: engine, minZ: -2808}

	// The dump state: the teach stop is current, the teacher leg
	// planned from the standing position (the re-planned corner
	// route, exactly what the stuck re-path machinery produces).
	loop.phase = phaseTownWalk
	loop.tripStart = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7155, Name: "Ellenia",
			X: 45725, Y: 52105, Z: -2792,
		},
		teach: true,
	}}
	require.True(t, loop.startWalkLeg(teacherWalkDest),
		"the corner re-plan from the pocket must find the teacher")

	// The first click aims at the reachable waypoint, not through the
	// wall: the dump's follower clicked 45992 52040 from this exact
	// position and the server canceled the move at once.
	loop.tick()
	sim.consume(game)
	require.NotEmpty(t, game.walks)
	require.Equal(t, [3]int32{46152, 51640, -2808}, game.walks[0],
		"the recovery click must go to the ramp foot waypoint")

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		loop.tick()
		sim.consume(game)
		sim.advance(bot)
		if loop.phase != phaseTownWalk {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, phaseTownSell, loop.phase,
		"the teacher leg must complete into the stop phase, no abort")
	require.LessOrEqual(t, loop.rePaths, 1,
		"the corner passes within the recovery budget (the dump "+
			"burned all three and aborted)")
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	require.True(t, ok)
	dist := dist3DTo(selfX, selfY, selfZ, 45725, 52105, -2792)
	t.Logf("the teacher leg walked to %d %d %d (dist to Ellenia %.0f)",
		selfX, selfY, selfZ, dist)
	require.LessOrEqual(t, dist, 350.0,
		"the walk must end inside the teacher approach ring - the "+
			"teacher approach machinery covers the rest")
}

// dist3DTo measures the full 3D distance between two world points.
func dist3DTo(x1, y1, z1, x2, y2, z2 int32) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)
	dz := float64(z1 - z2)

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
