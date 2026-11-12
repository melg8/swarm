# P06: the clock seam of state and hunt (status: in progress)

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

The seam lands; the conversion is next.
