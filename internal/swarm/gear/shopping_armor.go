// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "sort"
)

// enumerateArmorSetCap bounds the exhaustive armor set enumeration:
// the product of the per family frontier sizes stays under it for
// every real catalog (the elven armor shop holds five families of at
// most five options each); a pathological catalog trims each frontier
// to its top options by gain before the walk, so the enumeration
// never explodes.
const enumerateArmorSetCap = 1 << 16

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
// order, so the plan stays deterministic. The skip choice of every
// family participates, so a family whose every upgrade overflows the
// shared budget stays unpurchased. The real catalogs carry four to six
// options per family; capArmorFrontiers trims pathological frontiers.
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
