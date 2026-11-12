# The continuous waypoint flow of the ground walkers (status: in progress)

Started 2026-11-12, branch main, issue0 (owner report: a bot on a
long multi-waypoint route - the navmesh viewer route
`45475,48368,-2913` -> `25500,51095,-3408`, approach 200, smooth -
walks in micro-freezes instead of running through the plan).

## Goal

Both ground click followers (the town/trip walker and the manual walk
of `-move`/web UI orders) walk a long route continuously: the
character never stops on the intermediate waypoints of the plan, the
route shape and the final point survive, and the click rate stays the
existing one (no click storm). Done means:

- one click targets the farthest route waypoint the server click
  transport delivers inside a speed derived aim span, so the next
  request re-aims far before the character reaches the target;
- the server keeps the move running (it replaces the destination of a
  running move without stopping the character - verified in the
  Mobius sources), so the only stop is the final arrival;
- the route fidelity guard keeps switchbacks, priced water dips and
  ramps (the straight click may only shortcut inside the drawn
  corridor);
- a dead click transport still hands the walk to the stuck recovery
  (the skip bookkeeping short-circuits the flow scan);
- the CPU cost stays at the click cadence: the flow scan runs at most
  once per request period, and the corner pre-click scans at most once
  per half period (`flowScanAt`), never per tick;
- unit tests pin the far selection, the fallback, the switchback, the
  final point, the dead cell, the manual re-aim and the pre-click
  gates.

## Context

- The Mobius `MoveToLocation` handler replaces the destination of a
  running creature without a stop; the arrival broadcast is the zero
  distance `MoveToLocation`, and the server has no movement flood
  protector (`FloodProtector.ini` gates player actions at 1 s, not
  moves). The old follower clicked the current waypoint only, so every
  dense plan point stopped the character (the owner repro route walks
  14+ smoothed waypoints).
- The manual follower additionally stood down while `SelfWalking()`
  was true, so it never re-aimed a running move at all.
- The aim span is `SelfRunSpeed() * 2 request periods`: two periods of
  running keep one full period of head room for the next click to
  re-aim. The pre UserInfo default is 120 run speed, floored at 300
  and capped at `maxMoveDistance` (1000).
- The click target must pass the same gate as the plain click
  (`routeAdvanceClear`: frame anchored z, the 1000 unit clip, delivery
  within `waypointPassDist`), plus `flowOnRoute`: every skipped
  waypoint projects inside the click chord within `waypointCorridor`
  (100) and its anchored height stays within `flowLayerTolerance`
  (100) of the chord. The server validation alone allows a chord
  across a switchback or a priced water dip the plan walks around.
- The corner pre-click fires within `flowCornerLead` (80) of the
  current town waypoint while `SelfWalking()`, paced at
  `flowPreClickGap` (1 s, half a request period): the arrival radius of
  an intermediate town waypoint is `waypointPassDist` (50), so the
  50-80 ring is the window where the character still moves. The manual
  walker has no pre-click: its arrival loop consumes every waypoint
  within `waypointArriveDist` (150), and a second pre-click layer was
  implemented and removed as unreachable - the manual continuity rides
  the periodic far re-aim instead.
- The pre-click records its scan time in `flowScanAt`: a scan that
  finds no far target still spends the slot, so a route point no far
  target serves cannot turn the 30 unit ring into a per tick probe
  loop.
- The flow shares `skipMoveFresh`/`noteSkipStand` with the skip
  ladder: a jump from a cell the previous jump left frozen is denied
  before the validation probes, so a dead transport still reaches the
  stuck recovery. `planUserWalk` resets the bookkeeping so a manual
  plan never inherits a dead-cell verdict.

## Progress

### 2026-11-12 - the flow scan, the wiring and the pins

Added `internal/swarm/hunt/flow.go` (`flowAim`, `flowOnRoute`,
`flowAdvance`, `flowPreClick`) and the constants in `town.go`. Wired
`followWaypoints` (corner pre-click before the period gate, then the
flow advance before the click) and `followUserWaypoints` (the
`SelfWalking` early return removed, the click factored into
`clickUserWaypoint`, the flow advance before the click, the skip
reset in `planUserWalk`). `segmentAdvanceClear` became a wrapper over
`routeAdvanceClear` so both followers validate through one transport.
Added the `flowScanAt` pre-click scan pace and the ten pins of
`flow_test.go` (far selection, farthest validated fallback,
switchback, final point, dead cell, beyond-aim skip, manual re-aim,
mid-walk pre-click, standing hold, failed-scan pace). Updated the two
click frame pins (the far valid click is now the anchored one) and
the waypoint follower section of `docs/hunting.md`. The full hunt
package suite passes, `go build ./...`, `task lint:new` and
`task fmt:check` are clean.

### 2026-11-12 - the live probe and the route sized manual budget

Ran the committed build `bfe55ca7` against the live stack: bot `test1`
(level 16, run speed 125) at `25147,51600,-3493` ordered to the repro
point `45475,48368,-2913`. The search planned 58 waypoints / 22,346
units, and the follower walked them continuously: 9,170 units in
~73 s at ~125 u/s with only two standstills over the 2 s sample
window (2.2 s and 2.8 s), so the far re-aim holds the move running
between the clicks.

The same probe exposed a separate defect: `userMoveTimeout` (90 s)
aborted the walk in the middle of the route, and the character stood
and then resumed the hunt far from the target. A cross-map manual
order is valid, so the budget now follows the plan:
`userMoveBudget` is the base window plus twice the estimated travel
(`manualTravel`: the plan length over the measured run speed, the 120
default before UserInfo), capped at `userMoveTimeoutMax` (30 min).
The deadline is absolute from `planUserWalk` (`userDeadline`), reset
on a new command and in `resumeAuto`, so the direct server routed
walks (no plan) keep the base window. `pathfind.Result.Duration` is
the search time, not the travel time - the estimate reads
`Result.Length`. Pinned by `TestUserMoveBudgetFollowsThePlannedRoute`
(including `manualTravel`) and `TestUserPlanWalksPastTheBaseWindow`.
The owner route budgets 90 s + 2 × 179 s ≈ 7.5 min.

### 2026-11-12 - the end-to-end live acceptance

Re-ran the probe with the route sized budget from the working tree
(the probe build reports the dirty tree of `bfe55ca7`): the bot
repositioned to `45475,48368,-2913` (1,763 units), then walked the
repro direction. The search planned 63 waypoints / 23,100 units and
the follower arrived at exactly `25500,51095,-3297` (remaining 0) in
161.1 s, "manual walk arrived (path done)", no timeout and no death.
The average speed over the whole route is 143.4 u/s against the
snapshot run speed 143.75, so the character never stopped; the ten
2.2 s position gaps in the sample are the once per second movement
broadcast of the server (two intervals without an update), not
movement stops.

Next: decide whether `walkBlindWaypoints` (the short blind engage
reposition) and the quest trip `followWaypoint` need the same flow.

## Status

complete: the flow landed in `bfe55ca7`, the route sized budget, the
devlog round and this fragment land in the follow-up commit. The live
acceptance walked the full owner route continuously and arrived.

## Follow up

`walkBlindWaypoints` and the quest trip `followWaypoint` keep their
own click loops (short segments, left unchanged); the two followers
this round fixed are the town/trip walker and the manual walk.
