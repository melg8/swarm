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
	// stuckFastTimeout is the shorter timeout that applies after the
	// first stuck skip of a trip: once the walker knows the server
	// refuses its clicks on this leg, waiting the full 15 seconds for
	// every subsequent waypoint just burns the trip's time budget. The
	// shorter window keeps the recovery responsive while still letting
	// a slow server position broadcast land before the next skip.
	stuckFastTimeout = 4 * time.Second
	// maxRePaths bounds the re-paths of one trip before it aborts.
	// The budget bounds only the full leg re-plan (startWalkLeg) - the
	// waypoint skip of walkStuck does NOT consume it. This lets the
	// walker cycle through several waypoints looking for one the server
	// accepts without exhausting the budget, while still bounding the
	// expensive re-plan operations.
	maxRePaths = 3
	// merchantApproachDist is the distance the seller stands from the
	// merchant: below the 250 units interaction distance of the server.
	merchantApproachDist = 200.0
	// npcInteractionDist is the server INTERACTION_DISTANCE of 250:
	// the talk click (ClickObject) and the transactions succeed within
	// this 3D distance of the npc. The approach walk aims the offset
	// ring at 150 units 2D, and a small z gap (the trainer hall floor
	// is 40 units above the approach deck) keeps dist3D above the
	// approach gate (200) but well within this interaction gate - the
	// talk click must fire from the offset ring, not wait for the
	// approach gate that the z gap keeps unreachable (the 2026-09-11
	// 05:45 dump looped forever on the offset ring).
	npcInteractionDist = 250.0
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
	// clickShortenFloor bounds the halving of a walk click the
	// server validation refuses: below it the refusal is local (the
	// character stands boxed) and the follower hops or re-paths
	// instead of crawling micro legs.
	clickShortenFloor = 100.0
	// hopCoincideDist is the distance under which a walked-past
	// waypoint counts as stood on: the escape hop of a refused
	// click targets the nearest plan bend between it and the
	// waypoint arrival radius.
	hopCoincideDist = 15.0
	// npcApproachOffset is the 2D distance the bot stops short of
	// a town npc when the approach walk clicks the ground: the
	// click targets a point this many units from the npc toward
	// the bot, keeping the click line outside the building walls.
	// The server's getValidLocation walks a Bresenham line that
	// can "step over" onto the roof layer when the click targets
	// the npc's exact cell inside a building (the 2026-09-11 roof
	// teleport report: the bot clicked Cobendell's spawn point,
	// the line crossed the south wall and the height-step
	// fallback resolved the target onto the roof at z -2456
	// instead of the ground floor at z -2792). The offset keeps
	// the click target on the surrounding deck, within the 250
	// unit server interaction distance but outside the walled
	// interior.
	npcApproachOffset = 150.0
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
	// FindPathApproachDry plans the same walk with the water walled
	// off: every waypoint of a found route stands above the water
	// level, a target only swimming reaches answers not found. The
	// shore walks navigate with it - a planned swim is a plan the
	// click guard refuses leg by leg.
	FindPathApproachDry(start, end pathfind.Vec3, approachRadius float64) (
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
	// WaterCrossed reports whether the straight line between two
	// world positions crosses cells whose geodata surface lies below
	// the water level (the pure water raster - a height step of the
	// terrain is not water and never trips it). The town walker checks
	// every click target with it before sending the move request.
	WaterCrossed(start, end pathfind.Vec3) (bool, error)
	// FindWaterEscape plans the walk out of the water to the nearest
	// shore: a character standing over a lake bed cannot reach decks
	// the water has no walkable connection to, so the only sensible
	// walk is the one back to the shore.
	FindWaterEscape(start pathfind.Vec3) (*pathfind.Result, error)
	// ValidateClick mirrors the server-side validation of a
	// mouse-mode move request: it answers the destination the
	// server would actually walk to and whether the click runs at
	// all. A refused click (the geodata correction collapses the
	// target onto the walker) never moves the character - the
	// follower reacts to it instead of sending it.
	ValidateClick(from, to pathfind.Vec3) (pathfind.Vec3, bool)
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

// FindPathApproachDry searches the water walled path with the engine
// settings and the approach radius goal.
func (e engineNavigator) FindPathApproachDry(
	start, end pathfind.Vec3, approachRadius float64,
) (*pathfind.Result, error) {
	return e.engine.FindPathApproachDry(
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

// WaterCrossed answers the geodata water raster with the engine.
func (e engineNavigator) WaterCrossed(
	start, end pathfind.Vec3,
) (bool, error) {
	return e.engine.WaterCrossed(start, end)
}

// FindWaterEscape plans the nearest shore walk with the engine.
func (e engineNavigator) FindWaterEscape(
	start pathfind.Vec3,
) (*pathfind.Result, error) {
	return e.engine.FindWaterEscape(start)
}

// ValidateClick mirrors the server move validation with the engine.
func (e engineNavigator) ValidateClick(
	from, to pathfind.Vec3,
) (pathfind.Vec3, bool) {
	return e.engine.ValidateClick(from, to)
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

// tripCooldownOver reports whether a new town trip may start. A
// bare-handed character with an affordable weapon retries on the
// short weapon run cooldown: punching mobs through the five minute
// cooldown of an ordinary trip is the exact outcome the weapon run
// exists to prevent.
func (l *Loop) tripCooldownOver() bool {
	if l.tripEndedAt.IsZero() {
		return true
	}
	cooldown := tripCooldown
	if l.weaponlessRunWanted() {
		cooldown = weaponRunCooldown
	}

	return time.Since(l.tripEndedAt) >= cooldown
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
		time.Since(l.tracker.SessionStartedAt()) < skillListWaitLimit {
		return
	}
	// A bot outside the hunting zone returns first: the zone return
	// owns the walk until the bot is back in the zone. A town trip
	// started outside the zone (a village respawn after an emergency
	// logout) would try to walk to the village shops - where the bot
	// already stands - and then fail to return to the zone, leaving
	// the bot stuck at the village (the 2026-09-11 06:00 dump: test3
	// at 43000 50184, the learning trip started before the zone
	// return, both failed with "no dry path", the bot never moved).
	shopping := l.shoppingTripEnabled() && l.shoppingWanted()
	learning := l.learnTripWanted()
	// The weapon run outranks every other trip reason: a character
	// without any weapon shops for one at once, whatever the inventory
	// and the lesson queue say.
	weaponRun := l.weaponlessRunWanted()
	if l.navigator == nil || !l.tripCooldownOver() ||
		(!l.inventoryFull() && !shopping && !learning && !weaponRun) {
		return
	}
	// A bot outside the hunting zone returns first: the zone return
	// owns the walk until the bot is back in the zone. The weapon run
	// is the sole exception - a bare-handed character shops for a
	// weapon at once, even outside the zone (punching mobs through the
	// walk home is worse than a late return). The 2026-09-11 06:00
	// dump showed a learning trip starting at the village (outside the
	// zone) before the zone return, both searches failed with "no dry
	// path", and the bot never moved.
	if !weaponRun && l.zone() != nil && !l.inZoneSelf() {
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
	// The weapon leads the trip that buys it: the sell stop routes to
	// the weapon purchase's merchant, so the sell-first of the replaced
	// weapon and the buy share ONE stop (the junk sells at any
	// merchant) and the replacement lands right after the sale instead
	// of a village walk later - every abort in between used to leave
	// the character bare-handed. A bare-handed character runs the
	// weapon errand alone: the lessons and the books wait for the next
	// trip, nothing outranks the weapon.
	if weaponMerchant, ok := l.weaponStopMerchant(); ok {
		merchant = weaponMerchant
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
		teach:    false,
	}}
	l.buysPlanned = false
	l.buyAt = time.Time{}
	l.buyRequested = nil
	l.buyConfirmAt = time.Time{}
	l.buyRetries = 0
	l.resetReplacementSales()
	l.resetLearnState()
	if learning && !weaponRun {
		// The learning stops ride behind the sell stop: the books
		// after the junk sold (the fresh adena funds them), the
		// teacher behind them, the gear shopping behind the teacher.
		// The weapon run carries none of them: a bare-handed
		// character walks for the weapon and back, the lessons ride
		// the next trip (their queue keeps waiting).
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
	if weaponRun {
		// The bare-handed errand names itself: the 2 damage punches of
		// the dump report read at a glance in the log tail.
		reason = "no weapon in hand, the weapon run comes first"
	}
	if learning && !weaponRun {
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

// npcApproachPoint computes the ground click target for a town npc
// approach: a point npcApproachOffset units from the npc toward the
// bot, so the Bresenham click line the server validates stays outside
// the building walls. The z is the npc's z: the server resolves the
// target cell layer from it, picking the ground floor the npc stands
// on. When the bot already stands within the offset distance, the
// bot's own x and y are returned (the click collapses to a no-op the
// caller skips in favor of the talk click).
func npcApproachPoint(
	npcX, npcY, npcZ, selfX, selfY int32,
) (int32, int32, int32) {
	dx := float64(selfX - npcX)
	dy := float64(selfY - npcY)
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return npcX, npcY, npcZ
	}
	frac := npcApproachOffset / dist
	if frac >= 1 {
		return selfX, selfY, npcZ
	}
	ax := float64(npcX) + dx*frac
	ay := float64(npcY) + dy*frac

	return int32(math.Round(ax)), int32(math.Round(ay)), npcZ
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
// follower. The search is dry (the water walled off): a planned swim
// is a plan the click guard refuses leg by leg - the walker would burn
// its re-path budget re-planning the identical wet route and abort,
// the delevel water loop of the 2026-09-10 state dump. The search goal
// is the approach radius of the destination (the merchant interaction
// distance): a destination behind a counter or on a floor layer the
// geodata does not model is still reached on the surrounding deck. A
// destination the dry geodata cannot reach reports false - the callers
// abort the trip and arm their cooldowns instead of walking into the
// water. The planning position publishes as the leg origin of the
// walk plan view - the dump shows the whole walk from it. It reports
// whether the leg was planned.
func (l *Loop) startWalkLeg(dest pathfind.Vec3) bool {
	return l.startWalkLegSearch(dest, false)
}

// startZoneReturnLeg plans the zone return walk with a non-dry
// fallback: the dry search (water walled off) runs first, and when it
// fails the non-dry search (water allowed, with a cost penalty) runs
// as a fallback. The zone return must bring the bot home even when
// the dry search fails for unknown reasons (the 2026-09-11 06:00
// dump: the dry search reported "no dry path" from 43000 50184 to
// both the zone center and Herbiel 276 units away, while the offline
// probe against the same geodata found both paths - the runtime
// difference is unresolved, but the non-dry fallback gives the bot a
// route). The click guard of the town walk follower refuses water
// legs and re-paths around the shore, so a non-dry plan with water
// legs is still safe to walk - the bot follows the dry parts and
// re-plans at the waterline. It reports whether the leg was planned.
func (l *Loop) startZoneReturnLeg(dest pathfind.Vec3) bool {
	if l.startWalkLegSearch(dest, false) {
		return true
	}
	l.logger.Printf("Hunt: no dry zone return path to %d %d, "+
		"trying the non-dry search", int(dest.X), int(dest.Y))

	return l.startWalkLegSearch(dest, true)
}

// startWalkLegSearch plans the walk to the destination through either
// the dry or the non-dry approach search and arms the waypoint
// follower. The dry switch walls the water off (the town trips refuse
// a planned swim); the non-dry switch allows water crossings (the
// zone return fallback). It reports whether the leg was planned.
func (l *Loop) startWalkLegSearch(dest pathfind.Vec3, nonDry bool) bool {
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return false
	}
	from := pathfind.Vec3{
		X: float64(selfX),
		Y: float64(selfY),
		Z: float64(selfZ),
	}
	var result *pathfind.Result
	var err error
	if nonDry {
		result, err = l.navigator.FindPathApproach(
			from, dest, tripApproachRadius)
	} else {
		result, err = l.navigator.FindPathApproachDry(
			from, dest, tripApproachRadius)
	}
	if err != nil {
		l.logger.Printf("Hunt: town trip path search failed: %v", err)

		return false
	}
	if result == nil || !result.Found || len(result.Waypoints) == 0 {
		if !nonDry {
			l.logger.Printf("Hunt: no dry path to %d %d, the walk would "+
				"swim", int(dest.X), int(dest.Y))
		} else {
			l.logger.Printf("Hunt: no path to %d %d at all",
				int(dest.X), int(dest.Y))
		}

		return false
	}
	l.waypoints = result.Waypoints
	l.wpIndex = 0
	l.legDest = dest
	l.legStart = from
	l.waterEscape = false
	l.moveAt = time.Time{}
	l.stuckAt = time.Time{}
	l.stuckFast = false

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
			l.stuckFast = false
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

// advanceWaypoints walks the waypoint cursor forward as far as the
// character's position allows: a waypoint counts as passed when it is
// reached within its radius or already bypassed along the route AND
// the straight line from the actual standing cell to the successor
// waypoint is walkable. The arrival radius is wide enough to cover
// the tight waypoints of a ramp climb, but the line from the actual
// standing cell to the next waypoint may still cross a closed wall -
// skipping ahead would click through it and the server cancels the
// move at the character's own position (the 2026-09-11 teacher walk
// stuck: the follower skipped the 16 unit ramp steps and clicked the
// plaza waypoint through the railing). The gated waypoint stays the
// target: walking onto it re-opens the line.
func (l *Loop) advanceWaypoints(selfX, selfY, selfZ int32) {
	for l.wpIndex < len(l.waypoints) {
		arrived := waypointArrived(
			l.waypoints, l.wpIndex, selfX, selfY, selfZ)
		passed := !arrived && l.wpIndex+1 < len(l.waypoints) &&
			waypointPassed(l.waypoints[l.wpIndex],
				l.waypoints[l.wpIndex+1], selfX, selfY)
		if !arrived && !passed {
			return
		}
		if !l.legAdvanceClear(selfX, selfY, selfZ, l.wpIndex+1) {
			return
		}
		l.wpIndex++
		l.moveAt = time.Time{}
	}
}

// followWaypoints is the shared waypoint follower core of the town
// legs and the water escapes: the waypoint arrival (tight for the
// intermediate turns, wide for the final goal), the passed waypoint
// skipping, the stuck tracking and the click pacing. The waterGuard
// switch tells whether the click lines must verify dry before they
// are sent (the town legs: the character is ashore and must stay so)
// or not (the water escape: its legs intentionally cross the water
// back to the shore).
func (l *Loop) followWaypoints(
	selfX, selfY, selfZ int32, now time.Time, waterGuard bool,
) bool {
	l.advanceWaypoints(selfX, selfY, selfZ)
	if l.wpIndex >= len(l.waypoints) {
		return true
	}
	if l.walkStuck(now, selfX, selfY) {
		return false
	}
	if !l.moveAt.IsZero() && now.Sub(l.moveAt) < walkRequestPeriod {
		return false
	}
	l.clickWaypoint(selfX, selfY, selfZ, now, waterGuard)

	return false
}

// clickWaypoint aims the current waypoint, bends the click around the
// idle aggressive camps, guards the line against water and the server
// refusal and sends it. The leg splitting caps the click at the
// server move request limit; the water guard and the server click
// validation run after the steering so the line they verify is the
// one actually being sent. Without a navigator both guards stay off
// (the walk was planned elsewhere, the follower only walks it).
func (l *Loop) clickWaypoint(
	selfX, selfY, selfZ int32, now time.Time, waterGuard bool,
) {
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
	if waterGuard && l.navigator != nil && l.clickWouldEnterWater(
		selfX, selfY, selfZ, moveX, moveY, moveZ) {
		return
	}
	// The server click validation runs last: the server refuses
	// whole lines its Bresenham raster walks into walled corners -
	// a refused click never moves the character.
	if l.navigator != nil && !l.clickServerValidated(
		selfX, selfY, selfZ, &moveX, &moveY, &moveZ, now) {
		return
	}
	l.moveAt = now
	if err := l.game.WalkTo(int32(moveX), int32(moveY), int32(moveZ)); err != nil {
		l.logger.Printf("Hunt: town walk request failed: %v", err)
	}
}

// clickServerValidated gates a walk click through the server
// validation port (Navigator.ValidateClick): a click the server would
// cancel never moves the character, so sending it just grinds
// ActionFailed answers until the stuck timeout fires - the town walk
// stuck of 2026-09-10 (the bot clicked the second waypoint 58 units
// over the village plaza corner, the geodata correction collapsed the
// target onto the walker and the character froze through all three
// re-paths). A refused click shortens the leg first (the Bresenham
// prefix of a split leg is not a prefix of the full raster, a shorter
// line often validates), then hops to the nearest swallowed plan bend
// (the escape step out of a trap cell), and finally falls back to the
// re-path of the stuck path. It reports whether the click target in
// the move pointers may be sent.
func (l *Loop) clickServerValidated(
	selfX, selfY, selfZ int32,
	moveX, moveY, moveZ *float64, now time.Time,
) bool {
	from := pathfind.Vec3{
		X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
	}
	if _, ok := l.navigator.ValidateClick(from,
		pathfind.Vec3{X: *moveX, Y: *moveY, Z: *moveZ}); ok {
		return true
	}
	if l.shortenClickLeg(selfX, selfY, selfZ, moveX, moveY, moveZ, from) {
		return true
	}
	if l.clickEscapeHop(selfX, selfY, selfZ, moveX, moveY, moveZ) {
		return true
	}
	// No local escape works: re-path from the current position like
	// the stuck path does, bounded by the same budget.
	l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
	l.rePaths++
	if l.rePaths > maxRePaths {
		l.abortTownTrip("the server refuses every walk click")

		return false
	}
	l.logger.Printf("Hunt: the server would refuse the walk click to "+
		"%d %d, re-pathing (%d of %d)",
		int32(*moveX), int32(*moveY), l.rePaths, maxRePaths)
	if !l.startWalkLeg(l.legDest) {
		l.abortTownTrip("re-path failed")
	}

	return false
}

// shortenClickLeg halves a refused click leg toward its target until a
// prefix validates: the split legs of a long waypoint line rasterize
// differently than the planned leg (the Bresenham accumulator starts
// from the endpoint deltas), so the far half of a line can fail where
// a shorter prefix passes. It reports whether the move pointers carry
// a validated shorter target.
func (l *Loop) shortenClickLeg(
	selfX, selfY, selfZ int32,
	moveX, moveY, moveZ *float64, from pathfind.Vec3,
) bool {
	dx := *moveX - float64(selfX)
	dy := *moveY - float64(selfY)
	dz := *moveZ - float64(selfZ)
	full := math.Hypot(dx, dy)
	for leg := full / 2; leg >= clickShortenFloor; leg /= 2 {
		frac := leg / full
		shortX := float64(selfX) + dx*frac
		shortY := float64(selfY) + dy*frac
		shortZ := float64(selfZ) + dz*frac
		if _, ok := l.navigator.ValidateClick(from, pathfind.Vec3{
			X: shortX, Y: shortY, Z: shortZ,
		}); ok {
			*moveX, *moveY, *moveZ = shortX, shortY, shortZ

			return true
		}
	}

	return false
}

// clickEscapeHop walks the nearest plan bend the arrival slack
// swallowed: the geodata holds trap cells (entered legally through an
// open wall, their own walls box the walker in - the village terrace
// rows), and the re-path plans the escape step over them; but the bend
// sits inside the waypoint arrival radius, the cursor skips it and the
// far clicks keep leaving the boxed cell. The hop clicks the bend
// directly so the character stands on it and the following clicks
// validate from there. It reports whether the move pointers carry a
// validated hop target.
func (l *Loop) clickEscapeHop(
	selfX, selfY, selfZ int32,
	moveX, moveY, moveZ *float64,
) bool {
	from := pathfind.Vec3{
		X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
	}
	for j := l.wpIndex - 1; j >= 0; j-- {
		wp := l.waypoints[j]
		dist := waypointDistance(wp, selfX, selfY, selfZ)
		if dist > waypointPassDist {
			// Deeper waypoints stand farther back along the
			// route: walking to them retraces the route
			// instead of escaping the spot.
			break
		}
		if dist <= hopCoincideDist {
			// Stood on it: no hop needed.
			continue
		}
		if _, ok := l.navigator.ValidateClick(from, pathfind.Vec3{
			X: wp.X, Y: wp.Y, Z: wp.Z,
		}); ok {
			*moveX, *moveY, *moveZ = wp.X, wp.Y, wp.Z
			l.logger.Printf("Hunt: walk click refused, hopping "+
				"back to the plan bend at %d %d",
				int32(wp.X), int32(wp.Y))

			return true
		}
	}

	return false
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
	crossed, err := l.navigator.WaterCrossed(
		pathfind.Vec3{
			X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
		},
		pathfind.Vec3{X: moveX, Y: moveY, Z: moveZ},
	)
	if err != nil || !crossed {
		// A line the geodata cannot verify stays on the old behavior:
		// the walk was planned over the same data, the drift the
		// guard exists for shows up as a wet line, not an error. The
		// check itself is water only: a click over a height step of
		// the terrain (the village deck ramps, the plaza above the
		// shops) is a normal walk the server routing handles - the
		// teacher legs of the learning trips died on the line of sight
		// half of the old dry check, which read those ramps as water
		// and aborted every trip that carried them.
		return false
	}
	l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
	l.stuckFast = false
	l.rePaths++
	if l.rePaths > maxRePaths {
		l.abortTownTrip("the walk would cross water")

		return true
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
	l.moveAt = time.Time{}
	l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
	l.stuckFast = false
	last := result.Waypoints[len(result.Waypoints)-1]
	l.logger.Printf("Hunt: character stands in the water at %d %d %d, "+
		"escaping to the shore at %d %d %d",
		selfX, selfY, selfZ, int32(last.X), int32(last.Y), int32(last.Z))

	return true
}

// legAdvanceClear reports whether the follower may advance past the
// waypoint whose successor sits at the index: the straight line from
// the CURRENT character position to that next waypoint must be
// walkable over the geodata. The server validates every ground click
// as a straight line (GeoEngine.getValidLocation, the deployment runs
// PathFinding = 0 so no server side routing exists): a click whose
// first step hits a closed wall resolves to the character's own
// position, the move is canceled at once and the character never
// moves. The old follower skipped any waypoint inside the 50 unit
// pass radius - tighter than the 16 unit ramp steps of the trainer
// plaza approach - and clicked the far waypoint straight through the
// plaza railing: the click canceled, the 15 s stuck detector
// re-planned the identical deterministic route, the follower skipped
// the same tight waypoints again and the third budget burned into
// "town trip ended: aborted, walk stuck" (the 2026-09-11 teacher
// walk, the lessons never reached the teacher). The gate keeps the
// skipped-from waypoint as the target until walking onto it re-opens
// the line. A line the geodata cannot verify stays clear - the
// follower then keeps the pre-gate behavior.
func (l *Loop) legAdvanceClear(
	selfX, selfY, selfZ int32, next int,
) bool {
	if next >= len(l.waypoints) || l.navigator == nil {
		return true
	}
	walkable, err := l.navigator.LineOfSight(
		pathfind.Vec3{
			X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
		},
		l.waypoints[next],
	)
	if err != nil {
		return true
	}

	return walkable
}

// walkStuck tracks the movement progress of the walker and re-paths
// around the obstacle once the character stands still for too long.
// The first stuck of a trip waits the full stuckTimeout (a slow server
// position broadcast must not trip a false stuck); subsequent stucks
// within the same trip wait the shorter stuckFastTimeout - once the
// walker knows the server refuses its clicks on this leg, waiting the
// full window for every waypoint just burns the trip's time budget.
// The skip of a waypoint does NOT consume the re-path budget: only the
// full leg re-plan (startWalkLeg) does. It reports whether the trip had
// to abort.
func (l *Loop) walkStuck(now time.Time, selfX int32, selfY int32) bool {
	if l.stuckAt.IsZero() {
		l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY

		return false
	}
	if selfX != l.stuckX || selfY != l.stuckY {
		l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY

		return false
	}
	timeout := stuckTimeout
	if l.stuckFast {
		timeout = stuckFastTimeout
	}
	if now.Sub(l.stuckAt) < timeout {
		return false
	}
	l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
	if l.waterEscape {
		return l.stuckWaterEscape(now, selfX, selfY)
	}

	return l.stuckTownWalk(now, selfX, selfY)
}

// stuckWaterEscape re-plans the water escape itself when the character
// stands still mid-escape. The town leg is meaningless until the
// character is back ashore. Consumes the re-path budget.
func (l *Loop) stuckWaterEscape(now time.Time, selfX int32, selfY int32) bool {
	l.rePaths++
	if l.rePaths > maxRePaths {
		l.abortTownTrip("walk stuck")

		return true
	}
	l.logger.Printf("Hunt: water escape stuck, re-planning "+
		"(%d of %d)", l.rePaths, maxRePaths)
	if !l.planWaterEscape(selfX, selfY, l.selfZForEscape()) {
		l.abortTownTrip("water escape re-plan failed")

		return true
	}

	return false
}

// stuckTownWalk drives the town leg stuck recovery: first try to SKIP
// the current waypoint (the next one may be reachable through a cell
// the server accepts), and when no more waypoints remain to skip,
// re-plan the whole leg from the current position. The skip does NOT
// consume the re-path budget - it advances the cursor without
// re-planning, so the walker can skip several waypoints in a row while
// looking for one the server accepts. The fast timeout flag arms
// after the first skip so subsequent stuck detections fire on the
// shorter window.
func (l *Loop) stuckTownWalk(now time.Time, selfX int32, selfY int32) bool {
	if l.wpIndex+1 < len(l.waypoints) {
		l.wpIndex++
		l.moveAt = time.Time{}
		l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
		l.stuckFast = true
		l.logger.Printf("Hunt: town walk stuck, skipping waypoint "+
			"(cursor %d of %d)", l.wpIndex, len(l.waypoints))

		return false
	}
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
//
// The approach walk clicks the ground at the npc approach point, not
// at the merchant's exact cell: the server's getValidLocation walks a
// Bresenham line that can "step over" onto a roof layer when the
// click targets an interior cell (the 2026-09-11 roof teleport
// report), so the offset keeps the click line on the surrounding deck.
//
// The merchant select fires as soon as the bot is within the server
// interaction distance (npcInteractionDist = 250 in 3D), even when
// the z gap keeps the dist3D above the approach gate (200) - see
// approachTeacher for the same fix and the 2026-09-11 05:45 dump.
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
	// The merchant select fires within the server interaction distance
	// (250 in 3D) even when the approach gate (200) is not met: the
	// offset ring lands the bot at ~150 units 2D from the npc, and a
	// small z gap keeps dist3D above 200 but within 250. Without this
	// early return the bot looped on the offset ring forever (the
	// 2026-09-11 05:45 dump).
	if dist3D <= npcInteractionDist {
		return l.selectMerchant(now)
	}
	if dist3D > merchantApproachDist {
		ax, ay, az := npcApproachPoint(x, y, z, selfX, selfY)
		approachDist2D := math.Hypot(
			float64(ax-selfX), float64(ay-selfY))
		if dist2D <= merchantApproachDist {
			// The geodata pack misses some village ramps: the character
			// stands under the merchant deck (the 2D distance is met,
			// the z is not). The approach point collapses onto the
			// bot's own cell - the click is a no-op the server
			// collapses, the deck window bounds the wait before the
			// merchant is given up. Clicking the merchant's exact cell
			// here teleported the bot onto the roof (the 2026-09-11
			// report), so the offset keeps the click safe even when it
			// cannot help.
			if l.merchantDeckUntil.IsZero() {
				l.merchantDeckUntil = now.Add(merchantDeckWindow)
				l.logger.Printf("Hunt: %s stands on another deck (z %d vs "+
					"%d), re-walking by server routing",
					l.tracker.ObjectName(l.merchantID), selfZ, z)
			}
			if now.Before(l.merchantDeckUntil) {
				if approachDist2D > hopCoincideDist {
					l.walkToward(ax, ay, az, now)
				}

				return false
			}
			l.logger.Printf("Hunt: %s stays out of reach, the sells work "+
				"without it and its buys are skipped",
				l.tracker.ObjectName(l.merchantID))
			l.merchantID = -1

			return true
		}

		// Far away on the same level: a plain approach walk to the
		// offset point, not the merchant's exact cell. The dist3D <=
		// npcInteractionDist early return above takes over once the bot
		// arrives at the offset ring (the talk click lands from the ring
		// even with a small z gap).
		if approachDist2D > hopCoincideDist {
			l.walkToward(ax, ay, az, now)
		}

		return false
	}
	// dist3D in (merchantApproachDist, npcInteractionDist]: the bot is
	// on the offset ring, the merchant select proceeds.
	return l.selectMerchant(now)
}

// selectMerchant re-requests the merchant selection once per select
// period until the tracker confirms it, then reports ready. The
// transactions need the merchant as the selected target
// (RequestBuyItem checks it server side).
func (l *Loop) selectMerchant(now time.Time) bool {
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
	// A bare-handed character with an affordable weapon keeps walking
	// home: the zone entry fight would farm with the fists, and the
	// weapon run owns the next ticks anyway (the trip end arms the
	// short weapon run cooldown).
	if l.weaponlessRunWanted() {
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

// abortTownTrip finishes a failed trip with a log line. A deleveling
// walking through the shared machinery aborts the deleveling itself:
// the plain trip end would leave the delevel state armed without a
// cooldown, and the next tick restarted the walk into the same
// blocker - the reported bot hung cycling "deleveling to 9" and
// "the walk would cross water" forever (the 2026-09-10 state dump).
func (l *Loop) abortTownTrip(reason string) {
	if l.phase == phaseDelevel {
		l.abortDelevel(reason)

		return
	}
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
