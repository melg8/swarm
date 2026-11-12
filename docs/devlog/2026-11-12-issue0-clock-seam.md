# The clock seam round (issue0)

Date: 2026-11-12. Scope: the decision clock of `state.Bot` and
`hunt.Loop` and the hunt tests that slept for time windows. Issue0
(owner instruction: continue the refactor), landed on `main`: the seam
in `a682307e`, the state conversion in `147d8602`, the hunt conversion
in `b850f68b`, the test migration in `d9a7a36e`.

## Problem statement

Every timing decision of the two layers read `time.Now()` and
`time.Since` directly. The unit tests could not move the decision
instant, so a freshness, rest, trip or stuck window could only be
crossed by sleeping in real time - `internal/swarm/hunt` needed 97.3 s
of wall clock, with 28 `time.Sleep` calls in its tests, several of them
multi-second freshness windows.

## Root cause analysis

- 64 direct reads in `state`, of which 62 sit inside `*Bot` methods;
  the two constructor stamps of `NewBot` (`started`, `sessionAt`) run
  before the injection point.
- 114 direct reads in `hunt`, of which 108 sit inside `*Loop` methods.
  The six exceptions are the four `Duration` probes of
  `navmeshNavigator` (mesh query diagnostics, not decisions) and the
  two reads of `bypassPacer` (the server flood window is a real-time
  constraint a fake clock must not skip).
- `Loop.Run` paces the cadence with `time.NewTicker(tickPeriod)`. The
  ticker is the wall-clock rhythm of the loop, not a decision input,
  so it keeps the system clock; only the decision reads moved.
- The seam cannot be protected by a mutex: the packet apply paths
  already hold the tracker lock while they read the clock, so a second
  lock around the field would grow the hot lock scope. The contract is
  set-before-share: production installs nothing, the tests and the
  wiring set it before the loop starts.

## Reproduction

`go test ./internal/swarm/hunt/ -count=1 -v` on the Windows host
before the round: 97.3 s total, the top offenders
`TestReproPlazaPingPongWalksToHerbiel` 25.35 s (10 ms poll loop),
`TestBuildingEntryEscapesTheWalledAisle` 7.27 s (5 ms poll loop),
`TestReproRound59PhantomChaseSwitchesTarget` 6.31 s,
`TestInventoryResendOutlivesTheAttackDeferral` 4.10 s,
`TestLoopRestsAtTheKillSpot` 3.30 s,
`TestKiteCorneredHoldKeepsShooting` 3.20 s,
`TestEngageStuckTimeoutFiresThroughStaleAttackStance` 3.15 s and the
two `TestUserAttack*` cases 3.01 s each.

## Fix

- `state/clock.go` and `hunt/clock.go` add a nil-safe `now()` (the
  injected function or the system clock of the zero value) and a
  `SetClock`. No constructor change: every existing `NewBot`/`NewLoop`
  keeps the system clock.
- The 62 `*Bot` sites and the 108 `*Loop` sites call the seam;
  `time.Since(x)` became `now().Sub(x)`.
- `hunt/clock_fake_test.go` adds `fakeClock` (mutex-guarded `Now` and
  `Advance`, an `install` that sets the clock on the loop and its
  tracker). The sleeping tests advance it instead of sleeping, and the
  poll-driven walk tests advance 5-250 ms of fake time per iteration.
- The goroutine-delivered server answers of the gatekeeper and soe
  tests and the opt-in live quest walker suite keep their real waits.

## Verification

`go test ./internal/swarm/hunt/ -count=1` dropped from 97.3 s to
27.7 s; every migrated test now finishes under 0.3 s and the slowest
test of the package is `TestDriveQuestChainHappyPath` at 2.11 s (its
dialog pacing rides the real bypass window by design). No `time.Sleep`
was added anywhere: the diff of the four commits carries only removed
ones.

`go build ./...`, `go vet ./internal/swarm/state/...` and
`./internal/swarm/hunt/...`, `golangci-lint run` with 0 issues,
`task fmt:check` and `task verify` are green; the live run on the
Windows stack stays in the world 65 s, kills and loots, and exits 0 on
Ctrl+Break. The CI workflow runs the `-race` slice; the Windows host
has no cgo toolchain, so the race pass runs there.

## Follow-ups

The navmesh navigator diagnostics and `bypassPacer` stay on the system
clock on purpose. If a future test needs to skip the bypass flood
window, the pacer takes the loop clock instead of reading the system
one - the decision and the constraint then share one seam.
