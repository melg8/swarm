// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Shopping of the town trips: gear.PlanPurchases decides what to buy
// (the greedy value per adena planner), the trip FREEZES that plan at
// its start and executes it verbatim - the sell first step sells
// exactly the pieces the frozen plan counted on, the stops distribute
// exactly its purchases, and nothing re-plans in between. One buylist
// goes out per transaction request (buying shares the transaction
// flood protector with selling).

// Timing and threshold constants of the shopping.
const (
    // transactionPause is the shared pacing window of every buy and
    // sell request: FloodProtectorTransactionInterval = 10 game ticks
    // of 10 per second, plus a 250 ms margin over the decision tick.
    // An early request is refused with a chat message and costs
    // nothing (FloodProtectorTransactionPunishmentLimit = 0).
    transactionPause = 1250 * time.Millisecond
    // buyPause paces the buy requests through the shared transaction
    // window (see transactionPause and transactionWindowFree).
    buyPause = transactionPause
    // buyConfirmWait bounds the wait for the inventory update that
    // confirms a buy request: a refused transaction answers silently,
    // so the loop watches the inventory for the bought ids and
    // re-requests the batch when they never show up.
    buyConfirmWait = 15 * time.Second
    // stopBuyRetries bounds the re-requests of a lost buy batch
    // before the trip gives the purchases up.
    stopBuyRetries = 3
    // replaceSellTimeout bounds the wait for the sold pieces to leave
    // the inventory: the trip proceeds without the credit after it.
    replaceSellTimeout = 60 * time.Second
    // shoppingPlanPeriod bounds the shopping trigger re-plans: the
    // adena and the inventory change with every loot, the plan for
    // the trip trigger is cached for this period.
    shoppingPlanPeriod = 5 * time.Second
    // shoppingTripMinValue is the minimum total price of a plan that
    // justifies a shopping trip on its own (a trip without it would
    // walk to town for a handful of adena).
    shoppingTripMinValue = 100
    // weaponRunCooldown shortens the trip cooldown for the weapon
    // runs: a bare-handed character retries the errand within a
    // minute instead of the ordinary five.
    weaponRunCooldown = 45 * time.Second
    // townTaxRate is the buy tax markup of the elven village
    // merchants (baseTax 15 percent, no castle owns their tax on a
    // fresh server). A future region config carries its own rate.
    townTaxRate = 0.15
    // dionTownTaxRate is the buy tax markup of the Town of Dion
    // merchants (MerchantPriceConfig.xml priceConfig id=8 baseTax=20).
    dionTownTaxRate = 0.20
)

// tripStop is one merchant visit of a town trip: the merchant, the
// purchases to buy there, the junk selling flag, and the teacher or
// Newbie Guide stop kind (learning.go, guide_buffs.go).
type tripStop struct {
    merchant townNpc
    buys     []gear.Purchase
    sell     bool
    teach    bool
    guide    bool
}

// townShopCatalog is the static gear catalog of the elven village
// town merchants, built once from the generated buylist data.
var townShopCatalog = shopCatalog(townMerchants)

// dionShopCatalog is the static gear catalog of the Dion town
// merchants (the 20-25 band), built once like the elven catalog.
var dionShopCatalog = dionShopCatalogBuild(dionMerchants)

// dionShopCatalogBuild builds the Dion catalog at the 20 percent buy
// tax, unlike the elven 15 percent.
func dionShopCatalogBuild(merchants []townNpc) gear.Catalog {
    shops := make([]gear.Shop, 0, len(merchants))
    for _, merchant := range merchants {
        lists := npcdata.BuyListsOfNPC(merchant.TemplateID)
        if len(lists) == 0 {
            continue
        }
        shops = append(shops, gear.Shop{
            MerchantTemplateID: merchant.TemplateID,
            TaxRate:            dionTownTaxRate,
            Lists:              lists,
        })
    }

    return gear.Catalog{Shops: shops}
}

// shopCatalogForRegion returns the catalog of the town the loop farms
// near: elven by default, Dion when the active zone region is Dion.
func shopCatalogForRegion(region string) gear.Catalog {
    if region == regionDion {
        return dionShopCatalog
    }

    return townShopCatalog
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

// shoppingPlan plans the purchases against the current gear state:
// only the affordable part (the wanted tail is the widget save up
// view). The town trip calls it ONCE and freezes the result.
func (l *Loop) shoppingPlan() []gear.Purchase {
    return l.dropForeignMerchantPurchases(
        affordablePrefix(l.shoppingQueue()))
}

// dropForeignMerchantPurchases removes the plan lines whose merchant
// does not trade the item: a foreign line is a planner data bug, and
// dropping it loudly keeps the trip from burning its buy retries
// against the silent refusal. The sell first pieces of a dropped line
// stay unsold.
func (l *Loop) dropForeignMerchantPurchases(
    purchases []gear.Purchase,
) []gear.Purchase {
    kept := make([]gear.Purchase, 0, len(purchases))
    for _, purchase := range purchases {
        if merchantSellsItem(purchase.MerchantTemplateID, purchase.ItemID) {
            kept = append(kept, purchase)

            continue
        }
        merchant, known := merchantByTemplate(purchase.MerchantTemplateID)
        if known {
            l.logf("Hunt: shop: %s does not sell %s, dropping the "+
                "purchase", merchant.Name, npcdata.ItemName(purchase.ItemID))

            continue
        }
        l.logf("Hunt: shop: the merchant %d does not sell %s, "+
            "dropping the purchase", purchase.MerchantTemplateID,
            npcdata.ItemName(purchase.ItemID))
    }

    return kept
}

// shoppingQueue computes the fresh purchase queue against the current
// gear state: the affordable plan of the next trip plus the wanted
// tail (gear.PlanPurchaseQueue). The wallet leaves the spellbook
// reserve of the pending lessons unspent: the trip buys the books
// before the gear stops.
func (l *Loop) shoppingQueue() []gear.Purchase {
    if l.equip == nil {
        return nil
    }
    stats := l.tracker.InventoryStats()
    adena := int64(stats.Adena) - l.pendingBookBudget()
    if adena < 0 {
        adena = 0
    }
    queue := gear.PlanPurchaseQueue(
        l.equip.profile, l.equipment(),
        shopCatalogForRegion(l.zoneRegion), adena)
    // The keep one scroll line leads the queue: the escape spends the
    // scroll on every profitable trip and the safety stock must never
    // run dry.
    if soe, ok := l.soePurchaseLine(); ok {
        queue = append([]gear.Purchase{soe}, queue...)
    }

    return queue
}

// pendingBookBudget prices the spellbooks the learning queue demands
// and the inventory lacks, at the town tax price; zero without a
// waiting lesson.
func (l *Loop) pendingBookBudget() int64 {
    l.refreshLearnPlan()
    books := bookPurchases(l.walkLearnPrefix(false),
        l.tracker.InventoryHasItem)
    total := int64(0)
    for _, book := range books {
        total += book.Price
    }

    return total
}

// refreshShoppingCache recomputes the cached purchase queue and the
// built widget view once per shopping plan period: the cache drives
// both the trip trigger and the widget, and the view cache keeps the
// per tick publish allocation free (the planning wallet is cached
// with it).
func (l *Loop) refreshShoppingCache() {
    now := l.now()
    if !l.shoppingPlanAt.IsZero() &&
        now.Sub(l.shoppingPlanAt) < shoppingPlanPeriod {
        return
    }
    stats := l.tracker.InventoryStats()
    adena := int64(stats.Adena) - l.pendingBookBudget()
    if adena < 0 {
        adena = 0
    }
    l.shoppingPlanCache = l.shoppingQueue()
    l.shoppingPlanAt = now
    l.shoppingPlanAdena = adena
    l.shoppingViewCache = shoppingQueueView(
        l.shoppingPlanCache, l.shoppingPlanAdena)
    // The fresh plan re-feeds the weapon priority of the learning
    // queue: the planned next weapon purchase swaps the preferred
    // weapon family of the lesson order (see combat_skills.go).
    l.publishSkillWeaponPriority()
}

// shoppingWanted reports whether the plan justifies a town trip on
// its own: a cached plan with a total price above the trip minimum,
// summed without allocating a prefix slice. The keep one scroll line
// stays out of the total: a scroll-only plan never clears the
// minimum, so a fresh bot walks no errand just for the scroll.
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
        if purchase.ItemID == soeItemID {
            continue
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

// publishShoppingView refreshes the shopping widget every tick: the
// cached queue while the loop hunts, the remaining trip buys while a
// trip runs, and the cached queue again while a trip has no buys yet
// (the plan that triggered it is what the observer wants). Sessions
// without the shop strategy publish nothing.
func (l *Loop) publishShoppingView() {
    if !l.autonomous || !l.shoppingTripEnabled() {
        l.tracker.ClearShoppingPlan()

        return
    }
    if l.tripActive() {
        view := l.tripShoppingView()
        if len(view.Entries) > 0 {
            l.tracker.SetShoppingPlan(view)

            return
        }
        // No trip buys in flight yet (the walk to the shop, the sell
        // phase before the stop planning): the triggering plan stays.
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

// tripShoppingView builds the widget view of a running trip: the
// in-flight batch marked buying, then the pending purchases of the
// current and later stops.
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

// shoppingEntryView converts one planned purchase into the widget
// view, resolving the item stats through the generated npcdata
// dictionaries.
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
            purchase.MerchantTemplateID + npcdata.NPCDisplayOffset),
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

// replacementSellingActive reports whether the sell first step still
// runs: the auto equipment must not re-equip the pieces it just
// unequipped for their sale.
func (l *Loop) replacementSellingActive() bool {
    return len(l.replaceQueue) > 0 || len(l.replaceSelling) > 0
}

// stepReplacementSales runs the sell first step: every displaced
// piece is unequipped and sold before the buys run and the auto
// equipment wears the replacements. One step per tick; false while
// the step is busy.
func (l *Loop) stepReplacementSales(now time.Time) bool {
    if !l.replacePlanned {
        l.replacePlanned = true
        l.replaceQueue = l.replacementTargets()
        if len(l.replaceQueue) == 0 {
            return true
        }
        l.logf("Hunt: shop: %d equipped pieces feed the "+
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

// replaceUnequipsDone drives the unequip phase: every queued piece
// comes off, paced and retried twice before the buy runs without its
// credit. False while an unequip is in flight.
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

// replaceHeadSettled drops the queued head that needs no unequip
// anymore: offered by the junk flow, gone from the inventory or
// already unequipped.
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
// piece and reports whether the phase still waits on it; the request
// retries twice per piece.
func (l *Loop) replaceHeadOff(head int32, now time.Time) bool {
    if !l.replaceUnequipAt.IsZero() &&
        now.Sub(l.replaceUnequipAt) < equipActionPeriod {
        return false
    }
    if l.replaceTried >= 2 {
        l.logf("Hunt: shop: item %d does not come off, "+
            "buying without its credit", head)

        return true
    }
    l.replaceUnequipAt = now
    l.replaceTried++
    l.logf("Hunt: shop: unequipping the replaced item %d",
        head)
    if err := l.game.UseItem(head); err != nil {
        l.logf("Hunt: shop: unequip of %d failed: %v",
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
// flow has not offered go out as one batch through the transaction
// window. False while the offer is pending.
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
    if !l.transactionWindowFree(now) {
        return false
    }
    if err := l.game.SellItems(batch); err != nil {
        l.logf("Hunt: shop: replacement sell failed: %v", err)

        return false
    }
    for _, item := range batch {
        l.sold[item.ObjectID] = true
    }
    l.sellAt = now
    l.replaceSellSent = true
    l.logf("Hunt: shop: offered %d replaced pieces for sale",
        len(batch))

    return true
}

// replaceSalesSettled waits for the inventory update that confirms
// the replacement sales: the buys re-plan against the fresh adena
// only after it, bounded by replaceSellTimeout.
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

// resetReplacementSales drops the sell first step state, so an
// aborted trip leaves no half-sold queue behind.
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

// replacementTargets collects the equipped object ids the FROZEN
// TRIP PLAN displaces: the sell first step banks exactly the credit
// the buys were planned against.
func (l *Loop) replacementTargets() []int32 {
    purchases := l.tripPlan
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

// transactionWindowFree reports whether the shared buy and sell
// transaction window is free: every sender of the town trips paces
// through this one gate, so a sell fired behind a buy cannot burn
// the window.
func (l *Loop) transactionWindowFree(now time.Time) bool {
    last := l.buyAt
    if l.sellAt.After(last) {
        last = l.sellAt
    }

    return last.IsZero() || now.Sub(last) >= transactionPause
}

// planShoppingStops distributes the FROZEN TRIP PLAN into the buy
// stops: grouped by merchant, ordered by the walking distance from
// the character, merged into the current stop when already there.
func (l *Loop) planShoppingStops() {
    l.buysPlanned = true
    purchases := l.tripPlan
    if len(purchases) == 0 {
        l.logf("Hunt: shop: the trip plan carries no buys, " +
            "heading back")

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
            l.logf("Hunt: shop: no known merchant for template "+
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
    l.logf("Hunt: shop: planning to buy %d items from %d "+
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
            guide:    false,
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
    // The Dion merchants serve the band book stops (the spellbook
    // fallback of the lesson trips - the Sonia book list).
    for _, merchant := range dionMerchants {
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
        return l.merchantTemplates()
    }

    return []int32{
        l.tripStops[0].merchant.TemplateID + npcdata.NPCDisplayOffset,
    }
}

// tickStopShopping buys the purchases of the current stop: the
// merchant must be selected and in 3D range (the server checks
// both), one buylist per transaction request, one request per flood
// window, and every batch re-requested up to stopBuyRetries times
// before the stop gives it up. True when every purchase was
// requested AND confirmed.
//
//nolint:cyclop,funlen,gocognit // the stop shopping decision tree
func (l *Loop) tickStopShopping(now time.Time) bool {
    if len(l.tripStops) == 0 {
        return true
    }
    stop := l.tripStops[0]
    if len(stop.buys) == 0 && len(l.buyRequested) == 0 {
        return true
    }
    // The frozen plan's last responsible moment: an item the
    // inventory already carries must not be bought again (a loot drop
    // worn mid trip, a manual purchase).
    l.tripStops[0].buys = l.dropOwnedPurchases(l.tripStops[0].buys)
    stop = l.tripStops[0]
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
            l.logf("Hunt: shop: %d purchases confirmed",
                len(l.buyRequested))
            l.resetBuyRequest()
        case now.Sub(l.buyConfirmAt) < buyConfirmWait:
            return false
        default:
            l.buyRetries++
            if l.buyRetries > stopBuyRetries {
                l.logf("Hunt: shop: %d purchases never "+
                    "arrived after %d requests, skipping them",
                    len(l.buyRequested), l.buyRetries)
                l.resetBuyRequest()
            } else {
                l.logf("Hunt: shop: %d purchases did not "+
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
    // flood window is 1 s wide, the pause keeps the tick margin).
    if !l.transactionWindowFree(now) {
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
        l.logf("Hunt: shop: buy request failed: %v", err)

        return false
    }
    l.buyAt = now
    l.buyRequested = batch
    // The arrival baseline of the batch: the carried units of every
    // item id at the request moment (entries for the wearables, stacks
    // for the stackables). A retry keeps the original baseline.
    if l.buyBaseline == nil {
        l.buyBaseline = make(map[int32]int32, len(batch))
    }
    for _, purchase := range batch {
        if _, recorded := l.buyBaseline[purchase.ItemID]; !recorded {
            l.buyBaseline[purchase.ItemID] =
                l.carriedStackCount(purchase.ItemID)
        }
    }
    l.buyConfirmAt = now
    names := make([]string, 0, len(batch))
    cost := int64(0)
    for _, purchase := range batch {
        names = append(names, npcdata.ItemName(purchase.ItemID))
        cost += purchase.Price
    }
    if l.journal != nil {
        l.journal.Buy(l.tracker.ID(), strings.Join(names, ", "),
            len(batch), cost)
    }
    l.logf("Hunt: shop: buying %d items from %s (list %d): %s",
        len(batch), stop.merchant.Name, listID, strings.Join(names, ", "))

    return len(l.tripStops) > 0 && len(l.tripStops[0].buys) == 0 &&
        len(l.buyRequested) == 0
}

// isStackPurchase reports whether the order buys a stackable
// consumable: wearables carry one entry per equipped slot, the
// stackable delivery merges into the carried stack and only raises
// its count. Unknown types lean consumable.
func isStackPurchase(itemID int32) bool {
    switch npcdata.ItemType(itemID) {
    case itemTypeWeapon, itemTypeArmor, itemTypeShield:
        return false
    default:
        return true
    }
}

// carriedStackCount sums the tracked inventory count of one item id
// (the partial stacks the loot and the spends leave behind all merge
// into one count).
func (l *Loop) carriedStackCount(itemID int32) int32 {
    var count int32
    for _, item := range l.tracker.InventoryItems() {
        if item.ItemID == itemID {
            count += item.Count
        }
    }

    return count
}

// resetBuyRequest clears the in-flight buy batch with its arrival
// baseline (the confirmation, the retry budget skip and the trip
// boundaries all end the wait the same way).
func (l *Loop) resetBuyRequest() {
    l.buyRequested = nil
    l.buyBaseline = nil
    l.buyConfirmAt = time.Time{}
    l.buyRetries = 0
}

// dropOwnedPurchases filters the stop purchases whose item the
// inventory already carries beyond the plan: a pair family carries
// two copies, a single slot family blocks its second copy, and the
// stackable orders are count aware (they drop only when the carried
// count covers OwnedStack+Count).
func (l *Loop) dropOwnedPurchases(purchases []gear.Purchase) []gear.Purchase {
    items := l.tracker.InventoryItems()
    carried := make(map[int32]int, len(items))
    stackCount := make(map[int32]int32, len(items))
    for _, item := range items {
        carried[item.ItemID]++
        stackCount[item.ItemID] += item.Count
    }
    planned := make(map[int32]int, len(purchases))
    kept := purchases[:0]
    for _, purchase := range purchases {
        if isStackPurchase(purchase.ItemID) {
            if stackCount[purchase.ItemID] >=
                purchase.OwnedStack+purchase.Count {
                l.logger.Printf("Hunt: shop: the %s stack already "+
                    "covers the order, skipping the purchase",
                    npcdata.ItemName(purchase.ItemID))

                continue
            }
            kept = append(kept, purchase)

            continue
        }
        stats, hasStats := npcdata.ItemGearStats(purchase.ItemID)
        copies := 1
        if hasStats {
            copies = int(gear.FamilyCopiesOf(stats))
        }
        planned[purchase.ItemID]++
        if carried[purchase.ItemID]+planned[purchase.ItemID] > copies {
            l.logger.Printf("Hunt: shop: %s already in the inventory, "+
                "skipping the purchase", npcdata.ItemName(purchase.ItemID))

            continue
        }
        kept = append(kept, purchase)
    }

    return kept
}

// buysArrived reports whether every purchase of the batch shows up
// in the inventory: the wearable orders in ENTRY units (except the
// pair floors, which ride a carried id), the stackable orders in
// COUNT units, both against the baseline of the request. A lost
// baseline degrades to zero.
func (l *Loop) buysArrived(batch []gear.Purchase) bool {
    items := l.tracker.InventoryItems()
    carried := make(map[int32]int32, len(items))
    for _, item := range items {
        if isStackPurchase(item.ItemID) {
            carried[item.ItemID] += item.Count

            continue
        }
        carried[item.ItemID]++
    }
    for _, purchase := range batch {
        if carried[purchase.ItemID] <
            l.buyBaseline[purchase.ItemID]+purchase.Count {
            return false
        }
    }

    return true
}

// advanceTripStop finishes the current stop and walks to the next
// one (a buy stop or the return segment when none is left).
func (l *Loop) advanceTripStop() {
    if len(l.tripStops) > 0 {
        l.tripStops = l.tripStops[1:]
    }
    // A new stop starts with a fresh retry budget and no in-flight
    // batch (the previous stop only advances when its batch settled).
    l.resetBuyRequest()
    l.resetLearnState()
    l.resetGuideState()
    // A fresh stop starts with a fresh escape budget.
    // The finished stop talked to its npc: drop the selection it
    // left behind.
    l.clearServerSelection()
    if len(l.tripStops) == 0 {
        l.startReturnSegment()

        return
    }
    l.phase = phaseTownWalk
    l.merchantID = 0
    l.merchantPick = time.Time{}
    l.merchantDeckUntil = time.Time{}
    stop := l.tripStops[0]
    l.logf("Hunt: shop: walking to %s", stop.merchant.Name)
    var planFailed bool
    switch {
    case stop.teach:
        // The teacher stop walks right up to the class master: the
        // npc search radius plans the walk to the master's own point
        // (the walkable hall row beside it); a master cell the mesh
        // never connects stays reached through the partial answer.
        planFailed = !l.startWalkNpcSegment(
            townNpcPosition(stop.merchant))
    case stop.guide:
        // The guide stop walks the same npc segment as the teacher:
        // the plan ends on the closest walkable surface of the spawn.
        planFailed = !l.startWalkNpcSegment(
            townNpcPosition(stop.merchant))
    default:
        // The merchant stop walks the exact mesh search first (the
        // same authority the manual walk uses): the approach ring
        // would end the plan outside the shop, so the target is the
        // customer stand point of the stand table - face to face with
        // the merchant across the counter. The npc segment fallback
        // answers a plan that resolved onto the roof deck.
        planned := false
        if l.merchantWithinExactRange(stop.merchant) {
            var ringFallback bool
            planned, ringFallback = l.startWalkExactSegment(
                merchantStandPoint(stop.merchant))
            if !planned && ringFallback {
                planned = l.startWalkNpcSegment(
                    merchantStandPoint(stop.merchant))
            }
        }
        if !planned {
            planned = l.startWalkNpcSegment(
                merchantStandPoint(stop.merchant))
        }
        planFailed = !planned
    }
    if l.phase == phaseTownWalk && planFailed {
        l.abortTownTrip("no walkable path to the shop of " +
            stop.merchant.Name)
    }
}

// shoppingTripEnabled reports whether the shopping trigger may arm:
// the loop needs the gear profile and a known merchant catalog.
func (l *Loop) shoppingTripEnabled() bool {
    return l.equip != nil &&
        len(shopCatalogForRegion(l.zoneRegion).Shops) > 0
}

// emptyPurchase is the not-found sentinel of the weapon purchase
// probe (a plain zero var: nested empty literals trip exhaustruct).
var emptyPurchase gear.Purchase

// affordableWeaponPurchase returns the first affordable weapon
// purchase of the cached plan; the weapon outranks the rest of the
// plan (the sell and buy share one stop, the bare-handed character
// runs the errand alone).
func (l *Loop) affordableWeaponPurchase() (gear.Purchase, bool) {
    if !l.shoppingTripEnabled() {
        return emptyPurchase, false
    }
    l.refreshShoppingCache()
    for _, purchase := range l.shoppingPlanCache {
        if !purchase.Affordable {
            break
        }
        stats, ok := npcdata.ItemGearStats(purchase.ItemID)
        if !ok || gear.CategoryOf(stats) != gear.CategoryWeapon {
            continue
        }

        return purchase, true
    }

    return emptyPurchase, false
}

// weaponStopMerchant resolves the town merchant that sells the
// weapon purchase of the frozen trip plan (the plan the trip
// executes - the cached hunt queue already served its trigger).
func (l *Loop) weaponStopMerchant() (townNpc, bool) {
    for _, purchase := range l.tripPlan {
        stats, ok := npcdata.ItemGearStats(purchase.ItemID)
        if !ok || gear.CategoryOf(stats) != gear.CategoryWeapon {
            continue
        }

        return merchantByTemplate(purchase.MerchantTemplateID)
    }

    return zeroTownNpc, false
}

// weaponlessRunWanted reports whether the character fights bare-handed
// while its plan offers an affordable weapon: the target picks hold,
// the town trip runs the weapon errand without the teach stops and an
// aborted run retries on the short weapon cooldown.
func (l *Loop) weaponlessRunWanted() bool {
    if l.equip == nil {
        return false
    }
    if gear.HasWeapon(l.equip.profile, l.equipment()) {
        return false
    }
    _, ok := l.affordableWeaponPurchase()

    return ok
}

// snapshotTripGear records the paperdoll the trip starts with:
// gearDebtCheck compares the trip exits against it.
func (l *Loop) snapshotTripGear() {
    l.tripGearStart = l.tracker.PaperdollSlotObjectIDs()
    for index, objectID := range l.tripGearStart {
        if objectID == 0 {
            l.tripGearStartIDs[index] = 0

            continue
        }
        if item, ok := l.tracker.InventoryItemState(objectID); ok {
            l.tripGearStartIDs[index] = item.ItemID
        } else {
            l.tripGearStartIDs[index] = 0
        }
    }
}

// gearDebtCheck arms the gear debt at the trip exits: a slot that
// was occupied at the trip start, sits empty now and whose piece is
// gone from the inventory sold for a replacement that never landed.
// The debt runs the refill trip on the gear run cooldown; a piece
// still in the bag is no debt.
func (l *Loop) gearDebtCheck() {
    current := l.tracker.PaperdollSlotObjectIDs()
    items := l.tracker.InventoryItems()
    carried := make(map[int32]bool, len(items))
    for _, entry := range items {
        carried[entry.ObjectID] = true
    }
    for index, objectID := range l.tripGearStart {
        if objectID == 0 || current[index] != 0 || carried[objectID] {
            continue
        }
        if _, debt := l.gearDebt[index]; debt {
            continue
        }
        slot, ok := gear.SlotOfPaperdollIndex(index)
        if !ok {
            continue
        }
        if l.gearDebt == nil {
            l.gearDebt = make(map[int]int32)
        }
        l.gearDebt[index] = l.tripGearStartIDs[index]
        l.logf("Hunt: shop: the trip left the %s slot empty - the %s it "+
            "started with is gone (sold for a replacement that never "+
            "landed); the gear debt runs the refill trip on the gear "+
            "run cooldown", slot.String(), debtItemName(
            l.tripGearStartIDs[index]))
    }
    l.tripGearStart = [state.PaperdollSlots]int32{}
    l.tripGearStartIDs = [state.PaperdollSlots]int32{}
}

// gearDebtRunWanted reports whether an armed gear debt still waits
// for its refill: the debt only hurries the cooldown, the trip
// justification stays with the ordinary triggers.
func (l *Loop) gearDebtRunWanted() bool {
    if l.equip == nil {
        return false
    }
    if len(l.gearDebt) == 0 {
        return false
    }
    l.clearRefilledDebt()

    return len(l.gearDebt) > 0
}

// clearRefilledDebt drops the debt entries whose slot is dressed
// again and logs one line per recovered slot.
func (l *Loop) clearRefilledDebt() {
    current := l.tracker.PaperdollSlotObjectIDs()
    for index := range l.gearDebt {
        if current[index] == 0 {
            continue
        }
        delete(l.gearDebt, index)
        if slot, ok := gear.SlotOfPaperdollIndex(index); ok {
            l.logf("Hunt: shop: the %s slot is dressed again, the gear "+
                "debt clears", slot.String())
        }
    }
}

// debtItemName renders the lost piece of a gear debt entry for the
// logs: the generated dictionary name, or the item id fallback.
func debtItemName(itemID int32) string {
    name := npcdata.ItemName(itemID)
    if name == "" {
        name = "item #" + strconv.Itoa(int(itemID))
    }

    return name
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
    l.logf("Hunt: shop: %s, skipping %d purchases", reason,
        len(l.tripStops[0].buys))
    l.tripStops[0].buys = nil
}

// sellableStop reports whether the current stop sells the junk.
func (l *Loop) sellableStop() bool {
    return len(l.tripStops) > 0 && l.tripStops[0].sell
}
