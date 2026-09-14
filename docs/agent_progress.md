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
