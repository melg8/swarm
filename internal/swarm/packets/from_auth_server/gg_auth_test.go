// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"bytes"
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestNewGGAuthPacketFromBytes(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		want        *GGAuthPacket
		wantErr     bool
		description string
	}{
		{
			name:  "valid packet",
			input: []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00},
			want: &GGAuthPacket{
				SessionID: 1,
				Unknown:   2,
			},
			wantErr:     false,
			description: "should successfully parse valid input",
		},
		{
			name:        "incomplete data for SessionID",
			input:       []byte{0x01, 0x00, 0x00},
			want:        nil,
			wantErr:     true,
			description: "should return error when not enough bytes for SessionID",
		},
		{
			name:        "incomplete data for Unknown",
			input:       []byte{0x01, 0x00, 0x00, 0x00, 0x02},
			want:        nil,
			wantErr:     true,
			description: "should return error when not enough bytes for Unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGGAuthPacketFromBytes(tt.input)
			if tt.wantErr {
				require.Error(t, err, tt.description)

				return
			}
			require.NoError(t, err, tt.description)
			require.Equal(t, tt.want, got, tt.description)
		})
	}
}

func TestGGAuthPacket_ToBytes(t *testing.T) {
	tests := []struct {
		name        string
		packet      *GGAuthPacket
		want        []byte
		wantErr     bool
		description string
	}{
		{
			name: "valid conversion",
			packet: &GGAuthPacket{
				SessionID: 1,
				Unknown:   2,
			},
			want:        []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00},
			wantErr:     false,
			description: "should successfully convert to bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packetWriter := packet.NewWriter()

			err := tt.packet.ToBytes(packetWriter)
			if tt.wantErr {
				require.Error(t, err, tt.description)

				return
			}
			require.NoError(t, err, tt.description)
			require.True(t, bytes.Equal(tt.want, packetWriter.Bytes()), tt.description)
		})
	}
}

func TestGGAuthPacket_ToString(t *testing.T) {
	packet := &GGAuthPacket{
		SessionID: 1,
		Unknown:   2,
	}
	result := packet.ToString()
	require.Contains(t, result, "GGAuthPacket")
	require.Contains(t, result, "SessionID: 00000001")
	require.Contains(t, result, "Unknown: 00000002")
}

func TestGGAuthPacket_RoundTrip(t *testing.T) {
	original := &GGAuthPacket{
		SessionID: 12345,
		Unknown:   67890,
	}

	// Convert to bytes
	packetWriter := packet.NewWriter()
	err := original.ToBytes(packetWriter)
	require.NoError(t, err, "Failed to convert to bytes")

	// Convert back to packet
	reconstructed, err := NewGGAuthPacketFromBytes(packetWriter.Bytes())
	require.NoError(t, err, "Failed to parse bytes")

	// Compare
	require.Equal(t, original.SessionID, reconstructed.SessionID, "SessionID mismatch after round trip")
	require.Equal(t, original.Unknown, reconstructed.Unknown, "Unknown field mismatch after round trip")
}
