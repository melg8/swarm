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
