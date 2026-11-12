# P06: the clock seam of state and hunt (status: complete)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the file split rounds, the packet router and the
packet id deduplication).

## Goal

The P06 prompt (`docs/quality_review_and_agent_prompts.md`) asks for one
clock seam in `state.Bot` and `hunt.Loop`, the decision logic converted
to it, and the sleeping freshness tests moved onto a fake clock. Done
means:

- a nil-safe seam (`now()` reading the injected function or the system
  clock, `SetClock` to install one) in both types, so the zero value
  keeps working;
- every `time.Now()`/`time.Since` of a `*Bot`/`*Loop` method reads the
  seam instead of the system clock;
- the remaining direct reads are named and justified (the diagnostics
  timers of the navmesh navigator, the dialog bypass pacer, the
  constructor stamps);
- the freshness tests that sleep migrate to the fake clock, and the
  hunt suite wall time drops.

## Constraints

- No behavior change: with the system clock installed the same instant
  and the same order of reads.
- The seam is a set-before-share contract, not a locked field: the
  packet producers read it from several goroutines and the existing
  lock scope of the apply paths must not grow.
- `task fmt` (gofmt-spaces) after every slice, then the full gate.

## Context

Measured 2026-11-12 before the change:

- 64 direct `time.Now()`/`time.Since` sites in `state`, of which only
  the two constructor stamps of `NewBot` sit outside a `*Bot` method;
- 114 sites in `hunt`, of which six sit outside a `*Loop` method: the
  four duration probes of `navmeshNavigator` and the two reads of
  `bypassPacer`;
- the hunt suite takes 97 s wall time, with 28 `time.Sleep` calls in
  the tests, several of them multi-second freshness windows.

## Progress

### 2026-11-12 - the seam
`state/clock.go` and `hunt/clock.go` carry `now()`/`SetClock`. The zero
value reads the system clock, so every existing construction keeps its
behavior without a constructor change. `clock_test.go` in both packages
pins the two sources and the nil restore.

Verified: `go build ./...`, both seam tests green, `golangci-lint run`
0 issues on the two packages.

Next: the state conversion, then the hunt conversion, then the sleeping
tests.

## Status

Complete: the seam, the state and hunt conversions, the test migration,
`task verify` and the live runs are done. The fragment archives with the
devlog record of the round.

### 2026-11-12 - the state conversion
The 62 direct reads inside `*Bot` methods now call `b.now()`
(`time.Since(x)` becomes `b.now().Sub(x)`), covering the apply paths,
the freshness windows, the buff clock, the walk plan TTL and the
snapshot/scan decisions. The two constructor stamps of `NewBot` stay on
the system clock by design: the instance does not exist yet, and the
injection happens after construction. Three files stopped importing
`time` because the seam replaced their only read.

Verified: `go build ./...`, `go vet ./internal/swarm/state/...`,
`golangci-lint run` 0 issues, `go test ./internal/swarm/state/...`
green in 1.2 s.

### 2026-11-12 - the hunt conversion
The 108 direct reads inside `*Loop` methods now call `l.now()`
(`time.Since(x)` becomes `l.now().Sub(x)`): the engagement, rest, loot,
trip, delevel, user, shopping, quest, town and kite phases all read the
seam, including the session skill list gate of the first town trip.
Six reads stay on the system clock by design: the four `Duration`
probes of the navmesh navigator (a diagnostic of the mesh query, not a
decision) and the two reads of `bypassPacer` (the server flood window
is a real-time constraint a fake clock must not skip).

Verified: `go build ./...`, `go vet ./internal/swarm/hunt/...`,
`golangci-lint run` 0 issues, `go test ./internal/swarm/hunt/...` green
in 97.3 s (the same suite, no behavior change).

### 2026-11-12 - the sleeping tests on the fake clock
`clock_fake_test.go` adds `fakeClock` (a mutex-guarded `Now`/`Advance`
and an `install` that sets the clock on both the loop and its tracker,
so the decision windows of both layers advance together). The tests
that slept for a freshness, rest or trip window now advance it:

- `equip_test.go` (4.1 s of two sleeps), `kite_edge_test.go` (3.2 s),
  `kite_ledger_test.go` (1.8 s), `loop_test.go` kill-spot rest (3.3 s),
  `loop_los_test.go` (3.15 s + 2.1 s), `round59_repro_test.go` (two
  3.15 s waits plus the 10 ms ordering sleep of the phantom chase),
  `user_test.go` (two 6 s deadline polls), `kite_cycle_test.go`
  (150 ms of segment age);
- the poll-driven walk tests: `plaza_pingpong_repro_test.go` (25.35 s
  of 10 ms polls), `building_entry_test.go` (7.27 s of 5 ms polls),
  `town_repro_test.go` (2.74 s), `teacher_walk_test.go` (2.63 s) and
  `farm_readiness_shop_repro_test.go` (2.65 s) now advance 5-250 ms of
  fake time per iteration instead of sleeping it.

The remaining `time.Sleep` calls are deliberate: the gatekeeper and
soe tests deliver their server answers from a goroutine (a real wait
for an asynchronous producer), the quest walker suite is the opt-in
live-server test, and the production `bypassPacer` reads the system
clock by design (the server flood window must not be skipped).

Measured: `go test ./internal/swarm/hunt/ -count=1` 97.3 s -> 27.7 s;
the slowest test is now `TestDriveQuestChainHappyPath` at 2.11 s (its
dialog pacing rides the real bypass window).

Verified: `go build ./...`, `go vet ./internal/swarm/hunt/...`,
`golangci-lint run` 0 issues, the full hunt suite green, `task fmt:check`.

### 2026-11-12 - the gate and the live run
`task verify` exit 0 (build, vet, the full tree lint, the whole test
suite, `fmt:check`). Two live runs on the Windows stack with the
`-ldflags` identity of `d9a7a36e`: the first without geodata (the
temp-directory recipe) hunted, killed and looted Kaboo Orc Fighters
and reported the town trips disabled, exactly the documented
consequence of a missing `data/geodata`; the second with `-geodata
E:/work/swarm/data/geodata` loaded the 165 regions, walked the trip to
the guard Starden and steered around the mobs. Both stayed in the
world 65 s and exited 0 on Ctrl+Break.

Complete: the seam and the conversions land on `main`; the hunt suite
dropped from 97.3 s to 27.7 s. The round record is
`docs/devlog/2026-11-12-issue0-clock-seam.md`.
