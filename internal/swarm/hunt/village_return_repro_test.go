// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The reproduction of the 2026-09-10 town walk stuck report: the
// relogged character stood at x 44440 y 51688 z -2832 (the elven
// village terrace east of the plaza) in the townReturn phase, the
// geodata plan led over the plaza corner (wp1 z -2792, wp2 z -2832,
// 58 units away) and every click to the second waypoint collapsed
// back onto the walker: the server click validation (the Bresenham
// raster of GeoEngine.getValidLocation with the anti corner cut rule
// of checkNearestNsweAntiCornerCut) refused the diagonal step over
// the terrace wall, the geodata correction collapsed the destination
// onto the walker cell and the move was canceled - "town walk stuck,
// re-pathing (1 of 3)" ground through the whole budget while the
// character never moved. The live server log of the reproduction:
// "MOVEDBG: test1 move CANCELED, distance=0.0 (geodata collapsed the
// target onto the walker), cur 44440 51688 -2832 -> 44440 51688
// -2832".
const (
	// reproVillageX/Y/Z is the reported stuck position (the dump of
	// 2026-09-10, the relogin position of test1).
	reproVillageX = int32(44440)
	reproVillageY = int32(51688)
	reproVillageZ = int32(-2832)
	// reproVillageZoneX/Y/Z is the hunting zone center of the report
	// (the Kaboo Orc Grunt S anchoring spot of the level 13 hunter).
	reproVillageZoneX = int32(42278)
	reproVillageZoneY = int32(56761)
	reproVillageZoneZ = int32(-3672)
)

// villageClickServer simulates the server side of the walk with the
// faithful click semantics: every walk request is validated the way
// Creature.moveToLocation validates it (the Engine.ValidateClick
// port) and the character stands on the point the server would walk
// to. A refused click (the server cancels the move) never moves the
// character - the mechanism of the reported freeze.
type villageClickServer struct {
	engine   *pathfind.Engine
	requests int
	target   [3]int32
	refused  int
}

// consume takes the newest walk request of the fake game, validates
// it exactly once - the way the server validates a MoveToLocation on
// its arrival - and applies the destination the server would walk to:
// the movement broadcast of the arrival stands the character on the
// validated point (the sim collapses the walk time, the click
// geometry is what the test pins). A refused click (the server
// cancels the move) never moves the character - the mechanism of the
// reported freeze.
func (s *villageClickServer) consume(game *fakeGame, bot *state.Bot) {
	if len(game.walks) <= s.requests {
		return
	}
	s.requests = len(game.walks)
	s.target = game.walks[len(game.walks)-1]
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	if !ok {
		return
	}
	from := pathfind.Vec3{
		X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
	}
	to := pathfind.Vec3{
		X: float64(s.target[0]), Y: float64(s.target[1]), Z: float64(s.target[2]),
	}
	validated, ok := s.engine.ValidateClick(from, to)
	if !ok {
		s.refused++

		return
	}
	bot.ApplyMovement(state.Movement{
		ObjectID: 100,
		X:        int32(validated.X), Y: int32(validated.Y), Z: int32(validated.Z),
		DestX: int32(validated.X), DestY: int32(validated.Y),
		DestZ: int32(validated.Z),
	})
}

// TestReproVillageZoneReturnWalksThePlan replays the reported zone
// return against the real geodata pack: the plan from the dump
// position to the hunting zone, the follower clicking the waypoints,
// the simulated server validating every click with the ported rules.
// The walk must arrive with zero refused clicks and zero stuck
// re-paths - the exact inversion of the reported failure signature
// (three re-paths and a character frozen on its own cell).
//
//nolint:dupl // mirrors TestReproRound53ZoneReturnWalksThePlan with the village coords
func TestReproVillageZoneReturnWalksThePlan(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproVillageX, reproVillageY, reproVillageZ)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	sim := &villageClickServer{engine: engine}

	// The zone return leg: the destination the returnToZone flow
	// would plan (the zone center on its real deck height).
	deckZ, err := engine.ClosestHeight(
		float64(reproVillageZoneX), float64(reproVillageZoneY),
		int16(reproVillageZoneZ))
	require.NoError(t, err, "the zone deck height must resolve")
	dest := pathfind.Vec3{
		X: float64(reproVillageZoneX),
		Y: float64(reproVillageZoneY),
		Z: float64(deckZ),
	}
	require.True(t, loop.startWalkLeg(dest),
		"the zone return must plan a dry geodata route")

	// Walk the plan: the follower paces its clicks (the pacing gate is
	// bypassed between the ticks to keep the test fast), the sim
	// stands the character on the server validated destinations.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		loop.moveAt = time.Time{}
		selfX, selfY, selfZ, ok := bot.SelfPosition()
		require.True(t, ok, "the character position must be known")
		done := loop.followWaypoints(selfX, selfY, selfZ, time.Now(), true)
		sim.consume(game, bot)
		if done {
			break
		}
	}
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	dist := math.Hypot(
		float64(selfX-reproVillageZoneX), float64(selfY-reproVillageZoneY))
	require.LessOrEqual(t, dist, tripApproachRadius,
		"the walk must arrive within the approach radius of the zone")
	require.Zero(t, sim.refused,
		"every click must survive the server validation")
	require.Zero(t, loop.rePaths,
		"the reported failure signature: no stuck re-path may fire")
}
