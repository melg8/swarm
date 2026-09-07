// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	requestUseItemPacketID  = 0x14
	requestDropItemPacketID = 0x12
)

// RequestUseItem uses an inventory item: equippable items toggle their
// equipped state (see UseItem.runImpl calling useEquippableItem), other
// items run their item handler. The web UI drives it with double clicks
// and drag-and-drop of the equipment widget.
// Wire format (see UseItem.readImpl): [opcode 0x14][objectId: 4]. The
// trailing ctrl flag of later chronicles is not part of the C1 format.
type RequestUseItem struct {
	ObjectID int32
}

// NewRequestUseItem creates a zero valued use item request.
func NewRequestUseItem() *RequestUseItem {
	return &RequestUseItem{ObjectID: 0}
}

// ToBytes serializes the packet.
func (p *RequestUseItem) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(requestUseItemPacketID); err != nil {
		return fmt.Errorf("failed to write use item id: %w", err)
	}
	if err := writer.WriteInt32(p.ObjectID); err != nil {
		return fmt.Errorf("failed to write use item object id: %w", err)
	}

	return nil
}

// RequestDropItem drops an inventory item on the ground at the given
// world position. The server only accepts drops within 150 units of the
// character (see RequestDropItem.runImpl), so the caller passes the
// character position. Stackable items may drop a partial stack through
// the count; the server splits the stack itself.
// Wire format (see RequestDropItem.readImpl): [opcode 0x12]
// [objectId: 4][count: 4][x: 4][y: 4][z: 4].
type RequestDropItem struct {
	ObjectID int32
	Count    int32
	X        int32
	Y        int32
	Z        int32
}

// NewRequestDropItem creates a zero valued drop item request.
func NewRequestDropItem() *RequestDropItem {
	return &RequestDropItem{
		ObjectID: 0,
		Count:    0,
		X:        0,
		Y:        0,
		Z:        0,
	}
}

// ToBytes serializes the packet.
func (p *RequestDropItem) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(requestDropItemPacketID); err != nil {
		return fmt.Errorf("failed to write drop item id: %w", err)
	}
	if err := writer.WriteInt32(p.ObjectID); err != nil {
		return fmt.Errorf("failed to write drop object id: %w", err)
	}
	if err := writer.WriteInt32(p.Count); err != nil {
		return fmt.Errorf("failed to write drop count: %w", err)
	}
	if err := writer.WriteInt32(p.X); err != nil {
		return fmt.Errorf("failed to write drop x: %w", err)
	}
	if err := writer.WriteInt32(p.Y); err != nil {
		return fmt.Errorf("failed to write drop y: %w", err)
	}
	if err := writer.WriteInt32(p.Z); err != nil {
		return fmt.Errorf("failed to write drop z: %w", err)
	}

	return nil
}
