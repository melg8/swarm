# Agent progress log

Crash-safe task tracking: the current task, its full context and per-commit
progress live here (see the "Work protocol" section in AGENTS.md). Entries
are append-only; a new agent resumes the newest unfinished entry.

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
