// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "math"
    "sort"
)

// The purchase phases of the shop strategy, in the order of the user spending
// rule: the weapon milestone first (the top affordable strict upgrade, never an
// intermediate rung), the armor set next (the pdef maximizing choice of one
// piece per family inside the remaining budget: a rich wallet buys the advanced
// pieces, a poor one fills many slots with the cheap offers), the jewel floor
// behind a real weapon (the cheapest offer of every empty slot, both pair
// halves: the starting locations barely attack with magic) and the shield with
// the leftover (it shares the hand family, so a two hand weapon blocks it).
const (
    // phaseWeapon buys the weapon milestone: the top affordable strict upgrade,
    // whose top-tier guard keeps the cheaper rungs of the hand slots out of the
    // ranking whatever their value per adena.
    phaseWeapon = iota
    // phaseArmorSet marks the armor candidates of the set planner (see
    // planArmorSet); the walk pick never classifies them, the enumeration owns
    // the armor families.
    phaseArmorSet
    // phaseFloor fills the empty jewel slots with the cheapest offers (the
    // basic mDef outfit), only after a real weapon is worn: a starter weapon
    // keeps the slots empty until the milestone lands.
    phaseFloor
    // phaseShield upgrades the shield with the leftover budget of the
    // trip.
    phaseShield
)

// unboundedBudget widens the budget of the wanted tail walk: half the int64
// range keeps the price plus credit addition overflow safe while every shop
// price fits it with room to spare.
const unboundedBudget = math.MaxInt64 / 2

// planWalk carries the state of one purchase plan walk: the virtual paperdoll
// the simulated equips and the planned purchases build up, the budget
// accounting (the planning wallet, the cumulative spend and the sell credits)
// and the emission bookkeeping. The planned map counts the copies per item id:
// a pair family (the rings, the earrings) may carry two, while every other
// family stays at one purchase per trip.
type planWalk struct {
    virtual    [slotCount]ScoredItem
    candidates []purchaseCandidate
    equipment  Equipment
    strategy   *shopStrategy
    profile    Profile
    catalog    Catalog
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

// newPlanWalk builds the walk state: the virtual paperdoll the free inventory
// upgrades reach, the catalog candidates and the strategy with the cached jewel
// floor ids. The top-tier ladder starts armed; the affordable walk records it
// and the tail walk switches it off.
func newPlanWalk(
    profile Profile, equipment Equipment, catalog Catalog, adena int64,
) *planWalk {
    candidates := catalogCandidates(profile, catalog)

    return &planWalk{
        virtual:    SimulateInventory(profile, equipment),
        candidates: candidates,
        equipment:  equipment,
        profile:    profile,
        catalog:    catalog,
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

// runPhases walks the phases in the strategy order: the weapon milestone, the
// bow and quiver of the melee luring (behind the weapon milestone, worn or
// planned, never ahead of it, see bow.go), the basic jewel floor (the 261 adena
// outfit reserves ahead of the armor maximization, so every slot carries its
// basic jewel while the armor set takes the rest), the pdef maximizing armor
// set and the shield upgrade. The tail budget may end the walk between the
// phases (see tailDone).
func (w *planWalk) runPhases() {
    w.weaponPhase()
    if w.tailDone() {
        return
    }
    w.bowPhase()
    if w.tailDone() {
        return
    }
    w.jewelPhase()
    if w.tailDone() {
        return
    }
    w.armorPhase()
    if w.tailDone() {
        return
    }
    w.shieldPhase()
}

// runTail continues the same phases past the wallet: the entries are wanted
// ones (the widget save up view), the budget unbounds and the top-tier guard
// switches off, so the tail shows the next rung of every ladder. It stops after
// the given count of entries.
func (w *planWalk) runTail(tail int) {
    w.affordable = false
    w.tailLeft = tail
    w.ladderTop = nil
    w.budget = unboundedBudget
    w.runPhases()
}

// weaponPhase buys the weapon milestone: the top affordable strict upgrade of
// the hand slots, with the top-tier guard keeping the cheaper rungs out and the
// value gate pinning the best tier the wallet reaches. One weapon per trip.
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

// armorPhase buys the pdef maximizing armor set: one piece per armor family,
// chosen by the exhaustive enumeration of the per family efficient frontiers
// inside the remaining budget (see enumerateArmorSet), so a rich wallet buys
// the advanced pieces and a poor one fills many slots with the cheap offers.
// The pieces emit cheapest first, since the widget reads the floor fillers
// ahead of the advanced upgrades.
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

// jewelPhase fills every empty jewel slot with the cheapest offer of its
// family, both pair halves included, behind a real (worn or planned) weapon.
// The jewels never upgrade: the starting locations barely attack with magic and
// the cheapest set covers the mDef needs.
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

// shieldPhase buys the best shield strict upgrade the leftover budget reaches:
// the shield shares the hand family with the weapons, so a two hand weapon
// (worn or planned) blocks the pick and a one hand weapon leaves it open. Like
// the jewel floor it waits for a real weapon. One shield per trip.
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

// emit records one walked purchase: the budget pays the price and banks the
// sell credit, the virtual paperdoll wears the piece and the tail budget counts
// down. It reports false when the tail budget is exhausted and the entry never
// landed.
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

// tailDone reports whether the wanted tail walk emitted its whole entry budget;
// the affordable walk never ends here (its tail budget is unlimited).
func (w *planWalk) tailDone() bool {
    return !w.affordable && w.tailLeft <= 0
}
