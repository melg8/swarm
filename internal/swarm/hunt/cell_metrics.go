// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "sync"
    "time"
)

// The empirical measurement layer of the cell hunting: per cell
// rolling statistics of the session replace the gear points of the
// old zone gates. The metrics measure what the gates tried to
// predict: the loot income of the ground for THIS character (adena
// per active minute, rest included - hard fights internalize their
// own cost) and the danger of the ground (a decayed death heat). An
// undergeared bot automatically scores worse where it takes damage
// and drifts to easier cells without any MinGear numbers.

// The scoring constants of the cell economy (the spot economy values
// carried over: the elven respawn turnover and the C1 drop curve the
// live sessions measured).
const (
    // cellMetricsTrustMin is the active hunting time a ground needs
    // before the measured rate replaces the bootstrap prior.
    cellMetricsTrustMin = 5.0
    // cellKillCapacity is the kill rate a solo farmer sustains on
    // white-green mobs (a kill every five seconds incl. the pull
    // walk); the supply turnover bounds it below on sparse grounds.
    cellKillCapacity = 12.0
    // cellAdenaBase and cellAdenaPerLevel price one kill of the
    // bootstrap prior: the C1 elven drop curve rises with the mob
    // level, the measured rate replaces the curve as soon as the
    // real loot flows.
    cellAdenaBase     = 18.0
    cellAdenaPerLevel = 6.5
    // cellAggroPenalty is the static discount of an aggressive
    // heavy ground (the aggressive share of the mass pays this
    // fraction of the score).
    cellAggroPenalty = 0.35
    // cellDeathHeatWeight scales the decayed death heat inside the
    // safety multiplier: one death per hour costs a third of the
    // score.
    cellDeathHeatWeight = 2.0
    // cellDeathHalfLifeMin is the half-life of the death heat: an
    // hour old death weighs half.
    cellDeathHalfLifeMin = 30.0
    // cellProximityScale is the distance at which a cell pays half
    // its score: the walk to the ground is dead time. The neighbor
    // rotation keeps the real walks short; the discount only guards
    // the rare far relocation.
    cellProximityScale = 8000.0
    // cellUnripeDiscount is the score fraction a cell pays while its
    // respawn window has not passed since the hunter cleared it: the
    // ground holds nothing pickable until it ripens.
    cellUnripeDiscount = 0.25
)

// cellMetric accumulates the session experience of one cell.
type cellMetric struct {
    // activeMin is the total hunting time spent at the cell
    // (minutes, rest included, town trips excluded).
    activeMin float64
    // kills counts the kills of the cell ground.
    kills int
    // adena is the looted adena attributed to the cell.
    adena int64
    // deaths is the decayed death heat of the cell: every death adds
    // one, the heat halves every half-life - a cell that killed the
    // character recently reads hot, an old grudge fades.
    deaths float64
    // deathCount is the raw session death count of the cell (the
    // map view label).
    deathCount int32
    // lastDeathAt paces the decay of the heat.
    lastDeathAt time.Time
    // killX and killY track the EMA of the kill positions: the live
    // mass centroid of the ground as the character experiences it
    // (the walk-to-cell destination and the map view label).
    killX float64
    killY float64
    // killPosKnown reports whether at least one kill anchored the
    // EMA yet.
    killPosKnown bool
    // visitActiveMin, visitAdena and visitKills carry the running
    // stay at the cell: the live rate of the current visit folds
    // into the totals when the stay ends.
    visitActiveMin float64
    visitAdena     int64
    visitKills     int
}

// adenaPerMin returns the measured income rate of the cell: the
// current visit's live rate when the visit runs, the historical
// average otherwise. Zero before the first minute of experience.
func (m *cellMetric) adenaPerMin() float64 {
    if m.visitActiveMin > 0.5 {
        return float64(m.visitAdena) / m.visitActiveMin
    }
    if m.activeMin > 0.5 {
        return float64(m.adena) / m.activeMin
    }

    return 0
}

// trusted reports whether the measured rate has enough experience to
// replace the bootstrap prior: the trust horizon of active minutes at
// the ground.
func (m *cellMetric) trusted() bool {
    return m.activeMin+m.visitActiveMin >= cellMetricsTrustMin
}

// cellHub shares the cell occupancy of the fleet: every loop of the
// process publishes the cell it hunts, the picker divides the score
// of a cell by its occupancy so the swarm spreads over the grounds
// of one quality instead of stacking on the nearest one. The
// neighbor rotation plus the occupancy division keeps the bots of a
// fleet on DIFFERENT neighborhoods of the mesh: two bots never clear
// the same cells in lockstep, so the ripeness clock of a cell is the
// clock of its owning bot, not a race.
type cellHub struct {
    mu      sync.Mutex
    hunters map[string]int
}

// globalCellHub is the process wide cell occupancy registry: all bot
// sessions of the swarm share it.
var globalCellHub = &cellHub{ //nolint:exhaustruct_v5 // mu is zero
    hunters: make(map[string]int),
}

// enter claims the cell for a hunter: the occupancy of the cell grows
// by one. Returns the leave function that releases the claim.
func (h *cellHub) enter(cellID string) func() {
    h.mu.Lock()
    h.hunters[cellID]++
    h.mu.Unlock()

    return func() {
        h.mu.Lock()
        if h.hunters[cellID] > 0 {
            h.hunters[cellID]--
        }
        h.mu.Unlock()
    }
}

// occupancy returns the number of hunters holding the cell.
func (h *cellHub) occupancy(cellID string) int {
    h.mu.Lock()
    count := h.hunters[cellID]
    h.mu.Unlock()

    return count
}

// cellScore returns the economic score of a cell for the character:
// the income value (the measured adena per minute once trusted, the
// bootstrap prior from the window mass and the respawn turnover
// otherwise) times the safety multiplier (the static aggressive share
// and the measured death heat) times the proximity discount (the walk
// to the ground is dead time) times the ripeness discount (a cell the
// hunter just cleared holds nothing until its respawn lands) divided
// by the fleet occupancy (the shared cells feed several mouths).
func (h *cellHunter) cellScore(index int, now time.Time) float64 {
    cell := &h.cells[index]
    metric := &h.metrics[index]
    value := h.cellValue(cell, metric)
    safety := h.cellSafety(cell, metric, now)
    proximity := h.cellProximity(cell)
    ripeness := h.ripenessFactor(index, now)
    occupancy := float64(1 + h.hub.occupancy(cell.ID))

    return value * safety * proximity * ripeness / occupancy
}

// cellValue returns the income value of the cell: the measured rate
// of the session once the ground earned the trust horizon, the
// bootstrap prior before that. The prior models the expected kills
// per minute as the smaller of the window supply turnover (window
// mass / respawn window) and the kill capacity of a solo farmer, and
// prices each kill by the mob level (the C1 elven drop curve rises
// with the level; the measured rate replaces the curve as soon as
// the real loot starts flowing). A trusted rate is the honest income
// of THIS character on THIS ground - including the zero: a starved
// ground must not promise its prior forever.
func (h *cellHunter) cellValue(cell *Cell, metric *cellMetric) float64 {
    if metric.trusted() {
        return metric.adenaPerMin()
    }
    windowMass := cellWindowMass(*cell, h.level)
    if windowMass <= 0 {
        return 0
    }
    respawnMid := float64(cell.RespawnMin+cell.RespawnMax) / 2.0
    if respawnMid < 1 {
        respawnMid = 1
    }
    supply := windowMass * 60.0 / respawnMid
    killsPerMin := math.Min(supply, cellKillCapacity)
    // The count weighted window level prices the kills.
    levelMass := 0.0
    levelWeight := 0.0
    low := h.level - 5
    if low < 1 {
        low = 1
    }
    for index := range cell.Mobs {
        mob := &cell.Mobs[index]
        if mob.Level >= low && mob.Level <= h.level {
            levelMass += float64(mob.Level) * float64(mob.Count)
            levelWeight += float64(mob.Count)
        }
    }
    if levelWeight <= 0 {
        return 0
    }
    windowLevel := levelMass / levelWeight
    adenaPerKill := cellAdenaBase + cellAdenaPerLevel*windowLevel

    return killsPerMin * adenaPerKill
}

// cellSafety returns the safety multiplier of the cell: the static
// aggressive share of the ground discounts it a little (an aggressive
// mob answers the pull before the character is prepared), the
// measured death heat dominates once the ground drew blood - one
// death per hour already costs a third of the score. This replaces
// the MinGear gate: an undergeared character dies, the deaths heat
// the cell, the picker moves it away.
func (h *cellHunter) cellSafety(
    cell *Cell, metric *cellMetric, now time.Time,
) float64 {
    var mass float64
    for index := range cell.Mobs {
        mass += float64(cell.Mobs[index].Count)
    }
    aggro := cellAggroMass(*cell)
    static := 1.0 - cellAggroPenalty*aggro/math.Max(mass, 1.0)
    heat := h.deathHeat(metric, now)
    dynamic := 1.0 / (1.0 + cellDeathHeatWeight*heat)

    return static * dynamic
}

// cellProximity returns the distance discount of the cell: the walk
// to the ground is dead time, a cell 8000 units away pays half. The
// discount anchors at the live character position when known, the
// current cell focus otherwise (a re-score between the fights never
// sees a stale far position).
func (h *cellHunter) cellProximity(cell *Cell) float64 {
    x, y := int32(0), int32(0)
    if h.selfKnown {
        x, y = h.selfX, h.selfY
    } else if h.picked >= 0 && h.picked < len(h.cells) {
        x, y = h.cells[h.picked].FocusX, h.cells[h.picked].FocusY
    }
    dist := cellDistance(*cell, x, y)

    return 1.0 / (1.0 + dist/cellProximityScale)
}

// deathHeat decays the death heat of a metric to the given time: the
// heat halves every half-life since the last death, so an hour old
// death weighs half. The rate feeds the safety multiplier.
func (h *cellHunter) deathHeat(metric *cellMetric, now time.Time) float64 {
    if metric.deaths <= 0 || metric.lastDeathAt.IsZero() {
        return 0
    }
    elapsed := now.Sub(metric.lastDeathAt).Minutes()
    if elapsed <= 0 {
        elapsed = 0
    }

    return metric.deaths * math.Pow(0.5, elapsed/cellDeathHalfLifeMin)
}
