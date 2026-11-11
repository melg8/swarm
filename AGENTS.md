<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# AGENTS.md

Guidance for AI coding agents (and humans) working on this repository.
This file holds the RULES and the load-bearing FACTS only; the
implementation-level detail of every subsystem lives in `docs/` (see
the documentation map below) and is read on demand, not upfront.

## Start here (the first minute of a session)

1. Run the machine-checkable bootstrap: `bash tools/session_start.sh`
   (the git sync verdict, the login port probe, the open hypotheses,
   the active fragments).
2. Skim this file once - it holds the rules, the load-bearing facts and
   the commands, nothing else; the subsystem detail is one doc away
   (the documentation map below).
3. Load the matching playbook from `.agents/skills/` before opening
   code in its area (the Agent skills section names them; for Go source
   work the vendored `golang-*` collection is the knowledge base).
4. Find the active task context: the GitHub project board (the "In
   progress" column) names the issue, the issue comments carry the claim
   and the findings, and the task fragment under `docs/progress/`
   carries the working state. `docs/agent_progress.md` is a frozen
   archive - never append to it (the work protocol below).
5. Bring the stack up first (`tools/swarm_fast_deploy.sh` on the Linux
   sandbox, the manual deployment of `docs/deployment.md` on the Windows
   host) - no task runs against an undeployed stack.

## Time, cost and long running subprocesses (owner instruction, 2026-11-10)

- There is no wall-clock session limit: the agent runs in the owner's
  local opencode harness (DeepSeek V4.1 Flash), so do not stop mid task
  because of an elapsed-time mark - finish the atomic unit, push it,
  then report.
- Keep every verification bounded anyway. The cheap loop (`go build
  ./...`, the affected package tests, `task lint:new`, `task fmt:check`)
  answers "does it still work"; `task verify` is for a push; a long
  measurement is a separate, owner-approved question. Do not run long
  measurements (benchmarks, soak probes, timing loops, repeated live
  acceptance rounds) unasked: prefer the shortest one that answers the
  question (fewer iterations, a smaller bot count, a shorter window) and
  record the cycle time in the docs.
- Never assume a server or daemon started earlier is still serving:
  re-check it (`ss -ltn` / `netstat -ano`, `ps` / the task list) right
  before every reuse, kill the leftover and start a fresh twin (a port
  conflict means the previous instance is still alive).
- The reliable pattern is one foreground call with a timeout long
  enough for the command: it starts the service, waits for the port,
  runs every probe, stops the service and prints the result (a script
  under `scripts/` keeps it reproducible). A detached process is only
  for a service that must outlive the call; re-check it before use.

## Language (owner instruction, updated 2026-11-10)

- Answer in the language the owner prompt is written in: a Russian
  prompt gets a Russian answer, an English prompt gets an English
  answer, and the answer stays in that language until the owner
  switches. Do not translate the owner's words into another language
  in the reply.
- The code and the comments are English **always**: every identifier,
  comment, log string, test name, commit message, doc file and work
  log entry written in this repository is English. The code
  conventions section below enforces the comment part through the
  lint gate; this rule covers every produced text the gate does not
  see (the commit bodies and the docs prose).

## Tech stack at a glance (read this first)

- **Language**: Go. `go.mod` pins `go 1.27.2` and the
  `toolchain go1.27.2` line (the floor that carries the stdlib fixes of
  the 2026-11 govulncheck round): every `go` invocation inside the
  module switches to that toolchain, so the formatting and the gates
  agree on every host. Use the Go installation the host provides (the
  layout is in `docs/deployment.md`).
- **No other toolchain**: there is **no Rust, no Cargo, no C/C++ and no
  Node bundler** in the bot. Node is only used by the nine
  `tools/repro_*.js` web UI harnesses (plain JS, no npm); do not add a
  bundler or webpack.
- **Module path**: `github.com/melg8/swarm`.
- **Libraries**: `golang.org/x/crypto` (Blowfish), `golang.org/x/text`
  (UTF-16 handling), `testify` (assertions), `sergi/go-diff` (test
  helpers), `klauspost/compress` (the zstd navmesh tile pack).
- **Task runner**: `Taskfile.yml` (go-task) wraps every routine
  command - prefer the `task` aliases over bare `go`/lint calls.
- **Linter gate**: golangci-lint v2.15.0, strict (see
  `.golangci.yml`). `task`, `gofmt-spaces` and the rest of the dev tools
  are installed by `tools/install_dev_tools.sh`.

## What this project is

swarm is an out-of-game (OOG) multi-instance proxy botting tool written
in Go: it emulates a swarm of characters connected to a Lineage 2
world. The target server is a locally hosted
[L2J Mobius](https://gitlab.com/MobiusDevelopment/L2J_Mobius/) emulator,
the `L2J_Mobius_C1_HarbingersOfWar` module (Chronicle 1). The program
runs fully autonomously from the console, supports many concurrent bots
(9 minimum, 36 optimistic, up to 100 stretch goal) and shuts down
gracefully.

A bot connects to the login server, authenticates (the server
auto-creates missing accounts), picks the first available game server,
performs the game protocol handshake, creates an elven fighter when
needed, enters the world and stays online until Ctrl+C
(SIGINT/SIGTERM). A lost session (server restart, kicked connection) is
re-established automatically with a growing backoff: the process runs
24/7 and never exits on its own.

## Documentation map

The canonical index of every document, grouped by area, is
`docs/README.md`; read the one that matches the area you are about to
touch, never all of them. Each document is the reference for its
subsystem. The ones the rules below lean on most:

| Document | What it covers |
| --- | --- |
| `docs/deployment.md` | The stack: deployment, verification, logs, the operational pitfalls |
| `docs/hunting.md` | The autonomous hunt: combat safety, loot, zones, gear, town trips, deleveling |
| `docs/protocol_description.md` | The wire protocol: framing, the login and game flows, the per-packet layouts |
| `docs/webui.md` | The web UI: map, HUD, widgets, snapshot encoding, harnesses |
| `docs/pathfinding.md` | The geodata pathfinder: format, engine, test UI, benchmarks |
| `docs/navmesh.md` | The navmesh runtime, the mesh builder, the tile pack, the hybrid integration |
| `docs/hypotheses.md` | The registry of the not yet verified server assumptions |
| `docs/ROADMAP.md` | The goal ladder: the milestones and their live acceptance |
| `docs/devlog/` | The round records with the root cause analyses (read the matching rounds first) |
| `docs/progress/` | The active task fragments (the work protocol below) |

## Hypotheses and unknowns

Every server fact in this file and in `docs/` is verified - either
read from the Mobius C1 Java sources or observed on the live stack.
Anything about the server that is NOT yet verified is a hypothesis,
and a hypothesis never lives silently in a code comment, in a
conversation or in a commit message: it lives in
`docs/hypotheses.md`, with its assumption, its verification plan and
its status. Before code that relies on an unverified server
assumption is written, the assumption becomes the next free `H-NNN`
entry and the relying line carries a comment with that id. Running
the plan closes the entry: a confirmed fact moves into the matching
subsystem doc and the entry records `verified <date>, see <doc>`; ids
are never reused or renumbered.

**Advance or close one hypothesis per session** when the touched area
matches its verification plan - `tools/session_start.sh` lists the
open ids. An entry ages out of "open" only by the evidence its plan
asks for, never because a session decided it no longer matters.

## Mandatory first step of every task: deploy and verify the environment

Any task in this repository - a bug fix, a feature, a refactor, a test
run or an investigation - starts by bringing the stack up: the Linux
sandbox runs `tools/swarm_fast_deploy.sh`, the Windows host follows
the manual deployment of `docs/deployment.md`. Both install the Go
developer tools with `tools/install_dev_tools.sh` (`task`,
`golangci-lint`, `gci`, `gofmt-spaces`, `govulncheck`). Nearly every
task needs the live login server (2106), game server (7777) and
MariaDB (3306) to reproduce and validate behavior; a pure
documentation change needs only `go build ./...` and the lint gate.

```bash
bash tools/swarm_fast_deploy.sh        # ~90 s on a blank sandbox
bash tools/install_dev_tools.sh check  # preflight: exit 0 if all present
bash tools/install_dev_tools.sh        # install the missing dev tools (~10 s)
```

The deploy runs in the FOREGROUND of the shell call with a timeout of
at least 10 minutes (the time and cost section above holds the rule; it
applies to every long script). It is successful only when the last
lines print `STACK_READY: login :2106, game :7777, db :3306`, the
three ports listen (`ss -ltn`, `netstat -ano` on the Windows host) and
the schema count is 75; `tools/mobius_e2e.sh 45` (must print
`E2E_OK`) is the deeper end-to-end check of the Linux stack. The
script is idempotent and needs no root; `docs/deployment.md` holds the
full procedure, the failure handling and the stack tunables. If a
check fails, stop and fix the deployment first.

Verification budget: the deploy and the full test suite are minutes,
`go build ./...` and a warm `golangci-lint run` are seconds. `task
prepush` is the cheap pre-push answer; reach for `task verify` and the
E2E only when the change needs them.

## Work protocol: atomic commits, per-task fragments, shared files

- **Commit early, commit often.** Every finished logical unit (a
  function, a fix, a config slice, a test) is its own small atomic
  commit pushed to the remote branch immediately - never let meaningful
  changes sit only in the working tree.
- **Track the task and its progress in a per-task fragment under
  `docs/progress/`** (the naming, the structure and the rules:
  `docs/progress/README.md`): at the start write the full context
  (goal, constraints, acceptance criteria) into a NEW fragment named
  for the task, and after every atomic commit append a dated entry
  (what was done, what changed, what is next), pushed with that
  commit. When the task completes, move the fragment to
  `docs/progress/archive/` in the last commit, so `docs/progress/`
  reads as the active set.
- **Rationale: crash-safe handover.** The session can die at any
  moment, and a new agent must be able to `git pull`, read `AGENTS.md`
  plus the fragment and continue from the recorded next step - never
  keep task state only in the conversation, only in the working tree or
  only locally.
- Before starting any task, check the board's "In progress" column, the
  issue comments and `ls docs/progress/`: an unfinished fragment of a
  stalled task is resumed (verify its state against the code, then
  continue from its recorded next step) before any new work is taken.
- **Task coordination.** The claim/lease task queue is retired
  (2026-09-12, owner decision): the GitHub project board and the issue
  claims are the coordination layer, and every change must still
  advance a milestone of `docs/ROADMAP.md` - do not invent
  disconnected work.
- **Never append to a shared monolithic log or table from a branch.**
  Measured 2026-09-22 (issue #33): the retired "update
  `docs/agent_progress.md` with every atomic commit" protocol put a
  mandatory write of ONE shared file into every branch, and 7 of the 8
  then-open PRs conflicted with plain main on that file alone. The
  fragment directories end this by construction: different tasks write
  different files, so a merge is a set union, not a text clash.
- The frozen legacy logs (`docs/agent_progress.md`,
  `docs/agent_progress_archive.md`, `docs/development_log.md`) stay
  read-only forever. A branch still carrying legacy edits drops them on
  its rebase and carries that context in a `docs/progress/` fragment
  instead - one transition rebase, then the branch is clash-free.
  Per-round root-cause records go to `docs/devlog/` fragments
  (`docs/devlog/README.md`).
- The low-frequency residue: a new document gets its row in
  `docs/README.md` (the canonical index) and, when it is a core area, in
  the AGENTS.md shortlist; when a concurrent round added a neighboring
  row, keep BOTH rows.
- The reviewer-facing channel is the GitHub issue (findings, decisions,
  questions for the owner) and the PR review (concrete code comments);
  the fragment is the private working state.

## Agent skills

Operational playbooks live in `.agents/skills/`, are discovered by
the agent tooling and are read before opening code in their area; each
is a short step-by-step procedure, while this file stays the source of
truth for the rules and the facts. The seven hand-maintained project
playbooks (the upstream sync never touches them):

| Skill | Use when |
| --- | --- |
| `go-verify-loop` | Any Go change needs the build/vet/test/fmt/lint loop; `//nolint` etiquette. |
| `mobius-stack` | Bringing the stack up, login/game/MariaDB issues, E2E runs. |
| `packet-recipe` | Adding or changing a protocol packet (any opcode work). |
| `webui-harness` | Any web UI change (HTML/CSS/JS, snapshot fields, harnesses). |
| `performance` | Hot paths, SoA layout, allocations, benchmarks, fleet E2E. |
| `dump-state-repro` | A bot misbehaves on a long-lived server; reproducing the world state. |
| `e2e-repro` | A reproducible scenario (character, position, target) for an E2E test. |

Skip the skill when the task is unrelated (a typo fix needs none).

On top the repository vendors the `samber/cc-skills-golang` collection
(46 `golang-*` skills, MIT) as the generic Go knowledge base -
`golang-testing`, `golang-concurrency`, `golang-error-handling`,
`golang-lint`, `golang-code-style`, `golang-naming`,
`golang-performance`, `golang-troubleshooting` and the rest of the
family. It travels with the repository (a clone is enough, no network
access), and `tools/install_agent_skills.sh` manages it: no argument
(re)installs the pinned commit from the script header
(`SKILLS_COMMIT`), `latest` updates to upstream HEAD, `check` verifies
against the pin. The script touches the `golang-*` directories only;
after an update refresh the pin and commit the diff.

## Server integrity rules (non-negotiable)

The L2J Mobius C1 server is the reference implementation for this
project: its observed behavior is the spec the bot adapts to, never
the other way round.

- **The server comes ONLY from the official GitLab repository**
  (`https://gitlab.com/MobiusDevelopment/L2J_Mobius`, project id
  70889258, branch `master`, module directory
  `L2J_Mobius_C1_HarbingersOfWar`). Outdated copies and third party
  mirrors (GitHub mirrors, forum archives, old tarballs) are FORBIDDEN
  as a source: the mirror used before 2026-09-08 lagged three months
  behind and its datapack/SQL drift surfaced as phantom schema and
  item flag differences. A git protocol 403 (Cloudflare on the
  upload-pack endpoints) is answered by retrying or by the official
  archive API (`/api/v4/projects/MobiusDevelopment%2FL2J_Mobius/repository/archive.tar.gz?path=L2J_Mobius_C1_HarbingersOfWar`,
  one stable GET of the same commit) - `tools/swarm_fast_deploy.sh`
  implements both channels.
- **Never patch the game server to change its behavior.** Gameplay
  changes (AI decisions, damage, experience, drops, movement, guard
  retaliation) are out of bounds even when they look like obvious bugs
  - if the bot needs a behavior the server does not provide, the bot
  changes, not the server. The only accepted server-side patches are
  pure logging and diagnostics ones (a line that reveals what the
  server decided, like `DEATHLOG` / `GUARDDMG` / `MOVEDBG` in the
  local checkout); they must not alter any decision.
- Vanilla quirks discovered along the way (guards that follow without
  attacking, the Lucky newbie protection below level 10) go into the
  protocol notes, the matching subsystem doc and a `docs/devlog/`
  fragment (the frozen `docs/development_log.md` never takes appends),
  and shape the bot logic instead of a server fix. The historical
  `mobius_server_delevel.patch` (guard revenge + NPC kill penalty)
  violated this rule and was removed.
- The geodata region files live in `data/geodata` of this repository
  (the complete old-world pack, 165 regions, grid 16_10..26_26, l2j
  headerless format, sha1-verified at download time from the
  LGK-Games/Geodata mirror of the upstream pack) so the bot never
  depends on the server tree for its pathfinding; refresh the pack the
  same way when a deployment upgrade changes it. The format, the
  verification and the region list: `docs/pathfinding.md`.

## Repository layout

```
cmd/swarm/            The bot launcher and the operator CLI modes
                      (-h lists the flags)
cmd/navmesh-build/    Geodata -> navmesh tile pack
cmd/navmesh-export/   Dumps a built tile pack for external harnesses
cmd/navanalyze/       Navmesh/geodata scratch analysis
cmd/benchdiff/        The offline benchstat of `task bench:diff`
cmd/stuckprobe/       The RouteApproach verdict probe of frozen bots
cmd/cornerprobe/      The delevel guard route probe
cmd/counterprobe/     The merchant counter geometry probe
cmd/geotest/          The one-route geodata smoke probe
cmd/dbpos/            The stack DB character row dump
cmd/gofmt-spaces/     The spaces-only gofmt (`task fmt`)
internal/swarm/
  acceptance/         The live acceptance scenarios (-acceptance)
  connection/         Login flow, game session, packet framing
  crypt/              Login Blowfish, game XOR cipher, checksum
  state/              The live state tracker (the source of truth)
  hunt/               The autonomous hunt loop
  gear/               Equipment scoring and the shop planner
  npcdata/            The generated npc/item/shop tables
  packets/            The wire parsers, one package per direction
  pathfind/           The geodata path finder
  pathfind/navmesh/   The navmesh runtime (A*, funnel, approach)
  pathfind/navbuild/  The offline mesh builder
  proxy/              The MITM client proxy
  session/            The session journal and the dump report
  webserver/          The embedded web UI, its JSON and SSE endpoints
  huntaudit/          The spot audit
  fleete2e/           The 100 bot fleet benchmark
  fleetprofile/       The opt-in fleet profile window
  memwatch/           The once-a-minute process memory log
  helpers/            Debug dump helpers
internal/logfmt/      The log convention scan
internal/version/     The build identity
data/geodata/         The geodata pack (docs/pathfinding.md)
data/icons/           The item icon pack (3134 classic client PNGs)
data/client/          The redirected l2.ini for the proxy
tools/                The deploy/run scripts and the generators
.agents/skills/       The project playbooks plus the vendored Go skills
```

The per-tool detail is the doc comment of each `cmd/*/main.go`; the
subsystem detail is the documentation map above. Keep new code inside
`internal/` - there is no public API, everything is an internal
implementation detail of the bot.

## Commands

Run the application (expects the Mobius stack: login `:2106`, game
`:7777`, account `test1`/`test`, elven fighter `test1`). The login
address default is `127.0.0.1:2106`; the Windows deployment binds the
login server to `127.0.0.3:2106`, so that host passes
`-login 127.0.0.3:2106` (see `docs/deployment.md`). The web UI
listens on `127.0.0.1:8080` (`-web ""` disables it,
`-web 0.0.0.0:9000` moves it); `-bots N` launches N sessions in one
process (accounts `test2`..`testN` are auto-created):

```bash
task run:app              # or: go run ./cmd/swarm -web 127.0.0.1:8080
```

The botless modes (`-pathfind-test`, `-test-fight-ui`,
`-test-path-ui`, `-test-path-ui-v2`, `-test-fight-ui-v1`,
`-show-navmesh[=21_19]`) are described in docs/webui.md,
docs/pathfinding.md and docs/navmesh.md.

`task lint:new` is the seconds-fast changed-code verdict (`task lint`
is the full-tree gate); run the matching pair before considering work
done:

```bash
task verify               # the CI gate: build + vet + lint + test + fmt:check (alias: check:all)
task prepush              # the fast pre-push gate: build, vet, lint --new, whitespace, touched-package tests
task lint                 # golangci-lint run (full tree)
task lint:new             # golangci-lint run --new (changed code only)
task test:cover           # coverage + the per package delta vs runs/coverage-latest.txt
task bench:save PKG=./internal/swarm/pathfind           # commit runs/bench-<name>.txt
task bench:diff BASE=runs/bench-pathfind.txt NEW=<fresh>  # the offline benchstat
task fmt                  # gofmt-spaces + whitespace normalization (spaces only)
task fmt:check            # fails on any tab left in a tracked text file
task tidy                 # go mod tidy (re-tabs go.mod: run task fmt after)
task progress             # regenerate PROGRESS.md from the live sources
```

The committed CI workflow (`.github/workflows/ci.yml`) runs the same
gate, plus the race slice, the `internal/logfmt` convention scan and
govulncheck, on every push and pull request (`docs/ci_workflow.yml`
is the in-repository copy). The log scan enforces a capital first
letter (a lowercase component tag like `login#%d:` counts) and no
trailing period - fix the message, not the checker.

The live E2E (`tools/mobius_e2e.sh 45`: builds the bot, starts the
stack, stays in world N seconds, sends SIGINT, checks the graceful
shutdown) and the deployment workflow itself live in
`docs/deployment.md`; the `golang-benchmark` skill covers the
benchmark and pprof mechanics.

## Architecture rules (non-negotiable)

The live state tracker (`internal/swarm/state`) is the single source
of truth every other layer reads: `connection` feeds it from the parsed
packets, `hunt` decides against it, `webserver` serializes it into
snapshots and `proxy` patches the client replay from it. The hunt loop
is the autonomous brain (docs/hunting.md), the gear planner scores and
buys the equipment (docs/shopping_strategy.md), the zones registry
climbs the mob ladder, the pathfinder walks the geodata. Read the
matching doc before changing a layer.

The project lives or dies by its layer boundaries. A bot that grew into
a god object (the historical `state.Bot` of 2392 lines) is the failure
mode every refactor moves away from, not toward.

- **Single source of truth**: `state` is the only shared mutable
  state. The packet parsers (`packets/*`) stay pure - they fill
  caller-provided structs, never touch the tracker; `connection` calls
  the public `state.Bot` Apply API, never the struct fields;
  `webserver` reads snapshots, never talks to the connection.
- **No god objects.** A package past ~1500 lines of non-test Go is a
  refactor candidate; the first move is always a split by
  responsibility (`state` already split `objectStore`, `eventLog`,
  `chatLog` and `combatFeed` out of the bot). A function past `funlen`
  or `cyclop` is split, not suppressed with `//nolint`.
- **Testable by construction.** Every new code path arrives with a unit
  test next to it (`*_test.go`, same package) and, on a hot path, a
  benchmark. Tests use the fake-server pattern
  (`connection/game_test.go`) for protocol flows, never a live server
  unless the suite is opt-in (`fleete2e` gates on `SWARM_FLEET_E2E=1`).

## Performance and data-oriented design (the 100-bot constraint)

The stretch goal is 100 concurrent bots in one process against a live
Mobius server; `internal/swarm/fleete2e` verifies it end to end. Every
change in the hot path (state apply, packet parse, snapshot encode,
hunt tick, pathfinder) keeps that goal reachable.

- **SoA over AoS in the hot path.** The world object store is split
  into `objectHot` (position, speed, target, dead, in-combat - what the
  hunt tick and the scans read every frame) and `objectCold` (name,
  title, template id, max HP/MP) in `state/objects_store.go`. New
  fields land in the half their reader visits. Iterations walk the hot
  array by index so the prefetcher sees a contiguous block; a random
  access by object id is the `map[int32]int32` index lookup, never a
  linear scan.
- **Zero allocation is the steady state.** `AppendSnapshotJSON`
  (`state/snapshot_live.go`) walks the live state under the read lock
  and allocates nothing per event: the payload buffer is reusable, the
  view structs stay on the stack, the size estimate pre-sizes the one
  allocation. Its comment records the 26 KB of garbage per event the
  old snapshot-copy path paid on a 100-npc bot - match that bar.
- **Benchmarks are a gate, not a vanity.** Every `*_bench_test.go`
  reports `-benchmem`; a hot-path change compares allocations
  before/after and does not merge a growth without a written reason. A
  regression above 10% on `fleete2e` blocks the change until explained.
- **What is NOT a hotpath.** The web UI handlers (one per SSE client,
  human-paced), the bot startup, the gear planner (one per trip). Do not
  micro-optimize what runs once per minute.

When in doubt, measure: `go test -bench=. -benchmem -count=5` on the
affected package, then `fleete2e` for the fleet-wide view. The two
fleet instruments live next to those: the profile window
(`SWARM_FLEET_PROFILE=40`, `internal/swarm/fleetprofile`) captures the
CPU and allocation profile of the real Run cadence, and
`internal/swarm/memwatch` logs the process footprint once a minute.
Their package comments hold the invocations.

## Mobius stack operational notes

The pitfalls that bite while debugging connectivity are documented with
the full detail in `docs/deployment.md` ("Mobius stack operational
notes"); the `mobius-stack` skill is the working playbook. The three
that cost the most time:

- **Never probe the login port by connecting to it.** The login server
  runs `FloodProtectorListener` on 2106: an accepted socket from one IP
  increments a counter that never decays while the connection state
  exists, and past `MaxConnectionPerIP` (50) or inside
  `FastConnectionTime` (350 ms) the server silently drops the socket,
  which the client sees as EOF on the first read. Readiness checks use
  `ss -ltn` (what `port_open` in `tools/mobius_env.sh` does), never
  `/dev/tcp`.
- **An empty server list after a login restart means "wait".** The game
  server re-registers on port 9014 within seconds; `mobius_start.sh`
  waits for the `Updated Gameserver` line in `login.log`.
- **SIGTERM on the game server is slow**: the JVM shutdown hook saves
  the world and can hold port 7777 for tens of seconds, which looks like
  "already running" - wait for the process to disappear.

The rest (the self healing `account in use`, the
`AutoCreateAccounts = True` auto registration of `test1`/`test`, the
single foreground invocation rule of `tools/mobius_e2e.sh`) is in the
same document and in the `mobius-stack` skill.

## Client proxy

`internal/swarm/proxy` is the MITM server a real Lineage 2 C1 client
connects to (run the bot with `-proxy`). The emulated login accepts any
credentials and points the client at the proxy game port; the emulated
game server serves exactly one character (the bot selected in the web
UI) and answers the selection with the recorded `CharSelected` patched
to the live tracker; after the client's `EnterWorld` the recorded stream
is replayed and the proxy then relays both ways, re-encrypting on the
direction specific cipher chains - independent chains are the contract
of the `proxy.Transformer` seam. `docs/proxy.md` holds the l2.ini
recipes, the port layout, the relogin handoff and the `proxy.log`
triage; `tools/proxy_e2e.sh` walks the whole path (prints
`PROXY_E2E_OK`).

## Code conventions

Enforced by `.golangci.yml` (strict, most linters enabled):

- **Whitespace is spaces only, never tabs.** Four spaces per
  indentation step, repository wide (Go, go.mod, the tools scripts, the
  docs, the vendored skills). The formatter of record is
  `gofmt-spaces` (`cmd/gofmt-spaces`, run through `task fmt`): gofmt
  with every output tab replaced by four spaces, string literals
  protected; the non-Go text files are widened by
  `tools/normalize_whitespace.sh`. Do **not** run the stock `gofmt`,
  `gofumpt`, `goimports` or `golangci-lint fmt` on this tree - they all
  re-tab it, and `gci` is disabled for the same reason (its canonical
  form is tab-indented; import grouping stays with `gofmt-spaces` and
  review). `go mod tidy` re-tabs go.mod, so a `task fmt` belongs after
  it; `task fmt:check` (part of `task check:all`) fails on any tab left
  in a tracked text file.
- Line length is 80 characters (`lll`); comments end with a period
  (`godot`); comments and identifiers are English.
- Every source file starts with the SPDX header
  `SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>`
  and `SPDX-License-Identifier: MIT` (copy it from any existing file;
  shell scripts use `#` comments).
- Function length and complexity: `funlen` 65 lines / 45 statements,
  `cyclop` 15, `gocognit` 25, `maintidx` 20. A genuinely tangled
  function carries one `//nolint:<linters> // short note` line directly
  above the `func`, the note naming the refactor reason and staying
  within 80 columns; split the function when the note stops being true.
- Struct literals: a zero-value-intended partial init belongs to the
  `ignore-patterns` list of `exhaustruct_v5` in `.golangci.yml` (each
  group carries its reason); a non-zero start is set explicitly.
  `gofmt-spaces` is a gofmt clone, so the gofumpt extras (empty line
  trimming, the stricter idiom rules) do not apply.
- Errors: check every one (`errcheck`, tests included); never return a
  `nil` error with a `nil` value (`nilnil`); no dynamic error without
  wrapping - `errors.New` for a static message,
  `fmt.Errorf("context: %w", err)` for wrapping.
- Extract repeated string literals into constants (`goconst`); do not
  shadow predeclared identifiers (`predeclared`).

## Comments: the fact, not the story

The tree carries ~26k comment lines and the debt is real: a comment
that retells the code, the round or the old behavior costs a reader
more than it pays. `task comment:audit` counts it (the share, the
blocks above 8 lines, the history markers); the target is a share
under 10 percent and no block above 8 lines outside a package doc.

The rule every comment obeys:

- **A comment states a fact the code cannot say**: a server rule, a
  physical constraint, a rejected alternative, a subtle invariant. If
  the code already says it, the comment goes.
- **One sentence for a doc comment, two for an inline one.** A fact
  that needs a paragraph belongs in the matching `docs/` file, and
  the comment points there by name (`the flood protector window:
  docs/protocol_description.md`).
- **No history.** No dates, round numbers ("round 14"), "the legacy
  path", "the old implementation" - unless that behavior still exists
  in the code and a reader must avoid it. The rounds live in
  `docs/devlog/`, the measurements in the subsystem doc; the comment
  carries the current fact only.
- **No issue ids as explanation** (`issue #60`, "the run-3 finding"):
  they date a fact and the tracker outlives it. An id stays only for
  an open decision the code waits on.
- **No shouting.** One ALL-CAPS word per file at most; the current
  style spends `STRAIGHT`, `EVERY`, `NOT` on emphasis the sentence
  carries itself.
- **One word per concept** (the glossary below). A synonym invented
  per file is a bug: a reader who greps for `segment` must find every
  segment.
- **Before deleting a fact, move it to `docs/`** - in the same commit
  (the no-fact-lost rule of this file). A compressed comment that
  keeps the load-bearing number and drops the narrative is the goal.
- The mechanical part: English, capital first letter, period at the
  end (`godot`), 80 columns (`lll`).

### The glossary (one concept, one word)

| Concept | The word | Never |
| --- | --- | --- |
| A piece of a planned route (town trip, quest walk) | `segment` | leg, hop, stretch |
| One paced walk the kite issues | `step` | leg, leg of the flight |
| The armor slot and its item family | `legs` | - (matches the server `PAPERDOLL_LEGS`) |
| The defensive flee answer of a losing fight | `flight` | flee, escape run |
| A hunting ground of the zone registry | `zone` | spot, area, region |

## Tree cleanliness discipline (keep the gate green)

The full uncapped lint is **zero findings** since 2026-09-19 (the
~200 finding debt of the ungated parallel week is paid; the frozen
`docs/agent_progress_archive.md` keeps the history). The gate stays
cheap only if every commit keeps it at zero:

- `run.max-issues-per-linter` and `run.max-same-issues` are `0` in
  `.golangci.yml`: every run reports the FULL list, nothing hides
  behind the default caps.
- `task verify` and `task prepush` run before every push; the
  committed CI workflow runs the same gates plus the race slice,
  govulncheck and the coverage gate on every push and pull request.
- Before committing files another agent may have touched in parallel:
  `task fmt` first, then `golangci-lint run ./...` - a red result is
  fixed in the same commit, never passed on. Shrink the debt a touched
  function already owes (a stale `//nolint`, a function back under the
  limits) opportunistically, never in an unrelated change.
- Suppression: a `//nolint` needs an inline reason, stays on one line
  of at most 80 columns and sits directly above the line or
  declaration it silences (a wrapped prose continuation is fine, a
  second stacked directive is not - merge into `//nolint:a,b // reason`).
  `nolintlint` reports a directive that stopped matching; remove it in
  the same pass.
- Linter relief for whole paths (the analysis drivers, the C1 water
  zone data table, the test scenario scripts) lives in the
  `exclusions` rules of `.golangci.yml` with the documented reason, not
  in scattered directives.
- Dead code (`unused`) is deleted, not suppressed: an unused constant,
  field, function or test helper is a removal commit.
- The runtime artifacts of `tools/coverage_delta.sh`
  (`runs/.cover-run.log`, `runs/.coverage-current.txt`) are gitignored
  - never commit them (their `go test` output carries tabs and turns
  the whitespace gate red tree-wide). The committed baseline is
  `runs/coverage-latest.txt` only.
- Wall time budgets in tests ride `raceDetectorBudget` (the pathfind
  build tag pair: 10 under `-race`, 1 plain). The detector inflates the
  hot loop 6-10x, so a fixed budget under `-race` reports
  instrumentation cost as a failure; keep the plain figure at the
  production number.

## Testing and logging conventions

- Tests live in the same package as the code (`*_test.go` next to
  sources); every packet parser/serializer has a unit test and, where
  performance matters, a benchmark.
- Protocol flows are covered by in-process fake servers
  (`connection/game_test.go`), never a live server unless the suite is
  opt-in: `fleete2e` gates on `SWARM_FLEET_E2E=1` (its 10 minute
  100-session setup runs only when a hot path it covers changes).
- Benchmarks report allocations (`-benchmem`): memory-friendly parsing
  is a core requirement for hundreds of concurrent connections, so a
  packet change compares allocations before/after.
- Use `testify` (`require`/`assert`) with `testifylint`-clean style.
- Tests are deterministic: time windows use injected clocks (see the
  `go-verify-loop` skill); a test that depends on real wall clock timing
  is a bug in the test, not a tolerance to widen.
- A bot that misbehaves on a long-lived server carries a compact world
  snapshot (the `dump-state-repro` skill): replay it into a unit test
  instead of reproducing the live session by hand.
- Logs: a capital first letter, no trailing period, `Error`-prefixed
  error messages, `Println`-style output (`\n` only for a multi-line
  `Printf`).
- The bot log self-identifies: the line right after "Starting swarm
  bot" is `Build: <identity>` from `internal/version` (the same line the
  state dump carries), so every log tail pairs with the exact code
  state.

## Protocol notes (Mobius C1)

`docs/protocol_description.md`, the Mobius Java sources
(`L2J_Mobius_C1_HarbingersOfWar/java`) and the `packet-recipe` skill
are the source of truth for packet formats; the facts every packet
change builds on:

- Game protocol version 419 (`AllowedProtocolRevisions` in
  `dist/game/config/Server.ini`); both directions frame a 2 byte little
  endian size header (includes itself), then `[opcode: 1][body]`.
- Login traffic: the payload is padded to 8 byte alignment and ends
  with a 4 byte little endian XOR checksum of all preceding words,
  Blowfish encrypted with the static 21 byte key
  (`crypt.MobiusAuthKey()`, the `LoginClient` `NewCrypt`); `Init` is
  unencrypted.
- Game traffic: after the unencrypted `ProtocolVersion` <-> `KeyPacket`
  exchange every payload is XOR encrypted with the stateful 8 byte
  cipher (`crypt.GameCrypt`, mirrors `Encryption.java`): a running XOR
  chain with `key[i&7]` and a rolling offset in `key[0..3]` advanced by
  the payload size. No checksum, no padding, no Blowfish.
- Integers are little endian; strings are null-terminated UTF-16LE
  (`ReadablePacket.readString`).
- The elven fighter is race 1 (ELF), classId 18 (ELVEN_FIGHTER).
- After every self `TeleportToLocation` (0x38) the bot must answer the
  client `Appearing` (0x30), or the server holds the character in the
  teleporting state and silently ignores every move request - a village
  revive without the answer left the bot permanently stuck.

The packet layouts (the login and game flows, the equip and paperdoll
semantics, the buy, sell, bypass and logout packets) live in
`docs/protocol_description.md`; the hunting-side server facts (the
death penalty and deleveling numbers, the level gap drop rules, the
guard retaliation, the `isAggressive` default, the clan assist, the
shop prices) live in `docs/hunting.md` and `docs/shopping_strategy.md`.

A new packet: the struct in the correct direction package (`from_*` /
`to_*`), the parse and serialize through `packet.Reader`/`packet.Writer`,
a unit test plus a benchmark, and the layout documented in
`docs/protocol_description.md` with a link to the reference Mobius Java
class.

## Git conventions

- `main` is the default branch: a larger task gets a feature branch, a
  small round may land on `main` directly (the owner's working mode).
- Commit messages are short, imperative and lowercase-ish, e.g. `fix
  line size`, `add cyclop check`.
- Do not push build artifacts (`*.out`, `*out`, binaries are
  gitignored) or `.env`.
- **Rebase before every push.** Several sessions push to the same branch
  concurrently, so a push can be rejected as non-fast-forward at any
  moment: `git fetch origin`, `git rebase origin/<branch>`, `task
  prepush`, `git push`. Never merge remote commits into the local branch
  (the history stays linear) and never force-push (it would destroy the
  parallel sessions' work). On a conflict resolve both sides' behavior
  honestly, re-verify (at least `go build ./...`, the affected package
  tests and `golangci-lint run`), `git rebase --continue` and push
  again; a repeated rejection repeats the cycle from the fetch.
- **Always commit as melg8** (`user.name = melg8`, `user.email =
  public.melg8@gmail.com`): check both before the first commit of a
  session and set them repo-locally (never `--global`) if they differ.
  Never commit as another identity and never amend the identity of
  pushed commits.
- The GitHub side of a round (closing a landed or rejected PR,
  commenting the verdict, reading the PR state) runs through
  `bash tools/gh_token.sh ...`, which reuses the git credential
  instead of a second `gh auth login` token; `docs/deployment.md` holds
  the install and the rationale.
