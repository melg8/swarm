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
	// waypointArriveDist is the distance within which the FINAL
	// waypoint of a walk plan counts as reached: the wide trip
	// arrival radius (two geodata cells of slack) so a leg ends
	// even when the server stops the character slightly short of
	// the clicked point.
	waypointArriveDist = 150.0
	// waypointPassDist is the tighter arrival radius of the
	// INTERMEDIATE waypoints: a bridge ramp entry or a detour turn
	// must be walked THROUGH, not merely seen from the side. The
	// legacy wide radius here let the follower accept an entry
	// waypoint it never reached, cut the corner and grind into the
	// bridge railing side (the 2026-09-10 report: the plan held
	// the smooth semicircle onto the bridge, the follower skipped
	// it). Three geodata cells of slack.
	waypointPassDist = 50.0
	// waypointCorridor is the lateral distance from the segment
	// towards the next waypoint within which a character counts
	// as having PASSED the waypoint: only a character that moved
	// past the waypoint on the route itself may skip it (a server
	// correction, a jump), a character standing BESIDE the route
	// - the bridge railing side - has not passed anything and
	// walks back to the entry it missed.
	waypointCorridor = 100.0
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
	// skillListWaitLimit bounds the hold the first town trip of a
	// session puts on its start while the server skill list has not
	// arrived: the list lands within a second of the enter world, and
	// a trip started ahead of it drops the learning stops silently.
	skillListWaitLimit = 10 * time.Second
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
	// OverWater reports whether the walkable surface under the world
	// position lies below the C1 water level: the character stands
	// over a lake or sea bed (swimming or floating on it).
	OverWater(x, y float64, refZ int16) bool
	// DryLine reports whether the straight segment between two world
	// positions is a clean dry walk: walkable by the surface rules and
	// never dipping under the water level. The town walker checks
	// every click target with it before sending the move request.
	DryLine(start, end pathfind.Vec3) (bool, error)
	// FindWaterEscape plans the walk out of the water to the nearest
	// shore: a character standing over a lake bed cannot reach decks
	// the water has no walkable connection to, so the only sensible
	// walk is the one back to the shore.
	FindWaterEscape(start pathfind.Vec3) (*pathfind.Result, error)
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

// OverWater answers the geodata water surface check with the engine.
func (e engineNavigator) OverWater(x, y float64, refZ int16) bool {
	return e.engine.OverWater(x, y, refZ)
}

// DryLine answers the geodata dry line check with the engine settings.
func (e engineNavigator) DryLine(
	start, end pathfind.Vec3,
) (bool, error) {
	return e.engine.DryLine(start, end)
}

// FindWaterEscape plans the nearest shore walk with the engine.
func (e engineNavigator) FindWaterEscape(
	start pathfind.Vec3,
) (*pathfind.Result, error) {
	return e.engine.FindWaterEscape(start)
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
func (l *Loop) maybeStartTownTrip() { //nolint:cyclop,funlen // learning joined
	// The first trip of a session waits for the server skill list:
	// the learning stops plan on the skill queue and the packet burst
	// of the enter world (UserInfo, ItemList, SkillList) races the
	// first hunt ticks - a trip that starts between the ItemList and
	// the SkillList silently plans without the learning (the observed
	// sessions shopped on their first walk and never carried the
	// teach stop). The wait is bounded: a server that never lists
	// skills keeps the trips selling and shopping.
	if !l.tracker.SkillsListed() &&
		time.Since(l.tracker.StartedAt()) < skillListWaitLimit {
		return
	}
	shopping := l.shoppingTripEnabled() && l.shoppingWanted()
	learning := l.learnTripWanted()
	if l.navigator == nil || !l.tripCooldownOver() ||
		(!l.inventoryFull() && !shopping && !learning) {
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
	l.wetPlanTrusted = false
	l.waterEscapes = 0
	l.tripStops = []tripStop{{
		merchant: merchant,
		sell:     true,
		buys:     nil,
		teach:    false,
	}}
	l.buysPlanned = false
	l.buyAt = time.Time{}
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.resetReplacementSales()
	l.resetLearnState()
	if learning {
		// The learning stops ride behind the sell stop: the books
		// after the junk sold (the fresh adena funds them), the
		// teacher behind them, the gear shopping behind the teacher.
		l.planLearnStops()
	}
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
	if learning {
		// The learning contributes its lesson budget to the reason:
		// a learning-only trip names it, a combined one appends it.
		lessons := l.learnableLessons()
		lessonReason := strconv.Itoa(len(lessons)) + " lessons worth " +
			strconv.FormatInt(spTotal(lessons), 10) + " sp wait at " +
			"the teacher"
		if l.inventoryFull() || shopping {
			reason += ", " + lessonReason
		} else {
			reason = lessonReason
		}
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
	if l.interruptTripForAttacker(time.Now()) {
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

// interruptTripForAttacker answers the aggro that reaches the
// character mid trip: a mob holds it as the target (the blows of a
// social pull, an aggressive camp the steering could not dodge) and
// walking on only drags the chase through every camp on the route -
// the pile up the emergency logout then answers too late. The trip
// drops instead (the soft reset: no cooldown, the next tick re-arms
// the walk from wherever the answer leaves the character - the junk,
// the books and the sold proceeds all survive) and the mob gets the
// same aggro answer the hunting engage gives: a healthy character
// with a winnable attacker fights it at once, everything else keeps
// the defensive flow (the standard escape walk, the logout when the
// chase never shakes). It reports whether the tick was consumed by
// the answer.
func (l *Loop) interruptTripForAttacker(now time.Time) bool {
	attacker, ok := l.tracker.NearestAttacker()
	if !ok {
		return false
	}
	l.resetTownTrip()
	if l.attackerEngageable(attacker.ObjectID) {
		l.target = attacker.ObjectID
		l.engageAt = now
		l.clearBlindRecovery()
		l.logger.Printf("Hunt: town trip interrupted: %s is on us, "+
			"fighting it", attacker.Name)

		return true
	}
	l.logger.Printf("Hunt: town trip interrupted: %s is on us and "+
		"cannot be won, switching to the defense", attacker.Name)
	l.fleeFromThreat(now)

	return true
}

// startWalkLeg plans the walk to the destination and arms the waypoint
// follower. The search goal is the approach radius of the destination
// (the merchant interaction distance): a destination behind a counter
// or on a floor layer the geodata does not model is still reached on
// the surrounding deck, and unreachable destinations leave a fallback
// direct walk the server routes itself (its own pathfinder reaches
// what the pack misses, proven by the death leash of the earlier
// sessions). The planning position publishes as the leg origin of the
// walk plan view - the dump shows the whole walk from it. It reports
// whether the leg was planned.
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
	l.legStart = from
	l.waterEscape = false
	l.wetPlanTrusted = false
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

// waypointArrived reports whether the follower counts the waypoint at
// the index as reached from the character position: the intermediate
// waypoints need the tight pass radius (a detour turn or a bridge ramp
// entry must be walked through - the wide radius let the follower cut
// the corner into the railing), the final waypoint keeps the wide trip
// arrival radius (the search goal of the leg, the server may stop the
// character slightly short of the click).
func waypointArrived(
	waypoints []pathfind.Vec3, index int, selfX, selfY, selfZ int32,
) bool {
	radius := waypointPassDist
	if index == len(waypoints)-1 {
		radius = waypointArriveDist
	}

	return waypointDistance(waypoints[index], selfX, selfY, selfZ) <=
		radius
}

// waypointPassed reports whether the character already moved past the
// waypoint along the route towards the next one: the projection of the
// character onto the wp -> next segment is beyond the waypoint and the
// character stays within the corridor of the segment. A raw "the next
// waypoint is closer" test once let the follower skip the bridge entry
// waypoints while the character stood at the RAILING SIDE of the deck -
// the waypoint across the bridge was closer through the railing than
// the entry around the ramp - and the bot ground into the railing
// instead of walking the planned detour. The projection test keeps the
// skip working for its real purpose (a server correction or a restart
// jump placing the character ahead ON the route) while a character off
// to the side of the segment keeps targeting the waypoint it missed.
// The test is planar on purpose: the z axis belongs to the arrival
// distance, and a segment that degenerates in the plane (a vertical
// drop) never passes the character by the lateral logic.
func waypointPassed(
	wp, next pathfind.Vec3, selfX, selfY int32,
) bool {
	segX := next.X - wp.X
	segY := next.Y - wp.Y
	segLen := math.Hypot(segX, segY)
	if segLen < 1 {
		// A vertical drop segment: no planar pass geometry.
		return false
	}
	selfDX := float64(selfX) - wp.X
	selfDY := float64(selfY) - wp.Y
	along := (selfDX*segX + selfDY*segY) / segLen
	if along <= 0 {
		// Still before the waypoint: nothing passed yet.
		return false
	}
	lateral := math.Abs(selfDX*segY-selfDY*segX) / segLen

	return lateral <= waypointCorridor
}

// walkTownWaypoints follows the planned waypoints with ground click
// walks and returns true when the final waypoint is reached. The water
// guards run first: a character standing over a lake bed enters the
// shore escape (the water escape state below), and a character that
// walked out of one re-plans the interrupted leg from the shore.
// Legs longer than the server move request limit are split into
// straight intermediate points (the smoothing guarantees the line of
// sight of every leg, so the intermediate points stay on the verified
// segment). Every click of a dry walk is verified against the water:
// the server moves characters into water without any hesitation (its
// own pathfinding carries no water cost and swimming move requests
// skip the geodata validation entirely), so a click whose line would
// enter the water is never sent - the walk re-paths around the shore
// instead. A waypoint the character already passed ON THE ROUTE is
// skipped: a server position correction or a restart jump can place
// the character ahead of the follower, and walking back to a passed
// waypoint would loop. A walk that stands still re-paths from the
// current position to the leg destination, bounded by the re-path
// budget of the trip.
func (l *Loop) walkTownWaypoints() bool {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return false
	}
	if l.navigator != nil {
		if l.navigator.OverWater(
			float64(selfX), float64(selfY), int16(selfZ)) {
			return l.walkWaterEscape(selfX, selfY, selfZ)
		}
		if l.waterEscape {
			// The character is back on dry ground: the escape is done,
			// the interrupted leg re-plans from the shore with a fresh
			// re-path budget (the escape was a recovery, not a failure).
			l.waterEscape = false
			l.rePaths = 0
			l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
			l.logger.Printf("Hunt: back on the shore at %d %d %d, "+
				"re-planning the walk", selfX, selfY, selfZ)
			if !l.startWalkLeg(l.legDest) {
				l.abortTownTrip("no walkable path from the shore")

				return false
			}

			return false
		}
	}

	return l.followWaypoints(selfX, selfY, selfZ, time.Now(), true)
}

// followWaypoints is the shared waypoint follower core of the town
// legs and the water escapes: the waypoint arrival (tight for the
// intermediate turns, wide for the final goal), the passed waypoint
// skipping, the stuck tracking, the leg splitting and the click
// pacing. The waterGuard switch tells whether the click lines must
// verify dry before they are sent (the town legs: the character is
// ashore and must stay so) or not (the water escape: its legs
// intentionally cross the water back to the shore).
func (l *Loop) followWaypoints(
	selfX, selfY, selfZ int32, now time.Time, waterGuard bool,
) bool {
	for l.wpIndex < len(l.waypoints) {
		if waypointArrived(l.waypoints, l.wpIndex,
			selfX, selfY, selfZ) {
			l.wpIndex++
			l.moveAt = time.Time{}

			continue
		}
		// Not reached: skip it only when the character already
		// passed it on the route towards the next waypoint.
		if l.wpIndex+1 < len(l.waypoints) &&
			waypointPassed(l.waypoints[l.wpIndex],
				l.waypoints[l.wpIndex+1], selfX, selfY) {
			l.wpIndex++
			l.moveAt = time.Time{}

			continue
		}

		break
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
	// The aggro-aware steering: the camps of idle aggressive mobs
	// sitting on the leg bend it sideways (see loop_avoid.go). Every
	// transit walk passes through it - the town runs both ways, the
	// returns to the farm spot, the inter-ground walks of the spot
	// economy - while the mobs at the destination stay exempt.
	moveXI, moveYI := int32(math.Round(moveX)), int32(math.Round(moveY))
	moveZI := int32(math.Round(moveZ))
	if ax, ay, dodged := l.steerClearOfAggro(
		selfX, selfY, selfZ, moveXI, moveYI, moveZI,
		int32(l.legDest.X), int32(l.legDest.Y), now); dodged {
		moveX, moveY = float64(ax), float64(ay)
	}
	// The water guard runs after the steering so the line it verifies
	// is the one actually being sent (see legWaterGuarded).
	if l.legWaterGuarded(waterGuard) && l.clickWouldEnterWater(
		selfX, selfY, selfZ, moveX, moveY, moveZ) {
		return false
	}
	l.moveAt = now
	if err := l.game.WalkTo(int32(moveX), int32(moveY), int32(moveZ)); err != nil {
		l.logger.Printf("Hunt: town walk request failed: %v", err)
	}

	return false
}

// legWaterGuarded reports whether the follower must dry-check the
// click line of the current leg before sending it: the guard stays
// off without a navigator (the walk was planned elsewhere, the
// follower only walks it) and on a leg whose plan the guard already
// released (the trusted village plaza decks - the re-paths kept
// reproducing the same geodata water while the server routing knows
// the real plaza).
func (l *Loop) legWaterGuarded(waterGuard bool) bool {
	return waterGuard && !l.wetPlanTrusted && l.navigator != nil
}

// clickWouldEnterWater verifies the straight line of a ground click
// before it is sent and re-paths the walk around the shore when the
// line would enter the water: the server walks characters into lakes
// (its own routing has no water cost at all, its move validation
// accepts the gradual underwater beds, and once the character swims
// its move requests skip the geodata checks entirely - the reported
// trip swam below the elven village plateau this way and stood
// paralyzed under its cliff). It reports whether the click was
// refused and the walk re-planned.
func (l *Loop) clickWouldEnterWater(
	selfX, selfY, selfZ int32, moveX, moveY, moveZ float64,
) bool {
	dry, err := l.navigator.DryLine(
		pathfind.Vec3{
			X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
		},
		pathfind.Vec3{X: moveX, Y: moveY, Z: moveZ},
	)
	if err != nil || dry {
		// A line the geodata cannot verify stays on the old behavior:
		// the walk was planned over the same data, the drift the
		// guard exists for shows up as a wet line, not an error.
		return false
	}
	l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
	l.rePaths++
	if l.rePaths > maxRePaths {
		if l.waterEscapes > 0 {
			// A trusted plan already swam on this trip (the escape ran):
			// the geodata water on the route is real, the guard keeps its
			// old answer and ends the trip.
			l.abortTownTrip("the walk would cross water")

			return true
		}
		// The re-paths keep reproducing the same wet line: the geodata
		// pack itself routes through it (the disconnected village decks
		// - the plaza cells without a modeled floor resolve to the lake
		// layer below them, every straight line over the plaza center
		// fails the dry raster while the points themselves stand dry).
		// The plan is trusted for the rest of the leg: the server
		// routing knows the real plaza, and the standing water check
		// (OverWater) plus the shore escape still catch a genuine swim
		// - an escape on the trusted leg re-arms the abort above for
		// the next budget exhaustion.
		l.wetPlanTrusted = true
		l.logger.Printf("Hunt: the planned walk crosses geodata water "+
			"at %d %d with no dry re-route, trusting the plan over the "+
			"server routing", selfX, selfY)

		return false
	}
	l.logger.Printf("Hunt: the walk would enter water at %d %d, "+
		"re-pathing around the shore (%d of %d)",
		selfX, selfY, l.rePaths, maxRePaths)
	if !l.startWalkLeg(l.legDest) {
		l.abortTownTrip("re-path failed")

		return true
	}

	return true
}

// walkWaterEscape drives the shore recovery while the character
// stands over water: the first entry plans the nearest shore walk
// (the water escape search of the navigator), the following ticks
// walk it with the plain waypoint follower (no click guard - the
// escape legs cross the water by design), and an escape whose
// waypoints are walked out while the character still stands wet
// re-plans from the current position: the arrival slack may have
// stopped the character a wet cell short of the waterline. It never
// reports the trip leg complete - the leg re-plans from the shore
// once the character is dry (walkTownWaypoints routes back to the
// normal follower then).
func (l *Loop) walkWaterEscape(
	selfX, selfY, selfZ int32,
) bool {
	if !l.waterEscape {
		if !l.planWaterEscape(selfX, selfY, selfZ) {
			l.abortTownTrip("stuck in the water without a shore path")
		}

		return false
	}
	if !l.followWaypoints(selfX, selfY, selfZ, time.Now(), false) {
		return false
	}
	l.rePaths++
	if l.rePaths > maxRePaths {
		l.abortTownTrip("the water escape could not leave the water")

		return false
	}
	if !l.planWaterEscape(selfX, selfY, selfZ) {
		l.abortTownTrip("stuck in the water without a shore path")
	}

	return false
}

// planWaterEscape arms the walk out of the water to the nearest
// shore: the escape waypoints replace the current leg, the follower
// cursor restarts and the stuck tracking clears so the slow swim
// gets a fresh stuck window. It reports whether an escape was found.
func (l *Loop) planWaterEscape(selfX, selfY, selfZ int32) bool {
	from := pathfind.Vec3{
		X: float64(selfX),
		Y: float64(selfY),
		Z: float64(selfZ),
	}
	result, err := l.navigator.FindWaterEscape(from)
	if err != nil || result == nil || !result.Found ||
		len(result.Waypoints) == 0 {
		l.logger.Printf("Hunt: no walkable shore from %d %d %d: %v",
			selfX, selfY, selfZ, err)

		return false
	}
	l.waypoints = result.Waypoints
	l.wpIndex = 0
	l.waterEscape = true
	l.wetPlanTrusted = false
	l.waterEscapes++
	l.moveAt = time.Time{}
	l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
	last := result.Waypoints[len(result.Waypoints)-1]
	l.logger.Printf("Hunt: character stands in the water at %d %d %d, "+
		"escaping to the shore at %d %d %d",
		selfX, selfY, selfZ, int32(last.X), int32(last.Y), int32(last.Z))

	return true
}

// walkStuck tracks the movement progress of the walker and re-paths
// around the obstacle once the character stands still for too long.
// A stuck water escape re-plans the escape itself - the town leg is
// meaningless until the character is back ashore. It reports whether
// the trip had to abort.
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
	if l.waterEscape {
		// The escape itself stands still: re-plan it from the current
		// position (the swim may need a different shore click than the
		// first plan offered).
		l.logger.Printf("Hunt: water escape stuck, re-planning "+
			"(%d of %d)", l.rePaths, maxRePaths)
		if !l.planWaterEscape(selfX, selfY, l.selfZForEscape()) {
			l.abortTownTrip("water escape re-plan failed")

			return true
		}

		return false
	}
	l.logger.Printf("Hunt: town walk stuck, re-pathing (%d of %d)",
		l.rePaths, maxRePaths)
	if !l.startWalkLeg(l.legDest) {
		l.abortTownTrip("re-path failed")

		return true
	}

	return false
}

// selfZForEscape returns the current character z for the escape
// re-planning of walkStuck: the escape needs the full position and
// the stuck tracker only carries x and y, so the z comes from the
// tracker on demand (0 when the position is not known yet - the
// escape planner resolves the layer of the standing cell anyway).
func (l *Loop) selfZForEscape() int32 {
	_, _, z, ok := l.tracker.SelfPosition()
	if !ok {
		return 0
	}

	return z
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
func (l *Loop) tickTownSell() { //nolint:cyclop // legs of one trip
	now := time.Now()
	if l.teachStop() {
		// The teacher stop: approach the class master, click it and
		// learn the queued lessons (see learning.go). The stop carries
		// no buys and never sells - the junk selling belongs to the
		// first stop of the trip. A teacher that never showed up (or
		// stands out of reach) skips the lessons - the requests
		// resolve their trainer through the last folk npc and cannot
		// run without it.
		if !l.handleTeacher(now) {
			return
		}
		if l.teacherID > 0 && !l.tickTeacherLessons(now) {
			return
		}
		l.advanceTripStop()

		return
	}
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

// sellableJunk lists the inventory junk of the sell trips without
// the newbie kit: the starter items are unsellable on the server
// (is_sellable=false - every offer of them is silently skipped), so
// offering them only wastes a transaction window of the flood
// protector once per trip - the destroy flow of the replaced starters
// owns them instead.
func (l *Loop) sellableJunk() []state.InventoryItem {
	junk := make([]state.InventoryItem, 0, 8)
	for _, entry := range l.tracker.SellableItemsExcluding(
		l.plannedEquipKeeps()) {
		if gear.IsStarterItem(entry.ItemID) {
			continue
		}
		junk = append(junk, entry)
	}

	return junk
}

// junkRemaining reports whether sellable inventory items are left the
// trip has not offered yet: every vendor visit sells the accumulated
// junk completely, batch after batch, whatever started the trip. The
// planned equips of the auto equipment stay out of the junk (a looted
// or bought upgrade waits for its use item request, the sell stop
// must not eat it) and so does the unsellable newbie kit (the destroy
// flow owns it).
func (l *Loop) junkRemaining() bool {
	for _, item := range l.sellableJunk() {
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
	junk := l.sellableJunk()
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

// clearTalkedTarget drops the npc selection a stop or a whole trip
// left behind (the merchant select, the teacher talk click): the self
// click of the clear replaces the server side selection, so the
// hunting engage that follows the trip never adopts the friendly
// villager as its target (the forced attacks on it only burn the
// stuck timeout). The call is a no-op without a selection.
func (l *Loop) clearTalkedTarget() {
	if l.tracker.SelfTargetID() == 0 {
		return
	}
	if err := l.game.ClearTarget(); err != nil {
		l.logger.Printf("Hunt: target clear failed: %v", err)
	}
}

// endTownTrip finishes the trip and arms the trigger cooldown.
func (l *Loop) endTownTrip(reason string) {
	l.clearTalkedTarget()
	l.phase = phaseEngage
	l.target = 0
	l.clearBlindRecovery()
	l.lootID = 0
	l.waypoints = nil
	l.legDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
	l.legStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
	l.waterEscape = false
	l.tripStops = nil
	l.buysPlanned = false
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.shoppingPlanCache = nil
	l.shoppingPlanAt = time.Time{}
	l.shoppingPlanAdena = 0
	l.resetReplacementSales()
	l.resetLearnState()
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
	l.legStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
	l.waterEscape = false
	l.tripStops = nil
	l.buysPlanned = false
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.resetReplacementSales()
	l.resetLearnState()
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
