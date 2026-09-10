// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const abnormalStatusUpdatePacketID = 0x97

// abnormalBuffCap bounds the plausible buff count of the packet: the
// C1 buff window holds at most twenty slots, so 64 leaves generous
// headroom while a corrupted count fails fast instead of allocating
// the parse buffer towards gigabytes.
const abnormalBuffCap = 64

// BuffEntry is one active effect of the packet: the skill id (the
// display id of the C1 skills is the skill id itself), its level and
// the remaining seconds (a large value - 2147483647 - marks the
// effects the server never times out).
type BuffEntry struct {
	SkillID int32
	Level   int32
	Time    int32
}

// AbnormalStatusUpdatePacket is the active effect list of the
// character, sent whenever the effects change: a buff lands, a debuff
// expires, the last of a group leaves. The bot tracks the running
// buffs through it - the self buff casting of the hunt loop skips the
// buffs that already run, the web UI buffs widget renders the list
// with the remaining durations.
// Wire format (see AbnormalStatusUpdate.writeImpl): [opcode 0x97]
// [count: 2] then per effect [skillId: 4][level: 2][time: 4].
type AbnormalStatusUpdatePacket struct {
	Buffs []BuffEntry
}

// NewAbnormalStatusUpdatePacket creates a packet ready for parsing
// with a reusable entry buffer.
func NewAbnormalStatusUpdatePacket() *AbnormalStatusUpdatePacket {
	return &AbnormalStatusUpdatePacket{
		Buffs: make([]BuffEntry, 0, 8),
	}
}

// ParseAbnormalStatusUpdatePacket reads the packet from payload bytes.
func ParseAbnormalStatusUpdatePacket(
	p *AbnormalStatusUpdatePacket, data []byte,
) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(
		reader, abnormalStatusUpdatePacketID); err != nil {
		return err
	}
	count, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read buff count: %w", err)
	}
	if count < 0 || count > abnormalBuffCap {
		return fmt.Errorf("implausible buff count %d", count)
	}
	p.Buffs = p.Buffs[:0]
	for range count {
		skillID, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf(
				"failed to read buff skill id: %w", err)
		}
		level, err := reader.ReadInt16()
		if err != nil {
			return fmt.Errorf(
				"failed to read buff level: %w", err)
		}
		time, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf(
				"failed to read buff time: %w", err)
		}
		p.Buffs = append(p.Buffs, BuffEntry{
			SkillID: skillID,
			Level:   int32(level),
			Time:    time,
		})
	}

	return nil
}
