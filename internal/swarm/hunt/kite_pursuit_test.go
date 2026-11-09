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
// The chain has NO stall exit: a parity race that opens nothing still
// trades the melee blows for chase-cadence blows, so only a walled
// lane stands the archer down. The tests pin the continuation, the
// straight geometry it keeps, the per-cycle ledger reset, the
// never-stand slow race, the context expiry of a stale fight (the
// Mobius respawn keeps the object id), the conservative closed
// distance the floors read and the forced-attack deferral that keeps
// the collapsed shot from firing at all.

import (
    "bytes"
    "log"
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// lapseKiteWalk ages the in-flight kite walk past its window so the
// next tick lands on the window-end branch of the ladder - the exact
// moment the live server's disable window lapses under the retreat.
// The shot and the fight view are left to the caller (the slow-race
// scenes backdate them so the forced attack would own a tick the
// pursuit must keep instead).
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

// TestKitePursuitNeverStandsTheSlowRaceDown pins the removal of the
// stall exit (the owner rule: the bot must keep running even after
// the mob stands point blank between the shots, so it spends the next
// cycle closing the distance again). The fake mob never closes the
// gap the walks open and the fake character never moves, so every
// window ends where it started - the old progress ledger read that as
// an unwinnable race and stood the walk down to shoot. The chain now
// keeps re-arming through the diagnostic bound: every lapsed window
// issues its straight walk and the forced attack never owns a tick
// until the hostile clears the floor or the lane is walled.
func TestKitePursuitNeverStandsTheSlowRaceDown(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    var logBuf bytes.Buffer
    loop.SetLogger(log.New(&logBuf, "", 0))
    // The drought of the walled pocket: the last shot went out long
    // before the window ends, so the forced attack would own every
    // tick the pursuit does not.
    bot.ApplyAttackAt(shotOf(45200), time.Now().Add(-time.Minute))

    for step := 1; step <= kitePursuitStreakLimit+1; step++ {
        lapseKiteWalk(loop)
        loop.tick()
        require.Len(t, game.walks, 1+step,
            "every lapsed window re-arms the retreat (step %d)", step)
        require.Equal(t, step, loop.kitePursuitSteps,
            "the continuation ledger counts every walk")
        require.Empty(t, game.forces,
            "the collapsed re-shot waits for the regained floor")
    }
    require.Equal(t, 1,
        bytes.Count(logBuf.Bytes(), []byte("without a shot")),
        "crossing the diagnostic bound names the runaway once")
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
// reset: every fresh shot starts its own distance race, so the steps
// of the previous cycle never inherit into the new one - and the
// reset is idempotent across the ticks that still catch the same
// broadcast.
func TestKitePursuitLedgerResetsOnTheFreshShotCycle(t *testing.T) {
    _, _, loop := kiteBowBot(t, 45200)
    loop.kitePursuitSteps = 3

    shotAt := time.Now()
    loop.kitePursuitCycle(shotAt)
    require.Zero(t, loop.kitePursuitSteps,
        "the fresh shot cycle clears the step budget")

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
// alone would carry the old steps into the fresh fight.
func TestKitePursuitLedgerResetsOnAFreshWalkContext(t *testing.T) {
    _, _, loop := kiteBowBot(t, 45200)
    loop.kitePursuitSteps = 3
    loop.kitePursuitFor = 7
    loop.kitePursuitAt = time.Now().Add(
        -kitePursuitContext - time.Second)

    loop.kiteIssueWalk(
        time.Now(), time.Now().Add(time.Second),
        45000, 50000, -3500, 44278, 50000)
    require.Zero(t, loop.kitePursuitSteps,
        "the fresh context clears the step budget")
    require.Equal(t, int32(7), loop.kitePursuitFor,
        "the issued walk stamps its own fight")
    require.False(t, loop.kitePursuitAt.IsZero(),
        "the issued walk re-stamps the context")
}

// TestClosedDistanceBoundsTheChaserLag pins the conservative closure
// bound itself: the raw gap of a lagging movement broadcast shrinks
// by the fastest run of the segment age, never below zero, and a
// standing or speedless scene keeps the raw distance.
func TestClosedDistanceBoundsTheChaserLag(t *testing.T) {
    require.InDelta(t, 512.0, closedDistance(512, 0, 120), 0.001,
        "a fresh broadcast keeps the raw gap")
    require.InDelta(t, 392.0,
        closedDistance(512, time.Second, 120), 0.001,
        "a one second old segment shrinks by the run speed")
    require.InDelta(t, 0.0,
        closedDistance(100, 2*time.Second, 120), 0.001,
        "the closure never reads negative")
    require.InDelta(t, 512.0, closedDistance(512, time.Second, 0), 0.001,
        "a speedless scene keeps the raw gap")
    require.InDelta(t, 512.0,
        closedDistance(512, -time.Second, 120), 0.001,
        "a future stamp keeps the raw gap")
}

// TestKiteThreatUsesTheClosedDistance pins the wiring of the bound
// into the threat scan: the chaser's last movement broadcast stands
// 512 units out (beyond the 480 re-shot floor), but the segment aged
// while the mob kept running, so the live gap sits under the floor.
// The 2026-11-09 acceptance run measured exactly this lag (512 raw
// against 256 actual).
func TestKiteThreatUsesTheClosedDistance(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45512)
    // The chaser runs: fast enough to cross the 32 unit gap the raw
    // reading hides inside a few milliseconds of segment age.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45512, Y: 50000, RunSpeed: 2000, Name: "Keltir",
    })
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 7, TargetID: 100, Distance: 40,
        X: 45512, Y: 50000, TargetX: 45000, TargetY: 50000,
        TargetZ: -3500,
    })
    time.Sleep(150 * time.Millisecond)

    id, dist, ok := loop.kiteThreat(
        time.Now(), 45000, 50000, -3500)
    require.True(t, ok)
    require.Equal(t, int32(7), id)
    require.Less(t, dist, kiteReshotFloor,
        "the moving chaser closed the raw 512 gap")
    require.Greater(t, dist, 0.0, "the closure stays a distance")

    // The standing twin keeps the raw reading: no movement segment,
    // no closure.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45512, Y: 50000, Name: "Keltir",
    })
    _, dist, ok = loop.kiteThreat(time.Now(), 45000, 50000, -3500)
    require.True(t, ok)
    require.InDelta(t, 512.0, dist, 0.001,
        "a standing chaser keeps the raw gap")
}

// TestKiteDeferShotRunsTheRetreatInsteadOfTheCollapsedShot pins the
// gate of the forced attack itself: a live bow fight whose retreat
// already ran holds the re-shot while the (closed) distance sits
// under the re-shot floor - the request would commit the next windup
// at the collapsed range and hand the chaser the melee. The clock in
// this scene has the same shape as the live acceptance finding: the
// raw broadcast sits past the floor, the running segment puts the
// live gap under it.
func TestKiteDeferShotRunsTheRetreatInsteadOfTheCollapsedShot(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45512)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45512, Y: 50000, RunSpeed: 2000, Name: "Keltir",
    })
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 7, TargetID: 100, Distance: 40,
        X: 45512, Y: 50000, TargetX: 45000, TargetY: 50000,
        TargetZ: -3500,
    })
    loop.kitePursuitFor = 7
    loop.kitePursuitAt = time.Now()
    time.Sleep(150 * time.Millisecond)

    require.True(t, loop.kiteDeferShot(time.Now()),
        "the collapsed re-shot defers to the retreat")
    require.Len(t, game.walks, 1, "the deferral runs one straight walk")
    require.Less(t, game.walks[0][0], int32(45000),
        "the walk keeps the away-ray")
    require.Empty(t, game.forces,
        "no attack request commits the collapsed shot")
}

// TestKiteDeferShotProtectsTheFreshFight pins the birth half of the
// gate: the deferral belongs to a fight a kite retreat ACTUALLY served
// in the pursuit context. A fresh engage has no retreat behind it -
// the bow must land its first shot - and the attack request stays
// with the fight machinery.
func TestKiteDeferShotProtectsTheFreshFight(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    require.False(t, loop.kiteDeferShot(time.Now()),
        "the first engage keeps its attack request")
    require.Empty(t, game.walks,
        "the first engage issues no retreat behind it")
}

// TestKiteDeferShotLetsTheCorneredHoldShoot pins the exception: a
// genuinely blocked lane is the one legitimate standing-shoot case -
// the lane battery answers the hold, and the forced attack request
// keeps firing the bow at melee range.
func TestKiteDeferShotLetsTheCorneredHoldShoot(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.SetNavigator(&fakeNavigator{refuseClicks: true})
    loop.kitePursuitFor = 7
    loop.kitePursuitAt = time.Now()

    require.False(t, loop.kiteDeferShot(time.Now()),
        "a walled lane never holds the shot")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the cornered hold owns the cycle")
    require.Empty(t, game.walks, "the walled lane issues no walk")
}

// TestKiteLaneResolveRejectsThePartialRun pins the floor of the
// validated run: a click the server collapses onto a few dozen units
// (the 38 unit partial of the 2026-11-09 acceptance run) buys no
// distance and spends a whole reload window, so the lane battery
// treats it as no lane. A validated run past the shortest useful hop
// is a lane and its endpoint serves as the destination.
func TestKiteLaneResolveRejectsThePartialRun(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    nav := &fakeNavigator{}
    nav.validateHook = func(
        from, _ pathfind.Vec3,
    ) (pathfind.Vec3, bool) {
        return pathfind.Vec3{
            X: from.X - 38, Y: from.Y, Z: from.Z}, true
    }
    loop.SetNavigator(nav)

    _, _, ok := loop.kiteLaneResolve(
        45000, 50000, -3500, 44300, 50000, -1, 0)
    require.False(t, ok,
        "a 38 unit partial run is no lane")

    nav.validateHook = func(
        from, _ pathfind.Vec3,
    ) (pathfind.Vec3, bool) {
        return pathfind.Vec3{
            X: from.X - 300, Y: from.Y, Z: from.Z}, true
    }
    x, y, ok := loop.kiteLaneResolve(
        45000, 50000, -3500, 44300, 50000, -1, 0)
    require.True(t, ok, "a 300 unit run is a lane")
    require.Equal(t, int32(44700), x,
        "the validated endpoint serves as the destination")
    require.Equal(t, int32(50000), y)
}

// TestKiteRetreatLaneEscapesTheWalledDirectRay pins the corner-only
// angular fallback (the live 2026-11-09 spot): the direct away-ray is
// walled at every rung, so the battery walks the nearest straight
// bearing the terrain offers - here the perpendicular - instead of
// standing the archer at melee for the whole fight. The escape stays
// inside the away half-plane and replaces the preferred ray only when
// every direct rung fails.
func TestKiteRetreatLaneEscapesTheWalledDirectRay(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    nav := &fakeNavigator{}
    nav.validateHook = func(
        from, to pathfind.Vec3,
    ) (pathfind.Vec3, bool) {
        // Only the due-south corridor is open: every west-bound and
        // diagonal lane is walled.
        if math.Abs(to.X-from.X) <= 1 && to.Y <= from.Y-100 {
            return to, true
        }

        return from, false
    }
    loop.SetNavigator(nav)

    x, y, ok := loop.kiteRetreatLane(
        45000, 50000, -3500, -1, 0, -1, 0, 600)
    require.True(t, ok,
        "the angular escape finds the open corridor")
    require.Equal(t, int32(45000), x,
        "the escape runs the perpendicular ray")
    require.InDelta(t, 49400.0, float64(y), 0.001,
        "the escape walks the full leg down the open corridor")
}
