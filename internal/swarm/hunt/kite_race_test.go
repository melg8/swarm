// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The race legs of the archer kite (issue #70, the round-17
// redesign; the straight-lane rework of the 2026-11-09 owner round):
// the old fixed 400-unit retreat step never reached the re-shot
// floor - the walk window equaled the shot cycle, so the character
// stopped at the endpoint exactly when the server's re-shot was
// ready, and the armed stance fired it at the collapsed range (the
// "second shot of the cycle" the owner named). The design buys the
// whole deficit back in ONE straight leg (kiteStepLength), scales
// the movement window to the leg (kiteIssueWalk), keeps the
// re-click ladder alive for the WHOLE leg (kiteReclickWalk - a
// mid-leg silent drop re-clicks the endpoint from wherever the
// character stands), and gates the chase-stall watchdog on the
// window (loop.go - the mid-leg drop never walks the archer back
// INTO the mob). These pins are the race contract record: the step
// formula, the ring budget, the window scaling, the ladder recovery
// and the arrival stand-down.

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
// exchange rate under it, fenced at the absolute cap. The formula
// carries NO leash (round 19): the straight legs budget against the
// opening ring (kiteStraightLeg - the share of the roam radius
// around the mob's stand, landing the opening endpoint on the ring
// the chords circle) and the chords land equidistant by
// construction. The behavioral opening pin - a mob at 200 answering
// with the ring-budget leg - lives in
// TestKiteStepsAwayFromTheClosedTarget; the duel half here pins the
// floor boundary on the live fight: a live attacker 500 out (the
// ranged duel the pursue band keeps moving) answers with the plain
// 400 step, the deficit is gone past the floor.
func TestKiteRaceStepLengthBuysBackTheFloor(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.target = 7

    // The raw formula - the deficit 280 at the parity rate (400 +
    // 280*8 = 2640) fenced at the absolute cap.
    require.InDelta(t, kiteRaceStepMax,
        loop.kiteStepLength(200), 0.001,
        "the deficit leg caps at kiteRaceStepMax")
    // At or beyond the floor the plain step serves the fight.
    require.InDelta(t, kiteStep,
        loop.kiteStepLength(kiteReshotFloor), 0.001,
        "the floor itself answers with the plain step")
    require.InDelta(t, kiteStep,
        loop.kiteStepLength(500), 0.001,
        "beyond the floor the deficit is gone - the plain step")

    // The straight-leg budget (kiteStraightLeg): the endpoint must
    // stay on the opening ring - the leash share of the roam radius
    // around the fight anchor (the mob's stand) - minus the rounding
    // guard (kiteArrivalEpsilon's half keeps the int32 cell rounding
    // of the endpoint from crossing the ring the battery enforces),
    // never under the short hop of kiteMinStep. Before the fight
    // anchored the step rides unchanged.
    require.InDelta(t, kiteRaceStepMax,
        loop.kiteStraightLeg(kiteRaceStepMax, 45000, 50000), 0.001,
        "the unanchored fight keeps the raw race leg")
    loop.kiteFightFor = 7
    loop.kiteFightX, loop.kiteFightY = 45200, 50000
    ring := kiteRoamRadius * kiteRingLeashShare
    require.InDelta(t, ring-200-kiteArrivalEpsilon/2,
        loop.kiteStraightLeg(kiteRaceStepMax, 45000, 50000), 0.001,
        "the fresh anchor caps the opening leg at the ring budget")
    require.InDelta(t, ring-600-kiteArrivalEpsilon/2,
        loop.kiteStraightLeg(kiteRaceStepMax, 44600, 50000), 0.001,
        "a mid-ring stand keeps the remaining ring budget")
    require.InDelta(t, kiteStep,
        loop.kiteStraightLeg(kiteStep, 45200, 50000), 0.001,
        "a step that fits the ring budget rides unchanged")
    require.InDelta(t, kiteMinStep,
        loop.kiteStraightLeg(kiteRaceStepMax, 44200, 50000), 0.001,
        "a stand at the ring keeps the short hop of room")

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

// TestKiteRaceWindowScalesWithTheLeg pins the window scaling of
// kiteIssueWalk: the race leg runs many times the ordinary step's
// walk time, so the movement window scales with the walked distance
// at the per-unit pace of the bow-aware window formula - leg/400 *
// the C1 disable window of the live pAtkSpd. The 722 leg of the
// closed mob (the opening ring budget around its stand) at the
// pAtkSpd 337 of the Short Bow kit holds the movement for ~5.36s from
// the issue - the caller's own disable-end window (2.97s) never
// fences the leg short.
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
    require.Equal(t, [3]int32{44278, 50000, -3500}, game.walks[0],
        "the closed mob answers with the ring-budget race leg")

    // The scaled window: 722/400 * (500000 + reuse*333)/337 ms
    // ~= 5.36s from the issue. A variable (not a constant) keeps
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
// broadcast runs), the character moves 400 of the 722 opening leg,
// and the walk DROPS - no more movement broadcasts, the character
// stands mid-route, 322 short of the endpoint. The ladder re-bases
// onto the drop cell and re-clicks the SAME endpoint: the recovery the
// window-sized ladder never had (a silent drop mid-leg used to stand
// the character through the rest of the window while the chaser
// collected the difference).
func TestKiteRaceLadderReClicksTheMidRouteDrop(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{44278, 50000, -3500}, game.walks[0],
        "the race leg clicked")

    // The walk adopts: the character runs the first 400 of the leg.
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 44600, DestY: 50000, DestZ: -3500,
    })
    loop.tick()
    require.Len(t, game.walks, 1,
        "the running walk sends no re-click")

    // The walk drops mid-route: the character stands at 44600, 322
    // short of the clicked endpoint. The ladder re-bases onto the
    // drop cell and re-clicks the SAME endpoint from there.
    moveSelfTo(bot, 44600, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the mid-route drop re-clicks the endpoint")
    require.Equal(t, [3]int32{44278, 50000, -3500}, game.walks[1],
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
    require.Equal(t, [3]int32{44278, 50000, -3500}, game.walks[0])

    // A stand 150 short of the endpoint: OUTSIDE the arrival
    // radius - the leg is not done, the ladder re-bases and
    // re-clicks the endpoint.
    moveSelfTo(bot, 44428, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "a stand beyond the arrival radius keeps re-clicking")
    require.Equal(t, [3]int32{44278, 50000, -3500}, game.walks[1],
        "the short stand keeps pushing the same endpoint")

    // A stand 80 short of the endpoint: INSIDE the arrival radius -
    // the leg completed, the ladder stands down, the window's
    // remainder belongs to the arrival shot's own cycle. The mob
    // chases to 500 units: inside the bow stall radius (the chase
    // watchdog owns only the stretches beyond it) and beyond the
    // re-shot floor (the arrival chain preserves the stand of a
    // safe shot).
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 44858, Y: 50000, Name: "Keltir",
    })
    moveSelfTo(bot, 44358, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the arrived leg sends no re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder cleared at the arrival")
}

// TestKiteOpeningAnchorsOnTheMobStand pins the round-19 anchor: the
// first retreat resolution takes the FIGHT TARGET's stand as the
// circle center (the ground the fight really runs on), not the
// character's own position - and the opening straight leg lands on
// the opening ring around that stand.
func TestKiteOpeningAnchorsOnTheMobStand(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    require.Equal(t, int32(45200), loop.kiteFightX,
        "the fight anchors on the mob's stand")
    require.Equal(t, int32(50000), loop.kiteFightY,
        "the fight anchors on the mob's stand")
    radius := math.Hypot(
        float64(game.walks[0][0]-loop.kiteFightX),
        float64(game.walks[0][1]-loop.kiteFightY))
    require.InDelta(t,
        kiteRoamRadius*kiteRingLeashShare-kiteArrivalEpsilon/2,
        radius, 1.0,
        "the opening leg lands on the ring the chords circle")
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
    // the mob (past the 650 bow stall radius), 222 short of the
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
        require.Equal(t, int32(44378), walk[0],
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

    // Past the scaled window (622/400 * the 2s fallback = 3.11s
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
