// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// settleTestWorld places one aggressive orc 200 units east of the
// character: inside its own on-sight circle, so the first move breaks
// the spawn protection straight into the aggro.
func settleTestWorld(bot *state.Bot) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID:   9001,
        TemplateID: npcdata.NPCWireTemplateID(20472),
        Attackable: true,
        X:          45200, Y: 50000, Z: -3500,
        Name: "Kaboo Orc",
    })
}

// settleTestBot prepares a hurt character inside the aggressive
// ground with the settle enabled.
func settleTestBot(t *testing.T) (*state.Bot, *fakeGame, *Loop) {
    t.Helper()
    bot := newTestBot()
    hurtTo(bot, 40)
    settleTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.EnableSpawnSettle()
    loop.lastHit = time.Now().Add(-time.Minute)

    return bot, game, loop
}

// TestSettleHoldsUntilRecovered walks the full settle: the hold sits
// the hurt character down (the sit never clears the protection), the
// recovery stands it up and the first strike answers the aggressive
// mob from the spot - no walk ever breaks the protection.
func TestSettleHoldsUntilRecovered(t *testing.T) {
    bot, game, loop := settleTestBot(t)

    loop.tick()
    require.Equal(t, 1, game.sits, "the settle sits down to regenerate")
    require.Empty(t, game.walks, "the settle never moves")
    require.Empty(t, game.forces, "the settle never attacks")
    require.False(t, loop.settled, "the settle still holds")

    // The sit confirms; the hold keeps sitting while the health
    // regenerates.
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
    loop.tick()
    require.Equal(t, 1, game.sits, "no double toggle while sitting")
    require.False(t, loop.settled)

    // The recovery finishes: the stand transition runs first (the
    // sit request guard is waited out white box - the tests drive
    // the ticks in microseconds, the real pacing is seconds).
    hurtTo(bot, 95)
    loop.restActionAt = time.Now().Add(-time.Minute)
    loop.tick()
    require.Equal(t, 2, game.sits, "the recovered character stands up")
    require.False(t, loop.settled, "the strike waits for the stand")

    // The stand confirms; the settle window of the stand guard is
    // waited out white box (the 3 s server stand animation).
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: false})
    loop.restActionAt = time.Now().Add(-time.Minute)
    loop.tick()
    require.True(t, loop.settled, "the settle ends recovered")
    require.Equal(t, []int32{9001}, game.forces,
        "the first strike answers the aggressive mob from the spot")
    require.Empty(t, game.walks,
        "the strike never walks - the mob comes to the character")
}

// TestSettleArmsTheBowFirstStrike pins the ranged first strike: the
// hunter that owns the bow arms it on the spot and fires while the
// mob still has to close the distance (the lure shoot phase).
func TestSettleArmsTheBowFirstStrike(t *testing.T) {
    bot := newTestBot()
    hurtTo(bot, 95)
    settleTestWorld(bot)
    // The melee kit of the lure tests: the sword worn, the bow and
    // the quiver in the bag.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
        {ObjectID: 502, ItemID: 13, Count: 1},
        {ObjectID: 503, ItemID: 17, Count: 400},
    })
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 501
    bot.ApplyPaperdoll(paperdoll)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.EnableSpawnSettle()
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.NotNil(t, loop.lure, "the first strike arms the bow lure")
    require.Equal(t, lureArm, loop.lure.phase)
    require.Equal(t, int32(9001), loop.lure.target)
    require.Equal(t, int32(501), loop.lure.meleeObjID,
        "the melee weapon is remembered for the swap back")
    require.Contains(t, game.uses, int32(502),
        "the bow equip fires on the spot")
    require.Empty(t, game.walks,
        "the strike holds the spot, no standoff walk")
}

// TestSettleEndsOnBlow pins the blow abort: the protection strips
// aggro, it never absorbs damage - a landed blow means the ground
// reached the character anyway and the ordinary safety flow answers.
func TestSettleEndsOnBlow(t *testing.T) {
    bot, game, loop := settleTestBot(t)

    // A blow lands before the settle even starts.
    attackerHits(bot, 9001, 45200)
    loop.tick()
    require.True(t, loop.settled, "the blow ends the settle")
    require.Zero(t, game.sits, "the settle never sits into the blows")
    require.NotEmpty(t, game.walks,
        "the hurt character under attack keeps fleeing")
}

// TestSettleOffByDefault pins the opt in: the loops the tests and the
// manual sessions build never settle - the ordinary pick flow runs.
func TestSettleOffByDefault(t *testing.T) {
    bot := newTestBot()
    hurtTo(bot, 40)
    settleTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.True(t, loop.settleHoldAt.IsZero(),
        "no settle hold runs without the enable")
    require.Equal(t, 1, game.sits,
        "the ordinary rest flow owns the hurt character")
    require.Empty(t, game.walks,
        "the hurt character rests instead of walking")
    require.Empty(t, game.forces,
        "the hurt character never attacks from the rest")
}
