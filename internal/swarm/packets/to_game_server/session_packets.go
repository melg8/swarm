// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const enterWorldPacketID = 0x03

// EnterWorld is sent after character selection to spawn the character in
// the game world.
// Wire format: [opcode 0x03].
type EnterWorld struct{}

func (p *EnterWorld) ToBytes(writer *packet.Writer) error {
	return writer.WriteInt8(enterWorldPacketID)
}

const requestNetPingPacketID byte = 0xA8

// RequestNetPing keeps the connection alive while in game.
// Wire format: [opcode 0xA8].
type RequestNetPing struct{}

func (p *RequestNetPing) ToBytes(writer *packet.Writer) error {
	id := requestNetPingPacketID

	return writer.WriteInt8(int8(id))
}

const logoutPacketID = 0x09

// Logout gracefully leaves the game world.
// Wire format: [opcode 0x09].
type Logout struct{}

func (p *Logout) ToBytes(writer *packet.Writer) error {
	return writer.WriteInt8(logoutPacketID)
}
