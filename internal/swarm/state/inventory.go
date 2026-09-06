// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// Inventory type2 families of the Mobius item packets.
const (
	itemType2Weapon = 0
	itemType2Armor  = 1
	itemType2Jewel  = 2
	itemType2Quest  = 3
	itemType2Adena  = 4
	itemType2Item   = 5
)

// inventorySlotLimit is the player inventory slot limit of the Mobius
// server (PlayerConfig MaximumSlotsForNoDwarf, 80 by default).
const inventorySlotLimit = 80

// InventoryItem is one entry of the inventory packets. Type2 tells the
// item family: 0 weapon, 1 armor, 2 jewel, 3 quest item, 4 adena,
// 5 common item. Change carries the InventoryUpdate code: 1 add,
// 2 modify, 3 remove.
type InventoryItem struct {
	ObjectID int32
	ItemID   int32
	Count    int32
	Type1    int16
	Type2    int16
	Equipped bool
	Change   int16
}

// InventoryStats summarizes the inventory usage of the character.
type InventoryStats struct {
	Slots         int
	MaxSlots      int
	Load          int32
	MaxLoad       int32
	WeightPercent float64
	SlotPercent   float64
	Adena         int32
}

// ApplyItemList replaces the tracked inventory with the full list from
// the ItemList packet.
func (b *Bot) ApplyItemList(items []InventoryItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	clear(b.inventory)
	for _, item := range items {
		b.inventory[item.ObjectID] = item
	}
	b.touch()
	b.recordLocked("inventory listed: " + strconv.Itoa(len(items)) + " items")
}

// ApplyInventoryUpdate applies added, modified and removed inventory
// items from the InventoryUpdate packet.
func (b *Bot) ApplyInventoryUpdate(items []InventoryItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	changed := false
	for _, item := range items {
		switch item.Change {
		case 1, 2:
			existing, ok := b.inventory[item.ObjectID]
			if !ok {
				changed = true
				b.recordLocked("received " + inventoryItemName(item))
			}
			if !ok || existing.Count != item.Count {
				changed = true
			}
			b.inventory[item.ObjectID] = item
		case 3:
			if _, ok := b.inventory[item.ObjectID]; ok {
				changed = true
				b.recordLocked("lost " + inventoryItemName(item))
			}
			delete(b.inventory, item.ObjectID)
		}
	}
	if changed {
		b.touch()
	}
}

// InventoryStats reports the inventory and weight usage of the character.
func (b *Bot) InventoryStats() InventoryStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	//nolint:exhaustruct // zero value grows inside the loop
	stats := InventoryStats{
		Slots:    len(b.inventory),
		MaxSlots: inventorySlotLimit,
		Load:     b.char.CurrentLoad,
		MaxLoad:  b.char.MaxLoad,
	}
	if stats.MaxSlots > 0 {
		stats.SlotPercent = float64(stats.Slots) / float64(stats.MaxSlots) * 100
	}
	if stats.MaxLoad > 0 {
		stats.WeightPercent = float64(stats.Load) / float64(stats.MaxLoad) * 100
	}
	for _, item := range b.inventory {
		if item.Type2 == itemType2Adena {
			stats.Adena += item.Count
		}
	}

	return stats
}

// DestroyableItems returns up to limit junk inventory items: not
// equipped, not adena, preferring non stackable equipment drops and
// quest items over common stackables.
func (b *Bot) DestroyableItems(limit int) []InventoryItem {
	b.mu.RLock()
	defer b.mu.RUnlock()
	candidates := make([]InventoryItem, 0, len(b.inventory))
	for _, item := range b.inventory {
		if item.Equipped || item.Type2 == itemType2Adena {
			continue
		}
		candidates = append(candidates, item)
	}
	sort.Slice(candidates, func(i int, j int) bool {
		return destroyRank(candidates[i]) < destroyRank(candidates[j])
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates
}

// destroyRank orders destroy candidates: gear drops and quest items
// first, common stackables last, object ids breaking ties.
func destroyRank(item InventoryItem) int {
	switch item.Type2 {
	case itemType2Quest:
		return 0
	case itemType2Weapon, itemType2Armor, itemType2Jewel:
		return 1
	default:
		return 2
	}
}

// isGearFamily reports whether the item type2 belongs to the equipment
// families that drop as single non stackable pieces.
func isGearFamily(type2 int16) bool {
	return type2 == itemType2Weapon || type2 == itemType2Armor ||
		type2 == itemType2Jewel
}

// SellableItems returns the inventory items a shop trip sells, sorted
// most junky first: duplicate gear drops (all pieces of an item id but
// one) first, then the lowest sell value per unit weight - the cheap
// heavy items the user of the inventory wants to get rid of first.
// Equipped gear, adena and quest items are never returned; the server
// silently skips items it refuses to sell, so the caller must tolerate
// entries that come back.
func (b *Bot) SellableItems() []InventoryItem {
	b.mu.RLock()
	defer b.mu.RUnlock()
	candidates := make([]InventoryItem, 0, len(b.inventory))
	gearCount := make(map[int32]int)
	gearKept := make(map[int32]int32)
	for _, item := range b.inventory {
		if item.Equipped || item.Type2 == itemType2Adena ||
			item.Type2 == itemType2Quest {
			continue
		}
		candidates = append(candidates, item)
		if isGearFamily(item.Type2) {
			gearCount[item.ItemID]++
			if kept, ok := gearKept[item.ItemID]; !ok ||
				item.ObjectID < kept {
				gearKept[item.ItemID] = item.ObjectID
			}
		}
	}
	sort.Slice(candidates, func(i int, j int) bool {
		ri, rj := sellRank(candidates[i], gearCount, gearKept),
			sellRank(candidates[j], gearCount, gearKept)
		if ri != rj {
			return ri < rj
		}
		vi, vj := sellValuePerWeight(candidates[i]),
			sellValuePerWeight(candidates[j])
		if vi != vj {
			return vi < vj
		}

		return candidates[i].ObjectID < candidates[j].ObjectID
	})

	return candidates
}

// sellRank orders the sell candidates: duplicated gear pieces first
// (every piece of an item id but the lowest object id), everything else
// after. The caller must hold the read lock or own the maps.
func sellRank(
	item InventoryItem, gearCount map[int32]int, gearKept map[int32]int32,
) int {
	if !isGearFamily(item.Type2) || gearCount[item.ItemID] <= 1 {
		return 1
	}

	return boolToInt(item.ObjectID == gearKept[item.ItemID])
}

// boolToInt maps a boolean to 0/1 without a branch.
func boolToInt(value bool) int {
	if value {
		return 1
	}

	return 0
}

// sellValuePerWeight returns the sell value of one unit of the item
// (reference price / 2, like the Mobius RequestSellItem computes it)
// divided by its unit weight. Low values are cheap heavy junk that
// frees the most weight for the least adena lost; unknown items with no
// price and no weight count as the junkiest.
func sellValuePerWeight(item InventoryItem) float64 {
	sell := npcdata.ItemPrice(item.ItemID) / 2
	weight := npcdata.ItemWeight(item.ItemID)
	switch {
	case weight > 0:
		return float64(sell) / float64(weight)
	case sell > 0:
		return math.MaxFloat64
	default:
		return 0
	}
}

// LootItem describes a ground item the bot can pick up.
type LootItem struct {
	ObjectID int32
	Name     string
	X        int32
	Y        int32
	Z        int32
}

// NearestGroundItem returns the closest ground item within the given
// distance of the character.
func (b *Bot) NearestGroundItem(maxDistance float64) (LootItem, bool) {
	return b.NearestGroundItemExcluding(maxDistance, nil, nil)
}

// NearestGroundItemExcluding returns the closest ground item within the
// given distance, skipping the object ids whose skip deadline is still
// in the future and the points outside the zone (nil zone or nil skip
// map mean no limit).
func (b *Bot) NearestGroundItemExcluding(
	maxDistance float64, skipped map[int32]time.Time, zone *Zone,
) (LootItem, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	//nolint:exhaustruct // zero value grows inside the loop
	best := LootItem{}
	bestDist := maxDistance
	found := false
	now := time.Now()
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	for _, obj := range b.objects {
		if obj.Kind != KindItem {
			continue
		}
		if until, ok := skipped[obj.ObjectID]; ok && until.After(now) {
			continue
		}
		if !zone.Contains(obj.X, obj.Y) {
			continue
		}
		dist := math.Hypot(float64(obj.X)-selfX, float64(obj.Y)-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = LootItem{
				ObjectID: obj.ObjectID,
				Name:     obj.Name,
				X:        obj.X,
				Y:        obj.Y,
				Z:        obj.Z,
			}
		}
	}

	return best, found
}

// inventoryItemName resolves the display name of an inventory item.
func inventoryItemName(item InventoryItem) string {
	name := npcdata.ItemName(item.ItemID)
	if name != "" {
		if item.Count > 1 {
			return name + " x" + strconv.Itoa(int(item.Count))
		}

		return name
	}

	return "item #" + strconv.Itoa(int(item.ItemID))
}
