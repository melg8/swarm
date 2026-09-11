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

// The purchase phases of the shop strategy. The order is the user
// rule of the spending: the weapon milestone comes first (the top
// affordable strict upgrade - the wallet aims at the best tier it
// reaches, never an intermediate rung), the armor set follows (the
// pdef maximizing choice of one piece per armor family inside the
// remaining budget - a rich wallet buys the advanced pieces directly,
// a poor one fills many slots with the cheap offers), the jewel floor
// closes behind a real weapon (the cheapest offer of every empty
// jewel slot, both halves of every pair - the basic set only: the
// starting locations barely attack with magic, the mDef upgrades
// never pay), and the shield takes the leftover (the shield shares
// the hand family with the weapons - a two hand milestone blocks it).
const (
	// phaseWeapon buys the weapon milestone: the top affordable
	// strict upgrade - the top-tier guard of the pick keeps the
	// cheaper rungs of the hand slots out of the ranking, whatever
	// their value per adena.
	phaseWeapon = iota
	// phaseArmorSet marks the armor candidates of the set planner (see
	// planArmorSet); the walk pick never classifies them - the set
	// enumeration owns the armor families.
	phaseArmorSet
	// phaseFloor fills the empty jewel slots with the cheapest offers
	// of the catalogs (the basic mDef outfit) - only after a real
	// weapon is worn: a starter weapon (or none) keeps the jewel
	// slots empty until the milestone lands.
	phaseFloor
	// phaseShield upgrades the shield with the leftover budget of the
	// trip.
	phaseShield
)

// shoppingQueueTail bounds the wanted tail of a purchase queue: the
// save up entries the shop widget shows beyond the affordable plan of
// the next trip.
const shoppingQueueTail = 8

// unboundedBudget widens the budget of the wanted tail walk: half the
// int64 range keeps the price plus credit addition of the walker
// overflow safe while every shop price fits it with room to spare.
const unboundedBudget = math.MaxInt64 / 2

// enumerateArmorSetCap bounds the exhaustive armor set enumeration:
// the product of the per family frontier sizes stays under it for
// every real catalog (the elven armor shop holds five families of at
// most five options each); a pathological catalog trims each frontier
// to its top options by gain before the walk, so the enumeration
// never explodes.
const enumerateArmorSetCap = 1 << 16

// PlanPurchases plans the purchases of the catalog for the equipment
// within the adena budget, ranked by the strategy phases: the weapon
// milestone first (the top affordable strict upgrade - the best tier
// the wallet plus the sale credits reach; the top-tier guard drops
// every cheaper rung of the same slots whatever their value per
// adena), the pdef maximizing armor set second (one piece per armor
// family inside the remaining budget: a rich wallet buys the advanced
// pieces directly, a poor one fills many slots with the cheap offers -
// the set maximizes the summed pdef either way, the user rule of the
// armor spending), the basic jewel floor behind a real weapon (the
// cheapest offer of every empty slot, both halves of every pair - the
// starting locations barely attack with magic, the mDef upgrades
// never pay) and the shield with the leftover. Nothing gets bought
// that the inventory already carries (the free upgrades are simulated
// first). Simulated equips keep the plan consistent: after a planned
// purchase the virtual paperdoll carries the bought item and the next
// pick compares against it. Every slot gets at most ONE purchase per
// trip: the weapon phase buys one milestone, the set planner picks
// one piece per armor family and the jewel floor fills each empty
// slot once - the next trip re-plans against the paperdoll the
// previous purchases reached.
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

// planWalk carries the state of one purchase plan walk: the virtual
// paperdoll the simulated equips and the planned purchases build up,
// the budget accounting (the planning wallet, the cumulative spend
// and the sell credits) and the emission bookkeeping. The planned map
// counts the copies per item id: a pair family (the rings, the
// earrings) may carry two - the jewel floor fills both halves - while
// every other family stays at one purchase per trip.
type planWalk struct {
	virtual    [slotCount]ScoredItem
	candidates []purchaseCandidate
	equipment  Equipment
	strategy   *shopStrategy
	planned    map[int32]int32
	adena      int64
	spent      int64
	credited   int64
	budget     int64
	ladderTop  map[Slot]float64
	affordable bool
	tailLeft   int
	purchases  []Purchase
}

// newPlanWalk builds the walk state: the virtual paperdoll the free
// inventory upgrades reach, the catalog candidates and the strategy
// with the cached jewel floor ids. The top-tier ladder starts armed
// (the affordable walk records it, the tail walk switches it off).
func newPlanWalk(
	profile Profile, equipment Equipment, catalog Catalog, adena int64,
) *planWalk {
	candidates := catalogCandidates(profile, catalog)

	return &planWalk{
		virtual:    SimulateInventory(profile, equipment),
		candidates: candidates,
		equipment:  equipment,
		strategy: &shopStrategy{
			floorIDs: cachedCheapestJewelIDs(profile, catalog, candidates),
		},
		planned:    make(map[int32]int32, len(candidates)),
		adena:      adena,
		spent:      0,
		credited:   0,
		budget:     adena,
		ladderTop:  make(map[Slot]float64),
		affordable: true,
		tailLeft:   0,
		purchases:  make([]Purchase, 0, 16),
	}
}

// runPhases walks the phases in the strategy order: the weapon
// milestone, the pdef maximizing armor set, the basic jewel floor and
// the shield upgrade. The tail budget may end the walk between the
// phases (see tailDone).
func (w *planWalk) runPhases() {
	w.weaponPhase()
	if w.tailDone() {
		return
	}
	w.armorPhase()
	if w.tailDone() {
		return
	}
	w.jewelPhase()
	if w.tailDone() {
		return
	}
	w.shieldPhase()
}

// runTail continues the same phases past the wallet: the entries are
// wanted ones (the widget save up view), the budget unbounds and the
// top-tier guard switches off so the tail shows the next rung of
// every ladder. The tail stops after the given count of entries.
func (w *planWalk) runTail(tail int) {
	w.affordable = false
	w.tailLeft = tail
	w.ladderTop = nil
	w.budget = unboundedBudget
	w.runPhases()
}

// weaponPhase buys the weapon milestone: the top affordable strict
// upgrade of the hand slots (the top-tier guard keeps the cheaper
// rungs out, the value gate pins the best tier the wallet reaches).
// One weapon per trip.
func (w *planWalk) weaponPhase() {
	if w.tailDone() {
		return
	}
	best, gain, credit, sellFirst := w.bestPick(phaseWeapon)
	if best == nil {
		return
	}
	w.emit(best, gain, credit, sellFirst)
}

// armorPhase buys the pdef maximizing armor set: one piece per armor
// family, chosen by the exhaustive enumeration of the per family
// efficient frontiers inside the remaining budget (see
// enumerateArmorSet). A rich wallet buys the advanced pieces
// directly, a poor one fills many slots with the cheap offers - the
// set maximizes the summed pdef either way. The pieces emit cheapest
// first (the widget reads the floor fillers ahead of the advanced
// upgrades).
func (w *planWalk) armorPhase() {
	set := w.planArmorSet()
	if len(set) == 0 {
		return
	}
	sort.Slice(set, func(i int, j int) bool {
		if set[i].candidate.price != set[j].candidate.price {
			return set[i].candidate.price < set[j].candidate.price
		}

		return set[i].candidate.itemID < set[j].candidate.itemID
	})
	for index := range set {
		if w.tailDone() {
			return
		}
		option := &set[index]
		if !w.emit(option.candidate, option.gain, option.credit,
			option.sellFirst) {
			return
		}
	}
}

// jewelPhase fills every empty jewel slot with the cheapest offer of
// its family: the basic mDef set, both halves of the pairs included,
// behind a real (worn or planned) weapon. The jewels never upgrade:
// the starting locations barely attack with magic, the cheapest set
// covers the mDef needs (the user rule of the basic jewels).
func (w *planWalk) jewelPhase() {
	if weaponAnchor(w.virtual) <= 0 {
		return
	}
	for {
		if w.tailDone() {
			return
		}
		best, gain, credit, sellFirst := w.bestPick(phaseFloor)
		if best == nil {
			return
		}
		if !w.emit(best, gain, credit, sellFirst) {
			return
		}
	}
}

// shieldPhase buys the best shield strict upgrade the leftover budget
// reaches: the shield shares the hand family with the weapons, so a
// two hand weapon (worn or planned) blocks the pick and a one hand
// weapon leaves it open. The pick waits for a real (worn or planned)
// weapon like the jewel floor - a bare-handed or starter-armed
// character saves for the weapon first. One shield per trip.
func (w *planWalk) shieldPhase() {
	if w.tailDone() || weaponAnchor(w.virtual) <= 0 {
		return
	}
	best, gain, credit, sellFirst := w.bestPick(phaseShield)
	if best == nil {
		return
	}
	w.emit(best, gain, credit, sellFirst)
}

// emit records one walked purchase: the budget pays the price and
// banks the sell credit, the virtual paperdoll wears the piece and
// the tail budget counts down. It reports false when the tail budget
// of the queue walk is exhausted and the entry never landed.
func (w *planWalk) emit(
	best *purchaseCandidate, gain float64, credit int64, sellFirst []int32,
) bool {
	if w.tailDone() {
		return false
	}
	w.planned[best.itemID]++
	w.budget += credit - best.price
	w.spent += best.price
	w.credited += credit
	w.purchases = append(w.purchases, walkedPurchase(
		best, gain, credit, sellFirst, w.adena, w.spent, w.credited,
		w.affordable))
	applyToVirtual(&w.virtual, boughtEntry(best))
	if !w.affordable {
		w.tailLeft--
	}

	return true
}

// tailDone reports whether the wanted tail walk already emitted its
// whole entry budget: the affordable walk never ends here (its tail
// budget is unlimited).
func (w *planWalk) tailDone() bool {
	return !w.affordable && w.tailLeft <= 0
}

// armorOption is one candidate choice of one armor family: the pdef
// gain over the worn (or planned) piece and the net price the budget
// pays after the sell credit of the displaced real piece.
type armorOption struct {
	candidate *purchaseCandidate
	gain      float64
	cost      int64
	credit    int64
	sellFirst []int32
}

// armorFamilies lists the armor bodypart families of the set planner
// in a fixed order (the enumeration and the emission stay
// deterministic). The one-piece armor stays out: it owns the chest
// and the legs slots at once and no catalog of the deployment sells
// one (the guard keeps a future catalog honest - a onepiece family
// needs the joint slot interplay of the equip planner, not a blind
// set entry).
var armorFamilies = []string{
	partChest, partLegs, partHead, partGloves, partFeet, partBack,
}

// planArmorSet resolves the pdef maximizing armor set of the walk:
// the strict upgrade options of every armor family (pruned to their
// efficient frontiers), enumerated exhaustively inside the budget.
func (w *planWalk) planArmorSet() []armorOption {
	options := make([][]armorOption, 0, len(armorFamilies))
	for _, family := range armorFamilies {
		frontier := w.familyFrontier(family)
		if len(frontier) == 0 {
			continue
		}
		options = append(options, frontier)
	}

	return enumerateArmorSet(options, w.budget)
}

// familyFrontier builds the efficient frontier of one armor family:
// the strict upgrades over the virtual slot occupant with their net
// costs (the price minus the sell credit of the real displaced
// piece), pruned so every option gains strictly more pdef than every
// cheaper one - a dominated option never wins the enumeration. Equal
// gain and cost ties keep the lower item id (the determinism).
func (w *planWalk) familyFrontier(family string) []armorOption {
	slots := SlotsForBodyPart(family)
	if len(slots) != 1 {
		return nil
	}
	slot := slots[0]
	var options []armorOption
	for index := range w.candidates {
		candidate := &w.candidates[index]
		if candidate.stats.BodyPart != family ||
			CategoryOf(candidate.stats) != CategoryArmor {
			continue
		}
		if w.planned[candidate.itemID] != 0 {
			continue
		}
		gain := candidate.score - slotScore(w.virtual[slot])
		if gain <= 0 {
			continue
		}
		credit, sellFirst := displacedValue(w.equipment, []Slot{slot})
		cost := candidate.price - credit
		if cost > w.budget {
			continue
		}
		options = append(options, armorOption{
			candidate: candidate,
			gain:      gain,
			cost:      cost,
			credit:    credit,
			sellFirst: sellFirst,
		})
	}

	return pruneArmorFrontier(options)
}

// pruneArmorFrontier drops the dominated options of a family: the
// candidates sorted by cost ascending, every option that does not
// gain strictly more pdef than the best cheaper one leaves (an equal
// or lower gain at an equal or higher cost never wins a budget
// limited enumeration).
func pruneArmorFrontier(options []armorOption) []armorOption {
	if len(options) == 0 {
		return nil
	}
	sort.Slice(options, func(i int, j int) bool {
		if options[i].cost != options[j].cost {
			return options[i].cost < options[j].cost
		}
		if options[i].gain != options[j].gain {
			return options[i].gain > options[j].gain
		}

		return options[i].candidate.itemID < options[j].candidate.itemID
	})
	frontier := make([]armorOption, 0, len(options))
	bestGain := float64(0)
	for _, option := range options {
		if option.gain <= bestGain {
			continue
		}
		frontier = append(frontier, option)
		bestGain = option.gain
	}

	return frontier
}

// enumerateArmorSet walks the cross product of the family frontiers
// and returns the set with the maximum summed pdef gain inside the
// budget: ties prefer the cheaper set, then the earlier traversal
// order (the plan stays deterministic). The skip choice of every
// family participates, so a family whose every upgrade overflows the
// shared budget stays unpurchased. The frontier sizes of the real
// catalogs are tiny (four to six options per family); the
// enumerateArmorSetCap guard trims pathological frontiers to their
// top options by gain before the walk.
func enumerateArmorSet(
	options [][]armorOption, budget int64,
) []armorOption {
	options = capArmorFrontiers(options)
	var best []armorOption
	bestGain := float64(0)
	bestCost := int64(0)
	choice := make([]armorOption, 0, len(options))
	var walk func(index int, gain float64, cost int64)
	walk = func(index int, gain float64, cost int64) {
		if index == len(options) {
			if len(choice) == 0 {
				return
			}
			if best == nil || gain > bestGain ||
				(gain == bestGain && cost < bestCost) {
				best = append([]armorOption(nil), choice...)
				bestGain = gain
				bestCost = cost
			}

			return
		}
		// The skip choice: the family stays with its worn piece.
		walk(index+1, gain, cost)
		for _, option := range options[index] {
			if cost+option.cost > budget {
				continue
			}
			choice = append(choice, option)
			walk(index+1, gain+option.gain, cost+option.cost)
			choice = choice[:len(choice)-1]
		}
	}
	walk(0, 0, 0)

	return best
}

// capArmorFrontiers trims the family frontiers when their cross
// product would explode past enumerateArmorSetCap: every frontier
// keeps its top options by gain (the frontier order is the cost
// ascending one, the gain ascends with it). The real catalogs never
// reach the cap - the guard serves a future mega catalog.
func capArmorFrontiers(options [][]armorOption) [][]armorOption {
	product := 1
	for _, frontier := range options {
		product *= len(frontier) + 1
	}
	if product <= enumerateArmorSetCap {
		return options
	}
	for index, frontier := range options {
		if len(frontier) > 4 {
			options[index] = frontier[len(frontier)-4:]
		}
	}

	return options
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

// jewelIDCache holds the precomputed cheapest jewel IDs per (catalog,
// profile) pair, paralleling candidateCache.
var jewelIDCache sync.Map

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
