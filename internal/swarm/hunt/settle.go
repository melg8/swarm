// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The spawn protection settle of the hunt loop, split out of the
// loop.go god file: the post relogin window where the character
// holds the spot, regenerates under the server spawn protection and
// opens the next fight with a deliberate first strike instead of
// walking into an aggressive pack.

// The verified server facts the settle builds on (Mobius C1, see the
// H-005 registry entry and docs/hunting.md): every EnterWorld arms
// the spawn protection (PlayerSpawnProtection = 600 s in the deployed
// Player.ini), Attackable.getHating strips the aggro of a protected
// player every AI tick, and the protection clears ONLY on one of the
// five action packets - MoveToLocation, AttackRequest, Action,
// UseItem and RequestMagicSkillUse. The sit toggle of RequestActionUse
// is not in the list, so a sitting character keeps the protection and
// regenerates in peace even with an aggressive mob standing on it.

// settleAfterLogin holds the freshly logged in character on the spawn
// protection while it regenerates and aggressive mobs stand within
// their on-sight circles. It reports true while it owns the tick
// (the hold, the sit toggles, the stand up waits). The first call of
// a session that finds nothing to hold for, a blow that already
// landed (the protection is gone - somebody reached the character
// anyway) or a recovered health ends the settle: the recovered
// character opens with the first strike instead of walking (see
// maybeBeginFirstStrike), the rest falls through to the ordinary
// engage flow. The whole flow is opt in - the supervisor enables it
// for every session (every login IS a relogin into the same world
// spot), the unit tests and the manual sessions keep it off.
func (l *Loop) settleAfterLogin(now time.Time) bool {
    if !l.spawnSettle || l.settled {
        return false
    }
    if _, _, _, ok := l.tracker.SelfPosition(); !ok {
        // The self spawn packet has not arrived yet: the settle
        // holds without deciding - every scan below needs the
        // position.
        return true
    }
    startedAt := l.tracker.SessionStartedAt()
    if startedAt.IsZero() || now.Sub(startedAt) > settleWindow {
        l.endSettle("the settle window lapsed")

        return false
    }
    if l.tracker.SelfUnderAttack() {
        // A blow landed inside the window: the protection is already
        // broken (it strips hate, it never absorbs damage) - the
        // ordinary safety flow owns the fight from here.
        l.endSettle("a blow landed")

        return false
    }
    if !l.aggressiveMobNearby() {
        if l.tracker.KnownNpcCount() == 0 {
            // The enter world burst has not arrived yet: the empty
            // scan describes the packet gap, not the ground - hold
            // the spot instead of burning the one shot settle on it.
            return true
        }
        // The ground is loaded and clear: no aggressive mob reaches
        // the character, the protection has nothing to protect
        // against.
        l.endSettle("no aggressive mob in reach")

        return false
    }
    hp := l.tracker.SelfHealthPercent()
    if hp >= standUpHealthPercent {
        // The regeneration is done: stand up first (the server
        // refuses the attack requests of a sitting character, the
        // stand window of the rest guard paces the transition) and
        // open with the first strike.
        if l.settleSitPending(now) {
            // The sit request is still unconfirmed: hold until the
            // server answers before the stand and strike sequence.
            return true
        }
        if l.tracker.SelfSitting() || (!l.restActionAt.IsZero() &&
            !l.restActionSit) {
            if !l.standUpGuarded(now) {
                return true
            }
        }
        l.endSettle("")
        l.maybeBeginFirstStrike(now)

        return false
    }
    if l.settleHoldAt.IsZero() {
        l.settleHoldAt = now
        l.logf("Hunt: holding the spawn protection at %.0f%% HP, "+
            "regenerating before the first move", hp)
    }
    l.settleRest(now, hp)

    return true
}

// endSettle closes the settle window once. The empty reason skips
// the log line (a quiet end - the recovery simply finished).
func (l *Loop) endSettle(reason string) {
    if l.settled {
        return
    }
    l.settled = true
    l.settleHoldAt = time.Time{}
    if reason != "" {
        l.logf("Hunt: the spawn settle ended: %s", reason)
    }
}

// settleRest manages the sit/stand toggles of the settle hold: the
// character sits for the faster regeneration (the sit packet never
// clears the spawn protection) and holds the sit until the stand up
// health. The transitions reuse the rest confirmation fields - the
// same guard against a double toggle the rest logic runs.
func (l *Loop) settleRest(now time.Time, hp float64) {
    if l.tracker.SelfSitting() {
        return
    }
    if !l.restActionAt.IsZero() {
        if now.Sub(l.restActionAt) < restRetryPeriod {
            return
        }
        l.restActionAt = time.Time{}
    }
    if err := l.game.ActionSitStand(); err != nil {
        l.logf("Hunt: settle sit failed: %v", err)

        return
    }
    l.restActionAt = now
    l.restActionSit = true
    l.logf("Hunt: sitting down under the spawn protection at %.0f%% HP", hp)
}

// settleSitPending reports whether the settle sit request is still
// unconfirmed: the rest action guard window has not lapsed and the
// last transition sent was a sit. The stand and strike sequence
// waits it out - a strike fired before the confirm would land on a
// sitting character (the server refuses those attacks).
func (l *Loop) settleSitPending(now time.Time) bool {
    return !l.restActionAt.IsZero() && l.restActionSit &&
        now.Sub(l.restActionAt) < restRetryPeriod
}

// settleHolding reports whether the settle hold is active right now:
// the tick pile up gate suppresses the panic run while it holds - the
// protected character has no real attackers (the protection strips
// the hate before any broadcast), a phantom pair reading must not arm
// the irreversible run the settle exists to prevent.
func (l *Loop) settleHolding() bool {
    return l.spawnSettle && !l.settled
}

// EnableSpawnSettle arms the post relogin settle: the supervisor
// calls it on every fresh session (the login IS the relogin into the
// same world spot), the tests and the manual sessions stay off.
func (l *Loop) EnableSpawnSettle() {
    l.spawnSettle = true
}

// aggressiveMobNearby reports whether an idle aggressive mob stands
// within its own on-sight circle (plus the lure cover slack) of the
// character: the first move breaks the spawn protection straight
// into that mob's aggro (the AI scan adds the hate of a now
// unprotected player every second, no movement needed).
func (l *Loop) aggressiveMobNearby() bool {
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    threats := l.tracker.AppendAggroThreats(nil, 0, attackNearestRange)
    for i := range threats {
        threat := &threats[i]
        dist := math.Hypot(threat.X-float64(selfX), threat.Y-float64(selfY))
        if dist <= threat.AggroRange+lureCoverSlack {
            return true
        }
    }

    return false
}

// maybeBeginFirstStrike opens the post settle fight the deliberate
// way: the nearest aggressive mob inside the first strike range is
// attacked from the spot - the bow owner arms the ranged tool and
// fires while the mob still has to close the distance (the lure
// shoot phase runs the ranged fight and swaps the melee weapon back
// on the arrival), everyone else answers it in melee at the chosen
// spot with a recovered health bar. Without an aggressive mob in
// range the strike is a no-op: the ordinary pick flow walks out of
// the safe spot.
func (l *Loop) maybeBeginFirstStrike(now time.Time) bool {
    strike, ok := l.nearestAggressiveMob(firstStrikeRange)
    if !ok {
        return false
    }
    selfX, selfY, _, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    l.target = strike.ObjectID
    l.engageAt = now
    l.clearBlindRecovery()
    // The strike keeps the engage discipline: an attacker above the
    // level ceiling is not engaged (the protection burn would buy a
    // death risk), the dry mystic does not spend the cast it cannot
    // answer with.
    if !l.attackerEngageable(strike.ObjectID) || l.mageManaLow() {
        l.target = 0
        l.engageAt = time.Time{}

        return false
    }
    if bowID, arrowID, hasTool := l.bowAndArrow(); hasTool &&
        l.profileLuresWithBow() {
        var meleeObjID int32
        paperdoll := l.tracker.PaperdollSlotObjectIDs()
        if paperdoll[state.PaperdollRHand] != 0 {
            kind, weaponOK := l.tracker.SelfWeaponKind()
            if weaponOK && kind == weaponTypeBow {
                // The bow is already worn (an interrupted lure): the
                // arm phase skips its own equip.
                meleeObjID = 0
            } else {
                meleeObjID = paperdoll[state.PaperdollRHand]
            }
        }
        l.lure = &lureState{
            phase:      lureArm,
            target:     l.target,
            meleeObjID: meleeObjID,
            bowObjID:   bowID,
            arrowObjID: arrowID,
            standX:     selfX,
            standY:     selfY,
            startedAt:  now,
            segmentAt:  time.Time{},
            armAt:      time.Time{},
            armedBow:   false,
            armedArrow: false,
        }
        l.logf("Hunt: first strike on %s with the bow from the "+
            "protected spot", strike.Name)

        return true
    }
    l.logf("Hunt: first strike on %s from the protected spot",
        strike.Name)

    return true
}

// nearestAggressiveMob returns the closest idle aggressive npc within
// maxDistance of the character (the threat list of the tracker, the
// projected positions of the moving mobs included).
func (l *Loop) nearestAggressiveMob(
    maxDistance float64,
) (state.AggroThreat, bool) {
    threats := l.tracker.AppendAggroThreats(nil, 0, maxDistance)
    //nolint:exhaustruct_v5 // zero value grows inside the scan loop
    best := state.AggroThreat{}
    bestDist := math.MaxFloat64
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return best, false
    }
    for i := range threats {
        threat := &threats[i]
        dist := math.Hypot(threat.X-float64(selfX), threat.Y-float64(selfY))
        if dist < bestDist {
            bestDist = dist
            best = *threat
        }
    }

    return best, bestDist <= maxDistance
}
