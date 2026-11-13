// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

// Shop is one merchant of the shopping strategy: the packet template id, its
// buylists and the buy tax rate of its town. The Mobius buy price is the
// reference price times (1 + baseTax + castleTax); the elven village sells at
// 15 percent over reference while no castle owns the tax.
type Shop struct {
    // MerchantTemplateID is the packet template id of the merchant.
    MerchantTemplateID int32
    // TaxRate is the buy tax markup of the merchant town (0.15 elven).
    TaxRate float64
    // Lists are the buylist ids the merchant sells.
    Lists []int32
}

// Catalog is the set of shops one shopping trip can visit.
type Catalog struct {
    Shops []Shop
}

// Purchase is one planned order of the shop strategy.
type Purchase struct {
    // ItemID is the display id of the item to buy.
    ItemID int32
    // ListID is the buylist the item is bought from (one per request).
    ListID int32
    // MerchantTemplateID is the merchant the list belongs to.
    MerchantTemplateID int32
    // Count is the stack count of the order (1 for gear).
    Count int32
    // OwnedStack is the stack count the planner sized the order against: the
    // executor drops the order only when the live count already covers
    // OwnedStack+Count, so a budget capped partial restock stays real.
    OwnedStack int32
    // Price is the buy price of the order with the shop tax.
    Price int64
    // Reason is the human readable log line.
    Reason string
    // SellFirst lists the equipped object ids the purchase displaces (the real
    // paperdoll occupants of the slots it writes to): the trip unequips and
    // sells them first, so their proceeds fund the replacement.
    SellFirst []int32
    // SellCredit sums the sell value of SellFirst (the Mobius sell pays
    // referencePrice/2); the planner credits it to the buying trip.
    SellCredit int64
    // Gain is the score gain to the virtual paperdoll (weapon pAtk x attack
    // speed, armor pDef, jewel mDef, shield block); the shop widget shows it
    // next to the price.
    Gain float64
    // Affordable reports whether the planning adena plus the sell credits
    // cover the price; the trip buys only these.
    Affordable bool
    // Missing is the adena still lacking before everything through this entry
    // of a purchase queue is affordable (zero for the affordable plan).
    Missing int64
}

// shopTaxLimit rejects a shop tax rate above this margin as broken data.
const shopTaxLimit = 2.0

// shoppingQueueTail bounds the wanted tail of a purchase queue: the save up
// entries the shop widget shows beyond the affordable plan.
const shoppingQueueTail = 8

// PlanPurchases plans the next trip's purchases within the adena budget,
// ranked by the strategy phases: the weapon milestone, the pdef maximizing
// armor set (one piece per armor family), the basic jewel floor of the empty
// slots and the leftover shield. Nothing the inventory already carries gets
// bought (the free upgrades are simulated first) and every slot takes at most
// one purchase: after a planned purchase the virtual paperdoll carries the
// item and the next trip re-plans against it (docs/shopping_strategy.md).
func PlanPurchases(
    profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase {
    return planPurchases(profile, equipment, catalog, adena, 0)
}

// PlanPurchaseQueue plans the full queue of the shop widget: the affordable
// plan of the next trip first, then the wanted tail the wallet cannot pay for
// yet (at most shoppingQueueTail entries), in strategy order. Every tail entry
// carries the adena still missing before everything through it is affordable.
func PlanPurchaseQueue(
    profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase {
    return planPurchases(
        profile, equipment, catalog, adena, shoppingQueueTail)
}

// planPurchases walks the phased planner: the affordable phases first, then
// the wanted tail past the wallet when the queue mode asks for it.
func planPurchases(
    profile Profile, equipment Equipment, catalog Catalog, adena int64,
    tail int,
) []Purchase {
    walk := newPlanWalk(profile, equipment, catalog, adena)
    walk.runPhases()
    if tail > 0 {
        walk.runTail(tail)
    }

    return walk.purchases
}
