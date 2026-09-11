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

// The reproduction of the 2026-09-11 11:34 state dump report: the
// relogged character test2 stood at x 44296 y 51480 z -2848 (the
// elven village south terrace) in the townReturn phase, the plan held
// the 11 waypoints to the Kaboo Orc Grunt S zone (the first one 22
// units out at 44280 51464 -2832, up the 16 unit terrace step), and
// the character never moved a cell - "town walk stuck, re-pathing"
// burned the whole re-path budget twice (two complete trip cycles)
// while the follower kept clicking the same short waypoint and the
// server kept silently canceling the move.
//
// The root cause (probed against the live stack): the plan itself is
// valid - every leg of it validates against the server click port and
// the local stack walks the exact dump scenario in one go (proven with
// the MOVEDBG diagnostics build). The freeze sits in the interaction
// of the short first click with the server's own move machinery: a
// collapsed click only reaches the server side pathfinder when the
// ORIGINAL line was longer than 30 units (Creature.moveToLocation gates
// the findPath branch on (originalDistance - distance) > 30), so the
// 22 unit click the user's server collapsed was silently canceled -
// ActionFailed, no movement, no feedback the offline validation could
// see. The follower kept re-clicking the same un-rescuable target.
const (
	// reproRound57X/Y/Z is the reported stuck position (the dump of
	// 2026-09-11 11:34, the relogin position of test2).
	reproRound57X = int32(44296)
	reproRound57Y = int32(51480)
	reproRound57Z = int32(-2848)
	// reproRound57ZoneX/Y/Z is the hunting zone center of the report
	// (the Kaboo Orc Grunt S anchoring spot of the level 11 hunter).
	reproRound57ZoneX = int32(42278)
	reproRound57ZoneY = int32(56761)
	reproRound57ZoneZ = int32(-3672)
)

// shortClickFreezeServer simulates the user's server of the report:
// the geodata correction collapses the short clicks (a line under the
// findPath rescue threshold is canceled silently - ActionFailed, the
// character never moves) while the longer clicks walk their validated
// destination, and a correction that shortens a longer line past the
// threshold hands the click to the server side pathfinder (the
// character walks the planned first leg instead of the collapsed
// prefix). This is the observed behavior split of the 11:34 dump: the
// 22 unit waypoint click froze through two whole trip cycles while
// the very same cells walked under every longer click of the plan.
type shortClickFreezeServer struct {
	engine   *pathfind.Engine
	requests int
	refused  int
}

// consume takes the newest walk request of the fake game, validates
// it the way the reported server did and applies the destination the
// server would walk to.
func (s *shortClickFreezeServer) consume(game *fakeGame, bot *state.Bot) {
	if len(game.walks) <= s.requests {
		return
	}
	s.requests = len(game.walks)
	target := game.walks[len(game.walks)-1]
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	if !ok {
		return
	}
	from := pathfind.Vec3{
		X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
	}
	to := pathfind.Vec3{
		X: float64(target[0]), Y: float64(target[1]), Z: float64(target[2]),
	}
	full := math.Hypot(to.X-from.X, to.Y-from.Y)
	if full < 31 {
		// Under the findPath rescue threshold: the collapsed click is
		// canceled silently, the character never moves.
		s.refused++

		return
	}
	validated, ok := s.engine.ValidateClick(from, to)
	if !ok {
		s.refused++

		return
	}
	cut := math.Hypot(validated.X-from.X, validated.Y-from.Y)
	if full-cut > 30 {
		// The correction shortened the line past the threshold: the
		// server side pathfinder walks its own route to the original
		// target (the sim collapses the walk time into the first leg).
		if res, err := s.engine.FindPath(
			from, to, pathfind.DefaultMaxPassableHeight); err == nil &&
			res != nil && res.Found && len(res.Waypoints) > 1 {
			wp := res.Waypoints[1]
			bot.ApplyMovement(state.Movement{
				ObjectID: 100,
				X:        int32(wp.X), Y: int32(wp.Y), Z: int32(wp.Z),
				DestX: int32(wp.X), DestY: int32(wp.Y), DestZ: int32(wp.Z),
			})

			return
		}
	}
	if cut < 1 {
		s.refused++

		return
	}
	bot.ApplyMovement(state.Movement{
		ObjectID: 100,
		X:        int32(validated.X), Y: int32(validated.Y), Z: int32(validated.Z),
		DestX: int32(validated.X), DestY: int32(validated.Y),
		DestZ: int32(validated.Z),
	})
}

// TestReproRound57ShortClickFreezeWalksThePlan replays the exact dump
// scenario against the freeze server model: the first short waypoint
// click is canceled (the dump's freeze), the stuck re-path arms the
// short click extension and the follow-up clicks ride over the rescue
// threshold - the walk must arrive at the zone within a single
// recovery re-path, with every post-stuck click at least the floor
// length.
func TestReproRound57ShortClickFreezeWalksThePlan(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproRound57X, reproRound57Y, reproRound57Z)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	sim := &shortClickFreezeServer{engine: engine}

	// The zone return leg: the destination the returnToZone flow
	// would plan (the zone center on its real deck height).
	deckZ, err := engine.ClosestHeight(
		float64(reproRound57ZoneX), float64(reproRound57ZoneY),
		int16(reproRound57ZoneZ))
	require.NoError(t, err, "the zone deck height must resolve")
	dest := pathfind.Vec3{
		X: float64(reproRound57ZoneX),
		Y: float64(reproRound57ZoneY),
		Z: float64(deckZ),
	}
	require.True(t, loop.startZoneReturnLeg(dest),
		"the zone return must plan a dry geodata route")
	require.Len(t, loop.waypoints, 11,
		"the plan must be the dump's 11 waypoint route")
	require.Equal(t, pathfind.Vec3{
		X: 44280, Y: 51464, Z: -2832,
	}, loop.waypoints[1], "the dump's first waypoint pins the scenario")

	// Walk the plan under the freeze server: the synthetic clock
	// advances past every stuck window, the pacing gate is bypassed
	// between the ticks to keep the test fast. The click lengths after
	// the first stuck are watched for the rescue floor.
	now := time.Now()
	stuckClicks := 0
	arrived := false
	for i := 0; i < 400 && !arrived; i++ {
		now = now.Add(3 * time.Second)
		loop.moveAt = time.Time{}
		selfX, selfY, selfZ, ok := bot.SelfPosition()
		require.True(t, ok, "the character position must be known")
		clicksBefore := len(game.walks)
		done := loop.followWaypoints(selfX, selfY, selfZ, now, true)
		sim.consume(game, bot)
		if loop.extendArmed && len(game.walks) > clicksBefore {
			// Every click of the armed recovery rides over the rescue
			// threshold (the short waypoint clicks re-aim at the
			// forward route samples).
			for _, w := range game.walks[clicksBefore:] {
				stuckClicks++
				length := math.Hypot(
					float64(w[0])-float64(selfX), float64(w[1])-float64(selfY))
				require.GreaterOrEqual(t, length, 31.0,
					"every armed recovery click must clear the server "+
						"rescue threshold (was %.0f units to %d %d)",
					length, w[0], w[1])
			}
		}
		if done {
			arrived = true
		}
	}
	require.True(t, arrived, "the walk must finish within the loop budget")
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	dist := math.Hypot(
		float64(selfX-reproRound57ZoneX), float64(selfY-reproRound57ZoneY))
	require.LessOrEqual(t, dist, tripApproachRadius,
		"the walk must arrive within the approach radius of the zone")
	require.Positive(t, stuckClicks,
		"the armed recovery must have sent the extended clicks")
	require.Equal(t, 1, loop.rePaths,
		"exactly one recovery re-path: the dump burned three and "+
			"restarted the whole trip")
	require.Positive(t, sim.refused,
		"the freeze is modeled: the plain short waypoint clicks of the "+
			"stuck window are canceled without movement")
}

// round57VillageStarts are the village positions the zone return
// sweeps from: the dump standing cell, the trainer hall approaches of
// the round 56 report and the shop quarter spots.
var round57VillageStarts = []struct {
	name string
	x, y int32
	z    int32
}{
	{"the dump terrace cell", 44296, 51480, -2848},
	{"the aisle entrance", 44728, 51992, -2792},
	{"the trap pocket", 44776, 51992, -2808},
	{"the north terrace bend", 44568, 51528, -2808},
	{"the south approach", 44568, 52536, -2832},
	{"the east plaza", 46104, 51528, -2808},
	{"the shop deck", 44872, 47160, -2992},
	{"the southwest shore path", 43752, 48024, -2992},
}

// TestRound57ZoneReturnFromEveryVillageStart sweeps the zone return
// from every village position under the freeze server model (the
// short clicks canceled, the long clicks walked or rescued): the walk
// must arrive within the approach radius from every start - the short
// click extension recovers the collapsed first waypoints wherever the
// planner produces them.
func TestRound57ZoneReturnFromEveryVillageStart(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	deckZ, err := engine.ClosestHeight(
		float64(reproRound57ZoneX), float64(reproRound57ZoneY),
		int16(reproRound57ZoneZ))
	require.NoError(t, err, "the zone deck height must resolve")
	dest := pathfind.Vec3{
		X: float64(reproRound57ZoneX),
		Y: float64(reproRound57ZoneY),
		Z: float64(deckZ),
	}

	for _, start := range round57VillageStarts {
		t.Run(start.name, func(t *testing.T) {
			bot := newTestBot()
			moveSelfTo(bot, start.x, start.y, start.z)
			game := &fakeGame{}
			loop := NewLoop(game, bot)
			loop.SetNavigator(nav)
			loop.lastHit = time.Now().Add(-time.Minute)
			sim := &shortClickFreezeServer{engine: engine}

			require.True(t, loop.startZoneReturnLeg(dest),
				"the zone return must plan from this start")
			now := time.Now()
			arrived := false
			for i := 0; i < 400 && !arrived; i++ {
				now = now.Add(3 * time.Second)
				loop.moveAt = time.Time{}
				selfX, selfY, selfZ, ok := bot.SelfPosition()
				require.True(t, ok)
				done := loop.followWaypoints(selfX, selfY, selfZ, now, true)
				sim.consume(game, bot)
				if done {
					arrived = true
				}
			}
			require.True(t, arrived,
				"the zone return must arrive from this start")
			selfX, selfY, _, ok := bot.SelfPosition()
			require.True(t, ok)
			dist := math.Hypot(
				float64(selfX-reproRound57ZoneX),
				float64(selfY-reproRound57ZoneY))
			require.LessOrEqual(t, dist, tripApproachRadius,
				"the walk must end within the zone approach radius")
			require.LessOrEqual(t, loop.rePaths, 2,
				"the recovery stays within a couple of re-paths")
		})
	}
}

// TestReproRound57FrozenServerEscalatesFast pins the escalation: a
// server that moves nothing at all (the total freeze) aborts the trip
// after ONE no-movement re-path - two stuck windows instead of the
// dump's four per trip times the trip restart cycle - and the zone
// return escalates straight to the direct server routed legs.
func TestReproRound57FrozenServerEscalatesFast(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproRound57X, reproRound57Y, reproRound57Z)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)

	dest := pathfind.Vec3{
		X: float64(reproRound57ZoneX),
		Y: float64(reproRound57ZoneY),
		Z: float64(reproRound57ZoneZ),
	}
	require.True(t, loop.startZoneReturnLeg(dest))
	loop.phase = phaseTownReturn

	// The frozen aisle server: every click validates against the
	// ported rules but the character never moves (the total freeze of
	// the dump - whatever the user's server held against this
	// character, no click of the plan moved it).
	sim := &villageClickServer{engine: engine, frozen: true}
	now := time.Now()
	aborted := false
	for i := 0; i < 200 && !aborted; i++ {
		now = now.Add(5 * time.Second)
		loop.moveAt = time.Time{}
		selfX, selfY, selfZ, ok := bot.SelfPosition()
		require.True(t, ok)
		done := loop.followWaypoints(selfX, selfY, selfZ, now, true)
		sim.consume(game, bot)
		if done || loop.phase != phaseTownReturn {
			aborted = true
		}
	}
	require.True(t, aborted, "the frozen walk must end the trip")
	require.Equal(t, phaseEngage, loop.phase, "the trip aborts back to the hunt")
	require.LessOrEqual(t, loop.rePaths, frozenRepathLimit,
		"the frozen re-path aborts after one no-movement re-path")
	require.GreaterOrEqual(t, loop.zoneFails, zoneReturnFailBudget,
		"the frozen zone return escalates to the direct legs at once")
}
