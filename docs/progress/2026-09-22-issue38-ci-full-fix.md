# The full CI green round: the flake root causes, the coverage baseline, the trigger dedup (status: in progress)

Started 2026-09-22, branch feature/ci-flake-fixes, issue #38.

## Goal

Main runs the whole ci workflow green: the verify job (build, vet,
test, whitespace, lint, the race slice, govulncheck, logfmt), the
coverage job and the acceptance-list job. The owner feedback round
of 2026-09-22 06:33 UTC on issue #38 asks for three things: the full
fix lands HERE (the coverage job stopped deferring to PR #24), the
ci triggers stop running the whole workflow twice on the same source
(push + pull_request on a PR branch), and the flaky tests stay
fixed. Acceptance: a push to this branch shows one run per commit,
verify green, coverage green.

## Context

- Issue #38, PR #39 (the previous round: the window widening, the
  race timeout, the ledger rows - this round keeps those changes and
  builds on them). The previous round's fragment:
  `docs/progress/archive/2026-09-22-issue38-ci-flake-fixes.md`.
- The measured failure modes on main (run 35694731333): the race
  step of verify fails on `TestGameClientRunStreamsThePositionValidation`
  with 0 captured validation reports; the coverage delta gate fails
  on acceptance (-5.2 pp), connection (-2.4 pp), npcdata (-3.1 pp)
  with acceptance/botlog new without a baseline (the package split).
- The PR #39 head run failed differently: the same test dead with
  `game connection lost: failed to read packet header: EOF` at the
  plain Test step - the widened 5 s window outlived the fake
  server's 4 s absorb budget, the flow returned, the conn closed
  mid session.
- The root causes verified locally this round (instrumented runs,
  `go test -race -count=N` reproduces the 0-report mode at roughly
  every second iteration on the sandbox):
  1. The fake server's `characterFlow` wrote the CharCreateOk
     (0x25) BEFORE the updated char list (0x1F) - the real server
     order is the opposite (verified in CharacterCreate.java:
     `initNewChar` sends the CharSelectionInfo list and only then
     the handler sends CharCreateOk). With the inverted order the
     client's `awaitCharacterCreation` consumed both packets, so
     `drainCharCreateOk` always waited out its whole 2 s budget
     ("Char create ok not drained in time" on EVERY run, pass or
     fail) - a flat 2 s handshake tax and the wall-clock pressure
     behind the window flakes.
  2. The absorb loop of `absorbingFlowWithMoves` read with a
     rolling 500 ms read deadline. The deadline races the client's
     1 s validation ticker; when it fires mid frame, io.ReadFull
     has consumed the leading bytes of a packet and loses them,
     the framing desyncs and every later read returns garbage
     opcodes (observed: the client sent its 0x48 validations, the
     server read 0x5f). The reports channel stays empty, the test
     fails with 0 reports. This - not window starvation - is the
     actual mechanism behind the main-branch race failures.
  3. The absorb budget (4 s) is shorter than the widened session
     window (5 s): the flow tears the conn down while the session
     still runs, Run returns EOF (the PR #39 mode).
- The fix set: the wire order swap (kills the 2 s tax everywhere),
  the single budget-end read deadline (a read completes whole or
  the budget ends the flow), the 10 s absorb budget (outlives the
  5 s window with margin), on top of the previous round's window
  widening, poll extension, nudger cap and the race timeout.

## Progress

### 2026-09-22 07:20 UTC - test: the fake server rides the real creation order, the absorb read never splits a frame

The two test-infrastructure root causes. `characterFlow` writes the
updated list first and the CharCreateOk last (the CharacterCreate.java
order); `drainCharCreateOk` now confirms in milliseconds instead of
waiting out 2 s. The absorb loop of `absorbingFlowWithMoves` arms ONE
read deadline at the budget end (10 s, sized to outlive the 5 s
session window) instead of the rolling 500 ms deadline whose mid-frame
expiry desynced the framing. Measured on the sandbox: the flaky test
passes 10/10 under -race -count=10 (previously ~50% fail at count>=2),
the whole connection package passes under -race in 11 s (was 35 s with
the drain tax), the full `go test ./...` is green, `task lint:new` 0
issues, `task fmt:check` clean. Next: the ledger rows for the true
mechanisms, the ci trigger dedup, the coverage baseline refresh.

## Status

In progress. The flake fixes are verified locally; the ledger, the
workflow triggers and the coverage baseline remain.
