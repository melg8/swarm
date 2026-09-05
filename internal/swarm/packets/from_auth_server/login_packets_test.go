// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLoginOkPacket(t *testing.T) {
	t.Run("keys and tail", func(t *testing.T) {
		data := []byte{0x03}
		data = append(data, 0x44, 0x33, 0x22, 0x11)
		data = append(data, 0x88, 0x77, 0x66, 0x55)
		data = append(data, make([]byte, 5*4)...)

		p := NewLoginOkPacket()
		err := ParseLoginOkPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(0x11223344), p.LoginOkID1)
		require.Equal(t, int32(0x55667788), p.LoginOkID2)
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewLoginOkPacket()
		err := ParseLoginOkPacket(p, []byte{0x04})
		require.Error(t, err)
	})

	t.Run("missing tail", func(t *testing.T) {
		data := []byte{0x03}
		data = append(data, 0x44, 0x33, 0x22, 0x11)
		data = append(data, 0x88, 0x77, 0x66, 0x55)
		data = append(data, make([]byte, 4)...)

		p := NewLoginOkPacket()
		err := ParseLoginOkPacket(p, data)
		require.Error(t, err)
	})
}

func TestParsePlayOkPacket(t *testing.T) {
	t.Run("keys", func(t *testing.T) {
		data := []byte{0x07}
		data = append(data, 0x44, 0x33, 0x22, 0x11)
		data = append(data, 0x88, 0x77, 0x66, 0x55)

		p := NewPlayOkPacket()
		err := ParsePlayOkPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(0x11223344), p.PlayOkID1)
		require.Equal(t, int32(0x55667788), p.PlayOkID2)
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewPlayOkPacket()
		err := ParsePlayOkPacket(p, []byte{0x06, 0, 0, 0, 0, 0, 0, 0, 0})
		require.Error(t, err)
	})

	t.Run("truncated keys", func(t *testing.T) {
		p := NewPlayOkPacket()
		err := ParsePlayOkPacket(p, []byte{0x07, 1, 2, 3})
		require.Error(t, err)
	})
}

func TestParseLoginFailPacket(t *testing.T) {
	t.Run("reason in use", func(t *testing.T) {
		data := []byte{0x01, 0x07, 0x00, 0x00, 0x00}
		p := NewLoginFailPacket()
		err := ParseLoginFailPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(0x07), p.Reason)
		require.Equal(t, "account in use", p.ReasonText())
	})

	t.Run("unknown reason", func(t *testing.T) {
		data := []byte{0x01, 0x63, 0x00, 0x00, 0x00}
		p := NewLoginFailPacket()
		err := ParseLoginFailPacket(p, data)
		require.NoError(t, err)
		require.Contains(t, p.ReasonText(), "unknown reason 99")
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewLoginFailPacket()
		err := ParseLoginFailPacket(p, []byte{0x02, 0, 0, 0, 0})
		require.Error(t, err)
	})
}

func TestParseServerListPacket(t *testing.T) {
	t.Run("single server entry", func(t *testing.T) {
		data := []byte{
			0x04,         // opcode
			0x01,         // count
			0x02,         // last server
			0x02,         // server id
			127, 0, 0, 1, // ip
		}
		data = append(data, 0x61, 0x1E, 0x00, 0x00) // port 7777
		data = append(data, 0x00)                   // age
		data = append(data, 0x01)                   // pvp
		data = append(data, 0x00, 0x00)             // current players
		data = append(data, 0xD0, 0x07)             // max players
		data = append(data, 0x01)                   // status
		data = append(data, 0x00, 0x00, 0x00, 0x00) // bits
		data = append(data, 0x00)                   // brackets

		p := NewServerListPacket()
		err := ParseServerListPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int8(1), p.Count)
		require.Len(t, p.Servers, 1)

		server := p.FirstAvailableServer()
		require.NotNil(t, server)
		require.Equal(t, int8(2), server.ServerID)
		require.Equal(t, [4]byte{127, 0, 0, 1}, server.IP)
		require.Equal(t, int32(7777), server.Port)
		require.Equal(t, int8(1), server.Status)
		require.Equal(t, int16(2000), server.MaxPlayers)

		found := p.FindServerByID(2)
		require.NotNil(t, found)
		require.Nil(t, p.FindServerByID(9))
	})

	t.Run("empty list", func(t *testing.T) {
		data := []byte{0x04, 0x00, 0x00}
		p := NewServerListPacket()
		err := ParseServerListPacket(p, data)
		require.NoError(t, err)
		require.Nil(t, p.FirstAvailableServer())
	})

	t.Run("down server is skipped", func(t *testing.T) {
		data := []byte{0x04, 0x01, 0x00, 0x01, 10, 0, 0, 1}
		data = append(data, 0xE2, 0x1E, 0x00, 0x00)
		data = append(data, 0x00, 0x00, 0x00, 0x00, 0x00)
		data = append(data, 0x00, 0x00)
		data = append(data, 0x00) // status down
		data = append(data, 0x00, 0x00, 0x00, 0x00, 0x00)

		p := NewServerListPacket()
		err := ParseServerListPacket(p, data)
		require.NoError(t, err)
		require.Nil(t, p.FirstAvailableServer())
	})
}

func TestParseServerListPacketErrors(t *testing.T) {
	t.Run("invalid packet id", func(t *testing.T) {
		p := NewServerListPacket()
		err := ParseServerListPacket(p, []byte{0x05, 0x00, 0x00})
		require.Error(t, err)
	})

	t.Run("negative count", func(t *testing.T) {
		data := []byte{0x04, 0xFF, 0x00}
		p := NewServerListPacket()
		err := ParseServerListPacket(p, data)
		require.Error(t, err)
	})

	t.Run("truncated entry", func(t *testing.T) {
		data := []byte{0x04, 0x01, 0x00, 0x02, 127, 0, 0, 1}
		p := NewServerListPacket()
		err := ParseServerListPacket(p, data)
		require.Error(t, err)
	})
}
