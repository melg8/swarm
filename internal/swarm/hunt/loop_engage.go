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
// confirm. The safety gates run before every attack decision: a
// Pre-consolidation phase debt; the hunt loop cleanup is planned
// (docs/quality_review_and_agent_prompts.md P07).
// losing fight is fled instead of fought to the death, a hurt
// character under attack keeps running instead of standing in the
// blows, and the target search never initiates on mobs above the
// character level slack or on social pulls whose clan mates stand
// within the help range.
// The engage decision tree: the target search, the attack start,
// the loot, the rest and the safety gates form one state machine.
// Refactor candidate: split into the phase handlers.
//
//nolint:cyclop,funlen,gocognit,maintidx
func (l *Loop) engage() {
    // The hunting ground gate: a character outside its ground (a
    // village respawn, the walk home after a finished fight) with
    // NOTHING pickable in sight walks back instead of hunting - going
    // to a zero-enemy ground is the LAST resort of the free-roam
    // order. Three exceptions: a hurt character under attack flees
    // even outside the ground (the leash walk home would drag it
    // through the chasing pack), a fight that crossed the ground
    // line is finished where it stands - the mob that dragged the
    // character out (a chase, an aggressive pull) dies before the
    // walk home, dropping the fight here would walk home through the
    // blows and leave the loot on the ground (see adoptOutZoneFight)
    // - and a pickable enemy visible from the CURRENT position keeps
    // the hunt running wherever it stands: the cell mode never
    // rigidly binds the fights to the held hexagon, the engage picks
    // the nearest visible enemy (see pickZone).
    now := time.Now()
    if l.lureArmed() && (l.lure.target != l.target || l.phase != phaseEngage) {
        // The pick changed under the lure (a kill, a skip, a flee
        // drop): the stale state stands down, the auto equipment
        // restores the melee weapon.
        l.clearLure()
    }
    if !l.onHeldGround() {
        if l.tracker.SelfUnderAttack() &&
            l.tracker.SelfHealthPercent() < reengageHealthPercent {
            l.fleeFromThreat(now)

            return
        }
        if !l.adoptOutZoneFight(now) && !l.cellEnemiesVisible(now) {
            if l.weaponlessRunWanted() {
                // The bare-handed errand outranks the walk
                // home: the return walk through the
                // aggressive packs never arrives unarmed -
                // the panic logout saves the character on
                // the ground it flees, the relogin handoff
                // resumes that ground and the packs pile on
                // again (the 2026-09-11 farm round: the
                // reset bot walked from the village to the
                // Spore Fungus SW ground bare handed and
                // livelocked in the logout cycle). The trip
                // machinery starts the weapon run on the
                // next tick and its return segment walks home
                // armed; the wallet that cannot afford any
                // weapon keeps the ordinary return (the
                // punches are all it has).
                if !l.zoneReturn {
                    l.zoneReturn = true
                    l.logger.Printf("Hunt: no weapon in " +
                        "hand, the weapon run outranks " +
                        "the walk home")
                }

                return
            }
            l.returnToZone()

            return
        }
        // Enemies stand visible outside the ground: the ordinary
        // pick flow below takes the nearest one wherever it stands.
    }
    if l.zoneReturn {
        l.zoneReturn = false
        l.zoneFails = 0
        l.roadFights = 0
        l.logf("Hunt: back in the hunting zone, resuming the hunt")
    }
    // The mana rest of the caster runs before the server target
    // re-adopt: a caster whose mana sits under the re-engage
    // threshold owns its fights until the bar recovers (the dry
    // one drops the running fight, the standing one holds the new
    // picks), and the re-adopt below would otherwise hand the
    // dropped target straight back. Under attack the hold lifts:
    // the caster answers the blows instead of sitting into them,
    // and the safety nets further down stay armed whatever the
    // mana says (see combat_skills.go).
    manaHeld := l.mageManaLow() && !l.tracker.SelfUnderAttack()
    if manaHeld && l.target != 0 {
        l.dropFightForMana()
    }
    // Prefer the server view of the target while it lives: the
    // MyTargetSelected answer of the last attack request arrives
    // asynchronously, so the fresh value is read every tick. A stale
    // id of a dead or removed target must not be re-adopted: the
    // server never clears the selection of a corpse (only the next
    // selection replaces it), so blindly trusting it locked the
    // loop into an engage/loot ping-pong where the next target was
    // never selected. A target marked stuck (its repeated attack
    // requests never started the fight) is not re-adopted either
    // while its skip delay lasts. The mana held caster re-adopts
    // nothing - the rest owns the selection until the mana stands
    // back up. The adoption also requires an attackable npc: the
    // server side selection a town trip leaves behind (the talked
    // teacher or merchant) points at a friendly villager, and the
    // forced attack requests on it only burn the engage stuck
    // timeout before the skip - the trip end clears the selection
    // (see GameClient.ClearTarget), this gate keeps the engage
    // clean even when a selection lingers (a talk without the trip
    // end, a self selection of the clear click itself).
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

        return
    }
    // The aggro answer of the approach: a mob that aggros while the
    // character walks to a pick must not wait for the pick to start
    // fighting - the walk would drag the chase into a second
    // opponent while the swings keep landing unanswered (the
    // reported peaceful pick fights). It runs ahead of the stuck
    // timeout, the losing-fight verdict and the blind recovery: an
    // add grinding a repositioning character outranks all three,
    // and the switch below re-anchors the engage clock on the mob
    // that actually attacks. The selected pick holding the character
    // as its own target no longer suppresses the answer: the pick is
    // the attack target, the mob that just joined is a different
    // object, and the nearer attacker owns the next swing.
    if l.target != 0 && !l.tracker.SelfFighting(l.target) {
        if attacker, ok := l.tracker.NearestAttacker(); ok &&
            attacker.ObjectID != l.target {
            switch {
            case l.kiteOwnsRetreat() && l.tracker.SelfEngagedBy(l.target):
                // The live kite fight keeps its pick: every fresh
                // add joins the train the straight retreat direction
                // already answers, and switching the arrows to each
                // one spread the damage across the pack while every
                // mob regenerated it back - the 2026-11-10 run-22
                // fight never resolved a kill in 131 seconds while
                // three grunts took turns stealing the target. A
                // peaceful pick an add caught on the approach is
                // engaged by neither side, so the entry below still
                // answers it.
            case !l.attackerEngageable(attacker.ObjectID) &&
                l.kiteOwnsRetreat():
                // The attacker outranks the character and the archer
                // kite owns the retreat: keep the pick and let the
                // straight lane answer. The melee flee fan is clamped
                // to the CURRENT zone box (a kite retreat drifts
                // across cell boundaries), and the 2026-11-09 live
                // run logged the session out mid-kite exactly there;
                // the kite threat scan already treats the add as the
                // nearest threat that arms the next step.
            case !l.attackerEngageable(attacker.ObjectID):
                // The attacker outranks the character: the peaceful
                // pick waits, the defense flow answers.
                l.target = 0
                l.engageAt = time.Time{}
                l.fleeFromThreat(now)

                return
            default:
                l.logf("Hunt: %s aggroed on the approach, answering it "+
                    "first", attacker.Name)
                l.target = attacker.ObjectID
                l.engageAt = now
                l.clearBlindRecovery()
            }
        }
    }
    if l.target != 0 && !l.tracker.SelfEngagedBy(l.target) &&
        !l.engageAt.IsZero() && now.Sub(l.engageAt) > engageStuckTimeout &&
        !l.blindRecoveryArmed(now) {
        // The repeated attack requests never started a real
        // fight: the selection on the server points at an
        // object that refuses the forced attack (typically the
        // corpse of an abruptly disconnected session, which
        // the server keeps selected), or the armed attack
        // stance went stale behind an obstacle. The gate is
        // the FRESH fight view: the looser SelfEngaged stays
        // true forever behind an obstacle, because the server
        // arms the attack stance without a line of sight
        // check (Creature.onForcedAttack) and no swing ever
        // lands to disarm it - the observed 53 minute stall
        // held the loop exactly there. Only the selection of a
        // DIFFERENT object id replaces the stale one, so the
        // target is dropped and skipped for a while; the
        // blind recovery owns the obstructed case with its
        // own budgets instead. The target ATTACKING the
        // character counts as engaged through the same
        // SelfEngagedBy call: the bow reuse window can refuse
        // every attack request while the mob keeps swinging
        // (the 14:02:40 dump dropped a grunt that stood at
        // 17-34 units and hitting), and dropping it left the
        // blows unanswered.
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

        return
    }
    if l.target != 0 && l.losingFight() {
        // The fight turned into a death risk: the health fell under
        // the escape threshold or the target keeps a clear health
        // lead over a hurt character (the level gap pull of the
        // observed death, an add that joined a social pack).
        // Pressing on below that line dies far more often than it
        // kills, so the fight is dropped and the character runs.
        // It stays ahead of the blind recovery: an add grinding a
        // repositioning character must escalate into the flee, not
        // into another detour segment. The archer kite answers with
        // the straight flight (kiteFleeThreat) once the target is
        // dropped - the targetless defensive flee has no fight
        // cycle to run, and the zone-clamped melee fan is only its
        // walled-lane fallback (the 2026-11-09 5-minute acceptance
        // run measured the death the old kite exemption produced:
        // the pursuit chain walked while two chasers glued at 14
        // units, the health fell 100 -> 69 -> 32 -> 21 percent and
        // the critical-HP logout never saw a tick under its 12
        // percent line). The critical-HP logout stays armed above
        // both.
        l.fleeFromTarget(l.target, now)

        return
    }
    if l.target != 0 && l.blindRecoveryArmed(now) {
        // The server refuses the swings with "Cannot see
        // target." (a fresh refusal was seen) or the planned
        // reposition walk is still running: an obstacle stands
        // between the character and the selected target. Walk
        // around it over the geodata first, switch the target only
        // when no route clears the block (see loop_los.go).
        l.recoverBlindEngage(now)

        return
    }
    // The spawn settle of the fresh session (see settle.go): the
    // protected character holds the spot and regenerates before the
    // first move, then opens with the first strike. It runs before
    // the targetless pick - the strike pick it arms must not be
    // overwritten by the ordinary search.
    if l.settleAfterLogin(now) {
        return
    }
    if l.target == 0 {
        // Rest while the character is hurt: the regeneration is
        // faster out of combat and engaging with low HP risks
        // death. A sitting character stands up through the rest
        // logic once recovered. A hurt character under attack ran
        // out of targets it can win: it keeps fleeing instead of
        // standing in the blows or sitting into them.
        hurt := l.tracker.SelfHealthPercent() < reengageHealthPercent
        if !hurt && !l.tracker.SelfUnderAttack() {
            // The health recovered and the blows stopped: the flee
            // episode is over, a future escape gets a fresh budget.
            l.fleeSince = time.Time{}
            l.clearEscapeAnchor()
        }
        if hurt && l.tracker.SelfUnderAttack() {
            l.fleeFromThreat(now)

            return
        }
        if l.tracker.SelfSitting() && l.tracker.SelfUnderAttack() {
            // A mob reached a resting character above the hurt gate:
            // stand up - the pick below selects the attacker and
            // the fight answers itself (the hurt branch above runs
            // at the low health instead).
            l.standUpGuarded(now)

            return
        }
        if l.tracker.SelfSitting() || hurt {
            l.rest()

            return
        }
        if manaHeld {
            // The mana rest of the caster: the dry one sits
            // down (the rest toggles it), the one above the
            // sit threshold stands and regenerates. The self
            // buffs wait for the recovered bar - a cast now
            // would spend the mana the rest is rebuilding.
            if l.tracker.SelfManaPercent() < manaSitPercent {
                l.rest()
            }

            return
        }
        if l.maybeSelfBuff(now) {
            // The missing self buff just fired: the tick is
            // spent on it, the next pick happens on the next
            // tick (the select pacing gates it anyway).
            return
        }
        if now.Sub(l.lastHit) < selectPeriod {
            return
        }
        // The aggro answer of the targetless pick: a mob
        // already holds the character as its target (its swings
        // or its chase - both carry the character as the target
        // id), so walking to a fresh mob only drags the chase
        // into a second opponent and the fights pile up (the
        // reported deaths). A healthy character with a winnable
        // attacker engages it at once - the fight answers the
        // aggro where it stands; anything else (hurt, the
        // attacker above the level ceiling) stays with the
        // defensive flow: the standard escape walk that logs
        // out when it never shakes the chase.
        if attacker, ok := l.tracker.NearestAttacker(); ok {
            if l.kiteFlightHolds(now) ||
                !l.attackerEngageable(attacker.ObjectID) ||
                l.targetSkipped(attacker.ObjectID, now) ||
                l.tracker.HasBowAttackerBesides(attacker.ObjectID) {
                // The defensive flow owns this attacker: the
                // straight flight holds a chase the loop already
                // ran from, this one outranks the character, the
                // loop just ran from it (the skip keeps the
                // flight authority), or a bow add is still
                // firing - re-adopting the melee chaser there
                // walked the character back into the archer's
                // range on every flight tick (the 2026-11-10
                // run-23 window ping-ponged through the
                // drop/adopt cycle every half second). The
                // run-28 dump did the same with a fled Fighter
                // at the two-minute skip expiry: the flight
                // owns the answer until the server clears the
                // aggro (see kiteFlightHolds).
                l.fleeFromThreat(now)

                return
            }
            l.target = attacker.ObjectID
            l.engageAt = now
            l.clearBlindRecovery()
            l.logger.Printf("Hunt: %s is on us, fighting it "+
                "instead of picking a new target",
                attacker.Name)
            // Falls through: the forced attack request below
            // fires on this very tick, the chase stops where
            // it stands.
        } else {
            if l.weaponlessRunWanted() {
                // Bare-handed with an affordable weapon
                // in the plan: no fresh targets - the
                // fists land 2 damage while the plan
                // offers a sword, and the weapon run
                // (the town trip trigger of this tick)
                // owns the next moves. The attacker
                // answer above stays armed: a mob
                // already on the character is fought
                // whatever the weapon state is.
                l.logWeaponWait(now)

                return
            }
            pick, ok := l.tracker.NearestAttackablePreferredWindowed(
                attackNearestRange, l.pickZone(), l.activeSkips(now),
                l.minTargetLevel(), l.maxTargetLevel(), true,
                l.zoneMobPriority, l.avoidBowTargets())
            if !ok {
                if l.walkToFarTarget(now) {
                    return
                }
                l.patrolToCenter(now)

                return
            }
            l.noTargetSince = time.Time{}
            l.noPickLogAt = time.Time{}
            l.target = pick.ObjectID
            l.engageAt = now
            l.clearBlindRecovery()
            // The covered pick answers with the ranged lure when the
            // hunter owns the tool (see lure.go): the approach and the
            // arm phases own the next ticks, the ordinary attack flow
            // keeps running otherwise.
            l.maybeBeginLure(now)
        }
    }
    // The kite motion ledger (see kite_ledger.go, the round-14
    // feedback ask): the per-fight standing/moving accounting runs
    // at the TOP of the fight tick - before the ladder and the
    // deferred click that may spend and return the tick - so every
    // tick of the fight books its second, whatever the tick
    // machinery below does with the moment (the QA audit of the
    // round caught the placement bias: samples skipped behind the
    // ladder returns systematically under-counted the idle bucket).
    l.kiteLedgerTick(now)
    // The re-click ladder of the kite walk (see kite.go, issue #60,
    // the second round): the retreat click of the kite proved
    // fragile three ways (the stance transition swallow, the
    // endpoint cell refusal, the H-006 silent drop) and the single
    // click of the first round left the character standing through
    // the whole bow reload the owner's dump reports. The ladder
    // re-issues the click while the walk window runs and the
    // character stands still on the issue cell - the manual
    // behavior the owner demonstrated ("i can manually start
    // clicking behind character and it runs 2 seconds no problem,
    // covering large distance, and bot can replicate it right
    // now"). It ticks HERE, ahead of the fight flag branch, so it
    // runs whether the server still holds the fighting stance or
    // the walk already tore it down - the dead click must not wait
    // for a flag that says the character is fighting when the dump
    // says it stands.
    if l.kiteReclickWalk(now) {
        return
    }
    // The deferred retreat click of the live shot cycle (see
    // kite.go, issue #70): the server saves a MoveToLocation inside
    // the bow windup and replays it at the disable end where the
    // re-shot cancels it, so the shot broadcast arms a schedule
    // instead of a click - the click fires here at the windup end
    // and rides the accepted move window. It ticks ahead of the
    // fight flag branch for the same reason the ladder does: the
    // windup may lapse (or tear the stance down) while the tick
    // machinery runs, and the deferred click must not wait for a
    // flag that names a state the cycle already left.
    if l.kiteClickWalk(now) {
        return
    }
    // The shot-paced kite of the archer (see kite.go, issue #60) and
    // the proximity kite step of the same layer (issue #70): the
    // character's own Attack broadcast is the server commit of the
    // bow shot, so the retreat starts the moment the shot released,
    // and a hostile that closed inside the retreat radius steps the
    // character clear before the melee blows land. Both tick ahead
    // of the SelfFighting branch: the strict fight flag goes stale
    // after a few seconds of pure walking (fightingFreshWindow), so
    // a long retreat leg stood the triggers down exactly when the
    // next cycle needed them. The live-fight view keeps the own
    // swing freshness, the target's own aggro and a train member
    // holding the character, so an ordinary approach never fires the
    // retreat. The shot trigger outranks the proximity one: the
    // fresh shot is the earliest possible trigger of the same step.
    if l.kiteFightLive() {
        if l.kiteFromShot(now) {
            return
        }
        if l.kiteFromTarget(now) {
            return
        }
    }
    if l.tracker.SelfFighting(l.target) {
        if l.lureArmed() && l.lureTick(now) {
            return
        }
        // The confirmed running fight stamps its start once: the
        // kill journal record measures the real fight length
        // against it (engageAt re-anchors and cannot).
        if l.fightStartAt.IsZero() || l.fightStartFor != l.target {
            l.fightStartAt = now
            l.fightStartFor = l.target
        }
        // A running fight ends the flee episode: the character
        // answered instead of running, the escape budget resets.
        l.fleeSince = time.Time{}
        l.clearEscapeAnchor()
        // The fresh fight also re-anchors the engage clock: the
        // stuck timeout measures from the last real fight
        // activity, not from the original pick - a slow but
        // living fight must never trip it. The re-anchor requires
        // the fight to have progressed past the last "Cannot see
        // target." refusal: the server keeps broadcasting the chase
        // steps of an attack it refuses to execute (the phantom
        // chase), and a clock re-anchored past every refusal holds
        // both the stuck timeout and the blind recovery away from
        // the obstructed engage forever (the 2026-09-12 03:25 dump:
        // 80 units from the target, over a minute of refusals, no
        // walk, no switch).
        if l.fightClearedRefusal() {
            l.engageAt = now
        }
        // The impending add: an aggressive neighbor about to
        // join the fight gets a step of clearance BEFORE its
        // on-sight trigger fires (see loop_avoid.go) - backing
        // off a potential second opponent beats the pile up
        // run and the emergency relogin of the real one.
        if l.avoidImpendingAdd(now) {
            return
        }
        // The combat casting of the running fight: the warrior
        // strike of the weapon in hand, the mystic attack spell
        // (see combat_skills.go). The casts share the human pacing
        // of the loop, the auto attack keeps swinging whatever the
        // cast request does.
        l.maybeCastCombatSkill(now)
        // The healing potion of the running fight (see
        // maybeDrinkFightPotion): the tanked pile up grinds the
        // health bar down faster than the natural regeneration, the
        // potion buys the swings that finish the first attacker.
        l.maybeDrinkFightPotion(now)
        // The swings land right now: nothing to re-request. A stale
        // engagement (the fight was interrupted, the auto attack flag
        // and the combat window linger) falls through and keeps
        // re-requesting the forced attack instead of standing still
        // until the stuck timeout switches the target. A running chase
        // also needs the progress watchdog: the Mobius path search
        // stalls on some routes while the stuck chase packets keep the
        // engagement fresh - then the loop walks the stretch itself.
        if x, y, z, ok := l.tracker.ObjectPosition(l.target); ok {
            if selfX, selfY, _, selfOK := l.tracker.SelfPosition(); selfOK {
                dist := math.Hypot(
                    float64(x-selfX), float64(y-selfY))
                // A bow fight stands and shoots inside the weapon
                // range: the stall watchdog only owns the stretches
                // beyond it (see stallRadiusFor).
                stallRadius := l.stallRadiusFor()
                if dist > stallRadius &&
                    !l.lureArmed() &&
                    now.After(l.combatAvoidUntil) &&
                    !l.chaseProgress(
                        &l.engLastDist, &l.engDistAt, dist, now) &&
                    !l.tracker.SelfWalking() &&
                    now.Sub(l.lastHit) >= engageRetryPeriod {
                    l.lastHit = now
                    l.logf("Hunt: chase on %d stalled at "+
                        "%d units, walking to the target",
                        l.target, int(dist))
                    if err := l.game.WalkTo(x, y, z); err != nil {
                        l.logf("Hunt: chase walk failed: %v",
                            err)
                    }
                }
            }
        }

        return
    }
    if now.Before(l.combatAvoidUntil) {
        // The impending-add step owns the movement for its
        // short window: a forced attack request interrupts a
        // running walk server-side, and the add would meet the
        // character right back where it stood.
        return
    }
    if now.Sub(l.lastHit) < engageRetryPeriod {
        return
    }
    if l.lureArmed() && l.lureTick(now) {
        return
    }
    // The forced attack re-request here IS the next bow shot of the
    // kite cycle (the owner contract of the 2026-11-09 round): the
    // retreat window ends at the shot's disable end, so the request
    // goes out the moment the bow is ready - whatever range the
    // chaser reached meanwhile. Waiting for a "safe" distance is the
    // treadmill the 5-minute acceptance run measured (20-30 s
    // between the shots, the mob regenerating faster than the
    // arrows).
    if err := l.game.AttackTarget(l.target); err != nil {
        l.logf("Hunt: attack failed: %v", err)

        return
    }
    l.lastHit = now
}

// losingFight reports whether the running fight turned into a death
// risk: the character fell under the escape threshold, a bow-wielding
// add joined the melee fight, or a RUNNING fight dropped below the
// re-engage health while the target keeps a clear health lead (a level
// gap pull, an add that joined a social pack). The percent-gap rule
// needs the confirmed fight: before the first swing the target health
// is simply its full bar, and treating the untouched bar as a lead
// fled winnable aggroed mobs five seconds after the answer (the
// reported flee at 56 percent while the mob sat at 100). A target that
// is nearly finished does not count as losing - the kill is one swing
// away and fleeing drops its loot. The bow add is the exception: it
// outranges the kite's straight retreat, so the reload tails the shot
// cycle spends in its fire are free damage, and the nearly-dead
// exemption covers the melee trade only - the 2026-11-10 live run kept
// shooting a 1 percent grunt while a level 8 Kaboo Orc Archer brought
// the health from 68 to 10 percent and forced the emergency logout.
func (l *Loop) losingFight() bool {
    selfHP := l.tracker.SelfHealthPercent()
    targetHP := l.tracker.ObjectHealthPercent(l.target)
    // The bow add verdict (the 2026-11-10 acceptance run): the ranged
    // attacker joined a melee fight and every shot cycle kept the
    // character inside its 1100 range. The straight flight opens that
    // range in seconds while the archer cannot answer the standing
    // trade, so the fight is dropped the moment the add is known -
    // whatever the target and the character health are (the nearly
    // dead target does not buy the archer's fire).
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
        // request of the aggro answer): only the critical health
        // above flees, the engaged mob gets its fight.
        return false
    }
    if selfHP >= reengageHealthPercent {
        return false
    }

    return targetHP >= 0 && targetHP-selfHP > losingFightGap
}

// maxTargetLevel bounds the mob level the engage initiates on: the
// character level plus the slack. A fresh spawn whose UserInfo has
// not arrived yet (level 0) disables the filter instead of fencing
// every pick out.
func (l *Loop) maxTargetLevel() int32 {
    level := l.tracker.SelfLevel()
    if level <= 0 {
        return 0
    }

    return level + targetMaxLevelSlack
}
