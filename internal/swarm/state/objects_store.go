// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

// objectStore is the world object storage of one bot session, split
// out of the Bot god object: a dense slot array (no holes, the packet
// apply paths mutate the records in place and the scans walk the
// memory sequentially) plus the object id to slot index. The record
// itself is split into a hot and a cold array (see objectHot and
// objectCold): both halves are indexed by the same slot, the store
// keeps them length locked, and the scans stream the compact hot
// block while the display fields stay in the cold block behind it.
// The store owns the density invariant: removals swap the last
// records of both halves into the freed slot and move the index
// entry, so the arrays never fragment. Every method expects the
// caller to hold the bot lock (the store itself is not
// synchronized).
type objectStore struct {
	hot   []objectHot
	cold  []objectCold
	index map[int32]int32
}

// newObjectStore creates the empty store.
func newObjectStore() objectStore {
	return objectStore{
		hot:   nil,
		cold:  nil,
		index: make(map[int32]int32),
	}
}

// lookupLocked returns the pointers to the hot and cold records of
// the object id, both nil when the id is unknown. The pointers stay
// valid until the next append or removal - the packet apply paths
// finish their mutation before either happens. The caller must hold
// a lock.
func (s *objectStore) lookupLocked(
	objectID int32,
) (*objectHot, *objectCold) {
	if slot, ok := s.index[objectID]; ok {
		return &s.hot[slot], &s.cold[slot]
	}

	return nil, nil
}

// upsertLocked returns the pointers to the existing records of the
// object id or appends fresh ones for the id. A fresh record carries
// the spawn defaults of the old newWorldObject: running, unit move
// multiplier and unit item count. The caller must hold the write
// lock.
func (s *objectStore) upsertLocked(
	objectID int32, kind int8,
) (*objectHot, *objectCold) {
	if slot, ok := s.index[objectID]; ok {
		return &s.hot[slot], &s.cold[slot]
	}
	//nolint:exhaustruct_v5 // the spawn defaults, the rest starts zero
	s.hot = append(s.hot, objectHot{
		ObjectID:      objectID,
		Kind:          kind,
		Running:       true,
		MoveSpeedMult: 1,
	})
	//nolint:exhaustruct_v5 // the unit item count, the rest starts zero
	s.cold = append(s.cold, objectCold{Count: 1})
	slot := int32(len(s.hot) - 1)
	s.index[objectID] = slot

	return &s.hot[slot], &s.cold[slot]
}

// removeAtLocked frees a slot of the dense object arrays: the last
// records of both halves move into the freed slot and the index
// follows them, so the arrays stay dense and length locked. The
// caller must hold the write lock.
func (s *objectStore) removeAtLocked(slot int32, objectID int32) {
	last := int32(len(s.hot) - 1)
	if slot != last {
		s.hot[slot] = s.hot[last]
		s.cold[slot] = s.cold[last]
		s.index[s.hot[slot].ObjectID] = slot
	}
	s.hot = s.hot[:last]
	s.cold = s.cold[:last]
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
