// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The stagnation windows of the hunt loop livelock watch: the M1 soak
// proof requires that a silently stuck bot becomes a loud line, so the
// loop tracks when the character last gained experience and when it
// last stood on a different cell, and logs an explicit event when
// either value holds past its window. The values are calibrated
// against the measured honest flows of the elven lands autonomy:
// the longest experience stall of a healthy loop is the full town
// trip round (the walk to the village, the sell stop, the buys and
// the lessons - measured 2.2 min for the weapon run round, the whole
// farm readiness round with every stop landed at 14m54s), and the
// longest honest stand on one exact cell is a rest or a paced lesson
// stop (minutes). A frozen walk (the round 58 stuck cell held a
// character on one village cell for over an hour) or a dead engage
// blows straight past both windows.
const (
	// stagnationXPWindow is how long the experience may hold without
	// a livelock event: kills change it every few seconds, the death
	// penalty changes it on every delevel death at level 10+, so a
	// value this old means the loop stopped farming and stopped
	// dying - it is stuck somewhere in between.
	stagnationXPWindow = 20 * time.Minute
	// stagnationPositionWindow is how long the character may stand on
	// the exact same cell without a livelock event: the walks, the
	// fights and the death cycles all move it, a hold this long means
	// the server stopped accepting the move requests or the loop
	// stopped sending them.
	stagnationPositionWindow = 10 * time.Minute
)

// observeStagnation watches the character progress of the autonomous
// session and logs one explicit event per window when a value holds
// past its stagnation threshold: no experience change for
// stagnationXPWindow or the exact same position for
// stagnationPositionWindow. The event line carries the current loop
// phase so the log explains what the state machine was doing while
// frozen, and it lands on the loop logger (the console, the web UI
// event feed and the state dump all mirror it - see SetLogger) and
// in the tracker action note. The watch runs on every tick through
// the publish defer, whatever path the state machine took.
//
// The timers re-arm after an event (one line per window, a frozen
// character logs its stall every window instead of every 250 ms
// tick) and reset on the offline gaps (a lost session that
// reconnects starts its windows fresh - the supervisor relogin is
// not a livelock) and on the manual only sessions (an interactive
// character is allowed to stand still).
func (l *Loop) observeStagnation(now time.Time) {
	if !l.autonomous || l.tracker.Status() != state.StatusOnline {
		l.resetStagnationWatch()

		return
	}
	l.observeStagnationXP(now)
	l.observeStagnationPosition(now)
}

// observeStagnationXP arms, refreshes and reports the experience
// stall timer.
func (l *Loop) observeStagnationXP(now time.Time) {
	xp := l.tracker.SelfExp()
	if l.stagXPAt.IsZero() || xp != l.stagXP {
		l.stagXP = xp
		l.stagXPAt = now

		return
	}
	held := now.Sub(l.stagXPAt)
	if held < stagnationXPWindow {
		return
	}
	l.logf("Hunt: stagnation: no experience change for %s, "+
		"exp %d held, phase %s", held.Round(time.Minute), xp, l.phase)
	// The window restarts instead of the whole watch: a permanently
	// stalled loop keeps logging one line per window until the
	// character progresses again or the session ends.
	l.stagXPAt = now
}

// observeStagnationPosition arms, refreshes and reports the position
// freeze timer.
func (l *Loop) observeStagnationPosition(now time.Time) {
	x, y, z, ok := l.tracker.SelfPosition()
	if !ok || !l.stagPosSet || x != l.stagPosX || y != l.stagPosY ||
		z != l.stagPosZ {
		l.stagPosX = x
		l.stagPosY = y
		l.stagPosZ = z
		l.stagPosSet = ok
		l.stagPosAt = now

		return
	}
	held := now.Sub(l.stagPosAt)
	if held < stagnationPositionWindow {
		return
	}
	l.logf("Hunt: stagnation: position held %s at %d %d %d, phase %s",
		held.Round(time.Minute), x, y, z, l.phase)
	l.stagPosAt = now
}

// resetStagnationWatch clears the observed baselines: the next online
// tick re-arms both windows from fresh values, so neither an offline
// gap nor a manual session can fire an event for time the character
// was not autonomously in the world.
func (l *Loop) resetStagnationWatch() {
	l.stagXPAt = time.Time{}
	l.stagPosSet = false
	l.stagPosAt = time.Time{}
}
