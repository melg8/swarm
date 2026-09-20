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

// The zone return stuck regression of 2026-09-11 06:00: the dump
// (build c1faefb, bot unittest3, phase engage) showed the bot at
// 43000 50184 -2992 (near Herbiel, outside the zone) for 3 minutes.
// Events: "no dry path to 38553 50080, the walk would swim" repeated,
// then a learning trip started and also failed with "no dry path to
// 42766 50037" (Herbiel is only 276 units away). Two bugs:
//  1. The town trip started while the bot was outside the zone,
//     interfering with the zone return.
//  2. The zone return had no non-dry fallback: when the dry search
//     failed, it fell back to walkZoneSegment (direct walks) which may
//     cross water/walls and get refused by the server.

// TestTownTripBlockedOutsideTheZone pins fix #1: a bot outside the
// hunting zone with a learning budget does NOT start a town trip -
// the zone return owns the walk until the bot is back in the zone.
// The 2026-09-11 06:00 dump showed a learning trip starting at the
// village (outside the zone) before the zone return, both searches
// failed, and the bot never moved.
func TestTownTripBlockedOutsideTheZone(t *testing.T) {
    loop, _, bot := newLearnLoop(500)
    // The bot stands outside the zone (the zone center is 45000 50000,
    // the bot at 43000 50184 is well outside the 1448 half square).
    moveSelfTo(bot, 43000, 50184, -2992)
    // Set up the zone (the test bot starts at 45000 50000 which is
    // the zone center, so move it out and set the zone explicitly).
    loop.zoneCX = 45000
    loop.zoneCY = 50000
    loop.zoneHalf = 1448

    loop.tick()

    require.Empty(t, loop.tripStops,
        "no town trip stops while the bot is outside the zone")
    require.True(t, loop.zoneReturn,
        "the zone return is armed instead")
}

// TestTownTripStartsInsideTheZone pins the complement: a bot inside
// the zone with a learning budget starts the town trip normally.
func TestTownTripStartsInsideTheZone(t *testing.T) {
    loop, _, bot := newLearnLoop(500)
    // The bot stands at the zone center (inside the zone).
    moveSelfTo(bot, 45000, 50000, -3500)
    loop.zoneCX = 45000
    loop.zoneCY = 50000
    loop.zoneHalf = 1448

    loop.tick()

    require.True(t, loop.tripActive(),
        "the town trip starts inside the zone")
    require.False(t, loop.zoneReturn,
        "no zone return while the bot is in the zone")
}

// TestTownTripWeaponRunStartsOutsideTheZone pins the weapon run
// exception: a bare-handed character shops for a weapon at once,
// even outside the zone (punching mobs through the walk home is
// worse than a late return).
func TestTownTripWeaponRunStartsOutsideTheZone(t *testing.T) {
    bot := newTestBot()
    // Level 5, no weapon, enough adena for a Short Sword.
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest1", Level: 5, ClassID: 18, Race: 1, Sp: 100,
    })
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 142, Level: 1, Passive: true},
        {SkillID: 194, Level: 1, Passive: true},
    })
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrMaxHP, Value: 100},
        {ID: state.AttrCurHP, Value: 90},
    })
    // Give the bot enough adena for a weapon but no weapon equipped.
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 500, ItemID: 57, Count: 2000, Type2: 5, Change: 1},
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(&fakeNavigator{found: true})
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.zoneCX = 45000
    loop.zoneCY = 50000
    loop.zoneHalf = 1448
    moveSelfTo(bot, 43000, 50184, -2992)

    loop.tick()

    require.True(t, loop.tripActive(),
        "the weapon run starts even outside the zone")
}

// TestZoneReturnPlansThroughThePricedSearch pins the zone return
// planning: the priced approach search runs once and the walk arms on
// its answer - no walled form of the water exists anymore, the search
// either finds the walk or the zone return falls back to the direct
// walks.
func TestZoneReturnPlansThroughThePricedSearch(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    // The bot stands outside the zone.
    moveSelfTo(bot, 43000, 50184, -2992)
    loop.zoneCX = 38553
    loop.zoneCY = 50080
    loop.zoneHalf = 1448
    nav.found = true

    loop.tick()

    require.True(t, loop.zoneReturn,
        "the zone return is armed")
    require.Equal(t, phaseTownReturn, loop.phase,
        "the zone return is walking (the priced search found a path)")
    require.Equal(t, 1, nav.calls,
        "the priced search runs exactly once")
}

// TestZoneReturnDryFailureHoldsTheReturn pins the no-route rule: when
// the priced search fails, the zone return HOLDS instead of marching
// the direct segments toward the zone center (the owner rule of the
// 2026-09-19 round: НИКОГДА не идти напрямую - the paced log names the
// standing return).
func TestZoneReturnDryFailureHoldsTheReturn(t *testing.T) {
    loop, game, bot, nav := newTripLoop()
    moveSelfTo(bot, 43000, 50184, -2992)
    loop.zoneCX = 38553
    loop.zoneCY = 50080
    loop.zoneHalf = 1448
    // The search misses.
    nav.miss = true
    nav.found = false

    loop.tick()

    require.True(t, loop.zoneReturn,
        "the zone return is armed")
    require.Equal(t, phaseEngage, loop.phase,
        "the no-route hold keeps the engage phase")
    require.Empty(t, game.walks,
        "no direct walk toward the zone center ever goes out")
}

// TestStartZoneReturnSegmentRunsOnePricedSearch pins the search count of
// the zone return segment: one priced search per planning attempt - the
// dry then non-dry escalation of the old rounds is retired with the
// walled water form.
func TestStartZoneReturnSegmentRunsOnePricedSearch(t *testing.T) {
    loop, _, bot, nav := newTripLoop()
    moveSelfTo(bot, 43000, 50184, -2992)
    nav.found = true
    nav.route = []pathfind.Vec3{
        {X: 43000, Y: 50184, Z: -2992},
        {X: 42000, Y: 50180, Z: -2992},
        {X: 38553, Y: 50080, Z: -3512},
    }

    dest := pathfind.Vec3{X: 38553, Y: 50080, Z: -3512}
    ok := loop.startZoneReturnSegment(dest)

    require.True(t, ok, "the zone return segment was planned")
    require.Equal(t, 1, nav.calls,
        "the priced search runs once per planning attempt")
    require.NotEmpty(t, loop.waypoints,
        "the waypoints are armed from the search")
}
