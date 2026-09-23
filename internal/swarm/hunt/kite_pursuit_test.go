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

// The pursuit-ledger round (issue #70, the round-11 regression
// audit): the continuation chain owns its budget apart from the
// proximity streak - a per-shot-cycle ledger that counts only the
// COMPLETED walks (the cornered hold re-probe between the walks
// never mints a stall), stops the unwinnable parity race after two
// stalled windows and the never-resolving chain at the flat
// backstop, and expires with the pursuit context (the respawned
// same-id target never inherits the chain of its predecessor). The
// tests below pin the four gates one by one on the standard kite
// scene (the 3.2 s sleep carries each walk window out - the honest
// post-walk state the live loop ticks).

// pursuitWindowLapse ages the scene past the walk window and the
// fighting stance freshness, then hands the tick back: the shared
// dance of every pursuit test (the window end is the only moment
// the hold owns the tick, the stance lapse is what the live
// post-walk state looks like).
func pursuitWindowLapse(loop *Loop) {
    time.Sleep(3200 * time.Millisecond)
    loop.lastHit = time.Now().Add(-2 * time.Second)
    loop.tick()
}

// TestKitePursuitStallRunsTheParityRace pins the round-14
// always-run rule of the progress ledger: the fake mob never closes
// the gap the walks open (the tracker keeps it at the stand it
// spawned on), so every continuation window ends where it started,
// distance-wise - the parity race the stall ledger names. The scene
// holds the mob at 260 units (the pursuit-only band: at or beyond
// the 250 RetreatRadius the proximity path stays quiet, under the
// 480 re-shot floor the pursuit hold owns the tick - at 200 the
// proximity path would intercept the post-window tick before the
// hold ever runs). The stall counter still books the parity - the
// DIAGNOSTIC the crossing log rides - but the chain KEEPS WALKING:
// the old "two stalled windows - stand and fight" answer is gone
// (the standing trade is the melee damage the kite exists to
// avoid; the moving chase outwaits the mob's home leash).
func TestKitePursuitStallRunsTheParityRace(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // Window 1 lapses: the mob holds 260 (inside the re-shot floor),
    // the first continuation issues - no accounting behind it yet.
    pursuitWindowLapse(loop)
    require.Empty(t, game.forces,
        "the re-shot waits for the regained distance")
    require.Len(t, game.walks, 2,
        "the first continuation issues on the lapsed window")
    require.Equal(t, 1, loop.kitePursuitSteps)
    require.True(t, loop.kitePursuitWalkOpen,
        "the issued walk owes its window-end accounting")

    // Window 2 lapses: the walk opened nothing (260 -> 260), the
    // first stall books - one stall alone never stopped the chain.
    pursuitWindowLapse(loop)
    require.Len(t, game.walks, 3,
        "the first stall books without stopping the chain")
    require.Equal(t, 1, loop.kitePursuitStall)
    require.Empty(t, game.forces)

    // Window 3: the second consecutive stall names the parity race
    // in the ledger - and the chain keeps running anyway (the
    // always-run rule: no standing trade answer exists anymore).
    pursuitWindowLapse(loop)
    require.Len(t, game.walks, 4,
        "the parity race keeps running - the standing trade is gone")
    require.Equal(t, 2, loop.kitePursuitStall,
        "the parity still books in the ledger")
    require.Empty(t, game.forces,
        "the re-shot still waits for the regained distance")
}

// TestKitePursuitProgressResetsTheStallLedger pins the progress
// semantics: a walk that OPENS the distance clears the stall ledger
// (the honest race must run its full course to the re-shot floor),
// and only CONSECUTIVE stalls stop the chain - a stall that follows
// a gain starts its own count.
func TestKitePursuitProgressResetsTheStallLedger(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The first continuation issues at 260.
    pursuitWindowLapse(loop)
    require.Len(t, game.walks, 2)

    // The walk opened the gap (the mob fell behind to 440): the
    // ledger clears, the chain runs on.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45440, Y: 50000, Name: "Keltir",
    })
    pursuitWindowLapse(loop)
    require.Len(t, game.walks, 3,
        "the gaining race must run on")
    require.Zero(t, loop.kitePursuitStall,
        "the opened gap clears the stall ledger")

    // The mob closes back to 260: one stall alone - the chain runs.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45260, Y: 50000, Name: "Keltir",
    })
    pursuitWindowLapse(loop)
    require.Len(t, game.walks, 4,
        "a stall after a gain starts its own count, the chain runs")
    require.Equal(t, 1, loop.kitePursuitStall)
    require.Empty(t, game.forces)
}

// TestKitePursuitStreakNeverBoundsTheChain pins the round-14
// always-run rule of the runaway ledger: a chain that never
// resolves (the oscillating chase keeps resetting the stall ledger
// without ever reaching the re-shot floor) keeps walking past the
// old budget - the fight-anchor leash and the curving circle bound
// the drift, the standing trade answer is gone.
func TestKitePursuitStreakNeverBoundsTheChain(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The chain spent the old budget without a single shot cycle
    // resolving it - the walk keeps going anyway.
    loop.kitePursuitSteps = kitePursuitStreakLimit
    pursuitWindowLapse(loop)
    require.Len(t, game.walks, 2,
        "the runaway chain keeps running - the standing trade is gone")
    require.Empty(t, game.forces,
        "the re-shot still waits for the regained distance")
}

// TestKitePursuitContextExpiresTheStaleChain pins the context
// bound the round-11 QA audit named untested: the chain re-stamps
// every walk, so a stamp older than the pursuit context belongs to
// another fight - the Mobius respawn reuses the object id, and the
// respawned mob's approach must never inherit the predecessor's
// retreat chain. The 13 s stamp (one second past the 12 s bound,
// still under the 15 s RespawnMin floor the bound sits beneath)
// refuses the continuation: the attack owns the tick.
func TestKitePursuitContextExpiresTheStaleChain(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.False(t, loop.kitePursuitAt.IsZero(),
        "the issued walk stamped the pursuit context")

    // The context ages past its bound: the continuation must refuse
    // the stale chain whatever the distance holds.
    time.Sleep(3200 * time.Millisecond)
    loop.kitePursuitAt = time.Now().Add(-13 * time.Second)
    loop.lastHit = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the stale context must not answer the continuation")
    require.Len(t, game.walks, 1,
        "no continuation walk behind a stale context")
}
