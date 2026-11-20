// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "io"
    "log"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// newPartyClient builds a game client over a tracker without a socket: the
// inbound party handlers run on the reader goroutine and need no session.
func newPartyClient(tracker *state.Bot) *GameClient {
    return &GameClient{
        logger:  log.New(io.Discard, "", 0),
        tracker: tracker,
    }
}

// TestPartyHandlersMirrorTheWindow drives the party packets through the
// dispatch table: the ask, the joined member, the whole window, the placement
// and the party end.
func TestPartyHandlersMirrorTheWindow(t *testing.T) {
    tracker := state.NewBot("test2")
    client := newPartyClient(tracker)

    ask := appendInt32([]byte{0x4B}, 100)
    ask = appendInt32(ask, 0)
    client.handleServerPacket(ask)
    requestor, _ := tracker.PendingPartyAsk()
    require.Equal(t, int32(100), requestor)
    tracker.ClearPartyAsk()

    add := appendInt32([]byte{0x64}, 100)
    add = appendUTF16(add, "test1")
    for _, value := range []int32{100, 200, 50, 60, 18, 10, 0, 0} {
        add = appendInt32(add, value)
    }
    client.handleServerPacket(add)
    require.Equal(t, 2, tracker.PartyCount())
    member, ok := tracker.PartyMember(100)
    require.True(t, ok)
    require.Equal(t, "test1", member.Name)
    require.Equal(t, int32(10), member.Level)

    all := appendInt32([]byte{0x63}, 2)
    for _, objectID := range []int32{100, 200} {
        all = appendInt32(all, objectID)
        all = appendUTF16(all, "member")
        for _, value := range []int32{100, 200, 50, 60, 18, 0, 1} {
            all = appendInt32(all, value)
        }
    }
    client.handleServerPacket(all)
    require.Equal(t, 3, tracker.PartyCount())

    positions := appendInt32([]byte{0xC0}, 2)
    for _, point := range [][3]int32{{100, 45000, 50000}, {200, 44900, 50010}} {
        positions = appendInt32(positions, point[0])
        positions = appendUTF16(positions, "member")
        for _, value := range []int32{10, 18, 0, 0, 0, point[1], point[2],
            -3500} {
            positions = appendInt32(positions, value)
        }
    }
    client.handleServerPacket(positions)
    x, y, z, ok := tracker.ObjectPosition(100)
    require.True(t, ok)
    require.Equal(t, int32(45000), x)
    require.Equal(t, int32(50000), y)
    require.Equal(t, int32(-3500), z)

    client.handleServerPacket([]byte{0x65})
    require.Equal(t, 1, tracker.PartyCount())
}

// appendUTF16 appends a null terminated UTF-16LE string.
func appendUTF16(dst []byte, text string) []byte {
    for _, r := range text {
        dst = append(dst, byte(r), byte(r>>8))
    }

    return append(dst, 0, 0)
}
