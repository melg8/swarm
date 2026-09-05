// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	charCreateOkPacketID   = 0x25
	charCreateFailPacketID = 0x26
)

// CharCreateOkPacket confirms a successful character creation.
// Wire format: [opcode 0x25][1: 4].
type CharCreateOkPacket struct{}

// NewCharCreateOkPacket creates the confirmation packet value.
func NewCharCreateOkPacket() *CharCreateOkPacket {
	return &CharCreateOkPacket{}
}

// ParseCharCreateOkPacket reads the packet from payload bytes.
func ParseCharCreateOkPacket(data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, charCreateOkPacketID); err != nil {
		return err
	}

	value, err := reader.ReadInt32()
	if err != nil {
		return err
	}
	if value != 1 {
		return fmt.Errorf("unexpected char create ok value %d", value)
	}

	return nil
}

// CharCreateFailPacket reports a failed character creation.
// Wire format: [opcode 0x26][reason: 4].
type CharCreateFailPacket struct {
	Reason int32
}

// NewCharCreateFailPacket creates a zero valued failure packet ready for
// parsing.
func NewCharCreateFailPacket() *CharCreateFailPacket {
	return &CharCreateFailPacket{Reason: 0}
}

// ParseCharCreateFailPacket reads the packet from payload bytes.
func ParseCharCreateFailPacket(p *CharCreateFailPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, charCreateFailPacketID); err != nil {
		return err
	}

	var err error
	if p.Reason, err = reader.ReadInt32(); err != nil {
		return err
	}

	return nil
}

// ReasonText returns a human readable creation failure reason.
func (p *CharCreateFailPacket) ReasonText() string {
	switch p.Reason {
	case 0x00:
		return "creation failed"
	case 0x01:
		return "too many characters"
	case 0x02:
		return "name already exists"
	case 0x03:
		return "name exceeds 16 characters"
	case 0x04:
		return "incorrect name"
	case 0x05:
		return "creation not allowed on this server"
	case 0x06:
		return "choose another server"
	default:
		return fmt.Sprintf("unknown reason %d", p.Reason)
	}
}
