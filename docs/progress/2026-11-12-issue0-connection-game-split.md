# The game session split: core, actions, movement, login (status: in progress)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the loop, town, walker, state, launcher and shop
planner rounds).

## Goal

`internal/swarm/connection/game.go` (1535 lines) is the last hand
written file above the 1500 line god-file bar of AGENTS.md besides the
proxy session, which has active work around it. Done means:

- `game.go` keeps the protocol id tables, the `GameClient` and the
  connection core (the handshake, the send and read paths, the run
  loop, the close and drain);
- the outbound action senders move to `game_actions.go`;
- the client position validation and the two walk request methods move
  to `game_move.go`;
- the login and the character selection dance move to `game_login.go`;
- byte preserving move, no behavior change.

## Constraints

- `internal/swarm/connection` only; the hunt, proxy and CLI call sites
  are untouched.
- The fake server tests (`game_test.go`, `game_*_test.go`) are the
  safety net and stay green unchanged.
- `task fmt` (gofmt-spaces) after the move, then the full gate.

## Context

Measured 2026-11-12 before the split:

- 74 top-level declarations: the protocol and packet id tables, the
  `GameClient` with its nine setters and send helpers, 15 outbound
  action senders, the position validation block, the login and
  character creation block and the run loop;
- the adjacent `game_dispatch.go` and `game_apply.go` already hold the
  inbound table and the apply handlers, so the split extends the
  existing per-concern naming instead of inventing a new layout;
- the packet id tables stay in `game.go`: they are read by the
  dispatch, the apply handlers and the run loop at once, so no single
  consumer owns them.

## Progress

### 2026-11-12 - the plan
The declaration inventory and the four-file grouping above; the split
script (the AST range lister plus a verbatim copy, scratch) is next.

## Status

Inventory done, the move is next.

### 2026-11-12 - the move
The split script (the AST range lister plus a verbatim copy, scratch,
not committed) produced the four files:

| File | Lines | Content |
| --- | --- | --- |
| `game.go` | 678 | the protocol and packet id tables, `GameSessionParams`, `CharacterParams`, `GameClient`, the setters, `SendRaw`, `sendPacket`/`readPacket`, the run loop and the close/drain path |
| `game_actions.go` | 343 | the outbound action senders (attack, pickup, drop, use, sell, buy, say, sit, click, pull, target, skills, village restart) |
| `game_move.go` | 182 | `positionValidation`, `validatePosition`, `WalkTo`, `CursorKeyWalkTo`, `ClaimValidatePosition` |
| `game_login.go` | 377 | `Authenticate`, `EnsureCharacter`, the character creation dance, `EnterWorld` and the selection wait |

A move, not a rewrite: no declaration renamed, no signature touched,
no packet id changed. The import block of each file is derived from
the identifiers its code actually selects, aliases included
(`fromgameserver`, `togameserver`, `packet`).

Content check: the sorted non-blank line multiset of
`HEAD:internal/swarm/connection/game.go` against the four files reports
0 lost lines; the 32 extra lines are exactly the three added SPDX
headers plus the three `package connection` / `import (...)` blocks and
the import specs.

Verified: `go build ./...`, `go vet ./internal/swarm/connection/...`,
`golangci-lint run ./internal/swarm/connection/...` 0 issues,
`go test ./internal/swarm/connection/... -count=1` ok in 10.5 s.

Next: `task verify`, the live run, then the fragment archive.
