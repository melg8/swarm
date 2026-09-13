// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// TestChangeMoveTypePacketLayout pins the wire form of the movement
// toggle: the 0x1C opcode and the little endian run flag of the
// ChangeMoveType2.readImpl (the server starts every session walking,
// the official client flips it to running on entry).
func TestChangeMoveTypePacketLayout(t *testing.T) {
    request := NewChangeMoveTypePacket()
    request.TypeRun = 1
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{0x1C, 0x01, 0x00, 0x00, 0x00},
        writer.Bytes())
}

// TestChangeMoveTypePacketWalk pins the walk form: a zero flag asks
// the server to walk.
func TestChangeMoveTypePacketWalk(t *testing.T) {
    request := NewChangeMoveTypePacket()
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{0x1C, 0x00, 0x00, 0x00, 0x00},
        writer.Bytes())
}

func BenchmarkChangeMoveTypePacket(b *testing.B) {
    request := NewChangeMoveTypePacket()
    request.TypeRun = 1
    b.ResetTimer()
    for range b.N {
        writer := packet.NewWriter()
        _ = request.ToBytes(writer)
    }
}
