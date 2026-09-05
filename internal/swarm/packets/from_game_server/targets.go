// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// Target packets: MyTargetSelected, TargetSelected and TargetUnselected.

const (
	myTargetSelectedPacketID = 0xBF
	targetSelectedPacketID   = 0x39
	targetUnselectedPacketID = 0x3A
)

// MyTargetSelectedPacket tells the client which object it now targets.
// The server sends it whenever the own target changes, for example after
// an AttackRequest that first selects the target.
// Wire format (see MyTargetSelected.writeImpl): [opcode 0xBF]
// [objectId: 4][color: 2].
type MyTargetSelectedPacket struct {
	ObjectID int32
}

// NewMyTargetSelectedPacket creates a zero valued packet ready for
// parsing.
func NewMyTargetSelectedPacket() *MyTargetSelectedPacket {
	return &MyTargetSelectedPacket{ObjectID: 0}
}

// ParseMyTargetSelectedPacket reads the packet from payload bytes.
func ParseMyTargetSelectedPacket(p *MyTargetSelectedPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, myTargetSelectedPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read selected object id: %w", err)
	}
	// Skip the target color field.
	if err := reader.Skip(2); err != nil {
		return fmt.Errorf("not enough bytes for target color: %w", err)
	}

	return nil
}

// TargetSelectedPacket announces that another visible player targeted an
// object.
// Wire format (see TargetSelected.writeImpl): [opcode 0x39][objectId: 4]
// [targetId: 4][x: 4][y: 4][z: 4][unknown: 4].
type TargetSelectedPacket struct {
	ObjectID int32
	TargetID int32
	X        int32
	Y        int32
	Z        int32
}

// NewTargetSelectedPacket creates a zero valued packet ready for parsing.
func NewTargetSelectedPacket() *TargetSelectedPacket {
	return &TargetSelectedPacket{
		ObjectID: 0,
		TargetID: 0,
		X:        0,
		Y:        0,
		Z:        0,
	}
}

// ParseTargetSelectedPacket reads the packet from payload bytes.
func ParseTargetSelectedPacket(p *TargetSelectedPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, targetSelectedPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(
		reader, &p.ObjectID, &p.TargetID, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read target selection: %w", err)
	}
	// Skip the unknown trailing int.
	if err := reader.Skip(4); err != nil {
		return fmt.Errorf("not enough bytes for target selection: %w", err)
	}

	return nil
}

// TargetUnselectedPacket announces that a visible player dropped its
// target.
// Wire format (see TargetUnselected.writeImpl): [opcode 0x3A]
// [objectId: 4][x: 4][y: 4][z: 4][unknown: 4].
type TargetUnselectedPacket struct {
	ObjectID int32
}

// NewTargetUnselectedPacket creates a zero valued packet ready for
// parsing.
func NewTargetUnselectedPacket() *TargetUnselectedPacket {
	return &TargetUnselectedPacket{ObjectID: 0}
}

// ParseTargetUnselectedPacket reads the packet from payload bytes.
func ParseTargetUnselectedPacket(p *TargetUnselectedPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, targetUnselectedPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read unselected object id: %w", err)
	}
	// Skip the position and the unknown trailing ints.
	if err := reader.Skip(16); err != nil {
		return fmt.Errorf("not enough bytes for target unselection: %w", err)
	}

	return nil
}
