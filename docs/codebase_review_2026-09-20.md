<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Fresh-eyes codebase review (2026-09-20)

A full-tree evaluation performed with the vendored skills as the review
instruments, the way AGENTS.md prescribes. Four parallel review passes were
armed by the skills' own audit modes - golang-troubleshooting
(code-review-flags, common-go-bugs, safety references), golang-concurrency
(the five-checkpoint audit), golang-error-handling, golang-testing,
golang-security and go-verify-loop - over (1) nil/resources in connection,
crypt, packets, proxy; (2) error handling and slice/map safety in hunt, gear,
session, huntaudit, npcdata; (3) concurrency in state, webserver, connection,
proxy plus every goroutine spawn site; (4) testing, docs onboarding and the
CI/deps posture. Every finding below was re-verified against the tree at
1f9408c by the consolidation pass; findings a linter already covers are
excluded (the strict gate sits at 0 findings).

## Verified snapshot (2026-09-20, 1f9408c)

- `task verify` green: build, vet, uncapped lint 0 issues, 28 packages ok,
  fmt:check clean; `go test -race` green on state, webserver, connection.
- govulncheck: 0 reachable vulnerabilities (x/crypto imports blowfish only).
- No secrets in the tree; SPDX headers on 520 of 529 Go files.
- Coverage (runs/coverage-latest.txt): core packages 70-94 percent; thin:
  cmd/swarm 11.4, huntaudit 24.5, acceptance 33.0, to_game_server 65.3.
- Suite wall time this run: pathfind 160 s, hunt 92 s, navbuild 76 s.
- GitHub Actions: no workflow has ever run on this repo (only the dynamic
  Dependabot dependency-graph job) - docs/ci_workflow.yml was never placed.

## P0 - fix first (correctness, data loss, process blockers)

1. `internal/swarm/session/journal.go` (gzipFile, ~line 572): a failed
   `io.Copy`/`sink.Close`/`target.Close` is only logged, then
   `os.Remove(path)` runs unconditionally - disk-full during rotation
   destroys the original journal segment, contradicting the function's own
   comment ("a gzip failure keeps the original on disk"). Fix: track a
   failure flag; skip the remove when any step failed. Add a test that
   forces a copy failure and asserts the original survives.
2. `internal/swarm/state` (skills.go `ensureSkillQueueLocked` via
   snapshot_live.go `appendLiveSkillPlanJSON`; the same class in
   inventory.go `demandedBooksLocked`): the snapshot encoder runs under
   `mu.RLock()` and the lazy caches WRITE `b.skillQueue`, `b.bookKeep` and
   their revision keys - two concurrent readers (two SSE clients, or an SSE
   stream plus an API poll, after any SetSkills revision bump) race on the
   single source of truth. The race detector stays silent only because no
   test drives two concurrent encoders. Fix: rebuild the caches at the
   mutation points under the write lock, or drop the cache and build into a
   local slice per encode. Add a test with two concurrent
   AppendSnapshotJSON callers after a revision bump (run under -race).
3. Session lifecycle edges (connection/main.go): `runBot` never closes
   `gameConn` when `NewGameClient`/`Authenticate`/`EnsureCharacter`/
   `EnterWorld` fail (main.go ~452), so `runBotForever` bleeds one fd per
   retry around the clock; the whole login flow has no read deadlines
   (authentificator.go ~69-108) and the pre-world char-list/creation reads
   are unbounded (game.go ~933, ~988), so a stalled or half-open login
   parks the bot silently forever - the exact frozen state the in-world
   silence watchdog was built against. Fix: close on every pre-Run error
   path; set bounded read deadlines on the login and pre-world reads.
4. Continuous integration is designed but not active: docs/ci_workflow.yml
   has never been placed into .github/workflows (verified via the GitHub
   API: zero workflow runs). The committed copy also lags the local gate:
   its lint step runs `golangci-lint run --new` behind a stale
   "~200 findings" comment (the debt was paid 2026-09-19), has no
   `permissions:` block, and its race slice covers only connection and
   pathfind. Fix: refresh the workflow to the full uncapped lint, add
   `permissions: contents: read`, then activate it (the push token needs
   the workflow scope, else the owner places the file - the step is
   documented in four places already).
5. docs/agent_progress.md defeats its own handover protocol: 1718 lines
   carrying seven `## Active task` headings ordered oldest-first with no
   finished/unfinished marker, while AGENTS.md tells a new agent to "resume
   the newest unfinished entry". Fix (its own round): keep exactly one
   status-marked active task at the top, move the closed 2026-09-19/20
   rounds into agent_progress_archive.md, then restore the file to the
   lean "active + most recent" shape its header promises.

## P1 - next rounds

6. `internal/swarm/hunt/cell.go` (~258): `cellAggroMass` feeds the spawn
   xml id to `npcdata.NPCIsAggressive`, whose templateID guard returns
   false for every such id - the static 0.35 aggressive-share discount in
   `cellSafety` always computes 1.0 and the picker underweights aggressive
   grounds. Fix: resolve through `npcdata.NPCWireTemplateID` first, as the
   sibling lookups already do.
7. `internal/swarm/hunt/quest_trip.go` (~915): the `!ok` branch of
   `NearestNpcByTemplates` attacks the zero-value struct - a momentarily
   dead kill ground sends `WalkTo(0, 0, z)` every attack period and grinds
   refused clicks against the flood protector. Fix: continue (pace and
   rescan) when the scan comes up empty.
8. `internal/swarm/session/journal.go` (~274-297): a failed `os.OpenFile`
   after `j.file.Close()` strands the writer on the closed file forever -
   every later record encodes into the void and the report diverges from
   the file. Fix: open the next segment before closing the old one, and
   re-arm a reopen attempt from the flush path.
9. Shutdown tail: SSE writes have no write deadline (webserver/server.go
   ~89 - a half-open client blocks the handler forever), proxy Shutdown
   abandons the connected client goroutines (proxy/server.go ~556), and
   the hunt loop and journal sampler are spawned unjoined (main.go ~527,
   ~598) so their final records race the journal close. Fix: per-write
   deadlines, a connection registry closed in Shutdown, and a WaitGroup
   joined before the journal close.
10. Pathfind wall time: pathfind plus navbuild are ~4 minutes of the suite
    with no `-short` or build-tag split and a fresh geodata Engine parsed
    per test (19 `townTestEngine` call sites). Fix direction: gate the
    real-geodata helpers on `testing.Short()` (mirroring the race budget
    tag pair) and share one parsed engine per package; keep the full pack
    run as the CI/default gate.
11. Hot-path economics at the 100-bot goal: every outbound packet
    allocates a fresh writer and sends header and payload as two writes
    (game.go ~856, ~385, wire.go ~51-68), and the hot parse/relay paths
    log per packet through the global log mutex (game_apply.go ~93, ~127,
    ~57; proxy/game.go ~688). Fix: one reusable writer per client plus a
    single writev-style send; demote or rate-limit the per-packet lines.
12. Dependency posture is fully manual: no dependabot/renovate config and
    govulncheck wired nowhere (verified 0 reachable today - keep it that
    way). Fix: a minimal gomod dependabot config plus a govulncheck step
    in the workflow or `task verify`.
13. Docs drift the audit round missed: the Go-version story is told three
    ways (AGENTS.md tech stack says "auto-downloads 1.24" while go.mod
    pins `toolchain go1.26.8` and README says "Go 1.24"); the
    go-verify-loop skill self-contradicts (40 s vs 123 s suite) and cites
    .github/workflows/ci.yml without the manual-copy caveat; the AGENTS.md
    repository layout lists 2 of the 9 cmd/ binaries; docs/README.md
    misses rows for ci_workflow.yml and agent_progress_archive.md.
14. Test hygiene: multi-second real `time.Sleep` clock aging in hunt tests
    (loop_test.go ~1565, loop_los_test.go ~273, ~325) against the
    injected-clock rule; the fake game server leaks its listener
    goroutine (connection/game_test.go ~31); goleak is absent from the
    networked packages; a 200 ms sleep gates a negative assertion in
    acceptance/manager_test.go ~288.

## P2 - opportunistic

15. proxy accept loop exits permanently on a transient accept error
    (server.go ~469); huntaudit leaks the game socket on handshake failure
    (audit.go ~341, ~391); the session reader skips malformed journal
    lines without a counter (reader.go ~90).
16. Fleet envelope: the recorder caps history per bot (32 MB) with no
    process-wide budget across 100 bots (recorder.go ~48); navmesh
    prefetchAhead spawns unbounded (mesh.go ~375); acceptance scenarios
    detach from the process context (manager.go ~472); the stats sampler
    stop is unjoined (server.go ~342).
17. Coverage thin spots (cmd/swarm 11.4, huntaudit 24.5, acceptance 33.0,
    to_game_server 65.3, cmd/navmesh-build untested) are locked in by the
    drop-only delta gate; t.Parallel adoption is ~1 percent. Pick up
    opportunistically when touching those packages.

## What the fresh pass found genuinely good

- Copy-at-the-boundary discipline in the parsers and the proxy is
  exemplary - the "parser output aliases the reused buffer" bug class is
  structurally absent, and every wire-supplied count is bounds-checked
  before it can drive an allocation.
- The connection session lifecycle (reader join, drain, silence watchdog),
  the recorder's poison-on-overflow delivery and the journal's
  drop-don't-block counters are engineered the way the concurrency skill
  prescribes; the cipher contract between the hunt loop and the proxy
  holds exactly as documented.
- Hunt recovery ladders escalate with explicit budgets and honest abort
  reasons everywhere; the quest engine's `%w` wrapping is exemplary.
- The docs graph is fully connected (no dead links from AGENTS.md, the
  index or the map), the flake ledger is a live honest process with all
  pins still in the tree, and the coverage delta gate makes coverage
  regression a hard local failure.

## Recommended round order

Rounds 1-2: P0 items 1 and 2 (small diffs, data correctness, each with a
focused test). Round 3: P0 item 3 (session edges). Round 4: P0 item 5
(the agent_progress.md restructure, isolated to avoid handover conflicts).
Round 5: P0 item 4 plus P1 item 12 (the CI/deps batch). Then P1 6-8 (the
behavior bugs), 9-11, and the docs drift batch 13-14; P2 opportunistically.
Each round: atomic commit, `task prepush`, full `task verify` before the
push, rebase first - several agents share this branch.
