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

// TestLoopDiagnosticsWalkEta pins the walk ETA of the hunt
// diagnostics: the straight line length through the remaining
// waypoints into the destination divided by the run speed, the
// direct segment fallback without waypoints, no estimate outside
// the walking phases.
func TestLoopDiagnosticsWalkEta(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetLogger(recordingLogger(bot))
    // A distinctive run speed: the UserInfo drives the ETA divisor
    // away from the default 120 and keeps the fixture placement.
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest1", Level: 7,
        X: 45000, Y: 50000, Z: -3500,
        MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
        RunSpeed: 200, WalkSpeed: 80,
    })

    now := time.Now()
    loop.phase = phaseTownReturn
    loop.waypoints = []pathfind.Vec3{
        {X: 45600, Y: 50000, Z: -3500},
        {X: 46000, Y: 50000, Z: -3500},
    }
    loop.wpIndex = 0
    loop.segmentDest = pathfind.Vec3{X: 46200, Y: 50000, Z: -3500}

    // The character stands at 45000 50000 (newTestBot): 600 + 400 +
    // 200 units left, 1200 at speed 200 is six seconds.
    report := loop.diagnostics(now)
    require.Equal(t, int64(6000), report.WalkEtaMs)

    // The walked character carries the cursor with it: standing on
    // the second waypoint, the plan measures the last leg only (the
    // straight line restarts at the character, not at the passed
    // waypoint).
    loop.wpIndex = 1
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 46000, Y: 50000, Z: -3500,
    })
    report = loop.diagnostics(now)
    require.Equal(t, int64(1000), report.WalkEtaMs)

    // A direct segment without waypoints measures the straight line
    // into the armed destination: 400 units at 200 is two seconds.
    loop.waypoints = nil
    loop.wpIndex = 0
    loop.segmentDest = pathfind.Vec3{X: 45600, Y: 50000, Z: -3500}
    report = loop.diagnostics(now)
    require.Equal(t, int64(2000), report.WalkEtaMs)

    // No waypoints and no armed destination: no estimate (the trip
    // reset state).
    loop.segmentDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.WalkEtaMs)

    // The manual walk follows the user plan with the same estimate:
    // 400 + 200 units left at 200 is three seconds.
    loop.phase = phaseUser
    loop.userKind = state.CommandMove
    loop.userX, loop.userY, loop.userZ = 45800, 50000, -3500
    loop.userWaypoints = []pathfind.Vec3{
        {X: 45600, Y: 50000, Z: -3500},
    }
    loop.userWpIndex = 0
    report = loop.diagnostics(now)
    require.Equal(t, int64(3000), report.WalkEtaMs)

    // A manual command without a walk reports no estimate.
    loop.userKind = state.CommandAttack
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.WalkEtaMs)

    // The hunt phases carry no walk plan: the ETA stays out even
    // with a stale destination armed.
    loop.phase = phaseEngage
    loop.segmentDest = pathfind.Vec3{X: 46200, Y: 50000, Z: -3500}
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.WalkEtaMs)
}

// TestLoopDiagnosticsKillEta pins the kill ETA of the hunt
// diagnostics: the damage rate of the confirmed fight applied to the
// remaining health, no estimate for a fresh fight, a stale fight
// stamp, a whole target or an unknown one.
func TestLoopDiagnosticsKillEta(t *testing.T) {
    bot := newTestBot()
    spawnMob(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetLogger(recordingLogger(bot))
    // The gremlin holds 40 of 100 HP: ten seconds of fighting did
    // the 60 damage, the remaining 40 die at 6 hp per second.
    bot.ApplyStatusUpdate(7, []state.Attribute{
        {ID: state.AttrMaxHP, Value: 100},
        {ID: state.AttrCurHP, Value: 40},
    })

    now := time.Now()
    loop.target = 7
    loop.fightStartFor = 7
    loop.fightStartAt = now.Add(-10 * time.Second)

    report := loop.diagnostics(now)
    require.Equal(t, int64(6000), report.KillEtaMs)

    // A fight younger than the rate window is noise: no estimate.
    loop.fightStartAt = now.Add(-time.Second)
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.KillEtaMs)

    // A stale stamp of another target: no estimate.
    loop.fightStartAt = now.Add(-10 * time.Second)
    loop.fightStartFor = 99
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.KillEtaMs)

    // No target: no estimate even with a stamped start.
    loop.fightStartFor = 7
    loop.target = 0
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.KillEtaMs)

    // A target at full health shows no damage done: no estimate.
    loop.target = 7
    bot.ApplyStatusUpdate(7, []state.Attribute{
        {ID: state.AttrCurHP, Value: 100},
    })
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.KillEtaMs)

    // An unknown target id carries no vitals: no estimate.
    bot.ApplyStatusUpdate(7, []state.Attribute{
        {ID: state.AttrCurHP, Value: 40},
    })
    loop.target = 424242
    report = loop.diagnostics(now)
    require.Equal(t, int64(0), report.KillEtaMs)
}
