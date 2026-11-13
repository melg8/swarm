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

### 2026-11-12 - hunt/loop_movement.go keeps the movement invariants

- 23 blocks compress: the far-walk and patrol pacing (one segment per
  tick, the per second pick), the road fight budget and the flight
  hold, the `9900` unit move request limit, the plan-only return rule,
  the remembered-spot deck resolution and the two live deck numbers
  (the elven city at `z -2992` against the ground zone at `z -3664`),
  the stall window and the refusal split (progressed versus same cell).
  The round history drops (the `2026-09-11` road round, the
  `2026-09-12 01:50`, `2026-09-14 08:42`, `2026-09-14 10:18` and
  `2026-11-10 run-28` dumps, the owner round notes).
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. `loop_movement.go` fell from 36.6 to 21.7 percent
  (107 comment lines).

### 2026-11-12 - hunt/kite.go keeps the timing numbers

- 98 blocks compress: the retreat radius and step, the movement window
  and the four second `2450/500` bow disable band, the eight-step
  streak, the `800` unit train scan and the `1500` unit audit leash,
  the two cadences (the quarter-second loop, the once-per-second
  movement broadcast), the `1200` ms silent-probe and `250` ms
  refusal windows, the `1350` ms bow retry lead and the 1500 reuse
  delay of the bow family. The round history and the retold code drop.
- The blank line between a doc comment and its function is removed
  where it split the pair (six sites).
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. `kite.go` fell from 31.1 to 25.8 percent (264
  comment lines).

### 2026-11-12 - hunt/cell_policy.go keeps the economy numbers

- 54 blocks compress: the 15-20 s elven respawn, the 20/40/60 s
  patience ladder, the 5 minute minimum stay and 90 s starved stay,
  the 1.25 switch margin and the 5 minute starve cooldown, the 15
  minute death regression, the 8 level adena slack, the 500 unit
  drift floor and the 2 s drift period, the 400 mark map cap. The
  field registry keeps its per-field comments.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. `cell_policy.go` fell from 23.7 to 20.5 percent
  (225 comment lines); the tree share reads 13.1 percent.

### 2026-11-12 - hunt/learning.go keeps the request pacing

- 34 blocks compress: the 300 SP trip floor and the 2 s plan re-read,
  the 250 ms send spacing (the server knows no flood protector on
  `RequestAcquireSkill`, the SkillList confirm is the rate limit),
  the 5 s confirm wait and the 3 retry budget, the 45 s teacher wait
  and approach windows, the `INTERACTION_DISTANCE` of 250 and the
  roof-layer Bresenham hazard of the exact-cell click, the 15-20
  percent shop tax. The approach narrative (the roof report, the z gap
  dump) drops; docs/hunting.md already carries the ring and the skip.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. `learning.go` fell from 32.4 to 22.0 percent (128
  comment lines).

### 2026-11-12 - hunt/town_cursor_escape.go moves the escape facts to the doc

- 30 blocks compress: the keyboard-mode move request (mode 0), the
  `ValidatePosition` claim stream, the one step per second cadence,
  the pocket cap and the frame offset of the claimed z, the ground
  re-anchor at the first owed waypoint, the follow patience and the
  settle re-baseline. The escape narrative and the two dumps drop, and
  a new `### The cursor key escape` subsection of `docs/hunting.md`
  carries the moved facts (the refusing cell, the claims, the follow
  probe, the heading convention) in the same commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests,
  `task lint:new`. `town_cursor_escape.go` fell from 44.7 to 26.6
  percent (96 comment lines), no block above 8 lines.

### 2026-11-12 - pathfind/navmesh/query.go keeps the search contract

- 35 blocks compress: the `FindPathApproach` radius contract, the
  `NoNavmeshError` and partial answers, the column containment of
  `FindNearestPoly` (the ClosestLayer z ranking against the staircase
  drift), the water zone cuboids and the `115/50 = 2.3` swim price,
  the `WaterZones` nil fallback, the recovery bans and their escape
  ring, the waypoint clearance and the shortcut pass, the segmented
  `SegmentGuard` raster, the hierarchy escalation against the raised
  node budget of a banned query (the ~85k node diagonal against the
  64k default), the pocket escape and the indexed tile traversal.
- `docs/navmesh.md` gains the banned-query escalation rule (the coarse
  chain cannot see the bans) in the same commit; the rest of the moved
  facts already live there.
- Verified: `task fmt`, `go build ./...`, the navmesh tests, `task
  lint:new` (the rewrapped `RouteApproach` directive keeps its
  `//nolint` form). `query.go` fell from 35.9 to 25.3 percent (128
  comment lines), no block above 8 lines.

### 2026-11-12 - pathfind/navmesh/hierarchy.go keeps the search bounds

- 50 blocks compress: the `42240` cluster nodes of the 165 region map
  against `maxCoarseNodes`, the `32768` hop bound, the three
  `maxCoarseAttempts` ban and replan cycles, the `4096` hop cache
  capacity, the 24 region line prefetch cap, the `1.5*straight+8192`
  give-up band, the one cluster dilation of the confined set against
  the bay detours, the half weights of `corridorCostOf`. The doc
  already carries the priced guide and the gate free HNA* chain
  (`docs/navmesh.md`, the hierarchy section), so the blocks only lose
  the retold code.
- Verified: `task fmt`, `go build ./...`, the navmesh tests, `task
  lint:new`. `hierarchy.go` fell from 17.8 to 16.1 percent (187
  comment lines), no block above 8 lines; the tree share reads 12.9
  percent.

### 2026-11-12 - hunt/loop_avoid.go moves the steering facts to the doc

- 31 blocks compress: the on-sight trigger and the effective aggro
  range, the `avoidClearance` 150 and `avoidScanRange` 1600 windows,
  the `avoidArriveExempt` 500 destination exemption, the tangent
  steering and the inside-the-circle side-step, the skipped waypoint of
  the ping-pong (the 338 of 600 units case), the `combatAvoidWarning`
  150, `combatAvoidStep` 300, the 30/60 degree lane fan, the 2 s
  window, the 4 s pace, the 1 s scan and the 15 s hold log. A new
  `## The transit aggro steering` section of `docs/hunting.md` carries
  the moved facts (the exempt threats, the terrain contract, the
  observed incidents) in the same commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. `loop_avoid.go` fell from 41.4 to 29.5 percent (114
  comment lines), no block above 8 lines.

### 2026-11-12 - hunt/stagnation.go moves the watch facts to the doc

- 15 blocks compress: the `20` minute experience window and the `10`
  minute position window with their calibration (the 2.2 min weapon
  round, the 14m54s readiness round, the hour long frozen cell), the
  `15` minute rebuild cooldown, the event fan out (logger, journal,
  action note), the soft reset contents and the hard reset cure, the
  delevel exemption, the offline and manual baseline reset, the
  experience-first ordering and the 2026-09-14 dump it answers. A new
  `## The stagnation watch` section of `docs/hunting.md` carries the
  moved facts in the same commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. `stagnation.go` fell from 47.6 to 28.8 percent (57
  comment lines), no block above 8 lines.

### 2026-11-12 - hunt/quest_walker.go carries the current arrival gate

- 16 blocks compress: the 5 s page wait and 250 ms poll, the one
  action per second talk pace, the 3.2 s bypass pace against the three
  tick `FloodProtectorServerBypassInterval`, the `AnswerIsEffect`
  last-step exemption, the containment link match, the final page
  apply order. The doc was stale here: it still described the old
  content change arrival signal, so `docs/hunting.md` now carries the
  arrival generation gate, the issue #35 stale page case, the
  byte identical re-send, the paces and the INTERACT pull in the same
  commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. `quest_walker.go` fell from 48.5 to 30.5 percent (68
  comment lines), no block above 8 lines.

### 2026-11-12 - pathfind/navmesh/pocket.go moves the escape facts to the doc

- 20 blocks compress: the file header (41 lines) and the sealing
  geometry, the `pocketMaxSide` 2048 bound, the `pocketExitRadius` 768
  ring, the 8x water price (the 936 unit deck sandwich bed) and the 4x
  foreign ban price, the `pocketExitZWindow` 1024 and the one unit
  horizontal floor (the deck at 32 against the honest edge at 36), the
  aim march depths of 288/256/224 past the 150 unit arrival slack, the
  horizontal march, the island and start footprint guards, the flood
  box abort and the component rect guard. The `docs/navmesh.md` pocket
  bullet grew three entries carrying the moved facts (the 52 and 29
  polygon terraces, the 640x752 and 592x432 boxes, why the
  walk-what-you-can partial does not serve them, the constants) in the
  same commit.
- Verified: `task fmt`, `go build ./...`, the navmesh tests, `task
  lint:new`. `pocket.go` fell from 41.4 to 24.5 percent (75 comment
  lines), no block above 8 lines.

### 2026-11-12 - hunt/click_frame.go moves the frame facts to the doc

- 4 blocks compress: the 33 line file header (the nearest height layer
  resolution of the click, `isCompletelyBlocked`, the `getValidLocation`
  collapse, the 872 unit deck over water gap, the 164 unit teacher hall
  flip margin), the ground truth pair of the offset, the 864 unit rises
  of the Gludio segment run and the `frameOffsetLimit` 500 cap with its
  164/436/872 measurements. A new `## The server frame click transport`
  section of `docs/hunting.md` carries the moved facts in the same
  commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. `click_frame.go` fell from 73.7 to 48.7 percent (19
  comment lines), no block above 8 lines.

### 2026-11-12 - pathfind/capsule.go moves the post pass facts to the doc

- 26 blocks compress: the 31 line file header, the 7.5 capsule radius
  and the 16 unit cell slack, the `capsuleMaxDistance` 32 cap, the
  sampling step 4, the push headroom 0.5, the anchor spacing 16 and cap
  64, the push steps 12 and damping 0.75, the bend depth 4, the
  two-phase `ApplyPath` contract, the ridge convergence of `pushClear`,
  the perpendicular first anchor push, the `ShortenPath` fold with its
  window 32 and the `waterMoveSegment` 700 clamp split. The
  `## Capsule clearance` section of `docs/pathfinding.md` carries the
  moved facts (the two phases with every constant, the fold contract
  and the water blind grid oracle) in the same commit.
- Verified: `task fmt`, `go build ./...`, the pathfind tests, `task
  lint:new`. `capsule.go` fell from 30.4 to 21.5 percent (105 comment
  lines), no block above 8 lines.

### 2026-11-12 - hunt/walk_params.go keeps one line per parameter

- 32 blocks compress: the whole 33 constant table to one or two lines
  each, with every value and identifier preserved (the const pairs were
  diffed against the previous revision). The deeper rationale (the 9900
  unit refusal, the 30 unit collapse gate, the flow aim span, the stuck
  ladder) already lived in `docs/hunting.md`; the missing numbers - the
  3 s move start watchdog, the 192 unit refusal variant, the cursor
  escape 144/5/64/2500/3/2 s set, the 16/5 short click extension, the
  100 unit shortening floor and the 15 unit hop - moved into a new
  walk parameter bullet of the same commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. `walk_params.go` fell from 81.8 to 69.1 percent (96
  comment lines), no block above 8 lines.

### 2026-11-12 - pathfind/navmesh/smooth.go keeps the pass contract only

- 9 blocks compress: the 25 line shortcut pass header (the portal span
  crossing, the clearance radius, the merged answer that never adds a
  turn), the epsilon, the wall span cache and the complement derivation
  (the NSWE wall and the climb limit), the side segment mapping, the
  complement gaps and the Ericson segment distance. The
  `## The shortcut pass` section of `docs/navmesh.md` already carried
  the rules and gained the `smoothEpsilon` float rounding sentence in
  the same commit.
- Verified: `task fmt`, `go build ./...`, the navmesh tests, `task
  lint:new`. `smooth.go` fell from 30.3 to 14.0 percent (30 comment
  lines), no block above 8 lines.

### 2026-11-12 - gear/burst.go keeps the burst contract only

- 14 blocks compress: the 28 line burst header (the disabled UseItem
  flood protector, the shared `PacketExecutor` thread pool, the
  overlapping write sets), the `PlannedEquip` write set, the
  `BurstUpgrade` prefix contract and its termination bound, the
  conflict cut, the `expectedWriteSlots` read-ahead, `ServerWriteSlots`
  and the four displacement cases of `equipWriteSlots`. The
  `## Auto equipment` section of `docs/hunting.md` already carried the
  two source facts and gained the four displacement cases in the same
  commit.
- Verified: `task fmt`, `go build ./...`, the gear tests, `task
  lint:new`. `burst.go` fell from 33.4 to 23.5 percent (60 comment
  lines), no block above 8 lines.

### 2026-11-12 - gear/shopping_candidates.go moves the guard facts to the doc

- 22 blocks compress: the 29 line `viableCandidates` header (the family
  copy count, the 883 adena Short Sword versus the 62k top tier
  incident, the gain dominance measure, the `ladderTop` persistence and
  the score descending scan order), the 21 line `catalogCandidates`
  cache header (the 17 MB fleet run, the by value pointer key and the
  megabyte per bot per minute leak), the `walkView`, `shopStrategy` and
  `classify` contracts, the jewel floor gate, the candidate and jewel
  caches, the `catalogHash`, `bestPick`, `slotMarks`, `floorOffer`,
  `aspiredAbove`, `familyCopies`, `FamilyCopiesOf` and `purchaseGain`
  comments. The top-tier guard bullet of `docs/shopping_strategy.md`
  gained the moved facts and a new `## The candidate caches` section
  carries the cache history in the same commit.
- Verified: `task fmt`, `go build ./...`, the gear tests, `task
  lint:new`. `shopping_candidates.go` fell from 28.5 to 18.7 percent
  (111 comment lines), no block above 8 lines.

### 2026-11-12 - hunt/town_segment.go moves the segment ladder to the doc

- 12 blocks compress: the 26 line `startWalkExactSegment` header (the
  approach zero customer cell answer, the 147-232 unit ring report, the
  foreign deck fallback), the npc rung ladder, the `stopNpcArrivalMet`
  handoff (the 64 unit pack disagreement, the creation building
  interior), the priced approach planners, the `replanTownWalkSegment`
  contract, the `armTownWalkSegment` frame measurement and the refusal
  budget re-arm. `docs/hunting.md` gained the exact merchant segment
  bullet (the ring fallback failure class, the exact final approach
  gate) and the npc arrival handoff bullet in the same commit.
- Verified: `task fmt`, `go build ./...`, the hunt tests, `task
  lint:new`. `town_segment.go` fell from 40.0 to 29.2 percent (109
  comment lines), no block above 8 lines.

### 2026-11-12 - acceptance/plaza_herbiel.go keeps the repro contract

- 8 blocks compress: the 26 line scenario header (the ping-pong report,
  the farm readiness start state, the pass contract), the plaza cell,
  the corridor sample, the interaction ring, the timeout, the reset and
  the scenario contract. The header keeps the 63 unit ping-pong and the
  pass contract; the two stale "frozen corridor bans" mentions of
  `docs/hunting.md` and `docs/navmesh.md` now record that the session
  bans are gone (the avoiding form stays for the library contract).
- Verified: `task fmt`, `go build ./...`, the acceptance tests, `task
  lint:new`. `plaza_herbiel.go` fell from 29.1 to 18.4 percent (38
  comment lines), no block above 8 lines.

### 2026-11-12 - pathfind/click_validate.go moves the server rules to the doc

- 5 blocks compress: the 24 line `ValidateClick` header (the cancel
  mechanism, the far click rule, the Bresenham line, the climb limit and
  step-over, the anti corner cut, the final layer match), the raster
  difference against the search supercover, the height resolution, the
  anti corner flanks and the step-over escape. A new `## The server
  click validation port` section of `docs/pathfinding.md` carries the
  port rules and the four answer classes in the same commit.
- Verified: `task fmt`, `go build ./...`, the pathfind tests, `task
  lint:new`. `click_validate.go` fell from 23.5 to 18.3 percent (59
  comment lines), no block above 8 lines.

### 2026-11-12 - cmd/counterprobe/world.go keeps the derivation rules

- 17 blocks compress: the 24 line world derivation header, the curated
  stand map, the scan radii, the region cache, the flat region table,
  the stand struct, the verifier header and its two gates, the segment
  check, the derivation entry and its three stages, the candidate
  resolver, the run loop header, the report routing and the merchant
  row. The `docs/hunting.md` world stand round bullet now carries the
  concrete rules: the 64 unit heading push, the 48..240 ring bands in
  16 unit steps, the 60 degree facing sector first, the 250 unit
  distance and 96 unit z gates, the 200/400 unit neighborhood starts
  and the two way cap.
- Verified: `task fmt`, `go build ./...`, `go vet
  ./cmd/counterprobe/`, `task lint:new`. `world.go` fell from 21.5 to
  14.8 percent (61 comment lines of 412), no block above 8 lines.

### 2026-11-12 - hunt/lure.go gets its own doc section

- 29 blocks compress: the 24 line lure header, the seven constant
  notes, the phase and state notes, the begin/tick entry points, the
  three phase ticks with their confirmation gates, the abort and clear
  paths, the tool resolution, the cover detection and the standoff
  search. A new `## The covered pick lure` section of `docs/hunting.md`
  carries the cover detection, the 420 standoff and its eight
  direction search, the arm pacing, the pull semantics and the 200
  unit arrival swap.
- Verified: `task fmt`, `go build ./...`, the five lure tests, `task
  lint:new`. `lure.go` fell from 21.6 to 15.5 percent (72 comment
  lines of 465), no block above 8 lines.

### 2026-11-12 - hunt/town_sell.go keeps the transaction rules

- 23 blocks compress: the phase entry, the trip tick with the teacher,
  guide, sell and buy branches, the merchant handler, the approach walk
  with the 3D gate, the deck window, the pull ladder, the selection,
  the junk filter and the batch seller. The docs gained `The merchant
  approach gates` bullet: the 200 unit ring around the npc approach
  point, the 250 unit 3D interaction distance and the bounded deck
  window.
- Verified: `task fmt`, `go build ./...`, the merchant and sell tests,
  `task lint:new`. `town_sell.go` fell from 35.6 to 24.2 percent (90
  comment lines of 372), no block above 8 lines.

### 2026-11-12 - gear/shopping.go points at the strategy doc

- 14 blocks compress: the shop struct and its tax note, the seven
  purchase field notes, the two limits, the phase planner header (the
  weapon milestone, the armor set, the jewel floor, the shield), the
  queue planner and the phased walk. The doc gained `The purchase
  entries and the shop widget`: the gain scoring, the eight entry tail,
  the SellFirst/SellCredit contract, the OwnedStack staleness rule and
  the tax sanity bound.
- Verified: `task fmt`, `go build ./...`, the gear tests, `task
  lint:new`. `shopping.go` fell from 60.8 to 45.8 percent (49 comment
  lines of 107), no block above 8 lines.

### 2026-11-12 - hunt/loop_zone.go keeps the deck gate

- 8 blocks compress: the fence, the held ground, the enemy visibility
  (the 23 line deck gate header), the target leash, the inside check
  and the default square. `docs/hunting_cells.md` gained `The deck
  gate` bullet: the 400 unit z threshold, the collapsed click, the
  2026-09-14 terrace dump, the unresolved z pass and the missing self
  position fallback.
- Verified: `task fmt`, `go build ./...`, the zone and cell tests,
  `task lint:new`. `loop_zone.go` fell from 45.2 to 33.1 percent (42
  comment lines of 127), no block above 8 lines.

### 2026-11-12 - webserver/navmesh_geometry.go loses the stale bilinear note

- 20 blocks compress: the 23 line payload header, the magic word, the
  record, link and wall strides, the height step cap, the encoder and
  its two block builders, the overlap window, the border targets, the
  wall writers and the quantizers. The header claimed the bilinear
  surfaces that the exact square port removed; it now records the flat
  quads at the exact cell heights and points at `docs/navmesh.md`.
- Verified: `task fmt`, `go build ./...`, the webserver tests, `task
  lint:new`. `navmesh_geometry.go` fell from 29.0 to 21.5 percent (107
  comment lines of 498), no block above 8 lines.

### 2026-11-12 - hunt/kite_ledger.go loses the issue story

- 5 blocks compress: the 22 line ledger header (the owner complaint,
  the issue id and the four bucket stories), the noise bound, the tick,
  the bucket naming and the close. A new `### The kite motion ledger`
  subsection of `docs/hunting.md` carries the four buckets, the 2 s
  evidence bound and the summary contract.
- Verified: `task fmt`, `go build ./...`, the kite tests, `task
  lint:new`. `kite_ledger.go` fell from 29.8 to 17.5 percent (25
  comment lines of 143), no block above 8 lines.

### 2026-11-12 - hunt/kite_lane.go keeps the straight ray rule

- 21 blocks compress: the lane header, the rung count, the chaser
  weighing notes, the contact ray (the 21 s same-cell stand), the gap
  bisector, the train direction, the degenerate scene, the rung
  ladder, the escape angles (the walled west ray spot), the lane
  resolution, the terrain contract, the anchor leash and the
  half-plane gate. No docs change: `docs/hunting.md` already carries
  the straight ray rule, the rung ladder, the escape angles, the
  half-plane guard and the contact scene.
- Verified: `task fmt`, `go build ./...`, the kite tests, `task
  lint:new`. `kite_lane.go` fell from 43.7 to 27.6 percent (90
  comment lines of 326), no block above 8 lines.

### 2026-11-12 - hunt/city_shopping.go keeps the errand facts

- 45 blocks compress: the 21 line errand header (the route, the stock
  and the tax story now point at the doc), the errand name, the item
  ids, the tax, the six pacing bounds, the two transfer pairs, the
  stops and the plan notes. `docs/hunting.md` bounds bullet gained
  `cityShopPendingWait` 5 s and `cityShopPoll` 500 ms.
- Verified: `task fmt`, `go build ./...`, the 15 city shop tests,
  `task lint:new`. `city_shopping.go` fell from 26.4 to 18.2 percent
  (119 comment lines of 655), no block above 8 lines.

### 2026-11-12 - hunt/town_merchants.go keeps the stand resolution

- 11 blocks compress: the elven and Dion merchant headers, the 21 line
  stand table header, the eight per merchant row notes, the stand
  resolution, the region selection, the merchant template list, the
  approach point and the exact range gate. `docs/hunting.md` gained the
  `NPCDisplayOffset` clause on the tracker templates.
- Verified: `task fmt`, `go build ./...`, the full hunt package,
  `task lint:new`. `town_merchants.go` fell from 40.6 to 29.5 percent
  (57 comment lines of 193), no block above 8 lines.

### 2026-11-12 - acceptance/delevel.go hands the scenario state to the doc

- 21 blocks compress: the 21 line scenario header, the start level, the
  ground cell, the progress and return bounds, the no retry default and
  its env knob, the timeout, the six check notes, the outfit, the
  reset, the run entry, the observer and its two stages and the watch.
  A new `### The delevel acceptance scenario` subsection of
  `docs/hunting.md` carries the injected character, the ground cell and
  its median, the six checks, the 1..3600 second knob and the 25
  minute bound.
- Verified: `task fmt`, `go build ./...`, the acceptance package, `task
  lint:new`. `delevel.go` fell from 25.1 to 18.2 percent (72 comment
  lines of 395), no block above 8 lines.

### 2026-11-12 - pathfind/navbuild/sheet.go keeps the island drop

- 9 blocks compress: the sheet decomposition header, the three struct
  field notes, the bucket note, the flood header, the 19 line island
  drop header, the delta tables and the bucket helper. No docs change:
  `docs/navmesh.md` already carries the sheet decomposition, the island
  drop and its measured numbers.
- Verified: `task fmt`, `go build ./...`, the full navbuild package,
  `task lint:new`. `sheet.go` fell from 21.0 to 13.7 percent (34
  comment lines of 249), no block above 8 lines.

### 2026-11-12 - pathfind/navmesh/abstract.go hands the map arithmetic to the doc

- 40 blocks compress: the 21 line hierarchy header, the constant notes,
  the edge and class struct notes, the component and sidecar stat
  fields, the build entry (the dangling paragraph and the truncated
  nolint reason are gone), the cache bound, the sidecar loader, the
  checksum helpers and the edge derivation helpers. `docs/navmesh.md`
  gains the three level arithmetic (165 regions, ~50M level 0
  polygons, 42240 cluster nodes, 165 region nodes) and the edge dedupe
  bullet; `docs/fastpath_research.md` names the two 512 byte checksum
  windows.
- Verified: `task fmt`, `go build ./...`, the full navmesh package,
  `task lint:new`. `abstract.go` fell from 26.1 to 17.5 percent (108
  comment lines of 617), no block above 8 lines.

### 2026-11-12 - huntaudit/audit.go packs the audit origin into one sentence

- 5 blocks compress: the 23 line package doc, the character constants,
  the spot wait, the default z and the offline poll. The 62 unit leash
  miss moves to the audit paragraph of `docs/hunting_cells.md`; the
  package doc keeps the recipe, the reason and the resume contract.
- Verified: `task fmt`, `go build ./...`, the huntaudit package, `task
  lint:new`. `audit.go` fell from 18.3 to 15.6 percent (94 comment
  lines of 604), no block above 8 lines.

### 2026-11-12 - cmd/navmesh-export moves the dump layout to the doc

- 2 blocks compress: the 22 line package doc and the probe note. The
  binary layout (the header fields, the poly and link records, the
  skipped link count) moves to section 7 of `docs/fastpath_research.md`,
  next to the harness round it serves.
- Verified: `task fmt`, `go build ./...`, `go vet` (the tool has no
  tests), `task lint:new`. `main.go` fell from 13.5 to 8.4 percent (26
  comment lines of 309), no block above 8 lines.

### 2026-11-12 - cmd/navpack-verify trims the corpus prose

- 19 blocks compress: the 21 line package doc (the modes, the corpus
  wire and the gate semantics already live in `docs/navmesh.md`), the
  version and verdict notes, the four corpus record notes, the hash,
  the query runner, the anchor set, the corridor, sample and surface
  point notes and the three mode runners. The broken SPDX email line
  is fixed.
- Verified: `task fmt`, `go build ./...`, the cmd/navpack-verify
  package, `task lint:new`. `main.go` fell from 16.5 to 11.5 percent
  (72 comment lines of 627), no block above 8 lines.

### 2026-11-12 - acceptance/botlog trims the package story

- 10 blocks compress: the 21 line package doc (the layout doc already
  carries the problem and the intended use), the writer tuning, the
  header and generation notes, the queued record, the nil log
  contract, the open, the write loop, the entry renderer and the
  queue handoff.
- Verified: `task fmt`, `go build ./...`, the botlog package, `task
  lint:new`. `botlog.go` fell from 24.4 to 17.9 percent (59 comment
  lines of 329), no block above 8 lines.

### 2026-11-12 - cmd/counterprobe/detect.go hands the detector to the doc

- 2 blocks compress: the 21 line detector header and the printer note.
  A new `### The counter direction detector` subsection of
  `docs/hunting.md` carries the stall edge rule, the 8 cell window,
  the heading vote and the one cell customer offset.
- Verified: `task fmt`, `go build ./...`, `go vet` (the tool has no
  tests), `task lint:new`. `detect.go` fell from 28.6 to 14.8 percent
  (13 comment lines of 88), no block above 8 lines.

### 2026-11-12 - hunt/recovery_heal.go leans on the self heal doc bullet

- 8 blocks compress: the 20 line flow header, the flight margin, the
  skill pick, the in-flight check and the two rest branches. No docs
  change: the self heal recovery bullet of `docs/hunting.md` already
  carries the server refusals, the mana gate and the spawn protection
  interaction.
- Verified: `task fmt`, `go build ./...`, the full hunt package, `task
  lint:new`. `recovery_heal.go` fell from 36.2 to 19.4 percent (25
  comment lines of 129), no block above 8 lines.

### 2026-11-12 - hunt/navmesh_navigator.go and the stale fallback doc

- 16 blocks compress: the 20 line integration header, the navigator
  struct, the constructor, the filter, the four query entries, the
  mesh mapper, the mesh waypoint and position helpers, the pocket exit
  sweep, its distance, the refinement and the avoid conversion.
- The live integration section of `docs/navmesh.md` was stale: it
  still claimed the grid engine fallback and the engine confirmation
  of every partial, while the navigator is the sole route planner.
  The rewritten bullets say the mesh plans every route, a dropped
  destination answers not found and the caller walks the direct server
  route, and the partial surfaces without an engine run.
- Verified: `task fmt`, `go build ./...`, the full hunt package, `task
  lint:new`. `navmesh_navigator.go` fell from 32.6 to 21.5 percent (60
  comment lines of 279), no block above 8 lines.

### 2026-11-12 - hunt/equip.go hands the execution model to the strategy doc

- 20 blocks compress: the 20 line manager header, the field notes, the
  action period and retry delay, the keep set and lure keeps, the bow
  query, the quiver arming, the burst runner, the starter destroy and
  the inline gate notes. A new `## The equip execution` section of
  `docs/shopping_strategy.md` carries the write set model, the UseItem
  flood protector fact, the 2 s fixed-window spacing, the starter set
  flags, the quiver arming and the keep set.
- Verified: `task fmt`, `go build ./...`, the full hunt package and the
  gear package, `task lint:new`. `equip.go` fell from 38.5 to 24.6
  percent (82 comment lines of 333), no block above 8 lines.

### 2026-11-13 - packets/from_game_server/user_info.go moves the wire layout to the doc

- 7 blocks compress: the 20 line packet header, the paperdoll slot
  count, the skipped field groups, the parser note and the three
  inline skip notes. A new `### UserInfo (0x04) wire format` subsection
  of `docs/protocol_description.md` carries the full field layout and
  the base speed rule (the server divides the real speeds by the move
  multiplier, so the effective speed is `runSpd * moveMultiplier`).
- Verified: `task fmt`, `go build ./...`, the from_game_server
  package, `task lint:new`. `user_info.go` fell from 23.4 to 11.9
  percent (19 comment lines of 160), no block above 8 lines.

### 2026-11-13 - pathfind/search.go hands the step rules to the pathfinding doc

- 50 blocks compress: the score constants, the water and escape
  prices, the node key and the search struct field registry, the layer
  and node resolution, the run and the A*, the direct line and its
  three dry forms, the goal test, the cost and obstacle prices, the
  step and wall rules with the diagonal flanks, the heuristic, the
  neighbours and the raster, the server segment port, the smoothing,
  the avoid helpers and the heap notes.
- `docs/pathfinding.md` gains the wall proximity price (`ring/obstacles`
  over the 7x7 ring) and the direct dry line rule of `directOrAstar`.
- Verified: `task fmt`, `go build ./...`, the full pathfind package,
  `task lint:new`. `search.go` fell from 21.7 to 16.9 percent (141
  comment lines of 832), no block above 8 lines.

### 2026-11-13 - pathfind/navmesh/tile.go repairs the shuffled nolints

- 50 blocks compress: the 12 line package doc, the geometry and format
  constant notes, the area and side notes, the tile and grid struct
  notes, the poly, link and node field notes, the accessor notes, the
  three decoder entries, the span packer, the grid builder, the encoder
  and the v3 decoder notes.
- Four `//nolint:dupl` blocks carried a scrambled reason (the
  continuation line sat above the directive, leaving a truncated
  sentence); each is one coherent line now.
- `docs/navmesh.md` gains the `PolyRef` packing (column 16 bits at 48,
  row at 32, index plus one low) and the negative `To` external target
  encoding.
- Verified: `task fmt`, `go build ./...`, the full navmesh package,
  `task lint:new`. `tile.go` fell from 20.2 to 14.3 percent (132
  comment lines of 926), no block above 8 lines.

### 2026-11-13 - state/diagnostics.go keeps only the field reference

- 53 blocks compress: the header, the two struct field registries (the
  Diagnostics fields and the 21 HuntDiagnostics fields), the object
  tally, the errand, the packet window, the two action recorders and
  the locked helpers. The removed field prose lives in the
  `## Snapshot encoding` section of `docs/webui.md`, which gains the
  walk and kill ETA fields and the three delevel fields.
- Verified: `task fmt`, `go build ./...`, the state and webserver
  packages, `task lint:new`. `diagnostics.go` fell from 42.1 to 19.2
  percent (56 comment lines of 291), no block above 8 lines.

### 2026-11-13 - the proxy split files keep the contracts only

- The five files of the proxy game split (`game.go`,
  `game_client.go`, `game_relay.go`, `game_switch.go`, `game_view.go`)
  and `handoff.go` drop 14 blocks: the connection overview, the transit
  and relogin narratives, the restart dance history, the position
  report story and the veto handler chain. The static-key handshake,
  the one-char emulation, the keepalive feedback loop, the position
  band and the restart safety stay as one to eight line contracts,
  each pointing at the matching `docs/proxy.md` section.
- Verified: `task fmt`, `go build ./...`, the proxy tests,
  `golangci-lint run ./internal/swarm/proxy/...` 0 issues; no block
  above 8 lines in the six files.

### 2026-11-13 - loop_state.go, the third and last field pass

- The struct registry keeps one fact per field and drops the prose
  around it: 268 comment lines of 573 (46.7 percent) fall to 215 of
  520 (41.3 percent), and adjacent fields of one owner share a block
  (the impending-add pacing trio, the two hold logs, the cornered
  hold, the re-click ladder). Every pacing relation, window owner,
  latch reason and pointer to the owning file stays.
- Verified: `task fmt`, `go build ./...`, the hunt suite,
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - kite.go keeps the server rules only

- 104 blocks compress, the file falls from 264 comment lines of 1024
  (25.8 percent) to 214 of 976 (21.9 percent): the constant registry
  keeps one line per number instead of a paragraph around it, the
  predicate doc comments state the predicate, and the inline notes keep
  only the server rule they name (the windup replay, the ActionFailed
  attribution window, MAX_ATTACK_TIMEOUT, the bow disable formula, the
  RED SetupGauge priority). The design prose stays in
  docs/hunting.md and docs/kite_timing_findings.md.
- Verified: `task fmt`, `go build ./...`, the hunt suite (27.9 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues, no line above
  80 columns.

### 2026-11-13 - quest_trip.go, the trip engine

- 74 blocks compress, the file falls from 228 comment lines of 1140
  (20.0 percent) to 180 of 1093 (16.5 percent): the constant registry
  keeps the number and the one server rule it names (the 37,500 unit
  Gludio segment against the 1M search cap, the 70-80 percent retreat
  bar, the 10 s potion reuse), the waypoint outcome ladder states each
  outcome in one line, and the inline notes keep the flood protector
  pacing, the page generation gate and the move start watchdog.
- Verified: `task fmt`, `go build ./...`, the hunt suite (27.8 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - cell_policy.go, the second pass

- 75 blocks compress, the file falls from 225 comment lines of 1095
  (20.5 percent) to 195 of 1069 (18.3 percent): the decision constants
  keep their number and the server rule behind it (the 15-20 s elven
  respawn, the +8 adena edge, the 25 percent hysteresis, the 15 and 20
  percent buy taxes), and the economy notes keep the starve, ripeness
  and death-regression relations. The partition economy stays in
  docs/hunting_cells.md.
- Verified: `task fmt`, `go build ./...`, the hunt suite (27.8 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - shopping.go, the frozen plan contracts

- 77 blocks compress, the file falls from 212 comment lines of 1171
  (18.1 percent) to 165 of 1124 (14.7 percent): the timing constants
  keep their number and the server rule behind it (the 1 s plus 250 ms
  transaction window with punishment limit 0, the 15 and 20 percent
  buy taxes, the spellbook reserve), and the trip notes keep the frozen
  plan, the sell-first credit, the arrival baseline and the gear debt
  relations. The strategy prose stays in docs/shopping_strategy.md.
- Verified: `task fmt`, `go build ./...`, the hunt suite (28.0 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - user.go, the manual command phase

- 68 blocks compress, the file falls from 201 comment lines of 1035
  (19.4 percent) to 169 of 1003 (16.8 percent): the pacing constants
  keep their number and the server rule (the 600 ms confirmation gate
  with the disabled UseItem flood protector, the 4 s re-send bound
  against the attack-window equip deferral, the 150 unit drop reach),
  the gate notes keep the Mobius per-packet task and the same-item
  guard, and the manual walk keeps the redirect, the budget and the
  cursor key escape contracts.
- Verified: `task fmt`, `go build ./...`, the hunt suite (28.0 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - kite_timing_probe.go, the measurement vehicle

- 47 blocks compress, the file falls from 195 comment lines of 980
  (19.9 percent) to 149 of 934 (15.9 percent): the header drops the
  issue id and keeps the three phase bow question and the ladder, the
  constants keep the measured numbers (the 1.5 s windup, the 3 s
  disable window, the five claims a second, ~170 units per second, the
  15-20 s respawn), and the recorder notes keep the coarse clock stamp
  rule. The timing data stays in docs/kite_timing_findings.md.
- Verified: `task fmt`, `go build ./...`, the acceptance suite (8.9 s),
  `golangci-lint run ./internal/swarm/acceptance/...` 0 issues.

### 2026-11-13 - archer_fleet.go, the audit evidence

- 54 blocks compress, the file falls from 185 comment lines of 1030
  (17.9 percent) to 147 of 992 (14.7 percent): the telemetry block
  keeps the numbers (the 250 unit fight floor, the 1.65 s windup at
  pAtkSpd 337, the 1500 unit drift leash, the 35 percent standing
  ceiling, the level 7 maxima 214/82) and drops the prose around them,
  and the fold notes keep the first-retreat-of-a-shot rule, the
  mob-stand anchor and the evidence floors.
- Verified: `task fmt`, `go build ./...`, the acceptance suite (8.9 s),
  `golangci-lint run ./internal/swarm/acceptance/...` 0 issues.

### 2026-11-13 - loop_engage.go, the gate order

- 35 blocks compress, the file falls from 182 comment lines of 585
  (31.1 percent) to 134 of 537 (24.9 percent): the header keeps the
  Mobius double click semantics and the gate order, and each gate keeps
  the server rule it exists for (the corpse selection the server never
  clears, the stance armed without a line of sight check, the
  attack-window refusal of the bow, the percent-gap rule needing the
  confirmed fight, the bow add out-ranging the retreat). The block scan
  shows no run above 8 lines besides the package doc.
- Verified: `task fmt`, `go build ./...`, the hunt suite (27.8 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - scans.go second pass keeps the search rules only

- 44 blocks plus a 12 block re-wrap compress the file from 169 comment
  lines to 144: the target searches keep the projected position rule,
  the 1500 unit engage radius against the 200 unit bias, the 200 unit
  help margin and the 600 unit z limit, the single sided ALL clan, the
  256 bucket median and the pooled scan arrays; the repeated "of the
  zone" and "living attackable npc" prose drops.
- Verified: `task fmt`, `go build ./...`, the state suite (1.2 s),
  `golangci-lint run ./internal/swarm/state/...` 0 issues.

### 2026-11-13 - state/bot.go keeps the field reference only

- 32 field blocks compress: the tracker struct keeps the fact a field
  name cannot say (the clock seam, the density contract of the world
  store, the inventory version the equip cache keys on, the walk plan
  timing, the ActionFailed refusal channel without request identity,
  the critical hint window, the lifetime fields surviving a session
  reset) and points at the owning file. 169 comment lines of 302
  become 85 of 218 (39.0 percent, a struct registry).
- Verified: `task fmt`, `go build ./...`, the state suite (1.2 s),
  `golangci-lint run ./internal/swarm/state/...` 0 issues.

### 2026-11-13 - webserver/navmesh.go keeps the viewer contract only

- 32 blocks compress: the viewer header becomes eight lines and points
  at `docs/navmesh.md` for the dual pack view; the blocks keep the
  approach radius, the fold and grid oracle water blindness, the cold
  tile decode cost (roughly a millisecond per tile), the priced water
  search and the process cached NMV2 payload with its ETag. 168 comment
  lines of 681 become 104 of 617.
- Verified: `task fmt`, `go build ./...`, the webserver suite (8.3 s),
  `golangci-lint run ./internal/swarm/webserver/...` 0 issues.

### 2026-11-13 - state/snapshot.go keeps the wire contract only

- 36 view blocks compress: the snapshot keeps the dangling `json:"-"`
  timing fields and their Go side dump purpose, the JavaScript safe
  integer reason for the string clan mask, the tooltip zero rule, the
  walk origin/index/destination contract and the spot economy field
  meanings. 160 comment lines of 593 become 94 of 527.
- Verified: `task fmt`, `go build ./...`, the state suite (1.2 s),
  `golangci-lint run ./internal/swarm/state/...` 0 issues.

### 2026-11-13 - acceptance/manager.go keeps the run lifecycle only

- 31 blocks compress: the manager keeps the load-bearing numbers (the
  40 line log ring, the 90 s restart wait, the 2 s launch stagger, the
  12 check cap, the "Hunt:" prefix, the 350 ms flood window and the 15
  quick connect burst), the one bot per account partition, the
  collected parallel failures and the injection trail. 159 comment
  lines of 786 become 115 of 742.
- Verified: `task fmt`, `go build ./...`, the acceptance suite (8.9 s),
  `golangci-lint run ./internal/swarm/acceptance/...` 0 issues.

### 2026-11-13 - webserver/dump_parse.go keeps the repro contract only

- 10 blocks compress: the parser keeps the tolerant read rule, the
  minimal repro set, the Apply API replay invariants and the last walk
  plan fallback; the per helper examples stay one line each. 156
  comment lines of 872 become 123 of 839.
- Verified: `task fmt`, `go build ./...`, the webserver suite (8.3 s),
  `golangci-lint run ./internal/swarm/webserver/...` 0 issues.

### 2026-11-13 - hunt/loop_actions.go keeps the loot timing only

- 16 blocks compress: the loot constants keep the 15 s ranged kill
  grace, the 2 s corpse arrival hold, the 300 unit approach radius and
  search slack; the rest keeps the 2.5 s StandUpTask window, the one
  potion reuse window and the server side sit toggle confirmation. 150
  comment lines of 457 become 93 of 400.
- Verified: `task fmt`, `go build ./...`, the hunt suite (28.0 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - state/bot_model.go keeps the packet verdicts only

- 21 blocks compress: the model keeps the bow commit point and the
  C1 cooldown formula (500000/pAtkSpd + reuseDelay*333/pAtkSpd), the
  RED gauge over the stale ATK_SPD after a relogin, the cannot see
  target refusal, the landed hit split of the provocation, the Mobius
  hit flags (0x80 miss, 0x20 crit) and the freshness windows (5 s walk,
  3 s fight, 3 s under attack). 134 comment lines of 411 become 86 of
  363.
- Verified: `task fmt`, `go build ./...`, the state suite (1.2 s),
  `golangci-lint run ./internal/swarm/state/...` 0 issues.

### 2026-11-13 - hunt/guide_buffs.go keeps the gate rules only

- 22 blocks compress: the header keeps the 8-24 level band, the wire
  template 7599 with the 1000000 offset, the two dialog links and the
  refusal cooldown. The table comments keep the 1200 second durations,
  the 16..19 Life Cubic summon bound and the slot collision guard
  against the own auras. 135 comment lines of 420 become 80 of 365.
- Verified: `task fmt`, `go build ./...`, the hunt suite (28.0 s),
  `golangci-lint run ./internal/swarm/hunt/...` 0 issues.

### 2026-11-13 - webserver/server.go keeps the stream contract only

- 31 blocks compress: the server keeps the zero allocation SSE frame
  and payload reuse (the fresh 64 KB frame and payload per version
  change would be megabytes of garbage per second), the per frame 15 s
  write deadline, the 6x direct writer over json.Marshal, the build id
  reload and the dual pack cache split. The issue ids drop. 150 comment
  lines of 563 become 110 of 522.
- Verified: `task fmt`, `go build ./...`, the webserver suite (8.3 s),
  `golangci-lint run ./internal/swarm/webserver/...` 0 issues.

### 2026-11-13 - connection/game.go keeps the wire contract only

- 20 blocks compress: the session keeps the position validation ticker
  and its idle heartbeat, the silent maintenance opcode set (the
  complement of the ActionFailed answered family), the
  `gameSilenceTimeout` ping rule and its test seam, the stateful cipher
  ordering under one critical section, the in place encryption copy,
  the tap copy contract and the 15 s mid-combat store after logout.
  146 comment lines of 629 become 95 of 578.
- Verified: `task fmt`, `go build ./...`, the connection suite (10.5 s),
  `golangci-lint run ./internal/swarm/connection/...` 0 issues.

### 2026-11-13 - session/journal.go keeps the writer contract only

- 16 blocks compress: the journal keeps the tuning numbers (the 2 s
  flush, the 4096 queue, the 64 MiB rotation, the 120 lines a minute
  story cap and the well under thirty lines a healthy bot writes), the
  single writer goroutine and the never blocking send, the open next
  before closing old rotation rule, the first record flush and the
  Windows unlink rule of the gzip step. 129 comment lines of 623
  become 99 of 594.
- Verified: `task fmt`, `go build ./...`, the session suite (0.5 s),
  `golangci-lint run ./internal/swarm/session/...` 0 issues.

### 2026-11-13 - pathfind/navmesh/astar.go keeps the search facts only

- 24 blocks compress: the search keeps the 65536 node cap, the tile
  resolved once at relaxation, the 16384 unit water bucket cell, the
  build versus coverage cost split, the 494 ms map against the 35 ms
  typed arena measurement, the 0.7 load, the polygon granularity
  approach rule with its z axis layer split, the escape flood with the
  zero heuristic, the recovery ban graze rule and its 15k ford, and the
  external link resolution. 143 comment lines of 649 become 110 of 620.
- Verified: `task fmt`, `go build ./...`, the navmesh suite (32.6 s),
  `golangci-lint run ./internal/swarm/pathfind/navmesh/...` 0 issues.

### 2026-11-13 - state/bot_self.go keeps the server verdicts only

- 22 blocks compress: the self view keeps the enter world burst race of
  the SkillList, the strict fresh engagement against the lingering auto
  attack flag, the both sided engagement with the C1 bow reuse refusal
  (Creature.canUseRangeWeapon), the phantom chase of a newer refusal,
  the 80 unit stuck dump, the server snap of a report more than one move
  speed away, the 2.5 s stand window and the unknown max HP rule (100
  percent). The dates and issue ids drop. 142 comment lines of 427
  become 110 of 395.
- Verified: `task fmt`, `go build ./...`, the state suite (1.2 s),
  `golangci-lint run ./internal/swarm/state/...` 0 issues.

### 2026-11-13 - pathfind/engine.go keeps the grid facts only

- 16 blocks compress: the engine keeps the 40 unit HEIGHT_INCREASE_LIMIT
  climb gate, the 20 MB parsed region against the capacity of 4, the 4.2M
  cell grid and the million expansion cap, the 8..24 unit ramp step
  against a terrace jump, the getHeight(tx, ty, tz) target layer
  selection, the single region fast path, the read locked cache hit, the
  start cell escape ring rule with its avoidEscapeMultiplier and the
  click z layer gate. The dated session dump and the issue id drop.
  141 comment lines of 414 become 103 of 376.

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
`hunt/quest_trip.go` and `hunt/loop_safety.go`, `hunt/shopping.go`,
`hunt/loop_params.go`, `acceptance/archer_kite.go`, `hunt/user.go`,
`hunt/loop_movement.go`, `hunt/kite.go`, `hunt/cell_policy.go`,
`hunt/learning.go` and `hunt/town_cursor_escape.go`,
`pathfind/navmesh/query.go`, `pathfind/navmesh/hierarchy.go`,
`pathfind/navmesh/pocket.go`, `pathfind/capsule.go`, `hunt/loop_avoid.go`,
`hunt/stagnation.go`, `hunt/quest_walker.go`, `hunt/click_frame.go`,
`hunt/walk_params.go`, `pathfind/navmesh/smooth.go`, `gear/burst.go`,
`gear/shopping_candidates.go`, `hunt/town_segment.go`,
`acceptance/plaza_herbiel.go`, `pathfind/click_validate.go`,
`cmd/counterprobe/world.go`, `hunt/lure.go`, `hunt/town_sell.go` and
`gear/shopping.go`, `hunt/loop_zone.go`,
`webserver/navmesh_geometry.go`, `hunt/kite_ledger.go`,
`hunt/kite_lane.go`, `hunt/city_shopping.go`, `hunt/town_merchants.go`,
`acceptance/delevel.go`, `pathfind/navbuild/sheet.go`,
`pathfind/navmesh/abstract.go`, `huntaudit/audit.go`,
`cmd/navmesh-export/main.go`, `cmd/navpack-verify/main.go`,
`acceptance/botlog/botlog.go`, `cmd/counterprobe/detect.go` and
`hunt/recovery_heal.go`, `hunt/navmesh_navigator.go`, `hunt/equip.go`
`from_game_server/user_info.go`, `pathfind/search.go`,
`pathfind/navmesh/tile.go` and `state/diagnostics.go` (second passes).
Next: a fresh `task comment:audit --top 10` pick (the
`proxy/game.go` and `hunt/loop_state.go` entries stay untouched while
their fragments are active); the share and the block count are
re-measured at the end.
