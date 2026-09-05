// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// chatLines returns the chat window lines in chronological order.
func chatLines(bot *Bot) []ChatEvent {
	snap := bot.Snapshot()

	return snap.Chat
}

func TestApplySystemMessageFormatsAdena(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	// id 28: "You picked up $s1 adena." with an int parameter.
	bot.ApplySystemMessage(SystemMessage{
		ID:     28,
		Params: []ChatMessageParam{{Type: 1, Int: 25}},
	})
	lines := chatLines(bot)
	require.Len(t, lines, 1)
	require.Equal(t, "system", lines[0].Kind)
	require.Equal(t, "You picked up 25 adena.", lines[0].Text)
}

func TestApplySystemMessageResolvesItemNames(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	// id 30: "You picked up $s1." with an item name parameter: the
	// generated item dictionary resolves the display id.
	bot.ApplySystemMessage(SystemMessage{
		ID:     30,
		Params: []ChatMessageParam{{Type: 3, Int: 57}},
	})
	lines := chatLines(bot)
	require.Len(t, lines, 1)
	require.Equal(t, "You picked up Adena.", lines[0].Text)
}

func TestApplySystemMessageUnknownIdFallsBack(t *testing.T) {
	bot := NewBot("acc1")
	bot.ApplySystemMessage(SystemMessage{ID: 99999})
	lines := chatLines(bot)
	require.Len(t, lines, 1)
	require.Equal(t, "system message 99999", lines[0].Text)
}

func TestApplySocialActionMarkerAndLevelUps(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 100, Y: 100, Name: "Gremlin",
	})

	// Idle gestures of the surrounding npcs never reach the chat: they
	// only light the map marker of the animating npc.
	bot.ApplySocialAction(SocialAction{ObjectID: 7, ActionID: 2})
	bot.ApplySocialAction(SocialAction{ObjectID: 999, ActionID: 3})
	require.Empty(t, chatLines(bot))
	snap := bot.Snapshot()
	for _, obj := range snap.Objects {
		if obj.ObjectID == 7 {
			require.Greater(t, obj.SocialUntilMs, int64(0))
		}
	}

	// Level ups are rare and meaningful: they stay in the chat.
	bot.ApplySocialAction(SocialAction{ObjectID: 100, ActionID: 15})
	bot.ApplySocialAction(SocialAction{ObjectID: 7, ActionID: 15})
	lines := chatLines(bot)
	require.Len(t, lines, 2)
	require.Equal(t, "You reached a new level", lines[0].Text)
	require.Equal(t, "social", lines[0].Kind)
	require.Equal(t, "Gremlin reached a new level", lines[1].Text)
	fresh := bot.Snapshot()
	require.Greater(t, fresh.Character.SocialUntilMs, int64(0))
}

func TestChatWindowRollsOver(t *testing.T) {
	bot := NewBot("acc1")
	for i := 0; i < chatCapacity+10; i++ {
		bot.ApplySystemMessage(SystemMessage{ID: 28, Params: []ChatMessageParam{
			{Type: 1, Int: int32(i)},
		}})
	}
	lines := chatLines(bot)
	require.Len(t, lines, chatCapacity)
	// The oldest lines rolled out: the first visible one is move 10.
	require.Equal(t, "You picked up 10 adena.", lines[0].Text)
	require.Equal(t, "You picked up 73 adena.",
		lines[len(lines)-1].Text)
}
