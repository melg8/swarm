// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestActionUsePacketID = 0x45

// Action ids of RequestActionUse known to the bot.
const (
	// ActionSitStand toggles between sitting and standing. The server
	// refuses to sit while moving, casting or attacking, so the bot only
	// sends it while idle (see RequestActionUse.runImpl case 0 and
	// Player.sitDown/standUp of the Mobius server).
	ActionSitStand int32 = 0
)

// RequestActionUsePacket triggers a player action by id, for example the
// sit/stand toggle used by the hunt loop to rest faster.
// Wire format (see RequestActionUse.readImpl): [opcode 0x45]
// [actionId: 4][ctrlPressed: 4][shiftPressed: 1].
type RequestActionUsePacket struct {
	ActionID int32
	Ctrl     bool
	Shift    bool
}

// NewRequestActionUsePacket creates a zero valued action use request.
func NewRequestActionUsePacket() *RequestActionUsePacket {
	return &RequestActionUsePacket{
		ActionID: 0,
		Ctrl:     false,
		Shift:    false,
	}
}

// ToBytes serializes the packet.
func (p *RequestActionUsePacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(requestActionUsePacketID); err != nil {
		return fmt.Errorf("failed to write action use id: %w", err)
	}
	if err := writer.WriteInt32(p.ActionID); err != nil {
		return fmt.Errorf("failed to write action use action id: %w", err)
	}
	ctrl := int32(0)
	if p.Ctrl {
		ctrl = 1
	}
	if err := writer.WriteInt32(ctrl); err != nil {
		return fmt.Errorf("failed to write action use ctrl flag: %w", err)
	}
	shift := int8(0)
	if p.Shift {
		shift = 1
	}
	if err := writer.WriteInt8(shift); err != nil {
		return fmt.Errorf("failed to write action use shift flag: %w", err)
	}

	return nil
}
