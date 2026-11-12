# The hunt loop split: files by responsibility, state by concern (status: in progress)

Started 2026-11-12, branch refactor/loop-split, issue0 (owner
instruction: refactor loop.go).

## Goal

`internal/swarm/hunt/loop.go` stops being a 2663 line file that holds the
whole hunt state machine. Done means:

- the file carries one responsibility (the `Loop` type and its
  constructor), the interface, the timing constants, the phase dispatch,
  the engage decision tree, the death recovery and the zone predicates
  live in their own files;
- the `Loop` struct stops being one flat 257 field block: the fields are
  grouped into embedded per-concern sub-structs (`combat`, `travel`,
  `town`, `shop`, `learn`, `delevel`, `quest`, `cells`, `safety`,
  `diag`, `user`, `gear`, `kiteState`), so a reader sees the concerns
  instead of a 744 line field list;
- the two suppressed decision trees (`tickContext`,
  `engage`) are split into named steps and lose their
  `//nolint:cyclop,funlen,gocognit` suppressions;
- no behavior change: the hunt suite (33k test lines), the full
  `go test ./...`, the lint gate and the live E2E stay green.

Constraints: no field renames at the call sites (embedding promotes the
names, so the 474 `Loop` methods keep compiling unchanged); no logic
touch; the move is mechanical and byte preserving where it can be.

## Context

Measured 2026-11-12 before the first commit:

- `loop.go` 2663 lines: GameAPI 100, timing constants 295, phase 30,
  `Loop` struct 744 (257 fields), `NewLoop` 228, setters and zone
  helpers 240, `Run`/`tickContext` 230, `recoverFromDeath` 60,
  `engage` 571, `losingFight` 47;
- `engage` is the only `//nolint:...maintidx` of the package and the
  lowest maintainability index of the tree (MI 11 per `.golangci.yml`);
- the struct mixes thirteen concerns; the kite block alone is 60 fields
  with 120 lines of comments;
- the package already split the methods by concern
  (`loop_actions.go`, `loop_movement.go`, `loop_safety.go`,
  `loop_avoid.go`, `loop_los.go`, `loop_diagnostics.go`), so the file
  layout of the *methods* is fine - only the state declaration and the
  constructor lag behind;
- the P07 prompt of `docs/quality_review_and_agent_prompts.md` asks for
  the same field regrouping (its line numbers are stale) and for one
  shared waypoint walker - the walker unification stays out of this
  round, it touches town/user/delevel behavior and deserves its own
  task.

## Progress

### 2026-11-12 - branch and plan
Created `refactor/loop-split`, recorded the plan above. Next: the file
split (commit 1).

### 2026-11-12 - hunt: split the loop file by responsibility
`loop.go` went from 2663 to 1051 lines: the `GameAPI` interface moved to
`game_api.go` (114), the timing constants to `loop_params.go` (306), the
phase type with `Run`/`tick`/`tickContext`/`handleTownTrip`/`fightBusy`
to `loop_phase.go` (328), `journalKill`/`resetFightClock`/
`recoverFromDeath` to `loop_death.go` (107), `engage`/`losingFight`/
`maxTargetLevel` to `loop_engage.go` (663), the zone and ground
predicates to `loop_zone.go` (153). `loop.go` keeps the package doc, the
`Loop` type, `NewLoop` and the setters. Byte preserving move, no logic
change: `go build ./...`, `go vet`, `go test ./internal/swarm/hunt/...
-count=1` (97.5 s) and `golangci-lint run ./internal/swarm/hunt/...`
(0 issues) green.

## Status

Commit 1 done. Next: field grouping into per-concern embedded
sub-structs (commit 2), then the `tickContext` and `engage` splits.
