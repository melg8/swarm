// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const appearingPacketID = 0x30

// AppearingPacket confirms a finished teleport to the game server. The
// server keeps the character in the teleporting state after every
// TeleportToLocation until this packet arrives (the official client
// sends it right after the teleport screen closes): while the flag is
// set every move request is silently ignored by the character AI.
// Wire format (see Appearing.runImpl): [opcode 0x30], no body.
type AppearingPacket struct{}

// NewAppearingPacket creates the teleport confirmation.
func NewAppearingPacket() *AppearingPacket {
	return &AppearingPacket{}
}

// ToBytes serializes the packet.
func (p *AppearingPacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(appearingPacketID); err != nil {
		return fmt.Errorf("failed to write appearing id: %w", err)
	}

	return nil
}
