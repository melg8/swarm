// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"sort"
	"strings"
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
	// protector of the server (the transaction window is 10 game
	// ticks = 1 second wide, shared with the sell batches; the pause
	// keeps the proven generous margin - a refused transaction does
	// not extend the window and the punishment config may kick
	// repeat offenders).
	buyPause = 11 * time.Second
	// buyConfirmWait bounds the wait for the inventory update that
	// confirms a buy request landed: the server answers a refused
	// transaction SILENTLY (the flood refusal is a chat message, the
	// range refusal a bare ActionFailed - neither references the
	// request), so the loop watches the inventory for the bought
	// item ids instead and re-requests the batch when they never
	// show up.
	buyConfirmWait = 15 * time.Second
	// stopBuyRetries bounds the re-requests of a lost buy batch
	// before the trip gives the purchases up.
	stopBuyRetries = 3
	// replaceSellTimeout bounds the wait for the inventory update
	// that confirms the replacement sales: the junk flow batches the
	// pieces at its own pace, and a piece the server refuses to sell
	// never vanishes - the trip proceeds without its credit after the
	// wait instead of stalling.
	replaceSellTimeout = 60 * time.Second
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
// walk to, the purchases to buy there (grouped by their list ids),
// whether the junk selling happens at this stop and whether this is
// the skill teacher stop that learns the queued lessons (see
// learning.go).
type tripStop struct {
	merchant townNpc
	buys     []gear.Purchase
	sell     bool
	teach    bool
}

// townShopCatalog is the static gear catalog of the town merchants,
// built once from the generated buylist data: the town trip trigger
// consults it on every hunt tick, so the per tick catalog build of
// the shops slice was pure allocation churn.
var townShopCatalog = shopCatalog(townMerchants)

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
// The gear profile of the loop scores the items; the returned plan
// holds only the affordable purchases (the wanted tail of the queue
// is the widget's save up view, the trip never buys it).
func (l *Loop) shoppingPlan() []gear.Purchase {
	return affordablePrefix(l.shoppingQueue())
}

// shoppingQueue computes the fresh purchase queue against the current
// gear state: the affordable plan of the next trip plus the wanted
// tail with the cumulative missing adena (see
// gear.PlanPurchaseQueue). The character level drives the jewel
// upgrade gate of the strategy (the cheapest set serves until level
// 15, the upgrades open past it); the armor floor and the weapon
// milestone order the opening game (the jewels wait for the filled
// armor slots and the worn weapon).
func (l *Loop) shoppingQueue() []gear.Purchase {
	if l.equip == nil {
		return nil
	}
	stats := l.tracker.InventoryStats()

	return gear.PlanPurchaseQueue(
		l.equip.profile, l.equipment(), townShopCatalog,
		int64(stats.Adena), l.tracker.SelfLevel())
}

// refreshShoppingCache recomputes the cached purchase queue when the
// shopping plan period elapsed. The cache drives both the trip
// trigger (the affordable prefix, see shoppingWanted) and the widget
// view of the loop publish, so one recompute serves both per period.
// The adena the queue was planned against is cached with it: the
// widget view must show the planning wallet, not a drifted one.
// The built ShoppingPlanView is cached alongside the plan so the per
// tick publish does not rebuild it (the rebuild was the dominant
// allocation source of the 100 bot fleet under live profiling).
func (l *Loop) refreshShoppingCache() {
	now := time.Now()
	if !l.shoppingPlanAt.IsZero() &&
		now.Sub(l.shoppingPlanAt) < shoppingPlanPeriod {
		return
	}
	l.shoppingPlanCache = l.shoppingQueue()
	l.shoppingPlanAt = now
	l.shoppingPlanAdena = int64(l.tracker.InventoryStats().Adena)
	l.shoppingViewCache = shoppingQueueView(
		l.shoppingPlanCache, l.shoppingPlanAdena)
	// The fresh plan re-feeds the weapon priority of the learning
	// queue: the planned next weapon purchase swaps the preferred
	// weapon family of the lesson order (see combat_skills.go).
	l.publishSkillWeaponPriority()
}

// shoppingWanted reports whether the shop strategy justifies a town
// trip on its own: a cached plan with a total price above the trip
// minimum. The inventory full trigger runs independently of it.
// The affordable total is summed directly over the cached plan
// without allocating an affordablePrefix slice: the per tick call
// was allocating a []Purchase on every bot every 200ms (4.5 MB over
// a 3 minute fleet run) just to sum prices and throw the slice away.
func (l *Loop) shoppingWanted() bool {
	l.refreshShoppingCache()
	if len(l.shoppingPlanCache) == 0 {
		return false
	}
	var total int64
	for _, purchase := range l.shoppingPlanCache {
		if !purchase.Affordable {
			break
		}
		total += purchase.Price
	}

	return total >= shoppingTripMinValue
}

// affordablePrefix filters the affordable buys of a purchase queue:
// the trip executes only them, the wanted tail is the widget's save
// up view.
func affordablePrefix(queue []gear.Purchase) []gear.Purchase {
	buys := make([]gear.Purchase, 0, len(queue))
	for _, purchase := range queue {
		if !purchase.Affordable {
			break
		}
		buys = append(buys, purchase)
	}

	return buys
}

// publishShoppingView refreshes the shopping queue of the web UI
// widget on every tick: the cached queue view while the loop hunts
// (a fresh recompute per shoppingPlanPeriod through the shared cache)
// and the remaining trip buys while a town trip runs (the in-flight
// batch marked buying). Sessions without the shop strategy (no gear
// profile, no merchant catalogs, the manual only mode) publish
// nothing - the widget stays hidden. The built view is reused from
// the cache between recomputes so the per tick publish pays no
// allocation.
func (l *Loop) publishShoppingView() {
	if !l.autonomous || !l.shoppingTripEnabled() {
		l.tracker.ClearShoppingPlan()

		return
	}
	if l.tripActive() {
		l.tracker.SetShoppingPlan(l.tripShoppingView())

		return
	}
	l.refreshShoppingCache()
	l.tracker.SetShoppingPlan(l.shoppingViewCache)
}

// shoppingQueueView builds the widget view of a purchase queue: the
// entries in the walked order (the affordable plan first, the wanted
// tail behind) with the planning adena and the affordable total.
func shoppingQueueView(
	queue []gear.Purchase, adena int64,
) state.ShoppingPlanView {
	var total int64
	entries := make([]state.ShoppingEntryView, 0, len(queue))
	for _, purchase := range queue {
		if purchase.Affordable {
			total += purchase.Price
		}
		entries = append(entries, shoppingEntryView(purchase, false))
	}

	return state.ShoppingPlanView{
		Entries: entries,
		Adena:   adena,
		Total:   total,
		Trip:    false,
	}
}

// tripShoppingView builds the widget view of a running town trip: the
// in-flight buy batch (marked buying) first, then the pending
// purchases of the current stop and the later stops. The entries are
// the trip's own plan, the affordable fields of the walker hold.
func (l *Loop) tripShoppingView() state.ShoppingPlanView {
	var total int64
	entries := make([]state.ShoppingEntryView, 0, 16)
	for _, purchase := range l.buyRequested {
		total += purchase.Price
		entries = append(entries, shoppingEntryView(purchase, true))
	}
	for _, stop := range l.tripStops {
		for _, purchase := range stop.buys {
			total += purchase.Price
			entries = append(entries, shoppingEntryView(
				purchase, false))
		}
	}

	return state.ShoppingPlanView{
		Entries: entries,
		Adena:   int64(l.tracker.InventoryStats().Adena),
		Total:   total,
		Trip:    true,
	}
}

// shoppingEntryView converts one planned purchase into the tracker
// view of the shop widget: the item stats resolve through the
// generated npcdata dictionaries (name, icon, merchant, combat
// stats) so the web tooltip of the widget reuses the item tooltip
// shape.
func shoppingEntryView(
	purchase gear.Purchase, buying bool,
) state.ShoppingEntryView {
	stats, hasStats := npcdata.ItemGearStats(purchase.ItemID)
	itemType := stats.Type
	if !hasStats {
		itemType = npcdata.ItemType(purchase.ItemID)
	}

	return state.ShoppingEntryView{
		ItemID:     purchase.ItemID,
		Name:       npcdata.ItemName(purchase.ItemID),
		Icon:       npcdata.ItemIcon(purchase.ItemID),
		MerchantID: purchase.MerchantTemplateID,
		Merchant: npcdata.NPCName(
			purchase.MerchantTemplateID + npcDisplayOffset),
		Type:        itemType,
		WeaponType:  stats.WeaponType,
		ArmorType:   stats.ArmorType,
		BodyPartKey: stats.BodyPart,
		PAtk:        stats.PAtk,
		MAtk:        stats.MAtk,
		PDef:        stats.PDef,
		MDef:        stats.MDef,
		SDef:        stats.SDef,
		RShld:       stats.RShld,
		PAtkSpd:     stats.PAtkSpd,
		SoulShots:   stats.SoulShots,
		SpiritShots: stats.SpiritShots,
		Weight:      npcdata.ItemWeight(purchase.ItemID),
		Price:       purchase.Price,
		SellCredit:  purchase.SellCredit,
		Missing:     purchase.Missing,
		Gain:        purchase.Gain,
		Affordable:  purchase.Affordable,
		Buying:      buying,
		Reason:      purchase.Reason,
	}
}

// replacementSellingActive reports whether the sell first step of
// the replacement purchases is still in flight: the auto equipment
// must not re-equip the pieces the step just unequipped for their
// sale (the empty slot would pull them right back on).
func (l *Loop) replacementSellingActive() bool {
	return len(l.replaceQueue) > 0 || len(l.replaceSelling) > 0
}

// stepReplacementSales runs the sell first step of the replacement
// purchases: the plan credits the sell value of the equipped pieces
// its buys displace (see gear.PlanPurchases), and the trip actually
// banks that credit - every displaced piece is unequipped, sold to
// the merchant, and only then the buys run and the auto equipment
// wears the replacements into the freed slots. One step per tick;
// reports false while the step is still busy (a unequip in flight,
// the sale batch waiting for its transaction window or its
// inventory confirmation).
func (l *Loop) stepReplacementSales(now time.Time) bool {
	if !l.replacePlanned {
		l.replacePlanned = true
		l.replaceQueue = l.replacementTargets()
		if len(l.replaceQueue) == 0 {
			return true
		}
		l.logger.Printf("Hunt: shop: %d equipped pieces feed the "+
			"replacements, selling them first", len(l.replaceQueue))

		return false
	}
	if !l.replaceUnequipsDone(now) {
		return false
	}
	if !l.replaceOfferDone(now) {
		return false
	}

	return l.replaceSalesSettled(now)
}

// replaceUnequipsDone drives the unequip phase of the sell first
// step: every queued piece comes off (paced like the auto equipment,
// retried twice before the piece stays on and the buy runs without
// its credit). An unequipped piece turns into a plain sellable
// candidate and joins the sale list - the junk flow of the stop or
// the offer batch below sells it. Reports false while a unequip is
// still in flight, true when the queue is drained.
func (l *Loop) replaceUnequipsDone(now time.Time) bool {
	for len(l.replaceQueue) > 0 {
		head := l.replaceQueue[0]
		if l.replaceHeadSettled(head) {
			continue
		}
		if !l.replaceHeadOff(head, now) {
			return false
		}
		l.dropReplacementHead()
	}

	return true
}

// replaceHeadSettled reports whether the queued head needs no
// unequip request anymore and drops it from the queue: offered by
// the junk flow already, gone from the inventory, or unequipped
// (the piece joins the sale list for the settle wait).
func (l *Loop) replaceHeadSettled(head int32) bool {
	if l.sold[head] {
		l.dropReplacementHead()

		return true
	}
	item, ok := l.tracker.InventoryItemState(head)
	if !ok {
		l.dropReplacementHead()

		return true
	}
	if item.Equipped {
		return false
	}
	l.replaceQueue = l.replaceQueue[1:]
	l.replaceSelling = append(l.replaceSelling, item)
	l.replaceTried = 0

	return true
}

// replaceHeadOff sends the paced unequip request for the queued
// piece and reports whether the phase still waits on it. The request
// repeats every equip period, twice per piece before it is given up
// (the buy then runs without its credit - the server swap semantics
// still replace the piece).
func (l *Loop) replaceHeadOff(head int32, now time.Time) bool {
	if !l.replaceUnequipAt.IsZero() &&
		now.Sub(l.replaceUnequipAt) < equipActionPeriod {
		return false
	}
	if l.replaceTried >= 2 {
		l.logger.Printf("Hunt: shop: item %d does not come off, "+
			"buying without its credit", head)

		return true
	}
	l.replaceUnequipAt = now
	l.replaceTried++
	l.logger.Printf("Hunt: shop: unequipping the replaced item %d",
		head)
	if err := l.game.UseItem(head); err != nil {
		l.logger.Printf("Hunt: shop: unequip of %d failed: %v",
			head, err)

		return false
	}

	return false
}

// dropReplacementHead drops the head of the replacement queue with
// its retry budget.
func (l *Loop) dropReplacementHead() {
	l.replaceQueue = l.replaceQueue[1:]
	l.replaceTried = 0
}

// replaceOfferDone drives the sale phase: the handed pieces the junk
// flow has not offered yet go out as one batch (the transaction
// window paces it through l.sellAt, the buys wait it out behind the
// same window). Reports false while the offer is pending, true when
// the offer phase concluded - the own batch went out or the junk
// flow owns every piece.
func (l *Loop) replaceOfferDone(now time.Time) bool {
	if l.replaceSellSent || len(l.replaceSelling) == 0 {
		return true
	}
	batch := make([]state.InventoryItem, 0, len(l.replaceSelling))
	for _, item := range l.replaceSelling {
		if !l.sold[item.ObjectID] {
			batch = append(batch, item)
		}
	}
	if len(batch) == 0 {
		// The junk flow already offered every handed piece: the
		// settle phase waits out their removals.
		l.replaceSellSent = true

		return true
	}
	if !l.sellAt.IsZero() && now.Sub(l.sellAt) < sellPause {
		return false
	}
	if err := l.game.SellItems(batch); err != nil {
		l.logger.Printf("Hunt: shop: replacement sell failed: %v", err)

		return false
	}
	for _, item := range batch {
		l.sold[item.ObjectID] = true
	}
	l.sellAt = now
	l.replaceSellSent = true
	l.logger.Printf("Hunt: shop: offered %d replaced pieces for sale",
		len(batch))

	return true
}

// replaceSalesSettled waits for the inventory update that confirms
// the replacement sales: the proceeds land with it, and the buys
// re-plan against the fresh adena only after that - planning earlier
// would drop the replacement (its credit is spent, the adena has not
// arrived). A piece the server refuses to sell never vanishes, so
// the wait is bounded and the trip proceeds without its credit.
func (l *Loop) replaceSalesSettled(now time.Time) bool {
	if len(l.replaceSelling) == 0 {
		return true
	}
	if l.replaceWaitAt.IsZero() {
		l.replaceWaitAt = now
	}
	if now.Sub(l.replaceWaitAt) < replaceSellTimeout {
		for _, item := range l.replaceSelling {
			if _, ok := l.tracker.InventoryItemState(item.ObjectID); ok {
				return false
			}
		}
	}
	l.replaceSelling = nil

	return true
}

// resetReplacementSales drops the sell first step state: a fresh
// trip re-plans the replacement sales from the current gear state,
// an aborted trip leaves no half-sold queue behind that the auto
// equipment would have to steer around.
func (l *Loop) resetReplacementSales() {
	l.replacePlanned = false
	l.replaceDone = false
	l.replaceQueue = nil
	l.replaceSelling = nil
	l.replaceSellSent = false
	l.replaceUnequipAt = time.Time{}
	l.replaceWaitAt = time.Time{}
	l.replaceTried = 0
}

// replacementTargets collects the equipped object ids the planned
// purchases displace: the plan runs against the current gear state
// and carries the SellFirst list on every replacing purchase.
func (l *Loop) replacementTargets() []int32 {
	purchases := l.shoppingPlan()
	if len(purchases) == 0 {
		return nil
	}
	seen := make(map[int32]bool)
	var targets []int32
	for _, purchase := range purchases {
		for _, objectID := range purchase.SellFirst {
			if objectID == 0 || seen[objectID] {
				continue
			}
			seen[objectID] = true
			targets = append(targets, objectID)
		}
	}

	return targets
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
			teach:    false,
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

	return zeroTownNpc, false
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
// merchant must be selected and in 3D range for the buy requests (the
// server checks both), one buylist per transaction request and one
// request per flood protector window. Every sent batch waits for the
// inventory update that carries its items: a refused transaction (the
// flood window of a just finished sell, a selection reset, a range
// race) answers silently, so the batch is re-requested up to
// stopBuyRetries times before the stop gives it up. It reports true
// when every purchase of the stop was requested AND confirmed and the
// trip may advance.
// Pre-split debt of the buy confirmation gate (extract the gate into
// its own step when the stop pipeline is refactored).
func (l *Loop) tickStopShopping(now time.Time) bool { //nolint:cyclop,funlen
	if len(l.tripStops) == 0 {
		return true
	}
	stop := l.tripStops[0]
	if len(stop.buys) == 0 && len(l.buyRequested) == 0 {
		return true
	}
	if !l.handleMerchant(now, l.stopMerchantTemplates()) {
		return false
	}
	// The confirmation gate of the in-flight batch: the arrived items
	// complete it, the deadline re-requests it.
	if len(l.buyRequested) > 0 {
		switch {
		case l.buysArrived(l.buyRequested):
			l.logger.Printf("Hunt: shop: %d purchases confirmed",
				len(l.buyRequested))
			l.buyRequested = nil
			l.buyConfirmAt = time.Time{}
			l.buyRetries = 0
		case now.Sub(l.buyConfirmAt) < buyConfirmWait:
			return false
		default:
			l.buyRetries++
			if l.buyRetries > stopBuyRetries {
				l.logger.Printf("Hunt: shop: %d purchases never "+
					"arrived after %d requests, skipping them",
					len(l.buyRequested), l.buyRetries)
				l.buyRequested = nil
				l.buyConfirmAt = time.Time{}
				l.buyRetries = 0
			} else {
				l.logger.Printf("Hunt: shop: %d purchases did not "+
					"arrive, re-requesting (try %d of %d)",
					len(l.buyRequested), l.buyRetries, stopBuyRetries)
			}
		}
		if len(l.tripStops) == 0 ||
			(len(l.tripStops[0].buys) == 0 && len(l.buyRequested) == 0) {
			return true
		}
	}
	// The buy pacing shares the transaction window with the sells: the
	// first buy of a stop waits out the last sell batch as well (the
	// flood window is 1 s wide, the pause keeps the proven margin).
	last := l.buyAt
	if l.sellAt.After(last) {
		last = l.sellAt
	}
	if !last.IsZero() && now.Sub(last) < buyPause {
		return false
	}
	// One buylist per request: the purchases of the first list id of
	// the stop go out together, an in-flight retry re-sends its own
	// batch.
	batch := l.buyRequested
	remaining := make([]gear.Purchase, 0, len(stop.buys))
	var listID int32
	if len(batch) > 0 {
		listID = batch[0].ListID
	} else {
		listID = stop.buys[0].ListID
		for _, purchase := range stop.buys {
			if purchase.ListID == listID {
				batch = append(batch, purchase)
			} else {
				remaining = append(remaining, purchase)
			}
		}
		l.tripStops[0].buys = remaining
	}
	if err := l.game.BuyItems(listID, batch); err != nil {
		l.logger.Printf("Hunt: shop: buy request failed: %v", err)

		return false
	}
	l.buyAt = now
	l.buyRequested = batch
	l.buyConfirmAt = now
	names := make([]string, 0, len(batch))
	for _, purchase := range batch {
		names = append(names, npcdata.ItemName(purchase.ItemID))
	}
	l.logger.Printf("Hunt: shop: buying %d items from %s (list %d): %s",
		len(batch), stop.merchant.Name, listID, strings.Join(names, ", "))

	return len(l.tripStops) > 0 && len(l.tripStops[0].buys) == 0 &&
		len(l.buyRequested) == 0
}

// buysArrived reports whether every purchase of the batch shows up in
// the tracked inventory (the planner never buys an item id the
// inventory carries at the plan time, so the appearance of the ids is
// the arrival signal; an equipped purchase still counts - the auto
// equipment wears it within seconds).
func (l *Loop) buysArrived(batch []gear.Purchase) bool {
	items := l.tracker.InventoryItems()
	for _, purchase := range batch {
		found := false
		for _, item := range items {
			if item.ItemID == purchase.ItemID {
				found = true

				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// advanceTripStop finishes the current stop and walks to the next
// one (a buy stop or the return leg when none is left).
func (l *Loop) advanceTripStop() {
	if len(l.tripStops) > 0 {
		l.tripStops = l.tripStops[1:]
	}
	// A new stop starts with a fresh retry budget and no in-flight
	// batch (the previous stop only advances when its batch settled).
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.resetLearnState()
	if len(l.tripStops) == 0 {
		l.startReturnLeg()

		return
	}
	l.phase = phaseTownWalk
	l.merchantID = 0
	l.merchantPick = time.Time{}
	l.merchantDeckUntil = time.Time{}
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
	return l.equip != nil && len(townShopCatalog.Shops) > 0
}

// stopBuysPending reports whether the current stop still wants buys:
// unrequested purchases or an in-flight batch awaiting its arrival
// confirmation.
func (l *Loop) stopBuysPending() bool {
	return len(l.tripStops) > 0 &&
		(len(l.tripStops[0].buys) > 0 || len(l.buyRequested) > 0)
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
