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
	"time"

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
	// FindPathTo plans a walk that must end on the deck of the target
	// cell resolved against targetZ; Found=false when that deck is
	// unreachable from the start.
	FindPathTo(start, end pathfind.Vec3, targetZ int16) (
		*pathfind.Result, error,
	)
	// FindPath plans a walk to the target cell arriving on whatever
	// deck of it the walk reaches first.
	FindPath(start, end pathfind.Vec3) (*pathfind.Result, error)
}

// engineNavigator adapts a geodata engine to the Navigator interface,
// applying the configured maximum passable height of the engine.
type engineNavigator struct {
	engine *pathfind.Engine
}

// NewNavigator wraps a geodata engine into the town trip navigator.
func NewNavigator(engine *pathfind.Engine) Navigator {
	return engineNavigator{engine: engine}
}

// FindPathTo searches the walkable path with the engine settings and a
// strict arrival on the destination deck.
func (e engineNavigator) FindPathTo(
	start, end pathfind.Vec3, targetZ int16,
) (*pathfind.Result, error) {
	return e.engine.FindPathTo(
		start, end, targetZ, e.engine.MaxPassableHeight())
}

// FindPath searches the walkable path with the engine settings.
func (e engineNavigator) FindPath(
	start, end pathfind.Vec3,
) (*pathfind.Result, error) {
	return e.engine.FindPath(start, end, e.engine.MaxPassableHeight())
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
// enough and the trip cooldown is over. Everything that can block the
// trip (no navigator, no geodata, no path) arms the cooldown, so a
// broken deployment does not retry every tick.
func (l *Loop) maybeStartTownTrip() {
	if l.navigator == nil || !l.tripCooldownOver() ||
		!l.inventoryFull() {
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
	l.phase = phaseTownWalk
	stats := l.tracker.InventoryStats()
	l.logger.Printf("Hunt: inventory at %d slots and %.0f%% weight, "+
		"walking to the trader %s", stats.Slots, stats.WeightPercent,
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
		if l.walkTownWaypoints() {
			l.endTownTrip("back at the farm spot")
		}
	}
}

// startWalkLeg plans the walk to the destination and arms the waypoint
// follower. The destination is planned as a targeted search first (the
// walk must end on the deck the destination stands on); when that deck
// is unreachable - some village decks are disconnected from the fields
// in the geodata pack - the plain search falls back to any deck, and
// when no geodata path exists at all the leg becomes a single direct
// walk the server routes itself (its own pathfinding reaches what the
// pack misses, proven by the death leash of the earlier sessions). It
// reports whether the leg was planned.
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
	result, err := l.navigator.FindPathTo(from, dest, int16(dest.Z))
	if err != nil {
		l.logger.Printf("Hunt: town trip path search failed: %v", err)

		return false
	}
	if result == nil || !result.Found || len(result.Waypoints) == 0 {
		result, err = l.navigator.FindPath(from, dest)
		if err != nil {
			l.logger.Printf("Hunt: town trip path search failed: %v", err)

			return false
		}
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

// walkTownWaypoints follows the planned waypoints with ground click
// walks and returns true when the final waypoint is reached. Legs
// longer than the server move request limit are split into straight
// intermediate points (the smoothing guarantees the line of sight of
// every leg, so the intermediate points stay on the verified segment).
// A walk that stands still re-paths from the current position to the
// leg destination, bounded by the re-path budget of the trip.
func (l *Loop) walkTownWaypoints() bool {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return false
	}
	now := time.Now()
	for l.wpIndex < len(l.waypoints) {
		wp := l.waypoints[l.wpIndex]
		if math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY)) >
			waypointArriveDist {
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

// enterSellPhase switches into the selling state at the shop.
func (l *Loop) enterSellPhase() {
	l.phase = phaseTownSell
	l.sellPhaseAt = time.Now()
	l.sellAt = time.Time{}
	l.merchantID = 0
	l.merchantPick = time.Time{}
	l.logger.Printf("Hunt: shop reached, selling the junk")
}

// tickTownSell sells the inventory junk and heads back once the
// inventory is light again.
func (l *Loop) tickTownSell() {
	if !l.inventoryFull() {
		stats := l.tracker.InventoryStats()
		l.logger.Printf("Hunt: inventory light again (%d slots, %.0f%% "+
			"weight), heading back", stats.Slots, stats.WeightPercent)
		l.startReturnLeg()

		return
	}
	if !l.handleMerchant(time.Now()) {
		return
	}
	l.sellJunk()
}

// handleMerchant approaches the shop merchant and selects it like the
// official client does before a transaction. It reports false while the
// character still walks toward the merchant or waits for one to appear.
// The sale itself works without a merchant (the standard inventory sell
// list), so a merchant that never shows up only delays it.
func (l *Loop) handleMerchant(now time.Time) bool {
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
	merchant, ok := l.tracker.NearestNpcByTemplates(
		merchantTemplates(), merchantFindRadius)
	if ok {
		l.merchantID = merchant.ObjectID
		l.logger.Printf("Hunt: selling to " + merchant.Name)

		return false
	}
	if now.Sub(l.sellPhaseAt) < merchantWaitTimeout {
		return false
	}
	l.logger.Printf("Hunt: no merchant around, selling without one")
	l.merchantID = -1

	return true
}

// approachMerchant walks to the merchant, selects it inside the
// interaction distance and reports when the sale may start. The
// selection re-requests itself once per second until the
// MyTargetSelected answer confirms it. A merchant standing on another
// deck of the geodata (the disconnected village decks) is skipped: the
// 3D interaction distance of the server can never be met and the sale
// does not need the merchant.
func (l *Loop) approachMerchant(now time.Time) bool {
	x, y, z, ok := l.tracker.ObjectPosition(l.merchantID)
	if !ok {
		l.merchantID = -1

		return true
	}
	selfX, selfY, selfZ, _ := l.tracker.SelfPosition()
	dist := math.Hypot(float64(x-selfX), float64(y-selfY))
	if dist > merchantApproachDist {
		l.walkToward(x, y, z, now)

		return false
	}
	if math.Abs(float64(z-selfZ)) > merchantApproachDist {
		l.logger.Printf("Hunt: %s stands on another deck, selling "+
			"without one", l.tracker.ObjectName(l.merchantID))
		l.merchantID = -1

		return true
	}
	if l.tracker.SelfTargetID() != l.merchantID &&
		now.Sub(l.merchantPick) >= selectPeriod {
		l.merchantPick = now
		if err := l.game.AttackTarget(l.merchantID); err != nil {
			l.logger.Printf("Hunt: merchant select failed: %v", err)
		}

		return false
	}

	return true
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

// startReturnLeg plans the walk back to the farm spot.
func (l *Loop) startReturnLeg() {
	l.phase = phaseTownReturn
	dest := pathfind.Vec3{
		X: float64(l.farmX),
		Y: float64(l.farmY),
		Z: float64(l.farmZ),
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
	l.lootID = 0
	l.waypoints = nil
	l.legDest = pathfind.Vec3{}
	l.tripEndedAt = time.Now()
	l.logger.Printf("Hunt: town trip ended: " + reason)
}

// abortTownTrip finishes a failed trip with a log line.
func (l *Loop) abortTownTrip(reason string) {
	l.endTownTrip("aborted, " + reason)
}

// resetTownTrip drops the trip state after a death without arming the
// cooldown: the village restart is not a failed trip, and a full
// inventory should sell right after the revival.
func (l *Loop) resetTownTrip() {
	if !l.tripActive() {
		return
	}
	l.phase = phaseEngage
	l.target = 0
	l.lootID = 0
	l.waypoints = nil
	l.legDest = pathfind.Vec3{}
}
