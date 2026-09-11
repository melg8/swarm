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

// weaponRunUserInfo is the full sheet of the weapon run tests: the
// level drives the milestone ladder, the position keeps the character
// at the test farm spot and the vitals keep it healthy (a hurt or
// moved character routes the tick into the rest and the zone return
// instead of the tested path).
func weaponRunUserInfo(level int32) state.UserInfo {
	return state.UserInfo{
		Name: "test2", Level: level, ClassID: 18, Race: 1,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 285, CurHP: 285, MaxMP: 113, CurMP: 113,
	}
}

// TestWeaponlessRunStartsTheWeaponStop pins the weapon run trigger: a
// bare-handed character with an affordable weapon in the plan starts a
// town trip whose sell stop IS the weapon merchant - the junk sells
// there and the weapon buys at the same npc, no other stop rides the
// errand. The dump report walked to the armor trader first and carried
// the teacher stop whose stuck walk aborted the trip before the weapon
// buy stop ever ran.
func TestWeaponlessRunStartsTheWeaponStop(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	bot.ApplyUserInfo(weaponRunUserInfo(11))
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 14814, Type2: 4, Change: 1},
	})

	require.True(t, loop.weaponlessRunWanted(),
		"the bare-handed bot with an affordable weapon wants the run")

	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase,
		"the weapon run starts the town trip")
	require.Len(t, loop.tripStops, 1,
		"the weapon run carries the sell stop only")
	require.Equal(t, int32(7147), loop.tripStops[0].merchant.TemplateID,
		"the sell stop routes to Unoren, the weapon merchant")
	require.True(t, loop.tripStops[0].sell,
		"the weapon stop still sells the junk there")
	require.NotEmpty(t, game.walks,
		"the walk to the weapon merchant started")
	// The walk aims at Unoren, not at the nearest merchant of the farm
	// spot (Creamees/Herbiel across the village): the first leg is the
	// split of the long route (maxMoveLeg), so its target compares
	// through the leg walk helper.
	unoren := townMerchants[0]
	require.Equal(t, [][3]int32{legWalkTarget([3]int32{45000, 50000, -3500},
		pathfind.Vec3{
			X: float64(unoren.X), Y: float64(unoren.Y),
			Z: float64(unoren.Z),
		})}, game.walks,
		"the walk aims along the leg to the weapon merchant")
}

// TestWeaponlessRunSkipsLearning pins the priority: queued lessons
// never ride the weapon run - a bare-handed character walks for the
// weapon and back, the lessons keep waiting for the next trip.
func TestWeaponlessRunSkipsLearning(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	// The learning budget of the learn tests (500 sp of queued level 5
	// strikes) PLUS no weapon: the weapon run must win the trip.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 500,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
	})
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 14814, Type2: 4, Change: 1},
	})
	require.True(t, loop.learnTripWanted(),
		"the lesson budget alone would start a trip")

	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase)
	require.Len(t, loop.tripStops, 1,
		"the weapon run carries no teach stop")
	require.False(t, loop.tripStops[0].teach)
}

// TestWeaponlessRunHoldsFreshPicks pins the bare-handed engage gate:
// no fresh target is picked while the weapon run is pending, but an
// attacker already on the character is still fought - the self defense
// answer never depends on the weapon state.
func TestWeaponlessRunHoldsFreshPicks(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	bot.ApplyUserInfo(weaponRunUserInfo(11))
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 14814, Type2: 4, Change: 1},
	})
	spawnMob(bot)
	// The trip cannot start: the cooldown of a freshly ended trip holds
	// it, so the engage gate owns the tick.
	loop.tripEndedAt = time.Now()

	loop.tick()

	require.Empty(t, game.forces,
		"no fresh target is picked bare-handed")
	require.Equal(t, phaseEngage, loop.phase)

	// An attacker on the character is answered even bare-handed.
	mobHitsCharacter(bot)
	loop.tick()

	require.NotEmpty(t, game.forces,
		"the attacker is engaged for the self defense")
	require.Equal(t, int32(7), loop.target)
}

// TestWeaponlessPickProceedsWhenNoWeaponIsAffordable pins the gate
// scope: a wallet that cannot buy any weapon keeps farming - the
// punches are all the character has until the plan offers a sword.
func TestWeaponlessPickProceedsWhenNoWeaponIsAffordable(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	bot.ApplyUserInfo(weaponRunUserInfo(5))
	// 50 adena: below the cheapest weapon offer of the catalogs.
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 50, Type2: 4, Change: 1},
	})
	require.False(t, loop.weaponlessRunWanted())
	spawnMob(bot)
	loop.tripEndedAt = time.Now().Add(-time.Minute)

	loop.tick()

	require.NotEmpty(t, game.forces,
		"the bare-handed pick proceeds when no weapon is affordable")
	require.Equal(t, int32(7), loop.target)
}

// TestWeaponRunCooldownIsShort pins the retry pacing: an aborted
// weapon run retries after weaponRunCooldown, not after the five
// minute cooldown of an ordinary trip.
func TestWeaponRunCooldownIsShort(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	bot.ApplyUserInfo(weaponRunUserInfo(11))
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 14814, Type2: 4, Change: 1},
	})
	// Bare-handed: the short cooldown applies.
	loop.tripEndedAt = time.Now().Add(-weaponRunCooldown +
		2*time.Second)
	require.False(t, loop.tripCooldownOver(),
		"the weapon run cooldown still holds")
	loop.tripEndedAt = time.Now().Add(-weaponRunCooldown -
		2*time.Second)
	require.True(t, loop.tripCooldownOver(),
		"the weapon run retries on the short cooldown")

	// With a weapon the ordinary five minute cooldown applies.
	bot.ApplyInventoryUpdate([]state.InventoryItem{
		{ObjectID: 100, ItemID: 1, Count: 1, Equipped: true, Change: 1},
	})
	loop.tripEndedAt = time.Now().Add(-weaponRunCooldown -
		2*time.Second)
	require.False(t, loop.tripCooldownOver(),
		"an armed character waits out the ordinary trip cooldown")
}

// TestWeaponUpgradeRoutesToWeaponMerchant pins the stop routing of the
// upgrade case: a trip whose plan buys a weapon milestone puts its
// sell stop on the weapon merchant, so the sell-first of the replaced
// weapon and the replacement buy share ONE npc visit - the window
// between the sale and the buy collapses from a village walk to
// seconds.
func TestWeaponUpgradeRoutesToWeaponMerchant(t *testing.T) {
	loop, _, bot, _ := newTripLoop()
	// The sickle-wearing character of the replacement test with the
	// wallet for the next weapon milestone.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 8,
		X: 46112, Y: 41500, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
		PaperdollObjectIDs: [state.PaperdollSlots]int32{0, 0, 0, 0, 0, 0, 0, 100},
	})
	bot.ApplyItemList([]state.InventoryItem{
		{
			ObjectID: 100, ItemID: 153, Count: 1, Equipped: true,
			BodyPart: 0x80, Change: 1,
		},
		{ObjectID: 999, ItemID: 57, Count: 60000, Type2: 4, Change: 1},
	})

	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase)
	require.Equal(t, int32(7147), loop.tripStops[0].merchant.TemplateID,
		"the upgrade trip sells at the weapon merchant")
}

// TestEngagesOnZoneEntryHoldsWhenWeaponless pins the return leg: a
// bare-handed character walks home past a pickable target instead of
// engaging it - the weapon run owns the next ticks.
func TestEngagesOnZoneEntryHoldsWhenWeaponless(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	bot.ApplyUserInfo(weaponRunUserInfo(11))
	bot.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 14814, Type2: 4, Change: 1},
	})
	spawnMob(bot)
	loop.SetHuntingZone(45000, 50000, 2000)
	loop.phase = phaseTownReturn
	loop.lastHit = time.Now().Add(-time.Minute)

	require.False(t, loop.engagesOnZoneEntry(),
		"the zone entry engage holds while weaponless")
	require.Empty(t, game.forces)
}
