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

### 2026-11-12 - hunt: group the loop state by concern
`Loop` is no longer one flat field block: the 250 concern fields moved
into fourteen embedded sub-structs declared in `loop_state.go` (819
lines, one block per concern): `combatState` 33 fields, `lootState` 4,
`restState` 3, `questState` 3, `travelState` 49, `townState` 11,
`shopState` 24, `learnState` 17, `delevelState` 12, `groundState` 12,
`safetyState` 15, `diagState` 13, `userState` 21, `gearState` 2,
`kiteState` 31. The blocks are embedded, so the names are promoted and
not a single call site of the 474 `Loop` methods changed. `loop.go`
dropped to 122 lines: the package doc, the composition, the constructor
and the setters. `NewLoop` lost the 228 line flat literal: it now sets
the six core fields and the nine non-zero starts (the five maps, the
equip manager, the kite defaults, the trip approach radius and the
`zoneDeathCap` -1), every other field keeps its documented zero value
(byte for byte the values the literal spelled out). Verified: `go
build ./...`, `go vet ./...`, `task fmt:check`, `go test
./internal/swarm/hunt/... -count=1` (97.6 s), `golangci-lint run
./internal/swarm/hunt/...` (0 issues).

### 2026-11-12 - hunt: split the tick context into steps
`tickContext` dropped its `//nolint:cyclop,funlen,gocognit` and became
a nine line chain of named steps: `publishTickState` (the defer: phase
banner, shopping widget, diagnostics heartbeat, stagnation watch),
`tickDeathOrSafety` (the death restart, the critical health logout, the
armed pile up run, then `tickPileUp`), `tickCommandPhase` (the user
command drain, the delevel and manual phases, the manual only idle
hold), `tickErrands` (the auto equipment, the first cell pick, the
deleveling, the shopping trips) and `tickHunt` (the inventory cleanup,
the zone switch, the engage or loot step). The gate order and the
return semantics stay exactly as before: every helper reports `bool`
"the tick was consumed" and the only shared input is the single
`time.Now()` of the pile up step. Several 10-20 line comment blocks
that retold the moved code were compressed to their load-bearing fact.
Verified: `go build ./...`, `task fmt` + `task fmt:check`,
`go test ./internal/swarm/hunt/... -count=1` (97.6 s),
`golangci-lint run ./internal/swarm/hunt/...` (0 issues).

## Status

Commits 1-3 done. Next: the `engage` split (commit 4), full verify and
E2E.
