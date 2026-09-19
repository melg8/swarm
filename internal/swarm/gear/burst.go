// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
        "github.com/melg8/swarm/internal/swarm/npcdata"
        "github.com/melg8/swarm/internal/swarm/state"
)

// The burst plan of the auto equipment: the acceleration limit of the
// sequential use item chain on this server build.
//
// The two server facts that bound it (both verified against the
// deployed Mobius C1 source):
//
//   - The UseItem flood protector is disabled
//     (FloodProtectorUseItemInterval = 0, the retail matching config),
//     so no rate limit sits between the use item requests.
//   - The packet executor is one shared ThreadPoolExecutor
//     (commons/network/packet/PacketExecutor.java): every received
//     packet becomes its own pool task, so two packets of the SAME
//     client may run concurrently and in either order - the server
//     gives no same client ordering guarantee.
//
// The second fact is what keeps the chain from being a single blind
// burst: requests that race on one paperdoll slot (the two steps of a
// pair swap, the follow up equip of a slot a previous request still
// has to free) can interleave their reads and writes on the server
// and cancel each other. Requests on independent slots never touch
// the same server state and are safe to send together.
//
// So the limit is: every equip action whose server write set is
// disjoint from the others goes out in one tick - the whole starting
// bag dresses in a single burst - while the dependent steps (the
// second step of a pair swap, the chest refill behind a one-piece
// drop, the shield slot behind a two hand weapon) wait for the
// confirmation of their prerequisite and ride the next tick.

// PlannedEquip is one use item action of the burst plan with the
// paperdoll slots the server writes when the request applies: the
// equip write itself plus the slots of the pieces the server
// displaces with it (the shield behind a two hand weapon, the legs
// behind a one-piece armor, the replaced occupant of the slot).
type PlannedEquip struct {
        // EquipAction is the use item request (the object id, the
        // equip or free direction, the improved slot, the score gain
        // and the log reason).
        EquipAction
        // Slots are the paperdoll slots the server write set of the
        // request touches.
        Slots []Slot
}

// BurstUpgrade plans every use item action the paperdoll still needs
// and that can go out in one burst: NextUpgrade applied repeatedly to
// a simulated copy of the equipment, each offered action mirrored
// with the server write semantics, the chains cut at the steps that
// would race an earlier request of the same burst (their slot is
// already written by a collected action) and at the items that are
// already in flight. The result is the maximum independent prefix of
// the equip chain - nothing in it can interfere on the server,
// everything else waits for the confirmations of this burst.
func BurstUpgrade(profile Profile, equipment Equipment) []PlannedEquip {
        sim := cloneEquipment(equipment)
        locked := make(map[Slot]bool, slotCount)
        flight := make(map[int32]bool, len(equipment.Items))
        planned := make([]PlannedEquip, 0, slotCount)
        // Every iteration either collects one action (each object id is
        // collected at most twice - once equipping, once freeing) or
        // permanently removes one candidate item from the simulation, so
        // the bound below always terminates the loop.
        bound := 2*len(equipment.Items) + int(slotCount) + 1
        for range bound {
                action, ok := NextUpgrade(profile, sim)
                if !ok {
                        break
                }
                if flight[action.ObjectID] {
                        // The item already rides this burst: a second request
                        // would toggle the first one's effect back. Drop the
                        // candidate from the simulation so the planner moves on.
                        removeSimItem(&sim, action.ObjectID)

                        continue
                }
                before := cloneEquipment(sim)
                written := applyExpectedAction(&sim, action)
                conflict := false
                for _, slot := range written {
                        if locked[slot] {
                                conflict = true

                                break
                        }
                }
                if conflict {
                        // A collected action of this burst already writes one of
                        // the slots this step needs: the pair swap refill, the
                        // one-piece drop follow up. The step waits for the real
                        // confirmation on the next tick.
                        sim = before
                        flight[action.ObjectID] = true
                        removeSimItem(&sim, action.ObjectID)

                        continue
                }
                flight[action.ObjectID] = true
                for _, slot := range written {
                        locked[slot] = true
                }
                planned = append(planned, PlannedEquip{
                        EquipAction: action,
                        Slots:       written,
                })
        }

        return planned
}

// ServerWriteSlots reports the paperdoll slots the server writes when
// a use item, drop or destroy request of the inventory item applies:
// the equip landing plus the displaced pieces for a bagged equippable,
// the slot itself for a piece that comes off (an unequip toggle, a
// worn drop or destroy) and nothing for items without gear stats.
// The manual inventory commands of the hunt loop consult it for their
// confirmation gate.
func ServerWriteSlots(equipment Equipment, objectID int32) []Slot {
        item, ok := equipment.itemByID(objectID)
        if !ok {
                return nil
        }
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if !ok {
                return nil
        }
        slots := SlotsForBodyPart(stats.BodyPart)
        if len(slots) == 0 {
                return nil
        }
        for slot, occupant := range equipment.Slots {
                if occupant == objectID {
                        return []Slot{Slot(slot)}
                }
        }

        return equipWriteSlots(equipment, stats, slots[0])
}

// applyExpectedAction mirrors the server effect of one use item
// request on the working set copy: the equipped flags and the slot
// table land in the state the tracker will show once the flip arrives
// (equipWriteSlots owns the write rules). Returns the written slots.
func applyExpectedAction(equipment *Equipment, action EquipAction) []Slot {
        for i := range equipment.Items {
                if equipment.Items[i].ObjectID != action.ObjectID {
                        continue
                }
                item := equipment.Items[i]
                stats, ok := npcdata.ItemGearStats(item.ItemID)
                if !ok {
                        return nil
                }
                slots := SlotsForBodyPart(stats.BodyPart)
                if len(slots) == 0 {
                        return nil
                }
                written := make([]Slot, 0, 2)
                if !action.Equip {
                        // The freeing unequip of a pair swap: the piece comes off.
                        equipment.Items[i].Equipped = false
                        written = append(written, action.Slot)
                        equipment.Slots[action.Slot] = 0

                        return written
                }
                equipment.Items[i].Equipped = true
                written = append(written, equipWriteSlots(*equipment, stats,
                        action.Slot)...)
                setWrittenSlots(equipment, item.ObjectID, written)

                return written
        }

        return nil
}

// equipWriteSlots computes the slots the server write set of an equip
// request touches: the landing slot plus the displaced pieces the
// server removes with it (the shield behind a two hand weapon, the
// legs behind a one-piece, the one-piece behind legs, the two hand
// weapon behind a shield). The rules mirror the virtual paperdoll the
// purchase planner simulates (applyToVirtual).
func equipWriteSlots(
        equipment Equipment, stats npcdata.GearStats, slot Slot,
) []Slot {
        written := []Slot{slot}
        switch stats.BodyPart {
        case partLrhand:
                written = append(written, SlotLHand)
        case partOnepiece:
                written = append(written, SlotLegs)
        case partLegs:
                if occupant := equipment.Slots[SlotChest]; occupant != 0 {
                        if item, ok := equipment.itemByID(occupant); ok {
                                if occupantStats, ok := npcdata.ItemGearStats(
                                        item.ItemID); ok && occupantStats.BodyPart ==
                                        partOnepiece {
                                        written = append(written, SlotChest)
                                }
                        }
                }
        case partLhand:
                if occupant := equipment.Slots[SlotRHand]; occupant != 0 {
                        if item, ok := equipment.itemByID(occupant); ok {
                                if occupantStats, ok := npcdata.ItemGearStats(
                                        item.ItemID); ok && occupantStats.BodyPart ==
                                        partLrhand {
                                        written = append(written, SlotRHand)
                                }
                        }
                }
        }

        return written
}

// setWrittenSlots lands the equip write on the working set: the item
// takes the landing slot, every displaced occupant drops to the bag.
func setWrittenSlots(equipment *Equipment, objectID int32, written []Slot) {
        for _, slot := range written {
                occupant := equipment.Slots[slot]
                if occupant == 0 || occupant == objectID {
                        continue
                }
                for i := range equipment.Items {
                        if equipment.Items[i].ObjectID == occupant {
                                equipment.Items[i].Equipped = false

                                break
                        }
                }
        }
        equipment.Slots[written[0]] = objectID
}

// cloneEquipment copies the working set deep enough for the burst
// simulation: the item slice gets its own backing array, the slot
// table is a value array already.
func cloneEquipment(equipment Equipment) Equipment {
        items := make([]state.InventoryItem, len(equipment.Items))
        copy(items, equipment.Items)

        return Equipment{Items: items, Slots: equipment.Slots}
}

// removeSimItem drops one inventory item from the working set copy so
// the planner stops offering it for the rest of the burst.
func removeSimItem(equipment *Equipment, objectID int32) {
        for i := range equipment.Items {
                if equipment.Items[i].ObjectID == objectID {
                        equipment.Items = append(equipment.Items[:i],
                                equipment.Items[i+1:]...)

                        return
                }
        }
}
