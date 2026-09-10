// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The spot policy: the decision core of the spot-anchored hunting.
// The hunter holds ONE spot (the leash square keeps the character on
// the ground), reads the emptiness of the square through the level
// window of the engage, predicts the respawns of the kills it made
// and decides between waiting (the respawn comes back before any walk
// could reach an equivalent ground), drifting to the predicted
// respawn position (corpse camping) and switching grounds (an
// economic decision with a hysteresis margin - the alternative must
// be clearly better for a sustained time, or the current ground must
// be starved). The gear gates and the band ladder of the legacy zones
// are gone: the picker scores every eligible spot by the measured
// income and safety of the session (see spot_metrics.go).

// The decision constants of the spot policy.
const (
	// spotWaitPatience is the respawn window worth waiting out at
	// the anchor: the elven respawn runs 15-20 s per mob, so a
	// predicted respawn within this window beats every walk (the
	// median walk to the nearest ground of the old registry cost
	// ~14 s alone).
	spotWaitPatience = 20 * time.Second
	// spotNoDataPatience bounds the wait of a spot with NO respawn
	// predictions (a fresh session, kills of other hunters): the
	// window gets two chances before the economy takes over.
	spotNoDataPatience = 40 * time.Second
	// spotExhaustedPatience is the emptiness duration that marks
	// a spot as starved: nothing pickable for a full minute means
	// the ground cannot feed the character.
	spotExhaustedPatience = 60 * time.Second
	// spotMinStay bounds the voluntary economic switch: a better
	// spot only wins after the current ground got a fair trial.
	spotMinStay = 5 * time.Minute
	// spotExhaustedMinStay bounds the starved switch: a dead
	// ground may be left early, but not instantly (the emptiness
	// reading needs the patrol to settle first).
	spotExhaustedMinStay = 90 * time.Second
	// spotSwitchMargin is the hysteresis of the voluntary switch:
	// the alternative must score 25 percent above the current
	// ground, so the hunter never ping-pongs between comparable
	// spots.
	spotSwitchMargin = 1.25
	// spotWindowFloorSlack is the level distance below the
	// character that still pays full adena (mob level + 8, the C1
	// drop penalty edge): mobs below the floor never enter a
	// fight.
	spotWindowFloorSlack = 8
	// spotWaitWalkPeriod paces the drift toward a predicted
	// respawn position, spotWaitWalkMinDist suppresses it when the
	// character already stands on the respawn point.
	spotWaitWalkPeriod  = 2 * time.Second
	spotWaitWalkMinDist = 500.0
	// spotViewPeriod paces the map view republication: the
	// respawn ETA label ticks once per second.
	spotViewPeriod = time.Second
	// spotKillLogCap bounds the respawn overlay records, spotKillTTL
	// prunes the predictions the world already invalidated.
	spotKillLogCap = 96
	spotKillTTL    = 5 * time.Minute
	// spotMaxLevelSlack mirrors the engage ceiling: the hard
	// guard above the character level (targetMaxLevelSlack).
	spotMaxLevelSlack = targetMaxLevelSlack
)

// spotHunter is the spot mode state of one hunt loop: the picked
// ground, the respawn overlay of the kills, the per spot metrics of
// the session and the wait-or-switch bookkeeping.
type spotHunter struct {
	// spots is the registry of the deployment, picked the index of
	// the active spot (-1 before the first pick).
	spots  []Spot
	picked int
	// hub shares the spot occupancy with the fleet, leave releases
	// the claim of the current spot.
	hub   *spotHub
	leave func()
	// metrics carries the session experience per spot, aggro the
	// precomputed static danger share per spot.
	metrics []spotMetric
	aggro   []float64
	// enteredAt marks when the current stay began (the minimum
	// stay floors of the switch decisions).
	enteredAt time.Time
	// level is the character level the window state was built at.
	level int32
	// priority is the window biased engage map of the current
	// spot (mirrored into loop.zoneMobPriority).
	priority map[int32]int32
	// kills is the respawn overlay: the recent kill records with
	// their predicted respawn times.
	kills []killRecord
	// lastAdena and adenaKnown baseline the income attribution,
	// lastAccumAt paces the active time accumulation.
	lastAdena   int32
	adenaKnown  bool
	lastAccumAt time.Time
	// selfX, selfY and selfKnown cache the character position of
	// the tick (the proximity discount of the scores).
	selfX     int32
	selfY     int32
	selfKnown bool
	// emptySince arms the wait-or-move economy: the moment the
	// leash square last read empty through the level window.
	emptySince time.Time
	// lastWaitWalkAt paces the drift toward the predicted respawn
	// positions.
	lastWaitWalkAt time.Time
	// viewAt paces the map view republication.
	viewAt time.Time
	// userOverride holds the manual spot selection of the web UI
	// (index into the registry, -1 = automatic economy).
	userOverride int
}

// newSpotHunter creates the spot mode state for a registry.
func newSpotHunter(spots []Spot, hub *spotHub) *spotHunter {
	hunter := &spotHunter{
		spots:        spots,
		picked:       -1,
		hub:          hub,
		leave:        nil,
		userOverride: -1,
	}
	hunter.metrics = make([]spotMetric, len(spots))
	hunter.aggro = make([]float64, len(spots))
	for index := range spots {
		hunter.aggro[index] = spotAggroMass(spots[index])
	}

	return hunter
}

// SetHuntingSpots installs the spot registry of the deployment: the
// loop hunts the spot mode (the picker, the respawn aware pacing and
// the measured scoring replace the band ladder, the gear gates and
// the rotation of the legacy zones). The first pick happens on the
// next tick.
func (l *Loop) SetHuntingSpots(spots []Spot) {
	if len(spots) == 0 {
		return
	}
	l.spot = newSpotHunter(spots, globalSpotHub)
	// The legacy zone state stands down: the zone bookkeeping of
	// the loop (the picked id, the overrides, the death caps)
	// mirrors the spot state instead.
	l.zones = nil
	l.zonePickedID = ""
	l.zoneOverride = -1
	l.zoneDeaths = nil
	l.zoneDeathCap = -1
	l.zoneEmptySince = time.Time{}
	l.zoneEmptyUntil = nil
	l.zoneMobPriority = nil
}

// SetHuntingSpotRegion installs the spot registry of one region by
// name ("elven"): the deployment wiring of the future regions.
func (l *Loop) SetHuntingSpotRegion(region string) {
	switch region {
	case regionElven, "":
		l.SetHuntingSpots(ElvenHuntingSpots())
	default:
		l.logger.Printf("Hunt: no spot registry for region %q, "+
			"hunting without spots", region)
	}
}

// userSpotSelect applies the manual spot selection of the web UI: the
// index refers to the spot registry order. The override holds until
// the character outgrows the spot window (the automatic economy then
// re-picks - a manual ground the character cannot productively hunt
// anymore serves nobody).
func (l *Loop) userSpotSelect(index int32) {
	hunter := l.spot
	if hunter == nil {
		return
	}
	if index < 0 || int(index) >= len(hunter.spots) {
		l.logger.Printf("Hunt: spot index %d out of range", index)

		return
	}
	spot := hunter.spots[index]
	hunter.userOverride = int(index)
	l.stopForZoneSwitch()
	hunter.apply(l, int(index), time.Now())
	l.logger.Printf("Hunt: user selected the hunting spot %s", spot.Name)
}

// overrideExpired reports whether the manual spot selection lost its
// ground: the character outgrew the level window of the spot.
func (h *spotHunter) overrideExpired() bool {
	if h.userOverride < 0 || h.userOverride >= len(h.spots) {
		return false
	}

	return !spotEligible(h.spots[h.userOverride], h.level)
}

// spotEvaluate is the tick entry of the spot mode (the branch of
// maybeSwitchZone): it tracks the level changes, accumulates the
// metrics, republishes the map view and runs the wait-or-move
// economy between the fights.
func (l *Loop) spotEvaluate(now time.Time) {
	hunter := l.spot
	if hunter == nil {
		return
	}
	level := l.tracker.SelfLevel()
	if level > 0 && level != hunter.level {
		hunter.level = level
		if hunter.picked >= 0 {
			hunter.priority = spotMobPriorities(
				hunter.spots[hunter.picked], level)
			l.zoneMobPriority = hunter.priority
		}
		if hunter.overrideExpired() {
			spot := hunter.spots[hunter.userOverride]
			hunter.userOverride = -1
			l.logger.Printf("Hunt: outgrew the manual spot %s, "+
				"resuming the spot economy", spot.Name)
		}
	}
	if hunter.picked < 0 {
		if level <= 0 {
			// The character stats have not arrived yet: the
			// first pick waits for the level.
			return
		}
		if hunter.userOverride >= 0 && hunter.userOverride < len(hunter.spots) {
			hunter.apply(l, hunter.userOverride, now)
			l.logger.Printf("Hunt: level %d: anchoring the manual "+
				"spot %s", level,
				hunter.spots[hunter.userOverride].Name)

			return
		}
		if x, y, _, ok := l.tracker.SelfPosition(); ok {
			hunter.selfX, hunter.selfY, hunter.selfKnown = x, y, true
		}
		best, _ := hunter.pickBest(-1, now)
		if best >= 0 {
			hunter.apply(l, best, now)
			spot := hunter.spots[best]
			l.logger.Printf("Hunt: level %d: anchoring the spot "+
				"%s (levels %d-%d, %d mobs, respawn %d-%ds)",
				level, spot.Name, spot.MinLevel,
				spot.MaxLevel, int(spot.Mass),
				spot.RespawnMin, spot.RespawnMax)
		}

		return
	}
	hunter.readSelf(l)
	hunter.accumulate(l, now)
	hunter.publishView(l, now)
	hunter.waitOrMove(l, now)
}

// readSelf caches the character position of the tick.
func (h *spotHunter) readSelf(l *Loop) {
	if x, y, _, ok := l.tracker.SelfPosition(); ok {
		h.selfX, h.selfY, h.selfKnown = x, y, true
	}
}

// windowFloor returns the engage level floor of the spot mode: the
// character level minus the adena edge slack (zero while the level is
// unknown - the floor disables itself).
func (h *spotHunter) windowFloor() int32 {
	if h.level <= 0 {
		return 0
	}
	floor := h.level - spotWindowFloorSlack
	if floor < 1 {
		floor = 1
	}

	return floor
}

// pickBest resolves the best scoring eligible spot (the fallback
// ranks by the mob level distance when nothing passes the window).
// The exclude index keeps a ground out of the contest (the switch
// paths pick the best ALTERNATIVE, the initial pick excludes
// nothing).
func (h *spotHunter) pickBest(exclude int, now time.Time) (int, float64) {
	best := -1
	bestScore := 0.0
	for index := range h.spots {
		if index == exclude {
			continue
		}
		if !spotEligible(h.spots[index], h.level) {
			continue
		}
		if score := h.spotScore(index, now); score > bestScore {
			best, bestScore = index, score
		}
	}
	if best < 0 && exclude < 0 {
		// The level window holds no ground at all: the closest
		// mob level distance wins (a character above or below
		// every window still hunts something).
		bestGap := int32(1 << 30)
		for index := range h.spots {
			gap := spotLevelDistance(h.spots[index], h.level)
			if gap < bestGap {
				bestGap, best = gap, index
			}
		}

		return best, 0
	}

	return best, bestScore
}

// apply switches the hunter to a spot: the leash square of the loop
// and the tracker retarget, the fleet claim moves, the visit metrics
// of the old ground fold into the totals and the new visit begins.
func (h *spotHunter) apply(l *Loop, index int, now time.Time) {
	if h.picked >= 0 && h.picked < len(h.metrics) {
		h.foldVisit(h.picked)
	}
	if h.leave != nil {
		h.leave()
	}
	h.picked = index
	h.enteredAt = now
	spot := h.spots[index]
	h.leave = h.hub.enter(spot.ID)
	half := spot.leashHalf()
	l.zonePickedID = spot.ID
	l.zoneCX, l.zoneCY, l.zoneHalf = spot.AnchorX, spot.AnchorY, half
	l.farmX, l.farmY, l.farmZ = 0, 0, 0
	l.zoneEmptySince = time.Time{}
	h.emptySince = time.Time{}
	h.priority = spotMobPriorities(spot, h.level)
	l.zoneMobPriority = h.priority
	l.tracker.SetHuntingZone(spot.AnchorX, spot.AnchorY, half)
	// The income attribution re-baselines at the new ground.
	h.adenaKnown = false
	h.lastAccumAt = time.Time{}
	h.publishView(l, now)
}

// foldVisit closes the running stay of a spot: the live visit numbers
// fold into the totals of the ground.
func (h *spotHunter) foldVisit(index int) {
	metric := &h.metrics[index]
	metric.activeMin += metric.visitActiveMin
	metric.adena += metric.visitAdena
	metric.kills += metric.visitKills
	metric.visitActiveMin = 0
	metric.visitAdena = 0
	metric.visitKills = 0
}

// spotNoteKill records a kill of the overlay: the corpse position
// predicts where the mob returns (the server default respawns at the
// spawn point, EnableRandomMonsterSpawns = false keeps it near the
// death place), the respawn window of the species bounds WHEN. The
// record feeds the wait-or-move economy and the kill centroid EMA of
// the map view.
func (l *Loop) spotNoteKill(objectID int32, now time.Time) {
	h := l.spot
	if h == nil || h.picked < 0 {
		return
	}
	x, y, _, ok := l.tracker.ObjectPosition(objectID)
	if !ok {
		// The corpse vanished from the knownlist before the
		// record: it died at the character's feet.
		x, y = h.selfX, h.selfY
	}
	wire := l.tracker.ObjectTemplateID(objectID)
	rmin, rmax, found := spotMobRespawn(h.spots, wire)
	if !found {
		spot := h.spots[h.picked]
		rmin, rmax = spot.RespawnMin, spot.RespawnMax
	}
	mid := time.Duration(rmin+rmax) * time.Second / 2
	spot := h.spots[h.picked]
	h.kills = append(h.kills, killRecord{
		spotID: spot.ID, x: x, y: y, at: now,
		respawnAt: now.Add(mid),
	})
	h.pruneKills(now)
	if len(h.kills) > spotKillLogCap {
		h.kills = h.kills[len(h.kills)-spotKillLogCap:]
	}
	metric := &h.metrics[h.picked]
	metric.visitKills++
	if !metric.killPosKnown {
		metric.killX, metric.killY = float64(x), float64(y)
		metric.killPosKnown = true
	} else {
		metric.killX = metric.killX*0.8 + float64(x)*0.2
		metric.killY = metric.killY*0.8 + float64(y)*0.2
	}
}

// spotNoteDeath counts a hunting death against the current spot: the
// death heat rises (the safety score of the ground decays with it,
// per spot - never the whole band), and the economic re-pick fires
// right away: a fresh death usually means the ground outguns the
// character, the walk back to the village spawn should aim at an
// easier spot instead of the corpse ground.
func (l *Loop) spotNoteDeath(now time.Time) {
	h := l.spot
	if h == nil || h.picked < 0 {
		return
	}
	spot := h.spots[h.picked]
	metric := &h.metrics[h.picked]
	metric.deaths++
	metric.deathCount++
	metric.lastDeathAt = now
	l.logger.Printf("Hunt: death at the spot %s (heat %.1f)", spot.Name,
		metric.deaths)
	// The fresh heat crushed the score of the ground: re-pick when
	// any eligible alternative now outranks it.
	current := h.spotScore(h.picked, now)
	best, bestScore := h.pickBest(h.picked, now)
	if best >= 0 && best != h.picked && bestScore > current {
		h.foldVisit(h.picked)
		h.apply(l, best, now)
		l.logger.Printf("Hunt: regressing from %s to %s after the "+
			"death", spot.Name, h.spots[best].Name)
	}
	h.publishView(l, now)
}

// accumulate advances the visit metrics: the active time of the stay
// (town trips excluded - the walks to the vendor are not the ground's
// cost) and the adena delta of the inventory attributed as the income
// of the ground (rest included: hard fights internalize their own
// cost, the rate is the honest income per hunting minute).
func (h *spotHunter) accumulate(l *Loop, now time.Time) {
	if l.tripActive() {
		h.lastAccumAt = time.Time{}

		return
	}
	if h.lastAccumAt.IsZero() {
		h.lastAccumAt = now
		h.lastAdena = l.tracker.InventoryStats().Adena
		h.adenaKnown = true

		return
	}
	deltaMin := now.Sub(h.lastAccumAt).Minutes()
	if deltaMin <= 0 {
		return
	}
	h.lastAccumAt = now
	metric := &h.metrics[h.picked]
	metric.visitActiveMin += deltaMin
	adena := l.tracker.InventoryStats().Adena
	if h.adenaKnown {
		if gained := int64(adena - h.lastAdena); gained > 0 {
			metric.visitAdena += gained
		}
	}
	h.lastAdena = adena
	h.adenaKnown = true
}

// waitOrMove is the economy between the fights: a pickable mob inside
// the level window always wins (the engage takes it), a predicted
// respawn within the patience window beats every walk (the hunter
// drifts to the corpse position), and only a starved or clearly
// outscored ground is left. The guards mirror the legacy zone
// rotation: only a targetless, healthy, in-leash, central engage
// phase reads the emptiness at all - running fights, resting walks
// and town trips never rotate.
func (h *spotHunter) waitOrMove(l *Loop, now time.Time) { //nolint:cyclop,funlen
	if l.phase != phaseEngage || l.target != 0 || l.tripActive() ||
		l.tracker.SelfUnderAttack() || l.tracker.SelfSitting() ||
		!l.inZoneSelf() {
		h.emptySince = time.Time{}

		return
	}
	spot := h.spots[h.picked]
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	// The emptiness reading is only trustworthy while the character
	// stands central: the patrol walk brings it to the anchor
	// first, the far corners of the ground read empty from the
	// edges of the knownlist.
	if spotDistance(spot, selfX, selfY) > float64(spot.leashHalf()) {
		h.emptySince = time.Time{}

		return
	}
	if l.tracker.ZoneHasPickableWindowed(spot.zoneSquare(),
		h.windowFloor(), l.maxTargetLevel(), l.activeSkips(now)) {
		h.emptySince = time.Time{}

		return
	}
	if h.emptySince.IsZero() {
		h.emptySince = now

		return
	}
	waited := now.Sub(h.emptySince)
	eta, predX, predY, pending := h.earliestPendingRespawn(spot.ID, now)
	if pending && eta <= spotWaitPatience {
		// The respawn comes back before any walk could reach an
		// equivalent ground: hold the anchor, drift toward the
		// predicted corpse position (one paced leg).
		h.waitWalk(l, predX, predY, selfX, selfY, selfZ, now)

		return
	}
	if !pending && waited < spotNoDataPatience {
		// No kill records of the ground: give the respawn
		// window two chances before the economy decides.
		return
	}
	h.evaluateSwitch(l, &spot, now, waited, pending, eta)
}

// waitWalk drifts the waiting hunter toward a predicted respawn
// position: one paced short leg (the per second target search of the
// engage picks up any mob the leg comes past), suppressed while the
// character already stands on the respawn point.
func (h *spotHunter) waitWalk(
	l *Loop, x int32, y int32,
	selfX int32, selfY int32, selfZ int32, now time.Time,
) {
	dist := math.Hypot(float64(x-selfX), float64(y-selfY))
	if dist < spotWaitWalkMinDist {
		return
	}
	if !h.lastWaitWalkAt.IsZero() &&
		now.Sub(h.lastWaitWalkAt) < spotWaitWalkPeriod {
		return
	}
	h.lastWaitWalkAt = now
	moveX, moveY := x, y
	dx := float64(x - selfX)
	dy := float64(y - selfY)
	if dist > returnWalkLeg {
		frac := returnWalkLeg / dist
		moveX = int32(float64(selfX) + dx*frac)
		moveY = int32(float64(selfY) + dy*frac)
	}
	if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
		l.logger.Printf("Hunt: respawn drift walk failed: %v", err)
	}
}

// evaluateSwitch decides the ground change of an emptied spot: the
// starved path (nothing pickable for a full minute - the ground
// cannot feed the character, any eligible alternative wins after the
// short stay floor) and the economic path (the alternative scores
// beyond the hysteresis margin after the fair trial of the minimum
// stay). A hunter without any alternative keeps waiting - the ground
// refills on its own clock.
func (h *spotHunter) evaluateSwitch(
	l *Loop, spot *Spot, now time.Time,
	waited time.Duration, pending bool, eta time.Duration,
) {
	stayed := now.Sub(h.enteredAt)
	current := h.spotScore(h.picked, now)
	best, bestScore := h.pickBest(h.picked, now)
	if best < 0 {
		// Nothing eligible to switch to: keep waiting (the
		// patrol holds the anchor, the knownlist refreshes).
		return
	}
	starved := waited >= spotExhaustedPatience &&
		stayed >= spotExhaustedMinStay &&
		h.expectedSupply(spot.ID, now, time.Minute) == 0
	better := bestScore > current*spotSwitchMargin &&
		stayed >= spotMinStay
	if !starved && !better {
		return
	}
	if starved && !better {
		l.logger.Printf("Hunt: %s starved (waited %s, respawn eta "+
			"%s): moving to %s", spot.Name, waited.Round(
			time.Second), etaRound(pending, eta),
			h.spots[best].Name)
	} else {
		l.logger.Printf("Hunt: %s outscored (%.0f vs %.0f adena/min "+
			"score): switching to %s", spot.Name, bestScore,
			current, h.spots[best].Name)
	}
	h.apply(l, best, now)
}

// etaRound renders the pending respawn ETA for the logs (never for
// the starved message when no prediction exists).
func etaRound(pending bool, eta time.Duration) string {
	if !pending {
		return "unknown"
	}

	return eta.Round(time.Second).String()
}

// minTargetLevel returns the engage level floor of the loop: the spot
// window floor in the spot mode, zero (disabled) in the legacy zone
// mode.
func (l *Loop) minTargetLevel() int32 {
	if l.spot == nil {
		return 0
	}

	return l.spot.windowFloor()
}

// The respawn overlay of the spot hunting: the kill records with
// their predicted respawn times.

// killRecord is one kill of the overlay: the corpse position (the
// server default respawns the mob at its spawn point, the elven
// windows are short), the species respawn window midpoint as the
// predicted time.
type killRecord struct {
	spotID    string
	x         int32
	y         int32
	at        time.Time
	respawnAt time.Time
}

// pruneKills drops the overlay records the world already invalidated
// (the TTL passed long ago - the mob either respawned and died again
// or never came back where predicted).
func (h *spotHunter) pruneKills(now time.Time) {
	kept := h.kills[:0]
	for index := range h.kills {
		if now.Sub(h.kills[index].at) <= spotKillTTL {
			kept = append(kept, h.kills[index])
		}
	}
	h.kills = kept
}

// earliestPendingRespawn resolves the earliest UNEXPIRED prediction
// of a spot: the kill records whose respawn still lies in the future.
// The ETA, the predicted position, and whether any prediction exists
// at all.
func (h *spotHunter) earliestPendingRespawn(
	spotID string, now time.Time,
) (time.Duration, int32, int32, bool) {
	var bestETA time.Duration
	bestX, bestY := int32(0), int32(0)
	found := false
	for index := range h.kills {
		kill := &h.kills[index]
		if kill.spotID != spotID || !kill.respawnAt.After(now) {
			continue
		}
		eta := kill.respawnAt.Sub(now)
		if !found || eta < bestETA {
			bestETA, bestX, bestY, found = eta, kill.x, kill.y, true
		}
	}

	return bestETA, bestX, bestY, found
}

// expectedSupply counts the mob supply of a spot within a horizon:
// the pending respawn predictions that land inside it. The live
// pickable population is the emptiness reading itself (the caller
// reached here with an empty square), so the predictions are the only
// remaining supply.
func (h *spotHunter) expectedSupply(
	spotID string, now time.Time, horizon time.Duration,
) int {
	count := 0
	for index := range h.kills {
		kill := &h.kills[index]
		if kill.spotID != spotID || !kill.respawnAt.After(now) {
			continue
		}
		if kill.respawnAt.Sub(now) <= horizon {
			count++
		}
	}

	return count
}

// publishView pushes the spot registry with the live economy markers
// to the tracker snapshot: the map draws every spot as the circle of
// its radius (the size of the ground), highlights the active one and
// labels the respawn window, the measured income, the death heat and
// the next respawn prediction (the timers of the economy).
func (h *spotHunter) publishView(l *Loop, now time.Time) {
	if l.tracker == nil {
		return
	}
	if !h.viewAt.IsZero() && now.Sub(h.viewAt) < spotViewPeriod {
		return
	}
	h.viewAt = now
	h.pruneKills(now)
	views := make([]state.ZoneView, 0, len(h.spots))
	for index := range h.spots {
		spot := &h.spots[index]
		metric := &h.metrics[index]
		next := int32(-1)
		if eta, _, _, ok := h.earliestPendingRespawn(
			spot.ID, now); ok {
			next = int32(eta.Seconds())
		}
		view := state.ZoneView{
			ID: spot.ID, Name: spot.Name, Region: spot.Region,
			MinLevel: spot.MinLevel, MaxLevel: spot.MaxLevel,
			CX: spot.AnchorX, CY: spot.AnchorY,
			Half:           spot.leashHalf(),
			Active:         index == h.picked,
			Kind:           "spot",
			Radius:         spot.Radius,
			RespawnMinSec:  spot.RespawnMin,
			RespawnMaxSec:  spot.RespawnMax,
			SpawnMass:      int32(spot.Mass),
			AggroMass:      int32(h.aggro[index]),
			AdenaPerMin:    metric.adenaPerMin(),
			DeathHeat:      h.deathHeat(metric, now),
			NextRespawnSec: next,
			Occupancy:      int32(h.hub.occupancy(spot.ID)),
		}
		if metric.deathCount > 0 {
			view.Deaths = metric.deathCount
		}
		if metric.killPosKnown {
			view.KillX = int32(metric.killX)
			view.KillY = int32(metric.killY)
		}
		views = append(views, view)
	}
	l.tracker.SetHuntingZones(views)

	// The kill ring rides along: the positions of the recent kills
	// feed the fleet wide cross layer of the map (every kill of every
	// bot, independent of the observed bot - see Bot.SetKillMarks).
	marks := make([]state.KillMarkView, 0, len(h.kills))
	for index := range h.kills {
		kill := &h.kills[index]
		marks = append(marks, state.KillMarkView{
			X: kill.x, Y: kill.y, AtMs: kill.at.UnixMilli(),
		})
	}
	l.tracker.SetKillMarks(marks)
}
