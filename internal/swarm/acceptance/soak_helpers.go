// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"github.com/melg8/swarm/internal/swarm/state"
)

// deathEdgeTracker counts the character deaths over the soak window by
// watching the SelfDead flag for rising edges (false -> true). A death
// is a known maximum HP with a zero current HP (see Bot.SelfDead); the
// flag oscillates (the server resurrects the character at the village
// after the death), so the count is the number of transitions into the
// dead state, not the current flag value. The tracker is a
// single-goroutine read of the public tracker API, so it stays inside
// the T-001 acceptance scope.
type deathEdgeTracker struct {
	previous bool
	seen     bool
	count    int
}

// newDeathEdgeTracker arms the tracker. The first update seeds the
// previous flag without counting a death.
func newDeathEdgeTracker() *deathEdgeTracker {
	return &deathEdgeTracker{
		previous: false,
		seen:     false,
		count:    0,
	}
}

// update reads the tracker and advances the count on a rising edge.
func (d *deathEdgeTracker) update(tracker *state.Bot) {
	current := tracker.SelfDead()
	if !d.seen {
		d.seen = true
		d.previous = current

		return
	}
	if current && !d.previous {
		d.count++
	}
	d.previous = current
}

// deaths returns the counted death transitions.
func (d *deathEdgeTracker) deaths() int {
	return d.count
}

// soakRePaths reads the hunt loop re-path count from the tracker
// diagnostics. Each re-path is a stuck-and-replanned leg, so the delta
// over the soak window is the stuck-event metric the M1 trail
// reports. The snapshot is a read under the tracker lock; a nil
// engine or a not-yet-ticking loop reports zero.
func soakRePaths(tracker *state.Bot) int {
	return tracker.Snapshot().Diagnostics.Hunt.RePaths
}

// soakAdena reads the current adena count of the character from the
// snapshot. The field is the inventory adena total the shop strategy
// and the loot pickups maintain; zero before the first inventory
// update (a fresh level 1 character starts with no adena).
func soakAdena(tracker *state.Bot) int32 {
	return tracker.Snapshot().Character.Adena
}
