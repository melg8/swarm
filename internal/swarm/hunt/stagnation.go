// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The stagnation windows of the livelock watch: a silently stuck bot must
// become a loud line (the M1 soak proof) and then recover itself
// (docs/hunting.md, the stagnation watch section).
const (
    // stagnationXPWindow is how long the experience may hold without a livelock
    // event: kills change it every few seconds and the death penalty on every
    // delevel death, so a value this old means the loop stopped farming.
    stagnationXPWindow = 20 * time.Minute
    // stagnationPositionWindow is how long the character may stand on the exact
    // same cell: the walks, the fights and the death cycles all move it, a hold
    // this long means the moves stopped.
    stagnationPositionWindow = 10 * time.Minute
    // stagnationHardCooldown paces the session rebuild: a livelocked loop that
    // survives its relogin retries on the next window instead of thrashing
    // reconnects.
    stagnationHardCooldown = 15 * time.Minute
)

// observeStagnation watches the character progress and logs one explicit event
// per window when the experience or the exact position holds past its window,
// carrying the loop phase so the log explains the frozen state machine
// (docs/hunting.md, the stagnation watch). The first position stall soft resets
// the frozen loop state, and a stall that survives it or a full experience
// window rebuilds the session. The experience check runs first and arms
// logoutDone, so the position check must not run after it.
func (l *Loop) observeStagnation(now time.Time) {
    if !l.autonomous || l.tracker.Status() != state.StatusOnline {
        l.resetStagnationWatch()

        return
    }
    l.observeStagnationXP(now)
    if l.logoutDone {
        return
    }
    l.observeStagnationPosition(now)
}

// observeStagnationXP arms, refreshes and reports the experience
// stall timer.
func (l *Loop) observeStagnationXP(now time.Time) {
    xp := l.tracker.SelfExp()
    if l.stagXPAt.IsZero() || xp != l.stagXP {
        l.stagXP = xp
        l.stagXPAt = now
        l.stagXPFires = 0

        return
    }
    held := now.Sub(l.stagXPAt)
    if held < stagnationXPWindow {
        return
    }
    l.stagXPFires++
    l.logf("Hunt: stagnation: no experience change for %s, "+
        "exp %d held, phase %s", held.Round(time.Minute), xp, l.phase)
    if l.journal != nil {
        l.journal.Stall(l.tracker.ID(), "xp", held, 0, 0, string(l.phase))
    }
    // The window restarts instead of the whole watch: a permanently stalled
    // loop keeps logging one line per window until it progresses again.
    l.stagXPAt = now
    // The experience stall is the strongest livelock signal: the character may
    // drift between refused requests without farming anything, so the session
    // rebuild answers it directly.
    l.stagnationHardRecover("no experience change for "+
        held.Round(time.Minute).String(), now)
}

// observeStagnationPosition arms, refreshes and reports the position
// freeze timer.
func (l *Loop) observeStagnationPosition(now time.Time) {
    x, y, z, ok := l.tracker.SelfPosition()
    if !ok || !l.stagPosSet || x != l.stagPosX || y != l.stagPosY ||
        z != l.stagPosZ {
        l.stagPosX = x
        l.stagPosY = y
        l.stagPosZ = z
        l.stagPosSet = ok
        l.stagPosAt = now
        l.stagPosFires = 0

        return
    }
    held := now.Sub(l.stagPosAt)
    if held < stagnationPositionWindow {
        return
    }
    l.stagPosFires++
    l.logf("Hunt: stagnation: position held %s at %d %d %d, phase %s",
        held.Round(time.Minute), x, y, z, l.phase)
    if l.journal != nil {
        l.journal.Stall(l.tracker.ID(), "pos", held, x, y, string(l.phase))
    }
    l.stagPosAt = now
    // The freeze escalation: the first fire clears the loop state in place, a
    // freeze that survives it climbs to the session rebuild.
    if l.stagPosFires == 1 {
        l.stagnationSoftReset(now)

        return
    }
    l.stagnationHardRecover("position held "+held.Round(time.Minute).String()+
        " at "+strconv.Itoa(int(x))+" "+strconv.Itoa(int(y)), now)
}

// stagnationSoftReset clears the frozen loop state of the first position stall
// without ending the session: the engage bookkeeping, the pending loot, the
// blind recovery, the panic and flee episodes, a mid-freeze town trip and a
// sitting character all restart from the live world state. The delevel phase
// owns its own lifecycle (its deaths refresh the experience).
func (l *Loop) stagnationSoftReset(now time.Time) {
    if l.phase == phaseDelevel {
        l.logf("Hunt: stagnation recovery: the delevel phase owns " +
            "its own recovery, holding the soft reset")

        return
    }
    l.logf("Hunt: stagnation recovery: clearing the loop state, " +
        "restarting from the live world")
    if l.target != 0 {
        if l.targetSkip == nil {
            l.targetSkip = make(map[int32]time.Time)
        }
        // The frozen target keeps the short skip: the re-pick must prefer a
        // different object, the frozen one is the very suspect of the freeze.
        l.targetSkip[l.target] = now.Add(engageSkipDelay)
    }
    l.target = 0
    l.engageAt = time.Time{}
    l.clearBlindRecovery()
    l.lootID = 0
    l.panicAt = time.Time{}
    l.fleeAt = time.Time{}
    l.fleeSince = time.Time{}
    l.clearEscapeAnchor()
    l.noTargetSince = time.Time{}
    l.lastActionAt = time.Time{}
    // A trip caught mid freeze restarts from scratch: the frozen waypoints
    // drop and the trigger re-plans from where the character stands, while the
    // junk, the books and the adena stay in the inventory.
    l.resetTownTrip()
    // A frozen walk back home re-arms the pathfound return: the budget clears
    // so the next outside-the-zone tick plans a fresh route.
    l.zoneReturn = false
    l.zoneFails = 0
    l.zoneSegmentAt = time.Time{}
    l.standUpGuarded(now)
}

// stagnationHardRecover rebuilds the session through the emergency logout: the
// relogin constructs a fresh loop, resets the server side session state and
// clears every per-session structure a livelock can hide in - the known cure of
// the stale selection, the refused collapsed walk clicks and the frozen engage
// stance. The rebuild is paced by stagnationHardCooldown, and the delevel phase
// is exempt (its own timeout and death cycle own it).
func (l *Loop) stagnationHardRecover(reason string, now time.Time) {
    if l.logoutDone || l.phase == phaseDelevel {
        return
    }
    if !l.stagHardAt.IsZero() &&
        now.Sub(l.stagHardAt) < stagnationHardCooldown {
        // The recovery is on cooldown: the stall line above stays the only
        // output of this window.
        return
    }
    l.stagHardAt = now
    l.stagPosFires = 0
    l.stagXPFires = 0
    l.emergencyLogoutWithReason("stagnation " + reason +
        ", rebuilding the session")
}

// resetStagnationWatch clears the observed baselines: the next online tick
// re-arms both windows from fresh values, so neither an offline gap nor a
// manual session can fire an event for time the character was not autonomously
// in the world. The fire counters reset with the baselines.
func (l *Loop) resetStagnationWatch() {
    l.stagXPAt = time.Time{}
    l.stagPosSet = false
    l.stagPosAt = time.Time{}
    l.stagPosFires = 0
    l.stagXPFires = 0
}
