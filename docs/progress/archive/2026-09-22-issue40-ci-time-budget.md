# The CI time budget round - the seven minute contract (status: in review)

Started 2026-09-22, branch feature/ci-time-budget, issue #40.

## Goal

The CI wall time from change to final verdict must not exceed 7
minutes. No gate may be dropped or weakened - build, vet, the full
test suite, the whitespace scan, the uncapped lint, govulncheck, the
logfmt scan, the whole race slice and the coverage delta gate all
keep running. Splitting work across more runners is allowed.

## Context

- The measured baseline (run 35706014800, the PR #24 head 15b3270):
  the single verify job took 1046 s - the race step alone 740 s
  (the pathfind tree under -race is the whale), the pinned dev tool
  install 119 s, the full test suite 108 s. The coverage job paid
  the suite a second time for the audit report (109 s + 101 s).
- Local per test measurements: pathfind root = 86 tests, 96.7 s
  plain, the slowest five (EngineConcurrentSearchesRaceFree 15.3,
  the FindPath cliff/step-up/enclosed/approach family) own ~40%;
  navbuild = 27 tests, 55.4 s plain.
- The runner is ~3x faster than the 2-core agent host (108 s vs
  ~320 s for the full plain suite).

## Progress

### 2026-09-22 09:35 UTC - the parallel split lands on the branch
- The workflow becomes seven parallel jobs: verify (build, vet,
  test, fmt via `go run ./cmd/gofmt-spaces`, logfmt), lint (the
  tool install + golangci-lint + govulncheck), race-net
  (connection, state, webserver, the navmesh readers), race-pathfind
  (a 3-shard matrix over the root package), race-navbuild (a 2-shard
  matrix), acceptance-list, coverage.
- The race sharding partitions the sorted `go test -list` output
  round-robin - the union of the shards is exactly the full test
  list (verified locally: 29+29+28 = 86 and 14+13 = 27), a new test
  joins a shard automatically, nothing is skipped.
- The coverage job runs the suite once: coverage_delta.sh gains
  KEEP_RUN_LOG=1, coverage_report.sh gains -from-log <path> (both
  modes validated locally against a real run log).
- docs/ci_workflow.yml carries the verbatim copy (the NOTE header
  kept).

## Status

Branch work complete, all local gates green; next: push, the PR,
and the PR's own CI run is the acceptance measurement (it must
finish inside the 7 minute budget).

### 2026-09-22 09:35 UTC - the acceptance runs
- First cut (21d9a01, run 35709319245): 353 s, all green - but one
  navbuild shard carried the heavy region-replay tests (348 s job)
  and left only ~1 min of margin.
- The refinement (f77bef9, run 35710067754): the third navbuild
  shard and the exact shard-count log lines. **286 s wall (4 min
  46 s), all 11 checks green** - the contract holds with two
  minutes of margin. Shard evidence in the logs: 29+29+28 = 86 of
  86 pathfind, 9+9+9 = 27 of 27 navbuild.
- The issue comment carries the full report; PR #41 mergeable
  clean. The round closes at in review.

## Status (final)

In review: PR #41 head f77bef9, wall 4 min 46 s, every gate green.
