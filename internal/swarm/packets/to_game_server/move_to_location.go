// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const moveToLocationPacketID = 0x01

// MoveMode classifies the origin of a client MoveToLocation request.
const (
    // MoveModeMouse is the movement mode the official client sends when
    // the player clicks a destination on the ground. The server ignores
    // keyboard mode requests unless keyboard movement is enabled, so the
    // bot always walks in mouse mode.
    MoveModeMouse int32 = 1
    // MoveModeCursorKeys is the movement mode the official client
    // sends while the player walks with the arrow keys: the server
    // arms the cursor key movement of the session
    // (MoveToLocation.runImpl adopts the packet origin within the
    // thousand unit window and latches the cursor key flag), and
    // every ValidatePosition the session streams afterwards moves
    // the character server-side without any click validation
    // (ValidatePosition.runImpl syncs the claimed placement into the
    // world and broadcasts it). A server whose geodata seals the
    // standing cell answers no mouse click at all - the cursor key
    // stream is the only movement that still walks (the 2026-09-14
    // 15:10 report: even the official client stood frozen on the
    // village plaza cell until the player walked it out with the
    // arrows).
    MoveModeCursorKeys int32 = 0
)

// MoveToLocationRequestPacket asks the game server to walk the character
// to a world point, exactly like a ground click of the official client.
// The hunt loop uses it to run toward a drop before picking it up.
// Wire format (see MoveToLocation.readImpl): [opcode 0x01][targetX: 4]
// [targetY: 4][targetZ: 4][originX: 4][originY: 4][originZ: 4]
// [movementMode: 4].
type MoveToLocationRequestPacket struct {
    TargetX int32
    TargetY int32
    TargetZ int32
    OriginX int32
    OriginY int32
    OriginZ int32
    Mode    int32
}

// NewMoveToLocationRequestPacket creates a zero valued move request.
func NewMoveToLocationRequestPacket() *MoveToLocationRequestPacket {
    return &MoveToLocationRequestPacket{
        TargetX: 0,
        TargetY: 0,
        TargetZ: 0,
        OriginX: 0,
        OriginY: 0,
        OriginZ: 0,
        Mode:    MoveModeMouse,
    }
}

// ToBytes serializes the packet.
func (p *MoveToLocationRequestPacket) ToBytes(writer *packet.Writer) error {
    if err := writer.WriteInt8(moveToLocationPacketID); err != nil {
        return fmt.Errorf("failed to write move to location id: %w", err)
    }
    if err := writer.WriteInt32(p.TargetX); err != nil {
        return fmt.Errorf("failed to write move target x: %w", err)
    }
    if err := writer.WriteInt32(p.TargetY); err != nil {
        return fmt.Errorf("failed to write move target y: %w", err)
    }
    if err := writer.WriteInt32(p.TargetZ); err != nil {
        return fmt.Errorf("failed to write move target z: %w", err)
    }
    if err := writer.WriteInt32(p.OriginX); err != nil {
        return fmt.Errorf("failed to write move origin x: %w", err)
    }
    if err := writer.WriteInt32(p.OriginY); err != nil {
        return fmt.Errorf("failed to write move origin y: %w", err)
    }
    if err := writer.WriteInt32(p.OriginZ); err != nil {
        return fmt.Errorf("failed to write move origin z: %w", err)
    }
    if err := writer.WriteInt32(p.Mode); err != nil {
        return fmt.Errorf("failed to write move mode: %w", err)
    }

    return nil
}
