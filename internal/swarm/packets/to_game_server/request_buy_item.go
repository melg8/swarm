// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestBuyItemPacketID = 0x1F

// BuyItemEntry is one item of a buy request; the server resolves the object
// ids itself and prices every entry from the buylist product price (the item
// reference price when the list carries none) with the town tax.
type BuyItemEntry struct {
    ItemID int32
    Count  int32
}

// RequestBuyItemPacket buys items from a shop buylist: the server requires the
// merchant of the list as the selected target within the 250 unit interaction
// distance, answers with InventoryUpdate, an ItemList, a StatusUpdate and the
// spent adena message, and shares the transaction flood protector with selling,
// so both pace alike (docs/protocol_description.md). Wire format: [opcode
// 0x1F][listId: 4][count: 4] then count entries of [itemId: 4][count: 4]; more
// than 10000 entries are refused (PlayerConfig.MAX_ITEM_IN_PACKET).
type RequestBuyItemPacket struct {
    ListID int32
    Items  []BuyItemEntry
}

// NewRequestBuyItemPacket creates a buy request for the buylist.
func NewRequestBuyItemPacket() *RequestBuyItemPacket {
    return &RequestBuyItemPacket{
        ListID: 0,
        Items:  nil,
    }
}

// ToBytes serializes the packet.
func (p *RequestBuyItemPacket) ToBytes(writer *packet.Writer) error {
    if err := writer.WriteInt8(requestBuyItemPacketID); err != nil {
        return fmt.Errorf("failed to write buy item id: %w", err)
    }
    if err := writer.WriteInt32(p.ListID); err != nil {
        return fmt.Errorf("failed to write buy list id: %w", err)
    }
    if err := writer.WriteInt32(int32(len(p.Items))); err != nil {
        return fmt.Errorf("failed to write buy item count: %w", err)
    }
    for _, item := range p.Items {
        if err := writer.WriteInt32(item.ItemID); err != nil {
            return fmt.Errorf("failed to write buy item id: %w", err)
        }
        if err := writer.WriteInt32(item.Count); err != nil {
            return fmt.Errorf("failed to write buy item stack: %w", err)
        }
    }

    return nil
}
