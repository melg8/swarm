// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// TestValidatePositionPacketLayout pins the wire form of the client
// position validation: the 0x48 opcode and the little endian x, y, z,
// heading, vehicle fields of ValidatePosition.readImpl (the server
// refreshes its clientX/clientY/clientZ session view and the last
// server position of the door logout exploit check from it).
func TestValidatePositionPacketLayout(t *testing.T) {
    request := NewValidatePositionPacket()
    request.X = 45768
    request.Y = 49848
    request.Z = -3056
    request.Heading = 32114
    request.Vehicle = 0
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x48,
        0xC8, 0xB2, 0x00, 0x00, // x 45768
        0xB8, 0xC2, 0x00, 0x00, // y 49848
        0x10, 0xF4, 0xFF, 0xFF, // z -3056
        0x72, 0x7D, 0x00, 0x00, // heading 32114
        0x00, 0x00, 0x00, 0x00, // vehicle 0
    }, writer.Bytes())
}

// TestValidatePositionPacketZero pins the fresh constructor: every
// field zero, the vehicle id included (the C1 client always streams
// the same shape while its character stands and moves).
func TestValidatePositionPacketZero(t *testing.T) {
    request := NewValidatePositionPacket()
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    expected := append([]byte{0x48}, make([]byte, 20)...)
    require.Equal(t, expected, writer.Bytes())
}

func BenchmarkValidatePositionPacket(b *testing.B) {
    request := NewValidatePositionPacket()
    request.X = 45768
    request.Y = 49848
    request.Z = -3056
    request.Heading = 32114
    b.ResetTimer()
    for range b.N {
        writer := packet.NewWriter()
        _ = request.ToBytes(writer)
    }
}
