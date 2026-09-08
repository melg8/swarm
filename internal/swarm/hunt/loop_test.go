// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// fakeGame records the actions of the hunt loop and simulates the Mobius
// double click semantics: the first attack request for a new target only
// selects it, the repeated request starts the fight.
type fakeGame struct {
	forces    []int32
	pickups   []int32
	walks     [][3]int32
	sits      int
	restarts  int
	destroys  [][2]int32
	sells     [][]state.InventoryItem
	buys      [][]gear.Purchase
	uses      []int32
	drops     [][5]int32
	noTargets bool
	logouts   int
	lastError error
}

func (f *fakeGame) AttackTarget(objectID int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.forces = append(f.forces, objectID)

	return nil
}

func (f *fakeGame) WalkTo(x int32, y int32, z int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.walks = append(f.walks, [3]int32{x, y, z})

	return nil
}

func (f *fakeGame) PickupItem(item state.LootItem) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.pickups = append(f.pickups, item.ObjectID)

	return nil
}

func (f *fakeGame) ActionSitStand() error {
	if f.lastError != nil {
		return f.lastError
	}
	f.sits++

	return nil
}

func (f *fakeGame) RestartAtVillage() error {
	if f.lastError != nil {
		return f.lastError
	}
	f.restarts++

	return nil
}

func (f *fakeGame) DestroyItem(objectID int32, count int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.destroys = append(f.destroys, [2]int32{objectID, count})

	return nil
}

func (f *fakeGame) SellItems(items []state.InventoryItem) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.sells = append(f.sells, items)

	return nil
}

func (f *fakeGame) BuyItems(_ int32, items []gear.Purchase) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.buys = append(f.buys, items)

	return nil
}

func (f *fakeGame) UseItem(objectID int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.uses = append(f.uses, objectID)

	return nil
}

func (f *fakeGame) RequestLogout() error {
	if f.lastError != nil {
		return f.lastError
	}
	f.logouts++

	return nil
}

func (f *fakeGame) DropItem(
	objectID int32, count int32, x int32, y int32, z int32,
) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.drops = append(f.drops, [5]int32{objectID, count, x, y, z})

	return nil
}

func newTestBot() *state.Bot {
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	// A healthy character: the hunt loop engages the next target
	// immediately when the HP is above the re-engage threshold.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrMaxHP, Value: 100},
		{ID: state.AttrCurHP, Value: 90},
	})

	return bot
}

// spawnMob adds an attackable npc in reach of the test character.
func spawnMob(bot *state.Bot) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Name: "Gremlin",
	})
}

func TestLoopAttacksWhenIdle(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	// The pick happens through the tracker and the first request goes
	// out immediately: on the server it only selects the target.
	require.Equal(t, []int32{7}, game.forces)
	require.Equal(t, int32(7), loop.target)
}

func TestLoopForcesAttackAfterSelection(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The first tick sends the selecting request.
	loop.tick()
	require.Equal(t, []int32{7}, game.forces)

	// The server confirms the selection (MyTargetSelected), but no fight
	// packets arrive: the character just stands there. The next ticks must
	// repeat the attack request for the selected target.
	bot.ApplySelfTarget(7)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, []int32{7, 7}, game.forces,
		"the loop must send the second request that starts the attack")
}

func TestLoopRetriesForcedAttackUntilEngaged(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	bot.ApplySelfTarget(7)

	// The server keeps ignoring the forced requests: the loop retries
	// every engage retry period instead of standing still forever.
	for range 3 {
		loop.lastHit = time.Now().Add(-2 * time.Second)
		loop.tick()
	}
	require.Equal(t, []int32{7, 7, 7, 7}, game.forces)
}

func TestLoopStopsRequestingOnceEngaged(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	bot.ApplySelfTarget(7)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, []int32{7, 7}, game.forces)

	// The fight starts: the server broadcasts the chase and the auto
	// attack. The loop must stop re-requesting the target.
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 40,
		X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000, TargetZ: -3500,
	})
	for range 3 {
		loop.lastHit = time.Now().Add(-time.Minute)
		loop.tick()
	}
	require.Equal(t, []int32{7, 7}, game.forces,
		"no further forced attack requests once the fight runs")
}

func TestEngageSwitchesStuckTarget(t *testing.T) {
	bot := newTestBot()
	// The nearest mob and a second one a bit farther away.
	spawnMob(bot)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 46200, Y: 50100, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The nearest target is picked and its selection confirmed, but
	// the fight never starts: every forced attack request comes back
	// refused (the server keeps a corpse selected from an abruptly
	// disconnected session and never clears the selection).
	loop.tick()
	bot.ApplySelfTarget(7)
	require.Equal(t, []int32{7}, game.forces)

	// Past the stuck timeout the target is dropped and skipped.
	loop.engageAt = time.Now().Add(-engageStuckTimeout - time.Second)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, int32(0), loop.target, "the stuck target is dropped")
	require.True(t, loop.targetSkipped(7, time.Now()),
		"the stuck target is skipped for the search")

	// The next pick takes the second mob: the selection of a
	// different object id is what replaces the stale server
	// selection, and the attack request goes to it.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, int32(8), loop.target)
	require.Equal(t, []int32{7, 8}, game.forces)

	// The server may keep reporting the skipped id as the selected
	// target (the corpse selection never clears on its own): the
	// adoption must ignore it while the skip delay lasts.
	bot.ApplySelfTarget(7)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, int32(8), loop.target,
		"the skipped target is not re-adopted")
	require.Equal(t, []int32{7, 8, 8}, game.forces)
}

func TestLoopLootsAfterKill(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A killed target with a drop next to the corpse.
	spawnMob(bot)
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 9, TemplateID: 57, X: 45040, Y: 50040, Z: -3500,
	})
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	bot.ApplySelfTarget(7)
	loop.target = 7

	loop.tick()
	// The dead target moves the loop into the loot phase and the drop
	// is clicked.
	require.Equal(t, phaseLoot, loop.phase)
	require.Empty(t, game.forces)
	require.Equal(t, []int32{9}, game.pickups)
}

func TestLoopSkipsUnreachableLoot(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.phase = phaseLoot
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 9, TemplateID: 57, X: 45040, Y: 50040, Z: -3500,
	})

	// First tick starts the attempt, the item stays on the ground.
	loop.tick()
	require.Equal(t, []int32{9}, game.pickups)
	loop.lootAt = time.Now().Add(-(pickupTimeout + time.Second))

	// The timed out attempt is skipped and the phase returns to engage.
	loop.tick()
	require.Len(t, loop.skipped, 1)
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
}

func TestLoopWalksToFarLoot(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.phase = phaseLoot
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 9, TemplateID: 57, X: 45600, Y: 50000, Z: -3500,
	})

	// 600 units away: the loop walks to the item instead of clicking it.
	loop.tick()
	require.Empty(t, game.pickups)
	require.Equal(t, [][3]int32{{45600, 50000, -3500}}, game.walks)

	// The character arrives (the zero distance MoveToLocation of the
	// server): the next tick clicks the item.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45600, Y: 50000, Z: -3500,
		DestX: 45600, DestY: 50000, DestZ: -3500,
	})
	loop.lootMoveAt = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{9}, game.pickups)
}

func TestLoopWalksBackIntoTheZone(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	game.noTargets = true
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The character respawns far outside the hunting square: the leash
	// walks it back toward the zone center in short legs (the server
	// refuses move requests beyond 9900 units), not one direct walk.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Len(t, game.walks, 1, "the leash walks toward the zone center")
	leg := game.walks[0]
	dist := math.Hypot(float64(leg[0]-49308), float64(leg[1]-44213))
	// The int32 truncation of the leg target extends the leg by up to
	// sqrt(2) units; the server move limit itself is 9900.
	require.LessOrEqual(t, dist, returnWalkLeg+2,
		"the return leg stays below the server move limit")
	require.Greater(t, dist, 0.0, "the leg moves")
	dx := float64(leg[0] - 49308)
	dy := float64(leg[1] - 44213)
	tdx := float64(46112 - 49308)
	tdy := float64(41500 - 44213)
	perp := math.Abs(dx*tdy-dy*tdx) / math.Hypot(tdx, tdy)
	require.LessOrEqual(t, perp, 2.0,
		"the leg points at the zone center")

	// Back inside: the zone no longer pushes the walk.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 46112, Y: 41500, Z: -3539,
		DestX: 46112, DestY: 41500, DestZ: -3539,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Len(t, game.walks, 1, "no walk while inside the zone")
}

func TestLoopPathfindsBackIntoTheZone(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	game.noTargets = true
	nav := &fakeNavigator{found: true}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The character respawns outside the hunting square: the return is
	// planned through the geodata navigator (the walls and rivers between
	// the village and the fields make direct legs walk into obstacles).
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase,
		"the zone return follows the geodata waypoints")
	require.Equal(t, 1, nav.calls, "the return leg was planned")
	require.Empty(t, game.walks, "no walk before the follower tick")

	// The follower walks the planned waypoints in server legal legs.
	loop.tick()
	require.Len(t, game.walks, 1, "the follower walks the waypoints")
	leg := game.walks[0]
	require.LessOrEqual(t,
		math.Hypot(float64(leg[0]-49308), float64(leg[1]-44213)),
		maxMoveLeg+2, "the waypoint leg respects the server move limit")

	// Arriving home ends the return and resumes the hunt.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 46112, Y: 41500, Z: -3539,
		DestX: 46112, DestY: 41500, DestZ: -3539,
	})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the return ends back in the zone")
	// The next engage decision clears the return state.
	loop.tick()
	require.False(t, loop.zoneReturn)
}

func TestLoopIgnoresMobsOutsideTheZone(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A mob outside the hunting square is not attacked even though it
	// is the closest one.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Name: "Gremlin",
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Empty(t, game.forces,
		"no attack requests for a mob outside the zone")

	// The character walks back first; once inside the zone a mob of
	// the zone is attacked.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 46112, Y: 41500, Z: -3539,
		DestX: 46112, DestY: 41500, DestZ: -3539,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 46200, Y: 41800, Name: "Inside Gremlin",
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{8}, game.forces,
		"the mob inside the zone is attacked")
}

func TestLoopSitsDownWhenExhausted(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// HP below the sit threshold: the loop sits down to regenerate.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, game.sits, "an exhausted character sits down")

	// The server confirms the sit with ChangeWaitType: no repeated
	// toggles while the regeneration runs.
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	for range 3 {
		loop.lastHit = time.Now().Add(-2 * time.Second)
		loop.tick()
	}
	require.Equal(t, 1, game.sits)

	// Regeneration recovers: the loop stands up and engages.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 95},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 2, game.sits, "a recovered character stands up")
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: false})

	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
		"a standing character hunts again")
}

func TestLoopSitsAtTheNewThreshold(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// HP 55 sits (the old threshold was 30 and too low to survive the
	// next fight), HP 65 engages right away.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 55},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, game.sits, "55 percent sits down")

	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 65},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces, "65 percent engages again")
}

func TestLoopEscapesWhenHurtUnderAttack(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A mob hits the character (Attack broadcast with the bot as
	// target) while its HP is low: the character keeps moving instead
	// of sitting into the blows or starting a fight it cannot win -
	// one escape leg away from the attacker.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})

	bot.ApplyAttack(state.Attack{
		AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Zero(t, game.sits, "no sitting into the blows of a fight")
	require.Empty(t, game.forces,
		"a hurt character does not start a losing fight")
	require.Len(t, game.walks, 1, "one escape leg away from the attacker")
	require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[0])

	// The escape walk is paced: the next ticks do not spam move
	// requests while the chase holds.
	for range 3 {
		loop.lastHit = time.Now().Add(-2 * time.Second)
		loop.tick()
	}
	require.Len(t, game.walks, 1, "the escape legs are paced")

	// The health recovers above the hurt gate: even under the fresh
	// blows the character stops running and answers the attacker.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
		"a recovered character engages the attacker")
}

func TestLoopRestartsAfterDeath(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Equal(t, []int32{7}, game.forces)

	// The character dies mid fight: the loop stops hunting and requests
	// the village restart, dropping the stale target.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, game.restarts)
	require.Equal(t, int32(0), loop.target)

	// The request retries until the server revives the character.
	loop.restartAt = time.Now().Add(-6 * time.Second)
	loop.tick()
	require.Equal(t, 2, game.restarts)

	// Revived: the hunt continues with a fresh target selection.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 197},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Len(t, game.forces, 2)
	require.Equal(t, 2, game.restarts, "no restart spam after the revival")
}

func TestLoopAttackErrorIsLoggedNotFatal(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	game.lastError = errors.New("connection lost")
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	// The target is picked through the tracker; the request error only
	// delays the retry.
	require.Equal(t, int32(7), loop.target)
	require.Empty(t, game.forces)
}

func TestLoopSelectsNextTargetAfterKill(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The first mob is selected, fought and killed. The server never
	// clears the selection of the corpse: the tracker target stays at
	// the dead object id (simulated by re-selecting it after death).
	loop.tick()
	bot.ApplySelfTarget(7)
	loop.lastHit = time.Now().Add(-time.Minute)
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	bot.ApplySelfTarget(7)

	// Loot finishes with no drops on the next ticks: the loop must
	// select a new target instead of ping-ponging between the engage
	// and loot phases around the stale dead selection. A second living
	// mob stands by for the re-pick.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45900, Y: 50000, Name: "Second Gremlin",
	})
	for range 3 {
		loop.lastHit = time.Now().Add(-2 * time.Second)
		loop.tick()
	}
	require.Equal(t, int32(8), loop.target,
		"the next target must be selected after the kill")
}

func TestLoopWaitsForHealthWhenHurt(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The character is hurt: no new engagement until regeneration
	// recovers above the threshold.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})
	for range 3 {
		loop.lastHit = time.Now().Add(-2 * time.Second)
		loop.tick()
	}
	require.Empty(t, game.forces, "a hurt character must rest")

	// The regeneration recovers: the next target is selected.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
		"a recovered character engages the next target")
}

func TestLoopSkipsTooStrongTargets(t *testing.T) {
	bot := newTestBot()
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrLevel, Value: 3},
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000006, Attackable: true,
		X: 45100, Y: 50000, Name: "Orc Archer",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45500, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The level 3 character never initiates on the level 8 orc (the
	// two level slack), the level 1 gremlin behind it is the pick.
	loop.tick()
	require.Equal(t, int32(8), loop.target,
		"the level 8 orc stays out of the level 3 slack")
	require.Equal(t, []int32{8}, game.forces)
}

func TestLoopEscapesALosingFight(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// The fight runs on mob 7 and the health collapses under the
	// escape threshold (the mob vitals stay unknown - the low own
	// health alone triggers the escape).
	loop.target = 7
	bot.ApplySelfTarget(7)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Equal(t, int32(0), loop.target, "the losing fight is dropped")
	require.Empty(t, game.forces, "no further attack requests")
	require.Len(t, game.walks, 1, "one escape leg away from the mob")
	require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[0])
	require.True(t, loop.targetSkipped(7, time.Now().Add(time.Minute)),
		"the fled target stays out of the search for the long delay")
}

func TestLoopEscapesTheLevelGapFight(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000006, Attackable: true,
		X: 45600, Y: 50000, Name: "Orc Archer",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// The mob keeps 90 percent of its health while the character sank
	// to 45: the gap fight is a death risk even above the panic
	// threshold, the escape opens early.
	loop.target = 7
	bot.ApplySelfTarget(7)
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
		{ID: state.AttrMaxHP, Value: 100},
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 45},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Equal(t, int32(0), loop.target, "the gap fight is dropped early")
	require.Len(t, game.walks, 1, "one escape leg away from the mob")
}

func TestLoopFinishesTheBeatenTarget(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// The character is hurt but the target is one swing from dead:
	// finishing the kill beats fleeing and dropping the loot.
	loop.target = 7
	bot.ApplySelfTarget(7)
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 10},
		{ID: state.AttrMaxHP, Value: 100},
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Equal(t, int32(7), loop.target, "the beaten target stays")
	require.Equal(t, []int32{7}, game.forces, "the fight continues")
	require.Empty(t, game.walks, "no escape while the kill is close")
}

func TestLoopPatrolsTowardTheCenterWithoutTargets(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46500, 50800, 1500)
	loop.lastHit = time.Now().Add(-time.Minute)

	// No targets around: the first empty search arms the patience, no
	// walk yet.
	loop.tick()
	require.Empty(t, game.walks, "the patience holds the center walk")

	// The patience expires: one paced leg toward the zone center (the
	// next searches pick up any mob the walk passes).
	loop.noTargetSince = time.Now().Add(-noTargetPatience - time.Second)
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Len(t, game.walks, 1, "an idle hunter walks toward the center")
	leg := game.walks[0]
	dx := float64(46500 - 45000)
	dy := float64(50800 - 50000)
	dist := math.Hypot(dx, dy)
	frac := math.Min(1, returnWalkLeg/dist)
	require.InDelta(t, float64(45000)+dx*frac, float64(leg[0]), 1)
	require.InDelta(t, float64(50000)+dy*frac, float64(leg[1]), 1)
}

func TestLoopWalksToFarTargetsOfABigZone(t *testing.T) {
	bot := newTestBot()
	// The pack sits inside the big square but far outside the engage
	// radius: the character stands at the west edge, the mob 2000
	// units east (attackNearestRange is 1500). The hunter must walk
	// toward the pack instead of standing still.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 9, TemplateID: 1000001, Attackable: true,
		X: 47000, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46000, 50000, 1300)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.noTargetSince = time.Now().Add(-noTargetPatience - time.Second)

	loop.tick()
	require.NotEmpty(t, game.walks,
		"the targetless hunter walks toward the far pack")
	walk := game.walks[len(game.walks)-1]
	require.True(t, walk[0] > 45000 && walk[0] <= 46000,
		"the walk leg heads toward the mob (x grows, capped at the leg length)")
	require.Equal(t, int32(50000), walk[1],
		"the walk keeps the line to the mob")
}

func TestLoopKeepsStandingWhenTheFarMobEntersTheEngageRadius(t *testing.T) {
	bot := newTestBot()
	// The mob enters the engage radius on its own: the far target walk
	// must not fire, the normal engage pick takes over.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 9, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46000, 50000, 1300)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.noTargetSince = time.Now().Add(-noTargetPatience - time.Second)

	loop.tick()
	require.Empty(t, game.walks,
		"no far target walk while a valid target sits inside the radius")
	require.Equal(t, []int32{9}, game.forces,
		"the engage picks the target directly")
}

func TestLoopLogsOutWhenTheFleeNeverShakesTheChase(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A hurt character under attack flees - and the chase never ends:
	// after the flee budget the session logs out instead of running
	// forever, arming the 30 s login pause.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 40},
	})
	bot.ApplyAttack(state.Attack{
		AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Zero(t, game.logouts, "the fresh flee starts with a budget")

	// The episode ages past the budget while the blows keep landing:
	// the next escape leg never happens, the logout does.
	loop.fleeAt = time.Now().Add(-2 * time.Second)
	loop.fleeSince = time.Now().Add(-fleeLogoutAfter - time.Second)
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, game.logouts,
		"the endless chase ends the session")
	require.True(t, loop.logoutDone)
	require.GreaterOrEqual(t, bot.LoginCooldownRemaining(),
		panicLogoutPause-time.Second,
		"the relogin pause resets the mob aggro")

	// A single fresh flee never logs out: the budget only fires on a
	// long running episode.
	bot2 := newTestBot()
	spawnMob(bot2)
	game2 := &fakeGame{}
	loop2 := NewLoop(game2, bot2)
	loop2.lastHit = time.Now().Add(-time.Minute)
	bot2.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 40},
	})
	bot2.ApplyAttack(state.Attack{
		AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop2.lastHit = time.Now().Add(-2 * time.Second)
	loop2.tick()
	require.Zero(t, game2.logouts,
		"a fresh escape episode keeps the session alive")
}

func TestLoopLogsOutAtCriticalHealthUnderAttack(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The chase cornered the character: critical health, the blows
	// still landing.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 10},
	})
	bot.ApplyAttack(state.Attack{
		AttackerID: 7, X: 45600, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Equal(t, 1, game.logouts, "the emergency logout fires")
	require.Greater(t, bot.LoginCooldownRemaining(), 20*time.Second,
		"the login cooldown covers the combat stance and the reset")
	require.LessOrEqual(t, bot.LoginCooldownRemaining(), 30*time.Second,
		"the login cooldown is half a minute, not minutes")
	require.Len(t, game.walks, 1,
		"the last escape leg keeps the offline character moving")
	require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[0])

	// The request is one shot: the dying ticks stay quiet while the
	// session unwinds.
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, game.logouts)
	require.Len(t, game.walks, 1)
}

func TestLoopLogsOutWhenTwoMobsAggro(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Gremlin",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45400, Y: 50200, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The social pile up: a second gremlin joins the fight while
	// the character is still healthy - the pack only grows, the
	// logout fires on the attacker count alone.
	for _, id := range []int32{7, 8} {
		bot.ApplyAttack(state.Attack{
			AttackerID: id, X: 45500, Y: 50000, Z: -3500,
			TargetCount: 1,
			TargetIDs:   [state.AttackTargets]int32{100},
			TargetX:     45000, TargetY: 50000, TargetZ: -3500,
		})
	}
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Equal(t, 1, game.logouts,
		"two attackers trigger the emergency logout")
	require.LessOrEqual(t, bot.LoginCooldownRemaining(),
		30*time.Second, "the reconnect pause is half a minute")

	// The request stays one shot while the session unwinds.
	loop.tick()
	require.Equal(t, 1, game.logouts)
}

func TestLoopKeepsFightingAgainstOneAttacker(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A single fair fight never logs the character out, however
	// hard the mob swings.
	bot.ApplyAttack(state.Attack{
		AttackerID: 7, X: 45600, Y: 50000, Z: -3500,
		TargetCount: 1,
		TargetIDs:   [state.AttackTargets]int32{100},
		TargetX:     45000, TargetY: 50000, TargetZ: -3500,
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Zero(t, game.logouts,
		"one attacker is a normal fight, not a pile up")
	require.Equal(t, 0, int(bot.LoginCooldownRemaining()))
}

func TestLoopKeepsFightingAtCriticalHealthWithoutAggro(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// Critical health with nobody landing blows: the panic logout
	// waits - the character escapes or fights on its own terms.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 10},
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Zero(t, game.logouts,
		"no aggro, no logout - the rest and escape own the case")
}

func TestLoopDoesNotPanicLogoutWhileDeleveling(t *testing.T) {
	loop, game, _, _ := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)

	// The deleveling starts: level 11 over the level 1 zone mobs.
	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)

	// The guard deaths drive the health to critical with the blows
	// landing: the panic logout must stay off - the deaths are the
	// point of the phase.
	loop.tracker.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 8},
	})
	loop.tracker.ApplyAttack(state.Attack{
		AttackerID: 7, X: 45050, Y: 50050, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Zero(t, game.logouts, "the delevel deaths are the point")
	require.Equal(t, phaseDelevel, loop.phase)
}
