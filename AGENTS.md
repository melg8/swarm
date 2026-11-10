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

1. Skim this file once, top to bottom - it is the rules, the
   load-bearing facts and the commands, nothing else. The
   machine-checkable bootstrap is one command:
   `bash tools/session_start.sh` (the git sync verdict, the login
   port probe, the open hypotheses, the active fragments).
2. Find your area in the documentation map below and read that one
   doc, on demand - never all of them.
3. Load the matching playbook from `.agents/skills/` before opening
   code in its area (the Agent skills section below names them;
   for Go source work the vendored `golang-*` collection is the
   knowledge base).
4. Find the active task context: the GitHub project board (the "In
   progress" column) names the issue, the issue comments carry the
   claim and the findings, and the task fragment under
   `docs/progress/` (on the branch the task lives on) carries the
   working state. The legacy `docs/agent_progress.md` is a frozen
   archive - never append to it (the shared-file clash policy below).
5. Bring the stack up first (the Linux sandbox runs
   `tools/swarm_fast_deploy.sh`, the Windows host follows the manual
   deployment of `docs/deployment.md`) - no task runs against an
   undeployed stack.

## Time and cost discipline (owner instruction, updated 2026-11-10)

- There is no wall-clock session limit: the agent runs in the
  owner's local opencode harness (DeepSeek V4.1 Flash), not the
  retired z.ai sandbox. Do not stop mid task because of an
  elapsed-time mark - finish the atomic unit, push it, then report.
- Keep every verification bounded anyway. The cheap loop (`go build
  ./...`, the affected package tests, `task lint:new`, `task
  fmt:check`) answers "does it still work"; the full gate (`task
  verify`) is for a push; a long measurement is a separate,
  owner-approved question.
- Do not run long measurements (benchmarks, soak probes, timing
  loops, repeated live acceptance rounds) unless the owner asked
  for them. Prefer the shortest measurement that answers the
  question (fewer iterations, a smaller bot count, a shorter
  window) and record the cycle time in the docs so the next agent
  does not re-measure blindly.
- A foreground tool call still needs an explicit timeout long
  enough for the command (the deploy and the full suite are
  minutes, not seconds). Run long scripts in the foreground per
  docs/deployment.md rather than splitting them artificially.

## Long running subprocesses

- Never assume a server or daemon started earlier is still
  serving: re-check it (`ss -ltn` / `netstat -ano`, `ps` / the
  task list) immediately before every reuse, kill the leftover and
  start a fresh twin (a port conflict means the previous instance
  is still alive), and never hand a long task to a background
  process and walk away.
- The reliable pattern is one foreground call that starts the
  service, waits for the port, runs every probe, stops the service
  and prints the result (a script under `scripts/` keeps it
  reproducible). A detached process is only for a service that
  must outlive the call; the Linux sandbox reaper may still kill
  it, so re-check before use. The local Windows host has no
  reaper, which makes the stale-instance check above more
  important.

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

- **Language**: Go (deployed by `tools/swarm_fast_deploy.sh`).
  `go.mod` pins `go 1.27.2` and the `toolchain go1.27.2` line (the
  floor that carries the stdlib fixes of the 2026-11 govulncheck
  round) - every `go` invocation inside the module switches to the
  go1.27.2 toolchain, so the formatting and the gates agree on every
  host. There is **no Rust, no Node, no C** in
  the bot itself. Node is only used by the nine `tools/repro_*.js`
  web UI harnesses (plain JS, no npm).
- **Module path**: `github.com/melg8/swarm`.
- **Libraries**: `golang.org/x/crypto` (Blowfish), `golang.org/x/text`
  (UTF-16 handling), `testify` (assertions), `sergi/go-diff` (test
  helpers), `klauspost/compress` (the zstd navmesh tile pack).
- **Task runner**: `Taskfile.yml` (go-task) wraps every routine
  command - prefer the `task` aliases over bare `go`/lint calls.
- **Linter gate**: golangci-lint v2.15.0, strict (see
  `.golangci.yml`). `task` and the repository formatter
  `gofmt-spaces` are installed by `tools/install_dev_tools.sh`.

Do **not** install Rust, Cargo, Node bundlers, webpack or any C/C++
toolchain. Use the Go installation the host provides (the deployment
layout of the Linux sandbox and the Windows host lives in
`docs/deployment.md`); the module's `toolchain go1.27.2` line selects
the actual toolchain on either host.

## What this project is

swarm is an out-of-game (OOG) multi-instance proxy botting tool written
in Go. It emulates a swarm of characters connected to a Lineage 2
world. The target server is a locally hosted
[L2J Mobius](https://gitlab.com/MobiusDevelopment/L2J_Mobius/) emulator,
the `L2J_Mobius_C1_HarbingersOfWar` module (Chronicle 1). The program
must run fully autonomously from the console, support many concurrent
bots (9 minimum, 36 optimistic, up to 100 stretch goal) and a clean
graceful shutdown.

The MVP client connects to the login server, authenticates (the server
auto-creates missing accounts), picks the first available game server,
performs the game protocol handshake, creates an elven fighter
character when needed, enters the world and stays online until Ctrl+C
(SIGINT/SIGTERM). A lost session (server restart, kicked connection) is
re-established automatically with a growing backoff: the process runs
24/7 and never exits on its own.

## Documentation map

Read the document that matches the area you are about to touch; every
one of them is the reference for its subsystem:

| Document | What it covers |
| --- | --- |
| `docs/deployment.md` | Bringing the stack up (fast deploy, bootstrap, scripts), Windows host layout, stack logs, the Mobius operational pitfalls (flood protector, login re-registration, slow SIGTERM) |
| `docs/launch_config.md` | The launch configuration file (the -config flag): the JSON format, the flag precedence rules, the fleet composition (bot types with counts), the default config |
| `docs/hunting.md` | The autonomous hunt: hunt loop, combat safety, blind engage recovery, multi-zone hunting, the hexagon cell system, auto equipment, shop strategy execution, town trips, deleveling, live-validated facts |
| `docs/hunting_cells.md` | The hexagon cell hunting: the uniform ground partition, the visibility budget, the enemy-first roam, the ripeness-paced neighbor rotation, the mesh endpoint, the registry invariants |
| `docs/kite_timing_findings.md` | The measured Mobius C1 bow cycle (the shot, the windup end, the disable end) and the retreat-click ladder behind the shot-cycle kite; the current design lives in docs/hunting.md |
| `docs/shopping_strategy.md` | The shop strategy reasoning, prices, purchase phases and the was/is level journey comparison |
| `docs/webui.md` | The web interface: launch modes, map rendering, movement interpolation, HUD, equipment and shop queue widgets, interactivity, the bot statistics tab, snapshot encoding, the state tracker internals, repro harnesses |
| `docs/session_journal.md` | The persistent session journal: the JSONL record of the whole run (events, samples, kills, trips, purchases), the session dump report of the web UI, the offline -session-report CLI |
| `docs/acceptance_botlog.md` | The acceptance run logs: the per-run packet trail and the monitor verdicts of the live scenarios (-acceptance, logs/acceptance) |
| `docs/pathfinding.md` | The geodata pathfinder: format, engine, deviations from the original, test UI, benchmarks, geodata visualization |
| `docs/navmesh.md` | The navmesh pathfinding: the Detour-style runtime, the Go mesh builder from the geodata, the tile format, the pack cmd, the measured comparison, the live hybrid integration behind the Navigator seam |
| `docs/navigation_analysis.md` | The measured gaps on the road to universal A to B world navigation (continue navigation work from there) |
| `docs/recast_pathfinding.md` | The recastnavigation research: what the Detour navmesh gives the geodata pathfinder, the sheet decomposition converter, the Go runtime prototype and the migration verdict |
| `docs/proxy.md` | The MITM client proxy: running it, the l2.ini recipes, the protocol the client sees, the relogin handoff, debugging proxy.log |
| `docs/protocol_description.md` | The wire protocol: packet framing, the login and game flows, per-packet layouts (originally written against l2j-lisvus; Mobius C1 is the current reference) |
| `docs/hypotheses.md` | The registry of the server assumptions that are not yet verified: the rules, the H-NNN entries with their verification plans and the open/verified status |
| `docs/devlog/` | The permanent record of the development rounds, one fragment file per round with the root cause analyses (`docs/development_log.md` is the frozen pre-split archive) - read the matching rounds before reworking movement rendering or the hunt behavior |
| `docs/progress/` | The crash-safe task handover fragments, one file per task (see the work protocol below; `docs/agent_progress.md` and `docs/agent_progress_archive.md` are the frozen pre-split archive) |
| `docs/agent_feedback_loops.md` | The audit of the feedback an autonomous agent receives (the verification loop, the live acceptance, the observability, the process memory), what is good, what to improve, what is missing |
| `docs/project_description.md` | The long term design goals, scalability ideas (packet deduplication, "eyes" bot concept, synchronized party behavior) - read it before making architectural decisions |
| `docs/quality_review_and_agent_prompts.md` | The 2026-09-07 architecture review and its improvement program (a historical snapshot - verify the state of a finding against the code before acting on it) |
| `docs/codebase_review_2026-09-20.md` | The 2026-09-20 fresh-eyes review: the verified P0/P1/P2 findings and the prioritized improvement backlog - pick the next fix round from here |
| `docs/webui_modernization_proposal.md` | The pending web UI modernization proposal (awaiting user approval; do not implement before it) |
| `docs/quest_protocol.md` | The quest subsystem protocol (Mobius C1, the M2 first-profession research): the quest machine, the packet flows, the live traces |
| `docs/band_20_25_survey.md` | The 20-25 band survey (M3 preparation): the hunting grounds reachable from the elven lands, the travel, the shopping, the learning |
| `docs/fastpath_research.md` | The fast route planning research: the measured baseline, the hierarchical route options, the 10 second budget |
| `docs/flake_ledger.md` | The searchable memory of observed test flakes: every flake gets one row (the cause, the fix, the pin that closed it) |
| `docs/ROADMAP.md` | The goal ladder: the milestones with binary live-acceptance criteria; every change must advance one |
| `docs/hunting_system_redesign.md` | The spot-anchored farming research, SUPERSEDED by hunting_cells.md (historical - do not implement from it) |
| `docs/README.md` | The docs index itself, grouped by area (stack, bot, web, history, process) |

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
`golangci-lint`, `gci`, `gofmt-spaces`, `govulncheck`). Do not begin
the actual work on an undeployed or broken stack: nearly every task
needs the live login server (2106), game server (7777) and MariaDB
(3306) to reproduce, test and validate behavior. (A pure
documentation change that touches no code needs only
`go build ./...` and the lint gate.)

```bash
bash tools/swarm_fast_deploy.sh        # ~90 s on a blank sandbox
bash tools/install_dev_tools.sh check  # preflight: exit 0 if all present
bash tools/install_dev_tools.sh        # install missing dev tools (~10 s)
```

**Run the deploy (and every other long script) in the FOREGROUND of
the shell call, with a call timeout of at least 10 minutes.** Never
start it in the background - `&`, `nohup ... &`, `setsid`, `screen`,
`tmux`, the "run in background" option of the agent tooling. A
background process does not survive the return of the tool call that
started it in the sandboxed agent shells: the deploy dies mid-run,
the next call finds a half-installed stack, and the session pays the
same "the deploy process died, restarting in the foreground with a
long timeout" rediscovery round every agent before it already paid.
The scripts are idempotent, so a dead run is cheap to repeat - but
the foreground start with the long timeout is the only correct first
attempt. The full rule lives in `docs/deployment.md` ("Foreground
execution is mandatory").

The sandbox deploy is successful only when the last lines printed
contain `STACK_READY: login :2106, game :7777, db :3306` (printed by
`tools/mobius_start.sh` invoked at the end of `swarm_fast_deploy.sh`,
not by the deploy script itself), the three ports listen
(`ss -ltn | grep -E ':(2106|7777|3306) '`, `netstat -ano` on the
Windows host) and the schema count is 75. The deeper end-to-end check
of the Linux stack is `tools/mobius_e2e.sh 45` (must print `E2E_OK`).
The script is idempotent and needs no root; the full procedure, all
checks with commands and the failure handling live in
`docs/deployment.md` (which also covers the script inventory, the
Windows host deployment and the stack tunables). If any check fails,
stop and fix the deployment first.

Verification budget: the deploy is ~100 s, the dev tools ~10 s, `go
build ./...` seconds, a warm `golangci-lint run` seconds and a cold
one under a minute; the full test suite is minutes (the hunt, pathfind
and navmesh packages dominate), and `tools/mobius_e2e.sh 45` ~50 s.
`task prepush` is the cheap pre-push answer; reach for `task verify`
and the E2E only when the change actually needs them.

## Work protocol: atomic commits and per-task fragments

- **Commit early, commit often.** Every finished logical unit of work
  (a function, a fix, a config slice, a test) is its own small atomic
  commit pushed to the remote branch immediately after it is ready -
  never let meaningful changes sit only in the working tree.
- **Track the current task and its progress inside the repository, in
  a per-task fragment file under `docs/progress/`** (the naming, the
  structure and the rules: `docs/progress/README.md`): at the start of
  a task write its full context (goal, constraints, acceptance
  criteria) into a NEW fragment named for the task, and after every
  atomic commit append a dated progress entry (what was done, what
  changed, what is next). Update and push the fragment together with
  every atomic commit. When the task completes, move the fragment to
  `docs/progress/archive/` in the last commit of the branch, so main
  only ever receives it archived and `docs/progress/` reads as the
  active set.
- **Rationale: crash-safe handover.** The agent session can die at any
  moment (connection loss, sandbox restart). A new agent must be able
  to `git pull`, read `AGENTS.md` + the task fragment and continue
  the task from the exact point where the previous agent stopped,
  without rediscovering context or losing progress. Never keep task
  state only in the conversation, only in the working tree or only
  locally.
- Before starting any task, check the board's "In progress" column,
  the issue comments and `ls docs/progress/` first: an unfinished
  fragment of a stalled task (the board issue sits in "In progress"
  with no fresh claim) is resumed - verify the described state against
  the code, then continue from the recorded next step - before any new
  work is taken.
- **Task coordination.** The claim/lease task queue is retired
  (2026-09-12, removed by the owner decision): do not take work
  from any queue - the GitHub project board and the issue claims are
  the coordination layer. Every change must still advance a
  milestone of the goal ladder in `docs/ROADMAP.md` (the progress
  of the project is the highest green milestone) - do not invent
  disconnected work.

## The shared-file clash policy (why the logs are fragment files)

- **Never append to a shared monolithic log or table from a feature
  branch.** Measured 2026-09-22 (issue #33): the retired protocol
  ("update `docs/agent_progress.md` with every atomic commit") put a
  mandatory write of ONE shared file into every branch, and 7 of the
  8 then-open PRs conflicted with plain main on that file alone -
  every landed PR forced a rebase on every other branch. The
  fragment directories end this by construction: two agents working
  different tasks write different files, and a merge of both is a
  set union, not a text clash.
- The frozen legacy logs (`docs/agent_progress.md`,
  `docs/agent_progress_archive.md`, `docs/development_log.md`) keep
  the pre-split history and stay read-only forever. A branch still
  carrying legacy edits to them (opened before the policy): on its
  rebase drop the legacy appends and carry that context in a
  `docs/progress/` fragment of its own instead - one transition
  rebase, then the branch is clash-free.
- Per-round root-cause records go to `docs/devlog/` fragment files
  (see `docs/devlog/README.md`) - never to the frozen development
  log.
- The low-frequency residue: documentation map rows (the tables in
  AGENTS.md and `docs/README.md`) are added by rounds that introduce
  a new doc. Insert the row in the section's alphabetical position;
  when a concurrent round added a neighboring row, the resolution is
  mechanical - keep BOTH rows, never drop the other round's line.
- The reviewer-facing channel is the GitHub issue (findings,
  decisions, questions for the owner) and the PR review (concrete
  code comments) - the fragment is the private working state, and
  keeping the two apart keeps both small.

## Agent skills

Operational playbooks live in `.agents/skills/` and are discovered by
the agent tooling automatically; read the matching one before working
in its area. Each skill is a short (60-100 line) step-by-step
procedure; AGENTS.md stays the source of truth for rules and facts.

The seven project playbooks (hand-maintained, never touched by the
upstream sync):

| Skill | Use when |
| --- | --- |
| `go-verify-loop` | Any Go change needs the build/vet/test/fmt/lint loop; the lint etiquette of `//nolint`. |
| `mobius-stack` | Bringing the stack up, debugging login/game/MariaDB issues, E2E runs. |
| `packet-recipe` | Adding or changing a protocol packet (any opcode work). |
| `webui-harness` | Any web UI change (HTML/CSS/JS, snapshot fields, harness failures). |
| `performance` | Touching a hot path, SoA layout, allocations, benchmarks, fleet E2E. |
| `dump-state-repro` | A bot behaves badly on a long-lived server; reproducing the exact world state for a fix. |
| `e2e-repro` | Setting up a reproducible scenario (character, position, target) for an E2E test. |

Load the matching skill before opening code in its area; the skill
points at the right files, the right tests, the right order of checks.
Skip loading when the task is unrelated (a typo fix does not need the
packet-recipe skill).

On top of the project playbooks the repository vendors the
`samber/cc-skills-golang` collection (46 `golang-*` skills, MIT) as
the Go knowledge base for this tree's Go source: before writing or
reviewing Go, load the matching skill when the touched area matches
one - `golang-testing` (tests), `golang-concurrency` (goroutines,
shared state), `golang-error-handling` (error paths),
`golang-lint`/`golang-code-style` (conventions), `golang-naming`
(identifiers), `golang-performance` (hot paths),
`golang-troubleshooting` (a bug hunt) and the rest of the family.
They answer the generic Go questions so this file does not have to.
The vendored copy travels with the repository (a fresh
environment gets it through `git clone` alone, no network access
needed). Management:

```bash
tools/install_agent_skills.sh           # (re)install the pinned commit
tools/install_agent_skills.sh latest    # update to upstream HEAD
tools/install_agent_skills.sh check     # verify against the pin, exit 1 on drift
```

The script manages the `golang-*` directories only - the seven
project playbooks are hand-maintained and never touched. The pinned
upstream commit lives in the script header (`SKILLS_COMMIT`); after
updating, refresh the pin there and commit the diff.

## Server integrity rules (non-negotiable)

The L2J Mobius C1 server is the reference implementation for this
project: its observed behavior is the spec the bot has to adapt to,
never the other way round.

- **The server comes ONLY from the official GitLab repository**
  (`https://gitlab.com/MobiusDevelopment/L2J_Mobius`, project id
  70889258, branch `master`, module directory
  `L2J_Mobius_C1_HarbingersOfWar`). Outdated copies and third party
  mirrors (GitHub mirrors like `tichopad/L2J_Mobius`, forum archives,
  old tarballs) are FORBIDDEN as a source: the mirror used before
  2026-09-08 lagged three months behind and its datapack/SQL drift
  surfaced as phantom schema and item flag differences. When GitLab
  rate limits the git protocol with intermittent 403 answers
  (Cloudflare on the upload-pack endpoints), keep retrying or fall
  back to the official repository archive API
  (`/api/v4/projects/MobiusDevelopment%2FL2J_Mobius/repository/archive.tar.gz?path=L2J_Mobius_C1_HarbingersOfWar`,
  one stable GET, same repo, same commit) - `tools/swarm_fast_deploy.sh`
  implements both channels. Never substitute a non official source.
- **Never patch the game server to change its behavior.** Gameplay
  changes (AI decisions, damage, experience, drops, movement, guard
  retaliation) are out of bounds even when they look like obvious bugs
  - if the bot needs a behavior the server does not provide, the bot
  changes, not the server.
- The only accepted server-side patches are pure logging and
  diagnostics ones (a log line that reveals what the server decided and
  why, like the `DEATHLOG` / `GUARDDMG` / `MOVEDBG` lines in the local
  checkout). They must not alter any decision the server makes.
- Vanilla quirks discovered along the way (guards that follow without
  attacking, the Lucky newbie protection below level 10) go into the
  protocol notes below and into a `docs/devlog/` fragment (the frozen
  `docs/development_log.md` never takes appends), and shape the bot
  logic instead of a server fix. The historical
  `mobius_server_delevel.patch` (guard revenge + NPC kill penalty)
  violated this rule and was removed.
- The geodata region files the bot navigates with live in
  `data/geodata` of this repository: the complete old-world pack (165
  regions, grid 16_10..26_26, l2j headerless format, sha1-verified at
  download time from the LGK-Games/Geodata mirror of the upstream
  pack; 21_19.l2j is byte identical with the region the running server
  already used) so the bot never depends on the server tree for its
  pathfinding and future hunting grounds beyond the elven lands are
  covered too; refresh the pack the same way when a deployment upgrade
  changes it.

## Repository layout

```
cmd/swarm/                     Application entry point: the bot
                               launcher (login, account, password,
                               char, web, hunt, bots, proxy), the
                               botless test modes (pathfind-test,
                               test-fight-ui, show-navmesh) and the
                               acceptance/session-report CLI; the
                               full flag list is `-h` and
                               cmd/swarm/main.go.
cmd/navmesh-build/             The offline geodata -> navmesh tile
                               converter (docs/navmesh.md).
cmd/navmesh-export/            Dumps the built tile pack (polygons,
                               links, portals) for the external
                               pathfinding harnesses (docs/navmesh.md).
cmd/navanalyze/                Scratch analysis of the navmesh tile
                               geometry and the raw geodata (the viewer
                               defect hunts).
cmd/cornerprobe/               The delevel guard walk route probe:
                               every turn click validated the way the
                               server movement validation does.
cmd/counterprobe/              Scratch probe of the merchant counter
                               geometry: the geodata classification
                               and the mesh route answer per shop.
cmd/stuckprobe/                Prints the exact RouteApproach answers
                               for reported frozen bot positions (the
                               stuck verdict probe).
cmd/benchdiff/                 Compares two `go test -bench` output
                               files into the per benchmark deltas (the
                               plumbing of `task bench:diff`).
cmd/geotest/                   The one-route geodata smoke probe (the
                               hunting zone -> guard Kendell path).
cmd/dbpos/                     Prints the character rows of the stack
                               DB through the acceptance wire client
                               (the live scenario watch helper).
cmd/gofmt-spaces/              The spaces-only gofmt the repository
                               style mandates (installed by
                               tools/install_dev_tools.sh, run through
                               `task fmt`).
internal/swarm/
  acceptance/                  The live acceptance scenarios of the
                               -acceptance flag: the monitor, the
                               verdict lines and the per-run packet
                               log (docs/acceptance_botlog.md);
                               acceptance/botlog is the packet log
                               writer itself.
  pathfind/                    Geodata path finder (docs/pathfinding.md).
  pathfind/navmesh/            The navmesh runtime: tiles, mesh, A*,
                               funnel, the approach goal and the
                               recovery ban walls (docs/navmesh.md).
  pathfind/navbuild/           The offline navmesh mesh builder from
                               the geodata (docs/navmesh.md).
  connection/                  Login flow (authentificator), game session
                               (game.go), packet framing (wire.go).
  crypt/                       Login Blowfish framing (login_crypt.go),
                               game XOR cipher (game_crypt.go), checksum.
  state/                       Live bot state tracker: character vitals,
                               world objects (SoA store), inventory,
                               events, bot registry (docs/webui.md).
  hunt/                        Auto hunt loop: engage, loot, zones, town
                               trips, shopping, deleveling
                               (docs/hunting.md); navmesh_navigator.go
                               is the mesh/grid hybrid behind the
                               Navigator seam (docs/navmesh.md).
  huntaudit/                   The spot audit: injects a spot anchor and
                               records where the server places the
                               character plus the npc population around
                               it (the resumable JSON evidence).
  gear/                        Equipment scoring and the shop strategy
                               planner (docs/hunting.md,
                               docs/shopping_strategy.md).
  npcdata/                     Generated npc/item display id to name maps
                               and shop catalogs (tools/generate_*.sh).
  webserver/                   Embedded web UI: static files, JSON
                               snapshot endpoints, the SSE event stream,
                               the pathfind test mode (docs/webui.md).
  proxy/                       The MITM client proxy (docs/proxy.md).
  session/                     The session journal aggregation behind the
                               dump report and the offline session
                               report (docs/session_journal.md).
  fleete2e/                    The 100 bot fleet E2E benchmark
                               (docs/webui.md).
  fleetprofile/                The opt-in fleet profile window (the 40
                               bot CPU and allocation capture of the
                               real Run cadence; SWARM_FLEET_PROFILE).
  memwatch/                    The once-a-minute process memory log of
                               the unattended runs.
  helpers/                     Hex+ASCII dump helpers for debugging.
  packets/
    packet/                    Binary Reader/Writer primitives (little
                               endian).
    from_auth_server/          Login server -> client packets.
    to_auth_server/            Client -> login server packets.
    from_game_server/          Game server -> client packets.
    to_game_server/            Client -> game server packets.
internal/logfmt/               The repository log convention scan (the
                               TestRepoLogConventions gate).
internal/version/              The build identity line of the bot log
                               and the state dump.
data/geodata/                  Geodata region files (X_Y.l2j), see
                               data/geodata/Readme.txt and
                               docs/pathfinding.md.
data/icons/                    Item icon pack (3134 classic client PNGs)
                               served by the web UI at /icons/<name>.png;
                               see data/icons/Readme.txt.
data/client/                   The redirected l2.ini for the C1 client
                               proxy (docs/proxy.md).
tools/                         Idempotent bash scripts that deploy and
                               run the local Mobius C1 test server stack
                               (docs/deployment.md).
docs/                          Subsystem documentation (the map above).
.agents/skills/                The seven project playbooks plus the
                               vendored golang skill collection.
```

Keep new code inside `internal/`. There is no public API yet:
everything is an internal implementation detail of the bot. The
`tools/` shell scripts are the supported way to bring the Linux
sandbox stack up; the Windows host uses the manual deployment of
`docs/deployment.md`.

## Commands

Run the application (expects the Mobius stack on port 2106 for login
and 7777 for the game, account `test1`/`test`, elven fighter `test1`).
The login address default is `127.0.0.1:2106`; the Windows deployment
binds the login server to `127.0.0.3:2106`, so that host passes
`-login 127.0.0.3:2106` (see `docs/deployment.md`). The web interface
is served on `127.0.0.1:8080` by default; pass `-web ""` to disable
it, `-web 0.0.0.0:9000` to change the address and `-bots N` to launch
N concurrent bot sessions in one process (accounts `test2`..`testN`
are auto-created on the fly):

```bash
task run:app              # or: go run ./cmd/swarm -web 127.0.0.1:8080
```

The botless launch modes (`-pathfind-test`, `-test-fight-ui`,
`-test-fight-ui-v1`) are described in docs/webui.md and
docs/pathfinding.md; the 3D navmesh viewer of `-show-navmesh`
(bare flag: every tile stitched; `-show-navmesh=21_19`: the named
tiles) is described in docs/navmesh.md.

Tests and linters (run both before considering work done). The single
fastest verification of changed code is `task lint:new`: it runs only
the linters against the lines you touched and answers in seconds on a
warm cache. The full `task lint` is the CI gate and runs against the
whole tree - use it before a push, not on every save:

```bash
task check:all            # alias of verify: build + vet + lint + test + fmt:check (the CI gate)
task verify               # the same, the canonical name (v)
task prepush              # the fast pre-push gate: build, vet, lint --new, whitespace, touched-package tests
task lint                 # golangci-lint run (full, all code)
task lint:new             # golangci-lint run --new (changed code only)
task lint:fix             # golangci-lint run --fix
task test:cover           # coverage + the per package delta vs runs/coverage-latest.txt (fails on a drop > 2 pp)
task bench:save PKG=./internal/swarm/pathfind   # commit the benchmark baseline into runs/bench-<name>.txt
task bench:diff BASE=runs/bench-pathfind.txt NEW=<fresh>  # the offline benchstat (cmd/benchdiff)
task progress             # regenerate PROGRESS.md from the live sources
task fmt                  # gofmt-spaces + whitespace normalization (spaces only)
task fmt:check            # fails on any tab left in a tracked text file
task tidy                 # go mod tidy (re-tabs go.mod: run task fmt after)
```

The committed CI workflow (`.github/workflows/ci.yml`) runs the gate
on every push to main, on every pull request and on a manual
dispatch: build, vet, the full test suite, fmt:check, the full
uncapped lint, the logfmt scan, the race slice and govulncheck - a
red commit is caught by the runner, not by the next session.
`docs/ci_workflow.yml` is the in-repository reference copy. The
logging conventions are enforced by the `internal/logfmt` scan (the
`TestRepoLogConventions` test): capital first letter (a lowercase
component tag like `login#%d:` counts), no trailing period - fix the
message, not the checker.

The full uncapped lint is zero findings and every commit keeps it
there (the discipline lives in the tree cleanliness section below).
While iterating use `task lint:new` for the changed-code verdict -
if `--new` is clean, your change is lint-clean; run the full
`golangci-lint run ./...` before a push and fix any finding in the
same commit. Shrink the debt a touched function already owes
(a `//nolint` the refactor made stale, a function now under the
limits) opportunistically, never in an unrelated change.

Benchmarks exist for hot paths (crypt, packet parsing, hex view). Use
them when touching performance sensitive code:

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

End to end test against a live local Mobius stack (builds the bot,
starts the stack, keeps the character in the world for N seconds,
sends SIGINT and checks the graceful shutdown; N defaults to 45):

```bash
tools/mobius_e2e.sh 45
```

The full deployment workflow (fast deploy for the z.ai sandbox,
bootstrap for a clean host, the script inventory) lives in
`docs/deployment.md`.

## Architecture rules (non-negotiable)

The live state tracker (`internal/swarm/state`) is the single source
of truth every other layer reads: the game session
(`internal/swarm/connection`) feeds it from the parsed packets, the
hunt loop (`internal/swarm/hunt`) decides against it, the webserver
serializes it into snapshots and the proxy patches the client replay
from it. The hunt loop is the autonomous brain (docs/hunting.md), the
gear planner scores and buys the equipment
(docs/shopping_strategy.md), the zones registry climbs the mob
ladder, the pathfinder (`internal/swarm/pathfind`) walks the geodata.
Read the matching doc before changing a layer.

The project lives or dies by its layer boundaries. A bot that grew
into a god object (the historical `state.Bot` of 2392 lines) is the
failure mode every refactor must move away from, not toward.

- **Single source of truth**: `internal/swarm/state` is the only shared
  mutable state. The packet parsers (`internal/swarm/packets/*`) stay
  pure - they fill caller-provided structs, never touch the tracker.
  The connection layer (`internal/swarm/connection`) calls the public
  `state.Bot` Apply API, never the struct fields directly. The web
  layer (`internal/swarm/webserver`) reads snapshots, never talks to
  the connection.
- **No god objects, no blob of mud.** A package that grows past
  ~1500 lines of non-test Go is a refactor candidate. The first move
  is always to split by responsibility (the `state` package already
  split `objectStore`, `eventLog`, `chatLog`, `combatFeed` out of the
  bot; new sub-systems follow the same path). A function that grows
  past `funlen` (65 lines / 45 statements) or `cyclop` (15) is split,
  not suppressed with `//nolint`.
- **Testable by construction.** Every new code path arrives with a
  unit test next to it (`*_test.go` in the same package) and, when it
  touches a hot path, a benchmark (`*_bench_test.go` with
  `-benchmem`). A change without a test is not done. Tests use the
  fake-server pattern (`connection/game_test.go`) for protocol flows,
  never a live server unless the suite is explicitly opt-in (the
  `fleete2e` suite gates on `SWARM_FLEET_E2E=1`).

## Performance and data-oriented design (the 100-bot constraint)

The stretch goal is 100 concurrent bots on one process against a live
Mobius server. The fleet benchmark (`internal/swarm/fleete2e`)
verifies that goal end-to-end. Every code change in the hot path
(state apply, packet parse, snapshot encode, hunt tick) must keep that
goal reachable.

- **SoA over AoS in the hot path.** The world object store is already
  split into hot and cold arrays (`state/objects_store.go`):
  `objectHot` (the fields the hunt tick and the scans read every frame:
  position, speed, target, dead, in-combat) streams compactly, while
  `objectCold` (display fields: name, title, template id, max HP/MP)
  sits behind it. New fields land in the half their reader visits. A
  new field read every tick that lands in `cold` defeats the split.
- **Zero allocation is the default for steady state.** The snapshot
  encoder (`state/snapshot_live.go::AppendSnapshotJSON`) walks the
  live state under the read lock and allocates nothing per event: the
  payload buffer is reusable, the view structs stay on the stack, and
  the size estimate pre-sizes the one allocation the encode pays. The
  comment on `AppendSnapshotJSON` records the 26 KB of garbage per
  event the old snapshot-copy path used to pay on a 100-npc bot.
  Match that bar when adding a new encode path.
- **Benchmarks are a gate, not a vanity.** Every `*_bench_test.go`
  reports `-benchmem`; a change to a hot path compares allocations
  before/after and does not merge if they grew without a written
  reason. The `prealloc` and `perfsprint` linters enforce the easy
  wins; `gocritic` catches the rest. A regression of more than 10% on
  a fleet benchmark (`fleete2e`) blocks the change until explained.
- **Cache-friendly access.** Iterations over the world store walk the
  hot array by index (`for i := range b.world.hot`) so the CPU
  prefetcher sees a contiguous block. Random access by object id is
  the index lookup (a `map[int32]int32`), never a linear scan. A new
  scan that breaks the dense invariant (a sparse array, a pointer
  indirection per slot) is a performance bug.
- **Hotpaths to look for.** The packet apply path (every received
  packet mutates the tracker), the snapshot encode (the SSE stream
  fires it on every version change), the hunt tick (target search,
  loot scan, movement), and the pathfinder (the hunt, pathfind and
  navmesh packages hold most of the test suite's wall time). A change
  to any of these ships with a benchmark delta.
- **What is NOT a hotpath.** The web UI handlers (one per SSE client,
  human-paced), the bot startup (one per session), the gear planner
  (one per shopping trip). Optimize the first three before touching
  these. Do not micro-optimize code that runs once per minute.

When in doubt, measure: `go test -bench=. -benchmem -count=5` on the
affected package, then `fleete2e` for the fleet-wide view.

## Mobius stack operational notes

Lessons learned while running the stack locally; relevant when
debugging connectivity issues (the full inventory lives in
`docs/deployment.md`; the `mobius-stack` skill condenses them for a
debugging session):

- **Never probe the login port by connecting to it.** The login
  server runs `FloodProtectorListener` on 2106: every accepted socket
  from one IP increments an in-memory counter that never decays while
  the connection state exists, and once the count exceeds
  `MaxConnectionPerIP` (50) the server silently drops the socket,
  which the client sees as EOF on the first read. Readiness checks
  use `ss -ltn`, never `/dev/tcp` (this is what `port_open` in
  `tools/mobius_env.sh` does). A clean bot reconnect clears the
  counter entry.
- **A restarted login server loses the game server registration**
  for seconds - the game server re-registers on port 9014 within
  seconds, but until then the server list is empty and the bot fails
  with `no available game server in the server list`. An empty list
  means "wait", not "broken"; `mobius_start.sh` waits for the
  `Updated Gameserver` line in `login.log` for this reason.
- **Stuck `account in use` states self heal.** When the bot's login
  connection closes, `LoginClient` removes the login client and the
  flood protection entry in its `finally` block, so simply retrying
  works; restarting the login server also clears it instantly.
- **SIGTERM on the game server is slow.** The JVM shutdown hook saves
  the whole world and can hold port 7777 open for tens of seconds,
  which looks like "already running". Wait for the process to
  disappear before restarting the stack.
- **The stack and the bot belong in one invocation.**
  `tools/mobius_e2e.sh` starts the stack, runs the bot and stops
  everything inside a single call - the reliable way to test end to
  end (see the subprocess section above).
- Account auto registration is enabled by the shipped login config
  (`AutoCreateAccounts = True`), so the bot simply logs in with
  `test1`/`test` and the account is created on first use.

## Client proxy (short form)

`internal/swarm/proxy` is the MITM server a real Lineage 2 C1 client
connects to (run the bot with `-proxy`). `docs/proxy.md` is the
reference (the l2.ini recipes, the relogin handoff, `proxy.log`
triage); the facts every proxy change builds on:

- The emulated login server accepts any account/password pair and
  answers a one entry server list pointing at the proxy game port;
  the emulated game server serves exactly one character (the bot
  selected in the web UI, the first session without a selection) and
  answers the character selection with the recorded `CharSelected`
  packet of the bot session patched to the live tracker state.
- After the client's `EnterWorld` the proxy replays the recorded
  server->client stream (the `Recorder` history fed by the
  `GameClient` tap) and then relays live packets both ways,
  re-encrypting on the direction specific cipher chains. The replay
  model keeps the chains independent - that is what makes packet
  rewriting safe and is the contract of the `proxy.Transformer` seam
  (identity today, the future debug spoofing hangs there). The
  replayed self packets are live-patched (paperdoll on the char
  list, position/vitals on `CharSelected` and `UserInfo`, stale self
  movement dropped except the newest one), so a reconnecting client
  spawns where the bot actually stands.
- Client packets ride the SAME outbound cipher chain as the hunt
  loop actions (`GameClient.SendRaw` encrypts under the session
  writeMu), so proxied clicks and autonomous actions interleave
  without corrupting the cipher.
- The client connection log is a dedicated file (`proxy.log`,
  `-proxy-log`): connection numbers, credentials, state transitions,
  replay stats, every client -> server packet id and close reasons.
  A failed real client login is diagnosed from that file alone.
- The live E2E of the whole path is `tools/proxy_e2e.sh` (needs the
  deployed stack; a fake C1 client walks the real protocol through
  the proxy and prints `PROXY_E2E_OK`).
- Port layout: the classic C1 exe hardcodes the auth port 2106 (the
  ini [URL] Port line is an Unreal leftover it ignores), so the
  proxy login listeners answer 2106 and 2107 on both `127.0.0.1` and
  `127.0.0.2`, proxy game `127.0.0.1:7778` + `127.0.0.2:7778`; the
  redirected client l2.ini ships in `data/client/`.

## Code conventions

Enforced by `.golangci.yml` (strict, most linters enabled):

- **Whitespace is spaces only, never tabs.** Four spaces per
  indentation step, repository wide (Go, go.mod, the tools scripts,
  the docs, the vendored skills). The formatter of record is
  `gofmt-spaces` (`cmd/gofmt-spaces`, run through `task fmt`): it
  formats exactly like gofmt but replaces every tab of the output
  with four spaces, protecting the string literals; the
  non-Go text files are widened by `tools/normalize_whitespace.sh`
  (leading tabs of scripts and data, every tab of go.mod and the
  markdown). Do **not** run the stock `gofmt`, `gofumpt`, `goimports`
  or `golangci-lint fmt` on this tree - they all re-tab it (gci is
  disabled too: its canonical form is tab-indented and it flags every
  fresh spaces-only file; import grouping stays with `gofmt-spaces`
  and review). `go mod tidy` re-tabs go.mod, so an extra `task fmt`
  belongs after it. `task fmt:check` (part of `task check:all`) fails
  on any tab left in a tracked text file.
- Line length limit is 80 characters (`lll`).
- Comments must end with a period (`godot`). Comments and identifiers
  are in English.
- Every source file starts with an SPDX copyright header
  `SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>`
  followed by `SPDX-License-Identifier: MIT`. Copy it from any existing
  file (including shell scripts in `tools/`, which use `#` comments).
- Import grouping stays with `gofmt-spaces` and review; `gci` is
  disabled (its canonical form is tab-indented, so it flags every
  fresh spaces-only file - see the formatter notes of
  `.golangci.yml`).
- Function length and complexity are limited (`funlen` 65 lines /
  45 statements, `cyclop` 15, `gocognit` 25, `maintidx` 20). The
  genuinely tangled functions (the wire parsers, the hunt loop
  decision trees) carry a single `//nolint:<linters> // short note`
  directive directly above the `func` line with the refactor reason -
  one line, comma joined linters, the note stays inside the 80
  columns. Split the function when the note stops being true.
- Constructing a struct: the zero-value-intended partial inits
  belong to the `ignore-patterns` list of `exhaustruct_v5` in
  `.golangci.yml` (the empty answers, the option aggregates, the
  internal build state - each group carries its reason); a site that
  needs a non zero start sets it explicitly. `nilnil` stays on: do
  not return `nil` error together with a `nil` value.
  A gofmt-spaces note: it is a gofmt clone, so the gofumpt extras
  (empty line trimming, the stricter idiom rules) are not enforced
  anymore - do not rely on them appearing automatically.
- Do not return `nil` error together with a `nil` value (`nilnil`); do
  not create dynamic errors with `fmt.Errorf` without wrapping
  (`err113` is planned to be enabled): prefer `errors.New` for static
  messages and `fmt.Errorf("context: %w", err)` for wrapping.
- Check every returned error (`errcheck`); tests are linted too.
- Avoid repeated string literals, extract constants (`goconst`).
- Do not shadow predeclared identifiers (`predeclared`).

## Tree cleanliness discipline (keep the gate green)

The full uncapped lint is **zero findings** since 2026-09-19 (the
~200 finding debt of the ungated parallel week is paid; see the
frozen `docs/agent_progress_archive.md`). The gate only stays cheap
if every commit keeps it at zero:

- `run.max-issues-per-linter` and `run.max-same-issues` are `0` in
  `.golangci.yml`: nothing hides behind the default caps (the caps
  previously masked the tail of the debt behind 3-identical-issue
  rounds). Expect the FULL list from every lint run.
- `task verify` (build, vet, full `lint`, test, `fmt:check`) and
  `task prepush` run before every push; the committed CI workflow
  (`.github/workflows/ci.yml`) runs the same gates plus the race
  slice, govulncheck and the coverage gate on every push to main and
  every pull request.
- Before committing files another agent may have touched in
  parallel: run `task fmt` first (the parallel commits landed tab
  formatted ten times), then `golangci-lint run ./...` - a red
  result is fixed in the same commit, never passed on.
- Suppression policy: a `//nolint` directive needs an inline reason,
  stays on one line of at most 80 columns, and sits directly above
  the line or declaration it silences (a wrapped prose continuation
  after the directive is fine, a second stacked directive is not -
  merge into `//nolint:a,b // reason`). `nolintlint` reports any
  directive that stopped matching, so stale suppressions surface on
  the next run and must be removed in the same pass.
- Linter relief for whole paths (the analysis drivers, the C1 water
  zone data table, the test scenario scripts) lives in the
  `exclusions` rules of `.golangci.yml` with the documented reason,
  not in scattered directives.
- Dead code (`unused`) is deleted, not suppressed: an unused
  constant, field, function or test helper is a removal commit.
- The runtime artifacts of `tools/coverage_delta.sh`
  (`runs/.cover-run.log`, `runs/.coverage-current.txt`) are
  gitignored - never commit them (their `go test` output carries
  tabs and turns the whitespace gate red tree-wide). The committed
  baseline is `runs/coverage-latest.txt` only.
- Wall time budgets in tests ride `raceDetectorBudget` (the pathfind
  build tag pair: 10 under `-race`, 1 plain). The race detector
  inflates the hot loop 6-10x; a fixed budget under `-race`
  reports instrumentation cost as a failure (the flake ledger row
  of 2026-09-19). Keep the plain figure at the production number.

## Logging conventions

- Each log message starts with a capital letter.
- No periods at the end of log lines.
- Error messages start with `Error`.
- Use `Println`-style output; only use `\n` with `Printf`-style
  multi-line output.
- The bot log self-identifies: the line right after "Starting swarm
  bot" is `Build: <identity>` from `internal/version` (the same line
  the state dump carries) - every log tail pairs with the exact code
  state.

## Testing conventions

- Tests live in the same package as the code (`*_test.go` next to
  sources).
- Every packet parser/serializer has both a unit test and a benchmark
  test where performance matters (`*_bench_test.go`,
  `init_bench_v2_test.go` pattern).
- Protocol flows are covered by in-process fake servers (see
  `connection/game_test.go` for the scripted game session test).
- The live fleet E2E (`internal/swarm/fleete2e`) gates on
  `SWARM_FLEET_E2E=1` so the regular test suite never pays its 10-
  minute 100-session setup. Run it only when changing a hot path that
  the fleet benchmark covers.
- Benchmarks report allocations (`-benchmem`) because a core
  requirement is memory-friendly parsing for hundreds of concurrent
  connections. When changing packet code, compare allocations
  before/after. The `prealloc` and `perfsprint` linters enforce the
  easy wins.
- Use `testify` (`require`/`assert`) with `testifylint`-clean style.
- Tests are deterministic: time windows use injected clocks (see
  `go-verify-loop` skill). A flaky test that depends on real wall
  clock timing is a bug in the test, not a tolerance to widen.
- Dump state for reproduction: a bot that misbehaves on a long-lived
  server carries a compact world snapshot (the `dump-state-repro`
  skill). Use it to reproduce the exact world state in a unit test
  instead of replaying the live session by hand.

## Protocol notes (Mobius C1)

`docs/protocol_description.md` plus the Mobius Java sources are the
source of truth for packet formats (`L2J_Mobius_C1_HarbingersOfWar/java`).
The facts every packet change builds on:

- Game protocol version 419 (`AllowedProtocolRevisions` in
  `dist/game/config/Server.ini`).
- Packet framing both directions: 2 byte size header (little endian,
  includes itself), then the payload `[opcode: 1][body]`. Mobius reads
  and writes the header little endian (see
  `commons/network/pool/ResourcePool.java`).
- Login protocol: payload is padded to 8 byte alignment and ends with
  a 4 byte little endian XOR checksum of all preceding 4 byte words;
  Blowfish with the static 21 byte key (`crypt.MobiusAuthKey()`, see
  `loginserver/LoginClient` `NewCrypt`). The `Init` packet is
  unencrypted.
- Game protocol: after the unencrypted `ProtocolVersion` <-> `KeyPacket`
  exchange, every payload is XOR encrypted with the stateful 8 byte
  cipher (`crypt.GameCrypt`, mirrors
  `gameserver/network/Encryption.java`): running XOR chain with
  `key[i&7]`, rolling offset kept in `key[0..3]` (little endian,
  advanced by the payload size after each packet). No checksum, no
  padding, no Blowfish on the game connection.
- Integers in packet bodies are little endian. Strings are
  null-terminated UTF-16LE (see
  `commons/network/packet/ReadablePacket.readString`).
- Login flow: `Init` -> `RequestAuthLogin` (fixed 14 byte zero padded
  account/password fields) -> `LoginOk` -> `RequestServerList` ->
  `ServerList` -> `RequestServerLogin` -> `PlayOk`.
- Game flow: `ProtocolVersion` -> `KeyPacket` -> `AuthLogin` (login
  string + session keys in order playOk2, playOk1, loginOk1, loginOk2)
  -> `CharSelectionInfo` -> (`CharacterCreate` -> `CharCreateOk` ->
  updated list) -> `CharacterSelect` -> `CharSelected` -> `EnterWorld`
  -> world packets; keep alive with `RequestNetPing` (0xA8) and reply
  `NetPing` (0xEC); leave with `Logout` (0x09).
- After every self `TeleportToLocation` (0x38) the bot answers with
  the client `Appearing` (0x30) packet: the server holds the character
  in the teleporting state (`Creature._isTeleporting`) until
  `Appearing` arrives and silently ignores every move request meanwhile
  (the character AI checks `isMovementDisabled`). A village revive
  without the confirmation left the bot permanently stuck - the
  official client sends Appearing when the teleport screen closes.
- Death penalty and deleveling (Mobius C1): the doDie experience
  penalty branch runs for every killer - verified live: a guard (NPC)
  death at level 10 paid the penalty. It removes `percentLost` of the
  current level span (`data/stats/players/experienceLoss.xml`, ~9% at
  low levels, capped at 10% of the span, `Delevel` config enabled,
  karma multiplies it), and the Lucky newbie skill (id 194, granted
  with character creation) absorbs it entirely while level <= 9
  (`Player.isLucky`). The level gap rules of the drop calculation
  (`NpcTemplate.calculateGroupDrops`): item drops slide from 100% at
  mob level + 5 to 10% at + 10 and beyond, adena from 100% at + 8 to
  10% at + 15; experience and SP stop at + 11
  (`MonsterExpMaxLevelDifference`). The elven fields mobs are levels
  1-5, so a level 11 character gets 10% item drops - the deleveling
  exists to fix that (docs/hunting.md).
- Guard retaliation in the delevel cycle (live validated): an archer
  guard shoots at everything within its 850+ unit bow range with no
  karma gate (`thinkAttack` -> `doAttack`), while a melee guard only
  follows the provoker (`Guard.addDamage` -> `startFollow`) and the
  chase dies in the `checkTarget` gate (`Player.isAutoAttackable`
  returns karma > 0 for guards) - the delevel provokes the archer
  sentinels (Kendell, Starden) only, in melee.
- The elven fighter creation values: race 1 (ELF), classId 18
  (ELVEN_FIGHTER), see
  `gameserver/entity/actor/enums/player/PlayerClass`.
- Equip semantics of `UseItem` (0x14, `Player.useEquippableItem` /
  `Inventory.equipItem`): a right hand weapon, chest, neck, head,
  gloves, feet or back item replaces the occupant of its slot
  directly; the paperdoll listener unequips the old item. The pair
  families fill the first EMPTY ear/finger slot (left first, right
  second) and replace the LEFT slot blindly when both are occupied -
  swapping the better jewel needs the explicit unequip of the weaker
  piece first (see `gear.NextUpgrade`). A two hand weapon (`lrhand`:
  bows, poles) unequips the left hand shield on equip, a shield
  unequips a two hand weapon, a one-piece armor (`onepiece`) occupies
  the chest slot, blocks the legs slot and unequips on a legs equip -
  the planner guards all four cases.
- Paperdoll knowledge: the inventory packets carry the template
  bodypart mask (an earring is always 0x6) and cannot tell which ear
  or finger slot an equipped jewel occupies; the UserInfo (0x04)
  paperdoll object id block is the only source for that (slot order:
  underwear, right ear, left ear, neck, right finger, left finger,
  head, right hand, left hand, gloves, chest, legs, feet, back and a
  C1 duplicate right hand). The server broadcasts UserInfo after every
  equip and unequip.
- Shop protocol: `RequestBuyItem` (0x1F) `[listId: 4][count: 4]`
  entries of `[itemId: 4][count: 4]` requires the selected merchant of
  the list within the 250 unit interaction distance; the server prices
  every entry itself (buylist product price or the item reference
  price) with the town tax: Elven Village 15 percent over reference
  while no castle owns it (`MerchantPriceConfig.xml`), sell is always
  reference/2. Buying and selling share the transaction flood
  protector (10 seconds), so buy and sell requests pace like the sell
  batches. The buylist ids are the file names of `data/buylists/*.xml`;
  the merchant of a list is the npc of its `<npcs>` block (join to the
  packet template id through `stats/npcs/CT0_to_C4_ids.txt`, 30147
  Unoren -> 7147 etc).
- Logout semantics: `RequestLogout` is refused while the character
  holds an attack stance (`Player.canLogout` checks the
  `AttackStanceTaskManager`, `YOU_CANNOT_LOGOUT_WHILE_IN_COMBAT`); the
  stance lapses 15 seconds after the last combat event
  (`AttackStanceTaskManager.COMBAT_TIME`). A socket close of a
  character in combat stores it on the same 15 second delay
  (`Disconnection.onDisconnection`), so an emergency logout is one
  escape walk + the logout packet + the socket close either way.
- The Mobius `NpcTemplate` defaults `isAggressive` to TRUE: the mobs
  that omit the attribute (the Kaboo Orc Fighter) attack players on
  sight - the generated npc data mirrors this since the clan round.
- The clan assist of the Mobius `AttackableAI`: an attacked npc calls
  every nearby attackable within its `clanHelpRange` whose clans
  intersect its own (the ALL clan matches everything, the check runs
  against the ATTACKED mob's clan set - see `NpcTemplate.isClan`), a
  600 unit z distance blocks the call and npcs spawned within the last
  7 seconds ignore it.

When adding a new packet: implement the struct in the correct direction
package (`from_*` / `to_*`), add parsing/serialization via
`packet.Reader`/`packet.Writer`, cover it with unit tests and a
benchmark, and document the layout in `docs/protocol_description.md`
with a link to the reference Mobius Java class.

## Git conventions

- `main` is the default branch. A larger task gets a feature branch;
  a small round may land on `main` directly (the owner's current
  working mode).
- Commit messages are short, imperative and lowercase-ish, e.g. `fix
  line size`, `add cyclop check`, `make init packet parsing more memory
  friendly`.
- Do not push build artifacts (`*.out`, `*out`, binaries are
  gitignored) or `.env`.
- **Rebase before every push.** Several agent sessions push to the
  same branch concurrently, so a push can be rejected as
  non-fast-forward at any moment. The push procedure is: `git fetch
  origin`, `git rebase origin/<branch>`, `task prepush`, then `git
  push`. Never merge
  remote commits into the local branch (no "Merge branch" commits - the
  history stays linear) and never force-push (it would destroy the
  parallel sessions' work). On a rebase conflict resolve both sides'
  behavior honestly, re-verify (at least `go build ./...`, the affected
  package tests and `golangci-lint run`), commit the resolution with
  `git rebase --continue` and push again; if the push is rejected
  again, repeat from the fetch.
- **Always commit as melg8.** The canonical committer identity of this
  repository is `user.name = melg8`, `user.email =
  public.melg8@gmail.com`. Check both with `git config user.name` and
  `git config user.email` before the first commit of a session; if they
  differ, set them for the repository with `git config user.name melg8`
  and `git config user.email public.melg8@gmail.com` (repo-local, not
  `--global`). Never commit as another identity and never amend the
  identity of commits that are already pushed.
