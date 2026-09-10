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

// The looted gear of the shopping list must survive the junk flows: a
// drop the auto equipment wants to wear (the very item the shop
// strategy planned to buy) is never sold for its instant adena and
// never destroyed for bag space - the bot puts it on and uses it.

// TestLootedGearSurvivesTheSellStop runs the town trip sell stop with a
// freshly looted weapon in the bag: the shopping plan stops wanting the
// buy (the free upgrade satisfies it), the auto equipment would wear
// the sword, and the junk batches must not offer it even while the
// equip manager is paced out (a pair swap mid flight, the confirmation
// window) - the sell phase ends with the sword still in the bag.
func TestLootedGearSurvivesTheSellStop(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)
	// The looted Short Sword (item 1): the bot wears no weapon yet,
	// so the drop is the free upgrade the plan was saving up for.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 800, ItemID: 1, Count: 1, Type2: 0, Change: 1},
	})

	// The trip starts on the full inventory and walks to Herbiel.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)

	// The merchant is selected and the junk selling starts.
	settleMerchant(t, loop, game, bot, townMerchants[3], 55)
	// The equip manager is paced out: the sword waits for its use
	// request while the sell batches go out (the window the race
	// lives in).
	loop.equip.lastActionAt = time.Now()
	loop.sellAt = time.Time{}

	// Drive the junk selling to the end: every batch is confirmed,
	// the phase advances past the sells.
	batches := 0
	for range 6 {
		loop.sellAt = time.Now().Add(-sellPause - time.Second)
		loop.equip.lastActionAt = time.Now()
		loop.tick()
		if len(game.sells) == batches {
			continue
		}
		batches = len(game.sells)
		updates := make([]state.InventoryItem, 0, len(
			game.sells[batches-1]))
		for _, sold := range game.sells[batches-1] {
			require.NotEqual(t, int32(800), sold.ObjectID,
				"the looted weapon the bot wants to wear "+
					"must not be sold")
			updates = append(updates, state.InventoryItem{
				ObjectID: sold.ObjectID, ItemID: sold.ItemID,
				Count: sold.Count, Type2: sold.Type2, Change: 3,
			})
		}
		bot.ApplyInventoryUpdate(updates)
	}
	require.Positive(t, batches, "the junk must still sell")
	require.True(t, loop.buysPlanned,
		"the sell phase must complete with the sword kept")
	_, kept := bot.InventoryItemState(800)
	require.True(t, kept,
		"the looted weapon stays in the bag for the auto equipment")
}

// TestLootedGearSurvivesTheCleanupDestroy runs the inventory overflow
// cleanup with a freshly looted weapon in the bag: the destroy ranking
// prefers gear drops over common stackables, so the unequipped sword
// would be the first destroyed - the planned equip keep set must pull
// it out of the destroy candidates.
func TestLootedGearSurvivesTheCleanupDestroy(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	// 56 stackable items are 70 percent of the 80 slots: the
	// cleanup trigger, plus the looted Short Sword.
	items := make([]state.InventoryItem, 0, 57)
	for i := range 56 {
		items = append(items, state.InventoryItem{
			ObjectID: 500 + int32(i),
			ItemID:   1060,
			Count:    1,
			Type2:    5,
			Change:   1,
		})
	}
	items = append(items, state.InventoryItem{
		ObjectID: 800, ItemID: 1, Count: 1, Type2: 0, Change: 1,
	})
	bot.ApplyItemList(items)

	// The equip manager is paced out: the sword waits for its use
	// request (the pair swap window, the confirmation gate).
	loop.equip.lastActionAt = time.Now()
	loop.cleanupInventory()
	require.NotEmpty(t, game.destroys,
		"the overflowing junk must still be destroyed")
	for _, destroy := range game.destroys {
		require.NotEqual(t, int32(800), destroy[0],
			"the looted weapon the bot wants to wear must not "+
				"be destroyed for bag space")
	}
}

// TestLootedJewelSurvivesThePairSwapWindow pins the narrow race the
// keep set closes: the auto equipment swaps a looted jewel into a full
// pair in two paced steps (the weaker piece comes off, the better one
// waits for its use request), and the sell batches of the shop stop
// run right through that window. The displaced old piece sells (it is
// the junk the swap produces), the looted upgrade never does - and the
// auto equipment wears it as soon as the pacing window opens.
func TestLootedJewelSurvivesThePairSwapWindow(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)
	// One apprentice earring (112) is worn on the right ear, the
	// weaker twin already came off (the first step of the pair swap),
	// the looted mystic earring (113) waits for its paced equip.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{
			ObjectID: 100, ItemID: 112, Count: 1, Type2: 2,
			Equipped: true, Change: 1,
		},
		{ObjectID: 101, ItemID: 112, Count: 1, Type2: 2, Change: 1},
		{ObjectID: 102, ItemID: 113, Count: 1, Type2: 2, Change: 1},
	})
	bot.ApplyPaperdoll(paperdollPair(
		state.PaperdollREar, 100, state.PaperdollLEar, 0))

	// The trip starts and reaches the sell stop of Herbiel.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)
	settleMerchant(t, loop, game, bot, townMerchants[3], 55)

	// The equip manager is paced out (the second step of the pair
	// swap waits for its window): the first sell batch goes out
	// through the race window.
	loop.equip.lastActionAt = time.Now()
	loop.sellAt = time.Time{}
	loop.tick()
	require.NotEmpty(t, game.sells)
	lootedOffered, displacedOffered := false, false
	for _, sold := range game.sells[0] {
		if sold.ObjectID == 102 {
			lootedOffered = true
		}
		if sold.ObjectID == 101 {
			displacedOffered = true
		}
	}
	require.False(t, lootedOffered,
		"the looted mystic earring must survive the pair swap window")
	require.True(t, displacedOffered,
		"the displaced apprentice earring is the junk of the swap")

	// The pacing window opens: the auto equipment wears the kept
	// jewel instead of losing it to the shop.
	loop.equip.lastActionAt = time.Now().Add(-3 * time.Second)
	loop.sellAt = time.Now() // hold the sell batches for the equip.
	loop.tick()
	require.Contains(t, game.uses, int32(102),
		"the auto equipment must wear the looted jewel")
}
