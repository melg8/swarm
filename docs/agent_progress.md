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

### Status

In progress - the claim commit; the survey starts next.
