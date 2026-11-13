// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestAcquireSkillPacketID = 0x6C

// RequestAcquireSkillPacket learns one lesson of a class skill tree at the
// skill teacher (the last folk npc the character talked to): the server
// charges the SP and consumes the required book, and refuses silently or with
// a chat message when the character stands outside the 250 unit interaction
// distance, the level unlock is not reached, the SP falls short or the book
// is missing, so the caller re-checks its budgets and watches the SkillList
// answer (C1 always reads the skill type as CLASS).
// Wire format: [opcode 0x6C][skillId: 4][skillLevel: 4].
type RequestAcquireSkillPacket struct {
    SkillID int32
    Level   int32
}

// NewRequestAcquireSkillPacket creates a zero valued acquire request.
func NewRequestAcquireSkillPacket() *RequestAcquireSkillPacket {
    return &RequestAcquireSkillPacket{
        SkillID: 0,
        Level:   0,
    }
}

// ToBytes serializes the packet.
func (p *RequestAcquireSkillPacket) ToBytes(
    writer *packet.Writer,
) error {
    if err := writer.WriteInt8(requestAcquireSkillPacketID); err != nil {
        return fmt.Errorf(
            "failed to write acquire skill id: %w", err)
    }
    if err := writer.WriteInt32(p.SkillID); err != nil {
        return fmt.Errorf(
            "failed to write acquire skill id: %w", err)
    }
    if err := writer.WriteInt32(p.Level); err != nil {
        return fmt.Errorf(
            "failed to write acquire skill level: %w", err)
    }

    return nil
}
