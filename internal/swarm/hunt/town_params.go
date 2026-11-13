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
    // sellPause paces the sell requests through the shared transaction window
    // of the server flood protector (see transactionPause in shopping.go for
    // the server research: the window is 1 s wide, refusals cost nothing, the
    // 11 second margin of the early rounds made the junk selling crawl).
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
    // npcInteractionDist is the server INTERACTION_DISTANCE of 250: the talk
    // click and the transactions succeed within this 3D distance. The approach
    // walk aims the offset ring at 150 units 2D, and a small z gap (the trainer
    // hall floor is 40 units above the deck) keeps dist3D above the approach
    // gate (200) but within this gate, so the talk click fires from the ring
    // instead of waiting for a gate the z gap keeps unreachable.
    npcInteractionDist = 250.0
    // npcApproachRadius is the geodata search radius of every npc
    // destination walk (the merchant, teacher and delevel guard stops):
    // the plan must end at the npc's own point, at most this far short.
    // A wide radius ends the plan at the first walkable surface inside
    // its ball - the shop edge up to the whole radius away - and the
    // character never enters the shop. A merchant cell with no modeled
    // floor or behind a counter is reached through the partial answer:
    // the funnel ends at the closest reachable point.
    npcApproachRadius = 10.0
    // tripApproachRadius is the geodata search radius the non npc trip
    // walks (the farm spot return, the zone return) end within: the
    // walk home ends wherever the ball around the spot catches the
    // route, the hunt tick corrects the rest on the spot.
    tripApproachRadius = 200.0
    // merchantFindRadius is the radius around the character within which the
    // spawned merchant npc is looked up once the shop point is reached.
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
    // npcApproachOffset is the 2D distance the bot stops short of a
    // town npc on the approach walk's ground click: the click targets a
    // point this many units short toward the bot, keeping the click
    // line outside the building walls. The server getValidLocation
    // walks a Bresenham line that steps over onto the roof layer when
    // the target is the npc's own cell inside a building (the Cobendell
    // roof answers z -2456 instead of the floor at -2792), so the offset
    // keeps the click on the deck within the 250 unit interaction range.
    npcApproachOffset = 150.0
    // exactSegmentDeckTolerance bounds the z gap between the merchant
    // spawn z and the plan end cell of the exact merchant segment: the
    // customer cell sits on the merchant's own floor scale (the elven
    // stall fronts answer 0-3 units off, the trainer hall decks 40),
    // a roof layer over the shop sits hundreds above - the foreign
    // deck plan is discarded for the ring fallback instead of walked.
    exactSegmentDeckTolerance = 96.0
    // exactSegmentMaxDistance bounds how far from the merchant the exact
    // segment may plan: it serves the final approach (the customer cell across
    // the counter), while the priced ring search owns the long haul. An exact
    // search at a far unreachable destination degenerates into an exhaustive
    // exploration of the whole mesh component, so the gate keeps its partial
    // exhaustion local and bounded.
    exactSegmentMaxDistance = 2000.0
)
