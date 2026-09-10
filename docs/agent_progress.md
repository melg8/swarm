# Agent progress log

Crash-safe task tracking: the current task, its full context and
per-commit progress live here (see the "Work protocol" section in
AGENTS.md). Entries are append-only; a new agent resumes the newest
unfinished entry.

This file carries ONLY the active and the most recent context:
finished task entries and older progress streams move to
`agent_progress_archive.md` (append-only, same order). The permanent
root-cause history of every round lives in `docs/development_log.md`;
check the archive when the recent context references an older task.

## Active task: the webui modernization proposal (awaiting the user approval)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
The analysis-and-proposal round of the web UI work (the map toolbar
round below is done and live verified). Other agents may push to the
same branch concurrently - rebase before every push.

### Goal

The user report (2026-09-10, Russian): analyze the webui and propose
how to make the interface more readable, more modern and more
ergonomic; deliver the proposals as a file for the approval. No UI
changes land before the approval.

### Changes

- docs/webui_modernization_proposal.md: the proposal document (in
  Russian, the approval audience) - the analysis method, what stays
  untouched, the three-axis diagnosis (readability, modern feel,
  ergonomics), 30 numbered proposals in phases A/B/C plus the dev-mode
  minors, a three-wave rollout order with effort estimates and the
  per-item approval checklist at the end.

### Analysis inputs

- Live captures of every mode at 1440x900 (Mobius stack + `bot -hunt`,
  pathfind 8081, fight showcase v1 8082, fight gallery 8083): light and
  dark bot themes, the open view dropdown, the log tab, the open shop
  flyout, the expanded zone panel - /home/z/my-project/download/audit/.
- Full pass of style.css (2169 lines), index.html (377) and the UI
  logic of app.js/map.js/main.js; geometry measurements of every panel.
- Vision model reviews of the key screenshots (light, dark, log,
  pathfind) cross-checked against the code before landing in the
  document.

### Status: awaiting the user approval (2026-09-10)

- Nothing in internal/swarm/webserver/web/ changed this round; the
  deliverable is the proposal file itself.
- The implementation waves live in the proposal's section 9; every
  approved item lands as its own atomic commit with the repro suite
  updates and the live agent-browser verification, as the previous
  rounds did.

## Unfinished task: fleet scale hardening (100 bots), DOD round 2

Started: 2026-09-08 (second benchmark round). Branch:
`mobius-c1-client-1`. Commits as melg8, pushed as they land.

### Goal

The user asked to continue covering the code with tests and
benchmarks, to identify the remaining bottlenecks, and to optimize
them for the real deployment shape of the project: not one bot but
up to 100 concurrent bot sessions in one process. Apply data
oriented design, pay special attention to unnecessary memory
allocations and cache misses in the operations, make the hot paths
cache friendly, and refactor the oversized god object classes.

### Constraints

- AGENTS.md rules: the stack deployment first, atomic commits pushed
  immediately, the go-verify-loop (build + vet + test + lint) before
  every push, the live E2E at wrap up.
- The server integrity rules: no server patches, the bot adapts.
- The existing benchmark suite (state, npcdata, hunt, webserver)
  pins the previous round results; keep them green.
- A parallel agent may edit the same branch: rebase before push.

### Acceptance criteria

- Fleet benchmarks (100 bots) exist for the registry, the bot list
  endpoint, the SSE stream path and the aggregate apply load.
- The per SSE event allocation profile is fixed (the frame + encode
  buffers are reused, the intermediate deep copies removed where
  the hot path allows).
- The registry iterates a dense slice (no per bot string hash map
  lookups in the list walk).
- The god objects (state.Bot 2448 lines, connection.GameClient 1624,
  hunt.Loop 1480) are split into cohesive components without
  behavior changes (the existing tests stay green unchanged).
- Every optimization is proven by the benchmark before/after
  numbers recorded here.

### Progress

- 2026-09-08: the fleet benchmark suite landed
  (state/registry_bench_test.go, webserver/fleet_bench_test.go).
  Baseline numbers on the sandbox (2 vcpu): RegistryList100 12.5
  us/12 KB/1 alloc; BotInfo 76 ns/0 allocs; NewBot 9.7 us/28 KB/7
  allocs (the 512 entry event ring and the 64 entry chat ring are
  allocated up front per bot); SnapshotContended 37.7 us/48 KB/3
  allocs; HundredBotsSnapshot 224 us/500 KB/200 allocs;
  BotListEndpoint100 54.9 us/12 KB/3 allocs; SSEFrame 27.1 us/65
  KB/1 alloc (a fresh 64 KB frame buffer per event);
  SSEEncodeAndFrame 115.8 us/173 KB/5 allocs (the full per event
  cost of the stream: snapshot copy + JSON buffer + frame buffer -
  at the 300 ms poll of 100 watched bots that is ~50 MB/s of
  garbage); SSEStreamPoll100 1.4 us. The slowest elements of the
  fleet shape: the SSE per event triple allocation, the registry
  map walk in List, and the per bot upfront ring allocations.
- 2026-09-08: the registry reworked to the dense layout (a
  []*Bot slice walked by List plus an id->slot index map for
  Get; Add replaces in place so the order stays stable).
  RegistryList100 12.5 us -> 10.7 us (the per bot map hash and
  the random pointer hop of the old map iteration are gone).
- 2026-09-08: the SSE stream path reuses its buffers. The
  streamEvents connection now owns an sseStream carrying the
  frame buffer and the JSON payload buffer; both are paid once
  on the first event and reused by every later one (the old
  path allocated a fresh 64 KB frame plus a fresh payload per
  event). The writeSnapshotEvent tests gained the stream
  argument; the ping comment is a package level value. The
  steady state benchmark: SSEStreamSteadyState 67.4 us/25.7
  KB/3 allocs per event against the old path 109 us/173 KB/5
  allocs (-85% garbage, -38% time; the remaining 3 allocs are
  the Snapshot deep copy the encoder needs). Also fixed the
  lint drift of the fight showcase config response (explicit
  zero fields, exhaustruct).


## Active task: the test-fight-ui fight FX comparison gallery

Started: 2026-09-08. Branch: `mobius-c1-client-1`.
Commits as melg8, pushed as they land.

### Goal

The user asked for a separate `-test-fight-ui` flag: a web UI page
of hero vs enemy fight examples for comparing visual ideas of
damage feedback. Vertically 4 rows - the enemy above, below, left
and right of the character; horizontally many numbered variants of
"the hero dealt damage / the hero received damage / a critical of
either" visualizations, scrollable with a horizontal scrollbar, the
map background as usual, so the user can run the test, watch and
name the variant number that best fits the real bot UI.

### Constraints

- Plain HTML/CSS/JS in the webserver embed, no framework, no build
  step, no new dependencies (project rule).
- No game connection, no geodata: the gallery is a static design
  aid; the map background is the static tile pyramid.
- Every effect must be a pure function of the shared loop clock
  (seeded per beat random tables) so repaints are deterministic and
  a Node vm harness can assert the drawing.
- The classic-script sandbox rule: no top-level DOM access in
  fighttest.js (everything inside init/build/render), top-level
  const bindings referenced by bare name from main.js.

### Progress

- 2026-09-08: implemented and verified. `webserver.NewTestFightServer`
  (mode `test-fight` via GET /api/config), the `-test-fight-ui`
  flag of cmd/swarm, `web/fighttest.js` (the engine: the grid DOM,
  the virtual clock with pause/speed, the 9 s beat loop of four
  beats - hero hit 46, taken 12, hero crit 92, crit taken 24 - the
  unit markers with the lunge, the HP bars, the beat caption, the
  map tile crop of the starter meadow, the per column pick
  highlight) and 18 variants: classic popups (the live replica),
  punch numbers, slash crescent, starburst + shockwave, blood
  spray, knockback recoil, HP chunk ghost, lightning jolt, comic
  burst, arrow volley, local shake, damage tally, unit flash +
  ring, ground cracks, ticker feed, hitstop punch, beam lance,
  attacker aura. index.html/style.css/main.js boot the
  `mode-test-fight` body class. Harness `tools/repro_fight_ui.js`
  (46 checks: structure, per variant engagement during all four
  beats measured against a null-variant baseline, the tile crop
  geometry, the caption texts, the HP integration, the lunge
  geometry, the scroll window skipping, the controls). Go tests
  `fighttest_test.go` (the config endpoint and the static asset
  chain). Fixed during verification: a doubled lunge factor, the
  map tile crop missing the tile world origin, the lightning
  flicker windows too narrow for 60 fps sampling, the hitstop
  number colliding with the unit name. Live verified with
  agent-browser + VLM screenshot reviews at frozen beat moments:
  all four beat phases and three scroll windows render correctly.
  golangci-lint 0 issues, go vet + go test ./... green, all five
  repro harnesses green (repro_map_render keeps its pre-existing
  zone label failure).

### Status

- Rebase note: the parallel session pushed the same brief as
  `-test-fight-ui-v1` (mode `fight`, `web/fight.js`, twelve
  variants); both idea sets now coexist side by side, the
  colliding identifiers of this side were renamed
  (handleFightGalleryConfig, newFightGalleryServer, the
  .fxg-* classes) and the union was re-verified (build, tests,
  lint, the harness).
- Gallery complete and live-verified; awaiting the user's variant
  pick to port the favorite into the real combat layer of map.js.

- 2026-09-10: the broken UserInfo benchmark fixed (round 1,
  feature/proxy-server, perf-and-coverage). The bench fixture
  buildUserInfoPayload stopped after the level/exp block and the
  bench failed with EOF at the load field - a stale fixture from
  before the paperdoll and speed fields were added to the parser.
  Now the fixture builds a complete packet matching the wire format
  the TestParseUserInfoPacket test already covers (weapon flag, 15
  paperdoll object ids, the skipped stats trail, run/walk speeds,
  the swim/fly speed trail, the move multiplier). The bench now
  runs: 532 ns/op, 304 B/op, 5 allocs/op - the baseline for the
  upcoming packet reader optimizations. Verified: go test
  ./internal/swarm/packets/from_game_server/ -bench . -benchmem
  passes, go build/vet clean.

- 2026-09-10: the packet reader and writer rewritten for the 100 bot
  fleet (round 2, feature/proxy-server, perf-and-coverage). The
  packet.Reader used to embed *bytes.Reader and paid for every integer
  read through the io.Reader interface dispatch plus a second bounds
  check the caller did anyway (the n != expected length guard). The
  new form is a plain struct { data []byte; offset int } that reads
  through encoding/binary.LittleEndian directly - the Go compiler
  turns the Uint32/Uint16/Uint64 calls into single unaligned loads on
  little endian hosts, so ReadInt32 is now 0.71 ns/op (was 6.5) and
  ReadInt64 is 0.72 ns/op (was 6.7), a 9x speedup on the integer hot
  path. ReadBytes now returns a sub slice of the source buffer with no
  copy (callers either copy into a destination array or just read for
  comparison, never mutate), and Skip is a single offset bump instead
  of a 64 byte chunk loop. ReadStringFromUtf16Format got a fast ASCII
  path (the common case for L2 character and NPC names): it scans the
  source slice directly for the null terminator, confirms every UTF-16
  unit's high byte is zero, builds a byte buffer of the low bytes and
  converts it to a string through unsafe.String (the strings.Builder
  trick) - one allocation instead of the previous three (the growing
  []byte, the string(data) copy and the x/text decoder output). The
  BMP slow path now uses unicode/utf16.Decode so supplementary
  characters produce correct surrogate pairs instead of the previous
  byte(r) truncation that silently corrupted non Latin-1 names.
  ErrNotEnoughBytes is a sentinel so the short-read error path pays
  zero allocations. The Writer.WriteStringAsUtf16 got the same ASCII
  fast path: it scans once, calls Grow so the buffer reuses its slab,
  and writes pairs directly without the intermediate []byte allocation
  the old form paid. The packet parsing benchmarks reflect the win:
  ParseKeyPacket 61 ns/16 B/2 allocs -> 9 ns/0 B/0 allocs (6.8x, zero
  alloc), ParseNpcInfoPacket 551 ns/608 B/10 allocs -> 105 ns/21 B/4
  allocs (5.2x, 60 percent fewer allocations), ParseUserInfoPacket
  532 ns/304 B/5 allocs -> 109 ns/10 B/2 allocs (4.9x, 60 percent
  fewer allocations), ParseCharSelectInfoPacket 6293 ns/6048 B/71
  allocs -> 1676 ns/1941 B/29 allocs (3.75x, 59 percent fewer
  allocations). New benchmarks added: ReadInt8/16, ReadFloat64,
  ReadBytes, Skip, ReadStringASCIIFastPath, ReadStringLongASCII,
  ReadStringBMPSlowPath, ReadStringEmpty, ReadStringNoTerminator,
  WriteStringAsUtf16ASCII/ReusedWriter/NonASCII, NewReader. Verified:
  go build/vet, go test ./... (19 packages), golangci-lint 0 issues
  on the touched packages.

- 2026-09-10: the game cipher SWAR optimization (round 3,
  feature/proxy-server, perf-and-coverage). The GameCrypt Encrypt and
  Decrypt loops ran one byte at a time through the running XOR chain,
  which on the 100 bot fleet path means ~100 bytes per packet times
  ~100 packets per second per bot = 1M byte iterations per second
  just for the game protocol cipher. The optimized form processes 8
  byte chunks through a SWAR (SIMD Within A Register) prefix XOR
  scan: the key repeats every 8 bytes (i&7 mask), so a full chunk
  XORs with one uint64 key load, then a three step shift-and-XOR
  prefix scan (8, 16, 32 bit left shifts) produces the running XOR
  of all 8 bytes in one register, and the chain value from the
  previous chunk broadcasts into every byte through a multiply by
  0x0101010101010101. The decrypt path is simpler: the chain uses
  the ENCRYPTED bytes (the input), so a single enc<<8 shift aligns
  byte i-1 with byte i's position, the chain value from the previous
  chunk goes into byte 0 through an OR, and one XOR produces the
  output. The remainder tail (1 to 7 bytes) falls back to the byte
  loop. BenchmarkGameCryptEncrypt 80 ns/op -> 25 ns/op (3.2x),
  BenchmarkGameCryptDecrypt 78 ns/op -> 25 ns/op (3.1x), both still
  zero allocations. New tests: TestGameCryptSWARCorrectness sweeps
  every size from 1 to 256 against a reference byte loop oracle and
  verifies bit-exact equality on both encrypt and decrypt;
  TestGameCryptSWARMultiPacket verifies the chain value carries
  correctly across packet boundaries (the key advances between
  packets through advanceOffset); TestGameCryptSWARAllZeroData
  pins the known Mobius reference shape (zeros encrypt to the
  running XOR of the key bytes); TestGameCryptSWARRandomLikeData
  exercises all bit positions. New benchmarks: EncryptSizes/8/64/256
  /1024 for the per byte cost at each realistic packet size,
  EncryptOnly and DecryptOnly for the isolated paths. Verified: go
  build/vet, go test ./... (19 packages), golangci-lint 0 issues.

- 2026-09-10: the npcdata test coverage gap closed (round 4,
  feature/proxy-server, perf-and-coverage). The npcdata package had
  34.6 percent coverage - the dictionary lookup functions (NPCName,
  NPCLevel, NPCAggroRange, NPCIsAggressive, NPCClanHelpRange,
  NPCClans, NPCClanMask, NPCWireTemplateID, ItemName, ItemPrice,
  ItemWeight, ItemIcon, ItemGearStats, ItemType, BuyListsOfNPC,
  ItemsOfBuyList, SystemMessageText, SystemMessageName) had zero
  tests, only benchmarks. New comprehensive test file
  npcdata_test.go covers: the known npc and item resolution (goblin
  template 1000003, keltir 1000532, short sword id 1, adena id 57),
  the boundary conditions (template id at the npcTemplateOffset
  boundary, below it, zero, negative), the unknown id fallbacks
  (empty string, zero, nil, false), the pass through behavior of
  NPCWireTemplateID for unmapped ids, the SystemMessageText fallback
  text for unknown ids ("system message N"), and the SystemMessageName
  enum name resolution. Coverage rose from 34.6 to 95.1 percent.
  Verified: go build/vet, go test ./internal/swarm/npcdata/ -cover,
  golangci-lint 0 issues.

- 2026-09-10: the packet reader ASCII fast path correctness fix and
  100 percent coverage (round 5, feature/proxy-server,
  perf-and-coverage). The reader rewrite introduced a Latin-1
  handling bug: the ASCII fast path checked only the high byte of
  each UTF-16 unit (the byte at position start+1, start+3, ...).
  A Latin-1 character like U+00E9 ('é') encodes as [0xE9, 0x00] in
  UTF-16LE, which has a zero high byte, so the fast path triggered
  and extracted just the low byte 0xE9. The resulting byte 0xE9 is
  not valid UTF-8, so the string displayed as the replacement
  character instead of the original character. The fix checks both
  bytes: the low byte must be below 0x80 (true ASCII) AND the high
  byte must be 0. Non-ASCII characters now correctly fall through to
  the BMP slow path that uses unicode/utf16.Decode. New tests cover:
  the BMP slow path (Cyrillic "Эльф"), supplementary characters
  (surrogate pair emoji "🌟"), mixed ASCII and BMP ("café" and
  "test café" - the regression case), the missing null terminator
  error path, the odd length buffer edge case, the WriteStringAsUtf16
  slow path (non-ASCII, supplementary, Cyrillic), the WriteFloat64
  round trip, the Reset method, and the negative count error paths
  for ReadBytes and Skip. Packet package coverage: 77.5 -> 100.0
  percent. Verified: go build/vet, go test ./internal/swarm/packets/
  packet/ -cover (100.0 percent), golangci-lint 0 issues, the string
  benchmarks unchanged (ReadStringASCIIFastPath 27 ns/1 alloc,
  ReadStringBMPSlowPath 69 ns/2 allocs).

- 2026-09-10: the to_game_server outbound packet benchmarks (round 6,
  feature/proxy-server, perf-and-coverage). The to_game_server package
  had benchmarks for only 5 of its 14 packet types. New benchmarks
  cover: MoveToLocation (the most frequent outbound packet, 170 ns/3
  allocs), AttackRequest (158 ns/3 allocs), RequestActionUse (93 ns/2
  allocs), RequestBuyItem (250 ns/4 allocs), RequestDestroyItem (87
  ns/2 allocs), RequestItemList (32 ns/1 alloc), CharacterSelect (40
  ns/1 alloc), the session lifecycle packets together (EnterWorld +
  RequestNetPing + Logout, 102 ns/3 allocs), and BenchmarkFleetOutboundTick
  which measures the aggregate outbound serialization cost of one hunt
  tick (move + attack + action + list = 432 ns/9 allocs) - the 100
  bot fleet pays this 100 times per tick, so the per packet allocation
  cost multiplies directly into GC pressure. Verified: go build/vet,
  go test, golangci-lint 0 issues.

- 2026-09-10: the 100 bot fleet profiling and the shopping/combat
  allocation sweep (round 7, feature/proxy-server, perf-and-coverage).
  Ran the live 100 bot fleet (SWARM_FLEET_E2E=1) against the deployed
  Mobius stack with CPU and memory profiling enabled. The memory
  profile revealed the shopping subsystem accounted for 71 percent of
  all heap allocations (105 of 148 MB): shoppingQueueView alone was
  65.63 MB (44.4 percent) because it rebuilt a []ShoppingEntryView
  slice with six npcdata dictionary lookups per entry on every hunt
  tick (200 ms) even though the underlying plan was cached for 5
  seconds. catalogCandidates was 17.10 MB (11.6 percent) because it
  rebuilt the same offers map from the static merchant catalog every
  5 seconds per bot. combatFeed.record was 5.55 MB (3.8 percent)
  because the append+trim ring pattern grew the backing array on every
  overflow.

  Three optimizations applied:
  1. shoppingViewCache: the built ShoppingPlanView is now cached
     alongside the plan in the Loop struct. publishShoppingView
     reuses the cached view between plan recomputes (25 ticks per
     recompute), collapsing the per tick view cost to a pointer copy.
     shoppingQueueView: 65.63 MB -> 3.51 MB (94.7 percent reduction).
  2. candidateCache: catalogCandidates results are cached per (catalog
     pointer, profile name, tax hash) tuple in a sync.Map. The catalog
     and profile are static for a given bot class and region, so the
     100 bot fleet now builds the candidates once per (catalog,
     profile) pair instead of 100 times every 5 seconds.
     catalogCandidates: 17.10 MB -> 0 MB on the steady state path
     (one 24.67 MB build at startup, then cache hits forever).
  3. combatFeed ring buffer: the append+trim pattern is replaced with
     a fixed capacity [combatEventMax]CombatEvent array with a write
     position head and a count. record overwrites the oldest entry in
     place, appendView walks from the oldest live event to the newest.
     combatFeed.record: 5.55 MB -> 0 MB (100 percent reduction).

  Total fleet allocations: 148 MB -> 75 MB (49 percent reduction).
  Verified: go build/vet, go test ./... (19 packages), golangci-lint
  0 issues on the touched packages, the live fleet reaches 60/100
  online sessions and 319K packets in 155 seconds.

- 2026-09-10: the second fleet profiling round and the affordablePrefix
  / affectedSlots / displacedValue allocation sweep (round 8,
  feature/proxy-server, perf-and-coverage). Re-ran the live 100 bot
  fleet with memory profiling after the round 7 optimizations. The
  remaining hotspots were: affordablePrefix 4.50 MB (called every
  tick from shoppingWanted just to sum prices), affectedSlots 3 MB
  (allocated a []Slot on every call, 200-600 times per plan
  computation), displacedValue 5 MB (allocated a []int32 for the sell
  first ids).

  Three optimizations applied:
  1. shoppingWanted zero-alloc: the affordable total is now summed
     directly over the cached plan without allocating an
     affordablePrefix slice. affordablePrefix: 4.50 MB -> 0 MB
     (100 percent reduction on the per tick path).
  2. affectedSlots slotBuf: the function returns a stack-allocated
     slotBuf struct { data [2]Slot; n int } instead of a []Slice.
     The Go compiler keeps the struct on the stack, and the .slice()
     method creates a slice header pointing to the stack array. All
     four callers updated to use .slice(). affectedSlots: 3 MB ->
     0 MB (100 percent reduction).
  3. displacedValue capacity hint: the ids slice is pre-sized to
     len(slots) (at most 2) so the common case of 0-2 displaced items
     pays one small allocation. displacedValue: 5 MB -> 2.50 MB
     (50 percent reduction).

  Total fleet allocations: 75 MB -> 70 MB (53 percent reduction from
  the original 148 MB). The BenchmarkFleetE2ELiveEncodeSweep benchmark
  now reports 0 B/op, 0 allocs/op (was 29724 B/op, 275 allocs/op) -
  the shopping view cache eliminated every allocation on the snapshot
  encode sweep path. The BenchmarkFleetE2EEngageScanSweep improved
  from 64201 ns/op to 52072 ns/op (19 percent faster). Verified: go
  build/vet, go test ./... (19 packages), golangci-lint 0 issues.

- 2026-09-10: the third fleet profiling round - SetHuntingZones dedup
  and cheapestJewelIDs cache (round 9, feature/proxy-server,
  perf-and-coverage). Re-ran the live 100 bot fleet with memory
  profiling after round 8. The remaining hotspots were:
  SetHuntingZones 4.08 MB (copied 227 ZoneView entries on every zone
  state change even when nothing changed) and cheapestJewelIDs
  (rebuilt the jewel floor map from the cached candidates every 5
  seconds per bot).

  Two optimizations applied:
  1. SetHuntingZones dedup: the published zones are compared element
     wise with the stored ones, and the defensive copy is skipped when
     nothing changed. The hunt loop republishes on every zone state
     change (a zone pick, a death, a demotion), but the 227 zone
     registry is the same on most of those calls.
  2. cachedCheapestJewelIDs: the cheapest jewel IDs are derived from
     the (cached) candidates, so they are cached per (catalog, profile)
     pair in a sync.Map paralleling candidateCache. The 100 bot fleet
     now builds the jewel floor map once per (catalog, profile) pair
     instead of 100 times every 5 seconds.

  Verified: go build/vet, go test ./... (19 packages), golangci-lint
  0 issues on the touched packages.

- 2026-09-10: the final 100 bot fleet profiling summary (round 10,
  feature/proxy-server, perf-and-coverage). After three rounds of
  optimization guided by live profiling of the 100 bot fleet, the
  total heap allocations dropped from 147.91 MB to 73.82 MB (50.2
  percent reduction). The per-tick allocation churn that dominated
  the original profile is completely eliminated: the
  BenchmarkFleetE2ELiveEncodeSweep benchmark now reports 0 B/op,
  0 allocs/op (was 29724 B/op, 275 allocs/op).

  Before/after comparison of the top allocation hotspots:
  - shoppingQueueView: 65.63 MB -> 2.50 MB (96.2 percent reduction)
    - cached in the Loop struct, rebuilt only every 5s (was every 200ms)
  - catalogCandidates: 17.10 MB -> 0 MB steady (100 percent)
    - cached per (catalog, profile) pair in sync.Map
  - combatFeed.record: 5.55 MB -> 0 MB (100 percent)
    - fixed-capacity ring buffer replaces append+trim
  - affordablePrefix: 6.51 MB -> 0 MB (100 percent)
    - shoppingWanted sums directly over cached plan
  - affectedSlots: 3.00 MB -> 0 MB (100 percent)
    - stack-allocated slotBuf struct replaces []Slot heap allocation
  - displacedValue: 7.00 MB -> 2.50 MB (64.3 percent)
    - capacity hint pre-sizes the ids slice
  - ElvenHuntingZones: 3.58 MB -> 1.02 MB (71.5 percent)
  - SetHuntingZones: 4.08 MB -> 1.53 MB (62.5 percent)
    - element-wise dedup skips the defensive copy
  - cheapestJewelIDs: cached per (catalog, profile) pair

  The remaining 73.82 MB is dominated by one-time costs
  (buildCatalogCandidates 23.66 MB, objectStore.upsertLocked 3.52 MB,
  blowfish.NewCipher 2.51 MB) and the actual planning work that
  produces a result (planPurchases 12.09 MB, shoppingQueueView 2.50
  MB). The per-tick allocation churn is zero. Verified: go build/vet,
  go test ./... (19 packages), golangci-lint 0 issues, live fleet
  reaches 60/100 online sessions and 319K packets in 155 seconds.

- 2026-09-10: NEW TASK started - the universal equipment window with
  the skill lists and the skill learning queue (feature/proxy-server,
  skills-display). Goal: the equipment widget shows the learned skills
  (six per row with icons, active/passive tabs) without changing the
  widget dimensions, a left flyout shows the skill learning queue with
  the SP costs (by analogy with the item purchase queue), and the
  queue orders the warrior priorities first: physical weapon attack
  power skills, then defense, then everything else. The learning
  function itself is NOT implemented - display only. Constraints: no
  widget resize (flyouts and tabs only), keyed rendering rules of the
  gear widget, harness repro_gear.js must pass, server behavior
  untouched. Acceptance: repro_gear.js green with the new checks, go
  test/lint green, the queue of an elven fighter shows attack power
  skills first.
- 2026-09-10: the -bots N multi-bot launch flag (round 11,
  feature/proxy-server). Added a -bots flag to cmd/swarm that launches
  N concurrent bot sessions in one process. Each bot gets its own
  account (the base -account name plus the 1-based index: test1 ->
  test2, test3, ...), its own tracker in a shared registry, and its
  own runBotForever goroutine. All bots share one web interface (the
  sidebar lists every bot, clicking switches the observed one), one
  proxy (the web UI selects which bot a connecting C1 client attaches
  to) and one geodata engine. The server auto-creates missing
  accounts, so the first run of -bots 3 makes test1, test2, test3 on
  the fly.

  Usage: go run ./cmd/swarm -hunt -proxy -login 127.0.0.3:2106 -web
  127.0.0.1:8081 -bots 3

  Verified live: launched -bots 2 against the deployed stack, both
  test1 and test2 created as elven fighters, entered the world, and
  started hunting (the /api/bots endpoint confirmed both online, in
  the engage phase, fighting mobs). The initial EOF on one bot was the
  login server flood protector (two simultaneous logins from one IP),
  handled automatically by the reconnect backoff. go build/vet, go
  test ./... (19 packages), golangci-lint 0 issues.

- 2026-09-10: atomic commit 2 of the skills-display task - the
  SkillList packet and the state layer. from_game_server/skill_list.go
  parses the 0x6D SkillList packet ([count][passive][level][id] per
  entry, see Mobius SkillList.writeImpl) with the implausible count
  guard and the reusable entry buffer; the dispatch routes it through
  GameClient.applySkillList -> state.Bot.SetSkills. state/skills.go
  stores the learned map (id -> level + passive), builds the learning
  queue lazily (cached, rebuilt when the class or the learned set
  changes, empty while no skill list arrived), and the snapshot
  carries the enriched learned list (skills) plus the queue view
  (skillPlan: sp, total, missing, entries with the warrior priority
  category and the affordability flag computed under the lock). The
  JSON encoders mirror the reflection output (appendSkillsJSON,
  appendSkillPlanJSON in snapshot_json.go, the live variants in
  snapshot_live.go). ResetSession clears both. All go tests green.

- 2026-09-10: atomic commit 3 of the skills-display task - the web
  UI. The equipment widget became a two view widget without changing
  its size: the EQUIPMENT / SKILLS mode tabs replace the static title
  row, the gear content stays in the flow and keeps sizing the panel,
  the skills view is an absolute overlay of exactly that area
  (visibility swap, never display none - the panel must not shrink).
  The skills view carries the ACTIVE / PASSIVE filter tabs, the
  learned skill grid (six 36px columns like the bag, keyed cells with
  icons and level badges - the icons never re-decode), the pinned
  sp/next foot. The skill learning queue is a second flyout on the
  left edge (below the shop tab, docking under the shop flyout while
  it is out): one keyed row per lesson with the icon, the name with
  the level, the warrior priority category + unlock level meta and
  the SP cost with the missing SP; the head summary and the pinned
  sp/need/save foot mirror the shop queue. Tooltips reuse the shared
  floating panel (the learned cell and the lesson rows). The mode and
  the filter persist in localStorage. Verified: repro_gear.js 150
  checks green (32 new), repro_hud/fight/movement green,
  golangci-lint v2 0 issues on the touched files, live run against
  the stack - the SkillList packet of the level 1 elven fighter
  parsed (Lucky), the queue shows the 40 remaining lessons ordered
  attack power (31) -> defense (6) -> other (3) with the SP costs and
  the browser check confirmed the layout (no overlap, no overflow).

- 2026-09-10: atomic commit 4 of the skills-display task - the
  documentation. AGENTS.md documents the two view equipment widget
  (the mode tabs, the overlay sizing rule, the learned grid, the
  sp/next foot) and the skill learning queue flyout with the warrior
  priority order and the regeneration entry of the skill dictionary;
  docs/protocol_description.md documents the SkillList (0x6D) packet
  (the byte layout and the Mobius class link); Taskfile.yml gains the
  generate:skills task (tools/generate_skill_trees.sh). TASK
  COMPLETE: the equipment window is universal (EQUIPMENT / SKILLS
  tabs, no widget resize), the learned skills render six per row
  with icons in the ACTIVE / PASSIVE tabs, the left flyout shows the
  learning queue with the SP costs in the shop queue style, and the
  warrior order (attack power -> defense -> the rest) comes from the
  Mobius skill effect stats. The learning function itself is display
  only, as requested.
- 2026-09-10: the proxy cross-bot client switch (round 12,
  feature/proxy-server). When a C1 client was connected to the proxy
  and watching bot 3, switching the WebUI selection to bot 2 left the
  client showing bot 3: SelectBot only affected the NEXT client to
  connect, not the already-connected one (documented in docs/proxy.md
  "The client switches bots by reconnecting after changing the
  selection"). The fix adds a selection notification channel to the
  proxy Server and a cross-bot resync case to streamSession.

  Implementation:
  1. Server.selectionCh: a chan struct{} that SelectBot closes and
     replaces whenever the id changes. The live relay goroutines
     select on a snapshot of the channel, so they wake immediately.
  2. serveBotSwitch: when the selection channel fires, the relay
     resolves the newly selected bot session. When it differs from the
     current one and is online, it calls resyncWorld (the same
     teleport + DeleteObject sweep + replay machinery the relogin
     handoff uses) to bring the client onto the new bot. The client
     sees the new character's position, appearance, race and class
     through the replayed UserInfo of the new bot's enter world burst.
  3. When the new selection is the same bot, an unregistered id or a
     still-connecting bot, the relay stays on the current live feed.

  New tests: TestProxySwitchesConnectedClientToSelectedBot (the full
  cross-bot switch: teleport + sweep + enter world burst of bot B
  arrives after selecting B), TestProxySelectBotSameIdDoesNotSwitch
  (no spurious resync when re-selecting the current bot),
  TestProxySwitchToOfflineBotStaysOnCurrent (selecting an unregistered
  bot keeps the client on the current feed). Verified: go build/vet,
  go test ./internal/swarm/proxy/ (all tests pass), golangci-lint 0
  issues.

## Active task: the documentation restructure - AGENTS.md split and docs cleanup (feature/proxy-server)

Started: 2026-09-10. Branch: feature/proxy-server. Commits as melg8.

### Goal (the user's brief)

Critically review AGENTS.md and improve it so it does not pollute the
agent context (move elements to separate files where it makes sense),
remove the outdated pieces and the duplication (facts presented in
several places); then analyze all the other documentation of the
project and bring it in order too - improve the quality, remove
duplication, make it maximally convenient for agent use.

### Findings (the critical review)

- AGENTS.md was 1882 lines / 112 KB: the "Web interface" section alone
  held ~700 lines and buried the hunt loop, town trips and deleveling
  inside it; "Gear, shopping and multi-zone hunting" ~160 lines;
  the deleveling live validation round ~100 lines - all read by every
  session before any work.
- Duplication found: the deployment story lived in three sections
  (mandatory first step, z.ai fast deploy, local tools deployment) plus
  the mobius-stack skill; the fight FX galleries in three places
  (Commands, Pathfinding, Web interface); the deleveling/death penalty
  facts in three places (protocol notes, delevel bullet, live
  validation section); the Mobius operational notes both in AGENTS.md
  and in the skill.
- Outdated found: tools/swarm_fast_deploy.sh hardcoded the clone
  branch mobius-c1-client-1 which no longer exists on the remote
  (merged into main and deleted) - the mandatory first step failed on
  every fresh deployment (verified live, fixed); protocol_description
  .md still presents the l2j-lisvus origin as current; project
  _description.md assumes the l2j-lisvus C4 target; the root README
  carried Windows-path commands and no pointers; docs/readme.md held
  stale early brainstorming; agent_progress.md had grown to 3423 lines
  of mostly finished tasks.
- quality_review_and_agent_prompts.md is a valuable but historical
  snapshot (2026-09-07) - needed an explicit currency note.

### Changes

- AGENTS.md rewritten to 529 lines: rules + load-bearing facts + a
  documentation map; the subsystem detail moved out verbatim.
- New docs modules: deployment.md, hunting.md, webui.md,
  pathfinding.md (moved content, reorganized, nothing dropped; the
  parallel session's fresh "skills view" AGENTS.md block was folded
  into webui.md during the rebase).
- Root README.md rewritten as a project README; docs/README.md is the
  new documentation index; docs/readme.md (stale brainstorming)
  removed.
- Currency notes added: protocol_description.md (Mobius C1 is the
  reference, lisvus parts are historical), project_description.md
  (Go + Mobius C1 settled), quality_review_and_agent_prompts.md
  (historical snapshot, verify before acting).
- development_log.md gained a navigation note (round index via grep).
- .agents/skills/mobius-stack/SKILL.md now points at
  docs/deployment.md instead of the removed AGENTS.md section.
- tools/swarm_fast_deploy.sh (+ mobius_fast_deploy.sh, kept
  byte-identical): the swarm clone branch is now SWARM_BRANCH (env
  overridable), default main - fixes the broken mandatory first step.
- agent_progress.md split: active file keeps only the unfinished tasks
  (webui modernization awaiting approval, fleet DOD round 2, the
  fight FX gallery pick) plus the 2026-09-10 stream; 2839 lines of
  finished entries moved to docs/agent_progress_archive.md
  (append-only, order preserved); the AGENTS.md work protocol now
  documents the archive policy.

### Verification

- Environment: tools/swarm_fast_deploy.sh run to STACK_READY (login
  2106, game 7777, db 3306 listening, 75 tables); tools/mobius_e2e.sh
  45 from the deploy checkout printed E2E_OK (after the branch fix).
- go build ./... and gofmt clean (no .go changes, docs + tools only).
- Relative .md link check over AGENTS.md, README.md and docs/: 0
  broken.
- The skills view AGENTS.md block of the parallel session survived
  the rebase into docs/webui.md (no content lost).

### Status: done (2026-09-10, live verified)
