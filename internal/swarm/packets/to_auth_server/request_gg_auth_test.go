// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestNewRequestGGAuth(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		testData := []byte{
			0x07,                   // PacketID
			0x01, 0x00, 0x00, 0x00, // SessionID: 1
			0x02, 0x00, 0x00, 0x00, // Data1: 2
			0x03, 0x00, 0x00, 0x00, // Data2: 3
			0x04, 0x00, 0x00, 0x00, // Data3: 4
			0x05, 0x00, 0x00, 0x00, // Data4: 5
		}

		req, err := NewRequestGGAuthFrom(testData)
		require.NoError(t, err)
		require.NotNil(t, req)
		require.Equal(t, int32(1), req.SessionID)
		require.Equal(t, int32(2), req.Data1)
		require.Equal(t, int32(3), req.Data2)
		require.Equal(t, int32(4), req.Data3)
		require.Equal(t, int32(5), req.Data4)
	})

	t.Run("empty data", func(t *testing.T) {
		req, err := NewRequestGGAuthFrom([]byte{})
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("error reading SessionID", func(t *testing.T) {
		testData := []byte{0x07, // PacketID
			0x01, 0x00, 0x00} // Incomplete SessionID
		req, err := NewRequestGGAuthFrom(testData)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("error reading Data1", func(t *testing.T) {
		testData := []byte{
			0x07,                   // PacketID
			0x01, 0x00, 0x00, 0x00, // Complete SessionID
			0x02, 0x00, 0x00, // Incomplete Data1
		}
		req, err := NewRequestGGAuthFrom(testData)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("error reading Data2", func(t *testing.T) {
		testData := []byte{
			0x07,                   // PacketID
			0x01, 0x00, 0x00, 0x00, // Complete SessionID
			0x02, 0x00, 0x00, 0x00, // Complete Data1
			0x03, 0x00, 0x00, // Incomplete Data2
		}
		req, err := NewRequestGGAuthFrom(testData)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("error reading Data3", func(t *testing.T) {
		testData := []byte{
			0x07,                   // PacketID
			0x01, 0x00, 0x00, 0x00, // Complete SessionID
			0x02, 0x00, 0x00, 0x00, // Complete Data1
			0x03, 0x00, 0x00, 0x00, // Complete Data2
			0x04, 0x00, 0x00, // Incomplete Data3
		}
		req, err := NewRequestGGAuthFrom(testData)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("error reading Data4", func(t *testing.T) {
		testData := []byte{
			0x07,                   // PacketID
			0x01, 0x00, 0x00, 0x00, // Complete SessionID
			0x02, 0x00, 0x00, 0x00, // Complete Data1
			0x03, 0x00, 0x00, 0x00, // Complete Data2
			0x04, 0x00, 0x00, 0x00, // Complete Data3
			0x05, 0x00, 0x00, // Incomplete Data4
		}
		req, err := NewRequestGGAuthFrom(testData)
		require.Error(t, err)
		require.Nil(t, req)
	})
}

func TestRequestGGAuth_ToString(t *testing.T) {
	req := &RequestGGAuth{
		SessionID: 1,
		Data1:     2,
		Data2:     3,
		Data3:     4,
		Data4:     5,
	}

	str := req.ToString()
	require.Contains(t, str, "SessionID: 00000001")
	require.Contains(t, str, "Data1: 00000002")
	require.Contains(t, str, "Data2: 00000003")
	require.Contains(t, str, "Data3: 00000004")
	require.Contains(t, str, "Data4: 00000005")
}

func TestRequestGGAuth_ToBytes(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		req := &RequestGGAuth{
			SessionID: 1,
			Data1:     2,
			Data2:     3,
			Data3:     4,
			Data4:     5,
		}
		packetWriter := packet.NewWriter()
		err := req.ToBytes(packetWriter)
		data := packetWriter.Bytes()
		require.NoError(t, err)
		require.NotNil(t, data)

		// Expected byte sequence
		expected := []byte{
			0x07,                   // PacketID
			0x01, 0x00, 0x00, 0x00, // SessionID: 1
			0x02, 0x00, 0x00, 0x00, // Data1: 2
			0x03, 0x00, 0x00, 0x00, // Data2: 3
			0x04, 0x00, 0x00, 0x00, // Data3: 4
			0x05, 0x00, 0x00, 0x00, // Data4: 5
		}
		require.Equal(t, expected, data)

		// Verify that we can read it back
		decoded, err := NewRequestGGAuthFrom(data)
		require.NoError(t, err)
		require.Equal(t, req.SessionID, decoded.SessionID)
		require.Equal(t, req.Data1, decoded.Data1)
		require.Equal(t, req.Data2, decoded.Data2)
		require.Equal(t, req.Data3, decoded.Data3)
		require.Equal(t, req.Data4, decoded.Data4)
	})
}

func TestNewDefaultRequestGGAuth_ToBytes(t *testing.T) {
	req := NewDefaultRequestGGAuth(1)

	packetWriter := packet.NewWriter()
	err := req.ToBytes(packetWriter)
	data := packetWriter.Bytes()
	require.NoError(t, err)
	require.NotNil(t, data)

	// Expected byte sequence
	expected := []byte{
		0x07,                   // PacketID
		0x01, 0x00, 0x00, 0x00, // SessionID: 1
		// 23 01 00 00 67 45 00 00 AB 89 00 00 EF CD 00 00
		0x23, 0x01, 0x00, 0x00,
		0x67, 0x45, 0x00, 0x00,
		0xAB, 0x89, 0x00, 0x00,
		0xEF, 0xCD, 0x00, 0x00,
	}
	require.Equal(t, expected, data)
}
