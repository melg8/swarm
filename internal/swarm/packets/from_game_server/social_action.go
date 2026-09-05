// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const socialActionPacketID = 0x3D

// SocialActionLevelUp is the social action id the server broadcasts
// when any character reaches a new level (SocialAction.LEVEL_UP). The
// remaining ids are the idle animations of npcs and the client social
// commands of players.
const SocialActionLevelUp = 15

// SocialActionPacket announces that a creature around the character
// plays a social animation (an idle gesture of an npc, a player emote
// or the level up animation).
// Wire format (see SocialAction.writeImpl): [opcode 0x3D]
// [objectId: 4][actionId: 4].
type SocialActionPacket struct {
	ObjectID int32
	ActionID int32
}

// NewSocialActionPacket creates a zero valued packet ready for parsing.
func NewSocialActionPacket() *SocialActionPacket {
	return &SocialActionPacket{ObjectID: 0, ActionID: 0}
}

// ParseSocialActionPacket reads the packet from payload bytes.
func ParseSocialActionPacket(p *SocialActionPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, socialActionPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID, &p.ActionID); err != nil {
		return fmt.Errorf("failed to read social action: %w", err)
	}

	return nil
}
