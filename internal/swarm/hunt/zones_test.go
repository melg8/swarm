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
	require.Len(t, zones, 30)

	// A fresh level 1 character without gear hunts the village keltir
	// meadow (the nearest of the starter band to the village).
	zone, ok := PickHuntingZone(zones, 1, 0, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-keltir-village", zone.ID)

	// Level 5 with the starter gear (44+ points) moves to the goblin
	// band; level 5 with bare fists stays below it (the gear gate).
	zone, ok = PickHuntingZone(zones, 5, 44, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-goblin-camp", zone.ID)
	zone, ok = PickHuntingZone(zones, 5, 10, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-wolf-north", zone.ID)

	// Level 10 with the wooden set (130+ points) enters the lieutenant
	// band; without it (100 points) the gear gate holds the character
	// in the grunt band.
	zone, ok = PickHuntingZone(zones, 10, 130, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-lieutenant-woods", zone.ID)
	zone, ok = PickHuntingZone(zones, 10, 100, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-grunt-west", zone.ID)

	// Level 17 with the deep set (300+ points) hunts the lirein
	// grounds of the far southwest.
	zone, ok = PickHuntingZone(zones, 17, 320, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-pincer-forest", zone.ID)
}

func TestPickHuntingZoneNearestOfTheBand(t *testing.T) {
	zones := ElvenHuntingZones()
	// No current zone: the nearest ground of the winning band wins -
	// a character east of the village picks the eastern keltir field,
	// not the village meadow.
	zone, ok := PickHuntingZone(zones, 1, 0, "", 49000, 42100, -1)
	require.True(t, ok)
	require.Equal(t, "elven-keltir-east", zone.ID)
	// The same from the northwest.
	zone, ok = PickHuntingZone(zones, 1, 0, "", 41500, 39500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-keltir-hills-west", zone.ID)
}

func TestPickHuntingZoneKeepsCurrentZoneOfTheBand(t *testing.T) {
	zones := ElvenHuntingZones()
	// The current zone of the winning band keeps its post even when a
	// sibling square is nearer: the periodic re-pick never bounces the
	// character between the grounds of one band.
	zone, ok := PickHuntingZone(zones, 1, 0,
		"elven-keltir-far-east", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-keltir-far-east", zone.ID)
}

func TestPickHuntingZoneDeathCap(t *testing.T) {
	zones := ElvenHuntingZones()
	// The death regression capped the ladder below the goblin band
	// (MinLevel 5): a level 8 character with the gear for the fighters
	// regresses to the raider band instead.
	zone, ok := PickHuntingZone(zones, 8, 120, "", 51707, 50504, 4)
	require.True(t, ok)
	require.Equal(t, "elven-raider-southeast", zone.ID)
	// A negative cap (the default) disables the regression.
	zone, ok = PickHuntingZone(zones, 8, 120, "", 51707, 50504, -1)
	require.True(t, ok)
	require.Equal(t, "elven-fighter-ridge", zone.ID)
}

func TestPickHuntingZoneFallbacks(t *testing.T) {
	zones := ElvenHuntingZones()
	// A level below every band falls back to the starter zone.
	zone, ok := PickHuntingZone(zones, 0, 0, "", 46112, 41500, -1)
	require.True(t, ok)
	require.Equal(t, "elven-keltir-village", zone.ID)
	// A cap that closes the whole ladder falls back to the starter
	// zone as well (nothing above the keltir band to hunt).
	zone, ok = PickHuntingZone(zones, 10, 300, "", 46112, 41500, 0)
	require.True(t, ok)
	require.Equal(t, "elven-keltir-village", zone.ID)
	// An empty registry reports no zone.
	_, ok = PickHuntingZone(nil, 10, 100, "", 0, 0, -1)
	require.False(t, ok)
}

func TestLoopSwitchesZoneOnLevel(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())

	// The first tick picks the village keltir meadow for the level 1
	// character.
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)
	require.Equal(t, int32(1000), loop.zoneHalf)

	// The snapshot view carries the thirty zones with the village
	// meadow active.
	views := bot.Snapshot().HuntingZones
	require.Len(t, views, 30)
	require.True(t, views[0].Active)
	require.False(t, views[1].Active)

	// The character grows to level 5 with the gear: the next
	// evaluation switches to the goblin band.
	setZoneTestLevel(bot, 5)
	equipZoneWithGear(bot, 44)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, "elven-goblin-camp", loop.zonePickedID)
	require.Equal(t, int32(1100), loop.zoneHalf)
	views = bot.Snapshot().HuntingZones
	require.False(t, views[0].Active)
	require.True(t, views[13].Active)
}

func TestLoopKeepsZoneDuringCooldown(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)

	// The level grows but the evaluation cooldown holds the zone.
	setZoneTestLevel(bot, 6)
	equipZoneWithGear(bot, 44)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)
}

func TestUserZoneSelectOverridesPicker(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)

	// The user selects the fighter woods: the override applies at
	// once.
	loop.userZoneSelect(17)
	require.Equal(t, "elven-kaboo-fighter-woods", loop.zonePickedID)
	require.Equal(t, 17, loop.zoneOverride)

	// The automatic picker respects the override while the level
	// stays inside the band slack.
	setZoneTestLevel(bot, 10)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, "elven-kaboo-fighter-woods", loop.zonePickedID)

	// Outgrowing the band (level beyond max + slack) resumes the
	// automatic picker. The full shop dress (212 points) opens the
	// elder band, the spider band still waits for its 230 gate.
	setZoneTestLevel(bot, 16)
	equipZoneWithGear(bot, 250)
	loop.zoneCheckAt = time.Now().Add(-zoneSwitchPeriod)
	loop.tick()
	require.Equal(t, -1, loop.zoneOverride)
	require.Equal(t, "elven-elder-forest", loop.zonePickedID)
}

func TestUserZoneSelectBounds(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()

	loop.userZoneSelect(-1)
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)
	loop.userZoneSelect(int32(len(ElvenHuntingZones())))
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)
}

func TestZoneDeathsDemoteTheBand(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 5)
	equipZoneWithGear(bot, 44)
	loop.tick()
	require.Equal(t, "elven-goblin-camp", loop.zonePickedID)
	// The hunt walks into the goblin camp square: deaths count against
	// the ground they happen in (the position attribution).
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	zoneDeathWaitRevival(bot)
	// Two deaths count, the third demotes the band.
	for range 2 {
		zoneDeathKill(bot, loop)
		require.Equal(t, "elven-goblin-camp", loop.zonePickedID)
	}
	zoneDeathKill(bot, loop)
	require.Equal(t, int32(4), loop.zoneDeathCap,
		"the ladder caps below the goblin band (MinLevel 5)")
	require.True(t, loop.zoneCheckAt.IsZero(),
		"the demotion forces the ladder re-pick")

	// The revived character re-picks on the next tick: the goblin
	// band is capped out, the picker regresses to the raider band.
	zoneDeathWaitRevival(bot)
	loop.tick()
	require.Equal(t, "elven-raider-southeast", loop.zonePickedID)
	require.Equal(t, int32(3), loop.zoneDeaths["elven-goblin-camp"])

	// The view carries the deaths and the demoted marker of the
	// goblin band zones.
	views := bot.Snapshot().HuntingZones
	var goblin *state.ZoneView
	for index := range views {
		if views[index].ID == "elven-goblin-camp" {
			goblin = &views[index]
		}
	}
	require.NotNil(t, goblin)
	require.Equal(t, int32(3), goblin.Deaths)
	require.True(t, goblin.Demoted)
}

func TestZoneDeathsResetOnLevelChange(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 5)
	equipZoneWithGear(bot, 44)
	loop.tick()
	require.Equal(t, "elven-goblin-camp", loop.zonePickedID)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	zoneDeathWaitRevival(bot)
	require.Equal(t, int32(-1), loop.zoneDeathCap)

	// The character grows a level: the bookkeeping resets and the
	// picker may retry the demotable band later.
	setZoneTestLevel(bot, 6)
	loop.tick()
	require.Equal(t, int32(-1), loop.zoneDeathCap)
	require.Empty(t, loop.zoneDeaths)
}

// TestZoneDeathOnFreshSessionCountsByPosition pins the emergency
// logout scenario: the character died while the session was offline
// (the server keeps it in the world through the combat window), so
// the reconnect lands on a fresh loop before any zone pick - the
// death spot is the only truthful attribution and the position
// lookup counts it there.
func TestZoneDeathOnFreshSessionCountsByPosition(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	// The character reconnects dead at the goblin camp, no zone picked.
	setZoneTestLevel(bot, 5)
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
	require.Equal(t, "", loop.zonePickedID,
		"the death tick runs before the first zone pick")
	require.Equal(t, int32(1), loop.zoneDeaths["elven-goblin-camp"],
		"the death counts against the square it happened in")
}

func TestZoneDeathOfDelevelingNeverCounts(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 5)
	equipZoneWithGear(bot, 44)
	loop.tick()
	require.Equal(t, "elven-goblin-camp", loop.zonePickedID)

	// A death of the delevel phase is the point of the phase, not a
	// regression signal (the character stands inside the goblin camp
	// square: only the phase check keeps it from counting).
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
	})
	loop.phase = phaseDelevel
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	zoneDeathWaitRevival(bot)
	zoneDeathKill(bot, loop)
	require.Empty(t, loop.zoneDeaths)
	require.Equal(t, int32(-1), loop.zoneDeathCap)
}

func TestZoneRotationOnEmptySquare(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)

	// The character stands in the middle of the empty square, the
	// emptiness window is up: the rotation moves it to the nearest
	// sibling ground of the band.
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, "elven-keltir-east", loop.zonePickedID,
		"the empty square rotates to the nearest sibling")
	require.True(t, loop.zoneEmptySince.IsZero(),
		"the new square starts with a fresh emptiness window")
	require.False(t, loop.zoneCheckAt.IsZero(),
		"the rotation holds the ladder re-eval for a period")

	// The walk into the new square lands, it clears out as well: the
	// next rotation continues onto the following sibling.
	bot.ApplyPlacement(state.Placement{
		ObjectID: 100, X: 49100, Y: 42300, Z: -3500,
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, "elven-keltir-far-east", loop.zonePickedID)
}

func TestZoneRotationWaitsWhileMobsRemain(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)

	// A living attackable mob inside the square (far enough to stay
	// out of the engage radius, near enough to sit inside the zone):
	// no rotation however long the window runs.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 77, TemplateID: 1000530, Attackable: true,
		X: 46800, Y: 41800, Name: "Red Keltir",
	})
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	for range 3 {
		loop.tick()
	}
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)
	require.True(t, loop.zoneEmptySince.IsZero())
}

func TestZoneRotationSkipsMidFight(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()

	// A running fight (a live target) resets the emptiness window:
	// the rotation never abandons a fight.
	loop.target = 55
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)
	require.True(t, loop.zoneEmptySince.IsZero())
}

func TestZoneRotationWithoutSiblingStays(t *testing.T) {
	zones := []HuntingZone{
		{
			ID: "one", Name: "One", Region: "test",
			MinLevel: 1, MaxLevel: 3, MinGear: 0,
			CX: 46112, CY: 41500, Half: 1000,
		},
	}
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(zones)
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "one", loop.zonePickedID)

	// No sibling of the band: the empty window re-arms and the hunter
	// waits out the respawn of its only square.
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.tick()
	require.Equal(t, "one", loop.zonePickedID)
	require.False(t, loop.zoneEmptySince.IsZero(),
		"the window re-arms instead of spinning the check")
}

func TestZoneSwitchDropsTheStaleFarmSpot(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	setZoneTestLevel(bot, 1)
	loop.tick()
	require.Equal(t, "elven-keltir-village", loop.zonePickedID)

	// The farm spot of the village meadow (far outside the eastern
	// field): a zone switch must drop it, a return leg aims at the
	// new zone center instead of walking to the old square.
	loop.farmX, loop.farmY, loop.farmZ = 46200, 41600, -3455
	loop.userZoneSelect(1)
	require.Equal(t, "elven-keltir-east", loop.zonePickedID)
	require.Zero(t, loop.farmX)
	require.Zero(t, loop.farmY)
	zone := loop.zone()
	require.NotNil(t, zone)
	require.True(t, zone.Contains(
		zone.CX, zone.CY), "the fallback destination is the new center")
}

// setZoneTestLevel sets the character level through the userinfo
// path (the paperdoll resets with it, so equip the gear after) and
// places it at the elven village.
func setZoneTestLevel(bot *state.Bot, level int32) {
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: level,
		X: 46112, Y: 41500, Z: -3500,
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

// zoneDeathKill kills the character through the status update of the
// self object and runs one tick: recoverFromDeath counts the death
// against the active zone. The restart pacing clears first, so every
// kill of the sequence goes through the full recovery path.
func zoneDeathKill(bot *state.Bot, loop *Loop) {
	loop.restartAt = time.Time{}
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	loop.tick()
}

// zoneDeathWaitRevival revives the character and clears the restart
// pacing so the next tick runs the living phases again.
func zoneDeathWaitRevival(bot *state.Bot) {
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 90},
	})
}
