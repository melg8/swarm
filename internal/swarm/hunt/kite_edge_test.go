// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kite edge cases of the archer fight (issue #19, the third
// slice of #13; the straight-lane rules of the 2026-11-09 owner
// round): the retreat direction weighs the whole chaser train (the
// centroid away-vector), a surrounding train takes the widest gap
// until the whole straight ray is walled, the cornered kite keeps
// firing the bow (never a weapon switch, never an idle stutter), a
// blocked straight lane HOLDS the ground instead of turning, a
// neighboring camp no longer bends the retreat, and the away
// half-plane guard still rejects the fold-back.

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
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "a closed target with a train member must trigger the step")

    // The centroid direction, recomputed from first principles:
    // away(target) = (-1, 0), away(member) = (-141, -141)/199.4. The
    // leg is the plain shot-cycle step, fenced by the opening ring
    // budget around the target's stand (972-200-50 = 722), which
    // fits the 400 step whole.
    ax, ay := -1.0, 0.0
    bx := -141.0 / math.Hypot(141, 141)
    by := -141.0 / math.Hypot(141, 141)
    sumX, sumY := ax+bx, ay+by
    dirX := sumX / math.Hypot(sumX, sumY)
    dirY := sumY / math.Hypot(sumX, sumY)
    leg := math.Min(kiteStep,
        kiteRoamRadius*kiteRingLeashShare-200-kiteArrivalEpsilon/2)
    step := game.walks[0]
    require.InDelta(t, 45000+dirX*leg, float64(step[0]), 1.0,
        "the step direction is the centroid away-vector of the train")
    require.InDelta(t, 50000+dirY*leg, float64(step[1]), 1.0,
        "the step direction is the centroid away-vector of the train")
    // The bent lane still opens the distance to both chasers.
    toTarget := math.Hypot(float64(step[0]-45200), float64(step[1]-50000))
    toMember := math.Hypot(float64(step[0]-45141), float64(step[1]-50141))
    require.Greater(t, toTarget, kiteRetreatRadius,
        "the step must open the distance to the target")
    require.Greater(t, toMember, 200.0,
        "the step must open the distance to the train member")
}

// TestKiteEncircledTrainBreaksThroughTheGap pins the encircled
// escape rule the live fleet round of issue #70 forced: a chaser on
// every side cancels the centroid, and the step takes the widest
// gap instead - the perpendicular bisector of a two-mob line opens
// the distance to BOTH chasers at once. The standing answer of the
// earlier rounds let the train grow unchecked on the mass cells
// (a 26 second stand measured); the hold ground rule survives only
// for a gap the whole lane battery refuses (the walled pocket -
// kite_shot_test.go pins that half).
func TestKiteEncircledTrainBreaksThroughTheGap(t *testing.T) {
    bot, game, loop := kiteEdgeBot(t)
    // The target closed from the east, the train member stands 200
    // units west: away(target) + away(member) = 0 - the encircled
    // sum. Both mobs swing at the character (the fresh fighting
    // stance carries the shooting).
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 45200)

    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "an encircled train still leaves the widest gap - one step")
    step := game.walks[0]
    // The widest gap of the east-west chaser line (bearing 0 to the
    // target, pi to the member) spans the northern half-plane and
    // its bisector is the perpendicular ray - the deterministic
    // first-widest answer, no coin flip between the two halves. The
    // leg is the plain shot-cycle step (the opening ring budget
    // 972-200-50 = 722 fits it whole), so the perpendicular escape
    // marches 400 units north.
    require.InDelta(t, 45000.0, float64(step[0]), 1.0,
        "the gap bisector runs perpendicular to the chaser line")
    require.InDelta(t, 50000.0+kiteStep, float64(step[1]), 1.0,
        "the gap bisector runs perpendicular to the chaser line")
    // The perpendicular escape opens the distance to BOTH chasers.
    toTarget := math.Hypot(
        float64(step[0]-45200), float64(step[1]-50000))
    toMember := math.Hypot(
        float64(step[0]-44800), float64(step[1]-50000))
    require.Greater(t, toTarget, 200.0,
        "the perpendicular escape opens the distance to the target")
    require.Greater(t, toMember, 200.0,
        "the perpendicular escape opens the distance to the member")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the gap answer is a step, not a hold")
    require.Zero(t, game.sits,
        "the surrounded archer does not sit into the blows")
    require.Zero(t, game.logouts,
        "a winnable encircled train is a fight, not a flee")

    // The immediate re-tick stays quiet: the walk window owns the
    // pacing, the fight never stutters.
    loop.tick()
    require.Len(t, game.walks, 1,
        "the walk window owns the pacing between probes")
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
    // The pocket is SEALED: the server-side click validation refuses
    // every straight rung, so the away hemisphere is a wall the walk
    // cannot cross (the live contract is the server answer, not the
    // bot-side line of sight - the 2026-11-09 live dump held lanes
    // the server walks).
    nav := &fakeNavigator{refuseClicks: true}
    loop.SetNavigator(nav)
    var logBuf bytes.Buffer
    loop.SetLogger(log.New(&logBuf, "", 0))

    tickPastTheWindup(loop)
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
    loop.lastActionAt = time.Now().Add(-2 * time.Second)
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
    tickPastTheWindup(loop)
    require.Empty(t, game.walks,
        "a retreat into water is not a walkable lane")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the water corner arms the hold")
    require.False(t, loop.kiteHeldAt.IsZero(),
        "the hold must be armed")
}

// TestKiteBlockedStraightLaneHoldsGround pins the owner rule of the
// 2026-11-09 round: the retreat is the STRAIGHT away-ray and nothing
// else. A wall on that ray is not re-planned onto a side candidate -
// the archer holds the ground and keeps shooting, because a turn
// spends the distance the retreat just bought.
func TestKiteBlockedStraightLaneHoldsGround(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45440)
    // The server refuses every straight rung: the lane is a genuine
    // wall, so the archer holds instead of turning. (A lane whose far
    // end alone is refused walks the shorter hop - see
    // TestKiteBlockedFarEndTakesTheShorterHop.)
    nav := &fakeNavigator{refuseClicks: true}
    loop.SetNavigator(nav)
    var logBuf bytes.Buffer
    loop.SetLogger(log.New(&logBuf, "", 0))

    tickPastTheWindup(loop)
    require.Empty(t, game.walks,
        "a blocked straight lane must hold, never bend the retreat")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the hold is armed for the target")
    require.Equal(t, 1, bytes.Count(logBuf.Bytes(), []byte("cornered")),
        "the hold names itself exactly once per episode")
}

// TestKiteBlockedFarEndTakesTheShorterHop pins the straight-hop
// ladder of the lane battery: when the server refuses the FAR end of
// the straight ray, the same ray at the shorter rungs still walks -
// the archer buys the distance it can instead of holding on an open
// runway. The live 17:43/17:44 dumps measured the old cornered hold
// firing while the lane was walkable, and this is the in-process twin:
// the full plain step is refused, the three-quarter hop lands.
func TestKiteBlockedFarEndTakesTheShorterHop(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    nav := &fakeNavigator{}
    nav.validateHook = func(
        from, to pathfind.Vec3,
    ) (pathfind.Vec3, bool) {
        if int32(math.Round(to.X)) == 44600 {
            // The far end of the opening 400-unit leg is walled.
            return from, false
        }

        return to, true
    }
    loop.SetNavigator(nav)

    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "a far-end wall still walks the shorter straight hop")
    rung := math.Round(kiteStep * 0.75)
    require.Equal(t, int32(45000-rung), game.walks[0][0],
        "the ladder takes the three-quarter rung of the same ray")
    require.Equal(t, int32(50000), game.walks[0][1],
        "the shorter hop keeps the straight away-ray")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "a walking shorter hop never arms the hold")
}

// TestKiteLiveTargetReAnchorsTheRing pins the 2026-11-09 live fix:
// the fight anchor follows the TARGET on every retreat resolution.
// The live 17:43 dump froze the anchor at the mob's first stand and
// the 250-unit step then crossed the roam radius a few legs later -
// the cornered hold fired with the mob 111 units away and the lane
// open. A moved target re-anchors the ring around its live cell, so
// the retreat keeps running instead of fencing itself off from the
// fight.
func TestKiteLiveTargetReAnchorsTheRing(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The fight ran two legs west; the stale anchor sits at the
    // mob's ORIGINAL stand while the mob caught up to the character.
    moveSelfTo(bot, 44000, 50000, -3500)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 44100, Y: 50000, Name: "Keltir",
    })
    loop.kiteFightFor = 7
    loop.kiteFightX, loop.kiteFightY = 45200, 50000

    now := time.Now()
    require.True(t, loop.kiteResolveAndClick(
        now, now.Add(3*time.Second), 44000, 50000, -3500),
        "a moved target must re-anchor the ring and walk, not hold")
    require.Len(t, game.walks, 1)
    require.Equal(t, int32(44100), loop.kiteFightX,
        "the anchor follows the live target")
    require.Equal(t, int32(50000), loop.kiteFightY,
        "the anchor follows the live target")
    require.Equal(t, int32(43600), game.walks[0][0],
        "the leg runs the plain step from the re-anchored fight")
    require.Equal(t, int32(50000), game.walks[0][1],
        "the leg keeps the straight away-ray")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the re-anchored lane never arms the hold")
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

// TestKiteGapDirectionPicksTheWidestGap pins the gap geometry of the
// encircled escape (the pure half of the rule - the behavior halves
// live in TestKiteEncircledTrainBreaksThroughTheGap and the
// kite_shot_test.go pins): two opposite chasers leave two tied 180
// degree gaps and the deterministic answer takes the first - the
// same perpendicular ray every call, no coin flip; a one-sided
// cluster leaves the wraparound gap and its bisector points away
// from the whole cluster; a single bearing pins no escape geometry.
func TestKiteGapDirectionPicksTheWidestGap(t *testing.T) {
    // Two opposite chasers (bearings 0 and pi): the first widest
    // gap spans 0 to pi, its bisector is the northern perpendicular.
    x, y, ok := kiteGapDirection([]float64{0, math.Pi})
    require.True(t, ok, "two bearings leave a gap")
    require.InDelta(t, 0.0, x, 1e-9,
        "the tied gaps resolve deterministically to the perpendicular")
    require.InDelta(t, 1.0, y, 1e-9,
        "the tied gaps resolve deterministically to the perpendicular")

    // The one-sided cluster (bearings 0, 45 and 90 degrees): the
    // wraparound gap spans 270 degrees and its bisector points at
    // 225 degrees - straight away from the cluster.
    x, y, ok = kiteGapDirection([]float64{0, math.Pi / 4, math.Pi / 2})
    require.True(t, ok, "the cluster leaves the wraparound gap")
    require.InDelta(t, math.Cos(5*math.Pi/4), x, 1e-9,
        "the wraparound bisector points away from the cluster")
    require.InDelta(t, math.Sin(5*math.Pi/4), y, 1e-9,
        "the wraparound bisector points away from the cluster")

    // The unsorted input answers the same as the sorted one (the
    // gap search sorts its own copy).
    x2, y2, ok2 := kiteGapDirection([]float64{math.Pi, 0})
    require.True(t, ok2, "the order never changes the answer")
    require.InDelta(t, 0.0, x2, 1e-9,
        "the order never changes the answer")
    require.InDelta(t, 1.0, y2, 1e-9,
        "the order never changes the answer")

    // A single bearing (or none) pins no escape geometry - the
    // caller holds ground on a degenerate scene.
    _, _, ok = kiteGapDirection([]float64{0})
    require.False(t, ok, "a single bearing leaves no gap")
    _, _, ok = kiteGapDirection(nil)
    require.False(t, ok, "no bearings leave no gap")
}

// TestKiteCampDoesNotBendTheStraightRetreat pins the removal of the
// camp deflection (the owner rule of the 2026-11-09 round): an idle
// aggressive camp beside the lane no longer bends the retreat onto a
// tangent ray. The archer runs the straight away-ray - the camp is
// accepted as the price of the straight line, because a side step
// spends the distance the shot just bought.
func TestKiteCampDoesNotBendTheStraightRetreat(t *testing.T) {
    bot, game, loop := kiteEdgeBot(t)
    // The camp sits 424 units southwest, well inside its trigger
    // margin on the straight west retreat line.
    kiteCampMob(bot, 9, 44700, 49700)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the camp must not cancel the straight retreat")
    step := game.walks[0]
    require.InDelta(t, 44600.0, float64(step[0]), 1.0,
        "the retreat keeps the straight west ray")
    require.InDelta(t, 50000.0, float64(step[1]), 1.0,
        "the retreat keeps the straight west ray")
}

// TestKiteSingleChaserNeverEncircles pins the round-19 live fix: a
// lone chaser is a plain train whatever its bearing. The encircle
// rule used to fold "the summed unit vectors are shorter than one"
// into the encirclement check, and the trigonometric sum of a single
// bearing can land one ulp under 1 (cos/sin never reconstruct the
// unit length exactly) - the round-19 acceptance held ground with
// one mob on the field twice. Only the EMPTY bearing list is the
// degenerate no-chaser scene; every non-empty list guards the away
// ray.
func TestKiteSingleChaserNeverEncircles(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45200)

    // The exact live bearing that measured hypot 0.9999999999999998
    // (the round-19 hold on the Spore Fungus fight): the away vector
    // points back along the bearing, never an encirclement.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 35216, Y: 50013, Name: "Spore Fungus",
    })
    dirX, dirY, encircled := loop.kiteTrainDirection(35271, 50029, -3515)
    require.False(t, encircled,
        "the measured live single-chaser bearing must not encircle")
    require.InDelta(t, -math.Cos(-2.858497), dirX, 0.01,
        "the away vector of the single chaser")
    require.InDelta(t, -math.Sin(-2.858497), dirY, 0.01,
        "the away vector of the single chaser")

    // The whole circle of lone bears: every bearing guards its away
    // ray, none reads as a surrounding train.
    const radius = 200.0
    for step := range 72 {
        angle := float64(step) * 2 * math.Pi / 72
        mobX := int32(45000 + radius*math.Cos(angle))
        mobY := int32(50000 + radius*math.Sin(angle))
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID: 7, TemplateID: 1000001, Attackable: true,
            X: mobX, Y: mobY, Name: "Keltir",
        })
        dirX, dirY, encircled := loop.kiteTrainDirection(
            45000, 50000, -3500)
        require.False(t, encircled,
            "one chaser at bearing %d is never an encirclement", step)
        require.InDelta(t, -math.Cos(angle), dirX, 0.02,
            "the away ray of the lone chaser at bearing %d", step)
        require.InDelta(t, -math.Sin(angle), dirY, 0.02,
            "the away ray of the lone chaser at bearing %d", step)
    }
}

// mobHitsCharacterAt models the attack broadcast of the target mob:
// the blow lands on the character from the given position, so the
// tracker holds the mob as a live attacker too (the surrounded train
// swings from both sides).
func mobHitsCharacterAt(bot *state.Bot, x int32) {
    bot.ApplyAttack(state.Attack{
        AttackerID: stagedMobID, X: x, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
}

// TestKiteKeepsKitingPastTheStaleFightFlag pins the live-fight view
// of the triggers (the 2026-11-09 acceptance run): a long retreat
// leg spends seconds walking without a swing, the strict
// SelfFighting flag goes stale after 3 s, and the old placement
// inside that branch stood the whole kite layer down mid-retreat.
// The triggers now read kiteFightLive - the own swing freshness OR
// the target holding the character OR a train member chasing - so a
// mob that keeps pursuing keeps the retreat running.
func TestKiteKeepsKitingPastTheStaleFightFlag(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The last own swing and shot are 4 s old: past the 3 s fight
    // freshness window.
    bot.ApplyAttackAt(shotOf(45200), time.Now().Add(-4*time.Second))
    // The mob keeps chasing: its server target is the character,
    // which is the live engagement of the object side.
    mobChasesSelf(bot, 7, 45200, -3500)
    require.False(t, bot.SelfFighting(7),
        "the strict fight view is stale after 4 s of walking")
    require.True(t, bot.SelfEngagedBy(7),
        "the mob still holds the character as its target")
    require.True(t, loop.kiteFightLive(),
        "the live-fight view keeps the engagement")

    loop.tick()
    require.Len(t, game.walks, 1,
        "the stale strict flag must not stand the retreat down")
    require.Less(t, game.walks[0][0], int32(45000),
        "the retreat runs straight away from the chasing mob")
}

// TestKiteFleesTheMeleeAttackerPastTheCeiling pins the policy of the
// 2026-11-10 run-28 acceptance dump: the level 10 Kaboo Orc Fighter
// of the level 7 elven mass cell sits three levels above the archer,
// and the arrows cannot win that trade at any range. Roughly half of
// the shots missed while the Fighter pair took the character from
// 100 to 59 percent of its health in sixteen seconds and the target
// lost a third of its bar. The widened kite ceiling of the earlier
// round only delayed the flight that answers this attacker, so the
// general pick ceiling rules every profile again: the targetless
// pick refuses the fight and the straight flight owns the answer.
func TestKiteFleesTheMeleeAttackerPastTheCeiling(t *testing.T) {
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 7},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000471, Attackable: true,
        X: 45200, Y: 50000, Name: "Kaboo Orc Fighter",
    })
    mobChasesSelf(bot, 7, 45200, -3500)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastActionAt = time.Now().Add(-time.Minute)
    level, ok := bot.ObjectLevel(7)
    require.True(t, ok)
    require.Equal(t, int32(10), level,
        "the Fighter carries the level 10 of the live cell")
    require.False(t, loop.attackerEngageable(7),
        "the archer never starts a fight three levels up")

    loop.tick()
    require.Zero(t, loop.target,
        "the refused Fighter keeps the target register empty")
    require.NotEmpty(t, game.walks,
        "the straight flight answers the refused attacker")
    require.Less(t, game.walks[0][0], int32(45000),
        "the flight runs away from the Fighter")
    require.Zero(t, game.logouts,
        "the healthy flight never reaches the defensive logout")

    // The anti-archer rule of the round stays: a bow wielding
    // attacker is never a kite fight (a ranged enemy cannot be
    // outrun and the standing trade answers it).
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000471, Attackable: true,
        X: 45200, Y: 50000, RightHandItem: 14, Name: "Kaboo Archer",
    })
    require.False(t, loop.attackerEngageable(8),
        "no kite answer against a bow wielding enemy")

    // A melee attacker six levels above the character is no bow
    // fight either.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 9, TemplateID: 1000013, Attackable: true,
        X: 45200, Y: 50000, Name: "High Orc",
    })
    level, ok = bot.ObjectLevel(9)
    require.True(t, ok)
    require.Equal(t, int32(13), level)
    require.False(t, loop.attackerEngageable(9),
        "a melee attacker past the pick ceiling keeps the flight")
}

// TestKiteOwnsTheAddAnswer pins the aggro answer of an add past the
// pick ceiling on a bow fight (the 2026-11-09 acceptance run: the
// level 10 add outranked the level 7 archer, the old answer dropped
// the fight and ran the melee flee fan, whose zone-clamped lanes
// zeroed on the drifted kite ground and logged the session out).
// The archer kite keeps the pick and the straight lane answers the
// add instead.
func TestKiteOwnsTheAddAnswer(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    // The self at level 10 and the add at level 16 (template
    // 1000012): six levels up, well past the general pick ceiling.
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 10},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000012, Attackable: true,
        X: 45200, Y: 50000, Name: "High Orc",
    })
    mobChasesSelf(bot, 8, 45200, -3500)
    // The own swing is stale, so the add gate runs ahead of the
    // fight branch.
    bot.ApplyAttackAt(shotOf(45440),
        time.Now().Add(-4*time.Second))
    require.False(t, loop.attackerEngageable(8))
    require.True(t, loop.kiteOwnsRetreat())

    loop.tick()
    require.Equal(t, int32(7), loop.target,
        "the kite keeps the pick instead of dropping the fight")
    require.Zero(t, game.logouts,
        "the melee flee fan must not own the kite retreat")
    require.NotEmpty(t, game.walks,
        "the straight lane answers the add")
    require.Less(t, game.walks[0][0], int32(45000),
        "the step runs away from the add")
}

// TestKitePileUpTakesTheFlight pins the pile up answer of the archer
// fight: two attackers past the pick ceiling make the pile up
// unwinnable, and the old unconditional arming ran the melee escape
// and logged the session out on the drifted kite ground. The
// standing kite is no survival answer either - the shot cycle spends
// half of every reload inside the melee range - so the fight is
// dropped and the straight flight opens the ground. The critical-HP
// logout (< 12%) stays armed above both.
func TestKitePileUpTakesTheFlight(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 10},
    })
    for i, objectID := range []int32{8, 9} {
        x := int32(45100 + i*100)
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID: objectID, TemplateID: 1000012,
            Attackable: true, X: x, Y: 50000,
            Name: "Gora Werewolf",
        })
        mobChasesSelf(bot, objectID, x, -3500)
    }
    require.False(t, loop.pileUpWinnable(),
        "the pack outranks the engage ceiling")
    require.GreaterOrEqual(t, bot.SelfAttackerCount(),
        panicLogoutAttackers, "the pack arms the pile up gate")
    require.False(t, loop.settleHolding())
    require.True(t, loop.kiteOwnsRetreat(),
        "the bow owns the running fight")
    require.True(t, loop.kiteFlightWanted(),
        "the melee pack is fleeable")

    loop.tick()
    require.Zero(t, loop.target,
        "the unwinnable pile up drops the fight")
    require.True(t, loop.panicAt.IsZero(),
        "the straight flight replaces the pile up run")
    require.Zero(t, game.logouts,
        "the archer stays in the world and flies")
    require.NotEmpty(t, game.walks,
        "the straight flight answers the pile up")
    require.Less(t, game.walks[0][0], int32(45000),
        "the flight runs away from the pack")
}

// kitePileUpScene arms the unwinnable pile up of the 2026-11-10 run-24
// dump: a bow archer on a melee pick with two adds the pick ceiling
// refuses. The pick health selects the kill-window verdict.
func kitePileUpScene(
    t *testing.T, targetHP int32,
) (*state.Bot, *fakeGame, *Loop) {
    t.Helper()
    bot, game, loop := kiteBowBot(t, 45440)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 10},
    })
    setMobHealth(bot, targetHP)
    for i, objectID := range []int32{8, 9} {
        x := int32(45100 + i*100)
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID: objectID, TemplateID: 1000012,
            Attackable: true, X: x, Y: 50000,
            Name: "Gora Werewolf",
        })
        mobChasesSelf(bot, objectID, x, -3500)
    }

    return bot, game, loop
}

// TestKitePileUpFinishesTheKillWindow pins the kill window of the
// pile up gate: the 2026-11-10 run-24 dump dropped a 24 percent grunt
// when a level 10 add joined, threw away the two arrows that finished
// it and flew into a corner pocket 30 seconds later. A target one or
// two arrows from death keeps the fight; a healthy pick of the same
// pack still converts to the survival flight.
func TestKitePileUpFinishesTheKillWindow(t *testing.T) {
    _, game, loop := kitePileUpScene(t, 24)
    require.False(t, loop.pileUpWinnable(),
        "the pack outranks the engage ceiling")
    require.True(t, loop.kiteOwnsRetreat(),
        "the bow owns the running fight")
    require.True(t, loop.kiteFlightWanted(),
        "the melee pack is fleeable")
    require.True(t, loop.kiteKillWindow(),
        "24 percent is inside the kill window")

    loop.tick()
    require.Equal(t, int32(7), loop.target,
        "the kill window keeps the fight")
    require.True(t, loop.panicAt.IsZero(),
        "the kill window never arms the pile up run")
    require.Zero(t, game.logouts,
        "the kill finishes in the world")

    _, healthyGame, healthyLoop := kitePileUpScene(t, 80)
    require.False(t, healthyLoop.kiteKillWindow(),
        "a healthy pick stands past the kill window")
    healthyLoop.tick()
    require.Zero(t, healthyLoop.target,
        "the healthy pick still converts to the flight")
    require.NotEmpty(t, healthyGame.walks,
        "the straight flight answers the healthy pile up")
}

// TestKitePileUpKeepsTheWithinCeilingPack pins the pile up tank of
// the archer: two attackers inside the general pick ceiling keep the
// bow rhythm - the arrows kill the first and the aggro answer takes
// the survivor. The 2026-11-10 run-27 acceptance window farmed this
// way for its kills. The pack with an attacker above the ceiling is
// the run-26 case below.
func TestKitePileUpKeepsTheWithinCeilingPack(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 7},
    })
    for i, objectID := range []int32{8, 9} {
        x := int32(45300 - i*100)
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID: objectID, TemplateID: 1000509,
            Attackable: true, X: x, Y: 50000, Name: "Spore Fungus",
        })
        mobChasesSelf(bot, objectID, x, -3500)
    }
    require.GreaterOrEqual(t, bot.SelfAttackerCount(),
        panicLogoutAttackers, "the pair arms the pile up gate")
    require.True(t, loop.pileUpWinnable(),
        "both attackers sit inside the pick ceiling")

    loop.tick()
    require.Equal(t, int32(7), loop.target,
        "the within-ceiling pack keeps the bow fight")
    require.True(t, loop.panicAt.IsZero(),
        "the pile up run never arms")
    require.Zero(t, game.logouts,
        "the pack fight stays online")
}

// TestKitePileUpFliesTheMixedCeilingPack pins the ceiling of the pack
// tank on the run-26 scene: the level 9 Spore Fungus pick and the
// level 10 Kaboo Orc Fighter add beside it. The Fighter sits above the
// level 7 archer's pick ceiling and the arrows cannot win that trade
// (the 2026-11-10 run-28 dump measured it), so the pair converts into
// the straight flight instead of the old widened-ceiling tank.
func TestKitePileUpFliesTheMixedCeilingPack(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 7},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000509, Attackable: true,
        X: 45300, Y: 50000, Name: "Spore Fungus",
    })
    mobChasesSelf(bot, 8, 45300, -3500)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 9, TemplateID: 1000471, Attackable: true,
        X: 45200, Y: 50000, Name: "Kaboo Orc Fighter",
    })
    mobChasesSelf(bot, 9, 45200, -3500)
    require.GreaterOrEqual(t, bot.SelfAttackerCount(),
        panicLogoutAttackers, "the pair arms the pile up gate")
    require.False(t, loop.pileUpWinnable(),
        "the above-ceiling Fighter makes the pack unwinnable")
    require.True(t, loop.kiteFlightWanted(),
        "the melee pack is fleeable")

    loop.tick()
    require.Zero(t, loop.target,
        "the mixed pack converts the standing kite into the flight")
    require.True(t, loop.panicAt.IsZero(),
        "the straight flight replaces the pile up run")
    require.Zero(t, game.logouts,
        "the flight keeps the session online")
    require.NotEmpty(t, game.walks,
        "the straight flight answers the pack")
    require.Less(t, game.walks[0][0], int32(45000),
        "the flight runs away from the pack")
}

// TestKiteFlightOwnsTheTargetlessPileUp pins the survival handoff of
// the 2026-11-10 acceptance run: the losing-fight verdict drops the
// melee pick when a bow add joins, and the targetless straight flight
// - not the zoned pile up run with its emergency logout - owns the
// escape while a melee chaser stays the nearest threat.
func TestKiteFlightOwnsTheTargetlessPileUp(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    // The nearly-dead melee pick of the run: its one-shot exemption
    // keeps the shot cycle alive until the bow add invalidates it.
    setMobHealth(bot, 1)
    // The level 8 Kaboo Orc Archer of the run joined BEHIND the melee
    // pack, so the nearest attacker stays a melee chaser and the
    // straight flight answers it.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 8, TemplateID: 1000469, Attackable: true,
        X: 45350, Y: 50000, RightHandItem: 14,
        Name: "Kaboo Orc Archer",
    })
    attackerHits(bot, 8, 45350)
    for i, objectID := range []int32{9, 11} {
        x := int32(45200 - i*100)
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID: objectID, TemplateID: 1000010,
            Attackable: true, X: x, Y: 50000, Name: "High Orc",
        })
        mobChasesSelf(bot, objectID, x, -3500)
    }
    hurtTo(bot, 50)
    require.True(t, loop.losingFight(),
        "the bow add turns the nearly-dead pick into a loss")
    require.GreaterOrEqual(t, bot.SelfAttackerCount(),
        panicLogoutAttackers, "the pack arms the pile up gate")
    require.False(t, loop.pileUpWinnable(),
        "the hurt character cannot tank the pack")

    // The first tick drops the melee pick and the targetless straight
    // flight - not the zoned pile up run - opens the escape.
    loop.tick()
    require.Zero(t, loop.target,
        "the bow add drops the melee pick")
    require.True(t, loop.kiteFlightOwnsEscape(),
        "the nearest melee chaser leaves the flight in charge")
    require.True(t, loop.panicAt.IsZero(),
        "the pile up run must not replace the flight")
    require.Zero(t, game.logouts,
        "the flight keeps the session online")
    require.NotEmpty(t, game.walks,
        "the straight lane opens the escape")
    require.Less(t, game.walks[0][0], int32(45000),
        "the walk runs away from the chasers")

    // The next tick reads the SAME targetless pile up: the gate must
    // stand down for the flight again instead of arming the logout.
    loop.tick()
    require.True(t, loop.panicAt.IsZero(),
        "the pile up gate keeps standing down")
    require.Zero(t, game.logouts,
        "the flight owns the whole escape")
}

// TestKiteFlightHoldsAgainstTheFreshAttacker pins the flight
// authority of a chase the loop already ran from: while any current
// attacker still carries the flee skip, a fresh attacker that joins
// the chase does not re-open the fight. The 2026-11-10 run-28 dump
// showed the cost - the straight flight answered one Fighter and
// every adoption of the second (or of the same one at the skip
// expiry) restarted the losing melee race mid-flight.
func TestKiteFlightHoldsAgainstTheFreshAttacker(t *testing.T) {
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrLevel, Value: 7},
    })
    // The fled chaser still holds the character.
    spawnMobAt(bot, 8, 45300)
    mobChasesSelf(bot, 8, 45300, -3500)
    // The fresh add of the same social pack closes in front of it.
    spawnMobAt(bot, 9, 45200)
    mobChasesSelf(bot, 9, 45200, -3500)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastActionAt = time.Now().Add(-time.Minute)
    loop.targetSkip = map[int32]time.Time{
        8: time.Now().Add(fleeSkipDelay),
    }
    require.True(t, loop.kiteFlightHolds(time.Now()),
        "the skipped chaser keeps the flight in charge")
    require.False(t, loop.targetSkipped(9, time.Now()),
        "the fresh add itself is not skipped")
    require.True(t, loop.attackerEngageable(9),
        "only the flight authority refuses the fresh add")

    loop.engage()
    require.Zero(t, loop.target,
        "the flight holds instead of adopting the fresh add")
    require.NotEmpty(t, game.walks,
        "the straight flight keeps answering")
}

// TestFleeSkipOutlastsTheServerAttackTimeout pins the flight budget
// against the Mobius C1 aggro clock: AttackableAI.MAX_ATTACK_TIMEOUT
// (1200 game ticks of 100 ms) drops the aggro of a mob that cannot
// land a hit two minutes after the last swing. A flee skip shorter
// than that lapses while the chaser still holds the character and the
// aggro answer re-adopts it (the 2026-11-10 run-28 dump re-adopted
// the fled Fighter at exactly two minutes).
func TestFleeSkipOutlastsTheServerAttackTimeout(t *testing.T) {
    const (
        attackableAITick      = 100 * time.Millisecond
        maxAttackTimeoutTicks = 1200
    )
    require.Greater(t, fleeSkipDelay,
        time.Duration(maxAttackTimeoutTicks)*attackableAITick,
        "the flee skip must outlast the server aggro timeout")
}

// TestKiteDisengagesTheLosingFight pins the death-risk gate of the
// archer: a hurt bow archer below the escape threshold drops the
// fight and flies the straight defensive flight. The old exemption
// (the kite kept the fight and its own retreat) measured the death
// in the 2026-11-09 5-minute acceptance run: the pursuit chain
// walked while two chasers glued at 14 units, the health fell 100 ->
// 69 -> 32 -> 21 percent and the critical-HP logout never saw a
// tick under its 12 percent line. The flight is the targetless
// straight leg, not the zone-clamped melee fan; the critical-HP
// logout stays armed above it.
func TestKiteDisengagesTheLosingFight(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    mobChasesSelf(bot, 7, 45200, -3500)
    hurtTo(bot, 20)
    require.True(t, loop.losingFight(),
        "20% health under the running fight is a loss verdict")
    require.True(t, loop.kiteOwnsRetreat())

    loop.tick()
    require.Zero(t, loop.target,
        "the death risk drops the fight")
    require.Zero(t, game.logouts,
        "20% health stays above the critical logout")
    require.NotEmpty(t, game.walks,
        "the straight flight answers the disengage")
    require.Less(t, game.walks[0][0], int32(45000),
        "the flight runs away from the chaser, not through the fan")
}

// TestKiteDisengageClearsTheServerSelection pins the second half of
// the run-31 bow band failure: the fight dropped for the flight kept
// its server side selection, and the fight state kept reporting the
// abandoned mob while the character ran. The drop sends the clear
// self click with it, so the flight reads as a flight.
func TestKiteDisengageClearsTheServerSelection(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    bot.ApplySelfTarget(7)
    mobChasesSelf(bot, 7, 45200, -3500)
    hurtTo(bot, 20)

    loop.tick()
    require.Zero(t, loop.target, "the death risk drops the fight")
    require.Equal(t, 1, game.clears,
        "the abandoned selection clears with the dropped fight")
}
