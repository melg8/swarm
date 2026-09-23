// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The curving retreat of the archer kite (issue #70, the fifth
// behavior): the opening retreat of a fight runs the straight
// away-ray, every later one leans the fixed tangential bearing on
// the fight's turn side - the character circles the fight instead
// of marching one ray, so the drift away from the farm point stays
// bounded. The tests pin the opening straight step, the curved
// second step, the fresh-target reset and the zone-centered turn
// side.

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestKiteOpeningRetreatRunsStraight pins the opening semantics: the
// FIRST retreat of a fight is the panic retreat - the straight
// away-ray, the full kiteStep of distance at once, no curve.
func TestKiteOpeningRetreatRunsStraight(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, int32(44600), game.walks[0][0],
        "the opening retreat runs the straight away-ray")
    require.True(t, loop.kiteCurved,
        "the issued walk spends the opening straight step")
}

// TestKiteSecondRetreatCurvesTheCircle pins the circle itself: the
// SECOND retreat of the same fight leans the fixed tangential
// bearing on the fight's turn side - the away-ray rotated
// kiteCurveStep - and still opens the distance (the half-plane).
func TestKiteSecondRetreatCurvesTheCircle(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The next shot cycle of the SAME fight: the pacing windows
    // aged, a fresh broadcast arms the rhythm again.
    loop.combatAvoidUntil = time.Time{}
    loop.kiteAt = time.Now().Add(-kiteStepPeriod)
    selfSwingsAt(bot, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 2,
        "the second retreat of the fight must issue")
    curved := game.walks[1]
    wantX, wantY := rotatePlanar(-1, 0, kiteCurveStep)
    require.InDelta(t, float64(45000+int32(wantX*float64(kiteStep))),
        float64(curved[0]), 1.0,
        "the second retreat leans the tangential bearing")
    require.InDelta(t, float64(50000+int32(wantY*float64(kiteStep))),
        float64(curved[1]), 1.0,
        "the second retreat leans the tangential bearing")
    // The curve never folds back into the train: the endpoint still
    // opens the distance to the mob at 45200.
    toMob := float64(curved[0] - 45200)
    require.Less(t, toMob, -kiteStep*0.3,
        "the curved step still opens the distance")
}

// TestKiteFreshTargetResetsTheCircle pins the reset: a fresh fight
// starts its own circle - the opening straight retreat again.
func TestKiteFreshTargetResetsTheCircle(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The pick moved to a fresh mob: the server selection of the own
    // attack broadcast switches the fight. The first walk's window
    // aged out on the live timeline (the disable lapsed before the
    // next pick swung).
    spawnMobAt(bot, 8, 45400)
    loop.combatAvoidUntil = time.Time{}
    loop.kiteAt = time.Now().Add(-kiteStepPeriod)
    bot.ApplyAttack(state.Attack{
        AttackerID:  100,
        X:           45000,
        Y:           50000,
        Z:           -3500,
        TargetX:     45400,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{8},
        TargetCount: 1,
    })
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 2)
    fresh := game.walks[1]
    require.Equal(t, int32(44600), fresh[0],
        "the fresh fight opens with the straight away-ray")
}

// TestKiteCurveTurnsTowardTheZoneCenter pins the turn side pick: the
// circle bends back toward the hunting zone center when the leash
// knows one - the drift stays on the farm side.
func TestKiteCurveTurnsTowardTheZoneCenter(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The zone center sits SOUTH of the character: the turn side
    // that leans the retreat south is the one the circle takes (the
    // counterclockwise +70 degree ray of the west away-vector points
    // southwest, toward the center).
    loop.SetHuntingZone(45000, 48000, 2000)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    loop.combatAvoidUntil = time.Time{}
    loop.kiteAt = time.Now().Add(-kiteStepPeriod)
    selfSwingsAt(bot, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 2)
    curved := game.walks[1]
    require.Less(t, curved[1], int32(50000),
        "the circle bends the retreat toward the zone center")
    _, wantY := rotatePlanar(-1, 0, kiteCurveStep)
    require.InDelta(t, float64(50000+int32(wantY*float64(kiteStep))),
        float64(curved[1]), 1.0,
        "the south-leaning side carries the circle")
}

// TestKiteCurveStepStaysInTheAwayHalfPlane pins the constant
// contract of the circle bearing: the step stays under the
// half-plane bound (cos(step) >= the slack), so the bent ray never
// folds back into the train whatever the turn side. A step that
// violates the bound is a behavior change the curve cannot absorb.
func TestKiteCurveStepStaysInTheAwayHalfPlane(t *testing.T) {
    require.GreaterOrEqual(t, math.Cos(kiteCurveStep),
        kiteHalfPlaneSlack,
        "the circle bearing must keep the bent ray in the away "+
            "half-plane on both turn sides")
}
