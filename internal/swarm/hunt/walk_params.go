// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "time"
)

// The walk machinery parameters: the click pacing, the waypoint radii, the
// stuck and refusal windows and the cursor key escape, shared by the town
// trips, the manual walks, the quest trips, the zone returns and the delevel
// walks (docs/hunting.md, the walk machinery of the town trips).
const (
    // walkRequestPeriod paces the ground click walks of the waypoint follower
    // and the merchant approach.
    walkRequestPeriod = 2 * time.Second
    // refusalAnswerWindow bounds how long after a walk click an ActionFailed
    // still counts as its answer; it also covers a slow answer racing the next
    // request of the period.
    refusalAnswerWindow = 4 * time.Second
    // waypointArriveDist is the arrival radius of the final waypoint: two
    // geodata cells of slack, so a segment ends even when the server stops the
    // character slightly short of the clicked point.
    waypointArriveDist = 150.0
    // waypointPassDist is the tighter arrival radius of the intermediate
    // waypoints: a bridge ramp entry or a detour turn must be walked through,
    // not seen from the side (three cells of slack).
    waypointPassDist = 50.0
    // waypointCorridor is the lateral distance from the route within which a
    // character counts as having passed a waypoint; a character standing beside
    // the route has passed nothing and walks back to the entry it missed.
    waypointCorridor = 100.0
    // maxMoveDistance splits the walk segments under the server refusal of move
    // requests farther than 9900 units (MoveToLocation readImpl).
    maxMoveDistance = 1000.0
    // returnWalkSegment caps the direct walk requests of the zone return under
    // the same 9900 unit refusal: a village respawn or a long chase easily
    // starts farther than that from the zone center.
    returnWalkSegment = 1000.0
    // stuckTimeout is how long the character may stand still on a segment
    // before the walker re-paths around the obstacle.
    stuckTimeout = 15 * time.Second
    // stuckFastTimeout is the shorter window after the first stuck skip of a
    // trip: the server already refused the clicks on this segment, so the
    // recovery stays responsive while a slow position broadcast still lands.
    stuckFastTimeout = 4 * time.Second
    // moveStartWindow bounds the move start watchdog: a click whose movement
    // never started fast forwards the stuck recovery. The value covers the once
    // per second movement broadcast plus the network lag and stays under the
    // fast stuck window, so the never-started verdict lands first.
    moveStartWindow = 3 * time.Second
    // stuckProgressUnits is the net progress margin of the stuck window: the
    // waypoint must come closer by this many units to count as progress, so the
    // margin only filters the wobble.
    stuckProgressUnits = 50.0
    // maxRePaths bounds the full segment re-plans of one trip; the waypoint
    // skip of walkStuck does not consume it.
    maxRePaths = 3
    // minWalkClick is the floor length of the ground clicks: the server hands a
    // collapsed click to its pathfinder only when the original line was longer
    // than 30 units, so a shorter collapse is canceled with ActionFailed and
    // the click stays eligible for the rescue only above the floor. The floor
    // matches waypointPassDist.
    minWalkClick = 50.0
    // flowAimFloor is the lower bound of the continuous follower click span:
    // the run speed of a fresh character walks this far in two request periods.
    flowAimFloor = 300.0
    // flowAimPeriods is the number of request periods the click span covers:
    // two keep a full period of head room for the next re-aim.
    flowAimPeriods = 2.0
    // flowProbeMax bounds the click validation probes of one flow scan: the
    // scan runs at the click cadence, never per tick.
    flowProbeMax = 8
    // flowCornerLead is the distance to the current click target at which the
    // pre-click fires: about one third of a request period of walking, so the
    // pre-click still lands while the move runs.
    flowCornerLead = 80.0
    // flowPreClickGap bounds the pre-click cadence to one per half request
    // period, so a tight corridor cannot become a click storm.
    flowPreClickGap = walkRequestPeriod / 2
    // flowLayerTolerance bounds the vertical deviation of a skipped route point
    // from the click chord: a deeper dip is a priced water crossing or a ramp
    // the plan walks through on purpose.
    flowLayerTolerance = 100.0
    // frozenRepathLimit bounds the consecutive re-paths from the same cell
    // without a single cell of movement: a fresh plan that moved nothing proves
    // the trip cannot move the character either, and the callers own the
    // recovery.
    frozenRepathLimit = 1
    // refusalVariantsMax bounds the varied aim attempts one segment spends on
    // an online refusal: the refusal is target specific, so a shorter prefix
    // or a sideways offset often walks where the plain aim bounced.
    refusalVariantsMax = 4
    // refusalVariantStep is the sideways offset of the perpendicular refusal
    // variants: about twelve geodata cells off the route line, far enough to
    // bend the click raster off the refused flank.
    refusalVariantStep = 192.0
    // cursorEscapeStep is one claimed step of the cursor key escape: about one
    // second of run speed, the cadence the official client streams.
    cursorEscapeStep = 144.0
    // cursorEscapePeriod paces the claimed steps at one per second, the
    // ValidatePosition cadence of the official client.
    cursorEscapePeriod = time.Second
    // cursorEscapeFollowClaims bounds the patience of the follow probe: this
    // many claims without a server position change prove the server ignores the
    // stream.
    cursorEscapeFollowClaims = 5
    // cursorEscapeSettle bounds the wait for the position broadcasts after the
    // last claimed step.
    cursorEscapeSettle = 2 * time.Second
    // cursorEscapeAttemptsMax bounds the cursor key escape attempts of one town
    // trip: a pocket that keeps refusing the clicks is a wider problem.
    cursorEscapeAttemptsMax = 3
    // cursorEscapeFollowStep is the drift from the escape origin that proves
    // the server follows the claims and re-arms the follow patience.
    cursorEscapeFollowStep = 64.0
    // cursorEscapeRouteMax caps the planned route length one escape walks, so
    // the escape stays a pocket recovery; the planless aim clamps to the same
    // radius.
    cursorEscapeRouteMax = 2500.0
    // extendMarchStep is the stride of the forward route march of
    // extendShortClickCandidates: one geodata cell.
    extendMarchStep = 16.0
    // extendCandidateMax bounds the forward route samples the short click
    // extension tries: enough to step past a single route cell that walls the
    // chord.
    extendCandidateMax = 5
    // clickShortenFloor bounds the halving of a refused walk click: below it
    // the refusal is local (the character stands boxed) and the follower hops
    // or re-paths instead of crawling micro segments.
    clickShortenFloor = 100.0
    // hopCoincideDist is the distance under which a walked-past waypoint counts
    // as stood on: the escape hop targets the nearest plan bend between it and
    // the arrival radius.
    hopCoincideDist = 15.0
)
