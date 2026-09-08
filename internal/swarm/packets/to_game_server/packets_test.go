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
		require.Equal(t, 419, C1ProtocolVersion)
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

func TestRequestUseItemToBytes(t *testing.T) {
	t.Run("use item request", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestUseItem()
		request.ObjectID = 268476112
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x14,                   // opcode
			0xD0, 0x9E, 0x00, 0x10, // object id 268476112
		}, writer.Bytes())
	})
}

func TestRequestDropItemToBytes(t *testing.T) {
	t.Run("drop item request", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestDropItem()
		request.ObjectID = 17
		request.Count = 50
		request.X = 46112
		request.Y = 41500
		request.Z = -3056
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x12,                   // opcode
			0x11, 0x00, 0x00, 0x00, // object id
			0x32, 0x00, 0x00, 0x00, // count
			0x20, 0xB4, 0x00, 0x00, // x
			0x1C, 0xA2, 0x00, 0x00, // y
			0x10, 0xF4, 0xFF, 0xFF, // z
		}, writer.Bytes())
	})
}

func TestMoveToLocationToBytes(t *testing.T) {
	t.Run("ground click walk", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewMoveToLocationRequestPacket()
		request.TargetX = 46112
		request.TargetY = 41500
		request.TargetZ = -3056
		request.OriginX = 45008
		request.OriginY = 41492
		request.OriginZ = -3056
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x01,                   // opcode
			0x20, 0xB4, 0x00, 0x00, // target x
			0x1C, 0xA2, 0x00, 0x00, // target y
			0x10, 0xF4, 0xFF, 0xFF, // target z
			0xD0, 0xAF, 0x00, 0x00, // origin x
			0x14, 0xA2, 0x00, 0x00, // origin y
			0x10, 0xF4, 0xFF, 0xFF, // origin z
			0x01, 0x00, 0x00, 0x00, // mouse mode
		}, writer.Bytes())
	})

	t.Run("constructor defaults to mouse mode", func(t *testing.T) {
		request := NewMoveToLocationRequestPacket()
		require.Equal(t, MoveModeMouse, request.Mode)
		require.Zero(t, request.TargetX)
		require.Zero(t, request.OriginZ)
	})
}

func TestRequestActionUseToBytes(t *testing.T) {
	t.Run("sit stand toggle", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestActionUsePacket()
		request.ActionID = ActionSitStand
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x45,                   // opcode
			0x00, 0x00, 0x00, 0x00, // action id 0 (sit/stand)
			0x00, 0x00, 0x00, 0x00, // ctrl not pressed
			0x00, // shift not pressed
		}, writer.Bytes())
	})

	t.Run("ctrl and shift pressed", func(t *testing.T) {
		writer := packet.NewWriter()
		request := &RequestActionUsePacket{ActionID: 2, Ctrl: true, Shift: true}
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x45,                   // opcode
			0x02, 0x00, 0x00, 0x00, // action id
			0x01, 0x00, 0x00, 0x00, // ctrl pressed
			0x01, // shift pressed
		}, writer.Bytes())
	})
}

func TestRequestRestartPointToBytes(t *testing.T) {
	t.Run("village restart", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestRestartPointPacket()
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x6D,                   // opcode
			0x00, 0x00, 0x00, 0x00, // restart type village
		}, writer.Bytes())
	})
}

func TestNewAttackRequestPacketDefaults(t *testing.T) {
	request := NewAttackRequestPacket()
	require.Zero(t, request.TargetID)
	require.Zero(t, request.X)
	require.Zero(t, request.Y)
	require.Zero(t, request.Z)
	require.Equal(t, int8(0), request.Shift)
}
