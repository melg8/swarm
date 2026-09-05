// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestRestartPointPacketID = 0x6D

// Restart point types of RequestRestartPoint.
const (
	// RestartTypeVillage is the "restart in the nearest village" choice
	// of the death dialog: the server revives the character at the
	// village restart point with restored vitals.
	RestartTypeVillage int32 = 0
)

// RequestRestartPointPacket revives a dead character at the chosen
// restart point, exactly like the death dialog of the official client.
// The server accepts it only while the character is dead.
// Wire format (see RequestRestartPoint.readImpl): [opcode 0x6D]
// [pointType: 4].
type RequestRestartPointPacket struct {
	PointType int32
}

// NewRequestRestartPointPacket creates a zero valued restart request.
func NewRequestRestartPointPacket() *RequestRestartPointPacket {
	return &RequestRestartPointPacket{PointType: RestartTypeVillage}
}

// ToBytes serializes the packet.
func (p *RequestRestartPointPacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(requestRestartPointPacketID); err != nil {
		return fmt.Errorf("failed to write restart point id: %w", err)
	}
	if err := writer.WriteInt32(p.PointType); err != nil {
		return fmt.Errorf("failed to write restart point type: %w", err)
	}

	return nil
}
