// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestParseSocialActionPacket(t *testing.T) {
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(socialActionPacketID))
	require.NoError(t, writer.WriteInt32(268450291))
	require.NoError(t, writer.WriteInt32(SocialActionLevelUp))

	p := NewSocialActionPacket()
	require.NoError(t, ParseSocialActionPacket(p, writer.Bytes()))
	require.Equal(t, int32(268450291), p.ObjectID)
	require.Equal(t, int32(SocialActionLevelUp), p.ActionID)
}

func TestParseSystemMessagePacket(t *testing.T) {
	// id 28 "You picked up $s1 adena." with one int parameter.
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(systemMessagePacketID))
	require.NoError(t, writer.WriteInt32(28))
	require.NoError(t, writer.WriteInt32(1))
	require.NoError(t, writer.WriteInt32(sysParamIntNumber))
	require.NoError(t, writer.WriteInt32(25))

	p := NewSystemMessagePacket()
	require.NoError(t, ParseSystemMessagePacket(p, writer.Bytes()))
	require.Equal(t, int32(28), p.MessageID)
	require.Len(t, p.Params, 1)
	require.Equal(t, int32(sysParamIntNumber), p.Params[0].Type)
	require.Equal(t, int32(25), p.Params[0].Int)

	// Reuse: the second parse replaces the parameters.
	writer2 := packet.NewWriter()
	require.NoError(t, writer2.WriteInt8(systemMessagePacketID))
	require.NoError(t, writer2.WriteInt32(30))
	require.NoError(t, writer2.WriteInt32(2))
	require.NoError(t, writer2.WriteInt32(sysParamItemName))
	require.NoError(t, writer2.WriteInt32(1121))
	require.NoError(t, writer2.WriteInt32(sysParamText))
	require.NoError(t, writer2.WriteStringAsUtf16("Wooden Arrow"))

	require.NoError(t, ParseSystemMessagePacket(p, writer2.Bytes()))
	require.Equal(t, int32(30), p.MessageID)
	require.Len(t, p.Params, 2)
	require.Equal(t, int32(1121), p.Params[0].Int)
	require.Equal(t, "Wooden Arrow", p.Params[1].Text)
}

func TestParseSystemMessagePacketRejectsImplausibleCount(t *testing.T) {
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(systemMessagePacketID))
	require.NoError(t, writer.WriteInt32(28))
	require.NoError(t, writer.WriteInt32(1000))

	p := NewSystemMessagePacket()
	require.Error(t, ParseSystemMessagePacket(p, writer.Bytes()))
}
