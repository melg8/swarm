// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const charInfoPacketID = 0x03

// charInfoSkippedBytes skips the paperdoll, combat and appearance fields
// between the base class and the title string: 12 ints (unknown +
// paperdoll + unknown), 12 ints (attack stats, pvp, karma, speeds),
// 4 doubles (multipliers and collision) and 3 ints (hair, face).
const charInfoSkippedBytes = (12+12)*4 + 4*8 + 3*4

// CharInfoPacket describes another player around the character.
// Wire format (see CharInfo.writeImpl): [opcode 0x03][x: 4][y: 4][z: 4]
// [vehicleId: 4][objectId: 4][name: str][race: 4][female: 4]
// [baseClass: 4][140 bytes: paperdoll, combat, appearance]
// [title: str].
type CharInfoPacket struct {
	ObjectID int32
	Name     string
	Title    string
	Race     int32
	ClassID  int32
	X        int32
	Y        int32
	Z        int32
}

// NewCharInfoPacket creates a zero valued packet ready for parsing.
func NewCharInfoPacket() *CharInfoPacket {
	return &CharInfoPacket{
		ObjectID: 0,
		Name:     "",
		Title:    "",
		Race:     0,
		ClassID:  0,
		X:        0,
		Y:        0,
		Z:        0,
	}
}

// ParseCharInfoPacket reads the packet from payload bytes.
func ParseCharInfoPacket(p *CharInfoPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, charInfoPacketID); err != nil {
		return err
	}

	return parseCharIdentity(reader, p)
}

// parseCharIdentity reads the location and identity fields up to the
// base class, then the title behind the fixed appearance block.
func parseCharIdentity(reader *packet.Reader, p *CharInfoPacket) error {
	var err error
	if err = readInt32Fields(reader, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read char position: %w", err)
	}
	if p.Name, err = readCharName(reader, p); err != nil {
		return err
	}
	if p.Title, err = readCharTitle(reader); err != nil {
		return err
	}

	return nil
}

// readCharName reads the object id, name and class fields.
func readCharName(reader *packet.Reader, p *CharInfoPacket) (string, error) {
	// Skip the vehicle id before the object id.
	if err := reader.Skip(4); err != nil {
		return "", fmt.Errorf("not enough bytes for vehicle id: %w", err)
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return "", fmt.Errorf("failed to read char object id: %w", err)
	}
	name, err := reader.ReadStringFromUtf16Format()
	if err != nil {
		return "", fmt.Errorf("failed to read char name: %w", err)
	}
	if err := readInt32Fields(reader, &p.Race); err != nil {
		return "", fmt.Errorf("failed to read char race: %w", err)
	}
	// Skip the female field before the base class.
	if err := reader.Skip(4); err != nil {
		return "", fmt.Errorf("not enough bytes for char fields: %w", err)
	}
	if err := readInt32Fields(reader, &p.ClassID); err != nil {
		return "", fmt.Errorf("failed to read char class: %w", err)
	}

	return name, nil
}

// readCharTitle reads the title string behind the appearance block.
func readCharTitle(reader *packet.Reader) (string, error) {
	if err := reader.Skip(charInfoSkippedBytes); err != nil {
		return "", fmt.Errorf("not enough bytes for char fields: %w", err)
	}
	title, err := reader.ReadStringFromUtf16Format()
	if err != nil {
		return "", fmt.Errorf("failed to read char title: %w", err)
	}

	return title, nil
}
