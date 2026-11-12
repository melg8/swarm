# The launcher split: one file per operator mode (status: in progress)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the loop, town, walker and state rounds).

## Goal

`cmd/swarm/main.go` (2008 lines, 57 top-level declarations) is the
second largest hand-written file of the repository after the state
split. Done means:

- `main.go` keeps the operator entry point (`main`) and nothing else;
- the flag layer (`config`, `parseFlags`, `registerProxyFlags`, the
  defaults) moves to `flags.go`;
- the session lifecycle (the connect helpers, `runBot`,
  `runBotForever`, the journal wiring, the memory watch, the bot
  engine) moves to `session.go`, its journal query modes to
  `session_cli.go`;
- the fleet launch, the client proxy mode, the four botless UI modes,
  the pathfind/navmesh modes, the web server and the acceptance modes
  each get their own file (`fleet.go`, `proxy.go`, `testui.go`,
  `pathfind_mode.go`, `web.go`, `acceptance_mode.go`);
- byte preserving move, no behavior change: the same package, the
  same declarations, the same flag names and defaults.

## Constraints

- `cmd/swarm` only; the packages it imports are untouched.
- The existing tests (`main_test.go`, `main_helpers_test.go`,
  `main_extra_test.go`, `launch_config_test.go`) are the safety net
  and stay green unchanged.
- `task fmt` (gofmt-spaces) after the move, then the full gate.

## Context

Measured 2026-11-12 before the split:

- 57 declarations: the `config` struct (120 lines), `parseFlags`
  (241), `runBot` (155), `runBotForever` (63), `main` (157), the fleet
  launcher (102), the proxy mode (74), the five botless UI modes
  (~220), the pathfind/navmesh block (~300), the web and acceptance
  blocks (~190);
- the declaration order already groups the modes, so the split follows
  it rather than inventing a layout;
- every mode is reached only from `main` through one flag of `config`,
  so the package level move needs no call site change.

## Progress

### 2026-11-12 - the plan
The declaration inventory and the mode grouping above; the split
script (the AST range lister plus a verbatim copy, scratch) is next.

## Status

Inventory done, the move is next.

### 2026-11-12 - the move
The split script (the AST range lister plus a verbatim copy, scratch,
not committed) produced the ten files:

| File | Lines | Content |
| --- | --- | --- |
| `main.go` | 176 | the operator entry point: the mode switch |
| `flags.go` | 437 | the `config` struct, `parseFlags`, `registerProxyFlags`, the defaults and the geodata/navmesh candidate lists |
| `session.go` | 455 | the dial helpers and the dialer, `runBot`, `runBotForever`, the journal wiring, the memory watch, `newBotEngine`, the hunt event mirror |
| `session_cli.go` | 75 | the journal report/query/anomaly modes |
| `fleet.go` | 161 | `runFleet` and the account ladder |
| `proxy.go` | 90 | the client proxy mode |
| `testui.go` | 169 | the five botless UI modes (fight, path v1-v3, fight UI v1) |
| `pathfind_mode.go` | 324 | the pathfind test, the `navmeshShowFlag`, the navmesh viewer and its tile helpers |
| `web.go` | 71 | the web interface start and shutdown |
| `acceptance_mode.go` | 202 | the acceptance and hunt audit modes |

A move, not a rewrite: no declaration renamed, no flag touched, no
default changed. The import block of each file is derived from the
identifiers its code actually selects (a small lexer drops comments
and string literals first - a usage string that mentions `log.`
must not drag the package in).

Content check: the sorted non-blank line multiset of
`HEAD:cmd/swarm/main.go` against the ten files reports 0 lost lines;
the 125 extra lines are exactly the nine added SPDX headers plus the
nine `package main` / `import (...)` blocks and the per-file import
specs.

Verified: `go build ./...`, `go vet ./cmd/swarm/...`,
`golangci-lint run ./cmd/swarm/...` 0 issues,
`go test ./cmd/swarm/... -count=1` ok in 0.1 s.

Next: `task verify`, the live launch, then the fragment archive.
