// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestMagicSkillUsePacketID = 0x2F

// RequestMagicSkillUsePacket casts a learned active skill: a strike
// at the selected target, a self buff on the caster, a heal. The
// server resolves the skill level through the known skill list of the
// character and walks the cast flow (the range check, the mana cost,
// the reuse delay) itself; the answers are the MagicSkillUse
// broadcast with the setup cast bar and the ActionFailed refusals
// the cast re-requests retry. A ctrl pressed cast forces the
// attackable flag, a shift pressed cast keeps the current target
// selection - neither matters to the bot casts, both stay off.
// Wire format (see RequestMagicSkillUse.readImpl): [opcode 0x2F]
// [magicId: 4][ctrlPressed: 4][shiftPressed: 1].
type RequestMagicSkillUsePacket struct {
	SkillID int32
	Ctrl    bool
	Shift   bool
}

// NewRequestMagicSkillUsePacket creates a zero valued cast request.
func NewRequestMagicSkillUsePacket() *RequestMagicSkillUsePacket {
	return &RequestMagicSkillUsePacket{
		SkillID: 0,
		Ctrl:    false,
		Shift:   false,
	}
}

// ToBytes serializes the packet.
func (p *RequestMagicSkillUsePacket) ToBytes(
	writer *packet.Writer,
) error {
	if err := writer.WriteInt8(requestMagicSkillUsePacketID); err != nil {
		return fmt.Errorf(
			"failed to write magic skill use id: %w", err)
	}
	if err := writer.WriteInt32(p.SkillID); err != nil {
		return fmt.Errorf(
			"failed to write magic skill use id: %w", err)
	}
	ctrl := int32(0)
	if p.Ctrl {
		ctrl = 1
	}
	if err := writer.WriteInt32(ctrl); err != nil {
		return fmt.Errorf(
			"failed to write magic skill use ctrl flag: %w", err)
	}
	shift := int8(0)
	if p.Shift {
		shift = 1
	}
	if err := writer.WriteInt8(shift); err != nil {
		return fmt.Errorf(
			"failed to write magic skill use shift flag: %w", err)
	}

	return nil
}
