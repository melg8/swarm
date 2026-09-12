// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"bytes"
	"log"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The reproduction of the 2026-09-12 04:58 state dump report (build
// 4deb888, bot test2, phase engage, uptime 1m44s): the level 14
// elven fighter returned from its town trip without the legs armor -
// the equipment held 11 pieces (Sickle, Buckler, Leather Shirt,
// Wooden Helmet, Gloves, Leather Shoes, the basic jewels), the bag
// held nothing but the 13162 adena and the trip had ended as a
// success ("town trip ended: back at the farm spot" 10 seconds
// before the dump). The session started at the village, so the trip
// resumed across a relogin: the sell first step had banked the
// legs piece for a replacement the trip never landed (the session
// death, the silent buy refusal, the abort - every exit path ends
// the trip without the replacement), the return leg walked home and
// the machinery congratulated itself. The fix: the trip snapshots
// the paperdoll it starts with and every exit arms the gear debt
// for a slot it left empty with the piece gone - the debt logs the
// loss, shortens the trip cooldown to the gear run window and the
// next trip refills the hole.

// round60Pants is the legs piece of the reproduction: the Leather
// Pants (item 29) of the elven armor ladder.
const round60PantsObjectID = int32(200)

// round60DumpAdena is the wallet of the dump: 13162 adena.
const round60DumpAdena = int64(13162)

// round60AdenaObjectID is the adena stack of the test character.
const round60AdenaObjectID = int32(999)

// applyRound60Gear dresses the test character in the exact equipment
// of the dump (the object ids 100-110, every slot filled except the
// legs) with the given adena; withPants adds the Leather Pants of
// the stranding predecessor state.
func applyRound60Gear(bot *state.Bot, adena int64, withPants bool) {
	items := []state.InventoryItem{
		{ObjectID: 100, ItemID: 20, Count: 1, Equipped: true, BodyPart: 0x100},
		{ObjectID: 101, ItemID: 22, Count: 1, Equipped: true, BodyPart: 0x400},
		{ObjectID: 102, ItemID: 37, Count: 1, Equipped: true, BodyPart: 0x1000},
		{ObjectID: 103, ItemID: 43, Count: 1, Equipped: true, BodyPart: 0x40},
		{ObjectID: 104, ItemID: 49, Count: 1, Equipped: true, BodyPart: 0x200},
		{ObjectID: 105, ItemID: 112, Count: 1, Equipped: true, BodyPart: 0x6},
		{ObjectID: 106, ItemID: 112, Count: 1, Equipped: true, BodyPart: 0x6},
		{ObjectID: 107, ItemID: 116, Count: 1, Equipped: true, BodyPart: 0x30},
		{ObjectID: 108, ItemID: 116, Count: 1, Equipped: true, BodyPart: 0x30},
		{ObjectID: 109, ItemID: 118, Count: 1, Equipped: true, BodyPart: 0x8},
		{ObjectID: 110, ItemID: 153, Count: 1, Equipped: true, BodyPart: 0x80},
	}
	if withPants {
		items = append(items, state.InventoryItem{
			ObjectID: round60PantsObjectID, ItemID: 29, Count: 1,
			Equipped: true, BodyPart: 0x800,
		})
	}
	items = append(items, state.InventoryItem{
		ObjectID: round60AdenaObjectID, ItemID: 57, Count: int32(adena),
		Type2: 4,
	})
	bot.ApplyItemList(items)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test2", Level: 14, ClassID: 18, Race: 1,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 339, CurHP: 339, MaxMP: 137, CurMP: 137,
		PaperdollObjectIDs: round60Paperdoll(withPants),
	})
}

// round60Paperdoll builds the paperdoll block of the dump state
// (the legs slot only filled when the pants are worn).
func round60Paperdoll(withPants bool) [state.PaperdollSlots]int32 {
	paperdoll := [state.PaperdollSlots]int32{
		0, 105, 106, 109, 107, 108, 103, 110, 100, 104, 101, 0, 102, 0, 0,
	}
	if withPants {
		paperdoll[state.PaperdollLegs] = round60PantsObjectID
	}

	return paperdoll
}

// TestRound60DebtArmsOnStrandedReplacement walks the full stranding
// of the dump: the trip sells the displaced Leather Pants for the
// Hard Leather Pants replacement, the buy requests the server
// silently refuses never arrive, the retry budget skips them, the
// return leg walks home and the trip ends as a success - the gear
// debt arms at the end instead of letting the bot farm on half
// dressed.
func TestRound60DebtArmsOnStrandedReplacement(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	logBuf := &bytes.Buffer{}
	loop.SetLogger(log.New(logBuf, "", 0))
	// The dump character one town trip earlier: every slot filled
	// INCLUDING the Leather Pants, the wallet sized so the legs
	// upgrade only plans through the pants' sell credit. A junk stack
	// rides along: the sell stop of a real trip always carries loot to
	// sell, and the merchant pick of that flow is what the replacement
	// sale needs in reach.
	applyRound60Gear(bot, 6000, true)
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 300, ItemID: 1864, Count: 5, Type2: 5, Change: 1},
	})
	require.True(t, loop.shoppingWanted(),
		"the legs upgrade through the sell credit plans a trip")

	// The trip starts and walks to the nearest merchant.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	var sellCredit int64
	for _, purchase := range loop.tripPlan {
		if purchase.ItemID == 30 {
			sellCredit = purchase.SellCredit
		}
	}
	require.Positive(t, sellCredit,
		"the plan prices the Leather Pants as the sell first credit")
	require.Equal(t, []int32{round60PantsObjectID},
		legsSellFirstOf(loop.tripPlan),
		"the pants are the sell first piece of the legs upgrade")

	// The sell stop: the junk sells first (the merchant pick of that
	// flow is what keeps the replacement sale in interaction range).
	arriveAtStop(t, loop, bot)
	settleMerchant(t, loop, game, bot, loop.tripStops[0].merchant, 55)
	loop.sellAt = time.Time{}
	loop.tick()
	require.NotEmpty(t, game.sells, "the junk batch sells")
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 300, ItemID: 1864, Count: 0, Change: 3},
	})
	// The replacement step plans its targets.
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.Equal(t, []int32{round60PantsObjectID}, loop.replaceQueue)
	// The unequip request goes out.
	loop.replaceUnequipAt = time.Now().Add(-equipActionPeriod - time.Second)
	loop.tick()
	require.Contains(t, game.uses, round60PantsObjectID,
		"the displaced pants come off first")
	// The unequip lands: the flag flips and the paperdoll slot clears
	// (the UserInfo broadcast of the server).
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{
			ObjectID: round60PantsObjectID, ItemID: 29, Count: 1,
			Equipped: false, BodyPart: 0x800, Change: 2,
		},
	})
	bot.ApplyPaperdoll(round60Paperdoll(false))
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	pantsOffered := false
	for _, batch := range game.sells {
		for _, item := range batch {
			if item.ObjectID == round60PantsObjectID {
				pantsOffered = true
			}
		}
	}
	require.True(t, pantsOffered, "the pants are sold before the buys")
	// The sale lands: the pants vanish, the proceeds join the wallet.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: round60PantsObjectID, ItemID: 29, Count: 0, Change: 3},
		{
			ObjectID: round60AdenaObjectID, ItemID: 57,
			Count: int32(6000 + sellCredit), Type2: 4, Change: 2,
		},
	})
	loop.replaceWaitAt = time.Now().Add(-replaceSellTimeout - time.Second)
	loop.tick()
	require.True(t, loop.replaceDone, "the sell first step settles")

	// The buy stop of the plan (Ariel, the armor merchant) takes the
	// purchases; the buy requests go out and the items never arrive
	// (the silent refusal of the dump): the retry budget skips them.
	stopBuys := 0
	for _, stop := range loop.tripStops {
		stopBuys += len(stop.buys)
	}
	require.Positive(t, stopBuys, "the stops carry the planned buys")
	ariel := loop.tripStops[0].merchant
	moveSelfTo(bot, ariel.X, ariel.Y, ariel.Z)
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase,
		"the arrival at the buy stop enters the sell phase")
	settleMerchant(t, loop, game, bot, ariel, 57)
	loop.buyAt = time.Now().Add(-buyPause - time.Second)
	loop.sellAt = time.Now().Add(-buyPause - time.Second)
	loop.tick()
	require.NotEmpty(t, game.buys, "the buy request went out")
	for i := 1; i <= stopBuyRetries; i++ {
		loop.buyConfirmAt = time.Now().Add(-buyConfirmWait - time.Second)
		loop.buyAt = time.Now().Add(-buyPause - time.Second)
		loop.tick()
		require.Len(t, game.buys, i+1, "retry %d re-requests the batch", i)
	}
	loop.buyConfirmAt = time.Now().Add(-buyConfirmWait - time.Second)
	loop.buyAt = time.Now().Add(-buyPause - time.Second)
	loop.tick()
	// The skipped batch completes the stop on the next tick (the stop
	// advance runs after the stop shopping settled): the trip walks home.
	loop.tick()
	require.NotEqual(t, phaseTownSell, loop.phase,
		"the skipped batch completes the stop")
	// The return leg arrives at the farm spot and the trip ends.
	for loop.phase == phaseTownReturn {
		moveSelfTo(bot, int32(loop.legDest.X), int32(loop.legDest.Y),
			int32(loop.legDest.Z))
		loop.tick()
	}

	// The debt of the dump: the legs slot sits empty with the pants
	// gone - armed at the trip end, logged for the report.
	require.Equal(t, int32(29), loop.gearDebt[state.PaperdollLegs],
		"the stranded legs slot arms the gear debt")
	require.Contains(t, logBuf.String(), "the trip left the legs slot empty")
	require.Contains(t, logBuf.String(), "Leather Pants")
	// The ordinary cooldown would hold a fresh trip for five minutes;
	// the debt shortens it to the gear run window.
	loop.tripEndedAt = time.Now()
	require.False(t, loop.tripCooldownOver(),
		"the gear run cooldown still holds for a moment")
	loop.tripEndedAt = time.Now().Add(-weaponRunCooldown - time.Second)
	require.True(t, loop.tripCooldownOver(),
		"the debt retries the refill on the gear run cooldown")
}

// TestRound60DebtRunsTheRefillTrip continues the stranding above: the
// debt armed, the next trip after the gear run cooldown refills the
// hole - the fresh plan prices the affordable legs filler (the
// Leather Pants) and the trip start names the gear debt.
func TestRound60DebtRunsTheRefillTrip(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	logBuf := &bytes.Buffer{}
	loop.SetLogger(log.New(logBuf, "", 0))
	// The state after the stranding of the dump: the pants gone, the
	// 13162 adena of the report, the debt armed.
	applyRound60Gear(bot, round60DumpAdena, false)
	loop.gearDebt[state.PaperdollLegs] = 29
	loop.tripEndedAt = time.Now().Add(-weaponRunCooldown - time.Second)

	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase,
		"the debt refill trip starts after the gear run cooldown")
	var legsPurchase bool
	for _, purchase := range loop.tripPlan {
		if stats, ok := npcdata.ItemGearStats(purchase.ItemID); ok &&
			stats.BodyPart == "legs" {
			legsPurchase = true
		}
	}
	require.True(t, legsPurchase,
		"the refill plan carries the legs filler")
	require.Equal(t, int32(29), legsItemIDOf(loop.tripPlan),
		"the affordable legs filler of the dump wallet is the Leather Pants")
	require.Contains(t, logBuf.String(), "(the gear debt refill)",
		"the trip start names the debt")
	require.NotEmpty(t, game.walks, "the walk to the merchant started")
}

// TestRound60DebtClearsOnRefill pins the debt lifecycle: the refill
// lands (the filler bought and worn) and the debt clears with a log
// line instead of haunting the trip cadence forever.
func TestRound60DebtClearsOnRefill(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	logBuf := &bytes.Buffer{}
	loop.SetLogger(log.New(logBuf, "", 0))
	applyRound60Gear(bot, round60DumpAdena, false)
	loop.gearDebt[state.PaperdollLegs] = 29
	require.True(t, loop.gearDebtRunWanted(),
		"the armed debt wants its refill")

	// The refill lands: the Leather Pants arrives and the auto
	// equipment wears it (the UserInfo paperdoll confirms the slot).
	applyRound60Gear(bot, round60DumpAdena-2009, true)

	require.False(t, loop.gearDebtRunWanted(),
		"the dressed slot clears the debt")
	require.Empty(t, loop.gearDebt, "no debt entry survives the refill")
	require.Contains(t, logBuf.String(),
		"the legs slot is dressed again, the gear debt clears")
}

// TestRound60ResetTownTripArmsDebt pins the interrupt exit: an
// attacker mid trip drops the whole trip through resetTownTrip - the
// pieces the sell first step already sold stay sold, so the debt
// arms even though the trip never reached its end.
func TestRound60ResetTownTripArmsDebt(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	// The trip mid sell first: the pants were unequipped and sold (the
	// paperdoll cleared, the item gone, the proceeds landed) and the
	// interrupt hits.
	applyRound60Gear(bot, 6873, true)
	loop.snapshotTripGear()
	applyRound60Gear(bot, 6873, false)
	loop.phase = phaseTownSell
	loop.tripStops = []tripStop{{merchant: townMerchants[3], sell: true}}

	loop.resetTownTrip()

	require.Equal(t, int32(29), loop.gearDebt[state.PaperdollLegs],
		"the interrupt exit arms the gear debt of the sold piece")
	require.True(t, loop.tripCooldownOver(),
		"the reset clears the trip cooldown for the retry")
}

// TestRound60FreshLoopRefillsTheDumpState pins the fresh process
// case: the pantsless bot of the dump enters a loop with no trip
// history and no debt (a process death lost the bookkeeping) - the
// ordinary trigger alone (the affordable plan of the empty legs
// slot) must still send it shopping at once.
func TestRound60FreshLoopRefillsTheDumpState(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	applyRound60Gear(bot, round60DumpAdena, false)
	require.Empty(t, loop.gearDebt)
	require.True(t, loop.shoppingWanted(),
		"the affordable legs filler of the dump wallet plans a trip")

	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase,
		"the fresh loop shops for the missing pants at once")
	require.Equal(t, int32(29), legsItemIDOf(loop.tripPlan),
		"the trip plan buys the Leather Pants back")
}

// legsSellFirstOf lists the sell first object ids of the legs
// purchases of a plan.
func legsSellFirstOf(plan []gear.Purchase) []int32 {
	var ids []int32
	for _, purchase := range plan {
		if stats, ok := npcdata.ItemGearStats(purchase.ItemID); ok &&
			stats.BodyPart == "legs" && len(purchase.SellFirst) > 0 {
			ids = append(ids, purchase.SellFirst...)
		}
	}

	return ids
}

// legsItemIDOf resolves the item id of the first legs purchase of a
// plan (0 when the plan carries none).
func legsItemIDOf(plan []gear.Purchase) int32 {
	for _, purchase := range plan {
		if stats, ok := npcdata.ItemGearStats(purchase.ItemID); ok &&
			stats.BodyPart == "legs" {
			return purchase.ItemID
		}
	}

	return 0
}
