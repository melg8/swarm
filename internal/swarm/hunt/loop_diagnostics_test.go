// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// TestLoopPublishesHuntDiagnostics pins the per tick publication of
// the loop internals: the tick defer pushes the current target, its
// engagement age and the skip lists into the tracker, so the state
// dump carries them with a fresh heartbeat.
func TestLoopPublishesHuntDiagnostics(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetLogger(recordingLogger(bot))
	loop.lastHit = time.Now().Add(-time.Minute)

	// The fresh tracker carries no hunt view: the first tick
	// publishes it.
	require.Equal(t, state.HuntDiagnostics{},
		bot.Snapshot().Diagnostics.Hunt)

	loop.tick()

	snapshot := bot.Snapshot()
	require.Equal(t, int32(7), snapshot.Diagnostics.Hunt.TargetID)
	require.Equal(t, int64(0), snapshot.Diagnostics.Hunt.TargetForMs)
	require.Equal(t, 0, snapshot.Diagnostics.Hunt.SkippedTargets)
	require.Equal(t, int64(0), snapshot.Diagnostics.Hunt.TickAgoMs)
	require.Equal(t, "engage", snapshot.Phase)
	require.Equal(t, int64(0), snapshot.Diagnostics.PhaseForMs)

	// A target that never engages past the stuck timeout gets
	// skipped: the diagnostics show the empty target slot, the
	// grown skip list and the decision line of the event log.
	loop.engageAt = time.Now().Add(-20 * time.Second)
	loop.tick()

	snapshot = bot.Snapshot()
	hunt := snapshot.Diagnostics.Hunt
	require.Equal(t, int32(0), hunt.TargetID)
	require.Equal(t, 1, hunt.SkippedTargets)
	require.Equal(t, "Hunt: target 7 does not engage, switching to "+
		"another", hunt.LastAction)
	require.Equal(t, int64(0), hunt.LastActionAgoMs)

	found := false
	for _, event := range snapshot.Events {
		if event.Message == "Hunt: target 7 does not engage, "+
			"switching to another" {
			found = true
		}
	}
	require.True(t, found, "the decision must land in the event log")
}

// TestLoopNotesDeathDecision pins the decision log routing: the
// death recovery line lands in the tracker event log and becomes
// the last action of the hunt diagnostics view.
func TestLoopNotesDeathDecision(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetLogger(recordingLogger(bot))

	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})

	loop.tick()

	require.Equal(t, 1, game.restarts)
	snapshot := bot.Snapshot()
	require.Equal(t, "Hunt: character died, restarting at the nearest "+
		"village", snapshot.Diagnostics.Hunt.LastAction)
	found := false
	for _, event := range snapshot.Events {
		if event.Message == "Hunt: character died, restarting at "+
			"the nearest village" {
			found = true
		}
	}
	require.True(t, found, "the death must land in the event log")
}

// TestLoopDiagnosticsAges pins the duration references of the hunt
// diagnostics: never happened references report zero, live ones the
// floored age.
func TestLoopDiagnosticsAges(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetLogger(recordingLogger(bot))

	now := time.Now()
	loop.engageAt = now.Add(-(2*time.Second + 500*time.Millisecond))
	loop.tripStart = now.Add(-(65 * time.Second))
	loop.fleeSince = time.Time{}
	loop.stuckAt = now.Add(-(31 * time.Second))

	// The hunt phases carry no planned walk: the residual stuck and
	// trip stamps stay out of the report.
	loop.phase = phaseEngage
	report := loop.diagnostics(now)
	require.Equal(t, int64(2000), report.TargetForMs)
	require.Equal(t, int64(0), report.TripForMs)
	require.Equal(t, int64(0), report.StuckForMs)
	require.Equal(t, int64(0), report.FleeForMs)
	require.Equal(t, 0, report.WaypointsLeft)

	// The walking phases carry both walk clocks.
	loop.phase = phaseTownReturn
	report = loop.diagnostics(now)
	require.Equal(t, int64(65000), report.TripForMs)
	require.Equal(t, int64(31000), report.StuckForMs)
}

// TestLoopRemainingWaypoints pins the waypoint counting per phase:
// the manual plan of the user phase, the geodata plan of the town
// and delevel phases, zero elsewhere.
func TestLoopRemainingWaypoints(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetLogger(recordingLogger(bot))

	loop.phase = phaseEngage
	require.Equal(t, 0, loop.remainingWaypoints())

	loop.phase = phaseUser
	loop.userKind = state.CommandMove
	loop.userWaypoints = nil
	loop.userWpIndex = 0
	require.Equal(t, 0, loop.remainingWaypoints())

	loop.userWaypoints = make([]pathfind.Vec3, 5)
	loop.userWpIndex = 2
	require.Equal(t, 3, loop.remainingWaypoints())

	loop.userKind = state.CommandAttack
	require.Equal(t, 0, loop.remainingWaypoints())

	loop.phase = phaseTownReturn
	loop.waypoints = make([]pathfind.Vec3, 7)
	loop.wpIndex = 7
	require.Equal(t, 0, loop.remainingWaypoints())
	loop.wpIndex = 1
	require.Equal(t, 6, loop.remainingWaypoints())
}
