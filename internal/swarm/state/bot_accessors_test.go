// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelfAccessorsTrackTheCharacter(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	// The CharSelected placement answers before the first UserInfo.
	require.Equal(t, int32(100), bot.SelfObjectID())
	x, y, z, ok := bot.SelfPosition()
	require.True(t, ok)
	require.Equal(t, int32(45000), x)
	require.Equal(t, int32(50000), y)
	require.Equal(t, int32(-3500), z)
	require.False(t, bot.SelfSitting())
	require.False(t, bot.SelfWalking())
	require.False(t, bot.SelfUnderAttack())
	require.False(t, bot.SelfDead())

	paperdoll := [PaperdollSlots]int32{}
	paperdoll[0] = 555
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 7, Exp: 4242,
		X: 45100, Y: 50100, Z: -3400,
		MaxHP: 122, CurHP: 90, MaxMP: 40, CurMP: 35,
		CurrentLoad: 640, MaxLoad: 64000,
		RunSpeed: 165, WalkSpeed: 80, MoveSpeedMult: 1.1,
		PaperdollObjectIDs: paperdoll,
	})
	require.Equal(t, int32(7), bot.SelfLevel())
	require.Equal(t, int32(4242), bot.SelfExp())

	// The self movement broadcast starts the walk, the placement stops it.
	bot.ApplyMovement(Movement{
		ObjectID: 100, X: 45100, Y: 50100, Z: -3400,
		DestX: 45600, DestY: 50600, DestZ: -3400,
	})
	require.True(t, bot.SelfWalking())
	bot.ApplyPlacement(Placement{
		ObjectID: 100, X: 45500, Y: 50500, Z: -3400, Heading: 8192,
	})
	require.False(t, bot.SelfWalking())

	// A hit on the character arms the under attack window.
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, Attackable: true, X: 45200, Y: 50200, Name: "Gremlin"})
	bot.ApplyAttack(Attack{
		AttackerID: 7, X: 45200, Y: 50200, Z: -3400,
		TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
		TargetX: 45100, TargetY: 50100, TargetZ: -3400,
	})
	require.True(t, bot.SelfUnderAttack())

	// Zero current HP with a known maximum marks the death.
	bot.ApplyStatusUpdate(100, []Attribute{{ID: AttrCurHP, Value: 0}})
	require.True(t, bot.SelfDead())
}

func TestSelfPositionUnknownWithoutCharacter(t *testing.T) {
	bot := NewBot("acc1")

	_, _, _, ok := bot.SelfPosition()
	require.False(t, ok)
}

func TestApplyWaitTypeTracksTheSitState(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 45200, Y: 50200, Name: "Gremlin"})

	bot.ApplyWaitType(WaitType{ObjectID: 100, Sitting: true})
	require.True(t, bot.SelfSitting())

	bot.ApplyWaitType(WaitType{ObjectID: 100, Sitting: false})
	require.False(t, bot.SelfSitting())

	// The transitions of other objects never touch the character.
	bot.ApplyWaitType(WaitType{ObjectID: 7, Sitting: true})
	require.False(t, bot.SelfSitting())
}

func TestStatusUpdateAppliesLevelAndLoad(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	bot.ApplyStatusUpdate(100, []Attribute{
		{ID: AttrLevel, Value: 9},
		{ID: AttrCurLoad, Value: 700},
		{ID: AttrMaxLoad, Value: 64000},
		{ID: AttrMaxMP, Value: 42},
		{ID: AttrCurMP, Value: 12},
		{ID: AttrMaxHP, Value: 130},
	})

	snap := bot.Snapshot()
	require.Equal(t, int32(9), snap.Character.Level)
	require.Equal(t, int32(700), snap.Character.CurrentLoad)
	require.Equal(t, int32(64000), snap.Character.MaxLoad)
	require.InDelta(t, 42, snap.Character.MaxMP, 0.001)
	require.InDelta(t, 12, snap.Character.CurMP, 0.001)
	require.InDelta(t, 130, snap.Character.MaxHP, 0.001)

	// Updates of other objects never touch the character vitals.
	bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 1}})
	require.InDelta(t, 130, snap.Character.MaxHP, 0.001)
}

func TestObjectAccessorsServeKnownObjects(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, Attackable: true, X: 45200, Y: 50200, Z: -3400,
		Name: "Gremlin",
	})

	x, y, z, ok := bot.ObjectPosition(7)
	require.True(t, ok)
	require.Equal(t, int32(45200), x)
	require.Equal(t, int32(50200), y)
	require.Equal(t, int32(-3400), z)
	require.Equal(t, "Gremlin", bot.ObjectName(7))
	require.True(t, bot.ObjectAlive(7))

	_, _, _, ok = bot.ObjectPosition(8)
	require.False(t, ok)
	require.Empty(t, bot.ObjectName(8))
	require.False(t, bot.ObjectAlive(8))

	// Without vitals the health percent stays unknown.
	require.InDelta(t, -1.0, bot.ObjectHealthPercent(7), 0.0001)
	bot.ApplyStatusUpdate(7, []Attribute{
		{ID: AttrCurHP, Value: 25},
		{ID: AttrMaxHP, Value: 50},
	})
	require.InDelta(t, 50.0, bot.ObjectHealthPercent(7), 0.001)
}

func TestApplyItemInfoTracksGroundItems(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	bot.ApplyItemInfo(ItemInfo{
		ObjectID: 201, TemplateID: 57, Count: 25,
		X: 45100, Y: 50100, Z: -3400,
	})

	item, ok := bot.GroundItemByID(201)
	require.True(t, ok)
	require.Equal(t, int32(201), item.ObjectID)
	require.Equal(t, int32(45100), item.X)

	// Unknown and non item objects report false.
	_, ok = bot.GroundItemByID(202)
	require.False(t, ok)
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 45200, Y: 50200, Name: "Gremlin"})
	_, ok = bot.GroundItemByID(7)
	require.False(t, ok)

	// The removal of the item clears the lookup.
	bot.RemoveObject(201)
	_, ok = bot.GroundItemByID(201)
	require.False(t, ok)
}

func TestSetHuntingZonePublishesTheSquare(t *testing.T) {
	bot := NewBot("acc1")

	bot.SetHuntingZone(45000, 50000, 1650)
	snap := bot.Snapshot()
	require.NotNil(t, snap.HuntingZone)
	require.Equal(t, int32(45000), snap.HuntingZone.CX)
	require.Equal(t, int32(50000), snap.HuntingZone.CY)
	require.Equal(t, int32(1650), snap.HuntingZone.Half)

	bot.SetHuntingZones([]ZoneView{
		{ID: "keltirs", Name: "keltirs", MinLevel: 1, MaxLevel: 4, Active: true},
		{ID: "goblins", Name: "goblins", MinLevel: 5, MaxLevel: 7, MinGear: 40},
	})
	snap = bot.Snapshot()
	require.Len(t, snap.HuntingZones, 2)
	require.True(t, snap.HuntingZones[0].Active)
}

func TestCountPacketAccountsTraffic(t *testing.T) {
	bot := NewBot("acc1")

	require.Equal(t, int64(0), bot.Snapshot().Packets)
	bot.CountPacket()
	bot.CountPacket()
	bot.CountPacket()

	require.Equal(t, int64(3), bot.Snapshot().Packets)
}

func TestNearestAttackerFindsTheClosestChaser(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	spawnNpcInfo(bot, 7, 1000001, 45300)
	spawnNpcInfo(bot, 8, 1000001, 45100)

	// Both gremlins target the character.
	bot.ApplyAttack(Attack{
		AttackerID: 7, X: 45300, Y: 50000, Z: -3500,
		TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
	})
	bot.ApplyAttack(Attack{
		AttackerID: 8, X: 45100, Y: 50000, Z: -3500,
		TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
	})

	attacker, ok := bot.NearestAttacker()
	require.True(t, ok)
	require.Equal(t, int32(8), attacker.ObjectID)
	require.Equal(t, int32(45100), attacker.X)

	// Without a chaser there is no attacker.
	bot.RemoveObject(7)
	bot.RemoveObject(8)
	_, ok = bot.NearestAttacker()
	require.False(t, ok)
}

func TestZoneHasAttackableReadsTheSquare(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	require.False(t, bot.ZoneHasAttackable(nil))
	require.False(t, bot.ZoneHasAttackable(&Zone{CX: 45000, CY: 50000, Half: 300}))

	spawnNpcInfo(bot, 7, 1000001, 45100)
	require.True(t, bot.ZoneHasAttackable(&Zone{CX: 45000, CY: 50000, Half: 300}))

	// A dead mob does not count.
	bot.ApplyStatusUpdate(7, []Attribute{
		{ID: AttrCurHP, Value: 0},
		{ID: AttrMaxHP, Value: 30},
	})
	require.False(t, bot.ZoneHasAttackable(&Zone{CX: 45000, CY: 50000, Half: 300}))
}

func TestNearestNpcByTemplatesPicksTheClosest(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	spawnNpcInfo(bot, 7, 1000001, 45300)
	spawnNpcInfo(bot, 8, 1000003, 45100)

	// Only the wanted templates enter the search.
	target, ok := bot.NearestNpcByTemplates([]int32{1000003}, 1500)
	require.True(t, ok)
	require.Equal(t, int32(8), target.ObjectID)

	_, ok = bot.NearestNpcByTemplates([]int32{1001277}, 1500)
	require.False(t, ok)

	// The distance limit bounds the search.
	_, ok = bot.NearestNpcByTemplates([]int32{1000001}, 100)
	require.False(t, ok)
}

func TestMedianZoneMobLevel(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	require.Zero(t, bot.MedianZoneMobLevel(nil))
	zone := &Zone{CX: 45000, CY: 50000, Half: 500}
	require.Zero(t, bot.MedianZoneMobLevel(zone))

	// Gremlin level 1, goblin level 5, orc archer level 8: the median
	// of the sorted [1 5 8] is 5.
	spawnNpcInfo(bot, 7, 1000001, 45100)
	spawnNpcInfo(bot, 8, 1000003, 45200)
	spawnNpcInfo(bot, 9, 1000006, 45300)
	require.Equal(t, int32(5), bot.MedianZoneMobLevel(zone))

	// A mob outside the square never enters the median.
	spawnNpcInfo(bot, 10, 1000006, 46000)
	require.Equal(t, int32(5), bot.MedianZoneMobLevel(zone))
}

func TestCommandQueueDeliversAndDropsOldest(t *testing.T) {
	bot := NewBot("acc1")

	// Commands arrive in the push order.
	bot.PushCommand(Command{Kind: CommandMove, X: 1})
	bot.PushCommand(Command{Kind: CommandAttack, ObjectID: 7})
	select {
	case cmd := <-bot.Commands():
		require.Equal(t, CommandMove, cmd.Kind)
		require.Equal(t, int32(1), cmd.X)
	default:
		t.Fatal("the first queued command must arrive at once")
	}
	select {
	case cmd := <-bot.Commands():
		require.Equal(t, CommandAttack, cmd.Kind)
	default:
		t.Fatal("the second queued command must arrive at once")
	}

	// The overflow drops the oldest commands, the newest survive.
	for i := range commandQueueCapacity + 8 {
		bot.PushCommand(Command{Kind: CommandMove, X: int32(i)})
	}
	received := make([]Command, 0, commandQueueCapacity)
	for {
		select {
		case cmd := <-bot.Commands():
			received = append(received, cmd)
		default:
			goto drained
		}
	}
drained:
	require.Len(t, received, commandQueueCapacity)
	require.Equal(t, int32(8), received[0].X, "the eight oldest dropped")
	require.Equal(
		t, int32(commandQueueCapacity+7), received[len(received)-1].X)
}

func TestPaperdollAndInventoryAccessors(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	paperdoll := [PaperdollSlots]int32{}
	paperdoll[1] = 555
	paperdoll[7] = 556
	bot.ApplyPaperdoll(paperdoll)
	require.Equal(t, paperdoll, bot.PaperdollSlotObjectIDs())

	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 1, ItemID: 17, Count: 500, Type2: 5},
		{ObjectID: 2, ItemID: 1060, Count: 30, Type2: 5},
	})
	items := bot.InventoryItems()
	require.Len(t, items, 2)

	// ApplyInventoryUpdate tracks additions, the accessor copies them.
	bot.ApplyInventoryUpdate([]InventoryItem{
		{ObjectID: 3, ItemID: 57, Count: 25, Type2: 4, Change: 1},
	})
	require.Len(t, bot.InventoryItems(), 3)
}

func TestDestroyableItemsRanksQuestAndGearFirst(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyItemList([]InventoryItem{
		{ObjectID: 1, ItemID: 1864, Count: 10, Type2: 5}, // stackable junk
		{ObjectID: 2, ItemID: 1000, Count: 1, Type2: 3},  // quest item
		{ObjectID: 3, ItemID: 34, Count: 1, Type2: 1},    // weapon drop
		{ObjectID: 4, ItemID: 57, Count: 1, Type2: 4},    // adena never
		{
			ObjectID: 5, ItemID: 35, Count: 1, Type2: 0, Equipped: true,
		}, // equipped never
	})

	destroyable := bot.DestroyableItems(5)
	require.Len(t, destroyable, 3)
	require.Equal(t, int32(2), destroyable[0].ObjectID, "quest first")
	require.Equal(t, int32(3), destroyable[1].ObjectID, "weapon second")
	require.Equal(t, int32(1), destroyable[2].ObjectID, "stackable last")
}

func TestApplyAutoAttackFlagsTrackSelfAndUnknown(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	bot.ApplyAutoAttackStart(100)
	require.True(t, bot.Snapshot().Character.InCombat)

	// The flag flip bumps the state version even without a visible
	// snapshot field: the combat window keeps the warm state.
	version := bot.Version()
	bot.ApplyAutoAttackStop(100)
	require.Greater(t, bot.Version(), version)

	// Unknown objects are ignored without an error.
	bot.ApplyAutoAttackStart(999)
	bot.ApplyAutoAttackStop(999)
}
