// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"bytes"
	"log"
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The tests of the aggro-aware walk steering (see loop_avoid.go).
// The scene mobs use the Kaboo Orc Fighter template (the wire id
// 1000471): an aggressive monster whose effective on-sight trigger
// radius runs at the Mobius MaxAggroRange clamp (450) - the number
// the server actually attacks on.

// avoidSceneBot builds the standard steering scene bot: a character
// at 45000 50000 on the flat ground deck.
func avoidSceneBot() *state.Bot {
	return newTestBot()
}

// avoidCampMob spawns the aggressive camp mob at the given point.
func avoidCampMob(bot *state.Bot, x int32, y int32) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000471, Attackable: true,
		X: x, Y: y, Z: -3500, Name: "Kaboo Orc Fighter",
	})
}

// TestSteerClearDeflectsLegAroundCamp pins the core deflection: a
// leg whose straight line runs over an idle aggressive mob bends its
// endpoint sideways, and the bent endpoint clears the on-sight
// trigger circle of the mob (its aggro range plus the steering
// clearance) in the planar distance the walk can control.
func TestSteerClearDeflectsLegAroundCamp(t *testing.T) {
	bot := avoidSceneBot()
	avoidCampMob(bot, 45500, 50000)
	loop := NewLoop(&fakeGame{}, bot)

	toX, toY, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		47000, 50000, time.Now())
	require.True(t, dodged)
	// The deflected endpoint clears the trigger circle: at least the
	// aggro range (450) plus the clearance margin (150) away from the
	// camp mob, planar.
	clearance := math.Hypot(float64(toX-45500), float64(toY-50000))
	require.GreaterOrEqual(t, clearance, 600.0)
	// The bend leaves the original line: the endpoint moved sideways,
	// not just shortened along the leg.
	require.NotEqual(t, 50000, toY)
}

// TestSteerClearLeavesCleanLegAlone pins the negative: a leg whose
// line passes far beyond the trigger circle of every mob (the mob
// 2000 units off the line) issues unchanged - the steering never
// invents detours on clean ground.
func TestSteerClearLeavesCleanLegAlone(t *testing.T) {
	bot := avoidSceneBot()
	avoidCampMob(bot, 45500, 52000)
	loop := NewLoop(&fakeGame{}, bot)

	toX, toY, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		47000, 50000, time.Now())
	require.False(t, dodged)
	require.Equal(t, int32(46000), toX)
	require.Equal(t, int32(50000), toY)
}

// TestSteerClearExemptsDestinationMobs pins the arrival exemption:
// the mobs standing at the destination of the walk itself never bend
// the leg - the ground the walk deliberately enters carries its own
// content, and passing within the aggro range on arrival is the
// point of the walk (the engage phase answers whatever the entry
// radius offers).
func TestSteerClearExemptsDestinationMobs(t *testing.T) {
	bot := avoidSceneBot()
	// The camp sits ON the destination of the whole walk.
	avoidCampMob(bot, 46500, 50000)
	loop := NewLoop(&fakeGame{}, bot)

	_, _, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		46500, 50000, time.Now())
	require.False(t, dodged)
}

// TestSteerClearIgnoresPassiveBusyAndTargetMobs pins the threat
// filters of the steering: a passive mob on the line never bends the
// leg, a mob already holding a target never bends it (a chaser
// belongs to the flee machinery, a busy fighter to somebody else's
// fight), and the current fight target of the loop drops out through
// its id.
func TestSteerClearIgnoresPassiveBusyAndTargetMobs(t *testing.T) {
	// The passive mob: a Red Keltir (wire 1000534) on the line.
	bot := avoidSceneBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000534, Attackable: true,
		X: 45500, Y: 50000, Z: -3500, Name: "Red Keltir",
	})
	loop := NewLoop(&fakeGame{}, bot)
	_, _, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		47000, 50000, time.Now())
	require.False(t, dodged, "a passive mob never bends the leg")

	// The busy mob: the aggressive fighter holds another object as
	// its target.
	bot = avoidSceneBot()
	avoidCampMob(bot, 45500, 50000)
	bot.ApplyObjectTarget(7, 999)
	loop = NewLoop(&fakeGame{}, bot)
	_, _, dodged = loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		47000, 50000, time.Now())
	require.False(t, dodged, "a mob holding a target never bends the leg")

	// The fight target: the aggressive mob IS the target the loop
	// walks to (the forced approach of the engage).
	bot = avoidSceneBot()
	avoidCampMob(bot, 45500, 50000)
	loop = NewLoop(&fakeGame{}, bot)
	loop.target = 7
	_, _, dodged = loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		47000, 50000, time.Now())
	require.False(t, dodged, "the current fight target never bends the leg")
}

// TestSteerClearLeavesAnotherDeckAlone pins the 3D trigger gate: a
// mob standing 700 units above the walk deck never blocks a ground
// line - the server's own isInsideRadius3D check cannot fire across
// the deck gap, and neither may the steering.
func TestSteerClearLeavesAnotherDeckAlone(t *testing.T) {
	bot := avoidSceneBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000471, Attackable: true,
		X: 45500, Y: 50000, Z: -2800, Name: "Kaboo Orc Fighter",
	})
	loop := NewLoop(&fakeGame{}, bot)

	_, _, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		47000, 50000, time.Now())
	require.False(t, dodged)
}

// TestTownWalkSteersAroundCamp pins the steering inside the town trip
// follower: the waypoint walk across an aggressive camp issues its
// ground click at the deflected endpoint instead of the waypoint.
func TestTownWalkSteersAroundCamp(t *testing.T) {
	bot := avoidSceneBot()
	avoidCampMob(bot, 45500, 50000)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.phase = phaseTownReturn
	loop.legDest = pathfind.Vec3{X: 46500, Y: 50000, Z: -3500}
	loop.waypoints = []pathfind.Vec3{
		{X: 46000, Y: 50000, Z: -3500},
	}
	require.False(t, loop.walkTownWaypoints())
	require.Len(t, game.walks, 1)
	walk := game.walks[0]
	require.NotEqual(t, int32(50000), walk[1],
		"the issued walk bends off the camp line")
	clearance := math.Hypot(float64(walk[0]-45500), float64(walk[1]-50000))
	require.GreaterOrEqual(t, clearance, 600.0)
}

// TestZoneLegSteersAroundCamp pins the steering inside the direct
// zone return leg: the fallback walk toward the zone center bends
// around the camp the straight line would run over.
func TestZoneLegSteersAroundCamp(t *testing.T) {
	bot := avoidSceneBot()
	avoidCampMob(bot, 45400, 50000)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	zone := &state.Zone{CX: 46500, CY: 50000, Half: 800}
	loop.walkZoneLeg(zone, 45000, 50000, -3500)
	require.Len(t, game.walks, 1)
	walk := game.walks[0]
	require.NotEqual(t, int32(50000), walk[1])
	clearance := math.Hypot(float64(walk[0]-45400), float64(walk[1]-50000))
	require.GreaterOrEqual(t, clearance, 600.0)
}

// TestSteerRidesTheTangentOfTheCircle pins the tangent construction:
// a leg clipping the margin circle of a threat the character stands
// OUTSIDE of bends onto the tangent ray of that circle - the issued
// segment grazes the trigger distance without entering it, on the
// side the original leg leaned to.
func TestSteerRidesTheTangentOfTheCircle(t *testing.T) {
	bot := avoidSceneBot()
	// 762 units out, north-east of the eastbound line.
	avoidCampMob(bot, 45700, 50300)
	loop := NewLoop(&fakeGame{}, bot)

	toX, toY, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		48000, 50000, time.Now())
	require.True(t, dodged)
	// The minimum planar distance of the issued segment to the threat
	// stays at the trigger margin (the tangent graze).
	dirX, dirY := float64(toX-45000), float64(toY-50000)
	dirLen := math.Hypot(dirX, dirY)
	relX, relY := 45700.0-45000.0, 50300.0-50000.0
	foot := (relX*dirX + relY*dirY) / (dirLen * dirLen)
	require.GreaterOrEqual(t, foot, 0.0)
	require.LessOrEqual(t, foot, 1.0)
	cx, cy := 45000+dirX*foot, 50000+dirY*foot
	graze := math.Hypot(45700-cx, 50300-cy)
	require.InDelta(t, 600.0, graze, 1.0)
	// The tangent rides the south side (the leg leaned south of the
	// threat axis).
	require.Less(t, toY, int32(50000))
}

// TestSteerRecedingHorizonPassesCampCleanly pins the chained behavior
// the transit walks rely on: a character walking east past a camp
// through repeated steering requests (one per walk period, each from
// the live position) never dips inside the raw on-sight trigger
// radius of the camp and still makes it past the camp to the east.
func TestSteerRecedingHorizonPassesCampCleanly(t *testing.T) {
	bot := avoidSceneBot()
	avoidCampMob(bot, 46500, 50000)
	loop := NewLoop(&fakeGame{}, bot)

	selfX, selfY := int32(45000), int32(50000)
	const wayX, wayY = int32(48000), int32(50000)
	minDist := math.MaxFloat64
	for range 40 {
		dx, dy := float64(wayX-selfX), float64(wayY-selfY)
		dist := math.Hypot(dx, dy)
		toX, toY := wayX, wayY
		if dist > 1000 {
			frac := 1000 / dist
			toX = int32(float64(selfX) + dx*frac)
			toY = int32(float64(selfY) + dy*frac)
		}
		ax, ay, _ := loop.steerClearOfAggro(
			selfX, selfY, -3500, toX, toY, -3500,
			wayX, wayY, time.Now())
		// One walk period at run speed: ~350 units toward the issued
		// target, the tracker follows the arrival.
		wdx, wdy := float64(ax-selfX), float64(ay-selfY)
		walkLen := math.Hypot(wdx, wdy)
		if walkLen > 350 {
			frac := 350 / walkLen
			selfX = int32(float64(selfX) + wdx*frac)
			selfY = int32(float64(selfY) + wdy*frac)
		} else {
			selfX, selfY = ax, ay
		}
		bot.ApplyMovement(state.Movement{
			ObjectID: 100, X: selfX, Y: selfY, Z: -3500,
			DestX: selfX, DestY: selfY, DestZ: -3500,
		})
		if d := math.Hypot(
			float64(selfX-46500), float64(selfY-50000)); d < minDist {
			minDist = d
		}
	}
	require.GreaterOrEqual(t, minDist, 450.0,
		"the walk never enters the raw on-sight trigger circle")
	require.Greater(t, selfX, int32(47500),
		"the walk still passes the camp eastward")
}

// TestSteerUsesProjectedThreatPosition pins the moving-threat
// handling: the scan reads the PROJECTED position of a walking mob,
// not its stale spawn point - a camp drifting across the walk line
// (the slow movers of the user report) bends the leg exactly like a
// standing one, and a mob whose projected position already cleared
// the line does not.
func TestSteerUsesProjectedThreatPosition(t *testing.T) {
	bot := avoidSceneBot()
	// The camp spawns 500 north of the walk line - clear of it - and
	// its movement broadcast walks it south, across the line: the
	// projected position (46500 50100) already menaces the leg end.
	avoidCampMob(bot, 46500, 50500)
	bot.ApplyMovement(state.Movement{
		ObjectID: 7, X: 46500, Y: 50100, Z: -3500,
		DestX: 46500, DestY: 49500, DestZ: -3500,
	})
	loop := NewLoop(&fakeGame{}, bot)
	_, _, dodged := loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		48000, 50000, time.Now())
	require.True(t, dodged,
		"the projected position across the line bends the leg")

	// The same camp never moving stays 707 units off the leg end -
	// beyond the trigger margin - and never bends it.
	bot = avoidSceneBot()
	avoidCampMob(bot, 46500, 50500)
	loop = NewLoop(&fakeGame{}, bot)
	_, _, dodged = loop.steerClearOfAggro(
		45000, 50000, -3500, 46000, 50000, -3500,
		48000, 50000, time.Now())
	require.False(t, dodged,
		"a camp projected clear of the line never bends the leg")
}

// fightingScene builds a hunting loop mid-fight: the character at
// 45000 50000 swings at a passive keltir next to it (object 8) while
// an aggressive Kaboo Orc Fighter stalks at the given offset.
func fightingScene(addOffsetX int32) (*Loop, *fakeGame, *bytes.Buffer) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000534, Attackable: true,
		X: 45100, Y: 50000, Z: -3500, Name: "Red Keltir",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000471, Attackable: true,
		X: 45000 + addOffsetX, Y: 50000, Z: -3500, Name: "Kaboo Orc Fighter",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	logBuf := &bytes.Buffer{}
	loop.SetLogger(log.New(logBuf, "", 0))
	loop.target = 8
	// The fight runs: our own attack broadcast holds the fresh fight
	// window on the keltir.
	bot.ApplyAttack(state.Attack{
		AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
		TargetX: 45100, TargetY: 50000, TargetZ: -3500,
		TargetIDs:   [state.AttackTargets]int32{8},
		TargetCount: 1,
	})

	return loop, game, logBuf
}

// TestFightStepsClearOfImpendingAdd pins the combat half of the
// steering: a healthy fighting character with an idle aggressive mob
// stalking inside the on-sight trigger band (its aggro range plus the
// warning margin) steps straight away from it - the melee target
// follows, the fight resumes through the re-request, but the distance
// to the impending add opened before its trigger fired.
func TestFightStepsClearOfImpendingAdd(t *testing.T) {
	loop, game, logBuf := fightingScene(550)
	loop.tick()
	require.Len(t, game.walks, 1)
	walk := game.walks[0]
	require.Equal(t, int32(44700), walk[0])
	require.Equal(t, int32(50000), walk[1])
	require.True(t, time.Now().Before(loop.combatAvoidUntil),
		"the step owns the movement window")
	require.Contains(t, logBuf.String(),
		"Kaboo Orc Fighter stalks the fight 550 units out, stepping clear")
}

// TestFightIgnoresFarAdd pins the warning band: an aggressive mob
// beyond the band (800 units out against the 600 band edge) never
// disturbs a running fight.
func TestFightIgnoresFarAdd(t *testing.T) {
	loop, game, _ := fightingScene(800)
	loop.tick()
	require.Empty(t, game.walks)
}

// TestFightStepPacesItself pins the step pacing: the immediate
// re-ticks of the running fight never stack a second step on the
// window of the first.
func TestFightStepPacesItself(t *testing.T) {
	loop, game, _ := fightingScene(550)
	loop.tick()
	require.Len(t, game.walks, 1)
	loop.tick()
	loop.tick()
	require.Len(t, game.walks, 1)
}

// TestFightStepHoldsTheAttackRequests pins the movement window: while
// the step walks, the forced attack re-request of the engage waits -
// the server interrupts a running walk on the attack order, and the
// add would meet the character right back where it stood. The gate is
// observable through the issued attack requests on a target without a
// fresh fight window (the exact state a stepping character has once
// its attack broadcast lapses mid-step).
func TestFightStepHoldsTheAttackRequests(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, TemplateID: 1000534, Attackable: true,
		X: 45100, Y: 50000, Z: -3500, Name: "Red Keltir",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.target = 8
	loop.combatAvoidUntil = time.Now().Add(2 * time.Second)
	loop.tick()
	require.Empty(t, game.forces,
		"the step window holds the forced attack re-request")
	loop.combatAvoidUntil = time.Now().Add(-time.Second)
	loop.tick()
	require.NotEmpty(t, game.forces,
		"the lapsed window releases the re-request")
}

// TestFightStepRespectsTheLeash pins the leash priority: a step that
// would leave the hunting square is skipped - the leash outranks the
// add.
func TestFightStepRespectsTheLeash(t *testing.T) {
	loop, game, _ := fightingScene(550)
	loop.SetHuntingZone(45000, 50000, 200)
	loop.tick()
	require.Empty(t, game.walks)
}

// TestFightStepSkipsAnotherDeck pins the 3D gate of the combat scan:
// an add standing on another deck (700 units above the fight) cannot
// fire its trigger across the gap, and the fight never steps for it.
func TestFightStepSkipsAnotherDeck(t *testing.T) {
	loop, game, _ := fightingScene(550)
	loop.tracker.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1000471, Attackable: true,
		X: 45550, Y: 50000, Z: -2800, Name: "Kaboo Orc Fighter",
	})
	loop.tick()
	require.Empty(t, game.walks)
}
