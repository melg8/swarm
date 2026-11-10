// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The train geometry and the straight retreat lane of the kite fight
// (split from kite.go in the round-17 refactor, the QA debt of the
// file-size rule): the chaser bearings, the centroid and gap
// directions, the terrain contract (the fight-anchor leash, the
// water, the SERVER click validation) and the away half-plane
// guard. The owner rule of the 2026-11-09 round keeps the retreat
// STRAIGHT - the line directly away from the train and nothing
// else: no fan, no camp deflection, no circling chord. The lane
// battery tries the same straight ray at the full leg and the
// progressively shorter hops before it holds (the live 17:43 dump:
// the bot-side line of sight refused lanes the server accepts and
// the cornered verdict fired on an open runway); a lane every
// straight rung refuses still holds the ground and shoots
// (kiteHoldGround). The triggers, the shot rhythm and the re-click
// ladder stay in kite.go.

import (
    "math"
    "sort"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// kiteLaneRungCount bounds the straight-hop ladder of one retreat
// resolution: the full leg, the three quartered shortenings and the
// kiteMinStep floor (see kiteLaneRungs). A fixed count keeps the
// resolution allocation free.
const (
    // kiteLaneRungCount bounds the straight-hop ladder of one retreat
    // resolution: the full leg, the three quartered shortenings and
    // the kiteMinStep floor (see kiteLaneRungs). A fixed count keeps
    // the resolution allocation free.
    kiteLaneRungCount = 5
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

// kiteContactAway resolves the flight ray of a melee attacker
// standing ON the character. At zero distance the train geometry has
// no away bearing at all (kiteChaserBearings skips the contact
// member: the vector between the two bodies is empty) and the flight
// used to spend the tick on nothing - the 2026-11-10 run-29
// acceptance dump stood a healthy level 7 archer at (35516,51008)
// for 21 seconds while the level 10 Kaboo Orc Fighter broadcast from
// the very same coordinates landed 15 damage every 1.9 s, the health
// fell 78 -> 11 percent and the session ended at 6 percent after the
// emergency logout. Contact is the one scene the ground hold never
// owns: every swing lands from there, and the server attack timeout
// (AttackableAI MAX_ATTACK_TIMEOUT) only starts once the blows stop
// landing. The flight therefore takes the first OPEN ring ray instead
// of standing - the character's own facing first (the direction its
// client last moved or shot), then the two perpendiculars and the
// back ray - and each candidate is a straight full escape segment,
// never a turn: the owner rule of the round keeps the retreat
// straight, and a contact escape has no previous leg to bend away
// from. Reports the unit ray and whether any straight lane opens.
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

// kiteLaneRungs resolves the straight-hop ladder of one retreat
// resolution, longest first: the caller's leg, the three quartered
// shortenings and the kiteMinStep floor. The lane battery walks the
// rungs in order and takes the first the server accepts, so a
// straight ray whose FAR end is walled still walks the reachable
// prefix instead of holding - the live 17:43 dump measured the
// "cornered" hold firing on a lane the geodata probe found wide open
// at every probe distance, and the 17:44 fight repeated it: the
// fixed fence and the strict sight gate answered the hold while the
// server would have walked. The rungs never exceed the caller's leg
// (the ring budget and the race length own the length) and never
// fall under kiteMinStep (a shorter hop buys no distance and the
// cornered hold is the honest answer). The ladder is a fixed array -
// the resolution runs once per shot cycle and stays allocation free.

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

// kiteLaneEscapeAngles is the blocked-lane escape order of the
// retreat battery: the preferred away-ray first, then the smallest
// turns inside the away half-plane, both ways (22.5, 45, 67.5 and 90
// degrees). The owner rule of the 2026-11-09 round keeps the retreat
// STRAIGHT and removed the old fan; the angular escape is the
// corner-only fallback for the live 2026-11-09 acceptance spot
// (34792, 49416): the direct away-ray west was walled at every
// length while the south, east and north rays stood open at 250-900,
// so a hard "hold the ground" answer stood the archer at melee for
// the whole fight. The escape never folds back toward the train (the
// half-plane guard), never fans wide of the perpendicular and never
// replaces the preferred ray: it only walks the closest straight
// bearing the terrain actually offers.
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

// kiteRetreatLane resolves the retreat lane of a kite step: the
// STRAIGHT ray the caller preferred (the centroid away-vector of the
// train, the widest-gap bisector of an encircled one), at the
// caller's step length (the ring-budgeted leg of kiteStraightLeg).
// The owner rule of the
// 2026-11-09 round keeps the primary retreat straight: when a rung
// of the preferred ray walks, that rung is the lane, and the ray is
// never bent by a fan or a chord (a turn spends the distance the
// retreat just bought). Only when EVERY rung of the preferred ray is
// blocked does the battery walk the escape angles (see
// kiteLaneEscapeAngles) - the nearest straight bearing the terrain
// offers, still inside the away half-plane - and a scene every angle
// refuses stays the cornered hold (the archer stands and shoots).
// Each rung runs the terrain contract, the away half-plane guard and
// the SERVER click validation; the first accepted rung names the
// endpoint. Reports the endpoint and whether a lane walks.

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

// kiteTerrainLane resolves the TERRAIN contract of the retreat
// endpoint: the fight-anchor leash, the degenerate length and the
// water. The wall gate is the SERVER click validation of
// kiteLaneResolve (the bot-side line of sight was stricter than the
// server: the live 17:43 dump held a lane the server pathfinder
// walks). Without a navigator (the no-geodata runtime, the plain
// unit scenes) the leash alone fences the step.

func (l *Loop) kiteTerrainLane(
    selfX, selfY, selfZ, endX, endY int32,
) (int32, int32, bool) {
    if l.target != 0 && l.kiteFightFor == l.target {
        // The straight-leg fence (the round-14 leash, the live-target
        // re-anchor of the 2026-11-09 round): the endpoint must stay
        // inside the fight's own roam radius around the fight anchor -
        // the target's stand, re-read on every resolution (see
        // kiteResolveAndClick). A fight that starts anywhere on any
        // map keeps its retreats on its own ground; the zone tables
        // never gate the kite again.
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

// kiteLaneResolve resolves the retreat lane to its final endpoint
// and reports whether the lane carries a step: the terrain contract
// of kiteTerrainLane, the away half-plane guard and the server click
// validation. The half-plane gate guards the rounding and the callers
// that hand in a raw candidate ray: a lane may run perpendicular to
// the away-ray, but never fold back toward the chasing train. The
// click validation rides the SERVER rules (pathfind.ValidateClick
// ports Creature.moveToLocation and the GeoEngine correction): a
// click the server collapses onto the walker is no lane, while a
// partial result (the last legal cell of the line) IS a running click
// and its validated point serves as the endpoint. The bot-side
// LineOfSight is NOT the lane gate any more: it refuses corridors the
// server walks (the live 17:43 dump held an open runway), and the
// server validation is the contract the click lives under.

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
        // The validated point collapsed onto a partial run shorter
        // than the shortest useful hop: a 38 unit "walk" buys no
        // distance and spends a whole reload window (the live
        // 2026-11-09 acceptance run issued exactly one and stood
        // through the next cycle), so the lane battery treats it as
        // no lane. A genuinely shorter hop cannot pass the rung
        // ladder anyway (kiteLaneRungs floors at kiteMinStep).
        return 0, 0, false
    }

    return validX, validY, true
}
