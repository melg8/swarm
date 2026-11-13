// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// The lifetime statistics counters of a bot tracker, the session history facts
// the statistics tab aggregates. The counters survive the session resets (a
// relogin continues the same tracker), so a 24/7 process reports the whole
// deployment story. Attribution: a kill counts when the object the character
// fights dies (the fight target of the last swings and chase steps), and a
// steal between two swings counts too; a death counts on the alive to dead
// transition of the played character, the village restart that follows is a
// revive; a session counts every login attempt, so the first opens the run.

// killAttributionWindow bounds how fresh the last self combat activity must be
// for the death of the fighting target to count: the blow precedes the death by
// a sub second.
const killAttributionWindow = 10 * time.Second

// tickEmaWeight is the smoothing factor of the hunt tick duration EMA: fast
// enough to react, slow enough to keep one slow tick from spiking the average.
const tickEmaWeight = 0.1

// tickMaxWindow is how long the maximum tick duration stays armed: the view
// reports the worst tick of the last window, not of the process lifetime.
const tickMaxWindow = time.Minute

// botMetrics accumulates the lifetime counters of one bot tracker.
// Guarded by the bot mutex like every other mutable state.
type botMetrics struct {
    kills        int32
    deaths       int32
    sessions     int32
    swingsMade   int32
    swingsLanded int32
    swingsTaken  int32
    damageTaken  float64
    lastKillAt   time.Time
    lastDeathAt  time.Time
    tickEMA      time.Duration
    tickMax      time.Duration
    tickMaxAt    time.Time
    tickCount    int64
}

func newBotMetrics() botMetrics {
    return botMetrics{
        kills:        0,
        deaths:       0,
        sessions:     0,
        swingsMade:   0,
        swingsLanded: 0,
        swingsTaken:  0,
        damageTaken:  0,
        lastKillAt:   time.Time{},
        lastDeathAt:  time.Time{},
        tickEMA:      0,
        tickMax:      0,
        tickMaxAt:    time.Time{},
        tickCount:    0,
    }
}

// MetricsView is the read-only statistics snapshot: the lifetime counters with
// their reference times, so the aggregation layer derives the rates and ages; a
// zero time encodes "never happened".
type MetricsView struct {
    Kills        int32         `json:"kills"`
    Deaths       int32         `json:"deaths"`
    Sessions     int32         `json:"sessions"`
    SwingsMade   int32         `json:"swingsMade"`
    SwingsLanded int32         `json:"swingsLanded"`
    SwingsTaken  int32         `json:"swingsTaken"`
    DamageTaken  float64       `json:"damageTaken"`
    LastKillAt   time.Time     `json:"lastKillAt"`
    LastDeathAt  time.Time     `json:"lastDeathAt"`
    TickEMA      time.Duration `json:"tickEma"`
    TickMax      time.Duration `json:"tickMax"`
    TickCount    int64         `json:"tickCount"`
}

// Metrics returns the lifetime counters view of the bot.
func (b *Bot) Metrics() MetricsView {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return MetricsView{
        Kills:        b.metrics.kills,
        Deaths:       b.metrics.deaths,
        Sessions:     b.metrics.sessions,
        SwingsMade:   b.metrics.swingsMade,
        SwingsLanded: b.metrics.swingsLanded,
        SwingsTaken:  b.metrics.swingsTaken,
        DamageTaken:  b.metrics.damageTaken,
        LastKillAt:   b.metrics.lastKillAt,
        LastDeathAt:  b.metrics.lastDeathAt,
        TickEMA:      b.metrics.tickEMA,
        TickMax:      b.metrics.tickMax,
        TickCount:    b.metrics.tickCount,
    }
}

// NoteHuntTick feeds one measured hunt loop tick duration: the statistics tab
// reports the average and the worst tick (the loop cadence is the health signal
// of a loaded process).
func (b *Bot) NoteHuntTick(d time.Duration) {
    if d < 0 {
        return
    }
    b.mu.Lock()
    defer b.mu.Unlock()
    b.metrics.tickCount++
    b.metrics.tickEMA += time.Duration(
        tickEmaWeight * float64(d-b.metrics.tickEMA))
    now := b.now()
    if b.metrics.tickMaxAt.IsZero() ||
        now.Sub(b.metrics.tickMaxAt) > tickMaxWindow {
        b.metrics.tickMax = d
        b.metrics.tickMaxAt = now
    } else if d > b.metrics.tickMax {
        b.metrics.tickMax = d
    }
}

func (b *Bot) noteDeathLocked(now time.Time) {
    b.metrics.deaths++
    b.metrics.lastDeathAt = now
}

// noteKillLocked counts the death of the object the character fights, the last
// fighting target with fresh self combat activity.
func (b *Bot) noteKillLocked(objectID int32, now time.Time) {
    if objectID != b.char.FightingTargetID ||
        now.Sub(b.char.CombatActiveAt) > killAttributionWindow {
        return
    }
    b.metrics.kills++
    b.metrics.lastKillAt = now
}

// noteSwingsLocked counts one Attack broadcast: the swings the played character
// made (with how many connected) and the swings aimed at it by other units.
func (b *Bot) noteSwingsLocked(a Attack) {
    if a.AttackerID == b.selfID {
        b.metrics.swingsMade++
        for i := range a.TargetCount {
            if a.HitFlags[i]&attackHitMissFlag == 0 {
                b.metrics.swingsLanded++
            }
        }

        return
    }
    for i := range a.TargetCount {
        if a.TargetIDs[i] == b.selfID {
            b.metrics.swingsTaken++
        }
    }
}

// noteDamageTakenLocked folds one observed HP drop into the cumulative damage
// taken.
func (b *Bot) noteDamageTakenLocked(amount float64) {
    if amount > 0 {
        b.metrics.damageTaken += amount
    }
}
