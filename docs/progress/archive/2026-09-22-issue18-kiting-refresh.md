# The kiting branch refresh - the ride onto the new main (status: in review)

Started 2026-09-22, branch feature/archer-kiting, issue #18.

## Goal

PR #15 (the kiting core, four commits: the kite step, the train
member trigger, the parameter pinning) must ride clean onto the new
main and prove itself under the live CI - the branch predates the
fragment policy, the workflow activation and the seven minute CI
budget, and its head 5df6073 never ran a single CI job.

## Context

- The issue #18 round history: two agent rounds landed yesterday
  (the trigger + direction, the parameters), the branch was made
  linear and rebaseable (head 5df6073), the owner reviewed and moved
  the card back to Ready - the same re-triage pattern as #9 this
  morning.
- Main moved heavily since: the fragment policy (PR #34), the flake
  fixes and the baseline refresh (PR #39), the live CI activation
  and then the seven minute budget split (PR #41) - main now runs
  the 11-check parallel workflow.
- The merge conflict was exactly one file: docs/agent_progress.md
  (the frozen archive) - main's version wins, this fragment carries
  the round context.

## Progress

### 2026-09-22 10:05 UTC - the merge and the local gates
- Merged origin/main (dab5e4b) into feature/archer-kiting: the
  frozen archive resolved to main's version, everything else auto-
  merged (2e4a582).
- Local gates on the merged tree: build, vet, the hunt suite green
  (77 s, the kiting tests ride along), golangci-lint 0 issues,
  fmt:check clean, the logfmt scan green.

## Status

The merge is in and locally green; next: push (the new 11-check CI
verifies the branch for the first time), then the issue comment and
the board move.

### 2026-09-22 10:05 UTC - the proof and the close
- Pushed 90b0f11; run 35713338802 (the branch's first CI run ever):
  all 11 checks success in 5 min 27 s, PR #15 mergeable clean.
- The issue comment carries the report; the card sits in Ready for
  review, the claim messages are clean (mine and the three stale
  ones from yesterday).

## Status (final)

In review: PR #15 head 90b0f11. The kiting core rides the new main
and the live gate; the four kiting commits are content-unchanged.

### 2026-09-22 10:35 UTC - the linearity repair
- The owner feedback on the refresh round: "Fix conflicts, can't
  rebase" - the merge commit I rode in repeated the exact mistake
  yesterday's round documented (the rebase strategy replays the
  commits one by one; a merge commit kills the rebase button).
- The branch is rebuilt as a linear chain on top of current main:
  the three payload commits cherry picked clean (the kite step, the
  train member trigger, the parameter pinning - the docs-only task
  entry commit is obsolete under the fragment policy and dropped),
  the doc conflicts resolved to main's frozen archive.
- The payload is bit-identical to the CI-verified 5794d37 tree
  (git diff over internal/ and cmd/ is empty).

## Status (the linearity repair)

Linear on main, payload unchanged; next: the hunt suite + gates,
force push, the fresh CI verdict.

### 2026-09-22 10:50 UTC - the repair closes
- 39e5de8: all 11 checks green, PR #15 mergeable clean, linear
  history (zero merge commits), the rebase path open again.
- The issue comment carries the fix report; the card is in Ready
  for review, the claims are clean.

## Status (final)

In review: PR #15 head 39e5de8, linear, payload bit-identical to
the previously verified tree. Future rounds on this branch: rebase
or cherry-pick only, never merge main in.
