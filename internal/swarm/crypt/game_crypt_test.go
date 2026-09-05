// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// defaultKey returns a fresh copy of the static game key.
func defaultKey() [GameCryptKeySize]byte {
	return DefaultGameCryptKey()
}

func TestNewGameCrypt(t *testing.T) {
	t.Run("starts disabled with copied keys", func(t *testing.T) {
		key := DefaultGameCryptKey()
		cipher := NewGameCrypt(key)
		require.False(t, cipher.Enabled())

		key[0] = 0xFF
		require.Equal(t, byte(0x94), cipher.outKey[0],
			"key must be copied, not aliased")
	})

	t.Run("enable activates cipher", func(t *testing.T) {
		cipher := NewGameCrypt(DefaultGameCryptKey())
		cipher.Enable()
		require.True(t, cipher.Enabled())
	})
}

func TestGameCryptEncrypt(t *testing.T) {
	t.Run("disabled cipher is passthrough", func(t *testing.T) {
		cipher := NewGameCrypt(DefaultGameCryptKey())
		data := []byte{1, 2, 3, 4, 5}
		cipher.Encrypt(data)
		require.Equal(t, []byte{1, 2, 3, 4, 5}, data)
	})

	t.Run("matches mobius encryption reference", func(t *testing.T) {
		cipher := NewGameCrypt(DefaultGameCryptKey())
		cipher.Enable()

		data := []byte{0x08, 0x74, 0x00, 0x65, 0x00, 0x73, 0x00, 0x74, 0x00}
		cipher.Encrypt(data)

		expected, err := hex.DecodeString("9cddddb8190652a135")
		require.NoError(t, err)
		require.Equal(t, expected, data)
	})

	t.Run("empty data does not advance offset", func(t *testing.T) {
		cipher := NewGameCrypt(DefaultGameCryptKey())
		cipher.Enable()
		cipher.Encrypt(nil)
		require.Equal(t, DefaultGameCryptKey(), cipher.outKey)
	})
}

func TestGameCryptDecrypt(t *testing.T) {
	t.Run("disabled cipher is passthrough", func(t *testing.T) {
		cipher := NewGameCrypt(DefaultGameCryptKey())
		data := []byte{9, 8, 7}
		cipher.Decrypt(data)
		require.Equal(t, []byte{9, 8, 7}, data)
	})

	t.Run("roundtrip with independent ciphers", func(t *testing.T) {
		outCipher := NewGameCrypt(DefaultGameCryptKey())
		inCipher := NewGameCrypt(DefaultGameCryptKey())
		outCipher.Enable()
		inCipher.Enable()

		original := []byte{0x08, 0x74, 0x00, 0x65, 0x00, 0x73, 0x00, 0x74, 0x00}
		wire := append([]byte(nil), original...)
		outCipher.Encrypt(wire)
		inCipher.Decrypt(wire)
		require.Equal(t, original, wire)

		// Second packet keeps both ciphers in sync.
		ping := []byte{0xA8, 0x00, 0x01}
		wire2 := append([]byte(nil), ping...)
		outCipher.Encrypt(wire2)
		inCipher.Decrypt(wire2)
		require.Equal(t, ping, wire2)
	})

	t.Run("offsets advance equally on both sides", func(t *testing.T) {
		outCipher := NewGameCrypt(DefaultGameCryptKey())
		inCipher := NewGameCrypt(DefaultGameCryptKey())
		outCipher.Enable()
		inCipher.Enable()

		data := make([]byte, 300)
		for i := range data {
			data[i] = byte(i)
		}
		wire := append([]byte(nil), data...)
		outCipher.Encrypt(wire)
		inCipher.Decrypt(wire)
		require.Equal(t, data, wire)
		require.Equal(t, outCipher.outKey, inCipher.inKey)
	})
}

func TestGameCryptAdvanceOffset(t *testing.T) {
	t.Run("wraps little endian counter", func(t *testing.T) {
		cipher := &GameCrypt{
			inKey:   defaultKey(),
			outKey:  defaultKey(),
			enabled: false,
		}
		cipher.advanceOffset(&cipher.inKey, 300)
		require.Equal(t, byte(0x94+44), cipher.inKey[0])

		cipher.advanceOffset(&cipher.inKey, -300)
		require.Equal(t, defaultKey(), cipher.inKey,
			"offset must wrap around the 32 bit counter")
	})
}
