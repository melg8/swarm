// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// shortBowItemID mirrors the short bow of the generated item stats:
// lrhand BOW pAtk 16 (see the gear test constants).
const shortBowItemID = int32(13)

// woodenArrowItemID mirrors the wooden arrow of the generated item
// stats: the lhand quiver item the ammo discriminator resolves.
const woodenArrowItemID = int32(17)

// TestArcherArmsTheQuiver pins the quiver arming of the archer: the
// bow in the right hand shoots only with an arrow stack worn in the
// left, and the ammo scores zero under every profile (a consumable,
// not gear), so the loop itself arms it right behind the auto
// equipment - one use item request, the same gate the equips ride.
func TestArcherArmsTheQuiver(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetGearProfile(gear.Archer{})

    // The bow worn, the arrows in the bag, the left hand empty.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 700, ItemID: shortBowItemID, Count: 1, Equipped: true},
        {ObjectID: 701, ItemID: woodenArrowItemID, Count: 600},
    })
    bot.ApplyPaperdoll(
        paperdollWith(state.PaperdollRHand, 700))
    loop.tick()

    require.Equal(t, []int32{701}, game.uses,
        "the quiver must arm on the tick after the bow is worn")
    require.Contains(t, loop.pendingActions, int32(701),
        "the arming must ride the shared confirmation gate")

    // The server applies it: the arrows land in the left hand and no
    // further request fires (the arming holds while the quiver is
    // worn).
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {
            ObjectID: 701, ItemID: woodenArrowItemID, Count: 600,
            Equipped: true, Change: 2,
        },
    })
    bot.ApplyPaperdoll(paperdollPair(
        state.PaperdollRHand, 700, state.PaperdollLHand, 701))
    loop.tick()
    require.Len(t, game.uses, 1,
        "a worn quiver must not re-arm")

    // The stack runs dry: the server drops the empty quiver (the
    // removal) and the next bag stack re-arms.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 701, ItemID: woodenArrowItemID, Count: 0, Change: 3},
        {ObjectID: 702, ItemID: woodenArrowItemID, Count: 300, Change: 1},
    })
    bot.ApplyPaperdoll(
        paperdollWith(state.PaperdollRHand, 700))
    loop.tick()
    require.Equal(t, []int32{701, 702}, game.uses,
        "the dry quiver must re-arm from the next bag stack")
}

// TestFighterNeverArmsTheQuiver pins the melee side: the quiver
// arming is the archer's own step - the melee profile equips its
// arrows only inside the lure arm phase, never through this path.
func TestFighterNeverArmsTheQuiver(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    // The default profile of the loop is the melee fighter.

    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 700, ItemID: shortBowItemID, Count: 1, Equipped: true},
        {ObjectID: 701, ItemID: woodenArrowItemID, Count: 600},
    })
    bot.ApplyPaperdoll(
        paperdollWith(state.PaperdollRHand, 700))
    loop.tick()

    require.Empty(t, game.uses,
        "the melee profile never arms the quiver outside a lure")
}

// TestArcherQuiverSurvivesTheJunkFlows pins the ammo keeps: the arrow
// stacks score zero under every profile, so the junk flows (the shop
// selling, the overflow destroy) would sell the archer's ammo at the
// first vendor visit without the explicit quiver keep.
func TestArcherQuiverSurvivesTheJunkFlows(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetGearProfile(gear.Archer{})

    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 700, ItemID: shortBowItemID, Count: 1},
        {ObjectID: 701, ItemID: woodenArrowItemID, Count: 600},
        {ObjectID: 702, ItemID: woodenArrowItemID, Count: 120},
    })
    keeps := loop.plannedEquipKeeps()
    require.True(t, keeps[700],
        "the owned bow must survive the junk flows")
    require.True(t, keeps[701],
        "the arrow stacks must survive the junk flows")
    require.True(t, keeps[702],
        "every arrow stack must survive, not only the biggest")
}
