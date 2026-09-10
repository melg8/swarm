// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestRequestAcquireSkillToBytes(t *testing.T) {
	t.Run("one lesson", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestAcquireSkillPacket()
		request.SkillID = 3 // Power Strike
		request.Level = 1   // level 1 of the strike
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x6C,                   // opcode
			0x03, 0x00, 0x00, 0x00, // skill id 3
			0x01, 0x00, 0x00, 0x00, // level 1
		}, writer.Bytes())
	})
	t.Run("a book lesson", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestAcquireSkillPacket()
		request.SkillID = 91 // Defence Aura
		request.Level = 1
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x6C,
			0x5B, 0x00, 0x00, 0x00, // skill id 91
			0x01, 0x00, 0x00, 0x00, // level 1
		}, writer.Bytes())
	})
}

func TestRequestMagicSkillUseToBytes(t *testing.T) {
	t.Run("a plain cast", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestMagicSkillUsePacket()
		request.SkillID = 3 // Power Strike
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x2F,                   // opcode
			0x03, 0x00, 0x00, 0x00, // skill id 3
			0x00, 0x00, 0x00, 0x00, // ctrl off
			0x00, // shift off
		}, writer.Bytes())
	})
	t.Run("a forced cast", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestMagicSkillUsePacket()
		request.SkillID = 1177 // Wind Strike
		request.Ctrl = true
		request.Shift = true
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x2F,
			0x99, 0x04, 0x00, 0x00, // skill id 1177
			0x01, 0x00, 0x00, 0x00, // ctrl on
			0x01, // shift on
		}, writer.Bytes())
	})
}
