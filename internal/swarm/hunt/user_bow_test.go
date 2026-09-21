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

// The bow order shoots from the weapon range: with a bow equipped the
// forced attack request fires as soon as the target is inside the bow
// radius (450 units) - no melee approach walk delays the shot.
func TestUserAttackBowOrderShootsFromRange(t *testing.T) {
    bot := newTestBot()
    // The mob stands 300 units out: inside the bow radius, outside
    // the melee distance.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45300, Y: 50000, Name: "Gremlin",
    })
    // A training bow on the paperdoll (item 13, WeaponType BOW);
    // Change 1 is the upsert arm of the inventory update batch.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the first request selects the target")
    require.Empty(t, game.walks)

    bot.ApplySelfTarget(7)
    loop.userMoveAt = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, []int32{7, 7}, game.forces,
        "the bow order must force the shot from the weapon range")
    require.Empty(t, game.walks,
        "no melee approach walk may precede the bow shot")
}

// The melee order keeps the melee approach: without a bow the same
// distance closes with a walk first, the forced request waits for the
// engage radius.
func TestUserAttackMeleeOrderStillWalksFirst(t *testing.T) {
    bot := newTestBot()
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45300, Y: 50000, Name: "Gremlin",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
    loop.tick()
    require.Equal(t, []int32{7}, game.forces)

    bot.ApplySelfTarget(7)
    loop.userMoveAt = time.Now().Add(-2 * time.Second)
    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the melee order must not force the attack from 300 units")
    require.Len(t, game.walks, 1,
        "the melee order must approach the target with a walk")
}

// The bow fight stands and shoots inside the weapon range: the chase
// stall watchdog must not read the missing chase distance as a stall
// and walk the archer into melee mid fight.
func TestUserAttackBowFightInsideWeaponRangeStaysQuiet(t *testing.T) {
    bot := newTestBot()
    // The mob stands 500 units out: a healthy bow shot distance,
    // beyond the melee engage radius.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 45500, Y: 50000, Name: "Gremlin",
    })
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
    loop.tick()
    bot.ApplySelfTarget(7)

    // The fight is fresh (a swing of the character landed) and the
    // chase progress sample says the character stood still for a full
    // progress window: a melee engagement would read that as a stall.
    bot.ApplyAttack(state.Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetX: 45500, TargetY: 50000, TargetZ: -3500,
        TargetCount: 1, TargetIDs: [state.AttackTargets]int32{7},
    })
    loop.userLastDist = 500
    loop.userDistAt = time.Now().Add(-4 * time.Second)
    loop.userMoveAt = time.Now().Add(-2 * time.Second)
    loop.tick()

    require.Empty(t, game.walks,
        "a standing bow fight inside the weapon range is no stall")
    require.Len(t, game.forces, 1,
        "a running bow fight must not re-request the attack")
}

// The radius pick rides the weapon in hand: a bagged (unequipped)
// bow keeps the melee radii, an equipped bow switches both the
// engage and the stall boundary to the ranged values.
func TestEngageRadiiPickByTheWeaponInHand(t *testing.T) {
    bot := newTestBot()
    game := &fakeGame{}
    loop := NewLoop(game, bot)

    require.InDelta(t, userEngageRadius, loop.engageRadiusFor(), 0.0001,
        "a bare handed character engages at the melee distance")
    require.InDelta(t, userEngageRadius, loop.stallRadiusFor(), 0.0001,
        "a bare handed character stalls at the melee boundary")

    // A bow in the bag changes nothing: only the paperdoll counts.
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 98, ItemID: 13, Equipped: false, Change: 1},
    })
    require.InDelta(t, userEngageRadius, loop.engageRadiusFor(), 0.0001,
        "a bagged bow must not move the engage distance")

    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    require.InDelta(t, userBowEngageRadius, loop.engageRadiusFor(), 0.0001,
        "an equipped bow engages at the weapon range")
    require.InDelta(t, userBowStallRadius, loop.stallRadiusFor(), 0.0001,
        "an equipped bow stalls beyond the weapon range")
}
