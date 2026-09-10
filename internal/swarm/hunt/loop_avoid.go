// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The aggro-aware movement layer of the hunt loop: the transit walks
// (the town trips both ways, the zone returns, the inter-ground walks
// of the spot economy) steer clear of the aggressive mobs camped on
// their lines. The Mobius AttackableAI attacks a player on sight when
// the 3D distance drops inside the mob's effective aggro range (the
// xml value clamped at the server wide MaxAggroRange, 450 on this
// deployment) and a line of sight exists, so a walk that passes an
// idle aggressive mob closer than that collects a chaser - the train
// that later forces the pile up run and the emergency relogin. The
// steering deflects one walk leg at a time around such camps: the leg
// direction rides the tangent of the threat circle (the effective
// aggro range plus the clearance margin), so the issued leg never
// enters the trigger distance; the walk follower re-issues its
// requests every period from the live position, and the chained
// tangent legs arc the route around the camp while the underlying
// waypoint plan stays untouched. A character already inside the
// margin circle side-steps straight out first (the tangent does not
// exist from inside). Mobs that stand at the destination itself are
// exempt: the last stretch of a walk INTO a hunting ground
// deliberately meets its mobs, and the engage phase answers whatever
// the entry radius offers.

import (
        "math"
        "time"
)

// The steering constants of the aggro-aware movement.
const (
        // avoidClearance is the margin the steering keeps between the
        // walk line and an aggressive mob beyond its effective aggro
        // range: the projected positions lag the real server positions by
        // up to a broadcast interval and the mob may drift while the leg
        // walks, so the raw trigger radius alone would graze the circle.
        avoidClearance = 150.0
        // avoidMinLeg suppresses the steering on legs too short to bend
        // anywhere (a stop request, an arrival shuffle).
        avoidMinLeg = 100.0
        // avoidScanRange bounds the threat scan around the character: the
        // legs issue at most maxMoveLeg ahead, and only mobs within that
        // leg plus their trigger circle plus the clearance can endanger
        // the line.
        avoidScanRange = 1600.0
        // avoidArriveExempt drops the steering for the mobs standing at
        // the destination of the walk itself: the ground the walk
        // deliberately enters carries its own mobs (a hunting spot, the
        // merchant square), and passing within their aggro range on
        // arrival is the point of the walk - the engage phase answers
        // whatever the entry radius offers.
        avoidArriveExempt = 500.0
        // avoidLogPeriod paces the detour diagnostic: a busy corridor
        // bends every leg, the log names the corridor once per period.
        avoidLogPeriod = 5 * time.Second
)

// steerClearOfAggro deflects one walk leg around the aggressive mobs
// camped on its line: the leg end moves onto the tangent ray of the
// first threat circle the straight line would enter - the on-sight
// trigger distance (the effective aggro range plus the clearance
// margin) - on the side the original leg leaned to. The receding
// horizon does the rest: the walk follower re-issues its request
// every period from the live position, and every re-issue re-spawns
// the tangent, so the chained legs arc the route around the camp
// without touching the waypoint plan. The endangerment check is 3D
// like the server's own trigger (a mob on another deck never blocks
// a ground line), the tangent itself is planar (the walk cannot steer
// vertically anyway). Threats already holding a target never steer
// (the scan skips them: a chaser belongs to the flee machinery),
// threats at the destination are exempt (the walk wants to meet
// them), and the current fight target drops out through its id.
// Reports the adjusted endpoint and whether a deflection happened.
func (l *Loop) steerClearOfAggro(
        fromX int32, fromY int32, fromZ int32,
        toX int32, toY int32, toZ int32,
        destX int32, destY int32,
        now time.Time,
) (int32, int32, bool) {
        legX, legY := float64(toX-fromX), float64(toY-fromY)
        legLen := math.Hypot(legX, legY)
        if legLen < avoidMinLeg {
                return toX, toY, false
        }
        l.avoidScratch = l.tracker.AppendAggroThreats(
                l.avoidScratch[:0], l.target, avoidScanRange)
        if len(l.avoidScratch) == 0 {
                return toX, toY, false
        }
        // The first threat the straight leg would wake: the smallest
        // along-line parameter among the endangered mobs (the deepest
        // penetration breaks the tie - the mob the line grazes hardest).
        first := -1
        firstT := 1.0
        firstPen := 0.0
        for i := range l.avoidScratch {
                threat := &l.avoidScratch[i]
                if math.Hypot(threat.X-float64(destX), threat.Y-float64(destY)) <=
                        avoidArriveExempt {
                        // The mob stands at the destination of the whole walk: the
                        // ground the walk deliberately enters.
                        continue
                }
                // The closest point of the segment to the threat and its 3D
                // clearance, mirroring the server's own isInsideRadius3D
                // trigger (a mob on another deck never blocks a ground line).
                relX, relY := threat.X-float64(fromX), threat.Y-float64(fromY)
                t := (relX*legX + relY*legY) / (legLen * legLen)
                if t < 0 {
                        t = 0
                } else if t > 1 {
                        t = 1
                }
                cx := float64(fromX) + legX*t
                cy := float64(fromY) + legY*t
                dz := float64(threat.Z) - (float64(fromZ) + (float64(toZ)-float64(fromZ))*t)
                clear2 := math.Hypot(threat.X-cx, threat.Y-cy)
                clear3 := math.Hypot(clear2, dz)
                needed := threat.AggroRange + avoidClearance
                if pen := needed - clear3; pen > 0 && (t < firstT ||
                        (t == firstT && pen > firstPen)) {
                        first, firstT, firstPen = i, t, pen
                }
        }
        if first < 0 {
                return toX, toY, false
        }
        threat := &l.avoidScratch[first]
        dirX, dirY := tangentClearDirection(
                float64(fromX), float64(fromY), threat.X, threat.Y,
                legX/legLen, legY/legLen,
                threat.AggroRange+avoidClearance)
        endX := float64(fromX) + dirX*legLen
        endY := float64(fromY) + dirY*legLen
        endXI, endYI := int32(math.Round(endX)), int32(math.Round(endY))
        if l.avoidLogAt.IsZero() || now.Sub(l.avoidLogAt) >= avoidLogPeriod {
                l.avoidLogAt = now
                l.logger.Printf("Hunt: steering the walk around %s at %d %d "+
                        "(leg %d %d -> %d %d bends to %d %d)",
                        threat.Name, int32(math.Round(threat.X)),
                        int32(math.Round(threat.Y)),
                        fromX, fromY, toX, toY, endXI, endYI)
        }

        return endXI, endYI, true
}

// tangentClearDirection resolves the deflected leg direction around
// one threat circle: the tangent ray from the position to the circle
// of the needed radius, on the side the original leg direction leans
// to (the shorter arc around the camp). A position already inside
// the circle has no tangent - the direction becomes the pure
// side-step perpendicular to the threat axis there (the walk leaves
// the margin circle straight before any tangent ride, and the
// side-step never closes a unit of the distance: the perpendicular
// keeps the radius constant while the leg lasts).
func tangentClearDirection(
        fromX, fromY, threatX, threatY float64,
        legUX, legUY float64,
        needed float64,
) (float64, float64) {
        towardX, towardY := threatX-fromX, threatY-fromY
        sp := math.Hypot(towardX, towardY)
        if sp < 1 {
                // The threat sits on the position itself: any perpendicular
                // leaves it behind; right of the leg wins by convention.
                return legUY, -legUX
        }
        towardX, towardY = towardX/sp, towardY/sp
        if sp <= needed {
                // Inside the margin circle: the side-step perpendicular to
                // the threat axis, on the side the leg leans to (the arc the
                // route would take anyway), straight out of the circle.
                side := towardX*legUY - towardY*legUX
                if side >= 0 {
                        return -towardY, towardX
                }

                return towardY, -towardX
        }
        // The tangent half-angle: the angle between the threat axis and
        // the grazing ray (sin of it is the needed radius over the
        // distance). The deflected direction is the threat axis rotated
        // by it, toward the side the original leg leans to.
        beta := math.Asin(math.Min(1, needed/sp))
        s := 1.0
        if towardX*legUY-towardY*legUX < 0 {
                s = -1
        }
        cos, sin := math.Cos(beta), s*math.Sin(beta)

        return cos*towardX - sin*towardY, sin*towardX + cos*towardY
}
