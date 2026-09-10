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

func TestSkillBooksOfTheTrees(t *testing.T) {
	t.Run("the elven fighter book lessons carry their books", func(t *testing.T) {
		// Defence Aura level 1 wants the spellbook 1294 and
		// Attack Aura level 1 the spellbook 1095; the levels
		// beyond them learn without a book.
		tree, ok := SkillTree(18)
		require.True(t, ok)
		books := map[int32]int32{}
		for _, lesson := range tree {
			if lesson.BookItem != 0 {
				books[lesson.SkillID] = lesson.BookItem
			}
		}
		require.Equal(t, int32(1294), books[91])
		require.Equal(t, int32(1095), books[77])
		require.NotContains(t, books, int32(3))
	})

	t.Run("the elven mystic book lessons carry their books", func(t *testing.T) {
		// The mystic trees demand books for the first level of
		// the spells: Heal (1011) wants the spellbook 1152.
		tree, ok := SkillTree(25)
		require.True(t, ok)
		books := map[int32]int32{}
		for _, lesson := range tree {
			if lesson.BookItem != 0 {
				books[lesson.SkillID] = lesson.BookItem
			}
		}
		require.Equal(t, int32(1152), books[1011])
	})
}

func TestSkillCastsOfTheTrees(t *testing.T) {
	t.Run("the strike stats resolve", func(t *testing.T) {
		cast, ok := SkillCastOf(powerStrikeSkillID)
		require.True(t, ok)
		require.Equal(t, "A1", cast.Operate)
		require.Equal(t, "ONE", cast.Target)
		require.Equal(t, int32(40), cast.CastRange)
		require.Equal(t, int32(13000), cast.ReuseDelay)
		require.False(t, cast.Magic)
		require.True(t, cast.UsableWithWeapon("SWORD"))
		require.True(t, cast.UsableWithWeapon("BLUNT"))
		require.False(t, cast.UsableWithWeapon("BOW"))
	})

	t.Run("the per level mana cost clamps", func(t *testing.T) {
		cast, ok := SkillCastOf(powerStrikeSkillID)
		require.True(t, ok)
		require.Equal(t, int32(10), cast.MPCostOf(1))
		require.Equal(t, int32(13), cast.MPCostOf(4))
		// Beyond the table: the last entry answers.
		require.Equal(t, int32(19), cast.MPCostOf(50))
		// Below the table: the first entry answers.
		require.Equal(t, int32(10), cast.MPCostOf(0))
	})

	t.Run("the buff stats resolve", func(t *testing.T) {
		cast, ok := SkillCastOf(91) // Defence Aura
		require.True(t, ok)
		require.Equal(t, "A2", cast.Operate)
		require.Equal(t, "SELF", cast.Target)
		require.Equal(t, int32(1200), cast.BuffTime)
		require.True(t, cast.UsableWithWeapon("SWORD"))
	})

	t.Run("the magic attack stats resolve", func(t *testing.T) {
		cast, ok := SkillCastOf(1177) // Wind Strike
		require.True(t, ok)
		require.True(t, cast.Magic)
		require.Equal(t, int32(600), cast.CastRange)
		require.Equal(t, int32(6000), cast.ReuseDelay)
		require.Equal(t, int32(7), cast.MPCostOf(1))
	})

	t.Run("passive skills never enter the cast map", func(t *testing.T) {
		_, ok := SkillCastOf(armorMasterySkillID)
		require.False(t, ok)
		_, ok = SkillCastOf(unknownSkillID)
		require.False(t, ok)
	})
}

func TestTeachersOfClass(t *testing.T) {
	t.Run("the elven fighter teachers stand in the village", func(t *testing.T) {
		teachers := TeachersOfClass(18)
		require.NotEmpty(t, teachers)
		var ellenia bool
		for _, teacher := range teachers {
			if teacher.Name == "Ellenia" {
				ellenia = true
				require.Equal(t, int32(7155),
					teacher.TemplateID)
				require.Equal(t, int32(45725), teacher.X)
			}
		}
		require.True(t, ellenia)
	})

	t.Run("the elven mystic teachers stand in the village", func(t *testing.T) {
		teachers := TeachersOfClass(25)
		require.NotEmpty(t, teachers)
		var greenis bool
		for _, teacher := range teachers {
			if teacher.Name == "Greenis" {
				greenis = true
				require.Equal(t, int32(7157),
					teacher.TemplateID)
			}
		}
		require.True(t, greenis)
	})

	t.Run("an unknown class has no teachers", func(t *testing.T) {
		require.Empty(t, TeachersOfClass(999))
	})
}
