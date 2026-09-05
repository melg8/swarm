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
	selects   int
	forces    []int32
	pickups   []int32
	destroys  []int32
	lastError error
}

func (f *fakeGame) AttackNearest() (int32, error) {
	if f.lastError != nil {
		return 0, f.lastError
	}
	f.selects++

	return 7, nil
}

func (f *fakeGame) AttackTarget(objectID int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.forces = append(f.forces, objectID)

	return nil
}

func (f *fakeGame) PickupItem(item state.LootItem) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.pickups = append(f.pickups, item.ObjectID)

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
	require.Equal(t, 1, game.selects)
	require.Equal(t, int32(7), loop.target)
}

func TestLoopForcesAttackAfterSelection(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The first tick selects the target.
	loop.tick()
	require.Equal(t, 1, game.selects)
	require.Empty(t, game.forces)

	// The server confirms the selection (MyTargetSelected), but no fight
	// packets arrive: the character just stands there. The next ticks must
	// repeat the attack request for the selected target.
	bot.ApplySelfTarget(7)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
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
	require.Equal(t, []int32{7, 7, 7}, game.forces)
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
	require.Equal(t, []int32{7}, game.forces)

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
	require.Equal(t, []int32{7}, game.forces,
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
	require.Equal(t, 0, game.selects)
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

func TestLoopAttackErrorIsLoggedNotFatal(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	game.lastError = errors.New("connection lost")
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Equal(t, 0, game.selects)
	require.Equal(t, int32(0), loop.target)
}
