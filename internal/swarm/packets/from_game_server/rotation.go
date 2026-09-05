// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// Rotation and movement mode packets: BeginRotation, StopRotation,
// ChangeMoveType and TeleportToLocation.

const (
	beginRotationPacketID      = 0x77
	stopRotationPacketID       = 0x78
	changeMoveTypePacketID     = 0x3E
	teleportToLocationPacketID = 0x38
)

// moveTypeRun marks a running creature in ChangeMoveType.
const moveTypeRun = 1

// BeginRotationPacket announces that an object started turning in place.
// The server also sends it together with StopRotation when a standing
// player becomes visible, because CharInfo carries no heading (see
// Player.sendInfo of the Mobius server).
// Wire format (see StartRotation.writeImpl): [opcode 0x77][objectId: 4]
// [heading: 4][side: 4][speed: 4].
type BeginRotationPacket struct {
	ObjectID int32
	Heading  int32
}

// NewBeginRotationPacket creates a zero valued packet ready for parsing.
func NewBeginRotationPacket() *BeginRotationPacket {
	return &BeginRotationPacket{ObjectID: 0, Heading: 0}
}

// ParseBeginRotationPacket reads the packet from payload bytes.
func ParseBeginRotationPacket(p *BeginRotationPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, beginRotationPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID, &p.Heading); err != nil {
		return fmt.Errorf("failed to read rotation start: %w", err)
	}
	// Skip the side and the speed fields.
	if err := reader.Skip(8); err != nil {
		return fmt.Errorf("not enough bytes for rotation start: %w", err)
	}

	return nil
}

// StopRotationPacket announces the final heading of a turned object.
// Wire format (see StopRotation.writeImpl): [opcode 0x78][objectId: 4]
// [heading: 4][speed: 4][unknown: 1].
type StopRotationPacket struct {
	ObjectID int32
	Heading  int32
}

// NewStopRotationPacket creates a zero valued packet ready for parsing.
func NewStopRotationPacket() *StopRotationPacket {
	return &StopRotationPacket{ObjectID: 0, Heading: 0}
}

// ParseStopRotationPacket reads the packet from payload bytes.
func ParseStopRotationPacket(p *StopRotationPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, stopRotationPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID, &p.Heading); err != nil {
		return fmt.Errorf("failed to read rotation stop: %w", err)
	}
	// Skip the speed field and the unknown byte.
	if err := reader.Skip(5); err != nil {
		return fmt.Errorf("not enough bytes for rotation stop: %w", err)
	}

	return nil
}

// ChangeMoveTypePacket announces that a creature switched between walking
// and running, which halves the interpolation speed of walkers.
// Wire format (see ChangeMoveType.writeImpl): [opcode 0x3E][objectId: 4]
// [type: 4] with type 0 for walking and 1 for running.
type ChangeMoveTypePacket struct {
	ObjectID int32
	Running  bool
}

// NewChangeMoveTypePacket creates a zero valued packet ready for parsing.
func NewChangeMoveTypePacket() *ChangeMoveTypePacket {
	return &ChangeMoveTypePacket{ObjectID: 0, Running: false}
}

// ParseChangeMoveTypePacket reads the packet from payload bytes.
func ParseChangeMoveTypePacket(p *ChangeMoveTypePacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, changeMoveTypePacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read move type object id: %w", err)
	}
	moveType, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read move type: %w", err)
	}
	p.Running = moveType == moveTypeRun

	return nil
}

// TeleportToLocationPacket snaps an object to a new position.
// Wire format (see TeleportToLocation.writeImpl): [opcode 0x38]
// [objectId: 4][x: 4][y: 4][z: 4][fade: 4][heading: 4].
type TeleportToLocationPacket struct {
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
	Heading  int32
}

// NewTeleportToLocationPacket creates a zero valued packet ready for
// parsing.
func NewTeleportToLocationPacket() *TeleportToLocationPacket {
	return &TeleportToLocationPacket{
		ObjectID: 0,
		X:        0,
		Y:        0,
		Z:        0,
		Heading:  0,
	}
}

// ParseTeleportToLocationPacket reads the packet from payload bytes.
func ParseTeleportToLocationPacket(
	p *TeleportToLocationPacket, data []byte,
) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, teleportToLocationPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(
		reader, &p.ObjectID, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read teleport location: %w", err)
	}
	// Skip the fade field before the heading.
	if err := reader.Skip(4); err != nil {
		return fmt.Errorf("not enough bytes for teleport heading: %w", err)
	}
	if err := readInt32Fields(reader, &p.Heading); err != nil {
		return fmt.Errorf("failed to read teleport heading: %w", err)
	}

	return nil
}
