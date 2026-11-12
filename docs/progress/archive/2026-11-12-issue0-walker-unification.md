# P07: one waypoint walker and the tidy hunt Loop (status: complete)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the loop and town splits).

## Goal

The deferred P07 prompt (`docs/quality_review_and_agent_prompts.md`,
"One waypoint walker and a tidy hunt Loop"), minus what already
landed: the `Loop` field regrouping shipped with the loop split
(14 embedded sub-structs, `loop_state.go`). Done means:

- the walk machinery parameters live in one file with doc comments,
  not spread over the town, delevel and diagnostics files;
- one waypoint follower core serves the town trip, the manual walk,
  the quest trip, the zone return and the delevel walks, with the
  per-caller pacing and arrival radii preserved;
- `lastHit` stops being a five-purpose name, and the two skip
  conventions (`l.skipped` deadlines vs `l.targetSkip` event times)
  share one meaning;
- no behavior change: the hunt test suite and the fake-server flow
  test stay green, and the live walk of the town trip still reaches
  the merchant.

## Constraints

- Inside `internal/swarm/hunt` plus its tests; the doctests and the
  node harnesses are untouched.
- One atomic commit per slice, each verified with `go build ./...`,
  `go vet`, `golangci-lint run ./internal/swarm/hunt/...` and the
  package tests; `task verify` before the push.
- Byte preserving moves where the slice is a move; a behavior change
  is allowed only where a walker difference is a bug, and it carries
  its own commit plus a test.

## Context

Measured 2026-11-12 after the town split:

- four waypoint followers exist: `walkTownWaypoints`/`followWaypoints`
  (`town_walk.go`), `followUserWaypoints` (`user.go`),
  `followPlannedSegment`/`followWaypoint` (`quest_trip.go`) and
  `walkZoneSegment` (`loop_movement.go`); the stuck detection exists
  only in the town copy (`town_stuck.go`);
- the walk parameters split over `town_params.go` (the pacing, the
  waypoint radii, the stuck windows, the cursor escape),
  `delevel.go` (`returnWalkSegment`) and `loop_diagnostics.go`;
- `l.skipped` stores deadlines while `l.targetSkip` stores event
  times, and `lastHit` names the last action of several kinds.

## Progress

### 2026-11-12 - the plan
Landing the town split (`9892c250`, `68cbe495`, `80921f0e`,
`701de1cc`), then starting P07 with the parameter hoist.

## Status

Complete: three slices on `main` (the walk parameters, the shared
click/cursor core, the action pacing and skip names), `task verify`
green and the live run clean. The follower consolidation carries to the
next round with the reasons recorded above.

### 2026-11-12 - slice 1: the walk parameters in one file
`walk_params.go` now holds the 33 walk machinery constants: the click
pacing and the refusal answer window, the waypoint arrival/pass
radii and the corridor, the move request cap (`maxMoveDistance` and
`returnWalkSegment`, the latter moved out of `delevel.go`), the stuck
and move start windows, the re-path budget, the click floor, the
continuous flow aim, the frozen re-path and refusal variants, the
cursor key escape and the short click extension. `town_params.go`
keeps the 19 trip and merchant parameters (the trigger percentages,
the sell batch, the trip cooldown and timeout, the merchant and npc
geometry), so the two files now read by responsibility.

A move, not a rewrite: an AST tool (scratch, not committed) copied
every spec with its comment verbatim; the counts close (51 = 19
kept + 32 moved, plus `returnWalkSegment` from `delevel.go` = 33).

Verified: `go build ./...`, `go vet`, `golangci-lint run
./internal/swarm/hunt/...` 0 issues, `go test ./internal/swarm/hunt/...
-count=1` ok in 97.5 s.

Next: the follower core (slice 2).

### 2026-11-12 - slice 2: the shared click and cursor core
`walk_core.go` carries the two duplicated primitives of the walkers:

- `walkClickSegment` clips a ground click onto one server move request
  (the point on the line at the limit, the z interpolated along it).
  It replaces seven inline copies: the town waypoint click, the manual
  walk click, the far target walk, the blind reposition walk, the
  corpse approach, the lure approach, the respawn drift walk, the zone
  segment click and the advance line gate of `routeAdvanceClear`.
  The int-only callers pass their own z as the target height, which
  interpolates back to exactly the standing z they sent before;
- `advanceWalkCursor` walks the plan cursor over the reached and the
  passed-along-route waypoints behind the walker's optional line
  gate. The town follower (`advanceWaypoints`, gated by the click
  validation) and the manual follower (ungated) now share it; each
  resets its own click timestamp when the cursor moved.

`walk_core_test.go` pins both: the clip ring, the kept line, the
reached/passed advance, the gate, the plan end and the tight final
radius. The control flow of each walker is untouched - the recovery
ladders (the town stuck window, the manual refusal variants) stay
where they are, which is the deferred half of the unification.

Verified: build, vet, lint 0 issues, hunt tests ok in 97.5 s.

Next: `lastHit` and the skip conventions (slice 3).

### 2026-11-12 - slice 3: the pacing and skip names
`lastHit` is `lastActionAt`: it never meant a hit. Every reader is an
action gate (the pick, the engage retry, the far target walk, the
corpse approach, the trip start, the delevel guard pick) and every
writer stamps the moment the loop sent its action, so the name now
says what the field is - the package wide action pacing.

The two skip lists keep one convention, which the probe confirmed:
both `targetSkip` (the mobs held out of the target search) and the
old `skipped` (the drops on the pickup retry) store an expiry
deadline and answer `now.Before(until)`. The second list is now
`itemSkip`, so the pair reads as what it is: the mob list of
`combatState` and the drop list of `lootState`, same deadline rule.
The struct comments state both facts.

A rename only: 272 `lastActionAt` references (the tests included),
no logic touched.

Verified: build, vet, lint 0 issues, hunt tests ok in 97.5 s,
`fmt:check` exit 0.

## Status

Slices 1-3 land the parameter hoist, the shared click/cursor core
and the naming clean-up. The remaining P07 item - folding the four
followers into one walker type - is a control flow rewrite (the quest
walker is a blocking sleep loop with its own potion cadence, the town
and manual walkers are tick driven with different recovery
ladders); it needs its own round with a live acceptance of the town
trip, so the fragment closes here.

### 2026-11-12 - the gate and the live run
`task verify` exit 0 (build, vet, the full tree lint, the whole suite,
`fmt:check`). The live run on the Windows stack: the binary carries
the `-ldflags` identity of `a08314f0`, `-login 127.0.0.3:2106 -hunt
-web ""`, 65 s in world, then Ctrl+Break through the console process
group helper - the level 17 character farmed (the kite ledger line,
the Power Strike finishing blow, the kills and loots), the socket
closed with `Leaving the game world with test1` / `Bot finished`,
exit code 0.
