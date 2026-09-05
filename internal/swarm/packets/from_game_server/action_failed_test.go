// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

func TestParseActionFailedPacket(t *testing.T) {
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(actionFailedPacketID))

	p := NewActionFailedPacket()
	require.NoError(t, ParseActionFailedPacket(p, writer.Bytes()))
}

func TestParseActionFailedPacketRejectsWrongID(t *testing.T) {
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(socialActionPacketID))

	p := NewActionFailedPacket()
	require.Error(t, ParseActionFailedPacket(p, writer.Bytes()))
}
