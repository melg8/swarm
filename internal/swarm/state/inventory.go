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

// Paperdoll slot indices in the order the UserInfo packet writes its
// paperdoll object id block (see UserInfo.writeImpl). The slots are the
// equip positions of the character; the object id 0 means the slot is
// empty. The last entry is a C1 duplicate of the right hand the server
// writes to fill the 15 slot block and carries no extra information.
const (
	// PaperdollUnderwear is the underwear slot.
	PaperdollUnderwear = iota
	// PaperdollREar is the right ear slot.
	PaperdollREar
	// PaperdollLEar is the left ear slot.
	PaperdollLEar
	// PaperdollNeck is the necklace slot.
	PaperdollNeck
	// PaperdollRFinger is the right finger slot.
	PaperdollRFinger
	// PaperdollLFinger is the left finger slot.
	PaperdollLFinger
	// PaperdollHead is the head slot.
	PaperdollHead
	// PaperdollRHand is the right hand (weapon) slot.
	PaperdollRHand
	// PaperdollLHand is the left hand (shield) slot.
	PaperdollLHand
	// PaperdollGloves is the gloves slot.
	PaperdollGloves
	// PaperdollChest is the chest slot (a one-piece armor occupies
	// it and blocks the legs slot).
	PaperdollChest
	// PaperdollLegs is the legs slot.
	PaperdollLegs
	// PaperdollFeet is the feet slot.
	PaperdollFeet
	// PaperdollBack is the back (cloak) slot.
	PaperdollBack
	// PaperdollDuplicateRHand is the C1 duplicate of the right hand
	// slot of the paperdoll block.
	PaperdollDuplicateRHand
	// PaperdollSlots is the number of paperdoll slots of the block.
	PaperdollSlots
)

// InventoryItem is one entry of the inventory packets. Type2 tells the
// item family: 0 weapon, 1 armor, 2 jewel, 3 quest item, 4 adena,
// 5 common item. BodyPart is the slot mask of the item template (0x80
// right hand, 0x400 chest and so on, see the packet layer); an
// unequipped item carries the mask of the slot it would occupy.
// Change carries the InventoryUpdate code: 1 add, 2 modify, 3 remove.
type InventoryItem struct {
	ObjectID int32
	ItemID   int32
	Count    int32
	Type1    int16
	Type2    int16
	Equipped bool
	BodyPart int32
	Enchant  int16
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
	b.inventory.replaceLocked(items)
	b.inventoryVersion++
	b.touch()
	b.recordLocked("inventory listed: " + strconv.Itoa(len(items)) + " items")
}

// ApplyPaperdoll stores the equipped object ids of the paperdoll slots
// as the UserInfo packet reported them (0 = empty slot). The server
// broadcasts UserInfo after every equip and unequip, so the mapping
// stays current while the session lives.
func (b *Bot) ApplyPaperdoll(ids [PaperdollSlots]int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paperdoll = ids
	b.inventoryVersion++
	b.touch()
}

// PaperdollSlotObjectIDs returns the equipped object ids of the paperdoll
// slots in the UserInfo slot order (0 = empty slot).
func (b *Bot) PaperdollSlotObjectIDs() [PaperdollSlots]int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.paperdoll
}

// InventoryItems returns a copy of every tracked inventory item: the
// equipped gear and the carried items together, in the canonical
// widget order of the dense store. The gear managers use it as their
// working set.
func (b *Bot) InventoryItems() []InventoryItem {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items := make([]InventoryItem, len(b.inventory.items))
	copy(items, b.inventory.items)

	return items
}

// InventoryHasItem reports whether the character carries at least
// one item of the display id: the spellbook gate of the learning
// walk reads it.
func (b *Bot) InventoryHasItem(itemID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, item := range b.inventory.items {
		if item.ItemID == itemID {
			return true
		}
	}

	return false
}

// ApplyInventoryUpdate applies added, modified and removed inventory
// items from the InventoryUpdate packet. The batch restores the
// canonical order once at its end (see inventoryStore).
func (b *Bot) ApplyInventoryUpdate(items []InventoryItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	changed := false
	for _, item := range items {
		switch item.Change {
		case 1, 2:
			previous, ok := b.inventory.upsertLocked(item)
			if !ok {
				changed = true
				b.recordLocked("received " + inventoryItemName(item))
			}
			// The full record comparison catches the equip flag
			// flips of the UseItem toggles too, not only the
			// count changes: the version keyed scans of the
			// hunt loop and the web view refresh both need
			// every real state change.
			if !ok || previous != item {
				changed = true
			}
		case 3:
			if b.inventory.removeLocked(item.ObjectID) {
				changed = true
				b.recordLocked("lost " + inventoryItemName(item))
			}
		}
	}
	if changed {
		b.inventory.canonicalizeLocked()
		b.inventoryVersion++
		b.touch()
	}
}

// InventoryVersion returns the count of inventory and paperdoll
// mutations so far: unchanged since a previous read means the
// cached equipment scans of the hunt loop stay valid.
func (b *Bot) InventoryVersion() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.inventoryVersion
}

// InventoryItemState returns the tracked state of one inventory
// item by its object id: the equipped flag, the stack count and
// whether the item exists at all. The manual command gate of the
// hunt loop watches it for the server confirmation of a queued
// use, drop or destroy.
func (b *Bot) InventoryItemState(objectID int32) (InventoryItem, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	item, ok := b.inventory.lookupLocked(objectID)

	return item, ok
}

// InventoryStats reports the inventory and weight usage of the character.
func (b *Bot) InventoryStats() InventoryStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	//nolint:exhaustruct // zero value grows inside the loop
	stats := InventoryStats{
		Slots:    len(b.inventory.items),
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
	for _, item := range b.inventory.items {
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
	return b.DestroyableItemsExcluding(nil, limit)
}

// DestroyableItemsExcluding filters the destroy candidates with a keep
// set of object ids: the hunt loop passes the planned equips of the
// auto equipment (the looted or bought upgrades waiting for their use
// item request), and a kept item is never destroyed for bag space -
// the bot wears it instead. The ranking of the rest is unchanged.
func (b *Bot) DestroyableItemsExcluding(
	keep map[int32]bool, limit int,
) []InventoryItem {
	b.mu.RLock()
	defer b.mu.RUnlock()
	candidates := make([]InventoryItem, 0, len(b.inventory.items))
	for _, item := range b.inventory.items {
		if item.Equipped || item.Type2 == itemType2Adena ||
			keep[item.ObjectID] {
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
// most junky first (see SellableItemsExcluding).
func (b *Bot) SellableItems() []InventoryItem {
	return b.SellableItemsExcluding(nil)
}

// SellableItemsExcluding returns the inventory items a shop trip
// sells, sorted most junky first: duplicate gear drops (all pieces of
// an item id but one) first, then the lowest sell value per unit
// weight - the cheap heavy items the user of the inventory wants to
// get rid of first. Equipped gear, adena and quest items are never
// returned, and neither are the object ids of the keep set: the hunt
// loop passes the planned equips of the auto equipment (the looted or
// bought upgrades waiting for their use item request), so an item the
// bot is about to wear is never sold for its instant adena. The server
// silently skips items it refuses to sell, so the caller must tolerate
// entries that come back.
func (b *Bot) SellableItemsExcluding(keep map[int32]bool) []InventoryItem {
	b.mu.RLock()
	defer b.mu.RUnlock()
	candidates := make([]InventoryItem, 0, len(b.inventory.items))
	gearCount := make(map[int32]int)
	gearKept := make(map[int32]int32)
	for _, item := range b.inventory.items {
		if item.Equipped || item.Type2 == itemType2Adena ||
			item.Type2 == itemType2Quest || keep[item.ObjectID] {
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

// GroundItemByID returns the tracked ground item with the given object
// id. The manual pickup command of the web UI resolves its click
// through it; a picked up or vanished item reports false.
func (b *Bot) GroundItemByID(objectID int32) (LootItem, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	hot, cold := b.objectLocked(objectID)
	if hot == nil || hot.Kind != kindItem {
		return LootItem{
			ObjectID: 0,
			Name:     "",
			X:        0,
			Y:        0,
			Z:        0,
		}, false
	}

	return LootItem{
		ObjectID: hot.ObjectID,
		Name:     cold.Name,
		X:        hot.X,
		Y:        hot.Y,
		Z:        hot.Z,
	}, true
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
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindItem {
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
				Name:     b.world.cold[i].Name,
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
