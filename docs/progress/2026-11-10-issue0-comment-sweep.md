# The comment sweep: one concept one word, the fact not the story (status: in progress)

Started 2026-11-10, branch main, issue0 (owner instruction: the code
carries too many long and stale comments, `leg` names things it does
not mean, and the model needs a standing rule).

## Goal

The comment debt is measured and falling, the vocabulary is one word
per concept, and every future session reads the rule before writing a
comment. Done means:

- `AGENTS.md` carries the comment rule and the glossary, so the model
  applies it without being told again;
- `task comment:audit` reports the debt (the share, the blocks above
  8 lines, the history markers) and the baseline is recorded;
- the `leg` vocabulary is fixed where the word names something else
  (`step` for one kite walk, `segment` for a route piece, `len` for a
  length), while the armor `legs` stays (it matches the server);
- the worst files are compressed in their own commits, each keeping
  the load-bearing numbers and dropping the round history the docs
  already carry.

## Context

Measured 2026-11-10 (`python tools/comment_audit.py`):

- 25 958 comment lines of 177 062 (14.7 percent), 723 blocks above 8
  lines, 1 429 history markers (dates, round numbers, issue ids,
  "legacy", shouting);
- the worst files: `hunt/town.go` (1 948 lines, 45.5 percent),
  `hunt/loop.go` (1 425, 54.1), `hunt/kite.go` (1 090, 57.9),
  `state/bot.go` (1 050, 30.0), `gear/shopping.go` (471, 30.7);
- the longest single block: 143 lines at `hunt/kite.go:7` - the file
  header retelling every round from issue #13 to #70.

The facts those comments carry are not lost: `docs/hunting.md` holds
the kite sections (issues #60, #70 and the owner rounds), the protocol
notes hold the server rules, and the round fragments sit in
`docs/progress/archive/`. The compression therefore keeps the
measurable number and points at the document.

The `leg` usage, surveyed over 218 hits: the armor family
(`PaperdollLegs`, `SlotLegs`, `partLegs`, `planLegs`, the gear
planner) matches the server naming and stays; the route and kite hits
are the ones that rename - a route piece is a `segment` (the tree
already has `startWalkSegment`, `townSegments`), one paced kite walk
is a `step` (the tree already has `kiteStep`), and `outLeg` in the
corner probe is a length, not a leg at all.

## Progress

### 2026-11-10 - the tool, the rule, the renames

- `tools/comment_audit.py` and `task comment:audit` measure the debt;
  the baseline above is its first report.
- `AGENTS.md` gains "Comments: the fact, not the story" with the
  eight rules and the glossary table.
- Renamed: `kiteStraightLeg` to `kiteStraightStep` (the kite, the lane
  code and the tests), `auditLegs` to `auditSegments`, `legZ` to
  `segmentZ` and `TestLegZ` to `TestSegmentZ` (the corner probe),
  `outLeg` to `outLen` (it is a distance), `worldLegsVerified` to
  `worldSegmentsVerified` (the counter probe), `fleetRaceLegStream` to
  `fleetRaceStepStream` (the fleet race sampler). The armor `legs`
  identifiers are untouched.

### 2026-11-11 - the kite comments keep the constraint only

- The `hunt/kite.go` comments drop the round history (the dates, the
  run numbers, the issue ids and the stories behind each rule) and
  keep the constraint and the measured number; `docs/hunting.md` and
  `docs/kite_timing_findings.md` carry the rounds.
- Verified: `go build ./...`, the hunt package tests, `task lint:new`
  (0 issues), `task fmt:check`.

### 2026-11-12 - town_walk.go keeps the oracle contract only

- The 51 line `segmentAdvanceClear` doc and the 31 line
  `resolveClickExtension` doc compress to eight and seven lines: the
  gate/click one-oracle contract, the backward hold and the return
  value stay, the plaza and teacher round history drops.
- `docs/hunting.md` (the town walk) stops claiming a geodata line of
  sight gate and names the click transport as that one oracle.
- Verified: `task fmt`, the hunt tests, `task lint:new`.

### 2026-11-12 - loop_state.go keeps the field facts only

- The 521 comment lines of `loop_state.go` (63.0 percent of the file)
  compress to the one fact a field name cannot say: the pacing
  relation, the ownership of a window, the reason a latch exists. The
  struct narratives and the round evidence drop; the field registry
  keeps pointing at the explaining file (`zones.go`, `kite.go`,
  `settle.go`).
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The file leaves the audit top list; the tree share
  reads 14.4 percent, 663 blocks, 1,385 history markers.

### 2026-11-12 - shopping.go keeps the pacing numbers only

- The `shopping.go` comment blocks drop the round evidence and keep
  the server numbers: the 1250 ms transaction window over the 1 s
  flood interval, the 15 s buy confirmation, the 60 s sale timeout,
  the 45 s weapon run cooldown, the 20 percent Dion tax. 49 blocks
  compress in one pass, each still naming the function that owns the
  detail.
- Verified: `task fmt`, the hunt tests, `task lint:new`. The tree
  share reads 14.3 percent, 647 blocks, 1,380 history markers.

### 2026-11-12 - cell_policy.go keeps the economy rules only

- The 22 line section header compresses to eight and points at
  `docs/hunting_cells.md`; the 32 function and constant blocks keep
  the rule (the ripeness clock, the starved cooldown, the unfenced
  emptiness read, the global regression contest) and drop the crowd
  and dump stories.
- Verified: `task fmt`, the hunt tests, `task lint:new`.

### 2026-11-12 - user.go keeps the pacing and ownership rules only

- 33 blocks compress: the 600 ms confirmation gate, the 4 s same-item
  re-send window over the ~1.5 s windup and ~3 s bow cycle, the one
  action per second manual cadence, the cursor key escape ownership
  and the refusal ladder stay; the walk plan and attribution stories
  drop. The pre-consolidation debt note leaves `tickUserAttack`.
- Verified: `task fmt`, the hunt tests, `task lint:new`.

### 2026-11-12 - kite.go keeps the bow timing only

- 42 blocks compress: the 16 line header becomes eight and points at
  `docs/hunting.md` plus `docs/kite_timing_findings.md`; the function
  blocks keep the server facts (the 500000/pAtkSpd + reuse formula,
  the RED gauge outranking it, the 1200 tick attack timeout, the
  1200 ms silent probe, the 250 ms refusal window) and drop the round
  ids and the repeated one-lane story.
- Verified: `task fmt`, the hunt tests, `task lint:new`.

### 2026-11-12 - quest_trip.go keeps the walk numbers only

- 35 blocks compress: the 20 line engine header becomes eight and
  points at `docs/quest_protocol.md`; the blocks keep the 10 min walk,
  the 8000 unit segment over the 1M expansion cap, the 3 s move start
  window, the 55 and 80 percent health floors and the 250 unit
  interaction distance.
- Verified: `task fmt`, the hunt tests, `task lint:new`.

### 2026-11-12 - town_walk.go keeps the transport arithmetic only

- The second pass over `town_walk.go` compresses 33 blocks: the 15
  line `waypointDistanceAnchored` doc and the 22 line
  `clickForwardJump` doc become four lines each, and the arrival and
  pass radii (50 and 150 units), the 100 unit corridor, the 16 unit
  step floor, the rescue floor, the delivery test and the one-oracle
  gate survive. The file leaves the audit top list.
- Verified: `task fmt`, the hunt tests, `task lint:new`. The tree
  share reads 13.9 percent, 586 blocks, 1,330 history markers.

### 2026-11-12 - town_stuck.go keeps the recovery ladder rules only

- 23 blocks compress: the 25 line `refusalEvidence` doc and the 39
  line `escalateFrozenSegment` doc become four lines each; the blocks
  keep the 15 s and 4 s stuck windows, the target-specific refusal
  ladder, the stood-on-cell skip gate, the frozen re-path limit and
  the claimed-walk route contract. `town_stuck.go` leaves the audit
  top list.
- Verified: `task fmt`, the hunt tests, `task lint:new`. The tree
  share reads 13.8 percent, 571 blocks, 1,299 history markers.

### 2026-11-12 - archer_fleet.go keeps the thresholds only

- 30 blocks compress: the scenario header, the threshold table and the
  telemetry narratives keep the numbers (the 3 minute window, the 40 s
  launch budget, the 250 unit range floor, the 2200 ms lag ceiling, the
  400 unit re-shot floor, the 1500 unit leash, the 40 degree turn, the
  1650 ms windup, the 0.35 standing share, the 214 HP / 82 MP vitals)
  and drop the round evidence.
- Verified: `task fmt`, `go build ./...`, the acceptance tests,
  `task lint:new`.

### 2026-11-12 - pathfind/search.go keeps the cost model only

- 26 blocks compress: the water constants, the avoid ban and the step
  rules keep the numbers (the -3780 water surface, the 2.3 swim price,
  the 6x escape price, the 256 unit escape ring, the 40 unit
  HEIGHT_INCREASE_LIMIT, the 8..24 unit ramps, the 7x7 ring, the
  symmetric wall check, the reverse wall and the anti corner cut) and
  the round stories drop. The two stacked `wallsOpen` doc blocks merge
  into one.
- The doc loses two stale facts: `docs/pathfinding.md` read "drops of
  any height are walkable" (the plan gates the drop at 40 too) and
  `waterCostMultiplier` 3x (the code prices 115/50 = 2.3). The same
  paragraph now carries the avoid ban and the escape ring.
- Verified: `task fmt`, `go build ./...`, the pathfind tests,
  `task lint:new`. The tree share reads 13.7 percent, 546 blocks,
  1,267 history markers.

### 2026-11-12 - pathfind/navmesh/hierarchy.go keeps the phase rules only

- 33 blocks compress: the header, the bounded budgets, the coarse guide
  pricing and the fallback ladder keep the numbers (65536 coarse nodes,
  32768 hop nodes, 3 coarse attempts, the 4096 pop gate cap, the 1M
  confined nodes, the 4096 hop cache, the 24 region line prefetch, the
  4x budget retry, the quarter distance progress guard) and the round
  evidence drops.
- The doc loses a stale design note: `docs/navmesh.md` still described
  the uniform coarse guide, while the code prices the water edges since
  the zone aware swim model landed; the bullet now matches
  `hierQuery.waterMultiplier`.
- Verified: `task fmt`, `go build ./...`, the navmesh tests,
  `task lint:new`. The tree share reads 13.6 percent, 540 blocks,
  1,267 history markers.

### 2026-11-12 - state/scans.go keeps the scan contracts only

- 39 blocks compress: the target searches keep the projected position
  rule (the once per second broadcast leaves a moving mob far from its
  packet position), the 1500 unit engage radius against the 200 unit
  priority bias, the 200 unit social help margin and the 600 unit z
  limit, the single sided ALL clan, the 256 bucket median histogram and
  the pooled scan arrays; the round stories drop. The doc facts stay in
  `docs/hunting.md` (the projected ranking, the social fence, the
  priority bias, the delevel median).
- Verified: `task fmt`, `go build ./...`, the state tests,
  `task lint:new`. The tree share reads 13.6 percent, 529 blocks,
  1,262 history markers.

### 2026-11-12 - hunt/zones.go keeps the ladder numbers only

- 29 blocks compress: the header, the picker and the rotation keep the
  numbers (the level lead 1, the 30 s re-pick period, the 10 s rotate
  window, the 40 s empty cooldown, the 3 death limit, the 15-20 s
  respawn, the 1 hour danger window, the CT0 20471 to the wire 1000471
  offset) and the round evidence drops. The zone economy facts stay in
  `docs/hunting.md`.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.5 percent, 522 blocks,
  1,261 history markers.

### 2026-11-12 - hunt/town.go keeps the trip gates and numbers only

- 37 blocks compress: the trip lifecycle keeps the numbers (the 50
  percent slot and weight triggers, the five minute trip cooldown, the
  45 s weapon/guide/debt window, the 10 s skill list wait, the 20 minute
  trip timeout, the 2 tolerated aborts and the doubling cooldown capped
  at the hour, the 2 damage fists, the referencePrice/2 sell pricing)
  and the round stories drop.
- The duplicate "a bot outside the zone returns first" block goes: two
  copies sat in `maybeStartTownTrip`, the first one stale (it did not
  know the weapon and guide exceptions); one gate comment carries the
  rule now.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.5 percent, 509 blocks,
  1,250 history markers.

### 2026-11-12 - hunt/kite.go second pass trims the constant prose

- 29 blocks compress: the constants and the ladder keep the numbers (the
  250 unit retreat radius, the 400 step, the 800 train scan, the 0.3
  encirclement share, the 700 ms shot window, the 250 ms re-click, the
  1200 ms probe, the 250 ms refusal window, the 1350 ms bow retry lead,
  the 1500 reuse delay, the 1 s and 5 s window clamps, the 500000/pAtkSpd
  formula) and the round references drop (the owner round markers, issue
  id).
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.4 percent, 509 blocks,
  1,244 history markers.

### 2026-11-12 - hunt/cell_policy.go second pass keeps the economy only

- 22 blocks compress: the economy keeps the numbers (the 20 s cell wait
  patience, the 40 s no-data patience, the 60 s starvation, the 5 minute
  minimum stay and starve cooldown, the 90 s exhausted stay, the 1.25
  switch margin, the 15 minute death regression, the 8 level floor
  slack, the 500 unit drift floor, the 96 and 400 kill log caps) and the
  issue ids drop.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.4 percent, 508 blocks,
  1,236 history markers.

### 2026-11-12 - hunt/delevel.go keeps the server rules only

- 28 blocks compress: the package header drops from 35 lines to 14, and
  the constants keep the numbers (the trigger difference 7, the target
  gap 5, the Lucky level 9, the guide cubic floor 16, the 60 unit melee
  range, the 3 free deaths, the 30 minute free cooldown, the 20 s fight
  stage, the 5 minute abort backoff growing 5x, the 60 minute timeout,
  the 1100 bow range) with the detail left to `docs/hunting.md`.
- The stale phase-debt note (a pointer to a process document) goes.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.4 percent, 499 blocks,
  1,228 history markers.

### 2026-11-12 - hunt/quest_chains.go keeps the data, the pages move to the doc

- 39 blocks compress or leave: the package header drops from 21 lines
  to 8, the field comments keep the load-bearing facts (the display id
  plus 1000000 offset, the 2.5 s delayed quest drop, the onEvent versus
  onTalk setCond split) and the round stories drop.
- The per stage dialog narrative (page numbers, setCond values, item
  transfers, the accept event, the class change pages) moves to
  `docs/quest_protocol.md`, which now lists both chains' registered
  items and talk pages per cond; the code keeps the values only.
- The doc contradiction found en route: the return Gludin hop is
  Richlin (30320, 7300 adena), not Trisha (30059, the Dion gatekeeper);
  the geography section and the gatekeeper row are corrected.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.3 percent, 498 blocks,
  1,224 history markers; `quest_chains.go` fell from 45.6 to
  35.2 percent (160 comment lines).

### 2026-11-12 - hunt/quest_trip.go keeps the numbers only

- 32 blocks compress: the trip engine keeps its bounds (the 6000 unit
  npc scan, the 30 s find wait, the 10 minute walk, the 8000 unit
  segment under the 1M expansion cap, the 5 s stuck window, the 3 s move
  start, the 2000 unit arrival scatter and 20 s transfer wait, the 55
  and 85 percent rest band, the 80 percent potion line, the 10 s potion
  reuse, the 20 minute kill stage, the 700 ms attack period, the 1600
  unit kill scan, the 250 unit interaction distance, the 20 percent
  health floor, the 2600 unit retreat, the 39 percent measured walk
  arrival) and the round stories drop.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.3 percent, 497 blocks,
  1,223 history markers; `quest_trip.go` reads 20.0 percent.

### 2026-11-12 - hunt/loop_safety.go keeps the rules, drops the dumps

- 22 blocks compress: the flight keeps its rules (one paced segment per
  call, the ordered straight/45/90 lane fan clamped into the hunting
  square, the no-lane and no-progress guards, the fifteen second logout
  combat window, the run anchor distance, the health gate and the pick
  ceiling) and the round dumps drop (the run-31 868 unit sample, the
  run-24 99 percent reset, the 14:02/14:03 dumps collapse into the one
  kept number: a level 10 Kaboo Orc Fighter held ~19 units through 21 s,
  the level 7 archer ran 100 -> 59 percent in sixteen seconds, 85
  logouts on one ground).
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.3 percent, 492 blocks,
  1,219 history markers; `loop_safety.go` fell from 35.2 to
  25.8 percent (152 comment lines).

### 2026-11-12 - hunt/shopping.go keeps the windows, drops the narrative

- 26 blocks compress: the timing facts stay (the 1250 ms transaction
  pause over the 10 game ticks of 10 per second plus 250 ms, the 15 s
  buy confirm wait, the 3 buy retries, the 60 s sell settle, the 5 s
  plan period, the 100 adena trip minimum, the 45 s weapon cooldown,
  the 15 and 20 percent town taxes, the one buylist per request), the
  small talk drops (the "future region config" plan note, the round
  dumps, the retold code).
- The doc contradiction found en route: `docs/protocol_description.md`
  claimed the transaction flood protector was "10 s by default" in two
  places while the server source pins 10 game ticks of 10 per second
  (1 s) with the punishment limit 0; both lines are corrected.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. The tree share reads 13.3 percent, 491 blocks, 1,216
  history markers; `shopping.go` fell from 20.6 to 18.1 percent
  (212 comment lines).

### 2026-11-12 - hunt/loop_params.go keeps the numbers, drops the essays

- 41 constant docs compress: every registry entry keeps its number and
  its invariant (the 250 ms tick, the 2.5 s StandUpTask window, the
  1200 tick attack timeout behind the three minute flee skip, the deck
  gap 400, the 600 unit panic run, the 20 s flee budget, the 8 s gap
  window and 150 unit open margin, the 600 s spawn protection, the
  70 percent potion line above the 60 percent re-engage line) and the
  round dumps, the "reported 180 degree flips", the 2026-09-11 and
  2026-11-10 narratives drop. Six doc lines are re-wrapped under the
  80 column gate; the file fell from 78.2 to 70.0 percent (156
  comment lines), which is the registry floor.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. The tree share reads 13.2 percent, 486 blocks,
  1,209 history markers.

### 2026-11-12 - acceptance/archer_kite.go keeps the contract numbers

- 30 blocks compress: the scenario keeps its numbers (the temp18
  account slot, the 10 minute smoke window, the 90 s shutdown grace,
  the 7-10 cell band and the 8-24 guide window, the 600 arrow quiver,
  the 650 unit bow band plus 130 slack over the 780 ceiling, the 2.97 s
  bow disable at pAtkSpd 337 and the 4 s shot gap, the 6 gap evidence
  floor with the 750 permille share, the 10 percent HP floor, the one
  kill per two minutes floor) and the round history drops (the 2026-11-09
  796 unit sample, the #60 layer note, the "old safe-distance gate").
- The check id block is deleted outright: the labels of
  `archerKiteChecks` already name every check.
- Verified: `task fmt`, `go build ./...`, the acceptance tests, `task
  lint:new`. The tree share reads 13.2 percent, 481 blocks, 1,203
  history markers; `archer_kite.go` fell from 28.3 to 21.0 percent
  (162 comment lines).

### 2026-11-12 - hunt/user.go keeps the pacing numbers

- 23 blocks compress: the manual mode keeps its bounds (the 60 unit
  arrival radius, the 90 s walk and 30 minute cap, the 30 s pickup and
  attack timeouts, the 150 unit approach, the 600 ms confirm window
  and the 4 s re-send window over the ~1.5 s bow windup and ~3 s
  cycle, the 150 unit drop range, the disabled UseItem flood
  protector) and the retold code drops.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. `user.go` fell from 21.7 to 19.4 percent (201
  comment lines); the tree share is re-measured with the next commit.

## Status

The tool, the rule and the renames are in. The worst files are
compressed one commit each: `town.go`, `loop.go`, `kite.go`,
`shopping.go`, `cell_policy.go`, `user.go`, `quest_trip.go`,
`town_walk.go` (twice), `loop_state.go`, `town_stuck.go`,
`acceptance/archer_fleet.go`, `pathfind/search.go`,
`pathfind/navmesh/hierarchy.go`, `state/scans.go`, `hunt/zones.go` and
`hunt/town.go`, `hunt/kite.go` and `hunt/cell_policy.go` (both second
passes), `hunt/delevel.go`, the `hunt/loop_state.go` field registry (the
share of a struct file stays high by nature), `hunt/quest_chains.go` and
`hunt/quest_trip.go` and `hunt/loop_safety.go`. Next:
`hunt/loop_movement.go` (223, 36.6 percent), `hunt/kite.go` (346, 31.1
percent - the largest remaining file) and `hunt/cell_policy.go` (271,
23.7 percent); the share and the block count are re-measured at the end.
