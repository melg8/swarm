// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The delevel water loop regression of 2026-09-10: the state dump
// showed the bot cycling "level 11 is too high ... deleveling to 9 at
// the town guards" -> "walking to the guard Starden" -> "the walk
// would enter water at 40648 43432, re-pathing (1..3 of 3)" -> "town
// trip ended: aborted, the walk would cross water" every 1.3 seconds
// for 25 minutes straight. Two defects chained: the planner planned
// water crossing routes the click guard refused (fixed by the dry
// search of startWalkSegment), and the abort of a delevel walk ended only
// the town trip - the delevel state stayed armed without a cooldown,
// so the very next tick restarted the walk into the same blocker.

// TestDelevelWaterAbortArmsCooldown pins the loop break: a deleveling
// whose walk machinery aborts - here a character standing over water
// without any shore path - ends the DELEVELING (the cooldown arms, the
// walk home starts) instead of only the town trip, and the following
// ticks never restart the deleveling while the cooldown holds.
func TestDelevelWaterAbortArmsCooldown(t *testing.T) {
    loop, _, _, nav := newDelevelLoop(11)
    spawnZoneMobs(loop.tracker)
    nav.overWater = true // no escapeRoute: the shore search fails

    loop.tick()
    require.NotEqual(t, phaseDelevel, loop.phase,
        "the water abort must end the deleveling, not only the trip")
    require.False(t, loop.delevelEnd.IsZero(),
        "the abort arms the delevel cooldown")
    require.False(t, loop.delevelCooldownOver(),
        "the cooldown blocks the immediate restart")

    // The cooldown holds: no tick restarts the deleveling while it
    // runs (the old loop re-entered the delevel phase every 1.3 s).
    for range 8 {
        loop.tick()
        require.NotEqual(t, phaseDelevel, loop.phase,
            "the deleveling must stay down while the cooldown runs")
    }
}

// TestDelevelDryMissAborts pins the planner side of the same scene: a
// guard the search cannot reach aborts the deleveling at the
// planning tick - the cooldown arms and the walk home starts, the
// loop never enters the refuse-and-restart cycle.
func TestDelevelDryMissAborts(t *testing.T) {
    loop, game, _, nav := newDelevelLoop(11)
    spawnZoneMobs(loop.tracker)
    nav.miss = true

    loop.tick()
    require.NotEqual(t, phaseDelevel, loop.phase,
        "the deleveling aborts without a dry path to the guard")
    require.False(t, loop.delevelEnd.IsZero(),
        "the abort arms the delevel cooldown")
    require.False(t, loop.delevelCooldownOver(),
        "the cooldown blocks the immediate restart")
    require.Empty(t, game.walks,
        "no walk goes out without a dry plan")
}
