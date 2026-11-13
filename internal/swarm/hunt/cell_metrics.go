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
    // cellAdenaBase and cellAdenaPerLevel price one kill of the prior: the C1
    // elven drop curve rises with the mob level, the measured rate replaces it
    // once loot flows.
    cellAdenaBase     = 18.0
    cellAdenaPerLevel = 6.5
    // cellAggroPenalty discounts a heavy aggressive ground (the aggressive
    // share of the mass pays this fraction).
    cellAggroPenalty = 0.35
    // cellDeathHeatWeight scales the decayed death heat inside the safety
    // multiplier.
    cellDeathHeatWeight = 2.0
    // cellDeathHalfLifeMin is the half-life of the death heat: an hour old
    // death weighs a quarter.
    cellDeathHalfLifeMin = 30.0
    // cellProximityScale is the distance at which a cell pays half its score:
    // the walk is dead time. The neighbor rotation keeps the walks short, the
    // discount guards the rare far relocation.
    cellProximityScale = 8000.0
    // cellUnripeDiscount is the score fraction of a cell whose respawn window
    // has not passed since the hunter cleared it.
    cellUnripeDiscount = 0.25
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
// proximity discount times the ripeness discount, divided by the fleet
// occupancy.
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

// cellValue returns the income value: the measured rate once the ground earned
// the trust horizon, the prior before that. The prior models the kills per
// minute as the smaller of the window supply turnover (window mass / respawn
// window) and the solo kill capacity, priced by the mob level. A trusted rate
// is the honest income of this character on this ground, the zero included: a
// starved ground must not promise its prior forever.
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

// cellProximity returns the distance discount: a cell 8000 units away pays
// half. It anchors at the live character position, the current cell focus
// otherwise.
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
