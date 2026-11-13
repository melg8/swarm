// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "errors"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestBypassToServerPacketID = 0x21

// RequestBypassToServer routes a html dialog bypass command to the server (the
// gatekeeper teleport flow): the client sends the bypass string the
// NpcHtmlMessage button carried, and the server runs the matching
// BypassHandler or the npc onBypass feedback. The bot sends
// "npc_<objId>_Chat" for the next page and "npc_<objId>_teleport <list>
// <index>" to pay the fee and teleport.
// Wire format: [opcode 0x21][command: null-terminated UTF-16LE]
// (RequestBypassToServer.readImpl).
type RequestBypassToServer struct {
    Command string
}

// NewRequestBypassToServer builds a zero-valued packet ready to fill.
func NewRequestBypassToServer() *RequestBypassToServer {
    return &RequestBypassToServer{Command: ""}
}

// ToBytes serializes the packet. An empty command is refused: the
// server logs it and disconnects the client on an empty bypass (see
// RequestBypassToServer.runImpl), so the bot never sends one.
func (p *RequestBypassToServer) ToBytes(writer *packet.Writer) error {
    if p.Command == "" {
        return errors.New("bypass command is empty")
    }
    if err := writer.WriteInt8(requestBypassToServerPacketID); err != nil {
        return err
    }

    return writer.WriteStringAsUtf16(p.Command)
}
