// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

// buildAbnormalStatusUpdatePayload writes an AbnormalStatusUpdate
// packet body with the given entries: [count: 2] then per effect
// [skillId: 4][level: 2][time: 4].
func buildAbnormalStatusUpdatePayload(buffs [][3]int32) []byte {
	writer := packet.NewWriter()
	writer.WriteByte(abnormalStatusUpdatePacketID)
	writer.WriteInt16(int16(len(buffs)))
	for _, buff := range buffs {
		writer.WriteInt32(buff[0])        // skill id
		writer.WriteInt16(int16(buff[1])) // level
		writer.WriteInt32(buff[2])        // remaining seconds
	}

	return writer.Bytes()
}

func TestParseAbnormalStatusUpdatePacket(t *testing.T) {
	p := NewAbnormalStatusUpdatePacket()
	data := buildAbnormalStatusUpdatePayload([][3]int32{
		{91, 1, 1198}, // Defence Aura lvl 1, 19:58 left
		{77, 1, 600},  // Attack Aura lvl 1, 10 minutes left
	})

	require.NoError(t, ParseAbnormalStatusUpdatePacket(p, data))
	require.Len(t, p.Buffs, 2)

	require.Equal(t, int32(91), p.Buffs[0].SkillID)
	require.Equal(t, int32(1), p.Buffs[0].Level)
	require.Equal(t, int32(1198), p.Buffs[0].Time)

	require.Equal(t, int32(77), p.Buffs[1].SkillID)
	require.Equal(t, int32(1), p.Buffs[1].Level)
	require.Equal(t, int32(600), p.Buffs[1].Time)
}

func TestParseAbnormalStatusUpdatePacketReusesBuffer(t *testing.T) {
	p := NewAbnormalStatusUpdatePacket()
	first := buildAbnormalStatusUpdatePayload(
		[][3]int32{{91, 1, 100}, {77, 2, 200}})
	require.NoError(t, ParseAbnormalStatusUpdatePacket(p, first))
	require.Len(t, p.Buffs, 2)

	// The second parse resets the buffer instead of growing it.
	second := buildAbnormalStatusUpdatePayload([][3]int32{{1040, 1, 50}})
	require.NoError(t, ParseAbnormalStatusUpdatePacket(p, second))
	require.Len(t, p.Buffs, 1)
	require.Equal(t, int32(1040), p.Buffs[0].SkillID)
}

func TestParseAbnormalStatusUpdatePacketEmptyList(t *testing.T) {
	p := NewAbnormalStatusUpdatePacket()
	data := buildAbnormalStatusUpdatePayload(nil)

	require.NoError(t, ParseAbnormalStatusUpdatePacket(p, data))
	require.Empty(t, p.Buffs)
}

func TestParseAbnormalStatusUpdatePacketRejectsBadID(t *testing.T) {
	p := NewAbnormalStatusUpdatePacket()
	writer := packet.NewWriter()
	writer.WriteByte(0x6E)
	writer.WriteInt16(0)

	err := ParseAbnormalStatusUpdatePacket(p, writer.Bytes())
	require.Error(t, err)
}

func TestParseAbnormalStatusUpdatePacketRejectsImplausibleCount(t *testing.T) {
	p := NewAbnormalStatusUpdatePacket()
	writer := packet.NewWriter()
	writer.WriteByte(abnormalStatusUpdatePacketID)
	writer.WriteInt16(abnormalBuffCap + 1)

	err := ParseAbnormalStatusUpdatePacket(p, writer.Bytes())
	require.Error(t, err)
}

func TestParseAbnormalStatusUpdatePacketRejectsTruncatedEntry(t *testing.T) {
	p := NewAbnormalStatusUpdatePacket()
	writer := packet.NewWriter()
	writer.WriteByte(abnormalStatusUpdatePacketID)
	writer.WriteInt16(2)
	writer.WriteInt32(91)
	writer.WriteInt16(1)
	// The remaining seconds of the second entry are cut off.

	err := ParseAbnormalStatusUpdatePacket(p, writer.Bytes())
	require.Error(t, err)
}
