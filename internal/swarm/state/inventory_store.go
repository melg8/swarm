// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"slices"
)

// inventoryStore is the inventory storage of one bot session: a dense
// item slice kept in the canonical widget order (the equipped gear
// first, then the plain inventory, both by item id with the object id
// breaking ties, see compareInventoryItems) plus the object id to
// slot index for the O(1) lookups of the packet apply paths. The
// order is an invariant maintained at mutation time: appends, removals
// and equip flag flips mark it dirty and the mutation batch restores
// it once at its end, so every reader - the snapshot fill, the JSON
// encode and the gear scans - walks the memory sequentially without
// materializing and sorting a copy first. Every method expects the
// caller to hold the bot lock (the store itself is not synchronized).
type inventoryStore struct {
	items []InventoryItem
	index map[int32]int32
	// orderDirty reports that items left the canonical order: the
	// next canonicalizeLocked call re-sorts and rebuilds the index.
	orderDirty bool
}

// newInventoryStore creates the empty store.
func newInventoryStore() inventoryStore {
	return inventoryStore{
		items:      nil,
		index:      make(map[int32]int32),
		orderDirty: false,
	}
}

// lookupLocked returns the tracked record of the object id. The caller
// must hold a lock.
func (s *inventoryStore) lookupLocked(objectID int32) (InventoryItem, bool) {
	if slot, ok := s.index[objectID]; ok {
		return s.items[slot], true
	}

	//nolint:exhaustruct // the zero value reports the miss
	return InventoryItem{}, false
}

// upsertLocked stores the item record and reports the previous one: a
// new object id appends at the end of the slice, an existing one is
// modified in place. Both the append and the equip flag flip mark the
// canonical order dirty - a pure count, enchant or type change keeps
// the record position and costs no re-sort. The caller must hold the
// write lock.
func (s *inventoryStore) upsertLocked(
	item InventoryItem,
) (previous InventoryItem, existed bool) {
	if slot, ok := s.index[item.ObjectID]; ok {
		previous = s.items[slot]
		if previous.Equipped != item.Equipped {
			s.orderDirty = true
		}
		s.items[slot] = item

		return previous, true
	}
	s.items = append(s.items, item)
	s.index[item.ObjectID] = int32(len(s.items) - 1)
	s.orderDirty = true

	//nolint:exhaustruct // the zero value reports the miss
	return InventoryItem{}, false
}

// removeLocked drops the record of the object id and reports whether
// it existed: the last record swaps into the freed slot so the array
// stays dense. The swap leaves the canonical order, so the removal
// marks it dirty for the batch end canonicalizeLocked call. The
// caller must hold the write lock.
func (s *inventoryStore) removeLocked(objectID int32) bool {
	slot, ok := s.index[objectID]
	if !ok {
		return false
	}
	last := int32(len(s.items) - 1)
	if slot != last {
		s.items[slot] = s.items[last]
		s.index[s.items[slot].ObjectID] = slot
	}
	s.items = s.items[:last]
	delete(s.index, objectID)
	s.orderDirty = true

	return true
}

// replaceLocked swaps the whole tracked inventory with the given
// records (the full ItemList packet apply). The caller must hold the
// write lock.
func (s *inventoryStore) replaceLocked(items []InventoryItem) {
	s.items = s.items[:0]
	clear(s.index)
	s.items = append(s.items, items...)
	s.orderDirty = true
	s.canonicalizeLocked()
}

// canonicalizeLocked restores the canonical widget order and rebuilds
// the slot index: a no-op unless a mutation marked the order dirty.
// The inventory stays under the slot limit in practice, so the sort
// costs a microsecond on the rare mutation batches only. The caller
// must hold the write lock.
func (s *inventoryStore) canonicalizeLocked() {
	if !s.orderDirty {
		return
	}
	slices.SortFunc(s.items, compareInventoryItems)
	clear(s.index)
	for i := range s.items {
		s.index[s.items[i].ObjectID] = int32(i)
	}
	s.orderDirty = false
}
