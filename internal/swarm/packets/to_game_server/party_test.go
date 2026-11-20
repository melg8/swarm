// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// TestRequestJoinPartyToBytes pins the invitation layout: opcode 0x29, the
// target object id and the finders keepers distribution type.
func TestRequestJoinPartyToBytes(t *testing.T) {
    request := NewRequestJoinParty()
    request.ObjectID = 268435456

    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t,
        []byte{0x29, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00},
        writer.Bytes())
}

// TestRequestAnswerJoinPartyToBytes pins the answer layout: opcode 0x2A and
// the response value.
func TestRequestAnswerJoinPartyToBytes(t *testing.T) {
    request := NewRequestAnswerJoinParty()
    request.Response = PartyAnswerAccept

    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{0x2A, 0x01, 0x00, 0x00, 0x00},
        writer.Bytes())
}
