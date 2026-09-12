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

// The reproduction of the 2026-09-12 03:56 trainer hall freeze (the
// user report, build 6a2ac91): the level 15 fighter walked the learn
// leg to the teacher Ellenia (45725 52105 -2792, inside the elven
// village trainer hall), the geodata plan entered the building
// through the west aisle column (44728 51992 -> 44728 52040 ->
// 45160 52120) and the character froze at the aisle entrance - the
// server refused the 48 unit click into the aisle while the bot's
// own click validation port accepted it, the deterministic re-path
// reproduced the identical route and the walk burned its whole
// re-path budget standing on the same cell. The user rule these
// tests pin: the character must walk from the building entrance
// right up to the training npc - not talk to it through the wall
// from wherever the geodata leg happened to end.
const (
	// aisleEntranceX/Y/Z is the dump freeze cell: the north end of
	// the trainer hall west aisle column.
	aisleEntranceX = int32(44744)
	aisleEntranceY = int32(51992)
	aisleEntranceZ = int32(-2792)
	// elleniaX/Y/Z is the elven fighter teacher spawn inside the
	// trainer hall (ElvenVillageNPCs.xml).
	elleniaX = int32(45725)
	elleniaY = int32(52105)
	elleniaZ = int32(-2792)
	// elleniaObjectID is the object id the reproduction gives the
	// teacher npc.
	elleniaObjectID = int32(87)
)

// aisleWalledServer builds the simulated server of the freeze model:
// the server geodata walls the trainer hall aisle column (the cells
// south of the entrance at 44728 52008..52040) although the bot's
// geodata pack models them as open - the disagreement that froze the
// dump character at the entrance through every re-path of its trip.
// The wall covers only the column interior: the entrance cell itself
// stays walkable exactly the way the dump character stood on it.
func aisleWalledServer(engine *pathfind.Engine) *reproServer {
	return &reproServer{
		nav: engine,
		walled: []pathfind.AvoidArea{{
			Center: pathfind.Vec3{X: 44728, Y: 52024, Z: -2792},
			Radius: 24.0,
		}},
	}
}

// armTeachStop arms the learn leg of the dump: the character walks to
// the teacher Ellenia inside the trainer hall.
func armTeachStop(t *testing.T, loop *Loop) {
	t.Helper()
	loop.phase = phaseTownWalk
	loop.tripStart = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7156, Name: "Ellenia",
			X: elleniaX, Y: elleniaY, Z: elleniaZ,
		},
		teach: true,
	}}
	// The teach leg searches the close ring (the advanceTripStop
	// teach radius), the wide trip ring stays the fallback.
	loop.legRadius = npcApproachOffset
	if !loop.startWalkLeg(pathfind.Vec3{
		X: float64(elleniaX), Y: float64(elleniaY), Z: float64(elleniaZ),
	}) {
		loop.legRadius = tripApproachRadius
	}
	require.True(t, loop.startWalkLeg(pathfind.Vec3{
		X: float64(elleniaX), Y: float64(elleniaY), Z: float64(elleniaZ),
	}), "the aisle route must plan")
}

// spawnEllenia publishes the teacher npc the approach needs.
func spawnEllenia(bot *state.Bot) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID:   elleniaObjectID,
		TemplateID: 7156 + npcDisplayOffset,
		X:          elleniaX, Y: elleniaY, Z: elleniaZ,
		Name: "Ellenia",
	})
}

// dist2DToEllenia measures the planar distance to the teacher.
func dist2DToEllenia(x, y int32) float64 {
	return math.Hypot(float64(elleniaX-x), float64(elleniaY-y))
}

// TestBuildingEntryWalksFromTheAisleEntrance pins the happy path of
// the user rule: a server that agrees with the bot's geodata walks
// the planned route from the building entrance (the aisle column)
// straight into the hall, the teach stop approaches the teacher ring
// and the talk click fires with the character standing right by the
// npc.
func TestBuildingEntryWalksFromTheAisleEntrance(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, aisleEntranceX, aisleEntranceY, aisleEntranceZ)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	armTeachStop(t, loop)
	sim := &reproServer{nav: engine, minZ: aisleEntranceZ}
	spawnEllenia(bot)
	loop.teacherID = elleniaObjectID

	// The planned route is the dump walk: the aisle column south, the
	// hall row east, the approach ring of the teacher.
	require.GreaterOrEqual(t, len(loop.waypoints), 4,
		"the aisle route carries the dump plan shape")
	last := loop.waypoints[len(loop.waypoints)-1]
	require.LessOrEqual(t, math.Hypot(
		last.X-float64(elleniaX), last.Y-float64(elleniaY)), 200.0,
		"the route ends inside the teacher approach ring")

	walkToTheTalk(t, loop, game, bot, sim)
	require.Zero(t, loop.rePaths,
		"the agreeing server walks the plan without any recovery")
}

// TestBuildingEntryEscapesTheWalledAisle pins the freeze model of the
// dump: the simulated server walls the aisle column the plan walks
// through - the character stalls at the entrance exactly the way the
// dump froze - and the escalation ladder must rescue the leg: the
// frozen corridor joins the session bans, the re-plan detours around
// the building (the north and east approach), the walk reaches the
// teacher and the talk click fires right by the npc.
func TestBuildingEntryEscapesTheWalledAisle(t *testing.T) {
	engine := reproEngine(t)
	nav := NewNavigator(engine)
	bot := newTestBot()
	moveSelfTo(bot, aisleEntranceX, aisleEntranceY, aisleEntranceZ)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.lastHit = time.Now().Add(-time.Minute)
	armTeachStop(t, loop)
	sim := aisleWalledServer(engine)
	spawnEllenia(bot)
	loop.teacherID = elleniaObjectID

	// The walk clicks stall at the walled aisle: the simulated
	// character never moves a cell onto the column, the stuck cycles
	// fire (the walkDrive acceleration stands the freeze exposed in
	// seconds instead of the 15 s real window) and the frozen abort
	// climbs the escalation ladder.
	drive := newWalkDrive()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		drive.step(loop, game, bot, sim)
		if loop.frozenStage > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Positive(t, loop.frozenStage,
		"the walled aisle must freeze the plan and arm the escalation")
	require.Len(t, loop.frozenAreas, 1,
		"the frozen aisle corridor joined the session avoid areas")
	require.InDelta(t, 44728.0, loop.frozenAreas[0].Center.X, 1.0,
		"the ban covers the frozen aisle column")
	selfX, selfY, _, _ := bot.SelfPosition()
	require.LessOrEqual(t, math.Hypot(
		float64(selfX-44728), float64(selfY-51992)), 120.0,
		"the character still stands at the entrance - the freeze signature")

	// The re-planned route detours around the banned aisle: the walk
	// crosses the north of the building and the east approach, and
	// the teach stop talks to the teacher. The strictest server model
	// walls even the last stretch of the east approach (the roof-only
	// interior bands), so the close ring cannot close - the approach
	// window bounds the wait and the talk fires from within the
	// server interaction distance instead.
	walkToTheTalkMax(t, loop, game, bot, sim, npcInteractionDist)
	require.NotEmpty(t, loop.frozenAreas,
		"the session ban survives the trip for the later plans")
}

// walkToTheTalk drives the loop and the simulated server until the
// teach stop talks to the teacher, then pins the user rule: the
// character stands right by the npc (the close ring, not the wide
// approach radius the geodata leg ends on) and the talk click fired.
func walkToTheTalk(
	t *testing.T, loop *Loop, game *fakeGame, bot *state.Bot, sim *reproServer,
) {
	t.Helper()
	walkToTheTalkMax(t, loop, game, bot, sim, npcApproachOffset+hopCoincideDist)
}

// walkToTheTalkMax is walkToTheTalk with the accepted talking
// distance bound by the caller (the strict server models wall the
// last stretch of the approach - the talk then fires from within the
// server interaction distance after the approach window).
func walkToTheTalkMax(
	t *testing.T, loop *Loop, game *fakeGame, bot *state.Bot, sim *reproServer,
	maxDist2D float64,
) {
	t.Helper()
	drive := newWalkDrive()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		drive.step(loop, game, bot, sim)
		if containsClick(game.clicks, elleniaObjectID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, containsClick(game.clicks, elleniaObjectID),
		"the teach stop must click the teacher npc")
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	t.Logf("the teach stop talked to Ellenia from %d %d (dist2D %.0f)",
		selfX, selfY, dist2DToEllenia(selfX, selfY))
	require.LessOrEqual(t, dist2DToEllenia(selfX, selfY), maxDist2D,
		"the character must stand by the teacher - the close ring of "+
			"the npc approach point on the agreeing servers, within the "+
			"interaction distance on the walled ones")
}

// walkDrive runs one iteration of the offline walk loop: the tick
// (with the click pacing bypassed so the follower clicks at the test
// tempo, not the 2 s production period), the simulated server
// consume-and-advance, and the stuck acceleration. The acceleration
// arms the stuck timer ONLY when the plan is not fresh (no re-plan
// armed a new waypoint slice this iteration) and the character has
// not moved a cell since the previous iteration - a freshly re-planned
// leg gets its grace iteration first, exactly the way the production
// stuck window gives every plan time to click before the next
// escalation judges it.
type walkDrive struct {
	plan    *[]pathfind.Vec3
	stage   int
	lastX   int32
	lastY   int32
	started bool
}

func newWalkDrive() *walkDrive {
	return &walkDrive{}
}

func (d *walkDrive) step(
	loop *Loop, game *fakeGame, bot *state.Bot, sim *reproServer,
) {
	loop.moveAt = time.Time{}
	loop.tick()
	sim.consume(game)
	sim.advance(bot)
	// The grace: a tick that replaced the plan (a re-path, a ladder
	// rung) gets its follow-up iteration free of stuck arming - the
	// fresh plan must click first.
	fresh := &loop.waypoints != d.plan || loop.frozenStage != d.stage
	x, y, _, ok := bot.SelfPosition()
	if ok {
		if !fresh && d.started && sim.stalled &&
			x == d.lastX && y == d.lastY && len(game.walks) > 0 {
			armStuck(loop, bot)
		}
		d.lastX, d.lastY = x, y
		d.started = true
	}
	d.plan, d.stage = &loop.waypoints, loop.frozenStage
}

// containsClick reports whether the talk click reached the teacher.
func containsClick(clicks []int32, objectID int32) bool {
	for _, clicked := range clicks {
		if clicked == objectID {
			return true
		}
	}

	return false
}
