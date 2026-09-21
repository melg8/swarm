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

// The recovery self heal of the rest logic (see recovery_heal.go):
// a character that knows an instant self heal casts it instead of
// sitting down while the mana pays and no blow is landing, a
// sitting character stands up to cast, and the sit transition
// waits out the heal flight.

// healFightBot returns the fighter that knows Self Heal (the C1
// self heal the server granted) with the given health and mana
// fill (the maximum of 100 makes both read as percents).
func healFightBot(curHP, curMP int32) *state.Bot {
    bot := newTestBot()
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest1", Level: 10, ClassID: 18, Race: 1,
        X: 45000, Y: 50000, Z: -3500,
        MaxHP: 100, CurHP: curHP, MaxMP: 100, CurMP: curMP,
    })
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 194, Level: 1, Passive: true},
        {SkillID: 1216, Level: 1, Passive: false},
    })
    spawnMob(bot)

    return bot
}

// TestSelfHealReplacesTheRest pins the replacement: the hurt
// fighter with a ready heal and the mana to pay casts it instead of
// sitting down.
func TestSelfHealReplacesTheRest(t *testing.T) {
    bot := healFightBot(40, 50)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()

    require.Equal(t, []int32{1216}, game.casts,
        "the ready heal replaces the sit down")
    require.Zero(t, game.sits, "the heal cast never sits")
}

// TestSelfHealWaitsForTheLandingBeforeSitting pins the flight wait:
// the server refuses the sit of a casting character, so the sit
// request stays out until the heal lands - then the still low bar
// sits down through the ordinary transition.
func TestSelfHealWaitsForTheLandingBeforeSitting(t *testing.T) {
    bot := healFightBot(40, 50)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, []int32{1216}, game.casts)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Zero(t, game.sits,
        "the sit request waits for the heal landing")

    loop.healFlightUntil = time.Now().Add(-time.Second)
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Equal(t, 1, game.sits,
        "the landed heal hands the still low bar back to the sit")
}

// TestSelfHealWithoutManaSits pins the mana gate: the bar the mana
// cannot pay sits down - the heal only replaces the rest while the
// mana lasts.
func TestSelfHealWithoutManaSits(t *testing.T) {
    bot := healFightBot(40, 5)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()

    require.Empty(t, game.casts,
        "the mana below the cost never casts the heal")
    require.Equal(t, 1, game.sits,
        "the dry bar sits down to regenerate")
}

// TestSelfHealOnReuseSits pins the reuse gate: the heal inside its
// reuse window never re-casts, the sit regenerates meanwhile.
func TestSelfHealOnReuseSits(t *testing.T) {
    bot := healFightBot(40, 50)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.skillReuse[1216] = time.Now().Add(time.Minute)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()

    require.Empty(t, game.casts,
        "the reuse window holds the heal back")
    require.Equal(t, 1, game.sits)
}

// TestSittingCharacterStandsToHeal pins the stand to cast flow: the
// server refuses the casts of a sitting character, so the deep rest
// stands up first and casts on the next window.
func TestSittingCharacterStandsToHeal(t *testing.T) {
    bot := healFightBot(55, 50)
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Empty(t, game.casts,
        "a sitting character cannot cast yet")
    require.Equal(t, 1, game.sits,
        "the stand to heal transition goes out")

    // The server confirms the stand, the next window casts.
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: false})
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()

    require.Equal(t, []int32{1216}, game.casts,
        "the standing character casts the heal")
    require.Equal(t, 1, game.sits, "the heal cast never re-sits")
}

// TestSelfHealSkippedWhileUnderAttack pins the safety gate: the
// blows landing on the character hold the heal cast back - the
// recovery never casts into the fight.
func TestSelfHealSkippedWhileUnderAttack(t *testing.T) {
    bot := healFightBot(40, 50)
    mobHitsCharacter(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.rest()

    require.Empty(t, game.casts,
        "the heal never casts while the blows land")
}

// TestMysticManaRestKeepsSittingDespiteHeal pins the mana rest
// scope: the sitting caster that regenerates its mana never stands
// for the heal - the heal would spend exactly what the sit
// rebuilds.
func TestMysticManaRestKeepsSittingDespiteHeal(t *testing.T) {
    bot := newCasterBot(30)
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 1216, Level: 1, Passive: false},
    })
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
    spawnMob(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    for range 3 {
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.tick()
    }

    require.Empty(t, game.casts,
        "the mana rest never casts the heal")
    require.Zero(t, game.sits,
        "the mana rest keeps its sit")
}

// TestNoSelfHealSkillKeepsTheOldRest pins the regression fence: a
// character without a self heal skill rests exactly as before.
func TestNoSelfHealSkillKeepsTheOldRest(t *testing.T) {
    bot := newTestBot()
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest1", Level: 10, ClassID: 18, Race: 1,
        X: 45000, Y: 50000, Z: -3500,
        MaxHP: 100, CurHP: 40, MaxMP: 100, CurMP: 50,
    })
    spawnMob(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()

    require.Empty(t, game.casts)
    require.Equal(t, 1, game.sits,
        "the heal-less character sits down as before")
}
