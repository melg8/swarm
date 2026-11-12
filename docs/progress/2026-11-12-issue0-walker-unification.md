# P07: one waypoint walker and the tidy hunt Loop (status: in progress)

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

Slice 1 (the walk parameters in one file) in progress.

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
