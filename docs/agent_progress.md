# Agent progress log

Crash-safe task tracking: the current task, its full context and per-commit
progress live here (see the "Work protocol" section in AGENTS.md). Entries
are append-only; a new agent resumes the newest unfinished entry.

## Finished task: granular farm zone system (rotation, regression, web view)

Started and finished: 2026-09-08. Branch: `mobius-c1-client-1`. Commits
as melg8, pushed as they landed.

### Goal

The user asked to rework the farm square system: (1) the web UI must
show ALL farm grounds and highlight the one the bot heads to; (2) more
granular zones - smaller squares, more of them, with rotation when a
zone runs out of mobs, and the squares must match the real spawn
points (the old wide squares left part of the spawns outside and
emptied out); (3) three deaths in a zone mean the character does not
pull it - regress onto an easier ground.

### What was done

- Studied the spawn data: `ElvenStarting.xml` holds 106 spawn
  territories / 812 npcs in ten natural level bands (respawn 15-20 s);
  the old four squares (1650-3500 halves) covered only fractions of
  them. `scripts/analyze_spawns.py` (sandbox side) produced the
  territory -> centroid/radius/mob composition table the new registry
  was anchored on.
- Commit cd7828f: thirty granular zones (1000-1300 halves) in ten
  bands (elven-keltir-* 1-3 ... elven-pincer-forest 16-19), the gear
  gates of the old ladder kept at the matching levels; the picker
  keeps the current zone of the winning band (no same-band flapping),
  takes the nearest ground of an open band; the rotation (10 s of a
  mob-less square while standing central -> the nearest sibling of
  the band, `state.Bot.ZoneHasAttackable` as the raw emptiness
  reading); the death regression (3 deaths in a square -> the ladder
  caps below its band until the level changes; level change or
  delevel resets; delevel-phase deaths never count); the web map
  draws all thirty (active amber, demoted red, labels only when big
  enough on screen), the zone panel shows the death counts.
- Commit eb8f2c5: deaths count against the square they happen in (the
  position lookup) - round 1 showed an emergency logout death landing
  on a fresh session before any zone pick, where the picked-zone
  attribution silently dropped it. Pinned by
  TestZoneDeathOnFreshSessionCountsByPosition.
- The demotion also releases a manual zone override (the operator
  forced the ground, the character keeps dying in it - the regression
  walks it out instead of marching the corpse back), pinned by
  TestZoneDemotionBreaksTheManualOverride.

### Live verification (rich1, the stack up)

- Round 1 (5 min): picked Raider Trail South for the level 4/gear 199
  character standing in the old kaboo position, pathfound out of the
  fighter woods, engaged a Goblin Raider on the zone entry; a social
  add beat it to 10% -> the panic logout, the offline death (the bug
  above), the re-pick moved it sideways to Raider Field Southeast
  after the revival (death 1 of 3, below the threshold).
- Round 2 (8 min): the snapshot carried all 30 zones with the active
  highlight; 7 kills, zero deaths; the level up to 5 moved the picker
  to the goblin band automatically ("level 5 with gear 199: hunting
  Goblin Camp Southeast"); the DB logout store confirmed the level and
  the camp position (an immediate post-logout DB query can race the
  store - re-read after a few seconds).
- Round 3 (forced death attempt): the manual zone 17 (Kaboo Fighter
  Woods) through the web command worked; the character farmed the
  grunt edge of the square (3 grunt kills) without dying - the engage
  safety held. Round 4 teleported it into the pincer spider grounds
  via a DB position edit + game server restart: the spiders smashed
  it to 8% in 22 s, the emergency logout saved it (no death, the
  survival machinery works) and the walk home resumed after the
  cooldown.
- The rotation never fired live (the respawn refills the small
  squares faster than the kills empty them - exactly the intent); the
  death regression paths are pinned by the unit suite instead.

### Status

- All implemented, `go vet` + `go test ./...` green, 3 commits pushed
  (cd7828f, eb8f2c5, the override-release one), live rounds logged
  above.

## Active task: test coverage round 2 (the remaining weak packages)

Started: 2026-09-08 (third session). Branch: `mobius-c1-client-1`.

### Goal

Continue the coverage work of the finished first round. Baseline measured
with `go test ./... -cover -count=1` on the fresh sandbox deployment
(Go 1.24.4, stack deployed and verified: ports 2106/7777/3306, 75 tables,
E2E_OK):

- packet 75.0% (Skip, ReadFloat64 and NewWriterTo at 0%)
- to_game_server 64.5% (only unreachable writer error branches remain)
- webserver 66.3% (the pathfind HTTP API handlers at 0%)
- from_game_server 71.0% (ChangeWaitType and CharCreateOk parse paths,
  the truncated packet error paths of the parse functions)
- state 76.0% (twenty eight accessor and query functions at 0%)
- to_auth_server 78.1% (unreachable writer error branches)

### Constraints

Same as round 1: tests assert real behavior, no coverage gaming; the
unreachable `bytes.Buffer` writer error branches of the serializers stay
consciously uncovered (the decision of the first round holds); SPDX
headers, testify, go test + golangci-lint green.

### Acceptance criteria

- packet, webserver, from_game_server and state coverage measurably up.
- `go test ./... -count=1` green, `golangci-lint run` 0 issues.

### Progress

- 2026-09-08: packet 75.0% -> 97.8% - reader_writer_extra_test.go covers
  Skip (the 64 byte chunk loop, the whole buffer, the past end and empty
  errors), ReadFloat64 (the round trip through WriteInt64 plus the empty
  and truncated errors), ReadInt16 with a single byte and NewWriterTo
  (appends to seeded data and reads back). Lint clean.
- 2026-09-08: from_game_server 71.0% -> 98.6% -
  change_wait_type_test.go pins the ChangeWaitType parse (sitting,
  standing, the fake death default, wrong id, both truncations) and the
  CharCreateOk/ReasonText paths (all eight reason texts, the missing
  key packet result byte and tail). parse_error_paths_test.go sweeps
  every strict prefix of a valid packet through every parser family:
  a truncated packet must error, never parse and never panic (targets,
  rotations, teleport, social action, auto attack, delete/move/stop/
  validate, drop/spawn/get item, status update, net ping, attack with
  hits, char selected, char select info, user info, npc info, inventory
  update, system message with the typed parameters); CharInfo pins the
  optional clan/flag tail (cut keeps the defaults); the ItemList
  truncated entry stops the list without an error; ForEach caps the
  stored attributes; the implausible system message counts reject.
- 2026-09-08: state 76.0% -> 94.0% - bot_accessors_test.go covers the
  self accessors (object id, position with and without a character,
  walking, sitting, level, exp, under attack, dead), the object
  accessors (position, name, alive, health percent with and without
  vitals), ApplyWaitType (self transitions, other objects ignored),
  ApplyItemInfo + GroundItemByID (found, non item, removed),
  SetHuntingZone/SetHuntingZones, CountPacket, NearestAttacker (the
  closest chaser, none), ZoneHasAttackable (nil, empty, dead),
  NearestNpcByTemplates, MedianZoneMobLevel (nil, empty, median, out of
  square), the command queue (order, overflow drops the oldest, drain),
  ApplyPaperdoll/PaperdollSlotObjectIDs/InventoryItems,
  DestroyableItems ranking (destroyRank through the sort comparator),
  the ApplyAutoAttack self/unknown branches, ApplyStatusUpdate level
  and load attributes and the object separation.
- 2026-09-08: webserver 66.3% -> 94.3% - pathfind_api_test.go covers
  the pathfind HTTP API end to end over a synthetic flat region
  (GET /api/config with and without the view override, POST
  /api/pathfind found / missing cell error / invalid json, the geodata
  tile cache hit, the missing region 404, the bad tile parameters),
  toResponsePoints and downsample (short path untouched, oversized
  path keeps the shape), the geodata tile LRU cache itself (hit
  reorder, repeated put, eviction of the untouched key), the bot mode
  /api/config, Address, the SSE helpers (writePing and its write
  failure, writeSnapshotEvent unchanged/changed/no-repeat), the
  streamEvents flusher requirement, the poll driven second event, the
  shutdown closing active streams, writeJSON encode failure logging,
  the icon pack detection without a pack (404 serving), the zone
  command validation and the describeCommand fallback.
- 2026-09-08: round complete. Full verification: go vet ./..., go test
  ./... -count=1 green, golangci-lint run 0 issues. Coverage before ->
  after (go test ./... -cover, Go 1.24.4): packet 75.0 -> 97.8,
  from_game_server 71.0 -> 98.6, state 76.0 -> 94.0, webserver 66.3 ->
  94.3. Consciously left: the unreachable bytes.Buffer writer error
  branches of to_game_server (64.5) and to_auth_server (78.1) - the
  decision of the first round holds; npcdata and cmd/* stay generated/
  main packages by design.

### Status: coverage round 2 done

## Active task: test coverage round (the weakest packages)

Started: 2026-09-08. Branch: `mobius-c1-client-1`.

### Goal

The user asked to improve the code coverage. Baseline measured with
`go test ./... -cover -count=1` (2026-09-08):

- connection 38.1% (the worst real package: every game.go `apply*`
  packet handler and the whole authentificator.go login flow at ~0%)
- to_game_server 49.4% (half of the client -> game packet
  serializers untested)
- from_auth_server 57.4% (init.go parse path 0%, gg_auth partial)
- webserver 68.7%, state 75.9%, from_game_server 74.5% (mid)
- gear 85.3%, crypt 93.8%, pathfind 96.5%, helpers 100% (fine)
- npcdata 0% and cmd/* 0% are generated/main packages, skipped by design

### Constraints

- Tests must assert real behavior (round trips against the Mobius C1
  packet layouts from docs/protocol_description.md), never coverage
  gaming (no assertion-free "execute the function" tests).
- Reuse the existing harnesses: the scripted game session fake server
  of connection/game_test.go and hunt_flow_test.go patterns, the
  packets_test.go style for the packet packages.
- Every new test file carries the SPDX header; testify require/assert;
  go test + golangci-lint must stay green; benchmarks compare
  allocations only when touching packet parse code.
- Windows host: no cgo, so no -race locally (task test:race stays a
  CI/sandbox concern).

### Acceptance criteria

- connection, to_game_server and from_auth_server coverage measurably
  up (target: every package over 60% as the round goal, the biggest
  uncovered functions handled or consciously left with a reason).
- `go test ./... -count=1` green, `golangci-lint run` 0 issues.

### Progress

- 2026-09-08: baseline recorded; task started. Stack verified up
  (ports 2106/7777/3306 listening on the Windows dev deployment).
- 2026-09-08: round complete, all pushed and green. Coverage before ->
  after (go test ./... -cover):
  - connection 38.1% -> 89.9%: one scripted world session floods the
    client through every observed packet handler (UserInfo, CharInfo,
    DropItem/SpawnItem/GetItem, StopMove, MoveToPawn, Attack,
    AutoAttackStart/Stop, the rotation pair, ChangeMoveType/WaitType,
    TeleportToLocation with the Appearing confirmation, the target
    packets, ItemList/InventoryUpdate, SystemMessage, SocialAction,
    NetPing, leave world/server close/action failed/unknown id) plus a
    truncated one byte packet per known id (the parse error paths) and
    an empty frame; the tracker snapshot is asserted per packet family
    (TestGameClientAppliesWorldPackets). The client action methods
    (AttackTarget, PickupItem, WalkTo, UseItem, DestroyItem, DropItem,
    SellItems, BuyItems, RequestInventory, ActionSitStand,
    RestartAtVillage, RequestLogout) are verified through the received
    opcodes (TestGameClientSendsClientActions); the connection loss
    path of Run (TestGameClientReportsConnectionLoss) and the creation
    refusal (TestEnsureCharacterReportsCreationFail) included. The full
    login flow runs against a scripted fake login server
    (authentificator_test.go: Init, RequestAuthLogin, LoginOk,
    RequestServerList, RequestServerLogin, PlayOk with per field
    assertions, plus LoginFail, empty server list and unexpected init
    id rejection).
  - from_auth_server 57.4% -> 92.6%: InitPacket.WriteTo round trips
    through ParseInitPacket (with and without the Blowfish key, plus
    the too small destination), NewInitPacket zero values.
  - to_game_server 49.4% -> 65.3%: the MoveToLocation (0x01),
    RequestActionUse (0x45) and RequestRestartPoint (0x6D) serializers
    with byte exact layouts and the constructors.
  - BUG FIXED (found by the coverage work): a zero length frame
    (size header 2, no payload) panicked the run loop at
    handleServerPacket payload[0]; the empty payload is skipped now
    (game.go, covered by the flood session).
  - Lint fallout of the parallel zones commit (cd7828f) fixed on the
    way: the stale PickHuntingZone nolint removed, the
    maybeRotateEmptyZone guard chain carries a reasoned nolint, the
    zero zone returns use the noZone var, zones_test require.Empty.
    golangci-lint run: 0 issues. One rebase conflict against the
    parallel session resolved keeping both sides (containsPoint +
    noZone).
  - Consciously left uncovered: the unreachable writer error branches
    of the packet serializers (packet.Writer wraps bytes.Buffer and
    never errors), the 25 s ping ticker branch (would need an injected
    clock in connection), the tracker==nil branches of the apply
    handlers, the RequestLogout send-fail branch.

### Status: coverage round done

## Finished task: combat safety of the hunt loop (survivability round)

Started and finished: 2026-09-08. Branch: `mobius-c1-client-1`. Commits
as melg8, pushed as they landed.

### Goal

The long live session exposed five survivability defects of the hunt
loop: the bot initiated a fight it could not win (135/267 health, a
level 10 pull, death), a low health character under attack had no way
out except dying, the third hunting zone was too big, the bot marched
to the zone center past killable mobs, and it ignored the social clan
mechanics (pulling packs).

### What was implemented (commit order)

- `460a579` npc clan data: the generator extracts clanHelpRange and
  the clan list of every npc (plus the isAggressive=true default fix -
  the Mobius NpcTemplate defaults it true, the Kaboo Orc Fighter
  attacks on sight), npcdata exposes NPCClanHelpRange/NPCClans.
- `9a6026a` (rebased over the lint agent commits) state: the target
  search gains the level cap and the social fence - mobs above the
  character level slack and mobs whose clan mates stand within the
  help range (ALL clan matches everything, 600 z distance blocks the
  assist, projected positions, 200 unit margin) are never initiated
  on; the WorldObjects track the clan data of their templates.
- `e565361` hunt combat safety: the losing-fight escape (under 25%
  health or a 25+ percent target health lead under 60%, a two minute
  target skip, paced escape legs away from the threat, the beaten
  target is finished instead), the hurt-under-attack flee, the
  no-target center patrol after a 6 s patience, per-entry skip
  expiries.
- `28c9f84` panic logout: critical health (12%) under attack ends the
  session - one last escape leg, the RequestLogout packet plus the
  socket close (the server stores a mid combat character 15 s after
  the combat ends), a three minute login cooldown on the tracker that
  survives sessions, honored by the runBotForever supervisor.
- `e4d032e` zone entry: the return phase engages the first valid
  target inside the zone instead of walking to the center past it.
- `2d85d32` zones: the kaboo woods square shrunk onto the fighter
  camps (half 3200 -> 2000, center 35400 48300, 39% of the old area).
- `bb0cb5f` (found live) zone switches drop the stale farm spot: the
  live round 1 exposed a 3 second return loop after a zone switch -
  the returns aimed at the farm spot of the OLD square; the switch
  now clears it and both return paths guard against an out-of-zone
  farm spot.

### Live verification (rich1, level 4, the running stack)

- Round 1 (goblin zone switch): the trip flow, the one-item-per-slot
  buys and the gear swaps all work unchanged; the stale farm spot
  loop observed here produced the fix above.
- Round 2 (keltir return + zone switch back): "engaging Gremlin on
  the zone entry" - the return ended on the first target inside the
  zone; zero farm-spot loop lines; 23 kills, +667 exp, health
  regenerating between fights, no deaths; the panic logout never
  fired (nothing pushed the character that low).
- Rounds 3-4 (kaboo woods, the suicide scenario): a level 4 character
  inside the kaboo square and the fighter camps NEVER initiated a
  fight (0 kills, 0 hits landed, full health) - the level slack and
  the social fence hold the line exactly where the reported death
  happened.
- tools/mobius_e2e.sh: E2E_OK.

The escape and panic-logout reaction layers are pinned by unit tests
(TestLoopEscapesALosingFight, TestLoopEscapesTheLevelGapFight,
TestLoopFinishesTheBeatenTarget, TestLoopEscapesWhenHurtUnderAttack,
TestLoopLogsOutAtCriticalHealthUnderAttack,
TestLoopDoesNotPanicLogoutWhileDeleveling) - the live Mobius camps did
not aggro the character on demand (the aggressive fighters are sparse
and the passive camps ignore a standing character).

## Active task: golangci-lint v2 migration (lint toolchain repair)

Started: 2026-09-08. Branch: `mobius-c1-client-1`.

### Goal

Make the strict linter set runnable again on the Windows dev host: the
installed golangci-lint v1.64.8 cannot read the Go 1.27 standard library
export data ("export data version 4 is greater than maximum supported
version 2"), so `golangci-lint run` fails with bogus typechecking errors
and the repo has no working hygiene gate (the code itself is fine - the
errors are a tool/format mismatch). Identified in
`docs/quality_review_and_agent_prompts.md` as P03; the user asked for the
migration to be performed directly.

### Constraints

- Keep the strict linter set (the review found the config to be a quality
  asset): migrate, do not slim down.
- Fix real code findings the newer linters surface instead of disabling
  the linters, matching the repo conventions.
- No behavior changes; `go build ./...`, `go vet ./...`,
  `go test ./... -count=1` must stay green.
- Update AGENTS.md (Windows tooling caveats) after the migration works.

### Acceptance criteria

- `golangci-lint version` reports v2.x, `golangci-lint run` exits 0 on
  the whole repository.
- `.golangci.yml` is in the v2 format; no v1 leftovers.
- Tests stay green; AGENTS.md and this file updated.

### Progress

- 2026-09-08: committed `docs/quality_review_and_agent_prompts.md` (the
  architecture/quality review with the P01-P14 agent prompts written in
  the previous session; it was left uncommitted).
- 2026-09-08: migration complete, the lint gate is green.
  - golangci-lint 2.13.2 installed via
    `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`
    (reads the Go 1.27 stdlib export data that v1.64.8 could not).
  - `.golangci.yml` migrated to the v2 format (the `run.deadline` key
    blocks `golangci-lint migrate`, removed first); the strict set is
    preserved, comments restored by hand. Documented config decisions:
    gosec G115 excluded (wire-parser integer conversions, guarded by the
    parser capacity checks), cyclop max-complexity 15 (gocyclo dropped -
    it duplicated cyclop at 30), funlen 65/45, exhaustive
    `default-signifies-exhaustive`, test-file relief for errcheck/
    exhaustruct/goconst/noctx/dogsled/funlen/lll/prealloc, cmd/geotest
    exempt from forbidigo (it is a stdout tool), net.Dialer excluded
    from exhaustruct (listing its deprecated fields trips SA1019).
  - testify v1.4.0 (2019) -> v1.12.1: the testifylint autofix rewrites
    `require.Greater(t, x, 0)` into `require.Positive(t, x)`, which the
    pinned v1.4.0 did not compile. The upgrade is the honest fix.
  - The generated npcdata files now carry the canonical single-line
    `// Code generated ... DO NOT EDIT.` marker (the old two-line
    markers were not recognized, so linters flagged item_icons.go ~340
    times); all six generator scripts emit the canonical line.
  - ~790 accumulated findings fixed (the strict set had not run since
    the Go 1.27 toolchain broke): ~130 stale `//nolint` directives
    dropped, explicit zero initialization for every production struct
    literal (exhaustruct), the gear bodypart string masks extracted
    into constants (goconst), dead code deleted (connector.go + tests,
    crypt random_unique bench, three dead Engine methods), long lines
    wrapped, a real bounds check added to the webserver geodata tile
    range parse (G109), the SA4010 dead `names` append of the shop buy
    logging now actually logs the item names, the G602 slice guard in
    the gear planner, gosec G703/ireturn/unparam nolinted with reasons.
    The five complexity monsters (hunt engage 29, tickUserAttack 22,
    ParseUserInfoPacket 17, ParseNpcInfoPacket 16, fightDelevelGuard
    16) carry reasoned `//nolint:cyclop` markers pointing at the
    planned P07 refactor.
  - Verification: `golangci-lint run` 0 issues, `go vet ./...` clean,
    `gofmt -l .` clean, `go test ./... -count=1` all 13 packages ok.
  - Known follow-ups: the `exhaustruct` -> `exhaustruct_v5` rename (the
    v2.13 deprecation; the v5 major flags 50 new sites and needs its
    own round), the crypt dead Encryptor/Decryptor stack deletion and
    `task` binary install (P03 remainder), `-race` in the test task
    (P01).
- 2026-09-08: the parallel session pushes kept colliding with this one
  (two rebase rounds, one real conflict in hunt/shopping.go resolved
  keeping both sides' behavior). Git conventions in AGENTS.md gained two
  rules for the multi-agent workflow: rebase before every push (linear
  history, no merges, no force-push, re-verify after a conflict) and
  always commit as melg8 <public.melg8@gmail.com> (checked repo-locally
  before the first commit of a session).
- 2026-09-08 (later): P01 executed on top of the migration - the two
  verified data races are fixed. `sendPacket` now encrypts inside the
  writeMu critical section (the rolling XOR chain requires the
  encryption order to equal the wire order), guarded by
  `TestGameClientConcurrentSendKeepsCipherOrder` (a mirror-cipher drain
  that fails without the race detector too). The pathfind layerPool
  guards itself with a RWMutex (intern writes under the engine lock
  while concurrent searches read), guarded by
  `TestEngineConcurrentSearchesRaceFree` (2x2 synthetic regions, cache
  thrash). `-race` needs cgo+gcc, which the Windows host lacks: the new
  `task test:race` runs the suite where cgo exists. P03 remainders
  closed: the dead crypt framing stack deleted (Serializable moved into
  login_crypt.go) and `task` 3.53.1 installed. Four agent playbooks
  added under `.agents/skills/` (go-verify-loop, webui-harness,
  packet-recipe, mobius-stack), AGENTS.md gained the "Agent skills"
  pointer section; the round 31 entry is in the development log.
- 2026-09-08 (evening): vendored the samber/cc-skills-golang collection
  (46 `golang-*` skills, MIT, pinned commit 19a0626a) into
  `.agents/skills/` as the general Go knowledge base next to the four
  hand-maintained project playbooks. New tool `tools/install_agent_skills.sh`
  (install pinned / latest / check drift, exit 1 on drift) manages the
  vendored dirs only; AGENTS.md documents the commands. A fresh
  environment receives the skills through git clone alone.

## Task (completed): gear auto-equip, shop buying strategy, multi-zone hunting

Started: 2026-09-08. Branch: `mobius-c1-client-1`.

### Goal

Three features for the swarm bot (elven fighter start, elven village area,
designed to generalize later):

1. **Gear auto-equip.** If the inventory contains an item strictly better
   than the equipped one for a paperdoll slot, the bot swaps it in. If an
   item fits a paperdoll slot that is currently empty, the bot equips it.
   The invariant is maintained continuously while the bot works (after
   inventory-changing events: loot, buy, craft, sweep, downgrade after
   death penalty if items are lost).
2. **Shop buying strategy.** A documented, implementable strategy for
   buying weapons and armor from the village weapon/armor merchants so the
   character can keep leveling efficiently and move to stronger mobs that
   match its level (spend adena at the right time on the right slots).
3. **Multi-zone hunting.** Additional hunting zones for higher levels
   (mobs ~5-7, ~10+, gated by gear/level readiness), all visible on the
   web map with the currently active zone highlighted; the hunt loop picks
   zones according to level and gear. Zones must be data-driven and
   region-extensible (orcs, dwarves, dark elves, humans) and the design
   must not preclude a mage variant later.

### Constraints

- Server integrity rules (AGENTS.md): never patch the server; the bot
  adapts. Only logging patches allowed.
- Mobius C1 protocol quirks go into AGENTS.md protocol notes, not into
  server fixes.
- Keep code in `internal/`; `tools/` is the only supported way to run the
  stack. Tests and linters must pass (`task check:all` equivalent:
  `go vet`, `go test ./...`, golangci-lint if available).
- Frequent atomic commits, each pushed immediately, progress appended here
  per commit.

### Acceptance criteria

- Bot with empty slots auto-equips fitting inventory items; bot with
  better inventory items swaps them in; state stays consistent after
  buy/loot/death events (unit tested; live-checked via logs and web UI).
- Buying strategy documented in `docs/` and implemented as bot logic that
  visits the merchant and buys per the strategy (live verified: adena
  decreases, items appear in inventory and get equipped).
- Hunt zones: at least 3 zones for the elven lands (starter 1-4, mid 5-7,
  10+ when gear allows), selectable, visible on the map with the active
  one highlighted; hunt loop moves between them by level/gear rules
  (live verified: bot hunts in the zone matching its state).
- All designed with an eye to generalization: zone registry is
  data-driven; no hardcoded elf-only assumptions in the zone selection
  core.

### Progress

- 2026-09-08 03a398b: task started; work protocol (atomic commits +
  progress file) added to AGENTS.md and pushed. Environment already
  deployed and verified (login :2106, game :7777, db :3306, E2E_OK from
  the earlier session).

### Next

- Study the codebase: `internal/swarm/state` (inventory/paperdoll
  tracking), `internal/swarm/hunt`, `internal/swarm/packets` (equip and
  shop packets, what C1 sends), `internal/swarm/webserver` (map), then
  design the gear scoring model.

- 2026-09-08 01e4c8d: item gear stats layer done - generate_item_stats.sh now also emits bodypart, weapon type and combat stats (pAtk/mAtk/pDef/mDef/sDef/rShld/pAtkSpd) for all 1204 equippable items; npcdata exposes GearStats.
- 2026-09-08 c9b9d45 + 2412508: the UserInfo parser now reads the 15 paperdoll object ids (the only source that tells which ear/finger slot an equipped jewel occupies) and state.Bot tracks them (ApplyPaperdoll/PaperdollSlotObjectIDs/InventoryItems).
- 2026-09-08 gear package + hunt equip manager: melee fighter scoring profile (weapon = pAtk x attack speed, armor = pDef, jewel = mDef, shield = expected block value), NextUpgrade planner (empty slot fill, strict slot swap, pair swap through freeing the worse jewel first - the server replaces the LEFT slot blindly, two hand weapon and shield interplay guards, one-piece chest+legs family guards), TotalGearPoints zone gate metric. The hunt loop runs maybeEquipGear every tick, sharing the confirmation gate with manual useItem commands (markInventoryAction) so the two never race. Unit tests cover every planner branch. Feature 1 (auto-equip) implemented; live verification pending the e2e run.

- 2026-09-08 89ba9e7 + e6ddae0: shop strategy executed end to end - tools/generate_shop_catalogs.sh emits all 433 buylists of 185 merchants keyed by packet template id (CT0 display ids); gear.PlanPurchases is the greedy value-per-adena planner (fillers first, weapon upgrades when rich, nothing the inventory already carries); RequestBuyItem packet (opcode 0x1F, listId + itemId/count entries); the town trip became multi-stop: sell junk at the nearest merchant, re-plan with the fresh adena, walk to every merchant of the plan, select and buy one buylist per transaction (11s flood protector pacing), then walk home.
- 2026-09-08 multi-zone hunting: hunt/zones.go registry (elven: keltirs L1-4 gear 0, east goblins L5-7 gear 40, west kaboo woods L8-12 gear 110, southwest dryads L13-18 gear 200), PickHuntingZone gates on level AND TotalGearPoints, the loop re-evaluates every 30s between fights, the manual "zone" web command (index in the Count field) overrides until the character outgrows the band; snapshot carries huntingZones with the active marker; map.js draws every zone (active amber, future dimmed with gear gate labels); the sidebar zone panel has a hunt button per zone. Feature 3 implemented; live verification pending.

- 2026-09-08 097ef99: live verification round completed (bot run with -hunt, test1 given 5000 adena as a DB test fixture before the login). Findings and fixes: (1) the buy requests fired without the confirmed merchant selection - approachMerchant now waits for SelfTargetID == merchant before reporting ready; (2) the buylist requests must respect the transaction flood protector (10 s, window only extends on allowed requests); (3) the "inventory light again" log spammed every tick - now logged once per planning. LIVE VERIFIED END TO END: the shopping trip planned 12 items for 3879 adena, walked to Ariel and Creamees, bought every item (adena went from 4417 to exactly 538), the auto equipment wore each arrival within seconds (Leather Cap, Necklace of Magic, earrings, gloves, Leather Shield, rings, Short Sword swap) and looted drops too; the zone registry gated correctly (level 1-4 in keltirs with gear 59-147), the manual zone command (POST kind "zone" count 1) switched to the goblin camp with the zone leash engaging, and the graceful shutdown (SIGINT) works. The standard e2e (tools/mobius_e2e.sh) passes: E2E_OK.

### Status: all three features implemented and live verified

- Feature 1 (auto-equip): done - unit tested + live verified on loot and purchases.
- Feature 2 (shopping strategy): done - docs/shopping_strategy.md documents the strategy, gear.PlanPurchases implements it, the multi-stop town trips execute it live (12 items, exact budget).
- Feature 3 (multi-zone hunting): done - 4 elven zones with level+gear gates, auto switching (30 s re-evaluation between fights), map display with the active highlight, manual selection via the web UI.
- Extension design: gear.Profile for mage classes, per-region merchant lists and zone registries, per-town tax rates.

## Active task: shopping/trip behavior fixes from the first long live session

Started: 2026-09-08 (second session). Branch: `mobius-c1-client-1`.
Commits from melg8 (git author set to melg8 + noreply email per user
instruction).

### Goal

Three behavioral regressions observed in the first live run of a rich
character (40k adena):

1. **Redundant same-slot purchases.** The planner bought the whole
   upgrade chain in one walk: Knife + Short Sword + Sickle together,
   and both Necklace of Magic and Necklace of Knowledge while only the
   better one ever got worn - pure adena waste.
2. **Vendor walk mid-combat.** The bot left a mob alive and ran to
   sell (the trip trigger fired during the fight).
3. **Sell-after-farming instead of sell-on-arrival.** The bot walked
   to the farm spot with a bag of sellable junk and only later
   returned to town for the sale; buy trips also skipped the selling
   when the inventory was below the 50 percent trigger.

### Progress

- 2026-09-08 cd638c9 + 6a8ba3a (gofmt): fix 1 - PlanPurchases now
  marks the paperdoll slots every purchase fills or clears
  (affectedSlots mirrors the family logic: lrhand owns both hands,
  onepiece owns chest+legs, legs-vs-onepiece, lhand-vs-lrhand) and
  skips candidates writing into a marked slot: ONE item per slot per
  trip, the chains are cut. Regression tests pin the 40k scenario
  (one weapon, one necklace). The rich-weapon test now expects the
  Short Sword (value pick), not the Long Sword chain; the
  broadsword-in-inventory test expects the Dirk (best value upgrade).
- 2026-09-08 1b5b31d: fix 2 - tick() runs the trip start behind
  fightBusy() (a living target, a pending loot pickup or an incoming
  hit blocks it), so a trip only starts in the between-fights window;
  maybeEquipGear moved before the trip dispatch so bought gear is worn
  during the trip (equipped items are never sellable - also protects
  fresh purchases from the sell loop). Test: TestTripWaitsForTheFightToEnd.
- 2026-09-08 bbf216d: fix 3 - tickTownSell sells while junkRemaining()
  (unsold sellable items) instead of while inventoryFull(): every
  vendor trip sells ALL accumulated junk, batch after batch, whatever
  started the trip. recoverFromDeath clears tripEndedAt: a revival in
  the village sells at once even when a recent finished trip armed the
  5 minute cooldown (no more farm-first-sell-later after death).
  standUpBeforeTrip: a resting (sitting) character stands up before
  the trip walk (move requests are refused while sitting; the toggle
  shares the pending-transition gate with rest() so the two never
  double toggle). Tests updated: the full flows now sell both batches;
  new: TestShoppingTripSellsJunkBelowTheTrigger,
  TestTripStandsUpBeforeWalking.
- 2026-09-08 docs: shopping_strategy.md rule updates (one item per
  slot per trip; sell everything on every vendor visit; no
  mid-combat trips).

- 2026-09-08 502e746 (rebased over the f8ba5b0 review docs): the live
  verification round (rich1, 40k adena, naked gear) found a fourth
  defect - the FIRST buy after the sells fired within the transaction
  flood window (which is 10 game ticks = 1 second, not 10 seconds -
  FloodProtectorAction/ GameTimeTaskManager reading) and the server
  refused it silently: the planned Short Sword was never delivered
  while the stop reported it done. Fixes: (1) the buy pacing now waits
  out the last sell batch as well (max(buyAt, sellAt) + buyPause);
  (2) every sent buy batch waits for its arrival confirmation
  (buysArrived: the bought item ids show up in the inventory) and is
  re-requested up to stopBuyRetries = 3 times before the trip skips
  it - refused transactions (flood, range races, selection resets)
  all answer without referencing the request, so the inventory is the
  only reliable confirmation source; (3) approachMerchant gates on the
  3D distance (the server INTERACTION_DISTANCE 250 covers x, y and z
  together; the old separate 2D/z limits allowed a 283-unit stand-off
  where every transaction is refused). Test:
  TestStopBuyRetriesAndSkipsLostBatch; the buy flow tests simulate the
  arrival confirmations.
- 2026-09-08 LIVE VERIFIED (second run, rich1): the shopping trip sold
  the whole 22%-weight junk bag (only the non-sellable starter items
  stay - the server flags them is_sellable=false, the sell offers them
  once and tolerates the refusal), waited out the sell pacing, bought
  10 items from 3 merchants - one per slot - with every batch
  confirmed ("1/6/3 purchases confirmed"), the Short Sword arrived and
  swapped the Squire's Sword within seconds, the Leather set and the
  Necklace of Knowledge upgrade (ONE necklace, the Magic one sold
  context) equipped during the walk, and the hunt resumed at the farm
  spot (level 3, gear 189). No mid-combat trip starts, no redundant
  same-slot purchases, no lost transactions. The starter item
  non-sellability (Dagger, Squire's set: is_sellable=false in the
  Mobius item xml) is a server rule, the bot tolerates it by design.

### Status

- All four fixes implemented, unit tested (go vet + go test ./...
  green), pushed and live verified on the running stack.

## Active task: webui bot status banner and walk path view

Started: 2026-09-08. Branch: `mobius-c1-client-1`.

### Goal

The user asked to add to the webui (a) the display of the path drawn
from the multiple pathfind elements (the planned geodata waypoints of
every walking phase, not only the manual move) and (b) a small status
banner in the bot widget showing what the bot is doing right now
(hunting, running to a spot, running to sell items, deleveling). The
banner must coexist with concurrent edits of other models on the
same branch (rebase before every push, never force-push).

### Constraints

- The web UI is plain HTML/CSS/JS without a build step (project rule,
  see AGENTS.md): no framework, no bundler, no npm. New fields flow
  through the existing snapshot endpoint and the SSE stream.
- Adding a snapshot field follows the best practice path: track it in
  state, copy it into Snapshot and BotInfo, render it in app.js,
  cover it with a harness check, document it in AGENTS.md.
- The hunt loop phase is the source of truth for the activity banner.
  The phase must publish through a defer so every return path of
  tick() updates the tracker (a plain defer call evaluates its
  arguments at registration time, so a closure that captures l.phase
  by reference is required).
- The walk plan view must publish on every walking phase (phaseUser,
  phaseTownWalk, phaseTownReturn, phaseDelevel) and clear on the non
  walking phases (engage, loot, townSell, idle). The town trip and
  the deleveling share the l.waypoints slice and l.wpIndex cursor
  through startWalkLeg, so a single geodataWalkPlanTail covers them.
- exhaustruct, funlen, gci, lll: the strict lint set of .golangci.yml.
  The tick() function was already at the funlen limit (45 statements),
  so the new defer pushed it over - the town trip handling was
  extracted into handleTownTrip() to bring it back under.

### Acceptance criteria

- The webui shows a small status banner (top center chip on the map)
  with the current bot activity (hunting, walking to town, selling,
  walking to farm spot, deleveling, manual move, idle). The sidebar
  bot row carries the same activity text under the name.
- The webui draws the planned path of every walking phase: the manual
  move, the town trip walk to the trader, the town trip walk back to
  the farm spot, the deleveling guard walk. The destination marker
  (pulsing blue dot) draws at the last waypoint.
- go test ./... green, golangci-lint run --new 0 issues, every repro
  harness passes (repro_hud, repro_gear, repro_map_render,
  repro_movement; the pre-existing hunting zone label failure of
  repro_map_render is unrelated and was already failing before this
  task).
- The new harness check covers the phase to label mapping of every
  hunt loop phase and the renderBotStatus DOM update.
- AGENTS.md documents the new fields and the new banner.

### Progress

- 2026-09-08: state.Bot gains a phase field, SetPhase/Phase methods
  and the Snapshot/BotInfo payloads carry it. ResetSession clears it
  for the next login. The same phase refresh is a no-op so the per
  tick call never churns the event stream. Covered by
  state.TestSetPhase.
- 2026-09-08: hunt loop publishes the phase through a defer on every
  tick; the town trip handling was extracted into handleTownTrip to
  keep tick under the funlen limit. publishWalkPlan now covers every
  walking phase: the manual move (phaseUser), the town trip walk and
  return (phaseTownWalk, phaseTownReturn) and the deleveling guard
  walk (phaseDelevel) publish their remaining geodata waypoints with
  the leg destination last. Covered by hunt.TestTripWalkPlanPublishes
  (the existing TestUserWalkPlanPublishesAndClears still pins the
  manual move plan).
- 2026-09-08: webui gains the bot status banner (#bot-status in
  index.html, renderBotStatus + phaseLabel + userPhaseLabel in
  app.js, the .bot-status CSS). The sidebar bot row gains the
  activity line (botActivityLabel). The banner colors by activity
  through data-kind; the dot pulses while active. The pathfind test
  mode hides it. Covered by the new repro_hud checks (every hunt
  phase -> label/kind mapping, the connecting/offline fallback, the
  detail text).
- 2026-09-08: AGENTS.md documents the new bot status banner and the
  walk path view extension. All checks green: go test ./... ok,
  golangci-lint run --new 0 issues, repro_hud ALL PASS, repro_gear
  OK, repro_movement PASS (repro_map_render has the pre-existing
  hunting zone label failure, unrelated).

### Status

- All changes implemented, unit tested, harness checked, lint clean
  (new), documented. Pushed to mobius-c1-client-1 as melg8.

## Finished task: two-attacker logout, 30 s pause, combat animations

Started and finished: 2026-09-08. Branch: `mobius-c1-client-1`. Commits
as melg8, pushed as they landed (rebased over the parallel banner
commit of the lint agent).

### Goal

The user asked for three changes: (1) log the bot off when two or
more mobs aggro on it, and cut the reconnect cooldown to 30 seconds;
(2) an attack animation for the bot and for the mobs plus pretty
damage animations on both; (3) everything on the shared branch with
the concurrent edits of the other models respected.

### Implementation

- `state`: the tracker gains a combat animation feed
  (`combatEvents`, `CombatEvent`/`CombatEventView`, bounded to 64
  entries, 2 s snapshot window, monotonic `seq`). `ApplyAttack`
  records every swing with the attacker and hit-target placement;
  the HP deltas of `ApplyStatusUpdate` record the damage landings
  (heals record nothing - the Attack broadcast carries no damage
  value, the delta is the observed truth). `SelfAttackerCount` reads
  the live aggro load (living attackable npcs holding the character
  as their target - a chaser counts the same as a swinger).
- `hunt`: the emergency logout fires on EITHER critical health under
  attack OR `SelfAttackerCount >= 2` (the social pile up - the gate
  stays one-shot behind `logoutDone`); the login cooldown drops from
  3 minutes to 30 seconds (the combat stance plus the mob reset walk
  home; the farm stops idling for minutes). The log line names the
  actual trigger.
- `web/map.js`: the combat animation layer. Fresh `combatEvents`
  replay as canvas effects deduped by `seq` across the SSE
  snapshots: swings draw a windup swoosh at the attacker, a colored
  streak shooting to the target (light blue for the own attacks, red
  for the mob ones) and a white impact starburst; damage numbers pop
  in with an overshoot, rise and melt (amber on mobs, red on the
  character) with a flash ring under them; a hit on the character
  flashes the map edges red. The effects track the interpolated
  runtime positions and `needsMoreFrames` keeps the render loop
  alive while they live.

### Verification

- Unit tests: `state/combat_events_test.go` (the attacker count
  semantics, the swing feed, the damage feed incl. no-heal rule, the
  feed bound) and two new hunt tests (the two-mob logout with its
  30 s pause, the single-attacker fight never logging out); the
  existing critical-health logout test re-pinned to the 30 s pause.
  go vet + go test ./... green.
- Live: rich1 hunted the goblin camp - the snapshot carried
  `combatEvents` with live attack/damage beats (amounts 10-24, mob
  ids and positions) and the map page played them (the browser
  probe saw the anims list filling, `seq` advancing, zero page
  errors). Screenshots during real fights caught the floating
  damage numbers; controlled injections of every effect kind were
  each confirmed visually by a vision model (the blue bot swing, the
  red mob swing pointing at the character, the amber -47 on a mob,
  the red -31 with the flash ring on the character, the impact
  starburst).
- tools/mobius_e2e.sh 45: E2E_OK with the new code path.

### Status

- All three requested behaviors implemented, unit tested, live
  verified (feed + rendering), documented in AGENTS.md, pushed to
  mobius-c1-client-1 as melg8.

## Active task: survivability and web UX round (flee logout, zone view, shopping)

Started: 2026-09-08. Branch: `mobius-c1-client-1`. Commits as melg8,
pushed as they land (rebase before every push - other models edit the
branch concurrently).

### Goal

The user asked for eleven changes: (1) log out when fleeing from mobs
too long instead of running forever; (2) a brighter demonstration of
the inactive hunting zones plus a show/hide checkbox; (3) stop the bot
movement when the user switches the hunting zone manually; (4) move
the hunting zones into a collapsible list (the left sidebar holds
bots only); (5) fix the target search so a big zone with far mobs
never leaves the bot standing; (6) fix the resting softlock (user
clicks while sitting -> repeated Action failed, the bot never stands);
(7) recalibrate the zone level and gear gates so the bot hunts mobs
1-2 levels below itself; (8) an aggro radius display for aggressive
mobs (toggleable); (9) shopping counts the sell value of the replaced
item and sells it before buying the replacement; (10) remove the red
screen edge flash; (11) the attack animation only plays on hits that
actually land (single hit per event).

### Progress

- 2026-09-08: (7) the zone ladder recalibrated. A band now opens only
  above its top mob level plus `zoneLevelLead` (1) - the character
  hunts mobs 1-2 levels below itself; the gear gates of the elven
  registry raised (wolves 20, raiders 50, goblins 70, grunts 100,
  fighters 140, lieutenants 180, leaders 220, elders 260, spiders
  300, lirein 380); the below-every-band fallback became the starter
  band contest (the nearest ground of the first band, the current
  ground keeps its post - no re-pick bouncing for sub-band
  characters). Zone tests re-pinned to the new gates. AGENTS.md
  documents the new calibration.

- 2026-09-08: (1) the flee episode budget. `fleeSince` tracks the
  start of the running flee; past `fleeLogoutAfter` (20 s) without
  shaking the chase the loop calls the emergency logout (the 30 s
  login pause resets the aggro), a recovered health or a fresh
  fight clears the episode. Covered by
  hunt.TestLoopLogsOutWhenTheFleeNeverShakesTheChase.
- 2026-09-08: (5) the far target walk. The engage pick failure
  (nothing inside the 1500 engage radius of a big square) now falls
  through `walkToFarTarget` before the center patrol: the nearest
  valid in-zone mob is looked up with a 6000 unit radius and walked
  toward one paced leg per second, so a far pack is closed on
  instead of the bot standing central with an empty radius. Covered
  by hunt.TestLoopWalksToFarTargetsOfABigZone (and the negative:
  a mob inside the radius is engaged directly).
- 2026-09-08: (6) the resting softlock fix. `tickUser` stands a
  sitting character up before executing a manual move/attack/pickup
  (the server refuses every request of a sitting session with a
  bare ActionFailed - the old loop spammed refused walk requests
  and never stood up). Covered by
  hunt.TestUserMoveStandsUpTheRestingCharacter.

- 2026-09-08: (3, 9, 11 docs round) the previous commit landed the
  code of the zone switch stop, the sell-first shopping and the
  hit-only swing feed; this commit carries the AGENTS.md
  documentation of all three (the shopping strategy section, the
  zone command stop, the combat animation layer) - the doc patch of
  the previous commit failed its pattern match and the code went out
  alone.

## Active task: benchmark suite and data oriented optimization of the tracker hot paths

Started: 2026-09-08. Branch: `mobius-c1-client-1`. Commits as melg8,
pushed as they land (rebase before every push - other models edit the
branch concurrently).

### Goal

The user asked for benchmark tests over the code base, the slowest
elements identified from their results, and those elements eliminated
with data oriented design (dense arrays, cache friendly layouts,
precomputed flat data instead of per call allocations).

### Constraints

- The behavior of the tracker must not change: the existing unit
  tests of state, hunt and webserver stay green untouched.
- The stack was deployed and verified (STACK_READY, ports 2106/7777/
  3306, 75 tables) before the work started.
- Benchmarks follow the repo conventions: `_bench_test.go` files,
  `b.ReportAllocs()`, `for range b.N` (go.mod pins go 1.23).

### Measured baseline (the slowest elements)

- `state.NearestAttackableConstrained` 316 us / 200 npcs - the
  `avoidSocial` path rescans the whole objects map per candidate
  (O(N^2) with 200 byte struct copies out of the map).
- `webserver` snapshot JSON marshal 249 us, 120 KB, 104 allocs per
  event (encoding/json reflection over the full snapshot, one per
  version change - per received packet with a web client).
- `state.Snapshot` build 27 us, 48 KB, 6 allocs (map iteration with
  struct copies, sort.Slice reflection over the inventory).
- `state.MedianZoneMobLevel` 5.9 us, 3 allocs (sort.Slice).
- npcdata map lookups 5.2 ns each, `NPCClans` 60 ns + 1 alloc
  (strings.Split per NpcInfo packet).

### Progress

- 2026-09-08: benchmark suite added: `state/bot_bench_test.go`
  (Apply paths, target searches, zone scans, snapshot build),
  `npcdata/npcdata_bench_test.go` (dictionary lookups),
  `hunt/zones_bench_test.go` (zone picker), `webserver/
  server_bench_test.go` (snapshot JSON marshal). Baseline recorded
  above. Pushed as the first atomic commit.

- 2026-09-08: data oriented rework of the tracker storage and the
  target search landed. `state.Bot.objects` is now a dense
  `[]WorldObject` array with an `objectIndex map[int32]int32`
  (removals swap the last record into the freed slot): the packet
  apply paths mutate the records in place (no more 200 byte struct
  copies through the map per packet) and every scan walks the memory
  sequentially. The socially constrained target search
  (`nearestAttackableSocial`) flattens the living attackable npcs
  into compact `npcScan` records (one projection per npc, squared
  distances, no sqrt per pair) and checks the clan pull through
  precomputed clan bitmasks (`npcdata.NPCClanMask`, bit per clan of
  the sorted alphabet, the ALL marker bit) instead of nested string
  loops. `NPCClans` returns the pre-split shared dictionary entries
  (no strings.Split per NpcInfo packet). `sort.Slice` ->
  `slices.Sort`/`slices.SortFunc` in the median and the inventory
  ordering, `NearestNpcByTemplates` dropped its per call map
  allocation. Benchmark deltas (200 npc world):
  NearestAttackableConstrained 316 us -> 10 us (31x),
  SelfAttackerCount 2765 ns -> 212 ns (13x),
  NearestGroundItemExcluding 2678 ns -> 230 ns (12x),
  MedianZoneMobLevel 5.9 us/3 allocs -> 1.3 us/1 alloc,
  NPCClans 60 ns/1 alloc -> 5 ns/0 allocs. Full suite green, lint
  0 issues (the three pre-existing exhaustruct findings of the
  combat event literals included explicit zero fields, the swing
  loop moved into recordAttackSwingsLocked for the funlen limit).
  Pushed as the second atomic commit.

- 2026-09-08: (2, 4, 8, 10) the web round. The inactive hunting
  zones draw in a bright soft blue with a light fill (the future
  grounds read at a glance instead of barely visible dimmed hints)
  and the new `hunt zones` toolbar checkbox hides the whole layer;
  the aggression radius of the living aggressive mobs draws as a
  dashed circle (amber idle, red in combat) behind the `aggro`
  toolbar checkbox; the red map edge flash of a hit on the character
  is removed (the damage numbers and the swing streaks carry the
  combat, the blinking screen only distracted); the hunting zones
  moved out of the left sidebar into a floating collapsible panel on
  the map (bottom right, collapsed by default with a count chip -
  the sidebar lists bots only). Harness checks added: the map render
  harness covers the bright inactive square, the aggro circle and
  both toggles; the hud harness covers the zone panel collapse and
  render. All harnesses green (repro_map_render keeps its
  pre-existing zone label failure).
## Follow-up task: the Squire's starter kit destruction and the official-only server source

Started and finished: 2026-09-08. Branch: `mobius-c1-client-1`.
Commits as melg8, pushed as they land.

### Goal

Two user requests: (a) destroy the Squire's starter pieces
automatically once their replacements are worn (they are unsellable,
undroppable dead weight); (b) the server code may come ONLY from
the official GitLab repository - no outdated copies - and the
GitLab download attempts continue until they succeed.

### Progress

- 2026-09-08: (a) done and pushed. `gear.ReplacedStarterItems`
  detects the replaced kit pieces (equal or better scored item on
  their paperdoll slot, the one-piece chest displaces the starter
  pants), the hunt loop destroys them behind the shared
  confirmation gate with equip-first ordering and a 10 s retry
  pacing. Round 32 of the development log; tests in
  `gear/starters_test.go` and `hunt/starters_test.go`.
- 2026-09-08: (b) done and pushed. The 3 month old GitHub mirror
  checkout was deleted; the official master `43ac8878` is deployed
  (git clone with retries, then the official archive API for the
  rate-limited blob fetch, the checkout grafted back into a real
  git repo with `git fetch` working); the database was reloaded
  from the official SQL (74 -> 75 tables). `tools/swarm_fast_deploy.sh`
  implements the two official channels, AGENTS.md bans mirrors in
  the server integrity rules. Round 33 of the development log.
  `tools/mobius_e2e.sh 45` prints `E2E_OK` on the official stack.

- 2026-09-08: the final verification round. go vet + go test ./...
  green (14 packages), golangci-lint run back at the two
  pre-existing exhaustruct debt items of the damage literals (the
  swing literal got a scoped nolint, ApplyAttack's funlen overage
  resolved by extracting recordSwingEventsLocked, the parallel
  session's benchmark/starter-kit files lint-repaired as a drive-by:
  tab formatting, the lll splits, an unconvert). All four repro
  harnesses green (repro_map_render keeps its pre-existing zone
  label failure). tools/mobius_e2e.sh 45: E2E_OK, the bot entered
  the world and shut down gracefully. Live probe with -hunt -web:
  the level 1 character hunted the starter keltir meadow (the
  recalibrated picker), the snapshot carried 30 zone views with the
  active marker, the aggressive objects carried their aggroRange
  (the Newbie Helper 1000) and the combatEvents feed flowed with
  attack and damage beats through real fights.

### Status

- All eleven requested behaviors implemented, unit tested, harness
  checked, live verified, documented in AGENTS.md and pushed to
  mobius-c1-client-1 as melg8: (1) the flee episode budget ends the
  session through the emergency logout; (2) the inactive zones draw
  bright with a fill behind the hunt zones checkbox; (3) the manual
  zone selection stops the running walks; (4) the zones live in the
  collapsible map panel, the sidebar is bots-only; (5) the far
  target walk closes on packs beyond the engage radius; (6) the
  manual clicks stand a resting character up before acting; (7) the
  zone gates open one level above the band top with raised gear
  gates; (8) the aggression radius circles toggle behind the aggro
  checkbox; (9) the replacement purchases sell the displaced gear
  first and count its credit; (10) the red screen edge flash is
  removed; (11) the swing feed only records the hits that landed.

- 2026-09-08: the snapshot JSON encoding moved off the reflection
  path. `state.Snapshot` now carries a hand rolled append writer
  (`snapshot_json.go` + `snapshot_json_encode.go`): a linear field
  walk over the flat arrays producing the exact bytes of
  encoding/json (field order, ES6 float formatting, HTML escaping,
  RFC3339Nano times, null for nil slices) - pinned byte for byte
  against the reflection encoder by
  `TestSnapshotJSONMatchesReflection` and the string/float/time
  tables over the edge cases (HTML chars, control bytes, U+2028/29,
  invalid UTF-8, 1e-6/1e21 boundaries). The SSE stream
  (`writeSnapshotEvent`, the initial event of `streamEvents`), the
  state endpoint (`handleBotState`) and the SSE event frame
  (`writeEvent`, one pre-sized buffer) use `AppendJSON` directly;
  `json.Marshal(Snapshot)` keeps working through `MarshalJSON` for
  compatibility. Benchmark deltas (200 npc snapshot):
  stream encode 249 us/104 allocs -> 127 us/4 allocs (2x faster,
  26x fewer allocations); with the snapshot build 273 us -> 155 us.
  Profile note: json.Marshal over a MarshalJSON implementation pays
  a full compacting scan of the output (~450 us for 100 KB), which
  is why the production paths must call AppendJSON directly.
  AGENTS.md documents the layout contract. Full suite green, lint
  0 issues. Pushed as the third atomic commit.

- 2026-09-08: task wrap up. The final verification round: go build,
  go vet, go test ./... (14 packages green), golangci-lint run
  (0 issues), the benchmark sweep re-measured, and the live E2E
  (tools/mobius_e2e.sh 45) prints E2E_OK against the deployed
  stack. Final numbers (before -> after, 200 npc world unless
  noted): NearestAttackableConstrained 316 us -> 9.3 us (34x),
  SelfAttackerCount 2765 ns -> 217 ns (13x),
  NearestGroundItemExcluding 2678 ns -> 229 ns (12x),
  MedianZoneMobLevel 5.9 us/3 allocs -> 1.2 us/1 alloc,
  ApplyNpcInfo 371 ns/2 allocs -> 263 ns/1 alloc, NPCClans 60 ns/1
  alloc -> 5.1 ns/0 allocs, snapshot stream encode (SSE path) 249
  us/104 allocs -> 135 us/4 allocs (1.85x faster, 26x fewer
  allocations, including the snapshot build 273 us -> 136 us).
  The task is complete: benchmarks added, the slowest elements
  identified from their results (the O(N^2) social scan, the
  reflection JSON marshal, the map based object storage, the
  per packet dictionary allocations), and each eliminated with
  data oriented design. All work pushed to mobius-c1-client-1 as
  melg8 (four commits: the benchmark suite, the dense storage and
  social scan rework, the direct JSON writer, the wrap up entry).
## Follow-up task: the fight FX variant showcase (-test-fight-ui-v1)

Started and finished: 2026-09-08. Branch: `mobius-c1-client-1`.
Commits as melg8, pushed as they land.

### Goal

The user asked for a test command that demos the hero versus enemy
fight with different damage visualization ideas: four enemy positions
(top, bottom, left, right) stacked vertically, the animation variants
horizontally behind a scroll bar, the map background as usual and a
number per variant, so the winner can be picked by looking at the
running page.

### Progress

- 2026-09-08: done. `-test-fight-ui-v1` boots the bot less `fight`
  web mode; `web/fight.js` renders twelve numbered variant columns
  (floating numbers, comic pop, slash, sparks, shockwave, HP chunk,
  arrow, hit-stop, cell flash, dizzy stars, arcade banner, combo
  counter) x four enemy positions on the real map tile, all cells on
  one shared scripted fight clock (hit / take / crit / take crit /
  regen). The command name carries the v1 suffix per the user
  request - a future v2 idea set can coexist. Round 34 of the
  development log; `webserver/fight_test.go` pins the mode handshake
  and the static shell; live verified in the headless browser (12
  columns, 48 cells, zero console errors, screenshots in
  download/fight_shots/).


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

- 2026-09-08: the state god object split into components.
  state.Bot (2448 lines) held every concern: identity, character
  vitals, world objects, inventory, the event ring, the chat ring,
  the combat feed, zones, walk plans and the snapshot build. The
  storage mechanics moved into dedicated types with the bot as the
  locking facade: objectStore (dense world storage, the density
  invariant lives in it), eventLog + chatLog (rings that allocate
  lazily on the first record) and combatFeed (bounded animation
  feed with the TTL window read). The world scans moved to
  scans.go, the combat record helpers to combat.go. The lazy rings
  cut the per tracker footprint: NewBot 9.7 us/28 KB/7 allocs ->
  1.2 us/2.5 KB/5 allocs (a hundred idle trackers hold 250 KB
  instead of 2.8 MB). All state tests pass unchanged; the scan
  benchmarks stay at the previous round numbers (200 npc
  NearestAttackableConstrained 9.7 us, snapshot 26.7 us/3 allocs).
  AGENTS.md documents the new component layout contract.
- 2026-09-08: the connection god file split. game.go (1624 lines)
  held the session flow, the command senders, the opcode routing
  and thirty apply paths of one GameClient. The dispatch layer
  (handleServerPacket routing, the unknown packet log, the net
  ping keepalive) moved to game_dispatch.go and the world packet
  apply paths (one apply function per packet family, parsing into
  the reusable scratch structs) to game_apply.go; game.go keeps
  the client struct, the session flow and the command senders
  (852/247/560 lines). No behavior change; all connection tests
  pass unchanged, lint clean.
