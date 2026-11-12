// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "math"
    "sort"
    "sync"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// purchaseCandidate is one shop offer joined with the item stats.
type purchaseCandidate struct {
    itemID   int32
    listID   int32
    merchant int32
    price    int64
    stats    npcdata.GearStats
    score    float64
}

// walkView is the per-iteration snapshot of the walk the candidate
// classification reads: the walked virtual paperdoll, the best value
// of the surviving weapon upgrades (the milestone the top-tier guard
// leaves viable) and the weapon anchor of the jewel floor gate.
type walkView struct {
    virtual [slotCount]ScoredItem
    target  float64
    anchor  int64
}

// shopStrategy drives the candidate classification of the purchase
// walk: the jewel floor behind a real weapon (the cheapest set fills
// every empty slot, the pair halves included, only once the worn
// weapon is not the starter kit - the jewels never run ahead of the
// weapon) and the basic-only jewel rule (the starting locations
// barely attack with magic, the mDef upgrades never pay - the floor
// set covers the needs).
type shopStrategy struct {
    floorIDs map[int32]bool
}

// classify resolves the phase and the rank of one candidate against
// the walked paperdoll; ok is false when the strategy skips the
// candidate. The rank orders the picks inside the phase (higher
// wins): the weapons by the value per adena (only the best value
// strict upgrade is eligible - the wallet saves for the milestone,
// never a worse value weapon just because it is cheaper), the jewel
// floor by the price (the cheapest offers first), the shield by the
// raw gain. The armor never walks the pick: the set planner owns the
// armor families (see planArmorSet).
func (s *shopStrategy) classify(
    view walkView, candidate *purchaseCandidate, gain float64,
) (int, float64, bool) {
    switch CategoryOf(candidate.stats) {
    case CategoryJewel:
        // The jewel floor opens only behind a real weapon: the anchor
        // is the reference price of the worn weapon and stays zero
        // for the starter kit (or an empty hand), so no jewel runs
        // ahead of the weapon milestone. The floor fills every empty
        // slot with the cheapest offer of the family and never
        // replaces a worn jewel: the basic set covers the mDef needs.
        if s.floorIDs[candidate.itemID] && view.anchor > 0 &&
            floorSlotEmpty(view.virtual, candidate.stats.BodyPart) {
            return phaseFloor, -float64(candidate.price), true
        }

        return phaseFloor, 0, false
    case CategoryWeapon:
        if candidate.price <= 0 {
            return phaseWeapon, 0, false
        }
        value := gain / float64(candidate.price)
        if value < view.target {
            return phaseWeapon, 0, false
        }

        return phaseWeapon, value, true
    case CategoryShield:
        return phaseShield, gain, true
    default:
        // The armor and everything else stay with the set planner.
        return phaseArmorSet, 0, false
    }
}

// boughtEntry builds the virtual paperdoll entry of a planned
// purchase: the item id drives the anchor and defense pricing of the
// later picks, the object id stays zero (nothing equips it yet).
func boughtEntry(best *purchaseCandidate) ScoredItem {
    //nolint:exhaustruct_v5 // a planned buy has no inventory object yet
    return ScoredItem{
        Item:  state.InventoryItem{ItemID: best.itemID},
        Stats: best.stats,
        Score: best.score,
    }
}

// cheapestJewelIDs resolves the cheapest jewel offer per family (the
// ring, earring and necklace bodyparts): the jewel floor buys these
// only, so the empty slots fill with the cheapest pieces the shops
// sell.
func cheapestJewelIDs(candidates []purchaseCandidate) map[int32]bool {
    type cheapest struct {
        itemID int32
        price  int64
    }
    best := make(map[string]*cheapest, 3)
    for index := range candidates {
        candidate := &candidates[index]
        family := candidate.stats.BodyPart
        if !jewelBodyPart(family) {
            continue
        }
        current, seen := best[family]
        if !seen || candidate.price < current.price ||
            (candidate.price == current.price &&
                candidate.itemID < current.itemID) {
            best[family] = &cheapest{
                itemID: candidate.itemID,
                price:  candidate.price,
            }
        }
    }
    ids := make(map[int32]bool, len(best))
    for _, entry := range best {
        ids[entry.itemID] = true
    }

    return ids
}

// jewelIDCache holds the precomputed cheapest jewel IDs per (catalog
// content, profile) pair, paralleling candidateCache.
var jewelIDCache sync.Map

// cachedCheapestJewelIDs returns the cheapest jewel IDs for the
// catalog and profile, cached per (catalog content, profile) pair.
// The jewel IDs are derived from the (cached) candidates, so they are
// static for a given catalog and profile - the 100 bot fleet was
// rebuilding the same map 100 times every 5 seconds.
func cachedCheapestJewelIDs(
    profile Profile, catalog Catalog, candidates []purchaseCandidate,
) map[int32]bool {
    key := cacheKeyOf(profile, catalog)
    if cached, ok := jewelIDCache.Load(key); ok {
        return cached.(map[int32]bool)
    }
    ids := cheapestJewelIDs(candidates)
    jewelIDCache.Store(key, ids)

    return ids
}

// bestWeaponValue resolves the value per adena of the best weapon
// candidate among the walk survivors: the weapon phase buys only this
// one (the value gate of classify). The survivors already carry the
// top-tier guard - the top affordable tier of the hand slots - so the
// milestone the ranking aims at is the best tier the wallet reaches,
// never a cheaper rung below it.
func bestWeaponValue(survivors []walkCandidate) float64 {
    best := float64(0)
    for index := range survivors {
        item := &survivors[index]
        if CategoryOf(item.candidate.stats) != CategoryWeapon ||
            item.candidate.price <= 0 {
            continue
        }
        if value := item.gain / float64(item.candidate.price); value > best {
            best = value
        }
    }

    return best
}

// weaponAnchor prices the defense ceiling of the current stage: the
// reference price of the worn weapon. A starter weapon (or none)
// anchors zero - the first real weapon comes before any jewel buy.
func weaponAnchor(virtual [slotCount]ScoredItem) int64 {
    weapon := virtual[SlotRHand]
    if paperdollEmpty(weapon) || starterSet[weapon.Item.ItemID] {
        return 0
    }

    return npcdata.ItemPrice(weapon.Item.ItemID)
}

// floorSlotEmpty reports whether the jewel bodypart still has an
// empty slot to fill: the floor only fills, it never replaces a worn
// jewel.
func floorSlotEmpty(virtual [slotCount]ScoredItem, bodyPart string) bool {
    slots := SlotsForBodyPart(bodyPart)
    if len(slots) == 0 {
        return false
    }
    if len(slots) == 1 {
        return paperdollEmpty(virtual[slots[0]])
    }
    slot := pairSlot(virtual, slots)

    return slot != slotInvalid && paperdollEmpty(virtual[slot])
}

// walkedPurchase builds one entry of the queue walk: the cumulative
// adena accounting decides the missing amount (the adena still
// lacking before everything through this entry is affordable, 0
// while the wallet covers it) and the tail mode marks the entry
// unaffordable.
func walkedPurchase(
    best *purchaseCandidate, gain float64, credit int64, sellFirst []int32,
    adena int64, spent int64, credited int64, affordable bool,
) Purchase {
    missing := spent - adena - credited
    if missing < 0 {
        missing = 0
    }

    return Purchase{
        ItemID:             best.itemID,
        ListID:             best.listID,
        MerchantTemplateID: best.merchant,
        Count:              1,
        OwnedStack:         0,
        Price:              best.price,
        Reason:             "buying " + best.describe(gain),
        SellFirst:          sellFirst,
        SellCredit:         credit,
        Gain:               gain,
        Affordable:         affordable,
        Missing:            missing,
    }
}

// catalogCandidates joins the shop offers with the item gear stats,
// keeping the cheapest offer per item id (the same item can appear in
// several buylists) and dropping everything the profile cannot use.
// The result is cached per (catalog content, profile name) pair: the
// catalog and the profile are both static for a given bot class and
// region (the elven village merchants never change their stock, the
// melee fighter profile scores items the same way every call), so the
// 100 bot fleet was rebuilding the same offers map 100 times every 5
// seconds - 17 MB of allocations over a 3 minute fleet run. The cache
// collapses that to one build per (catalog, profile) pair for the
// lifetime of the process.
//
// The key hashes the catalog content (the merchants, the tax rates
// and the buylist ids), never a pointer: the catalog travels by
// value, so a pointer key would address the per call copy - every
// call missed the cache and stored a fresh entry (a full candidate
// slice) that nothing ever freed. That was the slow continuous heap
// growth of the long runs (about a megabyte per bot per minute),
// fixed by the content hash. The hashing is order sensitive: fine,
// the catalogs are process lifetime singletons built once (see the
// townShopCatalog and dionShopCatalog vars of the hunt package).
func catalogCandidates(
    profile Profile, catalog Catalog,
) []purchaseCandidate {
    key := cacheKeyOf(profile, catalog)
    if cached, ok := candidateCache.Load(key); ok {
        return cached.([]purchaseCandidate)
    }
    candidates := buildCatalogCandidates(profile, catalog)
    candidateCache.Store(key, candidates)

    return candidates
}

// candidateCacheKey is the composite key of the gear caches: the
// profile name (the melee fighter profile is a singleton) and the
// content hash of the catalog (see catalogHash). No pointers: the
// catalog parameter is a by value copy, and a pointer key addressed
// that copy - the maps grew by an entry per call forever (the
// continuous memory leak of the long runs).
type candidateCacheKey struct {
    profile     string
    catalogHash uint64
}

// cacheKeyOf builds the cache key of a (profile, catalog) pair.
func cacheKeyOf(profile Profile, catalog Catalog) candidateCacheKey {
    return candidateCacheKey{
        profile:     profile.Name(),
        catalogHash: catalogHash(catalog),
    }
}

// candidateCache holds the precomputed candidates per (catalog
// content, profile) pair. The cache is never cleared: the catalogs
// and profiles are process lifetime singletons, and the number of
// distinct pairs is bounded by the number of bot classes times the
// number of regions (one for the elven deployment today).
var candidateCache sync.Map

// catalogHash computes a stable hash of everything the candidate
// build reads from the catalog: the merchant ids, the tax rates and
// the buylist ids of every shop (FNV-1a over the byte pattern). Two
// catalogs of equal content hash equal, so a by value catalog copy
// hits the cache its source built - the cache stays bounded by the
// distinct catalog contents, not by the call count.
func catalogHash(catalog Catalog) uint64 {
    var h uint64 = 14695981039346656037
    hashByte := func(b byte) {
        h ^= uint64(b)
        h *= 1099511628211
    }
    for _, shop := range catalog.Shops {
        for shift := range 4 {
            hashByte(byte(shop.MerchantTemplateID >> (8 * shift)))
        }
        bits := math.Float64bits(shop.TaxRate)
        for range 8 {
            hashByte(byte(bits))
            bits >>= 8
        }
        for _, listID := range shop.Lists {
            for shift := range 4 {
                hashByte(byte(listID >> (8 * shift)))
            }
        }
    }

    return h
}

// buildCatalogCandidates is the uncached implementation of
// catalogCandidates: joins the shop offers with the item gear stats,
// keeping the cheapest offer per item id and dropping everything the
// profile cannot use.
func buildCatalogCandidates(
    profile Profile, catalog Catalog,
) []purchaseCandidate {
    type offer struct {
        listID   int32
        merchant int32
        price    int64
    }
    offers := make(map[int32]offer)
    for _, shop := range catalog.Shops {
        if shop.TaxRate < 0 || shop.TaxRate > shopTaxLimit {
            continue
        }
        for _, listID := range shop.Lists {
            for _, itemID := range npcdata.ItemsOfBuyList(listID) {
                stats, ok := npcdata.ItemGearStats(itemID)
                if !ok || scoreStats(profile, stats) <= 0 {
                    continue
                }
                price := int64(float64(npcdata.ItemPrice(itemID)) *
                    (1 + shop.TaxRate))
                current, seen := offers[itemID]
                if !seen || price < current.price ||
                    (price == current.price && listID < current.listID) {
                    offers[itemID] = offer{
                        listID:   listID,
                        merchant: shop.MerchantTemplateID,
                        price:    price,
                    }
                }
            }
        }
    }
    candidates := make([]purchaseCandidate, 0, len(offers))
    for itemID, offer := range offers {
        stats, _ := npcdata.ItemGearStats(itemID)
        candidates = append(candidates, purchaseCandidate{
            itemID:   itemID,
            listID:   offer.listID,
            merchant: offer.merchant,
            price:    offer.price,
            stats:    stats,
            score:    scoreStats(profile, stats),
        })
    }
    sort.Slice(candidates, func(i int, j int) bool {
        if candidates[i].score != candidates[j].score {
            return candidates[i].score > candidates[j].score
        }

        return candidates[i].itemID < candidates[j].itemID
    })

    return candidates
}

// walkCandidate is one candidate that passed the viability gates of
// a walk round, with the precomputed data the classification and the
// phase ranking need.
type walkCandidate struct {
    candidate *purchaseCandidate
    gain      float64
    credit    int64
    sellFirst []int32
}

// bestPick resolves the best candidate of one phase under the current
// walk state: the viability gates (not planned beyond the family copy
// count, a strict paperdoll improvement, affordable with the
// displaced credits), the top-tier slot guard (a nil ladderTop
// switches it off - the tail walk shows the ladder) and the phase
// classification with its ranking. The candidates of the other
// phases stay out of the ranking. The winner returns with its credit
// and the SellFirst object ids.
func (w *planWalk) bestPick(wantPhase int) (
    *purchaseCandidate, float64, int64, []int32,
) {
    survivors := w.viableCandidates()
    view := walkView{
        virtual: w.virtual,
        target:  bestWeaponValue(survivors),
        anchor:  weaponAnchor(w.virtual),
    }
    var best *walkCandidate
    bestRank := float64(0)
    for index := range survivors {
        item := &survivors[index]
        phase, rank, ok := w.strategy.classify(
            view, item.candidate, item.gain)
        if !ok || phase != wantPhase {
            continue
        }
        if pickBeats(rank, item, best, bestRank) {
            best = item
            bestRank = rank
        }
    }
    if best == nil {
        return nil, 0, 0, nil
    }

    return best.candidate, best.gain, best.credit, best.sellFirst
}

// pickBeats reports whether the classified candidate outranks the
// current best pick of the phase: the rank first (higher wins), then
// the gain, the price and the item id break the remaining ties.
func pickBeats(
    rank float64, item *walkCandidate, best *walkCandidate, bestRank float64,
) bool {
    if best == nil {
        return true
    }
    if rank != bestRank {
        return rank > bestRank
    }
    if item.gain != best.gain {
        return item.gain > best.gain
    }
    if item.candidate.price != best.candidate.price {
        return item.candidate.price < best.candidate.price
    }

    return item.candidate.itemID < best.candidate.itemID
}

// viableCandidates filters the catalog offers down to the ones this
// walk round can take: not planned beyond the family copy count (a
// pair family carries two - the jewel floor fills both halves), a
// strict paperdoll improvement and affordable (the sell credit of the
// displaced pieces included).
//
// The top-tier guard runs here, before the phase ranking: a viable
// candidate that another viable candidate outgains on an overlapping
// set of paperdoll slots is dropped, whatever its gain per adena.
// Without the guard the cheap ladder steps of a slot win the ranking
// (the full weapon gain over the 883 adena short sword beats the same
// slot's 62k top tier by an order of magnitude), so after selling the
// replaced weapon the plan bought the 1k intermediate sword back and
// the one-per-slot guard blocked the affordable top tier for the
// trip. With the guard a slot ladder contributes only its best viable
// step and the phase ranking decides between the per-slot winners.
// The guard holds across the whole plan, not only one walk round:
// ladderTop carries the best gain ever seen per slot. A nil
// ladderTop switches the guard off: the tail walk of the widget queue
// walks the unbounded budget, where the guard would collapse the
// wanted ladder of a slot to its top step and hide the milestones the
// widget shows. The floor offers bypass the guard too (see
// floorOffer): the jewel floor deliberately buys the cheapest offers
// of the empty families. The gain (the net paperdoll delta with the
// family clears) is the dominance measure, so the two hand versus
// one hand plus shield tradeoffs keep their semantics: the strictly
// better end state dominates, an equal gain at a lower price does not
// (the phase ranking keeps the cheaper pick). The survivors return
// with their credits and SellFirst ids.
func (w *planWalk) viableCandidates() []walkCandidate {
    survivors := make([]walkCandidate, 0, len(w.candidates))
    // The anchor of the floor probe is the same reference price of
    // the worn weapon the classification reads (see walkView): the
    // jewel floor opens only behind a real weapon.
    anchor := weaponAnchor(w.virtual)
    for index := range w.candidates {
        candidate := &w.candidates[index]
        if w.planned[candidate.itemID] >= familyCopies(candidate.stats) {
            continue
        }
        gain, ok := purchaseGain(w.virtual, candidate.stats, candidate.score)
        if !ok {
            continue
        }
        slots := affectedSlots(w.virtual, candidate.stats.BodyPart)
        if slotBlocked(w.virtual, candidate.stats.BodyPart, w.slotMarks()) {
            continue
        }
        credit, sellFirst := displacedValue(w.equipment, slots.slice())
        if candidate.price > w.budget+credit {
            continue
        }
        if w.ladderTop != nil && !floorOffer(
            w.strategy, w.virtual, anchor, candidate) {
            if aspiredAbove(w.ladderTop, slots.slice(), gain) {
                continue
            }
            for _, slot := range slots.slice() {
                if gain > w.ladderTop[slot] {
                    w.ladderTop[slot] = gain
                }
            }
        }
        survivors = append(survivors, walkCandidate{
            candidate: candidate,
            gain:      gain,
            credit:    credit,
            sellFirst: sellFirst,
        })
    }

    return survivors
}

// slotMarks resolves the bought-slot map of the walk round from the
// planned copy bookkeeping: a pair family with one copy planned still
// offers its second half (the empty slot), every other family blocks
// its slot once a purchase is planned. The one purchase per slot per
// trip invariant holds through the map.
func (w *planWalk) slotMarks() map[Slot]bool {
    marks := make(map[Slot]bool, len(w.planned))
    for index := range w.candidates {
        candidate := &w.candidates[index]
        if w.planned[candidate.itemID] == 0 {
            continue
        }
        if w.planned[candidate.itemID] >= familyCopies(candidate.stats) {
            for _, slot := range affectedSlots(
                w.virtual, candidate.stats.BodyPart).slice() {
                marks[slot] = true
            }
        }
    }

    return marks
}

// floorOffer reports whether the candidate is the floor offer of its
// family this round: the cheapest jewel of an empty jewel family slot
// behind a real weapon (the anchor above zero). The floor offers
// bypass the top-tier guard of viableCandidates: the floor
// deliberately buys the cheapest offers of the empty families (the
// basic outfit rule of the strategy, the floor fills and never
// replaces), the guard governs the upgrade phases only.
func floorOffer(
    strategy *shopStrategy, virtual [slotCount]ScoredItem, anchor int64,
    candidate *purchaseCandidate,
) bool {
    return CategoryOf(candidate.stats) == CategoryJewel &&
        strategy.floorIDs[candidate.itemID] && anchor > 0 &&
        floorSlotEmpty(virtual, candidate.stats.BodyPart)
}

// aspiredAbove reports whether the slots of the candidate carry the
// recorded gain of a better tier this plan already considered: the
// ladder keeps aiming at that top step, so the intermediate rung
// stays out of the plan even when the eroding budget of the later
// walk rounds would fit it again. The recording runs in the score
// descending scan order of the candidates, so within one walk round
// the scan itself drops every same-slot rung below the best viable
// tier (a lower score item can never outgain a higher score one on
// overlapping slots - the shared displacement subtracts the same
// scores), and the persistence of the map extends the guard over the
// later rounds. A gain equal to the record is not aspired - the
// phase ranking keeps the cheaper of two equal tiers.
func aspiredAbove(ladderTop map[Slot]float64, slots []Slot, gain float64) bool {
    for _, slot := range slots {
        if ladderTop[slot] > gain {
            return true
        }
    }

    return false
}

// familyCopies resolves how many copies of an item one plan may
// carry: the pair families (the rings, the earrings) fill two slots -
// the jewel floor buys both halves - while every other family stays
// at one purchase per trip.
func familyCopies(stats npcdata.GearStats) int32 {
    switch stats.BodyPart {
    case partEars, partFingers:
        return 2
    default:
        return 1
    }
}

// FamilyCopiesOf resolves the copy count of an item family for the
// trip execution: the pair families (the rings, the earrings) carry
// two copies of the same item id (one per slot), every other family
// one. The stop shopping consults it before buying an item the
// inventory already carries (see hunt dropOwnedPurchases).
func FamilyCopiesOf(stats npcdata.GearStats) int32 {
    return familyCopies(stats)
}

// purchaseGain computes the score gain the stats would bring to the
// virtual paperdoll and the slot they improve: the same family logic
// as the equip planner (a one-piece replaces the chest and legs
// family, a two hand weapon drops the shield, a pair slot swap beats
// the weaker half), applied to simulated equips only.
func purchaseGain(
    virtual [slotCount]ScoredItem, stats npcdata.GearStats, score float64,
) (float64, bool) {
    slots := SlotsForBodyPart(stats.BodyPart)
    if len(slots) == 0 {
        return 0, false
    }
    switch stats.BodyPart {
    case partLrhand:
        gain := score - slotScore(virtual[SlotRHand]) -
            slotScore(virtual[SlotLHand])

        return gain, gain > 0
    case partLhand:
        gain := score - slotScore(virtual[SlotLHand])
        if virtual[SlotRHand].Stats.BodyPart == partLrhand {
            gain -= slotScore(virtual[SlotRHand])
        }

        return gain, gain > 0
    case partOnepiece:
        gain := score - slotScore(virtual[SlotChest]) -
            slotScore(virtual[SlotLegs])

        return gain, gain > 0
    case partLegs:
        // Legs against a one-piece chest: the one-piece leaves the
        // chest empty, the chest refill comes as its own pick.
        if virtual[SlotChest].Stats.BodyPart == partOnepiece {
            gain := score - slotScore(virtual[SlotChest])

            return gain, gain > 0
        }
        gain := score - slotScore(virtual[SlotLegs])

        return gain, gain > 0
    case partEars, partFingers:
        first, second := slots[0], slots[1]
        worse := first
        if slotScore(virtual[second]) < slotScore(virtual[first]) {
            worse = second
        }
        gain := score - slotScore(virtual[worse])

        return gain, gain > 0
    default:
        slot := slots[0]
        gain := score - slotScore(virtual[slot])

        return gain, gain > 0
    }
}
