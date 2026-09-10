// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const skillListPacketID = 0x6D

// skillListCap bounds the plausible skill count of the packet: the
// biggest C1 class trees carry a few hundred skills, so 512 leaves
// generous headroom while a corrupted count fails fast instead of
// allocating the parse buffer towards gigabytes.
const skillListCap = 512

// SkillListEntry is one learned skill of the packet: the passive flag
// as the server sends it, the learned level and the skill id.
type SkillListEntry struct {
	SkillID int32
	Level   int32
	Passive bool
}

// SkillListPacket is the full skill list of the character, sent on
// entering the world and after every skill learn. The bot uses it to
// render the learned skills of the equipment widget and to compute
// the next lessons of the learning queue.
// Wire format (see SkillList.writeImpl): [opcode 0x6D][count: 4] then
// per skill [passive: 4][level: 4][id: 4].
type SkillListPacket struct {
	Skills []SkillListEntry
}

// NewSkillListPacket creates a packet ready for parsing with a
// reusable entry buffer.
func NewSkillListPacket() *SkillListPacket {
	return &SkillListPacket{
		Skills: make([]SkillListEntry, 0, 16),
	}
}

// ParseSkillListPacket reads the packet from payload bytes.
func ParseSkillListPacket(p *SkillListPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, skillListPacketID); err != nil {
		return err
	}
	count, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read skill count: %w", err)
	}
	if count < 0 || count > skillListCap {
		return fmt.Errorf("implausible skill count %d", count)
	}
	p.Skills = p.Skills[:0]
	for range count {
		passive, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read skill passive flag: %w", err)
		}
		level, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read skill level: %w", err)
		}
		id, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read skill id: %w", err)
		}
		p.Skills = append(p.Skills, SkillListEntry{
			SkillID: id,
			Level:   level,
			Passive: passive != 0,
		})
	}

	return nil
}
