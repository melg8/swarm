// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const npcInfoPacketID = 0x22

// npcInfoSkippedBytes skips the unknown int, speeds, multipliers and
// equipment fields between the heading and the flag bytes.
const npcInfoSkippedBytes = 88

// NpcInfoPacket describes one npc around the character.
// Wire format (see AbstractNpcInfo.writeImpl): [opcode 0x22]
// [objectId: 4][templateId: 4][isAttackable: 4][x: 4][y: 4][z: 4]
// [heading: 4][88 bytes: speeds, multipliers, equipment]
// [nameAbove: 1][running: 1][inCombat: 1][alikeDead: 1][summoned: 1]
// [name: str][title: str].
type NpcInfoPacket struct {
	ObjectID   int32
	TemplateID int32
	Attackable bool
	X          int32
	Y          int32
	Z          int32
	Heading    int32
	Running    bool
	InCombat   bool
	Name       string
	Title      string
}

// NewNpcInfoPacket creates a zero valued packet ready for parsing.
func NewNpcInfoPacket() *NpcInfoPacket {
	return &NpcInfoPacket{
		ObjectID:   0,
		TemplateID: 0,
		Attackable: false,
		X:          0,
		Y:          0,
		Z:          0,
		Heading:    0,
		Running:    false,
		InCombat:   false,
		Name:       "",
		Title:      "",
	}
}

// ParseNpcInfoPacket reads the packet from payload bytes.
func ParseNpcInfoPacket(p *NpcInfoPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, npcInfoPacketID); err != nil {
		return err
	}

	if err := readInt32Fields(reader, &p.ObjectID, &p.TemplateID); err != nil {
		return fmt.Errorf("failed to read npc identity: %w", err)
	}
	attackable, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read npc attackable flag: %w", err)
	}
	p.Attackable = attackable != 0
	if err := readInt32Fields(reader, &p.X, &p.Y, &p.Z, &p.Heading); err != nil {
		return fmt.Errorf("failed to read npc location: %w", err)
	}
	if err := reader.Skip(npcInfoSkippedBytes); err != nil {
		return fmt.Errorf("not enough bytes for npc fields: %w", err)
	}

	var flags [5]int8
	for i := range flags {
		flag, err := reader.ReadInt8()
		if err != nil {
			return fmt.Errorf("not enough bytes for npc flags: %w", err)
		}
		flags[i] = flag
	}
	p.Running = flags[1] != 0
	p.InCombat = flags[2] != 0

	if p.Name, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("failed to read npc name: %w", err)
	}
	if p.Title, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("failed to read npc title: %w", err)
	}

	return nil
}
