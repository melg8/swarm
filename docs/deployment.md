# Deployment and stack operations

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

Everything about bringing the Mobius C1 test stack up, verifying it,
running the bot end to end and debugging connectivity. The short
"mandatory first step" rule itself lives in AGENTS.md; this document
holds the full detail behind it.

## The mandatory first step: fast deploy and verify

Any task that needs the live environment starts here:

```bash
bash tools/swarm_fast_deploy.sh
```

Run it in the FOREGROUND of the tool call with a call timeout of at
least 10 minutes (the "Foreground execution is mandatory" section
below holds the full rule) - a background deploy does not survive
the return of the call that started it.

The script is idempotent (finished steps are detected and skipped,
re-running is always safe), needs no root and brings the whole stack
up from a blank z.ai-style sandbox (Debian 13, no javac/go/MariaDB
preinstalled) in about 90 seconds: it clones this repo (the branch of
the build clone is configurable through `SWARM_BRANCH`, default
`main` - the historical `mobius-c1-client-1` development branch was
merged and deleted from the remote), sparse-clones the Mobius C1
module, unpacks OpenJDK 25, MariaDB and Go 1.24 from `deb.debian.org`
into `~/opt`, compiles the server, loads the 75-table database and
starts the stack. All paths can be overridden through the same-named
environment variables (`BASE`, `SWARM`, `SWARM_BRANCH`, `MOBIUS_ROOT`,
`OPT`, ...), see the script header. The script is byte-identical to
`tools/mobius_fast_deploy.sh`; `swarm_fast_deploy.sh` is the canonical
name referenced by the mandatory first step rule.

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

For a deeper end-to-end verification run the bot test (must print
`E2E_OK`):

```bash
export PATH="$HOME/opt/go-root/usr/lib/go-1.24/bin:$PATH"
export JDK_DIR="$HOME/opt/jdk25-root/usr/lib/jvm/java-25-openjdk-amd64"
export MARIADB_DIR="$HOME/opt/mariadb"
tools/mobius_e2e.sh 45
```

The `JDK_DIR` and `MARIADB_DIR` exports rebind `tools/mobius_start.sh`
and `tools/mobius_e2e.sh` (which read them from the environment) to the
`~/opt/...` layout the fast deploy unpacks; plain `mariadb`/`go` are
not on the default PATH, hence the `PATH` export.

If any check fails, stop and fix the deployment first (re-run the
script, inspect `../logs/login.log`, `../logs/game.log`,
`../logs/mariadb.log`). Only after the environment is verified as up
does the actual task start.

## The script inventory (tools/)

The `tools/` scripts reproduce the whole test environment from a blank
machine, so nobody has to rediscover where JDK, MariaDB and the Mobius
sources come from. All of them are idempotent: finished steps are
detected via marker files or installed binaries and skipped, so
re-running is always safe. They only need `bash`, `curl`, `git`, `tar`
and about 4 GB of disk.

The canonical workflow on a clean host (or after a wiped container)
that is NOT a fast-deploy sandbox:

```bash
tools/mobius_bootstrap.sh   # one time: JDK 25 + MariaDB 11.8 + Mobius
                            # clone + compile + config + database schema
tools/mobius_start.sh       # start MariaDB + login (2106) + game (7777),
                            # waits until everything is really ready
tools/mobius_e2e.sh 45      # full E2E: bot in the world 45s, then SIGINT
```

What each script does:

- `tools/mobius_env.sh` - shared configuration sourced by the other
  scripts. Every path (JDK, MariaDB, data dirs, Mobius clone, logs,
  ports, timeouts) can be overridden through environment variables of
  the same name; defaults expect the checkout layout of this project.
- `tools/mobius_bootstrap.sh` - downloads Temurin JDK 25 (Adoptium
  `latest/25/ga` API URL) and the MariaDB 11.8.9 binary tarball
  (`archive.mariadb.org`, `bintar-linux-systemd-x86_64`), installs them
  under `~/opt`, initializes the MariaDB datadir, clones
  `https://gitlab.com/MobiusDevelopment/L2J_Mobius.git` (partial clone,
  `L2J_Mobius_C1_HarbingersOfWar` module), compiles all java sources
  with the jars from `dist/libs`, applies the local config tweaks and
  loads the SQL schema (75 tables). Completion markers:
  `build_bin/.compile_ok`, `~/.sql_loaded`.
- `tools/mobius_start.sh` - starts `mariadbd`, the login server
  (`org.l2jmobius.loginserver.LoginServer`) and the game server
  (`org.l2jmobius.gameserver.GameServer`) with `nohup` when their ports
  are not listening yet, waits for ports 2106/7777 and additionally
  waits for the game server to register with the login server. Prints
  `STACK_READY`.
- `tools/mobius_e2e.sh` - builds `./cmd/swarm`, calls
  `mobius_start.sh`, runs the bot with `timeout -s INT`, reports the
  exit code, received packet count and the game server log tail. Exits
  non zero when the bot does not shut down gracefully.
- `tools/repro_stuck_trip.sh` - the live reproduction of the round 35
  stuck report: moves the offline `test1` to the reported stuck
  position (45544 45880 -2992) through the database, arms the town trip
  trigger with 41 non stackable daggers, builds and runs the bot for
  one hunt session (90 s default, `SECONDS_IN_WORLD` overrides),
  captures the mid-walk state dump into `../logs/repro_stuck_dump.txt`
  and prints `REPRO_OK` only when the hunt log shows the shop reached
  without a single stuck re-path. The daggers sell during the trip, so
  the run cleans its own trigger state; the character must be offline
  and the stack up. Run it from the swarm root (the bot resolves its
  geodata relative to the CWD).
- `tools/proxy_e2e.sh` - the live E2E of the client proxy path (needs
  the deployed stack; a fake C1 client walks the real protocol through
  the proxy, moves the character through the relay and prints
  `PROXY_E2E_OK`).

## Build identity of the produced binaries

The `swarm_bot` binaries the scripts build carry the build identity
baked in via `-ldflags -X` into `internal/version`: the branch, the
full commit hash, the tree state and the build time. The state dump of
the web UI and the first line of the bot log then tell the exact code
state they came from. A plain `go build`/`go run` identifies itself
too: the package path form (`go run ./cmd/swarm`) carries the VCS
stamp Go embeds, and even a file path form (`go run
./cmd/swarm/main.go`, no stamp - `task run:app` used to do exactly
that) resolves the branch and the commit out of the `.git` directory
of the working directory.

## Local config tweaks applied by bootstrap (both idempotent)

- `dist/game/config/GeoEngine.ini`: `PathFinding = 0` (pathfinding is
  too heavy for test containers).
- `dist/game/config/ipconfig.xml`: copied from `default-ipconfig.xml`
  (gameserver address 127.0.0.1).
- The database settings work as shipped (`Database.ini` points to
  `l2jmobiusc1`, root, empty password via the local socket).

## Windows host deployment (the actual dev environment)

The `tools/` scripts are Linux first (mariadbd tarball, `nohup`, `ss`).
The Windows machine this project is currently developed on runs the
stack from a manual deployment that was faster to set up; its layout
and provenance (verified 2026-09-05) are the reference for future work:

- Workspace: `E:\work\lineage_workspace_fresh` with `L2J_Mobius` (full
  git clone of `https://gitlab.com/MobiusDevelopment/L2J_Mobius.git`,
  branch `master` - gitlab IS reachable from this host, the older
  "gitlab unreachable, use a mirror" note is obsolete here), the
  compiled build output and the extracted runtime dist
  `L2J_Mobius_C1_HarbingersOfWar` (`libs\GameServer.jar`, `game\`,
  `login\`, `db_installer\`).
- JDK: BellSoft Liberica JDK 25 installed from the MSI (fastest path
  on Windows; Temurin MSI is equivalent). `JAVA_HOME` must be set
  system wide because the dist launchers read it.
- MariaDB: XAMPP at `C:\xampp` (bundles MariaDB 10.4), started manually
  with `C:\xampp\mysql_start.bat` (`mysqld --standalone`), database
  `l2jmobiusc1` imported through phpMyAdmin, user root with an empty
  password. The 10.4 vs 11.8 version gap works for the C1 schema.
- The dist runs from the `.vbs` launchers (`GameServer.vbs`,
  `LoginServer.vbs` etc), which read `set/java.cfg` and restart
  themselves on exit code 2).
- Network: login listens on `0.0.0.0:2106`, game on `0.0.0.0:7777`, DB
  and inter-server traffic stay on `127.0.0.1`. No active
  `ipconfig.xml`: the game server auto registers its LAN IP, so bots
  can connect via `127.0.0.1` or the LAN address.
- Shipped config differences vs the bootstrap tweaks: `PathFinding = 2`
  and `AutoPlay.ini` has `EnableAutoPlay = False` on this deployment
  (ground items must be picked by the bot itself, nothing auto loots).
- Windows dev tooling caveats: the installed Go (1.27) is newer than
  `go.mod` (1.23) - fine for building and tests. The toolchain works:
  `task` 3.53.1 (installed 2026-09-08 via `go install
  github.com/go-task/task/v3/cmd/task@latest`) and golangci-lint
  v2.13.2 (the `.golangci.yml` v2 migration of the same day; the
  strict linter set is preserved with documented exclusions - G115 for
  the wire parser integer conversions, test-file relief for fixtures -
  see the comments in `.golangci.yml`). Run `golangci-lint run` before
  considering work done. The race detector needs cgo with a gcc
  toolchain, which the Windows host lacks: `task test:race` is the
  `-race` suite for environments with cgo (the Linux sandbox, CI); the
  plain `task test` stays race free. The `exhaustruct` ->
  `exhaustruct_v5` rename (deprecated since v2.13) is a known
  follow-up - the v5 major flags new sites and needs its own round.

## Mobius stack operational notes

Lessons learned while running the stack locally; relevant when
debugging connectivity issues:

- **Never probe the login port by connecting to it.** The login server
  runs `FloodProtectorListener` on 2106: every accepted socket from one
  IP increments an in-memory counter that never decays while the
  connection state exists, and once connections are closer than
  `FastConnectionTime` (350 ms) or the count exceeds
  `MaxConnectionPerIP` (50) the server silently drops the socket,
  which the client sees as EOF on the first read. Readiness checks must
  use `ss -ltn` instead of `/dev/tcp` (this is what `port_open` in
  `tools/mobius_env.sh` does). The counter entry is removed when the
  last client connection from that IP closes, so a clean bot reconnect
  also clears it.
- **A restarted login server loses its game server registration.** A
  running game server reconnects to port 9014 and re-registers within
  seconds, but until then the server list is empty and the bot fails
  with `no available game server in the server list`.
  `mobius_start.sh` waits for the `Updated Gameserver` line in
  `login.log` for this reason.
- **Stuck `account in use` states self heal.** When the bot's login
  connection closes, `LoginClient` removes the login client and the
  flood protection entry in its `finally` block, so simply retrying
  works. Restarting the login server also clears it instantly.
- **SIGTERM on the game server is slow.** The JVM shutdown hook saves
  the whole world and can hold port 7777 open for tens of seconds,
  which looks like "already running". Wait for the process to disappear
  before restarting the stack.
- **Restricted sandbox shells may kill background processes when the
  invoking shell exits.** In such environments start the stack and run
  the bot in a single invocation - `tools/mobius_e2e.sh` is written
  exactly for that and is the reliable way to test. This is the same
  trap the "Foreground execution is mandatory" section codifies: the
  first attempt of every long script is a foreground call with a
  long timeout; backgrounding is never the plan.
- Account auto registration is already enabled by the shipped login
  config (`AutoCreateAccounts = True`), so the bot simply logs in with
  `test1`/`test` and the account is created on first use.

## Foreground execution is mandatory (the background deploy trap)

A tool call of an AI coding agent does not keep its processes alive
past the call's return: the sandboxed agent shells reap every
background process (`&`, `nohup ... &`, `setsid`, `screen`, `tmux`,
the "run in background" option of the agent tooling) when the
invocation completes. A deploy started that way dies mid-run -
usually during the javac compile or the SQL import - and the next
call finds a half-installed stack. Sessions keep paying the same
rediscovery round ("the deploy process died, restarting in the
foreground with a long timeout - the script is idempotent"); the
rule below makes the knowledge explicit so no session pays it again.

- **The first attempt is always the foreground attempt.** Run
  `tools/swarm_fast_deploy.sh`, `tools/install_dev_tools.sh`,
  `tools/mobius_bootstrap.sh`, `tools/mobius_e2e.sh`, the full
  `go test ./...` and the node web harnesses in the foreground of
  the tool call, with a call timeout of at least 10 minutes (600 s).
  The timing budget of AGENTS.md tells what a step can cost: deploy
  ~99 s, e2e ~47 s, the full test suite ~123 s - 10 minutes covers
  them all with room to spare.
- **The script call itself never goes to the background, even when
  the script starts daemons.** `tools/mobius_start.sh` daemonizes
  the JVMs with `nohup` and returns only after the ports listen and
  the game server registered with the login server - that is the
  exact shape a long command must have: the call stays foreground
  until the readiness line prints (`STACK_READY`), and only the
  daemons the script manages internally run detached.
- **If the sandbox reaps even those daemons** when the shell exits,
  do not fight it: keep each logical run in one invocation
  (`tools/mobius_e2e.sh` starts the stack, runs the bot and tears
  it down in a single foreground call - that is its purpose).
  Whether the daemons survive between calls is a property of the
  host, not of the scripts; the e2e path works in both worlds.
- **A call that died mid-run is re-run, not repaired.** All the
  `tools/` scripts are idempotent (marker files, port checks and
  the `~/opt` installs are detected and skipped), so the recovery
  from a half-deploy is the same command, foreground, longer
  timeout - never manual surgery on the half-installed tree.

The related signal problem of background jobs (SIGINT/SIGQUIT set
to SIG_IGN, which Go cannot catch) lives in the "Sandbox signal
pitfall" section below; both reduce to the same habit: the
foreground is the only place a long command is safe.

## Sandbox signal pitfall

Non-interactive bash starts background jobs with SIGINT/SIGQUIT set to
SIG_IGN, and Go cannot catch a signal that was ignored on exec -
`kill -INT $bgpid` silently does nothing. Run bots that must exit
gracefully in the foreground behind `timeout -s INT Ns ./bot` (that is
what `tools/mobius_e2e.sh` does).

## Stack logs and tunables

Stack logs always land in `../logs` next to the repo (`login.log`,
`game.log`, `mariadb.log`, `bot.log`). Server JVM memory limits and
startup timeouts are tunable via `LOGIN_JAVA_MEM`, `GAME_JAVA_MEM`,
`WAIT_LOGIN_SECS`, `WAIT_GAME_SECS`.
