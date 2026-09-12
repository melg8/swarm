// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// classTransferJournal returns the quest journal of a bot walking
// the two elven class transfer quests: Q00406 at the topaz
// collection step and Q407 at the letter delivery step, with the
// topaz and emerald stacks of the kill grounds.
func classTransferJournal() ([]QuestEntryView, []QuestItemView) {
	return []QuestEntryView{
			{QuestID: 406, State: 1},
			{QuestID: 407, State: 3},
		}, []QuestItemView{
			{ItemID: 1205, Count: 20}, // Topaz Piece
			{ItemID: 1206, Count: 7},  // Emerald Piece
		}
}

// TestApplyQuestListPinsTheJournal pins the apply path: the whole
// journal lands and the accessors answer per quest and per item.
func TestApplyQuestListPinsTheJournal(t *testing.T) {
	bot := NewBot("test1")
	require.Equal(t, 0, bot.QuestCount())
	_, active := bot.QuestCond(406)
	require.False(t, active, "no quest before the server lists it")

	quests, items := classTransferJournal()
	bot.ApplyQuestList(quests, items)

	require.Equal(t, 2, bot.QuestCount())

	cond, active := bot.QuestCond(406)
	require.True(t, active)
	require.Equal(t, int32(1), cond)
	cond, active = bot.QuestCond(407)
	require.True(t, active)
	require.Equal(t, int32(3), cond)

	_, active = bot.QuestCond(219)
	require.False(t, active, "a quest the journal does not carry")

	require.True(t, bot.IsQuestItem(1205))
	require.True(t, bot.IsQuestItem(1206))
	require.False(t, bot.IsQuestItem(1145),
		"the medallion of the human warrior is not in this journal")

	count, present := bot.QuestItemCount(1205)
	require.True(t, present)
	require.Equal(t, int32(20), count)
	count, present = bot.QuestItemCount(1206)
	require.True(t, present)
	require.Equal(t, int32(7), count)
	_, present = bot.QuestItemCount(1202)
	require.False(t, present)
}

// TestApplyQuestListReplacesTheJournal pins the whole-list
// semantics: a later packet with fewer quests drops the stale
// entries instead of merging (the server resends everything).
func TestApplyQuestListReplacesTheJournal(t *testing.T) {
	bot := NewBot("test1")
	quests, items := classTransferJournal()
	bot.ApplyQuestList(quests, items)

	bot.ApplyQuestList([]QuestEntryView{{QuestID: 406, State: 6}}, nil)
	require.Equal(t, 1, bot.QuestCount())
	cond, active := bot.QuestCond(406)
	require.True(t, active)
	require.Equal(t, int32(6), cond)
	_, active = bot.QuestCond(407)
	require.False(t, active, "the dropped quest is gone")
	require.False(t, bot.IsQuestItem(1205),
		"the quest items of the dropped quest are gone")
}

// TestQuestIDsAreSorted pins the deterministic accessor order.
func TestQuestIDsAreSorted(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyQuestList([]QuestEntryView{
		{QuestID: 407, State: 1},
		{QuestID: 406, State: 1},
		{QuestID: 219, State: 2},
	}, nil)

	require.Equal(t, []int32{219, 406, 407}, bot.QuestIDs())
}

// TestResetSessionClearsTheJournal pins the session reset: the
// journal is session state the server repushes at world entry (the
// same semantics the skill list applies), so a lost session must not
// leak its quests into the next login window.
func TestResetSessionClearsTheJournal(t *testing.T) {
	bot := NewBot("test1")
	quests, items := classTransferJournal()
	bot.ApplyQuestList(quests, items)

	bot.ResetSession()
	require.Equal(t, 0, bot.QuestCount())
	_, active := bot.QuestCond(406)
	require.False(t, active)
	require.False(t, bot.IsQuestItem(1205))
}
