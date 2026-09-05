// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"errors"
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const statusUpdatePacketID = 0x1A

// statusUpdateMaxAttrs bounds the stored attributes; packets with more
// pairs are fully consumed but the tail pairs are ignored.
const statusUpdateMaxAttrs = 8

// statusUpdateMaxPairs bounds the pair count to reject corrupt packets.
const statusUpdateMaxPairs = 64

// StatusUpdatePacket carries vitals attribute changes of one object.
// Wire format (see StatusUpdate.writeImpl): [opcode 0x1A][objectId: 4]
// [count: 4] then count pairs [id: 4][value: 4].
type StatusUpdatePacket struct {
	ObjectID   int32
	Count      int32
	Attributes [statusUpdateMaxAttrs]StatusAttr
}

// StatusAttr is one id/value attribute pair.
type StatusAttr struct {
	ID    int32
	Value int32
}

// NewStatusUpdatePacket creates a zero valued packet ready for parsing.
func NewStatusUpdatePacket() *StatusUpdatePacket {
	var packet StatusUpdatePacket
	packet.ObjectID = 0
	packet.Count = 0

	return &packet
}

// ParseStatusUpdatePacket reads the packet from payload bytes.
func ParseStatusUpdatePacket(p *StatusUpdatePacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, statusUpdatePacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID, &p.Count); err != nil {
		return fmt.Errorf("failed to read status update header: %w", err)
	}
	if p.Count < 0 || p.Count > statusUpdateMaxPairs {
		return errors.New("invalid status update attribute count")
	}

	for i := range int(p.Count) {
		var attr StatusAttr
		if err := readInt32Fields(reader, &attr.ID, &attr.Value); err != nil {
			return fmt.Errorf("failed to read status attribute: %w", err)
		}
		if i < statusUpdateMaxAttrs {
			p.Attributes[i] = attr
		}
	}

	return nil
}

// ForEach visits the parsed attribute pairs.
func (p *StatusUpdatePacket) ForEach(visit func(id int32, value int32)) {
	count := int(p.Count)
	if count > statusUpdateMaxAttrs {
		count = statusUpdateMaxAttrs
	}
	for i := range count {
		visit(p.Attributes[i].ID, p.Attributes[i].Value)
	}
}
