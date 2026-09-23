// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The race legs of the archer kite (issue #70, the round-17
// redesign): the old fixed 400-unit retreat step never reached the
// re-shot floor - the walk window equaled the shot cycle, so the
// character stopped at the endpoint exactly when the server's re-shot
// was ready, and the armed stance fired it at the collapsed range
// (the "second shot of the cycle" the owner named). The redesign
// buys the whole deficit back in ONE leg (kiteStepLength), scales
// the movement window to the leg (kiteIssueWalk), keeps the re-click
// ladder alive for the WHOLE leg (kiteReclickWalk - a mid-leg silent
// drop re-clicks the endpoint from wherever the character stands),
// and gates the chase-stall watchdog on the window (loop.go - the
// mid-leg drop never walks the archer back INTO the mob). These pins
// are the round-17 contract record: the step formula, the
// equidistant chord of the circling fights, the window scaling, the
// ladder recovery and the arrival stand-down.

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// selfShotFrom models the character's own attack broadcast from a
// position other than the spawn cell: the moved scenes of the race
// legs (the mid-leg stand, the circling chord) commit their shot from
// where the character actually stands - the plain selfSwingsAt helper
// carries the fixed spawn coordinates and would drag the tracker's
// self position back to them.
func selfShotFrom(
    bot *state.Bot, selfX, selfY, mobX, mobY int32,
) {
    bot.ApplyAttack(state.Attack{
        AttackerID:  100,
        X:           selfX,
        Y:           selfY,
        Z:           -3500,
        TargetX:     mobX,
        TargetY:     mobY,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{stagedMobID},
        TargetCount: 1,
    })
}

// TestKiteRaceStepLengthBuysBackTheFloor pins the race formula
// itself (kiteStepLength): the plain kiteStep while the fight holds
// the re-shot floor (the distance is safe, the shot is affordable
// wherever it stands), the deficit bought back at the parity
// exchange rate under it, fenced at the absolute cap and at the
// fight's own leash budget (the roam radius minus the anchor
// distance and the cell-rounding guard, with the shove tier of
// room left at the fence). The
// behavioral opening pin - a mob at 200 answering with the 1300 leg
// (the fresh anchor's whole budget) - lives in
// TestKiteStepsAwayFromTheClosedTarget; the duel half here pins the
// floor boundary on the live fight: a live attacker 500 out (the
// ranged duel the pursue band keeps moving) answers with the plain
// 400 step, the deficit is gone past the floor.
func TestKiteRaceStepLengthBuysBackTheFloor(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.target = 7

    // Before the fight anchored (no retreat resolved yet): the raw
    // formula - the deficit 280 at the parity rate (400 + 280*8 =
    // 2640) fenced at the absolute cap.
    require.InDelta(t, kiteRaceStepMax,
        loop.kiteStepLength(200, 45000, 50000), 0.001,
        "the deficit leg caps at kiteRaceStepMax before the fight "+
            "anchored")
    // At or beyond the floor the plain step serves the fight.
    require.InDelta(t, kiteStep,
        loop.kiteStepLength(kiteReshotFloor, 45000, 50000), 0.001,
        "the floor itself answers with the plain step")
    require.InDelta(t, kiteStep,
        loop.kiteStepLength(500, 45000, 50000), 0.001,
        "beyond the floor the deficit is gone - the plain step")

    // The anchored fight (the first resolution anchors it AT the
    // self position): the leash budget caps the leg at the roam
    // radius minus the anchor distance and the rounding guard
    // (kiteArrivalEpsilon's half keeps the int32 cell rounding of
    // the endpoint from crossing the fence the battery enforces),
    // never under the shove tier.
    loop.kiteFightFor = 7
    loop.kiteFightX, loop.kiteFightY = 45000, 50000
    require.InDelta(t, kiteRoamRadius-kiteArrivalEpsilon/2,
        loop.kiteStepLength(200, 45000, 50000), 0.001,
        "the fresh anchor caps the race leg at its own leash budget")
    require.InDelta(t, kiteRoamRadius-1000-kiteArrivalEpsilon/2,
        loop.kiteStepLength(200, 44000, 50000), 0.001,
        "the mid-circle anchor leaves only the leash budget "+
            "(1350-1000-50 = 300)")
    require.InDelta(t, kiteShoveStep,
        loop.kiteStepLength(200, 45000-1330, 50000), 0.001,
        "a fight at the leash edge keeps the shove tier of room")

    // The duel half: a live attacker 500 out (past the floor) keeps
    // the rhythm moving and answers with the plain step.
    bot, game, duel := kiteBowBot(t, 45500)
    mobHitsCharacterAt(bot, 45500)
    tickPastTheWindup(duel)
    require.Len(t, game.walks, 1,
        "the ranged duel keeps the rhythm moving")
    require.Equal(t, [3]int32{44600, 50000, -3500}, game.walks[0],
        "a hostile past the floor answers with the plain 400 step")
}

// TestKiteRaceChordKeepsTheCircleRadius pins the equidistant
// property of the curving legs (kiteCurveDirection, reworked round
// 17): once the fight anchored and the character left the anchor
// cell, the retreat endpoint lands ON the anchor circle - exactly
// the circle's radius away from the anchor - swung on the fight's
// fixed turn side. The character keeps the SAME distance to the
// point it kites around: the owner's circling spec, exactly.
func TestKiteRaceChordKeepsTheCircleRadius(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the opening retreat anchors the fight and spends the "+
            "straight leg")

    // The character stands 600 west of the anchor, the mob chases
    // behind on the circling path (north): the second retreat
    // resolves on the anchor circle.
    selfX, selfY := kiteChaseBehindOnTheCircle(bot, loop)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 2,
        "the second retreat of the anchored fight must issue")
    chord := game.walks[1]

    // The leg at the stand: the deficit 280 bought back at the
    // parity rate, capped by the leash budget the anchor 600 away
    // leaves (1350-600-50 = 700).
    leg := math.Min(
        kiteStep+(kiteReshotFloor-200)*kiteRaceFactor,
        kiteRaceStepMax)
    leg = math.Min(leg, kiteRoamRadius-600-kiteArrivalEpsilon/2)
    wantX, wantY, radiusTarget := kiteWantChord(
        45000, 50000, selfX, selfY, leg)
    require.InDelta(t, wantX, float64(chord[0]), 1.0,
        "the endpoint is the chord point of the anchor circle")
    require.InDelta(t, wantY, float64(chord[1]), 1.0,
        "the endpoint is the chord point of the anchor circle")
    // The KEY pin: the endpoint keeps the circle's radius to the
    // anchor - here the CURRENT radius (the leg fits the ring
    // without growing it), so the character stays exactly as far
    // from the kite center as it stood.
    require.InDelta(t, radiusTarget,
        math.Hypot(float64(chord[0]-45000), float64(chord[1]-50000)),
        1.0,
        "the chord endpoint keeps the circle radius to the anchor")
    require.InDelta(t,
        math.Hypot(float64(selfX-45000), float64(selfY-50000)),
        math.Hypot(float64(chord[0]-45000), float64(chord[1]-50000)),
        1.0,
        "the character stays equidistant from the point it kites "+
            "around")
    // The curve side: the counterclockwise turn side of the west
    // radius swings the endpoint south of the stand.
    require.Less(t, chord[1], selfY,
        "the endpoint lands on the fight's turn side of the radius")
}

// TestKiteRaceWindowScalesWithTheLeg pins the window scaling of
// kiteIssueWalk: the race leg runs many times the ordinary step's
// walk time, so the movement window scales with the walked distance
// at the per-unit pace of the bow-aware window formula - leg/400 *
// the C1 disable window of the live pAtkSpd. The 1300 leg of the
// closed mob (the deficit cap of the fresh anchor) at the pAtkSpd
// 337 of the Short Bow kit holds the movement for ~9.64s from the
// issue - the caller's own disable-end window (2.97s) never fences
// the leg short.
func TestKiteRaceWindowScalesWithTheLeg(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The equipping broadcast of the bow: pAtkSpd 337 (the value
    // the acceptance dump carries for the Short Bow kit).
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrAtkSpd, Value: 337},
    })
    shotAt := bot.SelfLastShotAt()
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{43700, 50000, -3500}, game.walks[0],
        "the closed mob answers with the full-budget race leg")

    // The scaled window: 1300/400 * (500000 + reuse*333)/337 ms
    // ~= 9.64s from the issue. A variable (not a constant) keeps
    // the formula a runtime conversion - a constant one is not
    // representable as the integer Duration and refuses to compile.
    const pAtkSpd = 337.0
    cooldownMS := (500000.0 + kiteBowReuseDelay*333.0) / pAtkSpd
    leg := math.Hypot(
        float64(game.walks[0][0]-45000), float64(game.walks[0][1]-50000))
    want := time.Duration(
        cooldownMS * float64(time.Millisecond) * leg / kiteStep)
    aged := loop.kiteWalkUntil.Sub(loop.kiteWalkIssuedAt)
    require.InDelta(t, float64(want), float64(aged),
        float64(50*time.Millisecond),
        "the walk window scales with the leg at the C1 disable pace")
    // The scaled window outranks the shot's own disable end: the
    // leg, not the cooldown, bounds the movement.
    require.True(t, loop.kiteWalkUntil.After(
        shotAt.Add(loop.kiteWalkWindow())),
        "the scaled leg window outranks the shot disable end")
}

// TestKiteRaceLadderReClicksTheMidRouteDrop pins the mid-route drop
// recovery of the leg-long ladder: the walk adopts (the movement
// broadcast runs), the character moves 400 of the 1300 leg, and the
// walk DROPS - no more movement broadcasts, the character stands
// mid-route, 900 short of the endpoint. The ladder re-bases onto the
// drop cell and re-clicks the SAME endpoint: the recovery the
// window-sized ladder never had (a silent drop mid-leg used to stand
// the character through the rest of the window while the chaser
// collected the difference).
func TestKiteRaceLadderReClicksTheMidRouteDrop(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{43700, 50000, -3500}, game.walks[0],
        "the race leg clicked")

    // The walk adopts: the character runs the first 400 of the leg.
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 44600, DestY: 50000, DestZ: -3500,
    })
    loop.tick()
    require.Len(t, game.walks, 1,
        "the running walk sends no re-click")

    // The walk drops mid-route: the character stands at 44600, 900
    // short of the clicked endpoint. The ladder re-bases onto the
    // drop cell and re-clicks the SAME endpoint from there.
    moveSelfTo(bot, 44600, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the mid-route drop re-clicks the endpoint")
    require.Equal(t, [3]int32{43700, 50000, -3500}, game.walks[1],
        "the recovery aims the same endpoint from the new cell")
    require.Equal(t, int32(44600), loop.kiteWalkBaseX,
        "the ladder re-based onto the drop cell")
    require.Equal(t, int32(50000), loop.kiteWalkBaseY,
        "the ladder re-based onto the drop cell")
    require.Equal(t, 1, loop.kiteReclicks,
        "the fresh standing episode spent its first re-click")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the recovery keeps the ladder armed for the rest of the leg")
}

// TestKiteRaceLadderCompletesAtTheArrival pins the arrival boundary
// of the leg-long ladder: a stand WITHIN kiteArrivalEpsilon (100) of
// the endpoint completes the leg (the server's cell granularity
// never lands the character exactly on the clicked cell), while a
// stand beyond it - mid-route, or short of the endpoint after a
// partial path - keeps pushing the endpoint.
func TestKiteRaceLadderCompletesAtTheArrival(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{43700, 50000, -3500}, game.walks[0])

    // A stand 150 short of the endpoint: OUTSIDE the arrival
    // radius - the leg is not done, the ladder re-bases and
    // re-clicks the endpoint.
    moveSelfTo(bot, 43850, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "a stand beyond the arrival radius keeps re-clicking")
    require.Equal(t, [3]int32{43700, 50000, -3500}, game.walks[1],
        "the short stand keeps pushing the same endpoint")

    // A stand 80 short of the endpoint: INSIDE the arrival radius -
    // the leg completed, the ladder stands down, the window's
    // remainder belongs to the arrival shot's own cycle.
    moveSelfTo(bot, 43780, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the arrived leg sends no re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder cleared at the arrival")
}

// TestKiteRaceStallWatchdogRespectsTheWalkWindow pins the
// chase-stall gate of loop.go (the round-17 fix): during a kite walk
// window the stall watchdog NEVER walks toward the target - a
// mid-leg drop at a distance past the bow stall radius used to
// trigger the anti-kite walk INTO the mob on every tick the ladder
// did not spend, the exact melee the kite exists to avoid. The gate
// holds the watchdog through the whole scaled window; the ordinary
// stall answer returns only past it.
func TestKiteRaceStallWatchdogRespectsTheWalkWindow(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45300)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the opening retreat issued at the race length")

    // The walk drops mid-route: the character stands 700 west of
    // the mob (past the 650 bow stall radius), 900 short of the
    // clicked endpoint. The ladder spends its whole re-click budget
    // pushing the endpoint from the drop cell.
    moveSelfTo(bot, 44600, 50000, -3500)
    for range kiteReclickLimit + 1 {
        ageKiteReclick(loop)
        loop.tick()
    }
    require.Len(t, game.walks, 1+kiteReclickLimit,
        "the drop episode re-clicked the endpoint to the budget")
    for _, walk := range game.walks {
        require.Equal(t, int32(43700), walk[0],
            "every walk of the window runs away from the mob")
    }

    // The budget is spent, the window still runs: the tick the
    // ladder no longer owns would fall to the stall watchdog - the
    // gate holds it (no walk toward the mob), and the window holds
    // the attack re-request of the lapsed stance too.
    time.Sleep(3200 * time.Millisecond)
    loop.tick()
    require.Len(t, game.walks, 1+kiteReclickLimit,
        "during the kite window the stall watchdog never walks "+
            "toward the target")

    // Past the scaled window (1300/400 * the 2s fallback = 6.5s
    // from the issue) the ordinary answer returns: the ladder
    // stands down, and the stall watchdog walks the character back
    // toward the target - the gate opens exactly at the window end.
    // The fight view rides a fresh swing of the standing fight (the
    // fake mob never swings on its own).
    time.Sleep(4100 * time.Millisecond)
    selfShotFrom(bot, 44600, 50000, 45300, 50000)
    loop.tick()
    require.Len(t, game.walks, 2+kiteReclickLimit,
        "past the window the ordinary stall answer walks to the "+
            "target")
    stall := game.walks[1+kiteReclickLimit]
    require.Equal(t, int32(45300), stall[0],
        "the stall watchdog owns the movement only past the window")
    require.Equal(t, int32(50000), stall[1],
        "the stall walk runs to the target's own cell")
}
