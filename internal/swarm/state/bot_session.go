// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "time"
)

// StartedAt returns the moment the tracker was created, the uptime anchor that
// survives the reconnects; the statistics view derives the long run rates from
// it.
func (b *Bot) StartedAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.started
}

// SessionStartedAt returns the moment of the current login: the bounded waits
// of the hunt loop - the skill list gate of the first town trip - measure
// against it, so every reconnect re-arms them.
func (b *Bot) SessionStartedAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.sessionAt
}

// ID returns the session id of the bot.
func (b *Bot) ID() string {
    return b.id
}

// Version returns a counter that increases on every state change.
func (b *Bot) Version() uint64 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.version
}

// Packets returns the count of packets the session received so far:
// the fleet diagnostics sample it to report the live packet rate.
func (b *Bot) Packets() int64 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.packets
}

// Status returns the current session status.
func (b *Bot) Status() Status {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.status
}

// Phase returns the last published hunt loop phase, empty before the loop sets
// it (the manual only and pre-world sessions never do); the web UI maps it to
// an activity banner.
func (b *Bot) Phase() string {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.phase
}

// SetPhase publishes the current hunt loop phase for the activity banner of the
// web UI (hunting, walking to town, selling, deleveling). A steady state
// refresh is a no-op, so the per tick call never churns the event stream.
func (b *Bot) SetPhase(phase string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if b.phase == phase {
        return
    }
    b.phase = phase
    b.phaseAt = b.now()
    b.touch()
}

// SetKind tags the role of the bot: KindLongRunning for the 24/7 fleet bots,
// KindAcceptance for the temp bots of the acceptance manager. It is set once
// and never changes; the web UI splits its sidebar on it.
func (b *Bot) SetKind(kind string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.kind = kind
}

// SetOnline marks the character as being inside the world.
func (b *Bot) SetOnline(name string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.char.Name = name
    b.status = StatusOnline
    b.touch()
    b.recordLocked("entered the world as " + name)
}

// SetOffline marks the session as ended.
func (b *Bot) SetOffline() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.status = StatusOffline
    b.touch()
    b.recordLocked("left the world")
}

// SetHuntingZone configures the hunting square of the map view. The zone
// survives the session resets: it is a policy, not an observation.
func (b *Bot) SetHuntingZone(cx int32, cy int32, half int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.zone = &Zone{CX: cx, CY: cy, Half: half}
    b.touch()
}

// SetHuntingZones publishes the zone registry of the map view with the active
// marker of the zone the loop hunts in. A repeated identical set returns early:
// the loop republishes on every zone state change but the 227 zone registry is
// the same on most of them, and the defensive copy was 4 MB of churn over a 3
// minute 100 bot fleet run.
func (b *Bot) SetHuntingZones(zones []ZoneView) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if zoneViewsEqual(b.zoneViews, zones) {
        return
    }
    b.zoneViews = make([]ZoneView, len(zones))
    copy(b.zoneViews, zones)
    b.touch()
}

// SetKillMarks publishes the recent kill ring: the ground positions where the
// targets died. The marks feed the fleet wide cross layer of the map
// (Registry.FleetKillMarks serves them for every bot), so the crosses
// accumulate across the bot switches. A repeated identical set returns early:
// the loop republishes the ring with every spot view refresh.
func (b *Bot) SetKillMarks(marks []KillMarkView) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if killMarksEqual(b.killMarks, marks) {
        return
    }
    b.killMarks = make([]KillMarkView, len(marks))
    copy(b.killMarks, marks)
    b.touch()
}

// KillMarks returns a defensive copy of the recent kill marks of the
// bot (the registry aggregates them for the fleet endpoint).
func (b *Bot) KillMarks() []KillMarkView {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if len(b.killMarks) == 0 {
        return nil
    }
    marks := make([]KillMarkView, len(b.killMarks))
    copy(marks, b.killMarks)

    return marks
}

// killMarksEqual reports whether two kill mark slices are equal, so
// SetKillMarks skips the defensive copy while the hunt ring holds the same
// kills.
func killMarksEqual(a, b []KillMarkView) bool {
    if len(a) != len(b) {
        return false
    }
    for i := range a {
        if a[i] != b[i] {
            return false
        }
    }

    return true
}

// zoneViewsEqual reports whether two zone view slices are equal, so
// SetHuntingZones skips the defensive copy when the published zones have not
// changed.
func zoneViewsEqual(a, b []ZoneView) bool {
    if len(a) != len(b) {
        return false
    }
    for i := range a {
        if a[i] != b[i] {
            return false
        }
    }

    return true
}

// SetLoginCooldown arms the login cooldown: an emergency logout asks for a
// pause before the next session starts, so the mobs reset and the character
// regenerates in peace.
func (b *Bot) SetLoginCooldown(pause time.Duration) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.loginCooldownUntil = b.now().Add(pause)
    b.touch()
}

// LoginCooldownRemaining returns the pause left before the next
// login, zero when no cooldown is armed or it already lapsed.
func (b *Bot) LoginCooldownRemaining() time.Duration {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.loginCooldownUntil.IsZero() {
        return 0
    }
    remaining := time.Until(b.loginCooldownUntil)
    if remaining < 0 {
        return 0
    }

    return remaining
}

// DangerSpot is one recorded emergency logout position: the ground stays
// dangerous until the level changes, and the zone regression counts the recent
// spots of its square against it.
type DangerSpot struct {
    X  int32
    Y  int32
    At time.Time
}

// The danger spot ring cap: eight recent emergency logouts name the ground that
// keeps kicking the character out without a per-mountain ledger of the world.
const dangerSpotCap = 8

// NoteDangerSpot records the position of one emergency logout. The ring
// survives the session resets (an emergency logout ends the loop instance that
// would count it), so the next session's zone regression still sees how hostile
// the ground was.
func (b *Bot) NoteDangerSpot(x int32, y int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.dangerSpots = append(b.dangerSpots,
        DangerSpot{X: x, Y: y, At: b.now()})
    if len(b.dangerSpots) > dangerSpotCap {
        b.dangerSpots = append(b.dangerSpots[:0], b.dangerSpots[1:]...)
    }
    b.touch()
}

// DangerSpots returns the emergency logout spots younger than the window; the
// hunt loop regression decides which zones they count against.
func (b *Bot) DangerSpots(window time.Duration) []DangerSpot {
    b.mu.RLock()
    defer b.mu.RUnlock()
    cutoff := b.now().Add(-window)
    spots := make([]DangerSpot, 0, len(b.dangerSpots))
    for _, spot := range b.dangerSpots {
        if spot.At.After(cutoff) {
            spots = append(spots, spot)
        }
    }

    return spots
}

// SetDeathRegression records the hunting cell the loop regressed away from
// after a death (an emergency logout counts through the same cell note). The
// cell bookkeeping lives in the per-session loop the supervisor rebuilds on
// every relogin, so the tracker carries the record: the fresh loop refuses the
// deadly cell until the heat cools.
func (b *Bot) SetDeathRegression(cellID string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.deathCell = cellID
    b.deathCellAt = b.now()
    b.touch()
}

// DeathRegression returns the cell id of the last death regression and the
// moment it happened (an empty id when none was recorded). The caller applies
// its own cool down: the heat decays with the death half-life, the record only
// carries the fact.
func (b *Bot) DeathRegression() (string, time.Time) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.deathCell, b.deathCellAt
}

// SetEmergencyLogout records the honest reason of the intentional emergency
// logout: the supervisor consumes it when the session unwinds and replaces the
// misleading socket-close error of the lost record with it.
func (b *Bot) SetEmergencyLogout(reason string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.emergencyReason = reason
    b.touch()
}

// ConsumeEmergencyLogout returns the recorded reason once: the first
// lost-session record carries it, the later network drops of the same
// supervisor report themselves.
func (b *Bot) ConsumeEmergencyLogout() (string, bool) {
    b.mu.Lock()
    defer b.mu.Unlock()
    reason := b.emergencyReason
    b.emergencyReason = ""
    b.touch()

    return reason, reason != ""
}

// ResetSession clears the observed world state before a new login: the server
// starts a fresh known list on every session, so the objects and inventory of a
// lost session must not leak into the next one. The event log, the packet
// counter and the session start time survive, so the history and the uptime
// stay continuous across reconnects.
func (b *Bot) ResetSession() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.drainCommands()
    b.metrics.sessions++
    b.sessionAt = b.now()
    b.selfID = 0
    b.char = newCharacterState()
    b.world = newObjectStore()
    b.inventory = newInventoryStore()
    b.inventoryVersion++
    b.skills = nil
    b.skillsRevision++
    b.quests = nil
    b.questItems = nil
    b.dialog = openDialog{} //nolint:exhaustruct_v5 // the zero page clears
    b.skillWeapons = nil
    b.buffs = nil
    b.buffsAt = time.Time{}
    b.walkPlan = nil
    b.walkPlanAt = time.Time{}
    b.clearShoppingPlanLocked()
    b.phase = ""
    b.phaseAt = time.Time{}
    // The hunt internals of the dead session carry no meaning: the loop
    // republishes them after its first tick.
    b.hunt = HuntDiagnostics{} //nolint:exhaustruct_v5 // the zero view resets
    b.huntPublishedAt = time.Time{}
    b.huntLastAction = ""
    b.huntLastActionAt = time.Time{}
    b.huntErrand = HuntErrand{} //nolint:exhaustruct_v5 // zero resets
    b.status = StatusConnecting
    b.touch()
}
