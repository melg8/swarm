# Agent progress archive

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

