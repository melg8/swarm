// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// TestLevelMilestoneResetNearThreshold: the injected start state of
// the default level carries the near-threshold experience, the
// template vitals, the wallet and the creation spawn point.
func TestLevelMilestoneResetNearThreshold(t *testing.T) {
	reset := levelMilestoneReset(milestoneAccount, 10)

	require.Equal(t, milestoneAccount, reset.Account)
	require.Equal(t, int32(10), reset.Level)
	require.Equal(t, int64(milestoneWallet), reset.Adena)
	require.Equal(t, int64(milestoneSP), reset.SP)
	require.Equal(t, int32(elvenSpawnX), reset.X)
	require.Equal(t, int32(elvenSpawnY), reset.Y)
	require.Equal(t, int32(elvenSpawnZ), reset.Z)
	require.Empty(t, reset.Items)

	// The experience sits strictly inside the level 10 span and
	// within the last twentieth of it.
	threshold := soakExperienceTable[10]
	tail := milestoneXPTail(10)
	require.Equal(t, reset.Exp, threshold-tail)
	require.Less(t, reset.Exp, threshold)
	require.Greater(t, reset.Exp, soakExperienceTable[9])

	// The vitals match the elven fighter template gains.
	require.Equal(t, elvenFighterVitals[9].HP, reset.MaxHP)
	require.Equal(t, elvenFighterVitals[9].MP, reset.MaxMP)
	require.Equal(t, elvenFighterVitals[9].CP, reset.MaxCP)
}

// TestLevelMilestoneTailScalesWithSpan: every injectable level gets a
// strictly positive tail below one twentieth of its span, so the
// character is always one short stretch of kills from the level-up.
func TestLevelMilestoneTailScalesWithSpan(t *testing.T) {
	for level := int32(1); level <= milestoneMaxLevel; level++ {
		span := soakExperienceTable[level] -
			soakExperienceTable[level-1]
		tail := milestoneXPTail(level)
		require.GreaterOrEqual(t, tail, int64(1), "level %d", level)
		require.LessOrEqual(t, tail, span/milestoneXPTailDivisor,
			"level %d", level)

		reset := levelMilestoneReset(milestoneAccount, level)
		require.Less(t, reset.Exp, soakExperienceTable[level],
			"level %d", level)
		require.GreaterOrEqual(t, reset.Exp,
			soakExperienceTable[level-1], "level %d", level)
		require.Equal(t, elvenFighterVitals[level-1].HP, reset.MaxHP,
			"level %d", level)
	}
}

// TestMilestoneLevelFromEnv: the level knob reads the env variable,
// rejects the out-of-range and malformed values and falls back to
// the default.
func TestMilestoneLevelFromEnv(t *testing.T) {
	require.Equal(t, int32(milestoneDefaultLevel), milestoneLevel())

	t.Setenv("SWARM_LEVEL_MILESTONE_LEVEL", "16")
	require.Equal(t, int32(16), milestoneLevel())

	t.Setenv("SWARM_LEVEL_MILESTONE_LEVEL", "1")
	require.Equal(t, int32(1), milestoneLevel())

	t.Setenv("SWARM_LEVEL_MILESTONE_LEVEL", "20")
	require.Equal(t, int32(20), milestoneLevel())

	t.Setenv("SWARM_LEVEL_MILESTONE_LEVEL", "0")
	require.Equal(t, int32(milestoneDefaultLevel), milestoneLevel())

	t.Setenv("SWARM_LEVEL_MILESTONE_LEVEL", "21")
	require.Equal(t, int32(milestoneDefaultLevel), milestoneLevel())

	t.Setenv("SWARM_LEVEL_MILESTONE_LEVEL", "not-a-number")
	require.Equal(t, int32(milestoneDefaultLevel), milestoneLevel())
}

// TestEvaluateMilestoneLevel: the check evaluation reports the
// distance while the level is below the target and flips to done at
// the target level.
func TestEvaluateMilestoneLevel(t *testing.T) {
	bot := state.NewBot("temp8")
	bot.SetOnline("temp8")
	bot.ApplyUserInfo(state.UserInfo{
		Name: "temp8", Level: 10, ClassID: 18, Race: 1,
		X: elvenSpawnX, Y: elvenSpawnY, Z: elvenSpawnZ, Exp: 70053,
		MaxHP: 208, CurHP: 200, MaxMP: 81, CurMP: 70,
	})
	test := newMilestoneTest()

	evaluateMilestoneLevel(bot, test, 10)
	check := findCheck(test, checkMilestoneLevel)
	require.False(t, check.Done)
	require.Contains(t, check.Detail, "level 10")
	require.Contains(t, check.Detail, "xp to level 11")

	// The level-up broadcast refreshed the level.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "temp8", Level: 11, ClassID: 18, Race: 1,
		X: elvenSpawnX, Y: elvenSpawnY, Z: elvenSpawnZ, Exp: 71201,
		MaxHP: 222, CurHP: 215, MaxMP: 87, CurMP: 80,
	})
	evaluateMilestoneLevel(bot, test, 10)
	check = findCheck(test, checkMilestoneLevel)
	require.True(t, check.Done)
	require.Contains(t, check.Detail, "reached level 11")
}

// TestMilestoneChecksList: the check list starts with the world entry
// and the level-up condition.
func TestMilestoneChecksList(t *testing.T) {
	checks := levelMilestoneChecks()
	require.Len(t, checks, 2)
	require.Equal(t, checkOnline, checks[0].ID)
	require.Equal(t, checkMilestoneLevel, checks[1].ID)
	require.False(t, checks[0].Done)
	require.False(t, checks[1].Done)
}

// TestElvenFighterVitalsMatchTheTemplate: the level 15 row agrees
// with the farm readiness constants (280/111/112) and the level 1
// row with the template base gains (89/30/35.6 floored).
func TestElvenFighterVitalsMatchTheTemplate(t *testing.T) {
	require.Equal(t, int32(level15HP), elvenFighterVitals[14].HP)
	require.Equal(t, int32(level15MP), elvenFighterVitals[14].MP)
	require.Equal(t, int32(level15CP), elvenFighterVitals[14].CP)

	require.Equal(t, vitals{HP: 89, MP: 30, CP: 35}, elvenFighterVitals[0])
}

// TestMilestoneScenarioRegistered: the definition list carries the
// level milestone scenario with its account and timeout.
func TestMilestoneScenarioRegistered(t *testing.T) {
	var found bool
	for _, def := range Definitions() {
		if def.ID == milestoneScenarioID {
			found = true
			require.Equal(t, milestoneAccount, def.Account)
			require.Equal(t, milestoneTimeout, def.Timeout)
			require.NotNil(t, def.Scenario)
		}
	}
	require.True(t, found, "the level milestone scenario is registered")
}

// newMilestoneTest builds a bare test shell the check evaluation can
// write into.
func newMilestoneTest() *Test {
	return &Test{
		def: TestDef{
			ID:      milestoneScenarioID,
			Timeout: milestoneTimeout,
		},
		checks: levelMilestoneChecks(),
	}
}

// findCheck returns the check view of the given id.
func findCheck(test *Test, id string) Check {
	test.mu.Lock()
	defer test.mu.Unlock()
	for i := range test.checks {
		if test.checks[i].ID == id {
			return test.checks[i]
		}
	}

	return Check{}
}
