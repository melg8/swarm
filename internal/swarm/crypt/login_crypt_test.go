// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestChecksumLE(t *testing.T) {
	t.Run("xor of little endian words", func(t *testing.T) {
		// 0x00000004 ^ 0x00000000 = 4.
		checksum, err := ChecksumLE([]byte{4, 0, 0, 0, 0, 0, 0, 0})
		require.NoError(t, err)
		require.Equal(t, uint32(4), checksum)
	})

	t.Run("checksum word is excluded", func(t *testing.T) {
		// The trailing word must not change the checksum value.
		data := []byte{1, 0, 0, 0, 2, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF}
		checksum, err := ChecksumLE(data)
		require.NoError(t, err)
		require.Equal(t, uint32(1^2), checksum)
	})

	t.Run("too small data", func(t *testing.T) {
		_, err := ChecksumLE([]byte{1, 2, 3})
		require.Error(t, err)
	})

	t.Run("unaligned data", func(t *testing.T) {
		_, err := ChecksumLE([]byte{1, 2, 3, 4, 5})
		require.Error(t, err)
	})
}

func TestFillerSize(t *testing.T) {
	tests := []struct {
		content int
		filler  int
	}{
		{content: 0, filler: 4},
		{content: 4, filler: 0},
		{content: 5, filler: 7},
		{content: 9, filler: 3},
		{content: 16, filler: 4},
	}
	for _, tc := range tests {
		require.Equal(t, tc.filler, fillerSize(tc.content),
			"content size %d", tc.content)
	}
}

func TestLoginCryptSealOpen(t *testing.T) {
	t.Run("roundtrip preserves content", func(t *testing.T) {
		sealer := NewLoginCrypt(MobiusAuthKey())
		opener := NewLoginCrypt(MobiusAuthKey())

		content := []byte{0x07, 0x01, 0x00, 0x00, 0x00}
		wire, err := sealer.Seal(nil, content)
		require.NoError(t, err)
		require.Len(t, wire, 2+16, "payload must be aligned to 8 bytes")

		size := int(wire[0]) | int(wire[1])<<8
		require.Equal(t, 18, size, "size header counts itself")

		payload := wire[2:]
		opened, err := opener.Open(payload)
		require.NoError(t, err)
		require.Equal(t, content, opened[:len(content)])
	})

	t.Run("reuses destination buffer", func(t *testing.T) {
		sealer := NewLoginCrypt(MobiusAuthKey())
		content := []byte{0x01, 0x02, 0x03, 0x04}

		dst := make([]byte, 0, 64)
		wire, err := sealer.Seal(dst, content)
		require.NoError(t, err)

		// Same underlying storage means the buffer was reused.
		require.Len(t, wire, 2+8)

		wire2, err := sealer.Seal(wire, content)
		require.NoError(t, err)
		require.Equal(t, wire, wire2)
	})

	t.Run("header size is little endian", func(t *testing.T) {
		sealer := NewLoginCrypt(MobiusAuthKey())
		wire, err := sealer.Seal(nil, []byte{1})
		require.NoError(t, err)
		require.Equal(t, byte(10), wire[0])
		require.Equal(t, byte(0), wire[1])
	})

	t.Run("oversized content is rejected", func(t *testing.T) {
		sealer := NewLoginCrypt(MobiusAuthKey())
		content := make([]byte, loginMaxPacket)
		_, err := sealer.Seal(nil, content)
		require.Error(t, err)
	})
}

func TestLoginCryptOpen(t *testing.T) {
	t.Run("checksum mismatch", func(t *testing.T) {
		sealer := NewLoginCrypt(MobiusAuthKey())
		opener := NewLoginCrypt(MobiusAuthKey())

		wire, err := sealer.Seal(nil, []byte{0x07, 0x01, 0x00, 0x00, 0x00})
		require.NoError(t, err)

		payload := wire[2:]
		payload[len(payload)-1] ^= 0xFF // corrupt the checksum

		_, err = opener.Open(payload)
		require.Error(t, err)
	})

	t.Run("payload too small", func(t *testing.T) {
		opener := NewLoginCrypt(MobiusAuthKey())
		_, err := opener.Open([]byte{1, 2})
		require.Error(t, err)
	})

	t.Run("payload not aligned", func(t *testing.T) {
		opener := NewLoginCrypt(MobiusAuthKey())
		_, err := opener.Open(make([]byte, 12))
		require.Error(t, err)
	})
}

func TestMobiusAuthKey(t *testing.T) {
	t.Run("stable static key", func(t *testing.T) {
		cipher := MobiusAuthKey()
		require.NotNil(t, cipher)
		require.Equal(t, MobiusAuthKey(), cipher)
	})
}

func TestLoginCryptSealPacket(t *testing.T) {
	t.Run("serializes and seals a packet", func(t *testing.T) {
		sealer := NewLoginCrypt(MobiusAuthKey())
		opener := NewLoginCrypt(MobiusAuthKey())

		packet := &testLoginPacket{}
		wire, err := sealer.SealPacket(nil, packet)
		require.NoError(t, err)

		opened, err := opener.Open(wire[2:])
		require.NoError(t, err)
		require.Equal(t,
			[]byte{0x07, 0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0, 0, 0, 0}, opened)
	})
}

// testLoginPacket is a minimal serializable packet for tests.
type testLoginPacket struct{}

func (p *testLoginPacket) ToBytes(writer *packet.Writer) error {
	return writer.WriteBytes([]byte{0x07, 0x01, 0x02, 0x03, 0x04})
}
