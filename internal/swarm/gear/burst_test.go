// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The burst plan tests: the acceleration limit of the sequential use
// item chain - every independent equip rides one plan, the steps that
// would race an earlier request of the same burst on the server
// (the Mobius packet executor runs the client packets as concurrent
// thread pool tasks) wait for the next planner call.

// earringOfStrength (item 114): rear;lear mDef 16.
const earringOfStrengthID = int32(114)

// willowStaff (item 9): lrhand BLUNT pAtk 16 pAtkSpd 325.
const willowStaffID = int32(9)

// TestBurstUpgradeDressesTheWholeBagInOnePlan pins the world entry
// dress: an empty paperdoll and the outfit in the bag plan every
// piece in ONE burst - the actions are independent (one landing slot
// each, no shared writes) so the whole bag goes out in a single tick.
func TestBurstUpgradeDressesTheWholeBagInOnePlan(t *testing.T) {
    // Short Sword (rhand), Leather Shirt (chest), Cloth Cap (head),
    // Pants (legs), Necklace of Magic (neck): five independent slots.
    items := []state.InventoryItem{
        item(555, shortSwordID),
        item(556, leatherShirtID),
        item(557, clothCapID),
        item(558, pantsID),
        item(559, necklaceOfMagicID),
    }
    equipment := equipmentWith(items, nil)

    plan := BurstUpgrade(MeleeFighter{}, equipment)
    require.Len(t, plan, 5, "the whole bag plans in one burst")

    seen := make(map[int32]bool, 5)
    slots := make(map[Slot]bool, 5)
    for _, step := range plan {
        require.True(t, step.Equip, "every dress step equips")
        require.False(t, seen[step.ObjectID], "no item plans twice")
        require.False(t, slots[step.Slot], "no slot plans twice")
        seen[step.ObjectID] = true
        slots[step.Slot] = true
    }
    require.Contains(t, slots, SlotRHand)
    require.Contains(t, slots, SlotChest)
    require.Contains(t, slots, SlotHead)
    require.Contains(t, slots, SlotLegs)
    require.Contains(t, slots, SlotNeck)

    // The plan is stable: nothing left to plan afterwards.
    empty := BurstUpgrade(MeleeFighter{}, equipment)
    require.Len(t, empty, 5, "the plan repeats until the flips land")
}

// TestBurstUpgradeCutsThePairSwap pins the pair swap cut: the second
// step (the better jewel into the freed slot) never rides the same
// burst as the first (the unequip of the worse piece) - the two
// requests would race on the server and cancel each other.
func TestBurstUpgradeCutsThePairSwap(t *testing.T) {
    // Both ears worn (the apprentice on the right, the mystic on the
    // left), a better earring in the bag.
    apprentice := item(555, apprenticeEarringID)
    apprentice.Equipped = true
    mystic := item(700, mysticEarringID)
    mystic.Equipped = true
    items := []state.InventoryItem{
        apprentice,
        mystic,
        item(556, earringOfStrengthID),
    }
    equipment := equipmentWith(items, map[Slot]int32{
        SlotREar: 555,
        SlotLEar: 700,
    })

    plan := BurstUpgrade(MeleeFighter{}, equipment)
    require.Len(t, plan, 1, "only the freeing unequip rides the burst")
    require.Equal(t, int32(555), plan[0].ObjectID,
        "the weaker right earring frees first")
    require.False(t, plan[0].Equip, "the step is the unequip")
    require.Contains(t, plan[0].Slots, SlotREar)
}

// TestBurstUpgradeCutsTheOnePieceDropChain pins the one-piece cut:
// the chest refill behind the legs the one-piece drops waits for the
// next planner call - the two requests would race on the chest slot.
func TestBurstUpgradeCutsTheOnePieceDropChain(t *testing.T) {
    // Chest and legs worn separately, the one-piece armor in the bag.
    shirt := item(555, leatherShirtID)
    shirt.Equipped = true
    pants := item(556, pantsID)
    pants.Equipped = true
    items := []state.InventoryItem{
        shirt,
        pants,
        item(557, onePieceID),
    }
    equipment := equipmentWith(items, map[Slot]int32{
        SlotChest: 555,
        SlotLegs:  556,
    })

    plan := BurstUpgrade(MeleeFighter{}, equipment)
    require.Len(t, plan, 1, "only the one-piece ride the burst")
    require.Equal(t, int32(557), plan[0].ObjectID)
    require.Equal(t, []Slot{SlotChest, SlotLegs}, plan[0].Slots,
        "the one-piece write covers the legs it drops")
}

// TestBurstUpgradeCutsTheShieldDropChain pins the two hand weapon
// cut: the shield slot the two hander drops belongs to its write set,
// so no other burst step may race on it.
func TestBurstUpgradeCutsTheShieldDropChain(t *testing.T) {
    // Broadsword and small shield worn, the better two hand blunt in
    // the bag (5200 beats the 4169 of the broadsword plus the 11.2 of
    // the shield).
    sword := item(555, broadswordID)
    sword.Equipped = true
    shield := item(556, smallShieldID)
    shield.Equipped = true
    items := []state.InventoryItem{
        sword,
        shield,
        item(557, willowStaffID),
    }
    equipment := equipmentWith(items, map[Slot]int32{
        SlotRHand: 555,
        SlotLHand: 556,
    })

    plan := BurstUpgrade(MeleeFighter{}, equipment)
    require.Len(t, plan, 1, "only the two hand swap rides the burst")
    require.Equal(t, int32(557), plan[0].ObjectID)
    require.Equal(t, []Slot{SlotRHand, SlotLHand}, plan[0].Slots,
        "the two hand write covers the shield it drops")
}

// TestBurstUpgradeMixesIndependentAndDependent pins the mixed burst:
// the independent steps ride together, the dependent second step of
// the pair swap stays out and waits for its confirmation.
func TestBurstUpgradeMixesIndependentAndDependent(t *testing.T) {
    // The chest empty (an independent fill), both ears worn (the
    // apprentice and the mystic) and the better earring in the bag:
    // the chest equip and the freeing unequip ride one burst, the
    // refill into the freed ear waits.
    apprentice := item(555, apprenticeEarringID)
    apprentice.Equipped = true
    mystic := item(556, mysticEarringID)
    mystic.Equipped = true
    items := []state.InventoryItem{
        item(557, leatherShirtID),
        apprentice,
        mystic,
        item(558, earringOfStrengthID),
    }
    equipment := equipmentWith(items, map[Slot]int32{
        SlotREar: 555,
        SlotLEar: 556,
    })

    plan := BurstUpgrade(MeleeFighter{}, equipment)
    require.Len(t, plan, 2, "the independent steps ride together")
    planned := make(map[int32]PlannedEquip, 2)
    for _, step := range plan {
        planned[step.ObjectID] = step
    }
    require.Contains(t, planned, int32(557),
        "the chest fill is independent and rides")
    require.Contains(t, planned, int32(555),
        "the worse earring frees its slot")
    require.NotContains(t, planned, int32(558),
        "the refill into the freed ear waits for the confirmation")
}

// TestServerWriteSlotsRules pins the server write sets the manual
// command gate and the burst plan share.
func TestServerWriteSlotsRules(t *testing.T) {
    // The bagged two hand weapon writes both hand slots.
    equipment := equipmentWith([]state.InventoryItem{
        item(555, willowStaffID),
    }, nil)
    require.Equal(t, []Slot{SlotRHand, SlotLHand},
        ServerWriteSlots(equipment, 555))

    // The worn earring writes its own slot (the unequip toggle, the
    // worn drop or destroy).
    earring := item(556, apprenticeEarringID)
    earring.Equipped = true
    equipment = equipmentWith([]state.InventoryItem{earring},
        map[Slot]int32{SlotLEar: 556})
    require.Equal(t, []Slot{SlotLEar}, ServerWriteSlots(equipment, 556))

    // The bagged legs behind a worn one-piece write the legs slot and
    // the chest slot the one-piece drops from.
    onePiece := item(557, onePieceID)
    onePiece.Equipped = true
    equipment = equipmentWith([]state.InventoryItem{
        onePiece,
        item(558, pantsID),
    }, map[Slot]int32{SlotChest: 557})
    require.Equal(t, []Slot{SlotLegs, SlotChest},
        ServerWriteSlots(equipment, 558))

    // The bagged legs behind a normal chest write the legs slot only.
    shirt := item(557, leatherShirtID)
    shirt.Equipped = true
    equipment = equipmentWith([]state.InventoryItem{
        shirt,
        item(558, pantsID),
    }, map[Slot]int32{SlotChest: 557})
    require.Equal(t, []Slot{SlotLegs}, ServerWriteSlots(equipment, 558))

    // Items without gear stats write nothing.
    equipment = equipmentWith([]state.InventoryItem{item(559, 57)}, nil)
    require.Empty(t, ServerWriteSlots(equipment, 559),
        "the non gear item touches no paperdoll slot")
}
