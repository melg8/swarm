// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The offline reproduction of the farm readiness acceptance shop
// round: a fresh level 15 elven fighter wakes at the creation point
// (46045 41251 -3440) with 20,000 sp, 100,000 adena and an empty bag -
// the injected start state of the acceptance scenario - and the hunt
// loop must run the weapon run trip on its own: walk to the weapon
// merchant of the village, arrive at the customer stand point, select
// the merchant and fire the buy requests. The simulated server walks
// the character along the clicks under the real geodata pack, the
// merchant NpcInfo lands once the character comes into the knownlist
// range, and the selection answer (MyTargetSelected) follows the
// select request the way the live server answers it.

// farmLevel15Start builds the injected start state tracker of the
// acceptance scenario. The self object id is 100 - the object id the
// simulated server applies its movement broadcasts with (see
// reproServer.advance).
func farmLevel15Start() *state.Bot {
    bot := state.NewBot("temp1")
    bot.SetCharacter("temp1", 100, 18, 46045, 41251, -3440, 280, 111)
    bot.ApplyUserInfo(state.UserInfo{
        Name: "temp1", Level: 15, ClassID: 18,
        X: 46045, Y: 41251, Z: -3440,
        Exp: 254327, Sp: 20000,
        MaxHP: 280, CurHP: 280, MaxMP: 111, CurMP: 111,
        RunSpeed: 126,
    })
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 150000001, ItemID: 57, Count: 100000, Type2: 4},
    })
    // The server answers the enter world with the (empty) skill list
    // of the fresh character: the trip trigger reads SkillsListed.
    bot.SetSkills(nil)

    return bot
}

// TestReproFarmReadinessShopRound walks the whole acceptance shop
// round offline: the weapon run trip arms, the plan crosses the
// village into the shop, the merchant gets selected and the buy
// requests leave. The start stands on the village street south of the
// shop quarter (the pond deck edge of the round 35 report): the pack
// models the creation building interior floor 64 units below the
// standing z (the vintage disagreement the live server does not
// share - the live bot walks out of the building fine), the simulated
// server refuses the interior steps, so the round starts where the
// real walk began.
func TestReproFarmReadinessShopRound(t *testing.T) {
    disablePace(t)
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := farmLevel15Start()
    bot.ApplyUserInfo(state.UserInfo{
        Name: "temp1", Level: 15, ClassID: 18,
        X: reproStuckX, Y: reproStuckY, Z: reproStuckZ,
        Exp: 254327, Sp: 20000,
        MaxHP: 280, CurHP: 280, MaxMP: 111, CurMP: 111,
        RunSpeed: 126,
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.lastHit = time.Now().Add(-time.Minute)
    sim := &reproServer{nav: engine, minZ: reproStuckZ}

    // The knownlist broadcasts of the shop quarter and the teacher:
    // the live server streams the NpcInfo once the character comes
    // into range, the reproduction sends them up front (the walk
    // machinery never reads the npc list, the talk machinery finds
    // them at the shops).
    for _, npc := range []struct {
        objectID int32
        template int32
        x, y, z  int32
        name     string
    }{
        {reproUnorenID, 7147, reproUnorenX, reproUnorenY, reproUnorenZ, "Unoren"},
        {reproUnorenID + 1, 7148, 44683, 46952, -2981, "Ariel"},
        {reproUnorenID + 2, 7149, 42700, 50057, -2984, "Creamees"},
        {reproUnorenID + 3, 7150, 42766, 50037, -2984, "Herbiel"},
        {reproUnorenID + 4, 7155, 45725, 52105, -2792, "Ellenia"},
    } {
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID:   npc.objectID,
            TemplateID: npc.template + npcDisplayOffset,
            X:          npc.x, Y: npc.y, Z: npc.z,
            Name: npc.name,
        })
    }

    deadline := time.Now().Add(90 * time.Second)
    phase := phaseEngage
    walksSeen := 0
    for time.Now().Before(deadline) {
        loop.tick()
        sim.consume(game)
        sim.advance(bot)
        // The MyTargetSelected answer of the server for every select
        // request the loop sent (the folk npc select of the shop
        // quarter: no attackable npc stands near the stops).
        for _, forced := range game.forces {
            bot.ApplySelfTarget(forced)
        }
        if loop.phase != phase {
            phase = loop.phase
            selfX, selfY, selfZ, _ := bot.SelfPosition()
            t.Logf("phase -> %s at %d %d %d (walks %d, rePaths %d)",
                phase, selfX, selfY, selfZ, len(game.walks), loop.rePaths)
            if phase == phaseTownWalk {
                for i, wp := range loop.waypoints {
                    t.Logf("  wp[%d] %.0f %.0f %.0f", i, wp.X, wp.Y, wp.Z)
                }
            }
        }
        if len(game.walks) > walksSeen {
            for ; walksSeen < len(game.walks); walksSeen++ {
                w := game.walks[walksSeen]
                t.Logf("  walk request %d -> %d %d %d",
                    walksSeen+1, w[0], w[1], w[2])
            }
        }
        if len(game.buys) > 0 {
            break
        }
        time.Sleep(100 * time.Millisecond)
    }

    selfX, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    t.Logf("end state: phase %s at %d %d %d, dist to Unoren %.0f, "+
        "walks %d, rePaths %d, buys %d, sells %d",
        loop.phase, selfX, selfY, selfZ, dist3DToUnoren(selfX, selfY, selfZ),
        len(game.walks), loop.rePaths, len(game.buys), len(game.sells))

    require.NotEmpty(t, game.buys,
        "the acceptance shop round must end in the buy request of the "+
            "weapon purchase")
}
