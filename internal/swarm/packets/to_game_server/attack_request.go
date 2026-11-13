// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//nolint:dupl // the packet file mirrors ValidatePositionPacket by design
package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const attackRequestPacketID = 0x0A

// AttackRequestPacket asks the game server to attack a target. The server moves
// the character to the target and starts the auto attack when the target is
// reachable. Wire format (see AttackRequest.readImpl): [opcode 0x0A][objectId:
// 4] [originX: 4][originY: 4][originZ: 4][shift: 1].
type AttackRequestPacket struct {
    TargetID int32
    X        int32
    Y        int32
    Z        int32
    Shift    int8
}

func NewAttackRequestPacket() *AttackRequestPacket {
    return &AttackRequestPacket{
        TargetID: 0,
        X:        0,
        Y:        0,
        Z:        0,
        Shift:    0,
    }
}

// ToBytes serializes the packet field by field (the packet convention);
// sharing a builder with ValidatePositionPacket would hide the per packet
// layout the wire format documents.
func (p *AttackRequestPacket) ToBytes(writer *packet.Writer) error {
    if err := writer.WriteInt8(attackRequestPacketID); err != nil {
        return fmt.Errorf("failed to write attack request id: %w", err)
    }
    if err := writer.WriteInt32(p.TargetID); err != nil {
        return fmt.Errorf("failed to write attack target id: %w", err)
    }
    if err := writer.WriteInt32(p.X); err != nil {
        return fmt.Errorf("failed to write attack origin x: %w", err)
    }
    if err := writer.WriteInt32(p.Y); err != nil {
        return fmt.Errorf("failed to write attack origin y: %w", err)
    }
    if err := writer.WriteInt32(p.Z); err != nil {
        return fmt.Errorf("failed to write attack origin z: %w", err)
    }
    if err := writer.WriteInt8(p.Shift); err != nil {
        return fmt.Errorf("failed to write attack shift flag: %w", err)
    }

    return nil
}
