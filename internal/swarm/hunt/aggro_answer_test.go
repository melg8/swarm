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

// setMobHealth sets the current and maximum health of the spawned
// mob 7 (the attacker of the aggro-answer scenes): the health percent
// view of the losing-fight verdict needs both.
func setMobHealth(bot *state.Bot, hp int32) {
    bot.ApplyStatusUpdate(7, []state.Attribute{
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
    setMobHealth(bot, 100)
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
    setMobHealth(bot, 100)
    selfSwingsAt(bot, 45600)
    hurtTo(bot, 50)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.True(t, loop.losingFight(),
        "the running fight with a 50 point lead is lost")
}

// TestLosingFightFleesTheBowAddBeforeTheNearlyDeadTarget pins the bow
// add verdict of the 2026-11-10 live acceptance run: a level 8 Kaboo
// Orc Archer joined the melee fight and the shot cycle kept firing at
// a 1 percent grunt while the archer took the health from 68 to 10
// percent and the emergency logout broke the survival check. The
// nearly-dead exemption covers the melee trade only - the bow add
// outranges the retreat, so the fight is dropped at once.
func TestLosingFightFleesTheBowAddBeforeTheNearlyDeadTarget(
    t *testing.T,
) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    setMobHealth(bot, 1)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000469, Attackable: true,
        X: 45700, Y: 50000, RightHandItem: 14,
        Name: "Kaboo Orc Archer",
    })
    attackerHits(bot, 8, 45700)
    hurtTo(bot, 68)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.True(t, loop.losingFight(),
        "the bow add turns the nearly-dead target into a loss")
}

// TestLosingFightKeepsTheNearlyDeadMeleeTarget pins the other half of
// the bow add rule: a melee pack without a bow keeps the nearly-dead
// exemption, so the one-shot kill is finished instead of fled.
func TestLosingFightKeepsTheNearlyDeadMeleeTarget(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    setMobHealth(bot, 1)
    spawnMobAt(bot, 8, 45700)
    attackerHits(bot, 8, 45700)
    hurtTo(bot, 68)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.False(t, loop.losingFight(),
        "a melee add does not take the one-shot kill away")
}

// TestLosingFightStillEscapesTheCriticalHealth pins the pre-fight
// critical escape: below the escape threshold the untouched target
// does not buy a fight either.
func TestLosingFightStillEscapesTheCriticalHealth(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    setMobHealth(bot, 100)
    hurtTo(bot, 20)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7

    require.True(t, loop.losingFight(),
        "the critical health flees whatever the fight state is")
}

// TestKiteKeepsThePickWhenThePackJoins pins the target focus of the
// live kite fight: a fresh attacker no longer steals the pick while
// the bow owns the fight. Switching the arrows to every new grunt
// spread the damage across the social pack while each mob
// out-regenerated it - the 2026-11-10 run-22 acceptance window spent
// 131 seconds on a three-grunt pack and resolved no kill at all.
func TestKiteKeepsThePickWhenThePackJoins(t *testing.T) {
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45440, Y: 50000, Name: "Keltir",
    })
    mobChasesSelf(bot, 7, 45440, -3500)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000002, Attackable: true,
        X: 45100, Y: 50000, Name: "Kaboo Orc Grunt",
    })
    mobChasesSelf(bot, 8, 45100, -3500)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7
    loop.engageAt = time.Now()
    loop.lastHit = time.Now().Add(-time.Minute)
    require.False(t, bot.SelfFighting(7),
        "the reload tail carries no fresh swing")
    require.True(t, bot.SelfEngagedBy(7),
        "the pick still chases the character")

    loop.engage()
    require.Equal(t, int32(7), loop.target,
        "the live kite keeps the pick against the pack")
}

// TestKiteAnswersTheAddOnAPeacefulApproach pins the other half of
// the focus rule: a peaceful pick the add caught on the approach is
// engaged by neither side, so the nearer attacker still owns the
// next swing.
func TestKiteAnswersTheAddOnAPeacefulApproach(t *testing.T) {
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45440, Y: 50000, Name: "Keltir",
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000002, Attackable: true,
        X: 45100, Y: 50000, Name: "Kaboo Orc Grunt",
    })
    mobChasesSelf(bot, 8, 45100, -3500)
    loop := NewLoop(&fakeGame{}, bot)
    loop.target = 7
    loop.engageAt = time.Now()
    loop.lastHit = time.Now().Add(-time.Minute)
    require.False(t, bot.SelfEngagedBy(7),
        "the peaceful pick engages nobody")

    loop.engage()
    require.Equal(t, int32(8), loop.target,
        "the add owns the answer on the peaceful approach")
}

// TestTargetlessAggroKeepsFlyingTheBowAdd pins the flight
// persistence of the targetless pick: after the losing verdict drops
// a fight, the aggro answer must not re-adopt the melee chaser while
// a bow attacker still fires - every re-adoption turned the flight
// around and walked the character back into the archer range (the
// 2026-11-10 run-23 window ping-ponged through the drop/adopt cycle
// every half second and spent 40 percent of the health on it).
func TestTargetlessAggroKeepsFlyingTheBowAdd(t *testing.T) {
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000469, Attackable: true,
        X: 45350, Y: 50000, RightHandItem: 14,
        Name: "Kaboo Orc Archer",
    })
    attackerHits(bot, 8, 45350)
    spawnMobAt(bot, 9, 45200)
    mobChasesSelf(bot, 9, 45200, -3500)
    hurtTo(bot, 65)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)
    require.True(t, loop.attackerEngageable(9),
        "the melee chaser alone is engageable")

    loop.engage()
    require.Zero(t, loop.target,
        "the bow add keeps the fight dropped")
    require.NotEmpty(t, game.walks,
        "the flight owns the targetless pick")
}

// TestTargetlessAggroKeepsTheFleeSkip pins the other guard of the
// same answer: a mob the loop just fled is not re-adopted on the
// next tick, the skip holds the flight authority.
func TestTargetlessAggroKeepsTheFleeSkip(t *testing.T) {
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    spawnMobAt(bot, 7, 45200)
    mobChasesSelf(bot, 7, 45200, -3500)
    hurtTo(bot, 65)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.targetSkip = map[int32]time.Time{
        7: time.Now().Add(time.Minute),
    }

    loop.engage()
    require.Zero(t, loop.target,
        "the skipped chaser stays dropped")
    require.NotEmpty(t, game.walks,
        "the flight keeps the answer")
}
