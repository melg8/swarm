// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestProtocolVersionToBytes(t *testing.T) {
	t.Run("valid version", func(t *testing.T) {
		writer := packet.NewWriter()
		pv := NewProtocolVersion()
		err := pv.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x00,                   // opcode
			0xA3, 0x01, 0x00, 0x00, // 419 little endian
		}, writer.Bytes())
	})

	t.Run("constant matches c1", func(t *testing.T) {
		require.EqualValues(t, 419, C1ProtocolVersion)
	})
}

func TestAuthLoginToBytes(t *testing.T) {
	t.Run("valid keys", func(t *testing.T) {
		writer := packet.NewWriter()
		auth := &AuthLogin{
			Login:      "test1",
			PlayOkID1:  0x11223344,
			PlayOkID2:  0x55667788,
			LoginOkID1: -0x66554434,
			LoginOkID2: -0x22110100,
		}
		err := auth.ToBytes(writer)
		require.NoError(t, err)

		expected := []byte{0x08}
		expected = append(expected, utf16("test1")...)
		expected = append(expected,
			0x88, 0x77, 0x66, 0x55, // playOkID2 first
			0x44, 0x33, 0x22, 0x11, // playOkID1
			0xCC, 0xBB, 0xAA, 0x99, // loginOkID1 (0x99AABBCC)
			0x00, 0xFF, 0xEE, 0xDD, // loginOkID2 (0xDDEEFF00)
		)
		require.Equal(t, expected, writer.Bytes())
	})

	t.Run("empty login", func(t *testing.T) {
		writer := packet.NewWriter()
		auth := &AuthLogin{
			Login:      "",
			PlayOkID1:  1,
			PlayOkID2:  2,
			LoginOkID1: 3,
			LoginOkID2: 4,
		}
		err := auth.ToBytes(writer)
		require.Error(t, err)
		require.Empty(t, writer.Bytes())
	})
}

func TestCharacterCreateToBytes(t *testing.T) {
	t.Run("elven fighter", func(t *testing.T) {
		writer := packet.NewWriter()
		create := elvenFighterCreate("test1")
		err := create.ToBytes(writer)
		require.NoError(t, err)

		expected := []byte{0x0B}
		expected = append(expected, utf16("test1")...)
		for _, value := range []int32{1, 0, 18, 0, 0, 0, 0, 0, 0, 0, 0, 0} {
			expected = append(expected, byte(value), 0, 0, 0)
		}
		require.Equal(t, expected, writer.Bytes())
	})

	t.Run("empty name", func(t *testing.T) {
		writer := packet.NewWriter()
		create := elvenFighterCreate("")
		err := create.ToBytes(writer)
		require.Error(t, err)
	})

	t.Run("name too long", func(t *testing.T) {
		writer := packet.NewWriter()
		create := elvenFighterCreate("12345678901234567")
		err := create.ToBytes(writer)
		require.Error(t, err)
	})

	t.Run("name with multibyte runes", func(t *testing.T) {
		writer := packet.NewWriter()
		create := elvenFighterCreate("тест")
		err := create.ToBytes(writer)
		require.NoError(t, err)
	})
}

// elvenFighterCreate builds a creation packet for the given name.
func elvenFighterCreate(name string) *CharacterCreate {
	return &CharacterCreate{
		Name:      name,
		Race:      1,
		Female:    0,
		ClassID:   18,
		INT:       0,
		STR:       0,
		CON:       0,
		MEN:       0,
		DEX:       0,
		WIT:       0,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}
}

func TestCharacterSelectToBytes(t *testing.T) {
	t.Run("slot value", func(t *testing.T) {
		writer := packet.NewWriter()
		sel := &CharacterSelect{CharSlot: 3}
		err := sel.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x0D,
			0x03, 0x00, 0x00, 0x00,
		}, writer.Bytes())
	})
}

func TestSessionPacketsToBytes(t *testing.T) {
	t.Run("enter world", func(t *testing.T) {
		writer := packet.NewWriter()
		err := (&EnterWorld{}).ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{0x03}, writer.Bytes())
	})

	t.Run("request net ping", func(t *testing.T) {
		writer := packet.NewWriter()
		err := (&RequestNetPing{}).ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{0xA8}, writer.Bytes())
	})

	t.Run("logout", func(t *testing.T) {
		writer := packet.NewWriter()
		err := (&Logout{}).ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{0x09}, writer.Bytes())
	})
}

// utf16 encodes a string as null terminated utf16le bytes.
func utf16(value string) []byte {
	result := make([]byte, 0, len(value)*2+2)
	for _, r := range value {
		result = append(result, byte(r), byte(r>>8))
	}
	result = append(result, 0, 0)

	return result
}

func TestAttackRequestToBytes(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		writer := packet.NewWriter()
		req := &AttackRequestPacket{
			TargetID: 268473919, X: 45000, Y: 50000, Z: -3500, Shift: 0,
		}
		err := req.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, byte(0x0A), writer.Bytes()[0])
		require.Len(t, writer.Bytes(), 1+4*4+1)
	})
}

func TestActionRequestToBytes(t *testing.T) {
	t.Run("pickup click", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewActionRequestPacket()
		request.ObjectID = 9001
		request.X = 45000
		request.Y = 50000
		request.Z = -3500
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x04,                   // opcode
			0x29, 0x23, 0x00, 0x00, // 9001
			0xC8, 0xAF, 0x00, 0x00, // 45000
			0x50, 0xC3, 0x00, 0x00, // 50000
			0x54, 0xF2, 0xFF, 0xFF, // -3500
			0x00, // simple click
		}, writer.Bytes())
	})
}

func TestRequestDestroyItemToBytes(t *testing.T) {
	t.Run("destroy request", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestDestroyItem()
		request.ObjectID = 17
		request.Count = 1
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x59,                   // opcode
			0x11, 0x00, 0x00, 0x00, // object id
			0x01, 0x00, 0x00, 0x00, // count
		}, writer.Bytes())
	})
}

func TestAppearingToBytes(t *testing.T) {
	writer := packet.NewWriter()
	err := NewAppearingPacket().ToBytes(writer)
	require.NoError(t, err)
	require.Equal(t, []byte{0x30}, writer.Bytes())
}

func TestRequestSellItemToBytes(t *testing.T) {
	t.Run("two items", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestSellItemPacket()
		request.Items = []SellItemEntry{
			{ObjectID: 17, ItemID: 34, Count: 1},
			{ObjectID: 42, ItemID: 1061, Count: 500},
		}
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x1E,                   // opcode
			0x00, 0x00, 0x00, 0x00, // sell list id 0 (inventory sell)
			0x02, 0x00, 0x00, 0x00, // item count
			0x11, 0x00, 0x00, 0x00, // object id
			0x22, 0x00, 0x00, 0x00, // item id
			0x01, 0x00, 0x00, 0x00, // count
			0x2A, 0x00, 0x00, 0x00, // object id
			0x25, 0x04, 0x00, 0x00, // item id
			0xF4, 0x01, 0x00, 0x00, // count
		}, writer.Bytes())
	})
	t.Run("empty request", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestSellItemPacket()
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x1E,                   // opcode
			0x00, 0x00, 0x00, 0x00, // sell list id
			0x00, 0x00, 0x00, 0x00, // item count
		}, writer.Bytes())
	})
}

func TestRequestItemListToBytes(t *testing.T) {
	t.Run("empty request", func(t *testing.T) {
		writer := packet.NewWriter()
		request := &RequestItemList{}
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{0x0F}, writer.Bytes())
	})
}
