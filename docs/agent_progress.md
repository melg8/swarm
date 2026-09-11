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
