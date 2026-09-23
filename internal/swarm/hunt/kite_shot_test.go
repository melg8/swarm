// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The shot-paced layer of the archer kite (issue #60, reworked on
// the issue #70 findings): the character's own Attack broadcast is
// the server commit of the bow shot, but the server FORBIDS the
// movement through the windup - the first half of the bow disable
// window - and a MoveToLocation inside it is saved and replayed only
// at the disable end, where the re-shot cancels it. The broadcast
// therefore arms a DEFERRED retreat: the click waits the windup end
// (plus the safety lead), then fires into the accepted move window
// and the walk banks the reload tail. The tests pin the deferral
// timing, the fire-time re-checks, the pursue band, the streak
// exemption and the shared hold answers of the rhythm.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// kiteBowBotChaseFresh builds the kite scene with the fight
// freshness carried by the character's own chase step instead of a
// swing: the server broadcasts MoveToPawn while the character chases
// its target, and the chase refreshes the fight view (CombatActiveAt)
// without committing a shot - the shot-paced trigger stays quiet and
// the proximity path runs alone. The scenes that pin the proximity
// semantics (the streak bound, the radius knob, the deck gap) build
// on this one.
func kiteBowBotChaseFresh(
    t *testing.T, mobX int32,
) (*state.Bot, *fakeGame, *Loop) {
    t.Helper()
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: mobX, Y: 50000, Name: "Keltir",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.engageAt = time.Now()
    // The fight runs: the character chases the mob (the fresh fight
    // view of SelfFighting, no shot committed).
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 40,
        X: 45000, Y: 50000, TargetX: mobX, TargetY: 50000,
        TargetZ: -3500,
    })
    loop.lastHit = time.Now().Add(-time.Minute)

    return bot, game, loop
}

// ageKiteClick backdates the deferred retreat schedule so the next
// tick fires the click - the test twin of the windup the live server
// spends between the shot broadcast and the accepted move window
// (the loop ticks the windup out in real time there).
func ageKiteClick(loop *Loop) {
    loop.kiteClickAt = time.Now().Add(-time.Millisecond)
}

// tickPastTheWindup runs the two ticks of one shot-paced kite cycle
// the live server spends around the windup: the first arms the
// deferred schedule on the shot broadcast, the backdated schedule
// fires the retreat click on the second. Every fresh-shot scene
// dances through it before its walk assertion.
func tickPastTheWindup(loop *Loop) {
    loop.tick()
    ageKiteClick(loop)
    loop.tick()
}

// TestKiteShotPacedDefersTheRetreatPastTheWindup pins the core
// contract of the reworked rhythm: a fresh shot with the hostile
// inside the pursue band does NOT click at the broadcast (the server
// would save the move intention and replay it at the disable end,
// where the re-shot cancels it - the whole-reload stall the issue
// #70 findings measured); the broadcast arms a schedule that clicks
// at the windup end instead, and that click walks the character
// clear.
func TestKiteShotPacedDefersTheRetreatPastTheWindup(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    shotAt := bot.SelfLastShotAt()
    require.False(t, shotAt.IsZero(), "the scene committed a shot")
    loop.tick()
    require.Empty(t, game.walks,
        "a fresh shot must not click the retreat inside the windup")
    require.Empty(t, game.forces,
        "the kite tick must not re-request the attack")
    require.False(t, loop.kiteClickAt.IsZero(),
        "the shot broadcast must arm the deferred retreat")
    require.WithinDuration(t,
        shotAt.Add(kiteWindupLead+loop.kiteWindupWindow()),
        loop.kiteClickAt, 50*time.Millisecond,
        "the click waits the windup end plus the safety lead")
    require.WithinDuration(t,
        shotAt.Add(loop.kiteWalkWindow()+loop.kite.ReengageDelay),
        loop.kiteClickUntil, 50*time.Millisecond,
        "the walk window ends at the shot disable end")

    // The windup elapsed: the click fires into the accepted move
    // window and the retreat walks.
    ageKiteClick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the deferred click must fire once the windup elapsed")
    step := game.walks[0]
    _, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    require.Equal(t, selfZ, step[2],
        "the step keeps the character's deck")
    require.Equal(t, int32(44600), step[0],
        "the step runs straight away from the target")
    require.Equal(t, selfY, step[1])
    require.True(t, loop.kiteClickAt.IsZero(),
        "the fired schedule disarms itself")
}

// TestKiteShotPacedStaysQuietWithoutAFreshShot pins the trigger
// boundary: the fight view refreshed by a chase step (no shot
// committed) keeps the rhythm quiet - the retreat rides the shot
// cycle, not the mere running of the fight.
func TestKiteShotPacedStaysQuietWithoutAFreshShot(t *testing.T) {
    _, game, loop := kiteBowBotChaseFresh(t, 45440)
    loop.tick()
    require.Empty(t, game.walks,
        "no fresh shot must not arm the shot-paced retreat")
    require.Empty(t, game.forces,
        "a running fight inside the band keeps the re-request quiet")
}

// TestKiteShotPacedStaysQuietBeyondTheBand pins the pursue band: a
// hostile beyond the bow engage radius is the stall watchdog's
// re-approach, not a retreat - the rhythm never runs the fight away
// from a mob it should walk back to.
func TestKiteShotPacedStaysQuietBeyondTheBand(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45500)
    loop.tick()
    require.Empty(t, game.walks,
        "a hostile beyond the band must not arm the shot-paced retreat")
    require.Empty(t, game.forces,
        "a running bow fight must not re-request the attack")
    require.True(t, loop.kiteClickAt.IsZero(),
        "no schedule serves a hostile beyond the band")
}

// TestKiteShotPacedIgnoresTheStreakLimit pins the streak exemption:
// the shuffle bound of the proximity path exists because an endless
// step race starves the fight of every swing - the shot-paced cycle
// keeps swinging once per cooldown, so the exhausted streak must not
// stop the retreat.
func TestKiteShotPacedIgnoresTheStreakLimit(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.kiteFor = 7
    loop.kiteStreak = kiteStreakLimit
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the shot-paced retreat is the fight itself, not the shuffle")
}

// TestKiteShotPacedDoesNotCountTheStreak pins the bookkeeping side
// of the same exemption: the rhythm step spends none of the shuffle
// budget - the proximity path keeps its own count untouched.
func TestKiteShotPacedDoesNotCountTheStreak(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)
    require.Zero(t, loop.kiteStreak,
        "the shot-paced step must not spend the shuffle budget")
}

// TestKiteShotPacedWindowGuardsTheDoubleStep pins the trigger window:
// the broadcast stays catchable for two ticks, and the second tick
// inside the window must not issue a second schedule or step - the
// movement window owns the walk until it finished.
func TestKiteShotPacedWindowGuardsTheDoubleStep(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Empty(t, game.walks,
        "the broadcast arms the schedule, not a walk")
    loop.tick()
    require.Empty(t, game.walks,
        "the movement window guards the double trigger")
    ageKiteClick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the deferred click fires exactly once")
}

// TestKiteShotPacedEncircledTrainBreaksThroughTheGap pins the
// deferred click's encircled answer: a surrounding train cancels the
// centroid, and the click resolves to the widest-gap ray at its fire
// moment - the perpendicular break of a two-mob line, never a step
// into a flanker.
func TestKiteShotPacedEncircledTrainBreaksThroughTheGap(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 7, 45200)
    loop.tick()
    require.Empty(t, game.walks,
        "the broadcast tick arms the schedule, not a walk")
    ageKiteClick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the encircled train leaves the widest gap - the click takes it")
    step := game.walks[0]
    require.InDelta(t, 45000.0, float64(step[0]), 1.0,
        "the gap bisector runs perpendicular to the chaser line")
    require.InDelta(t, 50000+kiteStep, float64(step[1]), 1.0,
        "the gap bisector runs perpendicular to the chaser line")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the gap answer is a step, not a hold")
}

// TestKiteShotPacedEncircledGapBlockedHoldsGround pins the encircled
// hold of the CLOSED pocket: the widest-gap ray, its whole fan AND
// the anti-gap breakout cone stand walled (the sealed corner of the
// mass cells), and the deferred click resolves to the hold ground
// rule - the encircled archer stands and shoots the way out only
// when the whole circle is shut. A pocket that walls the gap
// hemisphere alone no longer holds: the breakout tier threads the
// anti-gap cone instead (kite_breakout_test.go pins that half).
func TestKiteShotPacedEncircledGapBlockedHoldsGround(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 7, 45200)
    nav := &fakeNavigator{}
    // The whole circle answers walled: the gap hemisphere (north of
    // the chaser line) AND the anti-gap breakout cone (south of it)
    // - the sealed pocket.
    nav.sightFunc = func(_, _ pathfind.Vec3) (bool, error) {
        return false, nil
    }
    loop.SetNavigator(nav)
    loop.tick()
    require.Empty(t, game.walks,
        "the broadcast tick arms the schedule, not a walk")
    ageKiteClick(loop)
    loop.tick()
    require.Empty(t, game.walks,
        "an encircled archer in a sealed pocket stops retreating")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the encircled hold is armed for the target")
}

// TestKiteShotPacedCorneredHoldsGround pins the corner answer of the
// rhythm: a wall behind the retreat blocks every lane of the away
// hemisphere, and the deferred click resolves to the cornered hold
// at its fire moment - the archer stands and shoots the way out.
func TestKiteShotPacedCorneredHoldsGround(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    nav := &fakeNavigator{}
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.X > 45000, nil
    }
    loop.SetNavigator(nav)
    loop.tick()
    require.Empty(t, game.walks,
        "the broadcast tick arms the schedule, not a walk")
    ageKiteClick(loop)
    loop.tick()
    require.Empty(t, game.walks,
        "a cornered archer stops retreating against the wall")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the cornered hold is armed for the target")
}

// TestKiteShotPacedRespectsTheProfileGate pins the profile gate of
// the rhythm: a kite-disabled profile (the standing-archer baseline
// of issue #29) never arms the shot-paced retreat either - the layer
// rides the same enable switch as the proximity path.
func TestKiteShotPacedRespectsTheProfileGate(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45440)
    loop.SetKiteParams(KiteParams{Enabled: false})
    loop.tick()
    require.Empty(t, game.walks,
        "a kite-disabled bow bot must not step on the shot trigger")
    require.True(t, loop.kiteClickAt.IsZero(),
        "a kite-disabled profile never arms the schedule")
}

// TestKiteDeferredClickDisarmsOnTargetSwitch pins the ownership rule
// of the schedule: the deferred click serves the fight that armed
// it - a target switch between the broadcast and the boundary (the
// old target died, a fresh pick landed) disarms it instead of
// walking a retreat the new fight never asked for. The switch is
// staged the way the live server does it: the character's own
// selection moved to the fresh mob (the attack broadcast of the
// new swing), so the tick's server-selection re-adoption lands the
// new fight - a bare loop.target write would be re-adopted right
// back to the old mob.
func TestKiteDeferredClickDisarmsOnTargetSwitch(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.False(t, loop.kiteClickAt.IsZero(),
        "the scene armed the deferred retreat")
    require.Equal(t, int32(7), loop.kiteClickFor,
        "the armed schedule serves the fight that armed it")
    ageKiteClick(loop)
    // The fresh pick landed: the character swings at mob 8 now (the
    // server selection of the own attack broadcast) - the aged
    // schedule of the dead 7 fight must not fire its click.
    spawnMobAt(bot, 8, 45400)
    bot.ApplyAttack(state.Attack{
        AttackerID:  100,
        X:           45000,
        Y:           50000,
        Z:           -3500,
        TargetX:     45400,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{8},
        TargetCount: 1,
    })
    loop.tick()
    require.Empty(t, game.walks,
        "a schedule of a dead fight must not click")
    require.Equal(t, int32(8), loop.kiteClickFor,
        "the fresh fight owns its own fresh schedule")
    require.False(t, loop.kiteClickAt.IsZero(),
        "the fresh swing of the new fight arms its own schedule")
}

// TestKiteDeferredClickDisarmsWhenTheThreatLeftTheBand pins the
// fire-time band re-check: a hostile that left the pursue band while
// the character stood the windup out (it fled, it outran the leash)
// is the stall watchdog's re-approach, not a retreat - the click
// stands down and the next shot cycle re-arms the rhythm on its own
// broadcast.
func TestKiteDeferredClickDisarmsWhenTheThreatLeftTheBand(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.False(t, loop.kiteClickAt.IsZero(),
        "the scene armed the deferred retreat")
    // The mob wandered 600 units out: beyond the 450 pursue band.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45600, Y: 50000, Name: "Keltir",
    })
    ageKiteClick(loop)
    loop.tick()
    require.Empty(t, game.walks,
        "a threat beyond the band must not click the retreat")
    require.True(t, loop.kiteClickAt.IsZero(),
        "the band exit disarms the schedule")
}

// TestKiteProximityDefersInsideTheWindup pins the phase-aware
// proximity path: a hostile closing inside the retreat radius while
// a LIVE shot cycle holds its windup (the broadcast stale - the
// shot-paced trigger already quiet, the cycle not yet over) rides
// the same deferred schedule instead of clicking into the windup -
// the immediate click would defer to the disable end and die at the
// re-shot, exactly the stall the issue #70 findings measured.
func TestKiteProximityDefersInsideTheWindup(t *testing.T) {
    bot, game, loop := kiteBowBotChaseFresh(t, 45200)
    // A shot landed 800 ms ago: stale for the shot-paced trigger
    // (the 700 ms freshness lapsed), live for the cycle (the 2 s
    // disable window still holds) - the fight view stays fresh (the
    // 3 s window of the chase step above).
    selfSwingsAt(bot, 45200)
    bot.ApplyAttackAt(shotOf(bot, 45200),
        time.Now().Add(-800*time.Millisecond))
    loop.tick()
    require.Empty(t, game.walks,
        "the proximity trigger must not click inside the windup")
    require.False(t, loop.kiteClickAt.IsZero(),
        "the proximity trigger arms the deferred schedule instead")
    ageKiteClick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the deferred click fires at the boundary")
}

// shotOf rebuilds the character's own attack broadcast against the
// given mob position (the ApplyAttackAt twin of selfSwingsAt).
func shotOf(bot *state.Bot, x int32) state.Attack {
    return state.Attack{
        AttackerID:  100,
        X:           45000,
        Y:           50000,
        Z:           -3500,
        TargetX:     x,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{stagedMobID},
        TargetCount: 1,
    }
}

// TestKiteDeferredWindowSpendsTheReuseTail pins the phase math on
// the live attack speed: the pAtkSpd 337 of the Short Bow kit puts
// the windup end at (timeAtk+reuse)/2 = 1483 ms and the disable end
// at timeAtk+reuse = 2966 ms, so the schedule clicks at 1633 ms (the
// windup end plus the 150 ms lead) and the walk window ends at the
// disable end - the walk owns the reuse tail, not the whole cycle
// (the immediate click of the old behavior owned nothing: the server
// deferred it to the very disable end where the re-shot ate it).
func TestKiteDeferredWindowSpendsTheReuseTail(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45200)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrAtkSpd, Value: 337},
    })
    shotAt := bot.SelfLastShotAt()
    loop.tick()
    // A variable (not a constant) keeps the formula a runtime
    // conversion - a constant one is not representable as the
    // integer Duration and refuses to compile.
    pAtkSpd := 337.0
    wantClick := time.Duration(
        ((500000.0 + kiteBowReuseDelay*333.0) / 2 / pAtkSpd) *
            float64(time.Millisecond))
    wantDisable := time.Duration(
        ((500000.0 + kiteBowReuseDelay*333.0) / pAtkSpd) *
            float64(time.Millisecond))
    require.WithinDuration(t, shotAt.Add(wantClick+kiteWindupLead),
        loop.kiteClickAt, 50*time.Millisecond,
        "the click lands at the windup end plus the lead")
    require.WithinDuration(t, shotAt.Add(wantDisable),
        loop.kiteClickUntil, 50*time.Millisecond,
        "the walk window ends at the disable end")
    require.Less(t, time.Until(loop.kiteClickUntil),
        wantDisable, "the window is anchored at the shot, not the tick")
}
