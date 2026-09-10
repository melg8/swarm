// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"encoding/json"
	"testing"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/stretchr/testify/require"
)

// elvenFighterSkills returns a learned set of the elven fighter test
// bot: the level 3 starter strikes plus the passive masteries.
func elvenFighterSkills() []LearnedSkill {
	return []LearnedSkill{
		{SkillID: 3, Level: 3, Passive: false},
		{SkillID: 16, Level: 3, Passive: false},
		{SkillID: 56, Level: 3, Passive: false},
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	}
}

// TestSetSkillsPublishesTheSnapshot pins the apply path: the learned
// list lands in the snapshot enriched with the display data of the
// generated dictionary, sorted by skill id.
func TestSetSkillsPublishesTheSnapshot(t *testing.T) {
	bot := NewBot("test1")
	require.Nil(t, bot.Snapshot().Skills,
		"no skills before the server lists them")

	bot.SetSkills(elvenFighterSkills())
	snap := bot.Snapshot()
	require.NotNil(t, snap.Skills)
	require.Len(t, snap.Skills, 5)

	first := snap.Skills[0]
	require.Equal(t, int32(3), first.SkillID)
	require.Equal(t, int32(3), first.Level)
	require.False(t, first.Passive)
	require.Equal(t, "Power Strike", first.Name)
	require.Equal(t, "skill0003", first.Icon)
	require.Equal(t,
		"Gathers power for a fierce strike. Used when equipped "+
			"with a sword or blunt type weapon. Over-hit is "+
			"possible. Power 30.",
		first.Desc)

	mastery := snap.Skills[3]
	require.Equal(t, int32(142), mastery.SkillID)
	require.True(t, mastery.Passive)
	require.Equal(t, "Armor Mastery", mastery.Name)
	require.Equal(t, "skill0142", mastery.Icon)
	require.Equal(t, "Defense increases.", mastery.Desc)
}

// TestSkillPlanOrdersWarriorPriorities pins the learning order of the
// warrior plan: the attack power skills come first (the physical
// strikes and the weapon mastery), the defense skills second, the
// rest last; within a category the unlock level orders the lessons.
func TestSkillPlanOrdersWarriorPriorities(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 100,
	})
	bot.SetSkills(elvenFighterSkills())

	plan := bot.Snapshot().SkillPlan
	require.NotNil(t, plan)
	require.NotEmpty(t, plan.Entries)

	// The category blocks must be contiguous and ordered attack,
	// defense, other.
	lastCategory := -1
	for _, entry := range plan.Entries {
		require.LessOrEqual(t, lastCategory, entry.Category,
			"category blocks must not interleave")
		lastCategory = entry.Category
	}
	require.Equal(t, npcdata.SkillCategoryAttack, plan.Entries[0].Category,
		"the first lesson must be an attack power skill")

	// The known Power Strike levels 1-3 never reappear; the next
	// lesson is level 4 at level 10.
	var strike *SkillPlanEntry
	for i := range plan.Entries {
		if plan.Entries[i].SkillID == 3 {
			strike = &plan.Entries[i]

			break
		}
	}
	require.NotNil(t, strike, "Power Strike 4 must stay queued")
	require.Equal(t, int32(4), strike.Level)
	require.Equal(t, int32(310), strike.SpCost)
	require.Equal(t, int32(10), strike.ReqLevel)
	require.False(t, strike.Affordable, "100 sp cannot pay 310")
	require.Equal(t,
		"Gathers power for a fierce strike. Used when equipped "+
			"with a sword or blunt type weapon. Over-hit is "+
			"possible. Power 39.",
		strike.Desc, "the description must answer with the "+
			"level being learned")

	// The auto granted Lucky never enters the queue.
	for _, entry := range plan.Entries {
		require.NotEqual(t, int32(194), entry.SkillID)
	}

	// The affordability flag follows the SP: with 400 sp the level 10
	// Power Strike lessons become affordable (the level 15 ones at
	// 1100 sp stay out of reach).
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 400,
	})
	plan = bot.Snapshot().SkillPlan
	for _, entry := range plan.Entries {
		if entry.SkillID == 3 && entry.ReqLevel <= 10 {
			require.True(t, entry.Affordable)
		}
	}
}

// TestSkillPlanTotals pins the queue economics: the SP wallet, the
// total cost of the whole queue and the missing SP.
func TestSkillPlanTotals(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 500,
	})
	bot.SetSkills(nil)

	plan := bot.Snapshot().SkillPlan
	require.NotNil(t, plan)
	require.Equal(t, int64(500), plan.Sp)

	var total int64
	for _, entry := range plan.Entries {
		total += int64(entry.SpCost)
	}
	require.Equal(t, total, plan.Total)
	require.Equal(t, total-500, plan.Missing)
}

// TestSkillPlanClearedBySessionReset pins the reset: a session reset
// drops the learned list and the queue (the fresh login republishes
// them).
func TestSkillPlanClearedBySessionReset(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1,
	})
	bot.SetSkills(elvenFighterSkills())
	require.NotNil(t, bot.Snapshot().SkillPlan)

	bot.ResetSession()
	snap := bot.Snapshot()
	require.Nil(t, snap.Skills)
	require.Nil(t, snap.SkillPlan)
}

// TestSkillPlanUnknownClassStaysNull pins the fallback: a class the
// generated dictionary does not know publishes no queue (mystic trees
// are not warriors - the dictionary still answers them - but a bogus
// class id must not crash the snapshot).
func TestSkillPlanUnknownClassStaysNull(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 999, Race: 1,
	})
	bot.SetSkills(elvenFighterSkills())

	require.Nil(t, bot.Snapshot().SkillPlan)
}

// TestSkillPlanJSONShape pins the JSON contract of the queue and the
// learned list: the field names the web UI reads.
func TestSkillPlanJSONShape(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 200,
	})
	bot.SetSkills([]LearnedSkill{{SkillID: 142, Level: 1, Passive: true}})

	encoded, err := json.Marshal(bot.Snapshot())
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(encoded, &raw))
	skills, ok := raw["skills"].([]any)
	require.True(t, ok)
	require.Len(t, skills, 1)
	skill := skills[0].(map[string]any)
	require.InDelta(t, 142, skill["skillId"], 0.0001)
	require.InDelta(t, 1, skill["level"], 0.0001)
	require.Equal(t, true, skill["passive"])
	require.Equal(t, "Armor Mastery", skill["name"])
	require.Equal(t, "skill0142", skill["icon"])
	require.Equal(t, "Defense increases.", skill["desc"])

	plan, ok := raw["skillPlan"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 200, plan["sp"], 0.0001)
	entries, ok := plan["entries"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, entries)
	first, ok := entries[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Power Strike", first["name"])
	require.InDelta(t, 60, first["spCost"], 0.0001)
	require.InDelta(t, 5, first["reqLevel"], 0.0001)
	require.Equal(t, false, first["passive"])
	require.Equal(t, true, first["affordable"])
	require.InDelta(t, 0, first["category"], 0.0001)
	require.Contains(t, first["desc"],
		"Gathers power for a fierce strike")
}
