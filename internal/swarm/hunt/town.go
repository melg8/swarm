// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Town trips of the hunt loop: when the inventory runs full, the
// character walks to the nearest town shop over the geodata (Navigator),
// sells the junk to the merchant and walks back to the farm spot. The
// sell request uses the standard inventory sell list of the official
// client: the Mobius server prices every item itself at
// referencePrice/2 and refuses nothing else, so no shop window flow is
// needed - only the merchant interaction distance has to be respected.
package hunt

import (
	"math"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
)

// npcDisplayOffset mirrors the display id offset the Mobius server adds
// to the npc id of every NpcInfo packet (AbstractNpcInfo writeImpl).
const npcDisplayOffset = 1000000

// Timing and threshold constants of the town trips.
const (
	// tripSlotPercent triggers a town trip at this inventory fill level.
	tripSlotPercent = 50.0
	// tripWeightPercent triggers a town trip at this weight level.
	tripWeightPercent = 50.0
	// sellBatchSize is the maximum item count of one sell request.
	sellBatchSize = 25
	// sellPause paces the sell requests after the transaction flood
	// protector of the server (10 seconds by default).
	sellPause = 11 * time.Second
	// walkRequestPeriod paces the ground click walks of the waypoint
	// follower and the merchant approach.
	walkRequestPeriod = 2 * time.Second
	// waypointArriveDist is the distance within which a waypoint counts
	// as reached: two geodata cells of slack.
	waypointArriveDist = 150.0
	// maxMoveLeg splits the walk legs: the server refuses move requests
	// with a target farther than 9900 units (MoveToLocation readImpl),
	// and the smoothed geodata paths happily produce longer legs over
	// the open terrain.
	maxMoveLeg = 1000.0
	// stuckTimeout is how long the character may stand still on a leg
	// before the walker re-paths around the obstacle.
	stuckTimeout = 15 * time.Second
	// maxRePaths bounds the re-paths of one trip before it aborts.
	maxRePaths = 3
	// merchantApproachDist is the distance the seller stands from the
	// merchant: below the 250 units interaction distance of the server.
	merchantApproachDist = 200.0
	// tripApproachRadius is the geodata search radius the trip walks
	// end within: a merchant cell without a modeled floor layer (the
	// elven village shops) or behind a counter stays reachable, the
	// water deck below the shop - far in z - does not.
	tripApproachRadius = 200.0
	// merchantFindRadius is the radius around the character within
	// which the spawned merchant npc is looked up once the shop point
	// is reached.
	merchantFindRadius = 2000.0
	// merchantWaitTimeout bounds the wait for the merchant NpcInfo
	// before the sale starts without a selected merchant.
	merchantWaitTimeout = 45 * time.Second
	// tripCooldown pauses new town trips after one ended, so a trip
	// that cannot reach the shop does not restart every tick.
	tripCooldown = 5 * time.Minute
	// tripTimeout ends a trip that got stuck somewhere in between so
	// the bot resumes hunting.
	tripTimeout = 20 * time.Minute
	// merchantDeckWindow bounds the server routed re-walk onto a
	// merchant deck the geodata pack cannot reach (the village
	// ramps): the ground clicks retry until the window closes.
	merchantDeckWindow = 30 * time.Second
)

// townNpc is a town npc the trip machinery navigates to: a shop
// merchant of the sell trips or a guard of the deleveling.
type townNpc struct {
	TemplateID int32
	Name       string
	X          int32
	Y          int32
	Z          int32
}

// zeroTownNpc is the not-found sentinel of the merchant and guard
// searches.
var zeroTownNpc = townNpc{TemplateID: 0, Name: "", X: 0, Y: 0, Z: 0}

// townMerchants are the shop merchants of the known towns. Any merchant
// accepts the sale of any sellable item (the inventory sell list), so
// the bot simply walks to the nearest one; the list grows with the
// farming areas of the deployment. Coordinates from the Mobius C1
// spawn data (ElvenTerritory/ElvenVillageNPCs.xml). TemplateID is the
// client display id the NpcInfo packet carries: the C1 spawn ids of the
// traders (30147..30150) map to the CT0 display ids through
// CT0_to_C4_ids.txt of the npc stats.
var townMerchants = []townNpc{
	{TemplateID: 7147, Name: "Unoren", X: 44667, Y: 46896, Z: -2982},
	{TemplateID: 7148, Name: "Ariel", X: 44683, Y: 46952, Z: -2981},
	{TemplateID: 7149, Name: "Creamees", X: 42700, Y: 50057, Z: -2984},
	{TemplateID: 7150, Name: "Herbiel", X: 42766, Y: 50037, Z: -2984},
}

// Navigator plans walkable paths through the world geodata. The
// pathfind engine is wrapped into one through NewNavigator; tests fake
// the interface.
type Navigator interface {
	// FindPathApproach plans a walk that must end within the
	// approach radius (3D) of the target point: the merchant stops
	// use the interaction distance, the exact target is preferred
	// whenever it is reachable.
	FindPathApproach(start, end pathfind.Vec3, approachRadius float64) (
		*pathfind.Result, error,
	)
	// FindPath plans a walk to the target cell arriving on whatever
	// deck of it the walk reaches first.
	FindPath(start, end pathfind.Vec3) (*pathfind.Result, error)
	// ClosestHeight resolves the height of the layer at the world
	// position closest to refZ - the deck the server itself picks
	// for a destination named with that z. The zone return resolves
	// its goal height through it before the approach search.
	ClosestHeight(x, y float64, refZ int16) (int16, error)
	// LineOfSight reports whether the geodata holds a clear straight
	// line between two world positions: the blind engage recovery uses
	// it to find a standing point that sees the obstructed target.
	LineOfSight(start, end pathfind.Vec3) (bool, error)
}

// engineNavigator adapts a geodata engine to the Navigator interface,
// applying the configured maximum passable height of the engine.
type engineNavigator struct {
	engine *pathfind.Engine
}

// NewNavigator wraps a geodata engine into the town trip navigator.
// Returning the Navigator interface is the deliberate seam of the
// hunt package (see the AGENTS.md interface map).
func NewNavigator(engine *pathfind.Engine) Navigator { //nolint:ireturn
	return engineNavigator{engine: engine}
}

// FindPathApproach searches the walkable path with the engine settings
// and the approach radius goal.
func (e engineNavigator) FindPathApproach(
	start, end pathfind.Vec3, approachRadius float64,
) (*pathfind.Result, error) {
	return e.engine.FindPathApproach(
		start, end, approachRadius, e.engine.MaxPassableHeight())
}

// FindPath searches the walkable path with the engine settings.
func (e engineNavigator) FindPath(
	start, end pathfind.Vec3,
) (*pathfind.Result, error) {
	return e.engine.FindPath(start, end, e.engine.MaxPassableHeight())
}

// ClosestHeight resolves the destination deck height with the engine.
func (e engineNavigator) ClosestHeight(
	x, y float64, refZ int16,
) (int16, error) {
	return e.engine.ClosestHeight(x, y, refZ)
}

// LineOfSight answers the geodata sight line with the engine settings.
func (e engineNavigator) LineOfSight(
	start, end pathfind.Vec3,
) (bool, error) {
	return e.engine.LineOfSight(start, end, e.engine.MaxPassableHeight())
}

// nearestMerchant returns the town merchant closest to the point.
func (l *Loop) nearestMerchant(
	selfX int32, selfY int32,
) (townNpc, bool) {
	best := townNpc{
		TemplateID: 0,
		Name:       "",
		X:          0,
		Y:          0,
		Z:          0,
	}
	bestDist := math.MaxFloat64
	found := false
	for _, merchant := range townMerchants {
		dist := math.Hypot(
			float64(merchant.X-selfX), float64(merchant.Y-selfY))
		if dist < bestDist {
			bestDist = dist
			best = merchant
			found = true
		}
	}

	return best, found
}

// merchantTemplates lists the packet template ids of the town merchants.
func merchantTemplates() []int32 {
	templates := make([]int32, 0, len(townMerchants))
	for _, merchant := range townMerchants {
		templates = append(templates, merchant.TemplateID+npcDisplayOffset)
	}

	return templates
}

// tripActive reports whether a town trip is running.
func (l *Loop) tripActive() bool {
	switch l.phase {
	case phaseTownWalk, phaseTownSell, phaseTownReturn:
		return true
	default:
		return false
	}
}

// tripCooldownOver reports whether a new town trip may start.
func (l *Loop) tripCooldownOver() bool {
	return l.tripEndedAt.IsZero() ||
		time.Since(l.tripEndedAt) >= tripCooldown
}

// inventoryFull reports whether the inventory passed a trip trigger
// threshold: more than half of the slots used or more than half of the
// maximum weight carried. The selling phase reuses it as the stop
// condition: the trip returns once the inventory is back below it.
func (l *Loop) inventoryFull() bool {
	stats := l.tracker.InventoryStats()

	return stats.SlotPercent > tripSlotPercent ||
		stats.WeightPercent > tripWeightPercent
}

// maybeStartTownTrip begins a town trip when the inventory is full
// enough or the shop strategy has a plan worth a trip and the trip
// cooldown is over. Everything that can block the trip (no navigator,
// no geodata, no path) arms the cooldown, so a broken deployment does
// not retry every tick.
func (l *Loop) maybeStartTownTrip() {
	shopping := l.shoppingTripEnabled() && l.shoppingWanted()
	if l.navigator == nil || !l.tripCooldownOver() ||
		(!l.inventoryFull() && !shopping) {
		return
	}
	// The walk needs a standing character: a resting one stands up
	// first and the trip starts on a later tick.
	if !l.standUpGuarded(time.Now()) {
		return
	}
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	merchant, ok := l.nearestMerchant(selfX, selfY)
	if !ok {
		return
	}
	// The walk back target: the farm spot when the trip starts inside
	// the hunting zone, the zone center otherwise (a village respawn,
	// a chase that ran away).
	l.rememberFarmSpot()
	l.tripStart = time.Now()
	l.sold = make(map[int32]bool)
	l.rePaths = 0
	l.tripStops = []tripStop{{
		merchant: merchant,
		sell:     true,
		buys:     nil,
	}}
	l.buysPlanned = false
	l.buyAt = time.Time{}
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.resetReplacementSales()
	l.phase = phaseTownWalk
	stats := l.tracker.InventoryStats()
	reason := "inventory at " + strconv.Itoa(stats.Slots) + " slots and " +
		strconv.FormatFloat(stats.WeightPercent, 'f', 0, 64) +
		"% weight"
	if !l.inventoryFull() {
		reason = "the shop strategy plans purchases worth " +
			strconv.FormatInt(gear.AdenaSpent(
				affordablePrefix(l.shoppingPlanCache)), 10) +
			" adena"
	}
	// The trigger plan cache drops: the stop planning recomputes it
	// with the fresh adena of the sales.
	l.shoppingPlanCache = nil
	l.shoppingPlanAt = time.Time{}
	l.shoppingPlanAdena = 0
	l.logger.Printf("Hunt: %s, walking to the trader %s", reason,
		merchant.Name)
	if !l.startWalkLeg(townNpcPosition(merchant)) {
		l.abortTownTrip("no walkable path to the shop")
	}
}

// townNpcPosition returns the spawn point of the npc.
func townNpcPosition(npc townNpc) pathfind.Vec3 {
	return pathfind.Vec3{
		X: float64(npc.X),
		Y: float64(npc.Y),
		Z: float64(npc.Z),
	}
}

// rememberFarmSpot stores the walk home target of a trip: the position
// of the character when it starts inside the hunting zone, the zone
// center otherwise (a village respawn, a chase that ran away).
func (l *Loop) rememberFarmSpot() {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	if l.inZoneSelf() {
		l.farmX, l.farmY, l.farmZ = selfX, selfY, selfZ

		return
	}
	zone := l.zone()
	if zone == nil {
		l.farmX, l.farmY, l.farmZ = selfX, selfY, selfZ

		return
	}
	l.farmX, l.farmY, l.farmZ = zone.CX, zone.CY, selfZ
}

// walkToward sends a ground click walk to the point at most once per
// walk request period, sharing the move pacing of the trip machinery.
func (l *Loop) walkToward(x, y, z int32, now time.Time) {
	if !l.moveAt.IsZero() && now.Sub(l.moveAt) < walkRequestPeriod {
		return
	}
	l.moveAt = now
	if err := l.game.WalkTo(x, y, z); err != nil {
		l.logger.Printf("Hunt: walk request failed: %v", err)
	}
}

// tickTownTrip advances the running town trip by one decision.
func (l *Loop) tickTownTrip() {
	if time.Since(l.tripStart) > tripTimeout {
		l.abortTownTrip("trip timed out")

		return
	}
	switch l.phase {
	case phaseTownWalk:
		if l.walkTownWaypoints() {
			l.enterSellPhase()
		}
	case phaseTownSell:
		l.tickTownSell()
	case phaseTownReturn:
		// Entering the zone with a target in reach ends the walk:
		// the hunt answers whatever the entry radius offers
		// instead of marching to the center first.
		if l.engagesOnZoneEntry() {
			return
		}
		if l.walkTownWaypoints() {
			l.endTownTrip("back at the farm spot")
		}
	default:
		// The non-town phases never reach the town tick (the trip
		// trigger starts the walk phase first).
	}
}

// startWalkLeg plans the walk to the destination and arms the waypoint
// follower. The search goal is the approach radius of the destination
// (the merchant interaction distance): a destination behind a counter
// or on a floor layer the geodata does not model is still reached on
// the surrounding deck, and unreachable destinations leave a fallback
// direct walk the server routes itself (its own pathfinder reaches
// what the pack misses, proven by the death leash of the earlier
// sessions). It reports whether the leg was planned.
func (l *Loop) startWalkLeg(dest pathfind.Vec3) bool {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return false
	}
	from := pathfind.Vec3{
		X: float64(selfX),
		Y: float64(selfY),
		Z: float64(selfZ),
	}
	result, err := l.navigator.FindPathApproach(from, dest, tripApproachRadius)
	if err != nil {
		l.logger.Printf("Hunt: town trip path search failed: %v", err)

		return false
	}
	if result != nil && result.Found && len(result.Waypoints) > 0 {
		l.waypoints = result.Waypoints
	} else {
		l.logger.Printf("Hunt: no geodata path to %d %d, walking by "+
			"server routing", int(dest.X), int(dest.Y))
		l.waypoints = []pathfind.Vec3{dest}
	}
	l.wpIndex = 0
	l.legDest = dest
	l.moveAt = time.Time{}
	l.stuckAt = time.Time{}

	return true
}

// waypointDistance measures the distance from the character to a
// planned waypoint in full 3D. The arrival and skip decisions of the
// waypoint followers must use it: a 2D-only radius once marked a deck
// edge drop waypoint as reached - 80 units away horizontally but 920
// units below the character - and the follower jumped straight to the
// leg beyond the drop the character never walked, straight into the
// city railing. The geodata waypoints carry the real layer height,
// so the z axis is exact for them.
func waypointDistance(
	wp pathfind.Vec3, selfX, selfY, selfZ int32,
) float64 {
	dx := wp.X - float64(selfX)
	dy := wp.Y - float64(selfY)
	dz := wp.Z - float64(selfZ)

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// walkTownWaypoints follows the planned waypoints with ground click
// walks and returns true when the final waypoint is reached. Legs
// longer than the server move request limit are split into straight
// intermediate points (the smoothing guarantees the line of sight of
// every leg, so the intermediate points stay on the verified segment).
// Waypoints the character already passed are skipped: a server position
// correction or a restart jump can place the character ahead of the
// follower, and walking back to a passed waypoint would loop. A walk
// that stands still re-paths from the current position to the leg
// destination, bounded by the re-path budget of the trip.
func (l *Loop) walkTownWaypoints() bool {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return false
	}
	now := time.Now()
	for l.wpIndex < len(l.waypoints) {
		wp := l.waypoints[l.wpIndex]
		dist := waypointDistance(wp, selfX, selfY, selfZ)
		if dist > waypointArriveDist {
			// The waypoint is not reached yet: skip it when the
			// next one is closer - the character already passed
			// it (a jump, a server correction).
			if l.wpIndex+1 < len(l.waypoints) {
				next := l.waypoints[l.wpIndex+1]
				nextDist := waypointDistance(next, selfX, selfY, selfZ)
				if nextDist < dist {
					l.wpIndex++
					l.moveAt = time.Time{}

					continue
				}
			}

			break
		}
		l.wpIndex++
		l.moveAt = time.Time{}
	}
	if l.wpIndex >= len(l.waypoints) {
		return true
	}
	if l.walkStuck(now, selfX, selfY) {
		return false
	}
	if !l.moveAt.IsZero() && now.Sub(l.moveAt) < walkRequestPeriod {
		return false
	}
	wp := l.waypoints[l.wpIndex]
	dx := wp.X - float64(selfX)
	dy := wp.Y - float64(selfY)
	dist := math.Hypot(dx, dy)
	moveX, moveY, moveZ := wp.X, wp.Y, wp.Z
	if dist > maxMoveLeg {
		frac := maxMoveLeg / dist
		moveX = float64(selfX) + dx*frac
		moveY = float64(selfY) + dy*frac
		moveZ = float64(selfZ) + (wp.Z-float64(selfZ))*frac
	}
	l.moveAt = now
	if err := l.game.WalkTo(int32(moveX), int32(moveY), int32(moveZ)); err != nil {
		l.logger.Printf("Hunt: town walk request failed: %v", err)
	}

	return false
}

// walkStuck tracks the movement progress of the walker and re-paths
// around the obstacle once the character stands still for too long. It
// reports whether the trip had to abort.
func (l *Loop) walkStuck(now time.Time, selfX int32, selfY int32) bool {
	if l.stuckAt.IsZero() {
		l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY

		return false
	}
	if selfX != l.stuckX || selfY != l.stuckY {
		l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY

		return false
	}
	if now.Sub(l.stuckAt) < stuckTimeout {
		return false
	}
	l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
	l.rePaths++
	if l.rePaths > maxRePaths {
		l.abortTownTrip("walk stuck")

		return true
	}
	l.logger.Printf("Hunt: town walk stuck, re-pathing (%d of %d)",
		l.rePaths, maxRePaths)
	if !l.startWalkLeg(l.legDest) {
		l.abortTownTrip("re-path failed")

		return true
	}

	return false
}

// enterSellPhase switches into the selling and shopping state at the
// shop.
func (l *Loop) enterSellPhase() {
	l.phase = phaseTownSell
	l.sellPhaseAt = time.Now()
	l.sellAt = time.Time{}
	l.buyAt = time.Time{}
	l.merchantID = 0
	l.merchantPick = time.Time{}
	l.merchantDeckUntil = time.Time{}
	l.logger.Printf("Hunt: shop reached, selling the junk")
}

// tickTownSell runs the sell stop (the first trip stop) and the buy
// stops. EVERY vendor trip sells the whole accumulated junk - the
// selling ends when nothing sellable is left, not when the inventory
// drops below the trip trigger (a bag of 30 percent junk on a buy
// trip still sells, or the bot would farm with it and walk back for
// the sale later). The fresh adena of the sales re-plans the
// purchases, every buy stop completes when its purchases were
// requested, and the return leg starts when no stop is left.
func (l *Loop) tickTownSell() {
	now := time.Now()
	if l.sellableStop() {
		if l.junkRemaining() {
			if !l.handleMerchant(now, merchantTemplates()) {
				return
			}
			l.sellJunk()

			return
		}
		if !l.replaceDone {
			// The replacement purchases sell their displaced pieces
			// first: the credit the plan counted on must be banked
			// before the buys spend it.
			if !l.stepReplacementSales(now) {
				return
			}
			l.replaceDone = true
		}
		if !l.buysPlanned {
			stats := l.tracker.InventoryStats()
			l.logger.Printf("Hunt: shop: junk sold (%d slots left, "+
				"%.0f%% weight), planning the purchases", stats.Slots,
				stats.WeightPercent)
			l.planShoppingStops()
		}
	}
	if l.stopBuysPending() {
		if !l.handleMerchant(now, l.stopMerchantTemplates()) {
			return
		}
		// The buys need the selected merchant within the interaction
		// distance: without it the sells still work, the buys are
		// skipped.
		if l.merchantID > 0 {
			l.tickStopShopping(now)
		} else {
			l.resetStopBuys("no merchant in reach for the buys")
		}

		return
	}
	if len(l.tripStops) > 1 || (len(l.tripStops) == 1 &&
		!l.tripStops[0].sell && !l.stopBuysPending()) ||
		l.buysPlanned {
		l.advanceTripStop()

		return
	}
	l.startReturnLeg()
}

// handleMerchant approaches the shop merchant and selects it like the
// official client does before a transaction. It reports false while the
// character still walks toward the merchant or waits for one to appear.
// The sale itself works without a merchant (the standard inventory sell
// list), so a merchant that never shows up only delays it; the buys
// need the merchant, their stops skip the purchases instead.
func (l *Loop) handleMerchant(now time.Time, templates []int32) bool {
	if l.merchantID < 0 {
		return true
	}
	if l.merchantID > 0 {
		return l.approachMerchant(now)
	}
	if now.Sub(l.merchantPick) < selectPeriod {
		return false
	}
	l.merchantPick = now
	l.merchantDeckUntil = time.Time{}
	merchant, ok := l.tracker.NearestNpcByTemplates(
		templates, merchantFindRadius)
	if ok {
		l.merchantID = merchant.ObjectID
		l.logger.Printf("Hunt: trading with " + merchant.Name)

		return false
	}
	if now.Sub(l.sellPhaseAt) < merchantWaitTimeout {
		return false
	}
	l.merchantID = -1
	if l.stopBuysPending() {
		// The buys cannot run without the selected merchant: skip
		// them instead of waiting forever.
		l.resetStopBuys("merchant never showed up")
	}

	return true
}

// approachMerchant walks to the merchant, selects it inside the
// interaction distance and reports when the sale may start. The
// distance gate is 3D (the server INTERACTION_DISTANCE of 250 checks
// x, y and z together - separate 2D and z limits would let a diagonal
// stand-off slip past 250 and refuse every transaction). The selection
// re-requests itself once per second until the MyTargetSelected
// answer confirms it. A merchant standing on another deck of the
// geodata (the disconnected village decks) is skipped: the 3D
// interaction distance of the server can never be met and the sale
// does not need the merchant.
func (l *Loop) approachMerchant(now time.Time) bool {
	x, y, z, ok := l.tracker.ObjectPosition(l.merchantID)
	if !ok {
		l.merchantID = -1

		return true
	}
	selfX, selfY, selfZ, _ := l.tracker.SelfPosition()
	dist2D := math.Hypot(float64(x-selfX), float64(y-selfY))
	dz := float64(z - selfZ)
	dist3D := math.Sqrt(dist2D*dist2D + dz*dz)
	if dist3D > merchantApproachDist {
		if dist2D <= merchantApproachDist {
			// The geodata pack misses some village ramps: the character
			// stands under the merchant deck (the 2D distance is met,
			// the z is not). Ground clicks route through the server
			// pathfinder, which knows the ramps - click the exact
			// merchant position until the z matches or the retry window
			// closes.
			if l.merchantDeckUntil.IsZero() {
				l.merchantDeckUntil = now.Add(merchantDeckWindow)
				l.logger.Printf("Hunt: %s stands on another deck (z %d vs "+
					"%d), re-walking by server routing",
					l.tracker.ObjectName(l.merchantID), selfZ, z)
			}
			if now.Before(l.merchantDeckUntil) {
				l.walkToward(x, y, z, now)

				return false
			}
			l.logger.Printf("Hunt: %s stays out of reach, the sells work "+
				"without it and its buys are skipped",
				l.tracker.ObjectName(l.merchantID))
			l.merchantID = -1

			return true
		}

		// Far away on the same level: a plain approach walk.
		l.walkToward(x, y, z, now)

		return false
	}
	if l.tracker.SelfTargetID() != l.merchantID {
		// The transactions need the merchant as the selected target
		// (RequestBuyItem checks it server side): re-request the
		// selection once per second until the tracker confirmed it
		// and only then report ready.
		if now.Sub(l.merchantPick) >= selectPeriod {
			l.merchantPick = now
			if err := l.game.AttackTarget(l.merchantID); err != nil {
				l.logger.Printf("Hunt: merchant select failed: %v", err)
			}
		}

		return false
	}

	return true
}

// junkRemaining reports whether sellable inventory items are left the
// trip has not offered yet: every vendor visit sells the accumulated
// junk completely, batch after batch, whatever started the trip.
func (l *Loop) junkRemaining() bool {
	for _, item := range l.tracker.SellableItems() {
		if !l.sold[item.ObjectID] {
			return true
		}
	}

	return false
}

// sellJunk sells the next batch of inventory junk, most junky items
// first. Every item is offered once per trip: the server silently skips
// what it refuses to sell, so re-offering it forever would stall the
// trip. An empty batch (nothing left to sell) ends the selling.
func (l *Loop) sellJunk() {
	now := time.Now()
	if !l.sellAt.IsZero() && now.Sub(l.sellAt) < sellPause {
		return
	}
	l.sellAt = now
	junk := l.tracker.SellableItems()
	batch := make([]state.InventoryItem, 0, sellBatchSize)
	for _, item := range junk {
		if l.sold[item.ObjectID] {
			continue
		}
		batch = append(batch, item)
		if len(batch) >= sellBatchSize {
			break
		}
	}
	if len(batch) == 0 {
		l.startReturnLeg()

		return
	}
	if err := l.game.SellItems(batch); err != nil {
		l.logger.Printf("Hunt: sell request failed: %v", err)

		return
	}
	for _, item := range batch {
		l.sold[item.ObjectID] = true
	}
	l.logger.Printf("Hunt: offered %d items for sale", len(batch))
}

// engagesOnZoneEntry ends the return walk the moment the hunting
// zone holds a valid target: entering a zone means fighting
// whatever the entry radius offers, the walk to the farm spot or
// the zone center only continues while the surroundings stay
// empty (the level slack and the social fence of the constrained
// search apply here too). The next engage tick picks the target
// the search found.
func (l *Loop) engagesOnZoneEntry() bool {
	zone := l.zone()
	if zone == nil || !l.inZoneSelf() {
		return false
	}
	now := time.Now()
	if now.Sub(l.lastHit) < selectPeriod {
		return false
	}
	pick, ok := l.tracker.NearestAttackablePreferred(
		attackNearestRange, zone, l.activeSkips(now),
		l.maxTargetLevel(), true, l.zoneMobPriority)
	if !ok {
		return false
	}
	l.endTownTrip("a target stands inside the zone")
	l.logger.Printf("Hunt: engaging %s on the zone entry", pick.Name)

	return true
}

// startReturnLeg plans the walk back to the farm spot.
func (l *Loop) startReturnLeg() {
	l.phase = phaseTownReturn
	destX, destY, destZ := l.farmX, l.farmY, l.farmZ
	if zone := l.zone(); zone != nil &&
		(!zone.Contains(destX, destY) || (destX == 0 && destY == 0)) {
		// The farm spot belongs to a previous square (a zone switch
		// mid trip): return to the new center instead.
		destX, destY = zone.CX, zone.CY
	}
	dest := pathfind.Vec3{
		X: float64(destX),
		Y: float64(destY),
		Z: float64(destZ),
	}
	if !l.startWalkLeg(dest) {
		l.abortTownTrip("no walkable path back to the farm spot")

		return
	}
	l.logger.Printf("Hunt: walking back to the farm spot")
}

// endTownTrip finishes the trip and arms the trigger cooldown.
func (l *Loop) endTownTrip(reason string) {
	l.phase = phaseEngage
	l.target = 0
	l.clearBlindRecovery()
	l.lootID = 0
	l.waypoints = nil
	l.legDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
	l.tripStops = nil
	l.buysPlanned = false
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.shoppingPlanCache = nil
	l.shoppingPlanAt = time.Time{}
	l.shoppingPlanAdena = 0
	l.resetReplacementSales()
	l.tripEndedAt = time.Now()
	l.logger.Printf("Hunt: town trip ended: " + reason)
}

// abortTownTrip finishes a failed trip with a log line.
func (l *Loop) abortTownTrip(reason string) {
	l.endTownTrip("aborted, " + reason)
}

// resetTownTrip drops the trip state after a death. The village
// restart lands next to the shops, and the cooldown of the trip the
// death interrupted is cleared as well, so a full inventory sells
// right after the revival instead of farming with the junk first.
func (l *Loop) resetTownTrip() {
	if !l.tripActive() {
		return
	}
	l.phase = phaseEngage
	l.target = 0
	l.clearBlindRecovery()
	l.lootID = 0
	l.waypoints = nil
	l.legDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
	l.tripStops = nil
	l.buysPlanned = false
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.resetReplacementSales()
	l.tripEndedAt = time.Time{}
}

// standUpGuarded stands a sitting character up before an action the
// server refuses while it sits: the trip walks, the zone returns and
// the escape runs of the combat safety all move the character, and a
// walk started sitting would stall into the stuck re-paths. The
// toggle shares the pending transition gate with the rest logic, so
// the two never double toggle each other, and the walk starts on a
// later tick once the ChangeWaitType broadcast confirms the standing.
// The confirmation broadcast itself is only the start of the stand:
// the server holds the character paralyzed on the REST intention for
// a fixed animation window after it (the 2.5 s StandUpTask), and
// every move request of that window bounces off ActionFailed - the
// guard holds the caller through the settle window so the first walk
// request lands on a movable character.
func (l *Loop) standUpGuarded(now time.Time) bool {
	if !l.tracker.SelfSitting() {
		// Standing already: a stand transition of this guard went
		// out recently - hold the caller through the server side
		// stand window, then consume the transition so it never
		// lingers into the rest logic.
		if !l.restActionAt.IsZero() && !l.restActionSit {
			if now.Sub(l.restActionAt) < standSettlePeriod {
				return false
			}
			l.restActionAt = time.Time{}
		}

		return true
	}
	// Sitting: a transition is in flight (the rest sit request or this
	// guard's stand) - wait out its confirmation window before the
	// stand request, never double toggle.
	if !l.restActionAt.IsZero() &&
		now.Sub(l.restActionAt) < restRetryPeriod {
		return false
	}
	if err := l.game.ActionSitStand(); err != nil {
		l.logger.Printf("Hunt: stand up failed: %v", err)

		return false
	}
	l.restActionAt = now
	l.restActionSit = false

	return false
}
