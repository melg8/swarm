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
// at the selected target, a self buff or a heal. The server resolves
// the skill level through the known skill list and walks the cast
// flow (range check, mana cost, reuse delay) itself; the answers are
// the MagicSkillUse broadcast with the setup cast bar and the
// ActionFailed refusals the re-requests retry. A ctrl cast forces the
// attackable flag and a shift cast keeps the current target; the bot
// sends neither. Wire: [opcode 0x2F][magicId: 4][ctrl: 4][shift: 1].
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
