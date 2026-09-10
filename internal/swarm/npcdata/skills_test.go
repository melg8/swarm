// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Known skill ids from the generated dictionary: Power Strike (3) is
// the elven fighter starter strike with a distinct description per
// level, Armor Mastery (142) collapses its identical level texts into
// two runs, Weapon Mastery (141) carries a single whole-skill text
// and Lucky (194) is the auto granted death protection.
const (
	powerStrikeSkillID  = 3
	armorMasterySkillID = 142
	unknownSkillID      = 999999999
)

func TestSkillDescription(t *testing.T) {
	t.Run("per level run answers with the exact level", func(t *testing.T) {
		// The level comments of Power Strike differ by the power
		// numbers, so the run of the learned level must answer.
		require.Equal(t,
			"Gathers power for a fierce strike. Used when equipped "+
				"with a sword or blunt type weapon. Over-hit is "+
				"possible. Power 30.",
			SkillDescription(powerStrikeSkillID, 3))
		require.Contains(t, SkillDescription(powerStrikeSkillID, 4),
			"Power 39.")
	})

	t.Run("a level beyond the last run clamps to it", func(t *testing.T) {
		// Power Strike declares nine described levels; a learned
		// level beyond them keeps the last text instead of emptying.
		require.Contains(t, SkillDescription(powerStrikeSkillID, 20),
			"Power 70.")
	})

	t.Run("collapsed runs cover their level span", func(t *testing.T) {
		// Armor Mastery levels 1-3 share one run, level 4 starts the
		// heavy/light armor text.
		require.Equal(t, "Defense increases.",
			SkillDescription(armorMasterySkillID, 3))
		require.Equal(t, "Increases defense when wearing heavy "+
			"equipment. Increases defense and evasion when wearing "+
			"light equipment.",
			SkillDescription(armorMasterySkillID, 4))
	})

	t.Run("a whole skill text answers for every level", func(t *testing.T) {
		require.Equal(t, "Attack power increases.",
			SkillDescription(141, 1))
		require.Equal(t, "Attack power increases.",
			SkillDescription(141, 40))
	})

	t.Run("unknown skill returns empty", func(t *testing.T) {
		require.Empty(t, SkillDescription(unknownSkillID, 1))
		require.Empty(t, SkillDescription(0, 1))
	})
}

func TestSkillDescsCoverTheTrees(t *testing.T) {
	// Every distinct skill of every class tree must carry a
	// description: the web UI tooltip drops the block for unknown
	// texts, and a tree skill without one would render a hollow
	// tooltip.
	for classID, tree := range skillTrees {
		for _, lesson := range tree {
			require.NotEmpty(t,
				SkillDescription(lesson.SkillID, lesson.Level),
				"class %d lesson skill %d level %d lost its"+
					" description",
				classID, lesson.SkillID, lesson.Level)
		}
	}
}
