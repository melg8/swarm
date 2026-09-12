// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const npcHTMLMessagePacketID = 0x1B

// NpcHTMLMessage is the server html dialog packet a gatekeeper (or any
// npc) sends in response to a bypass or a talk. The bot reads the html
// to find the bypass buttons of the teleport list (the
// "npc_<objId>_teleport <list> <index>" commands the H-002 verification
// drives), then sends RequestBypassToServer with the chosen command.
//
// Wire format:
//
//	[opcode 0x1B][npcObjId: 4][html: null-terminated UTF-16LE]
//	[itemId: 4]
//
// The itemId is non-zero when the dialog is an item html (the
// HtmlActionScope NpcItemHtml); zero for the npc html scope the
// gatekeeper uses. Reference: org/l2jmobius/gameserver/network/
// serverpackets/NpcHtmlMessage.java (writeImpl).
type NpcHTMLMessage struct {
	NpcObjID int32
	HTML     string
	ItemID   int32
}

// NewNpcHTMLMessage builds a zero-valued packet ready for parsing.
func NewNpcHTMLMessage() *NpcHTMLMessage {
	return &NpcHTMLMessage{
		NpcObjID: 0,
		HTML:     "",
		ItemID:   0,
	}
}

// ParseNpcHTMLMessage reads the packet from the payload bytes. The
// opcode is verified first, then the npc object id, the html string
// and the trailing item id.
func ParseNpcHTMLMessage(p *NpcHTMLMessage, data []byte) error {
	reader := packet.NewReader(data)
	if err := expectPacketID(reader, npcHTMLMessagePacketID); err != nil {
		return err
	}
	var err error
	if p.NpcObjID, err = reader.ReadInt32(); err != nil {
		return fmt.Errorf("npc html npc obj id: %w", err)
	}
	if p.HTML, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("npc html body: %w", err)
	}
	if p.ItemID, err = reader.ReadInt32(); err != nil {
		return fmt.Errorf("npc html item id: %w", err)
	}

	return nil
}
