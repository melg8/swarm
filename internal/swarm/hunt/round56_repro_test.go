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

// The reproduction of the 2026-09-11 06:19 building stuck report (the
// round 56 dump): bot test2 (level 11, phase townWalk, build 896865d)
// stood at the elven village trainer hall AISLE entrance (44728 51992
// -2792) on the walk to the teacher Ellenia (45725 52105 -2792) with
// the plan "wp 2 (44728 52040 -2792) <-- TARGET" - 48 units south -
// and froze there: "town walk stuck, skipping waypoint (1 of 3)" ->
// "the server would refuse the walk click to 45160 52120, re-pathing
// (2 of 3)" - the trip was burning its budget on the way to the abort.
//
// The geodata of the trainer hall building explains the mechanism.
// The building cells carry three layers: the sloped roof
// (-2600..-2448), the walkable floor (-2792..-2832, the walls encoded
// in the NSWE flags) and the water deck (-3928) under everything. The
// interior east of the aisle (x 44744..44792, y 52008..52039) has NO
// floor layer at all - only the roof and the water - so the straight
// line from the aisle entrance to the east hall waypoint resolves
// onto the roof layers, steps over the floorless gap at the running z
// and dies on the anti corner cut: the SW diagonal's flank cell
// (44760 51992) carries the building's north wall (its south wall is
// closed). The server's click validation collapses such a click onto
// the first step - a PARTIAL walk 16 units east. The blind stuck skip
// of the dump build armed exactly that partial click: the character
// crept 16 units per click along the building's north wall row into
// the dead-end pocket cell (44776 51992 -2808, its east and south
// walls closed), and from the pocket the click to the hall waypoint
// collapses onto the walker itself - the refused click the dump
// logged. The round 53 budget fix already stopped the abort; this
// round removes the trap itself: the skip only jumps onto a waypoint
// with a walkable line (nextClearWaypoint), so the recovery re-plans
// the aisle entrance route (whose first leg - 48 units south through
// the aisle column - the server always accepts) instead of creeping
// into the pocket.
const (
	// reproRound56X/Y/Z is the reported stuck position (the aisle
	// entrance of the trainer hall, the dump of 2026-09-11T06:19:10).
	reproRound56X = int32(44728)
	reproRound56Y = int32(51992)
	reproRound56Z = int32(-2792)
	// reproRound56ElleniaX/Y/Z is the teacher Ellenia (the dump
	// destination).
	reproRound56ElleniaX = int32(45725)
	reproRound56ElleniaY = int32(52105)
	reproRound56ElleniaZ = int32(-2792)
)

// walkToEllenia plans the town leg from the current position to
// Ellenia with the real geodata and walks it with the simulated
// server, bypassing the click pacing between the ticks. It reports
// whether the walk completed before the deadline.
func walkToEllenia(
	t *testing.T, loop *Loop, bot *state.Bot,
	game *fakeGame, sim *villageClickServer,
) bool {
	t.Helper()
	require.True(t, loop.startWalkLeg(pathfind.Vec3{
		X: float64(reproRound56ElleniaX),
		Y: float64(reproRound56ElleniaY),
		Z: float64(reproRound56ElleniaZ),
	}), "the aisle walk to Ellenia must plan a dry geodata route")

	return tickUntilArrived(loop, bot, game, sim)
}

// tickUntilArrived drives the follower and the simulated server until
// the plan completes or the deadline passes. It reports the arrival.
func tickUntilArrived(
	loop *Loop, bot *state.Bot, game *fakeGame, sim *villageClickServer,
) bool {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		loop.moveAt = time.Time{}
		selfX, selfY, selfZ, ok := bot.SelfPosition()
		if !ok {
			return false
		}
		done := loop.followWaypoints(selfX, selfY, selfZ, time.Now(), true)
		sim.consume(game, bot)
		if done {
			return true
		}
	}

	return false
}

// TestReproRound56AisleWalksToEllenia replays the reported walk
// against the real geodata pack: the character stands at the dump
// aisle entrance, the plan to Ellenia leads through the aisle column
// south into the hall and east to the teacher, the simulated server
// validates every click with the ported rules. The walk must arrive
// with zero refused clicks and zero stuck re-paths - the exact
// inversion of the reported failure signature.
func TestReproRound56AisleWalksToEllenia(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproRound56X, reproRound56Y, reproRound56Z)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	sim := &villageClickServer{engine: engine}

	require.True(t, walkToEllenia(t, loop, bot, game, sim),
		"the walk must complete before the deadline")
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	dist := math.Hypot(
		float64(selfX-reproRound56ElleniaX),
		float64(selfY-reproRound56ElleniaY))
	require.LessOrEqual(t, dist, tripApproachRadius,
		"the walk must arrive within the approach radius of Ellenia")
	require.Zero(t, sim.refused,
		"every click must survive the server validation")
	require.Zero(t, loop.rePaths,
		"the reported failure signature: no stuck re-path may fire")
}

// TestReproRound56StuckSkipNeverCreepsIntoThePocket replays the
// freeze itself: the character stands at the aisle entrance, the
// server stops moving it (the frozen sim - whatever silenced the
// dump's clicks), the stuck fires. The blind skip of the dump build
// armed the east hall waypoint here - the partial clicks crept the
// character into the dead-end pocket (44776 51992) and the pocket
// refused the click wholesale. The gated skip must instead re-plan
// the leg: the fresh aisle route starts with the 48 unit south click
// through the aisle column, and once the freeze releases the walk
// completes - with ZERO refused clicks (the pocket never armed) and
// exactly one recovery re-path.
func TestReproRound56StuckSkipNeverCreepsIntoThePocket(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproRound56X, reproRound56Y, reproRound56Z)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	sim := &villageClickServer{engine: engine}

	require.True(t, loop.startWalkLeg(pathfind.Vec3{
		X: float64(reproRound56ElleniaX),
		Y: float64(reproRound56ElleniaY),
		Z: float64(reproRound56ElleniaZ),
	}), "the aisle walk to Ellenia must plan a dry geodata route")

	// The freeze: the first click goes out, the server never moves the
	// character, the stuck timer passes the full window.
	loop.moveAt = time.Time{}
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	require.True(t, ok)
	done := loop.followWaypoints(selfX, selfY, selfZ, time.Now(), true)
	require.False(t, done)
	sim.frozen = true
	sim.consume(game, bot)
	armStuck(loop, bot)

	// The stuck fires: no successor of the aisle plan has a walkable
	// line from the aisle entrance (the lines to the east hall cross
	// the floorless interior and the walled flank), so the skip must
	// NOT arm them - the leg re-plans instead. The cursor returns to
	// the fresh plan's start and the character stays on the aisle
	// entrance cell, never creeping east toward the pocket.
	loop.moveAt = time.Time{}
	selfX, selfY, selfZ, ok = bot.SelfPosition()
	require.True(t, ok)
	require.False(t, loop.followWaypoints(selfX, selfY, selfZ,
		time.Now(), true))
	require.Equal(t, 1, loop.rePaths,
		"exactly one recovery re-path must fire")
	require.LessOrEqual(t, selfX, reproRound56X,
		"the character must not creep east of the aisle entrance")

	// The freeze releases: the re-planned route walks the aisle south
	// and the hall east to the teacher.
	sim.frozen = false
	require.True(t, tickUntilArrived(loop, bot, game, sim),
		"the walk must complete after the freeze")
	selfX, selfY, _, ok = bot.SelfPosition()
	require.True(t, ok)
	dist := math.Hypot(
		float64(selfX-reproRound56ElleniaX),
		float64(selfY-reproRound56ElleniaY))
	require.LessOrEqual(t, dist, tripApproachRadius,
		"the walk must arrive within the approach radius of Ellenia")
	require.Zero(t, sim.refused,
		"no click may be refused - the pocket creep never happens")
	require.Equal(t, 1, loop.rePaths,
		"the recovery re-path budget stays at the single re-plan")
}
