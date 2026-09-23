// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The train geometry and the retreat lane battery of the kite fight
// (split from kite.go in the round-17 refactor, the QA debt of the
// file-size rule): the chaser bearings, the centroid and gap
// directions, the fan battery with its dead-cell skipping, the
// terrain contract (the camp deflection, the fight-anchor leash, the
// geodata wall, the water) and the half-plane guard. The escape
// ladders (the breakout, the shove) live in kite_escape.go; the
// triggers, the shot rhythm and the re-click ladder stay in kite.go.

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
// ground answer survives for the gap that is walled or dead). The
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
    if sumLen < kiteEncircleShare*count || sumLen < 1 {
        // The encircled sum (or the degenerate zero): the centroid
        // names no direction - the widest-gap bisector serves the
        // step when the bearings leave one.
        if gapX, gapY, gap := kiteGapDirection(bearings); gap {
            return gapX, gapY, true
        }

        return 0, 0, true
    }

    return sumX / sumLen, sumY / sumLen, false
}

// kiteRetreatLaneSkipping resolves the retreat lane of one kite step:
// the straight away-ray first, then the fan candidates at kiteFanStep
// increments each side of it - the open backward lanes over the
// blocked corridors, the hemisphere edge as the last resort - minus
// the candidates whose RESOLVED endpoints sit
// in the dead set: the re-click ladder rotates a dead endpoint onto
// the next fan candidate (the destination-cell refusal answer of
// issue #60 - a cell the server refuses never starts a walk, so
// clicking it again changes nothing) and the rotation needs the
// lane battery to answer "which lane comes after these". An empty
// dead set keeps every candidate (the plain resolution of the
// first round). Every candidate runs the full lane gate battery
// (the camp deflection, the leash, the away half-plane, the wall
// and the water) at the caller's step LENGTH (the race-aware leg
// of kiteStepLength - the fan shares one length, the guards stay
// length-honest). Reports the endpoint and whether a walkable lane
// exists.

func (l *Loop) kiteRetreatLaneSkipping(
    selfX, selfY, selfZ int32, prefX, prefY, awayX, awayY float64,
    step float64, dead [][2]int32,
) (int32, int32, bool) {
    // The candidate rays of the away hemisphere around the
    // PREFERRED direction (the straight away-ray of the opening
    // retreat, the chord bearing of the circling ones): the
    // preferred ray first (the lane of record), then the fan
    // candidates widening symmetrically around it - the 45 degree
    // lanes before the 90 degree ones. The half-plane reference
    // stays the RAW away vector whatever the preference leans to.
    var rays [1 + 2*kiteFanSteps][2]float64
    rays[0] = [2]float64{prefX, prefY}
    count := 1
    for stepIdx := 1; stepIdx <= kiteFanSteps; stepIdx++ {
        angle := kiteFanStep * float64(stepIdx)
        for _, sign := range [2]float64{1, -1} {
            rays[count][0], rays[count][1] =
                rotatePlanar(prefX, prefY, sign*angle)
            count++
        }
    }
    for _, ray := range rays[:count] {
        endX := selfX + int32(math.Round(ray[0]*step))
        endY := selfY + int32(math.Round(ray[1]*step))
        laneX, laneY, ok := l.kiteLaneResolve(
            selfX, selfY, selfZ, endX, endY, awayX, awayY)
        if !ok {
            continue
        }
        refused := false
        for _, cell := range dead {
            if laneX == cell[0] && laneY == cell[1] {
                refused = true

                break
            }
        }
        if refused {
            // A cell the rotation already named dead (the server
            // refused it or it stayed silent through the probe) -
            // the next candidate serves the retreat instead.
            continue
        }

        return laneX, laneY, true
    }

    return 0, 0, false
}

// kiteTerrainLane resolves the TERRAIN contract of one retreat
// candidate: the camp deflection first (the tangent endpoint
// replaces the straight one when the lane would wake a neighborhood
// camp - the deflected endpoint runs the remaining gates like any
// candidate), then the leash, the degenerate length, the geodata
// wall and the water. The half-plane guard of the ordinary battery
// is NOT here: the breakout tier replaces it with its own flank
// clearance (see kiteBreakoutResolve). The terrain gates need the
// navigator - without one (the no-geodata runtime, the plain unit
// scenes) the leash alone fences the step.

func (l *Loop) kiteTerrainLane(
    selfX, selfY, selfZ, endX, endY int32,
) (int32, int32, bool) {
    if defX, defY, dodged := l.kiteDeflectFromCamps(
        selfX, selfY, selfZ, endX, endY); dodged {
        endX, endY = defX, defY
    }
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

// kiteLaneResolve resolves one retreat lane candidate to its final
// endpoint and reports whether the lane carries a step: the terrain
// contract of kiteTerrainLane plus the away half-plane guard. The
// half-plane gate guards the deflections hardest: a lane may bend
// sideways of the away-ray (the camp side-step exits the trigger
// circle perpendicular), but never fold back toward the chasing
// train.

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
        // The lane (a deep camp deflection, most likely) folded back
        // toward the train: stepping it would walk INTO the melee the
        // kite exists to avoid.
        return 0, 0, false
    }

    return laneX, laneY, true
}

// kiteBreakoutResolve resolves the CORNERED-POCKET breakout lane of
// the kite (issue #70, the walled-pocket round): when the hemisphere
// sweep found no walkable lane - the raw away-ray of a normal train,
// the gap ray of an encircled one - the escape may still live in the
// cone that sweep never covered, on the FAR side of the chaser
// line. The live fleet round measured the walled pocket of the hex
// cells eating whole windows: the archer stood through hold after
// hold (an 8.5 s median shot-to-retreat lag over four retreats)
// while the chasers shifted, because the only open lanes threaded
// between them - a step the away half-plane guard forbids by
// design. The breakout admits those lanes under a STRICTER
// contract than the guard it replaces: the flank clearance. Every
// candidate keeps more than ~36 degrees off EVERY chaser bearing
// (it weaves between the train's seams, it never runs onto a mob's
// own ray), the full terrain battery still applies (the camp
// deflection, the leash, the geodata wall, the water), the resolved
// lane re-clears the flank bound after the deflection may have bent
// it, and the dead cells stay skipped like any other lane. The
// candidate ladder fans the ANTI-gap side - the three rays the
// failed hemisphere never swept (its 93 degree reach leaves the
// opposite cone uncovered) - ordered by the escape preference: the
// 135 degree weaves off the gap ray first, the straight anti-gap
// ray last (it points into the train's middle, the clearance gate
// usually refuses it). A train of one no longer dies on the missing
// seam: the LONE-CHASER ladder (see the inline block) probes the
// wall-face wedges past the hemisphere edge instead - the escape
// past the chaser's flanks. No bearing at all (a tracker gap)
// names no geometry: the cornered hold owns the cycle. Reports the
// endpoint, the refusal reason (empty on success) and whether a
// breakout lane exists.
//
// Bounds (the round-11 QA audit): the ladders are DISCRETE ray
// sets, not cone sweeps - the anti-gap ladder leaves the wedges
// between the hemisphere edge (~93 degrees off the away-ray) and
// its 135 degree weaves unswept, and the lone-chaser ladder steps
// the same band at 105/120/135 (a pocket whose only lane sits at
// ~110 degrees still slips between the rungs - the sweep density
// is a live-tuning knob, not a correctness claim). The flank
// clearance is ANGULAR-only and distance-blind: a chaser 700 units
// off on a candidate's bearing vetoes it exactly like one at melee
// (kiteBreakoutFlankCos knows no radius), and a deflection that
// bends the resolved lane inside the flank bound reads as "walled"
// in the refusal reason even when the pre-deflection ray cleared.
// The live rounds of the walled-pocket session measured the
// observed pockets as LONE-CHASER terrain corners: the anti-gap
// ladder (two bearings minimum) never ran there, and the
// lone-chaser ladder is the round-12 answer to exactly that
// finding - its live verdict owns the next fleet round.

func (l *Loop) kiteDeflectFromCamps(
    selfX, selfY, selfZ, endX, endY int32,
) (int32, int32, bool) {
    segmentX := float64(endX - selfX)
    segmentY := float64(endY - selfY)
    segmentLen := math.Hypot(segmentX, segmentY)
    if segmentLen < avoidMinSegment {
        return endX, endY, false
    }
    l.avoidScratch = l.tracker.AppendAggroThreats(
        l.avoidScratch[:0], l.target, avoidScanRange)
    if len(l.avoidScratch) == 0 {
        return endX, endY, false
    }
    // The first camp the straight lane would wake: the smallest
    // along-lane parameter wins (the earliest wake), the deepest
    // penetration breaks the tie - the same selection rule as the
    // transit steering.
    first := -1
    firstT := 1.0
    firstPen := 0.0
    for i := range l.avoidScratch {
        threat := &l.avoidScratch[i]
        relX := threat.X - float64(selfX)
        relY := threat.Y - float64(selfY)
        t := (relX*segmentX + relY*segmentY) /
            (segmentLen * segmentLen)
        if t < 0 {
            t = 0
        } else if t > 1 {
            t = 1
        }
        cx := float64(selfX) + segmentX*t
        cy := float64(selfY) + segmentY*t
        // The planar clearance mirrors the server's own 3D trigger:
        // the retreat lane runs on the character's deck, a camp on
        // another deck never blocks it.
        dz := float64(threat.Z) - float64(selfZ)
        clearance := math.Hypot(
            math.Hypot(threat.X-cx, threat.Y-cy), dz)
        needed := threat.AggroRange + avoidClearance
        if pen := needed - clearance; pen > 0 && (t < firstT ||
            (t == firstT && pen > firstPen)) {
            first, firstT, firstPen = i, t, pen
        }
    }
    if first < 0 {
        return endX, endY, false
    }
    threat := &l.avoidScratch[first]
    defX, defY := tangentClearDirection(
        float64(selfX), float64(selfY), threat.X, threat.Y,
        segmentX/segmentLen, segmentY/segmentLen,
        threat.AggroRange+avoidClearance)

    return selfX + int32(math.Round(defX*l.kite.Step)),
        selfY + int32(math.Round(defY*l.kite.Step)), true
}

// kiteHoldGround arms the cornered hold of the kite: the archer
// stops retreating and keeps shooting the bow at melee range. The
// hold paces its own re-probe (the kite period) so a failed lane
// never stutters the tick loop, and logs its reason once per hold
// episode - a standing fight that names itself stays diagnosable in
// the event feed. The surrounded flag picks the diagnostic wording
// (the encircled train whose gap ray found no lane against the
// walled corner), the breakout string names why the pocket breakout
// refused the cone (the crowd, the terrain, the missing gap
// geometry - empty when the breakout never applied).
