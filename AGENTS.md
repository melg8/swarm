<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# AGENTS.md

Guidance for AI coding agents (and humans) working on this repository.
This file holds the RULES and the load-bearing FACTS only; the
implementation-level detail of every subsystem lives in `docs/` (see
the documentation map below) and is read on demand, not upfront.

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
| `docs/hunting.md` | The autonomous hunt: hunt loop, combat safety, blind engage recovery, multi-zone hunting, auto equipment, shop strategy execution, town trips, deleveling, live-validated facts |
| `docs/shopping_strategy.md` | The shop strategy reasoning, prices, purchase phases and the was/is level journey comparison |
| `docs/webui.md` | The web interface: launch modes, map rendering, movement interpolation, HUD, equipment and shop queue widgets, interactivity, snapshot encoding, the state tracker internals, repro harnesses |
| `docs/pathfinding.md` | The geodata pathfinder: format, engine, deviations from the original, test UI, benchmarks, geodata visualization |
| `docs/navigation_analysis.md` | The measured gaps on the road to universal A to B world navigation (continue navigation work from there) |
| `docs/proxy.md` | The MITM client proxy: running it, the l2.ini recipes, the protocol the client sees, the relogin handoff, debugging proxy.log |
| `docs/protocol_description.md` | The wire protocol: packet framing, the login and game flows, per-packet layouts (originally written against l2j-lisvus; Mobius C1 is the current reference) |
| `docs/development_log.md` | The permanent record of the development rounds with root cause analyses - read it before reworking movement rendering or the hunt behavior |
| `docs/agent_progress.md` | The crash-safe task handover file (see the work protocol below) |
| `docs/project_description.md` | The long term design goals, scalability ideas (packet deduplication, "eyes" bot concept, synchronized party behavior) - read it before making architectural decisions |
| `docs/quality_review_and_agent_prompts.md` | The 2026-09-07 architecture review and its improvement program (a historical snapshot - verify the state of a finding against the code before acting on it) |
| `docs/webui_modernization_proposal.md` | The pending web UI modernization proposal (awaiting user approval; do not implement before it) |

## Mandatory first step of every task: deploy and verify the environment

Any task in this repository - a bug fix, a feature, a refactor, a test
run or an investigation - starts by deploying the repository
dependencies with `tools/swarm_fast_deploy.sh` and verifying that it
brought the environment up successfully. Do not begin the actual work
on an undeployed or broken stack: nearly every task needs the live
login server (2106), game server (7777) and MariaDB (3306) to
reproduce, test and validate behavior. (A pure documentation change
that touches no code needs only `go build ./...` and the lint gate.)

```bash
bash tools/swarm_fast_deploy.sh
```

The deploy is successful only when the script printed
`STACK_READY: login :2106, game :7777, db :3306`, the three ports
listen (`ss -ltn | grep -E ':(2106|7777|3306) '`) and the schema count
is 75. The deeper end-to-end check is `tools/mobius_e2e.sh 45` (must
print `E2E_OK`). The script is idempotent and needs no root; the full
procedure, all checks with commands and the failure handling live in
`docs/deployment.md` (which also covers the script inventory, the
Windows host deployment and the stack tunables). If any check fails,
stop and fix the deployment first.

## Work protocol: atomic commits and progress tracking in the repo

- **Commit early, commit often.** Every finished logical unit of work
  (a function, a fix, a config slice, a test) is its own small atomic
  commit pushed to the remote branch immediately after it is ready -
  never let meaningful changes sit only in the working tree.
- **Track the current task and its progress inside the repository**, in
  `docs/agent_progress.md`: at the start of a task write its full
  context (goal, constraints, acceptance criteria), and after every
  atomic commit append a dated progress entry (what was done, what
  changed, what is next). Update and push that file together with every
  atomic commit. Keep the file lean: finished tasks move to
  `docs/agent_progress_archive.md` so a new session reads only the
  active context.
- **Rationale: crash-safe handover.** The agent session can die at any
  moment (connection loss, sandbox restart). A new agent must be able
  to `git pull`, read `AGENTS.md` + `docs/agent_progress.md` and
  continue the task from the exact point where the previous agent
  stopped, without rediscovering context or losing progress. Never keep
  task state only in the conversation, only in the working tree or only
  locally.
- Before starting any task, read `docs/agent_progress.md` first: if it
  contains an unfinished task entry, resume that task (verify the
  described state against the code, then continue from the recorded
  "next" step) before taking a new one.

## Agent skills

Operational playbooks live in `.agents/skills/` and are discovered by
the agent tooling automatically; read the matching one before working
in its area: `go-verify-loop` (the verification loop and the
lint/nolint etiquette), `webui-harness` (web UI changes and the repro
harnesses), `packet-recipe` (adding or debugging a protocol packet),
`mobius-stack` (the live server stack and its pitfalls). AGENTS.md
stays the source of truth for rules and facts; the skills are the
step-by-step procedures.

On top of the four project playbooks the repository vendors the
`samber/cc-skills-golang` collection (46 `golang-*` skills, MIT, e.g.
`golang-testing`, `golang-concurrency`, `golang-error-handling`,
`golang-lint`, `golang-troubleshooting`) as the general Go knowledge
base - load the matching one for generic Go questions. The vendored
copy travels with the repository (a fresh environment gets it through
`git clone` alone, no network access needed). Management:

```bash
tools/install_agent_skills.sh           # (re)install the pinned commit
tools/install_agent_skills.sh latest    # update to upstream HEAD
tools/install_agent_skills.sh check     # verify against the pin, exit 1 on drift
```

The script manages the `golang-*` directories only - the four project
playbooks are hand-maintained and never touched. The pinned upstream
commit lives in the script header (`SKILLS_COMMIT`); after updating,
refresh the pin there and commit the diff.

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
  protocol notes below and into `docs/development_log.md`, and shape
  the bot logic instead of a server fix. The historical
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

## Tech stack

- Go 1.23.2, module path `github.com/melg8/swarm`.
- `golang.org/x/crypto` (Blowfish), `golang.org/x/text` (UTF-16
  handling).
- `testify` for assertions, `sergi/go-diff` for test helpers.
- `Taskfile.yml` (go-task) wraps all routine commands. Prefer `task`
  aliases.
- `golangci-lint` with a strict linter set configured in
  `.golangci.yml`.

## Repository layout

```
cmd/swarm/                     Application entry point (flags: login,
                               account, password, char, web, hunt, bots,
                               proxy, pathfind-test, test-fight-ui,
                               test-fight-ui-v1, geodata, max-passable).
internal/swarm/
  pathfind/                    Geodata path finder (docs/pathfinding.md).
  connection/                  Login flow (authentificator), game session
                               (game.go), packet framing (wire.go).
  crypt/                       Login Blowfish framing (login_crypt.go),
                               game XOR cipher (game_crypt.go), checksum.
  state/                       Live bot state tracker: character vitals,
                               world objects (SoA store), inventory,
                               events, bot registry (docs/webui.md).
  hunt/                        Auto hunt loop: engage, loot, zones, town
                               trips, shopping, deleveling
                               (docs/hunting.md).
  gear/                        Equipment scoring and the shop strategy
                               planner (docs/hunting.md,
                               docs/shopping_strategy.md).
  npcdata/                     Generated npc/item display id to name maps
                               and shop catalogs (tools/generate_*.sh).
  webserver/                   Embedded web UI: static files, JSON
                               snapshot endpoints, the SSE event stream,
                               the pathfind test mode (docs/webui.md).
  proxy/                       The MITM client proxy (docs/proxy.md).
  fleete2e/                    The 100 bot fleet E2E benchmark
                               (docs/webui.md).
  helpers/                     Hex+ASCII dump helpers for debugging.
  packets/
    packet/                    Binary Reader/Writer primitives (little
                               endian).
    from_auth_server/          Login server -> client packets.
    to_auth_server/            Client -> login server packets.
    from_game_server/          Game server -> client packets.
    to_game_server/            Client -> game server packets.
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
.agents/skills/                The four project playbooks plus the
                               vendored golang skill collection.
```

Keep new code inside `internal/`. There is no public API yet:
everything is an internal implementation detail of the bot. The
`tools/` shell scripts are the only supported way to bring the test
server stack up.

## Commands

Run the application (expects the Mobius stack at `127.0.0.1:2106` and
`127.0.0.1:7777`, account `test1`/`test`, elven fighter `test1`). The
web interface is served on `127.0.0.1:8080` by default; pass `-web ""`
to disable it, `-web 0.0.0.0:9000` to change the address and `-bots N`
to launch N concurrent bot sessions in one process (accounts
`test2`..`testN` are auto-created on the fly):

```bash
task run:app              # or: go run ./cmd/swarm -web 127.0.0.1:8080
```

The bot less launch modes (`-pathfind-test`, `-test-fight-ui`,
`-test-fight-ui-v1`) are described in docs/webui.md and
docs/pathfinding.md.

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

## Architecture in one paragraph

The live state tracker (`internal/swarm/state`) is the single source
of truth every other layer reads: the game session
(`internal/swarm/connection`) feeds it from the parsed packets, the
hunt loop (`internal/swarm/hunt`) decides against it, the webserver
serializes it into snapshots and the proxy patches the client replay
from it. The hunt loop is the autonomous brain (docs/hunting.md), the
gear planner scores and buys the equipment (docs/shopping_strategy.md),
the zones registry climbs the mob ladder, the pathfinder
(`internal/swarm/pathfind`) walks the geodata. Read the matching doc
before changing a layer; keep the layer boundaries (parsers stay pure,
the tracker stays the only shared mutable state, the web layer never
talks to the connection directly).

## Mobius stack pitfalls (the two that bite hardest)

- **Never probe the login port by connecting to it** (the flood
  protector silently drops the socket after 50 connections from one
  IP; readiness checks use `ss -ltn`, never `/dev/tcp`).
- **A restarted login server loses the game server registration** for
  seconds - an empty server list means "wait", not "broken".

The rest of the operational notes (stuck `account in use`, slow
SIGTERM, sandbox shell kills, stack logs and tunables) live in
`docs/deployment.md`; the `mobius-stack` skill condenses them for a
debugging session.

## Code conventions

Enforced by `.golangci-lint` config (strict, most linters enabled):

- Line length limit is 80 characters (`lll`).
- Comments must end with a period (`godot`). Comments and identifiers
  are in English.
- Every source file starts with an SPDX copyright header
  `SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>`
  followed by `SPDX-License-Identifier: MIT`. Copy it from any existing
  file (including shell scripts in `tools/`, which use `#` comments).
- Format with `gofmt`/`gofumpt`; imports grouped by `gci`/`goimports`.
- Function length and cyclomatic complexity are limited (`funlen`,
  `cyclop`, `gocyclo`). Split long functions instead of disabling
  linters.
- Initialize all struct fields when constructing (`exhaustruct`);
  prefer `NewXxx()` constructors for parsed packet structs.
- Do not return `nil` error together with a `nil` value (`nilnil`); do
  not create dynamic errors with `fmt.Errorf` without wrapping
  (`err113` is planned to be enabled): prefer `errors.New` for static
  messages and `fmt.Errorf("context: %w", err)` for wrapping.
- Check every returned error (`errcheck`); tests are linted too.
- Avoid repeated string literals, extract constants (`goconst`).
- Do not shadow predeclared identifiers (`predeclared`).

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
- Benchmarks report allocations (`-benchmem`) because a core
  requirement is memory-friendly parsing for hundreds of concurrent
  connections. When changing packet code, compare allocations
  before/after.
- Use `testify` (`require`/`assert`) with `testifylint`-clean style.

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

- `main` is the default branch. Work in feature branches.
- Commit messages are short, imperative and lowercase-ish, e.g. `fix
  line size`, `add cyclop check`, `make init packet parsing more memory
  friendly`.
- Do not push build artifacts (`*.out`, `*out`, binaries are
  gitignored) or `.env`.
- **Rebase before every push.** Several agent sessions push to the
  same branch concurrently, so a push can be rejected as
  non-fast-forward at any moment. The push procedure is: `git fetch
  origin`, `git rebase origin/<branch>`, then `git push`. Never merge
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
