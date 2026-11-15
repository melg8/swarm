// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The loot interrupt regressions: a mob that reaches the character
// while it picks up the kill hands the tick to the engage, and a
// refused pickup skips its drop instead of retrying it.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestLootAnswersTheLiveAttacker pins the loot interrupt: a mob that
// reaches the character while it picks up the kill hands the tick to
// the engage instead of standing in the blows for the next drop.
func TestLootAnswersTheLiveAttacker(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    attackerHits(bot, 7, 45600)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.phase = phaseLoot

    loop.loot()
    require.Equal(t, phaseEngage, loop.phase,
        "the attacker interrupts the loot phase")
    require.Equal(t, int32(7), loop.target,
        "the live attacker owns the tick before the next corpse step")
}

// TestLootFleesTheUnwinnableAttacker pins the defensive half of the
// loot interrupt: a hurt character picking up a kill does not tank a
// fresh attacker - the kill loot stays and the escape opens.
func TestLootFleesTheUnwinnableAttacker(t *testing.T) {
    bot := newTestBot()
    spawnMobAt(bot, 7, 45600)
    attackerHits(bot, 7, 45600)
    hurtTo(bot, 40)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.phase = phaseLoot

    loop.loot()
    require.Equal(t, phaseEngage, loop.phase)
    require.Zero(t, loop.target)
    require.NotEmpty(t, game.walks,
        "the hurt loot phase opens the escape")
}

// corpseWithDrop kills the spawned mob and leaves one adena drop next to
// the corpse, the state the loot phase starts from.
func corpseWithDrop(bot *state.Bot) {
    spawnMob(bot)
    bot.ApplySpawnItem(state.ItemInfo{
        ObjectID: 9, TemplateID: 57, X: 45040, Y: 50040, Z: -3500,
    })
    bot.ApplyStatusUpdate(7, []state.Attribute{
        {ID: state.AttrCurHP, Value: 0},
    })
    bot.ApplySelfTarget(7)
}

// TestLootSkipsTheRefusedPickup pins the refusal skip: an ActionFailed that
// answers the pickup request skips the drop for the retry delay instead of
// herding the refusal storm (57 percent of the observed refusals landed right
// after a kill, and the 20 second pickup timeout retried every one of them).
func TestLootSkipsTheRefusedPickup(t *testing.T) {
    bot := newTestBot()
    corpseWithDrop(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.lastActionAt = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, phaseLoot, loop.phase)
    require.Equal(t, []int32{9}, game.pickups, "the drop is clicked")

    // The server refuses the pickup: the answer lands right after the
    // request (a fleet mate took the drop).
    bot.ApplyActionFailed(time.Now().Add(time.Millisecond))
    loop.tick()

    require.Zero(t, loop.lootID, "the refused drop stays out of flight")
    skipUntil, skipped := loop.itemSkip[9]
    require.True(t, skipped, "the drop lands on the skip list")
    require.True(t, skipUntil.After(time.Now()), "the skip is armed")

    // The drop is never clicked again inside the skip window.
    loop.tick()
    loop.tick()
    require.Equal(t, []int32{9}, game.pickups,
        "the refused drop is not retried")
}

// TestLootKeepsTheUnrefusedPickup is the control of the skip: a drop whose
// pickup got no ActionFailed stays in flight and is retried by the ordinary
// pacing.
func TestLootKeepsTheUnrefusedPickup(t *testing.T) {
    bot := newTestBot()
    corpseWithDrop(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.lastActionAt = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, []int32{9}, game.pickups)

    // The walk click of the corpse approach answers ActionFailed instead:
    // a refusal older than the pickup send belongs to another request.
    bot.ApplyActionFailed(time.Now().Add(-5 * time.Second))
    loop.tick()

    require.Equal(t, int32(9), loop.lootID,
        "the unrefused pickup stays in flight")
    require.NotContains(t, loop.itemSkip, int32(9))
}
