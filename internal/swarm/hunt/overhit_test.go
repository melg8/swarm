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
// combat_skills.go overhitSafetyFactor and overhitStrikeDamage): the
// server pays the overhit experience bonus only when the cast of an
// overhit-flagged skill is the killing blow, so the hold keeps the
// strike sheathed until the target bar fits the estimated strike
// damage and fires it into the beaten bar.

// overhitFightBot returns the fighter with the learned Power Strike
// level 9 (the overhit flagged sword strike, power 70), a sword in
// hand, the P.Atk the UserInfo refresh of the session carries and a
// spawned mob.
func overhitFightBot() *state.Bot {
    bot := newTestBot()
    setSelfPAtk(bot, 50)
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 3, Level: 9, Passive: false},
    })
    equipSword(bot)
    spawnMob(bot)

    return bot
}

// setMobVitals sets the bar of the spawned mob (object 7): the
// maximum and the current value the server refreshes over
// StatusUpdate.
func setMobVitals(bot *state.Bot, curHP, maxHP int32) {
    bot.ApplyStatusUpdate(7, []state.Attribute{
        {ID: state.AttrMaxHP, Value: maxHP},
        {ID: state.AttrCurHP, Value: curHP},
    })
}

// paceTicks runs n paced fight ticks: the retry pacing resets, the
// loop re-decides the cast on every tick.
func paceTicks(loop *Loop, n int) {
    for range n {
        loop.lastActionAt = time.Now().Add(-time.Minute)
        loop.castAt = time.Time{}
        loop.tick()
    }
}

// TestOverhitStrikeHeldWhileTargetHealthy pins the hold: a bar above
// the estimated strike damage keeps the strike sheathed - an early
// cast clears the armed server flag on the first non lethal hit and
// loses the bonus.
func TestOverhitStrikeHeldWhileTargetHealthy(t *testing.T) {
    bot := overhitFightBot()
    setMobVitals(bot, 100, 100)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)
    // A landed swing of 20 (100 -> 80): the level 9 strike of the
    // 50 P.Atk character estimates 20 * (50+70)/50 * 0.85 = 40.8, so
    // the bar of 80 holds.
    setMobVitals(bot, 80, 100)
    paceTicks(loop, 3)

    require.Empty(t, game.casts,
        "the overhit strike holds above the estimated damage")
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

// TestOverhitStrikeHoldsWithoutPAtk pins the data gate: no UserInfo
// P.Atk means no damage estimate, and a blind cast would spend the
// reuse window without the bonus.
func TestOverhitStrikeHoldsWithoutPAtk(t *testing.T) {
    bot := newTestBot()
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 3, Level: 9, Passive: false},
    })
    equipSword(bot)
    spawnMob(bot)
    setMobVitals(bot, 100, 100)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)
    setMobVitals(bot, 20, 100)
    paceTicks(loop, 3)

    require.Empty(t, game.casts,
        "the overhit strike holds without the P.Atk of the session")
}

// TestOverhitStrikeHoldsOnLowBarOfAHighHPMob pins the defect the
// damage window replaced: the percent rule fired the strike at 40
// percent of the bar whatever the mob maximum, and the level 17 mob
// of 292 HP left at 40 percent (117 HP) survived the 105 damage
// strike by one auto attack. The absolute estimate holds the same
// bar: the 80 HP left on a 400 HP mob stand above the 40.8 damage.
func TestOverhitStrikeHoldsOnLowBarOfAHighHPMob(t *testing.T) {
    bot := overhitFightBot()
    setMobVitals(bot, 400, 400)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)
    // A landed swing of 20 sets the estimate; the bar drops to 100,
    // a quarter of the maximum and well inside the retired 40
    // percent window, yet above the strike damage.
    setMobVitals(bot, 380, 400)
    paceTicks(loop, 1)
    setMobVitals(bot, 100, 400)
    paceTicks(loop, 3)

    require.Empty(t, game.casts,
        "the overhit strike holds above the estimated damage")
}

// TestOverhitStrikeFiresAsTheFinishingBlow pins the release: the
// bar ground under the estimated strike damage fires the strike, and
// the decision log names the finishing blow.
func TestOverhitStrikeFiresAsTheFinishingBlow(t *testing.T) {
    bot := overhitFightBot()
    setMobVitals(bot, 100, 100)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetLogger(recordingLogger(bot))

    startFight(loop, bot)
    require.Empty(t, game.casts, "the hold keeps the healthy bar cast free")

    // The swings ground the bar: the first lands 20 (100 -> 80), the
    // sample minimum, and the second takes it to 20, under the 40.8
    // estimate of the level 9 strike.
    setMobVitals(bot, 80, 100)
    paceTicks(loop, 1)
    require.Empty(t, game.casts, "the bar above the estimate holds the strike")
    setMobVitals(bot, 20, 100)
    paceTicks(loop, 1)

    require.Equal(t, []int32{3}, game.casts,
        "the strike fires under the estimated damage")
    var logged bool
    for _, event := range bot.NewestEvents(20) {
        if strings.Contains(event.Message, "as the finishing blow") {
            logged = true
        }
    }
    require.True(t, logged, "the log names the finishing blow")
}

// TestOverhitSwingSampleSkipsCrits pins the sample filter: a critical
// hit is a larger roll of the same swing and never sizes the strike
// window; only the non critical hits feed the estimate.
func TestOverhitSwingSampleSkipsCrits(t *testing.T) {
    bot := overhitFightBot()
    setMobVitals(bot, 100, 100)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    startFight(loop, bot)
    require.Empty(t, game.casts)

    // A critical swing of 60 (100 -> 40): were it sampled, the
    // estimate would be 122 and the bar of 40 would fire the strike.
    bot.ApplyAttack(state.Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetX: 46000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [state.AttackTargets]int32{7, 0, 0, 0},
        CritFlags:   [state.AttackTargets]bool{true, false, false, false},
        TargetCount: 1,
    })
    setMobVitals(bot, 40, 100)
    paceTicks(loop, 2)
    require.Empty(t, game.casts,
        "the critical hit never sizes the strike window")
    require.Zero(t, loop.swingDamage,
        "the critical hit stays out of the swing sample")

    // A non critical swing of 20 (40 -> 20) sizes the estimate at
    // 40.8, so the next paced tick releases the strike.
    setMobVitals(bot, 20, 100)
    paceTicks(loop, 1)

    require.Equal(t, []int32{3}, game.casts,
        "the non critical swing sizes the strike window")
    require.InDelta(t, 20, loop.swingDamage, 0.001,
        "the sample keeps the non critical swing")
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
