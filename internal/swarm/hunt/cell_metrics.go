// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "sync"
    "time"
)

// The empirical measurement layer of the cell hunting: per cell rolling
// statistics replace the gear points of the old zone gates. They measure what
// the gates tried to predict - the loot income of the ground for this character
// (adena per active minute, rest included, so hard fights internalize their
// cost) and the danger (a decayed death heat) - so an undergeared bot scores
// worse where it takes damage and drifts to easier cells without any MinGear
// numbers.

// The scoring constants of the cell economy, measured on live sessions.
const (
    // cellMetricsTrustMin is the hunt time a ground needs before the measured
    // rate wins.
    cellMetricsTrustMin = 5.0
    // cellKillCapacity is the kill rate a solo farmer sustains on white-green
    // mobs: a kill every five seconds including the pull walk; sparse grounds
    // stay below.
    cellKillCapacity = 12.0
    // cellAggroPenalty discounts a heavy aggressive ground (the aggressive
    // share of the mass pays this fraction).
    cellAggroPenalty = 0.35
    // cellDeathHeatWeight scales the decayed death heat inside the safety
    // multiplier.
    cellDeathHeatWeight = 2.0
    // cellDeathHalfLifeMin is the half-life of the death heat: an hour old
    // death weighs a quarter.
    cellDeathHalfLifeMin = 30.0
    // cellTripDwell and cellTripSpeed price the recurring town trip of a cell:
    // one filled bag buys about 1200 seconds of hunting, and the walk runs at
    // the unbuffed elven fighter speed (125 units per second). The round trip
    // repeats at every fill, so a far ground pays its distance every cycle.
    cellTripDwell = 1200.0
    cellTripSpeed = 125.0
    // cellUnripeDiscount is the score fraction of a cell whose respawn window
    // has not passed since the hunter cleared it.
    cellUnripeDiscount = 0.25
    // cellDangerPenalty discounts a ground an emergency logout fired on: the
    // pack that drove the session out still holds the spot, so any alternative
    // outranks it. The window matches the death regression.
    cellDangerPenalty = 0.2
    // cellDangerWindow is how long an emergency logout keeps its ground
    // demoted: fifteen minutes reopen the spot, matching the death
    // regression.
    cellDangerWindow = cellDeathRegressionWindow
)

// cellMetric accumulates the session experience of one cell.
type cellMetric struct {
    // activeMin is the hunting time at the cell (rest included, town trips
    // excluded).
    activeMin float64
    // kills counts the kills of the cell ground.
    kills int
    // adena is the looted adena attributed to the cell.
    adena int64
    // deaths is the decayed death heat: every death adds one, the heat halves
    // every half-life, so a recent kill reads hot and an old grudge fades.
    deaths float64
    // deathCount is the raw session death count (the map view label).
    deathCount int32
    // lastDeathAt paces the decay of the heat.
    lastDeathAt time.Time
    // dangerAt is the last emergency logout on this ground: the picker demotes
    // the cell for cellDangerWindow (see cellDangerHeld).
    dangerAt time.Time
    // killX and killY track the EMA of the kill positions, the live mass
    // centroid: the walk-to-cell destination and the map view label.
    killX float64
    killY float64
    // killPosKnown reports whether a kill anchored the EMA yet.
    killPosKnown bool
    // visitActiveMin, visitAdena and visitKills carry the running stay; it
    // folds into the totals when the stay ends.
    visitActiveMin float64
    visitAdena     int64
    visitKills     int
}

// adenaPerMin returns the measured income rate: the live rate of the running
// visit, the historical average otherwise; zero before the first minute of
// experience.
func (m *cellMetric) adenaPerMin() float64 {
    if m.visitActiveMin > 0.5 {
        return float64(m.visitAdena) / m.visitActiveMin
    }
    if m.activeMin > 0.5 {
        return float64(m.adena) / m.activeMin
    }

    return 0
}

func (m *cellMetric) trusted() bool {
    return m.activeMin+m.visitActiveMin >= cellMetricsTrustMin
}

// cellHub shares the cell occupancy of the fleet: every loop publishes the cell
// it hunts and the picker divides the score by the occupancy, so the swarm
// spreads over the grounds of one quality instead of stacking on the nearest.
// The neighbor rotation plus the division keeps the bots on different
// neighborhoods: two bots never clear the same cells in lockstep, so a cell
// ripeness clock belongs to its owning bot, not a race.
type cellHub struct {
    mu      sync.Mutex
    hunters map[string]int
}

// globalCellHub is the process wide cell occupancy registry of the swarm.
var globalCellHub = &cellHub{ //nolint:exhaustruct_v5 // mu is zero
    hunters: make(map[string]int),
}

// enter claims the cell for a hunter and returns the function releasing the
// claim.
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

func (h *cellHub) occupancy(cellID string) int {
    h.mu.Lock()
    count := h.hunters[cellID]
    h.mu.Unlock()

    return count
}

// cellScore returns the economic score of a cell: the income value times the
// safety multiplier (the aggressive share and the death heat) times the
// relocation cost, the recurring village trip drag and the ripeness
// discount, divided by the fleet occupancy.
func (h *cellHunter) cellScore(index int, now time.Time) float64 {
    cell := &h.cells[index]
    metric := &h.metrics[index]
    value := h.cellValue(cell, metric)
    safety := h.cellSafety(cell, metric, now)
    proximity := h.cellProximity(cell)
    village := h.cellVillageDrag(cell)
    ripeness := h.ripenessFactor(index, now)
    danger := h.cellDangerFactor(metric, now)
    occupancy := float64(1 + h.hub.occupancy(cell.ID))

    return value * safety * proximity * village * ripeness * danger / occupancy
}

// cellDangerFactor discounts a ground an emergency logout fired on: the pack
// that drove the session out still holds the spot, so the next session farms
// anywhere else first (the observed storms repeated the same ground within a
// minute). The factor lapses after cellDangerWindow.
func (h *cellHunter) cellDangerFactor(
    metric *cellMetric, now time.Time,
) float64 {
    if h.cellDangerHeld(metric, now) {
        return cellDangerPenalty
    }

    return 1.0
}

// cellDangerHeld reports whether the ground carries a fresh emergency logout.
func (h *cellHunter) cellDangerHeld(
    metric *cellMetric, now time.Time,
) bool {
    return !metric.dangerAt.IsZero() && now.Sub(metric.dangerAt) < cellDangerWindow
}

// noteDangerAt marks the ground holding the spot as dangerous and reports
// whether a cell owns it; the picker demotes and the resume refuses it.
func (h *cellHunter) noteDangerAt(x int32, y int32, at time.Time) bool {
    for index := range h.leashes {
        if !h.leashes[index].Contains(x, y) {
            continue
        }
        h.metrics[index].dangerAt = at

        return true
    }

    return false
}

// cellValue returns the income value: the measured rate once the ground earned
// the trust horizon, the prior before that. The prior prices one kill by the
// ground economy - the mean adena of the window after the level gap scale and
// the kill rate the fight plus the loot and risk overhead sustain at the
// measured damage rate - so a higher band pays only for its longer fight. The
// flat capacity bounds a ground before the meter warmed up. A trusted rate is
// the honest income of this character on this ground, the zero included: a
// starved ground must not promise its prior forever.
func (h *cellHunter) cellValue(cell *Cell, metric *cellMetric) float64 {
    if metric.trusted() {
        return metric.adenaPerMin()
    }
    economy, ok := cellWindowEconomy(*cell, h.level)
    if !ok {
        return 0
    }
    respawnMid := float64(cell.RespawnMin+cell.RespawnMax) / 2.0
    if respawnMid < 1 {
        respawnMid = 1
    }
    supply := economy.mass * 60.0 / respawnMid

    return math.Min(supply, groundKillCapacity(economy)) * economy.adena
}

// cellSafety returns the safety multiplier: the static aggressive share
// discounts it a little (an aggressive mob answers the pull before the
// character is prepared) and the measured death heat dominates once the ground
// drew blood. This replaces the MinGear gate: an undergeared character dies,
// the deaths heat the cell, the picker moves away.
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

// cellProximity returns the relocation discount: the one-time walk to the cell
// amortized over the dwell the ground buys before the bag fills. A cell 20000
// units away pays about 160 seconds of a 1200 second stay; the recurring town
// trip repeats every cycle and belongs to cellVillageDrag. It anchors at the
// live character position, the current cell focus otherwise.
func (h *cellHunter) cellProximity(cell *Cell) float64 {
    x, y := int32(0), int32(0)
    if h.selfKnown {
        x, y = h.selfX, h.selfY
    } else if h.picked >= 0 && h.picked < len(h.cells) {
        x, y = h.cells[h.picked].FocusX, h.cells[h.picked].FocusY
    }
    dist := cellDistance(*cell, x, y)

    return cellTripDwell / (cellTripDwell + dist/cellTripSpeed)
}

// cellVillageDrag discounts the recurring town trip: the walk to the
// village and back repeats at every inventory fill, so a far ground
// pays its distance on every cycle, not only on the relocation. The
// drag is the hunting share of the cycle: the dwell over the dwell
// plus the round trip. The live proximity above keeps pricing the
// one-time move.
func (h *cellHunter) cellVillageDrag(cell *Cell) float64 {
    if !h.villageKnown {
        return 1.0
    }
    dist := cellDistance(*cell, h.villageX, h.villageY)

    return cellTripDwell / (cellTripDwell + 2*dist/cellTripSpeed)
}

// deathHeat decays the heat to the given time, halving it every half-life (an
// hour old death weighs a quarter); the safety multiplier reads it.
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
