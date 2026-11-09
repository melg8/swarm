// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The defensive flight of the archer kite (the run-3 finding of the
// 2026-11-09 round): the melee flee machinery owns the defensive aggro
// answer, its fan is clamped to the current hunting zone box, and the
// run-3 acceptance dump measured the closure watchdog logging the
// session out MID-KITE with the bow healthy and the quiver full. The
// bow answer is the straight away-ray of the train (kiteFleeThreat),
// walked at the full escape segment: the tests pin the straight
// geometry, the continuous legs, the ladder that keeps the leg alive,
// the bypassed fan budgets, the fan fallback of a walled lane, the
// bow-attacker and disabled-profile gates and the stand-up guard.

import (
    "bytes"
    "log"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// kiteFleeBot builds the defensive flee scene: a bow-wielding
// character with NO selected target (the run-3 loot interruption
// shape - the target register dropped before the flee) and a melee
// mob holding the character as its attack target.
func kiteFleeBot(t *testing.T) (*state.Bot, *fakeGame, *Loop) {
    t.Helper()
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, Name: "Kaboo Orc Fighter",
    })
    bot.ApplyAttack(state.Attack{
        AttackerID:  7,
        X:           45200,
        Y:           50000,
        Z:           -3500,
        TargetX:     45000,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    return bot, game, loop
}

// TestKiteFleeThreatRunsTheStraightFlight pins the hook: the
// targetless defensive flee of a bow character flies the straight
// away-ray of the attacker instead of the zone-clamped fan, issues
// the retreat click through the shared walk seam, arms the movement
// window and spends the tick - and the fan's episode budget never
// starts behind the flight.
func TestKiteFleeThreatRunsTheStraightFlight(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    var logBuf bytes.Buffer
    loop.SetLogger(log.New(&logBuf, "", 0))
    // A stale fan episode of the earlier loot defense: the flight's
    // success clears the budget so the fan starts fresh if it ever
    // takes over.
    loop.fleeSince = time.Now().Add(-time.Minute)

    loop.fleeFromThreat(time.Now())
    require.Len(t, game.walks, 1, "the flight issues one straight leg")
    require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[0],
        "the leg runs the full escape segment down the away-ray")
    require.Empty(t, game.forces,
        "the defensive flight never requests an attack")
    require.Zero(t, game.logouts, "the flight never logs out")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the leg arms its walk window")
    require.True(t, loop.combatAvoidUntil.Equal(loop.kiteWalkUntil),
        "the movement holds the forced attack through the window")
    require.True(t, loop.fleeSince.IsZero(),
        "the flight owns the flee, the fan budget never starts")
    require.True(t, loop.fleeGapAt.IsZero(),
        "the closure probe of the fan never anchors")
    require.Zero(t, loop.kiteHeldFor,
        "no walled-lane hold in the open scene")
    require.Contains(t, logBuf.String(), "kiting clear (step",
        "the flight narrates the retreat in the acceptance marker")
}

// TestKiteFleeThreatKeepsTheLegAlive pins the ladder inside the
// flight window: a character still standing on the issue cell
// re-clicks the same straight endpoint at the ladder pacing - the
// dead-click recovery the fight cycle gets, applied to the flight.
func TestKiteFleeThreatKeepsTheLegAlive(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    base := time.Now()
    loop.fleeFromThreat(base)
    require.Len(t, game.walks, 1)

    loop.fleeFromThreat(base.Add(time.Second))
    require.Len(t, game.walks, 2,
        "the standing character re-clicks the flight leg")
    require.Equal(t, game.walks[0], game.walks[1],
        "the re-click keeps the same straight endpoint")
}

// TestKiteFleeThreatOutlastsTheClosureWatchdogs pins the run-3
// survival fix: the parity chaser keeps pace through the whole fan
// budget (the 20 s flee logout and the 8 s closure probe), and the
// flight must keep opening straight legs instead of ending the
// session. The next leg issues past the lapsed window, the flee
// budget stays zero and no logout fires.
func TestKiteFleeThreatOutlastsTheClosureWatchdogs(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    base := time.Now()
    loop.fleeFromThreat(base)
    require.Len(t, game.walks, 1)

    loop.fleeFromThreat(base.Add(2 * fleeLogoutAfter))
    require.False(t, loop.logoutDone,
        "the flight never ends the session on the paced chase")
    require.Zero(t, game.logouts, "no logout request goes out")
    require.Len(t, game.walks, 2, "the flight opens the next leg")
    require.Equal(t, game.walks[0], game.walks[1],
        "the next leg keeps the straight ray")
    require.True(t, loop.fleeSince.IsZero(),
        "the fan budget stays zero behind the flight")
}

// TestKiteFleeThreatHoldsTheWalledLane pins the full-pocket answer
// of the 2026-11-10 run-24 dump: when every rung of the away-ray and
// of the back half-plane is walled, the flight holds the ground and
// KEEPS the ticks - the zone-clamped melee fan never runs behind it,
// because its no-lane guard logged the drifted archer out mid-flight
// at 99 percent health. The hold lapses and the flight re-probes.
func TestKiteFleeThreatHoldsTheWalledLane(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    loop.SetNavigator(&fakeNavigator{refuseClicks: true})
    base := time.Now()

    require.True(t, loop.kiteFleeThreat(base),
        "the walled flight keeps the tick")
    require.Empty(t, game.walks, "the walled flight issues no walk")
    require.Equal(t, int32(0), loop.kiteHeldFor,
        "the flight hold is a targetless hold")
    require.False(t, loop.kiteHeldAt.IsZero(),
        "the walled lane stamps the hold")
    require.True(t, loop.fleeSince.IsZero(),
        "the direct call does not start the fan episode")

    // The hold paces the re-probe and the fan never takes over.
    loop.fleeFromThreat(base.Add(time.Second))
    require.Empty(t, game.walks, "the fan never walks behind the hold")
    require.Zero(t, game.logouts, "the hold never logs out")
    require.True(t, loop.fleeSince.IsZero(),
        "the fan episode never starts behind the flight")

    // The hold lapses: the flight re-probes the straight ray and owns
    // the movement again once the terrain opens.
    loop.SetNavigator(&fakeNavigator{})
    require.True(t, loop.kiteFleeThreat(base.Add(4*time.Second)),
        "the flight re-probes after the hold period")
    require.Len(t, game.walks, 1, "the re-probe issues its own leg")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the open lane clears the flight hold")
}

// TestKiteFleeThreatBreaksOutOfThePocket pins the cornered flight of
// the run-24 dump: the away half-plane is a dead end while the
// ground behind the character (through the chaser's bearing) stands
// open. The flight breaks out through the back half-plane instead of
// holding the pocket until the fan logs the session out.
func TestKiteFleeThreatBreaksOutOfThePocket(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    loop.SetNavigator(&fakeNavigator{
        validateHook: func(_, to pathfind.Vec3) (pathfind.Vec3, bool) {
            if to.X <= 45000 {
                // The north-west pocket of the live run: every
                // westward line and the perpendicular rail are
                // walled.
                return pathfind.Vec3{}, false
            }

            return to, true
        },
    })

    require.True(t, loop.kiteFleeThreat(time.Now()),
        "the flight breaks out instead of holding")
    require.Len(t, game.walks, 1, "the breakout issues its leg")
    require.Greater(t, game.walks[0][0], int32(45000),
        "the leg runs back through the chaser bearing")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the breakout stamps no hold")
}

// TestKiteFleeThreatWaitsTheBowWindup pins the run-24 false stall:
// the fight was dropped 250 ms after a shot, the first flight click
// went out inside the bow windup, the server saved it and replayed
// it at the disable end 2.7 s later - and the 3 s no-progress window
// ended a healthy 99 percent flight. The flight waits the windup out
// and arms nothing on the deferred click.
func TestKiteFleeThreatWaitsTheBowWindup(t *testing.T) {
    bot, game, loop := kiteFleeBot(t)
    shotAt := time.Now().Add(-500 * time.Millisecond)
    bot.ApplyShotGaugeAt(2965*time.Millisecond, shotAt)
    require.False(t, loop.kiteFlightMovable(time.Now()),
        "the windup holds the movement")

    require.True(t, loop.kiteFleeThreat(time.Now()),
        "the waiting flight keeps the tick")
    require.Empty(t, game.walks, "no click goes out inside the windup")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the waiting flight arms no walk window")
    require.True(t, loop.escapeAnchorAt.IsZero(),
        "no stall anchor arms on the deferred click")

    // The windup lapses (the accepted side starts past half the
    // cycle plus the lead): the first leg goes out.
    moved := shotAt.Add(2965*time.Millisecond/2 + kiteWindupLead +
        100*time.Millisecond)
    require.True(t, loop.kiteFlightMovable(moved),
        "the accepted side frees the movement")
    require.True(t, loop.kiteFleeThreat(moved),
        "the flight issues the first leg")
    require.Len(t, game.walks, 1, "the leg goes out after the windup")
    require.False(t, loop.escapeAnchorAt.IsZero(),
        "the issued leg arms the stall anchor")
}

// TestKiteFleeThreatIgnoresAStaleGauge pins the other side of the
// windup gate: a gauge that outlived its fight owns no live cycle,
// so the flight is free to move at once.
func TestKiteFleeThreatIgnoresAStaleGauge(t *testing.T) {
    bot, game, loop := kiteFleeBot(t)
    bot.ApplyShotGaugeAt(2965*time.Millisecond,
        time.Now().Add(-kiteGaugeMaxAge-time.Second))
    require.True(t, loop.kiteFlightMovable(time.Now()))

    require.True(t, loop.kiteFleeThreat(time.Now()))
    require.Len(t, game.walks, 1, "the stale gauge holds nothing")
}

// TestKiteFleeThreatLeavesTheBowAttackerToTheHunt pins the enemy
// weapon gate on the defensive path (the owner rule of the round):
// a bow-wielding attacker cannot be outrun, so the flight never arms.
// The zone-clamped fan never answers it either - the hunt's standing
// trade owns the next tick, not the logout.
func TestKiteFleeThreatLeavesTheBowAttackerToTheHunt(t *testing.T) {
    bot, game, loop := kiteFleeBot(t)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, RightHandItem: 14,
        Name: "Kaboo Orc Archer",
    })
    require.True(t, bot.ObjectUsesBow(7), "the scene carries a bow")

    require.False(t, loop.kiteFleeThreat(time.Now()),
        "the flight never arms against an archer")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the bow gate leaves no flight hold behind")

    loop.fleeFromThreat(time.Now())
    require.Empty(t, game.walks,
        "the fan never answers the bow attacker")
    require.Zero(t, game.logouts, "the standing trade owns the tick")
    require.True(t, loop.fleeSince.IsZero(),
        "the fan episode never starts")
}

// TestKiteFleeThreatStaysOffWithoutTheKite pins the profile gate: a
// kite-disabled bot (the standing-archer baseline) keeps the plain
// melee flee answer, never the straight flight.
func TestKiteFleeThreatStaysOffWithoutTheKite(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    loop.SetKiteParams(KiteParams{Enabled: false})
    require.False(t, loop.kiteFleeThreat(time.Now()))

    loop.fleeFromThreat(time.Now())
    require.Len(t, game.walks, 1, "the fan answers the disabled kite")
    require.False(t, loop.fleeSince.IsZero(),
        "the fan episode starts behind the disabled flight")
}

// TestKiteFleeThreatStandsTheSittingCharacterUp pins the stand-up
// guard: the server refuses move requests while the character sits,
// so the flight spends the tick on the stand request and the first
// walk waits for the movable character.
func TestKiteFleeThreatStandsTheSittingCharacterUp(t *testing.T) {
    _, game, loop := kiteFleeBot(t)
    loop.tracker.ApplyWaitType(state.WaitType{
        ObjectID: 100, Sitting: true})

    require.True(t, loop.kiteFleeThreat(time.Now()),
        "the stand request consumes the tick")
    require.Equal(t, 1, game.sits)
    require.Empty(t, game.walks,
        "the first walk waits for the standing character")
}
