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

// walkView is the per-iteration snapshot the classification reads: the walked
// virtual paperdoll, the best surviving weapon value and the weapon anchor of
// the jewel floor gate.
type walkView struct {
    virtual [slotCount]ScoredItem
    target  float64
    anchor  int64
}

// shopStrategy drives the candidate classification: the jewel floor behind a
// real weapon (the cheapest set fills every empty slot, the pair halves
// included, once the worn weapon is not the starter kit) and the basic-only
// jewel rule (the starting locations barely attack with magic, the floor set
// covers the mDef needs).
type shopStrategy struct {
    floorIDs map[int32]bool
}

// classify resolves the phase and the rank of one candidate against the walked
// paperdoll; ok is false when the strategy skips it. The rank orders the picks
// inside the phase (higher wins): the weapons by the value per adena (only the
// best value strict upgrade is eligible - the wallet saves for the milestone),
// the jewel floor by the price, the shield by the raw gain. The armor never
// walks the pick: the set planner owns the armor families (see planArmorSet).
func (s *shopStrategy) classify(
    view walkView, candidate *purchaseCandidate, gain float64,
) (int, float64, bool) {
    switch CategoryOf(candidate.stats) {
    case CategoryJewel:
        // The jewel floor opens only behind a real weapon (a non zero
        // anchor) and fills every empty slot with the cheapest offer of
        // the family; it never replaces a worn jewel.
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

// cheapestJewelIDs resolves the cheapest jewel offer per family (the ring,
// earring and necklace bodyparts); the floor buys these only.
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

// cachedCheapestJewelIDs returns the cheapest jewel IDs for the catalog and
// profile, cached per (catalog content, profile) pair like the candidates
// themselves.
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

// bestWeaponValue resolves the value per adena of the best weapon survivor: the
// weapon phase buys only this one (the value gate of classify), and the
// survivors already carry the top-tier guard, so the milestone is the best tier
// the wallet reaches.
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

// walkedPurchase builds one entry of the queue walk: the cumulative adena
// accounting decides the missing amount (0 while the wallet covers everything
// through this entry) and the tail mode marks the entry unaffordable.
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

// catalogCandidates joins the shop offers with the item gear stats, keeping the
// cheapest offer per item id (the same item can appear in several buylists) and
// dropping everything the profile cannot use. The result is cached per (catalog
// content, profile name) pair - both are static for a bot class and region, so
// one build serves the process lifetime (docs/shopping_strategy.md, the
// candidate caches).
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

// candidateCacheKey is the composite key of the gear caches: the profile name
// and the content hash of the catalog. Never a pointer - the catalog travels by
// value, so a pointer key addresses the per call copy (see catalogCandidates).
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

// candidateCache holds the precomputed candidates per (catalog content,
// profile) pair, never cleared: the pairs are bounded by the bot classes times
// the regions (one for the elven deployment today).
var candidateCache sync.Map

// catalogHash computes a stable hash of everything the candidate build reads
// from the catalog (the merchant ids, the tax rates and the buylist ids of
// every shop, FNV-1a): equal content hashes equal, so a by value copy hits the
// entry its source built.
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

// buildCatalogCandidates is the uncached implementation of catalogCandidates.
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

// bestPick resolves the best candidate of one phase: the viability gates of
// viableCandidates, the top-tier slot guard (a nil ladderTop switches it off -
// the tail walk shows the ladder) and the classification with its ranking over
// the candidates of that phase only. The winner returns with its credit and the
// SellFirst object ids.
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

// viableCandidates filters the catalog offers down to the ones this walk round
// can take: not planned beyond the family copy count (the pair families carry
// two), a strict paperdoll improvement and affordable with the displaced
// credits. The top-tier guard runs here before the phase ranking: a candidate
// another viable one outgains on overlapping slots is dropped whatever its
// value per adena, and ladderTop keeps the best gain ever seen per slot across
// the plan (a nil map switches it off, the floor offers bypass it); the gain is
// the dominance measure (docs/shopping_strategy.md, the top-tier slot guard).
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

// slotMarks resolves the bought-slot map from the planned copy bookkeeping: a
// pair family with one copy planned still offers its second half (the empty
// slot), every other family blocks its slot once a purchase is planned.
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

// floorOffer reports whether the candidate is the floor offer of its family:
// the cheapest jewel of an empty jewel family slot behind a real weapon. The
// floor offers bypass the top-tier guard - it governs the upgrade phases only.
func floorOffer(
    strategy *shopStrategy, virtual [slotCount]ScoredItem, anchor int64,
    candidate *purchaseCandidate,
) bool {
    return CategoryOf(candidate.stats) == CategoryJewel &&
        strategy.floorIDs[candidate.itemID] && anchor > 0 &&
        floorSlotEmpty(virtual, candidate.stats.BodyPart)
}

// aspiredAbove reports whether the candidate aims at a slot whose recorded gain
// already came from a better tier this plan considered; an equal gain is not
// aspired (the phase ranking keeps the cheaper of two equal tiers). The
// recording runs in the score descending scan order, so within one round the
// scan itself drops every same-slot rung below the best viable tier (a lower
// score item cannot outgain a higher score one on overlapping slots - the
// shared displacement subtracts the same scores) and the map extends the guard
// over the later rounds.
func aspiredAbove(ladderTop map[Slot]float64, slots []Slot, gain float64) bool {
    for _, slot := range slots {
        if ladderTop[slot] > gain {
            return true
        }
    }

    return false
}

// familyCopies resolves how many copies of an item one plan may carry: the pair
// families (the rings, the earrings) fill two slots, every other family one.
func familyCopies(stats npcdata.GearStats) int32 {
    switch stats.BodyPart {
    case partEars, partFingers:
        return 2
    default:
        return 1
    }
}

// FamilyCopiesOf resolves the copy count of an item family for the trip
// execution (see familyCopies); the stop shopping consults it before buying an
// item the inventory already carries (see hunt dropOwnedPurchases).
func FamilyCopiesOf(stats npcdata.GearStats) int32 {
    return familyCopies(stats)
}

// purchaseGain computes the score gain the stats would bring to the virtual
// paperdoll: the same family logic as the equip planner (a one-piece replaces
// the chest and legs family, a two hand weapon drops the shield, a pair swap
// beats the weaker half), applied to simulated equips only.
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
        // The one-piece leaves the chest empty: the chest refill comes
        // as its own pick.
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
