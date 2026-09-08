// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// combatFeed is the combat animation feed of the web view, split out
// of the Bot god object: the swings of the Attack broadcasts and the
// damage of the HP deltas, bounded by combatEventMax and read with
// the combatEventTTL window by the snapshot. The sequence numbers
// are monotonic per bot so the client can dedupe across snapshots.
type combatFeed struct {
	events []CombatEvent
	seq    uint64
}

// newCombatFeed creates the empty feed.
func newCombatFeed() combatFeed {
	return combatFeed{events: nil, seq: 0}
}

// record appends one combat animation beat with the next sequence
// number and bounds the feed length. The caller must hold the bot
// write lock.
func (f *combatFeed) record(e CombatEvent, now time.Time) {
	f.seq++
	e.Seq = f.seq
	e.At = now
	f.events = append(f.events, e)
	if len(f.events) > combatEventMax {
		f.events = f.events[len(f.events)-combatEventMax:]
	}
}

// appendView copies the beats of the live TTL window onto dst in
// chronological order and returns the grown slice.
func (f *combatFeed) appendView(
	dst []CombatEventView, now time.Time,
) []CombatEventView {
	cut := now.Add(-combatEventTTL)
	for _, ev := range f.events {
		if ev.At.Before(cut) {
			continue
		}
		dst = append(dst, CombatEventView{
			Seq:        ev.Seq,
			Kind:       ev.Kind,
			AttackerID: ev.AttackerID,
			TargetID:   ev.TargetID,
			Amount:     ev.Amount,
			AtMs:       ev.At.UnixMilli(),
			X:          ev.X,
			Y:          ev.Y,
			TargetX:    ev.TargetX,
			TargetY:    ev.TargetY,
		})
	}

	return dst
}

// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

// recordCharDamageLocked feeds the damage of a self HP drop into
// the animation feed: the damage numbers of the web view render
// from the observed HP deltas, the Attack broadcast carries no
// damage value. Heals record nothing. The caller must hold the
// write lock.
func (b *Bot) recordCharDamageLocked(attrs []Attribute, now time.Time) {
	for _, attr := range attrs {
		if attr.ID != AttrCurHP || float64(attr.Value) >= b.char.CurHP {
			continue
		}
		b.recordCombatEventLocked(CombatEvent{
			Kind:       CombatEventDamage,
			TargetID:   b.selfID,
			AttackerID: 0,
			Amount:     b.char.CurHP - float64(attr.Value),
			X:          b.char.X,
			Y:          b.char.Y,
			TargetX:    0,
			TargetY:    0,
			Seq:        0,
			At:         now,
		})
	}
}

// recordObjectDamageLocked feeds the damage of an object HP drop
// into the animation feed the same way as the character one. The
// caller must hold the write lock.
func (b *Bot) recordObjectDamageLocked(
	hot *objectHot, cold *objectCold,
	objectID int32, attrs []Attribute, now time.Time,
) {
	for _, attr := range attrs {
		if attr.ID != AttrCurHP || float64(attr.Value) >= cold.CurHP {
			continue
		}
		b.recordCombatEventLocked(CombatEvent{
			Kind:       CombatEventDamage,
			TargetID:   objectID,
			AttackerID: 0,
			Amount:     cold.CurHP - float64(attr.Value),
			X:          hot.X,
			Y:          hot.Y,
			TargetX:    0,
			TargetY:    0,
			Seq:        0,
			At:         now,
		})
	}
}

// markObjectCombatLocked refreshes the combat window of an object and
// logs the transition into combat once. The caller must hold the state
// write lock.
func (b *Bot) markObjectCombatLocked(
	hot *objectHot, cold *objectCold, now time.Time,
) {
	if !hot.inCombat(now.UnixNano()) && cold.Name != "" {
		b.recordLocked(cold.Name + " enters combat")
	}
	hot.CombatUntil = now.Add(combatWindow).UnixNano()
}
