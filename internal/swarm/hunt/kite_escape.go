// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The cornered-pocket escape ladders of the kite fight (split from
// kite.go in the round-17 refactor, the QA debt of the file-size
// rule): the anti-gap breakout cone and the shove tier that answer a
// hemisphere the lane battery (kite_lane.go) could not serve. The
// triggers, the shot rhythm and the re-click ladder stay in kite.go.

import (
    "fmt"
    "math"
)

func (l *Loop) kiteBreakoutResolve(
    selfX, selfY, selfZ int32, dead [][2]int32,
) (int32, int32, string, bool) {
    bearings := l.kiteChaserBearings(
        selfX, selfY, selfZ, l.kiteBearings[:0])
    gapX, gapY, gap := kiteGapDirection(bearings)
    if !gap {
        if len(bearings) != 1 {
            // No chaser bearing at all (the tracker gap inside the
            // broadcast window): no cone exists to probe - the
            // cornered hold owns the cycle.
            return 0, 0, "no gap geometry (the tracker owns no " +
                "bearing)", false
        }
        // THE LONE-CHASER TERRAIN CORNER (the round-11/12 live
        // diagnostics named every observed pocket this): the
        // hemisphere sweep died on the pocket walls, and no seam
        // cone exists - a lone chaser has no seams to thread. The
        // escape lives PAST the hemisphere edge, in the wall-face
        // wedges the fan never sweeps (its 90 degree reach stops
        // where these candidates start): the away ray rotated 105,
        // 120 and 135 degrees on each side, the least fold first.
        // The chaser entered through the pocket mouth, so the
        // candidates run past its flanks - the same flank
        // clearance the anti-gap ladder holds (every resolved lane
        // keeps more than ~36 degrees off the chaser's own
        // bearing), the same terrain battery (the deflection, the
        // leash, the geodata wall, the water), the same dead-cell
        // skip. The half-plane guard the normal fan answers to
        // stays bypassed exactly as the anti-gap ladder bypasses
        // it: the fold toward the chaser's flank IS the escape, the
        // clearance keeps it off the melee; the pursuit ledger's
        // stall gate owns the race the slower fold runs.
        awayX := -math.Cos(bearings[0])
        awayY := -math.Sin(bearings[0])
        crowded, walled := 0, 0
        for _, off := range [6]float64{
            7 * math.Pi / 12, -7 * math.Pi / 12, // the 105s
            2 * math.Pi / 3, -2 * math.Pi / 3, // the 120s
            3 * math.Pi / 4, -3 * math.Pi / 4, // the 135s
        } {
            rayX, rayY := rotatePlanar(awayX, awayY, off)
            laneX, laneY, status := l.kiteBreakoutCandidate(
                selfX, selfY, selfZ, rayX, rayY, dead, bearings)
            switch status {
            case kiteBreakoutOpen:
                return laneX, laneY, "", true
            case kiteBreakoutCrowded:
                crowded++
            default:
                walled++
            }
        }
        reason := fmt.Sprintf("the lone-chaser cone refused (%d of "+
            "%d rays crowded by the flanks, %d walled or dead)",
            crowded, crowded+walled, walled)

        return 0, 0, reason, false
    }
    crowded, walled := 0, 0
    for _, off := range [3]float64{
        3 * math.Pi / 4, -3 * math.Pi / 4, math.Pi,
    } {
        rayX, rayY := rotatePlanar(gapX, gapY, off)
        laneX, laneY, status := l.kiteBreakoutCandidate(
            selfX, selfY, selfZ, rayX, rayY, dead, bearings)
        switch status {
        case kiteBreakoutOpen:
            return laneX, laneY, "", true
        case kiteBreakoutCrowded:
            crowded++
        default:
            walled++
        }
    }
    // Every candidate of the anti-gap ladder refused: the reason
    // names the split so the next live round reads the pocket's
    // shape straight from the hold diagnostic (the fleet round of
    // the breakout measured fourteen silent refusals - the event
    // feed must carry the why, not just the hold).
    reason := fmt.Sprintf("the cone refused (%d of %d rays crowded "+
        "by the flanks, %d walled or dead)",
        crowded, crowded+walled, walled)

    return 0, 0, reason, false
}

// The candidate verdicts of the breakout ladders: the lane stands
// open, a chaser bearing crowds it, or the terrain battery (or the
// dead-cell memory) walled it.

func (l *Loop) kiteBreakoutCandidate(
    selfX, selfY, selfZ int32,
    rayX, rayY float64,
    dead [][2]int32, bearings []float64,
) (int32, int32, int) {
    if !kiteRayClearsTheFlanks(rayX, rayY, bearings) {
        // The candidate runs onto a chaser's own ray: the melee
        // the kite exists to avoid, refused before the terrain
        // battery pays a raycast on it.
        return 0, 0, kiteBreakoutCrowded
    }
    endX := selfX + int32(math.Round(rayX*l.kite.Step))
    endY := selfY + int32(math.Round(rayY*l.kite.Step))
    laneX, laneY, ok := l.kiteTerrainLane(
        selfX, selfY, selfZ, endX, endY)
    if ok {
        laneLen := math.Hypot(
            float64(laneX-selfX), float64(laneY-selfY))
        if laneLen < 1 || !kiteRayClearsTheFlanks(
            float64(laneX-selfX)/laneLen,
            float64(laneY-selfY)/laneLen, bearings) {
            // The camp deflection bent the resolved lane onto a
            // chaser's ray: the endpoint the walk would take
            // fails the same clearance the candidate passed.
            ok = false
        }
    }
    if !ok {
        return 0, 0, kiteBreakoutWalled
    }
    for _, cell := range dead {
        if laneX == cell[0] && laneY == cell[1] {
            // A cell the rotation already named dead (the server
            // refused it or it stayed silent through the probe) -
            // the next candidate serves the breakout instead.
            return 0, 0, kiteBreakoutWalled
        }
    }

    return laneX, laneY, kiteBreakoutOpen
}

// kiteRayClearsTheFlanks answers whether the planar unit ray keeps
// the breakout flank clearance off every chaser bearing: the dot
// product of the ray with each bearing's unit vector stays under
// kiteBreakoutFlankCos (the angular distance stays above ~36
// degrees). The pre-filter runs on the raw candidate before the
// terrain battery, the post-check on the deflected lane the walk
// would take - both read the same bearings scratch.

func kiteRayClearsTheFlanks(
    rayX, rayY float64, bearings []float64,
) bool {
    return kiteRayClearsTheFlanksAt(
        rayX, rayY, bearings, kiteBreakoutFlankCos)
}

// kiteRayClearsTheFlanksAt is the parameterized flank clearance of
// kiteRayClearsTheFlanks: the cosine bound rides the caller (the
// breakout cone holds the 36 degree bound, the shove tier accepts
// the tighter 25 degree seams of the sealed pocket).

func kiteRayClearsTheFlanksAt(
    rayX, rayY float64, bearings []float64, flankCos float64,
) bool {
    for _, bearing := range bearings {
        if rayX*math.Cos(bearing)+rayY*math.Sin(bearing) >=
            flankCos {
            return false
        }
    }

    return true
}

// kiteShoveResolve is the SHOVE tier of the cornered pocket (issue
// #70, the round-14 always-run redesign): the last escape before
// the hold. The breakout cone probes the folds AROUND the widest
// gap (the gap ray rotated 135 degrees and 180) under the strict 36
// degree flank bound; the shove probes the gap ray ITSELF (and its
// +-15/+-30 degree edges) under the tighter 25 degree bound and a
// shorter step - the seam the cone refuses by design is exactly
// the lane a sealed pocket has left, and a burst through it beats
// the standing melee trade the hold answers with.
//
// The round-14 live fleet measured the residual case the
// pass-through tier closes: a 3+ mob pack at melee range leaves NO
// ray inside the 25 degree bound (the surround holds ate whole
// windows at 29-80 units, the archer trading arrows point-blank).
// When every clean seam refuses by CROWDING, the LEAST-CROWDED
// terrain-passable ray still goes - a step through a mob's flank
// takes a glance blow on the way out, the standing hold takes the
// whole pack's swings for as long as the fight stands. The terrain
// battery (the camp deflection, the location-general leash, the
// geodata wall, the water) and the dead-cell memory gate BOTH
// tiers - a walled or server-refused endpoint never carries a
// walk, and only a pocket walled on every side keeps the hold.

func (l *Loop) kiteShoveResolve(
    selfX, selfY, selfZ int32, dead [][2]int32,
) (int32, int32, bool) {
    bearings := l.kiteChaserBearings(
        selfX, selfY, selfZ, l.kiteBearings[:0])
    gapX, gapY, gap := kiteGapDirection(bearings)
    if !gap {
        // A lone chaser or a tracker gap: no seam exists to thread
        // (the lone-chaser cone owns the wall-face wedges past the
        // hemisphere edge) - the hold owns the answer.
        return 0, 0, false
    }
    candidates := [5]float64{
        0, math.Pi / 12, -math.Pi / 12,
        math.Pi / 6, -math.Pi / 6,
    }
    // Tier one: the clean seam (every chaser bearing kept past the
    // 25 degree flank bound). Tier two books the least-crowded
    // terrain-passable ray along the way - the pass-through answer
    // of the 3+ mob pack (see the function comment).
    worstFlank := 2.0
    worstX, worstY := int32(0), int32(0)
    worstOK := false
    for _, off := range candidates {
        rayX, rayY := rotatePlanar(gapX, gapY, off)
        endX := selfX + int32(math.Round(rayX*kiteShoveStep))
        endY := selfY + int32(math.Round(rayY*kiteShoveStep))
        laneX, laneY, ok := l.kiteTerrainLane(
            selfX, selfY, selfZ, endX, endY)
        if !ok || deadLane(laneX, laneY, dead) {
            continue
        }
        // The post-deflection flank read (the QA audit of the
        // round caught the gap): the terrain battery may deflect
        // the endpoint around a camp, and the deflected lane - not
        // the raw candidate ray - is the walk the character takes.
        // The clearance verdicts (the clean-seam test and the
        // least-crowded book) read the RESOLVED lane's direction,
        // the same double read the breakout tier runs.
        laneLen := math.Hypot(float64(laneX-selfX),
            float64(laneY-selfY))
        if laneLen < 1 {
            continue
        }
        laneRX, laneRY := float64(laneX-selfX)/laneLen,
            float64(laneY-selfY)/laneLen
        worst := -2.0
        for _, bearing := range bearings {
            if c := laneRX*math.Cos(bearing) +
                laneRY*math.Sin(bearing); c > worst {
                worst = c
            }
        }
        if worst < kiteShoveFlankCos {
            // The clean seam: the first candidate whose RESOLVED
            // lane clears every flank serves the shove.
            return laneX, laneY, true
        }
        if worst < worstFlank {
            // The least-crowded terrain-passable lane so far (the
            // pass-through tier's book).
            worstFlank, worstX, worstY, worstOK =
                worst, laneX, laneY, true
        }
    }
    if worstOK {
        // Tier two: the pass-through - no clean seam exists, the
        // thinnest rank of the pack carries the step anyway.
        return worstX, worstY, true
    }

    return 0, 0, false
}

// deadLane reports whether the lane endpoint sits in the dead-cell
// memory (the server refused it or it stayed silent through the
// probe).

func deadLane(laneX, laneY int32, dead [][2]int32) bool {
    for _, cell := range dead {
        if laneX == cell[0] && laneY == cell[1] {
            return true
        }
    }

    return false
}

// kiteDeflectFromCamps bends one retreat candidate around the idle
// aggressive camps its line would wake: the first threat circle
// (the effective aggro range plus the steering clearance) the
// straight self-to-end segment enters deflects the endpoint onto the
// tangent ray of that circle, on the side the segment leaned to -
// the same geometry the transit walk steering uses
// (steerClearOfAggro), minus its destination exemption: a retreat
// has no destination-mob contract, a camp standing anywhere on the
// lane (its endpoint included) must not be met. The chasers never
// deflect the lane: they already hold the character as their target
// (the scan skips them by the held target). Reports the deflected
// endpoint and whether a deflection happened.
