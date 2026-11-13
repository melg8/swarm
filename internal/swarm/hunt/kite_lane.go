// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The train geometry and the straight retreat lane of the kite fight: the
// chaser bearings, the centroid and gap directions, the terrain contract (the
// fight-anchor leash, the water, the server click validation) and the away
// half-plane guard. The retreat stays STRAIGHT - the line directly away from
// the train, no fan, no camp deflection, no circling chord - and the lane
// battery tries the same ray at the full leg and the progressively shorter hops
// before it holds the ground and shoots. The triggers, the shot rhythm and the
// re-click ladder stay in kite.go (docs/hunting.md).

import (
    "math"
    "sort"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// kiteLaneRungCount bounds the straight-hop ladder of one retreat resolution:
// the full leg, the three quartered shortenings and the kiteMinStep floor.
const kiteLaneRungCount = 5

func (l *Loop) kiteChaserBearings(
    selfX, selfY, selfZ int32, out []float64,
) []float64 {
    // The fight target weighs first: its away vector always counts.
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        dx := float64(selfX) - float64(tx)
        dy := float64(selfY) - float64(ty)
        if math.Hypot(dx, dy) >= 1 {
            out = append(out, math.Atan2(
                float64(ty-selfY), float64(tx-selfX)))
        }
    }
    for _, chaser := range l.tracker.SelfAttackers() {
        if chaser.ObjectID == l.target {
            // The target already weighed in through its own vector.
            continue
        }
        if chaser.Z != 0 &&
            math.Abs(float64(chaser.Z-selfZ)) > deckReachableZ {
            // Another deck: its swings cannot land, its lane crosses the wall.
            continue
        }
        dx := float64(selfX) - float64(chaser.X)
        dy := float64(selfY) - float64(chaser.Y)
        dist := math.Hypot(dx, dy)
        if dist >= kiteTrainScanRange || dist < 1 {
            // Too far to reach the character inside a step window (the flee
            // machinery owns a chase that distant) or standing on it.
            continue
        }
        out = append(out, math.Atan2(
            float64(chaser.Y-selfY), float64(chaser.X-selfX)))
        if len(out) == cap(out) {
            // The scratch is full: the gap geometry of a wider train reads its
            // nearest members alone (kiteMaxTrainMembers).
            break
        }
    }

    return out
}

// kiteContactAway resolves the flight ray of a melee attacker standing ON the
// character: at zero distance the train geometry has no away bearing, and the
// live dump stood a healthy level 7 archer at (35516,51008) for 21 seconds
// while a same-cell Kaboo Orc Fighter landed 15 damage every 1.9 s. Contact is
// the one scene the ground hold never owns - every swing lands from there - so
// the flight takes the first OPEN ring ray (the own facing, the two
// perpendiculars, the back ray), each a straight full escape segment, never a
// turn (docs/hunting.md).
func (l *Loop) kiteContactAway(
    selfX, selfY, selfZ int32,
) (float64, float64, bool) {
    heading := float64(l.tracker.SelfHeading()) * 2 * math.Pi / 65536
    dirX, dirY := math.Cos(heading), math.Sin(heading)
    for _, angle := range [...]float64{
        0, math.Pi / 2, -math.Pi / 2, math.Pi,
    } {
        rayX, rayY := rotatePlanar(dirX, dirY, angle)
        if _, _, found := l.kiteRetreatLane(
            selfX, selfY, selfZ, rayX, rayY, rayX, rayY,
            escapeWalkDistance,
        ); found {
            return rayX, rayY, true
        }
    }

    return 0, 0, false
}

// kiteGapDirection resolves the escape ray of an encircled train: the bisector
// of the widest angular gap between the chaser bearings, the direction farthest
// from every flanker at once. The archer that keeps stepping through the widest
// gap breaks a pile-up before it forms. The perpendicular break of a two-mob
// line is the classic case - the bisector of the two 180 degree gaps opens the
// distance to both chasers. A single bearing or none pins no escape geometry
// (docs/hunting.md).

func kiteGapDirection(bearings []float64) (float64, float64, bool) {
    if len(bearings) < 2 {
        return 0, 0, false
    }
    sorted := make([]float64, len(bearings))
    copy(sorted, bearings)
    sort.Float64s(sorted)
    // The widest gap between consecutive bearings, wraparound included; the
    // first widest gap wins the ties, so a symmetric train stays deterministic.
    bestSpan := 0.0
    bestAt := sorted[0]
    for i := 1; i < len(sorted); i++ {
        if span := sorted[i] - sorted[i-1]; span > bestSpan {
            bestSpan, bestAt = span, sorted[i-1]
        }
    }
    if span := sorted[0] + 2*math.Pi - sorted[len(sorted)-1]; span > bestSpan {
        bestSpan, bestAt = span, sorted[len(sorted)-1]
    }
    bisector := bestAt + bestSpan/2

    return math.Cos(bisector), math.Sin(bisector), true
}

// kiteTrainDirection resolves the retreat direction of one kite step: the
// centroid away-vector of the chaser train - the fight target plus every
// SelfAttackers member in scan range on a reachable deck, each weighing one
// unit. A train whose away vectors cancel under the encirclement share names
// no centroid, and the widest-gap bisector takes the step; the hold ground
// answer survives the walled straight gap. The raw last-known packet
// positions serve this (the projected scan serves only the nearest chaser)
// and the trigger margin absorbs the broadcast lag (docs/hunting.md).

func (l *Loop) kiteTrainDirection(
    selfX, selfY, selfZ int32,
) (dirX, dirY float64, encircled bool) {
    bearings := l.kiteChaserBearings(
        selfX, selfY, selfZ, l.kiteBearings[:0])
    var sumX, sumY float64
    for _, bearing := range bearings {
        sumX += -math.Cos(bearing)
        sumY += -math.Sin(bearing)
    }
    count := float64(len(bearings))
    sumLen := math.Hypot(sumX, sumY)
    if len(bearings) == 0 {
        // The degenerate no-chaser scene: no ray to guard, the hold keeps the
        // pacing. The empty list is tested explicitly because a single bearing
        // sums to a unit vector whose hypot may land one ulp under 1.
        return 0, 0, true
    }
    if sumLen < kiteEncircleShare*count {
        // The encircled sum: the widest-gap bisector serves the step.
        if gapX, gapY, gap := kiteGapDirection(bearings); gap {
            return gapX, gapY, true
        }

        return 0, 0, true
    }

    return sumX / sumLen, sumY / sumLen, false
}

// kiteLaneRungs resolves the straight-hop ladder of one retreat resolution,
// longest first: the caller's leg, the three quartered shortenings and the
// kiteMinStep floor. The battery takes the first rung the server accepts, so a
// ray whose far end is walled still walks the reachable prefix. The rungs never
// exceed the caller's leg and never fall under kiteMinStep, and the fixed array
// keeps the resolution allocation free (docs/hunting.md).

func kiteLaneRungs(step float64, out [kiteLaneRungCount]float64) (
    [kiteLaneRungCount]float64, int,
) {
    out[0] = step
    count := 1
    for _, share := range [kiteLaneRungCount - 2]float64{0.75, 0.5, 0.25} {
        if length := step * share; length >= kiteMinStep &&
            length < out[count-1] {
            out[count] = length
            count++
        }
    }
    if kiteMinStep < out[count-1] {
        out[count] = kiteMinStep
        count++
    }

    return out, count
}

// kiteLaneEscapeAngles is the blocked-lane escape order: the preferred away-ray
// first, then the smallest turns inside the away half-plane, both ways (22.5,
// 45, 67.5 and 90 degrees). The corner-only fallback was measured at (34792,
// 49416): the direct west ray was walled at every length while the south, east
// and north rays stood open at 250-900. The escape never folds back toward the
// train, never fans wide of the perpendicular and never replaces the preferred
// ray (docs/hunting.md).
var kiteLaneEscapeAngles = [9]float64{
    0,
    math.Pi / 8,  // 22.5 degrees clockwise from the away-ray
    -math.Pi / 8, // 22.5 degrees counterclockwise
    math.Pi / 4,  // 45
    -math.Pi / 4,
    3 * math.Pi / 8, // 67.5
    -3 * math.Pi / 8,
    math.Pi / 2, // the perpendicular
    -math.Pi / 2,
}

// kiteRetreatLane resolves the retreat lane of a kite step: the preferred
// straight ray (the centroid away-vector, the widest-gap bisector of an
// encircled train) at the caller's step length. A rung of the preferred ray
// that walks is the lane, and the ray is never bent by a fan or a chord. Only
// when every rung is blocked does the battery walk the escape angles, and a
// scene every angle refuses stays the cornered hold. Each rung runs the terrain
// contract, the half-plane guard and the server click validation.

func (l *Loop) kiteRetreatLane(
    selfX, selfY, selfZ int32, prefX, prefY, awayX, awayY float64,
    step float64,
) (int32, int32, bool) {
    var scratch [kiteLaneRungCount]float64
    rungs, count := kiteLaneRungs(step, scratch)
    for _, angle := range kiteLaneEscapeAngles {
        rayX, rayY := prefX, prefY
        if angle != 0 {
            rayX, rayY = rotatePlanar(prefX, prefY, angle)
        }
        for i := range count {
            endX := selfX + int32(math.Round(rayX*rungs[i]))
            endY := selfY + int32(math.Round(rayY*rungs[i]))
            laneX, laneY, ok := l.kiteLaneResolve(
                selfX, selfY, selfZ, endX, endY, awayX, awayY)
            if ok {
                return laneX, laneY, true
            }
        }
    }

    return 0, 0, false
}

// kiteTerrainLane resolves the terrain contract of the retreat endpoint: the
// fight-anchor leash, the degenerate length and the water. The wall gate is the
// server click validation of kiteLaneResolve; without a navigator the leash
// alone fences the step.

func (l *Loop) kiteTerrainLane(
    selfX, selfY, selfZ, endX, endY int32,
) (int32, int32, bool) {
    if l.target != 0 && l.kiteFightFor == l.target {
        // The endpoint stays inside the fight's roam radius around the fight
        // anchor - the target's stand, re-read on every resolution - so a fight
        // keeps its retreats on its own ground (docs/hunting.md).
        if math.Hypot(float64(endX-l.kiteFightX),
            float64(endY-l.kiteFightY)) > kiteRoamRadius {
            return 0, 0, false
        }
    }
    laneX, laneY := float64(endX-selfX), float64(endY-selfY)
    if math.Hypot(laneX, laneY) < 1 {
        return 0, 0, false
    }
    if l.navigator != nil && l.navigator.OverWater(
        float64(endX), float64(endY), int16(selfZ)) {
        // Water behind: swimming trades the bow for the paddle.
        return 0, 0, false
    }

    return endX, endY, true
}

// kiteLaneResolve resolves the retreat lane to its final endpoint and reports
// whether the lane carries a step: the terrain contract, the away half-plane
// guard and the server click validation. The half-plane gate guards the
// rounding and the raw candidate rays: a lane may run perpendicular to the
// away-ray but never fold back toward the train. A click the server collapses
// onto the walker is no lane, while a partial result (the last legal cell) is a
// running click whose validated point serves as the endpoint
// (docs/hunting.md).

func (l *Loop) kiteLaneResolve(
    selfX, selfY, selfZ, endX, endY int32,
    dirX, dirY float64,
) (int32, int32, bool) {
    laneX, laneY, ok := l.kiteTerrainLane(
        selfX, selfY, selfZ, endX, endY)
    if !ok {
        return 0, 0, false
    }
    laneLen := math.Hypot(
        float64(laneX-selfX), float64(laneY-selfY))
    if (float64(laneX-selfX)/laneLen)*dirX+
        (float64(laneY-selfY)/laneLen)*dirY < kiteHalfPlaneSlack {
        // The lane folded back toward the train: stepping it would walk into
        // the melee the kite exists to avoid.
        return 0, 0, false
    }
    if l.navigator == nil {
        return laneX, laneY, true
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ)}
    to := pathfind.Vec3{X: float64(laneX), Y: float64(laneY),
        Z: float64(selfZ)}
    valid, runs := l.navigator.ValidateClick(from, to)
    if !runs {
        return 0, 0, false
    }
    validX := int32(math.Round(valid.X))
    validY := int32(math.Round(valid.Y))
    if math.Hypot(float64(validX-selfX),
        float64(validY-selfY)) < kiteMinStep {
        // The validated point collapsed onto a run shorter than the shortest
        // useful hop: a 38 unit walk buys no distance and spends a whole reload
        // window, so the battery treats it as no lane.
        return 0, 0, false
    }

    return validX, validY, true
}
