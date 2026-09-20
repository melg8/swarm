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
// for 25 minutes straight. The planner side retired with the priced
// mesh round (the water is walkable at the swim rate); the loop break
// pin below keeps the honest abort contract: a guard the search
// cannot reach ends the DELEVELING with its cooldown, the walk home
// starts, and no tick restarts the deleveling while the cooldown
// holds.

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
