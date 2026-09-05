<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# AGENTS.md

Guidance for AI coding agents (and humans) working on this repository.

## What this project is

swarm is an out-of-game (OOG) multi-instance proxy botting tool written in Go.
It emulates a swarm of characters connected to a Lineage 2 world. The target
server is a locally hosted
[L2J Mobius](https://gitlab.com/MobiusDevelopment/L2J_Mobius/) emulator, the
`L2J_Mobius_C1_HarbingersOfWar` module (Chronicle 1). The program must run
fully autonomously from the console, support many concurrent bots (9 minimum,
36 optimistic, up to 100 stretch goal) and a clean graceful shutdown.

The MVP client connects to the login server, authenticates (the server
auto-creates missing accounts), picks the first available game server, performs
the game protocol handshake, creates an elven fighter character when needed,
enters the world and stays online until Ctrl+C (SIGINT/SIGTERM).

Long term design goals, scalability ideas (packet deduplication, "eyes" bot
concept, synchronized party behavior) are documented in
`docs/project_description.md`. Read it before making architectural decisions.

## Tech stack

- Go 1.23.2, module path `github.com/melg8/swarm`.
- `golang.org/x/crypto` (Blowfish), `golang.org/x/text` (UTF-16 handling).
- `testify` for assertions, `sergi/go-diff` for test helpers.
- `Taskfile.yml` (go-task) wraps all routine commands. Prefer `task` aliases.
- `golangci-lint` with a strict linter set configured in `.golangci.yml`.

## Repository layout

```
cmd/swarm/                     Application entry point (flags: login, account,
                               password, char).
internal/swarm/
  connection/                  Login flow (authentificator), game session
                               (game.go), packet framing (wire.go).
  crypt/                       Login Blowfish framing (login_crypt.go), game
                               XOR cipher (game_crypt.go), checksum.
  helpers/                     Hex+ASCII dump helpers for debugging.
  packets/
    packet/                    Binary Reader/Writer primitives (little endian).
    from_auth_server/          Login server -> client packets.
    to_auth_server/            Client -> login server packets.
    from_game_server/          Game server -> client packets.
    to_game_server/            Client -> game server packets.
docs/                          Project goals and protocol description.
```

Keep new code inside `internal/`. There is no public API yet: everything is an
internal implementation detail of the bot.

## Commands

Run the application (expects the Mobius stack at `127.0.0.1:2106` and
`127.0.0.1:7777`, account `test1`/`test`, elven fighter `test1`):

```bash
task run:app              # or: go run ./cmd/swarm
```

Tests and linters (run both before considering work done):

```bash
task check:all            # lint + test
task lint                 # golangci-lint run
task lint:new             # golangci-lint run --new (changed code only)
task lint:fix             # golangci-lint run --fix
task test:cover           # go test ./... --cover --count=1
task fmt                  # go fmt ./...
task tidy                 # go mod tidy
```

Benchmarks exist for hot paths (crypt, packet parsing, hex view). Use them
when touching performance sensitive code:

```bash
go test -benchmem -run='^$' -bench '^BenchmarkEncryptor_Write$' \
  github.com/melg8/swarm/internal/swarm/crypt
```

Profiling:

```bash
go test -benchmem -cpuprofile=cpu_out -memprofile=mem_out -run='^$' \
  -bench '^BenchmarkEncryptor_Write$' github.com/melg8/swarm/internal/swarm/crypt
go tool pprof -http=localhost:8080 mem_out
```

## Code conventions

Enforced by `.golangci-lint` config (strict, most linters enabled):

- Line length limit is 80 characters (`lll`).
- Comments must end with a period (`godot`). Comments and identifiers are in
  English.
- Every source file starts with an SPDX copyright header (see existing files).
  Copy the header from any `.go` file and keep the MIT license line.
- Format with `gofmt`/`gofumpt`; imports grouped by `gci`/`goimports`.
- Function length and cyclomatic complexity are limited (`funlen`, `cyclop`,
  `gocyclo`). Split long functions instead of disabling linters.
- Initialize all struct fields when constructing (`exhaustruct`); prefer
  `NewXxx()` constructors for parsed packet structs.
- Do not return `nil` error together with a `nil` value (`nilnil`); do not
  create dynamic errors with `fmt.Errorf` without wrapping (`err113` is
  planned to be enabled): prefer `errors.New` for static messages and
  `fmt.Errorf("context: %w", err)` for wrapping.
- Check every returned error (`errcheck`); tests are linted too.
- Avoid repeated string literals, extract constants (`goconst`).
- Do not shadow predeclared identifiers (`predeclared`).

## Logging conventions

From `docs/readme.md`:

- Each log message starts with a capital letter.
- No periods at the end of log lines.
- Error messages start with `Error`.
- Use `Println`-style output; only use `\n` with `Printf`-style multi-line
  output.

## Testing conventions

- Tests live in the same package as the code (`*_test.go` next to sources).
- Every packet parser/serializer has both a unit test and a benchmark test
  where performance matters (`*_bench_test.go`, `init_bench_v2_test.go`
  pattern).
- Protocol flows are covered by in-process fake servers (see
  `connection/game_test.go` for the scripted game session test).
- Benchmarks report allocations (`-benchmem`) because a core requirement is
  memory-friendly parsing for hundreds of concurrent connections. When
  changing packet code, compare allocations before/after.
- Use `testify` (`require`/`assert`) with `testifylint`-clean style.

## Protocol notes (Mobius C1)

`docs/protocol_description.md` plus the Mobius Java sources are the source of
truth for packet formats (`L2J_Mobius_C1_HarbingersOfWar/java`). Summary:

- Game protocol version 419 (`AllowedProtocolRevisions` in
  `dist/game/config/Server.ini`).
- Packet framing both directions: 2 byte size header (little endian, includes
  itself), then the payload `[opcode: 1][body]`. Mobius reads and writes the
  header little endian (see `commons/network/pool/ResourcePool.java`).
- Login protocol: payload is padded to 8 byte alignment and ends with a 4
  byte little endian XOR checksum of all preceding 4 byte words; Blowfish
  with the static 21 byte key (`crypt.MobiusAuthKey()`, see
  `loginserver/LoginClient` `NewCrypt`). The `Init` packet is unencrypted.
- Game protocol: after the unencrypted `ProtocolVersion` <-> `KeyPacket`
  exchange, every payload is XOR encrypted with the stateful 8 byte cipher
  (`crypt.GameCrypt`, mirrors `gameserver/network/Encryption.java`): running
  XOR chain with `key[i&7]`, rolling offset kept in `key[0..3]` (little
  endian, advanced by the payload size after each packet). No checksum, no
  padding, no Blowfish on the game connection.
- Integers in packet bodies are little endian. Strings are null-terminated
  UTF-16LE (see `commons/network/packet/ReadablePacket.readString`).
- Login flow: `Init` -> `RequestAuthLogin` (fixed 14 byte zero padded
  account/password fields) -> `LoginOk` -> `RequestServerList` ->
  `ServerList` -> `RequestServerLogin` -> `PlayOk`.
- Game flow: `ProtocolVersion` -> `KeyPacket` -> `AuthLogin` (login string +
  session keys in order playOk2, playOk1, loginOk1, loginOk2) ->
  `CharSelectionInfo` -> (`CharacterCreate` -> `CharCreateOk` -> updated
  list) -> `CharacterSelect` -> `CharSelected` -> `EnterWorld` -> world
  packets; keep alive with `RequestNetPing` (0xA8) and reply `NetPing`
  (0xEC); leave with `Logout` (0x09).
- The elven fighter creation values: race 1 (ELF), classId 18
  (ELVEN_FIGHTER), see `gameserver/entity/actor/enums/player/PlayerClass`.

When adding a new packet: implement the struct in the correct direction
package (`from_*` / `to_*`), add parsing/serialization via
`packet.Reader`/`packet.Writer`, cover it with unit tests and a benchmark,
and document the layout in `docs/protocol_description.md` with a link to the
reference Mobius Java class.

## Git conventions

- `main` is the default branch. Work in feature branches.
- Commit messages are short, imperative and lowercase-ish, e.g. `fix line
  size`, `add cyclop check`, `make init packet parsing more memory friendly`.
- Do not push build artifacts (`*.out`, `*out`, binaries are gitignored) or
  `.env`.
