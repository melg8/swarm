// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestRequestAuthLoginToBytes(t *testing.T) {
	t.Run("valid credentials", func(t *testing.T) {
		writer := packet.NewWriter()
		req := &RequestAuthLogin{Account: "test1", Password: "test"}
		err := req.ToBytes(writer)
		require.NoError(t, err)

		expected := []byte{0x00}
		expected = append(expected, field("test1")...)
		expected = append(expected, field("test")...)

		require.Equal(t, expected, writer.Bytes())
	})

	t.Run("empty account", func(t *testing.T) {
		writer := packet.NewWriter()
		req := &RequestAuthLogin{Account: "", Password: "test"}
		err := req.ToBytes(writer)
		require.Error(t, err)
	})

	t.Run("empty password", func(t *testing.T) {
		writer := packet.NewWriter()
		req := &RequestAuthLogin{Account: "test1", Password: ""}
		err := req.ToBytes(writer)
		require.Error(t, err)
	})

	t.Run("account too long", func(t *testing.T) {
		writer := packet.NewWriter()
		req := &RequestAuthLogin{Account: "123456789012345", Password: "test"}
		err := req.ToBytes(writer)
		require.Error(t, err)
	})
}

func TestRequestServerListToBytes(t *testing.T) {
	t.Run("keys and flag", func(t *testing.T) {
		writer := packet.NewWriter()
		req := NewRequestServerList(0x11223344, 0x55667788)
		err := req.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x05,
			0x44, 0x33, 0x22, 0x11,
			0x88, 0x77, 0x66, 0x55,
			0x01,
		}, writer.Bytes())
	})
}

func TestRequestServerLoginToBytes(t *testing.T) {
	t.Run("keys and server id", func(t *testing.T) {
		writer := packet.NewWriter()
		req := &RequestServerLogin{
			LoginOkID1: 0x11223344,
			LoginOkID2: 0x55667788,
			ServerID:   2,
		}
		err := req.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x02,
			0x44, 0x33, 0x22, 0x11,
			0x88, 0x77, 0x66, 0x55,
			0x02,
		}, writer.Bytes())
	})
}

// field encodes a zero padded 14 byte credential field.
func field(value string) []byte {
	result := make([]byte, authFieldSize)
	copy(result, value)

	return result
}
