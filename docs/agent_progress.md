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

## Active task: the relogin ground fix and the aggro-aware movement

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

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
