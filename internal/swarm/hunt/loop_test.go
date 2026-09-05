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

// fakeGame records the actions of the hunt loop.
type fakeGame struct {
	attacks   int
	pickups   []int32
	destroys  []int32
	lastError error
}

func (f *fakeGame) AttackNearest() (bool, error) {
	if f.lastError != nil {
		return false, f.lastError
	}
	f.attacks++

	return true, nil
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

func TestLoopAttacksWhenIdle(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Equal(t, 1, game.attacks)
}

func TestLoopLootsAfterKill(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// A killed target with a drop next to the corpse.
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Name: "Gremlin",
	})
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
	require.Equal(t, 0, game.attacks)
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

func TestLoopCleansInventoryWhenFull(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	items := make([]state.InventoryItem, 60)
	for i := range items {
		//nolint:gosec // bounded loop index below math.MaxInt32
		items[i] = state.InventoryItem{
			ObjectID: int32(i + 1),
			ItemID:   1060,
			Count:    1,
			Type1:    0,
			Type2:    5,
			Equipped: false,
			Change:   0,
		}
	}
	bot.ApplyItemList(items)

	loop.tick()
	require.NotEmpty(t, game.destroys)
	require.Len(t, game.destroys, destroyBatch)
}

func TestLoopSurvivesActionErrors(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	game.lastError = errors.New("connection lost")
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
	require.Equal(t, 0, game.attacks)
}
