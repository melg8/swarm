// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// fakeClock is the injectable now seam of the stagnation guard tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) get() time.Time { return c.now }

// newSoakBot builds a tracker the guard reads: a level 1 elven
// fighter at the elven village spawn with zero experience. The
// ApplyUserInfo call seeds the level, the exp and the position the
// guard compares; the tests vary the state through ApplyUserInfo
// inside the loops.
const (
	soakTestStartX int32 = 45000
	soakTestStartY int32 = 41000
	soakTestStartZ int32 = -3400
)

// newSoakBot builds the guard test tracker at the elven village spawn.
func newSoakBot() *state.Bot {
	x, y, z := soakTestStartX, soakTestStartY, soakTestStartZ
	bot := state.NewBot(soakAccount)
	bot.SetCharacter(soakAccount, 100, 18, x, y, z, 100, 100)
	bot.ApplyUserInfo(state.UserInfo{
		Name:    soakAccount,
		Level:   1,
		ClassID: 18,
		X:       x,
		Y:       y,
		Z:       z,
		Exp:     0,
		MaxHP:   100,
		CurHP:   100,
		MaxMP:   100,
		CurMP:   100,
	})

	return bot
}

// TestStagnationGuardNoXpFires pins the M-minute XP stagnation: a bot
// that gains no experience past the startup grace trips the guard.
func TestStagnationGuardNoXpFires(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	bot := newSoakBot()
	guard := newStagnationGuard(clock.get)

	// Seed the guard, then walk past the startup grace.
	guard.update(bot)
	clock.now = clock.now.Add(stagnationStartupGrace + time.Second)
	guard.update(bot)

	// No XP change for the full M-minute window.
	clock.now = clock.now.Add(
		stagnationNoXpMinutes * time.Minute)
	guard.update(bot)
	require.True(t, guard.fired(), "the guard must fire after the XP window")
	require.Contains(t, guard.reason(), "no experience gain")
}

// TestStagnationGuardNoMoveFires pins the K-minute position stagnation:
// a bot that holds the same cell past the startup grace trips the guard
// even while its experience keeps moving.
func TestStagnationGuardNoMoveFires(t *testing.T) {
	clock := &fakeClock{now: time.Unix(2000, 0)}
	bot := newSoakBot()
	guard := newStagnationGuard(clock.get)

	guard.update(bot)
	clock.now = clock.now.Add(stagnationStartupGrace + time.Second)

	// Gain XP every tick but never move.
	for i := range stagnationNoMoveMinutes {
		clock.now = clock.now.Add(time.Minute)
		bot.ApplyUserInfo(state.UserInfo{
			Name:    soakAccount,
			Level:   1,
			ClassID: 18,
			X:       45000,
			Y:       41000,
			Z:       -3400,
			Exp:     int32(50 + i),
			MaxHP:   100,
			CurHP:   100,
			MaxMP:   100,
			CurMP:   100,
		})
		guard.update(bot)
	}
	require.True(t, guard.fired(), "the guard must fire after the move window")
	require.Contains(t, guard.reason(), "no position change")
}

// TestStagnationGuardHealthyNeverFires pins the happy path: a bot that
// gains XP and moves every tick never trips the guard.
func TestStagnationGuardHealthyNeverFires(t *testing.T) {
	clock := &fakeClock{now: time.Unix(3000, 0)}
	bot := newSoakBot()
	guard := newStagnationGuard(clock.get)

	guard.update(bot)
	clock.now = clock.now.Add(stagnationStartupGrace + time.Second)

	// Walk and gain XP every minute - well inside both thresholds.
	for i := range 30 {
		clock.now = clock.now.Add(time.Minute)
		bot.ApplyUserInfo(state.UserInfo{
			Name:    soakAccount,
			Level:   1,
			ClassID: 18,
			X:       45000 + int32(i),
			Y:       41000 + int32(i),
			Z:       -3400,
			Exp:     int32(50 + i),
			MaxHP:   100,
			CurHP:   100,
			MaxMP:   100,
			CurMP:   100,
		})
		guard.update(bot)
	}
	require.False(t, guard.fired(), "a healthy bot never trips the guard")
}

// TestStagnationGuardStartupGraceHolds pins the grace: a bot that does
// nothing does not trip the guard before the startup grace elapses.
func TestStagnationGuardStartupGraceHolds(t *testing.T) {
	clock := &fakeClock{now: time.Unix(4000, 0)}
	bot := newSoakBot()
	guard := newStagnationGuard(clock.get)

	guard.update(bot)
	clock.now = clock.now.Add(stagnationStartupGrace -
		10*time.Second)
	guard.update(bot)
	require.False(t, guard.fired(),
		"the guard holds during the startup grace")
}

// TestSoakMetricsJSON pins the field names and the PASS/FAIL encoding
// the renderer and any analysis script depend on.
func TestSoakMetricsJSON(t *testing.T) {
	row := soakMetrics{
		Date:        "2026-09-12T07:27:00Z",
		Scenario:    "soak",
		DurationSec: 600,
		StartLevel:  1,
		EndLevel:    3,
		XpPerHour:   1234.5,
		Deaths:      0,
		Adena:       500,
		StuckEvents: 1,
		Status:      "PASS",
	}
	payload, err := json.Marshal(row)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"scenario":"soak"`)
	require.Contains(t, string(payload), `"status":"PASS"`)
	require.NotContains(t, string(payload), `"failReason"`,
		"an empty failReason is omitted")

	fail := row
	fail.Status = "FAIL"
	fail.FailReason = "stagnation: no XP"
	payload, err = json.Marshal(fail)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"status":"FAIL"`)
	require.Contains(t, string(payload), `"failReason":"stagnation: no XP"`)
}

// TestAppendMetricsWritesOneLine pins the atomic single-line append:
// two rows produce exactly two newline-terminated JSON lines.
func TestAppendMetricsWritesOneLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.jsonl")
	for i := range 2 {
		err := appendMetrics(path, soakMetrics{
			Date:       "2026-09-12T07:27:00Z",
			Scenario:   "soak",
			Status:     "PASS",
			StartLevel: int32(i),
			EndLevel:   int32(i + 1),
		})
		require.NoError(t, err)
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := splitLines(string(data))
	require.Len(t, lines, 2, "exactly two metrics lines")
	var row soakMetrics
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &row))
	require.Equal(t, "PASS", row.Status)
}

// TestXpPerHour pins the hourly rate computation.
func TestXpPerHour(t *testing.T) {
	require.InDelta(t, 0.0, xpPerHour(0, 0, 0), 0.0)
	require.InDelta(t, 0.0, xpPerHour(100, 100, time.Hour), 0.0)
	require.InDelta(t, 1800.0,
		xpPerHour(0, 3600, 2*time.Hour), 0.001)
}

// TestCumulativeSoakXP pins the table lookup against the known C1
// values: level 1 is zero, level 2 is 68, the cumulative adds the
// within-level exp, and a sub-level-1 character returns the raw exp.
func TestCumulativeSoakXP(t *testing.T) {
	// The exp the tracker carries is the cumulative total (the
	// UserInfo packet broadcasts player.getExp()), so the cumulative
	// view is the exp itself whatever the level is.
	require.Equal(t, int64(0), cumulativeSoakXP(1, 0))
	require.Equal(t, int64(0), cumulativeSoakXP(2, 0))
	require.Equal(t, int64(100), cumulativeSoakXP(2, 100))
	require.Equal(t, int64(50), cumulativeSoakXP(0, 50))
	require.Equal(t, int64(363), cumulativeSoakXP(3, 363))
}

// TestDeathEdgeTrackerCountsRisingEdges pins the death count: two
// alive->dead transitions produce a count of two.
func TestDeathEdgeTrackerCountsRisingEdges(t *testing.T) {
	bot := state.NewBot(soakAccount)
	bot.SetCharacter(soakAccount, 100, 18, 100, 100, 100, 100, 100)
	tracker := newDeathEdgeTracker()

	// First update seeds the previous flag (alive).
	bot.ApplyUserInfo(state.UserInfo{
		Name: soakAccount, Level: 1, ClassID: 18,
		MaxHP: 100, CurHP: 100, MaxMP: 100, CurMP: 100,
	})
	tracker.update(bot)
	require.Equal(t, 0, tracker.deaths())

	// Die once.
	bot.ApplyUserInfo(state.UserInfo{
		Name: soakAccount, Level: 1, ClassID: 18,
		MaxHP: 100, CurHP: 0, MaxMP: 100, CurMP: 100,
	})
	tracker.update(bot)
	require.Equal(t, 1, tracker.deaths())

	// Revive.
	bot.ApplyUserInfo(state.UserInfo{
		Name: soakAccount, Level: 1, ClassID: 18,
		MaxHP: 100, CurHP: 100, MaxMP: 100, CurMP: 100,
	})
	tracker.update(bot)
	require.Equal(t, 1, tracker.deaths())

	// Die again.
	bot.ApplyUserInfo(state.UserInfo{
		Name: soakAccount, Level: 1, ClassID: 18,
		MaxHP: 100, CurHP: 0, MaxMP: 100, CurMP: 100,
	})
	tracker.update(bot)
	require.Equal(t, 2, tracker.deaths())
}

// TestSoakDurationEnv pins the env override and the default fallback.
func TestSoakDurationEnv(t *testing.T) {
	require.Equal(t, soakDefaultMinutes*time.Minute,
		soakDuration())
	t.Setenv("SWARM_SOAK_MINUTES", "3")
	require.Equal(t, 3*time.Minute, soakDuration())
	t.Setenv("SWARM_SOAK_MINUTES", "garbage")
	require.Equal(t, soakDefaultMinutes*time.Minute,
		soakDuration())
	t.Setenv("SWARM_SOAK_MINUTES", "0")
	require.Equal(t, soakDefaultMinutes*time.Minute,
		soakDuration())
}

// splitLines splits the metrics file payload into the non-empty lines.
func splitLines(value string) []string {
	var lines []string
	start := 0
	for i := range len(value) {
		if value[i] == '\n' {
			if start < i {
				lines = append(lines, value[start:i])
			}
			start = i + 1
		}
	}
	if start < len(value) {
		lines = append(lines, value[start:])
	}

	return lines
}
