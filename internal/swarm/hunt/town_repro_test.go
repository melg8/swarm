// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The reproduction of the original stuck report (development_log round
// 35): the character stood at x 45544 y 45880 z -2992 - the pond deck
// edge west of the elven village shop quarter - while a town trip
// tried to reach the trader Unoren (44667 46896 -2982). The old code
// sent the straight far walk and relied on the server routing, which
// stops at the deck edge: the straight line to the shop crosses the
// water gap, and the server move validation (GeoEngine.getValidLocation)
// ends the move at the last walkable cell. These tests reproduce the
// scenario with the exact reported positions against the real geodata
// pack: the first one pins the stall mechanism itself, the second one
// walks the full town trip - the planner routes the character around
// the pond over the village deck and the sale runs at the merchant -
// with zero stuck re-paths.
const (
	// reproStuckX/Y/Z is the user reported stuck position (round 35).
	reproStuckX = int32(45544)
	reproStuckY = int32(45880)
	reproStuckZ = int32(-2992)
	// reproUnorenX/Y/Z is the spawn of the trader Unoren, the shop
	// goal of the reported trip (ElvenVillageNPCs.xml).
	reproUnorenX = int32(44667)
	reproUnorenY = int32(46896)
	reproUnorenZ = int32(-2982)
	// reproUnorenID is the object id the reproduction gives the
	// merchant npc (the NpcInfo object id, not the template id).
	reproUnorenID = int32(60)
	// reproWaterSurface is the C1 water surface height (the maxZ of
	// the water zones, see round 35): a walk below it is a swim.
	reproWaterSurface = -3780.0
	// reproSimStep is the sim stride along a walk leg (one geodata
	// cell is 16 units; the sim advances the straight line cell by
	// cell like the server move validation does).
	reproSimStep = 16.0
	// reproSimSlice is the distance one sim advance covers: roughly
	// two run seconds of a level 1 elven fighter (speed 125), so the
	// walk completes in test-real seconds, not minutes.
	reproSimSlice = 250.0
)

// reproGeodataCandidates mirrors the geodata detection of the bot mode
// of cmd/swarm: the in-repository pack first (go test runs with the
// package directory as CWD, so the search walks up to the repository
// root where data/geodata lives), then the reference Windows deployment
// of this project.
func reproGeodataCandidates() []string {
	candidates := make([]string, 0, 8)
	if dir, err := os.Getwd(); err == nil {
		for range 6 {
			candidates = append(candidates, filepath.Join(dir, "data", "geodata"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	candidates = append(candidates, filepath.Join("E:\\", "work",
		"lineage_workspace_fresh", "L2J_Mobius_C1_HarbingersOfWar",
		"game", "data", "geodata"))

	return candidates
}

// reproEngine builds a geodata engine over the real pack when it is
// available on the machine and skips the test otherwise: the
// reproduction needs the actual elven village terrain - the deck edge,
// the pond and the shop quarter only exist in the deployed pack.
func reproEngine(t *testing.T) *pathfind.Engine {
	t.Helper()
	for _, candidate := range reproGeodataCandidates() {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			engine := pathfind.NewEngine(candidate)
			require.True(t, engine.Stats().HasData,
				"the geodata directory %s must contain region files",
				candidate)

			return engine
		}
	}
	t.Skip("no local geodata pack, the stuck spot reproduction needs it")
	t.Helper()

	return nil
}

// reproServer simulates the movement semantics the Mobius server
// applies to a MoveToLocation: the character follows the straight
// line to the requested target cell by cell, and the walk stops at
// the last walkable cell when the surface breaks - a climb beyond the
// passable height or a closed wall (the same terrace rule the server
// GeoEngine.getValidLocation applies, modeled conservatively here by
// the geodata line of sight between the step cells: the strict
// symmetric rule never lets the sim walk off a surface the server
// routing itself would refuse to follow straight). The sim tracks the
// deepest z the character ever stood on, so the tests can assert the
// route never swims.
type reproServer struct {
	nav       *pathfind.Engine
	requests  int
	target    [3]int32
	minZ      int32
	stalled   bool
	stalledAt [3]int32
}

// consume takes the newest walk request of the fake game as the
// active move of the simulated server.
func (s *reproServer) consume(game *fakeGame) {
	if len(game.walks) > s.requests {
		s.requests = len(game.walks)
		s.target = game.walks[len(game.walks)-1]
		s.stalled = false
	}
}

// advance walks one sim slice toward the active request and applies
// the resulting character position to the tracker (a zero distance
// movement broadcast: the server stops the creature at the point).
func (s *reproServer) advance(bot *state.Bot) {
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	if !ok || s.requests == 0 {
		return
	}
	x, y, z := float64(selfX), float64(selfY), float64(selfZ)
	budget := reproSimSlice
	for budget > 0 {
		dx := float64(s.target[0]) - x
		dy := float64(s.target[1]) - y
		dist := math.Hypot(dx, dy)
		if dist <= reproSimStep {
			// Arrival: stand on the requested target cell.
			x, y = float64(s.target[0]), float64(s.target[1])

			break
		}
		step := reproSimStep / dist
		px := x + dx*step
		py := y + dy*step
		pz, err := s.nav.ClosestHeight(px, py, int16(z))
		if err != nil {
			s.stall(x, y, z)

			break
		}
		sight, losErr := s.nav.LineOfSight(
			pathfind.Vec3{X: x, Y: y, Z: z},
			pathfind.Vec3{X: px, Y: py, Z: float64(pz)},
			pathfind.DefaultMaxPassableHeight)
		if losErr != nil || !sight {
			s.stall(x, y, z)

			break
		}
		x, y, z = px, py, float64(pz)
		budget -= reproSimStep
	}
	if z < float64(s.minZ) {
		s.minZ = int32(z)
	}
	bot.ApplyMovement(state.Movement{
		ObjectID: 100,
		X:        int32(x), Y: int32(y), Z: int32(z),
		DestX: int32(x), DestY: int32(y), DestZ: int32(z),
	})
}

// stall records a position the server walk could not leave.
func (s *reproServer) stall(x, y, z float64) {
	s.stalled = true
	s.stalledAt = [3]int32{int32(x), int32(y), int32(z)}
}

// request programs a move request directly (the original problem's
// straight far walk).
func (s *reproServer) request(x, y, z int32) {
	s.requests++
	s.target = [3]int32{x, y, z}
	s.stalled = false
}

// dist3DToUnoren measures the full 3D distance to the merchant spawn.
func dist3DToUnoren(x, y, z int32) float64 {
	dx := float64(reproUnorenX - x)
	dy := float64(reproUnorenY - y)
	dz := float64(reproUnorenZ - z)

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// TestReproStraightWalkStallsAtTheUserStuckSpot reproduces the
// mechanism of the original report: the direct straight walk from the
// stuck position toward the trader Unoren cannot cross the water gap
// north of the shop quarter. The simulated server walk stops at the
// pond deck edge, the character stays on the deck (never swims down
// into the pond) and never comes within the interaction distance of
// the merchant - the exact stall the user observed. This is the
// contract the geodata planning of the town trips exists to satisfy:
// a straight MoveToLocation is not a route.
func TestReproStraightWalkStallsAtTheUserStuckSpot(t *testing.T) {
	engine := reproEngine(t)
	bot := newTestBot()
	moveSelfTo(bot, reproStuckX, reproStuckY, reproStuckZ)
	sim := &reproServer{nav: engine, minZ: reproStuckZ}

	// The old code sent exactly this: one far walk straight at the
	// merchant spawn.
	sim.request(reproUnorenX, reproUnorenY, reproUnorenZ)
	for range 40 {
		sim.advance(bot)
	}
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	require.True(t, ok)
	t.Logf("the straight walk stalled at %d %d %d (dist to Unoren %.0f)",
		selfX, selfY, selfZ, dist3DToUnoren(selfX, selfY, selfZ))

	// The walk never reached the interaction distance: the reported
	// stall - the character stands at the deck edge, the shop is
	// unreachable through the direct line.
	require.Greater(t, dist3DToUnoren(selfX, selfY, selfZ), 250.0,
		"the straight walk must not reach the merchant interaction distance")
	// The character stayed on the village deck: no drop into the pond.
	require.InDelta(t, reproStuckZ, selfZ, 60.0,
		"the stall sits on the deck level of the stuck report")
	require.Greater(t, float64(sim.minZ), reproWaterSurface,
		"the straight walk must not swim through the pond")
	// The stall is the deck edge of the reported position, not
	// somewhere else along the line.
	require.LessOrEqual(t, math.Hypot(
		float64(selfX-reproStuckX), float64(selfY-reproStuckY)), 300.0,
		"the stall must sit at the reported stuck spot edge")

	// The geodata line of sight agrees with the server model: the
	// direct line is not walkable.
	sight, err := engine.LineOfSight(
		pathfind.Vec3{X: float64(reproStuckX), Y: float64(reproStuckY), Z: float64(reproStuckZ)},
		pathfind.Vec3{X: float64(reproUnorenX), Y: float64(reproUnorenY), Z: float64(reproUnorenZ)},
		pathfind.DefaultMaxPassableHeight)
	require.NoError(t, err)
	require.False(t, sight,
		"the straight line to the shop crosses the water gap")
}

// TestReproTownTripFromTheUserStuckSpot walks the full town trip from
// the reported stuck position against the real geodata pack: the
// inventory is full, the trip starts at the pond deck edge, the
// planner routes around the water over the village deck, the follower
// walks the plan under the simulated server semantics, the walk ends
// inside the interaction distance of the trader Unoren with zero
// stuck re-paths (the original failure signature), and the sale runs
// at the merchant. The walk plan publishes along the way - the same
// view the state dump prints for a live problem report.
func TestReproTownTripFromTheUserStuckSpot(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, reproStuckX, reproStuckY, reproStuckZ)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	fillInventory(bot)
	sim := &reproServer{nav: engine, minZ: reproStuckZ}

	// The trip starts at the stuck spot: the nearest merchant of the
	// position is the trader Unoren of the reported scenario.
	loop.tick()
	require.Equal(t, phaseTownWalk, loop.phase,
		"the trip must start from the full inventory")
	require.Equal(t, "Unoren", loop.tripStops[0].merchant.Name,
		"the trip must target the reported merchant")
	require.Greater(t, len(loop.waypoints), 1,
		"the plan must be a real geodata route, not the direct fallback")
	for _, wp := range loop.waypoints {
		require.Greater(t, wp.Z, reproWaterSurface,
			"the planned route must stay above the water surface")
	}

	// The walk plan publishes for the state dump view: the remaining
	// waypoints of the running trip.
	snap := bot.Snapshot()
	require.GreaterOrEqual(t, len(snap.WalkPath), 2,
		"the walk plan must publish while the trip walks")

	// Walk the plan under the simulated server: the follower paces a
	// walk request every two seconds, the sim advances the character
	// along each requested leg.
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		loop.tick()
		sim.consume(game)
		sim.advance(bot)
		if loop.phase == phaseTownSell {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, phaseTownSell, loop.phase,
		"the walk must reach the shop ring from the stuck spot")
	require.Zero(t, loop.rePaths,
		"the original failure signature: no stuck re-path may fire")
	selfX, selfY, selfZ, ok := bot.SelfPosition()
	require.True(t, ok)
	t.Logf("the trip walked to %d %d %d (dist to Unoren %.0f, min z %d)",
		selfX, selfY, selfZ, dist3DToUnoren(selfX, selfY, selfZ), sim.minZ)
	require.LessOrEqual(t, dist3DToUnoren(selfX, selfY, selfZ), 250.0,
		"the walk must end inside the merchant interaction distance")
	require.Greater(t, float64(sim.minZ), reproWaterSurface,
		"the walk must never swim under the village")

	// The merchant spawns in the known list (the NpcInfo of the shop
	// quarter): the loop selects it like the official client and sells.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID:   reproUnorenID,
		TemplateID: 7147 + npcDisplayOffset,
		X:          reproUnorenX, Y: reproUnorenY, Z: reproUnorenZ,
		Name: "Unoren",
	})
	sellDeadline := time.Now().Add(15 * time.Second)
	for len(game.sells) == 0 && time.Now().Before(sellDeadline) {
		loop.tick()
		for _, forced := range game.forces {
			if forced == reproUnorenID {
				// The MyTargetSelected answer of the server.
				bot.ApplySelfTarget(reproUnorenID)
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	require.NotEmpty(t, game.sells,
		"the sale must run at the reached merchant")
	require.Contains(t, game.forces, reproUnorenID,
		"the merchant selection must have been requested")
	require.Len(t, game.sells[0], sellBatchSize,
		"the first sell batch carries the full batch")
}
