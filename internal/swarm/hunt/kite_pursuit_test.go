// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The pursuit hold of the archer kite (issue #70, the owner rule of
// the 2026-11-09 round): the retreat must keep running between the
// shots. A walk window that ends while the nearest hostile still
// holds inside the re-shot floor re-arms one more straight walk, so
// the chaser spends every reload tail closing the distance again -
// the archer never stands through the reload and the re-shot fires
// from the regained floor instead of the range the windup collapsed.
// The tests pin the continuation, the straight geometry it keeps,
// the per-cycle ledger reset, the stall exit of the spent budget and
// the context expiry of a stale fight (the Mobius respawn keeps the
// object id).

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// lapseKiteWalk ages the in-flight kite walk past its window so the
// next tick lands on the window-end branch of the ladder - the exact
// moment the live server's disable window lapses under the retreat.
// The shot and the fight view are left to the caller (the stall
// scenes backdate them so the forced attack owns the post-walk
// tick).
func lapseKiteWalk(loop *Loop) {
    until := time.Now().Add(-time.Millisecond)
    // The live walk arms the movement hold with exactly its own
    // window end: the two lapse together, so the aged window must
    // carry the hold with it.
    if loop.combatAvoidUntil.Equal(loop.kiteWalkUntil) {
        loop.combatAvoidUntil = until
    }
    loop.kiteWalkUntil = until
    loop.lastHit = time.Now().Add(-2 * engageRetryPeriod)
}

// TestKitePursuitContinuesTheRetreatAtTheWindowEnd pins the core
// contract: the walk window ends with the hostile still inside the
// re-shot floor, so the retreat re-arms instead of standing the
// reload out - and the continuation keeps the STRAIGHT away-ray (the
// owner rule of the round: no fan, no turn; the archer runs the same
// line it was already running).
func TestKitePursuitContinuesTheRetreatAtTheWindowEnd(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1, "the shot cycle opened the retreat")
    require.Zero(t, loop.kitePursuitSteps,
        "the opening walk is not a continuation")

    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the lapsed window re-arms the retreat under the floor")
    require.Equal(t, game.walks[0], game.walks[1],
        "the continuation keeps the straight away-ray")
    require.Equal(t, 1, loop.kitePursuitSteps)
    require.True(t, loop.kitePursuitWalkOpen,
        "the issued walk owes its window-end accounting")
    require.Empty(t, game.forces,
        "the re-shot waits for the regained distance")
}

// TestKitePursuitOpensTheDistanceThenTheShotFires pins the win side
// of the race: once the hostile clears the re-shot floor the hold
// stands down, and the forced attack re-request fires the bow from
// the opened distance (the one shot per retreat cycle the owner
// asked for).
func TestKitePursuitOpensTheDistanceThenTheShotFires(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The race was won: the chaser fell behind to 600 units while
    // the last shot went out well before the window end (the stale
    // fight view of the honest post-walk state - the forced attack
    // owns the tick once the hold stands down).
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45600, Y: 50000, Name: "Keltir",
    })
    bot.ApplyAttackAt(shotOf(45600), time.Now().Add(-time.Minute))
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the won race issues no continuation")
    require.Equal(t, []int32{7}, game.forces,
        "the re-shot fires from the regained floor")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the arrival stand-down clears the ladder")
}

// TestKitePursuitStallStandsToShoot pins the stall exit of the
// progress ledger: the fake mob never closes the gap the walks open
// (the tracker keeps it at the stand it spawned on), so every
// continuation window ends where it started, distance-wise. A chain
// that spent its walk budget (the opening walk plus the
// continuations) with the race still stalled stands the walk down
// and hands the tick to the forced attack - the bow fires from the
// hold instead of walking the pocket forever (the temp18 acceptance
// dump of 2026-11-09: 33 walks and 63 seconds without a shot).
func TestKitePursuitStallStandsToShoot(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    // The drought of the walled pocket: the last shot went out long
    // before the window end, so the fight view is stale and the
    // stand-to-shoot forced attack below the fight branch owns the
    // exit tick.
    bot.ApplyAttackAt(shotOf(45200), time.Now().Add(-time.Minute))

    // Window 1 lapses: the mob holds 200 (inside the 480 re-shot
    // floor), the first continuation issues - no accounting behind
    // it yet.
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the first continuation issues on the lapsed window")
    require.Equal(t, 1, loop.kitePursuitSteps)
    require.Empty(t, game.forces,
        "the re-shot waits for the regained distance")

    // Window 2 lapses: the walk opened nothing (200 -> 200), the
    // first stall books - one stall alone never stopped the chain.
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 3,
        "the first stall books without stopping the chain")
    require.Equal(t, 1, loop.kitePursuitStall)
    require.Empty(t, game.forces)

    // Window 3: the second consecutive stall names the parity race
    // in the ledger. The chain still has the tail of its walk
    // budget, so the continuation issues once more.
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 4,
        "the parity crossing alone does not stop the chain")
    require.Equal(t, 2, loop.kitePursuitStall,
        "the parity still books in the ledger")
    require.Empty(t, game.forces,
        "the re-shot still waits for the regained distance")

    // Window 4: the budget is spent and the race still stalled -
    // the stall exit stands the walk down and the forced attack
    // re-request fires the bow from the hold.
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 4,
        "the lost race must not issue another walk")
    require.Equal(t, []int32{7}, game.forces,
        "the forced attack re-request fires the bow from the hold")
    require.Zero(t, loop.kitePursuitFor,
        "the chain is spent until the next shot cycle re-arms it")
}

// TestKitePursuitProgressResetsTheStallLedger pins the progress
// semantics: a walk that OPENS the distance clears the stall ledger
// (the honest race must run its full course to the re-shot floor),
// and only CONSECUTIVE stalls inside the spent walk budget stop the
// chain - a stall that follows a gain starts its own count.
func TestKitePursuitProgressResetsTheStallLedger(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)

    // The first continuation issues at 200.
    lapseKiteWalk(loop)
    loop.tick()
    require.Equal(t, 1, loop.kitePursuitSteps)

    // The walk opened the gap (the chaser fell behind to 400): the
    // ledger clears, the chain runs on.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45400, Y: 50000, Name: "Keltir",
    })
    lapseKiteWalk(loop)
    loop.tick()
    require.Equal(t, 2, loop.kitePursuitSteps,
        "the gaining race must run on")
    require.Zero(t, loop.kitePursuitStall,
        "the opened gap clears the stall ledger")

    // The mob closes back to 200: one stall alone - the chain runs.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, Name: "Keltir",
    })
    lapseKiteWalk(loop)
    loop.tick()
    require.Equal(t, 3, loop.kitePursuitSteps,
        "a stall after a gain starts its own count, the chain runs")
    require.Equal(t, 1, loop.kitePursuitStall)
}

// TestKitePursuitContextExpiresTheStaleChain pins the context bound:
// the chain re-stamps every walk, so a stamp older than the pursuit
// context belongs to another fight - the Mobius respawn reuses the
// object id, and the respawned mob's approach must never inherit the
// predecessor's retreat chain.
func TestKitePursuitContextExpiresTheStaleChain(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.False(t, loop.kitePursuitAt.IsZero(),
        "the issued walk stamped the pursuit context")
    bot.ApplyAttackAt(shotOf(45200), time.Now().Add(-time.Minute))

    loop.kitePursuitAt = time.Now().Add(
        -kitePursuitContext - time.Second)
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "no continuation walk behind a stale context")
    require.Equal(t, []int32{7}, game.forces,
        "the stale context lets the attack own the tick")
}

// TestKitePursuitLedgerResetsOnTheFreshShotCycle pins the per-cycle
// reset: every fresh shot starts its own distance race, so the steps,
// the stalls and the open walk of the previous cycle never inherit
// into the new one - and the reset is idempotent across the ticks
// that still catch the same broadcast.
func TestKitePursuitLedgerResetsOnTheFreshShotCycle(t *testing.T) {
    _, _, loop := kiteBowBot(t, 45200)
    loop.kitePursuitSteps = 3
    loop.kitePursuitDist = 200
    loop.kitePursuitStall = 2
    loop.kitePursuitWalkOpen = true

    shotAt := time.Now()
    loop.kitePursuitCycle(shotAt)
    require.Zero(t, loop.kitePursuitSteps,
        "the fresh shot cycle clears the step budget")
    require.Zero(t, loop.kitePursuitStall,
        "the fresh shot cycle clears the stall ledger")
    require.False(t, loop.kitePursuitWalkOpen,
        "the fresh shot cycle clears the open walk")
    require.Zero(t, loop.kitePursuitDist)

    // The very same broadcast seen again changes nothing: the reset
    // belongs to the NEW shot alone.
    loop.kitePursuitSteps = 1
    loop.kitePursuitCycle(shotAt)
    require.Equal(t, 1, loop.kitePursuitSteps,
        "the same broadcast must not rewrite the ledger twice")
}

// TestKitePursuitLedgerResetsOnAFreshWalkContext pins the second
// reset seam: a walk issued outside the previous chain's context (a
// new fight, the respawned same-id target) starts its own race - the
// proximity-first fights never see a shot, so the shot-cycle reset
// alone would carry the old stalls into the fresh fight.
func TestKitePursuitLedgerResetsOnAFreshWalkContext(t *testing.T) {
    _, _, loop := kiteBowBot(t, 45200)
    loop.kitePursuitSteps = 3
    loop.kitePursuitStall = 2
    loop.kitePursuitWalkOpen = true
    loop.kitePursuitFor = 7
    loop.kitePursuitAt = time.Now().Add(
        -kitePursuitContext - time.Second)

    loop.kiteIssueWalk(
        time.Now(), time.Now().Add(time.Second),
        45000, 50000, -3500, 44278, 50000)
    require.Zero(t, loop.kitePursuitSteps,
        "the fresh context clears the step budget")
    require.Zero(t, loop.kitePursuitStall,
        "the fresh context clears the stall ledger")
    require.False(t, loop.kitePursuitWalkOpen,
        "the fresh context clears the open walk")
    require.Equal(t, int32(7), loop.kitePursuitFor,
        "the issued walk stamps its own fight")
    require.False(t, loop.kitePursuitAt.IsZero(),
        "the issued walk re-stamps the context")
}
