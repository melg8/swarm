// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "math"
    "time"
)

// ObjectTargetsSelf reports whether the npc object currently holds
// the played character as its target: the mob whose blows land or
// whose chase steps carry the character as the target id. The hunt
// loop uses it to keep a selected target that is already part of the
// aggro answer in place instead of shuffling between two attackers.
func (b *Bot) ObjectTargetsSelf(objectID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)
    if obj == nil {
        return false
    }

    return b.selfID != 0 && obj.TargetID == b.selfID
}

// KnownNpcCount returns how many npcs the session knows right now:
// the warmup gauge of the enter world burst. A zero means the spawn
// packets have not arrived yet, so a scan over the world answers
// empty without describing the real surroundings.
func (b *Bot) KnownNpcCount() int {
    b.mu.RLock()
    defer b.mu.RUnlock()
    count := 0
    for i := range b.world.hot {
        if b.world.hot[i].Kind == kindNPC {
            count++
        }
    }

    return count
}

// ObjectPosition returns the last observed placement of a known object.
func (b *Bot) ObjectPosition(objectID int32) (int32, int32, int32, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)
    if obj == nil {
        return 0, 0, 0, false
    }

    return obj.X, obj.Y, obj.Z, true
}

// ObjectTrack is the chase view of a known object: the raw
// last-known placement plus the movement segment it belongs to. The
// server broadcasts a chasing mob's MoveToPawn at a one to two second
// cadence, so the raw placement can lag a whole close-in behind the
// live mob; the distance floors of the kite layer must not trust that
// stale gap (the 2026-11-09 live acceptance run measured a 512 unit
// raw gap against an actual 256).
type ObjectTrack struct {
    // X/Y/Z are the raw last-broadcast placement.
    X int32
    Y int32
    Z int32
    // MovedAt is the start of the movement segment the placement
    // belongs to; the zero time while the object stands.
    MovedAt time.Time
    // Speed is the fastest movement speed of the object in world
    // units per second (see chaseSpeed).
    Speed float64
    // Moving reports whether the last movement packet left the
    // object walking.
    Moving bool
}

// ObjectTrackOf returns the chase view of a known object (see
// ObjectTrack): the raw placement, the start of its movement segment,
// the fastest speed and the moving flag. The kite layer computes the
// conservative closed distance from it: a chaser whose broadcast
// lags may have closed speed*age units already, and the floor
// decisions only ever see a distance at or below the true gap.
func (b *Bot) ObjectTrackOf(objectID int32) (ObjectTrack, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)
    var track ObjectTrack
    if obj == nil {
        return track, false
    }
    track = ObjectTrack{
        X:       obj.X,
        Y:       obj.Y,
        Z:       obj.Z,
        MovedAt: time.Time{},
        Speed:   obj.chaseSpeed(),
        Moving:  obj.Moving,
    }
    if obj.Moving && obj.MoveAt != 0 {
        track.MovedAt = time.Unix(0, obj.MoveAt)
    }

    return track, true
}

// ObjectUsesBow reports whether a known npc carries a bow in its
// right hand: the NpcInfo equipment block names the template rhand
// item and the item stats resolve the weapon family. The hunt kite
// layer refuses to engage an archer enemy (the owner rule: a ranged
// opponent cannot be outrun, the standing trade answers it), so the
// gate reads this on every fight tick. Players never ride NpcInfo
// and unknown objects answer false.
func (b *Bot) ObjectUsesBow(objectID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)
    if obj == nil || obj.Kind != kindNPC {
        return false
    }

    return obj.BowWielding
}

// ObjectName returns the display name of a known object, an empty
// string when the object is unknown.
func (b *Bot) ObjectName(objectID int32) string {
    b.mu.RLock()
    defer b.mu.RUnlock()
    _, cold := b.objectLocked(objectID)
    if cold == nil {
        return ""
    }

    return cold.Name
}

// ObjectTemplateID returns the wire template id of a known npc (the
// NpcInfo template id, display id plus the 1000000 offset), zero when
// the object is unknown or not an npc. The spot respawn overlay keys
// the kill records through it onto the mob lists of the registry.
func (b *Bot) ObjectTemplateID(objectID int32) int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, cold := b.objectLocked(objectID)
    if obj == nil || cold == nil || obj.Kind != kindNPC {
        return 0
    }

    return cold.TemplateID
}

// ObjectAlive reports whether the object is known around the character
// and not dead.
func (b *Bot) ObjectAlive(objectID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)

    return obj != nil && !obj.Dead
}

// ObjectAttackable reports whether the object is a known living npc
// the server marked attackable (the NpcInfo flag of monsters; the
// villagers, guards and teachers carry false). The own object id and
// every object outside the known list answer false.
func (b *Bot) ObjectAttackable(objectID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)

    return obj != nil && obj.Kind == kindNPC && obj.Attackable && !obj.Dead
}

// ObjectLevel returns the observed level of a known object, false when
// the object is unknown or its level never arrived (a zero level of an
// unresolved template is indistinguishable and reports true, level 0 -
// the callers treat it as passable like the target search does).
func (b *Bot) ObjectLevel(objectID int32) (int32, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    obj, _ := b.objectLocked(objectID)
    if obj == nil {
        return 0, false
    }

    return obj.Level, true
}

// KnownObjectIDs lists the object ids of the currently known world (the
// known list of the session: npcs, players and ground items). The proxy
// snapshots it when a bot session ends and replays a DeleteObject for
// every entry before the resync onto the next session, so a held client
// never keeps ghosts of the old world. The ids are unordered.
func (b *Bot) KnownObjectIDs() []int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    ids := make([]int32, 0, len(b.world.hot))
    for i := range b.world.hot {
        ids = append(ids, b.world.hot[i].ObjectID)
    }

    return ids
}

// ObjectHealthPercent returns the HP of an observed object as a
// percentage of its maximum (0..100), -1 when the object or its
// vitals are unknown. The server refreshes the vitals of the mob the
// character attacks, so the value is exact where the hunt loop needs
// it: the current fight.
func (b *Bot) ObjectHealthPercent(objectID int32) float64 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    _, cold := b.objectLocked(objectID)
    if cold == nil || cold.MaxHP <= 0 {
        return -1
    }
    pct := cold.CurHP / cold.MaxHP * 100

    return math.Min(100, math.Max(0, pct))
}

// ObjectVitals returns the raw current and maximum HP of an observed
// object: the kill ETA input of the hunt loop (the damage rate needs
// the absolute numbers the percentage view loses). ok is false when
// the object or its maximum is unknown. The server refreshes the
// vitals of the mob the character attacks, so the values are exact
// where the hunt loop needs them: the current fight.
func (b *Bot) ObjectVitals(
    objectID int32,
) (curHp float64, maxHp float64, ok bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    _, cold := b.objectLocked(objectID)
    if cold == nil || cold.MaxHP <= 0 {
        return 0, 0, false
    }

    return cold.CurHP, cold.MaxHP, true
}
