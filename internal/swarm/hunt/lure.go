// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The covered pick lure: a melee hunter whose pick stands inside the aggro
// radius of a covering pack approaches the archer way - a standoff point
// outside every threat circle, the bow and quiver armed on the walk, the
// ordinary forced attack fired once the live target is inside the weapon
// range. The attacked mob runs to the shooter, the cover does not, and the
// arrival swaps the melee weapon back (docs/hunting.md).

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The timing and geometry constants of the luring.
const (
    // lureStandoff is the standoff ring: 20 units inside the 500 unit bow
    // range leave the target drift of the walk no way out of the range.
    lureStandoff = 480.0
    // lureShotRange is the live target distance the forced attack may fire
    // from: every C1 bow carries attack_range 500.
    lureShotRange = 500.0
    // lureArriveRange is the distance the pulled mob closes to before the
    // melee weapon swaps back.
    lureArriveRange = 200.0
    // lureSegmentPeriod paces the approach: one walk request per second, the
    // far target walk cadence.
    lureSegmentPeriod = 1 * time.Second
    // lureSwapHold bounds the melee handoff: the server defers the swap to the
    // bow attack end (UseItem), and the bow disable window never passes five
    // seconds. The auto equipment restores a refused swap after the lure.
    lureSwapHold = 5 * time.Second
    // lureStandEpsilon is the arrival radius of the standoff ring.
    lureStandEpsilon = 50.0
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
    // lureApproach walks to the standoff and arms the ranged tool on the way.
    lureApproach lurePhase = iota
    // lureShoot holds the standoff and shoots the pulled mob.
    lureShoot
    // lureMelee swaps the melee weapon back for the close fight.
    lureMelee
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
    swapAt     time.Time
    armedBow   bool
    armedArrow bool
    selected   bool
    onSpot     bool
}

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
    l.startLure(now, bowID, arrowID, meleeObjID, standX, standY, false)
    l.logf("Hunt: %s stands covered by %s: luring it with the bow "+
        "from the standoff %d %d", l.targetName(l.target), coverName,
        standX, standY)
}

// startLure installs the lure: the kite motion stands down first, the melee
// weapon is remembered for the swap back.
func (l *Loop) startLure(
    now time.Time, bowID, arrowID, meleeObjID int32,
    standX, standY int32, onSpot bool,
) {
    l.kiteStandDown()
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
        swapAt:     time.Time{},
        armedBow:   false,
        armedArrow: false,
        selected:   false,
        onSpot:     onSpot,
    }
}

// lureTick advances the lure state machine, reporting whether the tick is
// consumed: the approach and melee phases own the movement and action budget,
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
    case lureShoot:
        return l.lureShootTick(lure, now)
    case lureMelee:
        return l.lureMeleeTick(lure, now)
    }

    return false
}

// lureApproachTick walks the character to the standoff point one paced
// segment at a time and arms the tool on the way: the paces of the walk are
// the retry budget of the equips, so the arrival finds the bow ready.
func (l *Loop) lureApproachTick(lure *lureState, now time.Time) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return true
    }
    targetX, targetY, _, targetOK := l.tracker.ObjectPosition(lure.target)
    if !targetOK {
        return true
    }
    targetDist := math.Hypot(float64(targetX-selfX), float64(targetY-selfY))
    l.lureSelect(lure, now)
    l.lureArmStep(lure, now)
    if !l.lureArmed() {
        return false
    }
    toolReady := l.lureToolReady(lure)
    if lure.onSpot {
        // The spawn-settle strike fires from the spot: no standoff walk.
        if toolReady {
            lure.phase = lureShoot

            return false
        }

        return true
    }
    if toolReady && targetDist <= lureShotRange {
        lure.phase = lureShoot
        l.logf("Hunt: luring %s: inside the bow range at %d units",
            l.targetName(lure.target), int(targetDist))

        return false
    }
    if !lure.segmentAt.IsZero() && now.Sub(lure.segmentAt) < lureSegmentPeriod {
        return true
    }
    if now.Before(l.combatAvoidUntil) {
        // The impending-add step owns the movement: the approach walk waits
        // the hold out, the arming and the selection do not.
        return true
    }
    if math.Hypot(float64(lure.standX-selfX), float64(lure.standY-selfY)) <=
        lureStandEpsilon && targetDist > lureShotRange {
        // The target wandered off the ring: re-aim the standoff at its live
        // position instead of standing out of the range.
        lure.standX, lure.standY = l.lureStandoffPoint(
            selfX, selfY, targetX, targetY)
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

// lureSelect sends the click that selects the pull target: the Mobius
// AttackRequest selects on the first click and forces on the repeated one, so
// the approach spends the first and the range entry spends the second. The
// stamp is the shared action pacing: both clicks ride the same flood window.
func (l *Loop) lureSelect(lure *lureState, now time.Time) {
    if lure.selected {
        return
    }
    if l.tracker.SelfTargetID() == lure.target {
        // An earlier action already selected it: the next request is forced.
        lure.selected = true

        return
    }
    if now.Sub(l.lastActionAt) < selectPeriod {
        return
    }
    if err := l.game.AttackTarget(lure.target); err != nil {
        l.logf("Hunt: lure target selection failed: %v", err)

        return
    }
    lure.selected = true
    l.lastActionAt = now
}

// lureArmStep equips the bow and then the quiver through the use item
// requests: the next request waits the paperdoll confirmation of the previous
// one, the use item flood protector is disabled.
func (l *Loop) lureArmStep(lure *lureState, now time.Time) {
    if len(l.userDeferred) > 0 {
        // The manual command queue owns the item action budget right now.
        return
    }
    kind, weaponOK := l.tracker.SelfWeaponKind()
    if !weaponOK || kind != weaponTypeBow {
        l.lureArmBow(lure, now)

        return
    }
    l.lureArmQuiver(lure, now)
}

// lureArmBow sends the bow equip once: the confirmation is the paperdoll
// weapon family, so the quiver never races the slot the bow frees.
func (l *Loop) lureArmBow(lure *lureState, now time.Time) {
    if lure.armedBow {
        // The equip request is in flight: the paperdoll confirms it.
        return
    }
    if !l.inventoryItemAllowed(lure.bowObjID,
        gear.ServerWriteSlots(l.equipment(), lure.bowObjID)) {
        return
    }
    l.markInventoryAction(lure.bowObjID)
    if err := l.game.UseItem(lure.bowObjID); err != nil {
        l.logf("Hunt: lure bow equip failed: %v", err)
        l.abortLure("the bow refused to equip", now)

        return
    }
    lure.armedBow = true
    l.logf("Hunt: luring %s: the bow is out", l.targetName(lure.target))
}

// lureArmQuiver sends the quiver equip once the paperdoll shows the bow: the
// left hand slot the bow freed is the one the quiver fills.
func (l *Loop) lureArmQuiver(lure *lureState, now time.Time) {
    if lure.armedArrow {
        return
    }
    if l.tracker.PaperdollSlotObjectIDs()[state.PaperdollLHand] ==
        lure.arrowObjID {
        // An interrupted lure left the quiver in hand: the toggle would take
        // it off.
        lure.armedArrow = true

        return
    }
    if !l.inventoryItemAllowed(lure.arrowObjID,
        gear.ServerWriteSlots(l.equipment(), lure.arrowObjID)) {
        return
    }
    l.markInventoryAction(lure.arrowObjID)
    if err := l.game.UseItem(lure.arrowObjID); err != nil {
        l.logf("Hunt: lure quiver equip failed: %v", err)
        l.abortLure("the quiver refused to equip", now)

        return
    }
    lure.armedArrow = true
    l.logf("Hunt: luring %s: the quiver is on, waiting for the range",
        l.targetName(lure.target))
}

// lureToolReady reports whether the ranged tool is confirmed: the bow in the
// right hand and the quiver in the left, both read from the paperdoll.
func (l *Loop) lureToolReady(lure *lureState) bool {
    if !lure.armedArrow {
        return false
    }
    kind, ok := l.tracker.SelfWeaponKind()
    if !ok || kind != weaponTypeBow {
        return false
    }
    paperdoll := l.tracker.PaperdollSlotObjectIDs()

    return paperdoll[state.PaperdollLHand] == lure.arrowObjID
}

// lureShootTick watches the pulled mob run: the ordinary attack flow keeps
// the ranged auto attack up, the arrival hands the tick to the melee handoff.
func (l *Loop) lureShootTick(lure *lureState, now time.Time) bool {
    targetX, targetY, _, ok := l.tracker.ObjectPosition(lure.target)
    if !ok {
        l.clearLure()

        return false
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    dist := math.Hypot(float64(targetX-selfX), float64(targetY-selfY))
    if dist > lureArriveRange {
        return false
    }
    // The mob closed to the melee range: a leftover kite motion stands down
    // and the sword swap takes the tick.
    l.kiteStandDown()
    if lure.meleeObjID == 0 {
        // The bow was already worn when the lure began: the auto equipment
        // restores the melee weapon after the lure.
        l.clearLure()

        return false
    }
    lure.phase = lureMelee
    lure.swapAt = now
    l.markInventoryAction(lure.meleeObjID)
    if err := l.game.UseItem(lure.meleeObjID); err != nil {
        l.logf("Hunt: lure melee swap failed: %v", err)
        l.clearLure()

        return false
    }
    l.logf("Hunt: the pulled %s arrived, back to the melee",
        l.targetName(lure.target))

    return true
}

// lureMeleeTick holds the first melee request until the paperdoll confirms
// the sword: the equip and the attack would race on the server executor.
func (l *Loop) lureMeleeTick(lure *lureState, now time.Time) bool {
    kind, ok := l.tracker.SelfWeaponKind()
    if ok && kind != weaponTypeBow {
        l.logf("Hunt: the sword is back, closing into melee on %s",
            l.targetName(lure.target))
        l.clearLure()

        return false
    }
    if now.Sub(lure.swapAt) >= lureSwapHold {
        l.logf("Hunt: the melee swap did not confirm within %s",
            lureSwapHold)
        l.clearLure()

        return false
    }

    return true
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

func (l *Loop) targetName(objectID int32) string {
    if name := l.tracker.ObjectName(objectID); name != "" {
        return name
    }

    return "the target"
}
