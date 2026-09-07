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

// pushCommand queues one command like the web server does.
func pushCommand(bot *state.Bot, cmd state.Command) {
	bot.PushCommand(cmd)
}

// TestUserUseItemCommandRunsImmediately pins the one shot semantics of
// the inventory commands: a useItem command executes on the same tick
// it is consumed, no phase switch happens.
func TestUserUseItemCommandRunsImmediately(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	spawnMob(bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 555})
	loop.tick()

	require.Equal(t, []int32{555}, game.uses,
		"the useItem command must execute immediately")
	require.Equal(t, phaseEngage, loop.phase,
		"a useItem command must not switch the phase")
}

// TestUserDropCommandDropsAtSelfPosition pins the drop semantics: the
// server accepts drops at the feet only, so the drop point is the
// character position.
func TestUserDropCommandDropsAtSelfPosition(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{
		Kind: state.CommandDrop, ObjectID: 555, Count: 40,
	})
	loop.tick()

	require.Equal(t, [][5]int32{{555, 40, 45000, 50000, -3500}}, game.drops)
	require.Equal(t, phaseEngage, loop.phase)
}

// TestUserMoveCommandSwitchesToManualPhase pins the move semantics: the
// click switches the loop into the manual phase and the walk request
// goes to the clicked point; arrival ends the manual phase.
func TestUserMoveCommandSwitchesToManualPhase(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	spawnMob(bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45600, Y: 50400, Z: -3500,
	})
	loop.tick()

	require.Equal(t, phaseUser, loop.phase)
	require.Equal(t, state.CommandMove, loop.userKind)
	require.Equal(t, [][3]int32{{45600, 50400, -3500}}, game.walks,
		"the first tick of the manual move must send the walk")
	require.Empty(t, game.forces,
		"the manual move must suppress the autonomous engage")

	// The character arrives at the clicked point: the manual phase ends
	// and the autonomous hunting takes over again.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45600, Y: 50400, Z: -3500,
		DestX: 45600, DestY: 50400, DestZ: -3500,
	})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"arrival must resume the autonomous hunting")
}

// TestUserMoveTimesOut pins the timeout: a walk that never arrives
// hands control back after the manual deadline.
func TestUserMoveTimesOut(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.phase = phaseUser
	loop.userKind = state.CommandMove
	loop.userX = 45000
	loop.userY = 51000
	loop.userZ = -3500
	loop.userStart = time.Now().Add(-userMoveTimeout - time.Second)

	loop.tick()

	require.Equal(t, phaseEngage, loop.phase)
}

// TestUserAttackCommandEngages pins the attack semantics: the manual
// attack pins the target until the fight starts; a died target hands
// control to the looting phase so the drops are picked up.
func TestUserAttackCommandEngages(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
	loop.tick()

	require.Equal(t, phaseUser, loop.phase)
	require.Equal(t, []int32{7}, game.forces)

	// The target died: the manual phase must hand control to the
	// looting phase like the autonomous engage.
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()

	require.Equal(t, phaseLoot, loop.phase,
		"a killed manual target must switch into the loot phase")
	require.Empty(t, loop.userKind)
}

// TestUserPickupCommandWalksAndPicks pins the pickup semantics: a far
// item is approached with a walk, a close item is clicked, and a
// vanished item ends the manual phase.
func TestUserPickupCommandWalksAndPicks(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 900, TemplateID: 57, Stackable: true, Count: 25,
		X: 45040, Y: 50040, Z: -3500,
	})

	pushCommand(bot, state.Command{
		Kind: state.CommandPickup, ObjectID: 900,
	})
	loop.tick()

	require.Equal(t, phaseUser, loop.phase)
	require.Equal(t, []int32{900}, game.pickups,
		"a close item is clicked directly on the first tick")

	// The item is gone (picked up): the manual phase ends.
	bot.ApplyItemPickup(state.ItemPickup{ObjectID: 900, PlayerID: 100})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
}

// TestUserPickupFarItemWalksFirst pins the approach: an item outside
// the approach radius is walked to before the click.
func TestUserPickupFarItemWalksFirst(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 901, TemplateID: 57, Stackable: true, Count: 25,
		X: 46500, Y: 51500, Z: -3500,
	})

	pushCommand(bot, state.Command{
		Kind: state.CommandPickup, ObjectID: 901,
	})
	loop.tick()

	require.Equal(t, [][3]int32{{46500, 51500, -3500}}, game.walks)
	require.Empty(t, game.pickups)
}

// TestUserCommandIgnoredDuringDelevel pins the delevel guard: the guard
// walk must finish, so movement commands are refused.
func TestUserCommandIgnoredDuringDelevel(t *testing.T) {
	loop, game, bot, _ := newDelevelLoop(11)
	spawnZoneMobs(bot)

	// The outleveled character starts the deleveling.
	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)
	walksBefore := len(game.walks)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45000, Y: 51000, Z: -3500,
	})
	loop.tick()

	require.Equal(t, phaseDelevel, loop.phase,
		"a move command must not interrupt the deleveling")
	require.Len(t, game.walks, walksBefore,
		"the refused command must not add walks")
}

// TestUserCommandsDrainOnReset pins the queue reset of a session: a
// reconnect never replays the stale clicks of the previous session.
func TestUserCommandsDrainOnReset(t *testing.T) {
	bot := newTestBot()
	//nolint:exhaustruct // fake keeps zero defaults
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45000, Y: 51000, Z: -3500,
	})
	bot.ResetSession()
	loop.tick()

	require.Empty(t, game.walks,
		"the reset must drop the queued commands")
	require.Equal(t, phaseEngage, loop.phase)
}
