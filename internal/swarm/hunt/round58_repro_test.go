// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The reproduction of the 2026-09-12 01:50 state dump report (build
// 2149ad1, bot test1, phase engage, uptime 10h32m): the character stood
// at x 43048 y 50312 z -2992 (the elven village main street, next to
// Herbiel) with the hunting zone 7900 units away at 35214 51358 (the
// Kaboo Orc Fighter SW spot leash, half 1448), the walk plan empty, and
// for the last 1h15m the event log held nothing but the server pings -
// no hunt decision, no movement, a single frozen position.
//
// The event trail that led there: the level 14 character started a
// deleveling at the town guards, three guard deaths respawned it in the
// village, the guard walk's first click failed the offline validation
// from the respawn cell ("the server would refuse the walk click"),
// the frozen re-path rule aborted the deleveling and the machinery
// started the return leg to the farm spot - which inherited the FROZEN
// re-path cell of the guard walk (startWalkLegSearch kept
// repathX/repathY), so its own first refused click ended the whole
// trip in one second ("town trip ended: aborted, the server refuses
// the walk click from this cell"). The abortFrozenTrip escalation
// armed zoneFails = zoneReturnFailBudget and the engage phase fell
// back to the direct zone legs (walkZoneLeg): every one of those
// clicks aimed 1000 units southwest - straight into the walled
// southern side of the street - the server click validation collapses
// that line onto the walker, the move is silently canceled and the
// character freezes. walkZoneLeg never validated its clicks, never
// noticed the missing movement and never logged a line: the bot ground
// the same refused click once per second forever.
//
// The fix (two halves, both pinned here):
//  1. startWalkLegSearch resets the frozen re-path cell: a fresh plan
//     owns a fresh frozen budget, the return leg survives its first
//     refused click with a re-path like any walk.
//  2. guardZoneLegClick validates every direct zone leg through the
//     server click port: a refused leg is never sent, the refusal
//     re-arms the pathfound zone return (zoneFails back under the
//     budget) and the paced log line explains the standing hunter.
const (
	// reproRound58X/Y/Z is the reported stuck position (the dump of
	// 2026-09-12 01:50, the village respawn cell of test1).
	reproRound58X = int32(43048)
	reproRound58Y = int32(50312)
	reproRound58Z = int32(-2992)
	// reproRound58ZoneX/Y is the hunting zone center of the report
	// (the Kaboo Orc Fighter SW anchoring spot, the leash half 1448).
	reproRound58ZoneX = int32(35214)
	reproRound58ZoneY = int32(51358)
)

// TestReproRound58VillageStuckCellWalksToZone replays the exact dump
// state against the real geodata pack: the character on the reported
// cell, the zone of the report, the post-abort escalation state
// (zoneFails armed, the stale zoneReturn flag) and the faithful click
// server. The walk must leave the walled street pocket and arrive
// inside the zone with every sent click validated - the inversion of
// the dump signature (a single refused click per second, a character
// frozen for hours).
func TestReproRound58VillageStuckCellWalksToZone(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproRound58X, reproRound58Y, reproRound58Z)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(reproRound58ZoneX, reproRound58ZoneY, 1448)
	loop.lastHit = time.Now().Add(-time.Minute)
	// The post-abort state of the dump: the frozen re-path aborted the
	// return leg, abortFrozenTrip armed the fail budget and the zone
	// return flag survived the deleveling (the "back in the zone" reset
	// only runs inside the zone, which the character never reached).
	loop.zoneFails = zoneReturnFailBudget
	loop.zoneReturn = true

	sim := &villageClickServer{engine: engine}
	arrived := false
	for i := 0; i < 2400 && !arrived; i++ {
		// The pacing gates are bypassed between the ticks to keep the
		// test fast (the tick period is the real 250ms cadence).
		loop.lastHit = time.Now().Add(-time.Minute)
		loop.moveAt = time.Time{}
		loop.tick()
		sim.consume(game, bot)
		if x, y, _, ok := bot.SelfPosition(); ok &&
			inZoneSquare(x, y, reproRound58ZoneX, reproRound58ZoneY, 1448) &&
			!loop.tripActive() {
			arrived = true
		}
	}
	require.True(t, arrived,
		"the zone return must walk the dump cell into the zone")
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	require.LessOrEqual(t,
		math.Hypot(float64(selfX-reproRound58ZoneX),
			float64(selfY-reproRound58ZoneY)),
		1448.0, "the walk must end inside the zone square")
	require.Zero(t, sim.refused,
		"every sent click must survive the server validation - the "+
			"dump ground one refused click per second")
	require.Equal(t, phaseEngage, loop.phase,
		"the finished trip returns the hunt to the engage phase")
}

// inZoneSquare reports whether the position lies inside the leash
// square of the round 58 zone.
func inZoneSquare(x, y, cx, cy, half int32) bool {
	dx := x - cx
	if dx < 0 {
		dx = -dx
	}
	dy := y - cy
	if dy < 0 {
		dy = -dy
	}

	return dx <= half && dy <= half
}

// TestReproRound58ZoneLegGuardRefusalReArmsPathfoundReturn pins the
// guard itself: a direct zone leg the server would refuse is never
// sent, the refusal re-arms the pathfound return (zoneFails back
// under the budget) and the paced diagnostic names the wall.
func TestReproRound58ZoneLegGuardRefusalReArmsPathfoundReturn(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	moveSelfTo(bot, reproRound58X, reproRound58Y, reproRound58Z)
	loop.zoneCX = reproRound58ZoneX
	loop.zoneCY = reproRound58ZoneY
	loop.zoneHalf = 1448
	loop.zoneFails = zoneReturnFailBudget
	nav.refuseClicks = true

	zone := loop.zone()
	require.NotNil(t, zone)
	loop.walkZoneLeg(zone, reproRound58X, reproRound58Y, reproRound58Z)

	require.Empty(t, game.walks,
		"the refused direct leg is never sent to the server")
	require.Zero(t, loop.zoneFails,
		"the refusal re-arms the pathfound zone return")

	// The validated leg goes out as before: the guard must not silence
	// the working direct legs of the open ground.
	nav.refuseClicks = false
	loop.zoneFails = zoneReturnFailBudget
	loop.walkZoneLeg(zone, reproRound58X, reproRound58Y, reproRound58Z)
	require.Len(t, game.walks, 1,
		"a validated direct leg is sent")
	require.Equal(t, zoneReturnFailBudget, loop.zoneFails,
		"a validated leg keeps the escalation state")
}

// TestReproRound58ReturnLegResetsFrozenRepath pins the frozen budget
// reset: the return leg is a fresh logical unit, it never inherits the
// frozen re-path cell of the guard walk that came before it, so its
// first refused click re-paths instead of aborting the whole trip
// instantly (the dump lost the guard walk and the return leg from the
// same cell in one second).
func TestReproRound58ReturnLegResetsFrozenRepath(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	moveSelfTo(bot, reproRound58X, reproRound58Y, reproRound58Z)
	loop.zoneCX = reproRound58ZoneX
	loop.zoneCY = reproRound58ZoneY
	loop.zoneHalf = 1448
	loop.repathX, loop.repathY = reproRound58X, reproRound58Y
	loop.frozenRepaths = frozenRepathLimit
	nav.refuseClicks = true

	loop.startReturnLeg()

	require.Equal(t, phaseTownReturn, loop.phase,
		"the return leg takes over the trip")
	require.Zero(t, loop.repathX)
	require.Zero(t, loop.repathY)
	require.Zero(t, loop.frozenRepaths,
		"the return leg starts with a clean frozen budget")

	// The first refused click of the fresh return leg re-paths instead
	// of aborting: the inherited frozen cell is gone.
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	require.True(t, ok)
	require.False(t, loop.followWaypoints(selfX, selfY, selfZ, time.Now(), true),
		"the leg is not finished after one refused click")
	require.Equal(t, 1, loop.rePaths,
		"the refused click spent one ordinary re-path")
	require.NotEmpty(t, loop.waypoints,
		"the re-path re-planned the leg")
	require.Equal(t, phaseTownReturn, loop.phase,
		"the leg keeps running after the refused click")
}

// TestReproRound58DelevelAbortKeepsFrozenBudgetClean drives the abort
// path of the dump end to end: the deleveling that dies on a frozen
// cell hands a CLEAN frozen budget to its return leg - the abort, the
// return leg start and the first refused click of the return leg all
// happen in one tick sequence without the instant trip abort of the
// report.
func TestReproRound58DelevelAbortKeepsFrozenBudgetClean(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	moveSelfTo(bot, reproRound58X, reproRound58Y, reproRound58Z)
	loop.zoneCX = reproRound58ZoneX
	loop.zoneCY = reproRound58ZoneY
	loop.zoneHalf = 1448
	// The deleveling walked toward the guard and its re-path froze on
	// the respawn cell (the guard walk click the server refuses).
	loop.phase = phaseDelevel
	loop.repathX, loop.repathY = reproRound58X, reproRound58Y
	loop.frozenRepaths = frozenRepathLimit
	nav.refuseClicks = true

	loop.abortDelevel("the server refuses the walk click from this cell")

	require.Equal(t, phaseTownReturn, loop.phase,
		"the abort hands over to the return leg")
	require.Zero(t, loop.repathX)
	require.Zero(t, loop.repathY)
	require.Zero(t, loop.frozenRepaths,
		"the return leg starts with a clean frozen budget")
}
