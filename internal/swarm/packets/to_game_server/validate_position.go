// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Every outbound packet file repeats the const + struct + New +
// linear ToBytes shape the packet recipe mandates.
//
//nolint:dupl // mirrors AttackRequestPacket
package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const validatePositionPacketID = 0x48

// ValidatePositionPacket reports the client side position of the
// character to the game server, exactly like the periodic validation
// of the official client (ValidatePosition 0x48, see
// gameserver/network/clientpackets/ValidatePosition.java). The server
// uses it to adopt the client z within its tolerance band, to refresh
// the last server position the door logout exploit check compares
// against and to keep its clientX/clientY/clientZ view of the session
// fresh - a session that never validates leaves that view at zero
// forever, so every server side branch that reads it (the z adoption
// gate subtracts the client z) answers for a client that never spoke.
// Wire format: [opcode 0x48][x: 4][y: 4][z: 4][heading: 4]
// [vehicleId: 4].
type ValidatePositionPacket struct {
    X       int32
    Y       int32
    Z       int32
    Heading int32
    Vehicle int32
}

// NewValidatePositionPacket creates a zero valued validation request.
func NewValidatePositionPacket() *ValidatePositionPacket {
    return &ValidatePositionPacket{
        X:       0,
        Y:       0,
        Z:       0,
        Heading: 0,
        Vehicle: 0,
    }
}

// ToBytes serializes the packet.
func (p *ValidatePositionPacket) ToBytes(writer *packet.Writer) error {
    if err := writer.WriteInt8(validatePositionPacketID); err != nil {
        return fmt.Errorf("failed to write validate position id: %w", err)
    }
    if err := writer.WriteInt32(p.X); err != nil {
        return fmt.Errorf("failed to write validate x: %w", err)
    }
    if err := writer.WriteInt32(p.Y); err != nil {
        return fmt.Errorf("failed to write validate y: %w", err)
    }
    if err := writer.WriteInt32(p.Z); err != nil {
        return fmt.Errorf("failed to write validate z: %w", err)
    }
    if err := writer.WriteInt32(p.Heading); err != nil {
        return fmt.Errorf("failed to write validate heading: %w", err)
    }
    if err := writer.WriteInt32(p.Vehicle); err != nil {
        return fmt.Errorf("failed to write validate vehicle: %w", err)
    }

    return nil
}
