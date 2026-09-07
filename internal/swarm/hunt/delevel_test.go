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
// level 1): the delevel cycle is trigger at 10 (the guard death exp
// penalty only applies from level 10 up), target level 9.
func newDelevelLoop(level int32) (*Loop, *fakeGame, *state.Bot, *fakeNavigator) {
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: level, Race: 1, ClassID: 18,
		Exp: 50000,
		X:   45000, Y: 50000, Z: -3500,
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
// archer guard with the target level of the full drop chance boundary.
func TestDelevelTriggersOnOutleveledZone(t *testing.T) {
	loop, game, _, _ := newDelevelLoop(11)
	spawnZoneMobs(loop.tracker)

	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)
	require.Equal(t, int32(9), loop.delevelTarget,
		"gremlins are level 1, the target clamps at the Lucky protection level")
	require.Equal(t, [][3]int32{legWalkTarget(
		[3]int32{45000, 50000, -3500}, pathfind.Vec3{X: float64(stardenPos[0]), Y: float64(stardenPos[1]), Z: float64(stardenPos[2])})},
		game.walks, "the walk aims along the leg to the nearest guard")
}

// TestDelevelSkipsAtProperLevel verifies the hysteresis of the cycle: a
// character within the trigger difference of the zone mobs walks home
// over the geodata waypoints without deleveling, and without a navigator
// the deleveling never triggers.
func TestDelevelSkipsAtProperLevel(t *testing.T) {
	loop, _, _, _ := newDelevelLoop(7)
	spawnZoneMobs(loop.tracker)

	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase,
		"level 7 over level 1 mobs pathfinds home instead of deleveling")

	loop.navigator = nil
	bot := loop.tracker
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 11, Race: 1, ClassID: 18, Exp: 50000,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
		MoveSpeedMult: 1,
	})
	loop.tick()
	require.NotEqual(t, phaseDelevel, loop.phase,
		"no deleveling without geodata")
}

// TestDelevelSkipsBelowLevel10 verifies the experience rule of the
// deleveling: a guard death at level 9 or below removes no experience
// (the Lucky newbie protection absorbs the penalty), so an outleveled
// level 9 character keeps hunting instead of burning free deaths.
func TestDelevelSkipsBelowLevel10(t *testing.T) {
	loop, _, _, _ := newDelevelLoop(9)
	spawnZoneMobs(loop.tracker)

	loop.tick()
	require.NotEqual(t, phaseDelevel, loop.phase,
		"level 9 is below the delevel minimum: deaths are free there")
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
		Name: "test1", Level: 6, Race: 1, ClassID: 18, Exp: 46000,
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

// TestDelevelFightTimeoutSwitchesGuard verifies the guard fight
// protection: a guard that never fights back is marked as tried and the
// deleveling switches to the next guard, aborting into the walk home
// when every guard ignored the provocation.
func TestDelevelFightTimeoutSwitchesGuard(t *testing.T) {
	loop, game, bot, nav := newDelevelLoop(11)
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
	callsBefore := nav.calls

	// The guard never hits back: past the fight timeout the guard is
	// dropped, marked as tried and the walk replans to the next guard.
	provokeAndTimeout(t, loop, bot)
	require.Equal(t, int32(0), loop.delevelGuard, "the guard is dropped")
	require.Equal(t, 1, loop.rePaths)
	require.True(t, loop.delevelTried["Starden"],
		"the silent guard is marked as tried")
	require.Nil(t, loop.waypoints, "the walk replans")
	require.Equal(t, phaseDelevel, loop.phase, "the deleveling goes on")

	// The replanned walk goes to the nearest untried guard: Kendell
	// is the only other archer guard of the village, 4600 units east
	// of Starden's post, so the first walk leg stops 1000 units along
	// the way (the server move request limit).
	loop.tick()
	require.Equal(t, callsBefore+1, nav.calls, "the path replanned")
	require.Len(t, game.walks, 2, "the walk to the next guard started")
	require.Equal(t, legWalkTarget(stardenPos, pathfind.Vec3{
		X: float64(kendellPos[0]), Y: float64(kendellPos[1]),
		Z: float64(kendellPos[2]),
	}), game.walks[1], "the walk starts the leg toward the Kendell spawn")

	// Kendell ignores the provocation too: with every archer guard
	// marked as tried there is nothing left to walk to.
	arriveAndProvoke(loop, bot, 79, 7218+1000000, kendellPos, "Kendell")
	provokeAndTimeout(t, loop, bot)
	require.True(t, loop.delevelTried["Kendell"])
	require.Equal(t, 2, loop.rePaths)

	// Both archer guards ignored the provocation: the deleveling
	// aborts into the walk home through the town trip return leg
	// (a direct engage walk from the guard post would be refused by
	// the 9900 unit server move limit).
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase,
		"the aborted delevel walks home through the return leg")
	require.False(t, loop.delevelCooldownOver())
}

// TestDelevelAbortsOnFreeDeaths verifies the free death counter: a
// server whose guard deaths remove no experience at all (the vanilla
// penalty branch only runs for playable killers) never drops the
// level, so after the third penalty-free death the deleveling aborts
// and the long cooldown keeps it from retrying every minute.
func TestDelevelAbortsOnFreeDeaths(t *testing.T) {
	loop, game, bot, _ := newDelevelLoop(11)
	spawnZoneMobs(bot)

	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)
	require.Equal(t, int32(50000), loop.delevelExp,
		"the exp baseline of the free death counter is armed")

	// Three deaths without any exp change: the counter climbs, the
	// third aborts the deleveling once the character is alive again.
	for i := 1; i <= delevelFreeDeaths; i++ {
		loop.restartAt = time.Time{}
		bot.ApplyStatusUpdate(100, []state.Attribute{
			{ID: state.AttrCurHP, Value: 0},
		})
		loop.tick()
		require.Equal(t, phaseDelevel, loop.phase,
			"the free death keeps the deleveling while the restart retries")
		require.Equal(t, i, loop.delevelFree)

		// Revived without any exp change: the vanilla penalty
		// branch never ran, the UserInfo exp stays stale. The
		// alive tick clears the per death count; the last death
		// of the budget aborts on its alive tick instead.
		bot.ApplyStatusUpdate(100, []state.Attribute{
			{ID: state.AttrCurHP, Value: 100},
		})
		if i < delevelFreeDeaths {
			loop.tick()
			require.Equal(t, phaseDelevel, loop.phase,
				"the deleveling continues below the free death budget")
		}
	}
	loop.tick()
	require.Equal(t, phaseTownReturn, loop.phase,
		"the penalty-free deleveling aborts into the walk home")
	require.False(t, loop.delevelCooldownOver(),
		"the long free death cooldown is armed")
	require.Equal(t, 3, game.restarts,
		"the restart request went out for every death")
}

// TestDelevelDeathProgressResetsFreeCounter verifies that a death
// which removed experience (the UserInfo exp dropped) resets the free
// death counter: the deleveling keeps dying on such a server.
func TestDelevelDeathProgressResetsFreeCounter(t *testing.T) {
	loop, game, bot, _ := newDelevelLoop(11)
	spawnZoneMobs(bot)

	loop.tick()
	// One free death first: revived, the alive tick clears the per
	// death count.
	loop.restartAt = time.Time{}
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
	require.Equal(t, 1, loop.delevelFree)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 100},
	})
	loop.tick()
	require.Equal(t, phaseDelevel, loop.phase)

	// The server then pays the penalty: the UserInfo refresh carries
	// a lower exp and the next death resets the counter.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 11, Race: 1, ClassID: 18, Exp: 46000,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 1, MaxMP: 40, CurMP: 30,
		MaxLoad: 88320, RunSpeed: 125, WalkSpeed: 60,
		MoveSpeedMult: 1,
	})
	loop.restartAt = time.Time{}
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
	require.Equal(t, 0, loop.delevelFree,
		"the exp paying death reset the counter")
	require.Equal(t, phaseDelevel, loop.phase)
	require.Equal(t, 2, game.restarts,
		"both deaths requested their village restart")
}

// provokeAndTimeout advances one silent guard fight: the fight stage
// picked a guard (delevelGuard set), the timeout expires and the tick
// marks the guard as tried.
func provokeAndTimeout(t *testing.T, loop *Loop, bot *state.Bot) {
	t.Helper()
	require.NotZero(t, loop.delevelGuard, "a guard must be picked first")
	loop.delevelFight = time.Now().Add(-delevelFightTimeout - time.Second)
	loop.moveAt = time.Time{}
	loop.tick()
}

// arriveAndProvoke moves the character to the guard spawn, publishes the
// guard npc and ticks until the fight stage picks it.
func arriveAndProvoke(
	loop *Loop, bot *state.Bot, objectID int32, templateID int32,
	pos [3]int32, name string,
) {
	moveSelfTo(bot, pos[0], pos[1], pos[2])
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: objectID, TemplateID: templateID,
		X: pos[0], Y: pos[1], Z: pos[2], Name: name,
	})
	loop.moveAt = time.Time{}
	loop.tick()
}

// kendellPos is the spawn point of the Elven village sentinel Kendell,
// the archer guard of the eastern post.
var kendellPos = [3]int32{47595, 51569, -2992}
