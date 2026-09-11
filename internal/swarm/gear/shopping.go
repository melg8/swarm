// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"math"
	"sort"
	"strconv"
	"sync"

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

// jewelUpgradeLevel gates the jewel upgrades of the shop strategy:
// the starting locations barely attack with magic, so the cheapest
// jewel set covers the mDef needs until the character reaches this
// level.
const jewelUpgradeLevel = 15

// The purchase phases of the shop strategy: every pick of the walk
// ranks by its phase first, the phase specific order second. The order
// is the user rule of the opening game: the cheap armor fills every
// empty defense slot first, the weapon milestone follows, the jewels
// wait for a worn non-starter weapon, the upgrades close the walk.
const (
	// phaseArmorFloor fills the empty armor slots with the cheapest
	// offers of the catalogs (the basic defense outfit, level
	// independent): the opening buys of a fresh character.
	phaseArmorFloor = iota
	// phaseWeapon buys the next weapon milestone: the top
	// affordable strict upgrade - the top-tier guard of
	// bestPurchase keeps the cheaper rungs of the hand slots out
	// of the ranking, whatever their value per adena.
	phaseWeapon
	// phaseFloor fills the empty jewel slots with the cheapest offers
	// of the catalogs (the basic mDef outfit) - only after a real
	// weapon is worn: a starter weapon (or none) keeps the jewel
	// slots empty until the milestone lands.
	phaseFloor
	// phaseDefense upgrades the armor, the shield and (past the jewel
	// level gate) the jewels inside the budget of the worn weapon.
	phaseDefense
)

// shoppingQueueTail bounds the wanted tail of a purchase queue: the
// save up entries the shop widget shows beyond the affordable plan of
// the next trip.
const shoppingQueueTail = 8

// shoppingQueueMin is the minimum entry count of the widget purchase
// queue: the shop list reads as the progression the trips walk over
// time (the affordable plan plus the wanted tail), never a lone
// milestone - the user rule of the widget view (the reported bot
// wearing the starter kit showed a single sword line while the whole
// progression waited behind it). When the plain walk ends short of
// the floor, the wishlist extension continues it: the one purchase
// per slot guard stands down and the weapon phase accepts every
// strict upgrade ranked by its value per adena, so the milestones
// and the defense upgrades inside their grown anchors fill the list.
const shoppingQueueMin = 4

// unboundedBudget widens the budget of the wanted tail walk: half the
// int64 range keeps the price plus credit addition of the walker
// overflow safe while every shop price fits it with room to spare.
const unboundedBudget = math.MaxInt64 / 2

// PlanPurchases plans the purchases of the catalog for the equipment
// within the adena budget, ranked by the strategy phases: the armor
// floor (the cheapest armor pieces filling the empty armor slots -
// the opening buys of a fresh character), the weapon
// milestone (the top affordable strict weapon upgrade - the best tier
// the wallet plus the sale credits reach; the top-tier guard of
// bestPurchase drops every cheaper rung of the same slots whatever
// their value per adena, so the bot that sold its replaced weapon
// buys the tier its adena now covers instead of the intermediate
// sword it just sold), the jewel floor (the cheapest jewel set
// filling the empty slots, opened only after a real weapon is worn:
// the jewels never run ahead of the armor and the weapon) and the
// defense upgrades (the armor, shield and jewel buys ranked by their
// defense gain and bounded by the value of the worn weapon: after
// every weapon tier the defense may grow inside its budget, the next
// weapon tier always outranks it). Nothing gets bought that the
// inventory already carries (the free upgrades are simulated first).
// Simulated equips keep the plan consistent: after a planned purchase
// the virtual paperdoll carries the bought item and the next pick
// compares against it.
//
// Every slot gets at most ONE purchase per trip: each pick marks the
// slots it fills or clears (the family interplay included) and the
// next picks skip the candidates that would write into them. Without
// the guard the planner buys the whole upgrade chain of a slot in a
// single walk (a knife, a short sword and a sickle together, two
// necklaces with only the better one ever worn) and the unused steps
// are pure adena waste - the next trip re-plans against the paperdoll
// the previous purchases reached and upgrades from there.
func PlanPurchases(
	profile Profile, equipment Equipment, catalog Catalog, adena int64,
	level int32,
) []Purchase {
	return planPurchases(profile, equipment, catalog, adena, level, 0)
}

// PlanPurchaseQueue plans the full purchase queue of the shop widget:
// the affordable plan of the next trip first (exactly the
// PlanPurchases picks), then the wanted tail - the picks the wallet
// cannot pay for yet, in the order the strategy wants them. Every
// tail entry carries the adena still missing before everything
// through it becomes affordable, so the widget shows what the bot
// saves up for and how far away it is. The tail picks obey the same
// one purchase per slot per trip guard, so the queue reads as the
// single progression the trips walk over time.
func PlanPurchaseQueue(
	profile Profile, equipment Equipment, catalog Catalog, adena int64,
	level int32,
) []Purchase {
	return planPurchases(
		profile, equipment, catalog, adena, level, shoppingQueueTail)
}

// planPurchases walks the phased planner. The tail parameter appends
// the wanted entries beyond the budget (0 keeps the plain affordable
// plan, shoppingQueueTail serves the widget queue); the walker
// switches into the tail mode when the wallet cannot pay for any
// remaining candidate and widens the budget to unboundedBudget, so
// the same phase ordering continues past the affordability gate. The
// affordable picks of the walk stay byte identical to the plain
// planner: the budget gate and the pick loop are unchanged while the
// wallet lasts. A queue walk that still ends short of shoppingQueueMin
// entries extends through the wishlist mode: the one purchase per slot
// guard stands down and the weapon value gate opens (see
// shopStrategy.wishlist), so the progression the widget shows never
// collapses into a lone milestone.
//
// The budget gate, the pick loop and the wishlist tail are one
// decision tree; splitting them hurts the read flow.
//
//nolint:cyclop,funlen,gocognit // one decision tree (refactor candidate)
func planPurchases(
	profile Profile, equipment Equipment, catalog Catalog, adena int64,
	level int32, tail int,
) []Purchase {
	virtual := SimulateInventory(profile, equipment)
	candidates := catalogCandidates(profile, catalog)
	strategy := &shopStrategy{ //nolint:exhaustruct_v5 // wishlist starts false
		level:         level,
		floorIDs:      cachedCheapestJewelIDs(profile, catalog, candidates),
		armorFloorIDs: cachedCheapestArmorIDs(profile, catalog, candidates),
	}
	purchases := make([]Purchase, 0, len(candidates))
	budget := adena
	planned := make(map[int32]bool)
	boughtSlots := make(map[Slot]bool)
	ladderTop := make(map[Slot]float64)
	tailMode := false
	wishlist := false
	tailLeft := tail
	spent := int64(0)
	credited := int64(0)
	for {
		if !tailMode && budget <= 0 {
			if tailLeft == 0 {
				break
			}
			tailMode = true
			budget = unboundedBudget
			// The wanted tail walks without the top-tier guard: the
			// unbounded budget would collapse the wanted ladder of a
			// slot to its top step and hide the milestones the widget
			// shows (see viableCandidates).
			ladderTop = nil
		}
		best, gain, credit, sellFirst := bestPurchase(
			virtual, candidates, budget, planned, boughtSlots, equipment,
			strategy, ladderTop)
		if best == nil || gain <= 0 {
			if !tailMode && tailLeft > 0 {
				// Nothing affordable remains: the wanted
				// tail continues the walk beyond the wallet.
				tailMode = true
				budget = unboundedBudget
				// The tail walks without the guard, see the budget
				// flip above.
				ladderTop = nil

				continue
			}
			if !wishlist && tail > 0 && len(purchases) < shoppingQueueMin {
				// The queue came out short (the worn gear
				// leaves no empty floor slots and the defense
				// fits no budget): the wishlist extension
				// continues the progression past the one
				// purchase per slot guard.
				wishlist = true
				strategy.wishlist = true
				boughtSlots = make(map[Slot]bool)
				// The wishlist extension walks without the guard, see
				// the budget flip above.
				ladderTop = nil
				if tailLeft < shoppingQueueMin-len(purchases) {
					tailLeft = shoppingQueueMin - len(purchases)
				}
				budget = unboundedBudget

				continue
			}

			break
		}
		planned[best.itemID] = true
		budget += credit
		budget -= best.price
		spent += best.price
		credited += credit
		purchases = append(purchases, walkedPurchase(
			best, gain, credit, sellFirst, adena, spent, credited,
			!tailMode))
		for _, slot := range affectedSlots(virtual, best.stats.BodyPart).slice() {
			boughtSlots[slot] = true
		}
		applyToVirtual(&virtual, boughtEntry(best))
		if tailMode {
			tailLeft--
			if tailLeft <= 0 {
				break
			}
		}
	}

	return purchases
}

// walkView is the per-iteration snapshot of the walk the candidate
// classification reads: the walked virtual paperdoll, the best value
// of the surviving weapon upgrades (the milestone the top-tier guard
// leaves viable), the weapon priced ceiling of the defense phase and
// the reference value of the worn defense gear.
type walkView struct {
	virtual [slotCount]ScoredItem
	target  float64
	anchor  int64
	defense int64
}

// shopStrategy drives the candidate classification of the purchase
// walk: the armor floor (the cheapest armor pieces fill the empty
// armor slots first - the opening buys of a fresh character), the
// jewel floor behind a real weapon (the cheapest set
// fills the empty slots only once the worn weapon is not the starter
// kit: the jewels never run ahead of the armor and the weapon), the
// jewel freeze below jewelUpgradeLevel (the starting locations barely
// attack with magic, the upgrades wait for the level) and the defense
// budget rule (the reference value of the worn defense gear - armor,
// shield, jewels - may not exceed the reference value of the worn
// weapon: the weapon leads the gear progression, the defense follows
// inside its budget). The wishlist switch relaxes the weapon value
// gate for the extension walk of a short queue (see
// shoppingQueueMin): every strict upgrade enters the ranking, the
// widget shows the milestone ladder instead of its best value step
// alone.
type shopStrategy struct {
	level         int32
	floorIDs      map[int32]bool
	armorFloorIDs map[int32]bool
	wishlist      bool
}

// classify resolves the phase and the rank of one candidate against
// the walked paperdoll; ok is false when the strategy skips the
// candidate. The rank orders the picks inside the phase (higher
// wins): the floors by the price (the cheapest offers first), the
// weapon by the value per adena (only the best value strict upgrade
// is eligible - the strategy never buys a worse value weapon just
// because it is cheaper, the wallet saves for the milestone), the
// defense by the raw gain (the maximum defense per buy).
//
// The category switch and the slot guards form one classification
// matrix; splitting them hides the decision.
//
//nolint:cyclop
func (s *shopStrategy) classify(
	view walkView, candidate *purchaseCandidate, gain float64,
) (int, float64, bool) {
	switch CategoryOf(candidate.stats) {
	case CategoryJewel:
		// The jewel floor opens only behind a real weapon: the anchor
		// is the reference price of the worn weapon and stays zero
		// for the starter kit (or an empty hand), so no jewel runs
		// ahead of the armor and the weapon.
		if s.floorIDs[candidate.itemID] && view.anchor > 0 &&
			floorSlotEmpty(view.virtual, candidate.stats.BodyPart) {
			return phaseFloor, -float64(candidate.price), true
		}
		if s.level < jewelUpgradeLevel || !defenseFits(view, candidate) {
			return phaseDefense, 0, false
		}

		return phaseDefense, gain, true
	case CategoryWeapon:
		if candidate.price <= 0 {
			return phaseWeapon, 0, false
		}
		value := gain / float64(candidate.price)
		if value < view.target && !s.wishlist {
			return phaseWeapon, 0, false
		}

		return phaseWeapon, value, true
	case CategoryArmor, CategoryShield:
		// The armor floor outranks everything: the cheapest piece of
		// every empty armor family fills it before the weapon and the
		// jewels, the weapon budget rule does not gate the basic
		// outfit (the shield has no floor entry - it stays a defense
		// upgrade inside the weapon budget).
		if s.armorFloorIDs[candidate.itemID] &&
			floorSlotEmpty(view.virtual, candidate.stats.BodyPart) {
			return phaseArmorFloor, -float64(candidate.price), true
		}
		if !defenseFits(view, candidate) {
			return phaseDefense, 0, false
		}

		return phaseDefense, gain, true
	default:
		return phaseDefense, 0, false
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

// cheapestArmorIDs resolves the cheapest armor offer per armor
// family (the chest, legs, head, gloves, feet and back bodyparts):
// the armor floor buys these only, so the empty armor slots fill
// with the cheapest pieces the shops sell - the opening buys of a
// fresh character, ahead of the weapon and the jewels.
func cheapestArmorIDs(candidates []purchaseCandidate) map[int32]bool {
	type cheapest struct {
		itemID int32
		price  int64
	}
	best := make(map[string]*cheapest, 8)
	for index := range candidates {
		candidate := &candidates[index]
		if !armorFloorBodyPart(candidate.stats) {
			continue
		}
		family := candidate.stats.BodyPart
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

// armorFloorBodyPart reports whether the bodypart belongs to the
// armor floor families: the equippable armor pieces (the chest, legs,
// head, gloves, feet and back slots). The shield stays out - it
// shares the hand family with the weapons (a two hand milestone
// displaces it), so it belongs to the defense phase behind the
// weapon budget like every other upgrade.
func armorFloorBodyPart(stats npcdata.GearStats) bool {
	return CategoryOf(stats) == CategoryArmor
}

// cachedCheapestArmorIDs returns the cheapest armor floor IDs for the
// catalog and profile, cached per (catalog, profile) pair the same
// way the jewel floor cache works (see cachedCheapestJewelIDs).
func cachedCheapestArmorIDs(
	profile Profile, catalog Catalog, candidates []purchaseCandidate,
) map[int32]bool {
	key := candidateCacheKey{
		catalog: &catalog,
		profile: profile.Name(),
		taxHash: catalogTaxHash(catalog),
	}
	if cached, ok := armorIDCache.Load(key); ok {
		return cached.(map[int32]bool)
	}
	ids := cheapestArmorIDs(candidates)
	armorIDCache.Store(key, ids)

	return ids
}

// armorIDCache holds the precomputed cheapest armor floor IDs per
// (catalog, profile) pair, paralleling jewelIDCache.
var armorIDCache sync.Map

// cachedCheapestJewelIDs returns the cheapest jewel IDs for the
// catalog and profile, cached per (catalog, profile) pair. The jewel
// IDs are derived from the (cached) candidates, so they are static
// for a given catalog and profile - the 100 bot fleet was rebuilding
// the same map 100 times every 5 seconds.
func cachedCheapestJewelIDs(
	profile Profile, catalog Catalog, candidates []purchaseCandidate,
) map[int32]bool {
	key := candidateCacheKey{
		catalog: &catalog,
		profile: profile.Name(),
		taxHash: catalogTaxHash(catalog),
	}
	if cached, ok := jewelIDCache.Load(key); ok {
		return cached.(map[int32]bool)
	}
	ids := cheapestJewelIDs(candidates)
	jewelIDCache.Store(key, ids)

	return ids
}

// jewelIDCache holds the precomputed cheapest jewel IDs per (catalog,
// profile) pair, paralleling candidateCache.
var jewelIDCache sync.Map

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
// anchors zero - the first real weapon comes before any armor buy.
func weaponAnchor(virtual [slotCount]ScoredItem) int64 {
	weapon := virtual[SlotRHand]
	if paperdollEmpty(weapon) || starterSet[weapon.Item.ItemID] {
		return 0
	}

	return npcdata.ItemPrice(weapon.Item.ItemID)
}

// defenseValue sums the reference prices of the worn defense gear:
// the armor, shield and jewel entries of the virtual paperdoll.
func defenseValue(virtual [slotCount]ScoredItem) int64 {
	total := int64(0)
	for slot := Slot(0); slot < slotCount; slot++ {
		entry := virtual[slot]
		if paperdollEmpty(entry) {
			continue
		}
		switch CategoryOf(entry.Stats) {
		case CategoryArmor, CategoryShield, CategoryJewel:
			total += npcdata.ItemPrice(entry.Item.ItemID)
		case CategoryUnusable, CategoryWeapon:
			// the carried weapon prices into the anchor, not here
		}
	}

	return total
}

// defenseFits reports whether the defense purchase stays inside the
// weapon budget: the reference value of the worn defense gear after
// the swap may not exceed the anchor of the worn weapon.
func defenseFits(view walkView, candidate *purchaseCandidate) bool {
	if view.anchor <= 0 {
		return false
	}
	displaced := int64(0)
	for _, slot := range affectedSlots(view.virtual,
		candidate.stats.BodyPart).slice() {
		entry := view.virtual[slot]
		if !paperdollEmpty(entry) {
			displaced += npcdata.ItemPrice(entry.Item.ItemID)
		}
	}

	return view.defense-displaced+npcdata.ItemPrice(candidate.itemID) <=
		view.anchor
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
// The result is cached per (catalog pointer, profile name) pair: the
// catalog and the profile are both static for a given bot class and
// region (the elven village merchants never change their stock, the
// melee fighter profile scores items the same way every call), so the
// 100 bot fleet was rebuilding the same offers map 100 times every 5
// seconds - 17 MB of allocations over a 3 minute fleet run. The cache
// collapses that to one build per (catalog, profile) pair for the
// lifetime of the process.
func catalogCandidates(
	profile Profile, catalog Catalog,
) []purchaseCandidate {
	key := candidateCacheKey{
		catalog: &catalog,
		profile: profile.Name(),
		taxHash: catalogTaxHash(catalog),
	}
	if cached, ok := candidateCache.Load(key); ok {
		return cached.([]purchaseCandidate)
	}
	candidates := buildCatalogCandidates(profile, catalog)
	candidateCache.Store(key, candidates)

	return candidates
}

// candidateCacheKey is the composite key of the candidate cache: the
// catalog pointer (stable for the package level townShopCatalog var),
// the profile name (the melee fighter profile is a singleton) and a
// hash of the tax rates (a future region config with a different tax
// table invalidates the cache).
type candidateCacheKey struct {
	catalog *Catalog
	profile string
	taxHash uint64
}

// candidateCache holds the precomputed candidates per (catalog,
// profile) pair. The cache is never cleared: the catalogs and profiles
// are process lifetime singletons, and the number of distinct pairs is
// bounded by the number of bot classes times the number of regions
// (one for the elven deployment today).
var candidateCache sync.Map

// catalogTaxHash computes a stable hash of the tax rates of the catalog
// shops so a different tax table (a future region) invalidates the
// candidate cache. The hash is a simple FNV-1a over the merchant id
// and tax rate pairs.
func catalogTaxHash(catalog Catalog) uint64 {
	var h uint64 = 14695981039346656037
	for _, shop := range catalog.Shops {
		for _, b := range []byte{
			byte(shop.MerchantTemplateID),
			byte(shop.MerchantTemplateID >> 8),
			byte(shop.MerchantTemplateID >> 16),
			byte(shop.MerchantTemplateID >> 24),
		} {
			h ^= uint64(b)
			h *= 1099511628211
		}
		bits := math.Float64bits(shop.TaxRate)
		for range 8 {
			h ^= bits & 0xFF
			h *= 1099511628211
			bits >>= 8
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

// bestPurchase picks the best candidate of the walk iteration under
// the strategy phases: the armor floor, the weapon milestone, the
// jewel floor and the defense upgrades (see shopStrategy.classify).
// The candidates that would write into a slot this plan already
// bought for are skipped (one purchase per slot per trip). The
// affordability counts the sell credit of the pieces the purchase
// displaces (the trip sells them before buying, see displacedValue):
// a replacement is within reach as soon as the adena plus the
// proceeds cover it, so the character shops for it immediately
// instead of hoarding the full price first. The winner returns with
// its credit and the SellFirst object ids.
//
// The top-tier guard of viableCandidates runs before the
// classification: a viable candidate that another viable candidate
// outgains on an overlapping set of paperdoll slots is dropped,
// whatever its value per adena. Without the guard the weapon
// milestone aims at the cheap ladder steps (over an empty slot the
// full weapon gain over the 883 adena short sword beats every
// expensive tier by an order of magnitude), so the bot that sold its
// replaced weapon re-planned against the fresh adena and the empty
// weapon slot and bought the sold sword right back instead of the
// top affordable tier. A nil ladderTop switches the guard off (the
// tail and wishlist modes of the widget queue walk the unbounded
// budget).
func bestPurchase(
	virtual [slotCount]ScoredItem, candidates []purchaseCandidate,
	budget int64, planned map[int32]bool, boughtSlots map[Slot]bool,
	equipment Equipment, strategy *shopStrategy, ladderTop map[Slot]float64,
) (*purchaseCandidate, float64, int64, []int32) {
	survivors := viableCandidates(virtual, candidates, budget, planned,
		boughtSlots, equipment, strategy, ladderTop)
	view := walkView{
		virtual: virtual,
		target:  bestWeaponValue(survivors),
		anchor:  weaponAnchor(virtual),
		defense: defenseValue(virtual),
	}
	var best *walkCandidate
	var bestCandidate *purchaseCandidate
	bestPhase := phaseDefense + 1
	bestRank := float64(0)
	bestGain := float64(0)
	for index := range survivors {
		item := &survivors[index]
		phase, rank, ok := strategy.classify(
			view, item.candidate, item.gain)
		if !ok {
			continue
		}
		if phaseBeats(phase, rank, item.gain, item.candidate,
			bestCandidate, bestPhase, bestRank, bestGain) {
			best = item
			bestCandidate = item.candidate
			bestPhase = phase
			bestRank = rank
			bestGain = item.gain
		}
	}
	if best == nil {
		return nil, 0, 0, nil
	}

	return best.candidate, best.gain, best.credit, best.sellFirst
}

// viableCandidates filters the catalog offers down to the ones this
// walk round can take: not planned yet, a strict paperdoll
// improvement, not writing into a slot the plan already bought for
// and affordable (the sell credit of the displaced pieces included).
// The top-tier guard runs here, before the phase ranking: a viable
// candidate that another viable candidate outgains on an overlapping
// set of paperdoll slots is dropped, whatever its gain per adena.
// Without the guard the cheap ladder steps of a slot win the ranking
// (the full weapon gain over the 883 adena short sword beats the same
// slot's 62k top tier by an order of magnitude), so after selling the
// replaced weapon the plan bought the 1k intermediate sword back and
// the one-per-slot guard blocked the affordable top tier for the
// trip. With the guard a slot ladder contributes only its best viable
// step and the phase ranking decides between the per-slot winners -
// the cheap fillers of the other slots keep their documented
// priority.
//
// The guard holds across the whole plan, not only one walk round:
// ladderTop carries the best gain ever seen per slot, and a later
// round whose budget no longer reaches the top step leaves the slot
// unpurchased instead of buying the intermediate rung the budget
// suddenly fits again (the other picks of the plan must not crowd
// the top tier out and push a cheaper rung in - the bot would spend
// the sale proceeds of its replaced weapon on a downgrade). A nil
// ladderTop switches the guard off: the tail and wishlist modes of
// the widget queue walk the unbounded budget, where the guard would
// collapse the wanted ladder of a slot to its top step and hide the
// milestones the widget shows. The floor offers bypass the guard too
// (see floorOffer): the armor and jewel floors deliberately buy the
// cheapest offers of the empty families, the guard governs the
// upgrade phases only. The gain (the net paperdoll delta with the
// family clears) is the dominance measure, so the two hand versus
// one hand plus shield tradeoffs keep their semantics: the strictly
// better end state dominates, an equal gain at a lower price does not
// (the phase ranking keeps the cheaper pick). The survivors return
// with their credits and SellFirst ids.
func viableCandidates(
	virtual [slotCount]ScoredItem, candidates []purchaseCandidate,
	budget int64, planned map[int32]bool, boughtSlots map[Slot]bool,
	equipment Equipment, strategy *shopStrategy, ladderTop map[Slot]float64,
) []walkCandidate {
	survivors := make([]walkCandidate, 0, len(candidates))
	// The anchor of the floor probe is the same reference price of
	// the worn weapon the classification reads (see walkView): the
	// jewel floor opens only behind a real weapon.
	anchor := weaponAnchor(virtual)
	for index := range candidates {
		candidate := &candidates[index]
		if planned[candidate.itemID] {
			continue
		}
		gain, ok := purchaseGain(virtual, candidate.stats, candidate.score)
		if !ok {
			continue
		}
		slots := affectedSlots(virtual, candidate.stats.BodyPart)
		if slotBlocked(virtual, candidate.stats.BodyPart, boughtSlots) {
			continue
		}
		credit, sellFirst := displacedValue(equipment, slots.slice())
		if candidate.price > budget+credit {
			continue
		}
		if ladderTop != nil && !floorOffer(strategy, virtual, anchor,
			candidate) {
			if aspiredAbove(ladderTop, slots.slice(), gain) {
				continue
			}
			for _, slot := range slots.slice() {
				if gain > ladderTop[slot] {
					ladderTop[slot] = gain
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

// floorOffer reports whether the candidate is the floor offer of its
// family this round: the cheapest armor piece of an empty armor
// family, or the cheapest jewel of an empty jewel family behind a
// real weapon (the anchor above zero). The conditions mirror the
// floor branches of shopStrategy.classify - keep the two in sync.
// The floor offers bypass the top-tier guard of viableCandidates:
// the floors deliberately buy the cheapest offers of the empty
// families (the opening outfit rule of the strategy, the floor fills
// and never replaces), the guard governs the upgrade phases only.
func floorOffer(
	strategy *shopStrategy, virtual [slotCount]ScoredItem, anchor int64,
	candidate *purchaseCandidate,
) bool {
	switch CategoryOf(candidate.stats) {
	case CategoryArmor:
		return strategy.armorFloorIDs[candidate.itemID] &&
			floorSlotEmpty(virtual, candidate.stats.BodyPart)
	case CategoryJewel:
		return strategy.floorIDs[candidate.itemID] && anchor > 0 &&
			floorSlotEmpty(virtual, candidate.stats.BodyPart)
	default:
		return false
	}
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

// phaseBeats reports whether the classified candidate outranks the
// current best pick of the walk: the phase first, the phase rank
// second (higher wins), then the gain, the price and the item id
// break the remaining ties.
func phaseBeats(
	phase int, rank, gain float64, candidate *purchaseCandidate,
	best *purchaseCandidate, bestPhase int, bestRank, bestGain float64,
) bool {
	if best == nil {
		return true
	}
	if phase != bestPhase {
		return phase < bestPhase
	}
	if rank != bestRank {
		return rank > bestRank
	}
	if gain != bestGain {
		return gain > bestGain
	}
	if candidate.price != best.price {
		return candidate.price < best.price
	}

	return candidate.itemID < best.itemID
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
	if paperdollEmpty(entry) {
		return 0
	}

	return entry.Score
}

// slotBuf is a stack-allocated slot list: the maximum number of
// affected slots is 2 (lrhand, onepiece, ears, fingers), so a fixed
// array avoids the heap allocation that []Slot caused on every call.
// The 100 bot fleet called affectedSlots 200-600 times per plan
// computation (20-40 candidates x 5 walk steps x 2-3 calls each),
// which was 3 MB of allocations over a 3 minute run.
type slotBuf struct {
	data [2]Slot
	n    int
}

func (b slotBuf) slice() []Slot { return b.data[:b.n] }

// affectedSlots lists the paperdoll slots an item of the bodypart
// writes to when it is equipped on the virtual paperdoll: its own slot
// plus the family slots the equip clears on the server (a two hand
// weapon drops the shield, a one-piece empties the legs, a shield a
// two hand weapon and legs a one-piece chest). Pair jewels report the
// single slot the equip would take, so a second purchase of the pair
// may still fill the other, empty half. Returns a stack-allocated
// slotBuf so the caller iterates without heap traffic.
func affectedSlots(virtual [slotCount]ScoredItem, bodyPart string) slotBuf {
	switch {
	case bodyPart == partLrhand:
		return slotBuf{data: [2]Slot{SlotRHand, SlotLHand}, n: 2}
	case bodyPart == partOnepiece:
		return slotBuf{data: [2]Slot{SlotChest, SlotLegs}, n: 2}
	case bodyPart == partLhand && virtual[SlotRHand].Stats.BodyPart == partLrhand:
		return slotBuf{data: [2]Slot{SlotLHand, SlotRHand}, n: 2}
	case bodyPart == partLegs && virtual[SlotChest].Stats.BodyPart == partOnepiece:
		return slotBuf{data: [2]Slot{SlotLegs, SlotChest}, n: 2}
	case bodyPart == partEars || bodyPart == partFingers:
		slots := SlotsForBodyPart(bodyPart)
		slot := pairSlot(virtual, slots)
		if slot == slotInvalid {
			return slotBuf{data: [2]Slot{slots[0], slots[1]}, n: 2}
		}

		return slotBuf{data: [2]Slot{slot, 0}, n: 1}
	default:
		slots := SlotsForBodyPart(bodyPart)
		if len(slots) == 0 {
			return slotBuf{data: [2]Slot{}, n: 0}
		}

		return slotBuf{data: [2]Slot{slots[0], 0}, n: 1}
	}
}

// slotBlocked reports whether an item of the bodypart would write
// into a slot the plan already bought for on this trip.
func slotBlocked(
	virtual [slotCount]ScoredItem, bodyPart string, boughtSlots map[Slot]bool,
) bool {
	buf := affectedSlots(virtual, bodyPart)
	for i := range buf.n {
		if boughtSlots[buf.data[i]] {
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

// PlannedEquips lists the object ids of the inventory items the auto
// equipment will wear: the unequipped pieces SimulateInventory places
// on the virtual paperdoll (the free upgrades - the very drops and
// buys the shop plan treats as already owned). The junk flows of the
// hunt loop consult the set before selling or destroying: an item
// scheduled for wearing is never offered for its instant adena or
// destroyed for bag space, the bot puts it on and uses it instead.
// Pieces the simulation leaves off the paperdoll (duplicates, downgrades,
// the displaced halves of pair swaps) stay plain junk.
func PlannedEquips(
	profile Profile, equipment Equipment,
) map[int32]bool {
	equipped := make(map[int32]bool, slotCount)
	for _, objectID := range equipment.Slots {
		if objectID != 0 {
			equipped[objectID] = true
		}
	}
	keeps := make(map[int32]bool, len(equipment.Items))
	virtual := SimulateInventory(profile, equipment)
	for slot := Slot(0); slot < slotCount; slot++ {
		entry := virtual[slot]
		if paperdollEmpty(entry) || equipped[entry.Item.ObjectID] {
			continue
		}
		keeps[entry.Item.ObjectID] = true
	}

	return keeps
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
// fillers replace no one - and the newbie kit items contribute
// nothing too: the server silently skips every unsellable entry of a
// sell request (is_sellable=false in the Mobius item xml, see
// gear.IsStarterItem), so their credit never lands and counting it
// once armed a phantom budget - the reported bot wearing the starter
// dagger planned its sword replacement against 69 adena no shop would
// ever pay, walked to town for it and walked back empty every trip.
// The returned sellFirst slice is allocated with a capacity of 2 (the
// maximum number of affected slots) so the common case of 0-2
// displaced items pays one small allocation instead of a growable
// slice.
func displacedValue(equipment Equipment, slots []Slot) (int64, []int32) {
	var credit int64
	ids := make([]int32, 0, len(slots))
	for _, slot := range slots {
		objectID := equipment.Slots[slot]
		if objectID == 0 {
			continue
		}
		item, ok := equipment.itemByID(objectID)
		if !ok {
			continue
		}
		if starterSet[item.ItemID] {
			// The shops refuse the newbie kit: no credit,
			// no sell-first step - the destroy flow of the
			// replaced starters owns these items.
			continue
		}
		credit += npcdata.ItemPrice(item.ItemID) / 2
		ids = append(ids, objectID)
	}

	return credit, ids
}
