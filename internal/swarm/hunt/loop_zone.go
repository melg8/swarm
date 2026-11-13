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

// pickZone returns the target search fence of the engage: the cell mode hunts
// the nearest visible enemy wherever it stands (nil - the knownlist bounds the
// search), the legacy zone mode keeps the square leash. The far target walk
// and the targetless diagnostic read the same fence.
//
//nolint:ireturn // nil and the square are both ZoneArea by design
func (l *Loop) pickZone() state.ZoneArea {
    if l.cell != nil {
        return nil
    }

    return l.zone()
}

// onHeldGround reports whether the character stands on the held ground: the
// hexagon polygon in the cell mode (the patrol square is the movement anchor,
// the hexagon is the ground), the patrol square in the legacy zone mode.
func (l *Loop) onHeldGround() bool {
    if l.cell != nil && l.cell.leash != nil {
        x, y, _, ok := l.tracker.SelfPosition()

        return !ok || l.cell.leash.Contains(x, y)
    }

    return l.inZoneSelf()
}

// cellEnemiesVisible reports whether the cell mode sees a pickable enemy
// through the level window anywhere in the knownlist: the free-roam hunt fights
// wherever the enemies stand, so the walk home fires only when nothing is
// visible at all. A mob on a different deck (a z gap past deckReachableZ) reads
// as unreachable - the straight line crosses the railing and the deck edge, the
// geodata correction collapses the click onto the walker and the move is
// canceled - so the engage falls through to the pathfound return down the ramp
// instead of freezing on a walk that never moves (docs/hunting_cells.md).
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
        // No self position: the deck gate cannot run, the knownlist reading
        // alone stands.
        return true
    }
    // A mob whose z the server never reported (z == 0) passes the gate, the
    // same convention the level filter uses for an unresolved template; a
    // stale reading would freeze a bot whose only visible mob sits on it.
    if pick.Z != 0 && math.Abs(float64(pick.Z-selfZ)) > deckReachableZ {
        return false
    }

    return true
}

// targetZone returns the target leash of the loop: the convex polygon of the
// held cell in the cell mode (the economy emptiness reading and the audit fence
// on the whole hexagon), the movement square otherwise. The movement machinery
// (the patrol, the return segment, the flee steps) keeps the square zone() -
// the inscribed patrol square never leaves the cell - while the target search
// itself runs unfenced in the cell mode (pickZone).
//
//nolint:ireturn // the leash is polymorphic by design
func (l *Loop) targetZone() state.ZoneArea {
    if l.cell != nil && l.cell.leash != nil {
        return l.cell.leash
    }

    return l.zone()
}

// inZoneSelf reports whether the character stands inside the hunting zone
// (always true when the zone is disabled).
func (l *Loop) inZoneSelf() bool {
    zone := l.zone()
    if zone == nil {
        return true
    }
    x, y, _, ok := l.tracker.SelfPosition()

    return !ok || zone.Contains(x, y)
}

// DefaultHuntingZone is the configured hunting square: 3300x3300 units below
// the Newbie Helper of the Elven village. Wide enough to keep the gremlin pack
// on the east edge (x ~47460) and the keltir field south (y ~42980) inside.
func DefaultHuntingZone() (cx int32, cy int32, half int32) {
    return 46112, 41500, 1650
}
