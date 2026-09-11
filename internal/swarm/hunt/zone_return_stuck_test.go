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
// (build c1faefb, bot test3, phase engage) showed the bot at
// 43000 50184 -2992 (near Herbiel, outside the zone) for 3 minutes.
// Events: "no dry path to 38553 50080, the walk would swim" repeated,
// then a learning trip started and also failed with "no dry path to
// 42766 50037" (Herbiel is only 276 units away). Two bugs:
//  1. The town trip started while the bot was outside the zone,
//     interfering with the zone return.
//  2. The zone return had no non-dry fallback: when the dry search
//     failed, it fell back to walkZoneLeg (direct walks) which may
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
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 100,
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

// TestZoneReturnNonDryFallback pins fix #2: when the dry search fails
// for the zone return, the non-dry search runs as a fallback. The
// 2026-09-11 06:00 dump showed the dry search failing from 43000
// 50184 to the zone center; the non-dry fallback gives the bot a
// route (the click guard refuses water legs and re-paths).
func TestZoneReturnNonDryFallback(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	// The bot stands outside the zone.
	moveSelfTo(bot, 43000, 50184, -2992)
	loop.zoneCX = 38553
	loop.zoneCY = 50080
	loop.zoneHalf = 1448
	// The dry search fails, the non-dry search succeeds.
	nav.dryMiss = true
	nav.found = true

	loop.tick()

	require.True(t, loop.zoneReturn,
		"the zone return is armed")
	require.Equal(t, phaseTownReturn, loop.phase,
		"the zone return is walking (the non-dry fallback found a path)")
	require.Greater(t, nav.calls, 1,
		"both the dry and the non-dry search ran")
}

// TestZoneReturnDryFailureFallsBackToWalkZoneLeg pins the last
// resort: when both the dry and the non-dry searches fail, the zone
// return falls back to walkZoneLeg (direct walks toward the zone
// center).
func TestZoneReturnDryFailureFallsBackToWalkZoneLeg(t *testing.T) {
	loop, game, bot, nav := newTripLoop()
	moveSelfTo(bot, 43000, 50184, -2992)
	loop.zoneCX = 38553
	loop.zoneCY = 50080
	loop.zoneHalf = 1448
	// Both searches fail.
	nav.dryMiss = true
	nav.found = false

	loop.tick()

	require.True(t, loop.zoneReturn,
		"the zone return is armed")
	require.Equal(t, phaseEngage, loop.phase,
		"the fallback keeps the engage phase (walkZoneLeg sends direct walks)")
	require.NotEmpty(t, game.walks,
		"walkZoneLeg sends a direct walk toward the zone center")
}

// TestStartZoneReturnLegTriesDryThenNonDry pins the search order of
// the zone return leg: the dry search runs first, and only when it
// fails does the non-dry search run.
func TestStartZoneReturnLegTriesDryThenNonDry(t *testing.T) {
	loop, _, bot, nav := newTripLoop()
	moveSelfTo(bot, 43000, 50184, -2992)
	nav.dryMiss = true
	nav.found = true
	nav.route = []pathfind.Vec3{
		{X: 43000, Y: 50184, Z: -2992},
		{X: 42000, Y: 50180, Z: -2992},
		{X: 38553, Y: 50080, Z: -3512},
	}

	dest := pathfind.Vec3{X: 38553, Y: 50080, Z: -3512}
	ok := loop.startZoneReturnLeg(dest)

	require.True(t, ok, "the zone return leg was planned (non-dry fallback)")
	require.Equal(t, 2, nav.calls,
		"the dry search ran once (missed), the non-dry search ran once (found)")
	require.NotEmpty(t, loop.waypoints,
		"the waypoints are armed from the non-dry search")
}
