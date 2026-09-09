// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// ShoppingEntryView is one planned purchase of the shop strategy: the
// item the bot intends to buy, the merchant it comes from, the buy
// price with the shop tax and the planning bookkeeping (the score
// gain, the sell credit of the displaced gear, the affordability
// against the adena the plan was computed with and the adena still
// missing for the wanted entries beyond the wallet). The combat stat
// fields mirror InventoryItemSnapshot so the web tooltip of the shop
// widget reuses the item tooltip shape (the family resolution and the
// stat lines of the classic client tooltip).
type ShoppingEntryView struct {
	ItemID      int32   `json:"itemId"`
	Name        string  `json:"name"`
	Icon        string  `json:"icon"`
	MerchantID  int32   `json:"merchantId"`
	Merchant    string  `json:"merchant"`
	Type        string  `json:"type"`
	WeaponType  string  `json:"weaponType"`
	ArmorType   string  `json:"armorType"`
	BodyPartKey string  `json:"bodyPartKey"`
	PAtk        int32   `json:"pAtk"`
	MAtk        int32   `json:"mAtk"`
	PDef        int32   `json:"pDef"`
	MDef        int32   `json:"mDef"`
	SDef        int32   `json:"sDef"`
	RShld       int32   `json:"rShld"`
	PAtkSpd     int32   `json:"pAtkSpd"`
	SoulShots   int32   `json:"soulShots"`
	SpiritShots int32   `json:"spiritShots"`
	Weight      int32   `json:"weight"`
	Price       int64   `json:"price"`
	SellCredit  int64   `json:"sellCredit"`
	Missing     int64   `json:"missing"`
	Gain        float64 `json:"gain"`
	Affordable  bool    `json:"affordable"`
	Buying      bool    `json:"buying"`
	Reason      string  `json:"reason"`
}

// ShoppingPlanView is the published shopping queue of the web UI: the
// purchase entries in the order the strategy wants them (the
// affordable plan of the next trip first, then the wanted tail), the
// adena the plan was computed against and the total price of the
// affordable entries. Trip marks a view that mirrors the buys of a
// running town trip instead of the trigger plan (the in-flight batch
// entries carry Buying).
type ShoppingPlanView struct {
	Entries []ShoppingEntryView `json:"entries"`
	Adena   int64               `json:"adena"`
	Total   int64               `json:"total"`
	Trip    bool                `json:"trip"`
}

// shoppingPlanTTL bounds how long a published shopping plan survives
// without a refresh: the hunt loop republishes on every tick (well
// under a second), so an expired plan means the loop moved on (or
// died) and the snapshot falls back to null like an expired walk
// plan. The window is generous against a slow tick (a heavy geodata
// leg) because a stale queue misleads far less than a stale walk
// line.
const shoppingPlanTTL = 10 * time.Second

// SetShoppingPlan publishes the shopping queue of the shop strategy so
// the web UI can show what the bot plans to buy next. An empty view
// clears the plan. A repeated identical view only refreshes its
// lifetime: the periodic republish of the hunt loop never churns the
// event stream. The entries slice is copied defensively - the caller
// keeps its own queue state after the publish.
func (b *Bot) SetShoppingPlan(view ShoppingPlanView) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(view.Entries) == 0 {
		b.clearShoppingPlanLocked()

		return
	}
	if shoppingPlanEqual(b.shopping, view) {
		b.shoppingAt = time.Now()

		return
	}
	entries := make([]ShoppingEntryView, len(view.Entries))
	copy(entries, view.Entries)
	b.shopping = &ShoppingPlanView{
		Entries: entries,
		Adena:   view.Adena,
		Total:   view.Total,
		Trip:    view.Trip,
	}
	b.shoppingAt = time.Now()
	b.touch()
}

// ClearShoppingPlan drops the published shopping plan (a no-op when
// none is published).
func (b *Bot) ClearShoppingPlan() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clearShoppingPlanLocked()
}

// clearShoppingPlanLocked drops the shopping plan, the caller must
// hold the state write lock.
func (b *Bot) clearShoppingPlanLocked() {
	if b.shopping == nil {
		return
	}
	b.shopping = nil
	b.shoppingAt = time.Time{}
	b.touch()
}

// shoppingPlanLive reports whether the published shopping plan is
// fresh enough for the snapshot: nil plans answer false, expired ones
// too (the hunt loop died or moved on). The caller must hold a lock.
func (b *Bot) shoppingPlanLive(now time.Time) bool {
	return b.shopping != nil && now.Sub(b.shoppingAt) <= shoppingPlanTTL
}

// shoppingPlanEqual compares a published plan with a fresh view
// element wise (a nil plan never equals a non empty view).
func shoppingPlanEqual(
	published *ShoppingPlanView, view ShoppingPlanView,
) bool {
	if published == nil ||
		len(published.Entries) != len(view.Entries) ||
		published.Adena != view.Adena ||
		published.Total != view.Total ||
		published.Trip != view.Trip {
		return false
	}
	for index := range published.Entries {
		if published.Entries[index] != view.Entries[index] {
			return false
		}
	}

	return true
}
