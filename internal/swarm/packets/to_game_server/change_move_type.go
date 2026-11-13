// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const changeMoveTypePacketID = 0x1C

// ChangeMoveTypePacket asks the server to switch the character between
// walking and running, like the official UI toggle. The server starts
// every session in the WALK state (Creature._isRunning defaults to
// false), so a bot that never sends this walks the first long walk at
// the walk speed; the elven fighter run speed reads 97 units. Wire
// (ChangeMoveType2.readImpl): [opcode 0x1C][typeRun: 4] (1 run).
type ChangeMoveTypePacket struct {
    TypeRun int32
}

// NewChangeMoveTypePacket creates a zero valued (walk) move type request.
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
