// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The curving retreat of the archer kite (issue #70, the fifth
// behavior, reworked round 17): the opening retreat of a fight runs
// the straight away-ray, every later one aims the CHORD of the fight's
// anchor circle - the endpoint lands exactly the circle's radius away
// from the fight anchor (the live-observed ground the fight started
// on), so the character keeps the SAME distance to the point it kites
// around: the owner's equidistant circling spec. The circle owns the
// fight only once the opening legs carried it onto its own ring (the
// radius must clear kiteCircleMinRadius - a character still standing
// on the anchor cell keeps the straight race leg), so the chord
// scenes below MOVE the character away from the anchor between the
// retreats. The tests pin the opening straight step, the chord
// geometry of the second step, the fresh-target reset and the
// zone-side turn of the circle.

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// kiteChaseBehindOnTheCircle stages the second-retreat scene of the
// circling fights: the character ran 600 of the opening leg west and
// stands there (a mid-leg stand - the fake server never moves it on
// its own), and the mob chases BEHIND THE CIRCLING PATH, 200 north of
// the character: the fight's fixed counterclockwise turn side runs the
// circle southward from the west radius, so the chaser that follows
// the path closes from the north and the away-ray (south) agrees with
// the chord - the lane battery takes the chord preference as-is, the
// equidistant endpoint walks untouched.
func kiteChaseBehindOnTheCircle(
    bot *state.Bot, loop *Loop,
) (selfX, selfY int32) {
    // The mid-leg stand: 600 west of the anchor (45000, 50000).
    moveSelfTo(bot, 44400, 50000, -3500)
    // The chaser behind on the circle: 200 north of the stand.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 44400, Y: 50200, Name: "Keltir",
    })
    loop.combatAvoidUntil = time.Time{}
    loop.kiteAt = time.Now().Add(-kiteStepPeriod)
    selfShotFrom(bot, 44400, 50000, 44400, 50200)

    return 44400, 50000
}

// kiteWantChord resolves the chord geometry the second retreat must
// land on, from first principles (the test twin of kiteCurveDirection
// at the race leg length): the target radius (the current radius
// grown to the length the leg needs, fenced by the leash share), the
// half-arc (asin(leg/2R), capped at the 45 degree arc bound), and the
// equidistant endpoint - the radius vector swung the full arc on the
// fight's turn side (counterclockwise here: the pick ran on the
// anchor cell itself), rescaled to the target radius.
func kiteWantChord(
    anchorX, anchorY, selfX, selfY int32, leg float64,
) (wantX, wantY, radiusTarget float64) {
    radius := math.Hypot(float64(selfX-anchorX), float64(selfY-anchorY))
    radiusTarget = math.Max(radius, leg*math.Sin(kiteCircleArcHalf))
    if leashMax := kiteRoamRadius * kiteCircleLeashShare; radiusTarget > leashMax {
        radiusTarget = leashMax
    }
    half := math.Asin(math.Min(1, leg/(2*radiusTarget)))
    if half > kiteCircleArcHalf {
        half = kiteCircleArcHalf
    }
    unitX := float64(selfX-anchorX) / radius
    unitY := float64(selfY-anchorY) / radius
    swingX, swingY := rotatePlanar(unitX, unitY, 2*half)
    wantX = float64(anchorX) + swingX*radiusTarget
    wantY = float64(anchorY) + swingY*radiusTarget

    return wantX, wantY, radiusTarget
}

// TestKiteOpeningRetreatRunsStraight pins the opening semantics: the
// FIRST retreat of a fight is the panic retreat - the straight
// away-ray at the RACE length (the deficit leg capped at the fresh
// anchor's leash budget), no curve.
func TestKiteOpeningRetreatRunsStraight(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, int32(43650), game.walks[0][0],
        "the opening retreat runs the straight away-ray at the race length")
    require.True(t, loop.kiteCurved,
        "the issued walk spends the opening straight step")
}

// TestKiteSecondRetreatCurvesTheCircle pins the circle itself: the
// SECOND retreat of the same fight aims the CHORD of the anchor
// circle once the character left the anchor cell - the endpoint sits
// exactly the circle's radius away from the anchor (the equidistant
// property of the owner's circling spec) instead of marching the
// straight away-ray, on the turn side the fight keeps for its whole
// life.
func TestKiteSecondRetreatCurvesTheCircle(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{43650, 50000, -3500}, game.walks[0],
        "the opening retreat runs the straight race leg")

    // The character stands 600 west of the anchor (the mid-leg
    // stand), the mob chases behind on the circling path: the second
    // retreat resolves on the anchor circle now.
    selfX, selfY := kiteChaseBehindOnTheCircle(bot, loop)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 2,
        "the second retreat of the fight must issue")
    curved := game.walks[1]
    // The leg at the stand: the deficit 280 (the mob at 200) bought
    // back at the parity rate, capped by the leash budget the anchor
    // 600 away leaves (1350-600 = 750).
    leg := math.Min(
        kiteStep+(kiteReshotFloor-200)*kiteRaceFactor,
        kiteRaceStepMax)
    leg = math.Min(leg, kiteRoamRadius-600)
    wantX, wantY, radiusTarget := kiteWantChord(
        45000, 50000, selfX, selfY, leg)
    require.InDelta(t, wantX, float64(curved[0]), 1.0,
        "the second retreat lands on the anchor-circle chord")
    require.InDelta(t, wantY, float64(curved[1]), 1.0,
        "the second retreat lands on the anchor-circle chord")
    // The KEY pin: the endpoint keeps the circle's distance to the
    // anchor - the character stays equidistant from the point it
    // kites around.
    require.InDelta(t, radiusTarget,
        math.Hypot(float64(curved[0]-45000), float64(curved[1]-50000)),
        1.0,
        "the chord endpoint keeps the circle radius to the anchor")
    // The chord never folds back into the train: the endpoint still
    // opens the distance to the chaser at (44400, 50200).
    toMob := math.Hypot(
        float64(curved[0]-44400), float64(curved[1]-50200))
    require.Greater(t, toMob, 200.0,
        "the chord step still opens the distance")
}

// TestKiteFreshTargetResetsTheCircle pins the reset: a fresh fight
// starts its own circle - its own anchor (the self position at its
// first retreat resolution) and the opening straight retreat again.
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
    // The fresh fight re-anchors AT the self position and opens with
    // the straight away-ray at its own race length: the mob 400 out
    // owes a deficit of 80, so the leg runs 400 + 80*8 = 1040 - well
    // under the fresh anchor's 1350 leash budget.
    require.Equal(t, int32(43960), fresh[0],
        "the fresh fight opens with the straight away-ray")
}

// TestKiteCurveTurnsTowardTheZoneCenter pins the turn side pick: the
// circle bends back toward the hunting zone center when the leash
// knows one - the drift stays on the farm side. The fight keeps its
// turn side for its whole life (kiteCurveSide picks it once, toward
// the fight anchor - the counterclockwise side in this scene), and
// the chord endpoint lands on that side of the radius: south of the
// west stand, toward the zone center the scene parks south of the
// fight ground.
func TestKiteCurveTurnsTowardTheZoneCenter(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The zone center sits SOUTH of the character: the turn side
    // that leans the retreat south is the one the circle takes (the
    // counterclockwise chord of the west radius swings south, toward
    // the center).
    loop.SetHuntingZone(45000, 48000, 2000)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    selfX, selfY := kiteChaseBehindOnTheCircle(bot, loop)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 2)
    curved := game.walks[1]
    require.Less(t, curved[1], int32(50000),
        "the circle bends the retreat toward the zone center")
    // The endpoint is the chord, exactly: the equidistant geometry
    // on the south-leaning (counterclockwise) side of the radius.
    leg := math.Min(
        kiteStep+(kiteReshotFloor-200)*kiteRaceFactor,
        kiteRaceStepMax)
    leg = math.Min(leg, kiteRoamRadius-600)
    _, wantY, radiusTarget := kiteWantChord(
        45000, 50000, selfX, selfY, leg)
    require.InDelta(t, wantY, float64(curved[1]), 1.0,
        "the south-leaning side carries the circle")
    require.InDelta(t, radiusTarget,
        math.Hypot(float64(curved[0]-45000), float64(curved[1]-50000)),
        1.0,
        "the chord endpoint keeps the circle radius to the anchor")
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
