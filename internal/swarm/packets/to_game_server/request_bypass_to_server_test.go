// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

// TestRequestBypassToServerToBytes pins the wire format of the
// gatekeeper bypass: the 0x21 opcode followed by the null-terminated
// UTF-16LE command the Teleporter.onBypassFeedback chain reads.
func TestRequestBypassToServerToBytes(t *testing.T) {
	writer := packet.NewWriter()
	request := NewRequestBypassToServer()
	request.Command = "npc_30146_Chat"
	err := request.ToBytes(writer)
	require.NoError(t, err)
	require.Equal(t, []byte{
		0x21, // opcode
		'n', 0, 'p', 0, 'c', 0, '_', 0,
		'3', 0, '0', 0, '1', 0, '4', 0, '6', 0,
		'_', 0, 'C', 0, 'h', 0, 'a', 0, 't', 0,
		0, 0, // null terminator
	}, writer.Bytes())
}

// TestRequestBypassToServerTeleportCommand pins the teleport bypass
// shape the gatekeeper flow sends: "npc_<objId>_teleport <list> <idx>".
func TestRequestBypassToServerTeleportCommand(t *testing.T) {
	writer := packet.NewWriter()
	request := NewRequestBypassToServer()
	request.Command = "npc_30146_teleport 1 0"
	err := request.ToBytes(writer)
	require.NoError(t, err)
	// The first byte is the opcode; the last two are the null
	// terminator; the middle is the UTF-16LE command body.
	bytes := writer.Bytes()
	require.Equal(t, byte(0x21), bytes[0])
	require.Equal(t, []byte{0, 0}, bytes[len(bytes)-2:])
	// The command round-trips through a reader.
	reader := packet.NewReader(bytes)
	_, err = reader.ReadInt8()
	require.NoError(t, err)
	command, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, "npc_30146_teleport 1 0", command)
}

// TestRequestBypassToServerEmptyRefused pins the guard: an empty
// command is refused (the server disconnects on an empty bypass).
func TestRequestBypassToServerEmptyRefused(t *testing.T) {
	writer := packet.NewWriter()
	request := NewRequestBypassToServer()
	request.Command = ""
	err := request.ToBytes(writer)
	require.Error(t, err)
}
