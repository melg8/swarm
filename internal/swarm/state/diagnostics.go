// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// The diagnostics views of the snapshot: the liveness ages, rates and hunt
// internals a stuck bot report needs next to the world data (the field
// reference: docs/webui.md). An age of 0 means the reference never happened,
// not "right now"; the companion flags disambiguate the live values.

// Diagnostics is the health view of the snapshot: everything a live server
// report needs to tell a stuck or misbehaving bot from a healthy one. The
// encoders build it from the live state on every snapshot, so it rides the
// packet driven stream without extra publication churn.
type Diagnostics struct {
    PhaseForMs       int64   `json:"phaseForMs"`
    UpdatedAgoMs     int64   `json:"updatedAgoMs"`
    PacketsPerSecond float64 `json:"packetsPerSecond"`
    LoginCooldownMs  int64   `json:"loginCooldownMs"`
    AutoAttacking    bool    `json:"autoAttacking"`
    // FightingTargetID is the target the fight state tracks, distinct from the
    // character selection: a stale server side selection shows a corpse id.
    FightingTargetID  int32           `json:"fightingTargetId"`
    CombatActiveAgoMs int64           `json:"combatActiveAgoMs"`
    LastHitAgoMs      int64           `json:"lastHitAgoMs"`
    UnderAttack       bool            `json:"underAttack"`
    AttackerCount     int             `json:"attackerCount"`
    WalkFresh         bool            `json:"walkFresh"`
    MoveAgoMs         int64           `json:"moveAgoMs"`
    Objects           ObjectCounts    `json:"objects"`
    Hunt              HuntDiagnostics `json:"hunt"`
}

// ObjectCounts is the known list summary of the diagnostics: the per kind
// object tally with the dead units.
type ObjectCounts struct {
    NPCs    int `json:"npcs"`
    Players int `json:"players"`
    Items   int `json:"items"`
    Dead    int `json:"dead"`
}

// HuntDiagnostics is the hunt loop internals view of the snapshot; the field
// reference and the ETA arithmetic: docs/webui.md.
type HuntDiagnostics struct {
    TargetID           int32  `json:"targetId"`
    TargetForMs        int64  `json:"targetForMs"`
    SkippedTargets     int    `json:"skippedTargets"`
    NoTargetForMs      int64  `json:"noTargetForMs"`
    RePaths            int    `json:"rePaths"`
    StuckForMs         int64  `json:"stuckForMs"`
    WaypointsLeft      int    `json:"waypointsLeft"`
    TripForMs          int64  `json:"tripForMs"`
    FleeForMs          int64  `json:"fleeForMs"`
    BuyRetries         int    `json:"buyRetries"`
    XpStallForMs       int64  `json:"xpStallForMs"`
    PositionStallForMs int64  `json:"positionStallForMs"`
    LastAction         string `json:"lastAction"`
    DelevelActive      bool   `json:"delevelActive"`
    DelevelTarget      int32  `json:"delevelTarget"`
    DelevelFromLevel   int32  `json:"delevelFromLevel"`
    DelevelZoneMedian  int32  `json:"delevelZoneMedian"`
    LastActionAgoMs    int64  `json:"lastActionAgoMs"`
    TickAgoMs          int64  `json:"tickAgoMs"`
    WalkEtaMs          int64  `json:"walkEtaMs"`
    KillEtaMs          int64  `json:"killEtaMs"`
}

// AgeMs floors a duration to whole seconds reported as milliseconds; a negative
// duration (a clock step) clamps to zero.
func AgeMs(d time.Duration) int64 {
    if d < 0 {
        return 0
    }

    return int64(d/time.Second) * 1000
}

// HuntErrand is the status of a blocking errand of the hunt loop (the city
// shopping round); an open errand carries the zero EndedAt. The acceptance
// runner ends a scenario at once when the errand it follows already stopped.
type HuntErrand struct {
    Name      string    `json:"name"`
    StartedAt time.Time `json:"startedAt"`
    EndedAt   time.Time `json:"endedAt"`
    Err       string    `json:"err"`
}

// packetRateSeconds is the width of the packet rate window.
const packetRateSeconds = 10

// packetRateWindow counts the received packets per second over the last
// packetRateSeconds seconds. counts[0] holds the current unix second; filled
// tracks how many trailing seconds carry data, so a fresh session reports a
// rate over its real lifetime instead of a diluted window.
type packetRateWindow struct {
    second int64
    filled int
    counts [packetRateSeconds]int32
}

// note counts one packet arriving at the given unix second.
func (w *packetRateWindow) note(sec int64) {
    switch {
    case sec < w.second:
        // A clock step backwards restarts the window.
        w.second = sec
        w.filled = 0
        w.counts = [packetRateSeconds]int32{}
    case sec > w.second:
        age := int(min(sec-w.second, packetRateSeconds))
        if age >= packetRateSeconds {
            w.counts = [packetRateSeconds]int32{}
            w.filled = 0
        } else {
            copy(w.counts[age:], w.counts[:packetRateSeconds-age])
            for i := range age {
                w.counts[i] = 0
            }
            w.filled = min(w.filled+age, packetRateSeconds)
        }
        w.second = sec
    }
    w.counts[0]++
    if w.filled == 0 {
        w.filled = 1
    }
}

// rate returns the packets per second over the filled part of the
// window.
func (w *packetRateWindow) rate() float64 {
    if w.filled == 0 {
        return 0
    }
    sum := int64(0)
    for _, count := range w.counts {
        sum += int64(count)
    }

    return float64(sum) / float64(w.filled)
}

// SetHuntDiagnostics publishes the hunt loop internals for the diagnostics
// section. The call stamps the publication time (the TickAgoMs heartbeat) and
// does not bump the state version, so the per tick publication never churns the
// event stream on its own.
func (b *Bot) SetHuntDiagnostics(d HuntDiagnostics) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.hunt = d
    b.huntPublishedAt = b.now()
}

// NoteAction records a hunt loop decision: the message lands in the rolling
// event log and becomes the last action of the hunt diagnostics view. Like
// RecordEvent it does not bump the state version.
func (b *Bot) NoteAction(message string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.huntLastAction = message
    b.huntLastActionAt = b.now()
    b.recordLocked(message)
}

// NoteLastAction updates the last action view WITHOUT recording the message
// into the event log: the hunt loop logger already mirrors every decision line
// through its own writer (the huntEventLogger of cmd/swarm, the sessionLogger
// of the acceptance runner), so NoteAction would double every Hunt: line.
func (b *Bot) NoteLastAction(message string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.huntLastAction = message
    b.huntLastActionAt = b.now()
}

// SetHuntErrand publishes the status of the blocking hunt errand.
func (b *Bot) SetHuntErrand(errand HuntErrand) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.huntErrand = errand
}

// HuntErrand returns the last published hunt errand status (zero: none ran).
func (b *Bot) HuntErrand() HuntErrand {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.huntErrand
}

// worldCounts is the per kind tally of the world object array, collected during
// the encoder object walk (no second pass over the hot records).
type worldCounts struct {
    npcs      int
    players   int
    items     int
    dead      int
    attackers int
}

// note folds one hot record into the tally; the attacker count mirrors the
// SelfAttackerCount scan (a living attackable npc targeting the character).
func (c *worldCounts) note(hot *objectHot, selfID int32) {
    switch hot.Kind {
    case kindNPC:
        c.npcs++
    case kindPlayer:
        c.players++
    case kindItem:
        c.items++
    }
    if hot.Dead && hot.Kind != kindItem {
        c.dead++
    }
    if hot.Kind == kindNPC && hot.Attackable && !hot.Dead &&
        hot.TargetID == selfID {
        c.attackers++
    }
}

// diagnosticsLocked builds the diagnostics view from the live state and the
// object walk tally. The caller must hold a lock.
func (b *Bot) diagnosticsLocked(now time.Time, counts worldCounts) Diagnostics {
    updated := b.updated
    if updated.IsZero() {
        updated = b.started
    }
    diagnostics := Diagnostics{
        PhaseForMs:        ageSinceLocked(now, b.phaseAt),
        UpdatedAgoMs:      AgeMs(now.Sub(updated)),
        PacketsPerSecond:  b.packetWindow.rate(),
        LoginCooldownMs:   AgeMs(b.loginCooldownRemainingLocked(now)),
        AutoAttacking:     b.char.AutoAttacking,
        FightingTargetID:  b.char.FightingTargetID,
        CombatActiveAgoMs: ageSinceLocked(now, b.char.CombatActiveAt),
        LastHitAgoMs:      ageSinceLocked(now, b.char.LastHitAt),
        UnderAttack:       b.underAttackLocked(now),
        AttackerCount:     counts.attackers,
        WalkFresh: b.char.Moving &&
            now.Sub(b.char.MoveAt) <= walkingFreshWindow,
        MoveAgoMs: ageSinceLocked(now, b.char.MoveAt),
        Objects: ObjectCounts{
            NPCs:    counts.npcs,
            Players: counts.players,
            Items:   counts.items,
            Dead:    counts.dead,
        },
        Hunt: b.hunt,
    }
    diagnostics.Hunt.LastAction = b.huntLastAction
    diagnostics.Hunt.LastActionAgoMs = ageSinceLocked(
        now, b.huntLastActionAt)
    diagnostics.Hunt.TickAgoMs = ageSinceLocked(now, b.huntPublishedAt)

    return diagnostics
}

// ageSinceLocked returns the floored age of a timestamp (zero for the zero
// time). The caller must hold a lock.
func ageSinceLocked(now time.Time, at time.Time) int64 {
    if at.IsZero() {
        return 0
    }

    return AgeMs(now.Sub(at))
}

// loginCooldownRemainingLocked returns the login cooldown left at the given
// time (zero when none is armed or it already lapsed). The caller holds a lock.
func (b *Bot) loginCooldownRemainingLocked(now time.Time) time.Duration {
    if b.loginCooldownUntil.IsZero() {
        return 0
    }
    if remaining := b.loginCooldownUntil.Sub(now); remaining > 0 {
        return remaining
    }

    return 0
}

// underAttackLocked reports whether the character took a hit within the under
// attack window. The caller must hold a lock.
func (b *Bot) underAttackLocked(now time.Time) bool {
    return !b.char.LastHitAt.IsZero() &&
        now.Sub(b.char.LastHitAt) < underAttackWindow
}
