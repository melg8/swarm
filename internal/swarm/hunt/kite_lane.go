// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The train geometry and the straight retreat lane of the kite fight
// (split from kite.go in the round-17 refactor, the QA debt of the
// file-size rule): the chaser bearings, the centroid and gap
// directions, the terrain contract (the fight-anchor leash, the
// geodata wall, the water) and the away half-plane guard. The owner
// rule of the 2026-11-09 round keeps the retreat STRAIGHT - the
// line directly away from the train and nothing else: no fan, no
// camp deflection, no circling chord. A blocked straight lane holds
// the ground and shoots (kiteHoldGround). The triggers, the shot
// rhythm and the re-click ladder stay in kite.go.

import (
    "math"
    "sort"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

func (l *Loop) kiteChaserBearings(
    selfX, selfY, selfZ int32, out []float64,
) []float64 {
    // The fight target weighs first: it chases the retreat step
    // wherever it lands, its away vector always counts.
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
            // Another deck: its swings cannot land, its lane crosses
            // the geodata wall.
            continue
        }
        dx := float64(selfX) - float64(chaser.X)
        dy := float64(selfY) - float64(chaser.Y)
        dist := math.Hypot(dx, dy)
        if dist >= kiteTrainScanRange || dist < 1 {
            // Too far to reach the character inside a step window -
            // the flee machinery owns a chase that distant - or
            // standing on the character itself (no away direction).
            continue
        }
        out = append(out, math.Atan2(
            float64(chaser.Y-selfY), float64(chaser.X-selfX)))
        if len(out) == cap(out) {
            // The scratch is full: the gap geometry of a wider
            // train reads its nearest members alone (see
            // kiteMaxTrainMembers).
            break
        }
    }

    return out
}

// kiteGapDirection resolves the escape ray of an encircled train:
// the bisector of the WIDEST angular gap between the chaser
// bearings - the direction that stays farthest from every flanker
// at once. The live fleet round of issue #70 measured what the
// standing encircled answer costs on the mass cells (a 26 second
// stand with the train growing to three chasers, the potions and
// the emergency logout of the wounded slots): the archer that keeps
// stepping through the widest gap breaks the pile-up before it
// forms, the one that stands invites it. The perpendicular break of
// a two-mob line is the classic case - the bisector of the two 180
// degree gaps opens the distance to BOTH chasers at once. Reports
// the unit direction and whether the bearings leave a gap at all (a
// single bearing or none pins no escape geometry).

func kiteGapDirection(bearings []float64) (float64, float64, bool) {
    if len(bearings) < 2 {
        return 0, 0, false
    }
    sorted := make([]float64, len(bearings))
    copy(sorted, bearings)
    sort.Float64s(sorted)
    // The widest gap between consecutive bearings, the wraparound
    // included: the first widest gap wins the ties (a deterministic
    // answer for the symmetric trains - two opposite chasers name
    // the same perpendicular ray every call).
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

// kiteTrainDirection resolves the retreat DIRECTION of one kite
// step: the centroid away-vector of the chaser train - the fight
// target's own away unit vector plus every SelfAttackers member
// within the scan range and on a reachable deck. Each chaser weighs
// one unit - the centroid semantics the issue names; the nearest
// mob drags the direction hardest only through the trigger radius it
// already crossed (the armed threat of kiteThreat). A train whose
// away vectors cancel under the encirclement share names no
// centroid - the WIDEST-GAP bisector takes the step over (the live
// fleet round of issue #70 measured the standing encircled answer
// as a death trap on the mass cells: the train only grows while the
// archer stands - the gap ray keeps the movement contract of the
// kite, the lane battery still owns the terrain, and the hold
// ground answer survives for the straight gap that is walled). The
// positions are the raw last-known packet ones (the projected scan
// only serves the nearest chaser); the trigger margin absorbs the
// broadcast lag. Reports the unit direction (the centroid ray of a
// normal train, the gap bisector of an encircled one, the zero of
// the degenerate no-chaser scene) and whether the train encircles
// the character.

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
        // The degenerate no-chaser scene: no ray to guard at all -
        // the hold keeps the pacing until the next probe. The empty
        // list is tested EXPLICITLY: a single bearing sums to a unit
        // vector whose hypot may land one ulp UNDER 1 (the live
        // round-19 run measured the phantom surround on two single
        // chaser fights - cos/sin rounding read as the encirclement
        // and the zero gap answer pinned the archer at melee), so a
        // magnitude comparison can never separate "one chaser" from
        // "no chaser".
        return 0, 0, true
    }
    if sumLen < kiteEncircleShare*count {
        // The encircled sum: the centroid names no direction - the
        // widest-gap bisector serves the step.
        if gapX, gapY, gap := kiteGapDirection(bearings); gap {
            return gapX, gapY, true
        }

        return 0, 0, true
    }

    return sumX / sumLen, sumY / sumLen, false
}

// kiteRetreatLane resolves the one retreat lane of a kite step: the
// STRAIGHT ray the caller preferred (the centroid away-vector of the
// train, the widest-gap bisector of an encircled one), at the
// caller's step length (the race-aware leg of kiteStepLength, the
// ring-budgeted leg of kiteStraightLeg). The owner rule of the
// 2026-11-09 round removed the fan: when the straight lane is
// blocked the archer holds the ground and shoots instead of turning
// (a turn spends the distance the retreat just bought). The
// endpoint still runs the terrain contract and the away half-plane
// guard. Reports the endpoint and whether the straight lane walks.

func (l *Loop) kiteRetreatLane(
    selfX, selfY, selfZ int32, prefX, prefY, awayX, awayY float64,
    step float64,
) (int32, int32, bool) {
    endX := selfX + int32(math.Round(prefX*step))
    endY := selfY + int32(math.Round(prefY*step))

    return l.kiteLaneResolve(
        selfX, selfY, selfZ, endX, endY, awayX, awayY)
}

// kiteTerrainLane resolves the TERRAIN contract of the retreat
// endpoint: the fight-anchor leash, the degenerate length, the
// geodata wall and the water. The terrain gates need the navigator -
// without one (the no-geodata runtime, the plain unit scenes) the
// leash alone fences the step.

func (l *Loop) kiteTerrainLane(
    selfX, selfY, selfZ, endX, endY int32,
) (int32, int32, bool) {
    if l.target != 0 && l.kiteFightFor == l.target {
        // The location-general leash (the round-14 redesign): the
        // endpoint must stay inside the fight's own roam radius
        // around the fight anchor - the live-observed ground the
        // fight started on (see kiteResolveAndClick), not the
        // generated hunting-square registry. A fight that starts
        // anywhere on any map keeps its retreats on its own ground;
        // the zone tables never gate the kite again.
        if math.Hypot(float64(endX-l.kiteFightX),
            float64(endY-l.kiteFightY)) > kiteRoamRadius {
            return 0, 0, false
        }
    }
    laneX, laneY := float64(endX-selfX), float64(endY-selfY)
    if math.Hypot(laneX, laneY) < 1 {
        return 0, 0, false
    }
    if l.navigator == nil {
        // No geodata runtime: the leash alone fences the step (the
        // contract of the first kite round).
        return endX, endY, true
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ)}
    to := pathfind.Vec3{X: float64(endX), Y: float64(endY),
        Z: float64(selfZ)}
    if sight, err := l.navigator.LineOfSight(from, to); err != nil ||
        !sight {
        // A wall (or an unreadable geodata) behind: the lane is a
        // corridor the walk cannot take.
        return 0, 0, false
    }
    if l.navigator.OverWater(float64(endX), float64(endY),
        int16(selfZ)) {
        // Water behind: swimming trades the bow for the paddle.
        return 0, 0, false
    }

    return endX, endY, true
}

// kiteLaneResolve resolves the retreat lane to its final endpoint
// and reports whether the lane carries a step: the terrain contract
// of kiteTerrainLane plus the away half-plane guard. The half-plane
// gate guards the rounding and the callers that hand in a raw
// candidate ray: a lane may run perpendicular to the away-ray, but
// never fold back toward the chasing train.

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
        // The lane folded back toward the train: stepping it would
        // walk INTO the melee the kite exists to avoid.
        return 0, 0, false
    }

    return laneX, laneY, true
}
