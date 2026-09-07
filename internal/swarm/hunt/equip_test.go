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
	require.Equal(t, int32(555), loop.userPendingItem,
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
	require.True(t, loop.inventoryGateOpen(),
		"the confirmed equip must open the gate")

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

// TestAutoEquipDefersToManualCommands pins the shared budget: while a
// manual inventory command is unconfirmed or deferred, the auto
// equipment holds its requests.
func TestAutoEquipDefersToManualCommands(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
		{ObjectID: 556, ItemID: 41, Count: 1},
	})
	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 556})
	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 555})
	loop.tick()

	require.Equal(t, []int32{556}, game.uses,
		"the manual command owns the action budget first")
	require.Len(t, loop.userDeferred, 1,
		"the second manual command defers behind the confirmation")

	// The manual chain is still in flight: the auto equipment must
	// not toggle anything meanwhile.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 556, ItemID: 41, Count: 1, Equipped: true, Change: 2},
	})
	loop.tick()
	require.Equal(t, []int32{556, 555}, game.uses,
		"the deferred manual command flushes after the confirmation")
}

// TestAutoEquipPacesRequests pins the request pacing: two upgrades
// never share one flood protector window.
func TestAutoEquipPacesRequests(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
		{ObjectID: 556, ItemID: 41, Count: 1},
	})
	loop.tick()
	require.Equal(t, []int32{555}, game.uses)

	// The first equip confirms fast, but the second request must
	// wait for the pacing period.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{
			ObjectID: 555, ItemID: shortSwordItemID, Count: 1,
			Equipped: true, Change: 2,
		},
	})
	bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 555))
	loop.tick()
	require.Equal(t, []int32{555}, game.uses,
		"the second equip waits for the pacing period")

	loop.equip.lastActionAt = time.Now().Add(-equipActionPeriod)
	loop.tick()
	require.Equal(t, []int32{555, 556}, game.uses,
		"the paced second equip runs after the period")
}

// TestAutoEquipPairSwapTwoSteps verifies the two step pair swap: the
// unequip of the weaker jewel goes out first and the better jewel
// equips into the freed slot after the confirmation.
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
	loop.equip.lastActionAt = time.Now().Add(-equipActionPeriod)
	loop.tick()
	require.Equal(t, []int32{555, 556}, game.uses,
		"the better earring equips into the freed slot")
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
