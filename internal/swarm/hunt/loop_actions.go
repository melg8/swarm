// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The between-fights phases of the hunt loop, split out of the
// loop.go god file: resting to full health (with the stand guard the
// movement layers call first), looting the corpses and the inventory
// overflow cleanup.

import (
    "math"
    "time"
)

// The loot timing constants of the ranged-kill grace.
const (
    // lootKillGrace bounds how long the loot phase may hold after a
    // kill whose corpse sits away from the character (the bow lure,
    // the caster spells): the drops land at the corpse, their
    // broadcast can trail the death packet, and the walk there costs
    // seconds - the grace covers the approach and the broadcast lag
    // instead of leaving the loot on the ground.
    lootKillGrace = 15 * time.Second
    // lootKillArriveGrace is the short hold AFTER the character
    // reached the corpse: the drops that broadcast a moment behind
    // the death packet still land within it, then the hunt resumes -
    // a mob that dropped nothing never stalls the phase longer.
    lootKillArriveGrace = 2 * time.Second
    // lootKillApproachRadius is the distance that separates a melee
    // kill from a ranged one: a corpse within it died at the
    // character's feet (the melee range plus the scatter), a corpse
    // beyond it died to a ranged shot or a spell - the character
    // walks there and loots it instead of leaving the drops behind.
    lootKillApproachRadius = 300.0
    // lootKillSearchSlack widens the item search past the corpse
    // distance: the drop broadcast lands the items at the corpse
    // plus the scatter.
    lootKillSearchSlack = 300.0
)

// restSittingHeld reports whether the resting logic still holds a
// sitting character down: the health below the stand threshold, or a
// caster whose mana has not recovered to the stand one yet.
func (l *Loop) restSittingHeld(hp float64) bool {
    if !l.tracker.SelfSitting() {
        return false
    }

    return hp < standUpHealthPercent || l.mageManaLow()
}

// rest brings the resting character back to full health (and a
// caster back to full mana). Sitting accelerates the regeneration,
// so the character sits down below the sit threshold and stands up
// again once recovered. The mana of a mystic rides the same toggles:
// the dry caster sits down under the mana sit threshold and keeps
// sitting until the mana recovered to the stand one (see
// combat_skills.go), a fighter ignores the mana gates entirely. A
// character that knows an instant self heal casts it instead of the
// sit down while the mana pays and no blow is landing (see
// recovery_heal.go) - standing up first when the rest already sat
// it down. The sit/stand action is a server side toggle, so every
// transition is confirmed by the ChangeWaitType broadcast before
// the opposite one is ever sent.
func (l *Loop) rest() {
    now := time.Now()
    if now.Sub(l.lastHit) < selectPeriod {
        return
    }
    l.lastHit = now
    hp := l.tracker.SelfHealthPercent()
    wantSit := false
    standToHeal := false
    switch {
    case l.restSelfHeal(now, hp):
        // The ready self heal replaced the sit down this window.
        return
    case l.restStandToHeal(now, hp):
        // Sitting and the heal is ready: stand up to cast, the
        // transition below runs with the heal reason, the cast
        // itself fires on the next window.
        standToHeal = true
    case l.restSittingHeld(hp):
        // The sit is confirmed and the regeneration is running -
        // the health or the mana of the caster still holds it.
        return
    case l.tracker.SelfSitting():
        // Sitting and recovered: stand up (wantSit stays false).
    case hp < sitDownHealthPercent:
        wantSit = true
    case l.mageManaDry():
        // The caster ran its mana dry: sitting regenerates several
        // times faster than standing, the fights resume at the stand
        // threshold.
        wantSit = true
    default:
        l.logf("Hunt: resting, HP %.0f%% below %.0f%%",
            hp, reengageHealthPercent)

        return
    }
    if wantSit && l.healInFlight(now) {
        // The heal cast is still in flight: the server refuses the
        // sit of a casting character without breaking the cast, so
        // the landing flips the bar and the next window re-decides.
        return
    }
    l.restToggleTo(now, wantSit, standToHeal, hp)
}

// restToggleTo sends one sit or stand transition of the rest logic
// through its confirmation guard: the previous transition must be
// confirmed (or have timed out) before the opposite one goes out,
// so a slow ChangeWaitType broadcast can never double toggle. The
// standToHeal flag only names the stand reason in the log.
func (l *Loop) restToggleTo(
    now time.Time, wantSit, standToHeal bool, hp float64,
) {
    if l.tracker.SelfSitting() == wantSit {
        return
    }
    if !l.restActionAt.IsZero() {
        if l.restActionSit == l.tracker.SelfSitting() {
            // The previous transition is confirmed, consume it.
            l.restActionAt = time.Time{}
        } else if now.Sub(l.restActionAt) < restRetryPeriod {
            // Confirmation still pending, never double toggle.
            return
        }
    }
    switch {
    case wantSit && hp < sitDownHealthPercent:
        l.logf("Hunt: HP %.0f%% below %.0f%%, sitting down to regenerate",
            hp, sitDownHealthPercent)
    case wantSit:
        l.logf("Hunt: mana %.0f%% below %.0f%%, sitting down to regenerate",
            l.tracker.SelfManaPercent(), manaSitPercent)
    case standToHeal:
        l.logf("Hunt: standing up to cast the recovery heal")
    default:
        l.logf("Hunt: HP %.0f%% recovered, standing up", hp)
    }
    if err := l.game.ActionSitStand(); err != nil {
        l.logf("Hunt: sit/stand action failed: %v", err)

        return
    }
    l.restActionAt = now
    l.restActionSit = wantSit
}

// standUpGuarded stands a sitting character up before an action the
// server refuses while it sits: the trip walks, the zone returns and
// the escape runs of the combat safety all move the character, and a
// walk started sitting would stall into the stuck re-paths. The
// toggle shares the pending transition gate with the rest logic, so
// the two never double toggle each other, and the walk starts on a
// later tick once the ChangeWaitType broadcast confirms the standing.
// The confirmation broadcast itself is only the start of the stand:
// the server holds the character paralyzed on the REST intention for
// a fixed animation window after it (the 2.5 s StandUpTask), and
// every move request of that window bounces off ActionFailed - the
// guard holds the caller through the settle window so the first walk
// request lands on a movable character.
func (l *Loop) standUpGuarded(now time.Time) bool {
    if !l.tracker.SelfSitting() {
        // Standing already: a stand transition of this guard went
        // out recently - hold the caller through the server side
        // stand window, then consume the transition so it never
        // lingers into the rest logic.
        if !l.restActionAt.IsZero() && !l.restActionSit {
            if now.Sub(l.restActionAt) < standSettlePeriod {
                return false
            }
            l.restActionAt = time.Time{}
        }

        return true
    }
    // Sitting: a transition is in flight (the rest sit request or this
    // guard's stand) - wait out its confirmation window before the
    // stand request, never double toggle.
    if !l.restActionAt.IsZero() &&
        now.Sub(l.restActionAt) < restRetryPeriod {
        return false
    }
    if err := l.game.ActionSitStand(); err != nil {
        l.logf("Hunt: stand up failed: %v", err)

        return false
    }
    l.restActionAt = now
    l.restActionSit = false

    return false
}

// maybeDrinkFightPotion drinks a healing potion in the running fight
// when the health fell under the fight threshold: the tanked pile up
// grinds the bar down faster than the natural regeneration, and the
// potion is the difference between finishing the first attacker and
// the emergency logout (the owner recipe: the healers keep the
// first kill alive). The reuse window is shared with the quest trip
// potion pacing (questPotionAt) - the server paces ONE potion reuse
// window across every source, so the engine mirrors it.
func (l *Loop) maybeDrinkFightPotion(now time.Time) {
    if l.tracker.SelfHealthPercent() >= fightPotionHealthPercent {
        return
    }
    reuseHeld := !l.questPotionAt.IsZero() &&
        now.Sub(l.questPotionAt) < questPotionReuse
    if reuseHeld {
        return
    }
    for _, item := range l.tracker.InventoryItems() {
        if item.ItemID != questPotionItemID || item.Count <= 0 ||
            item.Equipped {
            continue
        }
        if err := l.game.UseItem(item.ObjectID); err != nil {
            l.logf("Hunt: healing potion use failed: %v", err)
            l.questPotionAt = now

            return
        }
        l.questPotionAt = now
        l.logf("Hunt: HP %.0f%% in the fight, drinking a healing potion",
            l.tracker.SelfHealthPercent())

        return
    }
}

// noteKillPosition records where the killed mob died: the corpse
// position of the target (the character position when the corpse
// already vanished from the knownlist - it died at the feet). The
// loot phase uses it for the ranged-kill grace walk: the drops land
// at the corpse, and a mob shot down from a distance is approached
// and looted, never left behind.
func (l *Loop) noteKillPosition(objectID int32, now time.Time) {
    l.killPosKnown = false
    l.killWalked = false
    l.killLooted = false
    l.killAt = now
    if x, y, z, ok := l.tracker.ObjectPosition(objectID); ok {
        l.killX, l.killY, l.killZ = x, y, z
        l.killPosKnown = true

        return
    }
    if x, y, z, ok := l.tracker.SelfPosition(); ok {
        l.killX, l.killY, l.killZ = x, y, z
        l.killPosKnown = true
    }
}

// killApproachWalk walks the looter to the corpse of a kill that
// happened at range (the bow lure, the caster spell) and holds the
// loot phase until the drops had their chance to land: the drops
// spawn at the corpse and their broadcast can trail the death
// packet, so the character walks there and the per tick item scan
// picks up anything that appears on the way. A melee kill (the
// corpse within lootKillApproachRadius) never holds the phase - the
// ordinary scan radius already covers its drops. Reports whether
// the loot phase stays held; false hands the phase back to the
// engage (nothing more to loot).
func (l *Loop) killApproachWalk() bool {
    if !l.killPosKnown || l.killAt.IsZero() || l.killLooted {
        return false
    }
    now := time.Now()
    if now.Sub(l.killAt) > lootKillGrace {
        // The grace expired: whatever dropped is on the ground
        // within the scan radius or never dropped at all.
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        // The position is unknown for a moment: hold the phase, the
        // next tick retries.
        return true
    }
    dist := math.Hypot(float64(l.killX-selfX), float64(l.killY-selfY))
    if dist <= lootKillApproachRadius {
        // A melee kill or the walk already arrived: the scan above
        // covered the corpse radius, the drops either landed within
        // it or never dropped. The arrival grace only holds for the
        // approached corpses (the broadcast can trail the walk).
        if !l.killWalked {
            return false
        }

        return now.Sub(l.killAt) < lootKillArriveGrace
    }
    if now.Sub(l.lootMoveAt) < selectPeriod {
        return true
    }
    l.killWalked = true
    l.lootMoveAt = now
    moveX, moveY, _ := walkClickSegment(selfX, selfY, selfZ,
        float64(l.killX), float64(l.killY), float64(selfZ),
        returnWalkSegment)
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logf("Hunt: corpse approach walk failed: %v", err)
    }

    return true
}

// loot picks up the ground items around the character until none is left
// within the loot radius, then hunts the next target. Farther items are
// approached with an explicit walk first so the character visibly runs
// toward the loot instead of trusting the click to start the whole
// approach. The search carries no zone filter: the kill that produced
// the drop often happens past the square line (the chase, the scatter
// of the drop), and a drop left on the ground because a line on the
// map crossed it is a pure loss - anything within the loot radius of
// the character is picked up, wherever it lies. The radius extends to
// the corpse of the last kill (a mob shot down from a distance drops
// at its corpse), and the corpse approach walk of the ranged kills
// holds the phase until the drops land instead of leaving them behind.
// A living attacker outranks the drops: the tick hands to the engage
// (see lootAnswerAttacker) and the loot resumes after the fight.
func (l *Loop) loot() {
    // The potion serves the loot too: the kill that emptied the
    // health bar (the tanked pair, the hard single) leaves the
    // character picking up drops at a bar the survivor's next swing
    // can empty - drink before the walk to the corpse.
    now := time.Now()
    l.maybeDrinkFightPotion(now)
    if l.lootAnswerAttacker(now) {
        return
    }
    radius := lootRadius
    if l.killPosKnown {
        if selfX, selfY, _, ok := l.tracker.SelfPosition(); ok {
            dist := math.Hypot(
                float64(l.killX-selfX), float64(l.killY-selfY))
            if reach := dist + lootKillSearchSlack; reach > radius {
                radius = reach
            }
        }
    }
    item, ok := l.tracker.NearestGroundItemExcluding(radius, l.skipped, nil)
    if ok {
        // A drop of the kill is in sight: the ordinary pickup flow
        // owns the phase (the approach walk to it, the pickup
        // request), the corpse grace stands down - the drops ARE
        // being looted.
        l.killLooted = true
    }
    if !ok {
        if l.killApproachWalk() {
            return
        }
        l.phase = phaseEngage
        l.target = 0

        return
    }
    if item.ObjectID != l.lootID {
        l.lootID = item.ObjectID
        l.lootAt = now
        l.lootMoveAt = time.Time{}
    }
    if now.Sub(l.lootAt) > pickupTimeout {
        // The pickup did not finish: the item is protected or
        // unreachable. Skip it for a while and try the next one.
        l.skipped[item.ObjectID] = now.Add(pickupRetryDelay)
        l.lootID = 0
        l.logf("Hunt: pickup of %d timed out, skipping", item.ObjectID)

        return
    }
    if now.Sub(l.lootMoveAt) < selectPeriod {
        return
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    dist := math.Hypot(float64(item.X-selfX), float64(item.Y-selfY))
    l.lootMoveAt = now
    if dist > lootApproachRadius {
        if err := l.game.WalkTo(item.X, item.Y, item.Z); err != nil {
            l.logf("Hunt: walk to loot failed: %v", err)
        }

        return
    }
    if err := l.game.PickupItem(item); err != nil {
        l.logf("Hunt: pickup failed: %v", err)
    }
}

// lootAnswerAttacker hands the loot tick to the engage when a living
// mob holds the character as its target: a mob that reached the
// character while it picks up the kill must be answered before
// another corpse step - standing in the blows for a drop is the
// reported peaceful pick death. The handoff is the engage's own aggro
// answer: the winnable attacker becomes the target (the loot phase
// resumes after the fight, the drops stay on the ground), the rest
// keep the defense flow. It reports whether the tick was consumed.
func (l *Loop) lootAnswerAttacker(now time.Time) bool {
    attacker, ok := l.tracker.NearestAttacker()
    if !ok {
        return false
    }
    l.phase = phaseEngage
    l.lootID = 0
    if !l.attackerEngageable(attacker.ObjectID) {
        l.logf("Hunt: %s interrupts the loot and cannot be won, "+
            "switching to the defense", attacker.Name)
        l.target = 0
        l.engageAt = time.Time{}
        l.fleeFromThreat(now)

        return true
    }
    l.logf("Hunt: %s interrupts the loot, answering it first",
        attacker.Name)
    l.target = attacker.ObjectID
    l.engageAt = now
    l.clearBlindRecovery()

    return true
}

// cleanupInventory destroys junk items when the slots or the weight of
// the character approach the server limits. The planned equips of the
// auto equipment stay out of the destroy candidates: a looted upgrade
// waiting for its paced use item request is never destroyed for bag
// space - the bot wears it instead.
func (l *Loop) cleanupInventory() {
    stats := l.tracker.InventoryStats()
    if stats.SlotPercent < cleanupSlotPercent &&
        stats.WeightPercent < cleanupWeightPercent {
        return
    }
    junk := l.tracker.DestroyableItemsExcluding(
        l.plannedEquipKeeps(), destroyBatch)
    if len(junk) == 0 {
        return
    }
    l.logf("Hunt: inventory at %d slots and %.0f%% weight, "+
        "destroying %d items", stats.Slots, stats.WeightPercent, len(junk))
    for _, item := range junk {
        err := l.game.DestroyItem(item.ObjectID, item.Count)
        if err != nil {
            l.logf("Hunt: destroy failed: %v", err)

            return
        }
    }
}
