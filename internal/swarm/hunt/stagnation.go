// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The stagnation windows of the hunt loop livelock watch: the M1
// soak proof requires that a silently stuck bot becomes a loud line,
// so the loop tracks when the character last gained experience and
// when it last stood on a different cell, and logs an explicit event
// when either value holds past its window. The values are calibrated
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
	// stagnationHardCooldown paces the hard recovery (the session
	// rebuild through the emergency logout): a livelocked loop that
	// survives its relogin retries on the next window instead of
	// thrashing reconnects, and a run that recovers by itself pays
	// nothing.
	stagnationHardCooldown = 15 * time.Minute
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
// The recovery escalation rides the same watch (the point of the
// watch is a stuck bot that unsticks itself, not only a loud line):
// the first position stall clears the frozen loop state in place
// (the soft reset - the target bookkeeping, the pending loot, the
// trip walk, the panic and flee episodes all restart from the live
// world state), and a stall that survives it or a full experience
// window without progress rebuilds the whole session through the
// emergency logout (the hard reset - the relogin rebuilds every loop
// structure and resets the server side session state, the known cure
// of every livelock class this project has met). The timers re-arm
// after an event (one line per window, a frozen character logs its
// stall every window instead of every 250 ms tick) and reset on the
// offline gaps (a lost session that reconnects starts its windows
// fresh - the supervisor relogin is not a livelock) and on the
// manual only sessions (an interactive character is allowed to
// stand still).
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
		l.stagXPFires = 0

		return
	}
	held := now.Sub(l.stagXPAt)
	if held < stagnationXPWindow {
		return
	}
	l.stagXPFires++
	l.logf("Hunt: stagnation: no experience change for %s, "+
		"exp %d held, phase %s", held.Round(time.Minute), xp, l.phase)
	if l.journal != nil {
		l.journal.Stall(l.tracker.ID(), "xp", held, 0, 0, string(l.phase))
	}
	// The window restarts instead of the whole watch: a permanently
	// stalled loop keeps logging one line per window until the
	// character progresses again or the session ends.
	l.stagXPAt = now
	// The experience stall is the strongest livelock signal: the
	// character may still drift between refused requests (a pacing
	// loop that moves the cell without farming anything), so the
	// position watch cannot be relied on to catch it - the session
	// rebuild answers it directly.
	l.stagnationHardRecover("no experience change for "+
		held.Round(time.Minute).String(), now)
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
		l.stagPosFires = 0

		return
	}
	held := now.Sub(l.stagPosAt)
	if held < stagnationPositionWindow {
		return
	}
	l.stagPosFires++
	l.logf("Hunt: stagnation: position held %s at %d %d %d, phase %s",
		held.Round(time.Minute), x, y, z, l.phase)
	if l.journal != nil {
		l.journal.Stall(l.tracker.ID(), "pos", held, x, y, string(l.phase))
	}
	l.stagPosAt = now
	// The freeze escalation: the first fire clears the loop state in
	// place, a freeze that survives it (or returns after the soft
	// recovery) climbs to the session rebuild.
	if l.stagPosFires == 1 {
		l.stagnationSoftReset(now)

		return
	}
	l.stagnationHardRecover("position held "+held.Round(time.Minute).String()+
		" at "+strconv.Itoa(int(x))+" "+strconv.Itoa(int(y)), now)
}

// stagnationSoftReset clears the frozen loop state of the first
// position stall without ending the session: the engage bookkeeping
// (the target with its stuck timers, the pending loot, the blind
// recovery), the armed panic and flee episodes and a town trip walk
// caught mid freeze all restart from the live world state, and a
// sitting character stands up. The next tick then re-runs its phase
// from scratch - the patrol re-arms, the zone return re-paths, the
// trip trigger re-plans. The deleveling owns its own lifecycle (its
// deaths refresh the experience, its own timeouts bound it), so its
// phase is left alone.
func (l *Loop) stagnationSoftReset(now time.Time) {
	if l.phase == phaseDelevel {
		l.logf("Hunt: stagnation recovery: the delevel phase owns " +
			"its own recovery, holding the soft reset")

		return
	}
	l.logf("Hunt: stagnation recovery: clearing the loop state, " +
		"restarting from the live world")
	if l.target != 0 {
		if l.targetSkip == nil {
			l.targetSkip = make(map[int32]time.Time)
		}
		// The frozen target keeps the short skip: the re-pick must
		// prefer a different object, the frozen one is the very
		// suspect of the freeze.
		l.targetSkip[l.target] = now.Add(engageSkipDelay)
	}
	l.target = 0
	l.engageAt = time.Time{}
	l.clearBlindRecovery()
	l.lootID = 0
	l.panicAt = time.Time{}
	l.fleeAt = time.Time{}
	l.fleeSince = time.Time{}
	l.noTargetSince = time.Time{}
	l.lastHit = time.Time{}
	// A trip caught mid freeze restarts from scratch: the frozen
	// waypoints drop and the trigger re-plans from wherever the
	// character stands (the junk, the books and the adena all stay
	// in the inventory - only the frozen walk plan dies).
	l.resetTownTrip()
	// A frozen walk back home re-arms the pathfound return: the
	// return budget clears so the next outside-the-zone tick plans a
	// fresh geodata route instead of inheriting the failed one.
	l.zoneReturn = false
	l.zoneFails = 0
	l.standUpGuarded(now)
}

// stagnationHardRecover rebuilds the session through the emergency
// logout: the relogin constructs a fresh loop (the spot handoff
// resumes the ground), resets the server side session state and
// clears every per-session structure a livelock can hide in - the
// known cure of all the livelock classes this project has met (the
// stale server selection, the refused collapsed walk clicks, the
// frozen engage stance). The hard reset is paced by
// stagnationHardCooldown so a loop that keeps livelocking retries
// one rebuild per cooldown window instead of thrashing reconnects,
// and the delevel phase is exempt (its own timeout and death cycle
// own it).
func (l *Loop) stagnationHardRecover(reason string, now time.Time) {
	if l.logoutDone || l.phase == phaseDelevel {
		return
	}
	if !l.stagHardAt.IsZero() && now.Sub(l.stagHardAt) < stagnationHardCooldown {
		// The recovery is on cooldown: the stall line above stays the
		// only output of this window (the previous rebuild either
		// fixed the loop or the next window retries).
		return
	}
	l.stagHardAt = now
	l.stagPosFires = 0
	l.stagXPFires = 0
	l.emergencyLogoutWithReason("stagnation " + reason +
		", rebuilding the session")
}

// resetStagnationWatch clears the observed baselines: the next online
// tick re-arms both windows from fresh values, so neither an offline
// gap nor a manual session can fire an event for time the character
// was not autonomously in the world. The fire counters reset with
// the baselines so a fresh session never inherits the escalation.
func (l *Loop) resetStagnationWatch() {
	l.stagXPAt = time.Time{}
	l.stagPosSet = false
	l.stagPosAt = time.Time{}
	l.stagPosFires = 0
	l.stagXPFires = 0
}
