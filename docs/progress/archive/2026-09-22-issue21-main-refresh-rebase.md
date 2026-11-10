# The archetype main refresh - the second linearization onto the merged main (status: in review)

Started 2026-09-22, branch feature/archer-archetype-check, issue #21.
Links the previous fragments:
docs/progress/2026-09-22-issue21-rebase-linearization.md and
docs/progress/2026-09-22-issue21-archetype-integration-repair.md.

## Goal

Keep PR #31 (the archetype integration contract) in the shape the
owner's rebase-merge strategy demands - a linear history sitting
directly on the current main, zero merge commits, a zero net diff on
the frozen logs - after main absorbed the coverage reporting PR #24
and the kiting core PR #15, which left the branch dirty again.

## Context

- The owner merged PR #24 (the coverage reporting, 12:16 UTC) and
  PR #15 (the kiting core, 12:17 UTC) into main (tip 55906b6) and
  moved issue #21 back to Ready - the integration branch's base
  went stale and the PR reported dirty.
- The branch carried twelve linear commits on dab5e4b: the kiting
  chain (the kite step, the train trigger, the parameters - now
  patch-identical to what PR #15 merged), the edge case slice (PR
  #25 still open), the config launch chain (PR #16 open), the bow
  gear plan (PR #23 open), the contract test, the repair round
  content and two docs fragment commits.
- The previous linearization round established the conventions this
  round follows: the tree identity check, the frozen log invariant
  (the net PR diff carries zero legacy log edits), the
  force-with-lease push against a verified remote head with a
  local backup ref.

## Progress

### 2026-09-22 12:40 UTC - the rebase onto 55906b6

- `git rebase origin/main` skipped the two patch-identical kiting
  commits (the kite step, the train trigger - "patch contents
  already upstream"): PR #15's merge already carries their content.
- The frozen log conflicts (docs/development_log.md, the round 129
  append collision; docs/agent_progress.md, the active task header)
  resolved to main's side per the clash policy; the pure legacy log
  append commit (the kite edge case task entry) dropped from the
  replay entirely - its context lives in the fragment.
- The frozen log invariant verified: the net diff of
  docs/agent_progress.md, docs/development_log.md and
  docs/agent_progress_archive.md against main is zero.
- The branch keeps the legitimate doc rows (AGENTS.md,
  docs/README.md - the launch config documentation map entries) and
  the two fragment files.

### 2026-09-22 12:55 UTC - the gates and the baseline

- build, vet, full tree golangci-lint 0 issues, gofmt-spaces clean.
- The touched suites green: hunt 79.7s (the edge case tests ride
  the merged kite step), gear, cmd/swarm.
- The coverage baseline regenerated from the full tree: cmd/swarm
  19.6 -> 27.4 (the launch config tests), gear 90.3 -> 90.5 (the
  archer profile tests), hunt 84.2 -> 84.4 (the edge case tests).
  No drops, total statement coverage 74.5%.

## Status

The rebased branch (ten linear commits, zero merge commits) sits on
55906b6. Next: the force-with-lease push (the backup ref holds the
pre-rebase head), the CI verdict, the issue comment and the board
move. The remaining open slices for the composition: PR #16, PR #23
and PR #25.
