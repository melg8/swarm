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

## Status

The tool, the rule and the renames are in. The worst files are
compressed one commit each: `town.go`, `loop.go`, `kite.go`,
`shopping.go`, `cell_policy.go`, `user.go`, `quest_trip.go`,
`town_walk.go` (twice), `loop_state.go`, `town_stuck.go`,
`acceptance/archer_fleet.go` and `pathfind/search.go`. Next:
`pathfind/navmesh/hierarchy.go` (298 lines), `state/scans.go` (284),
`hunt/zones.go` (280), `hunt/town.go` (272); the share and the block
count are re-measured at the end.
