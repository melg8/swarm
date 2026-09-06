// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// fakeNavigator plans straight two point paths: the start (reached
// instantly) and the requested destination. The targeted form behaves
// like the plain one unless the deck is flagged unreachable.
type fakeNavigator struct {
	fail            bool
	found           bool
	deckUnreachable bool
	calls           int
	callsAt         []time.Time
}

func (f *fakeNavigator) result(
	start, end pathfind.Vec3,
) (*pathfind.Result, error) {
	f.calls++
	f.callsAt = append(f.callsAt, time.Now())
	if f.fail {
		return nil, errors.New("no geodata")
	}
	if !f.found {
		return &pathfind.Result{
			Found:     false,
			Aborted:   false,
			Waypoints: nil,
			RawPath:   nil,
			Duration:  0,
			Explored:  0,
			OpenLeft:  0,
			Length:    0,
		}, nil
	}

	return &pathfind.Result{
		Found:     true,
		Aborted:   false,
		Waypoints: []pathfind.Vec3{start, end},
		RawPath:   []pathfind.Vec3{start, end},
		Duration:  0,
		Explored:  0,
		OpenLeft:  0,
		Length:    0,
	}, nil
}

// FindPathTo plans the deck targeted search: it reports not found when
// the fake deck is flagged unreachable, like the strict engine search
// does on the disconnected village decks.
func (f *fakeNavigator) FindPathTo(
	start, end pathfind.Vec3, _ int16,
) (*pathfind.Result, error) {
	if f.deckUnreachable {
		f.calls++
		f.callsAt = append(f.callsAt, time.Now())

		return &pathfind.Result{
			Found:     false,
			Aborted:   false,
			Waypoints: nil,
			RawPath:   nil,
			Duration:  0,
			Explored:  0,
			OpenLeft:  0,
			Length:    0,
		}, nil
	}

	return f.result(start, end)
}

// FindPath plans the plain search.
func (f *fakeNavigator) FindPath(
	start, end pathfind.Vec3,
) (*pathfind.Result, error) {
	return f.result(start, end)
}

// herbielPos is the spawn point of the Elven village trader Herbiel,
// the nearest town merchant of the test farm spot (45000, 50000).
var herbielPos = [3]int32{42766, 50037, -2984}

// legWalkTarget computes the walk target the follower sends for a leg
// from the point towards the waypoint: the waypoint itself when it is
// within the move leg limit, the intermediate straight line point
// otherwise (mirroring walkTownWaypoints).
func legWalkTarget(from [3]int32, to pathfind.Vec3) [3]int32 {
	dx := to.X - float64(from[0])
	dy := to.Y - float64(from[1])
	dist := math.Hypot(dx, dy)
	moveX, moveY, moveZ := to.X, to.Y, to.Z
	if dist > maxMoveLeg {
		frac := maxMoveLeg / dist
		moveX = float64(from[0]) + dx*frac
		moveY = float64(from[1]) + dy*frac
		moveZ = float64(from[2]) + (to.Z-float64(from[2]))*frac
	}

	return [3]int32{int32(moveX), int32(moveY), int32(moveZ)}
}

// fillInventory fills the slots of the inventory with junk items: 41
// items are 51 percent of the 80 slots and pass the trip trigger.
func fillInventory(bot *state.Bot, firstObjectID int32) {
	items := make([]state.InventoryItem, 0, 41)
	for i := range 41 {
		items = append(items, state.InventoryItem{
			ObjectID: firstObjectID + int32(i),
			ItemID:   1060,
			Count:    1,
			Type2:    5,
			Change:   1,
		})
	}
	bot.ApplyItemList(items)
}

// newTripLoop builds a hunt loop with a working navigator at the test
// farm spot.
func newTripLoop() (*Loop, *fakeGame, *state.Bot, *fakeNavigator) {
	bot := newTestBot()
	game := &fakeGame{}
	nav := &fakeNavigator{found: true}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)

	return loop, game, bot, nav
}

// moveSelfTo snaps the character to a world point (the zero distance
// move broadcast of the server).
func moveSelfTo(bot *state.Bot, x int32, y int32, z int32) {
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: x, Y: y, Z: z, DestX: x, DestY: y, DestZ: z,
	})
}

// TestTripTriggersOnFullSlots verifies the slot trigger and the first
// walk of the trip: the loop plans the path to the nearest town trader
// and starts following it.
func TestTripTriggersOnFullSlots(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, [][3]int32{legWalkTarget([3]int32{45000, 50000, -3500},
		pathfind.Vec3{X: float64(herbielPos[0]), Y: float64(herbielPos[1]), Z: float64(herbielPos[2])})}, game.walks,
		"the walk aims along the leg to the nearest town trader")

	// A second tick while the character has not moved does not resend
	// the walk immediately (rate limited) and does not re-plan.
	loop.tick()
	require.Len(t, game.walks, 1)
	require.Equal(t, 1, len(loop.waypoints)-loop.wpIndex,
		"the leg keeps its waypoint list")
}

// TestTripTriggersOnWeight verifies the weight trigger: half of the
// maximum load starts a trip even with empty slots.
func TestTripTriggersOnWeight(t *testing.T) {
	loop, game, bot, _ := newTripLoop()

	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurLoad, Value: 600},
		{ID: state.AttrMaxLoad, Value: 1000},
	})
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, [][3]int32{legWalkTarget([3]int32{45000, 50000, -3500},
		pathfind.Vec3{X: float64(herbielPos[0]), Y: float64(herbielPos[1]), Z: float64(herbielPos[2])})}, game.walks)
}

// TestTripNeedsNavigator verifies that a loop without geodata never
// leaves the hunting routine.
func TestTripNeedsNavigator(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
	require.Empty(t, game.walks)
}

// TestTripNoPathArmsCooldown verifies that a broken path search (a
// hard error, e.g. no geodata) does not retry every tick.
func TestTripNoPathArmsCooldown(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.fail = true
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase, "no trip without a path")
	require.False(t, loop.tripCooldownOver(), "the cooldown is armed")
	require.Equal(t, 1, nav.calls)
	loop.tick()
	require.Equal(t, 1, nav.calls, "no retry while the cooldown runs")

	loop.tripEndedAt = time.Now().Add(-tripCooldown - time.Second)
	loop.tick()
	require.Equal(t, 2, nav.calls, "a new trip starts after the cooldown")
}

// TestTripDirectWalkWhenNoGeodataPath verifies the last resort of the
// walk planning: when no geodata path exists (the disconnected village
// decks) the leg becomes a single direct walk the server routes
// itself.
func TestTripDirectWalkWhenNoGeodataPath(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	nav.found = false
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, [][3]int32{legWalkTarget(
		[3]int32{45000, 50000, -3500}, pathfind.Vec3{X: float64(herbielPos[0]), Y: float64(herbielPos[1]), Z: float64(herbielPos[2])})},
		game.walks, "the direct walk aims at the nearest town trader")
	require.Equal(t, 2, nav.calls,
		"the targeted and the plain search ran")
	require.Len(t, loop.waypoints, 1,
		"the fallback leg is the destination only")
}

// TestTripFallsBackWhenDeckUnreachable verifies the disconnected deck
// fallback: the targeted search reports not found (the shop deck is
// disconnected in the geodata), the plain search still plans the walk
// and the trip happens - the sale does not need the shop deck.
func TestTripFallsBackWhenDeckUnreachable(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	nav.deckUnreachable = true
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, [][3]int32{legWalkTarget(
		[3]int32{45000, 50000, -3500},
		pathfind.Vec3{X: float64(herbielPos[0]),
			Y: float64(herbielPos[1]), Z: float64(herbielPos[2])})},
		game.walks, "the fallback plans the walk to the shop")
}

// TestTripFullFlow walks the whole trip: farm to shop, the merchant
// interaction, the sale, the walk back and the hunt resuming.
func TestTripFullFlow(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot, 500)

	// The trip starts and walks to the shop.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)

	// The character arrives at the shop point: the selling phase starts.
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)

	// The merchant npc of the shop is in the known list: the loop picks
	// it and selects it like the official client before a transaction.
	// 107150 is the NpcInfo template id of Herbiel (display id 7150).
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 55, TemplateID: 7150 + 1000000,
		X: herbielPos[0], Y: herbielPos[1], Z: herbielPos[2],
		Name: "Herbiel",
	})
	loop.tick()
	require.Equal(t, int32(55), loop.merchantID, "the merchant is picked")
	loop.merchantPick = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{55}, game.forces,
		"the first request selects the merchant")
	// The selection is confirmed: the next tick sells the first batch.
	bot.ApplySelfTarget(55)
	loop.tick()
	require.Len(t, game.sells, 1)
	require.Len(t, game.sells[0], sellBatchSize)

	// A repeat sale is rate limited by the transaction flood protector.
	loop.tick()
	require.Len(t, game.sells, 1)

	// The server confirms the sale (InventoryUpdate removals): the
	// inventory drops below the trigger and the walk home starts.
	updates := make([]state.InventoryItem, 0, sellBatchSize)
	for _, item := range game.sells[0] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase)

	// The return leg walks home; the trip ends at the farm spot and
	// the hunt resumes.
	loop.tick()
	require.Equal(t, [][3]int32{
		legWalkTarget([3]int32{45000, 50000, -3500},
			pathfind.Vec3{X: float64(herbielPos[0]), Y: float64(herbielPos[1]), Z: float64(herbielPos[2])}),
		legWalkTarget(herbielPos,
			pathfind.Vec3{X: 45000, Y: 50000, Z: -3500}),
	}, game.walks, "the return leg walks home")
	moveSelfTo(bot, 45000, 50000, -3500)
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
}

// TestTripSellsRemainingJunkInBatches verifies that an inventory that
// stays full keeps selling: after the flood protector pause the next
// batch goes out, and items already offered are not offered twice.
func TestTripSellsRemainingJunkInBatches(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot, 500)
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])

	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)

	// No merchant visible: the wait must not block the sale forever.
	loop.tick()
	require.Empty(t, game.forces,
		"nothing is selected while no merchant is known")
	require.Empty(t, game.sells)

	loop.merchantID = -1
	loop.tick()
	require.Len(t, game.sells, 1, "the sale starts without a merchant")
	require.Len(t, game.sells[0], sellBatchSize)

	// The items were NOT removed (the server refused them): the next
	// batch skips the offered object ids.
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.Len(t, game.sells, 2)
	require.Len(t, game.sells[1], 41-sellBatchSize)
	for _, sold := range game.sells[0] {
		for _, next := range game.sells[1] {
			require.NotEqual(t, sold.ObjectID, next.ObjectID,
				"an offered item is never offered twice")
		}
	}

	// Everything offered and nothing sold: the trip walks home empty
	// handed instead of stalling at the shop.
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase)
}

// TestTripNoCleanupDuringTrip verifies that the destroy cleanup holds
// off while a trip runs (the items should be sold, not destroyed) and
// resumes after the trip.
func TestTripNoCleanupDuringTrip(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	items := make([]state.InventoryItem, 0, 76)
	for i := range 76 {
		items = append(items, state.InventoryItem{
			ObjectID: 500 + int32(i), ItemID: 1060, Count: 1,
			Type2: 5, Change: 1,
		})
	}
	bot.ApplyItemList(items)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Empty(t, game.destroys, "nothing is destroyed during a trip")

	// After the trip the destroy cleanup of the engage routine takes
	// over for whatever could not be sold.
	loop.endTownTrip("test end")
	loop.cleanupInventory()
	require.NotEmpty(t, game.destroys)
}

// TestTripStuckWalkRepaths verifies the stuck handling: a character
// standing still on a leg re-paths, and after the re-path budget is
// spent the trip aborts instead of walking into a wall forever.
func TestTripStuckWalkRepaths(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, 1, nav.calls)

	// The character stands still past the stuck timeout: the walker
	// re-paths from the current position.
	loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
	loop.tick()
	require.Equal(t, 2, nav.calls, "the stuck walk re-paths")
	require.Equal(t, phaseTownWalk, loop.phase)

	// Three more stuck re-paths exhaust the budget: the trip aborts.
	for range maxRePaths {
		loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
		loop.tick()
	}
	require.Equal(t, phaseEngage, loop.phase,
		"the trip aborts when the re-paths are spent")
	require.False(t, loop.tripCooldownOver())
}

// TestTripDeathResetsWithoutCooldown verifies that a death during a
// trip drops the trip state but lets a new trip start right after the
// revival: the village restart lands next to the shops.
func TestTripDeathResetsWithoutCooldown(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot, 500)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)

	// Death during the walk: the village restart request goes out and
	// the trip state is dropped.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
	require.Equal(t, 1, game.restarts)
	require.Equal(t, phaseEngage, loop.phase)
	require.True(t, loop.tripCooldownOver(),
		"the death does not arm the trip cooldown")

	// Revived in the village with a full inventory: a fresh trip starts
	// (from the village, so the farm spot is the zone center).
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 200},
	})
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase,
		"a full inventory sells right after the revival")
}
