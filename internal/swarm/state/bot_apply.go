// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// SetCharacter applies the initial character state from CharSelected.
func (b *Bot) SetCharacter(
    name string, objectID int32, classID int32,
    x int32, y int32, z int32, curHP float64, curMP float64,
) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.char.Name = name
    b.selfID = objectID
    b.char.ClassID = classID
    b.char.X = x
    b.char.Y = y
    b.char.Z = z
    b.char.CurHP = curHP
    b.char.CurMP = curMP
    b.touch()
    b.recordLocked("character selected: " + name)
}

// UserInfo carries the observed self state from a UserInfo packet.
type UserInfo struct {
    Name               string
    Level              int32
    Race               int32
    ClassID            int32
    X                  int32
    Y                  int32
    Z                  int32
    STR                int32
    DEX                int32
    CON                int32
    INT                int32
    WIT                int32
    MEN                int32
    Exp                int32
    Sp                 int32
    MaxHP              int32
    CurHP              int32
    MaxMP              int32
    CurMP              int32
    CurrentLoad        int32
    MaxLoad            int32
    RunSpeed           int32
    WalkSpeed          int32
    MoveSpeedMult      float64
    PaperdollObjectIDs [PaperdollSlots]int32
}

// ApplyUserInfo updates the character state from a UserInfo packet.
func (b *Bot) ApplyUserInfo(info UserInfo) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.char.Name = info.Name
    b.char.Level = info.Level
    b.char.Race = info.Race
    b.char.ClassID = info.ClassID
    b.applySelfPositionSampleLocked(info.X, info.Y, info.Z, b.now())
    b.char.STR = info.STR
    b.char.DEX = info.DEX
    b.char.CON = info.CON
    b.char.INT = info.INT
    b.char.WIT = info.WIT
    b.char.MEN = info.MEN
    b.char.Exp = info.Exp
    b.char.Sp = info.Sp
    b.char.MaxHP = float64(info.MaxHP)
    b.char.CurHP = float64(info.CurHP)
    b.char.MaxMP = float64(info.MaxMP)
    b.char.CurMP = float64(info.CurMP)
    b.char.CurrentLoad = info.CurrentLoad
    b.char.MaxLoad = info.MaxLoad
    b.char.RunSpeed = effectiveSpeed(
        info.RunSpeed, info.WalkSpeed, info.MoveSpeedMult)
    b.char.WalkSpeed = effectiveSpeed(
        info.WalkSpeed, info.WalkSpeed, info.MoveSpeedMult)
    b.paperdoll = info.PaperdollObjectIDs
    b.touch()
}

// ApplyPlacement updates the position and heading of self or an object.
func (b *Bot) ApplyPlacement(p Placement) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if p.ObjectID == b.selfID {
        b.char.X = p.X
        b.char.Y = p.Y
        b.char.Z = p.Z
        b.char.Heading = p.Heading
        b.clearCharMovement()
        b.touch()

        return
    }
    obj, cold := b.objectLocked(p.ObjectID)
    if obj == nil {
        return
    }
    obj.X = p.X
    obj.Y = p.Y
    obj.Z = p.Z
    cold.Heading = p.Heading
    obj.Moving = p.Moving
    if !p.Moving {
        obj.DestX = p.X
        obj.DestY = p.Y
        obj.DestZ = p.Z
    }
    nowNano := b.now().UnixNano()
    obj.MoveAt = nowNano
    cold.UpdatedAt = nowNano
    b.touch()
}

// ApplyMovement updates the current position and destination of a moving
// object, computing the heading from the movement direction like the
// server does for its creatures. A zero distance packet is the arrival
// broadcast of the server: the object stands at the destination and
// keeps the heading it moved with, exactly like the official client
// renders it.
func (b *Bot) ApplyMovement(m Movement) {
    b.mu.Lock()
    defer b.mu.Unlock()
    arrived := m.DestX == m.X && m.DestY == m.Y && m.DestZ == m.Z
    if m.ObjectID == b.selfID {
        b.char.X = m.X
        b.char.Y = m.Y
        b.char.Z = m.Z
        if arrived {
            b.clearCharMovement()
        } else {
            b.char.Heading = HeadingFromDelta(
                m.DestX-m.X, m.DestY-m.Y)
            b.char.Moving = true
            b.char.DestX = m.DestX
            b.char.DestY = m.DestY
            b.char.DestZ = m.DestZ
            b.char.MoveAt = b.now()
        }
        b.touch()

        return
    }
    obj, cold := b.objectLocked(m.ObjectID)
    if obj == nil {
        return
    }
    obj.X = m.X
    obj.Y = m.Y
    obj.Z = m.Z
    if !arrived {
        cold.Heading = HeadingFromDelta(m.DestX-m.X, m.DestY-m.Y)
    }
    obj.DestX = m.DestX
    obj.DestY = m.DestY
    obj.DestZ = m.DestZ
    obj.Moving = !arrived
    nowNano := b.now().UnixNano()
    obj.MoveAt = nowNano
    cold.UpdatedAt = nowNano
    b.touch()
}

// ApplyPawnMovement updates a chasing object: it runs toward the point
// `distance` in front of its target and faces the target. The packet
// rides an attack chase, so it refreshes the combat state and the
// target reference - with one exception: the Mobius player AI moves
// the pawn toward a folk npc for the interact intention too (the walk
// to the ~36 unit talk distance), and a known non attackable npc pawn
// marks no combat (selecting a buffer or a shop is an interaction, not
// a fight). The played character is handled as well because the server
// broadcasts its own chase to it.
func (b *Bot) ApplyPawnMovement(m PawnMovement) {
    b.mu.Lock()
    defer b.mu.Unlock()
    now := b.now()
    destX, destY := pawnDestination(m)
    if m.ObjectID == b.selfID {
        b.applySelfPawnMovementLocked(m, destX, destY, now)
        b.touch()

        return
    }
    if m.TargetID == b.selfID {
        b.applySelfPositionSampleLocked(m.TargetX, m.TargetY, m.TargetZ, now)
        b.char.CombatUntil = now.Add(combatWindow)
        b.char.LastHitAt = now
        b.touch()
    }
    obj, cold := b.objectLocked(m.ObjectID)
    if obj == nil {
        return
    }
    obj.X = m.X
    obj.Y = m.Y
    obj.Z = m.Z
    obj.DestX = destX
    obj.DestY = destY
    obj.DestZ = m.TargetZ
    if m.X != m.TargetX || m.Y != m.TargetY {
        cold.Heading = HeadingFromDelta(m.TargetX-m.X, m.TargetY-m.Y)
    }
    obj.Moving = true
    obj.Running = true
    obj.TargetID = m.TargetID
    b.markObjectCombatLocked(obj, cold, now)
    obj.MoveAt = now.UnixNano()
    cold.UpdatedAt = now.UnixNano()
    b.touch()
}

// applySelfPawnMovementLocked tracks the played character chasing its
// attack target. A pawn movement toward a known non attackable npc is
// the interact walk of the Mobius player AI (the talk distance
// approach): it updates the target reference but marks no combat. The
// caller must hold the state write lock.
func (b *Bot) applySelfPawnMovementLocked(
    m PawnMovement, destX int32, destY int32, now time.Time,
) {
    b.char.X = m.X
    b.char.Y = m.Y
    b.char.Z = m.Z
    if m.X != m.TargetX || m.Y != m.TargetY {
        b.char.Heading = HeadingFromDelta(m.TargetX-m.X, m.TargetY-m.Y)
    }
    b.char.DestX = destX
    b.char.DestY = destY
    b.char.DestZ = m.TargetZ
    b.char.Moving = destX != m.X || destY != m.Y
    b.char.MoveAt = now
    b.char.TargetID = m.TargetID
    if obj, _ := b.objectLocked(m.TargetID); obj != nil &&
        obj.Kind == kindNPC && !obj.Attackable {
        return
    }
    b.char.FightingTargetID = m.TargetID
    b.noteSelfCombatLocked(now)
}

// pawnDestination computes the stop point of a chasing object.
func pawnDestination(m PawnMovement) (int32, int32) {
    dx := float64(m.X - m.TargetX)
    dy := float64(m.Y - m.TargetY)
    dist := math.Hypot(dx, dy)
    if dist < 1 {
        return m.X, m.Y
    }
    stopX := m.TargetX + int32(dx/dist*float64(m.Distance))
    stopY := m.TargetY + int32(dy/dist*float64(m.Distance))

    return stopX, stopY
}

// ApplyAttack updates the attacker placement and marks the attacker and
// every known hit target as fighting. The attacker faces its first hit
// target, exactly like the official client renders it: the Attack packet
// carries the attacker and the target positions but no heading, and the
// server set the attacker heading toward the target before broadcasting
// (see Creature.doAttack). When the played character is the target, the
// trailing target location doubles as a position update of it.
func (b *Bot) ApplyAttack(a Attack) {
    b.ApplyAttackAt(a, b.now())
}

// ApplyShotGauge records a bow shot cycle announced by the RED
// SetupGauge packet: the max field of the packet is the full disable
// window the server enforces from the shot commit, in milliseconds
// (see CharacterState.BowCycleAt). A non positive cycle and a gauge
// of any other color are ignored by the parser gate (IsBowCycle),
// so the caller passes only bow cycles.
func (b *Bot) ApplyShotGauge(cycle time.Duration) {
    b.ApplyShotGaugeAt(cycle, b.now())
}

// ApplyShotGaugeAt is the timestamped twin of ApplyShotGauge: the
// repro scenes stage a gauge a controlled age in the past.
func (b *Bot) ApplyShotGaugeAt(cycle time.Duration, at time.Time) {
    if cycle <= 0 {
        return
    }
    b.mu.Lock()
    defer b.mu.Unlock()
    b.char.BowCycleAt = at
    b.char.BowCycle = cycle
    b.touch()
}

// ApplyAttackAt is the timestamped twin of ApplyAttack: the packet
// lands with the given arrival time instead of the wall clock. The
// live session always feeds time.Now (through ApplyAttack); the
// timestamped form serves the repro scenes that stage a shot a
// controlled age in the past (the kite windup tests - the shot
// freshness and the fight freshness age at different rates, and a
// backdated broadcast pins the boundary between them).
func (b *Bot) ApplyAttackAt(a Attack, at time.Time) {
    b.mu.Lock()
    defer b.mu.Unlock()
    now := at
    facing := HeadingFromDelta(a.TargetX-a.X, a.TargetY-a.Y)
    hasFacing := a.TargetX != a.X || a.TargetY != a.Y
    if a.AttackerID == b.selfID {
        b.applySelfPositionSampleLocked(a.X, a.Y, a.Z, now)
        if hasFacing {
            b.char.Heading = facing
        }
        if a.TargetCount > 0 {
            b.char.TargetID = a.TargetIDs[0]
            b.char.FightingTargetID = a.TargetIDs[0]
        }
        b.char.LastSelfShotAt = now
        b.noteSelfCombatLocked(now)
        b.touch()
    } else if obj, cold := b.objectLocked(a.AttackerID); obj != nil {
        obj.X = a.X
        obj.Y = a.Y
        obj.Z = a.Z
        if hasFacing {
            cold.Heading = facing
        }
        if a.TargetCount > 0 {
            obj.TargetID = a.TargetIDs[0]
        }
        b.markObjectCombatLocked(obj, cold, now)
        cold.UpdatedAt = now.UnixNano()
        b.touch()
    }
    b.recordSwingEventsLocked(a, now)
    b.noteCritHintLocked(a, now)
    b.noteSwingsLocked(a)
    for i := range a.TargetCount {
        if a.TargetIDs[i] == b.selfID {
            b.applySelfPositionSampleLocked(
                a.TargetX, a.TargetY, a.TargetZ, now)
            b.char.CombatUntil = now.Add(combatWindow)
            b.char.LastHitAt = now
            b.touch()

            continue
        }
        if obj, cold := b.objectLocked(a.TargetIDs[i]); obj != nil {
            b.markObjectCombatLocked(obj, cold, now)
            cold.UpdatedAt = now.UnixNano()
        }
    }
}

// recordSwingEventsLocked feeds the swing animation of the web view
// with one event per hit: the blows that land record the swing, the
// evaded ones record the miss float (the viewer shows both, so a
// fight stays legible even when the blows start missing). The Mobius
// Attack packet carries the miss flag of every hit; a missed blow
// carries no swing streak and no damage number. The caller must hold
// the write lock.
func (b *Bot) recordSwingEventsLocked(a Attack, now time.Time) {
    for i := range a.TargetCount {
        if a.HitFlags[i]&attackHitMissFlag != 0 {
            //nolint:exhaustruct_v5 // the ring assigns Seq, a miss
            // carries no amount
            b.recordCombatEventLocked(CombatEvent{
                Kind:       CombatEventMiss,
                AttackerID: a.AttackerID,
                TargetID:   a.TargetIDs[i],
                X:          a.X,
                Y:          a.Y,
                TargetX:    a.TargetX,
                TargetY:    a.TargetY,
                At:         now,
            })

            continue
        }
        if a.AttackerID == b.selfID {
            // A landed blow of the played character: the deleveling
            // fight stage reads the pair to tell a guard that ignores
            // real damage from a stage without any landed blow yet.
            b.char.LastLandedHitAt = now
            b.char.LastLandedHitTarget = a.TargetIDs[i]
        }
        //nolint:exhaustruct_v5 // the ring assigns Seq, a swing
        // carries no amount
        b.recordCombatEventLocked(CombatEvent{
            Kind:       CombatEventAttack,
            AttackerID: a.AttackerID,
            TargetID:   a.TargetIDs[i],
            X:          a.X,
            Y:          a.Y,
            TargetX:    a.TargetX,
            TargetY:    a.TargetY,
            At:         now,
        })
    }
}

// critHintWindow bounds the critical correlation: the Attack
// broadcast and its StatusUpdate HP drops ride the same read loop
// back to back, so half a second is generous while the false
// positives (an unrelated hit on the same victim inside the window)
// stay rare. The wrong "Crit!" label is cosmetic only.
const critHintWindow = 500 * time.Millisecond

// noteCritHintLocked remembers the victims of the landed critical
// blows of an Attack broadcast (the hint the next HP drops read): a
// dual weapon critting twice counts two awaited drops. The caller
// must hold the write lock.
func (b *Bot) noteCritHintLocked(a Attack, now time.Time) {
    for i := range a.TargetCount {
        if a.CritFlags[i] && a.HitFlags[i]&attackHitMissFlag == 0 {
            if b.critVictim != a.TargetIDs[i] || b.critVictimAt.IsZero() ||
                now.Sub(b.critVictimAt) > critHintWindow {
                b.critVictim = a.TargetIDs[i]
                b.critVictimAt = now
                b.critVictimHits = 0
            }
            b.critVictimHits++
        }
    }
}

// critHintLocked reports whether the observed HP drop of the given
// victim labels as a critical blow: the hint matches the victim, the
// window is live and an awaited drop count remains (each labeled
// drop consumes one - a plain follow-up hit of the same victim stays
// plain). The caller must hold the write lock.
// Known trade-off of the one slot: a MIXED broadcast (a plain hit
// followed by a crit on the same victim) lets the plain hit's drop
// consume the label first - the two floats swap their labels, which
// is cosmetic only.
func (b *Bot) critHintLocked(victimID int32, now time.Time) bool {
    if b.critVictim != victimID || b.critVictimAt.IsZero() ||
        b.critVictimHits <= 0 {
        return false
    }
    if now.Sub(b.critVictimAt) > critHintWindow {
        b.critVictim = 0
        b.critVictimHits = 0

        return false
    }
    b.critVictimHits--

    return true
}

// ApplyAutoAttackStart marks an object as auto attacking.
func (b *Bot) ApplyAutoAttackStart(objectID int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    now := b.now()
    if objectID == b.selfID {
        b.char.AutoAttacking = true
        b.char.CombatUntil = now.Add(combatWindow)
        b.touch()

        return
    }
    obj, cold := b.objectLocked(objectID)
    if obj == nil {
        return
    }
    obj.AutoAttacking = true
    b.markObjectCombatLocked(obj, cold, now)
    cold.UpdatedAt = now.UnixNano()
    b.touch()
}

// ApplyAutoAttackStop clears the auto attack flag of an object.
func (b *Bot) ApplyAutoAttackStop(objectID int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if objectID == b.selfID {
        b.char.AutoAttacking = false
        b.touch()

        return
    }
    obj, cold := b.objectLocked(objectID)
    if obj == nil {
        return
    }
    obj.AutoAttacking = false
    cold.UpdatedAt = b.now().UnixNano()
    b.touch()
}

// ApplyNpcInfo upserts an observed npc object.
func (b *Bot) ApplyNpcInfo(info NpcInfo) {
    b.mu.Lock()
    defer b.mu.Unlock()
    obj, cold := b.upsertLocked(info.ObjectID, KindNPC)
    cold.TemplateID = info.TemplateID
    obj.Attackable = info.Attackable
    obj.Aggressive = npcdata.NPCIsAggressive(info.TemplateID)
    // The enemy weapon family: the NpcInfo equipment block names the
    // template rhand item, and the anti-archer gate of the hunt kite
    // layer refuses to engage a bow wielding enemy (a ranged
    // opponent cannot be outrun, the standing trade answers it).
    obj.BowWielding = npcBowWielding(info.RightHandItem)
    cold.AggroRange = npcdata.NPCAggroRange(info.TemplateID)
    obj.Level = npcdata.NPCLevel(info.TemplateID)
    obj.ClanHelpRange = npcdata.NPCClanHelpRange(info.TemplateID)
    obj.ClanMask = npcdata.NPCClanMask(info.TemplateID)
    obj.X = info.X
    obj.Y = info.Y
    obj.Z = info.Z
    cold.Heading = info.Heading
    obj.RunSpeed = info.RunSpeed
    obj.WalkSpeed = info.WalkSpeed
    obj.MoveSpeedMult = info.MoveSpeedMult
    cold.CollisionRadius = info.CollisionRadius
    obj.Running = info.Running
    obj.Moving = false
    obj.DestX = info.X
    obj.DestY = info.Y
    obj.DestZ = info.Z
    obj.Dead = info.Dead
    cold.Name = resolveNpcName(info.Name, info.TemplateID)
    cold.Title = info.Title
    now := b.now()
    if info.InCombat {
        b.markObjectCombatLocked(obj, cold, now)
    }
    obj.MoveAt = now.UnixNano()
    cold.UpdatedAt = now.UnixNano()
    b.touch()
    b.recordLocked("npc spawned: " + cold.Name)
}

// ApplyPlayerInfo upserts an observed player object.
func (b *Bot) ApplyPlayerInfo(info PlayerInfo) {
    b.mu.Lock()
    defer b.mu.Unlock()
    obj, cold := b.upsertLocked(info.ObjectID, KindPlayer)
    cold.Name = info.Name
    cold.Title = info.Title
    obj.X = info.X
    obj.Y = info.Y
    obj.Z = info.Z
    obj.RunSpeed = info.RunSpeed
    obj.WalkSpeed = info.WalkSpeed
    obj.MoveSpeedMult = info.MoveSpeedMult
    cold.CollisionRadius = info.CollisionRadius
    obj.Running = info.Running
    obj.Dead = info.Dead
    cold.Sitting = info.Sitting
    obj.Moving = false
    obj.DestX = info.X
    obj.DestY = info.Y
    obj.DestZ = info.Z
    now := b.now()
    if info.InCombat {
        b.markObjectCombatLocked(obj, cold, now)
    }
    obj.MoveAt = now.UnixNano()
    cold.UpdatedAt = now.UnixNano()
    b.touch()
    b.recordLocked("player appeared: " + info.Name)
}

// ApplyItemInfo upserts an observed ground item object.
func (b *Bot) ApplyItemInfo(info ItemInfo) {
    b.mu.Lock()
    defer b.mu.Unlock()
    obj, cold := b.upsertLocked(info.ObjectID, KindItem)
    cold.TemplateID = info.TemplateID
    cold.Name = npcdata.ItemName(info.TemplateID)
    cold.Count = info.Count
    obj.X = info.X
    obj.Y = info.Y
    obj.Z = info.Z
    cold.UpdatedAt = b.now().UnixNano()
    b.touch()
    b.recordLocked("item dropped: " + itemName(cold.Name, info.TemplateID))
}

// RemoveObject deletes an object that left the known list. When the
// bot itself targeted the object, the target is dropped as well: the
// server answers the removal only with the DeleteObject broadcast.
func (b *Bot) RemoveObject(objectID int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    _, cold := b.world.lookupLocked(objectID)
    if cold == nil {
        return
    }
    name := cold.Name
    b.removeObjectAtLocked(b.world.slotLocked(objectID), objectID)
    if b.char.TargetID == objectID {
        b.clearSelfTargetLocked("target object removed")
    }
    b.touch()
    b.recordLocked("object removed: " + name)
}

// Status attribute ids from the Mobius StatusUpdate packet.
const (
    AttrLevel   = 0x01
    AttrCurHP   = 0x09
    AttrMaxHP   = 0x0A
    AttrCurMP   = 0x0B
    AttrMaxMP   = 0x0C
    AttrCurLoad = 0x0E
    AttrMaxLoad = 0x0F
    // AttrAtkSpd is the ATK_SPD attribute: the live physical attack
    // speed of the character, rebroadcast on every equipment or buff
    // change of it. The bow fight reads it for the cooldown window
    // (the C1 formulas: 500000/pAtkSpd + reuseDelay*333/pAtkSpd).
    AttrAtkSpd = 0x12
)

// ApplyStatusUpdate applies vitals attribute changes to self or an object.
func (b *Bot) ApplyStatusUpdate(objectID int32, attrs []Attribute) {
    b.mu.Lock()
    defer b.mu.Unlock()
    now := b.now()
    if objectID == b.charObjectID() {
        b.applySelfStatusLocked(attrs, now)

        return
    }
    obj, cold := b.objectLocked(objectID)
    if obj == nil {
        return
    }
    wasDead := obj.Dead
    b.recordObjectDamageLocked(obj, cold, objectID, attrs, now)
    for _, attr := range attrs {
        switch attr.ID {
        case AttrCurHP:
            cold.CurHP = float64(attr.Value)
            obj.Dead = attr.Value <= 0
        case AttrMaxHP:
            cold.MaxHP = float64(attr.Value)
        case AttrCurMP:
            cold.CurMP = float64(attr.Value)
        case AttrMaxMP:
            cold.MaxMP = float64(attr.Value)
        }
    }
    cold.UpdatedAt = now.UnixNano()
    if obj.Dead && !wasDead {
        b.noteKillLocked(objectID, now)
    }
    if obj.Dead && b.char.TargetID == objectID {
        // A killed target is no target anymore: the server keeps
        // the corpse selected, the tracker drops it so the HUD
        // and the hunt loop see the actual "no target" state.
        b.clearSelfTargetLocked("target died")
    }
    b.touch()
}

// applyCharAttr applies a single attribute to the played character.
func (b *Bot) applyCharAttr(attr Attribute) {
    switch attr.ID {
    case AttrCurHP:
        b.char.CurHP = float64(attr.Value)
    case AttrMaxHP:
        b.char.MaxHP = float64(attr.Value)
    case AttrCurMP:
        b.char.CurMP = float64(attr.Value)
    case AttrMaxMP:
        b.char.MaxMP = float64(attr.Value)
    case AttrLevel:
        b.char.Level = attr.Value
    case AttrCurLoad:
        b.char.CurrentLoad = attr.Value
    case AttrMaxLoad:
        b.char.MaxLoad = attr.Value
    case AttrAtkSpd:
        b.char.PAtkSpd = attr.Value
    }
}

// applySelfStatusLocked applies the vitals attributes of the played
// character: the damage bookkeeping first (it reads the previous HP),
// then the values, and the alive to dead transition counts one death
// (exactly one per demise - the village restart that follows revives
// without counting, see metrics.go). The caller must hold the write
// lock.
func (b *Bot) applySelfStatusLocked(attrs []Attribute, now time.Time) {
    wasDead := b.char.MaxHP > 0 && b.char.CurHP <= 0
    b.recordCharDamageLocked(attrs, now)
    for _, attr := range attrs {
        b.applyCharAttr(attr)
    }
    if !wasDead && b.char.MaxHP > 0 && b.char.CurHP <= 0 {
        b.noteDeathLocked(now)
    }
    b.touch()
}
