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

// lureTestSpots anchors one spot at the test character position: the
// level 6 window of the Crimson Spider ground.
func lureTestSpots() []Spot {
    return []Spot{
        {
            ID: "lure-home", Name: "Lure Ground", Region: "elven",
            MinLevel: 1, MaxLevel: 20,
            AnchorX: 45000, AnchorY: 50000, Radius: 2048,
            RespawnMin: 15, RespawnMax: 20, Mass: 4,
            Mobs: []SpotMob{
                {
                    TemplateID: 20013, Name: "Dryad",
                    Level: 13, Count: 4,
                    RespawnMin: 15, RespawnMax: 20,
                },
            },
        },
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

// TestLureAnswersTheCoveredPick walks the full lure state machine:
// the standoff approach, the bow and quiver equips, the pull and the
// melee swap on the arrival.
func TestLureAnswersTheCoveredPick(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingSpots(lureTestSpots())
    loop.tick()
    require.Equal(t, "lure-home", loop.zonePickedID)

    // The pick happens: the spider sits 800 units out, covered by the
    // orc. The lure takes the pick over - no forced attack yet.
    loop.tick()
    require.NotNil(t, loop.lure, "the covered pick must arm the lure")
    require.Equal(t, lureApproach, loop.lure.phase)
    require.Equal(t, int32(7001), loop.lure.target)
    require.Equal(t, int32(501), loop.lure.meleeObjID,
        "the worn sword is remembered for the swap back")

    // The approach walks toward the standoff (one paced leg).
    require.NotEmpty(t, game.walks)
    leg := game.walks[len(game.walks)-1]
    legDist := math.Hypot(float64(leg[0]-45800), float64(leg[1]-50000))
    require.InDelta(t, lureStandoff, legDist, 260,
        "the approach leg aims at the standoff ring of the target")
    require.Empty(t, game.forces, "no forced attack during the approach")

    // The character arrives at the standoff: the approach hands the
    // phase over, the next tick equips the bow.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: loop.lure.standX, Y: loop.lure.standY, Z: -3500,
    })
    loop.lure.legAt = time.Now().Add(-lureLegPeriod)
    loop.tick()
    require.Equal(t, lureArm, loop.lure.phase)
    loop.tick()
    require.Equal(t, []int32{502}, game.uses, "the bow equips first")

    // The server confirms the bow: the quiver rides the left hand.
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 502
    bot.ApplyPaperdoll(paperdoll)
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 502, ItemID: 13, Count: 1, Equipped: true, Change: 2},
        {ObjectID: 501, ItemID: 1, Count: 1, Change: 1},
    })
    loop.lure.armAt = time.Now().Add(-lureArmPeriod)
    loop.tick()
    require.Equal(t, []int32{502, 503}, game.uses,
        "the quiver equips behind the bow")

    // The arm completes: the pull fires through the ordinary forced
    // attack request.
    bot.ApplyPaperdoll(func() [state.PaperdollSlots]int32 {
        doll := paperdoll
        doll[state.PaperdollLHand] = 503

        return doll
    }())
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 503, ItemID: 17, Count: 400, Equipped: true, Change: 2},
    })
    loop.lure.armAt = time.Now().Add(-lureArmPeriod)
    loop.tick()
    require.NotNil(t, loop.lure)
    require.Equal(t, lureShoot, loop.lure.phase,
        "the armed lure hands the flow back to the attack path")
    require.NotEmpty(t, game.forces, "the pull fires the forced attack")
    require.Equal(t, int32(7001), game.forces[len(game.forces)-1])

    // The pulled Dryad runs to the shooter: once it closes to the
    // melee range the sword swaps back and the fight continues the
    // ordinary way. The attack retry pacing of the pull stamps
    // lastHit - the arrival tick waits out its window.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: loop.lure.standX + 150, Y: loop.lure.standY,
        Z: -3500,
    })
    loop.lastHit = time.Now().Add(-engageRetryPeriod)
    loop.tick()
    require.Nil(t, loop.lure, "the arrival ends the lure")
    require.Equal(t, []int32{502, 503, 501}, game.uses,
        "the melee weapon swaps back on the arrival")
}

// TestLureHoldsThePositionWhileThePullRuns pins the holding rule: the
// pulled mob is still closing (outside the melee range) - the chase
// watchdog must not walk the shooter into the cover.
func TestLureHoldsThePositionWhileThePullRuns(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingSpots(lureTestSpots())
    loop.tick()

    // The pull is running: the spider at 300 units and closing, the
    // lure armed in the shoot phase, the fight view fresh.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45100, Y: 50000, Z: -3500,
    })
    bot.ApplyPlacement(state.Placement{
        ObjectID: 7001, X: 45280, Y: 50000, Z: -3500,
    })
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrCurHP, Value: 90},
    })
    loop.target = 7001
    loop.engageAt = time.Now()
    loop.lure = &lureState{
        phase: lureShoot, target: 7001, meleeObjID: 501,
        bowObjID: 502, arrowObjID: 503,
        startedAt: time.Now(),
    }
    walksBefore := len(game.walks)
    loop.tick()
    require.Nil(t, loop.lure)
    require.Len(t, game.walks, walksBefore,
        "the shooter holds the position while the pull closes")
    require.NotEmpty(t, game.uses)
    require.Equal(t, int32(501), game.uses[len(game.uses)-1],
        "the arrival swapped the melee weapon back")
}

// TestLureAbortsWhenThePullNeverEngages pins the timeout: a mob that
// never answers the pull releases the target to the skip machinery.
func TestLureAbortsWhenThePullNeverEngages(t *testing.T) {
    bot := lureTestBot(t)
    lureTestWorld(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingSpots(lureTestSpots())
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
    loop.SetHuntingSpots(lureTestSpots())
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
    loop.SetHuntingSpots(lureTestSpots())
    loop.tick()

    loop.tick()
    require.Nil(t, loop.lure, "no bow, no lure")
    require.NotEmpty(t, game.forces, "the ordinary attack answers")
}
