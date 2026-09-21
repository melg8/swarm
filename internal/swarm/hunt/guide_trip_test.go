// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "log"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The guide run tests: the Newbie Guide support magic as a town trip
// reason of its own. The buff refill outranks the farming (a bot
// never keeps swinging without the buffs while the guide is
// reachable) and the death recovery rides it - the village revive
// lands next to the guide, the trip takes the buffs first and walks
// the preserved farm spot second.
//
// The level seeding zeroes the tracker position (ApplyUserInfo
// carries the coordinate block), so every test snaps the character
// to its spot AFTER setGuideLevel.

// farmSpotPoint snaps the character to the test farm spot inside the
// default test hunting square (the zone center itself).
func farmSpotPoint(bot *state.Bot) {
    moveSelfTo(bot, 45000, 50000, -3500)
}

// villageRevivePoint snaps the character next to the elven village
// Newbie Guide, the spot the village restart of a death lands on.
func villageRevivePoint(bot *state.Bot) {
    moveSelfTo(bot, guideStopNpc.X+80, guideStopNpc.Y+80, guideStopNpc.Z)
}

// TestGuideRunStartsFromTheVillageAfterDeath pins the death recovery:
// the revived character outside the hunting square starts the buff
// refill trip instead of the plain walk home, and the farm spot the
// death preserved stays the return target (the trip keeps it, the
// zone center overwrite of a village start never fires).
func TestGuideRunStartsFromTheVillageAfterDeath(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    logBuf := &bytes.Buffer{}
    loop.SetLogger(log.New(logBuf, "", 0))
    setGuideLevel(bot, 13)
    villageRevivePoint(bot)
    // The hunting square sits far from the village: the revive point
    // is outside it, the farm spot inside (the ground the death
    // interrupted).
    loop.SetHuntingZone(40000, 40000, 1000)
    loop.farmX, loop.farmY, loop.farmZ = 40300, 40400, -3500

    loop.tick()

    require.Equal(t, phaseTownWalk, loop.phase,
        "the buff refill trip starts from the village revive point")
    require.NotEmpty(t, loop.tripStops)
    require.True(t, loop.tripStops[0].sell,
        "the trip opens with the ordinary sell stop")
    require.Equal(t, int32(40300), loop.farmX,
        "the preserved farm spot survives the out of zone trip start")
    require.Contains(t, logBuf.String(), "support magic",
        "the trip reason names the buff refill")
}

// TestGuideRunStartsFromTheZoneWhenBuffsExpire pins the buff priority:
// the support magic lapses mid farm, the next tick walks the
// character to the guide instead of farming unbuffed on.
func TestGuideRunStartsFromTheZoneWhenBuffsExpire(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    farmSpotPoint(bot)
    loop.SetHuntingZone(45000, 50000, 2000)

    loop.tick()

    require.Equal(t, phaseTownWalk, loop.phase,
        "the missing support magic starts the refill trip")
    require.NotEmpty(t, loop.tripStops)
    require.True(t, loop.tripStops[0].sell)
    require.Equal(t, int32(45000), loop.farmX,
        "the in zone start remembers the current spot as the farm spot")
}

// TestGuideRunWaitsForTheBuffsAndTheBand pins the negatives of the
// trigger: a character whose expected support magic is all active
// keeps farming, a character outside the 8-24 band never starts the
// run.
func TestGuideRunWaitsForTheBuffsAndTheBand(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    farmSpotPoint(bot)
    // The scroll of the keep one line rides the inventory: the shop
    // plan stays empty and the guide trigger is the sole candidate.
    addSOE(bot, 1)
    loop.SetHuntingZone(45000, 50000, 2000)
    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 1204, Level: 2, Time: 1200},
        {SkillID: 1040, Level: 3, Time: 1200},
        {SkillID: 1045, Level: 1, Time: 1200},
        {SkillID: 1068, Level: 1, Time: 1200},
    })

    loop.tick()

    require.Empty(t, loop.tripStops,
        "the complete support magic holds the trip")
    require.Equal(t, phaseEngage, loop.phase,
        "the bot keeps farming under the full buffs")

    setGuideLevel(bot, 25)
    farmSpotPoint(bot)
    loop.tick()

    require.Empty(t, loop.tripStops,
        "the levels above the band never start the run")
    require.Equal(t, phaseEngage, loop.phase)
}

// TestGuideRunRefusedFallsBackToTheZoneReturn pins the refusal
// fallback: the guide refusal cooldown keeps the trip away, the plain
// zone return owns the walk home from the village revive point.
func TestGuideRunRefusedFallsBackToTheZoneReturn(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    loop.guideRefusedAt = time.Now().Add(time.Minute)
    villageRevivePoint(bot)
    loop.SetHuntingZone(40000, 40000, 1000)

    loop.tick()

    require.Empty(t, loop.tripStops,
        "the refusal cooldown keeps the buff run away")
    require.Equal(t, phaseTownReturn, loop.phase,
        "the plain zone return owns the walk instead")
    // The return tick plans the walk home; the follower sends the
    // first walk request on the next tick.
    loop.tick()
    require.NotEmpty(t, game.walks, "the return walk started")
}

// TestGuideRunStaysOffTheRegionsWithoutAGuide pins the region guard:
// the Dion region maps no Newbie Guide yet, so the run trigger stays
// off there and the stop planning never walks a Dion trip to the
// elven village guide across the whole map - while the elven default
// keeps the run available.
func TestGuideRunStaysOffTheRegionsWithoutAGuide(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)

    loop.SetHuntingZoneRegion(regionDion)
    require.False(t, loop.guideRunWanted(),
        "the region without a mapped guide never runs the buff errand")
    loop.planGuideStop()
    require.Empty(t, loop.tripStops,
        "the stop planning refuses the region without a guide too")

    loop.SetHuntingZoneRegion(regionElven)
    require.True(t, loop.guideRunWanted(),
        "the elven region keeps the buff errand available")
}

// TestGuideStopRidesTheJunklessSellStop pins the stop execution of
// the buff trip: the junk-less sell stop distributes the empty plan,
// the guide stop plans behind it and the walk to the guide spawn
// starts - the whole errand shares the ordinary trip machinery.
func TestGuideStopRidesTheJunklessSellStop(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    farmSpotPoint(bot)
    // The scroll of the keep one line rides the inventory: the plan
    // stays empty, so the buff refill is the sole trip reason and no
    // buy pends at the sell stop.
    addSOE(bot, 1)
    loop.SetHuntingZone(45000, 50000, 2000)

    loop.tick()
    require.True(t, loop.tripStops[0].sell, "the sell stop leads")

    arriveAtMerchantStand(bot, townMerchants[3])
    loop.tick()
    require.Equal(t, phaseTownSell, loop.phase)

    // The junk-less bag sells nothing: the stop planning distributes
    // the empty plan, appends the guide stop and the trip walks on.
    loop.tick()
    require.True(t, loop.guideStop(),
        "the guide stop plans behind the empty sell stop")
    require.Equal(t, phaseTownWalk, loop.phase,
        "the walk to the Newbie Guide spawn started")
}
