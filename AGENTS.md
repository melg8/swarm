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

The development history, root cause analyses and reproduction
instructions for the fixed bugs live in `docs/development_log.md`:
read it before reworking movement rendering or the hunt behavior.

The MVP client connects to the login server, authenticates (the server
auto-creates missing accounts), picks the first available game server, performs
the game protocol handshake, creates an elven fighter character when needed,
enters the world and stays online until Ctrl+C (SIGINT/SIGTERM). A lost
session (server restart, kicked connection) is re-established
automatically with a growing backoff: the process runs 24/7 and never
exits on its own.

Long term design goals, scalability ideas (packet deduplication, "eyes" bot
concept, synchronized party behavior) are documented in
`docs/project_description.md`. Read it before making architectural decisions.

## Mandatory first step of every task: deploy and verify the environment

Any task in this repository - a bug fix, a feature, a refactor, a test run
or an investigation - starts by deploying the repository dependencies with
`tools/swarm_fast_deploy.sh` and verifying that it brought the environment up
successfully. Do not begin the actual work on an undeployed or broken stack:
nearly every task needs the live login server (2106), game server (7777) and
MariaDB (3306) to reproduce, test and validate behavior.

```bash
bash tools/swarm_fast_deploy.sh
```

The script is idempotent (finished steps are detected and skipped, re-running
is always safe), needs no root and brings the whole stack up from a blank
z.ai-style sandbox (Debian 13, no javac/go/MariaDB preinstalled) in about 90
seconds: it clones this repo at `mobius-c1-client-1`, sparse-clones the
Mobius C1 module, unpacks OpenJDK 25, MariaDB and Go 1.24 from
`deb.debian.org` into `~/opt`, compiles the server, loads the 75-table
database and starts the stack. All paths can be overridden through the
same-named environment variables (`BASE`, `SWARM`, `MOBIUS_ROOT`, `OPT`,
...), see the script header. The script is byte-identical to
`tools/mobius_fast_deploy.sh`; `swarm_fast_deploy.sh` is the canonical name
referenced by this rule.

The deploy is considered successful only when ALL of these checks pass:

1. The script finished without errors and its last lines contain
   `STACK_READY: login :2106, game :7777, db :3306`.
2. The three ports are listening:

   ```bash
   ss -ltn | grep -E ':(2106|7777|3306) '
   ```

   must list all of 2106 (login), 7777 (game) and 3306 (MariaDB).

3. The database schema is loaded (the count must be 75):

   ```bash
   ~/opt/mariadb/bin/mariadb --socket="$HOME/mysql_tmp/mysql.sock" -u root -N \
       -e "SELECT COUNT(*) FROM information_schema.tables \
           WHERE table_schema='l2jmobiusc1';"
   ```

For a deeper end-to-end verification run the bot test (must print `E2E_OK`):

```bash
export PATH="$HOME/opt/go-root/usr/lib/go-1.24/bin:$PATH"
export JDK_DIR="$HOME/opt/jdk25-root/usr/lib/jvm/java-25-openjdk-amd64"
export MARIADB_DIR="$HOME/opt/mariadb"
tools/mobius_e2e.sh 45
```

The `JDK_DIR` and `MARIADB_DIR` exports rebind `tools/mobius_start.sh` and
`tools/mobius_e2e.sh` (which read them from the environment) to the
`~/opt/...` layout the fast deploy unpacks; plain `mariadb`/`go` are not on
the default PATH, hence the `PATH` export.

If any check fails, stop and fix the deployment first (re-run the script,
inspect `../logs/login.log`, `../logs/game.log`, `../logs/mariadb.log`).
Only after the environment is verified as up does the actual task start.

## Work protocol: atomic commits and progress tracking in the repo

- **Commit early, commit often.** Every finished logical unit of work (a
  function, a fix, a config slice, a test) is its own small atomic commit
  pushed to the remote branch immediately after it is ready - never let
  meaningful changes sit only in the working tree.
- **Track the current task and its progress inside the repository**, in
  `docs/agent_progress.md`: at the start of a task write its full context
  (goal, constraints, acceptance criteria), and after every atomic commit
  append a dated progress entry (what was done, what changed, what is next).
  Update and push that file together with every atomic commit.
- **Rationale: crash-safe handover.** The agent session can die at any
  moment (connection loss, sandbox restart). A new agent must be able to
  `git pull`, read `AGENTS.md` + `docs/agent_progress.md` and continue the
  task from the exact point where the previous agent stopped, without
  rediscovering context or losing progress. Never keep task state only in
  the conversation, only in the working tree or only locally.
- Before starting any task, read `docs/agent_progress.md` first: if it
  contains an unfinished task entry, resume that task (verify the described
  state against the code, then continue from the recorded "next" step)
  before taking a new one.

## Server integrity rules (non-negotiable)

The L2J Mobius C1 server is the reference implementation for this project:
its observed behavior is the spec the bot has to adapt to, never the other
way round.

- **Never patch the game server to change its behavior.** Gameplay
  changes (AI decisions, damage, experience, drops, movement, guard
  retaliation) are out of bounds even when they look like obvious bugs -
  if the bot needs a behavior the server does not provide, the bot
  changes, not the server.
- The only accepted server-side patches are pure logging and diagnostics
  ones (a log line that reveals what the server decided and why, like the
  `DEATHLOG` / `GUARDDMG` / `MOVEDBG` lines in the local checkout). They
  must not alter any decision the server makes.
- Vanilla quirks discovered along the way (guards that follow without
  attacking, the Lucky newbie protection below level 10) go into the
  protocol notes below and into `docs/development_log.md`, and shape
  the bot logic instead of a server fix. The historical
  `mobius_server_delevel.patch` (guard revenge + NPC kill penalty)
  violated this rule and was removed.
- The geodata region files the bot navigates with live in `data/geodata`
  of this repository: the complete old-world pack (165 regions, grid
  16_10..26_26, l2j headerless format, sha1-verified at download time
  from the LGK-Games/Geodata mirror of the upstream pack; 21_19.l2j is
  byte identical with the region the running server already used) so
  the bot never depends on the server tree for its pathfinding and
  future hunting grounds beyond the elven lands are covered too;
  refresh the pack the same way when a deployment upgrade changes it.

## Tech stack

- Go 1.23.2, module path `github.com/melg8/swarm`.
- `golang.org/x/crypto` (Blowfish), `golang.org/x/text` (UTF-16 handling).
- `testify` for assertions, `sergi/go-diff` for test helpers.
- `Taskfile.yml` (go-task) wraps all routine commands. Prefer `task` aliases.
- `golangci-lint` with a strict linter set configured in `.golangci.yml`.

## Repository layout

```
cmd/swarm/                     Application entry point (flags: login, account,
                               password, char, web, hunt, pathfind-test,
                               geodata, max-passable).
internal/swarm/
  pathfind/                    Geodata path finder: l2j region loader, A* with
                               post smoothing and line of sight over the
                               Mobius C1 X_Y.l2j files (ported from
                               L2Bot2.0 / L2jGeodataPathFinder).
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
                               endpoints, the SSE event stream and the
                               pathfind test mode.
  helpers/                     Hex+ASCII dump helpers for debugging.
  packets/
    packet/                    Binary Reader/Writer primitives (little endian).
    from_auth_server/          Login server -> client packets.
    to_auth_server/            Client -> login server packets.
    from_game_server/          Game server -> client packets.
    to_game_server/            Client -> game server packets.
data/geodata/                  Geodata region files (X_Y.l2j) the bot
                               pathfinds over: the complete old-world
                               pack (165 regions, 16_10..26_26), see
                               data/geodata/Readme.txt for provenance.
data/icons/                    Item icon pack (3134 classic client PNGs)
                               served by the web UI at /icons/<name>.png;
                               the item id mapping is generated into
                               npcdata/item_icons.go. C1 compatibility
                               verified, see data/icons/Readme.txt.
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

go run ./cmd/swarm -pathfind-test   # map pathfinding test UI (no bot),
                                    # flags: -geodata DIR, -max-passable N
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

### Fast deployment inside a z.ai sandbox (tools/mobius_fast_deploy.sh)

When the deployment target is a z.ai-style sandbox (Debian 13, no root/sudo,
git+curl+gcc+OpenJDK 21 JRE preinstalled, no javac/go/MariaDB server and a
slow `archive.mariadb.org`), use `tools/mobius_fast_deploy.sh` instead of the
`mobius_bootstrap.sh` path: it brings the whole stack up from a blank
sandbox in about 90 seconds. The same script is committed as
`tools/swarm_fast_deploy.sh` (byte-identical) and that is the canonical name
used by the mandatory first step rule at the top of this document:
`bash tools/swarm_fast_deploy.sh` is how every task starts. Everything is
unpacked from `deb.debian.org` packages via `apt-get download` + `dpkg -x`
into `~/opt`
(no root needed): OpenJDK 25 (the Mobius build requires Java 25), MariaDB
server and Go 1.24; broken Debian JDK symlinks are re-bound to the
extracted tree, three MariaDB wrapper scripts export the needed
`LD_LIBRARY_PATH`, Mobius is sparse-cloned (only the C1 module) and
compiled with plain `javac`. The repo scripts `tools/mobius_start.sh` and
`tools/mobius_e2e.sh` are then used as is through the `JDK_DIR` and
`MARIADB_DIR` environment overrides. The script is idempotent: finished
steps are detected and skipped, so re-running is always safe. Paths can be
overridden through the same-named environment variables (`BASE`, `SWARM`,
`MOBIUS_ROOT`, `OPT`, ...).

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

### Windows host deployment (the actual dev environment)

The `tools/` scripts are Linux first (mariadbd tarball, `nohup`, `ss`).
The Windows machine this project is currently developed on runs the stack
from a manual deployment that was faster to set up; its layout and
provenance (verified 2026-09-05) are the reference for future work:

- Workspace: `E:\work\lineage_workspace_fresh` with `L2J_Mobius` (full
  git clone of `https://gitlab.com/MobiusDevelopment/L2J_Mobius.git`,
  branch `master` - gitlab IS reachable from this host, the older
  "gitlab unreachable, use a mirror" note is obsolete here), the
  compiled build output and the extracted runtime dist
  `L2J_Mobius_C1_HarbingersOfWar` (`libs\GameServer.jar`,
  `game\`, `login\`, `db_installer\`).
- JDK: BellSoft Liberica JDK 25 installed from the MSI (fastest path on
  Windows; Temurin MSI is equivalent). `JAVA_HOME` must be set system
  wide because the dist launchers read it.
- MariaDB: XAMPP at `C:\xampp` (bundles MariaDB 10.4), started manually
  with `C:\xampp\mysql_start.bat` (`mysqld --standalone`), database
  `l2jmobiusc1` loaded once through the dist `db_installer` (root, empty
  password, localhost). The datadir holds the live characters - never
  wipe it; test bots use their own accounts (for example `swarmqa`).
- Build: Apache Ant (`ant`) inside the C1 module compiles `java/` and
  produces the dist zip; the servers then run from the dist launchers
  `login\LoginServer.vbs` and `game\GameServer.vbs` (each reads
  `java.cfg` and restarts itself on exit code 2).
- Network: login listens on `0.0.0.0:2106`, game on `0.0.0.0:7777`, DB
  and inter-server traffic stay on `127.0.0.1`. No active
  `ipconfig.xml`: the game server auto registers its LAN IP, so bots can
  connect via `127.0.0.1` or the LAN address.
- Shipped config differences vs the bootstrap tweaks: `PathFinding = 2`
  and `AutoPlay.ini` has `EnableAutoPlay = False` on this deployment
  (ground items must be picked by the bot itself, nothing auto loots).
- Windows dev tooling caveats: the installed Go (1.27) is newer than
  `go.mod` (1.23) - fine for building and tests - and the `task` binary
  is not installed, so run the underlying commands (`go build ./...`,
  `go vet ./...`, `go test ./... -count=1`, `gofmt -l .`,
  `golangci-lint run`) directly. golangci-lint v2.13.2 works since the
  2026-09-08 migration (`.golangci.yml` is in the v2 format; the strict
  linter set is preserved with documented exclusions: G115 integer
  conversions of the wire parsers, and test-file relief for fixtures -
  see the comments in `.golangci.yml`). Run `golangci-lint run` before
  considering work done; the `exhaustruct` -> `exhaustruct_v5` rename
  (deprecated since v2.13) is a known follow-up - the v5 major flags
  new sites and needs its own round.

## Gear, shopping and multi-zone hunting

The three growth subsystems of the autonomous hunt (all unit tested;
the design goal is per-class and per-region extension):

- **Auto equipment** (`internal/swarm/gear`, `hunt/equip.go`): the
  melee fighter profile scores every equippable item (weapon = pAtk x
  attack speed, armor = pDef, jewel = mDef, shield = expected block
  value; bows score zero for melee), `NextUpgrade` plans the next
  strictly improving use item request against the tracked paperdoll
  (empty slot fills, strict slot swaps, the pair swap through freeing
  the weaker jewel, the two hand weapon and one-piece family guards)
  and the hunt loop executes one action every 2 seconds behind the
  shared confirmation gate of the manual inventory commands, so the
  paperdoll stays optimal after every loot, buy and death event.
  `gear.TotalGearPoints` summarizes the equipped gear for the zone
  gates (weapon damage per hit plus defenses).
- **Shop strategy** (`gear/shopping.go`, `hunt/shopping.go`,
  `docs/shopping_strategy.md`): the greedy value-per-adena planner
  buys the best score gain per adena first (the cheap empty slot
  fillers beat the weapon upgrades early), never buys what the
  inventory already carries and respects the adena budget; **one item
  per paperdoll slot per trip** - every purchase marks the slots it
  fills or clears (the family interplay included) and the later picks
  skip them, so no upgrade chains are bought in a single walk (the
  next trip re-plans from the reached paperdoll). The town trips
  **sell all the accumulated junk first** (the selling ends when
  nothing sellable is left, not at the 50 percent trigger), re-plan
  with the fresh adena and walk to every merchant of the plan (one
  buylist per transaction request, 11 second pacing). A trip start
  never interrupts a fight: `fightBusy` (a living target, pending
  loot, an incoming hit) holds it until the between-fights window; the
  auto equipment runs during the trips so the purchases are worn at
  the shop already. The catalogs are generated from the Mobius
  buylists (`tools/generate_shop_catalogs.sh`, keyed by packet
  template id).
- **Multi-zone hunting** (`hunt/zones.go`): the zone registry
  ladders the elven lands (keltirs 1-4 gear 0, east goblins 5-7 gear
  40, west kaboo woods 8-12 gear 110, southwest dryads 13-18 gear
  200); `PickHuntingZone` gates on level AND gear points, the loop
  re-evaluates between fights (30 s cadence), the map draws every
  zone (active amber, future dimmed with the gear gate) and the
  sidebar zone panel switches zones manually (the `zone` command,
  index in the Count field; the override holds until the character
  outgrows the band).

Extension path: a mage class implements `gear.Profile` (mAtk
weapons, robe preference - the planner, the strategy and the trip
execution stay unchanged), a new region adds its `townMerchants`
list, its zone registry entries and its tax rate. The elven
deployment is the reference wiring of all three (`main.go`:
`SetHuntingZoneRegion("elven")`).

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

## Pathfinding

The long distance movement of the bot is served by the standalone
pathfinding module `internal/swarm/pathfind`. It is a Go port of the
pathfinder behind L2Bot2.0
([L2jGeodataPathFinder](https://github.com/k0t9i/L2jGeodataPathFinder),
MIT): an A* search over the geodata cell grid with wall (NSWE) and
height checks, a supercover line rasterization for line of sight and a
string pulling post smoothing that collapses the raw cell path into
turning points. The module only reads geodata files, never talks to the
game server, so it works without a bot and can later be used as a
movement service or a hunt helper (return to the farm spot after death,
walk to town and back) as soon as the benchmarks stay acceptable -
which they currently do (see below).

- **Geodata format** (Mobius C1 `GeoEngine.java` is the reference):
  headerless little endian region files `X_Y.l2j` with the same
  `World.TILE_ZERO_COORD` anchors as the map tiles (X = floor(x / 32768)
  + 20, Y = floor(y / 32768) + 18), 65536 blocks of 8x8 cells per
  region, block kinds flat (raw 2 byte height), complex (64 cell words:
  low nibble NSWE, height = (word & 0xFFF0) >> 1) and multilayer (per
  cell layer count byte + layer words). The wire heights are quantized
  to multiples of 8, flat blocks store the raw height.
- **Engine**: `pathfind.NewEngine(dir)` scans the directory, parses
  regions lazily and keeps an LRU cache of `DefaultCacheCapacity` (4)
  parsed regions (~20 MB each). `Engine.FindPath(start, end,
  maxPassableHeight)` returns the smoothed waypoints plus the raw cell
  path, the search duration, the explored node count and the path
  length; `ErrMissingCell` marks a start or target without geodata and
  `Result.Aborted` a search that hit `MaxSearchExpansions` (1M) - an
  unreachable target in open terrain would otherwise sweep the whole
  region grid. The start z drives the layer resolution of both ends
  (the original passes the start z as the target z too), and the search
  terminates on the target cell with whatever layer the walk arrived
  on.
- **Deliberate deviations from the original**: wall hits are skipped
  instead of being pushed into the open set with an astronomic cost
  (the original can return a wall crossing path for a sealed target);
  the smoothing anchor stays on the committed waypoint, so every leg of
  the smoothed path is line of sight verified (the original jumps the
  anchor past the commit and can cut wall corners near gaps); search
  nodes are keyed by (cell, layer height) instead of one node per cell
  - a cell first touched from the water must not lose its bridge deck
  layer, that poisoning cut the Elven village bridge in half until the
  fix (regression test `TestFindPathBridgeOverWater`); the open set is
  a binary heap instead of the linear scan, ties broken by remaining
  distance and insertion order for deterministic paths.
- **Pathfind test UI**: `go run ./cmd/swarm -pathfind-test` serves the
  map without any bot behind it (`-geodata` points at a geodata
  directory, auto detected at `./data/geodata` and the reference
  deployment otherwise; `-max-passable` overrides the default 30). The
  UI opens on the hunting zone, shows draggable A and B markers (or arm
  the set A/set B buttons and click the map), draws the found path as a
  red dashed line with a small circle at every turning point, the raw
  A* cell path as a faint line on demand (`raw path` toggle) and the
  search statistics (time, nodes explored, waypoints, path length,
  loaded regions) in the sidebar. Endpoints: `GET /api/config` (mode,
  geodata summary) and `POST /api/pathfind` (start/end world points).
- **Tests and benchmarks** run without a bot or server: the synthetic
  region builder in `region_test.go` writes hand built l2j files, the
  real Giran region of the original example ships as
  `pathfind/testdata/22_22.l2j` (from the L2jGeodataPathFinder usage
  example, MIT) and anchors the parser plus the cross city benchmark
  (the original example path near the Giran weapon shop to the north
  bridge). Numbers on the dev machine: cross city path ~15 ms / 13k
  allocs, line of sight ~1.4 ms, region parse ~140 ms (one time, 21 MB
  resident) - fast enough for occasional hunt usage; the extreme
  synthetic zigzag maze stays at ~2.3 s and is bounded by the expansion
  cap. Benchmarks: `go test ./internal/swarm/pathfind/ -bench .
  -benchmem`.
- **Geodata visualization**: the pathfind test map can replace the photo
  imagery with rendered geodata (`geodata` checkbox in the toolbar, mode
  select: height / walls / layers). The bot process renders each region
  tile on demand (`GET /api/geodata/tile/{level}/{bx}_{by}.png?mode=...`,
  pyramid level 0 renders one cell per pixel, 4 more levels halve the
  resolution) and caches the encoded PNGs in a small LRU; the browser
  caches them aggressively. The height mode paints the walk surface
  grayscale with a blue tint below the typical sea level, red over cells
  with closed walls and green on multilayer cells - the walls and layers
  modes isolate the connectivity and the multilayer structure. The view
  follows exactly what the search sees at the default height: the layer
  closest to zero.
- **World navigation gaps**: `docs/navigation_analysis.md` holds the
  measured analysis of what separates the current system from universal
  A to B navigation: scale limits of the grid search (a half world land
  route costs 25M node expansions, some village pairs have no geodata
  connection at all), the missing meta transport graph (352 gatekeeper
  destinations and 3 boat routes in the server data), water movement as
  a separate cost dimension (swim speed vs run speed, breath and
  drowning on deep crossings, unverified server swimming semantics),
  doors, the danger layer over the spawn data and the missing bot side
  waypoint walker. Continue any navigation work from that document.
- **Geodata deployment**: the empty `data/geodata` folder of the
  reference deployment was filled with a 215 region C1 compatible pack
  (all files verified against the block walk rule above; sources
  documented in the session log). The game server picks the files up on
  its next restart; the pathfinder reads them directly and needs no
  server restart.

## Web interface

The UI boots in one of two modes chosen by `GET /api/config`: the bot
control mode described here and the bot less pathfind test mode (see
## Pathfinding) that reuses the same map canvas, camera and theme - a
`body.mode-pathfind` class hides the bot panels and shows the pathfind
sidebar panel.

The bot embeds a web UI (`internal/swarm/webserver`, files in `web/`)
served from the bot process itself, so it is reachable exactly while the
bot runs. It follows the classic bot tool layout (L2Walker/Adrenaline
style: compact window, top tabs, left bot list, bottom status bar) and is
designed for many bots: the bot registry enumerates every session, the
sidebar lists them and clicking switches the observed bot. The light
theme is the default, a header button toggles to a dark theme (both are
CSS variable sets on `html[data-theme]`; the canvas reads its colors from
the same variables).

- Header: brand, Map/Log tabs and the live indicator share one compact
  34 px row - the working area must not lose vertical space to chrome.
- Tabs: Map is the source of truth: the character status lives as a HUD
  panel on the canvas (name, class, level, HP/MP bars, position,
  combat/rest chips, exp/sp) and the world is drawn around it; Log is
  the rolling event feed with a filter.
- Bot list: every row shows the status dot, the name, `combat`/`rest`
  chips, the level and three mini bars (HP red, MP blue, XP silver,
  same gradients as the HUD) fed by the extended `/api/bots` payload -
  the overview shows at a glance what every session is doing.
- Camera: follow mode centers on the interpolated character position;
  free mode (follow unchecked or a map drag) pins the view to a pan
  anchor captured at the moment follow was disabled and never moves on
  its own, so the map shows the chosen area regardless of bot movement.
  Dragging grabs the map like a sheet of paper: the camera moves by
  `-Δcursor / scale` in world units, so the grabbed world point stays
  exactly under the cursor.
- World map background: the map draws the game world map tiles under
  everything (`show-map` toggle in the toolbar, on by default). The
  tiles come from the L2Bot2.0 assets
  (`E:\work\L2Bot2.0\Client\Assets\maps`, 32 world units per
  source pixel, one 1024x1024 tile per 32768 units block named
  `BX_BY.jpg` with BX = floor(x / 32768) + 20 and
  BY = floor(y / 32768) + 18 - the same anchors as World.TILE_ZERO_COORD
  of the Mobius server; the `_1`/`_2` suffixed files are dungeon floors
  and are not shipped). `tools/generate_map_tiles.sh` builds a google
  maps style pyramid into `web/maps/{level}/{bx}_{by}.jpg`: every base
  tile ships at levels 0..3 (1024/512/256/128 px, jpeg quality 65) -
  the whole world at full resolution, about 31 MB total.
  `web/map.js drawMapBackground` picks the pyramid level allowing at
  most a 2x upscale of the tile pixels, draws the tiles covering the
  viewport through the usual world to screen transform (follow and pan
  included) and lazy loads them via the static file server; a tile
  missing from the source set falls back to the closest existing
  pyramid level of the same block stretched over the tile rect; the
  grid, the zone square, the units and the links draw on top. The wheel zoom
  anchors at the cursor in the free camera mode (the world point under
  the cursor stays under it) and centers on the character while follow
  is on.
- Map rendering: the unit markers use a fixed theme independent palette
  (mapColors in map.js, also mirrored into the legend css) so the icons
  read identically over the light map imagery and over both theme
  fills; the marker outline and the direction tick use the fixed slate
  mapColors.tick for the same reason; the labels draw white (gray for
  the dead, the own target red) with a dark halo and pass a declutter
  pass
  (priority: hovered, own target, in combat, then the distance to the
  character - overlapping names are dropped, the closest win). The unit
  markers scale with the zoom (a sub linear
  factor of the map scale, clamped 0.3..1.6) so zooming out shrinks
  them together with the map - the direction ticks, the line widths,
  the link rings, the labels and the social marker follow the same
  factor - and the draw order is deterministic -
  dead units first, then north to south, then the object id - because
  the snapshot objects arrive in random go map order and overlapping
  units would flicker otherwise. A sitting character draws the
  breathing zZ marker above its dot.
- Map rendering: every unit (character, mob, player) is a circle with a
  short look direction tick from the center over the edge (L2Bot2.0
  style), colored by threat: friendly gray, passive monster green,
  aggressive amber, fighting red, dead gray-faded, players violet, self
  blue with an accent ring; ground items are gold diamonds. The dashed
  square is the server loaded zone: the 3x3 world region block (region
  size 2048, `World.broadcastPacket` reaches exactly these regions)
  around the character region - the server only spawns/updates objects
  inside it, and it scales with zoom like every world element.
- Movement interpolation: `web/map.js projectTickwise` reproduces the
  server movement exactly instead of approximating it. The Mobius
  movement loop (Creature.updatePosition) runs on 100 ms game ticks and
  advances a creature by `xAccurate += (dest - xAccurate) * frac` with
  `frac = speed * ticks / 10 / (remaining - collision)` - a converging
  geometric walk that is slightly faster than the nominal speed, stops
  collision units short and snaps to the exact destination once frac
  exceeds 1. The map replays the same recurrence tick by tick from the
  packet position, speed and collision radius (NpcInfo and CharInfo
  carry it; UserInfo does not, the played character uses the constant
  9) and interpolates linearly between the two surrounding tick
  positions, because the server truth is a step function of one jump
  per tick and the official client renders it smoothed the same way.
  The drawn position chases this projection with a speed cap of 1.35x
  the unit speed, so delivery latency, retargets and arrival snaps
  become a slightly faster glide instead of a jump, and in steady
  motion the drawn position sits on the projection with zero lag. The
  packet speeds are exact: the server re-reads its move speed every
  tick (buffs and walk/run switches take effect with the next
  broadcast), races differ through their base speeds, and the
  transmitted values divide by the move multiplier which the tracker
  multiplies back. Snapshots arrive at most every ~300 ms (SSE poll),
  but the projection is anchored at the packet time, so staleness does
  not bias the position. The clock is corrected against the snapshot
  `serverTimeMs` (the maximum of the recent samples, because every
  sample underestimates by the snapshot transport delay). MoveToLocation
  for playables only arrives at move start and arrival, non forced
  broadcasts are throttled to one per second (a re-issued move inside
  the window stays invisible). The played character is interpolated the
  same way from its own movement broadcasts (Player.broadcastPacket
  sends every broadcast except CharInfo to the acting player itself).
  Position harness: `tools/repro_movement.js` (`task repro:movement`)
  drives the real map.js with a virtual clock - the simulation mode
  replays seven patterns (random walk, long run, character walk, chase,
  pickup hops, retarget approach, npc chase) against a faithful server
  simulation (100 ms ticks, the exact recurrence, the 1 s broadcast
  throttle) and measures per frame position error against the simulated
  truth (threshold 0.25x speed in world units) and frame speed spikes
  (threshold 2.5x); `--record <seconds> [file] [url] [bot]` captures
  the live SSE stream and `--replay file [--frames]` replays it at
  60 fps, printing the drawn position of every moving unit per frame
  and failing on spikes.
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
  0x17 (a player picked up an item: removal only - the packet position
  is the item position, not the picker one, so it must never snap the
  picker), MoveToLocation, MoveToPawn 0x75 (chasing creatures: stop point at
  distance from the target, marks combat and the target reference -
  also for the played character itself when it chases its attack
  target), StopMove/ValidateLocation (placement and heading), Attack
  0x06 (attacker position, hit targets: marks combat for both sides,
  attacker faces the target, target location refreshes the own
  position when the bot is the target), AutoAttackStart 0x3B /
  AutoAttackStop 0x3C (auto attack flags), MyTargetSelected 0xBF (the
  own target), TargetSelected 0x39 / TargetUnselected 0x3A (targets of
  other players), ChangeMoveType 0x3E (walk/run switch: mobs walk while
  idle and run when aggroed), ChangeWaitType 0x3F (sit/stand
  transitions, broadcast to the acting player itself too),
  SystemMessage 0x7A (message id plus typed parameters, resolved to the
  client text by the generated npcdata.SystemMessageText dictionary -
  the loot and status lines of the chat window),
  SocialAction 0x3D (idle animations of npcs, player emotes and level
  ups - a short lived ring marker above the animating creature on the
  map; only level ups also become chat lines),
  ActionFailed 0x35 (the one byte refusal answer of the server, logged
  as "Action failed" without further reaction),
  TeleportToLocation 0x38 (position snap),
  StatusUpdate (vitals, weight CUR_LOAD 0x0E / MAX_LOAD 0x0F changes,
  dead when hp is 0; for npcs the server sends only CUR_HP/MAX_HP and
  only to whoever targeted them - npc MP never arrives), ItemList 0x27 /
  InventoryUpdate 0x37 (the inventory: slot tracking for the hunt
  cleanup), DeleteObject (removals). Unknown packets land in the event
  log; setting `SWARM_TRACE_PACKETS=1` logs every received packet id for
  live protocol debugging. The combat window is 10 seconds after the
  last fight packet; the last hit on the character is tracked separately
  (`Bot.SelfUnderAttack`, 3 s) so the rest logic never sits into the
  blows of a running fight.
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
  closest attackable npc inside the hunting square (DefaultHuntingZone:
  3300x3300 centered just below the Newbie Helper of the Elven village,
  drawn on the map as a dashed amber square from the snapshot
  huntingZone; targets and drops outside are ignored and a character
  outside walks back to the zone center), picks up the drops around the
  corpse and
  destroys junk inventory items (RequestDestroyItem 0x59) when the slots
  reach 70% of the 80 slot limit or the weight reaches 75%, so a long
  living bot never litters the server. The destroy cleanup is the last
  resort only - the normal overflow handling is the town trip below,
  and the cleanup is suspended while a trip runs so it never destroys
  what the shop would have paid for.
  Loot approach: far items are
  reached with an explicit walk first (GameClient.WalkTo sends the
  client MoveToLocation 0x01 packet, the ground click of the official
  client, in mouse mode) and the click (Action 0x04) starts at 60 units:
  the server AI covers the last stretch and executes the pickup
  (StopMove to self, GetItem broadcast, InventoryUpdate), the walk keeps
  the approach smooth on the map. Failed pickups (protected or
  unreachable items) are skipped for 30 seconds after a 20 second
  attempt. Attack start: the Mobius AttackRequest 0x0A
  has double click semantics (AttackRequest.runImpl) - the first
  request for a new target only selects it (NpcClick.onAction ->
  setTarget, answered with MyTargetSelected), the repeated request for
  the already selected target resolves to onForcedAttack and starts
  the fight. The hunt loop therefore sends the selecting request
  (GameClient.AttackNearest), then re-sends the target once per second
  (the PlayerActionFloodProtector interval; GameClient.AttackTarget)
  until the tracker sees the character engaged with it
  (state.Bot.SelfEngaged: the chase MoveToPawn, Attack or
  AutoAttackStart broadcasts set the fighting target). The whole flow
  is covered end to end by the fake server in
  internal/swarm/connection/hunt_flow_test.go. Target death and idle
  chaining: the server never clears the selection of a killed target
  (MyTargetSelected only answers new selections; the removal path only
  broadcasts the own TargetUnselected), so the tracker clears the
  character target itself when the target dies, is removed or is
  unselected, and the hunt loop only adopts the server side target id
  while it is alive - otherwise the stale dead id re-enters the loop
  every tick and the next target is never selected. The loop decides on
  a 250 ms cadence, chains the next target immediately while the
  character HP is at or above 60 percent (rate limited to one player
  action per second for the flood protector) and rests with a logged
  reason below it. Resting sits the character down below 60 percent
  (RequestActionUse 0x45 action 0, GameClient.ActionSitStand - the
  sitting regeneration is faster) and stands up at 90 percent; the
  toggle is confirmed by the ChangeWaitType 0x3F broadcast
  (state.Bot.SelfSitting) and a repeat is only sent when the flip never
  happened, so a lost packet can never leave the character toggling
  between sit and stand. While the character is under attack
  (Bot.SelfUnderAttack) the loop keeps fighting instead of resting, and
  a dead character (Bot.SelfDead: CUR_HP 0) restarts at the nearest
  village automatically (RequestRestartPoint 0x6D type 0,
  GameClient.RestartAtVillage, retried every 5 s until the server
  revives it) so a 24/7 session survives a death.
  The nearest target ranking uses the projected
  current positions of moving npcs (state.projectedPosition), not the
  stale movement packet starts. The hunt chain behavior is covered by
  internal/swarm/hunt/loop_test.go (next target selection after a
  kill, rest gate, walk to far loot, sit/stand transitions, no sitting
  under attack) and the tracker clearing by
  internal/swarm/state/tracking_test.go.
- Town trips (internal/swarm/hunt/town.go): when the inventory passes
  50% of the slots or 50% of the maximum weight, the character stops
  hunting and walks to the nearest town shop over the geodata, sells
  the junk and walks back to the farm spot (the trip start position
  inside the zone, the zone center otherwise). The trip start waits
  for the fight to end (`fightBusy`: a living target, a pending loot
  pickup or an incoming hit hold it - the loot of the kill is the
  point of the fight) and stands a resting character up first (the
  server refuses move requests while sitting; the stand toggle shares
  the pending transition gate with the rest logic). The path plan comes
  from the pathfind engine through the hunt.Navigator interface (set
  in main.go with hunt.NewNavigator from the auto detected geodata
  directory; without geodata the bot hunts without trips). A
  waypoint follower walks the smoothed path with ground click walks
  (one per 2 s, arrival within 150 units) and re-paths around
  obstacles after 15 s of standing still (3 re-paths abort the trip);
  a trip timeout (20 min) and a trigger cooldown (5 min after every
  trip end) bound the whole feature, and a death - mid trip or not -
  clears the cooldown: the village restart lands next to the shops
  and a full inventory sells right after the revival instead of
  walking to the farm spot with the junk first.
  Merchants: townMerchants carries the shop npcs of the known towns
  with their spawn coordinates; the C1 spawn ids map to the client
  display ids the NpcInfo packets carry (30147..30150 -> 7147..7150
  through CT0_to_C4_ids.txt). The sell uses RequestSellItem 0x1E with
  the standard inventory sell list (list id 0, the CUSTOM_CB_SELL_LIST
  of the official client): the server prices every item itself at
  referencePrice/2 (no prices in the packet), answers with
  InventoryUpdate removals, ItemList, a CUR_LOAD StatusUpdate and the
  "The transaction is complete." SystemMessage, and accepts the
  transaction even without a targeted merchant - the bot still walks
  to the merchant and selects it (interaction distance 250, one
  selecting request per second) like the official client, and falls
  back to selling without one after a 45 s wait. The transaction
  flood protector paces the batches (one per 11 s, up to 25 items);
  every item is offered once per trip, so items the server refuses to
  sell can not stall the phase. The junk ranking lives in
  state.Bot.SellableItems: duplicated gear pieces first (every piece
  of an item id but one), then the lowest sell value per unit weight
  (the generated npcdata.ItemPrice/ItemWeight dictionaries, see
  tools/generate_item_stats.sh); equipped gear, adena and quest items
  never sell. The trip sells everything sellable: the selling ends
  when no unsold sellable item is left (`junkRemaining`), not when
  the inventory drops back below the trigger - a buy trip with a
  30 percent bag still sells the junk, so the bot never farms with
  sellable loot it could have sold on the visit. Path layer selection: the trip legs navigate
  with pathfind.Engine.FindPathTo, which resolves the target cell
  layer against the destination z (the merchant spawn z, the farm z)
  and strictly requires the arrival on that deck - the plain search
  would happily end on the water deck below a shop standing over the
  shore. The deployed geodata pack disconnects the Elven village
  decks from the hunting fields, so startWalkLeg falls back to the
  plain search (any deck) when the targeted one reports not found:
  the sale works from anywhere (list id 0) and only the merchant
  targeting degrades - approachMerchant also gives up targeting when
  the merchant stands more than the interaction distance above or
  below the character. The
  live verified cycle (2026-09-07): trigger at 55 slots/73% weight,
  walk to the trader ~18k units in ~60 s, one batch of 25 items sold
  (55 -> 30 slots, 73% -> 36% weight), walk back and hunting resumed;
  covered offline by internal/swarm/hunt/town_test.go (trigger,
  waypoints, merchant selection, batches without a merchant, no
  destroy during the trip, stuck re-paths, death reset).
- Deleveling (internal/swarm/hunt/delevel.go): when the character
  level exceeds the median level of the living attackable npcs inside
  the zone by 7 or more AND the level is at least 10, the bot walks to
  the nearest archer guard (delevelGuards: the Elven village sentinels
  Kendell and Starden only - display ids 7218 and 7220, Elven Bow,
  ARCHER ai type; the melee sentinels Veltress and Rayen may never
  retaliate and are never provoked), approaches it into melee range
  (60 units) and provokes it with the same select-then-attack flow the
  hunt uses; the archer always answers a provocation in its line of
  sight with bow shots and kills the character, the village restart
  revives it next to the guards, where the walk to the next death
  starts again. Only deaths at level 10+ remove experience (Lucky
  absorbs the penalty below 10), so the deleveling never triggers
  below 10 and its target never aims below 9. The vanilla guard deaths
  pay the penalty from level 10 up (verified live: level 10, exp
  48229 died to the archer Kendell -> level 9, exp 46190, lost 2039
  of the 22972 level span), and a free death counter stays as the
  safety net for servers where the deaths remove nothing: three
  consecutive penalty-free deaths abort the deleveling and arm a
  30 min cooldown. The deleveling stops at the target level = median
  zone mob level + 5 (the last level with the full item drop chance,
  floored at 9) and re-triggers after hunting raised the level back
  above the trigger. The fight stage re-paths when the guard does not
  fight back within 20 s and aborts the deleveling after 3 failed
  re-paths; the whole deleveling is bounded by a 60 min timeout and a
  1 min cooldown after it ends. The walk legs are split into at most
  1000 unit steps because the server refuses move requests with a
  target farther than 9900 units (MoveToLocation readImpl); the
  smoothed geodata routes happily exceed that over open terrain. A
  death during the deleveling keeps the phase running (the town trip
  aborts instead) and the destroy cleanup stays suspended like during
  the town trips. Covered by internal/swarm/hunt/delevel_test.go
  (trigger, hysteresis, guard fight, death continuation, target exit,
  fight timeout, free death abort).
- Map target links: the map renders the selection of every visible
  player, not only the own one. The own target is a red dashed line
  with a ring; the targets of other players are violet dashed lines
  with violet dashed rings around the claimed objects (and a violet
  ring around the bot itself when the bot is the target). A mob ringed
  in violet is claimed by someone else - the precondition for not
  training other players' mobs. The tooltips show what a unit
  targets. Unit markers draw the look direction tick only outside the
  circle; inside the radius the marker is a solid fill.
- HUD: the map top left holds one vertical stack of two panels: the
  character panel (name as the heading, the class text and the
  combat/rest chips share the line under it, HP/MP/EXP bars, then a two
  column grid: level/race, x/exp, y/sp, z/slots, weight/adena with the
  weight as a bare percentage) and directly below it the target panel
  of the currently selected object (name, level chip, HP bar, MP row
  that reads no data for npcs - the C1 server never sends their MP).
  The target panel exists only while there is a target: a killed,
  removed or missing target hides it completely. Long target names live
  in their own panel and cannot break the layout. The experience bar is the third
  bar under HP and MP, filled from the snapshot `expPercent` that the
  bot computes via the C1 experience table in
  `internal/swarm/state/experience.go`, regenerated by
  `tools/generate_experience_table.sh`. The bar colors follow the
  classic L2 C1 palette (HP red, MP blue, EXP light silver - gold is
  the CP color of later chronicles) with light-top/dark-bottom
  cylindrical gradients; the three gradients are shared CSS variables
  (`--grad-hp/mp/xp`) reused by the sidebar mini bars.
- Chat window: the bottom left corner of the map shows the parsed
  system messages and the social animations (`snapshot.chat`, a rolling
  64 line ring fed by ApplySystemMessage/ApplySocialAction). The auto
  scroll follows the newest line only while the view is at the bottom
  (`chatAtBottom`, 4 px tolerance): scrolling up detaches the follow to
  read the history, scrolling back to the bottom resumes it. The list
  itself is the scroll container (the box clips, the list scrolls).
  SystemMessage texts resolve through the generated
  `npcdata/system_messages.go` dictionary (id -> client text with $sN
  placeholders, substituted positionally with the packet parameters;
  item and npc name parameters resolve through the item and npc
  dictionaries). Regenerate with `task generate:system-messages`
  (tools/generate_system_messages.sh) after Mobius updates.
- Equipment widget: a floating overlay on the map in the top right
  corner (same panel chrome as the player HUD on the left; starts
  below the compass rose, hidden in the pathfind test mode) - not a
  layout column, so the map keeps the full body width. The inventory
  packets now parse the body part mask and the
  enchant level of every entry (see AbstractItemPacket.writeItem), the
  snapshot carries the whole inventory as `snapshot.inventory` with
  the resolved display name and icon file name per item, sorted
  equipped-first. The paperdoll is compact: a 3x3 wear block ordered
  like the C1 client doll (shirt, head, cloak / weapon, chest,
  shield / gloves, legs, boots) on the
  left and a 2x3 jewelry block on the right whose middle-left cell
  is a blank hole with the necklace on the middle-right - the
  classic character has only five jewelry slots
  (two earrings, a necklace, two rings). app.js places the equipped
  items by the C1 BodyPart mask (two handed weapons and full armor
  alias onto the weapon/chest slot, the either-or earring/ring masks
  0x6/0x30 fill the first free slot of their pair); below them the
  bag renders six columns by four visible rows with a scrollbar, one
  icon cell per item with stack count and enchant badges; an empty or
  missing icon falls back to a per type2 glyph. The cells are keyed
  (slot key / item objectId) and rendered incrementally: unchanged
  items keep their DOM - the icon `<img>` elements are never
  recreated by a snapshot (a fresh element re-decodes and the icon
  blinks), stack count and enchant updates only rewrite the text
  badges, and reordering moves the persistent cells. A pinned footer
  under the scrolling bag stays always visible: the adena line (gold,
  from `character.adena`) and the weight line (fill by load percent
  from `character.load/maxLoad`) and the trash bin at the far right
  end - a cell dragged onto it destroys the item. The weight fill
  colors by the server weight debuff thresholds (Player
  refreshOverloaded of the Mobius C1 source: the load per mille
  switches the penalty at 500/666/800/1000 - 50%, 66.6%, 80% and
  100%): green below the first threshold, then the fill melts from
  yellow through orange into red (a JS hue interpolation between
  the level anchors, set inline over the green gradient), the
  percent text takes the fill color, and the tooltip carries the
  raw numbers plus the active debuff level and its speed modifier
  (x0.90/x0.87/x0.84/x0.81). The icon pack
  lives in `data/icons` (3134 PNGs of the classic client naming
  scheme from the l2walker mirror, C1 compatibility verified
  4222/4222 items - see data/icons/Readme.txt), the web server serves
  it at `/icons/<name>.png` with a day of cache - the icons directory
  is resolved from the process working directory with a walk-up, so
  launch the bot from the repo root; the item id to icon
  mapping is generated into `npcdata/item_icons.go`
  (`tools/generate_item_icons.sh`). Reproduction harness:
  `tools/repro_gear.js` (`task repro:gear`) - it also pins the keyed
  rendering (image element identity across re-renders), the pinned
  footer values, the floating placement and the manual interactions.
- The web UI is interactive in every launch mode: a double click on
  the map (move/attack/pickup - hit test over the interpolated object
  positions) or on the target HUD panel (attack the shown target),
  a double click on a widget cell (useItem: equips a wearable bag
  item into its slot, unequips an equipped one - the same C1 packet
  toggles both, see UseItem.runImpl; an occupied slot swaps: the
  equipped item comes off first, the new one equips after it) and
  drags (bag cell -> paperdoll equips with the same swap, paperdoll
  cell -> bag unequips, any cell -> map drops on the ground at the
  character feet, stackable items ask the count through a small
  dialog - the scroll wheel over the open dialog steps the count by
  one, clamped into the stack (with the all/cancel/drop buttons,
  Enter and Escape); a cell dragged onto the trash target right of
  the adena and
  weight lines destroys the item - RequestDestroyItem 0x59
  `[objectId][count]`, stacks open the count dialog in the destroy
  mode, an equipped drag unequips first). The commands flow through
  `POST /api/bots/{id}/commands` -> `state.Bot.PushCommand` (a 32
  entry queue, newest wins) -> the loop drains it every tick
  (hunt/user.go): useItem/drop/destroy execute at once, gated on
  the server confirmation of the previous one - the Mobius packet
  executor runs every client packet as its own thread pool task, so
  a same-burst unequip+equip pair raced in the paperdoll and
  cancelled each other; the gate holds the newcomer until the
  tracker observed the effect of the previous action (the equipped
  flag flipped, the count changed or the item vanished, see
  `InventoryItemState`) with a 600 ms fallback timeout so a refused
  request never blocks the queue (the UseItem flood protector of
  this build is disabled: FloodProtectorUseItemInterval = 0, retail
  matching) - a swap pair lands in ~350 ms live instead of the old
  fixed one second pause; a deferred
  command retries on a later tick with the pair order intact,
  move/attack/pickup switch the `phaseUser` manual mode that
  overrides the autonomous hunting until
  arrival/death/timeout (an active town trip is cancelled, the
  deleveling refuses movement commands - the guard walk must
  finish). The attack phase falls into the loot phase on a killed
  target, the same way as the autonomous engage. The command queue
  is drained on session resets so a reconnect never replays stale
  clicks. Without `-hunt` the loop runs in the manual only mode
  (`SetAutonomy(false)`, the `phaseIdle` phase): the commands
  execute exactly the same way, the autonomous hunting, town trips
  and deleveling stay off, the village restart after a death still
  works. The geodata engine loads in every mode and serves the
  manual long walks: the server side pathfinder silently refuses far
  targets (observed stuck walks past a few thousand units), so a
  click beyond 2000 units plans the geodata path once and follows
  the waypoints in server accepted legs (userWaypoints, 1s request
  pace, legs capped at 1000 units). A manual command that replaces
  a walk still running on the server re-issues the walk request at
  once (`userRedirect`: the next tick fires the new
  MoveToLocation instead of waiting for the old walk - the server
  replaces the destination of a running walk), so a click somewhere
  else changes the direction immediately. While a manual move runs,
  the loop publishes the walk plan into the tracker
  (`state.Bot.SetWalkPlan`: the remaining waypoints with the
  clicked destination last, refreshed every tick, expiring on its
  own after 2 s without a refresh); the snapshot carries it as
  `snapshot.walkPath` and the map draws it while the paths toggle
  is on - a blue dashed polyline from the character through the
  remaining waypoints - plus the always visible destination marker
  shared with the click ripple: a light blue dot with a pulsing
  breathing ring (the self character also gets the same dashed
  destination line as every other moving object while it runs).
- The web UI is plain HTML/CSS/JS without a build step; keep it that way
  (embedded via go:embed). Watch out: top level `const` declarations are
  not `window` properties, so cross script references must use the bare
  binding name.
- Reproduction harnesses for the web layer (all exit 1 while their bug
  is present): `tools/repro_movement.js` (`task repro:movement`) for
  the movement interpolation, `tools/repro_map_render.js` (`task
  repro:map`) for the target links, unit markers and the static free
  camera (recording canvas in a Node vm sandbox), `tools/repro_hud.js`
  (`task repro:hud`) for the HUD and target panel rendering (stub DOM),
  `tools/repro_gear.js` (`task repro:gear`) for the equipment widget
  (paperdoll masks, either-or slot resolution, badges, slot counter).

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
- After every self `TeleportToLocation` (0x38) the bot answers with the
  client `Appearing` (0x30) packet: the server holds the character in
  the teleporting state (`Creature._isTeleporting`) until `Appearing`
  arrives and silently ignores every move request meanwhile (the
  character AI checks `isMovementDisabled`). A village revive without
  the confirmation left the bot permanently stuck - the official client
  sends Appearing when the teleport screen closes.
- Death penalty and deleveling (Mobius C1): the doDie experience
  penalty branch runs for every killer - verified live on the vanilla
  server: a guard (NPC) death at level 10 paid the penalty (level 10,
  exp 48229 -> level 9, exp 46190, lost 2039 of the 22972 level span,
  the DEATHLOG log line of the local checkout records both sides of
  every death). It removes `percentLost` of the current level span
  (`data/stats/players/experienceLoss.xml`, ~9% at low levels, the
  loss is capped at 10% of the span, `Delevel` config enabled, karma
  multiplies it), and the Lucky newbie skill (id 194, granted with
  character creation) absorbs it entirely while level <= 9
  (Player.isLucky) - verified: a guard death at level 9 removes
  nothing, at level 10 the penalty lands. The
  level gap rules of the drop calculation
  (`NpcTemplate.calculateGroupDrops`): item drops slide from 100% at
  mob level + 5 to 10% at + 10 and beyond, adena from 100% at + 8 to
  10% at + 15; experience and SP stop at + 11
  (`MonsterExpMaxLevelDifference`). The elven fields mobs are levels
  1-5, so a level 11 character gets 10% item drops - the deleveling
  exists to fix that.
- The elven fighter creation values: race 1 (ELF), classId 18
  (ELVEN_FIGHTER), see `gameserver/entity/actor/enums/player/PlayerClass`.
- Equip semantics of `UseItem` (0x14, `Player.useEquippableItem` /
  `Inventory.equipItem`): a right hand weapon, chest, neck, head,
  gloves, feet or back item replaces the occupant of its slot
  directly; the paperdoll listener unequips the old item. The pair
  families fill the first EMPTY ear/finger slot (left first, right
  second) and replace the LEFT slot blindly when both are occupied -
  swapping the better jewel needs the explicit unequip of the weaker
  piece first (see `gear.NextUpgrade`). A two hand weapon
  (`lrhand`: bows, poles) unequips the left hand shield on equip, a
  shield unequips a two hand weapon, a one-piece armor
  (`onepiece`) occupies the chest slot, blocks the legs slot and
  unequips on a legs equip - the planner guards all four cases.
- Paperdoll knowledge: the inventory packets carry the template
  bodypart mask (an earring is always 0x6) and cannot tell which ear
  or finger slot an equipped jewel occupies; the UserInfo (0x04)
  paperdoll object id block is the only source for that (slot order:
  underwear, right ear, left ear, neck, right finger, left finger,
  head, right hand, left hand, gloves, chest, legs, feet, back and a
  C1 duplicate right hand). The server broadcasts UserInfo after
  every equip and unequip.
- Shop protocol: `RequestBuyItem` (0x1F) `[listId: 4][count: 4]`
  entries of `[itemId: 4][count: 4]` requires the selected merchant
  of the list within the 250 unit interaction distance; the server
  prices every entry itself (buylist product price or the item
  reference price) with the town tax: Elven Village 15 percent over
  reference while no castle owns it (`MerchantPriceConfig.xml`),
  sell is always reference/2. Buying and selling share the
  transaction flood protector (10 seconds), so buy and sell requests
  pace like the sell batches. The buylist ids are the file names of
  `data/buylists/*.xml`; the merchant of a list is the npc of its
  `<npcs>` block (join to the packet template id through
  `stats/npcs/CT0_to_C4_ids.txt`, 30147 Unoren -> 7147 etc).

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
- **Rebase before every push.** Several agent sessions push to the same
  branch concurrently, so a push can be rejected as non-fast-forward at any
  moment. The push procedure is: `git fetch origin`, `git rebase
  origin/<branch>`, then `git push`. Never merge remote commits into the
  local branch (no "Merge branch" commits - the history stays linear) and
  never force-push (it would destroy the parallel sessions' work). On a
  rebase conflict resolve both sides' behavior honestly, re-verify (at
  least `go build ./...`, the affected package tests and
  `golangci-lint run`), commit the resolution with `git rebase --continue`
  and push again; if the push is rejected again, repeat from the fetch.
- **Always commit as melg8.** The canonical committer identity of this
  repository is `user.name = melg8`, `user.email =
  public.melg8@gmail.com`. Check both with `git config user.name` and
  `git config user.email` before the first commit of a session; if they
  differ, set them for the repository with `git config user.name melg8`
  and `git config user.email public.melg8@gmail.com` (repo-local, not
  `--global`). Never commit as another identity and never amend the
  identity of commits that are already pushed.

## Deleveling live validated (2026-09-07, round 3)

The town trips (sell loop) and the deleveling cycle are live verified
end-to-end on the vanilla server (only logging patches: the
DEATHLOG/GUARDDMG/MOVEDBG lines of the local checkout; the earlier
guard revenge + NPC kill penalty server patch was reverted - see the
server integrity rules). Facts measured on the deployed stack (do not
re-derive):

- **Appearing fix confirmed working**: after adding the 0x30 reply to
  the self TeleportToLocation (connection/game.go applyTeleport), the
  death -> village revive -> walk cycle worked: consecutive
  provoke -> die -> revive -> walk-to-guard cycles.
- **Archer guards always retaliate, melee guards may not**: an archer
  guard in the attack intention shoots at everything within its 850+
  unit bow range (the thinkAttack doAttack branch applies no karma
  gate); a melee guard only follows the provoker (Guard.addDamage
  startFollow) and the chase dies in the checkTarget gate
  (Player.isAutoAttackable returns karma > 0 for guards). Verified in
  the world. The deleveling therefore provokes the archer sentinels
  Kendell and Starden only, in melee (60 unit approach).
- **Guard death experience penalty**: verified live on the vanilla
  server: a guard death at level 9 removes no experience while at
  level 10 the penalty lands (the Lucky newbie skill absorbs it below
  10; the doDie penalty branch runs for every killer, guards
  included). Live sequence: test1 level 10, exp 48229 provoked the
  archer Kendell in melee, died, and the penalty removed 2039 of the
  22972 level span -> level 9, exp 46190 (DEATHLOG lines in the game
  log). The bot's free death counter stays armed as the safety net
  for penalty-free servers and does not interfere (the productive
  death resets it).
- **Server geodata loads after the restart**: the game server runs
  with PathFinding = 2 and the region 21_19 in
  dist/game/data/geodata ("GeoEngine: Loaded 1 regions"); the bot
  pathfinds over its own repo copy in data/geodata (first candidate
  of the geodata detection).
- **Stale server side selections after abrupt disconnects**: an
  abrupt disconnect (a killed process, a dropped pipe) while the
  character auto attacks leaves the server side attack running; when
  that target dies the corpse stays SELECTED (the server never clears
  the selection, only the next selection replaces it) and every
  forced attack on the same object id of the next session comes back
  ActionFailed forever - observed live twice (inCombat true, 2
  refused actions per second, no kills) after killing the bot
  mid-farm. Bot-side recovery (no server patch): the engage drops a
  target that never starts the fight within engageStuckTimeout (12 s)
  and skips it for engageSkipDelay (30 s), so the next pick selects a
  DIFFERENT object id - that selection replaces the stale one and the
  hunt resumes. The ATTACKLOG diagnostics lines in the local server
  checkout log which AttackRequest branch refused an action. The
  reproduction is timing dependent (the kill must land while the auto
  attack runs); three deliberate kill -9 attempts did not hit the
  window again, the fix is covered by unit tests instead.
- **Delevel trigger caveat**: `MedianZoneMobLevel` sees only the
  known objects around the char; at the village (8000+ units from
  the fields) the median is 0 and the delevel does not re-trigger -
  by design the trigger fires only from the farm zone.
- **Live validation result (2026-09-07)**: the full cycle ran on the
  vanilla server: trigger at level 10 over the level 1 gremlins ->
  pathfinding walk to Kendell (~85 s, geodata from the repo
  data/geodata) -> melee provocation -> the archer killed the
  character in ~7 s (GUARDDMG at distance 0) -> exp penalty: level
  10 -> 9 (DEATHLOG) -> "delevel finished at level 9, walking back"
  -> pathfinding walk back to the farm spot (~62 s) -> farming
  resumed (kills with loot). The bot then kept hunting at level 9
  until the test window ended.
