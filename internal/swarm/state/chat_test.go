// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// chatLines returns the chat window lines in chronological order.
func chatLines(bot *Bot) []ChatEvent {
    snap := bot.Snapshot()

    return snap.Chat
}

func TestApplySystemMessageFormatsAdena(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

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
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

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
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
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
            require.Positive(t, obj.SocialUntilMs)
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
    require.Positive(t, fresh.Character.SocialUntilMs)
}

func TestChatWindowRollsOver(t *testing.T) {
    bot := NewBot("acc1")
    for i := range chatCapacity + 10 {
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

func TestApplySayMapsChannelsToKinds(t *testing.T) {
    bot := NewBot("acc1")
    bot.ApplySay(Say{
        ObjectID: 7, Channel: 0, From: "Melg", Text: "hello",
    })
    bot.ApplySay(Say{
        ObjectID: 8, Channel: 1, From: "Trader", Text: "WTS bow",
    })
    bot.ApplySay(Say{
        ObjectID: 9, Channel: 2, From: "Ghost", Text: "psst",
    })
    bot.ApplySay(Say{
        ObjectID: 10, Channel: 8, From: "Merchant", Text: "selling",
    })
    bot.ApplySay(Say{
        ObjectID: 11, Channel: 42, From: "Odd", Text: "unknown channel",
    })

    lines := chatLines(bot)
    require.Len(t, lines, 5)
    kinds := make([]string, 0, len(lines))
    for _, line := range lines {
        kinds = append(kinds, line.Kind)
    }
    require.Equal(t,
        []string{"say", "shout", "whisper", "trade", "say"}, kinds)
    require.Equal(t, "Melg", lines[0].From)
    require.Equal(t, "hello", lines[0].Text)
    require.Equal(t, "Ghost", lines[2].From)
}

func TestApplySaySystemAndSocialLinesKeepEmptyFrom(t *testing.T) {
    bot := NewBot("acc1")
    bot.ApplySystemMessage(SystemMessage{ID: 181})
    bot.ApplySay(Say{ObjectID: 7, Channel: 0, From: "Melg", Text: "hi"})

    lines := chatLines(bot)
    require.Len(t, lines, 2)
    require.Empty(t, lines[0].From)
    require.Equal(t, "Melg", lines[1].From)
}

func TestApplySystemMessageRecordsCannotSeeTarget(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

    // Any other system message leaves the refusal time untouched.
    bot.ApplySystemMessage(SystemMessage{ID: 28})
    require.True(t, bot.SelfCannotSeeTargetAt().IsZero(),
        "an unrelated message never marks the blind attack")

    // The "Cannot see target." answer (id 181 of the Mobius C1
    // SystemMessageId) records its arrival time.
    before := time.Now()
    bot.ApplySystemMessage(SystemMessage{ID: 181})
    at := bot.SelfCannotSeeTargetAt()
    require.False(t, at.IsZero())
    require.False(t, at.Before(before),
        "the refusal time is the arrival time")

    // The chat window still carries the formatted line.
    lines := chatLines(bot)
    require.Len(t, lines, 2)
    require.Equal(t, "Cannot see target.", lines[1].Text)
}

func TestApplySystemMessageResolvesSkillNames(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

    // id 46: "Use $s1." with a skill name parameter: the generated
    // skill dictionary resolves the id and the level rides the
    // parameter's second int (Power Strike, skill id 3).
    bot.ApplySystemMessage(SystemMessage{
        ID: 46,
        Params: []ChatMessageParam{
            {Type: 4, Int: 3, Level: 3},
        },
    })
    lines := chatLines(bot)
    require.Len(t, lines, 1)
    require.Equal(t, "Use Power Strike lvl 3.", lines[0].Text)

    // A levelless skill parameter renders the name alone.
    bot.ApplySystemMessage(SystemMessage{
        ID:     46,
        Params: []ChatMessageParam{{Type: 4, Int: 3}},
    })
    lines = chatLines(bot)
    require.Len(t, lines, 2)
    require.Equal(t, "Use Power Strike.", lines[1].Text)
}

func TestApplySystemMessageResolvesBuffEffectNames(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

    // id 110: "You can feel $s1's effect." with the Might buff (skill
    // id 1068): the raw skill id never surfaces in the chat line.
    bot.ApplySystemMessage(SystemMessage{
        ID: 110,
        Params: []ChatMessageParam{
            {Type: 4, Int: 1068, Level: 1},
        },
    })
    lines := chatLines(bot)
    require.Len(t, lines, 1)
    require.Equal(t, "You can feel Might lvl 1's effect.", lines[0].Text)
}

func TestApplySystemMessageMissingTailParamStaysEmpty(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

    // The sendMessage texts of the server ride the generic "$s1 $s2"
    // template (id 614) with the whole sentence as the one text
    // parameter; the missing tail renders as nothing - not as a stray
    // "?" - and the template gap leaves no trailing space behind.
    bot.ApplySystemMessage(SystemMessage{
        ID: 614,
        Params: []ChatMessageParam{
            {Type: 0,
                Text: "You are no longer protected from aggressive " +
                    "monsters."},
        },
    })
    lines := chatLines(bot)
    require.Len(t, lines, 1)
    require.Equal(t,
        "You are no longer protected from aggressive monsters.",
        lines[0].Text)
}

func TestRenderChatParamUnknownSkillFallsBackToRawID(t *testing.T) {
    require.Equal(t, "999999", renderChatParam(
        []ChatMessageParam{{Type: 4, Int: 999999, Level: 1}}, 0))
}
