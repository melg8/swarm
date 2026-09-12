// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"bytes"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// stagnationBot builds an online autonomous session bot with the
// experience and the position the watch reads: level 10, exp 1000,
// standing at 45000 50000 -3500.
func stagnationBot() *state.Bot {
	bot := newTestBot()
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 10, ClassID: 18, Race: 1,
		X: 45000, Y: 50000, Z: -3500, Exp: 1000,
		MaxHP: 100, CurHP: 90, MaxMP: 100, CurMP: 50,
	})
	bot.SetOnline("test1")

	return bot
}

// newStagnationLoop builds the watch over the recording logger so
// the events the watch fires land in the buffer for the assertions.
func newStagnationLoop(
	bot *state.Bot, game GameAPI,
) (*Loop, *bytes.Buffer) {
	sink := &bytes.Buffer{}
	loop := NewLoop(game, bot)
	loop.SetLogger(log.New(
		io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))

	return loop, sink
}

// armStagnationXP seeds the experience baseline as observed one
// window (and a margin) ago, so the next observation holds past the
// threshold without waiting real time.
func armStagnationXP(loop *Loop, bot *state.Bot) {
	loop.stagXP = bot.SelfExp()
	loop.stagXPAt = time.Now().Add(-(stagnationXPWindow + time.Minute))
}

// armStagnationPosition seeds the position baseline as observed one
// window (and a margin) ago.
func armStagnationPosition(loop *Loop, bot *state.Bot) {
	x, y, z, _ := bot.SelfPosition()
	loop.stagPosX, loop.stagPosY, loop.stagPosZ = x, y, z
	loop.stagPosSet = true
	loop.stagPosAt = time.Now().Add(-(stagnationPositionWindow + time.Minute))
}

// TestStagnationXPFiresAfterWindow: an autonomous online session
// whose experience holds a full window logs the livelock event with
// the phase and re-arms (the next immediate observation stays quiet).
func TestStagnationXPFiresAfterWindow(t *testing.T) {
	bot := stagnationBot()
	loop, sink := newStagnationLoop(bot, &fakeGame{})
	armStagnationXP(loop, bot)

	loop.observeStagnation(time.Now())
	lines := sink.String()
	require.Contains(t, lines, "stagnation: no experience change")
	require.Contains(t, lines, "21m0s")
	require.Contains(t, lines, "phase engage")

	// The window re-armed: an immediate repeat observation logs
	// nothing until the next full window passes.
	sink.Reset()
	loop.observeStagnation(time.Now())
	require.NotContains(t, sink.String(), "stagnation")
}

// TestStagnationXPRefreshesWhileFarming: a fresh experience value
// refreshes the timer, so a healthy farm never fires the event.
func TestStagnationXPRefreshesWhileFarming(t *testing.T) {
	bot := stagnationBot()
	loop, sink := newStagnationLoop(bot, &fakeGame{})
	armStagnationXP(loop, bot)

	// The kill landed: the experience grew.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 10, ClassID: 18, Race: 1,
		X: 45000, Y: 50000, Z: -3500, Exp: 1042,
		MaxHP: 100, CurHP: 90, MaxMP: 100, CurMP: 50,
	})
	loop.observeStagnation(time.Now())
	require.NotContains(t, sink.String(), "stagnation")

	// The re-armed window also does not fire on the next tick.
	sink.Reset()
	loop.observeStagnation(time.Now().Add(time.Minute))
	require.NotContains(t, sink.String(), "stagnation")
}

// TestStagnationPositionFiresAfterWindow: an autonomous online
// session standing on the exact same cell a full window logs the
// freeze event with the cell and the phase.
func TestStagnationPositionFiresAfterWindow(t *testing.T) {
	bot := stagnationBot()
	loop, sink := newStagnationLoop(bot, &fakeGame{})
	armStagnationPosition(loop, bot)

	loop.observeStagnation(time.Now())
	lines := sink.String()
	require.Contains(t, lines, "stagnation: position held")
	require.Contains(t, lines, "11m0s")
	require.Contains(t, lines, "45000 50000 -3500")
	require.Contains(t, lines, "phase engage")

	// One line per window: the immediate repeat stays quiet.
	sink.Reset()
	loop.observeStagnation(time.Now())
	require.NotContains(t, sink.String(), "stagnation")
}

// TestStagnationPositionRefreshesWhileMoving: a moved character
// refreshes the freeze timer, so the walks never fire the event.
func TestStagnationPositionRefreshesWhileMoving(t *testing.T) {
	bot := stagnationBot()
	loop, sink := newStagnationLoop(bot, &fakeGame{})
	armStagnationPosition(loop, bot)

	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45060, Y: 50000, Z: -3500,
		DestX: 45060, DestY: 50000, DestZ: -3500,
	})
	loop.observeStagnation(time.Now())
	require.NotContains(t, sink.String(), "stagnation")

	sink.Reset()
	loop.observeStagnation(time.Now().Add(time.Minute))
	require.NotContains(t, sink.String(), "stagnation")
}

// TestStagnationQuietForManualAndOffline: the watch stays silent and
// resets for a manual only session and for an offline session, so
// neither the interactive standing nor the relogin gap fires an
// event or inherits a stale baseline.
func TestStagnationQuietForManualAndOffline(t *testing.T) {
	bot := stagnationBot()
	loop, sink := newStagnationLoop(bot, &fakeGame{})
	armStagnationXP(loop, bot)
	armStagnationPosition(loop, bot)

	// A manual only session stands still legitimately.
	loop.SetAutonomy(false)
	loop.observeStagnation(time.Now())
	require.NotContains(t, sink.String(), "stagnation")
	require.True(t, loop.stagXPAt.IsZero())
	require.False(t, loop.stagPosSet)

	// Back to autonomous but offline: still quiet, still reset.
	loop.SetAutonomy(true)
	bot.SetOffline()
	loop.observeStagnation(time.Now())
	require.NotContains(t, sink.String(), "stagnation")
	require.True(t, loop.stagXPAt.IsZero())

	// The relogin re-arms from the fresh values: no inherited stall.
	bot.SetOnline("test1")
	loop.observeStagnation(time.Now())
	require.False(t, loop.stagXPAt.IsZero())
	require.True(t, loop.stagPosSet)
	require.WithinDuration(t, time.Now(), loop.stagXPAt, time.Second)
}

// TestStagnationWatchRunsOnEveryTickPath: the tick defer observes the
// watch whatever early return the state machine took - a dead
// character tick still publishes the stall of the death cycle.
func TestStagnationWatchRunsOnEveryTickPath(t *testing.T) {
	bot := stagnationBot()
	loop, sink := newStagnationLoop(bot, &fakeGame{})
	armStagnationXP(loop, bot)
	// The dead branch returns before the phase dispatch.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})

	loop.tick()
	require.Contains(t, sink.String(), "stagnation: no experience change")
}

// TestStagnationEventSurfacesInTrackerFeed: the event line the watch
// writes reaches the bot tracker event log (the web UI event feed
// source) through the logf NoteAction mirror, with the phase word
// in the line.
func TestStagnationEventSurfacesInTrackerFeed(t *testing.T) {
	bot := stagnationBot()
	loop, _ := newStagnationLoop(bot, &fakeGame{})
	armStagnationPosition(loop, bot)

	loop.observeStagnation(time.Now())
	var found bool
	for _, event := range bot.Snapshot().Events {
		if strings.Contains(event.Message, "stagnation: position held") &&
			strings.Contains(event.Message, "phase") {
			found = true
		}
	}
	require.True(t, found,
		"the stagnation event must reach the tracker event feed")
}

// TestStagnationStallsSurfaceInDiagnostics: the diagnostics view the
// state dump carries reports both stall ages from the watch
// baselines, so a freeze report shows the held time at a glance.
func TestStagnationStallsSurfaceInDiagnostics(t *testing.T) {
	bot := stagnationBot()
	loop, _ := newStagnationLoop(bot, &fakeGame{})
	armStagnationXP(loop, bot)
	armStagnationPosition(loop, bot)

	report := loop.diagnostics(time.Now())
	require.Greater(t, report.XpStallForMs,
		int64(19*time.Minute/time.Millisecond))
	require.Greater(t, report.PositionStallForMs,
		int64(9*time.Minute/time.Millisecond))

	// A reset watch reports zero stalls, never a growing residual.
	loop.resetStagnationWatch()
	report = loop.diagnostics(time.Now())
	require.Zero(t, report.XpStallForMs)
	require.Zero(t, report.PositionStallForMs)
}

// TestStagnationXPStallRebuildsSession: a full experience window
// without progress drives the hard recovery - the emergency logout
// with the honest stagnation reason, the login cooldown that paces
// the supervisor reconnect and the quiet logoutDone tick gate.
func TestStagnationXPStallRebuildsSession(t *testing.T) {
	bot := stagnationBot()
	game := &fakeGame{}
	loop, sink := newStagnationLoop(bot, game)
	armStagnationXP(loop, bot)

	loop.observeStagnation(time.Now())
	require.Equal(t, 1, game.logouts, "the xp stall rebuilds the session")
	require.True(t, loop.logoutDone)
	require.Positive(t, bot.LoginCooldownRemaining())
	require.Contains(t, sink.String(), "rebuilding the session")
	require.Contains(t, sink.String(), "no experience change for 21m0s")
}

// TestStagnationHardCooldownHoldsSecondRebuild: a stall that fires
// again inside the cooldown window stays at one logout (the rebuild
// either fixed the loop or the next window retries), while a stall
// past the cooldown rebuilds again.
func TestStagnationHardCooldownHoldsSecondRebuild(t *testing.T) {
	bot := stagnationBot()
	game := &fakeGame{}
	loop, _ := newStagnationLoop(bot, game)
	now := time.Now()
	armStagnationXP(loop, bot)

	loop.observeStagnation(now)
	require.Equal(t, 1, game.logouts)

	// A logout request that failed to unwind the session (the socket
	// stayed open): the next fire lands inside the cooldown window and
	// must not thrash a second reconnect into the dying session. The
	// logoutDone gate already holds a healthy unwind; this models the
	// failed-request path where it does not.
	loop.logoutDone = false
	armStagnationXP(loop, bot)
	loop.observeStagnation(now.Add(time.Minute))
	require.Equal(t, 1, game.logouts, "the cooldown holds the rebuild")

	// Past the cooldown the rebuild retries.
	loop.logoutDone = false
	armStagnationXP(loop, bot)
	loop.observeStagnation(now.Add(stagnationHardCooldown + time.Minute))
	require.Equal(t, 2, game.logouts, "the rebuild retries past the cooldown")
}

// TestStagnationPositionFirstFireSoftResets: the first position
// stall clears the frozen loop state in place - the engage target
// drops onto the skip list, a trip caught mid freeze restarts from
// the engage phase, the blind recovery and the panic state clear -
// and the session stays (no logout fires).
func TestStagnationPositionFirstFireSoftResets(t *testing.T) {
	bot := stagnationBot()
	game := &fakeGame{}
	loop, sink := newStagnationLoop(bot, game)
	loop.target = 1234
	loop.phase = phaseTownWalk
	armStagnationPosition(loop, bot)

	loop.observeStagnation(time.Now())
	require.Zero(t, game.logouts, "the soft reset keeps the session")
	require.Zero(t, loop.target, "the frozen target drops")
	require.True(t, loop.targetSkipped(1234, time.Now()),
		"the dropped target stays skipped for the re-pick")
	require.Equal(t, phaseEngage, loop.phase, "the trip restarts from engage")
	require.Contains(t, sink.String(), "clearing the loop state")
}

// TestStagnationPositionSecondFireRebuildsSession: a freeze that
// survives the soft reset climbs to the session rebuild.
func TestStagnationPositionSecondFireRebuildsSession(t *testing.T) {
	bot := stagnationBot()
	game := &fakeGame{}
	loop, _ := newStagnationLoop(bot, game)
	armStagnationPosition(loop, bot)

	// First fire: the soft reset.
	loop.observeStagnation(time.Now())
	require.Zero(t, game.logouts)

	// Second fire without any movement between: the hard reset.
	armStagnationPosition(loop, bot)
	loop.observeStagnation(time.Now())
	require.Equal(t, 1, game.logouts, "the surviving freeze rebuilds")
	require.True(t, loop.logoutDone)
}

// TestStagnationMovementResetsEscalation: a character that moves
// after the soft reset clears the fire counter, so the next freeze
// starts from the soft recovery again instead of jumping straight
// to the rebuild.
func TestStagnationMovementResetsEscalation(t *testing.T) {
	bot := stagnationBot()
	game := &fakeGame{}
	loop, _ := newStagnationLoop(bot, game)
	armStagnationPosition(loop, bot)

	loop.observeStagnation(time.Now())
	require.Zero(t, game.logouts)
	require.Equal(t, 1, loop.stagPosFires)

	// The soft reset worked: the character moved again.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45100, Y: 50100, Z: -3500,
		DestX: 45100, DestY: 50100, DestZ: -3500,
	})
	loop.observeStagnation(time.Now())
	require.Zero(t, loop.stagPosFires, "the movement resets the escalation")

	// The next freeze starts over at the soft reset.
	armStagnationPosition(loop, bot)
	loop.observeStagnation(time.Now())
	require.Zero(t, game.logouts)
}

// TestStagnationDelevelExemptFromRecovery: the deleveling owns its
// lifecycle (its own timeout, its deaths refresh the experience),
// so neither the soft reset nor the session rebuild fires for it.
func TestStagnationDelevelExemptFromRecovery(t *testing.T) {
	bot := stagnationBot()
	game := &fakeGame{}
	loop, sink := newStagnationLoop(bot, game)
	loop.phase = phaseDelevel
	armStagnationXP(loop, bot)
	armStagnationPosition(loop, bot)

	loop.observeStagnation(time.Now())
	require.Zero(t, game.logouts, "the delevel phase rebuilds nothing")
	require.Contains(t, sink.String(),
		"the delevel phase owns its own recovery")
}
