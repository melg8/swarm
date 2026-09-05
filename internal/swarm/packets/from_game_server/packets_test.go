// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

// utf16 encodes a string as null terminated utf16le bytes.
func utf16(value string) []byte {
	result := make([]byte, 0, len(value)*2+2)
	for _, r := range value {
		result = append(result, byte(r), byte(r>>8))
	}
	result = append(result, 0, 0)

	return result
}

// putInt32 appends a little endian int32 value.
func putInt32(dst []byte, value int32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(value)) //nolint:gosec // test

	return append(dst, buf[:]...)
}

// putFloat64 appends a little endian float64 value.
func putFloat64(dst []byte, value float64) []byte {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))

	return append(dst, buf[:]...)
}

func TestParseKeyPacket(t *testing.T) {
	t.Run("accepted protocol", func(t *testing.T) {
		data := []byte{0x00}
		data = append(data, 0x01) // result ok
		data = append(data, 0x94, 0x35, 0x00, 0x00, 0xa1, 0x6c, 0x54, 0x87)
		data = putInt32(data, 2) // server id
		data = putInt32(data, 1) // tail

		p := NewKeyPacket()
		err := ParseKeyPacket(p, data)
		require.NoError(t, err)
		require.True(t, p.Ok())
		require.Equal(t, int32(2), p.ServerID)
		require.Equal(t,
			[8]byte{0x94, 0x35, 0x00, 0x00, 0xa1, 0x6c, 0x54, 0x87}, p.Key)
	})

	t.Run("rejected protocol", func(t *testing.T) {
		data := []byte{0x00, 0x00}
		data = append(data, make([]byte, 8)...)
		data = putInt32(data, 1)
		data = putInt32(data, 1)

		p := NewKeyPacket()
		err := ParseKeyPacket(p, data)
		require.NoError(t, err)
		require.False(t, p.Ok())
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewKeyPacket()
		err := ParseKeyPacket(p, []byte{0x01})
		require.Error(t, err)
	})

	t.Run("empty data", func(t *testing.T) {
		p := NewKeyPacket()
		err := ParseKeyPacket(p, nil)
		require.Error(t, err)
	})

	t.Run("truncated key", func(t *testing.T) {
		data := []byte{0x00, 0x01, 0x94, 0x35}
		p := NewKeyPacket()
		err := ParseKeyPacket(p, data)
		require.Error(t, err)
	})

	t.Run("invalid tail", func(t *testing.T) {
		data := []byte{0x00, 0x01}
		data = append(data, make([]byte, 8)...)
		data = putInt32(data, 2)
		data = putInt32(data, 0)

		p := NewKeyPacket()
		err := ParseKeyPacket(p, data)
		require.Error(t, err)
	})
}

func TestParseCharCreateOkPacket(t *testing.T) {
	t.Run("success value", func(t *testing.T) {
		data := append([]byte{0x25}, 0x01, 0x00, 0x00, 0x00)
		err := ParseCharCreateOkPacket(data)
		require.NoError(t, err)
	})

	t.Run("unexpected value", func(t *testing.T) {
		data := append([]byte{0x25}, 0x02, 0x00, 0x00, 0x00)
		err := ParseCharCreateOkPacket(data)
		require.Error(t, err)
	})

	t.Run("invalid packet id", func(t *testing.T) {
		err := ParseCharCreateOkPacket([]byte{0x24, 1, 0, 0, 0})
		require.Error(t, err)
	})

	t.Run("truncated", func(t *testing.T) {
		err := ParseCharCreateOkPacket([]byte{0x25, 1, 0})
		require.Error(t, err)
	})
}

func TestParseCharCreateFailPacket(t *testing.T) {
	t.Run("known reason", func(t *testing.T) {
		data := append([]byte{0x26}, 0x02, 0x00, 0x00, 0x00)
		p := NewCharCreateFailPacket()
		err := ParseCharCreateFailPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(2), p.Reason)
		require.Equal(t, "name already exists", p.ReasonText())
	})

	t.Run("unknown reason", func(t *testing.T) {
		data := append([]byte{0x26}, 0x63, 0x00, 0x00, 0x00)
		p := NewCharCreateFailPacket()
		err := ParseCharCreateFailPacket(p, data)
		require.NoError(t, err)
		require.Contains(t, p.ReasonText(), "unknown reason 99")
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewCharCreateFailPacket()
		err := ParseCharCreateFailPacket(p, []byte{0x25, 0, 0, 0, 0})
		require.Error(t, err)
	})

	t.Run("truncated", func(t *testing.T) {
		p := NewCharCreateFailPacket()
		err := ParseCharCreateFailPacket(p, []byte{0x26, 1, 2})
		require.Error(t, err)
	})
}

// buildCharacterEntry builds a binary character entry of CharSelectInfo.
func buildCharacterEntry(name, account string) []byte {
	data := utf16(name)
	data = putInt32(data, 100) // object id
	data = append(data, utf16(account)...)
	data = putInt32(data, 0)    // session id
	data = putInt32(data, 0)    // clan id
	data = putInt32(data, 0)    // builder
	data = putInt32(data, 0)    // sex
	data = putInt32(data, 1)    // race
	data = putInt32(data, 18)   // base class
	data = putInt32(data, 1)    // gs name
	data = putInt32(data, 100)  // x
	data = putInt32(data, 200)  // y
	data = putInt32(data, 300)  // z
	data = putFloat64(data, 50) // cur hp
	data = putFloat64(data, 30) // cur mp
	data = putInt32(data, 0)    // sp
	data = putInt32(data, 0)    // exp
	data = putInt32(data, 1)    // level
	// karma, 9 deprecated zeros and 30 paperdoll entries.
	for range 40 {
		data = putInt32(data, 0)
	}
	data = putInt32(data, 2)    // hair style
	data = putInt32(data, 1)    // hair color
	data = putInt32(data, 0)    // face
	data = putFloat64(data, 50) // max hp
	data = putFloat64(data, 30) // max mp
	data = putInt32(data, 0)    // delete timer

	return data
}

func TestParseCharSelectInfoPacket(t *testing.T) {
	t.Run("two characters", func(t *testing.T) {
		data := []byte{0x1F}
		data = putInt32(data, 2)
		data = append(data, buildCharacterEntry("test1", "test1")...)
		data = append(data, buildCharacterEntry("other", "test1")...)

		p := NewCharSelectInfoPacket()
		err := ParseCharSelectInfoPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(2), p.Count)
		require.Len(t, p.Characters, 2)

		slot, info, found := p.FindCharacterByName("test1")
		require.True(t, found)
		require.NotNil(t, info)
		require.Equal(t, 0, slot)
		require.Equal(t, "test1", info.Name)
		require.Equal(t, int32(18), info.BaseClassID)
		require.Equal(t, int32(1), info.Race)
		require.Equal(t, int32(1), info.Level)
		require.Equal(t, int32(100), info.X)
		require.Equal(t, int32(2), info.HairStyle)

		_, _, found = p.FindCharacterByName("missing")
		require.False(t, found)
	})

	t.Run("reuse packet buffer", func(t *testing.T) {
		p := NewCharSelectInfoPacket()
		data := []byte{0x1F}
		data = putInt32(data, 1)
		data = append(data, buildCharacterEntry("test1", "test1")...)

		require.NoError(t, ParseCharSelectInfoPacket(p, data))
		require.NoError(t, ParseCharSelectInfoPacket(p, data))
		require.Len(t, p.Characters, 1)
	})

	t.Run("zero characters", func(t *testing.T) {
		data := append([]byte{0x1F}, 0, 0, 0, 0)
		p := NewCharSelectInfoPacket()
		err := ParseCharSelectInfoPacket(p, data)
		require.NoError(t, err)
		require.Empty(t, p.Characters)
	})
}

func TestParseCharSelectInfoPacketErrors(t *testing.T) {
	t.Run("invalid packet id", func(t *testing.T) {
		p := NewCharSelectInfoPacket()
		err := ParseCharSelectInfoPacket(p, []byte{0x20, 0, 0, 0, 0})
		require.Error(t, err)
	})

	t.Run("negative count", func(t *testing.T) {
		data := append([]byte{0x1F}, 0xFF, 0xFF, 0xFF, 0xFF)
		p := NewCharSelectInfoPacket()
		err := ParseCharSelectInfoPacket(p, data)
		require.Error(t, err)
	})

	t.Run("truncated character", func(t *testing.T) {
		data := []byte{0x1F}
		data = putInt32(data, 1)
		data = append(data, utf16("test1")...)

		p := NewCharSelectInfoPacket()
		err := ParseCharSelectInfoPacket(p, data)
		require.Error(t, err)
	})
}

func TestParseCharSelectedPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x21}
		data = append(data, utf16("test1")...)
		data = putInt32(data, 100) // object id
		data = append(data, utf16("title")...)
		data = putInt32(data, 42) // session id
		data = putInt32(data, 0)  // clan id
		data = putInt32(data, 0)  // unknown
		data = putInt32(data, 0)  // sex
		data = putInt32(data, 1)  // race
		data = putInt32(data, 18) // class id

		p := NewCharSelectedPacket()
		err := ParseCharSelectedPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, "test1", p.Name)
		require.Equal(t, int32(100), p.ObjectID)
		require.Equal(t, int32(42), p.SessionID)
		require.Equal(t, int32(18), p.ClassID)
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewCharSelectedPacket()
		err := ParseCharSelectedPacket(p, []byte{0x20})
		require.Error(t, err)
	})

	t.Run("truncated", func(t *testing.T) {
		data := []byte{0x21}
		data = append(data, utf16("test1")...)

		p := NewCharSelectedPacket()
		err := ParseCharSelectedPacket(p, data)
		require.Error(t, err)
	})
}

func TestParseNetPingPacket(t *testing.T) {
	t.Run("game time", func(t *testing.T) {
		data := append([]byte{0xEC}, 0x0A, 0x00, 0x00, 0x00)
		p := NewNetPingPacket()
		err := ParseNetPingPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(10), p.GameTime)
	})

	t.Run("invalid packet id", func(t *testing.T) {
		p := NewNetPingPacket()
		err := ParseNetPingPacket(p, []byte{0xEB, 0, 0, 0, 0})
		require.Error(t, err)
	})

	t.Run("truncated", func(t *testing.T) {
		p := NewNetPingPacket()
		err := ParseNetPingPacket(p, []byte{0xEC, 1, 2})
		require.Error(t, err)
	})
}

func TestReadInt32Fields(t *testing.T) {
	t.Run("reads values in order", func(t *testing.T) {
		data := []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}
		reader := packet.NewReader(data)

		var a, b int32
		err := readInt32Fields(reader, &a, &b)
		require.NoError(t, err)
		require.Equal(t, int32(1), a)
		require.Equal(t, int32(2), b)
	})

	t.Run("truncated data", func(t *testing.T) {
		reader := packet.NewReader([]byte{0x01, 0x00, 0x00})

		var a int32
		err := readInt32Fields(reader, &a)
		require.Error(t, err)
	})
}
