// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"errors"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// squireSwordBagItem mirrors the Squire's Sword of the generated item
// stats (2369: rhand sword, pAtk 6 x 379) as the bag entry of the
// starter kit.
const squireSwordItemID = int32(2369)

// TestAutoDestroyReplacedStarterItems pins the forced cleanup: once
// the replacement is worn and the starter piece sulks in the bag, the
// next tick destroys it behind the shared confirmation gate and never
// repeats the request after the server confirmed the vanish.
func TestAutoDestroyReplacedStarterItems(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// The Short Sword is worn, the Squire's Sword is dead weight.
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 900, ItemID: squireSwordItemID, Count: 1},
		{ObjectID: 555, ItemID: shortSwordItemID, Count: 1, Equipped: true},
	})
	bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 555))
	loop.tick()

	require.Equal(t, [][2]int32{{900, 1}}, game.destroys,
		"the replaced starter sword must be destroyed on the next tick")
	require.Equal(t, int32(900), loop.userPendingItem,
		"the destroy must arm the shared confirmation gate")

	// The server destroys it: the item vanishes and the gate opens.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 900, ItemID: squireSwordItemID, Count: 0, Change: 3},
	})
	loop.tick()

	require.True(t, loop.inventoryGateOpen(),
		"the vanished item must confirm the destroy")
	require.Len(t, game.destroys, 1,
		"the confirmed destroy must not repeat")
}

// TestAutoDestroyWaitsForTheReplacementEquip pins the ordering: the
// replacement equips first, the starter piece is destroyed only after
// the tracker shows the replacement worn.
func TestAutoDestroyWaitsForTheReplacementEquip(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// The Short Sword sits in the bag next to the starter sword:
	// nothing is worn yet.
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 900, ItemID: squireSwordItemID, Count: 1},
		{ObjectID: 555, ItemID: shortSwordItemID, Count: 1},
	})
	loop.tick()

	require.Equal(t, []int32{555}, game.uses,
		"the replacement equips into the empty slot first")
	require.Empty(t, game.destroys,
		"the starter sword survives until its replacement is worn")

	// The equip lands: the gate opens and the pacing window rolls
	// over, the starter sword becomes dead weight.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{
			ObjectID: 555, ItemID: shortSwordItemID, Count: 1,
			Equipped: true, Change: 2,
		},
	})
	bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 555))
	loop.equip.lastActionAt = time.Now().Add(-equipActionPeriod)
	loop.tick()

	require.Equal(t, [][2]int32{{900, 1}}, game.destroys,
		"the starter sword is destroyed once the replacement is worn")
}

// TestAutoDestroyPacesAndRetriesFailedRequests pins the retry pacing:
// a failed destroy waits out the retry delay before it re-sends and
// never spams the request every pacing period.
func TestAutoDestroyPacesAndRetriesFailedRequests(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 900, ItemID: squireSwordItemID, Count: 1},
		{ObjectID: 555, ItemID: shortSwordItemID, Count: 1, Equipped: true},
	})
	bot.ApplyPaperdoll(paperdollWith(state.PaperdollRHand, 555))

	// The request fails: the retry delay arms and nothing was sent.
	game.lastError = errors.New("refused")
	loop.tick()

	require.Empty(t, game.destroys,
		"the failed request destroyed nothing")
	require.False(t, loop.equip.starterRetryAt[900].IsZero(),
		"the failure must arm the retry delay")

	// The pacing and the gate allow an action again, but the retry
	// window is not over: no re-send.
	game.lastError = nil
	loop.userPendingAt = time.Time{}
	loop.equip.lastActionAt = time.Now().Add(-equipActionPeriod)
	loop.tick()

	require.Empty(t, game.destroys,
		"the retry window suppresses the re-send")

	// The retry window rolls over: the request goes out.
	loop.equip.starterRetryAt[900] = time.Now().Add(-starterRetryDelay)
	loop.equip.lastActionAt = time.Now().Add(-equipActionPeriod)
	loop.tick()

	require.Equal(t, [][2]int32{{900, 1}}, game.destroys,
		"the retry fires after the delay")
}
