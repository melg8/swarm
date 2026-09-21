// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The diagnostics publication of the hunt loop, split out of the
// loop.go god file: the internals the state dump carries for the
// stuck bot reports and the decision log routing.

import (
    "fmt"
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The ETA model constants of the hunt diagnostics: the walk ETA
// divides the remaining plan length by the run speed, the kill ETA
// divides the observed damage rate into the remaining health.
const (
    // walkEtaDefaultSpeed is the fallback run speed of the walk ETA
    // in world units per second: a character whose speeds the server
    // never sent still gets an estimate (the plain monster default
    // of the state package).
    walkEtaDefaultSpeed = 120.0
    // killEtaMinElapsed is the fight age below which the damage rate
    // estimate is noise: one swing either way flips it.
    killEtaMinElapsed = 2 * time.Second
)

// logf reports a hunt loop decision on the console logger and the
// bot tracker: the message becomes an event log entry of the state
// dump (the live server report carries the decision history next to
// the world data) and the last action of the hunt diagnostics view.
// The event log entry arrives through the logger mirror of the wiring
// (huntEventLogger of cmd/swarm, sessionLogger of the acceptance
// runner); the last action update carries no record of its own - the
// old NoteAction here recorded the same line a second time and every
// Hunt: decision landed twice in the ring, the journal and the web UI.
func (l *Loop) logf(format string, args ...any) {
    message := fmt.Sprintf(format, args...)
    l.logger.Print(message)
    l.tracker.NoteLastAction(message)
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
        LastAction:        "",
        LastActionAgoMs:   0,
        DelevelActive:     false,
        DelevelTarget:     0,
        DelevelFromLevel:  0,
        DelevelZoneMedian: 0,
        TickAgoMs:         0,
        WalkEtaMs:         etaMs(l.walkEtaSeconds()),
        KillEtaMs:         etaMs(l.killEtaSeconds(now)),
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
    // The deleveling message of the webui: the phase banner renders
    // the target level and the trigger evidence (the start level
    // against the median mob level of the held ground) from these
    // fields. The zero view outside the phase clears the message.
    if l.phase == phaseDelevel {
        report.DelevelActive = true
        report.DelevelTarget = l.delevelTarget
        report.DelevelFromLevel = l.delevelLevel
        report.DelevelZoneMedian = l.delevelMedian
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

// walkEtaSeconds estimates the walking time left on the planned
// walk: the straight line length from the character through the
// remaining waypoints into the walk destination, divided by the run
// speed (world units per second). Zero when no walk is running (the
// phases without a plan, an unknown position, nothing left to walk).
func (l *Loop) walkEtaSeconds() float64 {
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return 0
    }
    speed := l.tracker.SelfRunSpeed()
    if speed <= 0 {
        speed = walkEtaDefaultSpeed
    }
    distance := l.walkRemainingDistance(float64(selfX), float64(selfY))
    if distance <= 0 {
        return 0
    }

    return distance / speed
}

// walkRemainingDistance measures the remaining walk length of the
// running walk: the character to the first unvisited waypoint, the
// waypoints chained, the last leg into the walk destination. The
// phases without a planned walk report zero.
func (l *Loop) walkRemainingDistance(selfX, selfY float64) float64 {
    switch l.phase {
    case phaseTownWalk, phaseTownReturn, phaseDelevel:

        return walkPlanDistance(selfX, selfY, l.waypoints, l.wpIndex,
            l.segmentDest)
    case phaseUser:
        if l.userKind != state.CommandMove {
            return 0
        }
        dest := pathfind.Vec3{
            X: float64(l.userX),
            Y: float64(l.userY),
            Z: float64(l.userZ),
        }

        return walkPlanDistance(selfX, selfY, l.userWaypoints,
            l.userWpIndex, dest)
    default:

        return 0
    }
}

// walkPlanDistance sums the straight 2D legs of a walk plan: the
// character through the remaining waypoints into the destination.
// An unarmed destination (the zero sentinel the trip resets restore)
// contributes no leg, so a plan walked to its end measures zero. The
// cursor clamps into the slice bounds - the transient reset races
// are a diagnostics input, never a walk decision.
func walkPlanDistance(
    selfX, selfY float64, waypoints []pathfind.Vec3, index int,
    dest pathfind.Vec3,
) float64 {
    fromX, fromY := selfX, selfY
    total := 0.0
    for _, wp := range waypoints[min(index, len(waypoints)):] {
        total += math.Hypot(wp.X-fromX, wp.Y-fromY)
        fromX, fromY = wp.X, wp.Y
    }
    if dest.X != 0 || dest.Y != 0 {
        total += math.Hypot(dest.X-fromX, dest.Y-fromY)
    }

    return total
}

// killEtaSeconds estimates the time until the current fight target
// dies: the damage the fight has done so far over its age is the
// rate, the remaining health over the rate the estimate. Zero when
// the fight is not confirmed running (the stamped start keyed by the
// target), the fight is too fresh for a stable rate (one swing
// either way flips it below killEtaMinElapsed), the target healed
// back above its start health or the vitals are unknown.
func (l *Loop) killEtaSeconds(now time.Time) float64 {
    if l.target == 0 || l.fightStartFor != l.target ||
        l.fightStartAt.IsZero() {
        return 0
    }
    elapsed := now.Sub(l.fightStartAt)
    if elapsed < killEtaMinElapsed {
        return 0
    }
    curHp, maxHp, ok := l.tracker.ObjectVitals(l.target)
    if !ok || maxHp <= 0 {
        return 0
    }
    damage := maxHp - curHp
    if damage <= 0 || curHp <= 0 {
        return 0
    }
    rate := damage / elapsed.Seconds()
    if rate <= 0 {
        return 0
    }

    return curHp / rate
}

// etaMs floors an estimated duration to whole seconds reported as
// milliseconds: the resolution of the ETA fields (the same flooring
// the age fields of the state package use). A sub second or negative
// estimate reports zero.
func etaMs(seconds float64) int64 {
    if seconds <= 0 {
        return 0
    }

    return state.AgeMs(time.Duration(seconds * float64(time.Second)))
}
