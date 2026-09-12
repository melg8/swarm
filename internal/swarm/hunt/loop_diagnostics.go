// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The diagnostics publication of the hunt loop, split out of the
// loop.go god file: the internals the state dump carries for the
// stuck bot reports and the decision log routing.

import (
	"fmt"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// logf reports a hunt loop decision on the console logger and the
// bot tracker: the message becomes an event log entry of the state
// dump (the live server report carries the decision history next to
// the world data) and the last action of the hunt diagnostics view.
func (l *Loop) logf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.logger.Print(message)
	l.tracker.NoteAction(message)
}

// diagnostics snapshots the loop internals for the diagnostics
// section of the state dump: the current target, the skip lists, the
// walk and trip progress and the safety episodes, with their ages
// relative to now. The tick defer publishes the values on every
// decision of the state machine.
func (l *Loop) diagnostics(now time.Time) state.HuntDiagnostics {
	report := state.HuntDiagnostics{
		TargetID:       l.target,
		TargetForMs:    state.AgeMs(since(now, l.engageAt)),
		SkippedTargets: l.activeSkipCount(now),
		NoTargetForMs:  state.AgeMs(since(now, l.noTargetSince)),
		RePaths:        l.rePaths,
		StuckForMs:     0,
		WaypointsLeft:  l.remainingWaypoints(),
		TripForMs:      0,
		FleeForMs:      state.AgeMs(since(now, l.fleeSince)),
		BuyRetries:     l.buyRetries,
		// The stagnation stalls read the watch baselines directly: a
		// zero stamp (offline gap, manual session, fresh session)
		// reports a zero age, so the dump never shows a stall the
		// watch itself does not count.
		XpStallForMs:       state.AgeMs(since(now, l.stagXPAt)),
		PositionStallForMs: state.AgeMs(since(now, l.stagPosAt)),
		// The last action and the publication age belong to the
		// tracker: NoteAction owns the text, the encoders age the
		// publication stamp of SetHuntDiagnostics.
		LastAction:      "",
		LastActionAgoMs: 0,
		TickAgoMs:       0,
	}
	// The stuck watchdog and the trip clock only run while the loop
	// follows a planned walk: outside those phases the residual
	// stamps would grow into misleading ages for a report.
	if l.walkPhase() {
		report.StuckForMs = state.AgeMs(since(now, l.stuckAt))
	}
	if l.tripActive() || l.phase == phaseDelevel {
		report.TripForMs = state.AgeMs(since(now, l.tripStart))
	}

	return report
}

// walkPhase reports whether the loop follows a planned geodata walk
// right now: the phases whose diagnostics carry the stuck watchdog
// age (the sell stop stands still by design, the hunt phases have no
// planned path).
func (l *Loop) walkPhase() bool {
	switch l.phase {
	case phaseTownWalk, phaseTownReturn, phaseDelevel:

		return true
	default:

		return false
	}
}

// since returns the elapsed duration of a timestamp, zero for the
// zero time (the never happened reference of the diagnostics ages).
func since(now time.Time, at time.Time) time.Duration {
	if at.IsZero() {
		return 0
	}

	return now.Sub(at)
}

// activeSkipCount counts the targets the searches currently skip:
// an inflated count is stuck evidence (the refused forced attacks of
// a stale server side selection, the timed out pickups of protected
// drops).
func (l *Loop) activeSkipCount(now time.Time) int {
	count := 0
	for _, until := range l.targetSkip {
		if now.Before(until) {
			count++
		}
	}
	for _, until := range l.skipped {
		if now.Before(until) {
			count++
		}
	}

	return count
}

// remainingWaypoints counts the waypoints of the walk the loop
// follows right now: the manual plan of the user phase, the geodata
// plan of the town and delevel phases.
func (l *Loop) remainingWaypoints() int {
	switch l.phase {
	case phaseUser:
		if l.userKind != state.CommandMove {
			return 0
		}

		return max(len(l.userWaypoints)-l.userWpIndex, 0)
	case phaseTownWalk, phaseTownReturn, phaseDelevel:

		return max(len(l.waypoints)-l.wpIndex, 0)
	default:

		return 0
	}
}
