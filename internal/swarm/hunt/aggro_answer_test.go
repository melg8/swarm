// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The aggro answer and the pre-fight verdict regressions of the
// reported chaotic fight: a mob that aggros while the character walks
// to a pick owns the next swing, and an untouched target bar is not a
// lost fight.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// setMobHealth sets the current and maximum health of a spawned mob:
// the health percent view of the losing-fight verdict needs both.
func setMobHealth(bot *state.Bot, objectID int32, hp int32) {
    bot.ApplyStatusUpdate(objectID, []state.Attribute{
        {ID: state.AttrMaxHP, Value: 100},
        {ID: state.AttrCurHP, Value: hp},
    })
}

// TestLoopSwitchesToTheAggroWhenThePickHoldsUs pins the dropped
// ObjectTargetsSelf gate: the selected pick holding the character as
// its own target no longer suppresses the answer to a DIFFERENT
// nearer attacker - the nearer mob owns the next swing, the pick does
// not shuffle back.
func TestLoopSwitchesToTheAggroWhenThePickHoldsUs(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 46000)
    spawnMobAt(bot, 8, 45500)
    attackerHits(bot, 7, 46000)
    attackerHits(bot, 8, 45500)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.engageAt = time.Now()
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, int32(8), loop.target,
        "the nearer attacker owns the next swing although the pick "+
            "holds us too")
    require.Equal(t, []int32{8}, game.forces)
}

// TestLoopAnswersTheAggroAheadOfTheStuckTimeout pins the branch
// order of the engage: the added attacker is answered before the
// stale-selection timeout can write the pick off, so the mob that
// actually swings is never skipped for a peaceful target.
func TestLoopAnswersTheAggroAheadOfTheStuckTimeout(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 46000)
    spawnMobAt(bot, 8, 45500)
    attackerHits(bot, 8, 45500)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.engageAt = time.Now().Add(-time.Minute)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, int32(8), loop.target,
        "the aggro answer pre-empts the stuck skip of the pick")
    require.NotContains(t, loop.targetSkip, int32(7),
        "the pick is not written off while the attacker is answered")
    require.Equal(t, []int32{8}, game.forces)
}

// TestLosingFightIgnoresTheUntouchedTargetBar pins the pre-fight
// rule: the full health bar of a target whose fight has not started
// is not a health lead. The old percent-gap rule fled a winnable
// aggroed mob five seconds after the answer (the reported flee at
// 56 percent against a full bar).
func TestLosingFightIgnoresTheUntouchedTargetBar(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    setMobHealth(bot, 7, 100)
    hurtTo(bot, 56)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.False(t, loop.losingFight(),
        "the untouched target bar is not a lead over the character")
}

// TestLosingFightKeepsTheGapRuleForTheRunningFight pins the other
// half: once the fight runs, the clear health lead over a hurt
// character still drops the target - the rule is not gone, only
// fenced to confirmed fights.
func TestLosingFightKeepsTheGapRuleForTheRunningFight(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    setMobHealth(bot, 7, 100)
    selfSwingsAt(bot, 45600)
    hurtTo(bot, 50)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.True(t, loop.losingFight(),
        "the running fight with a 50 point lead is lost")
}

// TestLosingFightStillEscapesTheCriticalHealth pins the pre-fight
// critical escape: below the escape threshold the untouched target
// does not buy a fight either.
func TestLosingFightStillEscapesTheCriticalHealth(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    setMobHealth(bot, 7, 100)
    hurtTo(bot, 20)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.True(t, loop.losingFight(),
        "the critical health flees whatever the fight state is")
}
