# The state bot split: files by responsibility (status: complete)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the hunt loop, town and walker rounds).

## Goal

`internal/swarm/state/bot.go` (3565 lines) is the largest hand-written
file of the repository and the one AGENTS.md calls the failure mode by
name (the historical 2392 line `state.Bot`). Done means:

- `bot.go` keeps the package doc, the `Bot` aggregate struct and
  `NewBot`;
- the world model value types move to `bot_model.go`, the self reads to
  `bot_self.go`, the object reads to `bot_objects.go`, the session
  metadata to `bot_session.go`, the walk plan to `bot_walkplan.go`, the
  apply handlers to `bot_apply.go`, the event sink and the locked
  mutation helpers to `bot_events.go`, the info/name helpers to
  `bot_info.go` and the snapshot view types plus `Snapshot` to
  `snapshot.go`;
- byte preserving move, no behavior change: the same package, the same
  declarations, the same comments and the same lock boundaries.

## Constraints

- The tracker stays the single source of truth: no field renamed, no
  method signature touched, no lock scope changed.
- The whole `state` test suite (`bot_test.go`, `bot_accessors_test.go`,
  `snapshot_*_test.go`, `scans_blocked_test.go`, ...) is the safety
  net and stays green unchanged; the other packages that read the
  tracker (`hunt`, `webserver`, `connection`, `proxy`) are only
  recompiled.
- `task fmt` (gofmt-spaces) after the move, then the full gate.

## Context

Measured 2026-11-12 before the split:

- 185 top-level declarations in one file: the `Bot` struct (231 lines),
  `Snapshot` (148), the `Snapshot` view type (113), `CharacterState`
  (93) and ~150 small accessor and apply methods;
- the declaration order already groups the concerns, so the split
  follows it instead of inventing a new layout;
- the package has neighboring files per concern already
  (`scans.go`, `inventory.go`, `objects_store.go`, `combat.go`,
  `tracking.go`, `snapshot_live.go`), so the new names take the
  free ones and never shadow an existing file.

## Progress

### 2026-11-12 - the plan
The declaration inventory (`decls` AST lister, scratch) and the
grouping above; next the range assignment and the move.

## Status

Complete: the ten files land on `main` as `6d382a90`, `task
verify` green and the live run clean.

### 2026-11-12 - the move
The split script (the AST lister for the declaration ranges plus a
verbatim copy, scratch, not committed) produced the ten files:

| File | Lines | Content |
| --- | --- | --- |
| `bot.go` | 299 | the package doc, `Bot`, `NewBot` |
| `bot_model.go` | 412 | `Status`, `CharacterState`, the event and info value types, the combat constants |
| `bot_self.go` | 428 | the self reads (`SelfPosition`, `SelfEngaged`, the skills, the vitals) and the facing apply |
| `bot_objects.go` | 243 | the object reads over the store (`ObjectPosition`, `ObjectName`, the vitals) |
| `bot_session.go` | 377 | the session metadata and phase setters, the danger spots, the death regression, `ResetSession` |
| `bot_walkplan.go` | 245 | the published walk plan types, the publish and the equality helpers |
| `bot_apply.go` | 703 | the packet apply handlers (`ApplyMovement`, `ApplyAttackAt`, `ApplyNpcInfo`, ...) |
| `bot_events.go` | 231 | the event sink, the request stamps and the locked mutation helpers |
| `snapshot.go` | 593 | the snapshot view types and `Snapshot` |
| `bot_info.go` | 131 | `BotInfo`, the npc/item name resolution |

A move, not a rewrite: no declaration renamed, no signature touched,
no lock scope changed. The import block of each file is derived from
its own body (the code lines only: a comment that mentions `sort.`
does not drag the package in), so no file carries a dead import.

Content check: the sorted non-blank line multiset of `HEAD:bot.go`
against the ten files reports 0 lost lines; the 67 extra lines are
exactly the nine added SPDX headers plus the nine `package state` /
`import (...)` blocks and the 13 per-file import specs.

Verified: `go build ./...`, `go vet ./internal/swarm/state/...`,
`golangci-lint run ./internal/swarm/state/...` 0 issues,
`go test ./internal/swarm/state/... -count=1` ok in 1.1 s,
`task fmt:check` exit 0.

Next: `task verify`, the live run, then the fragment archive.

### 2026-11-12 - the gate and the live run
`task verify` exit 0 (build, vet, the full tree lint, the whole test
suite, `fmt:check`). The live run on the Windows stack: the binary
carries the `-ldflags` identity of `6d382a90`, `-login
127.0.0.3:2106 -hunt -web ""`, 65 s in world, then Ctrl+Break
through the console process group helper - the tracker fed the login
and the spawn burst, the socket closed with `Leaving the game world
with test1` / `Bot finished`, exit code 0, no panic.
