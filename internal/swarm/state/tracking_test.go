// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRotationPacketsTurnObjects(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyPlayerInfo(PlayerInfo{ObjectID: 55, Name: "Other", X: 100, Y: 100})

	// The pair a standing player announces on spawn: begin carries the
	// start heading, stop the final one.
	bot.ApplyRotationStart(Rotation{ObjectID: 55, Heading: 30000})
	bot.ApplyRotationStop(Rotation{ObjectID: 55, Heading: 40000})

	snap := bot.Snapshot()
	require.Equal(t, int32(40000), snap.Objects[0].Heading)
	require.False(t, snap.Objects[0].Moving)
}

func TestAttackFacesTheTarget(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Z: -3500, Name: "Gremlin",
	})

	// The mob attacks the bot from the east of it: the mob must face
	// west toward the character.
	bot.ApplyAttack(Attack{
		AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		TargetIDs:   [AttackTargets]int32{100},
		TargetCount: 1,
	})

	snap := bot.Snapshot()
	require.Equal(t, int32(32768), snap.Objects[0].Heading)
	require.Equal(t, int32(100), snap.Objects[0].TargetID)
	// The character position refreshes from the trailing target
	// location of the attacker.
	require.Equal(t, int32(45000), snap.Character.X)
	require.True(t, snap.Character.InCombat)
}

func TestSelfAttackFacesTargetAndKeepsTarget(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 46000, Y: 50000, Z: -3500, Name: "Gremlin",
	})

	bot.ApplyAttack(Attack{
		AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
		TargetX: 46000, TargetY: 50000, TargetZ: -3500,
		TargetIDs:   [AttackTargets]int32{7},
		TargetCount: 1,
	})

	snap := bot.Snapshot()
	require.Equal(t, int32(0), snap.Character.Heading)
	require.Equal(t, int32(7), snap.Character.TargetID)
	require.True(t, snap.Character.InCombat)
}

func TestSelfPawnMovementChasesTarget(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	bot.ApplyPawnMovement(PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 40,
		X: 45000, Y: 50000, Z: -3500,
		TargetX: 45400, TargetY: 50000, TargetZ: -3500,
	})

	snap := bot.Snapshot()
	require.Equal(t, int32(45360), snap.Character.DestX)
	require.True(t, snap.Character.Moving)
	require.Equal(t, int32(7), snap.Character.TargetID)
	require.Equal(t, int32(0), snap.Character.Heading)
}

func TestMoveTypeUpdatesRunFlag(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 100, Y: 100, Name: "Gremlin", RunSpeed: 165, WalkSpeed: 80,
	})

	bot.ApplyMoveType(MoveType{ObjectID: 7, Running: false})
	snap := bot.Snapshot()
	require.False(t, snap.Objects[0].Running)
	require.InDelta(t, 80, snap.Objects[0].Speed, 0.001)
}

func TestSelfTargetTracking(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, TemplateID: 1000001, Attackable: true,
		X: 100, Y: 100, Name: "Gremlin",
	})

	bot.ApplySelfTarget(7)
	require.Equal(t, int32(7), bot.SelfTargetID())

	bot.ApplyObjectTarget(7, 100)
	bot.ApplyTargetClear(7)

	snap := bot.Snapshot()
	require.Equal(t, int32(7), snap.Character.TargetID)
	require.Equal(t, int32(0), snap.Objects[0].TargetID)
}

func TestTeleportSnapsPosition(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	bot.ApplyTeleport(Teleport{
		ObjectID: 100, X: 47000, Y: 51000, Z: -3500, Heading: 8192,
	})

	snap := bot.Snapshot()
	require.Equal(t, int32(47000), snap.Character.X)
	require.Equal(t, int32(8192), snap.Character.Heading)
	require.False(t, snap.Character.Moving)
}

func TestSpawnItemAndPickup(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	//nolint:exhaustruct // partial fields for the case
	bot.ApplySpawnItem(ItemInfo{
		ObjectID: 9, TemplateID: 1057, Count: 5, X: 45040, Y: 50040, Z: -3500,
	})
	item, ok := bot.NearestGroundItem(500)
	require.True(t, ok)
	require.Equal(t, int32(9), item.ObjectID)

	bot.ApplyItemPickup(ItemPickup{
		PlayerID: 100, ObjectID: 9, X: 45040, Y: 50040, Z: -3500,
	})
	_, ok = bot.NearestGroundItem(500)
	require.False(t, ok)
	snap := bot.Snapshot()
	require.Equal(t, int32(45040), snap.Character.X)
}

func TestNearestGroundItemSkipsBlacklisted(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplySpawnItem(ItemInfo{ObjectID: 1, TemplateID: 57, X: 100, Y: 0, Z: 0})
	//nolint:exhaustruct // partial fields for the case
	bot.ApplySpawnItem(ItemInfo{ObjectID: 2, TemplateID: 57, X: 500, Y: 0, Z: 0})

	item, ok := bot.NearestGroundItemExcluding(1000, map[int32]time.Time{
		1: time.Now().Add(time.Hour),
	})
	require.True(t, ok)
	require.Equal(t, int32(2), item.ObjectID)
}

func TestInventoryTracking(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)

	//nolint:exhaustruct // partial fields for the case
	items := []InventoryItem{
		//nolint:exhaustruct // partial fields for the case
		{ObjectID: 1, ItemID: 57, Count: 500, Type2: 4},
		//nolint:exhaustruct // partial fields for the case
		{ObjectID: 2, ItemID: 1146, Count: 1, Type2: 0},
		//nolint:exhaustruct // partial fields for the case
		{ObjectID: 3, ItemID: 1060, Count: 3, Type2: 5},
	}
	bot.ApplyItemList(items)

	stats := bot.InventoryStats()
	require.Equal(t, 3, stats.Slots)
	require.Equal(t, 80, stats.MaxSlots)
	require.Equal(t, int32(500), stats.Adena)

	// Pickup adds one stack, destroy removes another.
	bot.ApplyInventoryUpdate([]InventoryItem{
		//nolint:exhaustruct // partial fields for the case
		{ObjectID: 1, ItemID: 57, Count: 600, Type2: 4, Change: 2},
		//nolint:exhaustruct // partial fields for the case
		{ObjectID: 2, ItemID: 1146, Count: 1, Type2: 0, Change: 3},
	})
	stats = bot.InventoryStats()
	require.Equal(t, 2, stats.Slots)
	require.Equal(t, int32(600), stats.Adena)

	junk := bot.DestroyableItems(5)
	require.Len(t, junk, 1)
	require.Equal(t, int32(3), junk[0].ObjectID)
}

func TestInventoryStatsWeight(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyUserInfo(UserInfo{
		CurrentLoad: 60000, MaxLoad: 6400000, RunSpeed: 165,
	})
	stats := bot.InventoryStats()
	require.InDelta(t, 0.94, stats.WeightPercent, 0.01)
}

func TestEffectiveSpeedAppliesMultiplier(t *testing.T) {
	require.InDelta(t, 181.5, effectiveSpeed(165, 80, 1.1), 0.001)
	require.InDelta(t, 88.0, effectiveSpeed(0, 80, 1.1), 0.001)
	require.InDelta(t, 120.0, effectiveSpeed(0, 0, 0), 0.001)
}
