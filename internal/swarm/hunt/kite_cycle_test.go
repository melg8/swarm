// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The shot-cycle contract of the archer kite (the owner rule of the
// 2026-11-09 round): one shot per bow cooldown, the retreat between
// the shots, the forced attack at the disable end whatever range the
// chase reached. The 5-minute acceptance run of the round measured
// what the old safe-distance gate cost: 20-30 s between the shots
// against an equal-speed chaser, the mob regenerating faster than the
// arrows, the fight never resolving and the drift running the session
// into a level-15 zone. The tests pin the window-end re-shot, the
// arrival that never opens the attack gate early, the conservative
// closed distance the threat scan reads and the geometry helpers of
// the straight lane.

import (
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
// The shot and the fight view are left to the caller (the re-shot
// scenes backdate them so the forced attack owns the tick the walk
// ends).
func lapseKiteWalk(loop *Loop) {
    until := time.Now().Add(-time.Millisecond)
    // The live walk arms the movement hold with exactly its own
    // window end: the two lapse together, so the aged window must
    // carry the hold with it.
    if loop.combatAvoidUntil.Equal(loop.kiteWalkUntil) {
        loop.combatAvoidUntil = until
    }
    loop.kiteWalkUntil = until
    loop.lastActionAt = time.Now().Add(-2 * engageRetryPeriod)
}

// TestKiteWindowEndFiresTheShotAtMeleeRange pins the core contract of
// the round: the walk window lapses with the chaser standing in melee
// (the exact live failure - the mob caught the character between the
// shots), and the tick fires the forced attack instead of issuing
// another retreat leg. That request IS the next shot - the bow
// commits it and its own broadcast arms the next retreat.
func TestKiteWindowEndFiresTheShotAtMeleeRange(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1, "the shot cycle opened the retreat")

    // The chaser closed to melee while the character walked the
    // reload, and the shot broadcast is long past: the forced attack
    // owns the moment the window ends.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45060, Y: 50000, Name: "Keltir",
    })
    bot.ApplyAttackAt(shotOf(45200), time.Now().Add(-time.Minute))
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the window end issues no continuation retreat")
    require.Equal(t, []int32{7}, game.forces,
        "the forced attack fires the next shot at melee range")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the window end clears the ladder")
}

// TestKiteWindowEndFiresTheShotWhateverTheRange pins the same
// contract on the far side: the chaser may stand anywhere inside the
// pursue band when the disable end lapses - the re-shot never waits
// for a regained distance (the safe-distance gate was the treadmill
// of the failing acceptance run).
func TestKiteWindowEndFiresTheShotWhateverTheRange(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The chaser fell back to 400 units: still the fight, still the
    // shot cycle.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45400, Y: 50000, Name: "Keltir",
    })
    bot.ApplyAttackAt(shotOf(45400), time.Now().Add(-time.Minute))
    lapseKiteWalk(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a window end never issues another leg by itself")
    require.Equal(t, []int32{7}, game.forces,
        "the shot fires once the bow is ready, at any range")
}

// TestKiteArrivalKeepsTheHoldUntilTheDisableEnd pins the early
// arrival rule: the ladder stops re-clicking once the character
// stands on the endpoint, but the movement hold lives to the bow
// disable end - the forced attack must never fire at the collapsed
// range of the arrival cell, because that shot would commit its
// windup against the chaser standing on top of the character.
func TestKiteArrivalKeepsTheHoldUntilTheDisableEnd(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The character arrived 50 units short of the clicked endpoint
    // (44600): inside the arrival radius. The shot and the fight
    // view go stale - the forced attack would own the tick if the
    // walk window did not. (The backdated broadcast carries the
    // spawn cell, so the arrival snap comes after it.)
    bot.ApplyAttackAt(shotOf(45200), time.Now().Add(-time.Minute))
    moveSelfTo(bot, 44650, 50000, -3500)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, Name: "Keltir",
    })
    loop.lastActionAt = time.Now().Add(-2 * engageRetryPeriod)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the arrived leg sends no re-click")
    require.Empty(t, game.forces,
        "the forced attack waits the bow disable end")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the movement hold survives the early arrival")

    // The disable end: the ladder stands down and the forced attack
    // fires the next shot.
    lapseKiteWalk(loop)
    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the disable end fires the next shot")
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
// 512 units out, but the segment aged while the mob kept running, so
// the live gap sits well under the raw reading. The 2026-11-09
// acceptance run measured exactly this lag (512 raw against 256
// actual).
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
    require.Less(t, dist, 512.0,
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
