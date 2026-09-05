// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	actionPacketID             = 0x04
	requestDestroyItemPacketID = 0x59
	requestItemListPacketID    = 0x0F
)

// ActionRequestPacket clicks a world object: a simple click on a ground
// item makes the server walk the character to it and pick it up, a click
// on a creature selects it as the target. The origin coordinates are the
// client side position of the character.
// Wire format (see Action.readImpl): [opcode 0x04][objectId: 4]
// [originX: 4][originY: 4][originZ: 4][actionId: 1] with actionId 0 for
// a simple click and 1 for a shift click.
type ActionRequestPacket struct {
	ObjectID int32
	X        int32
	Y        int32
	Z        int32
	Shift    int8
}

// NewActionRequestPacket creates a zero valued action request.
func NewActionRequestPacket() *ActionRequestPacket {
	return &ActionRequestPacket{
		ObjectID: 0,
		X:        0,
		Y:        0,
		Z:        0,
		Shift:    0,
	}
}

// ToBytes serializes the packet.
func (p *ActionRequestPacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(actionPacketID); err != nil {
		return fmt.Errorf("failed to write action request id: %w", err)
	}
	if err := writer.WriteInt32(p.ObjectID); err != nil {
		return fmt.Errorf("failed to write action object id: %w", err)
	}
	if err := writer.WriteInt32(p.X); err != nil {
		return fmt.Errorf("failed to write action origin x: %w", err)
	}
	if err := writer.WriteInt32(p.Y); err != nil {
		return fmt.Errorf("failed to write action origin y: %w", err)
	}
	if err := writer.WriteInt32(p.Z); err != nil {
		return fmt.Errorf("failed to write action origin z: %w", err)
	}
	if err := writer.WriteInt8(p.Shift); err != nil {
		return fmt.Errorf("failed to write action shift flag: %w", err)
	}

	return nil
}

// RequestDestroyItem destroys inventory items to free slots or weight,
// used by the hunt loop to keep the inventory of a long living bot
// working.
// Wire format (see RequestDestroyItem.readImpl): [opcode 0x59]
// [objectId: 4][count: 4].
type RequestDestroyItem struct {
	ObjectID int32
	Count    int32
}

// NewRequestDestroyItem creates a zero valued destroy request.
func NewRequestDestroyItem() *RequestDestroyItem {
	return &RequestDestroyItem{ObjectID: 0, Count: 0}
}

// ToBytes serializes the packet.
func (p *RequestDestroyItem) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(requestDestroyItemPacketID); err != nil {
		return fmt.Errorf("failed to write destroy item id: %w", err)
	}
	if err := writer.WriteInt32(p.ObjectID); err != nil {
		return fmt.Errorf("failed to write destroy object id: %w", err)
	}
	if err := writer.WriteInt32(p.Count); err != nil {
		return fmt.Errorf("failed to write destroy count: %w", err)
	}

	return nil
}

// RequestItemList asks the server for the full inventory list.
// Wire format (see RequestItemList.readImpl): [opcode 0x0F].
type RequestItemList struct{}

// ToBytes serializes the packet.
func (p *RequestItemList) ToBytes(writer *packet.Writer) error {
	return writer.WriteInt8(requestItemListPacketID)
}
