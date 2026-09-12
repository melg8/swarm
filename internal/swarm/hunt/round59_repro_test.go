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

// The reproduction of the 2026-09-12 03:25 state dump report (build
// a4c9e15, bot test1, phase engage, uptime 7m13s): the level 14
// character stood at x 35224 y 47288 z -3656 with the selected target
// 268439361 (Kaboo Orc Fighter, level 10, hp 257/257) at 35144 47288
// -3656 - 80 units away, the exact melee standoff - with "moving: no",
// "walk plan: none", "in combat: true" and an empty combat feed ("recent
// combat (0)", no swing ever landed), while the chat window held
// "Cannot see target." every ~3 seconds for over a minute and the event
// log showed nothing but a Power Strike cast every 15 seconds. The bot
// neither moved, nor switched the target, nor attacked successfully.
//
// The event trail that led there: the previous kill finished at
// 03:24:12, the picker selected the aggressive Kaboo that had stalked
// the fight, the forced attack armed the server side ATTACK intention
// and the server chase started broadcasting the character's own
// MoveToPawn steps - but the geodata line of sight between the two
// positions is blocked, so every doAttack attempt of the armed
// intention answered "Cannot see target." and disarmed itself, only
// for the loop's 1s paced re-request to re-arm it seconds later. The
// MoveToPawn stream of that phantom chase kept the tracker's
// CombatActiveAt fresh and FightingTargetID on the target, so
// SelfFighting read true in a livelock: the engage branch re-anchored
// the engage clock on every such tick (loop.go), which held the 12s
// engage stuck timeout away forever, and the "Cannot see target."
// answers landed before the newest re-anchor, which held the blind
// engage recovery away forever. The bot stood 80 units from a mob it
// could not see, casting Power Strike at it every 15 seconds.
//
// The fix (three halves, pinned here):
//  1. blindEngageBlocked no longer trusts the fresh fight view: the
//     refusal being the NEWEST fight activity (nothing landed or
//     stepped after the server said "cannot see") marks the obstructed
//     engage even while the phantom chase keeps the activity fresh.
//  2. recoverBlindEngage stands down only when the fight progressed
//     past the refusal (activity newer than the answer), not merely
//     while the fresh chase view lives.
//  3. the engage clock re-anchor of the fighting branch requires the
//     same progression, so the phantom chase can no longer slide the
//     clock past every refusal.
const (
	// reproRound59X/Y/Z is the reported stuck position (the dump of
	// 2026-09-12 03:25:34, the character test1 in the Spore Fungus SW
	// spot).
	reproRound59X = int32(35224)
	reproRound59Y = int32(47288)
	reproRound59Z = int32(-3656)
	// reproRound59Target is the selected target of the dump (the
	// aggressive Kaboo Orc Fighter 80 units west, same heading line).
	reproRound59Target = int32(268439361)
	// reproRound59TargetX/Y is the position of the dump target.
	reproRound59TargetX = int32(35144)
	reproRound59TargetY = int32(47288)
	// reproRound59ZoneX/Y is the hunting zone center of the dump (the
	// Spore Fungus SW anchoring spot, the leash half 1448).
	reproRound59ZoneX = int32(35292)
	reproRound59ZoneY = int32(46424)
	// reproRound59Spare is the replacement pick of the dump's object
	// list: the Spore Fungus 809 units out, alive and inside the zone.
	reproRound59Spare     = int32(268439350)
	reproRound59SpareX    = int32(35560)
	reproRound59SpareY    = int32(46552)
	reproDumpKabooFighter = int32(1000471)
)

// reproRound59Scene builds the dump standoff: the character test1 at
// the dump position inside the dump zone, the obstructed Kaboo Orc
// Fighter at 80 units, the spare Spore Fungus of the dump object list
// as the only other pickable mob, and the loop in the engage phase
// with the target selected and the engage clock past the detection
// delay.
func reproRound59Scene(t *testing.T) (*Loop, *fakeGame, *state.Bot) {
	t.Helper()
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 268450864, 18,
		reproRound59X, reproRound59Y, reproRound59Z, 339, 137)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 14, Race: 1, ClassID: 18,
		X: reproRound59X, Y: reproRound59Y, Z: reproRound59Z,
		MaxHP: 339, CurHP: 339, MaxMP: 137, CurMP: 137,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: reproRound59Target, TemplateID: reproDumpKabooFighter,
		Attackable: true, X: reproRound59TargetX, Y: reproRound59TargetY,
		Z: reproRound59Z, Name: "Kaboo Orc Fighter",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID:   reproRound59Spare,
		TemplateID: reproDumpSporeFungusID,
		Attackable: true, X: reproRound59SpareX, Y: reproRound59SpareY,
		Z: -3688, Name: "Spore Fungus",
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZone(reproRound59ZoneX, reproRound59ZoneY, 1448)
	loop.target = reproRound59Target
	loop.engageAt = time.Now().Add(-3 * time.Second)
	loop.lastHit = time.Now().Add(-time.Minute)

	return loop, game, bot
}

// reproRound59PhantomChase steps the server side phantom of the dump:
// the armed but refused attack keeps broadcasting the character's own
// chase steps (MoveToPawn, zero distance - the dump's "moving: no" with
// a fresh fight view) and keeps answering the attack attempts with
// "Cannot see target." - the answer lands AFTER the newest chase step,
// so the refusal is the newest fight activity of the livelock.
func reproRound59PhantomChase(bot *state.Bot) {
	// The phantom chase step: the character chases the target, the
	// destination collapses onto the walker (the wall holds it), the
	// tracker still records the fresh fight activity and the fighting
	// target.
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 268450864, TargetID: reproRound59Target, Distance: 0,
		X: reproRound59X, Y: reproRound59Y, Z: reproRound59Z,
		TargetX: reproRound59TargetX, TargetY: reproRound59TargetY,
		TargetZ: reproRound59Z,
	})
	// The refusal of the armed attack: the doAttack of the chase
	// failed the line of sight check - the answer is fresher than the
	// chase step it answered.
	time.Sleep(10 * time.Millisecond)
	bot.ApplySystemMessage(state.SystemMessage{ID: 181})
}

// TestReproRound59PhantomChaseArmsBlindRecovery pins the activation
// fix: the phantom chase (a fresh fight view built from the chase steps
// of the very attack the server keeps refusing) must not suppress the
// blind engage recovery. The tick after the refusal arms the recovery
// and walks the first geodata reposition leg at once - the inversion of
// the dump signature (a character standing forever, no walk, no switch).
func TestReproRound59PhantomChaseArmsBlindRecovery(t *testing.T) {
	loop, game, bot := reproRound59Scene(t)
	nav := &fakeNavigator{found: true, route: []pathfind.Vec3{
		{X: 35190, Y: 47230, Z: -3656},
		{X: 35150, Y: 47190, Z: -3656},
	}}
	loop.SetNavigator(nav)
	reproRound59PhantomChase(bot)

	loop.tick()

	require.False(t, loop.losAt.IsZero(),
		"the phantom chase must not hold the blind recovery back")
	require.NotEmpty(t, loop.losWaypoints,
		"the geodata route becomes the reposition waypoints")
	require.Equal(t, 1, loop.losTried, "one planning attempt is spent")
	require.Empty(t, game.forces,
		"no attack re-request may fire while the reposition runs")
	require.Len(t, game.walks, 1,
		"the first reposition leg walks on the planning tick")
	require.InDelta(t, 35190, game.walks[0][0], 1)
	require.InDelta(t, 47230, game.walks[0][1], 1)
}

// TestReproRound59PhantomChaseSwitchesTarget pins the livelock exit:
// the refusal persists through the reposition attempts (the vantage
// point never clears the sight line), so the recovery spends its
// attempt budget and drops the obstructed target - the skip list holds
// the Kaboo out of the search and the next pick takes the Spore Fungus
// of the dump object list instead. The dump stood on the same target
// for over a minute; the fixed loop is off it inside two attempts.
func TestReproRound59PhantomChaseSwitchesTarget(t *testing.T) {
	loop, game, bot := reproRound59Scene(t)
	loop.SetNavigator(&fakeNavigator{found: true})
	reproRound59PhantomChase(bot)

	// Attempt one: the recovery arms and walks.
	loop.tick()
	require.False(t, loop.losAt.IsZero())
	require.Equal(t, 1, loop.losTried)

	// The walk reached its vantage point; the engage is handed back.
	vantage := loop.losWaypoints[len(loop.losWaypoints)-1]
	bot.SetCharacter("test1", 268450864, 18, int32(vantage.X), int32(vantage.Y), int32(vantage.Z), 339, 137)
	loop.tick()
	require.True(t, loop.losAt.IsZero(), "the arrival clears the walk")

	// The handoff re-requests the attack once the phantom chase view
	// of the setup goes stale (the live server cycle: the chase of the
	// refused attack disarms itself, the view ages out, the loop
	// re-requests - and the answer is the same refusal again).
	waitPastFightingFresh()
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Len(t, game.forces, 1,
		"the handoff re-requests the attack from the vantage point")
	bot.ApplySystemMessage(state.SystemMessage{ID: 181})

	// Attempt two: the persisting refusal re-arms the recovery. The
	// character still stands at the vantage point of the first walk,
	// so the re-planned route degenerates onto it and completes on the
	// arming tick - the engage is handed straight back.
	loop.tick()
	require.Equal(t, 2, loop.losTried,
		"the persisting block re-arms the second reposition")
	require.True(t, loop.losAt.IsZero(),
		"the zero-length reposition completes at once")

	// The third refusal meets the spent budget: the target is dropped
	// and skipped instead of a third walk around the same obstacle.
	waitPastFightingFresh()
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Len(t, game.forces, 2)
	bot.ApplySystemMessage(state.SystemMessage{ID: 181})
	loop.tick()

	require.Zero(t, loop.target,
		"the persisting blind engage ends in the target switch")
	require.True(t, loop.targetSkipped(reproRound59Target, time.Now()),
		"the obstructed Kaboo is held out of the search")
	require.Zero(t, loop.losAt, "the recovery bookkeeping clears")

	// The next pick takes the Spore Fungus of the dump scene.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, reproRound59Spare, loop.target,
		"the hunt continues on the spare mob of the dump")
}

// waitPastFightingFresh sleeps past the tracker's fighting fresh
// window: the phantom chase step of the setup must age out before the
// loop re-requests the attack (the fresh chase view holds the
// re-request back exactly like the live disarm gap of the server
// cycle).
func waitPastFightingFresh() {
	time.Sleep(3*time.Second + 150*time.Millisecond)
}

// TestReproRound59EngageClockHoldsPastRefusal pins the clock half of
// the livelock: the fighting branch re-anchors the engage clock only
// when the fight progressed past the last refusal. The phantom chase
// (the refusal is the newest activity) must not slide the clock - the
// stuck timeout and the detection delay measure the true engage age.
func TestReproRound59EngageClockHoldsPastRefusal(t *testing.T) {
	loop, _, bot := reproRound59Scene(t)
	// The detection delay still holds (the engage is one second old),
	// so the tick reaches the fighting branch of the phantom chase.
	loop.engageAt = time.Now().Add(-1 * time.Second)
	reproRound59PhantomChase(bot)
	before := loop.engageAt

	loop.tick()

	require.Equal(t, before, loop.engageAt,
		"the phantom chase must not re-anchor the engage clock")

	// The real progression re-anchors: the chase stepped AFTER the
	// refusal (the sight line cleared - the mob walked past the
	// obstacle edge), the running fight owns the clock again.
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 268450864, TargetID: reproRound59Target, Distance: 40,
		X: reproRound59X, Y: reproRound59Y, Z: reproRound59Z,
		TargetX: reproRound59TargetX, TargetY: reproRound59TargetY,
		TargetZ: reproRound59Z,
	})
	loop.lastHit = time.Now().Add(-time.Minute)
	anchored := time.Now()
	loop.tick()

	require.True(t, loop.engageAt.After(anchored),
		"a fight that progressed past the refusal re-anchors the clock")
	require.True(t, loop.losAt.IsZero(),
		"the progressed fight never arms the blind recovery")
}
