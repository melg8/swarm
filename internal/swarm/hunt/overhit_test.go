// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "strings"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The overhit finishing blow of the combat casting (see
// combat_skills.go overhitFinishPercent): the server pays the
// overhit experience bonus only when the cast of an overhit-flagged
// skill is the killing blow, so the hold keeps the strike sheathed
// while the target stands above the finish window and fires it into
// the beaten bar.

// overhitFightBot returns the fighter with the learned Power Strike
// (the overhit flagged sword strike) and a sword in hand.
func overhitFightBot() *state.Bot {
    bot := newTestBot()
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 3, Level: 1, Passive: false},
    })
    equipSword(bot)
    spawnMob(bot)

    return bot
}

// paceTicks runs n paced fight ticks: the retry pacing resets, the
// loop re-decides the cast on every tick.
func paceTicks(loop *Loop, n int) {
    for range n {
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.castAt = time.Time{}
        loop.tick()
    }
}

// TestOverhitStrikeHeldWhileTargetHealthy pins the hold: the strike
// of a healthy target never goes out, whatever the paced retries
// ask - an early cast would clear the armed server flag on the
// first non lethal damage and lose the bonus.
func TestOverhitStrikeHeldWhileTargetHealthy(t *testing.T) {
    bot := overhitFightBot()
    setMobHP(bot, 80)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)
    paceTicks(loop, 3)

    require.Empty(t, game.casts,
        "the overhit strike holds above the finish window")
}

// TestOverhitStrikeHeldOnUnknownVitals pins the fresh fight case:
// the target vitals arrive with the first landed blows, and until
// then the strike holds - the bar is full in practice.
func TestOverhitStrikeHeldOnUnknownVitals(t *testing.T) {
    bot := overhitFightBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)
    paceTicks(loop, 3)

    require.Empty(t, game.casts,
        "the overhit strike holds on unknown vitals")
}

// TestOverhitStrikeFiresAsTheFinishingBlow pins the release: the
// bar ground into the finish window fires the strike, and the
// decision log names the finishing blow.
func TestOverhitStrikeFiresAsTheFinishingBlow(t *testing.T) {
    bot := overhitFightBot()
    setMobHP(bot, 80)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetLogger(recordingLogger(bot))

    startFight(loop, bot)
    require.Empty(t, game.casts, "the hold keeps the healthy bar cast free")

    // The swings ground the bar into the window: the next paced
    // tick releases the strike.
    setMobHP(bot, 15)
    paceTicks(loop, 1)

    require.Equal(t, []int32{3}, game.casts,
        "the strike fires inside the finish window")
    var logged bool
    for _, event := range bot.NewestEvents(20) {
        if strings.Contains(event.Message, "as the finishing blow") {
            logged = true
        }
    }
    require.True(t, logged, "the log names the finishing blow")
}

// TestNonOverhitStrikeKeepsTheFightPace pins the scope: the hold
// keys on the overhit skill set - a strike without the flag (the
// mystic spell here) fires at a healthy target exactly as before.
func TestNonOverhitStrikeKeepsTheFightPace(t *testing.T) {
    bot := newCasterBot(80)
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 1177, Level: 1, Passive: false},
    })
    spawnMob(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)

    require.Equal(t, []int32{1177}, game.casts,
        "the unflagged spell fires at a healthy target")
}
