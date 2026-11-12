// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"
)

// engage attacks the nearest attackable npc while the current target
// lives, then switches to the loot phase. The Mobius AttackRequest has
// double click semantics: the first request selects the target (the
// MyTargetSelected answer), the repeated request for the already
// selected target triggers the forced attack. The loop therefore keeps
// re-requesting the target until the character is actually engaged in
// the fight, which the MoveToPawn/Attack/AutoAttackStart broadcasts
// confirm. The gate order below is the behavior: the ground gate, the
// mana rest, the server target adoption, the safety verdicts, the pick,
// the kite machinery and the attack request.
func (l *Loop) engage() {
    now := time.Now()
    l.syncLure()
    if l.guardGround(now) {
        return
    }
    manaHeld := l.restOnLowMana()
    if l.adoptServerTarget(now, manaHeld) {
        return
    }
    if l.answerApproachAggro(now) {
        return
    }
    if l.switchStaleEngage(now) {
        return
    }
    if l.respondLosingFight(now) {
        return
    }
    if l.answerBlindEngage(now) {
        return
    }
    if l.settleAfterLogin(now) {
        return
    }
    if l.target == 0 && l.pickTarget(now, manaHeld) {
        return
    }
    if l.engageKiteTick(now) {
        return
    }
    if l.fightTick(now) {
        return
    }
    l.requestAttack(now)
}

// syncLure stands the stale lure state down: a pick changed under the
// lure (a kill, a skip, a flee drop) restores the melee weapon.
func (l *Loop) syncLure() {
    if l.lureArmed() && (l.lure.target != l.target || l.phase != phaseEngage) {
        l.clearLure()
    }
}

// guardGround answers the hunting ground gate: a character outside its
// ground with nothing pickable in sight walks back instead of hunting.
// A hurt character under attack flees even outside the ground, a fight
// that crossed the ground line is finished where it stands (the mob
// that dragged the character out dies before the walk home), and a
// pickable enemy visible from the current position keeps the hunt
// running wherever it stands. It reports true when the tick was
// consumed.
func (l *Loop) guardGround(now time.Time) bool {
    if !l.onHeldGround() {
        if l.tracker.SelfUnderAttack() &&
            l.tracker.SelfHealthPercent() < reengageHealthPercent {
            l.fleeFromThreat(now)

            return true
        }
        if !l.adoptOutZoneFight(now) && !l.cellEnemiesVisible(now) {
            if l.weaponlessRunWanted() {
                // The bare-handed errand outranks the walk home: the
                // return walk through the aggressive packs never
                // arrives unarmed, and the panic logout only saves the
                // character on the ground it flees. The trip machinery
                // starts the weapon run on the next tick and its return
                // segment walks home armed; a wallet that cannot afford
                // any weapon keeps the ordinary return.
                if !l.zoneReturn {
                    l.zoneReturn = true
                    l.logger.Printf("Hunt: no weapon in hand, the " +
                        "weapon run outranks the walk home")
                }

                return true
            }
            l.returnToZone()

            return true
        }
    }
    if l.zoneReturn {
        l.zoneReturn = false
        l.zoneFails = 0
        l.roadFights = 0
        l.logf("Hunt: back in the hunting zone, resuming the hunt")
    }

    return false
}

// restOnLowMana drops the running fight of a caster whose mana sits
// under the re-engage threshold and reports the held rest: the dry one
// owns its fights until the bar recovers. Under attack the hold lifts,
// so the caster answers the blows instead of sitting into them (see
// combat_skills.go).
func (l *Loop) restOnLowMana() bool {
    manaHeld := l.mageManaLow() && !l.tracker.SelfUnderAttack()
    if manaHeld && l.target != 0 {
        l.dropFightForMana()
    }

    return manaHeld
}

// adoptServerTarget prefers the server view of the target while it
// lives: the MyTargetSelected answer arrives asynchronously, so the
// fresh value is read every tick. A stale id is not re-adopted - the
// server never clears the selection of a corpse, only the next
// selection replaces it, and blindly trusting it locked the loop into
// an engage/loot ping-pong. The adoption requires a living attackable
// npc (a lingering villager selection from a trip end is refused) and
// a mana held caster re-adopts nothing. A dead current target switches
// the loop into looting and reports the consumed tick.
func (l *Loop) adoptServerTarget(now time.Time, manaHeld bool) bool {
    serverTarget := l.tracker.SelfTargetID()
    if serverTarget != 0 && !manaHeld &&
        l.tracker.ObjectAlive(serverTarget) &&
        l.tracker.ObjectAttackable(serverTarget) &&
        !l.targetSkipped(serverTarget, now) {
        l.target = serverTarget
    }
    if l.target != 0 && !l.tracker.ObjectAlive(l.target) {
        if l.cell != nil {
            l.cellNoteKill(l.target, now)
        }
        l.noteKillPosition(l.target, now)
        l.journalKill(l.target, now)
        l.resetFightClock()
        l.logf("Hunt: target %d died, looting", l.target)
        l.target = 0
        l.clearBlindRecovery()
        l.phase = phaseLoot
        l.lootID = 0

        return true
    }

    return false
}

// answerApproachAggro answers a mob that aggros while the character
// walks to a pick: the walk would drag the chase into a second opponent
// while the swings keep landing unanswered. It runs ahead of the stuck
// timeout, the losing-fight verdict and the blind recovery. A live kite
// fight keeps its pick, an attacker the kite can answer joins the train
// the straight retreat already handles, an attacker above the level
// ceiling of a ground fight hands the tick to the flee, and everything
// else is adopted as the target. It reports true when the tick was
// consumed.
func (l *Loop) answerApproachAggro(now time.Time) bool {
    if l.target == 0 || l.tracker.SelfFighting(l.target) {
        return false
    }
    attacker, ok := l.tracker.NearestAttacker()
    if !ok || attacker.ObjectID == l.target {
        return false
    }
    switch {
    case l.kiteOwnsRetreat() && l.tracker.SelfEngagedBy(l.target):
        // The live kite fight keeps its pick: every fresh add joins the
        // train the straight retreat direction already answers, and
        // switching the arrows spread the damage across the pack while
        // every mob regenerated it back.
    case !l.attackerEngageable(attacker.ObjectID) && l.kiteOwnsRetreat():
        // The attacker outranks the character and the archer kite owns
        // the retreat: the straight lane answers. The melee flee fan is
        // clamped to the current zone box and logged the session out
        // mid-kite when a retreat drifted across a cell boundary.
    case !l.attackerEngageable(attacker.ObjectID):
        l.target = 0
        l.engageAt = time.Time{}
        l.fleeFromThreat(now)

        return true
    default:
        l.logf("Hunt: %s aggroed on the approach, answering it "+
            "first", attacker.Name)
        l.target = attacker.ObjectID
        l.engageAt = now
        l.clearBlindRecovery()
    }

    return false
}

// switchStaleEngage drops a target whose repeated attack requests never
// started a real fight: the selection on the server points at an object
// that refuses the forced attack (typically the corpse of an abruptly
// disconnected session) or the armed stance went stale behind an
// obstacle. The gate is the fresh fight view - the looser SelfEngaged
// stays true forever behind an obstacle, because the server arms the
// attack stance without a line of sight check and no swing ever lands
// to disarm it. Only the selection of a different object id replaces
// the stale one, so the target is skipped for a while. An attacker of
// the character counts as engaged through the same call: the bow reuse
// window can refuse every attack request while the mob keeps swinging.
// It reports true when the tick was consumed.
func (l *Loop) switchStaleEngage(now time.Time) bool {
    if l.target == 0 || l.tracker.SelfEngagedBy(l.target) ||
        l.engageAt.IsZero() || now.Sub(l.engageAt) <= engageStuckTimeout ||
        l.blindRecoveryArmed(now) {
        return false
    }
    l.logf("Hunt: target %d does not engage, "+
        "switching to another", l.target)
    if l.targetSkip == nil {
        l.targetSkip = make(map[int32]time.Time)
    }
    l.targetSkip[l.target] = now.Add(engageSkipDelay)
    l.target = 0
    l.engageAt = time.Time{}
    l.resetFightClock()
    l.clearBlindRecovery()

    return true
}

// respondLosingFight drops a fight that turned into a death risk and
// flees. It stays ahead of the blind recovery: an add grinding a
// repositioning character must escalate into the flee, not into another
// detour segment. The archer kite answers with the straight flight once
// the target is dropped; the critical health logout stays armed above
// both.
func (l *Loop) respondLosingFight(now time.Time) bool {
    if l.target == 0 || !l.losingFight() {
        return false
    }
    l.fleeFromTarget(l.target, now)

    return true
}

// answerBlindEngage walks around the obstacle between the character and
// the selected target over the geodata, and switches the target only
// when no route clears the block (see loop_los.go). It reports true
// when the tick was consumed.
func (l *Loop) answerBlindEngage(now time.Time) bool {
    if l.target == 0 || !l.blindRecoveryArmed(now) {
        return false
    }
    l.recoverBlindEngage(now)

    return true
}

// pickTarget answers the targetless pick: the health rest, the mana
// hold, the self buff, the select pacing, the aggro answer and the
// windowed search. A fresh target falls through to the attack request;
// every other outcome consumes the tick and reports true.
func (l *Loop) pickTarget(now time.Time, manaHeld bool) bool {
    if l.restBeforePick(now) {
        return true
    }
    if manaHeld {
        // The mana rest of the caster: the dry one sits down, the one
        // above the sit threshold stands and regenerates. The self buffs
        // wait for the recovered bar.
        if l.tracker.SelfManaPercent() < manaSitPercent {
            l.rest()
        }

        return true
    }
    if l.maybeSelfBuff(now) {
        return true
    }
    if now.Sub(l.lastActionAt) < selectPeriod {
        return true
    }
    if l.pickNearestAttacker(now) {
        return true
    }
    if l.target != 0 {
        return false
    }
    if l.weaponlessRunWanted() {
        // Bare-handed with an affordable weapon in the plan: the fists
        // land almost nothing while the weapon run owns the next moves.
        // The attacker answer above stays armed.
        l.logWeaponWait(now)

        return true
    }
    pick, ok := l.tracker.NearestAttackablePreferredWindowed(
        attackNearestRange, l.pickZone(), l.activeSkips(now),
        l.minTargetLevel(), l.maxTargetLevel(), true,
        l.zoneMobPriority, l.avoidBowTargets())
    if !ok {
        if l.walkToFarTarget(now) {
            return true
        }
        l.patrolToCenter(now)

        return true
    }
    l.noTargetSince = time.Time{}
    l.noPickLogAt = time.Time{}
    l.target = pick.ObjectID
    l.engageAt = now
    l.clearBlindRecovery()
    // The covered pick answers with the ranged lure when the hunter owns
    // the tool (see lure.go); the ordinary attack flow runs otherwise.
    l.maybeBeginLure(now)

    return false
}

// restBeforePick rests a hurt character and answers a mob that reached
// a resting character: the regeneration is faster out of combat, and a
// sitting character stands up rather than taking the blows. A hurt
// character under attack keeps fleeing - it ran out of targets it can
// win. It reports true when the tick was consumed.
func (l *Loop) restBeforePick(now time.Time) bool {
    hurt := l.tracker.SelfHealthPercent() < reengageHealthPercent
    if !hurt && !l.tracker.SelfUnderAttack() {
        // The health recovered and the blows stopped: the flee episode
        // is over, a future escape gets a fresh budget.
        l.fleeSince = time.Time{}
        l.clearEscapeAnchor()
    }
    if hurt && l.tracker.SelfUnderAttack() {
        l.fleeFromThreat(now)

        return true
    }
    if l.tracker.SelfSitting() && l.tracker.SelfUnderAttack() {
        // A mob reached a resting character above the hurt gate: stand
        // up, the pick below selects the attacker and the fight answers
        // itself.
        l.standUpGuarded(now)

        return true
    }
    if l.tracker.SelfSitting() || hurt {
        l.rest()

        return true
    }

    return false
}

// pickNearestAttacker answers a mob that already holds the character as
// its target: walking to a fresh mob only drags the chase into a second
// opponent. The defensive flow owns an attacker the kite flight holds,
// one above the level ceiling, a skipped one and a bow add still
// firing; a healthy character adopts a winnable attacker where it
// stands and the attack request fires on the same tick. It reports true
// when the tick was consumed.
func (l *Loop) pickNearestAttacker(now time.Time) bool {
    attacker, ok := l.tracker.NearestAttacker()
    if !ok {
        return false
    }
    if l.kiteFlightHolds(now) ||
        !l.attackerEngageable(attacker.ObjectID) ||
        l.targetSkipped(attacker.ObjectID, now) ||
        l.tracker.HasBowAttackerBesides(attacker.ObjectID) {
        // The flight owns the answer until the server clears the aggro:
        // re-adopting a melee chaser while a bow add fires walked the
        // character back into its range on every flight tick.
        l.fleeFromThreat(now)

        return true
    }
    l.target = attacker.ObjectID
    l.engageAt = now
    l.clearBlindRecovery()
    l.logger.Printf("Hunt: %s is on us, fighting it "+
        "instead of picking a new target", attacker.Name)

    return false
}

// engageKiteTick runs the kite machinery of the engage: the motion
// ledger, the re-click ladder of the walk, the deferred retreat click
// of the live shot cycle and the shot-paced and proximity steps. All of
// them tick ahead of the fight flag branch, because the strict fight
// flag goes stale after a few seconds of pure walking. It reports true
// when the tick was consumed.
func (l *Loop) engageKiteTick(now time.Time) bool {
    // The ledger runs at the top of the fight tick - before the ladder
    // and the deferred click that may spend and return the tick - so
    // every tick of the fight books its second.
    l.kiteLedgerTick(now)
    if l.kiteReclickWalk(now) {
        return true
    }
    if l.kiteClickWalk(now) {
        return true
    }
    if l.kiteFightLive() {
        // The shot trigger outranks the proximity one: the fresh shot
        // is the earliest possible trigger of the same step.
        if l.kiteFromShot(now) {
            return true
        }
        if l.kiteFromTarget(now) {
            return true
        }
    }

    return false
}

// fightTick runs the confirmed running fight: the lure shot cycle, the
// fight start stamp of the kill journal, the flee episode reset, the
// engage clock re-anchor, the impending add step, the combat casting,
// the healing potion and the stall watchdog of a chase. It reports true
// when the character was fighting and the tick was consumed.
func (l *Loop) fightTick(now time.Time) bool {
    if !l.tracker.SelfFighting(l.target) {
        return false
    }
    if l.lureArmed() && l.lureTick(now) {
        return true
    }
    // The confirmed running fight stamps its start once: the kill
    // journal record measures the real fight length against it
    // (engageAt re-anchors and cannot).
    if l.fightStartAt.IsZero() || l.fightStartFor != l.target {
        l.fightStartAt = now
        l.fightStartFor = l.target
    }
    // A running fight ends the flee episode: the character answered
    // instead of running, the escape budget resets.
    l.fleeSince = time.Time{}
    l.clearEscapeAnchor()
    // The fresh fight re-anchors the engage clock: the stuck timeout
    // measures from the last real fight activity, not from the original
    // pick. The re-anchor requires the fight to have progressed past the
    // last refusal: the server keeps broadcasting the chase steps of an
    // attack it refuses to execute, and a clock re-anchored past every
    // refusal holds the stuck timeout and the blind recovery away
    // forever.
    if l.fightClearedRefusal() {
        l.engageAt = now
    }
    // The impending add step gets a step of clearance before its
    // on-sight trigger fires: backing off a potential second opponent
    // beats the pile up run and the emergency relogin of the real one.
    if l.avoidImpendingAdd(now) {
        return true
    }
    l.maybeCastCombatSkill(now)
    l.maybeDrinkFightPotion(now)
    l.walkStalledChase(now)

    return true
}

// walkStalledChase walks the stretch a stalled chase never covers: the
// Mobius path search stalls on some routes while the stuck chase
// packets keep the engagement fresh. A bow fight stands and shoots
// inside the weapon range, so only the stretches beyond the stall
// radius are walked.
func (l *Loop) walkStalledChase(now time.Time) {
    x, y, z, ok := l.tracker.ObjectPosition(l.target)
    if !ok {
        return
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return
    }
    dist := math.Hypot(float64(x-selfX), float64(y-selfY))
    if dist <= l.stallRadiusFor() ||
        l.lureArmed() ||
        !now.After(l.combatAvoidUntil) ||
        l.chaseProgress(&l.engLastDist, &l.engDistAt, dist, now) ||
        l.tracker.SelfWalking() ||
        now.Sub(l.lastActionAt) < engageRetryPeriod {
        return
    }
    l.lastActionAt = now
    l.logf("Hunt: chase on %d stalled at %d units, walking to the target",
        l.target, int(dist))
    if err := l.game.WalkTo(x, y, z); err != nil {
        l.logf("Hunt: chase walk failed: %v", err)
    }
}

// requestAttack sends the forced attack request of the current target.
// The request IS the next bow shot of the kite cycle: the retreat
// window ends at the shot's disable end, so it goes out the moment the
// bow is ready whatever range the chaser reached meanwhile. Waiting for
// a safe distance is a treadmill the mob out-regenerates.
func (l *Loop) requestAttack(now time.Time) {
    if now.Before(l.combatAvoidUntil) {
        // The impending-add step owns the movement for its short
        // window: a forced attack request interrupts a running walk
        // server-side.
        return
    }
    if now.Sub(l.lastActionAt) < engageRetryPeriod {
        return
    }
    if l.lureArmed() && l.lureTick(now) {
        return
    }
    if err := l.game.AttackTarget(l.target); err != nil {
        l.logf("Hunt: attack failed: %v", err)

        return
    }
    l.lastActionAt = now
}

// losingFight reports whether the running fight turned into a death
// risk: the character fell under the escape threshold, a bow-wielding
// add joined the melee fight, or a RUNNING fight dropped below the
// re-engage health while the target keeps a clear health lead (a level
// gap pull, an add that joined a social pack). The percent-gap rule
// needs the confirmed fight: before the first swing the target health
// is simply its full bar, and treating the untouched bar as a lead fled
// winnable aggroed mobs five seconds after the answer. A target that is
// nearly finished does not count as losing - the kill is one swing away
// and fleeing drops its loot. The bow add is the exception: it outranges
// the kite's straight retreat, so the reload tails of the shot cycle
// are free damage.
func (l *Loop) losingFight() bool {
    selfHP := l.tracker.SelfHealthPercent()
    targetHP := l.tracker.ObjectHealthPercent(l.target)
    // The bow add verdict: the ranged attacker joined a melee fight and
    // every shot cycle keeps the character inside its range. The fight
    // is dropped the moment the add is known, whatever the health bars
    // say - the nearly dead target does not buy the archer's fire.
    if l.tracker.HasBowAttackerBesides(l.target) {
        return true
    }
    if targetHP >= 0 && targetHP < escapeTargetBeatenPercent {
        return false
    }
    if selfHP < escapeHealthPercent {
        return true
    }
    if !l.tracker.SelfFighting(l.target) {
        // The fight has not started (the approach, the first attack
        // request of the aggro answer): only the critical health above
        // flees, the engaged mob gets its fight.
        return false
    }
    if selfHP >= reengageHealthPercent {
        return false
    }

    return targetHP >= 0 && targetHP-selfHP > losingFightGap
}

// maxTargetLevel bounds the mob level the engage initiates on: the
// character level plus the slack. A fresh spawn whose UserInfo has not
// arrived yet (level 0) disables the filter instead of fencing every
// pick out.
func (l *Loop) maxTargetLevel() int32 {
    level := l.tracker.SelfLevel()
    if level <= 0 {
        return 0
    }

    return level + targetMaxLevelSlack
}
