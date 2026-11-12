# The town trip split: files by responsibility (status: in progress)

Started 2026-11-12, branch main, issue0 (owner instruction: continue the
refactor after the hunt loop split).

## Goal

`internal/swarm/hunt/town.go` (4338 lines) stops being the package's
second god file. Done means:

- the file keeps one responsibility: the town trip lifecycle (the
  inventory trigger, the trip start and end, the abort/backoff, the
  return segment) and the package doc;
- the merchant tables and stop geometry move to `town_merchants.go`,
  the geodata transport seam to `navigator.go`, the segment planning
  to `town_segment.go`, the waypoint follower and click machinery to
  `town_walk.go`, the cursor key escape to `town_cursor_escape.go`,
  the stuck and refusal recovery to `town_stuck.go`, the selling flow
  to `town_sell.go` and the timing/threshold constants to
  `town_params.go`;
- the rest guard `standUpGuarded` moves out of the town file into
  `loop_safety.go`, where the rest layer lives;
- byte preserving move, no behavior change: the same package, the same
  declarations, the same comments.

## Context

Measured 2026-11-12 before the split:

- `town.go` 4338 lines is the second largest hand written file of the
  package after the generated `cells_elven.go` (11836);
- it mixes ten concerns: the trip lifecycle, the merchant tables, the
  navigator adapter, the segment planner, the waypoint follower, the
  cursor key escape, the stuck/refusal recovery and the selling flow;
- the walk constants (`walkRequestPeriod`, `waypointArriveDist`,
  `waypointPassDist`, `maxMoveDistance`) live in it while the manual
  walker in `user.go` and the quest walker in `quest_trip.go` consume
  them - the P07 prompt records this as walk-constant debt;
- the same file split recipe as the loop split applies: the moved
  ranges keep their comments and formatting, `task fmt:check` and the
  full hunt suite are the gate.

## Progress

### 2026-11-12 - branch and plan
Landing the finished loop split on `main` (fast-forward `6683cb64`,
pushed), then the town split. Next: the split script and the first
commit.

### 2026-11-12 - the split itself (commit 1)
The one-shot split helper (`decls` AST lister + a range assigning
script, both in the scratch directory, not committed) moved 4338 lines
into nine files: `town.go` 730, `town_params.go` 331,
`town_merchants.go` 230, `navigator.go` 123, `town_segment.go` 439,
`town_walk.go` 953, `town_cursor_escape.go` 478, `town_stuck.go` 704,
`town_sell.go` 437.

Byte preserving by construction: every moved range was checked for
overlap and the sorted line multiset of the original file equals the
one of the nine files (the only additions are the per-file SPDX header,
the rebuilt import block and the blank separators). The declarations
keep their names, comments and order inside a file.

Measured after: `go build ./...` clean, `go vet` clean,
`golangci-lint run ./internal/swarm/hunt/...` 0 issues, `go test
./internal/swarm/hunt/... -count=1` ok in 97.7 s, `task fmt:check`
exit 0.

Next: the `standUpGuarded` move into `loop_safety.go` (commit 2).

### 2026-11-12 - the stand guard moves to the rest layer (commit 2)
`standUpGuarded` left the town file for `loop_actions.go`, not the
planned `loop_safety.go`: the guard is the other half of the pending
transition gate implemented by `restToggleTo` (`restActionAt` /
`restActionSit`), so the two live in one file instead of the guard
sitting with the escape walks that merely call it. `town.go` is 684
lines now, `loop_actions.go` 462.

The same gate: `go build ./...`, `go vet`, `golangci-lint run
./internal/swarm/hunt/...` 0 issues, `go test ./internal/swarm/hunt/...
-count=1` ok in 97.6 s, `task fmt:check` exit 0.

## Status

Both commits are verified. Next: `task verify`, the live E2E, then
archive the fragment.
