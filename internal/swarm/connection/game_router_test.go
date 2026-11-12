// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "log"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestPacketRouterTable pins the dispatch table: every route answers
// in the indexed table, no opcode is registered twice and the two
// forms carry the same size.
func TestPacketRouterTable(t *testing.T) {
    seen := make(map[byte]bool, len(packetRoutes))
    for _, route := range packetRoutes {
        require.NotNil(t, route.handle, "route 0x%02x has no handler", route.id)
        require.False(t, seen[route.id],
            "route 0x%02x is registered twice", route.id)
        seen[route.id] = true
        require.NotNil(t, packetHandlers[route.id],
            "route 0x%02x is missing from the indexed table", route.id)
    }
    require.Len(t, seen, len(packetRoutes))
}

// TestPacketRouterDispatch pins the routing itself: a registered id
// reaches the handler of its own packet family (the truncated body
// draws that handler's own parse error), the two log only routes
// answer their line, an unregistered id keeps the unknown packet log
// and the empty frame stays silent.
func TestPacketRouterDispatch(t *testing.T) {
    tests := []struct {
        name string
        id   byte
        want string
    }{
        {"unknown id", 0xFE, "Received packet id 0xfe"},
        {"leave world", leaveWorldID, "Server confirmed leave world"},
        {"server close", serverCloseID, "Server is closing the connection"},
        {"user info", userInfoID, "Failed to parse user info"},
        {"npc html", npcHTMLMessageID, "Failed to parse npc html"},
        {"skill list", skillListID, "Failed to parse skill list"},
        {"system message", systemMessageID, "Failed to parse system message"},
        {"movement", moveToLocationID, "Failed to parse movement"},
        {"teleport", teleportID, "Failed to parse teleport"},
        {"attack", attackID, "Failed to parse attack"},
        {"item list", itemListID, "Failed to parse item list"},
        {"net ping", netPingResponseID, "Failed to parse net ping"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            buffer := &testLogBuffer{}
            client := &GameClient{logger: log.New(buffer, "", 0)}

            client.handleServerPacket([]byte{tt.id})

            require.Contains(t, buffer.String(), tt.want)
        })
    }

    buffer := &testLogBuffer{}
    client := &GameClient{logger: log.New(buffer, "", 0)}
    client.handleServerPacket(nil)
    require.Empty(t, buffer.String(), "the zero length frame stays silent")
}
