# P05 follow-up: the packet packages own the opcodes (status: complete)

Started 2026-11-12, branch main, issue0 (the constant deduplication
slice of the P05 prompt, continued after the dispatch table round).

## Goal

`internal/swarm/connection/game.go` used to declare the opcode of every
inbound packet next to the handlers, while every parser in
`packets/from_game_server` declared the same opcode again for its own
`expectPacketID` check - two lists of the same 41 values whose drift
would only surface as a silent misparse. The login reader carried the
same duplication for `Init` (0x00) and `LoginFail` (0x01), and the send
path carried a third copy of the client `MoveToLocation` (0x01).

Done means:

- every parser package exports the opcode of the packet it parses;
- `connection` reads those constants and keeps no copy of its own;
- only the two ids without a parser (`LeaveWorld` 0x96 and
  `ServerClose` 0x36, both bodyless notifications the handlers only
  log) stay local, documented as such;
- the dispatch table lines stay under the 80 column gate.

## Constraints

- No behavior change: the same 41 opcodes reach the same handlers, the
  duplicate-registration panic of `buildPacketHandlers` stays.
- The parser packages keep their unexported helper names (`expectPacketID`
  and the parse functions) untouched.
- `task fmt` (gofmt-spaces) after the move, then the full gate.

## Context

Measured 2026-11-12 before the change:

- 41 ids duplicated between `connection/game.go` (plus `authentificator.go`
  and the send path) and the packets packages;
- the parser consts were already used by `expectPacketID`, so the value
  had a single reader inside the package and a second copy in
  `connection`;
- the mechanical comparison of the old constants against the new
  exported ones reports 41 pairs, 0 mismatches, and the two auth ids
  (`0x00`, `0x01`) match.

## Progress

### 2026-11-12 - the move
`from_game_server` exports `XxxPacketID` per parser (matching the packet
type names, `NpcInfoPacketID`, `NpcHTMLMessagePacketID`), `from_auth_server`
exports `InitPacketID` and `LoginFailPacketID`, `to_game_server` exports
`MoveToLocationPacketID`. The `connection` id blocks are gone: the
dispatch table, the login flow, the send path and the router test read
the packaged constants. `leaveWorldID` and `serverCloseID` stay with a
comment explaining they have no parser.

Verified: `go build ./...`, `go vet` on `connection` and `packets`,
`golangci-lint run` 0 issues on both, `go test ./internal/swarm/packets/...`
and `./internal/swarm/connection/...` green (10.5 s), the old-to-new id
comparison reports 0 mismatches over the 41 pairs.

Complete: the slice lands on `main` as `912326c2`; `task verify`
green and the live run clean.

### 2026-11-12 - the gate and the live run
`task verify` exit 0 (build, vet, the full tree lint, the whole test
suite, `fmt:check`). The live run on the Windows stack: the binary
carries the `-ldflags` identity of `912326c2`, `-login
127.0.0.3:2106 -hunt -web ""`, 65 s in world, then Ctrl+Break - the
dispatch table answered the spawn burst, the kill and the loot, the
unknown id 0x0b kept its log line, `Leaving the game world with test1`
/ `Bot finished`, exit code 0.
