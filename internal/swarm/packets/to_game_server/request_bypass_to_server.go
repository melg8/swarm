// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"errors"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestBypassToServerPacketID = 0x21

// RequestBypassToServer routes a html dialog bypass command to the
// server (the gatekeeper teleport flow, the H-002 verification): the
// client sends the bypass string the NpcHtmlMessage button carried,
// the server runs the matching BypassHandler or the npc_ onBypass
// feedback. The command shapes the bot uses:
//
//   - "npc_<objId>_Chat" opens the next html page of the npc.
//   - "npc_<objId>_teleport <list> <index>" pays the fee and teleports
//     to the destination of the teleporter list.
//
// Wire format: [opcode 0x21][command: null-terminated UTF-16LE].
// Reference: org/l2jmobius/gameserver/network/clientpackets/
// RequestBypassToServer.java (readImpl reads the single string).
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
