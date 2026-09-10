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
		{
			ObjectID: 10, ItemID: 34, Count: 1, Type2: 1, Equipped: true,
			Change: 1,
		},
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
		{
			ObjectID: 10, ItemID: 34, Count: 1, Type2: 1, Equipped: true,
			Change: 1,
		},
	})
	require.Empty(t, bot.SellableItems())
}

// TestSnapshotInventoryWidget verifies the equipment widget view of the
// snapshot: every inventory item carries its slot mask, enchant level,
// resolved display name and icon file name, the equipped gear sorts
// before the plain items and the adena counter matches.
func TestSnapshotInventoryWidget(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		// Short Sword, equipped on the right hand.
		{
			ObjectID: 1, ItemID: 1, Count: 1, Type2: 0, Equipped: true,
			BodyPart: 0x80, Change: 1,
		},
		// Leather Shirt on the chest.
		{
			ObjectID: 2, ItemID: 1146, Count: 1, Type2: 1, Equipped: true,
			BodyPart: 0x400, Change: 1,
		},
		// Adena.
		{ObjectID: 3, ItemID: 57, Count: 4242, Type2: 4, Change: 1},
		// Forest Bow in the bag, two handed template mask.
		{
			ObjectID: 4, ItemID: 166, Count: 1, Type2: 0,
			BodyPart: 0x4000, Change: 1,
		},
	})

	snap := bot.Snapshot()
	require.NotNil(t, snap.Inventory)
	require.Len(t, snap.Inventory, 4)
	// Equipped gear first, then the bag, ordered by item id.
	require.Equal(t, int32(1), snap.Inventory[0].ItemID)
	require.True(t, snap.Inventory[0].Equipped)
	require.Equal(t, int32(0x80), snap.Inventory[0].BodyPart)
	require.Equal(t, "Short Sword", snap.Inventory[0].Name)
	require.Equal(t, "weapon_small_sword_i00", snap.Inventory[0].Icon)
	require.Equal(t, int32(1146), snap.Inventory[1].ItemID)
	require.Equal(t, int32(0x400), snap.Inventory[1].BodyPart)
	require.NotEmpty(t, snap.Inventory[1].Icon)
	require.Equal(t, int32(57), snap.Inventory[2].ItemID)
	require.Equal(t, int32(4242), snap.Inventory[2].Count)
	require.Equal(t, "etc_adena_i00", snap.Inventory[2].Icon)
	require.Equal(t, int32(4242), snap.Character.Adena)
	require.Equal(t, int32(166), snap.Inventory[3].ItemID)
	require.False(t, snap.Inventory[3].Equipped)
	require.Equal(t, int32(0x4000), snap.Inventory[3].BodyPart)
}

// TestSnapshotInventoryEnchant verifies the enchant level pass-through
// of the widget view.
func TestSnapshotInventoryEnchant(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		{
			ObjectID: 1, ItemID: 10, Count: 1, Type2: 0, Equipped: true,
			BodyPart: 0x80, Enchant: 3, Change: 1,
		},
	})

	snap := bot.Snapshot()
	require.Len(t, snap.Inventory, 1)
	require.Equal(t, int16(3), snap.Inventory[0].Enchant)
}

// TestSellableItemsExcludingKeepsPlannedEquips pins the keep set of
// the shop sell selection: the object ids the hunt loop passes (the
// planned equips of the auto equipment - the looted or bought upgrades
// waiting for their use item request) never enter the sell list, the
// rest of the junk keeps its order. A kept duplicate also stops
// counting toward the duplicate rank of its item id: the surviving
// piece is the kept one, the surplus stays the sellable duplicate.
func TestSellableItemsExcludingKeepsPlannedEquips(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 1, ItemID: 17, Count: 500, Type2: 5, Change: 1},
		{ObjectID: 5, ItemID: 35, Count: 1, Type2: 1, Change: 1},
		{ObjectID: 6, ItemID: 35, Count: 1, Type2: 1, Change: 1},
	})

	// Without the keep set both short bows sell (the duplicate first).
	items := bot.SellableItems()
	require.Len(t, items, 3)

	// The looted upgrade 5 is a planned equip: only the surplus 6 and
	// the arrows remain, and 6 loses its duplicate rank (its item id
	// has no other candidate left). The bow is the cheaper value per
	// weight unit (0.014 against 0.17), so it sells before the arrows.
	items = bot.SellableItemsExcluding(map[int32]bool{5: true})
	objects := make([]int32, 0, len(items))
	for _, item := range items {
		objects = append(objects, item.ObjectID)
	}
	require.Equal(t, []int32{6, 1}, objects,
		"the planned equip never sells, the duplicate order adapts")
}

// TestDestroyableItemsExcludingKeepsPlannedEquips pins the keep set of
// the overflow destroy: a planned equip never enters the destroy
// candidates even though the gear ranking prefers it, the stackable
// junk behind it takes the destroy batch instead.
func TestDestroyableItemsExcludingKeepsPlannedEquips(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 1, ItemID: 1864, Count: 10, Type2: 5, Change: 1},
		{ObjectID: 2, ItemID: 1864, Count: 10, Type2: 5, Change: 1},
		{ObjectID: 3, ItemID: 34, Count: 1, Type2: 1, Change: 1},
	})

	// The plain ranking destroys the gear drop first.
	require.Equal(t, int32(3), bot.DestroyableItems(1)[0].ObjectID)

	// The gear drop is a planned equip: the destroy batch falls through
	// to the stackables behind it.
	destroyable := bot.DestroyableItemsExcluding(map[int32]bool{3: true}, 2)
	require.Len(t, destroyable, 2)
	require.Equal(t, int32(1), destroyable[0].ObjectID)
	require.Equal(t, int32(2), destroyable[1].ObjectID)
}

// TestSellableItemsKeepTheDemandedSpellbooks pins the book keep of the
// shop sell selection: a spellbook an unlocked queued lesson demands -
// bought at the learning trip or looted - never enters the junk, a
// book of a lesson above the character level (not learnable yet) and
// an unknown tome sell as before.
func TestSellableItemsKeepTheDemandedSpellbooks(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	// Level 5 elven fighter, the level 5 lessons queued: the queue
	// build needs the learned set and the skill tree of class 18.
	bot.SetSkills([]LearnedSkill{
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	bot.ApplyUserInfo(UserInfo{Name: "test1", Level: 5, ClassID: 18})
	bot.ApplyItemList([]InventoryItem{
		// The Attack Aura spellbook of the level 15 lesson: above the
		// level window, sellable junk.
		{ObjectID: 1, ItemID: 1095, Count: 1, Type2: 5, Change: 1},
		// Stems: plain junk.
		{ObjectID: 2, ItemID: 1864, Count: 10, Type2: 5, Change: 1},
	})
	// The queue view builds the stored learning queue.
	require.NotNil(t, bot.SkillPlan())

	items := bot.SellableItems()
	objects := make([]int32, 0, len(items))
	for _, item := range items {
		objects = append(objects, item.ObjectID)
	}
	require.Equal(t, []int32{1, 2}, objects,
		"level 5: no book of the queue is demanded, both items sell")
}

// TestSellableItemsKeepTheUnlockedSpellbooks is the counterpart at
// level 15: the Attack Aura and Defence Aura lessons are unlocked and
// their spellbooks (bought at the book stop or looted) leave the junk
// list whatever the sell ranking would do with them.
func TestSellableItemsKeepTheUnlockedSpellbooks(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	// Level 15 with the strikes and masteries learned: the queue
	// head is the Attack Aura lesson (spellbook 1095) and the Defence
	// Aura lesson (spellbook 1294) - the same setup the learning
	// trip tests use.
	bot.SetSkills([]LearnedSkill{
		{SkillID: 3, Level: 9, Passive: false},
		{SkillID: 16, Level: 9, Passive: false},
		{SkillID: 56, Level: 9, Passive: false},
		{SkillID: 141, Level: 3, Passive: true},
		{SkillID: 142, Level: 5, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	bot.ApplyUserInfo(UserInfo{Name: "test1", Level: 15, ClassID: 18})
	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 1, ItemID: 1095, Count: 1, Type2: 5, Change: 1},
		{ObjectID: 2, ItemID: 1294, Count: 1, Type2: 5, Change: 1},
		{ObjectID: 3, ItemID: 1864, Count: 10, Type2: 5, Change: 1},
	})
	require.NotNil(t, bot.SkillPlan())

	items := bot.SellableItems()
	require.Len(t, items, 1, "only the stems sell, the spellbooks stay")
	require.Equal(t, int32(3), items[0].ObjectID)

	// The overflow destroy keeps them too: the destroy batch falls
	// through to the stackable behind the books.
	destroyable := bot.DestroyableItems(3)
	require.Len(t, destroyable, 1)
	require.Equal(t, int32(3), destroyable[0].ObjectID)
}
