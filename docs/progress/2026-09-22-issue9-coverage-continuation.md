# The coverage reporting continuation - the merge refresh and the top-up rounds (status: in progress)

Started 2026-09-22, branch feature/coverage-reporting, issue #9.

## Goal

Keep melg8/swarm#9 ("Improve test coverage") moving: PR #24 must
stay mergeable and CI-green against the evolving main, the committed
coverage baseline must reflect the merged tree honestly, and the
remaining low-coverage packages named by the audit get topped up.

## Context

- The first two rounds of this issue (2026-09-21) landed on PR #24:
  the coverage_delta.sh repair, the packet builder failure-walk
  suites (to_game_server 65.3 -> 97.9, to_auth_server 78.1 -> 98.6),
  the cmd/swarm helper tests (11.4 -> 19.6), the version identity
  fallbacks (78.3 -> 87.0), tools/coverage_report.sh (the
  worst-first audit) and the live CI workflow. The full history is
  in the issue #9 comments and the Round 129 entry of
  docs/development_log.md.
- 2026-09-22: main absorbed PR #39 (the CI flake fixes, the trigger
  split, the baseline refresh split) and PR #34 (the merge clash
  policy - docs/progress/ fragments replace the shared
  agent_progress.md appends), which left PR #24 with merge
  conflicts (docs/agent_progress.md, runs/coverage-latest.txt,
  add/add tools/coverage_report.sh).
- The audit's remaining candidates, recorded in the issue:
  acceptance 27.8, huntaudit 24.3, acceptance/botlog 37.3 (the
  live-stack-bound orchestration - a design round of their own),
  npcdata 82.5 and connection 73.9 (the drift packages to top up),
  cmd/swarm 19.6 (the main.go wiring, needs seams), the eight 0%
  probe tools (one combined smoke pass at most).

## Progress

### 2026-09-22 08:20 UTC - the main merge resolves the PR #24 conflicts
- agent_progress.md: main's frozen version wins (the file is a
  frozen archive since the clash policy round); the coverage round
  context lives on in this fragment.
- tools/coverage_report.sh: both sides byte-identical, kept ours.
- runs/coverage-latest.txt: resolved provisionally with ours, then
  regenerated from the actual merged tree (the numbers of both
  parents were measured on different trees - neither is the truth
  of the merge result).

## Status

The merge commit is in; next: the full gate pass (build, vet, lint,
fmt, tests, the coverage regen and the delta gate), then push and
the top-up round for the drift packages.
