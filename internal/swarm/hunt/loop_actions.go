// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

// The between-fights phases of the hunt loop: resting to full health (with the
// stand guard the movement layers call first), looting and the inventory
// overflow cleanup.

import (
    "math"
    "time"
)

// The loot timing constants of the ranged-kill grace.
const (
    // lootKillGrace bounds the hold of the loot phase after a ranged kill:
    // the drops land at the corpse, their broadcast can trail the death
    // packet, and the walk there costs seconds.
    lootKillGrace = 15 * time.Second
    // lootKillArriveGrace is the short hold after the character reached the
    // corpse, so a late drop broadcast still lands.
    lootKillArriveGrace = 2 * time.Second
    // lootKillApproachRadius separates a melee kill from a ranged one: a
    // corpse within it died at the character's feet, a farther one died to a
    // shot or a spell and is walked to.
    lootKillApproachRadius = 300.0
    // lootKillSearchSlack widens the item search past the corpse distance:
    // the drops land at the corpse plus the scatter.
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

// rest brings the resting character back to full health (a caster to full
// mana). A mystic sits under the mana threshold and stands at the stand one
// (see combat_skills.go), a fighter ignores the mana gates, and an instant
// self heal replaces the sit while the mana pays and no blow lands (see
// recovery_heal.go). The sit/stand toggle is server side, so each transition
// waits for the ChangeWaitType broadcast before the opposite one.
func (l *Loop) rest() {
    now := l.now()
    if now.Sub(l.lastActionAt) < selectPeriod {
        return
    }
    l.lastActionAt = now
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

// restToggleTo sends one sit or stand transition through its confirmation
// guard: the previous one must be confirmed or timed out, so a slow
// ChangeWaitType broadcast can never double toggle.
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

// standUpGuarded stands a sitting character up before an action the server
// refuses while it sits (the trip walks, the zone returns and the escapes): a
// walk started sitting stalls into the stuck re-paths. The toggle shares the
// pending transition gate with the rest logic, and the caller is held through
// the 2.5 s StandUpTask window, where every move bounces off ActionFailed.
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

// maybeDrinkFightPotion drinks a healing potion when the health fell under the
// fight threshold: the tanked pile up grinds the bar faster than the natural
// regeneration. The server paces one potion reuse window across every source,
// so the window is shared with the quest trip (questPotionAt).
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

// noteKillPosition records where the mob died: the corpse position, or the
// character position when the corpse already left the knownlist. The loot
// phase uses it for the ranged-kill grace walk.
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

// killApproachWalk walks the looter to a corpse killed at range and holds the
// loot phase until the drops land: they spawn at the corpse and their
// broadcast can trail the death packet. A melee kill never holds the phase.
// Reports whether the phase stays held.
func (l *Loop) killApproachWalk() bool {
    if !l.killPosKnown || l.killAt.IsZero() || l.killLooted {
        return false
    }
    now := l.now()
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

// loot picks up the ground items within the loot radius, walking to the far
// ones first, then hunts the next target. The search carries no zone filter:
// the kill often happens past the square line, and a drop left because a line
// on the map crossed it is a pure loss. The radius extends to the last kill,
// whose corpse approach walk holds the phase until the drops land. A living
// attacker outranks the drops (see lootAnswerAttacker).
func (l *Loop) loot() {
    // The kill may have emptied the bar: drink before the corpse walk, while
    // the survivor's next swing can still land.
    now := l.now()
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
    item, ok := l.tracker.NearestGroundItemExcluding(radius, l.itemSkip, nil)
    if ok {
        // A drop is in sight: the pickup flow owns the phase, so the corpse
        // grace stands down.
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
        l.itemSkip[item.ObjectID] = now.Add(pickupRetryDelay)
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

// lootAnswerAttacker hands the loot tick to the engage when a living mob holds
// the character: a mob reached while it picks up a kill must be answered
// before another corpse step. A winnable attacker becomes the target, the rest
// take the defense flow. Reports whether the tick was consumed.
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

// cleanupInventory destroys junk items when the slots or the weight approach
// the server limits. The planned equips stay out: a looted upgrade waiting for
// its paced use request is worn, never destroyed for bag space.
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
