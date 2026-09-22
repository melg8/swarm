# The merge clash policy round - per-task fragments retire the shared hot files (status: in progress)

Started 2026-09-22, branch feature/merge-clash-policy, issue #33
("Reduce merge conflicts and agents clash").

## Goal

Every open PR currently conflicts with main on the same file:
measured 2026-09-22, 7 of the 8 open branches fail a plain
`git merge-tree` against main on `docs/agent_progress.md` alone, and
a sequential-merge simulation of all 8 conflicts on it at every step
plus `docs/development_log.md` twice. The AGENTS.md work protocol
mandates updating that single shared file with every atomic commit,
so any two concurrent agents are guaranteed to clash and every landed
PR forces a rebase on every other branch.

Done means: the policy stops mandating writes to shared hot files,
the crash-safe handover contract survives unchanged (pushed,
resumable task state), the permanent root-cause record survives
(per-round devlog fragments), and the transition is documented for
the branches still carrying legacy edits.

## Context

- The clash inventory (measured, see the devlog fragment of this
  round for the method): `docs/agent_progress.md` 8/8 branches
  (protocol-mandated per-commit appends at the same anchor),
  `docs/development_log.md` 5/8 (per-round appends),
  `docs/agent_progress_archive.md` 3/8 (finished-entry moves),
  `AGENTS.md` + `docs/README.md` 4/8 (documentation map row
  insertions at the same table anchor, low severity).
- Root cause: central append-only files with a shared write anchor.
  Two agents appending different entries to the same region of the
  same file is a conflict by construction; frequency comes from the
  protocol making the writes mandatory per commit.
- The fix pattern: fragment directories (one file per task / per
  round, the changelog-fragment convention) - different agents write
  different files, merges become set unions, conflicts become
  structurally impossible for these paths.
- The owner's board workflow (issues + claim comments + branch per
  task) already provides the discovery layer the old single file
  tried to provide; the fragment carries the per-task resume state.
- Constraint: the 8 already-open branches carry legacy
  agent_progress.md edits - they still need their one-time rebase;
  the policy must not add new conflict surface for future rounds.

## Progress

### 2026-09-22 05:20 UTC - docs: the fragment conventions

The two convention readers land: docs/progress/README.md (naming,
fragment structure, ownership and archive rules) and
docs/devlog/README.md (the per-round root-cause record). Next: the
AGENTS.md work protocol rewrite.

## Status

In progress: conventions written, the AGENTS.md protocol rewrite,
the legacy banners, the session_start.sh discovery update and this
fragment's own archive move are next.
