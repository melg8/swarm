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

func TestPickHuntingZoneGatesOnLevelAndGear(t *testing.T) {
	zones := ElvenHuntingZones()
	require.Len(t, zones, 4)

	// A fresh level 1 character without gear hunts the keltir field.
	zone, ok := PickHuntingZone(zones, 1, 0)
	require.True(t, ok)
	require.Equal(t, "elven-keltirs", zone.ID)

	// Level 5 with the starter gear (40+ points) moves to the goblin
	// camp; level 5 with bare fists stays with the keltirs (the gear
	// gate).
	zone, ok = PickHuntingZone(zones, 5, 44)
	require.True(t, ok)
	require.Equal(t, "elven-goblins", zone.ID)
	zone, ok = PickHuntingZone(zones, 5, 10)
	require.True(t, ok)
	require.Equal(t, "elven-keltirs", zone.ID)

	// Level 10 with the wooden set (110+ points) enters the kaboo
	// woods; without the gear it stays with the goblins.
	zone, ok = PickHuntingZone(zones, 10, 130)
	require.True(t, ok)
	require.Equal(t, "elven-kaboo", zone.ID)
	zone, ok = PickHuntingZone(zones, 10, 90)
	require.True(t, ok)
	require.Equal(t, "elven-goblins", zone.ID)

	// Level 15 with the bone set (200+ points) hunts the dryad forest.
	zone, ok = PickHuntingZone(zones, 15, 240)
	require.True(t, ok)
	require.Equal(t, "elven-dryads", zone.ID)
}

func TestPickHuntingZoneFallbacks(t *testing.T) {
	zones := ElvenHuntingZones()
	// A level below every band falls back to the starter zone.
	zone, ok := PickHuntingZone(zones, 0, 0)
	require.True(t, ok)
	require.Equal(t, "elven-keltirs", zone.ID)
	// An empty registry reports no zone.
	_, ok = PickHuntingZone(nil, 10, 100)
	require.False(t, ok)
}

func TestLoopSwitchesZoneOnLevel(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())

	// The first tick picks the keltir zone for the level 1 character.
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltirs", loop.zonePickedID)
	require.Equal(t, int32(1650), loop.zoneHalf)

	// The snapshot view carries the four zones with the keltirs
	// active.
	views := bot.Snapshot().HuntingZones
	require.Len(t, views, 4)
	require.True(t, views[0].Active)
	require.False(t, views[1].Active)

	// The character grows to level 5 with the gear: the next
	// evaluation switches to the goblin camp.
	setZoneTestLevel(bot, 5)
	equipZoneWithGear(bot, 44)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, "elven-goblins", loop.zonePickedID)
	require.Equal(t, int32(2900), loop.zoneHalf)
	views = bot.Snapshot().HuntingZones
	require.False(t, views[0].Active)
	require.True(t, views[1].Active)
}

func TestLoopKeepsZoneDuringCooldown(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltirs", loop.zonePickedID)

	// The level grows but the evaluation cooldown holds the zone.
	setZoneTestLevel(bot, 6)
	equipZoneWithGear(bot, 44)
	loop.tick()
	require.Equal(t, "elven-keltirs", loop.zonePickedID)
}

func TestUserZoneSelectOverridesPicker(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltirs", loop.zonePickedID)

	// The user selects the kaboo woods: the override applies at once.
	loop.userZoneSelect(2)
	require.Equal(t, "elven-kaboo", loop.zonePickedID)
	require.Equal(t, 2, loop.zoneOverride)

	// The automatic picker respects the override while the level
	// stays inside the band slack.
	setZoneTestLevel(bot, 9)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, "elven-kaboo", loop.zonePickedID)

	// Outgrowing the band (level beyond max + slack) resumes the
	// automatic picker.
	setZoneTestLevel(bot, 16)
	equipZoneWithGear(bot, 250)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, -1, loop.zoneOverride)
	require.Equal(t, "elven-dryads", loop.zonePickedID)
}

func TestUserZoneSelectBounds(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()

	loop.userZoneSelect(-1)
	require.Equal(t, "elven-keltirs", loop.zonePickedID)
	loop.userZoneSelect(int32(len(ElvenHuntingZones())))
	require.Equal(t, "elven-keltirs", loop.zonePickedID)
}

// setZoneTestLevel sets the character level through the userinfo
// path (the paperdoll resets with it, so equip the gear after).
func setZoneTestLevel(bot *state.Bot, level int32) {
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: level,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
	})
}

// equipZoneWithGear dresses the bot through the tracked inventory and
// paperdoll so the gear points reach the wanted level: the starter
// kit (short sword and shirt, 44), the wooden set step (with pants,
// 66) or the full shop dress (212 points: the long sword, the bone
// set, the helmet, the boots and the top jewels).
func equipZoneWithGear(bot *state.Bot, points int32) {
	items := []state.InventoryItem{
		{ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
		{ObjectID: 502, ItemID: 21, Count: 1, Equipped: true},
	}
	var paperdoll [state.PaperdollSlots]int32
	paperdoll[state.PaperdollRHand] = 501
	paperdoll[state.PaperdollChest] = 502
	switch {
	case points >= 200:
		items = []state.InventoryItem{
			{ObjectID: 501, ItemID: 2, Count: 1, Equipped: true},
			{ObjectID: 502, ItemID: 24, Count: 1, Equipped: true},
			{ObjectID: 503, ItemID: 31, Count: 1, Equipped: true},
			{ObjectID: 504, ItemID: 44, Count: 1, Equipped: true},
			{ObjectID: 505, ItemID: 38, Count: 1, Equipped: true},
			{ObjectID: 506, ItemID: 908, Count: 1, Equipped: true},
			{ObjectID: 507, ItemID: 845, Count: 1, Equipped: true},
			{ObjectID: 508, ItemID: 877, Count: 1, Equipped: true},
			{ObjectID: 509, ItemID: 877, Count: 1, Equipped: true},
		}
		paperdoll[state.PaperdollRHand] = 501
		paperdoll[state.PaperdollChest] = 502
		paperdoll[state.PaperdollLegs] = 503
		paperdoll[state.PaperdollHead] = 504
		paperdoll[state.PaperdollFeet] = 505
		paperdoll[state.PaperdollNeck] = 506
		paperdoll[state.PaperdollREar] = 507
		paperdoll[state.PaperdollRFinger] = 508
		paperdoll[state.PaperdollLFinger] = 509
	case points >= 66:
		items = append(items, state.InventoryItem{
			ObjectID: 503, ItemID: 28, Count: 1, Equipped: true,
		})
		paperdoll[state.PaperdollLegs] = 503
	case points >= 44:
	default:
		items = []state.InventoryItem{
			{ObjectID: 501, ItemID: 1, Count: 1, Equipped: true},
		}
		paperdoll = [state.PaperdollSlots]int32{}
		paperdoll[state.PaperdollRHand] = 501
	}
	bot.ApplyItemList(items)
	bot.ApplyPaperdoll(paperdoll)
}
