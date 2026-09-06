// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestSellItemPacketID = 0x1E

// SellListIDCustom is the sell list id the official client sends when
// selling from the inventory: the Mobius RequestSellItem treats list id
// 0 as the standard inventory sell (CUSTOM_CB_SELL_LIST) and computes
// the price itself as referencePrice/2, so the packet carries no prices.
const SellListIDCustom = 0

// SellItemEntry is one item of a sell request: the inventory object id,
// the item template id and the count to sell.
type SellItemEntry struct {
	ObjectID int32
	ItemID   int32
	Count    int32
}

// RequestSellItemPacket sells inventory items to a merchant. The server
// answers with InventoryUpdate (removals), ItemList, a StatusUpdate with
// the new load, a SystemMessage "The transaction is complete." and adds
// half of the reference price of every sold item as adena.
// Wire format (see RequestSellItem.readImpl): [opcode 0x1E]
// [listId: 4][count: 4] then count entries of
// [objectId: 4][itemId: 4][count: 4].
type RequestSellItemPacket struct {
	ListID int32
	Items  []SellItemEntry
}

// NewRequestSellItemPacket creates a sell request for the standard
// inventory sell list of the official client.
func NewRequestSellItemPacket() *RequestSellItemPacket {
	return &RequestSellItemPacket{
		ListID: SellListIDCustom,
		Items:  nil,
	}
}

// ToBytes serializes the packet.
func (p *RequestSellItemPacket) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(requestSellItemPacketID); err != nil {
		return fmt.Errorf("failed to write sell item id: %w", err)
	}
	if err := writer.WriteInt32(p.ListID); err != nil {
		return fmt.Errorf("failed to write sell list id: %w", err)
	}
	if err := writer.WriteInt32(int32(len(p.Items))); err != nil {
		return fmt.Errorf("failed to write sell item count: %w", err)
	}
	for _, item := range p.Items {
		if err := writer.WriteInt32(item.ObjectID); err != nil {
			return fmt.Errorf("failed to write sell object id: %w", err)
		}
		if err := writer.WriteInt32(item.ItemID); err != nil {
			return fmt.Errorf("failed to write sell item id: %w", err)
		}
		if err := writer.WriteInt32(item.Count); err != nil {
			return fmt.Errorf("failed to write sell count: %w", err)
		}
	}

	return nil
}
