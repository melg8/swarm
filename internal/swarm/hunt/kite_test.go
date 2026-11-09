// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The kite behavior of the archer fight (issue #13): a bow user that
// fights a mob steps clear when a hostile closes inside the retreat
// radius (the fight target or a train member - the direction weighs
// the whole train, see the centroid tests), the movement window
// pauses the forced attack re-requests, and the shooting resumes
// once the step finished. The tests pin the step geometry, the triggers that stay quiet, the
// streak limit and the re-request resume.

// kiteBowBot builds the standard kite scene: a healthy character at
// 45000/50000 with a training bow worn (item 13, WeaponType BOW),
// the loop in the running fight against the given mob.
func kiteBowBot(t *testing.T, mobX int32) (*state.Bot, *fakeGame, *Loop) {
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
    // The fight runs: the character swings at the mob (the running
    // fight view of SelfFighting).
    selfSwingsAt(bot, mobX)
    loop.lastHit = time.Now().Add(-time.Minute)

    return bot, game, loop
}

// TestKiteStepsAwayFromTheClosedTarget pins the core behavior: a bow
// target inside the retreat radius (250) makes the fighting character
// walk straight away from it - the RACE leg of the round-17 redesign
// (kiteStepLength): the deficit to the re-shot floor (480-200 = 280)
// buys back at the parity exchange rate (400 + 280*8 = 2640), the
// absolute cap fences it at kiteRaceStepMax (1700) and the fresh
// fight's own opening ring budget (the leash share of the roam radius
// around the MOB'S stand minus the stand distance and the
// cell-rounding guard - 972-200-50 = 722) caps the leg there: the
// opening walk lands the character on the ring the fight keeps
// instead of the old fence-edge pocket. The scene carries a fresh
// shot, so the first tick arms the deferred retreat (the windup hold
// of the issue #70 findings) and the aged tick fires it - the walk
// asserts the geometry once the click landed.
func TestKiteStepsAwayFromTheClosedTarget(t *testing.T) {
    // The mob closed to 200 units: inside the kite trigger, outside
    // the melee range.
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "a closed bow target must trigger the kite step")
    step := game.walks[0]
    _, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    require.Equal(t, selfZ, step[2],
        "the step keeps the character's deck")
    // The step direction: straight away from the target on the x
    // axis - the race leg capped at the fresh opening ring budget
    // (min(400+280*8, 1700, 972-200-50) = 722) from the self
    // position.
    require.Equal(t, int32(44278), step[0])
    require.Equal(t, selfY, step[1])
    require.Empty(t, game.forces,
        "the kite tick must not re-request the attack")
}

// TestKiteStaysQuietAtWeaponRange pins the quiet case: a bow target
// beyond the retreat radius is the good standing fight - no step, no
// re-request (the chase stall watchdog stays quiet inside the weapon
// range too, see the bow radii tests of user_bow_test.go).
func TestKiteStaysQuietAtWeaponRange(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45500)
    loop.tick()
    require.Empty(t, game.walks,
        "a standing bow fight inside the weapon range must not kite")
    require.Empty(t, game.forces,
        "a running bow fight must not re-request the attack")
}

// TestKiteIsBowOnly pins the weapon gate: a melee fighter with the
// same closed target stays in the fight - the kite is archer
// behavior. The melee answer of a target beyond the swing distance
// (200 units against the 150 unit melee stall radius) is the ordinary
// chase-stall approach walk TOWARD the target, never a retreat.
func TestKiteIsBowOnly(t *testing.T) {
    bot := newTestBot()
    // No bow on the paperdoll: the bare fists or a melee weapon.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45200, Y: 50000, Name: "Keltir",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.engageAt = time.Now()
    selfSwingsAt(bot, 45200)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    selfX := int32(45000)
    for _, walk := range game.walks {
        require.Greater(t, walk[0], selfX,
            "a melee fight may only close on its target, never retreat")
    }
}

// TestKiteRefusesTheArcherEnemy pins the enemy weapon gate (the
// owner rule of the 2026-11-09 round): a bow wielding fight target
// closes every kite layer - the proximity step and the shot-paced
// retreat alike. A ranged enemy cannot be outrun, the retreat only
// spends the bow cooldown on walking while the enemy keeps shooting,
// so the standing archer trade is the answer. The melee-target
// scenes above remain the positive control.
func TestKiteRefusesTheArcherEnemy(t *testing.T) {
    archerTarget := state.NpcInfo{
        ObjectID: 7, TemplateID: 1007218, Attackable: true,
        X: 45200, Y: 50000, RightHandItem: 276, Name: "Kendell",
    }

    t.Run("proximity layer", func(t *testing.T) {
        bot, game, loop := kiteBowBotChaseFresh(t, 45200)
        bot.ApplyNpcInfo(archerTarget)

        loop.tick()
        require.Empty(t, game.walks,
            "the archer enemy must never arm the proximity step")
        require.Empty(t, game.forces,
            "the standing fight keeps the attack, no retreat")
    })

    t.Run("shot paced layer", func(t *testing.T) {
        bot, game, loop := kiteBowBot(t, 45200)
        bot.ApplyNpcInfo(archerTarget)

        loop.tick()
        require.Empty(t, game.walks,
            "the archer enemy must never arm the shot-paced step")
        require.True(t, loop.kiteClickAt.IsZero(),
            "the shot broadcast must not arm the deferred retreat")
    })
}

// TestKiteWindowHoldsTheReRequests pins the movement window contract:
// while the kite step owns the tick (the walk runs), the engage holds
// its forced attack re-requests even after the fighting stance lapsed
// - a request would interrupt the running retreat walk server-side.
// The window is armed with the deferred schedule (combatAvoidUntil
// stretches to the shot disable end the moment the windup defers the
// click), so the hold covers the windup AND the walk.
func TestKiteWindowHoldsTheReRequests(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The fighting stance lapsed while the character walks (no fresh
    // swings), but the step window still owns the movement: no
    // forced attack may interrupt the retreat.
    loop.tick()
    require.Empty(t, game.forces,
        "the kite window must hold the attack re-requests")
    require.Len(t, game.walks, 1,
        "the pacing period bounds the steps to one per window")
}

// TestKiteLeashReadsTheFightAnchor pins the location-general leash
// of the round-14 redesign: the retreat endpoint must stay inside
// the fight's own roam radius around the FIGHT ANCHOR - the
// live-observed position where the fight's first retreat resolved
// - never the generated hunting-square registry (the zone read is
// gone: the kite works on any ground, mapped or not, and a square
// that would cover the endpoint no longer rescues it).
func TestKiteLeashReadsTheFightAnchor(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.target = 7
    loop.kiteFightFor = 7
    loop.kiteFightX, loop.kiteFightY = 45000, 50000

    // The endpoint inside the roam radius: a legal lane.
    _, _, inside := loop.kiteTerrainLane(
        45000, 50000, -3500,
        45000+int32(kiteRoamRadius)-100, 50000)
    require.True(t, inside,
        "the endpoint inside the roam radius walks")

    // The endpoint beyond the roam radius: the leash refuses it
    // even with a hunting square that covers it - the fight's own
    // ground is the only fence.
    loop.SetHuntingZone(45000, 50000, 60000)
    _, _, outside := loop.kiteTerrainLane(
        45000, 50000, -3500,
        45000+int32(kiteRoamRadius)+100, 50000)
    require.False(t, outside,
        "the roam radius fences the step even inside a hunting square")
}

// TestKiteStreakLimitNeverStopsTheShuffle pins the round-14
// always-run rule: the streak limit is a DIAGNOSTIC, not a stop - a
// chaser at least as fast as the character never falls behind, but
// the answer is the running straight line (the melee damage the kite
// exists to avoid), never the standing "fight it out" trade. Past
// the limit the proximity path keeps stepping. The fight freshness
// rides the chase step (no shot committed): the shot-paced rhythm
// owns its own layer (kite_shot_test.go pins the exemption).
func TestKiteStreakLimitNeverStopsTheShuffle(t *testing.T) {
    _, game, loop := kiteBowBotChaseFresh(t, 45200)
    loop.kiteFor = 7
    loop.kiteStreak = kiteStreakLimit
    loop.tick()
    require.NotEmpty(t, game.walks,
        "a target past the streak limit must keep running - the "+
            "standing trade is the melee damage the kite avoids")
}

// TestKiteStreakResetsForAFreshTarget pins the streak bookkeeping:
// the limit counts the steps of ONE target - a fresh target (the
// previous one died, the pick moved on) starts its own count. The
// chase-fresh scene keeps the shot-paced rhythm out of the answer.
func TestKiteStreakResetsForAFreshTarget(t *testing.T) {
    _, game, loop := kiteBowBotChaseFresh(t, 45200)
    // The streak of the previous target exhausted the budget...
    loop.kiteFor = 5
    loop.kiteStreak = kiteStreakLimit
    // ...but the current target is a fresh one.
    loop.tick()
    require.Len(t, game.walks, 1,
        "a fresh target must reset the kite streak")
}

// mobChasesSelf makes the npc of objectID chase the character from
// x: the attack broadcast the server sends for the mob's swings
// (AttackerID the mob, TargetIDs the character) - the state reads the
// npc as a self attacker (TargetID self, the NearestAttacker scan).
func mobChasesSelf(bot *state.Bot, objectID int32, x, z int32) {
    bot.ApplyAttack(state.Attack{
        AttackerID:  objectID,
        X:           x,
        Y:           50000,
        Z:           z,
        TargetX:     45000,
        TargetY:     50000,
        TargetZ:     -3500,
        TargetIDs:   [state.AttackTargets]int32{100},
        TargetCount: 1,
    })
}

// TestKiteTrainMemberArmsTheRetreat pins the train-member trigger of
// the issue: a mob that chases the character (it targets self) arms
// the retreat even while the fight target stays at range - the step
// goes away from the CLOSER threat (the chasing member), not the
// target the fight runs on. The fresh shot of the scene defers the
// click past the windup; the aged tick fires it.
func TestKiteTrainMemberArmsTheRetreat(t *testing.T) {
    // The fight target holds 440 units out: inside the bow engage
    // radius (450), outside the retreat radius (250) - the target
    // alone is the quiet standing fight.
    bot, game, loop := kiteBowBot(t, 45440)
    // A second mob aggros and closes to 200 units: a chase the
    // character did not start (the server target of the mob is the
    // character).
    spawnMobAt(bot, 8, 45200)
    mobChasesSelf(bot, 8, 45200, -3500)

    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "a chasing train member inside the retreat radius must arm the kite")
    step := game.walks[0]
    // The step direction: straight away from the MEMBER on the x
    // axis (self 45000, member 45200). The fight anchored on the
    // TARGET's stand at 45440, so the opening ring budget is
    // 972-440-50 = 482 west of the self, not away from the member's
    // distance.
    require.Equal(t, int32(44518), step[0])
    require.Empty(t, game.forces,
        "the kite tick must not re-request the attack")
}

// TestKiteSkipsADeckGapTrainMember pins the deck guard of the train
// member trigger: a mob on a deck the walk cannot reach (the z gap
// past deckReachableZ) is no melee threat - it must not arm the
// kite, the standing fight on the ranged target goes on. The
// chase-fresh scene keeps the shot-paced rhythm out of the answer
// (a fresh shot with the target inside the band would retreat).
func TestKiteSkipsADeckGapTrainMember(t *testing.T) {
    bot, game, loop := kiteBowBotChaseFresh(t, 45440)
    // The chasing member sits 500 units BELOW the character's deck
    // (-3000 against -3500): past the deckReachableZ (400) gap, its
    // swings cannot reach.
    spawnMobAt(bot, 8, 45200)
    mobChasesSelf(bot, 8, 45200, -3000)

    loop.tick()
    require.Empty(t, game.walks,
        "a deck gap member must not arm the kite step")
    require.Empty(t, game.forces,
        "the standing bow fight keeps its re-request quiet")
}

// TestKiteDirectionWeighsTheWholeTrain pins the direction choice of
// a multi-chaser fight (the edge case slice #19): the retreat
// direction is the centroid away-vector of EVERY chaser, not the
// armed threat alone. The fight target closes at 240 units east, a
// train member chases at 180 units northeast - the centroid bends
// the retreat west-southwest, away from both, instead of the pure
// west of the target-only vector (and a train on OPPOSITE sides
// cancels the vectors entirely - the encircled hold - which the
// surrounded case of kite_edge_test.go pins). The fresh shot of the
// scene defers the click past the windup; the aged tick fires it
// and the geometry reads off the fired walk.
func TestKiteDirectionWeighsTheWholeTrain(t *testing.T) {
    // The fight target closes at 240 units east: inside the radius,
    // a step away from it alone would land west (x 44600).
    bot, game, loop := kiteBowBot(t, 45240)
    // The train member chases at 180 units northeast (self-relative
    // dx 127, dy 127): its away vector points southwest, the
    // centroid drags the retreat off the pure west axis.
    trainMember(bot, 45127, 50127)

    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the closed hostiles must arm the kite")
    step := game.walks[0]
    // The centroid direction: away(target) = (-1, 0),
    // away(member) = (-127, -127)/179.6 -> the sum normalized. The
    // leg is the race step of the nearer threat (the member at 180:
    // 400 + 300*8, capped at the opening ring budget around the
    // TARGET's stand - 972-240-50 = 682).
    sumX := -1.0 - 127.0/math.Hypot(127, 127)
    sumY := -127.0 / math.Hypot(127, 127)
    sumLen := math.Hypot(sumX, sumY)
    leg := kiteStep + (kiteReshotFloor-180.0)*kiteRaceFactor
    if leg > kiteRaceStepMax {
        leg = kiteRaceStepMax
    }
    if budget := kiteRoamRadius*kiteRingLeashShare -
        240 - kiteArrivalEpsilon/2; budget < leg {
        leg = budget
    }
    require.InDelta(t, 45000+sumX/sumLen*leg, float64(step[0]), 1.0,
        "the step direction is the centroid away-vector of the train")
    require.InDelta(t, 50000+sumY/sumLen*leg, float64(step[1]), 1.0,
        "the step direction is the centroid away-vector of the train")
    require.Less(t, step[1], int32(50000),
        "the northeast member must drag the retreat south of the "+
            "pure target axis")
}
