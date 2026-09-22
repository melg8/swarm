# The archetype branch linearized for the rebase merge (status: in review)

Started 2026-09-22, branch feature/archer-archetype-check, issue #21.
Follow-up of 2026-09-22-issue21-archetype-integration-repair.md (the
main-merge round that left the merge commits in the history).

## Goal

The owner's review ask (issue comment 2026-09-22 10:25:39): "This
branch cannot be rebased due to conflicts, fix." The rebase-and-merge
strategy refuses branches whose history contains merge commits - the
two main-absorption merges (c79354a the chain composition, 366f678
the main merge of the repair round) blocked it. Done means: a linear
history sitting directly on the current main, tree-identical to the
CI-green head, all checks green, the branch rebased with a lease
guard.

## Context

- The repair round (see the linked fragment) made PR #31
  merge-clean by merging origin/main in - the previous practice of
  the branch rounds, which the rebase-merge strategy now rejects.
- The parallel kiting slice (feature/archer-kiting, issue #18)
  received the same owner ask and was linearized with a force
  update at ~10:36Z - the swarm converges on the same fix.
- The clash policy transition rule: on a rebase drop the legacy
  agent_progress.md appends - the two pure docs-entry commits of
  the chains (the kite step entry, the kite edge case entry) skip;
  the contexts live in the fragments.
- The git conventions: never force-push - overridden by the owner's
  explicit fix ask, executed as --force-with-lease against the
  known remote head (297ad44 verified by the fetch) so a concurrent
  session's push can never be destroyed.

## Progress

### 2026-09-22 10:40 UTC - the linearization

`git rebase origin/main` from 297ad44: 14 commits unique to the
branch, the 2 merge commits dropped by the replay, the 2 pure
legacy-log docs commits skipped (their only content was the frozen
log appends), 12 replayed - the agent_progress.md conflicts at the
docs/code commits resolved to main's frozen side. A backup ref
(backup/archer-archetype-check-297ad44) holds the pre-rebase state.

The result (head 7916967): 11 linear commits directly on dab5e4b,
`git diff 297ad44 7916967` EMPTY - the tree is bit-identical to the
CI-green head; `git diff origin/main HEAD -- docs/agent_progress.md`
EMPTY - the net PR diff carries zero legacy-log edits (the freeze
respected). task prepush OK (build, vet, lint, whitespace, the
cmd/swarm + gear + hunt suites).

### 2026-09-22 10:45 UTC - the lease-guarded push

`git push --force-with-lease=feature/archer-archetype-check:297ad44`
- the remote verified at 297ad44 (no concurrent session had pushed),
  the linearized history 297ad44..7916967 replaced it. All 11 CI
  checks green on 7916967 (the tree is the one that was already
  green - the history is the only change).

## Status

PR #31: mergeable clean, 11 commits, zero merge commits, sitting
on the current main - the rebase-and-merge button has nothing left
to refuse. The same linearization applies to the other
main-absorbed branches if the owner asks (the bow gear branch
feature/archer-bow-gear carries the same shape from this morning's
repair round).
