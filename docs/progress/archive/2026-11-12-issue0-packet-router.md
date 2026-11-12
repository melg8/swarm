# P05: one packet dispatch table (status: complete)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the file split rounds).

## Goal

The inbound dispatch of `internal/swarm/connection` is five chained
switch statements (`handleServerPacket` -> `handleChatPacket` ->
`handleWorldPacket` -> the object/placement/rotation/combat switches
-> `handleInventoryPacket`): one packet crosses up to six switches and
a new packet edits the group switch. Done means:

- one declarative `packetRoutes` slice holds one line per handled
  opcode, grouped by packet family;
- `packetHandlers` is the `[256]packetHandler` index built once from
  that slice, so dispatch is one array read and one call;
- a duplicate registration panics at startup (the second route would
  be unreachable and the compiler cannot see it);
- `game_router_test.go` pins the table (no duplicate, every id
  indexed) and the routing (one id per family reaches its own handler,
  the two log-only ids answer their line, an unknown id keeps the
  unknown packet log, the zero length frame stays silent);
- the `AGENTS.md` packet recipe names the table as the registration
  point.

## Constraints

- Zero behavior change for every handled packet: the same 38 opcodes,
  the same handlers, the same unknown-id path.
- The `applyXxx` methods stay where they are; the table points at
  them.
- No allocation on the dispatch path.

## Context

Measured 2026-11-12 before the change:

- 38 handled opcodes across seven families, and the ids are spread
  over the session flow, the chat, the objects, the placement, the
  rotation, the combat and the inventory switches;
- the packet id tables stay in `game.go` (they are read by the
  dispatch, the handlers and the run loop at once), so the duplicated
  constant cleanup of the P05 prompt is a separate round;
- the switch chain itself allocates nothing, so the table must keep
  the zero allocation property: a package level `[256]packetHandler`
  indexed by the opcode does.

## Progress

### 2026-11-12 - the table
`packetRoutes` (38 lines) and `buildPacketHandlers` replace the six
switch statements; `handleLeaveWorld` and `handleServerClose` become
named handlers for the two log-only ids. The mechanical check against
`HEAD` reports the same 38 opcodes, no id added or lost.

## Status

Complete: the table lands on `main` as `1e12eada`; the constant
deduplication stays open as the next P05 slice.

### 2026-11-12 - the gate and the live run
`task verify` exit 0 (build, vet, the full tree lint, the whole test
suite, `fmt:check`). The live run on the Windows stack: the binary
carries the `-ldflags` identity of `1e12eada`, `-login
127.0.0.3:2106 -hunt -web ""`, 65 s in world, then Ctrl+Break - the
character killed and looted, the `Received packet id 0x0b` line proves
the unknown-id route, `Leaving the game world with test1` / `Bot
finished`, exit code 0.
