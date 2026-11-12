// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Town trips of the hunt loop: when the inventory runs full, the
// character walks to the nearest town shop over the geodata (Navigator),
// sells the junk to the merchant and walks back to the farm spot. The
// sell request uses the standard inventory sell list of the official
// client: the Mobius server prices every item itself at
// referencePrice/2 and refuses nothing else, so no shop window flow is
// needed - only the merchant interaction distance has to be respected.
package hunt

import (
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// tripActive reports whether a town trip is running.
func (l *Loop) tripActive() bool {
    switch l.phase {
    case phaseTownWalk, phaseTownSell, phaseTownReturn:
        return true
    default:
        return false
    }
}

// tripCooldownOver reports whether a new town trip may start. A
// bare-handed character with an affordable weapon and a character
// with an armed gear debt (a slot the last trip stranded) retries
// on the short gear run cooldown: punching mobs through the five
// minute cooldown of an ordinary trip is the exact outcome the
// weapon run exists to prevent, and farming without the armor the
// merchant sold is the same wound (the 2026-09-12 04:58 dump: the
// level 14 fighter farmed the Kaboo woods without its legs armor
// through the whole cooldown window).
func (l *Loop) tripCooldownOver() bool {
    if l.tripEndedAt.IsZero() {
        return true
    }
    cooldown := tripCooldown
    if l.weaponlessRunWanted() || l.gearDebtRunWanted() ||
        l.guideRunWanted() {
        // The guide run shares the short window: the support magic
        // lapses by the server push and the refill must not wait out
        // the five minute cooldown of the last ordinary trip - the
        // owner rule of farming under the buffs (the 2026-09-21
        // guide priority round). No oscillation: the landed buffs
        // keep guideWanted false for the whole 1200 seconds.
        cooldown = weaponRunCooldown
    } else if l.tripAbortRun > tripAbortEscalateAfter {
        // The abort streak escalates: a trip that keeps failing
        // the same way keeps failing it for a reason no retry
        // fixes (no walkable path, no server routing), and the
        // flat cooldown just burns the ticks between the
        // identical aborts.
        cooldown = tripAbortCooldown(l.tripAbortRun)
    }

    return time.Since(l.tripEndedAt) >= cooldown
}

// inventoryFull reports whether the inventory passed a trip trigger
// threshold: more than half of the slots used or more than half of the
// maximum weight carried. The selling phase reuses it as the stop
// condition: the trip returns once the inventory is back below it.
func (l *Loop) inventoryFull() bool {
    stats := l.tracker.InventoryStats()

    return stats.SlotPercent > tripSlotPercent ||
        stats.WeightPercent > tripWeightPercent
}

// maybeStartTownTrip begins a town trip when the inventory is full
// enough or the shop strategy has a plan worth a trip and the trip
// cooldown is over. Everything that can block the trip (no navigator,
// no geodata, no path) arms the cooldown, so a broken deployment does
// not retry every tick.
//
//nolint:cyclop,funlen // the trigger and stop freeze decision tree
func (l *Loop) maybeStartTownTrip() {
    // The first trip of a session waits for the server skill list:
    // the learning stops plan on the skill queue and the packet burst
    // of the enter world (UserInfo, ItemList, SkillList) races the
    // first hunt ticks - a trip that starts between the ItemList and
    // the SkillList silently plans without the learning (the observed
    // sessions shopped on their first walk and never carried the
    // teach stop). The wait is bounded: a server that never lists
    // skills keeps the trips selling and shopping.
    if !l.tracker.SkillsListed() &&
        time.Since(l.tracker.SessionStartedAt()) < skillListWaitLimit {
        return
    }
    // A bot outside the hunting zone returns first: the zone return
    // owns the walk until the bot is back in the zone. A town trip
    // started outside the zone (a village respawn after an emergency
    // logout) would try to walk to the village shops - where the bot
    // already stands - and then fail to return to the zone, leaving
    // the bot stuck at the village (the 2026-09-11 06:00 dump: test3
    // at 43000 50184, the learning trip started before the zone
    // return, both failed with "no dry path", the bot never moved).
    shopping := l.shoppingTripEnabled() && l.shoppingWanted()
    learning := l.learnTripWanted()
    // The weapon run outranks every other trip reason: a character
    // without any weapon shops for one at once, whatever the inventory
    // and the lesson queue say.
    weaponRun := l.weaponlessRunWanted()
    // The guide run is the buff refill: the support magic of the
    // Newbie Guide expired or never landed, and farming without the
    // buffs gives the packs the fight the character paid the town
    // visit for. The trip rides the ordinary sell stop (the junk
    // sells at the first stop, the guide stop plans behind the
    // learning ones), so a buff-only errand shares the whole trip
    // machinery.
    guideRun := l.guideRunWanted()
    if l.navigator == nil || !l.tripCooldownOver() ||
        (!l.inventoryFull() && !shopping && !learning &&
            !weaponRun && !guideRun) {
        return
    }
    // A bot outside the hunting zone returns first: the zone return
    // owns the walk until the bot is back in the zone. The weapon run
    // is the one exception - a bare-handed character shops for a
    // weapon at once, even outside the zone (punching mobs through the
    // walk home is worse than a late return). The guide run joins it:
    // the village revive of a death lands next to the guide, and the
    // trip machinery walks the buffs first and the farm spot second -
    // the plain zone return would skip both. The 2026-09-11 06:00
    // dump showed a learning trip starting at the village (outside
    // the zone) before the zone return, both searches failed with "no
    // dry path", and the bot never moved - the learning stays inside
    // the gate.
    if !weaponRun && !guideRun && l.zone() != nil && !l.inZoneSelf() {
        return
    }
    // The trip owns the way from here on: the trigger gates all
    // passed, so a merely held engage target drops with its server
    // selection (the owner rule of the travel target reset) - the
    // fight of the old task must not bind the walk that replaces it.
    l.dropAttackTarget("the town trip owns the way")
    // The walk needs a standing character: a resting one stands up
    // first and the trip starts on a later tick.
    if !l.standUpGuarded(time.Now()) {
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
    // The trip plan freezes here, ONCE: the purchases the planner
    // picked against the current gear and adena are exactly what
    // this trip sells and buys - the sell first step banks their
    // SellFirst credits, the stop planning distributes the purchases
    // and nothing re-plans in between (a re-plan at the shop ran
    // against the freed slots and the fresh adena and drifted: it
    // re-bought the piece the trip had just sold and planned
    // purchases whose displaced pieces were never queued - the
    // 2026-09-11 two pairs of gloves report).
    // The scroll of escape leg runs before the plan freeze: the
    // escape replaces the walk to the village only when the bag
    // holds a scroll and the ground pays a trusted income worth more
    // than the scroll costs - the arrival lands the character at the
    // village respawn and the walk planning below prices the short
    // residual leg from there.
    l.maybeEscapeWithSOE(merchant)
    l.tripPlan = l.shoppingPlan()
    // The gear the trip starts with is the baseline the trip exits
    // compare against: a slot the trip empties without landing the
    // replacement becomes gear debt (see gearDebtCheck).
    l.snapshotTripGear()
    // The weapon leads the trip that buys it: the sell stop routes to
    // the weapon purchase's merchant, so the sell-first of the replaced
    // weapon and the buy share ONE stop (the junk sells at any
    // merchant) and the replacement lands right after the sale instead
    // of a village walk later - every abort in between used to leave
    // the character bare-handed. A bare-handed character runs the
    // weapon errand alone: the lessons and the books wait for the next
    // trip, nothing outranks the weapon.
    if weaponMerchant, ok := l.weaponStopMerchant(); ok {
        merchant = weaponMerchant
    }
    // The walk back target: the farm spot when the trip starts inside
    // the hunting zone, the zone center otherwise (a village respawn,
    // a chase that ran away). The guide run keeps the remembered spot
    // when it starts outside the zone: the death that armed the run
    // never touched it (resetTownTrip keeps the farm spot) and the
    // revive point sits next to the guide anyway - overwriting the
    // precise return the death preserved with the zone center would
    // move the resumed farming ground.
    if !guideRun || l.inZoneSelf() {
        l.rememberFarmSpot()
    }
    l.tripStart = time.Now()
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
    // The learning stops no longer ride the trip start: they plan
    // AFTER the gear stops at the sell stop (see tickTownSell), so
    // one town visit buys the weapon, the armor, the jewels, the
    // books and teaches the lessons - the user rule of the one
    // town visit. The weapon stop still runs FIRST (the sell stop
    // routes to the weapon merchant), so a stuck teacher segment can
    // no longer strand a bare-handed character: the weapon is
    // bought and worn before the teacher segment ever runs.
    l.phase = phaseTownWalk
    reason := l.tripStartReason(shopping, learning, weaponRun, guideRun)
    // The trigger plan cache drops: the frozen trip plan owns the
    // trip now, the cache only feeds the widget view between the
    // recomputes.
    l.shoppingPlanCache = nil
    l.shoppingPlanAt = time.Time{}
    l.shoppingPlanAdena = 0
    if l.journal != nil {
        l.journal.TripStart(l.tracker.ID(), reason)
    }
    l.logf("Hunt: %s, walking to the trader %s", reason,
        merchant.Name)
    // The first stop walks the exact mesh search when the merchant is
    // near enough (the same authority the later stops of
    // advanceTripStop plan with): the approach radius ends the plan on
    // the first deck polygon inside its radius - outside the shop, on
    // the outer railing side - and the talk fires from there (or
    // slides along the railing forever). The exact search lands on
    // the customer cell across the counter; the npc segment fallback
    // runs for the one failure class the exact search cannot answer -
    // the plan that resolved onto a foreign deck (the roof over the
    // shop), see startWalkExactSegment. The far merchants walk the npc
    // segment (the npcApproachRadius long haul): the plan ends at the
    // stand point - the requested cell inside the shop - instead of
    // the shop edge a wide ring caught.
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

// tripStartReason composes the journal and log reason of a starting
// town trip: every trigger names itself, the combined trips chain the
// reasons (the log tail reads at a glance in the state dumps). The
// order is the priority order of the triggers: the weapon first, the
// learning and the guide refill behind, the inventory and the shop
// plan as the baseline.
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
        // The bare-handed errand names itself: the 2 damage punches of
        // the dump report read at a glance in the log tail.
        reason = "no weapon in hand, the weapon run comes first"
    }
    if learning {
        // The learning contributes its lesson budget to the reason:
        // a learning-only trip names it, a combined one appends it.
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
        // The buff refill names itself: the support magic trip reads
        // at a glance in the log tail, the same convention as the
        // weapon run above.
        guideReason := "the support magic buffs wait at the Newbie Guide"
        if l.inventoryFull() || shopping || learning || weaponRun {
            reason += ", " + guideReason
        } else {
            reason = guideReason
        }
    }
    if l.gearDebtRunWanted() {
        // The refill names itself: the stranded slot of the dump
        // report reads at a glance in the log tail.
        reason += " (the gear debt refill)"
    }

    return reason
}

// rememberFarmSpot stores the walk home target of a trip: the position
// of the character when it starts inside the hunting zone, the zone
// center otherwise (a village respawn, a chase that ran away).
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

// tickTownTrip advances the running town trip by one decision.
func (l *Loop) tickTownTrip() {
    if time.Since(l.tripStart) > tripTimeout {
        l.abortTownTrip("trip timed out")

        return
    }
    if l.interruptTripForAttacker(time.Now()) {
        return
    }
    switch l.phase {
    case phaseTownWalk:
        if l.walkTownWaypoints() {
            // The completed segment hands over to the talk machinery -
            // unless the segment was the long haul ring approach and the
            // merchant is still beyond the customer ring: the exact
            // final approach segment arms here (near enough now for the
            // bounded search) and walks the character to the
            // customer cell across the counter (the shop quarter
            // round of the user report: the talk fired - or bounced -
            // from the outer railing side instead).
            planned, _ := l.startWalkExactSegment(l.segmentDest)
            if !l.exactApproachWanted() || !planned {
                l.enterSellPhase()
            }

            return
        }
        // The interaction handoff of the merchant stops: the plan's
        // final cell verification may grind against the vintage
        // geodata disagreement while the character already stands
        // within reach of the merchant it walked to (see
        // stopNpcArrivalMet) - the talk machinery owns the rest.
        if l.stopNpcArrivalMet() {
            l.logf("Hunt: the merchant is within the interaction " +
                "distance, handing the walk to the talk")
            l.enterSellPhase()
        }
    case phaseTownSell:
        l.tickTownSell()
    case phaseTownReturn:
        // Entering the zone with a target in reach ends the walk:
        // the hunt answers whatever the entry radius offers
        // instead of marching to the center first.
        if l.engagesOnZoneEntry() {
            return
        }
        if l.walkTownWaypoints() {
            l.endTownTrip("back at the farm spot")
        }
    default:
        // The non-town phases never reach the town tick (the trip
        // trigger starts the walk phase first).
    }
}

// interruptTripForAttacker answers the aggro that reaches the
// character mid trip: a mob holds it as the target (the blows of a
// social pull, an aggressive camp the steering could not dodge) and
// walking on only drags the chase through every camp on the route -
// the pile up the emergency logout then answers too late. The trip
// drops instead (the soft reset: no cooldown, the next tick re-arms
// the walk from wherever the answer leaves the character - the junk,
// the books and the sold proceeds all survive) and the mob gets the
// same aggro answer the hunting engage gives: a healthy character
// with a winnable attacker fights it at once, everything else keeps
// the defensive flow (the standard escape walk, the logout when the
// chase never shakes). It reports whether the tick was consumed by
// the answer.
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

// engagesOnZoneEntry ends the return walk the moment the hunting
// zone holds a valid target: entering a zone means fighting
// whatever the entry radius offers, the walk to the farm spot or
// the zone center only continues while the surroundings stay
// empty (the level slack and the social fence of the constrained
// search apply here too). The next engage tick picks the target
// the search found.
func (l *Loop) engagesOnZoneEntry() bool {
    zone := l.zone()
    if zone == nil || !l.inZoneSelf() {
        return false
    }
    // A bare-handed character with an affordable weapon keeps walking
    // home: the zone entry fight would farm with the fists, and the
    // weapon run owns the next ticks anyway (the trip end arms the
    // short weapon run cooldown).
    if l.weaponlessRunWanted() {
        return false
    }
    now := time.Now()
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

// startReturnSegment plans the walk back to the farm spot. The segment is a
// fresh logical unit of the trip machinery (the sell stop handed the
// walk over after the shopping, the deleveling aborted into it), so
// the frozen re-path cell of whatever walk came before dies here: the
// return segment must not inherit the frozen budget of the guard walk or
// the sell approach, or its own first refused click ends it instantly
// - the delevel abort and the return segment of the 2026-09-12 01:50 dump
// died back to back from the same cell in one second, leaving the
// character to the direct zone segments and the permanent freeze.
// startReturnSegment plans the walk back to the spot the trip left
// (the farm spot, the zone center it fell back to). The destination z
// resolves onto the deck the destination actually sits on before the
// search: the remembered spot z is the character's own standing z of
// another area, and a plan remembered on the village deck rides a
// zone center x/y whose deck lies hundreds of units lower - the mesh
// search binds the destination polygon inside the nearest window of
// the destination z (query.go nearestHalfZ), misses it by hundreds of
// units and answers the honest "no navmesh under the position"
// forever (the farm readiness round of 2026-09-20: every return of
// the trip aborted on exactly that error while the same query with
// the resolved deck answered 27 waypoints). The resolution mirrors
// the zone return goal (zoneReturnDestination); a lookup failure
// keeps the remembered z - the same-deck case it answers correctly.
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

// clearServerSelection drops a selection the loop no longer owns: the
// npc a stop or a whole trip left behind (the merchant select, the
// teacher talk click) and the fight target of a flight the loop just
// dropped. The self click of the clear replaces the server side
// selection, so the hunting engage that follows never adopts the
// leftover object as its target (the forced attacks on a villager
// only burn the stuck timeout, and the abandoned fight target kept
// the fight state reporting it - the run-31 acceptance sampled the
// dropped target at 868 units while the character ran a healthy
// flight and failed the bow band). The call is a no-op without a
// selection; the client skips a selection the clear itself left
// (the own object id).
func (l *Loop) clearServerSelection() {
    if l.tracker.SelfTargetID() == 0 {
        return
    }
    if err := l.game.ClearTarget(); err != nil {
        l.logger.Printf("Hunt: target clear failed: %v", err)
    }
}

// endTownTrip finishes the trip and arms the trigger cooldown. The
// frozen trip plan dies with it: the next trip freezes a fresh one
// against the gear the purchases reached.
func (l *Loop) endTownTrip(reason string) {
    l.clearServerSelection()
    // The trip answer for the gear debt: whatever kept the trip
    // from landing the replacements (a refused buy, an abort, a
    // session death the relogin resumed), the exits compare the
    // reached paperdoll against the trip start here.
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
    l.tripEndedAt = time.Now()
    l.noteTripAbortRun(reason)
    if l.journal != nil {
        l.journal.TripEnd(l.tracker.ID(), reason,
            time.Since(l.tripStart))
    }
    l.logf("Hunt: town trip ended: %s", reason)
}

// noteTripAbortRun maintains the consecutive abort streak of the trip
// cooldown escalation: an aborted trip grows it, every other ending
// (a completed sell, a farm spot return, a death handover) resets it.
// The escalation log lands once per growth past the threshold so the
// operator sees the cooldown change.
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

// tripAbortCooldown renders the escalating cooldown of an abort
// streak: the base cooldown for the tolerated prefix, then a doubling
// per abort capped at the hour.
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

// abortTownTrip finishes a failed trip with a log line. A deleveling
// walking through the shared machinery aborts the deleveling itself:
// the plain trip end would leave the delevel state armed without a
// cooldown, and the next tick restarted the walk into the same
// blocker - the reported bot hung cycling "deleveling to 9" and
// "the walk would cross water" forever (the 2026-09-10 state dump).
func (l *Loop) abortTownTrip(reason string) {
    if l.phase == phaseDelevel {
        l.abortDelevel(reason)

        return
    }
    l.endTownTrip("aborted, " + reason)
}

// resetTownTrip drops the trip state after a death. The village
// restart lands next to the shops, and the cooldown of the trip the
// death interrupted is cleared as well, so a full inventory sells
// right after the revival instead of farming with the junk first.
// The gear debt check runs here too: the interrupt that drops the
// trip (an attacker mid trip) leaves the sold pieces sold - a slot
// the sell first step already emptied stays a debt even though the
// trip never reached its end.
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
