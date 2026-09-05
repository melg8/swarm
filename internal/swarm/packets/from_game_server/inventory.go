// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// Inventory packets: ItemList and InventoryUpdate.

const (
	itemListPacketID        = 0x27
	inventoryUpdatePacketID = 0x37
)

// inventoryItemsCapacity bounds the stored inventory entries; the server
// caps player inventories at 80 slots (see PlayerConfig defaults).
const inventoryItemsCapacity = 96

// InventoryChange values of an InventoryUpdate entry.
const (
	InventoryChangeAdd    = 1
	InventoryChangeModify = 2
	InventoryChangeRemove = 3
)

// InventoryItem is one entry of the inventory packets. Type2 tells the
// item family: 0 weapon, 1 armor, 2 jewel, 3 quest item, 4 adena,
// 5 common item.
type InventoryItem struct {
	ObjectID int32
	ItemID   int32
	Count    int32
	Type1    int16
	Type2    int16
	Equipped bool
	Change   int16
}

// ItemListPacket carries the whole inventory. The server sends it on
// world entry and on explicit requests.
// Wire format (see ItemList.writeImpl): [opcode 0x27][showWindow: 2]
// [count: 2][items...] with one 28 byte entry per item (see
// AbstractItemPacket.writeItem).
type ItemListPacket struct {
	ShowWindow bool
	Items      []InventoryItem
}

// NewItemListPacket creates a packet with a fresh item slice.
func NewItemListPacket() *ItemListPacket {
	return &ItemListPacket{
		ShowWindow: false,
		Items:      make([]InventoryItem, 0, inventoryItemsCapacity),
	}
}

// ParseItemListPacket reads the packet from payload bytes. The item slice
// is reused between packets: entries are overwritten from the front and
// the slice is truncated to the packet count.
func ParseItemListPacket(p *ItemListPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, itemListPacketID); err != nil {
		return err
	}
	showWindow, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read item list window flag: %w", err)
	}
	p.ShowWindow = showWindow != 0
	count, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read item list count: %w", err)
	}
	if count < 0 || count > 300 {
		return fmt.Errorf("implausible item count %d", count)
	}
	p.Items = parseItemListEntries(reader, p.Items, int(count))

	return nil
}

// parseItemListEntries reads count item entries into the reused slice.
func parseItemListEntries(
	reader *packet.Reader, dst []InventoryItem, count int,
) []InventoryItem {
	dst = dst[:0]
	for range count {
		item, err := readInventoryItem(reader)
		if err != nil {
			return dst
		}
		if len(dst) < inventoryItemsCapacity {
			dst = append(dst, item)
		}
	}

	return dst
}

// InventoryUpdatePacket carries inventory changes: added, modified and
// removed items.
// Wire format (see AbstractInventoryUpdate.writeItems): [opcode 0x37]
// [count: 2][entries...] with a 2 byte change code before every item
// entry: 1 add, 2 modify, 3 remove.
type InventoryUpdatePacket struct {
	Items []InventoryItem
}

// NewInventoryUpdatePacket creates a packet with a fresh item slice.
func NewInventoryUpdatePacket() *InventoryUpdatePacket {
	return &InventoryUpdatePacket{
		Items: make([]InventoryItem, 0, 8),
	}
}

// ParseInventoryUpdatePacket reads the packet from payload bytes. The
// item slice is reused between packets.
func ParseInventoryUpdatePacket(p *InventoryUpdatePacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, inventoryUpdatePacketID); err != nil {
		return err
	}
	count, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read inventory update count: %w", err)
	}
	if count < 0 || count > 100 {
		return fmt.Errorf("implausible update count %d", count)
	}
	p.Items = p.Items[:0]
	for range count {
		change, err := reader.ReadInt16()
		if err != nil {
			return fmt.Errorf("failed to read inventory change: %w", err)
		}
		item, err := readInventoryItem(reader)
		if err != nil {
			return fmt.Errorf("failed to read inventory item: %w", err)
		}
		item.Change = change
		if len(p.Items) < inventoryItemsCapacity {
			p.Items = append(p.Items, item)
		}
	}

	return nil
}

// readInventoryItem reads one item entry of the inventory packets.
func readInventoryItem(reader *packet.Reader) (InventoryItem, error) {
	var item InventoryItem
	var err error
	if item.Type1, err = reader.ReadInt16(); err != nil {
		//nolint:wrapcheck // the caller wraps with the entry context
		return item, err
	}
	if err := readInt32Fields(
		reader, &item.ObjectID, &item.ItemID, &item.Count); err != nil {
		//nolint:wrapcheck // the caller wraps with the entry context
		return item, err
	}
	if item.Type2, err = reader.ReadInt16(); err != nil {
		//nolint:wrapcheck // the caller wraps with the entry context
		return item, err
	}
	// Skip the custom type1 field before the equipped flag.
	if err := reader.Skip(2); err != nil {
		//nolint:wrapcheck // the caller wraps with the entry context
		return item, err
	}
	equipped, err := reader.ReadInt16()
	if err != nil {
		//nolint:wrapcheck // the caller wraps with the entry context
		return item, err
	}
	item.Equipped = equipped != 0
	// Skip the body part int and the enchant and custom type2 shorts.
	if err := reader.Skip(8); err != nil {
		//nolint:wrapcheck // the caller wraps with the entry context
		return item, err
	}

	return item, nil
}
