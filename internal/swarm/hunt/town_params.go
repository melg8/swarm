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
    // tripAbortEscalateAfter is the abort streak the trip cooldown
    // tolerates before it starts doubling: two identical aborts can
    // be transient (a mob camped on the shore line, a slow server),
    // a third names a deployment the retry cannot fix.
    tripAbortEscalateAfter = 2
    // tripAbortMaxCooldown caps the escalation of the abort streak.
    tripAbortMaxCooldown = time.Hour
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
