// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// Object position and removal packets: MoveToLocation, StopMove,
// ValidateLocation, DeleteObject, DropItem, SpawnItem and GetItem.

const (
	moveToLocationPacketID   = 0x01
	moveToPawnPacketID       = 0x75
	stopMovePacketID         = 0x59
	validateLocationPacketID = 0x76
	deleteObjectPacketID     = 0x1E
	dropItemPacketID         = 0x16
	spawnItemPacketID        = 0x15
	getItemPacketID          = 0x17
)

// MoveToLocationPacket announces an object movement.
// Wire format (see MoveToLocation.writeImpl): [opcode 0x01]
// [objectId: 4][destX: 4][destY: 4][destZ: 4][x: 4][y: 4][z: 4].
// Note that the destination is written before the current position.
type MoveToLocationPacket struct {
	ObjectID int32
	DestX    int32
	DestY    int32
	DestZ    int32
	X        int32
	Y        int32
	Z        int32
}

// NewMoveToLocationPacket creates a zero valued packet ready for parsing.
func NewMoveToLocationPacket() *MoveToLocationPacket {
	return &MoveToLocationPacket{
		ObjectID: 0,
		DestX:    0,
		DestY:    0,
		DestZ:    0,
		X:        0,
		Y:        0,
		Z:        0,
	}
}

// ParseMoveToLocationPacket reads the packet from payload bytes.
func ParseMoveToLocationPacket(p *MoveToLocationPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, moveToLocationPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader,
		&p.ObjectID, &p.DestX, &p.DestY, &p.DestZ, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read movement: %w", err)
	}

	return nil
}

// MoveToPawnPacket announces a creature chasing a target to a stop
// distance, used instead of MoveToLocation by attacking mobs.
// Wire format (see MoveToPawn.writeImpl): [opcode 0x75][objectId: 4]
// [targetId: 4][distance: 4][x: 4][y: 4][z: 4][targetX: 4][targetY: 4]
// [targetZ: 4].
type MoveToPawnPacket struct {
	ObjectID int32
	TargetID int32
	Distance int32
	X        int32
	Y        int32
	Z        int32
	TargetX  int32
	TargetY  int32
	TargetZ  int32
}

// NewMoveToPawnPacket creates a zero valued packet ready for parsing.
func NewMoveToPawnPacket() *MoveToPawnPacket {
	return &MoveToPawnPacket{
		ObjectID: 0,
		TargetID: 0,
		Distance: 0,
		X:        0,
		Y:        0,
		Z:        0,
		TargetX:  0,
		TargetY:  0,
		TargetZ:  0,
	}
}

// ParseMoveToPawnPacket reads the packet from payload bytes.
func ParseMoveToPawnPacket(p *MoveToPawnPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, moveToPawnPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader,
		&p.ObjectID, &p.TargetID, &p.Distance,
		&p.X, &p.Y, &p.Z, &p.TargetX, &p.TargetY, &p.TargetZ); err != nil {
		return fmt.Errorf("failed to read pawn movement: %w", err)
	}

	return nil
}

// StopMovePacket announces an object that stopped moving.
// Wire format (see StopMove.writeImpl): [opcode 0x59][objectId: 4]
// [x: 4][y: 4][z: 4][heading: 4].
type StopMovePacket struct {
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
	Heading  int32
}

// NewStopMovePacket creates a zero valued packet ready for parsing.
func NewStopMovePacket() *StopMovePacket {
	return &StopMovePacket{
		ObjectID: 0,
		X:        0,
		Y:        0,
		Z:        0,
		Heading:  0,
	}
}

// ParseStopMovePacket reads the packet from payload bytes.
func ParseStopMovePacket(p *StopMovePacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, stopMovePacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader,
		&p.ObjectID, &p.X, &p.Y, &p.Z, &p.Heading); err != nil {
		return fmt.Errorf("failed to read stop move: %w", err)
	}

	return nil
}

// ValidateLocationPacket corrects the position and heading of an object.
// Wire format (see ValidateLocation.writeImpl): [opcode 0x76]
// [objectId: 4][x: 4][y: 4][z: 4][heading: 4].
type ValidateLocationPacket struct {
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
	Heading  int32
}

// NewValidateLocationPacket creates a zero valued packet ready for parsing.
func NewValidateLocationPacket() *ValidateLocationPacket {
	return &ValidateLocationPacket{
		ObjectID: 0,
		X:        0,
		Y:        0,
		Z:        0,
		Heading:  0,
	}
}

// ParseValidateLocationPacket reads the packet from payload bytes.
func ParseValidateLocationPacket(p *ValidateLocationPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, validateLocationPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader,
		&p.ObjectID, &p.X, &p.Y, &p.Z, &p.Heading); err != nil {
		return fmt.Errorf("failed to read validate location: %w", err)
	}

	return nil
}

// DeleteObjectPacket removes an object from the observed world.
// Wire format (see DeleteObject.writeImpl): [opcode 0x1E][objectId: 4].
type DeleteObjectPacket struct {
	ObjectID int32
}

// NewDeleteObjectPacket creates a zero valued packet ready for parsing.
func NewDeleteObjectPacket() *DeleteObjectPacket {
	return &DeleteObjectPacket{ObjectID: 0}
}

// ParseDeleteObjectPacket reads the packet from payload bytes.
func ParseDeleteObjectPacket(p *DeleteObjectPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, deleteObjectPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read deleted object id: %w", err)
	}

	return nil
}

// DropItemPacket announces an item dropped on the ground.
// Wire format (see DropItem.writeImpl): [opcode 0x16][dropperId: 4]
// [objectId: 4][displayId: 4][x: 4][y: 4][z: 4][stackable: 4][count: 4]
// [unknown: 4].
type DropItemPacket struct {
	ObjectID   int32
	TemplateID int32
	Stackable  bool
	Count      int32
	X          int32
	Y          int32
	Z          int32
}

// NewDropItemPacket creates a zero valued packet ready for parsing.
func NewDropItemPacket() *DropItemPacket {
	return &DropItemPacket{
		ObjectID:   0,
		TemplateID: 0,
		Stackable:  false,
		Count:      0,
		X:          0,
		Y:          0,
		Z:          0,
	}
}

// ParseDropItemPacket reads the packet from payload bytes.
func ParseDropItemPacket(p *DropItemPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, dropItemPacketID); err != nil {
		return err
	}
	// Skip the dropper object id.
	if err := reader.Skip(4); err != nil {
		return fmt.Errorf("not enough bytes for dropper id: %w", err)
	}
	if err := readInt32Fields(reader,
		&p.ObjectID, &p.TemplateID, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read drop location: %w", err)
	}
	stackable, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read drop stackable flag: %w", err)
	}
	p.Stackable = stackable != 0
	if err := readInt32Fields(reader, &p.Count); err != nil {
		return fmt.Errorf("failed to read drop count: %w", err)
	}

	return nil
}

// SpawnItemPacket announces an item that already lies on the ground when
// it enters the known list of the character, unlike the DropItem packet
// for freshly dropped items. Without it, items that existed before the
// bot logged in would stay invisible.
// Wire format (see SpawnItem.writeImpl): [opcode 0x15][objectId: 4]
// [displayId: 4][x: 4][y: 4][z: 4][stackable: 4][count: 4].
type SpawnItemPacket struct {
	ObjectID   int32
	TemplateID int32
	Stackable  bool
	Count      int32
	X          int32
	Y          int32
	Z          int32
}

// NewSpawnItemPacket creates a zero valued packet ready for parsing.
func NewSpawnItemPacket() *SpawnItemPacket {
	return &SpawnItemPacket{
		ObjectID:   0,
		TemplateID: 0,
		Stackable:  false,
		Count:      0,
		X:          0,
		Y:          0,
		Z:          0,
	}
}

// ParseSpawnItemPacket reads the packet from payload bytes.
func ParseSpawnItemPacket(p *SpawnItemPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, spawnItemPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader,
		&p.ObjectID, &p.TemplateID, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read spawn item location: %w", err)
	}
	stackable, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read spawn item stackable flag: %w", err)
	}
	p.Stackable = stackable != 0
	if err := readInt32Fields(reader, &p.Count); err != nil {
		return fmt.Errorf("failed to read spawn item count: %w", err)
	}

	return nil
}

// GetItemPacket announces that a player picked up a ground item: the item
// disappears and the picker stands at the given position.
// Wire format (see GetItem.writeImpl): [opcode 0x17][playerId: 4]
// [objectId: 4][x: 4][y: 4][z: 4].
type GetItemPacket struct {
	PlayerID int32
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
}

// NewGetItemPacket creates a zero valued packet ready for parsing.
func NewGetItemPacket() *GetItemPacket {
	return &GetItemPacket{
		PlayerID: 0,
		ObjectID: 0,
		X:        0,
		Y:        0,
		Z:        0,
	}
}

// ParseGetItemPacket reads the packet from payload bytes.
func ParseGetItemPacket(p *GetItemPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, getItemPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(
		reader, &p.PlayerID, &p.ObjectID, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read get item: %w", err)
	}

	return nil
}
