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

// The reproduction of the 2026-09-11 round 53 stuck report: bot
// test2 (level 11, phase townReturn) stood at x 46008 y 51992 z
// -2792 (the elven village teacher plaza) trying to walk back to the
// Kaboo Orc Grunt S hunting zone (42278 56761 -3672), but cycled
// "town walk stuck, skipping waypoint (1..3 of 3)" -> "the server
// would refuse the walk click to 45304 52152, re-pathing" -> "town
// trip ended: aborted, the server refuses every walk click" - twice
// within 90 seconds. The dump's walk plan aimed at wp 2 (46008 52040
// -2792), 48 units south of the character. The same build f1c3136
// that closed round 52 (the click collapse fix) reproduced the
// freeze on this new spot, so the round 52 port missed a refusal
// channel this dump exposes.
const (
	// reproRound53X/Y/Z is the reported stuck position (the dump of
	// 2026-09-11T05:06:14+03:00, bot test2 phase townReturn).
	reproRound53X = int32(46008)
	reproRound53Y = int32(51992)
	reproRound53Z = int32(-2792)
	// reproRound53ZoneX/Y/Z is the hunting zone center of the dump
	// (Kaboo Orc Grunt S).
	reproRound53ZoneX = int32(42278)
	reproRound53ZoneY = int32(56761)
	reproRound53ZoneZ = int32(-3672)
)

// TestReproRound53ZoneReturnWalksThePlan replays the reported zone
// return against the real geodata pack: the plan from the dump
// position to the hunting zone, the follower clicking the waypoints,
// the simulated server validating every click with the ported rules.
// The walk must arrive with zero refused clicks and zero stuck
// re-paths - the exact inversion of the reported failure signature.
//
// The test pins the round 53 regression: the round 52 fix
// (ValidateClick port) approved the click from (46008, 51992) to wp 2
// (46008, 52040, 48 units south), the click was sent, but the
// character never moved. The dump shows three re-path cycles in 90 s.
//
//nolint:dupl // mirrors TestReproVillageZoneReturnWalksThePlan with the round 53 coords
func TestReproRound53ZoneReturnWalksThePlan(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproRound53X, reproRound53Y, reproRound53Z)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	sim := &villageClickServer{engine: engine}

	// The zone return leg: resolve the destination deck height the way
	// returnToZone does (ClosestHeight on the zone center with the
	// character's z as the reference).
	deckZ, err := engine.ClosestHeight(
		float64(reproRound53ZoneX), float64(reproRound53ZoneY),
		int16(reproRound53ZoneZ))
	require.NoError(t, err, "the zone deck height must resolve")
	dest := pathfind.Vec3{
		X: float64(reproRound53ZoneX),
		Y: float64(reproRound53ZoneY),
		Z: float64(deckZ),
	}
	require.True(t, loop.startWalkLeg(dest),
		"the zone return must plan a dry geodata route")

	// Walk the plan: the follower paces its clicks (the pacing gate is
	// bypassed between the ticks to keep the test fast), the sim stands
	// the character on the server validated destinations.
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
		float64(selfX-reproRound53ZoneX), float64(selfY-reproRound53ZoneY))
	require.LessOrEqual(t, dist, tripApproachRadius,
		"the walk must arrive within the approach radius of the zone")
	require.Zero(t, sim.refused,
		"every click must survive the server validation")
	require.Zero(t, loop.rePaths,
		"the reported failure signature: no stuck re-path may fire")
}

// TestReproRound53FirstClickValidates isolates the very first click
// of the reported stuck leg: the character stands at the dump
// position (46008, 51992, -2792), the plan's wp 2 is 48 units south
// (46008, 52040, -2792), and the click between them MUST validate
// against the ported server rules. The dump's first event - "town
// walk stuck, skipping waypoint (1 of 3)" - implies the click did
// not move the character for 15 s. The test pins the click-level
// invariant the dump breaks.
func TestReproRound53FirstClickValidates(t *testing.T) {
	engine := reproEngine(t)
	from := pathfind.Vec3{
		X: float64(reproRound53X),
		Y: float64(reproRound53Y),
		Z: float64(reproRound53Z),
	}
	to := pathfind.Vec3{X: 46008, Y: 52040, Z: -2792}
	validated, ok := engine.ValidateClick(from, to)
	require.True(t, ok,
		"the click to wp 2 must validate (the dump's first leg)")
	require.InDelta(t, 46008.0, validated.X, 1.0,
		"the validated destination must be wp 2 itself")
	require.InDelta(t, 52040.0, validated.Y, 1.0,
		"the validated destination must be wp 2 itself")
}
