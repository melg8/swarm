// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// stardenPos is the spawn point of the Elven village sentinel Starden,
// the guard nearest to the test farm spot (45000, 50000).
var stardenPos = [3]int32{42971, 51372, -2992}

// newDelevelLoop builds a hunt loop over a character of the given level
// with the hunting zone set and gremlins inside it (median zone mob
// level 1): the delevel cycle is trigger at 8, target level 6.
func newDelevelLoop(level int32) (*Loop, *fakeGame, *state.Bot, *fakeNavigator) {
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: level, Race: 1, ClassID: 18,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
		MoveSpeedMult: 1,
	})
	game := &fakeGame{}
	nav := &fakeNavigator{found: true}
	loop := NewLoop(game, bot)
	loop.SetNavigator(nav)
	loop.SetHuntingZone(46112, 41500, 1650)
	loop.lastHit = time.Now().Add(-time.Minute)

	return loop, game, bot, nav
}

// spawnZoneMobs puts living gremlins inside the hunting zone.
func spawnZoneMobs(bot *state.Bot) {
	points := [][2]int32{{46000, 41400}, {46200, 41600}, {45900, 41300}}
	for i, p := range points {
		//nolint:exhaustruct // partial fields for the case
		bot.ApplyNpcInfo(state.NpcInfo{
			ObjectID: int32(200 + i), TemplateID: 1000001,
			Attackable: true, X: p[0], Y: p[1], Name: "Gremlin",
		})
	}
}

// TestDelevelTriggersOnOutleveledZone verifies the trigger: a level 11
// character over level 1 gremlins starts the deleveling at the nearest
// guard with the target level of the full drop chance boundary.
func TestDelevelTriggersOnOutleveledZone(t *testing.T) {
	loop, game, _, _ := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)

	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)
	require.Equal(t, int32(6), loop.delevelTarget,
		"gremlins are level 1, the target is level 6")
	require.Equal(t, [][3]int32{legWalkTarget(
		[3]int32{45000, 50000, -3500}, pathfind.Vec3{X: float64(stardenPos[0]), Y: float64(stardenPos[1]), Z: float64(stardenPos[2])})},
		game.walks, "the walk aims along the leg to the nearest guard")
}

// TestDelevelSkipsAtProperLevel verifies the hysteresis of the cycle: a
// character within the trigger difference of the zone mobs hunts on,
// and without a navigator the deleveling never triggers.
func TestDelevelSkipsAtProperLevel(t *testing.T) {
	loop, _, _, _ := newDelevelLoop(7)
	spawnZoneMobs(loop.tracker)

	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"level 7 over level 1 mobs still hunts (drop chance 82%)")

	loop.navigator = nil
	bot := loop.tracker
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 11, Race: 1, ClassID: 18,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
		MoveSpeedMult: 1,
	})
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase,
		"no deleveling without geodata")
}

// TestDelevelFightsTheGuard verifies the fight stage: the walk ends at
// the guard, the guard npc is picked and attacked once per second.
func TestDelevelFightsTheGuard(t *testing.T) {
	loop, game, bot, _ := newDelevelLoop(11)
	spawnZoneMobs(bot)

	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)

	// The character arrives at the guard spawn point: the fight stage
	// picks the guard and provokes it.
	moveSelfTo(bot, stardenPos[0], stardenPos[1], stardenPos[2])
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 77, TemplateID: 7220 + 1000000,
		X: stardenPos[0], Y: stardenPos[1], Z: stardenPos[2],
		Name: "Starden",
	})
	loop.tick()
	require.Equal(t, []int32{77}, game.forces,
		"the first request selects the guard")

	// The attack repeats once per second until the death.
	loop.lastHit = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{77, 77}, game.forces)
}

// TestDelevelDeathContinues verifies that a death during the deleveling
// keeps the phase running: the village restart goes out and the walk to
// the guard is replanned after the revival.
func TestDelevelDeathContinues(t *testing.T) {
	loop, game, bot, nav := newDelevelLoop(11)
	spawnZoneMobs(bot)

	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)

	// The guard kills the character: the restart request goes out and
	// the phase stays in the deleveling.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
	require.Equal(t, 1, game.restarts)
	require.Equal(t, phaseDelevel, loop.phase)
	require.Nil(t, loop.waypoints, "the walk replans from the village")

	// Revived at level 11 still: a fresh walk to the guard starts.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 100},
	})
	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)
	require.Equal(t, 2, nav.calls, "the walk replanned")
	require.Len(t, game.walks, 2, "the walk restarted")
}

// TestDelevelFinishesAtTarget verifies the exit: once the deaths bring
// the character to the target level, the bot walks back to the farm
// spot and resumes the hunt.
func TestDelevelFinishesAtTarget(t *testing.T) {
	loop, _, bot, _ := newDelevelLoop(11)
	spawnZoneMobs(bot)

	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)

	// The deaths did their work: level 6, the deleveling ends.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 6, Race: 1, ClassID: 18,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
		MoveSpeedMult: 1,
	})
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase,
		"the walk home starts")

	// The character walks home (the farm spot is the zone center, the
	// test position sits outside the zone): the hunt resumes there.
	loop.tick()
	moveSelfTo(bot, 46112, 41500, -3500)
	loop.tick()
	require.Equal(t, phaseEngage, loop.phase)
	require.False(t, loop.delevelCooldownOver(),
		"the delevel cooldown is armed")
}

// TestDelevelFightTimeoutRepaths verifies the guard fight protection: a
// guard that never fights back re-paths the walk, and after the budget
// is spent the deleveling aborts back to the hunt.
func TestDelevelFightTimeoutRepaths(t *testing.T) {
	loop, game, bot, _ := newDelevelLoop(11)
	spawnZoneMobs(bot)

	loop.tick()
	moveSelfTo(bot, stardenPos[0], stardenPos[1], stardenPos[2])
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 77, TemplateID: 7220 + 1000000,
		X: stardenPos[0], Y: stardenPos[1], Z: stardenPos[2],
		Name: "Starden",
	})
	loop.tick()
	require.Equal(t, int32(77), loop.delevelGuard)

	// The guard never hits back: past the fight timeout the stage walks
	// a step out of the village (the peace zone blocks the retaliation)
	// and keeps provoking.
	loop.delevelFight = time.Now().Add(-delevelFightTimeout - time.Second)
	loop.moveAt = time.Time{}
	loop.tick()
	require.Equal(t, int32(77), loop.delevelGuard, "the guard stays")
	require.Equal(t, 1, loop.rePaths)
	require.Equal(t, [][3]int32{{46112, 41500, -3500}}, game.walks[1:],
		"the step out walks toward the farm spot")

	// The budget runs out: the deleveling aborts.
	for range maxRePaths {
		loop.delevelFight = time.Now().
			Add(-delevelFightTimeout - time.Second)
		loop.moveAt = time.Time{}
		loop.tick()
	}
	require.Equal(t, phaseEngage, loop.phase,
		"the deleveling aborts when the re-paths are spent")
	require.False(t, loop.delevelCooldownOver())
}
