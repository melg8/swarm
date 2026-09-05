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
                               password, char, web, hunt).
internal/swarm/
  connection/                  Login flow (authentificator), game session
                               (game.go), packet framing (wire.go).
  crypt/                       Login Blowfish framing (login_crypt.go), game
                               XOR cipher (game_crypt.go), checksum.
  state/                       Live bot state tracker: character vitals,
                               world objects, inventory, events, bot registry.
  hunt/                        Auto hunt loop: engage, loot, inventory cleanup.
  npcdata/                     Generated npc/item display id to name maps
                               (tools/generate_npc_names.sh).
  webserver/                   Embedded web UI: static files, JSON snapshot
                               endpoints and the SSE event stream.
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
`127.0.0.1:7777`, account `test1`/`test`, elven fighter `test1`). The web
interface is served on `127.0.0.1:8080` by default; pass `-web ""` to
disable it or `-web 0.0.0.0:9000` to change the address:

```bash
task run:app              # or: go run ./cmd/swarm -web 127.0.0.1:8080
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

## Web interface

The bot embeds a web UI (`internal/swarm/webserver`, files in `web/`)
served from the bot process itself, so it is reachable exactly while the
bot runs. It follows the classic bot tool layout (L2Walker/Adrenaline
style: compact window, top tabs, left bot list, bottom status bar) and is
designed for many bots: the bot registry enumerates every session, the
sidebar lists them and clicking switches the observed bot. The light
theme is the default, a header button toggles to a dark theme (both are
CSS variable sets on `html[data-theme]`; the canvas reads its colors from
the same variables).

- Tabs: Map is the source of truth: the character status lives as a HUD
  panel on the canvas (name, class, level, HP/MP bars, position, facing,
  combat badge, exp/sp) and the world is drawn around it; Log is the
  rolling event feed with a filter.
- Map rendering: every unit (character, mob, player) is a circle with a
  short look direction tick from the center over the edge (L2Bot2.0
  style), colored by threat: friendly gray, passive monster green,
  aggressive amber, fighting red, dead gray-faded, players violet, self
  blue with an accent ring; ground items are gold diamonds. The dashed
  square is the server loaded zone: the 3x3 world region block (region
  size 2048, `World.broadcastPacket` reaches exactly these regions)
  around the character region - the server only spawns/updates objects
  inside it, and it scales with zoom like every world element.
- Movement interpolation: the server broadcasts MoveToLocation at most
  once per second per moving creature, so the client runs a
  requestAnimationFrame loop that advances every object from its last
  packet position toward the destination at `runSpd * moveMultiplier`
  world units per second (run/walk speeds come from NpcInfo for npcs,
  CharInfo for players and UserInfo for self; the fallback speed is
  120), clamps at the destination, corrects the clock against the
  snapshot `serverTimeMs` and exponentially smooths position jumps.
  The played character is interpolated the same way from its own
  movement broadcasts (Player.broadcastPacket sends every broadcast
  except CharInfo to the acting player itself, unlike the base
  Creature.broadcastPacket which skips self). The transmitted run and
  walk speeds are base values: the server divides the real speeds by
  the move multiplier before writing them, so the effective speed is
  always `speed * multiplier` (see UserInfo/CharInfo/AbstractNpcInfo
  writeImpl). MoveToLocation for playables only arrives at move start
  and arrival, so the whole path is interpolated from one packet at the
  exact transmitted speed - exactly what the official client does.
- Heading semantics (Mobius `LocationUtil.calculateHeadingFrom`):
  `atan2(dy, dx) * 65535 / 2pi`, 0 faces east, the angle grows clockwise
  because world y points south. The server announces arrival with a zero
  distance MoveToLocation (current == destination): keep the previous
  heading in that case, never recompute it (the delta is zero and would
  point every mob east - this was a real bug). CharInfo carries no
  heading at all: the server announces the facing of a standing player
  through the StartRotation + StopRotation pair it sends to every new
  observer (see `Player.sendInfo`), and keyboard rotation of a visible
  player is broadcast as BeginRotation 0x77 / StopRotation 0x78 (the
  client sends StartRotating 0x4A / FinishRotating 0x4B which the server
  relays). The Attack packet carries the attacker and target locations
  but no heading: the attacker faces its target, so the map computes the
  heading from the attacker -> first hit target vector (the server sets
  exactly this heading in `Creature.doAttack` before broadcasting).
- Endpoints: `GET /api/bots` (list), `GET /api/bots/{id}/state` (full JSON
  snapshot), `GET /api/bots/{id}/events` (SSE stream that pushes a
  snapshot whenever the bot state version changes), `GET /` and the
  static assets.
- The state tracker (`internal/swarm/state`) is fed by the game session
  from these packets: UserInfo (self vitals, weight and speeds), CharInfo
  (players with speeds and running/dead/combat flags), NpcInfo
  (npcs with heading, speeds, move multiplier, running/dead/combat flags
  and the attackable flag), DropItem 0x16 (fresh drops) and SpawnItem
  0x15 (items that already exist when they enter the known list - for
  example after a relogin; without it old drops stay invisible), GetItem
  0x17 (a player picked up an item: removal plus picker position),
  MoveToLocation, MoveToPawn 0x75 (chasing creatures: stop point at
  distance from the target, marks combat and the target reference -
  also for the played character itself when it chases its attack
  target), StopMove/ValidateLocation (placement and heading), Attack
  0x06 (attacker position, hit targets: marks combat for both sides,
  attacker faces the target, target location refreshes the own
  position when the bot is the target), AutoAttackStart 0x3B /
  AutoAttackStop 0x3C (auto attack flags), MyTargetSelected 0xBF (the
  own target), TargetSelected 0x39 / TargetUnselected 0x3A (targets of
  other players), ChangeMoveType 0x3E (walk/run switch: mobs walk while
  idle and run when aggroed), TeleportToLocation 0x38 (position snap),
  StatusUpdate (vitals, weight CUR_LOAD 0x0E / MAX_LOAD 0x0F changes,
  dead when hp is 0), ItemList 0x27 / InventoryUpdate 0x37 (the
  inventory: slot tracking for the hunt cleanup), DeleteObject
  (removals). Unknown packets land in the event log. The combat window
  is 10 seconds after the last fight packet.
- Threat data: the npc level, `aggroRange` and `isAggressive` ai flags
  come from the generated `internal/swarm/npcdata` maps (the C1 data
  pack marks every monster `isAggressive=false`: they only defend).
- The server sends NpcInfo names empty for most npcs (the classic client
  resolves them from NPCName-e.dat by display template id). The bot
  resolves them through `internal/swarm/npcdata`, generated from the
  Mobius data files by `tools/generate_npc_names.sh` (npc: template id
  - 1000000 -> CT0_to_C4_ids.txt -> stats/npcs/*.xml with name, level,
  ai aggroRange/isAggressive; item: stats/items). Regenerate after
  Mobius updates.
- `-hunt` is the auto hunt flag of `cmd/swarm`: `internal/swarm/hunt`
  runs a small state machine (engage -> loot -> engage) that attacks the
  closest attackable npc (AttackRequest 0x0A, the answer arrives as
  MyTargetSelected), picks up the drops around the corpse by clicking
  them (Action 0x04: the server AI walks the character to the item and
  picks it up - StopMove to self, GetItem broadcast, InventoryUpdate)
  and destroys junk inventory items (RequestDestroyItem 0x59) when the
  slots reach 70% of the 80 slot limit or the weight reaches 75%, so a
  long living bot never litters the server. Failed pickups (protected
  or unreachable items) are skipped for 30 seconds.
- The web UI is plain HTML/CSS/JS without a build step; keep it that way
  (embedded via go:embed). Watch out: top level `const` declarations are
  not `window` properties, so cross script references must use the bare
  binding name.

Sandbox signal pitfall: non-interactive bash starts background jobs with
SIGINT/SIGQUIT set to SIG_IGN, and Go cannot catch a signal that was
ignored on exec - `kill -INT $bgpid` silently does nothing. Run bots
that must exit gracefully in the foreground behind
`timeout -s INT Ns ./bot` (that is what `tools/mobius_e2e.sh` does).

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
