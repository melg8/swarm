// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

// Shop is one merchant of the shopping strategy: the packet template
// id (display id) of the NpcInfo packets, its buylists and the buy
// tax rate of its town (the Mobius buy price is the reference price
// times (1 + baseTax + castleTax); the elven village merchants sell
// at 15 percent over the reference price while no castle owns their
// tax).
type Shop struct {
    // MerchantTemplateID is the packet template id of the merchant.
    MerchantTemplateID int32
    // TaxRate is the buy tax markup of the merchant town (0.15 for
    // the elven village).
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
    // ListID is the buylist the item is bought from (one buy request
    // owns one list).
    ListID int32
    // MerchantTemplateID is the merchant the list belongs to.
    MerchantTemplateID int32
    // Count is the stack count of the order (1 for gear).
    Count int32
    // OwnedStack is the stack count the planner sized the order
    // against (the stackable orders): the executor staleness rule
    // drops the order only when the live count already covers the
    // plan target OwnedStack+Count (a budget capped partial restock
    // stays a real order - the carried stack is below the target, not
    // above it).
    OwnedStack int32
    // Price is the buy price of the order with the shop tax.
    Price int64
    // Reason is the human readable log line.
    Reason string
    // SellFirst lists the equipped object ids the purchase displaces
    // (the real paperdoll occupants of the slots it writes to): the
    // trip unequips and sells them before buying, so their proceeds
    // fund the replacement and the adena on hand only needs to cover
    // the difference.
    SellFirst []int32
    // SellCredit is the summed sell value of the SellFirst pieces
    // (the Mobius sell pays referencePrice/2). The planner credits it
    // to the budget of the trip that sells them.
    SellCredit int64
    // Gain is the score gain the purchase brings to the virtual
    // paperdoll (the profile scoring: weapon pAtk x attack speed,
    // armor pDef, jewel mDef, shield expected block value). The shop
    // widget shows it next to the price so a suspicious value per
    // adena pick is visible at a glance.
    Gain float64
    // Affordable reports whether the planning adena plus the sell
    // credits cover the price. The trip buys only the affordable
    // purchases; a purchase queue appends the unaffordable wanted
    // tail after them for the widget view.
    Affordable bool
    // Missing is the adena the bot still lacks before it can pay for
    // everything through this entry of a purchase queue (0 while the
    // wallet covers it): the wanted tail entries carry the growing
    // shortfall, the affordable plan entries stay zero.
    Missing int64
}

// shopTaxLimit bounds the tax sanity: a shop with a tax rate above
// this margin is rejected as broken data.
const shopTaxLimit = 2.0

// shoppingQueueTail bounds the wanted tail of a purchase queue: the
// save up entries the shop widget shows beyond the affordable plan of
// the next trip.
const shoppingQueueTail = 8

// PlanPurchases plans the purchases of the catalog for the equipment
// within the adena budget, ranked by the strategy phases: the weapon
// milestone first (the top affordable strict upgrade - the best tier
// the wallet plus the sale credits reach; the top-tier guard drops
// every cheaper rung of the same slots whatever their value per
// adena), the basic jewel floor second (the cheapest offer of every
// empty jewel slot, both halves of every pair - the 261 adena outfit
// covers every slot ahead of the armor maximization: the starting
// locations barely attack with magic, the mDef upgrades never pay
// and the slots must not stay empty because the armor set ate the
// wallet), the pdef maximizing armor set third (one piece per armor
// family inside the remaining budget: a rich wallet buys the
// advanced pieces directly, a poor one fills many slots with the
// cheap offers - the set maximizes the summed pdef either way, the
// user rule of the armor spending) and the shield with the leftover.
// Nothing gets bought that the inventory already carries (the free
// upgrades are simulated first). Simulated equips keep the plan
// consistent: after a planned purchase the virtual paperdoll carries
// the bought item and the next pick compares against it. Every slot
// gets at most ONE purchase per trip: the weapon phase buys one
// milestone, the jewel floor fills each empty slot once, the set
// planner picks one piece per armor family - the next trip re-plans
// against the paperdoll the previous purchases reached.
func PlanPurchases(
    profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase {
    return planPurchases(profile, equipment, catalog, adena, 0)
}

// PlanPurchaseQueue plans the full purchase queue of the shop widget:
// the affordable plan of the next trip first (exactly the
// PlanPurchases picks), then the wanted tail - the next steps the
// wallet cannot pay for yet (the next weapon rung, the next armor
// tier of every family, the remaining jewel slots, the next shield),
// in the order the strategy wants them. Every tail entry carries the
// adena still missing before everything through it becomes
// affordable, so the widget shows what the bot saves up for and how
// far away it is.
func PlanPurchaseQueue(
    profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase {
    return planPurchases(
        profile, equipment, catalog, adena, shoppingQueueTail)
}

// planPurchases walks the phased planner: the affordable phases first
// (the plain plan), the wanted tail behind them when the queue mode
// asks for it (the same phases past the wallet, every entry marked
// unaffordable, bounded by the tail budget).
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
