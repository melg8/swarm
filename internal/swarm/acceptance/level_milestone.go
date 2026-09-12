// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The level milestone scenario identity: the account is the next free
// temp slot after the soak temp7, the password equals the account (the
// Mobius login auto-creates missing accounts).
const (
	milestoneScenarioID = "level-milestone"
	milestoneAccount    = "temp8"
	milestonePassword   = "temp8"
)

// milestoneDefaultLevel is the default injected level: 10 sits in the
// middle of the M1 ladder (the elven registry bands below it stay
// winnable for a fresh dressed character, the liren band above it
// stays out of reach), so the default run exercises the full
// building block - the injection, the shopping round, the zone walk
// and the level-up - at a moderate cost. Later milestones inject
// their own level through the env variable.
const milestoneDefaultLevel = 10

// milestoneMaxLevel is the highest injectable level: the elven
// fighter vitals table covers the levels of the solo ladder the M1
// milestone tops out at (the liren 16-19 band leaves the character
// at 20), and the experience table holds the matching thresholds.
const milestoneMaxLevel = 20

// milestoneTimeout bounds the whole run: the weapon run shopping
// round measured 2.2 min, the walk to the mid bands a few more
// minutes, the near-threshold experience needs only a handful of
// kills and a death mid-run pays the penalty and farms back - a
// quarter of an hour keeps the slowest honest run inside the bound
// (the same budget shape the zone return scenario uses).
const milestoneTimeout = 15 * time.Minute

// The injected wallet of the milestone start state: the same values
// the farm readiness scenario uses (the shopping strategy reaches
// the full gear set of the elven village shops with the 100k wallet,
// the 20k sp buys the affordable lessons of the band).
const (
	milestoneWallet = 100000
	milestoneSP     = 20000
)

// milestoneXPTailDivisor shapes the near-threshold experience: the
// injected character starts one twentieth of its level span below
// the next threshold, so a handful of kills of the band mobs (the
// Kaboo Orc of the elven lands carries exp 176 at rate 1, the span
// at level 10 is 22972) reaches the level-up within minutes instead
// of the tens of kills the full span would demand.
const milestoneXPTailDivisor = 20

// checkMilestoneLevel is the condition id of the level-up check.
const checkMilestoneLevel = "level"

// milestoneLevel reads the injected level from the
// SWARM_LEVEL_MILESTONE_LEVEL environment variable (1..20); a missing
// or invalid value falls back to the default. The env shape mirrors
// the soak duration knob so the later milestone acceptances drive
// the building block without code changes.
func milestoneLevel() int32 {
	raw := os.Getenv("SWARM_LEVEL_MILESTONE_LEVEL")
	if raw == "" {
		return milestoneDefaultLevel
	}
	level, err := strconv.Atoi(raw)
	if err != nil || level < 1 || level > milestoneMaxLevel {
		return milestoneDefaultLevel
	}
	// The 1..milestoneMaxLevel bound is checked above, so the
	// conversion to int32 cannot overflow.
	return int32(level) //nolint:gosec // bounded by the checked range
}

// elvenFighterVitals holds the level-up gain vitals of the elven
// fighter (classId 18), floored to integers: hp, mp and cp per level,
// read from the Mobius C1 player template
// dist/game/data/stats/players/templates/StartingClass/ElvenFighter.xml
// (lvlUpgainData levels 1..20; the level 15 values match the farm
// readiness constants 280/111/112). The scenario injects the values
// of the start level so the database row agrees with the template
// the server would have leveled the character along.
var elvenFighterVitals = [...]vitals{
	{HP: 89, MP: 30, CP: 35},    // level 1
	{HP: 101, MP: 35, CP: 40},   // level 2
	{HP: 114, MP: 41, CP: 45},   // level 3
	{HP: 127, MP: 46, CP: 51},   // level 4
	{HP: 140, MP: 52, CP: 56},   // level 5
	{HP: 154, MP: 57, CP: 61},   // level 6
	{HP: 167, MP: 63, CP: 67},   // level 7
	{HP: 181, MP: 69, CP: 72},   // level 8
	{HP: 194, MP: 75, CP: 77},   // level 9
	{HP: 208, MP: 81, CP: 83},   // level 10
	{HP: 222, MP: 87, CP: 89},   // level 11
	{HP: 236, MP: 93, CP: 94},   // level 12
	{HP: 251, MP: 99, CP: 100},  // level 13
	{HP: 265, MP: 105, CP: 106}, // level 14
	{HP: 280, MP: 111, CP: 112}, // level 15
	{HP: 294, MP: 118, CP: 117}, // level 16
	{HP: 309, MP: 124, CP: 123}, // level 17
	{HP: 324, MP: 131, CP: 129}, // level 18
	{HP: 339, MP: 137, CP: 135}, // level 19
	{HP: 355, MP: 144, CP: 142}, // level 20
}

// vitals is one level row of the elven fighter gain table.
type vitals struct {
	HP int32
	MP int32
	CP int32
}

// milestoneXPTail returns the experience carved off the next level
// threshold: one milestoneXPTailDivisor of the level span, never
// less than 1 so the injected exp always sits strictly below the
// threshold (a character exactly at the threshold would be the next
// level already).
func milestoneXPTail(level int32) int64 {
	threshold := soakExperienceTable[level]
	span := threshold - soakExperienceTable[level-1]
	tail := span / milestoneXPTailDivisor
	if tail < 1 {
		tail = 1
	}

	return tail
}

// levelMilestoneReset builds the start state of the level milestone
// scenario: the character at the requested level with the near
// threshold experience, the wallet and the sp of the farm readiness
// round, an empty bag (the shopping strategy dresses the character
// for its band) and the creation spawn point of the elven village
// (the zone ladder walks it out to the hunting ground on its own).
func levelMilestoneReset(account string, level int32) characterReset {
	gain := elvenFighterVitals[level-1]

	return characterReset{
		Account: account,
		Char:    account,
		Level:   level,
		Exp:     soakExperienceTable[level] - milestoneXPTail(level),
		SP:      milestoneSP,
		Adena:   milestoneWallet,
		Items:   nil,
		X:       elvenSpawnX,
		Y:       elvenSpawnY,
		Z:       elvenSpawnZ,
		MaxHP:   gain.HP,
		MaxMP:   gain.MP,
		MaxCP:   gain.CP,
	}
}

// levelMilestoneScenario runs the generalized farm readiness: a
// character injected at level N (env selectable, default 10) with the
// near-threshold experience and the shopping wallet enters the world,
// dresses itself through the village shops, walks to its band hunting
// ground and farms the last kills to the level-up. The pass line is
// the observed level N+1 (the UserInfo the server broadcasts on the
// level-up refreshes the tracker level); the metrics trail receives
// one row either way.
func levelMilestoneScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(levelMilestoneChecks())
	level := milestoneLevel()
	test.appendLog(fmt.Sprintf(
		"acceptance: the level milestone target is %d -> %d",
		level, level+1))

	if err := m.ensureCharacter(milestoneAccount, milestonePassword,
		milestoneAccount, test.appendLog); err != nil {
		writeMilestoneFailMetrics(test, level, err.Error())

		return fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	reset := levelMilestoneReset(milestoneAccount, level)
	if err := m.injectReset(reset, test); err != nil {
		writeMilestoneFailMetrics(test, level, err.Error())

		return fmt.Errorf("inject start state: %w", err)
	}
	time.Sleep(ensurePause)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSessionSupervised(sessionCtx, milestoneAccount,
			milestonePassword, milestoneAccount, true, m.proxy,
			test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		return milestoneFail(test, level, err, cancelSession, sessionDone)
	}

	start := soakRunStart{
		at:    time.Now(),
		level: tracker.SelfLevel(),
		cumulative: cumulativeSoakXP(tracker.SelfLevel(),
			tracker.SelfExp()),
		rePaths: soakRePaths(tracker),
	}
	deaths := newDeathEdgeTracker()
	test.appendLog(fmt.Sprintf("acceptance: the bot is in the world at "+
		"level %d, watching the level-up", start.level))

	return milestoneWatch(ctx, test, tracker, level, &start, deaths,
		cancelSession, sessionDone)
}

// milestoneWatch observes the session until the character reaches the
// target level: every monitor tick refreshes the level check and the
// death edge tracker; the reached level hands over to the pass
// teardown, the context end (the manager timeout among others) fails
// the run.
func milestoneWatch(
	ctx context.Context, test *Test, tracker *state.Bot, level int32,
	start *soakRunStart, deaths *deathEdgeTracker,
	cancelSession context.CancelFunc, sessionDone chan error,
) error {
	for {
		if ctx.Err() != nil {
			return milestoneFail(test, level,
				fmt.Errorf("timeout before the level-up: %w", ctx.Err()),
				cancelSession, sessionDone)
		}
		deaths.update(tracker)
		evaluateMilestoneLevel(tracker, test, level)
		if tracker.SelfLevel() >= level+1 {
			return milestonePass(test, tracker, start, deaths,
				cancelSession, sessionDone)
		}
		select {
		case <-ctx.Done():
			return milestoneFail(test, level,
				fmt.Errorf("timeout before the level-up: %w", ctx.Err()),
				cancelSession, sessionDone)
		case <-time.After(monitorPeriod):
		}
	}
}

// milestonePass stops the session at the reached level and writes the
// PASS metrics row; a session teardown error flips the outcome.
func milestonePass(
	test *Test, tracker *state.Bot, start *soakRunStart,
	deaths *deathEdgeTracker, cancelSession context.CancelFunc,
	sessionDone chan error,
) error {
	test.appendLog(fmt.Sprintf(
		"acceptance: the character reached level %d, stopping the bot",
		tracker.SelfLevel()))
	cancelSession()
	sessionErr := <-sessionDone
	if sessionErr != nil {
		writeMilestoneOutcomeMetrics(test, tracker, start,
			time.Since(start.at), deaths.deaths(), true,
			"session end: "+sessionErr.Error())

		return fmt.Errorf("session end: %w", sessionErr)
	}
	test.appendLog("acceptance: the bot left the world gracefully")
	writeMilestoneOutcomeMetrics(test, tracker, start,
		time.Since(start.at), deaths.deaths(), false, "")

	return nil
}

// milestoneFail tears the supervised session down (when it was
// launched), writes the FAIL metrics row and returns the error as the
// scenario verdict.
func milestoneFail(
	test *Test, level int32, err error,
	cancel context.CancelFunc, done chan error,
) error {
	if cancel != nil {
		cancel()
		<-done
	}
	writeMilestoneFailMetrics(test, level, err.Error())

	return err
}

// evaluateMilestoneLevel refreshes the level-up check from the live
// tracker state: the detail line carries the observed level and the
// experience distance to the threshold.
func evaluateMilestoneLevel(tracker *state.Bot, test *Test, level int32) {
	if tracker.Status() != state.StatusOnline {
		return
	}
	current := tracker.SelfLevel()
	if current >= level+1 {
		test.updateCheck(checkMilestoneLevel, true,
			fmt.Sprintf("reached level %d", current))

		return
	}
	threshold := soakExperienceTable[level]
	distance := threshold - cumulativeSoakXP(current, tracker.SelfExp())
	test.updateCheck(checkMilestoneLevel, false,
		fmt.Sprintf("level %d, %d xp to level %d", current, distance,
			level+1))
}

// levelMilestoneChecks is the check list of the level milestone
// scenario: the world entry plus the level-up itself.
func levelMilestoneChecks() []Check {
	return []Check{
		{
			ID: checkOnline, Label: labelEnteredWorld, Done: false,
			Detail: "",
		},
		{
			ID: checkMilestoneLevel, Label: "reached the next level",
			Done: false, Detail: "",
		},
	}
}

// writeMilestoneOutcomeMetrics appends the metrics row of a finished
// milestone run (pass or fail): the same trail shape the soak
// scenario writes, with the milestone scenario label.
func writeMilestoneOutcomeMetrics(
	test *Test, tracker *state.Bot, start *soakRunStart,
	elapsed time.Duration, deathCount int, failed bool, reason string,
) {
	status := statusPass
	if failed {
		status = statusFail
	}
	endCum := cumulativeSoakXP(tracker.SelfLevel(), tracker.SelfExp())
	writeSoakMetrics(test, soakMetrics{
		Date:        nowUTC(),
		Scenario:    milestoneScenarioID,
		DurationSec: int64(elapsed / time.Second),
		StartLevel:  start.level,
		EndLevel:    tracker.SelfLevel(),
		XpPerHour:   xpPerHour(start.cumulative, endCum, elapsed),
		Deaths:      deathCount,
		Adena:       soakAdena(tracker),
		StuckEvents: soakRePaths(tracker) - start.rePaths,
		Status:      status,
		FailReason:  reason,
	})
}

// writeMilestoneFailMetrics appends the metrics row of a run that
// never reached the world (a setup or injection failure): the fail
// reason rides along, the numeric fields stay zero.
func writeMilestoneFailMetrics(test *Test, level int32, reason string) {
	writeSoakMetrics(test, soakMetrics{
		Date:        nowUTC(),
		Scenario:    milestoneScenarioID,
		DurationSec: 0,
		StartLevel:  level,
		EndLevel:    level,
		XpPerHour:   0,
		Deaths:      0,
		Adena:       0,
		StuckEvents: 0,
		Status:      statusFail,
		FailReason:  reason,
	})
}
