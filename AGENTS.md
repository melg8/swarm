<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# AGENTS.md

Guidance for AI coding agents (and humans) working on this repository.

## What this project is

swarm is an out-of-game (OOG) multi-instance proxy botting tool written in Go.
It emulates a swarm of characters connected to a Lineage 2 Chronicle 4 game
world. The target server is a locally hosted
[l2j-lisvus](https://gitlab.com/TheDnR/l2j-lisvus/) emulator with all
anticheat/antibot settings enabled in C4-only communication mode. The program
must run fully autonomously from the console, support many concurrent bots
(9 minimum, 36 optimistic, up to 100 stretch goal) and a clean graceful
shutdown.

Long term design goals, scalability ideas (packet deduplication, "eyes" bot
concept, synchronized party behavior) are documented in
`docs/project_description.md`. Read it before making architectural decisions.

The login flow currently implemented: TCP connect to auth server -> receive
`Init` -> send `RequestGGAuth` -> receive `GGAuth`. Login and game server
packets are not implemented yet.

## Tech stack

- Go 1.23.2, module path `github.com/melg8/swarm`.
- `golang.org/x/crypto` (Blowfish), `golang.org/x/text` (UTF-16 handling).
- `testify` for assertions, `sergi/go-diff` for test helpers.
- `Taskfile.yml` (go-task) wraps all routine commands. Prefer `task` aliases.
- `golangci-lint` with a strict linter set configured in `.golangci.yml`.

## Repository layout

```
cmd/swarm/                     Application entry point.
internal/swarm/
  connection/                  TCP connect + authentication flow state machine.
  crypt/                       Blowfish encryptor/decryptor, XOR checksum.
  helpers/                     Hex+ASCII dump helpers for debugging.
  packets/
    packet/                    Binary Reader/Writer primitives (little endian).
    from_auth_server/          Parsing of auth server -> client packets.
    to_auth_server/            Serialization of client -> auth server packets.
docs/                          Project goals and protocol description.
```

Keep new code inside `internal/`. There is no public API yet: everything is an
internal implementation detail of the bot.

## Commands

Run the application (expects auth server at `127.0.0.1:2106`):

```bash
task run:app              # or: go run ./cmd/swarm/main.go
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
- Initialize all struct fields when constructing (`exhaustruct`).
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
- Benchmarks report allocations (`-benchmem`) because a core requirement is
  memory-friendly parsing for hundreds of concurrent connections. When
  changing packet code, compare allocations before/after.
- Use `testify` (`require`/`assert`) with `testifylint`-clean style.

## Protocol notes (auth server)

`docs/protocol_description.md` is the source of truth for packet formats.
Summary:

- Auth protocol revision `c621` (l2j-lisvus).
- Packet layout: 2 bytes size (includes itself, little endian), 1 byte id,
  body, 0-7 bytes zero padding to 8 byte alignment, 4 bytes XOR checksum
  (big endian 4 byte blocks) for auth server communication.
- Auth server packets are encrypted with Blowfish using a hardcoded 21 byte
  key (see `crypt.DefaultAuthKey()`); the `Init` packet is sent unencrypted.
- Integers in packet bodies are little endian. Strings are null-terminated
  UTF-8 (UTF-16 in some game server packets).
- Packet processing order: check length -> concatenate partial reads ->
  decrypt -> verify checksum -> check expected packet id for current auth
  state -> deserialize -> advance state.

When adding a new packet: implement the struct in the correct direction
package (`from_auth_server` / `to_auth_server`), add parsing/serialization
via `packet.Reader`/`packet.Writer`, cover it with unit tests and a benchmark,
and document the layout in `docs/protocol_description.md` with a link to the
reference l2j-lisvus Java class.

## Git conventions

- `main` is the default branch. Work in feature branches.
- Commit messages are short, imperative and lowercase-ish, e.g. `fix line
  size`, `add cyclop check`, `make init packet parsing more memory friendly`.
- Do not push build artifacts (`*.out`, `*out`, binaries are gitignored) or
  `.env`.
