// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const npcInfoPacketID = 0x22

// NpcInfo skipped field groups of the wire format, counted in bytes.
const (
	// npcInfoSpeedLead skips the unknown int, mAtkSpd and pAtkSpd
	// fields between the heading and runSpd.
	npcInfoSpeedLead = 12
	// npcInfoSpeedTrail skips the two swim and the four fly speed
	// ints between walkSpd and the move multiplier (the fly pair is
	// written twice by the server).
	npcInfoSpeedTrail = 24
	// npcInfoAppearanceTail skips the attack speed multiplier, the
	// collision values and the equipment ints between the move
	// multiplier and the flag bytes.
	npcInfoAppearanceTail = 36
)

// NpcInfoPacket describes one npc around the character.
// Wire format (see AbstractNpcInfo.writeImpl): [opcode 0x22]
// [objectId: 4][templateId: 4][isAttackable: 4][x: 4][y: 4][z: 4]
// [heading: 4][unknown: 4][mAtkSpd: 4][pAtkSpd: 4][runSpd: 4][walkSpd: 4]
// [swim/fly speeds: 32][moveMultiplier: 8][attackSpeedMultiplier: 8]
// [collisionRadius: 8][collisionHeight: 8][equipment: 12]
// [nameAbove: 1][running: 1][inCombat: 1][alikeDead: 1][summoned: 1]
// [name: str][title: str].
type NpcInfoPacket struct {
	ObjectID      int32
	TemplateID    int32
	Attackable    bool
	X             int32
	Y             int32
	Z             int32
	Heading       int32
	RunSpeed      int32
	WalkSpeed     int32
	MoveSpeedMult float64
	Running       bool
	InCombat      bool
	Dead          bool
	Name          string
	Title         string
}

// NewNpcInfoPacket creates a zero valued packet ready for parsing.
func NewNpcInfoPacket() *NpcInfoPacket {
	return &NpcInfoPacket{
		ObjectID:      0,
		TemplateID:    0,
		Attackable:    false,
		X:             0,
		Y:             0,
		Z:             0,
		Heading:       0,
		RunSpeed:      0,
		WalkSpeed:     0,
		MoveSpeedMult: 0,
		Running:       false,
		InCombat:      false,
		Dead:          false,
		Name:          "",
		Title:         "",
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
	if err := readInt32Fields(
		reader, &p.X, &p.Y, &p.Z, &p.Heading); err != nil {
		return fmt.Errorf("failed to read npc location: %w", err)
	}
	if err := reader.Skip(npcInfoSpeedLead); err != nil {
		return fmt.Errorf("not enough bytes for npc speeds: %w", err)
	}
	if err := readInt32Fields(reader, &p.RunSpeed, &p.WalkSpeed); err != nil {
		return fmt.Errorf("failed to read npc speeds: %w", err)
	}
	if err := reader.Skip(npcInfoSpeedTrail); err != nil {
		return fmt.Errorf("not enough bytes for npc speeds: %w", err)
	}
	p.MoveSpeedMult, err = reader.ReadFloat64()
	if err != nil {
		return fmt.Errorf("failed to read npc move multiplier: %w", err)
	}
	if err := reader.Skip(npcInfoAppearanceTail); err != nil {
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
	p.Dead = flags[3] != 0

	if p.Name, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("failed to read npc name: %w", err)
	}
	if p.Title, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("failed to read npc title: %w", err)
	}

	return nil
}
