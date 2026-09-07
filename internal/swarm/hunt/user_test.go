// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
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

// TestUserDestroyCommandRunsImmediately pins the destroy semantics of
// the trash target: the stack is destroyed by its count without a
// phase switch, the same one shot execution as the drop.
func TestUserDestroyCommandRunsImmediately(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{
		Kind: state.CommandDestroy, ObjectID: 570, Count: 12,
	})
	loop.tick()

	require.Equal(t, [][2]int32{{570, 12}}, game.destroys,
		"the destroy command must execute immediately with its count")
	require.Equal(t, phaseEngage, loop.phase,
		"a destroy command must not switch the phase")
}

// TestUserDestroyRejectsGarbage pins the guard: a destroy without an
// object or with a non positive count is dropped silently.
func TestUserDestroyRejectsGarbage(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{Kind: state.CommandDestroy, ObjectID: 570})
	loop.tick()
	pushCommand(bot, state.Command{
		Kind: state.CommandDestroy, ObjectID: 570, Count: -3,
	})
	loop.tick()

	require.Empty(t, game.destroys,
		"garbage destroy commands must not reach the server")
}

// TestUserAttackReRequestsStaleEngagement pins the attack refresh: the
// auto attack flag and the combat window outlive an interrupted fight,
// so the manual attack keeps re-requesting the forced attack of the
// already selected target until the swings actually resume - the
// "double click on the selected target" fix.
func TestUserAttackReRequestsStaleEngagement(t *testing.T) {
	bot := newTestBot()
	// The mob stands in melee range: an out of range target is
	// approached with a walk, the re-request case needs the swing
	// range.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45100, Y: 50000, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
	loop.tick()
	require.Equal(t, []int32{7}, game.forces)

	// The server selected the target and the fight once ran (a swing
	// of the character landed): the tracker marks a fresh fight.
	bot.ApplySelfTarget(7)
	bot.ApplyAttack(state.Attack{
		AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
		TargetX: 46000, TargetY: 50000, TargetZ: -3500,
		TargetCount: 1, TargetIDs: [state.AttackTargets]int32{7},
	})
	require.True(t, bot.SelfEngaged(7),
		"the swing must mark the engagement")

	// The fight stops without the server clearing anything: the swings
	// stop landing and the engagement goes stale while the loose combat
	// window (10 seconds) still claims the fight is running. The poll
	// self calibrates on the public views instead of pinning the exact
	// fresh window constant.
	deadline := time.Now().Add(6 * time.Second)
	for bot.SelfFighting(7) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	require.False(t, bot.SelfFighting(7),
		"the fight must go stale without swings")
	require.True(t, bot.SelfEngaged(7),
		"the loose combat window must still claim the fight")

	loop.userMoveAt = time.Now().Add(-2 * time.Second)
	loop.tick()

	require.Equal(t, []int32{7, 7}, game.forces,
		"a stale engagement must re-request the forced attack")
	require.Equal(t, phaseUser, loop.phase)
}

// TestUserAttackFreshFightStaysQuiet pins the quiet phase and the
// deadline refresh: while the swings land the loop neither re-requests
// nor times the manual attack out, so a long fight under manual
// control never hands control back mid swing.
func TestUserAttackFreshFightStaysQuiet(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
	loop.tick()
	require.Equal(t, []int32{7}, game.forces)

	// The chase step of the running fight keeps the engagement fresh.
	bot.ApplySelfTarget(7)
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 60,
		X: 45900, Y: 50000, Z: -3500,
		TargetX: 46000, TargetY: 50000, TargetZ: -3500,
	})
	// Far past the deadline: the fresh fight must have pushed it out.
	loop.userStart = time.Now().Add(-userAttackTimeout - time.Minute)
	loop.tick()

	require.Equal(t, []int32{7}, game.forces,
		"a fresh fight must not re-request the attack")
	require.Equal(t, phaseUser, loop.phase,
		"a fresh fight must refresh the manual deadline")
}

// TestManualOnlyModeRunsCommandsAndStaysIdle pins the manual only mode
// of a session started without -hunt: the commands of the web UI still
// execute, the loop never engages anything on its own and returns to
// the idle phase when a manual action completes.
func TestManualOnlyModeRunsCommandsAndStaysIdle(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetAutonomy(false)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Empty(t, game.forces,
		"the manual only loop must not engage targets on its own")
	require.Equal(t, phaseIdle, loop.phase)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45600, Y: 50400, Z: -3500,
	})
	loop.tick()
	require.Equal(t, phaseUser, loop.phase)
	require.Equal(t, [][3]int32{{45600, 50400, -3500}}, game.walks,
		"the manual move command must still walk")

	// The character arrives: the manual phase ends in the idle phase
	// instead of resuming a hunt that does not exist.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45600, Y: 50400, Z: -3500,
		DestX: 45600, DestY: 50400, DestZ: -3500,
	})
	loop.tick()
	require.Equal(t, phaseIdle, loop.phase)
}

// TestManualOnlyKillReturnsToIdle pins the kill branch of the manual
// only mode: a dead manual target ends the manual phase in the idle
// phase, no looting phase follows because there is no hunt to resume.
func TestManualOnlyKillReturnsToIdle(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetAutonomy(false)

	pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
	loop.tick()

	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()

	require.Equal(t, phaseIdle, loop.phase)
	require.Empty(t, loop.userKind)
}

// TestManualOnlyDeathRestartsAtVillage pins the death recovery of the
// manual only mode: the village restart still runs (the official death
// dialog choice) and the phase stays idle afterwards.
func TestManualOnlyDeathRestartsAtVillage(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetAutonomy(false)

	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()

	require.Equal(t, 1, game.restarts,
		"the manual only loop must still restart after a death")
	require.Equal(t, phaseIdle, loop.phase)
}

// TestUserMoveFarWalkPlansLegs pins the geodata planning of a long
// manual move: the click beyond the planning distance asks the
// navigator once and follows its waypoints leg by leg (the far leg
// splits into server accepted pieces), the arrival at the final
// waypoint ends the manual phase.
func TestUserMoveFarWalkPlansLegs(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	navigator := &fakeNavigator{found: true}
	loop.SetNavigator(navigator)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The click lands 6000 units south: past the planning threshold.
	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45000, Y: 44000, Z: -3500,
	})
	loop.tick()
	require.Empty(t, game.walks,
		"the planning tick must not walk yet")
	require.NotNil(t, loop.userWaypoints,
		"the far click must plan the geodata path")

	// The follower walks the first leg on the next tick: the fake
	// path runs from the self position to the clicked point, the first
	// leg is capped by the server move limit.
	loop.tick()
	require.Len(t, game.walks, 1,
		"the follower must walk the first planned leg")
	first := game.walks[0]
	dist := math.Hypot(
		float64(first[0]-45000), float64(first[1]-50000))
	require.LessOrEqual(t, dist, maxMoveLeg+1,
		"the leg must stay within the server move limit")

	// The character completes the first leg: the follower issues the
	// next one without asking the navigator again (the request period
	// of the legs is backed off like a real walk pace).
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: first[0], Y: first[1], Z: -3500,
		DestX: first[0], DestY: first[1], DestZ: -3500,
	})
	loop.userMoveAt = time.Now().Add(-2 * walkRequestPeriod)
	loop.tick()
	require.Len(t, game.walks, 2,
		"the completed leg must be followed by the next one")
	require.Equal(t, 1, navigator.calls,
		"the path search must run exactly once per move")

	// The character arrives at the final waypoint: the manual phase
	// ends.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45000, Y: 44000, Z: -3500,
		DestX: 45000, DestY: 44000, DestZ: -3500,
	})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"the completed path must resume the autonomous hunting")
}

// TestUserMoveNearWalkGoesDirect pins the short click behavior: a
// click inside the planning distance never asks the navigator and
// walks straight to the point.
func TestUserMoveNearWalkGoesDirect(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	navigator := &fakeNavigator{found: true}
	loop.SetNavigator(navigator)
	loop.lastHit = time.Now().Add(-time.Minute)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45600, Y: 50400, Z: -3500,
	})
	loop.tick()

	require.Equal(t, [][3]int32{{45600, 50400, -3500}}, game.walks,
		"the near click must walk directly")
	require.Equal(t, 0, navigator.calls,
		"the near click must not plan a geodata path")
}

// TestUserMoveReplaceDropsThePlannedPath pins the replacement: a new
// move command drops the leg plan of the previous click, the new click
// starts fresh.
func TestUserMoveReplaceDropsThePlannedPath(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	navigator := &fakeNavigator{found: true}
	loop.SetNavigator(navigator)
	loop.lastHit = time.Now().Add(-time.Minute)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45000, Y: 44000, Z: -3500,
	})
	loop.tick()
	require.NotNil(t, loop.userWaypoints)

	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45600, Y: 50400, Z: -3500,
	})
	loop.tick()

	require.Nil(t, loop.userWaypoints,
		"the new move command must drop the old leg plan")
	require.Equal(t, [][3]int32{{45600, 50400, -3500}}, game.walks[len(game.walks)-1:],
		"the new click must walk directly (inside the planning distance)")
}

// TestUserSwapWaitsForServerConfirmation pins the inventory pacing: the
// Mobius packet executor runs every client packet as its own thread pool
// task, so the unequip and the equip of one swap must never share a
// burst - the second command defers until the tracker observed the
// effect of the first (the equipped flag flipped) and retries then, with
// the pair order intact.
func TestUserSwapWaitsForServerConfirmation(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 555, ItemID: 1146, Count: 1, Equipped: true},
		{ObjectID: 556, ItemID: 1147, Count: 1},
	})

	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 555})
	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 556})
	loop.tick()

	require.Equal(t, []int32{555}, game.uses,
		"the first useItem of the burst must go out")
	require.Len(t, loop.userDeferred, 1,
		"the second useItem must defer behind the unconfirmed one")
	require.Equal(t, int32(556), loop.userDeferred[0].ObjectID)

	// The server applies the first request: the paperdoll update flips
	// the equipped flag of the item, the gate opens for the second.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 555, ItemID: 1146, Count: 1, Equipped: false, Change: 2},
	})
	loop.tick()
	require.Equal(t, []int32{555, 556}, game.uses,
		"the deferred useItem must send right after the confirmation")
	require.Empty(t, loop.userDeferred)
}

// TestUserSwapFallbackTimeout pins the rescue path of the gate: a
// request the server refuses (the inventory never changes) must not
// block the queue forever - after inventoryConfirmTimeout the next
// command fires anyway.
func TestUserSwapFallbackTimeout(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 555, ItemID: 1146, Count: 1, Equipped: true},
		{ObjectID: 556, ItemID: 1147, Count: 1},
	})

	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 555})
	pushCommand(bot, state.Command{Kind: state.CommandUseItem, ObjectID: 556})
	loop.tick()
	require.Len(t, loop.userDeferred, 1,
		"the second useItem defers while the first is unconfirmed")

	// Nothing confirms the request (a refused item), the fallback
	// timeout opens the gate.
	loop.userPendingAt = time.Now().Add(-2 * inventoryConfirmTimeout)
	loop.tick()
	require.Equal(t, []int32{555, 556}, game.uses,
		"the timeout must release the deferred command")
	require.Empty(t, loop.userDeferred)
}

// TestUserMoveRedirectsARunningWalk pins the instant redirect: while the
// character walks toward the old click, a new move command must
// re-issue the walk on the same tick instead of waiting for the old
// server walk to finish.
func TestUserMoveRedirectsARunningWalk(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// The first click: the character walks toward 45600 50400.
	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45600, Y: 50400, Z: -3500,
	})
	loop.tick()
	require.Equal(t, [][3]int32{{45600, 50400, -3500}}, game.walks)

	// The server runs the walk: the character is moving toward the old
	// destination.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45100, Y: 50100, Z: -3500,
		DestX: 45600, DestY: 50400, DestZ: -3500,
	})
	// Without a new command the running walk must not restart.
	loop.tick()
	require.Len(t, game.walks, 1,
		"a running walk toward the manual target must not re-issue")

	// The user clicks somewhere else: the redirect re-issues the walk
	// at once even though the character is still moving.
	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 44800, Y: 49500, Z: -3500,
	})
	loop.tick()
	require.Equal(t, [][3]int32{
		{45600, 50400, -3500}, {44800, 49500, -3500},
	}, game.walks,
		"the new click must redirect the running walk immediately")
	require.False(t, loop.userRedirect,
		"the issued walk must consume the redirect")
}

// TestUserWalkPlanPublishesAndClears pins the walk plan view: a manual
// move publishes the clicked destination, a planned walk publishes the
// remaining waypoints with the destination last, and finishing the walk
// clears the plan again.
func TestUserWalkPlanPublishesAndClears(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// A near click: the plan is just the clicked destination.
	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 45600, Y: 50400, Z: -3500,
	})
	loop.tick()
	require.Equal(t, []state.WalkPoint{
		{X: 45600, Y: 50400, Z: -3500},
	}, bot.Snapshot().WalkPath,
		"the direct walk plan must be the clicked point")

	// The character arrives: the plan clears.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45600, Y: 50400, Z: -3500,
		DestX: 45600, DestY: 50400, DestZ: -3500,
	})
	loop.tick()
	require.Empty(t, bot.Snapshot().WalkPath,
		"the finished walk must clear the plan")

	// A far click plans a geodata path: the plan carries the remaining
	// waypoints with the destination last.
	navigator := &fakeNavigator{found: true}
	loop.SetNavigator(navigator)
	pushCommand(bot, state.Command{
		Kind: state.CommandMove, X: 48000, Y: 45000, Z: -3500,
	})
	loop.tick()
	require.NotNil(t, loop.userWaypoints,
		"the far click must plan a geodata path")
	plan := bot.Snapshot().WalkPath
	require.NotEmpty(t, plan)
	require.Equal(t, state.WalkPoint{X: 48000, Y: 45000, Z: -3500},
		plan[len(plan)-1],
		"the clicked destination must close the plan")

	// Waypoints the character passes drop out of the published plan.
	loop.userWpIndex = len(loop.userWaypoints) - 1
	loop.tick()
	trimmed := bot.Snapshot().WalkPath
	require.Len(t, trimmed, 1,
		"the passed waypoints must leave the plan")
	require.Equal(t, state.WalkPoint{X: 48000, Y: 45000, Z: -3500},
		trimmed[0],
		"the remaining plan must keep the destination last")
}

// TestUserAttackWalksStalledChase pins the chase progress watchdog: a
// target beyond melee range whose server chase stalls (the stuck chase
// packets keep the engagement fresh while the distance never shrinks)
// is approached with the loop's own walk instead of an endless stand.
func TestUserAttackWalksStalledChase(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
		"the first request selects the far target")

	// The target is selected and the chase "runs": the stuck chase
	// packets keep the engagement fresh without closing the distance
	// (the character sits on its stop point, not moving).
	bot.ApplySelfTarget(7)
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 1000,
		X: 45000, Y: 50000, Z: -3500,
		TargetX: 46000, TargetY: 50000, TargetZ: -3500,
	})
	require.True(t, bot.SelfFighting(7))
	require.False(t, bot.SelfWalking(),
		"the stuck chase does not move the character")

	// One tick takes the first chase sample, then the progress window
	// passes without the distance shrinking.
	loop.tick()
	require.False(t, loop.userDistAt.IsZero(),
		"the first sample must record the chase distance")
	deadline := time.Now().Add(6 * time.Second)
	for time.Since(loop.userDistAt) < chaseProgressWindow &&
		time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	loop.userMoveAt = time.Now().Add(-2 * selectPeriod)
	loop.tick()

	require.Empty(t, game.forces[1:],
		"the stalled chase must not re-request attacks")
	require.Equal(t, [][3]int32{{46000, 50000, 0}}, game.walks,
		"the stalled chase must walk toward the target")
}

// TestUserAttackApproachWalksFarTarget pins the approach: a selected
// target beyond melee range is walked to, the forced attack lands once
// the character reaches the swing range.
func TestUserAttackApproachWalksFarTarget(t *testing.T) {
	bot := newTestBot()
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
	loop.tick()
	require.Equal(t, []int32{7}, game.forces,
		"the first request selects the target")

	bot.ApplySelfTarget(7)
	loop.userMoveAt = time.Now().Add(-2 * selectPeriod)
	loop.tick()
	require.Equal(t, [][3]int32{{46000, 50000, 0}}, game.walks,
		"the approach of the selected far target walks")

	// The character arrives next to the target: the forced attack
	// lands.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 45900, Y: 50000, Z: -3500,
		DestX: 45900, DestY: 50000, DestZ: -3500,
	})
	loop.userMoveAt = time.Now().Add(-2 * selectPeriod)
	loop.tick()
	require.Equal(t, []int32{7, 7}, game.forces,
		"the melee range attack must force the swings")
}
