// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The travel target reset tests (the owner rule of 2026-09-21): the tasks of
// walking to town and changing the route own the target register - a route
// change drops a held engage target with its server selection, while the town
// trip start waits a fight out (two of three deaths of an observed run landed
// on a trip start under a mob).

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// holdMob arms a held fight: the mob spawns, the loop engages it and
// the server confirms the selection, the state the stuck-trip report
// showed (the loop target held, the trip start waiting on it).
func holdMob(t *testing.T, loop *Loop, game *fakeGame, bot *state.Bot) {
    t.Helper()
    spawnMob(bot)
    loop.phase = phaseEngage
    loop.target = 7
    loop.engageAt = time.Now()
    bot.ApplySelfTarget(7)
    require.Equal(t, int32(7), bot.SelfTargetID())
    require.Zero(t, game.clears)
}

// TestTripStartWaitsOutAHeldTarget pins the trip start hold: a loop with a
// full inventory and a live target keeps fighting, and the trip starts only
// once the fight ended - the walk never drops a living mob on its tail.
func TestTripStartWaitsOutAHeldTarget(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    fillInventory(bot)
    holdMob(t, loop, game, bot)

    loop.tick()

    require.Equal(t, phaseEngage, loop.phase, "the fight holds the trip")
    require.Equal(t, int32(7), loop.target, "the live target stays")
    require.Zero(t, game.clears, "no selection clears under a fight")
    require.Empty(t, game.walks, "the trip walk never starts")

    // The fight ended: the target cleared server-side and the loop dropped
    // it, so nothing binds the character any more.
    bot.ApplySelfTarget(0)
    loop.target = 0
    loop.tick()

    require.Equal(t, phaseTownWalk, loop.phase, "the trip starts")
    require.NotEmpty(t, game.walks, "the trip walk planned")
}

// TestTripStartWaitsForLootAndBlows pins the fight hold of the trip start:
// the loot phase and a character under attack still wait the trip out.
func TestTripStartWaitsForLootAndBlows(t *testing.T) {
    // The loot phase: the trip never starts over a pending pickup.
    // The loot handover ends the phase on its own when nothing lies
    // on the ground, so the tick pins the hold contract itself: no
    // clear, no trip start over the loot.
    loop, game, bot, _ := newTripLoop()
    fillInventory(bot)
    holdMob(t, loop, game, bot)
    loop.phase = phaseLoot

    loop.tick()

    require.NotEqual(t, phaseTownWalk, loop.phase, "the trip waits")
    require.Zero(t, game.clears, "the loot keeps its target")

    // The blows: an attacker on the character holds the start, the
    // trip interrupt machinery owns the answer mid trip.
    loop, game, bot, _ = newTripLoop()
    fillInventory(bot)
    spawnMob(bot)
    loop.phase = phaseEngage
    mobHitsCharacter(bot)
    loop.tick()

    require.Equal(t, phaseEngage, loop.phase, "the trip waits")
    require.Equal(t, []int32{7}, game.forces, "the engage answers the blow")
    require.Zero(t, game.clears, "nothing to clear yet")
}

// TestZoneSwitchDropsHeldTarget pins the route change drop of the
// web UI zone switch: the held fight of the old square drops with
// its server selection, the switch walk stays clean.
func TestZoneSwitchDropsHeldTarget(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    loop.SetHuntingZones(ElvenHuntingZones())
    holdMob(t, loop, game, bot)

    loop.stopForZoneSwitch()

    require.Zero(t, loop.target, "the switch drops the target")
    require.Equal(t, 1, game.clears, "the selection clears with it")
}

// TestZoneApplyDropsHeldTarget pins the applyHuntingZone drop: the
// ladder re-pick and the rotation both land in apply, the held fight
// of the old square drops there.
func TestZoneApplyDropsHeldTarget(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    holdMob(t, loop, game, bot)

    loop.applyHuntingZone(ElvenHuntingZones()[0])

    require.Zero(t, loop.target)
    require.Equal(t, 1, game.clears)
}

// TestDropAttackTargetIsANoOpWithoutATarget pins the cheap path: the
// helper sends nothing when neither the loop nor the server holds a
// target.
func TestDropAttackTargetIsANoOpWithoutATarget(t *testing.T) {
    loop, game, _, _ := newTripLoop()

    loop.dropAttackTarget("nothing held")

    require.Zero(t, game.clears)
}
