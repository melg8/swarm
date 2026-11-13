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

// Shopping of the town trips: gear.PlanPurchases decides what to buy, the
// trip freezes that plan and executes it verbatim - the sell step sells
// exactly the pieces the plan counted on. One buylist per transaction
// request (docs/shopping_strategy.md).

// Timing and threshold constants of the shopping.
const (
    // transactionPause is the shared pacing window of every buy and sell
    // request: 10 game ticks (1 s) plus a 250 ms margin, an early request is
    // refused (the punishment limit is 0, docs/protocol_description.md).
    transactionPause = 1250 * time.Millisecond
    // buyPause paces the buys through the shared transaction window.
    buyPause = transactionPause
    // buyConfirmWait bounds the wait for the inventory update that confirms
    // a buy: a refused transaction answers silently.
    buyConfirmWait = 15 * time.Second
    // stopBuyRetries bounds the re-requests of a lost buy batch.
    stopBuyRetries = 3
    // replaceSellTimeout bounds the wait for the sold pieces to leave the
    // inventory: the trip proceeds without the credit after it.
    replaceSellTimeout = 60 * time.Second
    // shoppingPlanPeriod bounds the shopping trigger re-plans: the adena and
    // the inventory change with every loot.
    shoppingPlanPeriod = 5 * time.Second
    // shoppingTripMinValue is the total price that justifies a trip on its
    // own (below it the walk to town buys a handful of adena).
    shoppingTripMinValue = 100
    // weaponRunCooldown shortens the trip cooldown of the weapon runs: a
    // bare-handed character retries within a minute, not five.
    weaponRunCooldown = 45 * time.Second
    // townTaxRate is the buy tax of the elven village merchants (baseTax 15
    // percent, no castle owns their tax on a fresh server).
    townTaxRate = 0.15
    // dionTownTaxRate is the buy tax of the Town of Dion merchants
    // (MerchantPriceConfig.xml priceConfig id=8 baseTax=20).
    dionTownTaxRate = 0.20
)

// tripStop is one merchant visit of a town trip: the merchant, the
// purchases to buy there, the junk flag and the teacher or guide kind.
type tripStop struct {
    merchant townNpc
    buys     []gear.Purchase
    sell     bool
    teach    bool
    guide    bool
}

// townShopCatalog is the static gear catalog of the elven village shops.
var townShopCatalog = shopCatalog(townMerchants)

// dionShopCatalog is the static gear catalog of the Dion 20-25 band.
var dionShopCatalog = dionShopCatalogBuild(dionMerchants)

// dionShopCatalogBuild builds the Dion catalog at its 20 percent buy tax.
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

func shopCatalogForRegion(region string) gear.Catalog {
    if region == regionDion {
        return dionShopCatalog
    }

    return townShopCatalog
}

// shopCatalog builds the gear catalog of the town merchants from the
// generated buylists at the town tax rate.
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

// shoppingPlan plans the purchases against the current gear state: only the
// affordable part (the wanted tail is the widget save up view). The town
// trip calls it ONCE and freezes the result.
func (l *Loop) shoppingPlan() []gear.Purchase {
    return l.dropForeignMerchantPurchases(
        affordablePrefix(l.shoppingQueue()))
}

// dropForeignMerchantPurchases removes the plan lines whose merchant does
// not trade the item: the trip must not burn its buy retries on the silent
// refusal. A dropped line's sell-first pieces stay unsold.
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

// shoppingQueue computes the fresh purchase queue (gear.PlanPurchaseQueue).
// The wallet leaves the spellbook reserve of the pending lessons unspent:
// the trip buys the books first.
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
    // The keep-one scroll line leads the queue: the escape spends it on every
    // profitable trip and the safety stock must never run dry.
    if soe, ok := l.soePurchaseLine(); ok {
        queue = append([]gear.Purchase{soe}, queue...)
    }

    return queue
}

// pendingBookBudget prices the missing spellbooks of the learning queue at
// the town tax price; zero without a waiting lesson.
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

// refreshShoppingCache recomputes the cached queue and its widget view once
// per plan period: the cache drives the trip trigger and the widget, and the
// view keeps the publish allocation free.
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
    // The fresh plan re-feeds the weapon priority of the learning queue: the
    // planned weapon purchase swaps the preferred family (combat_skills.go).
    l.publishSkillWeaponPriority()
}

// shoppingWanted reports whether the plan justifies a town trip on its own.
// The keep-one scroll line stays out of the total, so a fresh bot walks no
// errand just for the scroll.
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

// affordablePrefix filters the affordable buys: the trip executes only them,
// the wanted tail is the widget save up view.
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

// publishShoppingView refreshes the shopping widget every tick: the cached
// queue while the loop hunts, the remaining trip buys while one runs.
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
        // No trip buys in flight yet: the triggering plan stays.
    }
    l.refreshShoppingCache()
    l.tracker.SetShoppingPlan(l.shoppingViewCache)
}

// shoppingQueueView builds the widget view of a purchase queue: the entries
// in the walked order with the planning adena and the affordable total.
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

// tripShoppingView builds the widget view of a running trip: the in-flight
// batch marked buying, then the pending purchases of the stops.
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

// shoppingEntryView converts one purchase into the widget view, resolving
// the item stats through the generated npcdata dictionaries.
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

// replacementSellingActive reports whether the sell first step still runs:
// the auto equipment must not re-equip the pieces it just unequipped.
func (l *Loop) replacementSellingActive() bool {
    return len(l.replaceQueue) > 0 || len(l.replaceSelling) > 0
}

// stepReplacementSales runs the sell-first step: every displaced piece is
// unequipped and sold before the buys run. False while the step is busy.
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

// replaceUnequipsDone drives the unequip phase: every queued piece comes
// off, paced and retried twice before the buy runs without its credit.
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

// replaceHeadSettled drops the queued head that needs no unequip anymore:
// offered by the junk flow, gone from the inventory or already unequipped.
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

// replaceHeadOff sends the paced unequip request for the queued piece and
// reports whether the phase still waits on it; it retries twice per piece.
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

func (l *Loop) dropReplacementHead() {
    l.replaceQueue = l.replaceQueue[1:]
    l.replaceTried = 0
}

// replaceOfferDone drives the sale phase: the handed pieces the junk flow
// has not offered go out as one batch through the transaction window.
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
        // The junk flow already offered every piece: the settle phase waits
        // out their removals.
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

// replaceSalesSettled waits for the inventory update that confirms the
// sales: the buys re-plan against the fresh adena only after it.
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

// resetReplacementSales drops the sell first step state, so an aborted trip
// leaves no half-sold queue behind.
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

// replacementTargets collects the equipped object ids the FROZEN TRIP PLAN
// displaces: the sell first step banks exactly the planned credit.
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

// transactionWindowFree reports whether the shared transaction window is
// free: every sender of the town trips paces through this gate.
func (l *Loop) transactionWindowFree(now time.Time) bool {
    last := l.buyAt
    if l.sellAt.After(last) {
        last = l.sellAt
    }

    return last.IsZero() || now.Sub(last) >= transactionPause
}

// planShoppingStops distributes the FROZEN TRIP PLAN into the buy stops:
// grouped by merchant, ordered by the walking distance from the character,
// merged into the current stop when already there.
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
    // The group of the merchant the character stands at buys right here.
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

func merchantByTemplate(templateID int32) (townNpc, bool) {
    for _, merchant := range townMerchants {
        if merchant.TemplateID == templateID {
            return merchant, true
        }
    }
    // The Dion merchants serve the band book stops (the spellbook fallback
    // of the lesson trips).
    for _, merchant := range dionMerchants {
        if merchant.TemplateID == templateID {
            return merchant, true
        }
    }

    return zeroTownNpc, false
}

// stopMerchantTemplates lists the packet template ids of the current stop
// merchant (offset by the display id base).
func (l *Loop) stopMerchantTemplates() []int32 {
    if len(l.tripStops) == 0 {
        return l.merchantTemplates()
    }

    return []int32{
        l.tripStops[0].merchant.TemplateID + npcdata.NPCDisplayOffset,
    }
}

// tickStopShopping buys the purchases of the current stop: the merchant must
// be selected and in 3D range (the server checks both), one buylist per
// transaction window, and every batch re-requested up to stopBuyRetries
// times. True when every purchase was requested and confirmed.
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
    // The last responsible moment of the frozen plan: an item the inventory
    // already carries must not be bought again (a loot drop, a manual buy).
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
    // The buy pacing shares the transaction window with the sells: the first
    // buy of a stop waits out the last sell batch.
    if !l.transactionWindowFree(now) {
        return false
    }
    // One buylist per request: the purchases of the first list id go out
    // together, an in-flight retry re-sends its own batch.
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
    // The arrival baseline: the carried units of every item id at the
    // request. A retry keeps the original baseline.
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

// isStackPurchase reports whether the order buys a stackable consumable:
// wearables carry one entry per equipped slot, a stackable delivery merges
// into the carried stack and only raises its count.
func isStackPurchase(itemID int32) bool {
    switch npcdata.ItemType(itemID) {
    case itemTypeWeapon, itemTypeArmor, itemTypeShield:
        return false
    default:
        return true
    }
}

// carriedStackCount sums the tracked inventory count of one item id (the
// partial stacks the loot and the spends leave merge into one count).
func (l *Loop) carriedStackCount(itemID int32) int32 {
    var count int32
    for _, item := range l.tracker.InventoryItems() {
        if item.ItemID == itemID {
            count += item.Count
        }
    }

    return count
}

// resetBuyRequest clears the in-flight buy batch with its baseline: the
// confirmation, the retry skip and the trip boundaries end the wait alike.
func (l *Loop) resetBuyRequest() {
    l.buyRequested = nil
    l.buyBaseline = nil
    l.buyConfirmAt = time.Time{}
    l.buyRetries = 0
}

// dropOwnedPurchases filters the purchases the inventory already covers: a
// pair family carries two copies, a single slot family blocks its second,
// and a stackable order drops when the count covers OwnedStack+Count.
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

// buysArrived reports whether every purchase of the batch shows up in the
// inventory: the wearables in entry units (the pair floors ride a carried
// id), the stackables in count units, both against the request baseline.
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

// advanceTripStop finishes the current stop and walks to the next one (a
// buy stop or the return segment when none is left).
func (l *Loop) advanceTripStop() {
    if len(l.tripStops) > 0 {
        l.tripStops = l.tripStops[1:]
    }
    // A new stop starts with a fresh retry budget and no in-flight batch.
    l.resetBuyRequest()
    l.resetLearnState()
    l.resetGuideState()
    // The finished stop talked to its npc: drop the selection it left.
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
        // The teacher stop walks right up to the class master: a master cell
        // the mesh never connects stays reached through the partial answer.
        planFailed = !l.startWalkNpcSegment(
            townNpcPosition(stop.merchant))
    case stop.guide:
        // The guide stop walks the same npc segment as the teacher.
        planFailed = !l.startWalkNpcSegment(
            townNpcPosition(stop.merchant))
    default:
        // The merchant stop walks the exact mesh search first: the approach
        // ring would end outside the shop, so the target is the customer
        // stand point, face to face across the counter. The npc segment
        // fallback answers a plan on the roof deck.
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

// shoppingTripEnabled reports whether the shopping trigger may arm: the
// loop needs the gear profile and a known merchant catalog.
func (l *Loop) shoppingTripEnabled() bool {
    return l.equip != nil &&
        len(shopCatalogForRegion(l.zoneRegion).Shops) > 0
}

// emptyPurchase is the not-found sentinel of the weapon purchase probe.
var emptyPurchase gear.Purchase

// affordableWeaponPurchase returns the first affordable weapon purchase of
// the cached plan: the weapon outranks the rest and a bare-handed character
// runs the errand alone.
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

// weaponStopMerchant resolves the merchant that sells the weapon purchase of
// the frozen trip plan (the cached hunt queue served its trigger).
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

// weaponlessRunWanted reports whether the character fights bare-handed while
// its plan offers an affordable weapon: the trip runs the weapon errand
// without the teach stops, and an aborted run retries on the short cooldown.
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

// gearDebtCheck arms the gear debt at the trip exits: a slot occupied at the
// trip start, empty now and whose piece left the inventory was sold for a
// replacement that never landed. The debt runs the refill trip on the gear
// run cooldown; a piece still in the bag is no debt.
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

// gearDebtRunWanted reports whether an armed gear debt still waits for its
// refill: the debt only hurries the cooldown.
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

// debtItemName renders the lost piece of a gear debt entry for the logs.
func debtItemName(itemID int32) string {
    name := npcdata.ItemName(itemID)
    if name == "" {
        name = "item #" + strconv.Itoa(int(itemID))
    }

    return name
}

// stopBuysPending reports whether the current stop still wants buys: the
// unrequested purchases or an in-flight batch awaiting confirmation.
func (l *Loop) stopBuysPending() bool {
    return len(l.tripStops) > 0 &&
        (len(l.tripStops[0].buys) > 0 || len(l.buyRequested) > 0)
}

// resetStopBuys drops the pending buys of the current stop: the server
// refuses buys without the selected merchant target.
func (l *Loop) resetStopBuys(reason string) {
    if len(l.tripStops) == 0 {
        return
    }
    l.logf("Hunt: shop: %s, skipping %d purchases", reason,
        len(l.tripStops[0].buys))
    l.tripStops[0].buys = nil
}

func (l *Loop) sellableStop() bool {
    return len(l.tripStops) > 0 && l.tripStops[0].sell
}
