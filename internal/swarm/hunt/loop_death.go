// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "time"
)

// journalKill emits the structured kill record: the fight duration
// and the health the character ended with ride along for the combat
// efficiency aggregates of the session report.
func (l *Loop) journalKill(objectID int32, now time.Time) {
    if l.journal == nil {
        return
    }
    mob := l.tracker.ObjectName(objectID)
    mobLevel, _ := l.tracker.ObjectLevel(objectID)
    fight := 0.0
    if l.fightStartFor == objectID && !l.fightStartAt.IsZero() &&
        now.After(l.fightStartAt) {
        fight = now.Sub(l.fightStartAt).Seconds()
    }
    var x, y int32
    if selfX, selfY, _, ok := l.tracker.SelfPosition(); ok {
        x, y = selfX, selfY
    }
    l.journal.Kill(l.tracker.ID(), mob, mobLevel, fight,
        l.tracker.SelfHealthPercent(), x, y)
}

// resetFightClock drops the fight duration anchor of the killed or
// dropped target. The anchor must never outlive its fight: the Mobius
// object id free list hands the SAME id to the respawn of the same
// spawn point, so a stale anchor whose id matches the recycled one
// inherits the elapsed seconds of every earlier fight - the observed
// six hour run credited a lieutenant spawn with a 4369 second fight
// because the clock ran from the first kill of that id to the last, 188
// kills later. The fresh anchor lands on the next confirmed fight.
func (l *Loop) resetFightClock() {
    l.fightStartAt = time.Time{}
    l.fightStartFor = 0
}

// recoverFromDeath returns a dead character to the hunt: the village
// restart request is the death dialog choice of the official client and
// revives the character with restored vitals. It retries until the
// revival lands (the server refuses during the death delay), drops the
// stale target and loot references, and the engage lease walks the
// revived character back. A town trip interrupted by the death drops
// without a cooldown so a full inventory sells right away; a death
// during the deleveling keeps the phase running from the restart point.
func (l *Loop) recoverFromDeath() {
    now := l.now()
    if !l.restartAt.IsZero() && now.Sub(l.restartAt) < deathRestartPeriod {
        return
    }
    l.tracker.ClearWalkPlan()
    l.restartAt = now
    l.target = 0
    l.resetFightClock()
    l.clearBlindRecovery()
    l.lootID = 0
    if l.phase == phaseDelevel {
        // Count the death against the experience it removed
        // before the replan: the free death counter may abort
        // the deleveling once the character is alive again.
        l.noteDelevelDeath()
        l.waypoints = nil
        l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    } else {
        if l.autonomous {
            l.phase = phaseEngage
        } else {
            l.phase = phaseIdle
        }
        l.resetTownTrip()
        l.noTargetSince = time.Time{}
        l.fleeAt = time.Time{}
        l.fleeSince = time.Time{}
        l.clearEscapeAnchor()
        // The village restart lands next to the shops and the cooldown
        // a recent finished trip armed must not hold the sale back:
        // otherwise the revived character walks to the farm spot with
        // the full bag first and returns to sell later.
        l.tripEndedAt = time.Time{}
        // The death counts against the active zone: past the limit the band
        // ladder regresses onto easier grounds (the deleveling branch above
        // never counts, its deaths are the point of the phase).
        l.noteZoneDeath()
    }
    l.logf("Hunt: character died, restarting at the nearest village")
    if l.journal != nil {
        level := l.tracker.SelfLevel()
        x, y, _, _ := l.tracker.SelfPosition()
        l.journal.Death(l.tracker.ID(), level, x, y)
    }
    if err := l.game.RestartAtVillage(); err != nil {
        l.logf("Hunt: village restart failed: %v", err)
    }
}
