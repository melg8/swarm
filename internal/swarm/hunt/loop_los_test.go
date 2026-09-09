// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// armBlindEngage builds the live-dump stall: the mob is selected at
// melee range, the server answered the attack with "Cannot see
// target." and no swing or chase step ever landed. The engage
// timestamp sits past the detection delay and before the refusal.
func armBlindEngage(t *testing.T, bot *state.Bot, loop *Loop) {
	t.Helper()
	loop.engageAt = time.Now().Add(-3 * time.Second)
	bot.ApplySelfTarget(7)
	bot.ApplySystemMessage(state.SystemMessage{ID: 181})
	loop.target = 7
	loop.lastHit = time.Now().Add(-time.Minute)
}

// TestEngageRepositionsBlindTarget pins level A of the recovery: a
// fresh "Cannot see target." refusal on an engage that never went
// fresh plans the geodata route to a vantage point and walks its
// first leg at once, instead of re-requesting the refused attack (an
// attack request would replace the walk intention server side and
// cancel the recovery).
func TestEngageRepositionsBlindTarget(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	nav := &fakeNavigator{
		found: true, sight: true,
		route: []pathfind.Vec3{
			{X: 45300, Y: 50300, Z: -3500},
			{X: 45600, Y: 50600, Z: -3500},
		},
	}
	loop.SetNavigator(nav)
	armBlindEngage(t, bot, loop)

	loop.tick()

	require.False(t, loop.losAt.IsZero(),
		"the blind recovery must arm")
	require.NotEmpty(t, loop.losWaypoints,
		"the geodata route becomes the reposition waypoints")
	require.Equal(t, 1, loop.losTried, "one planning attempt is spent")
	require.Empty(t, game.forces,
		"no attack request may fire while the reposition runs")
	require.Len(t, game.walks, 1,
		"the first reposition leg walks on the planning tick")
	require.InDelta(t, 45300, game.walks[0][0], 1)
	require.InDelta(t, 50300, game.walks[0][1], 1)
	// The search goal sits on the vantage ring around the mob: the
	// melee radius standing circle at ~90 units from it.
	goal := nav.approachEnds[len(nav.approachEnds)-1]
	ring := dist2D(int32(goal.X), int32(goal.Y), 45115, 50015)
	require.InDelta(t, blindMeleeRadius, ring, 40,
		"the reposition goal is a melee range standing point")
}

// TestEngageBlindRepositionArrivesAndReEngages pins the handoff after
// the walk: reaching the final waypoint clears the recovery state and
// re-anchors the engage clock, so the loop re-requests the attack from
// the new standing point - a persisting block re-arms the recovery
// with the remaining attempt budget instead of hanging.
func TestEngageBlindRepositionArrivesAndReEngages(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	nav := &fakeNavigator{
		found: true, sight: true,
		route: []pathfind.Vec3{{X: 45300, Y: 50300, Z: -3500}},
	}
	loop.SetNavigator(nav)
	armBlindEngage(t, bot, loop)

	loop.tick()
	require.Len(t, game.walks, 1)

	// The walk ran to the waypoint: the character now stands there.
	bot.SetCharacter("test1", 100, 18, 45300, 50300, -3500, 50, 30)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrMaxHP, Value: 100},
		{ID: state.AttrCurHP, Value: 90},
	})

	loop.tick()

	require.True(t, loop.losAt.IsZero(),
		"the arrival clears the reposition walk")
	require.Empty(t, loop.losWaypoints)
	require.False(t, loop.engageAt.IsZero(),
		"the engage clock re-anchors at the new standing point")
	require.Equal(t, 1, loop.losTried,
		"the spent attempt stays counted for the budget")
	require.Empty(t, game.forces,
		"the re-request still waits out the action pacing")

	// The attack is re-requested from the vantage point.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces)
}

// TestEngageSwitchesBlindTargetWithoutGeodata pins level B without a
// navigator: no geodata, no reposition - the obstructed target is
// dropped and skipped so the search picks a different mob at once.
func TestEngageSwitchesBlindTargetWithoutGeodata(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45400, Y: 50400, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	armBlindEngage(t, bot, loop)

	loop.tick()

	require.Zero(t, loop.target, "the obstructed target is dropped")
	require.True(t, loop.targetSkipped(7, time.Now()),
		"the obstructed target is skipped for the search")
	require.True(t, loop.losAt.IsZero(),
		"no reposition walk started without geodata")

	// The next pick takes the other mob: the selection of a different
	// object id also clears the stale server side attack stance.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, int32(8), loop.target)
}

// TestEngageSwitchesBlindTargetWhenNoRoute pins level B when the
// geodata offers no vantage point: no standing point sees the target
// and no route exists - the target is switched instead of a fallback
// walk that would run straight into the same obstacle.
func TestEngageSwitchesBlindTargetWhenNoRoute(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45400, Y: 50400, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: false, sight: false})
	armBlindEngage(t, bot, loop)

	loop.tick()

	require.Zero(t, loop.target, "the unreachable target is dropped")
	require.True(t, loop.targetSkipped(7, time.Now()))
	require.Empty(t, game.walks,
		"no blind fallback walk without a geodata route")
}

// TestEngageBlindWalkBudgetSwitchesTarget pins the walk budget: a
// reposition that never reaches the vantage point (a blocked route, a
// stalled walk) ends in the target switch instead of walking forever.
func TestEngageBlindWalkBudgetSwitchesTarget(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000001, Attackable: true,
		X: 45400, Y: 50400, Name: "Gremlin",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true, sight: true})
	armBlindEngage(t, bot, loop)
	loop.losAt = time.Now().Add(-(blindWalkBudget + time.Second))
	loop.losWaypoints = []pathfind.Vec3{
		{X: 45300, Y: 50300, Z: -3500},
	}
	loop.losTried = 1

	loop.tick()

	require.Zero(t, loop.target, "the stalled reposition ends in a switch")
	require.True(t, loop.targetSkipped(7, time.Now()))
	require.True(t, loop.losAt.IsZero(),
		"the recovery bookkeeping clears with the switch")
}

// TestEngageBlindRecoveryRetriesOnce pins the attempt budget: after a
// completed reposition walk that did not clear the block (the
// vantage point lied, the mob moved), the recovery plans one more
// route before switching the target.
func TestEngageBlindRecoveryRetriesOnce(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true, sight: true})
	armBlindEngage(t, bot, loop)

	// Attempt one: the trivial route (start at the character, the
	// vantage point at the ring) is walked at once - the character
	// stands at the vantage point.
	loop.tick()
	require.Equal(t, 1, loop.losTried)
	vantage := loop.losWaypoints[len(loop.losWaypoints)-1]
	bot.SetCharacter("test1", 100, 18,
		int32(vantage.X), int32(vantage.Y), int32(vantage.Z), 50, 30)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrMaxHP, Value: 100},
		{ID: state.AttrCurHP, Value: 90},
	})

	// The arrival hands the engage back and the attack is re-requested.
	loop.tick()
	require.True(t, loop.losAt.IsZero())
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, []int32{7}, game.forces)

	// The refusal keeps coming after the walk: a fresh answer during
	// the re-anchored attempt arms the second reposition.
	armBlindEngage(t, bot, loop)
	loop.tick()
	require.Equal(t, 2, loop.losTried,
		"the second attempt re-plans the route")

	// Past the attempt budget the next block ends in the switch.
	armBlindEngage(t, bot, loop)
	loop.losAt = time.Time{}
	loop.losWaypoints = nil
	loop.tick()
	require.Zero(t, loop.target,
		"the third block switches the target")
	require.True(t, loop.targetSkipped(7, time.Now()))
}

// TestEngageStuckTimeoutFiresThroughStaleAttackStance pins the root
// cause of the live 53 minute stall: the server armed the attack
// stance without a line of sight check and no swing ever landed, so
// SelfEngaged stays true forever while nothing actually fights. The
// stuck timeout must measure the FRESH fight view and still fire.
func TestEngageStuckTimeoutFiresThroughStaleAttackStance(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	// The chase step armed the stance (FightingTargetID, auto attack
	// flag, one fresh combat mark), the swing behind the column never
	// landed and no AutoAttackStop ever arrives.
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 40,
		X: 45000, Y: 50000, TargetX: 45115, TargetY: 50015, TargetZ: -3500,
	})
	bot.ApplyAutoAttackStart(100)
	bot.ApplySelfTarget(7)
	loop.target = 7
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.engageAt = time.Now().Add(-(engageStuckTimeout + time.Second))

	// The combat mark went stale: three seconds (the state tracker's
	// fightingFreshWindow) without a swing or a chase step while the
	// stance flags linger.
	time.Sleep(3*time.Second + 150*time.Millisecond)
	loop.tick()

	require.Zero(t, loop.target,
		"the stale stance must not hold the stuck timeout")
	require.True(t, loop.targetSkipped(7, time.Now()))
}

// TestEngageStuckTimeoutHeldDuringBlindRecovery pins the timeout hold:
// while the reposition walk owns the engage, the plain stuck timeout
// stays out of the way - the recovery manages its own budgets.
func TestEngageStuckTimeoutHeldDuringBlindRecovery(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true, sight: true})
	bot.ApplySelfTarget(7)
	loop.target = 7
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.engageAt = time.Now().Add(-(engageStuckTimeout + time.Minute))
	loop.losAt = time.Now().Add(-2 * time.Second)
	loop.losWaypoints = []pathfind.Vec3{
		{X: 45300, Y: 50300, Z: -3500},
	}
	loop.losTried = 1

	loop.tick()

	require.Equal(t, int32(7), loop.target,
		"the running recovery holds the stuck timeout")
	require.False(t, loop.losAt.IsZero(),
		"the reposition walk keeps running")
	require.Empty(t, game.forces,
		"no attack request fires while the walk runs")
	require.Len(t, game.walks, 1,
		"the walk keeps moving toward the vantage point")
}

// TestEngageBlindDetectionIgnoresStaleRefusal pins the attempt
// scoping: a refusal that belongs to an earlier engage attempt (it
// arrived before the current attempt started) never arms the recovery
// of the new target.
func TestEngageBlindDetectionIgnoresStaleRefusal(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	// The refusal of the previous target landed, then the character
	// re-engaged: the fresh attempt started after the answer.
	bot.ApplySelfTarget(7)
	bot.ApplySystemMessage(state.SystemMessage{ID: 181})
	time.Sleep(2100 * time.Millisecond)
	loop.target = 7
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.engageAt = time.Now().Add(-2050 * time.Millisecond)

	loop.tick()

	require.True(t, loop.losAt.IsZero(),
		"a stale refusal never arms the recovery")
	require.Empty(t, game.walks)
	require.Equal(t, []int32{7}, game.forces,
		"the engage keeps re-requesting the attack normally")
}

// TestEngageBlindDetectionQuietWhileFighting pins the fresh fight
// guard: while swings or chase steps land (the mob walked past the
// obstacle edge), a leftover refusal line does not interrupt the
// running fight.
func TestEngageBlindDetectionQuietWhileFighting(t *testing.T) {
	bot := newTestBot()
	spawnCloseMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	bot.ApplySystemMessage(state.SystemMessage{ID: 181})
	bot.ApplySelfTarget(7)
	// The fight runs: the chase steps arrive right now.
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 40,
		X: 45000, Y: 50000, TargetX: 45115, TargetY: 50015, TargetZ: -3500,
	})
	loop.target = 7
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.engageAt = time.Now().Add(-time.Minute)

	loop.tick()

	require.True(t, loop.losAt.IsZero(),
		"a running fight never arms the blind recovery")
	require.Empty(t, game.walks)
	require.Empty(t, game.forces,
		"the running fight needs no re-request")
}

// spawnCloseMob adds an attackable npc at melee range of the test
// character (the live dump standoff: 115 units).
func spawnCloseMob(bot *state.Bot) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 45115, Y: 50015, Name: "Gremlin",
	})
}

// dist2D measures the planar distance between two world points.
func dist2D(x1, y1, x2, y2 int32) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)

	return math.Hypot(dx, dy)
}
