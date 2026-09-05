// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"errors"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const playOkPacketID = 0x07

// PlayOkPacket allows the client to connect to the selected game server.
// Wire format: [opcode 0x07][playOkID1: 4][playOkID2: 4].
type PlayOkPacket struct {
	PlayOkID1 int32
	PlayOkID2 int32
}

// NewPlayOkPacket creates a zero valued play ok packet ready for
// parsing.
func NewPlayOkPacket() *PlayOkPacket {
	return &PlayOkPacket{PlayOkID1: 0, PlayOkID2: 0}
}

// ParsePlayOkPacket reads the packet from decrypted content bytes.
func ParsePlayOkPacket(p *PlayOkPacket, data []byte) error {
	reader := packet.NewReader(data)

	id, err := reader.ReadInt8()
	if err != nil {
		return err
	}
	if id != playOkPacketID {
		return errors.New("invalid play ok packet id")
	}

	if p.PlayOkID1, err = reader.ReadInt32(); err != nil {
		return err
	}
	if p.PlayOkID2, err = reader.ReadInt32(); err != nil {
		return err
	}

	return nil
}
