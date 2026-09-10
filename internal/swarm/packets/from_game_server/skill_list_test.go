// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

// buildSkillListPayload writes a SkillList packet body with the given
// entries: [count: 4] then per skill [passive: 4][level: 4][id: 4].
func buildSkillListPayload(skills [][3]int32) []byte {
	writer := packet.NewWriter()
	writer.WriteByte(skillListPacketID)
	writer.WriteInt32(int32(len(skills)))
	for _, skill := range skills {
		writer.WriteInt32(skill[0]) // passive
		writer.WriteInt32(skill[1]) // level
		writer.WriteInt32(skill[2]) // id
	}

	return writer.Bytes()
}

func TestParseSkillListPacket(t *testing.T) {
	p := NewSkillListPacket()
	data := buildSkillListPayload([][3]int32{
		{0, 3, 3},   // Power Strike lvl 3, active
		{1, 1, 141}, // Weapon Mastery lvl 1, passive
		{0, 2, 56},  // Power Shot lvl 2, active
	})

	require.NoError(t, ParseSkillListPacket(p, data))
	require.Len(t, p.Skills, 3)

	require.Equal(t, int32(3), p.Skills[0].SkillID)
	require.Equal(t, int32(3), p.Skills[0].Level)
	require.False(t, p.Skills[0].Passive)

	require.Equal(t, int32(141), p.Skills[1].SkillID)
	require.Equal(t, int32(1), p.Skills[1].Level)
	require.True(t, p.Skills[1].Passive)

	require.Equal(t, int32(56), p.Skills[2].SkillID)
	require.Equal(t, int32(2), p.Skills[2].Level)
	require.False(t, p.Skills[2].Passive)
}

func TestParseSkillListPacketReusesBuffer(t *testing.T) {
	p := NewSkillListPacket()
	first := buildSkillListPayload([][3]int32{{0, 1, 3}, {0, 2, 3}})
	require.NoError(t, ParseSkillListPacket(p, first))
	require.Len(t, p.Skills, 2)

	// The second parse resets the buffer instead of growing it.
	second := buildSkillListPayload([][3]int32{{1, 1, 142}})
	require.NoError(t, ParseSkillListPacket(p, second))
	require.Len(t, p.Skills, 1)
	require.Equal(t, int32(142), p.Skills[0].SkillID)
}

func TestParseSkillListPacketRejectsBadID(t *testing.T) {
	p := NewSkillListPacket()
	writer := packet.NewWriter()
	writer.WriteByte(0x6C)
	writer.WriteInt32(0)

	err := ParseSkillListPacket(p, writer.Bytes())
	require.Error(t, err)
}

func TestParseSkillListPacketRejectsImplausibleCount(t *testing.T) {
	p := NewSkillListPacket()
	writer := packet.NewWriter()
	writer.WriteByte(skillListPacketID)
	writer.WriteInt32(skillListCap + 1)

	err := ParseSkillListPacket(p, writer.Bytes())
	require.Error(t, err)
	require.Contains(t, err.Error(), "implausible skill count")
}

func TestParseSkillListPacketRejectsTruncatedBody(t *testing.T) {
	p := NewSkillListPacket()
	data := buildSkillListPayload([][3]int32{{0, 1, 3}, {0, 2, 3}})
	// Drop the last five bytes so the second entry is incomplete.
	err := ParseSkillListPacket(p, data[:len(data)-5])
	require.Error(t, err)
}
