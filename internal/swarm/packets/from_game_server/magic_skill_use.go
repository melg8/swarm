// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

// magicSkillUsePacketID is the C1 opcode of the cast broadcast
// (ServerPackets.MAGIC_SKILL_USE, 0x5A).
const magicSkillUsePacketID = 0x5A

// MagicSkillUsePacket announces one creature starting a skill cast.
// The hit time is the cast animation length in milliseconds (the
// skill resolves when it elapses) and the reuse delay is the
// cooldown the server opened for the skill, also in milliseconds -
// both drive the cast fill and the cooldown countdown of the web
// view.
// Wire format (see MagicSkillUse.writeImpl): [opcode 0x5A]
// [casterId: 4][targetId: 4][skillId: 4][skillLevel: 4]
// [hitTime: 4][reuseDelay: 4]
// [casterX: 4][casterY: 4][casterZ: 4]
// [criticalFlag: 4][pad: 2 when critical]
// [targetX: 4][targetY: 4][targetZ: 4].
type MagicSkillUsePacket struct {
    CasterID   int32
    TargetID   int32
    SkillID    int32
    SkillLevel int32
    // HitTime is the cast time in milliseconds, ReuseDelay the
    // cooldown in milliseconds.
    HitTime    int32
    ReuseDelay int32
    X          int32
    Y          int32
    Z          int32
    TargetX    int32
    TargetY    int32
    TargetZ    int32
}

// NewMagicSkillUsePacket creates a zero valued packet ready for
// parsing.
func NewMagicSkillUsePacket() *MagicSkillUsePacket {
    return &MagicSkillUsePacket{
        CasterID:   0,
        TargetID:   0,
        SkillID:    0,
        SkillLevel: 0,
        HitTime:    0,
        ReuseDelay: 0,
        X:          0,
        Y:          0,
        Z:          0,
        TargetX:    0,
        TargetY:    0,
        TargetZ:    0,
    }
}

// ParseMagicSkillUsePacket reads the packet from payload bytes.
func ParseMagicSkillUsePacket(
    p *MagicSkillUsePacket, data []byte,
) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, magicSkillUsePacketID); err != nil {
        return err
    }
    if err := readInt32Fields(
        reader, &p.CasterID, &p.TargetID, &p.SkillID, &p.SkillLevel,
    ); err != nil {
        return fmt.Errorf("failed to read skill ids: %w", err)
    }
    if err := readInt32Fields(
        reader, &p.HitTime, &p.ReuseDelay,
    ); err != nil {
        return fmt.Errorf("failed to read skill timing: %w", err)
    }
    if err := readInt32Fields(
        reader, &p.X, &p.Y, &p.Z,
    ); err != nil {
        return fmt.Errorf("failed to read caster location: %w", err)
    }
    // The critical branch pads the tail with an extra int16 before
    // the target location (see MagicSkillUse.writeImpl).
    var critical int32
    if err := readInt32Fields(reader, &critical); err != nil {
        return fmt.Errorf("failed to read critical flag: %w", err)
    }
    if critical != 0 {
        if err := reader.Skip(2); err != nil {
            return fmt.Errorf("failed to skip critical pad: %w", err)
        }
    }
    if err := readInt32Fields(
        reader, &p.TargetX, &p.TargetY, &p.TargetZ,
    ); err != nil {
        return fmt.Errorf("failed to read target location: %w", err)
    }

    return nil
}
