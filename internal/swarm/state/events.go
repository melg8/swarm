// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// eventLog is the rolling packet event log of one bot session, split
// out of the Bot god object: a fixed capacity ring buffer written by
// the packet apply paths and read by the snapshot. The ring allocates
// lazily on the first record so a fleet of idle or connecting
// sessions pays no per bot log memory (a hundred trackers used to
// pay 20 KB each up front for rings they never filled).
type eventLog struct {
	ring   []Event
	length int
	head   int
}

// newEventLog creates the empty log.
func newEventLog() eventLog {
	return eventLog{ring: nil, length: 0, head: 0}
}

// record appends one event at the current time. The caller must hold
// the bot write lock.
func (l *eventLog) record(message string, at time.Time) {
	if l.ring == nil {
		l.ring = make([]Event, eventCapacity)
	}
	l.ring[l.head] = Event{Time: at, Message: message}
	l.head = (l.head + 1) % eventCapacity
	if l.length < eventCapacity {
		l.length++
	}
}

// appendNewest copies up to limit newest events onto dst in
// chronological order and returns the grown slice.
func (l *eventLog) appendNewest(dst []Event, limit int) []Event {
	count := min(l.length, limit)
	for i := count; i > 0; i-- {
		index := (l.head - i + eventCapacity) % eventCapacity
		dst = append(dst, l.ring[index])
	}

	return dst
}
