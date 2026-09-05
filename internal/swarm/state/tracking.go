// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// Rotation describes a BeginRotation or StopRotation packet.
type Rotation struct {
	ObjectID int32
	Heading  int32
}

// MoveType describes a ChangeMoveType packet.
type MoveType struct {
	ObjectID int32
	Running  bool
}

// Teleport describes a TeleportToLocation packet.
type Teleport struct {
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
	Heading  int32
}

// ItemPickup describes a GetItem packet.
type ItemPickup struct {
	PlayerID int32
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
}

// WaitType describes a ChangeWaitType packet: the server announces the
// sit/stand transition of a creature with the position it happens at.
type WaitType struct {
	ObjectID int32
	Sitting  bool
}

// ApplyWaitType tracks the sit/stand state of the played character. The
// packet is broadcast through Player.broadcastPacket, so the acting
// client receives its own transitions (see Player.sitDown/standUp).
func (b *Bot) ApplyWaitType(w WaitType) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.ObjectID != b.selfID {
		return
	}
	b.char.Sitting = w.Sitting
	b.touch()
	switch {
	case w.Sitting:
		b.recordLocked("sat down to rest")
	default:
		b.recordLocked("stood up")
	}
}

// ApplyRotationStart turns an object to the heading of the rotation start.
// The server sends the pair of rotation packets when a standing player
// becomes visible, because CharInfo carries no heading, and on keyboard
// rotation of a visible player.
func (b *Bot) ApplyRotationStart(r Rotation) {
	b.applyRotation(r, false)
}

// ApplyRotationStop turns an object to the final heading of its rotation.
func (b *Bot) ApplyRotationStop(r Rotation) {
	b.applyRotation(r, true)
}

// applyRotation updates the heading of self or an object.
func (b *Bot) applyRotation(r Rotation, stop bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.ObjectID == b.selfID {
		b.char.Heading = r.Heading
		if stop {
			b.clearCharMovement()
		}
		b.touch()

		return
	}
	obj, ok := b.objects[r.ObjectID]
	if !ok {
		return
	}
	obj.Heading = r.Heading
	if stop {
		obj.Moving = false
		obj.DestX = obj.X
		obj.DestY = obj.Y
		obj.DestZ = obj.Z
	}
	obj.UpdatedAt = time.Now()
	b.objects[r.ObjectID] = obj
	b.touch()
}

// ApplySelfTarget records the target the server assigned to the played
// character (MyTargetSelected).
func (b *Bot) ApplySelfTarget(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.char.TargetID == objectID {
		return
	}
	b.char.TargetID = objectID
	name := b.objectNameLocked(objectID)
	if objectID == 0 {
		b.touch()
		b.recordLocked("target cleared")

		return
	}
	b.touch()
	b.recordLocked("target selected: " + name)
}

// ApplyObjectTarget records the target another visible player selected.
func (b *Bot) ApplyObjectTarget(objectID int32, targetID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, ok := b.objects[objectID]
	if !ok {
		return
	}
	obj.TargetID = targetID
	obj.UpdatedAt = time.Now()
	b.objects[objectID] = obj
	b.touch()
}

// ApplyTargetClear drops the target reference of a visible player. The
// packet also arrives for the played character itself: the server
// clears the character target (for example when the target object is
// removed from the world) and notifies everyone including the actor,
// while never sending MyTargetSelected for the removal.
func (b *Bot) ApplyTargetClear(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if objectID == b.selfID {
		b.clearSelfTargetLocked("target unselected")

		return
	}
	obj, ok := b.objects[objectID]
	if !ok {
		return
	}
	obj.TargetID = 0
	obj.UpdatedAt = time.Now()
	b.objects[objectID] = obj
	b.touch()
}

// ApplyMoveType tracks a creature switching between walking and running.
func (b *Bot) ApplyMoveType(mt MoveType) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, ok := b.objects[mt.ObjectID]
	if !ok {
		return
	}
	obj.Running = mt.Running
	obj.UpdatedAt = time.Now()
	b.objects[mt.ObjectID] = obj
	b.touch()
}

// ApplyTeleport snaps an object or the played character to a new place.
func (b *Bot) ApplyTeleport(t Teleport) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if t.ObjectID == b.selfID {
		b.char.X = t.X
		b.char.Y = t.Y
		b.char.Z = t.Z
		b.char.Heading = t.Heading
		b.clearCharMovement()
		b.touch()

		return
	}
	obj, ok := b.objects[t.ObjectID]
	if !ok {
		return
	}
	obj.X = t.X
	obj.Y = t.Y
	obj.Z = t.Z
	obj.Heading = t.Heading
	obj.Moving = false
	obj.DestX = t.X
	obj.DestY = t.Y
	obj.DestZ = t.Z
	obj.UpdatedAt = time.Now()
	b.objects[t.ObjectID] = obj
	b.touch()
}

// ApplySpawnItem upserts a ground item that already existed when it
// entered the known list of the character, for example after a relogin.
func (b *Bot) ApplySpawnItem(info ItemInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj := b.upsertLocked(info.ObjectID, KindItem)
	obj.TemplateID = info.TemplateID
	obj.Name = npcdata.ItemName(info.TemplateID)
	obj.Count = info.Count
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	obj.UpdatedAt = time.Now()
	b.objects[info.ObjectID] = obj
	b.touch()
	b.recordLocked("item appeared: " + itemName(obj.Name, info.TemplateID))
}

// ApplyItemPickup removes a picked up ground item. The GetItem packet
// carries the position of the item, not of the picker: the official
// client only uses it to animate the item flying into the inventory. The
// picker position is already tracked from the StopMove broadcast of the
// arrival, so snapping the picker here teleported it across the map on
// every pickup.
func (b *Bot) ApplyItemPickup(p ItemPickup) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, ok := b.objects[p.ObjectID]
	if !ok {
		return
	}
	name := itemName(obj.Name, obj.TemplateID)
	delete(b.objects, p.ObjectID)
	pickerName := ""
	if p.PlayerID == b.selfID {
		pickerName = "self"
	} else if picker, found := b.objects[p.PlayerID]; found {
		pickerName = picker.Name
	}
	b.touch()
	switch pickerName {
	case "self":
		b.recordLocked("picked up " + name)
	case "":
		b.recordLocked("item removed: " + name)
	default:
		b.recordLocked(pickerName + " picked up " + name)
	}
}

// objectNameLocked resolves the display name of an object id. The caller
// must hold the state lock.
func (b *Bot) objectNameLocked(objectID int32) string {
	if objectID == b.selfID {
		return b.char.Name
	}
	if obj, ok := b.objects[objectID]; ok && obj.Name != "" {
		return obj.Name
	}

	return "object " + strconv.Itoa(int(objectID))
}
