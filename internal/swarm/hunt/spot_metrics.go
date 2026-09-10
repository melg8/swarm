// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"sync"
	"time"
)

// The empirical measurement layer of the spot hunting: per spot
// rolling statistics of the session replace the gear points of the
// old zone gates. The metrics measure what the gates tried to
// predict: the loot income of the ground for THIS character (adena
// per active minute, rest included - hard fights internalize their
// own cost) and the danger of the ground (a decayed death heat).
// An undergeared bot automatically scores worse where it takes
// damage and drifts to easier spots without any MinGear numbers.

// The scoring constants of the spot economy.
const (
	// spotMetricsTrustMin is the active hunting time a ground
	// needs before the measured rate replaces the bootstrap
	// prior.
	spotMetricsTrustMin = 5.0
	// spotKillCapacity is the kill rate a solo farmer sustains on
	// white-green mobs (a kill every five seconds incl. the pull
	// walk); the supply turnover bounds it below on sparse
	// grounds.
	spotKillCapacity = 12.0
	// spotAdenaBase and spotAdenaPerLevel price one kill of the
	// bootstrap prior: the C1 elven drop curve rises with the mob
	// level, the measured rate replaces the curve as soon as the
	// real loot flows.
	spotAdenaBase     = 18.0
	spotAdenaPerLevel = 6.5
	// spotAggroPenalty is the static discount of an aggressive
	// heavy ground (the aggressive share of the mass pays this
	// fraction of the score).
	spotAggroPenalty = 0.35
	// spotDeathHeatWeight scales the decayed death heat inside the
	// safety multiplier: one death per hour costs a third of the
	// score.
	spotDeathHeatWeight = 2.0
	// spotDeathHalfLifeMin is the half-life of the death heat: an
	// hour old death weighs half. The old band demotion lasted
	// "until the level changes", the heat decays on its own.
	spotDeathHalfLifeMin = 30.0
	// spotProximityScale is the distance at which a spot pays half
	// its score: the walk to the ground is dead time.
	spotProximityScale = 8000.0
)

// spotMetric accumulates the session experience of one spot.
type spotMetric struct {
	// visits counts the completed hunting stays at the spot.
	visits int
	// activeMin is the total hunting time spent at the spot
	// (minutes, rest included, town trips excluded).
	activeMin float64
	// kills counts the kills of the spot ground.
	kills int
	// adena is the looted adena attributed to the spot.
	adena int64
	// deaths is the decayed death heat of the spot: every death
	// adds one, the heat halves every half-life - a spot that
	// killed the character recently reads hot, an old grudge
	// fades. Replaces the per band death cap of the zones.
	deaths float64
	// deathCount is the raw session death count of the spot (the
	// map view label).
	deathCount int32
	// lastDeathAt paces the decay of the heat.
	lastDeathAt time.Time
	// killX and killY track the EMA of the kill positions: the
	// live mass centroid of the ground as the character
	// experiences it (the map view draws it as the kill cloud).
	killX float64
	killY float64
	// killPosKnown reports whether at least one kill anchored the
	// EMA yet.
	killPosKnown bool
	// visitActiveMin, visitAdena and visitKills carry the running
	// stay at the spot: the live rate of the current visit folds
	// into the totals when the stay ends.
	visitActiveMin float64
	visitAdena     int64
	visitKills     int
}

// adenaPerMin returns the measured income rate of the spot: the
// current visit's live rate when the visit runs, the historical
// average otherwise. Zero before the first minute of experience.
func (m *spotMetric) adenaPerMin() float64 {
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
func (m *spotMetric) trusted() bool {
	return m.activeMin+m.visitActiveMin >= spotMetricsTrustMin
}

// spotHub shares the spot occupancy of the fleet: every loop of the
// process publishes the spot it hunts, the picker divides the score
// of a spot by its occupancy so the swarm spreads over the spots of
// one quality instead of stacking on the nearest one (the cheap
// version of the shared world model: the respawn predictions of
// every bot enrich the same registry in the long term).
type spotHub struct {
	mu      sync.Mutex
	hunters map[string]int
}

// globalSpotHub is the process wide spot occupancy registry: all bot
// sessions of the swarm share it.
var globalSpotHub = &spotHub{hunters: make(map[string]int)}

// enter claims the spot for a hunter: the occupancy of the spot
// grows by one. Returns the leave function that releases the claim.
func (h *spotHub) enter(spotID string) func() {
	h.mu.Lock()
	h.hunters[spotID]++
	h.mu.Unlock()

	return func() {
		h.mu.Lock()
		if h.hunters[spotID] > 0 {
			h.hunters[spotID]--
		}
		h.mu.Unlock()
	}
}

// occupancy returns the number of hunters holding the spot.
func (h *spotHub) occupancy(spotID string) int {
	h.mu.Lock()
	count := h.hunters[spotID]
	h.mu.Unlock()

	return count
}

// spotScore returns the economic score of a spot for the character:
// the income value (the measured adena per minute once trusted, the
// bootstrap prior from the window mass and the respawn turnover
// otherwise) times the safety multiplier (the static aggressive
// share and the measured death heat) times the proximity discount
// (the walk to the ground is dead time) divided by the fleet
// occupancy (the shared spots feed several mouths).
func (h *spotHunter) spotScore(index int, now time.Time) float64 {
	spot := &h.spots[index]
	metric := &h.metrics[index]
	value := h.spotValue(spot, metric)
	safety := h.spotSafety(spot, metric, now)
	proximity := h.spotProximity(spot)
	occupancy := float64(1 + h.hub.occupancy(spot.ID))

	return value * safety * proximity / occupancy
}

// spotValue returns the income value of the spot: the measured rate
// of the session once the ground earned the trust horizon, the
// bootstrap prior before that. The prior models the expected kills
// per minute as the smaller of the window supply turnover (window
// mass / respawn window) and the kill capacity of a solo farmer, and
// prices each kill by the mob level (the C1 elven drop curve rises
// with the level; the measured rate replaces the curve as soon as
// the real loot starts flowing).
func (h *spotHunter) spotValue(spot *Spot, metric *spotMetric) float64 {
	if metric.trusted() {
		if rate := metric.adenaPerMin(); rate > 0 {
			return rate
		}
	}
	windowMass := spotWindowMass(*spot, h.level)
	if windowMass <= 0 {
		return 0
	}
	respawnMid := float64(spot.RespawnMin+spot.RespawnMax) / 2.0
	if respawnMid < 1 {
		respawnMid = 1
	}
	supply := windowMass * 60.0 / respawnMid
	killsPerMin := math.Min(supply, spotKillCapacity)
	// The count weighted window level prices the kills.
	levelMass := 0.0
	levelWeight := 0.0
	low := h.level - 5
	if low < 1 {
		low = 1
	}
	for index := range spot.Mobs {
		mob := &spot.Mobs[index]
		if mob.Level >= low && mob.Level <= h.level {
			levelMass += float64(mob.Level) * float64(mob.Count)
			levelWeight += float64(mob.Count)
		}
	}
	if levelWeight <= 0 {
		return 0
	}
	windowLevel := levelMass / levelWeight
	adenaPerKill := spotAdenaBase + spotAdenaPerLevel*windowLevel

	return killsPerMin * adenaPerKill
}

// spotSafety returns the safety multiplier of the spot: the static
// aggressive share of the ground discounts it a little (an
// aggressive mob answers the pull before the character is ready),
// the measured death heat dominates once the ground drew blood - one
// death per hour already costs a third of the score. This replaces
// the MinGear gate: an undergeared character dies, the deaths heat
// the spot, the picker moves it away.
func (h *spotHunter) spotSafety(spot *Spot, metric *spotMetric, now time.Time) float64 {
	var mass float64
	for index := range spot.Mobs {
		mass += float64(spot.Mobs[index].Count)
	}
	aggro := spotAggroMass(*spot)
	static := 1.0 - spotAggroPenalty*aggro/math.Max(mass, 1.0)
	heat := h.deathHeat(metric, now)
	dynamic := 1.0 / (1.0 + spotDeathHeatWeight*heat)

	return static * dynamic
}

// spotProximity returns the distance discount of the spot: the walk
// to the ground is dead time, a spot 8000 units away pays half. The
// discount anchors at the live character position when known, the
// current spot anchor otherwise (a re-score between the fights never
// sees a stale far position).
func (h *spotHunter) spotProximity(spot *Spot) float64 {
	x, y := int32(0), int32(0)
	if h.selfKnown {
		x, y = h.selfX, h.selfY
	} else if h.picked >= 0 && h.picked < len(h.spots) {
		x, y = h.spots[h.picked].AnchorX, h.spots[h.picked].AnchorY
	}
	dist := spotDistance(*spot, x, y)

	return 1.0 / (1.0 + dist/spotProximityScale)
}

// deathHeat decays the death heat of a metric to the given time: the
// heat halves every half-life since the last death, so an hour old
// death weighs half. The rate feeds the safety multiplier.
func (h *spotHunter) deathHeat(metric *spotMetric, now time.Time) float64 {
	if metric.deaths <= 0 || metric.lastDeathAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(metric.lastDeathAt).Minutes()
	if elapsed <= 0 {
		elapsed = 0
	}

	return metric.deaths * math.Pow(0.5, elapsed/spotDeathHalfLifeMin)
}
