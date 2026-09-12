// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// The diagnostics views of the snapshot: the liveness ages, rates and
// the hunt internals a stuck bot report needs next to the world data.
// The ages are floored to whole seconds (reported as milliseconds):
// a sub second resolution carries no diagnostic value, and the
// flooring keeps the two encode paths byte identical without a now
// race. An age of 0 means the reference never happened (the zero
// time), not "right now" - the companion flags (inCombat, moving,
// autoAttacking) disambiguate the live values.

// Diagnostics is the health view of the snapshot: everything a live
// server report needs to tell a stuck or misbehaving bot from a
// healthy one. The encoders build it from the live state on every
// snapshot, so it rides along the packet driven stream without extra
// publication churn.
type Diagnostics struct {
	// PhaseForMs is the time spent in the current hunt phase: the top
	// stuck signature (a townWalk of fifteen minutes is a dead walk).
	PhaseForMs int64 `json:"phaseForMs"`
	// UpdatedAgoMs is the time since the last state change (the
	// updated timestamp, the session start while nothing changed
	// yet): the world liveness signal.
	UpdatedAgoMs int64 `json:"updatedAgoMs"`
	// PacketsPerSecond is the received packet rate over the last
	// packetRateSeconds seconds: the protocol liveness signal (a
	// frozen socket shows 0 while the state may still look sane).
	PacketsPerSecond float64 `json:"packetsPerSecond"`
	// LoginCooldownMs is the reconnect pause left after an emergency
	// logout: the reason an offline bot stays away.
	LoginCooldownMs int64 `json:"loginCooldownMs"`
	// AutoAttacking is the raw auto attack flag of the character.
	AutoAttacking bool `json:"autoAttacking"`
	// FightingTargetID is the target the fight state tracks (the
	// chase and attack broadcasts set it), distinct from the
	// selection of the character view: the stale server side
	// selection failure shows a fighting id of a corpse.
	FightingTargetID int32 `json:"fightingTargetId"`
	// CombatActiveAgoMs is the time since the last swing or chase
	// step of the character: the freshness of the fight (the combat
	// window lingers ten seconds after the last blow).
	CombatActiveAgoMs int64 `json:"combatActiveAgoMs"`
	// LastHitAgoMs is the time since the character last took a hit.
	LastHitAgoMs int64 `json:"lastHitAgoMs"`
	// UnderAttack reports a hit within the under attack window.
	UnderAttack bool `json:"underAttack"`
	// AttackerCount is how many living attackable npcs hold the
	// character as their target right now: the aggro load of the
	// moment.
	AttackerCount int `json:"attackerCount"`
	// WalkFresh reports a moving flag backed by a fresh movement
	// broadcast: a stale flag is the lost stop packet signature of a
	// character that stands still while the dump claims motion.
	WalkFresh bool `json:"walkFresh"`
	// MoveAgoMs is the time since the last movement broadcast.
	MoveAgoMs int64 `json:"moveAgoMs"`
	// Objects summarizes the known list health of the session.
	Objects ObjectCounts `json:"objects"`
	// Hunt carries the internals the hunt loop published last.
	Hunt HuntDiagnostics `json:"hunt"`
}

// ObjectCounts is the known list summary of the diagnostics: the per
// kind object tally with the dead units - a report reads the state of
// the world around the bot at a glance (a bot that stands in a field
// of corpses with zero npcs left is a cleared square).
type ObjectCounts struct {
	NPCs    int `json:"npcs"`
	Players int `json:"players"`
	Items   int `json:"items"`
	Dead    int `json:"dead"`
}

// HuntDiagnostics is the hunt loop internals view of the snapshot.
// The loop publishes the scalar half every tick through
// SetHuntDiagnostics; the tracker owns the last action (NoteAction)
// and the encoders add the publication age (TickAgoMs) - a growing
// tick age means the loop goroutine stopped ticking.
type HuntDiagnostics struct {
	// TargetID is the object id the engage or loot phase works on.
	TargetID int32 `json:"targetId"`
	// TargetForMs is the age of the current target engagement: past
	// the engage stuck timeout the loop skips the target (the stale
	// server side selection case).
	TargetForMs int64 `json:"targetForMs"`
	// SkippedTargets is the count of targets the searches currently
	// skip: stuck evidence (refused engages, timed out pickups).
	SkippedTargets int `json:"skippedTargets"`
	// NoTargetForMs is the time the target search came up empty: the
	// patience before the patrol toward the zone center walks.
	NoTargetForMs int64 `json:"noTargetForMs"`
	// RePaths is the re-path count of the running walk: each entry
	// is a stuck and re-planned leg.
	RePaths int `json:"rePaths"`
	// StuckForMs is the time the walker stood still on the current
	// leg (the re-path watchdog input).
	StuckForMs int64 `json:"stuckForMs"`
	// WaypointsLeft is the count of waypoints remaining on the
	// planned walk (the manual plan, the town or delevel geodata
	// plan).
	WaypointsLeft int `json:"waypointsLeft"`
	// TripForMs is the age of the running town trip.
	TripForMs int64 `json:"tripForMs"`
	// FleeForMs is the age of the flee episode (a losing fight the
	// character runs from; past the budget the session logs out).
	FleeForMs int64 `json:"fleeForMs"`
	// BuyRetries is the retry count of the buy confirmation wait.
	BuyRetries int `json:"buyRetries"`
	// XpStallForMs is the time the character experience has been
	// static (the stagnation watch input): a healthy farm changes it
	// every few kills, a town trip holds it for minutes; the
	// stagnation event fires at the window bound (see
	// hunt.stagnationXPWindow).
	XpStallForMs int64 `json:"xpStallForMs"`
	// PositionStallForMs is the time the character has stood on the
	// same exact cell (the stagnation watch input): the walks and the
	// death cycles move it constantly; the freeze event fires at the
	// window bound (see hunt.stagnationPositionWindow).
	PositionStallForMs int64 `json:"positionStallForMs"`
	// LastAction is the last decision line of the loop (the same
	// text the event log carries).
	LastAction string `json:"lastAction"`
	// LastActionAgoMs is the age of the last decision.
	LastActionAgoMs int64 `json:"lastActionAgoMs"`
	// TickAgoMs is the time since the loop last published: the hunt
	// loop heartbeat age (a dead or blocked loop goroutine shows a
	// growing value while the session stays online).
	TickAgoMs int64 `json:"tickAgoMs"`
}

// AgeMs floors a duration to whole seconds reported as milliseconds:
// the resolution of every age field of the diagnostics views. A
// negative duration (a clock step) clamps to zero.
func AgeMs(d time.Duration) int64 {
	if d < 0 {
		return 0
	}

	return int64(d/time.Second) * 1000
}

// packetRateSeconds is the width of the packet rate window.
const packetRateSeconds = 10

// packetRateWindow counts the received packets per second over the
// last packetRateSeconds seconds: CountPacket feeds it under the
// write lock and the encoders read the rate under the read lock.
// counts[0] holds the current unix second, the older seconds shift
// back; filled tracks how many trailing seconds of the window carry
// data (the rate divides by it, so a fresh session reports a rate
// over its real lifetime instead of a diluted window).
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

// SetHuntDiagnostics publishes the hunt loop internals for the
// diagnostics section of the snapshot. The loop calls it on every
// tick; the call stamps the publication time (the tick heartbeat of
// TickAgoMs) and does not bump the state version - the values ride
// along the snapshots the packet traffic already drives, so the per
// tick publication never churns the event stream on its own.
func (b *Bot) SetHuntDiagnostics(d HuntDiagnostics) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hunt = d
	b.huntPublishedAt = time.Now()
}

// NoteAction records a hunt loop decision: the message lands in the
// rolling event log (so the dump carries the decision history next
// to the world data) and becomes the last action of the hunt
// diagnostics view. Like RecordEvent it does not bump the state
// version.
func (b *Bot) NoteAction(message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.huntLastAction = message
	b.huntLastActionAt = time.Now()
	b.recordLocked(message)
}

// worldCounts is the per kind tally of the world object array: the
// encoders collect it during their object walk (the diagnostics
// reuse it without a second pass over the hot records).
type worldCounts struct {
	npcs      int
	players   int
	items     int
	dead      int
	attackers int
}

// note folds one hot record into the tally. The attacker count
// mirrors the SelfAttackerCount scan: a living attackable npc that
// holds the character as its target.
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

// diagnosticsLocked builds the diagnostics view from the live state
// and the object walk tally of the caller. The caller must hold a
// lock.
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

// ageSinceLocked returns the floored age of a timestamp, zero for
// the zero time. The caller must hold a lock.
func ageSinceLocked(now time.Time, at time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return AgeMs(now.Sub(at))
}

// loginCooldownRemainingLocked returns the login cooldown left at
// the given time, zero when none is armed or it already lapsed. The
// caller must hold a lock.
func (b *Bot) loginCooldownRemainingLocked(now time.Time) time.Duration {
	if b.loginCooldownUntil.IsZero() {
		return 0
	}
	if remaining := b.loginCooldownUntil.Sub(now); remaining > 0 {
		return remaining
	}

	return 0
}

// underAttackLocked reports whether the character took a hit within
// the under attack window. The caller must hold a lock.
func (b *Bot) underAttackLocked(now time.Time) bool {
	return !b.char.LastHitAt.IsZero() &&
		now.Sub(b.char.LastHitAt) < underAttackWindow
}
