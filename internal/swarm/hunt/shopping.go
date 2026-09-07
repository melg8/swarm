// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"sort"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// Shopping of the town trips: the gear.PlanPurchases strategy decides
// what to buy (the greedy value per adena planner against the
// equipment and the adena), the trip visits the merchants of the plan
// after selling the junk (the fresh adena of the sales re-plans the
// purchases at the shop) and buys one buylist per transaction request
// (buying shares the transaction flood protector with selling).

// Timing and threshold constants of the shopping.
const (
	// buyPause paces the buy requests after the transaction flood
	// protector of the server (10 seconds by default, shared with
	// the sell batches).
	buyPause = 11 * time.Second
	// shoppingPlanPeriod bounds the shopping trigger re-plans: the
	// adena and the inventory change with every loot, the plan for
	// the trip trigger is cached for this period.
	shoppingPlanPeriod = 5 * time.Second
	// shoppingTripMinValue is the minimum total price of a plan that
	// justifies a shopping trip on its own (a trip without it would
	// walk to town for a handful of adena).
	shoppingTripMinValue = 100
	// townTaxRate is the buy tax markup of the elven village
	// merchants (baseTax 15 percent, no castle owns their tax on a
	// fresh server). A future region config carries its own rate.
	townTaxRate = 0.15
)

// tripStop is one merchant visit of a town trip: the merchant to
// walk to, the purchases to buy there (grouped by their list ids)
// and whether the junk selling happens at this stop.
type tripStop struct {
	merchant townNpc
	buys     []gear.Purchase
	sell     bool
}

// shopCatalog builds the gear catalog of the town merchants from the
// generated buylist data: every merchant of the trip targets sells
// its buylists at the town tax rate.
func shopCatalog(merchants []townNpc) gear.Catalog {
	shops := make([]gear.Shop, 0, len(merchants))
	for _, merchant := range merchants {
		lists := npcdata.BuyListsOfNPC(merchant.TemplateID)
		if len(lists) == 0 {
			continue
		}
		shops = append(shops, gear.Shop{
			MerchantTemplateID: merchant.TemplateID,
			TaxRate:            townTaxRate,
			Lists:              lists,
		})
	}

	return gear.Catalog{Shops: shops}
}

// shoppingPlan plans the purchases against the current gear state.
// The gear profile of the loop scores the items.
func (l *Loop) shoppingPlan() []gear.Purchase {
	if l.equip == nil {
		return nil
	}
	stats := l.tracker.InventoryStats()

	return gear.PlanPurchases(
		l.equip.profile, l.equipment(), shopCatalog(townMerchants),
		int64(stats.Adena))
}

// shoppingWanted reports whether the shop strategy justifies a town
// trip on its own: a cached plan with a total price above the trip
// minimum. The inventory full trigger runs independently of it.
func (l *Loop) shoppingWanted() bool {
	now := time.Now()
	if l.shoppingPlanAt.IsZero() ||
		now.Sub(l.shoppingPlanAt) >= shoppingPlanPeriod {
		l.shoppingPlanCache = l.shoppingPlan()
		l.shoppingPlanAt = now
	}
	if len(l.shoppingPlanCache) == 0 {
		return false
	}

	return gear.AdenaSpent(l.shoppingPlanCache) >= shoppingTripMinValue
}

// planShoppingStops plans the buy stops of the running trip with the
// fresh adena of the completed selling: the purchases group by their
// merchant, the groups order by the walking distance from the
// character, and a group of the merchant the character already
// stands at merges into the current stop.
func (l *Loop) planShoppingStops() {
	l.buysPlanned = true
	purchases := l.shoppingPlan()
	if len(purchases) == 0 {
		l.logger.Printf("Hunt: shop: nothing worth buying after the " +
			"sales, heading back")

		return
	}
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		selfX, selfY = 0, 0
	}
	// Group the purchases by merchant.
	groups := make(map[int32][]gear.Purchase)
	for _, purchase := range purchases {
		groups[purchase.MerchantTemplateID] = append(
			groups[purchase.MerchantTemplateID], purchase)
	}
	type merchantGroup struct {
		merchant townNpc
		buys     []gear.Purchase
	}
	stops := make([]merchantGroup, 0, len(groups))
	for templateID, buys := range groups {
		merchant, ok := merchantByTemplate(templateID)
		if !ok {
			l.logger.Printf("Hunt: shop: no known merchant for template "+
				"%d, skipping %d purchases", templateID, len(buys))
			continue
		}
		stops = append(stops, merchantGroup{merchant: merchant, buys: buys})
	}
	sort.Slice(stops, func(i int, j int) bool {
		di := math.Hypot(
			float64(stops[i].merchant.X-selfX),
			float64(stops[i].merchant.Y-selfY))
		dj := math.Hypot(
			float64(stops[j].merchant.X-selfX),
			float64(stops[j].merchant.Y-selfY))

		return di < dj
	})
	total := 0
	for _, stop := range stops {
		total += len(stop.buys)
	}
	l.logger.Printf("Hunt: shop: planning to buy %d items from %d "+
		"merchants", total, len(stops))
	// The first group of the merchant the character stands at (the
	// sell stop) buys right here.
	if len(stops) > 0 && len(l.tripStops) > 0 &&
		stops[0].merchant.TemplateID ==
			l.tripStops[0].merchant.TemplateID {
		l.tripStops[0].buys = stops[0].buys
		stops = stops[1:]
	}
	for _, stop := range stops {
		l.tripStops = append(l.tripStops, tripStop{
			merchant: stop.merchant,
			buys:     stop.buys,
			sell:     false,
		})
	}
}

// merchantByTemplate finds the town merchant of the packet template
// id.
func merchantByTemplate(templateID int32) (townNpc, bool) {
	for _, merchant := range townMerchants {
		if merchant.TemplateID == templateID {
			return merchant, true
		}
	}

	return townNpc{}, false
}

// stopMerchantTemplates lists the packet template ids of the current
// trip stop merchant (offset by the display id base).
func (l *Loop) stopMerchantTemplates() []int32 {
	if len(l.tripStops) == 0 {
		return merchantTemplates()
	}

	return []int32{
		l.tripStops[0].merchant.TemplateID + npcDisplayOffset,
	}
}

// tickStopShopping buys the purchases of the current stop: the
// merchant must be selected and in range for the buy requests (the
// server checks the target), one buylist per transaction request and
// one request per flood protector window. It reports true when every
// purchase of the stop was requested and the trip may advance.
func (l *Loop) tickStopShopping(now time.Time) bool {
	if len(l.tripStops) == 0 {
		return true
	}
	stop := l.tripStops[0]
	if len(stop.buys) == 0 {
		return true
	}
	if !l.handleMerchant(now, l.stopMerchantTemplates()) {
		return false
	}
	if !l.buyAt.IsZero() && now.Sub(l.buyAt) < buyPause {
		return false
	}
	// One buylist per request: the purchases of the first list id of
	// the stop go out together.
	listID := stop.buys[0].ListID
	batch := make([]gear.Purchase, 0, len(stop.buys))
	remaining := make([]gear.Purchase, 0, len(stop.buys))
	for _, purchase := range stop.buys {
		if purchase.ListID == listID {
			batch = append(batch, purchase)
		} else {
			remaining = append(remaining, purchase)
		}
	}
	if err := l.game.BuyItems(listID, batch); err != nil {
		l.logger.Printf("Hunt: shop: buy request failed: %v", err)

		return false
	}
	l.buyAt = now
	l.tripStops[0].buys = remaining
	names := make([]string, 0, len(batch))
	for _, purchase := range batch {
		names = append(names, npcdata.ItemName(purchase.ItemID))
	}
	l.logger.Printf("Hunt: shop: buying %d items from %s (list %d)",
		len(batch), stop.merchant.Name, listID)

	return len(remaining) == 0
}

// advanceTripStop finishes the current stop and walks to the next
// one (a buy stop or the return leg when none is left).
func (l *Loop) advanceTripStop() {
	if len(l.tripStops) > 0 {
		l.tripStops = l.tripStops[1:]
	}
	if len(l.tripStops) == 0 {
		l.startReturnLeg()

		return
	}
	l.phase = phaseTownWalk
	l.merchantID = 0
	l.merchantPick = time.Time{}
	stop := l.tripStops[0]
	l.logger.Printf("Hunt: shop: walking to %s", stop.merchant.Name)
	if !l.startWalkLeg(townNpcPosition(stop.merchant)) {
		l.abortTownTrip("no walkable path to the shop of " +
			stop.merchant.Name)
	}
}

// shoppingTripEnabled reports whether the shopping trigger may arm:
// the loop needs the gear profile (the strategy scores through it)
// and the known merchants must sell something (the generated
// catalogs).
func (l *Loop) shoppingTripEnabled() bool {
	return l.equip != nil &&
		len(shopCatalog(townMerchants).Shops) > 0
}

// stopBuysPending reports whether the current stop still wants buys.
func (l *Loop) stopBuysPending() bool {
	return len(l.tripStops) > 0 && len(l.tripStops[0].buys) > 0
}

// resetStopBuys drops the pending buys of the current stop (a
// merchant that never showed up: the server refuses buys without the
// selected merchant target).
func (l *Loop) resetStopBuys(reason string) {
	if len(l.tripStops) == 0 {
		return
	}
	l.logger.Printf("Hunt: shop: %s, skipping %d purchases", reason,
		len(l.tripStops[0].buys))
	l.tripStops[0].buys = nil
}

// sellableStop reports whether the current stop sells the junk.
func (l *Loop) sellableStop() bool {
	return len(l.tripStops) > 0 && l.tripStops[0].sell
}

// state import marker: the trip keeps the inventory items through
// the tracker.
var _ = state.InventoryItem{}
