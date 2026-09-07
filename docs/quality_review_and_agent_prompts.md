<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Architecture and code quality review with agent improvement prompts

Date: 2026-09-07. Scope: the whole repository on `mobius-c1-client-1`
(post-MVP state, rounds 1-30 of `docs/development_log.md`).

Purpose: the MVP phase is over. The project now has to grow into the
system `docs/project_description.md` describes (9-100 concurrent bots,
party synchronization, 24/7 operation), and most future work will be done
by autonomous AI agents instead of a human reading every diff. This
document

1. records a critical review of the current architecture and code
   quality (part 1-3),
2. turns the findings into an ordered improvement program (part 4) and
3. provides copy-paste-ready prompts, one per agent session (part 5).

Method: three independent exploration passes over the packages
(state+hunt, connection+packets+crypt, webserver+pathfind+web), followed
by manual verification of every load-bearing claim in the sources (the
two data races, the lock usage, the Taskfile, the registry API, the
one-bot shape of `cmd/swarm/main.go`). File:line references refer to the
state of the branch at review time and may drift.

---

## Part 1. Where the project stands

### What is genuinely good (keep it)

- **Documentation culture.** `AGENTS.md` is dense, accurate and
  agent-oriented; `docs/development_log.md` records root cause analyses
  with references into the Mobius sources; `docs/protocol_description.md`
  anchors every packet to its Java class. This is the single biggest
  asset for AI-agent-driven development.
- **The packet parser layer is clean.** Parsers in
  `packets/from_game_server` are pure (no I/O, no tracker access),
  defensive against implausible counts, covered by unit tests and
  benchmarks. This is the layer agents extend most often and it is the
  safest one to touch.
- **Regression harness culture.** `tools/repro_*.js` run the real
  `web/map.js`/`app.js` in a vm sandbox, were validated RED against the
  pre-fix code, and exit 1 while the bug is present. The fake-server
  integration tests (`connection/hunt_flow_test.go`) encode actual server
  semantics (the double click AttackRequest flow), which is exactly the
  right thing to test.
- **The single-lock `state.Bot` design is the right MVP call.** One
  RWMutex over a per-bot state object, `*Locked` suffix convention, a
  bounded event ring. It is correct and fast enough today; the problems
  with it are about growth, not correctness (part 2, B2).
- **Server integrity discipline.** The rule "the server is the reference,
  bot changes, never server patches" is written down, was enforced after
  a violation, and every vanilla quirk is documented where the bot logic
  can use it.

### The transition ahead

Everything below is judged against three requirements that the MVP did
not have:

1. **N bots per process** (9 minimum, 36 optimistic, 100 stretch) with
   staggered logins, per-bot config and lifecycle.
2. **Long-lived evolution by AI agents**: extension points must be cheap
   (one file, one registration), invariants must be visible, dead ends
   must be removed or fenced.
3. **Synchronized behavior between bots** later (party play, "eyes"
   dedup, cross-bot visibility per `docs/project_description.md`), which
   needs a seam where bots can see each other.

---

## Part 2. Findings

### A. Correctness issues (verified by hand)

- **A1. Outbound crypto race in `sendPacket`.**
  `connection/game.go:452` calls `gc.crypt.Encrypt(...)` *before* taking
  `gc.writeMu` at `game.go:454`. `crypt.GameCrypt.Encrypt` mutates the
  rolling `outKey` state, and at least two goroutines call `sendPacket`
  concurrently in normal operation: the run loop (ping at `game.go:758`,
  `Appearing` at `game.go:1453`, `Logout` at `game.go:778`) and the hunt
  loop (all client actions at `game.go:287-435`). Two consequences: a
  plain data race (`-race` would flag it) and a stream desync hazard
  (encryption order not matched to write order corrupts the XOR chain
  offset; the server then decrypts garbage). Probability rises sharply
  once web commands, more bots and more actors send concurrently.
- **A2. Data race in the pathfind layer pool.**
  `Engine.entry` (`pathfind/engine.go:210-247`) parses regions under
  `e.mu`, and `parseRegion` mutates the shared `layerPool` (`intern`
  appends to `p.layers`, `layer.go:44-58`). Searches and geodata tile
  renders call `Region.ClosestLayer/Layers` -> `pool.get` (`region.go:210-238`)
  *without* that lock while holding an already loaded region. A search
  that triggers a region load races any concurrent search reading the
  pool. Nothing in the test suite exercises concurrent engine use.
  Secondary: `Engine.maxPassableHeight` is read without the lock
  (`engine.go:95-102`); benign today, wrong in principle.
- **A3. The login phase ignores context and deadlines.**
  `NewGameClient`/`Authenticate`/`EnsureCharacter`/`EnterWorld`
  (`connection/game.go:486-673`) are unbounded blocking reads; only the
  handshake has a deadline. A stalling server hangs `runBot` forever and
  SIGINT is honored only at the next backoff tick of `runBotForever`
  (`cmd/swarm/main.go:245-269`). There is also no pong tracking: the
  `NetPing` reply is only logged (`game.go:1013-1021`), so a half-open
  connection is noticed only by TCP keepalive or a failed write.
- **A4. Dead code with a fake test.** `connection/connector.go`
  (`TCPConnector`/`RateLimitedConnector`/`RetryConnector`) is used by
  nothing but its own tests, `main.go` dials raw; its "succeeds after
  retries" test asserts on a counter that is never incremented (can never
  fail). Similarly `crypt` carries a second, unused framing/checksum
  stack (`Encryptor`/`Decryptor`/`Checksum` big-endian variant) and
  `random_unique_bench_test.go` is a generic algorithm playground, not
  crypto; `pathfind` has dead `Engine.cellLayers`; `packets` has two
  generations of the same benchmark (`init_bench_test.go` and
  `init_bench_v2_test.go`).
- **A5. No race detector anywhere.** `task test` runs plain
  `go test ./...` (no `-race`, no `-count=1`); the test:cover variant
  adds `--count=1` but not `-race`. Given A1/A2 and the multi-goroutine
  design, races can land unnoticed.
- **A6. Known data-type limit.** `state.CharacterState.Exp` is `int32`
  and the generated experience table documents that values wrap above
  level 78 (`state/experience.go:12-14`). Fine for the elven farm, wrong
  for a long-lived account.

### B. Architecture gaps on the road to N bots

- **B1. The process is shaped for exactly one bot.**
  `cmd/swarm/main.go` holds one account/char in flags, `runBotForever`
  supervises one tracker, `state.Registry` has `Add` but no
  `Remove`/lifecycle (`state/registry.go:27-63`), and there is no bot
  spec object (config), no staggered login (the server flood protectors
  punish 100 simultaneous logins) and no multi-instance story
  (`docs/project_description.md` explicitly wants several program
  instances with different configs to coexist). The webserver, by
  contrast, is already registry-shaped and multi-bot ready.
- **B2. `state.Bot` is a four-role god object with a global version.**
  `state/bot.go` (~1777 lines) mixes packet-fed observation, policy
  (zone), the JSON view (`Snapshot` knows icon packs, sorting and adena
  sums, `bot.go:1547-1675`) and the command transport (a 32-slot channel
  inside the state object). One RWMutex guards everything;
  `Snapshot()` deep-copies every object + inventory + events under the
  read lock; a single monotonic `version` (`bot.go:1196-1199`) is bumped
  by ~30 apply methods, so any change anywhere re-sends the whole
  snapshot. `CountPacket` takes the full write lock just to increment
  (`bot.go:1166-1170`). Per bot this is fine; per 100 bots with browsers
  attached it is the main scaling hot spot (see also B7).
- **B3. `hunt.Loop` is an ~80-field struct with three duplicated walk
  engines and hidden invariants.**
  - Waypoint following exists three times: `walkTownWaypoints`
    (`hunt/town.go:357-412`), `followUserWaypoints` (`hunt/user.go:404-466`,
    nearly line-for-line, subtly different z interpolation) and
    `walkZoneLeg` (`hunt/loop.go:722-734`); stuck detection exists in
    only one of them. Walk constants cross file boundaries
    (`returnWalkLeg` defined in `delevel.go:98`, used in `loop.go:726`;
    `maxMoveLeg`/`waypointArriveDist` defined in `town.go`, used in
    `user.go`).
  - `Loop.tracker` is the concrete `*state.Bot` (no interface), reached
    through ~25 distinct methods; the connection layer also reaches back
    into the tracker. There is no seam where a future party coordinator
    or "eyes" model can sit.
  - The delevel silently reuses the town-trip fields
    (`tripStart`/`rePaths`/`waypoints`, `delevel.go:174-231`) and must
    reset them by hand (`delevel.go:422-426`) - exactly the kind of
    hidden invariant an agent will violate.
  - The setters (`SetNavigator`/`SetAutonomy`/`SetHuntingZone`) are
    unsynchronized and only safe because main calls them before
    `go loop.Run`; nothing marks that convention.
  - `lastHit` (`loop.go:182`) is used as a generic "last action at"
    timestamp for five unrelated purposes; `l.skipped` stores deadlines
    while `l.targetSkip` stores event times - two conventions for the
    same idea.
- **B4. `connection/game.go` (1557 lines) merges six responsibilities.**
  Transport, framing, login state machine, keepalive, dispatch (five
  chained switches over 27 packet ids) and ~25 mechanical `applyXxx`
  state-application methods. Adding one inbound packet touches six
  places (parser file, id constant in `game.go`, reusable struct field,
  init in the constructor, a switch case, an apply method), and packet
  ids are declared twice (`game.go:36-75` and again in the packets
  package). This is the highest-friction extension point for agents and
  a merge-conflict magnet in one file.
- **B5. Observability does not scale to N.**
  Everything logs through `log.Default()` with no bot identity
  ("Hunt: ...", "Bot failed: ...", "Reconnecting in ..."); per-packet
  INFO logging is unbounded (every npc spawn, every char info, every
  unknown packet id, every ping - `game.go:1020,1060,1095,1128,1517`).
  At 9-100 bots stdout becomes an unattributable, serialized bottleneck.
- **B6. The shared pathfind engine is tuned for one bot.**
  LRU capacity 4 regions (~20 MB parsed each, `engine.go:26-28`), no
  `SetCapacity`, linear-scan touch; concurrent town trips in different
  territories will thrash it (re-parse on the hot path); the browser
  tile renderer shares the same engine and can evict a region a hunting
  bot needs. `render.go` (render modes, palettes, HTTP-shaped mode
  strings) is a visualization concern living inside the pathfinder - a
  documented MVP line, worth revisiting only if it grows.
- **B7. Web layer: full-snapshot fan-out, duplication, unbounded caches.**
  - SSE (`webserver/server.go:167-249`) deep-copies + marshals the whole
    snapshot *per connected client per version change* (no encode-once
    cache, no gzip); a combat snapshot is ~60-100 KB at up to 3.3 Hz per
    viewer. `Registry` never removes ended bots from `/api/bots`.
  - `web/app.js` carries the paperdoll placement rules twice
    (`assignPaperdoll` and `slotKeyOf`), `headingCardinal` duplicates
    `map.js`'s `cardinalOf`, and the four `tools/repro_*.js` harnesses
    each re-implement their own stub DOM/vm bootstrap/check helper
    (~300 lines of drift-prone copies).
  - `map.js` tile caches (`mapTiles`, `geoTiles`) never evict;
    `geoTiles` is keyed by render mode, a 2048x2048 tile decodes to a
    16 MB bitmap - pathfind-mode browsing can hold hundreds of MB.
    `worldToScreen` calls `getBoundingClientRect()` per object per frame
    (`map.js:1541-1550`).
  - The log panel watermark has a filter bug: `seenEvents` counts
    unfiltered events, so a changed filter without an array length
    change renders nothing (`app.js:1141-1163`).
- **B8. No clock abstraction.** Every timing decision uses
  `time.Now()` directly in `state` and `hunt`; tests back-date private
  fields and sleep in real time (e.g. `hunt/user_test.go:309-311,753-756`
  wait up to 6 s for 3 s windows), which is slow and mildly flaky, and
  makes time-driven logic untestable. This is the highest-leverage small
  change for future agents.
- **B9. Hardcoded environment and deployment data.**
  `defaultGeodataCandidates` contains the absolute path
  `E:\work\lineage_workspace_fresh\...` (`cmd/swarm/main.go:49-50`);
  the hunting zone, the merchant table and the guard table are
  package vars (`hunt/loop.go:375`, `hunt/town.go:89-94`,
  `hunt/delevel.go:115-118`); ~50 timing/threshold constants are spread
  over four files. Every new hunting ground or territory currently
  requires a code edit.

### C. AI-agent ergonomics (the axis that will dominate)

These are not bugs; they are the places where an autonomous agent is
most likely to lose time or break something.

- **C1. Hidden invariants with no visible marker.** The "set the loop
  config before `go loop.Run`" rule; the delevel/trip field sharing;
  "the icon pack resolves from the process working directory, launch
  from the repo root"; top-level `const` in classic scripts are not
  `window` properties (AGENTS.md warns, but the code does not); new
  top-level DOM access in `app.js`/`map.js` silently breaks the four vm
  harnesses unless guarded.
- **C2. Extension friction map.** Rough cost of common tasks today:
  new inbound packet: 6 touch points (B4); new gear slot: 2-3 places in
  `app.js` plus CSS; new hunt threshold: find the right file among four;
  new log line: just call `log` (no identity, no level) - too cheap;
  new snapshot field: Go struct + JSON tag + renderer - actually fine
  and should be documented as the good example.
- **C3. Tests are white-box and field-coupled.** Hunt tests back-date
  private `Loop` fields directly; any field regrouping breaks dozens of
  tests at once. That is safe for refactoring only if done in one
  disciplined move (hence prompt P07) - an incremental agent refactor
  would be punished. Combined with B8 there is no way to test
  time-driven behavior without sleeping.
- **C4. Traps for agents.** Two parallel crypt stacks and a dead
  connector package invite reuse of the wrong one; the two skip-map
  conventions and the `merchantID` sentinel triad (0/>0/-1) invite
  off-by-one semantics; `SelfEngaged` vs `SelfFighting` differ only in a
  freshness window and the difference decides whether attacks are
  re-requested (documented only in comments); `.tmp-geodata/` and
  untracked build artifacts sit locally (gitignored, but noisy); the
  lint toolchain is broken on the main dev machine (golangci-lint v1 vs
  Go 1.27 export data, v2 needs a config migration), so `task lint` is
  not currently executable where most work happens.
- **C5. What must NOT be "fixed" (fence it).** The plain-JS no-build web
  UI; the single-RWMutex `state.Bot` (do not shard preemptively before
  profiling at N); the vm-sandbox harness approach; the deliberate
  pathfind deviations documented in AGENTS.md; server integrity rules
  (never patch gameplay). Every prompt below repeats the important ones.

---

## Part 3. The improvement program

Four waves, each independently shippable, ordered by
`risk x leverage`. One prompt = one branch = one dev-log entry.

- **Wave 1 - stop the bleeding (correctness, no behavior change):**
  P01 races + race detector, P02 session lifecycle, P03 hygiene.
- **Wave 2 - foundations for everything else:** P04 per-bot logging,
  P05 packet router + recipe, P06 clock abstraction, P07 hunt
  consolidation (walker, fields, naming).
- **Wave 3 - the multi-bot core:** P08 world config, P09 tracker seam,
  P10 bot supervisor and specs.
- **Wave 4 - scale and polish:** P11 state/snapshot scaling, P12
  pathfind hardening, P13 web consolidation, P14 agent documentation.

Dependency notes: P04/P06 before P10; P07/P08/P09 before P10; P05 before
any "add packets" feature work; P11/P12/P13 are independent of P10 but
should land before real N-bot runs. P14 grows incrementally: every other
prompt updates the recipes it touches.

---

## Part 4. How to run the prompts

- One prompt per agent session, on its own feature branch; keep commits
  small and imperative per the git conventions.
- Each prompt is self-contained: it tells the agent what to read, what
  to do, what not to touch, and how acceptance is verified.
- Every prompt ends with the same closing ritual: run the full test
  suite plus the affected repro harnesses, write the entry into
  `docs/development_log.md` (format: date, scope, problem, root cause,
  reproduction, fix, verification), and update `AGENTS.md` wherever the
  documented behavior or a recipe changes.
- The Windows dev host has no `task` binary: agents should run the
  underlying commands directly (`go build ./...`, `go vet ./...`,
  `go test ./... -count=1 -race`, `gofmt -l .`, `node tools/repro_*.js`).
- Server integrity: no Mobius gameplay patches, only logging/diagnostic
  lines; the bot adapts to the server.

---

## Part 5. Agent prompts

### P01 - Fix the data races and make the race detector permanent

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch; follow all repo conventions (SPDX headers, 80 cols, godot
comments, testify).

Fix two verified data races:

1. internal/swarm/connection/game.go sendPacket (~line 443): the call
   gc.crypt.Encrypt(writer.Bytes()) at ~line 452 happens BEFORE
   gc.writeMu is taken at ~line 454. GameCrypt.Encrypt mutates the
   rolling key, and the run loop (ping, Appearing, Logout) and the hunt
   loop (client actions) call sendPacket concurrently. Move the encrypt
   inside the writeMu critical section (serialize+encrypt+write under
   one lock; keep the trace log where it is correct). Add a regression
   test that proves encryption and wire writes cannot interleave (e.g.
   a serializable fake that records order, or a -race targeted test
   with two goroutines spamming sendPacket).

2. internal/swarm/pathfind: Engine.entry parses regions under e.mu and
   parseRegion mutates the shared layerPool (layer.go intern), while
   Region.ClosestLayer/Layers call pool.get WITHOUT the lock from
   concurrent searches and tile renders. Make the layer pool safe
   (protect intern/get with its own small mutex - reads are one slice
   index, contention is negligible) and add a test that runs concurrent
   FindPath calls over multi-region geodata with -race (use
   pathfind/testdata/22_22.l2j).

Then make the race detector permanent: update Taskfile.yml test and
test:cover to include -race -count=1, run go test ./... -race -count=1
over the whole repo and fix anything it surfaces (known suspects:
Engine.maxPassableHeight read without lock in engine.go, testify
require calls from fake-server goroutines in connection tests).

Constraints: no behavior change beyond serialization of the two race
sites; do not touch the web layer; do not refactor anything else.

Acceptance: go build ./..., go vet ./..., go test ./... -race -count=1
green; the new regression tests fail on the pre-fix code (verify by
stashing your fix once); docs/development_log.md entry written; AGENTS.md
Testing conventions section updated to mention -race.
```

### P02 - Context and deadlines through the session lifecycle

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch.

Problem: the login phase of the game session ignores context and
deadlines. connection/game.go NewGameClient/Authenticate/EnsureCharacter/
EnterWorld (~lines 486-673) are unbounded blocking reads; a stalling
server hangs cmd/swarm runBot forever and SIGINT is only honored at the
next backoff tick of runBotForever. Additionally the RequestNetPing
reply (NetPing 0xEC, handled around game.go:1013) is only logged: a
half-open connection is never detected by the application.

Tasks:

1. Thread ctx through the login phase: change the signatures of
   NewGameClient, Authenticate, EnsureCharacter, EnterWorld to take
   context.Context, set read deadlines per step (reuse connectTimeout or
   introduce named per-step timeouts), and make every read-until-
   expected-packet loop select on ctx.Done() as well as the deadline.
   Update cmd/swarm/main.go call sites accordingly (sessionCtx already
   exists). Keep the game Run loop behavior unchanged.
2. Add pong tracking: record the time of the last NetPing reply; if no
   pong arrives within 2 ping periods, close the connection with a clear
   error so the normal reconnect path takes over. Cover it with a fake
   server test (send no pongs, require the client to drop).
3. Add a test proving Ctrl+C responsiveness: a fake game server that
   accepts ProtocolVersion and then stalls must not prevent ctx
   cancellation from returning runBot within a bounded time.

Constraints: do not change packet formats or the reconnect backoff
policy; keep logging conventions (capital first letter, no trailing
period).

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 green
including the new tests; a manual smoke note in the dev log describing
the stalled-server scenario and the SIGINT behavior.
```

### P03 - Remove dead code and repair the toolchain gaps

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch.

Goal: shrink the surface an AI agent can trip over. Remove verified dead
code, repair fake tests, and make lint runnable again on the Windows dev
host.

1. Delete connection/connector.go and connector_test.go (unused by
   production; its retry test asserts on a counter that is never
   incremented). Verify with grep that nothing imports it.
2. In internal/swarm/crypt: delete the unused big-endian
   Encryptor/Decryptor/Checksum stack (the live login path uses
   LoginCrypt.Seal/Open + ChecksumLE) and the unrelated
   random_unique_bench_test.go playground. Keep GameCrypt and
   LoginCrypt untouched.
3. Delete pathfind Engine.cellLayers (dead) and reconcile the duplicated
   packet benchmarks (packets init_bench_test.go vs init_bench_v2_test.go
   - keep one generation, rename its helpers clearly).
4. Unify small duplications: npcDisplayOffset = 1000000 is defined both
   in state/bot.go and hunt/town.go - keep one exported definition.
5. Fix the lint toolchain: golangci-lint v1 fails on this host (export
   data version mismatch with the newer Go). Migrate .golangci.yml to
   the v2 format per the official migration guide and verify
   golangci-lint run passes; if a linter from the strict set is
   unavailable in v2, keep the closest equivalent and record the change.
6. Update AGENTS.md commands section where tool names/behavior changed.

Constraints: removals only for code with zero production references
(prove each with grep); do not "clean up" anything documented as a
deliberate deviation in AGENTS.md.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 green;
golangci-lint run green (or a documented blocker in the dev log); grep
proof for every removal listed in the dev log entry.
```

### P04 - Per-bot logging identity and a log level floor

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch.

Problem: every log line goes through log.Default() with no bot identity
and no levels. Per-packet INFO logging is unbounded (every npc spawn,
char info, item list, ping, unknown packet in connection/game.go; hunt
actions in hunt/*.go). At 9-100 bots the interleaved stdout is
unattributable and the logging itself becomes a bottleneck.

Tasks:

1. Introduce a minimal leveled logger type in a small internal package
   (or as part of state): Debug/Info/Warn/Error, backed by log.Logger,
   with a per-bot prefix like "[test1] ". Constructors take it as a
   parameter: state.NewBot, hunt.NewLoop, connection.NewGameClient and
   the webserver already accept a logger - extend, do not keep
   log.Default() fallbacks inside the packages.
2. cmd/swarm builds one logger per bot with the account name and passes
   it everywhere; main-level lines (Starting, Reconnecting, Bot failed)
   get a process prefix.
3. Demote the per-packet noise: npc spawn/char info/item list/ping logs
   become Debug (compiled to no-ops at default level or gated by an
   -log-level flag, default info). Unknown packet ids keep a per-id
   once-only Info or a counting summary line instead of one line per
   packet. SWARM_TRACE_PACKETS keeps working unchanged.
4. Update AGENTS.md logging conventions: keep "capital first letter, no
   trailing period", add the prefix and level rules, and state that
   packages must never call log.Default() directly.

Constraints: do not switch to log/slog in this task (a separate
decision); do not change any behavior, only log plumbing; keep the
existing message texts where possible so the dev-log and e2e greps keep
working.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 green;
run the bot briefly against the local stack and paste into the dev log
an interleaved two-bot log sample where lines are attributable;
grep -rn "log.Default()" internal/ returns nothing outside main.
```

### P05 - Packet router table and the "add a packet" recipe

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch.

Problem: inbound dispatch in connection/game.go is five chained switch
statements; packet id constants are declared twice (game.go and the
packets package); adding one inbound packet touches six places and
concentrates edits in one 1557-line file. This is the highest-friction
extension point for agents.

Tasks:

1. Introduce a handler table in the connection package:
   map[byte]handlerFunc where handlerFunc is a method on GameClient
   taking the payload; build it once in NewGameClient from a
   declarative slice (id, name, handler) so the table is readable and
   diff-friendly. Replace the switch cascades with a single table
   lookup; unknown ids keep the current unknown-packet path.
2. Make the packets package the single source of packet id constants:
   export an ID per packet there and delete the duplicated constants
   from game.go.
3. Keep all applyXxx methods as they are (they are mechanical but
   working); they become the handler targets. Do not restructure
   GameClient in this task.
4. Document the recipe in AGENTS.md (a short "Adding a new inbound
   packet" checklist: parser file + test + bench in packets/..., one
   line in the handler table, an apply method, protocol_description.md
   entry). The recipe must make the next packet a two-file change.
5. Cover the router with a table-driven test that asserts every
   registered id maps to a non-nil handler and that dispatch reaches
   the right handler for a sample payload.

Constraints: zero behavior change for handled packets; do not move
applyXxx into other packages; keep the vm/web layer untouched.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 green;
the fake-server tests (game_test.go, hunt_flow_test.go) pass unchanged;
a dev-log entry listing the before/after touch-point count for adding a
packet.
```

### P06 - Clock abstraction for state and hunt

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch.

Problem: all timing in state and hunt uses time.Now() directly. Tests
back-date private fields and sleep in real time (hunt/user_test.go
waits up to 6 s for 3 s windows); time-driven logic (combat windows,
freshness, engagement stalls, confirmations) is untestable and slow.

Tasks:

1. Add a minimal clock seam (a now func() time.Time field defaulting to
   time.Now, or a tiny internal/clock package with a Clock interface
   and SystemClock) to state.Bot and hunt.Loop. Constructors default to
   the system clock; production code changes nothing else.
2. Replace direct time.Now()/time.Since reads inside state and hunt
   decision logic with the injected clock. Be exhaustive in hunt
   (engagement, rest, loot, trip, delevel, user phases) but do not
   touch ticker creation in Loop.Run beyond injecting the clock into a
   small tick source if trivial - otherwise leave the ticker and note
   it.
3. Migrate the tests that currently sleep for freshness windows
   (user_test.go TestUserSwap* freshness cases, the fighting-freshness
   poll, chase progress windows) to a fake clock: zero or near-zero
   wall time, deterministic.
4. Keep back-dating tests that are cheap and hermetic as they are; only
   convert tests that sleep.

Constraints: no behavior change; the exported API may grow but existing
signatures keep working (variadic options or NewXxxWithClock are fine);
do not introduce a full framework (no clockmock libraries).

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 -race
green; total wall time of go test ./internal/swarm/hunt noticeably
lower (report before/after seconds in the dev log); no new time.Sleep
added anywhere in the repo (grep proof).
```

### P07 - One waypoint walker and a tidy hunt Loop

```
Read AGENTS.md and docs/development_log.md first (the town trip, user
command and delevel rounds). Work on a feature branch. This is a
refactor with test coverage as the safety net: tests currently back-date
private Loop fields, so do the field regrouping and the test migration
in the same change.

Problems: waypoint following exists three times (walkTownWaypoints in
hunt/town.go:357-412, followUserWaypoints in hunt/user.go:404-466 with
a subtly different z interpolation, walkZoneLeg in hunt/loop.go:722-734);
stuck detection exists in only one copy; walk constants cross file
boundaries (returnWalkLeg defined in delevel.go used in loop.go,
maxMoveLeg/waypointArriveDist defined in town.go used in user.go);
Loop is an ~80-field flat struct mixing six phases; lastHit is a
misleading name used for five purposes; l.skipped stores deadlines
while l.targetSkip stores event times.

Tasks:

1. Extract one walker type in the hunt package: legs capped at maxLeg,
   request pacing (walkRequestPeriod), arrival radius, pass-skip of
   already reached waypoints, stuck detection (progress window + re-path
   callback), and optional plan publishing into the tracker
   (SetWalkPlan/ClearWalkPlan semantics preserved exactly - the web map
   draws walkPath from it). Give it the clock from P06.
2. Rewire town trips, user walks, delevel walks and the zone-return leg
   onto the walker. Preserve the exact externally visible behavior:
   log lines, pacing, timeouts, the userRedirect instant-replace
   behavior and the walk plan lifecycle (there are tests pinning all of
   these - keep them green or extend them, never weaken them).
3. Group Loop fields into per-concern sub-structs (config, engage,
   loot, rest, trip, delevel, user) and update the same-package tests
   in one move. Rename lastHit to lastActionAt (or split into the
   distinct timestamps it actually represents) and unify the two skip
   conventions into one (deadline-based) with both semantics renamed
   clearly.
4. Hoist all walk/movement constants into one file with doc comments
   (unit = world units, source of each value).

Constraints: this must stay inside internal/swarm/hunt plus its tests;
no behavior change (the fake-server hunt flow test must stay green);
do not touch delevel/town trigger logic beyond wiring the walker.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 -race
green; node tools/repro_gear.js etc. unaffected; go test
./internal/swarm/hunt wall time reported before/after; the dev log
entry documents the walker API and the new field layout.
```

### P08 - World configuration out of code

```
Read AGENTS.md, docs/development_log.md and docs/project_description.md
first. Work on a feature branch.

Problem: the world is compiled in. The hunting zone
(hunt.DefaultHuntingZone, a 3300x3300 square near the Elven village),
the merchant table (hunt/town.go townMerchants, Elven village only),
the delevel guard table (hunt/delevel.go delevelGuards), the character
creation preset (cmd/swarm/main.go elf fighter constants) and ~50
timing/threshold constants live in code. A new hunting ground currently
requires editing Go source; multi-bot (P10) needs per-bot world config;
docs/project_description.md explicitly wants multiple coexisting
instances with different configs.

Tasks:

1. Define a BotConfig struct in a new internal/swarm/config package
   (or internal/swarm/world - your call, keep it out of state):
   account, password, char name, char creation preset, autonomy,
   hunting zone, merchants, delevel guards, geodata dir override, web
   address. Provide Load(path) plus FromFlags for the current CLI
   flags, and embed a sane default config equivalent to today's
   behavior (same zone, same merchants, same guards).
2. Replace the hardcoded tables: hunt takes zone/merchants/guards from
   the config (the tables move to the default config), and the
   absolute E:\ path in cmd/swarm/main.go defaultGeodataCandidates
   becomes a SWARM_GEODATA environment variable or config field with
   the portable candidates kept.
3. Wire -config <file> into cmd/swarm (flags still override or fill
   missing fields; document the precedence). JSON is enough for now;
   keep it dependency free.
4. Add tests: default config equals current hardcoded behavior (table
   compare with the old values), config file parsing, precedence.
5. Update AGENTS.md commands section (-config) and note the config
   file location rules.

Constraints: the default run (no -config) must behave exactly as today;
do not make thresholds like restThreshold configurable yet unless they
fall out of the struct naturally - the goal is world data and identity,
not a settings UI.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 green;
a run with the default config reproduces today's zone/merchants/guards
(asserted by tests); a run with a test config pointing at a shifted
zone is validated by a unit test, no live server needed; dev log entry
written.
```

### P09 - Tracker seam: hunt against an interface

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch.

Problem: hunt.Loop.tracker is the concrete *state.Bot, used through
~25 methods; the connection layer also calls back into the tracker.
There is no seam where the future party coordinator, the "eyes" shared
world model (docs/project_description.md) or a test double can sit.
state.Bot is simultaneously growing UI-view responsibilities (Snapshot
knows icons and adena sums) - the roles need named boundaries before
N-bot work starts.

Tasks:

1. In the hunt package, define interface Tracker with exactly the
   methods the loop actually uses (harvest the list from the code, do
   not invent). Loop.tracker becomes that interface; state.Bot
   satisfies it implicitly; cmd/swarm passes the concrete bot. Update
   tests where they rely on concrete methods beyond the interface.
2. Move the pure JSON-view assembly (the Snapshot icon/name/adena
   decoration) behind a small view function or keep it but document
   Snapshot as "the web view constructor" in its doc comment - the
   goal is an honest comment boundary, not a new package yet.
3. Define the same seam for the connection side: the small set of
   tracker calls made from connection (RecordEvent, position reads)
   becomes a narrow interface on GameClient's tracker field.
4. Document in AGENTS.md (short section): "hunt and connection depend
   on interfaces, state.Bot is the implementation; anything cross-bot
   goes through a new implementation later, not by widening Bot".
5. No method renames unless a name is actively misleading; keep the
   diff mechanical.

Constraints: do not change state.Bot behavior; do not start the
multi-bot world model here - this task only cuts the seam and makes
the dependency direction explicit.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 green;
a grep shows internal/swarm/hunt no longer imports state except in
tests and cmd wiring (adjust if reality differs, and record the result
honestly in the dev log); the interface list is documented in the dev
log entry.
```

### P10 - Bot supervisor: specs, staggered logins, registry lifecycle

```
Read AGENTS.md, docs/development_log.md and docs/project_description.md
first (multi-instance and ordered login requirements). Work on a feature
branch. Depends on: P04 (per-bot logging), P06 (clock), P08 (config),
P09 (seams).

Problem: the process runs exactly one bot: flags hold one account,
runBotForever supervises one tracker, state.Registry has Add but no
Remove/lifecycle, and 100 simultaneous logins would trip the server
flood protectors (docs/project_description.md requires ordered logins,
e.g. tanks first).

Tasks:

1. Extend the P08 config to a list of bot specs (a top-level "bots"
   array in the config file; single-bot behavior preserved when the
   list has one entry or flags are used).
2. Build a supervisor in cmd/swarm (or a small internal/swarm/supervisor
   package): one goroutine per spec running the current runBotForever
   logic with per-bot backoff, plus a global login stagger (configurable
   delay between consecutive logins, default a few seconds, plus small
   jitter). Bot logins proceed in spec order.
3. Registry lifecycle: add Remove and Offline(id) (mark a bot
   disconnected but keep its last state for the UI), have the
   supervisor use them; /api/bots must show offline bots distinctly
   instead of growing forever.
4. The webserver is already registry-shaped - verify multi-bot
   selection works end to end and fix only what is actually broken.
5. Add an e2e script tools/mobius_e2e_multi.sh (modeled on
   mobius_e2e.sh) that runs N=3 bots against the local stack, waits,
   kills one connection deliberately, and verifies the reconnect +
   staggered relogin in the log.
6. Document: AGENTS.md gets the config file format and the supervisor
   behavior; the dev log entry records a live 3-bot run result.

Constraints: single-bot flag usage must keep working unchanged (the
default config path); do not implement party logic, shared world or
dedup here; do not change the login protocol flow.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 -race
green; tools/mobius_e2e_multi.sh passes on the local stack (or its
result honestly recorded as a known limitation); the web UI shows 3
bots with independent selection (manual smoke documented).
```

### P11 - Snapshot scaling: encode-once, versions, gzip

```
Read AGENTS.md and docs/development_log.md first. Work on a feature
branch. Depends on: P09 (seams) recommended.

Problem: state.Bot has one global version bumped by ~30 apply methods;
Snapshot() deep-copies everything under the read lock; the webserver
deep-copies + marshals the full snapshot PER CONNECTED CLIENT per
version change (webserver/server.go SSE loop), uncompressed. One
browser on a busy bot costs ~60-100 KB at up to 3.3 Hz; ten browsers
multiply it; 100 bots with viewers would collapse. Also known: Exp is
int32 and wraps above level 78 (state/experience.go comment).

Tasks:

1. Encode-once: cache the marshaled snapshot bytes keyed by the bot
   version inside the webserver (or the state package); concurrent SSE
   clients reuse the cached bytes; invalidate on version change. Verify
   marshal cost moves to once per version per bot, not per client.
2. Add gzip (Content-Encoding) to the snapshot/event responses -
   snapshot JSON compresses heavily; it is a small change with a big
   win. Keep the SSE framing valid for the browser EventSource.
3. Split the single version into a small set of section versions
   (character, objects, inventory, events/chat) OR document why the
   single version stays for now; if you split, the SSE loop sends the
   sections that changed instead of the whole snapshot, keeping the
   event name scheme compatible with app.js (extend, do not break the
   current single "snapshot" event for small changes).
4. Fix the Exp data type: switch the character exp fields to int64 in
   state and the snapshot, regenerate/adjust the experience table
   usage, add a test above the int32 wrap boundary.
5. Benchmarks: add a benchmark for Snapshot() at a realistic world
   size (e.g. 300 objects, 80 inventory items) and report
   before/after allocs and ns/op in the dev log.

Constraints: the browser contract (event names, JSON fields) stays
backward compatible; app.js changes only if a new section event is
introduced, and then guarded (the vm harnesses must keep passing).

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 -race
green; node tools/repro_hud.js, repro_gear.js, repro_map_render.js,
repro_movement.js ALL PASS; a webserver test proves two concurrent SSE
clients share one marshal (e.g. counting marshal invocations via a
hook or by timing/instrumentation); dev log entry with the benchmark
numbers.
```

### P12 - Pathfind engine hardening for concurrent bots

```
Read AGENTS.md (Pathfinding section) and docs/development_log.md first.
Work on a feature branch. Depends on: P01 (the layerPool race fix).
Note: the AI-agent-written analysis may be wrong in details - verify
each claim in code before acting.

Problem: one shared Engine serves all bots (town trips, manual walks)
plus the browser tile renderer. The LRU holds 4 regions (~20 MB parsed
each), has no capacity knob and a linear-scan touch; bots walking in
different territories will thrash it (a region parse is a multi-MB disk
read on the hot path); the browser can evict regions an active bot
needs; failed loads stay cached forever (good for missing files, but
worth a size bound); there are no concurrency tests beyond P01's.

Tasks:

1. Add Engine.SetCapacity (and a -pathfind-cache flag in cmd/swarm,
   default raised to something sane for multi-bot, e.g. 16 - measure
   resident memory with the real pack before committing the default).
2. Replace the linear-scan LRU touch with a container/list based
   implementation (the same for the webserver geodata tile cache if
   trivial); keep eviction semantics identical.
3. Fix the remaining lockless read of maxPassableHeight (small mutex or
   atomic).
4. Bound the negative cache (failed region entries) - e.g. cap it or
   expire; a wrong directory must not poison a long-lived process
   forever after the operator fixes the path.
5. Benchmarks + tests: a cache-thrash benchmark (alternating regions,
   capacity 1 vs 16), a concurrent search test under -race, and keep
   the existing benchmarks green. Report numbers in the dev log.
6. Do NOT move render.go out of the package in this task; if you
   consider it, write a recommendation paragraph in the dev log
   instead.

Constraints: no search algorithm changes; the deliberate deviations
from the original pathfinder documented in AGENTS.md are off limits.

Acceptance: go build ./..., go vet ./..., go test ./... -count=1 -race
green (pathfind benchmarks run with -bench . -benchmem); dev log entry
with capacity/memory/latency numbers for the chosen default.
```

### P13 - Web layer consolidation

```
Read AGENTS.md (Web interface section) and docs/development_log.md
first. Work on a feature branch. The web UI must stay plain
HTML/CSS/JS without a build step.

Problems: paperdoll placement rules exist twice in app.js
(assignPaperdoll and slotKeyOf); headingCardinal duplicates map.js
cardinalOf; the four tools/repro_*.js harnesses each carry their own
stub DOM + vm bootstrap + check helper (~300 lines of drift); map.js
tile caches never evict (geoTiles keys include render mode; a decoded
2048x2048 tile is ~16 MB); worldToScreen calls getBoundingClientRect
per object per frame; the log panel watermark (seenEvents) mishandles
filter changes (app.js ~1141-1163).

Tasks:

1. Extract tools/harness_lib.js (plain Node require, not browser code):
   shared stub DOM factory, vm loader, check/summary helpers. Rewire
   the four harnesses onto it; each keeps its own scenarios. The
   harnesses must still exit 1 while their bug is present - verify by
   running them against the current (good) code for PASS.
2. Deduplicate app.js: one paperdoll placement implementation used by
   both rendering and the equip-swap slot resolution; one heading
   helper shared via the existing bare-binding convention (watch the
   classic-script load order documented in AGENTS.md).
3. Bound map.js caches: an LRU or per-mode cap for geoTiles with
   eviction of Image objects; keep mapTiles as-is if it is bounded by
   the fixed world tile set (document why).
4. Cache the canvas rect once per frame (or per resize/scroll) instead
   of per worldToScreen call; keep the pixel behavior identical (the
   harness scenarios must stay green, add one if needed).
5. Fix the log filter watermark: a changed filter must re-render the
   visible rows deterministically; add a harness case for it.

Constraints: no framework, no build step, no visual changes; the keyed
incremental gear renderer (GearCells, the anti-flicker design) must not
be replaced by innerHTML rebuilds; all four harnesses plus go test
./... must stay green.

Acceptance: go test ./... -count=1 green; node tools/repro_hud.js,
repro_gear.js, repro_map_render.js, repro_movement.js ALL PASS; a dev
log entry listing the removed duplication (line counts before/after)
and the harness lib contract.
```

### P14 - The agent handbook: extension recipes and invariants

```
Read AGENTS.md, docs/development_log.md, docs/project_description.md
and docs/protocol_description.md first. Work on a documentation branch
(with small code changes only where a doc claims a recipe that does not
exist yet and needs a tiny enabling change).

Goal: make the invisible visible for autonomous agents. AGENTS.md is
behavioral documentation; this task adds the operational "how do I
extend X" layer and the invariant list.

Tasks:

1. Create docs/extension_guide.md with step-by-step recipes, each with
   the exact file list and the verification commands:
   - add an inbound/outbound packet (keep in sync with the P05 recipe
     if it landed; otherwise document the current switch flow);
   - add a snapshot field end to end (Go state -> JSON -> app.js
     renderer -> harness check) - this is the current best-practice
     path, document it as such;
   - add a hunt phase or threshold (which file, which test to mirror);
   - add a paperdoll slot or a widget interaction (masks, aliases,
     either-or pairs, the keyed renderer rules);
   - add a web panel (the renderSnapshot fan-out convention);
   - add a repro harness scenario (the vm sandbox constraints: no
     top-level DOM access, typeof Image guards, load order).
2. Add an "Invariants and traps" section to docs/extension_guide.md:
   the set-before-Run rule for loop config (or its fix), the walk plan
   lifecycle, the confirmation-paced command gate, the stale-selection
   recovery story, the icon working-directory rule, const-not-window,
   the server integrity rules, the "do not fix" list from the review
   (plain-JS no-build UI, single-lock Bot until profiled, vm harness
   approach, deliberate pathfind deviations).
3. Add a "constants inventory" table: the timing/threshold constants,
   their file, unit (world units/seconds), and origin (measured,
   Mobius-derived, arbitrary) - the delevel/town/loop/user files are
   the source.
4. Propose a development_log.md growth policy in the same doc: entries
   are append-only, one section per round, and when the file exceeds a
   threshold it gets split per year with an index (do not split it now).
5. Update AGENTS.md to link the guide where agents are addressed.

Constraints: documentation only plus the minimal enabling tweaks;
do not restructure code; keep every claim verifiable against the
current code (an agent must be able to follow each recipe and land a
green change).

Acceptance: a reviewer (or a follow-up agent) can execute the "add a
snapshot field" recipe literally and get a green test + harness run;
docs lint (gofmt untouched, markdown sane); dev log entry recording
the guide creation.
```

---

## Part 6. Findings-to-prompts map

| Finding | Prompt(s) |
| --- | --- |
| A1 crypto send race | P01 |
| A2 layerPool race | P01 |
| A3 login ctx/deadlines, no pong tracking | P02 |
| A4 dead code, fake test | P03 |
| A5 no race detector | P01, then permanent |
| A6 int32 exp wrap | P11 |
| B1 one-bot process | P08, P10 |
| B2 state.Bot god object, version/snapshot | P09, P11 |
| B3 hunt duplication, hidden invariants | P07 |
| B4 game.go god file, extension friction | P05 |
| B5 unattributed, unbounded logging | P04 |
| B6 pathfind LRU/capacity | P12 |
| B7 web fan-out, duplication, caches | P11, P13 |
| B8 no clock abstraction | P06 |
| B9 hardcoded world and paths | P08 |
| C1-C5 agent ergonomics | P03, P05, P07, P14 |

Deliberately not scheduled: party synchronization and the shared "eyes"
world model (they need P09/P10/P11 as prerequisites and their own design
round, see `docs/project_description.md` and
`docs/navigation_analysis.md`); the webserver multiplexed event stream
beyond P11's encode-once step; any UI redesign. Revisit after Wave 3
with real N-bot measurements.
