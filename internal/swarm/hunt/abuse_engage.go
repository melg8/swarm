package hunt

import (
    "math"
    "time"
)

// abuseEngageClaimFactor picks how deep inside the engage radius the
// engage claim of the abuse movement mode lands: half the radius
// keeps the point safely inside the range the loop itself attacks
// from (see engageRadiusFor) while staying clear of the collision
// hull of the target (the collision radii of a mob and a character
// sum to a few dozen units).
const abuseEngageClaimFactor = 0.5

// abuseEngagePoint picks the claim point of an abuse engage claim: on
// the line from the target toward the character, half the engage
// radius out. The claimed placement lands the character inside the
// weapon range of the target (the melee swing, the bow shot) without
// standing inside the target model.
func abuseEngagePoint(
    targetX, targetY, selfX, selfY int32, radius float64,
) (int32, int32) {
    dx := float64(selfX - targetX)
    dy := float64(selfY - targetY)
    length := math.Hypot(dx, dy)
    if length < 1 {
        // A degenerate line (the character stands on the target):
        // any offset of the factor keeps the claim off the standing
        // point, the direction is irrelevant.
        return targetX + int32(radius*abuseEngageClaimFactor), targetY
    }
    offset := radius * abuseEngageClaimFactor
    if offset > length-1 {
        // The character stands barely outside the radius: keep the
        // claim a unit short of the standing point.
        offset = length - 1
    }
    scale := offset / length

    return targetX + int32(dx*scale), targetY + int32(dy*scale)
}

// abuseEngageClaim lands the character next to the fight target
// through the movement abuse channel instead of the server side
// chase: the -abuse launch flag swapped every walk of the session
// onto the adopted position claims, but the approach to a mob kept
// its direct shape - the attack request arms the chase and the server
// AI runs the character to the target at run speed while the chase
// stays healthy (the stall watchdog only walks when the chase gives
// up, and the walk fallback of the manual attack waits out a running
// chase the same way). The engage claim closes that last leg exactly
// like the flag closes every other: ONE claim at the engage point
// inside the weapon radius of the target, so the swing (the bow shot)
// starts the moment the claim is adopted - the chase the server
// planned dies with the placement change, no run leg ever covers the
// distance.
//
// The report is whether the abuse mode owns the approach leg: true
// when a claim left the wire this tick OR the pacing holds the tick
// back while the echo of the last claim lands - the callers must not
// fall back onto the ordinary flow in either case, the server side
// run they would start is exactly the movement the mode replaces.
// The claim repeats at the engage retry period while the target keeps
// its distance (a fleeing mob, a target whose echo has not arrived
// yet); a target inside the engage radius, a session without the
// flag, an unknown position or an unselected target all report false
// and leave the ordinary flow untouched.
func (l *Loop) abuseEngageClaim(target int32, now time.Time) bool {
    if target == 0 || !l.game.AbuseMovementEnabled() {
        return false
    }
    x, y, z, ok := l.tracker.ObjectPosition(target)
    if !ok {
        return false
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    radius := l.engageRadiusFor()
    dist := math.Hypot(float64(x-selfX), float64(y-selfY))
    if dist <= radius {
        return false
    }
    if l.abuseEngageFor == target &&
        now.Sub(l.abuseEngageAt) < engageRetryPeriod {
        // The echo of the last claim needs a tick to land in the
        // tracker: the paced out tick owns the approach without
        // stacking another claim on the same point.
        return true
    }
    l.abuseEngageFor = target
    l.abuseEngageAt = now
    claimX, claimY := abuseEngagePoint(x, y, selfX, selfY, radius)
    if err := l.game.WalkTo(claimX, claimY, z); err != nil {
        l.logf("Hunt: abuse engage claim failed: %v", err)

        return true
    }
    l.logf("Hunt: abuse engage claim on %d at %d units, "+
        "teleporting into the fight", target, int(dist))

    return true
}
