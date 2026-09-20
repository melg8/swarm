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
    "strings"
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
    // sellPause paces the sell requests through the shared
    // transaction window of the server flood protector (see
    // transactionPause in shopping.go for the server research: the
    // window is 1 s wide, refusals cost nothing, the 11 second
    // margin of the early rounds made the junk selling crawl).
    sellPause = transactionPause
    // walkRequestPeriod paces the ground click walks of the waypoint
    // follower and the merchant approach.
    walkRequestPeriod = 2 * time.Second
    // refusalAnswerWindow bounds how long after a walk click the
    // ActionFailed answer still counts as ITS answer: the server
    // answers a refused request within a moment, so a refusal hours
    // old is not the answer to the click just sent. The window
    // covers the answer of the previous click too (a slow answer
    // racing the next request of the walk request period).
    refusalAnswerWindow = 4 * time.Second
    // waypointArriveDist is the distance within which the FINAL
    // waypoint of a walk plan counts as reached: the wide trip
    // arrival radius (two geodata cells of slack) so a segment ends
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
    // maxMoveDistance splits the walk segments: the server refuses
    // move requests with a target farther than 9900 units
    // (MoveToLocation readImpl),
    // and the smoothed geodata paths happily produce longer segments over
    // the open terrain.
    maxMoveDistance = 1000.0
    // stuckTimeout is how long the character may stand still on a segment
    // before the walker re-paths around the obstacle.
    stuckTimeout = 15 * time.Second
    // stuckFastTimeout is the shorter timeout that applies after the
    // first stuck skip of a trip: once the walker knows the server
    // refuses its clicks on this segment, waiting the full 15 seconds for
    // every subsequent waypoint just burns the trip's time budget. The
    // shorter window keeps the recovery responsive while still letting
    // a slow server position broadcast land before the next skip.
    stuckFastTimeout = 4 * time.Second
    // moveStartWindow bounds the move start watchdog: a walk click
    // whose movement never STARTED (no server movement broadcast, no
    // position change, no refusal answer) fast forwards the stuck
    // recovery after this window instead of waiting the full stuck
    // timeout - the owner rule of the 2026-09-19 round: a movement
    // command that did not start the movement switches the recovery
    // mode at once, the walker never stands out a whole window on a
    // click it can already name dead. The value covers the server's
    // once per second movement broadcast gate plus the network lag
    // with head room, and stays under the fast stuck window so the
    // never-started verdict always lands first.
    moveStartWindow = 3 * time.Second
    // stuckProgressUnits is the net progress margin of the stuck
    // window: the current waypoint must come closer by this many
    // units for the movement to count as progress. A walk covers
    // hundreds of units over the stuck window whatever its speed,
    // so the margin only filters the wobble (an oscillation that
    // bounces between two tangent endpoints never improves its
    // best distance by it).
    stuckProgressUnits = 50.0
    // tripAbortEscalateAfter is the abort streak the trip cooldown
    // tolerates before it starts doubling: two identical aborts can
    // be transient (a mob camped on the shore line, a slow server),
    // a third names a deployment the retry cannot fix.
    tripAbortEscalateAfter = 2
    // tripAbortMaxCooldown caps the escalation of the abort streak.
    tripAbortMaxCooldown = time.Hour
    // maxRePaths bounds the re-paths of one trip before it aborts.
    // The budget bounds only the full segment re-plan (startWalkSegment) - the
    // waypoint skip of walkStuck does NOT consume it. This lets the
    // walker cycle through several waypoints looking for one the server
    // accepts without exhausting the budget, while still bounding the
    // expensive re-plan operations.
    maxRePaths = 3
    // minWalkClick is the floor length of the ground clicks the
    // waypoint follower sends. The server's own move validation can
    // collapse a click onto the walker (the GeoEngine.getValidLocation
    // correction), and Creature.moveToLocation only hands such a
    // collapsed click over to the server side pathfinder when the
    // ORIGINAL line was longer than 30 units: the pathfinding branch
    // gates on (originalDistance - distance) > 30, so a shorter
    // collapse is silently canceled with ActionFailed and the
    // character never moves - the click stays eligible for the
    // rescue only above the floor (the 2026-09-11 11:34 village
    // return dump: the first plan waypoint sat 22 units out, every
    // re-click of it froze through two whole trip cycles). The floor
    // matches waypointPassDist: an intermediate waypoint under it
    // counts as arrived, the follower only CLICKS one when the
    // cursor is pinned on it (no clear successor line), and the
    // pinned click then extends past it along the plan polyline.
    minWalkClick = 50.0
    // frozenRepathLimit bounds the consecutive stuck re-paths that
    // start from the same cell without a single cell of movement in
    // between: the re-path re-plans from the standing position, so
    // a re-path that itself produced no movement proves the fresh
    // plan cannot move the character either (the refusal the
    // offline validation cannot see). The next identical re-path
    // would burn a full stuck window on the same freeze - the trip
    // aborts and hands the recovery to its callers instead (the
    // zone return escalates to the direct server routed segments at
    // once, the shop trip arms its cooldown).
    frozenRepathLimit = 1
    // refusalVariantsMax bounds the varied aim attempts one segment
    // spends on the online refusal answer (see stuckTownWalk): the
    // server refuses a click for its own reasons (a different build
    // or geodata validates the same line differently), and the
    // refusal is target specific - a shorter prefix or a sideways
    // offset of the same waypoint often walks where the plain aim
    // bounced. The variants are cheap (one click each, no re-plan),
    // so a small ladder covers the common target specific refusals
    // before the segment falls back to the re-path machinery.
    refusalVariantsMax = 4
    // refusalVariantStep is the sideways offset of the perpendicular
    // refusal variants: about twelve geodata cells off the route
    // line, far enough to bend the click raster away from the walled
    // flank the server refused, close enough to stay on the same
    // walkable deck the plan already verified.
    refusalVariantStep = 192.0
    // cursorEscapeStep is one claimed step of the cursor key escape:
    // about one second of run speed, the cadence the official client
    // streams its own movement simulation at while the player walks
    // with the arrows - the claim pace of the escape matches the
    // client the server already trusts.
    cursorEscapeStep = 144.0
    // cursorEscapePeriod paces the claimed steps of the cursor key
    // escape: one claim per second, the official client cadence of
    // the ValidatePosition stream (far under every flood protector
    // threshold of the reference server).
    cursorEscapePeriod = time.Second
    // cursorEscapeFollowClaims bounds the patience of the escape's
    // follow probe: this many claims with no server position change
    // prove the server ignores the stream (the keyboard movement
    // disabled, a build without the cursor key branch of
    // ValidatePosition), and the trip ends with the honest reason
    // instead of grinding claims into the silence.
    cursorEscapeFollowClaims = 5
    // cursorEscapeSettle bounds the wait for the position broadcasts
    // after the last claimed step: the server answers each claim
    // with a ValidateLocation broadcast, and the settle window lets
    // the tracker catch up before the segment resumes its clicks.
    cursorEscapeSettle = 2 * time.Second
    // cursorEscapeAttemptsMax bounds the cursor key escape attempts
    // of one town trip: a pocket that keeps refusing the clicks
    // after the escapes walks a wider problem than the escape
    // exists for, the honest abort owns it.
    cursorEscapeAttemptsMax = 3
    // cursorEscapeFollowStep is the movement margin that proves the
    // server follows the claims: any drift past it from the escape
    // origin re-arms the follow patience (the character moved - the
    // claims own it).
    cursorEscapeFollowStep = 64.0
    // cursorEscapeRouteMax caps the planned route length one escape
    // walks: the route-following claims follow the plan polyline
    // (see cursorEscapeRouteSteps) and the cap keeps the escape a
    // pocket recovery - the walk along the route ends at the cap
    // and the clicks resume from there, the escape never walks the
    // character across the whole map on claims alone. The planless
    // aim of the straight fallback is clamped to the same radius
    // (see beginCursorKeyEscape): a far target can never pull a
    // straight march out of the escape.
    cursorEscapeRouteMax = 2500.0
    // extendMarchStep is the stride of the forward route march of
    // extendShortClickCandidates: one geodata cell.
    extendMarchStep = 16.0
    // extendCandidateMax bounds the forward route samples the
    // short click extension tries: the first samples past the
    // floor, each one march step further along the route - enough
    // to step past a single route cell that walls the chord.
    extendCandidateMax = 5
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
    // npcApproachRadius is the geodata search radius of every npc
    // destination walk (the merchant stops, the teacher stops, the
    // delevel guard walks): the plan must end at the npc's own point -
    // the closest walkable surface of it (the customer cell across the
    // counter, the hall row beside the master) - at most this distance
    // short of it. A wide radius ends the plan at the FIRST walkable
    // surface inside its ball instead - the shop edge, up to the whole
    // radius away - and the character never enters the shop (the
    // 2026-09-20 report: the wide ring plans held the shop edge while
    // the requested point sat inside it). The merchant cells without a
    // modeled floor layer or behind a counter stay reached through the
    // partial answer: the search walks the corridor to the closest
    // reachable point and the funnel ends there (the water deck below
    // the shop - far in z - never satisfies the radius).
    npcApproachRadius = 10.0
    // tripApproachRadius is the geodata search radius the non npc trip
    // walks (the farm spot return, the zone return) end within: the
    // walk home ends wherever the ball around the spot catches the
    // route, the hunt tick corrects the rest on the spot.
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
    // instead of crawling micro segments.
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
    // exactSegmentDeckTolerance bounds the z gap between the merchant
    // spawn z and the plan end cell of the exact merchant segment: the
    // customer cell sits on the merchant's own floor scale (the elven
    // stall fronts answer 0-3 units off, the trainer hall decks 40),
    // a roof layer over the shop sits hundreds above - the foreign
    // deck plan is discarded for the ring fallback instead of walked.
    exactSegmentDeckTolerance = 96.0
    // exactSegmentMaxDistance bounds how far from the merchant the exact
    // segment may plan: the exact search serves the final approach (the
    // customer cell across the counter), the long haul belongs to the
    // priced ring search whose hierarchy answers the reachable far
    // routes in milliseconds. An exact search at a FAR unreachable
    // destination degenerates into an exhaustive exploration of the
    // whole mesh component (the Herbiel segment of the farm readiness
    // round froze the live bot for minutes inside one query) - the
    // distance gate keeps the exact search inside the neighborhood
    // where its partial exhaustion stays local and bounded.
    exactSegmentMaxDistance = 2000.0
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

// dionMerchants are the shop merchants of the Town of Dion the 20-25
// band shopping trip targets, from docs/band_20_25_survey.md. The
// coordinates are the spawn positions of spawns/Dion/DionNPCs.xml;
// TemplateID is the CT0-to-C4 display id (the npcdata.npcBuyLists map
// resolves their buylists). The elven village merchants stay the
// default of the 1-19 band; the hunt loop switches to the Dion
// merchants when the active zone region is Dion (see
// shopCatalogForRegion).
var dionMerchants = []townNpc{
    {TemplateID: 7060, Name: "Sabrin", X: 17999, Y: 144484, Z: -3048},
    {TemplateID: 7061, Name: "Casey", X: 17948, Y: 144560, Z: -3048},
    {TemplateID: 7062, Name: "Sonia", X: 19313, Y: 146229, Z: -3048},
    {TemplateID: 7063, Name: "Lara", X: 19223, Y: 146228, Z: -3048},
}

// merchantStands are the customer stand points of the merchants that
// trade behind a counter (the curated rows of the counter round: the
// elven village and the Dion quarter). The merchant spawn itself is no
// stand point: the spawn cell sits inside the roofed stall (the
// geodata pack models the stall interior as roof-only cells over the
// missing floor - the cell answers blocked, nobody can stand on it
// directly, the user report of the shop quarter round), and the mesh
// route to the spawn ends wherever the nearest floor poly happens to
// sit - for Unoren that was the outer side of the stall front 42 units
// north-west, not the customer side. The stand is the cell just
// beyond the counter front along the merchant facing heading (the
// counter sits on the facing side of the stall, the customer cell on
// the other side of it), derived and verified with cmd/counterprobe
// (-mode detect names the counter direction, -mode stands and
// -mode scan pin the cell: every entry routes found with the plan
// ending exactly on the cell, inside the interaction distance of the
// spawn). A corrected or a new entry is one table row plus a rerun
// of the probe; the z of every entry is the pack floor of the cell
// so the exact search resolves it onto the right deck. The world
// wide coverage of the same machinery rides the generated
// merchantStandsWorld table (merchant_stands_world.go).
var merchantStands = map[int32]pathfind.Vec3{
    // Unoren, the customer corridor west of the counter front, at the
    // corridor gate latitude (the gate cell row the east approach
    // lines pass through; deeper rows clip the walled counter corner
    // on the diagonal strides).
    7147: {X: 44584, Y: 46944, Z: -2984},
    // Ariel, the same corridor at her row.
    7148: {X: 44584, Y: 46952, Z: -2984},
    // Creamees, the open floor beyond the counter front south-east.
    7149: {X: 42727, Y: 50115, Z: -2984},
    // Herbiel, the open floor beyond the counter front south-east.
    7150: {X: 42798, Y: 50101, Z: -2984},
    // Sabrin, the customer corridor east of the counter front.
    7060: {X: 18072, Y: 144488, Z: -3040},
    // Casey, the same corridor at her row.
    7061: {X: 18044, Y: 144560, Z: -3040},
    // Sonia, the customer corridor north of the counter front.
    7062: {X: 19320, Y: 146168, Z: -3064},
    // Lara, the same corridor at her row.
    7063: {X: 19224, Y: 146168, Z: -3064},
}

// merchantStandPoint returns the customer stand point of the merchant
// when a stand table knows it, the spawn point otherwise. The curated
// counter table answers first (the hand pinned elven and Dion rows),
// the generated world table second (the counterprobe world pass rows
// of merchant_stands_world.go) and the spawn fallback covers the
// spawns the pack serves directly plus the flat placeholder regions.
// The walk planning of the merchant stops targets the stand (the
// character stops face to face with the merchant across the counter);
// the interaction and approach gates keep measuring the spawn (the
// stand sits 64-80 units from it, well inside the 250 interaction
// distance).
func merchantStandPoint(npc townNpc) pathfind.Vec3 {
    if stand, ok := merchantStands[npc.TemplateID]; ok {
        return stand
    }
    if stand, ok := merchantStandsWorld[npc.TemplateID]; ok {
        return stand
    }

    return townNpcPosition(npc)
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
    // FindPathApproachAvoiding plans the walk around the given avoid
    // areas: the composition layer's avoid seam (the mesh ban walls
    // with the escape ring of the own ban). The hunt loop's frozen
    // corridor bans were its historical consumer - the plaza round of
    // 2026-09-20 removed them (the rectangle granularity sealed whole
    // mesh sheets), the mesh level capability stays for the library
    // contract.
    FindPathApproachAvoiding(
        start, end pathfind.Vec3, approachRadius float64,
        avoid []pathfind.AvoidArea,
    ) (*pathfind.Result, error)
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
    // over a lake or sea bed (swimming or floating on it). The frame
    // measurement arms on it - a swimming character measures no
    // vintage shift (see click_frame.go).
    OverWater(x, y float64, refZ int16) bool
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

// FindPathApproachAvoiding searches the path around the avoid areas
// with the engine settings and the approach radius goal.
func (e engineNavigator) FindPathApproachAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    return e.engine.FindPathApproachAvoiding(
        start, end, approachRadius, e.engine.MaxPassableHeight(), avoid)
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

// ValidateClick mirrors the server move validation with the engine.
func (e engineNavigator) ValidateClick(
    from, to pathfind.Vec3,
) (pathfind.Vec3, bool) {
    return e.engine.ValidateClick(from, to)
}

// merchantsForRegion returns the shop merchants of the region the
// loop farms near: the elven village set is the default and the Dion
// set serves the 20-25 band region. The same set builds the region
// shop catalog (see shopCatalogForRegion), so the trip targets, the
// sell stop pick and the purchase plan always name the same town's
// traders.
func merchantsForRegion(region string) []townNpc {
    if region == regionDion {
        return dionMerchants
    }

    return townMerchants
}

// nearestMerchant returns the town merchant of the active region
// closest to the point.
func (l *Loop) nearestMerchant(
    selfX int32, selfY int32,
) (townNpc, bool) {
    best := zeroTownNpc
    bestDist := math.MaxFloat64
    found := false
    for _, merchant := range merchantsForRegion(l.zoneRegion) {
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

// merchantTemplates lists the packet template ids of the active
// region's town merchants: the sell stop pick accepts any of them
// (every vendor accepts the sale of any sellable item), while the buy
// stops name their own trader through stopMerchantTemplates.
func (l *Loop) merchantTemplates() []int32 {
    return townMerchantTemplates(merchantsForRegion(l.zoneRegion))
}

// townMerchantTemplates lists the packet template ids of the merchant
// set (the NpcInfo tracker ids carry the display id offset).
func townMerchantTemplates(merchants []townNpc) []int32 {
    templates := make([]int32, 0, len(merchants))
    for _, merchant := range merchants {
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
// bare-handed character with an affordable weapon and a character
// with an armed gear debt (a slot the last trip stranded) retries
// on the short gear run cooldown: punching mobs through the five
// minute cooldown of an ordinary trip is the exact outcome the
// weapon run exists to prevent, and farming without the armor the
// merchant sold is the same wound (the 2026-09-12 04:58 dump: the
// level 14 fighter farmed the Kaboo woods without its legs armor
// through the whole cooldown window).
func (l *Loop) tripCooldownOver() bool {
    if l.tripEndedAt.IsZero() {
        return true
    }
    cooldown := tripCooldown
    if l.weaponlessRunWanted() || l.gearDebtRunWanted() {
        cooldown = weaponRunCooldown
    } else if l.tripAbortRun > tripAbortEscalateAfter {
        // The abort streak escalates: a trip that keeps failing
        // the same way keeps failing it for a reason no retry
        // fixes (no walkable path, no server routing), and the
        // flat cooldown just burns the ticks between the
        // identical aborts.
        cooldown = tripAbortCooldown(l.tripAbortRun)
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
//
//nolint:cyclop,funlen,gocognit // learning joined
func (l *Loop) maybeStartTownTrip() {
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
    // The trip plan freezes here, ONCE: the purchases the planner
    // picked against the current gear and adena are exactly what
    // this trip sells and buys - the sell first step banks their
    // SellFirst credits, the stop planning distributes the purchases
    // and nothing re-plans in between (a re-plan at the shop ran
    // against the freed slots and the fresh adena and drifted: it
    // re-bought the piece the trip had just sold and planned
    // purchases whose displaced pieces were never queued - the
    // 2026-09-11 two pairs of gloves report).
    l.tripPlan = l.shoppingPlan()
    // The gear the trip starts with is the baseline the trip exits
    // compare against: a slot the trip empties without landing the
    // replacement becomes gear debt (see gearDebtCheck).
    l.snapshotTripGear()
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
    l.segmentRefused = false
    l.refusalVariants = 0
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
    l.segmentRefused = false
    l.refusalVariants = 0
    l.resetReplacementSales()
    l.resetLearnState()
    // The learning stops no longer ride the trip start: they plan
    // AFTER the gear stops at the sell stop (see tickTownSell), so
    // one town visit buys the weapon, the armor, the jewels, the
    // books and teaches the lessons - the user rule of the one
    // town visit. The weapon stop still runs FIRST (the sell stop
    // routes to the weapon merchant), so a stuck teacher segment can
    // no longer strand a bare-handed character: the weapon is
    // bought and worn before the teacher segment ever runs.
    l.phase = phaseTownWalk
    stats := l.tracker.InventoryStats()
    reason := "inventory at " + strconv.Itoa(stats.Slots) + " slots and " +
        strconv.FormatFloat(stats.WeightPercent, 'f', 0, 64) +
        "% weight"
    if !l.inventoryFull() {
        reason = "the shop strategy plans purchases worth " +
            strconv.FormatInt(gear.AdenaSpent(l.tripPlan), 10) +
            " adena"
    }
    if weaponRun {
        // The bare-handed errand names itself: the 2 damage punches of
        // the dump report read at a glance in the log tail.
        reason = "no weapon in hand, the weapon run comes first"
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
    if l.gearDebtRunWanted() {
        // The refill names itself: the stranded slot of the dump
        // report reads at a glance in the log tail.
        reason += " (the gear debt refill)"
    }
    // The trigger plan cache drops: the frozen trip plan owns the
    // trip now, the cache only feeds the widget view between the
    // recomputes.
    l.shoppingPlanCache = nil
    l.shoppingPlanAt = time.Time{}
    l.shoppingPlanAdena = 0
    if l.journal != nil {
        l.journal.TripStart(l.tracker.ID(), reason)
    }
    l.logf("Hunt: %s, walking to the trader %s", reason,
        merchant.Name)
    // The first stop walks the exact mesh search when the merchant is
    // near enough (the same authority the later stops of
    // advanceTripStop plan with): the approach radius ends the plan on
    // the first deck polygon inside its radius - outside the shop, on
    // the outer railing side - and the talk fires from there (or
    // slides along the railing forever). The exact search lands on
    // the customer cell across the counter; the npc segment fallback
    // runs for the one failure class the exact search cannot answer -
    // the plan that resolved onto a foreign deck (the roof over the
    // shop), see startWalkExactSegment. The far merchants walk the npc
    // segment (the npcApproachRadius long haul): the plan ends at the
    // stand point - the requested cell inside the shop - instead of
    // the shop edge a wide ring caught.
    planned := false
    if l.merchantWithinExactRange(merchant) {
        var ringFallback bool
        planned, ringFallback = l.startWalkExactSegment(
            merchantStandPoint(merchant))
        if !planned && ringFallback {
            planned = l.startWalkNpcSegment(merchantStandPoint(merchant))
        }
    }
    if !planned {
        if !l.startWalkNpcSegment(merchantStandPoint(merchant)) {
            l.abortTownTrip("no walkable path to the shop")
        }
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
        l.logf("Hunt: walk request failed: %v", err)
    }
}

// exactApproachWanted reports whether the completed town walk segment
// deserves the exact final approach: the current stop is a merchant
// stop whose segment this walk just finished (the segment destination IS the
// merchant spawn), the segment itself was the ring approach (the exact
// segments arrive at the customer cell and need no second pass), and the
// merchant sits beyond the customer ring but inside the exact
// distance gate - near enough for the bounded search, far enough
// that the talk would fire from the ring edge instead of the
// counter.
func (l *Loop) exactApproachWanted() bool {
    if len(l.tripStops) == 0 || l.tripStops[0].teach {
        return false
    }
    merchant := l.tripStops[0].merchant
    if l.segmentDest != merchantStandPoint(merchant) {
        return false
    }
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        return false
    }
    if !l.merchantWithinExactRange(merchant) {
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dx := float64(merchant.X - selfX)
    dy := float64(merchant.Y - selfY)
    dz := float64(merchant.Z - selfZ)

    return math.Sqrt(dx*dx+dy*dy+dz*dz) > npcApproachOffset
}

// stopNpcArrivalMet reports whether the running walk segment has
// brought the character close enough for the current merchant stop's
// talk machinery to take over: the character stands within the wide
// arrive radius of the segment destination (the customer stand point)
// AND within the server interaction distance of the stop's merchant
// npc. The tight verification of the plan's final cell can grind
// forever against the vintage geodata disagreement - the live server
// resolves the clicked destination onto its own surface (the pack
// holds cells 64 units off the live floor, the creation building
// interior among them) - and the stuck ladder then burns the whole
// escalation down to the trip abort while the character stands ready
// to trade: the stop exists to bring the talk within reach, not to
// verify the pack's cell. The teach stops keep their own walk
// contract (the hall rows need the real arrival), the non merchant
// segments answer false.
func (l *Loop) stopNpcArrivalMet() bool {
    if len(l.tripStops) == 0 || l.tripStops[0].teach {
        return false
    }
    dest := l.segmentDest
    if dest != merchantStandPoint(l.tripStops[0].merchant) {
        // The segment walks something else (the far haul leg, the
        // return): the npc handoff owns the merchant stop walks only.
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dx := dest.X - float64(selfX)
    dy := dest.Y - float64(selfY)
    dz := dest.Z - float64(selfZ)
    if math.Sqrt(dx*dx+dy*dy+dz*dz) > waypointArriveDist {
        return false
    }
    npc, ok := l.tracker.NearestNpcByTemplates(
        l.stopMerchantTemplates(), merchantFindRadius)
    if !ok {
        return false
    }
    dx = float64(npc.X - selfX)
    dy = float64(npc.Y - selfY)
    dz = float64(npc.Z - selfZ)

    return math.Sqrt(dx*dx+dy*dy+dz*dz) <= npcInteractionDist
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
            // The completed segment hands over to the talk machinery -
            // unless the segment was the long haul ring approach and the
            // merchant is still beyond the customer ring: the exact
            // final approach segment arms here (near enough now for the
            // bounded search) and walks the character to the
            // customer cell across the counter (the shop quarter
            // round of the user report: the talk fired - or bounced -
            // from the outer railing side instead).
            planned, _ := l.startWalkExactSegment(l.segmentDest)
            if !l.exactApproachWanted() || !planned {
                l.enterSellPhase()
            }

            return
        }
        // The interaction handoff of the merchant stops: the plan's
        // final cell verification may grind against the vintage
        // geodata disagreement while the character already stands
        // within reach of the merchant it walked to (see
        // stopNpcArrivalMet) - the talk machinery owns the rest.
        if l.stopNpcArrivalMet() {
            l.logf("Hunt: the merchant is within the interaction " +
                "distance, handing the walk to the talk")
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

// startWalkSegment plans the walk to the destination and arms the waypoint
// follower. The search prices the water at the swim rate (swimming is
// slower than running): the plan crosses a lake only when the crossing
// is the genuinely faster walk, and the follower walks the wet segments it
// planned - the walker and the planner share one water policy. The
// search goal is the approach radius of the destination (the merchant
// interaction distance): a destination behind a counter or on a floor
// layer the geodata does not model is still reached on the surrounding
// deck. A destination no route reaches reports false - the callers
// abort the trip and arm their cooldowns. The planning position
// publishes as the segment origin of the walk plan view - the dump shows
// the whole walk from it. It reports whether the segment was planned.
func (l *Loop) startWalkSegment(dest pathfind.Vec3) bool {
    return l.startWalkSegmentSearch(dest)
}

// startZoneReturnSegment plans the zone return walk through the priced
// approach search: the water crossings compete with the land detours
// on the honest travel time. The zone return must bring the bot home
// whenever any route exists (the 2026-09-11 06:00 dump: the search
// reported no route
// from 43000 50184 to both the zone center and Herbiel 276 units
// away, while the offline probe against the same geodata found both
// paths - the runtime difference is unresolved, the priced search
// with its partial corridor answer gives the bot the walkable route
// toward home in every case the corridor exists). It reports whether
// the segment was planned.
func (l *Loop) startZoneReturnSegment(dest pathfind.Vec3) bool {
    return l.startWalkSegmentSearch(dest)
}

// segmentSearchView freezes one mesh search contract into the walk plan
// view: the approach radius the search ran with.
func segmentSearchView(approach float64) *state.WalkSearch {
    return &state.WalkSearch{Approach: approach, Avoid: nil}
}

// startWalkSegmentSearch plans the walk to the destination through the
// priced approach search and arms the waypoint follower. The water
// crossings pay the swim rate (swimming is slower than running), so
// the plan prefers the land detours whenever they are the faster walk
// and swims whenever the water cut wins - the follower walks the wet
// segments the plan carries. It reports whether the segment was
// planned.
func (l *Loop) startWalkSegmentSearch(dest pathfind.Vec3) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    radius := l.segmentRadius
    if radius <= 0 {
        radius = tripApproachRadius
    }
    result, err := l.navigator.FindPathApproach(from, dest, radius)
    if err != nil {
        l.logf("Hunt: town trip path search failed: %v", err)

        return false
    }
    if result == nil || len(result.Waypoints) == 0 {
        l.logf("Hunt: no path to %d %d at all",
            int(dest.X), int(dest.Y))

        return false
    }
    if !result.Found {
        // The partial round (docs/navmesh.md): both engines agree the
        // destination is unreachable under the filter, and the mesh
        // funnel still holds the walkable corridor toward it - the
        // segment walks the closest reachable point instead of aborting
        // at the start position, so the trip continues from wherever
        // the ground ends (the shore of a swim-only destination, the
        // border of the sealed corridor).
        l.logf("Hunt: no route to %d %d, walking the closest "+
            "reachable point", int(dest.X), int(dest.Y))
    }

    return l.armTownWalkSegment(selfX, selfY, selfZ, from, dest, result,
        radius)
}

// startWalkNpcSegment plans the npc destination walk through the two
// rung ladder of the npc stops. The npc rung searches with the npc
// search radius (npcApproachRadius - the plan must end at the npc's
// own point, not at the first walkable surface a wide ball catches)
// and refuses the found plan that resolved onto a foreign deck: a
// connected roof over the shop answers the destination cell's
// closest-layer resolution hundreds of units above the npc floor, and
// the roof walk is the 2026-09-11 teleport geometry the deck tolerance
// exists to stop (the exact planner applies the same rule, see
// startWalkExactSegment). The wide rung searches with the wide trip
// ring and arms whatever walkable deck the ball catches: the
// conservative stop short of the npc for the destinations whose own
// point the mesh or the geodata cannot deliver (the roof resolution,
// the merchant cell the pack models as the water bed below the shop)
// - the offset click window of the talk machinery owns the last
// stretch there. It reports whether the segment was planned.
func (l *Loop) startWalkNpcSegment(dest pathfind.Vec3) bool {
    if l.planNpcSegment(dest) {
        return true
    }
    l.segmentRadius = tripApproachRadius

    return l.startWalkSegment(dest)
}

// planNpcSegment is the npc rung of the npc stop ladder: the npc
// search radius plan, armed only when it ends on the npc's deck (the
// found answers; the partial answers always arm - the closest
// reachable point is the walk-what-you-can contract, the trip
// continues from wherever the ground ends). It reports whether the
// segment was planned.
func (l *Loop) planNpcSegment(dest pathfind.Vec3) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    l.segmentRadius = npcApproachRadius
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    result, err := l.navigator.FindPathApproach(
        from, dest, npcApproachRadius)
    if err != nil {
        l.logf("Hunt: town trip npc path search failed: %v", err)

        return false
    }
    if result == nil || len(result.Waypoints) == 0 {
        l.logf("Hunt: no npc path to %d %d at all",
            int(dest.X), int(dest.Y))

        return false
    }
    if result.Found {
        last := result.Waypoints[len(result.Waypoints)-1]
        if dz := math.Abs(last.Z - dest.Z); dz > exactSegmentDeckTolerance {
            // The plan resolved onto a foreign deck: the roof over
            // the shop. Walking it would click the roof the server
            // resolves onto the character (the 2026-09-11 teleport
            // geometry) - the npc rung refuses, the wide rung stops
            // on the surrounding deck instead.
            l.logf("Hunt: npc route to %d %d lands %.0f units off "+
                "the npc deck, refusing the plan",
                int(dest.X), int(dest.Y), dz)

            return false
        }
    } else {
        // The partial round (docs/navmesh.md): the destination is
        // unreachable under the filter and the funnel still holds the
        // walkable corridor toward it - the segment walks the closest
        // reachable point (the customer cell across the counter).
        l.logf("Hunt: no npc route to %d %d, walking the closest "+
            "reachable point", int(dest.X), int(dest.Y))
    }

    return l.armTownWalkSegment(selfX, selfY, selfZ, from, dest, result,
        npcApproachRadius)
}

// merchantWithinExactRange reports whether the merchant is close
// enough for the exact segment: the distance gate of exactSegmentMaxDistance
// (see the constant comment - the far exact searches are the
// exhaustive flood class).
func (l *Loop) merchantWithinExactRange(npc townNpc) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dx := float64(npc.X - selfX)
    dy := float64(npc.Y - selfY)
    dz := float64(npc.Z - selfZ)

    return math.Sqrt(dx*dx+dy*dy+dz*dz) <= exactSegmentMaxDistance
}

// replanTownWalkSegment re-plans the current town segment from the standing
// cell to its own destination, preserving the segment's search contract:
// the exact segments re-plan the exact search (a stuck merchant walk must
// re-arm the customer cell plan, not degrade into the approach ring
// that ends the plan outside the shop again), the npc segments re-plan
// the npc search with its foreign deck refusal, the return segments
// re-plan with their own wide radius. It reports whether the segment
// was planned.
func (l *Loop) replanTownWalkSegment(dest pathfind.Vec3) bool {
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        planned, ringFallback := l.startWalkExactSegment(dest)
        if planned || !ringFallback {
            return planned
        }

        return l.startWalkNpcSegment(dest)
    }
    if l.segmentSearch != nil &&
        l.segmentSearch.Approach == npcApproachRadius {
        return l.startWalkNpcSegment(dest)
    }

    return l.startWalkSegmentSearch(dest)
}

// startWalkExactSegment plans the walk to the exact destination through
// the mesh corridor search (approach zero - the plan must arrive at
// the destination cell, whatever walkable cell the mesh resolves it
// onto) and arms the waypoint follower. The merchant stops navigate
// with it: the approach ring catches the first deck polygon inside
// its radius and ends the plan short - the shop quarter round of the
// user report held the wide ring plans 147-232 units from the
// merchant, on the outer railing side of the stall, and the talk
// fired from there (or slid along the railing forever). The exact
// search answers the customer cell across the counter instead: the
// merchant's own cell sits on ground the mesh never walks onto (the
// counter row), so the corridor ends at the closest walkable floor
// cell to the npc - the standing spot of a real customer.
//
// It reports whether the segment was planned and whether the approach
// ring fallback is worth running. The ring fallback owns exactly one
// failure class - the plan that resolved onto a foreign deck (a
// connected roof over the shop answers the closest layer of the
// destination cell, a roof-scale z gap above the merchant's floor):
// the conservative ring stop on the surrounding deck is the safe
// answer there. The hard navigator errors and the corridor-less mesh
// answers fail the ring search the very same way (the reachable
// component of the mesh is the same for both goals - no corridor to
// the destination cell means no corridor to its ring either), so the
// callers skip the fallback there and the trip attempt keeps its one
// search cost (the cooldown pins of the broken path search).
func (l *Loop) startWalkExactSegment(dest pathfind.Vec3) (bool, bool) {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false, false
    }
    from := pathfind.Vec3{
        X: float64(selfX),
        Y: float64(selfY),
        Z: float64(selfZ),
    }
    result, err := l.navigator.FindPath(from, dest)
    if err != nil {
        l.logf("Hunt: town trip exact path search failed: %v", err)

        return false, false
    }
    if result == nil || len(result.Waypoints) == 0 {
        l.logf("Hunt: no exact path to %d %d at all",
            int(dest.X), int(dest.Y))

        return false, false
    }
    if !result.Found && !result.Partial {
        // The manual walk contract (planUserWalk): a bare not found
        // with no corridor means the destination ground the mesh does
        // not reach at all - the ring goal of the same mesh answers
        // the same nothing.
        l.logf("Hunt: no exact route to %d %d, no corridor",
            int(dest.X), int(dest.Y))

        return false, false
    }
    last := result.Waypoints[len(result.Waypoints)-1]
    if dz := math.Abs(last.Z - dest.Z); dz > exactSegmentDeckTolerance {
        // The plan resolved onto a foreign deck: the roof over the
        // shop (the connected roof layer answers the destination
        // cell's closest-layer resolution) or a floor the merchant
        // does not stand on. The ring stop on the surrounding deck is
        // the safe answer, the talk machinery owns the rest.
        l.logf("Hunt: exact route to %d %d lands %.0f units off "+
            "the merchant deck, falling back to the ring",
            int(dest.X), int(dest.Y), dz)

        return false, true
    }
    if !result.Found {
        // The partial corridor of the exact search IS the answer the
        // merchant stop wants: the destination sits on the counter
        // row the mesh never walks onto, the funnel ends at the
        // customer cell in front of it.
        l.logf("Hunt: exact route to %d %d ends at the closest "+
            "walkable cell (the counter row)", int(dest.X), int(dest.Y))
    }

    return l.armTownWalkSegment(selfX, selfY, selfZ, from, dest, result, 0),
        false
}

// armTownWalkSegment arms the waypoint follower with a planned town walk:
// the plan view (the search contract of the answer), the frame
// measurement and the fresh segment state. Shared by the ring planner
// (startWalkSegmentSearch) and the exact planner (startWalkExactSegment).
func (l *Loop) armTownWalkSegment(
    selfX, selfY, selfZ int32,
    from, dest pathfind.Vec3,
    result *pathfind.Result,
    radius float64,
) bool {
    l.waypoints = result.Waypoints
    l.wpIndex = 0
    l.segmentDest = dest
    l.segmentStart = from
    // The plan view carries the search contract the segment answers (the
    // repro contract of the 3D pathfind link): the approach radius of
    // this very search, so a viewer replay rebuilds the walk the bot
    // follows instead of a lookalike (the 2026-09-19 route mismatch).
    l.segmentSearch = segmentSearchView(radius)
    // The fresh plan opens with a fresh frame measurement: the plan's
    // first waypoint IS the character's own cell resolved on the pack,
    // so the difference of the two z values is the vintage shift of
    // the standing surface (see click_frame.go) - the offset every
    // click of this segment rides into the server frame. A character the
    // pack answers underwater measures no shift: the swim z rides the
    // water surface while the mesh z names the floor, the pair is not
    // a vintage pair (the swim-floor gap is geometry, not a pack
    // disagreement) and anchoring by it would corrupt the segment's
    // clicks.
    l.segmentFrameOffset = 0
    if !l.navigator.OverWater(
        float64(selfX), float64(selfY), int16(selfZ)) {
        l.segmentFrameOffset = measureFrameOffset(
            selfZ, result.Waypoints[0].Z)
    }
    l.moveAt = time.Time{}
    l.stuckAt = time.Time{}
    l.stuckFast = false
    // A fresh plan is a fresh segment: the varied aim budget of the
    // online refusal answer re-arms (the segmentRefused latch itself
    // stays - it carries the trip level evidence the frozen trip
    // escalation gate reads, a re-path inside the same trip must
    // not erase it) and the move start watchdog re-arms with the
    // fresh plan's first click.
    l.refusalVariants = 0
    l.moveStartAt = time.Time{}
    l.forceStuck = false
    // A planned geodata segment owns the movement now: the direct zone
    // segment stall watcher stands down (its window would otherwise read
    // a trip's frozen standstill as its own and fire early on the
    // segments the budget gate resumes after the trip ends).
    l.zoneSegmentAt = time.Time{}

    return true
}

// waypointDistanceAnchored measures the character to waypoint distance
// with the waypoint height anchored into the server frame (see
// click_frame.go): the plan's own start waypoint resolved onto the
// standing cell and its pack height carries the vintage shift the
// frame offset measured, so the raw 3D distance to a waypoint the
// character stands ON is the shift alone. The jewelry shop porch of
// the farm readiness round (2026-09-20) measured 64 units on a plan
// whose every waypoint rode the shifted deck - 14 units past the
// intermediate pass radius - the cursor never advanced off the plan's
// own start and the self click of the pinned cursor burned the whole
// recovery ladder without a single walk attempt. The anchoring keeps
// the deck edge protection the 3D distance exists for: a waypoint a
// whole deck below the character still measures its true gap, the
// shift (tens to a few hundred units) explains only the vintage
// disagreement.
func waypointDistanceAnchored(
    wp pathfind.Vec3, selfX, selfY, selfZ int32, frameOffset float64,
) float64 {
    dx := wp.X - float64(selfX)
    dy := wp.Y - float64(selfY)
    dz := anchorZToServerFrame(wp.Z, frameOffset) - float64(selfZ)

    return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// waypointArrived reports whether the follower counts the waypoint at
// the index as reached from the character position: the intermediate
// waypoints need the tight pass radius (a detour turn or a bridge ramp
// entry must be walked through - the wide radius let the follower cut
// the corner into the railing), the final waypoint keeps the trip
// arrival radius of the segment (the search goal of the segment, the server
// may stop the character slightly short of the click). The tight ring
// segments (the teach stop close approach) pass their own tight final
// radius: the wide slack would end the walk a whole ring short of the
// teacher. The frame offset anchors the waypoint height into the
// server frame first (see waypointDistanceAnchored): the arrival test
// of the plan's own start must not measure the vintage shift.
func waypointArrived(
    waypoints []pathfind.Vec3, index int, selfX, selfY, selfZ int32,
    frameOffset, finalArrive float64,
) bool {
    radius := waypointPassDist
    if index == len(waypoints)-1 {
        radius = finalArrive
    }

    return waypointDistanceAnchored(waypoints[index], selfX, selfY,
        selfZ, frameOffset) <= radius
}

// finalArriveRadius answers the arrival radius of the final waypoint
// of the current town segment: the wide trip slack for the ordinary segments,
// the tight pass radius for the close ring segments - a segment that searches
// its route within npcApproachOffset of the npc must actually reach
// the route end, the wide slack accepts a stop a full ring short of
// the teacher and dumps the last stretch onto the straight offset
// clicks whose lines cross the roof-only interior bands (the trainer
// hall rows carry the floor, the spaces between them do not). The
// exact segments (the merchant stops' approach zero search) answer the
// tight radius the same way: their final cell is the customer spot
// the segment exists to reach.
func (l *Loop) finalArriveRadius() float64 {
    // The exact segments (the merchant stops' startWalkExactSegment) must walk
    // the plan all the way to its final cell: the wide arrive radius
    // would end the walk a whole arrive radius short of the customer
    // cell the exact search answered - the standing spot opposite the
    // merchant behind the counter.
    if l.segmentSearch != nil && l.segmentSearch.Approach == 0 {
        return waypointPassDist
    }
    if l.segmentRadius > 0 && l.segmentRadius < tripApproachRadius {
        return waypointPassDist
    }

    return waypointArriveDist
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

// waypointPassedAlongRoute reports whether the character already rides
// the route at or beyond the aimed waypoint: the projection onto the
// aimed waypoint's own segment past its start (the plain passed test
// of the cursor advance), or the projection onto ANY later segment of
// the remaining route (the march past a bend the pinned cursor never
// advanced onto - the 2026-09-20 acceptance round's armed extension
// walked the character along the route samples across the wp 1 bend
// while the cursor stayed pinned on wp 0, and the plain single segment
// test measured the lateral to the OLD segment - a full corridor
// width past it - and answered not passed, so the raw aim of the
// passed start waypoint walked the character the whole bend back).
// A character inside the arrival radius of the aimed waypoint never
// counts (the caller gates on it): the pull back onto the waypoint is
// the re-approach click the ramp corner needs, not a backward walk.
func waypointPassedAlongRoute(
    waypoints []pathfind.Vec3, index int, selfX, selfY int32,
) bool {
    for j := index; j+1 < len(waypoints); j++ {
        wp := waypoints[j]
        next := waypoints[j+1]
        segX := next.X - wp.X
        segY := next.Y - wp.Y
        segLen := math.Hypot(segX, segY)
        if segLen < 1 {
            continue
        }
        selfDX := float64(selfX) - wp.X
        selfDY := float64(selfY) - wp.Y
        along := (selfDX*segX + selfDY*segY) / segLen
        if along <= 0 {
            continue
        }
        lateral := math.Abs(selfDX*segY-selfDY*segX) / segLen
        if lateral > waypointCorridor {
            continue
        }
        if j > index || along <= segLen {
            // On a later segment (any point of it lies past the
            // aimed waypoint along the walk) or past the aimed
            // waypoint on its own segment.
            return true
        }
    }

    return false
}

// walkTownWaypoints follows the planned waypoints with ground click
// walks and returns true when the final waypoint is reached. The plan
// prices the water (the mesh swim rate of the C1 zone data), so the
// follower walks the wet segments it planned - the swim is a priced
// slowdown, not a failure.
// Segments longer than the server move request limit are split into
// straight intermediate points (the smoothing guarantees the line of
// sight of every segment, so the intermediate points stay on the verified
// segment). A waypoint the character already passed ON THE ROUTE is
// skipped: a server position correction or a restart jump can place
// the character ahead of the follower, and walking back to a passed
// waypoint would loop. A walk that stands still re-paths from the
// current position to the segment destination, bounded by the re-path
// budget of the trip.
func (l *Loop) walkTownWaypoints() bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    // The cursor key escape owns the segment while it runs: the claim
    // ladder drives the character (the escape arms from the follower
    // recovery too - the refusing pocket branch of stuckTownWalk),
    // the click machinery stays down until the escape ends.
    if l.cursorEscape.armed {
        l.driveCursorKeyEscape(time.Now(), selfX, selfY)

        return false
    }

    return l.followWaypoints(selfX, selfY, selfZ, time.Now())
}

// cursorEscapeState carries the armed cursor key escape: the origin
// the refusal verdict stood at, the claimed dry steps toward the
// validated hop aim, the claim cursor and the follow bookkeeping
// (see beginCursorKeyEscape).
type cursorEscapeState struct {
    armed   bool
    originX int32
    originY int32
    steps   [][3]int32
    // wpMap names the route waypoint every claimed step completes
    // (the index into the segment's waypoints, -1 for the mid segment
    // strides): the WASD walk IS the ground progress, the waypoint
    // the claims walk onto marks passed (the owner report of the
    // 2026-09-19 17:18 dump - the wasd walked points must never be
    // walked again by the resumed clicks). Nil for the planless
    // fallback whose straight ladder owns no route waypoints.
    wpMap []int
    next  int
    // lastClaimAt paces the claims at the official client cadence.
    lastClaimAt time.Time
    // claimsSinceMove counts the claims since the last observed
    // server position change: the follow probe of the escape.
    claimsSinceMove int
}

// zeroCursorEscape returns the cleared escape state of the segment and
// trip boundaries: every field explicit (the exhaustruct convention
// of the repository).
func zeroCursorEscape() cursorEscapeState {
    return cursorEscapeState{
        armed:           false,
        originX:         0,
        originY:         0,
        steps:           nil,
        wpMap:           nil,
        next:            0,
        lastClaimAt:     time.Time{},
        claimsSinceMove: 0,
    }
}

// beginCursorKeyEscape arms the cursor key escape of a click
// refusing cell: the routed walk clicks bounced with ActionFailed
// while the character stood still, and the 2026-09-14 15:10 report
// proved the user's server answers NO mouse click from such a cell
// at all - even the official client stood frozen on the village
// plaza cell until the player walked it out with the ARROW KEYS.
// The escape emulates exactly that movement: the keyboard-mode move
// request (MoveToLocation movement mode 0) arms the server's cursor
// key handling, and the claimed ValidatePosition stream the official
// client drives from its own movement simulation moves the character
// server-side without any click validation (ValidatePosition.runImpl
// syncs the claimed placement into the world and broadcasts it). The
// claimed steps follow the planned route when the segment holds one (the
// route ladder bends where the plan bends - see
// cursorEscapeRouteSteps) and fall back to the straight line toward
// the aim without a plan, one run-speed step per second - the aim
// clamped into the pocket radius, so a far target can never pull a
// straight march out of the planless escape (the owner rule of the
// 2026-09-19 round: NEVER walk the direct line). It reports whether
// the escape armed; a server that ignores the claims burns the
// attempts (see driveCursorKeyEscape) and the caller keeps its
// honest abort.
func (l *Loop) beginCursorKeyEscape(
    selfX, selfY, selfZ, aimX, aimY, aimZ int32,
) bool {
    if l.cursorEscapes >= cursorEscapeAttemptsMax {
        return false
    }
    marchStart := l.escapeMarchStart(selfX, selfY, selfZ)
    steps, wpMap := l.cursorEscapeRouteSteps(
        selfX, selfY, selfZ, marchStart)
    form := "along the planned route"
    if len(steps) == 0 {
        // The planless fallback: the straight ladder toward the aim,
        // pocket sized. A far aim is clamped to the pocket radius
        // along its line - the escape walks the first stretch toward
        // the target and the re-plans of the settle own the rest. The
        // aim z rides the frame offset first: the interpolation holds
        // only between two z of one frame, and the raw waypoint z is
        // the pack frame (see cursorEscapeRouteSteps).
        aimZ = int32(math.Round(
            anchorZToServerFrame(float64(aimZ), l.segmentFrameOffset)))
        dx := float64(aimX - selfX)
        dy := float64(aimY - selfY)
        if dist := math.Hypot(dx, dy); dist > cursorEscapeRouteMax {
            frac := cursorEscapeRouteMax / dist
            aimX = selfX + int32(dx*frac)
            aimY = selfY + int32(dy*frac)
        }
        steps = l.cursorEscapeSteps(selfX, selfY, selfZ,
            aimX, aimY, aimZ)
        wpMap = nil
        form = "toward the aim"
    }
    if len(steps) == 0 {
        // No stride fits the budget toward the aim: the honest abort
        // of the caller stands.
        return false
    }
    // The ground re-anchor: the cursor sits on the first waypoint the
    // character's ground owes - the claims own the segment now and the
    // bookkeeping bet of the click ladder (the forward jump, the skip
    // chain) is void the moment the escape arms. The marks of the
    // claimed strides advance the cursor from here as the ground is
    // walked, the settle's own advance reconciles it with the arrival
    // test, and the plan view the web UI serves during the escape
    // shows the walked prefix instead of the jumped cursor.
    l.wpIndex = marchStart
    // The arm request aims the ladder's far end: the mode 0 move
    // starts the server side walk the claims then own, and a target
    // on the character's own cell would answer the stopMove refusal
    // before the flag ever latches (the fresh plan's first waypoint
    // is the standing cell itself).
    arm := steps[len(steps)-1]
    l.cursorEscapes++
    l.cursorEscape = cursorEscapeState{
        armed:           true,
        originX:         selfX,
        originY:         selfY,
        steps:           steps,
        wpMap:           wpMap,
        next:            0,
        lastClaimAt:     time.Time{},
        claimsSinceMove: 0,
    }
    if err := l.game.CursorKeyWalkTo(arm[0], arm[1], arm[2]); err != nil {
        l.logf("Hunt: the cursor key arm failed: %v", err)
    }
    l.logf("Hunt: the refused clicks hand the walk to the cursor key "+
        "escape, walking %s toward %d %d (%d claimed steps)",
        form, arm[0], arm[1], len(steps))

    return true
}

// cursorEscapeRouteSteps builds the claimed steps of a route
// following escape: the planned waypoints from the character's ground
// position (the pathfind route the segment already holds)
// interpolated into run-speed strides - the claims follow the plan
// the mesh priced, wet strides included. The strides march
// every segment of the route in order, so the ladder bends where the
// plan bends: an obstacle the straight chord would push the
// character through (the tree on the plaza, the railing corner) is
// walked around the way the planner drew it. Every step z rides the
// segment's frame offset into the server frame (see click_frame.go):
// the claims name the placement the server owns, and a raw pack z
// would place the character below its own ground on every shifted
// cell - the server correction snaps it back and the escape walks
// the character nowhere (the farm readiness round of 2026-09-20:
// the porch claims rode the pack z 64 units under the live ground).
// The route length caps
// at cursorEscapeRouteMax so one escape stays a pocket recovery -
// the claims never walk the character across the whole map. It
// returns the steps with their waypoint map (the route waypoint
// every step completes, -1 for the mid segment strides - the WASD
// ground progress of the drive, see markEscapeClaimedWaypoint) and
// nil maps without a plan (the straight fallback owns the segment).
//
// The march starts at the first waypoint the CHARACTER's ground still
// owes (see escapeMarchStart), never blindly at the plan cursor: the
// cursor may sit AHEAD of the walked ground - the click ladder bets
// far waypoints (the forward jump of a refused click validates a
// longer chord and jumps the cursor onto it, the stuck skip jumps
// onto the first clear successor) and a server that refuses the bet
// leaves the cursor ahead while the character never moved. Marching
// from the jumped cursor would claim the straight chord from the
// standing cell to the far waypoint - off the route the planner drew
// and through whatever the chord cuts. The claims walk the ROUTE
// from the character's own ground: the stride sequence re-enters the
// plan at the first un-walked bend and every segment from there is
// the planner's own geometry.
func (l *Loop) cursorEscapeRouteSteps(
    selfX, selfY, selfZ int32, start int,
) ([][3]int32, []int) {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) ||
        start >= len(l.waypoints) {
        return nil, nil
    }
    steps := make([][3]int32, 0, 16)
    wpMap := make([]int, 0, 16)
    px, py, pz := float64(selfX), float64(selfY), float64(selfZ)
    budget := cursorEscapeRouteMax
    for i := start; i < len(l.waypoints); i++ {
        wp := l.waypoints[i]
        // The waypoint z anchors into the server frame BEFORE the
        // interpolation: the march interpolates between the character
        // z (the server frame) and the waypoint z, and a raw pack
        // waypoint z would blend the two frames mid segment - the
        // claimed stride then drifts below the live ground by half
        // the shift before the end of the first stretch.
        wpZ := anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
        ox, oy, oz := px, py, pz
        dx := wp.X - ox
        dy := wp.Y - oy
        dz := wpZ - oz
        dist := math.Hypot(dx, dy)
        stride := cursorEscapeStep
        for stride < dist && budget > 0 {
            frac := stride / dist
            sx := ox + dx*frac
            sy := oy + dy*frac
            sz := oz + dz*frac
            // A stride that lands within the coincide radius of the
            // segment's waypoint completes it (the closing step
            // below is then skipped).
            done := -1
            if math.Hypot(wp.X-sx, wp.Y-sy) <= hopCoincideDist {
                done = i
            }
            steps = append(steps, [3]int32{
                int32(math.Round(sx)),
                int32(math.Round(sy)),
                int32(math.Round(sz)),
            })
            wpMap = append(wpMap, done)
            px, py, pz = sx, sy, sz
            budget -= cursorEscapeStep
            stride += cursorEscapeStep
        }
        if budget <= 0 {
            return steps, wpMap
        }
        // Close the segment onto the waypoint itself when the last
        // stride ended short of it: the bend points stay on the
        // route, the next segment leaves from the route bend and
        // not from a corner the stride cut.
        if math.Hypot(wp.X-px, wp.Y-py) > hopCoincideDist {
            steps = append(steps, [3]int32{
                int32(math.Round(wp.X)),
                int32(math.Round(wp.Y)),
                int32(math.Round(wpZ)),
            })
            wpMap = append(wpMap, i)
            px, py, pz = wp.X, wp.Y, wpZ
            budget -= cursorEscapeStep
        }
    }

    return steps, wpMap
}

// escapeMarchStart answers the index of the first plan waypoint the
// character's ground still owes: the first waypoint that is neither
// arrived at (the intermediate pass radius - the walked ground
// contract of the arrival test) nor passed along the route (the
// projection test of advanceWaypoints). The plan cursor itself can
// sit ahead of this index - the forward jump and the stuck skip bet
// far waypoints whose clicks the offline port validates, and a server
// that refuses them leaves the bookkeeping ahead of the ground. The
// escape march trusts the ground: the claims re-enter the plan at the
// first un-walked bend and the marks (markEscapeClaimedWaypoint) pull
// the cursor back onto the walked prefix as the strides land.
func (l *Loop) escapeMarchStart(selfX, selfY, selfZ int32) int {
    for i := range l.waypoints {
        if waypointArrived(l.waypoints, i, selfX, selfY, selfZ,
            l.segmentFrameOffset, waypointPassDist) {
            continue
        }
        if i+1 < len(l.waypoints) &&
            waypointPassed(l.waypoints[i], l.waypoints[i+1], selfX, selfY) {
            continue
        }

        return i
    }

    return len(l.waypoints)
}

// cursorEscapeSteps builds the claimed steps of the cursor key
// escape: the straight line from the standing cell toward the
// validated hop aim, interpolated into run-speed steps - the claims
// follow the straight fallback exactly as planned. The steps stop at
// the aim.
func (l *Loop) cursorEscapeSteps(
    selfX, selfY, selfZ, aimX, aimY, aimZ int32,
) [][3]int32 {
    total := math.Hypot(float64(aimX-selfX), float64(aimY-selfY))
    if total < 1 {
        return nil
    }
    var steps [][3]int32
    for walked := cursorEscapeStep; walked < total; walked += cursorEscapeStep {
        frac := walked / total
        stepX := int32(float64(selfX) +
            float64(aimX-selfX)*frac)
        stepY := int32(float64(selfY) +
            float64(aimY-selfY)*frac)
        stepZ := int32(float64(selfZ) +
            float64(aimZ-selfZ)*frac)
        steps = append(steps, [3]int32{stepX, stepY, stepZ})
    }
    // The aim itself closes the ladder when it is not already the
    // last interpolated step.
    if len(steps) == 0 ||
        steps[len(steps)-1][0] != aimX || steps[len(steps)-1][1] != aimY {
        steps = append(steps, [3]int32{aimX, aimY, aimZ})
    }

    return steps
}

// driveCursorKeyEscape advances the armed cursor key escape by one
// decision: the claims pace at the official client cadence (one
// run-speed step per second), the follow probe watches the server
// position - the server that follows the claims moves the character
// and the escape runs its ladder to the aim, the server that ignores
// them (the keyboard movement disabled, a build without the cursor
// key branch of ValidatePosition) burns the follow patience and the
// escape ends with the honest log line for the caller's abort
// ladder. The settle window after the last claim lets the position
// broadcasts land before the segment resumes its clicks.
func (l *Loop) driveCursorKeyEscape(
    now time.Time, selfX, selfY int32,
) {
    // The follow probe: any drift past the margin from the escape
    // origin re-arms the patience (the character moved - the claims
    // own it).
    moved := math.Hypot(
        float64(selfX-l.cursorEscape.originX),
        float64(selfY-l.cursorEscape.originY))
    if moved > cursorEscapeFollowStep {
        l.cursorEscape.claimsSinceMove = 0
    } else if l.cursorEscape.claimsSinceMove >= cursorEscapeFollowClaims {
        l.cursorEscape.armed = false
        l.logf("Hunt: the cursor key escape made no progress - " +
            "the server ignores the claimed positions")

        return
    }
    // The claim pacing: one run-speed step per second, the official
    // client cadence of the ValidatePosition stream.
    if !l.cursorEscape.lastClaimAt.IsZero() &&
        now.Sub(l.cursorEscape.lastClaimAt) < cursorEscapePeriod {
        return
    }
    if l.cursorEscape.next < len(l.cursorEscape.steps) {
        step := l.cursorEscape.steps[l.cursorEscape.next]
        heading := cursorEscapeHeading(selfX, selfY, step[0], step[1])
        if err := l.game.ClaimValidatePosition(
            step[0], step[1], step[2], heading); err != nil {
            l.logf("Hunt: the claimed position failed: %v", err)
        }
        // The official client renders the arrow walk facing from its
        // own movement simulation - the claim facing IS the client
        // side truth while the keyboard movement owns the stream
        // (the server echo carries the arm heading instead, see
        // ApplySelfFacing).
        l.tracker.ApplySelfFacing(heading)
        l.cursorEscape.next++
        l.cursorEscape.lastClaimAt = now
        l.cursorEscape.claimsSinceMove++
        // The WASD ground progress: the claims of a FOLLOWED escape
        // mark the route waypoints passed (the server position moved
        // - the claims own the character, the walked points stay
        // passed for the resumed clicks). The claims of an ignored
        // escape mark nothing - the plan cursor never fakes the
        // ground the character never walked, the abort ladder of the
        // ignoring server stays honest.
        if l.escapeFollows(selfX, selfY) {
            for k := range l.cursorEscape.next {
                l.markEscapeClaimedWaypoint(k)
            }
        }

        return
    }
    // The settle message names the walk that resumes: the follower
    // mode resumes the planned waypoint clicks on the same plan the
    // claims just walked (the owner contract: the normal mode at the
    // point).
    if now.Sub(l.cursorEscape.lastClaimAt) >= cursorEscapeSettle {
        aim := l.cursorEscape.steps[len(l.cursorEscape.steps)-1]
        l.cursorEscape.armed = false
        // The WASD ground becomes the plan progress: the position the
        // character ACTUALLY reached advances the cursor (the arrival
        // radius and the route projection of the real ground) - the
        // resumed clicks aim the first waypoint still ahead, never
        // back to the walked ones (the owner report of the 2026-09-19
        // 17:18 dump: the walk returned to the wasd walked points and
        // burned two minutes skipping them). The ladder's aim itself
        // is never trusted here: a server that ignored the claims
        // left the character on the origin ground, and progress the
        // server never delivered must never complete the plan.
        if sx, sy, sz, ok := l.tracker.SelfPosition(); ok {
            l.advanceWaypoints(sx, sy, sz)
        }
        // The escape moved the character server side: the stuck
        // window re-baselines from the settle ground - the frozen
        // verdict of the pre escape ground must not fire on the
        // resumed clicks (the dump: the skip fired one second after
        // the resume).
        l.stuckAt, l.stuckX, l.stuckY = time.Time{}, 0, 0
        l.logf("Hunt: the cursor key escape walked to %d %d, "+
            "resuming the planned walk clicks", aim[0], aim[1])
    }
}

// escapeFollows reports whether the server follows the claimed stream
// of the running escape: the character position drifted past the
// follow margin from the escape origin (the same verdict the follow
// probe of the drive applies). The ground progress marking rides this
// verdict - a server that ignores the claims never moves the
// character, and progress it never delivered must never mark the
// plan.
func (l *Loop) escapeFollows(selfX, selfY int32) bool {
    return math.Hypot(
        float64(selfX-l.cursorEscape.originX),
        float64(selfY-l.cursorEscape.originY),
    ) > cursorEscapeFollowStep
}

// markEscapeClaimedWaypoint advances the plan cursor past the route
// waypoint the claimed step completed: the WASD walk IS the ground
// progress, the waypoint the claims walked onto counts as passed
// (the owner report: the points the wasd walked must show passed and
// the resumed clicks must never walk back to them). No line gate
// stands here on purpose: the claim ladder follows the planner's own
// bends, the ground the claim landed on IS the route ground - the
// conservative advance gates would re-introduce the exact backtrack
// the report pinned (the walked prefix marked unpassed because the
// line test ran from the walked ground to a waypoint behind it).
// The mark is UNCONDITIONAL about the cursor it finds: the cursor may
// sit AHEAD of the walked ground (the forward jump bet a far
// waypoint's chord and the server refused the click, the skip chain
// ran ahead while the character stood) - the claim's ground progress
// is the honest authority and the cursor follows it, the bookkeeping
// bet never keeps a waypoint the claims have not walked marked
// passed (the resumed clicks would aim the jumped far waypoint
// again and re-lose the ground the escape just gained).
func (l *Loop) markEscapeClaimedWaypoint(stepIndex int) {
    if stepIndex >= len(l.cursorEscape.wpMap) {
        return
    }
    wp := l.cursorEscape.wpMap[stepIndex]
    if wp < 0 {
        return
    }
    l.wpIndex = wp + 1
    l.moveAt = time.Time{}
}

// cursorEscapeHeading renders the L2 heading of a step direction in
// the convention of the reference Mobius master
// (LocationUtil.calculateHeadingFrom: the degrees of atan2(deltaY,
// deltaX) scaled by 65536 over 360, so east is 0, south 16384, west
// 32768 and north 49152) - the same convention the state tracker
// applies to the movement broadcasts (state.HeadingFromDelta). The
// swapped atan2 arguments this function carried before the
// 2026-09-19 17:18 report mirrored the facing (the character walked
// west while the web UI drew the south line) and the mirror is not
// only cosmetic: the mobius cursor key movement probes its obstacle
// front along the heading (Creature.updatePosition), the mirrored
// claims probe behind the character's back.
func cursorEscapeHeading(
    fromX, fromY, toX, toY int32,
) int32 {
    dx := float64(toX - fromX)
    dy := float64(toY - fromY)
    angle := math.Atan2(dy, dx)
    if angle < 0 {
        angle += 2 * math.Pi
    }

    return int32(angle * 65536 / (2 * math.Pi))
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
            l.waypoints, l.wpIndex, selfX, selfY, selfZ,
            l.segmentFrameOffset, l.finalArriveRadius())
        passed := !arrived && l.wpIndex+1 < len(l.waypoints) &&
            waypointPassed(l.waypoints[l.wpIndex],
                l.waypoints[l.wpIndex+1], selfX, selfY)
        if !arrived && !passed {
            return
        }
        if !l.segmentAdvanceClear(selfX, selfY, selfZ, l.wpIndex+1) {
            return
        }
        l.wpIndex++
        l.moveAt = time.Time{}
    }
}

// followWaypoints is the shared waypoint follower core of the town
// segments: the waypoint arrival (tight for the
// intermediate turns, wide for the final goal), the passed waypoint
// skipping, the stuck tracking and the click pace. The planned water
// segments walk like the dry ones: the plan prices the crossings (the
// swim rate), the follower follows it - the re-plan ladder owns
// the off-plan swims instead of a click guard (see walkTownWaypoints).
func (l *Loop) followWaypoints(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    l.advanceWaypoints(selfX, selfY, selfZ)
    if l.wpIndex >= len(l.waypoints) {
        return true
    }
    // The move start watchdog: a click whose movement never started
    // forces the stuck verdict below - the recovery ladder runs at
    // once instead of standing out the full window on a dead click.
    if l.noteMoveStart(now, selfX, selfY) {
        l.forceStuck = true
    }
    if l.walkStuck(now, selfX, selfY) {
        return false
    }
    // The stuck handling may have re-planned the segment: the fresh plan
    // starts at the standing cell, so its wp 0 IS the character's own
    // position and a click at it is the self-click the server always
    // collapses (distance below the cancellation limit) - the round 56
    // reproduction caught the recovery burning a second re-path on
    // exactly that refusal. Re-run the cursor advance so the click
    // below aims the fresh plan's first real waypoint instead. The
    // stuck recovery may also have armed the cursor key escape (the
    // refusing pocket branch): the claims own the segment then, the click
    // below stays down until the escape settles.
    if l.cursorEscape.armed {
        return false
    }
    l.advanceWaypoints(selfX, selfY, selfZ)
    if l.wpIndex >= len(l.waypoints) {
        return true
    }
    if !l.moveAt.IsZero() && now.Sub(l.moveAt) < walkRequestPeriod {
        return false
    }
    l.clickWaypoint(selfX, selfY, selfZ, now)

    return false
}

// waypointPassedBehind reports whether the aimed waypoint sits behind
// the character along the route: the character marched past it and a
// click onto it walks backward (see waypointPassedAlongRoute). A
// character inside the arrival radius of the aimed waypoint never
// counts - the pull back onto the waypoint is the re-approach click
// the ramp corner needs, not a backward walk (the round 56 gated
// waypoint design).
func (l *Loop) waypointPassedBehind(
    wp pathfind.Vec3, selfX, selfY, selfZ int32,
) bool {
    if l.wpIndex+1 >= len(l.waypoints) {
        return false
    }
    arrived := waypointDistanceAnchored(wp, selfX, selfY, selfZ,
        l.segmentFrameOffset) <= waypointPassDist

    return !arrived && waypointPassedAlongRoute(l.waypoints, l.wpIndex,
        selfX, selfY)
}

// clickWaypoint aims the current waypoint, bends the click around the
// idle aggressive camps, guards the line against the server refusal
// and sends it. The segment splitting caps the click at the
// server move request limit; the short click extension re-aims the
// clicks under the server rescue floor at the plan polyline (see
// minWalkClick); the server click validation runs after the steering
// so the line it verifies is the one actually
// being sent. Without a navigator the guard stays off (the walk was
// planned elsewhere, the follower only walks it).
func (l *Loop) clickWaypoint(
    selfX, selfY, selfZ int32, now time.Time,
) {
    wp := l.waypoints[l.wpIndex]
    dx := wp.X - float64(selfX)
    dy := wp.Y - float64(selfY)
    dist := math.Hypot(dx, dy)
    // The click z rides the server frame transport (see
    // click_frame.go): the mesh frame waypoint height plus the
    // measured vintage shift of the standing surface - the server
    // resolves the click's destination layer by the nearest height to
    // this z, and the raw mesh z names the wrong layer wherever the
    // packs disagree (the village sandwich refusals).
    wpZ := anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
    moveX, moveY, moveZ := wp.X, wp.Y, wpZ
    if dist > maxMoveDistance {
        frac := maxMoveDistance / dist
        moveX = float64(selfX) + dx*frac
        moveY = float64(selfY) + dy*frac
        moveZ = float64(selfZ) + (wpZ-float64(selfZ))*frac
    }
    // The backward walk guard: an aimed waypoint the character already
    // moved PAST along the route itself (see waypointPassedAlongRoute)
    // would walk it BACK off the ground the route samples just covered
    // - the one step away and return ping pong of the 2026-09-20
    // acceptance round. The armed extension keeps its own behind
    // verdict too (the far V-detour waypoint whose leaving segment
    // already points back, see waypointBehindRoute).
    passedBehind := l.waypointPassedBehind(wp, selfX, selfY, selfZ)
    switch {
    case dist < minWalkClick:
        // The rescue floor discipline runs IMMEDIATELY, no stuck
        // verdict needed: the server's findPath branch only takes a
        // collapsed click over the rescue threshold, a shorter one is
        // silently canceled with ActionFailed and never moves a cell
        // (the 2026-09-11 11:34 dump froze two whole trip cycles on
        // the 22 unit first waypoint click) - the sub-floor aim
        // re-aims at the forward route samples before any click
        // leaves the bot. No sample validating keeps the plain
        // waypoint click: the refusal machinery of
        // clickServerValidated answers it exactly like today.
        extX, extY, extZ, ok := l.extendShortClick(
            selfX, selfY, selfZ, moveX, moveY, moveZ)
        if ok {
            moveX, moveY, moveZ = extX, extY, extZ
        }
    case l.extendArmed && (passedBehind || waypointBehindRoute(
        l.waypoints, l.wpIndex, selfX, selfY)):
        // The recovery of a stuck segment (extendArmed): the stuck
        // proved the plain clicks of this segment do not move the
        // character (a server side refusal the offline click
        // validation cannot see), so the primary target behind the
        // character on the route gives way to the forward route
        // samples.
        extX, extY, extZ, ok := l.extendShortClick(
            selfX, selfY, selfZ,
            moveX, moveY, moveZ)
        if ok {
            moveX, moveY, moveZ = extX, extY, extZ
        } else {
            // No forward sample validates and the
            // waypoint is behind: clicking it walks
            // the character backward into the pocket
            // the route samples just escaped. Hold
            // the click - the stuck window re-plans
            // from the standing cell, and the
            // planner knows the wall the server-side
            // routing has to route around.
            return
        }
    case passedBehind:
        // The same hold without the armed extension: a backward
        // click is ground loss no matter the recovery state, and the
        // stuck window owns the answer (the skip ladder walks the
        // first clear successor, the re-path plans around the wall).
        return
    }
    // A waypoint inside an idle mob's trigger circle cannot be reached
    // by any tangent arc (the tangent side flips at every re-issue, see
    // segmentTargetThreatened): skip it ahead instead of ping-ponging
    // around the camp for hours. The verdict reads the WAYPOINT the
    // skip would abandon, never the issued click target: a far
    // waypoint's click rides the maxMoveDistance clip next to the
    // character, so a camp sitting between the character and the
    // horizon flags every far waypoint along that direction while the
    // waypoints themselves stay hundreds to thousands of units outside
    // the circle - the 2026-09-20 town walk (build 4805d9a, bot test3)
    // skipped cursors 9 through 23 of 42 in three seconds that way,
    // marked the whole middle of the plan passed while the character
    // stood still and walked the rest cross-country. The skip reuses
    // the stuck machinery's clear-successor gate, so the cursor only
    // jumps onto a waypoint the standing cell can click directly.
    moveXI, moveYI := int32(math.Round(moveX)), int32(math.Round(moveY))
    moveZI := int32(math.Round(moveZ))
    wpXI, wpYI := int32(math.Round(wp.X)), int32(math.Round(wp.Y))
    if l.segmentTargetThreatened(wpXI, wpYI,
        int32(l.segmentDest.X), int32(l.segmentDest.Y)) {
        if next := l.nextClearWaypoint(selfX, selfY, selfZ); next > l.wpIndex {
            l.wpIndex = next
            l.moveAt = time.Time{}
            l.logger.Printf("Hunt: the waypoint sits inside an aggro "+
                "circle, skipping ahead (cursor %d of %d)",
                l.wpIndex, len(l.waypoints))

            return
        }
        // No clear successor: the stuck escalation owns the segment.
    }
    // The aggro-aware steering: the camps of idle aggressive mobs
    // sitting on the segment bend it sideways (see loop_avoid.go). Every
    // transit walk passes through it - the town runs both ways, the
    // returns to the farm spot, the inter-ground walks of the spot
    // economy - while the mobs at the destination stay exempt.
    if ax, ay, dodged := l.steerClearOfAggro(
        selfX, selfY, selfZ, moveXI, moveYI, moveZI,
        int32(l.segmentDest.X), int32(l.segmentDest.Y), now); dodged {
        moveX, moveY = float64(ax), float64(ay)
    }
    // The server click validation runs last: the server refuses
    // whole lines its Bresenham raster walks into walled corners -
    // a refused click never moves the character.
    if l.navigator != nil && !l.clickServerValidated(
        selfX, selfY, selfZ, &moveX, &moveY, &moveZ, now) {
        return
    }
    l.moveAt = now
    if err := l.game.WalkTo(int32(moveX), int32(moveY),
        int32(moveZ)); err != nil {
        l.logf("Hunt: town walk request failed: %v", err)
    }
}

// waypointBehindRoute reports whether the waypoint at the index sits
// behind the character relative to the route segment leaving it: the
// cursor can stay pinned on a waypoint the character already moved
// past (the successor line is not clear yet), and clicking it walks
// the character BACK off the ground the forward route samples just
// walked - the extension takes over such clicks. Only a waypoint the
// character stands NEAR counts: the V-shaped detour routes (the
// 2026-09-12 trainer hall recovery - the route climbs far north
// before it doubles back south east) carry waypoints hundreds of
// units ahead whose position projects beyond the doubling segment,
// and clicking those far waypoints is the whole point of the climb -
// the projection test alone re-aimed their clicks at the far route
// samples whose straight lines cross the terrace walls (the freeze
// the walled aisle reproduction exposed). A waypoint ahead or beside
// the character (the normal pull back onto the route, the round 56
// gated waypoint design) never triggers it.
func waypointBehindRoute(
    waypoints []pathfind.Vec3, index int, selfX, selfY int32,
) bool {
    if index+1 >= len(waypoints) {
        return false
    }
    wp := waypoints[index]
    next := waypoints[index+1]
    fdx := next.X - wp.X
    fdy := next.Y - wp.Y
    if math.Hypot(fdx, fdy) < 1 {
        return false
    }
    if math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY)) >
        waypointPassDist {
        // The waypoint sits far away: the character has not passed
        // it, whatever the segment direction says.
        return false
    }

    return (wp.X-float64(selfX))*fdx+(wp.Y-float64(selfY))*fdy < 0
}

// extendShortClick re-aims a click whose primary target sits under the
// server rescue floor (minWalkClick) at the forward route samples past
// the floor: the click line still starts at the standing cell, but its
// target walks along the planned route far enough that a server side
// collapse hands the click to the server pathfinder instead of
// silently canceling it (see the minWalkClick contract). The candidates
// try in forward order and the first the local rules bless wins - the
// server click validation port runs on the chord, and a route
// cell that walls one chord (the 2026-09-11 11:34 reproduction: the
// terrace hillside step refused every chord crossing it) only skips
// that sample, the next one further along the route still carries the
// click. No candidate passing keeps the plain waypoint click - the
// refusal machinery of clickServerValidated answers it exactly like
// today.
func (l *Loop) extendShortClick(
    selfX, selfY, selfZ int32,
    primX, primY, primZ float64,
) (float64, float64, float64, bool) {
    if l.navigator == nil {
        return primX, primY, primZ, true
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    candidates, count := extendShortClickCandidates(
        selfX, selfY, l.waypoints, l.wpIndex, l.segmentFrameOffset)
    for c := range count {
        sample := candidates[c]
        if _, ok := l.navigator.ValidateClick(from, sample); !ok {
            continue
        }

        return sample.X, sample.Y, sample.Z, true
    }

    return primX, primY, primZ, false
}

// extendShortClickCandidates marches the forward route of the plan
// starting at the aimed waypoint and collects the first route samples
// whose straight line distance from the standing position reaches the
// floor: the click chords to them clear the server rescue threshold
// while their targets stay on the planned route. The march only looks
// FORWARD - the aimed waypoint itself may sit behind the character (a
// pinned cursor beside the route), and the backward polyline samples
// produced chords the server refused from the cells past the first
// step (the 2026-09-11 11:34 reproduction: the second extension
// clicked a 24 unit chord into the terrace wall and the escape hop
// walked the character back, a ping pong through the whole stuck
// cycle). A route whose whole forward stretch stays under the floor
// collects nothing - the arrival case keeps its plain waypoint click.
func extendShortClickCandidates(
    selfX, selfY int32,
    waypoints []pathfind.Vec3, index int, frameOffset float64,
) ([extendCandidateMax]pathfind.Vec3, int) {
    var out [extendCandidateMax]pathfind.Vec3
    count := 0
    px, py := float64(selfX), float64(selfY)
    for i := index; i+1 < len(waypoints) && count < extendCandidateMax; i++ {
        from, to := waypoints[i], waypoints[i+1]
        fdx, fdy := to.X-from.X, to.Y-from.Y
        seg := math.Hypot(fdx, fdy)
        if seg < 1 {
            continue
        }
        steps := int(seg/extendMarchStep) + 1
        for s := 1; s <= steps && count < extendCandidateMax; s++ {
            frac := float64(s) / float64(steps)
            qx := from.X + fdx*frac
            qy := from.Y + fdy*frac
            relX, relY := qx-px, qy-py
            if math.Hypot(relX, relY) < minWalkClick ||
                relX*fdx+relY*fdy <= 0 {
                // Under the rescue floor or backward along the
                // route: a sample behind the character pulls the
                // walk back off the ground the extension just
                // walked (the 2026-09-11 11:34 reproduction
                // ping ponged on exactly the samples near the
                // pinned waypoint the character already passed).
                continue
            }
            out[count] = pathfind.Vec3{
                X: qx, Y: qy,
                Z: anchorZToServerFrame(
                    from.Z+(to.Z-from.Z)*frac, frameOffset),
            }
            count++
        }
    }

    return out, count
}

// clickServerValidated gates a walk click through the server
// validation port (Navigator.ValidateClick): a click the server would
// cancel never moves the character, so sending it just grinds
// ActionFailed answers until the stuck timeout fires - the town walk
// stuck of 2026-09-10 (the bot clicked the second waypoint 58 units
// over the village plaza corner, the geodata correction collapsed the
// target onto the walker and the character froze through all three
// re-paths). A refused click shortens the segment first (the Bresenham
// prefix of a split segment is not a prefix of the full raster, a shorter
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
    if l.shortenClickSegment(selfX, selfY, selfZ, moveX, moveY, moveZ, from) {
        return true
    }
    if l.clickForwardJump(selfX, selfY, selfZ, moveX, moveY, moveZ, now) {
        return true
    }
    if l.clickEscapeHop(selfX, selfY, selfZ, moveX, moveY, moveZ) {
        return true
    }
    // No local escape works: re-path from the current position like
    // the stuck path does, bounded by the same budget. A re-path
    // from the cell the previous one already planned from (and
    // moved nothing on) proves the fresh plan cannot move the
    // character either: the trip aborts for its callers' recovery.
    if l.noteRepathCell(selfX, selfY) {
        l.abortFrozenTrip(
            "the server refuses the walk click from this cell")

        return false
    }
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.rePaths++
    if l.rePaths > maxRePaths {
        l.abortTownTrip("the server refuses every walk click")

        return false
    }
    l.logger.Printf("Hunt: the server would refuse the walk click to "+
        "%d %d, re-pathing (%d of %d)",
        int32(*moveX), int32(*moveY), l.rePaths, maxRePaths)
    if !l.replanTownWalkSegment(l.segmentDest) {
        // The re-path found no route: the same freeze evidence the
        // stuck path carries - the ladder owns it (see
        // stuckTownWalk).
        l.abortFrozenTrip("re-path failed")
    }

    return false
}

// clickForwardJump answers a refused click with the first later plan
// waypoint whose straight line the server transport validates from
// the standing cell: the corner turn band. The destination
// correction of the previous leg stops the character 8..50 units
// short of the turn pivot (the click transport cannot reach the
// funnel pivot over the corner approach cells), the pass radius
// counts the turn reached, and the turn chord from the stopped
// position cuts the corner - the server collapses it, the shorten
// ladder halves into the same corner and the back hop walks the
// character away from the corner it wants to round (the re-approach
// lands in the same band - the ping pong). A FARTHER waypoint's
// chord clears the corner at a wider angle (the real pack probe of
// the 2026-09-20 delevel round: from the stuck band at 40968 53400
// the turn wp and every forward sample along the outgoing leg refuse
// while wp+2 validates and the pivot neighborhood ring answers 33 of
// 36). The cursor jumps onto the validated waypoint and the move
// pointers carry its corrected destination - the same jump the stuck
// handler runs after its window, moved to click time: the 15 s stuck
// window never opens for a corner the scan answers, and the re-path
// ladder (whose deterministic re-plan reproduces the identical route)
// never burns. It reports whether the pointers carry a validated jump
// target.
func (l *Loop) clickForwardJump(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64, now time.Time,
) bool {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) {
        return false
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for next := l.wpIndex + 1; next < len(l.waypoints); next++ {
        wp := l.waypoints[next]
        to := pathfind.Vec3{
            X: wp.X, Y: wp.Y,
            Z: anchorZToServerFrame(wp.Z, l.segmentFrameOffset),
        }
        corrected, ok := l.navigator.ValidateClick(from, to)
        if !ok {
            continue
        }
        // The jump must move the character a real step: a validated
        // waypoint whose correction lands on the standing cell (the
        // click transport refuses the whole chord but the collapse
        // check passes on a rounding edge) would click the walker's
        // own position - the server cancels it and the stick stays.
        // One geodata cell is the honest step floor.
        if math.Hypot(corrected.X-from.X, corrected.Y-from.Y) <
            extendMarchStep {
            continue
        }
        l.wpIndex = next
        l.moveAt = now
        l.logger.Printf("Hunt: the turn click is walled, jumping the "+
            "cursor to the waypoint %d of %d whose line validates",
            next, len(l.waypoints))
        *moveX, *moveY, *moveZ = corrected.X, corrected.Y, corrected.Z

        return true
    }

    return false
}

// shortenClickSegment halves a refused click segment toward its target until a
// prefix validates: the split segments of a long waypoint line rasterize
// differently than the planned segment (the Bresenham accumulator starts
// from the endpoint deltas), so the far half of a line can fail where
// a shorter prefix passes. It reports whether the move pointers carry
// a validated shorter target.
func (l *Loop) shortenClickSegment(
    selfX, selfY, selfZ int32,
    moveX, moveY, moveZ *float64, from pathfind.Vec3,
) bool {
    dx := *moveX - float64(selfX)
    dy := *moveY - float64(selfY)
    dz := *moveZ - float64(selfZ)
    full := math.Hypot(dx, dy)
    for segment := full / 2; segment >= clickShortenFloor; segment /= 2 {
        frac := segment / full
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
        // The anchored distance: on a shifted deck the raw 3D
        // distance to a bend the character stands on measures the
        // shift alone, past the pass radius - the hop scan would
        // break on the first candidate and never hop (the same
        // frame duality the arrival test answered, see
        // waypointDistanceAnchored).
        dist := waypointDistanceAnchored(wp, selfX, selfY, selfZ,
            l.segmentFrameOffset)
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
            X: wp.X, Y: wp.Y,
            Z: anchorZToServerFrame(wp.Z, l.segmentFrameOffset),
        }); ok {
            *moveX, *moveY, *moveZ = wp.X, wp.Y,
                anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
            l.logger.Printf("Hunt: walk click refused, hopping "+
                "back to the plan bend at %d %d",
                int32(wp.X), int32(wp.Y))

            return true
        }
    }

    return false
}

// segmentAdvanceClear reports whether the follower may advance past the
// waypoint whose successor sits at the index: the straight line from
// the CURRENT character position to that next waypoint must be
// walkable by the SERVER CLICK TRANSPORT - the same oracle the click
// the follower would send for that waypoint passes (Navigator.
// ValidateClick, the GeoEngine.getValidLocation port: the climb limit
// with its layer step-over, the free drops, the source wall plus the
// anti corner cut). The server validates every ground click as a
// straight line (the deployment runs PathFinding = 0 so no server side
// routing exists): a click whose first step hits a closed wall
// resolves to the character's own position, the move is canceled at
// once and the character never moves. The old follower skipped any
// waypoint inside the 50 unit pass radius - tighter than the 16 unit
// ramp steps of the trainer plaza approach - and clicked the far
// waypoint straight through the plaza railing: the click canceled, the
// 15 s stuck detector re-planned the identical deterministic route,
// the follower skipped the same tight waypoints again and the third
// budget burned into "town trip ended: aborted, walk stuck" (the
// 2026-09-11 teacher walk, the lessons never reached the teacher). The
// gate keeps the skipped-from waypoint as the target until walking
// onto it re-opens the line.
//
// The oracle choice is the load bearing contract: the gate and the
// click MUST answer through one semantics. The old gate asked the grid
// engine's symmetric line of sight (the A* node rule: a step must
// climb no more than the passable height up AND down, both cells'
// walls open in both directions) while the clicks obeyed the
// asymmetric server rule - on ordinary terrain (a terrace drop, a one
// sided wall) the two disagree, the cursor pinned on a waypoint whose
// click the server walked fine, the short click extension escaped
// ~60 units out and the pinned cursor clicked the character right
// back - the plaza ping-pong of the 2026-09-20 farm readiness report
// (nine alternating MoveToLocation clicks between two points 63 units
// apart while the server accepted every one of them, the frozen trip
// verdict then banning the walkable plaza for the session). The
// round 57 family pinned the same disagreement from the other side:
// the pinned cursor kept re-clicking the 22 unit first waypoint the
// server's rescue threshold silently canceled while the far segments
// of the very plan validated fine.
//
// The gate mirrors the click exactly the follower would send: the
// waypoint height anchored into the server frame (see
// click_frame.go), the distance capped at the move request limit,
// and the port's own verdict - a click the transport validates walks
// the character SOMEWHERE along the line (a running click may stop
// a hundred units short of the asked cell at a terrace lip; the
// character still made that progress, the arrival test and the next
// click converge from there), a click the transport collapses onto
// the walker (the correction under the cancellation limit) never
// moves a cell and the cursor keeps the skipped-from waypoint
// targeted until walking onto it re-opens the line.
func (l *Loop) segmentAdvanceClear(
    selfX, selfY, selfZ int32, next int,
) bool {
    if next >= len(l.waypoints) || l.navigator == nil {
        return true
    }
    wp := l.waypoints[next]
    targetX, targetY := wp.X, wp.Y
    targetZ := anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
    dx := targetX - float64(selfX)
    dy := targetY - float64(selfY)
    if dist := math.Hypot(dx, dy); dist > maxMoveDistance {
        frac := maxMoveDistance / dist
        targetX = float64(selfX) + dx*frac
        targetY = float64(selfY) + dy*frac
        targetZ = float64(selfZ) + (targetZ-float64(selfZ))*frac
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    corrected, ok := l.navigator.ValidateClick(from, pathfind.Vec3{
        X: targetX, Y: targetY, Z: targetZ,
    })
    if !ok {
        return false
    }
    // The delivery test: the server's correction of the line must
    // land within the waypoint pass radius of the asked target - a
    // click that stops a hundred units short (the terrace lip of the
    // round 57 route, the hall wall of the round 56 aisle approach)
    // has not delivered the waypoint, the cursor keeps the skipped
    // from waypoint targeted and the character walks the plan's bends
    // in the order the planner drew them.
    return math.Hypot(corrected.X-targetX, corrected.Y-targetY) <=
        waypointPassDist
}

// refusalEvidence reports whether the server answered the last walk
// click of this segment with ActionFailed while the character stood
// still: the one byte refusal answer is the only online channel that
// names a server side move refusal (the offline click validation
// port mirrors the reference Mobius master, but a deployment running
// a different build or geodata validates the same click differently
// - the 2026-09-14 10:18 dump: every click of every plan bounced
// while the local reference stack walked the identical scenario
// cleanly, and the dump's unknown packet fingerprints 0x57/53 bytes
// and 0xe7/21 bytes do not exist on the reference master). The
// correlation is the sent click timestamp: the answer arrives after
// the request, and a click the server accepted moves the character
// instead (the stuck verdict calling this already proved it did
// not).
//
// The attribution is honest about the answer's owner: the ActionFailed
// packet carries no request identity, and every other request of the
// session (the equip and skill requests of the gear machinery, the
// transactions) answers ActionFailed the same way. An arrival with a
// non walk request sent after the click belongs to that request at
// least as likely as to the click - the gear swap refusals ride the
// walk clicks exactly this way (the 2026-09-14 12:31 dump rerun: the
// equip ActionFaileds landed one to three seconds after the walk
// clicks of the same tick window) - so the correlation skips them
// instead of latching a walk refusal that never happened.
func (l *Loop) refusalEvidence() bool {
    if l.moveAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    return failedAt.After(l.moveAt) &&
        failedAt.Sub(l.moveAt) <= refusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.moveAt, failedAt)
}

// sendVariedAim answers a stuck verdict with refusal evidence by
// varying the aim at the current waypoint instead of assuming the
// corridor froze: the server refused THIS click, and the refusal is
// target specific - the Bresenham raster of a shorter prefix or a
// sideways offset of the same waypoint often validates where the
// plain aim bounced. The variant list walks from the safest
// variation (half of the segment) to the sideways probes (the
// perpendicular offsets that relocate the character off the refused
// flank). Each variant passes the same offline gates as a planned
// click (the server click port on its line); the first gate-passing
// variant is clicked. It reports whether a
// variant was sent (the caller resets the stuck window for it).
func (l *Loop) sendVariedAim(
    selfX, selfY, selfZ int32, now time.Time,
) bool {
    if l.navigator == nil || l.wpIndex >= len(l.waypoints) ||
        l.refusalVariants >= refusalVariantsMax {
        return false
    }
    wp := l.waypoints[l.wpIndex]
    // The varied aims ride the same server frame transport as the
    // plain clicks (see click_frame.go): the interpolated z of every
    // variant starts from the anchored waypoint height, so the
    // variant that finally validates names the standing surface's
    // layer in the server frame, not the mesh frame the packs
    // disagree about.
    wp.Z = anchorZToServerFrame(wp.Z, l.segmentFrameOffset)
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    for ; l.refusalVariants < refusalVariantsMax; l.refusalVariants++ {
        variant := refusalVariantTarget(
            wp, from, l.refusalVariants)
        if variant == nil {
            continue
        }
        to := pathfind.Vec3{
            X: variant[0], Y: variant[1], Z: variant[2],
        }
        if _, ok := l.navigator.ValidateClick(from, to); !ok {
            continue
        }
        l.refusalVariants++
        l.logf("Hunt: the server refused the walk click, "+
            "varying the aim to %.0f %.0f (%d of %d)",
            to.X, to.Y, l.refusalVariants, refusalVariantsMax)
        l.moveAt = now
        if err := l.game.WalkTo(
            int32(to.X), int32(to.Y), int32(to.Z)); err != nil {
            l.logf("Hunt: town walk request failed: %v", err)
        }

        return true
    }

    return false
}

// refusalVariantTarget computes the varied click target of one
// refusal variant index. The variation works on the CLICK geometry,
// not the raw waypoint distance: the plain click caps at maxMoveDistance
// (the follower's segment splitting), so the variants must sit INSIDE
// that cap or they aim farther than the click that just bounced. On
// the capped segment, variant 0 halves the click (the Bresenham prefix
// of a split segment rasterizes differently than the full line), 1
// quarters it, 2 and 3 bend the half click sideways (left and right
// of the route direction - the probe that relocates the character
// off the refused flank while keeping the forward progress). It
// returns nil when the click is too short to vary.
func refusalVariantTarget(
    wp pathfind.Vec3, from pathfind.Vec3, index int,
) *[3]float64 {
    dx := wp.X - from.X
    dy := wp.Y - from.Y
    full := math.Hypot(dx, dy)
    if full < minWalkClick {
        return nil
    }
    // The click geometry: the plain click caps at maxMoveDistance, the
    // variants vary the click - never the raw waypoint distance.
    click := full
    if click > maxMoveDistance {
        click = maxMoveDistance
    }
    frac := 0.5
    side := 0.0
    switch index {
    case 1:
        frac = 0.25
    case 2:
        side = refusalVariantStep
    case 3:
        side = -refusalVariantStep
    }
    ux, uy := dx/full, dy/full
    x := from.X + ux*click*frac - uy*side
    y := from.Y + uy*click*frac + ux*side
    z := from.Z + (wp.Z-from.Z)*(click*frac/full)

    return &[3]float64{x, y, z}
}

// noteMoveStart is the move start watchdog of the walker: the
// deadline arms when a walk click goes out (the moveAt send slot)
// while the character stands still, a position change or the server's
// own movement broadcast clears it (the click obviously started the
// movement) and a deadline that passes with the character still on
// the baseline cell names the click dead. It reports the fire: the
// caller runs its next recovery mode at once instead of standing out
// the full stuck window - the owner rule of the 2026-09-19 round: a
// movement command that did not start the movement must not leave
// the character standing in one spot while the window burns. The
// window covers the server's once per second movement broadcast gate
// plus the network lag with head room, so an accepted click always
// clears the deadline before it can fire.
func (l *Loop) noteMoveStart(
    now time.Time, selfX int32, selfY int32,
) bool {
    if l.moveStartAt.IsZero() {
        if l.moveAt.IsZero() || l.tracker.SelfWalking() {
            return false
        }
        l.moveStartAt = l.moveAt.Add(moveStartWindow)
        l.moveStartX, l.moveStartY = selfX, selfY
        // Fall through: the arming tick may already sit past the
        // deadline (the click went out a tick before the watchdog
        // first saw it) - the fire below must not wait another
        // window for a verdict it can already name.
    }
    if selfX != l.moveStartX || selfY != l.moveStartY ||
        l.tracker.SelfWalking() {
        l.moveStartAt = time.Time{}

        return false
    }
    if now.Before(l.moveStartAt) {
        return false
    }
    // The click never started the movement: one fire per arming, the
    // deadline re-arms on the next recovery click.
    l.moveStartAt = time.Time{}

    return true
}

// walkStuck tracks the movement progress of the walker and re-paths
// around the obstacle once the character stands still for too long OR
// wobbles without net progress toward the current waypoint. The same
// cell check alone misses the oscillation loops: the aggro steering
// flips its tangent side whenever the segment target sits inside a threat
// circle, and the character ping-pongs between the two tangent
// endpoints - every hop moves, so a plain movement reset never fires,
// while the walk makes no progress at all (the observed delevel walk
// burned an hour between two points 300 units apart). The net progress
// gate resets the window only when the waypoint cursor advanced or the
// current waypoint came measurably closer; anything else that outlives
// the timeout is a stuck and runs the recovery below. The first stuck
// of a trip waits the full stuckTimeout (a slow server position
// broadcast must not trip a false stuck); subsequent stucks within the
// same trip wait the shorter stuckFastTimeout - once the walker knows
// the server refuses its clicks on this segment, waiting the full window
// for every waypoint just burns the trip's time budget. The skip of a
// waypoint does NOT consume the re-path budget: only the full segment
// re-plan (startWalkSegment) does. The move start watchdog force (see
// noteMoveStart, wired by followWaypoints) skips the window check for
// one verdict: a click that never started the movement runs the
// recovery at once. It reports whether the trip had to abort.
func (l *Loop) walkStuck(now time.Time, selfX int32, selfY int32) bool {
    forced := l.forceStuck
    l.forceStuck = false
    if l.stuckAt.IsZero() {
        l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
        l.stuckWP = l.wpIndex
        l.stuckBest = l.stuckWaypointDistance(selfX, selfY)

        return false
    }
    if l.stuckProgressed(selfX, selfY) {
        l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
        l.stuckWP = l.wpIndex
        l.stuckBest = l.stuckWaypointDistance(selfX, selfY)

        return false
    }
    timeout := stuckTimeout
    if l.stuckFast {
        timeout = stuckFastTimeout
    }
    if !forced && now.Sub(l.stuckAt) < timeout {
        return false
    }
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.stuckWP = l.wpIndex
    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)

    return l.stuckTownWalk(now, selfX, selfY)
}

// stuckWaypointDistance measures the planar distance from the position
// to the current waypoint (0 without a plan: the follower only runs
// over a planned segment).
func (l *Loop) stuckWaypointDistance(selfX int32, selfY int32) float64 {
    if l.wpIndex < 0 || l.wpIndex >= len(l.waypoints) {
        return 0
    }
    wp := l.waypoints[l.wpIndex]

    return math.Hypot(wp.X-float64(selfX), wp.Y-float64(selfY))
}

// stuckProgressed reports whether the walk made net progress since the
// stuck window opened: the waypoint cursor advanced AND the character
// moved (a cursor bump alone - a passed waypoint of a freshly planned
// route - is plan bookkeeping, not ground covered), the character
// moved and the current waypoint came closer by the progress margin,
// or the character moved and the SEGMENT DESTINATION came closer by
// the margin. The destination term is the pinned cursor's honest
// progress signal: the advance gate pins the cursor on a
// waypoint whose sight line ahead is walled (the 2026-09-20
// acceptance round: every line of the Ellenia corridor ahead of the
// plan start was walled for the sight oracle), the armed extension
// then marches the character forward along the route samples while
// the cursor waits - the aimed waypoint sits BEHIND the marching
// character and its distance GROWS, so the waypoint term alone
// judged the healthy march a stuck and burned the re-path budget on
// a walk that was covering ground every tick. The destination
// distance shrinks monotonically on that march and grows on a real
// oscillation (the one step away and return loop lands no closer),
// so the term separates the two exactly. A standstill and an
// oscillation both still fail the gate; a detour climb passes it
// (every waypoint of the route approaches in turn).
func (l *Loop) stuckProgressed(selfX int32, selfY int32) bool {
    moved := selfX != l.stuckX || selfY != l.stuckY
    if !moved {
        return false
    }
    if l.wpIndex != l.stuckWP {
        return true
    }

    waypointCloser := l.stuckWaypointDistance(selfX, selfY) <
        l.stuckBest-stuckProgressUnits

    return waypointCloser || l.stuckDestinationProgressed(selfX, selfY)
}

// stuckDestinationProgressed reports whether the segment destination
// came measurably closer since the stuck window opened WHILE the
// character stays inside the route corridor: the ground covered along
// the planned route counts as progress even when the cursor is pinned
// on a waypoint the character already marched past (see
// stuckProgressed for the pinned march round), while an off-route hop
// that merely happens to point destination-ward (the tangent
// flip-flop's sideways bounce) is no progress at all - the corridor
// test keeps the term honest exactly there.
func (l *Loop) stuckDestinationProgressed(selfX int32, selfY int32) bool {
    if !l.stuckOnRouteCorridor(selfX, selfY) {
        return false
    }

    return l.stuckDestinationDistance(selfX, selfY) <
        l.stuckDestinationDistance(l.stuckX, l.stuckY)-stuckProgressUnits
}

// stuckOnRouteCorridor reports whether the position sits within the
// lateral corridor of the remaining route (the aimed waypoint's
// segment and every segment after it): the route the follower plans
// is the ground the march may cover, the sideways bounce off it is
// not.
func (l *Loop) stuckOnRouteCorridor(selfX int32, selfY int32) bool {
    for i := l.wpIndex; i+1 < len(l.waypoints); i++ {
        wp := l.waypoints[i]
        next := l.waypoints[i+1]
        segX := next.X - wp.X
        segY := next.Y - wp.Y
        segLen := math.Hypot(segX, segY)
        if segLen < 1 {
            continue
        }
        selfDX := float64(selfX) - wp.X
        selfDY := float64(selfY) - wp.Y
        along := (selfDX*segX + selfDY*segY) / (segLen * segLen)
        lateral := math.Abs(selfDX*segY-selfDY*segX) / segLen
        if along >= 0 && along <= 1 && lateral <= waypointCorridor {
            return true
        }
    }

    return false
}

// stuckDestinationDistance measures the planar distance from a
// position to the segment destination (0 without a segment).
func (l *Loop) stuckDestinationDistance(selfX int32, selfY int32) float64 {
    if l.segmentDest.X == 0 && l.segmentDest.Y == 0 &&
        l.segmentDest.Z == 0 {
        return 0
    }

    return math.Hypot(l.segmentDest.X-float64(selfX),
        l.segmentDest.Y-float64(selfY))
}

// pocketRefused reports whether the current refusal verdict stands on
// the very cell where the segment's first refusal latched: the ground the
// character stands on refuses every click (the refusing pocket of
// the 2026-09-14 15:10 report), not one specific click line.
func (l *Loop) pocketRefused(selfX int32, selfY int32) bool {
    return l.segmentRefused &&
        selfX == l.segmentRefusedX && selfY == l.segmentRefusedY
}

// currentWaypoint returns the waypoint the follower cursor aims at.
func (l *Loop) currentWaypoint() (pathfind.Vec3, bool) {
    if l.wpIndex < 0 || l.wpIndex >= len(l.waypoints) {
        return pathfind.Vec3{X: 0, Y: 0, Z: 0}, false
    }

    return l.waypoints[l.wpIndex], true
}

// stuckTownWalk drives the town segment stuck recovery: first try to SKIP
// the current waypoint (a further one may be reachable through a cell
// the server accepts), and when no waypoint ahead has a walkable line
// from the standing cell, re-plan the whole segment from the current
// position. The skip does NOT consume the re-path budget - it advances
// the cursor without re-planning, so the walker can skip several
// waypoints in a row while looking for one the server accepts. The
// skip only jumps onto a waypoint whose straight line from the
// standing cell is walkable (the same gate the cursor advance
// applies): a blind skip arms the follower with a target whose click
// the server collapses partway - the Bresenham line stops at the
// first walled flank and the partial click creeps the character cell
// by cell toward the nearest trap pocket instead of walking the
// route (the 2026-09-11 06:19 aisle dump: the skip jumped from the
// aisle entrance onto the east hall waypoint, every click crept the
// character 16 units east into the dead-end pocket cell at 44776
// 51992 and the pocket's closed east wall then refused the click
// wholesale - "the server would refuse the walk click" - burning the
// re-path budget on the recovery). The fast timeout flag arms after
// the first skip so subsequent stuck detections fire on the shorter
// window.
//
//nolint:funlen // the town walk repeats the stuck checks per segment
func (l *Loop) stuckTownWalk(now time.Time, selfX int32, selfY int32) bool {
    // The online refusal answer separates the server side refusal
    // from the corridor freeze: an ActionFailed that answered the
    // sent click while the character stood still names a refusal
    // the offline validation cannot see (a different server build
    // or geodata), and the refusal is target specific - varying the
    // aim at the same waypoint (a shorter prefix, a sideways
    // offset) often walks where the plain click bounced. The
    // refusal branch runs BEFORE the waypoint skip: the skip aims a
    // FARTHER waypoint (a worse target for a length sensitive
    // refusal), the variation keeps the near aim the plan already
    // holds. Latching the refusal also arms the pocket gate below:
    // a segment whose every aim the server refused on the very
    // standing cell hands the walk to the cursor key escape at once.
    if l.refusalEvidence() {
        if !l.segmentRefused {
            l.segmentRefused = true
            l.segmentRefusedX, l.segmentRefusedY = selfX, selfY
            l.logf("Hunt: the server refused the walk click " +
                "(ActionFailed), varying the aim")
        }
        // The varied aims run first: the refusal is target specific
        // far more often than it is a pocket (the round 82 evidence),
        // one variant per stuck verdict - a variant the server
        // accepts walks the character where the plain aim bounced.
        if l.sendVariedAim(selfX, selfY, l.stuckSelfZ(), now) {
            l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
            l.stuckWP = l.wpIndex
            l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
            l.stuckFast = true

            return false
        }
        // The refusing pocket: the varied aims of this segment are spent
        // (or nothing exists to vary) and the character still stands
        // on the very cell where the first refusal latched - the
        // server answers NO click from this ground (the 2026-09-14
        // 15:10 report: even the official client's mouse clicks died
        // on the plaza cell). The cursor key escape owns the recovery
        // now, walking the claims along the planned route (see
        // beginCursorKeyEscape) - more refused clicks from the same
        // ground would only burn the trip budget.
        if l.pocketRefused(selfX, selfY) {
            if wp, ok := l.currentWaypoint(); ok {
                if l.beginCursorKeyEscape(selfX, selfY,
                    l.stuckSelfZ(), int32(wp.X), int32(wp.Y),
                    int32(wp.Z)) {
                    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
                    l.stuckWP = l.wpIndex
                    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
                    l.stuckFast = true

                    return false
                }
            }
        }
    }
    next := l.nextClearWaypoint(selfX, selfY, l.stuckSelfZ())
    if next > l.wpIndex {
        l.wpIndex = next
        l.moveAt = time.Time{}
        l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
        l.stuckWP = l.wpIndex
        l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
        l.stuckFast = true
        l.logger.Printf("Hunt: town walk stuck, skipping waypoint "+
            "(cursor %d of %d)", l.wpIndex, len(l.waypoints))

        return false
    }
    if l.noteRepathCell(selfX, selfY) {
        // The previous re-path started from this very cell and
        // its fresh plan moved the character nowhere: the next
        // re-path would plan the identical route into the same
        // refusal (see frozenRepathLimit). The trip ends and
        // the recovery hands over to its callers.
        l.abortFrozenTrip("walk stuck, no movement since the re-path")

        return true
    }
    // The stuck found no clear successor (the pinned cursor): the
    // plain waypoint clicks of this segment do not move the character
    // - arm the short click extension for the recovery clicks.
    l.extendArmed = true
    l.rePaths++
    if l.rePaths > maxRePaths {
        l.abortTownTrip("walk stuck")

        return true
    }
    l.logf("Hunt: town walk stuck, re-pathing (%d of %d)",
        l.rePaths, maxRePaths)
    if l.journal != nil {
        l.journal.Repath(l.tracker.ID(), l.rePaths)
    }
    if !l.replanTownWalkSegment(l.segmentDest) {
        // The fresh plan found no route (the water may wall every dry
        // one): the freeze evidence still belongs to the escalation
        // ladder - the escape arms over the current plan instead of
        // aborting the trip back into the identical cycle (the
        // 2026-09-14 08:42 dump: the re-path failure aborted past the
        // ladder, the corridor route returned through the fallback
        // and the bot looped on it forever).
        l.abortFrozenTrip("re-path failed")

        return true
    }
    // The re-path proved the plain clicks of this segment do not move
    // the character: arm the fast stuck window for the next detection
    // (4 s instead of 15 s) and re-baseline the stuck window from
    // the re-path tick, so the next stuck fires on the fast timeout
    // the same way the waypoint skip arm does. Without this the
    // first re-path waits the full 15 s before the next detection
    // - the dump of 2026-09-14 08:13 (build c7a0855, bot test3)
    // showed the bot sitting at 45768 49848 -3056 through the whole
    // stuckTimeout after a re-path that did not move it a cell, the
    // recovery burned 30 s of the trip budget on a freeze the fast
    // window would have caught in 4 s.
    l.stuckAt, l.stuckX, l.stuckY = now, selfX, selfY
    l.stuckWP = l.wpIndex
    l.stuckBest = l.stuckWaypointDistance(selfX, selfY)
    l.stuckFast = true

    return false
}

// noteRepathCell records the cell a stuck re-path plans from and
// reports whether the previous re-path of this trip started from the
// same cell without a single cell of movement in between: the
// deterministic planner re-plans the identical route from the same
// standing position, so the freeze the previous plan could not move
// through is exactly the freeze the next one re-clicks into (the
// 2026-09-11 11:34 dump: three identical re-paths per trip, two trip
// cycles, the character never moved a cell). Any movement between the
// re-paths clears the counter - a different plan shape then has a
// different first segment to try.
func (l *Loop) noteRepathCell(selfX int32, selfY int32) bool {
    frozen := selfX == l.repathX && selfY == l.repathY
    if frozen {
        l.frozenRepaths++
    } else {
        l.frozenRepaths = 0
    }
    l.repathX, l.repathY = selfX, selfY

    return l.frozenRepaths >= frozenRepathLimit
}

// abortFrozenTrip ends a trip whose re-path produced no movement and
// escalates the recovery of the frozen segment before giving up: the town
// walk segments and the zone return segments both hand the walk to the
// cursor key escape along the plan (see escalateFrozenSegment). Without
// the escape the zone return cycles between the
// pathfound-return-stuck-abort and the frozen
// re-plan, never moving: the deterministic planner re-plans the
// identical route the ground keeps refusing (the 2026-09-14 08:25
// dump, build d2ea298: the bot stood at the village terrace for 34s
// after the abort, no walk sent, no Hunt log). The shopping trips keep
// their cooldown recovery when the ladder is exhausted: the hunt
// continues and the next trip retries from a fresh state.
func (l *Loop) abortFrozenTrip(reason string) {
    if l.escalateFrozenSegment() {
        return
    }
    l.abortTownTrip(reason)
}

// escalateFrozenSegment arms the cursor key escape of a frozen town
// walk segment - a segment whose full re-path cycle produced no
// movement at all, the signature of a ground the click transport
// cannot cross (the 2026-09-12 trainer hall aisle dump: the plan
// entered the building through the west aisle column, the server
// walled it, and the character stood frozen through every re-path of
// two whole trips).
//
// The claims transport takes the walk over: the keyboard mode 0 arm
// plus the claimed ValidatePosition stream the server follows without
// any click validation (the 2026-09-14 15:10 report: the official
// client's mouse clicks died on the refusing cell while the ARROW
// KEYS walked it out - the claims are the only movement a click
// refusing ground answers) and walks the character ALONG THE PLANNED
// ROUTE - the plan stays the segment's own route, the claims bend
// where it bends, and the settle returns the walk to the normal
// routed clicks on the same plan (the owner contract of the
// 2026-09-19 round: wasd along the route, the normal mode at the
// point, NEVER walk the direct line - the rung never replaces the
// plan with a straight line to the far target). The re-arm repeats
// while the trip's escape attempts last (cursorEscapeAttemptsMax); a
// spent budget falls back to the plain trip abort with its cooldown.
//
// The historical rung this ladder replaced - the frozen corridor ban
// that sealed the walled waypoint's cells into every later search -
// is gone deliberately: its rectangle granularity walled the whole
// merged mesh sheet any ban touched (the plaza sheet of the
// 2026-09-20 farm readiness report spans 368x32 units around the ban
// center; the reachable world collapsed from 1.3M polygons to 2 and
// every later route search of the session answered "no path at
// all"), its trigger read the follower's own cursor pinning as a
// freeze (the advance gate disagreed with the click transport - see
// segmentAdvanceClear) and its widening (48 -> 1536 units) compounded
// every misfire into a sealed village quarter. The escape owns the
// freeze recovery alone: it walks the character through, it never
// poisons a later search, and its budget bounds itself.
//
// The ladder reports whether the escape took over the recovery (the
// caller skips its abort).
func (l *Loop) escalateFrozenSegment() bool {
    if l.navigator == nil {
        return false
    }
    // The ladder runs for both the town walk segments (phaseTownWalk) and
    // the zone return segments (phaseTownReturn): both follow a planned
    // geodata route whose clicks the click transport may refuse, and
    // both need the claims walk off the refusing ground (the
    // 2026-09-14 08:25 dump: the zone return at the village terrace
    // cycled between the pathfound-return-stuck-abort and the frozen
    // segments because the ladder never ran for it).
    if l.phase != phaseTownWalk && l.phase != phaseTownReturn {
        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    if wp, hasWp := l.currentWaypoint(); hasWp {
        if l.beginCursorKeyEscape(selfX, selfY, selfZ,
            int32(wp.X), int32(wp.Y), int32(wp.Z)) {
            return true
        }
    }

    return false
}

// nextClearWaypoint scans the plan ahead for the first waypoint the
// standing cell can click directly: the stuck skip must only arm
// targets the server walks, never a line it would collapse partway
// into a trap cell (the segmentAdvanceClear gate of the cursor advance).
// It returns the index of the first clear successor, or the current
// cursor when no successor ahead is reachable.
func (l *Loop) nextClearWaypoint(selfX, selfY, selfZ int32) int {
    for next := l.wpIndex + 1; next < len(l.waypoints); next++ {
        if l.segmentAdvanceClear(selfX, selfY, selfZ, next) {
            return next
        }
    }

    return l.wpIndex
}

// stuckSelfZ returns the current character z for the stuck recovery:
// the recovery needs the full position and the stuck tracker only
// carries x and y, so the z comes from the tracker on demand (0 when
// the position is not known yet - the planner resolves the layer of
// the standing cell anyway).
func (l *Loop) stuckSelfZ() int32 {
    _, _, z, ok := l.tracker.SelfPosition()
    if !ok {
        return 0
    }

    return z
}

// enterSellPhase switches into the selling and shopping state at the
// shop. The log names the stop honestly: the first stop sells the
// junk, the buy stops of the frozen plan only trade.
func (l *Loop) enterSellPhase() {
    l.phase = phaseTownSell
    l.sellPhaseAt = time.Now()
    l.sellAt = time.Time{}
    l.buyAt = time.Time{}
    l.merchantID = 0
    l.merchantPick = time.Time{}
    l.merchantDeckUntil = time.Time{}
    if l.sellableStop() {
        l.logf("Hunt: shop reached, selling the junk")

        return
    }
    if len(l.tripStops) > 0 {
        l.logf("Hunt: shop reached at %s",
            l.tripStops[0].merchant.Name)

        return
    }
    l.logf("Hunt: shop reached")
}

// tickTownSell runs the sell stop (the first trip stop) and the buy
// stops. EVERY vendor trip sells the whole accumulated junk - the
// selling ends when nothing sellable is left, not when the inventory
// drops below the trip trigger (a bag of 30 percent junk on a buy
// trip still sells, or the bot would farm with it and walk back for
// the sale later). The sell first step banks the credits the frozen
// trip plan counted on, the stop planning distributes its purchases,
// every buy stop completes when its purchases were requested, and
// the return segment starts when no stop is left.
//
//nolint:cyclop,gocognit // the town sell trip segments
func (l *Loop) tickTownSell() {
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
            if !l.handleMerchant(now, l.merchantTemplates()) {
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
            l.logf("Hunt: shop: junk sold (%d slots left, "+
                "%.0f%% weight), distributing the trip plan",
                stats.Slots, stats.WeightPercent)
            l.planShoppingStops()
            // The learning stops close the trip: the books and
            // the teacher ride BEHIND the gear stops, so one
            // town visit buys the weapon, the armor, the jewels,
            // the books and teaches the lessons (the user rule
            // of the one town visit - the books stop merges with
            // the gear stop of its merchant when they match, see
            // planLearnStops). The weapon stop already ran (the
            // sell stop routes to the weapon merchant), so an
            // abort on the teacher segment never strands a
            // bare-handed character.
            l.planLearnStops()
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
    l.startReturnSegment()
}

// handleMerchant approaches the shop merchant and selects it like the
// official client does before a transaction. It reports false while the
// character still walks toward the merchant or waits for one to appear.
// The sale itself works without a merchant (the standard inventory sell
// list), so a merchant that never shows up only delays it; the buys
// need the merchant, their stops skip the purchases instead.
//
// The selected npc must trade what the wanted templates name: the sell
// phase passes every town merchant (the junk sells to any vendor), the
// buy stops pass their own trader - the server resolves the buy
// through the targeted folk npc and silently refuses the list the
// targeted npc does not trade, so a selection left over from the sell
// phase (the nearest vendor, often the armor trader standing next to
// the weapon shop) re-picks here instead of aiming the weapon buy at
// the armor trader.
func (l *Loop) handleMerchant(now time.Time, templates []int32) bool {
    if l.merchantID < 0 {
        return true
    }
    if l.merchantID > 0 {
        if merchantTemplateWanted(
            l.tracker.ObjectTemplateID(l.merchantID), templates) {
            return l.approachMerchant(now)
        }
        l.logf("Hunt: shop: %s does not sell this stop's goods, "+
            "re-picking the merchant", l.tracker.ObjectName(l.merchantID))
        l.merchantID = 0
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
        l.logf("Hunt: trading with " + merchant.Name)

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
// geodata (the disconnected village decks) hands the close walk to
// the server - the select + attack analog pull ladder of the deck
// window walks the straight line the pack cannot; only when the
// window burns out is the merchant skipped (the sale does not need
// the merchant, the buys of its stop do).
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
            // the z is not). The ground clicks cannot close a z gap
            // (clicking the merchant's exact cell here teleported the
            // bot onto the roof, the 2026-09-11 report), so the deck
            // window bounds the select + attack analog pull ladder
            // that hands the walk to the server before the merchant
            // is given up.
            if l.merchantDeckUntil.IsZero() {
                l.merchantDeckUntil = now.Add(merchantDeckWindow)
                l.logf("Hunt: %s stands on another deck (z %d vs "+
                    "%d), the attack analog pull walks the straight "+
                    "line",
                    l.tracker.ObjectName(l.merchantID), selfZ, z)
            }
            if now.Before(l.merchantDeckUntil) {
                // The deck walk the ground clicks cannot close: the
                // select + attack analog pull ladder hands the walk
                // to the server - the interaction distance is met
                // however the deck geometry sits between them.
                l.merchantDeckPullLadder(now)

                return false
            }
            l.logf("Hunt: %s stays out of reach, the sells work "+
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

// merchantDeckPullLadder runs the select + attack analog pull ladder
// of the merchant deck wait. The plain click selects the npc at any
// distance, the pull click on the selected npc takes the interact
// intention and the server walks the character to the npc along the
// straight line (one click per select period - the player action
// flood protector).
func (l *Loop) merchantDeckPullLadder(now time.Time) {
    if now.Sub(l.merchantPull) < selectPeriod {
        return
    }
    l.merchantPull = now
    var err error
    if l.tracker.SelfTargetID() != l.merchantID {
        err = l.game.ClickObject(l.merchantID)
    } else {
        err = l.game.InteractPull(l.merchantID)
    }
    if err != nil {
        l.logf("Hunt: merchant pull failed: %v", err)
    }
}

// selectMerchant re-requests the merchant selection once per select
// period until the tracker confirms it, then fires the attack analog
// pull once and reports ready. The transactions need the merchant as
// the selected target (RequestBuyItem checks it server side); the
// pull is the practice of every npc interaction - the second plain
// click on the selected merchant (the client double click) hands the
// last stretch to the server: the interact intention walks the
// character to the merchant along the straight line, so the
// transaction distance is met with certainty. Within the interaction
// distance the same click only opens the merchant dialog - harmless,
// the transactions pace past it.
func (l *Loop) selectMerchant(now time.Time) bool {
    if l.tracker.SelfTargetID() != l.merchantID {
        // The transactions need the merchant as the selected target
        // (RequestBuyItem checks it server side): re-request the
        // selection once per second until the tracker confirmed it
        // and only then report ready.
        if now.Sub(l.merchantPick) >= selectPeriod {
            l.merchantPick = now
            if err := l.game.AttackTarget(l.merchantID); err != nil {
                l.logf("Hunt: merchant select failed: %v", err)
            }
        }

        return false
    }
    // The attack analog pull of the interaction practice: once per
    // merchant (the field compares object ids, a re-pick re-arms it).
    if l.merchantPulled != l.merchantID {
        l.merchantPulled = l.merchantID
        if err := l.game.InteractPull(l.merchantID); err != nil {
            l.logf("Hunt: merchant pull failed: %v", err)
        }
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
    if !l.transactionWindowFree(now) {
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
        l.startReturnSegment()

        return
    }
    if err := l.game.SellItems(batch); err != nil {
        l.logf("Hunt: sell request failed: %v", err)

        return
    }
    for _, item := range batch {
        l.sold[item.ObjectID] = true
    }
    if l.journal != nil {
        l.journal.Sell(l.tracker.ID(), len(batch))
    }
    l.logf("Hunt: offered %d items for sale", len(batch))
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
    l.logf("Hunt: engaging %s on the zone entry", pick.Name)

    return true
}

// startReturnSegment plans the walk back to the farm spot. The segment is a
// fresh logical unit of the trip machinery (the sell stop handed the
// walk over after the shopping, the deleveling aborted into it), so
// the frozen re-path cell of whatever walk came before dies here: the
// return segment must not inherit the frozen budget of the guard walk or
// the sell approach, or its own first refused click ends it instantly
// - the delevel abort and the return segment of the 2026-09-12 01:50 dump
// died back to back from the same cell in one second, leaving the
// character to the direct zone segments and the permanent freeze.
// startReturnSegment plans the walk back to the spot the trip left
// (the farm spot, the zone center it fell back to). The destination z
// resolves onto the deck the destination actually sits on before the
// search: the remembered spot z is the character's own standing z of
// another area, and a plan remembered on the village deck rides a
// zone center x/y whose deck lies hundreds of units lower - the mesh
// search binds the destination polygon inside the nearest window of
// the destination z (query.go nearestHalfZ), misses it by hundreds of
// units and answers the honest "no navmesh under the position"
// forever (the farm readiness round of 2026-09-20: every return of
// the trip aborted on exactly that error while the same query with
// the resolved deck answered 27 waypoints). The resolution mirrors
// the zone return goal (zoneReturnDestination); a lookup failure
// keeps the remembered z - the same-deck case it answers correctly.
func (l *Loop) startReturnSegment() {
    l.phase = phaseTownReturn
    l.repathX, l.repathY = 0, 0
    l.frozenRepaths = 0
    destX, destY, destZ := l.farmX, l.farmY, l.farmZ
    if zone := l.zone(); zone != nil &&
        (!zone.Contains(destX, destY) || (destX == 0 && destY == 0)) {
        // The farm spot belongs to a previous square (a zone switch
        // mid trip): return to the new center instead.
        destX, destY = zone.CX, zone.CY
    }
    destZ = l.resolveDestinationDeck(destX, destY, destZ)
    dest := pathfind.Vec3{
        X: float64(destX),
        Y: float64(destY),
        Z: float64(destZ),
    }
    l.segmentRadius = tripApproachRadius
    if !l.startWalkSegment(dest) {
        l.abortTownTrip("no walkable path back to the farm spot")

        return
    }
    l.logf("Hunt: walking back to the farm spot")
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

// endTownTrip finishes the trip and arms the trigger cooldown. The
// frozen trip plan dies with it: the next trip freezes a fresh one
// against the gear the purchases reached.
func (l *Loop) endTownTrip(reason string) {
    l.clearTalkedTarget()
    // The trip answer for the gear debt: whatever kept the trip
    // from landing the replacements (a refused buy, an abort, a
    // session death the relogin resumed), the exits compare the
    // reached paperdoll against the trip start here.
    l.gearDebtCheck()
    l.phase = phaseEngage
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.waypoints = nil
    l.segmentDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentFrameOffset = 0
    l.extendArmed = false
    l.cursorEscape = zeroCursorEscape()
    l.cursorEscapes = 0
    l.repathX, l.repathY = 0, 0
    l.frozenRepaths = 0
    l.moveStartAt = time.Time{}
    l.forceStuck = false
    l.segmentRefusedX, l.segmentRefusedY = 0, 0
    l.tripPlan = nil
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
    l.noteTripAbortRun(reason)
    if l.journal != nil {
        l.journal.TripEnd(l.tracker.ID(), reason,
            time.Since(l.tripStart))
    }
    l.logf("Hunt: town trip ended: " + reason)
}

// noteTripAbortRun maintains the consecutive abort streak of the trip
// cooldown escalation: an aborted trip grows it, every other ending
// (a completed sell, a farm spot return, a death handover) resets it.
// The escalation log lands once per growth past the threshold so the
// operator sees the cooldown change.
func (l *Loop) noteTripAbortRun(reason string) {
    if strings.HasPrefix(reason, tripAbortPrefix) {
        l.tripAbortRun++
        if l.tripAbortRun > tripAbortEscalateAfter {
            l.logf("Hunt: %d aborted trips in a row, the next "+
                "trip waits %s", l.tripAbortRun,
                tripAbortCooldown(l.tripAbortRun))
        }

        return
    }
    l.tripAbortRun = 0
}

// tripAbortPrefix marks the aborted trip endings of the streak.
const tripAbortPrefix = "aborted, "

// tripAbortCooldown renders the escalating cooldown of an abort
// streak: the base cooldown for the tolerated prefix, then a doubling
// per abort capped at the hour.
func tripAbortCooldown(run int) time.Duration {
    cooldown := tripCooldown
    for range max(0, run-tripAbortEscalateAfter) {
        cooldown *= 2
        if cooldown >= tripAbortMaxCooldown {
            return tripAbortMaxCooldown
        }
    }

    return cooldown
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
// The gear debt check runs here too: the interrupt that drops the
// trip (an attacker mid trip) leaves the sold pieces sold - a slot
// the sell first step already emptied stays a debt even though the
// trip never reached its end.
func (l *Loop) resetTownTrip() {
    if !l.tripActive() {
        return
    }
    l.gearDebtCheck()
    l.phase = phaseEngage
    l.target = 0
    l.clearBlindRecovery()
    l.lootID = 0
    l.waypoints = nil
    l.segmentDest = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
    l.segmentFrameOffset = 0
    l.extendArmed = false
    l.cursorEscape = zeroCursorEscape()
    l.cursorEscapes = 0
    l.repathX, l.repathY = 0, 0
    l.frozenRepaths = 0
    l.moveStartAt = time.Time{}
    l.forceStuck = false
    l.segmentRefusedX, l.segmentRefusedY = 0, 0
    l.tripPlan = nil
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
        l.logf("Hunt: stand up failed: %v", err)

        return false
    }
    l.restActionAt = now
    l.restActionSit = false

    return false
}
