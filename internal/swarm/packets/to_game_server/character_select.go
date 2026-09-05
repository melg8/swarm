// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const characterSelectPacketID = 0x0D

// CharacterSelect selects the character to enter the world with.
// Wire format: [opcode 0x0D][charSlot: 4].
type CharacterSelect struct {
	CharSlot int32
}

func (p *CharacterSelect) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(characterSelectPacketID); err != nil {
		return err
	}

	return writer.WriteInt32(p.CharSlot)
}
