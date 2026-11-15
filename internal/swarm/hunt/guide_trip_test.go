// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "log"
    "testing"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The guide trip tests: the support magic never owns a trip of its own. A
// dedicated walk to the guide cost a median 289 seconds of farm time per trip
// and the level 1 buffs changed no measurable kill rate (docs/hunting.md), so
// planGuideStop only appends its stop to an already running sell trip.
//
// The level seeding zeroes the tracker position (ApplyUserInfo carries the
// coordinate block), so every test snaps the character AFTER setGuideLevel.

// farmSpotPoint snaps the character to the test farm spot inside the default
// test hunting square (the zone center itself).
func farmSpotPoint(bot *state.Bot) {
    moveSelfTo(bot, 45000, 50000, -3500)
}

// villageRevivePoint snaps the character next to the elven Newbie Guide, the
// spot the village restart of a death lands on.
func villageRevivePoint(bot *state.Bot) {
    moveSelfTo(bot, guideStopNpc.X+80, guideStopNpc.Y+80, guideStopNpc.Z)
}

// hasGuideStop reports whether a guide stop waits in the frozen trip.
func hasGuideStop(stops []tripStop) bool {
    for index := range stops {
        if stops[index].guide {
            return true
        }
    }

    return false
}

// TestGuideNeverStartsADedicatedTripFromTheFarm pins the removed trip
// trigger: the missing support magic alone never walks the bot out of the
// hunting zone - the farm keeps going and the next sell trip may carry the
// guide stop.
func TestGuideNeverStartsADedicatedTripFromTheFarm(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    farmSpotPoint(bot)
    // The keep one scroll stock rides the bag: the shop plan stays empty,
    // so nothing but the removed guide trigger could start a trip.
    addSOE(bot, 1)
    loop.SetHuntingZone(45000, 50000, 2000)

    loop.tick()

    require.Empty(t, loop.tripStops,
        "the missing support magic starts no trip of its own")
    require.Equal(t, phaseEngage, loop.phase,
        "the bot keeps farming unbuffed")
}

// TestGuideNeverStartsADedicatedTripFromTheVillage pins the death recovery:
// the revived character outside the hunting square walks home through the
// ordinary zone return instead of a buff errand to the neighbouring guide.
func TestGuideNeverStartsADedicatedTripFromTheVillage(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)
    villageRevivePoint(bot)
    // The hunting square sits far from the village: the revive point is
    // outside it.
    loop.SetHuntingZone(40000, 40000, 1000)

    loop.tick()

    require.Empty(t, loop.tripStops,
        "the village revive starts no buff trip")
    require.Equal(t, phaseTownReturn, loop.phase,
        "the plain zone return owns the walk home")
    // The return tick plans the walk home; the follower sends the first
    // walk request on the next tick.
    loop.tick()
    require.NotEmpty(t, game.walks, "the return walk started")
}

// TestGuideStopJoinsAFullInventoryTrip pins the only way the support magic is
// collected: the sell trip the full inventory started appends the guide stop
// behind its sell stop once the bag is empty.
func TestGuideStopJoinsAFullInventoryTrip(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    logBuf := &bytes.Buffer{}
    loop.SetLogger(log.New(logBuf, "", 0))
    setGuideLevel(bot, 13)
    farmSpotPoint(bot)
    addSOE(bot, 1)
    fillInventory(bot)
    loop.SetHuntingZone(45000, 50000, 2000)

    loop.tick()
    require.Equal(t, phaseTownWalk, loop.phase,
        "the full inventory starts the trip")
    require.True(t, loop.tripStops[0].sell, "the sell stop leads")

    // The sale emptied the bag: the sold marks stand for the confirmed
    // InventoryUpdate removals, so the stop plans the rest of the trip.
    for _, item := range loop.sellableJunk() {
        loop.sold[item.ObjectID] = true
    }
    arriveAtMerchantStand(bot, townMerchants[3])
    loop.tick()
    require.Equal(t, phaseTownSell, loop.phase)
    loop.tick()

    require.Contains(t, logBuf.String(), "Newbie Guide",
        "the empty sell stop planned the guide stop")
    require.True(t, hasGuideStop(loop.tripStops),
        "the guide stop waits in the trip")
}

// TestGuideStopStaysOffTheRegionsWithoutAGuide pins the region guard: the
// Dion region maps no Newbie Guide yet, so the stop planning never walks a
// Dion trip to the elven village guide across the whole map - while the elven
// default plans the stop.
func TestGuideStopStaysOffTheRegionsWithoutAGuide(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    setGuideLevel(bot, 13)

    loop.SetHuntingZoneRegion(regionDion)
    loop.planGuideStop()
    require.Empty(t, loop.tripStops,
        "the region without a mapped guide never plans the stop")

    loop.SetHuntingZoneRegion(regionElven)
    loop.planGuideStop()
    require.True(t, hasGuideStop(loop.tripStops),
        "the elven region plans the stop")
}
