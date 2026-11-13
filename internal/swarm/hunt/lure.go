// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The covered pick lure: a melee hunter whose pick stands inside the aggro
// radius of a covering pack approaches the archer way - a standoff point
// outside every threat circle, the bow and quiver armed, the ordinary forced
// attack fired from there. The attacked mob runs to the shooter, the cover
// does not, and the arrival swaps the melee weapon back (docs/hunting.md).

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The timing and geometry constants of the luring.
const (
    // lureStandoff stops the approach a hair inside the 500 unit bow range,
    // far outside the melee approach that would walk into the cover.
    lureStandoff = 420.0
    // lureArriveRange is the distance the pulled mob closes to before the
    // melee weapon swaps back.
    lureArriveRange = 200.0
    // lureSegmentPeriod paces the approach: one walk request per second, the
    // far target walk cadence.
    lureSegmentPeriod = 1 * time.Second
    // lureArmPeriod paces the equip requests: the flood protector window of
    // the player actions.
    lureArmPeriod = 2 * time.Second
    // lureTimeout bounds the lure: a mob that never answers the pull releases
    // the target to the ordinary skip machinery.
    lureTimeout = 30 * time.Second
    // lureCoverSlack widens the cover detection: the melee approach ends
    // within ~100 units of the pick, so the reaching circle fires on it.
    lureCoverSlack = 200.0
    // lureCoverProbe walks the standoff candidates: eight directions, the
    // character side first.
    lureCoverProbe = 8
)

// weaponTypeBow is the gear weapon family of the bows.
const weaponTypeBow = "BOW"

// lurePhase is one step of the lure state machine.
type lurePhase int

const (
    // lureApproach walks the character to the standoff point.
    lureApproach lurePhase = iota
    // lureArm equips the bow and the quiver.
    lureArm
    // lureShoot fires the pull and holds the position.
    lureShoot
)

// lureState is the lure of one picked target.
type lureState struct {
    phase      lurePhase
    target     int32
    meleeObjID int32
    bowObjID   int32
    arrowObjID int32
    standX     int32
    standY     int32
    startedAt  time.Time
    segmentAt  time.Time
    armAt      time.Time
    armedBow   bool
    armedArrow bool
}

// lureArmed reports whether a lure owns the current pick.
func (l *Loop) lureArmed() bool {
    return l.lure != nil
}

// maybeBeginLure takes a fresh pick over when the target stands covered and
// the hunter owns the ranged tool; otherwise the ordinary attack flow runs.
func (l *Loop) maybeBeginLure(now time.Time) {
    if l.equip == nil || !l.profileLuresWithBow() {
        return
    }
    bowID, arrowID, hasTool := l.bowAndArrow()
    if !hasTool {
        return
    }
    covered, coverName := l.targetCovered(l.target)
    if !covered {
        return
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    targetX, targetY, _, targetOK := l.tracker.ObjectPosition(l.target)
    if !targetOK {
        return
    }
    standX, standY := l.lureStandoffPoint(
        selfX, selfY, targetX, targetY)
    var meleeObjID int32
    paperdoll := l.tracker.PaperdollSlotObjectIDs()
    if paperdoll[state.PaperdollRHand] != 0 {
        kind, weaponOK := l.tracker.SelfWeaponKind()
        if weaponOK && kind == weaponTypeBow {
            // The bow is already worn (an interrupted lure): the arm phase
            // skips its own equip.
            meleeObjID = 0
        } else {
            meleeObjID = paperdoll[state.PaperdollRHand]
        }
    }
    l.lure = &lureState{
        phase:      lureApproach,
        target:     l.target,
        meleeObjID: meleeObjID,
        bowObjID:   bowID,
        arrowObjID: arrowID,
        standX:     standX,
        standY:     standY,
        startedAt:  now,
        segmentAt:  time.Time{},
        armAt:      time.Time{},
        armedBow:   false,
        armedArrow: false,
    }
    l.logf("Hunt: %s stands covered by %s: luring it with the bow "+
        "from the standoff %d %d", l.targetName(l.target), coverName,
        standX, standY)
}

// lureTick advances the lure state machine, reporting whether the tick is
// consumed: the approach and arm phases own the movement and action budget,
// the shoot phase lets the ordinary attack flow fire and watches the arrival.
func (l *Loop) lureTick(now time.Time) bool {
    lure := l.lure
    if lure == nil {
        return false
    }
    if now.Sub(lure.startedAt) > lureTimeout {
        l.abortLure("the pull never engaged", now)

        return false
    }
    if lure.target != l.target || !l.tracker.ObjectAlive(lure.target) {
        // The target switched or died mid lure: the ordinary flow owns the
        // aftermath, the weapon restores through the auto equipment.
        l.clearLure()

        return false
    }
    switch lure.phase {
    case lureApproach:
        return l.lureApproachTick(lure, now)
    case lureArm:
        return l.lureArmTick(lure, now)
    case lureShoot:
        l.lureShootTick(lure)

        return false
    }

    return false
}

// lureApproachTick walks the character to the standoff point one paced
// segment at a time; the arrival hands the phase over to the arm.
func (l *Loop) lureApproachTick(lure *lureState, now time.Time) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return true
    }
    dist := math.Hypot(float64(lure.standX-selfX), float64(lure.standY-selfY))
    if dist < patrolCenterMinDist/2 {
        lure.phase = lureArm
        lure.armAt = time.Time{}

        return true
    }
    if !lure.segmentAt.IsZero() && now.Sub(lure.segmentAt) < lureSegmentPeriod {
        return true
    }
    lure.segmentAt = now
    moveX, moveY, _ := walkClickSegment(selfX, selfY, selfZ,
        float64(lure.standX), float64(lure.standY), float64(selfZ),
        returnWalkSegment)
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logf("Hunt: lure approach walk failed: %v", err)
    }

    return true
}

// lureArmTick equips the ranged tool, the bow first and then the quiver; the
// phase completes when the paperdoll shows both pieces.
func (l *Loop) lureArmTick(lure *lureState, now time.Time) bool {
    if !lure.armAt.IsZero() && now.Sub(lure.armAt) < lureArmPeriod {
        return true
    }
    if len(l.userDeferred) > 0 {
        // The manual command queue owns the item action budget right now.
        return true
    }
    kind, weaponOK := l.tracker.SelfWeaponKind()
    bowWorn := weaponOK && kind == weaponTypeBow
    if !bowWorn {
        if !lure.armedBow {
            if !l.inventoryItemAllowed(lure.bowObjID,
                gear.ServerWriteSlots(l.equipment(), lure.bowObjID)) {
                // The equip request is in flight: the confirmation gate holds
                // the arm until the paperdoll answers.
                lure.armAt = now

                return true
            }
            l.markInventoryAction(lure.bowObjID)
            if err := l.game.UseItem(lure.bowObjID); err != nil {
                l.logf("Hunt: lure bow equip failed: %v", err)
                l.abortLure("the bow refused to equip", now)

                return false
            }
            lure.armedBow = true
            lure.armAt = now
            l.logf("Hunt: luring %s: the bow is out", l.targetName(lure.target))

            return true
        }
        // The equip confirmation is still in flight: hold the next action.
        lure.armAt = now

        return true
    }
    if !lure.armedArrow {
        if !l.inventoryItemAllowed(lure.arrowObjID,
            gear.ServerWriteSlots(l.equipment(), lure.arrowObjID)) {
            // The quiver races the bow freeing the left hand: hold until the
            // write set confirms.
            lure.armAt = now

            return true
        }
        l.markInventoryAction(lure.arrowObjID)
        if err := l.game.UseItem(lure.arrowObjID); err != nil {
            l.logf("Hunt: lure quiver equip failed: %v", err)
            l.abortLure("the quiver refused to equip", now)

            return false
        }
        lure.armedArrow = true
        lure.armAt = now
        l.logf("Hunt: luring %s: the quiver is on, firing the pull",
            l.targetName(lure.target))

        return true
    }
    lure.phase = lureShoot

    return false
}

// lureShootTick watches the pulled mob run: the ordinary attack flow keeps
// the ranged auto attack up, the arrival swaps the melee weapon back.
func (l *Loop) lureShootTick(lure *lureState) {
    targetX, targetY, _, ok := l.tracker.ObjectPosition(lure.target)
    if !ok {
        l.clearLure()

        return
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return
    }
    dist := math.Hypot(float64(targetX-selfX), float64(targetY-selfY))
    if dist > lureArriveRange {
        return
    }
    // The mob closed to the melee range: back to the sword and the ordinary
    // fight.
    l.clearLure()
    if lure.meleeObjID != 0 {
        l.markInventoryAction(lure.meleeObjID)
        if err := l.game.UseItem(lure.meleeObjID); err != nil {
            l.logf("Hunt: lure melee swap failed: %v", err)

            return
        }
        l.logf("Hunt: the pulled %s arrived, back to the melee",
            l.targetName(lure.target))
    }
}

// abortLure ends the lure on a failure: the target skips for a while (the
// ordinary engage skip machinery) and the weapon restores through the auto
// equipment.
func (l *Loop) abortLure(reason string, now time.Time) {
    lure := l.lure
    if lure == nil {
        return
    }
    name := l.targetName(lure.target)
    l.clearLure()
    if lure.target != 0 {
        if l.targetSkip == nil {
            l.targetSkip = make(map[int32]time.Time)
        }
        l.targetSkip[lure.target] = now.Add(engageSkipDelay)
    }
    if lure.target == l.target {
        l.target = 0
        l.engageAt = time.Time{}
    }
    l.logf("Hunt: the lure of %s aborted: %s", name, reason)
}

// clearLure drops the lure state: the auto equipment restores the melee
// weapon because the bow scores zero for the melee profile.
func (l *Loop) clearLure() {
    l.lure = nil
}

// profileLuresWithBow reports whether the gear profile fights in melee (the
// mystic casts from range, no luring tool).
func (l *Loop) profileLuresWithBow() bool {
    if l.equip == nil {
        return false
    }

    return l.equip.profile.Name() == "melee fighter"
}

// bowAndArrow resolves the strongest bow (equipped or bagged) and the largest
// arrow stack of the inventory; a missing piece answers false.
func (l *Loop) bowAndArrow() (int32, int32, bool) {
    var bowID, arrowID int32
    var bowPower int32
    var arrowCount int32
    for _, item := range l.tracker.InventoryItems() {
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if ok && stats.WeaponType == "BOW" {
            if stats.PAtk > bowPower {
                bowPower = stats.PAtk
                bowID = item.ObjectID
            }

            continue
        }
        if ammo, ok := npcdata.ItemGearStats(item.ItemID); ok &&
            ammo.Type == "EtcItem" && ammo.BodyPart == "lhand" &&
            ammo.WeaponType == "" && item.Count > arrowCount {
            arrowCount = item.Count
            arrowID = item.ObjectID
        }
    }
    if bowID == 0 || arrowID == 0 {
        return 0, 0, false
    }

    return bowID, arrowID, true
}

// targetCovered reports whether an idle aggressive mob's on-sight circle
// reaches the target neighborhood or the straight approach segment; the
// second answer names the cover for the log.
func (l *Loop) targetCovered(targetID int32) (bool, string) {
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    targetX, targetY, _, targetOK := l.tracker.ObjectPosition(targetID)
    if !selfOK || !targetOK {
        return false, ""
    }
    span := math.Hypot(float64(targetX-selfX), float64(targetY-selfY)) + 800
    threats := l.tracker.AppendAggroThreats(nil, targetID, span)
    for index := range threats {
        threat := &threats[index]
        near := math.Hypot(threat.X-float64(targetX), threat.Y-float64(targetY))
        if near <= threat.AggroRange+lureCoverSlack {
            return true, threat.Name
        }
        if segmentDistance(
            float64(selfX), float64(selfY),
            float64(targetX), float64(targetY),
            threat.X, threat.Y,
        ) <= threat.AggroRange {
            return true, threat.Name
        }
    }

    return false, ""
}

// lureStandoffPoint resolves the shooting position: the direction from the
// target toward the character first, then the eight compass directions until
// a candidate sits outside every threat circle; the fallback keeps the
// character side.
func (l *Loop) lureStandoffPoint(
    selfX int32, selfY int32, targetX int32, targetY int32,
) (int32, int32) {
    dx := float64(selfX - targetX)
    dy := float64(selfY - targetY)
    if dist := math.Hypot(dx, dy); dist < 1 {
        dx, dy = 1, 0
    } else {
        dx, dy = dx/dist, dy/dist
    }
    threats := l.tracker.AppendAggroThreats(nil, 0, math.MaxFloat64)
    angle := math.Atan2(dy, dx)
    for attempt := range lureCoverProbe {
        radians := angle + float64(attempt)*math.Pi/4
        cx := float64(targetX) + math.Cos(radians)*lureStandoff
        cy := float64(targetY) + math.Sin(radians)*lureStandoff
        clean := true
        for index := range threats {
            threat := &threats[index]
            if math.Hypot(threat.X-cx, threat.Y-cy) <= threat.AggroRange {
                clean = false

                break
            }
        }
        if clean {
            return int32(math.Round(cx)), int32(math.Round(cy))
        }
    }

    return int32(math.Round(float64(targetX) + dx*lureStandoff)),
        int32(math.Round(float64(targetY) + dy*lureStandoff))
}

// segmentDistance measures the point to the line segment (the approach
// corridor check of the cover detection).
func segmentDistance(
    ax, ay, bx, by, px, py float64,
) float64 {
    dx, dy := bx-ax, by-ay
    length := dx*dx + dy*dy
    if length < 1e-9 {
        return math.Hypot(px-ax, py-ay)
    }
    t := ((px-ax)*dx + (py-ay)*dy) / length
    if t < 0 {
        t = 0
    }
    if t > 1 {
        t = 1
    }

    return math.Hypot(px-(ax+dx*t), py-(ay+dy*t))
}

// targetName resolves the display name of the object id for the log.
func (l *Loop) targetName(objectID int32) string {
    if name := l.tracker.ObjectName(objectID); name != "" {
        return name
    }

    return "the target"
}
