// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const changeWaitTypePacketID = 0x3F

// Wait type values of the ChangeWaitType packet (ChangeWaitType enum of
// the Mobius server).
const (
	waitTypeSitting  = 0
	waitTypeStanding = 1
)

// ChangeWaitTypePacket announces the sit/stand transition of a creature.
// The broadcast goes through Player.broadcastPacket, so the acting client
// receives its own transitions.
// Wire format (see ChangeWaitType.writeImpl): [opcode 0x3F][objectId: 4]
// [moveType: 4][x: 4][y: 4][z: 4] with moveType 0 for sitting and 1 for
// standing.
type ChangeWaitTypePacket struct {
	ObjectID int32
	Sitting  bool
}

// NewChangeWaitTypePacket creates a zero valued packet ready for parsing.
func NewChangeWaitTypePacket() *ChangeWaitTypePacket {
	return &ChangeWaitTypePacket{ObjectID: 0, Sitting: false}
}

// ParseChangeWaitTypePacket reads the packet from payload bytes.
func ParseChangeWaitTypePacket(p *ChangeWaitTypePacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, changeWaitTypePacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read wait type object id: %w", err)
	}
	moveType, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read wait type: %w", err)
	}
	switch moveType {
	case waitTypeSitting:
		p.Sitting = true
	case waitTypeStanding:
		p.Sitting = false
	default:
		// Fake death transitions and other values do not concern the
		// sitting state; treat them as standing so a lost packet can
		// never leave the bot sitting forever.
		p.Sitting = false
	}

	return nil
}
