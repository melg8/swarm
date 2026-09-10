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
	clicks    []int32
	clears    int
	lessons   [][2]int32
	casts     []int32
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

func (f *fakeGame) ClickObject(objectID int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.clicks = append(f.clicks, objectID)

	return nil
}

func (f *fakeGame) ClearTarget() error {
	if f.lastError != nil {
		return f.lastError
	}
	f.clears++

	return nil
}

func (f *fakeGame) AcquireSkill(skillID int32, level int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.lessons = append(f.lessons, [2]int32{skillID, level})

	return nil
}

func (f *fakeGame) UseMagicSkill(skillID int32) error {
	if f.lastError != nil {
		return f.lastError
	}
	f.casts = append(f.casts, skillID)

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
	// The server lists the skills of the character on the enter world
	// (Lucky is auto granted at the creation): the town trip trigger
	// holds its start until the list arrived.
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 194, Level: 1, Passive: true},
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

// mobHitsCharacter models the attack broadcast of the mob: the blow
// lands on the character, so the tracker holds the mob as a live
// attacker (its target points at the character, the under attack
// window refreshes).
func mobHitsCharacter(bot *state.Bot) {
	bot.ApplyAttack(state.Attack{
		AttackerID:  7,
		X:           45600,
		Y:           50000,
		Z:           -3500,
		TargetX:     45000,
		TargetY:     50000,
		TargetZ:     -3500,
		TargetIDs:   [state.AttackTargets]int32{100},
		TargetCount: 1,
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
	// The zone deck height the fake navigator resolves: the zone
	// center sits on the same ground the character walks (-3539), so
	// the 3D waypoint arrival of the follower accepts the arrival.
	nav := &fakeNavigator{found: true, height: -3539}
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

// TestLoopStandsUpBeforeTheZoneReturnWalk pins the sit gate of the
// zone return: a resting character outside the square (the user
// switched the hunting zone under a regenerating bot) stands up
// first and waits out the server side stand animation before any
// walk or path search - the server refuses every move request of a
// sitting character with ActionFailed, so the old code spammed the
// refusals while the character stayed glued to the spot.
func TestLoopStandsUpBeforeTheZoneReturnWalk(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	game.noTargets = true
	nav := &fakeNavigator{found: true, height: -3500}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The character rests (the regeneration sat it down) far outside
	// the hunting square.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Zero(t, game.walks,
		"no walk request while the character sits")
	require.Zero(t, nav.calls,
		"no path search while the character sits")
	require.Equal(t, 1, game.sits,
		"the zone return stands the character up")

	// The ChangeWaitType broadcast confirms the standing, the stand
	// animation window settles: the return plans and walks.
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: false})
	loop.restActionAt = time.Now().Add(-standSettlePeriod - time.Second)
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, nav.calls,
		"the return searches the path once the character stands")
	require.Equal(t, phaseTownReturn, loop.phase)

	// The follower walks the planned waypoints.
	loop.tick()
	require.NotEmpty(t, game.walks, "the follower walks home")
	require.Equal(t, 1, game.sits, "no double toggle")
}

// TestLoopResolvesTheZoneReturnDeckHeight pins the zone return goal:
// the destination carries the REAL geodata height of the zone center
// (resolved the way the server resolves a destination), not the
// fabricated self height - the floating city deck against a ground
// zone put the 3D approach goal mid air before, the search aborted
// at the expansion cap and the return fell back to the direct walk.
func TestLoopResolvesTheZoneReturnDeckHeight(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	game.noTargets = true
	nav := &fakeNavigator{found: true, height: -3664}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The character stands on the city deck (-2992) outside the zone
	// whose center sits on the ground (-3664).
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -2992,
		DestX: 49308, DestY: 44213, DestZ: -2992,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, nav.calls)
	require.InDelta(t, -3664, nav.approachEnds[0].Z, 0.001,
		"the zone return goal carries the resolved deck height")
	require.InDelta(t, 46112, nav.approachEnds[0].X, 0.001)
	require.InDelta(t, 41500, nav.approachEnds[0].Y, 0.001)
}

// TestLoopKeepsSelfHeightWhenTheZoneDeckLookupFails: a height lookup
// error (no geodata under the zone center) keeps the self height -
// the same-deck case the old code answered correctly.
func TestLoopKeepsSelfHeightWhenTheZoneDeckLookupFails(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	game.noTargets = true
	nav := &fakeNavigator{found: true, heightErr: true}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.lastHit = time.Now().Add(-time.Minute)

	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, nav.calls)
	require.InDelta(t, -3539, nav.approachEnds[0].Z, 0.001,
		"the failed lookup falls back to the self height")
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
	// health alone triggers the escape). The mob fights back: the
	// escape runs from the blows it lands.
	loop.target = 7
	bot.ApplySelfTarget(7)
	mobHitsCharacter(bot)
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
	// threshold, the escape opens early. The mob fights back, so the
	// escape direction measures against its blows.
	loop.target = 7
	bot.ApplySelfTarget(7)
	bot.ApplyStatusUpdate(7, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
		{ID: state.AttrMaxHP, Value: 100},
	})
	mobHitsCharacter(bot)
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
		panicLogoutPause-500*time.Millisecond,
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
	require.Greater(t, bot.LoginCooldownRemaining(), time.Second,
		"the login cooldown outlives the logout round trip")
	require.LessOrEqual(t, bot.LoginCooldownRemaining(), 2*time.Second,
		"the login cooldown is two seconds - the aggro resets on "+
			"the disappearance, a long pause would only idle the farm")
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

func TestLoopRunsFromThePileUpBeforeLoggingOut(t *testing.T) {
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
	// the character is still healthy. The pack only grows, so the
	// session will end - but not on the spot: the logout waits
	// until the run opened the escape distance from the aggro
	// point, so the relogin lands outside the pack's aggro range.
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

	require.Zero(t, game.logouts,
		"the pile up does not log the character out on the spot")
	require.Len(t, game.walks, 1,
		"the pile up run starts with an escape leg")
	require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[0],
		"the leg runs away from the mob pack")
	require.False(t, loop.panicAt.IsZero(), "the aggro anchor is armed")
	require.Equal(t, int32(45000), loop.panicX)
	require.Equal(t, int32(50000), loop.panicY)

	// The pacing holds the legs to one per second: an immediate
	// re-tick neither walks again nor logs out.
	loop.tick()
	require.Len(t, game.walks, 1)
	require.Zero(t, game.logouts)

	// The character covers the first leg (700 units west, past
	// the 600 unit escape distance): the logout fires there.
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 44300, Y: 50000, Z: -3500,
		DestX: 44300, DestY: 50000, DestZ: -3500,
	})
	loop.tick()
	require.Equal(t, 1, game.logouts,
		"the escape distance made, the session ends")
	require.Len(t, game.walks, 2,
		"the logout keeps the offline character moving")
	require.LessOrEqual(t, bot.LoginCooldownRemaining(), 2*time.Second,
		"the reconnect pause is two seconds")

	// The request stays one shot while the session unwinds.
	loop.tick()
	require.Equal(t, 1, game.logouts)
	require.Len(t, game.walks, 2)
}

func TestLoopLogsOutWhenThePileUpRunNeverMakesDistance(t *testing.T) {
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

	// A cornered run: the pack holds the character in place
	// (the escape legs keep failing, the character never moves).
	// The budget ends the session wherever it got to instead of
	// running forever.
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
	require.Zero(t, game.logouts, "the fresh run gets its budget")

	// The run ages past the whole flee budget without the
	// character moving a unit: the logout fires anyway.
	loop.fleeAt = time.Now().Add(-2 * time.Second)
	loop.panicAt = time.Now().Add(-fleeLogoutAfter - time.Second)
	loop.tick()
	require.Equal(t, 1, game.logouts,
		"a cornered pile up run still ends the session")
	require.True(t, loop.logoutDone)
	require.LessOrEqual(t, bot.LoginCooldownRemaining(), 2*time.Second)
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

// TestLoopRestsAtTheKillSpot pins the post kill rest: the fight is
// over (the target is dead, its last blow still fresh in the under
// attack window), a passive bystander stands nearby but never
// attacked. The hurt character must sit down where the kill happened
// - the old code armed the escape against the nearest living mob (the
// bystander) and ran several hundred units away before resting.
func TestLoopRestsAtTheKillSpot(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45200, Y: 50000, Name: "Dying Gremlin",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45600, Y: 50000, Name: "Bystander Gremlin",
	})
	// The dying mob lands its last blow and dies: the under attack
	// window is fresh while no living mob holds the character as its
	// target.
	bot.ApplyAttack(state.Attack{
		AttackerID: 8, X: 45200, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	bot.ApplyStatusUpdate(8, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 20},
	})
	loop.target = 8

	// The kill registers, the loot phase finds nothing and the engage
	// runs into the hurt gate: no escape run from the bystander.
	for range 4 {
		loop.lastHit = time.Now().Add(-2 * time.Second)
		loop.tick()
	}
	require.Empty(t, game.walks,
		"no escape run from a passive bystander after the kill")
	require.Empty(t, game.forces, "no new engage while hurt")

	// The under attack window (3 s) expires: the character sits down
	// right at the kill spot.
	time.Sleep(3*time.Second + 300*time.Millisecond)
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, 1, game.sits, "the character rests at the kill spot")
	require.Empty(t, game.walks, "the rest happens without running away")
}

// TestLoopFinishesTheFightOutsideTheZone pins the zone leash
// exception: the chase dragged the character out of the square
// mid-fight, the mob it fights stands outside too - the fight
// continues (the kill is finished) instead of dropping the target and
// walking home through the blows.
func TestLoopFinishesTheFightOutsideTheZone(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 49400, Y: 44213, Name: "Chasing Gremlin",
	})
	loop.target = 8
	bot.ApplySelfTarget(8)
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{8}, game.forces,
		"the fight that crossed the zone line continues")
	require.Empty(t, game.walks, "no leash walk while the fight runs")
}

// TestLoopFightsBackOutsideTheZone pins the chaser adoption: the
// character stands outside the square (the escape legs carried it
// out) with healthy health and a mob keeps attacking it - fighting
// back beats the walk home through the blows. A hurt character under
// attack keeps fleeing instead (pinned by the escape tests above).
func TestLoopFightsBackOutsideTheZone(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 49308, Y: 44213, Z: -3539,
		DestX: 49308, DestY: 44213, DestZ: -3539,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 49400, Y: 44213, Name: "Chasing Gremlin",
	})
	bot.ApplyAttack(state.Attack{
		AttackerID: 8, X: 49400, Y: 44213, Z: -3539,
		TargetX: 49308, TargetY: 44213, TargetZ: -3539,
		TargetIDs: [state.AttackTargets]int32{100}, TargetCount: 1,
	})
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{8}, game.forces,
		"the chaser outside the zone is fought back")
	require.Empty(t, game.walks, "no leash walk through the blows")
}

// TestLoopPicksUpLootOutsideTheZone pins the zone free loot search:
// the kill scattered its drop past the square line (the character
// stands outside too) - the pickup happens regardless of the zone
// border. The old zone filter silently left the drop on the ground.
func TestLoopPicksUpLootOutsideTheZone(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(46112, 41500, 450)
	loop.phase = phaseLoot
	bot.ApplyMovement(state.Movement{
		ObjectID: 100, X: 46112, Y: 42100, Z: -3500,
		DestX: 46112, DestY: 42100, DestZ: -3500,
	})
	bot.ApplySpawnItem(state.ItemInfo{
		ObjectID: 9, TemplateID: 57, X: 46130, Y: 42110, Z: -3500,
	})
	loop.tick()
	require.Equal(t, []int32{9}, game.pickups,
		"the drop outside the zone is picked up")
	require.Equal(t, phaseLoot, loop.phase,
		"the loot phase continues past the zone line")
}

func TestEngageIgnoresTheTalkedVillagerSelection(t *testing.T) {
	bot := newTestBot()
	// A hunting mob in reach and the teacher the last trip talked to:
	// the server side selection still points at the villager (the
	// server never clears it, only the next selection replaces it).
	spawnMob(bot)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 55, TemplateID: 1007155, Attackable: false,
		X: 45725, Y: 52105, Name: "Ellenia",
	})
	bot.ApplySelfTarget(55)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The engage must not adopt the friendly villager: the forced
	// attack requests on it only burn the stuck timeout, the fresh
	// mob pick replaces the stale selection on the server instead.
	loop.tick()
	require.NotContains(t, game.forces, int32(55),
		"the villager selection is never attacked")
	require.Equal(t, []int32{7}, game.forces,
		"the fresh mob pick replaces the villager selection")
	require.Equal(t, int32(7), loop.target)
}

func TestEngageFightsTheAttackerOverAFreshMob(t *testing.T) {
	bot := newTestBot()
	// A fresh gremlin nearer than the attacking orc archer: the
	// attacker holds the character as its target, the answer is the
	// fight with IT - walking to the fresh mob would drag the chase
	// into a second opponent.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45300, Y: 50000, Name: "Gremlin",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000003, Attackable: true,
		X: 45800, Y: 50000, Name: "Orc",
	})
	mobHitsCharacter(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Equal(t, int32(7), loop.target,
		"the attacker is the target, not the nearer fresh mob")
	require.Equal(t, []int32{7}, game.forces,
		"the forced attack request answers the attacker at once")
}

func TestEngageFleesTheUnwinnableAttacker(t *testing.T) {
	bot := newTestBot()
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrLevel, Value: 3},
	})
	// The level 8 orc archer holds the level 3 character as its
	// target: the fight is not winnable (five levels above the two
	// level slack), the defense flow answers - the escape walk away
	// from the mob.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000006, Attackable: true,
		X: 45600, Y: 50000, Name: "Orc Archer",
	})
	mobHitsCharacter(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.Zero(t, loop.target, "no fight with an unwinnable attacker")
	require.Empty(t, game.forces)
	require.Len(t, game.walks, 1, "the standard escape leg runs")
	require.Equal(t, [3]int32{44300, 50000, -3500}, game.walks[0])
	require.False(t, loop.fleeSince.IsZero(),
		"the flee episode is armed with its logout budget")
}
