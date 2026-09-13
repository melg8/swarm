// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const changeMoveTypePacketID = 0x1C

// ChangeMoveTypePacket asks the game server to switch the character
// between walking and running, exactly like the client movement
// toggle of the official UI. The server starts every session in the
// WALK state (Creature._isRunning defaults to false), so the first
// long walk of a bot that never sends this packet moves at the walk
// speed - the live class transfer run of 2026-09-12 spent four
// rounds dying in the Ruins of Agony before the 97 unit run speed
// read for what it was.
// Wire format (see ChangeMoveType2.readImpl): [opcode 0x1C]
// [typeRun: 4] (1 runs, 0 walks).
type ChangeMoveTypePacket struct {
	TypeRun int32
}

// NewChangeMoveTypePacket creates a zero valued (walk) move type
// request.
func NewChangeMoveTypePacket() *ChangeMoveTypePacket {
	return &ChangeMoveTypePacket{TypeRun: 0}
}

// ToBytes serializes the packet.
func (p *ChangeMoveTypePacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(changeMoveTypePacketID); err != nil {
		return fmt.Errorf("failed to write change move type id: %w", err)
	}
	if err := writer.WriteInt32(p.TypeRun); err != nil {
		return fmt.Errorf("failed to write change move type flag: %w", err)
	}

	return nil
}
