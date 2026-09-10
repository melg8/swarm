// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSetBuffsPublishesTheSnapshot pins the apply path: the active
// effect list lands in the snapshot enriched with the display data,
// sorted by skill id, with the remaining seconds counted down from
// the arrival of the server list.
func TestSetBuffsPublishesTheSnapshot(t *testing.T) {
	bot := NewBot("test1")
	require.Nil(t, bot.Snapshot().Buffs,
		"no buffs before the server lists them")

	bot.SetBuffs([]BuffEntry{
		{SkillID: 91, Level: 1, Time: 1200}, // Defence Aura
		{SkillID: 77, Level: 2, Time: 600},  // Attack Aura
	})

	snap := bot.Snapshot()
	require.NotNil(t, snap.Buffs)
	require.Len(t, snap.Buffs, 2)

	first := snap.Buffs[0]
	require.Equal(t, int32(77), first.SkillID)
	require.Equal(t, int32(2), first.Level)
	require.Equal(t, "Attack Aura", first.Name)
	require.Equal(t, "skill0077", first.Icon)
	require.InDelta(t, 600, first.Left, 2)

	second := snap.Buffs[1]
	require.Equal(t, int32(91), second.SkillID)
	require.Equal(t, "Defense Aura", second.Name)
	require.InDelta(t, 1200, second.Left, 2)
}

// TestSetBuffsCountsDown pins the remaining seconds: the views count
// down from the arrival of the last server list, floor at zero.
func TestSetBuffsCountsDown(t *testing.T) {
	bot := NewBot("test1")
	bot.SetBuffs([]BuffEntry{{SkillID: 91, Level: 1, Time: 10}})

	snap := bot.Snapshot()
	require.Len(t, snap.Buffs, 1)
	require.InDelta(t, 10, snap.Buffs[0].Left, 2)

	// A minute later the effect is gone from the countdown.
	bot.mu.Lock()
	bot.buffsAt = time.Now().Add(-time.Minute)
	bot.mu.Unlock()
	snap = bot.Snapshot()
	require.Len(t, snap.Buffs, 1)
	require.Zero(t, snap.Buffs[0].Left)
}

// TestSetBuffsReplacesTheList pins the full replace semantics: an
// expired buff leaves the list when the server refreshes it.
func TestSetBuffsReplacesTheList(t *testing.T) {
	bot := NewBot("test1")
	bot.SetBuffs([]BuffEntry{
		{SkillID: 91, Level: 1, Time: 1200},
		{SkillID: 77, Level: 1, Time: 600},
	})
	require.True(t, bot.SelfHasBuff(91))
	require.True(t, bot.SelfHasBuff(77))

	bot.SetBuffs([]BuffEntry{{SkillID: 91, Level: 1, Time: 900}})
	require.True(t, bot.SelfHasBuff(91))
	require.False(t, bot.SelfHasBuff(77),
		"the expired buff left the list")

	snap := bot.Snapshot()
	require.Len(t, snap.Buffs, 1)
	require.Equal(t, int32(91), snap.Buffs[0].SkillID)
}

// TestSetBuffsCapsTheNeverTimedOut pins the clamp: the server marks
// the effects it never times out with a huge value, the views clamp
// those to a day.
func TestSetBuffsCapsTheNeverTimedOut(t *testing.T) {
	bot := NewBot("test1")
	bot.SetBuffs([]BuffEntry{{SkillID: 91, Level: 1, Time: 2147483647}})

	snap := bot.Snapshot()
	require.Len(t, snap.Buffs, 1)
	require.Equal(t, int32(86400), snap.Buffs[0].Left)
}

// TestBuffsClearedBySessionReset pins the session reset: the effects
// die with the session (the server never persists them).
func TestBuffsClearedBySessionReset(t *testing.T) {
	bot := NewBot("test1")
	bot.SetBuffs([]BuffEntry{{SkillID: 91, Level: 1, Time: 1200}})
	require.True(t, bot.SelfHasBuff(91))

	bot.ResetSession()
	require.False(t, bot.SelfHasBuff(91))
	require.Nil(t, bot.Snapshot().Buffs)
}

// TestBuffsJSONShape pins the encoded field names of the buffs view.
func TestBuffsJSONShape(t *testing.T) {
	bot := NewBot("test1")
	bot.SetBuffs([]BuffEntry{{SkillID: 91, Level: 1, Time: 1200}})

	encoded := bot.AppendSnapshotJSON(nil)
	require.Contains(t, string(encoded), `"buffs":[{"skillId":91,`)
	require.Contains(t, string(encoded), `"level":1`)
	require.Contains(t, string(encoded), `"name":"Defense Aura"`)
	require.Contains(t, string(encoded), `"icon":"skill0091"`)
	require.Contains(t, string(encoded), `"left":1200`)
}

// TestSelfManaPercent pins the mana percent accessor.
func TestSelfManaPercent(t *testing.T) {
	bot := NewBot("test1")
	require.InDelta(t, 0, bot.SelfManaPercent(), 0.0001)

	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1,
		MaxMP: 100, CurMP: 42,
	})
	require.InDelta(t, 42.0, bot.SelfManaPercent(), 0.0001)
}

// TestSelfWeaponKind pins the weapon family accessor: the right hand
// paperdoll slot resolves through the item stats of the inventory.
func TestSelfWeaponKind(t *testing.T) {
	bot := NewBot("test1")
	kind, ok := bot.SelfWeaponKind()
	require.False(t, ok)
	require.Empty(t, kind)

	// A short sword (item id 1) in the right hand.
	bot.ApplyUserInfo(UserInfo{Name: "test1", Level: 5, ClassID: 18, Race: 1})
	bot.ApplyItemList([]InventoryItem{{
		ObjectID: 10, ItemID: 1, Count: 1,
	}})
	bot.ApplyPaperdoll(paperdollWithRHand(10))
	kind, ok = bot.SelfWeaponKind()
	require.True(t, ok)
	require.Equal(t, "SWORD", kind)
}

// paperdollWithRHand builds the paperdoll array with the right hand
// object id set.
func paperdollWithRHand(objectID int32) [PaperdollSlots]int32 {
	var slots [PaperdollSlots]int32
	slots[PaperdollRHand] = objectID

	return slots
}
