# The proxy game connection split: files by responsibility (status: complete)

Started 2026-11-13, branch main, issue0 (owner instruction: continue
the refactor with `internal/swarm/proxy/game.go`).

## Goal

`internal/swarm/proxy/game.go` is the last hand-written file above the
1500 line bar of the repository (1532 lines, 62 declarations). Done
means:

- `game.go` keeps the protocol constants, the `gameConn` and `gameState`
  types, the connection lifecycle (`handleGameConn`, `run`, `handshake`,
  `sendRawKeyPacket`), the session/state accessors and `shutdown`;
- the pre-world client handlers move to `game_client.go` (dispatch,
  auth login, char select, char create, enter world, the auth login
  reader);
- the outbound sender and the live relay move to `game_relay.go`
  (`transitToServer`, `runSender`, `runRelay`, `streamSession`,
  `deliverLiveUpdate`, `replayHistory`, `relayToClient`,
  `sendToClient`);
- the client view guards move to `game_view.go` (local NetPing answer,
  position divergence, correction, move veto);
- the bot switch and relogin dance move to `game_switch.go`
  (`serveBotSwitch`, `parkAtCharSelect`, auto select, `serveRelogin`);
- byte preserving move, no behavior change: same package, same
  declarations, same comments, same locks.

## Constraints

- No declaration renamed, no signature touched, no lock scope changed.
- The 12 proxy test files (~3.7k lines) are the safety net and stay
  green unchanged; no live server needed for the split.
- `task fmt` (gofmt-spaces) after the move, then the gate.

## Context

Measured 2026-11-13 before the split:

- 1532 lines, of which ~416 comment lines (27.1%): the file also leads
  the comment debt list, but the sweep is a parallel fragment - this
  round only moves the text;
- the declaration order already groups the concerns, so the split
  follows it instead of inventing a new layout;
- the package has neighboring files per concern already (`handoff.go`,
  `chars.go`, `replay.go`, `recorder.go`, `transformer.go`), so the new
  names take free ones;
- `handoff.go` owns `clientOpLogout`, `clientOpNetPing` and
  `serverOpNetPing`; the position opcodes stay with the protocol
  constants in `game.go`.

## Progress

### 2026-11-13 - the declaration inventory and the plan
The full file read, the declaration ranges listed (struct 25-101,
constants 103-183, lifecycle 185-324, client handlers 326-560, session
accessors 562-614, transit 616-706, view 708-867, sender/relay
869-1071, switch 1073-1395, replay/client send 1397-1462, shutdown
1464-1484, auth reader 1486-1532); next the move.

### 2026-11-13 - the move
The verbatim range copy produced the five files:

| File | Lines | Content |
| --- | --- | --- |
| `game.go` | 420 | the protocol constants, `gameConn`, `gameState`, the lifecycle, the session and state accessors, `shutdown` |
| `game_client.go` | 298 | the dispatch, the auth login, char select, char create, enter world and the auth login reader |
| `game_relay.go` | 373 | the transit, the outbound sender, the live relay and the replay filters |
| `game_switch.go` | 309 | the WebUI switch, the park and the relogin dance |
| `game_view.go` | 172 | the local NetPing answer, the divergence, correction and veto guards |

A line multiset comparison against `HEAD` is identical (1354 non-blank
lines). `go build ./...`, `go vet` and the proxy tests are green,
`golangci-lint run ./internal/swarm/proxy/...` reports 0 issues.

## Status

Complete: the five files land on `main` as `d258c723`, `task verify`
green, the tree pushed.
