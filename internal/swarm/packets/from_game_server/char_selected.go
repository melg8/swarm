// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"errors"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const charSelectedPacketID = 0x21

// CharSelectedPacket is sent as a response to character selection and moves
// the connection into the entering world state.
// Wire format: [opcode 0x21][name: str][objectID: 4][title: str]
// [sessionId: 4][clanId: 4][unknown: 4][sex: 4][race: 4][classId: 4]
// [active: 4][x: 4][y: 4][z: 4][curHp: 8][curMp: 8] plus a fixed tail.
type CharSelectedPacket struct {
	Name      string
	ObjectID  int32
	SessionID int32
	ClassID   int32
	X         int32
	Y         int32
	Z         int32
	CurrentHP float64
	CurrentMP float64
}

// NewCharSelectedPacket creates a zero valued packet ready for parsing.
func NewCharSelectedPacket() *CharSelectedPacket {
	return &CharSelectedPacket{
		Name:      "",
		ObjectID:  0,
		SessionID: 0,
		ClassID:   0,
		X:         0,
		Y:         0,
		Z:         0,
		CurrentHP: 0,
		CurrentMP: 0,
	}
}

// ParseCharSelectedPacket reads the packet from payload bytes. Fields after
// the class id are skipped because the client does not use them.
func ParseCharSelectedPacket(p *CharSelectedPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, charSelectedPacketID); err != nil {
		return err
	}

	var err error
	if p.Name, err = reader.ReadStringFromUtf16Format(); err != nil {
		return err
	}
	if p.ObjectID, err = reader.ReadInt32(); err != nil {
		return err
	}
	// Skip the title string.
	if _, err := reader.ReadStringFromUtf16Format(); err != nil {
		return err
	}
	if p.SessionID, err = reader.ReadInt32(); err != nil {
		return err
	}
	// Skip clanId, unknown, sex, race before the class id.
	if err := reader.Skip(4 * 4); err != nil {
		return errors.New("not enough bytes for char selected fields")
	}
	if p.ClassID, err = reader.ReadInt32(); err != nil {
		return err
	}
	// Skip the active flag before the position.
	if err := reader.Skip(4); err != nil {
		return errors.New("not enough bytes for char selected position")
	}
	if p.X, err = reader.ReadInt32(); err != nil {
		return err
	}
	if p.Y, err = reader.ReadInt32(); err != nil {
		return err
	}
	if p.Z, err = reader.ReadInt32(); err != nil {
		return err
	}
	if p.CurrentHP, err = reader.ReadFloat64(); err != nil {
		return err
	}
	p.CurrentMP, err = reader.ReadFloat64()

	return err
}
