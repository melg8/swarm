# The continuous waypoint flow round (issue0)

Date: 2026-11-12. Scope: the two ground click followers of the hunt
loop (`followWaypoints` of the town/trip walker and
`followUserWaypoints` of the manual `-move`/web UI walk), the corner
pre-click, the manual walk budget. Issue0 (owner report, no board
issue yet), landed on `main`: the flow in `bfe55ca7`, the route
budget in the commit carrying this fragment.

## Problem statement

A bot ordered onto a long multi-waypoint route (the navmesh viewer
route `45475,48368,-2913` -> `25500,51095,-3408`, approach 200,
smooth) walks in micro-freezes: the character stops at the dense
plan points instead of running through them. A cross-map manual
order additionally stopped after 90 s far from the target.

## Root cause analysis

- Each click targeted only the waypoint under the cursor, and the
  cursor advanced when the character arrived within
  `waypointPassDist` (50). A smoothed plan carries points tens to a
  few hundred units apart, so the server reached every click target
  and stopped the creature: the arrival broadcast is a zero distance
  `MoveToLocation`, which clears the movement state
  (`state.ApplyMovement` -> `clearCharMovement`) and stops the
  client animation. The arrival stop of one waypoint was the freeze.
- The server itself never stops a running creature when a new
  destination arrives: `MoveToLocation.runImpl` replaces the AI move
  intention (`player.getAI().setIntentionMoveTo(...)`), and the
  ceiling is 9900 units per request (`dx*dx+dy*dy > 98010000`), far
  above the follower's 1000 unit clip. Only a reached destination
  stops the creature, so a follower that re-aims before the arrival
  keeps the move running.
- The manual follower stood down while `SelfWalking()` was true, so
  it never re-aimed a running move at all; its arrival loop consumed
  every waypoint within `waypointArriveDist` (150).
- The manual walk deadline was the flat `userMoveTimeout` (90 s). The
  owner route is 22,346 units, ~179 s at the level 16 run speed 125,
  so the deadline aborted the walk halfway; the character stood in
  the wilds, was killed and revived in the village.
- A live probe (build `bfe55ca7`, bot `test1`, level 16) walked
  9,170 units in ~73 s at ~125 u/s with two stands over the 2 s
  sample window (2.2 s and 2.8 s). The probe samples the tracker
  positions, which advance on the server movement broadcast: a
  playable creature broadcasts `MoveToLocation` at most once per
  second (`Creature.broadcastMoveToLocation`, the 10 game tick
  gate), so a missed slot reads as a short stand - no click caused
  it.

## Reproduction

Start the stack, run one bot without `-hunt` (`swarm-flow-probe2.exe
-login 127.0.0.3:2106 -web 127.0.0.1:8080`), post the move command
through `POST /api/bots/{id}/commands` and sample
`GET /api/bots/{id}/state` every 250 ms. Before the fix the
character stood at nearly every dense plan point; after the fix it
runs through them and only the final arrival stops it.

## Fix

- `internal/swarm/hunt/flow.go`: `flowAim` (the run speed times two
  request periods, floored at 300, capped at `maxMoveDistance`),
  `flowOnRoute` (every skipped waypoint projects inside the click
  chord within `waypointCorridor`, anchored height within
  `flowLayerTolerance`), `flowAdvance` (farthest validated target
  first, bounded probes) and `flowPreClick` (a re-aim within
  `flowCornerLead` of the current town waypoint while the character
  walks, paced at `flowPreClickGap`; `flowScanAt` paces the failed
  scans too).
- `followWaypoints` runs the pre-click before the period gate and
  the flow advance before the click; `followUserWaypoints` re-aims
  every request period while the move runs; `segmentAdvanceClear`
  became a wrapper over `routeAdvanceClear` so both followers share
  one transport check. The dead-cell skip verdict
  (`skipMoveFresh`/`noteSkipStand`) short-circuits the scan and
  `planUserWalk` resets it.
- The manual walk budget follows the plan: `userMoveBudget` is the
  90 s base plus twice the estimated travel (`manualTravel`: plan
  length over the measured run speed, the 120 default before
  UserInfo), capped at `userMoveTimeoutMax` (30 min); the absolute
  deadline `userDeadline` is set in `planUserWalk` and reset on a new
  command and in `resumeAuto`, so direct server routed walks keep the
  base window.
- `docs/hunting.md` documents the far aim, the corridor, the
  pre-click and the budget.

## Verification

- `internal/swarm/hunt/flow_test.go`: ten pins (far selection, the
  farthest validated fallback, the switchback, the final point, the
  dead cell, the beyond-aim skip, the manual re-aim, the mid-walk
  pre-click, the standing hold, the failed-scan pace).
- `user_test.go`: `TestUserMoveBudgetFollowsThePlannedRoute` (the
  budget and `manualTravel`), `TestUserPlanWalksPastTheBaseWindow`
  (the planned deadline overrides the base window and the expired
  deadline resumes the hunt).
- Full `go test ./internal/swarm/hunt/` suite green,
  `go build ./...`, `task lint:new`, `task fmt:check` clean.
- The live acceptance: the bot repositioned to `45475,48368,-2913`
  and walked the 63 waypoint / 23,100 unit repro route to exactly
  `25500,51095` in 161.1 s - 143.4 u/s average against the 143.75
  snapshot run speed, so the character never stopped - and finished
  with "manual walk arrived (path done)", no timeout, no death. The
  sample gaps of 2.2 s equal two intervals of the once per second
  movement broadcast. Numbers in the progress fragment
  `archive/2026-11-12-issue0-continuous-waypoint-flow.md`.

## Follow ups

- `walkBlindWaypoints` (the short blind engage reposition) and the
  quest trip `followWaypoint` keep their own click loops; both walk
  short segments and were left unchanged.
- The two short stands of the live probe match the once per second
  movement broadcast gate; a tighter verdict needs a server side
  movement log rather than the client positions.
