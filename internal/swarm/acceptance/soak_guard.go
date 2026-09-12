// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"fmt"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The stagnation thresholds of the soak scenario: the guard fails the
// run when the character gains no experience for stagnationNoXpMinutes
// or holds the same position for stagnationNoMoveMinutes. The two
// constants are the M and K of the M1 acceptance ("no XP gain for M
// minutes, no position change for K minutes"). The elven lands hunt
// loop kills a mob every 10-30 seconds at level 1, so ten minutes with
// no XP is a real livelock, not a slow patch; the position window is
// tighter because a healthy bot walks between kills and a stuck cell
// freezes the coordinates first.
const (
	stagnationNoXpMinutes   = 10
	stagnationNoMoveMinutes = 5
	stagnationStartupGrace  = 90 * time.Second
)

// stagnationGuard watches the tracker for the two soak livelock
// signatures: no experience gain for M minutes and no position change
// for K minutes. The guard is a tracker-public-API read inside the
// acceptance package (T-001 scope); T-002 will later surface the same
// events from the hunt loop and the soak scenario can consume them.
//
// The now seam lets the unit tests drive the clock; the production
// path uses time.Now. The guard is single-goroutine: the soak monitor
// loop owns it and calls update on every tick.
type stagnationGuard struct {
	now         func() time.Time
	bootTime    time.Time
	lastXp      int64
	lastXpAt    time.Time
	lastPos     posKey
	lastPosAt   time.Time
	xpSeen      bool
	posSeen     bool
	firedReason string
}

// posKey is the position snapshot the guard compares: a coordinate
// triple floored to the meter so the natural sub-unit jitter of the
// server movement broadcasts does not reset the timer.
type posKey struct {
	x int32
	y int32
	z int32
}

// newStagnationGuard arms the guard with the given clock. The first
// update after the startup grace seeds the last-seen timestamps; the
// thresholds only start counting after the first real observation.
func newStagnationGuard(now func() time.Time) *stagnationGuard {
	boot := now()

	return &stagnationGuard{
		now:         now,
		bootTime:    boot,
		lastXp:      0,
		lastXpAt:    boot,
		lastPos:     posKey{x: 0, y: 0, z: 0},
		lastPosAt:   boot,
		xpSeen:      false,
		posSeen:     false,
		firedReason: "",
	}
}

// update reads the tracker and advances the guard. A change of the
// cumulative experience (level plus the within-level exp) refreshes
// lastXpAt; a change of the floored position refreshes lastPosAt. The
// guard fires (and records the reason) once the startup grace has
// elapsed and either threshold is crossed. The character being
// offline pauses the position check (no position is known) but the
// experience check still runs - a long offline stretch with no XP is
// a reconnect livelock.
func (g *stagnationGuard) update(tracker *state.Bot) {
	now := g.now()
	xp := cumulativeSoakXP(tracker.SelfLevel(), tracker.SelfExp())
	if g.xpSeen && xp != g.lastXp {
		g.lastXpAt = now
	}
	g.lastXp = xp
	g.xpSeen = true

	if x, y, z, ok := tracker.SelfPosition(); ok {
		key := posKey{x: x, y: y, z: z}
		if g.posSeen && key != g.lastPos {
			g.lastPosAt = now
		}
		g.lastPos = key
		g.posSeen = true
	}

	if now.Sub(g.bootTime) < stagnationStartupGrace {
		return
	}
	if g.firedReason != "" {
		return
	}
	noXpFor := now.Sub(g.lastXpAt)
	if noXpFor >= stagnationNoXpMinutes*time.Minute {
		g.firedReason = fmt.Sprintf(
			"no experience gain for %s (threshold %dm)",
			noXpFor.Round(time.Second),
			stagnationNoXpMinutes)

		return
	}
	if g.posSeen {
		noMoveFor := now.Sub(g.lastPosAt)
		if noMoveFor >= stagnationNoMoveMinutes*time.Minute {
			g.firedReason = fmt.Sprintf(
				"no position change for %s (threshold %dm)",
				noMoveFor.Round(time.Second),
				stagnationNoMoveMinutes)
		}
	}
}

// fired reports whether the guard tripped.
func (g *stagnationGuard) fired() bool {
	return g.firedReason != ""
}

// reason returns the recorded trip reason (empty when the guard
// never fired).
func (g *stagnationGuard) reason() string {
	return g.firedReason
}
