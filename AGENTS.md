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
tools/                         Idempotent bash scripts that deploy and run
                               the local Mobius C1 test server stack.
docs/                          Project goals and protocol description.
```

Keep new code inside `internal/`. There is no public API yet: everything is an
internal implementation detail of the bot. The `tools/` shell scripts are the
only supported way to bring the test server stack up (see below).

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

End to end test against a live local Mobius stack (builds the bot, starts
the stack, keeps the character in the world for N seconds, sends SIGINT and
checks the graceful shutdown; N defaults to 45):

```bash
tools/mobius_e2e.sh 45
```

## Local test server deployment (tools/)

The `tools/` scripts reproduce the whole test environment from a blank
machine, so nobody has to rediscover where JDK, MariaDB and the Mobius
sources come from. All of them are idempotent: finished steps are detected
via marker files or installed binaries and skipped, so re-running is always
safe. They only need `bash`, `curl`, `git`, `tar` and about 4 GB of disk.

The canonical workflow on a clean host (or after a wiped container):

```bash
tools/mobius_bootstrap.sh   # one time: JDK 25 + MariaDB 11.8 + Mobius
                            # clone + compile + config + database schema
tools/mobius_start.sh       # start MariaDB + login (2106) + game (7777),
                            # waits until everything is really ready
tools/mobius_e2e.sh 45      # full E2E: bot in the world 45s, then SIGINT
```

What each script does:

- `tools/mobius_env.sh` — shared configuration sourced by the other
  scripts. Every path (JDK, MariaDB, data dirs, Mobius clone, logs, ports,
  timeouts) can be overridden through environment variables of the same
  name; defaults expect the checkout layout of this project.
- `tools/mobius_bootstrap.sh` — downloads Temurin JDK 25 (Adoptium
  `latest/25/ga` API URL) and the MariaDB 11.8.9 binary tarball
  (`archive.mariadb.org`, `bintar-linux-systemd-x86_64`), installs them
  under `~/opt`, initializes the MariaDB datadir, clones
  `https://gitlab.com/MobiusDevelopment/L2J_Mobius.git` (partial clone,
  `L2J_Mobius_C1_HarbingersOfWar` module), compiles all java sources with
  the jars from `dist/libs`, applies the local config tweaks and loads the
  SQL schema (75 tables). Completion markers: `build_bin/.compile_ok`,
  `~/.sql_loaded`.
- `tools/mobius_start.sh` — starts `mariadbd`, the login server
  (`org.l2jmobius.loginserver.LoginServer`) and the game server
  (`org.l2jmobius.gameserver.GameServer`) with `nohup` when their ports are
  not listening yet, waits for ports 2106/7777 and additionally waits for
  the game server to register with the login server. Prints `STACK_READY`.
- `tools/mobius_e2e.sh` — builds `./cmd/swarm`, calls `mobius_start.sh`,
  runs the bot with `timeout -s INT`, reports the exit code, received
  packet count and the game server log tail. Exits non zero when the bot
  does not shut down gracefully.

Stack logs always land in `../logs` next to the repo (`login.log`,
`game.log`, `mariadb.log`, `bot.log`). Server JVM memory limits and startup
timeouts are tunable via `LOGIN_JAVA_MEM`, `GAME_JAVA_MEM`,
`WAIT_LOGIN_SECS`, `WAIT_GAME_SECS`.

Local config tweaks applied by bootstrap (both idempotent):

- `dist/game/config/GeoEngine.ini`: `PathFinding = 0` (pathfinding is too
  heavy for test containers).
- `dist/game/config/ipconfig.xml`: copied from `default-ipconfig.xml`
  (gameserver address 127.0.0.1).
- The database settings work as shipped (`Database.ini` points to
  `l2jmobiusc1`, root, empty password via the local socket).

## Mobius stack operational notes

Lessons learned while running the stack locally; relevant when debugging
connectivity issues:

- **Never probe the login port by connecting to it.** The login server
  runs `FloodProtectorListener` on 2106: every accepted socket from one IP
  increments an in-memory counter that never decays while the connection
  state exists, and once connections are closer than `FastConnectionTime`
  (350 ms) or the count exceeds `MaxConnectionPerIP` (50) the server
  silently drops the socket, which the client sees as EOF on the first
  read. Readiness checks must use `ss -ltn` instead of `/dev/tcp` (this is
  what `port_open` in `tools/mobius_env.sh` does). The counter entry is
  removed when the last client connection from that IP closes, so a clean
  bot reconnect also clears it.
- **A restarted login server loses its game server registration.** A
  running game server reconnects to port 9014 and re-registers within
  seconds, but until then the server list is empty and the bot fails with
  `no available game server in the server list`. `mobius_start.sh` waits
  for the `Updated Gameserver` line in `login.log` for this reason.
- **Stuck `account in use` states self heal.** When the bot's login
  connection closes, `LoginClient` removes the login client and the flood
  protection entry in its `finally` block, so simply retrying works.
  Restarting the login server also clears it instantly.
- **SIGTERM on the game server is slow.** The JVM shutdown hook saves the
  whole world and can hold port 7777 open for tens of seconds, which
  looks like "already running". Wait for the process to disappear before
  restarting the stack.
- **Restricted sandbox shells may kill background processes when the
  invoking shell exits.** In such environments start the stack and run the
  bot in a single invocation — `tools/mobius_e2e.sh` is written exactly
  for that and is the reliable way to test.
- Account auto registration is already enabled by the shipped login
  config (`AutoCreateAccounts = True`), so the bot simply logs in with
  `test1`/`test` and the account is created on first use.

## Code conventions

Enforced by `.golangci-lint` config (strict, most linters enabled):

- Line length limit is 80 characters (`lll`).
- Comments must end with a period (`godot`). Comments and identifiers are in
  English.
- Every source file starts with an SPDX copyright header
  `SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>`
  followed by `SPDX-License-Identifier: MIT`. Copy it from any existing
  file (including shell scripts in `tools/`, which use `#` comments).
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
