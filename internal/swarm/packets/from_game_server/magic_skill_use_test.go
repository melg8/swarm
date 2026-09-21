// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// writeMagicSkillUse builds a MagicSkillUse payload the way the
// Mobius C1 server writes it (see MagicSkillUse.writeImpl).
func writeMagicSkillUse(t *testing.T, critical bool) []byte {
    t.Helper()
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(magicSkillUsePacketID))
    require.NoError(t, writer.WriteInt32(100))  // caster
    require.NoError(t, writer.WriteInt32(300))  // target
    require.NoError(t, writer.WriteInt32(1077)) // skill id
    require.NoError(t, writer.WriteInt32(3))    // skill level
    require.NoError(t, writer.WriteInt32(1500)) // hit time ms
    require.NoError(t, writer.WriteInt32(6000)) // reuse delay ms
    require.NoError(t, writer.WriteInt32(45100))
    require.NoError(t, writer.WriteInt32(50200))
    require.NoError(t, writer.WriteInt32(-3500))
    if critical {
        require.NoError(t, writer.WriteInt32(1))
        require.NoError(t, writer.WriteInt16(0))
    } else {
        require.NoError(t, writer.WriteInt32(0))
    }
    require.NoError(t, writer.WriteInt32(45200))
    require.NoError(t, writer.WriteInt32(50300))
    require.NoError(t, writer.WriteInt32(-3500))

    return writer.Bytes()
}

func TestParseMagicSkillUsePacket(t *testing.T) {
    p := NewMagicSkillUsePacket()
    require.NoError(t, ParseMagicSkillUsePacket(
        p, writeMagicSkillUse(t, false)))
    require.Equal(t, int32(100), p.CasterID)
    require.Equal(t, int32(300), p.TargetID)
    require.Equal(t, int32(1077), p.SkillID)
    require.Equal(t, int32(3), p.SkillLevel)
    require.Equal(t, int32(1500), p.HitTime)
    require.Equal(t, int32(6000), p.ReuseDelay)
    require.Equal(t, int32(45100), p.X)
    require.Equal(t, int32(50200), p.Y)
    require.Equal(t, int32(-3500), p.Z)
    require.Equal(t, int32(45200), p.TargetX)
    require.Equal(t, int32(50300), p.TargetY)
    require.Equal(t, int32(-3500), p.TargetZ)
}

// The critical broadcast pads an extra int16 before the target
// location: the parser skips it and the target position stays
// aligned.
func TestParseMagicSkillUsePacketCritical(t *testing.T) {
    p := NewMagicSkillUsePacket()
    require.NoError(t, ParseMagicSkillUsePacket(
        p, writeMagicSkillUse(t, true)))
    require.Equal(t, int32(45200), p.TargetX)
    require.Equal(t, int32(50300), p.TargetY)
    require.Equal(t, int32(-3500), p.TargetZ)
}

func TestParseMagicSkillUsePacketRejectsWrongID(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(socialActionPacketID))

    p := NewMagicSkillUsePacket()
    require.Error(t, ParseMagicSkillUsePacket(p, writer.Bytes()))
}

func TestParseMagicSkillUsePacketRejectsTruncated(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(magicSkillUsePacketID))
    require.NoError(t, writer.WriteInt32(100))
    require.NoError(t, writer.WriteInt32(300))

    p := NewMagicSkillUsePacket()
    require.Error(t, ParseMagicSkillUsePacket(p, writer.Bytes()))
}
