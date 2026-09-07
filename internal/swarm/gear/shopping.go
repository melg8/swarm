// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
        "sort"
        "strconv"

        "github.com/melg8/swarm/internal/swarm/npcdata"
)

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
        // Price is the buy price of the order with the shop tax.
        Price int64
        // Reason is the human readable log line.
        Reason string
}

// purchaseCandidate is one shop offer joined with the item stats.
type purchaseCandidate struct {
        itemID   int32
        listID   int32
        merchant int32
        price    int64
        stats    npcdata.GearStats
        score    float64
}

// shopTaxLimit bounds the tax sanity: a shop with a tax rate above
// this margin is rejected as broken data.
const shopTaxLimit = 2.0

// PlanPurchases greedily plans the best value per adena purchases of
// the catalog for the equipment within the adena budget. The score
// gain per adena decides every pick, so the cheap empty slot fillers
// (a cloth cap for a handful of adena) come before the expensive
// weapon upgrades unless the weapon gain outweighs them, and nothing
// gets bought that the inventory already carries (the free upgrades
// are simulated first). Simulated equips keep the plan consistent:
// after a planned purchase the virtual paperdoll carries the bought
// item and the next pick compares against it.
//
// Every slot gets at most ONE purchase per trip: each pick marks the
// slots it fills or clears (the family interplay included) and the
// next picks skip the candidates that would write into them. Without
// the guard the greedy planner buys the whole upgrade chain of a slot
// in a single walk (a knife, a short sword and a sickle together, two
// necklaces with only the better one ever worn) and the unused steps
// are pure adena waste - the next trip re-plans against the paperdoll
// the previous purchases reached and upgrades from there.
func PlanPurchases(
        profile Profile, equipment Equipment, catalog Catalog, adena int64,
) []Purchase {
        virtual := SimulateInventory(profile, equipment)
        candidates := catalogCandidates(profile, catalog)
        purchases := make([]Purchase, 0, len(candidates))
        budget := adena
        planned := make(map[int32]bool)
        boughtSlots := make(map[Slot]bool)
        for budget > 0 {
                best, gain := bestPurchase(virtual, candidates, budget, planned, boughtSlots)
                if best == nil || gain <= 0 {
                        break
                }
                planned[best.itemID] = true
                budget -= best.price
                purchases = append(purchases, Purchase{
                        ItemID:             best.itemID,
                        ListID:             best.listID,
                        MerchantTemplateID: best.merchant,
                        Count:              1,
                        Price:              best.price,
                        Reason:             "buying " + best.describe(gain),
                })
                for _, slot := range affectedSlots(virtual, best.stats.BodyPart) {
                        boughtSlots[slot] = true
                }
                //nolint:exhaustruct // a bought item has no inventory entry yet
                applyToVirtual(&virtual, ScoredItem{
                        Stats: best.stats,
                        Score: best.score,
                })
        }

        return purchases
}

// catalogCandidates joins the shop offers with the item gear stats,
// keeping the cheapest offer per item id (the same item can appear in
// several buylists) and dropping everything the profile cannot use.
func catalogCandidates(
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

// bestPurchase picks the affordable candidate with the highest score
// gain per adena; the plain gain breaks ties between equally priced
// offers. The candidates that would write into a slot this plan
// already bought for are skipped (one purchase per slot per trip).
func bestPurchase(
        virtual [slotCount]ScoredItem, candidates []purchaseCandidate,
        budget int64, planned map[int32]bool, boughtSlots map[Slot]bool,
) (*purchaseCandidate, float64) {
        var best *purchaseCandidate
        bestValue := float64(0)
        bestGain := float64(0)
        for index := range candidates {
                candidate := &candidates[index]
                if planned[candidate.itemID] || candidate.price > budget {
                        continue
                }
                gain, _, ok := purchaseGain(virtual, candidate.stats, candidate.score)
                if !ok {
                        continue
                }
                if slotBlocked(virtual, candidate.stats.BodyPart, boughtSlots) {
                        continue
                }
                value := gain
                if candidate.price > 0 {
                        value = gain / float64(candidate.price)
                }
                if value > bestValue || (value == bestValue && gain > bestGain) {
                        best, bestValue, bestGain = candidate, value, gain
                }
        }

        return best, bestGain
}

// purchaseGain computes the score gain the stats would bring to the
// virtual paperdoll and the slot they improve: the same family logic
// as the equip planner (a one-piece replaces the chest and legs
// family, a two hand weapon drops the shield, a pair slot swap beats
// the weaker half), applied to simulated equips only.
func purchaseGain(
        virtual [slotCount]ScoredItem, stats npcdata.GearStats, score float64,
) (float64, Slot, bool) {
        slots := SlotsForBodyPart(stats.BodyPart)
        if len(slots) == 0 {
                return 0, slotInvalid, false
        }
        switch stats.BodyPart {
        case "lrhand":
                gain := score - slotScore(virtual[SlotRHand]) -
                        slotScore(virtual[SlotLHand])

                return gain, SlotRHand, gain > 0
        case "lhand":
                gain := score - slotScore(virtual[SlotLHand])
                if virtual[SlotRHand].Stats.BodyPart == "lrhand" {
                        gain -= slotScore(virtual[SlotRHand])
                }

                return gain, SlotLHand, gain > 0
        case "onepiece":
                gain := score - slotScore(virtual[SlotChest]) -
                        slotScore(virtual[SlotLegs])

                return gain, SlotChest, gain > 0
        case "legs":
                // Legs against a one-piece chest: the one-piece leaves the
                // chest empty, the chest refill comes as its own pick.
                if virtual[SlotChest].Stats.BodyPart == "onepiece" {
                        gain := score - slotScore(virtual[SlotChest])

                        return gain, SlotLegs, gain > 0
                }
                gain := score - slotScore(virtual[SlotLegs])

                return gain, SlotLegs, gain > 0
        case "rear;lear", "rfinger;lfinger":
                first, second := slots[0], slots[1]
                worse := first
                if slotScore(virtual[second]) < slotScore(virtual[first]) {
                        worse = second
                }
                gain := score - slotScore(virtual[worse])

                return gain, worse, gain > 0
        default:
                slot := slots[0]
                gain := score - slotScore(virtual[slot])

                return gain, slot, gain > 0
        }
}

// slotScore returns the score of the virtual slot entry (0 for an
// empty slot).
func slotScore(entry ScoredItem) float64 {
        if entry.Item.ObjectID == 0 && entry.Stats.BodyPart == "" {
                return 0
        }

        return entry.Score
}

// affectedSlots lists the paperdoll slots an item of the bodypart
// writes to when it is equipped on the virtual paperdoll: its own slot
// plus the family slots the equip clears on the server (a two hand
// weapon drops the shield, a one-piece empties the legs, a shield a
// two hand weapon and legs a one-piece chest). Pair jewels report the
// single slot the equip would take, so a second purchase of the pair
// may still fill the other, empty half.
func affectedSlots(virtual [slotCount]ScoredItem, bodyPart string) []Slot {
        switch {
        case bodyPart == "lrhand":
                return []Slot{SlotRHand, SlotLHand}
        case bodyPart == "onepiece":
                return []Slot{SlotChest, SlotLegs}
        case bodyPart == "lhand" && virtual[SlotRHand].Stats.BodyPart == "lrhand":
                return []Slot{SlotLHand, SlotRHand}
        case bodyPart == "legs" && virtual[SlotChest].Stats.BodyPart == "onepiece":
                return []Slot{SlotLegs, SlotChest}
        case bodyPart == "rear;lear" || bodyPart == "rfinger;lfinger":
                slots := SlotsForBodyPart(bodyPart)
                slot := pairSlot(virtual, slots)
                if slot == slotInvalid {
                        return slots
                }

                return []Slot{slot}
        default:
                slots := SlotsForBodyPart(bodyPart)
                if len(slots) == 0 {
                        return nil
                }

                return slots[:1]
        }
}

// slotBlocked reports whether an item of the bodypart would write
// into a slot the plan already bought for on this trip.
func slotBlocked(
        virtual [slotCount]ScoredItem, bodyPart string, boughtSlots map[Slot]bool,
) bool {
        for _, slot := range affectedSlots(virtual, bodyPart) {
                if boughtSlots[slot] {
                        return true
                }
        }

        return false
}

// applyToVirtual equips the entry on the virtual paperdoll with the
// same family effects the server applies: a one-piece empties the
// legs, a two hand weapon the left hand, a shield a two hand weapon
// and the legs a one-piece chest.
func applyToVirtual(virtual *[slotCount]ScoredItem, entry ScoredItem) {
        stats := entry.Stats
        slots := SlotsForBodyPart(stats.BodyPart)
        switch {
        case stats.BodyPart == "lrhand":
                entry.Slot = SlotRHand
                virtual[SlotRHand] = entry
                virtual[SlotLHand] = ScoredItem{}
        case stats.BodyPart == "onepiece":
                entry.Slot = SlotChest
                virtual[SlotChest] = entry
                virtual[SlotLegs] = ScoredItem{}
        case stats.BodyPart == "legs" &&
                virtual[SlotChest].Stats.BodyPart == "onepiece":
                entry.Slot = SlotLegs
                virtual[SlotLegs] = entry
                virtual[SlotChest] = ScoredItem{}
        case stats.BodyPart == "lhand" &&
                virtual[SlotRHand].Stats.BodyPart == "lrhand":
                entry.Slot = SlotLHand
                virtual[SlotLHand] = entry
                virtual[SlotRHand] = ScoredItem{}
        case stats.BodyPart == "rear;lear" || stats.BodyPart == "rfinger;lfinger":
                entry.Slot = pairSlot(*virtual, slots)
                if entry.Slot != slotInvalid {
                        virtual[entry.Slot] = entry
                }
        case len(slots) > 0:
                entry.Slot = slots[0]
                virtual[slots[0]] = entry
        }
}

// pairSlot picks the pair slot an equip fills: the empty one or the
// weaker one (mirroring the equip planner pair swap).
func pairSlot(virtual [slotCount]ScoredItem, slots []Slot) Slot {
        if len(slots) != 2 {
                return slotInvalid
        }
        first, second := slots[0], slots[1]
        if virtual[first].Item.ObjectID == 0 &&
                virtual[first].Stats.BodyPart == "" {
                return first
        }
        if virtual[second].Item.ObjectID == 0 &&
                virtual[second].Stats.BodyPart == "" {
                return second
        }
        if slotScore(virtual[second]) < slotScore(virtual[first]) {
                return second
        }

        return first
}

// SimulateInventory applies every free inventory upgrade to a copy of
// the paperdoll: the purchases plan against the paperdoll the auto
// equipment will reach anyway, so nothing gets bought that the
// inventory already carries.
func SimulateInventory(
        profile Profile, equipment Equipment,
) [slotCount]ScoredItem {
        virtual := equipment.Paperdoll(profile)
        candidates := scoreUnequipped(profile, equipment)
        for _, candidate := range candidates {
                _, _, ok := purchaseGain(virtual, candidate.Stats, candidate.Score)
                if ok {
                        applyToVirtual(&virtual, candidate)
                }
        }

        return virtual
}

// describe renders the candidate for purchase logs.
func (c *purchaseCandidate) describe(gain float64) string {
        name := npcdata.ItemName(c.itemID)
        if name == "" {
                name = "item #" + strconv.Itoa(int(c.itemID))
        }
        gainText := strconv.FormatFloat(gain, 'f', -1, 64)
        priceText := strconv.FormatInt(c.price, 10)

        return name + " (+" + gainText + " for " + priceText + " adena)"
}

// AdenaSpent sums the prices of the purchases.
func AdenaSpent(purchases []Purchase) int64 {
        total := int64(0)
        for _, purchase := range purchases {
                total += purchase.Price
        }

        return total
}
