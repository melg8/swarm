// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

// slotScore returns the score of the virtual slot entry (0 for an
// empty slot).
func slotScore(entry ScoredItem) float64 {
    if paperdollEmpty(entry) {
        return 0
    }

    return entry.Score
}

// slotBuf is a stack-allocated slot list: the maximum number of
// affected slots is 2 (lrhand, onepiece, ears, fingers), so a fixed
// array avoids the heap allocation that []Slot caused on every call.
// The 100 bot fleet called affectedSlots 200-600 times per plan
// computation (20-40 candidates x 5 walk steps x 2-3 calls each),
// which was 3 MB of allocations over a 3 minute run.
type slotBuf struct {
    data [2]Slot
    n    int
}

func (b slotBuf) slice() []Slot { return b.data[:b.n] }

// affectedSlots lists the paperdoll slots an item of the bodypart
// writes to when it is equipped on the virtual paperdoll: its own slot
// plus the family slots the equip clears on the server (a two hand
// weapon drops the shield, a one-piece empties the legs, a shield a
// two hand weapon and legs a one-piece chest). Pair jewels report the
// single slot the equip would take, so a second purchase of the pair
// may still fill the other, empty half. Returns a stack-allocated
// slotBuf so the caller iterates without heap traffic.
func affectedSlots(virtual [slotCount]ScoredItem, bodyPart string) slotBuf {
    switch {
    case bodyPart == partLrhand:
        return slotBuf{data: [2]Slot{SlotRHand, SlotLHand}, n: 2}
    case bodyPart == partOnepiece:
        return slotBuf{data: [2]Slot{SlotChest, SlotLegs}, n: 2}
    case bodyPart == partLhand &&
        virtual[SlotRHand].Stats.BodyPart == partLrhand:
        return slotBuf{data: [2]Slot{SlotLHand, SlotRHand}, n: 2}
    case bodyPart == partLegs &&
        virtual[SlotChest].Stats.BodyPart == partOnepiece:
        return slotBuf{data: [2]Slot{SlotLegs, SlotChest}, n: 2}
    case bodyPart == partEars || bodyPart == partFingers:
        slots := SlotsForBodyPart(bodyPart)
        slot := pairSlot(virtual, slots)
        if slot == slotInvalid {
            return slotBuf{data: [2]Slot{slots[0], slots[1]}, n: 2}
        }

        return slotBuf{data: [2]Slot{slot, 0}, n: 1}
    default:
        slots := SlotsForBodyPart(bodyPart)
        if len(slots) == 0 {
            return slotBuf{data: [2]Slot{}, n: 0}
        }

        return slotBuf{data: [2]Slot{slots[0], 0}, n: 1}
    }
}
