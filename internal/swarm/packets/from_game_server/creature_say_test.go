// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

func TestParseCreatureSayPacket(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(creatureSayPacketID))
    require.NoError(t, writer.WriteInt32(1234))
    require.NoError(t, writer.WriteInt32(1))
    require.NoError(t, writer.WriteStringAsUtf16("Melg"))
    require.NoError(t, writer.WriteStringAsUtf16("Hello world"))

    p := NewCreatureSayPacket()
    require.NoError(t, ParseCreatureSayPacket(p, writer.Bytes()))
    require.Equal(t, int32(1234), p.ObjectID)
    require.Equal(t, int32(1), p.Channel)
    require.Equal(t, "Melg", p.From)
    require.Equal(t, "Hello world", p.Text)
}

func TestParseCreatureSayPacketEmptyText(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(creatureSayPacketID))
    require.NoError(t, writer.WriteInt32(7))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteStringAsUtf16("Someone"))
    require.NoError(t, writer.WriteStringAsUtf16(""))

    p := NewCreatureSayPacket()
    require.NoError(t, ParseCreatureSayPacket(p, writer.Bytes()))
    require.Equal(t, "Someone", p.From)
    require.Empty(t, p.Text)
}

func TestParseCreatureSayPacketRejectsWrongID(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(systemMessagePacketID))
    require.NoError(t, writer.WriteInt32(1))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteStringAsUtf16("Melg"))
    require.NoError(t, writer.WriteStringAsUtf16("Hello"))

    p := NewCreatureSayPacket()
    require.Error(t, ParseCreatureSayPacket(p, writer.Bytes()))
}

func TestParseCreatureSayPacketRejectsTruncated(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(creatureSayPacketID))
    require.NoError(t, writer.WriteInt32(1234))

    p := NewCreatureSayPacket()
    require.Error(t, ParseCreatureSayPacket(p, writer.Bytes()))
}
