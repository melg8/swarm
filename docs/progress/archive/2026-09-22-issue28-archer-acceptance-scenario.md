# The archer acceptance scenario (status: in review)

Started 2026-09-22, branch feature/acceptance-archer-kite, issue #28.

## Goal

The codeable acceptance half of #20: a scenario in the style of
internal/swarm/acceptance/ that launches an archer-type temp bot
into a hunting cell and asserts the kiting contract end to end.
Done means: the scenario registered (the acceptance CLI lists it),
the pure evaluation unit-tested (the CI verifies the check logic
without the live stack), the full uncapped lint at zero, the
coverage delta gate inside its budget, all CI checks green on the
pushed head, the issue back in Ready for review.

## Context

- The dependency reading: the issue text says "do not start before
  both PRs merge" (the config launch #12/PR #16 and the bow gear
  #17/PR #23). PR #23 merged 2026-09-22 13:22. PR #16 is still
  open, but its CODE is on main already - the config-launch commits
  a2582bc and 27bed31 landed as content twins (launch_config.go,
  launch_config_test.go and configs/swarm.json are byte identical
  to the branch; cmd/swarm/main.go on main carries the wiring PLUS
  the archer gear profile line). The scenario builds on main, not
  on any in-review branch - the stacking the issue warns about is
  not happening. PR #16 itself is the reviewer's to disposition
  (content-merged close or feedback); the finding is posted on
  issue #12.
- The launch wiring: cmd/swarm maps the "archer" type to
  loop.SetGearProfile(gear.Archer{}). The acceptance runner
  constructs the loop inside runSession, so the type wiring rides
  the new sessionLoopHook variadic (runner.go) - the same one-line
  mapping, no runner fork.
- The start state: level 7 on the Kaboo Orc Grunt SW-5 cell
  (elven-hex-104, focus 36000 50229, Z -3456 from the geodata
  oracle). Why level 7: below the guide window (8-24, an unbuffed
  start would walk the 26 km guide trip first), inside the cell
  band (7-10, the ladder does not relocate), no delevel trigger
  (level far below median+5), and the aggressive melee Kaboo orcs
  close on the archer - exactly the threat the kite answers. Mass
  4.0 keeps the fight cadence honest; the ground sits ~13 km from
  the delevel scenario's cell (no temp14/temp18 mob competition in
  the parallel run all).
- The kit: the short bow (item 13), a full 600 arrow quiver (the
  restock target; the floor sits at 150 and the smoke window
  cannot spend the difference - no restock walk arms), the wooden
  outfit of the zone return dump and 3 healing potions. No wallet,
  no SP: nothing affordable, no shopping or learning trip arms.
- The telemetry: the fight distance (the self position against the
  fighting target while the swings run) folds into the range
  verdict (the samples beyond the melee radius, none past the bow
  stall radius); the kite log lines (kite.go's "kiting clear (step"
  and "holding ground and shooting" markers) count through the
  logLine wrapper of the session (every Hunt line still flows to
  the test log and the botlog run file); the survival reads the
  online hold, the death flag and the HP floor; the quiver sums
  the item 17 stacks of the inventory (worn plus bagged).
- The watch/verdict split is pure (the fold and the verdicts run
  without the tracker) - archer_kite_test.go pins the definition,
  the reset, the kit, the checks, the fold latches, the verdict
  table (the honest kite window, the melee tank, the chase beyond
  the band, the evidence floor, the static archer, the dry quiver,
  the death, the offline drop, the HP bottom), the log counter and
  the live extraction.
- The registry position: the archer-kite entry owns the head of
  Definitions() (the newest round does); the full-dress and
  village-escape positional tests moved with it (slots 1-4 and 6).
- The temp account: temp18 (temp15 and temp16 belong to the stuck
  point scenarios, temp17 to plaza Herbiel).
- The lint: the four archerKite aggregates (the log counter, the
  watch, the verdicts, the sample) are documented-zero-start
  partial inits - the exhaustruct_v5 ignore-patterns list carries
  them (the fullDressWatch precedent). runSession grew past the
  funlen bound with the hook loop - the loop wiring extracted into
  wireSessionLoop.

## Progress
### 2026-09-22 14:20 UTC - the archer kite scenario lands on the branch

The runner hook (sessionLoopHook), the scenario
(internal/swarm/acceptance/archer_kite.go: the reset, the checks,
the log counter, the pure fold/verdicts, the monitor, the await),
the unit tests (archer_kite_test.go, 9 tests), the registry entry
(the head of Definitions(), the full-dress and village-escape
positional pins moved), the exhaustruct patterns and the fresh
coverage baseline (the acceptance package 27.8 -> 29.5). The
verification: build, vet, the full acceptance suite, hunt (80 s),
state, cmd/swarm, the logfmt scan, gofmt-spaces, the uncapped lint
at zero, the acceptance CLI list (archer-kite first), the coverage
delta inside the budget. Next: push, the PR, the CI watch.

## Status

In review: the branch is pushed, the PR (Fixes #28) waits on the
CI and the reviewer. The live-stack run (the scenario against the
local deployment) is the reviewer's launch - the check logic is
unit-pinned, the scenario is duration-agnostic
(SWARM_ARCHER_KITE_MINUTES).
