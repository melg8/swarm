// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// shoppingView builds a two entry shopping plan view: an affordable
// buy and a wanted one with a missing amount.
func shoppingView() ShoppingPlanView {
	return ShoppingPlanView{
		Entries: []ShoppingEntryView{
			{
				ItemID: 1121, Name: "Apprentice's Shoes",
				Icon: "armor_t01_b_i00", MerchantID: 7148,
				Merchant: "Ariel", Type: "Armor", ArmorType: "LIGHT",
				BodyPartKey: "feet", PDef: 8, Weight: 210, Price: 9,
				Gain: 8, Affordable: true,
				Reason: "buying Apprentice's Shoes (+8 for 9 adena)",
			},
			{
				ItemID: 1, Name: "Short Sword",
				Icon: "weapon_small_sword_i00", MerchantID: 7147,
				Merchant: "Unoren", Type: "Weapon", WeaponType: "SWORD",
				BodyPartKey: "rhand", PAtk: 8, Weight: 1600, Price: 883,
				Missing: 383, Gain: 3.5,
				Reason: "buying Short Sword (+3.5 for 883 adena)",
			},
		},
		Adena: 500,
		Total: 9,
	}
}

// TestSetShoppingPlanPublishesTheSnapshot pins the publish path: the
// plan lands in the snapshot, an identical republish keeps the state
// version (the periodic refresh never churns the event stream) and a
// changed plan bumps it.
func TestSetShoppingPlanPublishesTheSnapshot(t *testing.T) {
	bot := NewBot("test1")
	require.Nil(t, bot.Snapshot().Shopping,
		"no plan is published before the loop starts")

	bot.SetShoppingPlan(shoppingView())
	snap := bot.Snapshot()
	require.NotNil(t, snap.Shopping)
	require.Equal(t, shoppingView(), *snap.Shopping)
	version := bot.Version()

	bot.SetShoppingPlan(shoppingView())
	require.Equal(t, version, bot.Version(),
		"an identical republish is a no-op")

	view := shoppingView()
	view.Adena = 900
	bot.SetShoppingPlan(view)
	require.Greater(t, bot.Version(), version,
		"a changed plan bumps the state version")
	require.Equal(t, int64(900), bot.Snapshot().Shopping.Adena)

	bot.ClearShoppingPlan()
	require.Nil(t, bot.Snapshot().Shopping)
	cleared := bot.Version()
	bot.ClearShoppingPlan()
	require.Equal(t, cleared, bot.Version(),
		"clearing without a plan is a no-op")
}

// TestSetShoppingPlanEmptyViewClears pins the empty view convention:
// the hunt loop publishes an empty queue when the shopping is off and
// the tracker drops the plan instead of storing an empty shell.
func TestSetShoppingPlanEmptyViewClears(t *testing.T) {
	bot := NewBot("test1")
	bot.SetShoppingPlan(shoppingView())
	require.NotNil(t, bot.Snapshot().Shopping)
	bot.SetShoppingPlan(ShoppingPlanView{})
	require.Nil(t, bot.Snapshot().Shopping)
}

// TestResetSessionClearsShoppingPlan pins the session reset: a
// reconnecting session publishes a fresh plan of its own, the plan of
// the lost session must not leak into its snapshots.
func TestResetSessionClearsShoppingPlan(t *testing.T) {
	bot := NewBot("test1")
	bot.SetShoppingPlan(shoppingView())
	require.NotNil(t, bot.Snapshot().Shopping)
	bot.ResetSession()
	require.Nil(t, bot.Snapshot().Shopping)
}

// TestShoppingPlanExpiryPinsTheTTL pins the freshness guard: a plan
// older than shoppingPlanTTL leaves the snapshot (the hunt loop died
// or moved on) exactly like an expired walk plan.
func TestShoppingPlanExpiryPinsTheTTL(t *testing.T) {
	bot := NewBot("test1")
	bot.SetShoppingPlan(shoppingView())
	require.NotNil(t, bot.Snapshot().Shopping)

	bot.mu.Lock()
	bot.shoppingAt = bot.shoppingAt.Add(-shoppingPlanTTL - 1)
	bot.mu.Unlock()
	require.Nil(t, bot.Snapshot().Shopping,
		"an expired plan is absent from the snapshot")
}

// TestSetShoppingPlanCopiesEntries pins the defensive copy: the
// caller mutating its queue slice after the publish must not change
// the published view (the hunt loop keeps its cache in place).
func TestSetShoppingPlanCopiesEntries(t *testing.T) {
	bot := NewBot("test1")
	view := shoppingView()
	entries := view.Entries
	bot.SetShoppingPlan(view)
	entries[0].Price = 12345
	require.Equal(t, int64(9), bot.Snapshot().Shopping.Entries[0].Price,
		"the published entries are a copy, not the caller slice")
}
