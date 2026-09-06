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

// fakeGame records the actions of the hunt loop and simulates the Mobius
// double click semantics: the first attack request for a new target only
// selects it, the repeated request starts the fight.
type fakeGame struct {
	forces    []int32
	pickups   []int32
	walks     [][3]int32
	sits      int
	restarts  int
	destroys  []int32
	noTargets bool
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

func (f *fakeGame) DestroyItem(objectID int32, _ int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.destroys = append(f.destroys, objectID)

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
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Name: "Gremlin",
	})
}

func TestLoopAttacksWhenIdle(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // partial fields for the case
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

func TestLoopLootsAfterKill(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A killed target with a drop next to the corpse.
	spawnMob(bot)
	//nolint:exhaustruct // partial fields for the case
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
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.phase = phaseLoot
	//nolint:exhaustruct // partial fields for the case
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
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.phase = phaseLoot
	//nolint:exhaustruct // partial fields for the case
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
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	game.noTargets = true
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The character respawns far outside the hunting square: the leash
	// walks it back to the zone center instead of idling.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, [][3]int32{{46112, 41500, -3539}}, game.walks,
		"the leash walks to the zone center")

	// Back inside: the zone no longer pushes the walk.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 46112, Y: 41500, Z: -3539,
		DestX: 46112, DestY: 41500, DestZ: -3539,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Len(t, game.walks, 1, "no walk while inside the zone")
}

func TestLoopIgnoresMobsOutsideTheZone(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A mob outside the hunting square is not attacked even though it
	// is the closest one.
	//nolint:exhaustruct // partial fields for the case
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
	//nolint:exhaustruct // partial fields for the case
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
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // fake keeps zero defaults
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

func TestLoopDoesNotSitWhileUnderAttack(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A mob hits the character (Attack broadcast with the bot as target)
	// while its HP is low: the loop keeps fighting instead of sitting.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyAttack(state.Attack{
		AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Zero(t, game.sits, "no sitting into the blows of a fight")
	require.Equal(t, []int32{7}, game.forces,
		"the fight continues instead")
}

func TestLoopRestartsAfterDeath(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // fake keeps zero defaults
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
	//nolint:exhaustruct // partial fields for the case
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
	//nolint:exhaustruct // fake keeps zero defaults
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
