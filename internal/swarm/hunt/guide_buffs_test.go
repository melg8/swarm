// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// setGuideLevel pins the character level of the guide tests (the
// elven fighter class of the newTestBot base).
func setGuideLevel(bot *state.Bot, level int32) {
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest1", Level: level, ClassID: 18, Race: 1, Sp: 0,
    })
}

// TestExpectedGuideBuffsByLevel pins the support magic windows of
// the guide gate: the level band, the class branches and the grant
// order of the deployed server data (the Life Cubic of the older
// server versions stays excluded from the table).
func TestExpectedGuideBuffsByLevel(t *testing.T) {
    for _, tc := range []struct {
        name  string
        level int32
        mage  bool
        want  []int32
    }{
        {"level 5 is below the band", 5, false, nil},
        {"level 10 fighter", 10, false, []int32{1204}},
        {"level 13 fighter", 13, false, []int32{1204, 1040, 1045, 1068}},
        {"level 18 fighter", 18, false,
            []int32{1204, 1040, 1045, 1068, 1044, 1086}},
        {"level 18 mage", 18, true,
            []int32{1204, 1040, 1048, 1085, 1078, 1059}},
        {"level 25 is above the band", 25, false, nil},
    } {
        t.Run(tc.name, func(t *testing.T) {
            got := expectedGuideBuffs(tc.level, tc.mage)
            if tc.want == nil {
                require.Empty(t, got)
            } else {
                require.Equal(t, tc.want, got)
            }
        })
    }
    for _, buff := range guideBuffSkills {
        require.NotEqual(t, int32(67), buff.SkillID,
            "the Life Cubic stays excluded")
    }
}

// shortenGuideSeams squeezes the guide stop waits (the dialog seams
// of the walker and the buff landing wait) so the dialog drive of
// the guide tests runs in well under a second; the production
// values return on cleanup.
func shortenGuideSeams(t *testing.T) {
    t.Helper()
    shortenDialogSeams(t)
    wait := guideBuffWait
    guideBuffWait = 20 * time.Millisecond
    t.Cleanup(func() { guideBuffWait = wait })
}

// TestReceiveGuideMagicLandsTheBuffs pins the support magic drive of
// the issue #35 fix end to end: the dialog walks the two guide links
// (the entry page, the SupportMagic.htm page), the apply bypass
// answers with the effect and NO page (the route's last step carries
// AnswerIsEffect), the buff watch sees the landing skill and the
// stop reports success - no refusal cooldown arms on the happy path
// (the old final page wait failed the walk after the buffs landed
// and armed the 10 minute cooldown on every success).
func TestReceiveGuideMagicLandsTheBuffs(t *testing.T) {
    shortenGuideSeams(t)
    bot := state.NewBot("guide")
    setGuideLevel(bot, 13)
    // The server answer of the apply bypass: the level-eligible
    // support magic trails in as abnormal status updates.
    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 1204, Level: 1, Time: 1200},
        {SkillID: 1040, Level: 1, Time: 1200},
    })
    script := &dialogScript{
        cur: -1,
        pages: []scriptPage{
            {npc: 30599, html: guideEntryPage},
            {npc: 30599, html: guideSupportPage},
            // No third page: the apply bypass answers with the
            // buffs, never with a dialog page.
        },
    }
    game := &scriptGame{fakeGame: &fakeGame{}, script: script}
    loop := NewLoop(game, bot)
    loop.guideID = 30599

    require.True(t, loop.receiveGuideMagic())
    require.Len(t, game.bypasses, 2,
        "the entry link and the apply link fired")
    require.Equal(t, "npc_30599_Link default/SupportMagic.htm",
        game.bypasses[0])
    require.Equal(t, "npc_30599_SupportMagic", game.bypasses[1])
    require.Equal(t, []string{"npc_30599_SupportMagic"}, script.dropped,
        "only the apply bypass is unanswered - its answer is the "+
            "buff effect, the entry link was answered with the page")
    require.True(t, loop.guideRefusedAt.IsZero(),
        "the happy path never arms the refusal cooldown")
}

// TestReceiveGuideMagicArmsRefusalWithoutBuffs pins the refusal
// path: the apply bypass fired but nothing landed (the refusal html
// of the server gate - the level band, the account), the buff watch
// times out and the stop arms the cooldown so the later trips stay
// away instead of retrying the refusal in a loop.
func TestReceiveGuideMagicArmsRefusalWithoutBuffs(t *testing.T) {
    shortenGuideSeams(t)
    bot := state.NewBot("guide")
    setGuideLevel(bot, 13)
    script := &dialogScript{
        cur: -1,
        pages: []scriptPage{
            {npc: 30599, html: guideEntryPage},
            {npc: 30599, html: guideSupportPage},
        },
    }
    game := &scriptGame{fakeGame: &fakeGame{}, script: script}
    loop := NewLoop(game, bot)
    loop.guideID = 30599

    require.True(t, loop.receiveGuideMagic())
    require.Len(t, game.bypasses, 2,
        "the dialog itself walked clean - the refusal is the answer")
    require.False(t, loop.guideRefusedAt.IsZero(),
        "the missing buffs arm the refusal cooldown")
}

// TestPlanGuideStopAppendsTheStop pins the trip planning: the stop
// of the Newbie Guide closes the trip behind the learning stops and
// plans once per trip.
func TestPlanGuideStopAppendsTheStop(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)

    loop.planGuideStop()

    require.Len(t, loop.tripStops, 1)
    stop := loop.tripStops[0]
    require.True(t, stop.guide, "the planned stop is the guide stop")
    require.False(t, stop.teach)
    require.False(t, stop.sell)
    require.Equal(t, guideStopNpc, stop.merchant)
    loop.planGuideStop()
    require.Len(t, loop.tripStops, 1,
        "the stop never duplicates in one trip")
}

// TestPlanGuideStopSkipsTheRefusedAndOutBandLevels pins the wanted
// gate of the planning: the levels outside the 8-24 band and the
// refusal cooldown never plan a stop.
func TestPlanGuideStopSkipsTheRefusedAndOutBandLevels(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 25)
    loop.planGuideStop()
    require.Empty(t, loop.tripStops, "the band gate refuses level 25")

    setGuideLevel(bot, 5)
    loop.planGuideStop()
    require.Empty(t, loop.tripStops, "the band gate refuses level 5")

    setGuideLevel(bot, 13)
    loop.guideRefusedAt = time.Now().Add(time.Minute)
    loop.planGuideStop()
    require.Empty(t, loop.tripStops, "the refusal cooldown gates")
}

// TestGuideBuffBlocksTheSelfCastOfTheSlot pins the collision guard
// of the self buff cast: the fresh Might of the guide holds the
// PA_UP slot, the own attack aura stays uncast.
func TestGuideBuffBlocksTheSelfCastOfTheSlot(t *testing.T) {
    bot := newTestBot()
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 194, Level: 1, Passive: true},
        {SkillID: 77, Level: 1},
    })
    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 1068, Level: 1, Time: 1200},
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    require.False(t, loop.maybeSelfBuff(time.Now()))
    require.Empty(t, game.casts,
        "the own aura never replaces the guide buff")
}

// TestActiveSelfBuffKeepsTheGuideAwayFromTheSlot pins the collision
// guard of the guide seek: the active own PA_UP aura keeps the bot
// from seeking Might, without it the missing Might is worth the walk.
func TestActiveSelfBuffKeepsTheGuideAwayFromTheSlot(t *testing.T) {
    bot := newTestBot()
    setGuideLevel(bot, 13)
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 77, Level: 1},
    })
    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 77, Level: 1, Time: 600},
        {SkillID: 1204, Level: 2, Time: 1200},
        {SkillID: 1040, Level: 3, Time: 1200},
        {SkillID: 1045, Level: 2, Time: 1200},
    })
    loop := NewLoop(&fakeGame{}, bot)

    require.False(t, loop.guideWanted(),
        "the guide must not seek Might while the own aura holds the slot")

    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 1204, Level: 2, Time: 1200},
        {SkillID: 1040, Level: 3, Time: 1200},
        {SkillID: 1045, Level: 2, Time: 1200},
    })
    require.True(t, loop.guideWanted(),
        "without the own aura the missing Might is worth the walk")
}

// TestAdvanceTripStopWalksToTheGuide pins the walk planning of the
// guide stop: the trip advance plans the npc segment of the spawn.
func TestAdvanceTripStopWalksToTheGuide(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    loop.tripStops = []tripStop{
        {merchant: townMerchants[0], buys: nil, sell: true,
            teach: false, guide: false},
        {merchant: guideStopNpc, buys: nil, sell: false,
            teach: false, guide: true},
    }

    loop.advanceTripStop()

    require.Equal(t, phaseTownWalk, loop.phase,
        "the advance plans the walk to the guide spawn")
}
