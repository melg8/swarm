// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// attackerHits models the attack broadcast of one mob: the blow lands
// on the character, so the tracker holds the mob as a live attacker
// (its target points at the character, the under attack window
// refreshes).
func attackerHits(bot *state.Bot, attackerID int32, x int32) {
    bot.ApplyAttack(state.Attack{
        AttackerID:  attackerID,
        X:           x,
        Y:           50000,
        Z:           -3500,
        TargetX:     45000,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
}

// selfSwingsAt models the attack broadcast of the character: the
// server armed the fighting stance toward the mob (the running fight
// view of SelfFighting).
func selfSwingsAt(bot *state.Bot, targetID int32, x int32) {
    bot.ApplyAttack(state.Attack{
        AttackerID:  100,
        X:           45000,
        Y:           50000,
        Z:           -3500,
        TargetX:     x,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{targetID},
        TargetCount: 1,
    })
}

// hurtTo sets the character health share (the percent of the same
// 100 max the test bot carries).
func hurtTo(bot *state.Bot, hp int32) {
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrMaxHP, Value: 100},
        {ID: state.AttrCurHP, Value: hp},
    })
}

// spawnMobAt places an attackable npc at the given x on the test
// character's y line.
func spawnMobAt(bot *state.Bot, objectID int32, x int32) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: objectID, TemplateID: 1000001, Attackable: true,
        X: x, Y: 50000, Name: "Gremlin",
    })
}

// TestLoopAnswersAttackerDuringApproach pins the approach aggro
// answer: a mob that aggros while the character walks to a peaceful
// pick is answered at once - the old flow kept chasing the pick
// while the swings of the attacker kept landing unanswered.
func TestLoopAnswersAttackerDuringApproach(t *testing.T) {
    bot := newTestBot()
    spawnMob(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, []int32{7}, game.forces)
    require.Equal(t, int32(7), loop.target)

    // The server confirms the selection, and a second mob aggros on
    // the approach.
    bot.ApplySelfTarget(7)
    spawnMobAt(bot, 8, 45500)
    attackerHits(bot, 8, 45500)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Equal(t, []int32{7, 8}, game.forces,
        "the aggroed mob outranks the peaceful pick")
    require.Equal(t, int32(8), loop.target)
}

// TestLoopKeepsTheAnsweredPick pins the no shuffle rule: a selected
// target that already holds the character as its own target IS the
// aggro answer - a second attacker does not shuffle the fight.
func TestLoopKeepsTheAnsweredPick(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    spawnMobAt(bot, 8, 45300)
    attackerHits(bot, 7, 45600)
    attackerHits(bot, 8, 45300)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, int32(8), loop.target,
        "the aggro answer picks the nearest attacker")
    require.Equal(t, []int32{8}, game.forces)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Equal(t, int32(8), loop.target,
        "the answered pick stays, no target shuffle")
    require.Equal(t, []int32{8, 8}, game.forces)
}

// TestLoopTanksAWinnablePileUp pins the tank answer of the reported
// softlock: a healthy character with two in-ceiling attackers fights
// the pair down instead of the panic run that relogged into the same
// pack. The health drop re-arms the panic run mid fight.
func TestLoopTanksAWinnablePileUp(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    spawnMobAt(bot, 8, 45400)
    attackerHits(bot, 7, 45600)
    attackerHits(bot, 8, 45400)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Zero(t, game.logouts,
        "the winnable pile up is fought, not fled")
    require.True(t, loop.panicAt.IsZero(),
        "no panic run arms while the fight stays winnable")
    require.Equal(t, int32(8), loop.target,
        "the nearest attacker is engaged")

    // The fight grinds the health down: the tank verdict flips and
    // the panic run takes over.
    hurtTo(bot, 40)
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.False(t, loop.panicAt.IsZero(),
        "the hurt character arms the pile up run")
    require.Zero(t, loop.target,
        "the run drops the fight")
    require.NotEmpty(t, game.walks,
        "the run opens the escape distance")
}

// TestFightPotionDrinksUnderHalf pins the fight potion: a running
// fight below the drink threshold (above the re-engage line, so the
// flee gates never pre-empt it) drinks a healing potion from the bag
// (the owner recipe that keeps the first pile up kill alive).
func TestFightPotionDrinksUnderHalf(t *testing.T) {
    bot := newTestBot()
    spawnMob(bot)
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 601, ItemID: 1060, Count: 5},
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    // The running fight: the mob swings, the character swings back.
    // The target is set directly - the aggro answer would flee the
    // hurt character below the re-engage health.
    mobHitsCharacter(bot)
    selfSwingsAt(bot, 7, 45600)
    hurtTo(bot, 40)
    loop.target = 7

    loop.tick()
    require.Contains(t, game.uses, int32(601),
        "the fight below the drink threshold drinks the potion")

    // The reuse window holds the next potion back.
    game.uses = nil
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Empty(t, game.uses,
        "the potion reuse window paces the uses")
}
