// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The npc stop arrival handoff round (the 2026-09-20 owner report):
// the farm readiness acceptance stopped at the shop - the walk
// reached the merchant neighborhood but the purchase never started,
// the trip ended in the hunting phase with the character standing
// inside the shop. The walk machinery verifies the plan's final cell
// with the tight pass radius in full 3D; the live server resolves
// the clicked destination onto its own geodata surface (the pack
// holds cells tens of units off the live floor - the creation
// building interior stands 64 units off), so the final waypoint can
// verify never, the stuck ladder burns down to the trip abort and
// the sell phase never arms. The fix: the merchant stop walk hands
// over to the talk machinery the moment the character stands within
// the wide arrive radius of the stand point AND within the server
// interaction distance of the stop's merchant npc - the stop exists
// to bring the talk within reach, not to verify the pack's cell.

// armGrindingMerchantWalk arms a merchant stop walk whose plan's
// final waypoint sits out of the tight pass radius from the
// character: the walk cannot complete its verification, the handoff
// is the only way the sell phase arms.
func armGrindingMerchantWalk(t *testing.T, loop *Loop) {
    t.Helper()
    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{
        {merchant: townNpc{TemplateID: 7147, Name: "Unoren",
            X: reproUnorenX, Y: reproUnorenY, Z: reproUnorenZ},
            sell: true},
    }
    loop.segmentDest = merchantStandPoint(loop.tripStops[0].merchant)
    // The plan: the final waypoint 100 units east of the character -
    // beyond the tight pass radius, the walk verification grinds.
    loop.waypoints = []pathfind.Vec3{
        {X: float64(reproUnorenX + 100), Y: float64(reproUnorenY),
            Z: float64(reproUnorenZ)},
    }
    loop.wpIndex = 0
    loop.segmentSearch = segmentSearchView(npcApproachRadius, nil)
}

// TestMerchantStopHandsOverOnTheInteractionDistance pins the
// handoff: the walk verification grinds, the merchant npc stands
// within the interaction distance, the sell phase arms without the
// stuck ladder.
func TestMerchantStopHandsOverOnTheInteractionDistance(t *testing.T) {
    disablePace(t)
    nav := &fakeNavigator{found: true}
    bot := newTestBot()
    // The character stands at the customer stand point of Unoren;
    // the merchant spawn is 96 units away - inside the 250
    // interaction distance, outside no gate.
    moveSelfTo(bot, 44584, 46944, -2984)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID:   reproUnorenID,
        TemplateID: 7147 + npcDisplayOffset,
        X:          reproUnorenX, Y: reproUnorenY, Z: reproUnorenZ,
        Name: "Unoren",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    armGrindingMerchantWalk(t, loop)

    loop.tickTownTrip()

    require.Equal(t, phaseTownSell, loop.phase,
        "the merchant stop walk must hand over to the sell phase once "+
            "the interaction distance is met - the plan's final cell "+
            "verification must not strand the purchase")
}

// TestMerchantStopHandoffNeedsTheStandNeighborhood pins the gate: a
// character beyond the wide arrive radius of the stand point keeps
// walking (the handoff must not fire mid route).
func TestMerchantStopHandoffNeedsTheStandNeighborhood(t *testing.T) {
    disablePace(t)
    nav := &fakeNavigator{found: true}
    bot := newTestBot()
    // 400 units south of the stand point: within the merchant find
    // radius but outside the wide arrive radius of the destination.
    moveSelfTo(bot, 44584, 46544, -2984)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID:   reproUnorenID,
        TemplateID: 7147 + npcDisplayOffset,
        X:          reproUnorenX, Y: reproUnorenY, Z: reproUnorenZ,
        Name: "Unoren",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    armGrindingMerchantWalk(t, loop)

    loop.tickTownTrip()

    require.Equal(t, phaseTownWalk, loop.phase,
        "the handoff must not fire while the character is still far "+
            "from the stand point")
}

// TestMerchantStopHandoffNeedsTheMerchantNpc pins the npc gate: a
// stand point arrival without the merchant NpcInfo in the tracker
// keeps the walk machinery (the merchant wait owns the lookup).
func TestMerchantStopHandoffNeedsTheMerchantNpc(t *testing.T) {
    disablePace(t)
    nav := &fakeNavigator{found: true}
    bot := newTestBot()
    moveSelfTo(bot, 44584, 46944, -2984)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    armGrindingMerchantWalk(t, loop)

    loop.tickTownTrip()

    require.Equal(t, phaseTownWalk, loop.phase,
        "the handoff must not fire without the merchant npc in the "+
            "tracker")
}

// lureKeepBot prepares a melee fighter with the luring tool and junk
// in the bag: the worn short sword, a Short Bow (PAtk 16, the weaker
// one), a Hunting Bow (PAtk 23, the stronger one the lure would
// pick), the arrow quiver and a duplicate short sword (the surplus
// copy the planner never places).
func lureKeepBot() *state.Bot {
    bot := newTestBot()
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
        {ObjectID: 502, ItemID: 13, Count: 1},
        {ObjectID: 503, ItemID: 14, Count: 1},
        {ObjectID: 504, ItemID: 17, Count: 400},
        {ObjectID: 505, ItemID: 1, Count: 1},
    })
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 501
    bot.ApplyPaperdoll(paperdoll)

    return bot
}

// TestJunkFlowKeepsTheLuringBow pins the owner report: the bot owns
// the luring bow (bought for the covered picks) and plans no more
// expensive bow - the junk flow must never sell the strongest owned
// bow nor the arrow stacks, the weaker second bow stays plain junk.
func TestJunkFlowKeepsTheLuringBow(t *testing.T) {
    bot := lureKeepBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    junk := loop.sellableJunk()
    ids := make([]int32, 0, len(junk))
    for _, item := range junk {
        ids = append(ids, item.ObjectID)
    }
    require.NotContains(t, ids, int32(503),
        "the strongest owned bow is the luring tool - the junk flow "+
            "must never sell it")
    require.NotContains(t, ids, int32(504),
        "the arrow quiver feeds the luring - the junk flow must "+
            "never sell it")
    require.Contains(t, ids, int32(502),
        "the weaker second bow is surplus - the junk flow sells it")
    require.Contains(t, ids, int32(505),
        "the duplicate sword stays junk")
}

// TestBowLurerProfileGate pins the profile gate of the lure keeps:
// the melee fighter keeps the tool, the mystic never does.
func TestBowLurerProfileGate(t *testing.T) {
    require.True(t, gear.BowLurer(gear.MeleeFighter{}),
        "the melee fighter lures with the bow")
    require.False(t, gear.BowLurer(gear.MysticFighter{}),
        "the mystic casts from range, no luring tool")
}

// TestDestroyFlowKeepsTheLuringBow pins the overflow cleanup gate:
// the luring tool survives the bag space destroy like the sell flow.
func TestDestroyFlowKeepsTheLuringBow(t *testing.T) {
    bot := lureKeepBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{found: true})

    keeps := loop.plannedEquipKeeps()
    require.True(t, keeps[503],
        "the strongest owned bow must sit in the keep set")
    require.True(t, keeps[504],
        "the arrow quiver must sit in the keep set")
    require.False(t, keeps[502],
        "the weaker second bow is surplus - the keep set holds "+
            "nothing for it")
    require.False(t, keeps[505],
        "the duplicate sword is junk - the keep set holds nothing")
}
