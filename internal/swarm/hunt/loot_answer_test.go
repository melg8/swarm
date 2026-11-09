// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The loot interrupt regressions: a mob that reaches the character
// while it picks up the kill hands the tick to the engage.

import (
    "testing"

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
