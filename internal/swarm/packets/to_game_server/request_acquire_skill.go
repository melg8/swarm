// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestAcquireSkillPacketID = 0x6C

// RequestAcquireSkillPacket learns one lesson of a class skill tree
// at the skill teacher: the SP cost is charged and the required skill
// book consumed when the character carries it (see
// RequestAcquireSkill.checkPlayerSkill). The server resolves the
// trainer through the last folk NPC the character talked to - the
// Action click on the trainer - and refuses the lesson when the
// character stands outside the interaction distance (250 units), the
// level unlock is not reached, the SP falls short or the required
// item is missing; a refusal answers silently or with a chat message,
// so the caller re-checks its own budgets and watches the SkillList
// answer instead. The C1 server always reads the skill type as CLASS
// - the fishing and pledge acquire types of the later chronicles do
// not exist on the wire here.
// Wire format (see RequestAcquireSkill.readImpl): [opcode 0x6C]
// [skillId: 4][skillLevel: 4].
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
