// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Town trips of the hunt loop: a full inventory walks the character to the
// nearest town shop over the geodata (Navigator), sells the junk and walks back
// to the farm spot. The sell request uses the standard inventory sell list: the
// Mobius server prices every item itself at referencePrice/2, so no shop window
// flow is needed, only the merchant interaction distance (docs/hunting.md).
package hunt

import (
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

func (l *Loop) tripActive() bool {
    switch l.phase {
    case phaseTownWalk, phaseTownSell, phaseTownReturn:
        return true
    default:
        return false
    }
}

// tripCooldownOver reports whether a new town trip may start. The bare handed
// weapon run, the armed gear debt and the guide refill retry on the short
// weaponRunCooldown (45 s) instead of the five minute trip cooldown: punching
// mobs or farming without the armor the merchant sold is the wound those runs
// prevent.
func (l *Loop) tripCooldownOver() bool {
    if l.tripEndedAt.IsZero() {
        return true
    }
    cooldown := tripCooldown
    if l.weaponlessRunWanted() || l.gearDebtRunWanted() ||
        l.guideRunWanted() {
        // The guide run shares the short window: the support magic lapses by
        // the server push, and the landed buffs keep guideWanted false for 1200
        // seconds.
        cooldown = weaponRunCooldown
    } else if l.tripAbortRun > tripAbortEscalateAfter {
        // The abort streak escalates: a trip failing for a reason no retry
        // fixes only burns the ticks between identical aborts.
        cooldown = tripAbortCooldown(l.tripAbortRun)
    }

    return l.now().Sub(l.tripEndedAt) >= cooldown
}

// inventoryFull reports whether the inventory passed a trip trigger threshold:
// more than half of the slots or of the maximum weight (tripSlotPercent,
// tripWeightPercent). The selling phase reuses it as the stop condition.
func (l *Loop) inventoryFull() bool {
    stats := l.tracker.InventoryStats()

    return stats.SlotPercent > tripSlotPercent ||
        stats.WeightPercent > tripWeightPercent
}

// maybeStartTownTrip begins a town trip when the inventory is full
// enough or the shop strategy has a plan worth a trip and the trip
// cooldown is over. Every blocker (no navigator, no geodata, no path)
// arms the cooldown, so a broken deployment does not retry every tick.
//
//nolint:cyclop,funlen // the trigger and stop freeze decision tree
func (l *Loop) maybeStartTownTrip() {
    // The first trip waits for the server skill list: the learning stops plan
    // on the queue, and a trip starting between the enter world ItemList and
    // SkillList would plan without it. skillListWaitLimit (10 s) keeps a silent
    // server sellable.
    if !l.tracker.SkillsListed() &&
        l.now().Sub(l.tracker.SessionStartedAt()) < skillListWaitLimit {
        return
    }
    shopping := l.shoppingTripEnabled() && l.shoppingWanted()
    learning := l.learnTripWanted()
    // The weapon run outranks every other trip reason.
    weaponRun := l.weaponlessRunWanted()
    // The guide run is the buff refill of the Newbie Guide. It rides the
    // ordinary sell stop, the guide stop planning behind the learning ones.
    guideRun := l.guideRunWanted()
    if l.navigator == nil || !l.tripCooldownOver() ||
        (!l.inventoryFull() && !shopping && !learning &&
            !weaponRun && !guideRun) {
        return
    }
    // A bot outside the hunting zone returns first. The weapon and guide runs
    // are the exceptions - a weapon walks first whatever the distance, and the
    // village revive lands next to the guide; the learning stops stay inside
    // the gate.
    if !weaponRun && !guideRun && l.zone() != nil && !l.inZoneSelf() {
        return
    }
    // The trip owns the way from here: a held engage target drops with its
    // server selection, so the old fight never binds the replacing walk.
    l.dropAttackTarget("the town trip owns the way")
    // The walk needs a standing character.
    if !l.standUpGuarded(l.now()) {
        return
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    merchant, ok := l.nearestMerchant(selfX, selfY)
    if !ok {
        return
    }
    // The trip plan freezes here once: the sell first step banks the SellFirst
    // credits and the stop planning distributes them, so a re-plan at the shop
    // would run against the freed slots and the fresh adena. The scroll of
    // escape leg runs before the freeze.
    l.maybeEscapeWithSOE(merchant)
    l.tripPlan = l.shoppingPlan()
    // The starting gear is the baseline the trip exits compare against: a slot
    // emptied without a landed replacement becomes gear debt (gearDebtCheck).
    l.snapshotTripGear()
    // The weapon leads the trip that buys it: the sell stop routes to the
    // weapon merchant, so the sale of the replaced weapon and the buy share one
    // stop. A bare-handed character runs the errand alone.
    if weaponMerchant, ok := l.weaponStopMerchant(); ok {
        merchant = weaponMerchant
    }
    // The walk back target: the farm spot when the trip starts inside the
    // hunting zone, the zone center otherwise. The guide run keeps the spot the
    // death preserved (resetTownTrip).
    if !guideRun || l.inZoneSelf() {
        l.rememberFarmSpot()
    }
    l.tripStart = l.now()
    l.sold = make(map[int32]bool)
    l.rePaths = 0
    l.segmentRefused = false
    l.refusalVariants = 0
    l.tripStops = []tripStop{{
        merchant: merchant,
        sell:     true,
        buys:     nil,
        teach:    false,
        guide:    false,
    }}
    l.buysPlanned = false
    l.buyAt = time.Time{}
    l.resetBuyRequest()
    l.segmentRefused = false
    l.refusalVariants = 0
    l.resetReplacementSales()
    l.resetLearnState()
    // The learning stops plan at the sell stop, after the gear stops
    // (tickTownSell), so one visit buys everything and teaches the lessons; the
    // weapon stop still runs first, so a stuck teacher never strands a
    // bare-handed character.
    l.phase = phaseTownWalk
    reason := l.tripStartReason(shopping, learning, weaponRun, guideRun)
    // The trigger plan cache drops: the frozen trip plan owns the trip, the
    // cache only feeds the widget view between the recomputes.
    l.shoppingPlanCache = nil
    l.shoppingPlanAt = time.Time{}
    l.shoppingPlanAdena = 0
    if l.journal != nil {
        l.journal.TripStart(l.tracker.ID(), reason)
    }
    l.logf("Hunt: %s, walking to the trader %s", reason,
        merchant.Name)
    // The first stop walks the exact mesh search when the merchant is near
    // enough: it lands on the customer cell across the counter, where a wide
    // approach radius would stop outside the shop. The npc segment fallback
    // covers the foreign deck plans; a far merchant walks the npc approach.
    planned := false
    if l.merchantWithinExactRange(merchant) {
        var ringFallback bool
        planned, ringFallback = l.startWalkExactSegment(
            merchantStandPoint(merchant))
        if !planned && ringFallback {
            planned = l.startWalkNpcSegment(merchantStandPoint(merchant))
        }
    }
    if !planned {
        if !l.startWalkNpcSegment(merchantStandPoint(merchant)) {
            l.abortTownTrip("no walkable path to the shop")
        }
    }
}

// tripStartReason composes the journal and log reason of a starting trip: every
// trigger names itself, and the combined trips chain the reasons in trigger
// priority - the weapon first, then the learning and guide refills, the
// inventory and shop plan.
func (l *Loop) tripStartReason(
    shopping, learning, weaponRun, guideRun bool,
) string {
    stats := l.tracker.InventoryStats()
    reason := "inventory at " + strconv.Itoa(stats.Slots) + " slots and " +
        strconv.FormatFloat(stats.WeightPercent, 'f', 0, 64) +
        "% weight"
    if !l.inventoryFull() {
        reason = "the shop strategy plans purchases worth " +
            strconv.FormatInt(gear.AdenaSpent(l.tripPlan), 10) +
            " adena"
    }
    if weaponRun {
        // The 2 damage punches read at a glance in the log tail.
        reason = "no weapon in hand, the weapon run comes first"
    }
    if learning {
        // A learning trip names its budget, a combined one appends it.
        lessons := l.learnableLessons()
        lessonReason := strconv.Itoa(len(lessons)) + " lessons worth " +
            strconv.FormatInt(spTotal(lessons), 10) + " sp wait at " +
            "the teacher"
        if l.inventoryFull() || shopping {
            reason += ", " + lessonReason
        } else {
            reason = lessonReason
        }
    }
    if guideRun {
        // The buff refill names itself, like the weapon run above.
        guideReason := "the support magic buffs wait at the Newbie Guide"
        if l.inventoryFull() || shopping || learning || weaponRun {
            reason += ", " + guideReason
        } else {
            reason = guideReason
        }
    }
    if l.gearDebtRunWanted() {
        reason += " (the gear debt refill)"
    }

    return reason
}

// rememberFarmSpot stores the walk home target of a trip: the position at the
// start inside the hunting zone, the zone center otherwise (a village respawn).
func (l *Loop) rememberFarmSpot() {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    if l.inZoneSelf() {
        l.farmX, l.farmY, l.farmZ = selfX, selfY, selfZ

        return
    }
    zone := l.zone()
    if zone == nil {
        l.farmX, l.farmY, l.farmZ = selfX, selfY, selfZ

        return
    }
    l.farmX, l.farmY, l.farmZ = zone.CX, zone.CY, selfZ
}

func (l *Loop) tickTownTrip() {
    if l.now().Sub(l.tripStart) > tripTimeout {
        l.abortTownTrip("trip timed out")

        return
    }
    if l.interruptTripForAttacker(l.now()) {
        return
    }
    switch l.phase {
    case phaseTownWalk:
        if l.walkTownWaypoints() {
            // The completed segment hands over to the talk machinery, unless it
            // was the long haul ring approach and the merchant still sits
            // beyond the customer ring: the exact final approach arms here.
            planned, _ := l.startWalkExactSegment(l.segmentDest)
            if !l.exactApproachWanted() || !planned {
                l.enterSellPhase()
            }

            return
        }
        // The interaction handoff: the final cell verification may grind
        // against the vintage geodata mismatch while the character already
        // stands within reach (stopNpcArrivalMet).
        if l.stopNpcArrivalMet() {
            l.logf("Hunt: the merchant is within the interaction " +
                "distance, handing the walk to the talk")
            l.enterSellPhase()
        }
    case phaseTownSell:
        l.tickTownSell()
    case phaseTownReturn:
        // Entering the zone with a target in reach ends the walk.
        if l.engagesOnZoneEntry() {
            return
        }
        if l.walkTownWaypoints() {
            l.endTownTrip("back at the farm spot")
        }
    default:
        // The trip trigger starts the walk phase first, so no other phase
        // reaches this tick.
    }
}

// interruptTripForAttacker answers the aggro that reaches the character mid
// trip: walking on would drag the chase through every camp on the route. The
// trip drops instead (the soft reset arms no cooldown, the junk and proceeds
// survive) and the mob gets the hunting answer: a winnable attacker is fought,
// the rest runs the defensive flow. It reports whether the tick was consumed.
func (l *Loop) interruptTripForAttacker(now time.Time) bool {
    attacker, ok := l.tracker.NearestAttacker()
    if !ok {
        return false
    }
    l.resetTownTrip()
    if l.attackerEngageable(attacker.ObjectID) {
        l.target = attacker.ObjectID
        l.engageAt = now
        l.clearBlindRecovery()
        l.logger.Printf("Hunt: town trip interrupted: %s is on us, "+
            "fighting it", attacker.Name)

        return true
    }
    l.logger.Printf("Hunt: town trip interrupted: %s is on us and "+
        "cannot be won, switching to the defense", attacker.Name)
    l.fleeFromThreat(now)

    return true
}

// engagesOnZoneEntry ends the return walk the moment the hunting zone holds a
// valid target: entering a zone means fighting whatever the entry radius
// offers, and the walk home continues only while the surroundings stay empty.
func (l *Loop) engagesOnZoneEntry() bool {
    zone := l.zone()
    if zone == nil || !l.inZoneSelf() {
        return false
    }
    // A bare-handed character with an affordable weapon keeps walking home: the
    // fight would farm with the 2 damage fists, and the weapon run owns the
    // next ticks.
    if l.weaponlessRunWanted() {
        return false
    }
    now := l.now()
    if now.Sub(l.lastActionAt) < selectPeriod {
        return false
    }
    pick, ok := l.tracker.NearestAttackablePreferred(
        attackNearestRange, zone, l.activeSkips(now),
        l.maxTargetLevel(), true, l.zoneMobPriority,
        l.avoidBowTargets())
    if !ok {
        return false
    }
    l.endTownTrip("a target stands inside the zone")
    l.logf("Hunt: engaging %s on the zone entry", pick.Name)

    return true
}

// startReturnSegment plans the walk back to the spot the trip left (the farm
// spot, the zone center it fell back to). The segment is a fresh logical unit:
// the frozen re-path cell of the previous walk dies here, or the first refused
// click would end it. The destination z resolves onto its actual deck first (a
// remembered z of another area misses the polygon in the nearestHalfZ window
// and answers no navmesh); a lookup failure keeps the remembered z.
func (l *Loop) startReturnSegment() {
    l.phase = phaseTownReturn
    l.repathX, l.repathY = 0, 0
    l.frozenRepaths = 0
    l.skipArmed = false
    l.skipX, l.skipY = 0, 0
    destX, destY, destZ := l.farmX, l.farmY, l.farmZ
    if zone := l.zone(); zone != nil &&
        (!zone.Contains(destX, destY) || (destX == 0 && destY == 0)) {
        // The farm spot belongs to a previous square (a zone switch
        // mid trip): return to the new center instead.
        destX, destY = zone.CX, zone.CY
    }
    destZ = l.resolveDestinationDeck(destX, destY, destZ)
    dest := pathfind.Vec3{
        X: float64(destX),
        Y: float64(destY),
        Z: float64(destZ),
    }
    l.segmentRadius = tripApproachRadius
    if !l.startWalkSegment(dest) {
        l.abortTownTrip("no walkable path back to the farm spot")

        return
    }
    l.logf("Hunt: walking back to the farm spot")
}

// clearServerSelection drops a selection the loop no longer owns: the npc a
// stop or a whole trip left behind and the fight target of a dropped flight.
// The self click replaces the server side selection, so the hunting engage
// never adopts the leftover object (forced attacks on a villager only burn the
// stuck timeout).
func (l *Loop) clearServerSelection() {
    if l.tracker.SelfTargetID() == 0 {
        return
    }
    if err := l.game.ClearTarget(); err != nil {
        l.logger.Printf("Hunt: target clear failed: %v", err)
    }
}

// endTownTrip finishes the trip and arms the trigger cooldown; the frozen trip
// plan dies with it, so the next trip freezes a fresh one against the reached
// gear.
func (l *Loop) endTownTrip(reason string) {
    l.clearServerSelection()
    // The trip answer for the gear debt: whatever kept the replacements
    // from landing (a refused buy, an abort, a resumed session), the exit
    // compares the reached paperdoll against the trip start here.
    l.gearDebtCheck()
    l.phase = phaseEngage
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.waypoints = nil
    l.segmentDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentFrameOffset = 0
    l.extendArmed = false
    l.cursorEscape = zeroCursorEscape()
    l.cursorEscapes = 0
    l.repathX, l.repathY = 0, 0
    l.frozenRepaths = 0
    l.skipArmed = false
    l.skipX, l.skipY = 0, 0
    l.moveStartAt = time.Time{}
    l.forceStuck = false
    l.segmentRefusedX, l.segmentRefusedY = 0, 0
    l.tripPlan = nil
    l.tripStops = nil
    l.buysPlanned = false
    l.resetBuyRequest()
    l.shoppingPlanCache = nil
    l.shoppingPlanAt = time.Time{}
    l.shoppingPlanAdena = 0
    l.resetReplacementSales()
    l.resetLearnState()
    l.tripEndedAt = l.now()
    l.noteTripAbortRun(reason)
    if l.journal != nil {
        l.journal.TripEnd(l.tracker.ID(), reason,
            l.now().Sub(l.tripStart))
    }
    l.logf("Hunt: town trip ended: %s", reason)
}

// noteTripAbortRun maintains the consecutive abort streak of the trip
// cooldown escalation: an aborted trip grows it, every other ending
// resets it. The escalation logs once per growth past the threshold.
func (l *Loop) noteTripAbortRun(reason string) {
    if strings.HasPrefix(reason, tripAbortPrefix) {
        l.tripAbortRun++
        if l.tripAbortRun > tripAbortEscalateAfter {
            l.logf("Hunt: %d aborted trips in a row, the next "+
                "trip waits %s", l.tripAbortRun,
                tripAbortCooldown(l.tripAbortRun))
        }

        return
    }
    l.tripAbortRun = 0
}

// tripAbortPrefix marks the aborted trip endings of the streak.
const tripAbortPrefix = "aborted, "

// tripAbortCooldown renders the escalating cooldown of an abort streak: the
// base for the tolerated prefix (tripAbortEscalateAfter, 2), then a doubling
// capped at one hour.
func tripAbortCooldown(run int) time.Duration {
    cooldown := tripCooldown
    for range max(0, run-tripAbortEscalateAfter) {
        cooldown *= 2
        if cooldown >= tripAbortMaxCooldown {
            return tripAbortMaxCooldown
        }
    }

    return cooldown
}

// abortTownTrip finishes a failed trip with a log line. A deleveling through
// the shared machinery aborts itself: the plain trip end would leave the
// delevel state armed without a cooldown and restart the walk into the same
// blocker.
func (l *Loop) abortTownTrip(reason string) {
    if l.phase == phaseDelevel {
        l.abortDelevel(reason)

        return
    }
    l.endTownTrip("aborted, " + reason)
}

// resetTownTrip drops the trip state after a death: the village restart lands
// next to the shops and the cooldown clears, so a full inventory sells right
// after the revival. The gear debt check runs here too, since a slot the sell
// first step emptied stays a debt even though the trip never ended.
func (l *Loop) resetTownTrip() {
    if !l.tripActive() {
        return
    }
    l.gearDebtCheck()
    l.phase = phaseEngage
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.waypoints = nil
    l.segmentDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentFrameOffset = 0
    l.extendArmed = false
    l.cursorEscape = zeroCursorEscape()
    l.cursorEscapes = 0
    l.repathX, l.repathY = 0, 0
    l.frozenRepaths = 0
    l.skipArmed = false
    l.skipX, l.skipY = 0, 0
    l.moveStartAt = time.Time{}
    l.forceStuck = false
    l.segmentRefusedX, l.segmentRefusedY = 0, 0
    l.tripPlan = nil
    l.tripStops = nil
    l.buysPlanned = false
    l.resetBuyRequest()
    l.resetReplacementSales()
    l.resetLearnState()
    l.tripEndedAt = time.Time{}
}
