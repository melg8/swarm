// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

// objectStore is the world object storage of one bot session, split
// out of the Bot god object: a dense slot array (no holes, the
// packet apply paths mutate the records in place and the scans walk
// the memory sequentially) plus the object id to slot index. The
// store owns the density invariant: removals swap the last record
// into the freed slot and move its index entry, so the array never
// fragments. Every method expects the caller to hold the bot lock
// (the store itself is not synchronized).
type objectStore struct {
	objects []WorldObject
	index   map[int32]int32
}

// newObjectStore creates the empty store.
func newObjectStore() objectStore {
	return objectStore{
		objects: nil,
		index:   make(map[int32]int32),
	}
}

// lookupLocked returns a pointer to the record of the object id, nil
// when the id is unknown. The pointer stays valid until the next
// append or removal - the packet apply paths finish their mutation
// before either happens. The caller must hold a lock.
func (s *objectStore) lookupLocked(objectID int32) *WorldObject {
	if slot, ok := s.index[objectID]; ok {
		return &s.objects[slot]
	}

	return nil
}

// upsertLocked returns a pointer to the existing object record or
// appends a fresh one for the id. The caller must hold the write
// lock.
func (s *objectStore) upsertLocked(
	objectID int32, kind ObjectKind,
) *WorldObject {
	if slot, ok := s.index[objectID]; ok {
		return &s.objects[slot]
	}
	s.objects = append(s.objects, newWorldObject(objectID, kind))
	slot := int32(len(s.objects) - 1)
	s.index[objectID] = slot

	return &s.objects[slot]
}

// removeAtLocked frees a slot of the dense object array: the last
// record moves into the freed slot and the index follows it, so the
// array stays dense. The caller must hold the write lock.
func (s *objectStore) removeAtLocked(slot int32, objectID int32) {
	last := int32(len(s.objects) - 1)
	if slot != last {
		s.objects[slot] = s.objects[last]
		s.index[s.objects[slot].ObjectID] = slot
	}
	s.objects = s.objects[:last]
	delete(s.index, objectID)
}

// slotLocked resolves the slot of an object id, -1 when unknown. The
// caller must hold a lock.
func (s *objectStore) slotLocked(objectID int32) int32 {
	if slot, ok := s.index[objectID]; ok {
		return slot
	}

	return -1
}
