// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "time"
)

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
    // flowAimFloor is the lower bound of the continuous follower's
    // click span: the run speed of a fresh character walks this far
    // in two request periods, so the target is never reached before
    // the next click re-aims farther.
    flowAimFloor = 300.0
    // flowAimPeriods is the number of walk request periods the click
    // span covers: two periods keep a full period of head room for
    // the next click to re-aim before the character reaches the
    // target and the server stops it.
    flowAimPeriods = 2.0
    // flowProbeMax bounds the click validation probes of one flow
    // scan: the scan runs at the click cadence (once per request
    // period plus the corner pre-click), never per tick.
    flowProbeMax = 8
    // flowCornerLead is the distance to the current click target at
    // which the pre-click fires: the character is about to arrive and
    // the server would stop it, so a farther validated target
    // replaces the destination mid-walk. About one third of a
    // request period of walking, so the pre-click still lands while
    // the move runs.
    flowCornerLead = 80.0
    // flowPreClickGap bounds the pre-click cadence: at most one
    // pre-click per half request period, so a tight corridor cannot
    // turn the follower into a click storm.
    flowPreClickGap = walkRequestPeriod / 2
    // flowLayerTolerance bounds the vertical deviation of a skipped
    // route point from the click chord: a deeper dip is a priced
    // water crossing or a ramp the plan walks through on purpose,
    // and the straight click must not cut it.
    flowLayerTolerance = 100.0
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
