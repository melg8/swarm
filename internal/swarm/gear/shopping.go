// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"sort"
	"strconv"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// emptyItem and emptyStats are the zero values the cleared virtual
// paperdoll entries carry (plain var declarations: the value types are
// cleared and rebuilt wholesale, never partially constructed).
var (
	emptyItem  state.InventoryItem
	emptyStats npcdata.GearStats
)

// clearedScoredItem is the empty entry a simulated equip writes into a
// paperdoll slot whose item the equip removes.
var clearedScoredItem = ScoredItem{
	Item:  emptyItem,
	Stats: emptyStats,
	Score: 0,
	Slot:  slotInvalid,
}

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
		best, gain, credit, sellFirst := bestPurchase(
			virtual, candidates, budget, planned, boughtSlots, equipment)
		if best == nil || gain <= 0 {
			break
		}
		planned[best.itemID] = true
		budget += credit
		budget -= best.price
		purchases = append(purchases, Purchase{
			ItemID:             best.itemID,
			ListID:             best.listID,
			MerchantTemplateID: best.merchant,
			Count:              1,
			Price:              best.price,
			Reason:             "buying " + best.describe(gain),
			SellFirst:          sellFirst,
			SellCredit:         credit,
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
// The affordability counts the sell credit of the pieces the
// purchase displaces (the trip sells them before buying, see
// displacedValue): a replacement is within reach as soon as the
// adena plus the proceeds cover it, so the character shops for it
// immediately instead of hoarding the full price first. The winner
// returns with its credit and the SellFirst object ids.
func bestPurchase(
	virtual [slotCount]ScoredItem, candidates []purchaseCandidate,
	budget int64, planned map[int32]bool, boughtSlots map[Slot]bool,
	equipment Equipment,
) (*purchaseCandidate, float64, int64, []int32) {
	var best *purchaseCandidate
	bestValue := float64(0)
	bestGain := float64(0)
	var bestCredit int64
	var bestSellFirst []int32
	for index := range candidates {
		candidate := &candidates[index]
		if planned[candidate.itemID] {
			continue
		}
		credit, sellFirst := displacedValue(equipment, affectedSlots(
			virtual, candidate.stats.BodyPart))
		if candidate.price > budget+credit {
			continue
		}
		gain, ok := purchaseGain(virtual, candidate.stats, candidate.score)
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
			best = candidate
			bestValue = value
			bestGain = gain
			bestCredit = credit
			bestSellFirst = sellFirst
		}
	}

	return best, bestGain, bestCredit, bestSellFirst
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
	case bodyPart == partLrhand:
		return []Slot{SlotRHand, SlotLHand}
	case bodyPart == partOnepiece:
		return []Slot{SlotChest, SlotLegs}
	case bodyPart == partLhand && virtual[SlotRHand].Stats.BodyPart == partLrhand:
		return []Slot{SlotLHand, SlotRHand}
	case bodyPart == partLegs && virtual[SlotChest].Stats.BodyPart == partOnepiece:
		return []Slot{SlotLegs, SlotChest}
	case bodyPart == partEars || bodyPart == partFingers:
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
	case stats.BodyPart == partLrhand:
		entry.Slot = SlotRHand
		virtual[SlotRHand] = entry
		virtual[SlotLHand] = clearedScoredItem
	case stats.BodyPart == partOnepiece:
		entry.Slot = SlotChest
		virtual[SlotChest] = entry
		virtual[SlotLegs] = clearedScoredItem
	case stats.BodyPart == partLegs &&
		virtual[SlotChest].Stats.BodyPart == partOnepiece:
		entry.Slot = SlotLegs
		virtual[SlotLegs] = entry
		virtual[SlotChest] = clearedScoredItem
	case stats.BodyPart == partLhand &&
		virtual[SlotRHand].Stats.BodyPart == partLrhand:
		entry.Slot = SlotLHand
		virtual[SlotLHand] = entry
		virtual[SlotRHand] = clearedScoredItem
	case stats.BodyPart == partEars || stats.BodyPart == partFingers:
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
		_, ok := purchaseGain(virtual, candidate.Stats, candidate.Score)
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

// SellCreditOf sums the sell credits of the purchases: the adena the
// trip banks from selling the displaced pieces before the buys.
func SellCreditOf(purchases []Purchase) int64 {
	total := int64(0)
	for _, purchase := range purchases {
		total += purchase.SellCredit
	}

	return total
}

// displacedValue prices the equipped pieces the purchase displaces:
// the real paperdoll occupants of the affected slots (the virtual
// paperdoll decides WHICH slots a purchase writes to, the real
// paperdoll decides WHAT is sold), each at its sell value of
// referencePrice/2. Empty slots contribute nothing - the empty slot
// fillers replace no one.
func displacedValue(equipment Equipment, slots []Slot) (int64, []int32) {
	var credit int64
	var ids []int32
	for _, slot := range slots {
		objectID := equipment.Slots[slot]
		if objectID == 0 {
			continue
		}
		item, ok := equipment.itemByID(objectID)
		if !ok {
			continue
		}
		credit += npcdata.ItemPrice(item.ItemID) / 2
		ids = append(ids, objectID)
	}

	return credit, ids
}
