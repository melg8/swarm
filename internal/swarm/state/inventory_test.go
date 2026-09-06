// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSellableItemsOrdersJunkFirst verifies the shop sell selection: the
// duplicated gear pieces come first (one piece of each item id is kept),
// then the cheapest weight unit sells before the valuable ones, and
// equipped gear, adena and quest items never enter the list. The item
// ids are real C1 stats: 17 Wooden Arrow (sell 1, weight 6), 35 Short
// Bow (sell 18, weight 1320), 34 (sell 15350, weight 3960), 1060 Lesser
// Healing Potion (sell 45, weight 5), 1864 Stem (sell 50, weight 2),
// 1867 Thread (sell 75, weight 2).
func TestSellableItemsOrdersJunkFirst(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 1, ItemID: 17, Count: 500, Type2: 5, Change: 1},
		{ObjectID: 2, ItemID: 1060, Count: 30, Type2: 5, Change: 1},
		{ObjectID: 3, ItemID: 1864, Count: 10, Type2: 5, Change: 1},
		{ObjectID: 4, ItemID: 1867, Count: 10, Type2: 5, Change: 1},
		{ObjectID: 5, ItemID: 34, Count: 1, Type2: 1, Change: 1},
		{ObjectID: 6, ItemID: 34, Count: 1, Type2: 1, Change: 1},
		{ObjectID: 7, ItemID: 35, Count: 1, Type2: 1, Change: 1},
		{ObjectID: 11, ItemID: 35, Count: 1, Type2: 1, Change: 1},
		{ObjectID: 8, ItemID: 57, Count: 12345, Type2: 4, Change: 1},
		{ObjectID: 9, ItemID: 1000, Count: 1, Type2: 3, Change: 1},
		{ObjectID: 10, ItemID: 34, Count: 1, Type2: 1, Equipped: true,
			Change: 1},
	})

	items := bot.SellableItems()
	objects := make([]int32, 0, len(items))
	for _, item := range items {
		objects = append(objects, item.ObjectID)
	}
	// The duplicated gear sells first (11 and 6; the kept pieces are 7
	// and 5). Rank 1 follows by value per weight: the kept short bow 7
	// (0.014), the arrow 1 (0.17), the kept shield 5 (3.9), the potion
	// 2 (9), the stem 3 (25) and the thread 4 (37.5).
	require.Equal(t, []int32{11, 6, 7, 1, 5, 2, 3, 4}, objects)
}

// TestSellableItemsEmptyInventory verifies the empty and the equipped
// only inventories return nothing to sell.
func TestSellableItemsEmptyInventory(t *testing.T) {
	bot := NewBot("acc1")
	items := bot.SellableItems()
	require.Empty(t, items)

	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 8, ItemID: 57, Count: 100, Type2: 4, Change: 1},
		{ObjectID: 10, ItemID: 34, Count: 1, Type2: 1, Equipped: true,
			Change: 1},
	})
	require.Empty(t, bot.SellableItems())
}
