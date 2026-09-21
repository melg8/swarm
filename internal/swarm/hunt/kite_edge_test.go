// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kite edge cases of the archer fight (issue #19, the third
// slice of #13): the retreat direction weighs the whole chaser train
// (the centroid away-vector), a surrounding train holds ground and
// shoots through it, the cornered kite keeps firing the bow (never a
// weapon switch, never an idle stutter), the retreat lane prefers
// the open backward lanes over the blocked corridors and re-plans a
// dead end at the next probe, and a camp on the lane deflects it onto
// the tangent ray instead of waking the mob.

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

// kiteEdgeBot builds the standard kite edge scene on top of the core
// kite scene (kiteBowBot): the bow fight runs against mob 7 closed to
// 200 units east of the character.
func kiteEdgeBot(t *testing.T) (*state.Bot, *fakeGame, *Loop) {
    t.Helper()

    return kiteBowBot(t, 45200)
}

// trainMember adds a second chaser to the scene: mob 8 stands at the
// given position and already targets the character (its attack
// broadcast carries the character as the target), so the
// SelfAttackers scan of the kite direction weighs it.
func trainMember(bot *state.Bot, x int32, y int32) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000001, Attackable: true,
        X: x, Y: y, Z: -3500, Name: "Keltir",
    })
    bot.ApplyAttack(state.Attack{
        AttackerID: 8, X: x, Y: y, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
}

// kiteCampMob spawns the aggressive Kaboo Orc Fighter camp at the
// given point under its own object id (the target and the train
// members own theirs).
func kiteCampMob(bot *state.Bot, objectID int32, x int32, y int32) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: objectID, TemplateID: 1000471, Attackable: true,
        X: x, Y: y, Z: -3500, Name: "Kaboo Orc Fighter",
    })
}

// TestKiteTrainBendsTheRetreatToTheCentroid pins the train direction
// rule: the retreat weighs EVERY chaser, not the target alone - a
// train member northeast of the character drags the centroid
// away-vector off the pure target axis, and the step lands on the
// bent ray (the endpoint keeps its distance from both the target and
// the chaser).
func TestKiteTrainBendsTheRetreatToTheCentroid(t *testing.T) {
    bot, game, loop := kiteEdgeBot(t)
    // The train member 200 units northeast: its away unit vector
    // points southwest, the target's own vector points west.
    trainMember(bot, 45141, 50141)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a closed target with a train member must trigger the step")

    // The centroid direction, recomputed from first principles:
    // away(target) = (-1, 0), away(member) = (-141, -141)/199.4.
    ax, ay := -1.0, 0.0
    bx := -141.0 / math.Hypot(141, 141)
    by := -141.0 / math.Hypot(141, 141)
    sumX, sumY := ax+bx, ay+by
    dirX := sumX / math.Hypot(sumX, sumY)
    dirY := sumY / math.Hypot(sumX, sumY)
    step := game.walks[0]
    require.InDelta(t, 45000+dirX*kiteStep, float64(step[0]), 1.0,
        "the step direction is the centroid away-vector of the train")
    require.InDelta(t, 50000+dirY*kiteStep, float64(step[1]), 1.0,
        "the step direction is the centroid away-vector of the train")
    // The bent lane still opens the distance to both chasers.
    toTarget := math.Hypot(float64(step[0]-45200), float64(step[1]-50000))
    toMember := math.Hypot(float64(step[0]-45141), float64(step[1]-50141))
    require.Greater(t, toTarget, kiteRetreatRadius,
        "the step must open the distance to the target")
    require.Greater(t, toMember, 200.0,
        "the step must open the distance to the train member")
}

// TestKiteSurroundedTrainHoldsGround pins the too-wide train rule: a
// chaser on every side cancels the away vectors, the train is too
// wide to outrun - the archer holds ground and shoots through it (no
// step, no flee answer: a winnable encircled train is a standing
// fight), and the hold paces its own probe so the fight never
// stutters on the failed geometry.
func TestKiteSurroundedTrainHoldsGround(t *testing.T) {
    bot, game, loop := kiteEdgeBot(t)
    // The target closed from the east, the train member stands 200
    // units west: away(target) + away(member) = 0 - the encircled
    // sum. Both mobs swing at the character (the fresh fighting
    // stance carries the shooting).
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 7, 45200)

    loop.tick()
    require.Empty(t, game.walks,
        "a surrounding train has no away direction - no step")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the hold belongs to the current target")
    require.False(t, loop.kiteHeldAt.IsZero(),
        "the hold must be armed")
    require.Zero(t, game.sits,
        "the surrounded archer does not sit into the blows")
    require.Zero(t, game.logouts,
        "a winnable encircled train is a standing fight, not a flee")

    // The immediate re-tick stays quiet: the hold owns the pacing
    // between probes, the fight never stutters.
    loop.tick()
    require.Empty(t, game.walks,
        "the hold owns the pacing between probes")
}

// TestKiteCorneredHoldKeepsShooting pins the walled corner rule: a
// wall behind the retreat (the geodata line of sight refuses every
// lane of the away hemisphere) stops the retreating, and the cornered
// archer KEEPS FIRING THE BOW at melee range - the archetype rule of
// #21: no weapon switch, no standing idle. The hold line lands once
// per episode, and once the fighting stance lapses the ordinary
// re-request machinery re-arms the shooting: no watchdog reads the
// cornered kite as a stall (the target never switches, no approach
// walk drags the archer INTO the melee range).
func TestKiteCorneredHoldKeepsShooting(t *testing.T) {
    _, game, loop := kiteEdgeBot(t)
    nav := &fakeNavigator{}
    // The whole away hemisphere answers walled (every retreat
    // candidate endpoint sits at or west of the character); the
    // target line to the east stays clear (the fight itself is
    // honest).
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.X > 45000, nil
    }
    loop.SetNavigator(nav)
    var logBuf bytes.Buffer
    loop.SetLogger(log.New(&logBuf, "", 0))

    loop.tick()
    require.Empty(t, game.walks,
        "a cornered archer stops retreating against the wall")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the cornered hold is armed for the target")

    // The immediate re-tick re-probes nothing (the hold pacing) and
    // the hold diagnostic names itself exactly once per episode.
    loop.tick()
    require.Empty(t, game.walks)
    require.Equal(t, 1, bytes.Count(logBuf.Bytes(), []byte("cornered")),
        "the cornered hold names itself exactly once per episode")

    // The fighting stance lapses (the walk-free hold outlives the
    // swing freshness only in the dump sense - the honest timeline
    // of a standing fight between swings): the ordinary re-request
    // re-arms the attack, the target stays, nothing walks.
    time.Sleep(3200 * time.Millisecond)
    loop.lastHit = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, int32(7), loop.target,
        "the cornered kite never switches its target away")
    require.Equal(t, []int32{7}, game.forces,
        "the cornered archer keeps shooting the bow at melee range")
    require.Empty(t, game.walks,
        "no watchdog may walk the cornered archer into its target")
}

// TestKiteWaterBehindHoldsGround pins the water corner rule: a
// retreat lane that ends over water is not a lane - swimming trades
// the bow for the paddle, the hold ground answer owns the spot.
func TestKiteWaterBehindHoldsGround(t *testing.T) {
    _, game, loop := kiteEdgeBot(t)
    loop.SetNavigator(&fakeNavigator{overWater: true})
    loop.tick()
    require.Empty(t, game.walks,
        "a retreat into water is not a walkable lane")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the water corner arms the hold")
    require.False(t, loop.kiteHeldAt.IsZero(),
        "the hold must be armed")
}

// TestKiteDeadEndLaneRePlansAtTheNextProbe pins the lane quality
// rule: a retreat whose straight lane closes ahead (the wall grew, a
// door shut) re-plans onto the open 45 degree lane at the very next
// probe - one hop later, never a walk into the dead end.
func TestKiteDeadEndLaneRePlansAtTheNextProbe(t *testing.T) {
    bot, game, loop := kiteEdgeBot(t)
    nav := &fakeNavigator{}
    loop.SetNavigator(nav)

    // The first probe: the straight west lane is open, the step
    // takes it.
    loop.tick()
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{44600, 50000, -3500}, game.walks[0],
        "the straight away lane is the lane of record")

    // The next probe cycle: the pacing windows aged out, the fight
    // view refreshed, and the straight lane now answers blocked (the
    // corridor ahead closed - the dead end the character walked
    // into).
    loop.combatAvoidUntil = time.Time{}
    loop.kiteAt = time.Now().Add(-kiteStepPeriod)
    selfSwingsAt(bot, 7, 45200)
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.X != 44600, nil
    }

    loop.tick()
    require.Len(t, game.walks, 2,
        "the dead end must re-plan, not stall the retreat")
    bent := game.walks[1]
    // The 45 degree lane southwest of the straight ray: the first
    // open candidate of the fan.
    require.InDelta(t, 45000-kiteStep*math.Sqrt2/2, float64(bent[0]), 1.0,
        "the re-plan bends onto the 45 degree lane")
    require.InDelta(t, 50000-kiteStep*math.Sqrt2/2, float64(bent[1]), 1.0,
        "the re-plan bends onto the 45 degree lane")
}

// TestKiteHalfPlaneGuardRejectsTheFoldBack pins the away half-plane
// guard of the lane battery: a lane (typically a deep camp
// deflection) may run sideways of the away-ray, but never fold back
// toward the chasing train - the guard is the hard boundary the fan
// and the deflections share.
func TestKiteHalfPlaneGuardRejectsTheFoldBack(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    // The away direction is west, no leash, no geodata runtime: the
    // half-plane gate is the only discriminator of the three lanes.
    _, _, folded := loop.kiteLaneResolve(
        45000, 50000, -3500, 45400, 50000, -1, 0)
    require.False(t, folded,
        "a lane walking east into the train is never walkable")

    _, _, sideways := loop.kiteLaneResolve(
        45000, 50000, -3500, 45000, 49600, -1, 0)
    require.True(t, sideways,
        "the perpendicular lane (dot zero) is a legal lane")

    _, _, straight := loop.kiteLaneResolve(
        45000, 50000, -3500, 44600, 50000, -1, 0)
    require.True(t, straight, "the away lane itself is a legal lane")
}

// TestKiteCampDeflectsTheRetreatLane pins the camp rule: a retreat
// lane that would steer the train through a neighboring camp (an
// idle aggressive mob inside its on-sight trigger circle) deflects
// onto the tangent ray of that circle - the kite borrows the
// aggro-aware steering of the transit walks instead of waking the
// mob onto the fight.
func TestKiteCampDeflectsTheRetreatLane(t *testing.T) {
    bot, game, loop := kiteEdgeBot(t)
    // The camp sits 424 units southwest, inside its 600 unit trigger
    // margin on the straight west retreat line (the closest point of
    // the lane clears it by 300 only).
    kiteCampMob(bot, 9, 44700, 49700)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the camp must deflect the retreat, not cancel it")

    // The side-step of the camp circle (the character starts inside
    // the margin): perpendicular to the self-to-threat axis, on the
    // side the straight lane leaned to - the northwest ray.
    step := game.walks[0]
    require.InDelta(t, 45000-kiteStep*math.Sqrt2/2, float64(step[0]), 1.0,
        "the lane deflects onto the camp circle side-step")
    require.InDelta(t, 50000+kiteStep*math.Sqrt2/2, float64(step[1]), 1.0,
        "the lane deflects onto the camp circle side-step")
}

// mobHitsCharacterAt models the attack broadcast of the target mob:
// the blow lands on the character from the given position, so the
// tracker holds the mob as a live attacker too (the surrounded train
// swings from both sides).
func mobHitsCharacterAt(bot *state.Bot, mobID int32, x int32) {
    bot.ApplyAttack(state.Attack{
        AttackerID: mobID, X: x, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
}
