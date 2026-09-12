// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The metrics status values and the shared label of the world entry
// check (the string literal repeats across the check lists, so the
// goconst gate holds it as a constant).
const (
	statusPass        = "PASS"
	statusFail        = "FAIL"
	labelEnteredWorld = "entered the world"
	soakScenarioID    = "soak"
)

// soakAccount is the temp account of the soak scenario: the next free
// slot after the building entry temp6, so the parallel run all still
// does not collide. The password equals the account (the Mobius login
// auto-creates missing accounts).
const (
	soakAccount  = "temp7"
	soakPassword = "temp7"
)

// soakDefaultMinutes is the smoke duration: short enough to fit a 2h
// session dev cycle (the unit tests run faster, the live smoke is the
// ~10 minute window the metrics trail validates against). The real M1
// run sets SWARM_SOAK_MINUTES=480 (8 hours) as a follow-up operator
// action; the scenario itself is duration-agnostic.
const soakDefaultMinutes = 10

// soakShutdownGrace is the budget the scenario gives the supervised
// session to unwind after the soak window: the logout announcement,
// the combat stance delay and the socket flush. The manager's own
// restartWait already covers the worst case; this grace is the local
// bound the pass criteria read against.
const soakShutdownGrace = 90 * time.Second

// soakDuration reads the SWARM_SOAK_MINUTES env and returns the soak
// window. A missing or invalid value falls back to the smoke default;
// a zero or negative value also falls back so an accidental empty
// string never produces a zero-length run.
func soakDuration() time.Duration {
	minutes := soakDefaultMinutes
	if raw := os.Getenv("SWARM_SOAK_MINUTES"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			minutes = parsed
		}
	}

	return time.Duration(minutes) * time.Minute
}

// soakTimeout computes the test timeout from the soak window: the
// window itself plus the startup (the character ensure, the login
// dial and the world entry) and the shutdown grace. The manager adds
// its own restartWait on top, so this bound only needs to cover the
// honest run.
func soakTimeout() time.Duration {
	return soakDuration() + onlineWait + ensurePause +
		soakShutdownGrace
}

// The soak check ids: the narrative of the M1 acceptance - the bot
// entered the world, stayed online for the whole window, never
// stagnated and left gracefully.
const (
	checkSoakHunted   = "hunted"
	checkSoakStagnant = "no-stagnation"
	checkSoakGraceful = "graceful"
)

// soakChecks is the initial check list of the soak scenario.
func soakChecks() []Check {
	return []Check{
		{
			ID: checkOnline, Label: labelEnteredWorld, Done: false,
			Detail: "",
		},
		{
			ID: checkSoakHunted, Label: "stayed online the soak window",
			Done: false, Detail: "",
		},
		{
			ID:    checkSoakStagnant,
			Label: "never stagnated (no XP, no move)",
			Done:  false, Detail: "",
		},
		{
			ID: checkSoakGraceful, Label: "shut down gracefully",
			Done: false, Detail: "",
		},
	}
}

// soakScenario runs the M1 acceptance vehicle: a fresh level 1
// character enters the world under the autonomous hunt loop and farms
// for the configured soak window (SWARM_SOAK_MINUTES, default the 10
// minute smoke; the real M1 run is 8h). The monitor watches the
// stagnation guard every tick; a trip (no XP for M minutes, no move
// for K minutes) fails the run. On every terminal path the scenario
// appends one JSON line to runs/metrics.jsonl with the M1 fields.
func soakScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(soakChecks())
	duration := soakDuration()

	start, tracker, sessionDone, cancelSession, err := soakSetup(
		ctx, m, test)
	if err != nil {
		soakTeardownSession(cancelSession, sessionDone)
		writeSoakMetrics(test, failSoakMetrics(err.Error()))

		return err
	}
	test.appendLog(fmt.Sprintf("acceptance: the bot is online, watching "+
		"the %s soak window", duration))

	deadline := start.at.Add(duration)
	guard := newStagnationGuard(time.Now)
	deaths := newDeathEdgeTracker()
	monitorCtx, cancelMonitor := context.WithCancel(ctx)
	defer cancelMonitor()
	monitorDone := make(chan soakOutcome, 1)
	go soakMonitor(monitorCtx, tracker, test, guard, deaths,
		&start, deadline, monitorDone)

	return soakAwaitOutcome(ctx, test, tracker, &start, cancelSession,
		sessionDone, cancelMonitor, monitorDone, deaths)
}

// soakSetup ensures the temp character, launches the supervised
// session and waits for the world entry. It returns the captured
// run-start state (the level, the cumulative XP, the re-path count),
// the tracker, the session done channel, the session cancel function
// and any setup error. The caller owns the session teardown on every
// exit path.
func soakSetup(
	ctx context.Context, m *Manager, test *Test,
) (soakRunStart, *state.Bot, chan error, context.CancelFunc, error) {
	if err := m.ensureCharacter(soakAccount, soakPassword,
		soakAccount, test.appendLog); err != nil {
		test.appendLog("acceptance: the character ensure failed: " +
			err.Error())

		return soakRunStart{}, nil, nil, nil,
			fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	sessionCtx, cancelSession := context.WithCancel(ctx)
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSessionSupervised(sessionCtx, soakAccount,
			soakPassword, soakAccount, true, m.proxy,
			test.appendLog)
	}()
	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		return soakRunStart{}, tracker, sessionDone, cancelSession, err
	}
	start := soakRunStart{
		at:    time.Now(),
		level: tracker.SelfLevel(),
		cumulative: cumulativeSoakXP(tracker.SelfLevel(),
			tracker.SelfExp()),
		rePaths: soakRePaths(tracker),
	}

	return start, tracker, sessionDone, cancelSession, nil
}

// soakAwaitOutcome waits for the monitor verdict or the context end,
// then tears down the session and writes the metrics row. The verdict
// is PASS when the monitor reached the deadline without the guard
// firing and the session ended gracefully; any failure (stagnation,
// session error, cancellation) produces a FAIL row with the reason.
func soakAwaitOutcome(
	ctx context.Context, test *Test, tracker *state.Bot,
	start *soakRunStart, cancelSession context.CancelFunc,
	sessionDone chan error, cancelMonitor context.CancelFunc,
	monitorDone chan soakOutcome, deaths *deathEdgeTracker,
) error {
	select {
	case <-ctx.Done():
		cancelMonitor()
		<-monitorDone
		soakTeardownSession(cancelSession, sessionDone)
		writeSoakMetrics(test, failSoakMetrics(
			"cancelled: "+ctx.Err().Error()))

		return fmt.Errorf("cancelled: %w", ctx.Err())
	case outcome := <-monitorDone:
		test.appendLog("acceptance: the soak window ended, stopping the bot")
		cancelSession()
		sessionErr := <-sessionDone
		elapsed := time.Since(start.at)
		finalOutcome := outcome
		if sessionErr != nil && !finalOutcome.failed {
			finalOutcome = soakOutcome{
				failed: true,
				reason: "session end: " + sessionErr.Error(),
			}
		}
		writeSoakOutcomeMetrics(test, tracker, start, elapsed,
			deaths.deaths(), finalOutcome)
		if finalOutcome.failed {
			test.appendLog("acceptance: the soak FAILED - " +
				finalOutcome.reason)

			return errors.New(finalOutcome.reason)
		}
		if sessionErr != nil {
			return fmt.Errorf("session end: %w", sessionErr)
		}
		test.updateCheck(checkSoakGraceful, true,
			"logout announced, no error")
		test.appendLog("acceptance: the session ended gracefully")

		return nil
	}
}

// soakTeardownSession cancels the session context and drains the
// session goroutine. Safe on a nil channel (the setup error path
// never launched the session).
func soakTeardownSession(
	cancel context.CancelFunc, done chan error,
) {
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}

// failSoakMetrics builds a fully populated FAIL metrics row for a
// setup or cancellation error: the bot never entered the world, so
// every numeric field is zero and the fail reason carries the error.
func failSoakMetrics(reason string) soakMetrics {
	return soakMetrics{
		Date:        nowUTC(),
		Scenario:    soakScenarioID,
		DurationSec: 0,
		StartLevel:  0,
		EndLevel:    0,
		XpPerHour:   0,
		Deaths:      0,
		Adena:       0,
		StuckEvents: 0,
		Status:      statusFail,
		FailReason:  reason,
	}
}

// soakRunStart captures the start state of the soak window: the
// timestamp, the level and the cumulative experience (for the hourly
// rate) and the hunt loop re-path count (for the stuck-event delta).
type soakRunStart struct {
	at         time.Time
	level      int32
	cumulative int64
	rePaths    int
}

// soakOutcome is the terminal verdict of the monitor loop: failed
// carries the stagnation reason, the elapsed time is read off the
// start by the caller.
type soakOutcome struct {
	failed bool
	reason string
}

// soakMonitor runs the stagnation guard and the check evaluation until
// the deadline or the context end. The monitor owns the guard and the
// death edge tracker (single goroutine); the outcome channel receives
// one value on exit.
func soakMonitor(
	ctx context.Context, tracker *state.Bot, test *Test,
	guard *stagnationGuard, deaths *deathEdgeTracker,
	start *soakRunStart, deadline time.Time,
	done chan<- soakOutcome,
) {
	defer close(done)
	for {
		if ctx.Err() != nil {
			return
		}
		guard.update(tracker)
		deaths.update(tracker)
		evaluateSoakProgress(test, guard, start)
		if guard.fired() {
			return
		}
		if time.Now().After(deadline) {
			test.updateCheck(checkSoakHunted, true,
				"online for the soak window")
			test.updateCheck(checkSoakStagnant, true,
				"no stagnation event")

			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(monitorPeriod):
		}
	}
}

// evaluateSoakProgress refreshes the hunted check from the elapsed
// time and the stagnation check from the guard state.
func evaluateSoakProgress(
	test *Test, guard *stagnationGuard, start *soakRunStart,
) {
	elapsed := time.Since(start.at)
	test.updateCheck(checkSoakHunted, false,
		fmt.Sprintf("%s of %s online", elapsed.Round(time.Second),
			soakDuration()))
	if guard.fired() {
		test.updateCheck(checkSoakStagnant, false, guard.reason())
	} else {
		test.updateCheck(checkSoakStagnant, true, "no stagnation event")
	}
}

// nowUTC returns the current UTC time as an ISO 8601 string (the date
// field of the metrics row).
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// writeSoakMetrics appends one metrics row and logs the outcome. A
// write error is logged but never masks the scenario verdict (the
// trail is the milestone artifact, not the gate).
func writeSoakMetrics(test *Test, row soakMetrics) {
	path := metricsPath()
	if err := appendMetrics(path, row); err != nil {
		test.appendLog("acceptance: the metrics write failed: " +
			err.Error())

		return
	}
	test.appendLog("acceptance: appended a metrics line to " + path)
}

// writeSoakOutcomeMetrics builds the full metrics row from the live
// tracker, the run start, the elapsed time and the monitor outcome,
// then appends it. The status field is PASS when the outcome did not
// fail, FAIL otherwise; the fail reason rides along when present.
func writeSoakOutcomeMetrics(
	test *Test, tracker *state.Bot, start *soakRunStart,
	elapsed time.Duration, deathCount int, outcome soakOutcome,
) {
	endCum := cumulativeSoakXP(tracker.SelfLevel(), tracker.SelfExp())
	stuckEvents := soakRePaths(tracker) - start.rePaths
	xp := xpPerHour(start.cumulative, endCum, elapsed)
	status := statusPass
	failReason := ""
	if outcome.failed {
		status = statusFail
		failReason = outcome.reason
	}
	row := soakMetrics{
		Date:        nowUTC(),
		Scenario:    soakScenarioID,
		DurationSec: int64(elapsed / time.Second),
		StartLevel:  start.level,
		EndLevel:    tracker.SelfLevel(),
		XpPerHour:   xp,
		Deaths:      deathCount,
		Adena:       soakAdena(tracker),
		StuckEvents: stuckEvents,
		Status:      status,
		FailReason:  failReason,
	}
	writeSoakMetrics(test, row)
}
