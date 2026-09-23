// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// shortSwordItemID mirrors the short sword of the generated item
// stats: rhand sword, pAtk 8.
const shortSwordItemID = int32(1)

// TestAutoEquipEquipsLootedGear verifies the loop keeps the paperdoll
// up to date: a looted equippable item (or a purchase) enters the
// inventory unequipped and the next tick equips it into the empty
// slot, armed behind the shared confirmation gate.
func TestAutoEquipEquipsLootedGear(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    // The loot lands: an unequipped short sword appears.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
    })
    loop.tick()

    require.Equal(t, []int32{555}, game.uses,
        "the looted sword must be equipped on the next tick")
    require.Contains(t, loop.pendingActions, int32(555),
        "the equip must arm the shared confirmation gate")

    // The server applies it: the equipped flag flips and the gate
    // opens for the next action (none is left).
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: 555, ItemID: shortSwordItemID, Count: 1,
            Equipped: true, Change: 2,
        },
    })
    bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 555))
    loop.tick()
    require.True(t, loop.inventoryItemAllowed(555, nil),
        "the confirmed equip must release its own item")
    require.Empty(t, loop.pendingActions,
        "the confirmed pending must prune away")

    // A second identical sword brings no gain: nothing more equips.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: 556, ItemID: shortSwordItemID, Count: 1,
            Change: 1,
        },
    })
    loop.tick()
    require.Equal(t, []int32{555}, game.uses,
        "an equal sword must not toggle the weapon off")
}

// TestAutoEquipRacesOnlyOnSharedState pins the gate granularity: the
// manual inventory command and the auto equipment ride the same tick
// when their write sets are disjoint (different items, different
// paperdoll slots), while a request on an item that is already in
// flight defers - the Mobius packet executor runs the client packets
// as concurrent thread pool tasks, so only the shared state races.
func TestAutoEquipRacesOnlyOnSharedState(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    // The cloth cap (head, item 41) and the short sword (right hand)
    // in the bag: the manual command on the cap and the auto equip of
    // the sword touch disjoint slots and ride together.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
        {ObjectID: 556, ItemID: 41, Count: 1},
    })
    pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 556})
    loop.tick()
    require.Equal(t, []int32{556, 555}, game.uses,
        "the manual head equip and the auto sword equip ride one tick")

    // A manual command on the sword while its auto equip is still
    // unconfirmed would toggle the same item: it defers.
    pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 555})
    loop.tick()
    require.Equal(t, []int32{556, 555}, game.uses,
        "the in flight sword holds the manual toggle back")
    require.Len(t, loop.userDeferred, 1,
        "the racing manual command waits in the deferred queue")

    // The server applies both equips: the deferred command flushes on
    // the next tick (the user asked for the toggle, the loop delivers
    // it once the first request confirmed).
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: 555, ItemID: shortSwordItemID, Count: 1,
            Equipped: true, Change: 2,
        },
        {ObjectID: 556, ItemID: 41, Count: 1, Equipped: true, Change: 2},
    })
    bot.ApplyPaperdoll(paperdollPair(state.PaperdollRHand, 555,
        state.PaperdollHead, 556))
    loop.tick()
    require.Equal(t, []int32{556, 555, 555}, game.uses,
        "the confirmed flip releases the deferred manual toggle")
}

// TestAutoEquipDressesTheBagOnePerTick pins the serialized equip
// chain (the round-14 fleet dress fix): the Mobius PacketExecutor
// runs one shared pool with NO same-client ordering, so a same-tick
// burst races itself server-side (two jewel packets can race the
// paperdoll placement read and bounce a member). The chain sends
// ONE request per tick in plan order - four ticks dress the four
// piece outfit - and never re-requests a piece while its flip is
// in flight (the same-item re-send guard outlives the server's
// attack-window equip deferral, see TestInventoryResendOutlivesThe
// AttackDeferral).
func TestAutoEquipDressesTheBagOnePerTick(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    // The outfit of four distinct slots: the Short Sword (right
    // hand), the Wooden Breastplate (chest), the Cloth Cap (head) and
    // the Leather Shoes (feet).
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
        {ObjectID: 556, ItemID: 23, Count: 1},
        {ObjectID: 557, ItemID: 41, Count: 1},
        {ObjectID: 558, ItemID: 37, Count: 1},
    })

    // One tick, ONE request: the chain serializes itself.
    loop.tick()
    require.Len(t, game.uses, 1,
        "one equip per tick - the burst cannot race itself")

    // The piece is still unconfirmed (in flight): the next ticks
    // send the REST of the plan, one per tick, never re-requesting
    // an in-flight piece (a second request would toggle it back
    // off).
    loop.tick()
    loop.tick()
    loop.tick()
    require.Len(t, game.uses, 4,
        "four ticks dress the four piece outfit, one per tick")
    require.Len(t, uniqueInt32(game.uses), 4,
        "an in flight piece is never re-requested")

    // The server applies everything: the next tick plans nothing.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: 555, ItemID: shortSwordItemID, Count: 1,
            Equipped: true, Change: 2,
        },
        {ObjectID: 556, ItemID: 23, Count: 1, Equipped: true, Change: 2},
        {ObjectID: 557, ItemID: 41, Count: 1, Equipped: true, Change: 2},
        {ObjectID: 558, ItemID: 37, Count: 1, Equipped: true, Change: 2},
    })
    bot.ApplyPaperdoll(paperdollMulti(map[int]int32{
        state.PaperdollRHand: 555,
        state.PaperdollChest: 556,
        state.PaperdollHead:  557,
        state.PaperdollFeet:  558,
    }))
    loop.tick()
    require.Len(t, game.uses, 4,
        "the dressed character has nothing left to equip")
    require.Empty(t, loop.pendingActions,
        "the confirmed pendings pruned away")
}

// TestAutoEquipPairSwapTwoSteps verifies the two step pair swap: the
// unequip of the weaker jewel goes out first and the better jewel
// equips into the freed slot after the confirmation - the two steps
// never ride one burst (the concurrent packet tasks would race on the
// freed slot).
func TestAutoEquipPairSwapTwoSteps(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    // Right ear: apprentice's earring (mDef 11, item 112); left ear:
    // mystic's earring (mDef 13, item 113); a second mystic's earring
    // (556) in the inventory beats the right one.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 555, ItemID: 112, Count: 1, Equipped: true},
        {ObjectID: 700, ItemID: 113, Count: 1, Equipped: true},
        {ObjectID: 556, ItemID: 113, Count: 1},
    })
    bot.ApplyPaperdoll(paperdollPair(state.PaperdollREar, 555,
        state.PaperdollLEar, 700))
    loop.tick()
    require.Equal(t, []int32{555}, game.uses,
        "the weaker right earring must be freed first")

    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 555, ItemID: 112, Count: 1, Equipped: false, Change: 2},
    })
    bot.ApplyPaperdoll(paperdollPair(state.PaperdollLEar, 700,
        state.PaperdollREar, 0))
    loop.tick()
    require.Equal(t, []int32{555, 556}, game.uses,
        "the better earring equips into the freed slot")
}

// TestAutoEquipSwapRefillWaitsForTheFlip pins the burst cut on the
// live loop: while the freeing unequip of a pair swap is still in
// flight, the refill into the freed slot holds back - the flip
// releases it on the very next tick.
func TestAutoEquipSwapRefillWaitsForTheFlip(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 555, ItemID: 112, Count: 1, Equipped: true},
        {ObjectID: 700, ItemID: 113, Count: 1, Equipped: true},
        {ObjectID: 556, ItemID: 113, Count: 1},
    })
    bot.ApplyPaperdoll(paperdollPair(state.PaperdollREar, 555,
        state.PaperdollLEar, 700))
    loop.tick()
    require.Equal(t, []int32{555}, game.uses,
        "the freeing unequip rides the first tick")

    // The unequip is unconfirmed: the refill does not race it.
    loop.tick()
    loop.tick()
    require.Equal(t, []int32{555}, game.uses,
        "the refill waits while the unequip is in flight")

    // The flip lands: the refill goes out immediately.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 555, ItemID: 112, Count: 1, Equipped: false, Change: 2},
    })
    bot.ApplyPaperdoll(paperdollPair(state.PaperdollLEar, 700,
        state.PaperdollREar, 0))
    loop.tick()
    require.Equal(t, []int32{555, 556}, game.uses,
        "the confirmed unequip releases the refill")
}

// paperdollMulti builds a paperdoll block with several occupied slots.
func paperdollMulti(filled map[int]int32) [state.PaperdollSlots]int32 {
    var ids [state.PaperdollSlots]int32
    for index, objectID := range filled {
        ids[index] = objectID
    }

    return ids
}

// paperdollWith builds a paperdoll block with one occupied slot.
func paperdollWith(index int, objectID int32) [state.PaperdollSlots]int32 {
    var ids [state.PaperdollSlots]int32
    ids[index] = objectID

    return ids
}

// paperdollPair builds a paperdoll block with two occupied slots.
func paperdollPair(
    first int, firstID int32, second int, secondID int32,
) [state.PaperdollSlots]int32 {
    var ids [state.PaperdollSlots]int32
    ids[first] = firstID
    ids[second] = secondID

    return ids
}

// uniqueInt32 answers the distinct values of the slice in order.
func uniqueInt32(values []int32) []int32 {
    seen := make(map[int32]bool, len(values))
    unique := make([]int32, 0, len(values))
    for _, value := range values {
        if !seen[value] {
            seen[value] = true
            unique = append(unique, value)
        }
    }

    return unique
}

// TestInventoryResendOutlivesTheAttackDeferral pins the round-14
// fleet dress fix: the Mobius UseItem handler defers an equip that
// lands inside the attack window to the attack end (~1.5 s bow
// windup, ~3 s cycle - the measured kite timing findings), so the
// SAME ITEM stays gated for the whole deferral (the 4 s
// inventoryResendTimeout) - the old shared 600 ms expiry re-sent
// the item into the window and the second packet toggled the piece
// right back off. A DIFFERENT item's slot overlap releases at the
// old 600 ms pace (the refused-request rescue).
func TestInventoryResendOutlivesTheAttackDeferral(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
        {ObjectID: 556, ItemID: 23, Count: 1},
    })

    // The first equip goes out and stays unconfirmed (the server
    // holds it through the attack window).
    loop.tick()
    require.Len(t, game.uses, 1)
    marked := game.uses[0]

    // 700 ms past the click - the OLD expiry line, deep inside the
    // server's attack-window deferral: the same item is STILL
    // gated...
    time.Sleep(700 * time.Millisecond)
    require.False(t, loop.inventoryItemAllowed(marked, nil),
        "the same item stays gated through the attack deferral")

    // ...while the OTHER item's slot overlap released long ago (it
    // equips on the very next tick).
    loop.tick()
    require.Len(t, game.uses, 2,
        "a different item rides the 600 ms slot-overlap release")

    // The deferral outlived: past the 4 s same-item window the
    // refused request finally retries.
    time.Sleep(3400 * time.Millisecond)
    require.True(t, loop.inventoryItemAllowed(marked, nil),
        "the same-item guard lapses at the resend timeout")
}
