# Agent progress archive

> FROZEN ARCHIVE (2026-09-22, the merge clash policy round, issue
> #33): the pre-split archive, closed for appends - finished task
> fragments live in `docs/progress/archive/` now (one file per
> task, see docs/progress/README.md). Read for history only.

Finished task entries and completed progress streams of
`docs/agent_progress.md`. Append-only, chronological order preserved
from the original file. Nothing here is lost context - the durable
summaries live in the docs/ modules (hunting.md, webui.md,
deployment.md, proxy.md, shopping_strategy.md) and the root cause
history in `docs/development_log.md`; this archive keeps the full
task-level detail (goal, constraints, acceptance criteria, per-commit
progress) for reference.

## Active task: the looted gear of the shopping list survives the junk flows

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.

### Goal

The user request (2026-09-10, Russian): verify that when an item that
is on the shopping list drops for the bot, the bot does not sell it
for its instant adena - it puts it on and uses it.

### Root causes and fixes

- The purchase side was already safe: `PlanPurchases` plans against
  the simulated paperdoll (`SimulateInventory`), so a looted item id
  the inventory carries is never bought twice ("nothing gets bought
  that the inventory already carries" - pinned by
  `gear` `TestPlanPurchasesSkipsInventoryItems`).
- The sell side was NOT safe: `state.Bot.SellableItems` lists every
  unequipped non-adena non-quest item, with no knowledge of what the
  auto equipment is about to wear. The rescue was pure timing - the
  auto equip request (paced 2 s, confirmed through the shared gate)
  usually flips the equipped flag before the first sell batch leaves.
  The race windows: the two-step pair swap (the better jewel waits
  for its use request while the displaced piece already came off),
  the confirmation window of an in-flight equip, a refused equip.
  Reproduction: `hunt` `TestLootedGearSurvivesTheSellStop` failed -
  the looted Short Sword went out in the first `SellItems` batch on
  the very tick the equip request was sent.
- The destroy side was worse: `DestroyableItems` ranks gear drops
  FIRST (before common stackables), so the overflow cleanup
  (70 percent slots) destroyed a freshly looted unequipped upgrade
  before the junk mats. Reproduction:
  `TestLootedGearSurvivesTheCleanupDestroy` failed - the destroy
  batch ate the sword.
- The fix introduces the planned equip keep set:
  `gear.PlannedEquips(profile, equipment)` collects the object ids
  the gear simulation places on the virtual paperdoll but that are
  not equipped yet - exactly the pending wearables the auto
  equipment walks through step by step. The hunt loop caches the set
  per inventory mutation (`equipManager.keepsCache`,
  `Loop.plannedEquipKeeps`) and passes it to the new junk filters
  `state.Bot.SellableItemsExcluding(keep)` and
  `state.Bot.DestroyableItemsExcluding(keep, limit)`; the plain
  methods delegate with nil. The kept pieces: looted upgrades,
  bought arrivals waiting for their paced equip, the better halves
  of pair swaps mid flight. Still junk (correctly): duplicates,
  looted downgrades, the displaced weaker halves of pair swaps.
- Three hunt tests pin the behavior end to end: the sell stop keeps
  the looted sword (the batches sell around it, the trip still
  completes), the overflow cleanup destroys the stackables behind
  the kept sword, and the pair swap window sells the displaced
  apprentice earring while the looted mystic earring survives and
  wears.

### Status: done (2026-09-10)

- Verify loop: go build/vet, gofmt clean, go test ./... (18
  packages, 0 failures), golangci-lint 0 new issues in the touched
  files (the pre-existing goconst on slots.go, the gofumpt on
  version_test.go and the nolintlint/unparam findings in untouched
  files remain).

## Active task: the shop strategy rework - the purchase phases (jewel floor, weapon, defense)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push (7 commits landed mid task: the packet reader/writer perf
rounds; pulled cleanly, no conflicts).

### Goal

The user request (2026-09-10, Russian): the purchase order of the NG
items is wrong for the starting locations. (1) Nothing there attacks
with magic - the cheapest first jewel set suffices until level 15+,
the jewel ladder is a waste below it. (2) The melee characters want
the weapon first, then the armor with the maximum defense, then the
next weapon tier. Rework the purchase order logic, study the server
prices, and deliver the comparison table of every NG purchase from
level 1 to 15+ as "was" and "is".

### Root causes and fixes

- The old planner ranked EVERY purchase by `gain / price` (greedy
  value per adena). The cheap empty slot fillers (Apprentice's Shoes
  8 pDef for 8 adena - 0.99 pDef per adena) outranked every weapon,
  so a fresh character spent levels 1-4 on shoes, gloves, caps and
  shields before the first Short Sword, bought the intermediate
  weapon ladder (Heavy Chisel -> Knife -> Sickle) whose steps resell
  at reference/2, and climbed the jewel ladder in magic-free zones.
- The rework phases the walk (`shopStrategy.classify` in
  gear/shopping.go): the jewel floor (the cheapest jewel per family
  fills the empty slots at any level - the basic outfit), the weapon
  milestone (only the best value STRICT weapon upgrade is eligible -
  the saving target; a cheaper worse value weapon never intercepts
  the save up) and the defense upgrades (ranked by the raw defense
  gain, bounded by the weapon budget: the reference value of the
  worn defense gear may not exceed the reference price of the worn
  weapon - the weapon leads the progression, the defense follows
  inside its tier budget). The jewel upgrades gate on level 15
  (`jewelUpgradeLevel`): below it only the floor items are planned,
  past it the upgrades join the defense phase.
- `PlanPurchases`/`PlanPurchaseQueue` grew the character level
  parameter (the hunt loop passes the tracker's `SelfLevel`), the
  virtual paperdoll entries carry the item id (the anchor and the
  defense pricing read them), and the whole file went through
  gofmt (the working tree copy had lost its tabs).

### Status: done (2026-09-10)

- The journey simulation test
  (`gear/shopping_strategy_test.go`:
  TestShoppingStrategyJourneyComparison) walks the elven fighter
  from the creation screen (the Squire's kit, zero adena) through
  level 20 once per planner - the legacy greedy copy (pinned as the
  comparison baseline) and the phased planner - with the income
  model built from the Mobius data (experience.xml exp per level /
  the mob exp of the level's ladder step, the npc adena drops at 70
  percent), one town trip per level, the sells, the server-side
  affordability re-check (the planner overprices the starter kit
  credit the shops refuse) and the auto equipment walk. It prints
  the was/is table and pins the ordering rules: the jewel floor
  first, the first weapon before any armor, no jewel upgrade below
  15, the jewel upgrades past 15, the greedy planner's filler
  detour (10 non-weapon buys, the Apprentice's Shoes opening).
- The comparison table and the waste analysis (the filler detour,
  the intermediate weapon ladder, the jewel ladder in magic-free
  zones, the shield ladder after the two-hander) live in
  docs/shopping_strategy.md ("The was/is journey of an elven
  fighter"); the strategy section describes the three phases.
- Verify loop: go build/vet, gofmt clean, go test ./... (18
  packages), golangci-lint on gear/hunt (only the pre-existing
  goconst on slots.go and nolintlint on plan.go remain), the live
  stack deployed fresh (STACK_READY: 2106/7777/3306, 75 tables) and
  tools/mobius_e2e.sh E2E_OK.

## Active task: rest at the kill spot, finish fights across the zone line, zone free loot

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push (one push landed mid task: the standing hunter round).

### Goal

The user report (2026-09-10, Russian), three hunt behavior complaints:
(1) the character runs too far away after a fight before it sits down
to rest; (2) the character stops interacting with the mobs when the
fight carries it out of the hunting zone - it should finish them off;
(3) the character does not always pick up ground items - the drops
outside the hunting zone must be picked up regardless. The game server
stack was already up on this Windows host (login 2106, game 7777,
db 3306 verified) and had to stay untouched.

### Root causes and fixes

- The escape threat lookup fell back to the nearest living attackable
  npc when no mob held the character as its target. A finished fight
  leaves a fresh 3 s under attack window (the dying mob's last blow),
  so the hurt character armed the flee against a passive bystander
  and ran up to three 700 unit legs away from the kill spot before
  resting. Fix: `threatPosition` drops the fallback - the escape runs
  from the living engaged target or a real attacker only
  (`NearestAttacker`), otherwise the rest happens where the fight
  ended.
- The zone leash of the engage dropped the fight the moment the
  character stood outside the square (`returnToZone` cleared the
  target and walked home through the blows). Fix:
  `adoptOutZoneFight` (loop_movement.go) adopts a live fight before
  the walk home - the own living target, the fresh server selection
  or the nearest attacking chaser (never a flee-skipped target) - and
  the engage flow finishes it outside the square; without a live
  fight the leash walks home unchanged, new fights still start inside
  the square only.
- The loot search passed the hunting zone filter: drops past the
  square line stayed on the ground forever. Fix: `loot()` searches
  without the zone - anything within the 900 unit loot radius of the
  character is picked up, wherever it lies.

### Status: done (2026-09-10)

- Four new tests pin the behaviors (rest at the kill spot, the fight
  continues outside the zone, the chaser is fought back, the loot is
  picked up past the line) - all four verified to fail on the old
  code (stash round). The two flee tests grew the missing Attack
  broadcast: the fleeing mob must actually hold the character as its
  target for the escape direction.
- Verify loop: go build/vet, gofmt clean, go test ./... (18 packages),
  golangci-lint (only the pre-existing unparam on
  pathfind/search_test.go).
- Live: the bot hunted the deployed stack directly (the real login
  server at 127.0.0.3:2106 - the proxy Recipe A layout of this host;
  game 7777) for 3.5 minutes: zone pick, kill, loot and the rest 3 s
  after the kill log line - the rest happened at the kill spot, no
  escape run (fix 1 demonstrated live); the pile up safety layer
  cycled its documented run + logout + relogin when the clanned orc
  pack joined. Hard kill stop (the SIGINT pitfall), the shutdown path
  is untouched by this round. Development log Round 45 carries the
  full writeup.


## Finished task: the standing hunter - the socially fenced square and the dead zone mob priorities

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
The user report: the 2026-09-10 02:11:07 state dump of the bot test1 -
the hunter stood in the phase engage at the exact center of the
elven-2019_23-b1 square (30502 62755 -3576), full hp, zero attackers,
no combat for 49 s - "why is the bot standing and not attacking
anyone? find it, fix it, and log the opponent positions". Other agents
push to the same branch concurrently - rebased onto their commits
(c01bb65, fc82ed4) before the push.

### Goal

Diagnose the standing hunter from the dump, fix the standing (the
hunt must move again), and make the hunt log name the opponents with
their positions so a fresh "the bot just stands there" report answers
itself from the log alone.

### Root cause (the dump held the answer)

- Only two living attackable npcs stood inside the square: a Kaboo
  Orc Fighter at 31126 61892 -3560 and a Kaboo Orc Fighter
  Lieutenant at 31137 61598 -3523 - 294 units apart, both of the ORC
  clan with the 300 unit help range. The engage pick skips any mob
  whose clan mate stands within help range + 200, so the pair fenced
  each other out of the target search.
- The deadlock chain: no pick -> no far target walk -> no patrol leg;
  the plain zone emptiness reading still counted the fenced pair ->
  no rotation; the aggressive fighter sat 1065 units out (past its
  1000 aggro range) -> no incoming attack either. Standing forever.
- Bonus find during the round: the zone mob priority bias was dead -
  the registries carry the CT0 xml ids (20471) while the wire sends
  the C4 display ids + 1000000 (1000471), so the priority keys never
  matched.

### Changes (commits 312ecfa, fc2d95d)

- state: ZoneHasPickable (the pick's own filters as the emptiness
  reading) + NearestBlockedTargets (the nearest rejected opponents
  with positions and reasons); hunt: maybeRotateEmptyZone rotates out
  of a fenced square after the regular 10 s window, and
  logNoPickableTargets logs the targetless diagnostic (1 line / 5 s).
- npcdata: the generated npcInternalWireIDs map (5781 entries,
  CT0_to_C4_ids.txt) + NPCWireTemplateID; zoneMobPriority translates
  the registry ids onto the wire keys; one aggression flag resynced
  (the Uthanka Pirate).
- Tests: engage_repro_test.go (the dump scene verbatim - 22 npcs at
  their exact dump positions, the character at the zone center; the
  diagnostic line test and the rotation test), scans_blocked_test.go
  (the reading gap, the filter parity, the blocked list),
  zones_test.go (the id translation pick bias test).

### Status: done (2026-09-10, verified)

- gofmt clean, go build/vet, go test ./... (19 packages) green;
  tools/mobius_e2e.sh 60 on the fix head fc2d95d -> E2E_OK.
- The live hunt from the dump scene now rotates out of the fenced
  square and the log names the blocked opponents with positions and
  reasons - the dump's object list no longer lives only in the dump.
- Round 44 of docs/development_log.md carries the full analysis.

## Active task: the map toolbar folds into one row (zoom buttons gone, layer checkboxes in a dropdown)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
A follow-up round of the web UI polish (the shop queue flyout round
below is done and live verified). Other agents may push to the same
branch concurrently - rebase before every push.

### Goal

The user report (2026-09-10, Russian): fix the top panel - remove the
-/+ buttons and rework it, it eats too much vertical space, make it a
dropdown list. Live measurement confirmed the diagnosis: the
`bot-only` checkbox span collapsed into a six-row column (the flex
item squeezed to 84 px wide), so the toolbar stood 101 px tall on a
900 px viewport.

### Changes

- web/index.html: the `-`/`+` zoom buttons are gone (the wheel owns
  the zoom, cursor-anchored); the seven layer checkboxes (labels,
  paths, zone, targets, hunt zones, aggro, map bg) moved from the
  inline span into the `view` dropdown - a `view-menu` wrapper, the
  `view-menu-btn` button (aria-expanded, aria-haspopup, a caret glyph)
  and the `view-menu-pop` checklist under it; the follow checkbox and
  the pathfind arm controls stay inline.
- web/style.css: the toolbar keeps one row (nowrap, a 6/12 padding,
  min-height 34); the dropdown chrome (the pop absolutely under the
  button with the panel background and shadow, the open state flips
  the caret 180 degrees, a separator above the map bg row); the
  bot-only checkbox rows carry `bot-layer` and hide in the pathfind
  mode (only map bg stays), the whole toolbar hides in the fight
  modes; the scale/object counters keep `white-space: nowrap`.
- web/map.js: the zoom-in/zoom-out button listeners removed (the
  wheel path in `onWheel` was already the real zoom).
- web/app.js: `initViewMenu` - the button toggles the pop, an outside
  click or Escape closes it, the clicks inside the pop stop their
  propagation (several checkbox toggles survive one open), the closed
  state is applied at init so the markup class and the aria value
  always agree.
- tools/repro_gear.js: the document stub grew an addEventListener
  recorder; the new checks pin the markup (no zoom buttons, the
  checkboxes inside the pop), the css (the pop under the button, the
  nowrap toolbar, the pathfind bot-layer rule) and the behavior
  (toggle open/close, a pop click keeps it open, an outside click and
  Escape close it, the aria flips).
- tools/repro_hud.js: the element stub grew setAttribute/getAttribute
  (the dropdown init syncs aria-expanded at load).
- AGENTS.md: the toolbar paragraph (the map toolbar bullet) and the
  hunt zones toggle wording now describe the dropdown.

### Status: done (2026-09-10, live verified on the local stack)

- The toolbar measures 37 px tall on a 900 px viewport (was 101), a
  single row in every mode; the map wrap grew from 739 to 803 px.
- The dropdown opens under the view button (btn [275,40,61,24], pop
  [275,70,132,170]) with the seven rows; unchecking `hunt zones`
  keeps the menu open; a click on the page header and the Escape key
  both close it; the aria-expanded value follows every state.
- The wheel zoom still works (a dispatched WheelEvent on the canvas
  moved the scale 0.12 -> 0.104); the zoom-in/zoom-out buttons are
  gone from the DOM.
- The pathfind mode hides the six bot-layer rows (map bg stays) and
  keeps the 37 px single-row toolbar.
- Screenshots (closed/open/pathfind) passed a vision model layout
  check; no JS console or page errors.
- Verify loop: repro_gear (all checks incl. the eight new dropdown
  ones), repro_hud, repro_movement, repro_fight_ui pass; go build,
  go vet, go test ./... (18 packages, no failures). The one
  repro_map_render failure ("hunting zone carries the label") is the
  pre-existing map label regression noted below - re-confirmed at
  HEAD with this task's changes stashed.

## Active task: the shop queue flyout slides out to the left (the equipment panel never resizes)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
A follow-up round of the shop queue widget task (the widget itself is
done and live verified - see the entry below). Other agents may push
to the same branch concurrently - rebase before every push.

### Goal

The user report (2026-09-10): the queue must slide out to the LEFT
side by a small triangle, without expanding the original equipment
panel - not downward. The previous implementation was a collapsible
section of the equipment panel between the paperdoll and the bag, so
opening it grew the whole panel downward.

### Changes

- web/index.html: the shop panel left the panel flow - it is now the
  last child of the gear panel (absolutely positioned, so the panel
  box never grows), plus the new shop-tab button: the small triangle
  tab sticking out of the left border, top aligned with the title
  row, carrying the aria-expanded state.
- web/style.css: the tab chrome (a 14x20 tab with its right border
  merged into the panel edge, the glyph flipped by the state), the
  flyout docked at `right: calc(100% + 14px)` (the tab column is the
  gap) with a 180 ms translate + opacity + visibility slide, the head
  a static label row instead of the toggle button.
- web/app.js: `ShopPanel.open` (open by default - the passive glance
  point of the widget), the tab click toggle, the tab hides without a
  plan, the purchase tooltip anchors to the LEFT of the hovered row
  (`positionItemTooltip` grew the side parameter with the viewport
  fallback) so the queue tooltip never covers the equipment panel.
- tools/repro_gear.js: the flyout checks (the markup placement after
  the inventory, the css rules, the tab visibility with and without a
  plan, the toggle with the glyph and the aria flips; the stub
  element grew setAttribute/getAttribute).

### Status: done (2026-09-10, live verified on the local stack)

- The geometry pinned with a headless browser against the running
  stack: the tab at [1161, 149, 14, 20] against the gear panel at
  [1174, 145, 254, 395] (the tab sticks out of the left border), the
  flyout right edge lands exactly at the tab, top aligned with the
  panel; the tab click closes the flyout (the glyph flips to the
  left-pointing triangle, aria-expanded false) and the gear panel
  rect is identical before and after the toggle - the equipment
  panel never resizes, the queue overlays the map to the left.
- The queue published 9 rows live (the affordable picks plus the
  wanted tail); the hover tooltip opened left of the rows (Magic
  Ring, Creamees, M.Def 7, Gain +7, Value/adena 0.189) without
  covering the equipment panel; the screenshots of the open, closed
  and tooltip states passed a vision model layout check; no JS
  console errors.
- Verify loop: repro_gear (all checks incl. the new flyout ones),
  repro_hud, repro_movement, repro_fight_ui pass; go build, go vet,
  go test ./... (18 packages, no failures). The one repro_map_render
  failure ("hunting zone carries the label") is the pre-existing map
  label regression noted below - re-confirmed at HEAD with this
  task's changes stashed.

## Active task: the web UI shop queue widget (what the bot plans to buy next)

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was deployed with `tools/swarm_fast_deploy.sh` and verified
(STACK_READY, ports 2106/7777/3306, 75 tables, `tools/mobius_e2e.sh 45`
printed E2E_OK) before the work started. Other agents may push to the
same branch concurrently - rebase before every push.

### Goal

The web UI must show the purchase queue of the shop strategy: what the
bot plans to buy next (weapons/armor), how much adena the purchases
need and how much is still missing. The user wants to see the items the
bot wants at any moment, spot a suboptimal pick (a poor gain per adena
purchase) and report it for a strategy fix - the widget is the
inspection tool, not a steering tool.

### Plan

- `gear.PlanPurchaseQueue`: the greedy planner walk extended past the
  wallet - the affordable plan of the next trip first (byte identical
  to `PlanPurchases`), then the wanted tail (the best value per adena
  picks the adena cannot pay for yet) with the cumulative missing
  adena per entry.
- `state.ShoppingPlanView` + `SetShoppingPlan`: the tracker view of the
  queue (per entry: item, icon, merchant, price, sell credit, gain,
  affordability, missing) carried in the snapshot as `shopping`, null
  when nothing is published.
- `hunt`: the loop publishes the queue every tick (a 5 s recompute
  cache, shared with the trip trigger), and while a town trip runs the
  view switches to the remaining trip buys (the in-flight batch
  marked `buying`).
- `web`: a collapsible SHOP QUEUE section of the floating equipment
  widget - small scrollable list, hover tooltips (the rich item
  tooltip shape plus the purchase lines: gain, value per adena,
  price, sell credit, missing), a collapsed one line summary and the
  pinned need/have/missing foot. Harness checks in
  `tools/repro_gear.js`.

### Status: in progress (2026-09-09)

- Committed: the gear queue walker (`PlanPurchaseQueue`, the
  `Affordable`/`Missing`/`Gain` fields of `Purchase`, the
  `shoppingQueueTail` bound of 8) with the queue tests
  (prefix-equals-plan, wanted tail missing accounting, sell credit of
  the tail, rich wallet has no tail). Next: the state view and the
  snapshot encoding.
- Committed: the state view (`state.ShoppingPlanView` +
  `SetShoppingPlan` with the no-op republish and the 10 s expiry,
  the `shopping` snapshot field through the golden and the live
  encoders, the ResetSession cleanup) and its tests.
- Committed: the hunt publish - the tick defer republishes the
  queue view (the 5 s recompute cache shared with the trip trigger,
  the planning adena cached with it), the trip view mirrors the
  remaining trip buys with the in-flight batch marked buying, the
  manual-only sessions clear the view. The cache restructure keeps
  the trigger semantics (the affordable prefix decides).
- Committed: the web widget - the collapsible SHOP QUEUE section of
  the equipment panel (keyed rows, summary line, buy/have/save foot,
  the rich purchase tooltip with the Gain / Value-per-adena planning
  block), the harness checks in repro_gear.js, the AGENTS.md and
  shopping_strategy.md notes.

### Status: done (2026-09-09, live verified on the deployed stack)

- The full stack verified before the work (STACK_READY, 75 tables,
  `tools/mobius_e2e.sh 45` -> E2E_OK).
- The widget verified live with a hunting bot + a headless browser:
  the queue publishes (10 entries: the 2 affordable fillers and the
  wanted tail through the Ring of Knowledge), the missing amounts
  shrink with the looted adena (wallet 16 -> 44 -> the short sword
  missing 806 -> 787), the head summary (count, affordable total,
  the whole queue save-up), the buy/have/save foot, the purchase
  tooltip (Gain +758, Value/adena 0.858, Sell first 69, Missing,
  the saving up status) and the collapse toggle both ways; no JS
  console errors.
- Verify loop green: go build, go vet, go test ./... (full suite),
  go test -race on state + hunt, gofmt clean, golangci-lint (the 9
  reported issues are pre-existing: the gear goconst/prealloc and
  the version/fleete2e/icons/server/pathfind findings of the other
  commits), every web harness passes (repro_gear with the 15 new
  shop queue checks, hud, movement, fight ui).
- Note for the next agent: `tools/repro_map_render.js` fails one
  check at HEAD without any of this task's changes ("hunting zone
  carries the label - no label at the square corner") - a pre
  existing regression of the map label rendering, worth its own
  task.

## Active task: the blind engage recovery (walk around the obstacle, then switch)

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was deployed with `tools/swarm_fast_deploy.sh` and verified
(STACK_READY, ports 2106/7777/3306, 75 tables) before the work started.
Other agents may push to the same branch concurrently - rebase before
every push.

### Status: done (2026-09-09, verified live and green)

### Goal

The hunt loop locks up when a small obstacle (a column) stands between
the character and its selected target: the bot stands at melee distance
(115 units in the live dump), the server refuses every swing with
"Cannot see target." and the bot never tries to walk around the column
nor switches to another mob - a 53 minute stall on one target (state
dump: phase engage, walk plan empty, "Cannot see target" chat spam
every ~3.5 s, no events).

Root cause (verified in the Mobius C1 source):
- `Creature.onForcedAttack` sets the AI ATTACK intention WITHOUT the
  line of sight check (the canSee check there is commented out), so the
  attack stance arms (`AutoAttackStart` broadcast) and the bot tracker
  holds `SelfEngaged` true forever (the stance never stops - no swing
  ever lands, no `AutoAttackStop` ever arrives).
- `PlayerAI.thinkAttack` calls `Creature.doAttack`, whose GeoData LOS
  check fails behind the column: it answers SystemMessage 181
  (CANNOT_SEE_TARGET) plus ActionFailed and keeps the intention armed -
  the AI retries forever.
- The hunt loop's `engageStuckTimeout` (12 s) is gated on
  `!SelfEngaged`, which the stale attack stance holds true, so the
  timeout never fires and the loop re-requests the forced attack
  forever.

### Plan

Two levels of recovery, pathfinding first, target switch as the
fallback (the user requirement):

- Level A (reposition): when the tracker sees a fresh "Cannot see
  target." answer during an engage attempt that never went fresh, the
  loop samples a ring of melee-range standing points around the target,
  keeps the ones with a bot-side geodata line of sight to the target
  (pathfind.Engine.LineOfSight through the hunt Navigator), plans the
  geodata path to the nearest one and walks it with paced ground clicks
  (the engage stops re-requesting the attack meanwhile - an attack
  request would replace the walk intention and cancel the recovery).
- Level B (switch target): if no navigator/geodata is available, no
  vantage point or path exists, the reposition walk misses its budget,
  or the block persists after the attempt budget, the target is dropped
  and skipped for a delay so the search picks a different mob.
- Stuck timeout hardening: the gate moves from `!SelfEngaged` (held
  true forever by the stale auto attack stance) to `!SelfFighting`
  (fresh swings or chase steps), and a running fight refreshes the
  engage timestamp so only a genuinely dead engagement (12 s without a
  fight packet) trips the timeout; while the blind recovery is armed
  the timeout stays held.

### Acceptance criteria

- Unit tests pin all levels: the reposition walk (no attack requests
  while it runs), the arrival re-engage, the immediate switch without
  geodata, the switch after the failed reposition, the stuck timeout
  firing through the stale attack stance, and the timeout held during
  the recovery.
- The state tracker records the last CANNOT_SEE_TARGET answer time.
- go build/vet, go test ./... and golangci-lint stay green.

### Implementation

- `state/chat.go` + `state/bot.go`: ApplySystemMessage records the
  arrival of the C1 SystemMessage 181 (CANNOT_SEE_TARGET) in the new
  CharacterState.CannotSeeTargetAt (the constant
  systemMessageCannotSeeTarget), and SelfCannotSeeTargetAt exposes it.
- `hunt/town.go`: the Navigator interface gained LineOfSight (the
  engineNavigator passes the engine MaxPassableHeight through
  pathfind.Engine.LineOfSight).
- `hunt/loop_los.go` (new): blindEngageBlocked (the detection: a
  fresh refusal during an attempt that never went fresh, past
  blindEngageDelay), recoverBlindEngage (level A start/walk/hold,
  level B switch), blindVantagePoint (16 candidate ring points at
  blindMeleeRadius around the target, nearest one with a clear sight
  line), walkBlindWaypoints (paced leg following with the arrival
  handoff), switchBlindTarget (drop + blindSkipDelay skip) and
  clearBlindRecovery.
- `hunt/loop.go`: the engage inserts the recovery branch after the
  losing fight check (an add grinding a repositioning character must
  escalate into the flee, not into another detour leg); the stuck
  timeout gate moved from !SelfEngaged to !SelfFighting with the
  engage clock re-anchored while a fight runs fresh and the timeout
  held while the recovery is armed; every target drop path (death,
  flee, pile up, zone return, user commands, town trips) clears the
  recovery bookkeeping.
- `hunt/loop_los_test.go` (new): nine tests - the reposition walk on
  the planning tick (vantage ring goal, first leg, no attack
  requests), the arrival re-engage, the switch without geodata, the
  switch with no vantage/route, the walk budget switch, the retry
  budget (two attempts then switch), the stale attack stance firing
  the stuck timeout (the live 53 minute hang reproduced in a test),
  the timeout hold during the recovery, the stale refusal scoping and
  the fresh fight guard.
- `state/chat_test.go`: the refusal recording test (id 181 records,
  other ids do not, the chat line still formats).

### Verification

- go build ./..., go vet ./... clean; go test ./... green (17
  packages).
- golangci-lint run: no new issues (the one pre-existing unparam on
  pathfind/search_test.go predates this task).
- Live against the deployed stack: `tools/mobius_e2e.sh 45` printed
  E2E_OK; `BOT_FLAGS=-hunt tools/mobius_e2e.sh 90` hunted live - the
  zone entry engage, 8 kills with looting, gear equipping and the
  graceful shutdown all intact (the stuck timeout gate change did not
  disturb the healthy engage flow).
- The blind recovery itself is pinned by the unit tests (the live
  reproduction needs a geodata obstacle between the bot and a mob -
  the elven fields are open terrain; the unit tests drive the exact
  packet conditions of the dump instead).

## Active task: item status tooltips on hover (paperdoll + inventory)

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was already up (login :2106, game :7777, db :3306, 75 tables)
and re-verified before the work started. Other agents may push to the
same branch concurrently - rebase before every push.

### Goal

Add a rich, multi-line item status tooltip to the equipment widget (the
floating paperdoll + inventory panel) that shows on hover over any cell -
both the equipped slots and the bag entries. The classic L2 item tooltip
shape per family:

- Armor: name, bodypart type (head/chest/legs/...), armor type
  (LIGHT/HEAVY/ROBE), P. Def, weight, description (if any).
- Weapon: name, weapon type (SWORD/BLUNT/DAGGER/BOW/POLE/ETC),
  P. Atk, M. Atk, Atk. Spd, consumed SoulShot count, consumed
  Spiritshot count, weight.
- Jewelry (earrings, rings, necklace): name, bodypart type, M. Def,
  weight.
- Other items (potions, scrolls, materials, adena): name, item type
  (EtcItem/Asset), weight, count.

The Mobius C1 item stats XML files carry `type` (Weapon/Armor/EtcItem),
`armor_type`, `weapon_type`, `bodypart`, `soulshots`, `spiritshots`
and the stat block - they have no `description` field (descriptions
live in the client-side `itemname-e.dat`), so the tooltip shows the
description line only when one becomes available later. The `weight`
and `price` are already in the generated `itemPrices`/`itemWeights`
maps; `pAtk`, `mAtk`, `pDef`, `mDef`, `sDef`, `rShld`, `pAtkSpd`,
`weapon_type`, `bodypart` are already in `itemGearStats` for the
equippable items.

### Plan

- Extend `npcdata.GearStats` with four new fields: `Type` (Weapon,
  Armor, EtcItem), `ArmorType` (LIGHT/HEAVY/ROBE), `SoulShots`,
  `SpiritShots`. Regenerate `item_stats.go` from the Mobius XML to
  extract them. Add a new `itemTypes` map for non-equippable items so
  the tooltip shows the right `Type` (EtcItem/Asset) for potions,
  scrolls, materials and adena too.
- Extend `state.InventoryItemSnapshot` (and the live encoder
  `appendInventoryItemJSON`) with the tooltip fields
  (`type`, `weaponType`, `armorType`, `pAtk`, `mAtk`, `pDef`, `mDef`,
  `sDef`, `rShld`, `pAtkSpd`, `soulShots`, `spiritShots`, `weight`,
  `price`). The live encoder stays allocation free: the per item view
  struct lives on the call stack.
- Replace the simple `itemTooltip` `title=` attribute in `app.js`
  with a custom DOM tooltip (`#item-tooltip`) shown on `mouseenter`
  over any cell (paperdoll wear, paperdoll jewel, inventory bag),
  positioned next to the cell. The tooltip renders the family-specific
  lines (`P. Atk`, `M. Atk`, `Atk. Spd`, `P. Def`, `M. Def`, `Weight`,
  `SoulShot xN`, `Spiritshot xN`) and gracefully omits any field the
  item does not carry (an EtcItem shows only name, type, count,
  weight).

### Implementation

- `internal/swarm/npcdata/npcdata.go`: the `GearStats` struct gained
  the four tooltip fields (`Type`, `ArmorType`, `SoulShots`,
  `SpiritShots`). A new `ItemType(displayID)` accessor returns the
  XML category of any item - it reads `GearStats.Type` for
  equippable items (so a single lookup suffices) and falls back to
  the new `itemTypes` map for the non equippable items (potions,
  scrolls, materials, adena).
- `tools/generate_item_stats.sh`: the generator now extracts the
  XML `type` attribute of every item plus the `armor_type`,
  `soulshots` and `spiritshots` set values of the equippable ones.
  The generated file gained a new `itemTypes` map (the XML category
  of every non equippable item) alongside the extended `itemGearStats`
  entries. Numbers: 2557 prices, 2481 weights, 1204 gear stats and
  3027 item types.
- `internal/swarm/state/bot.go`: `InventoryItemSnapshot` carries the
  tooltip fields (`type`, `weaponType`, `armorType`, `bodyPartKey`,
  `pAtk`, `mAtk`, `pDef`, `mDef`, `sDef`, `rShld`, `pAtkSpd`,
  `soulShots`, `spiritShots`, `weight`, `price`). `fillInventorySnapshot`
  resolves them through `npcdata.ItemGearStats` and the new
  `npcdata.ItemType` / `npcdata.ItemWeight` / `npcdata.ItemPrice`
  accessors.
- `internal/swarm/state/snapshot_live.go` and `snapshot_json.go`:
  the live encoder and the reflection golden encoder write the new
  fields in the same order, byte identical (pinned by
  `TestSnapshotJSONMatchesReflection` and
  `TestAppendSnapshotJSONMatchesSnapshot`). The per item size budget
  estimate grew from 448 to 768 bytes so the one shot buffer keeps
  everything in one allocation.
- `internal/swarm/webserver/web/index.html`: a new
  `<div id="item-tooltip">` element lives at the bottom of the page
  as the singleton floating panel.
- `internal/swarm/webserver/web/style.css`: the `.item-tooltip` rules
  cover the panel (fixed position, pointer events none, themed
  background, border, shadow), the family classes
  (`.fam-weapon`, `.fam-armor`, `.fam-jewel`, `.fam-etc` tint the
  name) and the per line layout (`.tip-name`, `.tip-line`,
  `.tip-key`, `.tip-val`, `.tip-enchant`, `.tip-foot`).
- `internal/swarm/webserver/web/app.js`: the new `renderItemTooltip`
  builds the family specific payload, `showItemTooltip` /
  `positionItemTooltip` / `hideItemTooltip` drive the floating panel
  on `mouseenter` / `mousemove` / `mouseleave`, the
  `attachGearCellTooltip` call wired into `makeCellRecord` binds every
  paperdoll and inventory cell to the panel, and the
  `refreshGearCellTooltip` call inside `applyItemCell` keeps an open
  tooltip current when a snapshot mutates the underlying item (an
  equip swap mid hover). The classic `title=` attribute stays as the
  keyboard / screen reader fallback.
- `tools/repro_gear.js`: ten new harness checks pin the tooltip
  markup, the css rules, the family classification and the per
  family lines (a weapon shows P. Atk / M. Atk / Atk. Spd /
  SoulShot / Spiritshot, an armor piece shows P. Def, a jewel shows
  M. Def, an etc item shows only name / type / count and an enchanted
  weapon shows the green +N prefix). The harness exports
  `itemFamily` and `renderItemTooltip` for the test.
- `internal/swarm/state/snapshot_json_test.go`: the golden snapshot
  fixture gained the new tooltip fields on its two inventory items
  (adena and an enchanted short sword), so the byte equality pin
  covers them.

### Verification

- go build/vet, go test ./... (18 packages green), golangci-lint run
  (no new issues; the one unparam warning on `pathfind/search_test.go`
  predates this task).
- The five repro harnesses (repro_gear, repro_hud, repro_map_render,
  repro_fight_ui, repro_movement) all green; repro_gear carries the
  ten new tooltip checks; repro_map_render keeps its pre existing
  zone label failure noted in AGENTS.md.
- Live: `tools/mobius_e2e.sh 20` prints `E2E_OK` against the
  deployed stack - the bot still connects, enters the world and shuts
  down gracefully with the new tooltip data in every snapshot.
- Live check: hover the equipped sword and an armor piece on the
  running bot, the tooltip shows the expected stat lines.

## Active task: the terrace rule of the pathfinder (the city deck exit)

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
Commit: `c6461d7`. The stack was already up (login :2106, game :7777,
db :3306) and verified before the work started.

### Goal

The follow-up live report of the zone switch (a state dump attached):
the bot stood on the floating elven city deck at 42440 49032 z -2992
heading to the Spore Fungus SW-e1 zone (center 32206 49064, ground
z -3696) and ground into the city walls - "town walk stuck,
re-pathing" repeating, the remaining walk plan pointing straight west
on the ground layer. The user's read: the pathfinding has no descent
- the city edge has railings, the character cannot simply drop down;
it must head to one of the three bridges, descend the gradual ramp
and walk from the bridge to the farm. Explicitly requested: a GENERAL
solution for stacked terraces (the water below, the city above), no
hardcoded bridge routes.

### Diagnosis

- Replanned the exact dump route on the real geodata: the A* DID find
  a route - as a 920 unit drop off the deck edge at (42360, 49048)
  into the lake bed (-3912) followed by a ~10000 unit swim west. The
  geodata holds no railing walls (the deck edge cells are fully open
  NSWE - the C1 pack does not model the fence mesh), and the search
  step rule mirrored only the upward Mobius HEIGHT_INCREASE_LIMIT:
  any drop was "walkable", so the A* happily planned the jump.
- The waypoint follower then walked the long west leg straight into
  the city building walls - and its 2D arrival check had already
  consumed the drop waypoint (80 units away horizontally, 920 below),
  so the character never even walked to the deck edge it was supposed
  to jump off. Stuck, re-path, the same drop route, forever.
- Measured on the real geodata: the bridge ramps and the lake shores
  step 8..24 units per cell; the deck edge jumps 920. The terraces
  connect ONLY through the gradual ramps - that is the general
  separation the step rule was missing.

### Fix

- The symmetric step rule (`search.canStep`): the height difference of
  a neighbouring cell step must stay within the passable height in
  BOTH directions. A step beyond it is a terrace boundary, not a
  walkable connection; the descent from the city deck must route
  through the ramps. No bridge is hardcoded anywhere - the A* finds
  the south bridge on its own: deck -> (42824, 51224) -> ramp
  -3224 -> ground -3480 -> zone approach -3680, 14702 units, ~0.6 s,
  46k expansions, every raw step within the limit, no swimming.
- The 3D waypoint arrival (`waypointDistance` in hunt/town.go, used
  by the town trip and the manual walk followers): a waypoint counts
  as reached only when the character stands near it in full 3D, so a
  drop waypoint hundreds of units below is never consumed without
  being walked.
- `run()`'s straight line shortcut answers only DRY walks now
  (`directOrAstar`): the old form returned any walkable raster line -
  including one fording a lake bed - which bypassed the water cost
  entirely (the synthetic channel route swam straight across while
  the cheaper bridge sat unplanned). A wet, walled or truncated line
  defers to the cost aware A*; the accepted dry line is its own
  smoothing (the two endpoints), so the region crossing concurrent
  searches stay instant instead of flooding the A* ellipse and the
  quadratic smoothing cascade.

### Verification

- New tests: `TestFindPathTerraceBoundaryNeedsARamp` (a synthetic
  500 unit terrace: sealed without a ramp, connected through one,
  every raw step within the limit, the climb back works);
  `TestFindPathFromDeckToFarWestZoneStaysOnRamps` (the live dump
  coordinates: found, ramp steps only, no swimming, under 5 s).
  Updated: `TestFindPathFromCityDeckToGroundZone` gained the
  `assertRampSteps` invariant; the water channel fixture now slopes
  BOTH shores (the real lake shores do - the north cliff let only the
  old asymmetric drop rule enter the water) and sits the start/end
  100 cells from the bridge (the old lx 100 margins were inside the
  diagonal shortcut noise); `TestLoopPathfindsBackIntoTheZone` pins
  the 3D arrival with a realistic zone deck height.
- go build/vet, go test ./... (18 packages), gofmt clean,
  golangci-lint: no new issues (11 pre-existing on the clean HEAD);
  live: `tools/mobius_e2e.sh 45` E2E_OK, `SWARM_PROXY_E2E=1` PASS.
- The next live zone switch from the city deck should plan through
  the south bridge, descend the ramp and reach the spore zone without
  a single stuck re-path - and the dump's walk plan will show the
  bridge waypoints instead of the straight west line.

## Active task: the zone switch sit freeze and the pathfinding return

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was redeployed and verified (STACK_READY: login :2106, game
:7777, db :3306, 75 tables).

### Goal

Two live problems of the manual hunting zone switch plus a web UI
feature: (1) a zone switch that lands on a sitting bot produces packet
errors - the bot must wait out the sit, stand up and continue; (2) the
bot does not pathfind on a zone switch and got stuck at
x 42536 y 48648 z -2992 on the flying elven city heading to the
Spore Fungus SW-d1 zone; (3) a Dump state button that copies the full
debug state (character, inventory, position, the recent log) to the
clipboard for live bug reports.

### Diagnosis

- The Mobius C1 server refuses every MoveToLocation while the
  character sits: `PlayerAI.setIntentionMoveTo` answers ActionFailed
  while the AI intention is REST. `sitDown`/`standUp` ignore toggles
  inside the 2.5 s `_sittingInProgress` window, the `StandUpTask`
  clears the paralysis and the REST intention 2.5 s after the stand
  broadcast. The bot's `returnToZone` had NO stand-up gate (only the
  town trip start and the flee had): a zone switch over a resting
  character sent walk requests every 2 s, each refused with an
  "Action failed" log line, forever - and nothing outside the zone
  ever stood the character up.
- `returnToZone` planned the search goal as
  `Vec3{zone.CX, zone.CY, selfZ}` - the WALKER height at the zone
  center. On the city deck (z -2992) against the spore zone ground
  (z -3664) the 3D approach goal sat mid air: no cell ever came within
  the 200 radius, the A* burned the whole 1M expansion cap (a 12-14 s
  frozen hunt tick per attempt - reproduced on the real geodata), and
  `startWalkLeg` fell back to the direct walk that ran the character
  into the west city railing at (42536, 48648) - exactly the reported
  stuck point (the deck ends there; the ground layer below is the
  lake bottom).

### Fix

- `hunt/loop_safety`-side sit gate: `returnToZone` gates on
  `standUpGuarded` before any walk planning, and `standUpGuarded`
  gained the `standSettlePeriod` (3 s) window: after the stand
  broadcast confirms, the caller waits out the server side stand
  animation before the first walk request (the guard's settle applies
  to the trip starts and the escapes too).
- `pathfind.Engine.ClosestHeight` (new): resolves the layer height at
  a world position closest to a reference z - the deck the server
  itself picks for a destination. The hunt Navigator interface and
  engineNavigator carry it; `returnToZone` resolves the real zone
  center deck before `FindPathApproach` (a lookup failure keeps the
  self height - the same-deck case). The city->spore route now plans
  in ~0.5 s over the real geodata (regression test
  `TestFindPathFromCityDeckToGroundZone`).
- The dump: `GET /api/bots/{id}/dump` (webserver/dump.go) assembles a
  plain text report - the character sheet, the aggro load, the zone,
  the equipment with slot names, the bag, the distance sorted
  objects, the walk plan, the combat beats, the chat and a 600 entry
  event window (`state.Bot.NewestEvents`, the ring holds 512). The
  HUD name row gained the copy button (clipboard API, textarea
  fallback, new tab escape). The hunt loop logger mirrors its
  decision lines into the tracker event log (`Loop.SetLogger` +
  the MultiWriter wiring in cmd/swarm) - the log tab and the dump
  carry the reasoning of the loop.

### Verification

- go build/vet, go test ./... (18 packages), gofmt clean,
  golangci-lint: 0 new issues (6 pre-existing ones on the clean HEAD -
  the golangci-lint v2.6.2 build of this sandbox is stricter than the
  one the repo last ran).
- New tests: `TestLoopStandsUpBeforeTheZoneReturnWalk`,
  `TestLoopResolvesTheZoneReturnDeckHeight`,
  `TestLoopKeepsSelfHeightWhenTheZoneDeckLookupFails`,
  `TestTripStandsUpBeforeWalking` (re-pinned to the settle window),
  `TestClosestHeight`, `TestFindPathFromCityDeckToGroundZone`,
  `TestFindPathFailsOnAFabricatedGoalHeight`, `TestBotDumpEndpoint`,
  `TestBuildStateDumpEventWindow`,
  `TestHuntEventLoggerMirrorsHuntLines`,
  `TestHuntEventLoggerKeepsTheConsoleFormat`.
- Live: SWARM_PROXY_E2E=1 E2E PASS; tools/mobius_e2e.sh 45 E2E_OK; a
  manual live run against the stack verified the dump endpoint end to
  end (character, zone, equipment, objects, events with the mirrored
  hunt lines).

## Active task: the deferred pile up logout and the two second relogin

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was already deployed and verified (STACK_READY, login 2106,
game 7777).

### Goal

The user called the emergency logout of the hunt loop too blunt in two
ways: (1) a character with two or more aggro mobs on it logged out on
the spot, so the relogin landed right back in the pack that piled up;
(2) the 30 s login pause idled the farm for nothing - a two second
relogin already resets the aggro on this stack (observed live). The new
behavior: run at least 600 units away from the aggro point before
logging out (the mobs stay behind, walk home while the character is
offline, the relogin lands outside their aggro range) and cut the
relogin pause to 2 seconds.

### Fix

- `hunt/loop_safety.go` `panicPileUpRun` (new): the attacker-count
  branch of the emergency logout no longer logs out at once. The first
  call anchors the aggro point (the position the pack piled up on),
  drops the current fight (the target lands on the long skip list, the
  engage bookkeeping clears - the same handoff `fleeFromTarget` makes)
  and starts the paced escape legs away from the threats (the same
  legs, pacing and zone clamping as the hurt flee). The logout fires
  once the character opened `panicRunDistance` (600) units between
  itself and the anchor.
- The run is committed once armed: the anchor, not the live mob count,
  drives the logout - a pack that thins out on the way cannot turn the
  run back into a lost fight.
- Two exits keep the run bounded: a cornered run (the legs never open
  the distance within the shared 20 s `fleeLogoutAfter` budget) logs
  out wherever it got to, and a run whose pack dissolved (no mob holds
  the target anymore, nothing attackable within the escape range) logs
  out at once instead of idling out the budget - nothing is left to
  run from, the spot is as safe as the run gets.
- The critical-health branch (HP under 12% with the blows landing)
  still logs out at once: one hit from death, the run has nothing left
  to protect.
- `panicLogoutPause` 30 s -> 2 s: the aggro resets the moment the
  character leaves the world, so the pause only needs to cover the
  logout round trip. The supervisor (`cmd/swarm/main.go`
  `runBotForever`) already retries a failed login with an exponential
  backoff, so a too-early reconnect (the server still holding the
  combat stance body) costs one retry, nothing more.

### Verification

- Unit tests: `TestLoopRunsFromThePileUpBeforeLoggingOut` (the pile up
  arms the anchor, walks the first leg, no logout on the spot, the
  pacing holds; after the character covers the leg past the 600 unit
  mark the logout fires with the 2 s cooldown; one shot),
  `TestLoopLogsOutWhenThePileUpRunNeverMakesDistance` (the cornered
  budget still ends the session), the cooldown assertions of the
  critical-health and endless-chase tests re-pinned to the 2 s pause.
- go build/vet, go test ./... (16 packages), gofmt clean,
  golangci-lint 0 issues.
- The live proxy E2E PASS against the running stack (login 2106,
  game 7777, 1.2 s) - the connection path is untouched by the change.

## Active task: silence the net ping answer log spam

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was already deployed and verified (STACK_READY,
PathFinding=2 like the reference deployment).

### Goal

The user reported the process log spamming "Net ping with game time"
messages many times per second whenever a real C1 client connects to
the game through the proxy. The task: identify what the message is
and, if it is just a debug line, remove it.

### Diagnosis

- The line lived in `connection/game_dispatch.go` `handleNetPing`:
  every NetPing (0xEC) answer of the game server was logged with the
  game time it carries, and nothing consumed that value (no pong
  tracking, no keepalive decision - the quality review already lists
  pong tracking as a future improvement, P02 task 2).
- The frequency comes from the client, not from a bug: the C1 client
  pings continuously on its own (RequestNetPing 0xA8, the client
  connection monitor). Behind the proxy every request transits
  through the bot session (`transitToServer` -> `SendRaw`), so the
  server answers at the client's ping rate - several replies per
  second observed live, one log line each (the GameClient logs into
  `log.Default`, the process log the user watches).
- The answers themselves are healthy: each one is tapped to the
  recorder and relayed back to the client, the connection stays
  alive. Only the logging was the problem.
- The proxy's own per-packet relay log ("game#N: client -> server
  0xa8") stays untouched: it lives in the separate `proxy.log`, which
  exists exactly for that per-packet trace (docs/proxy.md,
  "Debugging the client connection").

### Fix

- `handleNetPing` stays silent on the happy path. The packet is still
  parsed: a malformed one flags a cipher or protocol desync and still
  logs "Failed to parse net ping".
- Regression coverage: `TestNetPingAnswersStaySilent` (a fake server
  floods 100 valid replies plus one truncated: no spam line in the
  session log, the parse failure still logs) and the live proxy E2E
  gained the keepalive leg (a client burst of 10 RequestNetPing, all
  10 answers return through the relay, and the log assertions demand
  "client -> server 0xa8" present in proxy.log while "Net ping with
  game time" stays absent).

### Status: done (2026-09-09)

Verified: go build/vet, go test ./... (16 packages),
golangci-lint 0 issues, the live proxy E2E PASS against the running
stack (login 2106, game 7777) including the new ping burst leg.

## Active task: fix the elven village navigation (town trip to the trader)

Started: 2026-09-09. Branch: `feature/proxy-server`. Commits as melg8.
The stack was redeployed and verified first (STACK_READY, PathFinding=2
like the reference Windows deployment for the live reproduction).

### Goal

The user reported the bot stuck at x 45544 y 45880 z -2992 trying to
approach the trader Unoren (44667 46896 -2982) of the floating elven
village, and earlier sessions swam through the lake under the village
instead of crossing a bridge. The task: diagnose which Z coordinates
reach the pathfinder, whether it is used at all and what it returns,
then fix the navigation so the bridge/deck routes are planned and
verified from the stuck point and from the lake shore farm points.

### Diagnosis (measured on the deployed pack, live verified)

- The town trip DOES use the pathfinder (`startWalkLeg` ->
  `Navigator.FindPathTo(from, dest, int16(dest.Z))`): the start z is
  the live tracked character z, the target z is the merchant spawn z
  (-2982 for Unoren).
- `FindPathTo` resolves the target cell layer against that z: the shop
  cell holds layers -2632 (a raised surface) and -3928 (the lake
  floor), no deck layer -2984 - the C1 shop interiors have no floor
  layer in the l2j geodata, so the strict search hunts an unreachable
  roof layer and aborts at the 1M expansion cap (~12 s).
- The fallback plain search resolves the target against the START z,
  which picks the lake floor (-3928) under the shop; water cells cost
  the same as land, and the swim route (5683 units) is shorter than
  the bridge route (~6300), so the planned waypoints lead through the
  lake under the village - the observed swimming.
- The bridge ramps are gentle slopes (8..16 unit steps) from the
  fields onto the village deck (-3488 to -2984), fully connected in
  the geodata; the "disconnected village decks" note was a misreading
  of the missing shop floor layers plus the water preference.
- The server (Mobius C1 `GeoEngine.getValidLocation`) allows downward
  steps of any height, upward steps up to 40 (HEIGHT_INCREASE_LIMIT),
  and resolves the target z against the TARGET z (`PathFinding.findPath`
  uses `getHeight(tx, ty, tz)`); the water surface sits at -3780 (the
  water zones' maxZ) and the lake floor is walkable but slow.
- The interaction distance (250, 3D) is met from the deck ring around
  the shop terrace (the counter front), so the walk must only arrive
  within ~200 units of the merchant position, never inside the
  counter or on the roof layer.

### Plan

- pathfind: replace the strict layer-target search with an approach
  radius search (`FindPathApproach`): the A* terminates on the first
  node within the 3D radius of the target point, which keeps the
  arrival on the merchant's own deck and out of the water.
- pathfind: mirror the server step rules in the A* (downward any,
  upward 40), keep the strict symmetric rule for the line of sight
  smoothing so routes never collapse across drops.
- pathfind: water cost - layers below -3780 (the C1 water surface)
  cost 3x per step, so land routes beat swimming whenever they exist.
- hunt: the town trips and the manual long walks navigate through the
  approach search (200 for merchant stops, 150 for manual clicks).
- Acceptance: regression tests over the real pack from the stuck
  point and the lake shore farm points reach the merchant on the deck
  (end z near -2992, within 200 units of Unoren); the live walk
  crosses a bridge without entering the water; the full suite, lint
  and mobius_e2e stay green.

### Progress (2026-09-09)

- 2026-09-09: the fix landed (13660c7). The approach search replaces
  the strict layer-target search (FindPathTo is gone), the A* step
  rules mirror the Mobius validation (up 40, drops walkable), water
  below -3780 costs 3x, the manual long walks navigate with the
  approach search too. Verified: the real pack routes from the farm
  and the stuck spot end on the deck 189 units from Unoren (tests
  TestFindPathToShopDeck, the synthetic water/approach/drop/climb
  tests), go build/vet, go test ./... (16 packages), golangci-lint 0
  issues, mobius_e2e.sh 45 -> E2E_OK.
- 2026-09-09: live E2E on the deployed stack (PathFinding=2 like the
  reference deployment): a fresh bot walked spawn -> fields -> the
  bridge ramp (45912 42776) -> the village deck -> 59 units from
  Unoren in 45 s (search 0.46 s, 4 waypoints, z never below -3440 -
  no swimming), then selected the merchant (the target id confirmed,
  the character at 25 units from the NPC). AGENTS.md, the development
  log (round 35) and the navigation analysis carry the diagnosis.

## Active task: fix the real client connection failure (feature/proxy-server)

Started: 2026-09-08 (second round, after the user's first real client
test). Branch: `feature/proxy-server`. Commits as melg8. The stack was
redeployed and verified first (STACK_READY).

### Goal

The user's real C1 client could not connect ("не удалось подключиться").
The proxy.log they sent holds only the startup lines and the web UI
selection - no `login#N: client connected` at all - so the client never
reached the proxy. Root cause: the classic C1 executable hardcodes the
auth port 2106 (the l2.ini [URL] Port line is an Unreal leftover the
auth socket ignores - the user's stock ini shipped Port=7777 while the
real login always answered on 2106, which is also why the ini worked
against the real stack). The shipped ini's Port=2107 edit therefore did
nothing: the client dialed `127.0.0.1:2106`, the real Mobius login
server address, whose wildcard bind is also what made the proxy's
`127.0.0.2:2106` fallback fail with the Windows access permissions
error (a wildcard 0.0.0.0:port bind blocks every loopback address of
that port under Windows).

### Plan

- The proxy answers the hardcoded port 2106 AND the ini port 2107 on
  BOTH 127.0.0.1 and 127.0.0.2 (optional listeners: 127.0.0.1:2106 is
  normally the real login server address, so a bind failure is logged,
  not fatal).
- Family specific listener bookkeeping (the old flat slice would have
  made gamePort() read a login listener once several login listeners
  bind).
- Honest startup logging (only the actually bound addresses) plus a
  routing banner and a Windows aware bind hint naming the remedies.
- docs/proxy.md gains the two recipes: (A) move the real login server
  to 127.0.0.3 (LoginserverHostname=127.0.0.3 + swarm -login
  127.0.0.3:2106) so the proxy owns 127.0.0.1:2106, or (B) keep the
  real login on non-wildcard 127.0.0.1 and point the ini
  ServerAddr=127.0.0.2. Plus the Hyper-V/WinNAT excluded port range
  triage.
- Acceptance: unit tests for the listener semantics, the full suite,
  lint, and the live proxy E2E stay green; the startup log of a real
  run shows the bound listeners and the skip hint for the busy
  127.0.0.1:2106.

### Progress (2026-09-08)

- 2026-09-08: listener expansion landed (a57d741) and the routing docs
  with the two recipes (d946937). Live check on the standard sandbox:
  the busy 127.0.0.1:2106 is skipped with the remedy hint, the honest
  bound list and the routing banner land in proxy.log.
- 2026-09-08: the full Recipe A rehearsal against the live stack: the
  real login server rebound to 127.0.0.3:2106 (the game server
  re-registers automatically), the swarm with -login
  127.0.0.3:2106 -proxy, and a fake C1 client connecting through the
  hardcoded 127.0.0.1:2106 - login, one char list, world replay all
  green. The rehearsal exposed a second bug: a FRESH account hung at
  the character selection (see the next entry). PROXY_E2E_OK after the
  fix.
- 2026-09-08: fresh account creation race fixed (0eccffc). The Mobius
  creation handler writes the updated char list before the server
  cache update and the trailing create ok after it; a fast client's
  select lands in the window and is silently dropped. EnsureCharacter
  drains the trailing ok, EnterWorld retransmits the select with a
  bounded wait. Verified with a fresh account on the live stack.
- Sandbox note: the rehearsal temporarily rebound the login server to
  127.0.0.3 (dist/login/config/Server.ini); the sandbox is restored to
  127.0.0.1 afterwards (the standard E2E layout).

## Finished task: MITM proxy server for the real C1 client (feature/proxy-server)

Started: 2026-09-08. Branch: `feature/proxy-server`. Commits as melg8,
pushed as they land. The stack was deployed and verified first
(ports 2106/7777/3306, 75 tables, STACK_READY).

### Goal

A real Lineage 2 C1 client must be able to connect to the swarm process
and observe/control the character of the running in-game bot exactly as
if the client were connected to the L2J Mobius C1 server directly. The
swarm acts as a transparent MITM server (transparent for both the client
and the game server) and as the carrier of the in-game bot: with the bot
farming, a connecting user watches the automated gameplay from the
bot's perspective, and the packets of the user's own clicks flow to the
real server through the bot's session.

### Constraints (from the task brief)

- The swarm emulates BOTH the login server and the game server for the
  client. Any login/password pair is accepted; the character list shows
  exactly one character - the bot selected in the web UI, or the first
  bot when nothing is selected or the web UI is offline. Neither the
  server list nor the character list is used to pick which character to
  enter (the emulation only forwards the single preselected character).
- Multiple bots will run later: the web UI selection is the switch for
  which bot a connecting client attaches to; the client switches by
  reconnecting after changing the selection.
- swarm and Mobius run on the same machine, so connection interception
  on the same ports is impossible - the proxy listens on its own ports
  (login 127.0.0.1:2107 plus a 127.0.0.2:2106 hardcoded-port fallback,
  game 127.0.0.1:7778), and the C1 client is redirected through a
  re-encrypted l2.ini (data/client/l2.ini, open-l2encdec protocol 212).
- Every packet is decrypted and re-encrypted by the swarm in both
  directions (login Blowfish framing, game XOR cipher - the proxy owns
  two independent cipher chains per client connection, so future packet
  rewriting/dropping stays possible: that transformer seam is designed
  in, the current transformer is the identity/transparent one).
- Packets the swarm does not handle transit unchanged between the
  client and the real server.
- The C1 client runs on Windows only, so the real-client check is a
  user-side step: the swarm must log every client connection attempt to
  a dedicated file (proxy.log) so a failed attempt can be diagnosed from
  the file alone.

### Design

- `internal/swarm/proxy`: the emulated login server (Init -> any
  RequestAuthLogin -> LoginOk -> ServerList with one proxy entry ->
  PlayOk), the emulated game server (ProtocolVersion/KeyPacket with a
  proxy key -> AuthLogin -> one character CharSelectionInfo synthesized
  from the bot tracker -> replayed CharSelected -> EnterWorld replays
  the recorded world stream of the bot session -> live relay), the
  packet history recorder (the bot session tap: full stream with a
  prologue+tail cap), the transformer seam and the file logger.
- `connection.GameClient` grows a packet tap (every decrypted server
  packet, from CharSelectionInfo onward) and a raw send path
  (`SendRaw`) that encrypts through the same outKey/writeMu critical
  section the hunt loop uses, so client packets and hunt packets share
  one cipher chain without races.
- The replay model: the client gets the full recorded server->client
  stream of the bot session (everything after the bot's CharSelected),
  then the live feed continues at the cursor - the two cipher chains
  (server->proxy and proxy->client) advance independently, which makes
  the relay a true MITM instead of a byte pipe.

### Status: code complete, live E2E green, lint clean (2026-09-08)

- [x] Environment deployed and verified (STACK_READY).
- [x] l2.ini decrypted with open-l2encdec, [URL] Port 7777 -> 2107,
      re-encrypted and committed as data/client/l2.ini.
- [x] Login packet serializers (LoginOk/ServerList/PlayOk/Init opcode
      framing, CharSelectionInfo full serializer) with round trip
      tests.
- [x] connection: the GameClient tap (every decrypted server packet
      from the char list onward), SendRaw (raw client packets through
      the shared outbound cipher critical section) and the scrambled
      RSA modulus capture of the login Init (AuthResult).
- [x] proxy.Recorder: the session history with sequence numbers, the
      prologue+tail byte cap, live subscribers with poison on lag.
- [x] proxy login server emulation: Init -> any credentials -> LoginOk
      -> one entry ServerList (advertises the proxy game port on the
      login connection address family) -> PlayOk; GGAuth answered.
- [x] proxy game server emulation: ProtocolVersion/KeyPacket with a
      per client key, any session keys accepted, the one character
      list (recorded appearance + live tracker vitals), the recorded
      CharSelected answer, EnterWorld -> the recorded stream replay +
      the live relay, the client packet transit through the bot
      session, character create/delete refused.
- [x] The transformer seam (proxy.Transformer, identity passthrough
      today) documented as the future packet spoofing point.
- [x] web UI: GET/POST /api/proxy endpoints, the sidebar click
      selects the bot for connecting clients (the proxy chip).
- [x] main.go: -proxy, -proxy-login, -proxy-game, -proxy-log flags;
      the session registration/unregistration lifecycle, the proxy.log
      file logger.
- [x] Live stack E2E (tools/proxy_e2e.sh -> PROXY_E2E_OK): the bot
      enters the world, a fake C1 client logs in with garbage
      credentials, sees the one character, enters the world through the
      replay, receives the live NPC traffic, moves the character
      through the relay (the own movement echo arrives) and everything
      shuts down gracefully. The one real fix it forced: the movement
      mode of the client MoveToLocation is a full int (the server
      failed reading the short packet - see the game log "Failed
      reading: MoveToLocation").
- [x] Deploy tweak: LoginserverHostname = 127.0.0.1 on the login
      Server.ini so the proxy fallback 127.0.0.2:2106 binds (both
      deploy scripts synced).
- [x] Docs: docs/proxy.md (the user guide), the AGENTS.md proxy
      section, the protocol_description.md emulation packet notes.

### Verification summary

- go build / go vet / go test ./... - 16 packages green.
- golangci-lint run - 0 issues (the full strict set of .golangci.yml).
- tools/proxy_e2e.sh - PROXY_E2E_OK against the deployed stack (three
  consecutive runs).
- 14 atomic commits on feature/proxy-server, all pushed as melg8.

### Outcome of the real client check (2026-09-08)

The user ran the first real client test and the client could not
connect. The proxy.log they sent contained only the startup lines (no
client connection at all): the classic C1 exe hardcodes the auth port
2106, so the Port=2107 ini edit did not route the client to the proxy.
The follow-up task above ("fix the real client connection failure")
covers the listener expansion, the honest bind logging and the two
Windows recipes. The live check remains the user's: after applying a
recipe, on any failure send the proxy.log file - it records every
connection attempt, the credentials used, the state transitions and
the close reasons.


## Finished task: spawn-true hunting zones, all-mob farming, rotation sweep

Started: 2026-09-08. Branch: `mobius-c1-client-1`. Commits as melg8,
pushed as they land. The stack was deployed and verified first
(ports 2106/7777/3306, 75 tables, STACK_READY).

### Goal

The user reported three problems of the multi zone hunting: (1) most
hunting squares sit "past" the real points where the main mob groups
stand on the live server - suspecting the mob movement off the spawn
point; (2) the rotation ping pongs - zone A empty, the bot walks to B,
B is empty too, it walks back to A (still empty) instead of moving on
to C; (3) the zones must farm ALL mobs that live on the ground, not
just one species (killing only orcs in a square where goblins stand
in the same radius is pointless; per-mob priorities are acceptable).

### Root cause analysis

- The Mobius spawn mechanism is the key: `Spawn.initializeNpc` rolls
  every npc at a uniformly random point of the territory polygon
  (`NpcSpawnTerritory.getRandomPoint`), the random walk keeps the mob
  inside the polygon (AttackableAI checks `isInsideZone` for every
  wander target) and `MaxDriftRange = 300` leashes the rest (the
  WorldRegion teleports a mob back to its spawn once it drifted too
  far and the region emptied). The real mob distribution is therefore
  uniform over the polygon area - a square anchored on a "cluster
  centroid" only covers the fraction of the polygon inside it.
- Measured (scripts/analyze_spawns.py, sandbox side): the old hand
  placed registry of 30 squares covered 18% of the expected spawn
  mass; 88 of 106 territories sat below 50% coverage, many at 0% (the
  whole far southwest 2020 block, the grunt woods 2119_22..26, most
  of the 2019 west). That is why the zones "missed".
- The A-B-A bounce: `rotationZone` picked the nearest same-band
  sibling with no memory of recently emptied squares - from B the
  nearest sibling is A again.
- The "one species" complaint: the engage itself always attacked the
  nearest attackable npc (no name filter), but the old squares sat on
  parts of one territory, so the practical pick collapsed to that
  territory's mobs.

### What was done

- `tools/generate_hunt_zones.py` (committed): parses
  ElvenStarting.xml + the npc stats, drops the same-band
  sub-territories folded into their parents (>= 85% containment),
  partitions each polygon into a density-adaptive grid (cell
  2600-3600, square half 1300-1900, cells with >= 22% polygon overlap
  kept, one square for a small territory), assigns the ten band /
  gear gates by the territory's top mob level, emits the mob list
  with level-sorted priorities, orders the registry by band then
  village distance (zones[0] is the starter fallback). Output:
  `hunt/zones_elven.go` - 227 squares, 96% spawn mass coverage (was
  18%).
- `hunt.HuntingZone.Mobs []ZoneMob` (template id, name, level, count,
  priority) carries the full mob list of the ground;
  `applyHuntingZone` builds the priority map for the engage.
- `state.Bot.NearestAttackablePreferred`: the target search scores
  `dist - 200*priority` per candidate (plain and social variants
  share it), so every species stays attackable while the exp richer
  mobs of the ground win among comparably near candidates. The
  engage, the far-target walk and the zone-entry engage pass the
  priorities.
- `state.Bot.ZoneHasAttackableBelow`: the rotation emptiness reading
  applies the engage level ceiling - a square whose survivors all sit
  above the max target level counts as empty.
- The rotation cooldown: a rotated-away square keeps a 40 s
  `zoneEmptyCooldown` (`zoneEmptyUntil` map, pruned on write);
  `rotationZone` skips the cooling squares, so the sweep moves
  forward through the band (A to B to C) instead of bouncing back;
  when every sibling cools down the hunter waits out the respawn in
  place.
- zones_test.go rewritten to look zones up by band and position (no
  hardcoded generated ids - a regeneration keeps the suite green);
  new tests: the generated registry sanity (band order, unique ids,
  compact halves, non-empty mob lists), the cooldown sweep
  (A-B-C-blocked-return-expiry), the all-cooling wait, the level
  aware emptiness, the priority bias of the engage; state tests for
  ZoneHasAttackableBelow and NearestAttackablePreferred.

### Status: done

- Code complete: go vet + go test ./... green, golangci-lint clean,
  pushed as e42c6c5 after the rebase onto the fleet scale round.

### Live verification (zonetest1, the stack up)

- Round 1 (3 min, -hunt): the picker took the generated starter
  square "Red Keltir SE-a1" (elven-2119_01-a1, the nearest band 1-3
  square to the village - it sits on the eastern keltir polygon, not
  on the village meadow of the old registry); 8 kills with one rest
  cycle, level 1 -> 3; the character fought inside the square the
  whole round (no wandering through empty ground).
- Round 2 (55 s, -hunt -web): the snapshot carried all 227 zones
  with the active marker on elven-2119_01-a1; the character stood at
  (46812, 43419) inside the active square, in combat, with 12 Red
  Keltir + 3 Elder + 2 Young visible around it - the square sits
  exactly on the real spawn mass; three more kills during the
  round.
- The rotation never fired live (the 20 s respawn of a compact
  square keeps up with the kill rate of one hunter - the intended
  steady state); the sweep semantics stay pinned by the unit suite
  (A to B to C, the cooldown blocks the return).

### Progress

- 2026-09-08: analysis + generator + registry + cooldown + priorities
  landed as the first commit round; this entry.
- 2026-09-08: rebased onto the fleet scale round (a parallel agent
  split the god objects: the target search now lives in
  state/scans.go with the pooled scan arrays and the dense skip
  slices, the hunt loop split into loop_actions/loop_movement/
  loop_safety). The priority map threads through the new
  NearestAttackablePreferred signatures (skip []int32), the npcScan
  record carries the template id for the pooled social variant, the
  scans.go ZoneHasAttackableBelow replaces the bot.go edit. Zones,
  the generated registry, the tests and the docs port unchanged.


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
- 2026-09-08: the hunt loop god file split and the tick made
  allocation free. loop.go (1480 lines) held the state machine, the
  safety layer, the movement phases and the between-fights phases;
  the safety layer (flee, escape walks, emergency logout, the skip
  list) moved to loop_safety.go, the movement phases (far target
  walk, patrol, return to zone) to loop_movement.go and the
  rest/loot/cleanup phases to loop_actions.go (1010/181/195/144
  lines). The allocation profile of the engage tick (memprofile of
  BenchmarkHuntTickEngage): the per tick shopCatalog rebuild of
  the town trip trigger, the per tick starter item scan and equip
  scan over an unchanged inventory, and the per call skip map of
  the target search. Fixes: the town shop catalog is built once
  (package level, static generated data), the equip manager keys
  its scans on the new tracker InventoryVersion (a bag unchanged
  since an empty scan costs nothing; the version bumps on every
  inventory/paperdoll mutation - the full record compare also
  catches the equip flag flips now), the skip map became a reused
  dense slice on the loop (state scans take []int32), and the flat
  npcScan arrays of the social search come from a sync.Pool (the
  search runs under the read lock, a per bot scratch would race).
  BenchmarkHuntTickEngage 4352 ns/3040 B/5 allocs -> 3020 ns/0
  B/0 allocs; BenchmarkActiveSkips 61 ns/0 allocs;
  NearestAttackableConstrained 200 npc 9.7 us/10 KB/1 alloc ->
  7.1 us/0 B/0 allocs. A hundred hunting bots at 4 ticks/s now
  produce zero steady state garbage from the decision path.
- 2026-09-08: task wrap up. The final verification round: go build,
  go vet, go test ./... (14 packages green), golangci-lint run
  (0 issues) and the live E2E (tools/mobius_e2e.sh 45) prints
  E2E_OK against the deployed stack. Final fleet numbers (before
  -> after): SSE per event 109 us/173 KB/5 allocs -> 64 us/26
  KB/3 allocs (the frame and payload buffers are reused per
  connection, -85% garbage; the remaining cost is the snapshot
  deep copy the encoder needs), NewBot 9.7 us/28 KB/7 allocs ->
  1.3 us/2.5 KB/5 allocs (lazy rings, a hundred idle trackers hold
  250 KB instead of 2.8 MB), RegistryList100 12.5 -> 10.7 us
  (dense slice walk, no per bot map hashing), hunt engage tick
  4352 ns/3040 B/5 allocs -> 3000 ns/0 B/0 allocs (catalog cached
  once, scans keyed on InventoryVersion, pooled social search,
  dense skip list), NearestAttackableConstrained 200 npc 9.7
  us/1 alloc -> 7.1 us/0 allocs. The god objects are split: state
  bot.go 2448 -> 1848 lines (objectStore, eventLog, chatLog,
  combatFeed, scans.go), connection game.go 1624 -> 852 (dispatch
  + apply layers), hunt loop.go 1480 -> 1010 (safety, movement
  and action phase files). Six commits pushed to
  mobius-c1-client-1 as melg8.
- 2026-09-08: direct live encode round. The inventory map became a
  dense canonical store (inventory_store.go: the widget order -
  equipped first, then item id, object id - is restored once per
  mutation batch, so the snapshot fill walks the slice with no
  read time materialize-and-sort and the gear scans skip the map
  buckets). Bot.AppendSnapshotJSON (snapshot_live.go) encodes the
  whole state straight from the live records under the read lock:
  the per element view structs live on the call stack and the
  golden append functions of snapshot_json.go are reused per
  element (appendEventJSON, appendChatEventJSON, ...), so the
  steady state of a watched stream allocates nothing - the
  Snapshot() copy in between used to pay the object, combat, event
  and chat slice allocations. Byte equality with the copy path is
  pinned by TestAppendSnapshotJSONMatchesSnapshot (same
  millisecond retry) on top of the reflection golden suite; the
  SSE stream event and the state endpoint call the live encoder
  directly. SSEStreamSteadyState 65 us/25.7 KB/3 allocs ->
  53 us/3 B/0 allocs; SSEEncodeAndFrame 109 us/173 KB/5 allocs ->
  98 us/147 KB/2 allocs.
- 2026-09-08: SoA split of the world records. The 240 byte
  WorldObject became two parallel dense arrays in objectStore,
  length locked and indexed by the same slot: objectHot (88 bytes:
  the scan fields - position, destination, level, the one byte
  kind code, attack flags, the clan bitmask, speeds, move and
  combat unix nanosecond stamps with 0 as the zero time) and
  objectCold (names, title, template, heading, vitals, the social
  marker). Every scan and the movement projection walk the hot
  array only; a 200 npc world drops from 48 KB to 17.6 KB of scan
  traffic (the strings stop polluting the scan cache lines). The
  priority biased target search and the ZoneHasAttackableBelow
  rotation check of the parallel agent kept their semantics on
  the new layout. Constrained 200 npc scan 6.6 us/0 allocs (with
  the priority bias); the new BenchmarkFleetScanPressure (a
  hundred 200 npc worlds swept back to back, 1.76 MB of hot
  records vs 4.8 MB before the split) measures 1.2 ms per fleet
  sweep, and BenchmarkFleetLiveEncodePressure 8.4 ms per fleet
  sweep at 0 allocs.
- 2026-09-08: real 100 bot fleet E2E. internal/swarm/fleete2e
  launches a hundred live sessions against the deployed Mobius
  stack (login, elven fighters, hunt loops, the 24/7 reconnect
  supervisor with the emergency logout cooldown honored) and
  measures the state layer under the real packet load:
  BenchmarkFleetE2ELiveEncodeSweep 5.9 ms per 100 bot sweep
  (~59 us per bot) under live contention, the engage scan sweep
  64 us per fleet, and the fleet packet rate test samples 2860
  packets/s aggregate with 60-70 of the 100 sessions online
  (the crowded elven starting area cycles the rest through the
  emergency logout cooldowns - the breathing steady state is the
  real shape, the supervisor brings them back). The suite is
  opt in: SWARM_FLEET_E2E=1 go test ./internal/swarm/fleete2e/
  -bench . -benchtime 20x -timeout 25m. Full verification each
  round: go build/vet/test (14 packages), golangci-lint 0
  issues, mobius_e2e.sh 45 -> E2E_OK. Four commits pushed to
  mobius-c1-client-1 as melg8.
- 2026-09-08: invalid UTF-8 parity fix of the JSON string writer. The
  reflection golden suite (TestSnapshotJSONMatchesReflection,
  TestAppendJSONStringTable) compares the hand rolled writer against
  json.Marshal at runtime, and newer toolchains (the v2 backed
  encoding/json of GOEXPERIMENT=jsonv2 and the releases shipping it
  by default) replace the invalid UTF-8 bytes of a JSON string with
  the literal U+FFFD replacement rune instead of the classic \ufffd
  escape sequence, so the suite went red on those toolchains while
  Go 1.24 stayed green (the repo toolchain). appendJSONString now
  emits the replacement through the init time probed
  jsonInvalidUTF8Replacement - a one byte json.Marshal probe at
  package init, zero runtime cost, mirroring the same stdlib the
  reflection tests marshal with - which keeps the writer byte
  identical to the reflection encoder on every toolchain. Both
  replacement forms are pinned by TestAppendJSONStringInvalidUTF8Modes
  (the inactive branch is forced in the test so a toolchain switch
  flips a loud test, not a silent byte drift). AGENTS.md documents
  the probe contract. Verified: go build/vet, go test ./... (16
  packages), golangci-lint 0 issues.
- 2026-09-08: real client game handshake fix (round 3, feature/proxy-
  server). The user's Windows C1 client passed the login emulation and
  the server selection, then dropped on the game port with
  `failed to parse auth login: EOF`. Root cause: the emulated game
  server answered the KeyPacket with a random per connection cipher
  key, while the real Mobius C1 server always answers with the fixed
  GameClient.CRYPT_KEY (94 35 00 00 a1 6c 54 87, "the last 4 bytes
  are fixed") - the C1 client must stay compatible with a hardcoded
  key, so its encrypted AuthLogin desynchronized the proxy XOR chain
  and the parse failure closed the connection (the fake e2e client
  honors the packet bytes, which is why the suite stayed green).
  The handshake now sends the exact static key (regression test
  TestGameServerStaticKeyServesHardcodedKeyClient drives a client
  that ignores the KeyPacket bytes and encrypts with its own copy).
  Defense in depth: readGameAuthLogin is lenient now - it accepts the
  null terminated Mobius layout and the short length prefixed utf16
  layout, and a fully unreadable packet logs a bounded decrypted hex
  dump (`auth login packet unreadable (len N, decrypted XX:...)`)
  and continues under the `<unreadable>` account instead of dropping
  the client (the name is cosmetic, any pair is accepted). The
  proxy.md triage table documents the new signatures. Verified:
  stack redeployed (STACK_READY), SWARM_PROXY_E2E=1 full MITM e2e
  against the live stack, go test ./... (16 packages), golangci-lint
  0 issues.
- 2026-09-08: reconnection live state fix (round 4, feature/proxy-
  server). The user's real client reconnected after the bot had walked
  far from its login place and spawned at the stale login coordinates:
  the character ran into the server-side walls and the client crashed.
  The proxy replayed the recorded CharSelected and UserInfo byte for
  byte, so the entering world packets described the login-time state,
  not the current one. Three changes, all fed by the live state
  tracker (state.Bot): (1) the synthesized CharSelectionInfo now
  carries the paperdoll tables (15 slot object ids + 15 item ids,
  parsed and serialized by fromgameserver.CharacterInfo now) built
  from the tracker - the slot object ids come from the last UserInfo
  broadcast, the item ids from the tracked inventory - so the
  selection screen renders the equipped gear instead of a naked
  character (the user's second report); (2) the recorded CharSelected
  answer is binary-patched with the live x/y/z, curHp/curMp, sp, exp
  and level (patchCharSelectedLive scans the two utf16 strings and
  the header ints to find the x offset; an unscannable packet replays
  unchanged); (3) the replay patches every UserInfo of the played
  character with the live position/vitals/progression
  (patchUserInfoSelfLive - fixed header offsets for x/y/z, a name
  scan for the vitals block, position-only degradation when the name
  cannot be scanned) and drops the stale self movement packets
  (MoveToLocation, MoveToPawn, StopMove, ValidateLocation,
  TeleportToLocation) except the newest one, whose coordinates match
  the tracker by construction. state.Bot gained the light
  SelfSnapshot() accessor (character view only, no world copy) for
  the per-packet patch reads. New tests: TestPatchCharSelectedLive*,
  TestPatchUserInfoSelfLive*, TestSelfMovementFiltering,
  TestGameServerReconnectServesLiveSelfState (full flow: bot gears up
  and walks away, client reconnects, char list paperdoll + live
  position, patched char selected, patched replayed UserInfo, the
  last self movement kept); the e2e got the real reconnection leg
  (the client closes, the bot finishes the walk, a second client
  enters and must see the walked-to place in the char list, the char
  selected answer and the replayed UserInfo, plus the live paperdoll
  mirroring bot.PaperdollSlotObjectIDs + InventoryItems). Verified
  against the deployed stack: SWARM_PROXY_E2E=1 e2e PASS (the bot
  walked, the second client saw 46315 41341 -3440 with the real
  gear), go build/vet, go test ./... (16 packages), golangci-lint 0
  issues.
- 2026-09-09: the bot relogin handoff (round 5, feature/proxy-server).
  The second half of the proxy contract: the client must survive the
  session cycle of its bot. The hunt loop logs the character out (the
  emergency logout, the pile up escape) and the supervisor logs it
  back in seconds later - the user did nothing and must not be kicked
  to the login screen for a decision the bot made. Three pieces, all
  in the relay (streamSession/serveRelogin of proxy/game.go, the new
  proxy/handoff.go): (1) the LeaveWorld suppression - the answer of
  the real server to the bot's logout is dropped from both the
  recorded history and the live feed (a client that processes it drops
  itself to the login screen), unless the client asked for the logout
  itself (the classic flow keeps the relayed answer); (2) the hold -
  the recorder close keeps the connection open while swallowing the
  client packets (the character is offline, the world behind the
  client is frozen), a user Logout while held gets a synthesized
  LeaveWorld from the proxy itself (the login screen beats a
  swallowed intent), and the hold gives up after 2 minutes of a
  missing bot; (3) the resync - once the replacement session of the
  same bot id is back online, the played character receives a
  synthesized TeleportToLocation to its live position, every object
  id of the old known list is swept with DeleteObject (the new
  Bot.KnownObjectIDs accessor), the enter world burst of the new
  session replays through the ordinary live self state patch, and the
  connection swaps onto the new session (the client packets transit
  through the live bot link again). The sender got a bounded shutdown
  flush (the queued packets reach the client before the socket
  closes - the synthesized LeaveWorld of the held logout rides it),
  and a transit failure no longer kills the client connection (the
  narrow window before the hold engages would have dropped it). New
  tests: TestHandoffPacketBuilders (the byte layouts against the
  Mobius writeImpl bodies), TestGameServerHoldsClientThroughBotRelogin
  (the core scenario end to end), TestGameServerUserLogout*
  (the classic flow and the held logout), TestGameServerHoldTimeout,
  TestGameServerReloginWithoutCharSelected (the defensive live feed
  only path); the full client flow test flipped to the hold contract.
  One stack side note: the login server held a stale session key for
  the account after a wedged run (`Session key incorrect` in
  game.log), a login+game restart cleared it - the E2E failure was
  stack staleness, not code. Verified against the redeployed stack:
  SWARM_PROXY_E2E=1 e2e PASS, tools/mobius_e2e.sh E2E_OK, go
  build/vet, go test ./... (18 packages), go test -race on the proxy
  package, golangci-lint (2 pre-existing gosec, no new issues).
- 2026-09-09: the build identity of the state dump (round 6,
  feature/proxy-server). A live problem report must tell which code
  produced it: the "Cannot see target" investigation opened with a
  dump whose origin had to be inferred from the user's `git pull`
  output - the report itself said nothing about the branch or the
  commit. Now the second line of every state dump and the line right
  after the startup banner of the bot log carry `build: branch <name>,
  commit <hash> (dirty|clean), built <time>`. New `internal/version`
  package: the four link-time fields the build scripts bake in via
  -ldflags -X (Branch, Commit, Dirty, BuildTime), with two fallback
  levels for unstamped binaries - the vcs.* settings Go embeds into
  every binary built from a git repository (revision, modified,
  commit time) and, for the branch the VCS stamp does not carry, the
  .git/HEAD of the working directory (the ref form, the gitdir
  pointer of a linked worktree, empty on a detached HEAD - a stale
  hint would be worse than none). Unknown fields drop out of the
  rendered line. The three build sites bake the identity in
  (swarm_fast_deploy.sh and mobius_fast_deploy.sh byte-identical,
  mobius_e2e.sh echoes it right after the build); a plain
  go build/go run still identifies itself through the VCS stamp plus
  .git/HEAD. The dump test asserts only the `build: ` prefix (the
  values depend on the working tree of the moment). Verified live on
  the deployed stack: both build paths produced the identity line in
  the dump of a bot in the world and in the bot log;
  tools/mobius_e2e.sh E2E_OK, go build/vet, go test ./...
  (19 packages), golangci-lint 0 issues on the touched packages.
- 2026-09-09: the full commit hash and the file path build identity
  (round 7, feature/proxy-server). The round 41 identity line had two
  gaps in real runs: the hash was short (7 chars - awkward to search
  anywhere but git itself) and `go run ./cmd/swarm/main.go` - the
  exact Taskfile run:app form - compiles the command-line-arguments
  package, which Go leaves WITHOUT a VCS stamp, so the user's local
  dump showed the branch but no commit at all. Now the commit renders
  in the full 40 character form on every path (the scripts bake
  `git rev-parse HEAD`, the vcs stamp passes untrimmed), and the .git
  fallback resolves the commit too: HEAD -> the loose ref file ->
  packed-refs, with the gitdir pointer and the commondir indirection
  of linked worktrees handled; a detached HEAD is the hash itself.
  Taskfile run:app switched to the package path form (`go run
  ./cmd/swarm`) so the ordinary local run keeps the full stamp. All
  local git reads go through one readGitEntry helper - the gosec
  taint analysis does not flag it, so the round 41 nolint is gone.
  Verified live on the deployed stack: the file path build (no vcs
  stamp, confirmed with go version -m) rendered `commit
  675d2e545262...e09fc` in a dump of a bot in the world; the ldflags
  build rendered the full hash with (dirty) and the link timestamp;
  tools/mobius_e2e.sh E2E_OK. go build/vet, go test ./...
  (18 packages), golangci-lint 0 issues.
- 2026-09-09: the stuck spot reproduction pinned (round 8,
  feature/proxy-server). Return to the original round 35 stuck problem
  (the character stuck at 45544 45880 -2992 walking to the trader
  Unoren 44667 46896 -2982): everything re-measured on the deployed
  pack (the pond water cells at -3880 between the spot and the shop,
  the missing shop floor layer, the false line of sight of the direct
  line) and the whole scenario reproduced live on the stack - the
  character placed at the stuck spot through the database with 41
  non-stackable daggers as the trip trigger (stackable junk merges
  into one slot on login and never fills the bag), one hunt session,
  the trip walked the geodata route in 13 s, sold out, returned, zero
  stuck re-paths, the mid-walk state dump carrying the build identity
  line plus the published 3 waypoint walk plan. The reproduction is
  now pinned by two tests over the real geodata pack
  (internal/swarm/hunt/town_repro_test.go):
  TestReproStraightWalkStallsAtTheUserStuckSpot (a server simulating
  MoveToLocation follower - cell by cell advance with the geodata line
  of sight as the conservative model of the server's straight line
  validation - stalls at exactly the reported coordinates when sent
  straight at the merchant: the mechanism of the original report) and
  TestReproTownTripFromTheUserStuckSpot (the full trip from the same
  positions: the geodata route, the walk under the simulated server,
  the arrival within the interaction distance, ZERO stuck re-paths,
  the merchant selection and the first sell batch). The live scenario
  is repeatable through tools/repro_stuck_trip.sh (DB placement +
  trigger arming + one session + the mid-walk dump capture +
  REPRO_OK/FAIL verdict on the hunt log). One environment lesson
  documented in the tool header: the bot must run with the swarm root
  as CWD (or -geodata) - the relative geodata candidates silently
  degrade to a hunt without town trips otherwise. Verified: stack
  STACK_READY, gofmt, go build/vet, go test ./... (19 packages),
  golangci-lint 0 issues on hunt, tools/mobius_e2e.sh E2E_OK.


## Active task: the web map social, hover and fleet layers - the aggro truth of the server

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user request (2026-09-10, Russian): (1) the hunting zone names
show only on the map hover; (2) hovering a zone of the selection list
focuses the map on it, highlights it and shows its name; (3) the kill
crosses must cover the whole map and all bots instead of vanishing on
every bot switch; (4) find out why the map sometimes flashes white
(the loaded base disappearing for a microsecond, follow mode on);
(5) verify the exact monster aggro radii against the server (the map
circles looked exaggerated); (6) draw the monster sociality - the clan
assist links between neighbors, with warning links while they only
approach the range.

### Root causes (researched from the Mobius C1 sources)

- The aggro radii: the xml ai data carries aggroRange 1000 for most
  monsters, but the Mobius C1 NpcTemplate constructor clamps every
  range at MaxAggroRange (dist/game/config/NPC.ini ships 450 against
  the L2J default 1500) BEFORE the AttackableAI on-sight check
  (isInsideRadius3D against getAggroRange, a GeoEngine line of sight
  on top, and a 4-8 s blind window after spawn through rollGlobalAggro
  counting _globalAggro up from -(Rnd.get(5)+4)). The map drew the raw
  xml value - more than 2x the real trigger distance.
- The white flash: drawMapBackground skipped tiles whose entry was
  still loading with NO fallback to the coarser pyramid levels (the
  ancestor walk only ran for 404-missing tiles). In follow mode the
  camera pans with the walking bot, new blocks scroll into the view,
  and every still-loading block left a blank strip reading as the page
  background until the fine image landed.

### Implementation

- npcdata.NPCAggroRange caps at the server clamp (450): the tooltips
  and the aggro circles now carry the number the server acts on.
- The snapshot objects carry `clanHelpRange` and the clan `clanMask`
  (a decimal string - the ALL bit exceeds the JavaScript safe integer
  range, the map parses it with BigInt once per snapshot).
- The map: zone names light up only under the pointer (map hover or
  the list focus), the hovered/focused ground takes the highlight
  stroke and the brighter fill; `focusZone`/`blurZone` pin the camera
  on a list hover and restore it on leave; `drawSocialLinks` connects
  the same-clan npcs inside their clan help range with solid teal
  lines and the approaching pairs (within 1.25x) with dashed amber
  warnings; `drawKillMarks` paints the fleet kill crosses.
- The fleet kills: the hunt loop publishes its kill ring with the spot
  view (`Bot.SetKillMarks`), the registry merges the rings of all bots
  (`Registry.FleetKillMarks`, oldest first, capped at 400) and the web
  app polls `/api/fleet/kills` with the bot list - the crosses live in
  the map layer and survive the bot switches.
- The tile flash: `mapTileAncestor`/`geoTileAncestor` now return the
  finest READY pyramid entry - the loaded coarse ancestors paint the
  block while the fine tile streams, so a panning camera never bares
  the background.

### Status: done (2026-09-10)

- Verify loop: go build ./..., go vet, go test ./... (all packages
  green), gofumpt clean; the node repro harnesses
  `tools/repro_map_render.js` (the hunting zone scenario now pins the
  hover-only labels) and the new `tools/repro_zone_hover.js` (five
  scenarios: the list focus camera, the fleet crosses with the age
  fade, the social link geometry, the tile ancestor fallback, the spot
  hover) all pass; a headless browser check against
  `tools/webui_preview_server.js` (the real web dir with stub bot
  APIs) confirmed the rendering by pixel sampling: the aggro ring, the
  kill crosses, the teal social links, the hover label of the spot and
  the red coarse-ancestor fallback while the fine tile streams.
- The live C1 stack stays unavailable from this sandbox (GitLab 403):
  the Mobius C1 sources were fetched through the GitLab web raw
  endpoints for the research above.

## Active task: the spot-anchored hunting implementation

Started: 2026-09-10. Branch: `feature/proxy-server` (implemented on
the dedicated `feature/hunt-spots` branch born from it per the user
request, kept rebased onto the base, then folded back into the base
and the branch deleted). Commits as melg8.

### Goal

The user approved the hunting system redesign research (the docx
round): implement it, and add a visualization so the spots, their
sizes, the deaths and the timers read at a glance.

### Method and state

- The registry: `tools/generate_hunt_spots.py` clusters the 227
  registry squares into 71 spot anchors (territory anchor adjacency,
  recursive visibility split, mass weighted centroids, species
  respawn windows; the Mobius spawn XML path stays ready - GitLab
  throttled the clone of this session, the measured 15-20 s window
  applies).
- The engine: `hunt/spot*.go` - the leash square inscribed in the
  2048 visibility circle, the respawn overlay over the kills, the
  wait-or-move economy (patience 20 s, drift to the predicted corpse,
  25 percent switch hysteresis, 60 s starvation), the measured adena
  per active minute and the decayed per spot death heat replacing the
  gear gates and the band demotion, the fleet occupancy division, the
  white-green window wired into the windowed target search of state.
- The views: ZoneView carries the spot economy (JSON encoder
  extended), map.js draws the circles with the live labels,
  `tools/visualize_hunt_spots.py` renders the standalone interactive
  map (tiles embedded, tooltips, spot table, --simulate session demo).
- The legacy square zones stay as the dual registry mode
  (SetHuntingZones/SetHuntingSpots mutually stand down); all 40+ new
  unit tests plus the full suite pass, gofumpt clean.

### Status: done (2026-09-10)

- Verify loop: go build ./..., go vet, go test ./... (all packages
  green), gofumpt applied, the HTML map checked in a headless browser
  (71 circles, 4 tiles, tooltips, table, zoom/pan, click-to-center).
- The deliverable copy: download/hunt_spots_map.html (with the
  simulated session) + spot_viz_overview.png / spot_viz_zoomed.png.

## Active task: the real C1 client switch - the restart dance

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian): the cross-bot switch of
`51e716a`/`ae48f04` breaks on the REAL C1 client (the fake harness
client of the E2E never caught it). Switching between distant
characters crashes the client with `General protection fault!
History: UNetworkHandler::Tick <- Function Name=UserInfoPacket <- ...`;
switching between nearby characters does not change the character and
renders as if many monsters around died at once. The task: research
what the C1 client actually needs to allow a mid-session character
switch (the teleport hypothesis included) and implement it together
with the correct position, appearance, class and itemization.

### Root cause (researched from the Mobius C1 sources + the symptoms)

- The C1 client binds its PlayerPawn (the "self") to the object id of
  the login flow; a `UserInfo` (0x04) is only ever about the SELF
  character. The switch machinery sent a `UserInfo` carrying the NEW
  bot's object id:
  - Distant target: the id is unknown to the client's actor table ->
    the `UserInfoPacket` handler dereferences a missing pawn -> GPF.
  - Nearby target: the id exists as a remote pawn (CharInfo) -> the
    client updates that actor but keeps controlling its old pawn ->
    "the character does not change"; the full-history replay of the
    new bot (every Die/Attack/MoveToLocation of its session) plays
    the deaths "at once".
  - The `TeleportToLocation` for the unknown id is meaningless and
    the DeleteObject sweep never removed the old self pawn anyway
    (KnownObjectIDs excludes the self).
- There is NO packet that swaps the in-world self pawn of a C1 client
  onto a different object id. The only mid-session identity change the
  client implements is the **Restart flow** (the in-game Restart
  button): `RequestRestart(0x46)` -> the server answers
  `RestartResponse(0x74, result=1)` followed by `CharSelectionInfo`
  (the exact pair of `RequestRestart.handlePacket` of the Mobius C1
  server) -> the client tears its own world down and returns to the
  char select screen -> the character is picked
  (`CharacterSelect 0x0D`) -> `CharSelected(0x21)` -> `EnterWorld
  (0x03)` -> the full enter world burst (UserInfo with the new
  race/class/paperdoll, ItemList, SkillList, spawns). Position,
  appearance, race, class and items all arrive through the packets
  designed for exactly this transition.

### Implementation (the restart dance)

- `beginRestartSwitch` plays the official pair (`RestartResponse` +
  the char list of the target bot), swaps the session and arms the
  **auto select**: after 1.5 s the proxy itself serves the
  live-patched `CharSelected` - the exact answer the user's own double
  click of the only listed character would produce. The user's click
  also still works (it cancels the timer; both paths are idempotent).
- The relay parks at the char select screen (`parkAtCharSelect`) and
  waits for the read loop to report the re-entry; newer WebUI
  selections re-offer the newest char list, and the served
  `CharSelected` always re-resolves the newest selection. The relay
  goroutine stays the single owner of the stream for the whole life
  of the connection (no second relay, no lost selections).
- The relogin handoff rides the same dance (a relogged character gets
  a fresh object id from the server's id factory, so the old teleport
  resync had the same GPF): the held client is restarted onto the
  replacement session with the auto select - the AFK user re-enters
  the world of the same character without clicking anything.
- The teleport + DeleteObject sweep machinery is removed
  (`resyncWorld`, `buildTeleportToLocationPacket`,
  `buildDeleteObjectPacket`, `maxReplaySeq`, the oldKnowns hold
  snapshot); `holdForRelogin` waits for the replacement to be online
  WITH its `CharSelected` recorded before it dances.

### Status: done (2026-09-10)

- The switch tests rewritten for the dance (the packet pair, the char
  list content, the live-patched answer, the re-entry replay, the
  manual-click fallback, the mid-dance re-selection, the pending
  target), the relogin handoff tests rewritten (the dance of the
  replacement, the late-CharSelected hold), go test ./... -count=1
  green, -race green, golangci-lint clean, the live stack
  `proxy_e2e.sh` -> PROXY_E2E_OK.
- The real C1 client check of the dance is the user-side step per the
  project workflow; the auto select delay (1.5 s) is the knob to tune
  if the client needs more time to render the char select screen.

## Finished task: the hunting system redesign research (spot model)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user request (2026-09-10, Russian): the current zone hunting system
is unsatisfying. (1) The bot visibility follows the world region grid,
so parts of a zone do not render their mobs when the bot stands on the
opposite side. (2) The 227 zones overlap heavily and do not reflect the
actual monster concentrations. (3) The bot either drifts across the map
or wastes walking on premature zone rotations instead of waiting out
the respawn; the gear-score gates and the per-band death demotion
proved unreliable. The new system must farm white-green mobs (a few
levels below the bot, full drops, fast kills, maximum gold for the
equipment pipeline). Deliverable: the research/design document.

### Method and findings

- Deployed from the repo data only: GitLab returned HTTP 403 for every
  endpoint of this sandbox (git clone, the archive API, even the site
  root), so the live Mobius stack could not be brought up; the analysis
  runs on the generated registry `hunt/zones_elven.go` (the parsed
  ElvenStarting.xml), `npcdata/names.go` (aggressive flags, aggro and
  clan help ranges) and the live-validated server facts of AGENTS.md.
  The next session should retry `tools/swarm_fast_deploy.sh` first.
- `tools/analyze_hunt_registry.py` (new) computes the numbers:
  227 squares / 73 territories / 722 mobs, median 3.0 mobs per square,
  same-band overlap mean 64% (192/227 squares >30% overlapped),
  guaranteed visible radius 2048 vs zone corners 1840-2690 from the
  center and 2800 median opposite-edge distance, rotation cost 10 s
  patience + 14 s median walk against the 15-20 s respawn window
  (the system always abandons squares that refill faster than it can
  walk away), 33% aggressive spawn mass of which 86% sits in the 16-19
  band the ladder pushes bots into, 55/227 squares farther than 15 000
  units from the village.
- The design (docs/hunting_system_redesign.md): Spot = anchor +
  radius <= 2048 (visibility invariant) from a hotspot clustering of
  the spawn mass (prototype: 30 spots vs 227 squares); a respawn-aware
  overlay (kill position + death time -> predicted respawn, Mobius
  schedules death + rnd[15,20 s] with fixed spawn points by default)
  driving a wait-or-move expected-value decision with 15-20 s patience;
  soft/hard leashes against drift; the white-green window [L-5, L]
  with the [L-4, L-1] preference; safety and gear gating replaced by
  measured efficiency (loot value per active minute, HP lost per kill,
  per-spot death/flee EMAs with decay) in a hysteresis spot switch;
  fleet capacity sharing (bots of one process divide the spots).

### Status: done (2026-09-10)

- Committed: docs/hunting_system_redesign.md, tools/analyze_hunt_registry.py
  (+ the generated docs/hunt_analysis charts and summary.json).
- The docx render of the design document is delivered to the user's
  Downloads as hunting_system_redesign.docx (19 pages, cover + TOC +
  6 charts + 5 tables, postcheck 0 errors).
- GitLab remained HTTP 403 for the whole session (retried at wrap up);
  the live respawn-position verification is the first task of the next
  session together with the phase 1 implementation.
- Next steps for an implementation session: the phase 1 of the plan
  (the spot registry generator + the hunt data model), then phase 2
  (the respawn-aware wait-or-move in the engage loop).

## Finished task: the fleet UX round - proxy switch hardening, rest icons of every bot, armor-first shopping

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push (three foreign commits landed mid task: 70d1664, 075069b,
0ede8a3 - all pulled cleanly, no conflicts).

### Goal

The user request (2026-09-10, Russian), three items:

1. **The proxy client switch** (round 12 landed the live resync machinery:
   `51e716a feat: proxy switches connected C1 client to the WebUI selected
   bot`): a connected C1 client watching bot 3 must switch to bot 2 the
   moment the WebUI selects it - correct position, appearance, race and
   class. This round hardens the remaining gap: a target that is offline
   (registered but still connecting) at the selection moment never
   switches later, and re-clicking the same bot id is a no-op, so the
   client stays on the wrong bot forever. The relay must keep serving the
   current live feed while it polls for the target to come online and
   resync then.
2. **The rest icon of every bot**: the zZ marker of the map draws only
   for the own character of the observed bot (`drawSelf` reads
   `character.sitting`). The other bots of the fleet (player objects of
   the observed bot's world) show no rest icon because the tracker drops
   the ChangeWaitType broadcasts of foreign object ids
   (`ApplyWaitType` returns early) and the CharInfo standing state byte
   is not parsed. The icon must render for every bot on the web UI
   regardless of the focus.
3. **The armor-first shopping order**: the purchase phases must change -
   at the start of the game the bots buy the CHEAP ARMOR first (not
   jewelry), the jewel floor moves behind the weapon milestone, and
   jewelry (the floor and the upgrades) is bought only after every armor
   slot is filled and a new weapon was bought.

### Progress (atomic commits)

- 2026-09-10 (task 2 done): the rest icon of every bot. The tracker now
  keeps the sit state of foreign creatures: `ApplyWaitType` updates the
  world object when the broadcast is not the own character (it only
  tracked `char.Sitting` before), `objectCold.Sitting` stores it, the
  CharInfo standing state byte is parsed (`CharInfoPacket.Standing`,
  previously skipped) and feeds `PlayerInfo.Sitting` of the player
  objects, and the snapshot objects carry the new `sitting` field (the
  golden append encoder, the live view and the reflection path stay
  byte identical - pinned by the existing equality tests). map.js
  extracts the breathing zZ into `drawRestMarker` and draws it over
  every sitting unit of the observed bot's world (the other bots of the
  fleet), not only over the own character; the sidebar rest chips
  already covered all bots through the /api/bots payload. New tests:
  `TestForeignWaitTypeTracksRest`, `TestCharInfoCarriesSitting`, the
  CharInfo sitting subtest, and the two harness checks of
  `tools/repro_map_render.js` (the sitting player object draws zZ). The
  pre-existing harness failure "hunting zone carries the label" fails
  on the clean tree too (not this round).

- 2026-09-10 (task 3 done): the armor-first purchase order. The
  phases of `shopStrategy.classify` are now armor floor (0) -> weapon
  milestone (1) -> jewel floor (2) -> defense upgrades (3): the
  cheapest armor piece of every empty armor family (chest, legs,
  head, gloves, feet - `cheapestArmorIDs`, cached like the jewel
  floor) opens the journey ahead of every weapon and jewel; the jewel
  floor gate moved from "any level" to "a real weapon is worn"
  (`view.anchor > 0`, the starter kit anchors zero), so no jewel is
  ever planned before the armor is assembled and the first weapon
  milestone landed. The shield stays OUT of the armor floor (it
  shares the hand family with the two hand milestones - a floor
  shield would block the same-trip two-hander; it remains a defense
  upgrade inside the weapon budget). The journey table: levels 1-3
  buy the shoes/gloves/cap, level 5 lands the Short Sword and only
  then the Magic Ring / Apprentice's Earring / Necklace of Magic,
  the tiers past it unchanged. Tests: the jewel-floor-first pin
  became `TestPlanPurchasesArmorFloorFirst` (the armor-only opening
  + the jewel floor opening behind a worn real weapon), the journey
  rules now pin "the opening trip buys armor only" and "no jewel
  before the first weapon", the two hunt trip tests follow the new
  stop (Ariel instead of Creamees). docs/shopping_strategy.md
  describes the four phases and the new was/is table.

- 2026-10-10 (task 1 done): the proxy switch hardening on top of the
  round 12 machinery. The gap: a selection whose target bot was not
  online yet (registered but still connecting) logged "not online,
  staying" and the client NEVER switched - the target entering the
  world later refires nothing, and SelectBot of the same id is a
  no-op, so the client stayed on the wrong bot forever. The fix:
  `serveBotSwitch` now returns the target id together with the
  resolved session; an unresolvable target arms `pendingSwitch` on
  the relay cycle, and a 250 ms poll ticker
  (`retryPendingSwitch`) retries the resync while the current live
  feed keeps flowing - the client lands on the target within one
  period of it entering the world, without a reconnect or a re-click.
  A new selection supersedes the pending (the channel fires and
  overwrites it), the relogin handoff re-delivers it through the
  closed channel of the next cycle, and the replay start of the
  switched session moved into `botSwitchReplaySeq`. The new test
  `TestProxySwitchToOfflineTargetCompletesWhenOnline` pins the full
  offline-then-online path (the teleport carries the live position
  and the new self id). docs/proxy.md "Switching bots" documents the
  immediate switch and the pending behavior (it still described the
  old reconnect-only flow). Live: PROXY_E2E_OK against the deployed
  stack after the change.

### Acceptance criteria

- Task 1: `serveBotSwitch` resolves an offline-but-registered target by
  polling without stalling the live feed; the switch completes when the
  target enters the world; docs/proxy.md documents the immediate switch;
  tests cover the offline-then-online switch.
- Task 2: `ApplyWaitType` tracks the sit state of foreign objects,
  CharInfo's standing byte feeds the player object, the snapshot objects
  carry `sitting`, and map.js draws the breathing zZ over every sitting
  unit (the own character keeps its marker).
- Task 3: `shopStrategy.classify` orders the phases armor floor -> weapon
  -> jewel floor (gated on a worn non-starter weapon) -> defense; the
  opening trip of the journey buys cheap armor pieces only; no jewel is
  bought before the first weapon; the journey table and
  docs/shopping_strategy.md describe the new order.

### Status: done (2026-09-10)

- All three items landed as their own atomic commits (the rest icon
  round, the armor-first shopping round, the proxy switch hardening),
  each pushed right after its verify loop.
- Verify loop of the final round: go build/vet, gofmt clean,
  go test ./... -count=1 (18 packages, 0 failures), golangci-lint 0
  new issues (the pre-existing unparam on pathfind/search_test.go and
  the gofumpt finding on version_test.go remain).
- Live: the deployed stack (STACK_READY 2106/7777/3306, 75 tables),
  tools/mobius_e2e.sh E2E_OK, tools/proxy_e2e.sh PROXY_E2E_OK, and a
  100 s smoke fleet of 3 bots (-bots 3 -hunt -proxy -web) hunting
  with the proxy listeners up and the graceful SIGINT shutdown; the
  live API carries the new object `sitting` field and the bots see
  each other as player objects. The real C1 client check of the
  cross-bot switch remains the user-side step (see docs/proxy.md).
- The next agent note: the map render harness still carries the
  pre-existing "hunting zone carries the label" failure (fails on the
  clean tree too, not this round).

## Active task: the looted gear of the shopping list survives the junk flows

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.

### Goal

The user request (2026-09-10, Russian): verify that when an item that
is on the shopping list drops for the bot, the bot does not sell it
for its instant adena - it puts it on and uses it.

### Root causes and fixes

- The purchase side was already safe: `PlanPurchases` plans against
  the simulated paperdoll (`SimulateInventory`), so a looted item id
  the inventory carries is never bought twice ("nothing gets bought
  that the inventory already carries" - pinned by
  `gear` `TestPlanPurchasesSkipsInventoryItems`).
- The sell side was NOT safe: `state.Bot.SellableItems` lists every
  unequipped non-adena non-quest item, with no knowledge of what the
  auto equipment is about to wear. The rescue was pure timing - the
  auto equip request (paced 2 s, confirmed through the shared gate)
  usually flips the equipped flag before the first sell batch leaves.
  The race windows: the two-step pair swap (the better jewel waits
  for its use request while the displaced piece already came off),
  the confirmation window of an in-flight equip, a refused equip.
  Reproduction: `hunt` `TestLootedGearSurvivesTheSellStop` failed -
  the looted Short Sword went out in the first `SellItems` batch on
  the very tick the equip request was sent.
- The destroy side was worse: `DestroyableItems` ranks gear drops
  FIRST (before common stackables), so the overflow cleanup
  (70 percent slots) destroyed a freshly looted unequipped upgrade
  before the junk mats. Reproduction:
  `TestLootedGearSurvivesTheCleanupDestroy` failed - the destroy
  batch ate the sword.
- The fix introduces the planned equip keep set:
  `gear.PlannedEquips(profile, equipment)` collects the object ids
  the gear simulation places on the virtual paperdoll but that are
  not equipped yet - exactly the pending wearables the auto
  equipment walks through step by step. The hunt loop caches the set
  per inventory mutation (`equipManager.keepsCache`,
  `Loop.plannedEquipKeeps`) and passes it to the new junk filters
  `state.Bot.SellableItemsExcluding(keep)` and
  `state.Bot.DestroyableItemsExcluding(keep, limit)`; the plain
  methods delegate with nil. The kept pieces: looted upgrades,
  bought arrivals waiting for their paced equip, the better halves
  of pair swaps mid flight. Still junk (correctly): duplicates,
  looted downgrades, the displaced weaker halves of pair swaps.
- Three hunt tests pin the behavior end to end: the sell stop keeps
  the looted sword (the batches sell around it, the trip still
  completes), the overflow cleanup destroys the stackables behind
  the kept sword, and the pair swap window sells the displaced
  apprentice earring while the looted mystic earring survives and
  wears.

### Status: done (2026-09-10)

- Verify loop: go build/vet, gofmt clean, go test ./... (18
  packages, 0 failures), golangci-lint 0 new issues in the touched
  files (the pre-existing goconst on slots.go, the gofumpt on
  version_test.go and the nolintlint/unparam findings in untouched
  files remain).

## Active task: the shop strategy rework - the purchase phases (jewel floor, weapon, defense)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push (7 commits landed mid task: the packet reader/writer perf
rounds; pulled cleanly, no conflicts).

### Goal

The user request (2026-09-10, Russian): the purchase order of the NG
items is wrong for the starting locations. (1) Nothing there attacks
with magic - the cheapest first jewel set suffices until level 15+,
the jewel ladder is a waste below it. (2) The melee characters want
the weapon first, then the armor with the maximum defense, then the
next weapon tier. Rework the purchase order logic, study the server
prices, and deliver the comparison table of every NG purchase from
level 1 to 15+ as "was" and "is".

### Root causes and fixes

- The old planner ranked EVERY purchase by `gain / price` (greedy
  value per adena). The cheap empty slot fillers (Apprentice's Shoes
  8 pDef for 8 adena - 0.99 pDef per adena) outranked every weapon,
  so a fresh character spent levels 1-4 on shoes, gloves, caps and
  shields before the first Short Sword, bought the intermediate
  weapon ladder (Heavy Chisel -> Knife -> Sickle) whose steps resell
  at reference/2, and climbed the jewel ladder in magic-free zones.
- The rework phases the walk (`shopStrategy.classify` in
  gear/shopping.go): the jewel floor (the cheapest jewel per family
  fills the empty slots at any level - the basic outfit), the weapon
  milestone (only the best value STRICT weapon upgrade is eligible -
  the saving target; a cheaper worse value weapon never intercepts
  the save up) and the defense upgrades (ranked by the raw defense
  gain, bounded by the weapon budget: the reference value of the
  worn defense gear may not exceed the reference price of the worn
  weapon - the weapon leads the progression, the defense follows
  inside its tier budget). The jewel upgrades gate on level 15
  (`jewelUpgradeLevel`): below it only the floor items are planned,
  past it the upgrades join the defense phase.
- `PlanPurchases`/`PlanPurchaseQueue` grew the character level
  parameter (the hunt loop passes the tracker's `SelfLevel`), the
  virtual paperdoll entries carry the item id (the anchor and the
  defense pricing read them), and the whole file went through
  gofmt (the working tree copy had lost its tabs).

### Status: done (2026-09-10)

- The journey simulation test
  (`gear/shopping_strategy_test.go`:
  TestShoppingStrategyJourneyComparison) walks the elven fighter
  from the creation screen (the Squire's kit, zero adena) through
  level 20 once per planner - the legacy greedy copy (pinned as the
  comparison baseline) and the phased planner - with the income
  model built from the Mobius data (experience.xml exp per level /
  the mob exp of the level's ladder step, the npc adena drops at 70
  percent), one town trip per level, the sells, the server-side
  affordability re-check (the planner overprices the starter kit
  credit the shops refuse) and the auto equipment walk. It prints
  the was/is table and pins the ordering rules: the jewel floor
  first, the first weapon before any armor, no jewel upgrade below
  15, the jewel upgrades past 15, the greedy planner's filler
  detour (10 non-weapon buys, the Apprentice's Shoes opening).
- The comparison table and the waste analysis (the filler detour,
  the intermediate weapon ladder, the jewel ladder in magic-free
  zones, the shield ladder after the two-hander) live in
  docs/shopping_strategy.md ("The was/is journey of an elven
  fighter"); the strategy section describes the three phases.
- Verify loop: go build/vet, gofmt clean, go test ./... (18
  packages), golangci-lint on gear/hunt (only the pre-existing
  goconst on slots.go and nolintlint on plan.go remain), the live
  stack deployed fresh (STACK_READY: 2106/7777/3306, 75 tables) and
  tools/mobius_e2e.sh E2E_OK.

## Active task: rest at the kill spot, finish fights across the zone line, zone free loot

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push (one push landed mid task: the standing hunter round).

### Goal

The user report (2026-09-10, Russian), three hunt behavior complaints:
(1) the character runs too far away after a fight before it sits down
to rest; (2) the character stops interacting with the mobs when the
fight carries it out of the hunting zone - it should finish them off;
(3) the character does not always pick up ground items - the drops
outside the hunting zone must be picked up regardless. The game server
stack was already up on this Windows host (login 2106, game 7777,
db 3306 verified) and had to stay untouched.

### Root causes and fixes

- The escape threat lookup fell back to the nearest living attackable
  npc when no mob held the character as its target. A finished fight
  leaves a fresh 3 s under attack window (the dying mob's last blow),
  so the hurt character armed the flee against a passive bystander
  and ran up to three 700 unit legs away from the kill spot before
  resting. Fix: `threatPosition` drops the fallback - the escape runs
  from the living engaged target or a real attacker only
  (`NearestAttacker`), otherwise the rest happens where the fight
  ended.
- The zone leash of the engage dropped the fight the moment the
  character stood outside the square (`returnToZone` cleared the
  target and walked home through the blows). Fix:
  `adoptOutZoneFight` (loop_movement.go) adopts a live fight before
  the walk home - the own living target, the fresh server selection
  or the nearest attacking chaser (never a flee-skipped target) - and
  the engage flow finishes it outside the square; without a live
  fight the leash walks home unchanged, new fights still start inside
  the square only.
- The loot search passed the hunting zone filter: drops past the
  square line stayed on the ground forever. Fix: `loot()` searches
  without the zone - anything within the 900 unit loot radius of the
  character is picked up, wherever it lies.

### Status: done (2026-09-10)

- Four new tests pin the behaviors (rest at the kill spot, the fight
  continues outside the zone, the chaser is fought back, the loot is
  picked up past the line) - all four verified to fail on the old
  code (stash round). The two flee tests grew the missing Attack
  broadcast: the fleeing mob must actually hold the character as its
  target for the escape direction.
- Verify loop: go build/vet, gofmt clean, go test ./... (18 packages),
  golangci-lint (only the pre-existing unparam on
  pathfind/search_test.go).
- Live: the bot hunted the deployed stack directly (the real login
  server at 127.0.0.3:2106 - the proxy Recipe A layout of this host;
  game 7777) for 3.5 minutes: zone pick, kill, loot and the rest 3 s
  after the kill log line - the rest happened at the kill spot, no
  escape run (fix 1 demonstrated live); the pile up safety layer
  cycled its documented run + logout + relogin when the clanned orc
  pack joined. Hard kill stop (the SIGINT pitfall), the shutdown path
  is untouched by this round. Development log Round 45 carries the
  full writeup.

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
## Active task: the equipment widget skills view review fixes

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.

### Goal

The user review (2026-09-10, Russian) of the skills-display task found
three defects: (1) the sidebar had TWO queue toggles - it must have
exactly one, and the left flyout it opens must follow the widget tab
(the item purchase queue under EQUIPMENT, the skill learning queue
under SKILLS); (2) switching to the SKILLS tab kept the equipped gear
pictures visible - the widget must behave like real tabs, the switch
fully replacing the visible content (weapons/armor/weight on one tab,
skills only on the other); (3) the learned skills hung in the air when
few - the skills tab needs a small grid of ready cells with the
placeholder slots of the future elements.

### Root causes and fixes

- Defect (2) had two roots. The `applyGearMode` toggler targets
  `document.getElementById("gear-main")`, but the markup div carried
  only the class - the id was missing, so the `mode-skills` class
  never landed and `#gear-view` never turned invisible (the harness
  stub DOM lazily fabricates any id, which masked it: the harness was
  green while the real browser showed the bug). Even with the
  visibility fixed, the paperdoll icon/glyph/badge cells stack at
  z-index 1..3 and would still paint ABOVE a z-index auto sibling
  overlay. The fix: the markup gains `id="gear-main"`, and
  `.skills-view` gets `z-index: 4` so the overlay paints above every
  gear child - the tab switch now fully replaces the content, and the
  harness pins the id and the z-index against the real html/css
  strings.
- Defect (1): the second `skillq-tab` triangle (and its below-shop
  docking) is gone; the single `shop-tab` triangle now owns BOTH
  queues through the shared `QueueFlyout` open state and
  `applyQueueFlyoutState`: exactly one flyout is out at a time - the
  shop plan in EQUIPMENT mode, the lesson plan in SKILLS mode - and
  switching the widget mode re-docks the open state to the queue of
  the new view. The tab hides while the current view owns no queue.
- Defect (3): the learned grid became a small fixed grid (the bag
  metric: six 36px columns, four visible rows, 153px) padded with
  dashed `.skill-cell.empty` placeholders - complete rows, at least
  `SKILL_GRID_MIN_CELLS` (24) - so the learned skills sit in ready
  cells and the trailing slots read as the future lessons; an entirely
  empty filter tab shows the muted note instead. The sp/next foot is
  anchored to the panel bottom (`margin-top: auto`) like the
  adena/weight footer of the equipment view.

### Status: done (2026-09-10)

- Verify loop: repro_gear.js 157 checks green, repro_hud/fight/movement
  green (repro_map_render has one pre-existing failure - the hunting
  zone label - present on the clean tree too), go build/vet, go test
  ./... (all packages), golangci-lint 0 new issues (9 pre-existing in
  untouched files), live browser check: the mode swap swaps the flyout
  (shop out in gear, skill queue out in skills), no gear icon bleeds
  through the overlay (elementFromPoint returns only skills view
  nodes), the passive tab renders 1 learned cell + 23 placeholders,
  the foot sits flush at the bottom.


## Active task: the town trip water stuck - the lake under the elven village

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian): the bot got stuck trying to
return to the town - it ran into the water below the elven village
plateau and stood there (the state dump: the character at 47136 46564
-3738 floating over the lake bed, phase townWalk, the three re-path
budget burned, the plan pointing at the village cliff top). Three
asks: (1) how to correctly walk out of that point, (2) how the
character ended up there at all (the possible paths that produced the
final route), (3) a walk plan dump that shows the whole walk from
where we wanted to go to where we want to arrive, with a marker on
the current target waypoint - to debug such cases at a glance.

### Root cause (researched from the Mobius C1 sources + the geodata)

- The server never refuses water: swimming move requests skip the
  geodata validation entirely (straight to the click, clamped to 700
  units), the lake beds are ordinary walkable slopes to
  getValidLocation, and a failed server pathfind falls back to a
  straight blind walk.
- The swim z (-3738) floats ABOVE the water zone bound (-3780): the
  zone membership flip flops, and in the dry phases every click
  toward the village deck resolves onto a layer the lake bed has no
  walkable connection to - getValidLocation answers with the
  character's own position (a zero length walk), so the character
  never moves again and every re-path replans the same geometry.
- The skip rule aimed the follower at the closest waypoint (the
  unclimbable cliff top) instead of the planned northern escape leg -
  and the dump only showed the remaining tail, hiding the escape.
- The planner itself always routed dry (verified from every zone
  position): the character entered the water through the server side
  routing of the per click legs. The bot's own smoothing could,
  however, collapse dry legs across water (line of sight is water
  blind) - fixed as part of the defense in depth.

### Implementation

- pathfind: the smoothing keeps the shore detours (legDry - a
  collapsed leg between two dry points must stay dry); new engine
  queries OverWater, DryLine and FindWaterEscape (the BFS flood to
  the nearest shore over the walkable surface).
- hunt: the town walker refuses wet click lines (DryLine before every
  WalkTo, the re-paths share the trip budget) and recovers through
  the water escape (OverWater arms the shore walk, a stuck escape
  re-plans itself, a dry character re-plans the interrupted leg from
  the shore with a fresh budget).
- The walk plan (state, dump, map): the origin, the FULL waypoint
  list, the follower cursor (the `<-- TARGET` marker / the map ring)
  and the destination - plus `(passed)` markers in the dump.

### Status: done (2026-09-10)

- Verify loop: go build/vet, gofumpt clean, go test ./... (all
  packages green), golangci-lint: zero new findings over the branch
  base (19 pre-existing in the parallel agents' spot/zones code).
- Regression tests: the real geodata pack (the reported stuck position
  escapes to a shore, the route to Ariel stays dry leg by leg) and
  the synthetic engine/walker suites of water_escape_test.go and
  water_guard_test.go; the dump format pinned in webserver.
- See docs/development_log.md round 46 for the full root cause chain.

## Active task: the relogin ground fix and the aggro-aware movement

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

The user report (2026-09-10, Russian): the bot often runs to a hunting
ground THROUGH aggressive mobs, arrives with the train, resets it with
the emergency relogin - and the fresh session then walks straight to
the NEXT ground, ignoring the one it just arrived at (the user's
hypothesis: the mobs are invisible at the relogin moment). Plus the
feature request: teach the bot to move from A to B AROUND aggressive
mobs at a safe distance whenever they are not the walked-to target -
between the grounds, on the town runs in both directions, and while
already fighting (stepping clear of an impending second opponent
beats the pile up logout of the real one).

### Root causes (found in the spot economy)

- The fleet occupancy claim of the hunted spot LEAKS on every session
  death: `spotHunter.leave` is only invoked inside `apply` when a LIVE
  session switches grounds, but `runBot` builds a fresh Loop (and a
  fresh spotHunter) per session - nothing releases the claim of the
  dead loop. Every emergency relogin left one more ghost hunter in the
  process wide `globalSpotHub`, and the occupancy division of the
  picker halved the score of the standing ground with each cycle: the
  fresh pick after the relogin preferred the neighbor ground - exactly
  the reported "goes to the next zone right after the relogin" loop,
  compounding with every aggro reset.
- The fresh session also re-contested the whole spot economy instead
  of resuming the ground the character stands on: the panic run leaves
  it a few hundred units off the anchor it had just walked to, and
  with the ghost claim discount the scored pick had every reason to
  walk away.

### Implementation (commit 1 of 3: the relogin ground fix)

- `Loop.Run` releases the spot claim through a deferred
  `releaseSpotClaim` (`spotHunter.releaseClaim`): the claim lives
  exactly as long as the loop goroutine, so the 24/7 supervisor's
  session cycle (the emergency logout, a server restart, a lost
  connection) hands every claim back.
- The first pick of a fresh session (`spotEvaluate`) checks
  `standingGround` first: a character entering the world inside a
  spot's circle (the spawn mass the spot covers, wider than the leash
  square - the panic run endpoint) resumes THAT ground when it stays
  inside the character's level window; the log line
  "resuming the spot ... - the login landed on its ground" names the
  handoff. A relogin between the grounds (a session that died
  mid-walk) still falls to the scored pick.
- Tests: `spot_relogin_test.go` - the claim released on the session
  end (the Run cancel path), the reported scene end to end (the
  relogin on the ground resumes it even with a ghost claim halving
  its score), the off-ground scored pick, the outgrown ground never
  resumed, the nearest-anchor tie break of overlapping circles.

### Status: done (2026-09-10)

- Three commits pushed: 3aa894c (the claim leak fix + the
  standing-ground resume), 1143eb9 (the transit walk steering),
  f8445e7 (the impending-add step of the running fight). The
  permanent root-cause history lives in docs/development_log.md
  Round 46.
- Verify loop: go build, go vet, the full go test suite, gofumpt
  clean, golangci-lint with zero new findings (the 26 pre-existing
  findings of the branch reproduce on the untouched tree), -race
  green on the hunt and state packages.
- The live stack: the GitLab git-clone throttle of this sandbox broke
  the stock deploy (the clone hangs at 116K); the official API archive
  of the same repository unblocked it (the script's own fallback
  channel b), the stack then compiled and started - login 2106 up,
  75 tables loaded - but the sandbox reaps every background process
  once the invoking shell exits (the documented restricted-shell note
  of mobius_start.sh), so the live validation ran through
  mobius_e2e.sh in a single invocation: E2E_OK with BOT_FLAGS=-hunt
  (90 s), the spot anchoring, the pathfound return through the
  steering hooks and the keltir farm cycle all observed in the bot
  log.
- The user-side check on the real C1 client stays the project
  workflow: watch a bot that relogins through the emergency logout
  resume the same ground (the "resuming the spot ... - the login
  landed on its ground" log line) and watch a transit walk bend
  around an aggressive camp (the "steering the walk around" line).
## Active task: the bridge entry, the starter dagger and the shop queue floor

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian, three bugs):

1. The bot rams the village bridge from the railing side on the town
   returns: the pathfinder plans the smooth detour (the semicircle
   onto the bridge ramp) but the waypoint follower accepted or skipped
   the entry waypoints it never walked through - the arrival radius
   (150 units, ~9 geodata cells) plus the "the next waypoint is
   closer" skip rule let the cursor jump past the bridge entry while
   the character stood beside the deck, and the straight leg then
   ground into the railing (state dump: "town walk stuck, re-pathing
   1..3 of 3", "aborted, walk stuck").
2. The bot sold its sword, auto-equipped the starter dagger and the
   shop strategy plans to buy the old sword back forever: the elven
   fighter's Dagger (item 10) is the fourth unsellable, undroppable
   newbie item (is_sellable=false, is_dropable=false in the Mobius
   item xml - verified) but it is missing from gear.starterSet, so the
   destroy machinery ignores it and displacedValue credits its phantom
   sell value into every weapon plan.
3. The web UI: switching between bots sometimes shows no farming zone
   circles (the spot registry publishes only after the first spot
   pick - a bot that starts a town trip or a delevel at login never
   runs the picker) and no shop queue at all (the trip view is empty
   while the walk to town runs: the stops are planned only at the
   shop, so publishShoppingView clears the plan); the queue must also
   hold at least 3-4 entries instead of the lone sword milestone.

### Acceptance criteria

- The follower only counts a waypoint reached within the tight
  intermediate radius (50) and only skips one the character passed ON
  the route (projection past the waypoint, lateral within the
  corridor); the final waypoint keeps the wide arrival radius. The
  same rule in all three followers (town, manual, blind recovery).
- The Dagger joins the starter set: destroyed once a better weapon is
  worn, never credited as sell value, never offered as junk.
- SetHuntingSpots publishes the registry view at install; the shop
  widget keeps the triggering plan through the town walk; the widget
  queue never falls below 4 entries while candidates remain.

### Status: in progress (2026-09-10)

- The environment deployed (STACK_READY), the code surveyed, the
  Mobius item xml and RequestSellItem/RequestBuyItem verified for the
  unsellability and the price truncation parity (Go and Java truncate
  the same IEEE double - no price fix needed).

- Commit "the waypoint follower walks the bridge entry, not past it":
  the intermediate waypoints of all three followers (the town trips,
  the manual moves, the blind engage recovery) now count as reached
  only within the tight 50 unit radius (the final waypoint keeps the
  wide 150 trip arrival) and the legacy "the next waypoint is closer"
  skip rule became the projection pass test (the character must stand
  PAST the waypoint within the 100 unit corridor of the wp -> next
  segment - a character beside the route, the bridge railing side,
  keeps targeting the entry it missed). Covered by
  hunt/waypoint_follow_test.go (the pass geometry table, the two
  radii, the reported railing-side scene, the 60-units-short scene).
  A gofmt-only commit fixed the space-indented loop_movement.go
  another session left behind. (The rebase onto the concurrent water
  guard round moved the follower core into the shared followWaypoints
  of the water escape refactor - the arrival and pass rules apply
  there, the manual and blind followers keep their own copies.)

- Commit "the starter dagger leaves through the destroy request":
  the Dagger (item 10, the elven fighter's creation weapon,
  is_sellable=false, is_dropable=false in the Mobius item xml) joined
  gear.starterSet - ReplacedStarterItems destroys it once a better
  weapon is worn (the reported loop: the sword sold, the dagger
  equipped, the shop re-planning the old sword forever). The
  displacedValue sell credit of the newbie kit items dropped to zero
  (the phantom 69 adena of the dagger once armed an 850 adena wallet
  against the 883 sword - the trip walked to town and back empty
  every time), gear.IsStarterItem exported for the hunt layer, and
  the junk flows (junkRemaining/sellJunk through sellableJunk) never
  offer the kit pieces anymore. Tests:
  gear/starters_test.go (the dagger destroy, the worn guard, the kit
  membership), gear/shopping_test.go (no phantom credit: 850 adena
  plans no sword, 1300 plans it without SellFirst/SellCredit),
  hunt/town_test.go (the junk excludes the kit).

- Commit "the zone circles and the shop queue survive the bot
  switch": (1) SetHuntingSpots publishes the registry view at install
  time - a bot that starts a town trip, a delevel or a manual walk
  before its first pick never runs the picker (those phases consume
  the ticks ahead of maybeSwitchZone), so the circles exist from the
  login snapshot instead of appearing only after the first hunt
  resumed. (2) publishShoppingView keeps the triggering plan on the
  widget through the town walk: the trip view is empty until the
  stop planning runs at the shop and the empty view cleared the plan
  for the whole leg (the "the shopping list is missing" report).
  (3) The widget queue never falls below gear.shoppingQueueMin (4)
  entries: the wishlist extension of planPurchases continues the
  walk past the one purchase per slot guard with the weapon value
  gate open, so the list reads as the full save-up progression
  (the affordable prefix stays byte identical to the plain plan).
  (4) selectBot resets the map state of the previous bot
  (MapView.resetBot: the snapshot, the runtime objects, the social
  masks, the combat effects drop; the fleet kill marks stay) and the
  zone list cache. Tests: hunt/spot_test.go (the install publish),
  hunt/shopping_test.go (the plan holds through the walk),
  gear/shopping_test.go (the queue floor), the new
  tools/repro_bot_switch.js harness (the map reset, the kill mark
  survival, the repaint) and the full webui harness set.

### Status: done (2026-09-10)

- Three commits pushed: the waypoint follower fix (d3a8316 after the
  rebase onto the concurrent water guard round), the starter dagger
  destroy (7a72993), the zone circles and the shop queue of the bot
  switch (fbb5c59).
- Verify loop per commit: go build, go vet, the full go test suite,
  gofumpt clean, golangci-lint with zero findings in the touched
  files (the pre-existing findings of the branch stay untouched),
  -race green on the hunt, gear and state packages, all six webui
  repro harnesses green.
- Live validation on the local stack (E2E_OK with BOT_FLAGS=-hunt):
  the fresh elven fighter equipped the Squire's Sword and the log
  carries "gear: destroying Dagger (2165): the equipped Squire's
  Sword (2274) replaced it (unsellable, undroppable)" - the reported
  destroy request works; the web API snapshot of a hunting bot
  answers 71 huntingZones (the registry from the login snapshot) and
  a shopping queue of 9 entries (1 affordable + the wanted tail with
  the cumulative missing), not a lone milestone.
- The user-side check stays the project workflow: watch a town return
  walk hold its plan through the bridge entry (no "town walk stuck"
  re-paths at the railing), watch the dagger leave the bag after the
  next weapon lands, switch bots in the web UI and see the circles
  and the shop list of every bot.

## Active task: the delevel freeze under an attached client - the position ping pong
)


Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user request (2026-09-10, Russian): during the deleveling the bot
runs to a guard, hits it, dies, revives - and on the client an attack
animation plays as if it hit again (no visible damage), the bot "sees"
the target (its known list holds the guard after the respawn) and then
stands doing nothing for a long time instead of running back to the
guard. Debug it, find the root cause, cover it with tests, fix it and
explain it. The live dump (build d09e733) shows: the delevel death
cycles run 12:02-12:04, the event log holds full known-object list
flips between the Elven Village and the Starden guard camp every
second (12:04:16-12:04:19), no Die packet and no death event for the
12:04:41 sequence, then 26+ s of silence with the walk plan pointing
at the Starden spawn while the character stands at the village.

### Root cause (reproduced live on the local stack)

The Mobius `ValidatePosition.runImpl` "Check out of sync" branch
accepts any client position report beyond one move speed of the
server position **with no distance bound** and snaps the server side
character to it. The C1 client attached through the proxy reports its
local pawn position periodically, and that local view is stale for
every bot-initiated relocation it did not initiate itself: the death
spot held across the bot's village restart (the pawn freezes while
the death dialog is up), the teleport screen freeze, the long walk.
Every report then dragged the character back - the bot's walk legs
pulled it toward the village, the next report snapped it back to the
guard camp: the observed object list ping pong. The delevel state
machine desynced from the server truth (its tracker only follows the
server broadcasts, the snaps broadcast nothing): the fight stage
believed melee range while the character stood 3000+ units away, the
attacks swung from afar (the "animation without damage"), the guards
"never fought back", both guards got marked as tried, the delevel
aborted, and the return walk stalled in the same ping pong.

Secondary finding: the town guards are level 70 (Kendell/Starden
templates 30218/30220), the deleveling character is in the low tens,
and the vanilla `calcHitMiss` floors the hit chance at 20 percent
there - a whole 20 s fight timeout without a single landed blow is
the normal miss variance of that gap (~27 percent of stages), which
blacklisted healthy guards and aborted delevelings even without the
desync.

### Implementation

- The proxy intercepts the client's ValidatePosition
  (internal/swarm/proxy/game.go transitToServer): a report that
  contradicts the attached session's tracker position beyond
  validateDivergenceLimit (2000 units 2D) never transits; the proxy
  answers the client locally with a synthesized ValidateLocation
  (0x76) carrying the live place of the played character - the same
  correction the server sends for an out of sync report, which heals
  the client's stale view so the next reports transit again.
  In-sync reports transit unchanged (the manual control keeps the
  server contract).
- The delevel fight stage (internal/swarm/hunt/delevel.go) no longer
  blacklists a guard after a timeout window without a single landed
  blow: the tracker records the last landed own blow
  (internal/swarm/state SelfLandedHit, fed from the Attack packet hit
  flags) and the stage extends while nothing landed (the miss
  streak), switching guards only when landed damage went unanswered
  (the stale guard AI).
- Tests: internal/swarm/proxy/validateposition_test.go (the stale
  report corrected locally and never transiting, the healed report
  transiting again, the tracker move shifting the acceptance window,
  the live in-range report transiting unchanged) and the delevel
  suite (TestDelevelMissStreakExtendsFightStage new,
  TestDelevelFightTimeoutSwitchesGuard now requires landed damage).

### Verification

- go build, go vet, go test ./... all green.
- golangci-lint: no new issues in the touched files (the baseline
  holds pre-existing findings of the newer local linter version in
  untouched files).
- Live repro on the local stack (character pushed to the outleveled
  Pincer spot, deleveling to 23, a scripted C1 client attached
  through the proxy): before the fix the delevel broke within two
  death cycles (the guards "do not attack" timeouts, the abort, the
  stalled return walk - the exact user report); after the fix 23+
  death cycles at the healthy 35-40 s cadence, the correction firing
  at every restart (including the exact user signature: the client
  reporting the death spot 42971 51372 against the bot at the village
  45091 49002), the client view healed and following the walks, no
  aborts, no stalls.

### Status: done (2026-09-10)

- The fix, the tests, the docs (hunting.md deleveling section, the
  proxy.md "client position reports" section) and this entry land in
  one commit.


## Active task: the delevel water loop - the walk the planner planned as a swim

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian): the bot hangs cycling forever.
The state dump (build 6255088) shows the exact cycle, 1.3 s per turn,
25+ minutes non stop: "level 11 is too high for level 2 mobs,
deleveling to 9 at the town guards" -> "walking to the guard Starden"
-> "the walk would enter water at 40648 43432, re-pathing around the
shore (1..3 of 3)" -> "town trip ended: aborted, the walk would cross
water" -> the deleveling restarts. The character (test3, level 11)
stands dry at the elven village shore (40648 43432 -3624), HP full,
not moving, the hunting zone behind the bay.

### Root cause

Two defects chained:

1. The planner planned water crossing routes for the shore walks: the
   water step of the search only costs three land steps
   (waterCostMultiplier), so the cheapest route from the shore to the
   guard swam across the bay (the dump walk plan: wp1 z -3800, below
   the water level -3780). The click guard of the walker
   (clickWouldEnterWater) refused the first leg, and the "re-path
   around the shore" re-planned the IDENTICAL route - the search is
   deterministic and its water tolerance did not change - three times,
   then aborted. The planned wet route and the refusing walker could
   never agree.
2. The abort of a walk that runs during the deleveling ended only the
   town trip: abortTownTrip left the delevel state armed without any
   cooldown, the phase flipped to engage, and the very next tick
   delevelWanted() fired again (level 11 over median 2, no cooldown)
   -> startDelevel -> the identical wet plan -> the identical abort.
   An infinite tight loop.

### Fix

- pathfind: the search carries a dry mode (FindPathApproachDry) - a
  step onto an underwater cell costs impassable, so every waypoint of
  a found route stands above the water level (a wet start exits to the
  shore first). The water cost search stays for everything that is
  allowed to swim.
- hunt: startWalkLeg (the shared planner of the town trips, the
  deleveling, the zone returns, the shore re-plans) navigates with the
  dry search; a destination the dry geodata cannot reach reports a
  planning failure - the callers abort the leg and arm their
  cooldowns. The old not-found fallback (a single direct walk the
  server routes itself) is gone: it planned the swim by definition,
  the click guard refused it leg by leg, and the direct walk itself
  runs the character into the lake the guard exists to keep it out of.
- hunt: abortTownTrip during the delevel phase delegates to
  abortDelevel - the cooldown arms (delevelEnd) and the walk home
  starts. Any future walk blocker breaks the restart cycle the same
  way.

### Status: done (2026-09-10)

- Commit "hunt: the shore walks plan dry paths and the water abort
  breaks the deleveling": the dry approach search of the pathfind
  engine (FindPathApproachDry), the shared shore leg planner
  (startWalkLeg) navigates with it, the not-found server-routing
  fallback is gone, and abortTownTrip during the delevel phase
  delegates to abortDelevel (the cooldown arms, the walk home
  starts). Tests: pathfind dry_search_test.go (the channel detour,
  the swim-only strait refusal, the real pack shore route) and hunt
  water_loop_test.go (the delevel walk abort ends the deleveling into
  the cooldown, no refused click reaches the server before the trust,
  the dry miss aborts the deleveling at the planning tick, the town
  trip variant arms the trip cooldown; the two old fallback tests
  rewritten to pin the honest abort). Docs: hunting.md water safety
  and path layer selection paragraphs, development_log.md Round 48.
- Rebase onto the concurrent village water raster round (407c8f2):
  the two rounds compose - the dry planner removes the deliberate
  swims from the plans, the trusted-leg release of that round answers
  the geodata raster artifacts of a planned route (the village plaza
  cells), and the abort of a delevel walk lands in abortDelevel with
  its cooldown. The hunt suite re-verified green on the rebased
  tree.
- Verify loop: go build, go vet, the full go test suite (18 packages
  green), -race green on the hunt package, gofumpt clean,
  golangci-lint zero new findings in the touched files (the 19
  pre-existing branch findings of the spot/zones code stay
  untouched).
- Live validation on the local stack: mobius_e2e.sh 45 and
  BOT_FLAGS=-hunt 90 both E2E_OK - the bot equips, anchors spots,
  pathfinds through the dry planner, engages and loots with no water
  incidents. The real pack probe: the ordinary search plans exactly
  the dump route (wp1 40328 44408 -3800, the swim), the dry search
  plans an all-dry bypass around the bay (15831 units against 11837)
  - the shore detour the "re-pathing around the shore" always
  claimed to do.
- The user-side check stays the project workflow: run a deleveling
  bot from the elven village shore and watch it walk around the bay
  (the planned waypoints all above z -3780) instead of cycling
  "deleveling to 9" / "the walk would cross water"; any trip that
  still cannot find a dry route aborts once with a cooldown instead
  of spinning.

---

## Active task: the npc talk target, the spellbook junk, the teacher walk and the aggro answer

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian, four bugs):

1. The bot must clear its target after talking to any npc: the
   merchant/teacher selection the trips leave behind is re-adopted by
   the engage (the server-side MyTargetSelected of the villager
   survives the trip end), the bot then spends the 12 s engage stuck
   timeout attacking the friendly npc before skipping it.
2. The sell flow must exclude the items that are needed for anything:
   the spellbooks the learning trips buy for the queued lessons are
   plain junk for the junk ranking (unequipped, priced, weighted), so
   the next trip sells the bought book for referencePrice/2, the
   lesson then waits for its book forever and the trip after that
   buys it back - a buy/sell loop that never learns.
3. None of the bots ever reached the teacher npc to learn a skill
   (some own the spellbook already): the learning stop of the town
   trip must be debugged live to the root cause and fixed, so the
   queued lessons actually land.
4. The bot must track the aggro on itself and never run to aggro a
   NEW mob while chased: either engage the attacker immediately and
   try to win it, or switch to the defense mode and log out through
   the standard escape walk (fleeFromThreat -> the flee budget ->
   emergencyLogout).

### Acceptance criteria

- A conversation with a villager (the merchant select, the teacher
  click) ends with the selection cleared: the self click (Action 0x04
  on the own object id, the official client way) replaces it, and the
  engage never adopts a non-attackable selection even if one lingers.
- The spellbooks of the unlocked queued lessons never enter the sell
  batches and never enter the destroy cleanup.
- The live stack shows a bot learning a lesson at the teacher (the
  SkillList bump and the "learned <skill> level N" log line).
- The engage picks the attacker that holds the character as its
  target over any fresh mob while the character is healthy; a hurt
  character or an unbeatable attacker keeps the defensive flow (the
  escape walk, the logout). A town trip interrupted by an attacker
  drops its walk and answers the same way.

### Status: in progress (2026-09-10)

- The environment deployed (STACK_READY, ports 2106/7777/3306, 75
  tables), the hunt/town/learning/state code surveyed, the Mobius
  C1 sources of Action (0x04), PlayerClick, Player.setTarget,
  AttackRequest and RequestAcquireSkill verified for the selection
  semantics: the server never clears a selection (only the next
  selection replaces it), the self click runs through the
  PlayerClick handler and selects the character itself - the
  official client way of dropping an npc selection.

- Commit "the npc talk selection clears when the conversation ends":
  (1) state grows ObjectAttackable and ObjectLevel reads (the
  friendly villagers, the own id and the unknown objects are never
  attackable; the level comes from the hot record the NpcInfo
  resolved through the generated dictionary). (2) GameClient grows
  ClearTarget: the self click (Action 0x04 on the own object id)
  replaces the server side selection, a no-op without a selection.
  (3) The hunt GameAPI carries it; advanceTripStop and endTownTrip
  call it through clearTalkedTarget, so the talked merchant/teacher
  selection never survives the stop/trip. (4) The engage adoption
  requires an attackable npc: a lingering friendly selection (a talk
  outside the trip end) is skipped instead of burning the 12 s stuck
  timeout on refused forced attacks. Tests:
  state/tracking_test.go (the villager/mob/corpse/unknown split of
  ObjectAttackable, the ObjectLevel read), hunt/loop_test.go (the
  engage never attacks the talked Ellenia selection, the fresh mob
  pick replaces it), hunt/town_test.go (the sell stop end fires the
  clear while Herbiel stays selected).

- Commit "the spellbooks of the queued lessons never sell": the sell
  and destroy junk flows of the state tracker keep the spellbooks the
  unlocked queued lessons demand (demandedBooksLocked walks the
  stored learning queue, keeps the books of lessons with ReqLevel <=
  the character level, cached per skills revision and level): a book
  bought at the book stop or looted for a near term lesson no longer
  re-enters the junk ranking - the reported loop bought the book,
  sold it for referencePrice/2 at the next trip, re-bought it full
  priced forever while the lesson it feeds waited. The planned
  equips keep set stays unchanged (the object id map), the book keep
  keys the ITEM id inside the state layer so both the sell batches
  and the overflow destroy get it for free. Tests:
  state/inventory_test.go (level 5: the books of locked lessons sell
  as before; level 15: the Attack Aura and Defence Aura books stay
  out of the sell list AND the destroy batch, only the stems sell).

- Commit "the aggro on the character is answered, never walked
  past": (1) The targetless pick of the engage answers an attacker
  that holds the character as its target (NearestAttacker - the
  swings or the chase both carry the character as the mob's target
  id): a healthy character with a winnable attacker (the level
  ceiling of the engage covers it, see attackerEngageable) fights it
  at once - the forced attack request fires on the same tick; a hurt
  character or an unwinnable one keeps the defensive flow (the
  standard escape walk of fleeFromThreat with its flee budget and
  the emergency logout behind it). (2) The town trips interrupt the
  same way (interruptTripForAttacker runs first in tickTownTrip): a
  mob on the walking seller drops the trip through the SOFT reset
  (resetTownTrip - no cooldown, the junk/books/sold state survives)
  and answers with the fight or the defense instead of dragging the
  chase through every camp on the route - the reported pile up death
  of the walkers. (3) adoptOutZoneFight checks the winnability of
  the attacker it finishes outside the zone: an unbeatable chase
  switches to the escape instead of pressing a losing fight.
  Tests: hunt/loop_test.go (the attacker beats the nearer fresh mob
  of the pick; the level 8 attacker of a level 3 character arms the
  escape instead of a fight), hunt/town_test.go (the trip drops
  without a cooldown and fights the attacker; the unwinnable
  attacker gets the escape walk).

- Root causes of the teacher walks (reproduced live on the local
  stack, a level 15 elven fighter with 2000 SP and the Attack/Defence
  Aura lessons queued injected through the database): (1) the first
  town trip of a session races the enter world packet burst - a trip
  that starts between the ItemList and the SkillList plans without
  the learning stops (the observed sessions shopped on their first
  walk and never carried the teach stop); (2) the teacher leg dies on
  the water guard: the deployed geodata pack models holes under the
  village plaza (cells without a floor layer resolve to the lake
  layer below them - the probe: every straight line Creamees ->
  Cobendell/Ellenia fails the dry raster while both endpoints stand
  dry at -2984/-2792), every re-path reproduces the same wet line and
  the third exhausts the budget into "town trip ended: aborted, the
  walk would cross water" - the books were bought (the book stop
  comes first), the teacher stop never ran.

- Commit "the teacher legs survive the skill list race and the
  village water raster": (1) maybeStartTownTrip holds its start until
  the server skill list arrived (state.SkillsListed, bounded by
  skillListWaitLimit 10s through state.StartedAt so a server that
  never lists skills keeps the trips selling). (2) The water guard
  releases a plan the geodata itself routes through water: past the
  re-path budget the leg is trusted (wetPlanTrusted, the clicks skip
  the dry check for the rest of the leg and the server routing
  carries the walk over the real plaza) while the standing water
  check and the shore escape stay armed - a genuine swim counts
  (waterEscapes per trip) and the SECOND exhausted budget aborts as
  before, so the open water case cannot loop the trust. Tests:
  hunt/learning_test.go (the trip waits for the skill list, then
  carries the learning stops), hunt/water_guard_test.go (the budget
  exhaustion trusts the plan and the clicks go out; the trusted plan
  that actually swims re-arms the guard and the next exhaustion
  aborts).

- Refinement of the water guard half (the trust gamble replaced by
  the real root cause): the live re-run with the trust fallback let
  the character swim west toward a far hunting spot across a REAL
  lake (the spot walks have no dry route at all - the geodata A* is
  water blind). The measurement that split the cases: sampling
  OverWater along the lines - the plaza teacher lines cross ZERO wet
  cells, the lake lines 736-2640 units. The plaza failure was never
  water: the line of sight half of DryLine fails on the height step
  between the village decks (-2984 shop deck -> -2792 teacher plaza,
  192 units against the passable 30) and the guard misread it. The
  guard now reads pathfind.WaterCrossed (the pure water raster, no
  sight gate): the teacher legs walk (the server routing handles the
  ramps), the real lake refusals keep the old abort. The wetPlanTrusted
  trust machinery was reverted entirely. The session gate also
  re-arms on every reconnect now (state.sessionAt - ResetSession
  drops the skill list and the relogin burst re-delivers it a second
  later; the reconnect loops of the emergency logout made every
  relogin trip a shopping trip, the gate keyed on the tracker uptime
  never held). Engine test:
  pathfind/water_escape_test.go (WaterCrossed splits the channel
  from the tall dry step), hunt tests: the reconnected session
  re-arms the wait, the abort-on-budget stays for real water.

- Live verification (the local stack, a level 15 elven fighter with
  2000 SP and the queued book lessons injected through the database):
  the full cycle ran - the trip planned the learning stops (2
  spellbooks at Creamees, the teacher Ellenia), the books were
  bought ("Spellbook: Advanced Attack Power/Advanced Defense
  Power"), the character walked to Ellenia (the plaza leg survived
  the guard), the teacher was found and clicked, and both lessons
  landed: "learned Attack Aura level 1 for 920 sp", "learned Defense
  Aura level 1 for 160 sp" - the database holds skills 77 and 91 at
  level 1, the SP dropped to 938 and the books were consumed by the
  server. The E2E shutdown stayed graceful (exit 0 on SIGINT).

- Commit "the npc talks, the books and the aggro own their rules in
  the docs": docs/hunting.md updated - the combat safety section
  carries the aggro answer (the attacker pick, the trip interrupt,
  the winnability gate of adoptOutZoneFight), the town trip section
  carries the npc talk selection clearing, the spellbook keep and
  the skill list gate, and the water safety section carries the
  WaterCrossed raster story.

### Status: done (2026-09-10)

- All four fixes committed and pushed (six commits + the test follow
  up): the npc talk selection clear (983437c), the spellbook keep of
  the sell and destroy junk flows (ffd767d), the aggro answer of the
  engage and the town trips (6c6463a), the teacher legs (407c8f2 then
  d5428e3 - the water guard reads the pure water raster, the skill
  list gate re-arms per session, the trust gamble reverted), the docs
  (in d5428e3) and the rebase follow up for the concurrent water loop
  round (64153f9).
- The live stack validates the full learning cycle end to end (twice,
  including on the merged tree with the concurrent dry search round):
  the trip plans the learning stops, the spellbooks are bought at
  Creamees, the teacher (Ellenia/Cobendell) is reached, clicked and
  the lessons land - "learned Attack Aura level 1 for 920 sp",
  "learned Defense Aura level 1 for 160 sp", the database holds skills
  77 and 91 at level 1, the SP is charged, the books are consumed.
  The E2E run prints E2E_OK with the graceful SIGINT shutdown.
- Verify loop per commit: go build, go vet, the full go test suite,
  gofmt clean, golangci-lint with no new findings in the touched
  files (the pre-existing baseline of the newer local linter version
  in untouched files stays).
- The user-side check stays the project workflow: watch a bot talk to
  its teacher (the "learn:" log lines, the SkillList bumps), watch
  the emergency logout cycles of a piled up bot turn into fights (the
  "is on us, fighting it" line), watch a bought spellbook survive a
  sell trip (the junk batch without the book), and watch the hunt
  after a town trip start cleanly (no 12 s stall on the talked npc).

## Active task: the round 58 engage freeze - the inherited frozen re-path, the unguarded direct zone legs, the repro tests and the acceptance scenario (2026-09-12)

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push. Finished: 2026-09-12 (moved from agent_progress.md).

### Goal

The user report (the 01:50 state dump, build 2149ad1, bot test1,
phase engage): the bot stood at (43048 50312 -2992, the elven village
street) for over an hour with the hunting zone 7900 units away
(the Kaboo Orc Fighter SW leash), no walk plan, no events. The user
asked: find why the bot freezes, fix it, add the reproduction tests,
and add an acceptance test (runnable from the web UI / the
`-acceptance` CLI) where the bot starts at the same position with the
same set of items and walks to the selected zone.

### Root cause (probed against the real geodata pack)

A deleveling aborted on a frozen re-path cell; its return leg
inherited the frozen cell (`startDelevelReturnLeg` never cleared
`repathX/repathY/frozenRepaths`) and died on its own first refused
click in one second; `abortFrozenTrip` armed the fail budget and the
engage fell back to the direct zone legs, whose southwest line the
server click validation collapses onto the walker (the walled street
side) - `walkZoneLeg` never validated its clicks, never detected the
missing movement and never logged, so the bot ground the same refused
click once per second forever. See `docs/development_log.md` Round 58
for the full story and the probes.

### Fix (commit 1)

1. `hunt/town.go` (`startReturnLeg`): the return leg starts with a
   clean frozen re-path budget.
2. `hunt/loop_movement.go` (`guardZoneLegClick`): every direct zone
   leg is validated through the server click port; a refused leg is
   never sent, the refusal re-arms the pathfound zone return and the
   paced log line names the wall.

### Acceptance criteria

- The exact dump state (the dump cell, the dump zone, the post-abort
  escalation) walks into the zone against the real geodata pack with
  zero refused clicks (round58_repro_test.go).
- The round 56/57 contracts stay green (the in-plan frozen detection
  is untouched; the reset moved to the leg boundary).
- A new `zone-return` acceptance scenario: the temp character starts
  at the dump position with the dump inventory (level 14, the exact
  bag of the report) and the pass condition is standing inside its
  selected hunting zone.
- go build/vet/test/lint green; the live stack validates the walk.

### Status: done (2026-09-12)

- Commit 1 (the hunt fix): the frozen reset, the zone leg guard, the
  four round 58 repro tests, development_log Round 58, the entry.
  All tests and `golangci-lint run --new` green.
- Commit 2 (the acceptance scenario): the `zone-return` scenario - the
  temp character temp4 starts at the dump cell (43048 50312 -2992)
  with the exact dump state (level 14, exp 192206, sp 7549, adena
  31857, the 25 stacks of the report inventory: the Brandish two
  hander, the wooden armor set, the starter jewels, the arrows, the
  potions, the recipes and the crafting pile), the auto equipment
  dresses it and the pass condition is standing inside its selected
  hunting zone (`acceptance/zone_return_test.go` pins the reset, the
  item set, the checks, the condition evaluation and the item
  injection). The live stack validation passed: the scenario run
  (2026-09-12 00:21, build a4c9e15+zone-return) dressed the gear,
  anchored the Spore Fungus SW spot, walked the village-to-zone route
  in ~70 s and engaged a Kaboo Orc Fighter on the zone entry -
  `Acceptance: PASS` in 68 s; `tools/mobius_e2e.sh 45` E2E_OK.

## Active task: the farm readiness acceptance round - the one town visit and the pdef maximizing armor set (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

test (scenario 1, `farm-readiness`) passes but takes far too long and
the bot acts suboptimally - it buys only the weapon/armor, walks to
the farm spot, farms, and walks back for the books and the lessons.
The demanded behavior:

1. The bot buys the weapon, the armor, the spellbooks AND learns the
   skills in ONE town visit, before it ever leaves for the farm spot.
2. The bot spends its adena aggressively: the advanced armor pieces
   inside the leftover budget of the weapon milestone, maximizing the
   summed pdef of the whole set (the cheap multi slot fillers when
   they maximize pdef, the single expensive piece when that wins).
3. The jewels stay on the basic floor set in every slot (both halves
   of the pairs): the starting locations barely attack with magic,
   the mDef upgrades never pay.
4. Root cause found and fixed: the old planner's armor floor bought
   the cheapest piece of every empty armor family and the
   one-per-slot guard then blocked the advanced upgrades for the rest
   of the trip (the bot left town in the 8-88 pdef floor set with
   ~20k adena unspent and walked back for the upgrades next trip);
   the weapon budget rule capped the defense at the weapon reference
   price; the jewel upgrades at level 15 burned 17k adena on mDef.
5. Measure the post-fix acceptance duration and tighten the farm
   scenario timeout to the measurement plus margin (the live mob
   positions vary run to run).

### Acceptance criteria

- `gear.PlanPurchases` plans: the weapon milestone first, then the
  pdef maximizing armor set (an exhaustive enumeration over the per
  family efficient frontiers inside the remaining budget, one piece
  per armor family per trip), the basic jewel floor (both pair
  halves) behind a real weapon, the shield with the leftover.
- No jewel upgrades ever, no weapon-budget cap on the armor.
- The weapon run (a bare-handed character) carries the learning stops
  too: the books and the teacher join the gear stops in the same
  visit, the weapon stop runs first so an abort never strands the bot
  unarmed. The learn stops plan at the sell stop behind the gear
  stops, the book stop merges into the gear stop of its merchant.
- The trip's gear plan reserves the spellbook budget
  (`Loop.pendingBookBudget`) so the books always stay affordable.
- `dropOwnedPurchases` counts family copies (the pair families carry
  two) so the second ring/earring half buys instead of dropping.
- Every change ships with its unit test; the lint gate stays clean on
  the new lines.

### Progress (2026-09-11)

- Environment deployed per AGENTS.md before touching the code:
  `tools/swarm_fast_deploy.sh` in the foreground with a 10 minute
  timeout - `STACK_READY`, login 2106 / game 7777 / db 3306
  listening; `tools/install_dev_tools.sh` green (task, golangci-lint,
  gci, gofumpt); `go build ./...` green.
- Commit "gear: the pdef maximizing armor set replaces the floor and
  the defense phases": the planner phases are now the weapon
  milestone, the armor set enumeration (`planWalk.armorPhase`,
  `familyFrontier` with the sell-credit net costs, the dominance
  pruning and `enumerateArmorSet`), the basic jewel floor (both pair
  halves, `familyCopies`) and the leftover shield. The jewel
  upgrades, the weapon budget rule (`defenseFits`, `defenseValue`),
  the armor floor caches and the wishlist extension are gone; the
  level parameter left the public API. The 100k adena level 15 plan
  now buys the Brandish plus a 127 pdef armor set plus the five basic
  jewels (99.9 percent of the wallet) where the old plan bought the
  88 pdef floor and 17k adena of jewel upgrades.
- Commit "hunt: the weapon run carries the learning stops - one town
  visit buys the weapon, the armor, the books and teaches": the learn
  stops plan at the sell stop behind the gear stops (the book stop
  merges into the gear stop of its merchant - Creamees sells both the
  basic jewels and the spellbooks), `Loop.pendingBookBudget` reserves
  the spellbook adena out of the gear planning wallet and
  `dropOwnedPurchases` counts the family copies. The updated tests
  pin the new flow (`TestWeaponlessRunCarriesLearning`,
  `TestLearnTripTriggersOnTheSkillBudget`,
  `TestLearnTripBuysTheSpellbooks`).
- Commit "docs: the weapon-first shop strategy, the pdef maximizing
  armor set and the one town visit rule": `docs/shopping_strategy.md`
  rewritten around the new phases (the acceptance wallet check, the
  new was/is journey table), `docs/hunting.md` shop strategy section
  and the weapon run paragraph updated.
- Commit "gear: the basic jewel floor reserves ahead of the armor
  set": the first acceptance measurement run (481 s, PASS) showed the
  armor enumeration eating the wallet down to 53 adena - only 3 of
  the 5 jewel slots filled and a second village walk would follow for
  the remaining pair halves. The phase order is now the weapon
  milestone, the basic jewel floor (the 261 adena outfit covers every
  slot first), the pdef maximizing armor set and the leftover shield:
  the second measurement run bought all five jewels with the
  125 pdef armor set (472 s, PASS).
- Commit "acceptance: the farm readiness timeout tightened to the
  measured one town visit round": farmTimeout drops from 30 to
  20 minutes - the fixed flow measures 472-481 s (the lessons pacing
  dominates), the bound holds two and a half times that for the live
  run variance (the mob positions, the walk retries, the road fights).
- Status: done (2026-09-12). Two full acceptance runs PASS at 481 s
  and 472 s; `go build ./...`, the full `go test ./...` suite and
  `golangci-lint run --new` are green.

## Active task: the widening frozen corridor ban and the zone leg grind stall (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the state dump of `test3` (build 6e45624, uptime
1m33s): the level 15 character stood at x 45768 y 49848 z -3056 (the
elven village south terrace deck) with the held cell "Kaboo Orc
Fighter SW-7" 10200 units away at 36000 46765, the walk plan empty,
the character outside the zone and nothing moving it. The event log
carried three identical trip cycles - the town walk stuck, the
corridor ban at 43512 50504, "the detour route froze as well, walking
to 36000 46765 by the server routing", "town trip ended: aborted, the
server routed walk would swim" - and after the third abort ("3
aborted trips in a row, the next trip waits 10m0s") ten seconds of
total silence: no Hunt line, no walk, a frozen character.

### Root cause (three interlocking defects)

1. The frozen corridor ban never grew: `banFrozenCorridor` answered
   "covered, skip the rung" for every later freeze (the detour's
   aimed waypoint sat 66 units from the ban center, inside the
   radius-plus-floor coverage of 96), so no rung ever changed the plan
   shape again and the deterministic planner reproduced the same
   walled southwest corridor every trip (the geodata pack models it
   open, the user's server - PathFinding=2, its own A* plus the
   geodata correction - walls it).
2. The budget-gated direct zone legs ground silently: after the third
   abort `zoneFails` reached the budget and `walkZoneLeg` sent
   1000-unit hops that pass the offline click validation but are
   silently canceled by the server - with no movement watcher, no
   abort and no log line (the regression of the 2026-09-12 01:50
   report through the new abort path).
3. The composition holes the widening exposed: the avoid areas sealed
   a start standing deep inside its own ban (the search expanded one
   cell and answered no route - the documented "the start cell stays
   allowed" intent never held past the boundary ring), the non-dry
   zone return fallback ignored the bans entirely (reproducing the
   very corridor they exist to detour) and the re-path failure
   aborted past the escalation ladder.

### Fix

- `banFrozenCorridor` widens the covering ban (the radius doubles,
  capped at `frozenBanMaxRadius` 1536) instead of skipping the rung:
  every frozen trip pushes the modeled wall outward until the re-plan
  routes around the whole walled approach (the radius sweep against
  the real pack: 384 flips the village exit from the walled
  southwest corridor to the shop deck route).
- `walkZoneLeg` runs the `noteZoneLegStall` no-movement window: a
  position that holds past the stuck timeout while the direct legs go
  out logs one honest line and re-arms the pathfound zone return
  (the same recovery the offline refusal arms), so the grind feeds
  the widening ladder instead of grinding silently forever.
- The search escape ring (`pathfind/search.go`): the cells of the ban
  holding the start within `avoidEscapeRadius` (256) of the standing
  cell cost `avoidEscapeMultiplier` (6) instead of impassable - the
  way out of the own ban always exists, foreign bans and the own ban
  beyond the ring keep their walls (the sealed goal contract of the
  trainer hall aisle tests holds).
- The non-dry fallback respects the bans: `FindPathApproachAvoiding`
  (the engine, the Navigator, `startWalkLegSearch`) threads the avoid
  areas into the water permitting search.
- The re-path failure escalates through `abortFrozenTrip` (both the
  stuck path and the refused click path), so the ladder owns the
  freeze evidence.

### Acceptance criteria

- `TestReproCorridorWidenDoublesTheCoveredBan` pins the widening
  itself: the covered detour waypoint doubles the covering ban's
  radius (48 -> 96 -> 192 -> 384), the capped ban answers false, a
  fresh waypoint arms a new area and the area count cap blocks new
  areas but never the widening.
- `TestReproCorridorWidenEscapesTheWalledApproach` replays the whole
  dump standoff against the real geodata pack with a server model
  that walls the southwest approach (the walled patches of
  `reproWidenWalls`): the widening ladder must push the plan off the
  corridor (the radius 384 shop deck route) and the walk must arrive
  inside the zone - the inversion of the dump signature.
- `TestReproZoneLegGrindStallReArmsThePathfoundReturn` pins the
  grind stall: the window fires within the stuck timeout, the log
  names the frozen legs, the fail budget clears and the next
  returnToZone tick plans a fresh geodata route.
- `TestReproZoneLegStallRebaselinesOnMovement` and
  `TestReproZoneLegStallStandsDownOnPlannedLeg` pin the honest-flow
  guards: a moving character never stalls, a planned leg stands the
  watcher down.
- `TestAvoidingSearchEscapesTheOwnBanFromDeepInside` (pathfind) pins
  the escape ring: the search from the dump's crept cell (129 units
  inside its own 192-radius ban, a foreign corridor ban between it
  and the goal) finds the dry route out, the escape waypoints stay
  within the ring and the route never re-enters the own ban or
  touches the foreign one.
- The existing contracts stay green: the round 57/58 repro tests,
  the trainer hall aisle avoid tests (including the sealed goal),
  the full hunt and pathfind suites, `task fmt:check`, the
  `golangci-lint run --new` verdict (the three gci formatter
  artifacts on the touched files aside - the spaces-vs-tabs
  branch-wide fight the tree documents) and `tools/mobius_e2e.sh 45`
  (E2E_OK).

### Status: done

- Commit 1 (the fix + the six tests + this entry): the widening
  `banFrozenCorridor`, the `noteZoneLegStall` grind stall, the
  search escape ring, the `FindPathApproachAvoiding` fallback, the
  re-path failure escalation, the corridor widen repro tests, the
  pathfind escape ring test, the agent_progress entry. All
  `internal/swarm/hunt` and `internal/swarm/pathfind` tests green,
  `go build`/`go vet` clean, `gofmt-spaces` clean, E2E_OK.

## Active task: the recastnavigation research - what a Detour navmesh gives the geodata pathfinder (2026-09-14)

Started: 2026-09-14. Branch: `feature/new-pathfind` (based on the
feature/proxy-server tip e3f8267). Commits as melg8. Other agents may
push to the same branch concurrently - rebase before every push.

### Goal

The owner task: research
https://github.com/recastnavigation/recastnavigation as the candidate
replacement for the grid A* of `internal/swarm/pathfind`. The hard
case that motivates it: a route from the elven village to a point
UNDER the bridge - on the water below the floating village - where
the x and y coordinates match a walkable deck column but the z sits
hundreds of units below it. The current engine answers the case with
crutches (the 3x water cost, the dry wall, the water escape BFS, the
legDry smoothing rule). Questions to answer with runnable evidence:
what results does the navmesh give in the stacked-layer case, what
does the transition cost, how hard is the geodata port, how hard is
a Go port, how much faster/slower/more universal is it.

### Progress

- research scaffold `research/recast/` (README, pinned-clone build.sh,
  experiments): upstream 9f4ce64 built with the sandbox g++ (no cmake,
  no new toolchain - the bot itself stays pure Go), upstream tests 33
  cases / 5000 assertions green.
- experiment A+B (`experiments/bridge_water.cpp`,
  `results/bridge_water.txt`): the synthetic floating-village world
  proves the same x/y different z disambiguation (findNearestPoly
  picks the water poly under the bridge, the deck poly on top, the
  mid-height point resolves to the nearer surface), the full
  village -> under-bridge route crosses the bridge, descends the shore
  and swims at 3.7 us per query, the dry filter answers the honest
  partial (closest dry poly), the water escape is an ordinary
  findPath with a water area cost (no dedicated BFS) and the raycast
  reproduces the line of sight semantics.
- porting hazard found and documented: stacked walkable surfaces that
  are additionally within walkableClimb of each other laterally (a low
  bridge deck over its own ramp) merge into ONE self-overlapping
  region; the traced contours come out mangled and the poly link
  graph breaks silently (one way links, unreachable water).
- experiment C (`experiments/geodata_navmesh.cpp`,
  `results/geodata_navmesh.txt`): the naive span import of the real
  21_19 region CRASHES (137k vertex contour, 7335 overlapping
  regions) exactly as the hazard predicts; the implemented fix is the
  sheet decomposition (2D manifold surfaces, greedy graph coloring
  of areas, link height validation) which builds a healthy 14 062
  polygon / 2.89 MB Detour tile in 5.1 s offline (0 one-way links).
  The l2j block order trap (x-strip major blocks) is documented. The
  real bridge query works: village dump cell -> water under the
  bridge deck (880 units of stack) in 169 us, the dry filter answers
  the closest dry point, the reverse escape routes out of the water.
- experiment D: the 200 random pair replay through both engines - the
  grid engine answers 129/200 (39 aborted at the 1M cap, 2.94 s
  average, the hard pair 5.17 s / 500k nodes), the Detour tile 200
  / 200 at 339 us average. The fidelity audit: 413 692 NSWE walled
  neighbour pairs the height-only mesh would connect (the invisible
  wall gap and its three mitigations are in the report).
- the Go runtime prototype `internal/swarm/pathfind/navmesh`: parses
  the exported Detour tile (1.45 ms, 96x cheaper than the l2j region
  parse), answers the hard bridge pair in 571 us with the Detour A*,
  the nearest poly disambiguation passes on the real bridge column,
  the pair replay runs 163/200 at 915 us (the misses trace to the
  prototype's height approximation, noted with the production fix).
- the final report `docs/recast_pathfinding.md`: the comparison
  table, the crutch-by-crutch mapping, the port effort estimate
  (~5-6k Go LOC) and the verdict - port the Detour runtime, build
  the mesh in Go from the geodata with the sheet decomposition, keep
  the grid engine as the validation layer.

### Next

The research task is complete; the report's migration path is the
pending implementation decision of the owner. The concrete follow
ups when the migration is approved: the offline tile builder cmd,
the multi region tile loading, the funnel string pulling, the NSWE
link filter and the pooled query allocations.

## Active task: the cursor key escape - the reproduction of the refusing cell even the official client cannot walk (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report after the village escape round (build 70d49c5): the
acceptance character temp10 ran on the user's own server and stood at
the dump cell 45768 49848 -3056 again (the 15:10 state dump, uptime
3m46s, three session restarts, the full escalation ladder, the abort
"the server refused the routed walk clicks"). The user then proved
the refusal server side with the OFFICIAL CLIENT: from that exact
point the client's own ground clicks die too, only the ARROW KEYS
moved the character, and after the arrow walk the clicks worked
again. The demand: the reproduction of that situation - a cell no
click can leave, walked out the arrow way.

### Diagnosis (the Mobius source closed it)

- `MoveToLocation` carries a movement mode field ("is 0 if cursor
  keys are used 1 if mouse is used"): the mode 0 branch latches the
  player's cursor key flag and adopts the packet origin within the
  thousand unit window.
- While that flag holds, the cursor key branch of
  `ValidatePosition.runImpl` syncs EVERY claimed placement straight
  into the world and broadcasts it - the character follows the
  client's own movement simulation with NO click validation at all.
  That is the arrow walk: the only movement a click-refusing cell
  answers. A mouse-mode click clears the flag again (the session
  returns to the mouse movement).
- The honest attribution of the previous round held: the routed walk
  clicks were REALLY refused (the user's client met the same
  refusal) - the refusal evidence verdict was true, the recovery
  was missing.

### Progress (commit: the cursor key escape)

- The reproduction (hunt/cursor_escape_repro_test.go): the
  cursorKeyServer models the reported server honestly - the mouse
  clicks whose origin stands within the refusal radius of the dump
  cell answer ActionFailed and never move the character (the
  refusal the official client met live), the cursor key arm latches
  the flag, the claims move the character (the cursor key branch),
  an accepted mouse click clears the flag. Two tests pin the
  contract: the escape walks the character out of the refusing cell
  and the clicks resume from the escaped ground (the walk reaches
  the zone, every claimed step stays dry - the water guard holds
  for the claims); a server that ignores the claims too (the
  keyboard movement disabled) burns the escape attempts and aborts
  with the honest reason - the frozen-client reproduction.
- The escape (hunt/town.go): the routed walk refusal verdict arms
  the cursor key escape instead of aborting at once - the movement
  mode 0 arm (CursorKeyWalkTo) plus the claimed ValidatePosition
  steps toward the validated dry hop aim (one run-speed step per
  second, the official client cadence), the follow probe watches
  the server position (five unclaimed claims end the attempt), the
  settle window re-arms the leg window and the clicks resume. Three
  attempts per trip, the corridor bans stay off (the refusal
  evidence owns the verdict), the resets clear the state on every
  trip and leg boundary.
- The connection primitives (connection/game.go): CursorKeyWalkTo
  sends the mode 0 request with the tracked origin,
  ClaimValidatePosition sends the claimed placement and gates the
  echo ticker (the echo would lag the claims one broadcast behind
  and snap the character back while the flag holds), the first
  mouse-mode walk returns the echo (claimsOwnStream). The stream
  tests pin the mode, the claim semantics and the gate.

### Acceptance criteria

- The reproduction runs on the real geodata pack: the refusing cell
  with the cursor key semantics modeled, the escape walks out, the
  clicks resume, the walk reaches the zone
  (TestReproCursorKeyEscapeWalksOutOfTheRefusingCell).
- A server that ignores the claims aborts honestly after the
  attempts burn, no corridor ban, the character never moves a cell
  (TestReproCursorKeyEscapeAbortsWhenTheServerIgnoresTheClaims) -
  the reproduction of the user's frozen client.
- The cursor key arm carries the movement mode 0 and the tracked
  origin; the claims own the validation stream while the escape
  runs and the first mouse-mode walk returns the echo
  (TestGameClientCursorKeyWalkSendsTheKeyboardMode,
  TestGameClientClaimsOwnTheValidationStream).
- The claimed steps never cross water (the per-claim water guard of
  the escape test).

### Status: done

- Commit 1 (the reproduction + the escape + the tests + the docs):
  the cursor key escape of a click-refusing cell, the cursorKeyServer
  reproduction model, the connection primitives, the stream tests,
  the protocol description sections, the round 84 development log
  entry, this progress entry. go build/vet clean, the full hunt,
  connection and packets suites green, `task fmt:check` clean,
  `golangci-lint run --new` (the three documented gci artifacts
  aside), `tools/mobius_e2e.sh 45` E2E_OK.

## Active task: the village escape acceptance round - the honest refusal attribution and the client position stream (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report after the refusal channel round (build 73fcfa6,
"didnt fix it still") carried the 12:31 state dump: the abort reason
changed - the "would swim" message is gone (the water guard fix of
the routed hops held), but the trip now aborted on "the server
refused the routed walk clicks" while the character stood at
45768 49848 -3056 through a whole day of three dumps (08:42, 10:18
and 12:31 - never one cell of movement). The user demand: make the
acceptance test of that stuck cell (it shows FIRST in the web UI
list) and demonstrate that from this position the bot finds a path
and gets out of the city within two minutes at most.

### Diagnosis

- The refusal attribution was dishonest: the ActionFailed packet
  carries no request identity, and every other request of the
  session (the equip and skill requests of the gear machinery, the
  transactions) answers ActionFailed the same way. The 12:31 dump
  rerun showed the equip ActionFaileds landing one to three seconds
  after the walk clicks of the same tick window - the correlation
  latched them as walk refusals and aborted the routed walk the
  server never refused. The honest gate: an arrival with a non walk
  request in flight answers that request at least as likely as the
  click.
- The session never spoke the client position validation the server
  builds half its view from: the official client streams
  ValidatePosition 0x48 about once a second while moving, and
  ValidatePosition.runImpl feeds the clientX/clientY/clientZ, the
  client heading and the last server position of the door logout
  exploit check that MoveToLocation compares against. A bot that
  never validates leaves that view frozen at the login defaults -
  the C1 z adoption gate (Math.abs(_z - getClientZ()) < 800) can
  never run for a session whose client z the server still reads as
  zero, and every server side branch that reads the client view
  answers for a client that never spoke.

### Progress (commit: the village escape round)

- The honest refusal attribution: the send path records the walk
  clicks (opcode 0x01) and every answered request separately
  (sendPacket + silentSessionOpcodes - the maintenance stream that
  never sees an ActionFailed answer, the handshake family,
  ChangeMoveType2, Appearing, RequestNetPing and the validation
  stream itself, stays out of the bookkeeping), and
  refusalEvidence skips the attribution when a non walk request was
  sent inside the answer window (state.Bot.OtherRequestBetween).
- The client position validation stream: a one second ticker sends
  the placement the server itself broadcast (never a claimed
  position) when it changed, plus a fifteen second standing
  heartbeat - the official cadence, far under every flood protector
  threshold.
- The acceptance scenario "village-escape" owns the FIRST slot of
  the web UI list: temp10 wakes at the dump cell 45768 49848 -3056
  with the exact state of the report (level 15 at 87.09 percent of
  the level span, 1760 sp, 1312 adena, the Brandish sword, the bone
  armor set, the leather helmet and gloves, the starter jewels, 589
  arrows and the hunting bow in the bag) and must stand 3000+ units
  from the village plaza within two minutes of the world entry.
- The demonstration on the live geodata stack (the MOVEDBG trail of
  the game server log): temp10 walked from the exact dump cell -
  the first move request at 10:15:32 from 45768 49848 -3056, the
  server position updating on every leg, 3442 units out at 10:15:56
  and still moving. Twenty four seconds of continuous walking, not
  one refused click, no corridor ban - the two minute contract holds
  with a five fold margin.

### Acceptance criteria

- The web UI list serves the village escape scenario first
  (TestVillageEscapeLeadsTheWebUIList pins the head slot, the dump
  cell, the dump state and the two minute window).
- A refusal answer with an equip request in flight never reads as a
  walk refusal (TestRefusalEvidenceAttributesOnlyTheClicksOwnAnswer
  pins the attribution cases: the click's own answer counts, the
  equip's answer does not, an older request does not steal the
  answer, a request after the arrival keeps the attribution).
- The session streams ValidatePosition on the official cadence: on
  movement change and on the standing heartbeat, carrying the
  server broadcast placement only (the stream tests of
  validate_position_stream_test.go and the wire format test of
  validate_position_test.go).
- From 45768 49848 -3056 the bot walks itself out of the village
  within two minutes on the live stack (demonstrated: 24 s).

### Status: done

- Commit 1 (the fix + the acceptance scenario + the tests + the
  docs): the honest ActionFailed attribution, the ValidatePosition
  client stream, the village escape acceptance scenario (first in
  the web UI list), the refusal attribution tests, the stream
  tests, the protocol description section, the round 83 development
  log entry, this progress entry. go build/vet clean, `task
  fmt:check` clean, `golangci-lint run --new` (the three documented
  gci artifacts aside), the full `go test ./...` green (23
  packages), `tools/mobius_e2e.sh 45` E2E_OK.

## Active task: the server refused walk clicks - the ActionFailed refusal channel (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the state dump of `test3` (build c22d529, uptime
7m24s): the level 15 character stood at x 45768 y 49848 z -3056 (the
elven village center) in the townReturn phase, never moving a single
cell through nine aborted trips, while the corridor bans grew
(43512 50504 widened to r768, then the fresh bans 44104 49544,
44200 49560, 44296 49576, 44392 49592, 44488 49608 - the whole
northern exit sealed for the session), the direct zone legs ground
("moved nothing for 16s, re-arming the pathfound return") and every
server routed walk aborted on "the server routed walk would swim".
The user's hypothesis: "it might be something with anti water laws,
maybe they need to be changed".

### Root cause (the missing refusal channel)

Live reproduction on the local stack with the full geodata pack and
`PathFinding = 2` (the reference deployment layout the sandbox fast
deploy disables, `tools/swarm_fast_deploy.sh` sed) walks the SAME
dump scenario cleanly: temp3 injected at the dump position exits the
village through the exact "frozen corridor" 43512 50504 and engages
in the zone in ~90 s (the MOVEDBG diagnostics patch: every click
ACCEPTED, one A* found=true). The user's server build differs from
the reference master (the dump's unknown packet fingerprints 0x57/53
bytes and 0xe7/21 bytes do not exist on the local master) and
refuses the village-exit clicks its own way - while other fleet bots
in open ground keep moving. The bot has no channel for that answer:

1. The server answers every refused move with ActionFailed (0x35);
   the bot parses it (`connection/game_dispatch.go::applyActionFailed`)
   and DISCARDS it ("the hunt loop keeps driving its own retry logic
   without reacting to it") - only a log line survives.
2. Every stuck verdict therefore reads as a "frozen corridor": the
   ladder bans innocent corridors, the widening seals village exits
   for the session and the abort backoff climbs to 1h while the real
   diagnosis (the server refuses the clicks) never appears anywhere.
3. The server routed fallback clicks the zone center ~10000 units
   away: beyond the server's 9900 request cap (refused outright) and
   over the 3000 unit boundary where Mobius walks player clicks in a
   straight line (the water guard correctly rejects the wet line -
   the "anti water law" the user suspects is right in spirit but it
   guards an impossible geometry).

### Progress (commit: the refusal channel, the varied aim and the hop walk)

- The diagnosis closed the loop the dump opened: the local reference
  stack (geodata + PathFinding = 2, the reference deployment layout
  the fast deploy disables) walks the identical dump scenario cleanly
  - the user's server build (its unknown packet fingerprints pin it)
  refuses the clicks. The bot had no channel for that answer, so
  every stuck verdict poisoned the planner instead.
- The state tracker records the ActionFailed arrivals
  (ApplyActionFailed/LastActionFailed), the connection dispatch
  forwards them and the dump prints the age of the last refusal next
  to the session header.
- The hunt walk machinery reads them as the online refusal evidence
  (refusalEvidence, a freshness window of 4 s around the sent click):
  the stuck verdict varies the aim first (the half/quarter click and
  the perpendicular offsets, all offline validated and water
  guarded), the corridor ban rung is skipped for refused legs (the
  legRefused latch), the routed walk hops under the 3000 straight
  line boundary (offline validated per hop, wet hops shortened to
  the dry prefix, refused hops abort with the honest reason) and the
  zone leg stall holds the return backoff on the unmoved cell.
- The dump-character live rerun on the geodata stack walks to the
  zone and engages (no regression); `tools/mobius_e2e.sh 45` prints
  E2E_OK.
### Acceptance criteria

- The tracker records the ActionFailed arrivals; the hunt loop reads
  them as refusal evidence correlated with the sent click.
- A stuck verdict with refusal evidence varies the click aim
  (shorter prefixes, sideways offsets) instead of assuming the
  corridor froze; the corridor ban rung is skipped for legs whose
  freeze evidence carries ActionFailed answers.
- The server routed walk hops toward its target in capped legs
  (under the straight line semantics and the request cap), the water
  guard checks the hop line, and a refused routed leg aborts early
  with an honest reason.
- The zone leg stall does not re-arm the pathfound return forever
  when the server refuses the legs.
- The dump carries the last ActionFailed arrival.
- Repro tests: a refusing server never bans a corridor; a partially
  refusing server is walked out through the varied aim; the direct
  leg hops arrive; the live geodata stack still walks the dump
  scenario (no regression) and E2E_OK.

### Status: done

- Commit 1 (the fix + the five tests + the docs): the ActionFailed
  refusal channel end to end, the varied aim ladder, the corridor
  ban gate, the hop walk of the server routed fallback, the zone leg
  stall split, the refusal signal repro tests, the round 82
  development log entry, this progress entry. All tests green (23
  packages), fmt:check clean, lint:new carries only the three
  documented gci artifacts, E2E_OK on the geodata stack.

## Active task: the widening frozen corridor ban and the zone leg grind stall (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the state dump of `test3` (build 6e45624, uptime
1m33s): the level 15 character stood at x 45768 y 49848 z -3056 (the
elven village south terrace deck) with the held cell "Kaboo Orc
Fighter SW-7" 10200 units away at 36000 46765, the walk plan empty,
the character outside the zone and nothing moving it. The event log
carried three identical trip cycles - the town walk stuck, the
corridor ban at 43512 50504, "the detour route froze as well, walking
to 36000 46765 by the server routing", "town trip ended: aborted, the
server routed walk would swim" - and after the third abort ("3
aborted trips in a row, the next trip waits 10m0s") ten seconds of
total silence: no Hunt line, no walk, a frozen character.

### Root cause (three interlocking defects)

1. The frozen corridor ban never grew: `banFrozenCorridor` answered
   "covered, skip the rung" for every later freeze (the detour's
   aimed waypoint sat 66 units from the ban center, inside the
   radius-plus-floor coverage of 96), so no rung ever changed the plan
   shape again and the deterministic planner reproduced the same
   walled southwest corridor every trip (the geodata pack models it
   open, the user's server - PathFinding=2, its own A* plus the
   geodata correction - walls it).
2. The budget-gated direct zone legs ground silently: after the third
   abort `zoneFails` reached the budget and `walkZoneLeg` sent
   1000-unit hops that pass the offline click validation but are
   silently canceled by the server - with no movement watcher, no
   abort and no log line (the regression of the 2026-09-12 01:50
   report through the new abort path).
3. The composition holes the widening exposed: the avoid areas sealed
   a start standing deep inside its own ban (the search expanded one
   cell and answered no route - the documented "the start cell stays
   allowed" intent never held past the boundary ring), the non-dry
   zone return fallback ignored the bans entirely (reproducing the
   very corridor they exist to detour) and the re-path failure
   aborted past the escalation ladder.

### Fix

- `banFrozenCorridor` widens the covering ban (the radius doubles,
  capped at `frozenBanMaxRadius` 1536) instead of skipping the rung:
  every frozen trip pushes the modeled wall outward until the re-plan
  routes around the whole walled approach (the radius sweep against
  the real pack: 384 flips the village exit from the walled
  southwest corridor to the shop deck route).
- `walkZoneLeg` runs the `noteZoneLegStall` no-movement window: a
  position that holds past the stuck timeout while the direct legs go
  out logs one honest line and re-arms the pathfound zone return
  (the same recovery the offline refusal arms), so the grind feeds
  the widening ladder instead of grinding silently forever.
- The search escape ring (`pathfind/search.go`): the cells of the ban
  holding the start within `avoidEscapeRadius` (256) of the standing
  cell cost `avoidEscapeMultiplier` (6) instead of impassable - the
  way out of the own ban always exists, foreign bans and the own ban
  beyond the ring keep their walls (the sealed goal contract of the
  trainer hall aisle tests holds).
- The non-dry fallback respects the bans: `FindPathApproachAvoiding`
  (the engine, the Navigator, `startWalkLegSearch`) threads the avoid
  areas into the water permitting search.
- The re-path failure escalates through `abortFrozenTrip` (both the
  stuck path and the refused click path), so the ladder owns the
  freeze evidence.

### Acceptance criteria

- `TestReproCorridorWidenDoublesTheCoveredBan` pins the widening
  itself: the covered detour waypoint doubles the covering ban's
  radius (48 -> 96 -> 192 -> 384), the capped ban answers false, a
  fresh waypoint arms a new area and the area count cap blocks new
  areas but never the widening.
- `TestReproCorridorWidenEscapesTheWalledApproach` replays the whole
  dump standoff against the real geodata pack with a server model
  that walls the southwest approach (the walled patches of
  `reproWidenWalls`): the widening ladder must push the plan off the
  corridor (the radius 384 shop deck route) and the walk must arrive
  inside the zone - the inversion of the dump signature.
- `TestReproZoneLegGrindStallReArmsThePathfoundReturn` pins the
  grind stall: the window fires within the stuck timeout, the log
  names the frozen legs, the fail budget clears and the next
  returnToZone tick plans a fresh geodata route.
- `TestReproZoneLegStallRebaselinesOnMovement` and
  `TestReproZoneLegStallStandsDownOnPlannedLeg` pin the honest-flow
  guards: a moving character never stalls, a planned leg stands the
  watcher down.
- `TestAvoidingSearchEscapesTheOwnBanFromDeepInside` (pathfind) pins
  the escape ring: the search from the dump's crept cell (129 units
  inside its own 192-radius ban, a foreign corridor ban between it
  and the goal) finds the dry route out, the escape waypoints stay
  within the ring and the route never re-enters the own ban or
  touches the foreign one.
- The existing contracts stay green: the round 57/58 repro tests,
  the trainer hall aisle avoid tests (including the sealed goal),
  the full hunt and pathfind suites, `task fmt:check`, the
  `golangci-lint run --new` verdict (the three gci formatter
  artifacts on the touched files aside - the spaces-vs-tabs
  branch-wide fight the tree documents) and `tools/mobius_e2e.sh 45`
  (E2E_OK).

### Status: done

- Commit 1 (the fix + the six tests + this entry): the widening
  `banFrozenCorridor`, the `noteZoneLegStall` grind stall, the
  search escape ring, the `FindPathApproachAvoiding` fallback, the
  re-path failure escalation, the corridor widen repro tests, the
  pathfind escape ring test, the agent_progress entry. All
  `internal/swarm/hunt` and `internal/swarm/pathfind` tests green,
  `go build`/`go vet` clean, `gofmt-spaces` clean, E2E_OK.

## Active task: the zone return escalation ladder (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the state dump of `test3` (build d2ea298, uptime
55s): the level 15 character stood at x 45768 y 49848 z -3056 (the
Elven Village south terrace deck) in the engage phase, the held cell
"Kaboo Orc Fighter Leader SW-10" sat at center 28500 54560 (very far
from the village). The hunt log carried "outside the hunting zone,
pathfinding back" then "town walk stuck, re-pathing (1 of 3)" then
"town trip ended: aborted" at 22s, then nothing for 34s - the bot
cycled between the pathfound-return-stuck-abort and the refused
direct leg without ever moving.

### Root cause

`abortFrozenTrip` ran the `escalateFrozenLeg` ladder (the banned
detour re-plan, the direct server routed walk) ONLY for the town walk
phase (`phaseTownWalk`), NOT for the zone return phase
(`phaseTownReturn`). The zone return aborted straight to `abortTownTrip`
+ `zoneFails = zoneReturnFailBudget`. The next `returnToZone` tick saw
`zoneFails >= budget` → `walkZoneLeg` → `guardZoneLegClick` refused the
direct leg (the village railing walls it) → reset `zoneFails = 0` →
no walk sent. The next tick saw `zoneFails < budget` → pathfound
return → the SAME route the deterministic planner always produces →
town walk → stuck → abort → `zoneFails = 3` → repeat. The bot never
moved.

### Fix

`abortFrozenTrip` now calls `escalateFrozenLeg` for BOTH the town walk
and the zone return phases. `escalateFrozenLeg` accepts
`phaseTownReturn` alongside `phaseTownWalk`, and the re-plan uses
`startZoneReturnLeg` (the non-dry fallback) for the zone return and
`startWalkLeg` (the dry search) for the town walk. The banned detour
re-plan routes around the walled corridor instead of reproducing the
identical frozen route - the deterministic planner produces a
DIFFERENT route when the walled cells are in the avoid areas. The
direct server routed walk (rung 2) hands the routing to the server's
own pathfinder as the last resort.

### Acceptance criteria

- The existing `TestReproRound57FrozenServerEscalatesFast` updated to
  assert the escalation ladder runs both rungs (`frozenStage <= 2`)
  instead of the old straight-to-direct-legs behavior
  (`zoneFails >= zoneReturnFailBudget`).
- The round 58 tests (`TestReproRound58ZoneLegGuardRefusalReArmsPathfoundReturn`,
  `TestReproRound58VillageStuckCellWalksToZone`) stay green (the
  guard behavior for the first-attempt direct leg is unchanged).
- The full `internal/swarm/hunt` test suite stays green.

### Status: done

- Commit 1 (the fix + the test update + this entry): the
  `escalateFrozenLeg` ladder runs for `phaseTownReturn`, the
  `startZoneReturnOrWalkLeg` helper, the `abortFrozenTrip` call, the
  round 57 test assertion update, the agent_progress entry. All
  `internal/swarm/hunt` tests green, `go build`/`go vet` clean,
  `gofmt-spaces` clean.

## Active task: the fast stuck window of the town walk re-path (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the state dump of `test3` (build c7a0855, uptime
19s): the level 15 character stood at x 45768 y 49848 z -3056 (the
Elven Village south terrace deck) in the townReturn phase, the
pathfound zone return held the 6 waypoint route to the Kaboo Orc
Fighter SW-7 cell (dest 36000 46765 -3712), the first waypoint sat at
43512 50504 -2992 (the village plaza corner). The hunt log carried
"outside the hunting zone, pathfinding back" then a single "town
walk stuck, re-pathing (1 of 3)" line at 16s of uptime - the click to
wp1 was validated against the offline click port (passed), sent to the
server, but the server silently canceled the move. The character
never moved a cell, the stuck timeout fired at 15s, the re-path
planned the identical route, and the next detection waited the FULL
15s stuckTimeout again.

### Root cause

`stuckTownWalk` arms `stuckFast = true` on the WAYPOINT SKIP branch
(`nextClearWaypoint` finds a clear successor) but NOT on the RE-PATH
branch (no clear successor, the leg re-plans). The re-path proved
the plain clicks of this leg do not move the character - the same
evidence the waypoint skip carries - so the fast window belongs
there too. Without it the recovery burns the full 15s per re-path
detection (3 re-paths * 15s = 45s + the abort) for a freeze the
fast window (4s) would have caught in ~12s.

### Fix

`town.go::stuckTownWalk` arms `stuckFast = true` and re-baselines
the stuck window (`stuckAt`, `stuckX`, `stuckY`, `stuckWP`,
`stuckBest`) from the re-path tick on the re-path branch, the same
way the waypoint skip branch does. The next stuck detection fires on
`stuckFastTimeout` (4s) instead of the full `stuckTimeout` (15s),
so the recovery of the dump's freeze completes in ~12s instead of
~45s.

### Acceptance criteria

- A repro test that builds the dump standoff (the character at 45768
  49848 -3056, the 6 waypoint zone return, the line of sight blocked
  to every forward waypoint) asserts that after the first stuck
  re-path `stuckFast` is true and the next detection fires within
  `stuckFastTimeout + 2s` (`TestStuckRepathArmsFastWindow`).
- A regression guard that the waypoint skip branch still arms the
  fast window after the fix
  (`TestStuckSkipWaypointStillArmsFastWindow`).
- A repro test that the re-path arm re-baselines the stuck window
  from the re-path tick (`stuckAt` moves past the original
  baseline, `stuckX/Y/WP` reset to the standing cell and the
  current cursor - `TestStuckRepathRebaselinesStuckWindow`).
- The existing `TestWalkStuckRepathsAfterAllWaypointsSkipped` and
  `TestWalkStuckSkipNeedsAClearLine` tests updated to assert
  `stuckFast == true` after the re-plan (the corrected behavior).
- The full `internal/swarm/hunt` test suite stays green.

### Status: done

- Commit 1 (the fix + the three repro tests + the two existing test
  updates + this entry): the re-path arm of `stuckTownWalk`, the
  `stuckFast` and the stuck window re-baseline, the three stuck fast
  repro tests, the two existing test updates, the agent_progress
  entry. All `internal/swarm/hunt` tests green, `go build`/`go vet`
  clean, `gofmt-spaces` clean.

## Active task: the deck gate of the cell mode visible-enemy reading (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the state dump of `test2` (build 140aa42, uptime
1m57s): the level 14 character stood at x 42712 y 49128 z -2992 (the
Elven Village terrace deck) while its held hunting cell "Kaboo Orc
Fighter SW-7" sat 7000 units west at center 36000 46765 on the field
deck (z ~-3576). The dump listed pickable mobs in the knownlist (Kaboo
Orc Grunt level 7 at 3157 units, Green Dryad level 8 at 3799, Kaboo
Orc Archer level 8 at 3830, Spore Fungus level 9 at 4108) - all on
the field deck below the terrace. The hunt log carried a single
"level 14: holding the cell Kaboo Orc Fighter SW-7" line and nothing
else for the whole uptime: no "outside the hunting zone,
pathfinding back", no "engaging", no "far target walk". The bot
neither moved nor engaged.

### Root cause

The engage phase gates the pathfound zone return on
`cellEnemiesVisible`, and `cellEnemiesVisible` answered true (the
knownlist held pickable mobs on the field deck). The pick flow that
followed tried to walk straight toward the nearest visible mob
(`walkToFarTarget` -> `game.WalkTo` with `selfZ`), but the straight
line from the village terrace to the field deck crosses the village
railing and the deck edge - the server's geodata correction
collapses the click target onto the walker cell, the move is
silently canceled and the character never moves. The pathfound zone
return (the only path that walks the character down the terrace ramp
to the field deck) never armed because the visible-mob reading held
its gate shut.

### Fix

`loop.go::cellEnemiesVisible` now reads the nearest pickable mob
through `NearestAttackablePreferredWindowed` (instead of the boolean
`ZoneHasPickableWindowed`) and checks the z gap between the mob and
the character. A mob on a different deck than the character (a z gap
past the new `deckReachableZ` threshold, 400 units) reads as
unreachable by a direct walk - the cell mode treats the knownlist as
empty of directly-reachable enemies and the engage falls through to
`returnToZone`, which plans the geodata route down the terrace ramp.
The threshold matches the deck step the elven lands geography models
(the village terrace at z -2992, the fields at z -3500..-3600) and
stays under the small height steps of the field itself (the gentle
terrain undulation stays under 200 units). A mob whose z the server
never reported (z == 0) passes the deck gate - the same convention
the level filter uses for an unresolved template (level 0 passes
every window), so a stale z reading never freezes a bot whose only
visible mob sits on an unresolved z. The fresh-experience path (the
common position stall of the previous commit) is unaffected - the
deck gate only fires inside the engage's `cellEnemiesVisible` branch.

### Acceptance criteria

- A repro test that builds the exact dump standoff (the character on
  the village terrace, the held cell on the field deck, the nearest
  visible mob on the field deck) asserts that the zone return arms,
  the loop enters the pathfound town return phase, the pathfinder
  plans the ramp walk and the first waypoint walks
  (`TestReproDeckVillageReturnToZoneOnDifferentDeck`).
- A repro test that asserts the direct walk toward the cross-deck mob
  never fires - the walks of the tick stay on the pathfound route
  (the first waypoint sits under 2000 units west, the direct mob
  walk would head 3157 units west)
  (`TestReproDeckVillageNoDirectWalkTowardCrossDeckMob`).
- A regression guard that moves the character onto the field deck
  next to the mob (same deck, same z) and asserts the engage picks
  the mob directly (a forced attack fires, no zone return arms) -
  the free-roam hunt keeps its design on the same deck
  (`TestReproDeckVillageEngagesSameDeckMob`).
- The existing cell mode contract test `TestCellOutGroundEnemiesKeepTheHunt`
  stays green (the mob whose z the server never reported passes the
  deck gate the way an unresolved level passes the level filter).
- The full `internal/swarm/hunt` test suite stays green.

### Status: done

- Commit 1 (the fix + the three repro tests + this entry): the
  deck gate of `cellEnemiesVisible`, the `deckReachableZ` constant,
  the three deck repro tests, the agent_progress entry. All
  `internal/swarm/hunt` tests green, `go build`/`go vet` clean,
  `gofmt-spaces` clean.

## Active task: the stagnation soft reset after the hard recovery (2026-09-14)

Started: 2026-09-14. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user attached the session journal of `test2` (build 12f873ce) and
asked why the bot got stuck, to fix it and to write tests. The dump
showed a 3h20m freeze: the bot stood at 42712 49128 -2992 from
00:36:07Z to 03:56:38Z, phase `engage` throughout, no kills, no
deaths, no purchases, no Hunt log lines for 3h9m. The stagnation
watch fired once at 03:45:55Z (the position stall, the soft reset)
and again at 03:55:55Z (the experience stall, the hard recovery)
without unsticking the bot.

### Root cause (the part the fix addresses)

The two stagnation windows overlap on the same tick when the bot has
been frozen long enough. `observeStagnation` runs the experience
check before the position check. When the experience stall fires the
hard recovery, `stagnationHardRecover` arms `logoutDone` and resets
`stagPosFires` to zero (alongside `stagXPFires`). The position
check then runs in the SAME tick, reads the held position as a fresh
first fire (because `stagPosFires` is zero again) and runs
`stagnationSoftReset` on the dying session. The soft reset calls
`standUpGuarded` and `resetTownTrip` into the pending unwind - the
redundant packets never reach the server before the socket closes,
the relogin inherits none of the cleanup, and the misleading
"clearing the loop state" log line lands next to the honest
"rebuilding the session" one. The journal of the 03:55:55Z tick
shows exactly this sequence: the xp stall, the emergency logout,
the position stall, the soft reset, the lost connection, the
reconnect at the same cell.

### Fix

`stagnation.go::observeStagnation` skips `observeStagnationPosition`
when `l.logoutDone` is true after `observeStagnationXP`. The hard
recovery already owns the unwind; the position check has nothing
useful to add on the same tick. The fresh-experience path (the
common position stall) is unaffected - the gate only fires when the
hard recovery has armed `logoutDone` in the same call.

### Acceptance criteria

- A unit test that arms both windows overdue and calls
  `observeStagnation` asserts that exactly one logout fires
  (`game.logouts == 1`), `loop.logoutDone` is true, the frozen
  target is NOT cleared by the soft reset and the "clearing the
  loop state" log line does NOT land in the sink - only the
  "rebuilding the session" line of the hard recovery
  (`TestStagnationXPHardRecoverySkipsPositionSoftReset`).
- A regression guard that arms only the position window (fresh
  experience) verifies the soft reset still runs in the common
  path: the target drops, the "clearing the loop state" log lands,
  no logout fires
  (`TestStagnationPositionSoftResetStillRunsAfterXPFresh`).
- The full `internal/swarm/hunt` test suite stays green (the
  existing stagnation contracts unchanged).

### Open question (out of scope for this commit)

The dump also shows that the soft reset at 03:45:55Z did not unstick
the bot: the position held for another 10 minutes until the
experience stall fired the hard recovery. The bot stood outside the
held cell "Kaboo Orc Fighter SW-7" (the cell sits at 35000-37000
45899-47631, the bot at 42712 49128 - ~7000 units away), and the
engage phase should have called `returnToZone` to walk back. The
journal shows zero Hunt log lines for that 10 minute stretch
(`returnToZone` logs "Hunt: outside the hunting zone, pathfinding
back" on its first call), which suggests the loop either did not
reach `returnToZone` or reached it without logging - the deeper
freeze is not reachable from the journal alone. A live repro with
the dump-state diagnostics (see the `dump-state-repro` skill) is
the next step.

### Status: done

- Commit 1 (the fix + the two repro tests + this entry): the gate
  in `observeStagnation`, the two new stagnation tests, the
  agent_progress entry. All `internal/swarm/hunt` tests green,
  `go build`/`go vet` clean, `gofmt-spaces` clean.

## Active task: the hex grid, the enemy-first hunt and the ranged-kill loot (2026-09-13)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user order (2026-09-13, Russian), four changes over the Voronoi
cell partition landed earlier the same day:

1. The partition becomes a UNIFORM HEXAGON grid - same hex size over
   the whole map (the Voronoi cells varied in shape and size with the
   seed placement).
2. The map view shows ONLY the hex the fight runs in (the active one)
   and the hex under the cursor - every other hex stays invisible
   (the full-partition edge raster of the Voronoi layer retires).
3. The hunt moves enemy-first: the target pick and the far walk drop
   the held-cell fence - the bot fights the NEAREST VISIBLE enemy
   wherever it stands (even outside the held hex), walks to a zone
   that holds visible enemies, and only ever moves toward an
   enemy-less zone as the last resort (nothing pickable visible at
   all). The held hex follows the actual fight ground (the map
   highlight tracks where the bot really farms), the kills attribute
   to the hex the corpse lies in.
4. A mob killed at RANGE (the bow lure, the caster spells) is looted
   properly: the bot walks to the corpse, waits out the drop
   broadcast, picks up the loot and the adena - never leaves them on
   the ground.

### Plan

1. `tools/generate_hunt_cells.py`: the hex grid partition (flat-top
   hexagons of one circumradius, the analytic point-to-hex
   assignment, the 6-neighbor adjacency from the grid), the mob
   distribution and the naming/ordering/report kept.
2. `hunt`: the mesh version bump, the unfenced target search, the
   out-of-ground engage gate (fight the visible enemies outside the
   hex, walk home only when nothing is visible), the follow-ground
   switch, the corpse-position kill attribution, the loot kill grace
   walk.
3. `webui/map.js`: the active hex + the hovered hex only, the edge
   raster and its cache retire.
4. Tests: the registry invariants (the uniform hex geometry), the
   policy scenarios, the loot grace, the harnesses.
5. Docs: hunting_cells.md, hunting.md, webui.md, this entry.

### Progress

- tools/generate_hunt_cells.py: the hex grid partition (the flat-top
  hexagons of the 1000 circumradius, the analytic axial cube
  rounding point-to-hex, the 6-ring adjacency from the odd-q layout,
  the largest-remainder mob distribution and the naming/ordering/
  report kept). 540 uniform hexagons, the 812 mob mass preserved,
  the visibility invariant 633*sqrt(2)+1000=1896 <= 2048, the
  symmetric adjacency (2876 edges, mean degree 5.3), the audit
  cross-check 98.6 percent coverage. The generator output is
  gofmt-spaces clean.
- hunt: the mesh version "elven-hexes-1", the leashes cache and
  groundOf, the followGround switch (the held hexagon follows the
  actual fight, paced 10 s, the ripeness marking of the left
  ground), the corpse-position kill attribution, the UNFENCED
  emptiness reading of waitOrRotate, the pickZone/onHeldGround/
  cellEnemiesVisible enemy-first engage gate (fight the visible
  enemies outside the hexagon, walk home only when nothing pickable
  is visible - the legacy zone mode keeps the strict return), the
  unfenced far-target walk and the targetless diagnostic, the
  noteKillPosition/killApproachWalk ranged-kill loot grace (the
  corpse approach, the 15 s grace, the melee-kill and the
  already-looted exemptions).
- webui: map.js draws the active hexagon + the hovered hexagon only
  (drawActiveCell, drawHoveredCell), the edge raster, its cache and
  the cellBg state retired; the mesh fetch and the hover hit test
  stay.
- Tests: the uniform hexagon registry pins (6 corners, one
  circumradius, one area, one patrol half), the enemy-first scenarios
  (the out-of-ground pick, the follow switch, the hold-vs-walk-home
  gate, the kill attribution, the visible-enemy rotation hold), the
  ranged-kill loot grace tests, the updated zone-hover harness (the
  inactive hexagons draw nothing, the hovered hexagon draws, the
  pointer leaving hides it).
- Verification: go build/vet, the full go test ./... suite, task
  fmt:check, golangci-lint run --new (the three gci formatter
  artifacts on the touched files - the spaces-vs-tabs branch-wide
  fight the tree documents, the full-gate count unchanged 49=49 vs
  HEAD), all eight web harnesses green.
- Status: done (2026-09-13), pushing as the atomic commits below.

## Active task: the Voronoi cell partition of the hunting map (2026-09-13)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user order (2026-09-13, Russian): abandon the hunting zones as
intersecting circles and split the hunting map with a Voronoi
diagram (possibly with the focus points moving on the farm results).
The driver is the depletion/oversaturation failure of the circle
geometry: a circle that covers HALF of a respawn ground (the Dryad
case) farms that half to exhaustion while the other half
accumulates an unfarmed mob mass; the followup circle that covers
the second half then faces an oversaturated ground it cannot clear.
The requirements:

- The partition must give EVERY spawn point of the ground exactly
  one owning cell - no respawn area is ever half-covered again (the
  Voronoi assignment fixes this by construction).
- The cell geometry must respect the loading/visibility budget: the
  cell extent from its focus stays inside the guaranteed knownlist
  circle (~2048 units, the Mobius world region grid), so a bot
  standing anywhere in its patrol square sees the whole cell - no
  "left part loaded, right part not" depletion.
- The cell switch algorithm must let the bots travel the map freely
  WITHOUT far runs: the primary moves are the adjacent cells
  (the Voronoi neighbor graph), the rotation is paced by the respawn
  ripeness (a cell cleared at T is ripe at T + respawn window), so
  neither depletion nor oversaturation can build up.
- The map view carries the minimal set: the cell the bot is heading
  to as ONE highlighted element, the rest of the map outlined by
  the cell edges only (no permanent fills or shading - the render
  load matters), the static mesh served once per registry version
  (not in every live snapshot).
- The manual zone management retires entirely: the zone count of a
  full project grows past 1k, a hand-switched list is meaningless.
  The zone panel, the hunt buttons and the CommandZone path go.

### Plan

1. `tools/generate_hunt_cells.py`: the Voronoi partition generator -
   the seeds are the live-audited spot anchors (the 2026-09-13
   audit-validated geometry), the cells are the half-plane clipped
   Voronoi polygons over the spawn ground envelope, the mob
   composition comes from the territory sample points assigned by
   the nearest seed (complete coverage by construction), cells whose
   extent exceeds the leash bound split until stable. Emits
   `hunt/cells_elven.go` + the JSON twin + the change report.
2. `state`: the `ZoneArea` interface (the square `*Zone` satisfies
   it) + the convex `CellZone` polygon leash; the target searches
   take the area, the movement machinery keeps the inscribed patrol
   square.
3. `hunt`: the `cellHunter` replaces the `spotHunter` - the polygon
   target leash, the neighbor-first rotation paced by the respawn
   ripeness, the kill-EMA dynamic focus, the shared occupancy hub,
   the level windows and the death heat of the spot economy ported.
   The spot registry files retire.
4. `webserver`: the static hunt mesh endpoint + the slim live cell
   view in the snapshot.
5. `webui`: the Voronoi edge layer + the active cell highlight; the
   zone list panel and the manual zone command are removed.
6. Docs: `docs/hunting_cells.md`, the hunting.md sections, this
   progress log.

### Acceptance criteria

- Every spawn sample point of the registry maps to exactly one cell
  (the generator test pins the total mob mass and the coverage).
- Every cell: the maximum distance from the focus to any assigned
  sample point stays under the leash bound (1448); the patrol square
  is inscribed in the cell polygon; the neighbor relation is
  symmetric.
- The rotation: a cleared cell is not re-entered before its respawn
  window passes (ripeness); the picker prefers adjacent cells; the
  far relocation only fires when the level window empties the
  neighborhood; a repro test pins the no-depletion rotation.
- The map: the mesh edges render for all cells, the active cell is
  the only highlighted element, no fills on inactive cells, the mesh
  payload is fetched once per registry version.
- No manual zone control path remains (state, webserver, hunt, JS).
- `task check:all`, `golangci-lint run --new`, the web UI harnesses
  and a live smoke run against the deployed stack are green.

### Progress

- Commit 3b1bb7e "state: the zone area interface and the convex cell
  zone" (2026-09-13): the ZoneArea interface (the square *Zone
  satisfies it, the scans take the area), the convex CellZone
  polygon leash with the int64 cross products, the nil semantics
  through areaNil, the containment tests.
- Commit 3af3958 "hunt: the voronoi cell registry generator and the
  cell model" (2026-09-13): tools/generate_hunt_cells.py (the seeds
  from the audited piece centroids, the half-plane clipped Voronoi
  cells, the nearest-seed mob assignment, the densification to the
  visibility budget, the inscribed patrol square, the neighbor
  graph), the 349 cell elven registry (812 mobs preserved, the
  patrol*sqrt(2)+radius <= 2048 invariant, the symmetric adjacency,
  the live audit cross-check: 99.9 percent of the 1629 observed mobs
  belong to exactly one cell) and the registry invariant tests.
- Commit (next) "hunt: the cell policy replaces the spot mode": the
  cellHunter economy (the polygon target leash through
  Loop.targetZone, the patrol square movement, the neighbor-first
  rotation paced by the respawn ripeness - a cleared cell re-opens
  only after its respawn window - the starve livelock net, the
  2-hop ring widening, the far relocation only when the level
  window empties the neighborhood, the kill-EMA, the occupancy hub,
  the death heat, the income attribution), the spot files retire
  (spot.go, spot_policy.go, spot_metrics.go, spots_elven.go and
  their tests), the manual zone selection retires (CommandZone
  path, userZoneSelect, the zoneOverride machinery), the spotaudit
  package becomes huntaudit (the -hunt-audit flag, the polygon
  leash verdicts), the state hunt mesh + live cell record ride the
  snapshot (both encode paths mirrored, the golden test extended),
  the /api/hunt-mesh endpoint serves the static payload with the
  ETag.
- Commit 7029f6f "webui: the voronoi cell layer and the manual zone
  control retirement" (2026-09-13): the map draws the partition (the
  static mesh edges as one version-keyed cached raster - no fills, no
  shading of the inactive cells - plus exactly ONE highlighted
  element: the cell the bot holds or walks to with the farming/moving
  marker, the respawn clock and the measured income), the mesh
  payload fetches once per version through the new
  `/api/hunt-mesh` endpoint (the ETag is the version, the endpoint
  test pins the 304 and the 404 paths), the snapshot carries only the
  version marker plus the live cell record, the zone list panel, the
  hunt buttons and the focus machinery retire from the DOM, the CSS,
  app.js and map.js, the harnesses follow (the zone focus scenario
  became the hunt cell layer scenario: the mesh fetch, the edge
  strokes, the single highlight, the moving marker; the HUD zone
  panel checks removed). All eight harnesses green.
- Commit (docs): `docs/hunting_cells.md` (the design: the partition,
  the visibility budget, the ripeness rotation, the mesh endpoint,
  the invariants), the hunting.md spot section rewritten as the cell
  section, the webui.md zone drawing and cache sections updated, the
  hunting_system_redesign.md superseded note, the README/AGENTS doc
  maps.
- Status: done (2026-09-13). go build, go vet, the full `go test
  ./...` suite, `golangci-lint run --new` (only the branch wide gci
  formatter artifact the whole tree carries), `task fmt:check` and
  all eight web UI harnesses green; the live smoke run of the
  deployed stack verifies the cell rotation.

## Active task: the repository-wide switch to spaces only (2026-09-13)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user ordered the full whitespace conversion: every file of the
repository must use spaces only (no tabs anywhere, the included
vendored skills and go.mod too), the gofmt setup must be configured
for spaces as well, and AGENTS.md must carry the explicit rule. The
trigger was the recurring agent failure "my edits replaced the tabs
with spaces, fixing with gofmt/gofumpt" - the tab policy fought the
editing tools, so spaces become the single style.

### What changed

- `cmd/gofmt-spaces` (new): the formatter of record. It formats
  exactly like gofmt (go/format.Source) and then widens every tab of
  the canonical output to four spaces OUTSIDE string and character
  literals (a go/scanner pass locates the literal spans, so the tab
  of a literal is data and stays). Unit tests pin the literal
  protection, the field alignment, the idempotence and the re-indent
  of the wide 8-space files the session-journal round left behind.
- `tools/normalize_whitespace.sh` (new): widens the tabs of the
  tracked non-Go text files - every tab of go.mod and the markdown
  (no string literals), only the leading whitespace run of scripts
  and data (a tab inside a shell string literal is data).
- `task fmt` runs both (the Go tree plus `.agents/skills`); the new
  `task fmt:check` fails on any tab of a tracked text file and on
  any gofmt-spaces-dirty Go file; it joined `task check:all`.
- The lint gate: the `formatters` set keeps only `gci` import
  grouping; gofmt/gofumpt/goimports are disabled (they all re-tab
  the tree). `tools/install_dev_tools.sh` builds gofmt-spaces from
  the repo instead of installing gofumpt.
- The four npcdata generators call gofmt-spaces (the binary or
  `go run ./cmd/gofmt-spaces`) instead of the stock gofmt, and the
  system-messages heredoc emits the map entries space indented.
- `tools/install_agent_skills.sh`: the vendored-skills drift check
  compares content ignoring whitespace (`diff -r --brief -w`)
  because the vendored copy is whitespace-normalized on purpose;
  the install mode re-normalizes after every copy from upstream.
- `.editorconfig` (new): indent_style space everywhere, 4 for Go,
  2 for json/yml/js/css/html.
- AGENTS.md: the "Whitespace is spaces only, never tabs" rule in
  Code conventions (the formatter of record, the go mod tidy trap,
  the fmt:check gate), the tech stack and command notes updated, the
  go-verify-loop playbook teaches the new loop.

### Verification

- `git diff -w` is EMPTY over the whole change: the conversion is
  provably whitespace-only (the 388 Go files, go.mod, the tools
  scripts, the vendored skills).
- `git grep -IP '\t'` over the tracked tree: no match - not a
  single tab byte left (the binary geodata/icons/maps never carry
  text tabs; the .l2j/.png/.jpg files are untouched).
- go build, go vet, the full `go test ./...` (22 packages) green.
- Full `golangci-lint run`: 50 issues, down from the 59 pre-existing
  (the 8-space session files re-indented to 4 shed lll findings);
  `--new` clean for the two gofmt-spaces files themselves after the
  gosec G703 nolint (a formatter writes the paths it is pointed at,
  the gofmt -w trust model) and a whitespace fix.
- Known tradeoff: the tab-to-4-spaces widening pushed 78 production
  lines 1-5 chars past the lll 80 limit (max-same-issues caps what
  the report shows; the branch already carries lll findings, the
  total count still dropped). A follow-up reflow can clear them if
  the owner wants; it was NOT mixed into this commit to keep the
  whitespace-only proof intact.
- Pre-existing, untouched: `tools/install_agent_skills.sh check`
  still reports the golang-project-layout drift (the vendored copy
  lacks the skill the pinned upstream commit carries; the same on
  origin before this task - a re-vendoring decision, not a
  whitespace issue).

- Status: done (2026-09-13).

## Active task: the spot geometry live audit, the starve livelock and the bow luring (2026-09-13)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user reported three connected problems of the long-run spot
hunting: (1) test3 does not hunt - it ping-pongs between two spots
("Kaboo Orc Fighter Leader W" elven-spot-53 and "Crimson Spider W"
elven-spot-54, 1142 units apart) forever: both leash squares read
empty (the real spiders stand at x 14563-14703, 62-160 units WEST of
the spot-54 leash border x>=14765; the orcs at x 19053 sit beyond the
spot-53 leash x<=18332), every 90 s the starved switch moves the
hunter to the other ground, which starves identically - a livelock
with zero kills; (2) the spot geometry itself is wrong - the anchors
were clustered from the registry square centers (spawn polygons),
never measured against the live spawn positions, so the fix is a live
audit: launch the real C1 stack, DB-inject the probe character at
every spot anchor, dump what the character actually sees (the
knownlist npc population), and regenerate the spot registry so the
anchors sit on the measured mob centroids, the radii cover the real
mobs and the leashes stay inside the visibility squares; (3) melee
bots must carry a bow: buy bow + arrows, upgrade the bow over time,
restock arrows on the town trips, and LURE fenced mobs (a mob blocked
behind other monsters, unpullable by a walk without aggroing the
pack): equip the bow, shoot the mob from afar, hold the position
while it runs up, then swap back to the melee weapon and fight.

### Acceptance criteria

- A starved ground carries a starvation cooldown: the picker never
  walks straight back into a spot that just starved; when every
  alternative cools down the hunter waits out the respawn instead of
  bouncing. A repro test pins the two-spot livelock.
- `-spot-audit FILE` runs the live measurement: for every spot of the
  registry it DB-injects the probe character at the anchor, enters
  the world, waits out the knownlist, dumps the attackable npcs
  (name, wire template, level, position, distance) and logs out; the
  JSON file carries the full evidence.
- The regenerated spots_elven.go anchors/radii/counts come from the
  audit measurements: the observed mobs of every spot sit inside its
  leash square, the leash stays inside the 2048 visibility circle, the
  mob counts match the observed population.
- The bow luring: the melee bot owns a bow and arrows (bought,
  upgraded, restocked), and the engage answers a fenced target with
  the ranged pull instead of standing idle.
- go build, the full suite, `golangci-lint run --new` and the live
  smoke runs green.

### Progress

- Started: the analysis of the uploaded journal
  (session-20260913-102700-23540) located the livelock (the starve
  lines and the two patrol positions); the geometry mismatch is
  confirmed against the registry (the spider mass sits west of the
  spot-54 leash border). Next: the starve cooldown fix, then the
  audit tool.
- Commit bf39c88 "hunt: the starved ground cools down and its zero
  income scores zero" (2026-09-13): the starvation cooldown of
  pickBest (5 min, the sweep moves forward through the registry
  instead of ping-ponging between the two nearest grounds), the
  trusted zero income scores zero (the bootstrap prior no longer
  promises window mass a starved ground never delivered), the
  livelock repro test.
- The spotaudit package + the -spot-audit CLI: the probe character
  visits every spot anchor through the DB position injection and
  dumps the attackable npc population with the leash verdicts; the
  evidence file resumes across foreground runs (71 spots measured in
  three chunks).
- The audit findings: the old leashes hold 25-40 percent of the
  visible mobs everywhere (the anchors sit off the real mass, the
  territory polygons span 3000-9500 units - a 1448 leash can never
  cover them from one anchor).
- The regeneration (tools/regenerate_spots_from_audit.py): the
  territory polygons sampled on a 128 unit grid, cut into
  visibility-sized pieces (the Chebyshev extent of a piece stays
  under 1448), packed to the minimum piece count, the species counts
  distributed by the area share - 292 spots from 106 territories, the
  leash union covers 96.6 percent of the spawn ground sample points.
- The verification audit (-audit-stride 7, 42 spots at the new
  anchors): every piece observes at least half its own expected mass
  inside its leash (zero LOW pieces), the anchors see their mass plus
  the wandering neighbors - the geometry is live-confirmed.

## Active task: the statistics tab fixes - the exp resets, the adena zeros and the flicker (2026-09-13)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user reported three defects of the new statistics tab: (1) the
experience chart resets to zero when a player gains a level - the exp
must grow linearly, with the level itself (the level ups and the
delevel drops) on a separate scale of the same chart; (2) the adena
growth shows zeros even on a long hunt - fix it and add an adena/hour
metric; (3) the per bot metrics flicker on every recalculation - the
hunt tick time, the packet rate, the phase timeline and the events
visibly change between the five second polls.

### Root causes

- The adena zeros: `state.SelfSnapshot` hardcoded `adena: 0` (the
  stats sampler reads the wallet from it); the full `Snapshot()` path
  computed it correctly through `fillInventorySnapshot`.
- The exp "reset": the tracker character is zeroed by `ResetSession`
  during every relogin until the fresh UserInfo arrives; a statistics
  sample that lands inside that gap recorded exp 0, level 0, adena 0 -
  the expGained series crashed to the bottom (the chart "reset to
  zero"), the live KPI cards flashed level 0 and the event ring
  collected fake "reached level 0" events plus a second level event on
  the recovery. The exp itself is the C1 cumulative total (verified
  against the Mobius sources: `PlayableStat.addExp` does
  `setExp(getExp()+value)`, UserInfo broadcasts `(int) getExp()`), so
  the raw series never resets on a level up.
- The flicker: the history downsampling picked every stride-th sample
  by ARRAY INDEX - every appended sample and every window slide (the
  cut follows the poll clock) re-aligned the picks, so the noisy
  series (tick time, packet rate, phase colors) visibly jumped between
  the polls once a window held more than the 256 point bound.

### Acceptance criteria

- The adena wallet of every stats sample and view comes from the
  tracked inventory; the adena/hour metric exists in the per bot view,
  the fleet view and the bots table.
- A reconnect gap never zeroes a sample (the character facts carry
  through), never fakes a level event and never flashes the live KPI
  cards; the series baselines anchor on the first valid sample only.
- The exp chart carries the level as a staircase on its own right hand
  scale; the net exp line stays linear through the level ups.
- The served history points are a pure function of the sample
  timestamps (epoch aligned time buckets, the last sample per bucket)
  - two polls of one window derive the same points; a fresh sample
  only refreshes the trailing bucket.
- The event list keeps its DOM while the event set is unchanged (the
  ago labels refresh in place).
- go build, the full test suite, `golangci-lint run --new` and all
  six web UI harnesses green; a live smoke run against the deployed
  stack verifies the series.

### Progress

- Commit 0301743 "state: SelfSnapshot carries the tracked adena
  wallet" (2026-09-13): the compact self view now sums the adena items
  of the inventory store instead of the hardcoded zero (the statistics
  sampler and the proxy read the real wallet). Unit test added.
- Commit 37ba95a "webserver: the stats samples survive the reconnect
  gap and the adena income lands" (2026-09-13): the gap carry-forward
  of the character facts (exp, adena, level, health), the `based`
  baseline anchoring on the first valid sample, the live view hold
  through the gap, the adena income metrics (AdenaGained/AdenaPerHour
  of the bot view, AdenaGained/AdenaPerHour of the fleet view, the
  fleet adena sample and history series) and the epoch aligned bucket
  downsampling that replaces the index stride. Unit tests: the gap
  carry, the adena income, the bucket picks and the poll stability.
- Commit d8dd478 "webui: the level scale of the exp chart, the adena
  income and the steady events" (2026-09-13): the chart engine gained
  a per series right hand axis and step rendering (the golden level
  staircase next to the blue exp line), the adena chart draws the
  wallet plus the net gained line, the fleet adena chart and KPI card,
  the adena income sub lines of the bot KPI card, the adena/h table
  column and the keyed event rendering (the DOM survives an unchanged
  event set, the ago labels refresh in place). The harness pins 25
  checks including the level staircase, the adena lines and the keyed
  events.
- Live smoke run (2026-09-13): a fresh hunting bot leveled 1 -> 3
  over ten polls - the exp series monotonically grew through both
  level ups (no zero dips, no level 0 samples, no fake events), the
  adena wallet grew 8 -> 58 with the per hour rate, and the history
  prefix stayed identical between the polls.
- Status: done (2026-09-13). go build, go vet, the full suite,
  `golangci-lint run --new` (0 issues) and the harnesses green; the
  live smoke run PASSED.

## Active task: the session journal and the session dump report (2026-09-12)

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The dump-state snapshot is a moment picture and the tracker event
ring holds only 512 lines, so long unattended runs (8-24 h) had
nothing to analyze post-mortem. Build the three-layer session
evidence trail: the append-only JSONL journal per bot in logs/ (the
story mirror, the 30 s samples, kill/death/level/trip/buy/sell/zone/
stall/repath/lifecycle events, 64 MB rotation with gzip), the in-memory
aggregator (hourly bins, per-mob stats, fight histograms) and the
compact text report rendered from it, plus the web UI "session dump"
button that copies the report to the clipboard and the offline
`-session-report FILE` CLI.

### Acceptance criteria

- The journal writes every session event to logs/session-*.jsonl
  with rotation and never blocks the tracker (channel sink).
- GET /api/bots/{id}/session-report serves the report; the button
  copies it to the clipboard.
- The CLI renders the same report from a journal file.
- go build, the full suite and golangci-lint --new green; live runs
  verify the journal growth and the report content.

### Progress

- Commit c759f9e "session: the persistent session journal and the
  session dump report" (2026-09-12): internal/swarm/session (events,
  journal, aggregator, report, reader, sampler), the state event
  sink mirror, the hunt emission points, the -session-dir and
  -session-report flags, the web endpoint and the button.
- Commit b3cd6bb "session: register the fleet report endpoint and
  flush the first record at once" (2026-09-12, pushed 2026-09-13
  after the sandbox credential reset): the owner reported the button
  dead and a 0 KB journal - the fleet mode never called
  web.SetSessionJournal (the endpoint 404ed) and a hard kill inside
  the first 2 s flush window left the empty file. The fix registers
  the endpoint in runFleet, flushes the first record immediately and
  makes the button failure readable (console.error plus the report
  endpoint opened in a new tab). Round 79 of development_log.md
  carries the full root cause.
- Status: done (2026-09-13). Both symptoms reproduced and fixed
  live: the fleet endpoint answers 200 for every bot, the journal
  carries the identity line from the first ~100 ms; build, vet,
  lint --new (0 issues) and the full suite green.

## Active task: the bot statistics tab of the web UI (2026-09-13)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user asked for a dedicated web UI tab that visualizes the
behavior statistics of the bot fleet: how effectively the bots
fight, how they behave, how often they die and rejoin the game - the
overall picture of all bots and the per bot picture, with charts,
plus the long term observation tools (a day long run must read back
through the tab) and the technical health metrics (memory, tick
time) that matter when many bots run at once.

### Acceptance criteria

- A new Stats tab next to Map/Log of the bot control mode.
- The fleet view: KPI cards, history charts, a sortable comparison
  table of all bots.
- The per bot view: counters, per bot history, the phase
  distribution, the event timeline.
- The counters are exact and documented (kill attribution, death
  transitions, rejoins, swings, damage taken, tick duration).
- The history is bounded (the ring compaction) and the memory stays
  bounded for a 24/7 process.
- go build, the full test suite, golangci-lint --new and the web UI
  harnesses green; a live smoke run against the deployed stack.

### Progress (2026-09-13)

- Environment redeployed (`tools/swarm_fast_deploy.sh`: STACK_READY;
  `tools/install_dev_tools.sh` complete) - the sandbox was fresh.
- Commit "state: the lifetime bot metrics counters for the
  statistics view": state/metrics.go counts the kills (the death of
  the actively fought object, packet level attribution), the deaths
  (the alive to dead HP transition, one per demise), the sessions
  (every ResetSession - the rejoin story), the swing counters
  (made/landed/taken with the miss flag split) and the cumulative
  damage taken; NoteHuntTick feeds the hunt loop tick duration (EMA
  + the worst of the last minute). The hunt loop measures its tick
  and publishes it. Unit tests: metrics_test.go (10 cases).
- Commit "webserver: the statistics collector and the /api/stats
  endpoints": a 15 s sampler walks every registry tracker into
  bounded rings (2048 samples, the half-on-full compaction keeps the
  memory bounded whatever the uptime), derives the transition events
  from the counter deltas and aggregates the fleet ring with the
  process memory view. GET /api/stats (fleet view) and
  GET /api/stats/{id} (per bot view) serve the downsampled history
  (?window= seconds, default day, 0 all). The acceptance bots stay
  out of the fleet aggregates. Tests: stats_test.go (11 cases).
- Commit "webui: the bot statistics tab": the Stats tab of
  index.html (the toolbar with the window selector, the KPI cards,
  the chart grid, the bots table, the bot detail panel), stats.js
  (the pure canvas chart engine with the theme colors, the fleet and
  bot renderers, the 5 s polling that stops while the tab is
  hidden), the styles, the main.js hook and the preview server stubs.
  The repro_stats.js harness pins the rendering path (16 checks).
  The x axis edge labels clamp inside the plot (the vision review
  found the right-most label clipped).
- Live verification: 2 bots hunted for ~2 minutes against the
  deployed stack - /api/stats served 4 kills, 1 rejoin, the 0.09 ms
  average tick, the 83 percent hit rate; /api/stats/test9 served the
  kill/level events and the phase distribution. The headless browser
  walk (preview server) rendered the fleet view, the bot detail
  view and the dark theme.
- Docs: the webui.md statistics tab section, the endpoints list and
  the harness inventory entries.
- Status: done (2026-09-13). go build, go test (state, hunt,
  connection, webserver), golangci-lint run --new (0 issues) and all
  six web UI harnesses green; the live smoke run PASSED.

## Active task: the round 60 gear debt - the pantsless town trip of the 2026-09-12 04:58 dump (2026-09-12)

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user reported (state dump, build 4deb888, bot test2, phase
engage, uptime 1m44s) that the level 14 elven fighter returned from
its town trip WITHOUT the legs armor: every other slot filled, the
bag holding nothing but the 13162 adena, "town trip ended: back at
the farm spot" ten seconds before the dump. Find out why the bot
stayed without its pants and fix the market handling so similar
problems can never arise again.

### Acceptance criteria

- The root cause is named and documented (development_log round 60).
- A town trip can never strand a paperdoll slot silently: every trip
  exit detects a slot it left worse than it found it and arms gear
  debt with a log line.
- The armed debt shortens the trip cooldown to the gear run window
  and the refill trip dresses the slot; the debt clears on the
  refill.
- The state dump shows the empty paperdoll families (the report's
  hole was invisible - only occupied slots printed).
- Reproductions pin the machinery at the unit level and the exact
  dump state is injected into a live acceptance scenario ("gear
  gap") that must buy the legs armor back.
- go build, the full test suite and `golangci-lint run --new` green;
  the live scenario PASSes against the deployed stack.

### Progress (2026-09-12)

- Commits 041d945 "hunt: the gear debt - the town trip answers for
  the slot it stranded" and c0df26b "docs: the round 60 gear debt":
  the town trip exit detects a stranded paperdoll slot and arms the
  gear debt, the armed debt shortens the trip cooldown to the gear
  run window and the refill trip dresses the slot, the state dump
  prints the empty paperdoll families.
- Status: done (2026-09-12). The "gear gap" acceptance scenario
  PASSed live (the legs armor bought back into the empty slot);
  development_log Round 60 carries the full root cause and the fix.

## Active task: the sidebar LIVE/TESTS tab switch (2026-09-12)

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user wants to switch between the live bots and the acceptance
bots/scenarios instead of seeing both stacked in the same vertical
sidebar. The previous split (two groups under the BOTS header) was a
first iteration; the user asked for an explicit view switch.

### Acceptance criteria

- The sidebar carries a LIVE/TESTS tab strip at the top.
- LIVE shows only the long-running fleet bots; TESTS shows the
  acceptance bots and the scenarios panel.
- The active tab persists in localStorage (like the theme toggle).
- The pathfind and the fight modes keep hiding the whole bot section
  (they have no bots and no scenarios).
- An empty hint shows when the active view has nothing to render.

### Progress (2026-09-12)

- Environment redeployed (`tools/swarm_fast_deploy.sh`,
  `tools/install_dev_tools.sh`) - the session sandbox had been
  reset, Go 1.24.4 back; `go build ./...` green.
- Commit "webui: switch sidebar between LIVE and TESTS views through
  a tab strip": the bot-section now opens with a tab strip, the two
  views are `#view-live` (just the long-running bot list) and
  `#view-tests` (the acceptance bots group + the scenarios panel).
  Added `selectSidebarTab` / `initSidebarTabs` in app.js, the
  choice persists through `swarm.sidebarTab` in localStorage. The
  pathfind and fight modes hide `#view-tests` alongside the bot
  section. Empty hints show when the active view has nothing.
- Status: done (2026-09-12). `go build ./...`, the HTML structure
  (145 div opens / 145 div closes), `node -c app.js` and
  `golangci-lint run --new` green; the pathfind test UI smoke
  served the new tab strip on `http://127.0.0.1:8081/`.

## Active task: the round 59 phantom chase livelock - the "Cannot see target" standoff that never armed the blind recovery (2026-09-12)

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (the 03:25 state dump, build a4c9e15, bot test1,
phase engage, uptime 7m13s): the bot stood 80 units from the NPC it
wanted to attack for over a minute - no movement, no walk plan, no
target switch - while the server answered every attempt with
"Cannot see target." every ~3 s and the bot kept casting Power
Strike at the invisible target every 15 s. The user asked to
reproduce and fix it from three angles: the pathfinding (the
reposition walk), the pathfinding activation (why it never turned
on) and the target abandonment (give the target up after several
seconds of refusals).

### Root cause

The phantom chase of the refused attack: the server AI of the armed
ATTACK intention broadcasts the character's own MoveToPawn chase
steps while every doAttack of the same intention fails the
canSeeTarget check and answers "Cannot see target." - the chase
stream kept the tracker's fresh fight view (CombatActiveAt,
FightingTargetID) alive in a ~3 s cycle (chase -> refusal -> disarm
-> 3 s staleness -> the 1 s paced re-request re-arms the chase), so
SelfFighting read true, the fighting branch re-anchored the engage
clock past every refusal (holding the 12 s stuck timeout away
forever) and blindEngageBlocked died at its SelfFighting gate: the
recovery built for exactly this refusal never armed. See
`docs/development_log.md` Round 59 for the full trace.

### Fix (the refusal-vs-activity ordering rule)

1. `state/bot.go`: `SelfCombatActiveAt` exposes the raw last fight
   activity timestamp.
2. `hunt/loop_los.go`: `blindEngageBlocked` detects the block when a
   fresh refusal of the current attempt is the NEWEST fight activity
   (the SelfFighting early-out is gone); `fightClearedRefusal`
   gates the recovery standdown on activity strictly newer than the
   refusal.
3. `hunt/loop.go`: the engage clock re-anchor of the fighting branch
   requires the same progression past the last refusal.

### Acceptance criteria

- The exact dump standoff (the dump positions, the dump zone, the
  phantom MoveToPawn chase, the fresh refusals) arms the recovery
  and walks the reposition leg on the arming tick
  (round59_repro_test.go).
- The persisting refusal spends the two attempts and ends in the
  target switch with the skip list holding the obstructed mob out
  and the next pick taking the spare mob of the dump scene - the
  80 s livelock is bounded to the recovery budgets.
- The round 56/57/58 contracts and the whole loop_los_test.go suite
  stay green (the fresh fight guard: a chase step after the refusal
  still reads as a running fight).
- go build/vet/test/lint:new green.

### Status: done

- Commit 1 (the hunt fix + the repro tests + the docs): the ordering
  rule, the SelfCombatActiveAt accessor, the three round 59 repro
  tests, the hunting.md blind recovery section update, the
  development_log Round 59 entry, this entry. All tests and
  `golangci-lint run --new` green.
- Live stack validation: the deploy ran green before the work
  (STACK_READY); the unit repro pins the exact dump behavior. A
  longer live soak against the running server is the natural next
  step for a future round (the Mobius geodata of the standoff cell
  decides whether level A clears the line or level B switches the
  target - both are pinned and bounded).

## Active task: the trainer hall building entry - the frozen corridor ban and the close teacher ring (2026-09-12)

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user reproduced the 2026-09-12 03:56 hang locally (build 6a2ac91,
bot temp1, phase townWalk, the state dump of the report): the
character could not enter the trainer hall building - the learn leg to
the teacher Ellenia froze at the west aisle entrance (44728 51992
-2792) through every re-path of its trip. The demands:

1. A dedicated test for entering the building (the entrance -> the
   npc walk).
2. The character must walk from the building entrance right up to the
   training npc ("вплотную к npc для обучения") - not talk to it
   through the wall from wherever the geodata leg happened to end.
3. Fix the situation in general: the freeze must recover on any
   server configuration, whatever geodata disagreement causes it.

### Acceptance criteria

- An offline reproduction with the exact dump positions against the
  real geodata pack models the freeze (a server that walls the aisle
  corridor the pack models as open) and passes with the recovery.
- A live acceptance scenario (`building-entry`) runs the whole dump
  town visit from the aisle entrance: the weapon run, the purchases,
  the teacher leg through the building entrance, the first lesson.
- The full verify loop green: build, vet, the full test suite, gofmt,
  `golangci-lint run --new` clean (the full count stays at the
  pre-existing baseline).

### Progress (2026-09-12)

- The live probe against the sandbox stack walked the dump clicks
  clean (the sandbox server runs without geodata regions and accepts
  every click), localizing the freeze to a server whose geodata
  disagrees with the pack at the aisle cells - the fix had to make
  the recovery work on any server configuration.
- Commit "pathfind: the avoid areas of the approach search": a new
  `FindPathApproachDryAvoiding` search takes banned world patches -
  the A*, the direct line shortcut and the smoothing all refuse the
  banned ground, so a route around the corridor exists whenever one
  exists at all.
- Commit "hunt: the frozen leg escalation ladder": the frozen abort
  of a town walk leg climbs rungs - the corridor ban re-plan first
  (the session keeps the ban), the direct server routed walk second
  (the stop's npc approach point, bounded by a 45 s window), the
  plain trip abort last. The zone return keeps its own escalation.
  The `waypointBehindRoute` far-waypoint sabotage fixed (the V-shaped
  detour routes misjudged their far waypoints as behind).
- Commit "hunt: the teach legs walk the close ring up to the npc":
  the teacher stops search their route within npcApproachOffset with
  the wide ring fallback, the tight legs complete their route end
  with the pass radius, and the teacher approach walks the npc
  approach point ring before the talk (the approach window owns the
  dead ends).
- Commit "hunt: the delevel trigger requires the static spot median
  agreement": the live zone median flickers with the respawn windows
  and de-leveled a healthy level 15 on the building entry round; the
  trigger now requires the anchored spot's static median to agree and
  the spot picker skips grounds whose static median sits at the gap.
- Commit "acceptance: the building entry scenario": the temp5
  character wakes at the dump aisle entrance with the dump's town
  visit start state (level 15, 20k SP, 100k adena, the empty
  inventory - the dump's own weapon run arms at once, its exact
  first log line reproduces) and must reach Ellenia inside the hall
  with a lesson consumed. The live stack run passed in 2 m 10 s:
  the weapon run, the eleven purchases across Ariel, Unoren and
  Creamees, the teacher leg reached Ellenia inside the hall, Power
  Strike level 1 learned.
- Status: done (2026-09-12). The offline reproduction
  (hunt/building_entry_test.go) passes on both the agreeing server
  (the aisle plan walks clean, the talk at 149 units) and the walled
  model (the ladder rescues the leg, the talk within the interaction
  distance); the pathfind ban tests pin the detour shape; the full
  verify loop is green.

## Active task: the sidebar split, the test widget buttons and the CLI acceptance flag (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

Three user requests in one session:
1. Split the left sidebar tab into the long-running bots and the
   acceptance test bots. The fleet bots (`test1`, `test2`, ...) and
   the temp bots of the acceptance manager (`temp1`, `temp2`, `temp3`)
   land in the same flat list today; the user wants them grouped.
2. Fix the test widget buttons - they are too short ("куцые") in the
   sidebar acceptance panel.
3. Add a CLI flag so an agent can launch an acceptance test without
   entering the web UI (the same scenario the run button starts).

### Acceptance criteria

- A `kind` field on `state.Bot` tags the role of every bot; the
  acceptance manager tags its temp bots, the main entry tags the
  fleet bots.
- The sidebar `#bot-list` splits into two groups (long-running and
  acceptance) under a sub-title each; the empty group hides.
- The run all and the per scenario run buttons of the acceptance
  panel grow a comfortable click target (font-size, padding).
- A new `-acceptance <id|all>` flag launches the scenarios headless:
  no bot supervisor runs, the result prints to the log and the exit
  code reflects the pass/fail of the run.
- Every change ships with its unit test; the lint gate stays clean
  on the new lines.

### Progress (2026-09-11)

- Environment deployed per AGENTS.md before touching the code:
  `tools/swarm_fast_deploy.sh` ran in the foreground with a 10
  minute call timeout - `STACK_READY`, 75 tables, login 2106 /
  game 7777 / db 3306 listening; `tools/install_dev_tools.sh check`
  green (task, golangci-lint, gci; gofumpt missing but gofmt +
  gci cover the change); `go build ./...` green.
- Commit "state: tag bot role with Kind for the sidebar split":
  added `kind` field on `Bot`, `SetKind`, the `Kind` constants
  (`KindLongRunning`, `KindAcceptance`) and the `Kind` field on
  `BotInfo`; the acceptance manager tags its temp bots and the main
  entry tags the fleet bots. Added `TestBotKind` next to the change.
- Commit "webui: split sidebar bot list into long-running and
  acceptance groups": the sidebar `#bot-list` now renders two
  sub-groups (long-running and acceptance) under sub-titles, the
  empty group hides. The `buildBotItem` helper builds one plaque
  shared by both groups.
- Commit "webui: grow acceptance panel run buttons past the 26px
  icon tile": the run all and the per scenario run buttons grew a
  comfortable click target (font-size 11px, padding 6px 12px and
  4px 12px, min-height 28px and 24px, width auto, hover lift).
  The base `.btn` (26x24 px icon tile) had been cropping the text.
- Commit "acceptance: add -acceptance CLI flag for headless
  runs": a new `-acceptance <id|all|list>` flag drives the
  scenarios without the web UI. Added `Manager.Run` and
  `Manager.RunAll` (block until terminal state, return an error on
  fail), `Manager.IDs` and `DefinitionsIDs` (the CLI discovery),
  and `runAcceptanceCLI` in `cmd/swarm/main.go` (builds the
  registry, the optional proxy, the geodata engine and the manager;
  prints the result; exit code 1 on fail). Added tests for `Run`,
  `RunAll`, `IDs` and `newAcceptanceManager` next to the changes.
- Status: done (2026-09-11). `go build ./...`, the affected
  packages tests and `golangci-lint run --new` are green.

## Active task: the round 57 pathfind freeze - the un-rescuable short click and the identical re-plan (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian, the 11:34 state dump, build
d0cd543, bot test2, phase townReturn): the character stood at
(44296 51480 -2848) with the 11 waypoint plan to the hunting zone and
never moved a cell - "town walk stuck, re-pathing" burned the budget
twice (two whole trip cycles) with no refusal log. Find the source,
test it, fix it.

### Root cause (live probed)

The plan is valid and the local stack walks the exact dump scenario in
one go (MOVEDBG diagnostics build, both PathFinding modes, the
byte-identical geodata). The freeze is the interaction of the SHORT
first click (22 units to the terrace step waypoint) with the server's
own move machinery: Creature.moveToLocation hands a collapsed click
to the server pathfinder only when (originalDistance - distance) > 30
- a collapsed click under ~31 units is silently canceled (ActionFailed,
no movement, invisible to the offline validation). The user's server
collapsed that click; the bot kept re-clicking it, and every re-path
re-planned the identical route with the identical un-rescuable first
click.

### Fix

1. minWalkClick (50) - the armed short click extension: after the
   first stuck with no clear successor, the follower's short or
   backward clicks re-aim at the forward route samples past the rescue
   threshold (the march skips under-floor and backward samples, the
   water and validation port gate every sample, a walled sample skips
   forward).
2. frozenRepathLimit (1) - the identical re-plan rule: a re-path from
   the same cell that produced no movement aborts the trip at once
   and the zone return escalates straight to the direct server routed
   legs.

### Acceptance criteria

- The exact dump scenario under the freeze server model (the short
  clicks canceled, the long clicks walked or rescued) arrives at the
  zone within one recovery re-path, every armed click at least the
  floor length.
- The zone return sweep from eight village positions arrives under
  the same model.
- The total-freeze server aborts after one no-movement re-path and
  escalates the zone return to the direct legs.
- The teacher ramp design (the round 56 short waypoint clicks) stays
  untouched before any stuck.
- go build/vet/test/lint green; the live stack validates the dump
  walk and mobius_e2e stays E2E_OK.

### Status: done (2026-09-11)

- Commit "hunt: the round 57 pathfind freeze - the un-rescuable short
  click extends, the identical re-plan aborts".
- Tests: hunt/round57_repro_test.go (the dump freeze walk, the eight
  start sweep, the total-freeze escalation), hunt/click_floor_test.go
  (the march, the behind geometry, the armed/unarmed/hold gating), the
  click guard and walk stuck budget tests updated to the frozen re-path
  contract.
- Docs: development_log.md Round 57, agent_progress.md this entry,
  hunting.md the follower paragraph.

## Active task: the dump state diagnostics for stuck bot reports

Started: 2026-09-11. Branch: `feature/acceptance`. Commits as melg8,
pushed as they land. Stack deployed and verified as the mandatory
first step (tools/swarm_fast_deploy.sh: STACK_READY, ports
2106/7777/3306, 75 tables).

### Goal

The user asked for an analysis of what the state dump (the JSON
snapshot of `GET /api/bots/{id}/state` and the SSE stream) carries
today, what is missing, what is redundant, and an improvement so the
dump works as a live server report when bots get stuck or behave
inadequately.

### Gap analysis of the current dump

The snapshot carries: id, status, phase, a 39 field character view,
the full inventory, every world object (31 fields each), the last 100
events, the last 64 chat lines, the manual/town walk plan, the 2
second combat animation window, the hunting zones, packets, version,
serverTimeMs, startedAt, updatedAt.

Missing for a stuck bot report:

1. No liveness ages or rates: the packet counter is cumulative with
   no rate, updatedAt carries no age, the phase has no age - a bot
   stuck in townWalk for 15 minutes is indistinguishable from one
   that just switched.
2. No reconnect visibility: the login cooldown the emergency logout
   arms is tracked but never published, so an offline bot carries no
   reason.
3. No hunt loop internals: the loop state (current target, engagement
   age, the engage skip list, the re-path count, the stuck watchdog,
   the flee episode, the trip age, the buy retries) never reaches the
   tracker, and every loop decision is printed to the console logger
   only - the dump has no WHY.
4. Coarse combat view: inCombat is one boolean; the auto attack flag,
   the fighting target, the combat activity age and the hit age are
   missing, so a stale-flag fight is indistinguishable from a live
   one.
5. Walk freshness: the moving flag has no fresh window companion (a
   lost stop packet leaves it set forever).
6. No object summary: the known list health (npc/player/item/dead
   counts) requires scanning the whole array by hand.

Redundant for a report (kept anyway): the combat animation beats, the
per-object vitals and the per-item icon/name fields are UI payload of
the same endpoint - the diagnostics section adds the report layer
without growing the per-object cost.

### Fix plan

1. state: a `diagnostics` section at the end of the snapshot - phase
   age, update age, the 10 second packet rate, the login cooldown,
   the combat nuance (autoAttacking, fightingTargetId, combat
   activity age, hit age, under attack, attacker count), the walk
   freshness, the object counts, and the hunt subview (target,
   engagement age, skipped targets, no-target age, re-paths, stuck
   age, waypoints left, trip age, flee age, buy retries, last action
   and its age, loop tick age).
2. hunt: the loop publishes its internals every tick and routes its
   decision log lines into the tracker event log, so the dump events
   array carries the decision history and the last action.
3. webserver: the footer and the activity banner surface the key ages
   so a stuck bot is visible in the live UI too.

### Acceptance criteria

- The reflection golden suite and the live encode parity tests stay
  green with the new section (byte identical paths).
- Unit tests for every diagnostics field family (phase age, packet
  rate window, cooldown, combat nuance, walk freshness, counts, hunt
  publication, note action).
- go build, go vet, go test ./..., golangci-lint run green; the live
  stack e2e prints E2E_OK with the diagnostics flowing.

### Status: done (2026-09-11)

- In progress: the state diagnostics section first.
- 2026-09-11: the state diagnostics section. A new
  state/diagnostics.go defines the Diagnostics view (phaseForMs,
  updatedAgoMs, packetsPerSecond over a 10 second window,
  loginCooldownMs, autoAttacking, fightingTargetId,
  combatActiveAgoMs, lastHitAgoMs, underAttack, attackerCount,
  walkFresh, moveAgoMs, the ObjectCounts summary, the
  HuntDiagnostics subview) and its support state: the phaseAt stamp
  of SetPhase, the packet rate window fed by CountPacket, the hunt
  publication (SetHuntDiagnostics, no version bump - the values ride
  the packet driven snapshots) and NoteAction (the event log entry
  plus the last action of the hunt view). The ages floor to whole
  seconds through state.AgeMs so both encode paths stay byte
  identical without a now race. The snapshot struct gains the
  trailing diagnostics field; the reflection golden fixture covers
  every new branch; the live encoder walks the object array once and
  folds the hot records into the worldCounts tally (npcs, players,
  items, dead, attackers) reused by the diagnostics. Tests:
  diagnostics_test.go pins the rate window, the phase age, the
  update fallback, the cooldown, the walk freshness stall signature,
  the combat nuance, the object counts with the attacker tally, the
  hunt publication with the last action and the reset. go build, go
  vet, go test ./... green; golangci-lint 0 issues in state (the 10
  remaining findings of the full run reproduce on the untouched
  HEAD with the local golangci-lint 2.6.2 - linter version drift,
  not this change). Next: the hunt loop publication and the log
  routing.
- 2026-09-11: the hunt loop publication. A new
  internal/swarm/hunt/loop_diagnostics.go adds Loop.diagnostics (the
  internals report: the target, the engagement age, the active skip
  count of both skip maps, the no-target patience age, the re-path
  count, the stuck watchdog age, the waypoints left of the manual or
  geodata plan, the trip and flee episode ages, the buy retries)
  published through the extended tick defer together with the phase.
  The 118 loop decision log lines now route through Loop.logf: the
  message still prints on the console logger and additionally lands
  in the tracker event log (Bot.NoteAction), so the dump events
  array carries the decision history of the session - the WHY a
  stuck bot report needs. Tests: the per tick publication (target,
  ages, heartbeat, phase age), the stale server side selection skip
  flow (the skip count and the "does not engage" line in the event
  log and the last action), the death decision routing, the age
  references and the per phase waypoint counting. go build, go vet,
  go test ./... green; golangci-lint clean in hunt and state (the
  remaining gosec G602 findings in zones_test.go reproduce on the
  untouched HEAD). Next: the web UI surfaces.
- 2026-09-11: the web UI surfaces. The footer gains two cells -
  phase with its age and the live packet rate - and the existing
  cells grow the diagnostic detail: the object count splits into the
  npc/loot/dead summary and the updated cell shows the age of the
  last state change (the world liveness) instead of a wall clock
  stamp. The activity banner detail reads the hunt subview: the
  fighting target with its engagement age, the no-target patience
  with the skip count, the waypoints left and the trip age of the
  town walks, the buy retries of the sell stop, the login cooldown
  of an offline session. The log tab colors the new hunt decision
  lines (the Hunt: prefix) with the accent color so the decision
  history reads out of the noise. The special modes (pathfind, fight
  galleries) hide the new footer cells through the same CSS rules
  as the existing ones. The state endpoint test pins the diagnostics
  section presence and the object tally agreement. The HUD harness
  (tools/repro_hud.js) passes: the label fallbacks work for
  snapshots without diagnostics. go build, go vet, go test ./... (15
  packages) green. Next: the AGENTS.md documentation and the live
  stack verification.
- 2026-09-11: the phase gating and the documentation. The live run
  showed the residual stuckAt/tripStart stamps leaking misleading
  ages into the engage phase report (a stuckForMs of 28 s while
  hunting): Loop.diagnostics now carries the stuck watchdog age and
  the trip clock only in the walking phases (Loop.walkPhase for the
  stuck age, tripActive plus the delevel guard walk for the trip
  age), the hunt phases report zero. The AGENTS.md web interface
  section documents the diagnostics contract: the field families,
  the hunt publication, the NoteAction decision routing, the
  seconds flooring of the ages and the zero-never semantics.
- 2026-09-11: task wrap up. The live verification on the deployed
  stack: the bot hunted, looted and equipped gear while the dump
  showed the full report - phase for 11 s, the packet rate 6.3/s,
  the fighting target with its engagement age, under attack with
  the attacker count, the known list summary (32 npcs, 1 loot, 0
  dead), the hunt heartbeat fresh, the last action ("equipping
  Cloth Shoes into the empty feet slot") and the decision history
  riding the events array (the target died/loot/equip lines between
  the packet events). tools/mobius_e2e.sh 45 printed E2E_OK. All
  acceptance criteria met: the golden reflection and live parity
  suites stay green with the new section, every diagnostics field
  family has its unit tests, the build, vet, the full test suite
  (15 packages) and the lint of the touched packages are clean, and
  the live report answers the stuck bot questions (is the socket
  alive, is the loop ticking, how long in this phase, what did it
  decide last) straight from GET /api/bots/{id}/state.

- 2026-09-11: the port to feature/proxy-server. The 7 diagnostics
  commits rebased onto the evolved proxy line (the shop planner
  commits of the acceptance branch did not travel: their content
  lives here as the top-tier guard port plus the frozen trip plan
  round). The rebase merged both sides honestly: the proxy hunt
  loop evolution (the road fight budget, the weapon run, the blind
  engage recovery, the trip stops, the non-dry zone return, the
  spot kill marks, sessionAt) stays; the logf routing now also
  carries the new proxy decision lines (the HP/mana sit split, the
  frozen trip plan messages, the manual target death with the blind
  recovery clear); the diagnostics documentation section moved into
  docs/webui.md (AGENTS.md was split into the docs/ modules here).
  The lint run matches the pre-port proxy baseline exactly (4
  findings, all in files this port does not touch); the full
  19-package suite, vet and gofmt are green; the live stack run
  shows the diagnostics flowing - the decision history rode the
  events array, the hunt heartbeat, packet rate and phase gates
  answered green, the bot exited gracefully. The branch line above
  records where the work started; the port is the delivery into
  the proxy line.

## Active task: the foreground execution rule in the agent docs (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user reported that agent sessions keep starting the deploy (and
other long scripts) in the background, lose the process when the
tool call returns, and pay the same rediscovery round every time
("the deploy process died (the background process did not survive
the call completion), restarting in the foreground with a long
timeout - the script is idempotent"). The agent documentation must
require the foreground start EXPLICITLY, in the places every session
reads before running the long commands, so the knowledge stops
living in the session transcripts only.

### Acceptance criteria

- AGENTS.md states the foreground rule inside the mandatory first
  step section (the place a session reads right before deploying).
- `docs/deployment.md` carries the full rule as its own section:
  what happens to background processes when a tool call returns,
  which commands must run in the foreground, the timeout budget and
  the idempotent re-run path for a call that died anyway.
- The `mobius-stack` and `go-verify-loop` skills repeat the rule
  where their long commands live (the deploy and the verify loop).
- The environment is deployed per the docs first (fast deploy +
  dev tools), so the docs change happens on a verified stack.

### Progress (2026-09-11)

- Environment deployed per AGENTS.md before touching the docs:
  `tools/swarm_fast_deploy.sh` ran in the foreground with a 10
  minute call timeout - `STACK_READY`, 75 tables, login 2106 /
  game 7777 / db 3306 listening; `tools/install_dev_tools.sh` green
  (task, golangci-lint, gci, gofumpt); `go build ./...` green.
- Commit "docs: the foreground execution rule for the agent shells":
  the rule landed in AGENTS.md (the mandatory first step section),
  the "Foreground execution is mandatory (the background deploy
  trap)" section in `docs/deployment.md` (with the pointer from the
  mandatory first step, the four bullet rules and the cross link to
  the sandbox signal pitfall), the same rule condensed in the
  `mobius-stack` and `go-verify-loop` skills, and the restricted
  sandbox shells operational note now references the section.
- Status: done (2026-09-11). A pure documentation change - no code
  touched, `go build ./...` is the only gate it needs and it passed
  before the edits.

## Active task: the frozen trip plan - the shop manager stops re-planning (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user reported racing purchases on the 36954cf build (the
2026-09-11 10:30 test1 dump, level 14, adena 1800): the shopping trip
ended holding TWO pairs of gloves - the new Leather Gloves worn, the
old Gloves left in the bag - and required the manager to know exactly
what and how much ONE trip buys and to wait for the bot's execution
in place, instead of recalculating the plan at every step.

### Root cause

The trip planned TWICE against two different states. The sell first
step (`replacementTargets`) ran the plan computed at the trip start
(71420 adena, everything worn: Brandish 62214 with the Short Sword's
sell credit plus Low Boots 7785 with the Apprentice's Shoes credit -
exactly the dump's "worth 69999") and sold the displaced sword and
shoes. The stop planning (`planShoppingStops`) then computed a FRESH
plan against the freed slots and the fresh adena (71807): the armor
floor pulled the cheapest Apprentice's Shoes (8 adena) into the
emptied feet slot - blocking the planned Low Boots upgrade, the sold
piece bought right back - and the defense phase planned the Leather
Gloves (7785) whose displaced Gloves were never queued for the sale
(the replacement phase had already run), so the buy swapped the old
gloves into the bag. The dump's numbers match the reconstruction to
the adena: 71420 + 387 of the sales - 70007 of the actual buys
(Apprentice's Shoes 8 + Leather Gloves 7785 + Brandish 62214) = the
1800 the dump ends with. The same drift class produced the earlier
sold-weapon re-buy round (build f3b868e).

### Design

- `Loop.tripPlan` freezes `shoppingPlan()` ONCE in
  `maybeStartTownTrip`, before the weapon merchant routing; the trip
  trigger reason names the frozen plan's worth.
- `replacementTargets` and `planShoppingStops` read the frozen plan;
  no `shoppingPlan()` call happens after the trip starts.
  `weaponStopMerchant` routes by the frozen plan too.
- The buy execution keeps its arrival confirmations (the manager
  waits for every request's inventory answer) and gains one last
  responsible moment guard: `dropOwnedPurchases` removes the stop
  purchases whose item id the inventory already carries before any
  request goes out (a mid trip drop the auto equipment wore, a manual
  user purchase - a second copy is never part of the plan).
- The trip end (and the death reset) clears the frozen plan; the
  next trip freezes a fresh one against the gear the purchases
  reached.
- `enterSellPhase` logs the stop honestly: the buy stops of the plan
  no longer claim "selling the junk" on every arrival.

### Acceptance criteria

- `TestTripPlanFreezesPurchasesAgainstResale` replays the dump: the
  frozen plan is [Brandish + Low Boots] worth 69999, the replacements
  are [sword, shoes], the distributed stops carry exactly the frozen
  plan (no shoes re-buy, no purchase without its queued sale) and the
  merged stop buys the Brandish from Unoren.
- `TestStopShoppingSkipsOwnedItems` pins the owned purchase filter.
- The widget queue tests and the trip flow tests stay green;
  go build ./..., go vet ./..., the full go test suite, gofmt -l and
  golangci-lint (the hunt package) stay clean.

### Progress

- 2026-09-11, commit "shop: the trip freezes its purchase plan and
  executes it verbatim": the `Loop.tripPlan` field (frozen once in
  `maybeStartTownTrip` before the weapon routing), the frozen reads
  in `replacementTargets` / `planShoppingStops` /
  `weaponStopMerchant`, the `dropOwnedPurchases` guard of
  `tickStopShopping`, the honest per-stop log of `enterSellPhase`,
  the trip reason naming the frozen plan's worth, the trip end and
  death reset clearing the plan. Tests:
  `TestTripPlanFreezesPurchasesAgainstResale` replays the dump trip
  end to end (the frozen plan worth 69999, the sword and the shoes
  sold in the plan order, the stops carrying exactly the frozen
  plan, the Brandish bought from Unoren);
  `TestStopShoppingSkipsOwnedItems` pins the owned purchase filter;
  `TestPlanShoppingStopsMergesCurrentMerchant` and
  `TestReplacementSalesSellBeforeBuy` freeze the plan explicitly
  now. Verified: go build ./..., go vet ./..., the full go test
  suite green (19 packages), gofmt -l clean, golangci-lint 0 issues
  on the hunt package.

### Status: done (2026-09-11)

- The shop manager plans once per trip: the plan the trigger armed
  with is the plan the sell first step banks and the buy stops
  execute - nothing re-plans in between, every request waits for its
  inventory confirmation (the manager waits for the bot's
  implementation on the spot).
- The two pairs of gloves round cannot recur: the sold piece is
  never re-bought (the stops carry the frozen plan) and no purchase
  appears without its queued sale (the SellFirst union of the frozen
  plan is exactly what the replacement step sells); a second copy of
  an owned item is filtered out at the execution point.
- The dump numbers are pinned by the regression test: the frozen
  plan worth 69999 ([Brandish 62214 + Low Boots 7785], the
  replacements [sword, shoes]) against the reconstructed 71420
  adena state.
- All the checks green: go build ./..., go vet ./..., the full go
  test suite (19 packages), gofmt -l, golangci-lint (0 issues on the
  hunt package; the two pre-existing findings of the acceptance
  package predate this task).

## Active task: the port of the shop planner top-tier guard from feature/acceptance (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user asked to port the purchase queue fix from
`feature/acceptance` (commits 9306eee..dd9433f) to this branch: the
bot that sold its replaced weapon re-planned against the fresh adena
and the empty weapon slot and bought the same 1k Short Sword back
(the reported dump, build f3b868e, 09:05-09:06) instead of the top
affordable tier of the weapon ladder. The acceptance fix commits do
not cherry-pick: this branch rewrote the planner into the phased
strategy (the armor floor, the weapon milestone, the jewel floor,
the defense upgrades, the widget purchase queue), so the guard is
re-implemented inside its `bestPurchase` walk.

### Root cause on this branch

The weapon phase of `shopStrategy.classify` aims at
`view.target = bestWeaponValue` - the best `gain / price` among ALL
strict weapon upgrades, affordable or not. Over an EMPTY weapon slot
the 883 adena Short Sword owns that target (3.43 vs 0.30 of the
Knife): after the sell-first sale the re-plan bought the sold sword
right back. Over a WORN weapon the same ranking aims at cheap rungs
while the wallet already covers the 60k tier.

### Port design

- `bestPurchase` walks three passes: the viability pass collects the
  candidates that pass the planned / gain / slot / affordability
  gates and applies the top-tier guard (the `ladderTop` map records
  the best viable gain per paperdoll slot in the score descending
  scan order; a candidate aspired above the record on an overlapping
  slot is dropped - the same semantics the acceptance fix pinned);
  the view pass computes the weapon target from the SURVIVORS only;
  the classification pass runs the unchanged `classify` matrix and
  `phaseBeats` over the survivors.
- The guard persists across the pick rounds of one plan: the eroding
  budget of the later rounds must not crowd the top tier out and
  push a cheaper rung in - the slot stays unpurchased and waits for
  the next trip.
- The floor offers bypass the guard: the armor and jewel floors
  deliberately buy the cheapest offers of the empty families (the
  opening outfit rule of the strategy, see `classify`), the guard
  governs the upgrade phases only.
- The tail and wishlist modes of the widget purchase queue run
  without the guard (`ladderTop` is dropped at the mode flips): the
  unbounded budget would collapse the wanted ladder to its top tier
  and the widget would hide the milestones the bot saves for.

### Acceptance criteria

- The three regression scenarios of the acceptance fix pass here:
  the post-sale empty slot plans the top affordable tier, the
  replaced weapon plans the top tier with the SellFirst sale, and
  the intermediate never slides in behind the eroding budget.
- `TestPlanPurchaseQueueMatchesPlainPlan` stays green (the
  affordable prefix stays byte identical to the plain plan).
- The journey rules of `TestShoppingStrategyJourneyComparison` stay
  green; the IS table of `docs/shopping_strategy.md` is regenerated.
- `go build ./...`, `go vet ./...`, the gear tests,
  `golangci-lint run` and `gofmt -l` are clean.

### Progress

- 2026-09-11, commit "shop: the planner buys the top affordable tier
  of an upgrade slot, never an intermediate rung": `bestPurchase`
  walks three passes now - the viability pass (`viableCandidates`
  with the top-tier guard: the `ladderTop` map records the best
  viable gain per slot in the score descending scan order,
  `aspiredAbove` drops the rungs below the record; the floor offers
  bypass through `floorOffer`), the view pass (the weapon target
  computed from the survivors only) and the classification pass
  (the unchanged `classify` matrix and `phaseBeats`). The tail and
  wishlist modes drop `ladderTop` at their flips, so the widget
  queue keeps its wanted ladder. Tests: the three acceptance
  regression scenarios ported (`TestPlanPurchasesTopTierAfterSaleReplan`,
  `TestPlanPurchasesReplacedWeaponTargetsTopTier`,
  `TestPlanPurchasesIntermediateNeverFitsUnderTopTier`),
  `OneWeaponPerTrip` and `SkipsInventoryItems` pin the Long Sword
  top tier now, `CreditsDisplacedGear` pins the brandish through the
  sickle credit. Verified: go build, go vet, the full go test suite
  green (16 packages), gofmt clean; the journey comparison shows the
  new ladder (the chisel at 8, the knife at 9 through the sell-first
  credit, the sickle at 10, the brandish at 13, the Long Sword at
  16 - the same top gear, the save up trips gone).
- 2026-09-11, commit "docs: the shop strategy documents the
  top-tier slot guard": the Rule 1 weapon milestone wording (the top
  affordable tier, the guard semantics, the ladder with the chisel
  rung), the supporting rules bullet (the guard, its persistence,
  the floor and widget-tail bypasses), the regenerated IS journey
  table (the chisel at 8, the knife at 9 through the sell-first
  credit, the sickle at 10, the brandish at 13, the Long Sword at
  16 - the save up trips gone) and the shopping.go bullet of the
  piece map. Verified: `golangci-lint run` - 0 issues on the gear
  package (the only --new-from-rev findings on the tree live in the
  untracked reprodump diagnostic, which never ships), gofmt clean.

### Status: done (2026-09-11)

- The top-tier guard ported from feature/acceptance: the empty
  weapon slot plans the top affordable tier (the knife at the dump's
  post-sale wallet, the brandish when the wallet reaches it), the
  replaced weapon plans the top tier with the SellFirst sale, and
  the intermediate rungs never slide in behind the eroding budget of
  the later pick rounds.
- The widget purchase queue keeps its wanted ladder (the tail and
  wishlist modes walk without the guard) and the affordable prefix
  stays byte identical to the plain plan.
- All the checks green: go build ./..., go vet ./..., the full go
  test suite (16 packages), gofmt -l, golangci-lint run (0 issues on
  the gear package).

## Active task: the round 56 building stuck - the skip gate and the re-planned self-click (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian, the 06:19 state dump, build
896865d, bot test2, phase townWalk): the bot gets stuck trying to
enter the elven village trainer hall building on the teach walk to
Ellenia - explain how the building is represented in the geodata
(the user suspected the roof and floor z coordinates), find why an
impassable path to the NPC gets planned, fix it and prove with tests
that the bot now reliably reaches this NPC from different positions
in the town.

### Root cause (probed against the real geodata pack and the live stack)

The trainer hall cells carry three layers (the sloped roof
-2600..-2448, the walkable floor -2792 with the walls in the NSWE
flags, the water deck -3928) and the interior east of the aisle has no
floor layer at all. The dump's aisle route is walkable - the 48 unit
south leg validates in full and the live server walks every leg of it
(proven with raw clicks and a full live town trip). The failure was
in the follower recovery, not the plan:

1. The blind stuck skip armed the east hall waypoint whose click the
   server collapses onto the first step (the diagonal flank carries
   the building's north wall); the partial clicks crept the
   character 16 units at a time into the dead-end pocket cell
   (44776 51992) and from the pocket the click was refused wholesale
   - the dump's "the server would refuse the walk click" line.
2. After a stuck re-path the follower clicked the fresh plan's wp 0 -
   the standing cell itself - in the same tick; the server always
   refuses a self-click, so the recovery burned a second re-path on
   the guaranteed refusal (caught by the round 56 reproduction).

### Fix

1. `hunt/town.go`: the stuck skip only jumps onto a waypoint with a
   walkable line from the standing cell (`nextClearWaypoint` scans
   the plan through the `legAdvanceClear` gate); with no reachable
   successor the leg re-plans at once.
2. `hunt/town.go`: `followWaypoints` re-runs the cursor advance after
   the stuck handling, so a re-planned leg never clicks its own
   standing-cell wp 0.

### Acceptance criteria

- The exact dump walk (the aisle entrance to Ellenia) arrives with
  zero refused clicks and zero re-paths.
- A frozen aisle (the clicks silenced) recovers through exactly one
  re-path, the character never creeps east of the aisle entrance and
  no click is ever refused.
- The dry approach search from seven village positions (the aisle,
  the pocket, the north terrace, the south approach, the east plaza,
  the shop deck, the southwest shore path) all reach Ellenia with
  every leg fully validated against the ported server rules.

### Status: done (2026-09-11)

- Commit "hunt: the round 56 building stuck - the skip gate and the
  re-planned self-click": (1) the gated skip (`nextClearWaypoint`) and
  the post-stuck cursor advance (town.go); (2) tests:
  `hunt/round56_repro_test.go` (the aisle walk, the frozen-aisle
  recovery), `hunt/walk_stuck_skip_test.go` (the clear-line gate, the
  forward scan), `pathfind/teacher_aisle_test.go` (the seven-position
  Ellenia reach, the dump plan pin, the pocket refusal geometry);
  (3) docs: hunting.md (the follower paragraph), development_log.md
  Round 56, agent_progress.md this entry.
- Verify loop: go build, go vet, the full go test suite green,
  gofmt clean, golangci-lint --new zero findings.
- Live validation on the local stack: the full dump scenario (the
  east hunting zone, level 11, 934 SP, Power Strike wiped, the book
  unbought) ran the complete trip - the junk sale, the book purchase
  at Creamees, the walk to Ellenia - and all the lessons landed; the
  surgical raw clicks of the aisle legs all walked on the live
  server (44728 51992 -> 44728 52200 -> 45160 52120 -> 45725 52105).

## Active task: the round 53 town walk stuck - the shared re-path budget and the slow skip recovery (2026-09-11)

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian): the bot is stuck again after the
round 52 fix. The state dump (build f1c3136, bot test2, level 11,
phase townReturn, dumped 2026-09-11T05:06:14+03:00) shows the character
frozen at (46008, 51992, -2792) - the elven village teacher plaza -
cycling "town walk stuck, skipping waypoint (1 of 3)" -> "the server
would refuse the walk click to 45304 52152, re-pathing (2 of 3)" ->
"town walk stuck, skipping waypoint (3 of 3)" -> "town trip ended:
aborted, the server refuses every walk click" TWICE within 90 seconds.
The user asked to clarify why the bot is stuck, check the pathfinding,
write tests and ensure it is fixed.

### Root cause analysis

The round 52 click validation port (Engine.ValidateClick) correctly
mirrors the Mobius GeoEngine.getValidLocation: the offline reproduction
test TestReproRound53ZoneReturnWalksThePlan walks the exact dump
position to the hunting zone in 13 validated clicks with zero refused
clicks and zero re-paths. The pathfinder's plan is sound and every
click the follower sends would survive the server validation.

The freeze is NOT in the click validation. It is in the RECOVERY
BUDGET. The dump's event sequence shows the bot alternating between
walkStuck (skip waypoint, increment rePaths) and clickServerValidated
(re-path, increment rePaths). Both shared the same maxRePaths=3 budget.
The sequence consumed the budget in 2 cycles (40 seconds) and aborted.

Two compounding design flaws:

1. The waypoint skip (a cursor advance, no navigator call) consumed the
   same budget as the full leg re-plan (startWalkLeg, an A* search).
   The skip is cheap and should be retried freely; the re-plan is
   expensive and should be bounded.
2. The stuck timeout was 15 seconds for EVERY stuck. Once the walker
   knew the server refused its clicks, waiting 15 seconds for every
   subsequent waypoint just burned the trip's time budget.

### Fix

1. walkStuck: the waypoint skip no longer consumes the re-path budget.
   Only the full leg re-plan (startWalkLeg) and the water escape
   re-plan consume it.
2. walkStuck: after the first skip, the stuck timeout drops from 15s
   (stuckTimeout) to 4s (stuckFastTimeout). The stuckFast flag arms on
   the first skip and clears on startWalkLeg and the other full
   resets.
3. walkStuck split into walkStuck + stuckWaterEscape + stuckTownWalk to
   stay under the funlen limit.

### Status: done (2026-09-11)

- Commit "hunt: the round 53 stuck budget - the skip does not consume
  the re-path budget, the fast timeout cuts the recovery window".
- Verify loop: go build, go vet, the full go test suite (18 packages
  green), gofmt clean, golangci-lint zero new findings in the touched
  files.

## Active task: the webui polish pass - proxy accent, full bot name, instant skills, bright path

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian) asked for a batch of small
webui adjustments:

- Remove the "proxy" word from the left panel bot row; mark the
  proxy target bot with brighter accents (corner brackets around the
  whole bot plaque).
- Always show the full bot character nickname in the left panel (no
  truncation).
- Remove the green vertical stripe left of the "hunting" activity
  banner.
- Move the "dump state" button from the bot HUD (left of the map) up
  to the map toolbar next to follow / view.
- In the view menu, make the paths layer always active by default.
- Brighten the path color (the walk plan line and the destination
  marker) - the previous light blue blended with the self marker;
  use a bright non-blue color and label every waypoint with its
  coordinates.
- Move the effects (buffs) panel to the right of the character HUD
  (top left of the map), decoupled from the equipment widget.
- Make the ACTIVE / PASSIVE skills tab switch instant: previously the
  grid rebuild waited for the next snapshot, so the tab highlight
  landed a tick before the cell list.

### Status: in progress (2026-09-11)

- Edit 1: dropped the `chip-proxy` text chip from `renderBotList`,
  added the `is-proxy` class on the `bot-item` and four bright
  corner accents through CSS pseudo-elements (`style.css` +
  `app.js`).
- Edit 2: removed the `text-overflow: ellipsis` truncation of
  `.bot-item .bot-name`, allowed the name to wrap so the full
  nickname always shows.
- Edit 3: removed the green left border of `.bot-activity.kind-hunt`
  (set `border-left-color: transparent`).
- Edit 4: moved the `hud-dump` button markup from `.hud-name-row`
  into `.map-toolbar` next to the follow checkbox, updated the
  button CSS to fit the toolbar height (24px, bg-panel-2 surface).
- Edit 5: flipped `<input id="show-dest">` to `checked` by default
  so paths always render on first load.
- Edit 6: changed `mapColors.userPath` and `userMark` from
  `#4da3ff` (light blue) to `#ff44cc` (bright magenta) - distinct
  from the blue self marker and the red combat path. Added waypoint
  dots and coordinate labels `(x, y)` at every waypoint in
  `drawWalkPlan` with a dark stroke for readability over any
  background.
- Edit 7: moved `.buffs-panel` from `right: 278px` (just left of
  `gear-panel`, top right) to `left: 272px` (just right of the
  character HUD, top left), decoupling it from the equipment widget.
- Edit 8: instant skills tab switch - added `renderSkillsNow()` and
  called it from `setGearMode` and `setSkillFilter` so the grid
  rebuilds on the same frame as the tab highlight. Reworked
  `renderSkills` so the keyed cell cache (`SkillCells.cells`) keeps
  cells for both the active and the passive skills: the opposite
  filter's cells stay in the Map with their loaded icons and only
  detach from the grid DOM, so a switch back is fully instant (no
  new icon fetches).
- Updated `tools/repro_gear.js` to match the new buffs panel
  placement (`left: 272px` instead of `right: 278px`).
- Verify loop (no Go toolchain in this sandbox): all 7 repro
  harnesses pass (`repro_gear`, `repro_hud`, `repro_map_render`,
  `repro_movement`, `repro_bot_switch`, `repro_zone_hover`,
  `repro_fight_ui`). `node --check` confirms app.js and map.js are
  syntactically clean.

### Next

- Push this batch as one atomic commit on `feature/proxy-server`.
- A future round should run the live stack (`tools/mobius_e2e.sh`)
  and a real bot snapshot to confirm the new magenta path color
  reads over the actual elven map imagery.

## Active task: the zone return stuck - the town trip blocks out, the non-dry fallback routes home

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The 2026-09-11 06:00 user follow-up dump: the bot test3 (build
c1faefb, phase engage) stood at 43000 50184 -2992 (near Herbiel,
outside the zone) for 3 minutes. Events: "no dry path to 38553 50080,
the walk would swim" repeated, then a learning trip started and also
failed with "no dry path to 42766 50037" (Herbiel is only 276 units
away). The bot never moved.

### Root cause

Two bugs composed:

1. The town trip started while the bot was outside the zone.
   `handleTownTrip` runs before `engage()` in the tick, so
   `maybeStartTownTrip` fired and started a learning trip before the
   zone return had a chance. The learning trip failed, armed the 5
   minute cooldown, and the zone return then also failed.

2. The zone return had no non-dry fallback. `returnToZone` called
   `startWalkLeg` (dry search only). When the dry search failed, it
   fell back to `walkZoneLeg` (direct walks) which crossed water and
   walls, so the server refused and the bot stood still.

A geodata probe confirmed that `FindPathApproachDry` from the bot's
position to both Herbiel and the zone center SUCCEEDS in the offline
probe. The runtime failure reason is unresolved, but the non-dry
fallback gives the bot a route regardless.

### Fix

1. The town trip is blocked while the bot is outside the zone:
   `maybeStartTownTrip` checks `l.zone() != nil && !l.inZoneSelf()`
   and returns early. The weapon run is the sole exception.
2. The zone return tries the non-dry search as a fallback: the new
   `startZoneReturnLeg` runs the dry search first, then the non-dry
   search. The click guard refuses water legs and re-paths, so a
   non-dry plan is safe to walk.
3. `startWalkLeg` is split into `startWalkLeg` (dry only, town trips),
   `startZoneReturnLeg` (dry + non-dry, zone return), and the shared
   `startWalkLegSearch` core.

### Acceptance criteria

- A bot outside the zone with a learning budget does NOT start a town
  trip (the zone return is armed instead).
- A bot outside the zone with no weapon starts the weapon run (the
  exception).
- When the dry search fails for the zone return, the non-dry search
  runs as a fallback.
- When both searches fail, the direct walk fallback fires.
- The existing town trip tests stay green.

### Progress (2026-09-11)

- The geodata probe reproduced the scenario: `FindPathApproachDry`
  from 43000 50184 -2992 to both Herbiel (276 units) and the zone
  center (4447 units) SUCCEEDS in the offline probe. The runtime
  failure is unresolved.
- Commit "hunt: the zone return tries the non-dry fallback, the town
  trip blocks out outside the zone": (1) `maybeStartTownTrip` zone
  gate (town.go); (2) `startZoneReturnLeg` + `startWalkLegSearch`
  refactor (town.go); (3) `returnToZone` uses `startZoneReturnLeg`
  (loop_movement.go); (4) tests: `zone_return_stuck_test.go` (the
  zone gate, the weapon run exception, the non-dry fallback, the
  both-fail fallback, the search order); (5) docs: development_log.md
  Round 55, agent_progress.md this entry.
- Verify loop: go build, go vet, the full go test suite (19 packages
  green), gofmt clean, golangci-lint zero new findings.

### Status: done (2026-09-11)

- The fix pushed: the zone gate, the non-dry fallback, the tests, the
  docs.
- The user-side check: watch a bot respawn at the village (outside
  the zone) after an emergency logout - it returns to the zone first
  (no learning trip interference), and the zone return finds a route
  even when the dry search fails.

## Active task: the offset ring stuck - the talk click fires within the server interaction distance

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The 2026-09-11 05:45 user follow-up dump: after the roof teleport fix
of round 53, the bot still stuck. The dump (build 86b4c86, bot test1,
phase townSell) showed the character at 44616 52536 -2832 (dist 244
from Cobendell at 44823 52414 -2792, dz 40), target=self, stuck for
31 seconds after "learn: teacher Cobendell found, walking to it".

### Root cause (probed against the real geodata pack)

The roof teleport fix (the npc approach offset point) closed the roof
teleport, but the talk click then waited for dist3D <= 200 (the
approach gate) while the bot stood at dist3D 244 (within the server
250 interaction gate but above the 200 approach gate, because of the
z gap between the approach deck at -2832 and the trainer hall floor
at -2792). The bot looped on the offset ring forever.

The approach gate (200) was a planning heuristic (where to aim the
walk), but the talk click gate must match the server
INTERACTION_DISTANCE (250) - the server accepts the ClickObject
action and the transactions within 250 in 3D, regardless of the
approach gate.

### Fix

The talk click (and the merchant select) fire as soon as the bot is
within the server interaction distance (npcInteractionDist = 250 in
3D), even when the z gap keeps dist3D above the approach gate (200).

- `hunt/town.go`: `npcInteractionDist = 250.0` mirrors the server
  INTERACTION_DISTANCE. `approachMerchant` checks it first; the new
  `selectMerchant` helper handles the paced selection. The far walk
  only fires when dist3D > 250.
- `hunt/learning.go`: `approachTeacher` checks `npcInteractionDist`
  first; the new `clickTeacher` helper handles the paced talk click.
  The far walk only fires when dist3D > 250.
- The deck hop case (dist2D <= 200, dist3D > 200) is unchanged: the
  offset collapses, the deck window bounds the wait. The new early
  return takes over before the deck hop branch when dist3D <= 250.

### Acceptance criteria

- A bot at the dump position (dist3D 244 from the teacher) clicks the
  teacher directly - no ground walk, the talk click fires.
- A bot in the deck hop case with dist3D in (200, 250] also clicks
  the teacher (the deck hop window no longer fires for a small z gap).
- The existing offset tests stay green: the far walk still clicks the
  offset point when dist3D > 250.

### Progress (2026-09-11)

- The geodata probe reproduced the dump scenario: FindPathApproachDry
  from 44616 52536 -2832 to Cobendell with radius 200 returns 2
  waypoints, the bot is already within waypointArriveDist of the
  last, walkTownWaypoints returns true at once, approachTeacher fires
  with dist3D 244.
- Commit "hunt: the talk click fires within the server interaction
  distance": (1) `npcInteractionDist = 250.0` (town.go); (2)
  `approachMerchant` early return + `selectMerchant` helper
  (town.go); (3) `approachTeacher` early return + `clickTeacher`
  helper (learning.go); (4) tests: `npc_approach_test.go` updated
  (the deck hop test now pins the talk click, the new dump scenario
  test pins the exact position); (5) docs: development_log.md
  Round 54, agent_progress.md this entry.
- Verify loop: go build, go vet, the full go test suite (19 packages
  green), gofmt clean, golangci-lint zero new findings.

### Status: done (2026-09-11)

- The fix pushed: the npcInteractionDist early return, the
  selectMerchant / clickTeacher helpers, the tests, the docs.
- The user-side check: watch a bot at the dump position click the
  teacher directly (no "town walk stuck", no 30 s freeze), the
  lessons land.

## Active task: the roof teleport - the npc approach clicks the offset, not the exact cell

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian): the bots run somewhere behind
the building trying to talk to the village teachers (Cobendell et
al.), and when the character approaches the npc the server teleports
it onto the roof of the building instead of letting it enter inside.
The user asked to study where Cobendell and the similar npcs stand
and to make the bot approach them at a short distance, not talk
through the wall.

### Root cause (probed against the real geodata pack)

The Cobendell cell (44823 52414) carries two layers: the ground
floor at z -2792 (where the npc stands) and the roof at z -2448. The
server's `getValidLocation` (ported as `Engine.ValidateClick` in
round 52) walks a Bresenham cell line from the click origin to the
click target. When the click targets Cobendell's exact cell from the
south or west, the line crosses the building wall, the height-step
fallback resolves the target onto the roof layer, and the bot ends up
on the roof. The probe measured it: clicking on Cobendell from deg
30 (south-east) redirects to z -2576, from deg 180 (west) to z -2456
- all roof heights.

The bot's `approachTeacher` and `approachMerchant` clicked the npc's
EXACT spawn cell when the bot was far (dist3D > 200). The pathfinder
had already planned a route to within 200 units of the npc, but the
final approach leg clicked the exact cell - and the server
teleported the bot onto the roof.

### Fix

The approach walk clicks the npc approach point, not the npc's exact
cell. The approach point is `npcApproachOffset` (150) units from the
npc toward the bot, so the Bresenham click line stays outside the
building walls and the server validates it on the ground floor. The
150 unit offset keeps the bot within the 250 unit server interaction
distance (the talk click that follows works) but outside the walled
interior.

- `hunt/town.go`: `npcApproachOffset = 150.0` and `npcApproachPoint`
  compute the offset target. `approachMerchant` uses it for both the
  far walk and the deck hop case. The deck hop case skips the click
  when the offset collapses onto the bot's own cell (the
  `hopCoincideDist` gate).
- `hunt/learning.go`: `approachTeacher` uses the same offset for both
  the far walk and the deck hop case, with the same skip gate.

### Acceptance criteria

- A bot far from a town npc clicks the offset point (150 units from
  the npc toward the bot), never the npc's exact spawn cell.
- A bot in the deck hop case (2D close, z far) does NOT click the
  npc's exact cell - the offset collapses and the skip gate fires.
- The existing town trip, learning and merchant tests stay green.

### Progress (2026-09-11)

- The environment deployed (the Go toolchain installed at
  /home/z/my-project/goroot, the geodata pack at data/geodata with
  165 regions).
- The geodata probe (scripts/probe_cobendell, since deleted) measured
  the roof teleport mechanism: the Cobendell cell has the ground
  floor at z -2792 and the roof at z -2448; clicking on the exact
  cell from the south/west redirects to the roof (z -2456..-2576);
  the pathfinder reaches the npc from the north and east; the offset
  click (150 units toward the bot) validates on the ground floor.
- Commit "hunt: the npc approach clicks the offset, not the exact
  cell": (1) `npcApproachOffset` and `npcApproachPoint` (town.go);
  (2) `approachMerchant` uses the offset for both the far walk and
  the deck hop case (town.go); (3) `approachTeacher` uses the same
  offset (learning.go); (4) tests: `npc_approach_test.go` (the
  offset geometry, the collapse onto the bot, the teacher click, the
  merchant click, the deck hop skip); (5) docs: development_log.md
  Round 53, agent_progress.md this entry.
- Verify loop: go build, go vet, the full go test suite (19 packages
  green), gofmt clean, golangci-lint zero new findings in the touched
  files.

### Status: done (2026-09-11)

- The fix pushed: the offset approach point, the deck hop skip gate,
  the tests, the docs (Round 53).
- The user-side check stays the project workflow: watch a bot
  approach a village teacher (Cobendell, Ellenia) and click the
  offset point (the log line "the teacher stands on another deck"
  stays for the deck hop case, but the click no longer targets the
  exact cell), then click the teacher object within the interaction
  distance - no roof teleport, the lessons land.

### Followup: the proxy corner brackets (2026-09-11)

The user reported the four corner accents did not form a rectangle
but sat at scattered positions. Root cause: the original CSS drew
two corners on `.bot-item::before/::after` (top-left + bottom-right
of the whole item) and two corners on `.bot-row::before/::after`
(top-right + bottom-left of just the bot-row, which is only the
first row of the item). The `.bot-row::after` "bottom-left" corner
landed at the bottom of the first row instead of the bottom of the
whole plaque, so the four corners did not align on one box.

Fix: replaced all four rules with a single `.bot-item.is-proxy::before`
that fills the whole item (`inset: 0`) and draws the four L-corner
brackets through eight `linear-gradient` stripes, each positioned
relative to that same box - top-left, top-right, bottom-left,
bottom-right. Two stripes per corner (a horizontal 12x2 and a
vertical 2x12), all on the same element, guarantee a clean
rectangle regardless of how many activity or vital bars the row
carries underneath.

Verify loop: `repro_gear` and `repro_hud` PASS (they parse
`style.css`); the change is CSS only.

### Followup: the waypoint labels do not collide with the bot name (2026-09-11)

The user reported the waypoint coordinate labels `(x, y)` drawn on
the map by `drawWalkPlan` were hard to read AND collided with the
bot name label drawn at the character position by `drawLabels`
(especially the first waypoint, which sits right next to the
character). The user also asked to make the left-panel proxy
corner brackets brighter and not touch the bot name.

Fix 1 (map / `map.js`): the `drawWalkPlan` label loop now computes
the character's screen position once and skips the coordinate
label (but still draws the waypoint dot) for any waypoint within
`labelSkipPx = 30` pixels of the character - the bot name label
drawn by `drawLabels` at the character position no longer overlaps
the coordinate text. The remaining labels alternate above-right and
below-right offsets (`labelIndex % 2`) so adjacent waypoints do not
stack on each other either. The font is bumped from 10px to 11px,
the dark outline from 3px to 3.5px at alpha 0.9, so the text reads
on both the light map imagery and the dark fill.

Fix 2 (left panel / `style.css`): the `.bot-item.is-proxy::before`
corner brackets now use a bright gold `#ffb800` (distinct from
every other UI accent), 3px thick stripes (was 2px) and 16px long
(was 12px) - clearly visible on both themes. The pseudo-element
sits at `inset: 2px` instead of `inset: 0`, pulling the brackets
2px inside the bot-item edges so they never touch the wrapped
second line of the bot-name even when the item is short (offline
bot, no activity banner, no mini bars).

Verify loop: `repro_gear`, `repro_hud`, `repro_map_render` PASS;
`node --check map.js` clean.

## Active task: the town walk stuck loop - the reverse wall check and the waypoint skip

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian): the bots are stuck and not
learning. The state dump (build 36bfe99) shows the character test1
(level 13, 25023 SP) at the elven village teacher plaza
(44440 52552 -2832), cycling "town walk stuck, re-pathing (1 of 3)"
-> "town trip ended: aborted, walk stuck" and restarting. The learning
trip planned 25 lessons worth 6570 SP at the teacher Cobendell, walked
to the trader Herbiel first and stuck on the first leg. The user also
asked to verify which NPCs are the skill teachers and to check the
pathfinding.

### Root cause

The pathfinder's `wallsOpen` checked only the SOURCE cell's wall in
the step direction. The Mobius `MoveToLocation` handler checks the
TARGET cell too (`isCompletelyBlocked` rejects any target whose walls
are all closed, the movement validation checks the reverse wall). A
path that stepped onto a cell whose reverse wall was closed was a path
the server refused to walk - the character stood still, the stuck timer
fired, and the deterministic re-path planned the identical route.

### Fix

- pathfind: `wallsOpen` now checks the TARGET cell's reverse wall too.
  `Layer.IsCompletelyBlocked` documents the server's check.
- hunt: `walkStuck` first SKIPS the current waypoint before re-planning
  the whole leg. The skip breaks the deterministic re-path loop: the
  next waypoint may be reachable through a different cell.
- The skill teacher data was verified against the Mobius C1
  SkillLearn.xml: both Ellenia and Cobendell teach the elven fighter,
  the bot's nearest teacher selection is correct.

### Status: done (2026-09-11)

- Commit "pathfind: the reverse wall check and the waypoint skip":
  (1) pathfind/search.go `wallsOpen` checks the target cell's reverse
  wall; (2) pathfind/layer.go `IsCompletelyBlocked` helper; (3)
  hunt/town.go `walkStuck` skips the current waypoint before re-planning
  the leg; (4) tests: pathfind/reverse_wall_test.go, hunt/
  walk_stuck_skip_test.go, hunt/skill_teacher_test.go; (5) docs:
  development_log.md Round 49, agent_progress.md this entry.
- Verify loop: go build, go vet, the full go test suite (16 packages
  green), gofmt clean, golangci-lint zero new findings in the touched
  files.
- The real pack probe: the dry search from the dump stuck spot to
  Herbiel now plans a 9 waypoint route of length 3258 (was 3451) that
  avoids the plaza detour that triggered the stuck loop.

## Active task: the bare-handed bot - the weapon run owns the town trips

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian): the bot never may fight with
bare hands - buying a weapon is the highest priority whenever no
weapon exists; plus the reason why it sold its weapon without buying
the replacement right away had to be found.

The state dump (build 36bfe99, bot test2, level 11, 14814 adena)
shows the character punching Kaboo Orc Grunts for 2 damage with an
empty right hand while its plan says "the shop strategy plans
purchases worth 883 adena" (the Short Sword, the first weapon
milestone from an empty hand). The trip carrying that buy aborted on
the teacher walk: "town walk stuck, re-pathing (1..3 of 3)" ->
"town trip ended: aborted, walk stuck" - the weapon buy stop was
appended BEHIND the teach stop and never ran.

### Root cause

The sell-first replacement flow banks the worn weapon's
referencePrice/2 credit before the buy lands (stepReplacementSales),
and the buy stop of the weapon was the LAST leg of the trip: the sell
stop went to the NEAREST merchant (junk sells anywhere), the learning
stops rode behind it (the books, the teacher), and planShoppingStops
appended the buy groups by walking distance at the shop. Any failure
between the sale and the buy - the village stuck walks the dump
shows, an attacker interrupt (resetTownTrip drops the stops), a
merchant that never showed up, an exhausted buy retry budget - left
the character bare-fisted with the adena in the wallet. The next
trips re-planned the Short Sword but repeated the same stop order,
so the teacher leg abort kept eating the weapon buy. Nothing tied
the weapon sale to the weapon purchase, and nothing in the hunt loop
treated "no weapon" as the emergency it is.

### Fix

- gear.HasWeapon: the profile weapon probe over the whole inventory
  (equipped or bagged, the profile scoring decides what counts - a
  bow is no weapon for the melee fighter).
- The weapon leads every trip that buys one: the sell stop routes to
  the weapon purchase's merchant (the junk sells at any merchant, so
  the sell-first of the replaced weapon and the buy share ONE stop -
  the replacement lands immediately after the sale).
- The weapon run: a character with NO weapon and an affordable
  weapon in the plan runs the weapon errand alone - no teach stops,
  no books, a short retry cooldown (weaponRunCooldown 45s instead of
  the 5 minute tripCooldown) so an aborted run retries instead of
  punching mobs for five minutes.
- The bare-handed engage gate: a weaponless character with an
  affordable weapon never picks a fresh target (the weapon run owns
  the next ticks; the aggro self defense answer stays armed), and the
  zone entry engage of the return leg skips the same way.

### Acceptance criteria

- A weaponless bot with enough adena starts a town trip whose first
  (and only planned) stop is the weapon merchant, even with queued
  lessons waiting at the teacher.
- The weaponless engage gate holds fresh picks while the weapon run
  is pending; an attacker on the character still gets fought.
- A weapon upgrade trip routes its sell stop to the weapon merchant,
  so the displaced weapon sells and the replacement buys at one npc.
- An aborted weapon run retries after 45 seconds, not after 5
  minutes.

### Progress (2026-09-11)

- The environment deployed (STACK_READY, ports 2106/7777/3306, 75
  tables), the shopping/town/learning/equip code surveyed, the dump
  slot mask confusion resolved against the Mobius BodyPart enum
  (0x40 is head, 0x08 is neck - the dump table mislabeled them).
- Rebased onto the concurrent reverse wall round (c50e995): the two
  rounds compose - the wall check removes the planner side of the
  stuck walks my dump shows, the weapon run owns the trip priority
  side of the same story.

- Commit "hunt: the weapon run leads the town trips": (1)
  gear.HasWeapon (plan.go) probes the whole inventory for a profile
  usable weapon; (2) hunt/shopping.go grows the weapon probe
  (affordableWeaponPurchase, weaponStopMerchant,
  weaponlessRunWanted) and the weaponRunCooldown 45s; (3)
  maybeStartTownTrip routes the sell stop to the weapon purchase's
  merchant (the sell-first of the replaced weapon and the buy share
  one stop), a weapon run starts without the teach stops whatever the
  inventory and lesson queue say; (4) the bare-handed engage gate
  holds the fresh picks (the attacker self defense answer stays) and
  engagesOnZoneEntry skips the same way; (5) tripCooldownOver shortens
  to weaponRunCooldown while weaponless. Tests:
  gear/gear_test.go TestHasWeapon, hunt/weapon_run_test.go (the
  weapon stop routing, the learning skip, the held fresh picks with
  the armed attacker answer, the pick that proceeds when no weapon is
  affordable, the short cooldown, the upgrade stop routing, the zone
  entry hold).
- Commit "webserver: the dump slot names match the Mobius masks": the
  dumpSlotNames table of dump.go mirrored the Mobius BodyPart enum
  (verified against entity/item/enums/BodyPart.java) - the old table
  mislabeled 0x04/0x08/0x10/0x20/0x40/0x4000/0x8000 and missed the
  pair masks and legs/feet/back, so a healthy paperdoll read as
  corrupted (Cloth Cap [lfinger], Necklace of Magic [lear ear],
  Pants [part 0x800]). Test: webserver/dump_test.go
  TestDumpSlotNames.
- Verify loop: go build, go vet, the full go test suite (18 packages
  green), -race green on the hunt package, gofmt clean, gofumpt
  clean, golangci-lint zero new findings in the touched files (the
  pre-existing branch findings stay untouched).
- Live validation on the local stack: the dump state injected through
  the database (level 11, 14814 adena, the full armor floor, NO
  weapon, standing at the dump hunting spot 51558 50575). The bot
  held its target picks, ran the weapon errand at once ("no weapon in
  hand, the weapon run comes first, walking to the trader Unoren"),
  sold the junk at Unoren, bought the Short Sword two seconds later
  (list 3014700, 883 adena), equipped it into the empty right hand
  and walked back to the farm spot - the database holds the sword in
  PAPERDOLL slot 7 and the wallet at 14005. The SIGINT shutdown
  stayed graceful (exit 0).
- Commit "docs: the weapon rules of the shop strategy": hunting.md
  (the weapon-first paragraph of the town trips section),
  shopping_strategy.md (Rule 2a - the weapon outranks the trip
  itself, with the root cause story of the sold weapon),
  development_log.md Round 50.

### Status: done (2026-09-11)

- All three commits pushed: the round opener (a8b4788 after the
  rebase onto the concurrent reverse wall round), the weapon run fix
  (982bafd: gear.HasWeapon, the weapon stop routing, the weaponless
  engage gate, the short cooldown, the dump slot name fix, the
  tests) and the docs (hunting.md, shopping_strategy.md Rule 2a,
  development_log.md Round 50).
- The live stack validates the full weapon run end to end: the
  bare-handed character with 14814 adena walks straight to the weapon
  merchant Unoren, buys the Short Sword two seconds after the junk
  sale, equips it and resumes hunting - the exact dump scenario
  replayed with the opposite outcome.
- The user-side check stays the project workflow: watch a bot that
  lost its weapon (or a fresh one whose wallet crossed the cheapest
  weapon offer) drop its targets and walk for the sword at once (the
  "no weapon in hand, the weapon run comes first" log line), and
  watch a weapon upgrade trip sell the old weapon and buy the
  replacement at the same npc (no village walk between the sale and
  the buy).

## Active task: the teacher walk stuck - the bots never learn

Started: 2026-09-11. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-11, Russian): the bots freeze and never
learn ("боты застревают и не обучаются"). The state dump (build
36bfe99, bot test2, phase townWalk) shows the learning trip walking
to the teacher Ellenia, the follower passing waypoints 0..10 and
then standing frozen at 46152 51656 -2808 aiming at wp 11 until the
re-path budget aborts the trip ("town walk stuck" x3 -> "aborted,
walk stuck"). Also verify which points the skill buying actually
needs (the teacher, the spellbook merchants), check the pathfinding
and cover the fix with tests.

### Root cause (probed against the real geodata pack)

The deployment runs the server with PathFinding = 0: every ground
click is validated as a straight geodata line (getValidLocation),
and a click whose first step hits a closed wall resolves to the
character's own position - the move cancels silently. The follower
skipped the tight ramp waypoints of the trainer plaza approach
(16..48 units apart, all inside the 50 unit pass radius) from a
standing cell whose north AND west walls are closed (the plaza
railing pocket) and clicked the far plaza waypoint through the
wall: no movement, a deterministic re-plan reproduced the same
skip, the third budget burned and the trip aborted before the
teacher stop - no lesson ever landed, the bot looped
hunt -> near death -> emergency logout -> relogin -> the same trip
(a fresh Loop per session carries no trip cooldown).

The trip points themselves are correct (verified against the live
spawns and the buylists): Ellenia (45725 52105 -2792) and Cobendell
teach the elven fighter classes, Greenis/Esrandell the mystics; the
spellbooks the auras demand (1095/1294) sell at Creamees (42700
50057 -2984); Unoren sells the weapons (the dump's 883 adena plan
is the Short Sword), Ariel the armor. The dump's 6 lessons worth
1110 sp need no books - the walk was the only blocker.

The concurrent sessions of 2026-09-11 attacked the same report from
two more dumps: the reverse wall round (the planning level, the
route never steps onto a reverse walled cell) and the weapon run
round (the Short Sword purchase leads the trips). This round adds
the prevention level: the follower gate. The three compose.

### Fix

The follower gates every waypoint skip on the walkable line
(legAdvanceClear via Navigator.LineOfSight): the gated waypoint
stays the target until walking onto it re-opens the line; the skip
cursor moved into advanceWaypoints (the complexity limit).

### Status: done (2026-09-11)

- Commit "hunt: the waypoint skip needs a walkable line ahead":
  the follower gate (legAdvanceClear), the skip cursor extraction
  (advanceWaypoints), the fake navigator LOS default flip (sight ->
  blind with a per-line sightFunc override). Tests:
  pathfind/teacher_walk_test.go (the dump route, the pocket corner
  walls, the re-plan detour against the real pack) and
  hunt/teacher_walk_test.go (the exact dump state with the fake
  navigator - the first click goes to the ramp foot, never the
  walled plaza; the end-to-end recovery from the reported stuck
  spot under the simulated server). The gate test fails on the
  pre-fix code (wpIndex jumps to 11 - the dump signature). Docs:
  hunting.md follower paragraph, development_log.md Round 49.
- Rebase onto the concurrent rounds (74a38b9: the reverse wall
  check of the pathfinder, the stuck waypoint skip and the weapon
  run): the three fixes compose - the routes avoid reverse walled
  cells (planning), the follower only skips along walkable lines
  (prevention), a stuck walk skips its waypoint then re-plans
  (recovery). The teacher_walk tests adapted to the stricter engine
  rules (the plaza leg no longer collapses into one straight click,
  the route inserts verified steps instead); the whole suite and
  golangci-lint stay green (zero findings in the touched files).
- Live validation on the local stack (the exact dump scenario
  reproduced): test2 injected through the database as a level 11
  elven fighter with 1951 SP standing at the reported stuck pocket
  (46152 51656 -2808). The bot hunted, the learning trip planned
  "6 lessons worth 1110 sp wait at the teacher", sold the junk at
  Creamees, walked to the teacher Cobendell (no "town walk stuck"
  line in the whole session), found and clicked it and learned all
  six lessons - "learned Power Strike level 1..6 for 60/310 sp" -
  the database holds skill 3 at level 6 with 856 SP left; then it
  bought and equipped two armor pieces at Ariel and walked back to
  the farm spot. The SIGTERM shutdown stayed graceful.
- The user-side check stays the project workflow: watch a learning
  bot reach its teacher and print the "learned <skill> level N"
  lines instead of cycling "town walk stuck, re-pathing".

---

- All four fixes committed and pushed (six commits + the test follow
  up): the npc talk selection clear (983437c), the spellbook keep of
  the sell and destroy junk flows (ffd767d), the aggro answer of the
  engage and the town trips (6c6463a), the teacher legs (407c8f2 then
  d5428e3 - the water guard reads the pure water raster, the skill
  list gate re-arms per session, the trust gamble reverted), the docs
  (in d5428e3) and the rebase follow up for the concurrent water loop
  round (64153f9).
- The live stack validates the full learning cycle end to end (twice,
  including on the merged tree with the concurrent dry search round):
  the trip plans the learning stops, the spellbooks are bought at
  Creamees, the teacher (Ellenia/Cobendell) is reached, clicked and
  the lessons land - "learned Attack Aura level 1 for 920 sp",
  "learned Defense Aura level 1 for 160 sp", the database holds skills
  77 and 91 at level 1, the SP is charged, the books are consumed.
  The E2E run prints E2E_OK with the graceful SIGINT shutdown.
- Verify loop per commit: go build, go vet, the full go test suite,
  gofmt clean, golangci-lint with no new findings in the touched
  files (the pre-existing baseline of the newer local linter version
  in untouched files stays).
- The user-side check stays the project workflow: watch a bot talk to
  its teacher (the "learn:" log lines, the SkillList bumps), watch
  the emergency logout cycles of a piled up bot turn into fights (the
  "is on us, fighting it" line), watch a bought spellbook survive a
  sell trip (the junk batch without the book), and watch the hunt
  after a town trip start cleanly (no 12 s stall on the talked npc).

---

## Task: the town walk click collapse (round 52) - 2026-09-11

Goal: fix the 2026-09-10 state dump report - the bot test1 froze at
44440 51688 -2832 (the elven village terrace) in the townReturn phase
with "Hunt: town walk stuck, re-pathing" burning the whole budget
while the character never moved; the user hypothesis blamed the short
click distance to the next waypoint.

Constraints: the server is the spec (no server behavior patches, the
MOVEDBG diagnostics logging only); all changes on feature/proxy-server,
atomic commits pushed as melg8; the full verify loop per commit.

Acceptance criteria: the exact dump scenario walks to the hunting zone
on the live stack without a single stuck re-path; the offline
regression tests pin the mechanism; the full go test suite, gofmt,
vet and golangci-lint stay green with no new findings in the touched
files.

### Status: done (2026-09-11)

- Root cause (live confirmed with the MOVEDBG patch on the local
  Mobius checkout): the server click validation collapses the click
  destination onto the walker whenever the Bresenham line of the
  click cuts a walled corner (the anti corner cut of
  GeoEngine.checkNearestNsweAntiCornerCut) - "move CANCELED,
  distance=0.0 (geodata collapsed the target onto the walker)". The
  bot planned routes through exactly such corners: the A* diagonal
  rule checked only the source walls (not the flanks), and the
  smoothing verified legs with the supercover raster (cardinal steps)
  while the server validates with its Bresenham raster (diagonal
  double steps + the anti corner cut). The 58 unit click distance was
  not the trigger - any click over the same corner refuses; the
  collapse only equals the walker exactly for short in-cell clicks.
- Three fixes: (1) the search diagonal rule mirrors the server flank
  check (pathfind/search.go, wallsOpen/diagonalFlanksOpen); (2) the
  server click validation port Engine.ValidateClick (new
  pathfind/click_validate.go) with the smoothing verifying every leg
  against it; (3) the follower gates every click through the port and
  reacts to refusals with the leg shortening, the swallowed-bend hop
  and the re-path (hunt/town.go).
- Verification: go test ./... green (19 packages), gofmt/vet clean,
  golangci-lint zero new findings in the touched files; the offline
  regression TestReproVillageZoneReturnWalksThePlan walks the exact
  dump position to the zone in 13 validated clicks with zero
  re-paths; the live rerun of the dump scenario (PathFinding=2, the
  21_19 geodata region, level 13) reaches the Kaboo Orc Grunt S zone
  in 54 s with all clicks ACCEPTED and zero CANCELED on the server
  log, engages and kills on the zone entry; mobius_e2e.sh 45 stays
  E2E_OK.
- Known unrelated: tools/repro_stuck_trip.sh (round 35) fails on both
  the baseline and the fixed build with the current level 13 test1
  state (the auto equipment grinds the injected junk daggers); the
  failure reproduces on the unmodified 36bfe99 build.
- Follow ups (not blocking): the manual walk follower (hunt/user.go)
  and the blind engage walker (loop_los.go) still send unvalidated
  clicks and could adopt the same gate.

## Task: acceptance tests runnable from the live swarm web UI - 2026-09-11

Goal: the user must be able to verify swarm behavior straight from the
running web interface. Add a run test button with a test selection
list, per test hover descriptions (essence, start values, success
criteria), automatic test character provisioning and a green marker
when the scenario completes. The tests must also run one after another
(sequential mode) or all at once (parallel mode, one bot per test).

Constraints: all changes on feature/proxy-server, atomic commits
pushed as melg8 (rebase before push, other agents commit to the same
branch); the server is the spec - no server behavior patches, the
test character setup uses direct database injection of OFFLINE temp
characters only; test characters never collide with the -bots fleet
accounts (they live on temp1/temp2/temp3 accounts with passwords
temp1/temp2/temp3, character names temp1/temp2/temp3); the rest of the
UI keeps working - the test bot stays in the left bot list, the C1
client can attach through the proxy; re-pressing the run button during
or after a run recreates the bot with the same name and the same
scenario path.

The scenarios:
- farm readiness (temp1): a level 15 elven fighter with 20,000 SP and
  100,000 adena spawns at the elven creation point (46045 41251
  -3440, first node of the ElvenFighter template creationPoints),
  empty inventory, no learned skills. The bot must buy proper gear
  (weapon + armor) and the demanded spellbooks, learn every affordable
  lesson (Attack Aura 77 and Defence Aura 91 included), leave town for
  its farm zone and kill at least one mob there under both auras.
- bot lifetime (temp2): mirrors tools/mobius_e2e.sh in-process - the
  bot enters the world, stays online 30s and shuts down gracefully.
- proxy relay (temp3): mirrors tools/proxy_e2e.sh in-process - the
  bot session plus a dedicated proxy on ephemeral ports; a fake C1
  client passes the emulated login, the char list, the world entry
  replay, a live move echo and the locally answered net pings.

Acceptance criteria: the TESTS panel renders in the web UI with hover
descriptions and status colors; every scenario can be started by one
button, re-pressed to recreate the bot; sequential and parallel run
all modes work; the farm readiness scenario passes end to end on the
live stack; go build/vet/test/lint stay green.

### Status: in progress (2026-09-11)

- Environment: swarm_fast_deploy.sh brought the stack up
  (STACK_READY: login 2106, game 7777, db 3306; 75 tables; the
  GitLab clone channel hung, the official API archive channel was
  used instead - the sanctioned fallback of the deploy script).
- Design: internal/swarm/acceptance package - a minimal MariaDB wire
  client (TCP 127.0.0.1:3306, root, empty password), the character
  reset SQL (level/exp/sp/position UPDATE, items/skills/buffs/shortcuts
  wipes, an adena INSERT with an object id below FIRST_OBJECT_ID so
  the running IdManager never collides), the manager (per test status,
  checks, log ring, restart generations, sequential/parallel run all)
  and the session runner (the same wiring runBot uses).
- Next: implement the db client, the manager, the runner, the checks,
  the webserver endpoints and the web UI panel.

### Progress (2026-09-11, round 1)

- Commit "connection: plain Close of the char selection connection":
  the char selection stage probe needs a clean drop (no logout
  announcement) - GameClient.Close.
- Commit "acceptance: the test runner, the db injection and the
  scenarios": internal/swarm/acceptance (db.go + db_test.go with a
  fake wire server, setup.go with the reset SQL, manager.go,
  runner.go, monitor.go + monitor_test.go, scenarios.go,
  scenarios_run.go, relay.go). 19 packages green, 0 lint findings in
  the touched packages, all six web harnesses PASS.
- Commit "webui: the acceptance endpoints and the tests panel": the
  API endpoints, the TESTS sidebar panel, the hover tooltip, the run
  all buttons (sequential + parallel).
- Commit "main: the acceptance manager wiring": the manager attaches
  in both launch modes.
- Next: the live stack verification (run the scenarios through the
  web API against the deployed Mobius C1), then the docs.

### Progress (2026-09-11, round 2: the live verification)

- Live stack: swarm_fast_deploy.sh (SWARM_BRANCH=feature/proxy-server)
  brought the stack up; the Mobius clone needed the sanctioned API
  archive channel (the git protocol hung, then 403 - retried until it
  went through); STACK_READY, 75 tables.
- Commit "acceptance: the session creates the missing temp
  character": the lifetime and relay scenarios failed on a fresh
  database ("the bot never entered the world within 90s") because
  runSession selected a character that did not exist; the session now
  creates the missing elven fighter exactly like runBot does.
- Commit "acceptance: the supervised session, the inclusive wire
  framing and the database selection": three live findings fixed.
  (1) The relay fake client read and wrote the 2 byte size header as
  payload-exclusive while the proxy speaks the Mobius inclusive
  framing - the init read deadlocked ("read init: i/o timeout");
  both directions aligned with the proxy/connection convention plus
  relay_wire_test.go regression tests. (2) The db wire client never
  selected the schema ("1046 No database selected") - the handshake
  response now carries CLIENT_CONNECT_WITH_DB and the database name.
  (3) The farm scenario hung after the hunt loop's emergency logout
  because the acceptance runner had no session supervisor -
  runSessionSupervised mirrors runBotForever (login cooldown honored,
  backoff, stable session reset) and farmTimeout rose 15m -> 30m (the
  aggressive Kaboo Orc road mobs interrupt the town trips, the sell
  old weapon -> buy new cycle leaves the bot weaponless mid trip).
- Commit "acceptance: the sequential run all regression test":
  TestStartAllSequential pins the one after another order (b stays
  idle until a passed).
- Live verification through the web API: bot-lifetime PASSED (entered
  the world, 30s online window, graceful stop), proxy-relay PASSED
  (emulated login, char list, world replay, movement echo, net pings),
  farm-readiness PASSED end to end (level 15 temp1 with 20k SP and
  100k adena at the creation point bought the gear set and the
  spellbooks, learned the affordable lessons, reached the Spore Fungus
  SW zone, ran the auras and killed a mob there in the right clothes).
  The parallel run all starts all three at once (observed live), the
  sequential order is pinned by the unit test.
- go build/vet/test green (19 packages), golangci-lint run --new: 0
  issues, all five node web harnesses PASS.
- Next: none - the round is complete; the scenarios await the user's
  press of the TESTS panel buttons.

### Progress (2026-09-11, round 3: the weaponless livelock and the stagger)

- The farm scenario of the 07:17 process restart livelocked: the
  reset bot landed at the village spawn correctly, but the engage
  leash started the walk home to the picked Spore Fungus SW ground
  BARE HANDED - the hunt loop defers the weapon run until the trip
  machinery owns a tick, and the zone return occupies it. The Kaboo
  Orc packs piled on the unarmed walker, every emergency logout
  saved the character on the ground it fled, the relogin handoff
  resumed that ground and the weapon run from the zone never
  survived the road out: an unarmed logout cycle until the timeout.
- Commit "hunt: the weapon run outranks the zone return walk": the
  leash branch of the engage skips the return walk while
  weaponlessRunWanted holds (a one-time log line marks the hold,
  the zoneReturn flag reuses the ordinary back-home bookkeeping);
  the trip machinery starts the weapon errand on the next tick and
  its return leg walks home armed. A wallet that cannot afford any
  weapon keeps the ordinary return (the punches are all it has).
  TestWeaponlessHoldBlocksTheZoneReturn pins the hold, the one-time
  log and the weapon errand takeover.
- Live verification (process rebuilt at 07:37): the farm run logs
  the hold at 07:37:47, the weapon run starts at the village at
  once (no unarmed zone walk), the gear set lands (5 armor slots,
  5 jewels, weapon), the books buy, the lessons learn (5 skills,
  Attack Aura and Defence Aura included), the bot farms the Spore
  Fungus SW ground under both auras and kills there; the shop
  strategy's mid run upgrade round (the sold chest/head/feet
  rebought at Ariel) re-equips and the scenario PASSED at 08:01:27
  with every condition holding at once, the bot left the world
  gracefully.
- Commit "acceptance: the parallel launch staggers the logins": the
  simultaneous launch of the parallel run all raced the Mobius
  login flood protector (the 350ms window drops the connections of
  one address); the launch now spaces the scenarios two seconds
  apart. Observed live: the starts at 08:04:15.161, 08:04:17.161,
  08:04:19.161, all three temp bots entered the world, bot-lifetime
  and proxy-relay PASSED in the same window, the farm leg re-ran
  the full round.
- go build/vet/test green, golangci-lint run --new: 0 issues.
- Next: the parallel run's farm leg finish, then the push.

### Progress (2026-09-11, round 3 addendum: the road budget verified)

- The parallel run all's farm leg timed out ("cancelled: context
  deadline exceeded") on the road fights: the engage's out of zone
  adoption answered every attacker the aggressive Kaboo territory
  fed it, each kill adopted the next (the respawn window is 15-20s)
  and the leash never resumed the walk home - the log shows the
  Power Strike casts every fifteen seconds for nine straight
  minutes.
- Commit "hunt: the road fight budget lets the walk home resume":
  adoptOutZoneFight counts the consecutive road fights and stops
  starting new ones past roadFightBudget (3) - the walk home
  continues through the blows, the flee flow keeps owning the hurt
  case, the budget resets on the zone entry. Pinned by
  TestRoadFightBudgetResumesTheWalkHome.
- Live verification (rebuilt at 08:46): the farm scenario PASSED at
  09:01:54 - the full cycle (weapon run at the village, the gear
  set, the books, the lessons with the mid run gear upgrades, the
  walk home through the aggressive packs, the auras, the kill) ran
  in 14m54s against the 30m timeout, every condition held at once
  and the bot left the world gracefully.
- The parallel run all verified live: the staggered starts
  (08:04:15.161 / 08:04:17.161 / 08:04:19.161), all three temp bots
  entered the world, bot-lifetime and proxy-relay PASSED in the
  same window, the farm leg's timeout was the road fight finding
  above (fixed and re-verified standalone).
- go build/vet/test green, golangci-lint run --new: 0 issues; all
  four commits rebased over the shop freeze round and pushed.
- Next: none - the round is complete.

### Progress (2026-09-12, round 60: the pantsless return)

- Environment deployed fresh (swarm_fast_deploy.sh: STACK_READY, 75
  tables) and the dev tools installed; the branch checked out at
  4deb888.
- Root cause analysis: the sell-first step of the town trips banks
  the credit of displaced equipped pieces before the replacement
  buy; every exit between the two (a silently refused buy after the
  3 retries, a merchant no-show, an attacker interrupt that drops
  the whole trip, a walk abort, a session death the relogin resumed
  into the return leg) ends the trip without the replacement and
  nothing detects the regression - the five minute cooldown armed
  and the bot farmed on half dressed. The dump's own session
  started at the village (the previous session died mid trip) and
  the return leg finished as a success ten seconds before the dump.
- Commit "hunt: the gear debt - the town trip answers for the slot
  it stranded": the trip start snapshots the paperdoll
  (snapshotTripGear), every exit (endTownTrip, the interrupt
  resetTownTrip) arms gear debt for a slot that was occupied, sits
  empty and whose piece is gone (gearDebtCheck, a log line names
  the slot and the lost piece), the debt shortens the trip cooldown
  to the gear run window (gearDebtRunWanted) and clears with a log
  line when the slot is dressed again (clearRefilledDebt). The trip
  start reason appends "(the gear debt refill)".
- Commit "webui: the state dump names the empty paperdoll slots":
  the equipment section lists the unfilled families below the worn
  pieces ("empty slots: ..."), so the next pantsless report shows
  the hole at a glance (the two hand weapon and the one-piece
  blockers stay unlisted).
- Reproductions: gear/round60_repro_test.go (the planner plans the
  Leather Pants filler for the empty legs of the dump wallet) and
  hunt/round60_repro_test.go (the full stranding flow arms the debt
  with the short cooldown, the debt runs the refill trip, the debt
  lifecycle clears on the refill, the interrupt exit arms it, a
  fresh loop self-heals the dump state).
- Commit "acceptance: the gear gap scenario replays the pantsless
  dump and buys the legs armor back": the temp5 account wakes as
  the exact dump character (the 11 piece paperdoll minus the legs,
  13162 adena, the reported farm spot) and the run passes when the
  legs slot is dressed again. The zone-return and gear-gap
  scenarios share the new runSupervisedScenario skeleton (the dupl
  finding of the round).
- go build, gofmt, the full test suite (19 packages) and
  `golangci-lint run --new` (0 issues) green.
- Live verification: `-acceptance gear-gap` against the deployed
  stack PASSED - the plan triggered the trip, the sell-first sold
  the displaced Leather Shirt, Ariel bought "Leather Pants, Wooden
  Breastplate" and the auto equipment equipped "Leather Pants (27)
  into the empty legs slot".
- Docs: development_log round 60, shopping_strategy Rule 2c (the
  gear debt), the hunting.md town trip section, the AGENTS.md
  documentation map unchanged (the shopping doc entry covers it).
- Status: done (2026-09-12).

## Active task: the self-organization design notes, the roadmap ladder and the backlog queue

Started: 2026-09-12. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The owner asked (2026-09-12, Russian) how to organize the feedback
loops so the agents can always verify their assumptions about the
protocol, the server and the character behavior; which implementation
language to pick; how the agents should self-organize towards the
solo 1-60 goal from a single boot prompt; and how the human sees the
real progress. The deliverable is a design document plus the
operational scaffolding it prescribes.

### Result

- docs/agent_selforganization.md (Russian): the constraints-to-process
  table, the language argument (Go), the verification pyramid L0-L6
  with time budgets and the "verify what you touched" rule, the
  repo-as-external-brain self-organization protocol (claim by commit,
  4h lease, the stop protocol), the copy-paste boot prompt, the
  progress visibility plan and the failure playbook.
- docs/ROADMAP.md: the solo 1-60 milestone ladder M0-M6 with binary
  acceptance criteria (M0 done; M1 the soak proof; M2 the first
  profession; M3-M5 the bands; M6 the integral run).
- docs/BACKLOG.md: the task queue with the claim/lease protocol and
  the seed tasks T-001..T-007 (the soak metrics, the stagnation watch,
  the level milestone scenario, the quest research, the hypotheses
  registry, the band survey, the progress page).
- AGENTS.md: the work protocol now points at the roadmap and the
  backlog when agent_progress.md has no unfinished task.

### Verification

Docs-only round: the environment was deployed fresh
(swarm_fast_deploy.sh: STACK_READY, the schema loaded, the ports
listening), `tools/mobius_e2e.sh 45` printed E2E_OK and the full suite
`go test ./... -count=1` was green (19 packages) before the doc work
started; no Go files were touched by this task (the diff is markdown
only).

### Status: done (2026-09-12)

## Active task: T-001 the soak metrics trail

Started: 2026-09-12 07:27Z. Branch: `feature/proxy-server`. Commits as
melg8. Agent label: `soak-z`. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

Add the `soak` acceptance scenario, the `runs/metrics.jsonl` trail and
the `tools/progress_report.sh` renderer. The soak scenario is the M1
acceptance vehicle: a fresh account runs `-hunt` for N minutes (the
real M1 run is 8h, the dev smoke is ~10m) under supervision, and one
JSON line per run lands in `runs/metrics.jsonl` with date, scenario,
duration, start/end level, XP per hour, deaths, adena, stuck events
and PASS/FAIL. The pass criteria include the stagnation guard (no XP
gain for M minutes, no position change for K minutes fails the run);
the guard is an acceptance-package read of the tracker public API
within T-001 scope, so T-002 (the hunt-loop stagnation watch) can
later replace it with the tracker event source.

### Constraints

- Scope: `internal/swarm/acceptance/`, `cmd/swarm/`, `tools/`, `docs/`
  only. Do NOT touch `internal/swarm/hunt/` or `internal/swarm/state/`
  (that is T-002 and T-003 territory).
- The duration is configurable (env `SWARM_SOAK_MINUTES`, default a
  smoke value); the real 8h run is a follow-up operator action, not a
  2h-session deliverable.
- The metrics writer appends exactly one JSON line per run, atomically
  (open with O_APPEND, one Write call), so parallel runs never
  interleave.
- Lint clean (`golangci-lint run --new`), the touched package tests
  green, the smoke run produces one metrics line against the live
  stack.

### Acceptance

- `-acceptance soak` runs the scenario, prints PASS/FAIL and appends a
  line to `runs/metrics.jsonl`.
- `tools/progress_report.sh` writes `PROGRESS.md` from the metrics
  tail, the BACKLOG statuses and `git log --oneline -20`.
- Unit tests cover the stagnation guard and the metrics serialization.

### Progress

- 2026-09-12 07:27Z: T-001 claimed (BACKLOG status in_progress), the
  active task entry started. Environment already deployed
  (STACK_READY, 75 tables, dev tools installed). Reading the
  acceptance package shape next.
- 2026-09-12 07:34Z: the soak scenario, the stagnation guard, the
  metrics writer and the death/stuck helpers land in
  internal/swarm/acceptance/ (soak.go, soak_guard.go,
  soak_metrics.go, soak_helpers.go); the scenario is registered in
  scenarios.go (the temp7 account, the duration-aware timeout), the
  CLI flag help updated. Unit tests (soak_test.go) cover the guard
  fires (no XP, no move), the healthy never-fire, the startup grace,
  the metrics JSON, the append atomicity, the XP math, the death
  edge tracker, the duration env, the cumulative XP table.
  `golangci-lint run --new` clean (0 issues) after the funlen split,
  the exhaustruct full literals, the goconst constants and the
  nlreturn blank lines.
- 2026-09-12 07:43Z: tools/progress_report.sh renders PROGRESS.md
  from the metrics tail, the BACKLOG statuses and the last 20
  commits; runs/README documents the JSONL schema.
- 2026-09-12 07:45Z: live smoke run PASSED. SWARM_SOAK_MINUTES=2
  against the deployed stack: temp7 farmed level 1 to 2 in 120 s
  (9984 XP/h, 0 deaths, 0 stuck events, 28 adena), the bot left the
  world gracefully, runs/metrics.jsonl received the PASS row. The
  8-hour M1 proof is a follow-up operator run (the machinery is
  duration-agnostic).

### Status: done (2026-09-12)

T-001 complete: the soak scenario, the runs/metrics.jsonl trail, the
stagnation guard and the progress report ship together. The unit
tests are green, lint:new is clean, the live smoke run produced a
PASS metrics row. The 8-hour M1 acceptance run is the next step
(SWARM_SOAK_MINUTES=480), owned by whoever triggers the milestone
closure. The stagnation guard is an acceptance-package read today;
T-002 (the stagnation watch) can later move the events into the hunt
loop and the soak scenario can consume them.

## Active task: T-002 the stagnation watch

Started: 2026-09-12 07:36 UTC. Branch: `feature/proxy-server`.
Agent: zai-agent. Commits as melg8. Other agents may push to the
same branch concurrently - rebase before every push (the T-001
claim of this session lost to soak-z at 07:27Z, the queue rule
picked the next todo task).

### Goal

The M1 soak proof requires that a silent livelock becomes a loud
line: the hunt loop logs explicit events when the character gains
no XP for M minutes or holds the same position for K minutes. The
events surface in the web UI event feed (tracker event log) and in
the bot log with the current phase, so a freeze like the round 58
stuck cell (a character standing on one cell for over an hour) is
visible without a state dump.

### Constraints

- Scope: internal/swarm/hunt/, internal/swarm/state/, docs/ only.
  The acceptance package belongs to soak-z (T-001, in flight).
- Both thresholds are named constants, unit tested through the
  clock seam of the loop tests; no real wall clock timing in tests.
- The watch never blocks the hunt loop: it observes the tracker
  state the loop already reads and logs.
- The event lines carry the phase (tracker.Phase style) so the log
  explains what the loop was doing while frozen.

### Acceptance

- Unit tests: xp stagnation fires once per window and re-arms, the
  position freeze fires on a held cell, both stay silent while the
  values move, offline sessions pause the timers.
- go build, gofmt, the hunt and state package tests, golangci-lint
  run --new all green.
- A live note in docs/development_log.md (the round entry) - a full
  live acceptance belongs to T-001/T-003 scenarios.

### Progress

- 07:36 UTC: T-002 claimed in docs/BACKLOG.md.
- 07:40 UTC: the stagnation watch landed (hunt/stagnation.go): the
  named windows stagnationXPWindow (20 min, calibrated over the
  measured town trip rounds) and stagnationPositionWindow (10 min),
  the observeStagnation hook on the tick publish defer (every tick
  path, including the death and logout early returns), one event
  line per window via logf (console + tracker event feed +
  NoteAction), the phase word in every line, the offline/manual
  reset. The state dump diagnostics gained xpStallForMs and
  positionStallForMs (state.HuntDiagnostics + the snapshot
  encoder). 8 unit tests (hunt/stagnation_test.go): fire, re-arm,
  refresh-while-moving, manual/offline quiet, tick-path coverage,
  event feed surface, diagnostics wiring. go build/vet, the full
  test suite, gofmt and golangci-lint --new green; the full lint
  reports only the pre-existing findings of the branch.
- 07:52 UTC: docs closed out - hunting.md gained the stagnation
  watch bullet (the windows, the event shape, the routing, the
  reset semantics), webui.md documents the two new stall fields of
  the hunt diagnostics subview, development_log.md carries the
  round 64 entry (problem -> cause -> fix -> verification - renumbered over
  the two parallel round 63 entries of the branch).
- Live verification: tools/mobius_e2e.sh 45 printed E2E_OK (the
  manual session regression with the watch compiled in); a 60 s
  autonomous -hunt smoke against the live stack killed a mob with
  the watch armed, the live snapshot showed xpStallForMs and
  positionStallForMs tracking the real progress (the kill, the loot
  walk) and the log stayed free of stagnation lines; the SIGINT
  shutdown was graceful.
- 07:53 UTC: status done - the task is complete, the queue rule
  hands the round to the next claimable BACKLOG task.

## Active task: T-004 the quest subsystem research

Started: 2026-09-12 07:35 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: agent-quest.

### Goal

BACKLOG T-004 (milestone M2, P2, deps none): read the Mobius C1 Java
sources and write `docs/quest_protocol.md` - the quest packet flow
(the quest list, the NPC html dialog packets, quest state
transitions, quest item drops), with a link to every relevant Java
class, in the shape of `docs/protocol_description.md`. Research only:
no code changes. The follow-up code tasks land in the BACKLOG resume
notes.

### Context

T-001 (the soak metrics trail) was claimed by soak-z and T-002 (the
stagnation watch) by zai-agent while my claim commit lost the push
race - both are theirs now, no fight over tasks. T-003 waits on
T-002. T-004 is the top claimable task left and it de-risks M2 (the
first profession: the elven class transfer quest at level 20).

### Acceptance

- `docs/quest_protocol.md` documents: the quest list packet flow, the
  NPC dialog (html) packet flow, the quest state transitions, the
  quest item drop mechanics, every layout with the reference Java
  class, plus the elven fighter class transfer quest chain
  (ElvenKnight/ElvenScout) walked end to end from the sources.
- The follow-up code tasks are listed (in the resume notes and/or a
  proposal section of the doc).
- No Go code changes (docs-only round: `go build ./...` and the lint
  gate stay green trivially).

### Progress

- 07:35 UTC: claimed T-004 after losing the T-001 push race.

## Active task: T-005 the hypotheses registry convention

Started: 2026-09-12 07:38Z. Branch: `feature/proxy-server`. Commits as
melg8. Other agents may push to the same branch concurrently - rebase
before every push.

### Goal

Claimed from docs/BACKLOG.md (T-005, milestone M1, priority P3,
scope: AGENTS.md, docs/navigation_analysis.md): add the
"Hypotheses / Unknowns" convention to the AGENTS.md documentation
rules - every unverified server assumption must live in a registry
section with a verification plan, and code relying on it must
reference it. Seed the registry with the open items of
docs/navigation_analysis.md (swimming semantics, doors, the gatekeeper
graph) so the pattern starts populated.

### Session history

- T-002 and T-004 were claimed by parallel agents seconds before my
  claim pushes landed (fetch+rebase showed their commits first) - the
  protocol says do not fight over tasks, both were conceded.
- T-001 claimed by soak-z 07:27Z, T-002 by zai-agent 07:36Z, T-004 by
  agent-quest 07:35Z.

### Result

- AGENTS.md: the "Hypotheses and unknowns" registry convention - an
  unverified server assumption becomes an H-NNN entry before the
  relying code is written, the entry names its evidence (Mobius Java
  classes + live experiment), running the plan closes it (a
  confirmed fact moves into the subsystem doc, a refuted one records
  the server's actual behavior), entries append, ids never reuse.
- The seed entries H-001 (swimming semantics), H-002 (the gatekeeper
  teleport graph), H-003 (the boats), H-004 (the doors) - every
  named Java class was located in the local Mobius checkout before
  writing the plan (Stat.BREATH at mechanics/stats/Stat.java:117,
  Teleporter.onBypassFeedback, WaterTask, Door openable families,
  the vehicle packet set).
- docs/navigation_analysis.md links its water, meta transport and
  door open items to the registry ids.
- docs/development_log.md round 62 entry.

### Verification

Docs-only round: `go build ./...` green, `golangci-lint run --new`
clean (0 issues); the stack was up for the source reading
(STACK_READY, 75 tables); no behavior change, no e2e required.

### Status

Done (2026-09-12 07:55Z) - taking the next eligible BACKLOG task.

## Active task: T-006 the 20-25 band survey

Started: 2026-09-12 07:58Z. Branch: `feature/proxy-server`. Commits as
melg8.

### Goal

Claimed from docs/BACKLOG.md (T-006, milestone M3, priority P3, scope:
docs/, tools/generate_hunt_zones.py): survey the Mobius spawn data for
the 20-25 mob band reachable from the elven lands, in the shape of the
elven zone survey - territories, towns, merchants, teachers. Output: a
survey document plus the follow-up task list (the registry generation
itself is a separate task once M1 is green).

### Result

- tools/generate_hunt_zones.py: the --survey MIN MAX mode (the
  grounds, the mob stats with the effective aggression, the teleport
  anchors on the Mirabel/Bella/Trisha route chain); the default
  registry mode verified byte identical.
- docs/band_20_25_survey.md: the transport chain (13 600 one way to
  the Execution Ground), the four ground families with per-mob
  hp/exp/aggro, the Dion merchant buylists, the elven village
  teacher economy (25 200 round trip), the Cure Bleeding spellbook
  gap, six follow-up items.
- The backlog gained T-008 (the gatekeeper teleport flow, P2),
  T-009 (the band registry, gated on M1 green) and T-010 (the band
  gear catalogs).
- docs/navigation_analysis.md: the "mobs never attack on sight"
  claim refuted and corrected (NpcTemplate default isAggressive
  TRUE; the explicit false list is the passive one).
- docs/development_log.md round 63.

### Verification

The survey data comes from the live Mobius checkout (spawn xml,
npc stats, teleporter data) cross-checked with NpcTemplate.java and
the skill trees; `go build ./...` green, `golangci-lint run --new`
0 issues; docs + tooling only - no behavior change, no e2e run
required.

### Status

Done (2026-09-12 08:29Z) - taking the next eligible BACKLOG task.

## Active task: T-007 the PROGRESS.md page

Started: 2026-09-12 08:41Z. Branch: `feature/proxy-server`. Commits
as melg8.

### Goal

Claimed from docs/BACKLOG.md (T-007, milestone M1, priority P3, deps
T-001 done, scope: tools/, docs/): the one-page human dashboard -
milestone ladder green/red by the last acceptance results, the last
soak metrics, the active BACKLOG tasks, the last commits. Generated
by the tooling of T-001 (tools/progress_report.sh), committed after
every milestone-relevant run so the page history is the project
history.

### Result

- tools/progress_report.sh: render_milestone parses docs/ROADMAP.md
  and walks the whole ladder - the (DONE) headings render done, the
  first open milestone colors green/red by the last metrics row
  status, the rest pending; the section header renamed "Milestone
  ladder".
- PROGRESS.md: the first committed page (M0 done, M1 green from the
  07:45 smoke soak PASS, the BACKLOG table, the last 20 commits).
- docs/development_log.md round 65 (renumbered past the parallel T-002 round 64).

### Verification

The page re-rendered and the ladder cross-checked against
ROADMAP.md; `go build ./...` green, `golangci-lint run --new` 0
issues (bash/python tooling + docs only, no Go code touched); no
behavior change, no e2e required.

### Status

Done (2026-09-12 08:53Z) - session wrap-up window starts.

## Active task: T-003 the level milestone scenario

Started: 2026-09-12 07:51 UTC. Branch: `feature/proxy-server`.
Agent: zai-agent. Commits as melg8. Other agents may push to the
same branch concurrently - rebase before every push (T-002 closed
at 07:53 UTC by this session, its dep is green).

### Goal

Generalize farm-readiness into the level milestone scenario: a
scenario that DB-injects a character at an arbitrary level N with
the zone-appropriate gear and passes when the character reaches
level N+1 within the time budget. This is the building block every
later milestone acceptance reuses (M2 injects level 20 for the class
transfer, M3+ injects the band levels).

### Constraints

- Scope: internal/swarm/acceptance/ (+ docs). No hunt/state changes.
- The level N must be parameterizable (env) with a sane default;
  the near-threshold xp keeps the run minutes long, not hours.
- The injection starts the character inside its level-appropriate
  hunting ground with the gear the shop strategy would reach (the
  farm-readiness pattern: the wallet, the empty bag, the bot shops).
- The pass check watches the level (the UserInfo level of the
  tracker), the timeout bounds the run.

### Acceptance

- A live run of the scenario at the default level passes against
  the deployed stack (level N -> N+1 observed).
- Unit tests for the reset construction and the pass evaluation.
- golangci-lint run --new clean, the touched package tests green.

### Progress

- 07:51 UTC: T-003 claimed in docs/BACKLOG.md.
- 08:00 UTC: the level-milestone scenario landed
  (acceptance/level_milestone.go): the level N injection (the
  SWARM_LEVEL_MILESTONE_LEVEL env, default 10, 1..20) with the
  near-threshold exp (span/20 below the N+1 threshold), the
  elven fighter vitals table from the Mobius template XML, the
  farm readiness wallet and spawn; the pass watch on the tracker
  level; the metrics row on every outcome; the Definitions
  registration with the temp8 account (the manager account
  contract test extended).
- 08:08 UTC: live PASS - the default level 10 -> 11 run finished
  in 7m46s (0 deaths, 0 stuck, the row in runs/metrics.jsonl).
- 08:10 UTC: the live row exposed the xpPerHour double count of
  the T-001 cumulative helper (187169 for a real 1266 xp): the
  Mobius exp is the running total (PlayableStat.addExp +
  UserInfo.writeImpl read in the Java checkout), the helper added
  the level start threshold on top. Fixed (cumulativeSoakXP now
  returns the exp itself), the soak unit tests updated, the note
  in runs/README.md; the historical rows stay as written.
- 08:13 UTC: the second live PASS - the env run level 2 -> 3 in
  2m30s with the corrected metrics row (2111 xp/h for the real
  88 xp gain). The dev log carries the round 66 entry (renumbered
  past the parallel PROGRESS.md page round 65).
- 08:15 UTC: status done.

## Active task: T-008 the gatekeeper teleport flow

Started: 2026-09-12 07:55Z. Branch: `feature/proxy-server`. Commits
as melg8. Agent label: `soak-z`.

### Goal

The M3 gatekeeper dialog + teleport travel (the H-002 verification):
implement the packet chain the bot needs to drive Mirabel -> Gludio ->
Dion.

### Progress (session partial)

- 2026-09-12 07:55Z: the packet-protocol foundation landed.
  to_game_server/request_bypass_to_server.go (opcode 0x21, the bypass
  command string) and from_game_server/npc_html_message.go (opcode
  0x1B, npcObjId + html + itemId) with 3 unit tests each. Build, the
  two packet-package tests and golangci-lint --new are green.
- The hunt-loop integration (the 0x1B dispatcher case, the talk +
  html parse + RequestBypassToServer teleport + the arrival
  Appearing) and the live Mirabel -> Gludio -> Dion drive are the
  next agent's work (see the BACKLOG resume notes).

### Status: in_progress (2026-09-12) - handed off

The packet parsers are the foundation; the connection wiring, the
hunt-loop gatekeeper step and the live verification remain. A new
agent reads this entry and the BACKLOG resume to continue.

### Progress (2026-09-12, T-004 round)

- 07:35 UTC: claimed T-004 (the T-001 claim lost the push race to
  soak-z at 07:27; T-002 was taken by zai-agent at 07:36 - both
  theirs, T-003 waits on T-002, T-004 is the top claimable task).
- The Mobius source survey: the quest engine
  (mechanics/script: Quest, QuestState, State), the dialog entry
  (Action 0x04 -> NpcClick -> showChatWindow / ON_NPC_FIRST_TALK),
  the bypass channel (RequestBypassToServer 0x21 -> BypassHandler
  -> ScriptLink/ChatLink), the html action cache
  (AbstractHtmlPacket + HtmlUtil: the anti-injection gate, the 250
  unit origin check, the $-parameter prefix matching), the quest
  journal packet (QuestList 0x98, the cond/flags encoding), the
  persistence (character_quests: charId/name/var/value), the kill
  notification delay (Attackable._onKillDelay = 2500 ms), the
  dialog gates (weight penalty, 90 percent inventory, 25 quests).
- The class transfer chain walked end to end from the sources:
  Q00406 (Sorius Gludio -> skeletons Ruins of Agony -> Kluto Gludin
  -> Ol Mahum Novice -> the brooch 1204) and Q00407 (Reisa ->
  Moretti -> Babenco -> Prias -> the recommendation 1217), the
  class change at Rains 30288 (level 20 + the mark item ->
  setPlayerClass(19) + broadcastUserInfo), the gatekeeper legs
  (Mirabel 30146 elven village -> Gludio 9200a, Bella 30256 Gludio
  -> Gludin 7300a).
- Live verification (SWARM_TRACE_PACKETS=1, accounts trace1/trace2
  and the farm-readiness scenario on temp1, all against the
  deployed stack): the QuestList 0x98 (5 bytes, empty) arrives at
  world entry unrequested (EnterWorld.java line 303 confirms the
  push), and the merchant dialogs push NpcHtmlMessage 0x1B (813
  bytes) about once per second through the whole sell round - the
  dialog stream the bot currently drops unseen.
- The deliverable: docs/quest_protocol.md (the engine, the packet
  layouts with the Java references, the state machine, the event
  firing rules, the two quest chains, the geography gap, the
  follow-up task list).
- Docs-only round: `go build ./...` green, `golangci-lint run --new`
  clean (no Go file touched).
- Status: done (2026-09-12, 08:15 UTC) - docs/quest_protocol.md
  delivered, the follow-up code tasks listed in the BACKLOG resume
  notes, the development_log round 66 entry written. The T-008
  packet foundation of soak-z covers follow-up items 1 and 3.

## Active task: T-011 the quest journal parser and tracker section

Started: 2026-09-12 08:05 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: agent-quest.

### Goal

BACKLOG T-011 (milestone M2, follow-up 2 of the quest research):
parse the QuestList packet (0x98) and land the quest journal in
the state tracker. T-009/T-010 (the remaining todo tasks) both
depend on T-008 (in progress), so this is the top claimable work.

### Plan

1. packets/from_game_server/quest_list.go: the parser
   ([opcode 0x98][questCount: 2]{[questId: 4][state: 4]}
   [itemCount: 2]{[objectId: 4][itemId: 4][count: 4]
   [bodyPart: 4]}) with the caps (quests 64, items 256) and the
   golden tests incl. the live 5 byte empty form.
2. state/quests.go: the journal (map questId -> cond) + the quest
   item stacks (map itemId -> count, the quest-item id set feeds
   the sell filter later); accessors QuestCond/QuestCount/
   QuestItemCount; replaced whole like the skill list (the server
   always sends the full journal).
3. connection: the 0x98 dispatcher case + applyQuestList wiring
   (same shape as applySkillList).

### Progress

- 08:05 UTC: claimed T-011 (a new BACKLOG entry from the T-004
  follow-up list; T-009/T-010 wait on T-008).

- 08:07-08:12 UTC: implemented and verified:
  - packets/from_game_server/quest_list.go: the QuestList parser
    (readQuestEntries + readQuestItemEntries split for the cyclop
    bound), the caps (quests 64 - the server refuses more than 25
    started quests; items 256) and the reusable buffers;
  - state/quests.go: the journal (quest id -> cond/flags map, the
    quest item id -> count map), ApplyQuestList replaces whole
    (the server always sends the full journal), the accessors
    (QuestCond, QuestCount, QuestIDs sorted, IsQuestItem,
    QuestItemCount) and the ResetSession clear (the journal is
    session state the server repushes at world entry);
  - connection: the 0x98 dispatcher case, applyQuestList and the
    two convert helpers (the parse buffer is reused, the state
    view must not alias it);
  - tests: 7 parser tests (the live 5 byte empty form golden, the
    populated class transfer journal, buffer reuse, bad id,
    truncated quest/item entries, the count caps) + 4 state tests
    (the journal pins, the whole-list replacement, the sorted ids,
    the session reset clear);
  - go build ./... green, the three package test runs green,
    golangci-lint run --new: 0 issues (the cyclop and lll findings
    of the first draft fixed by the split and the reflow);
  - live verification against the deployed stack: the fresh trace3
    bot printed "Quest journal with 0 quests, 0 quest items" at
    the enter world second - the 0x98 push now lands in the
    tracker instead of dropping on the floor.

## Active task: T-012 the html dialog link parser

Started: 2026-09-12 08:15 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: agent-quest. Taken after T-011
closed (T-009/T-010 still wait on T-008).

### Progress

- 08:15-08:25 UTC: implemented and verified:
  - packets/from_game_server/html_links.go: ParseHTMLLinks
    extracts the bypass links of a server dialog page (the command
    per <a action="bypass ..."> with the -h prefix stripped and
    trimmed, plus the visible link text), mirroring
    HtmlUtil.buildHtmlBypassCache (the case-insensitive "=\"bypass "
    match on the lowercased html, the original casing preserved,
    the unterminated attribute ends the scan, the 128 link cap);
  - 8 unit tests: the real Sorius quest page, the Rains class
    master page (the -h strip, the multi-link order), the
    case-insensitive attribute, the $ parameter marker, the command
    trim, the non-bypass actions, the empty/broken pages, the cap;
  - verification against the real datapack: the parser walked every
    page of the Q00406 script, the ElfHumanFighterChange1 master
    pages and the Mirabel teleporter page - 112 links, commands and
    texts exact (the %objectId% of the file form stays as the raw
    token - the server replaces it at send time, the packet form
    arrives resolved);
  - go build ./... green, the package tests green,
    golangci-lint run --new: 0 issues (the HtmlLink -> HTMLLink
    naming and the TrimPrefix staticcheck findings fixed);
  - docs/quest_protocol.md: the follow-up list marks the link
    parser landed (and the T-011 journal parser landed).
- Status: done (2026-09-12, 08:25 UTC).

## Active task: T-013 the open dialog section of the tracker

Started: 2026-09-12 08:18 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: agent-quest.

### Progress

- 08:18-08:27 UTC: implemented and verified:
  - state/dialog.go: the open dialog page of the NPC_HTML scope
    (the npc origin, the item id, the links) - ApplyDialog replaces
    the page whole (one page per scope, the same semantics as the
    server cache), the accessors (DialogLinks defensive copy,
    DialogOrigin) and IsDialogCommand mirroring
    Player.validateHtmlAction (the exact match or the trimmed
    prefix of the '$' variable parameter link); ClearDialog and the
    ResetSession clear (the relogin starts with no dialog);
  - 7 unit tests: the Sorius quest page pins, the page replacement
    (the old links stop validating), the '$' parameter prefix rule
    (the prefix must match, the suffix commands pass, the near miss
    fails), the defensive copy (no aliasing of the applied page or
    the returned slice), the session reset and the explicit clear;
  - go build ./... green, the state and packet package tests green,
    golangci-lint run --new: 0 issues (the '$' prefix fix mirrors
    the Java exactly - the marker is stripped and the remainder
    trimmed before the prefix match; the exhaustruct findings
    resolved with the repo's zero-view nolint pattern);
  - the consumer wiring (the 0x1B dispatcher case feeding
    ApplyDialog) stays with T-008 (their scope: connection/).
- Status: done (2026-09-12, 08:27 UTC).

## Session handover notes (agent-quest, 2026-09-12 08:30 UTC)

The quest reading chain of this session (all live verified or
datapack verified, all pushed):

- T-004 (done): docs/quest_protocol.md - the quest protocol map
  (the engine, the dialog packets, the bypass validation, the two
  class transfer chains, the gatekeeper geography, the follow-up
  list).
- T-011 (done): the QuestList 0x98 parser + the state quest journal
  + the dispatcher wiring ("Quest journal with 0 quests, 0 quest
  items" at the enter world second, live).
- T-012 (done): ParseHTMLLinks - the bypass link extraction of a
  dialog page (112 links verified against the real datapack pages).
- T-013 (done): state/dialog.go - the open dialog page of the
  tracker with the IsDialogCommand mirror of
  Player.validateHtmlAction.

For the next agent:

- The dialog pieces now compose: the connection layer of T-008
  exposes GameClient.LastHTMLMessage() (the parsed html string of
  the last NpcHtmlMessage); the tracker exposes ApplyDialog (the
  links + the origin + the validation mirror); the walker should
  feed the tracker from the apply path (or read both) before any
  bypass is sent.
- T-009/T-010 wait on T-008 (in progress - the dispatcher wiring
  landed at 08:20, the live Mirabel -> Gludio drive remains); once
  it closes, the zone registry generation and the band gear
  catalogs open up.
- The M2 quest rounds that follow: the dialog walker (pick links by
  text through the tracker), the Q00406/Q00407 chains as data, the
  class-transfer acceptance scenario (the T-003 level milestone
  scenario - closed by another agent at 08:15 - is the injection
  vehicle).
- The quest journal, the dialog section and the link parser carry
  unit tests next to them; the live facts (the world entry push,
  the 813 byte merchant pages, the 2.5 s kill delay) live in
  docs/quest_protocol.md.

## Active task: T-014 the quest dialog walker engine

Started: 2026-09-12 08:45 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: quest-walker-e3f8.

Goal: the generic dialog walker the M2 quest brain runs on (the
follow-up 4 engine half of docs/quest_protocol.md): the two-click
talk entry, the new-page wait with content change detection, the
hunt-side feed of the tracker dialog section and the link-by-text
bypass walk with IsDialogCommand validation.

Constraints: new files only in internal/swarm/hunt/ (T-008 owns
the live edits of loop.go and the gatekeeper files); the loop
phase wiring is NOT this task (the M2 acceptance round composes
it); no behavior change of the running bot until the wiring.

Acceptance: unit tests on the real Q00406 and class master page
goldens (the fakeGame stub) green, golangci-lint run --new clean,
a live round trip against a village npc of the deployed stack
(click -> html -> link -> bypass -> next html) recorded in the dev
log, docs/quest_protocol.md follow-up list updated.

### Progress

- 08:45 UTC: claimed (BACKLOG T-014 in_progress + this entry).
## Session close: the M1 evidence runs (2026-09-12, zai-agent)

The session closed T-002 (the stagnation watch), T-003 (the level
milestone scenario) and the xpPerHour double-count fix of the metrics
trail; the last minutes added one more M1 evidence row:

- Live soak 15 min: PASS (temp7, level 3 -> 4, 5533 xp/h, 0 deaths,
  0 stuck events, the row in runs/metrics.jsonl) - the corrected
  metrics math on a real long run, the stagnation guard and the hunt
  watch both quiet on a healthy session.
- The M1 trail so far: the 2 min soak smoke (1 -> 2), the 15 min
  soak (3 -> 4), the level milestone 10 -> 11 and the env run
  2 -> 3 - all PASS. The 8 hour M1 proof remains the operator run
  (SWARM_SOAK_MINUTES=480) - it cannot fit a 2h agent session.
- Operational lesson recorded: a long acceptance run piped through
  grep to the tool call dies on the 1 MiB result frame limit - the
  next long run must redirect the bot stdout to a file (the
  tools/mobius_e2e.sh pattern) and poll the metrics file instead.
- The queue check at the close: every todo task (T-009, T-010)
  waits on T-008 (in_progress by another agent); nothing claimable
  without fighting over work.

Status: session complete - the next agent starts from the BACKLOG
queue (T-008 unblocks T-009/T-010) or the M1 operator soak run.


## Active task: T-008 the gatekeeper teleport flow (resumed + done)

Started: 2026-09-12 07:55Z (round 63 foundation). Resumed: 2026-09-12
08:08Z (this session). Branch: `feature/proxy-server`. Commits as
melg8. Agent label: `soak-z`.

### Goal

The M3 gatekeeper dialog + teleport travel (the H-002 verification):
implement the packet chain the bot needs to drive Mirabel -> Gludio ->
Dion.

### Progress

- 07:55Z (round 63): the packet parsers (RequestBypassToServer 0x21 +
  NpcHTMLMessage 0x1B) with unit tests + protocol_description.md docs.
- 08:15Z: the connection dispatch (0x1B → applyNpcHTMLMessage →
  LastHTMLMessage/LastHTMLDialog, SendBypass) with 4 unit tests.
- 08:23Z: the html bypass parser (hunt/gatekeeper_html.go:
  ParseGatekeeperHTML, FindTeleportButton, FindShowTeleportsButton) with
  7 unit tests.
- 08:30Z: the gatekeeper step (hunt/gatekeeper_step.go:
  DriveGatekeeperTeleport, bounded 5s dialog wait) + the GameAPI
  SendBypass/LastHTMLDialog seam + 4 unit tests.
- 08:40Z: live verified - the building-entry acceptance run with
  SWARM_TRACE_PACKETS=1 showed the server 0x1B packets (813 bytes)
  arriving and the dispatch parsing them (no parse failure,
  building-entry PASSED).
- 08:45Z: the ApplyDialog bridge - the 0x1B handler now feeds the
  state tracker's open dialog section through ParseHTMLLinks (T-012),
  completing the 0x1B wiring the T-013 round named as T-008 scope.
- go build/vet green, golangci-lint run --new: 0 issues on the touched
  packages (connection, hunt).

### Status: done (2026-09-12 08:45Z)

T-008 complete: the full packet chain ships (parsers, dispatch, send,
html parser, gatekeeper step, ApplyDialog bridge, docs). Unit tests
green, lint clean, the live packet dispatch verified against the real
server. The full Mirabel -> Gludio -> Dion live drive is the follow-up
(the hunt-loop gatekeeper trip phase, the T-009 prerequisite): the
packet chain, the parser and the step are ready to wire.

## Active task: T-010 the band gear catalogs (in-scope done)

Started: 2026-09-12 08:53Z. Branch: `feature/proxy-server`. Commits as
melg8. Agent label: `soak-z`.

### Goal

The D-grade gear catalog of the Dion merchants for the 20-25 band
shopping trip.

### Result

- gear.DionCatalog() (gear/dion_catalog.go): Sabrin (7060, weapons),
  Casey (7061, armor), Sonia (7062, jewels + spellbooks), Lara (7063,
  grocery) at the 20 percent Dion buy tax, buylists 3006000-3006300.
  Two unit tests pin the merchant set and the tax shape.
- The npcdata buylists and D-grade item GearStats were already
  generated (verified item 256: DUALFIST D-grade).
- The out-of-scope follow-up (a new task): the hunt multi-town catalog
  selection and the shopping_strategy.md Dion shop section.

### Status: done (2026-09-12 08:56Z) - in-scope

The gear catalog ships within the T-010 scope (gear/, npcdata/,
tools/). The hunt wiring is the follow-up.

### Progress (T-014)

- 08:45 UTC: claimed (BACKLOG T-014 in_progress + the entry above).
- 08:48 UTC: hunt/quest_walker.go landed and pushed - DriveDialog
  (the two-click talk entry, the content-change page wait, the
  hunt-side tracker feed, the link-by-text bypass walk with the
  IsDialogCommand validation) + 10 unit tests on the real Q00406
  and ElfHumanFighterChange1 page goldens through the scripted
  fakeGame wrapper (the html action cache emulation). Build, the
  hunt tests, golangci-lint --new green.
- 09:05 UTC: the live verification round landed and pushed -
  quest_walker_live_test.go (SWARM_LIVE_DIALOG=1, the fleet session
  recipe + the DB position injection through the mariadb CLI
  channel): the character injected at Ellenia's approach ring
  talked to her, the walker matched the "Quest" link (the bare
  `bypass Script` command of the trainer pages), sent it, the
  no-quest answer page arrived and landed in the tracker - 1.7 s
  round trip. The standard mobius_e2e.sh 45 prints E2E_OK. The
  live facts of the round (the glade pages carry no links, the
  SkillList link answers with a packet not a page, the trainer
  page link inventory) live in the dev log round 70.
- Status: done (2026-09-12, 09:10 UTC) - the engine, the tests, the
  live round trip, the docs (hunting.md section, quest_protocol.md
  follow-up list, dev log round 70). Not wired into the tick
  machine (the quest trip phase is the M2 acceptance round's work,
  after the Q00406 chain data task).

## Active task: T-015 the class transfer quest chains as data

Started: 2026-09-12 09:12 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: quest-data-e3f8. Taken after T-014
closed (the walker engine the data feeds).

Goal: the Q00406/Q00407/ElfHumanFighterChange1 chains as Go data
(new files in internal/swarm/hunt/): the npc chain, the cond
progression, the kill grounds (mob ids, item ids, drop chances,
the 20 piece counters), the dialog route steps (the link texts per
stage) and the class change requirements.

Constraints: new files only (T-008 still owns the live edits of
loop.go and the gatekeeper files); every id, chance and counter
cross-checked against the quest script Java sources and the
datapack pages (rule 5 - no guessed server facts); unit tests pin
the data.

Acceptance: the data file + tests green, golangci-lint --new
clean, the quest_protocol.md table cross-checked against the data
(the doc is the research, the data is the executable form).

### Progress

- 09:12 UTC: claimed.

### Progress (T-015, continued)

- 09:30 UTC (the restarted session, the shell outage of 09:25
  recovered): the data round landed and pushed - hunt/quest_chains.go
  (the two chains, the two class changes, the stage constructors,
  QuestStageByCond) + hunt/quest_chains_test.go (6 pin tests: the
  npcdata cross-checks, the ladders, the kill economy, the routes,
  the no-shared-state contract).
- The research correction verified against the Mobius sources: the
  Q00407 cond 2 mob is the Ol Mahum Patrol 20053 (display 53), not
  the Bugbear (npc 20133); quest_protocol.md corrected, the dev log
  round 71 records it.
- Status: done (2026-09-12, 09:40 UTC) - the data, the pins, the
  docs. T-016 (the class transfer acceptance scenario) opens up:
  its deps (T-003 level milestone + T-014 walker + T-015 data) are
  all done; the gatekeeper legs join when T-008 closes.

## Active task: T-009 the 20-25 zone registry generation

Started: 2026-09-12 09:24 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: `zones-zai`. Taken as the top todo of
the queue (T-008 done unblocks it; T-015 closed while this session
deployed, T-017 is in_progress by soak-z in the same hunt/ package -
no file overlap: this task touches the generator, a new generated
zones_dion.go, the zones.go region constant and new test files).

### Goal

The M3 zone registry of the 20-25 band: extend
tools/generate_hunt_zones.py with the Dion spawn sources of the
survey (CrumaMarshlands, ExecutionGrounds, PlainsOfDion) and
generate the spawn-true squares of the band as
internal/swarm/hunt/zones_dion.go in the shape of zones_elven.go
(the survey territories are the input polygons; the band tables of
docs/band_20_25_survey.md are the acceptance reference).

### Constraints

- The gate line of the task (M1 green) follows the T-010 precedent:
  the data preparation lands now, the hunt wiring into the 20-25
  band waits for the M1 operator soak.
- The elven generation mode must stay byte-identical (the committed
  zones_elven.go is regenerated and diffed as the regression gate).
- Every mob id, level and count comes from the spawn XMLs (no
  guessed server facts); the survey tables pin the output.
- The gear gates of the new bands are calibrated against the
  gear.TotalGearPoints probes of the buyable dress stages (NG dress
  284, Falchion dress ~292, Bastard+bone ~312, full D dress ~391).

### Acceptance

go build green, the hunt package tests green, golangci-lint run
--new clean, the generator elven-mode output byte-identical, the
zones_dion.go registry pinned by unit tests against the survey
tables (the mob sets of the grounds, the band ladder, the walking
distances of the survey), the hunting.md section, the dev log
round entry.

### Progress

- 09:24 UTC: claimed (BACKLOG T-009 in_progress + this entry).

## Active task: T-017 the multi-town gear catalog selection

Started: 2026-09-12 09:17Z. Branch: `feature/proxy-server`. Commits as
melg8. Agent label: `soak-z`.

### Goal

The T-010 follow-up: the hunt shopping loop selects the gear catalog
of the town it farms near (elven village default, Dion for the 20-25
band).

### Result

- dionMerchants (hunt/town.go): Sabrin, Casey, Sonia, Lara at their
  survey positions.
- dionShopCatalog + dionTownTaxRate + shopCatalogForRegion
  (hunt/shopping.go): the Dion catalog at the 20 percent Dion tax; the
  selector returns it for regionDion, the elven catalog otherwise.
- regionDion + Loop.zoneRegion + the SetHuntingZoneRegion Dion case
  (hunt/zones.go, hunt/loop.go): the region is recorded on the Loop;
  the Dion case logs the pending zone registry (T-009, gated on M1
  green) and lets the gear catalog selection fire.
- shoppingQueue and shoppingTripEnabled switch to
  shopCatalogForRegion(l.zoneRegion); the elven behavior is unchanged.
- 5 unit tests pin the elven default, the Dion selection, the tax
  rates, the merchant set and the live buylist ids.
- docs/shopping_strategy.md: the Dion shop section.

### Verification

go build green, the full hunt test suite (97 s) green (no M0
regression), golangci-lint run --new: 0 issues. Live verification
deferred: the Dion zone registry (T-009, gated on M1 green) is not
wired, so the bot never enters the Dion region in a live run yet;
the selection is unit-tested against the real npcdata buylists.

### Status: done (2026-09-12 09:40Z)

T-017 complete: the multi-town catalog selection ships. The Dion zone
registry (T-009) and the multi-town spellbook budget (M2 follow-up of
T-015/T-016) stay out of scope.
## Active task: T-016 the class transfer acceptance scenario

Started: 2026-09-12 09:52 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: quest-scenario-e3f8.

Goal: the M2 acceptance vehicle in internal/swarm/acceptance/: the
level 19 injection at Gludio, the quest drive on the T-015 chain
data through the T-014 walker, the class change and the
SelfClassID gate. Staged: this round builds the accept leg (the
journal cond flip of Q00406) - the kill stages need the quest trip
phase (the Gludio band combat wiring), the next round's work.

Constraints: acceptance/ + docs/ scope; the staged PASS line is
the quest journal cond 1 (the honest partial, the metrics row
carries the stage); no false M2 closure (the ladder renders by
the last row - the M2 gate stays the SelfClassID flip).

### Progress

- 09:52 UTC: claimed.

### Progress (T-009)

- 09:24 UTC: claimed (BACKLOG T-009 in_progress + the entry above).
- 09:40 UTC: the generator refactor landed and pushed - the elven
  main became the parameterized generate_registry pipeline
  (parse_territories with the band window filter, the shared fold/
  partition/sort/emit, the spec builders), the --dion flag generates
  zones_dion.go (75 squares over 25 kept territories, 96% spawn mass
  coverage, the five band windows with the gear gates calibrated
  against the TotalGearPoints probes of the buyable dress stages:
  NG 284 / Falchion 292 / Bastard+bone 312 / partial mithril 341 /
  full D 391). The elven mode verified byte-identical against the
  committed zones_elven.go. Build, hunt tests, lint --new green.
- 09:55 UTC: the rebase conflict with T-017 resolved (both rounds
  added regionDion to zones.go) - one comment now carries both the
  registry and the catalog-selection aspects; the pending log of the
  T-017 region case replaced by the DionHuntingZones() install (the
  seam T-017 explicitly left for the registry landing). The wiring
  test added (the region install stands the spot mode down and feeds
  the catalog selection). zones_dion_test.go pins the survey tables:
  the 13 species at their table levels, the sprout/cruma mob sets,
  the anchor walking distances, the picker ladder, the death cap.
  Full hunt suite green (97 s), lint --new 0 issues, pushed.
- 09:33 UTC: the docs round - the hunting.md "The Dion 20-25 band
  registry" section, the dev log round 72, the BACKLOG close. Status:
  done - in-scope. The follow-ups for the next agents: the M1
  operator soak run (the 8h proof that gates the production band
  entry), the band acceptance scenario of the survey follow-up 6
  (a DB-injected level 20 rides the teleport chain and kills its
  first sprout), the spot-registry migration of the band (the elven
  precedent - generate_hunt_spots.py - when the hunt loop moves off
  the square zones).

### Progress (T-016)

- 09:52 UTC: claimed (the staged build: the accept leg first).
- 10:30 UTC: the accept stage landed and PASSED live:
  - acceptance/class_transfer.go: the scenario (temp9, the level 19
    injection at Sorius's approach ring, the manual session seam
    exposing the game client, the Sorius find, the composed accept
    route, the journal flip gate, the metrics rows);
  - the registrations (the Definitions entry, the temp9 account of
    the manager test);
  - hunt/quest_chains.go: QuestEntryLinks (the live-corrected
    entry prefix - the static page Quest link resolves the single
    quest straight to its page);
  - hunt/quest_walker.go: dialogBypassPace (the 3 s bypass flood
    protector of the deployed stack - the second live discovery);
  - live: acceptance PASS ("Quest journal with 1 quests" at the
    accept bypass), e2e E2E_OK, the metrics trail has the FAIL
    discovery row and the PASS row.
- Status: in_progress - the accept stage done, the remaining legs:
  the kill stages (the quest trip phase of the hunt loop: the
  combat at the Ruins of Agony/Ol Mahum camps on the chain kill
  data, the stage loop through QuestStageByCond), the Kluto leg,
  the Rains class change and the SelfClassID + relogin gate. The
  gatekeeper hops (T-008 closed) join when the scenario moves its
  start to the elven village.

### Progress (T-018)

- 09:45 UTC: registered + claimed (BACKLOG T-018, the survey
  follow-up 4 - the queue was empty, every task done or owned).
- 09:43 UTC: the probe ran - both walking legs FOUND (elven ->
  Gludio 111 852 units / 1.25M nodes / 13 s at the raised 40M cap,
  aborts at the shipped 1M; Gludio -> Dion 42 831 units / 0.94M
  nodes, within the shipped cap). The temporary patch and the probe
  test reverted; the findings recorded in the survey doc, the
  navigation analysis (two new route rows) and the dev log round
  73. Status: done.

### Progress (T-019)

- 09:52 UTC: registered + claimed (the T-016 hand-off plan names
  the hunt half the acceptance scope cannot write).
- 10:03 UTC: quest_trip.go landed and pushed - DriveQuestChain (the
  accept + the stage ladder by the journal cond), FindQuestNpc, the
  walk arrival poll, the kill engage with the counters exit. The
  Mobius script source settled the kill stage exit design (the 20th
  piece drop itself sets the next cond - no turn-in livelock). The
  two scripted unit tests + the full hunt suite (99 s) + lint --new
  clean. Status: done - in-scope. The live verification rides the
  T-016 acceptance round (the consumer); the resume note records the
  hand-off: DriveQuestChain(ctx, ElvenKnightChain()) on the manual
  session loop, the Rains class change leg after it.

### Progress (T-020)

- 10:03 UTC: registered + claimed (the survey follow-up 5 - the
  level 24 lesson stall).
- 10:07 UTC: landed and pushed - the two-catalog bookPurchase (the
  village first, the Dion Sonia fallback at her own tax) + the
  merchantByTemplate widening to the Dion stations + 3 unit tests.
  Full hunt suite green (98 s), lint --new clean. Status: done -
  in-scope. The production walk legs are the band wiring (M1 gate).

### Progress (T-021)

- 10:10 UTC: registered + claimed (the committed guard of the
  T-018 finding).
- 10:14 UTC: landed and pushed - the corridor regression test on
  the real geodata pack (found, un-aborted, the length and the
  arrival pinned). The pathfind suite green, lint --new clean.
  Status: done.

## Session close: the T-009 through T-021 rounds (2026-09-12, zones-zai)

The session closed five tasks (all pushed, build/test/lint green at
every push, the E2E gate E2E_OK at the close):

- T-009 the 20-25 band zone registry: the generator --dion mode
  (the elven mode verified byte-identical), zones_dion.go (75
  spawn-true squares over 25 kept territories, 96% coverage, the
  five band windows at the D-grade dress gates), the regionDion
  const + the SetHuntingZoneRegion install closing the T-017 seam,
  the survey tables pinned by 6 unit tests. The rebase conflict with
  T-017 (both rounds added regionDion) resolved honestly.
- T-018 the walking leg verification: both walking legs EXIST
  (elven -> Gludio 111 852 units - aborts at the shipped 1M cap;
  Gludio -> Dion 42 831 units, within the cap). The teleport-only
  assumption of the survey refuted as a hard claim; the findings
  recorded in the survey, the navigation analysis and the dev log
  73. The probe itself was throwaway (the recipe is in the log).
- T-019 the quest trip phase engine: hunt/quest_trip.go -
  DriveQuestChain consumes a QuestChain (the accept, the stage
  ladder by the journal cond, the kill engage until the counters
  fill, the exit drop). The script-verified fact: the 20th piece
  drop itself sets the next cond. 2 unit tests. The T-016
  acceptance round consumes it for the M2 close.
- T-020 the spellbook catalog resolution: the two-catalog
  bookPurchase (the village first, the Dion Sonia fallback at her
  tax), the merchantByTemplate widening - the level 24 lesson stall
  (Cure Bleeding) closed. 3 unit tests.
- T-021 the Gludio-Dion corridor regression test: the T-018 route
  pinned on the real geodata pack.

The queue at the close: T-016 (the class transfer acceptance, the
M2 closer) is in_progress by the other agent - they now own
everything they need (the walker, the chain data, the trip engine,
the class change route). The follow-up field for the next agents:
the M1 operator soak (the 8h run that gates the production band
entry), the cap-as-a-parameter pathfind item (the elven -> Gludio
leg needs it), the spot migration of the Dion band, the web UI
quest widget when a consumer asks.

Status: session complete - the next agent starts from the T-016
close (the M2 acceptance) or the queue it mints.

## Session close: the self-organization system retirement (2026-09-12, owner-direct)

The owner reviewed the claim/lease self-organization loop and judged
it unsuccessful: the queue approach is retired, a replacement
coordination approach will be designed separately. This round removes
the system from the repository.

Removed and cleaned:

- docs/agent_selforganization.md deleted (the design document of the
  loop: the language argument, the verification pyramid narrative,
  the claim/lease protocol, the boot prompt).
- docs/BACKLOG.md deleted (the task queue). All 21 tasks it tracked
  are done except T-016 (in_progress, M2); its hand-off plan is
  preserved below so the in-flight work survives the deletion.
- AGENTS.md: the work protocol no longer points at the queue; the
  goal ladder (docs/ROADMAP.md) stays the definition of progress.
- docs/ROADMAP.md: the intro and the milestone-discipline rule no
  longer reference the removed document or the queue.
- tools/progress_report.sh: the BACKLOG section is gone from the
  page; the milestone ladder, the metrics trail and the commit feed
  remain. PROGRESS.md regenerated.
- runs/README.md, docs/quest_protocol.md, docs/band_20_25_survey.md:
  the dangling references rephrased.
- The historical entries of this journal and of
  docs/development_log.md keep their original wording (append-only
  history); the retirement is this entry and the dev log round 77.

The preserved hand-off of T-016 (the M2 closer, from the deleted
queue entry, verified against this journal's T-016 sections):

- Done so far: the ACCEPT stage PASSED live 2026-09-12 10:30 UTC
  (acceptance/class_transfer.go: temp9, the level 19 injection at the
  Sorius approach ring -13440 122493 -3103, the manual session seam,
  the composed accept route, the journal flip gate); the metrics trail
  holds the FAIL discovery row and the PASS row; the quest trip phase
  engine (hunt/quest_trip.go, DriveQuestChain of the T-019 round) is
  in.
- Remaining: (1) extend acceptance/class_transfer.go to the full run
  (the quest trip phase under the session, the Rains class change
  leg, pass when SelfClassID == 19 plus the character selection packet
  agreement after a relogin); (2) live-verify with
  `-acceptance class-transfer` (CLOSES M2).
- Two live-pinned server facts to respect: the bypass flood protector
  drops unpaced sends (dialogBypassPace handles it inside one
  conversation - a second DriveDialog call must not ride the tail of
  the first) and every quest npc talk starts from the STATIC page.

Status: the next agent resumes the T-016 hand-off above (the M2
acceptance); new-work coordination waits for the owner's replacement
approach.

## Active task: the session journal - the session dump of the long runs (owner-direct)

Started: 2026-09-12 19:00 UTC. Branch: `feature/proxy-server`.
Commits as melg8. The owner asked (Russian) for a dump-state-like but
bigger mechanism: a session journal that records everything about the
bot session from the application start, runs autonomously in logs/,
gets a web UI button for a clipboard export, and is compact enough for
an agent session to digest while answering the long-run questions
(why little money, how many stalls, what fought badly) of the 8-24 hour
runs on the user machine without bothering the user.

### Result

- internal/swarm/session (new package): the Journal (one append-only
  JSONL file per process, 64 MB rotation with background gzip, story
  flood cap 120/min/bot, nil-receiver no-op API), the per-bot
  in-memory aggregator (hourly buckets, level marks, per-mob fight
  stats with a duration histogram, trips, buys, stalls, story ring),
  the compact report renderer (7 sections), the offline journal
  parser (plain + gz, torn-line tolerant) and the 30 s tracker
  sampler.
- state.Bot.SetEventSink: every recorded event mirrors into the
  journal (non-blocking channel send under the tracker lock).
- hunt: kill (with the honest fight length from the new
  fightStartAt/fightStartFor pair - engageAt re-anchors for the stuck
  timeout and could not measure it), death, trip brackets, buy batches
  with the cost, sells, zone switches, stalls, re-paths.
- cmd/swarm: -session-dir (default logs, "" disables), the journal +
  sampler + sink wiring of the single and fleet modes, the lifecycle
  marks (entered/lost/reconnect-wait/shutdown), -session-report FILE
  for the offline post-mortem rendering.
- webserver: GET /api/bots/{id}/session-report + the Session dump
  button of the map toolbar (the same clipboard fallbacks as the state
  dump).
- The double-record fix: the loop logf recorded every Hunt: line twice
  (the logger mirror + NoteAction); logf now calls NoteLastAction
  (last-action view only), the wiring mirror is the single recorder.
- logs/ gitignored; docs/session_journal.md + the docs map entries.

### Verification

- go build, go vet, golangci-lint run --new: 0 issues.
- The full suite green; the session package (14 tests), the webserver
  endpoint tests and the hunt emission tests new.
- Live: two -hunt runs against the deployed stack (STACK_READY). The
  journal recorded 170-206 lines per 2.5-3 minute run (story, kills
  with honest 12.5 s fights, samples, lifecycle); the CLI report
  rendered the full 7-section page from the file; the duplicate Hunt:
  lines are gone.

Status: done (2026-09-12).

## Active task: the session dump fleet gap fix (owner-direct)

Started: 2026-09-12 20:45 UTC. Branch: `feature/proxy-server`.
The owner reported (Russian): clicking the session dump button does
nothing (no clipboard content) and logs/ holds one session file of
0 KB.

### Result

- Root cause of the dead button: the fleet mode never called
  web.SetSessionJournal - the single mode did, the fleet mode only
  opened the journal, so GET /api/bots/{id}/session-report did not
  exist (404) and the button died in its silent catch. runFleet now
  opens the journal before the web interface and registers it exactly
  like the single mode (verified live: both fleet bots answer 200
  with the full report).
- Root cause of the 0 KB file class: the first record (the build
  identity line) sat in the 64 KB bufio buffer until the first 2 s
  ticker, so a process hard-killed inside that window left an empty
  file. The flusher now flushes the first record at once (verified
  live: the file is non-empty at ~100 ms with the build line first;
  pinned by TestJournalFirstRecordFlush below the ticker period).
- The button failures are visible now: console.error plus the report
  endpoint opened in a new tab with the server answer, instead of a
  red flash nobody registers as feedback.
- Test hardening: TestSessionReportRenders window 5 s -> 10 s after a
  starvation flake on the 2-core sandbox with the L2J stack running.

### Verification

- Reproduced both symptoms before the fix (fleet button 404, single
  mode worked); after the fix the fleet repro answers HTTP 200 per
  bot, the journal grows in both modes, graceful shutdown writes the
  shutdown record.
- go build, go vet, golangci-lint run --new: 0 issues; the full
  suite green.
## Active task: the stuck bot - silence watchdog + stagnation recovery (owner-direct)

Started: 2026-09-12 20:37 UTC. Branch: `feature/proxy-server`.
Commits as melg8. The owner reported (Russian) a bot stuck doing
nothing with an attached dump (the file did not survive the session
handover - the upload directory was empty on arrival), asked to find
the cause and fix it.

### Investigation

- Environment deployed fresh (STACK_READY, 75 tables) per the
  mandatory first step; the branch pulled (in sync at c759f9e).
- Reproduction sweep, all green: 13+ min single hunt (kills, rest
  cycles, town trip, gear buys, lessons), 9 min five bot fleet
  (emergency logout cycles, relogins), kill -9 mid farm (relogin
  resumes), game server restart mid farm (backoff + relogin).
- Audit of every wait path: engage (stuck timeout, blind recovery,
  chase progress), flee/panic (budgets + logout), town trips
  (20 min timeout, re-path budget), lessons (windows + retries),
  delevel (60 min bound), spot economy (emptiness switches),
  supervisor (backoff + cooldown honoring).
- Two architectural gaps found: the game session read loop has no
  receive deadline (a half-open/wedged connection blocks forever -
  pings keep "succeeding" into the OS buffer while no packet ever
  arrives), and the stagnation watch only logs (round 64), never
  recovers.

### Result

- connection: the session silence watchdog - gameSilenceTimeout
  (3 min, test seam var) re-armed by every received packet; a silent
  session unwinds with an honest error and the supervisor
  reconnects.
- hunt: the stagnation recovery escalation - the first position
  stall soft-resets the loop state in place (target + skip, loot,
  blind recovery, panic/flee, trip restart, stand up, return
  re-arm), the surviving stall or the XP window (20 min) rebuilds
  the session through emergencyLogoutWithReason ("stagnation ...,
  rebuilding the session"); paced by stagnationHardCooldown (15
  min); the delevel phase exempt; fire counters reset on progress.
- emergencyLogout split into emergencyLogoutWithReason for the
  honest reason line.

### Verification

- go build, go vet, golangci-lint run --new: 0 issues.
- Full suite green (stagnation 14 tests, connection wedged-server +
  quiet-traffic tests new).
- Live: 5 min hunt + 4 min fleet with zero false positives; the
  wedged-server E2E (SIGSTOP the game JVM mid farm) fired the
  watchdog at exactly 3 min, unwound, and relogged + resumed
  farming after SIGCONT (dev log round 80).

Status: done (2026-09-12).

## Active task: the long-run log analysis - tools, behavior fixes, logging gaps (owner-direct)

Started: 2026-09-13 05:15 UTC. Branch: `feature/proxy-server`.
Commits as melg8. The owner attached the six hour fleet journal of the
Windows deployment (logs.7z: session-20260913-015130-28264.jsonl,
62531 records, test1/test2/test3) and asked for three things: the
missing analysis tools (so nobody reads the whole session by hand
again), the long-run behavior problems found and fixed, and the
logging gaps closed.

### Investigation (the journal analysis)

- The exploratory pass over the attached journal found four
  time-wasters and one measurement bug:
  1. 138 emergency logouts (test1: 85, test3: 35, test2: 18): the bots
     re-enter the aggressive spider ground, pile up 3 mobs, logout,
     relogin (median test1 session 82 s) and walk back into the same
     pack. The supervisor booked every one as "game connection lost:
     use of closed network connection" - a lie that hid the loop.
  2. The steering tangent flip-flop: during test2's delevel walk the
     bot ping-ponged 60 minutes between 28732 51927 and 28603 52218
     ("steering the walk around Kaboo Orc Fighter at 29176 52406" x717
     story lines). The waypoint sat 338 units inside the 600 unit
     aggro+clearance circle: no tangent arc can land there, and the
     side flips at every re-issue.
  3. The town trip abort loop: 47 identical "aborted, no walkable path
     to the shop" trips (the dry search refuses the water crossing),
     retried every ~5 minutes for the whole run while a 62214 adena
     shopping plan starved.
  4. The fight clock accumulation: fightStartAt never reset on a kill
     and the Mobius id free list recycles object ids, so camping one
     spawn point accumulated the clock across kills - one lieutenant
     spawn reads "avg 79.9s, max 4369s" over 188 kills; test1's spider
     fights grew 155s -> 1958s monotonically.
- Environment: the fast deploy started (Mobius sparse-clone in
  progress); Go 1.24 unpacked to ~/opt independently so the tooling
  and the fixes build and test while the stack comes up.

### Result (in progress - see the commits)

- session: the -session-anomalies CLI (the ranked findings scanner:
  logout loops, repeated decision lines, trip abort loops, fight
  outliers, death streaks, stalls, gaps, mute - every finding with a
  ready-made drill-down command) and the -session-query CLI (the grep
  of the journal: bot/event/regex/time filters, grep -C style context
  windows, the record cap). The report gained the -from/-to window.
- hunt: the fight clock resets on the kill and the dropped target.
- hunt: the follower skips waypoints inside an idle camp's trigger
  circle (legTargetThreatened) and the walk stuck detection got the
  net-progress watchdog (an oscillation without net progress fires
  the skip/re-path escalation like a standstill does).
- hunt: the emergency logout counts against the zone regression (the
  danger spot ring of the tracker carries it across the session
  boundary, the fresh loop seeds its counters from it), the journal
  gains the structured logout event and the supervisor's lost record
  names the honest reason.
- hunt: the town trip start falls back to the non-dry search (the
  zone return escalation) and the abort streak doubles the trip
  cooldown (5m base, capped at 1h).

### Verification

- go build, go vet: clean; the FULL suite green (the new
  longrun_repro_test.go pins every fix against the observed journal
  numbers), golangci-lint run --new: 0 issues.
- The stack deployed per the mandatory first step (STACK_READY, the
  three ports listening, 75 tables) and the E2E answered E2E_OK.
- Live: a 150 s hunt run against the stack demonstrated the fixes end
  to end - the honest lost record ("Bot failed: emergency logout: HP
  50% under attack"), the structured logout event in the journal, the
  fresh fight clocks (23 s, 18 s, 9 s, 15 s per kill instead of the
  accumulating clock) and the kill positions in the drill-down.

Status: done (2026-09-13). Commits 33c75ee..a3352f0.
## Active task: the memory leak hunt + the memory logging (owner-direct)

Started: 2026-09-13 06:05 UTC. Branch: `feature/proxy-server`.
Commits as melg8. The owner reported (Russian): a slow continuous
memory growth on the long runs (the process memory chart: the heap
baseline creeps up for hours while the bot count, the goroutines and
the packet rate stay flat); asked to find and fix the leak and to log
the used memory at least once a minute, so a growth report is
provable from the logs, not only from the UI charts.

### Investigation

- The static audit of the usual suspects found them all bounded: the
  state rings (events 512, chat 64, combat 64), the stats collector
  rings (2048 with the half on full compaction), the journal
  aggregator (appendCapped everywhere), the pathfind region LRU, the
  object store (dense swap removals, ResetSession clears the world).
- A live fleet soak (24 bots against the deployed stack, two heap
  profiles 5 minutes apart, the same packet load) diffed the
  inuse_space: one retainer - gear.buildCatalogCandidates under the
  hunt shopping refresh chain - held 16.5 MB of the 20 MB growth
  (about a megabyte per bot per minute, the exact chart shape).

### Progress (commit: the process memory logger)

- internal/swarm/memwatch: the process memory logger - one
  "Memory: heap %.1f MB, sys %.1f MB, goroutines %d, gc %d" line a
  minute (DefaultPeriod, the baseline line lands at once), wired into
  the single mode and the fleet mode of cmd/swarm until the shutdown
  context ends. The two guard cases (nil logger, non positive
  period) return at once.
- Verified: go build, go vet, golangci-lint run --new 0 issues, the
  package tests green.

### Result (commit: the gear cache leak fix)

- internal/swarm/gear: the candidateCache and jewelIDCache keys
  replaced the `&catalog` pointer (the address of the by value
  parameter copy - a fresh key per call, two leaked entries per
  replan: a full candidate slice plus the escaped catalog copy) with
  the catalog content hash (the merchants, the tax rates, the buylist
  ids) plus the profile name. Equal content hits one entry, a changed
  catalog rebuilds, the maps stay bounded by the distinct contents.
- shopping_cache_test.go: the regression pins (the same content
  returns the same slice; 50 replans keep both caches at one entry
  per content; every content dimension flips the hash).
- docs/development_log.md: Round 81 carries the full RCA.

### Verification

- go build, go vet, golangci-lint run --new: 0 issues; the full
  suite green (21 packages).
- The repeat live soak (24 bots, the same 5 minute window, the same
  packet load 125k packets): the heap growth dropped from +14.9 MB
  to +1.3 MB (the young session warmup), the pprof diff shows no
  growing retainer anymore.

Status: done (2026-09-13).

## Active task: the map fps chip in the status bar + the cpu load of the open map (owner-direct)

Started: 2026-09-13. Branch: `feature/proxy-server`. Commits as melg8.

### Context (owner request, Russian)

- Move the fps plate off the map canvas into the bottom status bar
  (the app footer) - it overlaps the map corner now.
- The cpu load grew noticeably with the map open. Keep the fps high
  but stop burning the cpu: analyze the render path and offload the
  work where possible (a gpu blit counts).

### Analysis (map.js render path)

- The rAF loop repaints the WHOLE world every frame while anything
  moves (a hunting bot keeps mobs moving almost always): the scaled
  tile blits of a zoomed out view (a dozen+ drawImage per frame),
  the grid pass and the loaded zone frame are camera-only data -
  they cost the same whether the objects moved or not.
- The SSE snapshots (300 ms) each trigger one extra full repaint.
- While the map tab is hidden the loop keeps ticking no-op frames at
  the display rate and update() keeps painting the hidden canvas.

### Plan

1. The fps item moves to the footer (foot-fps, right aligned,
   "fps: N - draw X ms", idle reads "fps: idle").
2. The static world (map/geodata tiles, grid, loaded zone frame)
   rasterizes once into an offscreen canvas anchored in world
   coordinates with one viewport of slack; every frame blits the
   visible slice with one drawImage (a gpu composite). Re-render
   triggers: zoom change, layer toggle, landed tile, theme flip,
   resize, camera leaving the slack box. Sandbox documents without a
   real canvas keep the direct per frame path (the harnesses).
3. The rAF loop stops while the map tab is hidden; the data driven
   repaints (update, kill marks, tile loads) skip the hidden canvas.
   The bot_switch harness stub flips classList.contains to true (it
   paints and asserts painted output - the map is semantically
   visible there, the same stub as the other map harnesses).
4. A new "background cache" scenario in tools/repro_map_render.js
   pins the cache behavior; docs/webui.md gets the update.

### Acceptance

- All five map.js harnesses pass (map_render with the new scenario,
  movement, zone_hover, bot_switch, and the fps scenario on the new
  element), go test ./internal/swarm/webserver green, task fmt:check
  green, task lint:new clean.

### Progress (commit: the chip relocation)

- index.html: the #map-fps chip left the map wrap, the #foot-fps item
  joined the app footer after "updated" (margin-left: auto pins it to
  the right end); style.css swapped the .map-fps block for .foot-fps.
- map.js: fpsChip resolves #foot-fps, the reading reads
  "fps: N - draw X ms" and the idle sentinel "fps: idle".
- tools/repro_map_render.js: the fps meter scenario follows the new
  element and the new text format.
- docs/webui.md: the fps meter paragraph describes the status bar
  placement.

### Progress (commit: the static world cache + the hidden tab guards)

- map.js: the offscreen background cache (bg state, bgKey,
  createBgCanvas, ensureBackground, renderBackground, blitBackground,
  the bgMarginOfView/bgDevicePixels constants). The paint pipeline
  blits the static world (tiles or geodata, the grid, the loaded zone
  frame) and keeps the hunt zones and the kill marks per frame (their
  labels are live data). The sandboxed harnesses fall back to the
  direct static path when no real offscreen canvas exists.
- map.js: the tile load handlers bump tileLoads and repaint through
  redraw; refreshColors bumps colorsRev; resize stores viewDpr.
- map.js: frame() stops the rAF loop while the map tab is hidden and
  the data driven repaints (update, setKillMarks, resetBot, tile
  loads, resize, theme flips) go through redraw() which skips the
  hidden canvas.
- tools/repro_bot_switch.js: the element stub classList.contains
  flipped to true - the harness paints and asserts painted output,
  so its map is semantically visible (the same stub the other map
  harnesses use; the false default predates the visibility guards).
- tools/repro_map_render.js: the canvas stub createElement, a
  drawImage entry on the recording contexts and the new "background
  cache" scenario (7 checks: the raster lands in the cache, a steady
  frame adds no static strokes and exactly one blit, the units keep
  painting, a pan inside the slack only shifts the blit, a zoom
  re-rasters).
- docs/webui.md: the render performance section documents the cache.

### Verification

- All eight harnesses pass (map_render with the background cache
  scenario, movement, zone_hover, bot_switch, hud, stats, gear,
  fight_ui); go test ./internal/swarm/webserver green; task fmt:check
  green; task lint:new 0 issues.

### Progress (commit: the hunt layer cache - the follow-up)

The owner reported the cpu load still there at the zoomed out view
and pointed at the number of hunting circles - confirmed: the elven
spot registry carries 292 grounds (hunt/spots_elven.go) and the far
view keeps them all on screen, so every animation frame paid a
save/restore, two dash array allocations and a full label string
concatenation PER ZONE (the label only draws on hover, but the old
code built it unconditionally).

- map.js: the hunt layer cache (huntBg state, huntKey,
  huntZoneVisualKey, huntLayerKey, ensureHuntLayer, renderHuntLayer,
  blitHuntLayer, the huntDevicePixels constant). The shapes (the
  circles, the squares, the anchor dots, the kill centroid crosses)
  rasterize into a second world anchored offscreen cache exactly
  like the static background (the same slack box pattern) and every
  frame composites them with one drawImage. The visual fingerprint
  keeps the per second economy fields (the respawn countdown, the
  adena rate, the occupancy, the death count) out of the cache key -
  they only feed labels, so a ticking countdown no longer re-rasters
  the registry.
- map.js: drawHuntingZoneDirect replaces drawHuntingSpotCircle and
  drawHuntingZoneRect - one path per style group (all future circles
  in a single stroke call, the heat fills bucketed by the alpha
  bucket, the dots and the crosses batched per group) instead of a
  state round trip per zone; no label work in the base pass at all.
- map.js: drawZoneEmphasis + drawSpotEmphasis/drawRectEmphasis +
  spotLabel/rectLabel paint the hovered or listed zone fresh on top
  of the cached shapes (the thicker stroke, the brighter fill and
  the live economy label) - at most two zones match, a couple of
  shapes per frame. The legacy single square keeps the direct path
  (one shape needs no cache).
- map.js: the kill crosses batch by fade bucket (killFadeBuckets,
  96 marks cost eight strokes instead of ninety six); the aggro
  circles skip the sub pixel radii of the far zoom (radius < 4 px).
- tools/repro_map_render.js: the new "hunt layer cache" scenario (8
  checks: the raster lands in the hunt cache, the steady frame
  re-strokes nothing and composites both caches, the economy only
  update re-rasters nothing, the active zone switch re-rasters, the
  hovered spot carries its live economy label, a zoom re-rasters);
  the "hunt zones view" scenario reads the square strokes from the
  hunt cache record.
- docs/webui.md: the render performance section documents the hunt
  layer cache, the kill cross batching and the aggro sub pixel skip.

### Verification (the follow-up)

- All eight harnesses pass (map_render with the new "hunt layer
  cache" scenario, movement, zone_hover, bot_switch, hud, stats,
  gear, fight_ui); go build ./... and go test ./... green; task
  fmt:check green; task lint:new 0 issues.

Status: done (2026-09-13, the hunt layer cache follow-up closed the
zoomed out cpu load).

### Progress (commit: the tile load storm throttle - the follow-up)

The owner reported the map fps still dips for a while after a
zoom-out while the background elements load, then recovers to the
acceptable 40-50. Root cause: the background cache key carried the
RAW tile arrival counter (bgKey used this.tileLoads), and a zoom-out
starts dozens of tile loads at once - every arrival of the burst
dropped the key, so EVERY animated frame of the load window
re-rasterized the whole static world (the render loop keeps painting
while the tiles stream in; each frame saw the stale key). Once the
burst went quiet the key stabilized and the fps recovered - exactly
the reported window.

- map.js: the arrivals now commit in throttled batches (tileArrived,
  bg.tilesCommitted/commitAt/commitTimer, the tilesCommitMs constant
  of 250 ms). The first arrival of a window commits immediately (the
  leading edge - the first coarse imagery appears at once), the rest
  of the burst coalesces into one trailing commit at the window end:
  a streaming load costs at most a couple of cache rasters per
  second instead of one per animated frame, and the final state
  always lands (the trailing timer fires even with the render loop
  idle and the tab hidden - the commit does not need a paint).
- map.js: the image loads go through img.decode() before the ready
  flag - the jpeg decode of a landed tile happens in the image
  pipeline instead of the first drawImage inside a cache re-render
  (a stack of synchronous decodes was the other cost of a raster
  during the load window). An undecodable bitmap stays un-drawn and
  the ancestor walk keeps the coarser fallback.
- map.js: bgKey rides bg.tilesCommitted instead of the raw counter.
- tools/repro_map_render.js: the sandbox gained a mutable clock
  (advanceClock) and a recording timer queue (runTimers), and the
  checkbox defaults now mirror the page (show-map checked - the stub
  default of false skipped the tile walk entirely). The new "tile
  load storm" scenario (6 checks: the first arrival commits
  immediately, a 24 tile burst inside the window re-rasters nothing
  on its own arrivals nor on a steady frame, the steady frames stay
  blit only, the trailing commit rasterizes the whole burst once,
  the settled cache re-rasters nothing).
- docs/webui.md: the render performance section documents the tile
  commit throttle and the decode hint.

### Verification (the follow-up)

- All eight harnesses pass (map_render with the new "tile load
  storm" scenario, movement, zone_hover, bot_switch, hud, stats,
  gear, fight_ui); go build ./... and go test ./internal/swarm/
  webserver green; task fmt:check green; task lint:new 0 issues.

Status: done (2026-09-13, the tile load storm throttle closed the
load window fps dip of the zoomed out map).
## Active task (status: in progress): the full size contact markers - issue #7, the melee pair slides apart instead of shrinking (2026-09-21, branch feature/contact-touch-no-shrink)

Started 2026-09-21 ~19:11 UTC (the kanban claim of melg8/swarm#7);
the review round 2026-09-21 ~19:52 UTC (the owner feedback on the
first implementation).

The owner ask: "Bot and npcs should not reduce their icon size even
if they get close to each other. When bot fights and it is too
close to enemy both icons should just touch facing each other, but
should not change size." The review ask: a bot and a mob that meet
too tight drifted apart SIDEWAYS (a north-south pair read west-east
while both kept looking north-south) - the facing and the rendered
positions must never mismatch.

Design (verified against the Round 124 contact pass):

- The shrink is replaced by a slide: the contact pass computes per
  frame screen-space OFFSETS (computeContactOffsets) instead of
  radii factors - every overlapping pair keeps both radii and
  pushes apart along the axis that connects the two centers, so
  the circles touch face to face with a 0.5px hair (contactGap).
- Two Gauss-Seidel rounds over the deterministic snapshot order
  (self first, then the sorted objects) so the offsets never
  flicker; a cheap axis-aligned early-out guards the pair loop;
  the per unit drift is capped at 2x its radius so a dense crowd
  stays anchored near its true spot.
- One new resolver (unitScreenPos) feeds every marker-anchored
  visual - the circle body, the look tick, the name band, the
  target rings, the combat floats and swings, the cast plate, the
  social links, the hover hit test - so the whole unit slides
  together. The world-anchored layers (the aggro range circles,
  the kill marks, the walk plans) keep the true positions: they
  are world facts, not unit plates.
- The review round fix: the separation axis of a tight pair comes
  from the LOOK DIRECTION, not the connecting centers. Under the
  new contactAxisEpsilon (3px screen) the center-to-center
  direction of a pair is packet jitter, not geometry - the old
  code split a stacked pair on a fixed west-east fallback and a
  sub pixel residual could aim the slide sideways, which read as
  the icons drifting perpendicular to the facing. contactAxis
  rotates the slide axis into the heading line of the pair (the
  same 65536-step circle the tick renders, so the axis lives in
  exactly the space the tick draws in), blending by the gap
  fraction so a pair wobbling around the epsilon does not pop: a
  pair that faces each other separates along the shared facing
  line with each unit backing away from what it looks at, two
  units facing the same way line up nose to tail, and the units
  without heading data (heading 0) keep the old horizontal split.

Status: the review round implementation complete, all nine web UI
harnesses green (repro_map_render grown with the facing contact
and the facing near contact scenarios - both fail on the pre-fix
map.js, pinning the reported drift), the change is web only, the
branch pushed for the PR review.

