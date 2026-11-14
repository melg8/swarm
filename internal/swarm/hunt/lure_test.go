// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// lureTestCells anchors one cell at the test character position: the
// level 16 window of the Dryad ground.
func lureTestCells() []Cell {
    return []Cell{
        quadCell("lure-home", "Lure Ground", 45000, 50000, 1400, nil,
            []CellMob{
                {
                    TemplateID: 20013, Name: "Dryad",
                    Level: 13, Count: 4,
                    RespawnMin: 15, RespawnMax: 20,
                },
            }),
    }
}

// lureTestBot prepares a level 16 melee fighter standing on the lure
// ground anchor with the ranged tool in the bag: the worn short
// sword, the Short Bow and the arrow quiver.
func lureTestBot(t *testing.T) *state.Bot {
    t.Helper()
    bot := newTestBot()
    setZoneTestLevel(bot, 16)
    // setZoneTestLevel parks the character at the village: the lure
    // ground anchor takes it back.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
    })
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
        {ObjectID: 502, ItemID: 13, Count: 1},
        {ObjectID: 503, ItemID: 17, Count: 400},
    })
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 501
    bot.ApplyPaperdoll(paperdoll)

    return bot
}

// lureTestWorld places the covered pick: a passive Dryad 800 units
// east of the character and an aggressive Kaboo Orc Fighter Leader
// sharing its ground - walking to the Dryad ends inside the orc's
// on-sight circle.
func lureTestWorld(bot *state.Bot) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7001, TemplateID: npcdata.NPCWireTemplateID(20013),
        Attackable: true, X: 45800, Y: 50000, Z: -3500,
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7002, TemplateID: npcdata.NPCWireTemplateID(20472),
        Attackable: true, X: 45900, Y: 50100, Z: -3500,
    })
}

// lureConfirmBow delivers the paperdoll and inventory answer of the bow
// equip: the weapon family reads BOW and the item lands equipped.
func lureConfirmBow(bot *state.Bot) {
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 502
    bot.ApplyPaperdoll(paperdoll)
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 502, ItemID: 13, Count: 1, Equipped: true, Change: 2},
        {ObjectID: 501, ItemID: 1, Count: 1, Change: 2},
    })
}

// lureConfirmQuiver delivers the paperdoll and inventory answer of the
// quiver equip: the left hand holds the arrow stack.
func lureConfirmQuiver(bot *state.Bot) {
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 502
    paperdoll[state.PaperdollLHand] = 503
    bot.ApplyPaperdoll(paperdoll)
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 503, ItemID: 17, Count: 400, Equipped: true, Change: 2},
    })
}

// lureConfirmSword delivers the paperdoll answer of the melee swap back.
func lureConfirmSword(bot *state.Bot) {
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 501
    bot.ApplyPaperdoll(paperdoll)
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 501, ItemID: 1, Count: 1, Equipped: true, Change: 2},
        {ObjectID: 502, ItemID: 13, Count: 1, Change: 2},
    })
}

// TestLureAnswersTheCoveredPick walks the full lure state machine: the
// standoff approach with the bow armed on the walk, the shot on the bow
// range entry, the pull and the melee swap on the arrival.
func TestLureAnswersTheCoveredPick(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()
    require.Equal(t, "lure-home", loop.zonePickedID)

    // The pick happens: the dryad sits 800 units out, covered by the
    // orc. The lure takes the pick over, selects the pull target
    // ahead and arms the bow on the first walk tick.
    loop.tick()
    require.NotNil(t, loop.lure, "the covered pick must arm the lure")
    require.Equal(t, lureApproach, loop.lure.phase)
    require.Equal(t, int32(7001), loop.lure.target)
    require.Equal(t, int32(501), loop.lure.meleeObjID,
        "the worn sword is remembered for the swap back")
    require.True(t, loop.lure.selected,
        "the approach selects the pull target ahead")
    require.Equal(t, []int32{7001}, game.forces,
        "the first request only selects the pull target")
    require.Equal(t, []int32{502}, game.uses,
        "the bow arms on the walk, not at the ring")

    // The approach walks straight to the standoff ring of the target.
    require.NotEmpty(t, game.walks)
    segment := game.walks[len(game.walks)-1]
    segmentDist := math.Hypot(float64(segment[0]-45800),
        float64(segment[1]-50000))
    require.InDelta(t, lureStandoff, segmentDist, 1,
        "the walk ends on the bow range ring")

    // The paperdoll confirms the bow: the quiver rides the very next
    // tick, no flood window stands between the two requests.
    lureConfirmBow(bot)
    loop.lastActionAt = time.Now().Add(-selectPeriod)
    loop.tick()
    require.Equal(t, []int32{502, 503}, game.uses,
        "the quiver equips behind the paperdoll confirmation")

    // The quiver confirms while the dryad is still 800 out: the walk
    // runs on, the shot waits for the range.
    lureConfirmQuiver(bot)
    loop.lure.segmentAt = time.Now().Add(-lureSegmentPeriod)
    walksBefore := len(game.walks)
    loop.tick()
    require.Equal(t, lureApproach, loop.lure.phase,
        "out of range: the approach keeps the phase")
    require.Len(t, game.walks, walksBefore+1,
        "the approach keeps walking")

    // The dryad drops inside the bow range: the shot fires at once,
    // no walk closer to the old ring and no flood wait.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45400, Y: 50000, Z: -3500,
    })
    loop.lastActionAt = time.Now().Add(-engageRetryPeriod)
    walksBefore = len(game.walks)
    loop.tick()
    require.Equal(t, lureShoot, loop.lure.phase,
        "the range entry hands over to the shot phase")
    require.Len(t, game.walks, walksBefore,
        "the shooter stops at the bow range")
    require.Len(t, game.forces, 2,
        "the forced attack fires on the range entry")
    require.Equal(t, int32(7001), game.forces[1])

    // The pulled dryad runs to the shooter: at the melee range the
    // sword swap takes the tick.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45550, Y: 50000, Z: -3500,
    })
    loop.tick()
    require.NotNil(t, loop.lure)
    require.Equal(t, lureMelee, loop.lure.phase,
        "the arrival hands the tick to the melee swap")
    require.Equal(t, []int32{502, 503, 501}, game.uses,
        "the melee weapon swaps back on the arrival")

    // The paperdoll confirms the sword: the first melee request fires
    // in the same tick.
    lureConfirmSword(bot)
    loop.lastActionAt = time.Now().Add(-engageRetryPeriod)
    forcesBefore := len(game.forces)
    loop.tick()
    require.Nil(t, loop.lure, "the sword confirmation ends the lure")
    require.Len(t, game.forces, forcesBefore+1,
        "the first melee request fires the moment the sword confirms")
    require.Equal(t, int32(7001), game.forces[len(game.forces)-1])
}

// TestLureFiresFromTheBowRange pins the range gate: the forced attack
// goes out the moment the live target sits inside the 500 unit bow
// range, with the character still beyond the old 420 ring.
func TestLureFiresFromTheBowRange(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    // The lure is armed: the tool confirmed, the target selected, the
    // character 495 units from the dryad (inside the bow range, well
    // outside the old ring).
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45300, Y: 50000, Z: -3500,
    })
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45795, Y: 50000, Z: -3500,
    })
    lureConfirmQuiver(bot)
    forcesBefore := len(game.forces)
    loop.target = 7001
    loop.engageAt = time.Now()
    loop.startLure(time.Now(), 502, 503, 501, 45315, 50000, false)
    loop.lure.armedBow = true
    loop.lure.armedArrow = true
    loop.lure.selected = true
    loop.lastActionAt = time.Now().Add(-engageRetryPeriod)

    walksBefore := len(game.walks)
    loop.tick()
    require.Equal(t, lureShoot, loop.lure.phase,
        "the range entry arms the shot phase")
    require.Len(t, game.forces, forcesBefore+1,
        "the shot fires from the bow range")
    require.Equal(t, int32(7001), game.forces[forcesBefore])
    require.Len(t, game.walks, walksBefore,
        "the shooter never walks inside the range")
}

// TestLureArmsTheToolOnTheApproach pins the arming speed: the bow and
// the quiver confirm back to back while the character still walks, so
// the arrival finds the tool ready instead of arming at the ring.
func TestLureArmsTheToolOnTheApproach(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    // The pick tick already sent the bow request (the walk is 800
    // units long, the arm rides it).
    loop.tick()
    require.Equal(t, []int32{502}, game.uses)

    // The bow confirms: the quiver request follows on the next tick,
    // no two second wait, while the dryad is still out of range.
    lureConfirmBow(bot)
    loop.lastActionAt = time.Now().Add(-selectPeriod)
    loop.tick()
    require.Equal(t, []int32{502, 503}, game.uses)
    require.Equal(t, lureApproach, loop.lure.phase,
        "the tool confirms out of range, the approach runs on")

    // The quiver confirms too: the paperdoll shows the whole tool,
    // ready for the range entry.
    lureConfirmQuiver(bot)
    loop.lure.segmentAt = time.Now().Add(-lureSegmentPeriod)
    loop.tick()
    require.True(t, loop.lureToolReady(loop.lure),
        "the paperdoll shows the whole tool")
    require.Equal(t, lureApproach, loop.lure.phase,
        "the dryad is still out of range")
}

// TestLureStandsTheKiteDown pins the gate: the kite motion armed
// before the lure (a walk window, a deferred click, a hold) drops on
// the arming, and no kite layer arms while the lure runs.
func TestLureStandsTheKiteDown(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()
    lureConfirmBow(bot)
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45320, Y: 50000, Z: -3500,
    })
    loop.target = 7001

    now := time.Now()
    until := now.Add(kiteStepWindow)
    loop.combatAvoidUntil = until
    loop.kiteWalkUntil = until
    loop.kiteClickAt = now.Add(time.Second)
    loop.kiteClickFor = 7001
    loop.kiteClickUntil = until
    loop.kiteHeldAt = now
    loop.kiteHeldFor = 7001

    loop.startLure(now, 502, 503, 501, 45320, 50000, false)
    require.True(t, loop.kiteClickAt.IsZero(),
        "the deferred retreat click stands down")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the retreat walk window stands down")
    require.True(t, loop.kiteHeldAt.IsZero(),
        "the cornered hold stands down")
    require.True(t, loop.combatAvoidUntil.IsZero(),
        "the movement hold releases the forced request")
    require.False(t, loop.kiteLayerGates(now),
        "no kite layer arms while the lure holds the standoff")
}

// TestLureMeleeHandoffFiresAtOnce pins the melee handoff: the arrival
// stands the ranged tool down and the first melee request waits only
// the sword confirmation, never a kite window.
func TestLureMeleeHandoffFiresAtOnce(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    // The pull reached the melee range: the arrival stands the motion
    // down and sends the sword swap.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45320, Y: 50000, Z: -3500,
    })
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45470, Y: 50000, Z: -3500,
    })
    now := time.Now()
    loop.target = 7001
    loop.engageAt = now
    loop.lure = &lureState{
        phase: lureShoot, target: 7001, meleeObjID: 501,
        bowObjID: 502, arrowObjID: 503, startedAt: now,
    }
    walksBefore := len(game.walks)
    usesBefore := len(game.uses)

    loop.tick()
    require.NotNil(t, loop.lure)
    require.Equal(t, lureMelee, loop.lure.phase,
        "the arrival hands the tick to the melee swap")
    require.Len(t, game.uses, usesBefore+1)
    require.Equal(t, int32(501), game.uses[usesBefore],
        "the sword swap takes the arrival")
    require.Len(t, game.walks, walksBefore,
        "the retreat stops with the melee handoff")

    // The sword confirms: the first melee request fires in the same
    // tick instead of waiting a kite window out.
    lureConfirmSword(bot)
    loop.lastActionAt = time.Now().Add(-engageRetryPeriod)
    forcesBefore := len(game.forces)
    loop.tick()
    require.Nil(t, loop.lure)
    require.Len(t, game.forces, forcesBefore+1,
        "the first melee request follows the sword confirmation")
    require.Equal(t, int32(7001), game.forces[forcesBefore])
}

// TestLureHandoffIgnoresTheAddHold pins the hold contract: the
// impending-add step pauses the approach walk, but the range entry and
// the arrival swap never wait it out.
func TestLureHandoffIgnoresTheAddHold(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    // The tool is ready and the dryad out of range while the add step
    // holds the movement: the approach walks nothing.
    lureConfirmQuiver(bot)
    loop.target = 7001
    loop.engageAt = time.Now()
    loop.startLure(time.Now(), 502, 503, 501, 45320, 50000, false)
    loop.lure.armedBow = true
    loop.lure.armedArrow = true
    loop.lure.selected = true
    loop.combatAvoidUntil = time.Now().Add(combatAvoidStepPeriod)
    walksBefore := len(game.walks)
    loop.tick()
    require.Equal(t, lureApproach, loop.lure.phase)
    require.Len(t, game.walks, walksBefore,
        "the add hold pauses the approach walk")

    // The dryad walks into the bow range inside the same hold: the
    // shot phase still arms.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45400, Y: 50000, Z: -3500,
    })
    loop.tick()
    require.Equal(t, lureShoot, loop.lure.phase,
        "the range entry ignores the movement hold")

    // The pull reaches the melee range: the swap takes the tick.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45150, Y: 50000, Z: -3500,
    })
    usesBefore := len(game.uses)
    loop.tick()
    require.Equal(t, lureMelee, loop.lure.phase,
        "the arrival swap ignores the movement hold")
    require.Len(t, game.uses, usesBefore+1)
    require.Equal(t, int32(501), game.uses[usesBefore])
}

// TestLureHoldsThePositionWhileThePullRuns pins the holding rule: the
// pulled mob is still closing beyond the melee range - the chase
// watchdog must not walk the shooter toward it.
func TestLureHoldsThePositionWhileThePullRuns(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    // The pull is running: the dryad at 700 units and closing, the
    // lure armed in the shoot phase, the fight view fresh.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45100, Y: 50000, Z: -3500,
    })
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45800, Y: 50000, Z: -3500,
    })
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrCurHP, Value: 90},
    })
    bot.ApplySelfTarget(7001)
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7001, Distance: 700,
        X: 45100, Y: 50000, Z: -3500,
        TargetX: 45800, TargetY: 50000, TargetZ: -3500,
    })
    require.True(t, bot.SelfFighting(7001))
    loop.target = 7001
    loop.engageAt = time.Now()
    loop.lure = &lureState{
        phase: lureShoot, target: 7001, meleeObjID: 501,
        bowObjID: 502, arrowObjID: 503, startedAt: time.Now(),
    }
    walksBefore := len(game.walks)
    usesBefore := len(game.uses)
    forcesBefore := len(game.forces)
    loop.tick()
    require.NotNil(t, loop.lure, "the pull stays armed")
    require.Equal(t, lureShoot, loop.lure.phase)
    require.Len(t, game.walks, walksBefore,
        "the shooter holds the position while the pull closes")
    require.Len(t, game.uses, usesBefore, "no swap before the arrival")
    require.Len(t, game.forces, forcesBefore,
        "the ranged fight needs no forced request while it runs")
}

// TestLureAbortsWhenThePullNeverEngages pins the timeout: a mob that
// never answers the pull releases the target to the skip machinery.
func TestLureAbortsWhenThePullNeverEngages(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    loop.target = 7001
    loop.engageAt = time.Now()
    loop.lure = &lureState{
        phase: lureShoot, target: 7001, meleeObjID: 501,
        bowObjID: 502, arrowObjID: 503,
        startedAt: time.Now().Add(-(lureTimeout + time.Second)),
    }
    loop.tick()
    require.Nil(t, loop.lure, "the timed out lure aborts")
    require.Equal(t, int32(0), loop.target, "the aborted target drops")
    require.NotEmpty(t, loop.targetSkip[7001],
        "the aborted target skips for a while")
}

// TestUncoveredPickHuntsNormally pins the gate: a pick that stands in
// the open never arms the lure, the ordinary engage answers it.
func TestUncoveredPickHuntsNormally(t *testing.T) {
    bot := lureTestBot(t)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7001, TemplateID: npcdata.NPCWireTemplateID(20460),
        Attackable: true, X: 45600, Y: 50000, Z: -3500,
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    loop.tick()
    require.Nil(t, loop.lure, "an open pick never arms the lure")
    require.Equal(t, int32(7001), loop.target)
    require.NotEmpty(t, game.forces, "the ordinary attack fires")
}

// TestLureNeedsTheTool pins the ownership gate: without the bow (or
// without the arrows) the covered pick falls back to the ordinary
// engage.
func TestLureNeedsTheTool(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
        {ObjectID: 503, ItemID: 17, Count: 400},
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(lureTestCells())
    loop.tick()

    loop.tick()
    require.Nil(t, loop.lure, "no bow, no lure")
    require.NotEmpty(t, game.forces, "the ordinary attack answers")
}
