---
name: packet-recipe
description: >-
  The end-to-end recipe for adding or changing a Lineage 2 C1 protocol
  packet in the swarm bot (parsers in packets/, application in
  connection/, documentation in docs/protocol_description.md). Use when
  the task mentions a packet, opcode, protocol, server message, or when
  a live log shows an unknown packet id, ActionFailed storms, or
  missing server data - even if the user only names the packet like
  "RecipeBookItem" or "0xD0".
---

# Packet recipe (swarm, Mobius C1)

The Mobius C1 Java sources are the spec
(`docs/protocol_description.md` links every implemented packet to its
reference class). Never guess a wire format - read the server class
first, then the client packets in the same directory of the Mobius tree
for the opposite direction.

## Adding an inbound packet (game server -> bot)

1. **Parse**: new file `internal/swarm/packets/from_game_server/<name>.go`.
   Pure function `ParseXxxPacket(dst *XxxPacket, data []byte) error`
   filling a caller-provided struct - no logging, no I/O, no tracker
   access. Little endian integers, null-terminated UTF-16LE strings
   (`packet.Reader`). Guard implausible counts with an explicit error
   (see `inventory.go` and `attack.go` for the pattern).
2. **Test + benchmark**: `<name>_test.go` with a hand-built byte
   payload (see `world_packets_test.go` builders) and, for hot packets,
   a `*_bench_test.go` reporting allocs - packet parsing is a hot path
   for hundreds of connections.
3. **Dispatch**: one entry in the connection handler table
   (`internal/swarm/connection/game.go`) and a mechanical `applyXxx`
   method that copies the parsed fields into `state.Bot` through its
   public Apply API. Unknown ids keep landing in the unknown-packet log.
4. **Document**: an entry in `docs/protocol_description.md` with the
   byte layout and the link to the Mobius Java class.

## Adding an outbound packet (bot -> game server)

Same, under `to_game_server/`: a struct with `ToBytes(*packet.Writer)`
(see `session_packets.go`), a byte-layout unit test, and a thin
`GameClient` method calling `sendPacket`. Note the server-side flood
protectors: player actions pace at 1 request per second, transactions
at ~10 s - pace the callers, not the packet code.

## Things that bite

- `Connection/GameClient` carries ~30 reusable parse structs; a new
  packet struct joins them in `NewGameClient` (the exhaustruct
  convention wants every field initialized).
- The game wire framing is `[size:2 little endian, includes
  itself][payload]`, everything after the unencrypted
  `ProtocolVersion`/`KeyPacket` exchange rides the stateful XOR cipher
  (`crypt.GameCrypt`). The encryption and the wire write share one
  critical section in `sendPacket` - do not move the `Encrypt` out of
  it (the send/encrypt race of 2026-09-08 is covered by
  `TestGameClientConcurrentSendKeepsCipherOrder`).
- The tracker mutex is the sync point: apply methods must take it via
  the public `state.Bot` API, never reach into fields.
- After a self `TeleportToLocation` the server holds the character
  teleporting until the client answers `Appearing` (0x30) - the apply
  side of the teleport does exactly that; every new teleport-like
  server flow needs the same confirmation answer.

## Live verification

With the stack up (`bash tools/swarm_fast_deploy.sh`, see the
mobius-stack skill), run the bot with `SWARM_TRACE_PACKETS=1` to log
every received packet id, and check the effect in the web UI state or
the fake-server tests. A protocol change is done when the parser test,
the fake-server flow and one live observation agree.
