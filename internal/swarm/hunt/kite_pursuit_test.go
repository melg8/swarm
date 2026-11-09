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
// never mints a stall), stands the walk down when the race spent
// its walk budget with two stalled windows behind it (the stall
// exit the temp18 acceptance dump of 2026-11-09 forced: 33 walks
// and 63 seconds without a shot in the walled elven pocket), and
// expires with the pursuit context (the respawned same-id target
// never inherits the chain of its predecessor). The tests below pin
// the gates one by one on the standard kite scene (the lapse dance
// of pursuitWindowLapse carries each scaled walk window of the race
// leg out - leg/400 * the bow window, the 662 leg of the 260-unit
// mob holds it for ~3.3s - and lands the arrival shot of the leg
// the way the live server does).

// pursuitWindowLapse ages the scene past the scaled walk window of
// the race leg (662/400 * the 2s fallback = 3.31s), lands the
// ARRIVAL SHOT of the leg (the server's auto re-shot the armed
// stance fires the moment the walk ends - the live fight view rides
// those shots through a chain the fake mob never swings into) and
// hands the tick back: the shared dance of the every-window tests
// (the window end is the only moment the hold owns the tick, the
// stance lapse is what the live post-walk state looks like).
func pursuitWindowLapse(bot *state.Bot, loop *Loop, mobX int32) {
    agePursuitWindow(loop)
    selfSwingsAt(bot, mobX)
    loop.tick()
}

// agePursuitWindow ages the scene past the walk window: the walk
// leg outlasts the 2s window (see pursuitWindowLapse), and the
// forced attack pacing gate needs the last request backdated.
func agePursuitWindow(loop *Loop) {
    time.Sleep(4 * time.Second)
    loop.lastHit = time.Now().Add(-2 * time.Second)
}

// pursuitWindowLapseQuiet ages the scene past the walk window
// WITHOUT landing the arrival shot - the droughted state the stall
// exit exists for: the live race spent its last shot long before the
// window end, so the fight view is stale and the stand-to-shoot
// forced attack below the fight branch owns the tick (a fresh swing
// would route the tick into the fighting branch, where the shot
// trigger arms the next retreat instead). The engage clock re-anchors
// here the way the live chase steps re-anchor it: the fake mob never
// moves and the scene never fights in the branch, so without the
// stamp the 12s stuck timeout drops the target before the stall exit
// runs.
func pursuitWindowLapseQuiet(loop *Loop) {
    agePursuitWindow(loop)
    loop.engageAt = time.Now()
    loop.tick()
}

// TestKitePursuitStallStandsToShoot pins the stall exit of the
// progress ledger: the fake mob never closes the gap the walks open
// (the tracker keeps it at the stand it spawned on), so every
// continuation window ends where it started, distance-wise. A chain
// that spent its walk budget (the opening walk plus the
// continuations) with the race still stalled stands the walk down
// and hands the tick to the forced attack - the bow fires from the
// hold instead of walking the pocket forever (the temp18 acceptance
// dump of 2026-11-09: 33 walks and 63 seconds without a shot). The
// scene holds the mob at 260 units (the pursuit-only band: at or
// beyond the 250 RetreatRadius the proximity path stays quiet, under
// the 480 re-shot floor the pursuit hold owns the tick - at 200 the
// proximity path would intercept the post-window tick before the
// hold ever runs).
func TestKitePursuitStallStandsToShoot(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // Window 1 lapses: the mob holds 260 (inside the re-shot floor),
    // the first continuation issues - no accounting behind it yet.
    pursuitWindowLapse(bot, loop, 45260)
    require.Empty(t, game.forces,
        "the re-shot waits for the regained distance")
    require.Len(t, game.walks, 2,
        "the first continuation issues on the lapsed window")
    require.Equal(t, 1, loop.kitePursuitSteps)
    require.True(t, loop.kitePursuitWalkOpen,
        "the issued walk owes its window-end accounting")

    // Window 2 lapses: the walk opened nothing (260 -> 260), the
    // first stall books - one stall alone never stopped the chain.
    pursuitWindowLapse(bot, loop, 45260)
    require.Len(t, game.walks, 3,
        "the first stall books without stopping the chain")
    require.Equal(t, 1, loop.kitePursuitStall)
    require.Empty(t, game.forces)

    // Window 3: the second consecutive stall names the parity race
    // in the ledger. The chain still has the tail of its walk
    // budget, so the continuation issues once more.
    pursuitWindowLapse(bot, loop, 45260)
    require.Len(t, game.walks, 4,
        "the parity crossing alone does not stop the chain")
    require.Equal(t, 2, loop.kitePursuitStall,
        "the parity still books in the ledger")
    require.Empty(t, game.forces,
        "the re-shot still waits for the regained distance")

    // Window 4: the budget is spent and the race still stalled -
    // the stall exit stands the walk down and the forced attack
    // re-request owns the tick: the bow fires from the hold. This
    // lapse lands NO arrival shot (the drought the exit exists
    // for): the stale fight view lets the tick past the fighting
    // branch to the forced attack, exactly like the live race whose
    // last shot went out a minute ago.
    pursuitWindowLapseQuiet(loop)
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
    bot, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The first continuation issues at 260.
    pursuitWindowLapse(bot, loop, 45260)
    require.Len(t, game.walks, 2)

    // The walk opened the gap (the mob fell behind to 440): the
    // ledger clears, the chain runs on.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45440, Y: 50000, Name: "Keltir",
    })
    pursuitWindowLapse(bot, loop, 45440)
    require.Len(t, game.walks, 3,
        "the gaining race must run on")
    require.Zero(t, loop.kitePursuitStall,
        "the opened gap clears the stall ledger")

    // The mob closes back to 260: one stall alone - the chain runs.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45260, Y: 50000, Name: "Keltir",
    })
    pursuitWindowLapse(bot, loop, 45260)
    require.Len(t, game.walks, 4,
        "a stall after a gain starts its own count, the chain runs")
    require.Equal(t, 1, loop.kitePursuitStall)
    require.Empty(t, game.forces)
}

// TestKitePursuitStreakAloneNeverBoundsTheChain pins the split
// between the two ledgers: the streak counter (walks without a
// shot) is a DIAGNOSTIC and never stops a chain on its own - the
// stall exit (kitePursuitDeadlockSteps) owns the chain whose windows
// stopped paying, so a chain that keeps opening distance still runs
// out its race whatever the streak holds.
func TestKitePursuitStreakAloneNeverBoundsTheChain(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45260)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The chain carries the full streak budget, but the current
    // window has no accounting behind it (the opening walk never
    // mints a stall): the continuation issues and the walk keeps
    // running.
    loop.kitePursuitSteps = kitePursuitStreakLimit
    pursuitWindowLapse(bot, loop, 45260)
    require.Len(t, game.walks, 2,
        "the streak alone never bounds the chain")
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
    // the stale chain whatever the distance holds. The sleep first
    // carries the scaled walk window of the race leg out (6.5s),
    // so the refused tick lands on the honest post-walk state -
    // the ladder stands down, the attack owns the tick.
    time.Sleep(7000 * time.Millisecond)
    loop.kitePursuitAt = time.Now().Add(-13 * time.Second)
    loop.lastHit = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the stale context must not answer the continuation")
    require.Len(t, game.walks, 1,
        "no continuation walk behind a stale context")
}
