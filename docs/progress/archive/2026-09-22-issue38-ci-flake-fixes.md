# The CI flake fixes round - the starved validation window and the race slice timeout (status: in progress)

Started 2026-09-22, branch feature/ci-flake-fixes, issue #38 ("Fix
main ci", referencing the flakes reported in #33).

## Goal

Main's CI must run clean: no flaky test failures, every job green.
Two failure modes were measured on PR #34's reruns (the account is
in issue #33's comments):

1. TestGameClientRunStreamsThePositionValidation (connection, the
   race slice): the 2600 ms session window is eaten by the handshake
   (the char-create drain alone waits up to charCreateOkWait = 2 s
   under -race on a loaded runner), starving the 1 s validation
   ticker - 0-1 fires where the test asserts 2.
2. The pathfind suite under -race measured 474-558 s on normal
   runners and hit the 600 s default package timeout on a slow one
   (the goroutine dump shows plain CPU-bound parseRegion work, no
   hang).

Done means: both failure modes addressed, the flake ledger carries
the two rows, the stronger deterministic pins named for the follow-up
rounds, and the coverage debt story (main red until PR #24 lands)
restated for the owner.

## Context

- The evidence: PR #34 (docs-only) ran the identical tree through
  four CI attempts - attempt 1 and 2 failed the connection timing
  test (0 and 1 ticks), attempt 3 passed connection but killed
  pathfind at 600.025 s, attempt 4 passed everything. The same tree
  passed the push-event run. Same bytes, varying outcomes: runner
  timing instability, not the diffs.
- The coverage job on main fails on the three pre-existing drift
  packages (acceptance 27.8, connection 73.9, npcdata 82.5) - PR #24
  carries the refreshed baseline and is fully green; main turns
  green at its merge. This round does not duplicate that baseline
  (it would clash with #24's file).

## Progress

### 2026-09-22 06:35 UTC - the connection test window

validate_position_stream_test.go: the session window 2600 ms -> 5 s
and the poll deadline 3 s -> 5 s, so the handshake keeps its real
cost while the 1 s ticker gets its two fires. The nudger walk caps
at 8 steps (800 units) and holds - the report envelope the
assertions pin (InDelta 900) stays exactly what it was; a longer
window changes how long the placement sits at the walk end, not
the envelope.

### 2026-09-22 06:37 UTC - the race slice timeout

The race step of both .github/workflows/ci.yml and the canonical
docs/ci_workflow.yml gains -timeout=15m: the suite duration under
-race is instrumentation plus runner-class cost (474-558 s normal),
the 10 m default kills it on slow runners. A truly hung test still
fails at 15 m - the bound stays a bound.

### 2026-09-22 06:38 UTC - the flake ledger rows

Two rows appended (the ledger's own rule: same-session rows), each
naming the landed fix and the stronger deterministic pin a
follow-up round could make: an injected clock/ticker seam for the
session run loop (the window stop being the honest stopgap), and a
race-slice job split for the pathfind duration.

## Status

In progress: commits landing, then the PR (marker Fixes #38) and
the issue account.
