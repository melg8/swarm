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
// instantly) and the requested destination.
type fakeNavigator struct {
	fail    bool
	found   bool
	calls   int
	callsAt []time.Time
	// height is the ClosestHeight answer for the zone return goal
	// (zero: the lookup fails and the self height stays).
	height    int16
	heightErr bool
	// blind marks the LineOfSight answer as blocked for every
	// queried line (the blind engage recovery asks it per candidate
	// standing point): the default answers a clear line - the
	// follower leg gate of the town walks needs one.
	blind bool
	// sightFunc answers the LineOfSight queries per line: the
	// waypoint follower gate tests pin the exact blocked corner
	// (nil: every line answers the blind flag).
	sightFunc func(from, to pathfind.Vec3) (bool, error)
	// overWater is the OverWater answer for the character position:
	// the water escape tests arm it to drop the character into a
	// lake.
	overWater bool
	// wetLine marks the WaterCrossed answer as wet: the click guard
	// tests arm it to refuse the clicks, every other walk stays dry
	// by default (the zero value answers a dry line).
	wetLine bool
	// refuseClicks makes the server click validation port refuse
	// every line: the follower reaction tests (the shorten, the hop
	// and the re-path) arm it.
	refuseClicks bool
	// validateHook overrides the ValidateClick answers line by
	// line (the shortening and hop targeting tests answer by
	// geometry).
	validateHook func(from, to pathfind.Vec3) (pathfind.Vec3, bool)
	// validatedClicks records the lines the follower asked the server
	// validation port about (the shorten loop and the hop targeting
	// checks pin their geometry here).
	validatedClicks []pathfind.Vec3
	// escapeRoute overrides the waypoints of the water escape search.
	escapeRoute []pathfind.Vec3
	// avoidRoute overrides the waypoints of the avoiding approach
	// search (the frozen leg recovery re-plan); avoiding records
	// the avoid areas the loop passed.
	avoidRoute []pathfind.Vec3
	avoiding   [][]pathfind.AvoidArea
	// escapeErr makes the water escape search fail hard.
	escapeErr   bool
	escapeCalls int
	// route overrides the planned waypoints of a successful search
	// (the blind reposition tests pin the leg following on a detour).
	route []pathfind.Vec3
	// approachEnds records the destinations the approach searches
	// received (the zone return goal checks live here).
	approachEnds []pathfind.Vec3
	// dryMiss makes the dry approach searches answer not found: the
	// walk would need a swim (the water loop regression tests).
	dryMiss bool
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
	if f.route != nil {
		return &pathfind.Result{
			Found:     true,
			Aborted:   false,
			Waypoints: f.route,
			RawPath:   f.route,
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

// FindPathApproach plans the approach radius search.
func (f *fakeNavigator) FindPathApproach(
	start, end pathfind.Vec3, _ float64,
) (*pathfind.Result, error) {
	f.approachEnds = append(f.approachEnds, end)

	return f.result(start, end)
}

// FindPathApproachDry plans the water walled approach search: it
// shares the routes of the ordinary search unless dryMiss is armed -
// the swim only destination answers not found.
func (f *fakeNavigator) FindPathApproachDry(
	start, end pathfind.Vec3, _ float64,
) (*pathfind.Result, error) {
	f.approachEnds = append(f.approachEnds, end)
	if f.dryMiss {
		f.calls++

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

// FindPathApproachDryAvoiding plans the water walled approach search
// around the avoid areas: the frozen leg recovery tests record the ban
// the loop passed and answer the configured avoiding route (nil: the
// plain result - the ban made no difference to the fake planner).
func (f *fakeNavigator) FindPathApproachDryAvoiding(
	start, end pathfind.Vec3, _ float64, avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
	f.avoiding = append(f.avoiding, avoid)
	if f.avoidRoute != nil {
		f.calls++
		f.callsAt = append(f.callsAt, time.Now())

		return &pathfind.Result{
			Found:     true,
			Aborted:   false,
			Waypoints: f.avoidRoute,
			RawPath:   f.avoidRoute,
			Duration:  0,
			Explored:  0,
			OpenLeft:  0,
			Length:    0,
		}, nil
	}

	return f.FindPathApproachDry(start, end, 0)
}

// FindPath plans the plain search.
func (f *fakeNavigator) FindPath(
	start, end pathfind.Vec3,
) (*pathfind.Result, error) {
	return f.result(start, end)
}

// ClosestHeight answers the configured zone deck height.
func (f *fakeNavigator) ClosestHeight(_, _ float64, _ int16) (int16, error) {
	if f.heightErr {
		return 0, errors.New("no geodata")
	}

	return f.height, nil
}

// LineOfSight answers the configured sight lines: the blind engage
// recovery tests decide which standing points see the target.
func (f *fakeNavigator) LineOfSight(
	from, to pathfind.Vec3,
) (bool, error) {
	if f.heightErr {
		return false, errors.New("no geodata")
	}
	if f.sightFunc != nil {
		return f.sightFunc(from, to)
	}

	return !f.blind, nil
}

// OverWater answers the configured water surface: the tests that walk
// a character into a lake arm it, everything else stays ashore.
func (f *fakeNavigator) OverWater(_, _ float64, _ int16) bool {
	return f.overWater
}

// WaterCrossed answers the configured water raster: the water guard
// tests arm the wet flag to make the follower refuse the clicks.
func (f *fakeNavigator) WaterCrossed(_, _ pathfind.Vec3) (bool, error) {
	if f.heightErr {
		return false, errors.New("no geodata")
	}

	return f.wetLine, nil
}

// ValidateClick answers the configured server click validation: the
// default accepts every line (the port is a pure mirror, the fake
// trusts the plan), the validateHook overrides the answers for the
// follower reaction tests and refuseClicks arms the blanket refusal.
func (f *fakeNavigator) ValidateClick(
	from, to pathfind.Vec3,
) (pathfind.Vec3, bool) {
	f.validatedClicks = append(f.validatedClicks, to)
	if f.validateHook != nil {
		return f.validateHook(from, to)
	}
	if f.refuseClicks {
		return from, false
	}

	return to, true
}

// FindWaterEscape answers the configured shore escape.
func (f *fakeNavigator) FindWaterEscape(
	_ pathfind.Vec3,
) (*pathfind.Result, error) {
	f.escapeCalls++
	if f.escapeErr {
		return nil, errors.New("no geodata")
	}
	if f.escapeRoute != nil {
		return &pathfind.Result{
			Found:     true,
			Aborted:   false,
			Waypoints: f.escapeRoute,
			RawPath:   f.escapeRoute,
			Duration:  0,
			Explored:  0,
			OpenLeft:  0,
			Length:    0,
		}, nil
	}

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
func fillInventory(bot *state.Bot) {
	items := make([]state.InventoryItem, 0, 41)
	for i := range 41 {
		items = append(items, state.InventoryItem{
			ObjectID: 500 + int32(i),
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
	fillInventory(bot)

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

// TestTripWalkPlanPublishes pins the walk plan view of a town trip:
// while the trip walks to the trader, the snapshot carries the
// remaining geodata waypoints with the trader destination last so
// the map draws the planned path. The phase publishes too so the
// bot widget banner shows "walking to town".
func TestTripWalkPlanPublishes(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	snap := bot.Snapshot()
	require.NotEmpty(t, snap.WalkPath,
		"the town walk must publish its plan")
	require.Equal(t, "townWalk", snap.Phase,
		"the phase must publish for the activity banner")
	last := snap.WalkPath[len(snap.WalkPath)-1]
	require.Equal(t, state.WalkPoint{
		X: herbielPos[0], Y: herbielPos[1], Z: herbielPos[2],
	}, last, "the trader destination must close the plan")
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
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
	require.Empty(t, game.walks)
}

// TestTripNoPathArmsCooldown verifies that a broken path search (a
// hard error, e.g. no geodata) does not retry every tick.
func TestTripNoPathArmsCooldown(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.fail = true
	fillInventory(bot)

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

// TestTripDryMissArmsCooldown verifies the not found handling of the
// dry planning: the approach search reports no dry route (the walk
// would need a swim), the trip aborts at once with the trigger
// cooldown armed - the old fallback planned the swim anyway and the
// click guard refused it leg by leg until the budget exhausted (the
// town trip variant of the delevel water loop).
func TestTripDryMissArmsCooldown(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	nav.found = false
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"no trip without a dry path")
	require.False(t, loop.tripCooldownOver(), "the cooldown is armed")
	require.Equal(t, 1, nav.calls, "the dry search ran once")
	loop.tick()
	require.Equal(t, 1, nav.calls, "no retry while the cooldown runs")

	loop.tripEndedAt = time.Now().Add(-tripCooldown - time.Second)
	loop.tick()
	require.Equal(t, 2, nav.calls, "a new trip starts after the cooldown")
}

// TestTripFullFlow walks the whole trip: farm to shop, the merchant
// interaction, the sale, the walk back and the hunt resuming.
func TestTripFullFlow(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)

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
	// remaining 16 junk items of the bag keep selling - the trip
	// sells everything sellable, not just past the trigger.
	updates := make([]state.InventoryItem, 0, sellBatchSize)
	for _, item := range game.sells[0] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase,
		"the unsold junk keeps selling")
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	require.Len(t, game.sells, 2)
	require.Len(t, game.sells[1], 41-sellBatchSize)

	// The second batch confirms as well: nothing sellable is left,
	// the purchases plan (nothing worth the adena here) and the
	// walk home starts.
	updates = make([]state.InventoryItem, 0, 41-sellBatchSize)
	for _, item := range game.sells[1] {
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
	fillInventory(bot)
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
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, 1, nav.calls)

	// The character stands still past the stuck timeout: the walker
	// re-paths from the current position.
	loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
	loop.tick()
	require.Equal(t, 2, nav.calls, "the stuck walk re-paths")
	require.Equal(t, phaseTownWalk, loop.phase)

	// The stuck cycles from the same cell climb the frozen leg
	// escalation ladder instead of re-planning the identical route:
	// the detour re-plan first, the direct server routed walk second.
	for range maxRePaths {
		loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
		loop.tick()
		require.Equal(t, phaseTownWalk, loop.phase,
			"the escalation keeps the trip walking")
	}
	require.True(t, loop.directLeg,
		"the ladder armed the direct server routed walk")

	// The direct window burning without progress aborts the trip.
	loop.directLegUntil = time.Now().Add(-directLegWindow - time.Second)
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the trip aborts when the direct walk window is spent")
	require.False(t, loop.tripCooldownOver())
}

// TestTripWaitsForTheFightToEnd pins the combat gate of the trip start:
// a full inventory must not send the character to the vendor while the
// target lives or the loot of the kill still lies on the ground - the
// drops are the point of the fight. Only the between-fights window
// starts the trip.
func TestTripWaitsForTheFightToEnd(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	spawnMob(bot)
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 9, TemplateID: 57, X: 45040, Y: 50040, Z: -3500,
	})

	// The mob is picked as the target while the inventory is still
	// light.
	loop.tick()
	require.Equal(t, int32(7), loop.target)
	require.False(t, loop.tripActive())

	// The bag crosses the trip threshold mid-fight: no vendor walk.
	fillInventory(bot)
	loop.tick()
	require.False(t, loop.tripActive(), "no vendor walk mid-combat")
	require.Equal(t, phaseEngage, loop.phase)

	// The target dies with a drop on the ground: the kill tick enters
	// the loot phase and the loot keeps the trip armed off.
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	bot.ApplySelfTarget(7)
	loop.tick()
	require.Equal(t, phaseLoot, loop.phase)
	require.False(t, loop.tripActive(), "the loot of the kill comes first")

	// The drop is picked up: the loot phase drains, the between-fights
	// window opens and the full inventory starts the trip.
	bot.ApplyItemPickup(state.ItemPickup{ObjectID: 9, PlayerID: 100})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.NotEmpty(t, game.walks)
}

// TestShoppingTripSellsJunkBelowTheTrigger pins the sell policy of
// every vendor trip: a light bag of accumulated junk below the 50
// percent trigger still sells completely when the shopping plan walks
// the character to a merchant - the bot must not farm with the junk
// first and return for the sale later.
func TestShoppingTripSellsJunkBelowTheTrigger(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	items := make([]state.InventoryItem, 0, 6)
	for i := range 5 {
		items = append(items, state.InventoryItem{
			ObjectID: 500 + int32(i), ItemID: 1060, Count: 1,
			Type2: 5, Change: 1,
		})
	}
	items = append(items, state.InventoryItem{
		ObjectID: 999, ItemID: 57, Count: 500, Type2: 4, Change: 1,
	})
	bot.ApplyItemList(items)

	// The shopping plan (500 adena of fillers) starts the trip.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)

	// No merchant visible: skip its wait like the other sell tests.
	loop.merchantID = -1
	loop.sellAt = time.Time{}
	loop.tick()
	require.Len(t, game.sells, 1)
	require.Len(t, game.sells[0], 5,
		"all the accumulated junk sells below the trigger")

	// The sale confirms: the purchases plan with the fresh state
	// and the trip advances to its buy stops.
	updates := make([]state.InventoryItem, 0, 5)
	for _, item := range game.sells[0] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.tick()
	require.True(t, loop.buysPlanned,
		"the buy planning runs after the complete sale")
	require.Equal(t, phaseTownWalk, loop.phase)
}

// TestTripStandsUpBeforeWalking pins the sit guard of the trip start:
// a resting character (the regen sits it down between the fights)
// stands up first - the server refuses move requests while sitting -
// and the trip starts once the ChangeWaitType broadcast confirms the
// standing and the server side stand animation window settles.
func TestTripStandsUpBeforeWalking(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)
	// The character rests: the regeneration sat it down.
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the trip does not start while the character sits")
	require.Equal(t, 1, game.sits, "the stand up toggle is sent")

	// The broadcast confirms the standing, but the server keeps the
	// character on the REST intention through the stand animation
	// window: the walk waits the settle out, no double toggle.
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: false})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the stand animation window holds the trip")
	require.Equal(t, 1, game.sits, "no double toggle")

	// The settle window passes: the trip starts.
	loop.restActionAt = time.Now().Add(-standSettlePeriod - time.Second)
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, 1, game.sits)
	require.NotEmpty(t, game.walks)
}

// TestTripDeathResetsWithoutCooldown verifies that a death during a
// trip drops the trip state but lets a new trip start right after the
// revival: the village restart lands next to the shops. A cooldown a
// recent finished trip armed is cleared as well - the revived
// character sells right there instead of farming with the full bag.
func TestTripDeathResetsWithoutCooldown(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	// A finished trip from minutes ago still holds the cooldown.
	loop.tripEndedAt = time.Now()

	// Death during the walk: the village restart request goes out and
	// the trip state is dropped.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
	require.Equal(t, 1, game.restarts)
	require.Equal(t, phaseEngage, loop.phase)
	require.True(t, loop.tripCooldownOver(),
		"the death clears the trip cooldown")

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

func TestReturnEngagesTargetOnZoneEntry(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	nav := &fakeNavigator{found: true}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	// The character stands inside the zone, the return leg still has
	// waypoints to go.
	loop.SetHuntingZone(45500, 50000, 1500)
	loop.phase = phaseTownReturn
	loop.tripStart = time.Now()
	loop.waypoints = []pathfind.Vec3{{X: 46000, Y: 50000, Z: -3500}}
	loop.wpIndex = 0
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45200, Y: 50000, Name: "Gremlin",
	})
	loop.lastHit = time.Now().Add(-time.Minute)

	// The entry radius offers a target: the return ends instead of
	// walking to the center first, and the next tick attacks.
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the return ends on the zone entry target")
	require.Empty(t, game.walks, "no walking while a target stands in reach")
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
		"the entry target is engaged")
}

func TestReturnWalksOnWithoutZoneTargets(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	nav := &fakeNavigator{found: true}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(45500, 50000, 1500)
	loop.phase = phaseTownReturn
	loop.tripStart = time.Now()
	loop.waypoints = []pathfind.Vec3{{X: 46000, Y: 50000, Z: -3500}}
	loop.wpIndex = 0
	loop.lastHit = time.Now().Add(-time.Minute)

	// No mob in reach: the return keeps walking its waypoints toward
	// the destination.
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase,
		"the return continues without targets")
	require.Len(t, game.walks, 1, "the waypoint walk goes on")
}

// TestSellableJunkSkipsStarterKit pins the newbie kit exclusion of the
// sell trips: the server silently refuses the unsellable starter
// pieces, so offering them only burns a transaction window of the
// flood protector - the destroy flow of the replaced starters owns
// them instead.
func TestSellableJunkSkipsStarterKit(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 910, ItemID: 10, Count: 1, Type2: 0, Change: 1},
		{ObjectID: 911, ItemID: 2369, Count: 1, Type2: 0, Change: 1},
		{ObjectID: 912, ItemID: 1060, Count: 1, Type2: 5, Change: 1},
		{ObjectID: 913, ItemID: 1060, Count: 1, Type2: 5, Change: 1},
	})

	junk := loop.sellableJunk()
	require.Len(t, junk, 2, "only the potions are offered")
	for _, entry := range junk {
		require.Equal(t, int32(1060), entry.ItemID,
			"the starter dagger and sword never enter the junk")
	}
	require.True(t, loop.junkRemaining(),
		"the unoffered potions keep the junk phase running")
	loop.sold[junk[0].ObjectID] = true
	loop.sold[junk[1].ObjectID] = true
	require.False(t, loop.junkRemaining(),
		"nothing sellable remains once the junk is offered")
}

// TestTripClearsTheTalkedNpcSelection pins the target hygiene of the
// npc talks: the merchant select and the teacher click leave the
// villager selected server side (the server never clears a selection,
// only the next selection replaces it), and the trip machinery drops
// it when the conversation is over - the self click of the clear (one
// ClearTarget per stop end and trip end) so the hunting engage that
// follows never adopts the friendly npc as its target.
func TestTripClearsTheTalkedNpcSelection(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	moveSelfTo(bot, herbielPos[0], herbielPos[1], herbielPos[2])
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 55, TemplateID: 7150 + 1000000,
		X: herbielPos[0], Y: herbielPos[1], Z: herbielPos[2],
		Name: "Herbiel",
	})
	loop.tick()
	require.Equal(t, int32(55), loop.merchantID)
	// The merchant is selected and confirmed: the sale may run.
	loop.merchantPick = time.Now().Add(-2 * time.Second)
	loop.tick()
	bot.ApplySelfTarget(55)
	loop.tick()
	require.Len(t, game.sells, 1, "the first sell batch ran")

	// The whole junk sells in two confirmed batches, then the trip
	// ends (nothing worth buying for the test wallet).
	updates := make([]state.InventoryItem, 0, sellBatchSize)
	for _, item := range game.sells[0] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.sellAt = time.Now().Add(-sellPause - time.Second)
	loop.tick()
	updates = make([]state.InventoryItem, 0, 41-sellBatchSize)
	for _, item := range game.sells[1] {
		updates = append(updates, state.InventoryItem{
			ObjectID: item.ObjectID, ItemID: item.ItemID,
			Count: item.Count, Type2: 5, Change: 3,
		})
	}
	bot.ApplyInventoryUpdate(updates)
	loop.tick()

	// The stop ends with Herbiel still selected: the clear fires
	// before the return leg starts.
	require.Equal(t, phaseTownReturn, loop.phase)
	require.GreaterOrEqual(t, game.clears, 1,
		"the stop end dropped the merchant selection")
}

// TestTripInterruptsForTheAttacker pins the aggro answer of the town
// walks: a mob that holds the character as its target stops the trip
// (the soft reset without the cooldown), the character fights the mob
// instead of dragging the chase through the route - the next tick
// re-arms the trip from wherever the fight leaves it.
func TestTripInterruptsForTheAttacker(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	// A mob reaches the walking character mid leg and holds it as
	// its target.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Gremlin",
	})
	mobHitsCharacter(bot)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the trip dropped for the fight")
	require.Equal(t, int32(7), loop.target,
		"the attacker is the fight of the interrupted trip")
	require.True(t, loop.tripEndedAt.IsZero(),
		"the interrupt is a pause: no trip cooldown is armed")
	// The walk requests of the leg stopped for the fight answer: the
	// initial trip leg went out before the mob attacked, nothing
	// follows it while the fight runs.
	require.Len(t, game.walks, 1)

	// The mob dies and the fight bookkeeping settles on its own (the
	// kill, the loot and the empty loot phase are the existing
	// machinery): the soft reset left no trip cooldown, so the still
	// full inventory re-arms the walk on the first decision after the
	// under attack window lapses instead of idling out the five
	// minute cooldown of a finished trip.
}

// TestTripInterruptsIntoDefenseForTheUnwinnableAttacker pins the
// defensive half of the trip interrupt: a mob the character cannot
// win (above the engage ceiling) never becomes the fight of the
// interrupted trip - the standard escape walk answers, the same
// defense the hunting engage runs, and the session logs out when the
// chase never shakes.
func TestTripInterruptsIntoDefenseForTheUnwinnableAttacker(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrLevel, Value: 3},
	})

	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase)
	// The level 8 orc archer chews on the level 3 walker.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000006, Attackable: true,
		X: 45600, Y: 50000, Name: "Orc Archer",
	})
	mobHitsCharacter(bot)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
	require.Zero(t, loop.target, "no fight with the unwinnable mob")
	require.Len(t, game.walks, 2,
		"the trip leg then the standard escape leg answered the interrupt")
	require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[1],
		"the escape runs away from the mob")
	require.False(t, loop.fleeSince.IsZero(),
		"the escape carries the logout budget")
}
