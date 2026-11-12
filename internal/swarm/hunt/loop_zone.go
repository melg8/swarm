// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// SetHuntingZone configures the hunting square: the bot attacks the
// monsters inside it only and walks back as soon as it leaves. A zero
// half disables the zone.
func (l *Loop) SetHuntingZone(cx int32, cy int32, half int32) {
    l.zoneCX, l.zoneCY, l.zoneHalf = cx, cy, half
    l.tracker.SetHuntingZone(cx, cy, half)
}

// zone returns the hunting zone of the loop, nil when disabled.
func (l *Loop) zone() *state.Zone {
    if l.zoneHalf == 0 {
        return nil
    }

    return &state.Zone{CX: l.zoneCX, CY: l.zoneCY, Half: l.zoneHalf}
}

// pickZone returns the TARGET SEARCH FENCE of the engage: the cell
// mode hunts the NEAREST VISIBLE enemy wherever it stands (nil - the
// knownlist bounds the search, the hunt never rigidly binds the
// fights to the held hexagon), the legacy zone mode keeps the
// square leash. The far target walk and the targetless diagnostic
// read the same fence.
//
//nolint:ireturn // nil and the square are both ZoneArea by design
func (l *Loop) pickZone() state.ZoneArea {
    if l.cell != nil {
        return nil
    }

    return l.zone()
}

// onHeldGround reports whether the character stands on the held
// hunting ground: the hexagon polygon in the cell mode (the
// free-roam gate - the patrol square is the movement anchor, the
// hexagon is the ground; the roam past the square but inside the
// hexagon is ordinary hunting), the patrol square in the legacy
// zone mode.
func (l *Loop) onHeldGround() bool {
    if l.cell != nil && l.cell.leash != nil {
        x, y, _, ok := l.tracker.SelfPosition()

        return !ok || l.cell.leash.Contains(x, y)
    }

    return l.inZoneSelf()
}

// cellEnemiesVisible reports whether the cell-mode hunt sees a
// pickable enemy through the level window ANYWHERE in the knownlist:
// the free-roam hunt keeps fighting wherever the enemies stand, so
// the walk home only fires when nothing at all is visible (going to
// a zero-enemy ground is the last resort). The legacy zone mode
// keeps the strict return - the square leash owns the movement there.
//
// The deck gate: a mob on a DIFFERENT deck than the character (a z
// gap past the deckReachableZ threshold) reads as unreachable by a
// direct walk - the straight line from the village terrace to the
// field deck below crosses the village railing and the deck edge,
// the server's geodata correction collapses the click target onto
// the walker cell and the move is silently canceled. The dump of
// 2026-09-14 07:53 (build 140aa42, bot test2) showed exactly this
// freeze: the level 14 character stood on the Elven Village terrace
// (z -2992) with its held cell on the field deck (z -3576), the
// knownlist held pickable mobs on the field deck, the cell mode
// read them as visible and the engage fell through to the direct
// walk of the pick flow - the walk never moved the character and
// the pathfound zone return (the ramp walk down to the field deck)
// never armed. The deck gate drops the cross-deck mobs from the
// visible reading so the engage falls through to returnToZone and
// the pathfinder walks the character down the ramp.
func (l *Loop) cellEnemiesVisible(now time.Time) bool {
    if l.cell == nil {
        return false
    }
    pick, ok := l.tracker.NearestAttackablePreferredWindowed(
        math.MaxFloat64, nil, l.activeSkips(now),
        l.minTargetLevel(), l.maxTargetLevel(), true,
        l.zoneMobPriority, l.avoidBowTargets())
    if !ok {
        return false
    }
    _, _, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        // No self position: the deck gate cannot run, the legacy
        // behavior stands (the knownlist reading alone).
        return true
    }
    // A mob whose z the server never reported (z == 0) passes the
    // deck gate: the same convention the level filter uses for an
    // unresolved template (level 0 passes every window). A stale
    // z reading would freeze a bot whose only visible mob sits on
    // an unresolved z - the legacy behavior stands for it.
    if pick.Z != 0 && math.Abs(float64(pick.Z-selfZ)) > deckReachableZ {
        return false
    }

    return true
}

// targetZone returns the TARGET LEASH of the loop: the convex
// polygon of the held cell in the cell mode (the emptiness reading
// of the economy and the audit fence on the WHOLE hexagon - the
// farm ground is the complete hexagon, never a square approximation
// of it), the movement square otherwise. The movement machinery (the
// patrol, the return segment, the flee steps) keeps the square zone() -
// the inscribed patrol square never leaves the cell. The target
// SEARCH itself runs unfenced in the cell mode (see pickZone) - the
// hunt fights the nearest visible enemy even outside the held
// hexagon.
//
//nolint:ireturn // the leash is polymorphic by design
func (l *Loop) targetZone() state.ZoneArea {
    if l.cell != nil && l.cell.leash != nil {
        return l.cell.leash
    }

    return l.zone()
}

// inZoneSelf reports whether the character stands inside the hunting
// zone (always true when the zone is disabled).
func (l *Loop) inZoneSelf() bool {
    zone := l.zone()
    if zone == nil {
        return true
    }
    x, y, _, ok := l.tracker.SelfPosition()

    return !ok || zone.Contains(x, y)
}

// DefaultHuntingZone is the configured hunting square of this
// deployment: 3300x3300 units centered just below the Newbie Helper of
// the Elven village. Wide enough to keep the gremlin pack on the east
// edge (up to x ~47460) and the keltir field south of it (up to
// y ~42980) inside the square.
func DefaultHuntingZone() (cx int32, cy int32, half int32) {
    return 46112, 41500, 1650
}
