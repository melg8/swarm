// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package togameserver contains serializers for client to game server
// packets of the Mobius C1 protocol.
package togameserver

import (
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// C1ProtocolVersion is the protocol revision accepted by the C1 server.
const C1ProtocolVersion = 419

const protocolVersionPacketID = 0x00

// ProtocolVersion is the first packet sent by the client. It is transferred
// unencrypted.
// Wire format: [opcode 0x00][version: 4].
type ProtocolVersion struct {
	Version int32
}

// NewProtocolVersion creates the packet with the C1 protocol version.
func NewProtocolVersion() *ProtocolVersion {
	return &ProtocolVersion{Version: C1ProtocolVersion}
}

func (p *ProtocolVersion) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(protocolVersionPacketID); err != nil {
		return err
	}

	return writer.WriteInt32(p.Version)
}
