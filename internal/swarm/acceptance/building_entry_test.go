// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The building entry scenario tests: the trainer hall round of the
// 2026-09-12 03:56 freeze dump (build 6a2ac91, bot temp1, phase
// townWalk) - the character wakes at the west aisle entrance of the
// trainer hall with the dump inventory and the learning trip armed,
// walks into the building right up to the teacher Ellenia and learns.

// TestBuildingEntryDefinition pins the scenario registration: the
// trainer hall round owns temp5, runs under the eight minute bound
// and carries the dump story.
func TestBuildingEntryDefinition(t *testing.T) {
	var found bool
	for _, def := range Definitions() {
		if def.ID != "building-entry" {
			continue
		}
		found = true
		require.Equal(t, entryAccount, def.Account)
		require.Equal(t, buildingEntryTimeout, def.Timeout)
		require.NotNil(t, def.Scenario)
		for _, needle := range []string{
			"44744 51992 -2792", "level 15", "20,000 SP",
			"100,000 adena", "Ellenia", "lesson",
		} {
			require.Contains(t, def.Description, needle)
		}
	}
	require.True(t, found, "the building-entry scenario is registered")
}

// TestBuildingEntryResetMatchesTheDump pins the injected start state
// of the report: the aisle entrance position, the level, the shopping
// wallet and the empty inventory of the weapon run round.
func TestBuildingEntryResetMatchesTheDump(t *testing.T) {
	reset := buildingEntryReset(entryAccount)
	require.Equal(t, int32(15), reset.Level)
	require.Equal(t, reset.Exp, int64(level15Exp))
	require.Equal(t, int64(20000), reset.SP)
	require.Equal(t, int64(100000), reset.Adena)
	require.Empty(t, reset.Items,
		"the weapon run round starts with the empty inventory")
	require.Equal(t, int32(44744), reset.X)
	require.Equal(t, int32(51992), reset.Y)
	require.Equal(t, int32(-2792), reset.Z)
	require.Equal(t, int32(358), reset.MaxHP)
	require.Equal(t, int32(145), reset.MaxMP)
}

// TestBuildingEntryChecksCoverTheStory pins the check list of the
// scenario: the world entry at the entrance, the walk up to the
// teacher and the first learned lesson.
func TestBuildingEntryChecksCoverTheStory(t *testing.T) {
	checks := buildingEntryChecks()
	require.Len(t, checks, 3)
	require.Equal(t, checkOnline, checks[0].ID)
	require.Equal(t, checkTeacher, checks[1].ID)
	require.Equal(t, checkLesson, checks[2].ID)
	for _, check := range checks {
		require.NotEmpty(t, check.Label)
		require.False(t, check.Done)
	}
}

// TestEvaluateTeacherPinsTheInteractionRing verifies the teacher
// distance check: the dump freeze position answers false, the offset
// ring inside the hall answers true.
func TestEvaluateTeacherPinsTheInteractionRing(t *testing.T) {
	bot := state.NewBot(entryAccount)
	bot.SetCharacter(entryAccount, 100, 18,
		entrySpawnX, entrySpawnY, entrySpawnZ, entryHP, entryMP)
	// The aisle entrance of the dump: ~1010 units from Ellenia.
	_, done := evaluateTeacher(bot)
	require.False(t, done, "the entrance position is far from the teacher")

	// The close ring inside the hall: 150 units from Ellenia.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100,
		X:        45576, Y: 52104, Z: elleniaSpawnZ,
		DestX: 45576, DestY: 52104, DestZ: elleniaSpawnZ,
	})
	detail, done := evaluateTeacher(bot)
	require.True(t, done, "the close ring position reaches the teacher")
	require.Contains(t, detail, "units from Ellenia")
}

// TestEvaluateLessonPinsTheSpDrop verifies the lesson check: the
// injected start SP answers false, the drop after the first lesson
// answers true.
func TestEvaluateLessonPinsTheSpDrop(t *testing.T) {
	bot := state.NewBot(entryAccount)
	bot.SetCharacter(entryAccount, 100, 18,
		entrySpawnX, entrySpawnY, entrySpawnZ, entryHP, entryMP)
	_, done := evaluateLesson(bot)
	require.False(t, done, "the fresh tracker has no SP answer yet")

	bot.ApplyUserInfo(state.UserInfo{
		Name:  entryAccount,
		Level: 15,
		Sp:    entrySP,
	})
	_, done = evaluateLesson(bot)
	require.False(t, done, "the start state holds the full SP")

	bot.ApplyUserInfo(state.UserInfo{
		Name:  entryAccount,
		Level: 15,
		Sp:    entrySP - 320,
	})
	detail, done := evaluateLesson(bot)
	require.True(t, done, "the SP drop proves a learned lesson")
	require.Contains(t, detail, "the lessons began")
}
