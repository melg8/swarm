// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"github.com/melg8/swarm/internal/swarm/helpers"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

type GGAuthPacket struct {
	SessionID int32
	Unknown   int32
}

func NewGGAuthPacketFromBytes(data []byte) (*GGAuthPacket, error) {
	reader := packet.NewReader(data)
	packet := GGAuthPacket{}
	if err := packet.FromBytes(reader); err != nil {
		return nil, err
	}
	return &packet, nil
}

func (p *GGAuthPacket) FromBytes(reader *packet.Reader) error {
	sessionID, err := reader.ReadInt32()
	if err != nil {
		return err
	}
	unknown, err := reader.ReadInt32()
	if err != nil {
		return err
	}
	p.SessionID = sessionID
	p.Unknown = unknown

	return nil
}

func (p *GGAuthPacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt32(p.SessionID); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.Unknown); err != nil {
		return err
	}

	return nil
}

func (p *GGAuthPacket) ToString() string {
	return "\nGGAuthPacket:" +
		"\n  SessionID: " + helpers.HexStringFromInt32(p.SessionID) +
		"\n  Unknown: " + helpers.HexStringFromInt32(p.Unknown)
}
